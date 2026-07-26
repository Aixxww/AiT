package local

import "math"

// ============================================================================
// Module: Volatility Squeeze Breakout
// ============================================================================
// Identifies instruments where Bollinger Band width is at a historical low
// percentile (deeply compressed) while OI is quietly building.  When the
// breakout occurs the expansion can be violent; this module aims to catch
// the first leg.

type volatilitySqueezeBreakoutModule struct{}

func (m *volatilitySqueezeBreakoutModule) Name() string           { return "volatility_squeeze_breakout" }
func (m *volatilitySqueezeBreakoutModule) SetupType() V7SetupType { return V7SetupVolatilitySqueeze }
func (m *volatilitySqueezeBreakoutModule) Direction() V7Direction { return V7DirLong }

func (m *volatilitySqueezeBreakoutModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil {
		return false
	}
	price := ctx.CurrentPrice
	if price <= 0 {
		return false
	}
	// BB width at extreme low percentile
	if ctx.BBWidthPercentile >= 15 {
		return false
	}
	// 1h ATR confirms compression
	if ctx.ATR1h > 0 && ctx.ATR1h/price >= 0.02 {
		return false
	}
	// OI must be building
	if ctx.Snapshot != nil && ctx.Snapshot.OIDelta1h <= 3 {
		return false
	}
	return true
}

func (m *volatilitySqueezeBreakoutModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	price := ctx.CurrentPrice
	base := 45.0

	// 1. BB compression depth: deeper = higher score
	if ctx.BBWidthPercentile < 10 {
		base += 10
	}
	if ctx.BBWidthPercentile < 5 {
		base += 5
	}

	// 2. OI growth: stronger build-up = higher score
	oiDelta := 0.0
	if ctx.Snapshot != nil {
		oiDelta = ctx.Snapshot.OIDelta1h
	}
	if oiDelta > 5 {
		base += 8
	}
	if oiDelta > 10 {
		base += 5
	}

	// 3. Price proximity to upper BB (bullish breakout direction)
	if ctx.BBUpper15m > 0 && ctx.BBLower15m > 0 {
		bbRange := ctx.BBUpper15m - ctx.BBLower15m
		if bbRange > 0 {
			bbPos := (price - ctx.BBLower15m) / bbRange
			if bbPos > 0.6 {
				base += 7 // near upper band — breakout likely upward
			}
		}
	}

	// 4. Taker buy ratio: directional hint
	takerBuy := ctx.TakerBuy15m
	if takerBuy > 0.53 {
		base += 5
	}
	if takerBuy > 0.57 {
		base += 3
	}

	// 5. Volume not yet exploding (pre-breakout quiet)
	if ctx.VolumeBurst5m < 1.5 && ctx.VolumeBurst5m > 0 {
		base += 4 // low vol = still coiling
	}

	// 6. Regime fitness
	if regime == V7RegimeCompression || regime == V7RegimeRange {
		base += 8
	} else if regime == V7RegimePanicDump {
		base -= 12
	}

	base = clampFloat(base, 0, 100)

	// Stop: BB middle or lower band (structural support)
	stop := ctx.BBLower15m
	if stop <= 0 {
		stop = price - ctx.ATR15m*1.5
	}
	risk := price - stop
	if risk <= 0 {
		risk = ctx.ATR15m
		stop = price - risk
	}

	// Targets based on BB width expansion
	bbWidth := ctx.BBUpper15m - ctx.BBLower15m
	if bbWidth <= 0 {
		bbWidth = ctx.ATR15m * 2
	}
	tp0 := price + math.Max(risk*0.8, bbWidth*0.5)
	tp1 := price + math.Max(risk*1.5, bbWidth*1.0)
	tp2 := price + math.Max(risk*3.0, bbWidth*2.0)

	s := newV7Signal(ctx, regime, V7SetupVolatilitySqueeze, V7DirLong, V7EntryBreakout, "B")
	// This module has no score floor and computes its own timing formula; the
	// header carries the scores directly and the tail stays hand-written.
	s.sig.SetupScore = base
	s.sig.TimingScore = clampFloat(50+takerBuy*40+oiDelta, 0, 100)
	s.sig.EntryZone = V7PriceZone{Lower: ctx.BBMiddle15m, Upper: ctx.BBUpper15m}
	s.sig.Invalidation = V7InvalidationRule{Price: stop, Reason: "BB lower band / structure"}
	s.sig.Targets = []V7Target{
		{Price: tp0, Reason: "TP0 half-BB-width"},
		{Price: tp1, Reason: "TP1 full-BB-width"},
		{Price: tp2, Reason: "TP2 double-BB-width"},
	}
	s.sig.PriceCtx = buildPriceCtx(ctx)

	// Legacy derivatives snapshot omits the LSR fields, so buildDerivCtx does
	// not apply here.
	if ctx.Snapshot != nil {
		s.sig.DerivativesCtx = &V7DerivativesContext{
			OIValue: ctx.Snapshot.OI, OIChange1h: ctx.Snapshot.OIDelta1h,
			OIChange4h: ctx.Snapshot.OIDelta4h, FundingRate: ctx.Snapshot.FundingRate,
			TakerBuy15m: takerBuy,
		}
	}

	s.reason("volatility_squeeze_detected")
	if oiDelta > 5 {
		s.reason("oi_building")
	}
	ApplyV7OIAccumulationEvidence(s.sig, ctx)

	return s.sig
}
