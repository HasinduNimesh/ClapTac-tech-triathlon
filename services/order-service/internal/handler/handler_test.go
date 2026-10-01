package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/service"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

type fakeAuth struct{ p *auth.Principal }

func (f fakeAuth) Authenticate(r *http.Request) (*auth.Principal, error) {
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return &auth.Principal{Subject: strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))}, nil
	}
	if f.p == nil {
		return nil, errAuth
	}
	return f.p, nil
}

var errAuth = errString("missing bearer token")

type errString string

func (e errString) Error() string { return string(e) }

type fakeProfiles map[string]authorization.Profile

func (f fakeProfiles) Resolve(_ context.Context, subject string) (*authorization.Profile, error) {
	p, ok := f[subject]
	if !ok {
		return &authorization.Profile{Subject: subject}, nil
	}
	cp := p
	return &cp, nil
}

type fakeOutlets struct{}

func (fakeOutlets) Resolve(string, string) (string, []string, []string, error) {
	return "", nil, nil, nil
}
func (fakeOutlets) Outlet(id, _ string) (domain.Outlet, error) {
	if id == "OUT034" {
		return domain.Outlet{ID: id, Brand: "Fresh"}, nil
	}
	if id == "OUT021" {
		return domain.Outlet{ID: id, Brand: "Style"}, nil
	}
	return domain.Outlet{}, errString("missing")
}

type noopAudit struct{}

func (noopAudit) PublishCreated(string, string, string, map[string]any) error { return nil }

type custodyPlanStub struct{}

func (custodyPlanStub) ByOrder(string) (domain.PlanningTracking, error) {
	return domain.PlanningTracking{TripID: "trip-1", Depot: "DEPOT_NORTH"}, nil
}

type custodyDeliveryStub struct{}

func (custodyDeliveryStub) ByOrder(string) (domain.DeliveryTracking, error) {
	return domain.DeliveryTracking{TripID: "trip-1", VehicleID: "VEH001", Proofs: []domain.ProofSummary{{OperationID: "photo-1", Type: "PHOTO"}}}, nil
}

