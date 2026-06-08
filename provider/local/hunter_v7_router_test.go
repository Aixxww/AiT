package local

import (
	"strings"
	"testing"
)

func TestDefaultV7ConfirmationsDoNotTreatEntryZoneAsReclaim(t *testing.T) {
	tests := []struct {
		name      string
		direction V7Direction
		entryMode V7EntryMode
		want      string
	}{
		{
			name:      "long wait reclaim",
			direction: V7DirLong,
			entryMode: V7EntryWaitReclaim,
			want:      "15m_close_above_vwap_or_ema20_or_entry_zone_upper",
		},
		{
			name:      "short wait reclaim",
			direction: V7DirShort,
			entryMode: V7EntryWaitReclaim,
			want:      "15m_close_below_vwap_or_ema20_or_entry_zone_lower",
		},
		{
			name:      "long wait price reversal",
			direction: V7DirLong,
			entryMode: V7EntryWaitPriceReversal,
			want:      "15m_close_above_vwap_or_ema20_or_entry_zone_upper",
		},
		{
			name:      "short wait price reversal",
			direction: V7DirShort,
			entryMode: V7EntryWaitPriceReversal,
			want:      "15m_close_below_vwap_or_ema20_or_entry_zone_lower",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirms := defaultV7Confirmations(&V7SignalOutput{
				Direction: tt.direction,
				EntryMode: tt.entryMode,
			})
			if len(confirms) == 0 || confirms[0] != tt.want {
				t.Fatalf("first confirmation = %q, want %q", confirms, tt.want)
			}
			for _, confirm := range confirms {
				if strings.Contains(confirm, "reclaim_vwap_or_entry_zone") {
					t.Fatalf("ambiguous confirmation remains: %q", confirm)
				}
			}
		})
	}
}

func TestFundingReversalDoesNotChaseShortAfterFastDropWithBuildingOI(t *testing.T) {
	mod := &fundingReversalModule{}
	ctx := &V7SymbolContext{
		Symbol:       "HUSDT",
		CurrentPrice: 0.63364,
		Change1h:     -5.77,
		Change4h:     7.28,
		Change24h:    -18.09,
		ATR15m:       0.0253,
		TakerBuy15m:  0.396,
		VWAP15m:      0.63576,
		High4h:       0.75672,
		ATR4h:        0.06255,
		Snapshot: &SymbolSnapshotData{
			FundingRate: 0.00005,
			LSR:         2.42,
			OIDelta1h:   9.86,
			OIDelta4h:   13.23,
		},
	}

	if sig := mod.Score(ctx, V7RegimeTrendDown); sig != nil {
		t.Fatalf("expected late short with building OI to be filtered before scoring, got %+v", sig)
	}
}

func TestRouterUsesGlobalMinPriorityForCandidateVisibility(t *testing.T) {
	signals := []V7SignalOutput{
		{
			Symbol:     "BREAKUSDT",
			Direction:  V7DirLong,
			SetupType:  V7SetupTrendBreakoutLong,
			Status:     V7StatusCandidate,
			AIPriority: 52,
		},
	}

	got := filterV7SignalsForLLM(signals, V7Config{
		MaxOutput:             10,
		MinOutput:             3,
		MinAIPriority:         50,
		FallbackMinAIPriority: 45,
		SetupThresholds:       DefaultSetupThresholds(),
	})

	if len(got) != 1 {
		t.Fatalf("signals = %d, want 1", len(got))
	}
	if got[0].Symbol != "BREAKUSDT" {
		t.Fatalf("symbol = %s, want BREAKUSDT", got[0].Symbol)
	}
}

