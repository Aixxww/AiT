package store

import "time"

// U7: E[R] estimator in shadow (redesign plan §2.4). The estimate is
// computed, stamped on candidates and persisted with every record, and is
// never used for tiering, ranking, the prompt or execution. Switching it to
// ranking needs the pre-registered F-1 and F-3 to pass
// (docs/t2-u7-preregistration.md); that switch is not part of U7.

// V7ExpectedRConfig is HunterConfig.V7ExpectedR (json v7_expected_r). Zero
// values mean the defaults below.
type V7ExpectedRConfig struct {
	// Enabled turns on the runtime shadow estimate (calibration refit from
	// the outcome records, plus the optional seed file). Off by default: it
	// adds a periodic outcome query and has no effect on decisions.
	Enabled bool `json:"enabled,omitempty"`
	// SeedFile is an optional JSON file of V7CalibrationOutcome rows used in
	// addition to the persisted outcomes (for example an offline fit input).
	SeedFile string `json:"seed_file,omitempty"`

	// Estimator (§2.4).
	KGrid                []float64 `json:"k_grid,omitempty"`
	MinRows              int       `json:"min_rows,omitempty"` // no calibration below this many outcomes (OD-8 N_min)
	LowerBoundConfidence float64   `json:"lower_bound_confidence,omitempty"`
	PoolDirections       bool      `json:"pool_directions,omitempty"` // false = cell key splits direction
	ExtraCostR           float64   `json:"extra_cost_r,omitempty"`    // c in E[R]; 0 under OD-2 (cost already in R)

	// Re-estimation cadence (OD-8): refit when any cell gained
	// RefitMinNewOutcomes terminal outcomes, or the fit is older than
	// RefitMaxAgeHours; checked at most every RefitCheckMinutes.
	RefitMinNewOutcomes int     `json:"refit_min_new_outcomes,omitempty"`
	RefitMaxAgeHours    float64 `json:"refit_max_age_hours,omitempty"`
	RefitCheckMinutes   float64 `json:"refit_check_minutes,omitempty"`
	WindowDays          float64 `json:"window_days,omitempty"` // 0 = all outcomes

	// Falsification protocol (F-1..F-3, docs/t2-u7-preregistration.md).
	AdmitFraction      float64 `json:"admit_fraction,omitempty"`
	RandomDraws        int     `json:"random_draws,omitempty"`
	BootstrapResamples int     `json:"bootstrap_resamples,omitempty"`
	BootstrapCI        float64 `json:"bootstrap_ci,omitempty"`
	Quantiles          int     `json:"quantiles,omitempty"`
	F1MinFolds         int     `json:"f1_min_folds,omitempty"`
	F2MinQuantiles     int     `json:"f2_min_quantiles,omitempty"`
	F3MaxP             float64 `json:"f3_max_p,omitempty"`
	Seed               int64   `json:"seed,omitempty"`
}

// Defaults (plan §2.4, OD-8, pre-registration).
var DefaultV7ExpectedRKGrid = []float64{0.5, 1, 2, 5, 10, 20, 50, 100, 200}

const (
	DefaultV7ExpectedRMinRows              = 30
	DefaultV7ExpectedRLowerBoundConfidence = 0.90
	DefaultV7ExpectedRRefitMinNewOutcomes  = 25
	DefaultV7ExpectedRRefitMaxAgeHours     = 720 // monthly
	DefaultV7ExpectedRRefitCheckMinutes    = 60
	DefaultV7ExpectedRAdmitFraction        = 0.5
	DefaultV7ExpectedRRandomDraws          = 1000
	DefaultV7ExpectedRBootstrapResamples   = 5000
	DefaultV7ExpectedRBootstrapCI          = 0.90
	DefaultV7ExpectedRQuantiles            = 5
	DefaultV7ExpectedRF1MinFolds           = 2
	DefaultV7ExpectedRF2MinQuantiles       = 4
	DefaultV7ExpectedRF3MaxP               = 0.10
	DefaultV7ExpectedRSeed                 = 20261118
)

