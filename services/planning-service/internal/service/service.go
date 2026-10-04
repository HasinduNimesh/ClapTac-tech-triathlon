package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/allocate"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/constraints"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/schedule"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/store"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
)

var validReasons = map[string]bool{
	domain.ReasonNoEligibleVehicle: true, domain.ReasonWeightExceeded: true, domain.ReasonVolumeExceeded: true,
	domain.ReasonRefrigeration: true, domain.ReasonVanRequired: true, domain.ReasonDepotMismatch: true,
	domain.ReasonDeliveryWindow: true, domain.ReasonFuelExceeded: true, domain.ReasonTripLimit: true,
	domain.ReasonVehicleUnavailable: true, domain.ReasonManualDeferral: true,
}

// ErrCodeWorkshopPending prefixes the error ConfirmBreakdown returns when the
// broken vehicle could not be moved to the workshop. The handler turns it into
// a 502 with the same stable code; it is never reported as a confirmed recovery.
const ErrCodeWorkshopPending = "workshop_pending"

type planVersionStore interface {
	Publish(context.Context, string, string, string) (domain.Publication, error)
	Publication(context.Context, string) (domain.Publication, error)
	Acknowledge(context.Context, string, int, string, string, string, string) error
	RecordReminder(context.Context, string, int, string, string, string, time.Duration) (domain.PlanReminder, bool, error)
	Revise(context.Context, string) error
	MoveTripAllocations(context.Context, string, string, string, int, []domain.Allocation) error
}

type Service struct {
	Repo  store.Postgres
	Peers client.Peers
}

func (s Service) ListDisruptionRisks(ctx context.Context, date string) ([]domain.DisruptionRisk, error) {
	if !validDeliveryDate(date) {
		return nil, fmt.Errorf("invalid: deliveryDate must be YYYY-MM-DD")
	}
	return s.Repo.ListDisruptionRisks(ctx, date)
}

func (s Service) CreateDisruptionRisk(ctx context.Context, profile *authorization.Profile, risk domain.DisruptionRisk) (domain.DisruptionRisk, error) {
	if profile == nil || strings.TrimSpace(profile.UserID) == "" {
		return domain.DisruptionRisk{}, fmt.Errorf("forbidden: dispatcher profile required")
	}
	risk.DeliveryDate = strings.TrimSpace(risk.DeliveryDate)
	if !validDeliveryDate(risk.DeliveryDate) {
		return domain.DisruptionRisk{}, fmt.Errorf("invalid: deliveryDate must be YYYY-MM-DD")
	}
	risk.Scope = strings.ToUpper(strings.TrimSpace(risk.Scope))
	risk.ScopeKey = strings.TrimSpace(risk.ScopeKey)
	risk.RiskType = strings.ToUpper(strings.TrimSpace(risk.RiskType))
	risk.Severity = strings.ToUpper(strings.TrimSpace(risk.Severity))
	risk.Summary = strings.TrimSpace(risk.Summary)
	risk.Source = strings.TrimSpace(risk.Source)
	risk.SourceReference = strings.TrimSpace(risk.SourceReference)
	if !oneOf(risk.Scope, "DEPOT", "DISTRICT", "ROUTE") || risk.ScopeKey == "" || len(risk.ScopeKey) > 120 {
		return domain.DisruptionRisk{}, fmt.Errorf("invalid: scope must be DEPOT, DISTRICT, or ROUTE with a scopeKey up to 120 characters")
	}
	if !oneOf(risk.RiskType, "HEAVY_RAIN", "FLOODING", "LANDSLIDE", "ROAD_CLOSURE", "ROAD_DAMAGE", "OTHER") {
		return domain.DisruptionRisk{}, fmt.Errorf("invalid: unsupported disruption risk type")
	}
	if !oneOf(risk.Severity, "LOW", "MEDIUM", "HIGH") {
		return domain.DisruptionRisk{}, fmt.Errorf("invalid: severity must be LOW, MEDIUM, or HIGH")
	}
	if risk.Summary == "" || len(risk.Summary) > 500 || risk.Source == "" || len(risk.Source) > 120 || len(risk.SourceReference) > 300 {
		return domain.DisruptionRisk{}, fmt.Errorf("invalid: summary, source, or source reference has invalid length")
	}
	if risk.Confidence < 0 || risk.Confidence > 1 || risk.Confidence != risk.Confidence {
		return domain.DisruptionRisk{}, fmt.Errorf("invalid: confidence must be between 0 and 1")
	}
	risk.CreatedBy = profile.UserID
	created, err := s.Repo.CreateDisruptionRisk(ctx, risk)
	if err != nil {
		return domain.DisruptionRisk{}, err
	}
	s.Peers.Publish(ctx, audit.ActionDisruptionRiskCreated, profile.UserID, "DISRUPTION_RISK", created.ID, map[string]any{
		"deliveryDate": created.DeliveryDate, "scope": created.Scope, "scopeKey": created.ScopeKey, "riskType": created.RiskType,
		"severity": created.Severity, "source": created.Source, "sourceReference": created.SourceReference, "confidence": created.Confidence,
	})
	return created, nil
}

