package local

// ============================================================================
// Hunter v8 — Condition Resonance Scorer (P1-A)
// ============================================================================
// Replaces the linear weighted scoring with a "condition resonance" model:
// when multiple high-value reason_codes co-occur, the signal gets a non-linear
// bonus. Conversely, negative-resonance patterns (momentum exhaustion) get a
// penalty.  The result is stored on V7SignalOutput.ResonanceBonus and folded
// into the AIPriority calculation in the router.

// ResonancePattern defines a named co-occurrence condition.
type ResonancePattern struct {
	Name           string   `json:"name"`
	RequiredCodes  []string `json:"required_codes"`  // reason_codes to match (AND)
	MinMatchCount  int      `json:"min_match_count"` // at least N must be present
	ResonanceBonus float64  `json:"resonance_bonus"` // positive = bonus, negative = penalty
	Confidence     string   `json:"confidence"`      // "HIGH" / "MEDIUM" / "LOW"
}

// MatchedPattern records which pattern matched and how strongly.
type MatchedPattern struct {
	Name        string  `json:"name"`
	MatchCount  int     `json:"match_count"`
	TotalNeeded int     `json:"total_needed"`
	Bonus       float64 `json:"bonus"`
	Confidence  string  `json:"confidence"`
}

// ResonanceResult is the output of a resonance evaluation.
type ResonanceResult struct {
	MatchedPatterns []MatchedPattern `json:"matched_patterns"`
	TotalBonus      float64          `json:"total_bonus"`
}

// HighResonancePatterns is the default set of resonance patterns.
var HighResonancePatterns = []ResonancePattern{
	{
		Name:           "strong_reversal_triple",
		RequiredCodes:  []string{"strong_reclaim", "flow_taker_buy_strong", "oi_massive_flush"},
		MinMatchCount:  3,
		ResonanceBonus: 18.0,
		Confidence:     "HIGH",
	},
	{
		Name:           "funding_extreme_short",
		RequiredCodes:  []string{"extreme_long_crowding", "price_turning_down", "strong_taker_sell_reversal", "elevated_funding"},
		MinMatchCount:  3,
		ResonanceBonus: 15.0,
		Confidence:     "HIGH",
	},
	{
		Name:           "displacement_breakout",
		RequiredCodes:  []string{"volume_breakout", "range_expansion", "flow_taker_buy_aggressive", "oi_building"},
		MinMatchCount:  3,
		ResonanceBonus: 14.0,
		Confidence:     "MEDIUM",
	},
	{
		Name:           "panic_capitulation_reversal",
		RequiredCodes:  []string{"deep_capitulation", "heavy_capitulation", "1h_green_shoot", "rsi_recovering_from_extreme"},
		MinMatchCount:  3,
		ResonanceBonus: 16.0,
		Confidence:     "HIGH",
	},
	{
		Name:           "stealth_accumulation_breakout",
		RequiredCodes:  []string{"oi_invisible_accumulation_detected", "bb_compressed", "volume_burst_at_breakout", "taker_buy_ratio_above_0.55"},
		MinMatchCount:  3,
		ResonanceBonus: 17.0,
		Confidence:     "MEDIUM",
	},
	{
		Name:           "momentum_exhaustion",
		RequiredCodes:  []string{"no_pullback_still_running", "momentum_decaying", "rsi_overbought", "divergence_bearish"},
		MinMatchCount:  3,
		ResonanceBonus: -12.0,
		Confidence:     "MEDIUM",
	},
}

// ResonanceScorer evaluates signal reason_codes against predefined patterns.
type ResonanceScorer struct {
	patterns []ResonancePattern
}

// NewResonanceScorer creates a scorer with custom patterns.
func NewResonanceScorer(patterns []ResonancePattern) *ResonanceScorer {
	return &ResonanceScorer{patterns: patterns}
}

// DefaultResonanceScorer returns a scorer with the built-in patterns.
func DefaultResonanceScorer() *ResonanceScorer {
	return NewResonanceScorer(HighResonancePatterns)
}

// Score evaluates the signal against all resonance patterns.
func (rs *ResonanceScorer) Score(signal *V7SignalOutput) *ResonanceResult {
	if rs == nil || signal == nil {
		return &ResonanceResult{}
	}
	result := &ResonanceResult{}

	for _, pattern := range rs.patterns {
		matchCount := 0
		for _, code := range pattern.RequiredCodes {
			if containsV7String(signal.ReasonCodes, code) {
				matchCount++
			}
		}
		if matchCount >= pattern.MinMatchCount {
			result.MatchedPatterns = append(result.MatchedPatterns, MatchedPattern{
				Name:        pattern.Name,
				MatchCount:  matchCount,
				TotalNeeded: len(pattern.RequiredCodes),
				Bonus:       pattern.ResonanceBonus,
				Confidence:  pattern.Confidence,
			})
			result.TotalBonus += pattern.ResonanceBonus
		}
	}

	// Cap negative resonance at -15
	if result.TotalBonus < -15 {
		result.TotalBonus = -15
	}

	return result
}

// ApplyToSignal runs Score and writes the bonus onto the signal.
func (rs *ResonanceScorer) ApplyToSignal(sig *V7SignalOutput) {
	if rs == nil || sig == nil {
		return
	}
	result := rs.Score(sig)
	sig.ResonanceBonus = result.TotalBonus
}
