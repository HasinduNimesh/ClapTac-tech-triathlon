package authorization

import (
	"context"
	"time"
)

// KeyValue is the slice of cache.Cache that CachedResolver needs.
type KeyValue interface {
	GetJSON(ctx context.Context, key string, v any) bool
	SetJSON(ctx context.Context, key string, v any, ttl time.Duration)
}

// CachedResolver remembers resolved profiles for a short time, so a request does not cost a round trip to
// shared-service and a database read every time. A change to someone's roles, outlets or vehicle therefore
// takes up to TTL to reach the other services. Only a profile that has roles is stored: the empty fallback
// returned when shared-service refuses is never remembered, so a failed lookup is retried at once.
type CachedResolver struct {
	Inner ProfileResolver
	Cache KeyValue
	TTL   time.Duration
}

func (c CachedResolver) Resolve(ctx context.Context, subject string) (*Profile, error) {
	if c.Cache == nil || c.TTL <= 0 || subject == "" {
		return c.Inner.Resolve(ctx, subject)
	}
	key := "profile:" + subject
	var hit Profile
	if c.Cache.GetJSON(ctx, key, &hit) && hit.Subject == subject {
		return &hit, nil
	}
	p, err := c.Inner.Resolve(ctx, subject)
	if err == nil && p != nil && p.Subject == subject && len(p.Roles) > 0 {
		c.Cache.SetJSON(ctx, key, p, c.TTL)
	}
	return p, err
}
