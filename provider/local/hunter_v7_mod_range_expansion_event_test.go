package local

import "testing"

func TestRangeExpansionEventModuleCapturesLongAndShortEvents(t *testing.T) {
	mod := &rangeExpansionEventModule{}

	longCtx := &V7SymbolContext{
		Symbol:           "LONGUSDT",
		CurrentPrice:     100,
		Change1h:         3.5,
		Change24h:        28,
		Amplitude24h:     32,
		RangeExpansion1h: 2.6,
		Velocity15m:      1.8,
		Velocity5m:       0.9,
		VolumeBurst15m:   3.0,
		TakerBuy15m:      0.56,
		ATR15m:           1.2,
		ATR1h:            2.5,
		VWAP15m:          98,
		Snapshot:         &SymbolSnapshotData{Price: 100, QuoteVolume24h: 20_000_000},
		ExecutionContext: &V7ExecutionContext{DataQuality: "complete_for_execution"},
	}
	if !mod.Match(longCtx, V7RegimeMixed) {
		t.Fatal("expected long event to match")
	}
	longSig := mod.Score(longCtx, V7RegimeMixed)
	if longSig == nil || longSig.Direction != V7DirLong {
		t.Fatalf("expected long signal, got %+v", longSig)
	}
	if !containsV7String(longSig.ReasonCodes, "range_expansion_event") || !containsV7String(longSig.ReasonCodes, "event_directional_followthrough") {
		t.Fatalf("missing long event reasons: %+v", longSig.ReasonCodes)
	}

	shortCtx := *longCtx
	shortCtx.Symbol = "SHORTUSDT"
	shortCtx.Change1h = -4.2
	shortCtx.Change24h = -24
	shortCtx.Velocity15m = -2.1
	shortCtx.Velocity5m = -1.0
	shortCtx.TakerBuy15m = 0.44
	shortCtx.VWAP15m = 102
	shortSig := mod.Score(&shortCtx, V7RegimeTrendDown)
	if shortSig == nil || shortSig.Direction != V7DirShort {
		t.Fatalf("expected short signal, got %+v", shortSig)
	}
	if !containsV7String(shortSig.ReasonCodes, "event_breakdown_short") {
		t.Fatalf("missing short event reason: %+v", shortSig.ReasonCodes)
	}
	if len(shortSig.Targets) == 0 || shortSig.Targets[0].Price >= shortCtx.CurrentPrice {
		t.Fatalf("short targets not directional: %+v", shortSig.Targets)
	}
}

func TestRangeExpansionEventDowngradesWeakOIAndVolumeFollowthrough(t *testing.T) {
	mod := &rangeExpansionEventModule{}
	ctx := &V7SymbolContext{
		Symbol:           "COVERUSDT",
		CurrentPrice:     100,
		Change1h:         4.5,
		Change24h:        31,
		Amplitude24h:     36,
		RangeExpansion1h: 2.7,
		Velocity15m:      2.1,
		Velocity5m:       0.8,
		VolumeBurst15m:   0.7,
		TakerBuy15m:      0.55,
		ATR15m:           1.4,
		ATR1h:            2.4,
		VWAP15m:          98.5,
		Snapshot: &SymbolSnapshotData{
			Price:          100,
			QuoteVolume24h: 30_000_000,
			OIDelta1h:      -2.1,
			OIDelta4h:      -5.8,
		},
		ExecutionContext: &V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes: map[string]V7ExecutionTimeframeSummary{
				"15m": {Timeframe: "15m", VolumeVsAvg5: 0.7},
			},
		},
	}

	sig := mod.Score(ctx, V7RegimeMixed)
	if sig == nil {
		t.Fatal("expected range expansion signal")
	}
	if sig.ExecutionQuality != V7ExecChaseRisk {
		t.Fatalf("execution quality = %s, want chase_risk (tags %+v reasons %+v)", sig.ExecutionQuality, sig.RiskTags, sig.ReasonCodes)
	}
	for _, want := range []string{"short_covering_not_new_long_build", "range_expansion_low_volume_followthrough", "event_chase_risk"} {
		if !containsV7String(sig.RiskTags, want) {
			t.Fatalf("missing risk tag %q: %+v", want, sig.RiskTags)
		}
	}
	if !containsV7String(sig.ReasonCodes, "event_followthrough_quality_insufficient") {
		t.Fatalf("missing quality reason: %+v", sig.ReasonCodes)
	}
	if !containsV7String(sig.RequiredConfirms, "oi_delta_1h_positive_or_quote_volume_expands") {
		t.Fatalf("missing OI confirmation: %+v", sig.RequiredConfirms)
	}
}

