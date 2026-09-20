package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/provider/local"
)

func TestHunterV7ZoneOIStopGates(t *testing.T) {
	base := CandidateCoin{
		V7SetupType:      "alt_ladder_breakdown_short",
		Direction:        "SHORT",
		V7AIPriority:     58,
		V7SetupScore:     70,
		V7TimingScore:    62,
		V7RiskScore:      20,
		V7LiquidityScore: 80,
		V7ReasonCodes: []string{
			"alt_ladder_breakdown_short",
			"alt_ladder_downshift_mid",
			"alt_ladder_taker_sell",
			"alt_ladder_new_shorts",
		},
		V7EntryZone:      local.V7PriceZone{Lower: 100, Upper: 101},
		V7Invalidation:   local.V7InvalidationRule{Price: 102.1},
		V7PriceContext:   &local.V7PriceContext{Last: 100.3},
		V7DerivativesCtx: &local.V7DerivativesContext{OIChange1h: 0.9, OIChange4h: 0.3, TakerBuy15m: 0.36},
	}

	t.Run("zone require known blocks missing position", func(t *testing.T) {
		coin := base
		coin.V7PriceContext = nil
		coin.V7EntryZone = local.V7PriceZone{}
		if hunterV7ZoneGateMatches(coin, hunterV7ZoneGate{MaxPos: 45, RequireKnown: true}) {
			t.Fatal("expected RequireKnown to fail without zone data")
		}
	})

	t.Run("zone max pos", func(t *testing.T) {
		coin := base
		if !hunterV7ZoneGateMatches(coin, hunterV7ZoneGate{MaxPos: 45, RequireKnown: true}) {
			t.Fatal("expected low-zone coin to pass")
		}
		coin.V7PriceContext = &local.V7PriceContext{Last: 100.9}
		if hunterV7ZoneGateMatches(coin, hunterV7ZoneGate{MaxPos: 45, RequireKnown: true}) {
			t.Fatal("expected high-zone coin to fail")
		}
	})

	t.Run("oi min change", func(t *testing.T) {
		coin := base
		if !hunterV7OIGateMatches(coin, hunterV7OIGate{MinChange1h: 0.8, RequireCtx: true}) {
			t.Fatal("expected OI 0.9 to clear 0.8 floor")
		}
		coin.V7DerivativesCtx = &local.V7DerivativesContext{OIChange1h: 0.5, TakerBuy15m: 0.36}
		if hunterV7OIGateMatches(coin, hunterV7OIGate{MinChange1h: 0.8, RequireCtx: true}) {
			t.Fatal("expected weak OI to fail")
		}
	})

	t.Run("stop allow unknown and max", func(t *testing.T) {
		coin := base
		coin.V7Invalidation = local.V7InvalidationRule{}
		if !hunterV7StopGateMatches(coin, hunterV7StopGate{MaxPct: 2.25, AllowUnknown: true}) {
			t.Fatal("unknown stop should pass when AllowUnknown")
		}
		// ~3.7% stop — fails both standard and strong caps
		coin.V7Invalidation = local.V7InvalidationRule{Price: 104}
		if hunterV7StopGateMatches(coin, hunterV7StopGate{MaxPct: 2.25, AllowUnknown: true}) {
			t.Fatal("wide stop should fail standard max")
		}
		if hunterV7StopGateMatches(coin, hunterV7StopGate{MaxPct: 2.45, AllowUnknown: true}) {
			t.Fatal("very wide stop should also fail strong max")
		}
		// ~2.35% stop — fails standard, passes strong
		coin.V7Invalidation = local.V7InvalidationRule{Price: 102.657}
		if hunterV7StopGateMatches(coin, hunterV7StopGate{MaxPct: 2.25, AllowUnknown: true}) {
			t.Fatal("2.35% stop should fail standard 2.25 cap")
		}
		if !hunterV7StopGateMatches(coin, hunterV7StopGate{MaxPct: 2.45, AllowUnknown: true}) {
			t.Fatal("2.35% stop should pass strong 2.45 cap")
		}
	})

	t.Run("soft release standard row matches strong-flow fixture shape", func(t *testing.T) {
		coin := base
		coin.V7DerivativesCtx = &local.V7DerivativesContext{OIChange1h: 0.8, OIChange4h: 0.3, TakerBuy15m: 0.38}
		if !hunterV7TierRuleMatches(coin, &altLadderShortSoftReleaseStandard) {
			t.Fatal("expected standard soft-release row to match")
		}
	})

	t.Run("soft release missing zone does not match", func(t *testing.T) {
		coin := base
		coin.V7PriceContext = nil
		coin.V7EntryZone = local.V7PriceZone{}
		coin.V7DerivativesCtx = &local.V7DerivativesContext{OIChange1h: 0.8, TakerBuy15m: 0.33}
		if hunterV7TierRuleMatches(coin, &altLadderShortSoftReleaseStrong) {
			t.Fatal("missing zone must block soft release")
		}
	})
}

func TestHunterV7WhaleLongGatesTableAligned(t *testing.T) {
	coin := CandidateCoin{
		V7SetupType:    "whale_flow_reversal",
		Direction:      "LONG",
		V7AIPriority:   65,
		V7TimingScore:  65,
		V7RiskScore:    40,
		V7EntryZone:    local.V7PriceZone{Lower: 100, Upper: 110},
		V7PriceContext: &local.V7PriceContext{Last: 103}, // 30%
		V7DerivativesCtx: &local.V7DerivativesContext{
			TakerBuy15m: 0.60,
		},
	}
	if !hunterV7WhaleFlowLongEntryGatesOK(coin) {
		t.Fatal("low-zone strong taker should pass")
	}
	coin.V7PriceContext = &local.V7PriceContext{Last: 108} // 80%
	if hunterV7WhaleFlowLongEntryGatesOK(coin) {
		t.Fatal("high-zone should fail")
	}
	coin.V7PriceContext = &local.V7PriceContext{Last: 103}
	coin.V7DerivativesCtx.TakerBuy15m = 0.50
	if hunterV7WhaleFlowLongEntryGatesOK(coin) {
		t.Fatal("weak taker should fail")
	}
	coin.Direction = "SHORT"
	if !hunterV7WhaleFlowLongEntryGatesOK(coin) {
		t.Fatal("SHORT should bypass LONG gates")
	}
}
