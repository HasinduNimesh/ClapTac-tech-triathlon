package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

type fakeAuth struct{ p *auth.Principal }

func (f fakeAuth) Authenticate(*http.Request) (*auth.Principal, error) {
	if f.p == nil {
		return nil, errString("missing bearer token")
	}
	return f.p, nil
}

type errString string

func (e errString) Error() string { return string(e) }

type fakeProfiles map[string]authorization.Profile

func (f fakeProfiles) Resolve(_ context.Context, subject string) (*authorization.Profile, error) {
	if p, ok := f[subject]; ok {
		cp := p
		return &cp, nil
	}
	return &authorization.Profile{Subject: subject}, nil
}

type stubPlanner struct {
	created                       domain.Plan
	createErr                     error
	generated                     bool
	generateErr                   error
	assignFails                   []domain.Result
	assignErr                     error
	confirmErr                    error
	proposalPlanID                string
	proposalVehicleID             string
	breakdownPlanID               string
	breakdownSourceVehicleID      string
	breakdownReplacementVehicleID string
	breakdownTripNumber           int
}

func (s *stubPlanner) Acknowledge(context.Context, *authorization.Profile, string, int) error {
	return nil
}
func (s *stubPlanner) Revise(context.Context, *authorization.Profile, string) error { return nil }
func (s *stubPlanner) BreakdownProposals(_ context.Context, planID, vehicleID string) (map[string]any, error) {
	s.proposalPlanID, s.proposalVehicleID = planID, vehicleID
	return map[string]any{"items": []any{}}, nil
}
func (s *stubPlanner) ConfirmBreakdown(_ context.Context, _ *authorization.Profile, planID, sourceVehicleID, replacementVehicleID string, tripNumber int) (map[string]any, error) {
	s.breakdownPlanID = planID
	s.breakdownSourceVehicleID = sourceVehicleID
	s.breakdownReplacementVehicleID = replacementVehicleID
	s.breakdownTripNumber = tripNumber
	return map[string]any{"status": "confirmed"}, nil
}

func (s *stubPlanner) Create(context.Context, *authorization.Profile, string) (domain.Plan, bool, error) {
	return s.created, true, s.createErr
}
func (s *stubPlanner) Get(context.Context, string) (map[string]any, error) {
	return map[string]any{"plan": s.created}, nil
}
func (s *stubPlanner) GetByDate(context.Context, string) (map[string]any, error) {
	return map[string]any{"plan": s.created}, nil
}
func (s *stubPlanner) Generate(context.Context, *authorization.Profile, string) (domain.GenerateResult, error) {
	if s.generated {
		return domain.GenerateResult{}, fmt.Errorf("conflict: generate already applied; reset first")
	}
	s.generated = true
	if s.generateErr != nil {
		return domain.GenerateResult{}, s.generateErr
	}
	return domain.GenerateResult{Allocated: 1}, nil
}
func (s *stubPlanner) Simulate(context.Context, string) (domain.GenerateResult, error) {
	return domain.GenerateResult{Allocated: 1}, nil
}
func (s *stubPlanner) Reset(context.Context, *authorization.Profile, string) error { return nil }
func (s *stubPlanner) Assign(context.Context, *authorization.Profile, string, string, string, int) (domain.Allocation, []domain.Result, error) {
	return domain.Allocation{}, s.assignFails, s.assignErr
}
func (s *stubPlanner) Reassign(context.Context, *authorization.Profile, string, string, string, int) ([]domain.Result, error) {
	return nil, nil
}
func (s *stubPlanner) Remove(context.Context, *authorization.Profile, string, string) error {
	return nil
}
func (s *stubPlanner) Defer(context.Context, *authorization.Profile, string, string, string, string) error {
	return nil
}
func (s *stubPlanner) Confirm(context.Context, *authorization.Profile, string) error {
	return s.confirmErr
}
func (s *stubPlanner) InternalTrips(context.Context, string, string) ([]domain.InternalTrip, error) {
	return []domain.InternalTrip{}, nil
}
func (s *stubPlanner) InternalTrip(context.Context, string) (domain.InternalTrip, error) {
	return domain.InternalTrip{}, nil
}
func (s *stubPlanner) InternalOrder(context.Context, string) (domain.OrderTracking, error) {
	return domain.OrderTracking{State: "CONFIRMED"}, nil
}
func (s *stubPlanner) ListDisruptionRisks(context.Context, string) ([]domain.DisruptionRisk, error) {
	return []domain.DisruptionRisk{}, nil
}
func (s *stubPlanner) CreateDisruptionRisk(_ context.Context, _ *authorization.Profile, risk domain.DisruptionRisk) (domain.DisruptionRisk, error) {
	risk.ID = "risk-1"
	return risk, nil
}
func (s *stubPlanner) OverrideDisruptionRisk(_ context.Context, _ *authorization.Profile, riskID, decision, severity, reason string) (domain.DisruptionRisk, error) {
	return domain.DisruptionRisk{ID: riskID, OverrideDecision: decision, OverrideSeverity: severity, OverrideReason: reason}, nil
}

