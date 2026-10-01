package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/validation"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/cutoff"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

var (
	ErrInvalid     = errors.New("invalid request")
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not found")
	ErrUnavailable = errors.New("profile unavailable")
)

type Service struct {
	Repo     domain.Repository
	Outlets  domain.ProfileClient
	Audit    domain.AuditPublisher
	Cutoff   *cutoff.Calendar
	Policy   domain.PolicyReader
	Calendar domain.CalendarReader
	Now      func() time.Time
	Planning domain.PlanningReader
	Delivery domain.DeliveryReader
}

var validReceiptIssueTypes = map[string]bool{"MISSING": true, "DAMAGED": true, "QUANTITY_MISMATCH": true, "OTHER": true}

func (s Service) Tracking(profile *authorization.Profile, id string) (domain.Tracking, error) {
	o, err := s.Get(profile, id)
	if err != nil {
		return domain.Tracking{}, err
	}
	t := domain.Tracking{Order: o, Stage: "CONFIRMED", Planning: domain.PlanningTracking{State: "CONFIRMED"}, ReceiptIssues: []domain.ReceiptIssue{}, Custody: []domain.CustodyEvent{}}
	if strings.EqualFold(strings.TrimSpace(o.Brand), "Tech") {
		if events, e := s.Repo.ListCustodyEvents(o.ID); e == nil {
			t.Custody = events
		} else {
			return domain.Tracking{}, e
		}
	}
	if s.Planning != nil {
		if p, e := s.Planning.ByOrder(o.ID); e == nil {
			t.Planning = p
		} else if !strings.Contains(e.Error(), "404") && !strings.Contains(e.Error(), "not found") {
			return domain.Tracking{}, fmt.Errorf("planning unavailable: %w", e)
		}
	}
	stage := strings.ToUpper(t.Planning.State)
	if stage == "" {
		stage = "CONFIRMED"
	}
	t.Stage = stage
	if s.Delivery != nil && t.Planning.TripID != "" {
		if d, e := s.Delivery.ByOrder(o.ID); e == nil {
			t.Delivery = &d
			switch d.RunStatus {
			case "in_progress":
				t.Stage = "OUT_FOR_DELIVERY"
			case "prepared":
				t.Stage = "READY_FOR_DEPARTURE"
			case "completed":
				if d.Outcome != "" {
					t.Stage = d.Outcome
				}
			}
			if d.Outcome != "" {
				t.Stage = d.Outcome
			}
		} else if !strings.Contains(e.Error(), "404") && !strings.Contains(e.Error(), "not found") {
			return domain.Tracking{}, fmt.Errorf("delivery unavailable: %w", e)
		}
	}
	if receipt, issues, e := s.Repo.GetReceipt(o.ID); e == nil {
		t.Receipt = &receipt
		t.ReceiptIssues = issues
		if receipt.Status == "confirmed_with_issue" {
			t.Stage = "RECEIPT_CONFIRMED_WITH_ISSUE"
		} else {
			t.Stage = "RECEIPT_CONFIRMED"
		}
	} else if !strings.Contains(e.Error(), "not found") {
		return domain.Tracking{}, e
	}
	return t, nil
}

