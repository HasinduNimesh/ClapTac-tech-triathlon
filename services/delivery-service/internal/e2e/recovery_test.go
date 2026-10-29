package e2e

import (
	"context"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/store"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverySuspensionPreventsLateCompletion(t *testing.T) {
	ctx := context.Background()
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2)}, Started: true})
	if err != nil {
		t.Skipf("Postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432")
	dsn := "postgres://waypoint:waypoint@" + host + ":" + port.Port() + "/waypoint?sslmode=disable"
	for _, m := range []string{"0001_init.sql", "0012_delivery.sql", "0080_delivery_recovery_suspensions.sql"} {
		applySQL(t, ctx, dsn, filepath.Join(repoRoot(t), "database", "migrations", m))
	}
	pool, err := db.Open(ctx, dsn, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repo := store.Postgres{Pool: pool}
	var runID, stopID string
	err = pool.QueryRow(ctx, `INSERT INTO runs(trip_id,plan_id,plan_ref,delivery_date,vehicle_id,depot,status) VALUES('old-trip','plan','PLAN',current_date,'broken','depot','in_progress') RETURNING id::text`).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO stops(run_id,allocation_id,order_id,stop_sequence,status) VALUES($1::uuid,'a1','order',1,'arrived') RETURNING id::text`, runID).Scan(&stopID)
	if err != nil {
		t.Fatal(err)
	}
	stops := []store.RecoveryStop{{TripID: "old-trip", OrderID: "order"}}
	if err = repo.SuspendRecovery(ctx, "case", stops); err != nil {
		t.Fatal(err)
	}
	if err = repo.SuspendRecovery(ctx, "case", stops); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE stops SET status='completed',outcome_code='DELIVERED' WHERE id=$1::uuid`, stopID); err == nil || !strings.Contains(err.Error(), "suspended") {
		t.Fatalf("late completion accepted: %v", err)
	}
	// A newly prepared replacement trip may finish the same order.
	err = pool.QueryRow(ctx, `INSERT INTO runs(trip_id,plan_id,plan_ref,delivery_date,vehicle_id,depot,status) VALUES('new-trip','plan','PLAN',current_date,'rescue','depot','in_progress') RETURNING id::text`).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO stops(run_id,allocation_id,order_id,stop_sequence,status) VALUES($1::uuid,'a1','order',1,'arrived') RETURNING id::text`, runID).Scan(&stopID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE stops SET status='completed',outcome_code='DELIVERED' WHERE id=$1::uuid`, stopID); err != nil {
		t.Fatal(err)
	}
	if err = repo.SuspendRecovery(ctx, "second-case", []store.RecoveryStop{{TripID: "new-trip", OrderID: "order"}}); err == nil {
		t.Fatal("completed delivery suspended")
	}
	// Test an in-flight completion which holds the shared advisory lock.
	err = pool.QueryRow(ctx, `INSERT INTO stops(run_id,allocation_id,order_id,stop_sequence,status) VALUES($1::uuid,'a2','racing-order',2,'arrived') RETURNING id::text`, runID).Scan(&stopID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE stops SET status='completed',outcome_code='DELIVERED' WHERE id=$1::uuid`, stopID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- repo.SuspendRecovery(ctx, "race-case", []store.RecoveryStop{{TripID: "new-trip", OrderID: "racing-order"}})
	}()
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err == nil {
		t.Fatal("suspension ignored concurrent completed delivery")
	}
	t.Log("Old/offline completion rejected, replacement completion allowed, completed and racing deliveries protected")
}
