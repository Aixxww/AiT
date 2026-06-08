package local

import "math"

func finalizeV7SignalForExecution(sig *V7SignalOutput, ctx *V7SymbolContext, cfg V7Config) {
	if sig == nil || ctx == nil {
		return
	}
	normalizeV7TargetsForExecution(sig, ctx.CurrentPrice)
	tightenV7InvalidationForExecution(sig, ctx)

	quality := V7ExecNearConfirm
	rr, rrOK := v7SignalRiskReward(sig, ctx.CurrentPrice)
	if !rrOK || rr < 1.2 {
		quality = V7ExecInvalidRR
		sig.Status = V7StatusWaitConfirm
		sig.RiskTags = appendIfMissing(sig.RiskTags, "invalid_rr_context_only")
	} else if rr < 1.5 {
		sig.Status = V7StatusWaitConfirm
		sig.RiskTags = appendIfMissing(sig.RiskTags, "thin_rr_wait_confirm")
	}

	thresholds := cfg.GetSetupThresholds(sig.SetupType)
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
		if sig.TimingScore >= 45 && sig.RiskScore < 55 && rrOK && rr >= 1.5 {
			quality = betterV7ExecutionQuality(quality, V7ExecReady)
		}
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
	}

	if quality == V7ExecNearConfirm && sig.Status == V7StatusCandidate && sig.TimingScore >= 60 && rrOK && rr >= 1.5 {
		quality = V7ExecReady
	}
	sig.ExecutionQuality = quality
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

	bestRR := 0.0
	for _, target := range sig.Targets {
		reward := v7TargetReward(sig.Direction, price, target.Price)
		if reward <= 0 {
			continue
		}
		rr := reward / risk
		if rr > bestRR {
			bestRR = rr
		}
	}
	if bestRR <= 0 {
		return 0, false
	}
	return bestRR, true
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
	sig.Targets = append(valid, expired...)
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
	if sig == nil || price <= 0 || sig.EntryZone.Lower <= 0 || sig.EntryZone.Upper <= sig.EntryZone.Lower {
		return 0, false
	}
	pos := (price - sig.EntryZone.Lower) / (sig.EntryZone.Upper - sig.EntryZone.Lower) * 100
	return clampFloat(pos, 0, 100), true
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
	case V7SetupLeaderMomentumLong:
		if sig.TimingScore < 60 {
			return -3
		}
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
