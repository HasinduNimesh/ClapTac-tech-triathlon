package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

// A dispatcher signs in and is told which depot they work from; one with no depot covers every depot.
func TestDispatcherProfileCarriesTheirDepot(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"}, WaitingFor: wait.ForListeningPort("5432/tcp")}
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
	// The real identity migrations and the new one, not a hand-made copy of them.
	if _, err = pool.Exec(ctx, `CREATE SCHEMA shared; CREATE SCHEMA audit;`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0002_identity.sql", "0008_loader_profiles.sql", "0010_driver_profiles.sql", "0045_dispatcher_profiles.sql"} {
		sql, err := os.ReadFile("../../../../database/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	_, err = pool.Exec(ctx, `INSERT INTO shared.users(id,identity_subject,role) VALUES
		('u-north','north','DISPATCHER'),('u-all','all','DISPATCHER'),('u-null','null-depot','DISPATCHER'),('u-loader','loader','LOADER');
	INSERT INTO shared.dispatcher_profiles(user_id,depot) VALUES ('u-north','DEPOT_NORTH'),('u-null',NULL);
	INSERT INTO shared.loader_profiles(user_id,depot) VALUES ('u-loader','DEPOT_SOUTH')`)
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	handler.Handler{Authn: auditAuth{}, Store: store.Store{Pool: pool}}.Routes(router)
	me := func(subject string) (depot string, roles []string) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/shared/profiles/me", nil)
		r.Header.Set("Authorization", "Bearer "+subject)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, r)
		var out struct {
			Profile struct {
				Depot string   `json:"depot"`
				Roles []string `json:"roles"`
			} `json:"profile"`
		}
		if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil {
			t.Fatalf("profile of %s: %d %s", subject, res.Code, res.Body.String())
		}
		return out.Profile.Depot, out.Profile.Roles
	}

	if depot, roles := me("north"); depot != "DEPOT_NORTH" || strings.Join(roles, ",") != "DISPATCHER" {
		t.Fatalf("a dispatcher assigned to a depot is told which: %q %v", depot, roles)
	}
	if depot, _ := me("all"); depot != "" {
		t.Fatalf("a dispatcher with no depot row covers every depot: %q", depot)
	}
	if depot, _ := me("null-depot"); depot != "" {
		t.Fatalf("a dispatcher whose depot is empty covers every depot: %q", depot)
	}
	if depot, _ := me("loader"); depot != "DEPOT_SOUTH" {
		t.Fatalf("loaders keep their depot: %q", depot)
	}
}
