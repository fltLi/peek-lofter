package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fltLi/peek-lofter/pkg/model"
)

func TestListRefWriterStreams(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewRefWriter(Options{Format: "list", Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	ref := model.Ref{
		ID:          123,
		URL:         "https://example-user.lofter.com/post/abc_def",
		Title:       "example title",
		Author:      model.Author{BlogName: "example-user"},
		PublishedAt: model.FromUnixMilli(1714883772000),
	}
	if err := w.WriteRef(ref); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"DATE", "example-user", "example title", ref.URL} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "{") {
		t.Fatalf("list output should not be JSON: %q", out)
	}
}

func TestAutoRefWriterUsesListOnTTY(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewRefWriter(Options{Format: "auto", IsTTY: true, Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteRef(model.Ref{ID: 1, URL: "https://example-user.lofter.com/post/abc_def"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "DATE") {
		t.Fatalf("auto TTY output is not list: %q", buf.String())
	}
}
