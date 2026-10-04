package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	orderclient "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	orderhandler "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/service"
	orderstore "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

type bearerAuth struct{}

func (bearerAuth) Authenticate(r *http.Request) (*auth.Principal, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, errors.New("missing bearer token")
	}
	sub := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	if sub == "" {
		return nil, errors.New("missing bearer token")
	}
	return &auth.Principal{Subject: sub}, nil
}

func TestVerticalSlicePostgres(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "waypoint",
			"POSTGRES_PASSWORD": "waypoint",
			"POSTGRES_DB":       "waypoint",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
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
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0001_init.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0002_identity.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0003_orders.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0004_audit.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0012_delivery.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0014_order_receipts.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0025_order_import_keys.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0027_delivery_service_time_eval_index.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0032_tech_custody_ledger.sql"))

	sharedPool, err := db.Open(ctx, dsn, "shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sharedPool.Close)
	_, err = sharedPool.Exec(ctx, `
		INSERT INTO outlets (id, brand, name) VALUES
			('OUT034', 'Fresh', 'Fresh OUT034'),
			('OUT021', 'Style', 'Style OUT021');
		INSERT INTO users (id, identity_subject, role) VALUES
			('USR001', 'usr-store-manager', 'STORE_MANAGER'),
			('USR002', 'usr-dispatcher', 'DISPATCHER'),
			('USR003', 'usr-store-manager-b', 'STORE_MANAGER');
		INSERT INTO store_manager_profiles (user_id, outlet_id) VALUES
			('USR001', 'OUT034'),
			('USR003', 'OUT021');
	`)
	if err != nil {
		t.Fatal(err)
	}

	sharedSrv := httptest.NewServer(sharedStub(sharedPool))
	t.Cleanup(sharedSrv.Close)

	orderPool, err := db.Open(ctx, dsn, "orders")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(orderPool.Close)
	sharedClient := orderclient.Shared{BaseURL: sharedSrv.URL}
	deliveryReader := &receiptDeliveryReader{items: map[string]domain.DeliveryTracking{}}
	orderRepo := orderstore.Postgres{Pool: orderPool}
	techOrder, err := orderRepo.Create(domain.Order{OutletID: "OUT034", Brand: "Tech", RequestedDeliveryDate: "2026-09-29", OrderUnits: 1, OrderWeightKg: 1, OrderVolumeM3: .1, TemperatureRequirement: "ambient", CreatedBy: "USR002"})
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"LOADED", "DISPATCHED", "DELIVERED", "RECEIVED"} {
		event := domain.CustodyEvent{Stage: stage, SealID: "SEAL-TEST", SerialNumbers: []string{"SN-TEST"}, Condition: "intact", IdempotencyKey: "custody-" + stage, RecordedBy: "USR002"}
		if stage == "RECEIVED" {
			event.ReceiverName = "Outlet manager"
		}
		createdEvent, created, err := orderRepo.AddCustodyEvent(techOrder, event)
		if err != nil || !created {
			t.Fatalf("custody stage %s created=%v err=%v", stage, created, err)
		}
		if stage == "LOADED" {
			retry := event
			retry.IdempotencyKey = "custody-LOADED-retry"
			previous, replayCreated, replayErr := orderRepo.AddCustodyEvent(techOrder, retry)
			if replayErr != nil || replayCreated || previous.ID != createdEvent.ID {
				t.Fatalf("same-stage replay event=%+v created=%v err=%v", previous, replayCreated, replayErr)
			}
		}
	}
	if _, created, err := orderRepo.AddCustodyEvent(techOrder, domain.CustodyEvent{Stage: "RECEIVED", SealID: "SEAL-TEST", SerialNumbers: []string{"SN-TEST"}, Condition: "intact", ReceiverName: "Outlet manager", IdempotencyKey: "custody-RECEIVED", RecordedBy: "USR002"}); err != nil || created {
		t.Fatalf("duplicate custody event created=%v err=%v", created, err)
	}
	if events, err := orderRepo.ListCustodyEvents(techOrder.ID); err != nil || len(events) != 4 {
		t.Fatalf("custody events=%d err=%v", len(events), err)
	} else if _, err := orderPool.Exec(ctx, `UPDATE orders.tech_custody_events SET condition='altered' WHERE id=$1::uuid`, events[0].ID); err == nil {
		t.Fatal("custody event was mutable")
	}
	orderH := orderhandler.Handler{
		Authn:    bearerAuth{},
		Profiles: orderclient.Resolver{Shared: sharedClient},
		Service: service.Service{
			Repo:     orderstore.Postgres{Pool: orderPool},
			Outlets:  sharedClient,
			Audit:    noopAudit{},
			Planning: receiptPlanningReader{},
			Delivery: deliveryReader,
		},
	}
	orderR := chi.NewRouter()
	orderH.Routes(orderR)
	orderSrv := httptest.NewServer(orderR)
	t.Cleanup(orderSrv.Close)

	body, _ := json.Marshal(domain.CreateRequest{
		RequestedDeliveryDate:  "2026-09-29",
		OrderUnits:             20,
		OrderWeightKg:          185.5,
		OrderVolumeM3:          2.4,
		TemperatureRequirement: "chilled",
	})
	createReq, _ := http.NewRequest(http.MethodPost, orderSrv.URL+"/api/v1/orders", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer usr-store-manager")
	createReq.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create %d", res.StatusCode)
	}
	var created struct {
		Order domain.Order `json:"order"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Order.OrderRef, "ORD") || created.Order.OutletID != "OUT034" {
		t.Fatalf("created %+v", created.Order)
	}
	deliveryReader.items[created.Order.ID] = domain.DeliveryTracking{RunID: "11111111-1111-4111-8111-111111111111", TripID: "trip-1", RunStatus: "completed", StopID: "22222222-2222-4222-8222-222222222222", Outcome: "DELIVERED", CompletedAt: ptrTime(time.Now().Add(-time.Minute)), Proofs: []domain.ProofSummary{{Type: "SIGNATURE", MimeType: "image/png"}}}
	trackingBytes := doGET(t, orderSrv.URL+"/api/v1/orders/"+created.Order.ID+"/tracking", "usr-store-manager")
	if !bytes.Contains(trackingBytes, []byte("plannedArrivalAt")) || !bytes.Contains(trackingBytes, []byte("DELIVERED")) {
		t.Fatalf("tracking missing ETA or delivery outcome: %s", trackingBytes)
	}
	confirmBytes, _ := json.Marshal(map[string]any{"receivedUnits": created.Order.OrderUnits})
	confirmResult := doJSON(t, orderSrv.URL+"/api/v1/orders/"+created.Order.ID+"/receipt/confirm", "usr-store-manager", confirmBytes)
	if !bytes.Contains(confirmResult, []byte(`"status":"confirmed"`)) {
		t.Fatalf("receipt not confirmed: %s", confirmResult)
	}
	if duplicate := doJSON(t, orderSrv.URL+"/api/v1/orders/"+created.Order.ID+"/receipt/confirm", "usr-store-manager", confirmBytes); !bytes.Contains(duplicate, []byte("confirmed")) {
		t.Fatalf("duplicate confirmation failed: %s", duplicate)
	}

	partialReq, _ := http.NewRequest(http.MethodPost, orderSrv.URL+"/api/v1/orders", bytes.NewReader(body))
	partialReq.Header.Set("Authorization", "Bearer usr-store-manager")
	partialReq.Header.Set("Content-Type", "application/json")
	partialRes, err := http.DefaultClient.Do(partialReq)
	if err != nil {
		t.Fatal(err)
	}
	var partialCreated struct {
		Order domain.Order `json:"order"`
	}
	if err := json.NewDecoder(partialRes.Body).Decode(&partialCreated); err != nil {
		t.Fatal(err)
	}
	partialRes.Body.Close()
	if partialRes.StatusCode != http.StatusCreated {
		t.Fatalf("partial order create %d", partialRes.StatusCode)
	}
	deliveryReader.items[partialCreated.Order.ID] = domain.DeliveryTracking{RunID: "33333333-3333-4333-8333-333333333333", TripID: "trip-2", RunStatus: "completed", StopID: "44444444-4444-4444-8444-444444444444", Outcome: "PARTIAL", CompletedAt: ptrTime(time.Now().Add(-time.Minute))}
	partialConfirmation, _ := json.Marshal(map[string]any{"receivedUnits": 16, "issue": map[string]any{"issueType": "MISSING", "affectedUnits": 4, "note": "Four units short", "idempotencyKey": "m6-short-1"}})
	issueResult := doJSON(t, orderSrv.URL+"/api/v1/orders/"+partialCreated.Order.ID+"/receipt/confirm", "usr-store-manager", partialConfirmation)
	if !bytes.Contains(issueResult, []byte("confirmed_with_issue")) {
		t.Fatalf("partial receipt not recorded: %s", issueResult)
	}
	dispatcherIssues := doGET(t, orderSrv.URL+"/api/v1/orders/receipt-issues", "usr-dispatcher")
	if !bytes.Contains(dispatcherIssues, []byte("MISSING")) || !bytes.Contains(dispatcherIssues, []byte(partialCreated.Order.OrderRef)) {
		t.Fatalf("dispatcher cannot see issue: %s", dispatcherIssues)
	}

	smList := doGET(t, orderSrv.URL+"/api/v1/orders", "usr-store-manager")
	if !bytes.Contains(smList, []byte(created.Order.OrderRef)) {
		t.Fatalf("store manager list missing order: %s", smList)
	}
	dispList := doGET(t, orderSrv.URL+"/api/v1/orders", "usr-dispatcher")
	if !bytes.Contains(dispList, []byte(created.Order.OrderRef)) {
		t.Fatalf("dispatcher list missing order: %s", dispList)
	}
	getReq, _ := http.NewRequest(http.MethodGet, orderSrv.URL+"/api/v1/orders/"+created.Order.ID, nil)
	getReq.Header.Set("Authorization", "Bearer usr-store-manager-b")
	getRes, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getRes.Body.Close()
	if getRes.StatusCode != http.StatusForbidden {
		t.Fatalf("store-manager-b get %d", getRes.StatusCode)
	}
	importURL := orderSrv.URL + "/api/v1/orders/import.csv?version=1&sourceSystem=erp_a"
	csvBody := "version,external_order_id,outlet_id,brand,requested_delivery_date,order_units,order_weight_kg,order_volume_m3,temperature_requirement\n1,ERP-100,OUT034,Fresh,2026-10-05,10,20,1,ambient\n"
	importReq, _ := http.NewRequest(http.MethodPost, importURL, strings.NewReader(csvBody))
	importReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	importReq.Header.Set("Content-Type", "text/csv")
	importRes, err := http.DefaultClient.Do(importReq)
	if err != nil {
		t.Fatal(err)
	}
	importBody, _ := io.ReadAll(importRes.Body)
	importRes.Body.Close()
	if importRes.StatusCode != http.StatusOK || !bytes.Contains(importBody, []byte(`"created":1`)) {
		t.Fatalf("CSV import %d %s", importRes.StatusCode, importBody)
	}
	importReq, _ = http.NewRequest(http.MethodPost, importURL, strings.NewReader(csvBody))
	importReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	importReq.Header.Set("Content-Type", "text/csv")
	importRes, err = http.DefaultClient.Do(importReq)
	if err != nil {
		t.Fatal(err)
	}
	importBody, _ = io.ReadAll(importRes.Body)
	importRes.Body.Close()
	if importRes.StatusCode != http.StatusOK || !bytes.Contains(importBody, []byte(`"duplicates":1`)) {
		t.Fatalf("CSV replay %d %s", importRes.StatusCode, importBody)
	}
	conflictCSV := strings.Replace(csvBody, ",10,20,1,ambient", ",11,20,1,ambient", 1)
	importReq, _ = http.NewRequest(http.MethodPost, importURL, strings.NewReader(conflictCSV))
	importReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	importReq.Header.Set("Content-Type", "text/csv")
	importRes, err = http.DefaultClient.Do(importReq)
	if err != nil {
		t.Fatal(err)
	}
	importBody, _ = io.ReadAll(importRes.Body)
	importRes.Body.Close()
	if importRes.StatusCode != http.StatusConflict {
		t.Fatalf("CSV conflicting replay %d %s", importRes.StatusCode, importBody)
	}
	// A conflicting later row must roll back an earlier new row in the same
	// import batch; otherwise callers cannot safely retry the whole file.
	rollbackCSV := strings.Replace(csvBody,
		"1,ERP-100,OUT034,Fresh,2026-10-05,10,20,1,ambient\n",
		"1,ERP-ROLLBACK-1,OUT034,Fresh,2026-10-05,5,10,0.5,ambient\n"+
			"1,ERP-100,OUT034,Fresh,2026-10-05,11,20,1,ambient\n", 1)
	rollbackReq, _ := http.NewRequest(http.MethodPost, importURL, strings.NewReader(rollbackCSV))
	rollbackReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	rollbackReq.Header.Set("Content-Type", "text/csv")
	rollbackRes, err := http.DefaultClient.Do(rollbackReq)
	if err != nil {
		t.Fatal(err)
	}
	rollbackBody, _ := io.ReadAll(rollbackRes.Body)
	rollbackRes.Body.Close()
	if rollbackRes.StatusCode != http.StatusConflict {
		t.Fatalf("CSV batch conflict %d %s", rollbackRes.StatusCode, rollbackBody)
	}
	rollbackExport := doGET(t, orderSrv.URL+"/api/v1/orders/export.csv", "usr-dispatcher")
	if bytes.Contains(rollbackExport, []byte("ERP-ROLLBACK-1")) {
		t.Fatalf("CSV batch partially committed before conflict: %s", rollbackExport)
	}
	duplicateCSV := strings.Replace(csvBody,
		"1,ERP-100,OUT034,Fresh,2026-10-05,10,20,1,ambient\n",
		"1,ERP-DUPLICATE-IN-FILE,OUT034,Fresh,2026-10-05,5,10,0.5,ambient\n"+
			"1,ERP-DUPLICATE-IN-FILE,OUT034,Fresh,2026-10-05,5,10,0.5,ambient\n", 1)
	duplicateReq, _ := http.NewRequest(http.MethodPost, importURL, strings.NewReader(duplicateCSV))
	duplicateReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	duplicateReq.Header.Set("Content-Type", "text/csv")
	duplicateRes, err := http.DefaultClient.Do(duplicateReq)
	if err != nil {
		t.Fatal(err)
	}
	duplicateBody, _ := io.ReadAll(duplicateRes.Body)
	duplicateRes.Body.Close()
	if duplicateRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("CSV duplicate in-file key %d %s", duplicateRes.StatusCode, duplicateBody)
	}
	apiReplay := []byte(`{"version":1,"sourceSystem":"erp_a","orders":[{"externalOrderId":"ERP-100","outletId":"OUT034","brand":"Fresh","requestedDeliveryDate":"2026-10-05","orderUnits":10,"orderWeightKg":20,"orderVolumeM3":1,"temperatureRequirement":"ambient"}]}`)
	apiResult := doJSON(t, orderSrv.URL+"/api/v1/orders/import", "usr-dispatcher", apiReplay)
	if !bytes.Contains(apiResult, []byte(`"duplicates":1`)) {
		t.Fatalf("API import replay %s", apiResult)
	}
	apiExport := doGET(t, orderSrv.URL+"/api/v1/orders/export?version=1", "usr-dispatcher")
	if !bytes.Contains(apiExport, []byte(`"version":1`)) || !bytes.Contains(apiExport, []byte("ERP-100")) {
		t.Fatalf("API export %s", apiExport)
	}
	exportReq, _ := http.NewRequest(http.MethodGet, orderSrv.URL+"/api/v1/orders/export.csv", nil)
	exportReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	exportRes, err := http.DefaultClient.Do(exportReq)
	if err != nil {
		t.Fatal(err)
	}
	exported, _ := io.ReadAll(exportRes.Body)
	exportRes.Body.Close()
	if exportRes.StatusCode != http.StatusOK || !bytes.Contains(exported, []byte("external_order_id")) || !bytes.Contains(exported, []byte("ERP-100")) {
		t.Fatalf("CSV export %d %s", exportRes.StatusCode, exported)
	}

	// The forecast uses operational depot, service allowance, and fleet sources
	// and remains explicit about its four-complete-week history and ten-week horizon.
	_, err = sharedPool.Exec(ctx, `ALTER TABLE shared.outlets ADD COLUMN depot TEXT NOT NULL DEFAULT '';
		CREATE TABLE shared.service_allowance(stop_kind TEXT PRIMARY KEY, minutes INTEGER NOT NULL);
		INSERT INTO shared.service_allowance VALUES ('normal', 18), ('mall', 30);
		CREATE TABLE shared.planning_policy_versions(version INTEGER PRIMARY KEY, max_trips_per_vehicle INTEGER NOT NULL);
		CREATE TABLE shared.planning_policy_current(singleton BOOLEAN PRIMARY KEY, version INTEGER NOT NULL);
		INSERT INTO shared.planning_policy_versions VALUES (1,2);
		INSERT INTO shared.planning_policy_current VALUES (true,1);
		ALTER TABLE shared.outlets ALTER COLUMN depot SET DEFAULT 'DEPOT_NORTH';
		UPDATE shared.outlets SET depot='DEPOT_NORTH' WHERE id IN ('OUT034','OUT021');
		CREATE TABLE fleet.vehicles(vehicle_id TEXT PRIMARY KEY,home_depot TEXT NOT NULL,weight_cap_kg NUMERIC NOT NULL,volume_cap_m3 NUMERIC NOT NULL);
		INSERT INTO fleet.vehicles VALUES ('VEH001','DEPOT_NORTH',1000,10);`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = orderPool.Exec(ctx, `INSERT INTO orders.orders(order_ref,outlet_id,brand,requested_delivery_date,order_units,order_weight_kg,order_volume_m3,temperature_requirement,status,created_by)
		VALUES ('FC001','OUT034','Fresh',$1,10,20,2,'chilled','confirmed','test'),('FC002','OUT034','Fresh',$2,10,30,3,'ambient','confirmed','test'),('FC003','OUT021','Style',$1,10,40,4,'ambient','confirmed','test')`, time.Now().AddDate(0, 0, -10).Format("2006-01-02"), time.Now().AddDate(0, 0, -17).Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	colombo, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	localNow := now.In(colombo)
	weekStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, colombo).AddDate(0, 0, -int((int(localNow.Weekday())+6)%7))
	observedAt := weekStart.AddDate(0, 0, -10)
	var runID string
	err = orderPool.QueryRow(ctx, `INSERT INTO delivery.runs(trip_id,plan_id,plan_ref,delivery_date,vehicle_id,depot,trip_number,status)
		VALUES('forecast-service-time-run','forecast-plan','forecast-ref',$1,'forecast-vehicle','DEPOT_NORTH',1,'completed') RETURNING id::text`, observedAt.Format("2006-01-02")).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		arrived := observedAt.Add(time.Duration(i) * time.Hour)
		duration := time.Duration(20+(i%3)*4) * time.Minute
		_, err = orderPool.Exec(ctx, `INSERT INTO delivery.stops(run_id,allocation_id,order_id,order_ref,outlet_id,brand,stop_sequence,status,arrived_at,outcome_code,outcome_at)
			VALUES($1::uuid,$2,$3,$4,'OUT034','Fresh',$5,'completed',$6,'DELIVERED',$7)`, runID, fmt.Sprintf("allocation-%d", i), fmt.Sprintf("order-%d", i), fmt.Sprintf("ref-%d", i), i+1, arrived, arrived.Add(duration))
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		arrived := observedAt.Add(time.Duration(12+i) * time.Hour)
		_, err = orderPool.Exec(ctx, `INSERT INTO delivery.stops(run_id,allocation_id,order_id,order_ref,outlet_id,brand,stop_sequence,status,arrived_at,outcome_code,outcome_at)
			VALUES($1::uuid,$2,$3,$4,'OUT021','Style',$5,'completed',$6,'DELIVERED',$7)`, runID, fmt.Sprintf("sparse-allocation-%d", i), fmt.Sprintf("sparse-order-%d", i), fmt.Sprintf("sparse-ref-%d", i), i+13, arrived, arrived.Add(18*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
	}
	f, err := (orderstore.Postgres{Pool: orderPool}).Forecast(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Weekly) != 20 || f.HorizonWeeks != 10 || f.Weekly[19].HorizonWeek != 10 || f.Weekly[19].WeekStarting != weekStart.AddDate(0, 0, 63).Format("2006-01-02") || f.Weekly[19].RangePercent != 28 || len(f.Weekly[0].WeekStarting) != 10 || f.Weekly[0].EstimatedWeightKg != 12.5 || f.Weekly[0].EstimatedVolumeM3 != 1.25 || f.ServiceMinutesPerStop != 24 || f.ServiceEstimateVersion != "configured_allowance_mean_v1" || f.ServiceTimeBacktestVersion != "configured_service_time_mae_v1" || f.ServiceTimeBacktestWindowStart != weekStart.AddDate(0, 0, -28).Format("2006-01-02") || f.ServiceTimeBacktestWindowEnd != weekStart.Format("2006-01-02") || len(f.ServiceTimeEvaluation) != 2 || f.ServiceTimeEvaluation[0].Brand != "Fresh" || f.ServiceTimeEvaluation[0].ActualStopCount != 12 || f.ServiceTimeEvaluation[0].Status != "EVALUATED" || f.ServiceTimeEvaluation[0].MeanObservedMinutes == nil || *f.ServiceTimeEvaluation[0].MeanObservedMinutes != 24 || f.ServiceTimeEvaluation[0].MeanAbsoluteErrorMinutes == nil || *f.ServiceTimeEvaluation[0].MeanAbsoluteErrorMinutes != 2.7 || f.ServiceTimeEvaluation[1].Brand != "Style" || f.ServiceTimeEvaluation[1].ActualStopCount != 2 || f.ServiceTimeEvaluation[1].Status != "INSUFFICIENT_HISTORY" || f.ServiceTimeEvaluation[1].MeanAbsoluteErrorMinutes != nil || f.ForecastVersion != "confirmed_order_mean_v2" || f.DriftModelVersion != "weekly_order_shift_v1" || f.BacktestModelVersion != "prior_four_week_order_count_ape_v1" || len(f.InputDrift) != 2 || f.InputDrift[0].Status != "INSUFFICIENT_HISTORY" || f.InputDrift[0].BacktestAPEPercent != nil || len(f.Capacity) != 1 || f.Capacity[0].Pressure != "low" || f.Capacity[0].ProjectedWeightKg != 22.5 {
		t.Fatalf("unexpected forecast estimate: %+v", f)
	}
}

type noopAudit struct{}

func (noopAudit) PublishCreated(string, string, string, map[string]any) error { return nil }

func sharedStub(pool *pgxpool.Pool) http.Handler {
	authn := bearerAuth{}
	r := chi.NewRouter()
	r.Get("/api/v1/shared/profiles/me", func(w http.ResponseWriter, req *http.Request) {
		p, err := authn.Authenticate(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var userID, subject, role, outlet string
		err = pool.QueryRow(req.Context(), `
			SELECT u.id, u.identity_subject, u.role, COALESCE(p.outlet_id, '')
			FROM users u LEFT JOIN store_manager_profiles p ON p.user_id = u.id
			WHERE u.identity_subject = $1
		`, p.Subject).Scan(&userID, &subject, &role, &outlet)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		outlets := []string{}
		if outlet != "" {
			outlets = []string{outlet}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"profile": map[string]any{"userId": userID, "subject": subject, "roles": []string{role}, "outletIds": outlets},
		})
	})
	r.Get("/api/v1/shared/outlets/{id}", func(w http.ResponseWriter, req *http.Request) {
		if _, err := authn.Authenticate(req); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(req, "id")
		var brand, name string
		err := pool.QueryRow(req.Context(), `SELECT brand, name FROM outlets WHERE id = $1`, id).Scan(&brand, &name)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"outlet": map[string]string{"id": id, "brand": brand, "name": name}})
	})
	return r
}

func doGET(t *testing.T, url, subject string) []byte {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+subject)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s %d %s", url, res.StatusCode, buf.String())
	}
	return buf.Bytes()
}

func doJSON(t *testing.T, url, subject string, body []byte) []byte {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+subject)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("POST %s status=%d body=%s", url, res.StatusCode, b)
	}
	return b
}

type receiptPlanningReader struct{}

func (receiptPlanningReader) ByOrder(string) (domain.PlanningTracking, error) {
	eta := time.Date(2026, 9, 30, 7, 0, 0, 0, time.UTC)
	return domain.PlanningTracking{State: "PLANNED", PlanID: "plan-1", PlanRef: "PLAN000001", TripID: "trip-1", StopSequence: 1, PlannedArrivalAt: &eta}, nil
}

type receiptDeliveryReader struct {
	items map[string]domain.DeliveryTracking
}

func (r *receiptDeliveryReader) ByOrder(id string) (domain.DeliveryTracking, error) {
	v, ok := r.items[id]
	if !ok {
		return domain.DeliveryTracking{}, fmt.Errorf("not found")
	}
	return v, nil
}
func ptrTime(t time.Time) *time.Time { return &t }

func applySQL(t *testing.T, ctx context.Context, dsn, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, string(b)); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}
