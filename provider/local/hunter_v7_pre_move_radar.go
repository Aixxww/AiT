package local

import (
	"math"
	"sort"
)

func BuildV7PreMoveRadar(universe []V7SymbolContext, regime V7MarketRegime, cfg V7Config) []V7SignalOutput {
	if cfg.WatchOutput <= 0 || len(universe) == 0 {
		return nil
	}

	watches := make([]V7SignalOutput, 0, cfg.WatchOutput*2)
	for i := range universe {
		ctx := &universe[i]
		if ctx == nil || ctx.CurrentPrice <= 0 || ctx.Snapshot == nil {
			continue
		}
		if ctx.Snapshot.QuoteVolume24h < 8e6 || ctx.Snapshot.OI <= 0 {
			continue
		}
		if sig := scorePreBreakoutWatch(ctx, regime, cfg); sig != nil {
			watches = append(watches, *sig)
		}
		if sig := scoreAccumulationWatch(ctx, regime, cfg); sig != nil {
			watches = append(watches, *sig)
		}
		if sig := scorePreSqueezeWatch(ctx, regime, cfg); sig != nil {
			watches = append(watches, *sig)
		}
		if sig := scorePreDistributionWatch(ctx, regime, cfg); sig != nil {
			watches = append(watches, *sig)
		}
	}

	watches = dedupeV7WatchSignals(watches)
	sort.Slice(watches, func(i, j int) bool {
		return watches[i].AIPriority > watches[j].AIPriority
	})
	if len(watches) > cfg.WatchOutput {
		watches = diversifyV7Signals(watches, cfg.WatchOutput)
	}
	return watches
}

func appendV7WatchSignals(confirmed, watches []V7SignalOutput, cfg V7Config) []V7SignalOutput {
	if cfg.WatchOutput <= 0 || len(watches) == 0 {
		return confirmed
	}
	usedSymbols := make(map[string]bool, len(confirmed))
	for _, sig := range confirmed {
		usedSymbols[sig.Symbol] = true
	}
	out := append([]V7SignalOutput{}, confirmed...)
	added := 0
	for _, sig := range watches {
		if added >= cfg.WatchOutput {
			break
		}
		if usedSymbols[sig.Symbol] {
			continue
		}
		out = append(out, sig)
		usedSymbols[sig.Symbol] = true
		added++
	}
	return out
}

func scorePreBreakoutWatch(ctx *V7SymbolContext, regime V7MarketRegime, cfg V7Config) *V7SignalOutput {
	snap := ctx.Snapshot
	if ctx.BBWidthPercentile <= 0 || ctx.BBWidthPercentile > 18 {
		return nil
	}
	if math.Abs(ctx.Change4h) > 4 || ctx.Change24h > 15 || ctx.Change24h < -10 {
		return nil
	}
	if snap.OIDelta1h < 2 || snap.OIDelta1h > 12 {
		return nil
	}
	if snap.OIDelta4h > 30 || snap.FundingRate > 0.0003 {
		return nil
	}
	nearTrigger := nearUpperTrigger(ctx)
	if !nearTrigger {
		return nil
	}

	sig := newPreMoveWatchSignal(ctx, regime, V7DirLong, V7SetupPreBreakoutWatch, V7EntryWaitBreakout)
	score := 38.0
	score += compressionScore(ctx.BBWidthPercentile, 18)
	score += boundedScore(snap.OIDelta1h, 2, 10, 18)
	if snap.OIDelta4h >= 5 && snap.OIDelta4h <= 25 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_4h_stealth_build")
	}
	if ctx.TakerBuy15m >= 0.50 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_improving")
	}
	if ctx.TakerBuy15m >= 0.54 {
		score += 6
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_bias_before_breakout")
	}
	if snap.FundingRate <= 0 {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "funding_not_crowded")
	}
	sig.ReasonCodes = append(sig.ReasonCodes, "compressed_oi_pre_breakout", "near_breakout_trigger")
	sig.RequiredConfirms = []string{
		"15m_close_above_bb_upper_or_4h_resistance",
		"oi_1h_remains_positive_after_breakout",
		"taker_buy_15m_gt_0_52",
		"no_failed_breakout_back_inside_range",
	}
	trigger := preMoveLongTrigger(ctx)
	sig.EntryZone = V7PriceZone{Lower: trigger, Upper: trigger + atrFallback(ctx)*0.6}
	sig.Invalidation = V7InvalidationRule{Price: preMoveLongInvalidation(ctx), Reason: "lose_pre_breakout_compression_range"}
	sig.Targets = preMoveLongTargets(ctx, "pre_breakout_expansion")
	return finalizePreMoveWatchSignal(sig, ctx, cfg, score)
}

