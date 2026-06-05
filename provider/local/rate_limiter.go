package local

import (
	"sync"
	"sync/atomic"
	"time"
)

// BinanceRateLimiter provides weight-based rate limiting for Binance Futures API.
// Binance enforces a 1200 weight/minute limit per IP. This limiter tracks
// accumulated weight per 60-second window and blocks when the budget would
// be exceeded, then auto-resets when the window expires.
//
// A semaphore controls maximum concurrent in-flight requests (default 15).
type BinanceRateLimiter struct {
	weightUsed  atomic.Int64
	weightLimit int64         // 1200 default
	windowStart atomic.Int64  // unix timestamp (seconds) of current window start
	semaphore   chan struct{} // concurrency limit
	mu          sync.Mutex    // protects waiters and window reset
}

// NewBinanceRateLimiter creates a new rate limiter.
// maxConcurrent controls how many goroutines may hold API calls simultaneously.
// Default weight limit is 1200 per 60-second window.
func NewBinanceRateLimiter(maxConcurrent int) *BinanceRateLimiter {
	if maxConcurrent <= 0 {
		maxConcurrent = 15
	}
	rl := &BinanceRateLimiter{
		weightLimit: 1200,
		semaphore:   make(chan struct{}, maxConcurrent),
	}
	rl.windowStart.Store(time.Now().Unix())
	return rl
}

// Acquire blocks until there is weight budget for the requested weight.
// It first acquires a semaphore slot (may block if at concurrency limit),
// then checks the weight budget. If the budget would be exceeded, it sleeps
// until the 60-second window resets.
//
// After Acquire returns, the caller MUST call Release() when the API call
// completes, and RecordWeight(w) to register the consumed weight.
func (rl *BinanceRateLimiter) Acquire(weight int) {
	// Acquire semaphore slot (blocks if at concurrency limit)
	rl.semaphore <- struct{}{}

	// Check weight budget
	now := time.Now().Unix()
	rl.mu.Lock()

	// Reset window if we've crossed the 60s boundary
	windowStart := rl.windowStart.Load()
	if now-windowStart >= 60 {
		rl.weightUsed.Store(0)
		rl.windowStart.Store(now)
		windowStart = now
	}

	// If budget exceeded, wait for window reset
	used := rl.weightUsed.Load()
	if used+int64(weight) > rl.weightLimit {
		remaining := 60 - (now - windowStart)
		if remaining <= 0 {
			remaining = 1
		}
		rl.mu.Unlock()
		time.Sleep(time.Duration(remaining) * time.Second)
		// After sleep, reset the window
		rl.mu.Lock()
		rl.weightUsed.Store(0)
		rl.windowStart.Store(time.Now().Unix())
	}

	rl.mu.Unlock()
}

// Release releases one semaphore slot back to the pool.
// Call this after every Acquire + API call, regardless of success/failure.
func (rl *BinanceRateLimiter) Release() {
	select {
	case <-rl.semaphore:
	default:
		// Shouldn't happen if Acquire/Release are paired correctly
	}
}

// RecordWeight records weight consumption from a completed API call.
// Binance returns X-MBX-USED-WEIGHT-1M in response headers, but we
// track it client-side using known weight costs.
func (rl *BinanceRateLimiter) RecordWeight(w int) {
	// Check for window reset before recording
	now := time.Now().Unix()
	windowStart := rl.windowStart.Load()
	if now-windowStart >= 60 {
		rl.mu.Lock()
		rl.weightUsed.Store(0)
		rl.windowStart.Store(now)
		rl.mu.Unlock()
	}
	rl.weightUsed.Add(int64(w))
}
