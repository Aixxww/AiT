package kernel

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// Fix 12: BuildUserPrompt records the prompt-final tier (what the LLM sees)
// next to the assembled tier, without changing the prompt. Since #86 the
// prompt-final tier is also written back to the candidates.

func promptFinalFixture() (*StrategyEngine, func() *Context) {
	engine := NewStrategyEngine(&store.StrategyConfig{Language: "en", CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"}})
	mk := func() *Context {
		up := hunterV7PromptReadyLeaderCandidate("LIVEINUSDT", 104) // assembled WATCH; the live price promotes it
		up.V7ExecutionTier, up.V7TierReason = "WATCH", "backend_rr_infeasible"
		held := hunterV7PromptReadyLeaderCandidate("HELDUSDT", 100) // held position: not shown
		nodata := hunterV7PromptReadyLeaderCandidate("NODATAUSDT", 100)
		return &Context{
			CandidateCoins: []CandidateCoin{up, held, nodata},
			Positions:      []PositionInfo{{Symbol: "HELDUSDT", Side: "long", Quantity: 1, EntryPrice: 100, MarkPrice: 100}},
			MarketDataMap: map[string]*market.Data{
				"LIVEINUSDT": hunterV7PromptTestMarketData("LIVEINUSDT", 100),
				"HELDUSDT":   hunterV7PromptTestMarketData("HELDUSDT", 100),
			},
		}
	}
	return engine, mk
}

func TestBuildUserPromptRecordsPromptFinalTier(t *testing.T) {
	engine, mk := promptFinalFixture()
	ctx := mk()
	prompt := engine.BuildUserPrompt(ctx)
	if !strings.Contains(prompt, "execution_tier=EXECUTABLE tier_reason=momentum_ready_strong_flow") {
		t.Fatalf("fixture must promote at prompt time\n%s", prompt)
	}
	if len(ctx.HunterV7PromptFinal) != 1 {
		t.Fatalf("prompt-final entries %+v, want only the shown candidate (held / no-data are not shown)", ctx.HunterV7PromptFinal)
	}
	f := ctx.HunterV7PromptFinal[0]
	if f.Symbol != "LIVEINUSDT" || f.Tier != "EXECUTABLE" || f.TierReason != "momentum_ready_strong_flow" ||
		f.AssembledTier != "WATCH" || f.AssembledReason != "backend_rr_infeasible" || f.SetupType == "" {
		t.Fatalf("entry %+v", f)
	}
	// #86 (AUD-B7): the prompt-final tier is authoritative. It is written
	// back to the candidate the AI-skip check, the validator and the guard
	// read; the assembly verdict is kept aside for records.
	c := ctx.CandidateCoins[0]
	if c.V7ExecutionTier != "EXECUTABLE" || c.V7TierReason != "momentum_ready_strong_flow" ||
		c.V7AssembledTier != "WATCH" || c.V7AssembledReason != "backend_rr_infeasible" {
		t.Fatalf("finalized candidate %s/%s (assembled %s/%s)", c.V7ExecutionTier, c.V7TierReason, c.V7AssembledTier, c.V7AssembledReason)
	}
	// Candidates the LLM was not shown keep their assembly verdict.
	for _, c := range ctx.CandidateCoins[1:] {
		if c.V7AssembledTier != "" {
			t.Fatalf("%s not shown but finalized: %+v", c.Symbol, c.V7AssembledTier)
		}
	}
	// The prompt text itself is unchanged by the recording: the U11 and
	// golden prompt fixtures (TestU11LegacyPromptGolden,
	// TestU11HybridV2PromptGolden, TestHunterV7GoldenPrompt) still match.
}

func TestApplyHunterV7PromptFinalTiers(t *testing.T) {
	recs := []store.HunterV7SignalRecord{
		{Symbol: "AUSDT", SetupType: "t2_ignition_early_long", ExecutionTier: "WATCH", TierReason: "t2_awaiting_trigger"},
		{Symbol: "AUSDT", SetupType: "trend_breakout_long", ExecutionTier: "WATCH", TierReason: "x"},
		{Symbol: "BUSDT", SetupType: "funding_reversal", ExecutionTier: "REJECTED"},
	}
	final := []HunterV7PromptFinalTier{
		{Symbol: "AUSDT", SetupType: "trend_breakout_long", Tier: "REVIEWABLE", TierReason: "live_reviewable_x"},
		{Symbol: "CUSDT", SetupType: "x", Tier: "EXECUTABLE"},
	}
	if n := ApplyHunterV7PromptFinalTiers(recs, final); n != 1 {
		t.Fatalf("stamped %d, want 1", n)
	}
	if recs[1].PromptFinalTier != "REVIEWABLE" || recs[1].PromptFinalTierReason != "live_reviewable_x" || recs[1].ExecutionTier != "WATCH" {
		t.Fatalf("matched row %+v", recs[1])
	}
	if recs[0].PromptFinalTier != "" || recs[2].PromptFinalTier != "" {
		t.Fatal("rows not in the prompt stay empty")
	}
	if ApplyHunterV7PromptFinalTiers(recs, nil) != 0 {
		t.Fatal("nil prompt-final stamps nothing")
	}
}
