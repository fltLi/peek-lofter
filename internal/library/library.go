// Package library scans local ref lists and downloaded work archives.
package library

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/post"
)

// Work is one local record: a ref from JSONL or a downloaded archive.
type Work struct {
	Ref  model.Ref
	Path string
	Tags []string
}

// Stats summarizes a scan.
type Stats struct {
	Inputs  int
	Records int
	Skipped int
}

// Scan reads paths, which may be .jsonl files, .zip files, directories, or -
// for refs on stdin. A directory may contain refs.jsonl and/or work archives.
func Scan(paths []string, stdin io.Reader) ([]Work, Stats, error) {
	var out []Work
	var stats Stats
	index := map[string]int{}
	add := func(w Work) {
		key := post.RefKey(w.Ref)
		if key == "" {
			stats.Skipped++
			return
		}
		if at, ok := index[key]; ok {
			if out[at].Path == "" && w.Path != "" {
				out[at] = w
			}
			return
		}
		index[key] = len(out)
		out = append(out, w)
	}

	for _, raw := range paths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		stats.Inputs++
		if raw == "-" {
			if stdin == nil {
				return nil, stats, fmt.Errorf("library: stdin requested but unavailable")
			}
			if err := readRefs(stdin, &stats, add); err != nil {
				return nil, stats, err
			}
			continue
		}
		info, err := os.Stat(raw)
		if err != nil {
			return nil, stats, err
		}
		if info.IsDir() {
			refsPath := filepath.Join(raw, "refs.jsonl")
			if _, err := os.Stat(refsPath); err == nil {
				f, err := os.Open(refsPath)
				if err != nil {
					return nil, stats, err
				}
				err = readRefs(f, &stats, add)
				_ = f.Close()
				if err != nil {
					return nil, stats, err
				}
			}
			if err := scanArchives(raw, &stats, add); err != nil {
				return nil, stats, err
			}
			continue
		}
		switch strings.ToLower(filepath.Ext(raw)) {
		case ".jsonl":
			f, err := os.Open(raw)
			if err != nil {
				return nil, stats, err
			}
			err = readRefs(f, &stats, add)
			_ = f.Close()
			if err != nil {
				return nil, stats, err
			}
		case ".zip":
			w, err := workFromArchive(raw)
			if err != nil {
				stats.Skipped++
				continue
			}
			stats.Records++
			add(w)
		default:
			return nil, stats, fmt.Errorf("library: unsupported input %s", raw)
		}
	}
	return out, stats, nil
}

func readRefs(r io.Reader, stats *Stats, add func(Work)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024), 4<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ref model.Ref
		if err := json.Unmarshal([]byte(line), &ref); err != nil {
			stats.Skipped++
			continue
		}
		if post.RefKey(ref) == "" {
			stats.Skipped++
			continue
		}
		if ref.URL == "" && ref.Permalink != "" {
			ref.URL = model.PostURL(ref.Author.BlogName, ref.Permalink)
		}
		stats.Records++
		add(Work{Ref: ref, Tags: sourceTags(ref)})
	}
	return sc.Err()
}

func scanArchives(root string, stats *Stats, add func(Work)) error {
	items, err := post.ScanArchives(root)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Err != nil {
			stats.Skipped++
			continue
		}
		p, err := post.ValidateArchive(item.Path, false)
		if err != nil {
			stats.Skipped++
			continue
		}
		if post.RefKey(p.Ref) == "" {
			stats.Skipped++
			continue
		}
		stats.Records++
		add(Work{Ref: p.Ref, Path: item.Path, Tags: workTags(p)})
	}
	return nil
}

func workFromArchive(path string) (Work, error) {
	p, err := post.ValidateArchive(path, false)
	if err != nil {
		return Work{}, err
	}
	return Work{Ref: p.Ref, Path: path, Tags: workTags(p)}, nil
}

func workTags(p model.Post) []string {
	tags := append([]string(nil), p.Tags...)
	if p.Source != nil && p.Source.Name != "" {
		tags = append(tags, p.Source.Name)
	}
	return tags
}

func sourceTags(ref model.Ref) []string {
	if ref.Source != nil && ref.Source.Name != "" {
		return []string{ref.Source.Name}
	}
	return nil
}

// Filters selects local works for browse.
type Filters struct {
	Query       string
	Author      string
	Tags        []string
	ExcludeTags []string
	Type        string
	Since       time.Time
	Until       time.Time
	Content     bool
}

// Filter returns matching works and the number excluded by tag filters.
func Filter(works []Work, f Filters) ([]Work, int, error) {
	query := strings.ToLower(strings.TrimSpace(f.Query))
	author := strings.ToLower(strings.TrimSpace(f.Author))
	include := normalizeTags(f.Tags)
	exclude := normalizeTags(f.ExcludeTags)
	var out []Work
	excluded := 0
	for _, w := range works {
		if !matchType(w.Ref.Type, f.Type) {
			continue
		}
		if !matchDate(w.Ref.PublishedAt, f.Since, f.Until) {
			continue
		}
		if author != "" &&
			!strings.Contains(strings.ToLower(w.Ref.Author.BlogName), author) &&
			!strings.Contains(strings.ToLower(w.Ref.Author.Nickname), author) {
			continue
		}
		tags := normalizeTags(w.Tags)
		if len(include) > 0 && !anyTag(tags, include) {
			continue
		}
		if len(exclude) > 0 && anyTag(tags, exclude) {
			excluded++
			continue
		}
		if query != "" && !matchQuery(w, query, f.Content) {
			continue
		}
		out = append(out, w)
	}
	return out, excluded, nil
}

func matchQuery(w Work, query string, withContent bool) bool {
	fields := []string{w.Ref.Title, w.Ref.Author.BlogName, w.Ref.Author.Nickname, w.Ref.URL, strings.Join(w.Tags, " ")}
	if withContent && w.Path != "" {
		if content, err := post.ReadContent(w.Path); err == nil {
			fields = append(fields, post.ToText(content))
		}
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func matchType(kind int, want string) bool {
	switch want {
	case "", "all":
		return true
	case "text":
		return kind == 1
	case "image":
		return kind == 2
	case "video":
		return kind == 4
	default:
		return true
	}
}

func matchDate(ts model.Timestamp, since, until time.Time) bool {
	if since.IsZero() && until.IsZero() {
		return true
	}
	if ts.IsZero() {
		return false
	}
	t := ts.Time
	if !since.IsZero() && t.Before(since) {
		return false
	}
	if !until.IsZero() && !t.Before(until) {
		return false
	}
	return true
}

func normalizeTags(in []string) []string {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		tag := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#")))
		if tag != "" {
			out = append(out, tag)
		}
	}
	return out
}

func anyTag(tags, wanted []string) bool {
	for _, tag := range tags {
		for _, want := range wanted {
			if tag == want {
				return true
			}
		}
	}
	return false
}

// Sort orders works newest first with a stable id tie-breaker.
func Sort(works []Work) {
	sortWorks(works)
}
