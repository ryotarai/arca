package arcad

import (
	"sync"
	"time"
)

type sessionValidationCacheKey struct {
	sessionID     string
	host          string
	ownerOnlyArca bool
}

type sessionValidationCacheEntry struct {
	expiresAt time.Time
}

type SessionValidationCache struct {
	mu    sync.RWMutex
	items map[sessionValidationCacheKey]sessionValidationCacheEntry
	now   func() time.Time
	ttl   time.Duration
}

func NewSessionValidationCache(ttl time.Duration) *SessionValidationCache {
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &SessionValidationCache{
		items: make(map[sessionValidationCacheKey]sessionValidationCacheEntry),
		now:   time.Now,
		ttl:   ttl,
	}
}

func (c *SessionValidationCache) IsValid(sessionID, host string, ownerOnlyArca bool) bool {
	if c == nil {
		return false
	}
	key := sessionValidationCacheKey{sessionID: sessionID, host: host, ownerOnlyArca: ownerOnlyArca}
	now := c.now()
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	return ok && now.Before(entry.expiresAt)
}

func (c *SessionValidationCache) MarkValid(sessionID, host string, ownerOnlyArca bool) {
	if c == nil {
		return
	}
	key := sessionValidationCacheKey{sessionID: sessionID, host: host, ownerOnlyArca: ownerOnlyArca}
	c.mu.Lock()
	c.items[key] = sessionValidationCacheEntry{expiresAt: c.now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *SessionValidationCache) Invalidate(sessionID, host string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.items {
		if key.sessionID == sessionID && (host == "" || key.host == host) {
			delete(c.items, key)
		}
	}
}
