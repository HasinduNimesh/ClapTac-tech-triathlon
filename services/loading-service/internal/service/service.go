package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/sequence"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/store"
)

type Service struct {
	Repo     store.Postgres
	Peers    client.Peers
	Sequence sequence.Policy
}

func (s Service) seq() sequence.Policy {
	if s.Sequence != nil {
		return s.Sequence
	}
	return sequence.ReverseLastOut{}
}

func (s Service) List(ctx context.Context, profile *authorization.Profile, date string) ([]map[string]any, error) {
	depot := ""
	if !authorization.HasPermission(profile.Roles, authorization.PermLoadingViewAll) {
		depot = profile.Depot
		if depot == "" {
			return nil, fmt.Errorf("forbidden: loader depot required")
		}
	}
	trips, err := s.Peers.ConfirmedTrips(ctx, date, depot)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, t := range trips {
		item := tripSummary(t)
		if sess, err := s.Repo.GetByTrip(ctx, t.TripID); err == nil {
			loads, _ := s.Repo.ListLoads(ctx, sess.ID)
			item["loadingStatus"] = sess.Status
			item["loadedCount"], item["shortfallCount"], item["pendingCount"] = counts(loads)
			item["planRef"] = sess.PlanRef
		} else {
			item["loadingStatus"] = domain.SessionPending
			item["loadedCount"], item["shortfallCount"], item["pendingCount"] = 0, 0, len(t.Allocations)
		}
		out = append(out, item)
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, nil
}

func (s Service) Get(ctx context.Context, profile *authorization.Profile, tripID string) (map[string]any, error) {
	if sess, err := s.Repo.GetByTrip(ctx, tripID); err == nil {
		if err := s.guardDepot(profile, sess.Depot); err != nil {
			return nil, err
		}
		return s.detailFromSession(ctx, profile, sess)
	}
	trip, err := s.Peers.ConfirmedTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	if err := s.guardDepot(profile, trip.VehicleDepot); err != nil {
		return nil, err
	}
	return pendingDetail(trip, s.seq(), profile), nil
}

func (s Service) Start(ctx context.Context, profile *authorization.Profile, tripID string, expectedPlanVersion int) (map[string]any, error) {
	if sess, err := s.Repo.GetByTrip(ctx, tripID); err == nil {
		if err := s.guardDepot(profile, sess.Depot); err != nil {
			return nil, err
		}
		if expectedPlanVersion > 0 && expectedPlanVersion != sess.PlanVersion {
			return nil, fmt.Errorf("conflict: loading session uses plan version %d, requested %d", sess.PlanVersion, expectedPlanVersion)
		}
		return s.detailFromSession(ctx, profile, sess)
	}
	trip, err := s.Peers.ConfirmedTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	if !strings.EqualFold(trip.PlanStatus, "confirmed") {
		return nil, fmt.Errorf("conflict: trip is not on a confirmed plan")
	}
	if trip.PlanVersion < 1 {
		return nil, fmt.Errorf("conflict: trip has no published plan version")
	}
	if expectedPlanVersion > 0 && expectedPlanVersion != trip.PlanVersion {
		return nil, fmt.Errorf("conflict: plan version changed from %d to %d; reload latest loading instructions", expectedPlanVersion, trip.PlanVersion)
	}
	if err := s.guardDepot(profile, trip.VehicleDepot); err != nil {
		return nil, err
	}
	stops := make([]domain.Stop, 0, len(trip.Allocations))
	for _, a := range trip.Allocations {
		stops = append(stops, domain.Stop{OrderID: a.OrderID, StopSequence: a.StopSequence})
	}
	sug := s.seq().Suggest(stops)
	sugBy := map[string]int{}
	for _, g := range sug {
		sugBy[g.OrderID] = g.SuggestedLoadSequence
	}
	var loads []domain.OrderLoad
	for _, a := range trip.Allocations {
		ord, err := s.Peers.Order(ctx, a.OrderID)
		if err != nil {
			return nil, fmt.Errorf("order details unavailable")
		}
		loads = append(loads, domain.OrderLoad{
			AllocationID: a.AllocationID, OrderID: a.OrderID, OrderRef: first(ord.OrderRef, a.OrderRef),
			OutletID: first(ord.OutletID, a.OutletID), Brand: ord.Brand, TemperatureRequirement: ord.TemperatureRequirement,
			StopSequence: a.StopSequence, SuggestedLoadSequence: sugBy[a.OrderID],
			ExpectedUnits: ord.OrderUnits,
		})
	}
	sess, err := s.Repo.StartTx(ctx, domain.Session{
		TripID: trip.TripID, PlanID: trip.PlanID, PlanRef: trip.PlanRef, DeliveryDate: trip.DeliveryDate,
		VehicleID: trip.VehicleID, Depot: trip.VehicleDepot, StartedBy: actor(profile),
		TripNumber: trip.TripNumber, VehicleType: trip.VehicleType, VehicleTemperatureCapability: trip.VehicleTemperatureCapability,
		PlanVersion: trip.PlanVersion,
	}, loads)
	if err != nil {
		if existing, e2 := s.Repo.GetByTrip(ctx, tripID); e2 == nil {
			return s.detailFromSession(ctx, profile, existing)
		}
		return nil, err
	}
	telemetry.LoadingSessionsStarted.Inc()
	s.Peers.Publish(ctx, audit.ActionLoadingStarted, actor(profile), "TRIP", tripID, map[string]any{"planRef": trip.PlanRef})
	return s.detailFromSession(ctx, profile, sess)
}

func (s Service) MarkLoaded(ctx context.Context, profile *authorization.Profile, tripID, orderID string) error {
	sess, load, err := s.mutableLoad(ctx, profile, tripID, orderID)
	if err != nil {
		return err
	}
	issues, _ := s.Repo.ListIssues(ctx, load.ID)
	if len(issues) > 0 {
		return fmt.Errorf("conflict: ACTIVE_LOADING_ISSUE")
	}
	if load.Status == domain.LoadLoaded {
		return nil
	}
	if err := s.Repo.SetLoadStatus(ctx, load.ID, domain.LoadLoaded, actor(profile)); err != nil {
		return err
	}
	telemetry.LoadingOrdersCompleted.Inc()
	s.Peers.Publish(ctx, audit.ActionOrderLoaded, actor(profile), "ORDER", orderID, map[string]any{"tripId": sess.TripID})
	return nil
}

func (s Service) CreateIssue(ctx context.Context, profile *authorization.Profile, tripID, orderID, typ, note, key string, units int) (domain.Issue, error) {
	if key == "" {
		return domain.Issue{}, fmt.Errorf("invalid: Idempotency-Key required")
	}
	if existing, err := s.Repo.GetIssueByKey(ctx, key); err == nil {
		return existing, nil
	}
	_, load, err := s.mutableLoad(ctx, profile, tripID, orderID)
	if err != nil {
		return domain.Issue{}, err
	}
	typ = strings.ToUpper(typ)
	if typ != domain.IssueMissing && typ != domain.IssueDamaged {
		return domain.Issue{}, fmt.Errorf("invalid: issue type")
	}
	if units <= 0 || units > load.ExpectedUnits {
		return domain.Issue{}, fmt.Errorf("invalid: affectedUnits")
	}
	iss, err := s.Repo.InsertIssue(ctx, domain.Issue{
		OrderLoadID: load.ID, IssueType: typ, AffectedUnits: units, Note: note, ReportedBy: actor(profile), IdempotencyKey: key,
	})
	if err != nil {
		if existing, e2 := s.Repo.GetIssueByKey(ctx, key); e2 == nil {
			return existing, nil
		}
		return domain.Issue{}, err
	}
	_ = s.Repo.SetLoadStatus(ctx, load.ID, domain.LoadShortfall, actor(profile))
	telemetry.LoadingShortfalls.WithLabelValues(typ).Inc()
	s.Peers.Publish(ctx, audit.ActionLoadingShortfallRecorded, actor(profile), "ORDER", orderID, map[string]any{"type": typ, "affectedUnits": units})
	return iss, nil
}

func (s Service) UpdateIssue(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID, typ, note string, units int) error {
	_, load, err := s.mutableLoad(ctx, profile, tripID, orderID)
	if err != nil {
		return err
	}
	iss, err := s.Repo.GetIssue(ctx, issueID)
	if err != nil || iss.OrderLoadID != load.ID {
		return fmt.Errorf("not found")
	}
	if typ == "" {
		typ = iss.IssueType
	}
	typ = strings.ToUpper(typ)
	if units <= 0 || units > load.ExpectedUnits {
		return fmt.Errorf("invalid: affectedUnits")
	}
	if err := s.Repo.UpdateIssue(ctx, issueID, units, note, typ); err != nil {
		return err
	}
	s.Peers.Publish(ctx, audit.ActionLoadingShortfallUpdated, actor(profile), "ORDER", orderID, map[string]any{"issueId": issueID})
	return nil
}

func (s Service) DeleteIssue(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID string) error {
	_, load, err := s.mutableLoad(ctx, profile, tripID, orderID)
	if err != nil {
		return err
	}
	iss, err := s.Repo.GetIssue(ctx, issueID)
	if err != nil || iss.OrderLoadID != load.ID {
		return fmt.Errorf("not found")
	}
	if err := s.Repo.DeleteIssue(ctx, issueID); err != nil {
		return err
	}
	s.Peers.Publish(ctx, audit.ActionLoadingShortfallRemoved, actor(profile), "ORDER", orderID, map[string]any{"issueId": issueID})
	return nil
}

func (s Service) Ready(ctx context.Context, profile *authorization.Profile, tripID string) (map[string]any, error) {
	sess, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	if err := s.guardDepot(profile, sess.Depot); err != nil {
		return nil, err
	}
	trip, err := s.Peers.ConfirmedTrip(ctx, tripID)
	if err != nil || trip.PlanVersion != sess.PlanVersion {
		return nil, fmt.Errorf("conflict: plan version changed; reload latest instructions and acknowledge before departure")
	}
	acked := false
	for _, ack := range trip.PlanAcknowledgements {
		if ack.ActorRole == authorization.RoleLoader && ack.ActorID == actor(profile) {
			acked = true
		}
	}
	if !acked {
		return nil, fmt.Errorf("conflict: loader must acknowledge the current plan version before departure")
	}
	if sess.Status == domain.SessionReady {
		return s.detailFromSession(ctx, profile, sess)
	}
	if sess.Status != domain.SessionInProgress {
		return nil, fmt.Errorf("conflict: session not in progress")
	}
	loads, _ := s.Repo.ListLoads(ctx, sess.ID)
	var pending, undecided []string
	for _, l := range loads {
		if l.Status == domain.LoadPending {
			pending = append(pending, l.OrderID)
			continue
		}
		if l.Status == domain.LoadShortfall {
			iss, _ := s.Repo.ListIssues(ctx, l.ID)
			if len(iss) == 0 {
				pending = append(pending, l.OrderID)
				continue
			}
			// A shortfall only lets the trip leave once the dispatcher has
			// accepted a partial load or moved the line to the next run.
			for _, i := range iss {
				if !domain.DecisionAllowsDeparture(i.Decision) {
					undecided = append(undecided, l.OrderID)
					break
				}
			}
		}
	}
	if len(pending) > 0 {
		return nil, IncompleteError{Pending: pending}
	}
	if len(undecided) > 0 {
		return nil, DecisionRequiredError{OrderIDs: undecided}
	}
	ready, err := s.Repo.MarkReady(ctx, sess.ID, actor(profile))
	if err != nil {
		if existing, e2 := s.Repo.GetByTrip(ctx, tripID); e2 == nil && existing.Status == domain.SessionReady {
			return s.detailFromSession(ctx, profile, existing)
		}
		return nil, err
	}
	telemetry.LoadingTripsReady.Inc()
	telemetry.LoadingDuration.Observe(time.Since(sess.StartedAt).Seconds())
	s.Peers.Publish(ctx, audit.ActionTripReadyForDeparture, actor(profile), "TRIP", tripID, map[string]any{"planRef": sess.PlanRef})
	return s.detailFromSession(ctx, profile, ready)
}

type IncompleteError struct{ Pending []string }

// DecisionRequiredError blocks departure while a loader shortfall has no
// dispatcher decision, or the dispatcher chose to hold the trip.
type DecisionRequiredError struct{ OrderIDs []string }

func (e DecisionRequiredError) Error() string { return "conflict: dispatcher_decision_required" }

// DecideIssue records the dispatcher's decision on a loader shortfall. The
// order is never reduced here: moving a line to the next run is done in
// planning, which publishes a new plan version for the loader to acknowledge.
func (s Service) DecideIssue(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID, decision, note string) (domain.Issue, error) {
	decision = strings.ToUpper(strings.TrimSpace(decision))
	if decision != domain.DecisionPartialLoad && decision != domain.DecisionHold && decision != domain.DecisionMoveToNextRun {
		return domain.Issue{}, fmt.Errorf("invalid: decision must be PARTIAL_LOAD, HOLD or MOVE_TO_NEXT_RUN")
	}
	if len(note) > 1000 {
		return domain.Issue{}, fmt.Errorf("invalid: note too long")
	}
	sess, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return domain.Issue{}, fmt.Errorf("not found")
	}
	if err := s.guardDepot(profile, sess.Depot); err != nil {
		return domain.Issue{}, err
	}
	if sess.Status == domain.SessionReady {
		return domain.Issue{}, fmt.Errorf("conflict: session ready")
	}
	load, err := s.Repo.GetLoad(ctx, sess.ID, orderID)
	if err != nil {
		return domain.Issue{}, fmt.Errorf("not found: order")
	}
	iss, err := s.Repo.GetIssue(ctx, issueID)
	if err != nil || iss.OrderLoadID != load.ID {
		return domain.Issue{}, fmt.Errorf("not found")
	}
	if iss.Decision == decision && iss.DecisionNote == note {
		return iss, nil
	}
	if err := s.Repo.DecideIssue(ctx, issueID, decision, note, actor(profile)); err != nil {
		return domain.Issue{}, err
	}
	s.Peers.Publish(ctx, audit.ActionLoadingShortfallDecided, actor(profile), "ORDER", orderID, map[string]any{"issueId": issueID, "tripId": tripID, "decision": decision, "note": note})
	return s.Repo.GetIssue(ctx, issueID)
}

