package local

import "testing"

func TestDisplacementUsesLaterTargetForRRRepair(t *testing.T) {
	mod := &displacementMomentumLongModule{}
	ctx := &V7SymbolContext{
		Symbol:            "MOVEUSDT",
		CurrentPrice:      100,
		Change1h:          4.5,
		Change4h:          7.5,
		Change24h:         18,
		ATR15m:            1,
		ATR1h:             3,
		ATR4h:             7,
		RangeExpansion1h:  2.8,
		TakerBuy15m:       0.56,
		VWAP15m:           99.4,
		High1h:            101,
		Low1h:             96,
		High4h:            103,
		VolumeBurst15m:    2,
		BBWidthPercentile: 40,
		Snapshot: &SymbolSnapshotData{
			Price:          100,
			QuoteVolume24h: 80_000_000,
			TradeCount24h:  120_000,
			OI:             12_000_000,
			OIDelta1h:      4,
			OIDelta4h:      10,
			FundingRate:    0.0001,
			LSR:            1.1,
			Volume24h:      10_000_000,
		},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected displacement signal")
	}
	if containsString(sig.RiskTags, "displacement_rr_insufficient") {
		t.Fatalf("unexpected rr rejection after repair: tags=%+v targets=%+v invalidation=%+v", sig.RiskTags, sig.Targets, sig.Invalidation)
	}
	if !containsString(sig.RiskTags, "displacement_rr_repaired") {
		t.Fatalf("missing repair tag: %+v", sig.RiskTags)
	}
	if !containsString(sig.ReasonCodes, "displacement_extension_rr_valid") {
		t.Fatalf("missing repair reason: %+v", sig.ReasonCodes)
	}
	rr, ok := displacementBestRR(sig, ctx)
	if !ok || rr < 1.5 {
		t.Fatalf("best rr %.2f ok=%v, want >= 1.5", rr, ok)
	}
	firstRR := displacementTargetRR(sig, ctx, sig.Targets[0].Price)
	if firstRR >= 1.5 {
		t.Fatalf("test fixture should require later target, first rr %.2f", firstRR)
	}
}

func TestDisplacementStillRejectsInfeasibleGeometry(t *testing.T) {
	ctx := &V7SymbolContext{
		Symbol:       "BADUSDT",
		CurrentPrice: 100,
	}
	sig := &V7SignalOutput{
		Symbol:       "BADUSDT",
		Direction:    V7DirLong,
		SetupType:    V7SetupDisplacementLong,
		EntryZone:    V7PriceZone{Lower: 99, Upper: 101},
		Invalidation: V7InvalidationRule{Price: 95},
		Targets:      []V7Target{{Price: 103}, {Price: 104}},
	}

	if displacementRRValid(sig, ctx) {
		t.Fatal("expected insufficient RR to remain invalid")
	}
}

func TestDisplacementFinalizeKeepsBackendRRHardGate(t *testing.T) {
	mod := &displacementMomentumLongModule{}
	ctx := &V7SymbolContext{
		Symbol:           "READYMOVEUSDT",
		CurrentPrice:     100,
		Change1h:         5,
		Change4h:         9,
		Change24h:        16,
		ATR15m:           1,
		ATR1h:            3,
		ATR4h:            7,
		RangeExpansion1h: 3,
		TakerBuy15m:      0.58,
		VWAP15m:          99.5,
		High1h:           101,
		Low1h:            96,
		High4h:           104,
		Snapshot: &SymbolSnapshotData{
			Price:          100,
			QuoteVolume24h: 100_000_000,
			TradeCount24h:  150_000,
			OI:             15_000_000,
			OIDelta1h:      6,
			OIDelta4h:      12,
			FundingRate:    0.0001,
			LSR:            1.05,
			Volume24h:      12_000_000,
		},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected displacement signal")
	}
	sig.SetupScore = 85
	sig.TimingScore = 75
	sig.RiskScore = 15
	sig.LiquidityScore = 90

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality == V7ExecInvalidRR {
		t.Fatalf("execution quality = %s, tags=%+v targets=%+v invalidation=%+v", sig.ExecutionQuality, sig.RiskTags, sig.Targets, sig.Invalidation)
	}
	rr, ok := v7SignalRiskReward(sig, ctx.CurrentPrice)
	if !ok || rr < 1.5 {
		t.Fatalf("backend rr %.2f ok=%v, want >= 1.5", rr, ok)
	}
}
