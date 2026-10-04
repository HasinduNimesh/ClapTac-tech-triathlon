package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/service"
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

type stubDriver struct {
	list        []map[string]any
	lateness    []domain.LatenessProbability
	detail      map[string]any
	listErr     error
	getErr      error
	mutErr      error
	proof       domain.Proof
	results     []map[string]any
	startOpID   string
	queueHealth domain.OfflineQueueHealth
}

func (s *stubDriver) List(context.Context, *authorization.Profile, string, string) ([]map[string]any, error) {
	return s.list, s.listErr
}
func (s *stubDriver) LatenessHistory(context.Context, *authorization.Profile, string) ([]domain.LatenessProbability, error) {
	return s.lateness, s.getErr
}
func (s *stubDriver) Get(context.Context, *authorization.Profile, string) (map[string]any, error) {
	return s.detail, s.getErr
}
func (s *stubDriver) Prepare(context.Context, *authorization.Profile, string) (map[string]any, error) {
	return s.detail, s.mutErr
}
func (s *stubDriver) Start(_ context.Context, _ *authorization.Profile, _ string, opID string) (map[string]any, error) {
	s.startOpID = opID
	return s.detail, s.mutErr
}
func (s *stubDriver) UpdateLocation(context.Context, *authorization.Profile, string, float64, float64, time.Time) (domain.Location, error) { return domain.Location{}, s.mutErr }
func (s *stubDriver) TripLocation(context.Context, *authorization.Profile, string) (*domain.Location, error) { return nil, s.getErr }
func (s *stubDriver) Arrive(context.Context, *authorization.Profile, string, string, string, string) (map[string]any, error) {
	return s.detail, s.mutErr
}
func (s *stubDriver) Outcome(context.Context, *authorization.Profile, string, string, string, string, string, string, string, string, *int) (map[string]any, error) {
	return s.detail, s.mutErr
}
func (s *stubDriver) UploadProof(context.Context, *authorization.Profile, string, string, string, string, string, []byte, string, string) (domain.Proof, error) {
	return s.proof, s.mutErr
}
func (s *stubDriver) Complete(context.Context, *authorization.Profile, string, string, string) (map[string]any, error) {
	return s.detail, s.mutErr
}
func (s *stubDriver) Sync(context.Context, *authorization.Profile, domain.SyncRequest) []map[string]any {
	return s.results
}
func (s *stubDriver) InternalOrder(context.Context, string) (domain.OrderTracking, error) {
	return domain.OrderTracking{}, nil
}
func (s *stubDriver) OutletLastServed(context.Context) ([]domain.OutletLastServed, error) {
	return []domain.OutletLastServed{}, nil
}
func (s *stubDriver) OutletLastAttempted(context.Context, string) ([]domain.OutletLastAttempted, error) {
	return []domain.OutletLastAttempted{}, nil
}
func (s *stubDriver) SendTripMessage(_ context.Context, _ *authorization.Profile, tripID, stopID, body string) (domain.TripMessage, error) {
	return domain.TripMessage{TripID: tripID, StopID: stopID, Body: body}, s.mutErr
}
func (s *stubDriver) TripMessages(context.Context, *authorization.Profile, string) ([]domain.TripMessage, error) {
	return []domain.TripMessage{{ID: "m1", Body: "Gate is on the south side"}}, s.getErr
}
func (s *stubDriver) AcknowledgeTripMessage(context.Context, *authorization.Profile, string, string) (domain.TripMessage, error) {
	return domain.TripMessage{ID: "m1", AcknowledgedBy: "USR006"}, s.mutErr
}
func (s *stubDriver) ReportOfflineQueueHealth(_ *authorization.Profile, report domain.OfflineQueueHealth) error {
	s.queueHealth = report
	return s.mutErr
}

