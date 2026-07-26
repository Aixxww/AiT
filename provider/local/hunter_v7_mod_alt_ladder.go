package local

// ============================================================================
// Alt-Ladder routers: high-amplitude altcoin lifecycle routes
// ============================================================================

type altLadderMomentumLongModule struct{}

func (m *altLadderMomentumLongModule) Name() string           { return "alt_ladder_momentum_long" }
func (m *altLadderMomentumLongModule) SetupType() V7SetupType { return V7SetupAltLadderLong }
func (m *altLadderMomentumLongModule) Direction() V7Direction { return V7DirLong }

func (m *altLadderMomentumLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil || ctx.CurrentPrice <= 0 || ctx.Snapshot == nil || coreLiquiditySymbols[ctx.Symbol] {
		return false
	}
	if ctx.Snapshot.QuoteVolume24h < 5_000_000 || ctx.Change24h < 5 || ctx.Change24h > 120 {
		return false
	}
	extreme := altLadderExtremeLong(ctx)
	if !(ctx.Change1h >= 1.2 || ctx.Change4h >= 4 || ctx.Velocity15m >= 0.8 || (extreme && ctx.Change4h >= 10)) {
		return false
	}
	minVotes := 2
	if extreme && altLadderExtremeLongTrendOK(ctx) {
		minVotes = 1
	}
	if !altLadderLongStructureOK(ctx) || altLadderLongFlowVotes(ctx) < minVotes {
		return false
	}
	if ctx.Change1h > 12 && !extreme {
		return false
	}
	if ctx.Change1h > 18 {
		return false
	}
	if ctx.RSI1h >= 86 && ctx.Snapshot.FundingRate > 0.0008 {
		return false
	}
	return true
}

func (m *altLadderMomentumLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}
	stage := altLadderLongStage(ctx)
	sig := &V7SignalOutput{
		Symbol:           ctx.Symbol,
		Direction:        V7DirLong,
		SetupType:        V7SetupAltLadderLong,
		Status:           V7StatusCandidate,
		EntryMode:        V7EntryMomentumTrailing,
		Confidence:       "B",
		MarketRegime:     regime,
		ReasonCodes:      []string{"alt_ladder_momentum_long", stage},
		RequiredConfirms: []string{"live_price_in_entry_zone", "taker_buy_15m_gt_0_52", "oi_delta_1h_positive_or_quote_volume_expands", "taker_flow_not_flipping_against_direction"},
		PriceCtx:         buildPriceCtx(ctx),
		DerivativesCtx:   buildDerivCtx(ctx),
		ExecutionQuality: V7ExecNearConfirm,
		MarketShape:      V7ShapeCleanMomentum,
		QuoteVolume24h:   ctx.Snapshot.QuoteVolume24h,
		SetupScore:       altLadderLongSetupScore(ctx, stage),
		TimingScore:      altLadderLongTimingScore(ctx),
		EntryZone:        altLadderLongEntryZone(ctx),
		Invalidation:     altLadderLongInvalidation(ctx),
		Targets:          altLadderLongTargets(ctx),
	}
	if ctx.TakerBuy15m >= 0.56 {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "alt_ladder_taker_buy")
	}
	if ctx.Snapshot.OIDelta1h > 0.5 || ctx.Snapshot.OIDelta4h > 2 {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "alt_ladder_oi_inflow")
	}
	if ctx.VolumeBurst15m >= 1.1 || ctx.VolumeBurst1h >= 1.2 {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "alt_ladder_volume_expansion")
	}
	if stage == "alt_ladder_stage_late" || ctx.Change1h > 7 {
		sig.RiskTags = appendIfMissing(sig.RiskTags, "alt_ladder_late_chase_risk")
	}
	if stage == "alt_ladder_stage_extreme" {
		sig.RiskTags = appendIfMissing(sig.RiskTags, "alt_ladder_extreme_continuation_watch")
	}
	return sig
}

type altLadderBreakdownShortModule struct{}

func (m *altLadderBreakdownShortModule) Name() string           { return "alt_ladder_breakdown_short" }
func (m *altLadderBreakdownShortModule) SetupType() V7SetupType { return V7SetupAltLadderShort }
func (m *altLadderBreakdownShortModule) Direction() V7Direction { return V7DirShort }

