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
	ok, _ := m.ExplainMatch(ctx, regime)
	return ok
}

// ExplainMatch is Match with the first failing condition's code (#163).
func (m *squeezeShortModule) ExplainMatch(ctx *V7SymbolContext, regime V7MarketRegime) (bool, string) {
	if ctx.CurrentPrice <= 0 {
		return false, "price_data_missing"
	}
	snap := ctx.Snapshot
	if snap == nil {
		return false, "snapshot_missing"
	}

	// Must have been positive recently (prior rally)
	if ctx.Change24h <= 0 {
		return false, "no_prior_rally"
	}

	// 1h drop < -3% — the selloff trigger
	if ctx.Change1h >= -3 {
		return false, "no_selloff_trigger"
	}

	// OI declining (1h < -3%) — longs getting liquidated
	if snap.OIDelta1h >= -3 {
		return false, "oi_not_flushing"
	}

	// Crowded long book
	if snap.LSR <= 2.0 {
		return false, "longs_not_crowded"
	}

	// === Forbid conditions — already too late ===

	// Already deep into flush: 24h drop >20% AND OI already flushed
	if ctx.Change24h > 20 && snap.OIDelta4h < -15 {
		return false, "squeeze_already_flushed" // Already crashed and OI flushed, squeeze is over
	}

	// TakerBuy recovering — selling exhaustion, bounce incoming
	if ctx.TakerBuy15m > 0.54 {
		return false, "taker_buy_recovering"
	}

	// Price at daily support — bad R:R for shorts
	if ctx.ATR1d > 0 && ctx.Low1d > 0 {
		distToDailyLow := (ctx.CurrentPrice - ctx.Low1d) / ctx.ATR1d
		if distToDailyLow < 0.5 {
			return false, "at_daily_support"
		}
	}

	return true, ""
}

func (m *squeezeShortModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	sig, _ := m.ExplainScore(ctx, regime)
	return sig
}

// ExplainScore is Score with the nil reason (#163).
func (m *squeezeShortModule) ExplainScore(ctx *V7SymbolContext, regime V7MarketRegime) (*V7SignalOutput, string) {
	if ok, code := m.ExplainMatch(ctx, regime); !ok {
		return nil, code
	}

	snap := ctx.Snapshot

	s := newV7Signal(ctx, regime, V7SetupLongSqueezeShort, V7DirShort, V7EntryFastConfirm, "B")

	// 1. Squeeze Strength (0-30): how violent the 1h drop + OI flush.
	squeezeMag := -ctx.Change1h // positive number for scoring
	if squeezeMag >= 6 {
		s.add(20, "sharp_1h_drop")
	} else if squeezeMag >= 4 {
		s.add(15, "sharp_1h_drop")
	} else {
		s.add(10, "moderate_1h_drop")
	}

	oiFlush := -snap.OIDelta1h // positive = OI declining
	if oiFlush >= 6 {
		s.add(10, "oi_heavy_flush")
	} else if oiFlush >= 3 {
		s.add(7, "oi_flushing")
	} else {
		s.add(3)
	}

	// 2. Crowding Level (0-25): how crowded the long book is.
	if snap.LSR >= 3.0 {
		s.add(25, "extreme_long_crowding")
	} else if snap.LSR >= 2.5 {
		s.add(20, "heavy_long_crowding")
	} else if snap.LSR >= 2.0 {
		s.add(12, "long_crowding")
	}

	// 3. Taker Selling (0-20): aggressive market sells.
	if ctx.TakerBuy15m < 0.40 {
		s.add(20, "heavy_taker_selling")
	} else if ctx.TakerBuy15m < 0.45 {
		s.add(15, "taker_selling")
	} else if ctx.TakerBuy15m < 0.48 {
		s.add(10, "mild_taker_selling")
	}

	// 4. Structure Break (0-15): price breaking below key levels.
	if ctx.ATR1h > 0 && ctx.EMA20_1h > 0 {
		if ctx.CurrentPrice < ctx.EMA20_1h {
			s.add(8, "below_ema20_1h")
		}
		if ctx.CurrentPrice < ctx.EMA60_1h {
			s.add(7, "below_ema60_1h")
		}
	}

	// 5. Cascade Potential (0-10): prior rally magnitude suggests more room to fall.
	if ctx.Change24h >= 15 {
		s.add(10, "high_cascade_potential")
	} else if ctx.Change24h >= 8 {
		s.add(7, "moderate_cascade_potential")
	} else if ctx.Change24h >= 3 {
		s.add(4, "low_cascade_potential")
	}

	// Short into any bounce within the selloff; invalidate above the 1h high;
	// targets ride the cascade into the gap below.
	s.zoneATR(0.3, 0.8)
	if ctx.High1h > 0 {
		s.invalidate(ctx.High1h+ctx.ATR15m*0.5, "break_1h_high_squeeze_failed")
	}
	if ctx.ATR4h > 0 {
		s.target(ctx.CurrentPrice-ctx.ATR4h*1.5, "squeeze_target_1")
		s.target(ctx.CurrentPrice-ctx.ATR4h*3.0, "squeeze_target_2")
	}
	if ctx.Low1d > 0 && ctx.Low1d < ctx.CurrentPrice {
		s.target(ctx.Low1d, "daily_low_target")
	}

	out := s.finish(30)
	if out == nil {
		return nil, v7CodeScoreBelowFloor
	}
	return out, ""
}
