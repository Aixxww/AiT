package local

import (
	"math"
	"sort"
	"strings"
	"sync/atomic"
)

// ============================================================================
// Hunter v7 — Structural geometry module (M2, plan §3 / Appendix F)
// ============================================================================
// Implements the C4 structural geometry: targets are read from confirmed
// pivots, higher-timeframe walls and trailing extremes — never from ATR or
// R multiples — and stops are placed beyond a structural anchor with a
// horizon-scaled buffer, subject to a noise band [d_min, d_max]. Admission
// is by measured RR net of fees against the structural TP1 (§3.5, RR_min
// from decision D1). The module is pure: callers pass bars (including any
// bars after the decision time); every series is filtered to bars whose
// close time is <= DecisionTimeMs before use, so the output cannot depend
// on future bars.
//
// Direction convention: LONG is worked directly; SHORT mirrors every rule
// (pivot lows become targets, pivot highs become stop anchors, distances
// are measured on the profitable/adverse side respectively).

// V7StructureBar is one OHLC bar. OpenTimeMs is the bar open time in
// milliseconds since the Unix epoch; the bar closes at
// OpenTimeMs+interval.
type V7StructureBar struct {
	OpenTimeMs int64
	Open       float64
	High       float64
	Low        float64
	Close      float64
}

// V7StructureFamilyClass selects the §3.3 structural-anchor rule.
type V7StructureFamilyClass int

const (
	// V7FamilyBreakoutContinuation: anchor = most recent confirmed TF_s
	// pivot extreme on the adverse side of E.
	V7FamilyBreakoutContinuation V7StructureFamilyClass = iota
	// V7FamilyPullbackReversal: anchor = extreme of the reaction leg since
	// the invalidation timestamp (requires InvalidationTimeMs).
	V7FamilyPullbackReversal
)

// V7StructureConfig carries every constant of plan §3 for one horizon
// class. The zero value resolves to the FAST class via withDefaults.
type V7StructureConfig struct {
	Class             string  // "FAST" or "SWING" (label only)
	TFSIntervalMs     int64   // structure timeframe
	TFHIntervalMs     int64   // higher timeframe
	PivotKTFS         int     // pivot half-window on TF_s (3 on 15m)
	PivotKTFH         int     // pivot half-window on TF_h (2 on 1h/4h)
	LookbackTFS       int     // trailing bars used on TF_s (96 = 24h of 15m)
	LookbackTFH       int     // trailing bars used on TF_h (168 = 7d of 1h)
	NoiseHorizonBars  int     // noise horizon h in TF_s bars (30m = 2 x 15m)
	NoiseWindowBars   int     // bars of TF_s history for N_H (7d = 672 x 15m)
	BufferATRMult     float64 // stop buffer = 0.30 * ATR14(TF_s)
	WallClusterATR    float64 // wall clustering width = 0.5 * ATR14(TF_h)
	FrontRunATRMult   float64 // target front-run f_c = 0.15 * ATR14(candidate TF)
	TargetDropPct     float64 // drop targets closer than cost+0.25% (= 0.0040)
	CostPct           float64 // cost_pct = 0.15%
	DMinFloor         float64 // d_min floor = 0.80%
	DMaxCap           float64 // d_max cap = 6.0%
	DMaxATRMult       float64 // d_max = min(cap, 3.0 * ATR14(TF_h) / E)
	SCSlackPct        float64 // S_C slack over N_H = 0.10%
	TPCapPct          float64 // effective-TP cap (FAST 5%, SWING 10%)
	RRMin             float64 // measured-RR admission floor (D1: 1.5)
	ATRPeriod         int     // 14
}

// V7FASTStructureConfig returns the §3.1 FAST class constants.
func V7FASTStructureConfig() V7StructureConfig {
	return V7StructureConfig{
		Class:            "FAST",
		TFSIntervalMs:    15 * 60 * 1000,
		TFHIntervalMs:    60 * 60 * 1000,
		PivotKTFS:        3,
		PivotKTFH:        2,
		LookbackTFS:      96,
		LookbackTFH:      168,
		NoiseHorizonBars: 2,
		NoiseWindowBars:  7 * 96,
		BufferATRMult:    0.30,
		WallClusterATR:   0.5,
		FrontRunATRMult:  0.15,
		TargetDropPct:    0.0015 + 0.0025,
		CostPct:          0.0015,
		DMinFloor:        0.008,
		DMaxCap:          0.06,
		DMaxATRMult:      3.0,
		SCSlackPct:       0.001,
		TPCapPct:         0.05,
		RRMin:            1.5,
		ATRPeriod:        14,
	}
}

// V7SWINGStructureConfig returns the spec v2 V2-1 SWING class constants:
// TF_s 1h, TF_h 4h, cap 10%, pivot k 2/2, lookback 168 x 1h and 90 x 4h,
// noise horizon 2h (2 x 1h) over a 7d window of 1h closes. Every other
// constant equals FAST.
func V7SWINGStructureConfig() V7StructureConfig {
	c := V7FASTStructureConfig()
	c.Class = "SWING"
	c.TFSIntervalMs = 60 * 60 * 1000
	c.TFHIntervalMs = 4 * 60 * 60 * 1000
	c.PivotKTFS = 2
	c.PivotKTFH = 2
	c.LookbackTFS = 168
	c.LookbackTFH = 90
	c.NoiseHorizonBars = 2
	c.NoiseWindowBars = 7 * 24
	c.TPCapPct = 0.10
	return c
}

