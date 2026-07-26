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
