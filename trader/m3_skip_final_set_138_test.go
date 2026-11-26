package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// #138 M3: a candidate dropped from MarketDataMap is not shown to the LLM
// and keeps its assembly REVIEWABLE on ctx.CandidateCoins. Once the round
// is finalized the AI-skip check counts the prompt-final set only, so it no
// longer triggers a paid AI call on an E+R=0 prompt.
func TestM3138SkipCountsPromptFinalSet(t *testing.T) {
	at := b7Trader()
	shownWatch := b7LeaderCandidate("LIVEOUTUSDT", 100) // live price 104 above the zone -> WATCH
	dropped := b7LeaderCandidate("NODATAUSDT", 100)
	dropped.V7ExecutionTier, dropped.V7TierReason = "REVIEWABLE", "assembly"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{shownWatch, dropped},
		MarketDataMap:  map[string]*market.Data{"LIVEOUTUSDT": b7MarketData("LIVEOUTUSDT", 104)},
	}
	if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
		t.Fatal(err)
	}
	if len(ctx.HunterV7PromptFinal) != 1 || ctx.HunterV7PromptFinal[0].Tier != "WATCH" {
		t.Fatalf("fixture: prompt-final set = %+v, want the one shown row at WATCH", ctx.HunterV7PromptFinal)
	}
	if ctx.CandidateCoins[1].V7ExecutionTier != "REVIEWABLE" {
		t.Fatalf("fixture: the dropped row must keep its assembly REVIEWABLE, got %s", ctx.CandidateCoins[1].V7ExecutionTier)
	}
	record := &store.DecisionRecord{}
	if !at.shouldSkipHunterV7NoExecutable(ctx, record) {
		t.Fatal("AI not skipped although the prompt the LLM would see has E+R=0")
	}
	if !strings.Contains(record.Decisions[0].Reasoning, "not_shown=1") {
		t.Fatalf("skip reason does not report the unshown row: %q", record.Decisions[0].Reasoning)
	}
}
