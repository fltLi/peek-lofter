package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/internal/render"
	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/store"
	"github.com/fltLi/peek-lofter/pkg/user"
)

// newUserCmd builds the profile command and its data-type subcommands.
func newUserCmd(g *config.Global, deps *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user <user>",
		Short: "Show a LOFTER user's profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFor(*g, *deps)
			if err != nil {
				return err
			}
			svc := user.New(client)
			u, err := svc.Profile(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return render.WriteUser(
				deps.Out,
				u,
				render.Options{Format: g.Format, Fields: g.Fields, Out: deps.Out, IsTTY: deps.IsTTY},
			)
		},
	}
	cmd.AddCommand(newUserListCmd(g, deps, "posts", "List a user's posts"))
	cmd.AddCommand(newUserListCmd(g, deps, "likes", "List posts a user liked"))
	cmd.AddCommand(newUserListCmd(g, deps, "recommends", "List posts a user recommended"))
	return cmd
}

// newUserListCmd builds posts, likes or recommends for one user.
func newUserListCmd(g *config.Global, deps *Deps, kind, short string) *cobra.Command {
	var (
		limit int
		since string
		until string
		out   string
	)
	cmd := &cobra.Command{
		Use:   kind + " <user>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUserList(cmd, g, deps, kind, args[0], limit, since, until, out)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "stop after N refs; 0 = all")
	cmd.Flags().StringVar(&out, "out", "", "dataset directory; omit to stream Ref JSONL to stdout")
	if kind == "posts" {
		cmd.Flags().StringVar(&since, "since", "", "only posts on/after this date")
		cmd.Flags().StringVar(&until, "until", "", "only posts before this date")
	}
	return cmd
}

// runUserList streams refs from the user service to stdout or a dataset.
func runUserList(
	cmd *cobra.Command,
	g *config.Global,
	deps *Deps,
	kind, blog string,
	limit int,
	since, until, out string,
) error {
	sinceT, err := parseDate(since, false)
	if err != nil {
		return &ExitError{Code: 2, Err: err}
	}
	untilT, err := parseDate(until, true)
	if err != nil {
		return &ExitError{Code: 2, Err: err}
	}
	client, err := clientFor(*g, *deps)
	if err != nil {
		return err
	}
	svc := user.New(client)
	var ds *store.Dataset
	if out != "" {
		ds, err = store.Open(out)
		if err != nil {
			return err
		}
		defer ds.Close()
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
	count := 0
	emit := func(ref model.Ref) error {
		if ds != nil {
			if err := ds.AppendRef(ref); err != nil {
				return err
			}
		}
		if rw != nil {
			if err := rw.WriteRef(ref); err != nil {
				return err
			}
		}
		count++
		return nil
	}
	opt := user.Options{Limit: limit, Since: sinceT, Until: untilT}
	switch kind {
	case "posts":
		err = svc.Posts(cmd.Context(), blog, opt, emit)
	case "likes":
		err = svc.Likes(cmd.Context(), blog, opt, emit)
	case "recommends":
		err = svc.Recommends(cmd.Context(), blog, opt, emit)
	default:
		err = fmt.Errorf("unknown user list %q", kind)
	}
	if err != nil {
		return err
	}
	if ds != nil {
		return writeJSONLine(deps.Out, map[string]any{"refs": count})
	}
	return nil
}
