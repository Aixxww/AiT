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

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirShort,
		SetupType:    V7SetupBreakdownShort,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryFastConfirm,
		Confidence:   "B",
		MarketRegime: regime,
	}

	score := 0.0
	switch {
	case ctx.Change1h <= -6:
		score += 25
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_1h_downside_momentum")
	case ctx.Change1h <= -3.5:
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "solid_1h_downside_momentum")
	case ctx.Change1h <= -2.5:
		score += 14
		sig.ReasonCodes = append(sig.ReasonCodes, "early_1h_downside_momentum")
	}
	if ctx.Velocity15m <= -1.5 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "fast_15m_downside_impulse")
	} else if ctx.Velocity15m <= -1.0 {
		score += 6
		sig.ReasonCodes = append(sig.ReasonCodes, "15m_downside_impulse")
	}

	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "below_vwap_breakdown")
	}
	if ctx.EMA20_1h > 0 && ctx.CurrentPrice < ctx.EMA20_1h {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "below_ema20_1h")
	}
	if ctx.BBMiddle15m > 0 && ctx.CurrentPrice < ctx.BBMiddle15m {
		score += 6
		sig.ReasonCodes = append(sig.ReasonCodes, "below_15m_boll_mid")
	}

	switch {
	case ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.42:
		score += 18
		sig.ReasonCodes = append(sig.ReasonCodes, "heavy_taker_selling")
	case ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.46:
		score += 14
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_selling")
	case ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.50:
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "mild_taker_selling")
	}

	if ctx.Snapshot != nil {
		if ctx.Snapshot.OIDelta1h > 0.5 {
			score += 12
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_confirms_new_shorts")
		} else if ctx.Snapshot.OIDelta1h < -2 {
			score += 8
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_flush_continuation")
		}
	}
	if ctx.VolumeBurst15m >= 1.8 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "sell_volume_expansion")
	} else if ctx.VolumeBurst15m >= 1.1 {
		score += 6
		sig.ReasonCodes = append(sig.ReasonCodes, "sell_volume_confirmed")
	}

	if ctx.Change4h <= -5 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "weak_4h_trend")
	}
	if ctx.Change24h < 0 {
		score += 4
		sig.ReasonCodes = append(sig.ReasonCodes, "daily_turning_negative")
	}

	sig.SetupScore = clampFloat(score, 0, 100)
	if sig.SetupScore < 45 {
		return nil
	}

	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

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
	sig.Invalidation = V7InvalidationRule{
		Price:  ctx.CurrentPrice + stopDist,
		Reason: "breakdown_momentum_reclaim_stop",
	}

	zonePad := ctx.CurrentPrice * 0.006
	if ctx.ATR15m > 0 {
		zonePad = maxFloat(zonePad, ctx.ATR15m*0.35)
	}
	sig.EntryZone = V7PriceZone{
		Lower: ctx.CurrentPrice - zonePad*0.45,
		Upper: ctx.CurrentPrice + zonePad,
	}

	targetDist := ctx.CurrentPrice * 0.035
	if ctx.ATR1h > 0 {
		targetDist = maxFloat(targetDist, ctx.ATR1h*1.15)
	}
	sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice - targetDist, Reason: "breakdown_momentum_target_1"})
	if ctx.ATR4h > 0 {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice - ctx.ATR4h*0.8, Reason: "breakdown_momentum_target_2"})
	}

	sig.TimingScore = calcBreakdownMomentumShortTimingScore(ctx)
	return sig
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