func (s Service) OverrideDisruptionRisk(ctx context.Context, profile *authorization.Profile, riskID, decision, severity, reason string) (domain.DisruptionRisk, error) {
	if profile == nil || strings.TrimSpace(profile.UserID) == "" {
		return domain.DisruptionRisk{}, fmt.Errorf("forbidden: dispatcher profile required")
	}
	decision = strings.ToUpper(strings.TrimSpace(decision))
	severity = strings.ToUpper(strings.TrimSpace(severity))
	reason = strings.TrimSpace(reason)
	if !oneOf(decision, "ACKNOWLEDGED", "OVERRIDE", "DISMISSED") || len(reason) < 8 || len(reason) > 500 {
		return domain.DisruptionRisk{}, fmt.Errorf("invalid: decision and an 8–500 character reason are required")
	}
	if (decision == "OVERRIDE" && !oneOf(severity, "LOW", "MEDIUM", "HIGH")) || (decision != "OVERRIDE" && severity != "") {
		return domain.DisruptionRisk{}, fmt.Errorf("invalid: severity is required only for an OVERRIDE decision")
	}
	updated, err := s.Repo.AppendDisruptionRiskOverride(ctx, riskID, decision, severity, reason, profile.UserID)
	if err != nil {
		return domain.DisruptionRisk{}, err
	}
	s.Peers.Publish(ctx, audit.ActionDisruptionRiskOverridden, profile.UserID, "DISRUPTION_RISK", riskID, map[string]any{
		"decision": decision, "severityOverride": severity, "reason": reason,
	})
	return updated, nil
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func validDeliveryDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func (s Service) Create(ctx context.Context, profile *authorization.Profile, date string) (domain.Plan, bool, error) {
	if date == "" {
		return domain.Plan{}, false, fmt.Errorf("invalid: deliveryDate required")
	}
	if existing, err := s.Repo.GetByDate(ctx, date); err == nil {
		return existing, false, nil
	}
	actor := ""
	if profile != nil {
		actor = profile.UserID
	}
	pl, err := s.Repo.Create(ctx, date, actor)
	if err != nil {
		return domain.Plan{}, false, err
	}
	telemetry.PlansCreated.Inc()
	s.Peers.Publish(ctx, audit.ActionPlanCreated, actor, "PLAN", pl.PlanRef, map[string]any{"deliveryDate": date})
	return pl, true, nil
}

func (s Service) Get(ctx context.Context, id string) (map[string]any, error) {
	pl, err := s.Repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	return s.detail(ctx, pl)
}

func (s Service) GetByDate(ctx context.Context, date string) (map[string]any, error) {
	pl, err := s.Repo.GetByDate(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	return s.detail(ctx, pl)
}

func (s Service) Generate(ctx context.Context, profile *authorization.Profile, id string) (domain.GenerateResult, error) {
	pl, err := s.Repo.Get(ctx, id)
	if err != nil {
		return domain.GenerateResult{}, fmt.Errorf("not found")
	}
	if pl.Status == domain.StatusConfirmed {
		return domain.GenerateResult{}, fmt.Errorf("conflict: plan confirmed")
	}
	n, err := s.Repo.CountAllocations(ctx, pl.ID)
	if err != nil {
		return domain.GenerateResult{}, err
	}
	if n > 0 || pl.GeneratedAt != nil {
		return domain.GenerateResult{}, fmt.Errorf("conflict: generate already applied; reset first")
	}
	guard, ok := any(s.Repo).(interface {
		ClaimGeneration(context.Context, string) error
		ReleaseGeneration(context.Context, string) error
	})
	if !ok {
		return domain.GenerateResult{}, fmt.Errorf("generation guard unavailable")
	}
	if err := guard.ClaimGeneration(ctx, pl.ID); err != nil {
		return domain.GenerateResult{}, err
	}
	completed := false
	defer func() {
		if !completed {
			// Clear a reservation if generation failed before writing any rows;
			// keep it when partial allocations require an explicit reset.
			_ = guard.ReleaseGeneration(context.WithoutCancel(ctx), pl.ID)
		}
	}()
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return domain.GenerateResult{}, err
	}
	start := time.Now()
    allocWorld, deferralRequests := holdDispatcherDeferrals(world)
    out := allocate.Generate(allocWorld)
    out.Unallocated = append(out.Unallocated, deferralRequests...)
	telemetry.AllocationDuration.Observe(time.Since(start).Seconds())
	for _, as := range out.Assignments {
		if err := s.persistAssignment(ctx, pl.ID, as); err != nil {
			return domain.GenerateResult{}, err
		}
		telemetry.OrdersAllocated.Inc()
	}
	_ = s.Repo.SetStatus(ctx, pl.ID, domain.StatusValidated)
	for _, f := range out.Unallocated {
		telemetry.ConstraintFailures.WithLabelValues(f.ReasonCode).Inc()
	}
	if err := s.Repo.ReplaceUnallocatedReasons(ctx, pl.ID, out.Unallocated); err != nil {
		return domain.GenerateResult{}, err
	}
	completed = true
	telemetry.PlansGenerated.Inc()
	actor := actorID(profile)
	s.Peers.Publish(ctx, audit.ActionPlanGenerated, actor, "PLAN", pl.PlanRef, map[string]any{"allocated": len(out.Assignments), "unallocated": len(out.Unallocated)})
	return domain.GenerateResult{Allocated: len(out.Assignments), Unallocated: len(out.Unallocated), Failures: out.Unallocated, FairnessSignalAvailable: world.FairnessSignalAvailable, FairnessPolicy: allocate.FairnessPolicyWithPolicy(world.FairnessSignalAvailable, world.Policy)}, nil
}

// Simulate runs the deterministic allocator without persisting assignments or
// changing the plan state.
func (s Service) Simulate(ctx context.Context, id string) (domain.GenerateResult, error) {
	pl, err := s.Repo.Get(ctx, id)
	if err != nil {
		return domain.GenerateResult{}, fmt.Errorf("not found")
	}
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return domain.GenerateResult{}, err
	}
    allocWorld, deferralRequests := holdDispatcherDeferrals(world)
    out := allocate.Generate(allocWorld)
    out.Unallocated = append(out.Unallocated, deferralRequests...)
	return domain.GenerateResult{Allocated: len(out.Assignments), Unallocated: len(out.Unallocated), Failures: out.Unallocated, FairnessSignalAvailable: world.FairnessSignalAvailable, FairnessPolicy: allocate.FairnessPolicyWithPolicy(world.FairnessSignalAvailable, world.Policy)}, nil
}

func (s Service) Reset(ctx context.Context, profile *authorization.Profile, id string) error {
	pl, err := s.Repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("not found")
	}
	if pl.Status == domain.StatusConfirmed {
		return fmt.Errorf("conflict: plan confirmed")
	}
	return s.Repo.Reset(ctx, pl.ID)
}

// manualReasonBounds mirrors the deferral comment's practical length limits:
// long enough for a real explanation, short enough to stay an audit note.
const (
	manualReasonMinLen = 3
	manualReasonMaxLen = 500
)

// validManualReason requires a non-trivial, human-written explanation for a
// manual allocation override (FR-54): the dispatcher is overruling or
// pre-empting the deterministic allocator, so the override itself must be
// explainable, not just the hard constraints it still has to satisfy.
func validManualReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if len(trimmed) < manualReasonMinLen || len(trimmed) > manualReasonMaxLen {
		return fmt.Errorf("invalid: reason")
	}
	return nil
}

func (s Service) Assign(ctx context.Context, profile *authorization.Profile, planID, orderID, vehicleID string, tripNo int, reason string) (domain.Allocation, []domain.Result, error) {
	if err := validManualReason(reason); err != nil {
		return domain.Allocation{}, nil, err
	}
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return domain.Allocation{}, nil, fmt.Errorf("not found")
	}
	if pl.Status == domain.StatusConfirmed {
		return domain.Allocation{}, nil, fmt.Errorf("conflict: plan confirmed")
	}
	if tripNo != 1 && tripNo != 2 {
		return domain.Allocation{}, nil, fmt.Errorf("invalid: tripNumber")
	}
	fails, err := s.validateAssign(ctx, pl, orderID, vehicleID, tripNo, "")
	if err != nil {
		return domain.Allocation{}, nil, err
	}
	if len(fails) > 0 {
		return domain.Allocation{}, fails, fmt.Errorf("allocation_invalid")
	}
	as := allocate.Assignment{Order: domain.Order{ID: orderID}, VehicleID: vehicleID, TripNumber: tripNo}
	if err := s.persistAssignment(ctx, pl.ID, as); err != nil {
		return domain.Allocation{}, nil, err
	}
	telemetry.OrdersAllocated.Inc()
	s.Peers.Publish(ctx, audit.ActionOrderAllocated, actorID(profile), "ORDER", orderID, map[string]any{"vehicleId": vehicleID, "tripNumber": tripNo, "reason": strings.TrimSpace(reason)})
	allocs, _ := s.Repo.ListAllocations(ctx, pl.ID)
	for _, a := range allocs {
		if a.OrderID == orderID {
			return a, nil, nil
		}
	}
	return domain.Allocation{}, nil, nil
}

func (s Service) publication(ctx context.Context, pl domain.Plan) domain.Publication {
	if st, ok := any(s.Repo).(planVersionStore); ok && pl.CurrentVersion > 0 {
		out, _ := st.Publication(ctx, pl.ID)
		return out
	}
	return domain.Publication{Version: pl.CurrentVersion, Acknowledgements: []domain.PlanAcknowledgement{}, Reminders: []domain.PlanReminder{}}
}

func (s Service) Reassign(ctx context.Context, profile *authorization.Profile, planID, allocID, vehicleID string, tripNo int, reason string) ([]domain.Result, error) {
	if err := validManualReason(reason); err != nil {
		return nil, err
	}
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	if pl.Status == domain.StatusConfirmed {
		return nil, fmt.Errorf("conflict: plan confirmed; open a revision first")
	}
	a, err := s.Repo.GetAllocation(ctx, pl.ID, allocID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	fails, err := s.validateAssign(ctx, pl, a.OrderID, vehicleID, tripNo, a.ID)
	if err != nil {
		return nil, err
	}
	if len(fails) > 0 {
		return fails, fmt.Errorf("allocation_invalid")
	}
	_ = s.Repo.DeleteAllocation(ctx, pl.ID, allocID)
	if err := s.persistAssignment(ctx, pl.ID, allocate.Assignment{Order: domain.Order{ID: a.OrderID}, VehicleID: vehicleID, TripNumber: tripNo}); err != nil {
		return nil, err
	}
	s.Peers.Publish(ctx, audit.ActionOrderReallocated, actorID(profile), "ORDER", a.OrderID, map[string]any{"from": a.VehicleID, "to": vehicleID, "reason": strings.TrimSpace(reason)})
	return nil, nil
}

func (s Service) Remove(ctx context.Context, profile *authorization.Profile, planID, allocID string) error {
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return fmt.Errorf("not found")
	}
	if pl.Status == domain.StatusConfirmed {
		return fmt.Errorf("conflict: plan confirmed; open a revision first")
	}
	a, err := s.Repo.GetAllocation(ctx, pl.ID, allocID)
	if err != nil {
		return fmt.Errorf("not found")
	}
	if err := s.Repo.DeleteAllocation(ctx, pl.ID, allocID); err != nil {
		return err
	}
	s.Peers.Publish(ctx, audit.ActionAllocationRemoved, actorID(profile), "ORDER", a.OrderID, map[string]any{"vehicleId": a.VehicleID})
	return nil
}