func scoreAccumulationWatch(ctx *V7SymbolContext, regime V7MarketRegime, cfg V7Config) *V7SignalOutput {
	snap := ctx.Snapshot
	if ctx.BBWidthPercentile <= 0 || ctx.BBWidthPercentile > 25 {
		return nil
	}
	if snap.OIDelta4h < 5 || snap.OIDelta4h > 28 {
		return nil
	}
	if math.Abs(ctx.Change4h) > 5 || ctx.Change24h > 12 || ctx.Change24h < -12 {
		return nil
	}
	if ctx.TakerBuy15m < 0.48 || ctx.TakerBuy15m > 0.60 {
		return nil
	}

	sig := newPreMoveWatchSignal(ctx, regime, V7DirLong, V7SetupAccumulationWatch, V7EntryWaitBreakout)
	score := 36.0
	score += compressionScore(ctx.BBWidthPercentile, 25)
	score += boundedScore(snap.OIDelta4h, 5, 22, 20)
	if snap.OIDelta1h > 0 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_1h_confirming_accumulation")
	}
	if math.Abs(ctx.Change1h) <= 1.2 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "quiet_1h_price_action")
	}
	if snap.LSR > 0.8 && snap.LSR < 1.25 {
		score += 6
		sig.ReasonCodes = append(sig.ReasonCodes, "lsr_balanced_accumulation")
	}
	sig.ReasonCodes = append(sig.ReasonCodes, "accumulation_watch", "oi_build_without_price_markup")
	sig.RequiredConfirms = []string{
		"15m_close_above_bb_upper_or_entry_zone",
		"bb_width_expansion_starts",
		"oi_continues_inflow",
		"taker_buy_15m_gt_0_52",
	}
	trigger := preMoveLongTrigger(ctx)
	sig.EntryZone = V7PriceZone{Lower: trigger, Upper: trigger + atrFallback(ctx)*0.5}
	sig.Invalidation = V7InvalidationRule{Price: preMoveLongInvalidation(ctx), Reason: "break_below_accumulation_range"}
	sig.Targets = preMoveLongTargets(ctx, "accumulation_breakout_expansion")
	return finalizePreMoveWatchSignal(sig, ctx, cfg, score)
}

func scorePreSqueezeWatch(ctx *V7SymbolContext, regime V7MarketRegime, cfg V7Config) *V7SignalOutput {
	snap := ctx.Snapshot
	if snap.FundingRate > 0 || snap.LSR >= 0.9 {
		return nil
	}
	if snap.OIDelta1h < 1 || snap.OIDelta4h < 3 {
		return nil
	}
	if ctx.Change1h < -3 || ctx.Change4h < -8 || ctx.Change24h < -22 {
		return nil
	}
	if ctx.TakerBuy15m < 0.50 {
		return nil
	}
	if ctx.VWAP15m > 0 && ctx.CurrentPrice > ctx.VWAP15m*1.01 {
		return nil
	}

	sig := newPreMoveWatchSignal(ctx, regime, V7DirLong, V7SetupPreSqueezeWatch, V7EntryWaitReclaim)
	score := 34.0
	if snap.FundingRate < -0.0002 {
		score += 14
		sig.ReasonCodes = append(sig.ReasonCodes, "negative_funding_short_crowding")
	}
	if snap.LSR > 0 && snap.LSR < 0.75 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "lsr_short_crowded")
	}
	score += boundedScore(snap.OIDelta4h, 3, 18, 16)
	if ctx.TakerBuy15m >= 0.52 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_recovery_before_squeeze")
	}
	if ctx.Change1h >= -1.5 {
		score += 6
		sig.ReasonCodes = append(sig.ReasonCodes, "sell_pressure_stalling")
	}
	sig.ReasonCodes = append(sig.ReasonCodes, "pre_short_squeeze_watch", "short_crowding_without_breakdown")
	sig.RequiredConfirms = []string{
		"15m_close_above_vwap_or_ema20",
		"taker_buy_15m_gt_0_52",
		"oi_stops_building_or_short_covering_starts",
		"no_new_low_after_reclaim",
	}
	trigger := ctx.VWAP15m
	if trigger <= 0 {
		trigger = ctx.CurrentPrice + atrFallback(ctx)*0.3
	}
	sig.EntryZone = V7PriceZone{Lower: trigger, Upper: trigger + atrFallback(ctx)*0.6}
	sig.Invalidation = V7InvalidationRule{Price: preMoveLongInvalidation(ctx), Reason: "new_low_invalidates_pre_squeeze"}
	sig.Targets = preMoveLongTargets(ctx, "short_squeeze_watch_extension")
	return finalizePreMoveWatchSignal(sig, ctx, cfg, score)
}

