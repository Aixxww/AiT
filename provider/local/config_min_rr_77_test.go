package local

import "testing"

// #77: the router's RR admission uses the strategy's configured floor; the
// former code floors (1.2 absolute, 1.5 executable) are gone.
func TestRouterRRAdmissionUsesConfiguredFloor(t *testing.T) {
	mk := func() (*V7SignalOutput, *V7SymbolContext) {
		// price 100, stop 97.5 (2.5%), target 103.25 -> RR 1.3
		return &V7SignalOutput{
			Symbol: "RRUSDT", Direction: V7DirLong, SetupType: V7SetupLeaderMomentumLong, Status: V7StatusCandidate,
			SetupScore: 80, TimingScore: 70, LiquidityScore: 100, EntryMode: V7EntryImmediate,
			EntryZone: V7PriceZone{Lower: 99.5, Upper: 100.5}, Invalidation: V7InvalidationRule{Price: 97.5},
			Targets: []V7Target{{Price: 103.25}},
		}, &V7SymbolContext{CurrentPrice: 100}
	}

	sig, ctx := mk()
	cfg := DefaultV7Config()
	cfg.MinRiskRewardRatio = 1.2
	finalizeV7SignalForExecution(sig, ctx, cfg)
	if sig.ExecutionQuality == V7ExecInvalidRR || containsV7String(sig.RiskTags, "thin_rr_wait_confirm") {
		t.Fatalf("RR 1.3 under a configured 1.2 floor: quality=%s tags=%v", sig.ExecutionQuality, sig.RiskTags)
	}

	sig, ctx = mk()
	cfg.MinRiskRewardRatio = 1.5
	finalizeV7SignalForExecution(sig, ctx, cfg)
	if sig.ExecutionQuality != V7ExecInvalidRR {
		t.Fatalf("RR 1.3 under a configured 1.5 floor must be invalid_rr, got %s", sig.ExecutionQuality)
	}

	if (V7Config{}).EffectiveMinRR() != 1.2 {
		t.Fatal("unset floor must be the store default 1.2")
	}
}
