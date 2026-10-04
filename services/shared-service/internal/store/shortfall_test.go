package store

import (
	"strings"
	"testing"
)

func TestShortfallBodyStatesUnitsAndDecisionInEveryLocale(t *testing.T) {
	for _, loc := range []string{"en", "si", "ta"} {
		for _, d := range []string{"PARTIAL_LOAD", "HOLD", "MOVE_TO_NEXT_RUN"} {
			b := ShortfallBody(loc, NotificationEvent{OrderRef: "ORD-7", Units: 12, Reason: d})
			if !strings.Contains(b, "ORD-7") || !strings.Contains(b, "12") {
				t.Fatalf("%s/%s: %q", loc, d, b)
			}
		}
	}
	if b := ShortfallBody("en", NotificationEvent{OrderRef: "O", Units: 3, Reason: "HOLD"}); !strings.Contains(b, "3 units short") || !strings.Contains(b, "on hold") {
		t.Fatal(b)
	}
}
