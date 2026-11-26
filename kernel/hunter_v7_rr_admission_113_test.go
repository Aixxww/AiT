package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// #113: a candidate whose backend RR is below its configured floor, or
// cannot be computed, never reaches the LLM as EXECUTABLE/REVIEWABLE,
// whatever tier rule promoted it.

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
}

// #113 owner decision: unknown RR fails closed. A row a rule promotes to
// REVIEWABLE (or EXECUTABLE) with incomplete geometry (no target, no stop,
// no reference price, or a stop already beyond the price) ends in WATCH
// rr_unknown, distinct from rr_below_floor.
func TestRRAdmission113_UnknownRRFailsClosed(t *testing.T) {
	g := rrAdmissionGeometry(1.2)
	cases := map[string]func(*CandidateCoin){
		"no target":         func(c *CandidateCoin) { c.V7Targets = nil },
		"no stop":           func(c *CandidateCoin) { c.V7Invalidation.Price = 0 },
		"stop beyond price": func(c *CandidateCoin) { c.V7Invalidation.Price = 101 },
		"no price": func(c *CandidateCoin) {
			c.V7PriceContext = &local.V7PriceContext{Change1h: -0.5}
			c.V7EntryZone = local.V7PriceZone{}
		},
	}
	gated := 0
	for name, mutate := range cases {
		coin := rrAdmissionCoin("RRUNKUSDT", 1.5)
		mutate(&coin)
		if _, _, ok := hunterV7BackendCappedRR(coin, g); ok {
			t.Fatalf("%s: RR must be unknown", name)
		}
		pre, _ := hunterV7TierRulesEval(coin, g)
		tier, reason := classifyHunterV7CandidateTierWithGeometry(coin, g)
		if pre != "EXECUTABLE" && pre != "REVIEWABLE" {
			// The rules already keep it out (e.g. no price: zone gates fail).
			if tier == "EXECUTABLE" || tier == "REVIEWABLE" {
				t.Fatalf("%s: got %s %s", name, tier, reason)
			}
			continue
		}
		gated++
		if tier != "WATCH" || reason != hunterV7RRUnknownReason {
			t.Fatalf("%s: rules promote to %s; got %s %s, want WATCH %s", name, pre, tier, reason, hunterV7RRUnknownReason)
		}
	}
	if gated < 3 {
		t.Fatalf("only %d cases exercised the gate", gated)
	}
	// The promoting rule really fires for the no-target fixture (pre-gate
	// verdict REVIEWABLE), so the WATCH comes from the gate.
	noTarget := rrAdmissionCoin("RRUNKUSDT", 1.5)
	noTarget.V7Targets = nil
	if tier, _ := hunterV7TierRulesEval(noTarget, g); tier != "REVIEWABLE" {
		t.Fatalf("fixture must be promoted by the rules, got %s", tier)
	}
	// EXECUTABLE is gated the same way.
	exec := hunterV7PromptReadyLeaderCandidate("RRUNKEXECUSDT", 100)
	if tier, _ := classifyHunterV7CandidateTierWithGeometry(exec, HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true)); tier != "EXECUTABLE" {
		t.Fatalf("fixture: complete geometry must stay EXECUTABLE, got %s", tier)
	}
	exec.V7Targets = nil
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(exec, HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true)); tier != "WATCH" || reason != hunterV7RRUnknownReason {
		t.Fatalf("EXECUTABLE with no target: got %s %s", tier, reason)
	}
	// Prompt readiness treats it like rr_below_floor.
	ready := hunterV7PromptExecutionReadiness(noTarget, hunterV7PromptTestMarketData("RRUNKUSDT", 100.5), "WATCH", hunterV7RRUnknownReason)
	if ready.Tier != local.V7ReadinessWatch || ready.BlockedGate != "execution_geometry" || ready.Reason != hunterV7RRUnknownReason {
		t.Fatalf("readiness %+v", ready)
	}
	// A shadow_only class keeps its own reason.
	shadow := noTarget
	shadow.V7ClassAdmission = store.V7AdmissionShadowOnly
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(shadow, g); tier != "WATCH" || reason != "class_shadow_only" {
		t.Fatalf("shadow_only: got %s %s", tier, reason)
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
