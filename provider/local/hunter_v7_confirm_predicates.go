package local

// ============================================================================
// Hunter v7 — shared confirmation predicates (#136, naming F-05)
// ============================================================================
// A required-confirmation code used to be evaluated twice with different
// formulas: once at scoring time (evaluateV7KnownConfirmation, against the
// V7SymbolContext) and once at prompt time (the kernel live verifier, against
// the prompt cycle's market data). The two drifted: whale_flow_reversal's
// 0.56 taker floor, the volume leg of the OI-or-volume code, and the VWAP /
// zone-mid legs of the directional 15m close existed only on one side.
//
// Each code below now has exactly one setup-aware predicate over a neutral
// fact set (V7ConfirmFacts). The scoring evaluator builds the facts from its
// symbol context, the kernel live verifier and the trader's decision-time
// recheck build them from live data, and all three call the same function.
// Membership in this table is what "live-verifiable" means; the vocab flags
// are tested against it (TestV7ConfirmLiveReviewableMatchesVerifiers).

// V7ConfirmFacts is everything a shared confirmation predicate may read.
// Zero values mean "unavailable": a predicate whose inputs are unavailable
// reports known=false rather than guessing.
type V7ConfirmFacts struct {
	Direction V7Direction
	SetupType V7SetupType
	Price     float64
	EntryZone V7PriceZone
	// TakerBuy15m is the latest 15m taker-buy ratio (0-1); <= 0 = unavailable.
	TakerBuy15m float64
	// OIDelta1h is the 1h OI change in percent; HasOIDelta1h says it is known.
	OIDelta1h    float64
	HasOIDelta1h bool
	// VolumeBurst15m is the last 15m bar's volume over the prior 8 bars'
	// average (V7VolumeBurst); <= 0 = unavailable.
	VolumeBurst15m float64
	// Timeframes holds the 15m / 5m close facts (LastClose, EMA20, VWAP20).
	Timeframes map[string]V7ExecutionTimeframeSummary
}

// Shared thresholds. One name per number so scoring and live cannot drift.
const (
	v7ConfirmTakerLong           = 0.52
	v7ConfirmTakerLongWhaleFlow  = 0.56 // matches the trader whale_flow guard
	v7ConfirmTakerShort          = 0.48
	v7ConfirmTakerShortStrong    = 0.45
	v7ConfirmTakerNotFlipping    = 0.50
	v7ConfirmVolumeBurstExpands  = 1.2
	v7ConfirmVolumeBurstLookback = 8
)

// V7ConfirmTakerLongThreshold is the taker_flow_confirms_long floor for a
// setup: whale_flow_reversal demands 0.56, every other setup 0.52.
func V7ConfirmTakerLongThreshold(setup V7SetupType) float64 {
	if setup == V7SetupWhaleFlow {
		return v7ConfirmTakerLongWhaleFlow
	}
	return v7ConfirmTakerLong
}

type v7ConfirmPredicate func(code string, f V7ConfirmFacts) (check V7ConfirmationCheck, known bool)

