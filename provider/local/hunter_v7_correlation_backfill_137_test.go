package local

import "testing"

// #137: with MinOutput > CorrelationMaxPerTheme the correlation filter still
// restores same-theme rows as context, but each restored row is tagged
// correlation_floor_context, its readiness is WATCH at most, and the tag is
// wait_only (a prompt hard block and a trader-guard reject).
func TestCorrelationBackfill137RowsAreContextOnly(t *testing.T) {
	ready := func(tier V7ReadinessTier) *V7ExecutionReadiness {
		return &V7ExecutionReadiness{Tier: tier, Reason: "readiness_ready"}
	}
	signals := []V7SignalOutput{
		{Symbol: "M1USDT", Direction: V7DirLong, SetupType: V7SetupLeaderMomentumLong, AIPriority: 90, ExecutionReadiness: ready(V7ReadinessExecutable)},
		{Symbol: "M2USDT", Direction: V7DirLong, SetupType: V7SetupTrendBreakoutLong, AIPriority: 89, ExecutionReadiness: ready(V7ReadinessExecutable)},
		{Symbol: "M3USDT", Direction: V7DirLong, SetupType: V7SetupDisplacementLong, AIPriority: 88, ExecutionReadiness: ready(V7ReadinessExecutable)},
		{Symbol: "M4USDT", Direction: V7DirLong, SetupType: V7SetupLeaderMomentumLong, AIPriority: 87, ExecutionReadiness: ready(V7ReadinessReviewable)},
	}
	cfg := V7Config{MinOutput: 4, MaxOutput: 10, CorrelationMaxPerTheme: 2}
	if cfg.MinOutput <= cfg.CorrelationMaxPerTheme {
		t.Fatal("fixture must have MinOutput > CorrelationMaxPerTheme")
	}
	got := applyV7CorrelationFilter(signals, cfg)
	if len(got) != 4 {
		t.Fatalf("rows = %d, want 4 (2 kept + 2 restored as context)", len(got))
	}
	backfilled := 0
	for _, sig := range got {
		if !containsString(sig.RiskTags, V7CorrelationFloorContextTag) {
			if sig.ExecutionReadiness.Tier != V7ReadinessExecutable {
				t.Errorf("%s: kept row was demoted (%s)", sig.Symbol, sig.ExecutionReadiness.Tier)
			}
			continue
		}
		backfilled++
		if sig.ExecutionReadiness.Tier != V7ReadinessWatch || sig.ExecutionReadiness.BlockedGate != V7CorrelationFloorContextTag {
			t.Errorf("%s: backfilled readiness = %s/%s, want WATCH/%s", sig.Symbol, sig.ExecutionReadiness.Tier, sig.ExecutionReadiness.BlockedGate, V7CorrelationFloorContextTag)
		}
		if sig.AIPriority == 0 {
			t.Errorf("%s: backfilled row lost its scores; it must still read as context", sig.Symbol)
		}
	}
	if backfilled != 2 {
		t.Fatalf("backfilled rows = %d, want 2", backfilled)
	}
	if action, ok := HunterV7TagLLMAction(V7CorrelationFloorContextTag); !ok || action != V7TagActionWaitOnly {
		t.Fatalf("%s catalog action = %q, want wait_only", V7CorrelationFloorContextTag, action)
	}
}
