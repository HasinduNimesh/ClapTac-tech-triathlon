package authorization

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type memKV map[string][]byte

func (m memKV) GetJSON(_ context.Context, k string, v any) bool {
	raw, ok := m[k]
	return ok && json.Unmarshal(raw, v) == nil
}
func (m memKV) SetJSON(_ context.Context, k string, v any, _ time.Duration) {
	m[k], _ = json.Marshal(v)
}

type countingResolver struct {
	calls   int
	profile Profile
}

func (c *countingResolver) Resolve(_ context.Context, subject string) (*Profile, error) {
	c.calls++
	p := c.profile
	p.Subject = subject
	return &p, nil
}

func TestCachedResolverAsksOnceThenRemembers(t *testing.T) {
	inner := &countingResolver{profile: Profile{UserID: "u1", Roles: []string{"DRIVER"}, VehicleID: "VEH035"}}
	r := CachedResolver{Inner: inner, Cache: memKV{}, TTL: time.Minute}
	for i := 0; i < 3; i++ {
		p, err := r.Resolve(context.Background(), "sub-1")
		if err != nil || p.VehicleID != "VEH035" {
			t.Fatalf("resolve %d: %+v %v", i, p, err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("shared-service should be asked once, was asked %d times", inner.calls)
	}
	if _, _ = r.Resolve(context.Background(), "sub-2"); inner.calls != 2 {
		t.Fatal("another user must not reuse the first user's profile")
	}
}

func TestCachedResolverNeverRemembersAnEmptyProfile(t *testing.T) {
	inner := &countingResolver{} // no roles: the fallback profile
	r := CachedResolver{Inner: inner, Cache: memKV{}, TTL: time.Minute}
	_, _ = r.Resolve(context.Background(), "sub-1")
	_, _ = r.Resolve(context.Background(), "sub-1")
	if inner.calls != 2 {
		t.Fatalf("a profile with no roles must be looked up every time, calls=%d", inner.calls)
	}
}

func TestCachedResolverIsAPassThroughWhenDisabled(t *testing.T) {
	inner := &countingResolver{profile: Profile{Roles: []string{"DRIVER"}}}
	for _, r := range []CachedResolver{{Inner: inner}, {Inner: inner, Cache: memKV{}, TTL: 0}} {
		_, _ = r.Resolve(context.Background(), "s")
		_, _ = r.Resolve(context.Background(), "s")
	}
	if inner.calls != 4 {
		t.Fatalf("no caching expected, calls=%d", inner.calls)
	}
}
