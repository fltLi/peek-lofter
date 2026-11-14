package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTimestampRoundTrip(t *testing.T) {
	ts := FromUnixMilli(1714900572387)
	b, err := json.Marshal(ts)
	if err != nil {
		t.Fatal(err)
	}
	want := "\"2024-05-05T17:16:12.387+08:00\""
	if string(b) != want {
		t.Fatalf("got %s want %s", b, want)
	}
	var got Timestamp
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.UnixMilli() != ts.UnixMilli() {
		t.Fatalf("got %d want %d", got.UnixMilli(), ts.UnixMilli())
	}
}

func TestPostEmbedsRefFlat(t *testing.T) {
	p := Post{Ref: Ref{ID: 7, URL: "https://x", Title: "t"}, Tags: []string{"a"}}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "\"id\":7") || !strings.Contains(s, "\"tags\":[\"a\"]") {
		t.Fatalf("unexpected json: %s", s)
	}
	if strings.Contains(s, "\"Ref\"") {
		t.Fatalf("Ref should be flattened: %s", s)
	}
}
