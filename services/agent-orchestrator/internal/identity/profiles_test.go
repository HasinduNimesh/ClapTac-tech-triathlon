package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
)

func TestResolveUsesSharedProfileAndForwardsHumanBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shared/profiles/me" || r.Header.Get("Authorization") != "Bearer human-token" {
			t.Errorf("unexpected profile lookup: path=%q authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":{"userId":"D1","subject":"dispatcher-sub","roles":["DISPATCHER"]}}`))
	}))
	defer srv.Close()

	profile, err := (Profiles{BaseURL: srv.URL}).Resolve(auth.WithBearer(context.Background(), "Bearer human-token"), "dispatcher-sub")
	if err != nil {
		t.Fatal(err)
	}
	if profile.UserID != "D1" || len(profile.Roles) != 1 || profile.Roles[0] != "DISPATCHER" {
		t.Fatalf("unexpected application profile: %+v", profile)
	}
}
