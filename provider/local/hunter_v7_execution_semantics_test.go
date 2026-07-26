package local

import "testing"

func TestFinalizeV7SignalAnnotatesBreakoutTriggerNear(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:         "TESTUSDT",
		Direction:      V7DirLong,
		SetupType:      V7SetupTrendBreakoutLong,
		Status:         V7StatusWaitConfirm,
		SetupScore:     86,
		TimingScore:    25,
		RiskScore:      0,
		LiquidityScore: 100,
		EntryMode:      V7EntryBreakout,
		EntryZone:      V7PriceZone{Lower: 98, Upper: 100},
		Invalidation:   V7InvalidationRule{Price: 97},
		Targets:        []V7Target{{Price: 103}},
	}
	ctx := &V7SymbolContext{CurrentPrice: 99.3}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.MarketShape != V7ShapeTrendBreakout {
		t.Fatalf("market_shape = %q, want %q", sig.MarketShape, V7ShapeTrendBreakout)
	}
	if sig.EntrySignal != V7EntrySignalTriggerNear {
		t.Fatalf("entry_signal = %q, want %q", sig.EntrySignal, V7EntrySignalTriggerNear)
	}
	if !containsV7String(sig.ReasonCodes, string(V7EntrySignalTriggerNear)) {
		t.Fatalf("reason_codes missing %q: %v", V7EntrySignalTriggerNear, sig.ReasonCodes)
	}
}

func TestFinalizeV7SignalAnnotatesFlowConfirmedBreakoutTriggerNear(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:         "NEARUSDT",
		Direction:      V7DirLong,
		SetupType:      V7SetupTrendBreakoutLong,
		Status:         V7StatusWaitConfirm,
		SetupScore:     76.8,
		TimingScore:    25,
		RiskScore:      8,
		LiquidityScore: 85,
		EntryMode:      V7EntryBreakout,
		ReasonCodes:    []string{"approaching_breakout", "flow_taker_buy_aggressive", "clear_air_above"},
		EntryZone:      V7PriceZone{Lower: 98.8, Upper: 100},
		Invalidation:   V7InvalidationRule{Price: 97.8},
		Targets:        []V7Target{{Price: 105}},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 99.55,
		TakerBuy15m:  0.64,
		Snapshot:     &SymbolSnapshotData{OIDelta1h: 0.3},
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.EntrySignal != V7EntrySignalTriggerNear {
		t.Fatalf("entry_signal = %q, want %q", sig.EntrySignal, V7EntrySignalTriggerNear)
	}
}

func TestTrendBreakoutEntryZoneUsesFixedBreakoutTrigger(t *testing.T) {
	mod := &trendBreakoutLongModule{}
	ctx := &V7SymbolContext{
		Symbol:            "TAOUSDT",
		CurrentPrice:      200.39,
		BBWidthPercentile: 12,
		BBUpper15m:        200.36,
		BBMiddle15m:       199.70,
		BBLower15m:        199.04,
		ATR15m:            0.98,
		ATR4h:             4.0,
		ATR1d:             6.0,
		High1d:            215,
		TakerBuy15m:       0.62,
		Snapshot:          &SymbolSnapshotData{OIDelta1h: 0.2, QuoteVolume24h: 90_000_000},
	}
	sig := mod.Score(ctx, V7RegimeTrendUp)
	if sig == nil {
		t.Fatal("expected trend breakout signal")
	}
	if sig.EntryZone.Upper != ctx.BBUpper15m {
		t.Fatalf("entry zone upper = %.8f, want fixed BB upper %.8f", sig.EntryZone.Upper, ctx.BBUpper15m)
	}
	if sig.EntryZone.Upper > ctx.CurrentPrice {
		t.Fatalf("entry zone upper %.8f should not chase above current %.8f", sig.EntryZone.Upper, ctx.CurrentPrice)
	}
}

func TestFinalizeV7SignalPromotesConfirmedBreakoutToOpenNow(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:         "TAOUSDT",
		Direction:      V7DirLong,
		SetupType:      V7SetupTrendBreakoutLong,
		Status:         V7StatusCandidate,
		SetupScore:     82,
		TimingScore:    45,
		RiskScore:      0,
		LiquidityScore: 85,
		EntryMode:      V7EntryBreakout,
		ReasonCodes:    []string{"breakout_attempt", "flow_taker_buy_aggressive", "clear_air_above"},
		EntryZone:      V7PriceZone{Lower: 99, Upper: 100},
		Invalidation:   V7InvalidationRule{Price: 98},
		Targets:        []V7Target{{Price: 105}},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 100.4,
		TakerBuy15m:  0.62,
		Snapshot:     &SymbolSnapshotData{OIDelta1h: 0.2},
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.EntrySignal != V7EntrySignalOpenNow {
		t.Fatalf("entry_signal = %q, want %q", sig.EntrySignal, V7EntrySignalOpenNow)
	}
	if sig.ConfirmSummary == nil || !sig.ConfirmSummary.PassedReview {
		t.Fatalf("confirmation summary should pass review: %+v", sig.ConfirmSummary)
	}
}

