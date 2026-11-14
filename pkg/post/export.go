package post

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fltLi/peek-lofter/pkg/model"
)

// Export layouts.
const (
	LayoutID          = "id"
	LayoutAuthorID    = "author-id"
	LayoutAuthorTitle = "author-title"
	LayoutTitle       = "title"
)

// Export formats.
const (
	FormatTxt  = "txt"
	FormatMD   = "md"
	FormatHTML = "html"
)

// Conflict policies.
const (
	ConflictSuffix = "suffix"
	ConflictSkip   = "skip"
	ConflictError  = "error"
)

// ExportOptions controls derived text output.
type ExportOptions struct {
	Format      []string
	Out         string
	Layout      string
	AuthorField string
	WithTitle   bool
	Force       bool
	Conflict    string
	DryRun      bool
}

// ExportResult reports one archive's export.
type ExportResult struct {
	Paths   []string
	Written int
	Skipped int
}

// ExportOne converts one work archive into the requested formats.
func ExportOne(path string, opt ExportOptions) (ExportResult, error) {
	p, content, err := ReadArchive(path)
	if err != nil {
		return ExportResult{}, err
	}
	formats, err := normalizeFormats(opt.Format)
	if err != nil {
		return ExportResult{}, err
	}
	opt = normalizeExportOptions(opt)
	var res ExportResult
	for _, format := range formats {
		data, err := RenderFormat(p, content, format, opt.WithTitle)
		if err != nil {
			return res, err
		}
		target, err := ExportPath(opt.Out, p, format, opt)
		if err != nil {
			return res, err
		}
		status, final, err := writeExport(target, data, p, opt)
		if err != nil {
			return res, err
		}
		res.Paths = append(res.Paths, final)
		switch status {
		case "written":
			res.Written++
		case "skipped":
			res.Skipped++
		}
	}
	return res, nil
}

// ExportStdout renders one archive in one format.
func ExportStdout(path, format string, withTitle bool) ([]byte, error) {
	p, content, err := ReadArchive(path)
	if err != nil {
		return nil, err
	}
	return RenderFormat(p, content, format, withTitle)
}

// RenderFormat converts HTML content to one export format.
func RenderFormat(p model.Post, content, format string, withTitle bool) ([]byte, error) {
	switch format {
	case FormatTxt:
		body := ToText(content)
		if withTitle {
			body = titleHeader(p.Title, false) + body
		}
		return []byte(body), nil
	case FormatMD:
		body := ToMarkdown(content)
		if withTitle {
			body = titleHeader(p.Title, true) + body
		}
		return []byte(body), nil
	case FormatHTML:
		return []byte(content), nil
	default:
		return nil, fmt.Errorf("post: unknown export format %q", format)
	}
}

// ExportPath builds an output path for one work and format.
func ExportPath(out string, p model.Post, format string, opt ExportOptions) (string, error) {
	if out == "" {
		out = "export"
	}
	ext := "." + format
	author := exportAuthor(p, opt.AuthorField)
	title := exportTitle(p.Title)
	switch opt.Layout {
	case LayoutID:
		if p.Author.BlogID <= 0 || p.ID <= 0 {
			return "", errors.New("post: missing ids for id export layout")
		}
		return filepath.Join(out, strconv.FormatInt(p.Author.BlogID, 10), strconv.FormatInt(p.ID, 10)+ext), nil
	case LayoutAuthorID:
		if p.ID <= 0 {
			return "", errors.New("post: missing post id for author-id export layout")
		}
		return filepath.Join(out, author, strconv.FormatInt(p.ID, 10)+ext), nil
	case LayoutTitle:
		return filepath.Join(out, title+ext), nil
	case LayoutAuthorTitle:
		return filepath.Join(out, author, title+ext), nil
	default:
		return "", fmt.Errorf("post: unknown export layout %q", opt.Layout)
	}
}

// NormalizeURL canonicalizes a post URL for deduplication.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Keep this dependency-free: trim fragment and query, lower scheme/host.
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		raw = raw[:i]
	}
	scheme := ""
	if i := strings.Index(raw, "://"); i > 0 {
		scheme = strings.ToLower(raw[:i])
		raw = raw[i+3:]
	}
	host := raw
	if i := strings.IndexByte(raw, '/'); i >= 0 {
		host = raw[:i]
	}
	host = strings.ToLower(host)
	path := strings.TrimSuffix(raw[len(host):], "/")
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + host + path
}

func normalizeFormats(in []string) ([]string, error) {
	if len(in) == 0 {
		return []string{FormatTxt}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			switch part {
			case FormatTxt, FormatMD, FormatHTML:
			default:
				return nil, fmt.Errorf("post: invalid export format %q (txt|md|html)", part)
			}
			if !seen[part] {
				seen[part] = true
				out = append(out, part)
			}
		}
	}
	if len(out) == 0 {
		return []string{FormatTxt}, nil
	}
	return out, nil
}

func normalizeExportOptions(opt ExportOptions) ExportOptions {
	if opt.Out == "" {
		opt.Out = "export"
	}
	if opt.Layout == "" {
		opt.Layout = LayoutAuthorTitle
	}
	if opt.AuthorField == "" {
		opt.AuthorField = "blogName"
	}
	if opt.Conflict == "" {
		opt.Conflict = ConflictSuffix
	}
	return opt
}

func exportAuthor(p model.Post, field string) string {
	name := p.Author.BlogName
	if field == "nickname" {
		name = p.Author.Nickname
	}
	name = sanitizeName(name)
	if name == "" {
		name = "unknown-author"
	}
	return name
}

func exportTitle(title string) string {
	title = sanitizeName(title)
	if title == "" {
		return "untitled"
	}
	return title
}

func titleHeader(title string, markdown bool) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if markdown {
		return "# " + title + "\n\n"
	}
	return title + "\n\n"
}

func writeExport(target string, data []byte, p model.Post, opt ExportOptions) (string, string, error) {
	if !opt.Force {
		if existing, err := os.ReadFile(target); err == nil {
			if bytes.Equal(existing, data) {
				return "skipped", target, nil
			}
			switch opt.Conflict {
			case ConflictSkip:
				return "skipped", target, nil
			case ConflictError:
				return "", target, fmt.Errorf("post: export target exists: %s", target)
			}
			target = conflictPath(target, p.ID)
		}
	}
	if opt.DryRun {
		return "written", target, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", target, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".export-*")
	if err != nil {
		return "", target, err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return "", target, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", target, err
	}
	if opt.Force {
		_ = os.Remove(target)
	}
	if err := os.Rename(tmpName, target); err != nil {
		cleanup()
		return "", target, err
	}
	return "written", target, nil
}

func conflictPath(target string, postID int64) string {
	ext := filepath.Ext(target)
	base := strings.TrimSuffix(target, ext)
	suffix := "_" + strconv.FormatInt(postID, 10)
	candidate := base + suffix + ext
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = base + suffix + "_" + strconv.Itoa(i) + ext
	}
}

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitizeName removes characters forbidden on Windows and caps the length.
func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r < 32 || strings.ContainsRune("<>:\"/\\|?*", r) {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	s = strings.TrimRight(b.String(), " .")
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:80])
	}
	if s == "" {
		return ""
	}
	if reservedNames[strings.ToUpper(s)] {
		s = "_" + s
	}
	return s
}
