package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/post"
)

func makeArchive(t *testing.T, root string, p model.Post) string {
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
	path, _, err := post.WriteArchive(root, p, staging, false)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func samplePost(id int64, title string, tags ...string) model.Post {
	return model.Post{
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
}

func TestScanRefsAndWorksDedupes(t *testing.T) {
	root := t.TempDir()
	zip := makeArchive(t, root, samplePost(7, "Work", "example-tag"))
	refsPath := filepath.Join(root, "refs.jsonl")
	ref := model.Ref{ID: 7, URL: "https://example-user.lofter.com/post/abc_def", Title: "Ref", Author: model.Author{BlogID: 1, BlogName: "example-user"}}
	other := model.Ref{ID: 8, URL: "https://example-user.lofter.com/post/xyz", Title: "Other", Author: model.Author{BlogID: 1, BlogName: "example-user"}}
	raw, _ := json.Marshal(ref)
	raw2, _ := json.Marshal(other)
	if err := os.WriteFile(refsPath, append(append(raw, '\n'), append(raw2, '\n')...), 0o644); err != nil {
		t.Fatal(err)
	}
	works, stats, err := Scan([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Records != 3 {
		t.Fatalf("records=%d", stats.Records)
	}
	if len(works) != 2 {
		t.Fatalf("works=%d", len(works))
	}
	if works[0].Ref.ID != 7 || works[0].Path != zip {
		t.Fatalf("work record should win: %+v", works[0])
	}
}

func TestFilterExcludeTagAndContent(t *testing.T) {
	root := t.TempDir()
	makeArchive(t, root, samplePost(7, "Keep", "example-tag"))
	makeArchive(t, root, samplePost(8, "Drop", "nsfw"))
	works, _, err := Scan([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, excluded, err := Filter(works, Filters{ExcludeTags: []string{"#NSFW"}})
	if err != nil {
		t.Fatal(err)
	}
	if excluded != 1 || len(got) != 1 || got[0].Ref.ID != 7 {
		t.Fatalf("unexpected filter: %+v excluded=%d", got, excluded)
	}
	got, _, err = Filter(works, Filters{Query: "keyword", Content: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("content query matches=%d", len(got))
	}
}
