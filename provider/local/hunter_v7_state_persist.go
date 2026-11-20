package local

import (
	"encoding/json"
	"fmt"
	"sort"
)

// TA-3 (docs/tier-a-improvement-plan-20261120.md): cross-restart state for
// offline tools. The validation harness clones both stores per attempt,
// commits them only on an accepted round, and saves them after each round,
// so WATCH -> TRIGGERED progress and native watch promotion survive a
// restart. The live engine does not use this (its T2 store stays in memory).
//
// Times are stored as absolute timestamps: after downtime the first
// UpdateOutcomes expires or ends events by wall-clock time, as designed.
// V7T2Signal.QualityGates is a map[string]interface{}; after a round trip
// its numbers are float64 (JSON), which nothing in the lifecycle reads.

const v7StateSchemaVersion = 1

type v7T2StoreState struct {
	Version        int               `json:"version"`
	Events         []*V7T2Event      `json:"events"`
	LatestBySymDir map[string]string `json:"latest_by_sym_dir"` // symbol|direction -> event id
}

// MarshalState serialises the store's events and cooldown index (not its
// lifecycle params, which RunV7T2Module configures on every scan).
func (st *V7T2EventStore) MarshalState() ([]byte, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.marshalLocked()
}

func (st *V7T2EventStore) marshalLocked() ([]byte, error) {
	s := v7T2StoreState{Version: v7StateSchemaVersion, LatestBySymDir: make(map[string]string, len(st.latestBySymDir))}
	ids := make([]string, 0, len(st.events))
	for id := range st.events {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s.Events = append(s.Events, st.events[id])
	}
	for key, ev := range st.latestBySymDir {
		if ev != nil {
			s.LatestBySymDir[key] = ev.EventID
		}
	}
	return json.Marshal(s)
}

// LoadState replaces the store's events and cooldown index with a state
// produced by MarshalState. The lifecycle params are kept.
func (st *V7T2EventStore) LoadState(b []byte) error {
	var s v7T2StoreState
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("t2 store state: %w", err)
	}
	if s.Version != v7StateSchemaVersion {
		return fmt.Errorf("t2 store state: schema version %d, want %d", s.Version, v7StateSchemaVersion)
	}
	events := make(map[string]*V7T2Event, len(s.Events))
	for _, ev := range s.Events {
		if ev == nil || ev.EventID == "" {
			return fmt.Errorf("t2 store state: event without id")
		}
		events[ev.EventID] = ev
	}
	latest := make(map[string]*V7T2Event, len(s.LatestBySymDir))
	for key, id := range s.LatestBySymDir {
		ev, ok := events[id]
		if !ok {
			return fmt.Errorf("t2 store state: cooldown index points at unknown event %s", id)
		}
		latest[key] = ev
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.events, st.latestBySymDir = events, latest
	return nil
}

// Clone returns an isolated deep copy (events, cooldown index and params).
func (st *V7T2EventStore) Clone() *V7T2EventStore {
	out := NewV7T2EventStore()
	if st == nil {
		return out
	}
	st.mu.Lock()
	b, err := st.marshalLocked()
	params := st.params
	st.mu.Unlock()
	if err != nil {
		panic(fmt.Sprintf("t2 store clone: %v", err)) // only plain data types: cannot fail
	}
	if err := out.LoadState(b); err != nil {
		panic(fmt.Sprintf("t2 store clone: %v", err))
	}
	out.params = params
	return out
}

// ReplaceFrom commits an isolated copy (see Clone) into this store.
func (st *V7T2EventStore) ReplaceFrom(next *V7T2EventStore) {
	if st == nil || next == nil {
		return
	}
	c := next.Clone()
	st.mu.Lock()
	defer st.mu.Unlock()
	st.events, st.latestBySymDir, st.params = c.events, c.latestBySymDir, c.params
}

type v7ModuleBreakerState struct {
	DryRounds int `json:"dry_rounds"`
	Skips     int `json:"skips"`
}

type v7SignalStateSnapshot struct {
	Version        int                                  `json:"version"`
	Entries        map[string]*v7WatchEntry             `json:"entries"`
	TriggerEntries map[string]*v7TriggerMemoryEntry     `json:"trigger_entries"`
	LastCycle      int                                  `json:"last_cycle"`
	ExpireGap      int                                  `json:"expire_gap"`
	ModuleBreaker  map[V7SetupType]v7ModuleBreakerState `json:"module_breaker"`
	BreakerRegime  V7MarketRegime                       `json:"breaker_regime"`
}

// ExportState serialises the native watch state, trigger memory and module
// breaker.
func (m *V7SignalStateManager) ExportState() ([]byte, error) {
	if m == nil {
		m = NewV7SignalStateManager()
	}
	s := v7SignalStateSnapshot{
		Version:        v7StateSchemaVersion,
		Entries:        m.entries,
		TriggerEntries: m.triggerEntries,
		LastCycle:      m.lastCycle,
		ExpireGap:      m.expireGap,
		ModuleBreaker:  make(map[V7SetupType]v7ModuleBreakerState, len(m.moduleBreaker)),
		BreakerRegime:  m.breakerRegime,
	}
	for setup, e := range m.moduleBreaker {
		if e != nil {
			s.ModuleBreaker[setup] = v7ModuleBreakerState{DryRounds: e.dryRounds, Skips: e.skips}
		}
	}
	return json.Marshal(s)
}

// ImportState replaces the manager's state with one produced by ExportState.
func (m *V7SignalStateManager) ImportState(b []byte) error {
	var s v7SignalStateSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("watch state: %w", err)
	}
	if s.Version != v7StateSchemaVersion {
		return fmt.Errorf("watch state: schema version %d, want %d", s.Version, v7StateSchemaVersion)
	}
	next := NewV7SignalStateManager()
	if s.Entries != nil {
		next.entries = s.Entries
	}
	if s.TriggerEntries != nil {
		next.triggerEntries = s.TriggerEntries
	}
	next.lastCycle = s.LastCycle
	if s.ExpireGap > 0 {
		next.expireGap = s.ExpireGap
	}
	if len(s.ModuleBreaker) > 0 {
		next.moduleBreaker = make(map[V7SetupType]*v7ModuleBreakerEntry, len(s.ModuleBreaker))
		for setup, e := range s.ModuleBreaker {
			next.moduleBreaker[setup] = &v7ModuleBreakerEntry{dryRounds: e.DryRounds, skips: e.Skips}
		}
	}
	next.breakerRegime = s.BreakerRegime
	*m = *next
	return nil
}
