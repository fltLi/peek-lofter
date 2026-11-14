package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/pkg/lofter"
	"github.com/fltLi/peek-lofter/pkg/version"
)

// Deps injects process-level dependencies so commands are testable.
type Deps struct {
	In        io.Reader
	Out       io.Writer
	Err       io.Writer
	IsTTY     bool
	Version   string
	Now       func() time.Time
	NewClient func(config.Global) (*lofter.Client, error)
}

// ExitError carries a process exit code and can suppress duplicate printing.
type ExitError struct {
	Code   int
	Err    error
	Silent bool
}

// Error implements the error interface.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying error for errors.Is and errors.As.
func (e *ExitError) Unwrap() error { return e.Err }

// Execute runs the CLI with args and returns the process exit code.
func Execute(ctx context.Context, args []string, deps Deps) int {
	if deps.In == nil {
		deps.In = os.Stdin
	}
	if deps.Out == nil {
		deps.Out = os.Stdout
	}
	if deps.Err == nil {
		deps.Err = os.Stderr
	}
	if deps.Version == "" {
		deps.Version = version.Version
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	root := newRoot(&deps)
	root.SetArgs(args)
	root.SetContext(ctx)
	err := root.Execute()
	if err == nil {
		return 0
	}
	code := 1
	silent := false
	var ee *ExitError
	if errors.As(err, &ee) {
		code = ee.Code
		silent = ee.Silent
	} else if errors.Is(err, lofter.ErrNotFound) {
		code = 3
	} else if errors.Is(err, lofter.ErrNetwork) || errors.Is(err, lofter.ErrUpstream) || errors.Is(err, lofter.ErrRateLimited) {
		code = 4
	}
	if !silent {
		fmt.Fprintln(deps.Err, "error:", err)
	}
	return code
}

// newRoot builds the command tree and binds global flags.
func newRoot(deps *Deps) *cobra.Command {
	g := config.Default()
	config.FromEnv(&g)
	root := &cobra.Command{
		Use:           "peek-lofter",
		Short:         "Read-only LOFTER CLI: tags, users, posts and local works",
		Long:          "peek-lofter crawls public LOFTER tags, users and posts without login.\nDownloads are work zips; browse and export run locally.\nData goes to stdout, progress and errors go to stderr.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       deps.Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(deps.Out)
	root.SetErr(deps.Err)
	root.SetIn(deps.In)
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &ExitError{Code: 2, Err: err}
	})

	pf := root.PersistentFlags()
	pf.StringVar(&g.Format, "format", "auto", "output format: auto|list|table|jsonl|json|ids|urls")
	pf.StringSliceVar(&g.Fields, "fields", nil, "comma-separated fields for jsonl/json output")
	pf.DurationVar(&g.Rate, "rate", g.Rate, "minimum interval between requests")
	pf.DurationVar(&g.Timeout, "timeout", g.Timeout, "per-request timeout")
	pf.IntVar(&g.Retries, "retries", g.Retries, "retry count for transient errors")
	pf.IntVar(&g.Concurrency, "concurrency", g.Concurrency, "post download concurrency")
	pf.BoolVar(&g.Verbose, "verbose", false, "print per-period and per-post details")
	pf.BoolVar(&g.Quiet, "quiet", false, "only print errors")

	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		if g.Quiet && g.Verbose {
			return &ExitError{Code: 2, Err: errors.New("--quiet and --verbose are mutually exclusive")}
		}
		return nil
	}

	root.AddCommand(newTagCmd(&g, deps))
	root.AddCommand(newUserCmd(&g, deps))
	root.AddCommand(newPostCmd(&g, deps))
	root.AddCommand(newBrowseCmd(&g, deps))
	root.AddCommand(newExportCmd(&g, deps))
	root.AddCommand(newMergeCmd(&g, deps))
	root.AddCommand(newVersionCmd(deps))
	return root
}

func newVersionCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(
				deps.Out,
				"peek-lofter %s\ncommit: %s\nbuilt: %s\n",
				version.Version,
				version.Commit,
				version.Date,
			)
			return nil
		},
	}
}

// clientFor uses the injected factory when present, otherwise the real client.
func clientFor(g config.Global, deps Deps) (*lofter.Client, error) {
	if deps.NewClient != nil {
		return deps.NewClient(g)
	}
	return lofter.New(lofter.Options{
		Rate:    g.Rate,
		Timeout: g.Timeout,
		Retries: g.Retries,
	})
}

// IsTerminal reports whether w is a character device.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

var cst = time.FixedZone("UTC+8", 8*60*60)

// parseDate accepts a date or RFC3339. Date-only end bounds are inclusive by
// moving the exclusive bound to the next local midnight.
func parseDate(s string, end bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(cst), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, cst); err == nil {
		if end {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid date %q (use 2006-01-02 or RFC3339)", s)
}

// writeJSONLine writes one compact JSON object per line without HTML escaping.
func writeJSONLine(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
