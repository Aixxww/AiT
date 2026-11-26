package trader

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/store"
)

// U9: TRIGGER-class conditional entry, option A (owner decision): a backend
// check on every completed 1-minute candle. See store.V7ConditionalEntryConfig.
//
// Lifecycle of a pending entry:
//   - registered when an approved open targets a conditional candidate
//     (kernel V7ConditionalEntry, hydrated as Decision.HunterV7ConditionalEntry).
//     It is validated against the trigger price as the entry (price drift,
//     stop/take-profit sides, stop distance, RR) and never sent to the
//     exchange at that point;
//   - on each completed 1m candle that closed after registration, a touch of
//     the invalidation price cancels it (checked first, stop-first
//     convention), else a close through the trigger fires it;
//   - it expires after MaxPendingMinutes.
//
// Firing removes the entry before anything is sent, so it executes at most
// once. The open goes through the normal open path (executeOpen*WithRecord:
// all risk checks, the live preflight, F04, the mandatory stop of F03) with
// the intent key of the approving cycle, so the F04 client order id is the
// same for any retry of that intent. #92 / AUD-C6: pending entries are
// persisted in the position lifecycle record; a restart re-arms them or
// cancels them visibly, and the execution guard is re-run at fire time on
// the latest snapshot.

type hunterV7PendingEntry struct {
	Decision     kernel.Decision
	Candidate    kernel.CandidateCoin
	Side         string
	Trigger      float64
	Invalidation float64
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastBarClose int64 // close time (ms) of the last candle processed
	// #92 / AUD-C6
	LifecycleID uint      // persisted pending_entry row (0 = not persisted)
	ArmedAt     time.Time // re-arm time after a restart: earlier candles can cancel, never fire
}

func pendingEntryKey(symbol, side string) string { return strings.ToUpper(symbol) + "|" + side }

type hunterV7ConditionalState struct {
	mu      sync.Mutex
	pending map[string]*hunterV7PendingEntry
	// entryMu serializes conditional executions with the main loop's
	// execution phase (opens, the decision record and its cycle number).
	entryMu sync.Mutex
	// test hooks
	bars    func(symbol string) []datafetch.Kline
	latest  func(symbol string) *datafetch.SymbolSnapshot // fire-time snapshot (AUD-C6)
	execute func(ctx *kernel.Context, decision *kernel.Decision, side string, record *store.DecisionAction) error
}

func (at *AutoTrader) conditionalEntryConfig() store.V7ConditionalEntryConfig {
	if at == nil || at.config.StrategyConfig == nil || at.config.StrategyConfig.CoinSource.Hunter == nil {
		return store.EffectiveV7ConditionalEntry(nil)
	}
	return store.EffectiveV7ConditionalEntry(at.config.StrategyConfig.CoinSource.Hunter.V7ConditionalEntry)
}

func (at *AutoTrader) conditionalEntryEnabled() bool {
	return at.isHunterV7Strategy() && at.conditionalEntryConfig().Enabled
}

