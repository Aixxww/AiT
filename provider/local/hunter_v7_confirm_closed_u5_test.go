package local

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
)

// Unit U5 (M2b): scoring-side closed-bar confirmation and bar-evidence tests.
// Verifies that buildSymbolContext populates ClosedConfirmTimeframes strictly
// from closed bars WITHOUT altering feature_bars default or non-confirmation
// features, and that EvaluateV7Confirmations produces consistent closed-bar
// verdicts and auditable BarEvidence.

func u5DatafetchBars(iv time.Duration, at time.Time, nClosed int, closedPrice, formingClose float64) []datafetch.Kline {
	formingOpen := at.Truncate(iv)
	out := make([]datafetch.Kline, 0, nClosed+1)
	for i := 0; i < nClosed; i++ {
		o := formingOpen.Add(-time.Duration(nClosed-i) * iv)
		out = append(out, datafetch.Kline{
			OpenTime:  o.UnixMilli(),
			CloseTime: o.Add(iv).UnixMilli() - 1,
			Open:      closedPrice,
			High:      closedPrice * 1.001,
			Low:       closedPrice * 0.999,
			Close:     closedPrice,
			Volume:    1000,
			TakerBuy:  400,
		})
	}
	out = append(out, datafetch.Kline{
		OpenTime:  formingOpen.UnixMilli(),
		CloseTime: formingOpen.Add(iv).UnixMilli() - 1,
		Open:      closedPrice,
		High:      closedPrice,
		Low:       formingClose,
		Close:     formingClose,
		Volume:    1000,
		TakerBuy:  400,
	})
	return out
}