// v7SwingSetupPrefixes lists the setup families plan §3.1 / spec v2 V2-1
// assign to the SWING class; every other setup is FAST.
var v7SwingSetupPrefixes = []string{
	// task-2 ambush families
	"bb_oi_", "compression_", "confirmed_", "reclaim_", "trend_",
	// native hunter_v7 families (plan §3.1)
	"accumulation_breakout", "trend_breakout", "short_squeeze", "long_squeeze",
	"volatility_squeeze", "pullback_reversal", "panic_reversal", "range_",
	"funding_reversal", "distribution",
}

// V7StructureClassForSetup returns "SWING" or "FAST" for a setup type.
func V7StructureClassForSetup(setup string) string {
	s := strings.ToLower(setup)
	for _, p := range v7SwingSetupPrefixes {
		if strings.HasPrefix(s, p) {
			return "SWING"
		}
	}
	return "FAST"
}

// V7StructureConfigForSetup returns the class configuration for a setup.
func V7StructureConfigForSetup(setup string) V7StructureConfig {
	if V7StructureClassForSetup(setup) == "SWING" {
		return V7SWINGStructureConfig()
	}
	return V7FASTStructureConfig()
}

func (c V7StructureConfig) withDefaults() V7StructureConfig {
	def := V7FASTStructureConfig()
	if c.Class == "" {
		c.Class = def.Class
	}
	if c.TFSIntervalMs == 0 {
		c.TFSIntervalMs = def.TFSIntervalMs
	}
	if c.TFHIntervalMs == 0 {
		c.TFHIntervalMs = def.TFHIntervalMs
	}
	if c.PivotKTFS == 0 {
		c.PivotKTFS = def.PivotKTFS
	}
	if c.PivotKTFH == 0 {
		c.PivotKTFH = def.PivotKTFH
	}
	if c.LookbackTFS == 0 {
		c.LookbackTFS = def.LookbackTFS
	}
	if c.LookbackTFH == 0 {
		c.LookbackTFH = def.LookbackTFH
	}
	if c.NoiseHorizonBars == 0 {
		c.NoiseHorizonBars = def.NoiseHorizonBars
	}
	if c.NoiseWindowBars == 0 {
		c.NoiseWindowBars = def.NoiseWindowBars
	}
	if c.BufferATRMult == 0 {
		c.BufferATRMult = def.BufferATRMult
	}
	if c.WallClusterATR == 0 {
		c.WallClusterATR = def.WallClusterATR
	}
	if c.FrontRunATRMult == 0 {
		c.FrontRunATRMult = def.FrontRunATRMult
	}
	if c.TargetDropPct == 0 {
		c.TargetDropPct = def.TargetDropPct
	}
	if c.CostPct == 0 {
		c.CostPct = def.CostPct
	}
	if c.DMinFloor == 0 {
		c.DMinFloor = def.DMinFloor
	}
	if c.DMaxCap == 0 {
		c.DMaxCap = def.DMaxCap
	}
	if c.DMaxATRMult == 0 {
		c.DMaxATRMult = def.DMaxATRMult
	}
	if c.SCSlackPct == 0 {
		c.SCSlackPct = def.SCSlackPct
	}
	if c.TPCapPct == 0 {
		c.TPCapPct = def.TPCapPct
	}
	if c.RRMin == 0 {
		c.RRMin = def.RRMin
	}
	if c.ATRPeriod == 0 {
		c.ATRPeriod = def.ATRPeriod
	}
	return c
}

// V7StructureInput is the full input to the geometry builder. Bars may
// extend beyond DecisionTimeMs; they are filtered internally.
type V7StructureInput struct {
	Direction         V7Direction
	EntryPrice        float64 // E: the single reference (executable) price
	DecisionTimeMs    int64   // only bars closed at or before this time are used
	BarsTFS           []V7StructureBar
	BarsTFH           []V7StructureBar
	Family            V7StructureFamilyClass
	InvalidationPrice float64 // optional module invalidation price (0 = none)
	InvalidationTimeMs int64  // optional; required for the pullback anchor rule
	Config            V7StructureConfig
}

// V7StopCandidate is one entry of the stop menu (plan §2b: <= 3, with ID).
type V7StopCandidate struct {
	ID     string  `json:"id"` // "S_A" | "S_B" | "S_C"
	Price  float64 `json:"price"`
	DistPct float64 `json:"dist_pct"` // |E - price| / E, in percent
	Anchor string  `json:"anchor"`   // human-readable anchor description
}

