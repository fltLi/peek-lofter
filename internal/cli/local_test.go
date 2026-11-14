package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/post"
)

func cliArchive(t *testing.T, root string, id int64, title string, tags ...string) string {
	t.Helper()
	p := model.Post{
		Ref: model.Ref{
			ID:          id,
			URL:         model.PostURL("example-user", "abc_def"),
			Permalink:   "abc_def",
			Title:       title,
			Author:      model.Author{BlogID: 1, BlogName: "example-user", Nickname: "Example"},
			PublishedAt: model.FromUnixMilli(1714900572387),
		},
		Content: "<p>keyword body</p>",
		Tags:    tags,
	}
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
	path, _, err := post.WriteArchive(root, p, staging, false)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func cliRef(id int64, title, tag string) model.Ref {
	return model.Ref{
		ID:          id,
		URL:         model.PostURL("example-user", "abc_def"),
		Title:       title,
		Author:      model.Author{BlogID: 1, BlogName: "example-user", Nickname: "Example"},
		PublishedAt: model.FromUnixMilli(1714900572387),
		Source:      &model.Source{Kind: "tag", Name: tag},
	}
}

func writeRefs(t *testing.T, path string, refs ...model.Ref) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, ref := range refs {
		if err := enc.Encode(ref); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocalCommandsHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"--help"}, Deps{Out: &out, Err: &errb}); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	for _, want := range []string{"browse", "export", "merge", "post"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q: %s", want, out.String())
		}
	}
	out.Reset()
	errb.Reset()
	if code := Execute(context.Background(), []string{"post", "--help"}, Deps{Out: &out, Err: &errb}); code != 0 {
		t.Fatalf("code=%d", code)
	}
	for _, bad := range []string{"--print", "--content", "--export"} {
		if strings.Contains(out.String(), bad) {
			t.Fatalf("post help still has %q: %s", bad, out.String())
		}
	}
}

func TestPostPrintRemoved(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"post", "https://example-user.lofter.com/post/abc_def", "--print"}, Deps{Out: &out, Err: &errb})
	if code != 2 || !strings.Contains(errb.String(), "print") {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}

func TestBrowseJSONLExcludeTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "refs.jsonl")
	writeRefs(t, path, cliRef(7, "Keep", "example-tag"), cliRef(8, "Drop", "nsfw"))
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"browse", path, "--format", "jsonl", "--exclude-tag", "nsfw"}, Deps{Out: &out, Err: &errb})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "\"id\":7") || strings.Contains(out.String(), "\"id\":8") {
		t.Fatalf("unexpected output: %q", out.String())
	}
	if !strings.Contains(errb.String(), "excluded 1") {
		t.Fatalf("missing excluded summary: %q", errb.String())
	}
}

func TestBrowsePager(t *testing.T) {
	dir := t.TempDir()
	cliArchive(t, dir, 7, "One", "example-tag")
	cliArchive(t, dir, 8, "Two", "example-tag")
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"browse", dir, "--page-size", "1"}, Deps{
		Out:   &out,
		Err:   &errb,
		In:    strings.NewReader("n\nq\n"),
		IsTTY: true,
	})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "page 1/2") || !strings.Contains(out.String(), "page 2/2") {
		t.Fatalf("missing paged output: %q", out.String())
	}
}

func TestExportRejectsRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "refs.jsonl")
	writeRefs(t, path, cliRef(7, "One", "example-tag"))
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"export", path}, Deps{Out: &out, Err: &errb})
	if code != 2 || !strings.Contains(errb.String(), "not downloaded works") {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}

func TestExportZip(t *testing.T) {
	dir := t.TempDir()
	zip := cliArchive(t, dir, 7, "Example title", "example-tag")
	outDir := t.TempDir()
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"export", zip, "--format", "txt", "--layout", "author-title", "--out", outDir}, Deps{Out: &out, Err: &errb})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	path := filepath.Join(outDir, "example-user", "Example title.txt")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing export: %v\n%s", err, errb.String())
	}
}

func TestMergeRefs(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jsonl")
	b := filepath.Join(dir, "b.jsonl")
	writeRefs(t, a, cliRef(7, "", "example-tag"))
	writeRefs(t, b, cliRef(7, "Filled", "example-tag"), cliRef(8, "Other", "example-tag"))
	outPath := filepath.Join(dir, "merged.jsonl")
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"merge", a, b, "-o", outPath}, Deps{Out: &out, Err: &errb})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "\"title\":\"Filled\"") {
		t.Fatalf("unexpected merged refs: %s", raw)
	}
}

func TestMergeMixedKinds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "refs.jsonl")
	writeRefs(t, path, cliRef(7, "One", "example-tag"))
	zip := cliArchive(t, dir, 8, "Two", "example-tag")
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"merge", path, zip, "-o", filepath.Join(dir, "out")}, Deps{Out: &out, Err: &errb})
	if code != 2 || !strings.Contains(errb.String(), "cannot merge refs and works") {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}

func TestMergeWorks(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	cliArchive(t, a, 7, "One", "example-tag")
	cliArchive(t, b, 8, "Two", "example-tag")
	outDir := t.TempDir()
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"merge", a, b, "--out", outDir}, Deps{Out: &out, Err: &errb})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "1", "7.zip")); err != nil {
		t.Fatalf("missing merged work: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "1", "8.zip")); err != nil {
		t.Fatalf("missing merged work: %v", err)
	}
}
