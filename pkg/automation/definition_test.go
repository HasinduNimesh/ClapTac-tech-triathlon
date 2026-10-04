package automation

import (
	"testing"
	"time"
)

func TestWeeklyScheduleAndVocabulary(t *testing.T) {
	d := Definition{Version: 1, Name: "Deferred", Weekday: 5, Time: "15:00", Timezone: "Asia/Colombo", Action: "notify_deferrals"}
	at := time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC)
	if d.Next(at) != at.AddDate(0, 0, 7) {
		t.Fatal("must not rerun the same slot")
	}
	if d.Previous(at.Add(time.Minute)) != at {
		t.Fatal("previous scheduled occurrence")
	}
	if err := d.Validate("STORE_MANAGER"); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"submit_order", "publish_plan", "http", "send_sms"} {
		d.Action = action
		if d.Validate("DISPATCHER") == nil {
			t.Fatalf("accepted %s", action)
		}
	}
	d.Action = "priority_review"
	if d.Validate("STORE_MANAGER") == nil {
		t.Fatal("store priority escalation")
	}
	d.Action = "notify_deferrals"
	d.Time = "25:00"
	if d.Validate("DISPATCHER") == nil {
		t.Fatal("invalid time")
	}
}
