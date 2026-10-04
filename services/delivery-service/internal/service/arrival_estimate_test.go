package service

import (
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

// These cases mirror apps/web/tests/arrivalEstimate.test.mjs one for one: the backend must project the
// same arrival the dispatcher page shows.

func at(t *testing.T, v string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t.Fatal(err)
	}
	return &parsed
}

func intp(v int) *int { return &v }

func TestPreviousReportedStopUsesLatestArrivedBeforeEarlierCompleted(t *testing.T) {
	completed := domain.Stop{OutcomeAt: at(t, "2026-09-30T08:30:00+05:30")}
	active := domain.Stop{ArrivedAt: at(t, "2026-09-30T09:00:00+05:30")}
	stops := []domain.Stop{completed, active, {}}
	if got := previousReportedStop(stops, 2); got != &stops[1] {
		t.Fatalf("expected the arrived stop, got %+v", got)
	}
	if got := previousReportedStop([]domain.Stop{completed, {}}, 1); got == nil || got.OutcomeAt == nil {
		t.Fatalf("expected the completed stop, got %+v", got)
	}
	if got := previousReportedStop([]domain.Stop{completed}, 0); got != nil {
		t.Fatalf("expected no previous stop, got %+v", got)
	}
}

func TestEstimateProjectsObservedDelayOfLastStop(t *testing.T) {
	got := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T08:30:00+05:30"), PlannedDepartureAt: at(t, "2026-09-30T08:15:00+05:30"),
		PreviousOutcomeAt: at(t, "2026-09-30T08:25:00+05:30"), WindowCloseAt: "2026-09-30T09:00:00Z", DeliveryDate: "2026-09-30",
		Now: *at(t, "2026-09-30T08:26:00+05:30"),
	})
	if got.ETA.Format(time.RFC3339) != "2026-09-30T03:10:00Z" || got.DelayMinutes != 10 || got.Risk != "On track" || got.Version != ArrivalEstimateVersion || got.Kind != "estimate" {
		t.Fatalf("%+v", got)
	}
}

func TestEstimateUsesServiceAllowanceForAnUnfinishedArrivedStop(t *testing.T) {
	got := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T09:30:00+05:30"), PlannedDepartureAt: at(t, "2026-09-30T09:15:00+05:30"),
		PreviousArrivedAt: at(t, "2026-09-30T09:00:00+05:30"), ServiceMinutesPerStop: intp(30),
		WindowCloseAt: "10:00", DeliveryDate: "2026-09-30", Now: *at(t, "2026-09-30T09:20:00+05:30"),
	})
	if got.ETA.Format(time.RFC3339) != "2026-09-30T04:15:00Z" || got.DelayMinutes != 15 || got.Risk != "Watch window" || got.Version != "event_delay_propagation_v2" {
		t.Fatalf("%+v", got)
	}
}

func TestEstimateDefaultsToTwentyMinuteAllowance(t *testing.T) {
	got := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T09:30:00+05:30"), PlannedDepartureAt: at(t, "2026-09-30T09:15:00+05:30"),
		PreviousArrivedAt: at(t, "2026-09-30T09:00:00+05:30"), Now: *at(t, "2026-09-30T09:20:00+05:30"),
	})
	if got.DelayMinutes != 5 {
		t.Fatalf("09:00 arrival + 20 min against a 09:15 planned departure is 5 minutes late, got %+v", got)
	}
	negative := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T09:30:00+05:30"), PlannedDepartureAt: at(t, "2026-09-30T09:15:00+05:30"),
		PreviousArrivedAt: at(t, "2026-09-30T09:00:00+05:30"), ServiceMinutesPerStop: intp(-5), Now: *at(t, "2026-09-30T09:20:00+05:30"),
	})
	if negative.DelayMinutes != 5 {
		t.Fatalf("a negative allowance falls back to the default: %+v", negative)
	}
}

