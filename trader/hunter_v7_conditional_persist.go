package trader

// #92 / AUD-C6: persistence of pending conditional entries in the position
// lifecycle record, the fire-time candidate, and the restart path.

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// persistedPendingEntry is the stored form of a pending entry. gob keeps the
// decision's hydrated (json:"-") contract fields, which JSON would drop.
type persistedPendingEntry struct {
	Decision          kernel.Decision
	Candidate         kernel.CandidateCoin
	Side              string
	Trigger           float64
	Invalidation      float64
	CreatedAt         time.Time
	ExpiresAt         time.Time
	ReroutedFromGuard bool
	ReroutedBlockCode string
}

func encodePendingEntry(p *hunterV7PendingEntry) (string, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(persistedPendingEntry{
		Decision: p.Decision, Candidate: p.Candidate, Side: p.Side, Trigger: p.Trigger,
		Invalidation: p.Invalidation, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt,
		ReroutedFromGuard: p.ReroutedFromGuard, ReroutedBlockCode: p.ReroutedBlockCode,
	})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func decodePendingEntry(payload string) (*hunterV7PendingEntry, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}
	var pe persistedPendingEntry
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&pe); err != nil {
		return nil, err
	}
	return &hunterV7PendingEntry{
		Decision: pe.Decision, Candidate: pe.Candidate, Side: pe.Side, Trigger: pe.Trigger,
		Invalidation: pe.Invalidation, CreatedAt: pe.CreatedAt, ExpiresAt: pe.ExpiresAt,
		ReroutedFromGuard: pe.ReroutedFromGuard, ReroutedBlockCode: pe.ReroutedBlockCode,
	}, nil
}

// persistPendingEntry writes the pending_entry row. A store failure is
// logged; the entry stays armed in memory (previous behaviour).
func (at *AutoTrader) persistPendingEntry(p *hunterV7PendingEntry) {
	ds := at.lifecycleStore()
	if ds == nil || p == nil {
		return
	}
	payload, err := encodePendingEntry(p)
	if err != nil {
		logger.Errorf("  ⚠ [HUNTER V7 CONDITIONAL] %s %s pending entry not persisted (encode): %v", p.Decision.Symbol, p.Side, err)
		return
	}
	rec := &store.PositionLifecycle{
		TraderID: at.id, Symbol: p.Decision.Symbol, Side: p.Side, Status: store.LifecycleStatusPendingEntry,
		SignalID: p.Decision.SelectedHunterV7SignalID, IntentKey: p.Decision.HunterV7IntentKey,
		PlanEntryPrice: p.Trigger, PlanStopLoss: p.Decision.StopLoss, PlanTakeProfit: p.Decision.TakeProfit,
		PlanSizeUSD: p.Decision.PositionSizeUSD, Leverage: p.Decision.Leverage,
		PendingPayload: payload, ExpiresAt: p.ExpiresAt.UnixMilli(), CreatedAt: p.CreatedAt.UnixMilli(),
	}
	if p.ReroutedFromGuard {
		rec.EntrySource = store.LifecycleEntrySourceGuardConfirmReroute
		rec.Note = fmt.Sprintf("%s: %s", store.LifecycleEntrySourceGuardConfirmReroute, p.ReroutedBlockCode)
	}
	if err := ds.CreateLifecycle(rec); err != nil {
		logger.Errorf("  ⚠ [HUNTER V7 CONDITIONAL] %s %s pending entry not persisted: %v", p.Decision.Symbol, p.Side, err)
		return
	}
	p.LifecycleID = rec.ID
}

// closePendingEntryRecord moves the pending_entry row to fired or cancelled.
func (at *AutoTrader) closePendingEntryRecord(p *hunterV7PendingEntry, status, note string) {
	ds := at.lifecycleStore()
	if ds == nil || p == nil || p.LifecycleID == 0 {
		return
	}
	if err := ds.UpdateLifecycle(p.LifecycleID, map[string]interface{}{"status": status, "note": note}); err != nil {
		logger.Errorf("  ⚠ [HUNTER V7 CONDITIONAL] %s %s pending row %d not updated: %v", p.Decision.Symbol, p.Side, p.LifecycleID, err)
	}
}