func (s Service) Defer(ctx context.Context, profile *authorization.Profile, planID, orderID, code, comment, nextRunTarget string) error {
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return fmt.Errorf("not found")
	}
	if pl.Status == domain.StatusConfirmed {
		return fmt.Errorf("conflict: plan confirmed; open a revision first")
	}
	if !validReasons[code] {
		return fmt.Errorf("invalid: reasonCode")
	}
	// W3: a deferral will not save without a reason, a decider and a next run.
	var vErr error
	if nextRunTarget, vErr = validateDeferral(actorID(profile), code, nextRunTarget, pl.DeliveryDate); vErr != nil {
		return vErr
	}
	allocs, _ := s.Repo.ListAllocations(ctx, pl.ID)
	for _, a := range allocs {
		if a.OrderID == orderID {
			return fmt.Errorf("conflict: order allocated")
		}
	}
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return err
	}
	var outletID string
	var orderRef string
	found := false
	for _, o := range world.Orders {
		if o.ID == orderID || o.OrderRef == orderID {
			found = true
			outletID = o.OutletID
			orderRef = o.OrderRef
			orderID = o.ID
			break
		}
	}
	if !found {
		return fmt.Errorf("not found: order")
	}
	if err := s.Repo.InsertDeferral(ctx, domain.Deferral{
		PlanID: pl.ID, OrderID: orderID, OutletID: outletID, ReasonCode: code,
		Comment: comment, DeferredBy: actorID(profile), NextRunTarget: nextRunTarget,
	}); err != nil {
		return err
	}
	telemetry.OrdersDeferred.Inc()
	s.Peers.Publish(ctx, audit.ActionOrderDeferred, actorID(profile), "ORDER", orderID, map[string]any{"reasonCode": code, "nextRunTarget": nextRunTarget})
	if err := s.Peers.QueueNotification(ctx, "deferral:"+pl.ID+":"+orderID, outletID, "DEFERRAL", orderRef, code, 0, nextRunTarget); err != nil && s.Peers.Logger != nil {
		s.Peers.Logger.Error("notification_enqueue_failed", "event", "deferral", "order_id", orderID, "error", err)
	}
	return nil
}

func (s Service) Confirm(ctx context.Context, profile *authorization.Profile, id string) error {
	pl, err := s.Repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("not found")
	}
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return err
	}
	allocs, _ := s.Repo.ListAllocations(ctx, pl.ID)
	deferred, _ := s.Repo.ListDeferrals(ctx, pl.ID)
	got := map[string]bool{}
	for _, a := range allocs {
		got[a.OrderID] = true
	}
	for _, d := range deferred {
		got[d.OrderID] = true
	}
	for _, o := range world.Orders {
		if !got[o.ID] {
			return fmt.Errorf("conflict: unresolved orders")
		}
	}
	if fails := s.revalidateAllocations(ctx, pl, world, allocs); len(fails) > 0 {
		return fmt.Errorf("conflict: allocation_invalid")
	}
	publicationStore, ok := any(s.Repo).(planVersionStore)
	if !ok {
		return fmt.Errorf("plan publication store unavailable")
	}
	instructions := struct {
		DeliveryDate string              `json:"deliveryDate"`
		Allocations  []domain.Allocation `json:"allocations"`
		Deferrals    []domain.Deferral   `json:"deferrals"`
	}{DeliveryDate: pl.DeliveryDate, Allocations: allocs, Deferrals: deferred}
	canonical, err := json.Marshal(instructions)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(canonical)
	pub, err := publicationStore.Publish(ctx, pl.ID, actorID(profile), hex.EncodeToString(digest[:]))
	if err != nil {
		return err
	}
	s.Peers.Publish(ctx, audit.ActionPlanConfirmed, actorID(profile), "PLAN", pl.PlanRef, map[string]any{"deliveryDate": pl.DeliveryDate, "version": pub.Version, "contentHash": pub.ContentHash})
	return nil
}

// ReminderWindow is how long a reminder for the same trip, audience and plan
// version suppresses another one.
const ReminderWindow = 5 * time.Minute

// Acknowledge records the caller's receipt of the current published version.
// tripID identifies the trip being acknowledged; it is empty only for legacy
// plan-level callers. A driver may acknowledge only a trip of their vehicle and
// a loader only a trip of their depot.
func (s Service) Acknowledge(ctx context.Context, profile *authorization.Profile, planID string, version int, tripID string) error {
	if profile == nil || len(profile.Roles) == 0 || profile.UserID == "" {
		return fmt.Errorf("forbidden")
	}
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return fmt.Errorf("not found")
	}
	if pl.Status != domain.StatusConfirmed {
		return fmt.Errorf("conflict: plan not published")
	}
	store, ok := any(s.Repo).(planVersionStore)
	if !ok {
		return fmt.Errorf("plan publication store unavailable")
	}
	tripID = strings.TrimSpace(tripID)
	vehicleID := ""
	if tripID != "" {
		trip, err := s.Repo.GetTrip(ctx, tripID)
		if err != nil || trip.PlanID != pl.ID {
			return fmt.Errorf("not found: trip is not part of this plan")
		}
		vehicleID = trip.VehicleID
		role := profile.Roles[0]
		switch role {
		case authorization.RoleDriver:
			if profile.VehicleID == "" || !strings.EqualFold(profile.VehicleID, trip.VehicleID) {
				return fmt.Errorf("forbidden: driver may only acknowledge trips for their assigned vehicle")
			}
		case authorization.RoleLoader:
			if profile.Depot != "" {
				vehicle, err := s.Peers.Vehicle(ctx, trip.VehicleID)
				if err != nil {
					return fmt.Errorf("conflict: trip depot unavailable")
				}
				if !strings.EqualFold(vehicle.HomeDepot, profile.Depot) {
					return fmt.Errorf("forbidden: loader may only acknowledge trips for their depot")
				}
			}
		}
	}
	return store.Acknowledge(ctx, pl.ID, version, profile.UserID, profile.Roles[0], tripID, vehicleID)
}

