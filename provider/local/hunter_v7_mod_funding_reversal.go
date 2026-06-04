package local

// ============================================================================
// Module J: Funding / Crowding Reversal
// ============================================================================
// Trades reversals driven by extreme funding rates and crowding imbalances.
// This is the "fade the crowded trade" module — it looks for:
//   - |FundingRate| > 0.0003 — expensive to hold the crowded side
//   - LSR extreme (>2.2 long-crowded or <0.8 short-crowded)
//   - Taker flow turning — exhaustion of the prevailing side
//   - Price stalling or holding support — reversal evidence
//
// For Short candidate: Funding>0.05%, LSR>2.2, TakerBuy declining, price stalling
// For Long candidate:  Funding<-0.03%, LSR<0.8, TakerBuy recovering, price holding support

type fundingReversalModule struct{}

func (m *fundingReversalModule) Name() string           { return "funding_reversal" }
func (m *fundingReversalModule) SetupType() V7SetupType { return V7SetupFundingReversal }
func (m *fundingReversalModule) Direction() V7Direction { return V7DirShort } // default, Score() can override

func (m *fundingReversalModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx.CurrentPrice <= 0 {
		return false
	}
	snap := ctx.Snapshot
	if snap == nil {
		return false
	}

	// Must have extreme funding OR extreme LSR
	hasFunding := snap.FundingRate > 0.0003 || snap.FundingRate < -0.0003
	hasLSR := snap.LSR > 2.2 || snap.LSR < 0.8

	if !hasFunding && !hasLSR {
		return false
	}

	return true
}