func (s Service) RecordCustody(profile *authorization.Profile, id string, event domain.CustodyEvent) (domain.CustodyEvent, bool, error) {
	if profile == nil {
		return domain.CustodyEvent{}, false, ErrForbidden
	}
	event.Stage = strings.ToUpper(strings.TrimSpace(event.Stage))
	var o domain.Order
	var err error
	if event.Stage == "RECEIVED" {
		o, err = s.Get(profile, id)
	} else {
		o, err = s.Repo.Get(id)
	}
	if err != nil {
		return domain.CustodyEvent{}, false, ErrNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(o.Brand), "Tech") {
		return domain.CustodyEvent{}, false, fmt.Errorf("invalid: custody workflow is only enabled for Tech orders")
	}
	roleAllowed := false
	switch event.Stage {
	case "LOADED":
		roleAllowed = authorization.HasPermission(profile.Roles, authorization.PermLoadingUpdate)
		if roleAllowed {
			if s.Planning == nil || strings.TrimSpace(profile.Depot) == "" {
				return domain.CustodyEvent{}, false, ErrForbidden
			}
			assigned, e := s.Planning.ByOrder(o.ID)
			if e != nil || assigned.TripID == "" || !strings.EqualFold(strings.TrimSpace(assigned.Depot), strings.TrimSpace(profile.Depot)) {
				return domain.CustodyEvent{}, false, ErrForbidden
			}
		}
	case "DISPATCHED", "DELIVERED":
		roleAllowed = authorization.HasPermission(profile.Roles, authorization.PermDeliveryUpdate)
		if roleAllowed {
			if s.Delivery == nil || strings.TrimSpace(profile.VehicleID) == "" {
				return domain.CustodyEvent{}, false, ErrForbidden
			}
			assigned, e := s.Delivery.ByOrder(o.ID)
			if e != nil || assigned.VehicleID == "" || assigned.VehicleID != profile.VehicleID {
				return domain.CustodyEvent{}, false, ErrForbidden
			}
		}
	case "RECEIVED":
		roleAllowed = authorization.HasPermission(profile.Roles, authorization.PermReceiptConfirm)
	default:
		return domain.CustodyEvent{}, false, fmt.Errorf("invalid: stage")
	}
	if !roleAllowed {
		return domain.CustodyEvent{}, false, ErrForbidden
	}
	if event.Stage == "DELIVERED" {
		delivery, e := s.Delivery.ByOrder(o.ID)
		if e != nil {
			return domain.CustodyEvent{}, false, fmt.Errorf("conflict: delivery proof unavailable")
		}
		photoFound := false
		for _, proof := range delivery.Proofs {
			if proof.Type == "PHOTO" && proof.OperationID == event.EvidenceRef && !proof.Pending {
				photoFound = true
				break
			}
		}
		if !photoFound {
			return domain.CustodyEvent{}, false, fmt.Errorf("conflict: referenced condition photo is not uploaded for this order")
		}
	}
	event.SealID = strings.TrimSpace(event.SealID)
	event.Condition = strings.TrimSpace(event.Condition)
	event.EvidenceRef = strings.TrimSpace(event.EvidenceRef)
	event.ReceiverName = strings.TrimSpace(event.ReceiverName)
	event.IdempotencyKey = strings.TrimSpace(event.IdempotencyKey)
	if len(event.SealID) < 1 || len(event.SealID) > 100 || len(event.Condition) < 1 || len(event.Condition) > 500 || len(event.EvidenceRef) > 500 || event.IdempotencyKey == "" || len(event.IdempotencyKey) > 128 {
		return domain.CustodyEvent{}, false, fmt.Errorf("invalid: seal, condition and idempotency key are required; text fields exceed limits")
	}
	if event.Stage == "DELIVERED" && event.EvidenceRef == "" {
		return domain.CustodyEvent{}, false, fmt.Errorf("invalid: condition photo proof reference required for delivery")
	}
	if event.Stage == "RECEIVED" && (event.ReceiverName == "" || len(event.ReceiverName) > 160) {
		return domain.CustodyEvent{}, false, fmt.Errorf("invalid: receiverName required for receipt")
	}
	if event.Stage != "RECEIVED" && event.ReceiverName != "" {
		return domain.CustodyEvent{}, false, fmt.Errorf("invalid: receiverName only applies to receipt")
	}
	if len(event.SerialNumbers) == 0 || len(event.SerialNumbers) > 100 {
		return domain.CustodyEvent{}, false, fmt.Errorf("invalid: 1 to 100 serial numbers required")
	}
	seen := map[string]bool{}
	for i, n := range event.SerialNumbers {
		n = strings.TrimSpace(n)
		if n == "" || len(n) > 120 || seen[strings.ToLower(n)] {
			return domain.CustodyEvent{}, false, fmt.Errorf("invalid: serial numbers must be unique non-empty values up to 120 characters")
		}
		seen[strings.ToLower(n)] = true
		event.SerialNumbers[i] = n
	}
	prior, err := s.Repo.ListCustodyEvents(o.ID)
	if err != nil {
		return domain.CustodyEvent{}, false, err
	}
	if len(prior) > 0 {
		first := prior[0]
		if first.SealID != event.SealID || !sameSerials(first.SerialNumbers, event.SerialNumbers) {
			return domain.CustodyEvent{}, false, fmt.Errorf("conflict: seal and serials must match the loaded custody record")
		}
	}
	event.RecordedBy = profile.UserID
	return s.Repo.AddCustodyEvent(o, event)
}

