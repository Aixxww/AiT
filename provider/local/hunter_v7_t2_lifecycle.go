package local

import (
	"fmt"
	"math"
	"sort"
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
// U1/P1 adds spec section 5.3 (update_outcomes): at the start of each scan
// every active event (WATCH, ARMED, TRIGGERED) is checked against the latest
// price. Price reaching the signal ENTRY (not the trigger) makes the event
// TRIGGERED; a pre-entry stop touch invalidates it (NO_ENTRY_INVALIDATED);
// after entry a stop touch (STOP or TP1_THEN_STOP), a target2 touch (TP2) or
// expiry ends it, with the scanner's planned realized R. Upstream this is
// outcome tracking; in AiT a TRIGGERED event is the reachable path from
// detection to a candidate for every family, including the families whose
// score can never reach the ARMED threshold (redesign plan OD-4).

// V7T2Event is one tracked Task-2 event (whale_events row equivalent).
type V7T2Event struct {
	EventID   string
	Symbol    string
	Direction string
	SetupType string
	Status    string // WATCH / ARMED / TRIGGERED / EXPIRED / INVALIDATED
	Score     float64
	WatchHits int
	CreatedAt time.Time
	ArmedAt   *time.Time
	ExpiresAt time.Time
	Signal    *V7T2Signal // latest payload

	// Section 5.3 outcome tracking (U1).
	TriggeredAt *time.Time // first time the price reached the entry
	MFER        float64    // max favourable excursion after entry, in R, at scan prices
	Outcome     string     // NO_ENTRY_INVALIDATED / STOP / TP1_THEN_STOP / TP2 / EXPIRED
	RealizedR   *float64   // planned realized R (nil for NO_ENTRY_INVALIDATED)
	ClosedAt    *time.Time
}

// V7T2Transition is one section 5.3 state change produced by UpdateOutcomes.
type V7T2Transition struct {
	Event   *V7T2Event
	From    string
	To      string
	Outcome string // set when the event ended
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
	params         V7T2LifecycleParams
}

// Configure sets the lifecycle parameters (U1). Zero fields keep the
// scanner constants.
func (st *V7T2EventStore) Configure(p V7T2LifecycleParams) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.params = p
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
	if v7t2Active(e.Status) && !now.Before(e.ExpiresAt) {
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
			// A TRIGGERED event stays TRIGGERED: triggered_at is the
			// record that the entry was reached, and the upgrade does not
			// undo it (interpretation; the spec text overwrites status).
			existing.Signal = sig
			existing.Score = sig.Score
			if existing.TriggeredAt == nil {
				existing.Status = sig.EffectiveStatus()
			}
			if sig.EffectiveStatus() == "ARMED" && existing.ArmedAt == nil {
				t := now
				existing.ArmedAt = &t
			}
			return V7T2UpsertResult{Event: existing, Notify: true}
		}
		// Plain re-hit: payload updated (with the new watch_hits), status
		// unchanged, expiry NOT extended.
		existing.Signal = sig
		watchPromoted := sig.Score >= st.params.watchScore() && sig.Score < st.params.alertScore() && existing.WatchHits == 2
		return V7T2UpsertResult{Event: existing, Notify: watchPromoted}
	}

	// New event_id: 60-minute same-symbol same-direction cooldown,
	// judged against the most recent event of ANY status and ANY setup
	// (spec sections 5.2 and 8 item 9).
	key := v7t2SymDirKey(sig.Symbol, sig.Direction)
	if latest, ok := st.latestBySymDir[key]; ok {
		if now.Sub(latest.CreatedAt) < st.params.cooldown() {
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
		ev.ExpiresAt = now.Add(st.params.armedExpiry())
	} else {
		ev.ExpiresAt = now.Add(st.params.watchExpiry()) // WATCH expiry, 2h by default (spec 5.2)
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
		if s := ev.effectiveStatus(now); v7t2Active(s) {
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

func v7t2Active(status string) bool {
	return status == "WATCH" || status == "ARMED" || status == "TRIGGERED"
}

// UpdateOutcomes ports update_outcomes (spec section 5.3), run at the start
// of each scan with the latest price per symbol. Events whose symbol has no
// price are only checked for expiry. Returns the transitions of this call.
func (st *V7T2EventStore) UpdateOutcomes(prices map[string]float64, now time.Time) []V7T2Transition {
	st.mu.Lock()
	defer st.mu.Unlock()
	var out []V7T2Transition
	for _, ev := range st.events {
		if !v7t2Active(ev.Status) || ev.Signal == nil {
			continue
		}
		from := ev.Status
		sig := ev.Signal
		long := sig.Direction == "LONG"
		risk := math.Abs(sig.Entry - sig.Stop)
		price := prices[ev.Symbol]
		dirR := func(px float64) float64 {
			if risk <= 0 {
				return 0
			}
			if long {
				return (px - sig.Entry) / risk
			}
			return (sig.Entry - px) / risk
		}
		end := func(outcome string, realized *float64) {
			ev.Status = "INVALIDATED"
			if outcome == "EXPIRED" {
				ev.Status = "EXPIRED"
			}
			ev.Outcome, ev.RealizedR = outcome, realized
			t := now
			ev.ClosedAt = &t
			out = append(out, V7T2Transition{Event: ev, From: from, To: ev.Status, Outcome: outcome})
		}
		if !now.Before(ev.ExpiresAt) {
			r := 0.0
			if ev.TriggeredAt != nil && price > 0 {
				r = math.Round(dirR(price)*10000) / 10000
			}
			end("EXPIRED", &r)
			continue
		}
		if price <= 0 || risk <= 0 {
			continue
		}
		if ev.TriggeredAt == nil {
			reached := (long && price >= sig.Entry) || (!long && price <= sig.Entry)
			if !reached {
				if (long && price <= sig.Stop) || (!long && price >= sig.Stop) {
					end("NO_ENTRY_INVALIDATED", nil)
				}
				continue
			}
			if st.params.DisableTrigger {
				continue
			}
			t := now
			ev.TriggeredAt = &t
			ev.Status = "TRIGGERED"
			out = append(out, V7T2Transition{Event: ev, From: from, To: "TRIGGERED"})
			from = "TRIGGERED"
		}
		if r := dirR(price); r > ev.MFER {
			ev.MFER = r
		}
		switch {
		case (long && price <= sig.Stop) || (!long && price >= sig.Stop):
			r := -1.0
			outcome := "STOP"
			if ev.MFER >= v7T2MinRR {
				r = 0.5*v7T2MinRR - 0.5
				outcome = "TP1_THEN_STOP"
			}
			end(outcome, &r)
		case (long && price >= sig.Target2) || (!long && price <= sig.Target2):
			r := 0.5*v7T2MinRR + 0.5*v7T2TP2Multiple
			end("TP2", &r)
		}
	}
	return out
}

// FreshlyTriggered returns the TRIGGERED events whose trigger is within the
// configured surface window (U1). They are the events re-emitted to the
// pipeline even when the detector does not fire on this scan.
func (st *V7T2EventStore) FreshlyTriggered(now time.Time) []*V7T2Event {
	st.mu.Lock()
	defer st.mu.Unlock()
	win := st.params.triggeredSurface()
	var out []*V7T2Event
	for _, ev := range st.events {
		if ev.Status == "TRIGGERED" && ev.TriggeredAt != nil && now.Sub(*ev.TriggeredAt) <= win && now.Before(ev.ExpiresAt) {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EventID < out[j].EventID })
	return out
}
