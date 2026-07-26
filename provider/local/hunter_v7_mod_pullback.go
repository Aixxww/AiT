package local

import "math"

// ============================================================================
// Module A: Pullback Reversal Long
// ============================================================================
// Catches pullbacks in uptrends that reach support zones.
// This is the core "buy the dip" module — it looks for:
//   - Uptrend still intact (EMA alignment)
//   - Price near structural support (4h/1d low within ATR)
//   - OI stabilizing (not in liquidation cascade)
//   - Taker buy recovering (smart money stepping in)
//   - LSR from extreme bearish levels turning up

type pullbackLongModule struct{}

func (m *pullbackLongModule) Name() string           { return "pullback_long" }
func (m *pullbackLongModule) SetupType() V7SetupType { return V7SetupPullbackLong }
func (m *pullbackLongModule) Direction() V7Direction { return V7DirLong }

func (m *pullbackLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	// Must have price data
	if ctx.CurrentPrice <= 0 || ctx.ATR4h <= 0 {
		return false
	}
	// 24h drop between -3% and -18%
	if ctx.Change24h > -3 || ctx.Change24h < -18 {
		return false
	}
	// Near 4h support (within 1.5 ATR)
	distToLow := ctx.CurrentPrice - ctx.Low4h
	if distToLow > 1.5*ctx.ATR4h {
		return false
	}
	// Trend not destroyed: EMA20 > EMA60 or price above EMA60
	if ctx.EMA20_4h > 0 && ctx.EMA60_4h > 0 {
		trendOK := ctx.EMA20_4h > ctx.EMA60_4h || ctx.CurrentPrice > ctx.EMA60_4h
		if !trendOK {
			return false
		}
	}
	return true
}

func (m *pullbackLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	s := newV7Signal(ctx, regime, V7SetupPullbackLong, V7DirLong, V7EntryWaitConfirm, "B")

	// 1. Position Score (0-25): proximity to support
	if ctx.ATR4h > 0 {
		dist := (ctx.CurrentPrice - ctx.Low4h) / ctx.ATR4h
		if dist <= 0.5 {
			s.add(25, "near_4h_support")
		} else if dist <= 1.0 {
			s.add(20, "near_4h_support")
		} else if dist <= 1.5 {
			s.add(15, "approaching_4h_support")
		}
	}
	// 1d support bonus
	if ctx.ATR1d > 0 && ctx.Low1d > 0 {
		dist1d := (ctx.CurrentPrice - ctx.Low1d) / ctx.ATR1d
		if dist1d <= 2.0 {
			s.add(10, "near_1d_support")
		}
	}

	// 2. Trend Integrity (0-20)
	if ctx.EMA20_4h > 0 && ctx.EMA60_4h > 0 {
		if ctx.EMA20_4h > ctx.EMA60_4h {
			s.add(15, "uptrend_intact")
		} else if ctx.CurrentPrice > ctx.EMA60_4h {
			s.add(10, "above_ema60")
		}
	}
	// RSI oversold bonus
	if ctx.RSI1h > 0 && ctx.RSI1h < 35 {
		s.add(5, "rsi_oversold")
	}

	// 3. OI Stabilization (0-15)
	snap := ctx.Snapshot
	if snap != nil {
		if snap.OIDelta1h > -8 && snap.OIDelta1h < 8 {
			s.add(10, "oi_stable")
		} else if snap.OIDelta1h >= 8 {
			// OI increasing = new money entering
			s.add(5, "oi_increasing")
		}
		// OI accumulated
		if snap.OIDelta4h > 8 && snap.OIDelta4h < 30 {
			s.add(5, "oi_accumulation")
		}
	}

	// 4. Taker Recovery (0-20)
	s.takerLadder(v7TakerLadders[V7SetupPullbackLong])

	// 5. LSR Reversal (0-10)
	if snap != nil {
		if snap.LSROldest < 0.9 && snap.LSR > snap.LSROldest {
			s.add(10, "lsr_reversal")
		} else if snap.LSR > 1.0 && snap.LSR > snap.LSRPrev {
			s.add(5, "lsr_improving")
		}
	}

	// 6. Volume Confirmation (0-10) — moderate pullback is healthier
	if ctx.Change24h < -5 && ctx.Change24h > -12 {
		s.add(5, "healthy_pullback")
	}

	// Signal confirmation filter: near_support without confirming signal →
	// lower score. The 0.70 multiplier applies to the clamped score, so clamp
	// here; finish() re-clamps to the identical value.
	s.score = clampFloat(s.score, 0, 100)
	hasSupport := false
	for _, code := range s.sig.ReasonCodes {
		if code == "near_4h_support" || code == "near_1d_support" {
			hasSupport = true
			break
		}
	}
	hasConfirm := false
	for _, code := range s.sig.ReasonCodes {
		if code == "oi_accumulation" || code == "lsr_reversal" || code == "flow_taker_buy_strong" || code == "flow_taker_buy_recovering" {
			hasConfirm = true
			break
		}
	}
	if hasSupport && !hasConfirm {
		s.score *= 0.70
		s.sig.Status = V7StatusWaitConfirm
		// Canonical, machine-evaluable codes. The old spelling
		// taker_buy_gt_0_52 had no evaluator anywhere, so every pullback that
		// entered this branch carried permanently unsatisfiable confirmations
		// and could never leave WATCH.
		s.sig.RequiredConfirms = []string{"taker_buy_15m_gt_0_52", "oi_stabilize", "lsr_turning_up"}
	}

	// Entry near the current price; invalidate below the 4h low; targets are
	// the mean-reversion magnets.
	s.zoneATR(0.5, 0.3)
	if ctx.Low4h > 0 {
		s.invalidate(ctx.Low4h-ctx.ATR4h*0.3, "break_4h_low")
	}
	if ctx.VWAP15m > ctx.CurrentPrice {
		s.target(ctx.VWAP15m, "mean_reversion_vwap")
	}
	if ctx.ATR4h > 0 {
		s.target(ctx.CurrentPrice+ctx.ATR4h*2, "atr_target")
	}

	return s.finish(30)
}

