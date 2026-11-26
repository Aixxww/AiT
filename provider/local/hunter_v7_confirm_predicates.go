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

	// Trigger codes: the live price against the entry-zone edge.
	"5m_or_15m_close_above_trigger": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7PriceThroughEdgePredicate(code, f, f.EntryZone.Upper, true, "long trigger has not cleared entry-zone upper bound")
	},
	"5m_or_15m_close_through_breakout_level": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7PriceThroughEdgePredicate(code, f, f.EntryZone.Upper, true, "long trigger has not cleared entry-zone upper bound")
	},
	"5m_or_15m_close_below_trigger": func(code string, f V7ConfirmFacts) (V7ConfirmationCheck, bool) {
		return v7PriceThroughEdgePredicate(code, f, f.EntryZone.Lower, false, "short trigger has not cleared entry-zone lower bound")
	},
}

// v7ConfirmPredicateAliases are codes that share another code's predicate
// verbatim at scoring time. They are not live-verifiable codes of their own.
var v7ConfirmPredicateAliases = map[string]string{
	"taker_buy_15m_stays_above_0_52":  "taker_buy_15m_gt_0_52",
	"oi_continues_inflow":             "oi_delta_1h_positive_or_quote_volume_expands",
	"oi_or_volume_expands_with_price": "oi_delta_1h_positive_or_quote_volume_expands",
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

func v7EMA20ClosePredicate(code string, f V7ConfirmFacts, timeframe string, long bool) (V7ConfirmationCheck, bool) {
	check := V7ConfirmationCheck{Code: code, Severity: V7ConfirmReviewWait, Reason: timeframe + " EMA20 is unavailable"}
	tf, ok := f.Timeframes[timeframe]
	if !ok || !tf.HasEMA20 || tf.LastClose <= 0 {
		return check, false
	}
	check.Actual = tf.CloseVsEMA20Pct
	if long {
		check.Passed = tf.CloseVsEMA20Pct >= 0
		check.Reason = timeframe + " close is below EMA20"
	} else {
		check.Passed = tf.CloseVsEMA20Pct <= 0
		check.Reason = timeframe + " close is above EMA20"
	}
	return check, true
}

// v7Directional15mClosePredicate: the latest 15m close is on the direction's
// side of VWAP20 OR EMA20 OR the given zone threshold (any one suffices).
func v7Directional15mClosePredicate(code string, f V7ConfirmFacts, long bool, zoneThreshold float64) (V7ConfirmationCheck, bool) {
	check := V7ConfirmationCheck{Code: code, Severity: V7ConfirmReviewWait, Reason: "15m execution close is unavailable"}
	tf, ok := f.Timeframes["15m"]
	if !ok || tf.LastClose <= 0 {
		return check, false
	}
	check.Actual = tf.LastClose
	check.Threshold = zoneThreshold
	passed := false
	if long {
		if tf.HasVWAP20 && tf.CloseVsVWAP20Pct >= 0 {
			passed = true
			check.Threshold = tf.VWAP20
		}
		if tf.HasEMA20 && tf.CloseVsEMA20Pct >= 0 {
			passed = true
		}
		if zoneThreshold > 0 && tf.LastClose >= zoneThreshold {
			passed = true
		}
		check.Reason = "15m close is not above VWAP/EMA20 or the required long zone threshold"
	} else {
		if tf.HasVWAP20 && tf.CloseVsVWAP20Pct <= 0 {
			passed = true
			check.Threshold = tf.VWAP20
		}
		if tf.HasEMA20 && tf.CloseVsEMA20Pct <= 0 {
			passed = true
		}
		if zoneThreshold > 0 && tf.LastClose <= zoneThreshold {
			passed = true
		}
		check.Reason = "15m close is not below VWAP/EMA20 or the required short zone threshold"
	}
	check.Passed = passed
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

func v7PriceThroughEdgePredicate(code string, f V7ConfirmFacts, edge float64, above bool, reason string) (V7ConfirmationCheck, bool) {
	check := V7ConfirmationCheck{Code: code, Actual: f.Price, Threshold: edge, Severity: V7ConfirmReviewWait, Reason: reason}
	if f.Price <= 0 || edge <= 0 {
		return check, false
	}
	if above {
		check.Passed = f.Price >= edge
	} else {
		check.Passed = f.Price <= edge
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
type V7Bar struct {
	High, Low, Close, Volume float64
}

// V7TimeframeFactsFromBars builds the close facts the shared predicates read
// (LastClose, EMA20, VWAP20) from live bars, with the scoring-time rules:
// VWAP20 over the last 20 bars and EMA20 only with at least 20 bars. A
// caller that already holds an EMA20 series value passes it as ema20 (> 0);
// otherwise EMA20 is computed from the bars.
func V7TimeframeFactsFromBars(timeframe string, bars []V7Bar, ema20 float64) (V7ExecutionTimeframeSummary, bool) {
	if len(bars) == 0 || bars[len(bars)-1].Close <= 0 {
		return V7ExecutionTimeframeSummary{}, false
	}
	kb := make([]klineBar, len(bars))
	for i, b := range bars {
		kb[i] = klineBar{High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume}
	}
	last := kb[len(kb)-1]
	out := V7ExecutionTimeframeSummary{Timeframe: timeframe, CandleCount: len(kb), LastClose: last.Close}
	if ema20 <= 0 && len(kb) >= 20 {
		ema20 = computeEMAFromKlines(kb, 20)
	}
	if ema20 > 0 {
		out.HasEMA20 = true
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
func v7ScoringConfirmFacts(sig *V7SignalOutput, ctx *V7SymbolContext) V7ConfirmFacts {
	f := V7ConfirmFacts{
		Direction:      sig.Direction,
		SetupType:      sig.SetupType,
		Price:          ctx.CurrentPrice,
		EntryZone:      sig.EntryZone,
		TakerBuy15m:    ctx.TakerBuy15m,
		VolumeBurst15m: ctx.VolumeBurst15m,
	}
	if ctx.Snapshot != nil {
		f.OIDelta1h, f.HasOIDelta1h = ctx.Snapshot.OIDelta1h, true
	}
	if ctx.ExecutionContext != nil {
		f.Timeframes = ctx.ExecutionContext.Timeframes
	}
	return f
}
