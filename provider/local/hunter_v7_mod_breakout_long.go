package local

// ============================================================================
// Module C: Trend Breakout Long
// ============================================================================
// Catches breakouts from compressed Bollinger Band ranges:
//   - BB width at low percentile (< 25) → volatility squeeze
//   - Price near or above BB upper band → breakout attempt
//   - Confirmed by OI, taker, and volume expansion
// This targets the early stages of new trends emerging from consolidation.

type trendBreakoutLongModule struct{}

func (m *trendBreakoutLongModule) Name() string           { return "trend_breakout_long" }
func (m *trendBreakoutLongModule) SetupType() V7SetupType { return V7SetupTrendBreakoutLong }
func (m *trendBreakoutLongModule) Direction() V7Direction { return V7DirLong }

func (m *trendBreakoutLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx.CurrentPrice <= 0 {
		return false
	}
	// BB width must be compressed — volatility squeeze in progress
	if ctx.BBWidthPercentile >= 25 {
		return false
	}
	// Price must be near or above upper BB band (breakout signal)
	if ctx.BBUpper15m <= 0 {
		return false
	}
	// Allow price within 1% below the upper band (approaching breakout)
	approachThreshold := ctx.BBUpper15m * 0.99
	if ctx.CurrentPrice < approachThreshold {
		return false
	}
	return true
}

func (m *trendBreakoutLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	snap := ctx.Snapshot

	s := newV7Signal(ctx, regime, V7SetupTrendBreakoutLong, V7DirLong, V7EntryBreakout, "B")

	// 1. Compression Score (0-20): lower percentile = tighter squeeze = better setup.
	if ctx.BBWidthPercentile < 10 {
		s.add(20, "extreme_compression")
	} else if ctx.BBWidthPercentile < 15 {
		s.add(16, "tight_compression")
	} else if ctx.BBWidthPercentile < 20 {
		s.add(12, "moderate_compression")
	} else {
		s.add(8, "mild_compression")
	}

	// 2. Breakout Strength (0-25): how far price has broken above upper band.
	if ctx.BBUpper15m > 0 && ctx.BBMiddle15m > 0 {
		bandRange := ctx.BBUpper15m - ctx.BBMiddle15m
		if bandRange > 0 {
			penetration := (ctx.CurrentPrice - ctx.BBUpper15m) / bandRange
			if penetration > 0.3 {
				s.add(25, "strong_breakout")
			} else if penetration > 0.1 {
				s.add(20, "confirmed_breakout")
			} else if penetration > -0.05 {
				// Price right at the upper band
				s.add(15, "breakout_attempt")
			} else {
				s.add(10, "approaching_breakout")
			}
		}
	}
	// Also check if near recent highs
	if ctx.High4h > 0 && ctx.CurrentPrice >= ctx.High4h*0.995 {
		s.add(5, "near_4h_high")
	}

	// 3. OI Confirmation (0-20): rising OI = new positions entering on breakout.
	if snap != nil {
		if snap.OIDelta1h > 5 {
			s.add(20, "oi_surge")
		} else if snap.OIDelta1h > 2 {
			s.add(15, "oi_rising")
		} else if snap.OIDelta1h > 0 {
			s.add(10, "oi_increasing")
		} else if snap.OIDelta1h > -3 {
			// Slight OI drop during breakout = short covering, still valid
			s.add(5, "oi_stable_breakout")
		}
	}

	// 4. Taker Confirmation (0-15): buy-side aggression.
	if ctx.TakerBuy15m > 0.60 {
		s.add(15, "taker_aggressive_buy")
	} else if ctx.TakerBuy15m > 0.55 {
		s.add(12, "taker_strong_buy")
	} else if ctx.TakerBuy15m > 0.52 {
		s.add(8, "taker_moderate_buy")
	} else {
		s.add(3)
	}

	// 5. Volume Expansion (0-10): breakout needs volume confirmation.
	if snap != nil && snap.QuoteVolume24h > 0 {
		if snap.QuoteVolume24h > 3e8 {
			s.add(10, "volume_expansion")
		} else if snap.QuoteVolume24h > 8e7 {
			s.add(7, "volume_decent")
		} else if snap.QuoteVolume24h > 2e7 {
			s.add(4, "volume_adequate")
		}
	}

	// 6. Resistance Clearance (0-10): no overhead resistance nearby.
	//    If 1d high is well above current price, room to run.
	if ctx.High1d > 0 && ctx.ATR1d > 0 {
		headroom := (ctx.High1d - ctx.CurrentPrice) / ctx.ATR1d
		if headroom > 2.0 {
			s.add(10, "clear_air_above")
		} else if headroom > 1.0 {
			s.add(7, "moderate_resistance_distance")
		} else if headroom > 0.3 {
			s.add(4, "some_resistance_overhead")
		}
		// Near 1d high = close to resistance, but could be retesting
		if ctx.CurrentPrice >= ctx.High1d*0.98 {
			s.add(3, "at_1d_high_breakout")
		}
	}

	// Entry in the breakout confirmation zone under the trigger; invalidate on
	// re-entry below the BB middle; targets are the measured move + extension.
	if ctx.ATR15m > 0 {
		trigger := ctx.BBUpper15m
		s.sig.EntryZone = V7PriceZone{
			Lower: trigger - ctx.ATR15m*0.2,
			Upper: trigger,
		}
	}
	if ctx.BBMiddle15m > 0 {
		s.invalidate(ctx.BBMiddle15m-ctx.ATR15m*0.3, "reentry_below_bb_middle")
	}
	if ctx.BBUpper15m > 0 && ctx.BBLower15m > 0 && ctx.ATR4h > 0 {
		compressedRange := ctx.BBUpper15m - ctx.BBLower15m
		s.target(ctx.CurrentPrice+compressedRange, "measured_move")
		s.target(ctx.CurrentPrice+ctx.ATR4h*2, "atr_extension")
	}

	return s.finish(30)
}