// EffectiveV7ExpectedR returns the config with defaults filled in.
func EffectiveV7ExpectedR(c *V7ExpectedRConfig) V7ExpectedRConfig {
	out := V7ExpectedRConfig{}
	if c != nil {
		out = *c
	}
	if len(out.KGrid) == 0 {
		out.KGrid = append([]float64(nil), DefaultV7ExpectedRKGrid...)
	}
	setI := func(v *int, d int) {
		if *v <= 0 {
			*v = d
		}
	}
	setF := func(v *float64, d float64) {
		if *v <= 0 {
			*v = d
		}
	}
	setI(&out.MinRows, DefaultV7ExpectedRMinRows)
	setF(&out.LowerBoundConfidence, DefaultV7ExpectedRLowerBoundConfidence)
	setI(&out.RefitMinNewOutcomes, DefaultV7ExpectedRRefitMinNewOutcomes)
	setF(&out.RefitMaxAgeHours, DefaultV7ExpectedRRefitMaxAgeHours)
	setF(&out.RefitCheckMinutes, DefaultV7ExpectedRRefitCheckMinutes)
	setF(&out.AdmitFraction, DefaultV7ExpectedRAdmitFraction)
	setI(&out.RandomDraws, DefaultV7ExpectedRRandomDraws)
	setI(&out.BootstrapResamples, DefaultV7ExpectedRBootstrapResamples)
	setF(&out.BootstrapCI, DefaultV7ExpectedRBootstrapCI)
	setI(&out.Quantiles, DefaultV7ExpectedRQuantiles)
	setI(&out.F1MinFolds, DefaultV7ExpectedRF1MinFolds)
	setI(&out.F2MinQuantiles, DefaultV7ExpectedRF2MinQuantiles)
	setF(&out.F3MaxP, DefaultV7ExpectedRF3MaxP)
	if out.Seed == 0 {
		out.Seed = DefaultV7ExpectedRSeed
	}
	return out
}

// V7CalibrationOutcome is one terminal fee-net outcome used to fit the
// estimator.
type V7CalibrationOutcome struct {
	Time      time.Time `json:"time"`
	SetupType string    `json:"setup_type"`
	Direction string    `json:"direction"`
	Class     string    `json:"signal_class"`
	Family    string    `json:"family"`
	RRBand    string    `json:"rr_band"`
	Outcome   string    `json:"fee_net_outcome"` // tp1 / stop / none
	RWin      float64   `json:"r_win"`           // planned fee-net R at TP1 from the walk entry (OD-2)
}

// CalibrationOutcomes returns the resolved fee-net outcomes since a time
// (zero = all), with R_win from the walk entry, the stop and TP1 at the
// OD-2 cost costPct. Rows whose R_win cannot be formed are skipped. Every outcome source counts: the watch_audit / shadow
// buckets stay out of win-rate denominators, but their fee-net walks are
// valid calibration rows.
func (s *HunterV7SignalStore) CalibrationOutcomes(since time.Time, costPct float64) ([]V7CalibrationOutcome, error) {
	var recs []HunterV7SignalRecord
	q := s.db.Select("timestamp", "setup_type", "direction", "signal_class", "family", "rr_band", "fee_net_outcome",
		"fee_net_entry", "invalidation_price", "tp1_price", "target_1").
		Where("fee_net_outcome IN ?", []string{V7FeeNetTP1, V7FeeNetStop, V7FeeNetNone})
	if !since.IsZero() {
		q = q.Where("timestamp >= ?", since)
	}
	if err := q.Order("timestamp").Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]V7CalibrationOutcome, 0, len(recs))
	for _, r := range recs {
		tp1 := r.TP1Price
		if tp1 <= 0 {
			tp1 = r.Target1
		}
		rwin, ok := V7FeeNetRRAtTP1(r.FeeNetEntry, r.InvalidationPrice, tp1, costPct)
		if !ok {
			continue
		}
		out = append(out, V7CalibrationOutcome{Time: r.Timestamp, SetupType: r.SetupType, Direction: r.Direction,
			Class: r.SignalClass, Family: r.Family, RRBand: r.RRBand, Outcome: r.FeeNetOutcome, RWin: rwin})
	}
	return out, nil
}
