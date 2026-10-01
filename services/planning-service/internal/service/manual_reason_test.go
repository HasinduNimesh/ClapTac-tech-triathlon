package service

import (
	"strings"
	"testing"
)

// FR-54: a manual allocation override (Assign/Reassign) must carry a real,
// human-written reason, not an empty or placeholder string.
func TestValidManualReasonRejectsMissingOrTrivialInput(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		ok     bool
	}{
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"too short", "ok", false},
		{"minimum length", "VIP", true},
		{"ordinary explanation", "Store manager escalated; move to earliest feasible van run.", true},
		{"padded with whitespace", "  Fits after breakdown reassignment freed capacity.  ", true},
		{"too long", strings.Repeat("a", 501), false},
		{"exactly at max", strings.Repeat("a", 500), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validManualReason(c.reason)
			if c.ok && err != nil {
				t.Fatalf("expected %q to be accepted, got error: %v", c.reason, err)
			}
			if !c.ok && err == nil {
				t.Fatalf("expected %q to be rejected", c.reason)
			}
		})
	}
}
