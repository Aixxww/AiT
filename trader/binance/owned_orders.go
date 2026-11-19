package binance

import (
	"context"
	"fmt"
	"strings"

	"github.com/Aixxww/AiT/logger"
	"github.com/adshao/go-binance/v2/futures"
)

// F26: cancel only this bot's own orders, and only on one position side.
//
// CancelAllOrders(symbol) cancels every regular and algo order on the
// symbol, for both position sides and for orders the bot did not place.
// It ran before every open and after every close, so in hedge mode opening
// a short removed the protection of an existing long, and it also removed
// manual orders. CancelOwnedOrders limits the cancel to orders whose client
// id carries the bot's prefix (every order placed by this package does:
// newClientOrderID and ClientOrderIDForIntent) and whose position side matches
// (one-way mode reports "BOTH", which matches either side), then re-reads
// the open orders and reports any owned same-side order that survived.
//
// F27 adds CancelOwnedProtectionOrders, the same owned/side-scoped cancel
// restricted to protective order types, for the position protector's stop
// rebuilds (which used the symbol-wide CancelStopOrders /
// CancelStopLossOrders). F28 adds ProtectiveStopOpen, which the protector
// uses to confirm a position is still protected after a failed rebuild.

// Ownership is decided by isOwnedClientID (client_ids.go): the OD-19
// ownership prefix, with or without a referral part.
func ownedSideMatch(clientID string, orderSide futures.PositionSideType, want string) bool {
	if !isOwnedClientID(clientID) {
		return false
	}
	return sideMatch(orderSide, want)
}

// sideMatch: one-way mode reports "BOTH" (or nothing), which matches either side.
func sideMatch(orderSide futures.PositionSideType, want string) bool {
	s := strings.ToUpper(string(orderSide))
	return s == "BOTH" || s == "" || s == strings.ToUpper(want)
}

// Protective order types. Regular (legacy) orders report their type as a
// string; algo orders as futures.AlgoOrderType.
var (
	regularStopTypes       = map[string]bool{"STOP_MARKET": true, "STOP": true}
	regularProtectiveTypes = map[string]bool{"STOP_MARKET": true, "STOP": true, "TAKE_PROFIT_MARKET": true, "TAKE_PROFIT": true}
	algoStopTypes          = map[futures.AlgoOrderType]bool{futures.AlgoOrderTypeStopMarket: true, futures.AlgoOrderTypeStop: true}
	algoProtectiveTypes    = map[futures.AlgoOrderType]bool{futures.AlgoOrderTypeStopMarket: true, futures.AlgoOrderTypeStop: true, futures.AlgoOrderTypeTakeProfitMarket: true, futures.AlgoOrderTypeTakeProfit: true, futures.AlgoOrderTypeTrailingStopMarket: true}
)

// ownedOrderFilter selects owned orders on one position side, optionally
// restricted to a set of order types (nil = every type).
type ownedOrderFilter struct {
	positionSide string
	regularTypes map[string]bool
	algoTypes    map[futures.AlgoOrderType]bool
}

func (f ownedOrderFilter) regular(o *futures.Order) bool {
	if !ownedSideMatch(o.ClientOrderID, o.PositionSide, f.positionSide) {
		return false
	}
	return f.regularTypes == nil || f.regularTypes[string(o.Type)]
}

func (f ownedOrderFilter) algo(a futures.GetAlgoOrderResp) bool {
	if !ownedSideMatch(a.ClientAlgoId, a.PositionSide, f.positionSide) {
		return false
	}
	return f.algoTypes == nil || f.algoTypes[a.OrderType]
}

// CancelOwnedOrders cancels this bot's open regular and algo orders for
// symbol on positionSide ("LONG" or "SHORT") and confirms by re-reading.
// A non-nil error means the cancel could not be confirmed: either a list
// call failed (state unknown) or owned same-side orders remain.
func (t *FuturesTrader) CancelOwnedOrders(symbol, positionSide string) error {
	return t.cancelOwnedMatching(symbol, ownedOrderFilter{positionSide: positionSide}, "owned")
}

