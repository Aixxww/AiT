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
		Leverage:        10,
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

func TestHunterV7ExecutionGuardBlocksCFundingReversalShortNearZoneLower(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "zone_pos") {
		t.Fatalf("expected zone position guard rejection, got: %v", err)
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

func TestShouldUseFastProtectionInterval(t *testing.T) {
	state := &positionProtectionState{PeakPnLPct: 5}
	if shouldUseFastProtectionInterval(state, 4) {
		t.Fatalf("expected base interval before TP1 zone")
	}

	state.PeakPnLPct = protectorTP1PnLPct
	if !shouldUseFastProtectionInterval(state, 4) {
		t.Fatalf("expected fast interval after peak reaches TP1")
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
