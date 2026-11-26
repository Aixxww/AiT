package main

import (
	"math"
	"testing"

	"github.com/Aixxww/AiT/provider/local"
)

// #135: a signal's derivatives_context.oi_value (USDT notional) must reach the
// prompt as the same notional: OIData stores contracts, so the tool converts
// OIValue back by price and NotionalUSDT reproduces OIValue exactly.
func TestOIUnit135PromptMarketDataRoundTrip(t *testing.T) {
	signals := []local.V7SignalOutput{{
		Symbol:         "OIUSDT",
		PriceCtx:       &local.V7PriceContext{Last: 500},
		DerivativesCtx: &local.V7DerivativesContext{OIValue: 20_000_000, FundingRate: 0.0001},
	}}
	d := promptMarketData(signals, nil)["OIUSDT"]
	if d == nil || d.OpenInterest == nil {
		t.Fatal("market data / OI missing")
	}
	if math.Abs(d.OpenInterest.Latest-40_000) > 1e-6 {
		t.Fatalf("OIData.Latest = %.3f contracts, want 40000", d.OpenInterest.Latest)
	}
	notional, _, ok := d.OpenInterest.NotionalUSDT(d.CurrentPrice)
	if !ok || math.Abs(notional-20_000_000) > 1e-3 {
		t.Fatalf("notional = %.3f/%v, want the signal's 20000000 oi_value", notional, ok)
	}
}
