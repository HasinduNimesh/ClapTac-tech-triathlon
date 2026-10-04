package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/objectstore"
	delclient "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/client"
	delhandler "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/handler"
	delservice "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/service"
	delstore "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/store"
)

type arrivalNotice struct {
	EventKey     string    `json:"eventKey"`
	OutletID     string    `json:"outletId"`
	OrderRef     string    `json:"orderRef"`
	Type         string    `json:"type"`
	OldArrivalAt time.Time `json:"oldArrivalAt"`
	NewArrivalAt time.Time `json:"newArrivalAt"`
}

// arrivalPeerStub is a loading/shared stub for a three-stop trip with published planned arrival and
// departure times, recording every notification enqueue request it receives.
type arrivalPeerStub struct {
	mu      sync.Mutex
	notices []arrivalNotice
}

func (p *arrivalPeerStub) recorded() []arrivalNotice {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]arrivalNotice(nil), p.notices...)
}

func (p *arrivalPeerStub) handler() http.Handler {
	colombo := time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	plan := func(h, m int) string { return time.Date(2026, 9, 29, h, m, 0, 0, colombo).Format(time.RFC3339) }
	trip := map[string]any{
		"tripId": "trip-eta", "planId": "plan-1", "planRef": "PLAN000001", "deliveryDate": "2026-09-29",
		"vehicleId": "VEH001", "vehicleType": "truck", "vehicleTemperatureCapability": "reefer",
		"depot": "DEPOT_NORTH", "tripNumber": 1, "loadingStatus": "ready",
		"planVersion": 1, "planAcknowledgements": []map[string]any{{"actorId": "USR006", "actorRole": "DRIVER"}},
		"orders": []map[string]any{
			{"allocationId": "a1", "orderId": "ord-1", "orderRef": "ORD000001", "outletId": "OUT034", "brand": "Fresh", "stopSequence": 1, "loadingStatus": "loaded", "temperatureRequirement": "ambient", "expectedUnits": 10, "shortfallSummary": []any{}, "plannedArrivalAt": plan(8, 0), "plannedDepartureAt": plan(8, 20)},
			{"allocationId": "a2", "orderId": "ord-2", "orderRef": "ORD000002", "outletId": "OUT021", "brand": "Fresh", "stopSequence": 2, "loadingStatus": "loaded", "temperatureRequirement": "ambient", "expectedUnits": 8, "shortfallSummary": []any{}, "plannedArrivalAt": plan(8, 40), "plannedDepartureAt": plan(9, 0)},
			{"allocationId": "a3", "orderId": "ord-3", "orderRef": "ORD000003", "outletId": "OUT055", "brand": "Fresh", "stopSequence": 3, "loadingStatus": "loaded", "temperatureRequirement": "ambient", "expectedUnits": 6, "shortfallSummary": []any{}, "plannedArrivalAt": plan(9, 20), "plannedDepartureAt": plan(9, 40)},
		},
	}
	r := chi.NewRouter()
	r.Get("/api/v1/loading/internal/trips/{tripId}", func(w http.ResponseWriter, req *http.Request) {
		if chi.URLParam(req, "tripId") != "trip-eta" {
			http.NotFound(w, req)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, trip)
	})
	r.Get("/api/v1/shared/outlets/{id}", func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"outlet": map[string]any{
			"id": chi.URLParam(req, "id"), "name": "Outlet", "district": "Colombo", "dockType": "normal",
			"parkingConstraint": "normal", "windowOpenTime": "08:00:00", "windowCloseTime": "23:00:00",
		}})
	})
	r.Post("/api/v1/shared/audit-events", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "ingested"})
	})
	r.Post("/api/v1/shared/internal/notifications/enqueue", func(w http.ResponseWriter, req *http.Request) {
		var notice arrivalNotice
		_ = json.NewDecoder(req.Body).Decode(&notice)
		p.mu.Lock()
		p.notices = append(p.notices, notice)
		p.mu.Unlock()
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"notification": map[string]any{"status": "enqueued"}})
	})
	return r
}