func TestFinalizeV7SignalBlocksLeaderMomentumUpperZoneChase(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:           "SKYAIUSDT",
		Direction:        V7DirLong,
		SetupType:        V7SetupLeaderMomentumLong,
		Status:           V7StatusCandidate,
		SetupScore:       100,
		TimingScore:      78,
		RiskScore:        0,
		LiquidityScore:   80,
		EntryMode:        V7EntryMomentumTrailing,
		ReasonCodes:      []string{"strong_24h_momentum", "solid_4h_momentum", "accelerating_1h", "oi_healthy_growth", "flow_taker_buy_strong", "no_pullback_still_running"},
		EntryZone:        V7PriceZone{Lower: 0.03549129456896678, Upper: 0.036259514536637456},
		Invalidation:     V7InvalidationRule{Price: 0.035329},
		Targets:          []V7Target{{Price: 0.03785941076677337}},
		RequiredConfirms: []string{"5m_price_holds_ema20_or_trailing_support", "momentum_not_exhausted", "taker_flow_not_flipping_against_direction"},
	}
	ctx := &V7SymbolContext{
		CurrentPrice:   0.03605,
		RSI1h:          72.5,
		TakerBuy15m:    0.5845970211937053,
		VWAP15m:        0.033929655667319744,
		VolumeBurst15m: 0.28,
		BBUpper15m:     0.036512,
		Snapshot:       &SymbolSnapshotData{OIDelta1h: -3.0062172399272358, OIDelta4h: 7.867981490480971},
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality != V7ExecChaseRisk {
		t.Fatalf("execution_quality = %q, want %q", sig.ExecutionQuality, V7ExecChaseRisk)
	}
	if sig.EntrySignal != V7EntrySignalChaseRisk {
		t.Fatalf("entry_signal = %q, want %q", sig.EntrySignal, V7EntrySignalChaseRisk)
	}
	if !containsV7String(sig.RiskTags, "momentum_upper_zone_chase") || !containsV7String(sig.RiskTags, "do_not_market_chase") {
		t.Fatalf("missing upper-zone chase risk tags: %+v", sig.RiskTags)
	}
	if !containsV7String(sig.ReasonCodes, "leader_momentum_upper_chase_wait") {
		t.Fatalf("missing upper-zone chase reason: %+v", sig.ReasonCodes)
	}
}

