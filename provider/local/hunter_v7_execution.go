package local

import (
	"math"
	"sort"
)

func finalizeV7SignalForExecution(sig *V7SignalOutput, ctx *V7SymbolContext, cfg V7Config) {
	if sig == nil || ctx == nil {
		return
	}
	normalizeV7EntryZoneForExecution(sig)
	normalizeV7TargetsForExecution(sig, ctx.CurrentPrice)
	tightenV7InvalidationForExecution(sig, ctx)
	summary := EvaluateV7Confirmations(sig, ctx, cfg)
	sig.ConfirmSummary = &summary

	quality := V7ExecNearConfirm
	rr, rrOK := v7SignalRiskReward(sig, ctx.CurrentPrice)
	if !rrOK || rr < V7MinAbsoluteRR {
		quality = V7ExecInvalidRR
		sig.Status = V7StatusWaitConfirm
		sig.RiskTags = appendIfMissing(sig.RiskTags, "invalid_rr_context_only")
	} else if rr < V7MinExecutableRR {
		sig.Status = V7StatusWaitConfirm
		sig.RiskTags = appendIfMissing(sig.RiskTags, "thin_rr_wait_confirm")
	}

	thresholds := cfg.GetSetupThresholds(sig.SetupType)
	thresholds = fundingFastTrackThresholds(sig, thresholds)
	if pos, ok := v7EntryZonePositionPct(sig, ctx.CurrentPrice); ok {
		if sig.Direction == V7DirShort && thresholds.MinZonePosShort > 0 && int(pos) < thresholds.MinZonePosShort {
			quality = worseV7ExecutionQuality(quality, V7ExecWatchOnly)
			sig.Status = V7StatusWaitConfirm
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "wait_zone_retest_required")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "not_near_short_retest_zone")
		}
		if sig.Direction == V7DirLong && thresholds.MaxZonePosLong < 100 && int(pos) > thresholds.MaxZonePosLong {
			quality = worseV7ExecutionQuality(quality, V7ExecWatchOnly)
			sig.Status = V7StatusWaitConfirm
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "wait_reclaim_or_lower_zone_required")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "not_near_long_reclaim_zone")
		}
	}

	if sig.EntryMode != V7EntryImmediate && sig.TimingScore < 45 {
		quality = worseV7ExecutionQuality(quality, V7ExecWatchOnly)
		sig.Status = V7StatusWaitConfirm
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "low_timing_watch_only")
	}

	switch sig.SetupType {
	case V7SetupPanicReversalLong:
		if sig.TimingScore >= 45 && sig.RiskScore < 55 && rrOK && rr >= V7MinExecutableRR {
			quality = betterV7ExecutionQuality(quality, V7ExecReady)
		}
	case V7SetupIntradayScalp:
		quality = worseV7ExecutionQuality(quality, V7ExecWatchOnly)
		sig.Status = V7StatusWaitConfirm
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "scalp_backend_geometry_context")
		sig.RiskTags = appendIfMissing(sig.RiskTags, "scalp_global_geometry_incompatible")
	case V7SetupFundingReversal:
		finalizeFundingReversalExecution(sig, ctx, &quality)
	case V7SetupShortSqueezeLong:
		if ctx.Change1h > 5 || ctx.Change24h > 18 {
			if sig.TimingScore < 65 {
				quality = worseV7ExecutionQuality(quality, V7ExecChaseRisk)
				sig.Status = V7StatusWaitConfirm
				sig.RiskTags = appendIfMissing(sig.RiskTags, "squeeze_chase_risk")
			}
		}
	case V7SetupLeaderMomentumLong:
		if sig.TimingScore < 60 {
			quality = worseV7ExecutionQuality(quality, V7ExecWatchOnly)
			sig.Status = V7StatusWaitConfirm
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "leader_momentum_timing_watch_only")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "momentum_confirmation_missing")
		}
		if hasV7ExecutionRiskTag(sig, "funding_extreme") {
			quality = worseV7ExecutionQuality(quality, V7ExecChaseRisk)
			sig.Status = V7StatusWaitConfirm
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "momentum_extreme_funding_wait")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "momentum_crowded_long")
		}
		if ctx.RSI1h >= 78 {
			quality = worseV7ExecutionQuality(quality, V7ExecChaseRisk)
			sig.Status = V7StatusWaitConfirm
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "momentum_rsi_overheated_wait")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "momentum_overheated")
		}
		if ctx.Change1h > 4 && sig.TimingScore < 60 {
			quality = worseV7ExecutionQuality(quality, V7ExecChaseRisk)
			sig.Status = V7StatusWaitConfirm
			sig.RiskTags = appendIfMissing(sig.RiskTags, "momentum_chase_risk")
		}
		if leaderMomentumUpperZoneChaseRisk(sig, ctx) {
			quality = worseV7ExecutionQuality(quality, V7ExecChaseRisk)
			sig.Status = V7StatusWaitConfirm
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "leader_momentum_upper_chase_wait")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "momentum_upper_zone_chase")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "do_not_market_chase")
			sig.RequiredConfirms = appendIfMissing(sig.RequiredConfirms, "price_holds_vwap_or_trailing_support")
			sig.RequiredConfirms = appendIfMissing(sig.RequiredConfirms, "oi_delta_1h_positive_or_quote_volume_expands")
		}
	}

	if quality == V7ExecNearConfirm && sig.Status == V7StatusCandidate && sig.TimingScore >= 60 && rrOK && rr >= V7MinExecutableRR {
		quality = V7ExecReady
	}
	sig.ExecutionQuality = quality
	readiness := CalculateV7ExecutionReadiness(sig, ctx)
	sig.ExecutionReadiness = &readiness
	repairV7DisplacementRRTagForExecution(sig, ctx, rr, rrOK)
	annotateV7ExecutionSemantics(sig, ctx, rr, rrOK)
}

