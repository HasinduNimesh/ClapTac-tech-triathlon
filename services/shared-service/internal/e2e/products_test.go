package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

func TestProductLookupIsScopedToTheCallersRange(t *testing.T) {
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
	if _, err = pool.Exec(ctx, `CREATE SCHEMA audit;
	CREATE SCHEMA shared;
	CREATE TABLE shared.outlets(id text primary key);
	INSERT INTO shared.outlets VALUES ('OUT001'),('OUT002');
	CREATE TABLE shared.users(id text primary key, identity_subject text unique, display_name text not null default '', role text);
	CREATE TABLE shared.store_manager_profiles(user_id text, outlet_id text);
	CREATE TABLE shared.loader_profiles(user_id text, depot text);
	CREATE TABLE shared.driver_profiles(user_id text, vehicle_id text);
	CREATE TABLE shared.dispatcher_profiles(user_id text, depot text);
	INSERT INTO shared.users(id,identity_subject,role) VALUES ('u-store','store-test','STORE_MANAGER'),('u-disp','disp-test','DISPATCHER'),('u-driver','driver-test','DRIVER');
	INSERT INTO shared.store_manager_profiles(user_id,outlet_id) VALUES('u-store','OUT001');
	INSERT INTO shared.dispatcher_profiles(user_id,depot) VALUES('u-disp',NULL)`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../../database/migrations/0050_shared_product_catalog.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply catalog migration: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO shared.product_categories(id,parent_id,brand,name) VALUES ('FR-DAIRY',NULL,'Fresh','Dairy');
	INSERT INTO shared.products(id,sku,gtin,brand,category_id,family,name,size,pack_name,units_per_pack,pack_weight_kg,pack_volume_m3,temperature) VALUES
	('FR-MILK-1L','SKU-1','1000000000001','Fresh','FR-DAIRY','milk','Fresh milk 1 L','1 L','crate',12,12.600,0.0210,'chilled'),
	('FR-RICE-5KG','SKU-2','1000000000002','Fresh','FR-DAIRY','rice','Samba rice 5 kg','5 kg','bag',1,5.100,0.0090,'ambient'),
	('FR-OLD-1KG','SKU-3','1000000000003','Fresh','FR-DAIRY','old','Old line','1 kg','bag',1,1.000,0.0010,'ambient');
	UPDATE shared.products SET status='discontinued' WHERE id='FR-OLD-1KG';
	INSERT INTO shared.outlet_products(outlet_id,product_id,avg_daily_sales_each) VALUES ('OUT001','FR-MILK-1L',36),('OUT002','FR-RICE-5KG',4)`); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	handler.Handler{Authn: auditAuth{}, Store: store.Store{Pool: pool}}.Routes(router)
	call := func(subject, path string) (int, []store.Product) {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+subject)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, r)
		var body struct {
			Items []store.Product `json:"items"`
		}
		_ = json.Unmarshal(res.Body.Bytes(), &body)
		return res.Code, body.Items
	}

	if code, items := call("store-test", "/api/v1/shared/products"); code != 200 || len(items) != 1 || items[0].ID != "FR-MILK-1L" || items[0].PackWeightKg != 12.6 || items[0].Pack != "crate" || items[0].AvgDailySalesEach != 36 {
		t.Fatalf("a store manager sees only their outlet's range: %d %+v", code, items)
	}
	if code, items := call("store-test", "/api/v1/shared/products?outlet_id=OUT002"); code != 200 || len(items) != 1 || items[0].ID != "FR-MILK-1L" {
		t.Fatalf("a store manager cannot ask for another outlet's range: %d %+v", code, items)
	}
	if code, items := call("disp-test", "/api/v1/shared/products?brand=Fresh"); code != 200 || len(items) != 2 || items[0].AvgDailySalesEach != 0 {
		t.Fatalf("staff see every active product; discontinued ones are hidden: %d %+v", code, items)
	}
	if code, items := call("disp-test", "/api/v1/shared/products?ids=FR-RICE-5KG,FR-MILK-1L,FR-RICE-5KG,FR-OLD-1KG"); code != 200 || len(items) != 2 {
		t.Fatalf("lookup by ids: %d %+v", code, items)
	}
	if code, _ := call("disp-test", "/api/v1/shared/products?ids=bad%27id"); code != http.StatusBadRequest {
		t.Fatalf("a malformed id must be rejected: %d", code)
	}
	if code, _ := call("driver-test", "/api/v1/shared/products"); code != http.StatusForbidden {
		t.Fatalf("a driver has no business reading the catalog: %d", code)
	}
}
