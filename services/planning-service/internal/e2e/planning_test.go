package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	planclient "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/client"
	planhandler "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/handler"
	planservice "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/service"
	planstore "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/store"
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
	p := &auth.Principal{Subject: sub}
	if strings.HasPrefix(sub, "svc-") {
		p.Scopes = []string{
			authorization.PermPlansReadInternal,
			authorization.PermOrdersReadInternal,
			authorization.PermFleetReadInternal,
			authorization.PermOutletsReadInternal,
			authorization.PermAuditWrite,
		}
	}
	return p, nil
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

func TestPlanningGenerateConfirm(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}
	pg, err := startPostgresContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
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
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0007_planning.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0013_planning_stop_times.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0020_planning_publications.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0035_planning_disruption_risks.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0036_planning_unallocated_reasons.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0037_planning_deferral_next_run.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0073_planning_ack_trip_identity.sql"))

	peers := httptest.NewServer(peerStub())
	t.Cleanup(peers.Close)

	planPool, err := db.Open(ctx, dsn, "planning")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(planPool.Close)
	client := planclient.Peers{
		OrdersURL: peers.URL, FleetURL: peers.URL, SharedURL: peers.URL, DeliveryURL: peers.URL,
		M2M: staticToken("svc-planning"),
	}
	planH := planhandler.Handler{
		Authn:    bearerAuth{},
		Profiles: staticProfiles{},
		Service:  planservice.Service{Repo: planstore.Postgres{Pool: planPool}, Peers: client},
	}
	planR := chi.NewRouter()
	planH.Routes(planR)
	planSrv := httptest.NewServer(planR)
	t.Cleanup(planSrv.Close)

	riskRequest, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/disruption-risks", strings.NewReader(`{"deliveryDate":"2026-10-01","scope":"DISTRICT","scopeKey":"Colombo North","riskType":"HEAVY_RAIN","severity":"HIGH","summary":"Flooding reported near bridge","source":"Manual dispatcher report","sourceReference":"incident-42","confidence":0.8}`))
	riskRequest.Header.Set("Authorization", "Bearer usr-dispatcher")
	riskRequest.Header.Set("Content-Type", "application/json")
	riskResponse, err := http.DefaultClient.Do(riskRequest)
	if err != nil {
		t.Fatal(err)
	}
	var createdRisk struct {
		ID string `json:"id"`
	}
	if riskResponse.StatusCode != http.StatusCreated {
		body := readBody(riskResponse)
		riskResponse.Body.Close()
		t.Fatalf("create disruption risk returned %d: %s", riskResponse.StatusCode, body)
	}
	if err := json.NewDecoder(riskResponse.Body).Decode(&createdRisk); err != nil {
		riskResponse.Body.Close()
		t.Fatal(err)
	}
	riskResponse.Body.Close()
	if createdRisk.ID == "" {
		t.Fatal("created disruption risk has no id")
	}
	getRisks, _ := http.NewRequest(http.MethodGet, planSrv.URL+"/api/v1/planning/disruption-risks?date=2026-10-01", nil)
	getRisks.Header.Set("Authorization", "Bearer usr-dispatcher")
	riskList, err := http.DefaultClient.Do(getRisks)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(riskList.Body).Decode(&listed); err != nil {
		riskList.Body.Close()
		t.Fatal(err)
	}
	riskList.Body.Close()
	if riskList.StatusCode != http.StatusOK || len(listed.Items) != 1 || listed.Items[0]["source"] != "Manual dispatcher report" {
		t.Fatalf("disruption risk list returned %d with %+v", riskList.StatusCode, listed.Items)
	}
	overrideRequest, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/disruption-risks/"+createdRisk.ID+"/override", strings.NewReader(`{"decision":"OVERRIDE","severity":"MEDIUM","reason":"Local road authority confirms passable route"}`))
	overrideRequest.Header.Set("Authorization", "Bearer usr-dispatcher")
	overrideRequest.Header.Set("Content-Type", "application/json")
	overrideResponse, err := http.DefaultClient.Do(overrideRequest)
	if err != nil {
		t.Fatal(err)
	}
	var overridden struct {
		OverrideDecision string `json:"overrideDecision"`
		OverrideSeverity string `json:"overrideSeverity"`
		OverrideReason   string `json:"overrideReason"`
	}
	if err := json.NewDecoder(overrideResponse.Body).Decode(&overridden); err != nil {
		overrideResponse.Body.Close()
		t.Fatal(err)
	}
	overrideResponse.Body.Close()
	if overrideResponse.StatusCode != http.StatusOK || overridden.OverrideDecision != "OVERRIDE" || overridden.OverrideSeverity != "MEDIUM" || overridden.OverrideReason != "Local road authority confirms passable route" {
		t.Fatalf("override returned %d with %+v", overrideResponse.StatusCode, overridden)
	}
	if _, err := planPool.Exec(ctx, `UPDATE planning.disruption_risks SET severity='LOW' WHERE id=$1::uuid`, createdRisk.ID); err == nil {
		t.Fatal("disruption risks must be append-only")
	}
	if _, err := planPool.Exec(ctx, `DELETE FROM planning.disruption_risk_overrides WHERE risk_id=$1::uuid`, createdRisk.ID); err == nil {
		t.Fatal("disruption risk override history must be append-only")
	}

	createPlan, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans", bytes.NewReader([]byte(`{"deliveryDate":"2026-09-29"}`)))
	createPlan.Header.Set("Authorization", "Bearer usr-dispatcher")
	createPlan.Header.Set("Content-Type", "application/json")
	pres, err := http.DefaultClient.Do(createPlan)
	if err != nil {
		t.Fatal(err)
	}
	defer pres.Body.Close()
	if pres.StatusCode != http.StatusCreated {
		t.Fatalf("create plan %d %s", pres.StatusCode, readBody(pres))
	}
	var created struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	if err := json.NewDecoder(pres.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	forbidden, _ := http.NewRequest(http.MethodGet, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID, nil)
	forbidden.Header.Set("Authorization", "Bearer usr-store-manager")
	fres, err := http.DefaultClient.Do(forbidden)
	if err != nil {
		t.Fatal(err)
	}
	defer fres.Body.Close()
	if fres.StatusCode != http.StatusForbidden {
		t.Fatalf("store manager view %d", fres.StatusCode)
	}

	type generateResponse struct {
		status int
		body   string
	}
	const concurrentGenerators = 8
	start := make(chan struct{})
	responses := make(chan generateResponse, concurrentGenerators)
	var generators sync.WaitGroup
	for i := 0; i < concurrentGenerators; i++ {
		generators.Add(1)
		go func() {
			defer generators.Done()
			<-start
			gen, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID+"/generate", nil)
			gen.Header.Set("Authorization", "Bearer usr-dispatcher")
			gres, err := http.DefaultClient.Do(gen)
			if err != nil {
				responses <- generateResponse{status: http.StatusInternalServerError, body: err.Error()}
				return
			}
			defer gres.Body.Close()
			responses <- generateResponse{status: gres.StatusCode, body: readBody(gres)}
		}()
	}
	close(start)
	go func() { generators.Wait(); close(responses) }()
	var successfulGeneration *generateResponse
	conflictingGenerations := 0
	for response := range responses {
		switch response.status {
		case http.StatusOK:
			if successfulGeneration != nil {
				t.Fatalf("concurrent generate succeeded more than once: %s", response.body)
			}
			copy := response
			successfulGeneration = &copy
		case http.StatusConflict:
			conflictingGenerations++
		default:
			t.Fatalf("concurrent generate returned %d: %s", response.status, response.body)
		}
	}
	if successfulGeneration == nil || conflictingGenerations != concurrentGenerators-1 {
		t.Fatalf("concurrent generate results: success=%v conflicts=%d", successfulGeneration != nil, conflictingGenerations)
	}
	var generated struct {
		FairnessSignalAvailable bool   `json:"fairnessSignalAvailable"`
		FairnessPolicy          string `json:"fairnessPolicy"`
	}
	if err := json.Unmarshal([]byte(successfulGeneration.body), &generated); err != nil {
		t.Fatal(err)
	}
	if !generated.FairnessSignalAvailable || !strings.Contains(generated.FairnessPolicy, "days since last successful delivery") {
		t.Fatalf("generate response missing fairness evidence: %+v", generated)
	}

	gen2, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID+"/generate", nil)
	gen2.Header.Set("Authorization", "Bearer usr-dispatcher")
	gres2, err := http.DefaultClient.Do(gen2)
	if err != nil {
		t.Fatal(err)
	}
	defer gres2.Body.Close()
	if gres2.StatusCode != http.StatusConflict {
		t.Fatalf("generate twice %d", gres2.StatusCode)
	}

	getPlan, _ := http.NewRequest(http.MethodGet, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID, nil)
	getPlan.Header.Set("Authorization", "Bearer usr-dispatcher")
	detailResponse, err := http.DefaultClient.Do(getPlan)
	if err != nil {
		t.Fatal(err)
	}
	var planDetail map[string]any
	if err := json.NewDecoder(detailResponse.Body).Decode(&planDetail); err != nil {
		detailResponse.Body.Close()
		t.Fatal(err)
	}
	detailResponse.Body.Close()
	allocations, ok := planDetail["allocations"].([]any)
	if !ok || len(allocations) == 0 {
		t.Fatalf("plan detail missing allocations: %#v", planDetail["allocations"])
	}
	allocation, ok := allocations[0].(map[string]any)
	if !ok || allocation["plannedArrivalAt"] == nil || allocation["plannedServiceStartAt"] == nil || allocation["plannedDepartureAt"] == nil {
		t.Fatalf("planned stop timing is not persisted/exposed: %#v", allocations[0])
	}
	fairness, ok := planDetail["fairness"].(map[string]any)
	if !ok || fairness["signalAvailable"] != true {
		t.Fatalf("plan detail missing available fairness signal: %#v", planDetail["fairness"])
	}
	orders, ok := planDetail["orders"].([]any)
	if !ok || len(orders) == 0 {
		t.Fatalf("plan detail missing fairness-scored orders: %#v", planDetail["orders"])
	}
	order, ok := orders[0].(map[string]any)
	if !ok || order["daysSinceLastServed"] != float64(1) || order["fairnessScore"] != float64(1) {
		t.Fatalf("last-served age/priority score not exposed: %#v", orders[0])
	}
	proposal, err := (planservice.Service{Repo: planstore.Postgres{Pool: planPool}, Peers: client}).BreakdownProposals(ctx, created.Plan.ID, "VEH001")
	if err != nil {
		t.Fatalf("breakdown feasibility preview: %v", err)
	}
	proposalTrips, _ := proposal["items"].([]map[string]any)
	if len(proposalTrips) != 1 {
		t.Fatalf("expected affected trip proposal: %#v", proposal)
	}
	proposalOptions, _ := proposalTrips[0]["options"].([]map[string]any)
	if len(proposalOptions) != 1 || proposalOptions[0]["vehicleId"] != "VEH002" || proposalOptions[0]["valid"] != true {
		t.Fatalf("replacement feasibility missed safe vehicle: %#v", proposalOptions)
	}

	conf, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID+"/confirm", nil)
	conf.Header.Set("Authorization", "Bearer usr-dispatcher")
	cres, err := http.DefaultClient.Do(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer cres.Body.Close()
	if cres.StatusCode != http.StatusOK {
		t.Fatalf("confirm %d %s", cres.StatusCode, readBody(cres))
	}
	publication, err := (planstore.Postgres{Pool: planPool}).Publication(ctx, created.Plan.ID)
	if err != nil || publication.Version != 1 || publication.PublishedBy != "USR002" {
		t.Fatalf("first publication missing: %+v err=%v", publication, err)
	}
	if _, err := (planservice.Service{Repo: planstore.Postgres{Pool: planPool}, Peers: client}).ConfirmBreakdown(ctx, &authorization.Profile{UserID: "USR002", Roles: []string{authorization.RoleDispatcher}}, created.Plan.ID, "VEH001", "VEH999", 1); err == nil {
		t.Fatal("infeasible replacement should be rejected")
	}
	unchanged, err := (planstore.Postgres{Pool: planPool}).Get(ctx, created.Plan.ID)
	if err != nil || unchanged.Status != "confirmed" || unchanged.CurrentVersion != 1 {
		t.Fatalf("rejected breakdown changed the published plan: %+v err=%v", unchanged, err)
	}
	ackReq, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID+"/acknowledgements", strings.NewReader(`{"version":1}`))
	ackReq.Header.Set("Authorization", "Bearer usr-loader")
	ackReq.Header.Set("Content-Type", "application/json")
	ackRes, err := http.DefaultClient.Do(ackReq)
	if err != nil {
		t.Fatal(err)
	}
	defer ackRes.Body.Close()
	if ackRes.StatusCode != http.StatusOK {
		t.Fatalf("loader ack %d %s", ackRes.StatusCode, readBody(ackRes))
	}
	// Repeating the same acknowledgement must remain idempotent for the
	// current publication rather than creating duplicate receipts.
	if err := (planstore.Postgres{Pool: planPool}).Acknowledge(ctx, created.Plan.ID, 1, "USR003", authorization.RoleLoader, "", ""); err != nil {
		t.Fatalf("duplicate current-version acknowledgement should be idempotent: %v", err)
	}
	var ackCount int
	if err := planPool.QueryRow(ctx, `SELECT count(*) FROM planning.plan_acknowledgements WHERE plan_id=$1::uuid AND version=1 AND actor_id='USR003' AND actor_role=$2`, created.Plan.ID, authorization.RoleLoader).Scan(&ackCount); err != nil || ackCount != 1 {
		t.Fatalf("duplicate acknowledgement created %d receipts (err=%v)", ackCount, err)
	}
	if err := (planstore.Postgres{Pool: planPool}).Acknowledge(ctx, created.Plan.ID, 0, "USR003", authorization.RoleLoader, "", ""); err == nil || err.Error() != "stale_version" {
		t.Fatalf("stale ack should be rejected, got %v", err)
	}
	breakdownResult, err := (planservice.Service{Repo: planstore.Postgres{Pool: planPool}, Peers: client}).ConfirmBreakdown(ctx, &authorization.Profile{UserID: "USR002", Roles: []string{authorization.RoleDispatcher}}, created.Plan.ID, "VEH001", "VEH002", 1)
	if err != nil || breakdownResult["status"] != "confirmed" || breakdownResult["planVersion"] != 2 {
		t.Fatalf("confirmed breakdown reassignment did not publish a new plan version: %+v err=%v", breakdownResult, err)
	}
	var movedVehicle string
	if err := planPool.QueryRow(ctx, `SELECT vehicle_id FROM planning.allocations WHERE order_id='ord-1'`).Scan(&movedVehicle); err != nil || movedVehicle != "VEH002" {
		t.Fatalf("breakdown allocation did not move atomically: %s %v", movedVehicle, err)
	}
	if err := (planstore.Postgres{Pool: planPool}).Acknowledge(ctx, created.Plan.ID, 1, "USR003", authorization.RoleLoader, "", ""); err == nil || err.Error() != "stale_version" {
		t.Fatalf("superseded version acknowledgement should fail, got %v", err)
	}
	orderID, _ := allocation["orderId"].(string)
	internal, err := (planservice.Service{Repo: planstore.Postgres{Pool: planPool}, Peers: client}).InternalOrder(ctx, orderID)
	if err != nil || internal.State != "PLANNED" || internal.PlannedArrivalAt == nil || internal.PlannedServiceStartAt == nil || internal.StopSequence < 1 || internal.Depot != "DEPOT_NORTH" {
		t.Fatalf("internal order tracking did not reuse persisted stop timing: %+v err=%v", internal, err)
	}

	missingTracking, _ := http.NewRequest(http.MethodGet, planSrv.URL+"/api/v1/planning/internal/orders/order-without-plan", nil)
	missingTracking.Header.Set("Authorization", "Bearer svc-order")
	missingResponse, err := http.DefaultClient.Do(missingTracking)
	if err != nil {
		t.Fatal(err)
	}
	defer missingResponse.Body.Close()
	if missingResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("unplanned order tracking should be a not-found response, got %d %s", missingResponse.StatusCode, readBody(missingResponse))
	}
}

