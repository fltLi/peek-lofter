package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/internal/render"
	"github.com/fltLi/peek-lofter/pkg/media"
	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/post"
)

// newPostCmd builds the download command. It only writes work archives.
func newPostCmd(g *config.Global, deps *Deps) *cobra.Command {
	var (
		from        []string
		blog        string
		out         string
		withMedia   bool
		withVideo   bool
		force       bool
		asJSON      bool
		dryRun      bool
		concurrency int
		limit       int
	)
	cmd := &cobra.Command{
		Use:     "post <ref...>",
		Short:   "Download posts into work archives",
		Long:    "Download posts from URLs, permalinks, numeric ids, --from files or stdin. Each work is written as one zip archive.",
		Args:    cobra.ArbitraryArgs,
		Example: "  peek-lofter post https://example-user.lofter.com/post/abc_def --out ./dl --with-media\n  peek-lofter tag example-tag | peek-lofter post --out ./dl",
		RunE: func(cmd *cobra.Command, args []string) error {
			refs, stats, err := collectRefs(args, from, deps, blog)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			if len(refs) == 0 {
				return &ExitError{Code: 2, Err: errors.New("post: no refs")}
			}
			if limit > 0 && len(refs) > limit {
				refs = refs[:limit]
			}
			if dryRun {
				return writeDryRun(deps, g, refs, stats)
			}
			if withVideo && !g.Quiet {
				fmt.Fprintln(deps.Err, "warning: --with-video is not implemented yet; video metadata is kept in meta.json")
			}
			client, err := clientFor(*g, *deps)
			if err != nil {
				return err
			}
			fetcher := &post.Fetcher{Source: client, Media: media.New(media.Options{})}
			report := newPostReport(deps.Err, deps.IsTTY, g.Quiet, g.Verbose, len(refs))
			defer report.Clear()
			sum, err := fetcher.FetchAll(cmd.Context(), refs, post.Options{
				Out:         out,
				WithMedia:   withMedia,
				WithVideo:   withVideo,
				Force:       force,
				Concurrency: concurrency,
			}, func(res post.Result) error {
				if asJSON {
					id, url, title := res.Ref.ID, res.Ref.URL, res.Ref.Title
					if id == 0 {
						id = res.Post.ID
					}
					if url == "" {
						url = res.Post.URL
					}
					if title == "" {
						title = res.Post.Title
					}
					m := map[string]any{
						"status": res.Status,
						"id":     id,
						"url":    url,
						"title":  title,
						"path":   res.Path,
					}
					if res.Err != nil {
						m["error"] = res.Err.Error()
					}
					return writeJSONLine(deps.Out, m)
				}
				report.Result(res)
				return nil
			})
			if err != nil {
				return err
			}
			report.Summary(sum)
			if !g.Quiet {
				fmt.Fprintf(deps.Err, "  output   %s\n", out)
			}
			if sum.Failed > 0 {
				return &ExitError{Code: 5, Silent: true}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&from, "from", nil, "read refs from file (repeatable, - for stdin)")
	cmd.Flags().StringVar(&blog, "blog", "", "blog name for numeric ids")
	cmd.Flags().StringVar(&out, "out", "peek-out", "output root directory")
	cmd.Flags().BoolVar(&withMedia, "with-media", false, "download images into work archives")
	cmd.Flags().BoolVar(&withVideo, "with-video", false, "keep video metadata (video files are not downloaded)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing work archives")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit one result JSON per post")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print a summary of planned refs without fetching")
	cmd.Flags().IntVar(&concurrency, "concurrency", g.Concurrency, "post download concurrency")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum refs to process; 0 = all")
	return cmd
}

// refStats records how many refs were read and deduped.
type refStats struct {
	Read    int
	Unique  int
	Dropped int
}

// collectRefs merges positional refs, files and stdin. Each line may be a Ref
// JSON object, a URL/permalink or the first column of a CSV.
func collectRefs(args, from []string, deps *Deps, blog string) ([]model.Ref, refStats, error) {
	resolver := post.Resolver{Blog: blog}
	var refs []model.Ref
	read := 0
	addLine := func(line string) error {
		line = strings.TrimSpace(line)
		if line == "" {
			return nil
		}
		var ref model.Ref
		if err := json.Unmarshal(
			[]byte(line),
			&ref,
		); err == nil &&
			(ref.ID != 0 || ref.URL != "" || ref.Permalink != "") {
			if ref.URL == "" && ref.Permalink != "" {
				ref.URL = model.PostURL(ref.Author.BlogName, ref.Permalink)
			}
			read++
			refs = append(refs, ref)
			return nil
		}
		if strings.Contains(line, ",") {
			first := strings.Trim(strings.SplitN(line, ",", 2)[0], "\" ")
			if first == "url" || first == "" {
				return nil
			}
			line = first
		}
		ref, err := resolver.Resolve(line)
		if err != nil {
			return err
		}
		read++
		refs = append(refs, ref)
		return nil
	}
	readReader := func(r io.Reader) error {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 1024), 1<<20)
		for sc.Scan() {
			if err := addLine(sc.Text()); err != nil {
				return err
			}
		}
		return sc.Err()
	}
	for _, a := range args {
		if err := addLine(a); err != nil {
			return nil, refStats{}, err
		}
	}
	for _, f := range from {
		if f == "-" {
			if err := readReader(deps.In); err != nil {
				return nil, refStats{}, err
			}
			continue
		}
		file, err := os.Open(f)
		if err != nil {
			return nil, refStats{}, err
		}
		err = readReader(file)
		_ = file.Close()
		if err != nil {
			return nil, refStats{}, err
		}
	}
	if len(args) == 0 && len(from) == 0 {
		if deps.IsTTY {
			return nil, refStats{}, errors.New("post: provide refs, --from, or pipe stdin")
		}
		if err := readReader(deps.In); err != nil {
			return nil, refStats{}, err
		}
	}
	unique := dedupeRefs(refs)
	return unique, refStats{Read: read, Unique: len(unique), Dropped: read - len(unique)}, nil
}

