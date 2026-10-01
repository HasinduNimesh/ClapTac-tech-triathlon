package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type testToken string

func (t testToken) Token(context.Context) (string, error) { return string(t), nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOutletLastServedLoadsSuccessfulDeliveryHistory(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/delivery/internal/outlets/last-served" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer planning-service-token" {
			t.Errorf("missing service authorization: %q", r.Header.Get("Authorization"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"items":[{"outletId":"OUT034","lastServedAt":"2026-09-29T12:30:00+05:30"}]}`)),
			Request:    r,
		}, nil
	})

	peers := Peers{DeliveryURL: "http://delivery.test", M2M: testToken("planning-service-token"), HTTP: &http.Client{Transport: transport}}
	items, err := peers.OutletLastServed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.September, 29, 12, 30, 0, 0, time.FixedZone("Asia/Colombo", 5*60*60+30*60))
	if got := items["OUT034"]; !got.Equal(want) {
		t.Fatalf("wrong delivery timestamp: %s", got)
	}
}

func TestActualFuelByVehicleReadsWeeklyFleetLedger(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/fleet/fuel/ledger" || r.URL.Query().Get("weekOf") != "2026-09-30" {
			t.Errorf("unexpected fuel ledger request: %s", r.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"items":[{"vehicleId":"VEH001","actualLitersL":125.5}]}`)),
			Request:    r,
		}, nil
	})
	peers := Peers{FleetURL: "http://fleet.test", M2M: testToken("planning-service-token"), HTTP: &http.Client{Transport: transport}}
	items, err := peers.ActualFuelByVehicle(context.Background(), "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if items["VEH001"] != 125.5 {
		t.Fatalf("wrong actual fuel total: %#v", items)
	}
}

func TestPlanningPolicyReadsVersionedSharedConfiguration(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/shared/policies/current" {
			t.Errorf("unexpected policy URL %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer planning-service-token" {
			t.Errorf("missing planning service authorization")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"policy":{"version":3,"cutoffLocalTime":"15:30:00","deferralWeightPoints":20,"maxDeferralCount":10,"maxUnservedDays":400,"maxTripsPerVehicle":1}}`)), Request: r}, nil
	})
	peers := Peers{SharedURL: "http://shared.test", M2M: testToken("planning-service-token"), HTTP: &http.Client{Transport: transport}}
	policy, err := peers.PlanningPolicy(context.Background())
	if err != nil || policy.Version != 3 || policy.DeferralWeightPoints != 20 || policy.CutoffLocalTime != "15:30:00" {
		t.Fatalf("policy=%+v err=%v", policy, err)
	}
}

func TestQueueNotificationUsesStableEventAndM2MAuth(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/shared/internal/notifications/enqueue" {
			t.Fatalf("unexpected notification endpoint: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer planning-service-token" {
			t.Fatal("notification enqueue must use service credentials")
		}
		b, _ := io.ReadAll(r.Body)
		for _, want := range []string{`"eventKey":"deferral:PLAN-01:ORD-01"`, `"outletId":"OUT001"`, `"type":"DEFERRAL"`, `"orderRef":"ORD-01"`} {
			if !strings.Contains(string(b), want) {
				t.Fatalf("missing %s in payload %s", want, b)
			}
		}
		return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"notification":{"status":"enqueued","id":1}}`)), Request: r}, nil
	})
	p := Peers{SharedURL: "http://shared.test", M2M: testToken("planning-service-token"), HTTP: &http.Client{Transport: transport}}
	if err := p.QueueNotification(context.Background(), "deferral:PLAN-01:ORD-01", "OUT001", "DEFERRAL", "ORD-01", "WINDOW", 0); err != nil {
		t.Fatal(err)
	}
}
