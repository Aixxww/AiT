package local

import (
	"math"
	"sort"
)

// ============================================================================
// Hunter v8 — Multi-Timeframe TP System (P0-B)
// ============================================================================
// Computes three take-profit levels (TP0/TP1/TP2) using ATR, VWAP, Bollinger
// Bands, and R-multiples across multiple timeframes.  TP0 is the nearest
// quick scalp target; TP1 is the swing target; TP2 is the extended runner.

// TargetLevel describes a single take-profit target with metadata.
type TargetLevel struct {
	Price      float64 `json:"price"`
	RR         float64 `json:"rr"`          // Risk-reward ratio vs entry
	TimeWindow string  `json:"time_window"` // e.g. "5m-30m", "1h-4h", "4h-24h"
	Method     string  `json:"method"`      // e.g. "nearest_0.8R", "median_1.5R"
}

// MultiTimeframeTP holds three structured TP levels.
type MultiTimeframeTP struct {
	TP0 TargetLevel `json:"tp0"` // Quick scalp
	TP1 TargetLevel `json:"tp1"` // Swing target
	TP2 TargetLevel `json:"tp2"` // Extended runner
}

// regimeTPMultiplier adjusts TP2 distance based on market regime.
var regimeTPMultiplier = map[V7MarketRegime]float64{
	V7RegimeTrendUp:     1.3,
	V7RegimeRotation:    1.0,
	V7RegimeRange:       0.8,
	V7RegimeCompression: 1.2,
	V7RegimeTrendDown:   0.9,
	V7RegimePanicDump:   1.4,
	V7RegimePullback:    1.1,
	V7RegimeManiaPump:   1.5,
	V7RegimeMixed:       1.0,
}

// CalcMultiTimeframeTP computes TP0/TP1/TP2 targets.
//
// TP0 (scalp): nearest of [0.8R, VWAP±0.5*ATR15m, BB upper/lower, 1*ATR15m]
// TP1 (swing):  median of [1.5R, 1.5*ATR1h, recent high/low * 0.995]
// TP2 (runner): 3*ATR1h * regimeMultiplier
func CalcMultiTimeframeTP(
	direction string,
	entryPrice, stopPrice, atr15m, atr1h float64,
	vwap15m, bbUpper, bbMiddle, bbLower float64,
	recentHigh, recentLow float64,
	regime string,
) *MultiTimeframeTP {
	if entryPrice <= 0 || stopPrice <= 0 || atr15m <= 0 {
		return nil
	}

	isLong := direction == "LONG" || direction == string(V7DirLong)
	var risk float64
	if isLong {
		risk = entryPrice - stopPrice
	} else {
		risk = stopPrice - entryPrice
	}
	if risk <= 0 {
		return nil
	}

	// --- TP0: nearest scalp target ---
	tp0Candidates := make([]float64, 0, 6)

	// 0.8R target
	if isLong {
		tp0Candidates = append(tp0Candidates, entryPrice+0.8*risk)
	} else {
		tp0Candidates = append(tp0Candidates, entryPrice-0.8*risk)
	}

	// VWAP + 0.5*ATR15m
	if vwap15m > 0 {
		if isLong {
			tp0Candidates = append(tp0Candidates, vwap15m+0.5*atr15m)
		} else {
			tp0Candidates = append(tp0Candidates, vwap15m-0.5*atr15m)
		}
	}

	// BB boundary
	if isLong && bbUpper > 0 {
		tp0Candidates = append(tp0Candidates, bbUpper)
	} else if !isLong && bbLower > 0 {
		tp0Candidates = append(tp0Candidates, bbLower)
	}

	// 1*ATR15m from entry
	if isLong {
		tp0Candidates = append(tp0Candidates, entryPrice+1.0*atr15m)
	} else {
		tp0Candidates = append(tp0Candidates, entryPrice-1.0*atr15m)
	}

	tp0Price := nearestInDirection(isLong, entryPrice, tp0Candidates)
	tp0RR := calcRR(entryPrice, stopPrice, tp0Price, isLong)
	tp0Method := "nearest_0.8R_vwap_bb_atr"

	// --- TP1: median swing target ---
	tp1Candidates := make([]float64, 0, 4)

	// 1.5R
	if isLong {
		tp1Candidates = append(tp1Candidates, entryPrice+1.5*risk)
	} else {
		tp1Candidates = append(tp1Candidates, entryPrice-1.5*risk)
	}

	// 1.5*ATR1h
	if atr1h > 0 {
		if isLong {
			tp1Candidates = append(tp1Candidates, entryPrice+1.5*atr1h)
		} else {
			tp1Candidates = append(tp1Candidates, entryPrice-1.5*atr1h)
		}
	}

	// Recent high/low * 0.995 (near structure)
	if isLong && recentHigh > 0 {
		tp1Candidates = append(tp1Candidates, recentHigh*0.995)
	} else if !isLong && recentLow > 0 {
		tp1Candidates = append(tp1Candidates, recentLow*1.005)
	}

	tp1Price := medianInDirection(isLong, entryPrice, tp1Candidates)
	if tp1Price <= 0 {
		tp1Price = tp0Price // fallback
	}
	tp1RR := calcRR(entryPrice, stopPrice, tp1Price, isLong)
	tp1Method := "median_1.5R_atr_structure"

	// --- TP2: extended runner ---
	mult := 3.0
	if m, ok := regimeTPMultiplier[V7MarketRegime(regime)]; ok {
		mult *= m
	}
	var tp2Price float64
	if atr1h > 0 {
		if isLong {
			tp2Price = entryPrice + mult*atr1h
		} else {
			tp2Price = entryPrice - mult*atr1h
		}
	} else {
		// Fallback: use 3*R
		if isLong {
			tp2Price = entryPrice + 3.0*risk
		} else {
			tp2Price = entryPrice - 3.0*risk
		}
	}
	tp2RR := calcRR(entryPrice, stopPrice, tp2Price, isLong)
	tp2Method := "3xATR1h_regime_mult"

	return &MultiTimeframeTP{
		TP0: TargetLevel{Price: tp0Price, RR: tp0RR, TimeWindow: "5m-30m", Method: tp0Method},
		TP1: TargetLevel{Price: tp1Price, RR: tp1RR, TimeWindow: "1h-4h", Method: tp1Method},
		TP2: TargetLevel{Price: tp2Price, RR: tp2RR, TimeWindow: "4h-24h", Method: tp2Method},
	}
}

