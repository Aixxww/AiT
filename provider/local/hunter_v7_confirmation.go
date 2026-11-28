package local

// V7ConfirmSeverity describes how a failed confirmation should affect routing.
type V7ConfirmSeverity string

const (
	V7ConfirmHardBlock  V7ConfirmSeverity = "hard_block"
	V7ConfirmReviewWait V7ConfirmSeverity = "review_wait"
	V7ConfirmContext    V7ConfirmSeverity = "context"
)

// V7ConfirmRiskRewardMinRR is the code of the hard RR check; its threshold
// is the resolved strategy RR floor (#108; was risk_reward_min_1_2).
const V7ConfirmRiskRewardMinRR = "risk_reward_min_rr"

// V7ConfirmBarEvidence captures the closed-bar evidence backing a trigger or
// bar-reading confirmation verdict (U5/M2b). Attached both on individual
// V7ConfirmationCheck items and on V7ConfirmationSummary.BarEvidence so passed
// and failed bar/trigger confirmations are equally auditable in prompt payloads
// and persisted signal records.
type V7ConfirmBarEvidence struct {
	Code             string  `json:"code,omitempty"`
	Passed           bool    `json:"passed"`
	Timeframe        string  `json:"timeframe,omitempty"`
	DecidingTF       string  `json:"deciding_tf,omitempty"`
	BarCloseTimeUnix int64   `json:"bar_close_time_unix,omitempty"`
	BarClose         float64 `json:"bar_close,omitempty"`
	ReferenceEdge    float64 `json:"reference_edge,omitempty"`
	ClosedOnly       bool    `json:"closed_only"`
}

// V7ConfirmationCheck is a machine-readable confirmation result for a single
// setup requirement. It turns free-form RequiredConfirms into measurable funnel
// attribution without changing each setup module's scoring contract.
type V7ConfirmationCheck struct {
	Code      string                `json:"code"`
	Passed    bool                  `json:"passed"`
	Actual    float64               `json:"actual,omitempty"`
	Threshold float64               `json:"threshold,omitempty"`
	Severity  V7ConfirmSeverity     `json:"severity"`
	Reason    string                `json:"reason,omitempty"`
	Evidence  *V7ConfirmBarEvidence `json:"evidence,omitempty"`
}

// V7ConfirmationSummary captures shared entry quality and risk geometry before
// a signal is promoted to executable/reviewable tiers.
type V7ConfirmationSummary struct {
	PassedHard        bool                   `json:"passed_hard"`
	PassedReview      bool                   `json:"passed_review"`
	MissingHard       []V7ConfirmationCheck  `json:"missing_hard,omitempty"`
	MissingReview     []V7ConfirmationCheck  `json:"missing_review,omitempty"`
	ContextChecks     []V7ConfirmationCheck  `json:"context_checks,omitempty"`
	BarEvidence       []V7ConfirmBarEvidence `json:"bar_evidence,omitempty"`
	EntryZonePosition float64                `json:"entry_zone_position,omitempty"`
	StopDistancePct   float64                `json:"stop_distance_pct,omitempty"`
	RewardPct         float64                `json:"reward_pct,omitempty"`
	RR                float64                `json:"rr,omitempty"`
	// Checklist (#133) lists the required_confirmations codes no backend
	// layer checks (V7ConfirmMachineVerifiable false): LLM checklist items,
	// never a reason to wait or block.
	Checklist []string `json:"checklist,omitempty"`
}

// UpsertBarEvidence records or replaces the closed-bar evidence for ev.Code.
func (s *V7ConfirmationSummary) UpsertBarEvidence(ev V7ConfirmBarEvidence) {
	if s == nil || ev.Code == "" {
		return
	}
	for i := range s.BarEvidence {
		if s.BarEvidence[i].Code == ev.Code {
			s.BarEvidence[i] = ev
			return
		}
	}
	s.BarEvidence = append(s.BarEvidence, ev)
}

// Clone returns a deep copy of the summary so callers can mutate checks,
// evidence, or reason flags without aliasing another stage's view.
func (s *V7ConfirmationSummary) Clone() *V7ConfirmationSummary {
	if s == nil {
		return nil
	}
	out := *s
	out.MissingHard = cloneV7ConfirmationChecks(s.MissingHard)
	out.MissingReview = cloneV7ConfirmationChecks(s.MissingReview)
	out.ContextChecks = cloneV7ConfirmationChecks(s.ContextChecks)
	if s.BarEvidence != nil {
		out.BarEvidence = append([]V7ConfirmBarEvidence(nil), s.BarEvidence...)
	}
	if s.Checklist != nil {
		out.Checklist = append([]string(nil), s.Checklist...)
	}
	return &out
}

