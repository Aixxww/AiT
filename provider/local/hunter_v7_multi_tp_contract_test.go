package local

import (
	"math"
	"testing"
)

func TestApplyMultiTimeframeTPSyncsFirstTargetToTP1(t *testing.T) {
	ctx := &V7SymbolContext{
		CurrentPrice: 100,
		ATR15m:       1,
		ATR1h:        2,
		VWAP15m:      99,
		BBUpper15m:   102,
		BBMiddle15m:  100,
		BBLower15m:   98,
		High1h:       106,
		High4h:       108,
		Low1h:        96,
		Low4h:        94,
	}
	sig := &V7SignalOutput{
		Direction:    V7DirLong,
		MarketRegime: V7RegimeTrendUp,
		EntryZone:    V7PriceZone{Lower: 99, Upper: 101},
		Invalidation: V7InvalidationRule{Price: 97},
		Targets:      []V7Target{{Price: 101.5, Reason: "legacy_target"}},
	}

	ApplyMultiTimeframeTP(sig, ctx)

	if sig.TP1Price <= 0 {
		t.Fatal("expected tp1 price to be populated")
	}
	if len(sig.Targets) == 0 {
		t.Fatal("expected targets to be populated")
	}
	if math.Abs(sig.Targets[0].Price-sig.TP1Price) > 1e-9 {
		t.Fatalf("targets[0] %.8f != tp1_price %.8f", sig.Targets[0].Price, sig.TP1Price)
	}
}

func TestHighVelocityTakeProfitPlanClampsTP0AndKeepsTarget1AsTP1(t *testing.T) {
	ctx := &V7SymbolContext{
		CurrentPrice: 100,
		ATR15m:       0.35,
		ATR1h:        0.8,
		VWAP15m:      100.2,
		BBUpper15m:   100.6,
		BBMiddle15m:  100,
		BBLower15m:   99.4,
		High1h:       106,
		High4h:       108,
		Low1h:        98,
		Low4h:        96,
	}
	sig := &V7SignalOutput{
		Direction:    V7DirLong,
		SetupType:    V7SetupRangeExpansion,
		MarketRegime: V7RegimeTrendUp,
		EntryZone:    V7PriceZone{Lower: 99.5, Upper: 100.5},
		Invalidation: V7InvalidationRule{Price: 98},
		Targets:      []V7Target{{Price: 100.8, Reason: "legacy_target"}},
	}

	ApplyMultiTimeframeTP(sig, ctx)

	if sig.TPPlan == nil {
		t.Fatal("expected TP execution plan")
	}
	entry := 100.0
	dist := math.Abs((sig.TP0Price - entry) / entry * 100)
	if dist < 1.2 || dist > 2.5 {
		t.Fatalf("tp0 distance = %.4f%%, want within 1.2%%-2.5%%", dist)
	}
	if sig.TPPlan.TP0ReducePctMin != 30 || sig.TPPlan.TP0ReducePctMax != 50 || !sig.TPPlan.MoveStopToBreakeven {
		t.Fatalf("unexpected tp plan: %+v", sig.TPPlan)
	}
	if len(sig.Targets) == 0 || math.Abs(sig.Targets[0].Price-sig.TP1Price) > 1e-9 {
		t.Fatalf("targets[0] should remain TP1, target=%+v tp1=%.8f", sig.Targets, sig.TP1Price)
	}
}
