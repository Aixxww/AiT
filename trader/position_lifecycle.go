package trader

// #92 / audit cluster 2 (docs/audit/optimization-design-20261124.md):
// the transactional open path.
//
//   - E3: a lifecycle row ("intent", with the plan) is written before the
//     order and updated after it, so a crash between fill and save leaves a
//     row that startup reconciles against the exchange.
//   - C4: the fill (executedQty, avgPrice) is read back from the exchange;
//     the record, the stop and the take-profit use it. Stop and TP keep the
//     plan's distances from the entry, measured from the fill.
//   - C7: the book is re-read after the last pre-order REST call and the
//     entry drift check is applied again.
//   - D6: one mutex per position (symbol + side) is shared by the LLM close
//     path and the protector; an LLM close cancels the protection orders
//     that position owns.

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// ---- D6: per-position lock ------------------------------------------------

// positionLock serializes close and protect actions on one position.
// closedAt is the last LLM close; the protector skips observations taken
// before it.
type positionLock struct {
	mu       sync.Mutex
	closedAt time.Time
}

func positionLockKey(symbol, side string) string {
	return strings.ToUpper(market.Normalize(symbol)) + "_" + strings.ToLower(strings.TrimSpace(side))
}

func (at *AutoTrader) positionLockFor(symbol, side string) *positionLock {
	v, _ := at.positionLocks.LoadOrStore(positionLockKey(symbol, side), &positionLock{})
	return v.(*positionLock)
}

// lockPosition takes the position's lock and returns the unlock func.
func (at *AutoTrader) lockPosition(symbol, side string) (*positionLock, func()) {
	l := at.positionLockFor(symbol, side)
	l.mu.Lock()
	return l, l.mu.Unlock
}

// llmClosePosition closes a whole position for the LLM close path under
// the position lock (D6), then cancels the protection orders this position
// owns and closes its lifecycle row. Observations the protector took before
// the close are skipped (closedAt).
func (at *AutoTrader) llmClosePosition(symbol, side string) (map[string]interface{}, error) {
	l, unlock := at.lockPosition(symbol, side)
	defer unlock()
	var (
		order map[string]interface{}
		err   error
	)
	if side == "long" {
		order, err = at.trader.CloseLong(symbol, 0) // 0 = close all
	} else {
		order, err = at.trader.CloseShort(symbol, 0)
	}
	if err != nil {
		return nil, err
	}
	l.closedAt = time.Now()
	at.cancelOwnedProtectionAfterClose(symbol, side)
	at.markLifecycleClosed(symbol, side, "closed by the LLM")
	return order, nil
}

// cancelOwnedProtectionAfterClose cancels the closed side's stop and TP.
// Exchanges without side-scoped cancel only cancel symbol-wide when no
// position is left on the symbol, so a hedge-mode opposite side keeps its
// protection.
func (at *AutoTrader) cancelOwnedProtectionAfterClose(symbol, side string) {
	if c, ok := at.trader.(sideScopedProtectionCanceller); ok {
		if err := c.CancelOwnedProtectionOrders(symbol, protectionPositionSide(side), false); err != nil {
			logger.Warnf("  ⚠ %s %s closed; owned protection not cancelled: %v", symbol, side, err)
		}
		return
	}
	positions, err := at.freshPositions()
	if err != nil {
		logger.Warnf("  ⚠ %s %s closed; protection left in place (positions unavailable: %v)", symbol, side, err)
		return
	}
	for _, s := range []string{"long", "short"} {
		if pos, ok := findPosition(positions, symbol, s); ok && math.Abs(positionFloat(pos, "positionAmt")) > 0 {
			logger.Infof("  %s %s closed; %s side still open, symbol-wide cancel skipped", symbol, side, s)
			return
		}
	}
	if err := at.trader.CancelStopOrders(symbol); err != nil {
		logger.Warnf("  ⚠ %s %s closed; protection not cancelled: %v", symbol, side, err)
	}
}

// ---- E3 / C4: lifecycle record ---------------------------------------------

