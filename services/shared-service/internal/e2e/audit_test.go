package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

type auditAuth struct{}

func (auditAuth) Authenticate(r *http.Request) (*auth.Principal, error) {
	return &auth.Principal{Subject: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")}, nil
}

func TestAuditSearchKPIsAndAppendOnly(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second)}
	pg, err := startAuditPostgres(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Skipf("testcontainers postgres unavailable: %v", err)
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
	pool, err := db.Open(ctx, "postgres://waypoint:waypoint@"+host+":"+port.Port()+"/waypoint?sslmode=disable", "shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `CREATE SCHEMA audit;
	CREATE TABLE audit.events (event_id text primary key, correlation_id text, actor_id text, actor_type text, action text not null, resource_type text, resource_id text, previous_state jsonb, new_state jsonb, reason text, timestamp timestamptz not null default now(), source text);
	CREATE OR REPLACE FUNCTION audit.reject_event_mutation() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'audit events are append-only'; END; $$ LANGUAGE plpgsql;
	CREATE TRIGGER audit_events_append_only BEFORE UPDATE OR DELETE ON audit.events FOR EACH ROW EXECUTE FUNCTION audit.reject_event_mutation();`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE SCHEMA shared;
	CREATE TABLE shared.users(id text primary key, identity_subject text unique, display_name text not null default '', role text);
	CREATE TABLE shared.store_manager_profiles(user_id text, outlet_id text);
	CREATE TABLE shared.loader_profiles(user_id text, depot text);
	CREATE TABLE shared.dispatcher_profiles(user_id text, depot text);
	CREATE TABLE shared.driver_profiles(user_id text, vehicle_id text);
	CREATE TABLE shared.outlets(id text primary key,brand text,name text,district text,depot text,dock_type text,parking_constraint text,mall_window boolean,window_open_time time,window_close_time time,access_instructions text not null default '',access_instructions_updated_by text not null default '',access_instructions_updated_at timestamptz,access_instructions_confirmed_by text not null default '',access_instructions_confirmed_at timestamptz,chilled_temperature_min_c numeric(5,2),chilled_temperature_max_c numeric(5,2),version integer not null default 1);
	CREATE TABLE shared.operating_calendar(date date primary key,is_operating boolean,version integer not null default 1);
	CREATE TABLE shared.planning_policy_versions(version integer primary key,cutoff_local_time time not null,deferral_weight_points integer not null,max_deferral_count integer not null,max_unserved_days integer not null,max_trips_per_vehicle integer not null,created_by text not null,created_at timestamptz not null default now());
	CREATE TABLE shared.planning_policy_current(singleton boolean primary key,version integer references shared.planning_policy_versions(version));
	INSERT INTO shared.planning_policy_versions(version,cutoff_local_time,deferral_weight_points,max_deferral_count,max_unserved_days,max_trips_per_vehicle,created_by) VALUES(1,'16:00',14,12,365,2,'system');
	INSERT INTO shared.planning_policy_current(singleton,version) VALUES(true,1);
	INSERT INTO shared.outlets(id,brand,name,district,depot,dock_type,parking_constraint,mall_window,window_open_time,window_close_time) VALUES
	('OUT001','Fresh','Test Outlet','Colombo','DEPOT_NORTH','normal','normal',false,'08:00','18:00'),
	('OUT002','Style','Other Outlet','Colombo','DEPOT_NORTH','normal','normal',false,'08:00','18:00');
	INSERT INTO shared.users(id,identity_subject,role) VALUES ('u-dispatcher','dispatcher-test','DISPATCHER'),('u-driver','driver-test','DRIVER'),('u-store','store-test','STORE_MANAGER');
	INSERT INTO shared.store_manager_profiles(user_id,outlet_id) VALUES('u-store','OUT001')`)
	if err != nil {
		t.Fatal(err)
	}
	// Outlet updates now read the outlet back with its position, so the test schema needs the real
	// location columns and district table.
	locationMigration, err := os.ReadFile("../../../../database/migrations/0043_outlet_locations.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(locationMigration)); err != nil {
		t.Fatalf("apply location migration: %v", err)
	}
	migration, err := os.ReadFile("../../../../database/migrations/0026_shared_notifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply notification migration: %v", err)
	}

	// Applied in migration-number order: 0056 and 0059 redefine the event-type check that 0070 widens.
	migration, err = os.ReadFile("../../../../database/migrations/0056_shared_arrival_notifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply arrival notification migration: %v", err)
	}
	migration, err = os.ReadFile("../../../../database/migrations/0059_shared_rejected_delivery.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply rejected delivery migration: %v", err)
	}

	shortfallMigration, err := os.ReadFile("../../../../database/migrations/0070_notification_load_shortfall.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(shortfallMigration)); err != nil {
		t.Fatalf("apply shortfall notification migration: %v", err)
	}
	inAppMigration, err := os.ReadFile("../../../../database/migrations/0072_notification_in_app_only.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(inAppMigration)); err != nil {
		t.Fatalf("apply in-app notification migration: %v", err)
	}
	if _, err = pool.Exec(ctx, string(inAppMigration)); err != nil {
		t.Fatalf("in-app notification migration must be safe to run twice: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	_, err = pool.Exec(ctx, `INSERT INTO audit.events(event_id,actor_id,action,resource_type,resource_id,new_state,timestamp,source) VALUES
	('e1','USR001','PLAN_GENERATED','PLAN','P-01','{"allocated":3}'::jsonb,$1,'planning-service'),
	('e2','USR002','ORDER_DEFERRED','ORDER','O-09','{"reasonCode":"WINDOW"}'::jsonb,$1 + interval '1 hour','planning-service'),
	('e3','USR001','DELIVERY_RUN_COMPLETED','TRIP','T-07','{}'::jsonb,$1 + interval '2 hour','delivery-service')`, now)
	if err != nil {
		t.Fatal(err)
	}
	s := store.Store{Pool: pool}
	preferences, err := s.SaveNotificationPreferences(ctx, store.NotificationPreferences{OutletID: "OUT001", PhoneE164: "+94771112222", ConsentEnabled: true, DeferralsEnabled: true, MajorDelaysEnabled: true, Locale: "en"}, 0, "u-dispatcher", audit.Event{Action: "OUTLET_NOTIFICATION_PREFERENCES_UPDATED"})
	if err != nil || preferences.Version != 1 || preferences.ConsentedAt == nil || !preferences.ConsentEnabled {
		t.Fatalf("save opt-in preferences=%+v err=%v", preferences, err)
	}
	first, err := s.EnqueueNotification(ctx, store.NotificationEvent{EventKey: "deferral:PLAN-01:ORD-01", OutletID: "OUT001", Type: "DEFERRAL", OrderRef: "ORD-01", Reason: "WINDOW"})
	if err != nil || first.Status != "enqueued" || first.ID == 0 {
		t.Fatalf("enqueue notification=%+v err=%v", first, err)
	}
	duplicate, err := s.EnqueueNotification(ctx, store.NotificationEvent{EventKey: "deferral:PLAN-01:ORD-01", OutletID: "OUT001", Type: "DEFERRAL", OrderRef: "ORD-01", Reason: "WINDOW"})
	if err != nil || duplicate.Status != "duplicate" || duplicate.ID != first.ID {
		t.Fatalf("deduplicated enqueue=%+v err=%v", duplicate, err)
	}
	claimed, err := s.ClaimNotification(ctx)
	if err != nil || claimed == nil || claimed.ID != first.ID || claimed.Phone != "+94771112222" {
		t.Fatalf("claim notification=%+v err=%v", claimed, err)
	}
	if err = s.FinishNotification(ctx, claimed.ID, "UNKNOWN", "", "PROVIDER_OUTCOME_UNKNOWN"); err != nil {
		t.Fatal(err)
	}
	deliveryEvent, err := s.EnqueueNotification(ctx, store.NotificationEvent{EventKey: "breakdown:PLAN-01:2:ORD-02", OutletID: "OUT001", Type: "MAJOR_DELAY", OrderRef: "ORD-02", DelayMinutes: 45})
	if err != nil || deliveryEvent.Status != "enqueued" {
		t.Fatalf("enqueue delay=%+v err=%v", deliveryEvent, err)
	}
	deliveryClaim, err := s.ClaimNotification(ctx)
	if err != nil || deliveryClaim == nil {
		t.Fatalf("claim delay=%+v err=%v", deliveryClaim, err)
	}
	if err = s.FinishNotification(ctx, deliveryClaim.ID, "QUEUED", "SM0123456789abcdef0123456789abcdef", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateNotificationStatus(ctx, "SM0123456789abcdef0123456789abcdef", "sent", ""); err != nil {
		t.Fatalf("sent callback: %v", err)
	}
	if err = s.UpdateNotificationStatus(ctx, "SM0123456789abcdef0123456789abcdef", "delivered", ""); err != nil {
		t.Fatalf("delivered callback: %v", err)
	}
	if err = s.UpdateNotificationStatus(ctx, "SM0123456789abcdef0123456789abcdef", "sent", ""); err != nil {
		t.Fatalf("older duplicate callback should be harmless: %v", err)
	}
	var deliveryStatus string
	if err = pool.QueryRow(ctx, `SELECT status FROM shared.notification_outbox WHERE id=$1`, deliveryClaim.ID).Scan(&deliveryStatus); err != nil || deliveryStatus != "DELIVERED" {
		t.Fatalf("callback status=%q err=%v", deliveryStatus, err)
	}
	arrival := store.NotificationEvent{EventKey: "arrival:stop-1:old:new", OutletID: "OUT001", Type: "ARRIVAL_CHANGE", OrderRef: "ORD-01", OldArrivalAt: "2026-10-03T08:00:00Z", NewArrivalAt: "2026-10-03T08:30:00Z"}
	arrivalFirst, err := s.EnqueueNotification(ctx, arrival)
	if err != nil || arrivalFirst.Status != "enqueued" {
		t.Fatalf("arrival notification=%+v err=%v", arrivalFirst, err)
	}
	arrivalAgain, err := s.EnqueueNotification(ctx, arrival)
	if err != nil || arrivalAgain.Status != "duplicate" || arrivalAgain.ID != arrivalFirst.ID {
		t.Fatalf("duplicate arrival notification=%+v err=%v", arrivalAgain, err)
	}
	var arrivalBody string
	if err := pool.QueryRow(ctx, `SELECT body FROM shared.notification_outbox WHERE id=$1`, arrivalFirst.ID).Scan(&arrivalBody); err != nil || !strings.Contains(arrivalBody, "from 03 Oct 13:30 to 03 Oct 14:00") {
		t.Fatalf("arrival body=%q err=%v", arrivalBody, err)
	}
	returned := store.NotificationEvent{EventKey: "returned-goods:stop-1", OutletID: "OUT001", Type: "DELIVERY_REJECTED",
		OrderRef: "ORD-01", Goods: "Rejected cartons", Units: 2, Reason: "GOODS_REJECTED",
		Resolution: "NEXT_RUN", FollowupDate: "2026-10-05"}
	returnedFirst, err := s.EnqueueNotification(ctx, returned)
	if err != nil || returnedFirst.Status != "enqueued" {
		t.Fatalf("return notice=%+v err=%v", returnedFirst, err)
	}
	returnedReplay, err := s.EnqueueNotification(ctx, returned)
	if err != nil || returnedReplay.Status != "duplicate" || returnedReplay.ID != returnedFirst.ID {
		t.Fatalf("duplicate return notice=%+v err=%v", returnedReplay, err)
	}
	preferences.ConsentEnabled = false
	preferences, err = s.SaveNotificationPreferences(ctx, preferences, 1, "u-dispatcher", audit.Event{Action: "OUTLET_NOTIFICATION_PREFERENCES_UPDATED"})
	if err != nil || preferences.Version != 2 || preferences.ConsentedAt != nil {
		t.Fatalf("consent withdrawal=%+v err=%v", preferences, err)
	}
	if _, err = s.SaveNotificationPreferences(ctx, preferences, 1, "u-dispatcher", audit.Event{Action: "OUTLET_NOTIFICATION_PREFERENCES_UPDATED"}); err == nil {
		t.Fatal("stale preferences version should conflict")
	}
	suppressed, err := s.EnqueueNotification(ctx, store.NotificationEvent{EventKey: "deferral:PLAN-02:ORD-02", OutletID: "OUT001", Type: "DEFERRAL", OrderRef: "ORD-02", Reason: "WINDOW"})
	if err != nil || suppressed.Status != "in_app_only" || suppressed.ID == 0 {
		t.Fatalf("opt-out must keep the in-app notice but not send it by SMS: %+v err=%v", suppressed, err)
	}
	if next, err := s.ClaimNotification(ctx); err != nil || next != nil {
		t.Fatalf("an in-app-only notice must never be claimed for SMS: %+v err=%v", next, err)
	}
	items, total, err := s.SearchAudit(ctx, store.AuditFilter{Query: "USR001", Limit: 10})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("search total=%d items=%d err=%v", total, len(items), err)
	}
	items, total, err = s.SearchAudit(ctx, store.AuditFilter{Action: "ORDER_DEFERRED", ResourceID: "O-09", Limit: 10})
	if err != nil || total != 1 || items[0].NewState["reasonCode"] != "WINDOW" {
		t.Fatalf("filtered audit result=%+v total=%d err=%v", items, total, err)
	}
	kpis, err := s.AuditKPIs(ctx, now.Add(-time.Minute), now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if kpis["totalEvents"] != 5 || kpis["plansGenerated"] != 1 || kpis["ordersDeferred"] != 1 || kpis["tripsCompleted"] != 1 {
		t.Fatalf("unexpected KPI summary: %+v", kpis)
	}
	if _, err = pool.Exec(ctx, `UPDATE audit.events SET action='changed' WHERE event_id='e1'`); err == nil {
		t.Fatal("audit updates must be rejected")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM audit.events WHERE event_id='e1'`); err == nil {
		t.Fatal("audit deletes must be rejected")
	}
	accessNote := "Use the east gate beside the pharmacy; drive the last 200 metres on the paved service lane."
	updated, err := s.UpdateOutlet(ctx, store.Outlet{ID: "OUT001", Brand: "Fresh", Name: "Test Outlet Updated", District: "Colombo", Depot: "DEPOT_NORTH", DockType: "normal", ParkingConstraint: "van_only", AccessInstructions: accessNote, Version: 1}, 1, audit.Event{ActorID: "u-dispatcher", Action: "MASTER_DATA_OUTLET_UPDATED", ResourceType: "OUTLET", ResourceID: "OUT001", Source: "test"})
	if err != nil || updated.Version != 2 || updated.ParkingConstraint != "van_only" || updated.AccessInstructions != accessNote || updated.AccessInstructionsUpdatedBy != "u-dispatcher" || updated.AccessInstructionsUpdatedAt == nil {
		t.Fatalf("outlet update=%+v err=%v", updated, err)
	}
	if _, err = s.UpdateOutlet(ctx, store.Outlet{ID: "OUT001", Brand: "Fresh", Name: "Stale", District: "Colombo", Depot: "DEPOT_NORTH", DockType: "normal", ParkingConstraint: "normal"}, 1, audit.Event{Action: "MASTER_DATA_OUTLET_UPDATED"}); err == nil {
		t.Fatal("stale outlet version should conflict")
	}
	day, err := s.UpdateCalendar(ctx, store.CalendarDay{Date: "2026-10-01", IsOperating: false}, 0, audit.Event{Action: "MASTER_DATA_CALENDAR_UPDATED", ResourceType: "CALENDAR", ResourceID: "2026-10-01"})
	if err != nil || day.Version != 1 || day.IsOperating {
		t.Fatalf("calendar update=%+v err=%v", day, err)
	}
	day, err = s.UpdateCalendar(ctx, store.CalendarDay{Date: "2026-10-01", IsOperating: true}, 1, audit.Event{Action: "MASTER_DATA_CALENDAR_UPDATED", ResourceType: "CALENDAR", ResourceID: "2026-10-01"})
	if err != nil || day.Version != 2 || !day.IsOperating {
		t.Fatalf("calendar revision=%+v err=%v", day, err)
	}
	if _, err = s.UpdateCalendar(ctx, store.CalendarDay{Date: "2026-10-01"}, 1, audit.Event{Action: "MASTER_DATA_CALENDAR_UPDATED"}); err == nil {
		t.Fatal("stale calendar version should conflict")
	}
	calendar, err := s.Calendar(ctx, "2026-10-01", "2026-10-01")
	if err != nil || len(calendar) != 1 || calendar[0].Version != 2 {
		t.Fatalf("calendar read=%+v err=%v", calendar, err)
	}
	policy, err := s.CurrentPolicy(ctx)
	if err != nil || policy.Version != 1 || policy.DeferralWeightPoints != 14 {
		t.Fatalf("current policy=%+v err=%v", policy, err)
	}
	policy, err = s.CreatePolicy(ctx, store.PlanningPolicy{CutoffLocalTime: "15:30", DeferralWeightPoints: 20, MaxDeferralCount: 10, MaxUnservedDays: 400, MaxTripsPerVehicle: 1, CreatedBy: "u-dispatcher"}, 1, audit.Event{ActorID: "u-dispatcher", Action: "PLANNING_POLICY_VERSION_CREATED", ResourceType: "PLANNING_POLICY", Source: "test"})
	if err != nil || policy.Version != 2 {
		t.Fatalf("create policy=%+v err=%v", policy, err)
	}
	policy, err = s.CurrentPolicy(ctx)
	if err != nil || policy.Version != 2 || policy.CutoffLocalTime != "15:30:00" {
		t.Fatalf("activate policy=%+v err=%v", policy, err)
	}
	router := chi.NewRouter()
	handler.Handler{Authn: auditAuth{}, Store: s}.Routes(router)
	request := func(subject, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+subject)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := request("dispatcher-test", "/api/v1/shared/audit/events"); res.Code != http.StatusOK {
		t.Fatalf("dispatcher audit search: %d %s", res.Code, res.Body.String())
	}
	if res := request("driver-test", "/api/v1/shared/audit/events"); res.Code != http.StatusForbidden {
		t.Fatalf("driver audit search: %d %s", res.Code, res.Body.String())
	}
	if res := request("dispatcher-test", "/api/v1/shared/audit/events?limit=1000"); res.Code != http.StatusBadRequest {
		t.Fatalf("invalid search bound: %d %s", res.Code, res.Body.String())
	}
	if res := request("dispatcher-test", "/api/v1/shared/audit/export.csv?action=MASTER_DATA_CALENDAR_UPDATED"); res.Code != http.StatusOK ||
		!strings.HasPrefix(res.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(res.Header().Get("Content-Disposition"), "attachment; filename=") ||
		!strings.HasPrefix(res.Body.String(), "event_id,timestamp,actor_id") {
		t.Fatalf("dispatcher audit export: %d %v %s", res.Code, res.Header(), res.Body.String())
	}
	if res := request("driver-test", "/api/v1/shared/audit/export.csv"); res.Code != http.StatusForbidden {
		t.Fatalf("driver audit export: %d", res.Code)
	}
	if res := request("dispatcher-test", "/api/v1/shared/audit/export.csv?from=bad"); res.Code != http.StatusBadRequest {
		t.Fatalf("invalid export filter: %d", res.Code)
	}
	put := func(subject, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+subject)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	body := `{"name":"API updated outlet","brand":"Fresh","district":"Colombo","depot":"DEPOT_NORTH","dockType":"normal","parkingConstraint":"normal","mallWindow":false,"windowOpenTime":"08:00","windowCloseTime":"17:00","accessInstructions":"Use the east gate beside the pharmacy; drive the last 200 metres on the paved service lane.","chilledTemperatureMinC":-2,"chilledTemperatureMaxC":4,"version":2}`
	if res := put("dispatcher-test", "/api/v1/shared/outlets/OUT001", body); res.Code != http.StatusOK {
		t.Fatalf("dispatcher outlet update: %d %s", res.Code, res.Body.String())
	} else if !strings.Contains(res.Body.String(), `"chilledTemperatureMinC":-2`) || !strings.Contains(res.Body.String(), `"chilledTemperatureMaxC":4`) {
		t.Fatalf("outlet temperature review limits missing from response: %s", res.Body.String())
	}
	invalidRange := `{"name":"API updated outlet","brand":"Fresh","district":"Colombo","depot":"DEPOT_NORTH","dockType":"normal","parkingConstraint":"normal","mallWindow":false,"accessInstructions":"Use the east gate","chilledTemperatureMinC":4,"chilledTemperatureMaxC":-2,"version":3}`
	if res := put("dispatcher-test", "/api/v1/shared/outlets/OUT001", invalidRange); res.Code != http.StatusBadRequest {
		t.Fatalf("reversed temperature range must fail validation: %d %s", res.Code, res.Body.String())
	}
	if res := put("driver-test", "/api/v1/shared/outlets/OUT001", body); res.Code != http.StatusForbidden {
		t.Fatalf("driver outlet update: %d %s", res.Code, res.Body.String())
	}
	if res := put("dispatcher-test", "/api/v1/shared/outlets/OUT001", body); res.Code != http.StatusConflict {
		t.Fatalf("stale outlet API update: %d %s", res.Code, res.Body.String())
	}
	confirm := func(subject, outletID string, expected int) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/shared/outlets/"+outletID+"/access-instructions/confirm", bytes.NewBufferString(fmt.Sprintf(`{"expectedVersion":%d}`, expected)))
		req.Header.Set("Authorization", "Bearer "+subject)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	confirmed := confirm("store-test", "OUT001", 3)
	if confirmed.Code != http.StatusOK || !strings.Contains(confirmed.Body.String(), `"accessInstructionsConfirmedBy":"u-store"`) || !strings.Contains(confirmed.Body.String(), `"accessInstructionsConfirmedAt"`) {
		t.Fatalf("store manager confirmation: %d %s", confirmed.Code, confirmed.Body.String())
	}
	if res := confirm("store-test", "OUT001", 3); res.Code != http.StatusConflict {
		t.Fatalf("stale confirmation should conflict: %d %s", res.Code, res.Body.String())
	}
	if res := confirm("store-test", "OUT002", 3); res.Code != http.StatusForbidden {
		t.Fatalf("store manager must not confirm another outlet: %d %s", res.Code, res.Body.String())
	}
	if res := confirm("dispatcher-test", "OUT001", 4); res.Code != http.StatusForbidden {
		t.Fatalf("dispatcher cannot make store-manager confirmation: %d %s", res.Code, res.Body.String())
	}
	if res := confirm("driver-test", "OUT001", 4); res.Code != http.StatusForbidden {
		t.Fatalf("driver cannot confirm access instructions: %d %s", res.Code, res.Body.String())
	}
	if res := request("dispatcher-test", "/api/v1/shared/policies/current"); res.Code != http.StatusOK {
		t.Fatalf("dispatcher policy read: %d %s", res.Code, res.Body.String())
	}
	if res := request("driver-test", "/api/v1/shared/policies/current"); res.Code != http.StatusForbidden {
		t.Fatalf("driver policy read: %d %s", res.Code, res.Body.String())
	}
	policyBody := `{"version":2,"cutoffLocalTime":"15:30","deferralWeightPoints":20,"maxDeferralCount":10,"maxUnservedDays":400,"maxTripsPerVehicle":1}`
	missingVersion := httptest.NewRequest(http.MethodPut, "/api/v1/shared/policies/current", bytes.NewBufferString(`{"cutoffLocalTime":"15:30","deferralWeightPoints":20,"maxDeferralCount":10,"maxUnservedDays":400,"maxTripsPerVehicle":1}`))
	missingVersion.Header.Set("Authorization", "Bearer dispatcher-test")
	missingVersionRes := httptest.NewRecorder()
	router.ServeHTTP(missingVersionRes, missingVersion)
	if missingVersionRes.Code != http.StatusBadRequest {
		t.Fatalf("policy update without expected version should fail: %d %s", missingVersionRes.Code, missingVersionRes.Body.String())
	}
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer dispatcher-test")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	putPolicy := httptest.NewRequest(http.MethodPut, "/api/v1/shared/policies/current", bytes.NewBufferString(policyBody))
	putPolicy.Header.Set("Authorization", "Bearer dispatcher-test")
	policyRes := httptest.NewRecorder()
	router.ServeHTTP(policyRes, putPolicy)
	if policyRes.Code != http.StatusCreated {
		t.Fatalf("dispatcher policy version: %d %s", policyRes.Code, policyRes.Body.String())
	}
	stalePolicy := httptest.NewRequest(http.MethodPut, "/api/v1/shared/policies/current", bytes.NewBufferString(policyBody))
	stalePolicy.Header.Set("Authorization", "Bearer dispatcher-test")
	stalePolicyRes := httptest.NewRecorder()
	router.ServeHTTP(stalePolicyRes, stalePolicy)
	if stalePolicyRes.Code != http.StatusConflict {
		t.Fatalf("stale policy version should conflict: %d %s", stalePolicyRes.Code, stalePolicyRes.Body.String())
	}
	if res := post("/api/v1/shared/policies/preview", policyBody); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"hardConstraintsUnchanged":true`) {
		t.Fatalf("policy preview: %d %s", res.Code, res.Body.String())
	}
	if res := post("/api/v1/shared/policies/preview", `{"cutoffLocalTime":"27:80","deferralWeightPoints":20,"maxDeferralCount":10,"maxUnservedDays":400,"maxTripsPerVehicle":1}`); res.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy preview: %d %s", res.Code, res.Body.String())
	}

	// The outlet can read back the messages queued for it. A queued-but-unsent shortfall notice shows
	// its text, the newest comes first, another outlet's messages and rows with no text are left out.
	_, err = pool.Exec(ctx, `INSERT INTO shared.notification_outbox(event_key,outlet_id,event_type,phone_e164,body,status,created_at) VALUES
	('shortfall:PLAN-03:ORD-03','OUT001','LOAD_SHORTFALL','+94771112222','Waypoint: order ORD-03 is 2 units short; the rest will arrive on the next delivery.','PENDING',now()+interval '1 minute'),
	('shortfall:PLAN-03:ORD-04','OUT002','LOAD_SHORTFALL','+94773334444','Waypoint: order ORD-04 is 5 units short.','PENDING',now()+interval '2 minutes'),
	('shortfall:PLAN-03:ORD-05','OUT001','LOAD_SHORTFALL','+94771112222','','SUPPRESSED',now()+interval '3 minutes')`)
	if err != nil {
		t.Fatal(err)
	}
	type outletNotifications struct {
		Items []struct {
			ID        int64  `json:"id"`
			EventType string `json:"eventType"`
			Body      string `json:"body"`
			Status    string `json:"status"`
			CreatedAt string `json:"createdAt"`
			Phone     string `json:"phoneE164"`
		} `json:"items"`
	}
	feed := func(subject, outletID string) (outletNotifications, int, string) {
		res := request(subject, "/api/v1/shared/outlets/"+outletID+"/notifications")
		var out outletNotifications
		if res.Code == http.StatusOK {
			if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode notifications: %v %s", err, res.Body.String())
			}
		}
		return out, res.Code, res.Body.String()
	}
	mine, code, raw := feed("store-test", "OUT001")
	if code != http.StatusOK || len(mine.Items) < 3 {
		t.Fatalf("store manager reads their outlet's messages: %d %s", code, raw)
	}
	if mine.Items[0].EventType != "LOAD_SHORTFALL" || mine.Items[0].Status != "PENDING" || !strings.Contains(mine.Items[0].Body, "ORD-03 is 2 units short") || mine.Items[0].CreatedAt == "" || mine.Items[0].ID == 0 {
		t.Fatalf("newest message must be the shortfall notice with its text: %+v", mine.Items[0])
	}
	if last := len(mine.Items); mine.Items[last-2].EventType != "MAJOR_DELAY" || mine.Items[last-1].EventType != "DEFERRAL" || strings.Contains(raw, "ORD-04") || strings.Contains(raw, "+9477") {
		t.Fatalf("older messages follow, nothing from another outlet, no phone numbers: %s", raw)
	}
	if _, code, _ = feed("store-test", "OUT002"); code != http.StatusForbidden {
		t.Fatalf("store manager must not read another outlet's messages: %d", code)
	}
	if all, code, _ := feed("dispatcher-test", "OUT002"); code != http.StatusOK || len(all.Items) != 1 || all.Items[0].EventType != "LOAD_SHORTFALL" {
		t.Fatalf("dispatcher reads any outlet's messages: %d %+v", code, all)
	}
	if _, code, _ = feed("dispatcher-test", "OUT404"); code != http.StatusNotFound {
		t.Fatalf("unknown outlet must be 404: %d", code)
	}
	if _, code, _ = feed("driver-test", "OUT001"); code != http.StatusForbidden {
		t.Fatalf("a driver cannot read outlet messages: %d", code)
	}
	for i := 0; i < 60; i++ {
		if _, err = pool.Exec(ctx, `INSERT INTO shared.notification_outbox(event_key,outlet_id,event_type,phone_e164,body,status,created_at) VALUES($1,'OUT002','DEFERRAL','+94773334444','bulk','PENDING',now()-interval '1 day')`, fmt.Sprintf("bulk:%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if capped, code, _ := feed("dispatcher-test", "OUT002"); code != http.StatusOK || len(capped.Items) != 50 || capped.Items[0].EventType != "LOAD_SHORTFALL" {
		t.Fatalf("the feed is capped at 50, newest first: %d %d", code, len(capped.Items))
	}
}

func startAuditPostgres(ctx context.Context, request testcontainers.GenericContainerRequest) (container testcontainers.Container, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("testcontainers could not access Docker: %v", recovered)
		}
	}()
	return testcontainers.GenericContainer(ctx, request)
}