// V7TargetCandidate is one entry of the target menu (plan §2b: <= 4,
// with ID). Price is the adjusted level T_c (after the §3.2 front-run).
type V7TargetCandidate struct {
	ID       string  `json:"id"` // "T1", "T2", ... by ascending distance
	Price    float64 `json:"price"`
	RawLevel float64 `json:"raw_level"` // structural level before front-run
	SourceTF string  `json:"source_tf"` // TF label of the source bars, e.g. "15m", "1h", "4h"
	Method   string  `json:"method"`    // pivot_tfs | wall_tfh | extreme_tfs | extreme_tfh
	Touches  int     `json:"touches"`
	Role     string  `json:"role"` // "TP0" | "TP1" | "TP2" | ""
}

// V7StructurePlan is the builder output. When Feasible is false, Reason
// carries the plan's wait code ("rr_insufficient", "backend_guard_risk"
// or "no_data"); the candidate menus are still populated for inspection.
type V7StructurePlan struct {
	Feasible   bool    `json:"feasible"`
	Reason     string  `json:"reason,omitempty"`
	EntryPrice float64 `json:"entry_price"`
	Direction  string  `json:"direction"`

	ATRTFS     float64 `json:"atr_tfs"`
	ATRTFH     float64 `json:"atr_tfh"`
	Buffer     float64 `json:"buffer"`
	NoiseHPct  float64 `json:"noise_h_pct"` // N_H, percent
	DMinPct    float64 `json:"d_min_pct"`
	DMaxPct    float64 `json:"d_max_pct"`
	SStruct    float64 `json:"s_struct"`

	StopCandidates   []V7StopCandidate   `json:"stop_candidates"`
	TargetCandidates []V7TargetCandidate `json:"target_candidates"`

	TP0 float64 `json:"tp0"` // effective (capped) prices; 0 when absent
	TP1 float64 `json:"tp1"`
	TP2 float64 `json:"tp2"`

	ChosenStop *V7StopCandidate `json:"chosen_stop,omitempty"`
	RRMeasTP1  float64          `json:"rr_meas_tp1,omitempty"`

	// Spec v2 report fields (never gates).
	Class       string `json:"class"`
	TP1Fallback bool   `json:"tp1_fallback,omitempty"` // TP1 is a 1-touch TF_s pivot
	// RejectDetail refines Reason: no_candidate_in_direction,
	// nearest_candidate_beyond_cap, no_tp1_eligible_within_cap,
	// no_structural_anchor, no_stop_in_band, s_a_fails_rr.
	RejectDetail string `json:"reject_detail,omitempty"`
	// NearestCandidateDistPct is the distance E -> nearest adjusted target
	// candidate in the trade direction, in percent, before the cap
	// (0 when there is none). Input for any future cap revision.
	NearestCandidateDistPct float64 `json:"nearest_candidate_dist_pct,omitempty"`
}

// ---------------------------------------------------------------------------
// Primitive series helpers
// ---------------------------------------------------------------------------

// v7ClosedBars returns the bars whose close time is <= decisionMs,
// sorted by open time. This is the single no-lookahead gate.
func v7ClosedBars(bars []V7StructureBar, intervalMs, decisionMs int64) []V7StructureBar {
	out := make([]V7StructureBar, 0, len(bars))
	for _, b := range bars {
		if b.OpenTimeMs+intervalMs <= decisionMs {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenTimeMs < out[j].OpenTimeMs })
	return out
}

func v7Trailing(bars []V7StructureBar, n int) []V7StructureBar {
	if len(bars) <= n {
		return bars
	}
	return bars[len(bars)-n:]
}

// v7ATR14 computes ATR with Wilder smoothing, matching the house
// convention (snapATR/computeATR): seed = SMA of the first `period`
// true ranges (bars 1..period), then Wilder over the remaining bars.
// Returns 0 when the series is too short.
func v7ATR(bars []V7StructureBar, period int) float64 {
	if period <= 0 || len(bars) < period+1 {
		return 0
	}
	tr := func(i int) float64 {
		h, l, pc := bars[i].High, bars[i].Low, bars[i-1].Close
		return math.Max(h-l, math.Max(math.Abs(h-pc), math.Abs(l-pc)))
	}
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += tr(i)
	}
	atr := sum / float64(period)
	for i := period + 1; i < len(bars); i++ {
		atr = (atr*float64(period-1) + tr(i)) / float64(period)
	}
	return atr
}

// v7Pivot is one confirmed swing point. A pivot at bar i is confirmed
// only once the k bars to its right have closed; because the input
// series is pre-filtered to closed bars, any pivot found in it is
// confirmed at the decision time.
type v7Pivot struct {
	Index   int
	Price   float64
	TimeMs  int64
	Touches int // filled later for the target side (cluster size)
}

// v7Pivots finds pivot highs (high=true) or pivot lows per §3.2:
// high_i > max(high_{i-k..i-1}) and high_i >= max(high_{i+1..i+k});
// lows mirror (< and <=).
func v7Pivots(bars []V7StructureBar, k int, high bool) []v7Pivot {
	var out []v7Pivot
	if k <= 0 || len(bars) < 2*k+1 {
		return out
	}
	for i := k; i+k < len(bars); i++ {
		p := bars[i].High
		if !high {
			p = bars[i].Low
		}
		ok := true
		for j := i - k; j < i && ok; j++ {
			v := bars[j].High
			if !high {
				v = bars[j].Low
			}
			if high && v >= p {
				ok = false
			}
			if !high && v <= p {
				ok = false
			}
		}
		for j := i + 1; j <= i+k && ok; j++ {
			v := bars[j].High
			if !high {
				v = bars[j].Low
			}
			if high && v > p {
				ok = false
			}
			if !high && v < p {
				ok = false
			}
		}
		if ok {
			out = append(out, v7Pivot{Index: i, Price: p, TimeMs: bars[i].OpenTimeMs})
		}
	}
	return out
}

