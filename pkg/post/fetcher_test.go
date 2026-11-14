package post

import (
	"context"
	"testing"

	"github.com/fltLi/peek-lofter/pkg/lofter"
	"github.com/fltLi/peek-lofter/pkg/model"
)

type fakeSource struct {
	calls int
	post  model.Post
}

func (f *fakeSource) PostDetail(context.Context, model.Ref, lofter.DetailMode) (model.Post, error) {
	f.calls++
	return f.post, nil
}

func TestFetchAllSkipsExistingWithoutRequest(t *testing.T) {
	p := testPost()
	p.Content = "<p>hello</p>"
	out := t.TempDir()
	ref := model.Ref{ID: p.ID, URL: p.URL, Author: model.Author{BlogID: 1, BlogName: "example-user"}}
	src := &fakeSource{post: p}
	fetcher := &Fetcher{Source: src}

	sum, err := fetcher.FetchAll(context.Background(), []model.Ref{ref}, Options{Out: out, Concurrency: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Saved != 1 || src.calls != 1 {
		t.Fatalf("first run: sum=%+v calls=%d", sum, src.calls)
	}

	sum, err = fetcher.FetchAll(context.Background(), []model.Ref{ref}, Options{Out: out, Concurrency: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Skipped != 1 || src.calls != 1 {
		t.Fatalf("second run should skip: sum=%+v calls=%d", sum, src.calls)
	}

	sum, err = fetcher.FetchAll(context.Background(), []model.Ref{ref}, Options{Out: out, Concurrency: 1, Force: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Saved != 1 || src.calls != 2 {
		t.Fatalf("force should refetch: sum=%+v calls=%d", sum, src.calls)
	}
}

func TestFetchAllFindsArchiveByURL(t *testing.T) {
	p := testPost()
	out := t.TempDir()
	src := &fakeSource{post: p}
	fetcher := &Fetcher{Source: src}
	ref := model.Ref{ID: p.ID, URL: p.URL, Author: model.Author{BlogID: 1, BlogName: "example-user"}}
	if _, err := fetcher.FetchAll(context.Background(), []model.Ref{ref}, Options{Out: out, Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	urlOnly := model.Ref{URL: p.URL, Author: model.Author{BlogName: "example-user"}}
	sum, err := fetcher.FetchAll(context.Background(), []model.Ref{urlOnly}, Options{Out: out, Concurrency: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Skipped != 1 || src.calls != 1 {
		t.Fatalf("url lookup should skip: sum=%+v calls=%d", sum, src.calls)
	}
}
