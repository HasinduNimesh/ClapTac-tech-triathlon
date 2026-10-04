package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/objectstore"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/validation"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/store"
)

type IncompleteError struct{ Pending []string }

func (e IncompleteError) Error() string { return "conflict: delivery_incomplete" }

type Service struct {
	Repo    store.Postgres
	Peers   client.Peers
	Objects objectstore.Store
	// ServiceMinutesPerStop is the per-stop service allowance used to project arrival times after a
	// driver event (see arrival_updates.go). Zero means the 20-minute default.
	ServiceMinutesPerStop int
}

func (s Service) List(ctx context.Context, profile *authorization.Profile, date, vehicleID string) ([]map[string]any, error) {
	if date == "" {
		return nil, fmt.Errorf("invalid: date required")
	}
	filterVehicle := vehicleID
	if !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		if profile.VehicleID == "" {
			return nil, fmt.Errorf("forbidden: driver vehicle required")
		}
		filterVehicle = profile.VehicleID
	}
	ready, err := s.Peers.ReadyTrips(ctx, date, filterVehicle)
	if err != nil {
		return nil, err
	}
	runs, err := s.Repo.ListRuns(ctx, date, filterVehicle)
	if err != nil {
		return nil, err
	}
	byTrip := map[string]domain.Run{}
	for _, r := range runs {
		byTrip[r.TripID] = r
	}
	seen := map[string]bool{}
	var out []map[string]any
	for _, t := range ready {
		seen[t.TripID] = true
		item := map[string]any{
			"tripId": t.TripID, "planId": t.PlanID, "planRef": t.PlanRef, "deliveryDate": t.DeliveryDate,
			"vehicleId": t.VehicleID, "depot": t.Depot, "tripNumber": t.TripNumber,
			"vehicleType": t.VehicleType, "vehicleTemperatureCapability": t.VehicleTemperatureCapability,
			"loadingStatus": t.LoadingStatus, "stopCount": len(t.Orders), "status": "ready",
		}
		if run, ok := byTrip[t.TripID]; ok {
			item["status"] = run.Status
			item["runId"] = run.ID
			stops, _ := s.Repo.ListStops(ctx, run.ID)
			item["stopCount"] = len(stops)
			item["completedStops"] = completedCount(stops)
		}
		out = append(out, item)
	}
	for _, run := range runs {
		if seen[run.TripID] {
			continue
		}
		stops, _ := s.Repo.ListStops(ctx, run.ID)
		out = append(out, map[string]any{
			"tripId": run.TripID, "planId": run.PlanID, "planRef": run.PlanRef, "deliveryDate": run.DeliveryDate,
			"vehicleId": run.VehicleID, "depot": run.Depot, "tripNumber": run.TripNumber,
			"vehicleType": run.VehicleType, "vehicleTemperatureCapability": run.VehicleTemperatureCapability,
			"status": run.Status, "runId": run.ID, "stopCount": len(stops), "completedStops": completedCount(stops),
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, nil
}

func (s Service) LatenessHistory(ctx context.Context, profile *authorization.Profile, tripID string) ([]domain.LatenessProbability, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return nil, fmt.Errorf("forbidden: dispatcher permission required")
	}
	if strings.TrimSpace(tripID) == "" {
		return nil, fmt.Errorf("invalid: tripId required")
	}
	if _, err := s.Repo.GetByTrip(ctx, tripID); err != nil {
		return nil, fmt.Errorf("not found")
	}
	return s.Repo.LatenessHistory(ctx, tripID)
}

func (s Service) SendTripMessage(ctx context.Context, profile *authorization.Profile, tripID, stopID, body string) (domain.TripMessage, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return domain.TripMessage{}, fmt.Errorf("forbidden: dispatcher required")
	}
	body = strings.TrimSpace(body)
	if tripID == "" || body == "" || len([]rune(body)) > 1000 {
		return domain.TripMessage{}, fmt.Errorf("invalid: message must contain 1 to 1000 characters")
	}
	return s.Repo.SendTripMessage(ctx, tripID, stopID, body, profile.UserID)
}

func (s Service) TripMessages(ctx context.Context, profile *authorization.Profile, tripID string) ([]domain.TripMessage, error) {
	if profile == nil || (!authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) && !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAssigned)) {
		return nil, fmt.Errorf("forbidden: trip message access denied")
	}
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) && profile.VehicleID != run.VehicleID {
		return nil, fmt.Errorf("forbidden: trip is assigned to another vehicle")
	}
	return s.Repo.ListTripMessages(ctx, tripID)
}

func (s Service) AcknowledgeTripMessage(ctx context.Context, profile *authorization.Profile, tripID, messageID string) (domain.TripMessage, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryUpdate) {
		return domain.TripMessage{}, fmt.Errorf("forbidden: driver update permission required")
	}
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return domain.TripMessage{}, err
	}
	if profile.VehicleID == "" || profile.VehicleID != run.VehicleID {
		return domain.TripMessage{}, fmt.Errorf("forbidden: trip is assigned to another vehicle")
	}
	return s.Repo.AcknowledgeTripMessage(ctx, tripID, messageID, profile.UserID)
}

func (s Service) Get(ctx context.Context, profile *authorization.Profile, tripID string) (map[string]any, error) {
	if run, err := s.Repo.GetByTrip(ctx, tripID); err == nil {
		if err := s.guardVehicle(profile, run.VehicleID); err != nil {
			return nil, err
		}
		return s.detail(ctx, run)
	}
	if authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) &&
		!authorization.HasPermission(profile.Roles, authorization.PermDeliveryStart) {
		trip, err := s.Peers.ReadyTrip(ctx, tripID)
		if err != nil {
			return nil, fmt.Errorf("not found")
		}
		return loadingPreview(trip), nil
	}
	return s.Prepare(ctx, profile, tripID)
}

