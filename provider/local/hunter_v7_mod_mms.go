package local

// ============================================================================
// MMS-Router: small-cap manipulation morphology routes
// ============================================================================

type mmsBottomWakeLongModule struct{}

func (m *mmsBottomWakeLongModule) Name() string           { return "mms_bottom_wake_long" }
func (m *mmsBottomWakeLongModule) SetupType() V7SetupType { return V7SetupMMSBottomWakeLong }
func (m *mmsBottomWakeLongModule) Direction() V7Direction { return V7DirLong }

func (m *mmsBottomWakeLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil || ctx.CurrentPrice <= 0 || ctx.Snapshot == nil {
		return false
	}
	if !mmsSmallCapProxyOK(ctx) {
		return false
	}
	if ctx.StdRatio1h72 <= 0 || ctx.StdRatio1h72 >= 0.025 {
		return false
	}
	if ctx.VolumeBurst1h < 2.5 {
		return false
	}
	if ctx.Snapshot.OIDelta4h < 12 || ctx.Snapshot.OIDelta4h > 45 {
		return false
	}
	if ctx.Change4h <= -2.5 || ctx.Change4h >= 6 {
		return false
	}
	if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m < 0.48 {
		return false
	}
	return true
}

func (m *mmsBottomWakeLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}
	s := newV7Signal(ctx, regime, V7SetupMMSBottomWakeLong, V7DirLong, V7EntryBreakout, "B")
	s.sig.Status = V7StatusWaitConfirm
	s.reason("mms_bottom_wake", "mms_small_cap_proxy", "mms_quiet_accumulation")
	s.riskTag("mms_breakout_not_confirmed")

	s.add(boundedScore(0.025-ctx.StdRatio1h72, 0.003, 0.02, 22))
	s.add(boundedScore(ctx.VolumeBurst1h, 2.5, 5.0, 20))
	s.add(boundedScore(ctx.Snapshot.OIDelta4h, 12, 30, 24))
	if ctx.Snapshot.OIDelta1h > 0 {
		s.add(8, "oi_consistent_inflow")
	}
	if ctx.VolumeBurst1h >= 3.0 {
		s.add(8, "mms_volume_wake")
	}
	if ctx.Snapshot.OIDelta4h >= 15 {
		s.add(8, "mms_oi_stealth_inflow")
	}
	if ctx.TakerBuy15m >= 0.50 && ctx.TakerBuy15m <= 0.58 {
		s.add(8, "taker_neutral_bullish")
	}
	if ctx.BBWidthPercentile > 0 && ctx.BBWidthPercentile < 25 {
		s.add(8, "extreme_compression")
	}

	timing := 45.0
	if ctx.VolumeBurst1h >= 3.0 && ctx.Snapshot.OIDelta1h > 0 {
		timing = 55
	}
	s.sig.RequiredConfirms = []string{"5m_or_15m_close_through_breakout_level", "oi_or_volume_expands_with_price", "live_price_in_entry_zone"}

	// Breakout-trigger zone anchored on the upper band, not the current price,
	// so the zoneATR/zonePad templates do not apply.
	trigger := ctx.CurrentPrice
	if ctx.BBUpper15m > 0 {
		trigger = ctx.BBUpper15m
	}
	pad := ctx.CurrentPrice * 0.004
	if ctx.ATR15m > 0 {
		pad = maxFloat(pad, ctx.ATR15m*0.35)
	}
	s.sig.EntryZone = V7PriceZone{Lower: trigger, Upper: trigger + pad}
	stop := ctx.CurrentPrice * 0.025
	if ctx.BBLower15m > 0 && ctx.CurrentPrice-ctx.BBLower15m > 0 {
		stop = maxFloat(stop, ctx.CurrentPrice-ctx.BBLower15m)
	}
	s.sig.Invalidation = V7InvalidationRule{Price: ctx.CurrentPrice - stop, Reason: "mms_bottom_wake_range_lost"}
	target := ctx.CurrentPrice + stop*2.2
	if ctx.ATR4h > 0 {
		target = maxFloat(target, ctx.CurrentPrice+ctx.ATR4h*1.2)
	}
	s.sig.Targets = []V7Target{{Price: target, Reason: "mms_bottom_wake_expansion_target"}}
	return s.finishWithTiming(0, timing)
}

