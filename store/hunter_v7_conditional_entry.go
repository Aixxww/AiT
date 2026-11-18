package store

// U9: TRIGGER-class conditional entry (plan §3.3, §6.2 "Execution: trigger
// entry"; owner decision 2026-11-18: option A, backend trigger check on
// every completed 1-minute candle, default off).
//
// When enabled, a Task-2 candidate of a conditional class that is still
// WATCH or ARMED (its price has not reached the trigger) is reviewable with
// entry permission "conditional_on_trigger". An approved open is not sent
// to the exchange: the trader keeps it as a pending entry and opens it,
// through the normal open path, on the first completed 1m candle that
// closes through the trigger. A candle that touches the invalidation first
// cancels it, and it expires after MaxPendingMinutes. Pending entries live
// in memory only, so a restart drops them (they are never re-executed).

// V7ConditionalEntryConfig is HunterConfig.V7ConditionalEntry.
type V7ConditionalEntryConfig struct {
	Enabled bool `json:"enabled,omitempty"`
	// Classes are the signal classes that may enter conditionally.
	Classes []string `json:"classes,omitempty"`
	// MaxPendingMinutes is a pending entry's lifetime.
	MaxPendingMinutes float64 `json:"max_pending_minutes,omitempty"`
	// CheckIntervalSeconds is how often the trader looks for newly
	// completed 1m candles (the entry delay after a candle closes).
	CheckIntervalSeconds float64 `json:"check_interval_seconds,omitempty"`
}

// Defaults.
var DefaultV7ConditionalEntryClasses = []string{V7ClassTrigger}

const (
	// DefaultV7ConditionalEntryMaxPendingMinutes equals the T2 WATCH event
	// lifetime (scanner spec 5.2: 2 h), the longest a not-yet-triggered
	// event stays live.
	DefaultV7ConditionalEntryMaxPendingMinutes = 120
	// DefaultV7ConditionalEntryCheckIntervalSeconds keeps the delay after a
	// candle closes well inside one candle.
	DefaultV7ConditionalEntryCheckIntervalSeconds = 10
)

// EffectiveV7ConditionalEntry returns the config with defaults filled in
// (Enabled stays false unless set).
func EffectiveV7ConditionalEntry(c *V7ConditionalEntryConfig) V7ConditionalEntryConfig {
	out := V7ConditionalEntryConfig{}
	if c != nil {
		out = *c
	}
	if len(out.Classes) == 0 {
		out.Classes = append([]string(nil), DefaultV7ConditionalEntryClasses...)
	}
	if out.MaxPendingMinutes <= 0 {
		out.MaxPendingMinutes = DefaultV7ConditionalEntryMaxPendingMinutes
	}
	if out.CheckIntervalSeconds <= 0 {
		out.CheckIntervalSeconds = DefaultV7ConditionalEntryCheckIntervalSeconds
	}
	return out
}

// ClassSet returns the enabled classes as a set (empty when disabled).
func (c V7ConditionalEntryConfig) ClassSet() map[string]bool {
	out := map[string]bool{}
	if !c.Enabled {
		return out
	}
	for _, cl := range c.Classes {
		out[cl] = true
	}
	return out
}
