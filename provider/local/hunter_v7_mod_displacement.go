package local

import "math"

// ============================================================================
// Module: Displacement Momentum Long
// ============================================================================
// Catches large volatility displacement events — symbols experiencing an
// outsized 1h true range (≥2× the 20h median) with volume expansion and
// aligned derivatives flow. These are the "mover" setups that the funnel
// audit showed were frequently missed.
//
// Anti-chase guardrails prevent entering after the move has already extended
// too far above VWAP or when RSI is extreme.

type displacementMomentumLongModule struct{}

func (m *displacementMomentumLongModule) Name() string           { return "displacement_momentum_long" }
func (m *displacementMomentumLongModule) SetupType() V7SetupType { return V7SetupDisplacementLong }
func (m *displacementMomentumLongModule) Direction() V7Direction { return V7DirLong }

func (m *displacementMomentumLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx.CurrentPrice <= 0 || ctx.Snapshot == nil {
		return false
	}
	// Must have displacement: range expansion >= 2.0
	if ctx.RangeExpansion1h < 2.0 {
		return false
	}
	// Need positive 1h change (upward displacement)
	if ctx.Change1h <= 0 {
		return false
	}
	// OI delta must confirm direction (longs entering)
	snap := ctx.Snapshot
	oiDeltaMissing := snap.OI <= 0 || (snap.OIDelta1h == 0 && snap.OIDelta4h == 0)
	if snap.OIDelta1h < 1 && !(oiDeltaMissing && ctx.Amplitude24h >= 12 && ctx.TakerBuy15m >= 0.52) {
		return false
	}
	// Funding must not be extreme (crowded longs already)
	if snap.FundingRate > 0.001 {
		return false
	}
	// Taker flow must be aligned (buy side dominant)
	if ctx.TakerBuy15m > 0 && ctx.TakerBuy15m < 0.48 {
		return false
	}
	return true
}

func (m *displacementMomentumLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	snap := ctx.Snapshot
	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupDisplacementLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryMomentumTrailing,
		Confidence:   "B",
		MarketRegime: regime,
	}

	var score float64

	// 1. Displacement magnitude (0-30): how large is the volatility break
	if ctx.RangeExpansion1h >= 3.0 {
		score += 30
		sig.ReasonCodes = append(sig.ReasonCodes, "massive_vol_displacement")
	} else if ctx.RangeExpansion1h >= 2.5 {
		score += 22
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_vol_displacement")
	} else {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_vol_displacement")
	}

	// 2. Price momentum confirmation (0-20)
	if ctx.Change1h >= 4 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_1h_impulse")
	} else if ctx.Change1h >= 2 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "solid_1h_impulse")
	} else {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "early_1h_displacement")
	}

	// 3. 4h range break or key level reclaim (0-20)
	if ctx.High4h > 0 && ctx.CurrentPrice >= ctx.High4h*0.995 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "breaks_4h_high")
	} else if ctx.High1h > 0 && ctx.CurrentPrice >= ctx.High1h*0.998 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "breaks_1h_high")
	} else if ctx.VWAP15m > 0 && ctx.CurrentPrice > ctx.VWAP15m {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "above_vwap_15m")
	}

	// 4. OI confirmation (0-15)
	oiDeltaMissing := snap.OI <= 0 || (snap.OIDelta1h == 0 && snap.OIDelta4h == 0)
	if snap.OIDelta1h >= 5 && snap.OIDelta1h < 40 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_confirms_new_demand")
	} else if snap.OIDelta1h >= 2 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_moderate_inflow")
	} else if oiDeltaMissing {
		score += 4
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_delta_missing_displacement")
		sig.RiskTags = append(sig.RiskTags, "needs_oi_confirmation")
		sig.RequiredConfirms = append(sig.RequiredConfirms,
			"price_holds_vwap_or_trailing_support",
			"taker_buy_15m_stays_above_0_52",
			"oi_delta_1h_positive_or_quote_volume_expands",
		)
	} else {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_minimal_confirm")
	}

	// 5. Taker flow alignment (0-15)
	if ctx.TakerBuy15m >= 0.55 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_aggressive")
	} else if ctx.TakerBuy15m >= 0.52 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_aligned")
	} else if ctx.TakerBuy15m >= 0.50 {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_buy_neutral")
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	// Timing score: how early are we in the displacement
	sig.TimingScore = calcDisplacementTiming(ctx, sig)

	// Anti-chase guardrails
	if oiDeltaMissing {
		sig.ExecutionQuality = V7ExecNearConfirm
		sig.EntryMode = V7EntryWaitConfirm
		sig.Confidence = "C"
	}
	if chaseReason := displacementChaseCheck(ctx, snap); chaseReason != "" {
		sig.ExecutionQuality = V7ExecChaseRisk
		sig.RiskTags = append(sig.RiskTags, chaseReason)
		sig.EntryMode = V7EntryWaitConfirm
		sig.Confidence = "C"
	}

	// RSI extreme check
	if ctx.RSI1h > 82 && math.Abs(snap.FundingRate) > 0.0005 {
		sig.ExecutionQuality = V7ExecWatchOnly
		sig.RiskTags = append(sig.RiskTags, "rsi_extreme_with_crowded_funding")
		sig.EntryMode = V7EntryWaitConfirm
		sig.Confidence = "C"
	}

	// Build entry zone, invalidation, targets
	sig.EntryZone = displacementEntryZone(ctx)
	sig.Invalidation = displacementInvalidation(ctx)
	sig.Targets = displacementTargets(ctx)

	// RR check: reject only when no positive continuation target can carry
	// minimum geometry. Backend and trader layers still recalculate live RR.
	if rr, ok := displacementBestRR(sig, ctx); !ok || rr < 1.5 {
		sig.ExecutionQuality = V7ExecInvalidRR
		sig.RiskTags = appendIfMissing(sig.RiskTags, "displacement_rr_insufficient")
	} else if rr >= 1.5 && len(sig.Targets) > 1 {
		firstRR := displacementTargetRR(sig, ctx, sig.Targets[0].Price)
		if firstRR > 0 && firstRR < 1.5 {
			sig.RiskTags = appendIfMissing(sig.RiskTags, "displacement_rr_repaired")
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "displacement_extension_rr_valid")
		}
	}

	// Build price/derivatives context
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	return sig
}

