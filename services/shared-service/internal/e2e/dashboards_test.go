package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

const receiptsDashboard = `{"spec":{"name":"Receipts and shortages","cards":["short","deadlines"]}}`

func TestSavedDashboardsAreOwnerScopedVersionedAndAudited(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second)}
	pg, err := startAuditPostgres(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Skipf("testcontainers postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432")
	pool, err := db.Open(ctx, "postgres://waypoint:waypoint@"+host+":"+port.Port()+"/waypoint?sslmode=disable", "shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, `CREATE SCHEMA audit;
	CREATE TABLE audit.events (event_id text primary key, correlation_id text, actor_id text, actor_type text, action text not null, resource_type text, resource_id text, previous_state jsonb, new_state jsonb, reason text, timestamp timestamptz not null default now(), source text);
	CREATE SCHEMA shared;
	CREATE TABLE shared.users(id text primary key, identity_subject text unique, display_name text not null default '', role text);
	CREATE TABLE shared.store_manager_profiles(user_id text, outlet_id text);
	CREATE TABLE shared.loader_profiles(user_id text, depot text);
	CREATE TABLE shared.driver_profiles(user_id text, vehicle_id text);
	INSERT INTO shared.users(id,identity_subject,role) VALUES ('u-store','store-test','STORE_MANAGER'),('u-store-b','store-b-test','STORE_MANAGER'),('u-driver','driver-test','DRIVER');
	INSERT INTO shared.store_manager_profiles(user_id,outlet_id) VALUES('u-store','OUT001'),('u-store-b','OUT002')`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../../database/migrations/0040_shared_user_dashboards.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply dashboard migration: %v", err)
	}
	router := chi.NewRouter()
	handler.Handler{Authn: auditAuth{}, Store: store.Store{Pool: pool}}.Routes(router)
	call := func(subject, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+subject)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}

	created := call("store-test", http.MethodPost, "/api/v1/shared/dashboards", receiptsDashboard)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var saved struct {
		Dashboard store.Dashboard `json:"dashboard"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &saved)
	id := saved.Dashboard.ID
	if id == "" || saved.Dashboard.Version != 1 || saved.Dashboard.Spec.Cards[1] != "deadlines" || saved.Dashboard.Spec.Filter != "all" {
		t.Fatalf("unexpected saved dashboard: %+v", saved.Dashboard)
	}

	if res := call("store-test", http.MethodPost, "/api/v1/shared/dashboards", `{"spec":{"name":"x","cards":["sql"]}}`); res.Code != http.StatusBadRequest {
		t.Fatalf("unknown card type status=%d", res.Code)
	}
	if res := call("store-test", http.MethodPost, "/api/v1/shared/dashboards", `{"spec":{"name":"x","cards":["receipts"]},"ownerUserId":"u-store-b"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("unknown fields must be rejected, status=%d", res.Code)
	}
	if res := call("driver-test", http.MethodGet, "/api/v1/shared/dashboards", ""); res.Code != http.StatusForbidden {
		t.Fatalf("driver status=%d", res.Code)
	}

	other := call("store-b-test", http.MethodGet, "/api/v1/shared/dashboards", "")
	if other.Code != http.StatusOK || bytes.Contains(other.Body.Bytes(), []byte(id)) {
		t.Fatalf("another store manager must not see the dashboard: %d %s", other.Code, other.Body.String())
	}
	if res := call("store-b-test", http.MethodPut, "/api/v1/shared/dashboards/"+id, `{"version":1,"spec":{"name":"stolen","cards":["receipts"]}}`); res.Code != http.StatusNotFound {
		t.Fatalf("cross-owner update status=%d", res.Code)
	}
	if res := call("store-b-test", http.MethodDelete, "/api/v1/shared/dashboards/"+id, ""); res.Code != http.StatusNotFound {
		t.Fatalf("cross-owner delete status=%d", res.Code)
	}

	reordered := `{"version":1,"spec":{"name":"Receipts and shortages","cards":["deadlines","short"],"filter":"chilled"}}`
	if res := call("store-test", http.MethodPut, "/api/v1/shared/dashboards/"+id, reordered); res.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", res.Code, res.Body.String())
	}
	if res := call("store-test", http.MethodPut, "/api/v1/shared/dashboards/"+id, reordered); res.Code != http.StatusConflict {
		t.Fatalf("stale version status=%d", res.Code)
	}
	list := call("store-test", http.MethodGet, "/api/v1/shared/dashboards", "")
	if !bytes.Contains(list.Body.Bytes(), []byte(`"version":2`)) || !bytes.Contains(list.Body.Bytes(), []byte(`"cards":["deadlines","short"]`)) {
		t.Fatalf("list after update: %s", list.Body.String())
	}
	if res := call("store-test", http.MethodDelete, "/api/v1/shared/dashboards/"+id, ""); res.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", res.Code)
	}

	var actions []string
	rows, err := pool.Query(ctx, `SELECT action FROM audit.events WHERE resource_type='USER_DASHBOARD' AND actor_id='u-store' ORDER BY timestamp`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		actions = append(actions, a)
	}
	rows.Close()
	if len(actions) != 3 || actions[0] != "USER_DASHBOARD_CREATED" || actions[1] != "USER_DASHBOARD_UPDATED" || actions[2] != "USER_DASHBOARD_DELETED" {
		t.Fatalf("audit trail=%v", actions)
	}
}