// ApplyMultiTimeframeTP fills TP0/TP1/TP2 fields on a V7SignalOutput.
func ApplyMultiTimeframeTP(sig *V7SignalOutput, ctx *V7SymbolContext) {
	if sig == nil || ctx == nil {
		return
	}

	// Use entry zone midpoint as entry price
	entry := (sig.EntryZone.Lower + sig.EntryZone.Upper) / 2
	if entry <= 0 {
		entry = ctx.CurrentPrice
	}
	stop := sig.Invalidation.Price
	if stop <= 0 || entry <= 0 {
		return
	}

	// Determine recent high/low from context
	recentHigh := ctx.High1h
	if ctx.High4h > recentHigh {
		recentHigh = ctx.High4h
	}
	recentLow := ctx.Low1h
	if recentLow <= 0 || (ctx.Low4h > 0 && ctx.Low4h < recentLow) {
		recentLow = ctx.Low4h
	}

	// BB middle fallback to VWAP
	bbMiddle := ctx.BBMiddle15m
	if bbMiddle <= 0 {
		bbMiddle = ctx.VWAP15m
	}

	mtp := CalcMultiTimeframeTP(
		string(sig.Direction),
		entry, stop,
		ctx.ATR15m, ctx.ATR1h,
		ctx.VWAP15m,
		ctx.BBUpper15m, bbMiddle, ctx.BBLower15m,
		recentHigh, recentLow,
		string(sig.MarketRegime),
	)
	if mtp == nil {
		return
	}

	sig.TP0Price = mtp.TP0.Price
	sig.TP0RR = mtp.TP0.RR
	sig.TP0TimeWindow = mtp.TP0.TimeWindow
	sig.TP0Method = mtp.TP0.Method
	sig.TP1Price = mtp.TP1.Price
	sig.TP1RR = mtp.TP1.RR
	sig.TP1TimeWindow = mtp.TP1.TimeWindow
	sig.TP1Method = mtp.TP1.Method
	sig.TP2Price = mtp.TP2.Price
	sig.TP2RR = mtp.TP2.RR
	sig.TP2TimeWindow = mtp.TP2.TimeWindow
	sig.TP2Method = mtp.TP2.Method
	syncV7TargetsWithMultiTimeframeTP(sig, mtp, entry)
	ApplyV7TakeProfitPlan(sig, ctx)
}

func ApplyV7TakeProfitPlan(sig *V7SignalOutput, ctx *V7SymbolContext) {
	if sig == nil || ctx == nil {
		return
	}
	entry := (sig.EntryZone.Lower + sig.EntryZone.Upper) / 2
	if entry <= 0 {
		entry = ctx.CurrentPrice
	}
	if entry <= 0 {
		return
	}

	if isV7HighVelocitySetup(sig.SetupType) {
		ensureV7TP0Distance(sig, entry, 1.2, 2.5)
	}
	if sig.TP0Price <= 0 {
		return
	}

	dist := math.Abs(pctDiff(sig.TP0Price, entry))
	plan := &V7TakeProfitPlan{
		TP0Price:               sig.TP0Price,
		TP0DistancePct:         dist,
		TP0ReducePctMin:        30,
		TP0ReducePctMax:        50,
		MoveStopToBreakeven:    true,
		TrailingStopMode:       "after_tp0_partial",
		TrailingBasis:          []string{"5m_ema20", "15m_vwap", "0.8-1.2atr15m"},
		TrailingDistancePctMin: 0.8,
		TrailingDistancePctMax: 1.2,
		StatsBucket:            "tp0_tp1_no_sl",
	}
	if !isV7HighVelocitySetup(sig.SetupType) {
		plan.TrailingStopMode = "after_tp0_optional"
	}
	sig.TPPlan = plan
}