func sameSerials(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	norm := func(in []string) []string {
		out := append([]string{}, in...)
		for i := range out {
			out[i] = strings.ToLower(strings.TrimSpace(out[i]))
		}
		return out
	}
	x, y := norm(a), norm(b)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func (s Service) PendingReceipts(profile *authorization.Profile) ([]domain.ReceiptTask, error) {
	orders, err := s.List(profile, domain.ListFilter{})
	if err != nil {
		return nil, err
	}
	out := []domain.ReceiptTask{}
	for _, o := range orders {
		t, e := s.Tracking(profile, o.ID)
		if e != nil {
			return nil, e
		}
		if t.Delivery != nil && (t.Delivery.Outcome == "DELIVERED" || t.Delivery.Outcome == "PARTIAL") && t.Receipt == nil {
			out = append(out, domain.ReceiptTask{Order: o, Tracking: t})
		}
	}
	return out, nil
}

func (s Service) Receipt(profile *authorization.Profile, id string) (domain.Receipt, []domain.ReceiptIssue, error) {
	o, err := s.Get(profile, id)
	if err != nil {
		return domain.Receipt{}, nil, err
	}
	r, issues, e := s.Repo.GetReceipt(o.ID)
	if e != nil && strings.Contains(e.Error(), "not found") {
		return domain.Receipt{}, nil, ErrNotFound
	}
	return r, issues, e
}

func (s Service) ConfirmReceipt(profile *authorization.Profile, id string, req domain.ReceiptConfirmation) (domain.Receipt, []domain.ReceiptIssue, bool, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermReceiptConfirm) {
		return domain.Receipt{}, nil, false, ErrForbidden
	}
	o, err := s.Get(profile, id)
	if err != nil {
		return domain.Receipt{}, nil, false, err
	}
	if req.ReceivedUnits <= 0 || req.ReceivedUnits > o.OrderUnits {
		return domain.Receipt{}, nil, false, fmt.Errorf("invalid: receivedUnits must be between 1 and expected units")
	}
	if existing, issues, e := s.Repo.GetReceipt(o.ID); e == nil {
		if existing.ReceivedUnits != req.ReceivedUnits {
			return domain.Receipt{}, nil, false, fmt.Errorf("conflict: receipt already confirmed with different quantity")
		}
		return existing, issues, false, nil
	} else if !strings.Contains(e.Error(), "not found") {
		return domain.Receipt{}, nil, false, e
	}
	if s.Delivery == nil {
		return domain.Receipt{}, nil, false, fmt.Errorf("conflict: delivery outcome unavailable")
	}
	d, err := s.Delivery.ByOrder(o.ID)
	if err != nil {
		return domain.Receipt{}, nil, false, fmt.Errorf("conflict: delivery outcome is not available")
	}
	if d.Outcome != "DELIVERED" && d.Outcome != "PARTIAL" {
		return domain.Receipt{}, nil, false, fmt.Errorf("conflict: receipt is unavailable for %s delivery", d.Outcome)
	}
	if req.ReceivedUnits <= 0 || req.ReceivedUnits > o.OrderUnits {
		return domain.Receipt{}, nil, false, fmt.Errorf("invalid: receivedUnits must be between 1 and expected units")
	}
	if req.ReceivedUnits < o.OrderUnits && req.Issue == nil {
		return domain.Receipt{}, nil, false, fmt.Errorf("invalid: a discrepancy issue is required when received units are below expected units")
	}
	if req.Issue != nil {
		req.Issue.IssueType = strings.ToUpper(strings.TrimSpace(req.Issue.IssueType))
		if err := validateReceiptIssue(*req.Issue, o.OrderUnits); err != nil {
			return domain.Receipt{}, nil, false, err
		}
		if req.ReceivedUnits < o.OrderUnits && req.Issue.AffectedUnits == 0 {
			return domain.Receipt{}, nil, false, fmt.Errorf("invalid: discrepancy issue must identify affected units")
		}
	}
	req.ConfirmedBy = profile.UserID
	req.DeliveryRunID = d.RunID
	req.DeliveryStopID = d.StopID
	req.DeliveryOutcome = d.Outcome
	req.ExpectedUnits = o.OrderUnits
	r, issues, created, err := s.Repo.ConfirmReceipt(o, req)
	if err != nil {
		return r, issues, false, err
	}
	if created {
		s.auditReceipt("RECEIPT_CONFIRMED", profile, o.OrderRef, map[string]any{"receivedUnits": r.ReceivedUnits, "expectedUnits": r.ExpectedUnits, "status": r.Status})
		if req.Issue != nil {
			s.auditReceipt("RECEIPT_ISSUE_REPORTED", profile, o.OrderRef, map[string]any{"issueType": req.Issue.IssueType, "affectedUnits": req.Issue.AffectedUnits})
			telemetry.ReceiptIssues.WithLabelValues(strings.ToUpper(req.Issue.IssueType)).Inc()
		}
		telemetry.ReceiptConfirmations.Inc()
		if d.CompletedAt != nil {
			delay := time.Since(*d.CompletedAt).Seconds()
			if delay < 0 {
				delay = 0
			}
			telemetry.ReceiptDelay.Observe(delay)
		}
	}
	return r, issues, created, nil
}

