package local

// ============================================================================
// Module K: Breakdown Momentum Short
// ============================================================================
// Captures downside continuation after a symbol starts trending lower. Unlike
// funding_reversal or long_squeeze_short, this module does not require extreme
// funding, LSR, or long-crowding. It focuses on tradeable downside momentum:
//   - 1h / 15m downside impulse
//   - Price below VWAP or 1h EMA20 / 15m middle band
//   - Taker sell flow
//   - OI or volume confirmation
//   - Avoid entries after an already exhausted crash near daily support.

type breakdownMomentumShortModule struct{}

func (m *breakdownMomentumShortModule) Name() string           { return "breakdown_momentum_short" }
func (m *breakdownMomentumShortModule) SetupType() V7SetupType { return V7SetupBreakdownShort }
func (m *breakdownMomentumShortModule) Direction() V7Direction { return V7DirShort }

func (m *breakdownMomentumShortModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil || ctx.CurrentPrice <= 0 {
		return false
	}
	if ctx.ATR1h <= 0 && ctx.ATR15m <= 0 {
		return false
	}

	downImpulse := ctx.Change1h <= -2.5 || ctx.Velocity15m <= -1.0 || (ctx.Change4h <= -4 && ctx.CurrentPrice < ctx.VWAP15m)
	if !downImpulse {
		return false
	}

	structureBreak := false
	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		structureBreak = true
	}
	if ctx.EMA20_1h > 0 && ctx.CurrentPrice < ctx.EMA20_1h {
		structureBreak = true
	}
	if ctx.BBMiddle15m > 0 && ctx.CurrentPrice < ctx.BBMiddle15m {
		structureBreak = true
	}
	if !structureBreak {
		return false
	}

	if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m > 0.50 {
		return false
	}

	confirm := ctx.VolumeBurst15m >= 1.1
	if ctx.Snapshot != nil {
		confirm = confirm || ctx.Snapshot.OIDelta1h > 0.5 || ctx.Snapshot.OIDelta1h < -2.0
	}
	if !confirm {
		return false
	}

	if ctx.Change1h <= -12 {
		return false
	}
	if ctx.Change24h <= -25 && ctx.Snapshot != nil && ctx.Snapshot.OIDelta4h < -12 {
		return false
	}
	if ctx.ATR1d > 0 && ctx.Low1d > 0 {
		distToDailyLow := (ctx.CurrentPrice - ctx.Low1d) / ctx.ATR1d
		if distToDailyLow >= 0 && distToDailyLow < 0.35 {
			return false
		}
	}
	return true
}

func (m *breakdownMomentumShortModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	s := newV7Signal(ctx, regime, V7SetupBreakdownShort, V7DirShort, V7EntryFastConfirm, "B")

	switch {
	case ctx.Change1h <= -6:
		s.add(25, "strong_1h_downside_momentum")
	case ctx.Change1h <= -3.5:
		s.add(20, "solid_1h_downside_momentum")
	case ctx.Change1h <= -2.5:
		s.add(14, "early_1h_downside_momentum")
	}
	if ctx.Velocity15m <= -1.5 {
		s.add(10, "fast_15m_downside_impulse")
	} else if ctx.Velocity15m <= -1.0 {
		s.add(6, "15m_downside_impulse")
	}

	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		s.add(12, "below_vwap_breakdown")
	}
	if ctx.EMA20_1h > 0 && ctx.CurrentPrice < ctx.EMA20_1h {
		s.add(10, "below_ema20_1h")
	}
	if ctx.BBMiddle15m > 0 && ctx.CurrentPrice < ctx.BBMiddle15m {
		s.add(6, "below_15m_boll_mid")
	}

	switch {
	case ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.42:
		s.add(18, "heavy_taker_selling")
	case ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.46:
		s.add(14, "taker_selling")
	case ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.50:
		s.add(8, "mild_taker_selling")
	}

	if ctx.Snapshot != nil {
		if ctx.Snapshot.OIDelta1h > 0.5 {
			s.add(12, "oi_confirms_new_shorts")
		} else if ctx.Snapshot.OIDelta1h < -2 {
			s.add(8, "oi_flush_continuation")
		}
	}
	if ctx.VolumeBurst15m >= 1.8 {
		s.add(10, "sell_volume_expansion")
	} else if ctx.VolumeBurst15m >= 1.1 {
		s.add(6, "sell_volume_confirmed")
	}

	if ctx.Change4h <= -5 {
		s.add(8, "weak_4h_trend")
	}
	if ctx.Change24h < 0 {
		s.add(4, "daily_turning_negative")
	}

	// Stop above the reclaim level (structure-aware), asymmetric entry pad
	// below/above price, percent-or-ATR floored targets — module-specific
	// geometry kept verbatim.
	stopDist := ctx.CurrentPrice * 0.022
	if ctx.ATR15m > 0 {
		stopDist = maxFloat(stopDist, ctx.ATR15m*0.8)
	}
	if ctx.High1h > ctx.CurrentPrice {
		structureStop := ctx.High1h
		if ctx.ATR15m > 0 {
			structureStop += ctx.ATR15m * 0.15
		}
		if structureStop-ctx.CurrentPrice > 0 && structureStop-ctx.CurrentPrice < stopDist*1.6 {
			stopDist = structureStop - ctx.CurrentPrice
		}
	}
	s.sig.Invalidation = V7InvalidationRule{
		Price:  ctx.CurrentPrice + stopDist,
		Reason: "breakdown_momentum_reclaim_stop",
	}

	zonePad := ctx.CurrentPrice * 0.006
	if ctx.ATR15m > 0 {
		zonePad = maxFloat(zonePad, ctx.ATR15m*0.35)
	}
	s.sig.EntryZone = V7PriceZone{
		Lower: ctx.CurrentPrice - zonePad*0.45,
		Upper: ctx.CurrentPrice + zonePad,
	}

	targetDist := ctx.CurrentPrice * 0.035
	if ctx.ATR1h > 0 {
		targetDist = maxFloat(targetDist, ctx.ATR1h*1.15)
	}
	s.sig.Targets = append(s.sig.Targets, V7Target{Price: ctx.CurrentPrice - targetDist, Reason: "breakdown_momentum_target_1"})
	if ctx.ATR4h > 0 {
		s.sig.Targets = append(s.sig.Targets, V7Target{Price: ctx.CurrentPrice - ctx.ATR4h*0.8, Reason: "breakdown_momentum_target_2"})
	}

	return s.finishWithTiming(45, calcBreakdownMomentumShortTimingScore(ctx))
}

func calcBreakdownMomentumShortTimingScore(ctx *V7SymbolContext) float64 {
	if ctx == nil {
		return 0
	}
	timing := 0.0
	if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.46 {
		timing += 22
	} else if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.50 {
		timing += 14
	}
	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		timing += 16
	}
	if ctx.EMA20_1h > 0 && ctx.CurrentPrice < ctx.EMA20_1h {
		timing += 14
	}
	if ctx.VolumeBurst15m >= 1.1 {
		timing += 12
	}
	if ctx.Snapshot != nil && (ctx.Snapshot.OIDelta1h > 0.5 || ctx.Snapshot.OIDelta1h < -2) {
		timing += 12
	}
	if ctx.Change1h <= -2.5 && ctx.Change1h > -9 {
		timing += 14
	}
	if ctx.Velocity15m <= -1.0 {
		timing += 8
	}
	return clampFloat(timing, 0, 100)
}