// v7Cluster is a single-linkage price cluster of pivots.
type v7Cluster struct {
	Level   float64 // median price
	Touches int
	Members []float64
}

// v7ClusterPivots clusters pivot prices whose neighbours in sorted
// order are within `width` of each other (single linkage). The level
// is the median member price; touches = member count.
func v7ClusterPivots(pivots []v7Pivot, width float64) []v7Cluster {
	if len(pivots) == 0 {
		return nil
	}
	prices := make([]float64, len(pivots))
	for i, p := range pivots {
		prices[i] = p.Price
	}
	sort.Float64s(prices)
	var clusters []v7Cluster
	start := 0
	flush := func(end int) {
		members := append([]float64(nil), prices[start:end]...)
		// Median price; for an even count, the mean of the two
		// central values (keeps LONG/SHORT mirror symmetry).
		level := members[len(members)/2]
		if len(members)%2 == 0 {
			level = (members[len(members)/2-1] + members[len(members)/2]) / 2
		}
		clusters = append(clusters, v7Cluster{
			Level:   level,
			Touches: len(members),
			Members: members,
		})
	}
	for i := 1; i < len(prices); i++ {
		if prices[i]-prices[i-1] > width {
			flush(i)
			start = i
		}
	}
	flush(len(prices))
	return clusters
}

// v7ClusterSizeOf returns the size of the cluster containing price
// (same single-linkage rule), used for TF_s pivot touch counts.
func v7ClusterSizeOf(pivots []v7Pivot, width, price float64) int {
	for _, c := range v7ClusterPivots(pivots, width) {
		for _, m := range c.Members {
			if m == price {
				return c.Touches
			}
		}
	}
	return 1
}

