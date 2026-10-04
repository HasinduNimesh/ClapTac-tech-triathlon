package handler

import "testing"

func TestWorkflowTemplateDoesNotDropConstraints(t *testing.T) {
	d, ok := parseWeekly("Every Friday at 3 PM, if any of my orders are still deferred, send me a list.")
	if !ok || d.Weekday != 5 || d.Time != "15:00" {
		t.Fatalf("%+v %v", d, ok)
	}
	for _, text := range []string{"Every Friday at 3, if any of my orders are still deferred, send me a list.", "Every Friday at 3 PM, if any of my Fresh orders are still deferred, send me a list.", "Every Friday at 3 PM, if any of my orders are still deferred, send me a list and submit them.", "Every Friday at 13 PM, if any of my orders are still deferred, send me a list."} {
		if _, ok = parseWeekly(text); ok {
			t.Fatalf("unsafe interpretation: %s", text)
		}
	}
}
