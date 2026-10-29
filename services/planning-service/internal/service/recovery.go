package service

import (
	"context"
	"fmt"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/recovery"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/schedule"
	"sort"
	"strings"
	"time"
)

func (s Service) recoveryInput(ctx context.Context, planID, incidentID string, ready time.Time) (recovery.Input, error) {
	var in recovery.Input
	pl, err := s.Repo.Get(ctx, planID)
	if err != nil {
		return in, err
	}
	if pl.Status != domain.StatusConfirmed {
		return in, fmt.Errorf("conflict: recovery requires a published plan")
	}
	incidents, err := s.Peers.RecoveryIncidents(ctx)
	if err != nil {
		return in, err
	}
	for _, i := range incidents {
		if i.ID == incidentID && i.Date == pl.DeliveryDate && i.Type == "breakdown" && i.Status == "open" {
			in.Incident = i
		}
	}
	if in.Incident.ID == "" {
		return in, fmt.Errorf("conflict: open breakdown incident required")
	}
	w, err := s.loadWorld(ctx, pl)
	if err != nil {
		return in, err
	}
	if !w.FuelLedgerAvailable || !w.PolicySignalAvailable {
		return in, fmt.Errorf("conflict: fuel or planning policy unavailable; try again")
	}
	availability, err := s.Peers.RecoveryAvailability(ctx, pl.DeliveryDate)
	if err != nil {
		return in, err
	}
	in.Plan, in.ReadyAt, in.Orders, in.Vehicles, in.Estimator = pl, ready, w.Orders, w.Vehicles, w.Est
	in.Allocations, err = s.Repo.ListAllocations(ctx, planID)
	if err != nil {
		return in, err
	}
	in.Trips, err = s.Repo.ListTrips(ctx, planID)
	if err != nil {
		return in, err
	}
	sort.Slice(in.Allocations, func(i, j int) bool { return in.Allocations[i].ID < in.Allocations[j].ID })
	sort.Slice(in.Trips, func(i, j int) bool { return in.Trips[i].ID < in.Trips[j].ID })
	sort.Slice(in.Orders, func(i, j int) bool { return in.Orders[i].ID < in.Orders[j].ID })
	sort.Slice(in.Vehicles, func(i, j int) bool { return in.Vehicles[i].ID < in.Vehicles[j].ID })
	for i := range in.Vehicles {
		v := &in.Vehicles[i]
		// Reserve every booked trip as well as recorded actual consumption.
		// This conservative bound can double-count past bookings but cannot
		// hide future fuel commitments behind the larger of the two totals.
		v.WeekFuelUsedL = v.WeekFuelActualL + v.WeekFuelPlannedL
		v.Status = availability[v.ID]
		if v.Status == "" {
			v.Status = "available"
		}
		// Workshop update is an expected idempotent approval side effect.
		if v.ID == in.Incident.VehicleID {
			v.Status = "in_workshop"
		}
	}
	in.Deliveries = map[string]recovery.DeliveryState{}
	for _, a := range in.Allocations {
		if a.VehicleID == in.Incident.VehicleID {
			d, err := s.Peers.RecoveryDelivery(ctx, a.OrderID)
			if err != nil {
				return in, err
			}
			in.Deliveries[a.OrderID] = d
		}
	}
	return in, nil
}

func (s Service) CreateRecovery(ctx context.Context, p *authorization.Profile, planID, incidentID string, ready time.Time) (recovery.Case, error) {
	var c recovery.Case
	if !recoveryDispatcher(p) {
		return c, fmt.Errorf("forbidden: dispatcher required")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	now := time.Now()
	if ready.Before(now.Add(5*time.Minute)) || ready.After(now.Add(24*time.Hour)) {
		return c, fmt.Errorf("invalid: handover must be at least five minutes ahead and within 24 hours")
	}
	in, err := s.recoveryInput(ctx, planID, incidentID, ready)
	if err != nil {
		return c, err
	}
	candidates, err := recovery.Build(in)
	if err != nil {
		return c, err
	}
	depot := ""
	for _, v := range in.Vehicles {
		if v.ID == in.Incident.VehicleID {
			depot = v.HomeDepot
		}
	}
	c = recovery.Case{PlanID: planID, PlanVersion: in.Plan.CurrentVersion, Incident: in.Incident, Depot: depot, ReadyAt: ready, CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute), Status: "pending", Snapshot: recovery.Digest(in), AllocationSnapshot: recovery.Digest(in.Allocations), Candidates: candidates, Preferences: []recovery.Preference{}}
	if ready.Before(c.ExpiresAt) {
		c.ExpiresAt = ready
	}
	return s.Repo.SaveRecovery(ctx, c)
}

func recoveryDispatcher(p *authorization.Profile) bool {
	return p != nil && authorization.HasPermission(p.Roles, authorization.PermPlanUpdate)
}
func ownsOutlet(p *authorization.Profile, id string) bool {
	if p == nil || id == "" {
		return false
	}
	for _, o := range p.OutletIDs {
		if o == id {
			return true
		}
	}
	return false
}