// fireTimeCandidate is the stored candidate refreshed from the latest
// snapshot: t0 refresh stamps are stripped so fire-time guards re-verify live
// freshness/confirmations instead of trusting registration-time evidence, and
// last price plus price/snapshot age are updated from the latest snapshot.
func (at *AutoTrader) fireTimeCandidate(p *hunterV7PendingEntry, now time.Time) kernel.CandidateCoin {
	c := p.Candidate
	if c.V7PriceContext != nil {
		pc := *c.V7PriceContext
		c.V7PriceContext = &pc
	}
	// U9b Finding 1: strip t0 refresh-confirmation stamps so the fire-time
	// execution guard re-runs collectHunterV7RefreshEvidence when required and
	// enforces signal freshness (>45s) / stale_data_risk at fire time.
	if len(c.V7ReasonCodes) > 0 {
		filtered := make([]string, 0, len(c.V7ReasonCodes))
		for _, code := range c.V7ReasonCodes {
			if code == "fresh_micro_confirmed" || code == "fresh_rest_confirmed" {
				continue
			}
			filtered = append(filtered, code)
		}
		c.V7ReasonCodes = filtered
	}
	// U9 P0-2: for a guard-veto rerouted pending entry, the firing closed 1m
	// bar through p.Trigger satisfies ONLY the single closed-bar trigger
	// confirmation that caused the reroute. Every other guard check
	// (freshness, snapshot budget, risk tags, HTF, breaker, drift, RR) still
	// runs in full at fire time.
	if p.ReroutedFromGuard && p.ReroutedBlockCode != "" && c.V7ConfirmSummary != nil {
		s := c.V7ConfirmSummary.Clone()
		barClose := p.FiredBarClose
		if barClose <= 0 {
			barClose = p.Trigger
		}
		var closeUnix int64
		if p.FiredBarCloseTime > 0 {
			closeUnix = p.FiredBarCloseTime / 1000
		}
		ev := local.V7ConfirmBarEvidence{
			Code:             p.ReroutedBlockCode,
			Passed:           true,
			Timeframe:        "1m",
			DecidingTF:       "1m",
			BarCloseTimeUnix: closeUnix,
			BarClose:         barClose,
			ReferenceEdge:    p.Trigger,
			ClosedOnly:       true,
		}
		remaining := make([]local.V7ConfirmationCheck, 0, len(s.MissingReview))
		for _, chk := range s.MissingReview {
			if chk.Code != p.ReroutedBlockCode {
				remaining = append(remaining, chk)
			}
		}
		s.MissingReview = remaining
		for i := range s.ContextChecks {
			if s.ContextChecks[i].Code == p.ReroutedBlockCode {
				evCopy := ev
				s.ContextChecks[i].Passed = true
				s.ContextChecks[i].Actual = barClose
				s.ContextChecks[i].Threshold = p.Trigger
				s.ContextChecks[i].Evidence = &evCopy
			}
		}
		s.PassedReview = s.PassedHard && len(s.MissingHard) == 0 && len(s.MissingReview) == 0
		s.UpsertBarEvidence(ev)
		c.V7ConfirmSummary = s
	}
	sym := at.latestSymbolSnapshot(p.Decision.Symbol)
	if sym == nil {
		at.applyMissingFireSnapshotAge(&c, p, now)
		return c
	}
	if sym.Price > 0 && c.V7PriceContext != nil {
		c.V7PriceContext.Last = sym.Price
	}
	if !sym.Timestamp.IsZero() {
		age := now.Sub(sym.Timestamp).Milliseconds()
		if age < 0 {
			age = 0
		}
		c.V7DataFreshness.PriceAgeMs = age
		c.V7DataFreshness.SnapshotAgeMs = age
	} else {
		at.applyMissingFireSnapshotAge(&c, p, now)
	}
	return c
}

