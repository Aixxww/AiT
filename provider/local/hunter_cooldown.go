package local

import (
	"sync"
	"time"
)

// symbolCooldown tracks per-symbol selection history for smart cooldown.
type symbolCooldown struct {
	ConsecutiveWaits  int       // how many cycles symbol was a candidate but AI said "wait"
	Selections24h     int       // times selected in last 24h
	LastSelected      time.Time // last time this symbol was selected
	OIFilteredCount   int       // times filtered by OI threshold
	VolatileTrapUntil time.Time // if set, exclude until this time
}

// cooldownTracker is a global in-memory tracker (reset on process restart).
type cooldownTracker struct {
	mu      sync.RWMutex
	symbols map[string]*symbolCooldown
}

var globalCooldown = &cooldownTracker{
	symbols: make(map[string]*symbolCooldown),
}

// getCooldownMultiplier returns score modifier for a symbol.
// Returns 1.0 (no penalty), 0.50 (consecutive waits), or 0.0 (volatile trap excluded).
func (ct *cooldownTracker) getCooldownMultiplier(symbol string) float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	sc, ok := ct.symbols[symbol]
	if !ok {
		return 1.0
	}
	// Volatile trap: exclude for 24h
	if !sc.VolatileTrapUntil.IsZero() && time.Now().Before(sc.VolatileTrapUntil) {
		return 0.0
	}
	// Consecutive waits penalty
	if sc.ConsecutiveWaits >= 2 {
		return 0.50
	}
	return 1.0
}

// recordWait increments consecutive_waits for a symbol.
func (ct *cooldownTracker) recordWait(symbol string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	sc, ok := ct.symbols[symbol]
	if !ok {
		sc = &symbolCooldown{}
		ct.symbols[symbol] = sc
	}
	sc.ConsecutiveWaits++
	// If 24h selections > 5, tag as volatile trap
	if sc.Selections24h > 5 {
		sc.VolatileTrapUntil = time.Now().Add(24 * time.Hour)
	}
}

// recordSelection resets consecutive_waits and increments 24h count.
func (ct *cooldownTracker) recordSelection(symbol string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	sc, ok := ct.symbols[symbol]
	if !ok {
		sc = &symbolCooldown{}
		ct.symbols[symbol] = sc
	}
	sc.ConsecutiveWaits = 0
	sc.Selections24h++
	sc.LastSelected = time.Now()
}

// getOIThresholdReduction returns a reduced OI threshold multiplier for
// coins that have been OI-filtered multiple times (anti-repeat).
func (ct *cooldownTracker) getOIThresholdReduction(symbol string) float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	sc, ok := ct.symbols[symbol]
	if !ok || sc.OIFilteredCount < 3 {
		return 1.0
	}
	return 0.80 // 20% lower OI threshold
}

func (ct *cooldownTracker) recordOIFilter(symbol string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	sc, ok := ct.symbols[symbol]
	if !ok {
		sc = &symbolCooldown{}
		ct.symbols[symbol] = sc
	}
	sc.OIFilteredCount++
}
