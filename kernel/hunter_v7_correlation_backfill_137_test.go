package kernel

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
)

// #137: a correlation-backfilled row may appear as context but never reaches
// REVIEWABLE/EXECUTABLE, never counts as open-review in the prompt, and its
// execution contract carries the tag as a hard block.

func backfill137Coins() []CandidateCoin {
	kept := hunterV7PromptReadyLeaderCandidate("KEEPUSDT", 2)
	backfilled := hunterV7PromptReadyLeaderCandidate("FILLUSDT", 20.05)
	backfilled.V7RiskTags = append(backfilled.V7RiskTags, local.V7CorrelationFloorContextTag)
	return []CandidateCoin{kept, backfilled}
}

func TestCorrelationBackfill137KernelTierCapsAtWatch(t *testing.T) {
	coins := backfill137Coins()
	keptTier, _ := classifyHunterV7CandidateTier(coins[0])
	if keptTier != "EXECUTABLE" && keptTier != "REVIEWABLE" {
		t.Fatalf("control: the untagged twin tiers %s; the fixture must be open-review without the tag", keptTier)
	}
	tier, reason := classifyHunterV7CandidateTier(coins[1])
	if tier != "WATCH" || reason != local.V7CorrelationFloorContextTag {
		t.Fatalf("backfilled tier = %s/%s, want WATCH/%s", tier, reason, local.V7CorrelationFloorContextTag)
	}
}

func TestCorrelationBackfill137PromptCountsAndContract(t *testing.T) {
	e := NewStrategyEngine(u11FixtureConfig("en"))
	coins := backfill137Coins()
	ctx := &Context{
		CandidateCoins: coins,
		MarketDataMap: map[string]*market.Data{
			coins[0].Symbol: hunterV7PromptTestMarketData(coins[0].Symbol, 2),
			coins[1].Symbol: hunterV7PromptTestMarketData(coins[1].Symbol, 20.05),
		},
	}
	items := e.finalizeHunterV7Tiers(ctx, map[string]bool{})
	exec, rev, watch, _ := hunterV7PromptTierCounts(items)
	if len(items) != 2 {
		t.Fatalf("items = %d, want both rows shown (the backfilled one as context)", len(items))
	}
	if exec+rev != 1 || watch != 1 {
		t.Fatalf("E=%d R=%d W=%d, want exactly the untagged row open-review and the backfilled row WATCH", exec, rev, watch)
	}
	for _, c := range ctx.CandidateCoins {
		if c.Symbol == "FILLUSDT" && c.V7ExecutionTier != "WATCH" {
			t.Fatalf("finalized candidate tier the trader reads = %s, want WATCH", c.V7ExecutionTier)
		}
	}
	for _, it := range items {
		if it.Coin.Symbol != "FILLUSDT" {
			continue
		}
		contract := buildHunterV7ExecutionContract(it.Coin)
		if contract.EntryPermission != "no_direct_open" || !strings.Contains(strings.Join(contract.HardBlocks, ","), local.V7CorrelationFloorContextTag) {
			t.Fatalf("contract = %+v, want no_direct_open with %s in hard_blocks", contract, local.V7CorrelationFloorContextTag)
		}
	}
}
