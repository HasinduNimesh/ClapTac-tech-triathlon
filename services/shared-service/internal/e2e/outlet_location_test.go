package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

// The dispatcher can record where an outlet really is, and drivers and maps then get that exact
// position instead of the approximate district one.
func TestDispatcherRecordsAnExactOutletLocation(t *testing.T) {
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
	_, err = pool.Exec(ctx, `CREATE SCHEMA audit;
	CREATE TABLE audit.events (event_id text primary key, correlation_id text, actor_id text, actor_type text, action text not null, resource_type text, resource_id text, previous_state jsonb, new_state jsonb, reason text, timestamp timestamptz not null default now(), source text);
	CREATE SCHEMA shared;
	CREATE TABLE shared.users(id text primary key, identity_subject text unique, display_name text not null default '', role text);
	CREATE TABLE shared.store_manager_profiles(user_id text, outlet_id text);
	CREATE TABLE shared.loader_profiles(user_id text, depot text);
	CREATE TABLE shared.dispatcher_profiles(user_id text, depot text);
	CREATE TABLE shared.driver_profiles(user_id text, vehicle_id text);
	CREATE TABLE shared.outlets(id text primary key,brand text,name text,district text,depot text,dock_type text,parking_constraint text,mall_window boolean,window_open_time time,window_close_time time,access_instructions text not null default '',access_instructions_updated_by text not null default '',access_instructions_updated_at timestamptz,access_instructions_confirmed_by text not null default '',access_instructions_confirmed_at timestamptz,chilled_temperature_min_c numeric(5,2),chilled_temperature_max_c numeric(5,2),version integer not null default 1);
	INSERT INTO shared.outlets(id,brand,name,district,depot,dock_type,parking_constraint,mall_window) VALUES ('OUT001','Fresh','Test Outlet','Colombo','DEPOT_NORTH','normal','normal',false);
	INSERT INTO shared.users(id,identity_subject,role) VALUES ('u-dispatcher','dispatcher-test','DISPATCHER'),('u-driver','driver-test','DRIVER');`)
	if err != nil {
		t.Fatal(err)
	}
	// The real migration, so the test uses the real columns and the district table.
	migration, err := readFile("../../../../database/migrations/0043_outlet_locations.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migration); err != nil {
		t.Fatalf("apply location migration: %v", err)
	}

	router := chi.NewRouter()
	handler.Handler{Authn: auditAuth{}, Store: store.Store{Pool: pool}}.Routes(router)
	send := func(method, subject, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+subject)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, r)
		return res
	}
	type outletResponse struct {
		Outlet struct {
			Version             int      `json:"version"`
			Latitude            *float64 `json:"latitude"`
			Longitude           *float64 `json:"longitude"`
			LocationApproximate bool     `json:"locationApproximate"`
		} `json:"outlet"`
	}
	read := func() outletResponse {
		res := send(http.MethodGet, "dispatcher-test", "/api/v1/shared/outlets/OUT001", "")
		var out outletResponse
		if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil {
			t.Fatalf("read outlet: %d %s", res.Code, res.Body.String())
		}
		return out
	}
	body := func(version int, extra string) string {
		return `{"name":"Test Outlet","brand":"Fresh","district":"Colombo","depot":"DEPOT_NORTH","dockType":"normal","parkingConstraint":"normal","mallWindow":false,"version":` + itoa(version) + extra + `}`
	}
	put := func(version int, extra string) *httptest.ResponseRecorder {
		return send(http.MethodPut, "dispatcher-test", "/api/v1/shared/outlets/OUT001", body(version, extra))
	}

	start := read()
	if !start.Outlet.LocationApproximate || start.Outlet.Latitude == nil {
		t.Fatalf("an outlet without a recorded position starts approximate: %+v", start)
	}
	approxLat, approxLng := *start.Outlet.Latitude, *start.Outlet.Longitude

	// A client that saves other fields and echoes back the approximate position it read must not
	// turn the approximation into a "verified" position.
	echo := `,"latitude":` + ftoa(approxLat) + `,"longitude":` + ftoa(approxLng) + `,"locationApproximate":true`
	if res := put(1, echo); res.Code != http.StatusOK {
		t.Fatalf("save with echoed approximate position: %d %s", res.Code, res.Body.String())
	}
	if after := read(); !after.Outlet.LocationApproximate {
		t.Fatalf("echoing the approximate position must leave it approximate: %+v", after)
	}

	// Recording the real position.
	res := put(2, `,"location":{"latitude":6.93441,"longitude":79.84281}`)
	var saved outletResponse
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &saved) != nil {
		t.Fatalf("record location: %d %s", res.Code, res.Body.String())
	}
	if saved.Outlet.LocationApproximate || saved.Outlet.Latitude == nil || *saved.Outlet.Latitude != 6.93441 || *saved.Outlet.Longitude != 79.84281 || saved.Outlet.Version != 3 {
		t.Fatalf("the save's response must carry the exact position and the new version: %+v", saved)
	}
	if got := read(); got.Outlet.LocationApproximate || *got.Outlet.Latitude != 6.93441 {
		t.Fatalf("reads show the exact position: %+v", got)
	}

	// Saving other fields later, without a location, keeps the recorded position.
	if res := put(3, ""); res.Code != http.StatusOK {
		t.Fatalf("save without location: %d %s", res.Code, res.Body.String())
	}
	if got := read(); got.Outlet.LocationApproximate || *got.Outlet.Latitude != 6.93441 {
		t.Fatalf("a save that says nothing about the location must keep it: %+v", got)
	}

	// Positions that cannot be in Sri Lanka, half a pair and a non-driver are refused, changing nothing.
	for name, extra := range map[string]string{
		"swapped":       `,"location":{"latitude":79.84,"longitude":6.93}`,
		"abroad":        `,"location":{"latitude":51.5,"longitude":-0.12}`,
		"only latitude": `,"location":{"latitude":6.9}`,
		"extra field":   `,"location":{"latitude":6.9,"longitude":79.8,"accuracy":5}`,
		"text":          `,"location":"Colombo"`,
	} {
		if res := put(4, extra); res.Code != http.StatusBadRequest {
			t.Fatalf("%s must be refused with 400, got %d %s", name, res.Code, res.Body.String())
		}
	}
	if res := send(http.MethodPut, "driver-test", "/api/v1/shared/outlets/OUT001", body(4, `,"location":{"latitude":6.9,"longitude":79.8}`)); res.Code != http.StatusForbidden {
		t.Fatalf("a driver cannot change an outlet's location: %d", res.Code)
	}
	if got := read(); got.Outlet.Version != 4 || *got.Outlet.Latitude != 6.93441 {
		t.Fatalf("refused saves must change nothing: %+v", got)
	}

	// Clearing goes back to the approximation.
	if res := put(4, `,"location":null`); res.Code != http.StatusOK {
		t.Fatalf("clear location: %d %s", res.Code, res.Body.String())
	}
	if got := read(); !got.Outlet.LocationApproximate || *got.Outlet.Latitude != approxLat {
		t.Fatalf("clearing returns to the approximate position: %+v", got)
	}

	// The audit trail says what the position was and became.
	rows, err := pool.Query(ctx, `SELECT previous_state->'exactLocation', new_state->'exactLocation' FROM audit.events WHERE action='MASTER_DATA_OUTLET_UPDATED' ORDER BY timestamp, event_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var trail []string
	for rows.Next() {
		var before, after *string
		if err := rows.Scan(&before, &after); err != nil {
			t.Fatal(err)
		}
		trail = append(trail, str(before)+"->"+str(after))
	}
	joined := strings.Join(trail, " | ")
	if !strings.Contains(joined, `null->{"latitude": 6.93441, "longitude": 79.84281}`) || !strings.Contains(joined, `{"latitude": 6.93441, "longitude": 79.84281}->null`) {
		t.Fatalf("audit history must record the exact position set and cleared: %s", joined)
	}
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func str(s *string) string {
	if s == nil {
		return "null"
	}
	return *s
}