func (at *AutoTrader) lifecycleStore() *store.DecisionStore {
	if at == nil || at.store == nil {
		return nil
	}
	return at.store.Decision()
}

// recordOpenIntent writes the "intent" row before the order. A store
// failure is logged and does not block the open (the exchange-side
// protections do not depend on it).
func (at *AutoTrader) recordOpenIntent(decision *kernel.Decision, side string, quantity, entryPrice float64) *store.PositionLifecycle {
	ds := at.lifecycleStore()
	if ds == nil || decision == nil {
		return nil
	}
	rec := &store.PositionLifecycle{
		TraderID:       at.id,
		Symbol:         decision.Symbol,
		Side:           side,
		Status:         store.LifecycleStatusIntent,
		SignalID:       decision.SelectedHunterV7SignalID,
		IntentKey:      at.openIntentKey(decision, side),
		PlanEntryPrice: entryPrice,
		PlanStopLoss:   decision.StopLoss,
		PlanTakeProfit: decision.TakeProfit,
		PlanQuantity:   quantity,
		PlanSizeUSD:    decision.PositionSizeUSD,
		Leverage:       decision.Leverage,
	}
	if err := ds.CreateLifecycle(rec); err != nil {
		logger.Errorf("  ⚠ [LIFECYCLE] %s %s intent not persisted: %v", decision.Symbol, side, err)
		return nil
	}
	return rec
}

func (at *AutoTrader) updateLifecycle(rec *store.PositionLifecycle, fields map[string]interface{}) {
	ds := at.lifecycleStore()
	if ds == nil || rec == nil || rec.ID == 0 {
		return
	}
	if err := ds.UpdateLifecycle(rec.ID, fields); err != nil {
		logger.Errorf("  ⚠ [LIFECYCLE] %s %s row %d not updated: %v", rec.Symbol, rec.Side, rec.ID, err)
	}
}

// markLifecycleClosed closes the newest open/intent row of a position.
func (at *AutoTrader) markLifecycleClosed(symbol, side, note string) {
	ds := at.lifecycleStore()
	if ds == nil {
		return
	}
	rec, err := ds.LatestLifecycle(at.id, symbol, side, store.LifecycleStatusOpen, store.LifecycleStatusIntent)
	if err != nil || rec == nil {
		return
	}
	at.updateLifecycle(rec, map[string]interface{}{"status": store.LifecycleStatusClosed, "note": note})
}

// openFill is the exchange's report of an open order.
type openFill struct {
	Qty       float64
	AvgPrice  float64
	Confirmed bool   // reported by the exchange (order or position)
	Partial   bool   // executed less than requested
	NotFilled bool   // terminal order state with nothing executed and no position
	Source    string // "order", "order_status", "position", "requested"
}

const (
	fillReadBackAttempts = 3
	fillReadBackDelay    = 400 * time.Millisecond
)

func mapFloat(m map[string]interface{}, key string) float64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

func terminalOrderStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "CANCELED", "CANCELLED", "EXPIRED", "REJECTED", "EXPIRED_IN_MATCH":
		return true
	}
	return false
}

