package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoveryDeliveryFailClosed(t *testing.T) {
	for _, status := range []int{200, 404, 401, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status == 200 {
					_, _ = w.Write([]byte(`{"delivery":{"tripId":"trip","outcome":"DELIVERED"}}`))
				}
			}))
			defer srv.Close()
			p := Peers{DeliveryURL: srv.URL}
			state, err := p.RecoveryDelivery(context.Background(), "order")
			if status == 200 {
				if err != nil || state.Outcome != "DELIVERED" {
					t.Fatalf("missing progress: %+v %v", state, err)
				}
			} else if status == 404 {
				if err != nil || state.Outcome != "" {
					t.Fatal("404 means no prepared delivery")
				}
			} else if err == nil {
				t.Fatalf("%d was treated as no delivery", status)
			}
		})
	}
}

func TestRecoveryAvailabilityNeverDefaultsAfterFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 503) }))
	defer srv.Close()
	_, err := (Peers{FleetURL: srv.URL}).RecoveryAvailability(context.Background(), "2026-10-26")
	if err == nil {
		t.Fatal("failed availability must not become an available vehicle")
	}
}
