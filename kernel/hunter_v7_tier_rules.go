package kernel

import "strings"

// Hunter v7 tier rules (U3.2)
//
// Each setup's EXECUTABLE/REVIEWABLE gating collapses into ordered rule rows:
// a row matches when every populated field passes, and the first match wins.
// Setups registered in hunterV7SetupTierSpecs are evaluated from the table;
// unregistered setups keep the legacy switch branches until their migration
// unit (U3.3a-u) lands. The shadow test in hunter_v7_tier_shadow_test.go
// replays frozen copies of the legacy switches against the table and fails on
// any divergence.

type hunterV7TakerGate struct {
	// Kind selects the taker predicate: "" (no gate), "at_least", "at_most",
	// "confirmed_at_least" (missing data fails instead of passing), "aligned"
	// (direction-dependent default thresholds).
	Kind      string
	Threshold float64
}

type hunterV7TierRule struct {
	// Identity matchers; empty means "any".
	Direction   string
	Quality     string
	Status      string
	EntrySignal string

	// Score floors; zero means "no floor". RiskBelow is exclusive (<),
	// RiskAtMost inclusive (<=) — the legacy branches use both shapes.
	MinAIPriority  float64
	MinSetupScore  float64
	MinTimingScore float64
	RiskBelow      float64
	RiskAtMost     float64
	// MinLiquidity encodes the recurring "(liquidity == 0 || liquidity >= X)"
	// pattern: unknown liquidity passes, known liquidity must clear the floor.
	MinLiquidity float64

	Taker hunterV7TakerGate

	// Reason-code requirements over V7ReasonCodes.
	RequireAll []string
	RequireAny [][]string
	ForbidAll  []string

	// Guards carry the setup-specific predicates that are not yet (or not
	// worth) data-encoding; all must pass.
	Guards []func(CandidateCoin) bool

	// Reason is the tier reason emitted when the rule matches.
	Reason string
}

type hunterV7SetupTierSpec struct {
	// Ready gates EXECUTABLE when execution quality is "ready" or the entry
	// signal is "entry_open_now"; NearConfirm gates EXECUTABLE for
	// "near_confirm"/candidate status; Reviewable gates REVIEWABLE.
	Ready       []hunterV7TierRule
	NearConfirm []hunterV7TierRule
	Reviewable  []hunterV7TierRule
	// OpenRateFloor gates live-reviewable escalation
	// (hunterV7OpenRateCandidateFloor); nil keeps the shared default floor.
	OpenRateFloor []hunterV7TierRule
}

// hunterV7SetupTierSpecs is the per-setup rule registry. Populated setup by
// setup in U3.3; each entry replaces every legacy switch branch for that setup.
// hunterV7ShortOrReversionTierSpec covers the four setups that shared the
// "short_or_reversion" legacy branches (U3.3a). OpenRateFloor stays nil: these
// setups always used the shared default floor.
var hunterV7ShortOrReversionTierSpec = hunterV7SetupTierSpec{
	Ready: []hunterV7TierRule{
		{MinAIPriority: 55, MinTimingScore: 55, RiskBelow: 55, Reason: "short_or_reversion_ready_confirmed"},
	},
	Reviewable: []hunterV7TierRule{
		{MinAIPriority: 50, MinTimingScore: 50, RiskBelow: 55, Reason: "short_or_reversion_reviewable"},
	},
}

// hunterV7MMSLongTierSpec covers mms_trend_ride_long / mms_squeeze_engine_long
// (U3.3c). The freshness and chase-block predicates stay as guards: they read
// data freshness and price-vs-zone geometry, not plain score floors.
var hunterV7MMSLongTierSpec = hunterV7SetupTierSpec{
	Ready: []hunterV7TierRule{
		{
			MinAIPriority: 58, MinSetupScore: 60, MinTimingScore: 60, RiskBelow: 55,
			Taker: hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
			Guards: []func(CandidateCoin) bool{
				hunterV7MMSLongExecutableFreshEnough,
				func(coin CandidateCoin) bool { return !hunterV7MMSLongExecutableChaseBlock(coin) },
			},
			Reason: "mms_long_ready_confirmed",
		},
	},
	Reviewable: []hunterV7TierRule{
		{
			MinAIPriority: 50, MinSetupScore: 55, MinTimingScore: 55, RiskBelow: 55,
			Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
			Reason: "mms_long_reviewable_confirmed",
		},
	},
}

