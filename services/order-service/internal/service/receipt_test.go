package service

import (
	"errors"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

type deliveryReaderStub struct {
	value domain.DeliveryTracking
	err   error
}

func (d deliveryReaderStub) ByOrder(string) (domain.DeliveryTracking, error) { return d.value, d.err }

type planningReaderStub struct {
	value domain.PlanningTracking
	err   error
}

func (p planningReaderStub) ByOrder(string) (domain.PlanningTracking, error) { return p.value, p.err }

type receiptAuditStub struct{ actions []string }

func (*receiptAuditStub) PublishCreated(string, string, string, map[string]any) error { return nil }
func (a *receiptAuditStub) PublishReceipt(action, _, _ string, _ map[string]any) error {
	a.actions = append(a.actions, action)
	return nil
}

func receiptService(outcome string) (Service, *store.Memory, *receiptAuditStub, *authorization.Profile) {
	repo := store.NewMemory()
	_, _ = repo.Create(domain.Order{ID: "order-1", OrderRef: "ORD000001", OutletID: "OUT034", OrderUnits: 20, Status: domain.StatusConfirmed})
	audit := &receiptAuditStub{}
	s := Service{Repo: repo, Planning: planningReaderStub{value: domain.PlanningTracking{State: "PLANNED", TripID: "trip-1", PlanRef: "PLAN000001", PlannedArrivalAt: timePtr(time.Date(2026, 9, 30, 7, 0, 0, 0, time.UTC))}}, Delivery: deliveryReaderStub{value: domain.DeliveryTracking{RunID: "run-1", TripID: "trip-1", RunStatus: "completed", StopID: "stop-1", Outcome: outcome, CompletedAt: timePtr(time.Now().Add(-time.Minute)), Proofs: []domain.ProofSummary{{Type: "SIGNATURE", MimeType: "image/png"}}}}, Audit: audit}
	p := &authorization.Profile{UserID: "USR001", Roles: []string{authorization.RoleStoreManager}, OutletIDs: []string{"OUT034"}}
	return s, repo, audit, p
}
func timePtr(t time.Time) *time.Time { return &t }

func TestConfirmReceiptExactUnitsIsIdempotent(t *testing.T) {
	s, _, audit, p := receiptService("DELIVERED")
	r, _, created, err := s.ConfirmReceipt(p, "order-1", domain.ReceiptConfirmation{ReceivedUnits: 20})
	if err != nil || !created || r.Status != "confirmed" || r.ExpectedUnits != 20 {
		t.Fatalf("first confirmation: %+v %v", r, err)
	}
	r2, _, created, err := s.ConfirmReceipt(p, "order-1", domain.ReceiptConfirmation{ReceivedUnits: 20})
	if err != nil || created || r2.ID != r.ID {
		t.Fatalf("duplicate confirmation: %+v created=%v err=%v", r2, created, err)
	}
	if len(audit.actions) != 1 || audit.actions[0] != "RECEIPT_CONFIRMED" {
		t.Fatalf("audits %v", audit.actions)
	}
	if _, _, _, err = s.ConfirmReceipt(p, "order-1", domain.ReceiptConfirmation{ReceivedUnits: 21}); err == nil {
		t.Fatal("received units above expected accepted")
	}
}

func TestTechCustodyRequiresOrderedConsistentHandoffs(t *testing.T) {
	repo := store.NewMemory()
	_, _ = repo.Create(domain.Order{ID: "tech-order", OrderRef: "ORDTECH", OutletID: "OUT034", Brand: "Tech", OrderUnits: 1, Status: domain.StatusConfirmed})
	s := Service{Repo: repo, Planning: planningReaderStub{value: domain.PlanningTracking{TripID: "trip-1", Depot: "DEPOT_NORTH"}}, Delivery: deliveryReaderStub{value: domain.DeliveryTracking{VehicleID: "VEH001", Proofs: []domain.ProofSummary{{OperationID: "proof-operation-1", Type: "PHOTO", Pending: false}}}}}
	loader := &authorization.Profile{UserID: "USR004", Roles: []string{authorization.RoleLoader}, Depot: "DEPOT_NORTH"}
	driver := &authorization.Profile{UserID: "USR006", Roles: []string{authorization.RoleDriver}, VehicleID: "VEH001"}
	manager := &authorization.Profile{UserID: "USR001", Roles: []string{authorization.RoleStoreManager}, OutletIDs: []string{"OUT034"}}
	base := domain.CustodyEvent{SealID: "SEAL-7", SerialNumbers: []string{"SN-001"}, Condition: "package intact"}
	loaded := base
	loaded.Stage = "LOADED"
	loaded.IdempotencyKey = "tech-load-1"
	if _, created, err := s.RecordCustody(loader, "tech-order", loaded); err != nil || !created {
		t.Fatalf("loaded created=%v err=%v", created, err)
	}
	wrongDepot := *loader
	wrongDepot.Depot = "DEPOT_SOUTH"
	if _, _, err := s.RecordCustody(&wrongDepot, "tech-order", domain.CustodyEvent{Stage: "LOADED", SealID: "SEAL-7", SerialNumbers: []string{"SN-001"}, Condition: "intact", IdempotencyKey: "tech-load-wrong-depot"}); err == nil {
		t.Fatal("loader from another depot recorded custody")
	}
	wrongVehicle := *driver
	wrongVehicle.VehicleID = "VEH002"
	if _, _, err := s.RecordCustody(&wrongVehicle, "tech-order", domain.CustodyEvent{Stage: "DISPATCHED", SealID: "SEAL-7", SerialNumbers: []string{"SN-001"}, Condition: "intact", IdempotencyKey: "tech-dispatch-wrong-vehicle"}); err == nil {
		t.Fatal("unassigned driver recorded custody")
	}
	if _, _, err := s.RecordCustody(driver, "tech-order", domain.CustodyEvent{Stage: "DELIVERED", SealID: "SEAL-7", SerialNumbers: []string{"SN-001"}, Condition: "intact", IdempotencyKey: "tech-deliver-early"}); err == nil {
		t.Fatal("out-of-order delivery accepted")
	}
	dispatched := base
	dispatched.Stage = "DISPATCHED"
	dispatched.IdempotencyKey = "tech-dispatch-1"
	if _, _, err := s.RecordCustody(driver, "tech-order", dispatched); err != nil {
		t.Fatal(err)
	}
	missingPhoto := base
	missingPhoto.Stage = "DELIVERED"
	missingPhoto.IdempotencyKey = "tech-delivery-no-photo"
	if _, _, err := s.RecordCustody(driver, "tech-order", missingPhoto); err == nil {
		t.Fatal("Tech delivery without photo reference accepted")
	}
	unuploaded := base
	unuploaded.Stage = "DELIVERED"
	unuploaded.EvidenceRef = "unknown-photo"
	unuploaded.IdempotencyKey = "tech-delivery-unuploaded"
	if _, _, err := s.RecordCustody(driver, "tech-order", unuploaded); err == nil {
		t.Fatal("unuploaded photo reference accepted")
	}
	delivered := base
	delivered.Stage = "DELIVERED"
	delivered.Condition = "handed to outlet intact"
	delivered.EvidenceRef = "proof-operation-1"
	delivered.IdempotencyKey = "tech-delivery-1"
	if _, _, err := s.RecordCustody(driver, "tech-order", delivered); err != nil {
		t.Fatal(err)
	}
	received := base
	received.Stage = "RECEIVED"
	received.Condition = "received intact"
	received.ReceiverName = "Outlet manager"
	received.IdempotencyKey = "tech-receipt-1"
	if _, created, err := s.RecordCustody(manager, "tech-order", received); err != nil || !created {
		t.Fatalf("received created=%v err=%v", created, err)
	}
	bad := received
	bad.IdempotencyKey = "tech-receipt-2"
	bad.SerialNumbers = []string{"SN-OTHER"}
	if _, _, err := s.RecordCustody(manager, "tech-order", bad); err == nil {
		t.Fatal("changed serial accepted after loading")
	}
	tracking, err := s.Tracking(manager, "tech-order")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracking.Custody) != 4 || tracking.Custody[0].RecordedBy != "USR004" || tracking.Custody[3].ReceiverName != "Outlet manager" {
		t.Fatalf("custody journal not complete: %+v", tracking.Custody)
	}
}

