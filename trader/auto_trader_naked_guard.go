package trader

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// AUD-C5 (P0): a filled position must end protected by an exchange stop or
// closed, and a position that is neither must be surfaced.
//
// Before this change three paths could leave a position without a stop:
//   - an open whose read-back was inconclusive returned order_state_unknown
//     without placing a stop, although the order may have filled;
//   - a failed emergency close only returned "position may be unprotected";
//   - nothing checked later whether a stop was still on the exchange.
//
// Now:
//   - handleOpenOrderStateUnknown treats the order as filled: it reads the
//     live position and places the decision's stop (or closes the position);
//   - auditProtectiveStop runs on every protector poll for every position: if
//     the exchange reports no stop on the side, it re-arms one from the plan
//     (or from the protector's hard-loss distance when no plan exists), or
//     closes the position if price is already beyond that stop;
//   - every position that stays without a stop is recorded as a naked-position
//     alert (error log on every poll, GetStatus "naked_position_alerts") until
//     a stop is confirmed or the position is gone.
//
// The protector's polled hard-loss market close is unchanged and still runs
// after the audit.

// nakedStopAuditGrace skips the stop audit for a position this young, so the
// poll does not race the open path, which places the stop right after the
// fill.
const nakedStopAuditGrace = 20 * time.Second

// errNakedEmergencyCloseFailed marks an emergency close that did not flatten
// the position; the position may still be open without a stop.
var errNakedEmergencyCloseFailed = errors.New("emergency close FAILED")

// NakedPositionAlert is one position without a confirmed exchange stop.
type NakedPositionAlert struct {
	Symbol   string    `json:"symbol"`
	Side     string    `json:"side"`
	Reason   string    `json:"reason"`
	Since    time.Time `json:"since"`
	LastSeen time.Time `json:"last_seen"`
	Count    int       `json:"count"`
}

type nakedPositionAlerts struct {
	mu sync.Mutex
	m  map[string]*NakedPositionAlert
}

func nakedKey(symbol, side string) string {
	return market.Normalize(symbol) + "_" + strings.ToLower(side)
}

// raiseNakedPositionAlert records (or refreshes) the alert and logs it at
// error level every time, so an unresolved naked position is never silent.
func (at *AutoTrader) raiseNakedPositionAlert(symbol, side, reason string) {
	now := protectionNow()
	at.naked.mu.Lock()
	if at.naked.m == nil {
		at.naked.m = map[string]*NakedPositionAlert{}
	}
	key := nakedKey(symbol, side)
	a, ok := at.naked.m[key]
	if !ok {
		a = &NakedPositionAlert{Symbol: market.Normalize(symbol), Side: strings.ToLower(side), Since: now}
		at.naked.m[key] = a
	}
	a.Reason, a.LastSeen = reason, now
	a.Count++
	since, count := a.Since, a.Count
	at.naked.mu.Unlock()
	at.logErrorf("🚨 NAKED POSITION %s %s has no confirmed exchange stop (since %s, alert #%d): %s",
		symbol, side, since.Format(time.RFC3339), count, reason)
}

func (at *AutoTrader) clearNakedPositionAlert(symbol, side, resolution string) {
	at.naked.mu.Lock()
	key := nakedKey(symbol, side)
	_, ok := at.naked.m[key]
	delete(at.naked.m, key)
	at.naked.mu.Unlock()
	if ok {
		at.logInfof("✅ Naked-position alert resolved for %s %s: %s", symbol, side, resolution)
	}
}

// NakedPositionAlerts returns the unresolved alerts (oldest first).
func (at *AutoTrader) NakedPositionAlerts() []NakedPositionAlert {
	at.naked.mu.Lock()
	defer at.naked.mu.Unlock()
	out := make([]NakedPositionAlert, 0, len(at.naked.m))
	for _, a := range at.naked.m {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Since.Equal(out[j].Since) {
			return out[i].Since.Before(out[j].Since)
		}
		return nakedKey(out[i].Symbol, out[i].Side) < nakedKey(out[j].Symbol, out[j].Side)
	})
	return out
}

// clearNakedAlertsForClosedPositions drops alerts whose position is gone.
func (at *AutoTrader) clearNakedAlertsForClosedPositions(positions []map[string]interface{}) {
	open := map[string]bool{}
	for _, pos := range positions {
		open[nakedKey(fmt.Sprint(pos["symbol"]), fmt.Sprint(pos["side"]))] = true
	}
	for _, a := range at.NakedPositionAlerts() {
		if !open[nakedKey(a.Symbol, a.Side)] {
			at.clearNakedPositionAlert(a.Symbol, a.Side, "position no longer open")
		}
	}
}

func findPosition(positions []map[string]interface{}, symbol, side string) (map[string]interface{}, bool) {
	for _, pos := range positions {
		if market.Normalize(fmt.Sprint(pos["symbol"])) == market.Normalize(symbol) && strings.EqualFold(fmt.Sprint(pos["side"]), side) {
			return pos, true
		}
	}
	return nil, false
}

func positionFloat(pos map[string]interface{}, key string) float64 {
	if v, ok := pos[key].(float64); ok {
		return v
	}
	return 0
}

