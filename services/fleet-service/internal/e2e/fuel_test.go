package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/store"
)

type fuelAuth struct{}

func (fuelAuth) Authenticate(r *http.Request) (*auth.Principal, error) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return nil, fmt.Errorf("missing token")
	}
	subject := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	p := &auth.Principal{Subject: subject}
	if subject == "svc-planning" {
		p.Scopes = []string{authorization.PermFleetReadInternal}
	}
	return p, nil
}

type fuelProfiles struct{}

func (fuelProfiles) Resolve(_ context.Context, subject string) (*authorization.Profile, error) {
	if subject == "usr-dispatcher" {
		return &authorization.Profile{UserID: "USR002", Subject: subject, Roles: []string{authorization.RoleDispatcher}}, nil
	}
	if subject == "usr-driver" {
		return &authorization.Profile{UserID: "USR006", Subject: subject, Roles: []string{authorization.RoleDriver}}, nil
	}
	return &authorization.Profile{Subject: subject}, nil
}

func TestFuelLedgerIdempotencyRBACAndOverQuotaReconciliation(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"},
		Env:        map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"},
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
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0006_fleet.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0015_fuel_ledger.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0019_fleet_master_data.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0023_vehicle_incidents.sql"))

	pool, err := db.Open(ctx, dsn, "fleet")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `INSERT INTO vehicles (vehicle_id,type,temp,weight_cap_kg,volume_cap_m3,km_per_l,weekly_fuel_quota_l,home_depot) VALUES ('VEH001','truck','ambient',1000,10,8,100,'DEPOT_NORTH')`)
	if err != nil {
		t.Fatal(err)
	}
	master, err := (store.Store{Pool: pool}).UpdateMasterData(ctx, domain.Vehicle{ID: "VEH001", Type: "van", Temp: "reefer", WeightCapacityKg: 900, VolumeCapacityM3: 9, FuelType: "diesel", KmPerL: 9, WeeklyFuelQuotaL: 110, HomeDepot: "DEPOT_SOUTH"}, 1, "USR002")
	if err != nil || master.Version != 2 || master.Type != "van" {
		t.Fatalf("master data update=%+v err=%v", master, err)
	}
	if _, err = (store.Store{Pool: pool}).UpdateMasterData(ctx, master, 1, "USR002"); err == nil {
		t.Fatal("stale vehicle version should conflict")
	}
	repo1 := store.Store{Pool: pool}
	outbox, err := repo1.PendingAudit(ctx, 10)
	if err != nil || len(outbox) != 1 || !strings.Contains(string(outbox[0].Payload), "MASTER_DATA_VEHICLE_UPDATED") {
		t.Fatalf("vehicle audit outbox=%+v err=%v", outbox, err)
	}
	pool2, err := db.Open(ctx, dsn, "fleet")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool2.Close)
	repo2 := store.Store{Pool: pool2}
	type claimResult struct {
		items []domain.AuditOutbox
		err   error
	}
	startClaims := make(chan struct{})
	results := make(chan claimResult, 2)
	var claimers sync.WaitGroup
	for _, repo := range []store.Store{repo1, repo2} {
		claimers.Add(1)
		go func(repo store.Store) {
			defer claimers.Done()
			<-startClaims
			items, claimErr := repo.ClaimPendingAudit(ctx, 10)
			results <- claimResult{items: items, err: claimErr}
		}(repo)
	}
	close(startClaims)
	claimers.Wait()
	close(results)
	var claimed []domain.AuditOutbox
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		claimed = append(claimed, result.items...)
	}
	if len(claimed) != 1 || claimed[0].EventID != outbox[0].EventID {
		t.Fatalf("two Fleet replicas must claim one event only once: %+v", claimed)
	}
	if err = repo1.MarkAudit(ctx, claimed[0].EventID, "shared audit service temporarily unavailable"); err != nil {
		t.Fatal(err)
	}
	pendingCount, oldestSeconds, healthErr := repo1.AuditOutboxHealth(ctx)
	if healthErr != nil || pendingCount != 1 || oldestSeconds < 0 {
		t.Fatalf("audit outbox health pending=%d oldest=%f err=%v", pendingCount, oldestSeconds, healthErr)
	}
	if pending, err := repo1.PendingAudit(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("failed audit should be backed off: pending=%+v err=%v", pending, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_outbox SET next_attempt_at=now()-interval '1 second' WHERE event_id=$1`, claimed[0].EventID); err != nil {
		t.Fatal(err)
	}
	recovered, err := repo1.ClaimPendingAudit(ctx, 10)
	if err != nil || len(recovered) != 1 {
		t.Fatalf("eligible failed event was not reclaimed: events=%+v err=%v", recovered, err)
	}
	if duplicate, err := repo2.ClaimPendingAudit(ctx, 10); err != nil || len(duplicate) != 0 {
		t.Fatalf("leased event was claimed by another replica: events=%+v err=%v", duplicate, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_outbox SET next_attempt_at=now()-interval '1 second' WHERE event_id=$1`, recovered[0].EventID); err != nil {
		t.Fatal(err)
	}
	recoveredAfterLease, err := repo2.ClaimPendingAudit(ctx, 10)
	if err != nil || len(recoveredAfterLease) != 1 {
		t.Fatalf("expired crash lease did not recover event: events=%+v err=%v", recoveredAfterLease, err)
	}
	if err := repo2.MarkAudit(ctx, recoveredAfterLease[0].EventID, ""); err != nil {
		t.Fatal(err)
	}
	if pending, err := repo1.PendingAudit(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("successfully published event should be complete: pending=%+v err=%v", pending, err)
	}

	h := handler.Handler{Authn: fuelAuth{}, Profiles: fuelProfiles{}, Store: store.Store{Pool: pool}}
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	vehicleBody := `{"type":"truck","temp":"reefer","weightCapacityKg":850,"volumeCapacityM3":8.5,"fuelType":"diesel","kmPerL":9,"weeklyFuelQuotaL":100,"homeDepot":"DEPOT_NORTH","version":2}`
	updateVehicle := func(subject string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/fleet/vehicles/VEH001/master-data", strings.NewReader(vehicleBody))
		req.Header.Set("Authorization", "Bearer "+subject)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	if res := updateVehicle("usr-driver"); res.Code != http.StatusForbidden {
		t.Fatalf("driver vehicle edit returned %d", res.Code)
	}
	if res := updateVehicle("usr-dispatcher"); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"version":3`) {
		t.Fatalf("dispatcher vehicle edit returned %d %s", res.Code, res.Body.String())
	}
	if res := updateVehicle("usr-dispatcher"); res.Code != http.StatusConflict {
		t.Fatalf("stale dispatcher vehicle edit returned %d %s", res.Code, res.Body.String())
	}
	today := time.Now().In(time.FixedZone("Asia/Colombo", 5*60*60+30*60)).Format(time.DateOnly)
	incidentBody, _ := json.Marshal(map[string]any{"vehicleId": "VEH001", "date": today, "tripId": "trip-1", "type": "breakdown", "description": "Engine failure", "affectedStops": []string{"OUT034", "OUT021"}})
	incidentReq := httptest.NewRequest(http.MethodPost, "/api/v1/fleet/incidents", bytes.NewReader(incidentBody))
	incidentReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	incidentRec := httptest.NewRecorder()
	r.ServeHTTP(incidentRec, incidentReq)
	if incidentRec.Code != http.StatusCreated || !strings.Contains(incidentRec.Body.String(), `"vehicleStatus":"unavailable"`) {
		t.Fatalf("incident record failed %d %s", incidentRec.Code, incidentRec.Body.String())
	}
	var unavailable bool
	if err := pool.QueryRow(ctx, `SELECT status='unavailable' FROM fleet.vehicle_availability WHERE vehicle_id='VEH001' AND date=$1`, today).Scan(&unavailable); err != nil || !unavailable {
		t.Fatalf("breakdown availability not blocked: %v %v", unavailable, err)
	}
	var incidentOutbox int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet.audit_outbox WHERE event_id LIKE 'fleet-incident-%'`).Scan(&incidentOutbox); err != nil || incidentOutbox != 1 {
		t.Fatalf("incident audit was not queued atomically: %d %v", incidentOutbox, err)
	}
	incidentList := httptest.NewRecorder()
	incidentListReq := httptest.NewRequest(http.MethodGet, "/api/v1/fleet/incidents?openOnly=true", nil)
	incidentListReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	r.ServeHTTP(incidentList, incidentListReq)
	if incidentList.Code != http.StatusOK || !strings.Contains(incidentList.Body.String(), `"tripId":"trip-1"`) {
		t.Fatalf("open incident list failed %d %s", incidentList.Code, incidentList.Body.String())
	}
	post := func(subject, key string, liters float64, receipt string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"vehicleId": "VEH001", "date": today, "liters": liters, "receiptRef": receipt})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/fleet/fuel/entries", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+subject)
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("usr-driver", "driver-denied", 5, "r-driver"); rec.Code != http.StatusForbidden {
		t.Fatalf("driver fuel write returned %d", rec.Code)
	}
	first := post("usr-dispatcher", "fuel-entry-1", 90, "receipt-1")
	if first.Code != http.StatusOK {
		t.Fatalf("fuel entry %d %s", first.Code, first.Body.String())
	}
	var created struct {
		Entry domain.FuelEntry `json:"entry"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil || created.Entry.ID == "" {
		t.Fatalf("fuel entry response: %+v err=%v", created, err)
	}
	replay := post("usr-dispatcher", "fuel-entry-1", 90, "receipt-1")
	var replayed struct {
		Entry domain.FuelEntry `json:"entry"`
	}
	_ = json.Unmarshal(replay.Body.Bytes(), &replayed)
	if replay.Code != http.StatusOK || replayed.Entry.ID != created.Entry.ID {
		t.Fatalf("idempotent replay changed the ledger: %d %+v", replay.Code, replayed)
	}
	if conflict := post("usr-dispatcher", "fuel-entry-1", 89, "receipt-1"); conflict.Code != http.StatusConflict {
		t.Fatalf("reused key with changed liters returned %d", conflict.Code)
	}
	if second := post("usr-dispatcher", "fuel-entry-2", 25, "receipt-2"); second.Code != http.StatusOK {
		t.Fatalf("over-quota actual entry must still be retained, got %d %s", second.Code, second.Body.String())
	}

	ledgerReq := httptest.NewRequest(http.MethodGet, "/api/v1/fleet/fuel/ledger?weekOf="+today, nil)
	ledgerReq.Header.Set("Authorization", "Bearer svc-planning")
	ledgerRec := httptest.NewRecorder()
	r.ServeHTTP(ledgerRec, ledgerReq)
	if ledgerRec.Code != http.StatusOK {
		t.Fatalf("planner ledger read %d %s", ledgerRec.Code, ledgerRec.Body.String())
	}
	var ledger domain.FuelLedger
	if err := json.Unmarshal(ledgerRec.Body.Bytes(), &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.WeekStart == "" || ledger.WeekEnd == "" || len(ledger.Entries) != 2 || len(ledger.Items) != 1 || ledger.Items[0].ActualLitersL != 115 || ledger.Items[0].ActualLitersL <= ledger.Items[0].WeeklyQuotaL {
		t.Fatalf("weekly ledger should reconcile actual over-quota use: %+v", ledger)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM fuel_entries`).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fuel_entries SET note='changed' WHERE id=$1`, created.Entry.ID); err == nil {
		t.Fatal("fuel ledger mutation trigger allowed an update")
	}

	future := time.Now().In(time.FixedZone("Asia/Colombo", 5*60*60+30*60)).AddDate(0, 0, 1).Format(time.DateOnly)
	futureBody, _ := json.Marshal(map[string]any{"vehicleId": "VEH001", "date": future, "liters": 1, "receiptRef": "future"})
	futureReq := httptest.NewRequest(http.MethodPost, "/api/v1/fleet/fuel/entries", bytes.NewReader(futureBody))
	futureReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	futureReq.Header.Set("Idempotency-Key", "future-entry")
	futureRec := httptest.NewRecorder()
	r.ServeHTTP(futureRec, futureReq)
	if futureRec.Code != http.StatusBadRequest {
		t.Fatalf("future fuel entry returned %d", futureRec.Code)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	path, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			t.Fatal("repo root not found")
		}
		path = parent
	}
}

func applySQL(t *testing.T, ctx context.Context, dsn, filename string) {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(contents)); err != nil {
		t.Fatal(err)
	}
}