func (s Service) ReportReceiptIssue(profile *authorization.Profile, id string, req domain.ReceiptIssueRequest) (domain.Receipt, domain.ReceiptIssue, bool, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryIssueCreate) {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, ErrForbidden
	}
	o, err := s.Get(profile, id)
	if err != nil {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, err
	}
	req.IssueType = strings.ToUpper(strings.TrimSpace(req.IssueType))
	if err := validateReceiptIssue(req, o.OrderUnits); err != nil {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, err
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.IdempotencyKey == "" {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, fmt.Errorf("invalid: Idempotency-Key required")
	}
	r, i, created, err := s.Repo.AddReceiptIssue(o, req, profile.UserID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return domain.Receipt{}, domain.ReceiptIssue{}, false, ErrNotFound
		}
		return r, i, false, err
	}
	if created {
		s.auditReceipt("RECEIPT_ISSUE_REPORTED", profile, o.OrderRef, map[string]any{"issueType": i.IssueType, "affectedUnits": i.AffectedUnits})
		telemetry.ReceiptIssues.WithLabelValues(i.IssueType).Inc()
	}
	return r, i, created, nil
}

func (s Service) ReceiptIssues(profile *authorization.Profile) ([]domain.ReceiptIssueView, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermOrderViewAll) {
		return nil, ErrForbidden
	}
	return s.Repo.ListReceiptIssues()
}

type ReceiptAuditPublisher interface {
	PublishReceipt(action, actorID, orderRef string, state map[string]any) error
}

func (s Service) auditReceipt(action string, p *authorization.Profile, ref string, state map[string]any) {
	if a, ok := s.Audit.(ReceiptAuditPublisher); ok {
		actor := ""
		if p != nil {
			actor = p.UserID
		}
		_ = a.PublishReceipt(action, actor, ref, state)
	}
}

func validateReceiptIssue(i domain.ReceiptIssueRequest, expected int) error {
	i.IssueType = strings.ToUpper(strings.TrimSpace(i.IssueType))
	if !validReceiptIssueTypes[i.IssueType] {
		return fmt.Errorf("invalid: issueType")
	}
	if i.AffectedUnits < 0 || i.AffectedUnits > expected {
		return fmt.Errorf("invalid: affectedUnits")
	}
	if strings.TrimSpace(i.IdempotencyKey) == "" {
		return fmt.Errorf("invalid: Idempotency-Key required")
	}
	if len(i.Note) > 1000 {
		return fmt.Errorf("invalid: note too long")
	}
	return nil
}

