package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// emptyClaimCachePrefix namespaces empty-claim keys away from realtime
// (ws:*) and auth (mul:auth:*) keys.
const emptyClaimCachePrefix = "mul:claim:runtime:empty:"

// EmptyClaimCacheTTL bounds how long a cached "no queued task" verdict
// stays believable. Choice tradeoff: too long means a missed
// invalidation delays claim until the TTL expires; too short means the
// fast path almost never triggers. 30s matches DefaultPollInterval so
// the worst-case staleness is one extra poll cycle — already the
// no-cache baseline — while still collapsing the steady-state warm
// empty path to a single Redis GET.
const EmptyClaimCacheTTL = 30 * time.Second

// EmptyClaimCache caches "this runtime currently has no queued task"
// so the daemon's poll-based claim path can short-circuit before
// hitting Postgres. Only the negative result is cached; positive
// results always re-check the DB so concurrent claimers race fairly
// in `ClaimAgentTask`'s `FOR UPDATE SKIP LOCKED`.
//
// The cache is invalidated synchronously on every enqueue (see
// TaskService.notifyTaskAvailable). A nil *EmptyClaimCache is safe to
// use — every method becomes a no-op or reports a cache miss, so
// single-node dev / tests with no REDIS_URL degrade cleanly to direct
// DB lookups.
type EmptyClaimCache struct {
	rdb *redis.Client
}

// NewEmptyClaimCache returns a cache backed by rdb. Pass nil to
// disable caching; the returned *EmptyClaimCache is safe to call but
// never hits Redis.
func NewEmptyClaimCache(rdb *redis.Client) *EmptyClaimCache {
	if rdb == nil {
		return nil
	}
	return &EmptyClaimCache{rdb: rdb}
}

func emptyClaimCacheKey(runtimeID string) string {
	return emptyClaimCachePrefix + runtimeID
}

// IsEmpty reports whether a recent cached check confirmed the runtime
// had no queued task. Returns false on cache miss or any Redis error —
// a dead Redis must not stop legitimate claims.
func (c *EmptyClaimCache) IsEmpty(ctx context.Context, runtimeID string) bool {
	if c == nil || runtimeID == "" {
		return false
	}
	_, err := c.rdb.Get(ctx, emptyClaimCacheKey(runtimeID)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn("empty_claim_cache: get failed; falling back to DB", "error", err)
		}
		return false
	}
	return true
}

// MarkEmpty stores the empty verdict for the given runtime with the
// default TTL. Errors are logged and swallowed — a cache write
// failure is not a request failure.
func (c *EmptyClaimCache) MarkEmpty(ctx context.Context, runtimeID string) {
	if c == nil || runtimeID == "" {
		return
	}
	if err := c.rdb.Set(ctx, emptyClaimCacheKey(runtimeID), "1", EmptyClaimCacheTTL).Err(); err != nil {
		slog.Warn("empty_claim_cache: set failed", "error", err)
	}
}

// Invalidate removes the empty verdict for the given runtime. Called
// from every enqueue path so a newly queued task is claimable
// immediately rather than waiting for the TTL to expire.
//
// Invalidation MUST run before the daemon WS wakeup is published —
// otherwise the wakeup arrives, the daemon claims, and the still-cached
// empty key returns null while the task sits queued for up to one full
// TTL window.
func (c *EmptyClaimCache) Invalidate(ctx context.Context, runtimeID string) {
	if c == nil || runtimeID == "" {
		return
	}
	if err := c.rdb.Del(ctx, emptyClaimCacheKey(runtimeID)).Err(); err != nil {
		slog.Warn("empty_claim_cache: invalidate failed; entry will expire on TTL", "error", err)
	}
}