func cloneV7ConfirmationChecks(checks []V7ConfirmationCheck) []V7ConfirmationCheck {
	if checks == nil {
		return nil
	}
	out := make([]V7ConfirmationCheck, len(checks))
	for i, c := range checks {
		if c.Evidence != nil {
			ev := *c.Evidence
			c.Evidence = &ev
		}
		out[i] = c
	}
	return out
}

// EvaluateV7Confirmations evaluates confirmations that can be measured from the
// current V7SymbolContext. Unknown confirmations remain context checks so this
// does not invent signal semantics, but catalogued execution confirmations
// should be machine-checked whenever the local execution context has the data.
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

	// #108: one configured RR floor (#77). The hard gate reads the resolved
	// strategy floor (cfg.EffectiveMinRR: V7Config.MinRiskRewardRatio, set
	// by the kernel from ResolveHunterV7; 0 -> store default). The former
	// fixed 1.5 review-wait check (the pre-#77 executable floor) is removed:
	// it kept RR between the configured floor and 1.5 out of review whatever
	// the owner configured.
	minRR := cfg.EffectiveMinRR()
	summary.addCheck(V7ConfirmationCheck{
		Code:      V7ConfirmRiskRewardMinRR,
		Passed:    summary.RR >= minRR,
		Actual:    summary.RR,
		Threshold: minRR,
		Severity:  V7ConfirmHardBlock,
		Reason:    "risk/reward below the configured RR floor",
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
		} else if !V7ConfirmMachineVerifiable(code) {
			// #133: a checklist (or unregistered) code is for the LLM only.
			summary.Checklist = appendIfMissing(summary.Checklist, code)
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
	if check.Evidence != nil {
		ev := *check.Evidence
		if ev.Code == "" {
			ev.Code = check.Code
		}
		ev.Passed = check.Passed
		check.Evidence = &ev
		s.UpsertBarEvidence(ev)
	}
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
	// #136: a code with a shared predicate is evaluated by that predicate,
	// the same one the kernel live verifier and the trader recheck call.
	// Unavailable inputs (known=false) yield the predicate's failing check.
	if check, _, ok := V7EvaluateSharedConfirmation(code, v7ScoringConfirmFacts(sig, ctx)); ok {
		return check, true
	}
	price := ctx.CurrentPrice
	switch code {
	case "15m_close_above_vwap_or_ema20_or_entry_zone_upper":
		return v7Directional15mCloseCheck(code, sig, ctx, true, sig.EntryZone.Upper), true
	case "15m_close_below_vwap_or_ema20_or_entry_zone_lower":
		return v7Directional15mCloseCheck(code, sig, ctx, false, sig.EntryZone.Lower), true
	case "15m_close_above_vwap_or_ema20":
		return v7Directional15mCloseCheck(code, sig, ctx, true, 0), true
	case "15m_close_below_vwap_or_ema20":
		return v7Directional15mCloseCheck(code, sig, ctx, false, 0), true
	case "price_holds_vwap_or_trailing_support":
		// #133: the direction's side of 15m VWAP20/EMA20 (the trailing
		// support/resistance the displacement repair names).
		return v7Directional15mCloseCheck(code, sig, ctx, sig.Direction != V7DirShort, 0), true
	case "15m_close_below_vwap":
		return v7VWAP15mCloseCheck(code, ctx, false), true
	case "15m_reclaim_vwap":
		return v7VWAP15mCloseCheck(code, ctx, true), true
	case "risk_level_not_extreme":
		passed := sig.RiskLevel != V7RiskExtreme && sig.RiskScore < 65
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    passed,
			Actual:    sig.RiskScore,
			Threshold: 65,
			Severity:  V7ConfirmReviewWait,
			Reason:    "risk level or risk score is too high for direct execution",
		}, true
	case "5m_close_above_ema20_or_entry_zone_mid":
		return v7Confirm5mCloseCheck(code, sig, ctx, true), true
	case "5m_close_below_ema20_or_entry_zone_mid":
		return v7Confirm5mCloseCheck(code, sig, ctx, false), true
	case "no_new_low_after_reclaim", "no_new_low_after_rejection", "no_new_low_after_reversal_signal", "no_new_low_after_crowding_signal":
		return v7NoNewExtremeCheck(code, ctx, true), true
	case "no_new_high_after_rejection", "no_new_high_after_reversal_signal", "no_new_high_after_crowding_signal":
		return v7NoNewExtremeCheck(code, ctx, false), true
	case "5m_or_15m_rejection_at_resistance_or_entry_zone", "5m_or_15m_rejection_from_range_top", "15m_rejection_at_resistance_or_entry_zone":
		check := v7NoNewExtremeCheck(code, ctx, false)
		if check.Passed && sig.EntryZone.Upper > 0 && price > sig.EntryZone.Upper {
			check.Passed = false
			check.Actual = price
			check.Threshold = sig.EntryZone.Upper
			check.Reason = "short rejection needs price back at or below entry-zone resistance"
		}
		return check, true
	case "5m_or_15m_rejection_at_support_or_entry_zone", "5m_or_15m_reclaim_from_range_bottom":
		check := v7NoNewExtremeCheck(code, ctx, true)
		if check.Passed && sig.EntryZone.Lower > 0 && price < sig.EntryZone.Lower {
			check.Passed = false
			check.Actual = price
			check.Threshold = sig.EntryZone.Lower
			check.Reason = "long reclaim needs price back at or above entry-zone support"
		}
		return check, true
	case "oi_1h_remains_positive_after_breakout":
		oiDelta := 0.0
		if ctx.Snapshot != nil {
			oiDelta = ctx.Snapshot.OIDelta1h
		}
		return V7ConfirmationCheck{
			Code:     code,
			Passed:   oiDelta > 0,
			Actual:   oiDelta,
			Severity: V7ConfirmReviewWait,
			Reason:   "1h OI change is not positive after the breakout",
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
	case "oi_stabilize":
		// Pullback continuation wants open interest to stop bleeding, not
		// necessarily to grow yet.
		if ctx.Snapshot == nil {
			break
		}
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    ctx.Snapshot.OIDelta1h >= -0.5,
			Actual:    ctx.Snapshot.OIDelta1h,
			Threshold: -0.5,
			Severity:  V7ConfirmReviewWait,
			Reason:    "1h OI must stop declining",
		}, true
	case "lsr_turning_up":
		if ctx.Snapshot == nil || ctx.Snapshot.LSR <= 0 || ctx.Snapshot.LSRPrev <= 0 {
			break
		}
		return V7ConfirmationCheck{
			Code:      code,
			Passed:    ctx.Snapshot.LSR > ctx.Snapshot.LSRPrev,
			Actual:    ctx.Snapshot.LSR,
			Threshold: ctx.Snapshot.LSRPrev,
			Severity:  V7ConfirmReviewWait,
			Reason:    "long/short ratio must inflect upward",
		}, true
	}
	return V7ConfirmationCheck{}, false
}

