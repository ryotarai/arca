package machine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

type ipCacheEntry struct {
	privateIP  string
	publicIP   string
	fetchedAt  time.Time
	refreshing bool
}

// MachineIPCache provides an in-memory cache for machine IP addresses
// with a TTL and stale-while-revalidate pattern.
type MachineIPCache struct {
	runtime Runtime
	store   TemplateCatalogStore
	mu      sync.Mutex
	entries map[string]*ipCacheEntry
	ttl     time.Duration
}

func NewMachineIPCache(runtime Runtime, store TemplateCatalogStore, ttl time.Duration) *MachineIPCache {
	return &MachineIPCache{
		runtime: runtime,
		store:   store,
		entries: make(map[string]*ipCacheEntry),
		ttl:     ttl,
	}
}

// Get returns machine IP info. If cached and fresh, returns immediately.
// If cached but stale, returns the stale value and kicks off a background refresh.
// If not cached, fetches synchronously.
func (c *MachineIPCache) Get(ctx context.Context, machine db.Machine) (*RuntimeMachineInfo, error) {
	c.mu.Lock()
	entry, ok := c.entries[machine.ID]
	if ok && time.Since(entry.fetchedAt) < c.ttl {
		// Fresh cache hit
		c.mu.Unlock()
		return &RuntimeMachineInfo{PrivateIP: entry.privateIP, PublicIP: entry.publicIP}, nil
	}
	if ok {
		// Stale cache hit — return stale value, refresh in background
		info := &RuntimeMachineInfo{PrivateIP: entry.privateIP, PublicIP: entry.publicIP}
		if !entry.refreshing {
			entry.refreshing = true
			c.mu.Unlock()
			go c.refresh(machine)
		} else {
			c.mu.Unlock()
		}
		return info, nil
	}
	c.mu.Unlock()

	// No cache entry — fetch synchronously
	return c.fetchAndStore(ctx, machine)
}

// Invalidate removes the cache entry for a machine.
func (c *MachineIPCache) Invalidate(machineID string) {
	c.mu.Lock()
	delete(c.entries, machineID)
	c.mu.Unlock()
}

func (c *MachineIPCache) fetchAndStore(ctx context.Context, machine db.Machine) (*RuntimeMachineInfo, error) {
	info, err := c.runtime.GetMachineInfo(ctx, machine)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, nil
	}

	c.mu.Lock()
	c.entries[machine.ID] = &ipCacheEntry{
		privateIP: info.PrivateIP,
		publicIP:  info.PublicIP,
		fetchedAt: time.Now(),
	}
	c.mu.Unlock()
	return info, nil
}

func (c *MachineIPCache) refresh(machine db.Machine) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := c.runtime.GetMachineInfo(ctx, machine)
	if err != nil {
		slog.Warn("ip cache background refresh failed", "machine_id", machine.ID, "error", err)
		c.mu.Lock()
		if entry, ok := c.entries[machine.ID]; ok {
			entry.refreshing = false
		}
		c.mu.Unlock()
		return
	}

	c.mu.Lock()
	if info != nil {
		c.entries[machine.ID] = &ipCacheEntry{
			privateIP: info.PrivateIP,
			publicIP:  info.PublicIP,
			fetchedAt: time.Now(),
		}
	} else {
		if entry, ok := c.entries[machine.ID]; ok {
			entry.refreshing = false
		}
	}
	c.mu.Unlock()
}