func leaderMomentumUpperZoneChaseRisk(sig *V7SignalOutput, ctx *V7SymbolContext) bool {
	if sig == nil || ctx == nil || sig.SetupType != V7SetupLeaderMomentumLong || sig.Direction != V7DirLong {
		return false
	}
	if containsV7String(sig.ReasonCodes, "trigger_memory_confirmed") ||
		containsV7String(sig.ReasonCodes, "confirmed_breakout") ||
		containsV7String(sig.ReasonCodes, "strong_breakout") ||
		containsV7String(sig.ReasonCodes, "taker_sustained_buy") ||
		containsV7String(sig.ReasonCodes, "taker_buy_aggressive") {
		return false
	}
	if containsV7String(sig.ReasonCodes, "micro_pullback") ||
		containsV7String(sig.ReasonCodes, "shallow_pullback") ||
		containsV7String(sig.ReasonCodes, "shallow_pullback_1h") {
		return false
	}
	if !containsV7String(sig.ReasonCodes, "no_pullback_still_running") {
		return false
	}
	pos, ok := v7EntryZonePositionPct(sig, ctx.CurrentPrice)
	if !ok || pos < 65 {
		return false
	}

	weakVotes := 0
	if ctx.RSI1h >= 70 && ctx.TakerBuy15m < 0.60 {
		weakVotes++
	}
	if ctx.Snapshot != nil && ctx.Snapshot.OIDelta1h < 0 {
		weakVotes++
	}
	if ctx.VolumeBurst15m > 0 && ctx.VolumeBurst15m < 0.8 {
		weakVotes++
	}
	if ctx.VWAP15m > 0 && ctx.CurrentPrice > ctx.VWAP15m {
		vwapDistancePct := (ctx.CurrentPrice - ctx.VWAP15m) / ctx.VWAP15m * 100
		if vwapDistancePct >= 4.0 {
			weakVotes++
		}
	}
	if ctx.BBUpper15m > 0 && ctx.CurrentPrice >= ctx.BBUpper15m*0.985 {
		weakVotes++
	}

	return weakVotes >= 2
}

