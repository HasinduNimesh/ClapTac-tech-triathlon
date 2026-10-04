package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/service"
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

type stubLoader struct {
	list         []map[string]any
	detail       map[string]any
	listErr      error
	getErr       error
	startErr     error
	startVersion int
	loadEntry    domain.LoadEntry
	loadCalls    int
	loadErr      error
	issueErr     error
	readyErr     error
	issue        domain.Issue
}

func (s *stubLoader) List(context.Context, *authorization.Profile, string) ([]map[string]any, error) {
	return s.list, s.listErr
}
func (s *stubLoader) Get(context.Context, *authorization.Profile, string) (map[string]any, error) {
	return s.detail, s.getErr
}
func (s *stubLoader) Start(_ context.Context, _ *authorization.Profile, _ string, expectedPlanVersion int) (map[string]any, error) {
	s.startVersion = expectedPlanVersion
	return s.detail, s.startErr
}
func (s *stubLoader) MarkLoaded(_ context.Context, _ *authorization.Profile, _, _ string, entry domain.LoadEntry) error {
	s.loadEntry = entry
	s.loadCalls++
	return s.loadErr
}
func (s *stubLoader) CreateIssue(context.Context, *authorization.Profile, string, string, string, string, string, int) (domain.Issue, error) {
	return s.issue, s.issueErr
}
func (s *stubLoader) UpdateIssue(context.Context, *authorization.Profile, string, string, string, string, string, int) error {
	return s.issueErr
}
func (s *stubLoader) DeleteIssue(context.Context, *authorization.Profile, string, string, string) error {
	return s.issueErr
}
func (s *stubLoader) DecideIssue(context.Context, *authorization.Profile, string, string, string, string, string) (domain.Issue, error) {
	return s.issue, s.issueErr
}
func (s *stubLoader) SyncPlanVersion(context.Context, *authorization.Profile, string, int) (map[string]any, error) {
	return s.detail, nil
}
func (s *stubLoader) AttachIssuePhoto(context.Context, *authorization.Profile, string, string, string, string, []byte) (domain.Issue, error) {
	return domain.Issue{}, nil
}
func (s *stubLoader) IssuePhoto(context.Context, *authorization.Profile, string, string, string) (io.ReadCloser, string, error) {
	return nil, "", fmt.Errorf("not found")
}
func (s *stubLoader) MarkIssueSeen(context.Context, *authorization.Profile, string, string, string) error {
	return nil
}
func (s *stubLoader) RaiseDockAlert(context.Context, *authorization.Profile, string, string, string, string, string, string) (domain.DockAlert, error) {
	return domain.DockAlert{}, nil
}
func (s *stubLoader) ListDockAlerts(context.Context, *authorization.Profile, string) ([]domain.DockAlert, error) {
	return []domain.DockAlert{}, nil
}
func (s *stubLoader) ResolveDockAlert(context.Context, *authorization.Profile, string) (domain.DockAlert, error) {
	return domain.DockAlert{}, nil
}
func (s *stubLoader) Ready(context.Context, *authorization.Profile, string, service.ReadyChecks) (map[string]any, error) {
	return s.detail, s.readyErr
}
func (s *stubLoader) InternalList(context.Context, string, string) ([]map[string]any, error) {
	return s.list, s.listErr
}
func (s *stubLoader) InternalGet(context.Context, string) (map[string]any, error) {
	return s.detail, s.getErr
}

