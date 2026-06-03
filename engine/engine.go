package engine

import (
	"context"
	"fmt"
	"nofx/datafetch"
	"sync"
	"time"
)

// ============================================================================
// Main Engine
// Runs the complete scoring cycle: fetch → compute → route → output.
// ============================================================================

// MainEngine is the top-level scoring engine.
type MainEngine struct {
	store  *datafetch.Store
	hub    *IndicatorHub
	router *SignalRouter
	cfg    HubConfig

	lastSignals []*TradeSignal
	mu          sync.RWMutex
}

// NewMainEngine creates a new MainEngine with the given store and config.
func NewMainEngine(store *datafetch.Store, cfg HubConfig) *MainEngine {
	hub := NewIndicatorHub(store, cfg)
	router := NewSignalRouter(hub, cfg)

	return &MainEngine{
		store:  store,
		hub:    hub,
		router: router,
		cfg:    cfg,
	}
}

// NewMainEngineWithDeps creates a MainEngine with pre-built dependencies (for testing).
func NewMainEngineWithDeps(store *datafetch.Store, hub *IndicatorHub, router *SignalRouter, cfg HubConfig) *MainEngine {
	return &MainEngine{
		store:  store,
		hub:    hub,
		router: router,
		cfg:    cfg,
	}
}

// RunCycle executes one complete scoring cycle.
func (e *MainEngine) RunCycle() ([]*TradeSignal, error) {
	snap := e.store.Current()
	if snap == nil {
		return nil, fmt.Errorf("no snapshot available")
	}

	if len(snap.Symbols) == 0 {
		return nil, fmt.Errorf("no symbols in snapshot")
	}

	// Score all symbols
	sets := e.hub.ScoreAll()
	if len(sets) == 0 {
		return nil, fmt.Errorf("no symbols scored")
	}

	// Route scored indicators to trade signals
	signals := e.router.Route(sets)

	// Store latest signals
	e.mu.Lock()
	e.lastSignals = signals
	e.mu.Unlock()

	return signals, nil
}

// GetLatestSignals returns the most recent scoring cycle results.
func (e *MainEngine) GetLatestSignals() []*TradeSignal {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(e.lastSignals) == 0 {
		return nil
	}

	// Return a copy to prevent external mutation
	result := make([]*TradeSignal, len(e.lastSignals))
	copy(result, e.lastSignals)
	return result
}

// GetLatestSignalsByGrade returns signals filtered by minimum grade.
func (e *MainEngine) GetLatestSignalsByGrade(minGrade Grade) []*TradeSignal {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*TradeSignal, 0)
	for _, sig := range e.lastSignals {
		if sig.Grade >= minGrade {
			result = append(result, sig)
		}
	}
	return result
}

// GetSignalBySymbol returns the latest signal for a specific symbol, or nil.
func (e *MainEngine) GetSignalBySymbol(symbol string) *TradeSignal {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, sig := range e.lastSignals {
		if sig.Symbol == symbol {
			return sig
		}
	}
	return nil
}

// RecordTrade notifies the router that a trade was executed (for cooldown).
func (e *MainEngine) RecordTrade(symbol string) {
	e.router.RecordTrade(symbol)
}

// Start runs periodic scoring cycles until the context is cancelled.
func (e *MainEngine) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			signals, err := e.RunCycle()
			if err != nil {
				continue
			}
			_ = signals // consumed via GetLatestSignals()
		}
	}
}

// StartWithCallback runs periodic scoring cycles and calls the callback with results.
func (e *MainEngine) StartWithCallback(ctx context.Context, interval time.Duration, callback func(signals []*TradeSignal, err error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once immediately
	signals, err := e.RunCycle()
	if callback != nil {
		callback(signals, err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			signals, err := e.RunCycle()
			if callback != nil {
				callback(signals, err)
			}
		}
	}
}

// Config returns the current engine configuration.
func (e *MainEngine) Config() HubConfig {
	return e.cfg
}

// Hub returns the indicator hub.
func (e *MainEngine) Hub() *IndicatorHub {
	return e.hub
}

// Router returns the signal router.
func (e *MainEngine) Router() *SignalRouter {
	return e.router
}