func (m *altLadderBreakdownShortModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil || ctx.CurrentPrice <= 0 || ctx.Snapshot == nil || coreLiquiditySymbols[ctx.Symbol] {
		return false
	}
	if ctx.Snapshot.QuoteVolume24h < 5_000_000 {
		return false
	}
	if !(ctx.Change1h <= -1.8 || ctx.Change4h <= -4 || ctx.Velocity15m <= -0.8) {
		return false
	}
	if !altLadderShortStructureOK(ctx) || altLadderShortFlowVotes(ctx) < 2 {
		return false
	}
	if ctx.Change1h <= -14 {
		return false
	}
	if ctx.ATR1d > 0 && ctx.Low1d > 0 {
		distToLowATR := (ctx.CurrentPrice - ctx.Low1d) / ctx.ATR1d
		if distToLowATR >= 0 && distToLowATR < 0.30 && ctx.Snapshot.OIDelta4h < -10 {
			return false
		}
	}
	return true
}

func (m *altLadderBreakdownShortModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}
	stage := altLadderShortStage(ctx)
	sig := &V7SignalOutput{
		Symbol:           ctx.Symbol,
		Direction:        V7DirShort,
		SetupType:        V7SetupAltLadderShort,
		Status:           V7StatusCandidate,
		EntryMode:        V7EntryFastConfirm,
		Confidence:       "B",
		MarketRegime:     regime,
		ReasonCodes:      []string{"alt_ladder_breakdown_short", stage},
		RequiredConfirms: []string{"live_price_in_entry_zone", "taker_buy_15m_lt_0_48", "5m_or_15m_close_below_trigger", "taker_flow_not_flipping_against_direction"},
		PriceCtx:         buildPriceCtx(ctx),
		DerivativesCtx:   buildDerivCtx(ctx),
		ExecutionQuality: V7ExecNearConfirm,
		MarketShape:      V7ShapeDistributionShort,
		QuoteVolume24h:   ctx.Snapshot.QuoteVolume24h,
		SetupScore:       altLadderShortSetupScore(ctx, stage),
		TimingScore:      altLadderShortTimingScore(ctx),
		EntryZone:        altLadderShortEntryZone(ctx),
		Invalidation:     altLadderShortInvalidation(ctx),
		Targets:          altLadderShortTargets(ctx),
	}
	if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.46 {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "alt_ladder_taker_sell")
	}
	if ctx.Snapshot.OIDelta1h > 0.5 {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "alt_ladder_new_shorts")
	} else if ctx.Snapshot.OIDelta1h < -2 {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "alt_ladder_long_flush")
	}
	if ctx.VolumeBurst15m >= 1.1 {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "alt_ladder_sell_volume")
	}
	if stage == "alt_ladder_downshift_late" && ctx.Snapshot.OIDelta1h >= 0 {
		sig.RiskTags = appendIfMissing(sig.RiskTags, "alt_ladder_late_short_risk")
	}
	return sig
}

func altLadderLongStructureOK(ctx *V7SymbolContext) bool {
	return (ctx.VWAP15m > 0 && ctx.CurrentPrice >= ctx.VWAP15m) ||
		(ctx.EMA20_1h > 0 && ctx.CurrentPrice >= ctx.EMA20_1h) ||
		(ctx.BBMiddle15m > 0 && ctx.CurrentPrice >= ctx.BBMiddle15m)
}

func altLadderShortStructureOK(ctx *V7SymbolContext) bool {
	return (ctx.VWAP15m > 0 && ctx.CurrentPrice <= ctx.VWAP15m) ||
		(ctx.EMA20_1h > 0 && ctx.CurrentPrice <= ctx.EMA20_1h) ||
		(ctx.BBMiddle15m > 0 && ctx.CurrentPrice <= ctx.BBMiddle15m)
}

func altLadderLongFlowVotes(ctx *V7SymbolContext) int {
	votes := 0
	if ctx.TakerBuy15m >= 0.52 {
		votes++
	}
	if ctx.Snapshot != nil && (ctx.Snapshot.OIDelta1h > 0.5 || ctx.Snapshot.OIDelta4h > 2) {
		votes++
	}
	if ctx.VolumeBurst15m >= 1.1 || ctx.VolumeBurst1h >= 1.2 {
		votes++
	}
	if ctx.Change1h >= 2.5 {
		votes++
	}
	return votes
}

