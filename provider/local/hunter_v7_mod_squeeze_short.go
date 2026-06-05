package local

// ============================================================================
// Module H: Long Squeeze Short
// ============================================================================
// Catches over-leveraged longs getting liquidated after a prior rally.
// This is the "fade the long crowd" module — it looks for:
//   - Recent uptrend (change24h was positive — prior rally)
//   - Sharp 1h selloff (< -3%) — the trigger event
//   - OI flushing (OI 1h < -3%) — longs being liquidated
//   - Crowded long book (LSR > 2.0) — there's more to squeeze
//   - Taker selling accelerating — aggressive market sells
//
// Forbid when: already deep into flush (24h drop >20% + OI flushed),
//   TakerBuy recovering (selling exhaustion), or price at daily support.

type squeezeShortModule struct{}

func (m *squeezeShortModule) Name() string           { return "long_squeeze_short" }
func (m *squeezeShortModule) SetupType() V7SetupType { return V7SetupLongSqueezeShort }
func (m *squeezeShortModule) Direction() V7Direction { return V7DirShort }

func (m *squeezeShortModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx.CurrentPrice <= 0 {
		return false
	}
	snap := ctx.Snapshot
	if snap == nil {
		return false
	}

	// Must have been positive recently (prior rally)
	if ctx.Change24h <= 0 {
		return false
	}

	// 1h drop < -3% — the selloff trigger
	if ctx.Change1h >= -3 {
		return false
	}

	// OI declining (1h < -3%) — longs getting liquidated
	if snap.OIDelta1h >= -3 {
		return false
	}

	// Crowded long book
	if snap.LSR <= 2.0 {
		return false
	}

	// === Forbid conditions — already too late ===

	// Already deep into flush: 24h drop >20% AND OI already flushed
	if ctx.Change24h > 20 && snap.OIDelta4h < -15 {
		return false // Already crashed and OI flushed, squeeze is over
	}

	// TakerBuy recovering — selling exhaustion, bounce incoming
	if ctx.TakerBuy15m > 0.54 {
		return false
	}

	// Price at daily support — bad R:R for shorts
	if ctx.ATR1d > 0 && ctx.Low1d > 0 {
		distToDailyLow := (ctx.CurrentPrice - ctx.Low1d) / ctx.ATR1d
		if distToDailyLow < 0.5 {
			return false
		}
	}

	return true
}

func (m *squeezeShortModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirShort,
		SetupType:    V7SetupLongSqueezeShort,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryFastConfirm,
		Confidence:   "B",
		MarketRegime: regime,
	}

	snap := ctx.Snapshot
	var score float64

	// 1. Squeeze Strength (0-30): how violent the 1h drop + OI flush
	squeezeMag := -ctx.Change1h // positive number for scoring
	if squeezeMag >= 6 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "sharp_1h_drop")
	} else if squeezeMag >= 4 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "sharp_1h_drop")
	} else {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_1h_drop")
	}

	oiFlush := -snap.OIDelta1h // positive = OI declining
	if oiFlush >= 6 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_heavy_flush")
	} else if oiFlush >= 3 {
		score += 7
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_flushing")
	} else {
		score += 3
	}

	// 2. Crowding Level (0-25): how crowded the long book is
	if snap.LSR >= 3.0 {
		score += 25
		sig.ReasonCodes = append(sig.ReasonCodes, "extreme_long_crowding")
	} else if snap.LSR >= 2.5 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "heavy_long_crowding")
	} else if snap.LSR >= 2.0 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "long_crowding")
	}

	// 3. Taker Selling (0-20): aggressive market sells
	if ctx.TakerBuy15m < 0.40 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "heavy_taker_selling")
	} else if ctx.TakerBuy15m < 0.45 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_selling")
	} else if ctx.TakerBuy15m < 0.48 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "mild_taker_selling")
	}

	// 4. Structure Break (0-15): price breaking below key levels
	if ctx.ATR1h > 0 && ctx.EMA20_1h > 0 {
		if ctx.CurrentPrice < ctx.EMA20_1h {
			score += 8
			sig.ReasonCodes = append(sig.ReasonCodes, "below_ema20_1h")
		}
		if ctx.CurrentPrice < ctx.EMA60_1h {
			score += 7
			sig.ReasonCodes = append(sig.ReasonCodes, "below_ema60_1h")
		}
	}

	// 5. Cascade Potential (0-10): prior rally magnitude suggests more room to fall
	if ctx.Change24h >= 15 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "high_cascade_potential")
	} else if ctx.Change24h >= 8 {
		score += 7
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_cascade_potential")
	} else if ctx.Change24h >= 3 {
		score += 4
		sig.ReasonCodes = append(sig.ReasonCodes, "low_cascade_potential")
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	if sig.SetupScore < 30 {
		return nil
	}

	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: short into any bounce within the selloff
	if ctx.ATR15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.CurrentPrice - ctx.ATR15m*0.3,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.8,
		}
	}

	// Invalidation: above prior swing high or 4h high
	if ctx.High1h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.High1h + ctx.ATR15m*0.5,
			Reason: "break_1h_high_squeeze_failed",
		}
	}

	// Targets: downside into the gap
	if ctx.ATR4h > 0 {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice - ctx.ATR4h*1.5, Reason: "squeeze_target_1"})
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice - ctx.ATR4h*3.0, Reason: "squeeze_target_2"})
	}
	// Daily low as a target if reasonable
	if ctx.Low1d > 0 && ctx.Low1d < ctx.CurrentPrice {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.Low1d, Reason: "daily_low_target"})
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
}