type mmsTrendRideLongModule struct{}

func (m *mmsTrendRideLongModule) Name() string           { return "mms_trend_ride_long" }
func (m *mmsTrendRideLongModule) SetupType() V7SetupType { return V7SetupMMSTrendRideLong }
func (m *mmsTrendRideLongModule) Direction() V7Direction { return V7DirLong }

func (m *mmsTrendRideLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil || ctx.CurrentPrice <= 0 || ctx.Snapshot == nil {
		return false
	}
	if !mmsSmallCapProxyOK(ctx) {
		return false
	}
	if ctx.EMA7_15m <= 0 || ctx.EMA25_15m <= 0 || ctx.EMA99_15m <= 0 {
		return false
	}
	if !(ctx.EMA7_15m > ctx.EMA25_15m && ctx.EMA25_15m > ctx.EMA99_15m) {
		return false
	}
	if ctx.Last15mLow <= 0 || ctx.Last15mClose <= 0 {
		return false
	}
	if !(ctx.Last15mLow <= ctx.EMA25_15m*1.006 && ctx.Last15mClose > ctx.EMA25_15m) {
		return false
	}
	if ctx.VolumeBurst15m <= 0 || ctx.VolumeBurst15m > 0.85 {
		return false
	}
	if ctx.RSI1h >= 78 {
		return false
	}
	if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m < 0.50 {
		return false
	}
	return true
}

func (m *mmsTrendRideLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}
	s := newV7Signal(ctx, regime, V7SetupMMSTrendRideLong, V7DirLong, V7EntryMomentumTrailing, "A")
	s.reason("mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "mms_low_volume_retest")
	s.add(65)
	if ctx.Change1h > 0 && ctx.Change4h > 0 {
		s.add(12, "mms_trend_continuation")
	}
	if ctx.Snapshot.OIDelta1h >= 0 {
		s.add(8, "oi_stable")
	}
	if ctx.TakerBuy15m >= 0.54 {
		s.add(8, "taker_buy_strong")
	}
	// Trend-ride longs drift instead of running when the move is dead on both
	// price frames, or when open interest is leaving on both frames. Either
	// condition alone on a single frame is normal noise inside a live trend, so
	// both checks require 1h AND 4h to agree before demoting to review-only.
	priceContextWeak := ctx.Change1h <= 0 && ctx.Change4h <= 0
	oiDoesNotSupport := ctx.Snapshot.OIDelta1h < 0 && ctx.Snapshot.OIDelta4h < 0
	if priceContextWeak || oiDoesNotSupport {
		s.riskTag("mms_weak_continuation_review_only")
	}
	s.sig.RequiredConfirms = []string{"5m_price_holds_ema20_or_trailing_support", "taker_flow_not_flipping_against_direction", "live_price_in_entry_zone"}

	// Retest zone anchored on EMA25, not the current price.
	pad := ctx.CurrentPrice * 0.005
	if ctx.ATR15m > 0 {
		pad = maxFloat(pad, ctx.ATR15m*0.5)
	}
	s.sig.EntryZone = V7PriceZone{Lower: ctx.EMA25_15m, Upper: ctx.EMA25_15m + pad}
	stopDist := ctx.CurrentPrice * 0.022
	if ctx.EMA99_15m > 0 && ctx.CurrentPrice-ctx.EMA99_15m > 0 && ctx.CurrentPrice-ctx.EMA99_15m <= ctx.CurrentPrice*0.035 {
		stopDist = maxFloat(stopDist, ctx.CurrentPrice-ctx.EMA99_15m)
	}
	s.sig.Invalidation = V7InvalidationRule{Price: ctx.CurrentPrice - stopDist, Reason: "mms_trend_ride_ema99_lost"}
	targetDist := stopDist * 2.0
	if targetDist > ctx.CurrentPrice*0.075 {
		targetDist = ctx.CurrentPrice * 0.075
	}
	s.sig.Targets = []V7Target{{Price: ctx.CurrentPrice + targetDist, Reason: "mms_trend_ride_continuation_target"}}
	return s.finishWithTiming(0, 68)
}

type mmsSqueezeEngineLongModule struct{}

