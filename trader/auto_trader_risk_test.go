package trader

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

type microRefreshTestTrader struct {
	Trader
	bids [][]float64
	asks [][]float64
	err  error
}

func (t *microRefreshTestTrader) PlaceLimitOrder(req *LimitOrderRequest) (*LimitOrderResult, error) {
	return nil, nil
}

func (t *microRefreshTestTrader) CancelOrder(symbol, orderID string) error {
	return nil
}

func (t *microRefreshTestTrader) GetOrderBook(symbol string, depth int) ([][]float64, [][]float64, error) {
	return t.bids, t.asks, t.err
}

func testRiskAutoTrader() *AutoTrader {
	return &AutoTrader{
		config: AutoTraderConfig{
			StrategyConfig: &store.StrategyConfig{
				RiskControl: store.RiskControlConfig{
					BTCETHMaxLeverage:  10,
					AltcoinMaxLeverage: 5,
					MinRiskRewardRatio: 2,
					MinConfidence:      70,
					MinPositionSize:    12,
					MaxPositions:       3,
					MaxMarginUsage:     0.9,
				},
			},
		},
	}
}

func TestValidateOpenDecisionAcceptsValidLong(t *testing.T) {
	at := testRiskAutoTrader()
	decision := &kernel.Decision{
		Symbol:          "SOLUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 100,
		StopLoss:        90,
		TakeProfit:      130,
		Confidence:      75,
	}

	if err := at.validateOpenDecision(decision, 100, "long"); err != nil {
		t.Fatalf("expected valid long decision, got error: %v", err)
	}
}

func TestValidateOpenDecisionRejectsEntryPriceDrift(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5
	decision := &kernel.Decision{
		Symbol:          "SOLUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 100,
		Price:           100,
		StopLoss:        90,
		TakeProfit:      130,
		Confidence:      75,
	}

	err := at.validateOpenDecision(decision, 100.8, "long")
	if err == nil || !strings.Contains(err.Error(), "Entry price drift") {
		t.Fatalf("expected entry price drift rejection, got: %v", err)
	}
}

func TestValidateOpenDecisionRequiresDecisionPriceForHunterV7(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	decision := &kernel.Decision{
		Symbol:          "SOLUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 100,
		StopLoss:        90,
		TakeProfit:      130,
		Confidence:      75,
	}

	err := at.validateOpenDecision(decision, 100, "long")
	if err == nil || !strings.Contains(err.Error(), "Decision price is required") {
		t.Fatalf("expected missing decision price rejection, got: %v", err)
	}
}

func TestValidateOpenDecisionRejectsLowConfidence(t *testing.T) {
	at := testRiskAutoTrader()
	decision := &kernel.Decision{
		Symbol:          "SOLUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 100,
		StopLoss:        90,
		TakeProfit:      130,
		Confidence:      60,
	}

	err := at.validateOpenDecision(decision, 100, "long")
	if err == nil || !strings.Contains(err.Error(), "Confidence") {
		t.Fatalf("expected confidence rejection, got: %v", err)
	}
}

func TestValidateOpenDecisionAllowsHunterV7MediumConfidence(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinConfidence = 75
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	decision := &kernel.Decision{
		Symbol:          "DASHUSDT",
		Action:          "open_short",
		Leverage:        5,
		PositionSizeUSD: 40,
		Price:           36.3,
		StopLoss:        37.03,
		TakeProfit:      34.85,
		Confidence:      70,
	}

	if err := at.validateOpenDecision(decision, 36.3, "short"); err != nil {
		t.Fatalf("expected Hunter v7 confidence 70 to pass, got: %v", err)
	}

	decision.Confidence = 69
	err := at.validateOpenDecision(decision, 36.3, "short")
	if err == nil || !strings.Contains(err.Error(), "Confidence") {
		t.Fatalf("expected Hunter v7 confidence 69 rejection, got: %v", err)
	}
}

func TestValidateOpenDecisionRejectsInvalidShortStops(t *testing.T) {
	at := testRiskAutoTrader()
	decision := &kernel.Decision{
		Symbol:          "SOLUSDT",
		Action:          "open_short",
		Leverage:        5,
		PositionSizeUSD: 100,
		StopLoss:        90,
		TakeProfit:      130,
		Confidence:      75,
	}

	err := at.validateOpenDecision(decision, 100, "short")
	if err == nil || !strings.Contains(err.Error(), "Invalid SHORT") {
		t.Fatalf("expected invalid short stop rejection, got: %v", err)
	}
}

func TestValidateOpenDecisionRejectsInvalidStopsWithoutStrategyConfig(t *testing.T) {
	at := &AutoTrader{}
	decision := &kernel.Decision{
		Symbol:          "SOLUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 100,
		StopLoss:        110,
		TakeProfit:      130,
		Confidence:      75,
	}

	err := at.validateOpenDecision(decision, 100, "long")
	if err == nil || !strings.Contains(err.Error(), "Invalid LONG") {
		t.Fatalf("expected invalid long stop rejection, got: %v", err)
	}
}

func TestValidateOpenDecisionRejectsLowRiskReward(t *testing.T) {
	at := testRiskAutoTrader()
	decision := &kernel.Decision{
		Symbol:          "BTCUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 100,
		StopLoss:        95,
		TakeProfit:      106,
		Confidence:      75,
	}

	err := at.validateOpenDecision(decision, 100, "long")
	if err == nil || !strings.Contains(err.Error(), "Risk-reward") {
		t.Fatalf("expected risk-reward rejection, got: %v", err)
	}
}

func TestValidateOpenDecisionRejectsTightHunterV7StopLoss(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	decision := &kernel.Decision{
		Symbol:          "HUSDT",
		Action:          "open_short",
		Leverage:        5,
		PositionSizeUSD: 60,
		Price:           0.63364,
		StopLoss:        0.645,
		TakeProfit:      0.50736,
		Confidence:      75,
	}

	err := at.validateOpenDecision(decision, 0.63578, "short")
	if err == nil || !strings.Contains(err.Error(), "Stop-loss distance") {
		t.Fatalf("expected tight stop-loss rejection, got: %v", err)
	}
}

func TestCapTakeProfitToTP1LongAndShort(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3

	longDecision := &kernel.Decision{
		Symbol:     "TAUSDT",
		StopLoss:   97,
		TakeProfit: 115,
	}
	if !at.capTakeProfitToTP1(longDecision, 100, "long") {
		t.Fatalf("expected long take profit to be capped")
	}
	if longDecision.TakeProfit != 103 {
		t.Fatalf("long TP = %v, want 103", longDecision.TakeProfit)
	}

	shortDecision := &kernel.Decision{
		Symbol:     "TAUSDT",
		StopLoss:   103,
		TakeProfit: 85,
	}
	if !at.capTakeProfitToTP1(shortDecision, 100, "short") {
		t.Fatalf("expected short take profit to be capped")
	}
	if shortDecision.TakeProfit != 97 {
		t.Fatalf("short TP = %v, want 97", shortDecision.TakeProfit)
	}
}