// v7SharedConfirmPredicates is the single definition of every code that is
// verified both at scoring time and against live data.
var v7SharedConfirmPredicates = map[string]v7ConfirmPredicate{
	"live_price_in_entry_zone": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		z := f.EntryZone
		check := V7ConfirmationCheck{Code: code, Actual: f.Price, Threshold: z.Upper, Severity: V7ConfirmReviewWait, Reason: "live price must be inside entry zone"}
		if f.Price <= 0 || z.Lower <= 0 || z.Upper <= z.Lower {
			return check, false
		}
		check.Passed = f.Price >= z.Lower && f.Price <= z.Upper
		return check, true
	},

	"taker_buy_15m_gt_0_52": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7TakerPredicate(code, f, v7ConfirmTakerLong, true)
	},
	"taker_buy_15m_lt_0_48": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7TakerPredicate(code, f, v7ConfirmTakerShort, false)
	},
	"taker_buy_15m_lt_0_45": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7TakerPredicate(code, f, v7ConfirmTakerShortStrong, false)
	},
	"taker_flow_confirms_long": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7TakerPredicate(code, f, V7ConfirmTakerLongThreshold(f.SetupType), true)
	},
	"taker_flow_confirms_short": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7TakerPredicate(code, f, v7ConfirmTakerShort, false)
	},
	"taker_flow_not_flipping_against_direction": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7TakerPredicate(code, f, v7ConfirmTakerNotFlipping, f.Direction != V7DirShort)
	},

	"5m_price_holds_ema20_or_trailing_support": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7EMA20ClosePredicate(code, f, "5m", f.Direction != V7DirShort)
	},
	"directional_15m_close_long": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7Directional15mClosePredicate(code, f, true, v7ZoneMid(f.EntryZone))
	},
	"directional_15m_close_short": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7Directional15mClosePredicate(code, f, false, v7ZoneMid(f.EntryZone))
	},

	"oi_delta_1h_positive_or_quote_volume_expands": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7OIOrVolumePredicate(code, f)
	},

	// Trigger codes (U5/M2b): evaluated strictly on the latest CLOSED 5m or 15m
	// bar's close against the entry-zone edge. Spot/live price NEVER satisfies
	// or fails a trigger code.
	"5m_or_15m_close_above_trigger": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7ClosedBarThroughEdgePredicate(code, f, f.EntryZone.Upper, true, "long trigger has not cleared entry-zone upper bound on closed 5m/15m bar")
	},
	"5m_or_15m_close_through_breakout_level": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		if f.Direction == V7DirShort {
			return v7ClosedBarThroughEdgePredicate(code, f, f.EntryZone.Lower, false, "short trigger has not cleared entry-zone lower bound on closed 5m/15m bar")
		}
		return v7ClosedBarThroughEdgePredicate(code, f, f.EntryZone.Upper, true, "long trigger has not cleared entry-zone upper bound on closed 5m/15m bar")
	},
	"5m_or_15m_close_below_trigger": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7ClosedBarThroughEdgePredicate(code, f, f.EntryZone.Lower, false, "short trigger has not cleared entry-zone lower bound on closed 5m/15m bar")
	},
}

// v7ConfirmPredicateAliases are codes that share another code's predicate
// verbatim at scoring time. They are not live-verifiable codes of their own.
var v7ConfirmPredicateAliases = map[string]string{
	"taker_buy_15m_stays_above_0_52":  "taker_buy_15m_gt_0_52",
	"oi_continues_inflow":             "oi_delta_1h_positive_or_quote_volume_expands",
	"oi_or_volume_expands_with_price": "oi_delta_1h_positive_or_quote_volume_expands",
	"5m_or_15m_close_above_entry_zone": "5m_or_15m_close_above_trigger",
}

// V7EvaluateSharedConfirmation evaluates a live-verifiable code (or one of
// its scoring aliases) on facts. ok is false for a code without a shared
// predicate; known is false when the facts lack the predicate's inputs.
func V7EvaluateSharedConfirmation(code string, f V7ConfirmFacts) (check V7ConfirmationCheck, known, ok bool) {
	pred, found := v7SharedConfirmPredicates[code]
	if !found {
		if target, alias := v7ConfirmPredicateAliases[code]; alias {
			pred, found = v7SharedConfirmPredicates[target]
		}
	}
	if !found {
		return V7ConfirmationCheck{}, false, false
	}
	check, known = pred(code, f)
	check.Code = code
	if check.Evidence != nil {
		check.Evidence.Code = code
		check.Evidence.Passed = check.Passed
	}
	return check, known, true
}

// V7ConfirmLiveVerifiable reports whether a code has a shared predicate that
// can be evaluated against live data (aliases excluded).
func V7ConfirmLiveVerifiable(code string) bool {
	_, ok := v7SharedConfirmPredicates[code]
	return ok
}

