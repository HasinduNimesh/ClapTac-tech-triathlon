// Package cache is a thin, fail-open Redis client. Every method works on a nil or unconfigured
// Cache and treats any Redis problem as "not there", so a Redis outage can slow a request down
// but never fail it. PostgreSQL stays the source of truth for everything.
package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	opTimeout = 150 * time.Millisecond
	// After a Redis error, skip Redis for this long so a down server costs one timeout, not one per request.
	backoff = 5 * time.Second
)

type Cache struct {
	client    *redis.Client
	logger    *slog.Logger
	downUntil atomic.Int64 // unix nanoseconds
}

// New returns a Cache for the given redis:// URL. An empty or invalid URL gives a disabled Cache.
func New(url string, logger *slog.Logger) *Cache {
	c := &Cache{logger: logger}
	if url == "" {
		return c
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		if logger != nil {
			logger.Warn("redis_url_invalid", "error", err.Error())
		}
		return c
	}
	opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout = opTimeout, opTimeout, opTimeout
	opts.MaxRetries = 0
	c.client = redis.NewClient(opts)
	return c
}

// Enabled reports whether a Redis client is configured (not whether the server is up).
func (c *Cache) Enabled() bool { return c != nil && c.client != nil }

func (c *Cache) usable() bool {
	return c.Enabled() && time.Now().UnixNano() >= c.downUntil.Load()
}

func (c *Cache) failed(op string, err error) {
	if err == nil || err == redis.Nil {
		return
	}
	if c.downUntil.Swap(time.Now().Add(backoff).UnixNano()) < time.Now().UnixNano() && c.logger != nil {
		c.logger.Warn("redis_unavailable", "op", op, "error", err.Error())
	}
}

// Ping checks the server; for health reporting only.
func (c *Cache) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return c.client.Ping(ctx).Err()
}

// Allow counts one hit against key in a fixed window and reports whether it is within limit.
// It fails open: when Redis is unavailable every hit is allowed.
func (c *Cache) Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration) {
	if !c.usable() || limit <= 0 {
		return true, 0
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	pipe := c.client.Pipeline()
	count := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	ttl := pipe.PTTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		c.failed("allow", err)
		return true, 0
	}
	if int(count.Val()) <= limit {
		return true, 0
	}
	wait := ttl.Val()
	if wait <= 0 {
		wait = window
	}
	return false, wait
}

// GetJSON loads key into v and reports whether it was found.
func (c *Cache) GetJSON(ctx context.Context, key string, v any) bool {
	if !c.usable() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		c.failed("get", err)
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

// SetJSON stores v under key for ttl. Failures are ignored.
func (c *Cache) SetJSON(ctx context.Context, key string, v any, ttl time.Duration) {
	if !c.usable() {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	c.failed("set", c.client.Set(ctx, key, raw, ttl).Err())
}

// Delete removes keys. Failures are ignored.
func (c *Cache) Delete(ctx context.Context, keys ...string) {
	if !c.usable() || len(keys) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	c.failed("del", c.client.Del(ctx, keys...).Err())
}

// Close releases the connection pool.
func (c *Cache) Close() {
	if c.Enabled() {
		_ = c.client.Close()
	}
}
