package local

// ============================================================================
// Module I: Range Reversion
// ============================================================================
// Trades mean-reversion within a well-defined price range.
// This is the "fade the range extremes" module — it looks for:
//   - Low ADX (<20) — no trend, ranging market
//   - Moderate BB Width — defined range (not squeezing, not exploding)
//   - OI with no strong trend — no directional leverage buildup
//   - Price near range edge with oscillator divergence
//
// Can output BOTH Long (near range bottom) and Short (near range top).
// Direction() defaults to V7DirLong; Score() overrides for short setups.

type rangeReversionModule struct{}

func (m *rangeReversionModule) Name() string        { return "range_reversion" }
func (m *rangeReversionModule) SetupType() V7SetupType { return V7SetupRangeReversion }
func (m *rangeReversionModule) Direction() V7Direction  { return V7DirLong }

func (m *rangeReversionModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx.CurrentPrice <= 0 {
		return false
	}

	// Low ADX — must be ranging, not trending
	if ctx.ADX1h >= 20 {
		return false
	}

	// BB Width moderate — defined range (not too tight, not too wide)
	if ctx.BBWidth15m <= 0 {
		return false
	}
	if ctx.BBWidthPercentile < 20 || ctx.BBWidthPercentile > 75 {
		return false
	}

	// OI should not have strong directional trend
	snap := ctx.Snapshot
	if snap != nil {
		if snap.OIDelta1h > 10 || snap.OIDelta1h < -10 {
			return false // OI exploding one way = directional flow
		}
	}

	return true
}

