package trader

import (
	"fmt"
	"time"
)

// protectorSimVenue is the simulated exchange of the protector walker and
// of the walker/live equivalence test: one position, one resting stop and
// one resting take-profit (each closes the whole side, as Binance
// ClosePosition algo orders do), side-scoped owned cancels (F27) and a
// stop check (F28).
type protectorSimVenue struct {
	symbol, side string
	entry        float64
	qty          float64
	leverage     int
	stop, tp     float64
	origStop     float64
	mark         float64
	why          string
	legs         []protectorWalkLeg
	moves        int
	orders       []string
}

func newProtectorSimVenue(symbol, side string, entry, qty float64, leverage int, stop, tp float64) *protectorSimVenue {
	return &protectorSimVenue{symbol: symbol, side: side, entry: entry, qty: qty, leverage: leverage, stop: stop, tp: tp, origStop: stop}
}

func (v *protectorSimVenue) log(format string, a ...any) {
	v.orders = append(v.orders, fmt.Sprintf(format, a...))
}

func (v *protectorSimVenue) closeAt(q float64) {
	if q <= 0 || q > v.qty {
		q = v.qty
	}
	v.legs = append(v.legs, protectorWalkLeg{q, v.mark, v.why})
	v.log("close %.10g@%.10g", q, v.mark)
	v.qty -= q
	if v.qty < 1e-12 {
		v.qty = 0
	}
}

// fillResting fills a resting order the bar trades through (stop first).
func (v *protectorSimVenue) fillResting(b protectorWalkBar, long bool) (string, bool) {
	adv, fav := b.Low, b.High
	if !long {
		adv, fav = b.High, b.Low
	}
	hit := func(px, lvl float64, favourable bool) bool {
		if favourable == long {
			return px >= lvl
		}
		return px <= lvl
	}
	if v.stop > 0 && hit(adv, v.stop, false) {
		v.mark, v.why = v.stop, "stop"
		if v.stop != v.origStop {
			v.why = "moved_stop"
		}
		why := v.why
		v.closeAt(0)
		return why, true
	}
	if v.tp > 0 && hit(fav, v.tp, true) {
		v.mark, v.why = v.tp, "exchange_tp"
		v.closeAt(0)
		return "exchange_tp", true
	}
	return "", false
}

// Trader interface.

func (v *protectorSimVenue) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// GetPositions reports the open position at the current mark (the live
// adapter reads it; the walker does not).
func (v *protectorSimVenue) GetPositions() ([]map[string]interface{}, error) {
	if v.qty <= 0 {
		return nil, nil
	}
	amt := v.qty
	if v.side == "short" {
		amt = -amt
	}
	return []map[string]interface{}{{
		"symbol": v.symbol, "side": v.side, "entryPrice": v.entry, "markPrice": v.mark,
		"positionAmt": amt, "leverage": float64(v.leverage),
	}}, nil
}
func (v *protectorSimVenue) OpenLong(string, float64, int) (map[string]interface{}, error) {
	return nil, fmt.Errorf("sim venue: no opens")
}
func (v *protectorSimVenue) OpenShort(string, float64, int) (map[string]interface{}, error) {
	return nil, fmt.Errorf("sim venue: no opens")
}
func (v *protectorSimVenue) CloseLong(_ string, q float64) (map[string]interface{}, error) {
	v.closeAt(q)
	return map[string]interface{}{"orderId": int64(1)}, nil
}
func (v *protectorSimVenue) CloseShort(_ string, q float64) (map[string]interface{}, error) {
	v.closeAt(q)
	return map[string]interface{}{"orderId": int64(1)}, nil
}
func (v *protectorSimVenue) SetLeverage(string, int) error          { return nil }
func (v *protectorSimVenue) SetMarginMode(string, bool) error       { return nil }
func (v *protectorSimVenue) GetMarketPrice(string) (float64, error) { return v.mark, nil }
func (v *protectorSimVenue) SetStopLoss(_, _ string, q, p float64) error {
	if p != v.stop {
		v.moves++
	}
	v.stop = p
	v.log("stop %.10g", p)
	return nil
}
func (v *protectorSimVenue) SetTakeProfit(_, _ string, q, p float64) error {
	v.tp = p
	v.log("tp %.10g", p)
	return nil
}
func (v *protectorSimVenue) CancelStopLossOrders(string) error {
	v.stop = 0
	v.log("cancel stops")
	return nil
}
func (v *protectorSimVenue) CancelTakeProfitOrders(string) error {
	v.tp = 0
	v.log("cancel tp")
	return nil
}
func (v *protectorSimVenue) CancelAllOrders(string) error {
	v.stop, v.tp = 0, 0
	v.log("cancel all")
	return nil
}
func (v *protectorSimVenue) CancelStopOrders(string) error {
	v.stop, v.tp = 0, 0
	v.log("cancel stop+tp")
	return nil
}
func (v *protectorSimVenue) FormatQuantity(_ string, q float64) (string, error) {
	return fmt.Sprintf("%.8f", q), nil
}
func (v *protectorSimVenue) GetOrderStatus(string, string) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "FILLED"}, nil
}
func (v *protectorSimVenue) GetClosedPnL(time.Time, int) ([]ClosedPnLRecord, error) { return nil, nil }

// GetOpenOrders reports the resting stop and take-profit (#93: the D5
// take-profit check reads them).
func (v *protectorSimVenue) GetOpenOrders(string) ([]OpenOrder, error) {
	ps := "LONG"
	if v.side == "short" {
		ps = "SHORT"
	}
	var out []OpenOrder
	if v.stop > 0 {
		out = append(out, OpenOrder{Symbol: v.symbol, PositionSide: ps, Type: "STOP_MARKET", StopPrice: v.stop, Quantity: v.qty})
	}
	if v.tp > 0 {
		out = append(out, OpenOrder{Symbol: v.symbol, PositionSide: ps, Type: "TAKE_PROFIT_MARKET", StopPrice: v.tp, Quantity: v.qty})
	}
	return out, nil
}

// CancelOwnedProtectionOrders is the F27 side-scoped owned cancel.
func (v *protectorSimVenue) CancelOwnedProtectionOrders(_, _ string, stopsOnly bool) error {
	v.stop = 0
	if !stopsOnly {
		v.tp = 0
		v.log("cancel side stop+tp")
	} else {
		v.log("cancel side stops")
	}
	return nil
}

// ProtectiveStopOpen is the F28 stop check.
func (v *protectorSimVenue) ProtectiveStopOpen(string, string) (bool, error) { return v.stop > 0, nil }
