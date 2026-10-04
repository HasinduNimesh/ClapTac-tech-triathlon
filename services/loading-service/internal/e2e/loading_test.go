package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	loadclient "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/client"
	loadhandler "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/handler"
	loadservice "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/service"
	loadstore "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/store"
)

type bearerAuth struct{}

func (bearerAuth) Authenticate(r *http.Request) (*auth.Principal, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, errors.New("missing bearer token")
	}
	sub := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	if sub == "" {
		return nil, errors.New("missing bearer token")
	}
	p := &auth.Principal{Subject: sub}
	if strings.HasPrefix(sub, "svc-") {
		p.Scopes = []string{
			authorization.PermPlansReadInternal,
			authorization.PermOrdersReadInternal,
			authorization.PermAuditWrite,
			authorization.PermLoadingReadInternal,
		}
	}
	return p, nil
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

type staticProfiles struct{}

func (staticProfiles) Resolve(_ context.Context, subject string) (*authorization.Profile, error) {
	switch subject {
	case "usr-loader":
		return &authorization.Profile{UserID: "USR004", Subject: subject, Roles: []string{"LOADER"}, Depot: "DEPOT_NORTH"}, nil
	case "usr-loader-kandy":
		return &authorization.Profile{UserID: "USR005", Subject: subject, Roles: []string{"LOADER"}, Depot: "DEPOT_SOUTH"}, nil
	case "usr-dispatcher":
		return &authorization.Profile{UserID: "USR002", Subject: subject, Roles: []string{"DISPATCHER"}}, nil
	case "usr-driver":
		return &authorization.Profile{UserID: "USR006", Subject: subject, Roles: []string{"DRIVER"}}, nil
	default:
		return &authorization.Profile{Subject: subject}, nil
	}
}