func (s Service) InternalOrder(ctx context.Context, orderID string) (domain.OrderTracking, error) {
	if strings.TrimSpace(orderID) == "" {
		return domain.OrderTracking{}, fmt.Errorf("invalid: orderId")
	}
	return s.Repo.OrderTracking(ctx, orderID)
}

func (s Service) OutletLastServed(ctx context.Context) ([]domain.OutletLastServed, error) {
	return s.Repo.OutletLastServed(ctx)
}

func (s Service) OutletLastAttempted(ctx context.Context, beforeDate string) ([]domain.OutletLastAttempted, error) {
	return s.Repo.OutletLastAttempted(ctx, beforeDate)
}

func (s Service) Prepare(ctx context.Context, profile *authorization.Profile, tripID string) (map[string]any, error) {
	if existing, err := s.Repo.GetByTrip(ctx, tripID); err == nil {
		if err := s.guardVehicle(profile, existing.VehicleID); err != nil {
			return nil, err
		}
		return s.detail(ctx, existing)
	}
	trip, err := s.Peers.ReadyTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	if !strings.EqualFold(trip.LoadingStatus, "ready") {
		return nil, fmt.Errorf("conflict: trip is not ready for departure")
	}
	if err := s.guardVehicle(profile, trip.VehicleID); err != nil {
		return nil, err
	}
	var stops []domain.Stop
	for _, o := range trip.Orders {
		st := domain.Stop{
			AllocationID: o.AllocationID, OrderID: o.OrderID, OrderRef: o.OrderRef, OutletID: o.OutletID,
			UnitLabel: "units",
			Brand:     o.Brand, StopSequence: o.StopSequence, TemperatureRequirement: o.TemperatureRequirement,
			LoadingStatus: o.LoadingStatus, LoadingShortfallSummary: o.ShortfallSummary, PlannedArrivalAt: o.PlannedArrivalAt,
		}
		if o.ExpectedUnits > 0 {
			units := o.ExpectedUnits
			st.ExpectedUnits = &units
		}
		if o.OutletID != "" {
			if out, e := s.Peers.Outlet(ctx, o.OutletID); e == nil {
				st.OutletName = out.Name
				st.District = out.District
				st.DockType = out.DockType
				st.ParkingConstraint = out.ParkingConstraint
				st.AccessInstructions = out.AccessInstructions
				st.AccessInstructionsUpdatedAt = out.AccessInstructionsUpdatedAt
				st.ChilledTemperatureMinC = out.ChilledTemperatureMinC
				st.ChilledTemperatureMaxC = out.ChilledTemperatureMaxC
				st.PlannedWindowOpen = out.WindowOpenTime
				st.PlannedWindowClose = out.WindowCloseTime
			}
		}
		if st.LoadingShortfallSummary == nil {
			st.LoadingShortfallSummary = []any{}
		}
		stops = append(stops, st)
	}
	run, err := s.Repo.PrepareTx(ctx, domain.Run{
		TripID: trip.TripID, PlanID: trip.PlanID, PlanRef: trip.PlanRef, DeliveryDate: trip.DeliveryDate,
		VehicleID: trip.VehicleID, Depot: trip.Depot, TripNumber: trip.TripNumber,
		VehicleType: trip.VehicleType, VehicleTemperatureCapability: trip.VehicleTemperatureCapability,
		PlanVersion: trip.PlanVersion,
	}, stops)
	if err != nil {
		if existing, e2 := s.Repo.GetByTrip(ctx, tripID); e2 == nil {
			return s.detail(ctx, existing)
		}
		return nil, err
	}
	s.Peers.Publish(ctx, audit.ActionDeliveryRunPrepared, actor(profile), "TRIP", tripID, map[string]any{"planRef": trip.PlanRef})
	return s.detail(ctx, run)
}

func (s Service) Start(ctx context.Context, profile *authorization.Profile, tripID, opID string) (map[string]any, error) {
	if opID == "" {
		return nil, fmt.Errorf("invalid: Idempotency-Key required")
	}
	if existing, err := s.Repo.GetOp(ctx, opID); err == nil {
		if existing.OperationType != domain.OpStart || existing.ResultStatus != domain.ResultApplied {
			return nil, fmt.Errorf("conflict: idempotency key was used for another operation")
		}
		run, err := s.Repo.GetByTrip(ctx, tripID)
		if err != nil {
			return nil, fmt.Errorf("not found")
		}
		if existing.RunID != run.ID {
			return nil, fmt.Errorf("conflict: idempotency key belongs to another trip")
		}
		if err := s.guardVehicle(profile, run.VehicleID); err != nil {
			return nil, err
		}
		return s.detail(ctx, run)
	}
	detail, err := s.Prepare(ctx, profile, tripID)
	if err != nil {
		return nil, err
	}
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if run.Status == domain.RunInProgress {
		_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, OperationType: domain.OpStart, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"status": domain.RunInProgress}})
		return detail, nil
	}
	if run.Status == domain.RunCompleted {
		return nil, fmt.Errorf("conflict: run already completed")
	}
	latest, err := s.Peers.ReadyTrip(ctx, tripID)
	if err != nil || latest.PlanVersion != run.PlanVersion {
		return nil, fmt.Errorf("conflict: plan version changed; reload and acknowledge current instructions")
	}
	acknowledged := false
	if profile != nil {
		for _, ack := range latest.PlanAcknowledgements {
			if ack.ActorID == profile.UserID && ack.ActorRole == authorization.RoleDriver {
				acknowledged = true
			}
		}
	}
	if !acknowledged {
		return nil, fmt.Errorf("conflict: driver must acknowledge the current plan version before starting")
	}
	checkout, err := s.Repo.Checkout(ctx, run.ID)
	if err != nil { return nil, err }
	if checkout == nil || checkout.Status != "confirmed" || checkout.PlanVersion != run.PlanVersion {
		return nil, fmt.Errorf("conflict: truck checkout required before departure")
	}
	if !strings.EqualFold(latest.LoadingStatus, "ready") {
		return nil, fmt.Errorf("conflict: current load is not ready")
	}
	missing, err := assessCheckout(latest.Orders, checkout.ConfirmedOrderIDs)
	if err != nil || len(missing) > 0 {
		return nil, fmt.Errorf("conflict: current load no longer matches checkout")
	}
	started, err := s.Repo.StartRun(ctx, run.ID, actor(profile))
	if err != nil {
		if existing, e2 := s.Repo.GetByTrip(ctx, tripID); e2 == nil && existing.Status == domain.RunInProgress {
			return s.detail(ctx, existing)
		}
		return nil, err
	}
	if err := s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: started.ID, OperationType: domain.OpStart, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"status": domain.RunInProgress}}); err != nil {
		return nil, err
	}
	telemetry.DeliveryRunsStarted.Inc()
	s.Peers.Publish(ctx, audit.ActionDeliveryRunStarted, actor(profile), "TRIP", tripID, map[string]any{"runId": started.ID})
	return s.detail(ctx, started)
}