func (s Service) Create(profile *authorization.Profile, bearer string, req domain.CreateRequest) (domain.Order, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermOrderCreate) {
		return domain.Order{}, ErrForbidden
	}
	if len(profile.OutletIDs) == 0 {
		return domain.Order{}, fmt.Errorf("%w: store manager profile missing", ErrNotFound)
	}
	if err := validateCreate(req); err != nil {
		return domain.Order{}, err
	}
	outletID := profile.OutletIDs[0]
	outlet, err := s.Outlets.Outlet(outletID, bearer)
	if err != nil {
		return domain.Order{}, fmt.Errorf("%w: outlet", ErrNotFound)
	}
	requested, err := time.Parse("2006-01-02", req.RequestedDeliveryDate)
	if err != nil {
		return domain.Order{}, fmt.Errorf("%w: requestedDeliveryDate", ErrInvalid)
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	if s.Cutoff != nil {
		cutoffTime := "16:00"
		if s.Policy != nil {
			if configured, err := s.Policy.CutoffLocalTime(); err == nil && configured != "" {
				cutoffTime = configured
			}
		}
		calendar := s.Cutoff
		if s.Calendar != nil {
			loc, _ := time.LoadLocation(cutoff.Zone)
			if loc == nil {
				loc = time.UTC
			}
			from, to := now.In(loc).Format("2006-01-02"), requested.In(loc).AddDate(0, 0, 14).Format("2006-01-02")
			if to >= from {
				if days, calendarErr := s.Calendar.OperatingDays(from, to); calendarErr == nil {
					configured := make([]cutoff.Day, 0, len(days))
					for _, d := range days {
						parsed, parseErr := time.ParseInLocation("2006-01-02", d.Date, loc)
						if parseErr == nil {
							configured = append(configured, cutoff.Day{Date: parsed, IsOperating: d.IsOperating})
						}
					}
					if len(configured) > 0 {
						calendar = cutoff.Load(configured)
					}
				}
			}
		}
		requested = calendar.AdjustWithCutoff(requested, now, cutoffTime)
	}
	order := domain.Order{
		OutletID:               outletID,
		Brand:                  outlet.Brand,
		RequestedDeliveryDate:  requested.Format("2006-01-02"),
		OrderUnits:             req.OrderUnits,
		OrderWeightKg:          req.OrderWeightKg,
		OrderVolumeM3:          req.OrderVolumeM3,
		TemperatureRequirement: req.TemperatureRequirement,
		Status:                 domain.StatusConfirmed,
		CreatedBy:              profile.UserID,
	}
	created, err := s.Repo.Create(order)
	if err != nil {
		return domain.Order{}, err
	}
	telemetry.OrdersCreated.Inc()
	if s.Audit != nil {
		_ = s.Audit.PublishCreated("", profile.UserID, created.OrderRef, map[string]any{
			"orderRef": created.OrderRef,
			"outletId": created.OutletID,
			"status":   created.Status,
		})
	}
	return created, nil
}

func (s Service) List(profile *authorization.Profile, filter domain.ListFilter) ([]domain.Order, error) {
	if profile == nil {
		return nil, ErrForbidden
	}
	if authorization.HasPermission(profile.Roles, authorization.PermOrderViewAll) || len(profile.Roles) == 0 {
		return s.Repo.List(filter)
	}
	if !authorization.HasPermission(profile.Roles, authorization.PermOrderViewOwn) || len(profile.OutletIDs) == 0 {
		return nil, ErrForbidden
	}
	filter.OutletID = profile.OutletIDs[0]
	return s.Repo.List(filter)
}

func (s Service) Forecast(profile *authorization.Profile) (domain.Forecast, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermOrderViewAll) {
		return domain.Forecast{}, ErrForbidden
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	return s.Repo.Forecast(now)
}