func TestU5ScoringClosedBarConfirmationsKeepFormingFeaturesIntact(t *testing.T) {
	at := time.Date(2026, 11, 28, 0, 22, 1, 0, time.UTC)
	zone := V7PriceZone{Lower: 99.0, Upper: 101.0}
	pClosed := 100.0
	pForming := 98.5 // dips below zone.Lower on forming bar only

	ss := &datafetch.SymbolSnapshot{
		Symbol:         "TESTUSDT",
		Price:          pForming,
		Timestamp:      at,
		ClosedBarsOnly: false, // default feature_bars="forming"
		Klines: map[string][]datafetch.Kline{
			"5m":  u5DatafetchBars(5*time.Minute, at, 25, pClosed, pForming),
			"15m": u5DatafetchBars(15*time.Minute, at, 25, pClosed, pForming),
		},
	}
	snap := &datafetch.Snapshot{
		Symbols:   map[string]*datafetch.SymbolSnapshot{"TESTUSDT": ss},
		CreatedAt: at,
	}

	ctx := buildSymbolContext("TESTUSDT", ss, snap)
	// Non-confirmation ExecutionContext stays on forming bars (default unchanged).
	if ctx.ExecutionContext == nil || ctx.ExecutionContext.Timeframes["5m"].LastClose != pForming {
		t.Fatalf("non-confirmation ExecutionContext changed: got %+v, want forming LastClose=%v", ctx.ExecutionContext, pForming)
	}
	// Confirmation timeframes strictly exclude the forming bar.
	if ctx.ClosedConfirmTimeframes["5m"].LastClose != pClosed {
		t.Fatalf("ClosedConfirmTimeframes[5m].LastClose = %v, want closed %v", ctx.ClosedConfirmTimeframes["5m"].LastClose, pClosed)
	}
	if ctx.ClosedConfirmTimeframes["15m"].LastClose != pClosed {
		t.Fatalf("ClosedConfirmTimeframes[15m].LastClose = %v, want closed %v", ctx.ClosedConfirmTimeframes["15m"].LastClose, pClosed)
	}

	sig := &V7SignalOutput{
		Symbol:           "TESTUSDT",
		Direction:        V7DirShort,
		SetupType:        V7SetupBreakdownShort,
		EntryZone:        zone,
		Invalidation:     V7InvalidationRule{Price: 102.5},
		Targets:          []V7Target{{Price: 94.0}},
		RequiredConfirms: []string{"5m_or_15m_close_below_trigger", "taker_buy_15m_lt_0_48"},
	}
	summary := EvaluateV7Confirmations(sig, ctx, DefaultV7Config())
	if summary.PassedReview {
		t.Fatalf("forming-only dip must NOT pass 5m_or_15m_close_below_trigger at scoring: %+v", summary)
	}
	if len(summary.MissingReview) != 1 || summary.MissingReview[0].Code != "5m_or_15m_close_below_trigger" {
		t.Fatalf("unexpected MissingReview: %+v", summary.MissingReview)
	}
	ev := summary.MissingReview[0].Evidence
	wantCloseUnix := at.Truncate(5 * time.Minute).Unix()
	if ev == nil || ev.Passed || !ev.ClosedOnly || ev.DecidingTF != "5m" ||
		ev.BarClose != pClosed || ev.ReferenceEdge != zone.Lower || ev.BarCloseTimeUnix != wantCloseUnix {
		t.Fatalf("scoring failed check evidence mismatch: %+v (want close=%v edge=%v closeUnix=%d)", ev, pClosed, zone.Lower, wantCloseUnix)
	}
	if len(summary.BarEvidence) != 1 || summary.BarEvidence[0] != *ev {
		t.Fatalf("summary.BarEvidence mismatch: %+v", summary.BarEvidence)
	}

	// Positive control: closed 5m bar DID close below zone.Lower, even though
	// forming bar + CurrentPrice bounced back above zone.Lower.
	ssPass := &datafetch.SymbolSnapshot{
		Symbol:         "TESTUSDT",
		Price:          100.5, // bounced inside zone
		Timestamp:      at,
		ClosedBarsOnly: false,
		Klines: map[string][]datafetch.Kline{
			"5m":  u5DatafetchBars(5*time.Minute, at, 25, 98.8, 100.5),
			"15m": u5DatafetchBars(15*time.Minute, at, 25, 100.0, 100.5),
		},
	}
	ctxPass := buildSymbolContext("TESTUSDT", ssPass, &datafetch.Snapshot{
		Symbols:   map[string]*datafetch.SymbolSnapshot{"TESTUSDT": ssPass},
		CreatedAt: at,
	})
	summaryPass := EvaluateV7Confirmations(sig, ctxPass, DefaultV7Config())
	if !summaryPass.PassedReview || len(summaryPass.MissingReview) != 0 {
		t.Fatalf("positive control: closed 5m bar <= zone.Lower must pass even when spot bounced: %+v", summaryPass)
	}
	if len(summaryPass.BarEvidence) != 1 || !summaryPass.BarEvidence[0].Passed ||
		summaryPass.BarEvidence[0].DecidingTF != "5m" || summaryPass.BarEvidence[0].BarClose != 98.8 {
		t.Fatalf("positive control BarEvidence mismatch: %+v", summaryPass.BarEvidence)
	}

	// Persisted RawJSON path (V7SignalOutput JSON marshaling) includes bar_evidence.
	sig.ConfirmSummary = &summaryPass
	raw, err := json.Marshal(sig)
	if err != nil {
		t.Fatalf("marshal V7SignalOutput: %v", err)
	}
	if !strings.Contains(string(raw), `"bar_evidence"`) || !strings.Contains(string(raw), `"bar_close_time_unix"`) {
		t.Fatalf("persisted V7SignalOutput JSON missing bar_evidence: %s", string(raw))
	}
}

