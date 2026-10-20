package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestDisabledCacheNeverBlocksAnything(t *testing.T) {
	for _, c := range []*Cache{nil, New("", nil), New("not a url", nil)} {
		ctx := context.Background()
		if ok, _ := c.Allow(ctx, "k", 1, time.Minute); !ok {
			t.Fatal("a disabled cache must allow every request")
		}
		var v string
		if c.GetJSON(ctx, "k", &v) {
			t.Fatal("a disabled cache has nothing stored")
		}
		c.SetJSON(ctx, "k", "x", time.Minute)
		c.Delete(ctx, "k")
		if err := c.Ping(ctx); err != nil {
			t.Fatalf("ping on a disabled cache: %v", err)
		}
	}
}

func TestAllowCountsWithinAWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	c := New("redis://"+mr.Addr()+"/0", nil)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if ok, _ := c.Allow(ctx, "rl:a", 3, time.Minute); !ok {
			t.Fatalf("hit %d should be allowed", i+1)
		}
	}
	ok, wait := c.Allow(ctx, "rl:a", 3, time.Minute)
	if ok || wait <= 0 {
		t.Fatalf("fourth hit should be refused with a wait, got ok=%v wait=%v", ok, wait)
	}
	if ok, _ := c.Allow(ctx, "rl:b", 3, time.Minute); !ok {
		t.Fatal("another key has its own allowance")
	}
	mr.FastForward(time.Minute + time.Second)
	if ok, _ := c.Allow(ctx, "rl:a", 3, time.Minute); !ok {
		t.Fatal("the allowance should reset after the window")
	}
}

func TestJSONRoundTripAndDelete(t *testing.T) {
	mr := miniredis.RunT(t)
	c := New("redis://"+mr.Addr()+"/0", nil)
	ctx := context.Background()
	type point struct{ Lat, Lon float64 }
	c.SetJSON(ctx, "loc:t1", point{6.9, 79.8}, time.Minute)
	var got point
	if !c.GetJSON(ctx, "loc:t1", &got) || got.Lat != 6.9 {
		t.Fatalf("round trip failed: %+v", got)
	}
	c.Delete(ctx, "loc:t1")
	if c.GetJSON(ctx, "loc:t1", &got) {
		t.Fatal("deleted key should be gone")
	}
}

func TestRedisOutageFailsOpenAndBacksOff(t *testing.T) {
	mr := miniredis.RunT(t)
	c := New("redis://"+mr.Addr()+"/0", nil)
	ctx := context.Background()
	mr.Close()

	start := time.Now()
	for i := 0; i < 20; i++ {
		if ok, _ := c.Allow(ctx, "rl:a", 1, time.Minute); !ok {
			t.Fatal("an outage must not refuse requests")
		}
		var v string
		if c.GetJSON(ctx, "k", &v) {
			t.Fatal("nothing can be read during an outage")
		}
	}
	// One failed attempt, then Redis is skipped; 40 calls must not each pay a timeout.
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("calls during an outage took %v; the back-off is not working", elapsed)
	}
}