// pendingConditionalEntries returns a snapshot of the pending entries.
func (at *AutoTrader) pendingConditionalEntries() []hunterV7PendingEntry {
	at.conditional.mu.Lock()
	defer at.conditional.mu.Unlock()
	out := make([]hunterV7PendingEntry, 0, len(at.conditional.pending))
	for _, p := range at.conditional.pending {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// registerHunterV7ConditionalEntry keeps an approved open as a pending
// entry instead of sending it (see the file comment).
func (at *AutoTrader) registerHunterV7ConditionalEntry(ctx *kernel.Context, decision *kernel.Decision, side string, record *store.DecisionAction, now time.Time) error {
	if !at.conditionalEntryEnabled() {
		// Fail closed: a conditional candidate must never be entered at market.
		return fmt.Errorf("❌ [HUNTER V7 CONDITIONAL] %s %s blocked: candidate is conditional on its trigger but conditional entry is disabled (conditional_entry_disabled)",
			decision.Symbol, decision.Action)
	}
	candidate := hunterV7CandidateForDecision(ctx, decision)
	if candidate == nil || !candidate.V7ConditionalEntry || decision.HunterV7TriggerPrice <= 0 {
		return fmt.Errorf("❌ [HUNTER V7 CONDITIONAL] %s %s blocked: no conditional candidate with a trigger price (conditional_contract_mismatch)",
			decision.Symbol, decision.Action)
	}
	trigger := decision.HunterV7TriggerPrice
	if err := at.validateOpenDecision(decision, trigger, side); err != nil {
		return fmt.Errorf("❌ [HUNTER V7 CONDITIONAL] %s %s rejected at the trigger %.8f: %w", decision.Symbol, decision.Action, trigger, err)
	}
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("failed to get positions: %w", err)
	}
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == side {
			return fmt.Errorf("❌ %s already has %s position, close it first", decision.Symbol, side)
		}
	}
	key := pendingEntryKey(decision.Symbol, side)
	at.conditional.mu.Lock()
	defer at.conditional.mu.Unlock()
	if at.conditional.pending == nil {
		at.conditional.pending = map[string]*hunterV7PendingEntry{}
	}
	if _, ok := at.conditional.pending[key]; ok {
		return fmt.Errorf("❌ [HUNTER V7 CONDITIONAL] %s %s blocked: a pending entry already exists (conditional_pending_exists)", decision.Symbol, side)
	}
	if err := at.enforceMaxPositions(at.effectivePositionCount(len(positions)) + len(at.conditional.pending)); err != nil {
		return err
	}
	inval := decision.HunterV7InvalidationPrice
	if inval <= 0 {
		inval = decision.StopLoss
	}
	p := &hunterV7PendingEntry{Decision: *decision, Candidate: *candidate, Side: side, Trigger: trigger, Invalidation: inval,
		CreatedAt: now, ExpiresAt: now.Add(time.Duration(at.conditionalEntryConfig().MaxPendingMinutes * float64(time.Minute)))}
	p.Decision.HunterV7IntentKey = at.openIntentKey(decision, side) + "|conditional"
	at.persistPendingEntry(p) // AUD-C6
	at.conditional.pending[key] = p
	if record != nil {
		record.ConditionalPending = true
		record.Price = trigger
	}
	logger.Infof("  ⏳ [HUNTER V7 CONDITIONAL] %s %s pending: enters on a 1m close through %.8f, cancelled at %.8f, expires %s",
		decision.Symbol, side, trigger, inval, p.ExpiresAt.Format(time.RFC3339))
	return nil
}

func (at *AutoTrader) conditionalBars(symbol string) []datafetch.Kline {
	if at.conditional.bars != nil {
		return at.conditional.bars(symbol)
	}
	se := at.snapshotEngine
	if se == nil && at.strategyEngine != nil {
		se = at.strategyEngine.GetSnapshotEngine()
	}
	if se == nil {
		return nil
	}
	snap := se.GetSnapshot()
	if snap == nil || snap.Symbols == nil || snap.Symbols[symbol] == nil {
		return nil
	}
	return snap.Symbols[symbol].Klines["1m"]
}

// checkHunterV7ConditionalEntries processes the completed 1m candles of
// every pending entry and fires, cancels or expires it.
func (at *AutoTrader) checkHunterV7ConditionalEntries(now time.Time) {
	nowMs := now.UnixMilli()
	var fire []hunterV7PendingEntry
	at.conditional.mu.Lock()
	keys := make([]string, 0, len(at.conditional.pending))
	for k := range at.conditional.pending {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := at.conditional.pending[k]
		if !now.Before(p.ExpiresAt) {
			delete(at.conditional.pending, k)
			logger.Infof("  ⌛ [HUNTER V7 CONDITIONAL] %s %s expired without a trigger close", p.Decision.Symbol, p.Side)
			at.closePendingEntryRecord(p, store.LifecycleStatusEntryCancelled, "expired without a trigger close")
			continue
		}
		bars := append([]datafetch.Kline(nil), at.conditionalBars(p.Decision.Symbol)...)
		sort.Slice(bars, func(i, j int) bool { return bars[i].OpenTime < bars[j].OpenTime })
		long := p.Side == "long"
		for _, b := range bars {
			// completed, unseen, and closed after registration
			if b.CloseTime <= 0 || b.CloseTime >= nowMs || b.CloseTime <= p.LastBarClose || b.CloseTime <= p.CreatedAt.UnixMilli() {
				continue
			}
			p.LastBarClose = b.CloseTime
			if (long && b.Low <= p.Invalidation) || (!long && b.High >= p.Invalidation) {
				delete(at.conditional.pending, k)
				logger.Infof("  ✖ [HUNTER V7 CONDITIONAL] %s %s cancelled: invalidation %.8f touched before the trigger", p.Decision.Symbol, p.Side, p.Invalidation)
				at.closePendingEntryRecord(p, store.LifecycleStatusEntryCancelled, "invalidation touched before the trigger")
				break
			}
			if !p.ArmedAt.IsZero() && b.CloseTime <= p.ArmedAt.UnixMilli() {
				continue // AUD-C6: a candle from before the re-arm can cancel, never fire
			}
			if (long && b.Close >= p.Trigger) || (!long && b.Close <= p.Trigger) {
				delete(at.conditional.pending, k) // removed before sending: executes at most once
				fire = append(fire, *p)
				break
			}
		}
	}
	at.conditional.mu.Unlock()
	for i := range fire {
		at.executeHunterV7PendingEntry(&fire[i])
	}
}

