package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	depotnames "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/depot"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/objectstore"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/validation"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/sequence"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/store"
)

// MaxPhotoBytes caps a shortfall photo at the same size as a delivery proof photo (4 MiB).
const MaxPhotoBytes = 4 << 20

type Service struct {
	Repo     store.Postgres
	Peers    client.Peers
	Sequence sequence.Policy
	// Objects stores shortfall photos; nil disables photo upload.
	Objects objectstore.Reader
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
	// Planning reports the vehicle's dataset depot ("Peliyagoda"); profiles
	// use DEPOT_NORTH. Fetch every trip and compare through sameDepot.
	trips, err := s.Peers.ConfirmedTrips(ctx, date, "")
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, t := range trips {
		if depot != "" && !depotnames.Same(t.VehicleDepot, depot) {
			continue
		}
		item := tripSummary(t)
		for k, v := range planningInfo(t) {
			item[k] = v
		}
		if sess, err := s.Repo.GetByTrip(ctx, t.TripID); err == nil {
			loads, _ := s.Repo.ListLoads(ctx, sess.ID)
			item["loadingStatus"] = sess.Status
			item["loadedCount"], item["shortfallCount"], item["pendingCount"] = counts(loads)
			item["planRef"] = sess.PlanRef
			item["preparedPlanVersion"] = sess.PlanVersion
			item["planChanged"] = sess.Status == domain.SessionInProgress && sess.PlanVersion < t.PlanVersion
			item["readyAt"], item["readyBy"] = sess.ReadyAt, sess.ReadyBy
		} else {
			item["loadingStatus"] = domain.SessionPending
			item["loadedCount"], item["shortfallCount"], item["pendingCount"] = 0, 0, len(t.Allocations)
		}
		item["acknowledgedVersion"] = acknowledgedVersion(t, profile)
		item["acknowledgedAt"] = acknowledgedAt(t, profile)
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
	return s.pendingDetail(ctx, trip, profile), nil
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
	sugBy := s.suggestions(trip.Allocations)
	details, err := s.orderDetails(ctx, orderIDs(trip.Allocations))
	if err != nil {
		return nil, fmt.Errorf("order details unavailable")
	}
	loads := make([]domain.OrderLoad, 0, len(trip.Allocations))
	for _, a := range trip.Allocations {
		loads = append(loads, newLoad(a, details[a.OrderID], sugBy[a.OrderID]))
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

func (s Service) suggestions(allocs []domain.PlanningAlloc) map[string]int {
	stops := make([]domain.Stop, 0, len(allocs))
	for _, a := range allocs {
		stops = append(stops, domain.Stop{OrderID: a.OrderID, StopSequence: a.StopSequence})
	}
	sugBy := map[string]int{}
	for _, g := range s.seq().Suggest(stops) {
		sugBy[g.OrderID] = g.SuggestedLoadSequence
	}
	return sugBy
}

func newLoad(a domain.PlanningAlloc, ord domain.OrderDetail, suggested int) domain.OrderLoad {
	return domain.OrderLoad{
		AllocationID: a.AllocationID, OrderID: a.OrderID, OrderRef: first(ord.OrderRef, a.OrderRef),
		OutletID: first(ord.OutletID, a.OutletID), Brand: ord.Brand, TemperatureRequirement: ord.TemperatureRequirement,
		StopSequence: a.StopSequence, SuggestedLoadSequence: suggested,
		ExpectedUnits: ord.OrderUnits,
	}
}

// orderFetchConcurrency bounds how many orders are read from order-service at once.
const orderFetchConcurrency = 8

func orderIDs(allocs []domain.PlanningAlloc) []string {
	ids := make([]string, 0, len(allocs))
	for _, a := range allocs {
		ids = append(ids, a.OrderID)
	}
	return ids
}

// orderDetails reads the distinct orders from order-service a few at a time rather
// than one after another, so a trip with many stops costs a handful of round trips'
// time, not one per stop. It returns what it could read and the first error:
// callers that need every order (starting or syncing a session) refuse on the
// error, callers that only decorate a response carry on with what came back.
func (s Service) orderDetails(ctx context.Context, ids []string) (map[string]domain.OrderDetail, error) {
	out := make(map[string]domain.OrderDetail, len(ids))
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)
	slots := make(chan struct{}, orderFetchConcurrency)
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		wg.Add(1)
		slots <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-slots }()
			ord, err := s.Peers.Order(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			out[id] = ord
		}(id)
	}
	wg.Wait()
	return out, firstErr
}