func TestRecordEffectiveOpenContractCapturesBackendRepairs(t *testing.T) {
	at := testRiskAutoTrader()
	decision := &kernel.Decision{
		Symbol:          "BTWUSDT",
		Action:          "open_long",
		PositionSizeUSD: 50.42,
		StopLoss:        0.06487,
		TakeProfit:      0.06906,
	}
	action := &store.DecisionAction{Action: "open_long", Symbol: "BTWUSDT"}

	at.recordEffectiveOpenContract(action, decision, 0.06634, 759, 155, true, true, "long")

	if action.EffectivePositionSizeUSD != 50.42 || action.EffectiveStopLoss != 0.06487 || action.EffectiveTakeProfit != 0.06906 {
		t.Fatalf("unexpected effective contract: %+v", action)
	}
	if !action.TPWasCapped || !action.PositionWasReduced {
		t.Fatalf("expected capped/reduced flags: %+v", action)
	}
	if action.RiskAtStopUSD <= 0 || action.RRAfterBackendRepair <= 0 {
		t.Fatalf("expected risk/RR metrics: %+v", action)
	}
	logLine := effectiveOpenContractLog(*action)
	for _, want := range []string{"effective_contract BTWUSDT open_long", "tp_capped=true", "position_reduced=true", "risk_at_sl=", "rr="} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("effective contract log missing %q: %s", want, logLine)
		}
	}
}

func TestHunterV7RaisesTakeProfitCapWhenRiskGeometryIsInfeasible(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3.0
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5

	if got := at.maxTakeProfitPriceMovePct(); got != 4.0 {
		t.Fatalf("effective max TP pct = %.2f, want 4.00", got)
	}
}

func TestRepairHunterV7OpenDecisionFixesBorderlineStopAndRR(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5

	decision := &kernel.Decision{
		Symbol:          "BSBUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 30,
		Price:           100,
		StopLoss:        98.01, // 1.99% risk, edge miss
		TakeProfit:      102.9, // RR below 1.5 after stop repair
		Confidence:      80,
	}

	if !at.repairHunterV7OpenDecision(decision, 100, "long") {
		t.Fatalf("expected preflight repair")
	}
	if err := at.validateOpenDecision(decision, 100, "long"); err != nil {
		t.Fatalf("expected repaired decision to validate, got: %v", err)
	}
}

func TestRepairHunterV7OpenDecisionRefreshesSmallEntryDrift(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5

	decision := &kernel.Decision{
		Symbol:          "EPICUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 30,
		Price:           100,
		StopLoss:        98,
		TakeProfit:      104,
		Confidence:      80,
	}

	if !at.repairHunterV7OpenDecision(decision, 100.55, "long") {
		t.Fatalf("expected small drift repair")
	}
	if decision.Price != 100.55 {
		t.Fatalf("decision price = %v, want refreshed execution price", decision.Price)
	}
	if err := at.validateOpenDecision(decision, 100.55, "long"); err != nil {
		t.Fatalf("expected repaired drift decision to validate, got: %v", err)
	}
}

func TestRepairHunterV7OpenDecisionFixesStopTightenedByAllowedDrift(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 4.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5

	decision := &kernel.Decision{
		Symbol:          "EPICUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 30,
		Price:           0.4812,
		StopLoss:        0.4711,
		TakeProfit:      0.4985,
		Confidence:      75,
	}
	currentPrice := 0.4794

	if !at.repairHunterV7OpenDecision(decision, currentPrice, "long") {
		t.Fatalf("expected stop repair for allowed entry-drift edge miss")
	}
	if stopPct := (currentPrice - decision.StopLoss) / currentPrice * 100; stopPct < 2.0 {
		t.Fatalf("repaired stop distance = %.3f%%, want >= 2.0%%", stopPct)
	}
	if err := at.validateOpenDecision(decision, currentPrice, "long"); err != nil {
		t.Fatalf("expected repaired EPIC-like decision to validate, got: %v", err)
	}
}

func TestRepairHunterV7OpenDecisionFixesActionableTightStop(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 4.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5

	decision := &kernel.Decision{
		Symbol:          "CLOUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 35.1,
		Price:           0.14895,
		StopLoss:        0.1452,
		TakeProfit:      0.1548,
		Confidence:      72,
	}
	currentPrice := 0.14708

	if !at.repairHunterV7OpenDecision(decision, currentPrice, "long") {
		t.Fatalf("expected actionable tight stop repair")
	}
	if stopPct := (currentPrice - decision.StopLoss) / currentPrice * 100; stopPct < 2.0 {
		t.Fatalf("repaired stop distance = %.3f%%, want >= 2.0%%", stopPct)
	}
	if err := at.validateOpenDecision(decision, currentPrice, "long"); err != nil {
		t.Fatalf("expected repaired CLO-like decision to validate, got: %v", err)
	}
}

func TestRepairHunterV7OpenDecisionDoesNotRepairLargeEntryDrift(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5

	decision := &kernel.Decision{
		Symbol:          "VELVETUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 30,
		Price:           100,
		StopLoss:        98,
		TakeProfit:      104,
		Confidence:      80,
	}

	if at.repairHunterV7OpenDecision(decision, 100.92, "long") {
		t.Fatalf("did not expect large drift repair")
	}
	err := at.validateOpenDecision(decision, 100.92, "long")
	if err == nil || !strings.Contains(err.Error(), "Entry price drift") {
		t.Fatalf("expected drift rejection after unrepaired decision, got: %v", err)
	}
}

func TestRepairHunterV7OpenDecisionRejectsFavorableDriftThroughStop(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5

	decision := &kernel.Decision{
		Symbol:          "EPICUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 55,
		Price:           0.7712,
		StopLoss:        0.7550,
		TakeProfit:      0.774592,
		Confidence:      80,
	}

	if !at.repairHunterV7OpenDecision(decision, 0.7448, "long") {
		t.Fatalf("expected favorable drift to refresh decision price")
	}
	if decision.Price != 0.7448 {
		t.Fatalf("decision price = %v, want live execution price", decision.Price)
	}
	if err := at.validateOpenDecision(decision, 0.7448, "long"); err == nil || !strings.Contains(err.Error(), "Invalid LONG SL/TP") {
		t.Fatalf("expected failed setup after live price crossed stop, got: %v", err)
	}
}

