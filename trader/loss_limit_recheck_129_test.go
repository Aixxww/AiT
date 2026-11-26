package trader

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// #129 / Audit R2 H4: the single-trade loss limit holds on the
// post-preflight price and stop.

func lossRecheckTrader(limitPct float64) *AutoTrader {
	return &AutoTrader{id: "h4", config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
		RiskControl: store.RiskControlConfig{MaxSingleTradeLossPct: limitPct, MinRiskRewardRatio: 1.2, MaxPositions: 1},
	}}}
}

func lossAt(d *kernel.Decision, price float64) float64 {
	return d.PositionSizeUSD * math.Abs(price-d.StopLoss) / price
}

// Audit scenario: 1.0% of 10,000 equity = 100 USDT. Sized at price 100 with
// the stop at 99.5 (0.5%), the position is 20,000. The refresh moves the
// executable price to 100.45 (+0.45%); then a stop of 99.5 is 0.95% away
// (~189 USDT, ~1.9% of equity), and a repaired, wider stop of 99.0 is
// ~1.44% away (~289 USDT).
func TestLossRecheck129_AuditScenario(t *testing.T) {
	const equity, limitUSD = 10000.0, 100.0
	at := lossRecheckTrader(1.0)
	for _, stopAfter := range []float64{99.5, 99.0} {
		d := &kernel.Decision{Symbol: "XUSDT", Action: "open_long", PositionSizeUSD: 30000, StopLoss: 99.5, TakeProfit: 104, Leverage: 5, Confidence: 80}
		size, capped, err := at.enforceSingleTradeLossLimit(d, 100, equity, "long")
		if err != nil || !capped || math.Abs(size-20000) > 1e-6 {
			t.Fatalf("pre-refresh sizing: %v %v %v", size, capped, err)
		}
		d.PositionSizeUSD = size
		// Preflight refresh: price +0.45%, stop repaired.
		const finalPrice = 100.45
		d.StopLoss = stopAfter
		if before := lossAt(d, finalPrice); before <= limitUSD*1.4 {
			t.Fatalf("fixture: without the recheck the loss must exceed the limit, got %.2f", before)
		}
		size, reduced, err := at.recheckRiskAfterPreflight(d, finalPrice, equity, "long")
		if err != nil {
			t.Fatalf("stop %.2f: recheck must resize, not block: %v", stopAfter, err)
		}
		if !reduced || size != d.PositionSizeUSD {
			t.Fatalf("stop %.2f: size %.2f reduced %v", stopAfter, size, reduced)
		}
		if got := lossAt(d, finalPrice); got > limitUSD+1e-6 {
			t.Fatalf("stop %.2f: effective max loss %.2f USDT > limit %.2f", stopAfter, got, limitUSD)
		}
	}
}

// Short mirror.
func TestLossRecheck129_Short(t *testing.T) {
	at := lossRecheckTrader(1.0)
	d := &kernel.Decision{Symbol: "XUSDT", Action: "open_short", PositionSizeUSD: 20000, StopLoss: 100.5, TakeProfit: 96, Leverage: 5, Confidence: 80}
	const finalPrice = 99.55 // refresh moved the bid down 0.45%
	if _, _, err := at.recheckRiskAfterPreflight(d, finalPrice, 10000, "short"); err != nil {
		t.Fatalf("recheck: %v", err)
	}
	if got := lossAt(d, finalPrice); got > 100+1e-6 {
		t.Fatalf("short effective loss %.2f > 100", got)
	}
}