func (e IncompleteError) Error() string { return "conflict: loading_incomplete" }

func (s Service) mutableLoad(ctx context.Context, profile *authorization.Profile, tripID, orderID string) (domain.Session, domain.OrderLoad, error) {
	sess, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return sess, domain.OrderLoad{}, fmt.Errorf("not found")
	}
	if err := s.guardDepot(profile, sess.Depot); err != nil {
		return sess, domain.OrderLoad{}, err
	}
	if sess.Status == domain.SessionReady {
		return sess, domain.OrderLoad{}, fmt.Errorf("conflict: session ready")
	}
	if sess.Status != domain.SessionInProgress {
		return sess, domain.OrderLoad{}, fmt.Errorf("conflict: session not started")
	}
	load, err := s.Repo.GetLoad(ctx, sess.ID, orderID)
	if err != nil {
		return sess, load, fmt.Errorf("not found: order")
	}
	return sess, load, nil
}

func (s Service) guardDepot(profile *authorization.Profile, depot string) error {
	if profile != nil && authorization.HasPermission(profile.Roles, authorization.PermLoadingViewAll) {
		return nil
	}
	if profile == nil || profile.Depot == "" || !strings.EqualFold(profile.Depot, depot) {
		return fmt.Errorf("forbidden: depot")
	}
	return nil
}

