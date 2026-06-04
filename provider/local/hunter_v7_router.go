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

	// Filter by minimum AI priority
	var filtered []V7SignalOutput
	for _, sig := range allSignals {
		if sig.Status == V7StatusFiltered {
			continue
		}
		if sig.AIPriority >= cfg.MinAIPriority || sig.Status == V7StatusConflictWatch {
			filtered = append(filtered, sig)
		}
	}

	// Sort by AI Priority descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].AIPriority > filtered[j].AIPriority
	})

	// Limit output
	if cfg.MaxOutput > 0 && len(filtered) > cfg.MaxOutput {
		filtered = filtered[:cfg.MaxOutput]
	}

	return filtered
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
				"15m_reject_vwap_or_entry_zone",
				"taker_buy_15m_lt_0_48",
				"no_new_high_after_rejection",
			}
		}
		return []string{
			"15m_reclaim_vwap_or_entry_zone",
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
				"15m_close_below_vwap_or_entry_zone",
				"taker_buy_15m_lt_0_45",
				"no_new_high_after_reversal_signal",
			}
		}
		return []string{
			"15m_reclaim_vwap_or_entry_zone",
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