// No move: the recheck changes nothing.
func TestLossRecheck129_UnchangedWhenPreflightDoesNotMove(t *testing.T) {
	at := lossRecheckTrader(1.0)
	d := &kernel.Decision{Symbol: "XUSDT", Action: "open_long", PositionSizeUSD: 30000, StopLoss: 99.5, TakeProfit: 104, Leverage: 5, Confidence: 80}
	size, _, _ := at.enforceSingleTradeLossLimit(d, 100, 10000, "long")
	d.PositionSizeUSD = size
	got, reduced, err := at.recheckRiskAfterPreflight(d, 100, 10000, "long")
	if err != nil || reduced || got != size || d.PositionSizeUSD != size {
		t.Fatalf("no move: size %.4f -> %.4f reduced %v err %v", size, got, reduced, err)
	}
	// No limit configured: no change either.
	free := lossRecheckTrader(0)
	free.config.StrategyConfig.RiskControl.MaxSingleTradeLossPct = 0
	d2 := &kernel.Decision{Symbol: "XUSDT", Action: "open_long", PositionSizeUSD: 500, StopLoss: 99, TakeProfit: 104, Leverage: 5, Confidence: 80}
	if got, reduced, err := free.recheckRiskAfterPreflight(d2, 100.4, 10000, "long"); err != nil || (free.maxSingleTradeLossPct() == 0 && (reduced || got != 500)) {
		t.Fatalf("no limit: %v %v %v (limit %v)", got, reduced, err, free.maxSingleTradeLossPct())
	}
}

// When the re-check cannot be satisfied the open is blocked with a precise
// reason, never silently opened.
func TestLossRecheck129_BlocksWhenUnsatisfiable(t *testing.T) {
	at := lossRecheckTrader(1.0)
	cases := map[string]struct {
		d      kernel.Decision
		price  float64
		equity float64
		want   string
	}{
		// The resized position falls below the minimum position size
		// (1% of 300 equity = 3 USDT at a 50% stop: 6 USDT < 12).
		"below minimum size": {kernel.Decision{Symbol: "XUSDT", Action: "open_long", PositionSizeUSD: 1000, StopLoss: 50, TakeProfit: 200, Leverage: 5, Confidence: 80}, 100, 300, "below minimum"},
		// The refresh moved the price through the stop.
		"price through stop": {kernel.Decision{Symbol: "XUSDT", Action: "open_long", PositionSizeUSD: 1000, StopLoss: 99.5, TakeProfit: 104, Leverage: 5, Confidence: 80}, 99.4, 10000, ""},
		// RR below the floor at the refreshed price.
		"rr below floor": {kernel.Decision{Symbol: "XUSDT", Action: "open_long", PositionSizeUSD: 1000, StopLoss: 99, TakeProfit: 101, Leverage: 5, Confidence: 80}, 100.4, 10000, "Risk-reward"},
	}
	for name, c := range cases {
		d := c.d
		_, _, err := at.recheckRiskAfterPreflight(&d, c.price, c.equity, "long")
		if err == nil || !strings.Contains(err.Error(), "post_preflight_risk_recheck") || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

// Wiring: both open executors run the recheck after the preflight refresh
// and before the quantity is computed. (The executors read live market data
// through market.GetWithExchange, so they are checked at the source level.)
func TestLossRecheck129_WiredAfterPreflightInBothExecutors(t *testing.T) {
	src, err := os.ReadFile("auto_trader_orders.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range []string{"executeOpenLongWithRecord", "executeOpenShortWithRecord"} {
		body := string(src)
		i := strings.Index(body, "func (at *AutoTrader) "+fn+"(")
		if i < 0 {
			t.Fatalf("%s not found", fn)
		}
		body = body[i:]
		if j := strings.Index(body[1:], "\nfunc "); j >= 0 {
			body = body[:j+1]
		}
		refresh := strings.Index(body, "refreshHunterV7OpenPreflight(")
		recheck := strings.Index(body, "recheckRiskAfterPreflight(")
		qty := strings.Index(body, "quantity := actualPositionSize / executionPrice")
		if refresh < 0 || recheck < 0 || qty < 0 || !(refresh < recheck && recheck < qty) {
			t.Fatalf("%s: refresh@%d recheck@%d quantity@%d: recheck must sit between the refresh and the quantity", fn, refresh, recheck, qty)
		}
	}
}