func TestRepairHunterV7OpenDecisionAllowsFavorableDriftWithValidLiveGeometry(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5

	decision := &kernel.Decision{
		Symbol:          "FLOORUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 55,
		Price:           100,
		StopLoss:        94.9,
		TakeProfit:      101,
		Confidence:      80,
	}

	if !at.repairHunterV7OpenDecision(decision, 97, "long") {
		t.Fatalf("expected favorable drift repair with valid live geometry")
	}
	if decision.Price != 97 {
		t.Fatalf("decision price = %v, want live execution price", decision.Price)
	}
	if err := at.validateOpenDecision(decision, 97, "long"); err != nil {
		t.Fatalf("expected valid favorable drift repair, got: %v", err)
	}
}

func TestEnforceSingleTradeLossLimitReducesPositionSize(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.RiskControl.MaxSingleTradeLossPct = 8
	decision := &kernel.Decision{
		Symbol:          "TAUSDT",
		PositionSizeUSD: 148,
		StopLoss:        96,
	}

	adjusted, capped, err := at.enforceSingleTradeLossLimit(decision, 100, 15, "long")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capped {
		t.Fatalf("expected position size to be capped")
	}
	// 8% of 15 USDT equity = 1.2 USDT max loss. With a 4% stop distance,
	// max notional is 1.2 / 0.04 = 30 USDT.
	if adjusted != 30 {
		t.Fatalf("adjusted = %v, want 30", adjusted)
	}
}

func TestHunterV7RepairBeforeSingleTradeLossKeepsSmallAccountExecutable(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 4.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5
	at.config.StrategyConfig.RiskControl.MaxSingleTradeLossPct = 6.0

	decision := &kernel.Decision{
		Symbol:          "ZESTUSDT",
		Action:          "open_long",
		Leverage:        5,
		PositionSizeUSD: 51.44,
		Price:           0.32663,
		StopLoss:        0.3185,
		TakeProfit:      0.3397,
		Confidence:      88,
	}
	currentPrice := 0.32663
	equity := 5.35414901

	if !at.repairHunterV7OpenDecision(decision, currentPrice, "long") {
		t.Fatalf("expected ZEST-like preflight repair")
	}
	adjusted, capped, err := at.enforceSingleTradeLossLimit(decision, currentPrice, equity, "long")
	if err != nil {
		t.Fatalf("unexpected loss-limit error: %v", err)
	}
	if !capped {
		t.Fatalf("expected position size cap after repaired stop")
	}
	if adjusted < 12.0 {
		t.Fatalf("adjusted size = %.2f, want >= 12.00 after repaired geometry", adjusted)
	}
	decision.PositionSizeUSD = adjusted
	if err := at.enforceMinPositionSize(decision.PositionSizeUSD); err != nil {
		t.Fatalf("expected repaired/capped size to pass min notional, got: %v", err)
	}
	if err := at.validateOpenDecision(decision, currentPrice, "long"); err != nil {
		t.Fatalf("expected repaired/capped ZEST-like decision to validate, got: %v", err)
	}
}

func TestHunterV7ExecutionGuardBlocksCFundingReversalShortWithBuildingOI(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:       "HUSDT",
				Direction:    "SHORT",
				V7SetupType:  "funding_reversal",
				V7Confidence: "C",
				V7EntryZone: local.V7PriceZone{
					Lower: 0.625,
					Upper: 0.645,
				},
				V7DerivativesCtx: &local.V7DerivativesContext{
					OIChange1h: 9.86,
					OIChange4h: 13.23,
				},
			},
		},
		MarketDataMap: map[string]*market.Data{
			"HUSDT": {Symbol: "HUSDT", CurrentPrice: 0.63578},
		},
	}
	decision := &kernel.Decision{
		Symbol: "HUSDT",
		Action: "open_short",
		Price:  0.63578,
	}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err == nil || !strings.Contains(err.Error(), "OI is still building") {
		t.Fatalf("expected building OI guard rejection, got: %v", err)
	}
}

func TestHunterV7ExecutionGuardAllowsFundingReversalShortNearZoneLower(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:       "HUSDT",
				Direction:    "SHORT",
				V7SetupType:  "funding_reversal",
				V7Confidence: "C",
				V7EntryZone: local.V7PriceZone{
					Lower: 0.625,
					Upper: 0.645,
				},
				V7DerivativesCtx: &local.V7DerivativesContext{
					OIChange1h: 0,
					OIChange4h: -1,
				},
			},
		},
		MarketDataMap: map[string]*market.Data{
			"HUSDT": {Symbol: "HUSDT", CurrentPrice: 0.633},
		},
	}
	decision := &kernel.Decision{
		Symbol: "HUSDT",
		Action: "open_short",
		Price:  0.633,
	}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err != nil {
		t.Fatalf("unexpected funding short zone guard rejection: %v", err)
	}
}

func TestHunterV7ExecutionGuardRejectsContextOnlyRequiredConfirmation(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:             "BTWUSDT",
				Direction:          "LONG",
				V7SetupType:        "whale_flow_reversal",
				V7RequiredConfirms: []string{"directional_15m_close_long"},
				V7ConfirmSummary: &local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: true,
					ContextChecks: []local.V7ConfirmationCheck{
						{Code: "directional_15m_close_long", Passed: false, Severity: local.V7ConfirmContext},
					},
				},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "BTWUSDT", Action: "open_long", Price: 1}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err == nil || !strings.Contains(err.Error(), "context-only") {
		t.Fatalf("expected context-only required confirmation rejection, got: %v", err)
	}
}

func TestHunterV7ExecutionGuardRejectsHighVolatilityTightStopCombo(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:      "SYNUSDT",
				Direction:   "LONG",
				V7SetupType: "whale_flow_reversal",
				V7RiskTags:  []string{"high_volatility", "execution_stop_tightened"},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "SYNUSDT", Action: "open_long", Price: 1}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err == nil || !strings.Contains(err.Error(), "wait_only_risk_combo") {
		t.Fatalf("expected high volatility tight-stop rejection, got: %v", err)
	}
}

func TestHunterV7ExecutionGuardReducesModerateLiquidityHighVolatilitySize(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:      "ALTUSDT",
				Direction:   "SHORT",
				V7SetupType: "range_expansion_event",
				V7RiskTags:  []string{"moderate_liquidity", "high_volatility"},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "ALTUSDT", Action: "open_short", Price: 1, PositionSizeUSD: 60}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err != nil {
		t.Fatalf("unexpected size-cap rejection: %v", err)
	}
	if decision.PositionSizeUSD != 20 {
		t.Fatalf("position size = %v, want 20", decision.PositionSizeUSD)
	}
}

func TestHunterV7ExecutionGuardAppliesRiskTagSizingAndLeverageCaps(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:          "RISKUSDT",
				Direction:       "LONG",
				V7SetupType:     "range_expansion_event",
				V7ExecutionTier: "REVIEWABLE",
				V7Confidence:    "B",
				V7RiskTags:      []string{"high_volatility", "range_expansion_exhaustion"},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "RISKUSDT", Action: "open_long", Price: 1, PositionSizeUSD: 100, Leverage: 20}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err != nil {
		t.Fatalf("unexpected risk sizing rejection: %v", err)
	}
	if decision.PositionSizeUSD != 50 {
		t.Fatalf("position size = %v, want 50", decision.PositionSizeUSD)
	}
	if decision.Leverage != 10 {
		t.Fatalf("leverage = %d, want 10", decision.Leverage)
	}
}