func finalizeFundingReversalExecution(sig *V7SignalOutput, ctx *V7SymbolContext, quality *V7ExecutionQuality) {
	if sig == nil || ctx == nil || quality == nil {
		return
	}
	if sig.Direction == V7DirLong {
		if sig.TimingScore < 65 {
			*quality = worseV7ExecutionQuality(*quality, V7ExecWatchOnly)
			sig.Status = V7StatusWaitConfirm
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "funding_long_needs_stronger_timing")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "funding_long_low_edge")
		}
		if sig.MarketRegime == V7RegimeTrendDown && sig.Confidence == "C" {
			*quality = worseV7ExecutionQuality(*quality, V7ExecWatchOnly)
			sig.Status = V7StatusWaitConfirm
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "trend_down_funding_long_watch_only")
		}
	}
	if sig.Direction == V7DirShort {
		if ctx.Change1h < -5 || ctx.Change24h < -12 {
			if ctx.Snapshot == nil || ctx.Snapshot.OIDelta1h >= 0 {
				*quality = worseV7ExecutionQuality(*quality, V7ExecChaseRisk)
				sig.Status = V7StatusWaitConfirm
				sig.RiskTags = appendIfMissing(sig.RiskTags, "late_short_without_oi_flush")
			}
		}
		if sig.Confidence == "C" && hasV7ExecutionRiskTag(sig, "not_near_short_retest_zone") {
			if ctx.Snapshot == nil || ctx.Snapshot.OIDelta4h > -1.0 {
				*quality = worseV7ExecutionQuality(*quality, V7ExecWatchOnly)
				sig.Status = V7StatusWaitConfirm
				sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "funding_short_weak_4h_flush_wait")
				sig.RiskTags = appendIfMissing(sig.RiskTags, "weak_4h_oi_flush")
			}
		}
	}
}

func fundingFastTrackThresholds(sig *V7SignalOutput, thresholds V7SetupThresholds) V7SetupThresholds {
	if sig == nil || sig.SetupType != V7SetupFundingReversal {
		return thresholds
	}
	if !hasV7ExecutionRiskTag(sig, "fast_tracked_funding") &&
		!containsV7String(sig.ReasonCodes, "funding_extreme_fast_track") {
		return thresholds
	}
	if sig.Direction == V7DirShort && thresholds.MinZonePosShort > 40 {
		thresholds.MinZonePosShort = 40
	}
	if sig.Direction == V7DirLong && thresholds.MaxZonePosLong < 60 {
		thresholds.MaxZonePosLong = 60
	}
	return thresholds
}

func v7SignalRiskReward(sig *V7SignalOutput, price float64) (float64, bool) {
	if sig == nil || price <= 0 || sig.Invalidation.Price <= 0 || len(sig.Targets) == 0 || sig.Targets[0].Price <= 0 {
		return 0, false
	}
	var risk float64
	if sig.Direction == V7DirShort {
		risk = sig.Invalidation.Price - price
	} else {
		risk = price - sig.Invalidation.Price
	}
	if risk <= 0 {
		return 0, false
	}

	targets := v7ActionableTargetsForExecution(sig, price)
	if len(targets) == 0 {
		sig.RiskTags = appendIfMissing(sig.RiskTags, "remote_target_only_context")
		return 0, false
	}
	bestReward := 0.0
	for _, target := range targets {
		reward := v7TargetReward(sig.Direction, price, target.Price)
		if reward > bestReward {
			bestReward = reward
		}
	}
	if bestReward <= 0 {
		return 0, false
	}
	return bestReward / risk, true
}

func normalizeV7EntryZoneForExecution(sig *V7SignalOutput) {
	if sig == nil || sig.EntryZone.Lower <= 0 || sig.EntryZone.Upper <= 0 {
		return
	}
	if sig.EntryZone.Lower <= sig.EntryZone.Upper {
		return
	}
	sig.EntryZone.Lower, sig.EntryZone.Upper = sig.EntryZone.Upper, sig.EntryZone.Lower
	sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "entry_zone_normalized")
}