// hunterV7WatchStateTierSpec covers the four pre-signal watch setups (U3.3h):
// generic default ready floor, upgrade-predicate reviewable.
var hunterV7WatchStateTierSpec = hunterV7SetupTierSpec{
	Ready: []hunterV7TierRule{
		{MinAIPriority: 60, MinTimingScore: 60, RiskBelow: 55, Reason: "execution_quality_ready"},
	},
	Reviewable: []hunterV7TierRule{
		{
			Guards: []func(CandidateCoin) bool{hunterV7WatchUpgradedReviewable},
			Reason: "watch_state_upgraded_reviewable",
		},
	},
}

// hunterV7DefaultOnlyTierSpec covers setups whose gating was entirely the
// generic default ready floor: no reviewable branch, shared open-rate floor.
var hunterV7DefaultOnlyTierSpec = hunterV7SetupTierSpec{
	Ready: []hunterV7TierRule{
		{MinAIPriority: 60, MinTimingScore: 60, RiskBelow: 55, Reason: "execution_quality_ready"},
	},
}

var hunterV7SetupTierSpecs = map[string]hunterV7SetupTierSpec{
	"volatility_squeeze_breakout": hunterV7DefaultOnlyTierSpec,
	"intraday_scalp_long":         hunterV7DefaultOnlyTierSpec,
	"pre_breakout_watch":     hunterV7WatchStateTierSpec,
	"pre_squeeze_watch":      hunterV7WatchStateTierSpec,
	"pre_distribution_watch": hunterV7WatchStateTierSpec,
	"accumulation_watch":     hunterV7WatchStateTierSpec,
	"alt_ladder_momentum_long": {
		Ready: []hunterV7TierRule{
			// The ladder-stage/oi/volume resonance lives in one composite
			// predicate; the row is just its carrier.
			{
				Guards: []func(CandidateCoin) bool{hunterV7AltLadderLongExecutable},
				Reason: "alt_ladder_long_ready_confirmed",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				MinAIPriority: 50, MinSetupScore: 55, MinTimingScore: 52, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
				Reason: "alt_ladder_long_reviewable_confirmed",
			},
		},
	},
	"displacement_momentum_long": {
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 55, MinSetupScore: 55, MinTimingScore: 50, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
				Reason: "displacement_ready_confirmed",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				MinAIPriority: 48, MinSetupScore: 50, MinTimingScore: 40, RiskBelow: 55, MinLiquidity: 50,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
				Reason: "displacement_reviewable_needs_confirm",
			},
		},
		OpenRateFloor: []hunterV7TierRule{
			{
				MinAIPriority: 60, MinSetupScore: 70, MinTimingScore: 50, RiskBelow: 45,
				ForbidAll:  []string{"chase_high_protection"},
				RequireAny: [][]string{{"oi_confirms_new_demand", "taker_buy_aggressive", "taker_buy_aligned"}},
			},
		},
	},
	"mms_bottom_wake_long": {
		// This setup had no dedicated ready branch: it fell through to the
		// generic default, which the first row reproduces explicitly.
		Ready: []hunterV7TierRule{
			{MinAIPriority: 60, MinTimingScore: 60, RiskBelow: 55, Reason: "execution_quality_ready"},
		},
		Reviewable: []hunterV7TierRule{
			{
				MinAIPriority: 45, MinSetupScore: 48, RiskBelow: 45, MinLiquidity: 50,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.48},
				Reason: "mms_bottom_wake_reviewable_breakout_required",
			},
		},
	},
	"short_squeeze_long": {
		// Shared long-setup ready floor; the setup never had a reviewable
		// branch of its own.
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 60, MinSetupScore: 60, MinTimingScore: 60, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
				Reason: "long_setup_ready_confirmed",
			},
		},
	},
	"pullback_reversal_long": {
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 60, MinSetupScore: 60, MinTimingScore: 60, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
				Reason: "long_setup_ready_confirmed",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				MinAIPriority: 48, MinSetupScore: 70, MinTimingScore: 55, RiskBelow: 45, MinLiquidity: 50,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
				Reason: "pullback_reviewable_strong_structure",
			},
		},
		OpenRateFloor: []hunterV7TierRule{
			{
				MinAIPriority: 48, MinSetupScore: 60, MinTimingScore: 50, RiskBelow: 45,
				RequireAny: [][]string{{"healthy_pullback", "near_4h_support", "strong_reclaim"}},
			},
		},
	},
	"whale_flow_reversal": {
		// Default ready floor; no reviewable path existed for this setup —
		// its open-review escalation runs through the open-rate floor below.
		Ready: []hunterV7TierRule{
			{MinAIPriority: 60, MinTimingScore: 60, RiskBelow: 55, Reason: "execution_quality_ready"},
		},
		OpenRateFloor: []hunterV7TierRule{
			{
				MinAIPriority: 48, MinSetupScore: 48, MinTimingScore: 50, RiskBelow: 45,
				RequireAll: []string{"whale_flow_detected"},
				RequireAny: [][]string{{"oi_1h_confirming_accumulation", "stealth_accumulation_breakout", "funding_not_crowded"}},
			},
		},
	},
	"mms_trend_ride_long":      hunterV7MMSLongTierSpec,
	"mms_squeeze_engine_long":  hunterV7MMSLongTierSpec,
	"distribution_short":       hunterV7ShortOrReversionTierSpec,
	"long_squeeze_short":       hunterV7ShortOrReversionTierSpec,
	"breakdown_momentum_short": hunterV7ShortOrReversionTierSpec,
	"range_reversion":          hunterV7ShortOrReversionTierSpec,
	"alt_ladder_breakdown_short": {
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 60, MinTimingScore: 65, RiskBelow: 35,
				Taker:      hunterV7TakerGate{Kind: "at_most", Threshold: 0.46},
				RequireAll: []string{"alt_ladder_taker_sell"},
				RequireAny: [][]string{{"alt_ladder_new_shorts", "alt_ladder_long_flush", "alt_ladder_sell_volume"}},
				Reason:     "alt_ladder_short_ready_strong_confirmed",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				MinAIPriority: 52, MinTimingScore: 58, RiskAtMost: 45,
				Taker:  hunterV7TakerGate{Kind: "at_most", Threshold: 0.48},
				Reason: "alt_ladder_short_reviewable_confirmed",
			},
		},
	},
}