func TestBreakdownMomentumShortModuleCapturesDownsideContinuation(t *testing.T) {
	mod := &breakdownMomentumShortModule{}
	ctx := &V7SymbolContext{
		Symbol:         "SKYAIUSDT",
		CurrentPrice:   0.03018,
		Change1h:       -6.4,
		Change4h:       -12.2,
		Change24h:      -2.14,
		ATR1h:          0.0012,
		ATR4h:          0.0021,
		ATR15m:         0.00048,
		ATR1d:          0.006,
		Low1d:          0.027,
		VWAP15m:        0.0328,
		EMA20_1h:       0.0332,
		BBMiddle15m:    0.0317,
		TakerBuy15m:    0.41,
		Velocity15m:    -1.6,
		VolumeBurst15m: 1.6,
		Snapshot:       &SymbolSnapshotData{OIDelta1h: 1.8, QuoteVolume24h: 22_398_888, OI: 4_000_000, TradeCount24h: 372_068},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected breakdown momentum short signal")
	}
	if sig.Direction != V7DirShort || sig.SetupType != V7SetupBreakdownShort {
		t.Fatalf("unexpected signal: dir=%s setup=%s", sig.Direction, sig.SetupType)
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality != V7ExecReady {
		t.Fatalf("execution_quality = %q, want %q", sig.ExecutionQuality, V7ExecReady)
	}
	if sig.EntrySignal != V7EntrySignalOpenNow {
		t.Fatalf("entry_signal = %q, want %q", sig.EntrySignal, V7EntrySignalOpenNow)
	}
	if !containsV7String(sig.ReasonCodes, "below_vwap_breakdown") ||
		!containsV7String(sig.ReasonCodes, "heavy_taker_selling") {
		t.Fatalf("missing breakdown evidence: %+v", sig.ReasonCodes)
	}
}

func TestMMSBottomWakeModuleDetectsQuietAccumulation(t *testing.T) {
	mod := &mmsBottomWakeLongModule{}
	ctx := &V7SymbolContext{
		Symbol:            "MMSAUSDT",
		CurrentPrice:      1.00,
		Change4h:          1.2,
		ATR15m:            0.01,
		ATR4h:             0.04,
		BBUpper15m:        1.01,
		BBLower15m:        0.98,
		BBWidthPercentile: 12,
		StdRatio1h72:      0.014,
		VolumeBurst1h:     3.2,
		TakerBuy15m:       0.52,
		Snapshot:          &SymbolSnapshotData{OIDelta1h: 2.5, OIDelta4h: 18, QuoteVolume24h: 18_000_000, OI: 2_000_000, TradeCount24h: 80_000},
	}

	sig := mod.Score(ctx, V7RegimeCompression)
	if sig == nil {
		t.Fatal("expected MMS bottom wake signal")
	}
	if sig.SetupType != V7SetupMMSBottomWakeLong {
		t.Fatalf("setup = %s, want %s", sig.SetupType, V7SetupMMSBottomWakeLong)
	}
	if !containsV7String(sig.ReasonCodes, "mms_bottom_wake") || !containsV7String(sig.RiskTags, "mms_breakout_not_confirmed") {
		t.Fatalf("missing MMS bottom wake tags: reasons=%v risks=%v", sig.ReasonCodes, sig.RiskTags)
	}
}

func TestMMSTrendRideModuleDetectsEMARetest(t *testing.T) {
	mod := &mmsTrendRideLongModule{}
	ctx := &V7SymbolContext{
		Symbol:         "MMSBUSDT",
		CurrentPrice:   1.05,
		Change1h:       1.4,
		Change4h:       6.0,
		ATR15m:         0.012,
		EMA7_15m:       1.052,
		EMA25_15m:      1.040,
		EMA99_15m:      1.000,
		Last15mLow:     1.043,
		Last15mClose:   1.050,
		VolumeBurst15m: 0.62,
		TakerBuy15m:    0.57,
		RSI1h:          64,
		Snapshot:       &SymbolSnapshotData{OIDelta1h: 1.2, QuoteVolume24h: 22_000_000, OI: 3_000_000, TradeCount24h: 90_000},
	}

	sig := mod.Score(ctx, V7RegimeTrendUp)
	if sig == nil {
		t.Fatal("expected MMS trend ride signal")
	}
	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())
	if sig.ExecutionQuality != V7ExecReady {
		t.Fatalf("execution_quality = %s, want %s", sig.ExecutionQuality, V7ExecReady)
	}
	if !containsV7String(sig.ReasonCodes, "mms_ema25_retest_hold") {
		t.Fatalf("missing EMA retest tag: %v", sig.ReasonCodes)
	}
}

func TestMMSSqueezeEngineModuleAllowsLongAndCarriesShortBanContext(t *testing.T) {
	mod := &mmsSqueezeEngineLongModule{}
	ctx := &V7SymbolContext{
		Symbol:         "MMSCUSDT",
		CurrentPrice:   1.20,
		Change1h:       4.2,
		ATR15m:         0.018,
		EMA25_1h:       1.12,
		TakerBuy15m:    0.57,
		VolumeBurst15m: 1.4,
		Snapshot:       &SymbolSnapshotData{LSR: 1.72, OIDelta1h: 12, QuoteVolume24h: 30_000_000, OI: 6_000_000, TradeCount24h: 120_000},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected MMS squeeze engine signal")
	}
	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())
	if sig.ExecutionQuality != V7ExecReady {
		t.Fatalf("execution_quality = %s, want %s", sig.ExecutionQuality, V7ExecReady)
	}
	if !containsV7String(sig.ReasonCodes, "mms_short_ban_active") {
		t.Fatalf("missing short ban context: %v", sig.ReasonCodes)
	}
	if containsV7String(sig.RiskTags, "mms_do_not_short_squeeze") {
		t.Fatalf("squeeze long should not carry wait-only short-ban risk tag: %v", sig.RiskTags)
	}
}