func scorePreDistributionWatch(ctx *V7SymbolContext, regime V7MarketRegime, cfg V7Config) *V7SignalOutput {
	snap := ctx.Snapshot
	if ctx.Change24h < 10 || ctx.Change24h > 55 {
		return nil
	}
	if snap.OIDelta4h < 8 || snap.OIDelta4h > 35 {
		return nil
	}
	if snap.FundingRate <= 0.0005 && snap.LSR <= 1.8 {
		return nil
	}
	if ctx.TakerBuy15m > 0.50 {
		return nil
	}
	if !nearUpperTrigger(ctx) {
		return nil
	}

	sig := newPreMoveWatchSignal(ctx, regime, V7DirShort, V7SetupPreDistribution, V7EntryWaitReject)
	score := 36.0
	score += boundedScore(ctx.Change24h, 10, 35, 16)
	score += boundedScore(snap.OIDelta4h, 8, 30, 18)
	if snap.FundingRate > 0.0008 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "funding_long_crowding")
	}
	if snap.LSR > 2 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "lsr_extreme_long")
	}
	if ctx.TakerBuy15m < 0.48 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_weakening")
	}
	if math.Abs(ctx.Change1h) <= 2 {
		score += 6
		sig.ReasonCodes = append(sig.ReasonCodes, "rally_stalling_near_high")
	}
	sig.ReasonCodes = append(sig.ReasonCodes, "pre_distribution_watch", "crowded_longs_near_resistance")
	sig.RequiredConfirms = []string{
		"15m_rejection_at_resistance_or_entry_zone",
		"15m_close_below_vwap_or_ema20",
		"taker_buy_15m_lt_0_48",
		"no_new_high_after_rejection",
	}
	trigger := ctx.CurrentPrice
	sig.EntryZone = V7PriceZone{Lower: trigger - atrFallback(ctx)*0.4, Upper: trigger + atrFallback(ctx)*0.3}
	sig.Invalidation = V7InvalidationRule{Price: preMoveShortInvalidation(ctx), Reason: "new_high_invalidates_pre_distribution"}
	sig.Targets = preMoveShortTargets(ctx, "pre_distribution_reversion")
	return finalizePreMoveWatchSignal(sig, ctx, cfg, score)
}

func newPreMoveWatchSignal(ctx *V7SymbolContext, regime V7MarketRegime, direction V7Direction, setup V7SetupType, entry V7EntryMode) *V7SignalOutput {
	return &V7SignalOutput{
		Symbol:           ctx.Symbol,
		Direction:        direction,
		SetupType:        setup,
		Status:           V7StatusWaitConfirm,
		EntryMode:        entry,
		ExecutionQuality: V7ExecWatchOnly,
		Confidence:       "C",
		MarketRegime:     regime,
		PriceCtx:         buildPriceCtx(ctx),
		DerivativesCtx:   buildDerivCtx(ctx),
		RiskTags:         []string{"pre_move_radar", "watch_only", "do_not_open_until_confirmed"},
	}
}

func finalizePreMoveWatchSignal(sig *V7SignalOutput, ctx *V7SymbolContext, cfg V7Config, score float64) *V7SignalOutput {
	if sig == nil || ctx == nil {
		return nil
	}
	if len(sig.Targets) == 0 || sig.Invalidation.Price <= 0 {
		return nil
	}
	sig.SetupScore = clampFloat(score, 0, 100)
	if sig.SetupScore < 50 {
		return nil
	}
	sig.RiskScore = AssessV7Risk(sig, ctx)
	sig.RiskLevel = ClassifyV7RiskLevel(sig.RiskScore)
	sig.LiquidityScore = AssessLiquidityScore(ctx)
	if sig.RiskScore >= 90 || sig.LiquidityScore < 35 {
		return nil
	}
	sig.TimingScore = clampFloat(calcTimingScore(sig, ctx)*0.55, 20, 55)
	sig.RegimeFitScore = preMoveRegimeFit(sig.SetupType, sig.Direction, regimeModuleProxy(ctx, sig.MarketRegime))
	sig.AIPriority = CalcAIPriority(sig, cfg)
	sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "watch_only_no_direct_open")
	return sig
}

func dedupeV7WatchSignals(signals []V7SignalOutput) []V7SignalOutput {
	best := make(map[string]V7SignalOutput)
	for _, sig := range signals {
		key := sig.Symbol
		if existing, ok := best[key]; !ok || sig.AIPriority > existing.AIPriority {
			best[key] = sig
		}
	}
	out := make([]V7SignalOutput, 0, len(best))
	for _, sig := range best {
		out = append(out, sig)
	}
	return out
}

func nearUpperTrigger(ctx *V7SymbolContext) bool {
	atr := atrFallback(ctx)
	if atr <= 0 {
		return false
	}
	levels := []float64{ctx.BBUpper15m, ctx.High1h, ctx.High4h}
	for _, level := range levels {
		if level <= 0 {
			continue
		}
		if math.Abs(ctx.CurrentPrice-level) <= atr*1.2 {
			return true
		}
		if ctx.CurrentPrice > level && ctx.CurrentPrice <= level+atr*0.8 {
			return true
		}
	}
	return false
}