func TestHunterV7ExecutionGuardRejectsStaleRiskWithoutFreshMicroConfirmation(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:       "STALEUSDT",
				Direction:    "SHORT",
				V7SetupType:  "range_expansion_event",
				V7Confidence: "B",
				V7RiskTags:   []string{"stale_data_risk"},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "STALEUSDT", Action: "open_short", Price: 1, PositionSizeUSD: 50, Leverage: 10}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err == nil || !strings.Contains(err.Error(), "fresh_micro_confirmed_missing") {
		t.Fatalf("expected missing fresh micro confirmation rejection, got: %v", err)
	}
}

func TestHunterV7ExecutionGuardCanFreshConfirmStaleRiskWithOrderBook(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.trader = &microRefreshTestTrader{
		bids: [][]float64{{99.98, 10}},
		asks: [][]float64{{100.02, 10}},
	}
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:       "STALEUSDT",
				Direction:    "LONG",
				V7SetupType:  "trend_breakout_long",
				V7Confidence: "B",
				V7RiskTags:   []string{"stale_data_risk"},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "STALEUSDT", Action: "open_long", Price: 100, PositionSizeUSD: 50, Leverage: 5}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err != nil {
		t.Fatalf("unexpected stale risk rejection after orderbook refresh: %v", err)
	}
	if !containsStringValue(ctx.CandidateCoins[0].V7ReasonCodes, "fresh_micro_confirmed") {
		t.Fatalf("fresh_micro_confirmed not recorded: %+v", ctx.CandidateCoins[0].V7ReasonCodes)
	}
}

func TestHunterV7ExecutionGuardRequiresSelectedSignalIDWhenCandidateHasID(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:       "IDUSDT",
				Direction:    "LONG",
				V7SignalID:   "42|IDUSDT|LONG|trend_breakout_long",
				V7SetupType:  "trend_breakout_long",
				V7Confidence: "B",
			},
		},
	}
	decision := &kernel.Decision{Symbol: "IDUSDT", Action: "open_long", Price: 1, PositionSizeUSD: 50, Leverage: 5}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err == nil || !strings.Contains(err.Error(), "signal_contract_missing") {
		t.Fatalf("expected missing signal id rejection, got: %v", err)
	}

	decision.SelectedHunterV7SignalID = "42|IDUSDT|LONG|trend_breakout_long"
	if err := at.validateHunterV7ExecutionGuard(ctx, decision); err != nil {
		t.Fatalf("expected selected signal id to pass contract guard: %v", err)
	}
}

func TestHunterV7ReviewableConfirmationRequiresFreshRefresh(t *testing.T) {
	candidate := &kernel.CandidateCoin{
		Symbol:             "ZECUSDT",
		Direction:          "LONG",
		V7SignalID:         "1|ZECUSDT|LONG|trend_breakout_long",
		V7SetupType:        "trend_breakout_long",
		V7ExecutionTier:    "REVIEWABLE",
		V7ExecutionQuality: "near_confirm",
		V7RequiredConfirms: []string{"5m_or_15m_close_through_breakout_level"},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{{
				Code:     "5m_or_15m_close_through_breakout_level",
				Severity: local.V7ConfirmReviewWait,
			}},
		},
	}
	decision := &kernel.Decision{
		Action:                   "open_long",
		Symbol:                   "ZECUSDT",
		SelectedHunterV7SignalID: "1|ZECUSDT|LONG|trend_breakout_long",
	}

	if err := validateHunterV7RequiredConfirmations(candidate, decision); err == nil || !strings.Contains(err.Error(), "confirmation_missing") {
		t.Fatalf("missing fresh refresh should block confirmation, got %v", err)
	}

	candidate.V7ReasonCodes = []string{"fresh_rest_confirmed", "fresh_micro_confirmed"}
	if err := validateHunterV7RequiredConfirmations(candidate, decision); err != nil {
		t.Fatalf("fresh reviewable confirmation should pass, got %v", err)
	}
}

func TestHunterV7OpenNeedsGuardRefreshForReviewableLiveGap(t *testing.T) {
	candidate := &kernel.CandidateCoin{
		V7ExecutionTier:    "REVIEWABLE",
		V7ExecutionQuality: "near_confirm",
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{{
				Code:     "directional_15m_close_long",
				Severity: local.V7ConfirmReviewWait,
			}},
		},
	}
	if !hunterV7OpenNeedsGuardRefresh(candidate) {
		t.Fatal("reviewable live confirmation gap should require guard refresh")
	}

	candidate.V7ReasonCodes = []string{"fresh_rest_confirmed", "fresh_micro_confirmed"}
	if hunterV7OpenNeedsGuardRefresh(candidate) {
		t.Fatal("freshly confirmed reviewable gap should not require another guard refresh")
	}
}

func TestHunterV7ReviewableSummaryGapRequiresAndAcceptsFreshRefresh(t *testing.T) {
	candidate := &kernel.CandidateCoin{
		Symbol:          "GUAUSDT",
		Direction:       "SHORT",
		V7SetupType:     "range_expansion_event",
		V7ExecutionTier: "REVIEWABLE",
		V7RequiredConfirms: []string{
			"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
			"taker_buy_15m_lt_0_48",
			"no_new_high_after_rejection",
			"fresh_micro_confirmed",
		},
	}
	decision := &kernel.Decision{Action: "open_short", Symbol: "GUAUSDT"}

	if !hunterV7OpenNeedsGuardRefresh(candidate) {
		t.Fatal("reviewable summary gap should require guard refresh before open")
	}
	if err := validateHunterV7RequiredConfirmations(candidate, decision); err == nil || !strings.Contains(err.Error(), "confirmation_missing") {
		t.Fatalf("summary gap without fresh refresh should block, got %v", err)
	}

	candidate.V7ReasonCodes = []string{"fresh_rest_confirmed", "fresh_micro_confirmed"}
	if hunterV7OpenNeedsGuardRefresh(candidate) {
		t.Fatal("freshly confirmed summary gap should not require another guard refresh")
	}
	if err := validateHunterV7RequiredConfirmations(candidate, decision); err != nil {
		t.Fatalf("freshly confirmed summary gap should pass, got %v", err)
	}
}