func TestCompletedOutcomeTimeBeatsArrivalPlusService(t *testing.T) {
	got := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T09:30:00+05:30"), PlannedDepartureAt: at(t, "2026-09-30T09:15:00+05:30"),
		PreviousArrivedAt: at(t, "2026-09-30T09:00:00+05:30"), PreviousOutcomeAt: at(t, "2026-09-30T09:12:00+05:30"),
		ServiceMinutesPerStop: intp(40), Now: *at(t, "2026-09-30T09:20:00+05:30"),
	})
	if got.ETA.Format(time.RFC3339) != "2026-09-30T04:00:00Z" || got.DelayMinutes != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestEarlyFinishNeverPullsTheEstimateEarlier(t *testing.T) {
	got := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T09:30:00+05:30"), PlannedDepartureAt: at(t, "2026-09-30T09:15:00+05:30"),
		PreviousOutcomeAt: at(t, "2026-09-30T08:50:00+05:30"), Now: *at(t, "2026-09-30T09:00:00+05:30"),
	})
	if got.DelayMinutes != 0 || got.ETA.Format(time.RFC3339) != "2026-09-30T04:00:00Z" {
		t.Fatalf("%+v", got)
	}
}

func TestEstimateFlagsWindowBreachAndHandlesMissingSchedule(t *testing.T) {
	atRisk := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T08:30:00+05:30"), PlannedDepartureAt: at(t, "2026-09-30T08:15:00+05:30"),
		PreviousOutcomeAt: at(t, "2026-09-30T08:55:00+05:30"), WindowCloseAt: "09:00", DeliveryDate: "2026-09-30",
		Now: *at(t, "2026-09-30T08:56:00+05:30"),
	})
	if atRisk.Risk != "Window at risk" || atRisk.Kind != "risk" {
		t.Fatalf("%+v", atRisk)
	}
	if got := EstimateArrival(ArrivalEstimateInput{}); got.Kind != "unknown" {
		t.Fatalf("%+v", got)
	}
}

func TestClosedWindowIsMissedAndBeatsStaleEarlierEstimate(t *testing.T) {
	missed := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T08:30:00+05:30"), WindowCloseAt: "08:00", DeliveryDate: "2026-09-30",
		Now: *at(t, "2026-09-30T08:10:00+05:30"),
	})
	if missed.Risk != "Window missed" {
		t.Fatalf("%+v", missed)
	}
	stale := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T08:30:00+05:30"), WindowCloseAt: "09:00", DeliveryDate: "2026-09-30",
		Now: *at(t, "2026-09-30T09:20:00+05:30"),
	})
	if stale.Risk != "Window missed" {
		t.Fatalf("%+v", stale)
	}
}

func TestElapsedEtaIsLateEvenWhenWindowIsNear(t *testing.T) {
	got := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T09:50:00+05:30"), WindowCloseAt: "10:02", DeliveryDate: "2026-09-30",
		Now: *at(t, "2026-09-30T10:00:00+05:30"),
	})
	if got.Risk != "ETA passed" || got.Kind != "late" {
		t.Fatalf("%+v", got)
	}
}

func TestOpenStopRiskAdvancesAsTheClockCrossesTheCloseTime(t *testing.T) {
	input := ArrivalEstimateInput{PlannedArrivalAt: at(t, "2026-09-30T09:50:00+05:30"), WindowCloseAt: "10:00", DeliveryDate: "2026-09-30"}
	input.Now = *at(t, "2026-09-30T09:49:00+05:30")
	if got := EstimateArrival(input); got.Risk != "Watch window" {
		t.Fatalf("%+v", got)
	}
	input.Now = *at(t, "2026-09-30T10:01:00+05:30")
	if got := EstimateArrival(input); got.Risk != "Window missed" {
		t.Fatalf("%+v", got)
	}
	input.WindowCloseAt = "10:00:00" // the stored form carries seconds
	if got := EstimateArrival(input); got.Risk != "Window missed" {
		t.Fatalf("%+v", got)
	}
}

func TestDelayMinutesRoundHalvesUp(t *testing.T) {
	got := EstimateArrival(ArrivalEstimateInput{
		PlannedArrivalAt: at(t, "2026-09-30T09:50:00+05:30"), PlannedDepartureAt: at(t, "2026-09-30T09:00:00+05:30"),
		PreviousOutcomeAt: at(t, "2026-09-30T09:10:30+05:30"), Now: *at(t, "2026-09-30T09:20:00+05:30"),
	})
	if got.DelayMinutes != 11 {
		t.Fatalf("10m30s rounds to 11 like Math.round: %+v", got)
	}
}
