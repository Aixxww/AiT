package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
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
