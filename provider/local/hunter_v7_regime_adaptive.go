package local

import (
	"math"
	"sort"
)

// ============================================================================
// Hunter v8 — Regime Adaptive Weight Engine (P1-C)
// ============================================================================
// Adjusts regime×module weights based on real PnL data accumulated by the
// SignalOutcomeTracker.  Runs once daily (or on-demand) to nudge weights
// toward setups that are actually winning and away from setups that are
// consistently losing.

// SetupRegimeStats holds aggregated statistics for a single regime×setup pair.
type SetupRegimeStats struct {
	Samples int     `json:"samples"`
	WinRate float64 `json:"win_rate"`
	AvgPnL  float64 `json:"avg_pnl"`
}

// SetupRegimeStatsInput is the public string-keyed input shape used by stores
// and reports before conversion to the internal regimeWeightKey.
type SetupRegimeStatsInput struct {
	Regime  string
	Setup   string
	Samples int
	WinRate float64
	AvgPnL  float64
}

// RegimeAdaptiveEngine manages data-driven weight adjustments.
type RegimeAdaptiveEngine struct {
	baseWeights   map[regimeWeightKey]float64
	adaptiveAdj   map[regimeWeightKey]float64
	minSamples    int     // minimum samples to trigger adjustment
	maxAdjustment float64 // maximum total adjustment per cell
}

// RegimeAdaptiveAdjustmentReport exposes the current base, adjustment, and
// effective weights for external reporting or dry-run inspection.
type RegimeAdaptiveAdjustmentReport struct {
	Regime          string  `json:"regime"`
	Setup           string  `json:"setup"`
	BaseWeight      float64 `json:"base_weight"`
	Adjustment      float64 `json:"adjustment"`
	EffectiveWeight float64 `json:"effective_weight"`
}

// NewRegimeAdaptiveEngine creates an engine seeded with the current static weights.
func NewRegimeAdaptiveEngine() *RegimeAdaptiveEngine {
	base := make(map[regimeWeightKey]float64, len(regimeWeightMatrix))
	adj := make(map[regimeWeightKey]float64, len(regimeWeightMatrix))
	for k, w := range regimeWeightMatrix {
		base[k] = w
		adj[k] = 0
	}
	return &RegimeAdaptiveEngine{
		baseWeights:   base,
		adaptiveAdj:   adj,
		minSamples:    5,
		maxAdjustment: 0.15,
	}
}

// RecalculateWeights adjusts weights based on accumulated PnL data.
// Call this once per day with stats aggregated from the PnL Tracker.
func (e *RegimeAdaptiveEngine) RecalculateWeights(stats map[regimeWeightKey]SetupRegimeStats) {
	if e == nil {
		return
	}
	for key, stat := range stats {
		if stat.Samples < e.minSamples {
			continue
		}
		currentAdj := e.adaptiveAdj[key]
		winRate := normalizeAdaptiveWinRate(stat.WinRate)

		switch {
		case winRate > 0.60 && stat.AvgPnL > 2.0:
			// Winning setup: boost (up to maxAdjustment)
			headroom := e.maxAdjustment - currentAdj
			if headroom > 0 {
				e.adaptiveAdj[key] += math.Min(0.05, headroom)
			}
		case winRate < 0.35 && stat.AvgPnL < -1.0:
			// Losing setup: suppress (down to -maxAdjustment)
			headroom := e.maxAdjustment + currentAdj
			if headroom > 0 {
				e.adaptiveAdj[key] -= math.Min(0.05, headroom)
			}
		default:
			// Neutral zone: decay 10% toward zero
			e.adaptiveAdj[key] *= 0.9
		}
	}
}

// RecalculateWeightsFromInput converts DB/report aggregates into internal
// regime×setup keys. Unknown regimes or setups are ignored, and low sample
// cells remain gated by RecalculateWeights.
func (e *RegimeAdaptiveEngine) RecalculateWeightsFromInput(rows []SetupRegimeStatsInput) {
	if e == nil {
		return
	}
	stats := make(map[regimeWeightKey]SetupRegimeStats, len(rows))
	for _, row := range rows {
		if row.Regime == "" || row.Setup == "" {
			continue
		}
		key := regimeWeightKey{regime: V7MarketRegime(row.Regime), setup: V7SetupType(row.Setup)}
		if _, ok := e.baseWeights[key]; !ok {
			continue
		}
		stats[key] = SetupRegimeStats{
			Samples: row.Samples,
			WinRate: row.WinRate,
			AvgPnL:  row.AvgPnL,
		}
	}
	e.RecalculateWeights(stats)
}

func normalizeAdaptiveWinRate(winRate float64) float64 {
	if winRate > 1 {
		return winRate / 100
	}
	return winRate
}

// GetEffectiveWeight returns base weight + adaptive adjustment, clamped to [0.1, 1.5].
func (e *RegimeAdaptiveEngine) GetEffectiveWeight(regime V7MarketRegime, setup V7SetupType) float64 {
	if e == nil {
		return 1.0
	}
	key := regimeWeightKey{regime, setup}
	base := e.baseWeights[key]
	if base == 0 {
		base = 1.0
	}
	adj := e.adaptiveAdj[key]
	effective := base + adj
	return math.Max(0.1, math.Min(1.5, effective))
}

// ResetAdaptive clears all adjustments, reverting to base weights.
func (e *RegimeAdaptiveEngine) ResetAdaptive() {
	if e == nil {
		return
	}
	for k := range e.adaptiveAdj {
		e.adaptiveAdj[k] = 0
	}
}

// GetAdjustmentMap returns a copy of the current adjustments for reporting.
func (e *RegimeAdaptiveEngine) GetAdjustmentMap() map[regimeWeightKey]float64 {
	if e == nil {
		return nil
	}
	out := make(map[regimeWeightKey]float64, len(e.adaptiveAdj))
	for k, v := range e.adaptiveAdj {
		out[k] = v
	}
	return out
}

// GetAdjustmentReport returns a public dry-run view of all regime/setup cells.
func (e *RegimeAdaptiveEngine) GetAdjustmentReport() []RegimeAdaptiveAdjustmentReport {
	if e == nil {
		return nil
	}
	out := make([]RegimeAdaptiveAdjustmentReport, 0, len(e.baseWeights))
	for key, base := range e.baseWeights {
		adj := e.adaptiveAdj[key]
		out = append(out, RegimeAdaptiveAdjustmentReport{
			Regime:          string(key.regime),
			Setup:           string(key.setup),
			BaseWeight:      base,
			Adjustment:      adj,
			EffectiveWeight: math.Max(0.1, math.Min(1.5, base+adj)),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Regime == out[j].Regime {
			return out[i].Setup < out[j].Setup
		}
		return out[i].Regime < out[j].Regime
	})
	return out
}