// Remind records a dispatcher reminder asking a trip's driver or loader to
// acknowledge the current plan version, audits it and tells the recipient.
// A repeat within ReminderWindow is a no-op that reports the earlier reminder.
func (s Service) Remind(ctx context.Context, profile *authorization.Profile, planID, tripID, audience string) (domain.ReminderResult, error) {
	if profile == nil || strings.TrimSpace(profile.UserID) == "" {
		return domain.ReminderResult{}, fmt.Errorf("forbidden: dispatcher profile required")
	}
	audience = strings.ToUpper(strings.TrimSpace(audience))
	tripID = strings.TrimSpace(tripID)
	if tripID == "" || !oneOf(audience, "DRIVER", "LOADER") {
		return domain.ReminderResult{}, fmt.Errorf("invalid: tripId and audience DRIVER or LOADER are required")
	}
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return domain.ReminderResult{}, fmt.Errorf("not found")
	}
	if pl.Status != domain.StatusConfirmed || pl.CurrentVersion < 1 {
		return domain.ReminderResult{}, fmt.Errorf("conflict: plan not published")
	}
	trip, err := s.Repo.GetTrip(ctx, tripID)
	if err != nil || trip.PlanID != pl.ID {
		return domain.ReminderResult{}, fmt.Errorf("not found: trip is not part of this plan")
	}
	store, ok := any(s.Repo).(planVersionStore)
	if !ok {
		return domain.ReminderResult{}, fmt.Errorf("plan publication store unavailable")
	}
	role := authorization.RoleDriver
	if audience == "LOADER" {
		role = authorization.RoleLoader
	}
	pub := s.publication(ctx, pl)
	for _, a := range pub.Acknowledgements {
		if a.ActorRole == role && a.TripID == tripID {
			return domain.ReminderResult{}, fmt.Errorf("conflict: %s already acknowledged this trip", strings.ToLower(audience))
		}
	}
	reminder, already, err := store.RecordReminder(ctx, pl.ID, pl.CurrentVersion, tripID, audience, profile.UserID, ReminderWindow)
	if err != nil {
		return domain.ReminderResult{}, err
	}
	result := domain.ReminderResult{Reminder: reminder, AlreadySent: already}
	if already {
		return result, nil
	}
	s.Peers.Publish(ctx, audit.ActionPlanAckReminderSent, profile.UserID, "PLAN", pl.PlanRef, map[string]any{
		"tripId": tripID, "vehicleId": trip.VehicleID, "tripNumber": trip.TripNumber, "audience": audience, "version": pl.CurrentVersion,
	})
	if audience == "DRIVER" {
		body := fmt.Sprintf("Dispatch reminder: open this trip and acknowledge plan %s version %d before starting.", pl.PlanRef, pl.CurrentVersion)
		if err := s.Peers.SendTripMessage(ctx, tripID, body); err != nil {
			result.TripMessage = "unavailable"
		} else {
			result.TripMessage = "sent"
		}
	}
	return result, nil
}

// Reminders lists the reminders addressed to the caller's role for a trip's
// current plan version, so the driver and loader apps can show them.
func (s Service) Reminders(ctx context.Context, profile *authorization.Profile, planID, tripID string) ([]domain.PlanReminder, error) {
	if profile == nil || len(profile.Roles) == 0 {
		return nil, fmt.Errorf("forbidden")
	}
	audience := ""
	switch profile.Roles[0] {
	case authorization.RoleDriver:
		audience = "DRIVER"
	case authorization.RoleLoader:
		audience = "LOADER"
	default:
		return nil, fmt.Errorf("forbidden: field role required")
	}
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	out := []domain.PlanReminder{}
	if pl.Status != domain.StatusConfirmed || pl.CurrentVersion < 1 {
		return out, nil
	}
	tripID = strings.TrimSpace(tripID)
	for _, r := range s.publication(ctx, pl).Reminders {
		if r.Audience == audience && (tripID == "" || r.TripID == tripID) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s Service) Revise(ctx context.Context, profile *authorization.Profile, planID string) error {
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return fmt.Errorf("not found")
	}
	if pl.Status != domain.StatusConfirmed {
		return fmt.Errorf("conflict: plan is not published")
	}
	store, ok := any(s.Repo).(planVersionStore)
	if !ok {
		return fmt.Errorf("plan publication store unavailable")
	}
	return store.Revise(ctx, pl.ID)
}

func (s Service) BreakdownProposals(ctx context.Context, planID, vehicleID string) (map[string]any, error) {
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return nil, err
	}
	allocs, err := s.Repo.ListAllocations(ctx, pl.ID)
	if err != nil {
		return nil, err
	}
	trips, err := s.Repo.ListTrips(ctx, pl.ID)
	if err != nil {
		return nil, err
	}
	tripNumbers := map[string]int{}
	for _, tr := range trips {
		tripNumbers[tr.ID] = tr.TripNumber
	}
	orders := map[string]domain.Order{}
	for _, o := range world.Orders {
		orders[o.ID] = o
	}
	groups := map[int][]domain.Allocation{}
	for _, a := range allocs {
		if a.VehicleID == vehicleID {
			groups[tripNumbers[a.TripID]] = append(groups[tripNumbers[a.TripID]], a)
		}
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("not found: vehicle has no assigned stops on this plan")
	}
	tripNos := make([]int, 0, len(groups))
	for n := range groups {
		tripNos = append(tripNos, n)
	}
	// Vehicles and stops are already stable-ordered by the planner. Sort trip numbers for deterministic previews.
	for i := 0; i < len(tripNos); i++ {
		for j := i + 1; j < len(tripNos); j++ {
			if tripNos[j] < tripNos[i] {
				tripNos[i], tripNos[j] = tripNos[j], tripNos[i]
			}
		}
	}
	var proposals []map[string]any
	critical := false
	for _, tn := range tripNos {
		moving := groups[tn]
		for i := 0; i < len(moving); i++ {
			for j := i + 1; j < len(moving); j++ {
				if moving[j].Sequence < moving[i].Sequence {
					moving[i], moving[j] = moving[j], moving[i]
				}
			}
		}
		var options []map[string]any
		for _, candidate := range world.Vehicles {
			if candidate.ID == vehicleID || !strings.EqualFold(candidate.Status, "available") {
				continue
			}
			state := domain.TripState{VehicleID: candidate.ID, TripNumber: tn}
			var failures []domain.Result
			// Rebuild the target trip without its current stops, then simulate the complete affected trip in stop order.
			for _, a := range allocs {
				if a.VehicleID != candidate.ID || tripNumbers[a.TripID] != tn {
					continue
				}
				o, ok := orders[a.OrderID]
				if !ok {
					continue
				}
				next, _, _, appendErr := schedule.AppendStop(state, o, candidate, schedule.PlanDate(pl.DeliveryDate), world.Est, world.Outlets)
				if appendErr == nil {
					state = next
				}
			}
			for _, a := range moving {
				o, ok := orders[a.OrderID]
				if !ok {
					failures = append(failures, domain.Result{ReasonCode: domain.ReasonNoEligibleVehicle})
					continue
				}
				fails := constraints.All(o, candidate, state, pl.DeliveryDate, world.Est, world.Outlets, 0)
				if len(fails) > 0 {
					failures = append(failures, fails...)
					continue
				}
				next, _, _, appendErr := schedule.AppendStop(state, o, candidate, schedule.PlanDate(pl.DeliveryDate), world.Est, world.Outlets)
				if appendErr != nil {
					failures = append(failures, domain.Result{ReasonCode: domain.ReasonDeliveryWindow})
					continue
				}
				state = next
			}
			options = append(options, map[string]any{"vehicleId": candidate.ID, "valid": len(failures) == 0, "failures": failures, "projectedStops": len(state.Stops)})
		}
		ranked := rankStrandedStops(moving, orders, world.Outlets)
		ordersOut := make([]map[string]any, 0, len(ranked))
		tripChilled := false
		for i, r := range ranked {
			tripChilled = tripChilled || r.Chilled
			ordersOut = append(ordersOut, map[string]any{"allocationId": r.Alloc.ID, "orderId": r.Alloc.OrderID, "orderRef": r.Order.OrderRef, "outletId": r.Order.OutletID, "stopSequence": r.Alloc.Sequence, "urgencyRank": i + 1, "chilled": r.Chilled, "windowClose": r.WindowClose})
		}
		if tripChilled {
			critical = true
		}
		proposals = append(proposals, map[string]any{"tripNumber": tn, "stops": ordersOut, "options": options, "chilledOnBoard": tripChilled})
	}
	severity := "high"
	if critical {
		severity = "critical"
	}
	return map[string]any{"planId": pl.ID, "vehicleId": vehicleID, "items": proposals, "confirmed": false, "critical": critical, "severity": severity}, nil
}

