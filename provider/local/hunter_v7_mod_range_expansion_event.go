package local

import "math"

// Module: Range Expansion Event
//
// Captures 24h high-amplitude movers that are already visible on Binance's
// mover board but may not fit legacy pullback/breakout/reversal modules.
// It is intentionally event-scoped: recall is improved without loosening every
// setup threshold globally.
type rangeExpansionEventModule struct{}

func (m *rangeExpansionEventModule) Name() string           { return "range_expansion_event" }
func (m *rangeExpansionEventModule) SetupType() V7SetupType { return V7SetupRangeExpansion }
func (m *rangeExpansionEventModule) Direction() V7Direction { return V7DirLong }

func (m *rangeExpansionEventModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil || ctx.Snapshot == nil || ctx.CurrentPrice <= 0 {
		return false
	}
	if ctx.Amplitude24h < 20 && !(ctx.Amplitude24h >= 15 && ctx.RangeExpansion1h >= 2.2 && math.Abs(ctx.Velocity15m) >= 1.5) {
		return false
	}
	if ctx.RangeExpansion1h < 1.6 && math.Abs(ctx.Velocity15m) < 2.0 && math.Abs(ctx.Change1h) < 3.0 {
		return false
	}
	if ctx.VolumeBurst15m > 0 && ctx.VolumeBurst15m < 0.8 && math.Abs(ctx.Change1h) < 3.5 {
		return false
	}
	return true
}

func (m *rangeExpansionEventModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}
	dir := rangeExpansionEventDirection(ctx)
	s := newV7Signal(ctx, regime, V7SetupRangeExpansion, dir, V7EntryFastConfirm, "B")
	s.reason("range_expansion_event")
	sig := s.sig

	s.add(20)
	switch {
	case ctx.Amplitude24h >= 45:
		s.add(24, "amplitude_24h_extreme")
	case ctx.Amplitude24h >= 30:
		s.add(18, "amplitude_24h_major")
	default:
		s.add(12, "amplitude_24h_event")
	}
	switch {
	case ctx.RangeExpansion1h >= 3.0:
		s.add(22, "massive_range_expansion_event")
	case ctx.RangeExpansion1h >= 2.2:
		s.add(16, "strong_range_expansion_event")
	default:
		s.add(10, "moderate_range_expansion_event")
	}
	impulse := ctx.Change1h
	velocity := ctx.Velocity15m
	if dir == V7DirShort {
		impulse = -impulse
		velocity = -velocity
	}
	if impulse >= 4 || velocity >= 2 {
		if dir == V7DirShort {
			s.add(16, "event_breakdown_short")
		} else {
			s.add(16, "event_continuation_long")
		}
	} else if impulse >= 1 || velocity >= 0.8 {
		s.add(10, "event_directional_followthrough")
	}
	if ctx.VolumeBurst15m >= 2.5 {
		s.add(12, "volume_burst_15m")
	} else if ctx.VolumeBurst5m >= 2.0 {
		s.add(8, "volume_burst_5m")
	}
	if rangeExpansionFlowAligned(ctx, dir) {
		if dir == V7DirShort {
			s.add(12, "taker_sell_aligned")
		} else {
			s.add(12, "taker_buy_aligned")
		}
	} else {
		s.riskTag("event_flow_confirmation_needed")
		sig.ExecutionQuality = V7ExecNearConfirm
		sig.Confidence = "C"
	}

	// Handwritten tail: the subtype/quality passes below mutate TimingScore
	// after it is first assigned, so the shared finish helpers cannot own it.
	sig.SetupScore = clampFloat(s.score, 0, 100)
	sig.TimingScore = rangeExpansionTimingScore(ctx, dir)
	if sig.TimingScore < 50 {
		sig.ExecutionQuality = V7ExecChaseRisk
		sig.RiskTags = append(sig.RiskTags, "event_chase_risk")
		sig.Confidence = "C"
	}
	sig.EntryZone = rangeExpansionEntryZone(ctx, dir)
	sig.Invalidation = rangeExpansionInvalidation(ctx, dir)
	sig.Targets = rangeExpansionTargets(ctx, sig)
	sig.RequiredConfirms = rangeExpansionRequiredConfirms(dir)
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)
	rangeExpansionApplySubtypeTags(ctx, sig)
	rangeExpansionApplyQualityDowngrade(ctx, sig)
	return sig
}

