package local

// V7ConfirmSeverity describes how a failed confirmation should affect routing.
type V7ConfirmSeverity string

const (
	V7ConfirmHardBlock  V7ConfirmSeverity = "hard_block"
	V7ConfirmReviewWait V7ConfirmSeverity = "review_wait"
	V7ConfirmContext    V7ConfirmSeverity = "context"
)

// V7ConfirmationCheck is a machine-readable confirmation result for a single
// setup requirement. It turns free-form RequiredConfirms into measurable funnel
// attribution without changing each setup module's scoring contract.
type V7ConfirmationCheck struct {
	Code      string            `json:"code"`
	Passed    bool              `json:"passed"`
	Actual    float64           `json:"actual,omitempty"`
	Threshold float64           `json:"threshold,omitempty"`
	Severity  V7ConfirmSeverity `json:"severity"`
	Reason    string            `json:"reason,omitempty"`
}

// V7ConfirmationSummary captures shared entry quality and risk geometry before
// a signal is promoted to executable/reviewable tiers.
type V7ConfirmationSummary struct {
	PassedHard        bool                  `json:"passed_hard"`
	PassedReview      bool                  `json:"passed_review"`
	MissingHard       []V7ConfirmationCheck `json:"missing_hard,omitempty"`
	MissingReview     []V7ConfirmationCheck `json:"missing_review,omitempty"`
	ContextChecks     []V7ConfirmationCheck `json:"context_checks,omitempty"`
	EntryZonePosition float64               `json:"entry_zone_position,omitempty"`
	StopDistancePct   float64               `json:"stop_distance_pct,omitempty"`
	RewardPct         float64               `json:"reward_pct,omitempty"`
	RR                float64               `json:"rr,omitempty"`
}

// EvaluateV7Confirmations evaluates confirmations that can be measured from the
// current V7SymbolContext. Unknown or candle-specific confirmations remain
// context checks so this does not over-tighten the strategy.
func EvaluateV7Confirmations(sig *V7SignalOutput, ctx *V7SymbolContext, cfg V7Config) V7ConfirmationSummary {
	summary := V7ConfirmationSummary{
		PassedHard:   true,
		PassedReview: true,
	}
	if sig == nil || ctx == nil {
		summary.PassedHard = false
		summary.PassedReview = false
		return summary
	}

	price := ctx.CurrentPrice
	if pos, ok := v7EntryZonePositionPct(sig, price); ok {
		summary.EntryZonePosition = pos
	}
	summary.StopDistancePct = v7SignalStopDistancePct(sig, price)
	summary.RewardPct = v7SignalBestRewardPct(sig, price)
	if rr, ok := v7SignalRiskReward(sig, price); ok {
		summary.RR = rr
	}

	summary.addCheck(V7ConfirmationCheck{
		Code:      "risk_reward_min_1_2",
		Passed:    summary.RR >= 1.2,
		Actual:    summary.RR,
		Threshold: 1.2,
		Severity:  V7ConfirmHardBlock,
		Reason:    "risk/reward below executable geometry floor",
	})
	summary.addCheck(V7ConfirmationCheck{
		Code:      "risk_reward_min_1_5",
		Passed:    summary.RR >= 1.5,
		Actual:    summary.RR,
		Threshold: 1.5,
		Severity:  V7ConfirmReviewWait,
		Reason:    "risk/reward below preferred review floor",
	})

	thresholds := cfg.GetSetupThresholds(sig.SetupType)
	if summary.EntryZonePosition > 0 || (sig.EntryZone.Lower > 0 && sig.EntryZone.Upper > sig.EntryZone.Lower) {
		if sig.Direction == V7DirLong && thresholds.MaxZonePosLong < 100 {
			summary.addCheck(V7ConfirmationCheck{
				Code:      "long_entry_zone_not_extended",
				Passed:    summary.EntryZonePosition <= float64(thresholds.MaxZonePosLong),
				Actual:    summary.EntryZonePosition,
				Threshold: float64(thresholds.MaxZonePosLong),
				Severity:  V7ConfirmReviewWait,
				Reason:    "long price is above allowed entry-zone position",
			})
		}
		if sig.Direction == V7DirShort && thresholds.MinZonePosShort > 0 {
			summary.addCheck(V7ConfirmationCheck{
				Code:      "short_entry_zone_retest",
				Passed:    summary.EntryZonePosition >= float64(thresholds.MinZonePosShort),
				Actual:    summary.EntryZonePosition,
				Threshold: float64(thresholds.MinZonePosShort),
				Severity:  V7ConfirmReviewWait,
				Reason:    "short price has not retested upper entry-zone area",
			})
		}
	}

	for _, code := range sig.RequiredConfirms {
		if check, ok := evaluateV7KnownConfirmation(code, sig, ctx); ok {
			summary.addCheck(check)
		} else {
			summary.addCheck(V7ConfirmationCheck{
				Code:     code,
				Passed:   false,
				Severity: V7ConfirmContext,
				Reason:   "confirmation requires candle pattern or unavailable field; leave to LLM/live market review",
			})
		}
	}

	return summary
}

