package main

import (
	"testing"

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

	candidates := signalsToCandidates(signals)
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
	})

	if !coverage.HasMomentum {
		t.Fatalf("displacement/range expansion should count as momentum family: %+v", coverage)
	}
	if !coverage.HasRange {
		t.Fatalf("range expansion should count as range/event family: %+v", coverage)
	}
}
