package store

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
)

// U12: per class × cell × regime dashboard tables (redesign plan §6.2
// "Observability", §5.2, §5.3, §2.4 drift monitor). Read-only: nothing here
// feeds admission, tiering or sizing. Owner decisions (U12):
//   - cell status is the §5.3 rule evaluated on the selected window, shown
//     next to the stamped tag, with no hysteresis;
//   - outcomes default to prompt_open_review, with the source as a filter;
//   - calibration drift is a reliability table per calibration_version over
//     the current and the previous window of the same length.

// Evaluated cell statuses (§5.3). V7CellStatusProbation is in
// hunter_v7_outcome_tags.go.
const (
	V7CellStatusEarned  = "earned"
	V7CellStatusBlocked = "blocked"
)

// Dashboard outcome-source filter values.
const (
	V7CellSourceAll = "all"
	// V7CellSourceDefault is the default filter (owner decision 3).
	V7CellSourceDefault = V7OutcomeSourcePromptOpenReview
)

// Dashboard group labels for empty tag values.
const (
	V7CellRegimeUnknown = "unknown"
)

// V7CellRuleConfig is HunterConfig.V7CellRule (json v7_cell_rule): the §5.3
// rule parameters (OD-8) and the dashboard's bootstrap and quantile
// settings. Zero values mean the defaults below.
type V7CellRuleConfig struct {
	// MinSamples is N_min: a cell is EARNED or BLOCKED only at n >= N_min.
	MinSamples int `json:"min_samples,omitempty"`
	// CILevel is the two-sided bootstrap CI level of mean fee-net R.
	CILevel            float64 `json:"ci_level,omitempty"`
	BootstrapResamples int     `json:"bootstrap_resamples,omitempty"`
	Seed               int64   `json:"seed,omitempty"`
	// Quantiles is the number of expected_r groups in the drift table.
	Quantiles int `json:"quantiles,omitempty"`
}

// Defaults: OD-8 recommendation (N_min 30, 90% bootstrap); resamples,
// seed and quantiles as in the U7 falsification protocol.
const (
	DefaultV7CellMinSamples         = 30
	DefaultV7CellCILevel            = 0.90
	DefaultV7CellBootstrapResamples = DefaultV7ExpectedRBootstrapResamples
	DefaultV7CellSeed               = DefaultV7ExpectedRSeed
	DefaultV7CellQuantiles          = DefaultV7ExpectedRQuantiles
)

// EffectiveV7CellRule returns the config with defaults filled in.
func EffectiveV7CellRule(c *V7CellRuleConfig) V7CellRuleConfig {
	out := V7CellRuleConfig{}
	if c != nil {
		out = *c
	}
	if out.MinSamples <= 0 {
		out.MinSamples = DefaultV7CellMinSamples
	}
	if out.CILevel <= 0 || out.CILevel >= 1 {
		out.CILevel = DefaultV7CellCILevel
	}
	if out.BootstrapResamples <= 0 {
		out.BootstrapResamples = DefaultV7CellBootstrapResamples
	}
	if out.Seed == 0 {
		out.Seed = DefaultV7CellSeed
	}
	if out.Quantiles <= 0 {
		out.Quantiles = DefaultV7CellQuantiles
	}
	return out
}

// V7BootstrapMeanCI is the percentile bootstrap CI of the mean of xs:
// resamples means of len(xs) draws with replacement from rng, sorted; the
// bounds are the (1-ci)/2 and 1-(1-ci)/2 order statistics. ok is false for
// an empty sample. This is the U7 F-2 computation (cmd/t2_u7_falsify),
// moved here so the dashboard and the protocol share one implementation;
// draws consume rng in the same order.
func V7BootstrapMeanCI(xs []float64, resamples int, ci float64, rng *rand.Rand) (lo, hi float64, ok bool) {
	if len(xs) == 0 || resamples <= 0 {
		return 0, 0, false
	}
	means := make([]float64, resamples)
	for s := range means {
		sum := 0.0
		for range xs {
			sum += xs[rng.Intn(len(xs))]
		}
		means[s] = sum / float64(len(xs))
	}
	sort.Float64s(means)
	tail := (1 - ci) / 2
	li := int(math.Round(float64(resamples) * tail))
	hi1 := int(math.Round(float64(resamples)*(1-tail))) - 1
	if li < 0 {
		li = 0
	}
	if hi1 >= resamples {
		hi1 = resamples - 1
	}
	if hi1 < li {
		hi1 = li
	}
	return means[li], means[hi1], true
}

