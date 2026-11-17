package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
)

// F02 / F05 / F25: under RiskControl.GeometryRepair = "reject" the Hunter v7
// open path never rewrites the LLM's price, stop or take-profit; misses are
// rejected. "repair" (and empty) keep the historical behaviour.

func rejectModeTrader(mode string) *AutoTrader {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	rc := &at.config.StrategyConfig.RiskControl
	rc.GeometryRepair = mode
	rc.MaxEntryPriceDeviationPct = 0.5
	rc.MinStopLossPriceMovePct = 2.0
	rc.MinRiskRewardRatio = 1.5
	rc.MaxTakeProfitPriceMovePct = 10
	rc.MinConfidence = 0
	return at
}

func rmDecision(price, sl, tp float64) *kernel.Decision {
	return &kernel.Decision{Symbol: "AAAUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 100,
		Price: price, StopLoss: sl, TakeProfit: tp, Confidence: 80}
}

func TestRejectModeRequiresEntryPrice(t *testing.T) {
	at := rejectModeTrader("reject")
	if err := at.enforceHunterV7EntryContract(rmDecision(0, 97, 106)); err == nil || !strings.Contains(err.Error(), "entry_price_missing") {
		t.Fatalf("missing price must be rejected, got %v", err)
	}
	if err := at.enforceHunterV7EntryContract(rmDecision(100, 97, 106)); err != nil {
		t.Fatalf("price present must pass, got %v", err)
	}
	if err := rejectModeTrader("repair").enforceHunterV7EntryContract(rmDecision(0, 97, 106)); err != nil {
		t.Fatalf("repair mode must not require price, got %v", err)
	}
}

func TestRejectModeDriftIsRejectedNotRefreshed(t *testing.T) {
	at := rejectModeTrader("reject")
	d := rmDecision(100, 96, 108)
	exec := 100.8 // 0.8% adverse drift > 0.5% max (repair mode would refresh within 0.6% band only; here no refresh at all)
	if at.repairHunterV7OpenDecision(d, exec, "long") {
		t.Fatalf("reject mode must not repair")
	}
	if d.Price != 100 {
		t.Fatalf("price rewritten to %v", d.Price)
	}
	if err := at.validateOpenDecision(d, exec, "long"); err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("drift beyond max must be rejected, got %v", err)
	}
	// Favourable drift is not silently accepted either (repair mode accepts up to 4%).
	d = rmDecision(100, 96, 108)
	if at.repairHunterV7OpenDecision(d, 98.5, "long") || d.Price != 100 {
		t.Fatalf("favourable drift must not refresh the price in reject mode")
	}
	if err := at.validateOpenDecision(d, 98.5, "long"); err == nil {
		t.Fatalf("1.5%% favourable drift beyond max must be rejected in reject mode")
	}
}

func TestRejectModeStopAndRRNotRepaired(t *testing.T) {
	at := rejectModeTrader("reject")
	// Stop 1.2% away (< 2.0% min): repair mode would widen it; reject mode rejects.
	d := rmDecision(100, 98.8, 106)
	if at.repairHunterV7OpenDecision(d, 100, "long") || d.StopLoss != 98.8 {
		t.Fatalf("stop must not be rewritten, got %v", d.StopLoss)
	}
	if err := at.validateOpenDecision(d, 100, "long"); err == nil || !strings.Contains(err.Error(), "Stop-loss distance") {
		t.Fatalf("tight stop must be rejected, got %v", err)
	}
	// Stop 2.8% away: repair mode would tighten it to min+0.10 (F25); reject mode keeps it.
	d = rmDecision(100, 97.2, 106)
	if at.repairHunterV7OpenDecision(d, 100, "long") || d.StopLoss != 97.2 {
		t.Fatalf("wide stop must not be tightened, got %v", d.StopLoss)
	}
	// RR 1.07 (< 1.5): repair mode would move TP; reject mode rejects.
	d = rmDecision(100, 97.2, 103)
	if at.repairHunterV7OpenDecision(d, 100, "long") || d.TakeProfit != 103 {
		t.Fatalf("take-profit must not be moved, got %v", d.TakeProfit)
	}
	if err := at.validateOpenDecision(d, 100, "long"); err == nil || !strings.Contains(err.Error(), "Risk-reward") {
		t.Fatalf("low RR must be rejected, got %v", err)
	}
}

func TestRepairModeUnchanged(t *testing.T) {
	at := rejectModeTrader("")
	d := rmDecision(100, 97.2, 106)
	if !at.repairHunterV7OpenDecision(d, 100, "long") {
		t.Fatalf("empty geometry_repair must keep the historical repair chain")
	}
	if d.StopLoss == 97.2 {
		t.Fatalf("historical behaviour tightens a 2.8%% stop to min+0.10; got unchanged")
	}
}
