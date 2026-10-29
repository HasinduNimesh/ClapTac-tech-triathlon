package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	planclient "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	planhandler "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/recovery"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/schedule"
	planservice "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/service"
	planstore "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecoveryWorkflow(t *testing.T) {
	ctx := context.Background()
	pg, err := startPostgresContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2)}, Started: true})
	if err != nil {
		t.Skipf("Postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432")
	dsn := "postgres://waypoint:waypoint@" + host + ":" + port.Port() + "/waypoint?sslmode=disable"
	for _, m := range []string{"0001_init.sql", "0007_planning.sql", "0013_planning_stop_times.sql", "0020_planning_publications.sql", "0037_planning_deferral_next_run.sql", "0079_disruption_recovery.sql"} {
		applySQL(t, ctx, dsn, filepath.Join(repoRoot(t), "database", "migrations", m))
	}
	pool, err := db.Open(ctx, dsn, "planning")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ready := time.Now().In(schedule.Location()).Add(30 * time.Minute)
	date := ready.Format("2006-01-02")
	// Store windows include the test's current time; examples assert fixed times separately.
	var mu sync.Mutex
	available := true
	progressChanged := false
	suspendCalls := 0
	workshopFail := false
	peers := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		path := r.URL.Path
		switch {
		case path == "/api/v1/fleet/incidents":
			httpx.WriteJSON(w, 200, map[string]any{"items": []recovery.Incident{{ID: "incident", VehicleID: "BROKEN", Date: date, Type: "breakdown", Status: "open"}}})
		case path == "/api/v1/fleet/vehicles":
			httpx.WriteJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "BROKEN", "homeDepot": "DEPOT_NORTH", "type": "van", "temp": "reefer", "weightCapacityKg": 1000, "volumeCapacityM3": 20, "kmPerL": 8, "weeklyFuelQuotaL": 200}, {"id": "RESCUE", "homeDepot": "DEPOT_NORTH", "type": "van", "temp": "reefer", "weightCapacityKg": 150, "volumeCapacityM3": 20, "kmPerL": 8, "weeklyFuelQuotaL": 200}}})
		case path == "/api/v1/fleet/availability":
			status := "available"
			if !available {
				status = "unavailable"
			}
			httpx.WriteJSON(w, 200, map[string]any{"items": []map[string]string{{"vehicleId": "RESCUE", "status": status}}})
		case strings.HasSuffix(path, "/availability") && r.Method == "PUT":
			if workshopFail {
				http.Error(w, "unavailable", 503)
				return
			}
			httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
		case path == "/api/v1/orders":
			httpx.WriteJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "order-a", "orderRef": "ORD-A", "outletId": "OUT034", "temperatureRequirement": "chilled", "orderWeightKg": 100, "orderVolumeM3": 1}, {"id": "order-b", "orderRef": "ORD-B", "outletId": "OTHER", "temperatureRequirement": "ambient", "orderWeightKg": 100, "orderVolumeM3": 1}}})
		case path == "/api/v1/shared/outlets":
			httpx.WriteJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "OUT034", "brand": "Fresh", "name": "Own store", "district": "Colombo", "depot": "DEPOT_NORTH", "windowOpenTime": "00:00", "windowCloseTime": "23:59"}, {"id": "OTHER", "brand": "Style", "name": "Private store", "district": "Colombo", "depot": "DEPOT_NORTH", "windowOpenTime": "00:00", "windowCloseTime": "23:59"}}})
		case path == "/api/v1/shared/policies/current":
			httpx.WriteJSON(w, 200, map[string]any{"policy": domain.PlanningPolicy{Version: 1, MaxTripsPerVehicle: 2}})
		case strings.HasPrefix(path, "/api/v1/delivery/internal/orders/"):
			if progressChanged && strings.HasSuffix(path, "order-a") {
				httpx.WriteJSON(w, 200, map[string]any{"delivery": map[string]string{"outcome": "DELIVERED"}})
			} else {
				http.NotFound(w, r)
			}
		case path == "/api/v1/delivery/internal/recovery/suspend":
			suspendCalls++
			httpx.WriteJSON(w, 200, map[string]bool{"suspended": true})
		default:
			httpx.WriteJSON(w, 200, map[string]any{"items": []any{}})
		}
	}))
	t.Cleanup(peers.Close)
	svc := planservice.Service{Repo: planstore.Postgres{Pool: pool}, Peers: planclient.Peers{OrdersURL: peers.URL, FleetURL: peers.URL, SharedURL: peers.URL, DeliveryURL: peers.URL, M2M: staticToken("service")}}
	dispatch := &authorization.Profile{UserID: "dispatcher", Roles: []string{authorization.RoleDispatcher}}
	store := &authorization.Profile{UserID: "manager", Roles: []string{authorization.RoleStoreManager}, OutletIDs: []string{"OUT034"}}
	var planID, tripID string
	if err = pool.QueryRow(ctx, `INSERT INTO plans(plan_ref,delivery_date,status,created_by,current_version) VALUES('RECOVERY-TEST',$1::date,'confirmed','dispatcher',1) RETURNING id::text`, date).Scan(&planID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO trips(plan_id,vehicle_id,trip_number) VALUES($1::uuid,'BROKEN',1) RETURNING id::text`, planID).Scan(&tripID); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"order-a", "order-b"} {
		if _, err = pool.Exec(ctx, `INSERT INTO allocations(plan_id,trip_id,vehicle_id,order_id,sequence) VALUES($1::uuid,$2::uuid,'BROKEN',$3,$4)`, planID, tripID, id, i+1); err != nil {
			t.Fatal(err)
		}
	}
	create := func() recovery.Case {
		t.Helper()
		c, err := svc.CreateRecovery(ctx, dispatch, planID, "incident", ready)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c := create()
	if len(c.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	if err = svc.RecoveryPreference(ctx, store, c.ID, "OTHER", c.Candidates[0].ID, "unsafe"); err == nil {
		t.Fatal("cross-store preference accepted")
	}
	if err = svc.RecoveryPreference(ctx, store, c.ID, "OUT034", c.Candidates[0].ID, "Please protect chilled stock"); err != nil {
		t.Fatal(err)
	}
	views, err := svc.RecoveryCases(ctx, store, date)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(views)
	if strings.Contains(string(b), "order-b") || strings.Contains(string(b), "Private store") {
		t.Fatalf("store data leak: %s", b)
	}
	if _, err = svc.ApproveRecovery(ctx, store, c.ID, c.Candidates[0].ID, "approved", true); err == nil {
		t.Fatal("store approved fleet plan")
	}
	mu.Lock()
	available = false
	mu.Unlock()
	if _, err = svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Crew confirmed", true); err == nil {
		t.Fatal("changed fleet accepted")
	}
	mu.Lock()
	available = true
	progressChanged = true
	mu.Unlock()
	if _, err = svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Crew confirmed", true); err == nil {
		t.Fatal("completed delivery accepted")
	}
	mu.Lock()
	progressChanged = false
	mu.Unlock()
	// A changed allocation is stale even before a publication version changes.
	_, err = pool.Exec(ctx, `UPDATE allocations SET sequence=sequence+10 WHERE plan_id=$1::uuid`, planID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Crew confirmed", true); err == nil {
		t.Fatal("changed allocations accepted")
	}
	_, _ = pool.Exec(ctx, `UPDATE allocations SET sequence=sequence-10 WHERE plan_id=$1::uuid`, planID)
	// Server-side expiry is authoritative, regardless of the browser clock.
	_, err = pool.Exec(ctx, `UPDATE planning.recovery_cases SET payload=jsonb_set(payload,'{expiresAt}',to_jsonb('2000-01-01T00:00:00Z'::text)) WHERE id=$1::uuid`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Crew confirmed", true); err == nil {
		t.Fatal("expired options accepted")
	}
	old := c
	c = create()
	if err = svc.RecoveryPreference(ctx, store, old.ID, "OUT034", old.Candidates[0].ID, "old"); err == nil {
		t.Fatal("superseded preference accepted")
	}
	mu.Lock()
	workshopFail = true
	mu.Unlock()
	if _, err = svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Crew confirmed", true); err == nil || !strings.HasPrefix(err.Error(), "workshop_pending") {
		t.Fatalf("expected retryable workshop failure: %v", err)
	}
	mu.Lock()
	workshopFail = false
	if suspendCalls != 0 {
		t.Fatal("suspended after workshop failure")
	}
	mu.Unlock()
	// Fail inside publication after assignments/deferrals have been written.
	_, err = pool.Exec(ctx, `CREATE FUNCTION planning.reject_test_publication() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test publication failure'; END $$; CREATE TRIGGER reject_test_publication BEFORE INSERT ON planning.plan_publications FOR EACH ROW EXECUTE FUNCTION planning.reject_test_publication()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Crew confirmed", true); err == nil {
		t.Fatal("publication failure not surfaced")
	}
	var count int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE vehicle_id='BROKEN'`).Scan(&count)
	if count != 2 {
		t.Fatal("partial assignments escaped rollback")
	}
	_, _ = pool.Exec(ctx, `DROP TRIGGER reject_test_publication ON planning.plan_publications`)
	// Both callers can start with the same pending case. One publication wins;
	// the other either observes it or receives a stale snapshot conflict.
	var concurrent sync.WaitGroup
	for i := 0; i < 2; i++ {
		concurrent.Add(1)
		go func() {
			defer concurrent.Done()
			_, e := svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Crew and depot handover confirmed", true)
			if e != nil && !strings.HasPrefix(e.Error(), "conflict:") {
				t.Errorf("concurrent approval: %v", e)
			}
		}()
	}
	concurrent.Wait()
	result, err := svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Crew and depot handover confirmed", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "approved" || result.PublishedVersion != 2 {
		t.Fatalf("missing publication: %+v", result)
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			again, e := svc.ApproveRecovery(ctx, dispatch, c.ID, c.Candidates[0].ID, "Safe retry", true)
			if e != nil || again.PublishedVersion != 2 {
				t.Errorf("idempotent retry: %+v %v", again, e)
			}
		}()
	}
	wg.Wait()
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM planning.plan_publications`).Scan(&count)
	if count != 1 {
		t.Fatalf("duplicate publication: %d", count)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM planning.deferrals`).Scan(&count)
	if count != c.Candidates[0].Deferred {
		t.Fatal("deferrals not committed")
	}
	// Exercise the actual authenticated list endpoint, including server time.
	r := chi.NewRouter()
	planhandler.Handler{Authn: bearerAuth{}, Profiles: staticProfiles{}, Service: svc}.Routes(r)
	req := httptest.NewRequest("GET", "/api/v1/planning/recovery?date="+date, nil)
	req.Header.Set("Authorization", "Bearer usr-store-manager")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "serverNow") || strings.Contains(rec.Body.String(), "order-b") {
		t.Fatalf("list API: %d %s", rec.Code, rec.Body.String())
	}
	t.Log(fmt.Sprintf("Published v%d: %d recovered, %d deferred; rollback, roles, preferences, stale fleet/progress and retry checks passed", result.PublishedVersion, c.Candidates[0].Recovered, c.Candidates[0].Deferred))
}
