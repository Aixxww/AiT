package store

import "time"

// F08: daily-loss breaker (Task A remainder; redesign plan §6.3, OD-15).
// Owner decisions (F08):
//   - mode off / shadow (default) / armed: shadow evaluates and records,
//     armed also enforces; armed is for pilot / live only;
//   - daily loss = drawdown from the equity at the start of the UTC day
//     (the day boundary is UTC midnight);
//   - a trip blocks new opens until the next UTC day boundary; existing
//     positions are never force-closed;
//   - every trip and near-trip evaluation is written to the decision
//     record's execution log; the day-start equity is persisted.

// Breaker modes.
const (
	DailyLossBreakerOff    = "off"
	DailyLossBreakerShadow = "shadow"
	DailyLossBreakerArmed  = "armed"
)

// Defaults. 15% is the owner-confirmed limit (Q5; plan OD-15: two full
// stops at 8% = 16% trip it). The near-trip level is 80% of the limit.
const (
	DefaultDailyLossBreakerMode         = DailyLossBreakerShadow
	DefaultDailyLossBreakerMaxLossPct   = 15.0
	DefaultDailyLossBreakerNearFraction = 0.8
)

// DailyLossBreakerConfig is RiskControlConfig.DailyLossBreaker (json
// daily_loss_breaker). Zero values mean the defaults.
type DailyLossBreakerConfig struct {
	Mode string `json:"mode,omitempty"`
	// MaxDailyLossPct is the trip level: loss from day-start equity, in
	// percent of day-start equity.
	MaxDailyLossPct float64 `json:"max_daily_loss_pct,omitempty"`
	// NearTripFraction: an evaluation with loss >= fraction x limit (and
	// below the limit) is a near-trip and is logged.
	NearTripFraction float64 `json:"near_trip_fraction,omitempty"`
}

// EffectiveDailyLossBreaker returns the config with defaults filled in.
// An unknown mode reads as the default (shadow), never as armed.
func EffectiveDailyLossBreaker(c *DailyLossBreakerConfig) DailyLossBreakerConfig {
	out := DailyLossBreakerConfig{}
	if c != nil {
		out = *c
	}
	switch out.Mode {
	case DailyLossBreakerOff, DailyLossBreakerShadow, DailyLossBreakerArmed:
	default:
		out.Mode = DefaultDailyLossBreakerMode
	}
	if out.MaxDailyLossPct <= 0 {
		out.MaxDailyLossPct = DefaultDailyLossBreakerMaxLossPct
	}
	if out.NearTripFraction <= 0 || out.NearTripFraction >= 1 {
		out.NearTripFraction = DefaultDailyLossBreakerNearFraction
	}
	return out
}

// DailyLossDay is the UTC day key of t ("2006-01-02").
func DailyLossDay(t time.Time) string { return t.UTC().Format("2006-01-02") }

// DailyLossBreakerState is the persisted per-trader state of one UTC day.
type DailyLossBreakerState struct {
	Day            string    `json:"day"`
	DayStartEquity float64   `json:"day_start_equity"`
	Tripped        bool      `json:"tripped"`
	TrippedAt      time.Time `json:"tripped_at,omitempty"`
	TripLossPct    float64   `json:"trip_loss_pct,omitempty"`
}

// DailyLossBreakerStateKey is the system_config key of a trader's state.
func DailyLossBreakerStateKey(traderID string) string { return "daily_loss_breaker:" + traderID }