// SyncPlanVersion moves a started loading session onto the dispatcher's newer
// plan version once the loader has acknowledged it. Orders added to the trip
// become pending lines, orders taken off the trip are removed, and lines whose
// stop moved are re-sequenced; a moved line that was already loaded goes back
// to pending so the loader rechecks where it sits.
func (s Service) SyncPlanVersion(ctx context.Context, profile *authorization.Profile, tripID string, expectedPlanVersion int) (map[string]any, error) {
	sess, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	if err := s.guardDepot(profile, sess.Depot); err != nil {
		return nil, err
	}
	trip, err := s.Peers.ConfirmedTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("conflict: trip is no longer on a confirmed plan")
	}
	if sess.PlanVersion >= trip.PlanVersion {
		return s.detailFromSession(ctx, profile, sess)
	}
	if expectedPlanVersion > 0 && expectedPlanVersion != trip.PlanVersion {
		return nil, fmt.Errorf("conflict: plan version changed from %d to %d; reload latest loading instructions", expectedPlanVersion, trip.PlanVersion)
	}
	if acknowledgedVersion(trip, profile) != trip.PlanVersion {
		return nil, fmt.Errorf("conflict: acknowledge plan version %d before loading against it", trip.PlanVersion)
	}
	loads, err := s.Repo.ListLoads(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	sugBy := s.suggestions(trip.Allocations)
	diff := diffPlan(loads, trip.Allocations, sugBy)
	details, err := s.orderDetails(ctx, orderIDs(diff.Added))
	if err != nil {
		return nil, fmt.Errorf("order details unavailable")
	}
	add := make([]domain.OrderLoad, 0, len(diff.Added))
	for _, a := range diff.Added {
		l := newLoad(a, details[a.OrderID], sugBy[a.OrderID])
		l.ChangeNote = fmt.Sprintf("Added in v%d", trip.PlanVersion)
		add = append(add, l)
	}
	remove, change := diff.removals(), diff.loadChanges()
	if err := s.Repo.SyncPlanVersion(ctx, sess.ID, trip.PlanVersion, actor(profile), add, remove, change); err != nil {
		return nil, err
	}
	s.Peers.Publish(ctx, audit.ActionLoadingPlanVersionSynced, actor(profile), "TRIP", tripID, map[string]any{
		"fromVersion": sess.PlanVersion, "toVersion": trip.PlanVersion, "added": len(add), "removed": len(remove), "changed": len(change),
	})
	updated, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return nil, err
	}
	return s.detailFromSession(ctx, profile, updated)
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

