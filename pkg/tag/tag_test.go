package tag

import (
	"testing"
	"time"
)

func TestPlannerSplitsTagsAndMonths(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	p, err := NewPlanner(Options{Tags: []string{"a,b"}, Since: since, Until: until})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := p.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 6 {
		t.Fatalf("got %d windows want 6", len(ws))
	}
	if ws[0].Tag != "a" || ws[3].Tag != "b" {
		t.Fatalf("tags not split: %+v", ws)
	}
	if ws[0].PostYm != "202603" || ws[1].PostYm != "202602" || ws[2].PostYm != "202601" {
		t.Fatalf("unexpected months: %+v", ws[:3])
	}
	if ws[0].Kind != "month" {
		t.Fatalf("kind=%q", ws[0].Kind)
	}
	if ws[0].Key == ws[1].Key {
		t.Fatalf("keys should differ")
	}
}

func TestPlannerClampsSince(t *testing.T) {
	since := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	p, err := NewPlanner(Options{Tags: []string{"a"}, Since: since, Until: until})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := p.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 {
		t.Fatalf("got %d windows want 2: %+v", len(ws), ws)
	}
	if !ws[1].Start.Equal(since) {
		t.Fatalf("since not clamped: %v", ws[1].Start)
	}
}

func TestPlannerDefaultsAllTypes(t *testing.T) {
	p, err := NewPlanner(Options{Tags: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.opts.Types != "1,2,4" {
		t.Fatalf("types=%q", p.opts.Types)
	}
}
