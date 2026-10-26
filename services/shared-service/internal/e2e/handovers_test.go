package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestShiftHandoverScopePersistenceRetriesAcknowledgementAndAudit(t *testing.T) {
	ctx := context.Background()
	pg, err := startAuditPostgres(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second)}, Started: true})
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
	_, err = pool.Exec(ctx, `CREATE SCHEMA shared; CREATE SCHEMA audit;
 CREATE TABLE audit.events(event_id text primary key,correlation_id text,actor_id text,actor_type text,action text not null,resource_type text,resource_id text,previous_state jsonb,new_state jsonb,reason text,timestamp timestamptz not null default now(),source text);
 CREATE TABLE shared.users(id text primary key,identity_subject text unique,display_name text not null default '',role text);
 CREATE TABLE shared.store_manager_profiles(user_id text,outlet_id text);
 CREATE TABLE shared.loader_profiles(user_id text,depot text); CREATE TABLE shared.driver_profiles(user_id text,vehicle_id text); CREATE TABLE shared.dispatcher_profiles(user_id text,depot text);
 INSERT INTO shared.users(id,identity_subject,display_name,role) VALUES ('s1','s1','Outgoing manager','STORE_MANAGER'),('s2','s2','Incoming manager','STORE_MANAGER'),('s3','s3','Second incoming manager','STORE_MANAGER'),('other','other','Other store','STORE_MANAGER'),('d1','d1','Outgoing dispatch','DISPATCHER'),('d2','d2','Incoming dispatch','DISPATCHER'),('driver','driver','Driver','DRIVER');
 INSERT INTO shared.store_manager_profiles VALUES('s1','OUT001'),('s2','OUT001'),('s3','OUT001'),('other','OUT002');`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../../database/migrations/0081_shift_handovers.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	handler.Handler{Authn: auditAuth{}, Store: store.Store{Pool: pool}}.Routes(router)
	call := func(actor, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+actor)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	scope := "?role=store&scope=OUT001"
	base := "/api/v1/shared/handovers"
	payload := func(id string, at time.Time) string {
		raw, _ := json.Marshal(map[string]any{"id": id, "snapshot": map[string]any{"title": "Evening shift", "notes": "Check receiving space with the team.", "asOf": at, "tasks": []any{map[string]any{"id": "receipt:1", "priority": "High", "title": "Confirm receipt", "reason": "Receipt overdue", "reference": "ORD-001", "action": "Review", "to": "/store-manager/receipts?order=1"}}}})
		return string(raw)
	}
	id := "00000000-0000-4000-8000-000000000001"
	body := payload(id, time.Now().UTC())
	// Concurrent retries publish one immutable record and one audit event.
	var wg sync.WaitGroup
	codes := make(chan int, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- call("s1", "POST", base+scope, body).Code }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 201 {
			t.Fatalf("publish retry status %d", code)
		}
	}
	for _, tc := range []struct{ actor, path string }{{"other", base + scope}, {"s1", base + "?role=store&scope=OUT002"}, {"s1", base + "?role=dispatcher&scope=fleet"}, {"driver", base + scope}, {"d1", base + scope}} {
		if w := call(tc.actor, "GET", tc.path, ""); w.Code != 403 {
			t.Fatalf("unauthorized list %s: %d", tc.actor, w.Code)
		}
	}
	list := call("s2", "GET", base+scope, "")
	if list.Code != 200 || !bytes.Contains(list.Body.Bytes(), []byte("Outgoing manager")) || list.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list %d %s", list.Code, list.Body)
	}
	if w := call("other", "GET", base+"?role=store&scope=OUT002", ""); w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(id)) {
		t.Fatal("cross-outlet leak")
	}
	if w := call("s1", "POST", base+"/"+id+"/acknowledge"+scope, ""); w.Code != 409 {
		t.Fatalf("self ack %d", w.Code)
	}
	if w := call("other", "POST", base+"/"+id+"/acknowledge?role=store&scope=OUT002", ""); w.Code != 404 {
		t.Fatalf("cross-scope ack %d", w.Code)
	}
	codes = make(chan int, 2)
	for _, actor := range []string{"s2", "s3"} {
		wg.Add(1)
		go func(actor string) {
			defer wg.Done()
			codes <- call(actor, "POST", base+"/"+id+"/acknowledge"+scope, "").Code
		}(actor)
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("ack race %v", counts)
	}
	var winner string
	if err = pool.QueryRow(ctx, `SELECT acknowledged_by FROM shared.shift_handovers WHERE id=$1`, id).Scan(&winner); err != nil {
		t.Fatal(err)
	}
	if w := call(winner, "POST", base+"/"+id+"/acknowledge"+scope, ""); w.Code != 200 {
		t.Fatal("ack retry not idempotent")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit.events WHERE resource_id=$1`, id).Scan(&count); err != nil || count != 2 {
		t.Fatalf("audit count %d %v", count, err)
	}
	if w := call("s1", "POST", base+scope, payload(id, time.Now().Add(time.Second))); w.Code != 409 {
		t.Fatalf("changed retry %d", w.Code)
	}
	if w := call("s1", "POST", base+scope, payload("00000000-0000-4000-8000-000000000002", time.Now().Add(-6*time.Minute))); w.Code != 409 {
		t.Fatalf("stale snapshot %d", w.Code)
	}
	if w := call("s1", "POST", base+scope, body+` {}`); w.Code != 400 {
		t.Fatalf("trailing JSON %d", w.Code)
	}
	if w := call("s1", "POST", base+scope, `{"authorId":"d1"}`); w.Code != 400 {
		t.Fatalf("actor injection %d", w.Code)
	}
	if w := call("s1", "POST", base+"?role=store&scope=OUT002", body); w.Code != 403 {
		t.Fatalf("cross-outlet publish %d", w.Code)
	}
	// Dispatcher handovers are fleet-wide and never mixed into store history.
	dispatchID := "00000000-0000-4000-8000-000000000003"
	dispatchBody := fmt.Sprintf(`{"id":%q,"snapshot":{"title":"Fleet handover","notes":"","asOf":%q,"tasks":[]}}`, dispatchID, time.Now().UTC().Format(time.RFC3339Nano))
	if w := call("d1", "POST", base+"?role=dispatcher&scope=fleet", dispatchBody); w.Code != 201 {
		t.Fatalf("dispatch publish %d %s", w.Code, w.Body)
	}
	if w := call("d2", "POST", base+"/"+dispatchID+"/acknowledge?role=dispatcher&scope=fleet", ""); w.Code != 200 {
		t.Fatalf("dispatch ack %d", w.Code)
	}
	if w := call("s1", "GET", base+scope, ""); bytes.Contains(w.Body.Bytes(), []byte(dispatchID)) {
		t.Fatal("cross-role leak")
	}
	// Audit failure must roll back publication, rather than leave an unaudited report.
	if _, err = pool.Exec(ctx, `ALTER TABLE audit.events RENAME TO saved_events`); err != nil {
		t.Fatal(err)
	}
	rollbackID := "00000000-0000-4000-8000-000000000004"
	if w := call("s1", "POST", base+scope, payload(rollbackID, time.Now())); w.Code != 500 {
		t.Fatalf("audit failure %d", w.Code)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shared.shift_handovers WHERE id=$1`, rollbackID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("publication rollback %d %v", count, err)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE audit.saved_events RENAME TO events`); err != nil {
		t.Fatal(err)
	}

	// A failed acknowledgement audit must not record a receiver.
	if w := call("s1", "POST", base+scope, payload(rollbackID, time.Now())); w.Code != 201 {
		t.Fatalf("retry rolled-back publication %d", w.Code)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE audit.events RENAME TO saved_events`); err != nil {
		t.Fatal(err)
	}
	if w := call("s2", "POST", base+"/"+rollbackID+"/acknowledge"+scope, ""); w.Code != 500 {
		t.Fatalf("ack audit failure %d", w.Code)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shared.shift_handovers WHERE id=$1 AND acknowledged_by IS NOT NULL`, rollbackID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("ack rollback %d %v", count, err)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE audit.saved_events RENAME TO events`); err != nil {
		t.Fatal(err)
	}
	if w := call("s2", "POST", base+"/"+rollbackID+"/acknowledge"+scope, ""); w.Code != 200 {
		t.Fatalf("ack retry %d", w.Code)
	}
	// Simulate an already committed report aging beyond the freshness window.
	old := time.Now().UTC().Add(-time.Hour)
	oldBody := payload(rollbackID, old)
	var aged struct {
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if err = json.Unmarshal([]byte(oldBody), &aged); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE shared.shift_handovers SET snapshot=$2 WHERE id=$1`, rollbackID, []byte(aged.Snapshot)); err != nil {
		t.Fatal(err)
	}
	if w := call("s1", "POST", base+scope, oldBody); w.Code != 201 {
		t.Fatalf("aged committed retry %d %s", w.Code, w.Body)
	}
	// History is bounded and the next page contains the remaining records.
	if _, err = pool.Exec(ctx, `INSERT INTO shared.shift_handovers(id,role,scope,author_id,author_name,snapshot) SELECT gen_random_uuid(),role,scope,author_id,author_name,snapshot FROM shared.shift_handovers CROSS JOIN generate_series(1,21) WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items   []json.RawMessage `json:"items"`
		HasMore bool              `json:"hasMore"`
	}
	if w := call("s1", "GET", base+scope, ""); json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 20 || !page.HasMore {
		t.Fatalf("first history page %s", w.Body)
	}
	if w := call("s1", "GET", base+scope+"&offset=20", ""); json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 3 || page.HasMore {
		t.Fatalf("second history page %s", w.Body)
	}
}