func TestRouterBackfillsContextCandidatesWhenPrimaryPoolIsThin(t *testing.T) {
	signals := []V7SignalOutput{
		{Symbol: "AUSDT", Direction: V7DirLong, SetupType: V7SetupPanicReversalLong, Status: V7StatusCandidate, AIPriority: 57},
		{Symbol: "BUSDT", Direction: V7DirShort, SetupType: V7SetupDistributionShort, Status: V7StatusCandidate, AIPriority: 49},
		{Symbol: "CUSDT", Direction: V7DirLong, SetupType: V7SetupPullbackLong, Status: V7StatusCandidate, AIPriority: 46},
		{Symbol: "DUSDT", Direction: V7DirLong, SetupType: V7SetupLeaderMomentumLong, Status: V7StatusCandidate, AIPriority: 43},
		{Symbol: "EUSDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, Status: V7StatusFiltered, AIPriority: 80},
	}

	got := filterV7SignalsForLLM(signals, V7Config{
		MaxOutput:             10,
		MinOutput:             3,
		MinAIPriority:         50,
		FallbackMinAIPriority: 45,
	})

	if len(got) != 3 {
		t.Fatalf("signals = %d, want 3", len(got))
	}
	wantSymbols := []string{"AUSDT", "BUSDT", "CUSDT"}
	for i, want := range wantSymbols {
		if got[i].Symbol != want {
			t.Fatalf("signal[%d] = %s, want %s", i, got[i].Symbol, want)
		}
	}
	for _, sig := range got {
		if sig.Symbol == "EUSDT" {
			t.Fatal("hard-filtered signal was backfilled")
		}
		if sig.AIPriority < 50 && !containsString(sig.RiskTags, "context_only_low_priority") {
			t.Fatalf("backfill signal %s missing context risk tag: %+v", sig.Symbol, sig.RiskTags)
		}
	}
}

func TestRouterBackfillFallbackTracksLoweredMinPriority(t *testing.T) {
	signals := []V7SignalOutput{
		{Symbol: "AUSDT", Direction: V7DirLong, SetupType: V7SetupTrendBreakoutLong, Status: V7StatusCandidate, AIPriority: 46},
		{Symbol: "BUSDT", Direction: V7DirLong, SetupType: V7SetupTrendBreakoutLong, Status: V7StatusCandidate, AIPriority: 41},
		{Symbol: "CUSDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, Status: V7StatusCandidate, AIPriority: 38},
	}

	got := filterV7SignalsForLLM(signals, V7Config{
		MaxOutput:             10,
		MinOutput:             3,
		MinAIPriority:         45,
		FallbackMinAIPriority: 45,
	})

	if len(got) != 3 {
		t.Fatalf("signals = %d, want 3", len(got))
	}
	for _, sig := range got {
		if sig.AIPriority < 45 && !containsString(sig.RiskTags, "context_only_low_priority") {
			t.Fatalf("fallback signal %s missing context tag: %+v", sig.Symbol, sig.RiskTags)
		}
	}
}

func TestV7ExecutionQualityDowngradesLowTimingWaitSetups(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:       "SLOWUSDT",
		Direction:    V7DirLong,
		SetupType:    V7SetupPanicReversalLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryWaitReclaim,
		TimingScore:  30,
		RiskScore:    20,
		EntryZone:    V7PriceZone{Lower: 99, Upper: 102},
		Invalidation: V7InvalidationRule{Price: 98},
		Targets:      []V7Target{{Price: 104}},
	}
	ctx := &V7SymbolContext{CurrentPrice: 100}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality != V7ExecWatchOnly {
		t.Fatalf("execution quality = %s, want %s", sig.ExecutionQuality, V7ExecWatchOnly)
	}
	if sig.Status != V7StatusWaitConfirm {
		t.Fatalf("status = %s, want %s", sig.Status, V7StatusWaitConfirm)
	}
	if !containsString(sig.ReasonCodes, "low_timing_watch_only") {
		t.Fatalf("missing low timing reason: %+v", sig.ReasonCodes)
	}
}

func TestV7ExecutionQualityDowngradesLowTimingLeaderMomentum(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:       "BLESSUSDT",
		Direction:    V7DirLong,
		SetupType:    V7SetupLeaderMomentumLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryMomentumTrailing,
		TimingScore:  48,
		RiskScore:    15,
		SetupScore:   74,
		EntryZone:    V7PriceZone{Lower: 99, Upper: 102},
		Invalidation: V7InvalidationRule{Price: 98},
		Targets:      []V7Target{{Price: 104}},
	}
	ctx := &V7SymbolContext{CurrentPrice: 100, Change1h: 2.0}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality != V7ExecWatchOnly {
		t.Fatalf("execution quality = %s, want %s", sig.ExecutionQuality, V7ExecWatchOnly)
	}
	if sig.Status != V7StatusWaitConfirm {
		t.Fatalf("status = %s, want %s", sig.Status, V7StatusWaitConfirm)
	}
	if !containsString(sig.ReasonCodes, "leader_momentum_timing_watch_only") {
		t.Fatalf("missing leader timing reason: %+v", sig.ReasonCodes)
	}
	if !containsString(sig.RiskTags, "momentum_confirmation_missing") {
		t.Fatalf("missing confirmation tag: %+v", sig.RiskTags)
	}
}