func startPostgresContainer(ctx context.Context, request testcontainers.GenericContainerRequest) (container testcontainers.Container, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("testcontainers could not access Docker: %v", recovered)
		}
	}()
	return testcontainers.GenericContainer(ctx, request)
}

type staticProfiles struct{}

func (staticProfiles) Resolve(_ context.Context, subject string) (*authorization.Profile, error) {
	switch subject {
	case "usr-dispatcher":
		return &authorization.Profile{UserID: "USR002", Subject: subject, Roles: []string{"DISPATCHER"}}, nil
	case "usr-store-manager":
		return &authorization.Profile{UserID: "USR001", Subject: subject, Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT034"}}, nil
	case "usr-loader":
		return &authorization.Profile{UserID: "USR003", Subject: subject, Roles: []string{authorization.RoleLoader}}, nil
	case "usr-driver-a":
		return &authorization.Profile{UserID: "USR010", Subject: subject, Roles: []string{authorization.RoleDriver}, VehicleID: "VEH-A"}, nil
	case "usr-driver-b":
		return &authorization.Profile{UserID: "USR011", Subject: subject, Roles: []string{authorization.RoleDriver}, VehicleID: "VEH-B"}, nil
	case "usr-loader-north":
		return &authorization.Profile{UserID: "USR012", Subject: subject, Roles: []string{authorization.RoleLoader}, Depot: "DEPOT_NORTH"}, nil
	case "usr-loader-south":
		return &authorization.Profile{UserID: "USR013", Subject: subject, Roles: []string{authorization.RoleLoader}, Depot: "DEPOT_SOUTH"}, nil
	default:
		return &authorization.Profile{Subject: subject}, nil
	}
}

// TestPlanningUnallocatedReasonPersists proves FR-12's allocator explanation
// survives past the one-time generate response: an order too heavy for either
// vehicle must still show its real reason code and other limiting factors when
// the Dispatcher reopens the plan later, not the generic placeholder that was
// synthesized before this reason was persisted.
func TestPlanningUnallocatedReasonPersists(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}
	pg, err := startPostgresContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
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
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0007_planning.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0013_planning_stop_times.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0020_planning_publications.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0035_planning_disruption_risks.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0036_planning_unallocated_reasons.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0037_planning_deferral_next_run.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0073_planning_ack_trip_identity.sql"))

	peers := httptest.NewServer(overweightOrderPeerStub())
	t.Cleanup(peers.Close)

	planPool, err := db.Open(ctx, dsn, "planning")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(planPool.Close)
	client := planclient.Peers{
		OrdersURL: peers.URL, FleetURL: peers.URL, SharedURL: peers.URL, DeliveryURL: peers.URL,
		M2M: staticToken("svc-planning"),
	}
	planH := planhandler.Handler{
		Authn:    bearerAuth{},
		Profiles: staticProfiles{},
		Service:  planservice.Service{Repo: planstore.Postgres{Pool: planPool}, Peers: client},
	}
	planR := chi.NewRouter()
	planH.Routes(planR)
	planSrv := httptest.NewServer(planR)
	t.Cleanup(planSrv.Close)

	createPlan, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans", bytes.NewReader([]byte(`{"deliveryDate":"2026-09-29"}`)))
	createPlan.Header.Set("Authorization", "Bearer usr-dispatcher")
	createPlan.Header.Set("Content-Type", "application/json")
	pres, err := http.DefaultClient.Do(createPlan)
	if err != nil {
		t.Fatal(err)
	}
	defer pres.Body.Close()
	if pres.StatusCode != http.StatusCreated {
		t.Fatalf("create plan %d %s", pres.StatusCode, readBody(pres))
	}
	var created struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	if err := json.NewDecoder(pres.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	gen, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID+"/generate", nil)
	gen.Header.Set("Authorization", "Bearer usr-dispatcher")
	gres, err := http.DefaultClient.Do(gen)
	if err != nil {
		t.Fatal(err)
	}
	genBody := readBody(gres)
	gres.Body.Close()
	if gres.StatusCode != http.StatusOK {
		t.Fatalf("generate %d: %s", gres.StatusCode, genBody)
	}
	var generated struct {
		Unallocated int `json:"unallocated"`
		Failures    []struct {
			OrderID    string         `json:"orderId"`
			ReasonCode string         `json:"reasonCode"`
			Details    map[string]any `json:"details"`
		} `json:"failures"`
	}
	if err := json.Unmarshal([]byte(genBody), &generated); err != nil {
		t.Fatal(err)
	}
	if generated.Unallocated != 1 || len(generated.Failures) != 1 {
		t.Fatalf("expected exactly one unallocated order: %s", genBody)
	}
	if generated.Failures[0].OrderID != "ord-heavy" || generated.Failures[0].ReasonCode != "WEIGHT_CAPACITY_EXCEEDED" {
		t.Fatalf("unexpected failure on generate: %+v", generated.Failures[0])
	}
	if _, ok := generated.Failures[0].Details["otherLimitingFactors"]; !ok {
		t.Fatalf("generate response missing other limiting factors: %+v", generated.Failures[0].Details)
	}

	// FR-54: a manual allocation override must carry a reason. Missing one is
	// rejected before the constraint engine even runs, so this still returns
	// 400 (not 409) for an order that would fail constraints anyway.
	noReason, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID+"/allocations", strings.NewReader(`{"orderId":"ord-heavy","vehicleId":"VEH001","tripNumber":1}`))
	noReason.Header.Set("Authorization", "Bearer usr-dispatcher")
	noReason.Header.Set("Content-Type", "application/json")
	noReasonRes, err := http.DefaultClient.Do(noReason)
	if err != nil {
		t.Fatal(err)
	}
	noReasonBody := readBody(noReasonRes)
	noReasonRes.Body.Close()
	if noReasonRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("manual assign without a reason should be rejected 400, got %d: %s", noReasonRes.StatusCode, noReasonBody)
	}

	// With a real reason supplied, the request reaches the constraint engine
	// instead (and is rejected 409 for the same weight failure as above).
	withReason, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID+"/allocations", strings.NewReader(`{"orderId":"ord-heavy","vehicleId":"VEH001","tripNumber":1,"reason":"Dispatcher is testing manual override capture."}`))
	withReason.Header.Set("Authorization", "Bearer usr-dispatcher")
	withReason.Header.Set("Content-Type", "application/json")
	withReasonRes, err := http.DefaultClient.Do(withReason)
	if err != nil {
		t.Fatal(err)
	}
	withReasonBody := readBody(withReasonRes)
	withReasonRes.Body.Close()
	if withReasonRes.StatusCode != http.StatusConflict {
		t.Fatalf("manual assign with a reason should still enforce constraints (409), got %d: %s", withReasonRes.StatusCode, withReasonBody)
	}

	// Fetch the plan detail twice, simulating the Dispatcher closing and
	// reopening the plan well after the one-time generate response is gone.
	for i := 0; i < 2; i++ {
		getPlan, _ := http.NewRequest(http.MethodGet, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID, nil)
		getPlan.Header.Set("Authorization", "Bearer usr-dispatcher")
		dres, err := http.DefaultClient.Do(getPlan)
		if err != nil {
			t.Fatal(err)
		}
		var detail map[string]any
		if err := json.NewDecoder(dres.Body).Decode(&detail); err != nil {
			dres.Body.Close()
			t.Fatal(err)
		}
		dres.Body.Close()
		unalloc, ok := detail["unallocated"].([]any)
		if !ok || len(unalloc) != 1 {
			t.Fatalf("round %d: plan detail unallocated: %#v", i, detail["unallocated"])
		}
		row, ok := unalloc[0].(map[string]any)
		if !ok || row["orderId"] != "ord-heavy" {
			t.Fatalf("round %d: unexpected unallocated row: %#v", i, unalloc[0])
		}
		if row["reasonCode"] != "WEIGHT_CAPACITY_EXCEEDED" {
			t.Fatalf("round %d: persisted reason regressed to placeholder: %#v", i, row)
		}
		details, ok := row["details"].(map[string]any)
		if !ok {
			t.Fatalf("round %d: persisted reason missing details: %#v", i, row)
		}
		if _, ok := details["otherLimitingFactors"]; !ok {
			t.Fatalf("round %d: persisted reason missing other limiting factors: %#v", i, details)
		}
	}

	// If the persisted-reason table becomes unreadable, the Dispatcher must
	// see an explicit "unavailable" signal, never a specific-looking but
	// possibly wrong reason silently standing in for it.
	if _, err := planPool.Exec(ctx, `DROP TABLE planning.unallocated_reasons`); err != nil {
		t.Fatal(err)
	}
	getPlanAfterDrop, _ := http.NewRequest(http.MethodGet, planSrv.URL+"/api/v1/planning/plans/"+created.Plan.ID, nil)
	getPlanAfterDrop.Header.Set("Authorization", "Bearer usr-dispatcher")
	afterDropRes, err := http.DefaultClient.Do(getPlanAfterDrop)
	if err != nil {
		t.Fatal(err)
	}
	var afterDrop map[string]any
	if err := json.NewDecoder(afterDropRes.Body).Decode(&afterDrop); err != nil {
		afterDropRes.Body.Close()
		t.Fatal(err)
	}
	afterDropRes.Body.Close()
	if afterDropRes.StatusCode != http.StatusOK {
		t.Fatalf("plan detail should still degrade gracefully, not fail outright: %d", afterDropRes.StatusCode)
	}
	if afterDrop["unallocatedReasonsAvailable"] != false {
		t.Fatalf("expected unallocatedReasonsAvailable=false once the table is gone: %#v", afterDrop["unallocatedReasonsAvailable"])
	}
	unallocAfterDrop, _ := afterDrop["unallocated"].([]any)
	if len(unallocAfterDrop) != 1 {
		t.Fatalf("expected the one unallocated order to remain listed: %#v", afterDrop["unallocated"])
	}
	rowAfterDrop, _ := unallocAfterDrop[0].(map[string]any)
	if rowAfterDrop["reasonCode"] != "REASON_UNAVAILABLE" {
		t.Fatalf("expected an explicit unavailable reason, not a misleading placeholder: %#v", rowAfterDrop)
	}
}