// V7EvaluateCellStatus is the §5.3 rule on one evaluation (no hysteresis).
func V7EvaluateCellStatus(n int, lo, hi float64, ciOK bool, minSamples int) string {
	switch {
	case ciOK && n >= minSamples && lo > 0:
		return V7CellStatusEarned
	case ciOK && n >= minSamples && hi < 0:
		return V7CellStatusBlocked
	default:
		return V7CellStatusProbation
	}
}

// V7CellRow is one dashboard row. Class, RRBand and Regime are "" on the
// levels that pool them.
type V7CellRow struct {
	Class   string  `json:"signal_class"`
	RRBand  string  `json:"rr_band,omitempty"`
	Regime  string  `json:"market_regime,omitempty"`
	N       int     `json:"n"`
	TP1     int     `json:"tp1"`
	Stop    int     `json:"stop"`
	None    int     `json:"none"`
	TP1Rate float64 `json:"tp1_first_rate"`
	MeanR   float64 `json:"mean_fee_net_r"`
	CILo    float64 `json:"ci_lo"`
	CIHi    float64 `json:"ci_hi"`
	// StampedStatus lists the cell_status values stamped on the rows
	// (sorted, distinct); EvaluatedStatus is the §5.3 rule on this window.
	StampedStatus   []string `json:"stamped_status"`
	EvaluatedStatus string   `json:"evaluated_status"`
}

// V7CellTables is the class, cell (class × RR band) and cell × regime
// breakdown of fee-net outcomes.
type V7CellTables struct {
	Rows        int         `json:"rows"`
	Classes     []V7CellRow `json:"classes"`
	Cells       []V7CellRow `json:"cells"`
	CellRegimes []V7CellRow `json:"cell_regimes"`
}

// V7CellOutcomeExclusions counts resolved rows without a fee-net R
// (degenerate geometry, no anchor) by outcome; they are not in the tables.
type V7CellOutcomeExclusions map[string]int

// V7CellSourceMatches reports whether a record's outcome source passes the
// dashboard filter.
func V7CellSourceMatches(recordSource, filter string) bool {
	return filter == V7CellSourceAll || recordSource == filter
}

// ValidV7CellSource reports whether a filter value is accepted.
func ValidV7CellSource(filter string) bool {
	switch filter {
	case V7CellSourceAll, V7OutcomeSourcePromptOpenReview, V7OutcomeSourceWatchAudit, V7OutcomeSourceShadow:
		return true
	}
	return false
}

func v7CellClass(r HunterV7SignalRecord) string {
	if r.SignalClass == "" {
		return V7ClassUnassigned
	}
	return r.SignalClass
}

func v7CellBand(r HunterV7SignalRecord) string {
	if r.RRBand == "" {
		return V7BandUnknown
	}
	return r.RRBand
}

func v7CellRegime(r HunterV7SignalRecord) string {
	if r.MarketRegime == "" {
		return V7CellRegimeUnknown
	}
	return r.MarketRegime
}

// v7BandOrder sorts band labels by their lower edge: lt_x first, then a_b
// by a, ge_x by x, unknown last.
func v7BandOrder(label string) float64 {
	switch {
	case label == V7BandUnknown:
		return math.Inf(1)
	case strings.HasPrefix(label, "lt_"):
		return math.Inf(-1)
	case strings.HasPrefix(label, "ge_"):
		v, err := strconv.ParseFloat(strings.TrimPrefix(label, "ge_"), 64)
		if err == nil {
			return v
		}
	default:
		if i := strings.Index(label, "_"); i > 0 {
			if v, err := strconv.ParseFloat(label[:i], 64); err == nil {
				return v
			}
		}
	}
	return math.Inf(1)
}

