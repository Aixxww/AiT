package store

// U10: data timing (redesign plan §6.2 rows "Data: forming bar", "Data:
// cycle alignment", "Data: snapshot age"). Owner decision 2026-11-18: all
// three default off, so behaviour is unchanged until opted in. Enabling
// cycle alignment stays gated on the U10 parity test (OD-21).

// Feature-bar modes.
const (
	V7FeatureBarsForming = "forming" // default: features include the still-forming bar
	V7FeatureBarsClosed  = "closed"  // features on closed bars; the forming bar only for price
)

// Cycle-alignment modes.
const (
	V7CycleWallClock = "wall_clock" // default: the ScanInterval ticker
	V7CycleAligned   = "aligned"    // scan at each AlignIntervalMinutes bar close + SettleSeconds
)

// V7DataTimingConfig is HunterConfig.V7DataTiming (json v7_data_timing).
type V7DataTimingConfig struct {
	FeatureBars          string  `json:"feature_bars,omitempty"`
	CycleAlignment       string  `json:"cycle_alignment,omitempty"`
	AlignIntervalMinutes int     `json:"align_interval_minutes,omitempty"`
	SettleSeconds        float64 `json:"settle_seconds,omitempty"`
	// FreshnessBudgetSeconds is the per-class snapshot age budget: an open
	// on a candidate of a listed class is rejected when its snapshot is
	// older. Empty = gate off (default). No budget value is adopted (U0-C
	// left it [H]).
	FreshnessBudgetSeconds map[string]float64 `json:"freshness_budget_seconds,omitempty"`
}

const (
	// DefaultV7AlignIntervalMinutes is the bar the scan aligns to (plan
	// §6.2: the 5m close).
	DefaultV7AlignIntervalMinutes = 5
	// DefaultV7SettleSeconds lets one data-collector REST refresh (the
	// datafetch collector's 30 s RestInterval) land after the bar close.
	DefaultV7SettleSeconds = 30
)

// EffectiveV7DataTiming returns the config with defaults filled in.
func EffectiveV7DataTiming(c *V7DataTimingConfig) V7DataTimingConfig {
	out := V7DataTimingConfig{}
	if c != nil {
		out = *c
	}
	if out.FeatureBars != V7FeatureBarsClosed {
		out.FeatureBars = V7FeatureBarsForming
	}
	if out.CycleAlignment != V7CycleAligned {
		out.CycleAlignment = V7CycleWallClock
	}
	if out.AlignIntervalMinutes <= 0 {
		out.AlignIntervalMinutes = DefaultV7AlignIntervalMinutes
	}
	if out.SettleSeconds <= 0 {
		out.SettleSeconds = DefaultV7SettleSeconds
	}
	return out
}