// readBackOpenFill reads the fill of an open order: the order response
// first, then GetOrderStatus, then the position itself. When nothing is
// reported it falls back to the requested quantity and price, unconfirmed,
// which keeps the previous behaviour (protect as if filled).
func (at *AutoTrader) readBackOpenFill(symbol, side string, order map[string]interface{}, requestedQty, requestedPrice float64) openFill {
	res := openFill{Qty: requestedQty, AvgPrice: requestedPrice, Source: "requested"}
	finish := func(qty, price float64, source string) openFill {
		res.Qty, res.Confirmed, res.Source = qty, true, source
		if price > 0 {
			res.AvgPrice = price
		}
		res.Partial = requestedQty > 0 && qty < requestedQty*0.999
		return res
	}
	if q := mapFloat(order, "executedQty"); q > 0 {
		return finish(q, mapFloat(order, "avgPrice"), "order")
	}

	terminal := terminalOrderStatus(fmt.Sprint(order["status"]))
	orderID := int64(mapFloat(order, "orderId"))
	if orderID > 0 && at.trader != nil {
		sleep := at.readBackSleep
		if sleep == nil {
			sleep = time.Sleep
		}
		for i := 0; i < fillReadBackAttempts; i++ {
			st, err := at.trader.GetOrderStatus(symbol, strconv.FormatInt(orderID, 10))
			if err == nil && st != nil {
				if q := mapFloat(st, "executedQty"); q > 0 {
					return finish(q, mapFloat(st, "avgPrice"), "order_status")
				}
				status := strings.ToUpper(fmt.Sprint(st["status"]))
				if terminalOrderStatus(status) {
					terminal = true
					break
				}
				if status == "FILLED" {
					return finish(requestedQty, mapFloat(st, "avgPrice"), "order_status")
				}
			}
			if i < fillReadBackAttempts-1 {
				sleep(fillReadBackDelay)
			}
		}
	}

	// The position is the ground truth when the order report is silent.
	if positions, err := at.freshPositions(); err == nil {
		if pos, ok := findPosition(positions, symbol, side); ok {
			if q := math.Abs(positionFloat(pos, "positionAmt")); q > 0 {
				return finish(q, positionFloat(pos, "entryPrice"), "position")
			}
		}
		if terminal {
			res.NotFilled = true
			res.Qty = 0
		}
	}
	return res
}

// fillAnchoredProtection moves the plan's stop and TP with the fill: the
// distances from the planned entry are kept, measured from the average
// fill price. Invalid results keep the plan's prices.
func fillAnchoredProtection(side string, planEntry, fillPrice, stop, tp float64) (float64, float64) {
	if planEntry <= 0 || fillPrice <= 0 || stop <= 0 {
		return stop, tp
	}
	shift := fillPrice - planEntry
	newStop := stop + shift
	newTP := tp
	if tp > 0 {
		newTP = tp + shift
	}
	switch side {
	case "long":
		if newStop <= 0 || newStop >= fillPrice || (tp > 0 && newTP <= fillPrice) {
			return stop, tp
		}
	case "short":
		if newStop <= fillPrice || (tp > 0 && (newTP <= 0 || newTP >= fillPrice)) {
			return stop, tp
		}
	default:
		return stop, tp
	}
	return newStop, newTP
}

// ---- C7: book re-read before the order ------------------------------------

// recheckOpenDriftBeforeOrder re-reads the book after the last pre-order
// REST call and applies the entry drift check again.
func (at *AutoTrader) recheckOpenDriftBeforeOrder(decision *kernel.Decision, side string, plannedPrice float64) (float64, error) {
	latest := at.resolveOpenExecutionPrice(decision.Symbol, side, plannedPrice)
	if latest <= 0 {
		return plannedPrice, nil
	}
	if err := at.checkEntryPriceDrift(decision, latest); err != nil {
		return latest, fmt.Errorf("%w (book re-read before order)", err)
	}
	return latest, nil
}

// ---- shared open tail ------------------------------------------------------