func TestHunterV7RESTSnapshotValidationRejectsFlowFlip(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	candidate := &kernel.CandidateCoin{Symbol: "FLOWUSDT", V7SetupType: "range_expansion_event"}
	decision := &kernel.Decision{Symbol: "FLOWUSDT", Action: "open_long"}
	ss := &datafetch.SymbolSnapshot{
		Symbol: "FLOWUSDT",
		Price:  100,
		Klines: map[string][]datafetch.Kline{
			"1m": {
				{OpenTime: nowMs - 60_000, CloseTime: nowMs - 30_000, Open: 100, Close: 99.4, Volume: 100, TakerBuy: 20},
				{OpenTime: nowMs - 30_000, CloseTime: nowMs + 30_000, Open: 99.4, Close: 99.0, Volume: 100, TakerBuy: 20},
			},
		},
	}

	err := hunterV7ValidateRESTSnapshot(candidate, decision, "long", 100, ss)
	if err == nil || !strings.Contains(err.Error(), "flow_flip") {
		t.Fatalf("expected flow flip rejection, got: %v", err)
	}
}

func TestHunterV7RESTSnapshotValidationAcceptsFreshAlignedFlow(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	candidate := &kernel.CandidateCoin{Symbol: "FLOWUSDT", V7SetupType: "range_expansion_event"}
	decision := &kernel.Decision{Symbol: "FLOWUSDT", Action: "open_short"}
	ss := &datafetch.SymbolSnapshot{
		Symbol: "FLOWUSDT",
		Price:  100,
		Klines: map[string][]datafetch.Kline{
			"1m": {
				{OpenTime: nowMs - 60_000, CloseTime: nowMs - 30_000, Open: 100, Close: 99.8, Volume: 100, TakerBuy: 40},
				{OpenTime: nowMs - 30_000, CloseTime: nowMs + 30_000, Open: 99.8, Close: 99.6, Volume: 100, TakerBuy: 38},
			},
		},
	}

	if err := hunterV7ValidateRESTSnapshot(candidate, decision, "short", 100, ss); err != nil {
		t.Fatalf("expected fresh aligned REST snapshot to pass: %v", err)
	}
}

func TestHunterV7MicroRefreshRejectsWideSpreadAndDrift(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:          "WIDEUSDT",
				Direction:       "LONG",
				V7SetupType:     "range_expansion_event",
				V7ExecutionTier: "REVIEWABLE",
			},
		},
	}
	decision := &kernel.Decision{Symbol: "WIDEUSDT", Action: "open_long", Price: 100}

	at.trader = &microRefreshTestTrader{
		bids: [][]float64{{99.0, 10}},
		asks: [][]float64{{101.0, 10}},
	}
	err := at.validateHunterV7MicroRefresh(ctx, decision, "long", 100, 100)
	if err == nil || !strings.Contains(err.Error(), "spread") {
		t.Fatalf("expected spread rejection, got: %v", err)
	}

	at.trader = &microRefreshTestTrader{
		bids: [][]float64{{100.9, 10}},
		asks: [][]float64{{101.0, 10}},
	}
	err = at.validateHunterV7MicroRefresh(ctx, decision, "long", 101, 100)
	if err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("expected drift rejection, got: %v", err)
	}

	at.trader = &microRefreshTestTrader{err: fmt.Errorf("book unavailable")}
	err = at.validateHunterV7MicroRefresh(ctx, decision, "long", 100, 100)
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable orderbook rejection, got: %v", err)
	}
}

func TestHunterV7ExecutionGuardRejectsWhaleFlowLongAboveZoneLimit(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:      "RIFUSDT",
				Direction:   "LONG",
				V7SetupType: "whale_flow_reversal",
				V7EntryZone: local.V7PriceZone{Lower: 1.00, Upper: 1.10},
				V7DerivativesCtx: &local.V7DerivativesContext{
					TakerBuy15m: 0.58,
				},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "RIFUSDT", Action: "open_long", Price: 1.052}

	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err == nil || !strings.Contains(err.Error(), "exceeds 45") {
		t.Fatalf("expected whale flow zone rejection, got: %v", err)
	}
}

func TestHunterV7LiveOpenGuardRejectsShortReasoningDirectionConflict(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:      "BLESSUSDT",
				Direction:   "SHORT",
				V7SetupType: "range_expansion_event",
			},
		},
	}
	decision := &kernel.Decision{
		Symbol:    "BLESSUSDT",
		Action:    "open_short",
		Reasoning: "SHORT confirmation: 15m close above VWAP/EMA20 true, taker flow weak.",
	}

	err := at.validateHunterV7LiveOpenGuard(ctx, decision, "short", 0.0085, 0.0085)
	if err == nil || !strings.Contains(err.Error(), "direction_confirmation_conflict") {
		t.Fatalf("expected direction conflict rejection, got: %v", err)
	}
}

func TestHunterV7LiveOpenGuardRejectsRangeExpansionShortReboundFromDecisionPrice(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:      "LABUSDT",
				Direction:   "SHORT",
				V7SetupType: "range_expansion_event",
				V7DerivativesCtx: &local.V7DerivativesContext{
					TakerBuy15m: 0.42,
				},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "LABUSDT", Action: "open_short"}

	err := at.validateHunterV7LiveOpenGuard(ctx, decision, "short", 7.243, 7.146)
	if err == nil || !strings.Contains(err.Error(), "rebound_risk_wait") {
		t.Fatalf("expected rebound risk rejection, got: %v", err)
	}
}

func TestHunterV7LiveOpenGuardRejectsRangeExpansionShortDeepBelowEMA(t *testing.T) {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{
				Symbol:      "LABUSDT",
				Direction:   "SHORT",
				V7SetupType: "range_expansion_event",
				V7ExecutionContext: &local.V7ExecutionContext{
					Timeframes: map[string]local.V7ExecutionTimeframeSummary{
						"15m": {
							Timeframe:       "15m",
							CandleCount:     30,
							HasEMA20:        true,
							CloseVsEMA20Pct: -18.5,
						},
					},
				},
				V7DerivativesCtx: &local.V7DerivativesContext{
					TakerBuy15m: 0.42,
				},
			},
		},
	}
	decision := &kernel.Decision{Symbol: "LABUSDT", Action: "open_short"}

	err := at.validateHunterV7LiveOpenGuard(ctx, decision, "short", 7.14, 7.14)
	if err == nil || !strings.Contains(err.Error(), "late-short exhaustion") {
		t.Fatalf("expected late-short exhaustion rejection, got: %v", err)
	}
}

func TestCalculateLeveragedPnLPctLongAndShort(t *testing.T) {
	if got := calculateLeveragedPnLPct("long", 100, 101, 10); got != 10 {
		t.Fatalf("long pnl = %v, want 10", got)
	}
	if got := calculateLeveragedPnLPct("short", 100, 99, 10); got != 10 {
		t.Fatalf("short pnl = %v, want 10", got)
	}
}

func TestCalculateUnleveragedPnLPctLongAndShort(t *testing.T) {
	if got := calculateUnleveragedPnLPct("long", 100, 101); got != 1 {
		t.Fatalf("long pnl = %v, want 1", got)
	}
	if got := calculateUnleveragedPnLPct("short", 100, 99); got != 1 {
		t.Fatalf("short pnl = %v, want 1", got)
	}
}