// applyMissingFireSnapshotAge ensures that when no fire-time snapshot
// timestamp is available, a t0 PriceAgeMs == 0 (e.g. zeroed by registration-time
// refresh evidence) is never treated as evidence of fire-time freshness. It
// computes t0's stored age plus elapsed time since registration and floors it
// above the guard's freshness limits so high-velocity / budgeted guards fail
// closed unless live fire-time evidence is obtained.
func (at *AutoTrader) applyMissingFireSnapshotAge(c *kernel.CandidateCoin, p *hunterV7PendingEntry, now time.Time) {
	baseAge := c.V7DataFreshness.PriceAgeMs
	if c.V7DataFreshness.SnapshotAgeMs > baseAge {
		baseAge = c.V7DataFreshness.SnapshotAgeMs
	}
	var elapsed int64
	if !p.CreatedAt.IsZero() && now.After(p.CreatedAt) {
		elapsed = now.Sub(p.CreatedAt).Milliseconds()
	}
	age := baseAge + elapsed
	const signalFreshnessLimitMs int64 = 45_000
	minStale := signalFreshnessLimitMs + 1
	if c.V7DataFreshness.StaleThresholdMs >= minStale {
		minStale = c.V7DataFreshness.StaleThresholdMs + 1
	}
	if budget := at.dataTimingConfig().FreshnessBudgetSeconds[c.V7SignalClass]; budget > 0 {
		if budgetMs := int64(budget*1000) + 1; budgetMs > minStale {
			minStale = budgetMs
		}
	}
	if age < minStale {
		age = minStale
	}
	c.V7DataFreshness.PriceAgeMs = age
	c.V7DataFreshness.SnapshotAgeMs = age
}

func (at *AutoTrader) latestSymbolSnapshot(symbol string) *datafetch.SymbolSnapshot {
	if at.conditional.latest != nil {
		return at.conditional.latest(symbol)
	}
	se := at.snapshotEngine
	if se == nil && at.strategyEngine != nil {
		se = at.strategyEngine.GetSnapshotEngine()
	}
	if se == nil {
		return nil
	}
	snap := se.GetSnapshot()
	if snap == nil || snap.Symbols == nil {
		return nil
	}
	return snap.Symbols[symbol]
}

// restorePendingConditionalEntries re-arms the persisted pending entries
// after a restart, or cancels them visibly (log, row status and a decision
// record): expired, conditional entry disabled, unreadable payload, or a
// position already open on the side.
func (at *AutoTrader) restorePendingConditionalEntries(now time.Time) {
	ds := at.lifecycleStore()
	if ds == nil {
		return
	}
	rows, err := ds.ListLifecycles(at.id, store.LifecycleStatusPendingEntry)
	if err != nil {
		logger.Errorf("  ⚠ [HUNTER V7 CONDITIONAL] restore: cannot list pending entries: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	var positions []map[string]interface{}
	if at.trader != nil {
		positions, _ = at.freshPositions()
	}
	for i := range rows {
		row := rows[i]
		cancel := func(reason string) {
			line := fmt.Sprintf("✖ %s %s pending conditional entry cancelled at restart: %s", row.Symbol, row.Side, reason)
			logger.Warnf("  [HUNTER V7 CONDITIONAL] %s", line)
			_ = ds.UpdateLifecycle(row.ID, map[string]interface{}{"status": store.LifecycleStatusEntryCancelled, "note": "restart: " + reason})
			record := &store.DecisionRecord{ExecutionLog: []string{line}, Success: false}
			if err := at.saveDecision(record); err != nil {
				logger.Warnf("⚠ Failed to save conditional cancel record: %v", err)
			}
		}
		p, err := decodePendingEntry(row.PendingPayload)
		if err != nil {
			cancel("stored entry unreadable: " + err.Error())
			continue
		}
		p.LifecycleID = row.ID
		switch {
		case !now.Before(p.ExpiresAt):
			cancel("expired while the trader was stopped")
			continue
		case !at.conditionalEntryEnabled():
			cancel("conditional entry is disabled")
			continue
		}
		if _, open := findPosition(positions, p.Decision.Symbol, p.Side); open {
			cancel("a position is already open on this side")
			continue
		}
		key := pendingEntryKey(p.Decision.Symbol, p.Side)
		at.conditional.mu.Lock()
		if at.conditional.pending == nil {
			at.conditional.pending = map[string]*hunterV7PendingEntry{}
		}
		if _, dup := at.conditional.pending[key]; dup {
			at.conditional.mu.Unlock()
			cancel("a newer pending entry exists for this side")
			continue
		}
		p.ArmedAt = now
		at.conditional.pending[key] = p
		at.conditional.mu.Unlock()
		logger.Infof("  ⏳ [HUNTER V7 CONDITIONAL] %s %s pending entry re-armed after restart (trigger %.8f, invalidation %.8f, expires %s)",
			p.Decision.Symbol, p.Side, p.Trigger, p.Invalidation, p.ExpiresAt.Format(time.RFC3339))
	}
}
