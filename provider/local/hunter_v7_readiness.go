package local

import "math"

func CalculateV7ExecutionReadiness(sig *V7SignalOutput, ctx *V7SymbolContext) V7ExecutionReadiness {
	readiness := V7ExecutionReadiness{
		Tier:        V7ReadinessWatch,
		Reason:      "needs_confirmation",
		DataQuality: "complete",
	}
	if sig == nil || ctx == nil {
		readiness.Tier = V7ReadinessRejected
		readiness.Reason = "missing_signal_context"
		readiness.BlockedGate = "prompt_data_quality"
		readiness.DataQuality = "partial"
		readiness.MissingHard = []string{"market_data"}
		return readiness
	}

	readiness.MissingHard, readiness.MissingExecution, readiness.MissingContext = v7ReadinessMissingFields(sig, ctx)
	if len(readiness.MissingHard) > 0 {
		readiness.DataQuality = "partial"
		readiness.Tier = V7ReadinessRejected
		readiness.Reason = readiness.MissingHard[0] + "_missing"
		readiness.BlockedGate = "execution_geometry"
		readiness.NextConfirm = append([]string{}, sig.RequiredConfirms...)
		readiness.ReadyScore = v7ClampScore(sig.AIPriority * 0.35)
		return readiness
	}
	if len(readiness.MissingExecution) > 0 || len(readiness.MissingContext) > 0 {
		readiness.DataQuality = "partial"
	}

	price := ctx.CurrentPrice
	if price <= 0 && sig.PriceCtx != nil {
		price = sig.PriceCtx.Last
	}
	readiness.EntryZonePos = -1
	if pos, ok := v7EntryZonePositionPct(sig, price); ok {
		readiness.EntryZonePos = pos
	}
	readiness.PriceDeviation = v7ReadinessPriceDeviationPct(sig, price)
	readiness.WindowHealth = v7ReadinessWindowHealth(sig, ctx, readiness.EntryZonePos)
	readiness.ReadyScore = v7ReadinessScore(sig, ctx, readiness.WindowHealth)
	readiness.NextConfirm = v7ReadinessNextConfirmations(sig)

	switch {
	case sig.Status == V7StatusFiltered || sig.RiskLevel == V7RiskExtreme:
		readiness.Tier = V7ReadinessRejected
		readiness.Reason = "hard_filtered"
		readiness.BlockedGate = "router_priority"
	case sig.ExecutionQuality == V7ExecInvalidRR:
		readiness.Tier = V7ReadinessRejected
		readiness.Reason = "invalid_rr"
		readiness.BlockedGate = "execution_geometry"
	case sig.RiskScore >= 65:
		readiness.Tier = V7ReadinessRejected
		readiness.Reason = "risk_score_gte_65"
		readiness.BlockedGate = "kernel_tier"
	case len(readiness.MissingExecution) > 0:
		readiness.Tier = V7ReadinessReviewable
		readiness.Reason = readiness.MissingExecution[0] + "_missing"
		readiness.BlockedGate = "confirmation_missing"
	case sig.ExecutionQuality == V7ExecReady && readiness.ReadyScore >= 70 && readiness.WindowHealth >= 60:
		readiness.Tier = V7ReadinessExecutable
		readiness.Reason = "readiness_ready"
	case (sig.ExecutionQuality == V7ExecReady || sig.ExecutionQuality == V7ExecNearConfirm) && readiness.ReadyScore >= 55:
		readiness.Tier = V7ReadinessReviewable
		readiness.Reason = "readiness_reviewable"
	case sig.ExecutionQuality == V7ExecChaseRisk:
		readiness.Tier = V7ReadinessWatch
		readiness.Reason = "chase_risk_wait"
		readiness.BlockedGate = "execution_window"
	default:
		readiness.Tier = V7ReadinessWatch
		readiness.Reason = "needs_confirmation"
		readiness.BlockedGate = "confirmation_missing"
	}
	if readiness.BlockedGate == "" && (readiness.Tier == V7ReadinessWatch || readiness.Tier == V7ReadinessRejected) {
		readiness.BlockedGate = "kernel_tier"
	}
	return readiness
}

