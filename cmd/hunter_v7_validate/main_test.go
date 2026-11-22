package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/provider/local"
)

func TestValidateFormatSkipsExecutionGeometryForConflictWatch(t *testing.T) {
	signals := []local.V7SignalOutput{
		{
			Symbol:           "BASEDUSDT",
			Direction:        local.V7DirLong,
			SetupType:        local.V7SetupWhaleFlow,
			Status:           local.V7StatusConflictWatch,
			SetupScore:       99,
			RiskScore:        0,
			LiquidityScore:   0,
			TimingScore:      0,
			RegimeFitScore:   0,
			AIPriority:       75,
			ReasonCodes:      []string{"directional_conflict"},
			EntryMode:        local.V7EntryWaitConfirm,
			EntryZone:        local.V7PriceZone{Lower: 0.10, Upper: 0.11},
			RequiredConfirms: []string{"directional_15m_close_long"},
			Confidence:       "C",
			RiskLevel:        local.V7RiskLow,
			MarketRegime:     local.V7RegimeRotation,
			PriceCtx:         &local.V7PriceContext{Last: 0.107},
			DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.50},
		},
	}

	check, issues := validateFormat(signals)

	for _, is := range issues {
		if is.Code == "missing_invalidation" || is.Code == "missing_targets" {
			t.Fatalf("conflict_watch should not report executable geometry issue: %+v", issues)
		}
	}
	if check.ExecutableGapCount != 0 {
		t.Fatalf("executable gap count = %d, want 0; issues=%+v", check.ExecutableGapCount, issues)
	}
}

func TestValidatePromptParsesFinalTierSummary(t *testing.T) {
	prompt := `
noise
Tier Summary: EXECUTABLE=0 | REVIEWABLE=1 | WATCH=5 | REJECTED=4
more noise
`

	check := validatePrompt(prompt, 10)

	if !check.PromptContainsTierSummary {
		t.Fatalf("prompt tier summary was not detected")
	}
	want := map[string]int{
		"EXECUTABLE": 0,
		"REVIEWABLE": 1,
		"WATCH":      5,
		"REJECTED":   4,
	}
	for tier, count := range want {
		if check.PromptTierCounts[tier] != count {
			t.Fatalf("tier %s = %d, want %d; counts=%v", tier, check.PromptTierCounts[tier], count, check.PromptTierCounts)
		}
	}
}

func TestPromptIssuesAllowsNoOpenReviewCandidatesWithoutExpandedJSON(t *testing.T) {
	check := aiRecognitionCheck{
		PromptContainsTierSummary: true,
		PromptTierCounts: map[string]int{
			"EXECUTABLE": 0,
			"REVIEWABLE": 0,
			"WATCH":      8,
			"REJECTED":   1,
		},
	}

	if issues := promptIssues(check); len(issues) != 0 {
		t.Fatalf("no open-review candidates should not require expanded v7 JSON: %+v", issues)
	}
}

func TestFormatRunSummaryMarksAllDegradedSampleInvalidForWinRate(t *testing.T) {
	md := formatRunSummaryMarkdown(validationRunSummary{
		GeneratedAt: "2026-08-02 12:00:00 CST",
		Rounds:      2,
		ValidRounds: 0,
		Round: []validationRoundSummary{
			{Round: 1, Regime: "compression", SignalCount: 8, Degraded: true, Reasons: []string{"universe_coverage_lt_30pct"}},
			{Round: 2, Regime: "trend_down", SignalCount: 9, Degraded: true, Reasons: []string{"rest_error_rate_gt_20pct"}},
		},
	}, "summary.json")

	if !strings.Contains(md, "INVALID_SAMPLE_DO_NOT_USE_FOR_WINRATE") {
		t.Fatalf("all-degraded summary should mark sample invalid for win-rate use:\n%s", md)
	}
}

