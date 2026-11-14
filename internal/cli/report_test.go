package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/post"
	"github.com/fltLi/peek-lofter/pkg/tag"
)

func testWindow() tag.Window {
	return tag.Window{
		Tag:   "example-tag",
		Start: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
	}
}

func TestTagProgressNonTTY(t *testing.T) {
	var buf bytes.Buffer
	w := testWindow()
	p := newTagProgress(tagProgressOptions{
		Out:   &buf,
		Until: w.End,
		Since: w.Start,
		Tags:  []string{"example-tag"},
	})
	p.Window(w)
	p.Ref(model.Ref{ID: 1})
	p.WindowDone(w, tag.WindowResult{Status: "complete"})
	p.Done(tag.Summary{Refs: 1})
	p.Clear()
	out := buf.String()
	if !strings.Contains(out, "tag example-tag  requested 2026-11-01..2026-11-02") {
		t.Fatalf("missing header: %q", out)
	}
	for _, bad := range []string{"window", "page", "refs", "[window]"} {
		if strings.Contains(out, bad) {
			t.Fatalf("progress leaked %q: %q", bad, out)
		}
	}
}

func TestTagProgressHeartbeat(t *testing.T) {
	var buf bytes.Buffer
	w := testWindow()
	p := newTagProgress(tagProgressOptions{Out: &buf, Until: w.End, Since: w.Start, Tags: []string{"example-tag"}})
	p.Window(w)
	p.now = func() time.Time { return p.start.Add(3 * time.Second) }
	p.lastBeat = p.start
	p.Ref(model.Ref{ID: 1})
	if !strings.Contains(buf.String(), "[00:03] example-tag") {
		t.Fatalf("missing heartbeat: %q", buf.String())
	}
	p.Clear()
}

func TestTagProgressVerbose(t *testing.T) {
	var buf bytes.Buffer
	w := testWindow()
	p := newTagProgress(tagProgressOptions{
		Out:     &buf,
		Verbose: true,
		Until:   w.End,
		Since:   w.Start,
		Tags:    []string{"example-tag"},
	})
	p.Window(w)
	p.Ref(model.Ref{ID: 1})
	p.WindowDone(w, tag.WindowResult{Status: "complete"})
	out := buf.String()
	for _, want := range []string{"2026-11-01..2026-11-01", "complete", "1 post"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	for _, bad := range []string{"window", "page", "refs"} {
		if strings.Contains(out, bad) {
			t.Fatalf("verbose leaked %q: %q", bad, out)
		}
	}
}

func TestTagProgressTTYLive(t *testing.T) {
	var buf bytes.Buffer
	w := testWindow()
	p := newTagProgress(tagProgressOptions{
		Out:   &buf,
		TTY:   true,
		Until: w.End,
		Since: w.Start,
		Tags:  []string{"example-tag"},
	})
	p.Window(w)
	p.Ref(model.Ref{ID: 1})
	p.WindowDone(w, tag.WindowResult{Status: "complete"})
	out := buf.String()
	for _, want := range []string{"\r", "completed", "posts 1", "100%"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	p.Done(tag.Summary{})
	p.Clear()
}

func TestPostReportDefaultErrors(t *testing.T) {
	var buf bytes.Buffer
	r := newPostReport(&buf, false, false, false, 7)
	for range 7 {
		r.Result(post.Result{Ref: model.Ref{URL: "u"}, Status: "failed", Err: errors.New("boom")})
	}
	r.Summary(post.Summary{Total: 7, Failed: 7})
	out := buf.String()
	if got := strings.Count(out, "u  boom"); got != 5 {
		t.Fatalf("got %d error lines in %q", got, out)
	}
	if !strings.Contains(out, "(2 more; use --verbose)") {
		t.Fatalf("missing truncation hint: %q", out)
	}
	if !strings.Contains(out, "post\n  total    7\n  saved    0\n  skipped  0\n  failed   7") {
		t.Fatalf("missing summary: %q", out)
	}
}

func TestPostReportQuiet(t *testing.T) {
	var buf bytes.Buffer
	r := newPostReport(&buf, false, true, false, 1)
	r.Result(post.Result{Ref: model.Ref{URL: "u"}, Status: "failed", Err: errors.New("boom")})
	r.Summary(post.Summary{Total: 1, Failed: 1})
	out := buf.String()
	if !strings.Contains(out, "u  boom") {
		t.Fatalf("quiet mode lost the error: %q", out)
	}
	if strings.Contains(out, "post\n") {
		t.Fatalf("quiet mode printed a summary: %q", out)
	}
}

func TestPostReportVerboseSaved(t *testing.T) {
	var buf bytes.Buffer
	r := newPostReport(&buf, false, false, true, 1)
	r.Result(post.Result{Ref: model.Ref{URL: "u"}, Path: "dir", Status: "saved"})
	r.Summary(post.Summary{Total: 1, Saved: 1})
	out := buf.String()
	if !strings.Contains(out, "saved  dir") {
		t.Fatalf("missing verbose saved line: %q", out)
	}
	if !strings.Contains(out, "post\n  total    1") {
		t.Fatalf("missing summary: %q", out)
	}
}

func TestPostReportDefaultSuccess(t *testing.T) {
	var buf bytes.Buffer
	r := newPostReport(&buf, false, false, false, 1)
	r.Result(post.Result{Ref: model.Ref{URL: "u"}, Path: "dir", Status: "saved"})
	r.Summary(post.Summary{Total: 1, Saved: 1})
	out := buf.String()
	if strings.Contains(out, "saved  dir") {
		t.Fatalf("default mode printed per-post output: %q", out)
	}
	if !strings.Contains(out, "post\n  total    1") {
		t.Fatalf("missing summary: %q", out)
	}
}

func TestPostReportSkipped(t *testing.T) {
	var buf bytes.Buffer
	r := newPostReport(&buf, false, false, false, 1)
	r.Result(post.Result{Ref: model.Ref{URL: "u"}, Path: "dir", Status: "skipped"})
	r.Summary(post.Summary{Total: 1, Skipped: 1})
	out := buf.String()
	if !strings.Contains(out, "skipped  1") {
		t.Fatalf("missing skipped count: %q", out)
	}
}
