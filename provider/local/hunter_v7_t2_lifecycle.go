package local

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Hunter v7 — Task-2 event lifecycle (M3, spec section 5)
// ============================================================================
// Ports the scanner's event state machine (event_id zoning, upsert_event,
// cooldown, expiry) into an in-memory store. The store's namespace is
// deliberately ISOLATED from the native v7 watch-state manager
// (V7SignalStateManager): Task-2 trigger memory neither reads nor writes
// native v7 state, per the M3 milestone spec.
//
// Scope note: outcome tracking (spec section 5.3: TRIGGERED transitions,
// MFE/MAE, realized R) is the monitor's tracker, not a selection rule, and
// is out of scope for this transplant. Events here terminate only via
// expiry or activity invalidation.

// V7T2Event is one tracked Task-2 event (whale_events row equivalent).
type V7T2Event struct {
	EventID   string
	Symbol    string
	Direction string
	SetupType string
	Status    string // WATCH / ARMED / EXPIRED / INVALIDATED
	Score     float64
	WatchHits int
	CreatedAt time.Time
	ArmedAt   *time.Time
	ExpiresAt time.Time
	Signal    *V7T2Signal // latest payload
}

// V7T2UpsertResult mirrors upsert_event's return contract (spec 5.2).
type V7T2UpsertResult struct {
	Event      *V7T2Event
	Created    bool
	Notify     bool   // upstream would send a Telegram notification
	Dropped    bool   // signal discarded (terminal event / cooldown)
	DropReason string // "terminal_event" | "cooldown"
}

// V7T2EventID ports event_id() (spec section 5.1):
// zone = round(trigger / (price * zone_pct)) with zone_pct 0.02 for
// momentum_continuation* and 0.005 otherwise. Python's round() is
// round-half-even, hence math.RoundToEven.
func V7T2EventID(sig *V7T2Signal, price float64) string {
	zonePct := 0.005
	if strings.HasPrefix(sig.SetupType, "momentum_continuation") {
		zonePct = 0.02
	}
	var zone int64
	if step := price * zonePct; step > 0 {
		zone = int64(math.RoundToEven(sig.Trigger / step))
	}
	return fmt.Sprintf("%s:%s:%s:%d", sig.Symbol, sig.SetupType, sig.Direction, zone)
}

// V7T2EventStore is the isolated Task-2 trigger memory.
type V7T2EventStore struct {
	mu     sync.Mutex
	events map[string]*V7T2Event
	// latestBySymDir tracks the most recently created event per
	// symbol+direction (any setup, any status) for the cooldown rule.
	latestBySymDir map[string]*V7T2Event
}

// NewV7T2EventStore creates an empty store.
func NewV7T2EventStore() *V7T2EventStore {
	return &V7T2EventStore{
		events:         make(map[string]*V7T2Event),
		latestBySymDir: make(map[string]*V7T2Event),
	}
}

func v7t2SymDirKey(symbol, direction string) string { return symbol + "|" + direction }

// effectiveStatus resolves lazy expiry: an active event past its
// expires_at reads as EXPIRED (upstream finalizes this in
// update_outcomes; the distinction matters for the no-revival rule).
func (e *V7T2Event) effectiveStatus(now time.Time) string {
	if (e.Status == "WATCH" || e.Status == "ARMED") && !now.Before(e.ExpiresAt) {
		return "EXPIRED"
	}
	return e.Status
}

// Upsert ports upsert_event (spec section 5.2). price is the snapshot
// price used for event-id zoning; now is the scan time.
func (st *V7T2EventStore) Upsert(sig *V7T2Signal, price float64, now time.Time) V7T2UpsertResult {
	st.mu.Lock()
	defer st.mu.Unlock()
	id := V7T2EventID(sig, price)

	if existing, ok := st.events[id]; ok {
		if st := existing.effectiveStatus(now); st == "INVALIDATED" || st == "EXPIRED" {
			// Terminal events are never revived.
			return V7T2UpsertResult{Dropped: true, DropReason: "terminal_event", Event: existing}
		}
		existing.WatchHits++
		if sig.Score > existing.Score+10 {
			// Score upgrade: payload, score AND status are replaced
			// (event-level status = effective status, section 6.2).
			existing.Signal = sig
			existing.Score = sig.Score
			existing.Status = sig.EffectiveStatus()
			if sig.EffectiveStatus() == "ARMED" && existing.ArmedAt == nil {
				t := now
				existing.ArmedAt = &t
			}
			return V7T2UpsertResult{Event: existing, Notify: true}
		}
		// Plain re-hit: payload updated (with the new watch_hits), status
		// unchanged, expiry NOT extended.
		existing.Signal = sig
		watchPromoted := sig.Score >= v7T2MinWatchScore && sig.Score < v7T2MinAlertScore && existing.WatchHits == 2
		return V7T2UpsertResult{Event: existing, Notify: watchPromoted}
	}

	// New event_id: 60-minute same-symbol same-direction cooldown,
	// judged against the most recent event of ANY status and ANY setup
	// (spec sections 5.2 and 8 item 9).
	key := v7t2SymDirKey(sig.Symbol, sig.Direction)
	if latest, ok := st.latestBySymDir[key]; ok {
		if now.Sub(latest.CreatedAt) < time.Duration(v7T2CooldownMinutes*float64(time.Minute)) {
			return V7T2UpsertResult{Dropped: true, DropReason: "cooldown"}
		}
	}

	ev := &V7T2Event{
		EventID:   id,
		Symbol:    sig.Symbol,
		Direction: sig.Direction,
		SetupType: sig.SetupType,
		Status:    sig.EffectiveStatus(),
		Score:     sig.Score,
		WatchHits: 1,
		CreatedAt: now,
		Signal:    sig,
	}
	if sig.EffectiveStatus() == "ARMED" {
		t := now
		ev.ArmedAt = &t
		ev.ExpiresAt = now.Add(time.Duration(v7T2EventExpiryHours * float64(time.Hour)))
	} else {
		ev.ExpiresAt = now.Add(2 * time.Hour) // WATCH expiry is 2h (spec 5.2)
	}
	st.events[id] = ev
	st.latestBySymDir[key] = ev
	return V7T2UpsertResult{Event: ev, Created: true, Notify: sig.EffectiveStatus() == "ARMED"}
}

// InvalidateActivity ports invalidate_activity_events (spec section 1.5):
// live events whose symbol entered the activity list are forced to
// INVALIDATED and stamped HARD_EXCLUDE. Returns how many were hit.
func (st *V7T2EventStore) InvalidateActivity(symbol string, now time.Time) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	n := 0
	for _, ev := range st.events {
		if ev.Symbol != symbol {
			continue
		}
		if s := ev.effectiveStatus(now); s == "WATCH" || s == "ARMED" {
			ev.Status = "INVALIDATED"
			if ev.Signal != nil {
				ev.Signal.ActivityStatus = "HARD_EXCLUDE"
			}
			n++
		}
	}
	return n
}

// Get returns the event with the given id (test/debug helper).
func (st *V7T2EventStore) Get(eventID string) *V7T2Event {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.events[eventID]
}

// Len returns the number of tracked events.
func (st *V7T2EventStore) Len() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.events)
}
