package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/service"
)

type policyStub struct {
	cutoff string
	err    error
}

func (p policyStub) CutoffLocalTime() (string, error) { return p.cutoff, p.err }

// Screens tell people the order cutoff, so they must be told the one the service applies, not a
// time written into the page.
func TestCutoffEndpointReportsTheCutoffTheServiceApplies(t *testing.T) {
	profiles := fakeProfiles{
		"usr-store-manager": {UserID: "USR001", Subject: "usr-store-manager", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT034"}},
		"usr-dispatcher":    {UserID: "USR002", Subject: "usr-dispatcher", Roles: []string{"DISPATCHER"}},
		"usr-driver":        {UserID: "USR006", Subject: "usr-driver", Roles: []string{"DRIVER"}, VehicleID: "VEH001"},
	}
	ask := func(policy service.Service, subject string) (int, string) {
		h := Handler{Authn: fakeAuth{p: &auth.Principal{Subject: subject}}, Profiles: profiles, Service: policy}
		r := chi.NewRouter()
		h.Routes(r)
		srv := httptest.NewServer(r)
		defer srv.Close()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/orders/cutoff", nil)
		req.Header.Set("Authorization", "Bearer "+subject)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out struct {
			Cutoff struct {
				LocalTime string `json:"localTime"`
				Timezone  string `json:"timezone"`
			} `json:"cutoff"`
		}
		_ = json.NewDecoder(res.Body).Decode(&out)
		if res.StatusCode == http.StatusOK && out.Cutoff.Timezone != "Asia/Colombo" {
			t.Fatalf("timezone = %q", out.Cutoff.Timezone)
		}
		return res.StatusCode, out.Cutoff.LocalTime
	}

	changed := service.Service{Policy: policyStub{cutoff: "15:30:00"}}
	if code, local := ask(changed, "usr-store-manager"); code != http.StatusOK || local != "15:30" {
		t.Fatalf("a store manager is told the policy's cutoff as HH:MM: %d %q", code, local)
	}
	if code, local := ask(changed, "usr-dispatcher"); code != http.StatusOK || local != "15:30" {
		t.Fatalf("a dispatcher is told the same: %d %q", code, local)
	}
	if code, _ := ask(changed, "usr-driver"); code != http.StatusForbidden {
		t.Fatalf("a driver has no use for the order cutoff: %d", code)
	}
	if code, local := ask(service.Service{Policy: policyStub{err: errors.New("down")}}, "usr-store-manager"); code != http.StatusOK || local != "16:00" {
		t.Fatalf("with the policy unreadable the service applies 16:00, so that is what is reported: %d %q", code, local)
	}
	if code, local := ask(service.Service{}, "usr-store-manager"); code != http.StatusOK || local != "16:00" {
		t.Fatalf("no policy configured means the 16:00 default: %d %q", code, local)
	}
}