func (s Service) Arrive(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, occurred string) (map[string]any, error) {
	return s.applyArrive(ctx, profile, tripID, stopID, opID, occurred, "")
}

func (s Service) applyArrive(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, occurred, depends string) (map[string]any, error) {
	if res, ok, err := s.replayOrReject(ctx, opID, depends); ok {
		return res, err
	}
	run, stop, err := s.mutableStop(ctx, profile, tripID, stopID)
	if err != nil {
		return s.finishRejected(ctx, opID, run.ID, stopID, domain.OpArrived, err)
	}
	if run.Status != domain.RunInProgress {
		return s.record(ctx, opID, run.ID, stopID, domain.OpArrived, domain.ResultRejected, map[string]any{"detail": "run is not in progress"}, fmt.Errorf("conflict: run is not in progress"))
	}
	if stop.Status == domain.StopArrived || stop.Status == domain.StopCompleted {
		if stop.Status == domain.StopArrived {
			_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, StopID: stopID, OperationType: domain.OpArrived, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"status": stop.Status}})
			telemetry.DeliverySyncOps.WithLabelValues(domain.ResultApplied).Inc()
			return s.detail(ctx, run)
		}
		return s.record(ctx, opID, run.ID, stopID, domain.OpArrived, domain.ResultConflict, map[string]any{"detail": "stop already completed"}, fmt.Errorf("conflict: stop already completed"))
	}
	occurredAt := parseTime(occurred)
	if err := s.Repo.MarkArrived(ctx, stop.ID, occurredAt); err != nil {
		return s.record(ctx, opID, run.ID, stopID, domain.OpArrived, domain.ResultConflict, map[string]any{"detail": err.Error()}, err)
	}
	_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, StopID: stopID, OperationType: domain.OpArrived, OccurredAt: &occurredAt, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"status": domain.StopArrived}})
	telemetry.DeliverySyncOps.WithLabelValues(domain.ResultApplied).Inc()
	s.Peers.Publish(ctx, audit.ActionDeliveryStopArrived, actor(profile), "STOP", stopID, map[string]any{"tripId": tripID})
	s.Peers.Publish(ctx, audit.ActionDeliverySyncApplied, actor(profile), "SYNC", opID, map[string]any{"type": domain.OpArrived})
	s.refreshArrivalPredictionsAsync(tripID)
	run, _ = s.Repo.GetByTrip(ctx, tripID)
	return s.detail(ctx, run)
}

func (s Service) Outcome(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, depends, code, reason, note, occurred string, deliveredUnits *int, returned domain.ReturnDetails) (map[string]any, error) {
	return s.applyOutcome(ctx, profile, tripID, stopID, opID, depends, code, reason, note, occurred, deliveredUnits, returned)
}

