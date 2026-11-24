package trader

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Aixxww/AiT/logger"
)

// F27 + F28: the position protector's stop replacement.
//
// F27: the breakeven rebuild after a partial close used CancelStopOrders
// (symbol-wide: on Binance every algo order of the symbol, both hedge sides
// and the take-profit) and the dynamic stop update used
// CancelStopLossOrders (symbol-wide stops). Both removed the opposite hedge
// side's protection and manual orders. Exchanges that can scope the cancel
// to this bot's own orders on one side (Binance:
// CancelOwnedProtectionOrders, built on the F26 ownership rule) now do;
// others keep their previous call, so nothing regresses there.
//
// F28: a failed cancel was swallowed and a failed SetStopLoss after a
// partial close was only logged, so the remainder could be left without a
// stop. Now the new stop is always attempted; if it fails, the protector
// confirms that some stop still protects the side (ProtectiveStopOpen where
// the exchange supports it) and otherwise emergency-closes the remainder
// with the F03 close-and-verify helper. An unknown protection state (check
// unavailable or failed) counts as unprotected, as in F03.

// sideScopedProtectionCanceller is implemented by exchanges that can cancel
// only this bot's protective orders on one position side.
type sideScopedProtectionCanceller interface {
	CancelOwnedProtectionOrders(symbol, positionSide string, stopsOnly bool) error
}

// protectiveStopChecker is implemented by exchanges that can report whether
// a stop order is open on one position side.
type protectiveStopChecker interface {
	ProtectiveStopOpen(symbol, positionSide string) (bool, error)
}

var (
	// errProtectionEmergencyClosed: the position was emergency-closed (and
	// verified flat) because its protection could not be confirmed.
	errProtectionEmergencyClosed = errors.New("position emergency-closed: protective stop could not be confirmed")
	// errProtectionCancelUnconfirmed: the new stop IS in place, but the
	// cancel of the old protection was not confirmed (a surplus owned order
	// may remain; the F26 owned cancel removes it on the next open or close).
	errProtectionCancelUnconfirmed = errors.New("new stop placed; old protection cancel not confirmed")
)

func protectionPositionSide(side string) string {
	if strings.EqualFold(side, "long") {
		return "LONG"
	}
	return "SHORT"
}

// cancelSideProtection cancels the managed side's protective orders:
// side-scoped and owned-only where supported (F27), else the exchange's
// previous symbol-wide call.
func (at *AutoTrader) cancelSideProtection(symbol, side string, stopsOnly bool) error {
	if c, ok := at.trader.(sideScopedProtectionCanceller); ok {
		return c.CancelOwnedProtectionOrders(symbol, protectionPositionSide(side), stopsOnly)
	}
	if stopsOnly {
		return at.trader.CancelStopLossOrders(symbol)
	}
	return at.trader.CancelStopOrders(symbol)
}

