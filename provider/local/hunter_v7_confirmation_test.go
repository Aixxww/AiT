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

func hasV7MissingConfirmation(checks []V7ConfirmationCheck, code string) bool {
	for _, check := range checks {
		if check.Code == code {
			return true
		}
	}
	return false
}