// TestDriverEventsPublishArrivalPredictionsAndNotifyTheStore drives a run the way a driver does and checks
// the W9 chain without any dispatcher request: a late arrival re-projects the stops that follow, the new
// predictions are persisted, the shared service is asked to tell each store the old and new arrival, and
// repeating an event that changes nothing does not notify again.
func TestDriverEventsPublishArrivalPredictionsAndNotifyTheStore(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env:          map[string]string{"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint"},
		WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
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
	root := repoRoot(t)
	for _, migration := range []string{
		"0001_init.sql", "0002_identity.sql", "0005_outlets_expand.sql", "0009_loading.sql", "0012_delivery.sql",
		"0017_master_data_versions.sql", "0022_delivery_plan_version.sql", "0024_trip_messages.sql",
		"0028_delivery_proof_retention.sql", "0029_delivery_lateness_history_index.sql", "0034_delivery_planned_arrival_snapshot.sql",
		"0030_outlet_access_instructions.sql", "0031_cold_chain_readings.sql", "0038_delivery_proof_receiver_name.sql",
		"0039_delivery_driver_incidents.sql", "0040_delivery_stop_expected_units.sql", "0041_delivery_outcome_units.sql",
		"0041_delivery_checkout.sql", "0043_delivery_arrival_predictions.sql", "0044_delivery_returns.sql",
	} {
		applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", migration))
	}

	stub := &arrivalPeerStub{}
	peers := httptest.NewServer(stub.handler())
	t.Cleanup(peers.Close)
	pool, err := db.Open(ctx, dsn, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	svc := delservice.Service{
		Repo:    delstore.Postgres{Pool: pool},
		Peers:   delclient.Peers{LoadingURL: peers.URL, SharedURL: peers.URL, OrdersURL: peers.URL, M2M: staticToken("svc-delivery")},
		Objects: &objectstore.Memory{},
	}
	r := chi.NewRouter()
	delhandler.Handler{Authn: bearerAuth{}, Profiles: staticProfiles{}, Service: svc}.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	// Let in-flight background refreshes finish before the database goes away.
	t.Cleanup(svc.WaitForArrivalPredictions)

	const base = "/api/v1/delivery/trips/trip-eta"
	if res := do(t, srv, http.MethodPost, base+"/prepare", "usr-driver", nil, ""); res.status != http.StatusOK {
		t.Fatalf("prepare %d %s", res.status, res.body)
	}
	if res := do(t, srv, http.MethodPost, base+"/checkout", "usr-driver", []byte(`{"planVersion":1,"confirmedOrderIds":["ord-1","ord-2","ord-3"]}`), ""); res.status != http.StatusOK {
		t.Fatalf("checkout %d %s", res.status, res.body)
	}
	started := do(t, srv, http.MethodPost, base+"/start", "usr-driver", nil, "eta-start")
	if started.status != http.StatusOK {
		t.Fatalf("start %d %s", started.status, started.body)
	}
	var detail struct {
		Stops []struct {
			ID string `json:"id"`
		} `json:"stops"`
	}
	if err := json.Unmarshal([]byte(started.body), &detail); err != nil || len(detail.Stops) != 3 {
		t.Fatalf("started %v %s", err, started.body)
	}
	stop1, stop2, stop3 := detail.Stops[0].ID, detail.Stops[1].ID, detail.Stops[2].ID
	svc.WaitForArrivalPredictions()
	if n := len(stub.recorded()); n != 0 {
		t.Fatalf("no driver event yet, but %d notices were queued", n)
	}

	colombo := time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	clock := func(h, m int) time.Time { return time.Date(2026, 9, 29, h, m, 0, 0, colombo) }
	type prediction struct {
		current, communicated time.Time
		source                *time.Time
		sequence              int
	}
	stored := func(stopID string) *prediction {
		t.Helper()
		var p prediction
		err := pool.QueryRow(ctx, `SELECT current_eta,communicated_eta,source_event_at,notice_sequence FROM delivery.arrival_predictions WHERE stop_id=$1::uuid`, stopID).
			Scan(&p.current, &p.communicated, &p.source, &p.sequence)
		if err != nil {
			return nil
		}
		return &p
	}
	sameInstant := func(label string, got, want time.Time) {
		t.Helper()
		if !got.Equal(want) {
			t.Fatalf("%s: got %s want %s", label, got.In(colombo), want.In(colombo))
		}
	}

	// 1. The driver reaches stop 1 an hour late (planned 08:00, here 09:00). With the 20-minute service
	// allowance it is projected to leave at 09:20 against a planned 08:20: 60 minutes of delay.
	if res := do(t, srv, http.MethodPost, base+"/stops/"+stop1+"/arrive", "usr-driver", []byte(`{"occurredAt":"`+clock(9, 0).Format(time.RFC3339)+`"}`), "eta-arrive-1"); res.status != http.StatusOK {
		t.Fatalf("arrive %d %s", res.status, res.body)
	}
	svc.WaitForArrivalPredictions()
	p2, p3 := stored(stop2), stored(stop3)
	if p2 == nil || p3 == nil {
		t.Fatalf("predictions were not persisted: stop2=%v stop3=%v", p2, p3)
	}
	sameInstant("stop 2 current ETA", p2.current, clock(9, 40))
	sameInstant("stop 2 told to the store", p2.communicated, clock(9, 40))
	sameInstant("stop 3 current ETA", p3.current, clock(10, 20))
	if p2.sequence != 1 || p3.sequence != 1 {
		t.Fatalf("each stop should have one notice so far: %d %d", p2.sequence, p3.sequence)
	}
	if stored(stop1) != nil {
		t.Fatal("the stop being served has no downstream prediction")
	}
	notices := stub.recorded()
	if len(notices) != 2 {
		t.Fatalf("expected one ARRIVAL_CHANGE per delayed stop, got %+v", notices)
	}
	byOutlet := map[string]arrivalNotice{}
	for _, n := range notices {
		if n.Type != "ARRIVAL_CHANGE" || n.EventKey == "" {
			t.Fatalf("notice %+v", n)
		}
		byOutlet[n.OutletID] = n
	}
	n2, n3 := byOutlet["OUT021"], byOutlet["OUT055"]
	if n2.OrderRef != "ORD000002" || n3.OrderRef != "ORD000003" {
		t.Fatalf("notices went to the wrong stops: %+v", notices)
	}
	sameInstant("stop 2 notice old", n2.OldArrivalAt, clock(8, 40))
	sameInstant("stop 2 notice new", n2.NewArrivalAt, clock(9, 40))
	sameInstant("stop 3 notice old", n3.OldArrivalAt, clock(9, 20))
	sameInstant("stop 3 notice new", n3.NewArrivalAt, clock(10, 20))

	// 2. The driver then records stop 1's outcome at exactly the projected 09:20. The projection does not
	// move, so nothing is re-sent, even though the event itself is new.
	if res := do(t, srv, http.MethodPost, base+"/stops/"+stop1+"/outcome", "usr-driver", []byte(`{"code":"NOT_DELIVERED","reason":"OUTLET_CLOSED","occurredAt":"`+clock(9, 20).Format(time.RFC3339)+`"}`), "eta-out-1"); res.status != http.StatusOK {
		t.Fatalf("outcome %d %s", res.status, res.body)
	}
	svc.WaitForArrivalPredictions()
	if n := len(stub.recorded()); n != 2 {
		t.Fatalf("an unchanged projection must not notify again: %+v", stub.recorded())
	}
	after := stored(stop2)
	if after.source == nil || !after.source.Equal(clock(9, 20)) || after.sequence != 1 {
		t.Fatalf("the prediction should now rest on the outcome event without a new notice: %+v", after)
	}

	// 3. Stop 2 is reached 100 minutes late (10:20 vs planned 08:40), so it is projected to leave at 10:40 against 09:00. Stop 3 moves from 10:20 to 11:00, so
	// its store is told the old and new time again; stop 2 itself has been reached and is not re-projected.
	if res := do(t, srv, http.MethodPost, base+"/stops/"+stop2+"/arrive", "usr-driver", []byte(`{"occurredAt":"`+clock(10, 20).Format(time.RFC3339)+`"}`), "eta-arrive-2"); res.status != http.StatusOK {
		t.Fatalf("arrive 2 %d %s", res.status, res.body)
	}
	svc.WaitForArrivalPredictions()
	notices = stub.recorded()
	if len(notices) != 3 {
		t.Fatalf("expected a third notice for stop 3 only, got %+v", notices)
	}
	last := notices[2]
	if last.OutletID != "OUT055" || last.EventKey == n3.EventKey {
		t.Fatalf("third notice %+v", last)
	}
	sameInstant("stop 3 second notice old", last.OldArrivalAt, clock(10, 20))
	sameInstant("stop 3 second notice new", last.NewArrivalAt, clock(11, 0))
	if kept := stored(stop2); kept.sequence != 1 || !kept.current.Equal(clock(9, 40)) {
		t.Fatalf("a reached stop's prediction is left as it was: %+v", kept)
	}

	// 4. The same arrival again (a retry from an offline queue) is a replay: nothing changes.
	if res := do(t, srv, http.MethodPost, base+"/stops/"+stop2+"/arrive", "usr-driver", []byte(`{"occurredAt":"`+clock(10, 20).Format(time.RFC3339)+`"}`), "eta-arrive-2-retry"); res.status != http.StatusOK {
		t.Fatalf("arrive retry %d %s", res.status, res.body)
	}
	svc.WaitForArrivalPredictions()
	if n := len(stub.recorded()); n != 3 {
		t.Fatalf("a replayed event notified again: %+v", stub.recorded())
	}
}
