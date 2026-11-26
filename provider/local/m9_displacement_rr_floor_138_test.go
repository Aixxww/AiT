package local

import "testing"

// #138 M9: the displacement module-time RR gate reads the strategy floor
// (V7Config.EffectiveMinRR) through the router pass, not a literal 1.5.
func TestM9138DisplacementRRGateUsesStrategyFloor(t *testing.T) {
	mk := func() (*V7SignalOutput, *V7SymbolContext) {
		ctx := &V7SymbolContext{Symbol: "DISPUSDT", CurrentPrice: 100}
		sig := &V7SignalOutput{
			Symbol: "DISPUSDT", Direction: V7DirLong, SetupType: V7SetupDisplacementLong,
			EntryZone:    V7PriceZone{Lower: 100, Upper: 100},
			Invalidation: V7InvalidationRule{Price: 98},
			Targets:      []V7Target{{Price: 102.6}}, // best RR 1.3
		}
		return sig, ctx
	}
	sig, ctx := mk()
	applyDisplacementRRFloor(sig, ctx, (V7Config{MinRiskRewardRatio: 1.2}).EffectiveMinRR())
	if sig.ExecutionQuality == V7ExecInvalidRR || containsString(sig.RiskTags, "displacement_rr_insufficient") {
		t.Fatalf("RR 1.3 rejected at a 1.2 strategy floor: %+v", sig.RiskTags)
	}
	sig, ctx = mk()
	applyDisplacementRRFloor(sig, ctx, (V7Config{MinRiskRewardRatio: 1.5}).EffectiveMinRR())
	if sig.ExecutionQuality != V7ExecInvalidRR || !containsString(sig.RiskTags, "displacement_rr_insufficient") {
		t.Fatalf("RR 1.3 must fail a 1.5 strategy floor: quality=%s tags=%+v", sig.ExecutionQuality, sig.RiskTags)
	}
	// Other setups are untouched by the pass.
	other, ctx := mk()
	other.SetupType = V7SetupLeaderMomentumLong
	applyDisplacementRRFloor(other, ctx, 5)
	if other.ExecutionQuality != "" || len(other.RiskTags) != 0 {
		t.Fatalf("non-displacement signal modified: %+v", other)
	}
	// The pass is wired first in the router pipeline with the cfg floor.
	passes := buildV7SignalPasses(nil, V7RegimeRotation, V7Config{MinRiskRewardRatio: 1.2})
	if len(passes) == 0 || passes[0].name != "displacement_rr_floor" {
		t.Fatalf("displacement_rr_floor must be the first router pass")
	}
	sig, ctx = mk()
	passes[0].apply(sig, ctx)
	if sig.ExecutionQuality == V7ExecInvalidRR {
		t.Fatal("router pass did not use the configured 1.2 floor")
	}
}