func testRouter(subject string, svc Driver) http.Handler {
	profiles := fakeProfiles{
		"usr-driver":        {UserID: "USR006", Subject: "usr-driver", Roles: []string{"DRIVER"}, VehicleID: "VEH001"},
		"usr-dispatcher":    {UserID: "USR002", Subject: "usr-dispatcher", Roles: []string{"DISPATCHER"}},
		"usr-loader":        {UserID: "USR004", Subject: "usr-loader", Roles: []string{"LOADER"}, Depot: "DEPOT_NORTH"},
		"usr-store-manager": {UserID: "USR001", Subject: "usr-store-manager", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT034"}},
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

func TestTripMessageRoleRoutes(t *testing.T) {
	dispatcher := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/trips/TRIP1/messages", strings.NewReader(`{"body":"Gate is on the south side","stopId":""}`))
	testRouter("usr-dispatcher", &stubDriver{}).ServeHTTP(dispatcher, req)
	if dispatcher.Code != http.StatusCreated {
		t.Fatalf("dispatcher message send status=%d %s", dispatcher.Code, dispatcher.Body.String())
	}
	driver := httptest.NewRecorder()
	testRouter("usr-driver", &stubDriver{}).ServeHTTP(driver, httptest.NewRequest(http.MethodPost, "/api/v1/delivery/trips/TRIP1/messages/m1/ack", nil))
	if driver.Code != http.StatusOK {
		t.Fatalf("driver acknowledgement status=%d %s", driver.Code, driver.Body.String())
	}
	denied := httptest.NewRecorder()
	testRouter("usr-store-manager", &stubDriver{}).ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/v1/delivery/trips/TRIP1/messages", strings.NewReader(`{"body":"test"}`)))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("store message send status=%d", denied.Code)
	}
}

func TestOfflineQueueHealthRequiresDriverAndRejectsUnknownFields(t *testing.T) {
	stub := &stubDriver{}
	accepted := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/telemetry/offline-queue", strings.NewReader(`{"ageBucket":"30d_plus","countBucket":"6_plus"}`))
	testRouter("usr-driver", stub).ServeHTTP(accepted, request)
	if accepted.Code != http.StatusNoContent || stub.queueHealth.AgeBucket != "30d_plus" || stub.queueHealth.CountBucket != "6_plus" {
		t.Fatalf("driver health report status=%d body=%s report=%+v", accepted.Code, accepted.Body.String(), stub.queueHealth)
	}

	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/delivery/telemetry/offline-queue", strings.NewReader(`{"ageBucket":"none","countBucket":"none"}`))
	testRouter("usr-dispatcher", &stubDriver{}).ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("dispatcher report status=%d want 403", denied.Code)
	}

	bad := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/delivery/telemetry/offline-queue", strings.NewReader(`{"ageBucket":"30d_plus","countBucket":"6_plus","tripId":"TRIP-PRIVATE"}`))
	testRouter("usr-driver", &stubDriver{}).ServeHTTP(bad, request)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown private field status=%d want 400", bad.Code)
	}

	for name, body := range map[string]string{
		"multiple objects": `{"ageBucket":"none","countBucket":"none"}{"ageBucket":"none","countBucket":"none"}`,
		"oversized body":   strings.Repeat(" ", 1025) + `{"ageBucket":"none","countBucket":"none"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/telemetry/offline-queue", strings.NewReader(body))
			testRouter("usr-driver", &stubDriver{}).ServeHTTP(rec, request)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("invalid report status=%d want 400", rec.Code)
			}
		})
	}
}

func TestOutletLastServedRequiresInternalScope(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/delivery/internal/outlets/last-served", nil)
	rec := httptest.NewRecorder()
	testRouter("usr-driver", &stubDriver{}).ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("driver internal access returned %d", rec.Code)
	}

	profiles := fakeProfiles{}
	h := Handler{Authn: fakeAuth{p: &auth.Principal{Subject: "svc-planner", Scopes: []string{authorization.PermDeliveriesReadInternal}}}, Profiles: profiles, Service: &stubDriver{}}
	r := chi.NewRouter()
	h.Routes(r)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/internal/outlets/last-served", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("planning internal access returned %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnauthenticatedRejected(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("", &stubDriver{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/trips?date=2026-09-29", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestLoaderAndStoreManagerForbidden(t *testing.T) {
	for _, sub := range []string{"usr-loader", "usr-store-manager"} {
		rec := httptest.NewRecorder()
		testRouter(sub, &stubDriver{list: []map[string]any{}}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/trips?date=2026-09-29", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s GET %d", sub, rec.Code)
		}
	}
}

func TestDriverAndDispatcherCanView(t *testing.T) {
	svc := &stubDriver{list: []map[string]any{{"tripId": "t1"}}}
	for _, sub := range []string{"usr-driver", "usr-dispatcher"} {
		rec := httptest.NewRecorder()
		testRouter(sub, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/trips?date=2026-09-29", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s list %d %s", sub, rec.Code, rec.Body.String())
		}
	}
}

func TestLatenessHistoryIsDispatcherOnly(t *testing.T) {
	item := domain.EstimateLatenessProbability("DEPOT_NORTH", "Fresh", 12, 3)
	svc := &stubDriver{lateness: []domain.LatenessProbability{item}}
	rec := httptest.NewRecorder()
	testRouter("usr-dispatcher", svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/trips/t1/lateness-history", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"modelVersion":"`+domain.LatenessModelVersion+`"`) {
		t.Fatalf("dispatcher history response %d %s", rec.Code, rec.Body.String())
	}
	for _, subject := range []string{"usr-driver", "usr-loader", "usr-store-manager"} {
		rec = httptest.NewRecorder()
		testRouter(subject, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/trips/t1/lateness-history", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s lateness history returned %d", subject, rec.Code)
		}
	}
}