func TestPartialReceiptRequiresAtomicIssue(t *testing.T) {
	s, _, audit, p := receiptService("PARTIAL")
	if _, _, _, err := s.ConfirmReceipt(p, "order-1", domain.ReceiptConfirmation{ReceivedUnits: 16}); err == nil {
		t.Fatal("partial receipt without discrepancy accepted")
	}
	req := domain.ReceiptConfirmation{ReceivedUnits: 16, Issue: &domain.ReceiptIssueRequest{IssueType: "MISSING", AffectedUnits: 4, Note: "Four units short", IdempotencyKey: "receipt-short-1"}}
	r, issues, created, err := s.ConfirmReceipt(p, "order-1", req)
	if err != nil || !created || r.Status != "confirmed_with_issue" || len(issues) != 1 {
		t.Fatalf("receipt=%+v issues=%v err=%v", r, issues, err)
	}
	_, issues, created, err = s.ConfirmReceipt(p, "order-1", req)
	if err != nil || created || len(issues) != 1 {
		t.Fatalf("duplicate receipt=%v created=%v err=%v", issues, created, err)
	}
	if len(audit.actions) != 2 || audit.actions[1] != "RECEIPT_ISSUE_REPORTED" {
		t.Fatalf("audits %v", audit.actions)
	}
}

func TestNotDeliveredCannotBeReceivedAndOutletIsScoped(t *testing.T) {
	s, _, _, p := receiptService("NOT_DELIVERED")
	if _, _, _, err := s.ConfirmReceipt(p, "order-1", domain.ReceiptConfirmation{ReceivedUnits: 1}); err == nil {
		t.Fatal("NOT_DELIVERED receipt accepted")
	}
	other := &authorization.Profile{UserID: "USR003", Roles: []string{authorization.RoleStoreManager}, OutletIDs: []string{"OUT021"}}
	if _, err := s.Tracking(other, "order-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-outlet read err=%v", err)
	}
}

func TestIssueIdempotencyAndBounds(t *testing.T) {
	s, _, _, p := receiptService("DELIVERED")
	_, _, _, err := s.ConfirmReceipt(p, "order-1", domain.ReceiptConfirmation{ReceivedUnits: 20})
	if err != nil {
		t.Fatal(err)
	}
	req := domain.ReceiptIssueRequest{IssueType: "DAMAGED", AffectedUnits: 2, IdempotencyKey: "damage-1"}
	r, i, created, err := s.ReportReceiptIssue(p, "order-1", req)
	if err != nil || !created || r.Status != "confirmed_with_issue" || i.IssueType != "DAMAGED" {
		t.Fatalf("%+v %+v %v", r, i, err)
	}
	_, i, created, err = s.ReportReceiptIssue(p, "order-1", req)
	if err != nil || created || i.ID == "" {
		t.Fatalf("duplicate %+v %v %v", i, created, err)
	}
	if _, _, _, err = s.ReportReceiptIssue(p, "order-1", domain.ReceiptIssueRequest{IssueType: "OTHER", AffectedUnits: 21, IdempotencyKey: "damage-2"}); err == nil {
		t.Fatal("issue units above order accepted")
	}
}
