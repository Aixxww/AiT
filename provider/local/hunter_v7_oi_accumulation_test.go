package local

import "testing"

func TestAssessV7OIAccumulationDetectsInvisibleBuild(t *testing.T) {
	ctx := &V7SymbolContext{
		Symbol:            "SUIUSDT",
		CurrentPrice:      1.0,
		Change1h:          0.4,
		Change4h:          1.2,
		Change24h:         4,
		BBWidthPercentile: 8,
		VolumeBurst5m:     2.2,
		TakerBuy15m:       0.57,
		Snapshot: &SymbolSnapshotData{
			OIDelta1h:   6,
			OIDelta4h:   14,
			FundingRate: 0.0001,
			LSR:         1.02,
		},
	}

	ev := assessV7OIAccumulation(ctx)

	if !ev.InvisibleAccumulation {
		t.Fatalf("expected invisible accumulation, got %+v", ev)
	}
	if !ev.OI4hStealthBuild || !ev.OI1hConfirming || !ev.BBCompressed || !ev.VolumeBurstAtBreakout || !ev.TakerBuyRatioAbove055 {
		t.Fatalf("missing expected evidence flags: %+v", ev)
	}
}

func TestAssessV7OIAccumulationDoesNotFlagCrowdedMarkup(t *testing.T) {
	ctx := &V7SymbolContext{
		Symbol:            "LATEUSDT",
		CurrentPrice:      1.0,
		Change1h:          5,
		Change4h:          12,
		Change24h:         38,
		BBWidthPercentile: 35,
		VolumeBurst5m:     3,
		TakerBuy15m:       0.61,
		Snapshot: &SymbolSnapshotData{
			OIDelta1h:   9,
			OIDelta4h:   28,
			FundingRate: 0.0008,
			LSR:         1.8,
		},
	}

	ev := assessV7OIAccumulation(ctx)

	if ev.InvisibleAccumulation {
		t.Fatalf("crowded markup should not be invisible accumulation: %+v", ev)
	}
	if ev.FundingNotCrowded {
		t.Fatalf("funding should be crowded in fixture: %+v", ev)
	}
}

func TestVolatilitySqueezeAppliesUnifiedOIResonanceTags(t *testing.T) {
	mod := &volatilitySqueezeBreakoutModule{}
	ctx := &V7SymbolContext{
		Symbol:            "SUIUSDT",
		CurrentPrice:      1.0,
		Change1h:          0.4,
		Change4h:          1.2,
		Change24h:         4,
		ATR15m:            0.01,
		ATR1h:             0.01,
		ATR4h:             0.03,
		BBWidthPercentile: 4,
		BBUpper15m:        1.01,
		BBMiddle15m:       1.0,
		BBLower15m:        0.99,
		VolumeBurst5m:     2.2,
		TakerBuy15m:       0.57,
		Snapshot: &SymbolSnapshotData{
			OIDelta1h:      6,
			OIDelta4h:      14,
			FundingRate:    0.0001,
			LSR:            1.02,
			QuoteVolume24h: 80_000_000,
		},
	}

	sig := mod.Score(ctx, V7RegimeCompression)
	if sig == nil {
		t.Fatal("expected squeeze signal")
	}

	wantCodes := []string{
		"oi_invisible_accumulation_detected",
		"bb_compressed",
		"volume_burst_at_breakout",
		"taker_buy_ratio_above_0.55",
	}
	for _, code := range wantCodes {
		if !containsString(sig.ReasonCodes, code) {
			t.Fatalf("missing %s in reason codes: %+v", code, sig.ReasonCodes)
		}
	}

	DefaultResonanceScorer().ApplyToSignal(sig)
	if sig.ResonanceBonus < 17 {
		t.Fatalf("resonance bonus = %.1f, want stealth accumulation bonus", sig.ResonanceBonus)
	}
}

func TestWhaleFlowUsesUnifiedOIAccumulationTags(t *testing.T) {
	mod := &whaleFlowReversalModule{}
	ctx := &V7SymbolContext{
		Symbol:            "JTOUSDT",
		CurrentPrice:      2.0,
		Change1h:          -0.6,
		Change4h:          1.0,
		Change24h:         3,
		ATR15m:            0.02,
		ATR1h:             0.05,
		BBWidthPercentile: 12,
		VolumeBurst15m:    2.0,
		TakerBuy15m:       0.56,
		Snapshot: &SymbolSnapshotData{
			OIDelta1h:      8,
			OIDelta4h:      18,
			FundingRate:    0.00005,
			LSR:            1.04,
			LSRPrev:        0.98,
			QuoteVolume24h: 90_000_000,
		},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected whale flow signal")
	}
	if !containsString(sig.ReasonCodes, "oi_invisible_accumulation_detected") {
		t.Fatalf("missing invisible accumulation code: %+v", sig.ReasonCodes)
	}
	if !containsString(sig.ReasonCodes, "funding_not_crowded") {
		t.Fatalf("missing funding_not_crowded code: %+v", sig.ReasonCodes)
	}
	if sig.SetupScore <= 50 {
		t.Fatalf("setup score %.1f should include module and OI evidence", sig.SetupScore)
	}
}