func testRouter(subject string, svc Loader) http.Handler {
	profiles := fakeProfiles{
		"usr-loader":        {UserID: "USR004", Subject: "usr-loader", Roles: []string{"LOADER"}, Depot: "DEPOT_NORTH"},
		"usr-loader-kandy":  {UserID: "USR005", Subject: "usr-loader-kandy", Roles: []string{"LOADER"}, Depot: "DEPOT_SOUTH"},
		"usr-dispatcher":    {UserID: "USR002", Subject: "usr-dispatcher", Roles: []string{"DISPATCHER"}},
		"usr-driver":        {UserID: "USR006", Subject: "usr-driver", Roles: []string{"DRIVER"}},
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

func TestUnauthenticatedRejected(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("", &stubLoader{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/loading/trips?date=2026-09-29", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestDriverForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("usr-driver", &stubLoader{list: []map[string]any{}}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/loading/trips?date=2026-09-29", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("driver GET %d", rec.Code)
	}
}

func TestStoreManagerForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("usr-store-manager", &stubLoader{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/loading/trips?date=2026-09-29", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("store manager GET %d", rec.Code)
	}
}

func TestLoaderAndDispatcherCanView(t *testing.T) {
	svc := &stubLoader{list: []map[string]any{{"tripId": "t1"}}, detail: map[string]any{"tripId": "t1"}}
	for _, sub := range []string{"usr-loader", "usr-dispatcher"} {
		rec := httptest.NewRecorder()
		testRouter(sub, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/loading/trips?date=2026-09-29", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s list %d %s", sub, rec.Code, rec.Body.String())
		}
	}
}

func TestDispatcherCannotMutate(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("usr-dispatcher", &stubLoader{detail: map[string]any{"tripId": "t1"}}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/loading/trips/t1/start", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("dispatcher start %d", rec.Code)
	}
}

func TestLoaderCanStart(t *testing.T) {
	service := &stubLoader{detail: map[string]any{"tripId": "t1", "status": "in_progress"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/loading/trips/t1/start", nil)
	req.Header.Set("If-Match", `"7"`)
	testRouter("usr-loader", service).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start %d %s", rec.Code, rec.Body.String())
	}
	if service.startVersion != 7 {
		t.Fatalf("expected plan version %d, got %d", 7, service.startVersion)
	}
}

func TestLoaderStartRejectsMalformedPlanVersion(t *testing.T) {
	service := &stubLoader{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/loading/trips/t1/start", nil)
	req.Header.Set("If-Match", "latest")
	testRouter("usr-loader", service).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed If-Match status %d", rec.Code)
	}
	if service.startVersion != 0 {
		t.Fatalf("malformed version reached service: %d", service.startVersion)
	}
}

func TestLoadedActiveIssueConflict(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("usr-loader", &stubLoader{loadErr: fmt.Errorf("conflict: ACTIVE_LOADING_ISSUE")}).ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/loading/trips/t1/orders/o1/loaded", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("loaded %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("ACTIVE_LOADING_ISSUE")) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestReadyIncomplete(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("usr-loader", &stubLoader{readyErr: service.IncompleteError{Pending: []string{"o1"}}}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/loading/trips/t1/ready", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("ready %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["type"] != "loading_incomplete" {
		t.Fatalf("type %v", body["type"])
	}
}

func TestIssueRequiresIdempotencyKey(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/loading/trips/t1/orders/o1/issues", bytes.NewReader([]byte(`{"type":"MISSING","affectedUnits":1}`)))
	testRouter("usr-loader", &stubLoader{issueErr: fmt.Errorf("invalid: Idempotency-Key required")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("issue %d", rec.Code)
	}
}

func TestInternalReadyForbiddenToLoaderAndDriver(t *testing.T) {
	svc := &stubLoader{list: []map[string]any{{"tripId": "t1"}}, detail: map[string]any{"tripId": "t1"}}
	for _, sub := range []string{"usr-loader", "usr-driver"} {
		rec := httptest.NewRecorder()
		testRouter(sub, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/loading/internal/trips?date=2026-09-29", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s internal list %d", sub, rec.Code)
		}
	}
}

func TestInternalReadyAllowedToDispatcher(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter("usr-dispatcher", &stubLoader{list: []map[string]any{{"tripId": "t1"}}}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/loading/internal/trips?date=2026-09-29", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("dispatcher internal %d %s", rec.Code, rec.Body.String())
	}
}

func putLoaded(svc *stubLoader, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var rd io.Reader
	if body != "" {
		rd = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/loading/trips/t1/orders/o1/loaded", rd)
	testRouter("usr-loader", svc).ServeHTTP(rec, req)
	return rec
}

func TestLoadedManualWithoutReasonIsRejected(t *testing.T) {
	for _, body := range []string{`{"entryMethod":"MANUAL"}`, `{"entryMethod":"MANUAL","reasonCode":""}`, `{"entryMethod":"MANUAL","reasonCode":"BORED"}`} {
		svc := &stubLoader{}
		rec := putLoaded(svc, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400 (%s)", body, rec.Code, rec.Body.String())
		}
		if svc.loadCalls != 0 {
			t.Fatalf("%s: a typed ID without a reason reached the service", body)
		}
	}
}

func TestLoadedManualWithReasonReachesTheService(t *testing.T) {
	svc := &stubLoader{}
	rec := putLoaded(svc, `{"entryMethod":"MANUAL","reasonCode":"DAMAGED_LABEL","note":"torn corner"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	want := domain.LoadEntry{Method: "MANUAL", ReasonCode: "DAMAGED_LABEL", Note: "torn corner"}
	if svc.loadEntry != want {
		t.Fatalf("entry = %+v, want %+v", svc.loadEntry, want)
	}
}

func TestLoadedWithoutBodyIsALegacyEntry(t *testing.T) {
	svc := &stubLoader{}
	if rec := putLoaded(svc, ""); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if svc.loadCalls != 1 || svc.loadEntry != (domain.LoadEntry{}) {
		t.Fatalf("legacy call = %d calls, entry %+v", svc.loadCalls, svc.loadEntry)
	}
}

func TestLoadedScanAndMalformedBody(t *testing.T) {
	svc := &stubLoader{}
	if rec := putLoaded(svc, `{"entryMethod":"SCAN"}`); rec.Code != http.StatusOK || svc.loadEntry.Method != "SCAN" {
		t.Fatalf("scan: status %d entry %+v", rec.Code, svc.loadEntry)
	}
	if rec := putLoaded(&stubLoader{}, `{not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: status %d", rec.Code)
	}
}
