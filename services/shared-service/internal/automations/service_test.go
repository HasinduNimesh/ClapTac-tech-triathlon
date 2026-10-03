package automations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/automation"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type testAuth struct{}

func (testAuth) Authenticate(r *http.Request) (*auth.Principal, error) {
	if r.Header.Get("Authorization") == "" {
		return nil, fmt.Errorf("no token")
	}
	return &auth.Principal{Subject: r.Header.Get("Authorization")}, nil
}

func TestDurableHabitsAndWorkflowLifecycle(t *testing.T) {
	ctx := context.Background()
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2)}, Started: true})
	if err != nil {
		t.Fatal("PostgreSQL integration test requires Docker: ", err)
	}
	t.Cleanup(func() { pg.Terminate(ctx) })
	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432/tcp")
	pool, err := db.Open(ctx, "postgres://waypoint:waypoint@"+host+":"+port.Port()+"/waypoint?sslmode=disable", "public")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrations, err := filepath.Glob("../../../../database/migrations/*.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatal("migrations missing")
	}
	for _, path := range migrations {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatalf("%s: %v", path, e)
		}
	}
	_, err = pool.Exec(ctx, `INSERT INTO shared.users(id,identity_subject,role) VALUES('store','store','STORE_MANAGER'),('other','other','STORE_MANAGER'),('dispatch','dispatch','DISPATCHER'); INSERT INTO shared.outlets(id,brand,name,district,depot,dock_type,parking_constraint,mall_window) VALUES('OUT001','Fresh','One','Colombo','North','normal','normal',false),('OUT002','Fresh','Two','Colombo','North','normal','normal',false)`)
	if err != nil {
		t.Fatal(err)
	}
	profiles := authorization.StaticProfileResolver{Profiles: map[string]authorization.Profile{"store": {UserID: "store", Subject: "store", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT001"}}, "other": {UserID: "other", Subject: "other", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT002"}}, "dispatch": {UserID: "dispatch", Subject: "dispatch", Roles: []string{"DISPATCHER"}}}}
	p, _ := profiles.Resolve(ctx, "store")
	now := time.Now().UTC()
	s := Service{Pool: pool, Profiles: profiles, Now: func() time.Time { return now }, Snapshot: func(ctx context.Context, at time.Time) (automation.Snapshot, error) {
		return automation.Snapshot{AsOf: at, HistorySince: now.AddDate(0, 0, -28), Items: []automation.Deferred{{OrderID: "own", OutletID: "OUT001", Date: "2026-10-02", Count: 2}, {OrderID: "secret", OutletID: "OUT002", Date: "2026-10-02", Count: 3}}}, nil
	}}
	pre := automation.Prefill{OutletID: "OUT001", Units: 30, Weight: 150, Volume: .42, Temperature: "ambient"}
	for i := 1; i <= 3; i++ {
		_, err = pool.Exec(ctx, `INSERT INTO audit.events(event_id,actor_id,action,new_state,timestamp,source) VALUES($1,'store','order.created',$2,$3,'order-service')`, fmt.Sprint(i), marshal(pre), now.AddDate(0, 0, -7*i))
		if err != nil {
			t.Fatal(err)
		}
	}
	habits, err := s.habits(ctx, p)
	if err != nil || len(habits) != 1 {
		t.Fatalf("habits %+v %v", habits, err)
	}
	// Concurrent duplicate Yes: only one counts, including after reconstruction.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.respond(ctx, p, habits[0].ID, "yes") }()
	}
	wg.Wait()
	var count int
	if err = pool.QueryRow(ctx, `SELECT yes_count FROM shared.habit_feedback WHERE owner_id='store'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate answer counted %d %v", count, err)
	}
	other, _ := profiles.Resolve(ctx, "other")
	if _, err = s.respond(ctx, other, habits[0].ID, "yes"); err == nil {
		t.Fatal("cross owner response accepted")
	}
	// Simulate separate occurrences to exercise persistent suppression.
	for i := 0; i < 2; i++ {
		var id string
		err = pool.QueryRow(ctx, `INSERT INTO shared.habit_suggestions(owner_id,pattern_key,occurrence_key,payload) VALUES('store',$1,$2,$3) RETURNING id::text`, habits[0].Pattern, fmt.Sprint("future", i), marshal(habits[0])).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.respond(ctx, p, id, "no"); err != nil {
			t.Fatal(err)
		}
	}
	habits, err = s.habits(ctx, p)
	if err != nil || len(habits) != 0 {
		t.Fatal("two No responses must suppress")
	}
	// Three genuine accepted occurrences offer promotion, while other patterns stay suppressed.
	promotion := Habit{Pattern: "promotion-test", Action: "prefill_order", Prefill: &pre}
	for i := 0; i < 3; i++ {
		var id string
		err = pool.QueryRow(ctx, `INSERT INTO shared.habit_suggestions(owner_id,pattern_key,occurrence_key,payload) VALUES('store',$1,$2,$3) RETURNING id::text`, promotion.Pattern, fmt.Sprint(i), marshal(promotion)).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		accepted, e := s.respond(ctx, p, id, "yes")
		if e != nil || accepted.YesCount != i+1 {
			t.Fatalf("promotion counter %+v %v", accepted, e)
		}
	}
	// Dispatcher habit is based on their manual review history, not other users' clicks.
	dp, _ := profiles.Resolve(ctx, "dispatch")
	for i := 1; i <= 3; i++ {
		_, err = pool.Exec(ctx, `INSERT INTO audit.events(event_id,actor_id,action,timestamp,source) VALUES($1,'dispatch','PRIORITY_REVIEW_MARKED',$2,'shared-service')`, fmt.Sprint("priority", i), now.AddDate(0, 0, -i))
		if err != nil {
			t.Fatal(err)
		}
	}
	dispatchHabits, e := s.habits(ctx, dp)
	if e != nil || len(dispatchHabits) != 2 {
		t.Fatalf("dispatcher habits %+v %v", dispatchHabits, e)
	}
	if _, e = s.respond(ctx, dp, dispatchHabits[0].ID, "yes"); e != nil {
		t.Fatal(e)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shared.priority_reviews WHERE owner_id='dispatch'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("priority flag missing")
	}
	router := chi.NewRouter()
	Handler{Service: s, Authn: testAuth{}}.Routes(router)
	request := func(method, path, actor string, body any) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/shared/automations"+path, bytes.NewReader(marshal(body)))
		req.Header.Set("Authorization", actor)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	d := automation.Definition{Version: 1, Name: "My weekly list", Weekday: 5, Time: "15:00", Timezone: "Asia/Colombo", Action: "notify_deferrals"}
	w := request("POST", "/preview", "store", map[string]any{"definition": d, "at": "previous"})
	if w.Code != 200 {
		t.Fatalf("preview %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal("preview leaks another outlet")
	}
	var preview struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &preview)
	w = request("POST", "/", "other", map[string]any{"previewId": preview.ID})
	if w.Code != 409 {
		t.Fatal("cross owner preview activated")
	}
	w = request("POST", "/", "store", map[string]any{"previewId": preview.ID})
	if w.Code != 201 {
		t.Fatalf("activate %d %s", w.Code, w.Body.String())
	}
	var saved struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &saved)
	w = request("POST", "/", "store", map[string]any{"previewId": preview.ID})
	if w.Code != 201 {
		t.Fatal("idempotent activation failed")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shared.automations`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate activation")
	}
	_, err = pool.Exec(ctx, `UPDATE shared.automations SET next_run_at=$1`, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.RunDue(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shared.automation_inbox`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("notification duplication count=%d %v", count, err)
	}
	w = request("GET", "/inbox", "store", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("inbox %s", w.Body.String())
	}
	w = request("GET", "/inbox", "other", nil)
	if strings.Contains(w.Body.String(), "own") {
		t.Fatal("inbox owner leak")
	}
	w = request("POST", "/"+saved.ID+"/status", "other", map[string]any{"status": "deleted"})
	if w.Code != 404 {
		t.Fatal("cross owner delete")
	}
	w = request("POST", "/"+saved.ID+"/status", "store", map[string]any{"status": "paused"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_, _ = pool.Exec(ctx, `UPDATE shared.automations SET next_run_at=$1`, now.Add(-time.Minute))
	if err = s.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shared.automation_inbox`).Scan(&count); err != nil || count != 1 {
		t.Fatal("paused automation ran")
	}
	w = request("POST", "/"+saved.ID+"/status", "store", map[string]any{"status": "deleted"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = request("GET", "/", "store", nil)
	if strings.Contains(w.Body.String(), saved.ID) {
		t.Fatal("deleted automation listed")
	}
	d.Action = "submit_order"
	w = request("POST", "/preview", "store", map[string]any{"definition": d, "at": "now"})
	if w.Code != 400 {
		t.Fatal("unsafe action accepted")
	}
	// Provider outages never turn into empty successful previews.
	broken := s
	broken.Snapshot = func(context.Context, time.Time) (automation.Snapshot, error) {
		return automation.Snapshot{}, fmt.Errorf("unavailable")
	}
	d.Action = "notify_deferrals"
	if _, e := broken.evaluate(ctx, p, d, now); e == nil {
		t.Fatal("provider outage hidden")
	}
	unauthorized := automation.Definition{Version: 1, Name: "Wrong outlet", Weekday: 1, Time: "09:00", Timezone: "Asia/Colombo", Action: "prefill_order", Prefill: &automation.Prefill{OutletID: "OUT002", Units: 1, Weight: 1, Volume: 1, Temperature: "ambient"}}
	if validate(unauthorized, p) == nil {
		t.Fatal("cross-outlet prefill")
	}

}