func TestFinalizeV7SignalRepairsDisplacementRRTagAfterExecutionGeometry(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:           "DODOXUSDT",
		Direction:        V7DirLong,
		SetupType:        V7SetupDisplacementLong,
		Status:           V7StatusCandidate,
		SetupScore:       93.5,
		TimingScore:      70,
		RiskScore:        30,
		LiquidityScore:   90,
		EntryMode:        V7EntryMomentumTrailing,
		EntryZone:        V7PriceZone{Lower: 98.5, Upper: 100.3},
		Invalidation:     V7InvalidationRule{Price: 97.5},
		Targets:          []V7Target{{Price: 104.7}},
		RiskTags:         []string{"displacement_rr_insufficient", "displacement_chase_risk_overextended"},
		RequiredConfirms: []string{"price_holds_trailing_support"},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 100,
		TakerBuy15m:  0.53,
		Snapshot:     &SymbolSnapshotData{OIDelta1h: 11.8},
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if containsV7String(sig.RiskTags, "displacement_rr_insufficient") {
		t.Fatalf("unexpected stale rr tag after final RR repair: %+v", sig.RiskTags)
	}
	if !containsV7String(sig.RiskTags, "displacement_rr_repaired_needs_review") {
		t.Fatalf("missing repaired review tag: %+v", sig.RiskTags)
	}
	if sig.EntrySignal != V7EntrySignalRRRepairable {
		t.Fatalf("entry_signal = %q, want %q", sig.EntrySignal, V7EntrySignalRRRepairable)
	}
	if !containsV7String(sig.RequiredConfirms, "taker_buy_15m_stays_above_0_52") {
		t.Fatalf("missing displacement flow confirmation: %+v", sig.RequiredConfirms)
	}
}

func TestFinalizeV7SignalKeepsDisplacementRRRejectWhenFinalRRStillInsufficient(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:         "ZECUSDT",
		Direction:      V7DirLong,
		SetupType:      V7SetupDisplacementLong,
		Status:         V7StatusCandidate,
		SetupScore:     80,
		TimingScore:    70,
		RiskScore:      20,
		LiquidityScore: 90,
		EntryMode:      V7EntryMomentumTrailing,
		EntryZone:      V7PriceZone{Lower: 99, Upper: 100.3},
		Invalidation:   V7InvalidationRule{Price: 97.5},
		Targets:        []V7Target{{Price: 101.2}},
		RiskTags:       []string{"displacement_rr_insufficient"},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 100,
		TakerBuy15m:  0.57,
		Snapshot:     &SymbolSnapshotData{OIDelta1h: 2.2},
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if !containsV7String(sig.RiskTags, "displacement_rr_insufficient") {
		t.Fatalf("rr reject tag was removed despite insufficient final RR: %+v", sig.RiskTags)
	}
	if containsV7String(sig.RiskTags, "displacement_rr_repaired_needs_review") {
		t.Fatalf("unexpected repaired tag: %+v", sig.RiskTags)
	}
}

