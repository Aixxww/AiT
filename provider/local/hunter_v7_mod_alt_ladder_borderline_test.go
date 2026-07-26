package local

import "testing"

// altLadderBorderlineTestContext clears every altLadderMomentumLongModule Match
// precondition (mid stage: 24h +8%, structure above VWAP, OI + volume flow
// votes) so tests only vary the taker reading.
func altLadderBorderlineTestContext(takerBuy15m float64) *V7SymbolContext {
	return &V7SymbolContext{
		Symbol:         "BANKUSDT",
		CurrentPrice:   0.38,
		Change24h:      8,
		Change1h:       2.0,
		Change4h:       5,
		VWAP15m:        0.375,
		VolumeBurst15m: 1.3,
		TakerBuy15m:    takerBuy15m,
		RSI1h:          60,
		Snapshot: &SymbolSnapshotData{
			QuoteVolume24h: 40_000_000,
			OIDelta1h:      5.97,
			OIDelta4h:      2.39,
		},
	}
}

// BANKUSDT 2026-07-26 17:30: taker 0.5588 vs the 0.56 cutoff — 0.0012 short,
// flow tag lost, signal held out of the executable path, then hit TP0. A
// reading inside sampling noise of the cutoff must keep the flow tag with a
// borderline marker instead of being discarded.
func TestAltLadderTakerBuyBorderlineBand(t *testing.T) {
	module := &altLadderMomentumLongModule{}
	cases := []struct {
		name           string
		taker          float64
		wantFlowCode   bool
		wantBorderline bool
	}{
		{"above_cutoff_full_conviction", 0.5620, true, false},
		{"bank_borderline_keeps_flow_tag", 0.5588, true, true},
		{"at_buffer_floor", 0.5500, true, true},
		{"below_buffer_discarded", 0.5480, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig := module.Score(altLadderBorderlineTestContext(tc.taker), V7RegimeTrendUp)
			if sig == nil {
				t.Fatal("expected alt_ladder_momentum_long signal from fixture")
			}
			if got := containsTagValue(sig.ReasonCodes, "alt_ladder_taker_buy"); got != tc.wantFlowCode {
				t.Fatalf("alt_ladder_taker_buy = %v, want %v (codes=%v)", got, tc.wantFlowCode, sig.ReasonCodes)
			}
			if got := containsTagValue(sig.RiskTags, "taker_buy_borderline"); got != tc.wantBorderline {
				t.Fatalf("taker_buy_borderline = %v, want %v (tags=%v)", got, tc.wantBorderline, sig.RiskTags)
			}
		})
	}
}
