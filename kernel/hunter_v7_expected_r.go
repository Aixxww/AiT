package kernel

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/Aixxww/AiT/store"
)

// U7: the E[R] estimator of redesign plan §2.4, in shadow. It is stamped on
// candidates and persisted with the records; no tier, ranking, prompt or
// execution path reads it.
//
//	E[R] = p_tp1 * R_win - p_stop * 1 - c
//
// p_tp1 and p_stop are empirical-Bayes shrunk rates per cell (family x
// direction x RR band), shrunk toward the class rate, which is shrunk toward
// the global rate, with weights n/(n+k); k is chosen by leave-one-fold-out.

// HunterV7ERCounts are the outcome counts of one level of the hierarchy.
type HunterV7ERCounts struct {
	N    int `json:"n"`
	TP1  int `json:"tp1"`
	Stop int `json:"stop"`
}

func (c *HunterV7ERCounts) add(outcome string) {
	c.N++
	switch outcome {
	case store.V7FeeNetTP1:
		c.TP1++
	case store.V7FeeNetStop:
		c.Stop++
	}
}

// HunterV7ERCalibration is one fitted estimator.
type HunterV7ERCalibration struct {
	Version        string                      `json:"version"`
	FittedAt       time.Time                   `json:"fitted_at"`
	K              float64                     `json:"k"`
	PoolDirections bool                        `json:"pool_directions"`
	Z              float64                     `json:"z"`
	ExtraCostR     float64                     `json:"extra_cost_r"`
	Global         HunterV7ERCounts            `json:"global"`
	Class          map[string]HunterV7ERCounts `json:"class"`
	Cell           map[string]HunterV7ERCounts `json:"cell"`
}

// HunterV7ERInput is what the estimator needs about one signal.
type HunterV7ERInput struct {
	Class, Family, Direction, RRBand string
	RWin                             float64 // planned fee-net R at TP1
}

// HunterV7ERCellKey is the cell key of the hierarchy's lowest level.
func HunterV7ERCellKey(family, direction, rrBand string, poolDirections bool) string {
	if poolDirections {
		return family + "|" + rrBand
	}
	return family + "|" + direction + "|" + rrBand
}

func hunterV7ERRowInput(r store.V7CalibrationOutcome) HunterV7ERInput {
	fam := r.Family
	if fam == "" {
		fam = store.V7Family(r.SetupType)
	}
	return HunterV7ERInput{Class: r.Class, Family: fam, Direction: r.Direction, RRBand: r.RRBand}
}

// hunterV7ERZ is the one-sided normal quantile of a confidence level.
func hunterV7ERZ(confidence float64) float64 {
	return math.Sqrt2 * math.Erfinv(2*confidence-1)
}

func hunterV7ERCount(rows []store.V7CalibrationOutcome, k float64, cfg store.V7ExpectedRConfig) *HunterV7ERCalibration {
	cal := &HunterV7ERCalibration{K: k, PoolDirections: cfg.PoolDirections, Z: hunterV7ERZ(cfg.LowerBoundConfidence),
		ExtraCostR: cfg.ExtraCostR, Class: map[string]HunterV7ERCounts{}, Cell: map[string]HunterV7ERCounts{}}
	for _, r := range rows {
		in := hunterV7ERRowInput(r)
		cal.Global.add(r.Outcome)
		c := cal.Class[in.Class]
		c.add(r.Outcome)
		cal.Class[in.Class] = c
		key := HunterV7ERCellKey(in.Family, in.Direction, in.RRBand, cfg.PoolDirections)
		cc := cal.Cell[key]
		cc.add(r.Outcome)
		cal.Cell[key] = cc
	}
	return cal
}

// Rates returns the shrunk (p_tp1, p_stop) of a signal and its effective n.
func (c *HunterV7ERCalibration) Rates(in HunterV7ERInput) (pTP1, pStop, nEff float64) {
	if c == nil || c.Global.N == 0 {
		return 0, 0, 0
	}
	shrink := func(n, x int, prior float64) float64 { return (float64(x) + c.K*prior) / (float64(n) + c.K) }
	gT, gS := float64(c.Global.TP1)/float64(c.Global.N), float64(c.Global.Stop)/float64(c.Global.N)
	cl := c.Class[in.Class]
	cT, cS := shrink(cl.N, cl.TP1, gT), shrink(cl.N, cl.Stop, gS)
	ce := c.Cell[HunterV7ERCellKey(in.Family, in.Direction, in.RRBand, c.PoolDirections)]
	return shrink(ce.N, ce.TP1, cT), shrink(ce.N, ce.Stop, cS), float64(ce.N) + c.K
}

// Estimate returns E[R] and its lower bound for one signal.
func (c *HunterV7ERCalibration) Estimate(in HunterV7ERInput) (er, lower float64, ok bool) {
	pT, pS, nEff := c.Rates(in)
	if nEff <= 0 {
		return 0, 0, false
	}
	mean := pT*in.RWin - pS
	variance := pT*in.RWin*in.RWin + pS - mean*mean
	if variance < 0 {
		variance = 0
	}
	er = mean - c.ExtraCostR
	return er, er - c.Z*math.Sqrt(variance/nEff), true
}

