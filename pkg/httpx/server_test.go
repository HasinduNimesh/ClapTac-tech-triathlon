package httpx

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRoutePatternUsesTemplateAndSecurityHeaders(t *testing.T) {
	var logs bytes.Buffer
	r := NewRouter(Options{Service: "httpx-test", Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	r.Get("/api/v1/orders/{orderId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orders/order-secret-42", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q", got)
	}
	if strings.Contains(logs.String(), "order-secret-42") {
		t.Fatalf("access log contains a resource id: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "/api/v1/orders/{orderId}") {
		t.Fatalf("access log omitted route template: %s", logs.String())
	}

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "path" && strings.Contains(label.GetValue(), "order-secret-42") {
					t.Fatalf("metric contains a resource id: %s", label.GetValue())
				}
			}
		}
	}
}

func TestRequestBodyLimit(t *testing.T) {
	const limit = 8
	var readErr error
	h := requestBodyLimit(limit)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("123456789")))
	if readErr == nil {
		t.Fatal("expected oversized request body to be rejected")
	}
}

func TestRoutePatternForUnmatchedPathIsBounded(t *testing.T) {
	r := chi.NewRouter()
	rec := httptest.NewRecorder()
	r.Get("/ok/{id}", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(routePattern(req)))
	})
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok/secret", nil))
	if rec.Body.String() != "/ok/{id}" {
		t.Fatalf("route pattern = %q", rec.Body.String())
	}
}

func TestTracingMiddlewareUsesRouteTemplateAndCorrelationID(t *testing.T) {
	previous := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	r := NewRouter(Options{Service: "trace-test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	r.Get("/api/v1/orders/{orderId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/order-secret-42", nil)
	req.Header.Set("X-Correlation-ID", "corr-123")
	r.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "GET /api/v1/orders/{orderId}" {
		t.Fatalf("span name = %q", span.Name())
	}
	values := map[string]string{}
	for _, item := range span.Attributes() {
		if item.Key == "http.route" || item.Key == "waypoint.correlation_id" {
			values[string(item.Key)] = item.Value.AsString()
		}
	}
	if values["http.route"] != "/api/v1/orders/{orderId}" || values["waypoint.correlation_id"] != "corr-123" {
		t.Fatalf("span attributes = %#v", values)
	}
	if strings.Contains(span.Name(), "order-secret-42") {
		t.Fatalf("span includes resource id: %s", span.Name())
	}
}
