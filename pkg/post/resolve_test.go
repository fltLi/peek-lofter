package post

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fltLi/peek-lofter/pkg/model"
)

func TestResolver(t *testing.T) {
	r := Resolver{Blog: "example-user"}
	ref, err := r.Resolve("https://example-user.lofter.com/post/abc_def")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Permalink != "abc_def" || ref.Author.BlogName != "example-user" {
		t.Fatalf("unexpected url ref: %+v", ref)
	}
	ref, err = r.Resolve("abc_def")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Permalink != "abc_def" || ref.Author.BlogName != "example-user" {
		t.Fatalf("unexpected permalink ref: %+v", ref)
	}
	if ref.URL != "https://example-user.lofter.com/post/abc_def" {
		t.Fatalf("unexpected url: %s", ref.URL)
	}
	ref, err = r.Resolve("123")
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != 123 || ref.Author.BlogName != "example-user" {
		t.Fatalf("unexpected id ref: %+v", ref)
	}
}

func TestArchiveNames(t *testing.T) {
	if got := sanitizeName("a/b:c"); got != "a_b_c" {
		t.Fatalf("sanitizeName=%q", got)
	}
	p := model.Post{
		Ref: model.Ref{
			ID:     7,
			Title:  "hello",
			Author: model.Author{BlogID: 1, BlogName: "blog"},
		},
	}
	got, err := ArchivePath("out", p)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.ToSlash(got) != "out/1/7.zip" {
		t.Fatalf("unexpected archive path: %s", got)
	}
	p.Author.BlogID = 0
	if _, err := ArchivePath("out", p); err == nil {
		t.Fatal("expected missing author id error")
	}
}

func TestMarkdown(t *testing.T) {
	md := ToMarkdown("<p>hi <strong>x</strong></p>")
	if !strings.Contains(md, "hi") || !strings.Contains(md, "**x**") {
		t.Fatalf("unexpected markdown: %q", md)
	}
	urls := ExtractImageURLs("<p><img src=\"https://a/b.jpg\"/></p>")
	if len(urls) != 1 || urls[0] != "https://a/b.jpg" {
		t.Fatalf("unexpected urls: %+v", urls)
	}
}