func (s Service) detailFromSession(ctx context.Context, profile *authorization.Profile, sess domain.Session) (map[string]any, error) {
	loads, _ := s.Repo.ListLoads(ctx, sess.ID)
	var orders []map[string]any
	for _, l := range loads {
		iss, _ := s.Repo.ListIssues(ctx, l.ID)
		ord, _ := s.Peers.Order(ctx, l.OrderID)
		orders = append(orders, map[string]any{
			"orderId": l.OrderID, "orderRef": first(l.OrderRef, ord.OrderRef), "outletId": first(l.OutletID, ord.OutletID),
			"brand": first(l.Brand, ord.Brand), "temperatureRequirement": first(l.TemperatureRequirement, ord.TemperatureRequirement),
			"expectedUnits": l.ExpectedUnits, "stopSequence": l.StopSequence,
			"suggestedLoadSequence": l.SuggestedLoadSequence, "status": l.Status, "issues": iss, "allocationId": l.AllocationID,
		})
	}
	if orders == nil {
		orders = []map[string]any{}
	}
	loaded, short, pend := counts(loads)
	acknowledged := false
	if trip, err := s.Peers.ConfirmedTrip(ctx, sess.TripID); err == nil {
		for _, ack := range trip.PlanAcknowledgements {
			if ack.ActorID == actor(profile) && ack.ActorRole == authorization.RoleLoader && trip.PlanVersion == sess.PlanVersion {
				acknowledged = true
			}
		}
	}
	return map[string]any{
		"tripId": sess.TripID, "planId": sess.PlanID, "planRef": sess.PlanRef, "deliveryDate": sess.DeliveryDate,
		"vehicleId": sess.VehicleID, "depot": sess.Depot, "status": sess.Status,
		"tripNumber": sess.TripNumber, "vehicleType": sess.VehicleType, "vehicleTemperatureCapability": sess.VehicleTemperatureCapability,
		"loadedCount": loaded, "shortfallCount": short, "pendingCount": pend,
		"planVersion": sess.PlanVersion, "acknowledgedVersion": func() int {
			if acknowledged {
				return sess.PlanVersion
			}
			return 0
		}(),
		"orders": orders, "session": sess,
	}, nil
}