// placeOpenOrderWithLifecycle is the common tail of both open paths: margin
// mode, the C7 re-check, the intent row, the order, the fill read-back, the
// fill-anchored stop and TP, and the lifecycle update.
func (at *AutoTrader) placeOpenOrderWithLifecycle(decision *kernel.Decision, side string, quantity, executionPrice, requestedPositionSize float64, tpWasCapped, positionWasReduced bool, actionRecord *store.DecisionAction) error {
	upper := strings.ToUpper(side)

	// Set margin mode (the last pre-order REST call in this path).
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Infof("  ⚠️ Failed to set margin mode: %v", err)
		// Continue execution, doesn't affect trading
	}

	// C7: the book may have moved during the REST calls above.
	if _, err := at.recheckOpenDriftBeforeOrder(decision, side, executionPrice); err != nil {
		return err
	}

	// E3: the plan is persisted before the order.
	lc := at.recordOpenIntent(decision, side, quantity, executionPrice)

	order, err := at.openMarketPosition(decision, side, quantity)
	if err != nil {
		if errors.Is(err, errOpenOrderStateUnknown) {
			// AUD-C5: the order may have filled; protect it or close it.
			// The row stays "intent" so startup reconciles it too.
			at.updateLifecycle(lc, map[string]interface{}{"note": "order_state_unknown: " + err.Error()})
			return at.handleOpenOrderStateUnknown(decision, side, actionRecord, err)
		}
		at.updateLifecycle(lc, map[string]interface{}{"status": store.LifecycleStatusFailed, "note": err.Error()})
		return err
	}

	orderID, _ := order["orderId"].(int64)
	if orderID != 0 {
		actionRecord.OrderID = orderID
	}

	// C4: read back the fill.
	fill := at.readBackOpenFill(decision.Symbol, side, order, quantity, executionPrice)
	if fill.NotFilled {
		at.updateLifecycle(lc, map[string]interface{}{"status": store.LifecycleStatusFailed, "order_id": orderID, "note": "order ended without a fill"})
		return fmt.Errorf("❌ %s %s order %d ended without a fill", decision.Symbol, side, orderID)
	}
	planStop, planTP := decision.StopLoss, decision.TakeProfit
	stop, tp := fillAnchoredProtection(side, executionPrice, fill.AvgPrice, planStop, planTP)
	decision.StopLoss, decision.TakeProfit = stop, tp
	if fill.Qty > 0 && fill.AvgPrice > 0 {
		decision.PositionSizeUSD = fill.Qty * fill.AvgPrice
	}
	at.recordEffectiveOpenContract(actionRecord, decision, fill.AvgPrice, fill.Qty, requestedPositionSize, tpWasCapped, positionWasReduced || fill.Partial, side)
	at.updateLifecycle(lc, map[string]interface{}{
		"status":         store.LifecycleStatusOpen,
		"order_id":       orderID,
		"executed_qty":   fill.Qty,
		"avg_fill_price": fill.AvgPrice,
		"fill_confirmed": fill.Confirmed,
		"stop_loss":      stop,
		"take_profit":    tp,
		"note":           "fill source: " + fill.Source,
		"first_seen_at":  time.Now().UnixMilli(), // #93 / AUD-D4
	})
	if fill.Partial {
		logger.Warnf("  ⚠ %s %s partial fill: %.8f of %.8f", decision.Symbol, side, fill.Qty, quantity)
	}

	logger.Infof("  ✓ Position opened successfully, order ID: %v, quantity: %.4f @ %.8f (%s)", order["orderId"], fill.Qty, fill.AvgPrice, fill.Source)

	at.ClearPeakPnLCache(decision.Symbol, side)

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "open_"+side, fill.Qty, fill.AvgPrice, decision.Leverage, 0)

	// Record position opening time
	at.positionFirstSeenTime[decision.Symbol+"_"+side] = time.Now().UnixMilli()

	// Set stop loss and take profit
	// F03: the stop is mandatory. If it cannot be placed the position is
	// emergency-closed and verified flat; the take-profit is not placed.
	if err := at.trader.SetStopLoss(decision.Symbol, upper, fill.Qty, stop); err != nil {
		at.updateLifecycle(lc, map[string]interface{}{"note": "stop not placed: " + err.Error()})
		return at.closeUnprotectedPosition(decision.Symbol, side, fill.Qty, fmt.Errorf("failed to set mandatory stop loss: %w", err))
	}
	// A missing take-profit leaves the position protected by its stop; it is
	// reported, not fatal.
	if err := at.trader.SetTakeProfit(decision.Symbol, upper, fill.Qty, tp); err != nil {
		logger.Errorf("  ⚠ Failed to set take profit for %s %s (stop is in place): %v", decision.Symbol, upper, err)
		actionRecord.Error = fmt.Sprintf("take_profit_not_placed: %v", err)
	}
	return nil
}