// replaceProtectiveStop replaces the managed side's protection with one
// stop at stopPrice (F27/F28). stopsOnly keeps take-profit orders (dynamic
// update); otherwise the side's take-profit is cancelled too, as before.
//
// Outcomes:
//   - nil: the new stop is in place and the old protection was cancelled.
//   - errProtectionCancelUnconfirmed: the new stop is in place.
//   - errProtectionEmergencyClosed (closedAll=true): the new stop failed, no
//     stop was confirmed on the side, and the remainder was closed flat.
//   - any other error: the new stop failed and either an existing stop still
//     protects the side, or the emergency close itself failed.
func (at *AutoTrader) replaceProtectiveStop(symbol, side string, quantity, stopPrice float64, stopsOnly bool) (closedAll bool, err error) {
	positionSide := protectionPositionSide(side)
	cancelErr := at.cancelSideProtection(symbol, side, stopsOnly)
	if cancelErr != nil {
		// Still place the new stop: a surplus owned order is harmless, an
		// unprotected position is not.
		logger.Errorf("  ⚠ Protective cancel for %s %s not confirmed: %v", symbol, side, cancelErr)
	}
	setErr := at.trader.SetStopLoss(symbol, positionSide, quantity, stopPrice)
	if setErr == nil {
		if cancelErr != nil {
			return false, fmt.Errorf("%w for %s %s: %v", errProtectionCancelUnconfirmed, symbol, side, cancelErr)
		}
		return false, nil
	}
	if chk, ok := at.trader.(protectiveStopChecker); ok {
		open, chkErr := chk.ProtectiveStopOpen(symbol, positionSide)
		if chkErr == nil && open {
			return false, fmt.Errorf("stop replacement failed for %s %s (%v); an existing stop still protects the position", symbol, side, setErr)
		}
		if chkErr != nil {
			logger.Errorf("  ⚠ Protection state for %s %s unknown: %v", symbol, side, chkErr)
		}
	}
	protectionErr := fmt.Errorf("protective stop replacement failed for %s %s: %w", symbol, side, setErr)
	logger.Errorf("  🚨 %v; emergency-closing unprotected remainder", protectionErr)
	if closeErr := at.emergencyClosePositionAndVerify(symbol, side, quantity); closeErr != nil {
		at.raiseNakedPositionAlert(symbol, side, fmt.Sprintf("%v; emergency close failed: %v", protectionErr, closeErr)) // AUD-C5
		return false, fmt.Errorf("%w; %w, position may be unprotected: %v", protectionErr, errNakedEmergencyCloseFailed, closeErr)
	}
	return true, fmt.Errorf("%w (%v)", errProtectionEmergencyClosed, protectionErr)
}

// reprotectAfterPartial re-protects the remainder after a protector partial
// close and reports whether the remainder had to be emergency-closed.
func (at *AutoTrader) reprotectAfterPartial(symbol, side string, remainingQty, entryPrice, buffer float64, action protectionAction) bool {
	return at.reprotectAfterPartialAt(symbol, side, remainingQty, entryPrice, 0, buffer, action)
}

// reprotectAfterPartialAt places the remainder's stop and then, when the
// position's open take-profit is known and still ahead of the mark, re-places
// that take-profit for the remaining quantity (F31: the rebuild cancels the
// side's take-profit). A failed take-profit is logged, not fatal: the stop
// protects the remainder, as at open.
func (at *AutoTrader) reprotectAfterPartialAt(symbol, side string, remainingQty, entryPrice, markPrice, buffer float64, action protectionAction) bool {
	closedAll, err := at.rebuildProtectionStopsWithBuffer(symbol, side, remainingQty, entryPrice, buffer)
	if err != nil {
		logger.Errorf("⚠ Protection rebuild after %s close (%s %s): %v", action, symbol, side, err)
	}
	if closedAll || remainingQty <= 0 || (err != nil && !errors.Is(err, errProtectionCancelUnconfirmed)) {
		return closedAll
	}
	state := at.protectionStateFor(symbol, side)
	if state == nil || state.ExchangeTakeProfit <= 0 {
		return false
	}
	tp := state.ExchangeTakeProfit
	if markPrice > 0 && ((side == "long" && tp <= markPrice) || (side == "short" && tp >= markPrice)) {
		logger.Infof("  ℹ Take-profit %.8f not re-placed for %s %s: mark %.8f is already beyond it", tp, symbol, side, markPrice)
		return false
	}
	if tpErr := at.trader.SetTakeProfit(symbol, protectionPositionSide(side), remainingQty, tp); tpErr != nil {
		logger.Errorf("  ⚠ Take-profit re-place after %s close failed for %s %s (stop is in place): %v", action, symbol, side, tpErr)
	}
	return false
}

// replaceDynamicStop is the dynamic-stop form of replaceProtectiveStop.
// placed reports whether the new stop is on the exchange, so the caller may
// record it in the protection state.
func (at *AutoTrader) replaceDynamicStop(symbol, side string, quantity, stopPrice float64) (placed bool, err error) {
	_, err = at.replaceProtectiveStop(symbol, side, quantity, stopPrice, true)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errProtectionCancelUnconfirmed):
		logger.Errorf("⚠ %v", err)
		return true, nil
	default:
		return false, err
	}
}
