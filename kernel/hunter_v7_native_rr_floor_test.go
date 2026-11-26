package kernel

import (
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Fix 07: the default strategy's native RR floor is 1.2 (design standard,
// the same as the T2 floor). The kernel fallbacks still apply only when a
// strategy leaves the floor unset.

func TestDefaultStrategyNativeRRFloorIs1_2(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	if got := cfg.RiskControl.MinRiskRewardRatio; got != 1.2 {
		t.Fatalf("default native floor %.2f, want 1.2", got)
	}
	if got := cfg.RiskControl.EffectiveT2MinRiskRewardRatio(); got != 1.2 {
		t.Fatalf("T2 floor %.2f, want 1.2 (unchanged)", got)
	}
	if zh := store.GetDefaultStrategyConfig("zh"); zh.RiskControl.MinRiskRewardRatio != 1.2 {
		t.Fatalf("zh default native floor %.2f, want 1.2", zh.RiskControl.MinRiskRewardRatio)
	}

	cfg.CoinSource.SourceType = "hunter_v7"
	g := HunterV7ExecutionGeometryForConfig(&cfg)
	if g.MinRR != 1.2 || g.T2MinRR != 1.2 {
		t.Fatalf("assembly geometry native %.2f / T2 %.2f, want 1.2 / 1.2", g.MinRR, g.T2MinRR)
	}
	// The decision validator floor follows the strategy value.
	if got := FixedDecisionRRFloor(cfg.RiskControl.MinRiskRewardRatio)(&Decision{}); got != 1.2 {
		t.Fatalf("decision floor %.2f, want 1.2", got)
	}
}

// #77: the unset fallback is the store default (1.2), not a code floor of
// 1.5; a configured floor is used as is.
func TestNativeRRFallbackOnlyWhenUnset(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{
		{0, store.DefaultMinRiskRewardRatio},
		{-1, store.DefaultMinRiskRewardRatio},
		{1.2, 1.2},
		{3.0, 3.0},
	} {
		if got := HunterV7EffectiveExecutionGeometry(0, 0, 0, c.in, true).MinRR; got != c.want {
			t.Errorf("strategy floor %.2f -> assembly floor %.2f, want %.2f", c.in, got, c.want)
		}
	}
	// The router receives the same configured floor (no separate 1.5).
	if got := (local.V7Config{MinRiskRewardRatio: 1.2}).EffectiveMinRR(); got != 1.2 {
		t.Fatalf("router floor %.2f, want 1.2", got)
	}
	if got := (local.V7Config{}).EffectiveMinRR(); got != store.DefaultMinRiskRewardRatio {
		t.Fatalf("router unset floor %.2f, want the store default", got)
	}
	// #138 M9: the decision validator's unset fallback is the same store
	// default (it was a separate literal 3.0). RR 1.5 passes an unset floor.
	d := &Decision{Symbol: "XUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 100, StopLoss: 98, TakeProfit: 103, Confidence: 80}
	if err := validateDecision(d, 1000, 20, 20, 10, 10, nil, map[string]float64{"XUSDT": 100}, FixedDecisionRRFloor(0), 0); err != nil {
		t.Fatalf("unset validator floor must be the store default %.2f, RR 1.5 rejected: %v", store.DefaultMinRiskRewardRatio, err)
	}
	d.TakeProfit = 102.2 // RR 1.1 < 1.2
	if err := validateDecision(d, 1000, 20, 20, 10, 10, nil, map[string]float64{"XUSDT": 100}, FixedDecisionRRFloor(0), 0); err == nil {
		t.Fatal("unset validator floor must still reject RR below the store default")
	}
}

// The native floor also feeds the TP1-distance feasibility floor of the
// shared geometry: (min stop 2.0 + drift 0.5) x floor + 0.25. With the
// default limits the cap is 7.75% at 3.0 and 3.25% at 1.2.
func TestNativeRRFloorFeedsTPCap(t *testing.T) {
	if got := HunterV7EffectiveExecutionGeometry(3.0, 2.0, 0.5, 3.0, true).MaxTPMovePct; got != 7.75 {
		t.Fatalf("TP cap at floor 3.0: %.2f, want 7.75", got)
	}
	if got := HunterV7EffectiveExecutionGeometry(3.0, 2.0, 0.5, 1.2, true).MaxTPMovePct; got != 3.25 {
		t.Fatalf("TP cap at floor 1.2: %.2f, want 3.25", got)
	}
}
