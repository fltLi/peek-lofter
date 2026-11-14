package lofter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
)

func TestTagPageMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/newapi/tagPosts.json" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("tag") != "example-tag" {
			t.Errorf("tag=%q", r.Form.Get("tag"))
		}
		if got := r.Form.Get("postTypes"); got != "1,2,4" {
			t.Errorf("postTypes=%q, want 1,2,4 for all types", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{
				"offset": 10,
				"list": []any{
					map[string]any{
						"postData": map[string]any{
							"postView": map[string]any{
								"id":          123,
								"blogId":      456,
								"title":       "t",
								"type":        2,
								"permalink":   "abc_def",
								"publishTime": 1714900572387,
								"tagList":     []string{"a"},
							},
							"postCount": map[string]any{
								"responseCount": 1,
								"favoriteCount": 2,
								"hotCount":      3,
							},
						},
						"blogInfo": map[string]any{
							"blogName":     "example-user",
							"blogNickName": "Example User",
							"blogId":       456,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL, Rate: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.TagPage(context.Background(), model.TagPageQuery{Tag: "example-tag", Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Refs) != 1 {
		t.Fatalf("got %d refs", len(page.Refs))
	}
	r := page.Refs[0]
	if r.ID != 123 || r.Author.BlogName != "example-user" || r.Permalink != "abc_def" {
		t.Fatalf("unexpected ref: %+v", r)
	}
	if r.URL != "https://example-user.lofter.com/post/abc_def" {
		t.Fatalf("unexpected url: %s", r.URL)
	}
	if page.Next != 10 {
		t.Fatalf("next=%d", page.Next)
	}
}