func (s *V7ConfirmationSummary) addCheck(check V7ConfirmationCheck) {
	if check.Passed {
		if check.Severity == V7ConfirmContext {
			s.ContextChecks = append(s.ContextChecks, check)
		}
		return
	}
	switch check.Severity {
	case V7ConfirmHardBlock:
		s.PassedHard = false
		s.PassedReview = false
		s.MissingHard = append(s.MissingHard, check)
	case V7ConfirmReviewWait:
		s.PassedReview = false
		s.MissingReview = append(s.MissingReview, check)
	default:
		s.ContextChecks = append(s.ContextChecks, check)
	}
}

func evaluateV7KnownConfirmation(code string, sig *V7SignalOutput, ctx *V7SymbolContext) (V7ConfirmationCheck, bool) {
	price := ctx.CurrentPrice
	switch code {
	case "live_price_in_entry_zone":
		passed := price >= sig.EntryZone.Lower && price <= sig.EntryZone.Upper
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    passed,
			Actual:    price,
			Threshold: sig.EntryZone.Upper,
			Severity:  V7ConfirmReviewWait,
			Reason:    "live price must be inside entry zone",
		}, true
	case "taker_buy_15m_gt_0_52", "taker_buy_15m_stays_above_0_52":
		return v7TakerConfirmationCheck(code, ctx.TakerBuy15m, 0.52, true), true
	case "taker_buy_15m_lt_0_48":
		return v7TakerConfirmationCheck(code, ctx.TakerBuy15m, 0.48, false), true
	case "taker_buy_15m_lt_0_45":
		return v7TakerConfirmationCheck(code, ctx.TakerBuy15m, 0.45, false), true
	case "5m_close_above_ema20_or_entry_zone_mid":
		mid := v7EntryZoneMid(sig)
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    mid > 0 && price >= mid,
			Actual:    price,
			Threshold: mid,
			Severity:  V7ConfirmReviewWait,
			Reason:    "live price should be above entry-zone midpoint when 5m EMA is unavailable",
		}, true
	case "5m_close_below_ema20_or_entry_zone_mid":
		mid := v7EntryZoneMid(sig)
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    mid > 0 && price <= mid,
			Actual:    price,
			Threshold: mid,
			Severity:  V7ConfirmReviewWait,
			Reason:    "live price should be below entry-zone midpoint when 5m EMA is unavailable",
		}, true
	case "5m_or_15m_close_above_entry_zone":
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    sig.EntryZone.Upper > 0 && price >= sig.EntryZone.Upper,
			Actual:    price,
			Threshold: sig.EntryZone.Upper,
			Severity:  V7ConfirmReviewWait,
			Reason:    "breakout confirmation requires price above entry-zone upper bound",
		}, true
	case "5m_or_15m_close_above_trigger", "5m_or_15m_close_through_breakout_level":
		trigger := sig.EntryZone.Upper
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    trigger > 0 && price >= trigger,
			Actual:    price,
			Threshold: trigger,
			Severity:  V7ConfirmReviewWait,
			Reason:    "long trigger has not cleared entry-zone upper bound",
		}, true
	case "5m_or_15m_close_below_trigger":
		trigger := sig.EntryZone.Lower
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    trigger > 0 && price <= trigger,
			Actual:    price,
			Threshold: trigger,
			Severity:  V7ConfirmReviewWait,
			Reason:    "short trigger has not cleared entry-zone lower bound",
		}, true
	case "oi_continues_inflow", "oi_delta_1h_positive_or_quote_volume_expands":
		oiDelta := 0.0
		if ctx.Snapshot != nil {
			oiDelta = ctx.Snapshot.OIDelta1h
		}
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    oiDelta > 0 || ctx.VolumeBurst15m >= 1.2,
			Actual:    oiDelta,
			Threshold: 0,
			Severity:  V7ConfirmReviewWait,
			Reason:    "OI or short-term volume expansion is not yet confirmed",
		}, true
	case "bb_width_expansion_starts":
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    ctx.BBWidthPercentile >= 1.05 || ctx.BBWidth5m > 0,
			Actual:    ctx.BBWidthPercentile,
			Threshold: 1.05,
			Severity:  V7ConfirmContext,
			Reason:    "Bollinger expansion is weak or unavailable",
		}, true
	case "momentum_not_exhausted":
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    ctx.RSI1h < 78 && ctx.Change1h <= 4,
			Actual:    ctx.RSI1h,
			Threshold: 78,
			Severity:  V7ConfirmReviewWait,
			Reason:    "momentum is overheated or late",
		}, true
	case "taker_flow_not_flipping_against_direction":
		if sig.Direction == V7DirShort {
			return v7TakerConfirmationCheck(code, ctx.TakerBuy15m, 0.50, false), true
		}
		return v7TakerConfirmationCheck(code, ctx.TakerBuy15m, 0.50, true), true
	}
	return V7ConfirmationCheck{}, false
}

