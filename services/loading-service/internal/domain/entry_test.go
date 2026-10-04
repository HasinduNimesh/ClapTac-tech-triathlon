package domain

import (
	"strings"
	"testing"
)

func TestNormalizeEntryLegacyClientSendsNothing(t *testing.T) {
	e, err := NormalizeEntry("", "", "")
	if err != nil || e != (LoadEntry{}) {
		t.Fatalf("legacy entry = %+v, %v; want zero value and no error", e, err)
	}
}

func TestNormalizeEntryScan(t *testing.T) {
	e, err := NormalizeEntry(" scan ", "", "")
	if err != nil || e.Method != EntryScan || e.Manual() {
		t.Fatalf("scan entry = %+v, %v", e, err)
	}
	if _, err := NormalizeEntry("SCAN", ReasonOther, ""); err == nil {
		t.Fatal("a reason on a scanned entry must be refused")
	}
	if _, err := NormalizeEntry("SCAN", "", "typed"); err == nil {
		t.Fatal("a note on a scanned entry must be refused")
	}
}

func TestNormalizeEntryManualRequiresReason(t *testing.T) {
	for _, reason := range []string{"", "  ", "BORED", "TORN"} {
		_, err := NormalizeEntry("MANUAL", reason, "")
		if err == nil || !strings.HasPrefix(err.Error(), "invalid") {
			t.Fatalf("MANUAL with reason %q: err = %v, want an invalid error", reason, err)
		}
	}
}

func TestNormalizeEntryManualAcceptsEveryAllowedReason(t *testing.T) {
	for _, reason := range []string{ReasonDamagedLabel, ReasonUnreadable, ReasonNoCamera, ReasonOther} {
		e, err := NormalizeEntry("manual", strings.ToLower(reason), "  smudged  ")
		if err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
		if !e.Manual() || e.ReasonCode != reason || e.Note != "smudged" {
			t.Fatalf("%s: entry = %+v", reason, e)
		}
	}
}

func TestNormalizeEntryNoteLength(t *testing.T) {
	if _, err := NormalizeEntry("MANUAL", ReasonOther, strings.Repeat("x", MaxManualNoteLength)); err != nil {
		t.Fatalf("a note at the limit is allowed: %v", err)
	}
	if _, err := NormalizeEntry("MANUAL", ReasonOther, strings.Repeat("x", MaxManualNoteLength+1)); err == nil {
		t.Fatal("a note over the limit must be refused")
	}
}

func TestNormalizeEntryReasonWithoutMethodAndUnknownMethod(t *testing.T) {
	if _, err := NormalizeEntry("", ReasonDamagedLabel, ""); err == nil {
		t.Fatal("a reason without a method must be refused")
	}
	if _, err := NormalizeEntry("VOICE", "", ""); err == nil {
		t.Fatal("an unknown method must be refused")
	}
}