// BuildV7CellTables builds the tables from records that carry a fee-net R
// and pass the source filter (rows without one are skipped). Each row's
// bootstrap uses a fresh generator seeded with cfg.Seed, so a row's CI
// does not depend on the other rows.
func BuildV7CellTables(records []HunterV7SignalRecord, source string, cfg V7CellRuleConfig) V7CellTables {
	type acc struct {
		row     V7CellRow
		rs      []float64
		stamped map[string]bool
	}
	groups := map[string]*acc{}
	add := func(key string, row V7CellRow, r HunterV7SignalRecord) {
		a, ok := groups[key]
		if !ok {
			a = &acc{row: row, stamped: map[string]bool{}}
			groups[key] = a
		}
		a.rs = append(a.rs, *r.FeeNetR)
		switch r.FeeNetOutcome {
		case V7FeeNetTP1:
			a.row.TP1++
		case V7FeeNetStop:
			a.row.Stop++
		case V7FeeNetNone:
			a.row.None++
		}
		a.stamped[r.CellStatus] = true
	}
	out := V7CellTables{}
	for _, r := range records {
		if r.FeeNetR == nil || !V7CellSourceMatches(r.OutcomeSource, source) {
			continue
		}
		out.Rows++
		c, b, g := v7CellClass(r), v7CellBand(r), v7CellRegime(r)
		add("c\x00"+c, V7CellRow{Class: c}, r)
		add("b\x00"+c+"\x00"+b, V7CellRow{Class: c, RRBand: b}, r)
		add("r\x00"+c+"\x00"+b+"\x00"+g, V7CellRow{Class: c, RRBand: b, Regime: g}, r)
	}
	for key, a := range groups {
		row := a.row
		row.N = len(a.rs)
		sum := 0.0
		for _, x := range a.rs {
			sum += x
		}
		row.MeanR = sum / float64(row.N)
		row.TP1Rate = float64(row.TP1) / float64(row.N)
		lo, hi, ok := V7BootstrapMeanCI(a.rs, cfg.BootstrapResamples, cfg.CILevel, rand.New(rand.NewSource(cfg.Seed)))
		row.CILo, row.CIHi = lo, hi
		row.EvaluatedStatus = V7EvaluateCellStatus(row.N, lo, hi, ok, cfg.MinSamples)
		row.StampedStatus = []string{}
		for s := range a.stamped {
			if s == "" {
				s = "none"
			}
			row.StampedStatus = append(row.StampedStatus, s)
		}
		sort.Strings(row.StampedStatus)
		switch key[0] {
		case 'c':
			out.Classes = append(out.Classes, row)
		case 'b':
			out.Cells = append(out.Cells, row)
		default:
			out.CellRegimes = append(out.CellRegimes, row)
		}
	}
	less := func(rows []V7CellRow) func(i, j int) bool {
		return func(i, j int) bool {
			a, b := rows[i], rows[j]
			if a.Class != b.Class {
				return a.Class < b.Class
			}
			if a.RRBand != b.RRBand {
				oa, ob := v7BandOrder(a.RRBand), v7BandOrder(b.RRBand)
				if oa != ob {
					return oa < ob
				}
				return a.RRBand < b.RRBand
			}
			return a.Regime < b.Regime
		}
	}
	for _, rows := range [][]V7CellRow{out.Classes, out.Cells, out.CellRegimes} {
		sort.Slice(rows, less(rows))
	}
	if out.Classes == nil {
		out.Classes, out.Cells, out.CellRegimes = []V7CellRow{}, []V7CellRow{}, []V7CellRow{}
	}
	return out
}

// V7CellExclusions counts filtered rows with a resolved outcome but no
// fee-net R.
func V7CellExclusions(records []HunterV7SignalRecord, source string) V7CellOutcomeExclusions {
	out := V7CellOutcomeExclusions{}
	for _, r := range records {
		if r.FeeNetR != nil || r.FeeNetOutcome == "" || !V7CellSourceMatches(r.OutcomeSource, source) {
			continue
		}
		out[r.FeeNetOutcome]++
	}
	return out
}

// ---------------------------------------------------------------------------
// Calibration drift (§2.4 drift monitor)
// ---------------------------------------------------------------------------

// V7CalibrationQuantile is one expected_r group of the reliability table.
type V7CalibrationQuantile struct {
	Q        int     `json:"quantile"`
	N        int     `json:"n"`
	PredMean float64 `json:"pred_mean"`
	RealMean float64 `json:"realized_mean"`
	CILo     float64 `json:"ci_lo"`
	CIHi     float64 `json:"ci_hi"`
	Inside   bool    `json:"inside"`
}

// V7CalibrationWindow is the reliability table of one window.
type V7CalibrationWindow struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	N    int       `json:"n"`
	// Sufficient is false when the window has fewer rows than quantiles
	// (no table is built).
	Sufficient bool                    `json:"sufficient"`
	Outside    int                     `json:"quantiles_outside"`
	Quantiles  []V7CalibrationQuantile `json:"quantiles"`
}