func (s Service) ConfirmBreakdown(ctx context.Context, profile *authorization.Profile, planID, sourceVehicle, targetVehicle string, tripNo int) (map[string]any, error) {
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	versionStore, ok := any(s.Repo).(planVersionStore)
	if !ok {
		return nil, fmt.Errorf("plan publication store unavailable")
	}
	if pl.Status == domain.StatusDraft {
		return nil, fmt.Errorf("conflict: generate or validate this plan before applying a breakdown")
	}
	preview, err := s.BreakdownProposals(ctx, pl.ID, sourceVehicle)
	if err != nil {
		return nil, err
	}
	items, _ := preview["items"].([]map[string]any)
	var selected map[string]any
	for _, item := range items {
		if item["tripNumber"] == tripNo {
			selected = item
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("not found: affected trip")
	}
	options, _ := selected["options"].([]map[string]any)
	feasible := false
	for _, option := range options {
		if option["vehicleId"] == targetVehicle && option["valid"] == true {
			feasible = true
		}
	}
	if !feasible {
		return nil, fmt.Errorf("conflict: replacement vehicle failed one or more hard constraints")
	}
	// The published plan stays untouched until the replacement has passed the
	// same feasibility checks shown to the dispatcher and every in-memory step
	// below has succeeded.
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return nil, err
	}
	allocs, err := s.Repo.ListAllocations(ctx, pl.ID)
	if err != nil {
		return nil, err
	}
	trips, err := s.Repo.ListTrips(ctx, pl.ID)
	if err != nil {
		return nil, err
	}
	tripNumber := map[string]int{}
	for _, tr := range trips {
		tripNumber[tr.ID] = tr.TripNumber
	}
	orders := map[string]domain.Order{}
	for _, o := range world.Orders {
		orders[o.ID] = o
	}
	vehicles := map[string]domain.Vehicle{}
	for _, v := range world.Vehicles {
		vehicles[v.ID] = v
	}
	target := vehicles[targetVehicle]
	state := domain.TripState{VehicleID: targetVehicle, TripNumber: tripNo}
	for _, a := range allocs {
		if a.VehicleID != targetVehicle || tripNumber[a.TripID] != tripNo {
			continue
		}
		next, _, _, e := schedule.AppendStop(state, orders[a.OrderID], target, schedule.PlanDate(pl.DeliveryDate), world.Est, world.Outlets)
		if e == nil {
			state = next
		}
	}
	var moved []domain.Allocation
	var notices []map[string]any
	for _, a := range allocs {
		if a.VehicleID != sourceVehicle || tripNumber[a.TripID] != tripNo {
			continue
		}
		o := orders[a.OrderID]
		next, _, _, e := schedule.AppendStop(state, o, target, schedule.PlanDate(pl.DeliveryDate), world.Est, world.Outlets)
		if e != nil {
			return nil, fmt.Errorf("conflict: replacement schedule could not be built")
		}
		state = next
		stop := state.Stops[len(state.Stops)-1]
		a.PlannedArrivalAt = &stop.Arrival
		a.PlannedServiceStartAt = &stop.ServiceStart
		a.PlannedDepartureAt = &stop.Depart
		moved = append(moved, a)
		notices = append(notices, map[string]any{"orderRef": o.OrderRef, "outletId": o.OutletID, "estimatedArrival": stop.Arrival})
	}
	if len(moved) == 0 {
		return nil, fmt.Errorf("not found: affected trip")
	}
	// The broken vehicle must be out of the planning pool before the revision is
	// committed, and the recovery is not complete without it. The fleet update
	// is an upsert, so repeating it on a retry is harmless. If fleet-service is
	// unreachable nothing has been committed yet, so the dispatcher can retry
	// the same request without a half-applied plan version or duplicate audit.
	if err := s.Peers.SetVehicleWorkshop(ctx, sourceVehicle, pl.DeliveryDate, "breakdown recovery for plan "+pl.PlanRef); err != nil {
		if s.Peers.Logger != nil {
			s.Peers.Logger.Error("breakdown_workshop_failed", "vehicle_id", sourceVehicle, "error", err)
		}
		return nil, fmt.Errorf("%s: vehicle %s could not be marked in the workshop, so the recovery is not complete and the plan was not changed; retry the same request", ErrCodeWorkshopPending, sourceVehicle)
	}
	// Keep the published plan intact until the replacement is feasible and the
	// vehicle is in the workshop.
	if pl.Status == domain.StatusConfirmed {
		if err := versionStore.Revise(ctx, pl.ID); err != nil {
			return nil, err
		}
	}
	if err := versionStore.MoveTripAllocations(ctx, pl.ID, sourceVehicle, targetVehicle, tripNo, moved); err != nil {
		return nil, err
	}
	if err := s.Confirm(ctx, profile, pl.ID); err != nil {
		return map[string]any{"status": "reassignment_saved_plan_not_published", "noticeDrafts": notices}, err
	}
	updated, _ := s.Repo.Get(ctx, pl.ID)
	s.Peers.Publish(ctx, audit.ActionBreakdownRecoveryConfirmed, actorID(profile), "PLAN", pl.PlanRef, map[string]any{"decidedBy": actorID(profile), "sourceVehicleId": sourceVehicle, "replacementVehicleId": targetVehicle, "tripNumber": tripNo, "version": updated.CurrentVersion, "movedStops": len(moved), "vehicleInWorkshop": true})
	s.Peers.Publish(ctx, audit.ActionPlanConfirmed, actorID(profile), "PLAN", pl.PlanRef, map[string]any{"reason": "vehicle_breakdown", "sourceVehicleId": sourceVehicle, "replacementVehicleId": targetVehicle, "tripNumber": tripNo, "version": updated.CurrentVersion})
	oldByOrder := make(map[string]domain.Allocation, len(allocs))
	for _, a := range allocs {
		oldByOrder[a.OrderID] = a
	}
	for _, a := range moved {
		old := oldByOrder[a.OrderID]
		if old.PlannedArrivalAt == nil || a.PlannedArrivalAt == nil {
			continue
		}
		delay := majorDelayMinutes(old.PlannedArrivalAt, a.PlannedArrivalAt)
		if delay < 30 {
			continue
		}
		o := orders[a.OrderID]
		if err := s.Peers.QueueNotification(ctx, fmt.Sprintf("breakdown:%s:%d:%s", pl.ID, updated.CurrentVersion, a.OrderID), o.OutletID, "MAJOR_DELAY", o.OrderRef, "", delay, ""); err != nil && s.Peers.Logger != nil {
			s.Peers.Logger.Error("notification_enqueue_failed", "event", "major_delay", "order_id", a.OrderID, "error", err)
		}
	}
	return map[string]any{"status": "confirmed", "planVersion": updated.CurrentVersion, "sourceVehicleId": sourceVehicle, "replacementVehicleId": targetVehicle, "tripNumber": tripNo, "movedStops": len(moved), "noticeDrafts": notices, "vehicleInWorkshop": true, "decidedBy": actorID(profile)}, nil
}

func majorDelayMinutes(previous, next *time.Time) int {
	if previous == nil || next == nil {
		return 0
	}
	minutes := int(next.Sub(*previous).Minutes())
	if minutes < 0 {
		return 0
	}
	return minutes
}

func (s Service) persistAssignment(ctx context.Context, planID string, as allocate.Assignment) error {
	if as.Arrival.IsZero() || as.ServiceStart.IsZero() || as.Departure.IsZero() {
		pl, err := s.Repo.Get(ctx, planID)
		if err != nil {
			return err
		}
		world, err := s.loadWorld(ctx, pl)
		if err != nil {
			return err
		}
		for _, order := range world.Orders {
			if order.ID == as.Order.ID {
				as.Order = order
				break
			}
		}
		var vehicle domain.Vehicle
		for _, candidate := range world.Vehicles {
			if candidate.ID == as.VehicleID {
				vehicle = candidate
				break
			}
		}
		trips, err := s.Repo.ListTrips(ctx, planID)
		if err != nil {
			return err
		}
		tripNumbers := make(map[string]int, len(trips))
		for _, trip := range trips {
			tripNumbers[trip.ID] = trip.TripNumber
		}
		allocations, err := s.Repo.ListAllocations(ctx, planID)
		if err != nil {
			return err
		}
		state := domain.TripState{VehicleID: as.VehicleID, TripNumber: as.TripNumber}
		for _, allocation := range allocations {
			if allocation.VehicleID != as.VehicleID || tripNumbers[allocation.TripID] != as.TripNumber || allocation.OrderID == as.Order.ID {
				continue
			}
			for _, order := range world.Orders {
				if order.ID == allocation.OrderID {
					state, _, _, _ = schedule.AppendStop(state, order, vehicle, schedule.PlanDate(pl.DeliveryDate), world.Est, world.Outlets)
					break
				}
			}
		}
		planned, _, _, err := schedule.AppendStop(state, as.Order, vehicle, schedule.PlanDate(pl.DeliveryDate), world.Est, world.Outlets)
		if err != nil {
			return err
		}
		last := planned.Stops[len(planned.Stops)-1]
		as.Arrival, as.ServiceStart, as.Departure = last.Arrival, last.ServiceStart, last.Depart
	}
	trip, err := s.Repo.EnsureTrip(ctx, planID, as.VehicleID, as.TripNumber)
	if err != nil {
		return err
	}
	seq, err := s.Repo.NextSeq(ctx, trip.ID)
	if err != nil {
		return err
	}
	_, err = s.Repo.InsertAllocation(ctx, domain.Allocation{
		PlanID: planID, OrderID: as.Order.ID, TripID: trip.ID, VehicleID: as.VehicleID, Sequence: seq,
		PlannedArrivalAt: &as.Arrival, PlannedServiceStartAt: &as.ServiceStart, PlannedDepartureAt: &as.Departure,
	})
	return err
}