// v7Percentile80 returns the 80th percentile (nearest-rank).
func v7Percentile80(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := append([]float64(nil), values...)
	sort.Float64s(s)
	rank := int(math.Ceil(0.8 * float64(len(s))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(s) {
		rank = len(s)
	}
	return s[rank-1]
}

// v7NoiseH computes N_H (§3.3): P80 over the trailing window of
// |close_t - close_{t-h}| / close_{t-h}, h in TF_s bars.
func v7NoiseH(closes []float64, lag int) float64 {
	if lag <= 0 || len(closes) <= lag {
		return 0
	}
	vals := make([]float64, 0, len(closes)-lag)
	for t := lag; t < len(closes); t++ {
		if closes[t-lag] > 0 {
			vals = append(vals, math.Abs(closes[t]-closes[t-lag])/closes[t-lag])
		}
	}
	return v7Percentile80(vals)
}

// ---------------------------------------------------------------------------
// Plan builder
// ---------------------------------------------------------------------------

// BuildV7StructurePlan computes the §3 structural geometry for one
// decision. All formulas follow Appendix F literally; SHORT mirrors.
func BuildV7StructurePlan(in V7StructureInput) *V7StructurePlan {
	cfg := in.Config.withDefaults()
	plan := &V7StructurePlan{
		EntryPrice: in.EntryPrice,
		Direction:  string(in.Direction),
		Class:      cfg.Class,
	}
	E := in.EntryPrice
	if E <= 0 {
		plan.Reason = "no_data"
		return plan
	}
	dir := 1.0
	if in.Direction == V7DirShort {
		dir = -1.0
	}
	isLong := dir > 0
	tfsLabel, tfhLabel := v7TFLabel(cfg.TFSIntervalMs), v7TFLabel(cfg.TFHIntervalMs)

	sAll := v7ClosedBars(in.BarsTFS, cfg.TFSIntervalMs, in.DecisionTimeMs)
	hAll := v7ClosedBars(in.BarsTFH, cfg.TFHIntervalMs, in.DecisionTimeMs)
	sBars := v7Trailing(sAll, cfg.LookbackTFS)
	hBars := v7Trailing(hAll, cfg.LookbackTFH)

	atrS := v7ATR(sBars, cfg.ATRPeriod)
	atrH := v7ATR(hBars, cfg.ATRPeriod)
	plan.ATRTFS, plan.ATRTFH = atrS, atrH
	if atrS <= 0 || atrH <= 0 {
		plan.Reason = "no_data"
		return plan
	}
	plan.Buffer = cfg.BufferATRMult * atrS

	// N_H over the trailing noise window of TF_s closes.
	noiseBars := v7Trailing(sAll, cfg.NoiseWindowBars)
	closes := make([]float64, len(noiseBars))
	for i, b := range noiseBars {
		closes[i] = b.Close
	}
	nh := v7NoiseH(closes, cfg.NoiseHorizonBars)
	plan.NoiseHPct = nh * 100
	dMin := math.Max(cfg.DMinFloor, nh)
	dMax := math.Min(cfg.DMaxCap, cfg.DMaxATRMult*atrH/E)
	plan.DMinPct, plan.DMaxPct = dMin*100, dMax*100

	// Pivots on both timeframes, both sides.
	sHighs := v7Pivots(sBars, cfg.PivotKTFS, true)
	sLows := v7Pivots(sBars, cfg.PivotKTFS, false)
	hHighs := v7Pivots(hBars, cfg.PivotKTFH, true)
	hLows := v7Pivots(hBars, cfg.PivotKTFH, false)

	// ---- Targets (§3.2) -------------------------------------------------
	// Target side: pivot highs / walls / trailing high for LONG;
	// pivot lows / walls / trailing low for SHORT.
	var tPivots, hPivots []v7Pivot
	if isLong {
		tPivots, hPivots = sHighs, hHighs
	} else {
		tPivots, hPivots = sLows, hLows
	}
	// TF_s touch counts via clustering on the target side.
	for i := range tPivots {
		tPivots[i].Touches = v7ClusterSizeOf(tPivots, cfg.WallClusterATR*atrS, tPivots[i].Price)
	}
	walls := v7ClusterPivots(hPivots, cfg.WallClusterATR*atrH)

	type rawCand struct {
		raw     float64
		tf      string
		method  string
		touches int
		atr     float64
	}
	var raws []rawCand
	for _, p := range tPivots {
		if dir*(p.Price-E) > 0 {
			raws = append(raws, rawCand{p.Price, tfsLabel, "pivot_tfs", p.Touches, atrS})
		}
	}
	for _, w := range walls {
		if w.Touches >= 2 && dir*(w.Level-E) > 0 {
			raws = append(raws, rawCand{w.Level, tfhLabel, "wall_tfh", w.Touches, atrH})
		}
	}
	// Trailing extremes over the TF_s and TF_h lookbacks (FAST: 24h / 7d;
	// SWING: 7d / 15d).
	if len(sBars) > 0 {
		ext := sBars[0].High
		if !isLong {
			ext = sBars[0].Low
		}
		for _, b := range sBars {
			if isLong && b.High > ext {
				ext = b.High
			}
			if !isLong && b.Low < ext {
				ext = b.Low
			}
		}
		if dir*(ext-E) > 0 {
			raws = append(raws, rawCand{ext, tfsLabel, "extreme_tfs", 1, atrS})
		}
	}
	if len(hBars) > 0 {
		ext := hBars[0].High
		if !isLong {
			ext = hBars[0].Low
		}
		for _, b := range hBars {
			if isLong && b.High > ext {
				ext = b.High
			}
			if !isLong && b.Low < ext {
				ext = b.Low
			}
		}
		if dir*(ext-E) > 0 {
			raws = append(raws, rawCand{ext, tfhLabel, "extreme_tfh", 1, atrH})
		}
	}
	// Front-run adjustment and the cost+0.25% drop rule.
	type adjCand struct {
		rawCand
		adj float64
	}
	priority := func(c rawCand) int {
		switch {
		case c.method == "wall_tfh":
			return 0
		case c.method == "pivot_tfs" && c.touches >= 2:
			return 1
		case c.method == "extreme_tfh":
			return 2
		case c.method == "extreme_tfs":
			return 3
		default:
			return 4
		}
	}
	var cands []adjCand
	for _, r := range raws {
		adj := r.raw - dir*cfg.FrontRunATRMult*r.atr
		if dir*(adj-E)/E < cfg.TargetDropPct {
			continue
		}
		cands = append(cands, adjCand{r, adj})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].adj != cands[j].adj {
			return dir*cands[i].adj < dir*cands[j].adj
		}
		return priority(cands[i].rawCand) < priority(cands[j].rawCand)
	})
	// Dedupe exact adjusted-price duplicates, keeping the
	// highest-priority source (Appendix F: dedup rule NOT SPECIFIED;
	// no minimum spacing is invented).
	dedup := cands[:0]
	for i, c := range cands {
		if i > 0 && c.adj == cands[i-1].adj {
			continue
		}
		dedup = append(dedup, c)
	}
	cands = dedup

	targets := make([]V7TargetCandidate, len(cands))
	for i, c := range cands {
		targets[i] = V7TargetCandidate{
			ID:       "T" + v7Itoa(i+1),
			Price:    c.adj,
			RawLevel: c.raw,
			SourceTF: c.tf,
			Method:   c.method,
			Touches:  c.touches,
		}
	}
	// Spec v2 V2-2. Only candidates whose adjusted price lies within the
	// class cap are eligible for any role or for the menu; a target beyond
	// the cap is never clamped (a clamped target is the cap, an arithmetic
	// level). Extremes may be TP0 or TP2, never TP1.
	if len(targets) > 0 {
		plan.NearestCandidateDistPct = dir * (targets[0].Price - E) / E * 100
	}
	capPrice := E * (1 + dir*cfg.TPCapPct)
	var eligible []V7TargetCandidate
	for _, t := range targets {
		if dir*(t.Price-capPrice) <= 0 {
			eligible = append(eligible, t)
		}
	}
	isTFH := func(t V7TargetCandidate) bool { return t.Method == "wall_tfh" || t.Method == "extreme_tfh" }
	idxTP0, idxTP1, idxTP2 := -1, -1, -1
	if len(eligible) > 0 {
		idxTP0 = 0
	}
	for i, t := range eligible {
		if t.Method == "wall_tfh" || (t.Method == "pivot_tfs" && t.Touches >= 2) {
			idxTP1 = i
			break
		}
	}
	if idxTP1 < 0 {
		for i, t := range eligible {
			if t.Method == "pivot_tfs" {
				idxTP1 = i
				plan.TP1Fallback = true
				break
			}
		}
	}
	if idxTP1 >= 0 {
		for i := idxTP1 + 1; i < len(eligible); i++ {
			if isTFH(eligible[i]) {
				idxTP2 = i
				break
			}
		}
	}
	if idxTP0 >= 0 {
		eligible[idxTP0].Role = "TP0"
		plan.TP0 = eligible[idxTP0].Price
	}
	if idxTP1 >= 0 {
		eligible[idxTP1].Role = "TP1"
		plan.TP1 = eligible[idxTP1].Price
	}
	if idxTP2 >= 0 {
		eligible[idxTP2].Role = "TP2"
		plan.TP2 = eligible[idxTP2].Price
	}
	// Target menu: at most 4 eligible entries, role holders always included.
	menuIdx := map[int]bool{}
	for i := 0; i < len(eligible) && i < 4; i++ {
		menuIdx[i] = true
	}
	for _, ix := range []int{idxTP0, idxTP1, idxTP2} {
		if ix >= 0 {
			menuIdx[ix] = true
		}
	}
	for len(menuIdx) > 4 {
		best := -1
		for ix := range menuIdx {
			if ix != idxTP0 && ix != idxTP1 && ix != idxTP2 && ix > best {
				best = ix
			}
		}
		if best < 0 {
			break
		}
		delete(menuIdx, best)
	}
	var menu []V7TargetCandidate
	for i, t := range eligible {
		if menuIdx[i] {
			menu = append(menu, t)
		}
	}
	plan.TargetCandidates = menu

	// ---- Stops (§3.3) ---------------------------------------------------
	// Adverse side: pivot lows for LONG, pivot highs for SHORT.
	var aPivots []v7Pivot
	if isLong {
		aPivots = sLows
	} else {
		aPivots = sHighs
	}
	var sStruct float64
	if in.Family == V7FamilyPullbackReversal && in.InvalidationTimeMs > 0 {
		// Reaction leg: bars at/after the invalidation timestamp.
		found := false
		for _, b := range sAll {
			if b.OpenTimeMs >= in.InvalidationTimeMs {
				v := b.Low
				if !isLong {
					v = b.High
				}
				if !found || dir*(v-sStruct) < 0 {
					sStruct, found = v, true
				}
			}
		}
		if !found {
			sStruct = 0
		}
	} else {
		// Most recent confirmed adverse pivot beyond E (in time order).
		for _, p := range aPivots {
			if dir*(E-p.Price) > 0 {
				sStruct = p.Price
			}
		}
	}
	// If the module invalidation price is further away, use it.
	if in.InvalidationPrice > 0 && sStruct > 0 && dir*(sStruct-in.InvalidationPrice) > 0 {
		sStruct = in.InvalidationPrice
	}
	if in.InvalidationPrice > 0 && sStruct == 0 {
		sStruct = in.InvalidationPrice
	}
	plan.SStruct = sStruct

	// Spec v2 V2-3: every listed stop lies beyond S_struct.
	buffer := plan.Buffer
	inBand := func(d float64) bool { return d >= dMin && d <= dMax }
	var stops []V7StopCandidate
	sAIdx := -1
	if sStruct > 0 && dir*(E-sStruct) > 0 {
		priceA := sStruct - dir*buffer
		dA := dir * (E - priceA) / E
		idA, anchorA := "S_A", "structural anchor "+ftoa(sStruct)+" minus buffer"
		dFinal := dA
		if dA < dMin {
			// Pushed outward to the noise floor, never inward. dFinal is
			// set to d_min exactly so float rounding of the recomputed
			// distance cannot drop the floored stop out of the band.
			priceA, idA, dFinal = E*(1-dir*dMin), "S_A_floor", dMin
			anchorA = "structural anchor " + ftoa(sStruct) + " pushed out to d_min"
		}
		if inBand(dFinal) {
			c := V7StopCandidate{ID: idA, Price: priceA, DistPct: dFinal * 100, Anchor: anchorA}
			stops = append(stops, c)
			sAIdx = len(stops) - 1
		}
		// S_B: next TF_s pivot beyond S_struct, minus buffer (menu only).
		var next float64
		for _, p := range aPivots {
			if dir*(sStruct-p.Price) > 0 && (next == 0 || dir*(p.Price-next) > 0) {
				next = p.Price
			}
		}
		if next > 0 {
			priceB := next - dir*buffer
			if dB := dir * (E - priceB) / E; inBand(dB) {
				stops = append(stops, V7StopCandidate{
					ID: "S_B", Price: priceB, DistPct: dB * 100,
					Anchor: "next TF_s pivot beyond anchor " + ftoa(next) + " minus buffer",
				})
			}
		}
		// S_C = E*(1 - dir*(max(d_min, d_A) + slack)), listed only if it
		// lies beyond S_struct and differs from S_A (menu only).
		dC := math.Max(dMin, dA) + cfg.SCSlackPct
		priceC := E * (1 - dir*dC)
		if inBand(dC) && dir*(sStruct-priceC) > 0 && (sAIdx < 0 || priceC != stops[sAIdx].Price) {
			stops = append(stops, V7StopCandidate{
				ID: "S_C", Price: priceC, DistPct: dC * 100,
				Anchor: "max(d_min, d_A) + slack from E",
			})
		}
	}
	plan.StopCandidates = stops

	// ---- Admission (spec v2 V2-4) --------------------------------------
	if idxTP1 < 0 {
		plan.Reason = "rr_insufficient"
		switch {
		case len(targets) == 0:
			plan.RejectDetail = "no_candidate_in_direction"
		case len(eligible) == 0:
			plan.RejectDetail = "nearest_candidate_beyond_cap"
		default:
			plan.RejectDetail = "no_tp1_eligible_within_cap"
		}
		return plan
	}
	if sAIdx < 0 {
		plan.Reason = "backend_guard_risk"
		if sStruct <= 0 || dir*(E-sStruct) <= 0 {
			plan.RejectDetail = "no_structural_anchor"
		} else {
			plan.RejectDetail = "no_stop_in_band"
		}
		return plan
	}
	fee := E * cfg.CostPct
	sA := stops[sAIdx]
	rr := (dir*(plan.TP1-E) - fee) / (dir*(E-sA.Price) + fee)
	plan.RRMeasTP1 = rr
	if rr < cfg.RRMin {
		plan.Reason = "backend_guard_risk"
		plan.RejectDetail = "s_a_fails_rr"
		return plan
	}
	chosen := sA
	plan.ChosenStop = &chosen
	plan.Feasible = true
	return plan
}

