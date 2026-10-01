package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
)

func TestResolverPreservesOperationalProfileScope(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer loader-session" {
			t.Errorf("authorization header = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"profile":{"userId":"USR004","subject":"usr-loader","roles":["LOADER"],"depot":"DEPOT_NORTH"}}`)),
		}, nil
	})}

	ctx := auth.WithBearer(context.Background(), "Bearer loader-session")
	profile, err := (Resolver{Shared: Shared{BaseURL: "http://shared.test", HTTP: httpClient}}).Resolve(ctx, "usr-loader")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if profile.Depot != "DEPOT_NORTH" {
		t.Fatalf("loader depot = %q, want DEPOT_NORTH", profile.Depot)
	}
	if profile.UserID != "USR004" || len(profile.Roles) != 1 || profile.Roles[0] != "LOADER" {
		t.Fatalf("profile identity/roles were not preserved: %+v", profile)
	}
}
