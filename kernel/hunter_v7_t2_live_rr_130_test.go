package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/store"

	"github.com/Aixxww/AiT/market"
	local "github.com/Aixxww/AiT/provider/local"
)

// #130 / Audit R2 H7: at prompt-final a market-entry (non-conditional)
// Task-2 candidate is RR-gated at the live price, a conditional entry at
// its trigger price; the assembly-time entry-zone edge no longer decides.

// promptFinalTier repeats what hunterV7PromptItems does before tiering.
func promptFinalTier(c CandidateCoin, livePrice float64, g HunterV7ExecutionGeometry) (CandidateCoin, string, string) {
	c = hunterV7CandidateWithLiveMarketPrice(c, &market.Data{Symbol: c.Symbol, CurrentPrice: livePrice})
	tier, reason := classifyHunterV7CandidateTierWithGeometry(c, g)
	return c, tier, reason
}

func TestT2LiveRR130_MarketEntryUsesLivePrice(t *testing.T) {
	g := condGeometry(true)
	// LONG zone 100/100, stop 99, targets 101.8 / 103; T2 floor 1.2.
	c := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{condSig("ignition_early_long", local.V7DirLong, "TRIGGERED")}, "BOTH", g)[0]
	if c.V7ExecutionTier != "REVIEWABLE" || c.V7ConditionalEntry || c.V7RRReferencePrice != 100 {
		t.Fatalf("fixture: %s/%s cond=%v ref=%v", c.V7ExecutionTier, c.V7TierReason, c.V7ConditionalEntry, c.V7RRReferencePrice)
	}
	// Live 101: RR from the zone edge would be 3.0; from the live price it
	// is (103-101)/(101-99) = 1.0 < 1.2.
	stale := c
	stale.V7PriceContext = &local.V7PriceContext{Last: 101}
	if tier, _ := classifyHunterV7CandidateTierWithGeometry(stale, g); tier != "REVIEWABLE" {
		t.Fatalf("fixture: the stale zone edge must have admitted it, got %s", tier)
	}
	got, tier, reason := promptFinalTier(c, 101, g)
	if got.V7RRReferencePrice != 0 || hunterV7ReferencePrice(got) != 101 {
		t.Fatalf("reference price %v / %v, want the live price", got.V7RRReferencePrice, hunterV7ReferencePrice(got))
	}
	// A T2 row reaches the rules' own backend RR check before its lifecycle
	// rule promotes it, so that check (same computation as the #113 gate)
	// names the demotion.
	if tier != "WATCH" || reason != "backend_rr_infeasible" {
		t.Fatalf("live RR 1.0 < 1.2 must demote to WATCH on the RR gate, got %s %s", tier, reason)
	}
	// At a live price that keeps RR above the floor it stays admitted.
	if _, tier, reason := promptFinalTier(c, 100.2, g); tier != "REVIEWABLE" {
		t.Fatalf("live 100.2 (RR %.2f): got %s %s", (103-100.2)/(100.2-99), tier, reason)
	}
}

// The admission gate after the rules (rr_below_floor) also reads the live
// price for a T2 row the rules promoted without their own RR check.
func TestT2LiveRR130_AdmissionGateSeesLivePrice(t *testing.T) {
	g := condGeometry(true)
	c := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{condSig("ignition_early_long", local.V7DirLong, "TRIGGERED")}, "BOTH", g)[0]
	c = hunterV7CandidateWithLiveMarketPrice(c, &market.Data{CurrentPrice: 101})
	if rr, floor, ok := hunterV7BackendCappedRR(c, g); !ok || rr >= floor || rr > 1.01 {
		t.Fatalf("backend RR at the live price: %v floor %v ok %v", rr, floor, ok)
	}
}

func TestT2LiveRR130_ConditionalGatesOnTrigger(t *testing.T) {
	g := condGeometry(true)
	c := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{condSig("ignition_early_long", local.V7DirLong, "WATCH")}, "BOTH", g)[0]
	if c.V7ExecutionTier != "REVIEWABLE" || !c.V7ConditionalEntry {
		t.Fatalf("fixture: %s/%s cond=%v", c.V7ExecutionTier, c.V7TierReason, c.V7ConditionalEntry)
	}
	// The live price is irrelevant to a pending entry: it fills at the
	// trigger (100), RR 3.0 there.
	got, tier, reason := promptFinalTier(c, 101.5, g)
	if got.V7RRReferencePrice != 100 || tier != "REVIEWABLE" {
		t.Fatalf("conditional at trigger 100: ref %v, %s %s", got.V7RRReferencePrice, tier, reason)
	}
	// A trigger at 102 fails RR there ((103-102)/(102-99) = 0.33) although
	// the zone edge 100 would pass: the trigger decides.
	c.V7TriggerPrice = 102
	got, tier, reason = promptFinalTier(c, 101.5, g)
	if got.V7RRReferencePrice != 102 || tier != "WATCH" {
		t.Fatalf("conditional at trigger 102: ref %v, %s %s", got.V7RRReferencePrice, tier, reason)
	}
}

// Native candidates are untouched (they never carry a reference price).
func TestT2LiveRR130_NativeUnchanged(t *testing.T) {
	c := rrAdmissionCoin("NATUSDT", 1.5)
	c.V7RRReferencePrice = 0
	got := hunterV7CandidateWithLiveMarketPrice(c, &market.Data{CurrentPrice: 100.5})
	if got.V7RRReferencePrice != 0 || got.V7PriceContext.Last != 100.5 {
		t.Fatalf("native: %+v", got.V7PriceContext)
	}
}

// End to end through FinalizeHunterV7Tiers: the prompt-final tier the LLM
// and the gate see is WATCH; the assembled tier stays REVIEWABLE.
func TestT2LiveRR130_PromptFinal(t *testing.T) {
	cfg := &store.StrategyConfig{Language: "en", CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7",
		Hunter: &store.HunterConfig{V7T2Enabled: true, V7ConditionalEntry: &store.V7ConditionalEntryConfig{Enabled: true}}}}
	cfg.RiskControl.MinRiskRewardRatio = 1.2
	e := NewStrategyEngine(cfg)
	sig := condSig("ignition_early_long", local.V7DirLong, "TRIGGERED")
	cands := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "BOTH", e.hunterV7ExecutionGeometry())
	if len(cands) != 1 || cands[0].V7ExecutionTier != "REVIEWABLE" {
		t.Fatalf("fixture: %+v", cands)
	}
	ctx := &Context{CandidateCoins: cands, MarketDataMap: map[string]*market.Data{"AUSDT": hunterV7PromptTestMarketData("AUSDT", 101)}}
	e.FinalizeHunterV7Tiers(ctx)
	if len(ctx.HunterV7PromptFinal) != 1 {
		t.Fatalf("prompt-final %+v", ctx.HunterV7PromptFinal)
	}
	f := ctx.HunterV7PromptFinal[0]
	if f.Tier != "WATCH" || f.AssembledTier != "REVIEWABLE" {
		t.Fatalf("prompt-final %s/%s (assembled %s)", f.Tier, f.TierReason, f.AssembledTier)
	}
	if c := hunterV7PromptFinalTierCounts(ctx.HunterV7PromptFinal); c.Executable+c.Reviewable != 0 {
		t.Fatalf("E+R must be 0: %+v", c)
	}
}