func TestV7ExecutionQualityUsesValidDirectionalTargetForRR(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:       "TARGETUSDT",
		Direction:    V7DirLong,
		SetupType:    V7SetupPanicReversalLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryImmediate,
		TimingScore:  65,
		RiskScore:    25,
		EntryZone:    V7PriceZone{Lower: 99, Upper: 102},
		Invalidation: V7InvalidationRule{Price: 98},
		Targets: []V7Target{
			{Price: 99, Reason: "expired_vwap"},
			{Price: 104, Reason: "valid_extension"},
		},
	}
	ctx := &V7SymbolContext{CurrentPrice: 100}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality == V7ExecInvalidRR {
		t.Fatalf("execution quality = %s, want valid RR from later target", sig.ExecutionQuality)
	}
	if sig.Targets[0].Price != 104 {
		t.Fatalf("first target = %.2f, want valid target promoted", sig.Targets[0].Price)
	}
	if containsString(sig.RiskTags, "invalid_rr_context_only") {
		t.Fatalf("unexpected invalid_rr tag: %+v", sig.RiskTags)
	}
}

func TestV7ExecutionQualityTightensOverWideInvalidationForScoring(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:       "STOPUSDT",
		Direction:    V7DirLong,
		SetupType:    V7SetupPanicReversalLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryImmediate,
		TimingScore:  65,
		RiskScore:    25,
		EntryZone:    V7PriceZone{Lower: 99, Upper: 102},
		Invalidation: V7InvalidationRule{Price: 80, Reason: "far_4h_low"},
		Targets:      []V7Target{{Price: 104, Reason: "valid_extension"}},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 100,
		ATR15m:       1,
		ATR1h:        3,
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality == V7ExecInvalidRR {
		t.Fatalf("execution quality = %s, want valid RR after near stop tightening", sig.ExecutionQuality)
	}
	if sig.Invalidation.Price >= 99 || sig.Invalidation.Price <= 97 {
		t.Fatalf("invalidation price = %.2f, want near execution stop around 98", sig.Invalidation.Price)
	}
	if !containsString(sig.RiskTags, "execution_stop_tightened") {
		t.Fatalf("missing execution_stop_tightened tag: %+v", sig.RiskTags)
	}
}

func TestCalcAIPriorityUsesEmpiricalSetupAndExecutionBias(t *testing.T) {
	base := V7SignalOutput{
		Status:         V7StatusCandidate,
		SetupScore:     60,
		TimingScore:    65,
		RegimeFitScore: 65,
		LiquidityScore: 70,
		RiskScore:      25,
	}
	panicLong := base
	panicLong.Direction = V7DirLong
	panicLong.SetupType = V7SetupPanicReversalLong
	panicLong.ExecutionQuality = V7ExecReady

	fundingLong := base
	fundingLong.Direction = V7DirLong
	fundingLong.SetupType = V7SetupFundingReversal
	fundingLong.MarketRegime = V7RegimeTrendDown
	fundingLong.ExecutionQuality = V7ExecWatchOnly

	panicPriority := CalcAIPriority(&panicLong, DefaultV7Config())
	fundingPriority := CalcAIPriority(&fundingLong, DefaultV7Config())
	if panicPriority <= fundingPriority {
		t.Fatalf("panic priority %.2f should rank above weak funding long %.2f", panicPriority, fundingPriority)
	}
	if panicPriority-fundingPriority < 20 {
		t.Fatalf("priority gap %.2f too small for empirical setup bias", panicPriority-fundingPriority)
	}
}