func altLadderShortFlowVotes(ctx *V7SymbolContext) int {
	votes := 0
	if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m <= 0.48 {
		votes++
	}
	if ctx.Snapshot != nil && (ctx.Snapshot.OIDelta1h > 0.5 || ctx.Snapshot.OIDelta1h < -2) {
		votes++
	}
	if ctx.VolumeBurst15m >= 1.1 {
		votes++
	}
	if ctx.Change4h <= -5 {
		votes++
	}
	return votes
}

func altLadderLongStage(ctx *V7SymbolContext) string {
	switch {
	case altLadderExtremeLong(ctx):
		return "alt_ladder_stage_extreme"
	case ctx.Change24h >= 25 || ctx.Change4h >= 18:
		return "alt_ladder_stage_late"
	case ctx.Change24h >= 12 || ctx.Change4h >= 8:
		return "alt_ladder_stage_mid"
	default:
		return "alt_ladder_stage_early"
	}
}

func altLadderShortStage(ctx *V7SymbolContext) string {
	switch {
	case ctx.Change24h <= -20 || ctx.Change4h <= -15:
		return "alt_ladder_downshift_late"
	case ctx.Change24h <= -10 || ctx.Change4h <= -8:
		return "alt_ladder_downshift_mid"
	default:
		return "alt_ladder_downshift_early"
	}
}

func altLadderLongSetupScore(ctx *V7SymbolContext, stage string) float64 {
	score := 46.0 + float64(altLadderLongFlowVotes(ctx))*10
	if altLadderLongStructureOK(ctx) {
		score += 8
	}
	if stage == "alt_ladder_stage_mid" {
		score += 8
	} else if stage == "alt_ladder_stage_late" {
		score -= 4
	} else if stage == "alt_ladder_stage_extreme" {
		score -= 10
		if altLadderExtremeLongTrendOK(ctx) {
			score += 8
		}
	}
	return clampFloat(score, 0, 100)
}

func altLadderShortSetupScore(ctx *V7SymbolContext, stage string) float64 {
	score := 48.0 + float64(altLadderShortFlowVotes(ctx))*10
	if altLadderShortStructureOK(ctx) {
		score += 8
	}
	if stage == "alt_ladder_downshift_mid" {
		score += 8
	} else if stage == "alt_ladder_downshift_late" {
		score -= 2
	}
	return clampFloat(score, 0, 100)
}

func altLadderLongTimingScore(ctx *V7SymbolContext) float64 {
	timing := 38.0 + float64(altLadderLongFlowVotes(ctx))*9
	if ctx.Change1h >= 1.2 && ctx.Change1h <= 6 {
		timing += 12
	}
	if ctx.Velocity15m >= 0.8 && ctx.Velocity15m <= 3 {
		timing += 8
	}
	if ctx.Change1h > 7 || ctx.RSI1h >= 80 {
		timing -= 12
	}
	if altLadderExtremeLong(ctx) {
		timing -= 18
		if ctx.Snapshot != nil && ctx.Snapshot.OIDelta4h >= 20 && ctx.Change4h >= 10 {
			timing += 8
		}
	}
	return clampFloat(timing, 0, 100)
}

func altLadderShortTimingScore(ctx *V7SymbolContext) float64 {
	timing := 40.0 + float64(altLadderShortFlowVotes(ctx))*9
	if ctx.Change1h <= -1.8 && ctx.Change1h >= -8 {
		timing += 12
	}
	if ctx.Velocity15m <= -0.8 && ctx.Velocity15m >= -4 {
		timing += 8
	}
	if ctx.Change1h < -10 {
		timing -= 10
	}
	return clampFloat(timing, 0, 100)
}

