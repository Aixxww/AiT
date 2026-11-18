package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
)

// U6: a hydrated class-profile TP cap replaces the strategy cap in the TP1
// cap and the RR repair; without it the strategy cap applies as before.
func TestDecisionMaxTakeProfitUsesHydratedCap(t *testing.T) {
	at := t2StopTrader() // strategy cap 4.0
	if got := at.decisionMaxTakeProfitPriceMovePct(&kernel.Decision{}); got != at.maxTakeProfitPriceMovePct() {
		t.Fatalf("no hydrated cap = %v", got)
	}
	d := &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long", StopLoss: 98, TakeProfit: 110, HunterV7MaxTPPct: 6}
	if !at.capTakeProfitToTP1(d, 100, "long") || d.TakeProfit != 106 {
		t.Fatalf("TP capped to %v, want 106", d.TakeProfit)
	}
	n := &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long", StopLoss: 98, TakeProfit: 110}
	if !at.capTakeProfitToTP1(n, 100, "long") || n.TakeProfit != 104 {
		t.Fatalf("strategy cap TP = %v, want 104", n.TakeProfit)
	}
}
