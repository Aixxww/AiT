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
	"github.com/Aixxww/AiT/store"
)

// persistedPendingEntry is the stored form of a pending entry. gob keeps the
// decision's hydrated (json:"-") contract fields, which JSON would drop.
type persistedPendingEntry struct {
	Decision     kernel.Decision
	Candidate    kernel.CandidateCoin
	Side         string
	Trigger      float64
	Invalidation float64
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

func encodePendingEntry(p *hunterV7PendingEntry) (string, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(persistedPendingEntry{
		Decision: p.Decision, Candidate: p.Candidate, Side: p.Side, Trigger: p.Trigger,
		Invalidation: p.Invalidation, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt,
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
	return &hunterV7PendingEntry{Decision: pe.Decision, Candidate: pe.Candidate, Side: pe.Side, Trigger: pe.Trigger,
		Invalidation: pe.Invalidation, CreatedAt: pe.CreatedAt, ExpiresAt: pe.ExpiresAt}, nil
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
// snapshot: last price and the price/snapshot age at fire time, so the
// guard's freshness checks see the real age.
func (at *AutoTrader) fireTimeCandidate(p *hunterV7PendingEntry, now time.Time) kernel.CandidateCoin {
	c := p.Candidate
	if c.V7PriceContext != nil {
		pc := *c.V7PriceContext
		c.V7PriceContext = &pc
	}
	sym := at.latestSymbolSnapshot(p.Decision.Symbol)
	if sym == nil {
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
	}
	return c
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