func altLadderLongEntryZone(ctx *V7SymbolContext) V7PriceZone {
	pad := ctx.CurrentPrice * 0.006
	if ctx.ATR15m > 0 {
		pad = maxFloat(pad, ctx.ATR15m*0.45)
	}
	if altLadderExtremeLong(ctx) {
		anchor := 0.0
		if ctx.EMA25_15m > 0 {
			anchor = ctx.EMA25_15m
		}
		if ctx.VWAP15m > 0 && (anchor <= 0 || ctx.VWAP15m > anchor) && ctx.VWAP15m < ctx.CurrentPrice {
			anchor = ctx.VWAP15m
		}
		if anchor > 0 && anchor < ctx.CurrentPrice {
			return V7PriceZone{Lower: anchor - pad*0.25, Upper: anchor + pad*0.75}
		}
	}
	return V7PriceZone{Lower: ctx.CurrentPrice - pad*0.75, Upper: ctx.CurrentPrice + pad*0.35}
}

func altLadderShortEntryZone(ctx *V7SymbolContext) V7PriceZone {
	pad := ctx.CurrentPrice * 0.006
	if ctx.ATR15m > 0 {
		pad = maxFloat(pad, ctx.ATR15m*0.45)
	}
	return V7PriceZone{Lower: ctx.CurrentPrice - pad*0.35, Upper: ctx.CurrentPrice + pad*0.75}
}

func altLadderLongInvalidation(ctx *V7SymbolContext) V7InvalidationRule {
	stopDist := ctx.CurrentPrice * 0.022
	if ctx.ATR15m > 0 {
		stopDist = maxFloat(stopDist, ctx.ATR15m*1.0)
	}
	if ctx.VWAP15m > 0 && ctx.CurrentPrice > ctx.VWAP15m {
		vwapDist := ctx.CurrentPrice - ctx.VWAP15m
		if vwapDist > 0 && vwapDist <= ctx.CurrentPrice*0.035 {
			stopDist = maxFloat(stopDist, vwapDist+ctx.CurrentPrice*0.004)
		}
	}
	return V7InvalidationRule{Price: ctx.CurrentPrice - stopDist, Reason: "alt_ladder_long_structure_lost"}
}

func altLadderShortInvalidation(ctx *V7SymbolContext) V7InvalidationRule {
	stopDist := ctx.CurrentPrice * 0.022
	if ctx.ATR15m > 0 {
		stopDist = maxFloat(stopDist, ctx.ATR15m*1.0)
	}
	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		vwapDist := ctx.VWAP15m - ctx.CurrentPrice
		if vwapDist > 0 && vwapDist <= ctx.CurrentPrice*0.035 {
			stopDist = maxFloat(stopDist, vwapDist+ctx.CurrentPrice*0.004)
		}
	}
	return V7InvalidationRule{Price: ctx.CurrentPrice + stopDist, Reason: "alt_ladder_short_reclaim_stop"}
}

func altLadderLongTargets(ctx *V7SymbolContext) []V7Target {
	targetDist := ctx.CurrentPrice * 0.04
	if ctx.ATR1h > 0 {
		targetDist = maxFloat(targetDist, ctx.ATR1h*1.2)
	}
	targetDist = minFloat(targetDist, ctx.CurrentPrice*0.075)
	return []V7Target{{Price: ctx.CurrentPrice + targetDist, Reason: "alt_ladder_next_leg_target"}}
}

func altLadderExtremeLong(ctx *V7SymbolContext) bool {
	return ctx != nil && (ctx.Change24h >= 45 || ctx.Change4h >= 25)
}

func altLadderExtremeLongTrendOK(ctx *V7SymbolContext) bool {
	if ctx == nil || ctx.Snapshot == nil {
		return false
	}
	emaFan := ctx.EMA7_15m > 0 && ctx.EMA25_15m > 0 && ctx.EMA99_15m > 0 &&
		ctx.EMA7_15m > ctx.EMA25_15m && ctx.EMA25_15m > ctx.EMA99_15m
	return emaFan && ctx.Change4h >= 10 && ctx.Snapshot.OIDelta4h >= 15
}

func altLadderShortTargets(ctx *V7SymbolContext) []V7Target {
	targetDist := ctx.CurrentPrice * 0.04
	if ctx.ATR1h > 0 {
		targetDist = maxFloat(targetDist, ctx.ATR1h*1.2)
	}
	targetDist = minFloat(targetDist, ctx.CurrentPrice*0.075)
	return []V7Target{{Price: ctx.CurrentPrice - targetDist, Reason: "alt_ladder_downshift_target"}}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