func v7ReadinessMissingFields(sig *V7SignalOutput, ctx *V7SymbolContext) (hard, execution, context []string) {
	if sig.EntryZone.Lower <= 0 || sig.EntryZone.Upper <= sig.EntryZone.Lower {
		hard = append(hard, "entry_zone")
	}
	if sig.Invalidation.Price <= 0 {
		hard = append(hard, "invalidation")
	}
	if len(sig.Targets) == 0 || sig.Targets[0].Price <= 0 {
		hard = append(hard, "target1")
	}
	if ctx == nil || ctx.CurrentPrice <= 0 {
		hard = append(hard, "market_data")
		return hard, execution, context
	}
	if sig.DerivativesCtx == nil || sig.DerivativesCtx.TakerBuy15m <= 0 {
		execution = append(execution, "taker_buy_15m")
	}
	if sig.SetupType == V7SetupFundingReversal && ctx.VWAP15m <= 0 {
		execution = append(execution, "15m_vwap")
	}
	if ctx.ATR15m <= 0 {
		context = append(context, "15m_atr")
	}
	return hard, execution, context
}

func v7ReadinessScore(sig *V7SignalOutput, ctx *V7SymbolContext, windowHealth float64) float64 {
	setup := sig.SetupScore * 0.25
	timing := sig.TimingScore * 0.20
	flow := v7ReadinessFlowScore(sig, ctx) * 0.20
	zone := windowHealth * 0.15
	rr := v7ReadinessRRScore(sig, ctx.CurrentPrice) * 0.10
	liq := sig.LiquidityScore * 0.05
	fresh := 100.0 * 0.05
	riskPenalty := math.Max(0, sig.RiskScore-35) * 0.35
	return v7ClampScore(setup + timing + flow + zone + rr + liq + fresh - riskPenalty)
}

func v7ReadinessWindowHealth(sig *V7SignalOutput, ctx *V7SymbolContext, zonePos float64) float64 {
	score := 45.0
	if zonePos >= 0 && zonePos <= 100 {
		score += 25
		if zonePos >= 20 && zonePos <= 80 {
			score += 10
		}
	} else if math.Abs(zonePos) <= 150 {
		score += 10
	}
	if v7ReadinessFlowScore(sig, ctx) >= 60 {
		score += 10
	}
	if sig.ConfirmSummary != nil && sig.ConfirmSummary.PassedReview {
		score += 10
	} else if len(sig.RequiredConfirms) > 0 && len(sig.ReasonCodes) >= 2 {
		score += 5
	}
	if math.Abs(ctx.Velocity5m) <= 4 {
		score += 5
	}
	return v7ClampScore(score)
}

func v7ReadinessFlowScore(sig *V7SignalOutput, ctx *V7SymbolContext) float64 {
	taker := ctx.TakerBuy15m
	if sig.DerivativesCtx != nil && sig.DerivativesCtx.TakerBuy15m > 0 {
		taker = sig.DerivativesCtx.TakerBuy15m
	}
	if taker <= 0 {
		return 50
	}
	if sig.Direction == V7DirShort {
		return v7ClampScore(50 + (0.50-taker)*180)
	}
	return v7ClampScore(50 + (taker-0.50)*180)
}

func v7ReadinessRRScore(sig *V7SignalOutput, price float64) float64 {
	if rr, ok := v7SignalRiskReward(sig, price); ok {
		return v7ClampScore((rr - 1.0) * 65)
	}
	return 0
}

func v7ReadinessPriceDeviationPct(sig *V7SignalOutput, price float64) float64 {
	if price <= 0 || sig.EntryZone.Lower <= 0 || sig.EntryZone.Upper <= sig.EntryZone.Lower {
		return 0
	}
	if price >= sig.EntryZone.Lower && price <= sig.EntryZone.Upper {
		return 0
	}
	if price < sig.EntryZone.Lower {
		return (price - sig.EntryZone.Lower) / sig.EntryZone.Lower * 100
	}
	return (price - sig.EntryZone.Upper) / sig.EntryZone.Upper * 100
}

func v7ReadinessNextConfirmations(sig *V7SignalOutput) []string {
	out := append([]string{}, sig.RequiredConfirms...)
	if len(out) == 0 {
		out = defaultV7Confirmations(sig)
	}
	if sig.SetupType == V7SetupPreBreakoutWatch {
		out = appendIfMissing(out, "5m_close_above_trigger")
		out = appendIfMissing(out, "15m_breakout_close")
	}
	return out
}

func v7ClampScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
