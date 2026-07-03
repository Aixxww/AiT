package local

import "testing"

func TestEvaluateV7ConfirmationsCapturesGeometryAndLiveChecks(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:    "TESTUSDT",
		Direction: V7DirLong,
		SetupType: V7SetupPullbackLong,
		EntryZone: V7PriceZone{
			Lower: 100,
			Upper: 104,
		},
		Invalidation: V7InvalidationRule{Price: 98},
		Targets: []V7Target{
			{Price: 111},
		},
		RequiredConfirms: []string{
			"live_price_in_entry_zone",
			"taker_buy_15m_gt_0_52",
			"5m_close_above_ema20_or_entry_zone_mid",
		},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 103,
		TakerBuy15m:  0.49,
	}

	summary := EvaluateV7Confirmations(sig, ctx, DefaultV7Config())
	if !summary.PassedHard {
		t.Fatalf("hard confirmations should pass: %+v", summary.MissingHard)
	}
	if summary.PassedReview {
		t.Fatalf("review confirmations should fail on weak taker flow")
	}
	if summary.EntryZonePosition != 75 {
		t.Fatalf("entry zone position = %.2f, want 75", summary.EntryZonePosition)
	}
	if !hasV7MissingConfirmation(summary.MissingReview, "long_entry_zone_not_extended") {
		t.Fatalf("missing long zone extension check: %+v", summary.MissingReview)
	}
	if !hasV7MissingConfirmation(summary.MissingReview, "taker_buy_15m_gt_0_52") {
		t.Fatalf("missing taker confirmation: %+v", summary.MissingReview)
	}
}

func TestEvaluateV7ConfirmationsMarksInvalidRRHardBlock(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:       "TESTUSDT",
		Direction:    V7DirShort,
		SetupType:    V7SetupFundingReversal,
		EntryZone:    V7PriceZone{Lower: 98, Upper: 102},
		Invalidation: V7InvalidationRule{Price: 103},
		Targets: []V7Target{
			{Price: 100.5},
		},
		RequiredConfirms: []string{"taker_buy_15m_lt_0_48"},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 101,
		TakerBuy15m:  0.44,
	}

	summary := EvaluateV7Confirmations(sig, ctx, DefaultV7Config())
	if summary.PassedHard {
		t.Fatalf("hard confirmations should fail when RR is below floor: %+v", summary)
	}
	if !hasV7MissingConfirmation(summary.MissingHard, "risk_reward_min_1_2") {
		t.Fatalf("missing hard RR check: %+v", summary.MissingHard)
	}
	if hasV7MissingConfirmation(summary.MissingReview, "taker_buy_15m_lt_0_48") {
		t.Fatalf("taker flow should pass for short: %+v", summary.MissingReview)
	}
}

func TestEvaluateV7ConfirmationsMachineChecksRangeExpansionShort(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:       "ESPORTSUSDT",
		Direction:    V7DirShort,
		SetupType:    V7SetupRangeExpansion,
		EntryZone:    V7PriceZone{Lower: 0.02369, Upper: 0.02415},
		Invalidation: V7InvalidationRule{Price: 0.02445},
		Targets: []V7Target{
			{Price: 0.02261},
		},
		RequiredConfirms: []string{
			"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
			"taker_buy_15m_lt_0_48",
			"no_new_high_after_rejection",
		},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 0.02393,
		TakerBuy15m:  0.335,
		ExecutionContext: &V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes: map[string]V7ExecutionTimeframeSummary{
				"15m": {
					LastClose:        0.02383,
					HasEMA20:         true,
					CloseVsEMA20Pct:  -4.15,
					HasVWAP20:        true,
					VWAP20:           0.02444,
					CloseVsVWAP20Pct: -2.50,
					NoNewHigh:        true,
				},
				"5m": {
					LastClose: 0.02384,
					NoNewHigh: true,
				},
			},
		},
	}

	summary := EvaluateV7Confirmations(sig, ctx, DefaultV7Config())
	for _, code := range sig.RequiredConfirms {
		if hasV7MissingConfirmation(summary.MissingReview, code) ||
			hasV7MissingConfirmation(summary.ContextChecks, code) {
			t.Fatalf("required confirmation %s should be machine-passed: %+v", code, summary)
		}
	}
}

func TestWhaleFlowWeakTakerDowngradesReadiness(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:           "TACUSDT",
		Direction:        V7DirLong,
		SetupType:        V7SetupWhaleFlow,
		Status:           V7StatusCandidate,
		ExecutionQuality: V7ExecReady,
		RiskLevel:        V7RiskLow,
		AIPriority:       85,
		SetupScore:       90,
		TimingScore:      82,
		RegimeFitScore:   80,
		LiquidityScore:   100,
		EntryZone:        V7PriceZone{Lower: 0.03736, Upper: 0.03898},
		Invalidation:     V7InvalidationRule{Price: 0.03741},
		Targets: []V7Target{
			{Price: 0.04419},
		},
		RequiredConfirms: []string{
			"directional_15m_close_long",
			"taker_flow_confirms_long",
			"risk_level_not_extreme",
		},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 0.038102,
		TakerBuy15m:  0.498,
		ExecutionContext: &V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes: map[string]V7ExecutionTimeframeSummary{
				"15m": {
					LastClose:        0.03812,
					HasEMA20:         true,
					CloseVsEMA20Pct:  3.94,
					HasVWAP20:        true,
					VWAP20:           0.03405,
					CloseVsVWAP20Pct: 11.95,
					NoNewLow:         true,
				},
				"5m": {
					LastClose: 0.038132,
					NoNewLow:  true,
				},
			},
		},
	}

	summary := EvaluateV7Confirmations(sig, ctx, DefaultV7Config())
	sig.ConfirmSummary = &summary
	if hasV7MissingConfirmation(summary.ContextChecks, "directional_15m_close_long") ||
		hasV7MissingConfirmation(summary.ContextChecks, "taker_flow_confirms_long") ||
		hasV7MissingConfirmation(summary.ContextChecks, "risk_level_not_extreme") {
		t.Fatalf("new known confirmations must not remain context-only: %+v", summary.ContextChecks)
	}
	if !hasV7MissingConfirmation(summary.MissingReview, "taker_flow_confirms_long") {
		t.Fatalf("weak whale-flow taker confirmation should fail review: %+v", summary)
	}

	readiness := CalculateV7ExecutionReadiness(sig, ctx)
	if readiness.Tier != V7ReadinessWatch {
		t.Fatalf("readiness tier = %s, want WATCH: %+v", readiness.Tier, readiness)
	}
	if readiness.BlockedGate != "confirmation_missing" {
		t.Fatalf("blocked gate = %s, want confirmation_missing", readiness.BlockedGate)
	}
	if len(readiness.NextConfirm) != 1 || readiness.NextConfirm[0] != "taker_flow_confirms_long" {
		t.Fatalf("next confirmations = %+v, want taker_flow_confirms_long", readiness.NextConfirm)
	}
}

func hasV7MissingConfirmation(checks []V7ConfirmationCheck, code string) bool {
	for _, check := range checks {
		if check.Code == code {
			return true
		}
	}
	return false
}
