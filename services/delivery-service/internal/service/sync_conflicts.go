package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

// recordedPlanVersion reads the plan version the driver app was showing. It is
// accepted at the operation level or inside the payload.
func recordedPlanVersion(op domain.SyncOperation) int {
	if op.PlanVersion > 0 {
		return op.PlanVersion
	}
	if v, ok := op.Payload["planVersion"].(float64); ok && v > 0 && v == float64(int(v)) {
		return int(v)
	}
	return 0
}

// isOlderPlan reports whether a record was made on a plan version older than
// the current one. Unknown versions are never treated as conflicts.
func isOlderPlan(recorded, current int) bool {
	return recorded > 0 && current > recorded
}

func conflictDetail(recorded, current int) string {
	return fmt.Sprintf("Recorded offline on plan v%d; the current plan is v%d. The record was kept and applied; nothing was overwritten. Dispatcher to settle.", recorded, current)
}

// flagOlderPlanVersion keeps an applied operation and, when it was recorded on
// an older plan version, stores a conflict for the dispatcher and tells the
// driver app about the newer version. It never changes the operation outcome.
func (s Service) flagOlderPlanVersion(ctx context.Context, op domain.SyncOperation, result map[string]any) map[string]any {
	if status, _ := result["status"].(string); status != domain.ResultApplied {
		return result
	}
	recorded := recordedPlanVersion(op)
	if recorded == 0 {
		return result
	}
	runID := op.RunID
	tripID := op.TripID
	current := 0
	if run, err := s.Repo.GetRun(ctx, runID); runID != "" && err == nil {
		current, tripID = run.PlanVersion, run.TripID
	} else if tripID != "" {
		if run, err := s.Repo.GetByTrip(ctx, tripID); err == nil {
			runID, current = run.ID, run.PlanVersion
		}
	}
	if tripID != "" {
		if trip, err := s.Peers.ReadyTrip(ctx, tripID); err == nil && trip.PlanVersion > current {
			current = trip.PlanVersion
		}
	}
	if runID == "" || !isOlderPlan(recorded, current) {
		return result
	}
	detail := conflictDetail(recorded, current)
	if err := s.Repo.InsertSyncConflict(ctx, domain.SyncConflict{RunID: runID, StopID: op.StopID, OperationID: op.OperationID, RecordedPlanVersion: recorded, CurrentPlanVersion: current, Detail: detail}); err != nil {
		// The record is already applied. Surface the failure so it is not silent.
		result["conflictRecordError"] = "could not list this record for the dispatcher; retry sync"
		return result
	}
	s.Peers.Publish(ctx, audit.ActionDeliverySyncConflict, "", "SYNC", op.OperationID, map[string]any{"detail": detail, "recordedPlanVersion": recorded, "currentPlanVersion": current})
	result["conflict"] = map[string]any{"recordedPlanVersion": recorded, "currentPlanVersion": current, "detail": detail}
	return result
}


func (s Service) ListSyncConflicts(ctx context.Context, profile *authorization.Profile, date string, openOnly bool) ([]domain.SyncConflict, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return nil, fmt.Errorf("forbidden: dispatcher access required")
	}
	return s.Repo.ListSyncConflicts(ctx, strings.TrimSpace(date), openOnly)
}

func (s Service) SettleSyncConflict(ctx context.Context, profile *authorization.Profile, id string) (domain.SyncConflict, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return domain.SyncConflict{}, fmt.Errorf("forbidden: dispatcher access required")
	}
	c, err := s.Repo.SettleSyncConflict(ctx, id, actor(profile))
	if err != nil {
		return c, err
	}
	s.Peers.Publish(ctx, audit.ActionDeliverySyncConflictSettled, actor(profile), "SYNC", c.OperationID, map[string]any{"conflictId": c.ID, "recordedPlanVersion": c.RecordedPlanVersion, "currentPlanVersion": c.CurrentPlanVersion})
	return c, nil
}