func (s Service) applyOutcome(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, depends, code, reason, note, occurred string, deliveredUnits *int, returned domain.ReturnDetails) (map[string]any, error) {
	if res, ok, err := s.replayOrReject(ctx, opID, depends); ok {
		return res, err
	}
	run, stop, err := s.mutableStop(ctx, profile, tripID, stopID)
	if err != nil {
		return s.finishRejected(ctx, opID, run.ID, stopID, domain.OpStopOutcome, err)
	}
	code = strings.ToUpper(code)
	if code != domain.OutcomeDelivered && code != domain.OutcomePartial && code != domain.OutcomeNotDelivered && code != domain.OutcomeFailed && code != domain.OutcomeRefused {
		return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultRejected, map[string]any{"detail": "invalid outcome"}, fmt.Errorf("invalid: outcome code"))
	}
	if deliveredUnits != nil {
		if stop.ExpectedUnits == nil || *deliveredUnits < 0 || *deliveredUnits > *stop.ExpectedUnits ||
			(code == domain.OutcomeDelivered && *deliveredUnits != *stop.ExpectedUnits) ||
			(code == domain.OutcomePartial && (*deliveredUnits == 0 || *deliveredUnits == *stop.ExpectedUnits)) ||
			(code != domain.OutcomeDelivered && code != domain.OutcomePartial && *deliveredUnits != 0) {
			return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultRejected, map[string]any{"detail": "deliveredUnits must match the outcome and expectedUnits"}, fmt.Errorf("invalid: deliveredUnits must match outcome and expectedUnits"))
		}
	}
	if code == domain.OutcomeNotDelivered || code == domain.OutcomeFailed || code == domain.OutcomeRefused {
		if !validNonDeliveryReason(reason) {
			return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultRejected, map[string]any{"detail": "valid reasonCode required"}, fmt.Errorf("invalid: valid reasonCode required for non-delivery"))
		}
		reason = strings.ToUpper(strings.TrimSpace(reason))
	}
    returned.Goods = strings.TrimSpace(returned.Goods)
    if code == domain.OutcomeRefused && !validReturnDetails(returned) {
        return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultRejected,
            map[string]any{"detail":"returned goods, positive units and follow-up choice required"},
            fmt.Errorf("invalid: returned goods, positive units and follow-up choice required"))
    }
	if stop.Status != domain.StopArrived {
		if stop.Status == domain.StopCompleted && stop.OutcomeCode == code &&
			(deliveredUnits == nil || (stop.DeliveredUnits != nil && *deliveredUnits == *stop.DeliveredUnits)) {
            if code == domain.OutcomeRefused {
                original, err := s.Repo.ReturnedGoods(ctx, stop.ID)
                if err != nil { return nil, err }
                if original == nil || original.Goods != returned.Goods || original.Units != returned.Units ||
                    original.Resolution != returned.Resolution || original.Reason != reason || original.Note != note {
                    return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultConflict,
                        map[string]any{"detail": "rejected delivery already recorded"},
                        fmt.Errorf("conflict: rejected delivery already recorded"))
                }
                if err := s.finishReturnedGoods(ctx, stop.ID); err != nil { return nil, err }
            }
			_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, StopID: stopID, OperationType: domain.OpStopOutcome, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"code": code}})
			telemetry.DeliverySyncOps.WithLabelValues(domain.ResultApplied).Inc()
			return s.detail(ctx, run)
		}
		if stop.Status == domain.StopCompleted {
			telemetry.DeliverySyncConflicts.Inc()
			return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultConflict, map[string]any{"detail": "stop already completed"}, fmt.Errorf("conflict: stop already completed"))
		}
		return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultRejected, map[string]any{"detail": "stop is not arrived"}, fmt.Errorf("rejected: arrive before outcome"))
	}
	if code == domain.OutcomeDelivered || code == domain.OutcomePartial {
		n, _ := s.Repo.CountReadyProofs(ctx, stop.ID)
		if n == 0 {
			return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultRejected, map[string]any{"detail": "proof required"}, fmt.Errorf("rejected: proof required before outcome"))
		}
	}
    if code == domain.OutcomeRefused {
        manifest, err := s.Peers.ReadyTrip(ctx, tripID)
        if err != nil { return nil, err }
        expected := 0
        for _, order := range manifest.Orders {
            if order.OrderID == stop.OrderID { expected = order.ExpectedUnits; break }
        }
        if expected < 1 || returned.Units > expected {
            return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultRejected,
                map[string]any{"detail":"return quantity exceeds the planned goods"},
                fmt.Errorf("invalid: return quantity exceeds the planned goods"))
        }
    }
	occurredAt := parseTime(occurred)
    var outcomeErr error
    if code == domain.OutcomeRefused {
        outcomeErr = s.Repo.MarkRefusedOutcome(ctx, run, stop, opID, actor(profile), reason, note, returned, occurredAt)
    } else {
        outcomeErr = s.Repo.MarkOutcome(ctx, stop.ID, code, reason, note, occurredAt, deliveredUnits)
    }
	if err := outcomeErr; err != nil {
		telemetry.DeliverySyncConflicts.Inc()
		return s.record(ctx, opID, run.ID, stopID, domain.OpStopOutcome, domain.ResultConflict, map[string]any{"detail": err.Error()}, err)
	}
    if code == domain.OutcomeRefused {
        if err := s.finishReturnedGoods(ctx, stop.ID); err != nil { return nil, err }
    }
	_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, StopID: stopID, OperationType: domain.OpStopOutcome, OccurredAt: &occurredAt, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"code": code}})
	telemetry.DeliveryStopsCompleted.Inc()
	telemetry.DeliveryOutcomes.WithLabelValues(code).Inc()
	telemetry.DeliverySyncOps.WithLabelValues(domain.ResultApplied).Inc()
    auditPayload := map[string]any{"code": code, "tripId": tripID}
    if code == domain.OutcomeRefused {
        auditPayload["goods"] = returned.Goods
        auditPayload["units"] = returned.Units
        auditPayload["reason"] = reason
        auditPayload["resolution"] = returned.Resolution
        auditPayload["occurredAt"] = occurredAt
    }
	s.Peers.Publish(ctx, audit.ActionDeliveryOutcomeRecorded, actor(profile), "STOP", stopID, auditPayload)
	s.Peers.Publish(ctx, audit.ActionDeliverySyncApplied, actor(profile), "SYNC", opID, map[string]any{"type": domain.OpStopOutcome})
	s.refreshArrivalPredictionsAsync(tripID)
	run, _ = s.Repo.GetByTrip(ctx, tripID)
	return s.detail(ctx, run)
}

func validNonDeliveryReason(reason string) bool {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case domain.ReasonOutletClosed, domain.ReasonAccessBlocked, domain.ReasonReceiverUnavailable,
		domain.ReasonGoodsRejected, domain.ReasonVehicleIssue, domain.ReasonOther:
		return true
	default:
		return false
	}
}

