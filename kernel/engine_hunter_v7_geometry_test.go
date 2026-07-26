package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// U3.1 regression: candidate construction must classify with the
// engine-configured execution geometry, not the default geometry. Before the
// fix, a strategy with a wide stop floor produced EXECUTABLE-looking tiers at
// construction that the prompt path (configured geometry) would later reject.
func TestHunterV7SignalsToCandidateCoinsClassifyWithConfiguredGeometry(t *testing.T) {
	signal := local.V7SignalOutput{
		Symbol:         "GEOUSDT",
		Direction:      local.V7DirLong,
		SetupType:      local.V7SetupTrendBreakoutLong,
		Status:         "candidate",
		RiskLevel:      "MEDIUM",
		MarketRegime:   local.V7RegimeTrendUp,
		AIPriority:     62,
		SetupScore:     60,
		TimingScore:    58,
		LiquidityScore: 60,
		RiskScore:      30,
		PriceCtx:       &local.V7PriceContext{Last: 100},
		Invalidation:   local.V7InvalidationRule{Price: 98.8},
		Targets:        []local.V7Target{{Price: 104.5}},
	}

	defaultEngine := NewStrategyEngine(&store.StrategyConfig{
		CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"},
	})
	defaults := defaultEngine.hunterV7SignalsToCandidateCoins([]local.V7SignalOutput{signal}, "BOTH")
	if len(defaults) != 1 {
		t.Fatalf("default candidates = %d, want 1", len(defaults))
	}
	if defaults[0].V7TierReason == "backend_rr_infeasible" {
		t.Fatalf("default geometry should keep RR feasible, got tier=%s reason=%s",
			defaults[0].V7ExecutionTier, defaults[0].V7TierReason)
	}

	tightEngine := NewStrategyEngine(&store.StrategyConfig{
		CoinSource:  store.CoinSourceConfig{SourceType: "hunter_v7"},
		RiskControl: store.RiskControlConfig{MinStopLossPriceMovePct: 4.0},
	})
	tightened := tightEngine.hunterV7SignalsToCandidateCoins([]local.V7SignalOutput{signal}, "BOTH")
	if len(tightened) != 1 {
		t.Fatalf("tight candidates = %d, want 1", len(tightened))
	}
	if tightened[0].V7ExecutionTier != "WATCH" || tightened[0].V7TierReason != "backend_rr_infeasible" {
		t.Fatalf("configured geometry must reach construction-time tiering, got tier=%s reason=%s",
			tightened[0].V7ExecutionTier, tightened[0].V7TierReason)
	}
}