func validIssueType(typ string) bool {
	return typ == domain.IssueMissing || typ == domain.IssueDamaged || typ == domain.IssueWrongItem
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
	if !validIssueType(typ) {
		return domain.Issue{}, fmt.Errorf("invalid: issue type")
	}
	if units <= 0 || units > load.ExpectedUnits {
		return domain.Issue{}, fmt.Errorf("invalid: affectedUnits")
	}
	if len(note) > 500 {
		return domain.Issue{}, fmt.Errorf("invalid: note too long")
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

// AttachIssuePhoto stores a PNG or JPEG photo of the missing or damaged goods
// on a report. A newer photo replaces the earlier one.
func (s Service) AttachIssuePhoto(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID, mime string, body []byte) (domain.Issue, error) {
	if s.Objects == nil {
		return domain.Issue{}, fmt.Errorf("conflict: photo storage is not configured")
	}
	sess, load, err := s.mutableLoad(ctx, profile, tripID, orderID)
	if err != nil {
		return domain.Issue{}, err
	}
	iss, err := s.Repo.GetIssue(ctx, issueID)
	if err != nil || iss.OrderLoadID != load.ID {
		return domain.Issue{}, fmt.Errorf("not found")
	}
	mime = strings.ToLower(strings.TrimSpace(mime))
	if mime != "image/png" && mime != "image/jpeg" {
		return domain.Issue{}, fmt.Errorf("invalid: PNG or JPEG only")
	}
	if len(body) == 0 || len(body) > MaxPhotoBytes {
		return domain.Issue{}, fmt.Errorf("invalid: photo size")
	}
	if !validation.HasImageMagic(mime, body) {
		return domain.Issue{}, fmt.Errorf("invalid: file content does not match PNG or JPEG")
	}
	ext := ".jpg"
	if mime == "image/png" {
		ext = ".png"
	}
	key := fmt.Sprintf("loading/%s/%s/%s%s", sess.DeliveryDate, sess.TripID, iss.ID, ext)
	if err := s.Objects.Put(ctx, key, mime, body); err != nil {
		return domain.Issue{}, fmt.Errorf("photo storage write failed")
	}
	if err := s.Repo.SetIssuePhoto(ctx, iss.ID, key, mime); err != nil {
		return domain.Issue{}, err
	}
	s.Peers.Publish(ctx, audit.ActionLoadingShortfallPhotoAdded, actor(profile), "ORDER", orderID, map[string]any{"issueId": iss.ID, "tripId": tripID})
	return s.Repo.GetIssue(ctx, iss.ID)
}

// IssuePhoto returns a report's photo to a loader at that depot or a dispatcher.
func (s Service) IssuePhoto(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID string) (io.ReadCloser, string, error) {
	sess, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return nil, "", fmt.Errorf("not found")
	}
	if err := s.guardDepot(profile, sess.Depot); err != nil {
		return nil, "", err
	}
	load, err := s.Repo.GetLoad(ctx, sess.ID, orderID)
	if err != nil {
		return nil, "", fmt.Errorf("not found")
	}
	iss, err := s.Repo.GetIssue(ctx, issueID)
	if err != nil || iss.OrderLoadID != load.ID || iss.PhotoKey == "" || s.Objects == nil {
		return nil, "", fmt.Errorf("not found")
	}
	photo, err := s.Objects.Open(ctx, iss.PhotoKey)
	if err != nil {
		return nil, "", fmt.Errorf("not found: photo")
	}
	return photo, iss.PhotoMime, nil
}

// MarkIssueSeen records that a dispatcher opened a report, so the loader can
// see it has been seen.
func (s Service) MarkIssueSeen(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID string) error {
	sess, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return fmt.Errorf("not found")
	}
	if err := s.guardDepot(profile, sess.Depot); err != nil {
		return err
	}
	load, err := s.Repo.GetLoad(ctx, sess.ID, orderID)
	if err != nil {
		return fmt.Errorf("not found")
	}
	iss, err := s.Repo.GetIssue(ctx, issueID)
	if err != nil || iss.OrderLoadID != load.ID {
		return fmt.Errorf("not found")
	}
	return s.Repo.MarkIssueSeen(ctx, issueID, actor(profile))
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
	if !validIssueType(typ) {
		return fmt.Errorf("invalid: issue type")
	}
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
	if iss.PhotoKey != "" && s.Objects != nil {
		_ = s.Objects.Delete(ctx, iss.PhotoKey)
	}
	if remaining, _ := s.Repo.ListIssues(ctx, load.ID); len(remaining) == 0 && load.Status == domain.LoadShortfall {
		_ = s.Repo.SetLoadStatus(ctx, load.ID, domain.LoadPending, actor(profile))
	}
	s.Peers.Publish(ctx, audit.ActionLoadingShortfallRemoved, actor(profile), "ORDER", orderID, map[string]any{"issueId": issueID})
	return nil
}