func (m *fundingReversalModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	snap := ctx.Snapshot

	// Determine short vs long candidate
	isShortCandidate := false
	isLongCandidate := false

	// Short: positive funding (longs paying), crowded longs
	if snap.FundingRate > 0.0005 || snap.LSR > 2.2 {
		isShortCandidate = true
	}

	// Long: negative funding (shorts paying), crowded shorts
	if snap.FundingRate < -0.0003 || snap.LSR < 0.8 {
		isLongCandidate = true
	}

	if !isShortCandidate && !isLongCandidate {
		return nil
	}

	// Short candidate refinements
	if isShortCandidate {
		// TakerBuy should be declining (below 0.50)
		if ctx.TakerBuy15m > 0.52 {
			isShortCandidate = false
		}
	}

	// Long candidate refinements
	if isLongCandidate {
		// TakerBuy should be recovering (above 0.48)
		if ctx.TakerBuy15m < 0.46 {
			isLongCandidate = false
		}
		// Price should be holding support
		if ctx.ATR1d > 0 && ctx.Low1d > 0 {
			distToLow := (ctx.CurrentPrice - ctx.Low1d) / ctx.ATR1d
			if distToLow > 2.0 {
				isLongCandidate = false // Price too far from support
			}
		}
	}

	if !isShortCandidate && !isLongCandidate {
		return nil
	}

	// Determine direction
	dir := V7DirShort
	if isLongCandidate && !isShortCandidate {
		dir = V7DirLong
	}
	// If both, prefer direction aligned with negative funding (shorts pay longs = bullish)
	if isLongCandidate && isShortCandidate {
		if snap.FundingRate < 0 {
			dir = V7DirLong
		}
	}

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    dir,
		SetupType:    V7SetupFundingReversal,
		Status:       V7StatusWaitConfirm,
		EntryMode:    V7EntryWaitPriceReversal,
		Confidence:   "C",
		MarketRegime: regime,
	}
	sig.RequiredConfirms = fundingReversalConfirmations(dir)

	var score float64

	// 1. Funding Extreme (0-25): how extreme the funding rate is
	fundingAbs := snap.FundingRate
	if fundingAbs < 0 {
		fundingAbs = -fundingAbs
	}
	if fundingAbs >= 0.0015 {
		score += 25
		sig.ReasonCodes = append(sig.ReasonCodes, "extreme_funding")
	} else if fundingAbs >= 0.0010 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "very_high_funding")
	} else if fundingAbs >= 0.0005 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "high_funding")
	} else {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "elevated_funding")
	}

	// 2. Crowding (0-25): how extreme the LSR is
	if dir == V7DirShort {
		if snap.LSR >= 3.0 {
			score += 25
			sig.ReasonCodes = append(sig.ReasonCodes, "extreme_long_crowding")
		} else if snap.LSR >= 2.5 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "heavy_long_crowding")
		} else if snap.LSR >= 2.2 {
			score += 12
			sig.ReasonCodes = append(sig.ReasonCodes, "long_crowding")
		}
	} else {
		lsrAbs := snap.LSR
		if lsrAbs > 0 && lsrAbs < 0.5 {
			score += 25
			sig.ReasonCodes = append(sig.ReasonCodes, "extreme_short_crowding")
		} else if lsrAbs < 0.65 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "heavy_short_crowding")
		} else if lsrAbs < 0.8 {
			score += 12
			sig.ReasonCodes = append(sig.ReasonCodes, "short_crowding")
		}
	}

	// 3. Price Divergence (0-20): price vs. funding divergence evidence
	if dir == V7DirShort {
		// For short: price stalling after rally (recent 1h negative while 24h positive)
		if ctx.Change1h < -1 && ctx.Change24h > 3 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "price_stalling_after_rally")
		} else if ctx.Change1h < 0 && ctx.Change24h > 0 {
			score += 12
			sig.ReasonCodes = append(sig.ReasonCodes, "price_turning_down")
		} else if ctx.Change1h < 1 {
			score += 6
			sig.ReasonCodes = append(sig.ReasonCodes, "price_flattening")
		}
	} else {
		// For long: price holding support after drop (recent 1h positive while 24h negative)
		if ctx.Change1h > 1 && ctx.Change24h < -3 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "price_bouncing_from_support")
		} else if ctx.Change1h > 0 && ctx.Change24h < 0 {
			score += 12
			sig.ReasonCodes = append(sig.ReasonCodes, "price_turning_up")
		} else if ctx.Change1h > -1 {
			score += 6
			sig.ReasonCodes = append(sig.ReasonCodes, "price_flattening")
		}
	}

	// 4. OI Structure (0-15): OI behavior confirms crowding
	if dir == V7DirShort {
		// OI elevated or starting to drop (longs getting liquidated)
		if snap.OIDelta1h < -3 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_declining_long_flush")
		} else if snap.OIDelta1h > 5 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_elevated_still_building")
		} else if snap.OIDelta1h > 0 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_mild_buildup")
		}
	} else {
		// OI declining for shorts or stable
		if snap.OIDelta1h > 3 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_rising_short_cover")
		} else if snap.OIDelta1h < -5 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_declining_still_building")
		} else {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_stable")
		}
	}

	// 5. Taker Reversal (0-15): taker flow turning against the crowd
	if dir == V7DirShort {
		if ctx.TakerBuy15m < 0.42 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "strong_taker_sell_reversal")
		} else if ctx.TakerBuy15m < 0.46 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_selling_emerging")
		} else if ctx.TakerBuy15m < 0.50 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_neutral")
		}
	} else {
		if ctx.TakerBuy15m > 0.58 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "strong_taker_buy_reversal")
		} else if ctx.TakerBuy15m > 0.53 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_buying_emerging")
		} else if ctx.TakerBuy15m > 0.50 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "taker_neutral")
		}
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	// Funding reversal requires stronger confirmation — penalty if no taker reversal signal
	hasTakerReversal := false
	for _, code := range sig.ReasonCodes {
		if code == "strong_taker_sell_reversal" || code == "taker_selling_emerging" ||
			code == "strong_taker_buy_reversal" || code == "taker_buying_emerging" {
			hasTakerReversal = true
			break
		}
	}
	if !hasTakerReversal {
		sig.SetupScore *= 0.75
		sig.Status = V7StatusWaitConfirm
	}

	if sig.SetupScore < 30 {
		return nil
	}

	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: wait for price reversal signal
	if ctx.ATR15m > 0 {
		if dir == V7DirShort {
			sig.EntryZone = V7PriceZone{
				Lower: ctx.CurrentPrice - ctx.ATR15m*0.3,
				Upper: ctx.CurrentPrice + ctx.ATR15m*0.5,
			}
		} else {
			sig.EntryZone = V7PriceZone{
				Lower: ctx.CurrentPrice - ctx.ATR15m*0.5,
				Upper: ctx.CurrentPrice + ctx.ATR15m*0.3,
			}
		}
	}

	// Invalidation: beyond the extreme that triggered the signal
	if dir == V7DirShort {
		if ctx.High4h > 0 {
			sig.Invalidation = V7InvalidationRule{
				Price:  ctx.High4h + ctx.ATR4h*0.3,
				Reason: "break_high_squeeze_failed",
			}
		}
	} else {
		if ctx.Low4h > 0 {
			sig.Invalidation = V7InvalidationRule{
				Price:  ctx.Low4h - ctx.ATR4h*0.3,
				Reason: "break_low_squeeze_failed",
			}
		}
	}

	// Targets: mean reversion toward VWAP and key levels
	if dir == V7DirShort {
		if ctx.VWAP15m > 0 && ctx.VWAP15m < ctx.CurrentPrice {
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.VWAP15m, Reason: "vwap_mean_reversion"})
		}
		if ctx.ATR4h > 0 {
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice - ctx.ATR4h*2, Reason: "funding_reversal_target"})
		}
		if ctx.Low1h > 0 && ctx.Low1h < ctx.CurrentPrice {
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.Low1h, Reason: "1h_low_target"})
		}
	} else {
		if ctx.VWAP15m > 0 && ctx.VWAP15m > ctx.CurrentPrice {
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.VWAP15m, Reason: "vwap_mean_reversion"})
		}
		if ctx.ATR4h > 0 {
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR4h*2, Reason: "funding_reversal_target"})
		}
		if ctx.High1h > 0 && ctx.High1h > ctx.CurrentPrice {
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.High1h, Reason: "1h_high_target"})
		}
	}

	// Fallback execution boundaries: every AI-facing signal must include a
	// concrete invalidation and at least one target even when a symbol lacks
	// complete 4h/VWAP context.
	if sig.Invalidation.Price <= 0 && ctx.CurrentPrice > 0 {
		stopDistance := ctx.ATR1h
		if stopDistance <= 0 {
			stopDistance = ctx.ATR15m * 2
		}
		if stopDistance <= 0 {
			stopDistance = ctx.CurrentPrice * 0.025
		}
		if dir == V7DirShort {
			sig.Invalidation = V7InvalidationRule{
				Price:  ctx.CurrentPrice + stopDistance,
				Reason: "fallback_atr_stop_above_reversal_level",
			}
		} else {
			stop := ctx.CurrentPrice - stopDistance
			if stop <= 0 {
				stop = ctx.CurrentPrice * 0.975
			}
			sig.Invalidation = V7InvalidationRule{
				Price:  stop,
				Reason: "fallback_atr_stop_below_reversal_level",
			}
		}
	}

	if len(sig.Targets) == 0 && ctx.CurrentPrice > 0 {
		targetDistance := ctx.ATR1h * 1.5
		if targetDistance <= 0 {
			targetDistance = ctx.ATR15m * 3
		}
		if targetDistance <= 0 {
			targetDistance = ctx.CurrentPrice * 0.03
		}
		if dir == V7DirShort {
			target := ctx.CurrentPrice - targetDistance
			if target <= 0 {
				target = ctx.CurrentPrice * 0.97
			}
			sig.Targets = append(sig.Targets, V7Target{Price: target, Reason: "fallback_atr_reversal_target"})
		} else {
			sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + targetDistance, Reason: "fallback_atr_reversal_target"})
		}
	}

	// Timing score: funding/crowding reversals need their own timing model.
	sig.TimingScore = calcFundingReversalTimingScore(sig, ctx)
	if sig.TimingScore >= 60 && hasTakerReversal {
		sig.Status = V7StatusCandidate
	}

	return sig
}