// V7ConfirmLiveVerifiableCodes lists the live-verifiable codes (unordered).
func V7ConfirmLiveVerifiableCodes() []string {
	out := make([]string, 0, len(v7SharedConfirmPredicates))
	for code := range v7SharedConfirmPredicates {
		out = append(out, code)
	}
	return out
}

// V7ConfirmBarTimeframes returns the closed-bar timeframe(s) a shared
// bar/trigger confirmation code depends on ("5m", "15m", or both). Non-bar
// codes (spot price, taker flow, OI/volume) return nil.
func V7ConfirmBarTimeframes(code string) []string {
	if target, alias := v7ConfirmPredicateAliases[code]; alias {
		code = target
	}
	switch code {
	case "5m_price_holds_ema20_or_trailing_support":
		return []string{"5m"}
	case "directional_15m_close_long", "directional_15m_close_short":
		return []string{"15m"}
	case "5m_or_15m_close_above_trigger",
		"5m_or_15m_close_through_breakout_level",
		"5m_or_15m_close_below_trigger":
		return []string{"5m", "15m"}
	default:
		return nil
	}
}

func v7TakerPredicate(code string, f V7ConfirmFacts, threshold float64, above bool) (V7ConfirmationCheck, bool) {
	check := V7ConfirmationCheck{Code: code, Actual: f.TakerBuy15m, Threshold: threshold, Severity: V7ConfirmReviewWait}
	if above {
		check.Reason = "15m taker buy is below required long threshold"
	} else {
		check.Reason = "15m taker buy is above required short threshold"
	}
	if f.TakerBuy15m <= 0 {
		check.Reason = "15m taker buy is unavailable"
		return check, false
	}
	if above {
		check.Passed = f.TakerBuy15m >= threshold
	} else {
		check.Passed = f.TakerBuy15m <= threshold
	}
	return check, true
}

func v7SummaryEMA20(tf V7ExecutionTimeframeSummary) float64 {
	if tf.EMA20 > 0 {
		return tf.EMA20
	}
	if tf.HasEMA20 && tf.LastClose > 0 {
		denom := 1.0 + tf.CloseVsEMA20Pct/100.0
		if denom > 0 {
			return tf.LastClose / denom
		}
	}
	return 0
}

func v7EMA20ClosePredicate(code string, f V7ConfirmFacts, timeframe string, long bool) (V7ConfirmationCheck, bool) {
	check := V7ConfirmationCheck{
		Code:     code,
		Severity: V7ConfirmReviewWait,
		Reason:   timeframe + " EMA20 is unavailable",
		Evidence: &V7ConfirmBarEvidence{
			Code:       code,
			Passed:     false,
			Timeframe:  timeframe,
			DecidingTF: "none",
			ClosedOnly: true,
		},
	}
	tf, ok := f.Timeframes[timeframe]
	if !ok || !tf.HasEMA20 || tf.LastClose <= 0 {
		// Missing required closed bar / EMA20 -> known, failed (consistent with
		// v7ClosedBarThroughEdgePredicate so every stage fails closed).
		return check, true
	}
	check.Actual = tf.CloseVsEMA20Pct
	if long {
		check.Passed = tf.CloseVsEMA20Pct >= 0
		check.Reason = timeframe + " close is below EMA20"
	} else {
		check.Passed = tf.CloseVsEMA20Pct <= 0
		check.Reason = timeframe + " close is above EMA20"
	}
	check.Evidence = &V7ConfirmBarEvidence{
		Code:             code,
		Passed:           check.Passed,
		Timeframe:        timeframe,
		DecidingTF:       timeframe,
		BarCloseTimeUnix: tf.LastCloseTimeUnix,
		BarClose:         tf.LastClose,
		ReferenceEdge:    v7SummaryEMA20(tf),
		ClosedOnly:       true,
	}
	return check, true
}

