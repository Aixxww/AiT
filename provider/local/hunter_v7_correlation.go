package local

import "sort"

// ============================================================================
// Hunter v8 — Correlation / Thematic De-dup Filter (v8-SPEC P2-2)
// ============================================================================
// Prevents flooding the LLM prompt with too many similar signals from the same
// thematic bucket.  After the router ranks all signals, this filter keeps at
// most maxPerTheme per theme (sorted by AIPriority descending).

// SignalTheme groups similar setups into broad thematic buckets.
type SignalTheme string

const (
	ThemeLongMomentum  SignalTheme = "long_momentum"
	ThemeLongReversal  SignalTheme = "long_reversal"
	ThemeShortReversal SignalTheme = "short_reversal"
	ThemeRangeBound    SignalTheme = "range_bound"
	ThemeSqueeze       SignalTheme = "squeeze"
	ThemeAccumulation  SignalTheme = "accumulation"
	ThemeFunding       SignalTheme = "funding"
	ThemeScalp         SignalTheme = "scalp"
	ThemeUnknown       SignalTheme = "unknown"
)

// CorrelationFilter limits the number of signals per thematic bucket.
type CorrelationFilter struct {
	maxPerTheme int
}

// NewCorrelationFilter creates a filter with the given per-theme cap.
func NewCorrelationFilter(maxPerTheme int) *CorrelationFilter {
	if maxPerTheme <= 0 {
		maxPerTheme = 3
	}
	return &CorrelationFilter{maxPerTheme: maxPerTheme}
}

// DefaultCorrelationFilter returns a filter allowing 3 signals per theme.
func DefaultCorrelationFilter() *CorrelationFilter {
	return NewCorrelationFilter(3)
}

// ClassifyTheme assigns a thematic bucket to a signal.
func (cf *CorrelationFilter) ClassifyTheme(sig *V7SignalOutput) SignalTheme {
	if sig == nil {
		return ThemeUnknown
	}
	switch sig.SetupType {
	case V7SetupLeaderMomentumLong, V7SetupTrendBreakoutLong, V7SetupDisplacementLong:
		return ThemeLongMomentum
	case V7SetupPullbackLong, V7SetupPanicReversalLong, V7SetupShortSqueezeLong:
		return ThemeLongReversal
	case V7SetupDistributionShort, V7SetupLongSqueezeShort:
		return ThemeShortReversal
	case V7SetupRangeReversion:
		return ThemeRangeBound
	case V7SetupAccumulationLong, V7SetupAccumulationWatch, V7SetupWhaleFlow:
		return ThemeAccumulation
	case V7SetupFundingReversal:
		return ThemeFunding
	case V7SetupVolatilitySqueeze, V7SetupPreSqueezeWatch:
		return ThemeSqueeze
	case V7SetupIntradayScalp:
		return ThemeScalp
	default:
		return ThemeUnknown
	}
}

// FilterByCorrelation keeps at most maxPerTheme signals per theme, selected
// by highest AIPriority.  The returned slice is sorted by AIPriority desc.
func (cf *CorrelationFilter) FilterByCorrelation(signals []V7SignalOutput) []V7SignalOutput {
	if cf == nil || len(signals) == 0 {
		return signals
	}

	// Group by theme
	type themed struct {
		theme   SignalTheme
		signals []V7SignalOutput
	}
	themeBuckets := make(map[SignalTheme]*themed)

	for _, sig := range signals {
		theme := cf.ClassifyTheme(&sig)
		b, ok := themeBuckets[theme]
		if !ok {
			b = &themed{theme: theme}
			themeBuckets[theme] = b
		}
		b.signals = append(b.signals, sig)
	}

	// Sort each bucket by AIPriority desc, keep top maxPerTheme
	var result []V7SignalOutput
	for _, b := range themeBuckets {
		sort.Slice(b.signals, func(i, j int) bool {
			return b.signals[i].AIPriority > b.signals[j].AIPriority
		})
		limit := cf.maxPerTheme
		if limit > len(b.signals) {
			limit = len(b.signals)
		}
		result = append(result, b.signals[:limit]...)
	}

	// Sort result by AIPriority desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].AIPriority > result[j].AIPriority
	})

	return result
}