func preMoveLongTrigger(ctx *V7SymbolContext) float64 {
	trigger := math.Max(ctx.BBUpper15m, ctx.High1h)
	if trigger <= 0 {
		trigger = ctx.CurrentPrice + atrFallback(ctx)*0.3
	}
	return trigger
}

func preMoveLongInvalidation(ctx *V7SymbolContext) float64 {
	atr := atrFallback(ctx)
	price := ctx.Low1h
	if ctx.BBLower15m > 0 && (price <= 0 || ctx.BBLower15m < price) {
		price = ctx.BBLower15m
	}
	if price <= 0 || ctx.CurrentPrice-price > atr*4 {
		price = ctx.CurrentPrice - atr*1.2
	}
	return price - atr*0.3
}

func preMoveShortInvalidation(ctx *V7SymbolContext) float64 {
	atr := atrFallback(ctx)
	price := ctx.High1h
	if ctx.High4h > price {
		price = ctx.High4h
	}
	if price <= 0 || price-ctx.CurrentPrice > atr*4 {
		price = ctx.CurrentPrice + atr*1.2
	}
	return price + atr*0.35
}

func preMoveLongTargets(ctx *V7SymbolContext, reason string) []V7Target {
	atr := atrFallback(ctx)
	targets := []V7Target{}
	if ctx.High4h > ctx.CurrentPrice && ctx.High4h-ctx.CurrentPrice <= atr*5 {
		targets = append(targets, V7Target{Price: ctx.High4h, Reason: "4h_high_probe"})
	}
	targets = append(targets, V7Target{Price: ctx.CurrentPrice + atr*3.0, Reason: reason})
	if ctx.ATR4h > 0 {
		targets = append(targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR4h*1.5, Reason: "4h_atr_extension"})
	}
	return targets
}

func preMoveShortTargets(ctx *V7SymbolContext, reason string) []V7Target {
	atr := atrFallback(ctx)
	targets := []V7Target{}
	if ctx.VWAP15m > 0 && ctx.VWAP15m < ctx.CurrentPrice {
		targets = append(targets, V7Target{Price: ctx.VWAP15m, Reason: "vwap_reversion"})
	}
	targets = append(targets, V7Target{Price: ctx.CurrentPrice - atr*3.0, Reason: reason})
	if ctx.EMA20_1h > 0 && ctx.EMA20_1h < ctx.CurrentPrice && ctx.CurrentPrice-ctx.EMA20_1h <= atr*5 {
		targets = append(targets, V7Target{Price: ctx.EMA20_1h, Reason: "ema20_1h_reversion"})
	}
	return targets
}

func atrFallback(ctx *V7SymbolContext) float64 {
	if ctx.ATR15m > 0 {
		return ctx.ATR15m
	}
	if ctx.ATR1h > 0 {
		return ctx.ATR1h * 0.45
	}
	if ctx.ATR4h > 0 {
		return ctx.ATR4h * 0.25
	}
	return ctx.CurrentPrice * 0.006
}

func compressionScore(percentile, maxPercentile float64) float64 {
	if percentile <= 0 || maxPercentile <= 0 {
		return 0
	}
	return clampFloat((maxPercentile-percentile)/maxPercentile*18, 0, 18)
}

func boundedScore(value, lo, hi, maxScore float64) float64 {
	if hi <= lo {
		return 0
	}
	if value < lo {
		return 0
	}
	if value > hi {
		value = hi
	}
	return (value - lo) / (hi - lo) * maxScore
}

func preMoveRegimeFit(setup V7SetupType, direction V7Direction, proxy float64) float64 {
	base := 58.0 + proxy
	switch setup {
	case V7SetupPreBreakoutWatch, V7SetupAccumulationWatch:
		base += 8
	case V7SetupPreSqueezeWatch:
		base += 4
	case V7SetupPreDistribution:
		base += 6
	}
	if direction == V7DirShort {
		base -= 2
	}
	return clampFloat(base, 35, 85)
}

func regimeModuleProxy(ctx *V7SymbolContext, regime V7MarketRegime) float64 {
	switch regime {
	case V7RegimeCompression:
		return 10
	case V7RegimeRotation, V7RegimeRange:
		return 6
	case V7RegimeTrendUp:
		if ctx.Change24h >= 0 {
			return 5
		}
	case V7RegimeTrendDown:
		if ctx.Change24h <= 0 {
			return 3
		}
	case V7RegimeManiaPump:
		return -2
	case V7RegimePanicDump:
		return -6
	}
	return 0
}