// handleOpenOrderStateUnknown treats an unconfirmed open as filled (AUD-C5).
// It returns nil when a live position was found and protected (the open is
// then recorded as successful, with a note), and an error otherwise.
func (at *AutoTrader) handleOpenOrderStateUnknown(decision *kernel.Decision, side string, actionRecord *store.DecisionAction, openErr error) error {
	symbol := decision.Symbol
	logger.Errorf("  🚨 %s %s open state unknown (%v); assuming filled and checking the live position", symbol, side, openErr)
	positions, err := at.freshPositions()
	if err != nil {
		at.raiseNakedPositionAlert(symbol, side, fmt.Sprintf("open order state unknown and position read failed: %v", err))
		return fmt.Errorf("%w; assume-filled position read failed: %v (naked-position alert raised; the protector poll re-checks)", openErr, err)
	}
	pos, ok := findPosition(positions, symbol, side)
	if !ok {
		// Not filled as far as the exchange reports; a late fill is caught
		// by the per-poll stop audit.
		return fmt.Errorf("%w; assume-filled check: no %s %s position on the exchange", openErr, symbol, side)
	}
	qty := math.Abs(positionFloat(pos, "positionAmt"))
	if qty <= 0 {
		qty = actionRecord.Quantity
	}
	if err := at.trader.SetStopLoss(symbol, protectionPositionSide(side), qty, decision.StopLoss); err != nil {
		return at.closeUnprotectedPosition(symbol, side, qty, fmt.Errorf("%w; position found after unknown open state, but the mandatory stop failed: %v", openErr, err))
	}
	if err := at.trader.SetTakeProfit(symbol, protectionPositionSide(side), qty, decision.TakeProfit); err != nil {
		logger.Errorf("  ⚠ Failed to set take profit for %s %s after unknown open state (stop is in place): %v", symbol, side, err)
	}
	at.ClearPeakPnLCache(symbol, side)
	at.positionFirstSeenTime[symbol+"_"+side] = time.Now().UnixMilli()
	actionRecord.Quantity = qty
	actionRecord.Error = fmt.Sprintf("order_state_unknown_assumed_filled: position %.8f found and stop %.8f placed (%v)", qty, decision.StopLoss, openErr)
	logger.Infof("  ✓ %s %s position found after unknown open state; stop %.8f placed", symbol, side, decision.StopLoss)
	return nil
}

// defaultRearmStopFrac is the stop distance (fraction of entry) used when no
// plan or protector stop is known: the protector's hard-loss level, so the
// exchange stop sits where the polled hard-loss close would act anyway.
func defaultRearmStopFrac(e *protectorEngine, side string, leverage int) float64 {
	if frac, ok := (protectorPos{Side: side, Leverage: leverage}).priceFrac(e.cfg.HardLoss); ok && frac != 0 {
		return math.Abs(frac)
	}
	return math.Abs(store.DefaultProtectorHardLossROEPct) / 100 / float64(store.DefaultProtectorLeverage)
}

// rearmStopPrice picks the stop to re-arm: the most protective of the plan's
// stop and the protector's active/dynamic stop, else the default distance.
func rearmStopPrice(e *protectorEngine, side string, entry float64, leverage int, plan plannedPositionRisk, state *positionProtectionState) (float64, string) {
	protector := 0.0
	if state != nil {
		protector = mostProtectiveStop(side, state.ActiveStopLoss, state.DynamicStop)
	}
	if stop := mostProtectiveStop(side, plan.stopLoss, protector); stop > 0 {
		if stop == plan.stopLoss {
			return stop, "plan"
		}
		return stop, "protector"
	}
	frac := defaultRearmStopFrac(e, side, leverage)
	if side == "long" {
		return entry * (1 - frac), "default_hard_loss_distance"
	}
	return entry * (1 + frac), "default_hard_loss_distance"
}

// auditProtectiveStop is the per-poll stop-present check (AUD-C5). It
// returns true when it closed the position.
func (at *AutoTrader) auditProtectiveStop(e *protectorEngine, o protectorObs) bool {
	checker, ok := at.trader.(protectiveStopChecker)
	if !ok {
		return false // the exchange cannot report stops; nothing to audit
	}
	if !o.OpenedAt.IsZero() && o.Now.Sub(o.OpenedAt) < nakedStopAuditGrace {
		return false
	}
	posSide := protectionPositionSide(o.Side)
	open, err := checker.ProtectiveStopOpen(o.Symbol, posSide)
	if err != nil {
		at.raiseNakedPositionAlert(o.Symbol, o.Side, fmt.Sprintf("stop check failed: %v", err))
		return false
	}
	if open {
		at.clearNakedPositionAlert(o.Symbol, o.Side, "exchange stop present")
		return false
	}

	state := at.protectionStateFor(o.Symbol, o.Side)
	stop, source := rearmStopPrice(e, o.Side, o.Entry, o.Leverage, o.Plan, state)
	if !isStopOnProtectiveSide(o.Side, stop, o.Mark) {
		// Price is already beyond the stop the position should have had.
		err := at.closeUnprotectedPosition(o.Symbol, o.Side, o.Qty,
			fmt.Errorf("no exchange stop on %s %s and mark %.8f is already beyond the %s stop %.8f", o.Symbol, o.Side, o.Mark, source, stop))
		return !errors.Is(err, errNakedEmergencyCloseFailed)
	}
	if err := at.trader.SetStopLoss(o.Symbol, posSide, o.Qty, stop); err != nil {
		cerr := at.closeUnprotectedPosition(o.Symbol, o.Side, o.Qty,
			fmt.Errorf("no exchange stop on %s %s and re-arming the %s stop %.8f failed: %v", o.Symbol, o.Side, source, stop, err))
		return !errors.Is(cerr, errNakedEmergencyCloseFailed)
	}
	if state != nil {
		at.protectionStateMutex.Lock()
		state.ActiveStopLoss = mostProtectiveStop(o.Side, state.ActiveStopLoss, stop)
		at.protectionStateMutex.Unlock()
	}
	at.logErrorf("🛡 Re-armed missing exchange stop for %s %s at %.8f (%s)", o.Symbol, o.Side, stop, source)
	at.clearNakedPositionAlert(o.Symbol, o.Side, fmt.Sprintf("stop re-armed at %.8f (%s)", stop, source))
	return false
}
