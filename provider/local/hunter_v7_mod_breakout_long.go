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

func (m *trendBreakoutLongModule) Name() string        { return "trend_breakout_long" }
func (m *trendBreakoutLongModule) SetupType() V7SetupType { return V7SetupTrendBreakoutLong }
func (m *trendBreakoutLongModule) Direction() V7Direction  { return V7DirLong }

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

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupTrendBreakoutLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryBreakout,
		Confidence:   "B",
		MarketRegime: regime,
	}

	snap := ctx.Snapshot
	var score float64

	// 1. Compression Score (0-20): lower percentile = tighter squeeze = better setup
	if ctx.BBWidthPercentile < 10 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "extreme_compression")
	} else if ctx.BBWidthPercentile < 15 {
		score += 16
		sig.ReasonCodes = append(sig.ReasonCodes, "tight_compression")
	} else if ctx.BBWidthPercentile < 20 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_compression")
	} else {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "mild_compression")
	}

	// 2. Breakout Strength (0-25): how far price has broken above upper band
	if ctx.BBUpper15m > 0 && ctx.BBMiddle15m > 0 {
		bandRange := ctx.BBUpper15m - ctx.BBMiddle15m
		if bandRange > 0 {
			penetration := (ctx.CurrentPrice - ctx.BBUpper15m) / bandRange
			if penetration > 0.3 {
				score += 25
				sig.ReasonCodes = append(sig.ReasonCodes, "strong_breakout")
			} else if penetration > 0.1 {
				score += 20
				sig.ReasonCodes = append(sig.ReasonCodes, "confirmed_breakout")
			} else if penetration > -0.05 {
				// Price right at the upper band
				score += 15
				sig.ReasonCodes = append(sig.ReasonCodes, "breakout_attempt")
			} else {
				score += 10
				sig.ReasonCodes = append(sig.ReasonCodes, "approaching_breakout")
			}
		}
	}
	// Also check if near recent highs
	if ctx.High4h > 0 && ctx.CurrentPrice >= ctx.High4h*0.995 {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "near_4h_high")
	}

	// 3. OI Confirmation (0-20): rising OI = new positions entering on breakout
	if snap != nil {
		if snap.OIDelta1h > 5 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_surge")
		} else if snap.OIDelta1h > 2 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_rising")
		} else if snap.OIDelta1h > 0 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_increasing")
		} else if snap.OIDelta1h > -3 {
			// Slight OI drop during breakout = short covering, still valid
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_stable_breakout")
		}
	}

	// 4. Taker Confirmation (0-15): buy-side aggression
	if ctx.TakerBuy15m > 0.60 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_aggressive_buy")
	} else if ctx.TakerBuy15m > 0.55 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_strong_buy")
	} else if ctx.TakerBuy15m > 0.52 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_moderate_buy")
	} else {
		score += 3
	}

	// 5. Volume Expansion (0-10): breakout needs volume confirmation
	if snap != nil && snap.QuoteVolume24h > 0 {
		if snap.QuoteVolume24h > 3e8 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "volume_expansion")
		} else if snap.QuoteVolume24h > 8e7 {
			score += 7
			sig.ReasonCodes = append(sig.ReasonCodes, "volume_decent")
		} else if snap.QuoteVolume24h > 2e7 {
			score += 4
			sig.ReasonCodes = append(sig.ReasonCodes, "volume_adequate")
		}
	}

	// 6. Resistance Clearance (0-10): no overhead resistance nearby
	//    If 1d high is well above current price, room to run
	if ctx.High1d > 0 && ctx.ATR1d > 0 {
		headroom := (ctx.High1d - ctx.CurrentPrice) / ctx.ATR1d
		if headroom > 2.0 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "clear_air_above")
		} else if headroom > 1.0 {
			score += 7
			sig.ReasonCodes = append(sig.ReasonCodes, "moderate_resistance_distance")
		} else if headroom > 0.3 {
			score += 4
			sig.ReasonCodes = append(sig.ReasonCodes, "some_resistance_overhead")
		}
		// Near 1d high = close to resistance, but could be retesting
		if ctx.CurrentPrice >= ctx.High1d*0.98 {
			score += 3
			sig.ReasonCodes = append(sig.ReasonCodes, "at_1d_high_breakout")
		}
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	if sig.SetupScore < 30 {
		return nil
	}

	// Build price context
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: breakout confirmation zone
	if ctx.ATR15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.BBUpper15m - ctx.ATR15m*0.2,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.5,
		}
	}

	// Invalidation: back below BB middle — breakout failed
	if ctx.BBMiddle15m > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.BBMiddle15m - ctx.ATR15m*0.3,
			Reason: "reentry_below_bb_middle",
		}
	}

	// Targets: measured move from the compression range + extension
	if ctx.BBUpper15m > 0 && ctx.BBLower15m > 0 && ctx.ATR4h > 0 {
		compressedRange := ctx.BBUpper15m - ctx.BBLower15m
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + compressedRange, Reason: "measured_move"})
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR4h*2, Reason: "atr_extension"})
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
}
