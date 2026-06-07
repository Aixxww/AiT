package local

import (
	"sort"
)

// ============================================================================
// Hunter v7 — Signal Router
// ============================================================================
// The router is the core dispatcher: it runs all signal modules against all
// symbols, applies regime weights, computes final scores, filters, and outputs
// a sorted list of actionable signals.

// V7Router dispatches symbols through all registered signal modules.
type V7Router struct {
	modules []V7SignalModule
}

// NewV7Router creates a router with all 10 signal modules registered.
func NewV7Router() *V7Router {
	r := &V7Router{}
	r.modules = []V7SignalModule{
		&pullbackLongModule{},
		&shortSqueezeLongModule{},
		&trendBreakoutLongModule{},
		&leaderMomentumLongModule{},
		&panicReversalLongModule{},
		&accumulationBreakoutLongModule{},
		&distributionShortModule{},
		&squeezeShortModule{},
		&rangeReversionModule{},
		&fundingReversalModule{},
	}
	return r
}

// Route runs all modules against the universe and returns sorted signals.
func (r *V7Router) Route(universe []V7SymbolContext, regime V7MarketRegime, cfg V7Config) []V7SignalOutput {
	var allSignals []V7SignalOutput

	for i := range universe {
		ctx := &universe[i]
		for _, mod := range r.modules {
			// Regime weight gate: skip modules with very low weight
			weight := regimeModuleWeight(regime, mod.SetupType())
			if weight < 0.2 {
				continue
			}

			// Fast pre-filter
			if !mod.Match(ctx, regime) {
				continue
			}

			// Full scoring
			sig := mod.Score(ctx, regime)
			if sig == nil {
				continue
			}
			if len(sig.RequiredConfirms) == 0 {
				sig.RequiredConfirms = defaultV7Confirmations(sig)
			}

			// Apply regime weight to setup score
			sig.SetupScore *= weight
			sig.SetupScore = clampFloat(sig.SetupScore, 0, 100)
			sig.RegimeFitScore = weight * 67 // normalize to 0-100 (1.5 * 67 ≈ 100)

			// Compute risk score
			riskScore := AssessV7Risk(sig, ctx)
			sig.RiskScore = riskScore
			sig.RiskLevel = ClassifyV7RiskLevel(riskScore)

			// Compute liquidity score
			sig.LiquidityScore = AssessLiquidityScore(ctx)

			// Compute AI Priority (composite ranking score)
			sig.AIPriority = CalcAIPriority(sig, cfg)

			// Filter: risk extreme
			if riskScore >= 90 {
				sig.Status = V7StatusFiltered
				sig.RiskTags = append(sig.RiskTags, "risk_filtered")
			}

			// Filter: liquidity too low
			if sig.LiquidityScore < 30 {
				sig.Status = V7StatusFiltered
				sig.RiskTags = append(sig.RiskTags, "liquidity_filtered")
			}

			allSignals = append(allSignals, *sig)
		}
	}

	// Resolve conflicts (same symbol, opposite directions)
	allSignals = ResolveV7Conflicts(allSignals)

	return filterV7SignalsForLLM(allSignals, cfg)
}

// CalcAIPriority computes the composite AI priority score.
// Formula: setup×0.35 + timing×0.20 + regime_fit×0.20 + liquidity×0.15 - risk×0.10
func CalcAIPriority(sig *V7SignalOutput, cfg V7Config) float64 {
	if cfg.Aggressive {
		// Aggressive: more weight on setup and timing
		return sig.SetupScore*0.40 +
			sig.TimingScore*0.25 +
			sig.RegimeFitScore*0.15 +
			sig.LiquidityScore*0.10 -
			sig.RiskScore*0.10
	}
	// Balanced (default)
	return sig.SetupScore*0.35 +
		sig.TimingScore*0.20 +
		sig.RegimeFitScore*0.20 +
		sig.LiquidityScore*0.15 -
		sig.RiskScore*0.10
}

