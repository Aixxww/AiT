package local

import "testing"

func TestV7WatchStateManagerAddsMicroConfirmationsOnReviewableUpgrade(t *testing.T) {
	manager := NewV7SignalStateManager()
	var upgraded []V7SignalOutput
	for cycle := 1; cycle <= 4; cycle++ {
		upgraded = manager.Process([]V7SignalOutput{{
			Symbol:           "WATCHUSDT",
			Direction:        V7DirShort,
			SetupType:        V7SetupPreDistribution,
			Status:           V7StatusWaitConfirm,
			ExecutionQuality: V7ExecWatchOnly,
			ReasonCodes:      []string{"crowded_longs_near_resistance", "taker_buy_weakening", "rally_stalling_near_high"},
			RiskTags:         []string{"do_not_open_until_confirmed"},
			RiskScore:        25,
			LiquidityScore:   70,
			EntryZone:        V7PriceZone{Lower: 99, Upper: 102},
			Invalidation:     V7InvalidationRule{Price: 103},
			PriceCtx:         &V7PriceContext{Last: 101},
		}}, cycle)
	}

	if len(upgraded) != 1 {
		t.Fatalf("upgraded signals = %d, want 1", len(upgraded))
	}
	got := upgraded[0]
	if got.ExecutionQuality != V7ExecNearConfirm || got.Status != V7StatusCandidate {
		t.Fatalf("upgrade state quality/status = %s/%s, want near_confirm/candidate", got.ExecutionQuality, got.Status)
	}
	if !containsString(got.ReasonCodes, "multi_cycle_confirmation") {
		t.Fatalf("missing multi_cycle_confirmation: %+v", got.ReasonCodes)
	}
	if !containsString(got.RequiredConfirms, "live_price_in_entry_zone") ||
		!containsString(got.RequiredConfirms, "5m_close_below_ema20_or_entry_zone_mid") {
		t.Fatalf("missing micro confirmations: %+v", got.RequiredConfirms)
	}
}

func TestV7WatchStateManagerDoesNotUpgradeBrokenInvalidation(t *testing.T) {
	manager := NewV7SignalStateManager()
	var upgraded []V7SignalOutput
	for cycle := 1; cycle <= 4; cycle++ {
		upgraded = manager.Process([]V7SignalOutput{{
			Symbol:           "FAILUSDT",
			Direction:        V7DirLong,
			SetupType:        V7SetupPreBreakoutWatch,
			Status:           V7StatusWaitConfirm,
			ExecutionQuality: V7ExecWatchOnly,
			ReasonCodes:      []string{"compressed_oi_pre_breakout", "taker_buy_bias_before_breakout"},
			RiskScore:        20,
			LiquidityScore:   80,
			EntryZone:        V7PriceZone{Lower: 100, Upper: 102},
			Invalidation:     V7InvalidationRule{Price: 98},
			PriceCtx:         &V7PriceContext{Last: 97.5},
		}}, cycle)
	}

	if len(upgraded) != 0 {
		t.Fatalf("upgraded signals = %d, want 0 after invalidation break", len(upgraded))
	}
}

func TestV7SignalStateManagerPromotesBreakoutTriggerMemory(t *testing.T) {
	manager := NewV7SignalStateManager()
	first := V7SignalOutput{
		Symbol:           "SUIUSDT",
		Direction:        V7DirLong,
		SetupType:        V7SetupTrendBreakoutLong,
		Status:           V7StatusWaitConfirm,
		ExecutionQuality: V7ExecWatchOnly,
		EntrySignal:      V7EntrySignalTriggerNear,
		ReasonCodes:      []string{"entry_trigger_near"},
		RiskScore:        20,
		LiquidityScore:   90,
		EntryZone:        V7PriceZone{Lower: 0.758, Upper: 0.763},
		Invalidation:     V7InvalidationRule{Price: 0.755},
		PriceCtx:         &V7PriceContext{Last: 0.761},
	}
	if upgraded := manager.Process([]V7SignalOutput{first}, 1); len(upgraded) != 0 {
		t.Fatalf("first cycle upgraded = %d, want 0", len(upgraded))
	}

	second := first
	second.PriceCtx = &V7PriceContext{Last: 0.764}
	upgraded := manager.Process([]V7SignalOutput{second}, 2)

	if len(upgraded) != 1 {
		t.Fatalf("upgraded signals = %d, want 1", len(upgraded))
	}
	got := upgraded[0]
	if got.EntrySignal != V7EntrySignalOpenNow {
		t.Fatalf("entry_signal = %q, want %q", got.EntrySignal, V7EntrySignalOpenNow)
	}
	if !containsString(got.ReasonCodes, "trigger_memory_confirmed") {
		t.Fatalf("missing trigger_memory_confirmed: %+v", got.ReasonCodes)
	}
	if got.ExecutionQuality != V7ExecNearConfirm {
		t.Fatalf("execution_quality = %q, want %q", got.ExecutionQuality, V7ExecNearConfirm)
	}
}

