package kernel

import (
	"sort"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
)

// #136 (naming F-05): every live-verifiable confirmation code has one shared
// predicate; scoring (EvaluateV7Confirmations) and the kernel live verifier
// must agree on the same facts, including the boundary fixtures the audit
// named.

type confirm136Fixture struct {
	name      string
	direction string
	setup     string
	price     float64
	zone      local.V7PriceZone
	taker     float64
	oiDelta   float64
	closes15m []float64 // 15m bar closes (oldest first); volume 10 each unless volumes set
	volumes   []float64
}

func confirm136Bars(closes, volumes []float64) ([]market.KlineBar, []local.V7Bar) {
	mk := make([]market.KlineBar, len(closes))
	lb := make([]local.V7Bar, len(closes))
	for i, c := range closes {
		v := 10.0
		if i < len(volumes) {
			v = volumes[i]
		}
		mk[i] = market.KlineBar{Open: c, High: c, Low: c, Close: c, Volume: v}
		lb[i] = local.V7Bar{High: c, Low: c, Close: c, Volume: v}
	}
	return mk, lb
}

func confirm136Flat(n int, level float64, last float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = level
	}
	out[n-1] = last
	return out
}

// scoring and live verdicts for one code on one fixture.
func confirm136Verdicts(t *testing.T, code string, fx confirm136Fixture) (scoring, live, liveKnown bool) {
	t.Helper()
	mk, lb := confirm136Bars(fx.closes15m, fx.volumes)
	volumes := make([]float64, len(lb))
	for i, b := range lb {
		volumes[i] = b.Volume
	}
	tf15, ok := local.V7TimeframeFactsFromBars("15m", lb, 0)
	if !ok {
		t.Fatalf("%s: no 15m facts", fx.name)
	}
	dir := local.V7DirLong
	if fx.direction == "SHORT" {
		dir = local.V7DirShort
	}
	sig := &local.V7SignalOutput{
		Symbol:           "CONFUSDT",
		Direction:        dir,
		SetupType:        local.V7SetupType(fx.setup),
		EntryZone:        fx.zone,
		RequiredConfirms: []string{code},
	}
	ctx := &local.V7SymbolContext{
		CurrentPrice:     fx.price,
		TakerBuy15m:      fx.taker,
		VolumeBurst15m:   local.V7VolumeBurst(volumes, local.V7VolumeBurst15mLookback),
		Snapshot:         &local.SymbolSnapshotData{OIDelta1h: fx.oiDelta},
		ExecutionContext: &local.V7ExecutionContext{Timeframes: map[string]local.V7ExecutionTimeframeSummary{"15m": tf15}},
	}
	summary := local.EvaluateV7Confirmations(sig, ctx, local.DefaultV7Config())
	scoring = true
	for _, c := range append(append([]local.V7ConfirmationCheck{}, summary.MissingHard...), summary.MissingReview...) {
		if c.Code == code {
			scoring = false
		}
	}
	for _, c := range summary.ContextChecks {
		if c.Code == code && !c.Passed {
			scoring = false
		}
	}

	coin := CandidateCoin{
		Symbol:           "CONFUSDT",
		Direction:        fx.direction,
		V7SetupType:      fx.setup,
		V7EntryZone:      fx.zone,
		V7PriceContext:   &local.V7PriceContext{Last: fx.price},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: fx.taker, OIChange1h: fx.oiDelta},
	}
	data := &market.Data{
		Symbol:        "CONFUSDT",
		CurrentPrice:  fx.price,
		TimeframeData: map[string]*market.TimeframeSeriesData{"15m": {Timeframe: "15m", Klines: mk}},
	}
	live, liveKnown = hunterV7VerifyLiveConfirmation(coin, data, code)
	return scoring, live, liveKnown
}