func TestU5ScoringTriggerFailsClosedWhenZeroClosedBarsExist(t *testing.T) {
	at := time.Date(2026, 11, 28, 0, 22, 1, 0, time.UTC)
	ss := &datafetch.SymbolSnapshot{
		Symbol:    "EMPTYUSDT",
		Price:     98.0, // spot below lower edge
		Timestamp: at,
		Klines: map[string][]datafetch.Kline{
			"5m":  u5DatafetchBars(5*time.Minute, at, 0, 98.0, 98.0),
			"15m": u5DatafetchBars(15*time.Minute, at, 0, 98.0, 98.0),
		},
	}
	ctx := buildSymbolContext("EMPTYUSDT", ss, &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{"EMPTYUSDT": ss}, CreatedAt: at})
	if ctx.ClosedConfirmTimeframes == nil || len(ctx.ClosedConfirmTimeframes) != 0 {
		t.Fatalf("expected non-nil empty ClosedConfirmTimeframes, got %#v", ctx.ClosedConfirmTimeframes)
	}
	// Trigger alias 5m_or_15m_close_above_entry_zone and primary trigger code both fail closed (known=true, passed=false).
	for _, code := range []string{"5m_or_15m_close_below_trigger", "5m_or_15m_close_above_entry_zone"} {
		sig := &V7SignalOutput{
			Symbol:           "EMPTYUSDT",
			Direction:        V7DirShort,
			EntryZone:        V7PriceZone{Lower: 99, Upper: 101},
			Invalidation:     V7InvalidationRule{Price: 102},
			Targets:          []V7Target{{Price: 94}},
			RequiredConfirms: []string{code},
		}
		check, known, ok := V7EvaluateSharedConfirmation(code, v7ScoringConfirmFacts(sig, ctx))
		if !ok || !known || check.Passed || check.Evidence == nil || check.Evidence.DecidingTF != "none" {
			t.Fatalf("%s with zero closed bars: ok=%v known=%v passed=%v ev=%+v, want (true, true, false, deciding=none)",
				code, ok, known, check.Passed, check.Evidence)
		}
	}
	// U5b Fix 1: bar-reading predicates also return known=true, passed=false on missing closed bars.
	for _, code := range []string{"directional_15m_close_short", "directional_15m_close_long", "5m_price_holds_ema20_or_trailing_support"} {
		sig := &V7SignalOutput{
			Symbol:           "EMPTYUSDT",
			Direction:        V7DirShort,
			EntryZone:        V7PriceZone{Lower: 99, Upper: 101},
			RequiredConfirms: []string{code},
		}
		check, known, ok := V7EvaluateSharedConfirmation(code, v7ScoringConfirmFacts(sig, ctx))
		if !ok || !known || check.Passed || check.Evidence == nil || check.Evidence.DecidingTF != "none" {
			t.Fatalf("U5b %s with zero closed bars: ok=%v known=%v passed=%v ev=%+v, want (true, true, false, deciding=none)",
				code, ok, known, check.Passed, check.Evidence)
		}
	}
}