func TestInternalOrderRequiresNarrowScope(t *testing.T) {
	h := Handler{Authn: fakeAuth{p: &auth.Principal{Subject: "svc", Scopes: []string{authorization.PermDeliveriesReadInternal}}}, Profiles: fakeProfiles{}, Service: &stubDriver{}}
	r := chi.NewRouter()
	h.Routes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/internal/orders/order-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("service scope status %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	testRouter("usr-dispatcher", &stubDriver{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/internal/orders/order-1", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("dispatcher without internal scope status %d", rec.Code)
	}
}

func TestDispatcherCannotStart(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("usr-dispatcher", &stubDriver{detail: map[string]any{"tripId": "t1"}}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/delivery/trips/t1/start", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("dispatcher start %d", rec.Code)
	}
}

func TestDriverCanStart(t *testing.T) {
	svc := &stubDriver{detail: map[string]any{"status": "in_progress"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/trips/t1/start", nil)
	req.Header.Set("Idempotency-Key", "start-1")
	testRouter("usr-driver", svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start %d %s", rec.Code, rec.Body.String())
	}
	if svc.startOpID != "start-1" {
		t.Fatalf("idempotency key not passed to service: %q", svc.startOpID)
	}
}

func TestCompleteIncomplete(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("usr-driver", &stubDriver{mutErr: service.IncompleteError{Pending: []string{"s1"}}}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/delivery/trips/t1/complete", bytes.NewReader([]byte(`{}`))))
	if rec.Code != http.StatusConflict {
		t.Fatalf("complete %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["type"] != "delivery_incomplete" {
		t.Fatalf("type %v", body["type"])
	}
}

func TestOutcomeProofRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/trips/t1/stops/s1/outcome", bytes.NewReader([]byte(`{"code":"DELIVERED"}`)))
	testRouter("usr-driver", &stubDriver{mutErr: fmt.Errorf("rejected: proof required before outcome")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("outcome %d", rec.Code)
	}
}

func TestProofRejectsOversizedMultipartRequest(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "proof.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(bytes.Repeat([]byte("x"), domain.MaxPhotoBytes+(256<<10))); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/delivery/trips/t1/stops/s1/proofs", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	testRouter("usr-driver", &stubDriver{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized proof returned %d: %s", rec.Code, rec.Body.String())
	}
}