func testServer(t *testing.T, subject string) (*httptest.Server, *store.Memory) {
	t.Helper()
	mem := store.NewMemory()
	profiles := fakeProfiles{
		"usr-store-manager":   {UserID: "USR001", Subject: "usr-store-manager", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT034"}},
		"usr-store-manager-b": {UserID: "USR003", Subject: "usr-store-manager-b", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT021"}},
		"usr-dispatcher":      {UserID: "USR002", Subject: "usr-dispatcher", Roles: []string{"DISPATCHER"}},
		"usr-loader":          {UserID: "USR004", Subject: "usr-loader", Roles: []string{"LOADER"}, Depot: "DEPOT_NORTH"},
		"usr-driver":          {UserID: "USR006", Subject: "usr-driver", Roles: []string{"DRIVER"}, VehicleID: "VEH001"},
	}
	var p *auth.Principal
	if subject != "" {
		p = &auth.Principal{Subject: subject}
	}
	h := Handler{
		Authn:    fakeAuth{p: p},
		Profiles: profiles,
		Service: service.Service{
			Repo:     mem,
			Outlets:  fakeOutlets{},
			Audit:    noopAudit{},
			Planning: custodyPlanStub{},
			Delivery: custodyDeliveryStub{},
		},
	}
	r := chi.NewRouter()
	h.Routes(r)
	return httptest.NewServer(r), mem
}

func TestTechCustodyRouteRecordsRoleScopedHandoffs(t *testing.T) {
	srv, mem := testServer(t, "usr-store-manager")
	defer srv.Close()
	o, err := mem.Create(domain.Order{ID: "tech-order", OrderRef: "ORD-TECH", OutletID: "OUT034", Brand: "Tech", OrderUnits: 1})
	if err != nil {
		t.Fatal(err)
	}
	post := func(subject, stage, key, condition, receiver string) (int, string) {
		body, _ := json.Marshal(domain.CustodyEvent{Stage: stage, SealID: "SEAL-1", SerialNumbers: []string{"SN-1"}, Condition: condition, EvidenceRef: map[bool]string{true: "photo-1"}[stage == "DELIVERED"], ReceiverName: receiver, IdempotencyKey: key})
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/orders/"+o.ID+"/custody", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+subject)
		req.Header.Set("Content-Type", "application/json")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		response, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(response)
	}
	for _, item := range []struct{ subject, stage, key, condition, receiver string }{{"usr-loader", "LOADED", "load-1", "packed intact", ""}, {"usr-driver", "DISPATCHED", "dispatch-1", "seal checked", ""}, {"usr-driver", "DELIVERED", "deliver-1", "received intact", ""}, {"usr-store-manager", "RECEIVED", "receive-1", "received intact", "Outlet manager"}} {
		if status, body := post(item.subject, item.stage, item.key, item.condition, item.receiver); status != http.StatusCreated {
			t.Fatalf("%s stage status=%d body=%s", item.stage, status, body)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/orders/tech-order/tracking", nil)
	req.Header.Set("Authorization", "Bearer usr-store-manager")
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var body struct {
		Tracking domain.Tracking `json:"tracking"`
	}
	if e = json.NewDecoder(res.Body).Decode(&body); e != nil {
		t.Fatal(e)
	}
	if res.StatusCode != http.StatusOK || len(body.Tracking.Custody) != 4 || body.Tracking.Custody[3].ReceiverName != "Outlet manager" {
		t.Fatalf("custody tracking status=%d %+v", res.StatusCode, body.Tracking.Custody)
	}
	if status, body := post("usr-dispatcher", "LOADED", "bad-dispatcher", "packed intact", ""); status != http.StatusForbidden {
		t.Fatalf("dispatcher custody write status=%d body=%s", status, body)
	}
	if status, body := post("usr-store-manager-b", "RECEIVED", "wrong-outlet", "received intact", "Outlet manager"); status != http.StatusNotFound {
		t.Fatalf("other outlet receipt write status=%d body=%s", status, body)
	}
}

func TestReceiptWriteRoleMatrix(t *testing.T) {
	body := []byte(`{"receivedUnits":20}`)
	for _, tc := range []struct {
		sub    string
		status int
	}{{"usr-store-manager", http.StatusConflict}, {"usr-dispatcher", http.StatusForbidden}, {"usr-loader", http.StatusForbidden}, {"usr-driver", http.StatusForbidden}, {"", http.StatusUnauthorized}} {
		srv, mem := testServer(t, tc.sub)
		o, err := mem.Create(domain.Order{ID: "receipt-order", OrderRef: "ORD-RECEIPT", OutletID: "OUT034", OrderUnits: 20})
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/orders/"+o.ID+"/receipt/confirm", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if tc.sub != "" {
			req.Header.Set("Authorization", "Bearer "+tc.sub)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("%s status=%d want=%d", tc.sub, res.StatusCode, tc.status)
		}
		srv.Close()
	}
	srv, mem := testServer(t, "usr-dispatcher")
	defer srv.Close()
	_, _ = mem.Create(domain.Order{ID: "receipt-order", OrderRef: "ORD-RECEIPT", OutletID: "OUT034", OrderUnits: 20})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/orders/receipt-order/receipt/issues", bytes.NewReader([]byte(`{"issueType":"MISSING","affectedUnits":2,"idempotencyKey":"i1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer usr-dispatcher")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("dispatcher issue write %d", res.StatusCode)
	}
	other := httptest.NewRecorder()
	h := Handler{Authn: fakeAuth{p: &auth.Principal{Subject: "usr-store-manager-b"}}, Profiles: fakeProfiles{"usr-store-manager-b": {UserID: "USR003", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT021"}}}, Service: service.Service{Repo: mem}}
	r := chi.NewRouter()
	h.Routes(r)
	r.ServeHTTP(other, httptest.NewRequest(http.MethodGet, "/api/v1/orders/receipt-order/tracking", nil))
	if other.Code != http.StatusForbidden {
		t.Fatalf("cross outlet tracking %d", other.Code)
	}
}

func createBody() []byte {
	b, _ := json.Marshal(domain.CreateRequest{
		RequestedDeliveryDate:  "2026-09-29",
		OrderUnits:             20,
		OrderWeightKg:          185.5,
		OrderVolumeM3:          2.4,
		TemperatureRequirement: "chilled",
	})
	return b
}

func TestUnauthenticatedCreateRejected(t *testing.T) {
	srv, _ := testServer(t, "")
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/v1/orders", "application/json", bytes.NewReader(createBody()))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestStoreManagerCanCreate(t *testing.T) {
	srv, _ := testServer(t, "usr-store-manager")
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/v1/orders", "application/json", bytes.NewReader(createBody()))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestDispatcherCannotCreate(t *testing.T) {
	srv, _ := testServer(t, "usr-dispatcher")
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/v1/orders", "application/json", bytes.NewReader(createBody()))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestForecastIsDispatcherOnly(t *testing.T) {
	srv, _ := testServer(t, "usr-dispatcher")
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/orders/forecast", nil)
	req.Header.Set("Authorization", "Bearer usr-dispatcher")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("dispatcher forecast status %d", res.StatusCode)
	}
	var payload struct {
		Forecast struct {
			DriftModelVersion          string `json:"driftModelVersion"`
			BacktestModelVersion       string `json:"backtestModelVersion"`
			ServiceTimeBacktestVersion string `json:"serviceTimeBacktestVersion"`
			InputDrift                 []any  `json:"inputDrift"`
			ServiceTimeEvaluation      []any  `json:"serviceTimeEvaluation"`
		} `json:"forecast"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Forecast.DriftModelVersion != "weekly_order_shift_v1" || payload.Forecast.BacktestModelVersion != "prior_four_week_order_count_ape_v1" || payload.Forecast.ServiceTimeBacktestVersion != "configured_service_time_mae_v1" || payload.Forecast.InputDrift == nil || payload.Forecast.ServiceTimeEvaluation == nil {
		t.Fatalf("forecast governance fields missing from API: %+v", payload.Forecast)
	}

	other, _ := testServer(t, "usr-driver")
	defer other.Close()
	deniedReq, _ := http.NewRequest(http.MethodGet, other.URL+"/api/v1/orders/forecast", nil)
	deniedReq.Header.Set("Authorization", "Bearer usr-driver")
	denied, err := http.DefaultClient.Do(deniedReq)
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("driver forecast status %d", denied.StatusCode)
	}
}

func TestVerticalSliceAndIsolation(t *testing.T) {
	mem := store.NewMemory()
	profiles := fakeProfiles{
		"usr-store-manager":   {UserID: "USR001", Subject: "usr-store-manager", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT034"}},
		"usr-store-manager-b": {UserID: "USR003", Subject: "usr-store-manager-b", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT021"}},
		"usr-dispatcher":      {UserID: "USR002", Subject: "usr-dispatcher", Roles: []string{"DISPATCHER"}},
	}
	svc := service.Service{Repo: mem, Outlets: fakeOutlets{}, Audit: noopAudit{}}
	hFor := func(sub string) http.Handler {
		hh := Handler{Authn: fakeAuth{p: &auth.Principal{Subject: sub}}, Profiles: profiles, Service: svc}
		r := chi.NewRouter()
		hh.Routes(r)
		return r
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(createBody()))
	hFor("usr-store-manager").ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d", rec.Code)
	}
	var created struct {
		Order domain.Order `json:"order"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Order.OrderRef == "" || created.Order.OutletID != "OUT034" {
		t.Fatalf("created %+v", created.Order)
	}

	rec = httptest.NewRecorder()
	hFor("usr-store-manager").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(created.Order.OrderRef)) {
		t.Fatalf("sm list %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	hFor("usr-dispatcher").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(created.Order.OrderRef)) {
		t.Fatalf("dispatcher list %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	hFor("usr-store-manager-b").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+created.Order.ID, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sm-b get %d", rec.Code)
	}
}
