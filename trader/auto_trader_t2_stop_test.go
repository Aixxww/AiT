package trader

import (
	"math"
	"testing"

	"github.com/Aixxww/AiT/kernel"
)

func t2StopTrader() *AutoTrader {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.5
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 4.0
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5
	return at
}

func t2StopDecision(stop, tp, minStop float64) *kernel.Decision {
	return &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 30, Price: 100,
		StopLoss: stop, TakeProfit: tp, Confidence: 80, HunterV7MinRR: 1.2, HunterV7MinStopPct: minStop}
}

// U4/P2b: validateOpenDecision applies the hydrated T2 stop floor; without
// it the strategy floor applies as before.
func TestValidateOpenDecisionUsesHydratedT2StopFloor(t *testing.T) {
	at := t2StopTrader()
	if err := at.validateOpenDecision(t2StopDecision(99, 101.8, 0.5), 100, "long"); err != nil {
		t.Fatalf("T2 1%% stop must pass the 0.5%% floor: %v", err)
	}
	if err := at.validateOpenDecision(t2StopDecision(99, 101.8, 0), 100, "long"); err == nil {
		t.Fatalf("decision without a hydrated floor must keep the 2%% strategy floor")
	}
	if err := at.validateOpenDecision(t2StopDecision(99.7, 101, 0.5), 100, "long"); err == nil {
		t.Fatalf("T2 0.3%% stop must fail the 0.5%% floor")
	}
	if got := at.decisionMinStopLossPriceMovePct(t2StopDecision(99, 101.8, 0)); got != 2.0 {
		t.Fatalf("strategy floor = %.2f, want 2", got)
	}
}

// The repair path widens edge misses to the T2 floor, leaves a T2 stop
// between the T2 and native floors alone, and keeps the native narrowing.
func TestRepairHunterV7StopUsesT2Floor(t *testing.T) {
	at := t2StopTrader()

	d := t2StopDecision(99, 101.8, 0.5) // 1% stop
	at.repairHunterV7StopLossDistance(d, 100, "long")
	if d.StopLoss != 99 {
		t.Fatalf("T2 1%% stop must not be widened, got %.4f", d.StopLoss)
	}
	n := t2StopDecision(99, 101.8, 0) // native: widened to 2.1%
	n.HunterV7MinRR = 0
	at.repairHunterV7StopLossDistance(n, 100, "long")
	if math.Abs(n.StopLoss-97.9) > 1e-9 {
		t.Fatalf("native 1%% stop must be widened to 2.1%%, got %.4f", n.StopLoss)
	}

	e := t2StopDecision(99.7, 101, 0.5) // 0.3% stop: widened to 0.6%
	if !at.repairHunterV7StopLossDistance(e, 100, "long") || math.Abs(e.StopLoss-99.4) > 1e-9 {
		t.Fatalf("T2 0.3%% stop must be widened to 0.6%%, got %.4f", e.StopLoss)
	}

	w := t2StopDecision(97.5, 106, 0.5) // 2.5% stop: native narrowing to 2.1%, as before U4
	if !at.repairHunterV7StopLossDistance(w, 100, "long") || math.Abs(w.StopLoss-97.9) > 1e-9 {
		t.Fatalf("wide T2 stop must keep the native narrowing, got %.4f", w.StopLoss)
	}
	m := t2StopDecision(98.5, 103, 0.5) // 1.5% stop: inside both bands, untouched
	if at.repairHunterV7StopLossDistance(m, 100, "long") || m.StopLoss != 98.5 {
		t.Fatalf("T2 1.5%% stop must be untouched, got %.4f", m.StopLoss)
	}
}
