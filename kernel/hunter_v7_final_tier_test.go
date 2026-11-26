package kernel

import (
	"path/filepath"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// #86 (AUD-B7): one authoritative tier per round.

// hunterV7GateVsPromptDivergences counts candidates the LLM was shown whose
// gate-visible tier (ctx.CandidateCoins) differs from the prompt-final tier.
func hunterV7GateVsPromptDivergences(ctx *Context) int {
	gate := make(map[string]CandidateCoin, len(ctx.CandidateCoins))
	for _, c := range ctx.CandidateCoins {
		gate[HunterV7PromptFinalKey(c.Symbol, c.V7SetupType)] = c
	}
	n := 0
	for _, f := range ctx.HunterV7PromptFinal {
		c, ok := gate[HunterV7PromptFinalKey(f.Symbol, f.SetupType)]
		if !ok || c.V7ExecutionTier != f.Tier || c.V7TierReason != f.TierReason {
			n++
		}
	}
	return n
}

func finalTierEngine() *StrategyEngine {
	return NewStrategyEngine(&store.StrategyConfig{Language: "en", CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"}})
}

func TestFinalizeHunterV7Tiers_PromotionAndDemotionReachTheGate(t *testing.T) {
	up := hunterV7PromptReadyLeaderCandidate("LIVEINUSDT", 104) // assembled WATCH, live price promotes
	up.V7Invalidation.Price = 98                                // #113: stop below the live price 100, so the backend RR is computable
	up.V7ExecutionTier, up.V7TierReason = "WATCH", "backend_rr_infeasible"
	down := hunterV7PromptReadyLeaderCandidate("LIVEOUTUSDT", 100) // assembled EXECUTABLE, live price demotes
	down.V7ExecutionTier, down.V7TierReason = "EXECUTABLE", "cached_from_scoring"
	assembled := []CandidateCoin{up, down}
	ctx := &Context{
		CandidateCoins: assembled,
		MarketDataMap: map[string]*market.Data{
			"LIVEINUSDT":  hunterV7PromptTestMarketData("LIVEINUSDT", 100),
			"LIVEOUTUSDT": hunterV7PromptTestMarketData("LIVEOUTUSDT", 104),
		},
	}
	finalTierEngine().FinalizeHunterV7Tiers(ctx)

	got := map[string]CandidateCoin{}
	for _, c := range ctx.CandidateCoins {
		got[c.Symbol] = c
	}
	if c := got["LIVEINUSDT"]; c.V7ExecutionTier != "EXECUTABLE" || c.V7AssembledTier != "WATCH" {
		t.Fatalf("promotion not authoritative: %s (assembled %s)", c.V7ExecutionTier, c.V7AssembledTier)
	}
	if c := got["LIVEOUTUSDT"]; c.V7ExecutionTier != "WATCH" || c.V7AssembledTier != "EXECUTABLE" {
		t.Fatalf("demotion not authoritative: %s (assembled %s)", c.V7ExecutionTier, c.V7AssembledTier)
	}
	if n := hunterV7GateVsPromptDivergences(ctx); n != 0 {
		t.Fatalf("%d gate vs prompt-final divergences", n)
	}
	if assembled[0].V7ExecutionTier != "WATCH" || assembled[1].V7ExecutionTier != "EXECUTABLE" {
		t.Fatal("finalization mutated the caller's candidate slice")
	}
}

// Finalizing before the prompt (the trader path) and finalizing lazily
// inside BuildUserPrompt (preview, harness, golden) produce the same prompt,
// and the tiers are computed once.
func TestFinalizeHunterV7Tiers_PromptIdenticalAndIdempotent(t *testing.T) {
	mk := func() *Context {
		up := hunterV7PromptReadyLeaderCandidate("LIVEINUSDT", 104)
		up.V7Invalidation.Price = 98 // #113: stop below the live price 100, so the backend RR is computable
		up.V7ExecutionTier, up.V7TierReason = "WATCH", "backend_rr_infeasible"
		return &Context{
			CurrentTime:    "2026-11-25 00:00:00",
			CandidateCoins: []CandidateCoin{up},
			MarketDataMap:  map[string]*market.Data{"LIVEINUSDT": hunterV7PromptTestMarketData("LIVEINUSDT", 100)},
		}
	}
	engine := finalTierEngine()
	lazy := engine.BuildUserPrompt(mk())

	ctx := mk()
	engine.FinalizeHunterV7Tiers(ctx)
	first := ctx.hunterV7Final
	engine.FinalizeHunterV7Tiers(ctx) // idempotent
	if ctx.hunterV7Final != first || !ctx.HunterV7TiersFinalized() {
		t.Fatal("second finalization recomputed the tiers")
	}
	eager := engine.BuildUserPrompt(ctx)
	if eager != lazy {
		t.Fatal("prompt differs between eager and lazy finalization")
	}
	if ctx.HunterV7PromptFinal[0].AssembledTier != "WATCH" {
		t.Fatalf("record lost the assembly verdict: %+v", ctx.HunterV7PromptFinal[0])
	}
}

func TestFinalizeHunterV7Tiers_NonHunterV7IsNoop(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{Language: "en", CoinSource: store.CoinSourceConfig{SourceType: "static"}})
	ctx := &Context{CandidateCoins: []CandidateCoin{{Symbol: "AUSDT", V7SetupType: "x", V7ExecutionTier: "WATCH"}}}
	engine.FinalizeHunterV7Tiers(ctx)
	if ctx.HunterV7TiersFinalized() || ctx.CandidateCoins[0].V7AssembledTier != "" {
		t.Fatal("non-hunter_v7 strategy was finalized")
	}
}

// The X4 regression metric without the AIT_EXPERIMENTS gate: on the frozen
// golden round the gate-visible tier equals the prompt-final tier for every
// shown candidate (0 divergences).
func TestFinalizeHunterV7Tiers_GoldenRoundHasNoSplitBrain(t *testing.T) {
	fixture, err := local.LoadV7GoldenFixture(filepath.Join("..", "provider", "local", "testdata", "golden", "universe-20260726.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	result := local.NewV7Router().RouteDetailed(fixture.Universe, fixture.Regime, fixture.Config)
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.HunterLimit = len(result.OutputSignals)
	engine := NewStrategyEngine(&cfg)
	coins := engine.hunterV7SignalsToCandidateCoins(result.OutputSignals, "BOTH")
	md := make(map[string]*market.Data, len(coins))
	for i := range fixture.Universe {
		c := &fixture.Universe[i]
		if c.Snapshot == nil {
			continue
		}
		md[c.Symbol] = &market.Data{Symbol: c.Symbol, CurrentPrice: c.CurrentPrice, PriceChange1h: c.Change1h, PriceChange4h: c.Change4h}
	}
	ctx := &Context{CurrentTime: "2026-07-26 20:07:24", CallCount: 1,
		Account: AccountInfo{TotalEquity: 1000, AvailableBalance: 1000}, CandidateCoins: coins, MarketDataMap: md}
	engine.FinalizeHunterV7Tiers(ctx)
	if len(ctx.HunterV7PromptFinal) == 0 {
		t.Fatal("no prompt-final tiers recorded")
	}
	if n := hunterV7GateVsPromptDivergences(ctx); n != 0 {
		t.Fatalf("%d gate vs prompt-final divergences on the golden round", n)
	}
}
