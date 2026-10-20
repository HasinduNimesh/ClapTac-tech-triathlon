package httpx

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
)

// Limiter counts hits per key. cache.Cache implements it and fails open, so a limiter outage never blocks traffic.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration)
}

// RateLimit caps requests per signed-in user, and per client address for requests with no valid token.
// NGINX still limits every address at the edge; this adds a per-user allowance shared by all instances.
type RateLimit struct {
	Limiter      Limiter
	Authn        auth.Authenticator
	PerUser      int // requests per minute for one subject
	PerAnonymous int // requests per minute for one address with a missing or invalid token
}

const rateWindow = time.Minute

func (rl *RateLimit) middleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if rl == nil || rl.Limiter == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/health/") || r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}
			key, limit := "rl:"+service+":ip:"+r.RemoteAddr, rl.PerAnonymous
			if rl.Authn != nil && r.Header.Get("Authorization") != "" {
				if p, err := rl.Authn.Authenticate(r); err == nil && p != nil && p.Subject != "" {
					key, limit = "rl:"+service+":u:"+p.Subject, rl.PerUser
				}
			}
			if ok, wait := rl.Limiter.Allow(r.Context(), key, limit, rateWindow); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many requests. Wait a moment and try again.", "code": "rate_limited"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