func TestAltLadderMomentumLongDetectsEarlyAltImpulse(t *testing.T) {
	mod := &altLadderMomentumLongModule{}
	ctx := &V7SymbolContext{
		Symbol:         "ALTAUSDT",
		CurrentPrice:   1.08,
		Change24h:      8.2,
		Change4h:       4.6,
		Change1h:       1.8,
		Velocity15m:    0.9,
		ATR15m:         0.012,
		ATR1h:          0.035,
		VWAP15m:        1.062,
		EMA20_1h:       1.045,
		BBMiddle15m:    1.055,
		TakerBuy15m:    0.57,
		VolumeBurst15m: 1.25,
		VolumeBurst1h:  1.15,
		RSI1h:          66,
		Snapshot:       &SymbolSnapshotData{OIDelta1h: 1.4, OIDelta4h: 4.2, QuoteVolume24h: 18_000_000, FundingRate: 0.0001},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected alt ladder long signal")
	}
	if sig.SetupType != V7SetupAltLadderLong || sig.Direction != V7DirLong {
		t.Fatalf("unexpected signal: setup=%s dir=%s", sig.SetupType, sig.Direction)
	}
	if !containsV7String(sig.ReasonCodes, "alt_ladder_stage_early") ||
		!containsV7String(sig.ReasonCodes, "alt_ladder_taker_buy") {
		t.Fatalf("missing early alt ladder evidence: %+v", sig.ReasonCodes)
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality != V7ExecReady {
		t.Fatalf("execution_quality = %s, want ready (reasons=%v risks=%v summary=%+v)", sig.ExecutionQuality, sig.ReasonCodes, sig.RiskTags, sig.ConfirmSummary)
	}
}

func TestAltLadderMomentumLongFlagsLateStageChaseRisk(t *testing.T) {
	mod := &altLadderMomentumLongModule{}
	ctx := &V7SymbolContext{
		Symbol:         "ALTBUSDT",
		CurrentPrice:   1.30,
		Change24h:      31,
		Change4h:       14,
		Change1h:       7.4,
		Velocity15m:    2.1,
		ATR15m:         0.018,
		ATR1h:          0.045,
		VWAP15m:        1.276,
		EMA20_1h:       1.21,
		BBMiddle15m:    1.25,
		TakerBuy15m:    0.59,
		VolumeBurst15m: 1.55,
		VolumeBurst1h:  1.4,
		RSI1h:          75,
		Snapshot:       &SymbolSnapshotData{OIDelta1h: 4.5, OIDelta4h: 12, QuoteVolume24h: 46_000_000, FundingRate: 0.0002},
	}

	sig := mod.Score(ctx, V7RegimeManiaPump)
	if sig == nil {
		t.Fatal("expected late-stage alt ladder long signal")
	}
	if !containsV7String(sig.ReasonCodes, "alt_ladder_stage_late") ||
		!containsV7String(sig.RiskTags, "alt_ladder_late_chase_risk") {
		t.Fatalf("missing late-stage chase semantics: reasons=%v risks=%v", sig.ReasonCodes, sig.RiskTags)
	}
}

func TestAltLadderMomentumLongTracksExtremeStageAsPullbackWatch(t *testing.T) {
	mod := &altLadderMomentumLongModule{}
	ctx := &V7SymbolContext{
		Symbol:         "AKEUSDT",
		CurrentPrice:   0.00192,
		Change24h:      93,
		Change4h:       18,
		Change1h:       1.8,
		Velocity15m:    -0.5,
		ATR15m:         0.00005,
		ATR1h:          0.00018,
		VWAP15m:        0.00180,
		EMA7_15m:       0.00188,
		EMA25_15m:      0.00173,
		EMA99_15m:      0.00143,
		EMA20_1h:       0.00155,
		BBMiddle15m:    0.00170,
		TakerBuy15m:    0.49,
		VolumeBurst15m: 0.15,
		VolumeBurst1h:  0.25,
		RSI1h:          78,
		Snapshot:       &SymbolSnapshotData{OIDelta1h: 6.8, OIDelta4h: 28, QuoteVolume24h: 1_000_000_000, FundingRate: 0.0011},
	}

	sig := mod.Score(ctx, V7RegimeTrendUp)
	if sig == nil {
		t.Fatal("expected extreme-stage alt ladder watch signal")
	}
	if !containsV7String(sig.ReasonCodes, "alt_ladder_stage_extreme") ||
		!containsV7String(sig.RiskTags, "alt_ladder_extreme_continuation_watch") {
		t.Fatalf("missing extreme-stage semantics: reasons=%v risks=%v", sig.ReasonCodes, sig.RiskTags)
	}
	if sig.EntryZone.Upper >= ctx.CurrentPrice {
		t.Fatalf("extreme-stage entry zone should wait below current price: zone=%+v current=%.8f", sig.EntryZone, ctx.CurrentPrice)
	}
}

func TestAltLadderBreakdownShortDetectsEarlyBreakdown(t *testing.T) {
	mod := &altLadderBreakdownShortModule{}
	ctx := &V7SymbolContext{
		Symbol:         "ALTCUSDT",
		CurrentPrice:   0.92,
		Change24h:      -6.5,
		Change4h:       -5.2,
		Change1h:       -2.4,
		Velocity15m:    -1.1,
		ATR15m:         0.010,
		ATR1h:          0.030,
		ATR1d:          0.18,
		Low1d:          0.78,
		VWAP15m:        0.944,
		EMA20_1h:       0.955,
		BBMiddle15m:    0.936,
		TakerBuy15m:    0.44,
		VolumeBurst15m: 1.35,
		Snapshot:       &SymbolSnapshotData{OIDelta1h: 1.2, OIDelta4h: 2.5, QuoteVolume24h: 20_000_000},
	}

	sig := mod.Score(ctx, V7RegimePullback)
	if sig == nil {
		t.Fatal("expected alt ladder breakdown short signal")
	}
	if sig.SetupType != V7SetupAltLadderShort || sig.Direction != V7DirShort {
		t.Fatalf("unexpected signal: setup=%s dir=%s", sig.SetupType, sig.Direction)
	}
	if !containsV7String(sig.ReasonCodes, "alt_ladder_downshift_early") ||
		!containsV7String(sig.ReasonCodes, "alt_ladder_taker_sell") {
		t.Fatalf("missing early downshift evidence: %+v", sig.ReasonCodes)
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality != V7ExecReady {
		t.Fatalf("execution_quality = %s, want ready (reasons=%v risks=%v summary=%+v)", sig.ExecutionQuality, sig.ReasonCodes, sig.RiskTags, sig.ConfirmSummary)
	}
}
