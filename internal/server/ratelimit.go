package server

import (
	"context"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

// RateLimiter provides DB-backed sliding-window rate limiting.
type RateLimiter struct {
	store  *db.Store
	limit  int
	window time.Duration
}

// NewRateLimiter creates a rate limiter that allows limit requests per window.
func NewRateLimiter(store *db.Store, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{store: store, limit: limit, window: window}
}

// Allow returns true if the request identified by key is within the rate limit.
// It records the attempt when allowed.
func (rl *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().Unix()
	windowStart := now - int64(rl.window.Seconds())
	count, err := rl.store.CountRateLimitEntries(ctx, key, windowStart)
	if err != nil {
		return false, err
	}
	if count >= int64(rl.limit) {
		return false, nil
	}
	if err := rl.store.InsertRateLimitEntry(ctx, key, now); err != nil {
		return false, err
	}
	return true, nil
}

// StartCleanup runs periodic cleanup of expired rate limit entries.
func (rl *RateLimiter) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Unix() - int64(rl.window.Seconds()) - 60
				_ = rl.store.CleanupRateLimitEntries(ctx, cutoff)
			}
		}
	}()
}
