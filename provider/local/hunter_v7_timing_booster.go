package local

// ============================================================================
// Hunter v8 — Timing Booster (P0-D)
// ============================================================================
// Detects chase conditions and penalises timing score when the entry price is
// stretched toward the chase end of the zone for the signal's direction, or
// RSI5m is overbought. Zone position is the canonical zone_position (#134:
// percent from the zone's lower edge, clamped to [0,100]): a LONG chases at
// the top of the zone, a SHORT chases at the bottom.

// TimingBoosterConfig holds tunable parameters for the timing booster.
type TimingBoosterConfig struct {
	MaxEntryZonePosition      float64 // 70 (percent) — LONG above this is "extended"
	MinTakerBuyForTopZone     float64 // 0.60 — LONG needs strong buy flow to justify top-zone entry
	MinEntryZonePositionShort float64 // 30 (percent) — SHORT below this is "extended" (mirror of 70)
	MaxTakerBuyForBottomZone  float64 // 0.40 — SHORT needs strong sell flow (taker buy <= this) to justify bottom-zone entry
	MomentumDecayWindow       int     // 5 — candles for momentum decay check
	MaxDecayRatio             float64 // 0.4 — max allowed decay ratio
	MinVolSustainRatio        float64 // 0.7 — volume must sustain above this
	RSI5mOverbought           float64 // 80.0 — RSI5m above this = overbought
	TimingPenalty             float64 // -15 — base penalty for chase-high
}

// TimingBoostResult captures the output of a timing enhancement pass.
type TimingBoostResult struct {
	OriginalTiming float64            `json:"original_timing"`
	AdjustedTiming float64            `json:"adjusted_timing"`
	Delta          float64            `json:"delta"`
	Adjustments    []TimingAdjustment `json:"adjustments"`
	DowngradeTier  *V7ReadinessTier   `json:"downgrade_tier,omitempty"`
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
			MaxEntryZonePosition:      70,
			MinTakerBuyForTopZone:     0.60,
			MinEntryZonePositionShort: 30,
			MaxTakerBuyForBottomZone:  0.40,
			MomentumDecayWindow:       5,
			MaxDecayRatio:             0.4,
			MinVolSustainRatio:        0.7,
			RSI5mOverbought:           80.0,
			TimingPenalty:             -15,
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

	// --- Check 1: Entry zone position (direction-aware chase protection) ---
	if pos, ok := v7EntryZonePositionPct(sig, ctx.CurrentPrice); ok {
		if code := tb.chaseZoneReason(sig.Direction, pos, ctx.TakerBuy15m); code != "" {
			penalty := tb.config.TimingPenalty
			result.AdjustedTiming += penalty
			result.Adjustments = append(result.Adjustments, TimingAdjustment{
				ReasonCode: code,
				Delta:      penalty,
			})
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, code)
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

// chaseZoneReason returns the chase-protection reason code for a zone position
// (percent from the lower edge, clamped) or "" when the entry is not a chase.
// LONG chases high in the zone without strong buy flow (chase_high_protection);
// SHORT chases low in the zone without strong sell flow (chase_low_protection).
// A SHORT near the zone top is at its retest edge and is never penalised here.
func (tb *TimingBooster) chaseZoneReason(direction V7Direction, pos, takerBuy float64) string {
	switch direction {
	case V7DirLong:
		if pos > tb.config.MaxEntryZonePosition && takerBuy < tb.config.MinTakerBuyForTopZone {
			return "chase_high_protection"
		}
	case V7DirShort:
		if pos < tb.config.MinEntryZonePositionShort && takerBuy > tb.config.MaxTakerBuyForBottomZone {
			return "chase_low_protection"
		}
	}
	return ""
}