func tightenV7InvalidationForExecution(sig *V7SignalOutput, ctx *V7SymbolContext) {
	if sig == nil || ctx == nil || ctx.CurrentPrice <= 0 || sig.Invalidation.Price <= 0 {
		return
	}

	price := ctx.CurrentPrice
	currentRisk := 0.0
	switch sig.Direction {
	case V7DirShort:
		currentRisk = sig.Invalidation.Price - price
	case V7DirLong:
		currentRisk = price - sig.Invalidation.Price
	default:
		return
	}
	if currentRisk <= 0 {
		return
	}

	execRisk := v7ExecutionStopDistance(ctx)
	if execRisk <= 0 || currentRisk <= execRisk*1.35 {
		return
	}

	if sig.Direction == V7DirShort {
		sig.Invalidation = V7InvalidationRule{
			Price:  price + execRisk,
			Reason: "execution_near_structure_stop_above_entry",
		}
	} else {
		stop := price - execRisk
		if stop <= 0 {
			return
		}
		sig.Invalidation = V7InvalidationRule{
			Price:  stop,
			Reason: "execution_near_structure_stop_below_entry",
		}
	}
	sig.RiskTags = appendIfMissing(sig.RiskTags, "execution_stop_tightened")
}

func v7ExecutionStopDistance(ctx *V7SymbolContext) float64 {
	if ctx == nil || ctx.CurrentPrice <= 0 {
		return 0
	}
	minDist := ctx.CurrentPrice * 0.02
	maxDist := ctx.CurrentPrice * 0.025
	dist := minDist
	if ctx.ATR15m > 0 {
		dist = math.Max(dist, math.Min(ctx.ATR15m*0.65, maxDist))
	}
	if ctx.ATR1h > 0 {
		dist = math.Min(dist, ctx.ATR1h*1.2)
	}
	return dist
}

func normalizeV7TargetsForExecution(sig *V7SignalOutput, price float64) {
	if sig == nil || price <= 0 || len(sig.Targets) <= 1 {
		return
	}

	valid := make([]V7Target, 0, len(sig.Targets))
	expired := make([]V7Target, 0, len(sig.Targets))
	for _, target := range sig.Targets {
		if v7TargetReward(sig.Direction, price, target.Price) > 0 {
			valid = append(valid, target)
		} else {
			expired = append(expired, target)
		}
	}
	if len(valid) == 0 {
		return
	}
	sort.SliceStable(valid, func(i, j int) bool {
		return v7TargetReward(sig.Direction, price, valid[i].Price) < v7TargetReward(sig.Direction, price, valid[j].Price)
	})
	sig.Targets = append(valid, expired...)
}

func v7ActionableTargetsForExecution(sig *V7SignalOutput, price float64) []V7Target {
	if sig == nil || price <= 0 {
		return nil
	}
	const maxShortTermRewardPct = 8.0
	targets := make([]V7Target, 0, len(sig.Targets))
	for _, target := range sig.Targets {
		reward := v7TargetReward(sig.Direction, price, target.Price)
		if reward <= 0 {
			continue
		}
		if reward/price*100 > maxShortTermRewardPct {
			continue
		}
		targets = append(targets, target)
	}
	sort.SliceStable(targets, func(i, j int) bool {
		return v7TargetReward(sig.Direction, price, targets[i].Price) < v7TargetReward(sig.Direction, price, targets[j].Price)
	})
	return targets
}

func annotateV7ExecutionSemantics(sig *V7SignalOutput, ctx *V7SymbolContext, rr float64, rrOK bool) {
	if sig == nil || ctx == nil {
		return
	}
	if sig.MarketShape == "" {
		sig.MarketShape = inferV7MarketShape(sig.SetupType)
	}
	if sig.MarketShape != "" {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, string(sig.MarketShape))
	}

	sig.EntrySignal = inferV7EntrySignal(sig, ctx, rr, rrOK)
	if sig.EntrySignal != "" {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, string(sig.EntrySignal))
	}
}

func inferV7MarketShape(setup V7SetupType) V7MarketShape {
	switch setup {
	case V7SetupTrendBreakoutLong, V7SetupAccumulationLong, V7SetupMMSBottomWakeLong:
		return V7ShapeTrendBreakout
	case V7SetupLeaderMomentumLong, V7SetupShortSqueezeLong, V7SetupDisplacementLong, V7SetupAltLadderLong, V7SetupMMSTrendRideLong, V7SetupMMSSqueezeLong:
		return V7ShapeCleanMomentum
	case V7SetupPullbackLong:
		return V7ShapePullbackContinuation
	case V7SetupPanicReversalLong:
		return V7ShapePanicReversal
	case V7SetupFundingReversal:
		return V7ShapeFundingCrowdingReverse
	case V7SetupDistributionShort, V7SetupLongSqueezeShort, V7SetupBreakdownShort, V7SetupAltLadderShort:
		return V7ShapeDistributionShort
	case V7SetupRangeReversion:
		return V7ShapeRangeReversion
	case V7SetupPreBreakoutWatch, V7SetupPreSqueezeWatch, V7SetupPreDistribution, V7SetupAccumulationWatch:
		return V7ShapeCompressionPrebreakout
	case V7SetupModuleNoMatch:
		return V7ShapeNoiseNoTrade
	default:
		return ""
	}
}

