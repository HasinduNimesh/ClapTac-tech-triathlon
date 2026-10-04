package service

import (
	"context"
	"errors"
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

type fakeConflictRepo struct {
	run      domain.Run
	failures int // InsertSyncConflict fails this many times first
	rows     map[string]domain.SyncConflict
	attempts int
}

func (f *fakeConflictRepo) GetRun(context.Context, string) (domain.Run, error) { return f.run, nil }
func (f *fakeConflictRepo) GetByTrip(context.Context, string) (domain.Run, error) {
	return f.run, nil
}
func (f *fakeConflictRepo) InsertSyncConflict(_ context.Context, c domain.SyncConflict) (bool, error) {
	f.attempts++
	if f.failures > 0 {
		f.failures--
		return false, errors.New("database unavailable")
	}
	if f.rows == nil {
		f.rows = map[string]domain.SyncConflict{}
	}
	if _, ok := f.rows[c.OperationID]; ok {
		return false, nil
	}
	f.rows[c.OperationID] = c
	return true, nil
}

type fakeConflictPeers struct{ published int }

func (f *fakeConflictPeers) ReadyTrip(context.Context, string) (domain.LoadingTrip, error) {
	return domain.LoadingTrip{}, errors.New("no loading service in this test")
}
func (f *fakeConflictPeers) Publish(context.Context, string, string, string, string, map[string]any) {
	f.published++
}

func oldPlanOp() domain.SyncOperation {
	return domain.SyncOperation{OperationID: "op-old", Type: domain.OpStopOutcome, TripID: "trip-1", RunID: "run-1", StopID: "stop-1", PlanVersion: 3}
}

func TestConflictPersistenceFailureIsRetriableThenRecovers(t *testing.T) {
	ctx := context.Background()
	repo := &fakeConflictRepo{run: domain.Run{ID: "run-1", TripID: "trip-1", PlanVersion: 4}, failures: 1}
	peers := &fakeConflictPeers{}
	op := oldPlanOp()

	first := flagOlderPlan(ctx, repo, peers, op, map[string]any{"operationId": op.OperationID, "status": domain.ResultApplied})
	if first["status"] != domain.ResultRetry {
		t.Fatalf("a lost conflict must not look like success, got %v", first["status"])
	}
	if first["status"] == domain.ResultApplied || first["conflict"] != nil || len(repo.rows) != 0 || peers.published != 0 {
		t.Fatalf("nothing may be reported as stored yet: %v rows=%d published=%d", first, len(repo.rows), peers.published)
	}

	// The retry reaches the server as a DUPLICATE of the operation applied before.
	retry := flagOlderPlan(ctx, repo, peers, op, map[string]any{"operationId": op.OperationID, "status": domain.ResultDuplicate, "originalStatus": domain.ResultApplied})
	if retry["status"] != domain.ResultDuplicate || retry["originalStatus"] != domain.ResultApplied {
		t.Fatalf("recovered retry should be a duplicate of an applied op: %v", retry)
	}
	conflict, _ := retry["conflict"].(map[string]any)
	if conflict["recordedPlanVersion"] != 3 || conflict["currentPlanVersion"] != 4 {
		t.Fatalf("recovered result must carry the conflict: %v", retry)
	}
	if len(repo.rows) != 1 || peers.published != 1 {
		t.Fatalf("the missing conflict must be created once: rows=%d published=%d", len(repo.rows), peers.published)
	}

	// Another repeat keeps the info but neither adds a row nor a second audit event.
	again := flagOlderPlan(ctx, repo, peers, op, map[string]any{"operationId": op.OperationID, "status": domain.ResultDuplicate, "originalStatus": domain.ResultApplied})
	if again["conflict"] == nil || len(repo.rows) != 1 || peers.published != 1 {
		t.Fatalf("a repeat must not duplicate the conflict: %v rows=%d published=%d", again, len(repo.rows), peers.published)
	}
}

func TestConflictRetryStaysRetriableWhilePersistenceKeepsFailing(t *testing.T) {
	repo := &fakeConflictRepo{run: domain.Run{ID: "run-1", TripID: "trip-1", PlanVersion: 4}, failures: 2}
	op := oldPlanOp()
	dup := func() map[string]any {
		return map[string]any{"operationId": op.OperationID, "status": domain.ResultDuplicate, "originalStatus": domain.ResultApplied}
	}
	if out := flagOlderPlan(context.Background(), repo, &fakeConflictPeers{}, op, dup()); out["status"] != domain.ResultRetry {
		t.Fatalf("%v", out)
	}
	if out := flagOlderPlan(context.Background(), repo, &fakeConflictPeers{}, op, dup()); out["status"] != domain.ResultRetry {
		t.Fatalf("%v", out)
	}
	if out := flagOlderPlan(context.Background(), repo, &fakeConflictPeers{}, op, dup()); out["status"] != domain.ResultDuplicate || out["conflict"] == nil {
		t.Fatalf("%v", out)
	}
}

func TestDuplicateOfNonAppliedOperationCreatesNoConflict(t *testing.T) {
	repo := &fakeConflictRepo{run: domain.Run{ID: "run-1", TripID: "trip-1", PlanVersion: 4}}
	op := oldPlanOp()
	out := flagOlderPlan(context.Background(), repo, &fakeConflictPeers{}, op, map[string]any{"operationId": op.OperationID, "status": domain.ResultDuplicate, "originalStatus": domain.ResultRejected})
	if out["conflict"] != nil || repo.attempts != 0 {
		t.Fatalf("%v", out)
	}
}