// calcDisplacementTiming scores how early/fresh the displacement is.
func calcDisplacementTiming(ctx *V7SymbolContext, sig *V7SignalOutput) float64 {
	timing := 40.0
	// Fresher displacement = higher timing
	if ctx.Change1h >= 3 {
		timing += 20 // just happened
	}
	// Volume expansion adds timing confidence
	if ctx.Snapshot != nil && ctx.Snapshot.Volume24h > 0 {
		timing += 10
	}
	// Taker buy strengthening
	if ctx.TakerBuy15m >= 0.54 {
		timing += 10
	}
	return clampFloat(timing, 20, 85)
}

// displacementChoseCheck returns a chase risk tag if the move is overextended.
func displacementChaseCheck(ctx *V7SymbolContext, snap *SymbolSnapshotData) string {
	if ctx.Change1h > 8 && ctx.VWAP15m > 0 && ctx.ATR15m > 0 {
		// Price > VWAP15m + 2.5 * ATR15m = overextended
		if ctx.CurrentPrice > ctx.VWAP15m+2.5*ctx.ATR15m {
			return "displacement_chase_risk_overextended"
		}
	}
	if ctx.Change1h > 12 {
		return "displacement_chase_risk_extreme_1h_move"
	}
	return ""
}

// displacementEntryZone computes a pullback-based entry zone.
func displacementEntryZone(ctx *V7SymbolContext) V7PriceZone {
	atr := atrFallback(ctx)
	// Entry zone: current price pullback area
	lower := ctx.CurrentPrice - atr*0.8
	upper := ctx.CurrentPrice + atr*0.2
	if ctx.VWAP15m > 0 && ctx.VWAP15m > lower {
		lower = ctx.VWAP15m - atr*0.3 // VWAP as support floor
	}
	if upper < lower {
		upper = lower + atr*0.5
	}
	return V7PriceZone{Lower: lower, Upper: upper}
}

// displacementInvalidation computes the stop level.
func displacementInvalidation(ctx *V7SymbolContext) V7InvalidationRule {
	atr := atrFallback(ctx)
	execRisk := v7ExecutionStopDistance(ctx)
	if execRisk <= 0 {
		execRisk = math.Max(ctx.CurrentPrice*0.02, atr*1.2)
	}
	maxStructureRisk := math.Max(execRisk*1.25, atr*2.2)
	minRisk := math.Max(ctx.CurrentPrice*0.012, atr*0.9)
	maxRisk := math.Max(minRisk, maxStructureRisk)

	reason := "displacement_low_broken"
	stopPrice := 0.0
	if ctx.Low1h > 0 && ctx.Low1h < ctx.CurrentPrice {
		structRisk := ctx.CurrentPrice - ctx.Low1h + atr*0.15
		if structRisk <= maxRisk {
			stopPrice = ctx.Low1h - atr*0.15
		}
	}
	if stopPrice <= 0 {
		risk := clampFloat(math.Max(execRisk, minRisk), minRisk, maxRisk)
		stopPrice = ctx.CurrentPrice - risk
		reason = "displacement_execution_structure_stop"
	}
	if stopPrice <= 0 {
		stopPrice = ctx.CurrentPrice - minRisk
	}
	return V7InvalidationRule{
		Price:  stopPrice,
		Reason: reason,
	}
}

