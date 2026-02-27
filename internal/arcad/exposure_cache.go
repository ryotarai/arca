package arcad

import (
	"context"
	"sync"
	"time"
)

const exposureCacheTTL = 30 * time.Second

type exposureCacheEntry struct {
	exposure Exposure
	expires  time.Time
}

type ExposureCache struct {
	client ControlPlaneClient

	mu    sync.RWMutex
	items map[string]exposureCacheEntry
	now   func() time.Time
}

func NewExposureCache(client ControlPlaneClient) *ExposureCache {
	return &ExposureCache{
		client: client,
		items:  make(map[string]exposureCacheEntry),
		now:    time.Now,
	}
}

func (c *ExposureCache) GetByHost(ctx context.Context, host string) (Exposure, error) {
	now := c.now()
	c.mu.RLock()
	item, ok := c.items[host]
	c.mu.RUnlock()
	if ok && now.Before(item.expires) {
		return item.exposure, nil
	}

	exposure, err := c.client.GetExposureByHost(ctx, host)
	if err != nil {
		return Exposure{}, err
	}

	c.mu.Lock()
	c.items[host] = exposureCacheEntry{exposure: exposure, expires: now.Add(exposureCacheTTL)}
	c.mu.Unlock()
	return exposure, nil
}

func (c *ExposureCache) Invalidate(host string) {
	c.mu.Lock()
	delete(c.items, host)
	c.mu.Unlock()
}