// Store views contain only the caller's orders and preferences. Global totals,
// other-store notes and actor identifiers are not sent to the browser.
func scopeRecovery(c recovery.Case, p *authorization.Profile) (recovery.Case, bool) {
	if recoveryDispatcher(p) {
		return c, true
	}
	candidates := []recovery.Candidate{}
	for _, candidate := range c.Candidates {
		candidate.Moves = append([]recovery.Move{}, candidate.Moves...)
		moves := []recovery.Move{}
		vs := map[string]bool{}
		candidate.Recovered, candidate.Deferred, candidate.ChilledDeferred, candidate.DelayMinutes, candidate.DistanceKm = 0, 0, 0, 0, 0
		for _, m := range candidate.Moves {
			if ownsOutlet(p, m.OutletID) {
				m.AllocationID, m.SourceTripID = "", ""
				moves = append(moves, m)
				if m.Deferred {
					candidate.Deferred++
					if m.Chilled {
						candidate.ChilledDeferred++
					}
				} else {
					candidate.Recovered++
					vs[m.VehicleID] = true
				}
				candidate.DelayMinutes += m.DelayMinutes
			}
		}
		candidate.Moves, candidate.Vehicles = moves, len(vs)
		if len(moves) > 0 {
			candidates = append(candidates, candidate)
		}
	}
	c.Candidates = candidates
	prefs := []recovery.Preference{}
	for _, pref := range c.Preferences {
		if ownsOutlet(p, pref.OutletID) {
			pref.Actor = ""
			prefs = append(prefs, pref)
		}
	}
	c.Preferences = prefs
	c.Incident.Description = ""
	c.ApprovalNote = ""
	c.ApprovedBy = ""
	return c, len(candidates) > 0
}

func (s Service) RecoveryCases(ctx context.Context, p *authorization.Profile, date string) ([]recovery.Case, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, fmt.Errorf("invalid: delivery date required")
	}
	items, err := s.Repo.RecoveryCases(ctx, date)
	if err != nil {
		return nil, err
	}
	out := []recovery.Case{}
	for _, c := range items {
		if scoped, ok := scopeRecovery(c, p); ok {
			out = append(out, scoped)
		}
	}
	return out, nil
}

func (s Service) RecoveryPreference(ctx context.Context, p *authorization.Profile, caseID, outletID, candidateID, note string) error {
	if !ownsOutlet(p, outletID) || p.UserID == "" {
		return fmt.Errorf("forbidden: choose a preference only for your store")
	}
	if len(note) > 500 {
		return fmt.Errorf("invalid: note must be under 500 characters")
	}
	c, err := s.Repo.RecoveryCase(ctx, caseID)
	if err != nil {
		return err
	}
	valid := false
	for _, option := range c.Candidates {
		if option.ID == candidateID {
			for _, m := range option.Moves {
				if m.OutletID == outletID {
					valid = true
				}
			}
		}
	}
	if !valid {
		return fmt.Errorf("invalid: candidate does not cover this store")
	}
	return s.Repo.SaveRecoveryPreference(ctx, c, recovery.Preference{OutletID: outletID, CandidateID: candidateID, Note: strings.TrimSpace(note), Actor: p.UserID})
}

func (s Service) ApproveRecovery(ctx context.Context, p *authorization.Profile, caseID, candidateID, note string, readyConfirmed bool) (recovery.Case, error) {
	var c recovery.Case
	if !recoveryDispatcher(p) {
		return c, fmt.Errorf("forbidden: dispatcher approval required")
	}
	if !readyConfirmed || len(strings.TrimSpace(note)) < 5 || len(note) > 1000 {
		return c, fmt.Errorf("invalid: confirm driver and cargo readiness and explain the decision")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	c, err := s.Repo.RecoveryCase(ctx, caseID)
	if err != nil {
		return c, err
	}
	if c.Status == "approved" {
		if c.SelectedID == candidateID {
			return c, nil
		}
		return c, fmt.Errorf("conflict: another option was already approved")
	}
	if c.Status != "pending" || !time.Now().Before(c.ExpiresAt) {
		return c, fmt.Errorf("conflict: options expired or superseded; generate fresh options")
	}
	in, err := s.recoveryInput(ctx, c.PlanID, c.Incident.ID, c.ReadyAt)
	if err != nil {
		return c, err
	}
	if recovery.Digest(in) != c.Snapshot {
		return c, fmt.Errorf("conflict: fleet, delivery progress or plan changed; generate fresh options")
	}
	options, err := recovery.Build(in)
	if err != nil {
		return c, err
	}
	var selected *recovery.Candidate
	for i := range options {
		if options[i].ID == candidateID {
			selected = &options[i]
		}
	}
	if selected == nil {
		return c, fmt.Errorf("invalid: unknown candidate")
	}
	if err = s.Peers.SetVehicleWorkshop(ctx, c.Incident.VehicleID, in.Plan.DeliveryDate, "recovery "+c.ID); err != nil {
		return c, fmt.Errorf("workshop_pending: source vehicle could not be suspended; retry")
	}
	if err = s.Peers.SuspendRecovery(ctx, c.ID, selected.Moves); err != nil {
		return c, err
	}
	// Re-read after the source route is closed to further completion events.
	in, err = s.recoveryInput(ctx, c.PlanID, c.Incident.ID, c.ReadyAt)
	if err != nil {
		return c, err
	}
	if recovery.Digest(in) != c.Snapshot {
		return c, fmt.Errorf("conflict: state changed during approval; generate fresh options; broken vehicle remains suspended")
	}
	if !time.Now().Before(c.ExpiresAt) {
		return c, fmt.Errorf("conflict: options expired; generate fresh options")
	}
	result, err := s.Repo.ApplyRecovery(ctx, c, *selected, p.UserID, strings.TrimSpace(note), schedule.PlanDate(in.Plan.DeliveryDate).AddDate(0, 0, 1).Format("2006-01-02"))
	if err == nil {
		s.Peers.Publish(ctx, audit.ActionBreakdownRecoveryConfirmed, p.UserID, "PLAN", in.Plan.PlanRef, map[string]any{"caseId": c.ID, "candidateId": candidateID, "version": result.PublishedVersion})
	}
	return result, err
}