// v7TFLabel renders a bar interval as the exchange-style label.
func v7TFLabel(ms int64) string {
	switch {
	case ms > 0 && ms%(24*3600*1000) == 0:
		return v7Itoa(int(ms/(24*3600*1000))) + "d"
	case ms > 0 && ms%(3600*1000) == 0:
		return v7Itoa(int(ms/(3600*1000))) + "h"
	default:
		return v7Itoa(int(ms/60000)) + "m"
	}
}

// V7StructureSize implements §3.6 sizing: a wider structural stop gives
// a proportionally smaller quantity at constant USD risk.
//
//	risk_usd = equity_day_start * risk_frac
//	fee      = E * cost_pct
//	qty_risk = risk_usd / (|E - SL| + fee)
//	qty_cap  = equity * 1.0 / E
//	qty      = floor_step(min(qty_risk, qty_cap, qty_llm))
//
// qtyLLM <= 0 means "no LLM cap". step <= 0 means "no lot rounding".
func V7StructureSize(equityDayStart, riskFrac, equity, entry, stop, step, qtyLLM float64, cfg V7StructureConfig) float64 {
	cfg = cfg.withDefaults()
	if entry <= 0 || stop <= 0 || equityDayStart <= 0 || riskFrac <= 0 {
		return 0
	}
	fee := entry * cfg.CostPct
	riskUSD := equityDayStart * riskFrac
	denom := math.Abs(entry-stop) + fee
	if denom <= 0 {
		return 0
	}
	q := riskUSD / denom
	if equity > 0 {
		if cap := equity * 1.0 / entry; cap < q {
			q = cap
		}
	}
	if qtyLLM > 0 && qtyLLM < q {
		q = qtyLLM
	}
	if step > 0 {
		q = math.Floor(q/step) * step
	}
	if q < 0 {
		return 0
	}
	return q
}