// HunterV7ERFolds splits rows into time tertiles (OD-2 fold rule); fold of
// row i in time order is 3*i/n.
func HunterV7ERFolds(rows []store.V7CalibrationOutcome) [3][]store.V7CalibrationOutcome {
	idx := make([]int, len(rows))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return rows[idx[a]].Time.Before(rows[idx[b]].Time) })
	var f [3][]store.V7CalibrationOutcome
	for k, i := range idx {
		f[k*3/len(rows)] = append(f[k*3/len(rows)], rows[i])
	}
	return f
}

// hunterV7ERRealizedR is the OD-2 R of a calibration row given its R_win.
func hunterV7ERRealizedR(outcome string, rWin float64) float64 {
	switch outcome {
	case store.V7FeeNetTP1:
		return rWin
	case store.V7FeeNetStop:
		return -1
	}
	return 0
}

// HunterV7ERChooseK picks k from the grid by leave-one-fold-out squared
// error of E[R] against realized R over the given folds (first minimum in
// grid order). Folds without rows are skipped as test folds.
func HunterV7ERChooseK(folds [][]store.V7CalibrationOutcome, cfg store.V7ExpectedRConfig) float64 {
	best, bestK := math.Inf(1), cfg.KGrid[0]
	for _, k := range cfg.KGrid {
		sse := 0.0
		for t := range folds {
			var train []store.V7CalibrationOutcome
			for o := range folds {
				if o != t {
					train = append(train, folds[o]...)
				}
			}
			if len(train) == 0 {
				continue
			}
			cal := hunterV7ERCount(train, k, cfg)
			for _, r := range folds[t] {
				in := hunterV7ERRowInput(r)
				in.RWin = r.RWin
				er, _, _ := cal.Estimate(in)
				d := er - hunterV7ERRealizedR(r.Outcome, r.RWin)
				sse += d * d
			}
		}
		if sse < best {
			best, bestK = sse, k
		}
	}
	return bestK
}

// FitHunterV7ExpectedR fits the estimator with a given k.
func FitHunterV7ExpectedR(rows []store.V7CalibrationOutcome, k float64, cfg store.V7ExpectedRConfig, fittedAt time.Time) *HunterV7ERCalibration {
	cal := hunterV7ERCount(rows, k, cfg)
	cal.FittedAt = fittedAt
	cal.Version = fmt.Sprintf("er-v1:%s:n%d:k%g", fittedAt.UTC().Format("20060102T150405Z"), cal.Global.N, k)
	return cal
}

// FitHunterV7ExpectedRFromOutcomes is the runtime fit: below MinRows it
// returns nil (no calibration); k is chosen by leave-one-fold-out over the
// rows' time tertiles.
func FitHunterV7ExpectedRFromOutcomes(rows []store.V7CalibrationOutcome, cfg store.V7ExpectedRConfig, fittedAt time.Time) *HunterV7ERCalibration {
	if len(rows) < cfg.MinRows {
		return nil
	}
	tert := HunterV7ERFolds(rows)
	return FitHunterV7ExpectedR(rows, HunterV7ERChooseK(tert[:], cfg), cfg, fittedAt)
}

// hunterV7ERState holds the engine's current shadow calibration.
type hunterV7ERState struct {
	mu  sync.RWMutex
	cal *HunterV7ERCalibration
}

func (s *hunterV7ERState) get() *HunterV7ERCalibration {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cal
}

func (s *hunterV7ERState) set(c *HunterV7ERCalibration) {
	s.mu.Lock()
	s.cal = c
	s.mu.Unlock()
}

// HunterV7StampExpectedR stamps the shadow E[R], its lower bound and the
// calibration version on candidates (U7). It changes no other field and is
// read by no gate.
func HunterV7StampExpectedR(cands []CandidateCoin, cal *HunterV7ERCalibration, tags store.V7OutcomeTagConfig) {
	if cal == nil {
		return
	}
	for i := range cands {
		cc := &cands[i]
		rwin, ok := hunterV7DecisionRWin(*cc, tags.FeeNetCostPct)
		if !ok {
			continue
		}
		in := HunterV7ERInput{Class: cc.V7SignalClass, Family: store.V7Family(cc.V7SetupType), Direction: cc.Direction,
			RRBand: store.V7BandLabel(rwin, true, tags.RRBandEdges), RWin: rwin}
		if in.Class == "" {
			in.Class = store.V7ClassUnassigned
		}
		er, lb, ok := cal.Estimate(in)
		if !ok {
			continue
		}
		cc.V7ExpectedR, cc.V7ExpectedRLower, cc.V7CalibrationVersion = &er, &lb, cal.Version
	}
}

// hunterV7DecisionRWin is the decision-time planned fee-net R at TP1, from
// the same reference price the RR band uses.
func hunterV7DecisionRWin(cc CandidateCoin, costPct float64) (float64, bool) {
	tp1 := cc.V7TP1Price
	if tp1 <= 0 && len(cc.V7Targets) > 0 {
		tp1 = cc.V7Targets[0].Price
	}
	return store.V7FeeNetRRAtTP1(hunterV7ReferencePrice(cc), cc.V7Invalidation.Price, tp1, costPct)
}