// TestPlanningDeferralNextRunAndRepeatWarning proves FR-53: a deferral records
// the Dispatcher's expected next-run date, and a later plan for the same
// outlet is warned that it was deferred on its last run.
func TestPlanningDeferralNextRunAndRepeatWarning(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER": "waypoint", "POSTGRES_PASSWORD": "waypoint", "POSTGRES_DB": "waypoint",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}
	pg, err := startPostgresContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
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
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0007_planning.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0013_planning_stop_times.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0020_planning_publications.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0035_planning_disruption_risks.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0036_planning_unallocated_reasons.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0037_planning_deferral_next_run.sql"))
	applySQL(t, ctx, dsn, filepath.Join(root, "database", "migrations", "0073_planning_ack_trip_identity.sql"))

	peers := httptest.NewServer(peerStub())
	t.Cleanup(peers.Close)

	planPool, err := db.Open(ctx, dsn, "planning")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(planPool.Close)
	client := planclient.Peers{
		OrdersURL: peers.URL, FleetURL: peers.URL, SharedURL: peers.URL, DeliveryURL: peers.URL,
		M2M: staticToken("svc-planning"),
	}
	planH := planhandler.Handler{
		Authn:    bearerAuth{},
		Profiles: staticProfiles{},
		Service:  planservice.Service{Repo: planstore.Postgres{Pool: planPool}, Peers: client},
	}
	planR := chi.NewRouter()
	planH.Routes(planR)
	planSrv := httptest.NewServer(planR)
	t.Cleanup(planSrv.Close)

	createPlanA, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans", bytes.NewReader([]byte(`{"deliveryDate":"2026-09-29"}`)))
	createPlanA.Header.Set("Authorization", "Bearer usr-dispatcher")
	createPlanA.Header.Set("Content-Type", "application/json")
	presA, err := http.DefaultClient.Do(createPlanA)
	if err != nil {
		t.Fatal(err)
	}
	defer presA.Body.Close()
	if presA.StatusCode != http.StatusCreated {
		t.Fatalf("create plan A %d %s", presA.StatusCode, readBody(presA))
	}
	var createdA struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	if err := json.NewDecoder(presA.Body).Decode(&createdA); err != nil {
		t.Fatal(err)
	}

	// An out-of-range next-run target (not after this plan's own date) is rejected.
	badDefer, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+createdA.Plan.ID+"/deferrals", strings.NewReader(`{"orderId":"ord-1","reasonCode":"NO_ELIGIBLE_VEHICLE","nextRunTarget":"2026-09-29"}`))
	badDefer.Header.Set("Authorization", "Bearer usr-dispatcher")
	badDefer.Header.Set("Content-Type", "application/json")
	badDeferRes, err := http.DefaultClient.Do(badDefer)
	if err != nil {
		t.Fatal(err)
	}
	badDeferBody := readBody(badDeferRes)
	badDeferRes.Body.Close()
	if badDeferRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("a next-run target on or before the plan date should be rejected 400, got %d: %s", badDeferRes.StatusCode, badDeferBody)
	}

	deferReq, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans/"+createdA.Plan.ID+"/deferrals", strings.NewReader(`{"orderId":"ord-1","reasonCode":"NO_ELIGIBLE_VEHICLE","comment":"No reefer free today","nextRunTarget":"2026-09-30"}`))
	deferReq.Header.Set("Authorization", "Bearer usr-dispatcher")
	deferReq.Header.Set("Content-Type", "application/json")
	deferRes, err := http.DefaultClient.Do(deferReq)
	if err != nil {
		t.Fatal(err)
	}
	deferBody := readBody(deferRes)
	deferRes.Body.Close()
	if deferRes.StatusCode != http.StatusCreated {
		t.Fatalf("defer with a valid next-run target %d: %s", deferRes.StatusCode, deferBody)
	}

	getPlanA, _ := http.NewRequest(http.MethodGet, planSrv.URL+"/api/v1/planning/plans/"+createdA.Plan.ID, nil)
	getPlanA.Header.Set("Authorization", "Bearer usr-dispatcher")
	planARes, err := http.DefaultClient.Do(getPlanA)
	if err != nil {
		t.Fatal(err)
	}
	var planADetail struct {
		Deferrals []struct {
			OrderID       string `json:"orderId"`
			NextRunTarget string `json:"nextRunTarget"`
		} `json:"deferrals"`
	}
	if err := json.NewDecoder(planARes.Body).Decode(&planADetail); err != nil {
		planARes.Body.Close()
		t.Fatal(err)
	}
	planARes.Body.Close()
	if len(planADetail.Deferrals) != 1 || planADetail.Deferrals[0].NextRunTarget != "2026-09-30" {
		t.Fatalf("next-run target was not persisted: %+v", planADetail.Deferrals)
	}

	// A later plan for the same outlet (OUT034, carried by ord-1 in peerStub)
	// must now see the repeat-deferral warning from plan A.
	createPlanB, _ := http.NewRequest(http.MethodPost, planSrv.URL+"/api/v1/planning/plans", bytes.NewReader([]byte(`{"deliveryDate":"2026-09-30"}`)))
	createPlanB.Header.Set("Authorization", "Bearer usr-dispatcher")
	createPlanB.Header.Set("Content-Type", "application/json")
	presB, err := http.DefaultClient.Do(createPlanB)
	if err != nil {
		t.Fatal(err)
	}
	defer presB.Body.Close()
	if presB.StatusCode != http.StatusCreated {
		t.Fatalf("create plan B %d %s", presB.StatusCode, readBody(presB))
	}
	var createdB struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	if err := json.NewDecoder(presB.Body).Decode(&createdB); err != nil {
		t.Fatal(err)
	}

	getPlanB, _ := http.NewRequest(http.MethodGet, planSrv.URL+"/api/v1/planning/plans/"+createdB.Plan.ID, nil)
	getPlanB.Header.Set("Authorization", "Bearer usr-dispatcher")
	planBRes, err := http.DefaultClient.Do(getPlanB)
	if err != nil {
		t.Fatal(err)
	}
	var planBDetail struct {
		Orders []struct {
			OutletID         string `json:"outletId"`
			DeferredLastRun  bool   `json:"deferredLastRun"`
			LastDeferralDate string `json:"lastDeferralDate"`
		} `json:"orders"`
	}
	if err := json.NewDecoder(planBRes.Body).Decode(&planBDetail); err != nil {
		planBRes.Body.Close()
		t.Fatal(err)
	}
	planBRes.Body.Close()
	if len(planBDetail.Orders) != 1 || !planBDetail.Orders[0].DeferredLastRun || planBDetail.Orders[0].LastDeferralDate != "2026-09-29" {
		t.Fatalf("plan B should warn that OUT034 was deferred last run (2026-09-29): %+v", planBDetail.Orders)
	}
}

