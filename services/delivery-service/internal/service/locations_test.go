package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func sharedWith(t *testing.T, body string, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if r.URL.Path != "/api/v1/shared/outlets" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestStopsCarryTheirOutletPosition(t *testing.T) {
	var hits int32
	srv := sharedWith(t, `{"items":[
		{"id":"OUT001","latitude":6.93441,"longitude":79.84281,"locationApproximate":false},
		{"id":"OUT002","latitude":6.95,"longitude":79.9,"locationApproximate":true}]}`, &hits)
	svc := Service{Peers: client.Peers{SharedURL: srv.URL}}

	original := []domain.Stop{{ID: "s1", OutletID: "OUT001"}, {ID: "s2", OutletID: "OUT002"}, {ID: "s3", OutletID: "OUT404"}}
	stops := svc.withLocations(context.Background(), original)

	if stops[0].Latitude == nil || *stops[0].Latitude != 6.93441 || *stops[0].Longitude != 79.84281 || stops[0].LocationApproximate {
		t.Fatalf("an outlet with a recorded position gives an exact stop position: %+v", stops[0])
	}
	if stops[1].Latitude == nil || !stops[1].LocationApproximate {
		t.Fatalf("an approximate outlet position stays marked approximate so nobody navigates to it as the shop: %+v", stops[1])
	}
	if stops[2].Latitude != nil || stops[2].Longitude != nil {
		t.Fatalf("an unknown outlet has no position: %+v", stops[2])
	}
	if original[0].Latitude != nil {
		t.Fatal("the stored stops must not be changed")
	}
	// The outlet list is shared between requests for a short time.
	svc.withLocations(context.Background(), original)
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("shared-service asked %d times for two trips served together; want 1", n)
	}
}

func TestTripIsStillServedWhenOutletPositionsCannotBeRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusBadGateway) }))
	t.Cleanup(srv.Close)
	svc := Service{Peers: client.Peers{SharedURL: srv.URL + "/unreachable"}}
	stops := svc.withLocations(context.Background(), []domain.Stop{{ID: "s1", OutletID: "OUT001"}})
	if len(stops) != 1 || stops[0].Latitude != nil {
		t.Fatalf("without positions the stop is served as it was: %+v", stops)
	}
}
