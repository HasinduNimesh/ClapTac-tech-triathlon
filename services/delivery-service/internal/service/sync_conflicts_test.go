package service

import (
	"strings"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func TestIsOlderPlan(t *testing.T) {
	for _, tc := range []struct {
		recorded, current int
		want              bool
	}{{3, 4, true}, {4, 4, false}, {5, 4, false}, {0, 4, false}} {
		if got := isOlderPlan(tc.recorded, tc.current); got != tc.want {
			t.Fatalf("isOlderPlan(%d,%d)=%v want %v", tc.recorded, tc.current, got, tc.want)
		}
	}
}

func TestRecordedPlanVersionFromFieldOrPayload(t *testing.T) {
	if v := recordedPlanVersion(domain.SyncOperation{PlanVersion: 3}); v != 3 {
		t.Fatal(v)
	}
	if v := recordedPlanVersion(domain.SyncOperation{Payload: map[string]any{"planVersion": float64(2)}}); v != 2 {
		t.Fatal(v)
	}
	if v := recordedPlanVersion(domain.SyncOperation{Payload: map[string]any{"planVersion": 1.5}}); v != 0 {
		t.Fatal(v)
	}
}

func TestFlagOlderPlanVersionLeavesRejectedAndUnversionedAlone(t *testing.T) {
	s := Service{}
	rejected := map[string]any{"status": domain.ResultRejected}
	if out := s.flagOlderPlanVersion(nil, domain.SyncOperation{PlanVersion: 1}, rejected); out["conflict"] != nil {
		t.Fatal("rejected op must not create a conflict")
	}
	applied := map[string]any{"status": domain.ResultApplied}
	if out := s.flagOlderPlanVersion(nil, domain.SyncOperation{}, applied); out["conflict"] != nil {
		t.Fatal("unversioned op must not create a conflict")
	}
}

func TestConflictDetailSaysNothingWasOverwritten(t *testing.T) {
	d := conflictDetail(3, 4)
	if !strings.Contains(d, "v3") || !strings.Contains(d, "v4") || !strings.Contains(d, "nothing was overwritten") {
		t.Fatal(d)
	}
}