func TestSignalsToCandidatesPreservesSignalContractFields(t *testing.T) {
	signals := []local.V7SignalOutput{
		{
			SignalID:      "7|BTCUSDT|LONG|trend_breakout_long",
			Symbol:        "BTCUSDT",
			Direction:     local.V7DirLong,
			SetupType:     local.V7SetupTrendBreakoutLong,
			Status:        local.V7StatusCandidate,
			ReasonCodes:   []string{"confirmed_breakout"},
			Confidence:    "B",
			MarketRegime:  local.V7RegimeTrendUp,
			DataFreshness: local.V7DataFreshness{PriceAgeMs: 1234, SnapshotAgeMs: 5678},
		},
	}

	candidates := kernel.AssembleHunterV7CandidateCoins(signals, "BOTH", executionGeometryFromStrategy(nil))
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if candidates[0].V7SignalID != "7|BTCUSDT|LONG|trend_breakout_long" {
		t.Fatalf("signal id = %q", candidates[0].V7SignalID)
	}
	if candidates[0].V7DataFreshness.PriceAgeMs != 1234 || candidates[0].V7DataFreshness.SnapshotAgeMs != 5678 {
		t.Fatalf("freshness = %+v", candidates[0].V7DataFreshness)
	}
}

func TestValidateCoverageClassifiesDisplacementAndRangeExpansionFamilies(t *testing.T) {
	coverage := validateCoverage([]local.V7SignalOutput{
		{Symbol: "AUSDT", Direction: local.V7DirLong, SetupType: local.V7SetupDisplacementLong, MarketRegime: local.V7RegimeTrendUp},
		{Symbol: "BUSDT", Direction: local.V7DirLong, SetupType: local.V7SetupRangeExpansion, MarketRegime: local.V7RegimeTrendUp},
	}, executionGeometryFromStrategy(nil))

	if !coverage.HasMomentum {
		t.Fatalf("displacement/range expansion should count as momentum family: %+v", coverage)
	}
	if !coverage.HasRange {
		t.Fatalf("range expansion should count as range/event family: %+v", coverage)
	}
}

// ta1T2Outputs returns WATCH, ARMED, TRIGGERED and observe-only TRIGGERED
// Task-2 outputs as the converter emits them (before TA-1).
func ta1T2Outputs() []local.V7SignalOutput {
	mk := func(setup, status string, score float64, observe bool) *local.V7T2Signal {
		return &local.V7T2Signal{Symbol: "AUSDT", Direction: "LONG", SetupType: setup, Status: status, Score: score,
			Trigger: 100, Entry: 100, Stop: 99, Target1: 101.8, Target2: 103, ObserveOnly: observe,
			Reasons: []string{"t2_test"}}
	}
	return []local.V7SignalOutput{
		mk("ignition_early_long", "WATCH", 70, false).ToV7SignalOutputWithLifecycle("", "WATCH"),
		mk("momentum_continuation_long", "ARMED", 90, false).ToV7SignalOutputWithLifecycle("", "ARMED"),
		mk("ignition_early_long", "WATCH", 70, false).ToV7SignalOutputWithLifecycle("", "TRIGGERED"),
		mk("trend_pullback_long", "ARMED", 94, true).ToV7SignalOutputWithLifecycle("", "TRIGGERED"),
	}
}

// TA-1: before the fix, every T2 output misses exactly 2 JSON fields (the
// Tier A "missing_fields = 2 x module signals" rule); after it, the harness
// contract passes with no missing fields and no executable gaps.
func TestTA1T2OutputsPassValidationContract(t *testing.T) {
	outs := ta1T2Outputs()
	before, _ := validateFormat(outs)
	if before.MissingFieldCount != 2*len(outs) {
		t.Fatalf("pre-TA-1 missing_fields = %d, want %d (2 per T2 signal)", before.MissingFieldCount, 2*len(outs))
	}
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{
		"AUSDT": {Symbol: "AUSDT", Price: 100, FundingRate: 0.0001, OI: 1000},
	}}
	for i := range outs {
		local.ApplyV7T2ContractFields(&outs[i], snap)
	}
	after, issues := validateFormat(outs)
	if after.MissingFieldCount != 0 || after.ExecutableGapCount != 0 {
		t.Fatalf("post-TA-1 missing=%d gaps=%d issues=%+v", after.MissingFieldCount, after.ExecutableGapCount, issues)
	}
	if err := reportAcceptanceError(validationReport{FormatCheck: after}); err != nil {
		t.Fatalf("contract still rejects: %v", err)
	}
}