func (s Service) UploadProof(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, proofType, mime string, body []byte, captured, receiverName string) (domain.Proof, error) {
	if opID == "" {
		return domain.Proof{}, fmt.Errorf("invalid: Idempotency-Key required")
	}
	run, stop, err := s.mutableStop(ctx, profile, tripID, stopID)
	if err != nil {
		return domain.Proof{}, err
	}
	if run.Status != domain.RunInProgress {
		return domain.Proof{}, fmt.Errorf("conflict: run is not in progress")
	}
	if stop.Status == domain.StopPending {
		return domain.Proof{}, fmt.Errorf("rejected: arrive before proof")
	}
	proofType = strings.ToUpper(proofType)
	if proofType != domain.ProofSignature && proofType != domain.ProofPhoto {
		return domain.Proof{}, fmt.Errorf("invalid: proof type")
	}
	mime = strings.ToLower(mime)
	if mime != "image/png" && mime != "image/jpeg" {
		return domain.Proof{}, fmt.Errorf("invalid: PNG or JPEG only")
	}
	if !validation.HasImageMagic(mime, body) {
		return domain.Proof{}, fmt.Errorf("invalid: file content does not match PNG or JPEG")
	}
	max := domain.MaxPhotoBytes
	if proofType == domain.ProofSignature {
		max = domain.MaxSignatureBytes
	}
	if len(body) == 0 || len(body) > max {
		return domain.Proof{}, fmt.Errorf("invalid: proof size")
	}
	receiverName = strings.TrimSpace(receiverName)
	if len(receiverName) > 120 {
		return domain.Proof{}, fmt.Errorf("invalid: receiverName")
	}
	if existing, err := s.Repo.GetProofByKey(ctx, opID); err == nil {
		if !existing.Pending {
			return existing, nil
		}
		return s.storeProofObject(ctx, profile, run, stop, existing, mime, body, opID)
	}
	placeholder := "pending/" + opID
	sum := sha256.Sum256(body)
	var capturedAt *time.Time
	if captured != "" {
		t := parseTime(captured)
		capturedAt = &t
	}
	pr, err := s.Repo.InsertProof(ctx, domain.Proof{
		StopID: stop.ID, ProofType: proofType, ObjectKey: placeholder, MimeType: mime,
		SHA256: hex.EncodeToString(sum[:]), CapturedAt: capturedAt, CreatedBy: actor(profile), IdempotencyKey: opID, ReceiverName: receiverName,
	})
	if err != nil {
		if existing, e2 := s.Repo.GetProofByKey(ctx, opID); e2 == nil {
			if !existing.Pending {
				return existing, nil
			}
			return s.storeProofObject(ctx, profile, run, stop, existing, mime, body, opID)
		}
		return domain.Proof{}, err
	}
	return s.storeProofObject(ctx, profile, run, stop, pr, mime, body, opID)
}

func (s Service) storeProofObject(ctx context.Context, profile *authorization.Profile, run domain.Run, stop domain.Stop, pr domain.Proof, mime string, body []byte, opID string) (domain.Proof, error) {
	key := fmt.Sprintf("delivery/%s/%s/%s", run.ID, stop.ID, pr.ID)
	if s.Objects != nil {
		if err := s.Objects.Put(ctx, key, mime, body); err != nil {
			_ = s.Objects.Delete(ctx, key)
			return domain.Proof{}, fmt.Errorf("object store write failed")
		}
	}
	if err := s.Repo.FinalizeProof(ctx, pr.ID, key); err != nil {
		if s.Objects != nil {
			_ = s.Objects.Delete(ctx, key)
		}
		return domain.Proof{}, err
	}
	pr.ObjectKey = key
	pr.Pending = false
	now := time.Now().UTC()
	pr.UploadedAt = &now
	if _, err := s.Repo.GetOp(ctx, opID); err != nil {
		_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, StopID: stop.ID, OperationType: domain.OpProofUpload, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"proofId": pr.ID}})
	}
	telemetry.DeliveryProofs.WithLabelValues(pr.ProofType).Inc()
	telemetry.DeliverySyncOps.WithLabelValues(domain.ResultApplied).Inc()
	s.Peers.Publish(ctx, audit.ActionDeliveryProofCaptured, actor(profile), "STOP", stop.ID, map[string]any{"type": pr.ProofType, "proofId": pr.ID})
	return pr, nil
}

func (s Service) Complete(ctx context.Context, profile *authorization.Profile, tripID, opID, occurred string) (map[string]any, error) {
	return s.applyComplete(ctx, profile, tripID, opID, "", occurred)
}

func (s Service) applyComplete(ctx context.Context, profile *authorization.Profile, tripID, opID, depends, occurred string) (map[string]any, error) {
	if res, ok, err := s.replayOrReject(ctx, opID, depends); ok {
		return res, err
	}
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	if err := s.guardVehicle(profile, run.VehicleID); err != nil {
		return nil, err
	}
	if run.Status == domain.RunCompleted {
		_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, OperationType: domain.OpRouteCompleted, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"status": run.Status}})
		return s.detail(ctx, run)
	}
	if run.Status != domain.RunInProgress {
		return s.record(ctx, opID, run.ID, "", domain.OpRouteCompleted, domain.ResultRejected, map[string]any{"detail": "run is not in progress"}, fmt.Errorf("conflict: run is not in progress"))
	}
	pending, err := s.Repo.IncompleteStopIDs(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if len(pending) > 0 {
		_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, OperationType: domain.OpRouteCompleted, ResultStatus: domain.ResultRejected, ResultPayload: map[string]any{"pendingStopIds": pending}})
		telemetry.DeliverySyncOps.WithLabelValues(domain.ResultRejected).Inc()
		return nil, IncompleteError{Pending: pending}
	}
	completed, err := s.Repo.CompleteRun(ctx, run.ID, actor(profile))
	if err != nil {
		return s.record(ctx, opID, run.ID, "", domain.OpRouteCompleted, domain.ResultConflict, map[string]any{"detail": err.Error()}, err)
	}
	occurredAt := parseTime(occurred)
	_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: run.ID, OperationType: domain.OpRouteCompleted, OccurredAt: &occurredAt, ResultStatus: domain.ResultApplied, ResultPayload: map[string]any{"status": domain.RunCompleted}})
	telemetry.DeliveryRunsCompleted.Inc()
	telemetry.DeliverySyncOps.WithLabelValues(domain.ResultApplied).Inc()
	if completed.StartedAt != nil {
		telemetry.DeliveryRunDuration.Observe(time.Since(*completed.StartedAt).Seconds())
	}
	s.Peers.Publish(ctx, audit.ActionDeliveryRunCompleted, actor(profile), "TRIP", tripID, map[string]any{"runId": completed.ID})
	return s.detail(ctx, completed)
}