func v7TakerConfirmationCheck(code string, actual, threshold float64, above bool) V7ConfirmationCheck {
	passed := actual >= threshold
	reason := "15m taker buy is below required long threshold"
	if !above {
		passed = actual <= threshold
		reason = "15m taker buy is above required short threshold"
	}
	return V7ConfirmationCheck{
		Code:      code,
		Passed:    passed,
		Actual:    actual,
		Threshold: threshold,
		Severity:  V7ConfirmReviewWait,
		Reason:    reason,
	}
}

func v7EntryZoneMid(sig *V7SignalOutput) float64 {
	if sig == nil || sig.EntryZone.Lower <= 0 || sig.EntryZone.Upper <= sig.EntryZone.Lower {
		return 0
	}
	return (sig.EntryZone.Lower + sig.EntryZone.Upper) / 2
}

func v7SignalStopDistancePct(sig *V7SignalOutput, price float64) float64 {
	if sig == nil || price <= 0 || sig.Invalidation.Price <= 0 {
		return 0
	}
	risk := 0.0
	if sig.Direction == V7DirShort {
		risk = sig.Invalidation.Price - price
	} else {
		risk = price - sig.Invalidation.Price
	}
	if risk <= 0 {
		return 0
	}
	return risk / price * 100
}

func v7SignalBestRewardPct(sig *V7SignalOutput, price float64) float64 {
	if sig == nil || price <= 0 {
		return 0
	}
	best := 0.0
	for _, target := range sig.Targets {
		reward := v7TargetReward(sig.Direction, price, target.Price)
		if reward > best {
			best = reward
		}
	}
	if best <= 0 {
		return 0
	}
	return best / price * 100
}
