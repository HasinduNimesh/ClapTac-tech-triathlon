package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/objectstore"
	delclient "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/client"
	deldomain "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
	delhandler "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/retention"
	delservice "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/service"
	delstore "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/store"
)

var png1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

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
	if sub == "svc-planner" || sub == "svc-order" {
		p.Scopes = []string{authorization.PermDeliveriesReadInternal}
	}
	return p, nil
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

type staticProfiles struct{}

func (staticProfiles) Resolve(_ context.Context, subject string) (*authorization.Profile, error) {
	switch subject {
	case "usr-driver":
		return &authorization.Profile{UserID: "USR006", Subject: subject, Roles: []string{"DRIVER"}, VehicleID: "VEH001"}, nil
	case "usr-driver-other":
		return &authorization.Profile{UserID: "USR007", Subject: subject, Roles: []string{"DRIVER"}, VehicleID: "VEH099"}, nil
	case "usr-dispatcher":
		return &authorization.Profile{UserID: "USR002", Subject: subject, Roles: []string{"DISPATCHER"}}, nil
	case "usr-loader":
		return &authorization.Profile{UserID: "USR004", Subject: subject, Roles: []string{"LOADER"}, Depot: "DEPOT_NORTH"}, nil
	case "usr-loader-south":
		return &authorization.Profile{UserID: "USR005", Subject: subject, Roles: []string{"LOADER"}, Depot: "DEPOT_SOUTH"}, nil
	default:
		return &authorization.Profile{Subject: subject}, nil
	}
}