func (s Service) validateAssign(ctx context.Context, pl domain.Plan, orderID, vehicleID string, tripNo int, skipAlloc string) ([]domain.Result, error) {
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return nil, err
	}
	if world.Policy.MaxTripsPerVehicle > 0 && tripNo > world.Policy.MaxTripsPerVehicle {
		return []domain.Result{constraints.TripLimit(tripNo)}, fmt.Errorf("allocation_invalid")
	}
	var order domain.Order
	for _, o := range world.Orders {
		if o.ID == orderID || o.OrderRef == orderID {
			order = o
			break
		}
	}
	if order.ID == "" {
		return nil, fmt.Errorf("not found: order")
	}
	var veh domain.Vehicle
	for _, v := range world.Vehicles {
		if v.ID == vehicleID {
			veh = v
			break
		}
	}
	if veh.ID == "" {
		return nil, fmt.Errorf("not found: vehicle")
	}
	allocs, _ := s.Repo.ListAllocations(ctx, pl.ID)
	state := domain.TripState{VehicleID: vehicleID, TripNumber: tripNo}
	for _, a := range allocs {
		if a.ID == skipAlloc || a.VehicleID != vehicleID {
			continue
		}
		// same trip only if we can match trip number via trip list
		trips, _ := s.Repo.ListTrips(ctx, pl.ID)
		tn := 1
		for _, tr := range trips {
			if tr.ID == a.TripID {
				tn = tr.TripNumber
			}
		}
		if tn != tripNo {
			continue
		}
		var o domain.Order
		for _, cand := range world.Orders {
			if cand.ID == a.OrderID {
				o = cand
			}
		}
		day := schedule.PlanDate(pl.DeliveryDate)
		state, _, _, _ = schedule.AppendStop(state, o, veh, day, world.Est, world.Outlets)
	}
	return constraints.All(order, veh, state, pl.DeliveryDate, world.Est, world.Outlets, 0), nil
}

func (s Service) loadWorld(ctx context.Context, pl domain.Plan) (allocate.Input, error) {
	orders, err := s.Peers.Orders(ctx, pl.DeliveryDate)
	if err != nil {
		return allocate.Input{}, err
	}
	outlets, err := s.Peers.Outlets(ctx)
	if err != nil {
		return allocate.Input{}, err
	}
	vehicles, err := s.Peers.Vehicles(ctx, pl.DeliveryDate)
	if err != nil {
		return allocate.Input{}, err
	}
	counts, countsErr := s.Repo.OutletDeferralCounts(ctx)
	lastServed, lastServedErr := s.Peers.OutletLastServed(ctx)
	lastAttempted, lastAttemptedErr := s.Peers.OutletLastAttempted(ctx, pl.DeliveryDate)
	if lastAttemptedErr != nil {
		lastAttempted = map[string]time.Time{}
		if s.Peers.Logger != nil {
			s.Peers.Logger.Warn("outlet_last_attempted_unavailable", "error", lastAttemptedErr)
		}
	}
	lastDeferralByOutlet, lastDeferralErr := s.Repo.LatestDeferralsByOutlet(ctx, pl.DeliveryDate)
	if lastDeferralErr != nil {
		lastDeferralByOutlet = map[string]string{}
		if s.Peers.Logger != nil {
			s.Peers.Logger.Warn("repeat_deferral_signal_unavailable", "error", lastDeferralErr)
		}
	}
	earlierDeferralByOutlet, earlierDeferralErr := s.Repo.EarlierDeferralsByOutlet(ctx, pl.DeliveryDate)
	if earlierDeferralErr != nil {
		earlierDeferralByOutlet = map[string]string{}
		if s.Peers.Logger != nil {
			s.Peers.Logger.Warn("repeat_deferral_history_unavailable", "error", earlierDeferralErr)
		}
	}
	policy, policyErr := s.Peers.PlanningPolicy(ctx)
	policySignalAvailable := policyErr == nil
	if policyErr != nil {
		policy = domain.PlanningPolicy{Version: 1, DeferralWeightPoints: allocate.DeferralFairnessWeightDays, MaxDeferralCount: allocate.MaxFairnessDeferrals, MaxUnservedDays: allocate.MaxDaysSinceLastServed, MaxTripsPerVehicle: 2, CutoffLocalTime: "16:00:00"}
		if s.Peers.Logger != nil {
			s.Peers.Logger.Warn("planning_policy_unavailable_using_safe_defaults", "error", policyErr)
		}
	}
	fairnessSignalAvailable, err := applyFairnessHistory(orders, counts, countsErr, lastServed, lastServedErr, schedule.PlanDate(pl.DeliveryDate), policy, lastDeferralByOutlet, earlierDeferralByOutlet, lastAttempted)
	if err != nil {
		return allocate.Input{}, err
	}
	if lastServedErr != nil && s.Peers.Logger != nil {
		s.Peers.Logger.Warn("last_served_signal_unavailable", "error", lastServedErr)
	}
	for i := range orders {
		if o, ok := outlets[orders[i].OutletID]; ok {
			orders[i].Outlet = o
			orders[i].Brand = o.Brand
		}
	}
	est := s.Peers.Estimator(ctx)
	fuelSignalAvailable := s.applyWeekFuel(ctx, pl, vehicles, outlets, est)
	return allocate.Input{Date: pl.DeliveryDate, Orders: orders, Vehicles: vehicles, Outlets: outlets, Est: est, FairnessSignalAvailable: fairnessSignalAvailable, FuelLedgerAvailable: fuelSignalAvailable, Policy: policy, PolicySignalAvailable: policySignalAvailable}, nil
}