func hunterV7EvalTierRules(coin CandidateCoin, rules []hunterV7TierRule) (bool, string) {
	for i := range rules {
		if hunterV7TierRuleMatches(coin, &rules[i]) {
			return true, rules[i].Reason
		}
	}
	return false, ""
}

func hunterV7TierRuleMatches(coin CandidateCoin, rule *hunterV7TierRule) bool {
	if rule.Direction != "" && !strings.EqualFold(coin.Direction, rule.Direction) {
		return false
	}
	if rule.Quality != "" && coin.V7ExecutionQuality != rule.Quality {
		return false
	}
	if rule.Status != "" && coin.V7Status != rule.Status {
		return false
	}
	if rule.EntrySignal != "" && coin.V7EntrySignal != rule.EntrySignal {
		return false
	}
	if coin.V7AIPriority < rule.MinAIPriority {
		return false
	}
	if coin.V7SetupScore < rule.MinSetupScore {
		return false
	}
	if coin.V7TimingScore < rule.MinTimingScore {
		return false
	}
	if rule.RiskBelow > 0 && coin.V7RiskScore >= rule.RiskBelow {
		return false
	}
	if rule.RiskAtMost > 0 && coin.V7RiskScore > rule.RiskAtMost {
		return false
	}
	if rule.MinLiquidity > 0 && coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < rule.MinLiquidity {
		return false
	}
	switch rule.Taker.Kind {
	case "at_least":
		if !hunterV7TakerBuyAtLeast(coin, rule.Taker.Threshold) {
			return false
		}
	case "at_most":
		if !hunterV7TakerBuyAtMost(coin, rule.Taker.Threshold) {
			return false
		}
	case "confirmed_at_least":
		if !hunterV7TakerBuyConfirmedAtLeast(coin, rule.Taker.Threshold) {
			return false
		}
	case "aligned":
		if !hunterV7TakerBuyAligned(coin) {
			return false
		}
	}
	for _, code := range rule.RequireAll {
		if !containsStringValue(coin.V7ReasonCodes, code) {
			return false
		}
	}
	for _, group := range rule.RequireAny {
		if !containsAnyStringValue(coin.V7ReasonCodes, group) {
			return false
		}
	}
	for _, code := range rule.ForbidAll {
		if containsStringValue(coin.V7ReasonCodes, code) {
			return false
		}
	}
	for _, guard := range rule.Guards {
		if !guard(coin) {
			return false
		}
	}
	return true
}