func rangeExpansionApplySubtypeTags(ctx *V7SymbolContext, sig *V7SignalOutput) {
	if ctx == nil || sig == nil {
		return
	}
	impulse := ctx.Change1h
	velocity15m := ctx.Velocity15m
	velocity5m := ctx.Velocity5m
	if sig.Direction == V7DirShort {
		impulse = -impulse
		velocity15m = -velocity15m
		velocity5m = -velocity5m
	}
	flowAligned := rangeExpansionFlowAligned(ctx, sig.Direction)
	decelerating := velocity15m >= 0.8 && velocity5m <= -0.15
	lateChase := sig.ExecutionQuality == V7ExecChaseRisk ||
		math.Abs(ctx.Velocity5m) > 5 ||
		(ctx.Amplitude24h >= 45 && impulse >= 6 && velocity5m < velocity15m*0.25)
	retest := rangeExpansionRetestConfirmed(ctx, sig.Direction) && flowAligned && !lateChase
	exhaustion := ctx.Amplitude24h >= 45 && (decelerating || !flowAligned)
	continuation := impulse >= 2 && velocity15m >= 1.2 && flowAligned && !lateChase && !exhaustion

	switch {
	case lateChase:
		sig.RiskTags = appendV7Unique(sig.RiskTags, "late_event_chase")
		sig.RiskTags = appendV7Unique(sig.RiskTags, "do_not_market_chase")
		sig.ReasonCodes = appendV7Unique(sig.ReasonCodes, "range_expansion_late_chase")
		sig.ExecutionQuality = V7ExecChaseRisk
		sig.Confidence = "C"
	case retest:
		sig.ReasonCodes = appendV7Unique(sig.ReasonCodes, "range_expansion_retest")
		sig.ReasonCodes = appendV7Unique(sig.ReasonCodes, "retest_confirmed")
	case exhaustion:
		sig.RiskTags = appendV7Unique(sig.RiskTags, "range_expansion_exhaustion")
		sig.ReasonCodes = appendV7Unique(sig.ReasonCodes, "velocity_decelerating")
		sig.RequiredConfirms = appendV7Unique(sig.RequiredConfirms, "fresh_micro_confirmed")
		if sig.ExecutionQuality == "" || sig.ExecutionQuality == V7ExecReady {
			sig.ExecutionQuality = V7ExecNearConfirm
		}
	case continuation:
		sig.ReasonCodes = appendV7Unique(sig.ReasonCodes, "range_expansion_continuation")
	default:
		sig.ReasonCodes = appendV7Unique(sig.ReasonCodes, "range_expansion_needs_retest")
		if sig.ExecutionQuality == "" || sig.ExecutionQuality == V7ExecReady {
			sig.ExecutionQuality = V7ExecNearConfirm
		}
	}
	if decelerating {
		sig.RiskTags = appendV7Unique(sig.RiskTags, "velocity_decelerating")
		sig.RiskTags = appendV7Unique(sig.RiskTags, "micro_reversal_against_signal")
	}
}

