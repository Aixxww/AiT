package trader

import (
	"fmt"
	"strings"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
)

// F04: idempotent market entries.
//
// Before this change the exchange client generated a random client order id
// inside OpenLong/OpenShort, so after a timeout the caller could not tell
// whether the order existed. Exchanges that implement clientIDOrderTrader
// now receive a deterministic id derived from the decision intent, and an
// ambiguous submission error is resolved by reading the order back by that
// id. Design reference: Nofx trader/execution (SubmitOrder/LookupOrder,
// "never resubmit an unconfirmed order"); same AGPL-3.0 lineage.

// clientIDOrderTrader is implemented by exchange clients that accept a
// caller-chosen client order id (currently Binance futures).
type clientIDOrderTrader interface {
	OpenLongWithClientID(symbol string, quantity float64, leverage int, clientOrderID string) (map[string]interface{}, error)
	OpenShortWithClientID(symbol string, quantity float64, leverage int, clientOrderID string) (map[string]interface{}, error)
	LookupOrderByClientID(symbol, clientOrderID string) (map[string]interface{}, bool, error)
	IsAmbiguousOrderError(err error) bool
	ClientOrderIDForIntent(intentKey string) string
}

const (
	// openReadBackAttempts is the number of reads used to resolve an
	// ambiguous submission; absence must be seen on every read before the
	// order is treated as not placed (two-read rule, as in the t2exec
	// protective-order fix).
	openReadBackAttempts = 2
	openReadBackDelay    = 1500 * time.Millisecond
)

// openIntentKey identifies one open decision: trader, cycle, symbol, side
// and the selected Hunter v7 signal. A later cycle is a new intent.
func (at *AutoTrader) openIntentKey(decision *kernel.Decision, side string) string {
	if decision.HunterV7IntentKey != "" {
		return decision.HunterV7IntentKey // U9: pending conditional entry
	}
	return fmt.Sprintf("%s|%d|%s|%s|%s", at.id, at.callCount, strings.ToUpper(decision.Symbol), side, decision.SelectedHunterV7SignalID)
}

// openMarketPosition opens the position. With a clientIDOrderTrader it uses
// a deterministic client id and resolves ambiguous errors by read-back:
//   - found (live or filled)          -> adopted, returned as success;
//   - found but CANCELED/EXPIRED/REJECTED -> the original error;
//   - absent on every read             -> the original error (not placed);
//   - any read failed                  -> an order_state_unknown error. The
//     order is never resubmitted; the next cycle's same-symbol position
//     check prevents a duplicate entry.
func (at *AutoTrader) openMarketPosition(decision *kernel.Decision, side string, quantity float64) (map[string]interface{}, error) {
	ct, ok := at.trader.(clientIDOrderTrader)
	if !ok {
		if side == "long" {
			return at.trader.OpenLong(decision.Symbol, quantity, decision.Leverage)
		}
		return at.trader.OpenShort(decision.Symbol, quantity, decision.Leverage)
	}
	cid := ct.ClientOrderIDForIntent(at.openIntentKey(decision, side))
	var order map[string]interface{}
	var err error
	if side == "long" {
		order, err = ct.OpenLongWithClientID(decision.Symbol, quantity, decision.Leverage, cid)
	} else {
		order, err = ct.OpenShortWithClientID(decision.Symbol, quantity, decision.Leverage, cid)
	}
	if err == nil || !ct.IsAmbiguousOrderError(err) {
		return order, err
	}
	logger.Warnf("  ⚠ %s %s open result ambiguous (%v); reading back client id %s", decision.Symbol, side, err, cid)
	sleep := at.readBackSleep
	if sleep == nil {
		sleep = time.Sleep
	}
	absent, unknown := 0, 0
	for i := 0; i < openReadBackAttempts; i++ {
		sleep(openReadBackDelay)
		got, found, lerr := ct.LookupOrderByClientID(decision.Symbol, cid)
		switch {
		case lerr != nil:
			unknown++
		case !found:
			absent++
		default:
			status := strings.ToUpper(fmt.Sprint(got["status"]))
			if status == "CANCELED" || status == "CANCELLED" || status == "EXPIRED" || status == "REJECTED" {
				return nil, fmt.Errorf("%w; read-back: order %s is %s", err, cid, status)
			}
			got["adopted"] = true
			logger.Infof("  ✓ %s %s order %s found on read-back (status %s); adopted, not resubmitted", decision.Symbol, side, cid, status)
			return got, nil
		}
	}
	if absent == openReadBackAttempts {
		return nil, fmt.Errorf("%w; read-back: order %s absent on %d reads, not placed", err, cid, absent)
	}
	return nil, fmt.Errorf("%w; read-back inconclusive for %s (absent %d, unknown %d): order_state_unknown, not resubmitted", err, cid, absent, unknown)
}