func (s Service) ReportOfflineQueueHealth(profile *authorization.Profile, report domain.OfflineQueueHealth) error {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliverySync) || profile.VehicleID == "" {
		return fmt.Errorf("forbidden: assigned driver required")
	}
	validAge := map[string]bool{"none": true, "unknown": true, "lt24h": true, "1d_7d": true, "7d_30d": true, "30d_plus": true}
	validCount := map[string]bool{"none": true, "one": true, "2_5": true, "6_plus": true}
	if !validAge[report.AgeBucket] || !validCount[report.CountBucket] ||
		(report.AgeBucket == "none") != (report.CountBucket == "none") {
		return fmt.Errorf("invalid: offline queue health bucket")
	}
	telemetry.DriverOfflineQueueReports.WithLabelValues(report.AgeBucket, report.CountBucket).Inc()
	return nil
}

func (s Service) Sync(ctx context.Context, profile *authorization.Profile, req domain.SyncRequest) []map[string]any {
	var out []map[string]any
	for _, op := range req.Operations {
		out = append(out, s.flagOlderPlanVersion(ctx, op, s.syncOne(ctx, profile, op)))
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

func (s Service) syncOne(ctx context.Context, profile *authorization.Profile, op domain.SyncOperation) map[string]any {
	if op.OperationID == "" {
		return map[string]any{"operationId": op.OperationID, "status": domain.ResultRejected, "detail": "operationId required"}
	}
	if existing, err := s.Repo.GetOp(ctx, op.OperationID); err == nil {
		return map[string]any{"operationId": op.OperationID, "status": domain.ResultDuplicate, "originalStatus": existing.ResultStatus, "payload": existing.ResultPayload}
	}
	switch strings.ToUpper(op.Type) {
	case domain.OpProofUpload:
		telemetry.DeliverySyncOps.WithLabelValues(domain.ResultRejected).Inc()
		_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: op.OperationID, RunID: op.RunID, StopID: op.StopID, OperationType: domain.OpProofUpload, ResultStatus: domain.ResultRejected, ResultPayload: map[string]any{"detail": "proofs are not accepted in /sync"}})
		return map[string]any{"operationId": op.OperationID, "status": domain.ResultRejected, "detail": "proofs are not accepted in /sync"}
	case domain.OpArrived:
		_, err := s.applyArrive(ctx, profile, tripOf(ctx, s, op), op.StopID, op.OperationID, op.OccurredAt, op.DependsOnOperationID)
		return syncResult(op.OperationID, err)
	case domain.OpStopOutcome:
		code, _ := op.Payload["code"].(string)
		reason, _ := op.Payload["reason"].(string)
		note, _ := op.Payload["note"].(string)
		var deliveredUnits *int
		if raw, present := op.Payload["deliveredUnits"]; present {
			number, ok := raw.(float64)
			if !ok || number < 0 || number > 2147483647 || number != float64(int(number)) {
				return s.recordSyncRejection(ctx, op, "deliveredUnits must be a non-negative integer")
			}
			units := int(number)
			deliveredUnits = &units
		}
        returned := domain.ReturnDetails{}
        if raw, ok := op.Payload["returnedGoods"].(map[string]any); ok {
            returned.Goods, _ = raw["goods"].(string)
            if units, ok := raw["units"].(float64); ok && units == float64(int(units)) { returned.Units = int(units) }
            returned.Resolution, _ = raw["resolution"].(string)
        }
		_, err := s.applyOutcome(ctx, profile, tripOf(ctx, s, op), op.StopID, op.OperationID, op.DependsOnOperationID, code, reason, note, op.OccurredAt, deliveredUnits, returned)
		return syncResult(op.OperationID, err)
	case domain.OpTemperatureReading:
		value, ok := op.Payload["valueC"].(float64)
		if !ok {
			return s.recordSyncRejection(ctx, op, "temperature valueC must be numeric")
		}
		note, _ := op.Payload["note"].(string)
		reading, err := s.recordTemperature(ctx, profile, tripOf(ctx, s, op), op.StopID, op.OperationID, value, note, op.OccurredAt)
		if err != nil {
			return syncResult(op.OperationID, err)
		}
		return map[string]any{"operationId": op.OperationID, "status": domain.ResultApplied, "payload": map[string]any{"reading": reading}}
	case domain.OpRouteCompleted:
		_, err := s.applyComplete(ctx, profile, tripOf(ctx, s, op), op.OperationID, op.DependsOnOperationID, op.OccurredAt)
		return syncResult(op.OperationID, err)
	case domain.OpIncidentReport:
		category, _ := op.Payload["category"].(string)
		description, _ := op.Payload["description"].(string)
		item, err := s.recordIncident(ctx, profile, tripOf(ctx, s, op), op.StopID, op.OperationID, category, description, op.OccurredAt)
		if err != nil {
			return syncResult(op.OperationID, err)
		}
		return map[string]any{"operationId": op.OperationID, "status": domain.ResultApplied, "payload": map[string]any{"incident": item}}
	default:
		telemetry.DeliverySyncOps.WithLabelValues(domain.ResultRejected).Inc()
		return map[string]any{"operationId": op.OperationID, "status": domain.ResultRejected, "detail": "unknown type"}
	}
}

