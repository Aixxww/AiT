package local

import (
	"sync"
	"time"
)

// Default TTL constants for local data cache
const (
	CacheTTLTicker  = 60 * time.Second  // 24h ticker data (price, volume, trades)
	CacheTTLHistory = 300 * time.Second // OI history, LSR, taker ratio (5 min)
	CacheTTLScore   = 120 * time.Second // AI500 composite scores (2 min)
	CacheTTLHunter  = 180 * time.Second // Hunter composite scores (3 min)
)

// cacheEntry represents a single cached item with expiration time
type cacheEntry struct {
	data      interface{}
	expiresAt time.Time
}

// Cache is a TTL-enabled thread-safe cache
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

// NewCache creates a new empty *Cache and starts a background cleaner.
func NewCache() *Cache {
	c := &Cache{
		entries: make(map[string]cacheEntry),
	}
	// Background cleaner runs every 60s and drops expired entries
	go c.cleaner()
	return c
}

// Get returns cached data for key if it exists and has not expired.
// Returns (data, true) on hit, (nil, false) on miss or expiry.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		// Entry expired — delete and return miss
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.data, true
}

// Set stores data under key with the given time-to-live.
func (c *Cache) Set(key string, data interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
}

// cleaner removes expired cache entries every 60 seconds.
func (c *Cache) cleaner() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for key, entry := range c.entries {
			if now.After(entry.expiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}
