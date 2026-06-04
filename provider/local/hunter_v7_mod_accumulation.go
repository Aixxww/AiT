package local

// ============================================================================
// Module F: Accumulation Breakout Long
// ============================================================================
// Catches compressed volatility regimes where smart money is quietly accumulating.
// This module looks for:
//   - Bollinger Band width at historic lows (volatility compression)
//   - OI quietly increasing (stealth position building)
//   - Price range-bound / stable (not yet breaking out)
//   - Neutral taker flow (no panic, no euphoria)
//   - Volume pattern showing quiet accumulation

type accumulationBreakoutLongModule struct{}

func (m *accumulationBreakoutLongModule) Name() string        { return "accumulation_breakout_long" }
func (m *accumulationBreakoutLongModule) SetupType() V7SetupType { return V7SetupAccumulationLong }
func (m *accumulationBreakoutLongModule) Direction() V7Direction  { return V7DirLong }

func (m *accumulationBreakoutLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	// Must have price data
	if ctx.CurrentPrice <= 0 {
		return false
	}
	// BBWidthPercentile < 25 (compressed volatility)
	if ctx.BBWidthPercentile >= 25 || ctx.BBWidthPercentile <= 0 {
		return false
	}
	// OI must be accumulating: 8% to 30% over 4h
	snap := ctx.Snapshot
	if snap == nil {
		return false
	}
	if snap.OIDelta4h < 8 || snap.OIDelta4h > 30 {
		return false
	}
	// Price must not have surged (accumulation, not breakout yet)
	if ctx.Change24h > 8 {
		return false
	}
	return true
}

func (m *accumulationBreakoutLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupAccumulationLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryWaitBreakout,
		Confidence:   "B",
		MarketRegime: regime,
	}

	snap := ctx.Snapshot
	var score float64

	// 1. Compression Quality (0-25): tighter BB = stronger setup
	bbPctl := ctx.BBWidthPercentile
	if bbPctl < 10 {
		score += 25
		sig.ReasonCodes = append(sig.ReasonCodes, "extreme_compression")
	} else if bbPctl < 18 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "deep_compression")
	} else if bbPctl < 25 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "mild_compression")
	}

	// 2. OI Accumulation (0-25): stealth position building
	oiD4h := snap.OIDelta4h
	if oiD4h >= 15 && oiD4h <= 30 {
		score += 25
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_oi_accumulation")
	} else if oiD4h >= 10 && oiD4h < 15 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "steady_oi_accumulation")
	} else if oiD4h >= 8 && oiD4h < 10 {
		score += 14
		sig.ReasonCodes = append(sig.ReasonCodes, "mild_oi_accumulation")
	}
	// OI consistency: 1h and 4h both positive
	if snap.OIDelta1h > 0 && oiD4h > 0 {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_consistent_inflow")
	}

	// 3. Price Stability (0-20): tight range, no wild swings
	if ctx.ATR4h > 0 && ctx.High4h > 0 && ctx.Low4h > 0 {
		range4h := (ctx.High4h - ctx.Low4h) / ctx.CurrentPrice * 100
		if range4h < 2.0 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "ultra_tight_range")
		} else if range4h < 4.0 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "tight_range")
		} else if range4h < 6.0 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "moderate_range")
		}
	}
	// Price near BB middle (balanced, not trending)
	if ctx.BBMiddle15m > 0 && ctx.ATR15m > 0 {
		distToMiddle := (ctx.CurrentPrice - ctx.BBMiddle15m) / ctx.ATR15m
		if distToMiddle > -0.5 && distToMiddle < 0.5 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "near_bb_middle")
		}
	}

	// 4. Taker Neutral (0-15): no aggressive selling, slight buy bias preferred
	if ctx.TakerBuy15m >= 0.50 && ctx.TakerBuy15m <= 0.58 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_neutral_bullish")
	} else if ctx.TakerBuy15m >= 0.48 && ctx.TakerBuy15m < 0.50 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_neutral")
	} else if ctx.TakerBuy15m > 0.58 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_biased")
	} else {
		// Heavy selling during accumulation is suspicious
		sig.RiskTags = append(sig.RiskTags, "taker_sell_during_accumulation")
	}

	// 5. Volume Pattern (0-15): quiet or declining volume = accumulation phase
	if snap != nil {
		// LSR range-bound (not extreme) suggests quiet accumulation
		if snap.LSR > 0.85 && snap.LSR < 1.15 {
			score += 8
			sig.ReasonCodes = append(sig.ReasonCodes, "lsr_neutral_accumulation")
		}
	}
	// Price change 1h small = quiet
	if ctx.Change1h > -1 && ctx.Change1h < 1 {
		score += 7
		sig.ReasonCodes = append(sig.ReasonCodes, "quiet_1h_price_action")
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	if sig.SetupScore < 30 {
		return nil
	}

	// Build price and derivatives context
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: wait for breakout above BB upper
	if ctx.BBUpper15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.BBUpper15m,
			Upper: ctx.BBUpper15m + ctx.ATR15m*0.5,
		}
	} else if ctx.ATR15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.CurrentPrice,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.5,
		}
	}

	// Invalidation: below BB lower or below compression range low
	if ctx.BBLower15m > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.BBLower15m - ctx.ATR15m*0.3,
			Reason: "break_below_bb_lower",
		}
	} else if ctx.Low4h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.Low4h - ctx.ATR4h*0.2,
			Reason: "break_compression_low",
		}
	}

	// Targets: breakout expansion targets
	if ctx.BBMiddle15m > 0 && ctx.BBUpper15m > 0 {
		bbRange := ctx.BBUpper15m - ctx.BBMiddle15m
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.BBUpper15m + bbRange, Reason: "bb_expansion_target"})
	}
	if ctx.ATR4h > 0 {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR4h*3, Reason: "atr_breakout_target"})
	}
	if ctx.High4h > ctx.CurrentPrice {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.High4h, Reason: "4h_high_breakout"})
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
}