func (s Service) Ready(ctx context.Context, profile *authorization.Profile, tripID string, checks ReadyChecks) (map[string]any, error) {
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
	if acknowledgedVersion(trip, profile) != trip.PlanVersion {
		return nil, fmt.Errorf("conflict: loader must acknowledge the current plan version before departure")
	}
	if sess.Status == domain.SessionReady {
		return s.detailFromSession(ctx, profile, sess)
	}
	if sess.Status != domain.SessionInProgress {
		return nil, fmt.Errorf("conflict: session not in progress")
	}
	loads, _ := s.Repo.ListLoads(ctx, sess.ID)
	issues := map[string][]domain.Issue{}
	for _, l := range loads {
		if l.Status == domain.LoadShortfall {
			issues[l.ID], _ = s.Repo.ListIssues(ctx, l.ID)
		}
	}
	// The plan checked above is confirmed and is the version this session is
	// loading against, so its allocations say which orders are still on the trip.
	onPlan := map[string]bool{}
	for _, a := range trip.Allocations {
		onPlan[a.OrderID] = true
	}
	pending, undecided, moveUnpublished := departureBlockers(loads, issues, onPlan)
	if len(pending) > 0 {
		return nil, IncompleteError{Pending: pending}
	}
	if len(undecided) > 0 || len(moveUnpublished) > 0 {
		return nil, DecisionRequiredError{OrderIDs: append(undecided, moveUnpublished...), MoveUnpublished: moveUnpublished}
	}
	// The departure checks are not optional for a refrigerated vehicle: a request
	// without them must not release the trip, whatever the app in front of it does.
	checks, err = validateReadyChecks(sess.VehicleTemperatureCapability, checks)
	if err != nil {
		return nil, err
	}
	ready, err := s.Repo.MarkReady(ctx, sess.ID, actor(profile), checks.ChilledTemperatureC, checks.SealNumber)
	if err != nil {
		if existing, e2 := s.Repo.GetByTrip(ctx, tripID); e2 == nil && existing.Status == domain.SessionReady {
			return s.detailFromSession(ctx, profile, existing)
		}
		return nil, err
	}
	telemetry.LoadingTripsReady.Inc()
	telemetry.LoadingDuration.Observe(time.Since(sess.StartedAt).Seconds())
	state := map[string]any{"planRef": sess.PlanRef}
	if checks.ChilledTemperatureC != nil {
		state["chilledTemperatureC"] = *checks.ChilledTemperatureC
	}
	if checks.SealNumber != "" {
		state["sealNumber"] = checks.SealNumber
	}
	s.Peers.Publish(ctx, audit.ActionTripReadyForDeparture, actor(profile), "TRIP", tripID, state)
	return s.detailFromSession(ctx, profile, ready)
}

type IncompleteError struct{ Pending []string }

// DecisionRequiredError blocks departure while a loader shortfall has no
// dispatcher decision, the dispatcher chose to hold the trip, or the line was
// sent to the next run but planning has not published a plan without it yet
// (MoveUnpublished, a subset of OrderIDs).
type DecisionRequiredError struct {
	OrderIDs        []string
	MoveUnpublished []string
}

func (e DecisionRequiredError) Error() string { return "conflict: dispatcher_decision_required" }

// departureBlockers sorts the order lines that stop a trip leaving: lines not
// yet loaded or reported (pending), shortfalls the dispatcher has not cleared
// (undecided, including HOLD), and lines moved to the next run whose order is
// still on the confirmed plan (moveUnpublished). The last case is what keeps a
// "move to next run" decision from releasing the trip with the short line still
// on it if the planning calls that follow it fail or have not finished.
func departureBlockers(loads []domain.OrderLoad, issues map[string][]domain.Issue, onPlan map[string]bool) (pending, undecided, moveUnpublished []string) {
	for _, l := range loads {
		if l.Status == domain.LoadPending {
			pending = append(pending, l.OrderID)
			continue
		}
		if l.Status != domain.LoadShortfall {
			continue
		}
		iss := issues[l.ID]
		if len(iss) == 0 {
			pending = append(pending, l.OrderID)
			continue
		}
		for _, i := range iss {
			if domain.DecisionAllowsDeparture(i.Decision, onPlan[l.OrderID]) {
				continue
			}
			if i.Decision == domain.DecisionMoveToNextRun {
				moveUnpublished = append(moveUnpublished, l.OrderID)
			} else {
				undecided = append(undecided, l.OrderID)
			}
			break
		}
	}
	return
}

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
	_ = s.Repo.MarkIssueSeen(ctx, issueID, actor(profile))
	s.Peers.Publish(ctx, audit.ActionLoadingShortfallDecided, actor(profile), "ORDER", orderID, map[string]any{"issueId": issueID, "tripId": tripID, "decision": decision, "note": note})
	return s.Repo.GetIssue(ctx, issueID)
}

func (e IncompleteError) Error() string { return "conflict: loading_incomplete" }