func (at *AutoTrader) executeHunterV7PendingEntry(p *hunterV7PendingEntry) {
	at.isRunningMutex.RLock()
	running := at.isRunning
	at.isRunningMutex.RUnlock()
	if !running || at.safeMode {
		logger.Infof("  ✖ [HUNTER V7 CONDITIONAL] %s %s dropped at trigger: trader stopped or in safe mode", p.Decision.Symbol, p.Side)
		at.closePendingEntryRecord(p, store.LifecycleStatusEntryCancelled, "dropped at trigger: trader stopped or in safe mode")
		return
	}
	at.conditional.entryMu.Lock()
	defer at.conditional.entryMu.Unlock()

	d := p.Decision
	rec := store.DecisionAction{Action: d.Action, Symbol: d.Symbol, Leverage: d.Leverage, StopLoss: d.StopLoss, TakeProfit: d.TakeProfit,
		HunterV7TP0Price: d.HunterV7TP0Price, HunterV7TP1Price: d.HunterV7TP1Price, HunterV7TP2Price: d.HunterV7TP2Price,
		Confidence: d.Confidence, Reasoning: d.Reasoning, Trigger: decisionTriggerForStore(d.Trigger), Timestamp: time.Now().UTC(),
		WhyThisCouldFail: string(d.WhyThisCouldFail)} // #111 S3
	ctx := &kernel.Context{CandidateCoins: []kernel.CandidateCoin{at.fireTimeCandidate(p, time.Now())}}
	exec := at.conditional.execute
	if exec == nil {
		exec = func(ctx *kernel.Context, decision *kernel.Decision, side string, record *store.DecisionAction) error {
			if side == "long" {
				return at.executeOpenLongWithRecord(ctx, decision, record)
			}
			return at.executeOpenShortWithRecord(ctx, decision, record)
		}
	}
	// AUD-C6: the execution guard (tier, tags, freshness, confirmations)
	// runs again at fire time on the latest snapshot.
	err := at.validateHunterV7ExecutionGuard(ctx, &d)
	if err == nil {
		err = exec(ctx, &d, p.Side, &rec)
	} else {
		err = fmt.Errorf("execution guard at fire time: %w", err)
	}
	line := fmt.Sprintf("⚡ %s %s conditional entry fired on a 1m close through %.8f", d.Symbol, d.Action, p.Trigger)
	if err != nil {
		at.closePendingEntryRecord(p, store.LifecycleStatusEntryCancelled, "failed at trigger: "+err.Error())
	} else {
		at.closePendingEntryRecord(p, store.LifecycleStatusEntryFired, fmt.Sprintf("fired on a 1m close through %.8f", p.Trigger))
	}
	if err != nil {
		rec.Error = err.Error()
		line = fmt.Sprintf("❌ %s %s conditional entry failed at trigger %.8f: %v", d.Symbol, d.Action, p.Trigger, err)
		logger.Errorf("  %s", line)
	} else {
		rec.Success = true
		logger.Infof("  %s", line)
	}
	record := &store.DecisionRecord{ExecutionLog: []string{line}, Decisions: []store.DecisionAction{rec}, Success: err == nil}
	if err := at.saveDecision(record); err != nil {
		logger.Warnf("⚠ Failed to save conditional entry record: %v", err)
	}
}

// startHunterV7ConditionalEntryMonitor runs the 1m candle check while the
// trader runs (only when conditional entry is enabled).
func (at *AutoTrader) startHunterV7ConditionalEntryMonitor() {
	if !at.conditionalEntryEnabled() {
		return
	}
	interval := time.Duration(at.conditionalEntryConfig().CheckIntervalSeconds * float64(time.Second))
	at.monitorWg.Add(1)
	go func() {
		defer at.monitorWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-at.stopMonitorCh:
				return
			case <-ticker.C:
				at.checkHunterV7ConditionalEntries(time.Now())
			}
		}
	}()
	at.logInfof("⏳ Hunter v7 conditional entry monitor enabled (1m candle check every %s)", interval)
}
