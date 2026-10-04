package store

import (
	"context"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutomationSnapshotRespectsHistoricalAllocation(t *testing.T) {
	ctx := context.Background()
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2)}, Started: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pg.Terminate(ctx) })
	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432/tcp")
	pool, err := db.Open(ctx, "postgres://waypoint:waypoint@"+host+":"+port.Port()+"/waypoint?sslmode=disable", "public")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	paths, _ := filepath.Glob("../../../../database/migrations/*.sql")
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	p := Postgres{Pool: pool}
	before := time.Now().Add(-time.Hour)
	snap, err := p.AutomationSnapshot(ctx, before)
	if err != nil || !before.Before(snap.HistorySince) {
		t.Fatal("missing history must be explicit")
	}
	var plan string
	err = pool.QueryRow(ctx, `INSERT INTO planning.plans(plan_ref,delivery_date,status,created_by) VALUES('DEMO','2026-10-03','draft','test') RETURNING id::text`).Scan(&plan)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO planning.deferrals(plan_id,order_id,outlet_id,reason_code,deferred_by) VALUES($1::uuid,'ORDER1','OUT1','CAPACITY','test')`, plan)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	snap, err = p.AutomationSnapshot(ctx, at)
	if err != nil || len(snap.Items) != 1 {
		t.Fatalf("deferral missing: %+v %v", snap, err)
	}
	// A mutation that rolls back must not leave phantom history.
	tx, _ := pool.Begin(ctx)
	_, err = tx.Exec(ctx, `DELETE FROM planning.deferrals WHERE plan_id=$1::uuid`, plan)
	if err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	snap, err = p.AutomationSnapshot(ctx, time.Now())
	if err != nil || len(snap.Items) != 1 {
		t.Fatal("rollback lost deferral")
	}
	_, err = pool.Exec(ctx, `DELETE FROM planning.deferrals WHERE plan_id=$1::uuid`, plan)
	if err != nil {
		t.Fatal(err)
	}
	snap, err = p.AutomationSnapshot(ctx, time.Now())
	if err != nil || len(snap.Items) != 0 {
		t.Fatal("resolved deferral remained")
	}
	snap, err = p.AutomationSnapshot(ctx, at)
	if err != nil || len(snap.Items) != 1 {
		t.Fatal("historical test used current state")
	}
	// An allocation on a later run supersedes an earlier deferral.
	_, err = pool.Exec(ctx, `INSERT INTO planning.automation_order_history(plan_id,order_id,outlet_id,delivery_date,state,recorded_at) VALUES($1::uuid,'ORDER2','OUT1','2026-10-02','deferred',now()),($1::uuid,'ORDER2','','2026-10-03','allocated',now())`, plan)
	if err != nil {
		t.Fatal(err)
	}
	snap, err = p.AutomationSnapshot(ctx, time.Now())
	if err != nil || len(snap.Items) != 0 {
		t.Fatal("allocated order still deferred")
	}
}
