package traderutil

import (
	"sync"
	"time"
)

// TimedCache is a goroutine-safe, time-based cache for a single value.
// It replaces the duplicated RLock/check-TTL/RUnlock → fetch → Lock/store/Unlock
// pattern found in every exchange adapter's GetBalance / GetPositions.
type TimedCache[T any] struct {
	mu       sync.RWMutex
	value    T
	updated  time.Time
	duration time.Duration
	hasValue bool
}

// NewTimedCache creates a cache with the given TTL.
func NewTimedCache[T any](ttl time.Duration) *TimedCache[T] {
	return &TimedCache[T]{duration: ttl}
}

// Get returns the cached value if it exists and has not expired.
// ok is false when the caller should fetch a fresh value and call Set.
func (c *TimedCache[T]) Get() (value T, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.hasValue && time.Since(c.updated) < c.duration {
		return c.value, true
	}
	var zero T
	return zero, false
}

// Set stores a fresh value and resets the expiry clock.
func (c *TimedCache[T]) Set(value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = value
	c.updated = time.Now()
	c.hasValue = true
}

// Invalidate clears the cache so the next Get returns ok=false.
func (c *TimedCache[T]) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hasValue = false
	c.updated = time.Time{}
}

// GetStale returns the last cached value even if expired (useful during
// temporary API outages such as IP bans). ok is false only when no value
// has ever been stored.
func (c *TimedCache[T]) GetStale() (value T, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.hasValue {
		return c.value, true
	}
	var zero T
	return zero, false
}
