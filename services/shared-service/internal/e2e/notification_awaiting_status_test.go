package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

// A text sent through the cellular gateway is final at SENT (the gateway sends no delivery receipt), so it must not
// count as "awaiting a provider status". A Twilio-style SENT or any QUEUED message still does.
func TestGatewaySentTextsAreNotCountedAsAwaitingStatus(t *testing.T) {
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
	if _, err = pool.Exec(ctx, `CREATE SCHEMA shared; CREATE TABLE shared.outlets(id text primary key); INSERT INTO shared.outlets(id) VALUES('O1')`); err != nil {
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
	insert := func(key, status, providerID string, age string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO shared.notification_outbox(event_key,outlet_id,event_type,phone_e164,body,status,provider_message_id,updated_at) VALUES($1,'O1','DEFERRAL','+94770000001','b',$2,NULLIF($3,''),now()-$4::interval)`, key, status, providerID, age); err != nil {
			t.Fatalf("insert %s: %v", key, err)
		}
	}
	s := store.Store{Pool: pool}

	// Only gateway texts, however old: nothing is waiting.
	insert("g1", "SENT", "cg:req_aaaaaaaaaaaaaaaaaaaa", "14 days")
	insert("g2", "SENT", "cg:req_bbbbbbbbbbbbbbbbbbbb", "1 day")
	stats, err := s.NotificationOutboxStats(ctx)
	if err != nil || stats.AwaitingStatus != 0 || stats.AwaitingOldestSeconds != 0 {
		t.Fatalf("gateway SENT texts must not be awaiting: %+v err=%v", stats, err)
	}

	// A Twilio-style SENT text and a QUEUED text still owe a status.
	insert("t1", "SENT", "SM0123456789abcdef0123456789abcdef", "2 hours")
	insert("q1", "QUEUED", "cg:req_cccccccccccccccccccc", "30 minutes")
	stats, err = s.NotificationOutboxStats(ctx)
	if err != nil || stats.AwaitingStatus != 2 || stats.AwaitingOldestSeconds < 2*3600-60 || stats.AwaitingOldestSeconds > 2*3600+300 {
		t.Fatalf("a Twilio SENT and a QUEUED text are awaiting: %+v err=%v", stats, err)
	}
}