func TestConfirm136ScoringAndLiveAgreeOnBoundaries(t *testing.T) {
	flat := confirm136Flat(25, 100, 100)
	cases := []struct {
		code string
		fx   confirm136Fixture
		want bool
	}{
		// Audit case: whale_flow_reversal needs 0.56 on both sides.
		{"taker_flow_confirms_long", confirm136Fixture{name: "whale_0.53", direction: "LONG", setup: "whale_flow_reversal", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.53, closes15m: flat}, false},
		{"taker_flow_confirms_long", confirm136Fixture{name: "whale_0.56", direction: "LONG", setup: "whale_flow_reversal", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.56, closes15m: flat}, true},
		{"taker_flow_confirms_long", confirm136Fixture{name: "other_0.52", direction: "LONG", setup: "leader_momentum_long", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.52, closes15m: flat}, true},
		{"taker_flow_confirms_long", confirm136Fixture{name: "other_0.519", direction: "LONG", setup: "leader_momentum_long", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.519, closes15m: flat}, false},
		{"taker_flow_confirms_short", confirm136Fixture{name: "short_0.48", direction: "SHORT", setup: "distribution_short", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.48, closes15m: flat}, true},
		{"taker_buy_15m_lt_0_45", confirm136Fixture{name: "0.451", direction: "SHORT", setup: "funding_reversal", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.451, closes15m: flat}, false},
		{"taker_flow_not_flipping_against_direction", confirm136Fixture{name: "short_0.50", direction: "SHORT", setup: "distribution_short", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.50, closes15m: flat}, true},

		// Audit case: the volume leg passes without positive OI.
		{"oi_delta_1h_positive_or_quote_volume_expands", confirm136Fixture{name: "oi_neg_volume_burst", direction: "LONG", setup: "displacement_long", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.55, oiDelta: -1, closes15m: flat,
			volumes: append(confirm136Flat(24, 10, 10), 13)}, true},
		{"oi_delta_1h_positive_or_quote_volume_expands", confirm136Fixture{name: "oi_neg_no_burst", direction: "LONG", setup: "displacement_long", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.55, oiDelta: -1, closes15m: flat,
			volumes: append(confirm136Flat(24, 10, 10), 11)}, false},
		{"oi_delta_1h_positive_or_quote_volume_expands", confirm136Fixture{name: "oi_pos", direction: "LONG", setup: "displacement_long", price: 100, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.55, oiDelta: 0.5, closes15m: flat}, true},

		// Audit case: directional 15m close accepts the zone-mid leg on both
		// sides (close below EMA20/VWAP20 but above the zone midpoint).
		{"directional_15m_close_long", confirm136Fixture{name: "zone_mid_leg", direction: "LONG", setup: "pullback_reversal_long", price: 99.5, zone: local.V7PriceZone{Lower: 98, Upper: 99}, taker: 0.55, closes15m: confirm136Flat(25, 100, 99.5)}, true},
		{"directional_15m_close_long", confirm136Fixture{name: "no_leg", direction: "LONG", setup: "pullback_reversal_long", price: 99.5, zone: local.V7PriceZone{Lower: 100, Upper: 102}, taker: 0.55, closes15m: confirm136Flat(25, 100, 99.5)}, false},
		{"directional_15m_close_short", confirm136Fixture{name: "short_below_ema", direction: "SHORT", setup: "distribution_short", price: 99.5, zone: local.V7PriceZone{Lower: 100, Upper: 102}, taker: 0.45, closes15m: confirm136Flat(25, 100, 99.5)}, true},

		// Trigger codes now have a live verifier with the scoring rule.
		{"5m_or_15m_close_above_trigger", confirm136Fixture{name: "at_upper", direction: "LONG", setup: "squeeze_breakout_long", price: 101, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.55, closes15m: flat}, true},
		{"5m_or_15m_close_below_trigger", confirm136Fixture{name: "above_lower", direction: "SHORT", setup: "breakdown_momentum_short", price: 99.01, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.45, closes15m: flat}, false},
		{"live_price_in_entry_zone", confirm136Fixture{name: "at_lower_edge", direction: "LONG", setup: "leader_momentum_long", price: 99, zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: 0.55, closes15m: flat}, true},
	}
	for _, tc := range cases {
		t.Run(tc.code+"/"+tc.fx.name, func(t *testing.T) {
			scoring, live, known := confirm136Verdicts(t, tc.code, tc.fx)
			if !known {
				t.Fatalf("live verifier reports unknown for a fixture with all inputs")
			}
			if scoring != live {
				t.Fatalf("scoring=%v live=%v: the two layers disagree", scoring, live)
			}
			if scoring != tc.want {
				t.Fatalf("verdict=%v, want %v", scoring, tc.want)
			}
		})
	}
}

// Sweep: on a spread of fixtures, every live-verifiable code gets the same
// verdict at scoring time and live, and live never passes what scoring fails.
func TestConfirm136EveryLiveVerifiableCodeAgrees(t *testing.T) {
	codes := local.V7ConfirmLiveVerifiableCodes()
	sort.Strings(codes)
	if len(codes) < 14 {
		t.Fatalf("only %d live-verifiable codes; the shared table looks broken", len(codes))
	}
	var fixtures []confirm136Fixture
	for _, dir := range []string{"LONG", "SHORT"} {
		for _, setup := range []string{"whale_flow_reversal", "leader_momentum_long", "distribution_short"} {
			for _, taker := range []float64{0.44, 0.45, 0.48, 0.50, 0.52, 0.53, 0.56, 0.60} {
				for _, price := range []float64{98.5, 99, 100, 101, 101.5} {
					fixtures = append(fixtures, confirm136Fixture{
						name: dir + "/" + setup, direction: dir, setup: setup, price: price,
						zone: local.V7PriceZone{Lower: 99, Upper: 101}, taker: taker, oiDelta: -0.2,
						closes15m: confirm136Flat(25, 100, price), volumes: append(confirm136Flat(24, 10, 10), 12),
					})
				}
			}
		}
	}
	for _, code := range codes {
		for _, fx := range fixtures {
			scoring, live, known := confirm136Verdicts(t, code, fx)
			if known && live && !scoring {
				t.Fatalf("%s %s taker=%.2f price=%.1f: live passes what scoring fails", code, fx.name, fx.taker, fx.price)
			}
			if known && live != scoring {
				t.Fatalf("%s %s taker=%.2f price=%.1f: scoring=%v live=%v", code, fx.name, fx.taker, fx.price, scoring, live)
			}
		}
	}
}
