package local

// ============================================================================
// Hunter v8 — Funding Fast-Track (P0-F)
// ============================================================================
// When funding rate is extreme (>0.001), funding reversal SHORT signals are
// blocked by wait_zone_retest_required because the price hasn't retested the
// upper entry zone.  This fast-track relaxes the zone position requirement
// and provides a small setup score boost, allowing these high-edge signals
// to surface.

// FundingFastTrackConfig holds tunable parameters.
type FundingFastTrackConfig struct {
	ExtremeFundingThreshold    float64 // 0.001 — funding above this = extreme positive
	ShortZonePosRelaxation     int     // lower zone-pos requirement from 65 to 40
	TimingPenaltyReduction     float64 // reduce timing penalty by 10
	EnableShortFastTrack       bool    // enable fast-track for SHORT signals
	EnableLongFastTrack        bool    // enable fast-track for LONG signals
	ExtremeNegFundingThreshold float64 // -0.001 — funding below this = extreme negative
}

// DefaultFundingFastTrackConfig returns sensible defaults.
func DefaultFundingFastTrackConfig() *FundingFastTrackConfig {
	return &FundingFastTrackConfig{
		ExtremeFundingThreshold:    0.001,
		ShortZonePosRelaxation:     40,
		TimingPenaltyReduction:     10,
		EnableShortFastTrack:       true,
		EnableLongFastTrack:        true,
		ExtremeNegFundingThreshold: -0.001,
	}
}

// FundingFastTrack applies fast-track logic for funding reversal signals with
// extreme funding conditions.
type FundingFastTrack struct {
	config *FundingFastTrackConfig
}

// NewFundingFastTrack creates a new fast-track with the given config.
func NewFundingFastTrack(cfg *FundingFastTrackConfig) *FundingFastTrack {
	if cfg == nil {
		cfg = DefaultFundingFastTrackConfig()
	}
	return &FundingFastTrack{config: cfg}
}

// DefaultFundingFastTrack returns a fast-track with default configuration.
func DefaultFundingFastTrack() *FundingFastTrack {
	return NewFundingFastTrack(nil)
}

// ShouldFastTrack returns true if the signal qualifies for funding fast-track.
func (fft *FundingFastTrack) ShouldFastTrack(sig *V7SignalOutput, ctx *V7SymbolContext) bool {
	if fft == nil || sig == nil || ctx == nil {
		return false
	}
	if sig.SetupType != V7SetupFundingReversal {
		return false
	}
	funding := fft.getFundingRate(ctx)
	// SHORT with extreme positive funding
	if sig.Direction == V7DirShort && fft.config.EnableShortFastTrack {
		return funding > fft.config.ExtremeFundingThreshold
	}
	// LONG with extreme negative funding
	if sig.Direction == V7DirLong && fft.config.EnableLongFastTrack {
		return funding < fft.config.ExtremeNegFundingThreshold
	}
	return false
}

// ApplyFastTrack modifies the signal with fast-track adjustments.
func (fft *FundingFastTrack) ApplyFastTrack(sig *V7SignalOutput) {
	if fft == nil || sig == nil {
		return
	}
	sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "funding_extreme_fast_track")
	sig.RiskTags = appendIfMissing(sig.RiskTags, "fast_tracked_funding")
	sig.SetupScore += 5
	sig.SetupScore = clampFloat(sig.SetupScore, 0, 100)
}

// getFundingRate extracts the funding rate from the context.
func (fft *FundingFastTrack) getFundingRate(ctx *V7SymbolContext) float64 {
	if ctx.Snapshot != nil {
		return ctx.Snapshot.FundingRate
	}
	return 0
}