// v7Directional15mClosePredicate: the latest closed 15m close is on the
// direction's side of VWAP20 OR EMA20 OR the given zone threshold (any one
// suffices).
func v7Directional15mClosePredicate(code string, f V7ConfirmFacts, long bool, zoneThreshold float64) (V7ConfirmationCheck, bool) {
	check := V7ConfirmationCheck{
		Code:      code,
		Threshold: zoneThreshold,
		Severity:  V7ConfirmReviewWait,
		Reason:    "15m execution close is unavailable",
		Evidence: &V7ConfirmBarEvidence{
			Code:          code,
			Passed:        false,
			Timeframe:     "15m",
			DecidingTF:    "none",
			ReferenceEdge: zoneThreshold,
			ClosedOnly:    true,
		},
	}
	tf, ok := f.Timeframes["15m"]
	if !ok || tf.LastClose <= 0 {
		// Missing required closed 15m bar -> known, failed (consistent with
		// v7ClosedBarThroughEdgePredicate so every stage fails closed).
		return check, true
	}
	check.Actual = tf.LastClose
	check.Threshold = zoneThreshold
	ema20 := v7SummaryEMA20(tf)
	refEdge := zoneThreshold
	passed := false
	if long {
		if tf.HasVWAP20 && tf.CloseVsVWAP20Pct >= 0 {
			passed = true
			check.Threshold = tf.VWAP20
			refEdge = tf.VWAP20
		}
		if tf.HasEMA20 && tf.CloseVsEMA20Pct >= 0 {
			passed = true
			if refEdge <= 0 && ema20 > 0 {
				refEdge = ema20
			}
		}
		if zoneThreshold > 0 && tf.LastClose >= zoneThreshold {
			passed = true
			if refEdge <= 0 {
				refEdge = zoneThreshold
			}
		}
		check.Reason = "15m close is not above VWAP/EMA20 or the required long zone threshold"
	} else {
		if tf.HasVWAP20 && tf.CloseVsVWAP20Pct <= 0 {
			passed = true
			check.Threshold = tf.VWAP20
			refEdge = tf.VWAP20
		}
		if tf.HasEMA20 && tf.CloseVsEMA20Pct <= 0 {
			passed = true
			if refEdge <= 0 && ema20 > 0 {
				refEdge = ema20
			}
		}
		if zoneThreshold > 0 && tf.LastClose <= zoneThreshold {
			passed = true
			if refEdge <= 0 {
				refEdge = zoneThreshold
			}
		}
		check.Reason = "15m close is not below VWAP/EMA20 or the required short zone threshold"
	}
	if refEdge <= 0 {
		if tf.HasVWAP20 && tf.VWAP20 > 0 {
			refEdge = tf.VWAP20
		} else if ema20 > 0 {
			refEdge = ema20
		}
	}
	check.Passed = passed
	check.Evidence = &V7ConfirmBarEvidence{
		Code:             code,
		Passed:           check.Passed,
		Timeframe:        "15m",
		DecidingTF:       "15m",
		BarCloseTimeUnix: tf.LastCloseTimeUnix,
		BarClose:         tf.LastClose,
		ReferenceEdge:    refEdge,
		ClosedOnly:       true,
	}
	return check, true
}

// v7OIOrVolumePredicate: 1h OI delta > 0 OR the 15m volume burst >= 1.2.
// Either leg alone is enough to pass; it is known when either leg is known.
func v7OIOrVolumePredicate(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
	check := V7ConfirmationCheck{Code: code, Actual: f.OIDelta1h, Threshold: 0, Severity: V7ConfirmReviewWait,
		Reason: "OI or short-term volume expansion is not yet confirmed"}
	if !f.HasOIDelta1h && f.VolumeBurst15m <= 0 {
		return check, false
	}
	check.Passed = (f.HasOIDelta1h && f.OIDelta1h > 0) || f.VolumeBurst15m >= v7ConfirmVolumeBurstExpands
	return check, true
}