// TestOutletLastAttemptedScopedByDate proves FR-53's review follow-up: a
// delivery attempt must only count toward "was this outlet run since its
// last deferral" if it happened before the plan being evaluated, not just
// before "now". Without the beforeDate filter, reopening an older plan could
// be cleared by an attempt that, as of that plan's own date, had not
// happened yet.
func TestOutletLastAttemptedScopedByDate(t *testing.T) {
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
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0012_delivery.sql"))

	pool, err := db.Open(ctx, dsn, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := delstore.Postgres{Pool: pool}

	if _, err := pool.Exec(ctx, `
		INSERT INTO delivery.runs (trip_id, plan_id, plan_ref, delivery_date, vehicle_id, depot, status)
		VALUES ('trip-scoped', 'plan-scoped', 'PLAN000001', '2026-09-20', 'VEH001', 'DEPOT_NORTH', 'completed')
	`); err != nil {
		t.Fatal(err)
	}
	var runID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM delivery.runs WHERE trip_id='trip-scoped'`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	// Two outcomes for two different outlets: one on 2026-09-22 (before the
	// plan date used below), one on 2026-09-28 (after it). Plus a Colombo
	// (+5:30) date-boundary pair: 2026-09-24T18:29:59Z is 2026-09-24T23:59:59
	// in Colombo - still "before" 2026-09-25 - while 2026-09-24T19:00:00Z is
	// already 2026-09-25T00:30 in Colombo, i.e. the plan's own day, not
	// before it. A UTC-midnight comparison would wrongly include the latter.
	if _, err := pool.Exec(ctx, `
		INSERT INTO delivery.stops (run_id, allocation_id, order_id, outlet_id, stop_sequence, status, outcome_code, outcome_at)
		VALUES
			($1::uuid, 'a-early', 'ord-early', 'OUT-EARLY', 1, 'completed', 'DELIVERED', '2026-09-22T10:00:00Z'),
			($1::uuid, 'a-late', 'ord-late', 'OUT-LATE', 2, 'completed', 'NOT_DELIVERED', '2026-09-28T10:00:00Z'),
			($1::uuid, 'a-boundary-before', 'ord-boundary-before', 'OUT-BOUNDARY-BEFORE', 3, 'completed', 'DELIVERED', '2026-09-24T18:29:59Z'),
			($1::uuid, 'a-boundary-after', 'ord-boundary-after', 'OUT-BOUNDARY-AFTER', 4, 'completed', 'DELIVERED', '2026-09-24T19:00:00Z')
	`, runID); err != nil {
		t.Fatal(err)
	}

	// A plan dated 2026-09-25 must see the 2026-09-22 attempt and the
	// 18:29:59Z boundary case (still Sep 24 in Colombo), but not the
	// 2026-09-28 one or the 19:00:00Z boundary case (already Sep 25 in
	// Colombo - the plan's own day, not before it).
	items, err := store.OutletLastAttempted(ctx, "2026-09-25")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.OutletID] = true
	}
	if !seen["OUT-EARLY"] {
		t.Fatalf("expected OUT-EARLY's 2026-09-22 attempt to be visible to a 2026-09-25 plan: %+v", items)
	}
	if seen["OUT-LATE"] {
		t.Fatalf("OUT-LATE's 2026-09-28 attempt is in the future relative to a 2026-09-25 plan and must not be visible: %+v", items)
	}
	if !seen["OUT-BOUNDARY-BEFORE"] {
		t.Fatalf("2026-09-24T18:29:59Z is still 2026-09-24 in Colombo (+5:30) and must be visible to a 2026-09-25 plan: %+v", items)
	}
	if seen["OUT-BOUNDARY-AFTER"] {
		t.Fatalf("2026-09-24T19:00:00Z is already 2026-09-25T00:30 in Colombo (+5:30) - the plan's own day, not before it - and must not be visible: %+v", items)
	}

	// A later plan (2026-09-30) must see both.
	itemsLater, err := store.OutletLastAttempted(ctx, "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	seenLater := map[string]bool{}
	for _, it := range itemsLater {
		seenLater[it.OutletID] = true
	}
	if !seenLater["OUT-EARLY"] || !seenLater["OUT-LATE"] {
		t.Fatalf("a 2026-09-30 plan should see both earlier attempts: %+v", itemsLater)
	}
}

func TestDeliveryWorkflow(t *testing.T) {
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
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0002_identity.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0005_outlets_expand.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0012_delivery.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0017_master_data_versions.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0022_delivery_plan_version.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0024_trip_messages.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0028_delivery_proof_retention.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0029_delivery_lateness_history_index.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0034_delivery_planned_arrival_snapshot.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0030_outlet_access_instructions.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0031_cold_chain_readings.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0038_delivery_proof_receiver_name.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0039_delivery_driver_incidents.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0041_delivery_checkout.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0043_delivery_arrival_predictions.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0044_delivery_returns.sql"))

	peers := httptest.NewServer(peerStub())
	t.Cleanup(peers.Close)

	pool, err := db.Open(ctx, dsn, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	objects := &objectstore.Memory{}
	h := delhandler.Handler{
		Authn:    bearerAuth{},
		Profiles: staticProfiles{},
		Service: delservice.Service{
			Repo:    delstore.Postgres{Pool: pool},
			Peers:   delclient.Peers{LoadingURL: peers.URL, SharedURL: peers.URL, OrdersURL: peers.URL, M2M: staticToken("svc-delivery")},
			Objects: objects,
		},
	}
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	if code := do(t, srv, http.MethodGet, "/api/v1/delivery/trips?date=2026-09-29", "usr-loader", nil, "").status; code != http.StatusForbidden {
		t.Fatalf("loader list %d", code)
	}

	listed := do(t, srv, http.MethodGet, "/api/v1/delivery/trips?date=2026-09-29", "usr-driver", nil, "")
	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"tripId":"trip-north"`) {
		t.Fatalf("driver list %d %s", listed.status, listed.body)
	}
	if strings.Contains(listed.body, "trip-south") {
		t.Fatalf("other vehicle leaked: %s", listed.body)
	}
	queueHealth := do(t, srv, http.MethodPost, "/api/v1/delivery/telemetry/offline-queue", "usr-driver", []byte(`{"ageBucket":"7d_30d","countBucket":"2_5"}`), "")
	if queueHealth.status != http.StatusNoContent {
		t.Fatalf("driver queue-health telemetry %d %s", queueHealth.status, queueHealth.body)
	}
	queueHealthDenied := do(t, srv, http.MethodPost, "/api/v1/delivery/telemetry/offline-queue", "usr-dispatcher", []byte(`{"ageBucket":"7d_30d","countBucket":"2_5"}`), "")
	if queueHealthDenied.status != http.StatusForbidden {
		t.Fatalf("dispatcher queue-health telemetry %d %s", queueHealthDenied.status, queueHealthDenied.body)
	}

	disp := do(t, srv, http.MethodGet, "/api/v1/delivery/trips?date=2026-09-29", "usr-dispatcher", nil, "")
	if disp.status != http.StatusOK || !strings.Contains(disp.body, "trip-south") {
		t.Fatalf("dispatcher view-all %d %s", disp.status, disp.body)
	}

	if code := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north", "usr-driver-other", nil, "").status; code != http.StatusForbidden {
		t.Fatalf("other driver %d", code)
	}

	beforeCheckout := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/start", "usr-driver", nil, "start-before-checkout")
	if beforeCheckout.status != http.StatusConflict { t.Fatalf("departure without checkout %d %s", beforeCheckout.status, beforeCheckout.body) }
	staleCheckout := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/checkout", "usr-driver", []byte(`{"planVersion":2,"confirmedOrderIds":["ord-1","ord-2","ord-4","ord-5"]}`), "")
	if staleCheckout.status != http.StatusConflict { t.Fatalf("stale plan checkout %d %s", staleCheckout.status, staleCheckout.body) }
	wrongDriver := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/checkout", "usr-driver-other", []byte(`{"planVersion":1,"confirmedOrderIds":["ord-1","ord-2","ord-4","ord-5"]}`), "")
	if wrongDriver.status != http.StatusForbidden { t.Fatalf("other vehicle checkout %d %s", wrongDriver.status, wrongDriver.body) }
	blockedCheckout := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/checkout", "usr-driver", []byte(`{"planVersion":1,"confirmedOrderIds":["ord-1","ord-4","ord-5"]}`), "")
	if blockedCheckout.status != http.StatusOK || !strings.Contains(blockedCheckout.body, `"status":"blocked"`) {
		t.Fatalf("missing goods did not block checkout %d %s", blockedCheckout.status, blockedCheckout.body)
	}
	if alert := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north/checkout", "usr-loader", nil, ""); alert.status != http.StatusOK || !strings.Contains(alert.body, "ord-2") {
		t.Fatalf("loader did not receive checkout alert %d %s", alert.status, alert.body)
	}
	if alert := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north/checkout", "usr-dispatcher", nil, ""); alert.status != http.StatusOK || !strings.Contains(alert.body, "ord-2") {
		t.Fatalf("dispatcher did not receive checkout alert %d %s", alert.status, alert.body)
	}
	if other := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north/checkout", "usr-loader-south", nil, ""); other.status != http.StatusForbidden {
		t.Fatalf("other depot saw checkout alert %d %s", other.status, other.body)
	}
	if denied := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/start", "usr-driver", nil, "start-blocked"); denied.status != http.StatusConflict {
		t.Fatalf("departure after blocked checkout %d %s", denied.status, denied.body)
	}
	confirmedCheckout := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/checkout", "usr-driver", []byte(`{"planVersion":1,"confirmedOrderIds":["ord-1","ord-2","ord-4","ord-5"]}`), "")
	if confirmedCheckout.status != http.StatusOK || !strings.Contains(confirmedCheckout.body, `"status":"confirmed"`) {
		t.Fatalf("complete checkout %d %s", confirmedCheckout.status, confirmedCheckout.body)
	}
	duplicateCheckout := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/checkout", "usr-driver", []byte(`{"planVersion":1,"confirmedOrderIds":["ord-5","ord-4","ord-2","ord-1"]}`), "")
	if duplicateCheckout.status != http.StatusOK || duplicateCheckout.body != confirmedCheckout.body {
		t.Fatalf("duplicate checkout changed the recorded report: %d %s", duplicateCheckout.status, duplicateCheckout.body)
	}
	reblockedCheckout := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/checkout", "usr-driver", []byte(`{"planVersion":1,"confirmedOrderIds":["ord-1","ord-4","ord-5"]}`), "")
	if reblockedCheckout.status != http.StatusOK || !strings.Contains(reblockedCheckout.body, `"status":"blocked"`) {
		t.Fatalf("new missing goods must block a prior checkout: %d %s", reblockedCheckout.status, reblockedCheckout.body)
	}
	if denied := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/start", "usr-driver", nil, "start-reblocked"); denied.status != http.StatusConflict {
		t.Fatalf("departure after a revised blocked checkout %d %s", denied.status, denied.body)
	}
	reconfirmed := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/checkout", "usr-driver", []byte(`{"planVersion":1,"confirmedOrderIds":["ord-1","ord-2","ord-4","ord-5"]}`), "")
	if reconfirmed.status != http.StatusOK || !strings.Contains(reconfirmed.body, `"status":"confirmed"`) {
		t.Fatalf("reconfirmed checkout %d %s", reconfirmed.status, reconfirmed.body)
	}
	started := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/start", "usr-driver", nil, "start-1")
	if started.status != http.StatusOK {
		t.Fatalf("start %d %s", started.status, started.body)
	}
	startReplay := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/start", "usr-driver", nil, "start-1")
	if startReplay.status != http.StatusOK || !strings.Contains(startReplay.body, `"in_progress"`) {
		t.Fatalf("start replay %d %s", startReplay.status, startReplay.body)
	}
	var detail struct {
		Stops []struct {
			ID                          string    `json:"id"`
			Status                      string    `json:"status"`
			AccessInstructions          string    `json:"accessInstructions"`
			AccessInstructionsUpdatedAt time.Time `json:"accessInstructionsUpdatedAt"`
			ChilledTemperatureMinC      *float64  `json:"chilledTemperatureMinC"`
			ChilledTemperatureMaxC      *float64  `json:"chilledTemperatureMaxC"`
		} `json:"stops"`
		Run struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"run"`
	}
	if err := json.Unmarshal([]byte(started.body), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Run.Status != "in_progress" || len(detail.Stops) != 4 {
		t.Fatalf("started %s", started.body)
	}
	expectedAccessNoteUpdatedAt, _ := time.Parse(time.RFC3339, "2026-09-28T10:00:00Z")
	if detail.Stops[0].AccessInstructions != "Use the red gate beside the pharmacy; final 200 m is a narrow lane." || !detail.Stops[0].AccessInstructionsUpdatedAt.Equal(expectedAccessNoteUpdatedAt) {
		t.Fatalf("stop did not retain the outlet access note snapshot: %+v", detail.Stops[0])
	}
	if detail.Stops[0].ChilledTemperatureMinC == nil || detail.Stops[0].ChilledTemperatureMaxC == nil || *detail.Stops[0].ChilledTemperatureMinC != -5 || *detail.Stops[0].ChilledTemperatureMaxC != 5 {
		t.Fatalf("outlet temperature limits were not snapshotted into the run: %+v", detail.Stops[0])
	}
	stopID := detail.Stops[0].ID
	stop2 := detail.Stops[1].ID
	stop3 := detail.Stops[2].ID
	stop4 := detail.Stops[3].ID
	colombo := time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	historicalArrivalResiduals := []int{-20, -4, 10, 4, 18, 0, 12, -8, 2, 20, -14, 6, 16, -2, 8, -12, 14, -6, 0, 4, -18, 12, 2, 10}
	for i := 0; i < 48; i++ {
		deliveryDate := time.Date(2026, time.August, i+1, 0, 0, 0, 0, time.UTC)
		tripID := fmt.Sprintf("history-%02d", i+1)
		var runID string
		if err := pool.QueryRow(ctx, `INSERT INTO delivery.runs (trip_id,plan_id,plan_ref,delivery_date,vehicle_id,depot,trip_number,status) VALUES ($1,'historical-plan','HISTORY',$2,'VEH001','DEPOT_NORTH',1,'completed') RETURNING id::text`, tripID, deliveryDate).Scan(&runID); err != nil {
			t.Fatal(err)
		}
		arrivedAt := time.Date(deliveryDate.Year(), deliveryDate.Month(), deliveryDate.Day(), 8, 30, 0, 0, colombo)
		if (i/2)%3 == 0 {
			arrivedAt = time.Date(deliveryDate.Year(), deliveryDate.Month(), deliveryDate.Day(), 9, 30, 0, 0, colombo)
		}
		temperature := "chilled"
		if i%2 == 1 {
			temperature = "ambient"
		}
		residual := historicalArrivalResiduals[i/2]
		plannedArrival := arrivedAt.Add(-time.Duration(residual) * time.Minute)
		if _, err := pool.Exec(ctx, `INSERT INTO delivery.stops (run_id,allocation_id,order_id,outlet_id,brand,temperature_requirement,stop_sequence,planned_window_close,planned_arrival_at,status,arrived_at) VALUES ($1::uuid,$2,$3,'OUT034','Fresh',$4,1,'09:00',$5,'completed',$6)`, runID, tripID, tripID, temperature, plannedArrival, arrivedAt); err != nil {
			t.Fatal(err)
		}
	}
	arrivalOutliers := []struct {
		tripID, date, depot, brand, temperature, window string
	}{
		{"history-too-old-chilled", "2026-06-30", "DEPOT_NORTH", "Fresh", "chilled", "09:00"},
		{"history-too-old-ambient", "2026-06-30", "DEPOT_NORTH", "Fresh", "ambient", "09:00"},
		{"history-other-depot", "2026-09-20", "DEPOT_SOUTH", "Fresh", "chilled", "09:00"},
		{"history-other-brand", "2026-09-21", "DEPOT_NORTH", "Style", "chilled", "09:00"},
		{"history-invalid-window", "2026-09-22", "DEPOT_NORTH", "Fresh", "ambient", "24:00"},
	}
	for _, sample := range arrivalOutliers {
		var runID string
		if err := pool.QueryRow(ctx, `INSERT INTO delivery.runs (trip_id,plan_id,plan_ref,delivery_date,vehicle_id,depot,trip_number,status) VALUES ($1,'historical-plan','HISTORY',$2,'VEH001',$3,1,'completed') RETURNING id::text`, sample.tripID, sample.date, sample.depot).Scan(&runID); err != nil {
			t.Fatal(err)
		}
		arrivedAt := time.Date(2026, time.September, 23, 9, 30, 0, 0, colombo)
		if _, err := pool.Exec(ctx, `INSERT INTO delivery.stops (run_id,allocation_id,order_id,outlet_id,brand,temperature_requirement,stop_sequence,planned_window_close,status,arrived_at) VALUES ($1::uuid,$2,$3,'OUT034',$4,$5,1,$6,'completed',$7)`, runID, sample.tripID, sample.tripID, sample.brand, sample.temperature, sample.window, arrivedAt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (delstore.Postgres{Pool: pool}).LatenessHistory(ctx, "trip-north"); err != nil {
		t.Fatalf("query lateness history directly: %v", err)
	}
	lateness := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north/lateness-history", "usr-dispatcher", nil, "")
	if lateness.status != http.StatusOK {
		t.Fatalf("dispatcher lateness history %d %s", lateness.status, lateness.body)
	}
	var latenessResponse struct {
		Items []deldomain.LatenessProbability `json:"items"`
	}
	if err := json.Unmarshal([]byte(lateness.body), &latenessResponse); err != nil || len(latenessResponse.Items) != 2 {
		t.Fatalf("invalid lateness response %s: %v", lateness.body, err)
	}
	// Both temperature cohorts must have independent rates and rolling scores.
	// The 90-day boundary outlier is outside the current estimate, but remains
	// valid training history for earlier out-of-time predictions in the 180-day backtest.
	priorLate, scoreSum, scoreSamples := 1, 0.0, 0
	for i := 0; i < 24; i++ {
		priorCount := i + 1 // one valid 91-day boundary arrival plus earlier observations in this condition
		if priorCount >= 10 {
			predicted := float64(priorLate+1) / float64(priorCount+2)
			actual := 0.0
			if i%3 == 0 {
				actual = 1
			}
			brier := predicted - actual
			scoreSum += brier * brier
			scoreSamples++
		}
		if i%3 == 0 {
			priorLate++
		}
	}
	expectedBrier := math.Round(scoreSum/float64(scoreSamples)*10000) / 10000
	for _, observed := range latenessResponse.Items {
		if observed.TemperatureRequirement != "chilled" && observed.TemperatureRequirement != "ambient" {
			t.Fatalf("unexpected operating condition: %+v", observed)
		}
		if observed.SampleCount != 24 || observed.LateCount != 8 || observed.Probability == nil || math.Abs(*observed.Probability-0.3462) > 0.0001 {
			t.Fatalf("lateness estimate=%+v", observed)
		}
		if observed.ArrivalRangeStatus != "CALIBRATED" || observed.ArrivalRangeSamples != 24 || observed.ArrivalRangeHoldouts != 14 || observed.ArrivalRangeCoverage == nil || math.Abs(*observed.ArrivalRangeCoverage-(11.0/14.0)) > 0.0001 || observed.ArrivalOffsetP10 == nil || math.Abs(*observed.ArrivalOffsetP10-(-13.4)) > 0.01 || observed.ArrivalOffsetP90 == nil || math.Abs(*observed.ArrivalOffsetP90-15.4) > 0.01 {
			t.Fatalf("arrival range calibration=%+v", observed)
		}
		if observed.CalibrationStatus != "EVALUATED" || observed.CalibrationVersion != deldomain.LatenessCalibrationVersion || scoreSamples != 15 || observed.CalibrationSampleCount != scoreSamples || observed.BrierScore == nil || math.Abs(*observed.BrierScore-expectedBrier) > 0.0001 {
			t.Fatalf("lateness calibration=%+v expected Brier %.4f with %d samples", observed, expectedBrier, scoreSamples)
		}
	}
	if denied := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north/lateness-history", "usr-driver", nil, ""); denied.status != http.StatusForbidden {
		t.Fatalf("driver lateness history %d %s", denied.status, denied.body)
	}
	message := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/messages", "usr-dispatcher", []byte(`{"stopId":"`+stopID+`","body":"Use the south entrance"}`), "")
	if message.status != http.StatusCreated || !strings.Contains(message.body, "Use the south entrance") {
		t.Fatalf("message send %d %s", message.status, message.body)
	}
	var sent struct {
		Message deldomain.TripMessage `json:"message"`
	}
	if err := json.Unmarshal([]byte(message.body), &sent); err != nil {
		t.Fatal(err)
	}
	msgs := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north/messages", "usr-driver", nil, "")
	if msgs.status != http.StatusOK || !strings.Contains(msgs.body, sent.Message.ID) {
		t.Fatalf("driver message view %d %s", msgs.status, msgs.body)
	}
	acked := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/messages/"+sent.Message.ID+"/ack", "usr-driver", nil, "")
	if acked.status != http.StatusOK || !strings.Contains(acked.body, `"acknowledgedBy":"USR006"`) {
		t.Fatalf("message acknowledgement %d %s", acked.status, acked.body)
	}
	var messageEvents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.trip_message_events WHERE message_id=$1::uuid`, sent.Message.ID).Scan(&messageEvents); err != nil || messageEvents != 2 {
		t.Fatalf("message audit events=%d err=%v", messageEvents, err)
	}

	if code := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/complete", "usr-driver", []byte(`{}`), "done-early").status; code != http.StatusConflict {
		t.Fatalf("complete early %d", code)
	}
	tempBeforeArrival := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"temp-before-arrival","type":"TEMPERATURE_READING","tripId":"trip-north","stopId":"`+stopID+`","payload":{"valueC":2.0}}]}`), "")
	if tempBeforeArrival.status != http.StatusOK || !strings.Contains(tempBeforeArrival.body, `"REJECTED"`) {
		t.Fatalf("temperature readings must wait until the driver arrives: %d %s", tempBeforeArrival.status, tempBeforeArrival.body)
	}

	arrived := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/stops/"+stopID+"/arrive", "usr-driver", []byte(`{"occurredAt":"2026-09-29T08:00:00Z"}`), "arr-1")
	if arrived.status != http.StatusOK {
		t.Fatalf("arrive %d %s", arrived.status, arrived.body)
	}
	tempOut := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"temp-out-1","type":"TEMPERATURE_READING","tripId":"trip-north","stopId":"`+stopID+`","occurredAt":"2026-09-29T08:02:00Z","payload":{"valueC":8.2,"note":"Probe at receiving door"}}]}`), "")
	if tempOut.status != http.StatusOK || !strings.Contains(tempOut.body, `"OUT_OF_RANGE"`) || !strings.Contains(tempOut.body, `"actorId":"USR006"`) || !strings.Contains(tempOut.body, `"unit":"C"`) {
		t.Fatalf("out-of-range temperature evidence %d %s", tempOut.status, tempOut.body)
	}
	var temperatureCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.temperature_readings WHERE stop_id=$1::uuid AND actor_id='USR006' AND occurred_at='2026-09-29T08:02:00Z'::timestamptz AND evaluation='OUT_OF_RANGE'`, stopID).Scan(&temperatureCount); err != nil || temperatureCount != 1 {
		t.Fatalf("temperature evidence count=%d err=%v", temperatureCount, err)
	}
	tempReplay := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"temp-out-1","type":"TEMPERATURE_READING","tripId":"trip-north","stopId":"`+stopID+`","payload":{"valueC":1.5}}]}`), "")
	if tempReplay.status != http.StatusOK || !strings.Contains(tempReplay.body, `"DUPLICATE"`) {
		t.Fatalf("temperature idempotent replay %d %s", tempReplay.status, tempReplay.body)
	}
	tempIn := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"temp-in-1","type":"TEMPERATURE_READING","tripId":"trip-north","stopId":"`+stopID+`","occurredAt":"2026-09-29T08:03:00Z","payload":{"valueC":2.4}}]}`), "")
	if tempIn.status != http.StatusOK || !strings.Contains(tempIn.body, `"IN_RANGE"`) {
		t.Fatalf("in-range temperature evidence %d %s", tempIn.status, tempIn.body)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.temperature_readings WHERE stop_id=$1::uuid`, stopID).Scan(&temperatureCount); err != nil || temperatureCount != 2 {
		t.Fatalf("temperature readings must append, count=%d err=%v", temperatureCount, err)
	}
	tripWithTemperature := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north", "usr-driver", nil, "")
	if tripWithTemperature.status != http.StatusOK || strings.Count(tripWithTemperature.body, `"operationId":"temp-`) < 2 {
		t.Fatalf("trip detail must return its append-only temperature evidence: %d %s", tripWithTemperature.status, tripWithTemperature.body)
	}

	outNoProof := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/stops/"+stopID+"/outcome", "usr-driver", []byte(`{"code":"DELIVERED","dependsOnOperationId":"proof-1"}`), "out-early")
	if outNoProof.status != http.StatusBadRequest {
		t.Fatalf("outcome without proof %d %s", outNoProof.status, outNoProof.body)
	}

	syncBeforeProof := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"out-sync-early","type":"STOP_OUTCOME","tripId":"trip-north","stopId":"`+stopID+`","dependsOnOperationId":"proof-1","payload":{"code":"DELIVERED"}}]}`), "")
	if syncBeforeProof.status != http.StatusOK || !strings.Contains(syncBeforeProof.body, `"REJECTED"`) {
		t.Fatalf("sync before proof %d %s", syncBeforeProof.status, syncBeforeProof.body)
	}

	pr := uploadProofWithReceiver(t, srv, "trip-north", stopID, "usr-driver", "proof-1", "SIGNATURE", "Nimal Perera")
	if pr.status != http.StatusCreated || !strings.Contains(pr.body, `"receiverName":"Nimal Perera"`) {
		t.Fatalf("proof %d %s", pr.status, pr.body)
	}
	replayProof := uploadProof(t, srv, "trip-north", stopID, "usr-driver", "proof-1", "PHOTO")
	if replayProof.status != http.StatusCreated || !strings.Contains(replayProof.body, "SIGNATURE") {
		t.Fatalf("proof replay %d %s", replayProof.status, replayProof.body)
	}
	if len(objects.Objects) != 1 {
		t.Fatalf("object count %d", len(objects.Objects))
	}

	outOK := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/stops/"+stopID+"/outcome", "usr-driver", []byte(`{"code":"DELIVERED","dependsOnOperationId":"proof-1","occurredAt":"2026-09-29T08:10:00Z"}`), "out-1")
	if outOK.status != http.StatusOK || !strings.Contains(outOK.body, `"outcomeCode":"DELIVERED"`) {
		t.Fatalf("delivered %d %s", outOK.status, outOK.body)
	}

	syncDup := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"out-1","type":"STOP_OUTCOME","tripId":"trip-north","stopId":"`+stopID+`","payload":{"code":"FAILED"}}]}`), "")
	if syncDup.status != http.StatusOK || !strings.Contains(syncDup.body, `"DUPLICATE"`) || !strings.Contains(syncDup.body, `"APPLIED"`) {
		t.Fatalf("duplicate %d %s", syncDup.status, syncDup.body)
	}
	got := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north", "usr-driver", nil, "")
	if strings.Contains(got.body, `"outcomeCode":"FAILED"`) {
		t.Fatalf("duplicate overwrote APPLIED: %s", got.body)
	}

	proofJSON := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"proof-json","type":"PROOF_UPLOAD","tripId":"trip-north","stopId":"`+stop2+`"}]}`), "")
	if !strings.Contains(proofJSON.body, `"REJECTED"`) {
		t.Fatalf("proof in sync %s", proofJSON.body)
	}

	arrive2 := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"arr-2","type":"ARRIVED","tripId":"trip-north","stopId":"`+stop2+`","occurredAt":"2026-09-29T09:00:00Z"}]}`), "")
	if arrive2.status != http.StatusOK || !strings.Contains(arrive2.body, `"APPLIED"`) {
		t.Fatalf("sync arrive 2 %d %s", arrive2.status, arrive2.body)
	}
	tempUnconfigured := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"temp-unconfigured","type":"TEMPERATURE_READING","tripId":"trip-north","stopId":"`+stop2+`","occurredAt":"2026-09-29T09:01:00Z","payload":{"valueC":3.4}}]}`), "")
	if tempUnconfigured.status != http.StatusOK || !strings.Contains(tempUnconfigured.body, `"LIMITS_UNCONFIGURED"`) {
		t.Fatalf("unconfigured temperature limits must prompt review, not claim safe: %d %s", tempUnconfigured.status, tempUnconfigured.body)
	}
	if code := uploadProofWithReceiver(t, srv, "trip-north", stop2, "usr-driver", "proof-2", "PHOTO", "Kamal Silva").status; code != http.StatusCreated {
		t.Fatalf("photo 2")
	}
	proofTracking := do(t, srv, http.MethodGet, "/api/v1/delivery/internal/orders/ord-2", "svc-order", nil, "")
	if proofTracking.status != http.StatusOK || !strings.Contains(proofTracking.body, `"operationId":"proof-2"`) || !strings.Contains(proofTracking.body, `"type":"PHOTO"`) {
		t.Fatalf("internal order view did not expose uploaded photo identity: %d %s", proofTracking.status, proofTracking.body)
	}
	if !strings.Contains(proofTracking.body, `"receiverName":"Kamal Silva"`) {
		t.Fatalf("FR-25: internal order view did not expose the proof's recipient name: %s", proofTracking.body)
	}
	partial := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"out-2","type":"STOP_OUTCOME","tripId":"trip-north","stopId":"`+stop2+`","dependsOnOperationId":"proof-2","payload":{"code":"PARTIAL","occurredAt":"2026-09-29T09:10:00Z"}}]}`), "")
	if partial.status != http.StatusOK || !strings.Contains(partial.body, `"APPLIED"`) {
		t.Fatalf("sync partial %d %s", partial.status, partial.body)
	}
	arrive3 := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"arr-3","type":"ARRIVED","tripId":"trip-north","stopId":"`+stop3+`"}]}`), "")
	if arrive3.status != http.StatusOK || !strings.Contains(arrive3.body, `"APPLIED"`) {
		t.Fatalf("sync arrive 3 %d %s", arrive3.status, arrive3.body)
	}
	ambientTemp := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"temp-ambient","type":"TEMPERATURE_READING","tripId":"trip-north","stopId":"`+stop3+`","payload":{"valueC":2.0}}]}`), "")
	if ambientTemp.status != http.StatusOK || !strings.Contains(ambientTemp.body, `"REJECTED"`) {
		t.Fatalf("ambient stop must reject cold-chain reading %d %s", ambientTemp.status, ambientTemp.body)
	}

	// FR-22: a Driver-reported incident tied to a stop (a goods issue found on arrival).
	incidentAtStop := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"incident-goods","type":"INCIDENT_REPORT","tripId":"trip-north","stopId":"`+stop3+`","payload":{"category":"goods","description":"Carton for this order arrived crushed."}}]}`), "")
	if incidentAtStop.status != http.StatusOK || !strings.Contains(incidentAtStop.body, `"APPLIED"`) {
		t.Fatalf("stop-scoped incident report %d %s", incidentAtStop.status, incidentAtStop.body)
	}
	// An incident with no stop (a road closure between stops) must still be accepted.
	incidentNoStop := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"incident-road","type":"INCIDENT_REPORT","tripId":"trip-north","payload":{"category":"ROAD","description":"Road closed near the Kelani bridge; took the Peliyagoda detour."}}]}`), "")
	if incidentNoStop.status != http.StatusOK || !strings.Contains(incidentNoStop.body, `"APPLIED"`) {
		t.Fatalf("trip-scoped incident report without a stop %d %s", incidentNoStop.status, incidentNoStop.body)
	}
	// An unrecognised category is rejected rather than silently stored as OTHER.
	badIncident := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"incident-bad","type":"INCIDENT_REPORT","tripId":"trip-north","payload":{"category":"WEATHER","description":"Heavy rain."}}]}`), "")
	if badIncident.status != http.StatusOK || !strings.Contains(badIncident.body, `"REJECTED"`) {
		t.Fatalf("unrecognised incident category must be rejected %d %s", badIncident.status, badIncident.body)
	}
	// A different vehicle's driver cannot report an incident against this trip.
	incidentWrongVehicle := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver-other", []byte(`{"operations":[{"operationId":"incident-forbidden","type":"INCIDENT_REPORT","tripId":"trip-north","payload":{"category":"SAFETY","description":"Should not be accepted from another driver."}}]}`), "")
	if incidentWrongVehicle.status != http.StatusOK || !strings.Contains(incidentWrongVehicle.body, `"REJECTED"`) {
		t.Fatalf("incident report from an unassigned vehicle must be rejected %d %s", incidentWrongVehicle.status, incidentWrongVehicle.body)
	}
	// Idempotent replay of the same operationId does not create a duplicate record.
	incidentReplay := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"incident-goods","type":"INCIDENT_REPORT","tripId":"trip-north","stopId":"`+stop3+`","payload":{"category":"goods","description":"Carton for this order arrived crushed."}}]}`), "")
	if incidentReplay.status != http.StatusOK || !strings.Contains(incidentReplay.body, `"DUPLICATE"`) {
		t.Fatalf("replayed incident report should report duplicate, not re-apply %d %s", incidentReplay.status, incidentReplay.body)
	}
	missingReason := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/stops/"+stop3+"/outcome", "usr-driver", []byte(`{"code":"NOT_DELIVERED"}`), "out-3-no-reason")
	if missingReason.status != http.StatusBadRequest {
		t.Fatalf("not-delivered outcome without reason code should fail: %d %s", missingReason.status, missingReason.body)
	}
	invalidReason := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/stops/"+stop3+"/outcome", "usr-driver", []byte(`{"code":"NOT_DELIVERED","reason":"closed"}`), "out-3-invalid-reason")
	if invalidReason.status != http.StatusBadRequest {
		t.Fatalf("free-text reason should not replace structured reason code: %d %s", invalidReason.status, invalidReason.body)
	}
	failedWithoutReason := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/stops/"+stop3+"/outcome", "usr-driver", []byte(`{"code":"FAILED"}`), "failed-no-reason")
	if failedWithoutReason.status != http.StatusBadRequest {
		t.Fatalf("failed outcome without structured reason should fail: %d %s", failedWithoutReason.status, failedWithoutReason.body)
	}
	result := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"out-3","type":"STOP_OUTCOME","tripId":"trip-north","stopId":"`+stop3+`","payload":{"code":"FAILED","reason":"VEHICLE_ISSUE"}}]}`), "")
	if result.status != http.StatusOK || !strings.Contains(result.body, `"APPLIED"`) {
		t.Fatalf("sync failed outcome %d %s", result.status, result.body)
	}
	arrive4 := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"arr-4","type":"ARRIVED","tripId":"trip-north","stopId":"`+stop4+`"}]}`), "")
	if arrive4.status != http.StatusOK || !strings.Contains(arrive4.body, `"APPLIED"`) {
		t.Fatalf("sync arrive 4 %d %s", arrive4.status, arrive4.body)
	}
	refusedWithoutReason := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/stops/"+stop4+"/outcome", "usr-driver", []byte(`{"code":"REFUSED","reason":"unknown"}`), "refused-invalid-reason")
	if refusedWithoutReason.status != http.StatusBadRequest {
		t.Fatalf("refused outcome with an invalid structured reason should fail: %d %s", refusedWithoutReason.status, refusedWithoutReason.body)
	}
	missingReturn := do(t, srv, http.MethodPost, "/api/v1/delivery/trips/trip-north/stops/"+stop4+"/outcome", "usr-driver", []byte(`{"code":"REFUSED","reason":"GOODS_REJECTED"}`), "refused-missing-return")
	if missingReturn.status != http.StatusBadRequest {
		t.Fatalf("refused outcome must require return details: %d %s", missingReturn.status, missingReturn.body)
	}
	refused := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"out-4","type":"STOP_OUTCOME","tripId":"trip-north","stopId":"`+stop4+`","payload":{"code":"REFUSED","reason":"GOODS_REJECTED","returnedGoods":{"goods":"Rejected cartons","units":2,"resolution":"NEXT_RUN"}}}]}`), "")
	if refused.status != http.StatusOK || !strings.Contains(refused.body, `"APPLIED"`) {
		t.Fatalf("sync refused outcome %d %s", refused.status, refused.body)
	}
	replayedReturn := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"out-4","type":"STOP_OUTCOME","tripId":"trip-north","stopId":"`+stop4+`","payload":{"code":"REFUSED","reason":"GOODS_REJECTED","returnedGoods":{"goods":"Rejected cartons","units":2,"resolution":"NEXT_RUN"}}}]}`), "")
	if replayedReturn.status != http.StatusOK || !strings.Contains(replayedReturn.body, `"DUPLICATE"`) {
		t.Fatalf("duplicate return must not create a new attempt: %d %s", replayedReturn.status, replayedReturn.body)
	}
	changedReturn := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"out-4-changed","type":"STOP_OUTCOME","tripId":"trip-north","stopId":"`+stop4+`","payload":{"code":"REFUSED","reason":"GOODS_REJECTED","returnedGoods":{"goods":"Other goods","units":1,"resolution":"REQUEST_DEFERRAL"}}}]}`), "")
	if changedReturn.status != http.StatusOK || !strings.Contains(changedReturn.body, `"CONFLICT"`) {
		t.Fatalf("changed return cannot replace the original attempt: %d %s", changedReturn.status, changedReturn.body)
	}
	var returnCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.returned_goods WHERE stop_id=$1::uuid`, stop4).Scan(&returnCount); err != nil || returnCount != 1 {
		t.Fatalf("returned goods count=%d err=%v", returnCount, err)
	}

    returnedTracking := do(t, srv, http.MethodGet, "/api/v1/delivery/internal/orders/ord-5", "svc-order", nil, "")
    if returnedTracking.status != http.StatusOK || !strings.Contains(returnedTracking.body, `"goods":"Rejected cartons"`) ||
        !strings.Contains(returnedTracking.body, `"followupOrderRef":"ORD000006"`) {
        t.Fatalf("returned goods tracking: %d %s", returnedTracking.status, returnedTracking.body)
    }
    var dispatcherNotices int
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.trip_messages WHERE stop_id=$1::uuid AND sent_by='system:returned-goods'`, stop4).Scan(&dispatcherNotices); err != nil || dispatcherNotices != 1 {
        t.Fatalf("dispatcher return notice count=%d err=%v", dispatcherNotices, err)
    }
	done := do(t, srv, http.MethodPost, "/api/v1/delivery/sync", "usr-driver", []byte(`{"operations":[{"operationId":"done-1","type":"ROUTE_COMPLETED","tripId":"trip-north"}]}`), "")
	if done.status != http.StatusOK || !strings.Contains(done.body, `"APPLIED"`) {
		t.Fatalf("complete %d %s", done.status, done.body)
	}
	dispatcherDetail := do(t, srv, http.MethodGet, "/api/v1/delivery/trips/trip-north", "usr-dispatcher", nil, "")
	if dispatcherDetail.status != http.StatusOK || !strings.Contains(dispatcherDetail.body, `"outcomeCode":"DELIVERED"`) || !strings.Contains(dispatcherDetail.body, `"outcomeCode":"PARTIAL"`) || !strings.Contains(dispatcherDetail.body, `"outcomeCode":"FAILED"`) || !strings.Contains(dispatcherDetail.body, `"outcomeCode":"REFUSED"`) {
		t.Fatalf("dispatcher did not see delivery outcomes: %d %s", dispatcherDetail.status, dispatcherDetail.body)
	}
	lastServed := do(t, srv, http.MethodGet, "/api/v1/delivery/internal/outlets/last-served", "svc-planner", nil, "")
	if lastServed.status != http.StatusOK || !strings.Contains(lastServed.body, `"outletId":"OUT034"`) || !strings.Contains(lastServed.body, `"outletId":"OUT021"`) || strings.Contains(lastServed.body, `"outletId":"OUT055"`) {
		t.Fatalf("internal last-served signal must include delivered/partial only: %d %s", lastServed.status, lastServed.body)
	}

	loadingReady, err := http.Get(peers.URL + "/api/v1/loading/internal/trips/trip-north")
	if err != nil {
		t.Fatal(err)
	}
	defer loadingReady.Body.Close()
	body, _ := io.ReadAll(loadingReady.Body)
	if loadingReady.StatusCode != http.StatusOK || !strings.Contains(string(body), `"loadingStatus":"ready"`) {
		t.Fatalf("loading snapshot mutated %d %s", loadingReady.StatusCode, body)
	}

	var expiringProofID, expiringObject, heldProofID, heldObject string
	if err := pool.QueryRow(ctx, `SELECT id::text,object_key FROM delivery.proofs WHERE stop_id=$1::uuid`, stopID).Scan(&expiringProofID, &expiringObject); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text,object_key FROM delivery.proofs WHERE stop_id=$1::uuid`, stop2).Scan(&heldProofID, &heldObject); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE delivery.runs SET completed_at=$2 WHERE trip_id=$1`, "trip-north", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE delivery.proofs SET retention_hold=TRUE,retention_hold_reason='open customer claim' WHERE id=$1::uuid`, heldProofID); err != nil {
		t.Fatal(err)
	}
	runner := retention.Runner{Store: delstore.Postgres{Pool: pool}, Files: objects, Days: 180, Now: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }}
	erased, err := runner.Sweep(ctx)
	if err != nil || erased != 1 {
		t.Fatalf("retention sweep erased=%d err=%v", erased, err)
	}
	if _, ok := objects.Objects[expiringObject]; ok {
		t.Fatal("expired proof blob was not removed")
	}
	if _, ok := objects.Objects[heldObject]; !ok {
		t.Fatal("proof on customer-claim hold was deleted")
	}
	var activeProofs, erasedEvents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.proofs WHERE id IN ($1::uuid,$2::uuid)`, expiringProofID, heldProofID).Scan(&activeProofs); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery.proof_retention_events WHERE proof_id=$1::uuid`, expiringProofID).Scan(&erasedEvents); err != nil {
		t.Fatal(err)
	}
	if activeProofs != 1 || erasedEvents != 1 {
		t.Fatalf("proof metadata lifecycle active=%d erasure_events=%d", activeProofs, erasedEvents)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM delivery.proof_retention_events WHERE proof_id=$1::uuid`, expiringProofID); err == nil {
		t.Fatal("proof retention audit must be append-only")
	}
}

func peerStub() http.Handler {
	r := chi.NewRouter()
	north := map[string]any{
		"tripId": "trip-north", "planId": "plan-1", "planRef": "PLAN000001", "deliveryDate": "2026-09-29",
		"vehicleId": "VEH001", "vehicleType": "truck", "vehicleTemperatureCapability": "reefer",
		"depot": "DEPOT_NORTH", "tripNumber": 1, "loadingStatus": "ready",
		"planVersion": 1, "planAcknowledgements": []map[string]any{{"actorId": "USR006", "actorRole": "DRIVER"}},
		"orders": []map[string]any{
			{"allocationId": "a1", "orderId": "ord-1", "orderRef": "ORD000001", "outletId": "OUT034", "brand": "Fresh", "stopSequence": 1, "loadingStatus": "loaded", "temperatureRequirement": "chilled", "expectedUnits": 10, "shortfallSummary": []any{}},
			{"allocationId": "a2", "orderId": "ord-2", "orderRef": "ORD000002", "outletId": "OUT021", "brand": "Fresh", "stopSequence": 2, "loadingStatus": "loaded", "temperatureRequirement": "chilled", "expectedUnits": 8, "shortfallSummary": []any{}},
			{"allocationId": "a4", "orderId": "ord-4", "orderRef": "ORD000004", "outletId": "OUT055", "brand": "Fresh", "stopSequence": 3, "loadingStatus": "loaded", "temperatureRequirement": "ambient", "expectedUnits": 6, "shortfallSummary": []any{}},
			{"allocationId": "a5", "orderId": "ord-5", "orderRef": "ORD000005", "outletId": "OUT056", "brand": "Fresh", "stopSequence": 4, "loadingStatus": "loaded", "temperatureRequirement": "ambient", "expectedUnits": 4, "shortfallSummary": []any{}},
		},
	}
	south := map[string]any{
		"tripId": "trip-south", "planId": "plan-1", "planRef": "PLAN000001", "deliveryDate": "2026-09-29",
		"vehicleId": "VEH002", "depot": "DEPOT_SOUTH", "tripNumber": 2, "loadingStatus": "ready", "planVersion": 1,
		"orders": []map[string]any{
			{"allocationId": "a3", "orderId": "ord-3", "orderRef": "ORD000003", "outletId": "OUT099", "stopSequence": 1, "loadingStatus": "loaded", "shortfallSummary": []any{}},
		},
	}
	r.Get("/api/v1/loading/internal/trips", func(w http.ResponseWriter, req *http.Request) {
		items := []map[string]any{north, south}
		if v := req.URL.Query().Get("vehicleId"); v != "" {
			var filtered []map[string]any
			for _, it := range items {
				if it["vehicleId"] == v {
					filtered = append(filtered, it)
				}
			}
			items = filtered
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	r.Get("/api/v1/loading/internal/trips/{tripId}", func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "tripId")
		if id == "trip-south" {
			httpx.WriteJSON(w, http.StatusOK, south)
			return
		}
		if id == "trip-north" {
			httpx.WriteJSON(w, http.StatusOK, north)
			return
		}
		http.NotFound(w, req)
	})
	r.Get("/api/v1/shared/outlets/{id}", func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"outlet": map[string]any{
			"id": chi.URLParam(req, "id"), "name": "Outlet", "district": "Colombo", "dockType": "normal",
			"parkingConstraint": "normal", "windowOpenTime": "08:00:00", "windowCloseTime": "18:00:00",
			"accessInstructions": "Use the red gate beside the pharmacy; final 200 m is a narrow lane.", "accessInstructionsUpdatedAt": "2026-09-28T10:00:00Z",
			"chilledTemperatureMinC": func() any {
				if chi.URLParam(req, "id") == "OUT021" {
					return nil
				}
				return -5.0
			}(), "chilledTemperatureMaxC": func() any {
				if chi.URLParam(req, "id") == "OUT021" {
					return nil
				}
				return 5.0
			}(),
		}})
	})
	r.Post("/api/v1/shared/audit-events", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "ingested"})
	})
    r.Post("/api/v1/orders/internal/delivery-followups", func(w http.ResponseWriter, _ *http.Request) {
        httpx.WriteJSON(w, http.StatusOK, map[string]any{"order": map[string]any{
            "id": "followup-ord-5", "orderRef": "ORD000006", "requestedDeliveryDate": "2026-09-30",
        }})
    })
    r.Post("/api/v1/shared/internal/notifications/enqueue", func(w http.ResponseWriter, _ *http.Request) {
        httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"notification": map[string]any{"status": "enqueued"}})
    })
	return r
}

func uploadProof(t *testing.T, srv *httptest.Server, tripID, stopID, subject, key, typ string) resp {
	t.Helper()
	return uploadProofWithReceiver(t, srv, tripID, stopID, subject, key, typ, "")
}

func uploadProofWithReceiver(t *testing.T, srv *httptest.Server, tripID, stopID, subject, key, typ, receiverName string) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("type", typ)
	_ = w.WriteField("mimeType", "image/png")
	if receiverName != "" {
		_ = w.WriteField("receiverName", receiverName)
	}
	fw, err := w.CreateFormFile("file", "proof.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(png1)
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/delivery/trips/"+tripID+"/stops/"+stopID+"/proofs", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+subject)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Idempotency-Key", key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{status: res.StatusCode, body: string(b)}
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
