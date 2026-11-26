package trader

import (
	"errors"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/store"
)

// quoteOutageFake: the ticker works until SetMarginMode returns, then every
// quote fetch fails (the order book is not offered at all), so the C7 re-read
// has no fresh executable quote.
type quoteOutageFake struct {
	*lifecycleFake
	down bool
}

func (f *quoteOutageFake) SetMarginMode(s string, c bool) error {
	f.down = true
	return f.lifecycleFake.SetMarginMode(s, c)
}

func (f *quoteOutageFake) GetMarketPrice(s string) (float64, error) {
	if f.down {
		return 0, errors.New("ticker unavailable")
	}
	return f.lifecycleFake.GetMarketPrice(s)
}

// #138 M1: with both quote sources down the re-read fails closed: no order,
// no intent row. Before, it fell back to the planned price and passed.
func TestM1138RecheckFailsClosedWithoutFreshQuote(t *testing.T) {
	st := newLifecycleStore(t)
	inner := &lifecycleFake{bookPrice: 100.1, fillPrice: 100.1}
	ft := &quoteOutageFake{lifecycleFake: inner}
	at := newLifecycleTrader(t, inner, st)
	at.trader = ft
	err := at.placeOpenOrderWithLifecycle(lcDecision("AAAUSDT"), "long", 1, 100.1, 100, false, false, &store.DecisionAction{})
	if err == nil || !strings.Contains(err.Error(), "entry_quote_unavailable") {
		t.Fatalf("want a fail-closed quote rejection, got %v", err)
	}
	if inner.opens != 0 || len(lifecycleRows(t, st, "p0")) != 0 {
		t.Fatalf("no order and no intent row may exist: opens=%d", inner.opens)
	}
	// Other callers keep their documented fallback.
	if got := at.resolveOpenExecutionPrice("AAAUSDT", "long", 42); got != 42 {
		t.Fatalf("resolveOpenExecutionPrice fallback = %v, want 42", got)
	}
}