func TestV7SignalStateManagerKeepsStableLongBreakoutTrigger(t *testing.T) {
	manager := NewV7SignalStateManager()
	first := V7SignalOutput{
		Symbol:           "CAPUSDT",
		Direction:        V7DirLong,
		SetupType:        V7SetupTrendBreakoutLong,
		Status:           V7StatusWaitConfirm,
		ExecutionQuality: V7ExecWatchOnly,
		EntrySignal:      V7EntrySignalTriggerNear,
		ReasonCodes:      []string{"entry_trigger_near"},
		RiskScore:        0,
		LiquidityScore:   70,
		EntryZone:        V7PriceZone{Lower: 0.01870, Upper: 0.018997},
		Invalidation:     V7InvalidationRule{Price: 0.01840},
		PriceCtx:         &V7PriceContext{Last: 0.01883},
	}
	if upgraded := manager.Process([]V7SignalOutput{first}, 1); len(upgraded) != 0 {
		t.Fatalf("first cycle upgraded = %d, want 0", len(upgraded))
	}

	second := first
	second.EntryZone = V7PriceZone{Lower: 0.01870, Upper: 0.019067}
	second.PriceCtx = &V7PriceContext{Last: 0.01901}
	upgraded := manager.Process([]V7SignalOutput{second}, 2)

	if len(upgraded) != 1 {
		t.Fatalf("upgraded signals = %d, want 1", len(upgraded))
	}
	got := upgraded[0]
	if got.EntrySignal != V7EntrySignalOpenNow {
		t.Fatalf("entry_signal = %q, want %q", got.EntrySignal, V7EntrySignalOpenNow)
	}
	if got.EntryZone.Upper != first.EntryZone.Upper {
		t.Fatalf("entry zone upper = %.8f, want stable trigger %.8f", got.EntryZone.Upper, first.EntryZone.Upper)
	}
}

func TestV7SignalStateManagerDoesNotPromoteAltLadderShortWithoutCloseThrough(t *testing.T) {
	manager := NewV7SignalStateManager()
	first := altLadderShortMemorySignal(0.0866, false)
	if upgraded := manager.Process([]V7SignalOutput{first}, 1); len(upgraded) != 0 {
		t.Fatalf("first cycle upgraded = %d, want 0", len(upgraded))
	}

	second := altLadderShortMemorySignal(0.0864, false)
	upgraded := manager.Process([]V7SignalOutput{second}, 2)
	if len(upgraded) != 0 {
		t.Fatalf("missing close-through upgraded = %d, want 0", len(upgraded))
	}
}