func inferV7EntrySignal(sig *V7SignalOutput, ctx *V7SymbolContext, rr float64, rrOK bool) V7EntrySignal {
	if sig.LiquidityScore > 0 && sig.LiquidityScore < 50 {
		return V7EntrySignalLiquidityBlocked
	}
	if sig.Status == V7StatusFiltered {
		return V7EntrySignalNoTrade
	}
	if containsV7String(sig.RiskTags, "displacement_rr_repaired_needs_review") {
		return V7EntrySignalRRRepairable
	}
	switch sig.ExecutionQuality {
	case V7ExecReady:
		return V7EntrySignalOpenNow
	case V7ExecInvalidRR:
		if v7RRRepairable(sig, ctx, rr, rrOK) {
			return V7EntrySignalRRRepairable
		}
		return V7EntrySignalRRInvalid
	case V7ExecChaseRisk:
		return V7EntrySignalChaseRisk
	}

	if sig.SetupType == V7SetupTrendBreakoutLong || sig.SetupType == V7SetupAccumulationLong {
		if v7BreakoutConfirmedForOpen(sig, ctx, rr, rrOK) {
			return V7EntrySignalOpenNow
		}
		if v7BreakoutTriggerNear(sig, ctx) {
			return V7EntrySignalTriggerNear
		}
		return V7EntrySignalBreakoutWait
	}

	switch sig.EntryMode {
	case V7EntryWaitReclaim:
		return V7EntrySignalReclaimWait
	case V7EntryWaitBreakout, V7EntryBreakout:
		return V7EntrySignalBreakoutWait
	case V7EntryRangeEdge, V7EntryWaitConfirm, V7EntryWaitReject, V7EntryWaitPriceReversal:
		return V7EntrySignalPullbackWait
	}
	if sig.ExecutionQuality == V7ExecNearConfirm {
		return V7EntrySignalTriggerNear
	}
	return V7EntrySignalNoTrade
}

func v7BreakoutConfirmedForOpen(sig *V7SignalOutput, ctx *V7SymbolContext, rr float64, rrOK bool) bool {
	if sig == nil || ctx == nil || ctx.CurrentPrice <= 0 || sig.EntryZone.Upper <= 0 {
		return false
	}
	if sig.Direction != V7DirLong {
		return false
	}
	if sig.SetupType != V7SetupTrendBreakoutLong && sig.SetupType != V7SetupAccumulationLong {
		return false
	}
	if sig.RiskScore >= 45 || (sig.LiquidityScore > 0 && sig.LiquidityScore < 70) {
		return false
	}
	if !rrOK || rr < V7MinExecutableRR {
		return false
	}
	if ctx.CurrentPrice < sig.EntryZone.Upper {
		return false
	}
	if sig.ConfirmSummary != nil && (!sig.ConfirmSummary.PassedHard || !sig.ConfirmSummary.PassedReview) {
		return false
	}
	if !containsV7String(sig.ReasonCodes, "trigger_memory_confirmed") &&
		!containsV7String(sig.ReasonCodes, "confirmed_breakout") &&
		!containsV7String(sig.ReasonCodes, "strong_breakout") &&
		!containsV7String(sig.ReasonCodes, "breakout_attempt") {
		return false
	}
	if !hunterV7BreakoutFlowOK(sig, ctx) {
		return false
	}
	if ctx.Snapshot != nil && ctx.Snapshot.OIDelta1h < -0.5 {
		return false
	}
	return true
}