// dedupeRefs keeps the first ref for each id or URL.
func dedupeRefs(refs []model.Ref) []model.Ref {
	seen := map[string]bool{}
	out := make([]model.Ref, 0, len(refs))
	for _, ref := range refs {
		key := post.RefKey(ref)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ref)
	}
	return out
}

// writeDryRun prints a summary by default and full refs only when requested.
func writeDryRun(deps *Deps, g *config.Global, refs []model.Ref, stats refStats) error {
	switch g.Format {
	case "", "auto":
		if deps.IsTTY {
			return writeDryRunText(deps.Out, refs, stats)
		}
		return writeJSONLine(deps.Out, dryRunSummary(refs, stats))
	case "json":
		return writeJSONLine(deps.Out, dryRunSummary(refs, stats))
	case "list", "table", "jsonl", "ids", "urls":
		rw, err := render.NewRefWriter(
			render.Options{Format: g.Format, Fields: g.Fields, Out: deps.Out, IsTTY: deps.IsTTY},
		)
		if err != nil {
			return &ExitError{Code: 2, Err: err}
		}
		defer rw.Close()
		for _, ref := range refs {
			if err := rw.WriteRef(ref); err != nil {
				return err
			}
		}
		return nil
	default:
		return &ExitError{Code: 2, Err: fmt.Errorf("post: invalid --format %q", g.Format)}
	}
}

type dryRunStats struct {
	Refs    int    `json:"refs"`
	Read    int    `json:"read"`
	Dropped int    `json:"dropped"`
	First   string `json:"first,omitempty"`
	Last    string `json:"last,omitempty"`
}

func dryRunSummary(refs []model.Ref, stats refStats) dryRunStats {
	summary := dryRunStats{
		Refs:    len(refs),
		Read:    stats.Read,
		Dropped: stats.Dropped,
	}
	if len(refs) > 0 {
		summary.First = refs[0].URL
		summary.Last = refs[len(refs)-1].URL
	}
	return summary
}

func writeDryRunText(w io.Writer, refs []model.Ref, stats refStats) error {
	if _, err := fmt.Fprintf(
		w,
		"post: %d %s (%d read, %d duplicates)\n",
		len(refs),
		plural(len(refs), "ref", "refs"),
		stats.Read,
		stats.Dropped,
	); err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "first: %s\nlast: %s\n", refs[0].URL, refs[len(refs)-1].URL)
	return err
}
