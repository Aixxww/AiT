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

func (m *accumulationBreakoutLongModule) Name() string           { return "accumulation_breakout_long" }
func (m *accumulationBreakoutLongModule) SetupType() V7SetupType { return V7SetupAccumulationLong }
func (m *accumulationBreakoutLongModule) Direction() V7Direction { return V7DirLong }

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

	snap := ctx.Snapshot

	s := newV7Signal(ctx, regime, V7SetupAccumulationLong, V7DirLong, V7EntryWaitBreakout, "B")

	// 1. Compression Quality (0-25): tighter BB = stronger setup.
	bbPctl := ctx.BBWidthPercentile
	if bbPctl < 10 {
		s.add(25, "extreme_compression")
	} else if bbPctl < 18 {
		s.add(20, "deep_compression")
	} else if bbPctl < 25 {
		s.add(15, "mild_compression")
	}

	// 2. OI Accumulation (0-25): stealth position building.
	oiD4h := snap.OIDelta4h
	if oiD4h >= 15 && oiD4h <= 30 {
		s.add(25, "strong_oi_accumulation")
	} else if oiD4h >= 10 && oiD4h < 15 {
		s.add(20, "steady_oi_accumulation")
	} else if oiD4h >= 8 && oiD4h < 10 {
		s.add(14, "mild_oi_accumulation")
	}
	// OI consistency: 1h and 4h both positive
	if snap.OIDelta1h > 0 && oiD4h > 0 {
		s.add(5, "oi_consistent_inflow")
	}

	// 3. Price Stability (0-20): tight range, no wild swings.
	if ctx.ATR4h > 0 && ctx.High4h > 0 && ctx.Low4h > 0 {
		range4h := (ctx.High4h - ctx.Low4h) / ctx.CurrentPrice * 100
		if range4h < 2.0 {
			s.add(20, "ultra_tight_range")
		} else if range4h < 4.0 {
			s.add(15, "tight_range")
		} else if range4h < 6.0 {
			s.add(10, "moderate_range")
		}
	}
	// Price near BB middle (balanced, not trending)
	if ctx.BBMiddle15m > 0 && ctx.ATR15m > 0 {
		distToMiddle := (ctx.CurrentPrice - ctx.BBMiddle15m) / ctx.ATR15m
		if distToMiddle > -0.5 && distToMiddle < 0.5 {
			s.add(5, "near_bb_middle")
		}
	}

	// 4. Taker Neutral (0-15): no aggressive selling, slight buy bias preferred.
	if ctx.TakerBuy15m >= 0.50 && ctx.TakerBuy15m <= 0.58 {
		s.add(15, "taker_neutral_bullish")
	} else if ctx.TakerBuy15m >= 0.48 && ctx.TakerBuy15m < 0.50 {
		s.add(10, "taker_neutral")
	} else if ctx.TakerBuy15m > 0.58 {
		s.add(8, "taker_buy_biased")
	} else {
		// Heavy selling during accumulation is suspicious
		s.riskTag("taker_sell_during_accumulation")
	}

	// 5. Volume Pattern (0-15): quiet or declining volume = accumulation phase.
	if snap != nil {
		// LSR range-bound (not extreme) suggests quiet accumulation
		if snap.LSR > 0.85 && snap.LSR < 1.15 {
			s.add(8, "lsr_neutral_accumulation")
		}
	}
	// Price change 1h small = quiet
	if ctx.Change1h > -1 && ctx.Change1h < 1 {
		s.add(7, "quiet_1h_price_action")
	}

	// Entry above the breakout trigger; invalidate below the compression
	// floor; targets are the expansion projections.
	if ctx.BBUpper15m > 0 {
		s.sig.EntryZone = V7PriceZone{
			Lower: ctx.BBUpper15m,
			Upper: ctx.BBUpper15m + ctx.ATR15m*0.5,
		}
	} else if ctx.ATR15m > 0 {
		s.sig.EntryZone = V7PriceZone{
			Lower: ctx.CurrentPrice,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.5,
		}
	}
	if ctx.BBLower15m > 0 {
		s.invalidate(ctx.BBLower15m-ctx.ATR15m*0.3, "break_below_bb_lower")
	} else if ctx.Low4h > 0 {
		s.invalidate(ctx.Low4h-ctx.ATR4h*0.2, "break_compression_low")
	}
	if ctx.BBMiddle15m > 0 && ctx.BBUpper15m > 0 {
		bbRange := ctx.BBUpper15m - ctx.BBMiddle15m
		s.target(ctx.BBUpper15m+bbRange, "bb_expansion_target")
	}
	if ctx.ATR4h > 0 {
		s.target(ctx.CurrentPrice+ctx.ATR4h*3, "atr_breakout_target")
	}
	if ctx.High4h > ctx.CurrentPrice {
		s.target(ctx.High4h, "4h_high_breakout")
	}

	out := s.finish(30)
	if out != nil {
		ApplyV7OIAccumulationEvidence(out, ctx)
	}
	return out
}
