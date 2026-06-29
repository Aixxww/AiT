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