func TestDiversifyV7SignalsPromotesSetupCoverageBeforeFilling(t *testing.T) {
	signals := []V7SignalOutput{
		{Symbol: "F1USDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, AIPriority: 90},
		{Symbol: "F2USDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, AIPriority: 89},
		{Symbol: "F3USDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, AIPriority: 88},
		{Symbol: "F4USDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, AIPriority: 87},
		{Symbol: "F5USDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, AIPriority: 86},
		{Symbol: "P1USDT", Direction: V7DirLong, SetupType: V7SetupPanicReversalLong, AIPriority: 70},
	}

	got := diversifyV7Signals(signals, 4)
	if len(got) != 4 {
		t.Fatalf("signals = %d, want 4", len(got))
	}
	if !containsV7Symbol(got, "P1USDT") {
		t.Fatalf("diversified output should include alternate setup, got %+v", got)
	}
}

func TestPreMoveRadarBuildsWatchOnlySignals(t *testing.T) {
	ctx := V7SymbolContext{
		Symbol:            "EARLYUSDT",
		CurrentPrice:      99.5,
		Change1h:          0.8,
		Change4h:          1.6,
		Change24h:         5.2,
		ATR15m:            1.0,
		ATR1h:             2.5,
		ATR4h:             5.0,
		BBWidthPercentile: 10,
		BBUpper15m:        100,
		BBMiddle15m:       97,
		BBLower15m:        94,
		TakerBuy15m:       0.54,
		VWAP15m:           98.8,
		High1h:            100,
		Low1h:             94.5,
		High4h:            104,
		Low4h:             93,
		Snapshot: &SymbolSnapshotData{
			Price:          99.5,
			QuoteVolume24h: 80_000_000,
			TradeCount24h:  120_000,
			OI:             12_000_000,
			OIDelta1h:      5,
			OIDelta4h:      13,
			FundingRate:    0.0001,
			LSR:            1.05,
		},
	}
	cfg := DefaultV7Config()
	cfg.WatchOutput = 3

	got := BuildV7PreMoveRadar([]V7SymbolContext{ctx}, V7RegimeCompression, cfg)
	if len(got) == 0 {
		t.Fatal("expected at least one pre-move watch signal")
	}
	sig := got[0]
	if sig.Status != V7StatusWaitConfirm {
		t.Fatalf("status = %s, want %s", sig.Status, V7StatusWaitConfirm)
	}
	if sig.ExecutionQuality != V7ExecWatchOnly {
		t.Fatalf("execution quality = %s, want %s", sig.ExecutionQuality, V7ExecWatchOnly)
	}
	if !containsString(sig.RiskTags, "do_not_open_until_confirmed") {
		t.Fatalf("missing do_not_open_until_confirmed tag: %+v", sig.RiskTags)
	}
	if len(sig.RequiredConfirms) == 0 {
		t.Fatal("watch signal missing required confirmations")
	}
}

func TestFundingShortWeak4hFlushNearLowerZoneStaysWatch(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:           "PARTIUSDT",
		Direction:        V7DirShort,
		SetupType:        V7SetupFundingReversal,
		Status:           V7StatusCandidate,
		Confidence:       "C",
		TimingScore:      80,
		RiskScore:        15,
		ExecutionQuality: V7ExecNearConfirm,
		RiskTags:         []string{"not_near_short_retest_zone"},
		EntryZone:        V7PriceZone{Lower: 0.054011, Upper: 0.054675},
		Invalidation:     V7InvalidationRule{Price: 0.055345},
		Targets:          []V7Target{{Price: 0.05209}},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 0.05426,
		ATR15m:       0.00083,
		ATR1h:        0.001819,
		Snapshot: &SymbolSnapshotData{
			OIDelta1h: -3.80,
			OIDelta4h: -0.50,
		},
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if sig.ExecutionQuality != V7ExecWatchOnly {
		t.Fatalf("execution quality = %s, want %s (tags %+v reasons %+v)", sig.ExecutionQuality, V7ExecWatchOnly, sig.RiskTags, sig.ReasonCodes)
	}
	if !containsString(sig.RiskTags, "weak_4h_oi_flush") {
		t.Fatalf("missing weak 4h flush tag: %+v", sig.RiskTags)
	}
	if !containsString(sig.ReasonCodes, "funding_short_weak_4h_flush_wait") {
		t.Fatalf("missing weak 4h flush reason: %+v", sig.ReasonCodes)
	}
}

func TestFundingShortStrong4hFlushAvoidsWeakFlushTag(t *testing.T) {
	sig := &V7SignalOutput{
		Symbol:           "OPENUSDT",
		Direction:        V7DirShort,
		SetupType:        V7SetupFundingReversal,
		Status:           V7StatusCandidate,
		Confidence:       "C",
		TimingScore:      72,
		RiskScore:        15,
		ExecutionQuality: V7ExecNearConfirm,
		RiskTags:         []string{"not_near_short_retest_zone"},
		EntryZone:        V7PriceZone{Lower: 0.19794, Upper: 0.19969},
		Invalidation:     V7InvalidationRule{Price: 0.20257},
		Targets:          []V7Target{{Price: 0.1907}},
	}
	ctx := &V7SymbolContext{
		CurrentPrice: 0.1986,
		ATR15m:       0.002188,
		ATR1h:        0.004669,
		Snapshot: &SymbolSnapshotData{
			OIDelta1h: -0.39,
			OIDelta4h: -3.15,
		},
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())

	if containsString(sig.RiskTags, "weak_4h_oi_flush") {
		t.Fatalf("unexpected weak 4h flush tag: %+v", sig.RiskTags)
	}
	if containsString(sig.ReasonCodes, "funding_short_weak_4h_flush_wait") {
		t.Fatalf("unexpected weak 4h flush reason: %+v", sig.ReasonCodes)
	}
}

func TestAppendV7WatchSignalsDoesNotConsumeConfirmedMaxOutput(t *testing.T) {
	confirmed := []V7SignalOutput{
		{Symbol: "READY1USDT", Direction: V7DirLong, SetupType: V7SetupPanicReversalLong, AIPriority: 80},
		{Symbol: "READY2USDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, AIPriority: 75},
	}
	watches := []V7SignalOutput{
		{Symbol: "WATCH1USDT", Direction: V7DirLong, SetupType: V7SetupPreBreakoutWatch, Status: V7StatusWaitConfirm, ExecutionQuality: V7ExecWatchOnly, AIPriority: 55},
		{Symbol: "READY1USDT", Direction: V7DirLong, SetupType: V7SetupAccumulationWatch, Status: V7StatusWaitConfirm, ExecutionQuality: V7ExecWatchOnly, AIPriority: 54},
	}
	cfg := DefaultV7Config()
	cfg.MaxOutput = 2
	cfg.WatchOutput = 1

	got := appendV7WatchSignals(confirmed, watches, cfg)
	if len(got) != 3 {
		t.Fatalf("signals = %d, want 3", len(got))
	}
	if got[0].Symbol != "READY1USDT" || got[1].Symbol != "READY2USDT" || got[2].Symbol != "WATCH1USDT" {
		t.Fatalf("unexpected output order/symbols: %+v", got)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsV7Symbol(values []V7SignalOutput, want string) bool {
	for _, value := range values {
		if value.Symbol == want {
			return true
		}
	}
	return false
}