func (m *mmsSqueezeEngineLongModule) Name() string           { return "mms_squeeze_engine_long" }
func (m *mmsSqueezeEngineLongModule) SetupType() V7SetupType { return V7SetupMMSSqueezeLong }
func (m *mmsSqueezeEngineLongModule) Direction() V7Direction { return V7DirLong }

func (m *mmsSqueezeEngineLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil || ctx.CurrentPrice <= 0 || ctx.Snapshot == nil {
		return false
	}
	if ctx.Snapshot.QuoteVolume24h < 5_000_000 {
		return false
	}
	if ctx.Snapshot.LSR < 1.55 {
		return false
	}
	if ctx.Snapshot.OIDelta1h < 8 || ctx.Change1h < 2.5 {
		return false
	}
	if ctx.TakerBuy15m < 0.54 && ctx.VolumeBurst15m < 1.3 {
		return false
	}
	return true
}

func (m *mmsSqueezeEngineLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}
	s := newV7Signal(ctx, regime, V7SetupMMSSqueezeLong, V7DirLong, V7EntryFastConfirm, "A")
	s.reason("mms_squeeze_engine", "mms_top_trader_long_lock", "mms_oi_price_squeeze_fuel", "mms_short_ban_active")
	s.add(45)
	s.add(boundedScore(ctx.Snapshot.LSR, 1.55, 2.4, 18))
	s.add(boundedScore(ctx.Snapshot.OIDelta1h, 8, 25, 18))
	s.add(boundedScore(ctx.Change1h, 2.5, 8, 16))
	if ctx.TakerBuy15m >= 0.58 {
		s.add(10, "taker_buy_strong")
	}
	if ctx.VolumeBurst15m >= 1.3 {
		s.add(8, "volume_expansion")
	}
	if ctx.Change24h > 35 || ctx.RSI1h >= 82 {
		s.riskTag("mms_squeeze_late_chase")
	}
	s.sig.RequiredConfirms = []string{"5m_or_15m_close_above_trigger", "taker_buy_15m_gt_0_52", "oi_delta_1h_positive_or_quote_volume_expands"}

	// Asymmetric fast-confirm band around the current price (0.4x pad below,
	// full pad above) — not the symmetric zonePad template.
	pad := ctx.CurrentPrice * 0.006
	if ctx.ATR15m > 0 {
		pad = maxFloat(pad, ctx.ATR15m*0.4)
	}
	s.sig.EntryZone = V7PriceZone{Lower: ctx.CurrentPrice - pad*0.4, Upper: ctx.CurrentPrice + pad}
	stopDist := ctx.CurrentPrice * 0.022
	if ctx.EMA25_1h > 0 && ctx.CurrentPrice-ctx.EMA25_1h > 0 && ctx.CurrentPrice-ctx.EMA25_1h <= ctx.CurrentPrice*0.035 {
		stopDist = maxFloat(stopDist, ctx.CurrentPrice-ctx.EMA25_1h)
	}
	s.sig.Invalidation = V7InvalidationRule{Price: ctx.CurrentPrice - stopDist, Reason: "mms_squeeze_ema25_1h_lost"}
	targetDist := stopDist * 2.0
	if targetDist > ctx.CurrentPrice*0.075 {
		targetDist = ctx.CurrentPrice * 0.075
	}
	s.sig.Targets = []V7Target{{Price: ctx.CurrentPrice + targetDist, Reason: "mms_squeeze_continuation_target"}}
	return s.finishWithTiming(0, 72)
}

func mmsSmallCapProxyOK(ctx *V7SymbolContext) bool {
	if ctx == nil || ctx.Snapshot == nil || coreLiquiditySymbols[ctx.Symbol] {
		return false
	}
	qv := ctx.Snapshot.QuoteVolume24h
	if qv < 3_000_000 || qv > 120_000_000 {
		return false
	}
	oi := ctx.Snapshot.OI
	if oi > 0 && (oi < 300_000 || oi > 20_000_000) {
		return false
	}
	if ctx.Snapshot.TradeCount24h < 20_000 && ctx.VolumeBurst15m < 1.2 && ctx.VolumeBurst1h < 1.5 {
		return false
	}
	return true
}
