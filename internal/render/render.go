package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/fltLi/peek-lofter/pkg/model"
)

// Options selects an output format, field projection and destination.
type Options struct {
	Format string
	Fields []string
	Out    io.Writer
	IsTTY  bool
}

// RefWriter streams refs to one output format.
type RefWriter interface {
	WriteRef(model.Ref) error
	Close() error
}

// NewRefWriter picks a writer for the requested format. Auto uses list on a
// TTY and JSONL otherwise, so pipes stay machine-readable.
func NewRefWriter(opt Options) (RefWriter, error) {
	format := opt.Format
	if format == "" || format == "auto" {
		if opt.IsTTY {
			format = "list"
		} else {
			format = "jsonl"
		}
	}
	switch format {
	case "list":
		return newListRefWriter(opt.Out), nil
	case "table":
		return newTableRefWriter(opt.Out), nil
	case "jsonl":
		return &jsonlRefWriter{enc: newEncoder(opt.Out), fields: opt.Fields}, nil
	case "json":
		return &jsonRefWriter{out: opt.Out, fields: opt.Fields}, nil
	case "ids":
		return &lineRefWriter{out: opt.Out, field: "id"}, nil
	case "urls":
		return &lineRefWriter{out: opt.Out, field: "url"}, nil
	default:
		return nil, fmt.Errorf("render: unknown format %q", format)
	}
}

// WriteUser writes a profile as a table, JSON or JSONL.
func WriteUser(w io.Writer, u model.User, opt Options) error {
	format := opt.Format
	if format == "" || format == "auto" {
		if opt.IsTTY {
			fmt.Fprintf(w, "blogId:   %d\n", u.BlogID)
			fmt.Fprintf(w, "blogName: %s\n", u.BlogName)
			fmt.Fprintf(w, "nickname: %s\n", u.Nickname)
			for _, a := range u.Archives {
				total := 0
				for _, n := range a.Months {
					total += n
				}
				fmt.Fprintf(w, "%d: %d\n", a.Year, total)
			}
			return nil
		}
		format = "json"
	}
	if format == "table" {
		fmt.Fprintln(w, "BLOGID\tNAME\tNICKNAME")
		fmt.Fprintf(w, "%d\t%s\t%s\n", u.BlogID, u.BlogName, u.Nickname)
		return nil
	}
	enc := newEncoder(w)
	if format == "jsonl" {
		return enc.Encode(u)
	}
	return encodeIndent(w, u)
}

type jsonlRefWriter struct {
	enc    *json.Encoder
	fields []string
}

func (w *jsonlRefWriter) WriteRef(ref model.Ref) error {
	if len(w.fields) > 0 {
		return w.enc.Encode(ProjectRef(ref, w.fields))
	}
	return w.enc.Encode(ref)
}
func (w *jsonlRefWriter) Close() error { return nil }

type jsonRefWriter struct {
	out    io.Writer
	fields []string
	refs   []model.Ref
}

func (w *jsonRefWriter) WriteRef(ref model.Ref) error {
	w.refs = append(w.refs, ref)
	return nil
}

func (w *jsonRefWriter) Close() error {
	if len(w.fields) > 0 {
		out := make([]map[string]any, 0, len(w.refs))
		for _, ref := range w.refs {
			out = append(out, ProjectRef(ref, w.fields))
		}
		return encodeIndent(w.out, out)
	}
	return encodeIndent(w.out, w.refs)
}

type lineRefWriter struct {
	out   io.Writer
	field string
}

func (w *lineRefWriter) WriteRef(ref model.Ref) error {
	switch w.field {
	case "id":
		_, err := fmt.Fprintln(w.out, ref.ID)
		return err
	default:
		_, err := fmt.Fprintln(w.out, ref.URL)
		return err
	}
}
func (w *lineRefWriter) Close() error { return nil }

// RefListHeader is the header for the streaming list format.
func RefListHeader() string {
	return fmt.Sprintf(
		"%s  %s  %s  %s  %s",
		padRight("ID", 10),
		padRight("DATE", 10),
		padRight("AUTHOR", 20),
		padRight("TITLE", 40),
		"URL",
	)
}

// FormatRefList formats one ref as a human-readable list line.
func FormatRefList(ref model.Ref) string {
	date := ""
	if !ref.PublishedAt.IsZero() {
		date = ref.PublishedAt.Format("2006-01-02")
	}
	return fmt.Sprintf(
		"%s  %s  %s  %s  %s",
		padRight(strconv.FormatInt(ref.ID, 10), 10),
		padRight(date, 10),
		padRight(truncate(oneLine(ref.Author.BlogName), 20), 20),
		padRight(truncate(oneLine(ref.Title), 40), 40),
		ref.URL,
	)
}

// listRefWriter streams one human-readable line per ref.
type listRefWriter struct {
	out io.Writer
}

func newListRefWriter(out io.Writer) *listRefWriter {
	fmt.Fprintln(out, RefListHeader())
	return &listRefWriter{out: out}
}

func (w *listRefWriter) WriteRef(ref model.Ref) error {
	_, err := fmt.Fprintln(w.out, FormatRefList(ref))
	return err
}
func (w *listRefWriter) Close() error { return nil }

type tableRefWriter struct {
	tw *tabwriter.Writer
}

func newTableRefWriter(out io.Writer) *tableRefWriter {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "DATE\tAUTHOR\tTITLE\tID\tURL")
	return &tableRefWriter{tw: tw}
}

func (w *tableRefWriter) WriteRef(ref model.Ref) error {
	date := ""
	if !ref.PublishedAt.IsZero() {
		date = ref.PublishedAt.Format("2006-01-02")
	}
	title := strings.ReplaceAll(ref.Title, "\t", " ")
	author := ref.Author.BlogName
	_, err := fmt.Fprintf(w.tw, "%s\t%s\t%s\t%d\t%s\n", date, author, title, ref.ID, ref.URL)
	return err
}

func (w *tableRefWriter) Close() error { return w.tw.Flush() }

func encodeIndent(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func newEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc
}

// ProjectRef builds a small object containing only the requested fields.
func ProjectRef(ref model.Ref, fields []string) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		switch f {
		case "id":
			out["id"] = ref.ID
		case "url":
			out["url"] = ref.URL
		case "permalink":
			out["permalink"] = ref.Permalink
		case "title":
			out["title"] = ref.Title
		case "type":
			out["type"] = ref.Type
		case "publishedAt":
			out["publishedAt"] = ref.PublishedAt
		case "author", "author.blogName":
			out["author"] = map[string]any{"blogName": ref.Author.BlogName}
		case "author.name", "author.nickname":
			out["author"] = map[string]any{"name": ref.Author.Nickname}
		case "author.blogId":
			out["author"] = map[string]any{"blogId": ref.Author.BlogID}
		}
	}
	return out
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 3 {
		return string(r[:width])
	}
	return string(r[:width-3]) + "..."
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