func (m *rangeReversionModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	snap := ctx.Snapshot

	// Determine long vs short candidacy
	isLongCandidate := false
	isShortCandidate := false

	// Long candidate: price near 1h low, RSI<35 recovering, TakerBuy recovering
	if ctx.Low1h > 0 && ctx.ATR1h > 0 {
		distToLow := (ctx.CurrentPrice - ctx.Low1h) / ctx.ATR1h
		if distToLow < 0.8 {
			isLongCandidate = true
		}
	}
	if isLongCandidate && ctx.RSI1h >= 35 {
		isLongCandidate = false
	}
	// TakerBuy recovering (need >0.50 or trending up)
	if isLongCandidate && ctx.TakerBuy15m < 0.48 {
		isLongCandidate = false // Still heavy selling, not a good long yet
	}

	// Short candidate: price near 1h high, RSI>65 falling, TakerSell strong
	if ctx.High1h > 0 && ctx.ATR1h > 0 {
		distToHigh := (ctx.High1h - ctx.CurrentPrice) / ctx.ATR1h
		if distToHigh < 0.8 {
			isShortCandidate = true
		}
	}
	if isShortCandidate && ctx.RSI1h <= 65 {
		isShortCandidate = false
	}
	// TakerSell strong (TakerBuy < 0.48)
	if isShortCandidate && ctx.TakerBuy15m > 0.52 {
		isShortCandidate = false // Still buying pressure, not a good short
	}

	if !isLongCandidate && !isShortCandidate {
		return nil
	}

	// Build output
	dir := V7DirLong
	if isShortCandidate && !isLongCandidate {
		dir = V7DirShort
	}
	// If both are true, prefer the one with better edge proximity
	if isLongCandidate && isShortCandidate {
		distToLow := (ctx.CurrentPrice - ctx.Low1h) / ctx.ATR1h
		distToHigh := (ctx.High1h - ctx.CurrentPrice) / ctx.ATR1h
		if distToHigh < distToLow {
			dir = V7DirShort
		}
	}

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    dir,
		SetupType:    V7SetupRangeReversion,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryRangeEdge,
		Confidence:   "B",
		MarketRegime: regime,
	}

	var score float64

	// 1. Range Quality (0-30): how well-defined the range is
	// Lower ADX = better range quality
	if ctx.ADX1h < 10 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_range")
	} else if ctx.ADX1h < 15 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "defined_range")
	} else {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "weak_range")
	}
	// BB width moderate bonus
	if ctx.BBWidthPercentile >= 30 && ctx.BBWidthPercentile <= 60 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "ideal_bb_width")
	} else {
		score += 5
	}

	// 2. Edge Proximity (0-30): how close to range boundary
	if ctx.ATR1h > 0 {
		if dir == V7DirLong {
			dist := (ctx.CurrentPrice - ctx.Low1h) / ctx.ATR1h
			if dist < 0.3 {
				score += 30
				sig.ReasonCodes = append(sig.ReasonCodes, "at_range_bottom")
			} else if dist < 0.5 {
				score += 22
				sig.ReasonCodes = append(sig.ReasonCodes, "near_range_bottom")
			} else if dist < 0.8 {
				score += 15
				sig.ReasonCodes = append(sig.ReasonCodes, "approaching_range_bottom")
			}
		} else {
			dist := (ctx.High1h - ctx.CurrentPrice) / ctx.ATR1h
			if dist < 0.3 {
				score += 30
				sig.ReasonCodes = append(sig.ReasonCodes, "at_range_top")
			} else if dist < 0.5 {
				score += 22
				sig.ReasonCodes = append(sig.ReasonCodes, "near_range_top")
			} else if dist < 0.8 {
				score += 15
				sig.ReasonCodes = append(sig.ReasonCodes, "approaching_range_top")
			}
		}
	}

	// 3. Oscillator Signal (0-20): RSI confirmation at the edge
	if dir == V7DirLong {
		if ctx.RSI1h < 25 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "rsi_deeply_oversold")
		} else if ctx.RSI1h < 30 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "rsi_oversold")
		} else if ctx.RSI1h < 35 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "rsi_approaching_oversold")
		}
	} else {
		if ctx.RSI1h > 75 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "rsi_deeply_overbought")
		} else if ctx.RSI1h > 70 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "rsi_overbought")
		} else if ctx.RSI1h > 65 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "rsi_approaching_overbought")
		}
	}

	// 4. Taker Confirm (0-10): directional taker flow at the edge
	if dir == V7DirLong {
		if ctx.TakerBuy15m > 0.53 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_recovering")
		} else if ctx.TakerBuy15m > 0.50 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_neutral")
		}
	} else {
		if ctx.TakerBuy15m < 0.45 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_sell_strong")
		} else if ctx.TakerBuy15m < 0.48 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_sell_mild")
		}
	}

	// 5. OI Neutrality (0-10): no directional leverage
	if snap != nil {
		oiAbs := snap.OIDelta1h
		if oiAbs < 0 {
			oiAbs = -oiAbs
		}
		if oiAbs < 3 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_neutral")
		} else if oiAbs < 6 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_mild")
		}
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	if sig.SetupScore < 30 {
		return nil
	}

	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: at the range edge
	if ctx.ATR15m > 0 {
		if dir == V7DirLong {
			sig.EntryZone = V7PriceZone{
				Lower: ctx.Low1h,
				Upper: ctx.Low1h + ctx.ATR15m*0.5,
			}
		} else {
			sig.EntryZone = V7PriceZone{
				Lower: ctx.High1h - ctx.ATR15m*0.5,
				Upper: ctx.High1h,
			}
		}
	}

	// Invalidation: beyond the range boundary
	if dir == V7DirLong && ctx.Low1h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.Low1h - ctx.ATR1h*0.5,
			Reason: "break_range_low",
		}
	} else if dir == V7DirShort && ctx.High1h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.High1h + ctx.ATR1h*0.5,
			Reason: "break_range_high",
		}
	}

	// Targets: range midpoint and opposite edge
	if dir == V7DirLong {
		if ctx.High1h > ctx.CurrentPrice {
			mid := (ctx.High1h + ctx.Low1h) / 2
			sig.Targets = append(sig.Targets, V7Target{Price: mid, Reason: "range_midpoint"})
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.High1h, Reason: "range_top"})
		}
	} else {
		if ctx.Low1h < ctx.CurrentPrice {
			mid := (ctx.High1h + ctx.Low1h) / 2
			sig.Targets = append(sig.Targets, V7Target{Price: mid, Reason: "range_midpoint"})
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.Low1h, Reason: "range_bottom"})
		}
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
}
