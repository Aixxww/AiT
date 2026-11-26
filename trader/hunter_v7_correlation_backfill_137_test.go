package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
)

// #137: the execution guard never opens a correlation-backfilled row, even
// if some path left it tiered EXECUTABLE: the tag is wait_only and the
// guard's tag-action gate rejects it.
func TestCorrelationBackfill137ExecutionGuardBlocksOpen(t *testing.T) {
	for _, tier := range []string{"EXECUTABLE", "REVIEWABLE", "WATCH"} {
		t.Run(tier, func(t *testing.T) {
			at := testRiskAutoTrader()
			at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
			ctx := &kernel.Context{
				CandidateCoins: []kernel.CandidateCoin{{
					Symbol:          "FILLUSDT",
					Direction:       "LONG",
					V7SetupType:     "leader_momentum_long",
					V7ExecutionTier: tier,
					V7EntryZone:     local.V7PriceZone{Lower: 19.8, Upper: 20.2},
					V7RiskTags:      []string{local.V7CorrelationFloorContextTag},
				}},
				MarketDataMap: map[string]*market.Data{"FILLUSDT": {Symbol: "FILLUSDT", CurrentPrice: 20}},
			}
			decision := &kernel.Decision{Symbol: "FILLUSDT", Action: "open_long", Price: 20}
			err := at.validateHunterV7ExecutionGuard(ctx, decision)
			if err == nil {
				t.Fatal("a correlation_floor_context row passed the execution guard")
			}
			if tier != "WATCH" && !strings.Contains(err.Error(), local.V7CorrelationFloorContextTag) {
				t.Fatalf("blocked for the wrong reason: %v", err)
			}
		})
	}
}
