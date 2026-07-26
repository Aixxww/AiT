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
	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupMMSBottomWakeLong,
		Status:       V7StatusWaitConfirm,
		EntryMode:    V7EntryBreakout,
		Confidence:   "B",
		MarketRegime: regime,
		ReasonCodes:  []string{"mms_bottom_wake", "mms_small_cap_proxy", "mms_quiet_accumulation"},
		RiskTags:     []string{"mms_breakout_not_confirmed"},
	}

	score := 0.0
	score += boundedScore(0.025-ctx.StdRatio1h72, 0.003, 0.02, 22)
	score += boundedScore(ctx.VolumeBurst1h, 2.5, 5.0, 20)
	score += boundedScore(ctx.Snapshot.OIDelta4h, 12, 30, 24)
	if ctx.Snapshot.OIDelta1h > 0 {
		score += 8
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "oi_consistent_inflow")
	}
	if ctx.VolumeBurst1h >= 3.0 {
		score += 8
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "mms_volume_wake")
	}
	if ctx.Snapshot.OIDelta4h >= 15 {
		score += 8
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "mms_oi_stealth_inflow")
	}
	if ctx.TakerBuy15m >= 0.50 && ctx.TakerBuy15m <= 0.58 {
		score += 8
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "taker_neutral_bullish")
	}
	if ctx.BBWidthPercentile > 0 && ctx.BBWidthPercentile < 25 {
		score += 8
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "extreme_compression")
	}

	sig.SetupScore = clampFloat(score, 0, 100)
	sig.TimingScore = 45
	if ctx.VolumeBurst1h >= 3.0 && ctx.Snapshot.OIDelta1h > 0 {
		sig.TimingScore = 55
	}
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)
	sig.RequiredConfirms = []string{"5m_or_15m_close_through_breakout_level", "oi_or_volume_expands_with_price", "live_price_in_entry_zone"}

	trigger := ctx.CurrentPrice
	if ctx.BBUpper15m > 0 {
		trigger = ctx.BBUpper15m
	}
	pad := ctx.CurrentPrice * 0.004
	if ctx.ATR15m > 0 {
		pad = maxFloat(pad, ctx.ATR15m*0.35)
	}
	sig.EntryZone = V7PriceZone{Lower: trigger, Upper: trigger + pad}
	stop := ctx.CurrentPrice * 0.025
	if ctx.BBLower15m > 0 && ctx.CurrentPrice-ctx.BBLower15m > 0 {
		stop = maxFloat(stop, ctx.CurrentPrice-ctx.BBLower15m)
	}
	sig.Invalidation = V7InvalidationRule{Price: ctx.CurrentPrice - stop, Reason: "mms_bottom_wake_range_lost"}
	target := ctx.CurrentPrice + stop*2.2
	if ctx.ATR4h > 0 {
		target = maxFloat(target, ctx.CurrentPrice+ctx.ATR4h*1.2)
	}
	sig.Targets = []V7Target{{Price: target, Reason: "mms_bottom_wake_expansion_target"}}
	return sig
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
	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupMMSTrendRideLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryMomentumTrailing,
		Confidence:   "A",
		MarketRegime: regime,
		ReasonCodes:  []string{"mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "mms_low_volume_retest"},
	}
	score := 65.0
	if ctx.Change1h > 0 && ctx.Change4h > 0 {
		score += 12
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "mms_trend_continuation")
	}
	if ctx.Snapshot.OIDelta1h >= 0 {
		score += 8
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "oi_stable")
	}
	if ctx.TakerBuy15m >= 0.54 {
		score += 8
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "taker_buy_strong")
	}
	sig.SetupScore = clampFloat(score, 0, 100)
	sig.TimingScore = 68
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)
	sig.RequiredConfirms = []string{"5m_price_holds_ema20_or_trailing_support", "taker_flow_not_flipping_against_direction", "live_price_in_entry_zone"}

	pad := ctx.CurrentPrice * 0.005
	if ctx.ATR15m > 0 {
		pad = maxFloat(pad, ctx.ATR15m*0.5)
	}
	sig.EntryZone = V7PriceZone{Lower: ctx.EMA25_15m, Upper: ctx.EMA25_15m + pad}
	stopDist := ctx.CurrentPrice * 0.022
	if ctx.EMA99_15m > 0 && ctx.CurrentPrice-ctx.EMA99_15m > 0 && ctx.CurrentPrice-ctx.EMA99_15m <= ctx.CurrentPrice*0.035 {
		stopDist = maxFloat(stopDist, ctx.CurrentPrice-ctx.EMA99_15m)
	}
	sig.Invalidation = V7InvalidationRule{Price: ctx.CurrentPrice - stopDist, Reason: "mms_trend_ride_ema99_lost"}
	targetDist := stopDist * 2.0
	if targetDist > ctx.CurrentPrice*0.075 {
		targetDist = ctx.CurrentPrice * 0.075
	}
	sig.Targets = []V7Target{{Price: ctx.CurrentPrice + targetDist, Reason: "mms_trend_ride_continuation_target"}}
	return sig
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
	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupMMSSqueezeLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryFastConfirm,
		Confidence:   "A",
		MarketRegime: regime,
		ReasonCodes:  []string{"mms_squeeze_engine", "mms_top_trader_long_lock", "mms_oi_price_squeeze_fuel", "mms_short_ban_active"},
	}
	score := 45.0
	score += boundedScore(ctx.Snapshot.LSR, 1.55, 2.4, 18)
	score += boundedScore(ctx.Snapshot.OIDelta1h, 8, 25, 18)
	score += boundedScore(ctx.Change1h, 2.5, 8, 16)
	if ctx.TakerBuy15m >= 0.58 {
		score += 10
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "taker_buy_strong")
	}
	if ctx.VolumeBurst15m >= 1.3 {
		score += 8
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "volume_expansion")
	}
	if ctx.Change24h > 35 || ctx.RSI1h >= 82 {
		sig.RiskTags = appendIfMissing(sig.RiskTags, "mms_squeeze_late_chase")
	}
	sig.SetupScore = clampFloat(score, 0, 100)
	sig.TimingScore = 72
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)
	sig.RequiredConfirms = []string{"5m_or_15m_close_above_trigger", "taker_buy_15m_gt_0_52", "oi_delta_1h_positive_or_quote_volume_expands"}

	pad := ctx.CurrentPrice * 0.006
	if ctx.ATR15m > 0 {
		pad = maxFloat(pad, ctx.ATR15m*0.4)
	}
	sig.EntryZone = V7PriceZone{Lower: ctx.CurrentPrice - pad*0.4, Upper: ctx.CurrentPrice + pad}
	stopDist := ctx.CurrentPrice * 0.022
	if ctx.EMA25_1h > 0 && ctx.CurrentPrice-ctx.EMA25_1h > 0 && ctx.CurrentPrice-ctx.EMA25_1h <= ctx.CurrentPrice*0.035 {
		stopDist = maxFloat(stopDist, ctx.CurrentPrice-ctx.EMA25_1h)
	}
	sig.Invalidation = V7InvalidationRule{Price: ctx.CurrentPrice - stopDist, Reason: "mms_squeeze_ema25_1h_lost"}
	targetDist := stopDist * 2.0
	if targetDist > ctx.CurrentPrice*0.075 {
		targetDist = ctx.CurrentPrice * 0.075
	}
	sig.Targets = []V7Target{{Price: ctx.CurrentPrice + targetDist, Reason: "mms_squeeze_continuation_target"}}
	return sig
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