func TestRangeExpansionEventLabelsRetestContinuationAndLateChase(t *testing.T) {
	mod := &rangeExpansionEventModule{}
	retestCtx := &V7SymbolContext{
		Symbol:           "RETESTUSDT",
		CurrentPrice:     100,
		Change1h:         2.5,
		Amplitude24h:     30,
		RangeExpansion1h: 2.4,
		Velocity15m:      1.4,
		Velocity5m:       0.4,
		VolumeBurst15m:   2.0,
		TakerBuy15m:      0.56,
		ATR15m:           1.0,
		VWAP15m:          99.7,
		Snapshot:         &SymbolSnapshotData{Price: 100, QuoteVolume24h: 20_000_000},
	}
	retestSig := mod.Score(retestCtx, V7RegimeMixed)
	if retestSig == nil || !containsV7String(retestSig.ReasonCodes, "range_expansion_retest") || !containsV7String(retestSig.ReasonCodes, "retest_confirmed") {
		t.Fatalf("missing retest labels: %+v", retestSig)
	}

	continuationCtx := *retestCtx
	continuationCtx.Symbol = "CONTUSDT"
	continuationCtx.CurrentPrice = 105
	continuationCtx.VWAP15m = 100
	continuationCtx.Change1h = 5
	continuationCtx.Velocity15m = 2.2
	continuationCtx.Velocity5m = 1.1
	continuationSig := mod.Score(&continuationCtx, V7RegimeTrendUp)
	if continuationSig == nil || !containsV7String(continuationSig.ReasonCodes, "range_expansion_continuation") {
		t.Fatalf("missing continuation label: %+v", continuationSig)
	}

	lateCtx := continuationCtx
	lateCtx.Symbol = "LATEUSDT"
	lateCtx.Amplitude24h = 60
	lateCtx.Velocity5m = 6
	lateSig := mod.Score(&lateCtx, V7RegimeManiaPump)
	if lateSig == nil || !containsV7String(lateSig.RiskTags, "late_event_chase") || lateSig.ExecutionQuality != V7ExecChaseRisk {
		t.Fatalf("missing late chase downgrade: %+v", lateSig)
	}
}

func TestRangeExpansionEventLabelsExhaustionAndMicroReversal(t *testing.T) {
	mod := &rangeExpansionEventModule{}
	ctx := &V7SymbolContext{
		Symbol:           "EXHAUSTUSDT",
		CurrentPrice:     100,
		Change1h:         5,
		Amplitude24h:     50,
		RangeExpansion1h: 2.7,
		Velocity15m:      2.0,
		Velocity5m:       -0.4,
		VolumeBurst15m:   2.0,
		TakerBuy15m:      0.50,
		ATR15m:           1.2,
		VWAP15m:          97,
		Snapshot:         &SymbolSnapshotData{Price: 100, QuoteVolume24h: 20_000_000},
	}
	sig := mod.Score(ctx, V7RegimeMixed)
	if sig == nil {
		t.Fatal("expected range expansion signal")
	}
	for _, want := range []string{"range_expansion_exhaustion", "velocity_decelerating", "micro_reversal_against_signal"} {
		if !containsV7String(sig.RiskTags, want) {
			t.Fatalf("missing risk tag %q: %+v", want, sig.RiskTags)
		}
	}
	if !containsV7String(sig.RequiredConfirms, "fresh_micro_confirmed") {
		t.Fatalf("missing fresh confirmation: %+v", sig.RequiredConfirms)
	}
}