func (s Service) InternalList(ctx context.Context, date, vehicleID string) ([]map[string]any, error) {
	if date == "" {
		return nil, fmt.Errorf("invalid: date required")
	}
	sessions, err := s.Repo.ListReady(ctx, date, vehicleID)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, sess := range sessions {
		d, err := s.internalDTO(ctx, sess)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, nil
}

func (s Service) InternalGet(ctx context.Context, tripID string) (map[string]any, error) {
	sess, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	if sess.Status != domain.SessionReady {
		return nil, fmt.Errorf("conflict: trip is not ready for departure")
	}
	return s.internalDTO(ctx, sess)
}

func (s Service) internalDTO(ctx context.Context, sess domain.Session) (map[string]any, error) {
	loads, _ := s.Repo.ListLoads(ctx, sess.ID)
	arrivalByAllocation := map[string]*time.Time{}
	if trip, err := s.Peers.ConfirmedTrip(ctx, sess.TripID); err == nil {
		arrivalByAllocation = plannedArrivalByAllocation(trip.Allocations)
	}
	var orders []map[string]any
	for _, l := range loads {
		iss, _ := s.Repo.ListIssues(ctx, l.ID)
		var short []map[string]any
		for _, i := range iss {
			short = append(short, map[string]any{"type": i.IssueType, "affectedUnits": i.AffectedUnits, "note": i.Note})
		}
		if short == nil {
			short = []map[string]any{}
		}
		orders = append(orders, map[string]any{
			"allocationId": l.AllocationID, "orderId": l.OrderID, "orderRef": l.OrderRef, "outletId": l.OutletID,
			"brand": l.Brand, "stopSequence": l.StopSequence, "loadingStatus": l.Status,
			"temperatureRequirement": l.TemperatureRequirement, "expectedUnits": l.ExpectedUnits,
			"shortfallSummary": short,
			"plannedArrivalAt": arrivalByAllocation[l.AllocationID],
		})
	}
	if orders == nil {
		orders = []map[string]any{}
	}
	currentVersion := sess.PlanVersion
	var acknowledgements []domain.PlanAcknowledgement
	if trip, err := s.Peers.ConfirmedTrip(ctx, sess.TripID); err == nil {
		currentVersion = trip.PlanVersion
		acknowledgements = trip.PlanAcknowledgements
	}
	if acknowledgements == nil {
		acknowledgements = []domain.PlanAcknowledgement{}
	}
	return map[string]any{
		"tripId": sess.TripID, "planId": sess.PlanID, "planRef": sess.PlanRef, "deliveryDate": sess.DeliveryDate,
		"vehicleId": sess.VehicleID, "vehicleType": sess.VehicleType, "vehicleTemperatureCapability": sess.VehicleTemperatureCapability,
		"depot": sess.Depot, "tripNumber": sess.TripNumber, "loadingStatus": sess.Status, "planVersion": currentVersion, "preparedPlanVersion": sess.PlanVersion, "planAcknowledgements": acknowledgements, "orders": orders,
	}, nil
}

func plannedArrivalByAllocation(allocations []domain.PlanningAlloc) map[string]*time.Time {
	arrivals := make(map[string]*time.Time, len(allocations))
	for _, allocation := range allocations {
		if allocation.AllocationID != "" && allocation.PlannedArrivalAt != nil {
			arrival := *allocation.PlannedArrivalAt
			arrivals[allocation.AllocationID] = &arrival
		}
	}
	return arrivals
}

func pendingDetail(trip domain.PlanningTrip, pol sequence.Policy, profile *authorization.Profile) map[string]any {
	stops := make([]domain.Stop, 0, len(trip.Allocations))
	for _, a := range trip.Allocations {
		stops = append(stops, domain.Stop{OrderID: a.OrderID, StopSequence: a.StopSequence})
	}
	sug := pol.Suggest(stops)
	sugBy := map[string]int{}
	for _, g := range sug {
		sugBy[g.OrderID] = g.SuggestedLoadSequence
	}
	var orders []map[string]any
	for _, a := range trip.Allocations {
		orders = append(orders, map[string]any{
			"orderId": a.OrderID, "orderRef": a.OrderRef, "outletId": a.OutletID,
			"stopSequence": a.StopSequence, "suggestedLoadSequence": sugBy[a.OrderID],
			"status": domain.LoadPending, "issues": []domain.Issue{},
		})
	}
	if orders == nil {
		orders = []map[string]any{}
	}
	acknowledgedVersion := 0
	for _, ack := range trip.PlanAcknowledgements {
		if ack.ActorID == actor(profile) && ack.ActorRole == authorization.RoleLoader && trip.PlanVersion > 0 {
			acknowledgedVersion = trip.PlanVersion
			break
		}
	}
	return map[string]any{
		"tripId": trip.TripID, "planId": trip.PlanID, "planRef": trip.PlanRef, "deliveryDate": trip.DeliveryDate,
		"vehicleId": trip.VehicleID, "vehicleType": trip.VehicleType,
		"vehicleTemperatureCapability": trip.VehicleTemperatureCapability,
		"depot":                        trip.VehicleDepot, "tripNumber": trip.TripNumber, "status": domain.SessionPending,
		"planVersion": trip.PlanVersion, "acknowledgedVersion": acknowledgedVersion,
		"loadedCount": 0, "shortfallCount": 0, "pendingCount": len(trip.Allocations), "orders": orders,
	}
}

func tripSummary(t domain.PlanningTrip) map[string]any {
	return map[string]any{
		"tripId": t.TripID, "planRef": t.PlanRef, "vehicleId": t.VehicleID, "tripNumber": t.TripNumber,
		"depot": t.VehicleDepot, "vehicleType": t.VehicleType, "planVersion": t.PlanVersion,
		"vehicleTemperatureCapability": t.VehicleTemperatureCapability, "stopCount": len(t.Allocations),
	}
}

func counts(loads []domain.OrderLoad) (loaded, short, pending int) {
	for _, l := range loads {
		switch l.Status {
		case domain.LoadLoaded:
			loaded++
		case domain.LoadShortfall:
			short++
		default:
			pending++
		}
	}
	return
}

func actor(p *authorization.Profile) string {
	if p == nil {
		return ""
	}
	return p.UserID
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
