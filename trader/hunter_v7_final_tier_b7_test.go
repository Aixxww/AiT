package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// #86 (AUD-B7): the trader acts on the prompt-final tier. Fixtures copied
// from kernel/engine_prompt_compact_test.go (prompt-time promotion and
// demotion by the live price).

func b7LeaderCandidate(symbol string, price float64) kernel.CandidateCoin {
	return kernel.CandidateCoin{
		Symbol:             symbol,
		Sources:            []string{"hunter"},
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       73,
		V7SetupScore:       72,
		V7TimingScore:      80,
		V7RegimeFitScore:   75,
		V7RiskScore:        15,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7EntryMode:        "momentum_with_trailing_stop",
		V7Confidence:       "B",
		V7ReasonCodes:      []string{"strong_4h_momentum", "flow_taker_buy_sustained"},
		V7EntryZone:        local.V7PriceZone{Lower: price * 0.99, Upper: price * 1.01},
		V7Invalidation:     local.V7InvalidationRule{Price: price * 0.98},
		V7Targets:          []local.V7Target{{Price: price * 1.05}},
		V7PriceContext:     &local.V7PriceContext{Last: price},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.56, OIChange1h: 2.2, OIChange4h: 6.5},
	}
}

func b7MarketData(symbol string, price float64) *market.Data {
	return &market.Data{
		Symbol:       symbol,
		CurrentPrice: price,
		CurrentRSI7:  58,
		OpenInterest: &market.OIData{
			Latest:  1000000,
			Average: 950000,
		},
		FundingRate: 0.0001,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"15m": {
				Klines: []market.KlineBar{
					{Open: price * 0.98, High: price * 0.99, Low: price * 0.97, Close: price * 0.985, Volume: 100000},
					{Open: price * 0.985, High: price * 1.00, Low: price * 0.98, Close: price * 0.995, Volume: 110000},
					{Open: price * 0.995, High: price * 1.01, Low: price * 0.99, Close: price, Volume: 130000},
				},
				EMA20Values: []float64{price * 0.995},
				RSI7Values:  []float64{58},
				RSI14Values: []float64{55},
				ATR14:       price * 0.01,
			},
			"5m": {
				Klines: []market.KlineBar{
					{Open: price * 0.99, High: price * 1.00, Low: price * 0.985, Close: price * 0.995, Volume: 50000},
					{Open: price * 0.995, High: price * 1.005, Low: price * 0.99, Close: price, Volume: 60000},
				},
				RSI7Values:  []float64{57},
				RSI14Values: []float64{54},
				ATR14:       price * 0.005,
			},
		},
	}
}

func b7Trader() *AutoTrader {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.strategyEngine = kernel.NewStrategyEngine(at.config.StrategyConfig)
	return at
}

// AUD-B7 consequence (a): when flat and the assembly tier has no E/R
// candidate, the AI used to be skipped although the prompt would promote
// one. After PrepareHunterV7Tiers the skip check reads the final tier.
func TestB7_AISkipReadsPromptFinalTier(t *testing.T) {
	at := b7Trader()
	up := b7LeaderCandidate("LIVEINUSDT", 104)
	up.V7ExecutionTier, up.V7TierReason = "WATCH", "backend_rr_infeasible"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{up},
		MarketDataMap:  map[string]*market.Data{"LIVEINUSDT": b7MarketData("LIVEINUSDT", 100)},
	}
	if !at.shouldSkipHunterV7NoExecutable(&kernel.Context{CandidateCoins: []kernel.CandidateCoin{up}}, &store.DecisionRecord{}) {
		t.Fatal("fixture: the assembly tier alone must skip the AI")
	}
	if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
		t.Fatal(err)
	}
	if at.shouldSkipHunterV7NoExecutable(ctx, &store.DecisionRecord{}) {
		t.Fatalf("AI skipped although the prompt-final tier is %s", ctx.CandidateCoins[0].V7ExecutionTier)
	}
}

// AUD-B7 consequence (c): a candidate the prompt demoted to WATCH, but that
// was EXECUTABLE at assembly, used to pass the gate.
func TestB7_ExecutionGateReadsPromptFinalTier(t *testing.T) {
	at := b7Trader()
	down := b7LeaderCandidate("LIVEOUTUSDT", 100)
	down.V7ExecutionTier, down.V7TierReason = "EXECUTABLE", "cached_from_scoring"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{down},
		MarketDataMap:  map[string]*market.Data{"LIVEOUTUSDT": b7MarketData("LIVEOUTUSDT", 104)},
	}
	if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
		t.Fatal(err)
	}
	if got := ctx.CandidateCoins[0].V7ExecutionTier; got != "WATCH" {
		t.Fatalf("final tier %s, want WATCH", got)
	}
	err := at.validateHunterV7ExecutionGuard(ctx, &kernel.Decision{Symbol: "LIVEOUTUSDT", Action: "open_long", Price: 104})
	if err == nil || !strings.Contains(err.Error(), "execution_tier=WATCH") {
		t.Fatalf("demoted candidate not blocked by the tier gate: %v", err)
	}
}