func fundingReversalConfirmations(dir V7Direction) []string {
	if dir == V7DirShort {
		return []string{
			"15m_close_below_vwap",
			"taker_buy_15m_lt_0_45",
			"funding_remains_positive_or_lsr_crowded_long",
			"oi_flush_or_failed_oi_rebuild",
			"no_new_high_after_crowding_signal",
		}
	}
	return []string{
		"15m_reclaim_vwap",
		"taker_buy_15m_gt_0_52",
		"funding_remains_negative_or_lsr_crowded_short",
		"oi_stabilizes_or_short_covering_starts",
		"no_new_low_after_crowding_signal",
	}
}

func calcFundingReversalTimingScore(sig *V7SignalOutput, ctx *V7SymbolContext) float64 {
	snap := ctx.Snapshot
	if snap == nil {
		return calcTimingScore(sig, ctx)
	}

	var timing float64

	// Price trigger: reversal candidates should wait for VWAP reclaim/loss.
	if ctx.VWAP15m > 0 {
		if sig.Direction == V7DirShort && ctx.CurrentPrice < ctx.VWAP15m {
			timing += 25
		}
		if sig.Direction == V7DirLong && ctx.CurrentPrice > ctx.VWAP15m {
			timing += 25
		}
	}

	// Taker confirmation: crowd starts unwinding.
	if sig.Direction == V7DirShort {
		switch {
		case ctx.TakerBuy15m < 0.42:
			timing += 25
		case ctx.TakerBuy15m < 0.45:
			timing += 20
		case ctx.TakerBuy15m < 0.48:
			timing += 12
		}
		if snap.OIDelta1h < -3 && ctx.Change1h < 0 {
			timing += 20
		} else if snap.OIDelta1h < -3 || ctx.Change1h < 0 {
			timing += 12
		}
		if snap.FundingRate > 0.0005 || snap.LSR > 2.2 {
			timing += 10
		}
	} else {
		switch {
		case ctx.TakerBuy15m > 0.58:
			timing += 25
		case ctx.TakerBuy15m > 0.55:
			timing += 20
		case ctx.TakerBuy15m > 0.52:
			timing += 12
		}
		if snap.OIDelta1h > 3 && ctx.Change1h > 0 {
			timing += 20
		} else if snap.OIDelta1h > 3 || ctx.Change1h > 0 {
			timing += 12
		}
		if snap.FundingRate < -0.0003 || snap.LSR < 0.8 {
			timing += 10
		}
	}

	// Tight invalidation improves executability.
	if sig.Invalidation.Price > 0 && ctx.ATR1h > 0 {
		var risk float64
		if sig.Direction == V7DirShort {
			risk = sig.Invalidation.Price - ctx.CurrentPrice
		} else {
			risk = ctx.CurrentPrice - sig.Invalidation.Price
		}
		if risk > 0 {
			riskATR := risk / ctx.ATR1h
			if riskATR <= 1.0 {
				timing += 15
			} else if riskATR <= 1.5 {
				timing += 10
			}
		}
	}

	return clampFloat(timing, 0, 100)
}