// RaiseDockAlert is the loader's "tell dispatcher" for goods found at the
// wrong vehicle. The trip is the vehicle the goods were found at.
func (s Service) RaiseDockAlert(ctx context.Context, profile *authorization.Profile, tripID, typ, orderRef, belongsVehicleID, note, key string) (domain.DockAlert, error) {
	if key == "" {
		return domain.DockAlert{}, fmt.Errorf("invalid: Idempotency-Key required")
	}
	typ = strings.ToUpper(strings.TrimSpace(typ))
	if typ == "" {
		typ = domain.AlertWrongVehicle
	}
	if typ != domain.AlertWrongVehicle {
		return domain.DockAlert{}, fmt.Errorf("invalid: alert type")
	}
	orderRef = strings.TrimSpace(orderRef)
	if orderRef == "" || len(orderRef) > 64 || len(note) > 500 || len(belongsVehicleID) > 64 {
		return domain.DockAlert{}, fmt.Errorf("invalid: orderRef and note")
	}
	depot, date := "", ""
	if sess, err := s.Repo.GetByTrip(ctx, tripID); err == nil {
		depot, date = sess.Depot, sess.DeliveryDate
	} else {
		trip, err := s.Peers.ConfirmedTrip(ctx, tripID)
		if err != nil {
			return domain.DockAlert{}, fmt.Errorf("not found")
		}
		depot, date = trip.VehicleDepot, trip.DeliveryDate
	}
	if err := s.guardDepot(profile, depot); err != nil {
		return domain.DockAlert{}, err
	}
	a, err := s.Repo.InsertAlert(ctx, domain.DockAlert{
		TripID: tripID, Depot: depot, DeliveryDate: date, Type: typ, OrderRef: orderRef,
		BelongsVehicleID: strings.TrimSpace(belongsVehicleID), Note: strings.TrimSpace(note), ReportedBy: actor(profile),
	}, key)
	if err != nil {
		return domain.DockAlert{}, err
	}
	s.Peers.Publish(ctx, audit.ActionLoadingDockAlertRaised, actor(profile), "TRIP", tripID, map[string]any{"type": typ, "orderRef": orderRef, "belongsVehicleId": a.BelongsVehicleID})
	return a, nil
}

// ListDockAlerts returns a date's dock alerts: a loader sees their depot, a
// dispatcher every depot.
func (s Service) ListDockAlerts(ctx context.Context, profile *authorization.Profile, date string) ([]domain.DockAlert, error) {
	depot := ""
	if !authorization.HasPermission(profile.Roles, authorization.PermLoadingViewAll) {
		depot = profile.Depot
		if depot == "" {
			return nil, fmt.Errorf("forbidden: loader depot required")
		}
	}
	all, err := s.Repo.ListAlerts(ctx, date, "")
	if err != nil || depot == "" {
		return all, err
	}
	out := []domain.DockAlert{}
	for _, a := range all {
		if depotnames.Same(a.Depot, depot) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s Service) ResolveDockAlert(ctx context.Context, profile *authorization.Profile, id string) (domain.DockAlert, error) {
	a, err := s.Repo.GetAlert(ctx, id)
	if err != nil {
		return a, fmt.Errorf("not found")
	}
	if err := s.guardDepot(profile, a.Depot); err != nil {
		return a, err
	}
	if err := s.Repo.ResolveAlert(ctx, id, actor(profile)); err != nil {
		return a, err
	}
	s.Peers.Publish(ctx, audit.ActionLoadingDockAlertResolved, actor(profile), "TRIP", a.TripID, map[string]any{"alertId": id})
	return s.Repo.GetAlert(ctx, id)
}

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
	if profile == nil || profile.Depot == "" || !depotnames.Same(profile.Depot, depot) {
		return fmt.Errorf("forbidden: depot")
	}
	return nil
}

