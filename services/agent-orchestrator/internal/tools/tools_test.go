package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/credentials"
)

func TestCatalogUsesRealRoutesAndForwardsHumanToken(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tracking":{"stage":"PLANNED"}}`))
	}))
	defer srv.Close()
	t.Setenv("ORDER_SERVICE_URL", srv.URL)
	var tracking Tool
	for _, candidate := range Catalog(credentials.InboundForwarder{}) {
		if candidate.Name() == "GetOrderTracking" {
			tracking = candidate
		}
	}
	if tracking == nil {
		t.Fatal("GetOrderTracking missing from catalog")
	}
	args := map[string]any{"orderId": "order-1"}
	if err := tracking.Validate(args); err != nil {
		t.Fatal(err)
	}
	got, err := tracking.Call(credentials.WithInboundToken(context.Background(), "Bearer user-token"), args)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/orders/order-1/tracking" || gotAuth != "Bearer user-token" {
		t.Fatalf("request path=%q auth=%q", gotPath, gotAuth)
	}
	if got["tracking"] == nil {
		t.Fatalf("unexpected result %#v", got)
	}
}

func TestToolSchemaRejectsMalformedArguments(t *testing.T) {
	var create Tool
	for _, candidate := range Catalog(credentials.InboundForwarder{}) {
		if candidate.Name() == "CreateOrder" {
			create = candidate
		}
	}
	if create == nil {
		t.Fatal("CreateOrder missing")
	}
	if err := create.Validate(map[string]any{"requestedDeliveryDate": "2026-09-29"}); err == nil {
		t.Fatal("required fields accepted as missing")
	}
	valid := map[string]any{"requestedDeliveryDate": "2026-09-29", "orderUnits": 8, "orderWeightKg": 12.0, "orderVolumeM3": 1.4, "temperatureRequirement": "CHILLED"}
	if err := create.Validate(valid); err != nil {
		t.Fatal(err)
	}
	valid["untrustedUrl"] = "http://attacker"
	if err := create.Validate(valid); err == nil {
		t.Fatal("extra argument accepted")
	}
	for _, candidate := range Catalog(credentials.InboundForwarder{}) {
		if candidate.Name() == "GetOrderTracking" {
			if err := candidate.Validate(map[string]any{"orderId": "../admin"}); err == nil {
				t.Fatal("path traversal argument accepted")
			}
		}
	}
}