func rangeExpansionRetestConfirmed(ctx *V7SymbolContext, dir V7Direction) bool {
	if ctx == nil || ctx.CurrentPrice <= 0 || ctx.VWAP15m <= 0 {
		return false
	}
	distPct := math.Abs(ctx.CurrentPrice-ctx.VWAP15m) / ctx.CurrentPrice * 100
	atrPct := 0.8
	if ctx.ATR15m > 0 {
		atrPct = math.Max(0.45, math.Min(1.2, ctx.ATR15m/ctx.CurrentPrice*100*0.75))
	}
	if distPct > atrPct {
		return false
	}
	if dir == V7DirLong {
		return ctx.CurrentPrice >= ctx.VWAP15m*0.998
	}
	return ctx.CurrentPrice <= ctx.VWAP15m*1.002
}

func rangeExpansionApplyQualityDowngrade(ctx *V7SymbolContext, sig *V7SignalOutput) {
	if ctx == nil || sig == nil {
		return
	}
	shortCoveringRisk := false
	lowVolumeRisk := false
	if sig.Direction == V7DirLong && ctx.Snapshot != nil && ctx.Snapshot.OIDelta1h < 0 && ctx.Snapshot.OIDelta4h < 0 {
		shortCoveringRisk = true
		sig.RiskTags = appendV7Unique(sig.RiskTags, "short_covering_not_new_long_build")
		sig.RequiredConfirms = appendV7Unique(sig.RequiredConfirms, "oi_delta_1h_positive_or_quote_volume_expands")
	}
	if rangeExpansionLowVolumeFollowthrough(ctx) {
		lowVolumeRisk = true
		sig.RiskTags = appendV7Unique(sig.RiskTags, "range_expansion_low_volume_followthrough")
		if sig.Direction == V7DirLong {
			sig.RequiredConfirms = appendV7Unique(sig.RequiredConfirms, "taker_buy_15m_gt_0_52")
		} else {
			sig.RequiredConfirms = appendV7Unique(sig.RequiredConfirms, "taker_buy_15m_lt_0_48")
		}
	}
	if !shortCoveringRisk && !lowVolumeRisk {
		return
	}
	sig.TimingScore = clampFloat(sig.TimingScore-10, 25, 88)
	if sig.Confidence == "B" {
		sig.Confidence = "C"
	}
	if shortCoveringRisk && lowVolumeRisk {
		sig.ExecutionQuality = V7ExecChaseRisk
		sig.RiskTags = appendV7Unique(sig.RiskTags, "event_chase_risk")
		sig.ReasonCodes = appendV7Unique(sig.ReasonCodes, "event_followthrough_quality_insufficient")
		return
	}
	if sig.ExecutionQuality == "" || sig.ExecutionQuality == V7ExecReady {
		sig.ExecutionQuality = V7ExecNearConfirm
	}
}

func rangeExpansionLowVolumeFollowthrough(ctx *V7SymbolContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.VolumeBurst15m > 0 && ctx.VolumeBurst15m < 0.8 {
		return true
	}
	if ctx.ExecutionContext == nil {
		return false
	}
	if tf, ok := ctx.ExecutionContext.Timeframes["15m"]; ok && tf.VolumeVsAvg5 > 0 && tf.VolumeVsAvg5 < 0.8 {
		return true
	}
	return false
}

func appendV7Unique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func rangeExpansionEventDirection(ctx *V7SymbolContext) V7Direction {
	bull := 0.0
	bear := 0.0
	if ctx.Change1h > 0 {
		bull += ctx.Change1h
	} else {
		bear += -ctx.Change1h
	}
	if ctx.Velocity15m > 0 {
		bull += ctx.Velocity15m * 1.5
	} else {
		bear += -ctx.Velocity15m * 1.5
	}
	if ctx.TakerBuy15m >= 0.52 {
		bull += (ctx.TakerBuy15m - 0.50) * 100
	} else if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.48 {
		bear += (0.50 - ctx.TakerBuy15m) * 100
	}
	if ctx.VWAP15m > 0 {
		if ctx.CurrentPrice >= ctx.VWAP15m {
			bull += 2
		} else {
			bear += 2
		}
	}
	if bear > bull {
		return V7DirShort
	}
	return V7DirLong
}

