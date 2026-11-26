package local

import "testing"

// #133: checklist codes are recorded for the LLM and never become a
// missing check; machine codes are evaluated.
func TestConfirmRouting133_ChecklistNeverBlocks(t *testing.T) {
	sig := &V7SignalOutput{Symbol: "RUSDT", Direction: V7DirLong, SetupType: V7SetupRangeReversion,
		EntryZone: V7PriceZone{Lower: 99, Upper: 101}, Invalidation: V7InvalidationRule{Price: 97}, Targets: []V7Target{{Price: 106}},
		RequiredConfirms: []string{"enter_only_near_range_bottom", "stop_below_range_low", "unregistered_future_code", "live_price_in_entry_zone"}}
	ctx := &V7SymbolContext{CurrentPrice: 100}
	s := EvaluateV7Confirmations(sig, ctx, DefaultV7Config())
	want := []string{"enter_only_near_range_bottom", "stop_below_range_low", "unregistered_future_code"}
	if len(s.Checklist) != 3 || s.Checklist[0] != want[0] || s.Checklist[2] != want[2] {
		t.Fatalf("checklist %v, want %v", s.Checklist, want)
	}
	for _, list := range [][]V7ConfirmationCheck{s.MissingHard, s.MissingReview, s.ContextChecks} {
		for _, c := range list {
			for _, w := range want {
				if c.Code == w {
					t.Fatalf("checklist code %s became a check: %+v", w, c)
				}
			}
		}
	}
	sig.ConfirmSummary = &s
	for _, c := range v7MissingRequiredConfirmationCodes(sig) {
		if !V7ConfirmMachineVerifiable(c) {
			t.Fatalf("readiness counts checklist code %s as missing", c)
		}
	}
	// Without a summary, only machine codes are missing.
	sig.ConfirmSummary = nil
	if got := v7MissingRequiredConfirmationCodes(sig); len(got) != 1 || got[0] != "live_price_in_entry_zone" {
		t.Fatalf("missing without summary %v", got)
	}
}

// The codes that were in limbo and are now machine-verified evaluate.
func TestConfirmRouting133_NewMachineCodesEvaluate(t *testing.T) {
	ctx := &V7SymbolContext{CurrentPrice: 100, ExecutionContext: &V7ExecutionContext{Timeframes: map[string]V7ExecutionTimeframeSummary{
		"5m":  {Timeframe: "5m", CandleCount: 50, LastClose: 100, HasEMA20: true, CloseVsEMA20Pct: 0.3, NoNewLow: true, NoNewHigh: false},
		"15m": {Timeframe: "15m", CandleCount: 50, LastClose: 100, HasEMA20: true, CloseVsEMA20Pct: 0.2, HasVWAP20: true, VWAP20: 99.5, CloseVsVWAP20Pct: 0.5, NoNewLow: true},
	}}}
	long := &V7SignalOutput{Direction: V7DirLong, EntryZone: V7PriceZone{Lower: 99, Upper: 101}}
	short := &V7SignalOutput{Direction: V7DirShort, EntryZone: V7PriceZone{Lower: 99, Upper: 101}}
	for _, c := range []struct {
		code string
		sig  *V7SignalOutput
		pass bool
	}{
		{"5m_price_holds_ema20_or_trailing_support", long, true},
		{"5m_price_holds_ema20_or_trailing_support", short, false},
		{"price_holds_vwap_or_trailing_support", long, true},
		{"15m_close_above_vwap_or_ema20", long, true},
		{"15m_close_below_vwap_or_ema20", short, false},
		{"15m_close_below_vwap", short, false},
		{"15m_reclaim_vwap", long, true},
		{"no_new_low_after_rejection", long, true},
		{"no_new_low_after_reversal_signal", long, true},
		{"no_new_high_after_reversal_signal", short, false},
	} {
		check, ok := evaluateV7KnownConfirmation(c.code, c.sig, ctx)
		if !ok || check.Passed != c.pass {
			t.Errorf("%s (%s): ok=%v passed=%v, want %v", c.code, c.sig.Direction, ok, check.Passed, c.pass)
		}
	}
	// Missing data fails closed.
	if check, ok := evaluateV7KnownConfirmation("5m_price_holds_ema20_or_trailing_support", long, &V7SymbolContext{}); !ok || check.Passed {
		t.Fatalf("no 5m data must fail: %+v", check)
	}
	oi := &V7SymbolContext{Snapshot: &SymbolSnapshotData{OIDelta1h: 0.4}}
	if check, ok := evaluateV7KnownConfirmation("oi_1h_remains_positive_after_breakout", long, oi); !ok || !check.Passed {
		t.Fatalf("oi positive: %+v", check)
	}
	oi.Snapshot.OIDelta1h = -0.1
	if check, _ := evaluateV7KnownConfirmation("oi_1h_remains_positive_after_breakout", long, oi); check.Passed {
		t.Fatalf("oi negative must fail: %+v", check)
	}
}