// v7ClosedBarThroughEdgePredicate (U5/M2b) evaluates trigger codes strictly on
// the latest CLOSED 5m OR 15m bar's close against edge (longs: close >= edge;
// shorts: close <= edge). Spot/live f.Price is never read. When edge > 0 and
// neither timeframe has a closed bar, the verdict is known and failed (not
// unknown) so all stages fail closed consistently.
func v7ClosedBarThroughEdgePredicate(code string, f V7ConfirmFacts, edge float64, above bool, reason string) (V7ConfirmationCheck, bool) {
	check := V7ConfirmationCheck{
		Code:      code,
		Threshold: edge,
		Severity:  V7ConfirmReviewWait,
		Reason:    reason,
		Evidence: &V7ConfirmBarEvidence{
			Code:          code,
			Passed:        false,
			Timeframe:     "5m|15m",
			DecidingTF:    "none",
			ReferenceEdge: edge,
			ClosedOnly:    true,
		},
	}
	if edge <= 0 {
		return check, false
	}
	tf5, ok5 := f.Timeframes["5m"]
	has5 := ok5 && tf5.LastClose > 0
	tf15, ok15 := f.Timeframes["15m"]
	has15 := ok15 && tf15.LastClose > 0

	barPasses := func(closeVal float64) bool {
		if above {
			return closeVal >= edge
		}
		return closeVal <= edge
	}
	pass5 := has5 && barPasses(tf5.LastClose)
	pass15 := has15 && barPasses(tf15.LastClose)

	if pass5 || pass15 {
		decidingTF := "5m"
		chosen := tf5
		if !pass5 {
			decidingTF = "15m"
			chosen = tf15
		}
		check.Passed = true
		check.Actual = chosen.LastClose
		check.Evidence = &V7ConfirmBarEvidence{
			Code:             code,
			Passed:           true,
			Timeframe:        "5m|15m",
			DecidingTF:       decidingTF,
			BarCloseTimeUnix: chosen.LastCloseTimeUnix,
			BarClose:         chosen.LastClose,
			ReferenceEdge:    edge,
			ClosedOnly:       true,
		}
		return check, true
	}

	if !has5 && !has15 {
		// Neither timeframe has a closed bar -> known, failed (not unknown).
		return check, true
	}

	// Neither closed bar crossed edge; report the closest closed bar as evidence
	// (tie-break to 5m, the faster/more recent timeframe).
	decidingTF := "5m"
	chosen := tf5
	if !has5 {
		decidingTF = "15m"
		chosen = tf15
	} else if has15 {
		if above && tf15.LastClose > tf5.LastClose {
			decidingTF = "15m"
			chosen = tf15
		} else if !above && tf15.LastClose < tf5.LastClose {
			decidingTF = "15m"
			chosen = tf15
		}
	}
	check.Passed = false
	check.Actual = chosen.LastClose
	check.Evidence = &V7ConfirmBarEvidence{
		Code:             code,
		Passed:           false,
		Timeframe:        "5m|15m",
		DecidingTF:       decidingTF,
		BarCloseTimeUnix: chosen.LastCloseTimeUnix,
		BarClose:         chosen.LastClose,
		ReferenceEdge:    edge,
		ClosedOnly:       true,
	}
	return check, true
}

func v7ZoneMid(z V7PriceZone) float64 {
	if z.Lower <= 0 || z.Upper <= z.Lower {
		return 0
	}
	return (z.Lower + z.Upper) / 2
}

// ---- fact builders shared by scoring and live data ----

// V7Bar is the OHLCV a live-data layer hands to V7TimeframeFactsFromBars.
// OpenTime and CloseTime are optional Unix millisecond timestamps used to
// populate LastCloseTimeUnix in bar evidence.
type V7Bar struct {
	OpenTime, CloseTime      int64
	High, Low, Close, Volume float64
}