func rangeExpansionFlowAligned(ctx *V7SymbolContext, dir V7Direction) bool {
	if ctx.TakerBuy15m <= 0 {
		return false
	}
	if dir == V7DirShort {
		return ctx.TakerBuy15m <= 0.48
	}
	return ctx.TakerBuy15m >= 0.52
}

func rangeExpansionTimingScore(ctx *V7SymbolContext, dir V7Direction) float64 {
	score := 45.0
	velocity := ctx.Velocity15m
	change := ctx.Change1h
	if dir == V7DirShort {
		velocity = -velocity
		change = -change
	}
	if velocity >= 1.2 {
		score += 18
	}
	if change >= 2.0 {
		score += 14
	}
	if ctx.VolumeBurst15m >= 1.5 || ctx.VolumeBurst5m >= 1.5 {
		score += 10
	}
	if ctx.Amplitude24h > 55 {
		score -= 10
	}
	if math.Abs(ctx.Velocity5m) > 5 {
		score -= 8
	}
	return clampFloat(score, 25, 88)
}

func rangeExpansionEntryZone(ctx *V7SymbolContext, dir V7Direction) V7PriceZone {
	atr := atrFallback(ctx)
	if atr <= 0 {
		atr = ctx.CurrentPrice * 0.015
	}
	if dir == V7DirShort {
		lower := ctx.CurrentPrice - atr*0.25
		upper := ctx.CurrentPrice + atr*0.65
		return V7PriceZone{Lower: lower, Upper: upper}
	}
	lower := ctx.CurrentPrice - atr*0.65
	upper := ctx.CurrentPrice + atr*0.25
	if ctx.VWAP15m > 0 && ctx.VWAP15m < ctx.CurrentPrice && ctx.VWAP15m > lower {
		lower = ctx.VWAP15m - atr*0.25
	}
	return V7PriceZone{Lower: lower, Upper: upper}
}

func rangeExpansionInvalidation(ctx *V7SymbolContext, dir V7Direction) V7InvalidationRule {
	atr := atrFallback(ctx)
	risk := math.Max(ctx.CurrentPrice*0.018, atr*1.25)
	if dir == V7DirShort {
		return V7InvalidationRule{Price: ctx.CurrentPrice + risk, Reason: "range_expansion_short_invalidated"}
	}
	return V7InvalidationRule{Price: ctx.CurrentPrice - risk, Reason: "range_expansion_long_invalidated"}
}

func rangeExpansionTargets(ctx *V7SymbolContext, sig *V7SignalOutput) []V7Target {
	entry := (sig.EntryZone.Lower + sig.EntryZone.Upper) / 2
	if entry <= 0 {
		entry = ctx.CurrentPrice
	}
	atr := atrFallback(ctx)
	risk := math.Abs(entry - sig.Invalidation.Price)
	if risk <= 0 {
		risk = math.Max(ctx.CurrentPrice*0.018, atr*1.25)
	}
	d1 := math.Max(atr*2.0, risk*1.6)
	d2 := math.Max(atr*3.2, risk*2.4)
	if sig.Direction == V7DirShort {
		return []V7Target{
			{Price: entry - d1, Reason: "range_expansion_event_target_1"},
			{Price: entry - d2, Reason: "range_expansion_event_target_2"},
		}
	}
	return []V7Target{
		{Price: entry + d1, Reason: "range_expansion_event_target_1"},
		{Price: entry + d2, Reason: "range_expansion_event_target_2"},
	}
}

func rangeExpansionRequiredConfirms(dir V7Direction) []string {
	if dir == V7DirShort {
		return []string{
			"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
			"taker_buy_15m_lt_0_48",
			"no_new_high_after_rejection",
		}
	}
	return []string{
		"15m_close_above_vwap_or_ema20_or_entry_zone_upper",
		"taker_buy_15m_gt_0_52",
		"no_new_low_after_reclaim",
	}
}
