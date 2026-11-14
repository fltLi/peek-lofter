package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fltLi/peek-lofter/internal/config"
	"github.com/fltLi/peek-lofter/pkg/lofter"
	"github.com/fltLi/peek-lofter/pkg/tag"
)

func TestHelp(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"--help"}, Deps{Out: &out, Err: &errb, IsTTY: false})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "peek-lofter") {
		t.Fatalf("unexpected help: %s", out.String())
	}
	for _, bad := range []string{"--plan", "--no-progress", "compact"} {
		if strings.Contains(out.String(), bad) {
			t.Fatalf("help still mentions %q: %s", bad, out.String())
		}
	}
	if !strings.Contains(out.String(), "list") {
		t.Fatalf("help does not mention list: %s", out.String())
	}
}

func TestMapTypes(t *testing.T) {
	cases := map[string]string{
		"":      "1,2,4",
		"all":   "1,2,4",
		"text":  "1",
		"image": "2",
		"video": "4",
	}
	for in, want := range cases {
		got, err := mapTypes(in)
		if err != nil {
			t.Fatalf("mapTypes(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("mapTypes(%q)=%q want %q", in, got, want)
		}
	}
	if _, err := mapTypes("bad"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestTagPlanRemoved(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute(
		context.Background(),
		[]string{"tag", "example-tag", "--plan"},
		Deps{Out: &out, Err: &errb, IsTTY: false},
	)
	if code != 2 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "--plan") {
		t.Fatalf("unexpected error: %s", errb.String())
	}
}

func TestQuietAndVerboseConflict(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute(
		context.Background(),
		[]string{"tag", "example-tag", "--quiet", "--verbose"},
		Deps{Out: &out, Err: &errb, IsTTY: false},
	)
	if code != 2 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "mutually exclusive") {
		t.Fatalf("unexpected error: %s", errb.String())
	}
}

func TestFormatTagSummary(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	covered := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	info := tagRunInfo{
		tags:          []string{"example-tag"},
		since:         since,
		until:         until,
		sinceExplicit: true,
		coveredStart:  covered,
		elapsed:       103 * time.Second,
	}
	sum := tag.Summary{Refs: 2, Suspect: 1}
	got := formatTagSummary(sum, info)
	for _, want := range []string{
		"tag example-tag",
		"requested  2026-11-01..2026-11-15",
		"completed  2026-11-08..2026-11-15",
		"posts      2",
		"status     partial",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"windows", "requests", "refs", "suspect"} {
		if strings.Contains(got, bad) {
			t.Fatalf("summary leaks %q:\n%s", bad, got)
		}
	}
}

func TestTagSummaryJSON(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	info := tagRunInfo{
		tags:          []string{"example-tag"},
		since:         since,
		until:         until,
		sinceExplicit: true,
		coveredStart:  since,
		elapsed:       time.Minute,
	}
	got := tagSummary(tag.Summary{Refs: 3}, info)
	if got.Status != "complete" || got.Posts != 3 || got.Since != "2026-11-01" || got.Until != "2026-11-15" {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestPostDryRunSummary(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute(
		context.Background(),
		[]string{
			"post",
			"https://example-user.lofter.com/post/abc_def",
			"https://example-user.lofter.com/post/abc_def",
			"--dry-run",
		},
		Deps{Out: &out, Err: &errb, IsTTY: false},
	)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid summary %q: %v", out.String(), err)
	}
	if got["refs"].(float64) != 1 {
		t.Fatalf("refs=%v", got["refs"])
	}
	if got["read"].(float64) != 2 {
		t.Fatalf("read=%v", got["read"])
	}
	if got["dropped"].(float64) != 1 {
		t.Fatalf("dropped=%v", got["dropped"])
	}
}

func TestPostDryRunJSONL(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute(
		context.Background(),
		[]string{
			"post",
			"https://example-user.lofter.com/post/abc_def",
			"--dry-run",
			"--format",
			"jsonl",
		},
		Deps{Out: &out, Err: &errb, IsTTY: false},
	)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines: %q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "\"url\":\"https://example-user.lofter.com/post/abc_def\"") {
		t.Fatalf("unexpected line: %s", lines[0])
	}
}

func TestPostDryRunQuiet(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute(
		context.Background(),
		[]string{
			"post",
			"https://example-user.lofter.com/post/abc_def",
			"--dry-run",
			"--quiet",
		},
		Deps{Out: &out, Err: &errb, IsTTY: false},
	)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "\"refs\":1") {
		t.Fatalf("quiet mode suppressed data: %q", out.String())
	}
}

func TestTagNonTTYSummary(t *testing.T) {
	requests := 0
	postTypes := "unset"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/newapi/tagPosts.json" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err == nil {
			postTypes = r.Form.Get("postTypes")
		}
		requests++
		if requests > 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "ok",
				"data": map[string]any{"offset": 0, "list": []any{}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{
				"offset": 0,
				"list": []any{
					map[string]any{
						"postData": map[string]any{
							"postView": map[string]any{
								"id":          123,
								"title":       "t",
								"type":        2,
								"permalink":   "abc_def",
								"publishTime": time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC).UnixMilli(),
							},
						},
						"blogInfo": map[string]any{
							"blogName": "example-user",
							"blogId":   456,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	code := Execute(
		context.Background(),
		[]string{"tag", "example-tag", "--since", "2026-11-01", "--until", "2026-11-02"},
		Deps{
			Out:   &out,
			Err:   &errb,
			IsTTY: false,
			Now:   func() time.Time { return time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC) },
			NewClient: func(config.Global) (*lofter.Client, error) {
				return lofter.New(lofter.Options{
					BaseURL: srv.URL,
					DWRURL:  srv.URL,
					Rate:    time.Millisecond,
					Timeout: time.Second,
				})
			},
		},
	)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if strings.Trim(postTypes, ",") != "1,2,4" {
		t.Fatalf("postTypes=%q, want all-types cold key", postTypes)
	}
	if strings.Contains(errb.String(), "window") || strings.Contains(errb.String(), "page") {
		t.Fatalf("progress leaked internals: %q", errb.String())
	}
	for _, want := range []string{
		"tag example-tag  requested 2026-11-01..2026-11-03",
		"posts      1",
		"status     complete",
		"completed  2026-11-01..2026-11-03",
	} {
		if !strings.Contains(errb.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, errb.String())
		}
	}
	if !strings.Contains(out.String(), "\"id\":123") {
		t.Fatalf("missing ref: %q", out.String())
	}
}

func TestTagEmptyMonthsValidation(t *testing.T) {
	var out, errb bytes.Buffer
	code := Execute(
		context.Background(),
		[]string{"tag", "example-tag", "--empty-months", "-1"},
		Deps{Out: &out, Err: &errb, IsTTY: false},
	)
	if code != 2 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "--empty-months") {
		t.Fatalf("unexpected error: %s", errb.String())
	}
}

func TestFormatTagSummaryStopped(t *testing.T) {
	info := tagRunInfo{
		tags:        []string{"example-tag"},
		until:       time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC),
		emptyMonths: 3,
	}
	got := formatTagSummary(tag.Summary{StoppedByEmpty: true}, info)
	if !strings.Contains(got, "no posts for 3 months") {
		t.Fatalf("unexpected summary:\n%s", got)
	}
}