// overweightOrderPeerStub is a minimal peer stub for TestPlanningUnallocatedReasonPersists:
// one order fits easily, the other exceeds both vehicles' weight capacity so it
// is left unallocated with a specific, checkable reason.
func overweightOrderPeerStub() http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/delivery/internal/outlets/last-served", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}})
	})
	r.Get("/api/v1/orders", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{
			{
				"id": "ord-light", "orderRef": "ORD000001", "outletId": "OUT034", "brand": "Fresh",
				"temperatureRequirement": "ambient", "orderWeightKg": 40, "orderVolumeM3": 1.2,
				"requestedDeliveryDate": "2026-09-29", "status": "confirmed",
			},
			{
				"id": "ord-heavy", "orderRef": "ORD000002", "outletId": "OUT034", "brand": "Fresh",
				"temperatureRequirement": "ambient", "orderWeightKg": 99999, "orderVolumeM3": 1.2,
				"requestedDeliveryDate": "2026-09-29", "status": "confirmed",
			},
		}})
	})
	r.Get("/api/v1/fleet/vehicles", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{
			// VEH001 is home-depot eligible but too light for ord-heavy (WEIGHT_CAPACITY_EXCEEDED).
			{"id": "VEH001", "type": "truck", "temp": "ambient", "homeDepot": "DEPOT_NORTH", "weightCapacityKg": 2000, "volumeCapacityM3": 20, "kmPerL": 8, "weeklyFuelQuotaL": 400},
			// VEH002 could carry the weight but is based at a different depot (DEPOT_MISMATCH),
			// so the explanation has two distinct reasons across the failed attempts.
			{"id": "VEH002", "type": "truck", "temp": "ambient", "homeDepot": "DEPOT_SOUTH", "weightCapacityKg": 200000, "volumeCapacityM3": 2000, "kmPerL": 8, "weeklyFuelQuotaL": 400},
		}})
	})
	r.Get("/api/v1/fleet/vehicles/{id}", func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"vehicle": map[string]any{"id": chi.URLParam(req, "id"), "homeDepot": "DEPOT_NORTH"}})
	})
	r.Get("/api/v1/fleet/availability", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}})
	})
	r.Get("/api/v1/shared/outlets", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{
			"id": "OUT034", "brand": "Fresh", "district": "Colombo", "depot": "DEPOT_NORTH",
			"parkingConstraint": "normal", "mallWindow": false, "windowOpenTime": "06:30", "windowCloseTime": "08:00",
		}}})
	})
	r.Get("/api/v1/shared/travel", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}, "defaultService": 15, "mallService": 20})
	})
	r.Get("/api/v1/shared/profiles/me", func(w http.ResponseWriter, req *http.Request) {
		p, err := bearerAuth{}.Authenticate(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"profile": map[string]any{"userId": "USR002", "subject": p.Subject, "roles": []string{"DISPATCHER"}}})
	})
	r.Post("/api/v1/shared/audit-events", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "ingested"})
	})
	return r
}

