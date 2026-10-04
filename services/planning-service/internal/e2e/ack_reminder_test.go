package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	planclient "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	planhandler "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/handler"
	planservice "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/service"
	planstore "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/store"
)

type recordedCall struct {
	path, authorization, body string
}

// ackPeers records the audit events and the driver trip messages planning emits.
type ackPeers struct {
	mu          sync.Mutex
	audits      []map[string]any
	messages    []recordedCall
	messageCode atomic.Int32
}

func (a *ackPeers) handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/fleet/vehicles/{id}", func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"vehicle": map[string]any{"id": chi.URLParam(req, "id"), "homeDepot": "DEPOT_NORTH"}})
	})
	r.Post("/api/v1/shared/audit-events", func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)
		a.mu.Lock()
		a.audits = append(a.audits, body)
		a.mu.Unlock()
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "ingested"})
	})
	r.Post("/api/v1/delivery/trips/{tripId}/messages", func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		a.mu.Lock()
		a.messages = append(a.messages, recordedCall{path: req.URL.Path, authorization: req.Header.Get("Authorization"), body: string(b)})
		a.mu.Unlock()
		code := int(a.messageCode.Load())
		if code == 0 {
			code = http.StatusCreated
		}
		httpx.WriteJSON(w, code, map[string]any{"message": map[string]any{"id": "m1"}})
	})
	return r
}

func (a *ackPeers) auditsFor(action string) []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []map[string]any
	for _, e := range a.audits {
		if e["action"] == action {
			out = append(out, e)
		}
	}
	return out
}

func (a *ackPeers) messageCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.messages)
}

func startAckDB(t *testing.T) (context.Context, string, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env:          map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"},
		WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}
	pg, err := startPostgresContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
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
	pool, err := db.Open(ctx, dsn, "planning")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, dsn, pool
}

func doAs(t *testing.T, method, url, subject, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+subject)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return res.StatusCode, readBody(res)
}

func findAck(acks []domain.PlanAcknowledgement, role, tripID string) *domain.PlanAcknowledgement {
	for i := range acks {
		if acks[i].ActorRole == role && acks[i].TripID == tripID {
			return &acks[i]
		}
	}
	return nil
}