// helper: build price context
func buildPriceCtx(ctx *V7SymbolContext) *V7PriceContext {
	return &V7PriceContext{
		Last:      ctx.CurrentPrice,
		Change1h:  ctx.Change1h,
		Change4h:  ctx.Change4h,
		Change24h: ctx.Change24h,
		ATR1h:     ctx.ATR1h,
		ATR4h:     ctx.ATR4h,
		VWAP15m:   ctx.VWAP15m,
	}
}

// helper: build derivatives context
func buildDerivCtx(ctx *V7SymbolContext) *V7DerivativesContext {
	dc := &V7DerivativesContext{}
	if ctx.Snapshot != nil {
		dc.OIValue = ctx.Snapshot.OI
		dc.OIChange1h = ctx.Snapshot.OIDelta1h
		dc.OIChange4h = ctx.Snapshot.OIDelta4h
		dc.FundingRate = ctx.Snapshot.FundingRate
		dc.LSROldest = ctx.Snapshot.LSROldest
		dc.LSRNewest = ctx.Snapshot.LSR
	}
	dc.TakerBuy15m = ctx.TakerBuy15m
	return dc
}

// calcTimingScore evaluates whether NOW is a good entry time.
func calcTimingScore(sig *V7SignalOutput, ctx *V7SymbolContext) float64 {
	var timing float64

	// Price in entry zone?
	if sig.EntryZone.Lower > 0 && ctx.CurrentPrice >= sig.EntryZone.Lower && ctx.CurrentPrice <= sig.EntryZone.Upper {
		timing += 30
	}

	// Confirming signals present?
	for _, code := range sig.ReasonCodes {
		switch code {
		case "flow_taker_buy_strong", "lsr_reversal", "oi_accumulation":
			timing += 15
		case "flow_taker_buy_recovering", "oi_stable":
			timing += 10
		case "flow_taker_sell_dominant", "flow_taker_sell_emerging", "lsr_bearish_reversal", "oi_distribution", "oi_long_squeeze":
			timing += 15
		// The former taker_selling_emerging arm here was dead: its only
		// producer (funding_reversal) computes its own timing score and never
		// reaches this shared switch, so the U6.3 synonym merge into
		// flow_taker_sell_emerging (the +15 arm above) changes nothing live.
		case "oi_declining_long_flush", "price_turning_down":
			timing += 10
		}
	}

	// R:R check
	if len(sig.Targets) > 0 && sig.Invalidation.Price > 0 {
		var risk, reward float64
		if sig.Direction == V7DirShort {
			risk = sig.Invalidation.Price - ctx.CurrentPrice
			reward = ctx.CurrentPrice - sig.Targets[0].Price
		} else {
			risk = ctx.CurrentPrice - sig.Invalidation.Price
			reward = sig.Targets[0].Price - ctx.CurrentPrice
		}
		if risk > 0 && reward/risk >= 1.8 {
			timing += 25
		} else if risk > 0 && reward/risk >= 1.3 {
			timing += 15
		}
	}

	return clampFloat(timing, 0, 100)
}

// clampFloat restricts v to [lo, hi].
func clampFloat(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
