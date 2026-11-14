package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/internal/library"
	"github.com/fltLi/peek-lofter/internal/render"
)

// newBrowseCmd builds the local browsing and search entry point.
func newBrowseCmd(g *config.Global, deps *Deps) *cobra.Command {
	var (
		query       string
		author      string
		tags        []string
		excludeTags []string
		typeArg     string
		since       string
		until       string
		content     bool
		page        int
		pageSize    int
		noPager     bool
		paths       bool
	)
	cmd := &cobra.Command{
		Use:   "browse [path...]",
		Short: "Browse and search local refs and works",
		Long:  "Browse local ref JSONL files or downloaded work archives. The pager is local-only and never downloads.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				args = []string{"peek-out"}
			}
			if typeArg != "" && typeArg != "all" && typeArg != "text" && typeArg != "image" && typeArg != "video" {
				return &ExitError{Code: 2, Err: fmt.Errorf("browse: invalid --type %q (all|text|image|video)", typeArg)}
			}
			sinceT, err := parseDate(since, false)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			untilT, err := parseDate(until, true)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			works, stats, err := library.Scan(args, deps.In)
			if err != nil {
				return err
			}
			hasRefs := false
			for _, w := range works {
				if w.Path == "" {
					hasRefs = true
					break
				}
			}
			if content && hasRefs && !g.Quiet {
				fmt.Fprintln(deps.Err, "warning: --content only applies to downloaded works")
			}
			matches, excluded, err := library.Filter(works, library.Filters{
				Query:       query,
				Author:      author,
				Tags:        tags,
				ExcludeTags: excludeTags,
				Type:        typeArg,
				Since:       sinceT,
				Until:       untilT,
				Content:     content,
			})
			if err != nil {
				return err
			}
			library.Sort(matches)

			if paths {
				n := 0
				for _, w := range matches {
					if w.Path == "" {
						continue
					}
					fmt.Fprintln(deps.Out, w.Path)
					n++
				}
				if !g.Quiet {
					fmt.Fprintf(deps.Err, "browse\n  items    %d\n  matches  %d\n  paths    %d\n", stats.Records, len(matches), n)
				}
				return nil
			}

			pageSet := cmd.Flags().Changed("page")
			format := g.Format
			if format == "" || format == "auto" {
				if deps.IsTTY || pageSet {
					format = "list"
				} else {
					format = "jsonl"
				}
			}
			if format != "list" && format != "table" {
				rw, err := render.NewRefWriter(render.Options{Format: format, Fields: g.Fields, Out: deps.Out, IsTTY: deps.IsTTY})
				if err != nil {
					return &ExitError{Code: 2, Err: err}
				}
				defer rw.Close()
				for _, w := range matches {
					if err := rw.WriteRef(w.Ref); err != nil {
						return err
					}
				}
				printBrowseSummary(deps, g, stats, len(matches), excluded)
				return nil
			}

			if g.Verbose {
				for _, w := range matches {
					printWork(deps, w)
				}
				printBrowseSummary(deps, g, stats, len(matches), excluded)
				return nil
			}

			rows := make([]string, 0, len(matches))
			for _, w := range matches {
				rows = append(rows, render.FormatRefList(w.Ref))
			}
			if err := writePages(rows, pageConfig{
				Page:     page,
				PageSize: pageSize,
				PageSet:  pageSet,
				NoPager:  noPager || !deps.IsTTY,
				TTY:      deps.IsTTY,
				In:       deps.In,
				Out:      deps.Out,
				Prompt:   deps.Err,
				Header:   render.RefListHeader(),
			}); err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			printBrowseSummary(deps, g, stats, len(matches), excluded)
			return nil
		},
	}
	cmd.Flags().StringVarP(&query, "query", "q", "", "substring query")
	cmd.Flags().StringVar(&author, "author", "", "author blogName or nickname")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "include tag (repeatable, any match)")
	cmd.Flags().StringArrayVar(&excludeTags, "exclude-tag", nil, "exclude tag (repeatable, any match)")
	cmd.Flags().StringVar(&typeArg, "type", "all", "post type: all|text|image|video")
	cmd.Flags().StringVar(&since, "since", "", "only works on/after this date")
	cmd.Flags().StringVar(&until, "until", "", "only works before this date")
	cmd.Flags().BoolVar(&content, "content", false, "also search downloaded HTML content")
	cmd.Flags().IntVar(&page, "page", 1, "show only this page")
	cmd.Flags().IntVar(&pageSize, "page-size", 20, "rows per page")
	cmd.Flags().BoolVar(&noPager, "no-pager", false, "print all rows")
	cmd.Flags().BoolVar(&paths, "paths", false, "print work archive paths only")
	return cmd
}

// printBrowseSummary writes local scan and match counts to stderr.
func printBrowseSummary(deps *Deps, g *config.Global, stats library.Stats, matches, excluded int) {
	if g.Quiet {
		return
	}
	fmt.Fprintf(deps.Err, "browse\n  items    %d\n  matches  %d\n", stats.Records, matches)
	if excluded > 0 {
		fmt.Fprintf(deps.Err, "  excluded %d\n", excluded)
	}
	if stats.Skipped > 0 {
		fmt.Fprintf(deps.Err, "  skipped  %d\n", stats.Skipped)
	}
}

// oneLine collapses whitespace for single-line display.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// printWork writes verbose details for one local record.
func printWork(deps *Deps, w library.Work) {
	fmt.Fprintf(deps.Out, "%d  %s\n", w.Ref.ID, oneLine(w.Ref.Title))
	fmt.Fprintf(deps.Out, "  author  %s (%s)\n", w.Ref.Author.BlogName, w.Ref.Author.Nickname)
	if !w.Ref.PublishedAt.IsZero() {
		fmt.Fprintf(deps.Out, "  date    %s\n", w.Ref.PublishedAt.Format("2006-01-02"))
	}
	if w.Ref.URL != "" {
		fmt.Fprintf(deps.Out, "  url     %s\n", w.Ref.URL)
	}
	if len(w.Tags) > 0 {
		fmt.Fprintf(deps.Out, "  tags    %s\n", strings.Join(w.Tags, ", "))
	}
	if w.Path != "" {
		fmt.Fprintf(deps.Out, "  path    %s\n", w.Path)
	}
}