func v7Directional15mCloseCheck(code string, sig *V7SignalOutput, ctx *V7SymbolContext, long bool, zoneThreshold float64) V7ConfirmationCheck {
	check, _ := v7Directional15mClosePredicate(code, v7ScoringConfirmFacts(sig, ctx), long, zoneThreshold)
	return check
}

// v7Confirm5mCloseCheck evaluates 5m_close_above/below_ema20_or_entry_zone_mid
// strictly on the latest closed 5m bar (U5/M2b).
func v7Confirm5mCloseCheck(code string, sig *V7SignalOutput, ctx *V7SymbolContext, above bool) V7ConfirmationCheck {
	mid := v7EntryZoneMid(sig)
	check := V7ConfirmationCheck{
		Code:      code,
		Threshold: mid,
		Severity:  V7ConfirmReviewWait,
		Reason:    "closed 5m bar is unavailable",
		Evidence: &V7ConfirmBarEvidence{
			Code:          code,
			Passed:        false,
			Timeframe:     "5m",
			DecidingTF:    "none",
			ReferenceEdge: mid,
			ClosedOnly:    true,
		},
	}
	tf, ok := v7ExecutionTimeframe(ctx, "5m")
	if !ok || tf.LastClose <= 0 {
		return check
	}
	check.Actual = tf.LastClose
	ema20 := v7SummaryEMA20(tf)
	refEdge := mid
	if above {
		passed := false
		if tf.HasEMA20 && tf.CloseVsEMA20Pct >= 0 {
			passed = true
			if ema20 > 0 {
				refEdge = ema20
			}
		}
		if mid > 0 && tf.LastClose >= mid {
			passed = true
			if refEdge <= 0 {
				refEdge = mid
			}
		}
		check.Passed = passed
		check.Reason = "closed 5m bar is not above EMA20 or entry-zone midpoint"
	} else {
		passed := false
		if tf.HasEMA20 && tf.CloseVsEMA20Pct <= 0 {
			passed = true
			if ema20 > 0 {
				refEdge = ema20
			}
		}
		if mid > 0 && tf.LastClose <= mid {
			passed = true
			if refEdge <= 0 {
				refEdge = mid
			}
		}
		check.Passed = passed
		check.Reason = "closed 5m bar is not below EMA20 or entry-zone midpoint"
	}
	if refEdge <= 0 {
		refEdge = ema20
	}
	check.Threshold = refEdge
	check.Evidence = &V7ConfirmBarEvidence{
		Code:             code,
		Passed:           check.Passed,
		Timeframe:        "5m",
		DecidingTF:       "5m",
		BarCloseTimeUnix: tf.LastCloseTimeUnix,
		BarClose:         tf.LastClose,
		ReferenceEdge:    refEdge,
		ClosedOnly:       true,
	}
	return check
}

