package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
)

// U6: a hydrated class-profile TP cap replaces the strategy cap in the TP1
// cap and the RR repair; without it the strategy cap applies as before.
func TestDecisionMaxTakeProfitUsesHydratedCap(t *testing.T) {
	at := t2StopTrader() // strategy cap 4.0
	if got := at.decisionMaxTakeProfitPriceMovePct(&kernel.Decision{}); got != at.maxTakeProfitPriceMovePct() {
		t.Fatalf("no hydrated cap = %v", got)
	}
	d := &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long", StopLoss: 98, TakeProfit: 110, HunterV7MaxTPPct: 6}
	if !capTP(at, d, 100, "long") || d.TakeProfit != 106 {
		t.Fatalf("TP capped to %v, want 106", d.TakeProfit)
	}
	n := &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long", StopLoss: 98, TakeProfit: 110}
	if !capTP(at, n, 100, "long") || n.TakeProfit != 104 {
		t.Fatalf("strategy cap TP = %v, want 104", n.TakeProfit)
	}
}

// Owner decision on U6, end to end through kernel + executor:
// intraday_scalp_long at RR 1.1 is assembled with the production geometry of
// the trader's own strategy config, the candidate's stamped floor is carried
// on the decision (as hydrateHunterV7DecisionExecutionPlans does, covered in
// kernel TestSetupOverrideReachesValidatorAndHydration), and the executor
// accepts it; before the fix the executor used the strategy 1.5 and rejected.
func TestExecutorAcceptsSetupOverrideRR(t *testing.T) {
	at := t2StopTrader() // hunter_v7, strategy RR 1.5, stop 2.0, TP cap 4.0
	geom := kernel.HunterV7ExecutionGeometryForConfig(at.config.StrategyConfig)
	sig := local.V7SignalOutput{
		SignalID: "scalp", Symbol: "SOLUSDT", Direction: local.V7DirLong, SetupType: "intraday_scalp_long", Status: local.V7StatusCandidate,
		PriceCtx: &local.V7PriceContext{Last: 100}, EntryZone: local.V7PriceZone{Lower: 100, Upper: 100},
		Invalidation: local.V7InvalidationRule{Price: 98}, Targets: []local.V7Target{{Price: 102.2}}, TP1Price: 102.2,
	}
	c := kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "BOTH", geom)
	if len(c) != 1 || c[0].V7TierReason == "backend_rr_infeasible" || c[0].V7RequiredRR != 1.0 {
		t.Fatalf("kernel: %+v", c)
	}
	mk := func(minRR, maxTP float64) *kernel.Decision {
		return &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 30, Price: 100,
			StopLoss: 98, TakeProfit: 102.2, Confidence: 80, SelectedHunterV7SignalID: "scalp",
			HunterV7MinRR: minRR, HunterV7MaxTPPct: maxTP}
	}
	d := mk(c[0].V7RequiredRR, c[0].V7RequiredMaxTP)
	at.repairHunterV7OpenDecision(d, 100, "long")
	if err := at.validateOpenDecision(d, 100, "long"); err != nil {
		t.Fatalf("executor must accept the RR 1.1 the kernel accepted: %v", err)
	}
	if d.TakeProfit != 102.2 || d.StopLoss != 98 {
		t.Fatalf("repair must not move a valid plan: tp %v sl %v", d.TakeProfit, d.StopLoss)
	}
	if err := at.validateOpenDecision(mk(0, 0), 100, "long"); err == nil {
		t.Fatalf("without the stamp the strategy floor 1.5 applies (the pre-fix gap)")
	}

	// displacement_momentum_long: the stamped TP cap 12 replaces the 4.0 cap.
	sig.SignalID, sig.SetupType, sig.Targets, sig.TP1Price = "disp", "displacement_momentum_long", []local.V7Target{{Price: 110}}, 110
	c = kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "BOTH", geom)
	if c[0].V7RequiredMaxTP != 12 || c[0].V7RequiredRR != 1.35 {
		t.Fatalf("displacement stamps = %v/%v", c[0].V7RequiredMaxTP, c[0].V7RequiredRR)
	}
	dd := mk(c[0].V7RequiredRR, c[0].V7RequiredMaxTP)
	dd.TakeProfit = 110
	if capTP(at, dd, 100, "long") || dd.TakeProfit != 110 {
		t.Fatalf("TP 10%% is inside the 12%% setup cap, got %v", dd.TakeProfit)
	}
}
