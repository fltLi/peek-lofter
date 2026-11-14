package store

import (
	"testing"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
)

func TestDatasetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.Configure(
		[]string{"example-tag"},
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
		"1,2,3,4,5,6",
	)
	if err := d.AppendRef(model.Ref{ID: 1, URL: "https://a"}); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendWindow(WindowEvent{Window: "w", Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	if !d.Completed("w") {
		t.Fatal("window should be completed")
	}
	if err := d.SaveManifest(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if !d2.Completed("w") {
		t.Fatal("completed not persisted")
	}
	count := 0
	if err := d2.ReadRefs(func(model.Ref) error { count++; return nil }); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d refs", count)
	}
	if got := d2.Manifest().Tags; len(got) != 1 || got[0] != "example-tag" {
		t.Fatalf("unexpected tags: %+v", got)
	}
}