func (s Service) applyWeekFuel(ctx context.Context, pl domain.Plan, vehicles []domain.Vehicle, outlets map[string]domain.Outlet, est travel.Estimator) bool {
	day := schedule.PlanDate(pl.DeliveryDate)
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	from := day.AddDate(0, 0, -(weekday - 1))
	to := from.AddDate(0, 0, 6)
	others, err := s.Repo.ConfirmedPlansInWeek(ctx, from.Format("2006-01-02"), to.Format("2006-01-02"), pl.ID)
	if err != nil {
		others = nil
	}
	used := map[string]float64{}
	vehByID := map[string]domain.Vehicle{}
	for _, v := range vehicles {
		vehByID[v.ID] = v
	}
	for _, op := range others {
		orders, err := s.Peers.Orders(ctx, op.DeliveryDate)
		if err != nil {
			continue
		}
		byID := map[string]domain.Order{}
		for _, o := range orders {
			if out, ok := outlets[o.OutletID]; ok {
				o.Outlet = out
			}
			byID[o.ID] = o
		}
		allocs, _ := s.Repo.ListAllocations(ctx, op.ID)
		trips, _ := s.Repo.ListTrips(ctx, op.ID)
		tripNo := map[string]int{}
		for _, tr := range trips {
			tripNo[tr.ID] = tr.TripNumber
		}
		states := map[string]domain.TripState{}
		for _, a := range allocs {
			o, ok := byID[a.OrderID]
			if !ok {
				continue
			}
			v := vehByID[a.VehicleID]
			key := a.VehicleID + "|" + fmt.Sprintf("%d", tripNo[a.TripID])
			st := states[key]
			st.VehicleID = a.VehicleID
			st.TripNumber = tripNo[a.TripID]
			next, _, _, _ := schedule.AppendStop(st, o, v, schedule.PlanDate(op.DeliveryDate), est, outlets)
			states[key] = next
		}
		for key, st := range states {
			vid := strings.Split(key, "|")[0]
			if v, ok := vehByID[vid]; ok {
				used[vid] += schedule.FuelLiters(schedule.TripDistanceKm(st, v, outlets, est), v.KmPerL)
			}
		}
	}
	for i := range vehicles {
		vehicles[i].WeekFuelPlannedL = used[vehicles[i].ID]
	}
	actual, actualErr := s.Peers.ActualFuelByVehicle(ctx, pl.DeliveryDate)
	if actualErr != nil {
		if s.Peers.Logger != nil {
			s.Peers.Logger.Warn("weekly_actual_fuel_unavailable", "error", actualErr)
		}
		actual = map[string]float64{}
	}
	for i := range vehicles {
		vehicles[i].WeekFuelActualL = actual[vehicles[i].ID]
		vehicles[i].WeekFuelUsedL = vehicles[i].WeekFuelPlannedL
		if vehicles[i].WeekFuelActualL > vehicles[i].WeekFuelUsedL {
			vehicles[i].WeekFuelUsedL = vehicles[i].WeekFuelActualL
		}
	}
	return actualErr == nil && err == nil
}

func currentPlanFuel(pl domain.Plan, world allocate.Input, allocs []domain.Allocation, trips []domain.Trip) map[string]float64 {
	orders := make(map[string]domain.Order, len(world.Orders))
	vehicles := make(map[string]domain.Vehicle, len(world.Vehicles))
	tripNumbers := make(map[string]int, len(trips))
	for _, order := range world.Orders {
		orders[order.ID] = order
	}
	for _, vehicle := range world.Vehicles {
		vehicles[vehicle.ID] = vehicle
	}
	for _, trip := range trips {
		tripNumbers[trip.ID] = trip.TripNumber
	}
	states := make(map[string]domain.TripState)
	for _, allocation := range allocs {
		order, orderOK := orders[allocation.OrderID]
		vehicle, vehicleOK := vehicles[allocation.VehicleID]
		if !orderOK || !vehicleOK {
			continue
		}
		tripNumber := tripNumbers[allocation.TripID]
		if tripNumber == 0 {
			tripNumber = 1
		}
		key := allocation.VehicleID + "|" + fmt.Sprintf("%d", tripNumber)
		state := states[key]
		state.VehicleID = allocation.VehicleID
		state.TripNumber = tripNumber
		next, _, _, _ := schedule.AppendStop(state, order, vehicle, schedule.PlanDate(pl.DeliveryDate), world.Est, world.Outlets)
		states[key] = next
	}
	used := make(map[string]float64)
	for key, state := range states {
		vehicleID := strings.Split(key, "|")[0]
		vehicle := vehicles[vehicleID]
		used[vehicleID] += schedule.FuelLiters(schedule.TripDistanceKm(state, vehicle, world.Outlets, world.Est), vehicle.KmPerL)
	}
	return used
}

func (s Service) revalidateAllocations(ctx context.Context, pl domain.Plan, world allocate.Input, allocs []domain.Allocation) []domain.Result {
	trips, _ := s.Repo.ListTrips(ctx, pl.ID)
	tripNo := map[string]int{}
	for _, tr := range trips {
		tripNo[tr.ID] = tr.TripNumber
	}
	states := map[string]domain.TripState{}
	var bad []domain.Result
	for _, a := range allocs {
		var order domain.Order
		for _, o := range world.Orders {
			if o.ID == a.OrderID {
				order = o
				break
			}
		}
		var veh domain.Vehicle
		for _, v := range world.Vehicles {
			if v.ID == a.VehicleID {
				veh = v
				break
			}
		}
		tn := tripNo[a.TripID]
		if tn == 0 {
			tn = 1
		}
		key := a.VehicleID + "|" + fmt.Sprintf("%d", tn)
		st := states[key]
		st.VehicleID = a.VehicleID
		st.TripNumber = tn
		fails := constraints.All(order, veh, st, pl.DeliveryDate, world.Est, world.Outlets, 0)
		if len(fails) > 0 {
			bad = append(bad, fails...)
			continue
		}
		next, _, _, _ := schedule.AppendStop(st, order, veh, schedule.PlanDate(pl.DeliveryDate), world.Est, world.Outlets)
		states[key] = next
	}
	return bad
}

func (s Service) detail(ctx context.Context, pl domain.Plan) (map[string]any, error) {
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return nil, err
	}
	allocs, _ := s.Repo.ListAllocations(ctx, pl.ID)
	trips, _ := s.Repo.ListTrips(ctx, pl.ID)
	planFuel := currentPlanFuel(pl, world, allocs, trips)
	for i := range world.Vehicles {
		world.Vehicles[i].PlanFuelL = planFuel[world.Vehicles[i].ID]
	}
	deferred, _ := s.Repo.ListDeferrals(ctx, pl.ID)
	allocated := map[string]bool{}
	for _, a := range allocs {
		allocated[a.OrderID] = true
	}
	deferredIDs := map[string]bool{}
	for _, d := range deferred {
		deferredIDs[d.OrderID] = true
	}
	markPriorityNextPlan(world.Orders, deferredIDs)
	reasons, reasonsErr := s.Repo.ListUnallocatedReasons(ctx, pl.ID)
	reasonsAvailable := reasonsErr == nil
	if !reasonsAvailable && s.Peers.Logger != nil {
		s.Peers.Logger.Warn("unallocated_reasons_unavailable", "plan_id", pl.ID, "error", reasonsErr)
	}
	var unalloc []map[string]any
	for _, o := range world.Orders {
		if allocated[o.ID] || deferredIDs[o.ID] {
			continue
		}
		// A persisted reason comes from the last successful generation. An
		// order added or changed afterwards (e.g. a late import) has none
		// yet, so it keeps the generic placeholder until regenerated - but
		// if the lookup itself failed, say so explicitly rather than
		// showing a specific-looking reason that may not be true.
		entry := map[string]any{"orderId": o.ID, "orderRef": o.OrderRef, "reasonCode": domain.ReasonNoEligibleVehicle}
		if !reasonsAvailable {
			entry["reasonCode"] = domain.ReasonUnavailable
		} else if r, ok := reasons[o.ID]; ok {
			entry["reasonCode"] = r.ReasonCode
			if len(r.Details) > 0 {
				entry["details"] = r.Details
			}
		}
		unalloc = append(unalloc, entry)
	}
	if unalloc == nil {
		unalloc = []map[string]any{}
	}
	var publication domain.Publication
	if versionStore, ok := any(s.Repo).(planVersionStore); ok && pl.CurrentVersion > 0 {
		publication, _ = versionStore.Publication(ctx, pl.ID)
	}
	return map[string]any{
		"plan": pl, "trips": trips, "allocations": allocs, "deferrals": deferred,
		"publication": publication,
		"unallocated": unalloc, "vehicles": world.Vehicles, "orders": world.Orders,
		"unallocatedReasonsAvailable": reasonsAvailable,
		"fairness":                    map[string]any{"signalAvailable": world.FairnessSignalAvailable, "policy": allocate.FairnessPolicyWithPolicy(world.FairnessSignalAvailable, world.Policy), "asOf": pl.DeliveryDate},
		"fuelLedgerAvailable":         world.FuelLedgerAvailable,
		"planningPolicy":              world.Policy, "policySignalAvailable": world.PolicySignalAvailable,
	}, nil
}

