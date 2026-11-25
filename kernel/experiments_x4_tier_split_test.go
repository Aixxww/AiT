package kernel

// Experiment X4 (issue #80, audit docs/audit/adversarial-review-20261124.md §6):
// tier split-brain rate. Measurement only: no assertion about desired
// behaviour, no production code touched. Skipped unless AIT_EXPERIMENTS=1.
//
// The audit asks for the rate "over the stored rounds". The stored Tier A
// rerun rounds are not reachable (see the design doc, X1 blocker), so this
// measures the single frozen round universe-20260726 (N=1 round) through the
// same path as TestHunterV7GoldenPrompt. The fixture's market data carries
// only price and 1h/4h change (no klines, no derivatives context), so
// prompt-time readiness demotions that depend on that data are part of what
// is measured and must be read as an upper bound for a fully populated round.
//
// Run: AIT_EXPERIMENTS=1 go test ./kernel -run TestExperimentX4 -v -count=1

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

func TestExperimentX4_TierSplitBrainOnGoldenRound(t *testing.T) {
	if os.Getenv("AIT_EXPERIMENTS") != "1" {
		t.Skip("measurement-only experiment; set AIT_EXPERIMENTS=1 to run")
	}
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

	marketData := make(map[string]*market.Data, len(coins))
	for i := range fixture.Universe {
		c := &fixture.Universe[i]
		if c.Snapshot == nil {
			continue
		}
		marketData[c.Symbol] = &market.Data{Symbol: c.Symbol, CurrentPrice: c.CurrentPrice,
			PriceChange1h: c.Change1h, PriceChange4h: c.Change4h}
	}
	ctx := &Context{CurrentTime: "2026-07-26 20:07:24", CallCount: 1,
		Account: AccountInfo{TotalEquity: 1000, AvailableBalance: 1000}, CandidateCoins: coins, MarketDataMap: marketData}
	_ = engine.BuildUserPrompt(ctx)

	assembled := map[string]int{}
	for _, c := range coins {
		assembled[c.V7ExecutionTier]++
	}
	final := map[string]int{}
	transitions := map[string]int{}
	diverged := 0
	gateFlips := 0 // assembly E/R vs final E/R disagree: what the trader gate would decide differently
	isER := func(tier string) bool { return tier == "EXECUTABLE" || tier == "REVIEWABLE" }
	for _, f := range ctx.HunterV7PromptFinal {
		final[f.Tier]++
		if f.Tier != f.AssembledTier {
			diverged++
			transitions[f.AssembledTier+"->"+f.Tier]++
			t.Logf("X4 diverged: %s %s assembled=%s(%s) final=%s(%s)", f.Symbol, f.SetupType, f.AssembledTier, f.AssembledReason, f.Tier, f.TierReason)
		}
		if isER(f.Tier) != isER(f.AssembledTier) {
			gateFlips++
		}
		t.Logf("X4 item: %s %s assembled=%s final=%s reason=%s", f.Symbol, f.SetupType, f.AssembledTier, f.Tier, f.TierReason)
	}
	keys := make([]string, 0, len(transitions))
	for k := range transitions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Logf("X4 [N=1 round, golden universe-20260726, sparse market data] router_out=%d candidates=%d prompt_items=%d", len(result.OutputSignals), len(coins), len(ctx.HunterV7PromptFinal))
	t.Logf("X4 assembled tiers=%v final tiers=%v", assembled, final)
	t.Logf("X4 diverged=%d/%d gate_flips(E/R membership)=%d", diverged, len(ctx.HunterV7PromptFinal), gateFlips)
	// #86 regression metric: the tier the gate reads (ctx.CandidateCoins)
	// equals the prompt-final tier for every candidate the LLM was shown.
	if n := hunterV7GateVsPromptDivergences(ctx); n != 0 {
		t.Fatalf("X4: %d candidates where the gate tier differs from the prompt-final tier", n)
	}
	for _, k := range keys {
		t.Logf("X4 transition %s: %d", k, transitions[k])
	}

	// Sensitivity (SYNTHETIC perturbation): the prompt re-tiers at the live
	// price, so move every live price by a fixed percentage between assembly
	// and prompt and count how many candidates the two verdicts split on.
	for _, pct := range []float64{-2, -1, -0.5, 0.5, 1, 2} {
		md := make(map[string]*market.Data, len(marketData))
		for sym, d := range marketData {
			cp := *d
			cp.CurrentPrice = d.CurrentPrice * (1 + pct/100)
			md[sym] = &cp
		}
		pctx := &Context{CurrentTime: ctx.CurrentTime, CallCount: 1, Account: ctx.Account,
			CandidateCoins: engine.hunterV7SignalsToCandidateCoins(result.OutputSignals, "BOTH"), MarketDataMap: md}
		_ = engine.BuildUserPrompt(pctx)
		if n := hunterV7GateVsPromptDivergences(pctx); n != 0 {
			t.Fatalf("X4 [%+.1f%%]: %d gate vs prompt-final divergences", pct, n)
		}
		div, flips := 0, 0
		tr := map[string]int{}
		for _, f := range pctx.HunterV7PromptFinal {
			if f.Tier != f.AssembledTier {
				div++
				tr[f.AssembledTier+"->"+f.Tier]++
			}
			if isER(f.Tier) != isER(f.AssembledTier) {
				flips++
			}
		}
		t.Logf("X4 sensitivity [SYNTHETIC live price %+.1f%%]: diverged=%d/%d gate_flips=%d transitions=%v", pct, div, len(pctx.HunterV7PromptFinal), flips, tr)
	}
}
