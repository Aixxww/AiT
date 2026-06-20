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
