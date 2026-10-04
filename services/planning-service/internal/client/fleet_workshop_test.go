package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type workshopRT func(*http.Request) (*http.Response, error)

func (f workshopRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSetVehicleWorkshopCallsFleetAvailability(t *testing.T) {
	tr := workshopRT(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/fleet/vehicles/V-1/availability" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		for _, w := range []string{`"status":"in_workshop"`, `"date":"2026-10-05"`} {
			if !strings.Contains(string(b), w) {
				t.Fatalf("missing %s in %s", w, b)
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
	})
	p := Peers{FleetURL: "http://fleet.test", HTTP: &http.Client{Transport: tr}}
	if err := p.SetVehicleWorkshop(context.Background(), "V-1", "2026-10-05", "breakdown"); err != nil {
		t.Fatal(err)
	}
	bad := workshopRT(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`no`)), Request: r}, nil
	})
	p.HTTP = &http.Client{Transport: bad}
	if err := p.SetVehicleWorkshop(context.Background(), "V-1", "2026-10-05", "x"); err == nil {
		t.Fatal("expected error on 403")
	}
}
