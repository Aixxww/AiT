package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// #113: a candidate whose backend RR is below its configured floor never
// reaches the LLM as EXECUTABLE/REVIEWABLE, whatever tier rule promoted it.

// rrAdmissionCoin is a leader_momentum_long late-pullback candidate: the
// rule that promotes it (hunterV7LeaderMomentumLatePullbackReviewable)
// returns REVIEWABLE before the tier rules' own backend RR check. Entry
// reference 100.5, stop 97 (risk 3.4826%, above the 2% minimum), so the
// backend RR is reward% / 3.4826; rr sets the target.
func rrAdmissionCoin(symbol string, rr float64) CandidateCoin {
	const price, stop = 100.5, 97.0
	riskPct := (price - stop) / price * 100
	target := price * (1 + rr*riskPct/100)
	return CandidateCoin{
		Symbol: symbol, Direction: "LONG",
		V7SignalID: "1|" + symbol + "|LONG|leader_momentum_long", V7SetupType: "leader_momentum_long",
		V7Status: "candidate", V7ExecutionQuality: "ready",
		V7AIPriority: 80, V7SetupScore: 80, V7TimingScore: 60, V7RiskScore: 10, V7LiquidityScore: 70,
		V7ReasonCodes:    []string{"strong_4h_momentum"},
		V7EntryZone:      local.V7PriceZone{Lower: 98, Upper: 101},
		V7Invalidation:   local.V7InvalidationRule{Price: stop},
		V7Targets:        []local.V7Target{{Price: target}},
		V7PriceContext:   &local.V7PriceContext{Last: price, Change1h: -0.5},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.55},
	}
}

func rrAdmissionGeometry(minRR float64) HunterV7ExecutionGeometry {
	return HunterV7ExecutionGeometry{MinRR: minRR, MinSLMovePct: 2, MaxTPMovePct: 30, MaxDriftPct: 0.5}
}

func TestRRAdmission113_BelowFloorStaysWatch(t *testing.T) {
	g := rrAdmissionGeometry(1.2)
	below := rrAdmissionCoin("RRBELOWUSDT", 1.19)
	// The promoting rule fires before the tier rules' own RR check: without
	// the admission gate this candidate would be REVIEWABLE.
	if tier, reason := hunterV7TierRulesEval(below, g); tier != "REVIEWABLE" || reason != "momentum_late_pullback_reviewable_trailing_entry" {
		t.Fatalf("fixture must exercise the pre-RR promotion path, got %s %s", tier, reason)
	}
	if rr, floor, ok := hunterV7BackendCappedRR(below, g); !ok || floor != 1.2 || rr >= 1.2 || rr < 1.18 {
		t.Fatalf("backend RR %v floor %v ok %v", rr, floor, ok)
	}
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(below, g); tier != "WATCH" || reason != hunterV7RRBelowFloorReason {
		t.Fatalf("RR 1.19 < 1.2 must stay WATCH rr_below_floor, got %s %s", tier, reason)
	}
	for _, rr := range []float64{1.2, 1.5} {
		at := rrAdmissionCoin("RRATUSDT", rr)
		if tier, reason := classifyHunterV7CandidateTierWithGeometry(at, g); tier != "REVIEWABLE" || reason != "momentum_late_pullback_reviewable_trailing_entry" {
			t.Fatalf("RR %.2f >= floor must be unaffected, got %s %s", rr, tier, reason)
		}
	}
	// Incomplete geometry (no target) is not judged by the gate.
	noTarget := rrAdmissionCoin("RRNOTGTUSDT", 1.0)
	noTarget.V7Targets = nil
	if _, _, ok := hunterV7BackendCappedRR(noTarget, g); ok {
		t.Fatal("no target: RR must be unknown")
	}
}

// The floor follows the configured strategy value, through the same
// geometry the engine builds from RiskControl.MinRiskRewardRatio.
func TestRRAdmission113_FloorFollowsStrategyConfig(t *testing.T) {
	coin := rrAdmissionCoin("RRCFGUSDT", 1.6)
	for _, c := range []struct {
		floor float64
		tier  string
	}{{1.2, "REVIEWABLE"}, {1.5, "REVIEWABLE"}, {1.7, "WATCH"}, {2.0, "WATCH"}} {
		cfg := &store.StrategyConfig{Language: "en", CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"}}
		cfg.RiskControl.MinRiskRewardRatio = c.floor
		g := NewStrategyEngine(cfg).hunterV7ExecutionGeometry()
		if g.MinRR != c.floor {
			t.Fatalf("engine geometry MinRR %v, want the configured %v", g.MinRR, c.floor)
		}
		g.MaxTPMovePct = 30 // keep the TP cap out of the comparison
		tier, reason := classifyHunterV7CandidateTierWithGeometry(coin, g)
		if tier != c.tier || (c.tier == "WATCH" && reason != hunterV7RRBelowFloorReason) {
			t.Fatalf("floor %.2f, RR 1.6: got %s %s, want %s", c.floor, tier, reason, c.tier)
		}
	}
}

// End to end through the prompt-final tiering: the sub-floor candidate is
// WATCH rr_below_floor in ctx.HunterV7PromptFinal and in the gate-visible
// candidates, so it is not in the E/R sets the LLM sees or the B2 counts.
func TestRRAdmission113_PromptFinalExcludesSubFloor(t *testing.T) {
	cfg := &store.StrategyConfig{Language: "en", CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"}}
	cfg.RiskControl.MinRiskRewardRatio = 1.2
	below := rrAdmissionCoin("RRBELOWUSDT", 1.19)
	below.V7ExecutionTier, below.V7TierReason = "REVIEWABLE", "assembled"
	ctx := &Context{
		CandidateCoins: []CandidateCoin{below},
		MarketDataMap:  map[string]*market.Data{"RRBELOWUSDT": hunterV7PromptTestMarketData("RRBELOWUSDT", 100.5)},
	}
	NewStrategyEngine(cfg).FinalizeHunterV7Tiers(ctx)
	if len(ctx.HunterV7PromptFinal) != 1 {
		t.Fatalf("prompt-final rows: %+v", ctx.HunterV7PromptFinal)
	}
	if f := ctx.HunterV7PromptFinal[0]; f.Tier != "WATCH" || f.TierReason != hunterV7RRBelowFloorReason {
		t.Fatalf("prompt-final tier %s %s", f.Tier, f.TierReason)
	}
	if c := hunterV7PromptFinalTierCounts(ctx.HunterV7PromptFinal); c.Executable+c.Reviewable != 0 {
		t.Fatalf("E+R must be 0, got %+v", c)
	}
	if c := ctx.CandidateCoins[0]; c.V7ExecutionTier != "WATCH" || c.V7TierReason != hunterV7RRBelowFloorReason {
		t.Fatalf("gate-visible tier %s %s", c.V7ExecutionTier, c.V7TierReason)
	}
}