func filterV7SignalsForLLM(signals []V7SignalOutput, cfg V7Config) []V7SignalOutput {
	minPriority := cfg.MinAIPriority
	if minPriority <= 0 {
		minPriority = 55
	}
	fallbackMinPriority := cfg.FallbackMinAIPriority
	if fallbackMinPriority <= 0 {
		fallbackMinPriority = minPriority - 10
	}
	if fallbackMinPriority < 0 {
		fallbackMinPriority = 0
	}
	minOutput := cfg.MinOutput
	if minOutput <= 0 {
		minOutput = 3
	}
	maxOutput := cfg.MaxOutput
	if maxOutput > 0 && minOutput > maxOutput {
		minOutput = maxOutput
	}

	eligible := make([]V7SignalOutput, 0, len(signals))
	for _, sig := range signals {
		if sig.Status == V7StatusFiltered {
			continue
		}
		eligible = append(eligible, sig)
	}
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].AIPriority > eligible[j].AIPriority
	})

	filtered := make([]V7SignalOutput, 0, len(eligible))
	used := make([]bool, len(eligible))
	for i, sig := range eligible {
		if sig.AIPriority >= minPriority || sig.Status == V7StatusConflictWatch {
			filtered = append(filtered, sig)
			used[i] = true
		}
	}

	if len(filtered) < minOutput {
		for i, sig := range eligible {
			if len(filtered) >= minOutput {
				break
			}
			if used[i] || sig.AIPriority < fallbackMinPriority {
				continue
			}
			if sig.Status == V7StatusCandidate {
				sig.Status = V7StatusWaitConfirm
			}
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "candidate_floor_context")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "context_only_low_priority")
			filtered = append(filtered, sig)
			used[i] = true
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].AIPriority > filtered[j].AIPriority
	})
	if maxOutput > 0 && len(filtered) > maxOutput {
		filtered = filtered[:maxOutput]
	}
	return filtered
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func defaultV7Confirmations(sig *V7SignalOutput) []string {
	if sig == nil {
		return nil
	}

	switch sig.EntryMode {
	case V7EntryImmediate:
		return nil
	case V7EntryBreakout:
		return []string{
			"15m_close_through_breakout_level",
			"oi_or_volume_expands_with_price",
			"no_failed_breakout_back_inside_range",
		}
	case V7EntryFastConfirm:
		if sig.Direction == V7DirShort {
			return []string{
				"5m_or_15m_close_below_trigger",
				"taker_buy_15m_lt_0_48",
				"no_immediate_reclaim_of_trigger_level",
			}
		}
		return []string{
			"5m_or_15m_close_above_trigger",
			"taker_buy_15m_gt_0_52",
			"no_immediate_loss_of_trigger_level",
		}
	case V7EntryWaitReclaim:
		if sig.Direction == V7DirShort {
			return []string{
				"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
				"taker_buy_15m_lt_0_48",
				"no_new_high_after_rejection",
			}
		}
		return []string{
			"15m_close_above_vwap_or_ema20_or_entry_zone_upper",
			"taker_buy_15m_gt_0_52",
			"no_new_low_after_reclaim",
		}
	case V7EntryWaitBreakout:
		return []string{
			"15m_close_above_entry_zone",
			"oi_continues_inflow",
			"bb_width_expansion_starts",
		}
	case V7EntryWaitReject:
		if sig.Direction == V7DirShort {
			return []string{
				"15m_rejection_at_resistance_or_entry_zone",
				"taker_buy_15m_lt_0_48",
				"no_new_high_after_rejection",
			}
		}
		return []string{
			"15m_rejection_at_support_or_entry_zone",
			"taker_buy_15m_gt_0_52",
			"no_new_low_after_rejection",
		}
	case V7EntryRangeEdge:
		if sig.Direction == V7DirShort {
			return []string{
				"enter_only_near_range_top",
				"15m_rejection_from_range_top",
				"stop_above_range_high",
			}
		}
		return []string{
			"enter_only_near_range_bottom",
			"15m_reclaim_from_range_bottom",
			"stop_below_range_low",
		}
	case V7EntryWaitPriceReversal:
		if sig.Direction == V7DirShort {
			return []string{
				"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
				"taker_buy_15m_lt_0_45",
				"no_new_high_after_reversal_signal",
			}
		}
		return []string{
			"15m_close_above_vwap_or_ema20_or_entry_zone_upper",
			"taker_buy_15m_gt_0_52",
			"no_new_low_after_reversal_signal",
		}
	case V7EntryMomentumTrailing:
		return []string{
			"price_holds_trailing_support",
			"momentum_not_exhausted",
			"taker_flow_not_flipping_against_direction",
		}
	default:
		if sig.Direction == V7DirShort {
			return []string{"directional_15m_close_short", "taker_flow_confirms_short", "risk_level_not_extreme"}
		}
		return []string{"directional_15m_close_long", "taker_flow_confirms_long", "risk_level_not_extreme"}
	}
}
