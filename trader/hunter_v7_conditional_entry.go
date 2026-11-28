package trader

import (
	"errors"
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

var errConditionalEntryDisabled = errors.New("conditional_entry_disabled")

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
	// U9 P0-2: provenance when registered via confirmation-only guard-veto reroute.
	ReroutedFromGuard bool
	ReroutedBlockCode string
	// Fire-time closed 1m bar evidence (in-memory only, populated when triggered).
	FiredBarClose     float64
	FiredBarCloseTime int64
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

type hunterV7ConditionalRegisterOptions struct {
	ReroutedFromGuard    bool
	ReroutedBlockCode    string
	TriggerOverride      float64
	InvalidationOverride float64
}

// hunterV7RerouteTriggerPrice resolves the trigger price for a P0-2 guard-veto
// reroute. It anchors strictly to the closed-bar confirmation predicate's own
// edge (EntryZone.Lower for SHORT, EntryZone.Upper for LONG), requiring a valid
// V7EntryZone. If an explicit trigger disagrees on the loose/inverted side
// (SHORT: trig > edge; LONG: trig < edge — e.g. trend_pullback_*), the setup is
// ineligible and returns 0 so a pending entry can never settle a predicate at a
// threshold the predicate itself does not evaluate. When an explicit trigger
// equals or is stricter than the edge, registration still anchors to the edge
// itself so firing settles the exact predicate that blocked.
func hunterV7RerouteTriggerPrice(candidate *kernel.CandidateCoin, decision *kernel.Decision, side string) float64 {
	if candidate == nil {
		return 0
	}
	z := candidate.V7EntryZone
	if !(z.Lower > 0 && z.Upper >= z.Lower) {
		return 0
	}
	var edge float64
	switch side {
	case "short":
		edge = z.Lower
	case "long":
		edge = z.Upper
	default:
		return 0
	}
	explicitAllowed := func(trig float64) bool {
		if trig <= 0 {
			return true
		}
		if side == "short" {
			return trig <= edge
		}
		return trig >= edge
	}
	if decision != nil && !explicitAllowed(decision.HunterV7TriggerPrice) {
		return 0
	}
	if !explicitAllowed(candidate.V7TriggerPrice) {
		return 0
	}
	return edge
}

// hunterV7RerouteGeometry validates that the candidate/decision carries both a
// valid predicate-edge trigger price and valid directional invalidation/stop
// geometry relative to that trigger before attempting a P0-2 reroute. When
// decision.StopLoss > 0 is tighter than the candidate/decision invalidation,
// the tighter stop is used as the pending invalidation (SHORT: min; LONG: max).
func hunterV7RerouteGeometry(candidate *kernel.CandidateCoin, decision *kernel.Decision, side string) (trigger, inval float64, ok bool) {
	trigger = hunterV7RerouteTriggerPrice(candidate, decision, side)
	if !(trigger > 0) {
		return 0, 0, false
	}
	if decision != nil && decision.HunterV7InvalidationPrice > 0 {
		inval = decision.HunterV7InvalidationPrice
	} else if candidate != nil && candidate.V7Invalidation.Price > 0 {
		inval = candidate.V7Invalidation.Price
	}
	hasSL := decision != nil && decision.StopLoss > 0
	if inval <= 0 {
		if hasSL {
			inval = decision.StopLoss
		} else {
			return 0, 0, false
		}
	}
	switch side {
	case "long":
		if !(inval < trigger) {
			return 0, 0, false
		}
		if hasSL {
			if !(decision.StopLoss < trigger) {
				return 0, 0, false
			}
			if decision.StopLoss > inval {
				inval = decision.StopLoss
			}
		}
	case "short":
		if !(inval > trigger) {
			return 0, 0, false
		}
		if hasSL {
			if !(decision.StopLoss > trigger) {
				return 0, 0, false
			}
			if decision.StopLoss < inval {
				inval = decision.StopLoss
			}
		}
	default:
		return 0, 0, false
	}
	return trigger, inval, true
}

// rerouteHunterV7ConfirmationVeto (U9 P0-2) routes an eligible approval whose
// SOLE execution-guard blocker is a closed-bar trigger confirmation through the
// daily-loss breaker check and into U9 conditional entry registration.
// Class-agnostic (keyed on block reason, not on strategy classes); remains
// fail-closed when v7_conditional_entry.enabled is false or breaker is armed.
func (at *AutoTrader) rerouteHunterV7ConfirmationVeto(ctx *kernel.Context, decision *kernel.Decision, side, blockCode string, trigger, inval float64, record *store.DecisionAction, now time.Time) error {
	// F08 / U3: breaker semantics cover rerouted registration identically to
	// direct opens and scan-time conditional registrations.
	if err := at.enforceDailyLossBreaker(decision.Symbol, decision.Action); err != nil {
		return err
	}
	return at.registerHunterV7ConditionalEntryWithOptions(ctx, decision, side, record, now, hunterV7ConditionalRegisterOptions{
		ReroutedFromGuard:    true,
		ReroutedBlockCode:    blockCode,
		TriggerOverride:      trigger,
		InvalidationOverride: inval,
	})
}

// registerHunterV7ConditionalEntry keeps an approved open as a pending
// entry instead of sending it (see the file comment).
func (at *AutoTrader) registerHunterV7ConditionalEntry(ctx *kernel.Context, decision *kernel.Decision, side string, record *store.DecisionAction, now time.Time) error {
	return at.registerHunterV7ConditionalEntryWithOptions(ctx, decision, side, record, now, hunterV7ConditionalRegisterOptions{})
}

func (at *AutoTrader) registerHunterV7ConditionalEntryWithOptions(ctx *kernel.Context, decision *kernel.Decision, side string, record *store.DecisionAction, now time.Time, opts hunterV7ConditionalRegisterOptions) error {
	if !opts.ReroutedFromGuard && !at.conditionalEntryEnabled() {
		// Fail closed: a conditional candidate must never be entered at market.
		return fmt.Errorf("❌ [HUNTER V7 CONDITIONAL] %s %s blocked: candidate is conditional on its trigger but conditional entry is disabled (%w)",
			decision.Symbol, decision.Action, errConditionalEntryDisabled)
	}
	candidate := hunterV7CandidateForDecision(ctx, decision)
	trigger := decision.HunterV7TriggerPrice
	if opts.ReroutedFromGuard && opts.TriggerOverride > 0 {
		trigger = opts.TriggerOverride
	}
	if candidate == nil || (!opts.ReroutedFromGuard && !candidate.V7ConditionalEntry) || trigger <= 0 {
		return fmt.Errorf("❌ [HUNTER V7 CONDITIONAL] %s %s blocked: no conditional candidate with a trigger price (conditional_contract_mismatch)",
			decision.Symbol, decision.Action)
	}
	storedDecision := *decision
	storedDecision.HunterV7TriggerPrice = trigger
	if err := at.validateOpenDecision(&storedDecision, trigger, side); err != nil {
		return fmt.Errorf("❌ [HUNTER V7 CONDITIONAL] %s %s rejected at the trigger %.8f: %w", decision.Symbol, decision.Action, trigger, err)
	}
	if opts.ReroutedFromGuard && !at.conditionalEntryEnabled() {
		// Fail closed on the reroute path after validating the open contract at
		// trigger so infeasible approvals record reroute_conditional_entry_failed
		// rather than inflating reroute_conditional_entry_disabled telemetry.
		return fmt.Errorf("❌ [HUNTER V7 CONDITIONAL] %s %s blocked: candidate is conditional on its trigger but conditional entry is disabled (%w)",
			decision.Symbol, decision.Action, errConditionalEntryDisabled)
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
	if opts.ReroutedFromGuard && opts.InvalidationOverride > 0 {
		inval = opts.InvalidationOverride
	} else if inval <= 0 && opts.InvalidationOverride > 0 {
		inval = opts.InvalidationOverride
	}
	if inval <= 0 {
		inval = decision.StopLoss
	}
	if opts.ReroutedFromGuard || storedDecision.HunterV7InvalidationPrice <= 0 {
		storedDecision.HunterV7InvalidationPrice = inval
	}
	storedDecision.HunterV7IntentKey = at.openIntentKey(decision, side) + "|conditional"
	p := &hunterV7PendingEntry{
		Decision:          storedDecision,
		Candidate:         *candidate,
		Side:              side,
		Trigger:           trigger,
		Invalidation:      inval,
		CreatedAt:         now,
		ExpiresAt:         now.Add(time.Duration(at.conditionalEntryConfig().MaxPendingMinutes * float64(time.Minute))),
		ReroutedFromGuard: opts.ReroutedFromGuard,
		ReroutedBlockCode: opts.ReroutedBlockCode,
	}
	at.persistPendingEntry(p) // AUD-C6
	at.conditional.pending[key] = p
	if record != nil {
		record.ConditionalPending = true
		record.Price = trigger
	}
	if opts.ReroutedFromGuard {
		logger.Infof("  ⏳ [HUNTER V7 CONDITIONAL] %s %s pending (rerouted_conditional_entry blocker=%s): enters on a 1m close through %.8f, cancelled at %.8f, expires %s",
			decision.Symbol, side, opts.ReroutedBlockCode, trigger, inval, p.ExpiresAt.Format(time.RFC3339))
	} else {
		logger.Infof("  ⏳ [HUNTER V7 CONDITIONAL] %s %s pending: enters on a 1m close through %.8f, cancelled at %.8f, expires %s",
			decision.Symbol, side, trigger, inval, p.ExpiresAt.Format(time.RFC3339))
	}
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
				firedCopy := *p
				firedCopy.FiredBarClose = b.Close
				firedCopy.FiredBarCloseTime = b.CloseTime
				fire = append(fire, firedCopy)
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
	if p.ReroutedFromGuard {
		rec.ConditionalRerouteSource = p.ReroutedBlockCode
	}
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
