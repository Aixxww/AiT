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

func (m *distributionShortModule) Name() string           { return "distribution_short" }
func (m *distributionShortModule) SetupType() V7SetupType { return V7SetupDistributionShort }
func (m *distributionShortModule) Direction() V7Direction { return V7DirShort }

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

	s := newV7Signal(ctx, regime, V7SetupDistributionShort, V7DirShort, V7EntryWaitReject, "B")

	// 1. Distribution Signature (0-25): big rally with stalling momentum.
	if ctx.Change24h > 30 {
		s.add(15, "extreme_rally")
	} else if ctx.Change24h > 25 {
		s.add(12, "strong_rally")
	} else if ctx.Change24h > 20 {
		s.add(8, "overextended_rally")
	}
	if ctx.High4h > 0 && ctx.ATR4h > 0 {
		distToHigh := (ctx.High4h - ctx.CurrentPrice) / ctx.ATR4h
		if distToHigh < 0.5 {
			s.add(10, "at_4h_high")
		} else if distToHigh < 1.0 {
			s.add(6, "near_4h_high")
		}
	}

	// 2. Crowding Risk (0-25): overcrowded longs.
	if snap != nil {
		oiD4h := snap.OIDelta4h
		if oiD4h > 30 {
			s.add(25, "extreme_oi_surge")
		} else if oiD4h > 22 {
			s.add(20, "heavy_oi_surge")
		} else if oiD4h > 15 {
			s.add(15, "oi_surge")
		}
		if snap.FundingRate > 0.0005 {
			s.add(5, "positive_funding_crowding")
		}
	}

	// 3. Taker Divergence (0-20): sell pressure emerging.
	takerSellRatio := 1.0 - ctx.TakerBuy15m
	if takerSellRatio > 0.60 {
		s.add(20, "taker_sell_dominant")
	} else if takerSellRatio > 0.55 {
		s.add(15, "taker_sell_emerging")
	} else if takerSellRatio > 0.50 {
		s.add(8, "taker_sell_neutral")
	}
	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		s.add(5, "below_vwap_distribution")
	}

	// 4. OI Structure (0-15): OI building while price shows exhaustion.
	if snap != nil {
		if snap.OIDelta1h > 5 && snap.OIDelta4h > 15 {
			s.add(10, "continuous_oi_crowding")
		} else if snap.OIDelta1h > 3 {
			s.add(5, "oi_expanding")
		}
		if snap.LSR > 1.5 {
			s.add(5, "lsr_extreme_long")
		} else if snap.LSR > 1.2 {
			s.add(3, "lsr_bullish_crowded")
		}
	}

	// 5. Volume Pattern (0-15): mean-reversion pressure and stalling momentum.
	if ctx.EMA20_4h > 0 && ctx.ATR4h > 0 {
		emaDist := (ctx.CurrentPrice - ctx.EMA20_4h) / ctx.ATR4h
		if emaDist > 4.0 {
			s.add(10, "extreme_ema_extension")
		} else if emaDist > 3.0 {
			s.add(7, "far_from_ema20")
		}
	}
	if ctx.Change24h > 20 && math.Abs(ctx.Change1h) < 2 {
		s.add(5, "momentum_stalling")
	}

	// Entry on rejection pullback; invalidate above the distribution high;
	// targets are the mean-reversion magnets.
	s.zoneATR(0.3, 0.5)
	if ctx.High4h > 0 {
		s.invalidate(ctx.High4h+ctx.ATR4h*0.5, "break_above_distribution_high")
	}
	if ctx.EMA20_4h > 0 && ctx.EMA20_4h < ctx.CurrentPrice {
		s.target(ctx.EMA20_4h, "ema20_mean_reversion")
	}
	if ctx.VWAP15m > 0 && ctx.VWAP15m < ctx.CurrentPrice {
		s.target(ctx.VWAP15m, "vwap_mean_reversion")
	}
	if ctx.ATR4h > 0 {
		s.target(ctx.CurrentPrice-ctx.ATR4h*2, "atr_reversion_target")
	}

	return s.finish(30)
}
