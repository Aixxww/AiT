package trader

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

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

func TestCalculateLeveragedPnLPctLongAndShort(t *testing.T) {
	if got := calculateLeveragedPnLPct("long", 100, 101, 10); got != 10 {
		t.Fatalf("long pnl = %v, want 10", got)
	}
	if got := calculateLeveragedPnLPct("short", 100, 99, 10); got != 10 {
		t.Fatalf("short pnl = %v, want 10", got)
	}
}

func TestChoosePositionProtectionActionTP1ThenTP2(t *testing.T) {
	state := &positionProtectionState{InitialQuantity: 100, PeakPnLPct: 0}

	action, _ := choosePositionProtectionAction(state, protectorTP1PnLPct)
	if action != protectionTP1 {
		t.Fatalf("action = %q, want TP1", action)
	}

	state.TP1Done = true
	action, _ = choosePositionProtectionAction(state, protectorTP2PnLPct)
	if action != protectionTP2 {
		t.Fatalf("action = %q, want TP2", action)
	}
}

func TestChoosePositionProtectionActionTrailClose(t *testing.T) {
	state := &positionProtectionState{
		InitialQuantity: 100,
		TP1Done:         true,
		TP2Done:         true,
		PeakPnLPct:      20,
	}

	action, drawdown := choosePositionProtectionAction(state, 12)
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

	action, drawdown := choosePositionProtectionAction(state, 1)
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

	action, drawdown = choosePositionProtectionAction(state, 3.5)
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

	action, drawdown := choosePositionProtectionAction(state, 3.0)
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
	action, _ = choosePositionProtectionAction(youngState, 3.0)
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

	action, drawdown := choosePositionProtectionAction(state, -1.8)
	if action != protectionNone {
		t.Fatalf("action = %q, want no mechanical close for small post-TP1 loss", action)
	}
	if drawdown <= 100 {
		t.Fatalf("drawdown = %v, want >100 after peak profit crosses to loss", drawdown)
	}

	action, drawdown = choosePositionProtectionAction(state, -5.5)
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

	action, _ := choosePositionProtectionAction(state, 5.54)
	if action != protectionGivebackClose {
		t.Fatalf("action = %q, want near-TP1 second-chance close", action)
	}

	youngState := &positionProtectionState{
		InitialQuantity: 117,
		PeakPnLPct:      5.99,
		OpenedAt:        time.Now().Add(-5 * time.Minute),
	}
	action, _ = choosePositionProtectionAction(youngState, 5.54)
	if action != protectionNone {
		t.Fatalf("action = %q, want none before minimum hold duration", action)
	}

	noPriorNearTP1 := &positionProtectionState{
		InitialQuantity: 117,
		PeakPnLPct:      5.54,
		OpenedAt:        time.Now().Add(-50 * time.Minute),
	}
	action, _ = choosePositionProtectionAction(noPriorNearTP1, 5.54)
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