func TestLoadingWorkflow(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Skipf("testcontainers postgres:16-alpine unavailable: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	host, err := pg.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := pg.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}
	dsn := "postgres://waypoint:waypoint@" + host + ":" + port.Port() + "/waypoint?sslmode=disable"
	root := repoRoot(t)
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0001_init.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0009_loading.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0011_loading_delivery_snapshot.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0021_loading_plan_version.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0042_loading_issue_decisions.sql"))

	peers := httptest.NewServer(peerStub())
	t.Cleanup(peers.Close)

	pool, err := db.Open(ctx, dsn, "loading")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	h := loadhandler.Handler{
		Authn:    bearerAuth{},
		Profiles: staticProfiles{},
		Service: loadservice.Service{
			Repo:  loadstore.Postgres{Pool: pool},
			Peers: loadclient.Peers{PlanningURL: peers.URL, OrdersURL: peers.URL, SharedURL: peers.URL, M2M: staticToken("svc-loading")},
		},
	}
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	if code := do(t, srv, http.MethodGet, "/api/v1/loading/trips?date=2026-09-29", "usr-driver", nil, "").status; code != http.StatusForbidden {
		t.Fatalf("driver list %d", code)
	}

	listed := do(t, srv, http.MethodGet, "/api/v1/loading/trips?date=2026-09-29", "usr-loader", nil, "")
	if listed.status != http.StatusOK {
		t.Fatalf("loader list %d %s", listed.status, listed.body)
	}
	if !strings.Contains(listed.body, `"tripId":"trip-north"`) {
		t.Fatalf("north trip missing: %s", listed.body)
	}
	if strings.Contains(listed.body, "trip-south") {
		t.Fatalf("south trip leaked to north loader: %s", listed.body)
	}

	disp := do(t, srv, http.MethodGet, "/api/v1/loading/trips?date=2026-09-29", "usr-dispatcher", nil, "")
	if disp.status != http.StatusOK || !strings.Contains(disp.body, "trip-south") {
		t.Fatalf("dispatcher view-all %d %s", disp.status, disp.body)
	}

	if code := do(t, srv, http.MethodGet, "/api/v1/loading/trips/trip-north", "usr-loader-kandy", nil, "").status; code != http.StatusForbidden {
		t.Fatalf("kandy loader north trip %d", code)
	}

	staleStart := doWithPlanVersion(t, srv, "/api/v1/loading/trips/trip-north/start", "usr-loader", 99)
	if staleStart.status != http.StatusConflict || !strings.Contains(staleStart.body, "plan version changed") {
		t.Fatalf("stale plan start %d %s", staleStart.status, staleStart.body)
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM loading.sessions WHERE trip_id='trip-north'`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("stale start created a loading session: count=%d err=%v", sessions, err)
	}
	started := doWithPlanVersion(t, srv, "/api/v1/loading/trips/trip-north/start", "usr-loader", 1)
	if started.status != http.StatusOK {
		t.Fatalf("start %d %s", started.status, started.body)
	}
	if !strings.Contains(started.body, `"suggestedLoadSequence":2`) || !strings.Contains(started.body, `"suggestedLoadSequence":1`) {
		t.Fatalf("suggested load order missing: %s", started.body)
	}
	again := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-north/start", "usr-loader", nil, "")
	if again.status != http.StatusOK {
		t.Fatalf("idempotent start %d %s", again.status, again.body)
	}

	if code := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-north/ready", "usr-loader", nil, "").status; code != http.StatusConflict {
		t.Fatalf("ready while pending %d", code)
	}

	loaded1 := do(t, srv, http.MethodPut, "/api/v1/loading/trips/trip-north/orders/ord-2/loaded", "usr-loader", nil, "")
	if loaded1.status != http.StatusOK {
		t.Fatalf("load ord-2 %d %s", loaded1.status, loaded1.body)
	}

	iss := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-north/orders/ord-1/issues", "usr-loader", []byte(`{"type":"MISSING","affectedUnits":2,"note":"crate short"}`), "iss-1")
	if iss.status != http.StatusCreated {
		t.Fatalf("issue %d %s", iss.status, iss.body)
	}
	replay := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-north/orders/ord-1/issues", "usr-loader", []byte(`{"type":"DAMAGED","affectedUnits":1}`), "iss-1")
	if replay.status != http.StatusCreated || !strings.Contains(replay.body, "MISSING") {
		t.Fatalf("idempotent issue %d %s", replay.status, replay.body)
	}

	blocked := do(t, srv, http.MethodPut, "/api/v1/loading/trips/trip-north/orders/ord-1/loaded", "usr-loader", nil, "")
	if blocked.status != http.StatusConflict || !strings.Contains(blocked.body, "ACTIVE_LOADING_ISSUE") {
		t.Fatalf("loaded with issue %d %s", blocked.status, blocked.body)
	}

	undecided := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-north/ready", "usr-loader", nil, "")
	if undecided.status != http.StatusConflict || !strings.Contains(undecided.body, "dispatcher_decision_required") {
		t.Fatalf("ready with undecided shortfall %d %s", undecided.status, undecided.body)
	}
	var created1 struct {
		Issue struct{ ID string } `json:"issue"`
	}
	if err := json.Unmarshal([]byte(iss.body), &created1); err != nil || created1.Issue.ID == "" {
		t.Fatalf("issue id %v %s", err, iss.body)
	}
	decisionPath := "/api/v1/loading/trips/trip-north/orders/ord-1/issues/" + created1.Issue.ID + "/decision"
	if code := do(t, srv, http.MethodPost, decisionPath, "usr-loader", []byte(`{"decision":"PARTIAL_LOAD"}`), "").status; code != http.StatusForbidden {
		t.Fatalf("loader decided own shortfall %d", code)
	}
	if code := do(t, srv, http.MethodPost, decisionPath, "usr-dispatcher", []byte(`{"decision":"SHRUG"}`), "").status; code != http.StatusBadRequest {
		t.Fatalf("invalid decision %d", code)
	}
	hold := do(t, srv, http.MethodPost, decisionPath, "usr-dispatcher", []byte(`{"decision":"HOLD","note":"replacement stock on the way"}`), "")
	if hold.status != http.StatusOK || !strings.Contains(hold.body, `"decision":"HOLD"`) {
		t.Fatalf("hold decision %d %s", hold.status, hold.body)
	}
	if code := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-north/ready", "usr-loader", nil, "").status; code != http.StatusConflict {
		t.Fatalf("ready while on hold %d", code)
	}
	partial := do(t, srv, http.MethodPost, decisionPath, "usr-dispatcher", []byte(`{"decision":"PARTIAL_LOAD","note":"leave on time"}`), "")
	if partial.status != http.StatusOK || !strings.Contains(partial.body, `"decidedBy":"USR002"`) {
		t.Fatalf("partial decision %d %s", partial.status, partial.body)
	}
	if detail := do(t, srv, http.MethodGet, "/api/v1/loading/trips/trip-north", "usr-loader", nil, ""); !strings.Contains(detail.body, `"decision":"PARTIAL_LOAD"`) {
		t.Fatalf("decision not visible to loader %s", detail.body)
	}

	readyWithShort := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-north/ready", "usr-loader", nil, "")
	if readyWithShort.status != http.StatusOK {
		t.Fatalf("ready with shortfall %d %s", readyWithShort.status, readyWithShort.body)
	}
	if !strings.Contains(readyWithShort.body, `"status":"ready"`) {
		t.Fatalf("ready body %s", readyWithShort.body)
	}
	internal := do(t, srv, http.MethodGet, "/api/v1/loading/internal/trips/trip-north", "svc-loading", nil, "")
	if internal.status != http.StatusOK || !strings.Contains(internal.body, `"loadingStatus":"ready"`) || !strings.Contains(internal.body, `"orderRef"`) {
		t.Fatalf("internal ready %d %s", internal.status, internal.body)
	}
	if code := do(t, srv, http.MethodGet, "/api/v1/loading/internal/trips/trip-north", "usr-loader", nil, "").status; code != http.StatusForbidden {
		t.Fatalf("loader internal get %d", code)
	}

	after := do(t, srv, http.MethodPut, "/api/v1/loading/trips/trip-north/orders/ord-2/loaded", "usr-loader", nil, "")
	if after.status != http.StatusConflict {
		t.Fatalf("mutate after ready %d %s", after.status, after.body)
	}

	secondReady := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-north/ready", "usr-loader", nil, "")
	if secondReady.status != http.StatusOK {
		t.Fatalf("idempotent ready %d %s", secondReady.status, secondReady.body)
	}

	clearStart := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-clear/start", "usr-loader", nil, "")
	if clearStart.status != http.StatusOK {
		t.Fatalf("clear start %d %s", clearStart.status, clearStart.body)
	}
	iss2 := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-clear/orders/ord-4/issues", "usr-loader", []byte(`{"type":"DAMAGED","affectedUnits":1}`), "iss-clear")
	if iss2.status != http.StatusCreated {
		t.Fatalf("clear issue %d %s", iss2.status, iss2.body)
	}
	var created struct {
		Issue struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := json.Unmarshal([]byte(iss2.body), &created); err != nil {
		t.Fatal(err)
	}
	del := do(t, srv, http.MethodDelete, "/api/v1/loading/trips/trip-clear/orders/ord-4/issues/"+created.Issue.ID, "usr-loader", nil, "")
	if del.status != http.StatusOK {
		t.Fatalf("delete issue %d %s", del.status, del.body)
	}
	got := do(t, srv, http.MethodGet, "/api/v1/loading/trips/trip-clear", "usr-loader", nil, "")
	if !strings.Contains(got.body, `"status":"shortfall"`) {
		t.Fatalf("should stay shortfall after last issue deleted: %s", got.body)
	}
	if code := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-clear/ready", "usr-loader", nil, "").status; code != http.StatusConflict {
		t.Fatalf("ready after cleared issue should stay incomplete %d", code)
	}
	if code := do(t, srv, http.MethodPut, "/api/v1/loading/trips/trip-clear/orders/ord-4/loaded", "usr-loader", nil, "").status; code != http.StatusOK {
		t.Fatalf("explicit loaded after clear %d", code)
	}
	if code := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-clear/ready", "usr-loader", nil, "").status; code != http.StatusOK {
		t.Fatalf("ready after explicit loaded %d", code)
	}

	// "Move to the next run" is recorded before planning has published a plan
	// without the line. Whatever happens to the planning calls that follow, the
	// trip must not be released with the short line still on it.
	if code := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-move/start", "usr-loader", nil, "").status; code != http.StatusOK {
		t.Fatalf("move trip start %d", code)
	}
	if code := do(t, srv, http.MethodPut, "/api/v1/loading/trips/trip-move/orders/ord-9/loaded", "usr-loader", nil, "").status; code != http.StatusOK {
		t.Fatalf("move trip load ord-9 %d", code)
	}
	moveIssue := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-move/orders/ord-8/issues", "usr-loader", []byte(`{"type":"MISSING","affectedUnits":3}`), "iss-move")
	if moveIssue.status != http.StatusCreated {
		t.Fatalf("move trip issue %d %s", moveIssue.status, moveIssue.body)
	}
	var moveCreated struct {
		Issue struct{ ID string } `json:"issue"`
	}
	if err := json.Unmarshal([]byte(moveIssue.body), &moveCreated); err != nil || moveCreated.Issue.ID == "" {
		t.Fatalf("move issue id %v %s", err, moveIssue.body)
	}
	moveDecision := "/api/v1/loading/trips/trip-move/orders/ord-8/issues/" + moveCreated.Issue.ID + "/decision"
	if code := do(t, srv, http.MethodPost, moveDecision, "usr-dispatcher", []byte(`{"decision":"MOVE_TO_NEXT_RUN"}`), "").status; code != http.StatusOK {
		t.Fatalf("move decision %d", code)
	}
	readyMove := func(label string) resp {
		t.Helper()
		r := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-move/ready", "usr-loader", nil, "")
		if r.status != http.StatusConflict {
			t.Fatalf("%s: ready must be refused, got %d %s", label, r.status, r.body)
		}
		return r
	}
	// 1. The planning calls after the decision failed: the old plan is still
	// confirmed and still carries ord-8.
	if r := readyMove("move decided, plan unchanged"); !strings.Contains(r.body, "dispatcher_decision_required") || !strings.Contains(r.body, `"moveUnpublishedOrderIds":["ord-8"]`) {
		t.Fatalf("expected the unpublished move to be named: %s", r.body)
	}
	// 2. The plan is reopened for revision (not confirmed) while the loader presses Ready.
	moveState.Store(1)
	readyMove("plan being revised")
	// 3. A new plan without ord-8 is confirmed, but this session has not moved onto it.
	moveState.Store(2)
	readyMove("new plan confirmed, session not synced")
	// 4. Planning is rolled back to the original confirmed plan: still refused.
	moveState.Store(0)
	readyMove("planning rolled back")
	var moveStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM loading.sessions WHERE trip_id='trip-move'`).Scan(&moveStatus); err != nil || moveStatus != "in_progress" {
		t.Fatalf("the session must still be in progress, got %q %v", moveStatus, err)
	}
	// Holding or leaving the shortfall undecided is refused for the same reason.
	if code := do(t, srv, http.MethodPost, moveDecision, "usr-dispatcher", []byte(`{"decision":"HOLD"}`), "").status; code != http.StatusOK {
		t.Fatalf("hold decision %d", code)
	}
	readyMove("on hold")
	// A partial load is the decision that releases the trip.
	if code := do(t, srv, http.MethodPost, moveDecision, "usr-dispatcher", []byte(`{"decision":"PARTIAL_LOAD"}`), "").status; code != http.StatusOK {
		t.Fatalf("partial decision %d", code)
	}
	if code := do(t, srv, http.MethodPost, "/api/v1/loading/trips/trip-move/ready", "usr-loader", nil, "").status; code != http.StatusOK {
		t.Fatalf("ready after partial load %d", code)
	}
}

// moveState drives the planning stub for trip-move: 0 confirmed v1 with both
// orders, 1 reopened for revision (no confirmed plan), 2 confirmed v2 without ord-8.
var moveState atomic.Int32

func moveTrip() (map[string]any, bool) {
	state := moveState.Load()
	if state == 1 {
		return nil, false
	}
	version := 1
	allocs := []map[string]any{
		{"allocationId": "a8", "orderId": "ord-8", "orderRef": "ORD000008", "outletId": "OUT034", "stopSequence": 1},
		{"allocationId": "a9", "orderId": "ord-9", "orderRef": "ORD000009", "outletId": "OUT021", "stopSequence": 2},
	}
	if state == 2 {
		version = 2
		allocs = allocs[1:]
	}
	return map[string]any{
		"planId": "plan-1", "planRef": "PLAN000001", "deliveryDate": "2026-09-29", "planStatus": "confirmed",
		"planVersion": version, "planAcknowledgements": []map[string]any{{"actorId": "USR004", "actorRole": "LOADER"}},
		"tripId": "trip-move", "tripNumber": 4, "vehicleId": "VEH004", "vehicleType": "truck",
		"vehicleTemperatureCapability": "reefer", "vehicleDepot": "DEPOT_NORTH",
		"allocations": allocs,
	}, true
}

func peerStub() http.Handler {
	r := chi.NewRouter()
	north := map[string]any{
		"planId": "plan-1", "planRef": "PLAN000001", "deliveryDate": "2026-09-29", "planStatus": "confirmed",
		"planVersion": 1, "planAcknowledgements": []map[string]any{{"actorId": "USR004", "actorRole": "LOADER"}},
		"tripId": "trip-north", "tripNumber": 1, "vehicleId": "VEH001", "vehicleType": "truck",
		"vehicleTemperatureCapability": "reefer", "vehicleDepot": "DEPOT_NORTH",
		"allocations": []map[string]any{
			{"allocationId": "a1", "orderId": "ord-1", "orderRef": "ORD000001", "outletId": "OUT034", "stopSequence": 1},
			{"allocationId": "a2", "orderId": "ord-2", "orderRef": "ORD000002", "outletId": "OUT021", "stopSequence": 2},
		},
	}
	clear := map[string]any{
		"planId": "plan-1", "planRef": "PLAN000001", "deliveryDate": "2026-09-29", "planStatus": "confirmed",
		"planVersion": 1, "planAcknowledgements": []map[string]any{{"actorId": "USR004", "actorRole": "LOADER"}},
		"tripId": "trip-clear", "tripNumber": 3, "vehicleId": "VEH003", "vehicleType": "truck",
		"vehicleTemperatureCapability": "reefer", "vehicleDepot": "DEPOT_NORTH",
		"allocations": []map[string]any{
			{"allocationId": "a4", "orderId": "ord-4", "orderRef": "ORD000004", "outletId": "OUT034", "stopSequence": 1},
		},
	}
	south := map[string]any{
		"planId": "plan-1", "planRef": "PLAN000001", "deliveryDate": "2026-09-29", "planStatus": "confirmed",
		"planVersion": 1, "planAcknowledgements": []map[string]any{{"actorId": "USR005", "actorRole": "LOADER"}},
		"tripId": "trip-south", "tripNumber": 2, "vehicleId": "VEH002", "vehicleType": "van",
		"vehicleTemperatureCapability": "dry", "vehicleDepot": "DEPOT_SOUTH",
		"allocations": []map[string]any{
			{"allocationId": "a3", "orderId": "ord-3", "orderRef": "ORD000003", "outletId": "OUT099", "stopSequence": 1},
		},
	}
	r.Get("/api/v1/planning/internal/trips", func(w http.ResponseWriter, req *http.Request) {
		items := []map[string]any{north, south, clear}
		if depot := req.URL.Query().Get("depot"); depot != "" {
			var filtered []map[string]any
			for _, it := range items {
				if it["vehicleDepot"] == depot {
					filtered = append(filtered, it)
				}
			}
			items = filtered
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	r.Get("/api/v1/planning/internal/trips/{tripId}", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "tripId")
		trip := north
		switch id {
		case "trip-move":
			moved, ok := moveTrip()
			if !ok {
				http.NotFound(w, req)
				return
			}
			trip = moved
		case "trip-south":
			trip = south
		case "trip-clear":
			trip = clear
		case "trip-north":
		default:
			http.NotFound(w, req)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"trip": trip})
	})
	r.Get("/api/v1/orders/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "id")
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"order": map[string]any{
			"id": id, "orderRef": strings.ToUpper(id), "outletId": "OUT034", "brand": "Fresh",
			"orderUnits": 10, "orderWeightKg": 40, "orderVolumeM3": 1.2, "temperatureRequirement": "chilled",
		}})
	})
	r.Post("/api/v1/shared/audit-events", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "ingested"})
	})
	return r
}

type resp struct {
	status int
	body   string
}

func do(t *testing.T, srv *httptest.Server, method, path, subject string, body []byte, idem string) resp {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+subject)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(res.Body)
	return resp{status: res.StatusCode, body: buf.String()}
}

func doWithPlanVersion(t *testing.T, srv *httptest.Server, path, subject string, planVersion int) resp {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+subject)
	req.Header.Set("If-Match", strconv.Itoa(planVersion))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(res.Body)
	return resp{status: res.StatusCode, body: buf.String()}
}

func applySQL(t *testing.T, ctx context.Context, dsn, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, string(b)); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}
