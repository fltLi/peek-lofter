package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/post"
)

// mergeKind selects the merge output semantics.
type mergeKind int

const (
	mergeRefs mergeKind = iota
	mergeWorks
)

// newMergeCmd merges ref JSONL files or downloaded work libraries.
func newMergeCmd(g *config.Global, deps *Deps) *cobra.Command {
	var (
		out   string
		force bool
	)
	cmd := &cobra.Command{
		Use:   "merge <input...>",
		Short: "Merge ref lists or downloaded work libraries",
		Long:  "Merge Ref JSONL files into one list, or merge downloaded zip libraries into one id-based library.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, err := detectMergeKind(args)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			if kind == mergeRefs {
				return mergeRefsRun(deps, g, args, out)
			}
			if out == "" {
				return &ExitError{Code: 2, Err: fmt.Errorf("merge: --out is required when merging works")}
			}
			return mergeWorksRun(deps, g, args, out, force)
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output file for refs or directory for works")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing work archives")
	return cmd
}

func detectMergeKind(args []string) (mergeKind, error) {
	var refs, works int
	for _, raw := range args {
		if raw == "-" {
			refs++
			continue
		}
		info, err := os.Stat(raw)
		if err != nil {
			return 0, err
		}
		if info.IsDir() {
			hasRefs := false
			if _, err := os.Stat(filepath.Join(raw, "refs.jsonl")); err == nil {
				hasRefs = true
			}
			hasWorks := hasAnyZip(raw)
			if hasRefs && hasWorks {
				return 0, fmt.Errorf("merge: %s contains both refs and works; merge them separately", raw)
			}
			if hasRefs {
				refs++
			} else if hasWorks {
				works++
			} else {
				return 0, fmt.Errorf("merge: %s contains neither refs.jsonl nor work archives", raw)
			}
			continue
		}
		switch strings.ToLower(filepath.Ext(raw)) {
		case ".jsonl":
			refs++
		case ".zip":
			works++
		default:
			return 0, fmt.Errorf("merge: unsupported input %s", raw)
		}
	}
	if refs > 0 && works > 0 {
		return 0, fmt.Errorf("merge: cannot merge refs and works in one command")
	}
	if refs > 0 {
		return mergeRefs, nil
	}
	return mergeWorks, nil
}

func hasAnyZip(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipAll
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".zip") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func mergeRefsRun(deps *Deps, g *config.Global, args []string, out string) error {
	byKey := map[string]int{}
	var refs []model.Ref
	read, invalid := 0, 0
	add := func(ref model.Ref) {
		key := post.RefKey(ref)
		if key == "" {
			invalid++
			return
		}
		read++
		if at, ok := byKey[key]; ok {
			fillRef(&refs[at], ref)
			return
		}
		byKey[key] = len(refs)
		refs = append(refs, ref)
	}
	for _, raw := range args {
		if raw == "-" {
			if err := readRefJSONL(deps.In, "stdin", add); err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			continue
		}
		info, err := os.Stat(raw)
		if err != nil {
			return err
		}
		path := raw
		if info.IsDir() {
			path = filepath.Join(raw, "refs.jsonl")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		err = readRefJSONL(f, path, add)
		_ = f.Close()
		if err != nil {
			return &ExitError{Code: 2, Err: err}
		}
	}
	w := deps.Out
	var tmp string
	if out != "" {
		if info, err := os.Stat(out); err == nil && info.IsDir() {
			return &ExitError{Code: 2, Err: fmt.Errorf("merge: --out is a directory: %s", out)}
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(out), ".merge-*")
		if err != nil {
			return err
		}
		tmp = f.Name()
		w = f
		defer func() {
			_ = f.Close()
			_ = os.Remove(tmp)
		}()
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, ref := range refs {
		if err := enc.Encode(ref); err != nil {
			return err
		}
	}
	if out != "" {
		if f, ok := w.(*os.File); ok {
			if err := f.Close(); err != nil {
				return err
			}
		}
		_ = os.Remove(out)
		if err := os.Rename(tmp, out); err != nil {
			return err
		}
	}
	if !g.Quiet {
		fmt.Fprintf(deps.Err, "merge\n  kind       refs\n  inputs     %d\n  read       %d\n  unique     %d\n  duplicates %d\n  invalid    %d\n", len(args), read, len(refs), read-len(refs), invalid)
		if out != "" {
			fmt.Fprintf(deps.Err, "  output     %s\n", out)
		}
	}
	return nil
}

func readRefJSONL(r io.Reader, source string, add func(model.Ref)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024), 4<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var ref model.Ref
		if err := json.Unmarshal([]byte(raw), &ref); err != nil {
			return fmt.Errorf("%s:%d: %w", source, line, err)
		}
		add(ref)
	}
	return sc.Err()
}