func testRouter(subject string, svc Planner) http.Handler {
	profiles := fakeProfiles{
		"usr-store-manager": {UserID: "USR001", Subject: "usr-store-manager", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT034"}},
		"usr-dispatcher":    {UserID: "USR002", Subject: "usr-dispatcher", Roles: []string{"DISPATCHER"}},
		"usr-loader":        {UserID: "USR003", Subject: "usr-loader", Roles: []string{"LOADER"}},
		"usr-driver":        {UserID: "USR004", Subject: "usr-driver", Roles: []string{"DRIVER"}},
	}
	var p *auth.Principal
	if subject != "" {
		p = &auth.Principal{Subject: subject}
	}
	h := Handler{Authn: fakeAuth{p: p}, Profiles: profiles, Service: svc}
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

func TestUnauthenticatedCreateRejected(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans", bytes.NewReader([]byte(`{"deliveryDate":"2026-09-29"}`)))
	testRouter("", &stubPlanner{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestDisruptionRiskCreateIsDispatcherOnlyAndReturnsProvenance(t *testing.T) {
	body := []byte(`{"deliveryDate":"2026-10-01","scope":"DISTRICT","scopeKey":"Colombo North","riskType":"HEAVY_RAIN","severity":"HIGH","summary":"Flooding reported near bridge","source":"Manual report","sourceReference":"incident-42","confidence":0.8}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/planning/disruption-risks", bytes.NewReader(body))
	testRouter("usr-dispatcher", &stubPlanner{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"source":"Manual report"`) {
		t.Fatalf("dispatcher create returned %d %s", rec.Code, rec.Body.String())
	}

	denied := httptest.NewRecorder()
	testRouter("usr-driver", &stubPlanner{}).ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/v1/planning/disruption-risks", bytes.NewReader(body)))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("driver create returned %d: %s", denied.Code, denied.Body.String())
	}
}

func TestStoreManagerCannotCreateOrView(t *testing.T) {
	r := testRouter("usr-store-manager", &stubPlanner{created: domain.Plan{ID: "p1"}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans", bytes.NewReader([]byte(`{"deliveryDate":"2026-09-29"}`))))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/plans/p1", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("get %d", rec.Code)
	}
}

func TestDispatcherCanCreateAndView(t *testing.T) {
	r := testRouter("usr-dispatcher", &stubPlanner{created: domain.Plan{ID: "p1", PlanRef: "PLAN000001"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans", bytes.NewReader([]byte(`{"deliveryDate":"2026-09-29"}`)))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/plans/p1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get %d", rec.Code)
	}
}

func TestGenerateTwiceConflicts(t *testing.T) {
	svc := &stubPlanner{created: domain.Plan{ID: "p1"}}
	r := testRouter("usr-dispatcher", svc)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans/p1/generate", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("first generate %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans/p1/generate", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("second generate %d", rec.Code)
	}
}

func TestInternalTripsPermissions(t *testing.T) {
	r := testRouter("usr-dispatcher", &stubPlanner{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/internal/trips?date=2026-09-29", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("dispatcher internal %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	testRouter("usr-store-manager", &stubPlanner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/internal/trips?date=2026-09-29", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("store manager internal %d", rec.Code)
	}
}

func TestPlanAcknowledgementRolePermissions(t *testing.T) {
	r := testRouter("usr-loader", &stubPlanner{})
	body := bytes.NewBufferString(`{"version":1}`)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans/p1/acknowledgements", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("loader acknowledgement %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	testRouter("usr-dispatcher", &stubPlanner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans/p1/acknowledgements", bytes.NewBufferString(`{"version":1}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("dispatcher should not acknowledge as a field actor: %d", rec.Code)
	}
}

func TestInternalOrderRequiresNarrowScope(t *testing.T) {
	h := Handler{Authn: fakeAuth{p: &auth.Principal{Subject: "svc", Scopes: []string{authorization.PermPlansReadInternal}}}, Profiles: fakeProfiles{}, Service: &stubPlanner{}}
	r := chi.NewRouter()
	h.Routes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/internal/orders/order-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("service scope status %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	testRouter("usr-dispatcher", &stubPlanner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/internal/orders/order-1", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("dispatcher without internal scope status %d", rec.Code)
	}
}

func TestAssignConstraintConflict(t *testing.T) {
	svc := &stubPlanner{
		assignErr:   fmt.Errorf("allocation_invalid"),
		assignFails: []domain.Result{{Valid: false, ReasonCode: domain.ReasonWeightExceeded}},
	}
	r := testRouter("usr-dispatcher", svc)
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"orderId": "o1", "vehicleId": "v1", "tripNumber": 1})
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans/p1/allocations", bytes.NewReader(body)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("assign %d %s", rec.Code, rec.Body.String())
	}
}

func TestBreakdownProposalIsDispatcherReadableAndRequiresVehicle(t *testing.T) {
	svc := &stubPlanner{}
	r := testRouter("usr-dispatcher", svc)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/plans/p1/breakdowns/proposals?vehicleId=VEH001", nil))
	if rec.Code != http.StatusOK || svc.proposalPlanID != "p1" || svc.proposalVehicleID != "VEH001" {
		t.Fatalf("proposal request status=%d plan=%q vehicle=%q body=%s", rec.Code, svc.proposalPlanID, svc.proposalVehicleID, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/plans/p1/breakdowns/proposals", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("proposal without source vehicle should be rejected, got %d", rec.Code)
	}
	for _, subject := range []string{"usr-store-manager", "usr-loader", "usr-driver"} {
		rec = httptest.NewRecorder()
		testRouter(subject, &stubPlanner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/plans/p1/breakdowns/proposals?vehicleId=VEH001", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s must not inspect dispatcher breakdown proposals, got %d", subject, rec.Code)
		}
	}
}

func TestBreakdownReassignmentRequiresDispatcherAndValidRequest(t *testing.T) {
	svc := &stubPlanner{}
	r := testRouter("usr-dispatcher", svc)
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"sourceVehicleId":"VEH001","replacementVehicleId":"VEH002","tripNumber":2}`)
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans/p1/breakdowns/reassign", body))
	if rec.Code != http.StatusOK || svc.breakdownPlanID != "p1" || svc.breakdownSourceVehicleID != "VEH001" || svc.breakdownReplacementVehicleID != "VEH002" || svc.breakdownTripNumber != 2 {
		t.Fatalf("reassignment status=%d args=%+v body=%s", rec.Code, svc, rec.Body.String())
	}
	for _, invalid := range []string{`{}`, `{"sourceVehicleId":"VEH001","replacementVehicleId":"VEH002","tripNumber":0}`, `{"sourceVehicleId":"VEH001","replacementVehicleId":"VEH002","tripNumber":3}`} {
		rec = httptest.NewRecorder()
		testRouter("usr-dispatcher", &stubPlanner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans/p1/breakdowns/reassign", bytes.NewBufferString(invalid)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid reassignment %s should be rejected, got %d", invalid, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	testRouter("usr-store-manager", &stubPlanner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/planning/plans/p1/breakdowns/reassign", bytes.NewBufferString(`{"sourceVehicleId":"VEH001","replacementVehicleId":"VEH002","tripNumber":1}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("store manager must not reassign a breakdown trip, got %d", rec.Code)
	}
}