func hunterV7BreakoutFlowOK(sig *V7SignalOutput, ctx *V7SymbolContext) bool {
	if ctx != nil && ctx.TakerBuy15m >= 0.55 {
		return true
	}
	return containsV7String(sig.ReasonCodes, "taker_aggressive_buy") ||
		containsV7String(sig.ReasonCodes, "taker_strong_buy") ||
		containsV7String(sig.ReasonCodes, "taker_buy_aggressive")
}

func v7BreakoutTriggerNear(sig *V7SignalOutput, ctx *V7SymbolContext) bool {
	if sig == nil || ctx == nil || ctx.CurrentPrice <= 0 || sig.EntryZone.Upper <= 0 {
		return false
	}
	if sig.Direction != V7DirLong {
		return false
	}
	if sig.RiskScore >= 55 || (sig.LiquidityScore > 0 && sig.LiquidityScore < 70) {
		return false
	}
	trigger := sig.EntryZone.Upper
	if ctx.CurrentPrice > trigger {
		return false
	}
	distancePct := (trigger - ctx.CurrentPrice) / ctx.CurrentPrice * 100
	if distancePct < 0.15 || distancePct > 0.9 {
		return false
	}
	if sig.SetupScore >= 78 {
		return true
	}
	if sig.SetupScore < 74 || ctx.TakerBuy15m < 0.58 {
		return false
	}
	if !containsV7String(sig.ReasonCodes, "clear_air_above") {
		return false
	}
	if ctx.Snapshot != nil && ctx.Snapshot.OIDelta1h < 0 {
		return false
	}
	rr, rrOK := v7SignalRiskReward(sig, ctx.CurrentPrice)
	return rrOK && rr >= V7MinExecutableRR
}

func repairV7DisplacementRRTagForExecution(sig *V7SignalOutput, ctx *V7SymbolContext, rr float64, rrOK bool) {
	if sig == nil || ctx == nil || sig.SetupType != V7SetupDisplacementLong {
		return
	}
	if !containsV7String(sig.RiskTags, "displacement_rr_insufficient") {
		return
	}
	if !rrOK || rr < V7MinExecutableRR {
		return
	}
	if sig.RiskScore >= 55 || (sig.LiquidityScore > 0 && sig.LiquidityScore < 70) {
		return
	}
	if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m < 0.52 {
		return
	}
	if ctx.Snapshot != nil && ctx.Snapshot.OIDelta1h < 0 {
		return
	}

	sig.RiskTags = removeTag(sig.RiskTags, "displacement_rr_insufficient")
	sig.RiskTags = appendIfMissing(sig.RiskTags, "displacement_rr_repaired_needs_review")
	sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "displacement_rr_revalidated_after_execution_repair")
	sig.RequiredConfirms = appendIfMissing(sig.RequiredConfirms, "price_holds_vwap_or_trailing_support")
	sig.RequiredConfirms = appendIfMissing(sig.RequiredConfirms, "taker_buy_15m_stays_above_0_52")
	sig.RequiredConfirms = appendIfMissing(sig.RequiredConfirms, "oi_delta_1h_positive_or_quote_volume_expands")
}

func v7RRRepairable(sig *V7SignalOutput, ctx *V7SymbolContext, rr float64, rrOK bool) bool {
	if sig == nil || ctx == nil {
		return false
	}
	if rrOK && rr >= 1.0 {
		return true
	}
	if containsV7String(sig.RiskTags, "remote_target_only_context") {
		return false
	}
	if sig.SetupScore >= 75 && sig.RiskScore < 45 && sig.LiquidityScore >= 70 {
		return true
	}
	return false
}

func v7TargetReward(direction V7Direction, price, targetPrice float64) float64 {
	if price <= 0 || targetPrice <= 0 {
		return 0
	}
	if direction == V7DirShort {
		return price - targetPrice
	}
	return targetPrice - price
}

func hasV7ExecutionRiskTag(sig *V7SignalOutput, tag string) bool {
	if sig == nil {
		return false
	}
	for _, riskTag := range sig.RiskTags {
		if riskTag == tag {
			return true
		}
	}
	return false
}

func v7EntryZonePositionPct(sig *V7SignalOutput, price float64) (float64, bool) {
	if sig == nil {
		return 0, false
	}
	return V7ZonePositionPctClamped(sig.EntryZone, price)
}

