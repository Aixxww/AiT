package trader

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/store"
)

// U7: shadow E[R] calibration refresh. It refits the estimator from the
// persisted fee-net outcomes (plus the optional seed file) on the OD-8
// cadence and installs it on the strategy engine, which stamps candidates
// and records with it. Nothing reads the estimate for a decision.

type hunterV7ERRefresher struct {
	mu        sync.Mutex
	cfg       store.V7ExpectedRConfig
	costPct   float64
	load      func(since time.Time) ([]store.V7CalibrationOutcome, error)
	install   func(*kernel.HunterV7ERCalibration)
	seed      []store.V7CalibrationOutcome
	lastCheck time.Time
	fitted    *kernel.HunterV7ERCalibration
	fitCells  map[string]int
}

func newHunterV7ERRefresher(cfg store.V7ExpectedRConfig, costPct float64,
	load func(time.Time) ([]store.V7CalibrationOutcome, error), install func(*kernel.HunterV7ERCalibration)) *hunterV7ERRefresher {
	r := &hunterV7ERRefresher{cfg: cfg, costPct: costPct, load: load, install: install}
	if cfg.SeedFile != "" {
		if b, err := os.ReadFile(cfg.SeedFile); err != nil {
			logger.Warnf("⚠ [HUNTER V7 E[R]] seed file %s: %v", cfg.SeedFile, err)
		} else if err := json.Unmarshal(b, &r.seed); err != nil {
			logger.Warnf("⚠ [HUNTER V7 E[R]] seed file %s: %v", cfg.SeedFile, err)
		}
	}
	return r
}

func hunterV7ERCellCounts(rows []store.V7CalibrationOutcome, pool bool) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		fam := r.Family
		if fam == "" {
			fam = store.V7Family(r.SetupType)
		}
		out[kernel.HunterV7ERCellKey(fam, r.Direction, r.RRBand, pool)]++
	}
	return out
}

// maybeRefit runs at most every RefitCheckMinutes. It refits when there is
// no fit yet, the fit is older than RefitMaxAgeHours, or any cell gained
// RefitMinNewOutcomes outcomes since the fit (OD-8).
func (r *hunterV7ERRefresher) maybeRefit(now time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.lastCheck.IsZero() && now.Sub(r.lastCheck) < time.Duration(r.cfg.RefitCheckMinutes*float64(time.Minute)) {
		return
	}
	r.lastCheck = now
	since := time.Time{}
	if r.cfg.WindowDays > 0 {
		since = now.Add(-time.Duration(r.cfg.WindowDays * 24 * float64(time.Hour)))
	}
	rows, err := r.load(since)
	if err != nil {
		logger.Warnf("⚠ [HUNTER V7 E[R]] load outcomes: %v", err)
		return
	}
	rows = append(append([]store.V7CalibrationOutcome(nil), r.seed...), rows...)
	cells := hunterV7ERCellCounts(rows, r.cfg.PoolDirections)
	due := r.fitted == nil || now.Sub(r.fitted.FittedAt) >= time.Duration(r.cfg.RefitMaxAgeHours*float64(time.Hour))
	for k, n := range cells {
		if n-r.fitCells[k] >= r.cfg.RefitMinNewOutcomes {
			due = true
		}
	}
	if !due {
		return
	}
	cal := kernel.FitHunterV7ExpectedRFromOutcomes(rows, r.cfg, now)
	if cal == nil {
		return // below MinRows: stay without a calibration
	}
	r.fitted, r.fitCells = cal, cells
	r.install(cal)
	logger.Infof("📐 [HUNTER V7 E[R]] shadow calibration %s (k=%g, %d outcomes, %d cells)", cal.Version, cal.K, cal.Global.N, len(cal.Cell))
}
