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
	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    dir,
		SetupType:    V7SetupRangeExpansion,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryFastConfirm,
		Confidence:   "B",
		MarketRegime: regime,
		ReasonCodes:  []string{"range_expansion_event"},
	}

	score := 20.0
	switch {
	case ctx.Amplitude24h >= 45:
		score += 24
		sig.ReasonCodes = append(sig.ReasonCodes, "amplitude_24h_extreme")
	case ctx.Amplitude24h >= 30:
		score += 18
		sig.ReasonCodes = append(sig.ReasonCodes, "amplitude_24h_major")
	default:
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "amplitude_24h_event")
	}
	switch {
	case ctx.RangeExpansion1h >= 3.0:
		score += 22
		sig.ReasonCodes = append(sig.ReasonCodes, "massive_range_expansion_event")
	case ctx.RangeExpansion1h >= 2.2:
		score += 16
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_range_expansion_event")
	default:
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_range_expansion_event")
	}
	impulse := ctx.Change1h
	velocity := ctx.Velocity15m
	if dir == V7DirShort {
		impulse = -impulse
		velocity = -velocity
	}
	if impulse >= 4 || velocity >= 2 {
		score += 16
		if dir == V7DirShort {
			sig.ReasonCodes = append(sig.ReasonCodes, "event_breakdown_short")
		} else {
			sig.ReasonCodes = append(sig.ReasonCodes, "event_continuation_long")
		}
	} else if impulse >= 1 || velocity >= 0.8 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "event_directional_followthrough")
	}
	if ctx.VolumeBurst15m >= 2.5 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "volume_burst_15m")
	} else if ctx.VolumeBurst5m >= 2.0 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "volume_burst_5m")
	}
	if rangeExpansionFlowAligned(ctx, dir) {
		score += 12
		if dir == V7DirShort {
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_sell_aligned")
		} else {
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_aligned")
		}
	} else {
		sig.RiskTags = append(sig.RiskTags, "event_flow_confirmation_needed")
		sig.ExecutionQuality = V7ExecNearConfirm
		sig.Confidence = "C"
	}

	sig.SetupScore = clampFloat(score, 0, 100)
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
	return sig
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