func (s Service) recordSyncRejection(ctx context.Context, op domain.SyncOperation, detail string) map[string]any {
	_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: op.OperationID, RunID: op.RunID, StopID: op.StopID, OperationType: domain.OpTemperatureReading, ResultStatus: domain.ResultRejected, ResultPayload: map[string]any{"detail": detail}})
	return map[string]any{"operationId": op.OperationID, "status": domain.ResultRejected, "detail": detail}
}

func (s Service) recordTemperature(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID string, value float64, note, occurred string) (domain.TemperatureReading, error) {
	if opID == "" || value < -40 || value > 100 || len([]rune(note)) > 500 {
		return domain.TemperatureReading{}, fmt.Errorf("invalid: temperature must be -40 to 100 °C and note at most 500 characters")
	}
	run, stop, err := s.mutableStop(ctx, profile, tripID, stopID)
	if err != nil {
		return domain.TemperatureReading{}, err
	}
	if !strings.EqualFold(stop.TemperatureRequirement, "chilled") {
		return domain.TemperatureReading{}, fmt.Errorf("rejected: temperature readings are only supported for chilled deliveries")
	}
	if stop.Status != domain.StopArrived {
		return domain.TemperatureReading{}, fmt.Errorf("rejected: arrive before recording a temperature reading")
	}
	at := parseTime(occurred)
	reading, err := s.Repo.RecordTemperatureReading(ctx, run.ID, stop.ID, opID, actor(profile), at, value, strings.TrimSpace(note))
	if err != nil {
		return domain.TemperatureReading{}, err
	}
	payload := map[string]any{"evaluation": reading.Evaluation, "valueC": reading.ValueC, "unit": "C", "actorId": reading.ActorID, "occurredAt": reading.OccurredAt}
	s.Peers.Publish(ctx, audit.ActionDeliveryTemperatureRecorded, actor(profile), "STOP", stop.ID, payload)
	if reading.Evaluation == "OUT_OF_RANGE" {
		s.Peers.Publish(ctx, audit.ActionDeliveryTemperatureException, actor(profile), "STOP", stop.ID, payload)
	}
	return reading, nil
}

var validIncidentCategories = map[string]bool{
	domain.IncidentVehicle: true, domain.IncidentRoad: true, domain.IncidentOutlet: true,
	domain.IncidentGoods: true, domain.IncidentSafety: true, domain.IncidentOther: true,
}

// recordIncident is FR-22: a Driver-reported field incident categorised as
// vehicle, road, outlet, goods, safety, or other. Unlike a temperature
// reading, it is not tied to an arrived stop - a road or vehicle problem can
// happen between stops - so stopID may be empty.
func (s Service) recordIncident(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, category, description, occurred string) (domain.DriverIncident, error) {
	category = strings.ToUpper(strings.TrimSpace(category))
	description = strings.TrimSpace(description)
	if opID == "" || !validIncidentCategories[category] || description == "" || len([]rune(description)) > 1000 {
		return domain.DriverIncident{}, fmt.Errorf("invalid: incident category must be one of vehicle/road/outlet/goods/safety/other, with a 1-1000 character description")
	}
	var run domain.Run
	var err error
	if stopID != "" {
		run, _, err = s.mutableStop(ctx, profile, tripID, stopID)
	} else {
		run, err = s.mutableRun(ctx, profile, tripID)
	}
	if err != nil {
		return domain.DriverIncident{}, err
	}
	at := parseTime(occurred)
	item, err := s.Repo.InsertDriverIncident(ctx, run.ID, stopID, opID, category, description, actor(profile), at)
	if err != nil {
		return domain.DriverIncident{}, err
	}
	s.Peers.Publish(ctx, audit.ActionDeliveryIncidentReported, actor(profile), "RUN", run.ID, map[string]any{"category": item.Category, "description": item.Description, "stopId": item.StopID, "tripId": tripID})
	return item, nil
}

func tripOf(ctx context.Context, s Service, op domain.SyncOperation) string {
	if op.TripID != "" {
		return op.TripID
	}
	if op.RunID == "" {
		return ""
	}
	run, err := s.Repo.GetRun(ctx, op.RunID)
	if err != nil {
		return ""
	}
	return run.TripID
}

func syncResult(opID string, err error) map[string]any {
	if err == nil {
		return map[string]any{"operationId": opID, "status": domain.ResultApplied}
	}
	msg := err.Error()
	status := domain.ResultRejected
	if strings.HasPrefix(msg, "conflict") || strings.Contains(msg, "delivery_incomplete") {
		status = domain.ResultConflict
		if strings.Contains(msg, "delivery_incomplete") {
			status = domain.ResultRejected
		}
	}
	if strings.HasPrefix(msg, "rejected") || strings.HasPrefix(msg, "invalid") {
		status = domain.ResultRejected
	}
	out := map[string]any{"operationId": opID, "status": status, "detail": msg}
	var inc IncompleteError
	if asIncomplete(err, &inc) {
		out["pendingStopIds"] = inc.Pending
	}
	return out
}

func asIncomplete(err error, dest *IncompleteError) bool {
	if inc, ok := err.(IncompleteError); ok {
		*dest = inc
		return true
	}
	return false
}

func (s Service) replayOrReject(ctx context.Context, opID, depends string) (map[string]any, bool, error) {
	if opID == "" {
		return nil, true, fmt.Errorf("invalid: operationId required")
	}
	if existing, err := s.Repo.GetOp(ctx, opID); err == nil {
		return map[string]any{"duplicate": true, "operationId": opID, "status": domain.ResultDuplicate, "originalStatus": existing.ResultStatus, "payload": existing.ResultPayload}, true, nil
	}
	if depends != "" {
		dep, err := s.Repo.GetOp(ctx, depends)
		if err != nil || dep.ResultStatus != domain.ResultApplied {
			telemetry.DeliverySyncOps.WithLabelValues(domain.ResultRejected).Inc()
			return nil, true, fmt.Errorf("rejected: dependsOnOperationId not applied")
		}
	}
	return nil, false, nil
}