func TestV7SignalStateManagerPromotesAltLadderShortAfterCloseThrough(t *testing.T) {
	manager := NewV7SignalStateManager()
	first := altLadderShortMemorySignal(0.0866, false)
	if upgraded := manager.Process([]V7SignalOutput{first}, 1); len(upgraded) != 0 {
		t.Fatalf("first cycle upgraded = %d, want 0", len(upgraded))
	}

	second := altLadderShortMemorySignal(0.0857, true)
	upgraded := manager.Process([]V7SignalOutput{second}, 2)
	if len(upgraded) != 1 {
		t.Fatalf("upgraded signals = %d, want 1", len(upgraded))
	}
	got := upgraded[0]
	if got.EntrySignal != V7EntrySignalOpenNow {
		t.Fatalf("entry_signal = %q, want %q", got.EntrySignal, V7EntrySignalOpenNow)
	}
	if got.ExecutionQuality != V7ExecReady {
		t.Fatalf("execution_quality = %q, want %q", got.ExecutionQuality, V7ExecReady)
	}
	if !containsString(got.ReasonCodes, "alt_ladder_multi_cycle_close_through") {
		t.Fatalf("missing alt_ladder_multi_cycle_close_through: %+v", got.ReasonCodes)
	}
	if !containsString(got.RequiredConfirms, "no_new_high_after_rejection") {
		t.Fatalf("missing no-new-high rebound-failure confirmation: %+v", got.RequiredConfirms)
	}
}

func TestV7SignalStateManagerDoesNotPromoteAltLadderShortWithoutRejectionFailure(t *testing.T) {
	manager := NewV7SignalStateManager()
	first := altLadderShortMemorySignal(0.0866, false)
	if upgraded := manager.Process([]V7SignalOutput{first}, 1); len(upgraded) != 0 {
		t.Fatalf("first cycle upgraded = %d, want 0", len(upgraded))
	}

	second := altLadderShortMemorySignalWithRejection(0.0857, true, false)
	upgraded := manager.Process([]V7SignalOutput{second}, 2)
	if len(upgraded) != 0 {
		t.Fatalf("missing rebound-failure confirmation upgraded = %d, want 0", len(upgraded))
	}
}

func altLadderShortMemorySignal(price float64, closeThrough bool) V7SignalOutput {
	return altLadderShortMemorySignalWithRejection(price, closeThrough, closeThrough)
}

func altLadderShortMemorySignalWithRejection(price float64, closeThrough, noNewHigh bool) V7SignalOutput {
	summary := &V7ConfirmationSummary{
		PassedHard:   true,
		PassedReview: closeThrough && noNewHigh,
	}
	if closeThrough {
		summary.ContextChecks = []V7ConfirmationCheck{{
			Code:   "5m_or_15m_close_below_trigger",
			Passed: true,
		}}
	} else {
		summary.MissingReview = []V7ConfirmationCheck{{
			Code:   "5m_or_15m_close_below_trigger",
			Passed: false,
		}}
	}
	if noNewHigh {
		summary.ContextChecks = append(summary.ContextChecks, V7ConfirmationCheck{
			Code:   "no_new_high_after_rejection",
			Passed: true,
		})
	} else {
		summary.MissingReview = append(summary.MissingReview, V7ConfirmationCheck{
			Code:   "no_new_high_after_rejection",
			Passed: false,
		})
	}
	return V7SignalOutput{
		Symbol:           "OPUSDT",
		Direction:        V7DirShort,
		SetupType:        V7SetupAltLadderShort,
		Status:           V7StatusWaitConfirm,
		ExecutionQuality: V7ExecNearConfirm,
		EntrySignal:      V7EntrySignalTriggerNear,
		ReasonCodes: []string{
			"alt_ladder_breakdown_short",
			"alt_ladder_downshift_mid",
			"alt_ladder_taker_sell",
			"alt_ladder_new_shorts",
		},
		RequiredConfirms: []string{
			"live_price_in_entry_zone",
			"taker_buy_15m_lt_0_48",
			"5m_or_15m_close_below_trigger",
			"no_new_high_after_rejection",
		},
		RiskScore:      30,
		LiquidityScore: 80,
		TimingScore:    64,
		AIPriority:     60,
		EntryZone:      V7PriceZone{Lower: 0.0858, Upper: 0.0868},
		Invalidation:   V7InvalidationRule{Price: 0.0884},
		PriceCtx:       &V7PriceContext{Last: price},
		DerivativesCtx: &V7DerivativesContext{TakerBuy15m: 0.44},
		ConfirmSummary: summary,
	}
}