// v7VWAP15mCloseCheck: the latest closed 15m close above (reclaim) or below
// VWAP20 (#133, U5/M2b).
func v7VWAP15mCloseCheck(code string, ctx *V7SymbolContext, above bool) V7ConfirmationCheck {
	check := V7ConfirmationCheck{
		Code:     code,
		Severity: V7ConfirmReviewWait,
		Reason:   "15m VWAP20 is unavailable",
		Evidence: &V7ConfirmBarEvidence{
			Code:       code,
			Passed:     false,
			Timeframe:  "15m",
			DecidingTF: "none",
			ClosedOnly: true,
		},
	}
	tf, ok := v7ExecutionTimeframe(ctx, "15m")
	if !ok || !tf.HasVWAP20 || tf.LastClose <= 0 {
		return check
	}
	check.Actual, check.Threshold = tf.LastClose, tf.VWAP20
	if above {
		check.Passed = tf.CloseVsVWAP20Pct >= 0
		check.Reason = "15m close has not reclaimed VWAP20"
	} else {
		check.Passed = tf.CloseVsVWAP20Pct <= 0
		check.Reason = "15m close is not below VWAP20"
	}
	check.Evidence = &V7ConfirmBarEvidence{
		Code:             code,
		Passed:           check.Passed,
		Timeframe:        "15m",
		DecidingTF:       "15m",
		BarCloseTimeUnix: tf.LastCloseTimeUnix,
		BarClose:         tf.LastClose,
		ReferenceEdge:    tf.VWAP20,
		ClosedOnly:       true,
	}
	return check
}

func v7NoNewExtremeCheck(code string, ctx *V7SymbolContext, noNewLow bool) V7ConfirmationCheck {
	check := V7ConfirmationCheck{
		Code:     code,
		Severity: V7ConfirmReviewWait,
		Reason:   "5m/15m recent-high-low execution context is unavailable",
	}
	if tf, ok := v7ExecutionTimeframe(ctx, "15m"); ok {
		if noNewLow {
			check.Passed = tf.NoNewLow
			check.Actual = tf.RecentLow3
			check.Reason = "15m/5m made a new low after reclaim"
		} else {
			check.Passed = tf.NoNewHigh
			check.Actual = tf.RecentHigh3
			check.Reason = "15m/5m made a new high after rejection"
		}
	}
	if tf, ok := v7ExecutionTimeframe(ctx, "5m"); ok {
		if noNewLow {
			check.Passed = check.Passed || tf.NoNewLow
			if check.Actual == 0 {
				check.Actual = tf.RecentLow3
			}
		} else {
			check.Passed = check.Passed || tf.NoNewHigh
			if check.Actual == 0 {
				check.Actual = tf.RecentHigh3
			}
		}
	}
	return check
}

func v7ExecutionTimeframe(ctx *V7SymbolContext, timeframe string) (V7ExecutionTimeframeSummary, bool) {
	if ctx == nil {
		return V7ExecutionTimeframeSummary{}, false
	}
	// U5/M2b: prefer strictly closed-bar confirmation summaries when built from
	// raw klines; a non-nil empty map means zero closed bars exist and must NOT
	// fall back to forming bars in ExecutionContext.
	if ctx.ClosedConfirmTimeframes != nil {
		tf, ok := ctx.ClosedConfirmTimeframes[timeframe]
		return tf, ok
	}
	if ctx.ExecutionContext == nil || len(ctx.ExecutionContext.Timeframes) == 0 {
		return V7ExecutionTimeframeSummary{}, false
	}
	tf, ok := ctx.ExecutionContext.Timeframes[timeframe]
	return tf, ok
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
