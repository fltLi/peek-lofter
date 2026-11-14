package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/pkg/lofter"
)

func TestPostCreatesArchiveAndSkipsExisting(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/newweb/post/detail.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"blogInfo": map[string]any{"blogId": 1, "blogName": "example-user", "blogNickName": "Example"},
					"postData": map[string]any{
						"postView": map[string]any{
							"id": 7, "blogId": 1, "title": "Example title", "type": 1,
							"publishTime": time.Date(2024, 5, 5, 12, 36, 12, 0, time.UTC).UnixMilli(),
							"permalink":   "abc_def", "tagList": []string{"example-tag"},
						},
						"postCountView": map[string]any{},
					},
				},
			})
		case "/oldapi/post/detail.api":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"meta": map[string]any{"status": 200},
				"response": map[string]any{
					"posts": []any{map[string]any{"post": map[string]any{
						"id": 7, "blogId": 1, "title": "Example title", "content": "<p>hello</p>",
						"permalink": "abc_def", "type": 1,
						"publishTime": time.Date(2024, 5, 5, 12, 36, 12, 0, time.UTC).UnixMilli(),
						"tagList":     []string{"example-tag"}, "photoLinks": "[]",
					}}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	out := t.TempDir()
	deps := Deps{
		Out:   &bytes.Buffer{},
		Err:   &bytes.Buffer{},
		IsTTY: false,
		NewClient: func(config.Global) (*lofter.Client, error) {
			return lofter.New(lofter.Options{BaseURL: srv.URL, DWRURL: srv.URL, Rate: time.Millisecond, Timeout: time.Second})
		},
	}
	args := []string{"post", "https://example-user.lofter.com/post/abc_def", "--out", out}
	if code := Execute(context.Background(), args, deps); code != 0 {
		t.Fatalf("first code=%d err=%s", code, deps.Err.(*bytes.Buffer).String())
	}
	path := filepath.Join(out, "1", "7.zip")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing archive: %v", err)
	}
	firstRequests := requests
	if firstRequests == 0 {
		t.Fatal("expected network requests")
	}
	if code := Execute(context.Background(), args, deps); code != 0 {
		t.Fatalf("second code=%d err=%s", code, deps.Err.(*bytes.Buffer).String())
	}
	if requests != firstRequests {
		t.Fatalf("second run made %d extra requests", requests-firstRequests)
	}
}
