package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newRedisTestClient mirrors the helper in internal/auth: connect to
// REDIS_TEST_URL, flush, and skip when unset so `go test ./...` works
// on a stock laptop without a Redis instance running.
func newRedisTestClient(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		t.Skip("REDIS_TEST_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse REDIS_TEST_URL: %v", err)
	}
	rdb := redis.NewClient(opts)
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("REDIS_TEST_URL unreachable: %v", err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushdb: %v", err)
	}
	t.Cleanup(func() {
		rdb.FlushDB(context.Background())
		rdb.Close()
	})
	return rdb
}

func TestEmptyClaimCache_NilSafe(t *testing.T) {
	var c *EmptyClaimCache // nil
	ctx := context.Background()

	if c.IsEmpty(ctx, "any-runtime") {
		t.Fatal("nil cache must report not-empty (cache miss)")
	}
	c.MarkEmpty(ctx, "any-runtime")
	c.Invalidate(ctx, "any-runtime")
}

func TestNewEmptyClaimCache_NilRedisReturnsNil(t *testing.T) {
	if c := NewEmptyClaimCache(nil); c != nil {
		t.Fatalf("NewEmptyClaimCache(nil) must return nil, got %#v", c)
	}
}

func TestEmptyClaimCache_EmptyRuntimeIDIsNoOp(t *testing.T) {
	rdb := newRedisTestClient(t)
	c := NewEmptyClaimCache(rdb)
	ctx := context.Background()

	c.MarkEmpty(ctx, "")
	if c.IsEmpty(ctx, "") {
		t.Fatal("empty runtime ID must not hit cache")
	}
	c.Invalidate(ctx, "")
}

func TestEmptyClaimCache_MarkIsEmptyInvalidate(t *testing.T) {
	rdb := newRedisTestClient(t)
	c := NewEmptyClaimCache(rdb)
	ctx := context.Background()

	if c.IsEmpty(ctx, "rt-1") {
		t.Fatal("expected miss before mark")
	}
	c.MarkEmpty(ctx, "rt-1")
	if !c.IsEmpty(ctx, "rt-1") {
		t.Fatal("expected hit after mark")
	}
	c.Invalidate(ctx, "rt-1")
	if c.IsEmpty(ctx, "rt-1") {
		t.Fatal("expected miss after invalidate")
	}
}

func TestEmptyClaimCache_TTL(t *testing.T) {
	rdb := newRedisTestClient(t)
	c := NewEmptyClaimCache(rdb)
	ctx := context.Background()

	c.MarkEmpty(ctx, "rt-ttl")
	ttl, err := rdb.TTL(ctx, emptyClaimCacheKey("rt-ttl")).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > EmptyClaimCacheTTL+time.Second {
		t.Fatalf("unexpected TTL %v (want ~%v)", ttl, EmptyClaimCacheTTL)
	}
}

func TestEmptyClaimCache_RuntimeIsolation(t *testing.T) {
	rdb := newRedisTestClient(t)
	c := NewEmptyClaimCache(rdb)
	ctx := context.Background()

	c.MarkEmpty(ctx, "rt-A")
	if c.IsEmpty(ctx, "rt-B") {
		t.Fatal("marking rt-A must not affect rt-B")
	}
	c.Invalidate(ctx, "rt-A")
	c.MarkEmpty(ctx, "rt-B")
	if c.IsEmpty(ctx, "rt-A") {
		t.Fatal("marking rt-B must not affect rt-A")
	}
}
