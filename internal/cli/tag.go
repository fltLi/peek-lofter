package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/internal/render"
	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/store"
	"github.com/fltLi/peek-lofter/pkg/tag"
)

// newTagCmd builds the unified tag command. A bare invocation crawls fully;
// --limit samples the feed.
func newTagCmd(g *config.Global, deps *Deps) *cobra.Command {
	var (
		since       string
		until       string
		out         string
		typeArg     string
		limit       int
		emptyMonths int
		strict      bool
		resume      bool
	)
	cmd := &cobra.Command{
		Use:     "tag <tag...>",
		Short:   "Crawl posts under one or more tags",
		Long:    "Crawl a tag fully. Time coverage, retries and dedupe happen internally. Use --limit for a sample.",
		Args:    cobra.MinimumNArgs(1),
		Example: "  peek-lofter tag example-tag --since 2024-01-01 --out ./data/example-tag\n  peek-lofter tag example-tag --limit 10",
		RunE: func(cmd *cobra.Command, args []string) error {
			postTypes, err := mapTypes(typeArg)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			sinceT, err := parseDate(since, false)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			untilT, err := parseDate(until, true)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			if untilT.IsZero() {
				untilT = deps.Now()
			}
			if emptyMonths < 0 {
				return &ExitError{Code: 2, Err: errors.New("--empty-months must be >= 0")}
			}
			planner, err := tag.NewPlanner(tag.Options{
				Tags:  args,
				Since: sinceT,
				Until: untilT,
				Types: postTypes,
			})
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			windows, err := planner.Plan()
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			client, err := clientFor(*g, *deps)
			if err != nil {
				return err
			}
			tags := splitTagArgs(args)
			var sink tag.Sink
			var ds *store.Dataset
			if out != "" {
				ds, err = store.Open(out)
				if err != nil {
					return err
				}
				defer ds.Close()
				datasetSince := sinceT
				if datasetSince.IsZero() {
					datasetSince = untilT.AddDate(-10, 0, 0)
				}
				// Persist parameters before the first window so resume is explainable.
				ds.Configure(tags, datasetSince, untilT, postTypes)
				if resume {
					sink = ds
				} else {
					sink = noResumeSink{Dataset: ds}
				}
			}
			var rw render.RefWriter
			if out == "" {
				rw, err = render.NewRefWriter(
					render.Options{Format: g.Format, Fields: g.Fields, Out: deps.Out, IsTTY: deps.IsTTY},
				)
				if err != nil {
					return &ExitError{Code: 2, Err: err}
				}
				defer rw.Close()
			}
			emit := func(ref model.Ref) error {
				if rw != nil {
					return rw.WriteRef(ref)
				}
				return nil
			}
			progress := newTagProgress(tagProgressOptions{
				Out:     deps.Err,
				TTY:     deps.IsTTY,
				Quiet:   g.Quiet,
				Verbose: g.Verbose,
				Since:   sinceT,
				Until:   untilT,
				Tags:    tags,
			})
			defer progress.Clear()
			stopAfter := 0
			if since == "" || cmd.Flags().Changed("empty-months") {
				stopAfter = emptyMonths
			}
			runner := &tag.Runner{Source: client, Sink: sink, Progress: progress, Limit: limit, StopAfterEmpty: stopAfter}
			sum, runErr := runner.Run(cmd.Context(), windows, emit)
			coveredStart, _ := progress.Coverage()
			info := tagRunInfo{
				tags:          tags,
				since:         sinceT,
				until:         untilT,
				sinceExplicit: since != "",
				coveredStart:  coveredStart,
				elapsed:       progress.Elapsed(),
				emptyMonths:   stopAfter,
			}
			if ds != nil {
				_ = ds.SaveManifest()
				st, _ := ds.LoadState()
				st.LastSync = deps.Now()
				_ = ds.SaveState(st)
				if err := writeJSONLine(deps.Out, tagSummary(sum, info)); err != nil {
					return err
				}
			} else if runErr == nil && !g.Quiet {
				fmt.Fprintln(deps.Err, formatTagSummary(sum, info))
			}
			if runErr != nil {
				var pe *tag.PartialError
				if errors.As(runErr, &pe) {
					if out == "" && !g.Quiet {
						fmt.Fprintln(deps.Err, formatTagSummary(sum, info))
					}
					return &ExitError{Code: 5, Silent: true}
				}
				return runErr
			}
			if strict && (sum.Suspect > 0 || sum.Unresolved > 0) {
				return &ExitError{Code: 5, Silent: true}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "start date (default: walk back until --empty-months is reached)")
	cmd.Flags().StringVar(&until, "until", "", "end date (default: now)")
	cmd.Flags().StringVar(&out, "out", "", "dataset directory; omit to stream Ref output to stdout")
	cmd.Flags().StringVar(&typeArg, "type", "all", "post type: all|text|image|video")
	cmd.Flags().IntVar(&limit, "limit", 0, "stop after N refs; 0 = all")
	cmd.Flags().IntVar(&emptyMonths, "empty-months", 6, "stop after N consecutive empty months; 0 = never (walk-back)")
	cmd.Flags().BoolVar(&strict, "strict", false, "exit 5 on an incomplete period")
	cmd.Flags().BoolVar(&resume, "resume", true, "resume already crawled periods when --out is set")
	return cmd
}

// noResumeSink makes the runner ignore completed windows.
type noResumeSink struct {
	*store.Dataset
}

func (s noResumeSink) Completed(string) bool { return false }

// mapTypes converts the user-facing --type value to API postTypes. The API
// returns only one item for 1,2,3,4,5,6 and does not paginate empty values
// under a date filter, so all types are 1,2,4.
func mapTypes(s string) (string, error) {
	switch s {
	case "", "all":
		return "1,2,4", nil
	case "text":
		return "1", nil
	case "image":
		return "2", nil
	case "video":
		return "4", nil
	default:
		return "", fmt.Errorf("invalid --type %q (all|text|image|video)", s)
	}
}

type tagRunInfo struct {
	tags          []string
	since         time.Time
	until         time.Time
	sinceExplicit bool
	coveredStart  time.Time
	elapsed       time.Duration
	emptyMonths   int
}

// tagSummaryJSON is the machine-readable summary printed with --out.
type tagSummaryJSON struct {
	Status         string   `json:"status"`
	Tags           []string `json:"tags"`
	Since          string   `json:"since,omitempty"`
	Until          string   `json:"until"`
	CompletedSince string   `json:"completedSince,omitempty"`
	Posts          int      `json:"posts"`
	Elapsed        string   `json:"elapsed"`
	Reason         string   `json:"reason,omitempty"`
}

// tagSummary shapes the machine-readable summary printed with --out.
func tagSummary(sum tag.Summary, info tagRunInfo) tagSummaryJSON {
	status, reason := tagStatus(sum)
	out := tagSummaryJSON{
		Status:  status,
		Tags:    info.tags,
		Until:   info.until.Format(time.DateOnly),
		Posts:   sum.Refs,
		Elapsed: shortDuration(info.elapsed),
		Reason:  reason,
	}
	if info.sinceExplicit {
		out.Since = info.since.Format(time.DateOnly)
	}
	if !info.coveredStart.IsZero() {
		out.CompletedSince = info.coveredStart.Format(time.DateOnly)
	}
	return out
}

// formatTagSummary is the human-readable summary block.
func formatTagSummary(sum tag.Summary, info tagRunInfo) string {
	status, reason := tagStatus(sum)
	statusText := status
	switch reason {
	case "limit":
		statusText += " (limit reached)"
	case "empty":
		statusText += fmt.Sprintf(" (no posts for %d months)", info.emptyMonths)
	}
	requested := "auto (up to 10 years)"
	if info.sinceExplicit {
		requested = fmt.Sprintf("%s..%s", info.since.Format(time.DateOnly), info.until.Format(time.DateOnly))
	}
	completed := "none"
	if !info.coveredStart.IsZero() {
		completed = fmt.Sprintf("%s..%s", info.coveredStart.Format(time.DateOnly), info.until.Format(time.DateOnly))
	}
	return fmt.Sprintf(
		"tag %s\n  requested  %s\n  completed  %s\n  posts      %d\n  elapsed    %s\n  status     %s",
		tagLabel(info.tags),
		requested,
		completed,
		sum.Refs,
		shortDuration(info.elapsed),
		statusText,
	)
}

func tagStatus(sum tag.Summary) (string, string) {
	if sum.Suspect > 0 || sum.Unresolved > 0 {
		return "partial", ""
	}
	if sum.StoppedByLimit {
		return "stopped", "limit"
	}
	if sum.StoppedByEmpty {
		return "stopped", "empty"
	}
	return "complete", ""
}

func tagLabel(tags []string) string {
	if len(tags) == 0 {
		return "unknown"
	}
	if len(tags) == 1 {
		return tags[0]
	}
	return fmt.Sprintf("%s (+%d)", tags[0], len(tags)-1)
}

// splitTagArgs accepts both repeated args and comma-separated tags.
func splitTagArgs(args []string) []string {
	var out []string
	for _, raw := range args {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