// V7TimeframeFactsFromBars builds the close facts the shared predicates read
// (LastClose, EMA20, VWAP20) from closed bars, with the scoring-time rules:
// VWAP20 over the last 20 bars and EMA20 only with at least 20 bars. A
// caller that already holds an EMA20 series value passes it as ema20 (> 0);
// otherwise EMA20 is computed from the bars.
func V7TimeframeFactsFromBars(timeframe string, bars []V7Bar, ema20 float64) (V7ExecutionTimeframeSummary, bool) {
	if len(bars) == 0 || bars[len(bars)-1].Close <= 0 {
		return V7ExecutionTimeframeSummary{}, false
	}
	kb := make([]klineBar, len(bars))
	for i, b := range bars {
		kb[i] = klineBar{
			OpenTime:  b.OpenTime,
			CloseTime: b.CloseTime,
			High:      b.High,
			Low:       b.Low,
			Close:     b.Close,
			Volume:    b.Volume,
		}
	}
	last := kb[len(kb)-1]
	out := V7ExecutionTimeframeSummary{
		Timeframe:         timeframe,
		CandleCount:       len(kb),
		LastClose:         last.Close,
		LastCloseTimeUnix: v7BarCloseTimeUnix(last.OpenTime, last.CloseTime, timeframe),
	}
	if ema20 <= 0 && len(kb) >= 20 {
		ema20 = computeEMAFromKlines(kb, 20)
	}
	if ema20 > 0 {
		out.HasEMA20 = true
		out.EMA20 = ema20
		out.CloseVsEMA20Pct = pctDiff(last.Close, ema20)
	}
	if len(kb) >= 20 {
		if vwap20 := computeVWAP(lastV7Klines(kb, 20)); vwap20 > 0 {
			out.HasVWAP20 = true
			out.VWAP20 = vwap20
			out.CloseVsVWAP20Pct = pctDiff(last.Close, vwap20)
		}
	}
	return out, true
}

// V7VolumeBurst is the last value over the average of the prior lookback
// values (the scoring-time VolumeBurst15m definition); 0 = unavailable.
func V7VolumeBurst(volumes []float64, lookback int) float64 {
	if len(volumes) < 3 || lookback <= 0 {
		return 0
	}
	last := volumes[len(volumes)-1]
	start := len(volumes) - 1 - lookback
	if start < 0 {
		start = 0
	}
	sum, count := 0.0, 0
	for i := start; i < len(volumes)-1; i++ {
		sum += volumes[i]
		count++
	}
	if count == 0 {
		return 0
	}
	avg := sum / float64(count)
	if avg <= 0 {
		return 0
	}
	return last / avg
}

// V7VolumeBurst15mLookback is the lookback VolumeBurst15m uses everywhere.
const V7VolumeBurst15mLookback = v7ConfirmVolumeBurstLookback

// v7ScoringConfirmFacts builds the shared facts from the scoring context.
// U5/M2b: prefers strictly closed-bar confirmation summaries/volume burst when
// built from raw klines; falls back to ExecutionContext.Timeframes only when
// ClosedConfirmTimeframes is nil (unit tests / golden fixtures).
func v7ScoringConfirmFacts(sig *V7SignalOutput, ctx *V7SymbolContext) V7ConfirmFacts {
	volBurst := ctx.VolumeBurst15m
	if ctx.ClosedConfirmTimeframes != nil {
		volBurst = ctx.ClosedVolumeBurst15m
	}
	f := V7ConfirmFacts{
		Direction:      sig.Direction,
		SetupType:      sig.SetupType,
		Price:          ctx.CurrentPrice,
		EntryZone:      sig.EntryZone,
		TakerBuy15m:    ctx.TakerBuy15m,
		VolumeBurst15m: volBurst,
	}
	if ctx.Snapshot != nil {
		f.OIDelta1h, f.HasOIDelta1h = ctx.Snapshot.OIDelta1h, true
	}
	if ctx.ClosedConfirmTimeframes != nil {
		f.Timeframes = ctx.ClosedConfirmTimeframes
	} else if ctx.ExecutionContext != nil {
		f.Timeframes = ctx.ExecutionContext.Timeframes
	}
	return f
}