// V7CalibrationDrift is the drift entry of one calibration_version: drift
// is flagged when both windows are sufficient and each has at least one
// quantile whose predicted mean lies outside the realized mean's CI.
type V7CalibrationDrift struct {
	Version  string              `json:"calibration_version"`
	Current  V7CalibrationWindow `json:"current"`
	Previous V7CalibrationWindow `json:"previous"`
	Drift    bool                `json:"drift"`
}

func v7CalibrationWindow(rows []HunterV7SignalRecord, from, to time.Time, cfg V7CellRuleConfig) V7CalibrationWindow {
	w := V7CalibrationWindow{From: from, To: to, N: len(rows), Quantiles: []V7CalibrationQuantile{}}
	if len(rows) < cfg.Quantiles {
		return w
	}
	w.Sufficient = true
	sorted := append([]HunterV7SignalRecord(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if *a.ExpectedR != *b.ExpectedR {
			return *a.ExpectedR < *b.ExpectedR
		}
		if !a.Timestamp.Equal(b.Timestamp) {
			return a.Timestamp.Before(b.Timestamp)
		}
		return a.ID < b.ID
	})
	groups := make([][]HunterV7SignalRecord, cfg.Quantiles)
	for i, r := range sorted {
		q := i * cfg.Quantiles / len(sorted)
		groups[q] = append(groups[q], r)
	}
	rng := rand.New(rand.NewSource(cfg.Seed))
	for q, g := range groups {
		pred, real := make([]float64, len(g)), make([]float64, len(g))
		for i, r := range g {
			pred[i], real[i] = *r.ExpectedR, *r.FeeNetR
		}
		lo, hi, _ := V7BootstrapMeanCI(real, cfg.BootstrapResamples, cfg.CILevel, rng)
		qo := V7CalibrationQuantile{Q: q + 1, N: len(g), PredMean: v7Mean(pred), RealMean: v7Mean(real), CILo: lo, CIHi: hi}
		qo.Inside = qo.PredMean >= qo.CILo && qo.PredMean <= qo.CIHi
		if !qo.Inside {
			w.Outside++
		}
		w.Quantiles = append(w.Quantiles, qo)
	}
	return w
}

func v7Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// BuildV7CalibrationDrift builds one entry per calibration_version from
// records with both expected_r and fee-net R that pass the source filter:
// the previous window is [prevFrom, from), the current one [from, to].
func BuildV7CalibrationDrift(records []HunterV7SignalRecord, prevFrom, from, to time.Time, source string, cfg V7CellRuleConfig) []V7CalibrationDrift {
	cur, prev := map[string][]HunterV7SignalRecord{}, map[string][]HunterV7SignalRecord{}
	versions := map[string]bool{}
	for _, r := range records {
		if r.ExpectedR == nil || r.FeeNetR == nil || !V7CellSourceMatches(r.OutcomeSource, source) {
			continue
		}
		v := r.CalibrationVersion
		switch {
		case !r.Timestamp.Before(from) && !r.Timestamp.After(to):
			cur[v] = append(cur[v], r)
		case !r.Timestamp.Before(prevFrom) && r.Timestamp.Before(from):
			prev[v] = append(prev[v], r)
		default:
			continue
		}
		versions[v] = true
	}
	names := make([]string, 0, len(versions))
	for v := range versions {
		names = append(names, v)
	}
	sort.Strings(names)
	out := []V7CalibrationDrift{}
	for _, v := range names {
		d := V7CalibrationDrift{
			Version:  v,
			Current:  v7CalibrationWindow(cur[v], from, to, cfg),
			Previous: v7CalibrationWindow(prev[v], prevFrom, from, cfg),
		}
		d.Drift = d.Current.Sufficient && d.Previous.Sufficient && d.Current.Outside > 0 && d.Previous.Outside > 0
		out = append(out, d)
	}
	return out
}

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

// CellSource returns the records with a resolved fee-net outcome in
// [from, to] (timestamp = signal time), with only the columns the
// dashboard reads.
func (s *HunterV7SignalStore) CellSource(from, to time.Time) ([]HunterV7SignalRecord, error) {
	if !to.After(from) {
		return nil, fmt.Errorf("cell source: empty window")
	}
	var recs []HunterV7SignalRecord
	err := s.db.Select("id", "timestamp", "signal_class", "rr_band", "market_regime", "cell_status",
		"calibration_version", "fee_net_r", "fee_net_outcome", "outcome_source", "expected_r").
		Where("timestamp >= ? AND timestamp <= ? AND COALESCE(fee_net_outcome, '') <> ''", from, to).
		Order("timestamp, id").Find(&recs).Error
	return recs, err
}