// displacementTargets computes take-profit targets.
func displacementTargets(ctx *V7SymbolContext) []V7Target {
	atr := atrFallback(ctx)
	entry := (displacementEntryZone(ctx).Lower + displacementEntryZone(ctx).Upper) / 2
	if entry <= 0 {
		entry = ctx.CurrentPrice
	}
	stop := displacementInvalidation(ctx).Price
	risk := entry - stop
	if risk <= 0 {
		risk = math.Max(ctx.CurrentPrice*0.02, atr*1.2)
	}

	addTarget := func(targets []V7Target, price float64, reason string) []V7Target {
		if price <= ctx.CurrentPrice {
			return targets
		}
		for _, existing := range targets {
			if math.Abs(existing.Price-price)/ctx.CurrentPrice < 0.001 {
				return targets
			}
		}
		return append(targets, V7Target{Price: price, Reason: reason})
	}

	targets := make([]V7Target, 0, 4)
	// T1: practical continuation target; close enough for partial take-profit,
	// but not so close that strong displacement is rejected by first-target RR.
	targets = append(targets, V7Target{
		Price:  ctx.CurrentPrice + math.Max(atr*1.8, risk*1.25),
		Reason: "displacement_practical_extension",
	})
	// T2: minimum RR continuation target from the entry-zone midpoint.
	targets = addTarget(targets, entry+risk*1.65, "displacement_rr_extension")
	// T3: 4h high if price has not already consumed it.
	if ctx.High4h > ctx.CurrentPrice {
		targets = addTarget(targets, ctx.High4h, "4h_high_retest")
	}
	extensionATR := math.Max(atr*3.2, risk*2.2)
	if ctx.ATR4h > 0 {
		extensionATR = math.Max(extensionATR, ctx.ATR4h*0.75)
	}
	targets = addTarget(targets, ctx.CurrentPrice+extensionATR, "displacement_range_expansion_run")
	sortV7TargetsLong(targets)
	return targets
}

// displacementRRValid checks if the signal can achieve RR >= 1.5.
func displacementRRValid(sig *V7SignalOutput, ctx *V7SymbolContext) bool {
	rr, ok := displacementBestRR(sig, ctx)
	return ok && rr >= 1.5
}

func displacementBestRR(sig *V7SignalOutput, ctx *V7SymbolContext) (float64, bool) {
	if sig == nil || len(sig.Targets) == 0 || sig.Invalidation.Price <= 0 {
		return 0, false
	}
	entry := (sig.EntryZone.Lower + sig.EntryZone.Upper) / 2
	if entry <= 0 {
		entry = ctx.CurrentPrice
	}
	risk := entry - sig.Invalidation.Price
	if risk <= 0 {
		return 0, false
	}
	best := 0.0
	for _, target := range sig.Targets {
		reward := target.Price - entry
		if reward <= 0 {
			continue
		}
		rr := reward / risk
		if rr > best {
			best = rr
		}
	}
	if best <= 0 {
		return 0, false
	}
	return best, true
}

func displacementTargetRR(sig *V7SignalOutput, ctx *V7SymbolContext, targetPrice float64) float64 {
	if sig == nil || ctx == nil || targetPrice <= 0 || sig.Invalidation.Price <= 0 {
		return 0
	}
	entry := (sig.EntryZone.Lower + sig.EntryZone.Upper) / 2
	if entry <= 0 {
		entry = ctx.CurrentPrice
	}
	risk := entry - sig.Invalidation.Price
	if risk <= 0 {
		return 0
	}
	reward := targetPrice - entry
	if reward <= 0 {
		return 0
	}
	return reward / risk
}

func sortV7TargetsLong(targets []V7Target) {
	for i := 1; i < len(targets); i++ {
		target := targets[i]
		j := i - 1
		for j >= 0 && targets[j].Price > target.Price {
			targets[j+1] = targets[j]
			j--
		}
		targets[j+1] = target
	}
}
