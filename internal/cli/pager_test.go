package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestWritePagesExplicitPage(t *testing.T) {
	var out bytes.Buffer
	rows := []string{"a", "b", "c", "d", "e"}
	err := writePages(rows, pageConfig{
		Page:     2,
		PageSize: 2,
		PageSet:  true,
		TTY:      false,
		Out:      &out,
		Header:   "HEAD",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"page 2/3", "HEAD", "c", "d"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "\ne\n") {
		t.Fatalf("page leaked next row: %q", got)
	}
}

func TestWritePagesNoPager(t *testing.T) {
	var out bytes.Buffer
	rows := []string{"a", "b", "c"}
	err := writePages(rows, pageConfig{PageSize: 1, NoPager: true, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "a\nb\nc\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestWritePagesInteractive(t *testing.T) {
	var out, prompt bytes.Buffer
	rows := []string{"a", "b", "c"}
	err := writePages(rows, pageConfig{
		PageSize: 1,
		TTY:      true,
		In:       strings.NewReader("n\nq\n"),
		Out:      &out,
		Prompt:   &prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "page 1/3") || !strings.Contains(out.String(), "page 2/3") {
		t.Fatalf("missing pages: %q", out.String())
	}
	if !strings.Contains(prompt.String(), "[n]ext") {
		t.Fatalf("missing prompt: %q", prompt.String())
	}
}

func TestWritePagesOutOfRange(t *testing.T) {
	var out bytes.Buffer
	err := writePages([]string{"a"}, pageConfig{Page: 2, PageSize: 1, PageSet: true, Out: &out})
	if err == nil {
		t.Fatal("expected an error")
	}
}
