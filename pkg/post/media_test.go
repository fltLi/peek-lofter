package post

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fltLi/peek-lofter/pkg/model"
)

type fakeMedia struct{}

func (fakeMedia) Download(_ context.Context, _ model.Image, dir, base string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, base+".jpg")
	if err := os.WriteFile(path, []byte("image"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func TestSaveWithMedia(t *testing.T) {
	p := testPost()
	p.Images = []model.Image{{Raw: "https://example.invalid/a.jpg"}}
	f := &Fetcher{Media: fakeMedia{}}
	path, skipped, err := f.Save(context.Background(), p, Options{Out: t.TempDir(), WithMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("unexpected skip")
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, file := range zr.File {
		names[file.Name] = true
	}
	for _, want := range []string{"meta.json", "content.html", "images/001.jpg"} {
		if !names[want] {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	if _, err := ValidateArchive(path, true); err != nil {
		t.Fatal(err)
	}
}

func TestSaveSkipsBeforeMedia(t *testing.T) {
	root := t.TempDir()
	p := testPost()
	p.Images = []model.Image{{Raw: "https://example.invalid/a.jpg"}}
	f := &Fetcher{Media: fakeMedia{}}
	path, _, err := f.Save(context.Background(), p, Options{Out: root, WithMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	// Remove the image from the archive's metadata to prove the second call
	// skips based on the archive rather than downloading again.
	f2 := &Fetcher{Media: panicMedia{}}
	if _, skipped, err := f2.Save(context.Background(), p, Options{Out: root, WithMedia: false}); err != nil || !skipped {
		t.Fatalf("expected skip, skipped=%v err=%v path=%s", skipped, err, path)
	}
}

type panicMedia struct{}

func (panicMedia) Download(context.Context, model.Image, string, string) (string, error) {
	panic("media should not be called")
}

func TestSaveAddsMediaToExistingArchive(t *testing.T) {
	root := t.TempDir()
	p := testPost()
	p.Images = []model.Image{{Raw: "https://example.invalid/a.jpg"}}
	f := &Fetcher{Media: fakeMedia{}}
	if _, _, err := f.Save(context.Background(), p, Options{Out: root}); err != nil {
		t.Fatal(err)
	}
	path, skipped, err := f.Save(context.Background(), p, Options{Out: root, WithMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("media request should rewrite the archive")
	}
	if _, err := ValidateArchive(path, true); err != nil {
		t.Fatal(err)
	}
}

func TestTextNoLinks(t *testing.T) {
	got := ToText(`<p><a href="https://example.invalid">link</a></p>`)
	if strings.Contains(got, "https://example.invalid") || !strings.Contains(got, "link") {
		t.Fatalf("unexpected text: %q", got)
	}
}
