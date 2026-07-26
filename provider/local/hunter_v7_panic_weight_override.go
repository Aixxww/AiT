package local

// ============================================================================
// Hunter v8 — Panic Weight Override (P0-E)
// ============================================================================
// Panic reversal signals are punished by low timing_score in panic_dump
// regime.  This module boosts timing_weight in the AI priority formula for
// specific regime+setup combos, allowing panic reversal and short squeeze
// longs to surface despite weak timing.

// regimeSetupKey uniquely identifies a regime×setup combination.
type regimeSetupKey struct {
	regime V7MarketRegime
	setup  V7SetupType
}

// OverrideEntry describes how to adjust scoring for a regime+setup pair.
type OverrideEntry struct {
	TimingWeightBoost float64 `json:"timing_weight_boost"`  // e.g. +0.10
	MinTimingForReady float64 `json:"min_timing_for_ready"` // lowered from default 60
	Reason            string  `json:"reason"`
}

// PanicWeightOverride holds the override map and applies adjustments.
type PanicWeightOverride struct {
	overrides map[regimeSetupKey]OverrideEntry
}

// DefaultPanicWeightOverride returns an override with sensible defaults.
func DefaultPanicWeightOverride() *PanicWeightOverride {
	return &PanicWeightOverride{
		overrides: map[regimeSetupKey]OverrideEntry{
			{V7RegimePanicDump, V7SetupPanicReversalLong}: {
				TimingWeightBoost: 0.10,
				MinTimingForReady: 35,
				Reason:            "panic_dump_panic_reversal_long_timing_boost",
			},
			{V7RegimePanicDump, V7SetupShortSqueezeLong}: {
				TimingWeightBoost: 0.05,
				MinTimingForReady: 40,
				Reason:            "panic_dump_short_squeeze_long_timing_boost",
			},
			{V7RegimeTrendDown, V7SetupPanicReversalLong}: {
				TimingWeightBoost: 0.08,
				MinTimingForReady: 38,
				Reason:            "trend_down_panic_reversal_long_timing_boost",
			},
		},
	}
}

// GetOverride returns the override entry for a regime+setup pair, or nil if
// no override exists.
func (p *PanicWeightOverride) GetOverride(regime V7MarketRegime, setup V7SetupType) *OverrideEntry {
	if p == nil {
		return nil
	}
	key := regimeSetupKey{regime, setup}
	if e, ok := p.overrides[key]; ok {
		return &e
	}
	return nil
}

// AdjustAIPriority recalculates AIPriority with boosted timing weight for
// matching regime+setup combos.  Returns the adjusted priority (clamped 0-100).
//
// The standard formula is:
//
//	setup×0.35 + timing×0.20 + regime×0.20 + liquidity×0.15 - risk×0.10
//
// This function replaces the timing coefficient (0.20) with
// (0.20 + timing_weight_boost) and re-evaluates.
func (p *PanicWeightOverride) AdjustAIPriority(
	originalAIPriority, timingScore float64,
	regime V7MarketRegime, setup V7SetupType,
) float64 {
	if p == nil {
		return originalAIPriority
	}
	override := p.GetOverride(regime, setup)
	if override == nil {
		return originalAIPriority
	}
	// The boost effectively replaces:
	//   timing × 0.20  →  timing × (0.20 + boost)
	// So the delta is:  timingScore × boost
	delta := timingScore * override.TimingWeightBoost
	return clampFloat(originalAIPriority+delta, 0, 100)
}
