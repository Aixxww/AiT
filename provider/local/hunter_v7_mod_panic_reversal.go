package local

// ============================================================================
// Module E: Panic V-Reversal Long
// ============================================================================
// Catches capitulation-driven V-shaped reversals after sharp selloffs.
// This module looks for:
//   - 24h drop between -15% and -45% (deep capitulation)
//   - OI heavily flushed (leveraged longs wiped out, cleansing washout)
//   - Price showing early reclaim signals (bounce off the lows)
//   - Taker buy recovering (smart money stepping into the wreckage)
//   - Volume exhaustion (selling pressure subsiding)

type panicReversalLongModule struct{}

func (m *panicReversalLongModule) Name() string        { return "panic_reversal_long" }
func (m *panicReversalLongModule) SetupType() V7SetupType { return V7SetupPanicReversalLong }
func (m *panicReversalLongModule) Direction() V7Direction  { return V7DirLong }

func (m *panicReversalLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	// Must have price data
	if ctx.CurrentPrice <= 0 || ctx.ATR4h <= 0 {
		return false
	}
	// 24h drop between -15% and -45%
	if ctx.Change24h > -15 || ctx.Change24h < -45 {
		return false
	}
	// Forbid: 24h drop > 50% (extreme crash, too dangerous)
	if ctx.Change24h < -50 {
		return false
	}
	// OI must have dropped (leveraged positions flushed out)
	snap := ctx.Snapshot
	if snap != nil {
		if snap.OIDelta4h >= 0 {
			return false
		}
	}
	return true
}

func (m *panicReversalLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupPanicReversalLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryWaitReclaim,
		Confidence:   "B",
		MarketRegime: regime,
	}

	snap := ctx.Snapshot
	var score float64

	// 1. Capitulation Depth (0-20): deeper = more cleansed
	drop := ctx.Change24h // negative value
	if drop <= -30 && drop >= -45 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "deep_capitulation")
	} else if drop <= -22 && drop > -30 {
		score += 16
		sig.ReasonCodes = append(sig.ReasonCodes, "heavy_capitulation")
	} else if drop <= -15 && drop > -22 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_capitulation")
	}

	// 2. OI Flush (0-20): massive leverage cleanup
	if snap != nil {
		oiDrop := snap.OIDelta4h // should be negative
		if oiDrop <= -30 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_massive_flush")
		} else if oiDrop <= -20 {
			score += 16
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_heavy_flush")
		} else if oiDrop <= -12 {
			score += 12
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_flush")
		} else if oiDrop < 0 {
			score += 6
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_declining")
		}
		// Forbid: OI increasing while price drops (new shorts piling in, not cleansing)
		if oiDrop > 0 && ctx.Change24h < -15 {
			sig.RiskTags = append(sig.RiskTags, "oi_up_price_down")
			score -= 10
		}
	}

	// 3. Reclaim Strength (0-25): price bouncing from the lows
	if ctx.ATR4h > 0 && ctx.Low4h > 0 {
		distFromLow := (ctx.CurrentPrice - ctx.Low4h) / ctx.ATR4h
		if distFromLow >= 1.5 {
			score += 25
			sig.ReasonCodes = append(sig.ReasonCodes, "strong_reclaim")
		} else if distFromLow >= 1.0 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "solid_reclaim")
		} else if distFromLow >= 0.5 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "early_reclaim")
		} else if distFromLow > 0.2 {
			score += 8
			sig.ReasonCodes = append(sig.ReasonCodes, "weak_bounce")
		} else {
			// Forbid: price at the absolute low with no reclaim
			sig.RiskTags = append(sig.RiskTags, "no_reclaim_signal")
			score -= 5
		}
	}

	// 4. Taker Recovery (0-15): smart money stepping in
	if ctx.TakerBuy15m > 0.58 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_aggressive")
	} else if ctx.TakerBuy15m > 0.54 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_strong")
	} else if ctx.TakerBuy15m > 0.51 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_recovering")
	} else if ctx.TakerBuy15m > 0.48 {
		score += 3
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_neutral")
	}

	// 5. Volume Exhaustion (0-10): selling pressure subsiding
	if snap != nil {
		// If 1h OI delta is less negative than 4h, selling is decelerating
		if snap.OIDelta1h > snap.OIDelta4h/4 && snap.OIDelta4h < -10 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "selling_exhaustion")
		} else if snap.OIDelta1h > snap.OIDelta4h/3 {
			score += 6
			sig.ReasonCodes = append(sig.ReasonCodes, "selling_decelerating")
		}
	}

	// 6. Market Stabilization (0-10): broader market showing signs of life
	if ctx.Change1h > 0 && ctx.Change1h < 5 {
		score += 6
		sig.ReasonCodes = append(sig.ReasonCodes, "1h_green_shoot")
	}
	if ctx.RSI1h > 0 && ctx.RSI1h > 25 && ctx.RSI1h < 40 {
		score += 4
		sig.ReasonCodes = append(sig.ReasonCodes, "rsi_recovering_from_extreme")
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	if sig.SetupScore < 30 {
		return nil
	}

	// Build price and derivatives context
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: wait for reclaim of a key level, near current bounce
	if ctx.ATR15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.CurrentPrice - ctx.ATR15m*0.3,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.8,
		}
	}

	// Invalidation: below the absolute low
	if ctx.Low4h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.Low4h - ctx.ATR4h*0.2,
			Reason: "break_capitulation_low",
		}
	}

	// Targets: V-reversal targets — reversion to VWAP and prior breakdown level
	if ctx.VWAP15m > ctx.CurrentPrice {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.VWAP15m, Reason: "vwap_reclaim"})
	}
	if ctx.ATR4h > 0 {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR4h*2.5, Reason: "atr_v_reversal_target"})
	}
	// If EMA20 above current price, target it
	if ctx.EMA20_4h > ctx.CurrentPrice {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.EMA20_4h, Reason: "ema20_reclaim"})
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
}
