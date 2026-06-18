package local

import "math"

// ============================================================================
// Module: Intraday Scalp Long
// ============================================================================
// Captures short-duration (5-30m) high-frequency micro-waves within the
// current trading session.  TP0 targets 0.8%-1.5% with a very tight stop.
// This module thrives in range and compression regimes where there is
// consistent intraday volatility but no clear directional trend.

type intradayScalpLongModule struct{}

func (m *intradayScalpLongModule) Name() string           { return "intraday_scalp_long" }
func (m *intradayScalpLongModule) SetupType() V7SetupType { return V7SetupIntradayScalp }
func (m *intradayScalpLongModule) Direction() V7Direction  { return V7DirLong }

func (m *intradayScalpLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil {
		return false
	}
	price := ctx.CurrentPrice
	if price <= 0 || ctx.ATR5m <= 0 {
		return false
	}
	// 5m must have velocity and volume burst
	if ctx.Velocity5m <= 0.5 {
		return false
	}
	if ctx.VolumeBurst5m <= 1.5 {
		return false
	}
	// Reject extreme 1h moves (this is a scalper, not a trend-follower)
	if math.Abs(ctx.Change1h) > 8 {
		return false
	}
	// ATR5m must provide enough room (> 0.3% of price)
	if ctx.ATR5m/price < 0.003 {
		return false
	}
	return true
}

func (m *intradayScalpLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	price := ctx.CurrentPrice
	base := 40.0

	// 1. 5m velocity bonus (strong directional move)
	if ctx.Velocity5m > 1.0 {
		base += 10
	}
	if ctx.Velocity5m > 2.0 {
		base += 5
	}

	// 2. Taker buy ratio (recent aggression)
	takerBuy := ctx.TakerBuy15m
	if ctx.Snapshot != nil && ctx.Snapshot.TakerBuy > 0 {
		takerBuy = ctx.Snapshot.TakerBuy
	}
	if takerBuy > 0.55 {
		base += 8
	}
	if takerBuy > 0.60 {
		base += 5
	}

	// 3. Volume burst bonus
	if ctx.VolumeBurst5m > 2.0 {
		base += 7
	}
	if ctx.VolumeBurst5m > 3.0 {
		base += 3
	}

	// 4. BB position: prefer price near lower BB (mean reversion upside)
	if ctx.BBLower15m > 0 && price > 0 {
		bbRange := ctx.BBUpper15m - ctx.BBLower15m
		if bbRange > 0 {
			bbPos := (price - ctx.BBLower15m) / bbRange
			if bbPos < 0.3 {
				base += 6 // near lower band — upside potential
			}
		}
	}

	// 5. VWAP support: price above VWAP is bullish
	if ctx.VWAP15m > 0 && price > ctx.VWAP15m {
		base += 4
	}

	// 6. Regime fitness
	if regime == V7RegimeRange || regime == V7RegimeCompression {
		base += 6
	} else if regime == V7RegimeTrendDown || regime == V7RegimePanicDump {
		base -= 10
	}

	base = clampFloat(base, 0, 100)

	// Targets: tight scalper
	risk := ctx.ATR5m * 0.5 // very tight stop
	stop := price - risk
	tp0 := price + math.Max(risk*0.8, ctx.ATR5m*1.0)   // ~0.8R or 1×ATR5m
	tp1 := price + math.Max(risk*1.5, ctx.ATR5m*2.0)   // ~1.5R or 2×ATR5m
	tp2 := price + math.Max(risk*3.0, ctx.ATR5m*4.0)   // runner

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupIntradayScalp,
		Status:       V7StatusCandidate,
		SetupScore:   base,
		TimingScore:  clampFloat(40+takerBuy*60+ctx.Velocity5m*5, 0, 100),
		EntryMode:    V7EntryFastConfirm,
		Confidence:   "B",
		MarketRegime: regime,
		EntryZone:    V7PriceZone{Lower: price - ctx.ATR5m*0.3, Upper: price + ctx.ATR5m*0.2},
		Invalidation: V7InvalidationRule{Price: stop, Reason: "0.5xATR5m scalp stop"},
		Targets: []V7Target{
			{Price: tp0, Reason: "TP0 scalp"},
			{Price: tp1, Reason: "TP1 intraday"},
			{Price: tp2, Reason: "TP2 runner"},
		},
		PriceCtx: &V7PriceContext{
			Last:      price,
			Change1h:  ctx.Change1h,
			Change4h:  ctx.Change4h,
			Change24h: ctx.Change24h,
			ATR1h:     ctx.ATR1h,
			ATR4h:     ctx.ATR4h,
			VWAP15m:   ctx.VWAP15m,
		},
	}

	if ctx.Snapshot != nil {
		sig.DerivativesCtx = &V7DerivativesContext{
			OIValue:     ctx.Snapshot.OI,
			OIChange1h:  ctx.Snapshot.OIDelta1h,
			OIChange4h:  ctx.Snapshot.OIDelta4h,
			FundingRate: ctx.Snapshot.FundingRate,
			TakerBuy15m: takerBuy,
		}
	}

	sig.ReasonCodes = append(sig.ReasonCodes, "intraday_scalp_entry")
	if ctx.Velocity5m > 1.5 {
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_5m_velocity")
	}
	if takerBuy > 0.58 {
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_strong")
	}

	return sig
}
