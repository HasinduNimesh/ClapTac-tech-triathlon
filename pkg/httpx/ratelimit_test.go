package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
)

type countingLimiter struct {
	hits map[string]int
	down bool
}

func (c *countingLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, time.Duration) {
	if c.down {
		return true, 0
	}
	c.hits[key]++
	return c.hits[key] <= limit, 30 * time.Second
}

type tokenAuth struct{}

func (tokenAuth) Authenticate(r *http.Request) (*auth.Principal, error) {
	if r.Header.Get("Authorization") == "Bearer good" {
		return &auth.Principal{Subject: "u1"}, nil
	}
	return nil, errors.New("bad token")
}

func serve(rl *RateLimit, path, authz string) *httptest.ResponseRecorder {
	h := rl.middleware("svc")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "10.0.0.9"
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimitIsPerUserAndAnonymousHasItsOwnLimit(t *testing.T) {
	l := &countingLimiter{hits: map[string]int{}}
	rl := &RateLimit{Limiter: l, Authn: tokenAuth{}, PerUser: 2, PerAnonymous: 1}

	for i := 0; i < 2; i++ {
		if got := serve(rl, "/api/v1/x", "Bearer good").Code; got != http.StatusOK {
			t.Fatalf("signed-in request %d: %d", i+1, got)
		}
	}
	rec := serve(rl, "/api/v1/x", "Bearer good")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "30" {
		t.Fatalf("third signed-in request should be limited with Retry-After, got %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	// A bad token and no token share the stricter per-address allowance and do not use the user's.
	if serve(rl, "/api/v1/x", "Bearer forged").Code != http.StatusOK {
		t.Fatal("first anonymous request should pass")
	}
	if serve(rl, "/api/v1/x", "").Code != http.StatusTooManyRequests {
		t.Fatal("second anonymous request from the same address should be limited")
	}
}

func TestRateLimitSkipsHealthAndMetricsAndFailsOpen(t *testing.T) {
	l := &countingLimiter{hits: map[string]int{}}
	rl := &RateLimit{Limiter: l, Authn: tokenAuth{}, PerUser: 1, PerAnonymous: 1}
	for i := 0; i < 5; i++ {
		if serve(rl, "/health/ready", "").Code != http.StatusOK || serve(rl, "/metrics", "").Code != http.StatusOK {
			t.Fatal("health and metrics must never be limited")
		}
	}
	l.down = true
	for i := 0; i < 5; i++ {
		if serve(rl, "/api/v1/x", "Bearer good").Code != http.StatusOK {
			t.Fatal("a limiter outage must not block requests")
		}
	}
	if (*RateLimit)(nil).middleware("svc") == nil {
		t.Fatal("a nil RateLimit must be a pass-through")
	}
}