func TestChoosePositionProtectionActionTP1ThenTP2(t *testing.T) {
	state := &positionProtectionState{InitialQuantity: 100, PeakPnLPct: 0}

	action, _ := choosePositionProtectionAction(state, protectorTP1PnLPct, protectorTP1MinPriceMovePct)
	if action != protectionTP1 {
		t.Fatalf("action = %q, want TP1", action)
	}

	state.TP0Done = true
	state.TP1Done = true
	action, _ = choosePositionProtectionAction(state, protectorTP2PnLPct, protectorTP2MinPriceMovePct)
	if action != protectionTP2 {
		t.Fatalf("action = %q, want TP2", action)
	}
}

func TestChoosePositionProtectionActionDoesNotTP1OnLeveragedMicroMove(t *testing.T) {
	state := &positionProtectionState{InitialQuantity: 100, PeakPnLPct: 0}

	action, _ := choosePositionProtectionAction(state, protectorTP1PnLPct, 0.30)
	if action != protectionNone {
		t.Fatalf("action = %q, want none for leveraged micro move", action)
	}
}

func TestChoosePositionProtectionActionTP0ForHighROE(t *testing.T) {
	state := &positionProtectionState{
		InitialQuantity: 100,
		PeakPnLPct:      16,
		OpenedAt:        time.Now().Add(-2 * time.Minute),
	}

	action, _ := choosePositionProtectionAction(state, 12, 0.6)
	if action != protectionTP0 {
		t.Fatalf("action = %q, want TP0 for high ROE even before raw TP1 move", action)
	}
}

func TestShouldTriggerPlannedTP0Price(t *testing.T) {
	longState := &positionProtectionState{PlannedTakeProfit: 101}
	if !shouldTriggerPlannedTP0Price("long", 101.2, 2.0, longState) {
		t.Fatal("expected long planned TP0 price trigger")
	}
	if shouldTriggerPlannedTP0Price("long", 101.2, -0.1, longState) {
		t.Fatal("expected no planned TP0 trigger when current PnL is not positive")
	}
	longState.TP0Done = true
	if shouldTriggerPlannedTP0Price("long", 101.2, 2.0, longState) {
		t.Fatal("expected no duplicate planned TP0 trigger")
	}

	shortState := &positionProtectionState{PlannedTakeProfit: 99}
	if !shouldTriggerPlannedTP0Price("short", 98.8, 2.0, shortState) {
		t.Fatal("expected short planned TP0 price trigger")
	}
	if shouldTriggerPlannedTP0Price("short", 99.2, 2.0, shortState) {
		t.Fatal("expected no short trigger before planned TP")
	}
}

func TestProtectionStateRememberPlannedTakeProfitKeepsNearestTarget(t *testing.T) {
	state := &positionProtectionState{}
	state.rememberPlannedTakeProfit("long", 105)
	state.rememberPlannedTakeProfit("long", 103)
	if state.PlannedTakeProfit != 103 {
		t.Fatalf("long planned TP = %v, want nearest 103", state.PlannedTakeProfit)
	}

	state = &positionProtectionState{}
	state.rememberPlannedTakeProfit("short", 95)
	state.rememberPlannedTakeProfit("short", 97)
	if state.PlannedTakeProfit != 97 {
		t.Fatalf("short planned TP = %v, want nearest 97", state.PlannedTakeProfit)
	}
}

func TestChoosePositionProtectionActionHardLossClose(t *testing.T) {
	state := &positionProtectionState{
		InitialQuantity: 100,
		OpenedAt:        time.Now().Add(-5 * time.Minute),
	}

	action, _ := choosePositionProtectionAction(state, -8.1, -0.81)
	if action != protectionHardLossClose {
		t.Fatalf("action = %q, want hard loss close for early loss", action)
	}

	state = &positionProtectionState{
		InitialQuantity: 100,
		OpenedAt:        time.Now().Add(-30 * time.Minute),
	}
	action, _ = choosePositionProtectionAction(state, -12.1, -1.21)
	if action != protectionHardLossClose {
		t.Fatalf("action = %q, want hard loss close for absolute loss", action)
	}
}

func TestChoosePositionProtectionActionTrailClose(t *testing.T) {
	state := &positionProtectionState{
		InitialQuantity: 100,
		TP0Done:         true,
		TP1Done:         true,
		TP2Done:         true,
		PeakPnLPct:      20,
	}

	action, drawdown := choosePositionProtectionAction(state, 12, 1.2)
	if action != protectionTrailClose {
		t.Fatalf("action = %q, want trail close", action)
	}
	if drawdown < protectorTrailDrawdownPct {
		t.Fatalf("drawdown = %v, want >= %v", drawdown, protectorTrailDrawdownPct)
	}
}

func TestChoosePositionProtectionActionPreTPGiveback(t *testing.T) {
	state := &positionProtectionState{
		InitialQuantity: 100,
		PeakPnLPct:      5,
		OpenedAt:        time.Now().Add(-5 * time.Minute),
	}

	action, drawdown := choosePositionProtectionAction(state, 1, 0.5)
	if action != protectionNone {
		t.Fatalf("action = %q, want none for young low-peak position", action)
	}
	if drawdown == 0 {
		t.Fatalf("expected drawdown to be calculated")
	}

	state = &positionProtectionState{
		InitialQuantity: 100,
		PeakPnLPct:      20,
		OpenedAt:        time.Now().Add(-25 * time.Minute),
	}

	action, drawdown = choosePositionProtectionAction(state, 3.5, protectorTP1MinPriceMovePct)
	if action != protectionGivebackClose {
		t.Fatalf("action = %q, want giveback close for mature high-peak position", action)
	}
	if drawdown < protectorPreTPGivebackPct {
		t.Fatalf("drawdown = %v, want >= %v", drawdown, protectorPreTPGivebackPct)
	}
}

func TestChoosePositionProtectionActionNearTP1Giveback(t *testing.T) {
	state := &positionProtectionState{
		InitialQuantity: 100,
		PeakPnLPct:      5.99,
		OpenedAt:        time.Now().Add(-40 * time.Minute),
	}

	action, drawdown := choosePositionProtectionAction(state, 3.0, protectorTP1MinPriceMovePct)
	if action != protectionGivebackClose {
		t.Fatalf("action = %q, want near-TP1 giveback close", action)
	}
	if drawdown < protectorNearTP1GivebackPct {
		t.Fatalf("drawdown = %v, want >= %v", drawdown, protectorNearTP1GivebackPct)
	}

	youngState := &positionProtectionState{
		InitialQuantity: 100,
		PeakPnLPct:      5.99,
		OpenedAt:        time.Now().Add(-5 * time.Minute),
	}
	action, _ = choosePositionProtectionAction(youngState, 3.0, protectorTP1MinPriceMovePct)
	if action != protectionNone {
		t.Fatalf("action = %q, want none before minimum hold duration", action)
	}
}