// ---- startup reconciliation -------------------------------------------------

// reconcileLifecycleOnStartup runs before the protector starts. Rows still
// in "intent" are matched against the exchange's positions:
//   - a position exists: the row becomes "open" with the exchange's size and
//     entry, and a missing stop is placed from the plan's stop distance (or
//     the position is closed if that fails, as F03);
//   - no position: the row becomes "failed".
//
// Pending conditional entries are re-armed or visibly cancelled (C6).
func (at *AutoTrader) reconcileLifecycleOnStartup(now time.Time) {
	ds := at.lifecycleStore()
	if ds == nil {
		return
	}
	intents, err := ds.ListLifecycles(at.id, store.LifecycleStatusIntent)
	if err != nil {
		logger.Errorf("  ⚠ [LIFECYCLE] reconcile: cannot list intent rows: %v", err)
	} else if len(intents) > 0 {
		positions, perr := at.freshPositions()
		if perr != nil {
			logger.Errorf("  ⚠ [LIFECYCLE] reconcile: positions unavailable, %d intent rows left for the next start: %v", len(intents), perr)
		} else {
			for i := range intents {
				at.reconcileIntentRow(&intents[i], positions)
			}
		}
	}
	at.restorePendingConditionalEntries(now)
}

func (at *AutoTrader) reconcileIntentRow(rec *store.PositionLifecycle, positions []map[string]interface{}) {
	side := strings.ToLower(rec.Side)
	pos, ok := findPosition(positions, rec.Symbol, side)
	qty := 0.0
	if ok {
		qty = math.Abs(positionFloat(pos, "positionAmt"))
	}
	if !ok || qty <= 0 {
		logger.Warnf("  [LIFECYCLE] reconcile: %s %s intent row %d has no exchange position; marked failed", rec.Symbol, rec.Side, rec.ID)
		at.updateLifecycle(rec, map[string]interface{}{"status": store.LifecycleStatusFailed, "note": "reconcile: no exchange position"})
		return
	}
	entry := positionFloat(pos, "entryPrice")
	stop, tp := fillAnchoredProtection(side, rec.PlanEntryPrice, entry, rec.PlanStopLoss, rec.PlanTakeProfit)
	fields := map[string]interface{}{
		"status":         store.LifecycleStatusOpen,
		"executed_qty":   qty,
		"avg_fill_price": entry,
		"fill_confirmed": true,
		"stop_loss":      stop,
		"take_profit":    tp,
		"note":           "reconciled at startup from the exchange position",
	}
	stopOpen := false
	if checker, ok := at.trader.(protectiveStopChecker); ok {
		if open, err := checker.ProtectiveStopOpen(rec.Symbol, strings.ToUpper(side)); err == nil {
			stopOpen = open
		}
	}
	if !stopOpen && stop > 0 {
		if err := at.trader.SetStopLoss(rec.Symbol, strings.ToUpper(side), qty, stop); err != nil {
			fields["note"] = "reconciled at startup; stop not placed, position closed: " + err.Error()
			at.updateLifecycle(rec, fields)
			_ = at.closeUnprotectedPosition(rec.Symbol, side, qty, fmt.Errorf("reconcile: failed to place stop: %w", err))
			at.markLifecycleClosed(rec.Symbol, side, "closed at reconcile: no stop")
			return
		}
		if tp > 0 {
			if err := at.trader.SetTakeProfit(rec.Symbol, strings.ToUpper(side), qty, tp); err != nil {
				logger.Errorf("  ⚠ [LIFECYCLE] reconcile: %s %s take profit not placed (stop is in place): %v", rec.Symbol, rec.Side, err)
			}
		}
	}
	logger.Infof("  [LIFECYCLE] reconcile: %s %s intent row %d adopted (qty %.8f, entry %.8f, stop placed=%v)", rec.Symbol, rec.Side, rec.ID, qty, entry, !stopOpen)
	at.updateLifecycle(rec, fields)
}