func (s Service) record(ctx context.Context, opID, runID, stopID, typ, status string, payload map[string]any, err error) (map[string]any, error) {
	_ = s.Repo.InsertOp(ctx, domain.SyncOp{OperationID: opID, RunID: runID, StopID: stopID, OperationType: typ, ResultStatus: status, ResultPayload: payload})
	telemetry.DeliverySyncOps.WithLabelValues(status).Inc()
	if status == domain.ResultConflict {
		telemetry.DeliverySyncConflicts.Inc()
		s.Peers.Publish(ctx, audit.ActionDeliverySyncConflict, "", "SYNC", opID, payload)
	}
	return nil, err
}

func (s Service) finishRejected(ctx context.Context, opID, runID, stopID, typ string, err error) (map[string]any, error) {
	if err == nil {
		return nil, nil
	}
	status := domain.ResultRejected
	if strings.HasPrefix(err.Error(), "conflict") || strings.HasPrefix(err.Error(), "forbidden") {
		status = domain.ResultConflict
	}
	return s.record(ctx, opID, runID, stopID, typ, status, map[string]any{"detail": err.Error()}, err)
}

func (s Service) mutableStop(ctx context.Context, profile *authorization.Profile, tripID, stopID string) (domain.Run, domain.Stop, error) {
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return domain.Run{}, domain.Stop{}, fmt.Errorf("not found")
	}
	if err := s.guardVehicle(profile, run.VehicleID); err != nil {
		return run, domain.Stop{}, err
	}
	stop, err := s.Repo.GetStop(ctx, run.ID, stopID)
	if err != nil {
		return run, stop, fmt.Errorf("not found")
	}
	return run, stop, nil
}

// mutableRun resolves and authorizes a trip's run without requiring a stop,
// for driver actions (FR-22's incident report) that are not tied to one.
func (s Service) mutableRun(ctx context.Context, profile *authorization.Profile, tripID string) (domain.Run, error) {
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return domain.Run{}, fmt.Errorf("not found")
	}
	if err := s.guardVehicle(profile, run.VehicleID); err != nil {
		return run, err
	}
	return run, nil
}

func (s Service) guardVehicle(profile *authorization.Profile, vehicleID string) error {
	if authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return nil
	}
	if profile == nil || profile.VehicleID == "" || !strings.EqualFold(profile.VehicleID, vehicleID) {
		return fmt.Errorf("forbidden: driver may only access trips for their assigned vehicle")
	}
	return nil
}

func (s Service) detail(ctx context.Context, run domain.Run) (map[string]any, error) {
	stops, _ := s.Repo.ListStops(ctx, run.ID)
	stops = s.withLocations(ctx, stops)
	currentVersion := run.PlanVersion
	var loadList []domain.LoadingOrder
	var acknowledgements []domain.PlanAcknowledgement
	if trip, err := s.Peers.ReadyTrip(ctx, run.TripID); err == nil {
		currentVersion = trip.PlanVersion
		loadList = trip.Orders
		acknowledgements = trip.PlanAcknowledgements
	}
	if acknowledgements == nil {
		acknowledgements = []domain.PlanAcknowledgement{}
	}
	checkout, _ := s.Repo.Checkout(ctx, run.ID)
	return map[string]any{"tripId": run.TripID, "run": run, "stops": stops, "status": run.Status, "currentPlanVersion": currentVersion, "planAcknowledgements": acknowledgements, "loadList": loadList, "checkout": checkout}, nil
}

// withLocations adds each stop's position from its outlet. It is best effort: if shared-service cannot
// be reached the trip is still served, just without positions, and a driver app falls back to searching
// by outlet name.
func (s Service) withLocations(ctx context.Context, stops []domain.Stop) []domain.Stop {
	if len(stops) == 0 {
		return stops
	}
	outlets, err := s.Peers.Outlets(ctx)
	if err != nil {
		return stops
	}
	located := make([]domain.Stop, len(stops))
	copy(located, stops)
	for i := range located {
		if o, ok := outlets[located[i].OutletID]; ok && o.Latitude != nil && o.Longitude != nil {
			located[i].Latitude, located[i].Longitude, located[i].LocationApproximate = o.Latitude, o.Longitude, o.LocationApproximate
		}
	}
	return located
}

func loadingPreview(trip domain.LoadingTrip) map[string]any {
	var stops []map[string]any
	for _, o := range trip.Orders {
		stops = append(stops, map[string]any{
			"orderId": o.OrderID, "orderRef": o.OrderRef, "outletId": o.OutletID, "brand": o.Brand,
			"stopSequence": o.StopSequence, "loadingStatus": o.LoadingStatus, "status": domain.StopPending,
			"expectedUnits": o.ExpectedUnits, "unitLabel": "units",
			"loadingShortfallSummary": o.ShortfallSummary,
		})
	}
	if stops == nil {
		stops = []map[string]any{}
	}
	return map[string]any{
		"tripId": trip.TripID, "status": "ready", "stops": stops,
		"run": map[string]any{
			"tripId": trip.TripID, "planId": trip.PlanID, "planRef": trip.PlanRef, "deliveryDate": trip.DeliveryDate,
			"vehicleId": trip.VehicleID, "depot": trip.Depot, "tripNumber": trip.TripNumber, "status": "ready",
		},
	}
}

func actor(p *authorization.Profile) string {
	if p == nil {
		return ""
	}
	return p.UserID
}

func parseTime(v string) time.Time {
	if v == "" {
		return time.Now().UTC()
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC()
	}
	return time.Now().UTC()
}

func completedCount(stops []domain.Stop) int {
	n := 0
	for _, s := range stops {
		if s.Status == domain.StopCompleted {
			n++
		}
	}
	return n
}

func NewOperationID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
