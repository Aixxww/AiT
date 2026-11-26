package store

// Hunter v7 higher-timeframe (1h/4h) direction gate (#122, #111 owner
// decision Q2). The prompt rule (S1, PR #121) is: the candidate's own 1h/4h
// structure sets direction, 5m/15m only timing. This file is the single code
// source for which setups the rule hard-gates and which get the downgrade
// channel, and for the downgrade channel's confidence floor.

// V7CounterTrendMinConfidence is the confidence floor for an open against
// the candidate's 1h/4h direction (owner-set, #122). It equals the prompt's
// high sizing band (>=85), the band S1 tells the LLM to require for such an
// open.
const V7CounterTrendMinConfidence = 85

// HTF gate families.
const (
	// V7HTFFamilyTrend setups never open against 1h/4h: their tier is held
	// at WATCH and an open is rejected.
	V7HTFFamilyTrend = "trend"
	// V7HTFFamilyCounterTrend setups may trade against 1h/4h through the
	// downgrade channel only: REVIEWABLE at most, and an open needs
	// confidence >= V7CounterTrendMinConfidence.
	V7HTFFamilyCounterTrend = "counter_trend"
)

// v7HTFFamilies is the owner-confirmed split from S1 (legacy doctrine S3):
// trend = Momentum continuation, Breakout, Ladder, breakdown_momentum_short,
// relative_weakness_short; counter-trend = Reversal repair,
// distribution_short, long_squeeze_short, range_reversion.
// range_expansion_event and pre_distribution_watch are in neither list and
// are not gated. Task-2 setup types are not listed (see V7HTFFamily).
var v7HTFFamilies = map[string]string{
	// Momentum continuation
	"leader_momentum_long":       V7HTFFamilyTrend,
	"short_squeeze_long":         V7HTFFamilyTrend,
	"displacement_momentum_long": V7HTFFamilyTrend,
	"mms_trend_ride_long":        V7HTFFamilyTrend,
	"mms_squeeze_engine_long":    V7HTFFamilyTrend,
	"intraday_scalp_long":        V7HTFFamilyTrend,
	// Breakout
	"trend_breakout_long":         V7HTFFamilyTrend,
	"accumulation_breakout_long":  V7HTFFamilyTrend,
	"volatility_squeeze_breakout": V7HTFFamilyTrend,
	"pre_breakout_watch":          V7HTFFamilyTrend,
	"pre_squeeze_watch":           V7HTFFamilyTrend,
	// Ladder
	"alt_ladder_momentum_long":   V7HTFFamilyTrend,
	"alt_ladder_breakdown_short": V7HTFFamilyTrend,
	// Weakness (trend side)
	"breakdown_momentum_short": V7HTFFamilyTrend,
	"relative_weakness_short":  V7HTFFamilyTrend,

	// Reversal repair
	"panic_reversal_long":    V7HTFFamilyCounterTrend,
	"pullback_reversal_long": V7HTFFamilyCounterTrend,
	"whale_flow_reversal":    V7HTFFamilyCounterTrend,
	"funding_reversal":       V7HTFFamilyCounterTrend,
	"mms_bottom_wake_long":   V7HTFFamilyCounterTrend,
	"accumulation_watch":     V7HTFFamilyCounterTrend,
	// Distribution (counter-trend side)
	"distribution_short": V7HTFFamilyCounterTrend,
	"long_squeeze_short": V7HTFFamilyCounterTrend,
	"range_reversion":    V7HTFFamilyCounterTrend,
}

// V7HTFFamily returns the HTF gate family of a native setup type, or "" for
// a setup the gate does not cover. Task-2 setups are not covered: their tier
// comes from the T2 lifecycle/class rules, and the S1 split names native
// families only.
func V7HTFFamily(setup string) string {
	return v7HTFFamilies[setup]
}

// V7HTFFamilies returns a copy of the gate table (for tests and audits).
func V7HTFFamilies() map[string]string {
	out := make(map[string]string, len(v7HTFFamilies))
	for k, v := range v7HTFFamilies {
		out[k] = v
	}
	return out
}