// TA-1 harness fix: a signal without a derivatives context no longer
// shows funding 0 in the prompt market data; the snapshot's rate is kept.
func TestTA1PromptMarketDataKeepsSnapshotFunding(t *testing.T) {
	now := time.Now()
	bars := func(n int, intervalMs int64) []datafetch.Kline {
		out := make([]datafetch.Kline, n)
		for i := range out {
			ct := now.UnixMilli() - int64(n-1-i)*intervalMs
			out[i] = datafetch.Kline{OpenTime: ct - intervalMs + 1, CloseTime: ct, Open: 100, High: 101, Low: 99, Close: 100, Volume: 10}
		}
		return out
	}
	withKlines := &datafetch.SymbolSnapshot{Symbol: "KUSDT", Price: 100, FundingRate: 0.0005, Timestamp: now,
		Klines: map[string][]datafetch.Kline{"5m": bars(80, 300000), "15m": bars(80, 900000), "1h": bars(80, 3600000)}}
	sparse := &datafetch.SymbolSnapshot{Symbol: "SUSDT", Price: 5, FundingRate: -0.0003, Timestamp: now}
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{"KUSDT": withKlines, "SUSDT": sparse}, CreatedAt: now}

	signals := []local.V7SignalOutput{{Symbol: "KUSDT"}, {Symbol: "SUSDT"}} // nil DerivativesCtx (pre-TA-1 T2 shape)
	md := promptMarketData(signals, snap)
	if md["KUSDT"] == nil || md["KUSDT"].FundingRate != 0.0005 {
		t.Fatalf("full-converter funding = %+v, want 0.0005", md["KUSDT"])
	}
	if md["SUSDT"] == nil || md["SUSDT"].FundingRate != -0.0003 {
		t.Fatalf("sparse funding = %+v, want -0.0003", md["SUSDT"])
	}

	// A signal's own derivatives context still wins (native behaviour).
	signals = []local.V7SignalOutput{{Symbol: "KUSDT", DerivativesCtx: &local.V7DerivativesContext{FundingRate: 0.0009}}}
	if got := promptMarketData(signals, snap)["KUSDT"].FundingRate; got != 0.0009 {
		t.Fatalf("derivatives-context funding = %v, want 0.0009", got)
	}
}

// P4-unblock proof (TA-1 full). The narrowed form (both contexts, Last = 0,
// no confirmations) left one executable gap (missing_required_confirmations)
// on every wait_confirm T2 output, so reportAcceptanceError rejected the
// round. With the TA-1 confirmation a WATCH output, and an observe-only
// TRIGGERED output (also wait_confirm), pass with 0 missing and 0 gaps.
func TestTA1P4UnblockWatchPassesContract(t *testing.T) {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{
		"AUSDT": {Symbol: "AUSDT", Price: 100, FundingRate: 0.0001, OI: 1000},
	}}
	for _, o := range ta1T2Outputs() {
		if o.EntryMode == local.V7EntryImmediate {
			continue
		}
		narrowed := o
		narrowed.PriceCtx = &local.V7PriceContext{}
		narrowed.DerivativesCtx = &local.V7DerivativesContext{}
		nc, _ := validateFormat([]local.V7SignalOutput{narrowed})
		if nc.MissingFieldCount != 0 || nc.ExecutableGapCount != 1 || reportAcceptanceError(validationReport{FormatCheck: nc}) == nil {
			t.Fatalf("%s narrowed: missing=%d gaps=%d, want 0/1 and a rejected round", o.LifecycleState, nc.MissingFieldCount, nc.ExecutableGapCount)
		}

		full := o
		local.ApplyV7T2ContractFields(&full, snap)
		if full.Status != local.V7StatusWaitConfirm || full.PriceCtx == nil || full.PriceCtx.Last != snap.Symbols[o.Symbol].Price || full.PriceCtx.Last <= 0 ||
			full.DerivativesCtx == nil || len(full.RequiredConfirms) != 1 || full.RequiredConfirms[0] != "live_price_in_entry_zone" {
			t.Fatalf("%s full TA-1 shape: %+v", o.LifecycleState, full)
		}
		fc, issues := validateFormat([]local.V7SignalOutput{full})
		if fc.MissingFieldCount != 0 || fc.ExecutableGapCount != 0 {
			t.Fatalf("%s full: missing=%d gaps=%d issues=%+v, want 0/0", o.LifecycleState, fc.MissingFieldCount, fc.ExecutableGapCount, issues)
		}
		if err := reportAcceptanceError(validationReport{FormatCheck: fc}); err != nil {
			t.Fatalf("%s full: round still rejected: %v", o.LifecycleState, err)
		}
		t.Logf("%s (%s): narrowed missing=%d gaps=%d -> full missing=%d gaps=%d", o.LifecycleState, o.Status, nc.MissingFieldCount, nc.ExecutableGapCount, fc.MissingFieldCount, fc.ExecutableGapCount)
	}
}