func isV7HighVelocitySetup(setup V7SetupType) bool {
	switch setup {
	case V7SetupRangeExpansion,
		V7SetupDisplacementLong,
		V7SetupLeaderMomentumLong,
		V7SetupTrendBreakoutLong,
		V7SetupVolatilitySqueeze:
		return true
	default:
		return false
	}
}

func ensureV7TP0Distance(sig *V7SignalOutput, entry, minPct, maxPct float64) {
	if sig == nil || entry <= 0 {
		return
	}
	dist := math.Abs(pctDiff(sig.TP0Price, entry))
	if dist <= 0 {
		dist = (minPct + maxPct) / 2
	}
	dist = math.Max(minPct, math.Min(maxPct, dist))
	if sig.Direction == V7DirShort {
		sig.TP0Price = entry * (1 - dist/100)
	} else {
		sig.TP0Price = entry * (1 + dist/100)
	}
	if sig.Invalidation.Price > 0 {
		risk := math.Abs(entry - sig.Invalidation.Price)
		if risk > 0 {
			sig.TP0RR = math.Abs(sig.TP0Price-entry) / risk
		}
	}
	if sig.TP0TimeWindow == "" {
		sig.TP0TimeWindow = "5m-30m"
	}
	if sig.TP0Method == "" || sig.TP0Method == "nearest_0.8R" {
		sig.TP0Method = "tp0_dynamic_1.2_2.5pct"
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nearestInDirection returns the candidate closest to entry in the profitable
// direction (i.e. the lowest positive-distance target for longs).
func nearestInDirection(isLong bool, entry float64, candidates []float64) float64 {
	if len(candidates) == 0 {
		return 0
	}
	// Filter to valid targets in the correct direction
	valid := make([]float64, 0, len(candidates))
	for _, c := range candidates {
		if c <= 0 {
			continue
		}
		if isLong && c > entry {
			valid = append(valid, c)
		} else if !isLong && c < entry {
			valid = append(valid, c)
		}
	}
	if len(valid) == 0 {
		return 0
	}
	// Sort by distance from entry
	sort.Float64s(valid)
	if isLong {
		return valid[0] // closest above entry
	}
	return valid[len(valid)-1] // closest below entry
}

// medianInDirection returns the median candidate in the profitable direction.
func medianInDirection(isLong bool, entry float64, candidates []float64) float64 {
	valid := make([]float64, 0, len(candidates))
	for _, c := range candidates {
		if c <= 0 {
			continue
		}
		if isLong && c > entry {
			valid = append(valid, c)
		} else if !isLong && c < entry {
			valid = append(valid, c)
		}
	}
	if len(valid) == 0 {
		return 0
	}
	sort.Float64s(valid)
	mid := len(valid) / 2
	if isLong {
		return valid[mid]
	}
	// For short, sort ascending; median is in the middle (larger = closer to entry)
	return valid[mid]
}

// calcRR computes risk-reward ratio given entry, stop, and target.
func calcRR(entry, stop, target float64, isLong bool) float64 {
	risk := 0.0
	reward := 0.0
	if isLong {
		risk = entry - stop
		reward = target - entry
	} else {
		risk = stop - entry
		reward = entry - target
	}
	if risk <= 0 {
		return 0
	}
	return math.Abs(reward / risk)
}

func syncV7TargetsWithMultiTimeframeTP(sig *V7SignalOutput, mtp *MultiTimeframeTP, entry float64) {
	if sig == nil || mtp == nil || mtp.TP1.Price <= 0 || entry <= 0 {
		return
	}
	isLong := sig.Direction == V7DirLong
	isDirectional := func(price float64) bool {
		if price <= 0 {
			return false
		}
		if isLong {
			return price > entry
		}
		return price < entry
	}
	if !isDirectional(mtp.TP1.Price) {
		return
	}

	original := append([]V7Target{}, sig.Targets...)
	targets := []V7Target{{Price: mtp.TP1.Price, Reason: "tp1_" + mtp.TP1.Method}}
	if isDirectional(mtp.TP2.Price) && !v7TargetPriceExists(targets, mtp.TP2.Price, entry) {
		targets = append(targets, V7Target{Price: mtp.TP2.Price, Reason: "tp2_" + mtp.TP2.Method})
	}
	for _, target := range original {
		if !isDirectional(target.Price) || v7TargetPriceExists(targets, target.Price, entry) {
			continue
		}
		targets = append(targets, target)
	}
	sig.Targets = targets
}

func v7TargetPriceExists(targets []V7Target, price, entry float64) bool {
	if price <= 0 || entry <= 0 {
		return false
	}
	for _, target := range targets {
		if math.Abs(target.Price-price)/entry < 0.001 {
			return true
		}
	}
	return false
}