func (s Service) detailFromSession(ctx context.Context, profile *authorization.Profile, sess domain.Session) (map[string]any, error) {
	loads, _ := s.Repo.ListLoads(ctx, sess.ID)
	trip, tripErr := s.Peers.ConfirmedTrip(ctx, sess.TripID)
	allocBy := map[string]domain.PlanningAlloc{}
	if tripErr == nil {
		for _, a := range trip.Allocations {
			allocBy[a.OrderID] = a
		}
	}
	loadIDs := make([]string, 0, len(loads))
	for _, l := range loads {
		loadIDs = append(loadIDs, l.OrderID)
	}
	details, _ := s.orderDetails(ctx, loadIDs) // decoration only: carry on with what came back
	var orders []map[string]any
	for _, l := range loads {
		iss, _ := s.Repo.ListIssues(ctx, l.ID)
		ord := details[l.OrderID]
		row := map[string]any{
			"orderId": l.OrderID, "orderRef": first(l.OrderRef, ord.OrderRef), "outletId": first(l.OutletID, ord.OutletID),
			"brand": first(l.Brand, ord.Brand), "temperatureRequirement": first(l.TemperatureRequirement, ord.TemperatureRequirement),
			"expectedUnits": l.ExpectedUnits, "stopSequence": l.StopSequence,
			"suggestedLoadSequence": l.SuggestedLoadSequence, "status": l.Status, "issues": iss, "allocationId": l.AllocationID,
			"weightKg": ord.OrderWeightKg, "volumeM3": ord.OrderVolumeM3,
			"changedInVersion": l.ChangedInVersion, "changeNote": l.ChangeNote,
		}
		if a, ok := allocBy[l.OrderID]; ok {
			addAccess(row, a)
		}
		orders = append(orders, row)
	}
	if orders == nil {
		orders = []map[string]any{}
	}
	loaded, short, pend := counts(loads)
	out := map[string]any{
		"tripId": sess.TripID, "planId": sess.PlanID, "planRef": sess.PlanRef, "deliveryDate": sess.DeliveryDate,
		"vehicleId": sess.VehicleID, "depot": sess.Depot, "status": sess.Status,
		"tripNumber": sess.TripNumber, "vehicleType": sess.VehicleType, "vehicleTemperatureCapability": sess.VehicleTemperatureCapability,
		"loadedCount": loaded, "shortfallCount": short, "pendingCount": pend,
		"planVersion": sess.PlanVersion, "preparedPlanVersion": sess.PlanVersion, "acknowledgedVersion": 0,
		"planChanged": false, "changes": []map[string]any{},
		"readyBy": sess.ReadyBy, "readyAt": sess.ReadyAt, "readyTemperatureC": sess.ReadyTemperatureC, "readySeal": sess.ReadySeal,
		"orders": orders, "session": sess,
	}
	if tripErr == nil {
		for k, v := range planningInfo(trip) {
			out[k] = v
		}
		out["planVersion"] = trip.PlanVersion
		out["acknowledgedVersion"] = acknowledgedVersion(trip, profile)
		out["acknowledgedAt"] = acknowledgedAt(trip, profile)
		if sess.Status == domain.SessionInProgress && sess.PlanVersion < trip.PlanVersion {
			out["planChanged"] = true
			out["changes"] = planChanges(loads, trip)
		}
	}
	return out, nil
}

// planChanges lists how the current plan version differs from the lines the
// loader is working from: orders added, taken off the trip, or moved stop. It
// describes the same comparison syncing applies (see diffPlan).
func planChanges(loads []domain.OrderLoad, trip domain.PlanningTrip) []map[string]any {
	return diffPlan(loads, trip.Allocations, nil).describe()
}

func addAccess(row map[string]any, a domain.PlanningAlloc) {
	row["district"] = a.District
	row["outletName"] = a.OutletName
	row["dockType"] = a.DockType
	row["windowOpen"] = a.WindowOpen
	row["windowClose"] = a.WindowClose
	row["parkingConstraint"] = a.ParkingConstraint
	row["mallWindow"] = a.MallWindow
	row["plannedArrivalAt"] = a.PlannedArrivalAt
	if a.WeightKg > 0 {
		row["weightKg"] = a.WeightKg
	}
	if a.VolumeM3 > 0 {
		row["volumeM3"] = a.VolumeM3
	}
}