// TestU5bVolumeBurstDoesNotFallBackToFormingWhenClosedTimeframesPresent (Item 2B):
// When ClosedConfirmTimeframes != nil and ClosedVolumeBurst15m == 0 (e.g., < 3
// closed 15m bars or zero closed volume), a large forming-bar VolumeBurst15m
// must NOT leak into oi_delta_1h_positive_or_quote_volume_expands or its aliases.
func TestU5bVolumeBurstDoesNotFallBackToFormingWhenClosedTimeframesPresent(t *testing.T) {
	at := time.Date(2026, 11, 28, 0, 22, 1, 0, time.UTC)
	// Build 2 closed 15m bars (volume 100 each) + 1 forming 15m bar with a
	// huge volume spike (volume 500 -> forming VolumeBurst15m = 5.0 >= 1.2,
	// while closed 15m bars have len=2 < 3 -> ClosedVolumeBurst15m == 0).
	bars15m := u5DatafetchBars(15*time.Minute, at, 2, 100.0, 101.0)
	bars15m[0].Volume = 100
	bars15m[1].Volume = 100
	bars15m[2].Volume = 500 // forming bar spike

	ss := &datafetch.SymbolSnapshot{
		Symbol:         "VOLUSDT",
		Price:          101.0,
		Timestamp:      at,
		ClosedBarsOnly: false,
		OIDelta1h:      -1.5, // OI leg is negative/failing
		Klines: map[string][]datafetch.Kline{
			"15m": bars15m,
			"5m":  u5DatafetchBars(5*time.Minute, at, 25, 100.0, 101.0),
		},
	}
	ctx := buildSymbolContext("VOLUSDT", ss, &datafetch.Snapshot{
		Symbols:   map[string]*datafetch.SymbolSnapshot{"VOLUSDT": ss},
		CreatedAt: at,
	})
	if ctx.VolumeBurst15m < v7ConfirmVolumeBurstExpands {
		t.Fatalf("fixture bug: expected forming VolumeBurst15m >= %v, got %v", v7ConfirmVolumeBurstExpands, ctx.VolumeBurst15m)
	}
	if ctx.ClosedConfirmTimeframes == nil {
		t.Fatalf("fixture bug: expected non-nil ClosedConfirmTimeframes")
	}
	if ctx.ClosedVolumeBurst15m != 0 {
		t.Fatalf("fixture bug: expected ClosedVolumeBurst15m == 0 with 2 closed bars, got %v", ctx.ClosedVolumeBurst15m)
	}

	for _, code := range []string{
		"oi_delta_1h_positive_or_quote_volume_expands",
		"oi_continues_inflow",
		"oi_or_volume_expands_with_price",
	} {
		sig := &V7SignalOutput{
			Symbol:           "VOLUSDT",
			Direction:        V7DirLong,
			SetupType:        V7SetupDisplacementLong,
			EntryZone:        V7PriceZone{Lower: 99, Upper: 102},
			Invalidation:     V7InvalidationRule{Price: 97},
			Targets:          []V7Target{{Price: 110}},
			RequiredConfirms: []string{code},
		}
		facts := v7ScoringConfirmFacts(sig, ctx)
		if facts.VolumeBurst15m != 0 {
			t.Fatalf("%s: v7ScoringConfirmFacts leaked forming VolumeBurst15m=%v when ClosedConfirmTimeframes != nil", code, facts.VolumeBurst15m)
		}
		check, known, ok := V7EvaluateSharedConfirmation(code, facts)
		if !ok || !known || check.Passed {
			t.Fatalf("%s: expected known failure from closed facts, got ok=%v known=%v passed=%v", code, ok, known, check.Passed)
		}
		summary := EvaluateV7Confirmations(sig, ctx, DefaultV7Config())
		if summary.PassedReview {
			t.Fatalf("%s: EvaluateV7Confirmations passed from forming-bar VolumeBurst15m: %+v", code, summary)
		}
	}

	// Control: synthetic context with ClosedConfirmTimeframes == nil still falls
	// back to ctx.VolumeBurst15m as intended.
	synthCtx := &V7SymbolContext{
		Symbol:         "VOLUSDT",
		CurrentPrice:   100.0,
		VolumeBurst15m: 2.0,
		Snapshot:       &SymbolSnapshotData{OIDelta1h: -1.5},
	}
	synthSig := &V7SignalOutput{
		Symbol:           "VOLUSDT",
		Direction:        V7DirLong,
		SetupType:        V7SetupDisplacementLong,
		RequiredConfirms: []string{"oi_delta_1h_positive_or_quote_volume_expands"},
	}
	if check, known, ok := V7EvaluateSharedConfirmation("oi_delta_1h_positive_or_quote_volume_expands", v7ScoringConfirmFacts(synthSig, synthCtx)); !ok || !known || !check.Passed {
		t.Fatalf("synthetic control (ClosedConfirmTimeframes==nil) should still use VolumeBurst15m: ok=%v known=%v passed=%v", ok, known, check.Passed)
	}
}
