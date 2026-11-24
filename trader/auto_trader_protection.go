package trader

import (
	"fmt"
	"strings"

	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
)

// F03: a position opened without a confirmed protective stop is closed.
//
// Provenance: ported from upstream Nofx (NoFxAiOS/nofx @ 65e9037,
// trader/auto_trader_signal_policy.go: closeUnprotectedPosition,
// emergencyClosePositionAndVerify, getFreshPositions). Same AGPL-3.0
// lineage. Adaptation: the upstream post-close CancelAllOrders(symbol) +
// "zero open orders" check is NOT ported, because CancelAllOrders is
// symbol-wide and in hedge mode would also cancel the opposite side's
// protection (audit F26). On this path no protective order was placed for
// the new position (the stop failed and the take-profit is skipped), so
// there is nothing of ours to clean up.

// emergencyCloseAttempts bounds the close-and-verify loop (upstream: 3).
const emergencyCloseAttempts = 3

// freshPositions reads positions bypassing the trader's position cache.
func (at *AutoTrader) freshPositions() ([]map[string]interface{}, error) {
	at.invalidateTraderPositionCacheForDecision()
	return at.trader.GetPositions()
}

// closeUnprotectedPosition emergency-closes a just-opened position whose
// mandatory stop could not be placed. It always returns an error so the
// open is never reported as successful.
func (at *AutoTrader) closeUnprotectedPosition(symbol, side string, quantity float64, protectionErr error) error {
	logger.Errorf("  🚨 %v; emergency-closing unprotected %s %s", protectionErr, symbol, side)
	if closeErr := at.emergencyClosePositionAndVerify(symbol, side, quantity); closeErr != nil {
		// AUD-C5: never only returned; the alert stays until the per-poll
		// stop audit confirms a stop or the position is gone.
		at.raiseNakedPositionAlert(symbol, side, fmt.Sprintf("%v; emergency close failed: %v", protectionErr, closeErr))
		return fmt.Errorf("%w; %w, position may be unprotected: %v", protectionErr, errNakedEmergencyCloseFailed, closeErr)
	}
	return fmt.Errorf("%w; opened position was emergency-closed", protectionErr)
}

// emergencyClosePositionAndVerify closes the position and confirms with a
// fresh position read that no residual position remains on that side.
func (at *AutoTrader) emergencyClosePositionAndVerify(symbol, side string, quantity float64) error {
	var lastErr error
	for attempt := 1; attempt <= emergencyCloseAttempts; attempt++ {
		var err error
		switch side {
		case "long":
			_, err = at.trader.CloseLong(symbol, quantity)
		case "short":
			_, err = at.trader.CloseShort(symbol, quantity)
		default:
			return fmt.Errorf("unknown position direction: %s", side)
		}
		if err != nil {
			lastErr = err
			continue
		}
		positions, err := at.freshPositions()
		if err != nil {
			lastErr = fmt.Errorf("verify flat position: %w", err)
			continue
		}
		residual := false
		for _, pos := range positions {
			if market.Normalize(fmt.Sprint(pos["symbol"])) == market.Normalize(symbol) && strings.EqualFold(fmt.Sprint(pos["side"]), side) {
				residual = true
				break
			}
		}
		if !residual {
			return nil
		}
		lastErr = fmt.Errorf("residual %s position remains after close attempt %d", side, attempt)
	}
	return lastErr
}
