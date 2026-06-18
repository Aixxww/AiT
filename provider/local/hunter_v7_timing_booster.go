package local

// ============================================================================
// Hunter v8 — Timing Booster (P0-D)
// ============================================================================
// Detects chase-high conditions and penalises timing score when the entry
// price is stretched (high entry-zone position) or RSI5m is overbought.
// This prevents the AI from entering at the top of a short-term impulse.

// TimingBoosterConfig holds tunable parameters for the timing booster.
type TimingBoosterConfig struct {
	MaxEntryZonePosition  float64 // 0.70 — above this is "extended"
	MinTakerBuyForTopZone float64 // 0.60 — need strong buy flow to justify top-zone entry
	MomentumDecayWindow   int     // 5 — candles for momentum decay check
	MaxDecayRatio         float64 // 0.4 — max allowed decay ratio
	MinVolSustainRatio    float64 // 0.7 — volume must sustain above this
	RSI5mOverbought       float64 // 80.0 — RSI5m above this = overbought
	TimingPenalty         float64 // -15 — base penalty for chase-high
}

// TimingBoostResult captures the output of a timing enhancement pass.
type TimingBoostResult struct {
	OriginalTiming float64             `json:"original_timing"`
	AdjustedTiming float64             `json:"adjusted_timing"`
	Delta          float64             `json:"delta"`
	Adjustments    []TimingAdjustment  `json:"adjustments"`
	DowngradeTier  *V7ReadinessTier    `json:"downgrade_tier,omitempty"`
}

// TimingAdjustment describes a single score adjustment applied.
type TimingAdjustment struct {
	ReasonCode string  `json:"reason_code"`
	Delta      float64 `json:"delta"`
}

// TimingBooster applies chase-high and overbought penalties to timing score.
type TimingBooster struct {
	config TimingBoosterConfig
}

// DefaultTimingBooster returns a TimingBooster with sensible defaults.
func DefaultTimingBooster() *TimingBooster {
	return &TimingBooster{
		config: TimingBoosterConfig{
			MaxEntryZonePosition:  0.70,
			MinTakerBuyForTopZone: 0.60,
			MomentumDecayWindow:   5,
			MaxDecayRatio:         0.4,
			MinVolSustainRatio:    0.7,
			RSI5mOverbought:       80.0,
			TimingPenalty:         -15,
		},
	}
}

// EnhanceTiming checks for chase-high conditions and adjusts timing score.
func (tb *TimingBooster) EnhanceTiming(sig *V7SignalOutput, ctx *V7SymbolContext) *TimingBoostResult {
	if sig == nil || ctx == nil {
		return nil
	}

	result := &TimingBoostResult{
		OriginalTiming: sig.TimingScore,
		AdjustedTiming: sig.TimingScore,
		Adjustments:    make([]TimingAdjustment, 0, 3),
	}

	// --- Check 1: Entry zone position (chase-high protection) ---
	if pos, ok := v7EntryZonePositionPct(sig, ctx.CurrentPrice); ok {
		posRatio := pos / 100.0 // convert to 0-1
		if posRatio > tb.config.MaxEntryZonePosition && ctx.TakerBuy15m < tb.config.MinTakerBuyForTopZone {
			penalty := tb.config.TimingPenalty
			result.AdjustedTiming += penalty
			result.Adjustments = append(result.Adjustments, TimingAdjustment{
				ReasonCode: "chase_high_protection",
				Delta:      penalty,
			})
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "chase_high_protection")
		}
	}

	// --- Check 2: RSI5m overbought ---
	if ctx.RSI5m > tb.config.RSI5mOverbought {
		rsiPenalty := -12.0
		result.AdjustedTiming += rsiPenalty
		result.Adjustments = append(result.Adjustments, TimingAdjustment{
			ReasonCode: "rsi5m_overbought",
			Delta:      rsiPenalty,
		})
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "rsi5m_overbought")
	}

	// Clamp adjusted timing
	result.AdjustedTiming = clampFloat(result.AdjustedTiming, 0, 100)
	result.Delta = result.AdjustedTiming - result.OriginalTiming

	// Apply to signal
	if result.Delta != 0 {
		sig.TimingScore = result.AdjustedTiming
	}

	// Suggest tier downgrade if timing dropped significantly
	if result.Delta <= -15 {
		downgrade := V7ReadinessWatch
		result.DowngradeTier = &downgrade
	}

	return result
}
