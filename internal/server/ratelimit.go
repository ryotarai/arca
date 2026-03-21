package server

import (
	"context"
	"log/slog"
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

// Check returns true if the key is within the rate limit (read-only, does not record).
func (rl *RateLimiter) Check(ctx context.Context, key string) (bool, error) {
	now := time.Now().Unix()
	windowStart := now - int64(rl.window.Seconds())
	count, err := rl.store.CountRateLimitEntries(ctx, key, windowStart)
	if err != nil {
		return false, err
	}
	return count < int64(rl.limit), nil
}

// Record records an attempt for the given key.
func (rl *RateLimiter) Record(ctx context.Context, key string) error {
	return rl.store.InsertRateLimitEntry(ctx, key, time.Now().Unix())
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
				if err := rl.store.CleanupRateLimitEntries(ctx, cutoff); err != nil {
					slog.Warn("rate limit cleanup failed", "error", err)
				}
			}
		}
	}()
}
