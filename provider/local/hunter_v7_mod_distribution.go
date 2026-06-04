package local

import "math"

// ============================================================================
// Module G: Distribution Short
// ============================================================================
// Catches overextended rallies showing signs of distribution (smart money exiting).
// This module looks for:
//   - 24h gain > 20% (overextended)
//   - Price near resistance or far above EMA20 (>3 ATR)
//   - OI surging on 4h (>15%) indicating overcrowded longs
//   - Taker flow weakening (sell pressure emerging)
//   - Volume pattern showing distribution (high vol but stalling price)
//
// Must have "weakening confirmation": TakerSell > 0.55 or price below VWAP.

type distributionShortModule struct{}

func (m *distributionShortModule) Name() string        { return "distribution_short" }
func (m *distributionShortModule) SetupType() V7SetupType { return V7SetupDistributionShort }
func (m *distributionShortModule) Direction() V7Direction  { return V7DirShort }

func (m *distributionShortModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	// Must have price data
	if ctx.CurrentPrice <= 0 || ctx.ATR4h <= 0 {
		return false
	}
	// 24h gain > 20%
	if ctx.Change24h < 20 {
		return false
	}
	// Must be near resistance OR far from EMA20
	nearResistance := false
	if ctx.ATR4h > 0 && ctx.High4h > 0 {
		distToHigh := (ctx.High4h - ctx.CurrentPrice) / ctx.ATR4h
		if distToHigh < 1.0 {
			nearResistance = true
		}
	}
	farFromEMA := false
	if ctx.EMA20_4h > 0 && ctx.ATR4h > 0 {
		emaDist := (ctx.CurrentPrice - ctx.EMA20_4h) / ctx.ATR4h
		if emaDist > 3.0 {
			farFromEMA = true
		}
	}
	if !nearResistance && !farFromEMA {
		return false
	}
	// OI 4h must be surging (>+15%)
	snap := ctx.Snapshot
	if snap != nil {
		if snap.OIDelta4h <= 15 {
			return false
		}
	}
	return true
}

func (m *distributionShortModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	snap := ctx.Snapshot

	// Weakening confirmation gate: must have TakerSell > 0.55 or price below VWAP
	hasWeakeningConfirm := false
	takerSell := 1.0 - ctx.TakerBuy15m
	if takerSell > 0.55 {
		hasWeakeningConfirm = true
	}
	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		hasWeakeningConfirm = true
	}
	if !hasWeakeningConfirm {
		return nil
	}

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirShort,
		SetupType:    V7SetupDistributionShort,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryWaitReject,
		Confidence:   "B",
		MarketRegime: regime,
	}

	var score float64

	// 1. Distribution Signature (0-25): classic distribution pattern
	// Big rally with stalling momentum
	if ctx.Change24h > 30 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "extreme_rally")
	} else if ctx.Change24h > 25 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_rally")
	} else if ctx.Change24h > 20 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "overextended_rally")
	}
	// Price near 4h high (rejection zone)
	if ctx.High4h > 0 && ctx.ATR4h > 0 {
		distToHigh := (ctx.High4h - ctx.CurrentPrice) / ctx.ATR4h
		if distToHigh < 0.5 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "at_4h_high")
		} else if distToHigh < 1.0 {
			score += 6
			sig.ReasonCodes = append(sig.ReasonCodes, "near_4h_high")
		}
	}

	// 2. Crowding Risk (0-25): overcrowded longs
	if snap != nil {
		oiD4h := snap.OIDelta4h
		if oiD4h > 30 {
			score += 25
			sig.ReasonCodes = append(sig.ReasonCodes, "extreme_oi_surge")
		} else if oiD4h > 22 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "heavy_oi_surge")
		} else if oiD4h > 15 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_surge")
		}
		// Funding rate positive (longs paying shorts — crowded long)
		if snap.FundingRate > 0.0005 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "positive_funding_crowding")
		}
	}

	// 3. Taker Divergence (0-20): sell pressure emerging
	takerSellRatio := 1.0 - ctx.TakerBuy15m
	if takerSellRatio > 0.60 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_sell_dominant")
	} else if takerSellRatio > 0.55 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_sell_emerging")
	} else if takerSellRatio > 0.50 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_sell_neutral")
	}
	// Price below VWAP is distribution signal
	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "below_vwap_distribution")
	}

	// 4. OI Structure (0-15): OI building while price shows exhaustion
	if snap != nil {
		// OI up 1h and 4h both — continuous crowding
		if snap.OIDelta1h > 5 && snap.OIDelta4h > 15 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "continuous_oi_crowding")
		} else if snap.OIDelta1h > 3 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_expanding")
		}
		// LSR extremely bullish = too many longs
		if snap.LSR > 1.5 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "lsr_extreme_long")
		} else if snap.LSR > 1.2 {
			score += 3
			sig.ReasonCodes = append(sig.ReasonCodes, "lsr_bullish_crowded")
		}
	}

	// 5. Volume Pattern (0-15): distribution volume signature
	// Far above EMA20 = mean reversion pressure
	if ctx.EMA20_4h > 0 && ctx.ATR4h > 0 {
		emaDist := (ctx.CurrentPrice - ctx.EMA20_4h) / ctx.ATR4h
		if emaDist > 4.0 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "extreme_ema_extension")
		} else if emaDist > 3.0 {
			score += 7
			sig.ReasonCodes = append(sig.ReasonCodes, "far_from_ema20")
		}
	}
	// 1h price stalling while 24h is up (momentum divergence)
	if ctx.Change24h > 20 && math.Abs(ctx.Change1h) < 2 {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "momentum_stalling")
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	if sig.SetupScore < 30 {
		return nil
	}

	// Build price and derivatives context
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: wait for rejection from high, enter on pullback
	if ctx.ATR15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.CurrentPrice - ctx.ATR15m*0.3,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.5,
		}
	}

	// Invalidation: above the 4h high + buffer
	if ctx.High4h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.High4h + ctx.ATR4h*0.5,
			Reason: "break_above_distribution_high",
		}
	}

	// Targets: mean reversion toward EMA20 and VWAP
	if ctx.EMA20_4h > 0 && ctx.EMA20_4h < ctx.CurrentPrice {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.EMA20_4h, Reason: "ema20_mean_reversion"})
	}
	if ctx.VWAP15m > 0 && ctx.VWAP15m < ctx.CurrentPrice {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.VWAP15m, Reason: "vwap_mean_reversion"})
	}
	if ctx.ATR4h > 0 {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice - ctx.ATR4h*2, Reason: "atr_reversion_target"})
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
}