func TestChoosePositionProtectionActionNearTP1GivebackToLoss(t *testing.T) {
	state := &positionProtectionState{
		InitialQuantity: 117,
		PeakPnLPct:      5.99,
		OpenedAt:        time.Now().Add(-40 * time.Minute),
	}

	action, drawdown := choosePositionProtectionAction(state, -1.8, -0.18)
	if action != protectionGivebackClose {
		t.Fatalf("action = %q, want giveback close once 5%% peak crosses to loss", action)
	}
	if drawdown <= 100 {
		t.Fatalf("drawdown = %v, want >100 after peak profit crosses to loss", drawdown)
	}

	action, drawdown = choosePositionProtectionAction(state, -5.5, -0.55)
	if action != protectionGivebackClose {
		t.Fatalf("action = %q, want near-TP1 giveback close after material loss", action)
	}
	if drawdown <= 100 {
		t.Fatalf("drawdown = %v, want >100 after peak profit crosses to loss", drawdown)
	}
}

func TestChoosePositionProtectionActionNearTP1SecondChance(t *testing.T) {
	state := &positionProtectionState{
		InitialQuantity: 117,
		PeakPnLPct:      5.99,
		OpenedAt:        time.Now().Add(-50 * time.Minute),
	}

	action, _ := choosePositionProtectionAction(state, 5.54, protectorTP1MinPriceMovePct)
	if action != protectionGivebackClose {
		t.Fatalf("action = %q, want near-TP1 second-chance close", action)
	}

	youngState := &positionProtectionState{
		InitialQuantity: 117,
		PeakPnLPct:      5.99,
		OpenedAt:        time.Now().Add(-5 * time.Minute),
	}
	action, _ = choosePositionProtectionAction(youngState, 5.54, protectorTP1MinPriceMovePct)
	if action != protectionNone {
		t.Fatalf("action = %q, want none before minimum hold duration", action)
	}

	noPriorNearTP1 := &positionProtectionState{
		InitialQuantity: 117,
		PeakPnLPct:      5.54,
		OpenedAt:        time.Now().Add(-50 * time.Minute),
	}
	action, _ = choosePositionProtectionAction(noPriorNearTP1, 5.54, protectorTP1MinPriceMovePct)
	if action != protectionNone {
		t.Fatalf("action = %q, want none without prior near-TP1 peak", action)
	}
}

func TestEnsurePeakPnLCacheInitializedRestoresFromRecentPrompt(t *testing.T) {
	at := testRiskAutoTrader()
	at.id = "test-trader"
	st, err := store.New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatalf("store init failed: %v", err)
	}
	at.store = st

	openedAt := time.Now().Add(-40 * time.Minute).UTC()
	oldPrompt := "1. OPENUSDT SHORT | Entry 0.1997 Current 0.1980 | PnL+8.00% | Peak PnL8.00% | Leverage 10x"
	currentPrompt := "1. OPENUSDT SHORT | Entry 0.1997 Current 0.1999 | PnL-1.00% | Peak PnL5.99% | Leverage 10x"
	if err := st.Decision().LogDecision(&store.DecisionRecord{
		TraderID:    at.id,
		CycleNumber: 1,
		Timestamp:   openedAt.Add(-10 * time.Minute),
		Success:     true,
		InputPrompt: oldPrompt,
	}); err != nil {
		t.Fatalf("old decision log failed: %v", err)
	}
	if err := st.Decision().LogDecision(&store.DecisionRecord{
		TraderID:    at.id,
		CycleNumber: 2,
		Timestamp:   openedAt.Add(30 * time.Minute),
		Success:     true,
		InputPrompt: currentPrompt,
	}); err != nil {
		t.Fatalf("current decision log failed: %v", err)
	}

	peak := at.ensurePeakPnLCacheInitialized("OPENUSDT", "short", -1.0, openedAt)
	if peak != 5.99 {
		t.Fatalf("peak = %v, want restored 5.99", peak)
	}
	state := at.getOrCreateProtectionState("OPENUSDT_short", 117, -1.0, openedAt)
	if state.PeakPnLPct != 5.99 {
		t.Fatalf("state peak = %v, want restored 5.99", state.PeakPnLPct)
	}
}

func TestShouldUseFastProtectionInterval(t *testing.T) {
	state := &positionProtectionState{PeakPnLPct: 5}
	if shouldUseFastProtectionInterval(state, 4) {
		t.Fatalf("expected base interval before TP1 zone")
	}

	state.PeakPnLPct = protectorTP1PnLPct * protectorNearTP1PeakRatio
	if !shouldUseFastProtectionInterval(state, 4) {
		t.Fatalf("expected fast interval after peak reaches near-TP1 zone")
	}

	state = &positionProtectionState{TP1Done: true, PeakPnLPct: 2}
	if !shouldUseFastProtectionInterval(state, 1) {
		t.Fatalf("expected fast interval after TP1 is done")
	}
}

func TestDynamicProtectionStopUpdatesStopLossOnlyWhenProtective(t *testing.T) {
	ft := &contextTestTrader{}
	at := testRiskAutoTrader()
	at.trader = ft
	state := &positionProtectionState{
		InitialQuantity: 10,
		PeakPnLPct:      12,
		OpenedAt:        time.Now().Add(-45 * time.Minute),
	}

	if err := at.updateDynamicProtectionStop("BTCUSDT", "long", 10, 100, 106, 10, state); err != nil {
		t.Fatalf("dynamic stop update failed: %v", err)
	}
	if ft.cancelStopLossCalls != 1 || len(ft.stopLossCalls) != 1 {
		t.Fatalf("expected one stop-loss replacement, cancel=%d calls=%d", ft.cancelStopLossCalls, len(ft.stopLossCalls))
	}
	call := ft.stopLossCalls[0]
	if call.positionSide != "LONG" || call.stopPrice <= protectionBaseStopFromRisk("long", 100, 10) || call.stopPrice >= 106 {
		t.Fatalf("unexpected long stop call: %+v", call)
	}

	state.LastStopUpdateAt = time.Now().Add(-positionProtectorBaseInterval)
	if err := at.updateDynamicProtectionStop("BTCUSDT", "long", 10, 100, 102, 10, state); err != nil {
		t.Fatalf("second dynamic stop update failed: %v", err)
	}
	if len(ft.stopLossCalls) != 1 {
		t.Fatalf("expected no less-protective rewrite, calls=%d", len(ft.stopLossCalls))
	}

	state.LastStopUpdateAt = time.Now().Add(-positionProtectorBaseInterval)
	state.PeakPnLPct = 70
	if err := at.updateDynamicProtectionStop("BTCUSDT", "long", 10, 100, 106, 10, state); err != nil {
		t.Fatalf("trailing dynamic stop update failed: %v", err)
	}
	if len(ft.stopLossCalls) != 2 {
		t.Fatalf("expected trailing stop rewrite, calls=%d", len(ft.stopLossCalls))
	}
	if got := ft.stopLossCalls[1].stopPrice; got <= 100 || got >= 106 {
		t.Fatalf("trailing stop = %v, want locked above entry and below mark", got)
	}
}

