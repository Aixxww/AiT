package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
)

// U2/P2a: the executor applies the hydrated T2 floor; without it the
// strategy floor applies as before.
func TestValidateOpenDecisionUsesHydratedT2Floor(t *testing.T) {
	at := testRiskAutoTrader() // MinRiskRewardRatio 2
	mk := func(minRR float64) *kernel.Decision {
		// entry 100, SL 98 (2%), TP 102.6 (2.6%) -> RR 1.3
		return &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 100,
			StopLoss: 98, TakeProfit: 102.6, Confidence: 80, HunterV7MinRR: minRR}
	}
	if err := at.validateOpenDecision(mk(1.2), 100, "long"); err != nil {
		t.Fatalf("T2 decision at RR 1.3 must pass its 1.2 floor: %v", err)
	}
	if err := at.validateOpenDecision(mk(0), 100, "long"); err == nil {
		t.Fatalf("decision without a hydrated floor must keep the strategy floor 2.0")
	}
	if got := at.decisionMinRiskRewardRatio(mk(0)); got != 2 {
		t.Fatalf("strategy floor = %.2f, want 2", got)
	}
}