// TestPlanAcknowledgementIdentityAndReminders proves the dispatcher tracker's
// inputs are accurate: a driver or loader acknowledgement covers one trip only,
// legacy trip-less receipts survive the migration without covering any trip,
// and a reminder is a persisted, audited, de-duplicated server action.
func TestPlanAcknowledgementIdentityAndReminders(t *testing.T) {
	ctx, dsn, pool := startAckDB(t)
	root := repoRoot(t)
	for _, m := range []string{"0001_init.sql", "0007_planning.sql", "0013_planning_stop_times.sql", "0020_planning_publications.sql"} {
		applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", m))
	}

	var planID, tripA, tripB string
	if err := pool.QueryRow(ctx, `INSERT INTO planning.plans(plan_ref,delivery_date,status,created_by,current_version,published_at) VALUES('PLAN-ACK','2026-10-05','confirmed','USR002',1,now()) RETURNING id::text`).Scan(&planID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO planning.plan_publications(plan_id,version,content_hash,published_by) VALUES($1::uuid,1,'hash','USR002')`, planID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO planning.trips(plan_id,vehicle_id,trip_number) VALUES($1::uuid,'VEH-A',1) RETURNING id::text`, planID).Scan(&tripA); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO planning.trips(plan_id,vehicle_id,trip_number) VALUES($1::uuid,'VEH-B',1) RETURNING id::text`, planID).Scan(&tripB); err != nil {
		t.Fatal(err)
	}
	var otherPlan, otherTrip string
	if err := pool.QueryRow(ctx, `INSERT INTO planning.plans(plan_ref,delivery_date,status,created_by) VALUES('PLAN-OTHER','2026-10-06','draft','USR002') RETURNING id::text`).Scan(&otherPlan); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO planning.trips(plan_id,vehicle_id,trip_number) VALUES($1::uuid,'VEH-A',1) RETURNING id::text`, otherPlan).Scan(&otherTrip); err != nil {
		t.Fatal(err)
	}
	// A receipt written by the pre-0073 code: no trip, primary key (plan, version, actor, role).
	if _, err := pool.Exec(ctx, `INSERT INTO planning.plan_acknowledgements(plan_id,version,actor_id,actor_role) VALUES($1::uuid,1,'USR099','DRIVER')`, planID); err != nil {
		t.Fatal(err)
	}
	mig := filepath.Join(root, "database", "migrations", "0073_planning_ack_trip_identity.sql")
	applySQL(t, ctx, dsn, mig)
	applySQL(t, ctx, dsn, mig) // idempotent

	peers := &ackPeers{}
	stub := httptest.NewServer(peers.handler())
	t.Cleanup(stub.Close)
	repo := planstore.Postgres{Pool: pool}
	client := planclient.Peers{OrdersURL: stub.URL, FleetURL: stub.URL, SharedURL: stub.URL, DeliveryURL: stub.URL, M2M: staticToken("svc-planning")}
	r := chi.NewRouter()
	planhandler.Handler{Authn: bearerAuth{}, Profiles: staticProfiles{}, Service: planservice.Service{Repo: repo, Peers: client}}.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	ackURL := srv.URL + "/api/v1/planning/plans/" + planID + "/acknowledgements"
	remindURL := srv.URL + "/api/v1/planning/plans/" + planID + "/reminders"

	pub := func() domain.Publication {
		p, err := repo.Publication(ctx, planID)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	// The legacy row survived the migration and covers no trip.
	if legacy := findAck(pub().Acknowledgements, "DRIVER", ""); legacy == nil || legacy.ActorID != "USR099" {
		t.Fatalf("legacy receipt lost or given a trip by the migration: %+v", pub().Acknowledgements)
	}

	// Driver A acknowledges trip A; that must not touch trip B.
	if code, body := doAs(t, http.MethodPost, ackURL, "usr-driver-a", `{"version":1,"tripId":"`+tripA+`"}`); code != http.StatusOK {
		t.Fatalf("driver A ack trip A: %d %s", code, body)
	}
	// Idempotent per recipient and trip.
	if code, body := doAs(t, http.MethodPost, ackURL, "usr-driver-a", `{"version":1,"tripId":"`+tripA+`"}`); code != http.StatusOK {
		t.Fatalf("repeat ack: %d %s", code, body)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM planning.plan_acknowledgements WHERE actor_id='USR010' AND trip_id=$1::uuid`, tripA).Scan(&n); err != nil || n != 1 {
		t.Fatalf("repeat ack created %d rows (err=%v)", n, err)
	}
	a := findAck(pub().Acknowledgements, "DRIVER", tripA)
	if a == nil || a.ActorID != "USR010" || a.VehicleID != "VEH-A" {
		t.Fatalf("driver A receipt should carry trip A and its vehicle: %+v", pub().Acknowledgements)
	}
	if findAck(pub().Acknowledgements, "DRIVER", tripB) != nil {
		t.Fatal("driver A's acknowledgement of trip A marked trip B acknowledged")
	}
	// A driver cannot acknowledge another vehicle's trip, nor can a different driver cover trip A.
	if code, _ := doAs(t, http.MethodPost, ackURL, "usr-driver-a", `{"version":1,"tripId":"`+tripB+`"}`); code != http.StatusForbidden {
		t.Fatalf("driver A acknowledging trip B must be forbidden, got %d", code)
	}
	if code, _ := doAs(t, http.MethodPost, ackURL, "usr-driver-b", `{"version":1,"tripId":"`+tripA+`"}`); code != http.StatusForbidden {
		t.Fatalf("driver B acknowledging trip A must be forbidden, got %d", code)
	}
	// Trips of another plan are rejected.
	if code, _ := doAs(t, http.MethodPost, ackURL, "usr-driver-a", `{"version":1,"tripId":"`+otherTrip+`"}`); code != http.StatusNotFound {
		t.Fatalf("a trip of another plan must be rejected, got %d", code)
	}
	// Loaders are tracked per load list and depot-scoped.
	if code, body := doAs(t, http.MethodPost, ackURL, "usr-loader-north", `{"version":1,"tripId":"`+tripA+`"}`); code != http.StatusOK {
		t.Fatalf("loader ack trip A: %d %s", code, body)
	}
	if code, _ := doAs(t, http.MethodPost, ackURL, "usr-loader-south", `{"version":1,"tripId":"`+tripA+`"}`); code != http.StatusForbidden {
		t.Fatalf("loader of another depot must be forbidden, got %d", code)
	}
	if findAck(pub().Acknowledgements, "LOADER", tripA) == nil || findAck(pub().Acknowledgements, "LOADER", tripB) != nil {
		t.Fatalf("loader receipt must cover trip A only: %+v", pub().Acknowledgements)
	}
	// Legacy callers that send no trip still work and stay plan-level.
	if code, body := doAs(t, http.MethodPost, ackURL, "usr-loader-north", `{"version":1}`); code != http.StatusOK {
		t.Fatalf("legacy loader ack: %d %s", code, body)
	}
	if legacy := findAck(pub().Acknowledgements, "LOADER", ""); legacy == nil || findAck(pub().Acknowledgements, "LOADER", tripB) != nil {
		t.Fatalf("trip-less receipt must stay plan-level: %+v", pub().Acknowledgements)
	}

	// Reminders: permission, validation, persistence, audit, delivery, idempotency.
	for _, who := range []string{"usr-driver-a", "usr-loader-north", "usr-store-manager"} {
		if code, _ := doAs(t, http.MethodPost, remindURL, who, `{"tripId":"`+tripB+`","audience":"DRIVER"}`); code != http.StatusForbidden {
			t.Fatalf("%s must not send reminders, got %d", who, code)
		}
	}
	if code, _ := doAs(t, http.MethodPost, remindURL, "usr-dispatcher", `{"tripId":"`+tripB+`","audience":"CUSTOMER"}`); code != http.StatusBadRequest {
		t.Fatalf("bad audience must be rejected, got %d", code)
	}
	if code, _ := doAs(t, http.MethodPost, remindURL, "usr-dispatcher", `{"tripId":"`+otherTrip+`","audience":"DRIVER"}`); code != http.StatusNotFound {
		t.Fatalf("trip of another plan must be rejected, got %d", code)
	}
	if code, body := doAs(t, http.MethodPost, remindURL, "usr-dispatcher", `{"tripId":"`+tripA+`","audience":"DRIVER"}`); code != http.StatusConflict {
		t.Fatalf("reminding a driver who already acknowledged must conflict, got %d %s", code, body)
	}

	code, body := doAs(t, http.MethodPost, remindURL, "usr-dispatcher", `{"tripId":"`+tripB+`","audience":"DRIVER"}`)
	var first domain.ReminderResult
	if code != http.StatusOK || json.Unmarshal([]byte(body), &first) != nil || first.AlreadySent || first.TripMessage != "sent" || first.Reminder.TripID != tripB || first.Reminder.Audience != "DRIVER" || first.Reminder.Version != 1 || first.Reminder.RemindedBy != "USR002" {
		t.Fatalf("first reminder: %d %s", code, body)
	}
	if peers.messageCount() != 1 || peers.messages[0].path != "/api/v1/delivery/trips/"+tripB+"/messages" || peers.messages[0].authorization != "Bearer usr-dispatcher" || !strings.Contains(peers.messages[0].body, "acknowledge") {
		t.Fatalf("driver trip message not delivered as the dispatcher: %+v", peers.messages)
	}
	audits := peers.auditsFor("PLAN_ACK_REMINDER_SENT")
	if len(audits) != 1 {
		t.Fatalf("expected one audit event, got %d", len(audits))
	}
	state, _ := audits[0]["newState"].(map[string]any)
	if audits[0]["actorId"] != "USR002" || state["tripId"] != tripB || state["audience"] != "DRIVER" || state["version"] != float64(1) {
		t.Fatalf("audit event lacks trip/audience/version/actor: %+v", audits[0])
	}
	// Persisted: it comes back from the plan read model (survives reload).
	var persisted *domain.PlanReminder
	reminders := pub().Reminders
	for i, rem := range reminders {
		if rem.TripID == tripB && rem.Audience == "DRIVER" {
			persisted = &reminders[i]
		}
	}
	if persisted == nil || persisted.RemindedAt.IsZero() || !persisted.RemindedAt.Equal(first.Reminder.RemindedAt) {
		t.Fatalf("reminder not persisted: %+v", reminders)
	}
	// A repeat inside the window neither records, audits nor re-messages.
	code, body = doAs(t, http.MethodPost, remindURL, "usr-dispatcher", `{"tripId":"`+tripB+`","audience":"DRIVER"}`)
	var second domain.ReminderResult
	if code != http.StatusOK || json.Unmarshal([]byte(body), &second) != nil || !second.AlreadySent || !second.Reminder.RemindedAt.Equal(first.Reminder.RemindedAt) {
		t.Fatalf("repeat reminder should be idempotent: %d %s", code, body)
	}
	if peers.messageCount() != 1 || len(peers.auditsFor("PLAN_ACK_REMINDER_SENT")) != 1 {
		t.Fatalf("repeat reminder re-sent: messages=%d audits=%d", peers.messageCount(), len(peers.auditsFor("PLAN_ACK_REMINDER_SENT")))
	}
	// Audiences are independent.
	if code, body := doAs(t, http.MethodPost, remindURL, "usr-dispatcher", `{"tripId":"`+tripB+`","audience":"LOADER"}`); code != http.StatusOK || strings.Contains(body, `"alreadySent":true`) {
		t.Fatalf("loader reminder for the same trip must be separate: %d %s", code, body)
	}
	if peers.messageCount() != 1 {
		t.Fatalf("loader reminder must not post a driver trip message: %+v", peers.messages)
	}
	// After the window a new reminder is recorded; a failed message channel is reported, not hidden.
	if _, err := pool.Exec(ctx, `UPDATE planning.plan_ack_reminders SET reminded_at = now() - interval '6 minutes' WHERE trip_id=$1::uuid AND audience='DRIVER'`, tripB); err != nil {
		t.Fatal(err)
	}
	peers.messageCode.Store(http.StatusNotFound)
	code, body = doAs(t, http.MethodPost, remindURL, "usr-dispatcher", `{"tripId":"`+tripB+`","audience":"DRIVER"}`)
	var third domain.ReminderResult
	if code != http.StatusOK || json.Unmarshal([]byte(body), &third) != nil || third.AlreadySent || third.TripMessage != "unavailable" || !third.Reminder.RemindedAt.After(first.Reminder.RemindedAt) {
		t.Fatalf("reminder after the window: %d %s", code, body)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM planning.plan_ack_reminders WHERE trip_id=$1::uuid AND audience='DRIVER'`, tripB).Scan(&n); err != nil || n != 2 {
		t.Fatalf("expected 2 driver reminders for trip B, got %d (err=%v)", n, err)
	}

	// The recipient apps read only the reminders addressed to their role and trip.
	readReminders := func(subject, trip string) []domain.PlanReminder {
		code, body := doAs(t, http.MethodGet, remindURL+"?tripId="+trip, subject, "")
		if code != http.StatusOK {
			t.Fatalf("%s reminders: %d %s", subject, code, body)
		}
		var out struct {
			Items []domain.PlanReminder `json:"items"`
		}
		_ = json.Unmarshal([]byte(body), &out)
		return out.Items
	}
	if got := readReminders("usr-driver-b", tripB); len(got) != 1 || got[0].Audience != "DRIVER" {
		t.Fatalf("driver B reminders: %+v", got)
	}
	if got := readReminders("usr-loader-north", tripB); len(got) != 1 || got[0].Audience != "LOADER" {
		t.Fatalf("loader reminders: %+v", got)
	}
	if got := readReminders("usr-driver-b", tripA); len(got) != 0 {
		t.Fatalf("no reminder was sent for trip A: %+v", got)
	}
	if code, _ := doAs(t, http.MethodGet, remindURL, "usr-dispatcher", ""); code != http.StatusForbidden {
		t.Fatalf("dispatcher must not use the field reminder read, got %d", code)
	}

	// An unpublished plan cannot be reminded about.
	if _, err := pool.Exec(ctx, `UPDATE planning.plans SET status='draft' WHERE id=$1::uuid`, planID); err != nil {
		t.Fatal(err)
	}
	if code, _ := doAs(t, http.MethodPost, remindURL, "usr-dispatcher", `{"tripId":"`+tripB+`","audience":"LOADER"}`); code != http.StatusConflict {
		t.Fatalf("draft plan reminder must conflict, got %d", code)
	}
}
