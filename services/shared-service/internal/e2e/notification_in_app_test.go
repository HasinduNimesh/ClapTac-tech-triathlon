package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

// A store is always told in-app what will arrive. SMS consent and alert choices decide only whether a
// notice is also sent by SMS: an opted-out store, a store with the deferral alert off and a store with no
// preference row each get an in-app row that the SMS worker can never claim.
func TestStoreNoticesAreStoredInAppIndependentlyOfSMSPreferences(t *testing.T) {
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
	if _, err = pool.Exec(ctx, `CREATE SCHEMA shared; CREATE TABLE shared.outlets(id text primary key);
	INSERT INTO shared.outlets(id) VALUES('OPTED_OUT'),('ALERT_OFF'),('NO_PREFS'),('SMS_ON')`); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"0026_shared_notifications.sql", "0070_notification_load_shortfall.sql", "0072_notification_in_app_only.sql"} {
		sql, err := os.ReadFile("../../../../database/migrations/" + f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	// OPTED_OUT: has a number but consent is off. ALERT_OFF: consent on, deferral alert off, delay alert on
	// (the shortfall notice follows the deferral alert). SMS_ON: fully opted in, Sinhala.
	if _, err = pool.Exec(ctx, `INSERT INTO shared.outlet_notification_preferences(outlet_id,phone_e164,consent_enabled,deferrals_enabled,major_delays_enabled,locale,consented_at,updated_by) VALUES
	('OPTED_OUT','+94770000001',false,true,true,'ta',NULL,'t'),
	('ALERT_OFF','+94770000002',true,false,true,'en',now(),'t'),
	('SMS_ON','+94770000003',true,true,true,'si',now(),'t')`); err != nil {
		t.Fatal(err)
	}
	s := store.Store{Pool: pool}
	shortfall := func(key, outlet string) store.NotificationEvent {
		return store.NotificationEvent{EventKey: key, OutletID: outlet, Type: "LOAD_SHORTFALL", OrderRef: "ORD-9", Units: 4, Reason: "PARTIAL_LOAD"}
	}

	for _, outlet := range []string{"OPTED_OUT", "ALERT_OFF", "NO_PREFS"} {
		first, err := s.EnqueueNotification(ctx, shortfall("sf:"+outlet, outlet))
		if err != nil || first.Status != "in_app_only" || first.ID == 0 {
			t.Fatalf("%s shortfall must still be stored in-app: %+v err=%v", outlet, first, err)
		}
		again, err := s.EnqueueNotification(ctx, shortfall("sf:"+outlet, outlet))
		if err != nil || again.Status != "duplicate" || again.ID != first.ID {
			t.Fatalf("%s duplicate event must return the first row: %+v err=%v", outlet, again, err)
		}
		var rows int
		var status, phone, body string
		if err = pool.QueryRow(ctx, `SELECT count(*),min(status),min(phone_e164),min(body) FROM shared.notification_outbox WHERE event_key=$1`, "sf:"+outlet).Scan(&rows, &status, &phone, &body); err != nil || rows != 1 || status != "IN_APP_ONLY" || phone != "" || !strings.Contains(body, "4") {
			t.Fatalf("%s: rows=%d status=%q phone=%q body=%q err=%v", outlet, rows, status, phone, body, err)
		}
		feed, err := s.OutletNotifications(ctx, outlet, 50)
		if err != nil || len(feed) != 1 || feed[0].Status != "IN_APP_ONLY" || feed[0].EventType != "LOAD_SHORTFALL" || feed[0].Body != body {
			t.Fatalf("%s: the store feed must show the in-app notice: %+v err=%v", outlet, feed, err)
		}
	}

	// Other event types are stored in-app too, in the outlet's language (English with no preference row).
	if r, err := s.EnqueueNotification(ctx, store.NotificationEvent{EventKey: "d:NO_PREFS", OutletID: "NO_PREFS", Type: "DEFERRAL", OrderRef: "ORD-1", Reason: "WINDOW"}); err != nil || r.Status != "in_app_only" {
		t.Fatalf("deferral without preferences: %+v err=%v", r, err)
	}
	if r, err := s.EnqueueNotification(ctx, store.NotificationEvent{EventKey: "m:OPTED_OUT", OutletID: "OPTED_OUT", Type: "MAJOR_DELAY", OrderRef: "ORD-2", DelayMinutes: 45}); err != nil || r.Status != "in_app_only" {
		t.Fatalf("delay for an opted-out store: %+v err=%v", r, err)
	}
	if r, err := s.EnqueueNotification(ctx, store.NotificationEvent{EventKey: "d:ALERT_OFF", OutletID: "ALERT_OFF", Type: "DEFERRAL", OrderRef: "ORD-3", Reason: "WINDOW"}); err != nil || r.Status != "in_app_only" {
		t.Fatalf("deferral with the alert off: %+v err=%v", r, err)
	}
	noPrefs, err := s.OutletNotifications(ctx, "NO_PREFS", 50)
	if err != nil || len(noPrefs) != 2 || !strings.HasPrefix(noPrefs[0].Body, "Waypoint: order ORD-1 was deferred") {
		t.Fatalf("no-preference outlet reads English text: %+v err=%v", noPrefs, err)
	}
	if feed, err := s.OutletNotifications(ctx, "OPTED_OUT", 50); err != nil || len(feed) != 2 || !strings.Contains(feed[0].Body, "45") {
		t.Fatalf("opted-out outlet reads its delay notice: %+v err=%v", feed, err)
	}
	if _, err = s.EnqueueNotification(ctx, shortfall("sf:UNKNOWN", "NO_SUCH_OUTLET")); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("an unknown outlet must still be rejected: %v", err)
	}

	// None of the in-app rows can be claimed by the SMS worker.
	if n, err := s.ClaimNotification(ctx); err != nil || n != nil {
		t.Fatalf("in-app-only rows must never be claimed for SMS: %+v err=%v", n, err)
	}

	// An opted-in store gets the same notice both ways: pending for SMS and in its feed. The shortfall
	// follows the deferral alert, so it is claimable.
	sms, err := s.EnqueueNotification(ctx, shortfall("sf:SMS_ON", "SMS_ON"))
	if err != nil || sms.Status != "enqueued" {
		t.Fatalf("opted-in shortfall: %+v err=%v", sms, err)
	}
	claimed, err := s.ClaimNotification(ctx)
	if err != nil || claimed == nil || claimed.ID != sms.ID || claimed.Phone != "+94770000003" || claimed.EventType != "LOAD_SHORTFALL" {
		t.Fatalf("an opted-in shortfall must be sent by SMS: %+v err=%v", claimed, err)
	}

	// Consent withdrawn after a notice was queued: the worker turns it in-app-only and keeps its text.
	later, err := s.EnqueueNotification(ctx, store.NotificationEvent{EventKey: "d:SMS_ON", OutletID: "SMS_ON", Type: "DEFERRAL", OrderRef: "ORD-5", Reason: "WINDOW"})
	if err != nil || later.Status != "enqueued" {
		t.Fatalf("deferral before withdrawal: %+v err=%v", later, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE shared.outlet_notification_preferences SET consent_enabled=false,consented_at=NULL WHERE outlet_id='SMS_ON'`); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ClaimNotification(ctx); err != nil || n != nil {
		t.Fatalf("withdrawn consent must not send: %+v err=%v", n, err)
	}
	feed, err := s.OutletNotifications(ctx, "SMS_ON", 50)
	if err != nil || len(feed) != 2 || feed[0].Status != "IN_APP_ONLY" || !strings.Contains(feed[0].Body, "ORD-5") {
		t.Fatalf("a notice whose SMS was withdrawn stays visible: %+v err=%v", feed, err)
	}
	var phone string
	if err = pool.QueryRow(ctx, `SELECT phone_e164 FROM shared.notification_outbox WHERE id=$1`, later.ID).Scan(&phone); err != nil || phone != "" {
		t.Fatalf("no phone number is kept on an in-app-only row: %q err=%v", phone, err)
	}
}