// ---------------------------------------------------------------------------
// geometry_mode flag (plan §5, M2 row) and TP-plan integration
// ---------------------------------------------------------------------------

// V7GeometryMode selects the geometry source of the take-profit plan
// path. The default is legacy: existing behaviour is unchanged unless
// the mode is explicitly set to structure (and structure bars are
// supplied on the symbol context).
type V7GeometryMode string

const (
	V7GeometryModeLegacy    V7GeometryMode = "legacy"
	V7GeometryModeStructure V7GeometryMode = "structure"
)

var v7GeometryMode atomic.Value // stores V7GeometryMode

func init() { v7GeometryMode.Store(V7GeometryModeLegacy) }

// V7CurrentGeometryMode returns the process-wide geometry mode.
func V7CurrentGeometryMode() V7GeometryMode {
	if v, ok := v7GeometryMode.Load().(V7GeometryMode); ok {
		return v
	}
	return V7GeometryModeLegacy
}

// SetV7GeometryMode sets the process-wide geometry mode. Unknown
// values are ignored (the mode stays whatever it was).
func SetV7GeometryMode(m V7GeometryMode) {
	if m == V7GeometryModeLegacy || m == V7GeometryModeStructure {
		v7GeometryMode.Store(m)
	}
}

// v7ResolveGeometryMode applies the per-context override when present.
func v7ResolveGeometryMode(ctx *V7SymbolContext) V7GeometryMode {
	if ctx != nil {
		switch V7GeometryMode(ctx.GeometryMode) {
		case V7GeometryModeLegacy, V7GeometryModeStructure:
			return V7GeometryMode(ctx.GeometryMode)
		}
	}
	return V7CurrentGeometryMode()
}

