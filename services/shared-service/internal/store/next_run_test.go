package store

import "testing"

func TestNextRunText(t *testing.T) {
	if got := nextRunText("2026-10-07"); got != "2026-10-07" {
		t.Fatalf("got %q", got)
	}
	if got := nextRunText(""); got != "to be confirmed" {
		t.Fatalf("got %q", got)
	}
}