func v7SetupExpectancyBonus(sig *V7SignalOutput) float64 {
	if sig == nil {
		return 0
	}
	switch sig.SetupType {
	case V7SetupPanicReversalLong:
		if sig.Direction == V7DirLong && sig.TimingScore >= 45 && sig.RiskScore < 55 {
			return 6
		}
		return 2
	case V7SetupFundingReversal:
		if sig.Direction == V7DirShort {
			if sig.TimingScore >= 60 {
				return 2
			}
			return -2
		}
		penalty := -8.0
		if sig.MarketRegime == V7RegimeTrendDown {
			penalty -= 4
		}
		if sig.TimingScore < 65 {
			penalty -= 4
		}
		return penalty
	case V7SetupShortSqueezeLong:
		return -5
	case V7SetupBreakdownShort:
		if sig.TimingScore >= 60 && sig.RiskScore < 55 {
			return 3
		}
		return 1
	case V7SetupAltLadderLong:
		if sig.TimingScore >= 60 && !hasV7ExecutionRiskTag(sig, "alt_ladder_late_chase_risk") {
			return 4
		}
		return 1
	case V7SetupAltLadderShort:
		if sig.TimingScore >= 60 && !hasV7ExecutionRiskTag(sig, "alt_ladder_late_short_risk") {
			return 4
		}
		return 1
	case V7SetupLeaderMomentumLong:
		if sig.TimingScore < 60 {
			return -3
		}
		return 2
	case V7SetupMMSBottomWakeLong:
		if sig.TimingScore >= 50 {
			return 3
		}
		return 1
	case V7SetupMMSTrendRideLong:
		return 4
	case V7SetupMMSSqueezeLong:
		return 5
	}
	return 0
}

func v7ExecutionQualityBonus(q V7ExecutionQuality) float64 {
	switch q {
	case V7ExecReady:
		return 6
	case V7ExecNearConfirm:
		return 0
	case V7ExecWatchOnly:
		return -8
	case V7ExecChaseRisk:
		return -12
	case V7ExecInvalidRR:
		return -18
	default:
		return 0
	}
}

func betterV7ExecutionQuality(current, candidate V7ExecutionQuality) V7ExecutionQuality {
	if v7ExecutionQualityRank(candidate) < v7ExecutionQualityRank(current) {
		return candidate
	}
	return current
}

func worseV7ExecutionQuality(current, candidate V7ExecutionQuality) V7ExecutionQuality {
	if v7ExecutionQualityRank(candidate) > v7ExecutionQualityRank(current) {
		return candidate
	}
	return current
}

func v7ExecutionQualityRank(q V7ExecutionQuality) int {
	switch q {
	case V7ExecReady:
		return 0
	case V7ExecNearConfirm:
		return 1
	case V7ExecWatchOnly:
		return 2
	case V7ExecChaseRisk:
		return 3
	case V7ExecInvalidRR:
		return 4
	default:
		return 1
	}
}

func diversifyV7Signals(signals []V7SignalOutput, maxOutput int) []V7SignalOutput {
	if maxOutput <= 0 || len(signals) <= maxOutput {
		return signals
	}

	maxPerSetup := int(math.Max(3, math.Ceil(float64(maxOutput)/3)))
	maxPerDirection := int(math.Max(4, math.Ceil(float64(maxOutput)*0.67)))
	selected := make([]V7SignalOutput, 0, maxOutput)
	used := make([]bool, len(signals))
	setupCounts := make(map[V7SetupType]int)
	directionCounts := make(map[V7Direction]int)

	for i, sig := range signals {
		if len(selected) >= maxOutput {
			break
		}
		if setupCounts[sig.SetupType] >= maxPerSetup || directionCounts[sig.Direction] >= maxPerDirection {
			continue
		}
		selected = append(selected, sig)
		used[i] = true
		setupCounts[sig.SetupType]++
		directionCounts[sig.Direction]++
	}

	for i, sig := range signals {
		if len(selected) >= maxOutput {
			break
		}
		if used[i] {
			continue
		}
		selected = append(selected, sig)
	}
	return selected
}