func peerStub() http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/delivery/internal/outlets/last-served", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{"outletId": "OUT034", "lastServedAt": "2026-09-28T16:00:00Z"}}})
	})
	r.Get("/api/v1/orders", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{
			"id": "ord-1", "orderRef": "ORD000001", "outletId": "OUT034", "brand": "Fresh",
			"temperatureRequirement": "chilled", "orderWeightKg": 40, "orderVolumeM3": 1.2,
			"requestedDeliveryDate": "2026-09-29", "status": "confirmed",
		}}})
	})
	r.Get("/api/v1/fleet/vehicles", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{
			"id": "VEH001", "type": "truck", "temp": "reefer", "homeDepot": "DEPOT_NORTH",
			"weightCapacityKg": 2000, "volumeCapacityM3": 20, "kmPerL": 8, "weeklyFuelQuotaL": 400,
		}, {"id": "VEH002", "type": "truck", "temp": "reefer", "homeDepot": "DEPOT_NORTH", "weightCapacityKg": 2000, "volumeCapacityM3": 20, "kmPerL": 8, "weeklyFuelQuotaL": 400}}})
	})
	r.Get("/api/v1/fleet/vehicles/{id}", func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"vehicle": map[string]any{"id": chi.URLParam(req, "id"), "homeDepot": "DEPOT_NORTH"}})
	})
	r.Get("/api/v1/fleet/availability", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}})
	})
	r.Get("/api/v1/shared/outlets", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{
			"id": "OUT034", "brand": "Fresh", "district": "Colombo", "depot": "DEPOT_NORTH",
			"parkingConstraint": "normal", "mallWindow": false, "windowOpenTime": "06:30", "windowCloseTime": "08:00",
		}}})
	})
	r.Get("/api/v1/shared/travel", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}, "defaultService": 15, "mallService": 20})
	})
	r.Get("/api/v1/shared/profiles/me", func(w http.ResponseWriter, req *http.Request) {
		p, err := bearerAuth{}.Authenticate(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(p.Subject, "svc-") {
			http.NotFound(w, req)
			return
		}
		role := "DISPATCHER"
		if p.Subject == "usr-store-manager" {
			role = "STORE_MANAGER"
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"profile": map[string]any{"userId": "USR002", "subject": p.Subject, "roles": []string{role}}})
	})
	r.Post("/api/v1/shared/audit-events", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "ingested"})
	})
	return r
}

func readBody(res *http.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(res.Body)
	return buf.String()
}

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