// CancelOwnedProtectionOrders (F27) cancels this bot's protective orders on
// one position side and confirms by re-reading. stopsOnly limits it to stop
// orders (the dynamic stop update keeps the take-profit); otherwise stop,
// take-profit and trailing orders are cancelled, which is what the
// symbol-wide CancelStopOrders did for the managed side. The opposite
// side's orders and orders without the bot's prefix are never touched.
func (t *FuturesTrader) CancelOwnedProtectionOrders(symbol, positionSide string, stopsOnly bool) error {
	f := ownedOrderFilter{positionSide: positionSide, regularTypes: regularProtectiveTypes, algoTypes: algoProtectiveTypes}
	label := "owned protective"
	if stopsOnly {
		f.regularTypes, f.algoTypes = regularStopTypes, algoStopTypes
		label = "owned stop"
	}
	return t.cancelOwnedMatching(symbol, f, label)
}

// ProtectiveStopOpen (F28) reports whether any stop order (regular or algo,
// owned or not) is open for symbol on positionSide. A list failure returns
// an error: the protection state is then unknown.
func (t *FuturesTrader) ProtectiveStopOpen(symbol, positionSide string) (bool, error) {
	orders, err := t.client.NewListOpenOrdersService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return false, fmt.Errorf("list open orders %s: %w", symbol, err)
	}
	for _, o := range orders {
		if regularStopTypes[string(o.Type)] && sideMatch(o.PositionSide, positionSide) {
			return true, nil
		}
	}
	algos, err := t.client.NewListOpenAlgoOrdersService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return false, fmt.Errorf("list open algo orders %s: %w", symbol, err)
	}
	for _, a := range algos {
		if algoStopTypes[a.OrderType] && sideMatch(a.PositionSide, positionSide) {
			return true, nil
		}
	}
	return false, nil
}

func (t *FuturesTrader) cancelOwnedMatching(symbol string, f ownedOrderFilter, label string) error {
	canceled, err := t.cancelOwnedPass(symbol, f)
	if err != nil {
		return err
	}
	remaining, err := t.ownedOpenOrderCount(symbol, f)
	if err != nil {
		return fmt.Errorf("%s-order cancel for %s %s not confirmed: %w", label, symbol, f.positionSide, err)
	}
	if remaining > 0 {
		return fmt.Errorf("%s-order cancel for %s %s not confirmed: %d %s order(s) still open", label, symbol, f.positionSide, remaining, label)
	}
	if canceled > 0 {
		logger.Infof("  ✓ Canceled %d %s %s order(s) for %s (confirmed)", canceled, label, f.positionSide, symbol)
	}
	return nil
}

func (t *FuturesTrader) cancelOwnedPass(symbol string, f ownedOrderFilter) (int, error) {
	orders, err := t.client.NewListOpenOrdersService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("list open orders %s: %w", symbol, err)
	}
	algos, err := t.client.NewListOpenAlgoOrdersService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("list open algo orders %s: %w", symbol, err)
	}
	canceled := 0
	for _, o := range orders {
		if !f.regular(o) {
			continue
		}
		if _, err := t.client.NewCancelOrderService().Symbol(symbol).OrderID(o.OrderID).Do(context.Background()); err != nil {
			logger.Infof("  ⚠ Failed to cancel owned order %d (%s): %v", o.OrderID, o.ClientOrderID, err)
			continue // the re-read decides
		}
		canceled++
	}
	for _, a := range algos {
		if !f.algo(a) {
			continue
		}
		if _, err := t.client.NewCancelAlgoOrderService().AlgoID(a.AlgoId).Do(context.Background()); err != nil {
			logger.Infof("  ⚠ Failed to cancel owned algo order %d (%s): %v", a.AlgoId, a.ClientAlgoId, err)
			continue
		}
		canceled++
	}
	return canceled, nil
}

func (t *FuturesTrader) ownedOpenOrderCount(symbol string, f ownedOrderFilter) (int, error) {
	orders, err := t.client.NewListOpenOrdersService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, err
	}
	algos, err := t.client.NewListOpenAlgoOrdersService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, o := range orders {
		if f.regular(o) {
			n++
		}
	}
	for _, a := range algos {
		if f.algo(a) {
			n++
		}
	}
	return n, nil
}