func (s Service) InternalTrips(ctx context.Context, date, depot string) ([]domain.InternalTrip, error) {
	plans, err := s.Repo.ListConfirmedByDate(ctx, date)
	if err != nil {
		return nil, err
	}
	var out []domain.InternalTrip
	for _, pl := range plans {
		trips, err := s.internalForPlan(ctx, pl, depot)
		if err != nil {
			return nil, err
		}
		out = append(out, trips...)
	}
	if out == nil {
		out = []domain.InternalTrip{}
	}
	return out, nil
}

func (s Service) InternalTrip(ctx context.Context, tripID string) (domain.InternalTrip, error) {
	tr, err := s.Repo.GetTrip(ctx, tripID)
	if err != nil {
		return domain.InternalTrip{}, fmt.Errorf("not found")
	}
	pl, err := s.Repo.Get(ctx, tr.PlanID)
	if err != nil || pl.Status != domain.StatusConfirmed {
		return domain.InternalTrip{}, fmt.Errorf("not found")
	}
	trips, err := s.internalForPlan(ctx, pl, "")
	if err != nil {
		return domain.InternalTrip{}, err
	}
	for _, t := range trips {
		if t.TripID == tripID {
			return t, nil
		}
	}
	return domain.InternalTrip{}, fmt.Errorf("not found")
}

func (s Service) InternalOrder(ctx context.Context, orderID string) (domain.OrderTracking, error) {
	if strings.TrimSpace(orderID) == "" {
		return domain.OrderTracking{}, fmt.Errorf("invalid: orderId")
	}
	tracking, err := s.Repo.GetOrderTracking(ctx, orderID)
	if err != nil {
		return domain.OrderTracking{}, err
	}
	if tracking.VehicleID != "" {
		vehicle, e := s.Peers.Vehicle(ctx, tracking.VehicleID)
		if e != nil {
			return domain.OrderTracking{}, fmt.Errorf("vehicle scope unavailable: %w", e)
		}
		tracking.Depot = vehicle.HomeDepot
	}
	return tracking, nil
}

func (s Service) internalForPlan(ctx context.Context, pl domain.Plan, depot string) ([]domain.InternalTrip, error) {
	world, err := s.loadWorld(ctx, pl)
	if err != nil {
		return nil, err
	}
	trips, _ := s.Repo.ListTrips(ctx, pl.ID)
	allocs, _ := s.Repo.ListAllocations(ctx, pl.ID)
	vehByID := map[string]domain.Vehicle{}
	for _, v := range world.Vehicles {
		vehByID[v.ID] = v
	}
	orderByID := map[string]domain.Order{}
	for _, o := range world.Orders {
		orderByID[o.ID] = o
	}
	var out []domain.InternalTrip
	for _, tr := range trips {
		v := vehByID[tr.VehicleID]
		if depot != "" && !strings.EqualFold(v.HomeDepot, depot) {
			continue
		}
		publication := domain.Publication{Version: pl.CurrentVersion}
		if versionStore, ok := any(s.Repo).(planVersionStore); ok && pl.CurrentVersion > 0 {
			publication, _ = versionStore.Publication(ctx, pl.ID)
		}
		it := domain.InternalTrip{
			PlanID: pl.ID, PlanRef: pl.PlanRef, DeliveryDate: pl.DeliveryDate, PlanStatus: pl.Status,
			TripID: tr.ID, TripNumber: tr.TripNumber, VehicleID: tr.VehicleID,
			VehicleType: v.Type, VehicleTemperatureCapability: v.Temp, VehicleDepot: v.HomeDepot,
			VehicleWeightCapacityKg: v.WeightCap, VehicleVolumeCapacityM3: v.VolumeCap,
			Allocations: []domain.InternalAllocation{},
			PlanVersion: pl.CurrentVersion, PlanPublishedAt: pl.PublishedAt, PlanPublishedBy: publication.PublishedBy,
			PlanAcknowledgements: publication.Acknowledgements,
		}
		for _, a := range allocs {
			if a.TripID != tr.ID {
				continue
			}
			o := orderByID[a.OrderID]
			it.Allocations = append(it.Allocations, domain.InternalAllocation{
				AllocationID: a.ID, OrderID: a.OrderID, OrderRef: o.OrderRef, OutletID: o.OutletID, StopSequence: a.Sequence,
				PlannedArrivalAt: a.PlannedArrivalAt, PlannedDepartureAt: a.PlannedDepartureAt,
				Brand: o.Brand, WeightKg: o.WeightKg, VolumeM3: o.VolumeM3, Temperature: o.Temp,
				OutletName: o.Outlet.Name, DockType: o.Outlet.DockType,
				District: o.Outlet.District, WindowOpen: o.Outlet.WindowOpen, WindowClose: o.Outlet.WindowClose,
				ParkingConstraint: o.Outlet.ParkingConstraint, MallWindow: o.Outlet.MallWindow,
			})
		}
		sort.Slice(it.Allocations, func(i, j int) bool { return it.Allocations[i].StopSequence < it.Allocations[j].StopSequence })
		it.PlannedDepartureAt, it.PlannedReturnAt = tripDepotTimes(it.Allocations, v.HomeDepot, world.Est)
		out = append(out, it)
	}
	return out, nil
}

// tripDepotTimes estimates when a trip leaves and returns to its depot: the
// first planned arrival minus the depot leg, and the last planned departure
// plus the leg home.
func tripDepotTimes(allocs []domain.InternalAllocation, depot string, est travel.Estimator) (*time.Time, *time.Time) {
	if len(allocs) == 0 {
		return nil, nil
	}
	var leave, back *time.Time
	if first := allocs[0]; first.PlannedArrivalAt != nil {
		_, minutes := est.Estimate(depot, firstNonEmpty(first.District, first.OutletID))
		t := first.PlannedArrivalAt.Add(-time.Duration(minutes) * time.Minute)
		leave = &t
	}
	if last := allocs[len(allocs)-1]; last.PlannedDepartureAt != nil {
		_, minutes := est.Estimate(firstNonEmpty(last.District, last.OutletID), depot)
		t := last.PlannedDepartureAt.Add(time.Duration(minutes) * time.Minute)
		back = &t
	}
	return leave, back
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func actorID(p *authorization.Profile) string {
	if p == nil {
		return ""
	}
	return p.UserID
}


// The driver's deferral choice is a request. W3 remains the only action that
// records an actual plan deferral; generation leaves these orders for review.
func holdDispatcherDeferrals(world allocate.Input) (allocate.Input, []domain.ConstraintFailure) {
    candidates := make([]domain.Order, 0, len(world.Orders))
    pending := []domain.ConstraintFailure{}
    for _, order := range world.Orders {
        if order.SourceSystem == "delivery-deferral-request" {
            pending = append(pending, domain.ConstraintFailure{OrderID:order.ID, ReasonCode:domain.ReasonManualDeferral})
        } else {
            candidates = append(candidates, order)
        }
    }
    world.Orders = candidates
    return world, pending
}

// validateDeferral enforces the three required parts of a deferral: a known
// reason code, a decider, and a next-run date after the plan's delivery date.
// It returns the normalised next-run date.
func validateDeferral(decider, code, nextRunTarget, planDeliveryDate string) (string, error) {
	if !validReasons[code] {
		return "", fmt.Errorf("invalid: reasonCode")
	}
	if strings.TrimSpace(decider) == "" {
		return "", fmt.Errorf("invalid: decider required")
	}
	nextRunTarget = strings.TrimSpace(nextRunTarget)
	if nextRunTarget == "" {
		return "", fmt.Errorf("invalid: nextRunTarget is required")
	}
	target, err := time.Parse("2006-01-02", nextRunTarget)
	if err != nil {
		return "", fmt.Errorf("invalid: nextRunTarget")
	}
	planDate, _ := time.Parse("2006-01-02", planDeliveryDate)
	if !target.After(planDate) {
		return "", fmt.Errorf("invalid: nextRunTarget")
	}
	return nextRunTarget, nil
}
