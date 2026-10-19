package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	orderstore "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

func TestOrderLinesArePersistedWithTheOrderAndReadBack(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second)}
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Skipf("testcontainers postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432")
	dsn := "postgres://waypoint:waypoint@" + host + ":" + port.Port() + "/waypoint?sslmode=disable"
	root := repoRoot(t)
	for _, m := range []string{"0001_init.sql", "0002_identity.sql", "0003_orders.sql", "0014_order_receipts.sql", "0025_order_import_keys.sql", "0051_orders_order_lines.sql"} {
		applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", m))
	}
	shared, err := db.Open(ctx, dsn, "shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(shared.Close)
	if _, err = shared.Exec(ctx, `INSERT INTO outlets (id, brand, name) VALUES ('OUT034','Fresh','Fresh OUT034')`); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, dsn, "orders")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repo := orderstore.Postgres{Pool: pool}

	with, err := repo.Create(domain.Order{OutletID: "OUT034", Brand: "Fresh", RequestedDeliveryDate: "2026-10-07", OrderUnits: 5, OrderWeightKg: 46.2, OrderVolumeM3: 0.086, TemperatureRequirement: domain.TempChilled, CreatedBy: "u", Lines: []domain.OrderLine{
		{LineNo: 1, ProductID: "FR-MILK-1L", ProductName: "Fresh milk 1 L", Pack: "crate", UnitsPerPack: 12, PackQty: 3, WeightKg: 37.8, VolumeM3: 0.063, Source: "form"},
		{LineNo: 2, ProductID: "FR-CURD-500G", ProductName: "Curd 500 g", Pack: "tray", UnitsPerPack: 8, PackQty: 2, WeightKg: 8.4, VolumeM3: 0.0228, Source: "text_helper"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	without, err := repo.Create(domain.Order{OutletID: "OUT034", Brand: "Fresh", RequestedDeliveryDate: "2026-10-07", OrderUnits: 2, OrderWeightKg: 10, OrderVolumeM3: 0.1, TemperatureRequirement: domain.TempAmbient, CreatedBy: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if len(with.Lines) != 2 || len(without.Lines) != 0 {
		t.Fatalf("create: %d and %d lines", len(with.Lines), len(without.Lines))
	}

	got, err := repo.Get(with.OrderRef)
	if err != nil || len(got.Lines) != 2 || got.Lines[0].ProductID != "FR-MILK-1L" || got.Lines[1].PackQty != 2 || got.Lines[1].Source != "text_helper" || got.Lines[0].WeightKg != 37.8 {
		t.Fatalf("get: %+v %v", got, err)
	}
	if got, err = repo.Get(without.ID); err != nil || len(got.Lines) != 0 {
		t.Fatalf("an order without lines has none: %+v %v", got, err)
	}
	list, err := repo.List(domain.ListFilter{OutletID: "OUT034"})
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %+v %v", list, err)
	}
	for _, o := range list {
		if o.ID == with.ID && len(o.Lines) != 2 || o.ID == without.ID && len(o.Lines) != 0 {
			t.Fatalf("list attached the wrong lines to %s: %+v", o.OrderRef, o.Lines)
		}
	}

	recent, err := repo.RecentLines("OUT034", "chilled", "2026-10-01")
	if err != nil || len(recent) != 1 || recent[0].Packs["FR-MILK-1L"] != 3 || recent[0].Packs["FR-CURD-500G"] != 2 || recent[0].DeliveryDate != "2026-10-07" {
		t.Fatalf("history groups one order's lines together: %+v %v", recent, err)
	}
	if none, _ := repo.RecentLines("OUT034", "ambient", "2026-10-01"); len(none) != 0 {
		t.Fatalf("an order without lines, or another goods type, is not history: %+v", none)
	}
	if none, _ := repo.RecentLines("OUT034", "chilled", "2026-10-08"); len(none) != 0 {
		t.Fatalf("orders before the cutoff date are not history: %+v", none)
	}
	if none, _ := repo.RecentLines("OUT999", "chilled", "2026-10-01"); len(none) != 0 {
		t.Fatalf("another outlet's orders are not history: %+v", none)
	}

	// A failing line must not leave a half-saved order behind.
	before, _ := repo.List(domain.ListFilter{})
	_, err = repo.Create(domain.Order{OutletID: "OUT034", Brand: "Fresh", RequestedDeliveryDate: "2026-10-07", OrderUnits: 1, OrderWeightKg: 1, OrderVolumeM3: 1, TemperatureRequirement: domain.TempAmbient, CreatedBy: "u", Lines: []domain.OrderLine{
		{LineNo: 1, ProductID: "FR-RICE-5KG", ProductName: "Rice", Pack: "bag", UnitsPerPack: 1, PackQty: 1, WeightKg: 1, VolumeM3: 1, Source: "form"},
		{LineNo: 1, ProductID: "FR-RICE-5KG", ProductName: "Rice", Pack: "bag", UnitsPerPack: 1, PackQty: 1, WeightKg: 1, VolumeM3: 1, Source: "form"},
	}})
	after, _ := repo.List(domain.ListFilter{})
	if err == nil || len(after) != len(before) {
		t.Fatalf("a duplicate line number must roll the whole order back: err=%v before=%d after=%d", err, len(before), len(after))
	}
}