func (s Service) ImportOrders(profile *authorization.Profile, bearer, source string, items []domain.Order) ([]domain.ImportResult, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermOrderViewAll) {
		return nil, ErrForbidden
	}
	source = strings.TrimSpace(source)
	if len(source) < 1 || len(source) > 40 {
		return nil, fmt.Errorf("invalid: sourceSystem must be 1 to 40 characters")
	}
	for _, r := range source {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return nil, fmt.Errorf("invalid: sourceSystem may contain letters, digits, underscore, and hyphen")
		}
	}
	if len(items) == 0 || len(items) > 500 {
		return nil, fmt.Errorf("invalid: import must contain 1 to 500 orders")
	}
	seen := map[string]bool{}
	for i := range items {
		o := &items[i]
		o.ExternalOrderID = strings.TrimSpace(o.ExternalOrderID)
		if o.ExternalOrderID == "" || len(o.ExternalOrderID) > 120 || seen[o.ExternalOrderID] {
			return nil, fmt.Errorf("invalid: missing or duplicate external order ID")
		}
		seen[o.ExternalOrderID] = true
		if err := validateCreate(domain.CreateRequest{RequestedDeliveryDate: o.RequestedDeliveryDate, OrderUnits: o.OrderUnits, OrderWeightKg: o.OrderWeightKg, OrderVolumeM3: o.OrderVolumeM3, TemperatureRequirement: o.TemperatureRequirement}); err != nil {
			return nil, err
		}
		outlet, err := s.Outlets.Outlet(strings.TrimSpace(o.OutletID), bearer)
		if err != nil {
			return nil, fmt.Errorf("invalid: unknown outlet %s", o.OutletID)
		}
		if strings.TrimSpace(o.Brand) != "" && strings.TrimSpace(o.Brand) != outlet.Brand {
			return nil, fmt.Errorf("invalid: brand does not match outlet %s", o.OutletID)
		}
		o.OutletID = outlet.ID
		o.Brand = outlet.Brand
		o.Status = domain.StatusConfirmed
		o.CreatedBy = profile.UserID
	}
	results, err := s.Repo.ImportOrders(source, items)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		if r.Created {
			telemetry.OrdersCreated.Inc()
			if s.Audit != nil {
				_ = s.Audit.PublishCreated("", profile.UserID, r.Order.OrderRef, map[string]any{"orderRef": r.Order.OrderRef, "sourceSystem": source, "externalOrderId": r.Order.ExternalOrderID, "status": r.Order.Status})
			}
		}
	}
	return results, nil
}

func (s Service) Get(profile *authorization.Profile, id string) (domain.Order, error) {
	if profile == nil {
		return domain.Order{}, ErrForbidden
	}
	order, err := s.Repo.Get(id)
	if err != nil {
		return domain.Order{}, ErrNotFound
	}
	if authorization.HasPermission(profile.Roles, authorization.PermOrderViewAll) || len(profile.Roles) == 0 {
		return order, nil
	}
	if !authorization.HasPermission(profile.Roles, authorization.PermOrderViewOwn) {
		return domain.Order{}, ErrForbidden
	}
	for _, oid := range profile.OutletIDs {
		if oid == order.OutletID {
			return order, nil
		}
	}
	return domain.Order{}, ErrForbidden
}

func validateCreate(req domain.CreateRequest) error {
	if err := validation.Required("requestedDeliveryDate", req.RequestedDeliveryDate); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	if _, err := time.Parse("2006-01-02", req.RequestedDeliveryDate); err != nil {
		return fmt.Errorf("%w: requestedDeliveryDate", ErrInvalid)
	}
	if req.OrderUnits <= 0 || req.OrderWeightKg <= 0 || req.OrderVolumeM3 <= 0 {
		return fmt.Errorf("%w: quantities must be greater than zero", ErrInvalid)
	}
	temp := strings.ToLower(string(req.TemperatureRequirement))
	if err := validation.OneOf("temperatureRequirement", temp, "ambient", "chilled"); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	return nil
}
