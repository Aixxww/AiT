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
// getBrOrderID and ClientOrderIDForIntent) and whose position side matches
// (one-way mode reports "BOTH", which matches either side), then re-reads
// the open orders and reports any owned same-side order that survived.

// ownedOrderPrefix is the client-id prefix of every order this package
// places (see getBrOrderID and ClientOrderIDForIntent).
const ownedOrderPrefix = "x-KzrpZaP9"

func ownedSideMatch(clientID string, orderSide futures.PositionSideType, want string) bool {
	if !strings.HasPrefix(clientID, ownedOrderPrefix) {
		return false
	}
	s := strings.ToUpper(string(orderSide))
	return s == "BOTH" || s == "" || s == strings.ToUpper(want)
}

// CancelOwnedOrders cancels this bot's open regular and algo orders for
// symbol on positionSide ("LONG" or "SHORT") and confirms by re-reading.
// A non-nil error means the cancel could not be confirmed: either a list
// call failed (state unknown) or owned same-side orders remain.
func (t *FuturesTrader) CancelOwnedOrders(symbol, positionSide string) error {
	canceled, err := t.cancelOwnedPass(symbol, positionSide)
	if err != nil {
		return err
	}
	remaining, err := t.ownedOpenOrderCount(symbol, positionSide)
	if err != nil {
		return fmt.Errorf("owned-order cancel for %s %s not confirmed: %w", symbol, positionSide, err)
	}
	if remaining > 0 {
		return fmt.Errorf("owned-order cancel for %s %s not confirmed: %d owned order(s) still open", symbol, positionSide, remaining)
	}
	if canceled > 0 {
		logger.Infof("  ✓ Canceled %d owned %s order(s) for %s (confirmed)", canceled, positionSide, symbol)
	}
	return nil
}

func (t *FuturesTrader) cancelOwnedPass(symbol, positionSide string) (int, error) {
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
		if !ownedSideMatch(o.ClientOrderID, o.PositionSide, positionSide) {
			continue
		}
		if _, err := t.client.NewCancelOrderService().Symbol(symbol).OrderID(o.OrderID).Do(context.Background()); err != nil {
			logger.Infof("  ⚠ Failed to cancel owned order %d (%s): %v", o.OrderID, o.ClientOrderID, err)
			continue // the re-read decides
		}
		canceled++
	}
	for _, a := range algos {
		if !ownedSideMatch(a.ClientAlgoId, a.PositionSide, positionSide) {
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

func (t *FuturesTrader) ownedOpenOrderCount(symbol, positionSide string) (int, error) {
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
		if ownedSideMatch(o.ClientOrderID, o.PositionSide, positionSide) {
			n++
		}
	}
	for _, a := range algos {
		if ownedSideMatch(a.ClientAlgoId, a.PositionSide, positionSide) {
			n++
		}
	}
	return n, nil
}