func TestDynamicProtectionStopDoesNotLoosenRestoredActiveStopLoss(t *testing.T) {
	ft := &contextTestTrader{}
	at := testRiskAutoTrader()
	at.trader = ft
	state := &positionProtectionState{
		InitialQuantity: 759,
		PeakPnLPct:      0,
		ActiveStopLoss:  0.06487,
		OpenedAt:        time.Now().Add(-49 * time.Second),
	}

	err := at.updateDynamicProtectionStop("BTWUSDT", "long", 759, 0.06634, 0.06636, 10, state)
	if err != nil {
		t.Fatalf("dynamic stop update failed: %v", err)
	}
	if ft.cancelStopLossCalls != 0 || len(ft.stopLossCalls) != 0 {
		t.Fatalf("expected no stop-loss replacement when candidate loosens active SL, cancel=%d calls=%d",
			ft.cancelStopLossCalls, len(ft.stopLossCalls))
	}
	if state.ActiveStopLoss != 0.06487 {
		t.Fatalf("active stop changed to %v, want original 0.06487", state.ActiveStopLoss)
	}
}

func TestDynamicProtectionStopDelaysEarlyProfitFloorForSmallMove(t *testing.T) {
	ft := &contextTestTrader{}
	at := testRiskAutoTrader()
	at.trader = ft
	state := &positionProtectionState{
		InitialQuantity: 788,
		PeakPnLPct:      5.59,
		ActiveStopLoss:  0.01825548,
		OpenedAt:        time.Now().Add(-2 * time.Minute),
	}

	err := at.updateDynamicProtectionStop("DEEPUSDT", "short", 788, 0.01788, 0.01783, 20, state)
	if err != nil {
		t.Fatalf("dynamic stop update failed: %v", err)
	}
	if ft.cancelStopLossCalls != 0 || len(ft.stopLossCalls) != 0 {
		t.Fatalf("expected early small-move stop rewrite to be delayed, cancel=%d calls=%d",
			ft.cancelStopLossCalls, len(ft.stopLossCalls))
	}
	if state.DynamicStop != 0 || state.ActiveStopLoss != 0.01825548 {
		t.Fatalf("state changed unexpectedly: dynamic=%v active=%v", state.DynamicStop, state.ActiveStopLoss)
	}
}

func TestDynamicProtectionStopAllowsEarlyLockAfterTP0Peak(t *testing.T) {
	ft := &contextTestTrader{}
	at := testRiskAutoTrader()
	at.trader = ft
	state := &positionProtectionState{
		InitialQuantity: 788,
		PeakPnLPct:      protectorDynamicStopEarlyPeakPnLPct,
		ActiveStopLoss:  0.01825548,
		OpenedAt:        time.Now().Add(-3 * time.Minute),
	}

	err := at.updateDynamicProtectionStop("DEEPUSDT", "short", 788, 0.01788, 0.01778, 20, state)
	if err != nil {
		t.Fatalf("dynamic stop update failed: %v", err)
	}
	if len(ft.stopLossCalls) != 1 {
		t.Fatalf("expected early TP0-zone stop rewrite, got %d calls", len(ft.stopLossCalls))
	}
	got := ft.stopLossCalls[0].stopPrice
	if got >= 0.01788 || got <= 0.01778 {
		t.Fatalf("early profit-lock stop = %.8f, want between mark and entry", got)
	}
}

func TestDynamicProtectionStopLocksProfitFloorForShort(t *testing.T) {
	ft := &contextTestTrader{}
	at := testRiskAutoTrader()
	at.trader = ft
	state := &positionProtectionState{
		InitialQuantity: 100,
		PeakPnLPct:      16.15,
		OpenedAt:        time.Now().Add(-3 * time.Minute),
	}

	err := at.updateDynamicProtectionStop("IDOLUSDT", "short", 100, 0.01636, 0.01625, 20, state)
	if err != nil {
		t.Fatalf("dynamic stop update failed: %v", err)
	}
	if len(ft.stopLossCalls) != 1 {
		t.Fatalf("expected one stop-loss call, got %d", len(ft.stopLossCalls))
	}
	got := ft.stopLossCalls[0].stopPrice
	if got >= 0.01636 || got <= 0.01625 {
		t.Fatalf("short profit floor stop = %.8f, want between mark and entry", got)
	}
}

func TestProtectionStopHelpersShort(t *testing.T) {
	base := protectionBaseStopFromRisk("short", 100, 10)
	if base <= 100 {
		t.Fatalf("short base stop = %v, want above entry", base)
	}
	if !isMoreProtectiveStop("short", 101, 102) {
		t.Fatalf("short lower stop should be more protective")
	}
	if !isStopOnProtectiveSide("short", 101, 100) {
		t.Fatalf("short stop should be above mark price")
	}
	if isStopOnProtectiveSide("short", 99, 100) {
		t.Fatalf("short stop below mark should be rejected")
	}
	floor := protectionProfitFloorStop("short", 100, 20, 16)
	if floor >= 100 {
		t.Fatalf("short profit floor stop = %v, want below entry", floor)
	}
	earlyFloor := protectionProfitFloorStop("short", 0.01788, 20, 5.59)
	lockedPnLPct := ((0.01788 - earlyFloor) / 0.01788) * 20 * 100
	if lockedPnLPct < protectorProfitFloorNetBufferPnLPct {
		t.Fatalf("locked PnL = %.2f%%, want at least %.2f%%", lockedPnLPct, protectorProfitFloorNetBufferPnLPct)
	}
}

func TestProtectionCloseQuantityClosesAllWhenPartialTooSmall(t *testing.T) {
	at := testRiskAutoTrader()
	closeQty, closeAll := at.protectionCloseQuantity(10, 1, protectorTP1CloseRatio)
	if !closeAll {
		t.Fatalf("expected close all for small notional")
	}
	if closeQty != 10 {
		t.Fatalf("closeQty = %v, want full quantity", closeQty)
	}
}

func TestProtectionCloseQuantityPartial(t *testing.T) {
	at := testRiskAutoTrader()
	closeQty, closeAll := at.protectionCloseQuantity(100, 10, protectorTP1CloseRatio)
	if closeAll {
		t.Fatalf("expected partial close")
	}
	if closeQty != 40 {
		t.Fatalf("closeQty = %v, want 40", closeQty)
	}
}