func fillRef(dst *model.Ref, src model.Ref) {
	if dst.ID == 0 {
		dst.ID = src.ID
	}
	if dst.URL == "" {
		dst.URL = src.URL
	}
	if dst.Permalink == "" {
		dst.Permalink = src.Permalink
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.Author.BlogID == 0 {
		dst.Author.BlogID = src.Author.BlogID
	}
	if dst.Author.BlogName == "" {
		dst.Author.BlogName = src.Author.BlogName
	}
	if dst.Author.Nickname == "" {
		dst.Author.Nickname = src.Author.Nickname
	}
	if dst.PublishedAt.IsZero() {
		dst.PublishedAt = src.PublishedAt
	}
	if dst.Type == 0 {
		dst.Type = src.Type
	}
	if dst.Source == nil {
		dst.Source = src.Source
	}
}

func mergeWorksRun(deps *Deps, g *config.Global, args []string, out string, force bool) error {
	type entry struct {
		src string
		p   model.Post
	}
	var entries []entry
	seen := map[string]bool{}
	read, skipped := 0, 0
	for _, raw := range args {
		info, err := os.Stat(raw)
		if err != nil {
			return err
		}
		if info.IsDir() {
			items, err := post.ScanArchives(raw)
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.Err != nil {
					skipped++
					continue
				}
				if _, err := post.ValidateArchive(item.Path, false); err != nil {
					skipped++
					continue
				}
				read++
				key := post.RefKey(item.Post.Ref)
				if key == "" {
					skipped++
					continue
				}
				if seen[key] && !force {
					continue
				}
				seen[key] = true
				entries = append(entries, entry{src: item.Path, p: item.Post})
			}
			continue
		}
		p, err := post.ValidateArchive(raw, false)
		if err != nil {
			skipped++
			continue
		}
		read++
		key := post.RefKey(p.Ref)
		if key == "" {
			skipped++
			continue
		}
		if seen[key] && !force {
			continue
		}
		seen[key] = true
		entries = append(entries, entry{src: raw, p: p})
	}

	written := 0
	for _, e := range entries {
		target, err := post.ArchivePath(out, e.p)
		if err != nil {
			skipped++
			continue
		}
		if filepath.Clean(target) == filepath.Clean(e.src) {
			skipped++
			continue
		}
		if !force {
			if _, err := os.Stat(target); err == nil {
				skipped++
				continue
			}
		}
		if err := copyFileAtomic(e.src, target); err != nil {
			return err
		}
		written++
	}
	if !g.Quiet {
		fmt.Fprintf(deps.Err, "merge\n  kind       works\n  inputs     %d\n  read       %d\n  unique     %d\n  duplicates %d\n  written    %d\n  skipped    %d\n  output     %s\n", len(args), read, len(entries), read-len(entries), written, skipped, out)
	}
	return nil
}

func copyFileAtomic(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".copy-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}
	if _, err := io.Copy(tmp, in); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	_ = os.Remove(dst)
	if err := os.Rename(name, dst); err != nil {
		cleanup()
		return err
	}
	return nil
}