// planningInfo is the trip-level capacity, timing and publication detail the
// loader checks against: weight, volume and chilled volume against the
// vehicle, and trip time against the Fresh budget.
func planningInfo(t domain.PlanningTrip) map[string]any {
	var kg, m3, chilledM3 float64
	fresh := false
	areas := []string{}
	seen := map[string]bool{}
	for _, a := range t.Allocations {
		kg += a.WeightKg
		m3 += a.VolumeM3
		if chilled(a.Temperature) {
			chilledM3 += a.VolumeM3
		}
		if strings.EqualFold(a.Brand, "Fresh") {
			fresh = true
		}
		if a.District != "" && !seen[a.District] {
			seen[a.District] = true
			areas = append(areas, a.District)
		}
	}
	out := map[string]any{
		"vehicleWeightCapacityKg": t.VehicleWeightCapacityKg, "vehicleVolumeCapacityM3": t.VehicleVolumeCapacityM3,
		"totalWeightKg": round1(kg), "totalVolumeM3": round1(m3), "chilledVolumeM3": round1(chilledM3),
		"plannedDepartureAt": t.PlannedDepartureAt, "plannedReturnAt": t.PlannedReturnAt,
		"planPublishedAt": t.PlanPublishedAt, "planPublishedBy": t.PlanPublishedBy, "areas": areas,
	}
	if n := len(t.Allocations); n > 0 && t.PlannedDepartureAt != nil && t.Allocations[n-1].PlannedDepartureAt != nil {
		out["tripMinutes"] = int(t.Allocations[n-1].PlannedDepartureAt.Sub(*t.PlannedDepartureAt).Minutes())
	}
	if fresh {
		out["freshBudgetMinutes"] = domain.FreshTripBudgetMinutes
	}
	return out
}

// acknowledgedAt is when this loader acknowledged the current plan version.
func acknowledgedAt(t domain.PlanningTrip, profile *authorization.Profile) *time.Time {
	for _, ack := range t.PlanAcknowledgements {
		if ack.ActorID == actor(profile) && ack.ActorRole == authorization.RoleLoader && t.PlanVersion > 0 {
			at := ack.AcknowledgedAt
			return &at
		}
	}
	return nil
}

func acknowledgedVersion(t domain.PlanningTrip, profile *authorization.Profile) int {
	for _, ack := range t.PlanAcknowledgements {
		if ack.ActorID == actor(profile) && ack.ActorRole == authorization.RoleLoader && t.PlanVersion > 0 {
			return t.PlanVersion
		}
	}
	return 0
}

func chilled(temp string) bool {
	t := strings.ToLower(temp)
	return strings.Contains(t, "chill") || strings.Contains(t, "frozen") || strings.Contains(t, "refriger")
}

func refrigerated(capability string) bool {
	c := strings.ToLower(capability)
	return chilled(c) || strings.Contains(c, "multi") || strings.Contains(c, "reefer")
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

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
		"readyTemperatureC": sess.ReadyTemperatureC, "readySeal": sess.ReadySeal,
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

func (s Service) pendingDetail(ctx context.Context, trip domain.PlanningTrip, profile *authorization.Profile) map[string]any {
	sugBy := s.suggestions(trip.Allocations)
	details, _ := s.orderDetails(ctx, orderIDs(trip.Allocations)) // decoration only
	var orders []map[string]any
	for _, a := range trip.Allocations {
		row := map[string]any{
			"orderId": a.OrderID, "orderRef": a.OrderRef, "outletId": a.OutletID,
			"stopSequence": a.StopSequence, "suggestedLoadSequence": sugBy[a.OrderID],
			"status": domain.LoadPending, "issues": []domain.Issue{},
			"brand": a.Brand, "temperatureRequirement": a.Temperature, "weightKg": a.WeightKg, "volumeM3": a.VolumeM3,
		}
		if ord, ok := details[a.OrderID]; ok {
			row["expectedUnits"] = ord.OrderUnits
			row["brand"] = first(a.Brand, ord.Brand)
			row["temperatureRequirement"] = first(a.Temperature, ord.TemperatureRequirement)
		}
		addAccess(row, a)
		orders = append(orders, row)
	}
	if orders == nil {
		orders = []map[string]any{}
	}
	out := map[string]any{
		"tripId": trip.TripID, "planId": trip.PlanID, "planRef": trip.PlanRef, "deliveryDate": trip.DeliveryDate,
		"vehicleId": trip.VehicleID, "vehicleType": trip.VehicleType,
		"vehicleTemperatureCapability": trip.VehicleTemperatureCapability,
		"depot":                        trip.VehicleDepot, "tripNumber": trip.TripNumber, "status": domain.SessionPending,
		"planVersion": trip.PlanVersion, "preparedPlanVersion": trip.PlanVersion, "acknowledgedVersion": acknowledgedVersion(trip, profile),
		"acknowledgedAt": acknowledgedAt(trip, profile),
		"planChanged": false, "changes": []map[string]any{},
		"loadedCount": 0, "shortfallCount": 0, "pendingCount": len(trip.Allocations), "orders": orders,
	}
	for k, v := range planningInfo(trip) {
		out[k] = v
	}
	return out
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