// v7StructureFamilyForSetup maps a setup type to the §3.3 anchor rule:
// pullback/reversal families use the reaction-leg rule, everything
// else the breakout/continuation pivot rule.
func v7StructureFamilyForSetup(setup V7SetupType) V7StructureFamilyClass {
	s := strings.ToLower(string(setup))
	if strings.Contains(s, "pullback") || strings.Contains(s, "reversal") {
		return V7FamilyPullbackReversal
	}
	return V7FamilyBreakoutContinuation
}

// applyV7StructurePlan is the structure branch of
// ApplyV7TakeProfitPlan. It returns false when structure mode is not
// active or no structure bars were supplied (caller then runs the
// legacy path unchanged). When structure mode is active it returns
// true: a feasible plan rewrites the TP prices, the invalidation
// (chosen structural stop) and the take-profit plan; an infeasible
// plan leaves TPPlan nil (no plan, no trade).
func applyV7StructurePlan(sig *V7SignalOutput, ctx *V7SymbolContext, entry float64) bool {
	if sig == nil || ctx == nil || entry <= 0 {
		return false
	}
	if v7ResolveGeometryMode(ctx) != V7GeometryModeStructure {
		return false
	}
	cfg := V7StructureConfigForSetup(string(sig.SetupType))
	barsS, barsH := ctx.StructureBars15m, ctx.StructureBars1h
	if cfg.Class == "SWING" {
		barsS, barsH = ctx.StructureBars1h, ctx.StructureBars4h
	}
	if len(barsS) == 0 || len(barsH) == 0 {
		return false
	}
	last := barsS[len(barsS)-1]
	decisionMs := last.OpenTimeMs + cfg.TFSIntervalMs
	plan := BuildV7StructurePlan(V7StructureInput{
		Direction:         sig.Direction,
		EntryPrice:        entry,
		DecisionTimeMs:    decisionMs,
		BarsTFS:           barsS,
		BarsTFH:           barsH,
		Family:            v7StructureFamilyForSetup(sig.SetupType),
		InvalidationPrice: sig.Invalidation.Price,
		Config:            cfg,
	})
	if !plan.Feasible || plan.ChosenStop == nil {
		sig.TPPlan = nil
		return true
	}
	stop := plan.ChosenStop.Price
	sig.Invalidation.Price = stop
	sig.Invalidation.Reason = "structure_stop_" + strings.ToLower(plan.ChosenStop.ID)
	risk := math.Abs(entry - stop)
	rrOf := func(tp float64) float64 {
		if tp <= 0 || risk <= 0 {
			return 0
		}
		return math.Abs(tp-entry) / risk
	}
	idOf := func(role string) string {
		for _, t := range plan.TargetCandidates {
			if t.Role == role {
				return t.ID
			}
		}
		return ""
	}
	if plan.TP0 > 0 {
		sig.TP0Price = plan.TP0
		sig.TP0RR = rrOf(plan.TP0)
		sig.TP0TimeWindow = "5m-30m"
		sig.TP0Method = "structure_" + idOf("TP0")
	}
	if plan.TP1 > 0 {
		sig.TP1Price = plan.TP1
		sig.TP1RR = rrOf(plan.TP1)
		sig.TP1TimeWindow = "1h-4h"
		sig.TP1Method = "structure_" + idOf("TP1")
	}
	if plan.TP2 > 0 {
		sig.TP2Price = plan.TP2
		sig.TP2RR = rrOf(plan.TP2)
		sig.TP2TimeWindow = "4h-24h"
		sig.TP2Method = "structure_" + idOf("TP2")
	}
	targets := []V7Target{{Price: plan.TP1, Reason: "structure_tp1_" + idOf("TP1")}}
	if plan.TP2 > 0 {
		targets = append(targets, V7Target{Price: plan.TP2, Reason: "structure_tp2_" + idOf("TP2")})
	}
	sig.Targets = targets
	sig.TPPlan = &V7TakeProfitPlan{
		TP0Price:            plan.TP0,
		TP0DistancePct:      math.Abs(pctDiff(plan.TP0, entry)),
		TP0ReducePctMin:     30,
		TP0ReducePctMax:     50,
		MoveStopToBreakeven: true,
		TrailingStopMode:    "after_tp0_partial",
		TrailingBasis:       []string{"structure_levels"},
		StatsBucket:         "tp0_tp1_no_sl_structure",
	}
	return true
}

// v7Itoa is a tiny integer formatter kept local to this file.
func v7Itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [8]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
