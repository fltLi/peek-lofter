package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/pkg/post"
)

// newExportCmd builds the local archive exporter.
func newExportCmd(g *config.Global, deps *Deps) *cobra.Command {
	var (
		formats     []string
		out         string
		layout      string
		authorField string
		withTitle   bool
		force       bool
		onConflict  string
		stdout      bool
		dryRun      bool
	)
	cmd := &cobra.Command{
		Use:   "export <zip|dir...>",
		Short: "Export downloaded works to txt, md or html",
		Long:  "Export local work archives. Inputs are zip files, directories containing work archives, or zip paths on stdin.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formatList, err := normalizeExportFormats(formats)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			if !validLayout(layout) {
				return &ExitError{Code: 2, Err: fmt.Errorf("export: invalid --layout %q (id|author-id|author-title|title)", layout)}
			}
			if !validConflict(onConflict) {
				return &ExitError{Code: 2, Err: fmt.Errorf("export: invalid --on-conflict %q (suffix|skip|error)", onConflict)}
			}
			if authorField != "blogName" && authorField != "nickname" {
				return &ExitError{Code: 2, Err: fmt.Errorf("export: invalid --author-field %q (blogName|nickname)", authorField)}
			}
			zips, err := collectArchives(args, deps)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			if len(zips) == 0 {
				return &ExitError{Code: 2, Err: fmt.Errorf("export: no work archives found")}
			}
			if stdout {
				if len(zips) != 1 || len(formatList) != 1 {
					return &ExitError{Code: 2, Err: fmt.Errorf("export: --stdout requires exactly one archive and one format")}
				}
				data, err := post.ExportStdout(zips[0], formatList[0], withTitle)
				if err != nil {
					return err
				}
				_, err = deps.Out.Write(data)
				return err
			}
			opt := post.ExportOptions{
				Format:      formatList,
				Out:         out,
				Layout:      layout,
				AuthorField: authorField,
				WithTitle:   withTitle,
				Force:       force,
				Conflict:    onConflict,
				DryRun:      dryRun,
			}
			works, files, skipped, failed := 0, 0, 0, 0
			for _, zip := range zips {
				res, err := post.ExportOne(zip, opt)
				if err != nil {
					failed++
					fmt.Fprintf(deps.Err, "error  %s  %v\n", zip, err)
					continue
				}
				works++
				if dryRun {
					files += len(res.Paths)
				} else {
					files += res.Written
				}
				skipped += res.Skipped
				if g.Verbose {
					for _, path := range res.Paths {
						fmt.Fprintf(deps.Err, "exported  %s\n", path)
					}
				}
			}
			if !g.Quiet {
				fmt.Fprintf(deps.Err, "export\n  works    %d\n  files    %d\n  skipped  %d\n  failed   %d\n", works, files, skipped, failed)
				if !dryRun {
					fmt.Fprintf(deps.Err, "  output   %s\n", out)
				}
			}
			if failed > 0 {
				return &ExitError{Code: 5, Silent: true}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&formats, "format", nil, "export formats: txt,md,html")
	cmd.Flags().StringVar(&out, "out", "export", "output root directory")
	cmd.Flags().StringVar(&layout, "layout", post.LayoutAuthorTitle, "output layout: id|author-id|author-title|title")
	cmd.Flags().StringVar(&authorField, "author-field", "blogName", "author name field: blogName|nickname")
	cmd.Flags().BoolVar(&withTitle, "with-title", false, "include the title in txt/md output")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing exports")
	cmd.Flags().StringVar(&onConflict, "on-conflict", post.ConflictSuffix, "name conflict policy: suffix|skip|error")
	cmd.Flags().BoolVar(&stdout, "stdout", false, "write a single export to stdout")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print planned paths without writing")
	return cmd
}

// normalizeExportFormats validates and deduplicates requested formats.
func normalizeExportFormats(in []string) ([]string, error) {
	if len(in) == 0 {
		return []string{post.FormatTxt}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			switch part {
			case post.FormatTxt, post.FormatMD, post.FormatHTML:
			default:
				return nil, fmt.Errorf("export: invalid --format %q (txt|md|html)", part)
			}
			if !seen[part] {
				seen[part] = true
				out = append(out, part)
			}
		}
	}
	return out, nil
}

// validLayout reports whether a layout name is supported.
func validLayout(layout string) bool {
	switch layout {
	case post.LayoutID, post.LayoutAuthorID, post.LayoutAuthorTitle, post.LayoutTitle:
		return true
	default:
		return false
	}
}

// validConflict reports whether a conflict policy is supported.
func validConflict(policy string) bool {
	switch policy {
	case post.ConflictSuffix, post.ConflictSkip, post.ConflictError:
		return true
	default:
		return false
	}
}

// collectArchives expands zip files and directories into archive paths.
func collectArchives(args []string, deps *Deps) ([]string, error) {
	var zips []string
	seen := map[string]bool{}
	add := func(path string) {
		path = filepath.Clean(path)
		if seen[path] {
			return
		}
		seen[path] = true
		zips = append(zips, path)
	}
	addDir := func(dir string) error {
		items, err := post.ScanArchives(dir)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Err != nil {
				continue
			}
			add(item.Path)
		}
		return nil
	}
	for _, raw := range args {
		if raw == "-" {
			sc := bufio.NewScanner(deps.In)
			sc.Buffer(make([]byte, 1024), 1<<20)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "" {
					continue
				}
				if err := addPath(line, add, addDir); err != nil {
					return nil, err
				}
			}
			if err := sc.Err(); err != nil {
				return nil, err
			}
			continue
		}
		if err := addPath(raw, add, addDir); err != nil {
			return nil, err
		}
	}
	return zips, nil
}

// addPath classifies one export input and rejects Ref JSONL.
func addPath(path string, add func(string), addDir func(string) error) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return addDir(path)
	}
	if strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return fmt.Errorf("export: input contains refs, not downloaded works; run post first")
	}
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		return fmt.Errorf("export: unsupported input %s", path)
	}
	add(path)
	return nil
}
