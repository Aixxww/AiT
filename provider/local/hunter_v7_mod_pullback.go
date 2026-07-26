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

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupPullbackLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryWaitConfirm,
		Confidence:   "B",
		MarketRegime: regime,
	}

	var score float64

	// 1. Position Score (0-25): proximity to support
	if ctx.ATR4h > 0 {
		dist := (ctx.CurrentPrice - ctx.Low4h) / ctx.ATR4h
		if dist <= 0.5 {
			score += 25
			sig.ReasonCodes = append(sig.ReasonCodes, "near_4h_support")
		} else if dist <= 1.0 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "near_4h_support")
		} else if dist <= 1.5 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "approaching_4h_support")
		}
	}
	// 1d support bonus
	if ctx.ATR1d > 0 && ctx.Low1d > 0 {
		dist1d := (ctx.CurrentPrice - ctx.Low1d) / ctx.ATR1d
		if dist1d <= 2.0 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "near_1d_support")
		}
	}

	// 2. Trend Integrity (0-20)
	if ctx.EMA20_4h > 0 && ctx.EMA60_4h > 0 {
		if ctx.EMA20_4h > ctx.EMA60_4h {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "uptrend_intact")
		} else if ctx.CurrentPrice > ctx.EMA60_4h {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "above_ema60")
		}
	}
	// RSI oversold bonus
	if ctx.RSI1h > 0 && ctx.RSI1h < 35 {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "rsi_oversold")
	}

	// 3. OI Stabilization (0-15)
	snap := ctx.Snapshot
	if snap != nil {
		if snap.OIDelta1h > -8 && snap.OIDelta1h < 8 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_stable")
		} else if snap.OIDelta1h >= 8 {
			score += 5 // OI increasing = new money entering
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_increasing")
		}
		// OI accumulated
		if snap.OIDelta4h > 8 && snap.OIDelta4h < 30 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_accumulation")
		}
	}

	// 4. Taker Recovery (0-20)
	if ctx.TakerBuy15m > 0.55 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_strong")
	} else if ctx.TakerBuy15m > 0.52 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_recovering")
	} else if ctx.TakerBuy15m > 0.50 {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_neutral")
	}

	// 5. LSR Reversal (0-10)
	if snap != nil {
		if snap.LSROldest < 0.9 && snap.LSR > snap.LSROldest {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "lsr_reversal")
		} else if snap.LSR > 1.0 && snap.LSR > snap.LSRPrev {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "lsr_improving")
		}
	}

	// 6. Volume Confirmation (0-10) — basic check
	if ctx.Change24h < -5 && ctx.Change24h > -12 {
		score += 5 // moderate pullback is healthier
		sig.ReasonCodes = append(sig.ReasonCodes, "healthy_pullback")
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	// Signal confirmation filter: near_support without confirming signal → lower score
	hasSupport := false
	for _, code := range sig.ReasonCodes {
		if code == "near_4h_support" || code == "near_1d_support" {
			hasSupport = true
			break
		}
	}
	hasConfirm := false
	for _, code := range sig.ReasonCodes {
		if code == "oi_accumulation" || code == "lsr_reversal" || code == "taker_buy_strong" || code == "taker_buy_recovering" {
			hasConfirm = true
			break
		}
	}
	if hasSupport && !hasConfirm {
		sig.SetupScore *= 0.70
		sig.Status = V7StatusWaitConfirm
		// Canonical, machine-evaluable codes. The old spelling
		// taker_buy_gt_0_52 had no evaluator anywhere, so every pullback that
		// entered this branch carried permanently unsatisfiable confirmations
		// and could never leave WATCH.
		sig.RequiredConfirms = []string{"taker_buy_15m_gt_0_52", "oi_stabilize", "lsr_turning_up"}
	}

	if sig.SetupScore < 30 {
		return nil
	}

	// Build price context
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: near current price
	if ctx.ATR15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.CurrentPrice - ctx.ATR15m*0.5,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.3,
		}
	}

	// Invalidation: below 4h low
	if ctx.Low4h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.Low4h - ctx.ATR4h*0.3,
			Reason: "break_4h_low",
		}
	}

	// Targets: mean reversion to VWAP and prior resistance
	if ctx.VWAP15m > ctx.CurrentPrice {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.VWAP15m, Reason: "mean_reversion_vwap"})
	}
	if ctx.ATR4h > 0 {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR4h*2, Reason: "atr_target"})
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
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
		case "taker_buy_strong", "lsr_reversal", "oi_accumulation":
			timing += 15
		case "taker_buy_recovering", "oi_stable":
			timing += 10
		case "taker_sell_dominant", "taker_sell_emerging", "lsr_bearish_reversal", "oi_distribution", "oi_long_squeeze":
			timing += 15
		case "taker_selling_emerging", "oi_declining_long_flush", "price_turning_down":
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
