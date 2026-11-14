package post

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fltLi/peek-lofter/pkg/model"
)

func testPost() model.Post {
	return model.Post{
		Ref: model.Ref{
			ID:        7,
			URL:       "https://example-user.lofter.com/post/abc_def",
			Permalink: "abc_def",
			Title:     "Example title",
			Author:    model.Author{BlogID: 1, BlogName: "example-user", Nickname: "Example"},
		},
		Content: "<p>hello</p><p>world</p>",
		Tags:    []string{"example-tag"},
	}
}

func writeTestArchive(t *testing.T, root string, p model.Post) string {
	t.Helper()
	staging := t.TempDir()
	meta := p
	meta.Content = ""
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "meta.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "content.html"), []byte(p.Content), 0o644); err != nil {
		t.Fatal(err)
	}
	path, skipped, err := WriteArchive(root, p, staging, false)
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("unexpected skip")
	}
	return path
}

func TestArchiveRoundTrip(t *testing.T) {
	root := t.TempDir()
	path := writeTestArchive(t, root, testPost())
	if filepath.ToSlash(path) != filepath.ToSlash(filepath.Join(root, "1", "7.zip")) {
		t.Fatalf("unexpected path: %s", path)
	}
	p, content, err := ReadArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 7 || p.Author.BlogID != 1 {
		t.Fatalf("unexpected meta: %+v", p)
	}
	if p.Content != "" {
		t.Fatalf("meta should not duplicate content: %q", p.Content)
	}
	if !strings.Contains(content, "hello") {
		t.Fatalf("unexpected content: %q", content)
	}
	if _, err := ValidateArchive(path, false); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveSkipAndForce(t *testing.T) {
	root := t.TempDir()
	p := testPost()
	writeTestArchive(t, root, p)
	staging := t.TempDir()
	meta := p
	meta.Content = ""
	raw, _ := json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(staging, "meta.json"), raw, 0o644)
	_ = os.WriteFile(filepath.Join(staging, "content.html"), []byte(p.Content), 0o644)
	_, skipped, err := WriteArchive(root, p, staging, false)
	if err != nil {
		t.Fatal(err)
	}
	if !skipped {
		t.Fatal("expected skip")
	}
}

func TestScanArchives(t *testing.T) {
	root := t.TempDir()
	writeTestArchive(t, root, testPost())
	items, err := ScanArchives(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Err != nil || items[0].Post.ID != 7 {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestTextPreservesParagraphs(t *testing.T) {
	got := ToText("<p>one</p><p>two</p><ul><li>a</li><li>b</li></ul>")
	for _, want := range []string{"one\n\ntwo", "- a", "- b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestExportLayouts(t *testing.T) {
	p := testPost()
	cases := []struct {
		layout string
		want   string
	}{
		{LayoutID, "out/1/7.txt"},
		{LayoutAuthorID, "out/example-user/7.txt"},
		{LayoutAuthorTitle, "out/example-user/Example title.txt"},
		{LayoutTitle, "out/Example title.txt"},
	}
	for _, tc := range cases {
		got, err := ExportPath("out", p, FormatTxt, ExportOptions{Layout: tc.layout, AuthorField: "blogName"})
		if err != nil {
			t.Fatalf("%s: %v", tc.layout, err)
		}
		if filepath.ToSlash(got) != tc.want {
			t.Fatalf("%s: got %s want %s", tc.layout, got, tc.want)
		}
	}
}

func TestExportOneFormatsAndTitle(t *testing.T) {
	root := t.TempDir()
	path := writeTestArchive(t, root, testPost())
	out := t.TempDir()
	res, err := ExportOne(path, ExportOptions{
		Format:    []string{"txt", "md"},
		Out:       out,
		Layout:    LayoutAuthorTitle,
		WithTitle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 2 || len(res.Paths) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	txt, err := os.ReadFile(filepath.Join(out, "example-user", "Example title.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(txt), "Example title\n\n") {
		t.Fatalf("missing title header: %q", txt)
	}
	md, err := os.ReadFile(filepath.Join(out, "example-user", "Example title.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(md), "# Example title") {
		t.Fatalf("missing markdown title: %q", md)
	}
}

func TestExportConflictSuffix(t *testing.T) {
	root := t.TempDir()
	path := writeTestArchive(t, root, testPost())
	out := t.TempDir()
	opt := ExportOptions{Format: []string{"txt"}, Out: out, Layout: LayoutTitle}
	if _, err := ExportOne(path, opt); err != nil {
		t.Fatal(err)
	}
	p2 := testPost()
	p2.ID = 8
	p2.Content = "<p>different</p>"
	path2 := writeTestArchive(t, root, p2)
	res, err := ExportOne(path2, opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 1 || !strings.HasSuffix(res.Paths[0], "_8.txt") {
		t.Fatalf("unexpected conflict result: %+v", res)
	}
}
