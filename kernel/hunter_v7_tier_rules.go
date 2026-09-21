package kernel

import (
	"strings"

	"github.com/Aixxww/AiT/provider/local"
)

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
	// "confirmed_at_least" / "confirmed_at_most" (missing data fails),
	// "aligned" (direction-dependent default thresholds).
	Kind      string
	Threshold float64
}

// hunterV7ZoneGate constrains entry-zone position %. Zero thresholds mean
// "no bound". RequireKnown fails closed when position cannot be computed.
type hunterV7ZoneGate struct {
	MaxPos       float64
	MinPos       float64
	RequireKnown bool
}

// hunterV7OIGate constrains derivatives OI change %. Zero thresholds mean
// "no bound". RequireCtx fails when DerivativesCtx is missing.
// AllowMissing lets a nil context pass (legacy "unknown OI does not block").
type hunterV7OIGate struct {
	MinChange1h  float64
	MaxChange1h  float64
	MinChange4h  float64
	RequireCtx   bool
	AllowMissing bool
}

// hunterV7StopGate constrains stop distance %. AllowUnknown passes when the
// distance cannot be computed (legacy soft-release behaviour).
type hunterV7StopGate struct {
	MaxPct       float64
	AllowUnknown bool
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
	Zone  hunterV7ZoneGate
	OI    hunterV7OIGate
	Stop  hunterV7StopGate

	// RequireInsideZone demands hunterV7PriceInsideEntryZone.
	RequireInsideZone bool
	// RequireConfirmAll demands hunterV7ConfirmationPassed for each code.
	RequireConfirmAll []string

	// Reason-code requirements over V7ReasonCodes.
	RequireAll []string
	RequireAny [][]string
	ForbidAll  []string
	// ForbidRiskAny rejects when any listed tag appears in V7RiskTags.
	ForbidRiskAny []string
	// RequireRiskAny requires at least one listed tag in V7RiskTags.
	RequireRiskAny []string

	// Guards carry the setup-specific predicates that are not yet (or not
	// worth) data-encoding; all must pass. Prefer Zone/OI/Stop/Forbid* first
	// (docs/hunter-v7-entropy-recoil-20260813.md).
	Guards []func(CandidateCoin) bool

	// Reason is the tier reason emitted when the rule matches. Rules whose
	// reason depends on the candidate set ReasonFunc instead: after the
	// matchers pass, it decides the final verdict and supplies the reason; a
	// false verdict falls through to the next rule.
	Reason     string
	ReasonFunc func(CandidateCoin) (bool, string)
}

type hunterV7SetupTierSpec struct {
	// EarlyWatch is evaluated before hard REJECTED gates that are setup-
	// agnostic only when the setup wants a visible WATCH (e.g. extreme
	// continuation). First match wins → ("WATCH", Reason).
	EarlyWatch []hunterV7TierRule
	// ConfirmWatchReason runs when required-confirmation wait fires. A
	// non-empty return overrides the wait reason with a setup-specific WATCH.
	ConfirmWatchReason func(coin CandidateCoin, waitReason string) string
	// Ready gates EXECUTABLE when execution quality is "ready" or the entry
	// signal is "entry_open_now"; NearConfirm gates EXECUTABLE for
	// "near_confirm"/candidate status; Reviewable gates REVIEWABLE.
	Ready       []hunterV7TierRule
	NearConfirm []hunterV7TierRule
	Reviewable  []hunterV7TierRule
	// OpenRateFloor gates live-reviewable escalation
	// (hunterV7OpenRateCandidateFloor); nil keeps the shared default floor.
	OpenRateFloor []hunterV7TierRule
	// PromptWait is the setup's prompt-time semantic override: after prompt
	// readiness is computed, a non-empty reason demotes EXECUTABLE/REVIEWABLE
	// to WATCH. Nil means the setup has no prompt-time override.
	PromptWait func(CandidateCoin, local.V7ExecutionReadiness) string
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
				hunterV7MMSLongLiveSupportOK,
			},
			Reason: "mms_long_ready_confirmed",
		},
	},
	Reviewable: []hunterV7TierRule{
		{
			MinAIPriority: 50, MinSetupScore: 55, MinTimingScore: 55, RiskBelow: 55,
			Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
			Guards: []func(CandidateCoin) bool{hunterV7MMSLongLiveSupportOK},
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

// hunterV7BreakoutLongTierSpec covers trend_breakout_long and
// accumulation_breakout_long (U3.3o). The cross-setup trigger-memory /
// trigger-near escalations stay in the shared pre-switch section of
// hunterV7ReviewableCandidateReason — they run before table dispatch.
var hunterV7BreakoutLongTierSpec = hunterV7SetupTierSpec{
	Ready: []hunterV7TierRule{
		{
			MinAIPriority: 60, MinSetupScore: 60, MinTimingScore: 60, RiskBelow: 55,
			Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
			Reason: "long_setup_ready_confirmed",
		},
	},
	Reviewable: []hunterV7TierRule{
		{
			MinAIPriority: 55, MinSetupScore: 58, MinTimingScore: 50, RiskBelow: 50,
			Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
			Reason: "long_setup_reviewable_needs_realtime_confirm",
		},
		{
			Quality:       "near_confirm",
			MinAIPriority: 45, MinSetupScore: 50, MinTimingScore: 45, RiskBelow: 35, MinLiquidity: 50,
			RequireAll: []string{"confirmed_breakout", "flow_taker_buy_aggressive"},
			Reason:     "breakout_reviewable_confirmed_low_risk_floor",
		},
		{
			Quality:       "near_confirm",
			MinAIPriority: 45, MinSetupScore: 38, MinTimingScore: 45, RiskBelow: 35, MinLiquidity: 60,
			Taker:      hunterV7TakerGate{Kind: "at_least", Threshold: 0.52},
			RequireAll: []string{"clear_air_above"},
			RequireAny: [][]string{
				{"approaching_breakout", "breakout_attempt", "confirmed_breakout"},
				{"volume_adequate", "oi_increasing", "oi_stable_breakout"},
			},
			Reason: "breakout_reviewable_low_risk_pressure_floor",
		},
		{
			Guards: []func(CandidateCoin) bool{hunterV7TrendBreakoutStrongFlowReviewable},
			Reason: "breakout_watch_strong_flow_reviewable",
		},
	},
	OpenRateFloor: []hunterV7TierRule{
		{
			MinAIPriority: 55, MinSetupScore: 55, MinTimingScore: 45, RiskBelow: 45,
			RequireAny: [][]string{
				{"approaching_breakout", "breakout_attempt", "confirmed_breakout"},
				{"volume_expansion", "volume_adequate", "oi_increasing", "oi_stable_breakout", "clear_air_above"},
			},
		},
	},
}

var hunterV7SetupTierSpecs = map[string]hunterV7SetupTierSpec{
	"trend_breakout_long":         hunterV7BreakoutLongTierSpec,
	"accumulation_breakout_long":  hunterV7BreakoutLongTierSpec,
	"volatility_squeeze_breakout": hunterV7DefaultOnlyTierSpec,
	"intraday_scalp_long":         hunterV7DefaultOnlyTierSpec,
	"pre_breakout_watch":          hunterV7WatchStateTierSpec,
	"pre_squeeze_watch":           hunterV7WatchStateTierSpec,
	"pre_distribution_watch":      hunterV7WatchStateTierSpec,
	"accumulation_watch":          hunterV7WatchStateTierSpec,
	"alt_ladder_momentum_long": {
		EarlyWatch: []hunterV7TierRule{
			{
				RequireRiskAny: []string{"alt_ladder_extreme_continuation_watch"},
				RequireAll:     []string{"alt_ladder_stage_extreme"},
				Reason:         "alt_ladder_extreme_continuation_watch",
			},
		},
		Ready: altLadderLongReadyRules,
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
				RequireAny: [][]string{{"oi_confirms_new_demand", "flow_taker_buy_aggressive", "flow_taker_buy_aligned"}},
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
	"funding_reversal": {
		Ready: []hunterV7TierRule{
			{
				Direction:     "SHORT",
				MinAIPriority: 50, MinTimingScore: 60, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_most", Threshold: 0.48},
				Reason: "funding_short_ready_core_ok",
			},
			{
				Direction:     "LONG",
				MinAIPriority: 65, MinTimingScore: 65, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.52},
				Reason: "funding_long_ready_strong_confirm",
			},
		},
		NearConfirm: []hunterV7TierRule{
			{
				Direction:     "SHORT",
				MinAIPriority: 55, MinTimingScore: 65, RiskBelow: 45,
				Taker:  hunterV7TakerGate{Kind: "at_most", Threshold: 0.45},
				Reason: "funding_short_near_confirm_core_ok",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				Direction:     "SHORT",
				MinAIPriority: 47, MinTimingScore: 55, RiskBelow: 60,
				Taker:  hunterV7TakerGate{Kind: "at_most", Threshold: 0.50},
				Reason: "funding_short_reviewable_crowding_reversal",
			},
			{
				Direction: "LONG", Quality: "ready",
				MinAIPriority: 60, MinTimingScore: 60, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.52},
				Reason: "funding_long_reviewable_strong_only",
			},
		},
	},
	"leader_momentum_long": {
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 65, MinSetupScore: 70, MinTimingScore: 65, RiskBelow: 45,
				Taker:     hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
				ForbidAll: []string{"flow_taker_buy_weak"},
				Guards: []func(CandidateCoin) bool{
					func(coin CandidateCoin) bool { return !hunterV7LeaderMomentumUpperChaseWait(coin) },
				},
				Reason: "momentum_ready_strong_flow",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				Quality:       "ready",
				MinAIPriority: 70, MinSetupScore: 75, MinTimingScore: 65, RiskBelow: 40,
				Taker:     hunterV7TakerGate{Kind: "at_least", Threshold: 0.48},
				ForbidAll: []string{"flow_taker_buy_weak"},
				Reason:    "momentum_reviewable_strong_but_needs_flow_check",
			},
			{
				Quality:       "ready",
				MinAIPriority: 75, MinSetupScore: 80, MinTimingScore: 62, RiskBelow: 25, MinLiquidity: 50,
				Taker:     hunterV7TakerGate{Kind: "at_least", Threshold: 0.50},
				ForbidAll: []string{"flow_taker_buy_weak"},
				Reason:    "momentum_reviewable_ready_priority_floor",
			},
			{
				MinAIPriority: 72, MinSetupScore: 80, MinTimingScore: 62, RiskBelow: 25, MinLiquidity: 80,
				Taker:  hunterV7TakerGate{Kind: "confirmed_at_least", Threshold: 0.52},
				Guards: []func(CandidateCoin) bool{hunterV7LeaderMomentumHasCleanPullback},
				Reason: "momentum_reviewable_high_priority_pullback",
			},
			{
				Quality:       "ready",
				MinAIPriority: 62, MinSetupScore: 55, MinTimingScore: 65, RiskBelow: 25, MinLiquidity: 60,
				Taker:      hunterV7TakerGate{Kind: "confirmed_at_least", Threshold: 0.52},
				RequireAll: []string{"strong_symbol_regime_override"},
				RequireAny: [][]string{
					{"solid_4h_momentum", "strong_4h_momentum"},
					{"solid_24h_momentum", "strong_24h_momentum"},
					{"oi_healthy_growth", "oi_moderate_growth"},
				},
				Guards: []func(CandidateCoin) bool{
					hunterV7LeaderMomentumHasCleanPullback,
					hunterV7ConfirmationSummaryReviewPassed,
				},
				Reason: "momentum_reviewable_confirmed_relative_strength",
			},
			{
				Quality:       "ready",
				MinAIPriority: 62, MinSetupScore: 55, MinTimingScore: 65, RiskBelow: 25, MinLiquidity: 80,
				Taker:      hunterV7TakerGate{Kind: "confirmed_at_least", Threshold: 0.50},
				RequireAll: []string{"strong_symbol_regime_override"},
				RequireAny: [][]string{
					{"solid_4h_momentum", "strong_4h_momentum"},
					{"solid_24h_momentum", "strong_24h_momentum"},
					{"oi_healthy_growth", "oi_moderate_growth"},
				},
				Guards: []func(CandidateCoin) bool{hunterV7LeaderMomentumHasCleanPullback},
				Reason: "momentum_reviewable_relative_strength_floor",
			},
			{ReasonFunc: hunterV7LeaderMomentumFlexibleReviewableReason},
		},
	},
	"panic_reversal_long": {
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 55, MinSetupScore: 45, MinTimingScore: 45, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.52},
				Reason: "panic_reversal_ready_core_ok",
			},
		},
		NearConfirm: []hunterV7TierRule{
			{
				MinAIPriority: 60, MinSetupScore: 55, MinTimingScore: 50, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: 0.52},
				Reason: "panic_reversal_near_confirm_core_ok",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				MinAIPriority: 45, MinSetupScore: 65, MinTimingScore: 35, RiskBelow: 45, MinLiquidity: 50,
				Taker:      hunterV7TakerGate{Kind: "confirmed_at_least", Threshold: 0.52},
				RequireAll: []string{"reviewable_floor_rescue"},
				Reason:     "panic_reversal_reviewable_floor_live_confirm",
			},
			{
				MinAIPriority: 50, MinSetupScore: 55, MinTimingScore: 30, RiskBelow: 55, MinLiquidity: 50,
				Taker:  hunterV7TakerGate{Kind: "confirmed_at_least", Threshold: 0.52},
				Guards: []func(CandidateCoin) bool{hunterV7PanicReversalHasHighWinReclaim},
				Reason: "panic_reversal_reviewable_high_win_reclaim",
			},
			{
				MinAIPriority: 50, MinSetupScore: 38, MinTimingScore: 40, RiskBelow: 60,
				Guards: []func(CandidateCoin) bool{hunterV7PanicReversalCoreFlowOK},
				Reason: "panic_reversal_reviewable_core_present",
			},
			{
				MinAIPriority: 45, MinSetupScore: 30, MinTimingScore: 45, RiskBelow: 35, MinLiquidity: 50,
				RequireAll: []string{"strong_reclaim"},
				RequireAny: [][]string{
					{"flow_taker_buy_strong", "flow_taker_buy_aggressive"},
					{"selling_decelerating", "selling_exhaustion"},
				},
				Reason: "panic_reversal_reviewable_capitulation_floor",
			},
		},
	},
	"range_expansion_event": {
		PromptWait: hunterV7RangeExpansionShortExhaustionPromptWait,
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 65, MinSetupScore: 65, MinTimingScore: 60, RiskBelow: 35,
				Guards: []func(CandidateCoin) bool{
					func(coin CandidateCoin) bool { return hunterV7ConfirmedRangeExpansionContinuation(coin, true) },
				},
				Reason: "range_expansion_ready_confirmed_continuation",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				MinAIPriority: 60, MinSetupScore: 60, MinTimingScore: 50, RiskBelow: 45,
				Guards: []func(CandidateCoin) bool{
					func(coin CandidateCoin) bool { return hunterV7ConfirmedRangeExpansionContinuation(coin, false) },
				},
				Reason: "range_expansion_reviewable_confirmed_continuation",
			},
		},
		OpenRateFloor: []hunterV7TierRule{
			{
				MinAIPriority: 58, MinSetupScore: 58, MinTimingScore: 50, RiskBelow: 40,
				RequireAny: [][]string{{"range_expansion_continuation", "range_expansion_retest", "retest_confirmed"}},
				ForbidAll:  []string{"range_expansion_late_chase", "range_expansion_exhaustion"},
			},
		},
	},
	"relative_weakness_short": {
		// Grind-down shorts: strong sell flow is the confirmation leg; the
		// floors sit slightly above the short/reversion family because this
		// module deliberately trades counter to up-regimes.
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 58, MinSetupScore: 55, MinTimingScore: 55, RiskBelow: 50,
				Taker:  hunterV7TakerGate{Kind: "at_most", Threshold: 0.45},
				Reason: "relative_weakness_ready_flow_confirmed",
			},
		},
		Reviewable: []hunterV7TierRule{
			{
				MinAIPriority: 50, MinSetupScore: 50, MinTimingScore: 50, RiskBelow: 55,
				Taker:  hunterV7TakerGate{Kind: "at_most", Threshold: 0.48},
				Reason: "relative_weakness_reviewable",
			},
		},
	},
	"whale_flow_reversal": {
		PromptWait: hunterV7WhaleFlowDataPromptWait,
		// Ready mirrors the trader's whale-flow LONG gates (zone position
		// <=45%, taker_buy_15m >= 0.56). SHORT keeps the score floor only —
		// trader does not apply the LONG zone/taker confirmation leg to shorts.
		Ready: []hunterV7TierRule{
			{
				Direction:     "LONG",
				MinAIPriority: 60, MinTimingScore: 60, RiskBelow: 55,
				Zone:   hunterV7ZoneGate{MaxPos: hunterV7WhaleLongMaxZonePos},
				Taker:  hunterV7TakerGate{Kind: "at_least", Threshold: hunterV7WhaleLongMinTaker},
				Reason: "whale_flow_ready_zone_and_flow_ok",
			},
			{
				Direction:     "SHORT",
				MinAIPriority: 60, MinTimingScore: 60, RiskBelow: 55,
				Reason: "whale_flow_ready_zone_and_flow_ok",
			},
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
		ConfirmWatchReason: hunterV7AltLadderShortConfirmWatchReason,
		Ready: []hunterV7TierRule{
			{
				MinAIPriority: 60, MinTimingScore: 65, RiskBelow: 35,
				Taker:             hunterV7TakerGate{Kind: "at_most", Threshold: 0.46},
				RequireAll:        []string{"alt_ladder_taker_sell"},
				RequireAny:        [][]string{{"alt_ladder_new_shorts", "alt_ladder_long_flush", "alt_ladder_sell_volume"}},
				RequireConfirmAll: []string{"no_new_high_after_rejection"},
				Reason:            "alt_ladder_short_ready_strong_confirmed",
			},
		},
		// Reviewable rows are ordered: soft-release → classic late/close-through/early.
		Reviewable: []hunterV7TierRule{
			altLadderShortSoftReleaseStandard,
			altLadderShortSoftReleaseStrong,
			altLadderShortClassicLate,
			altLadderShortClassicCloseThrough,
			altLadderShortClassicEarly,
		},
	},
}

// Alt-ladder soft-release thresholds (single source — Entropy Recoil E2.1).
// Tune here only; do not re-embed these literals in engine.go helpers.
const (
	hunterV7AltLadderSoftMaxZonePos    = 45.0
	hunterV7AltLadderSoftMinLiquidity  = 70.0
	hunterV7AltLadderSoftTakerStandard = 0.38
	hunterV7AltLadderSoftTakerStrong   = 0.34
	hunterV7AltLadderSoftStopStandard  = 2.25
	hunterV7AltLadderSoftStopStrong    = 2.45
	hunterV7AltLadderSoftMinOI1h       = 0.8

	hunterV7WhaleLongMaxZonePos = 45.0
	hunterV7WhaleLongMinTaker   = 0.56
)

var altLadderSoftReleaseShared = hunterV7TierRule{
	MinAIPriority: 52, MinTimingScore: 58, RiskAtMost: 45,
	MinLiquidity: hunterV7AltLadderSoftMinLiquidity,
	RequireAny: [][]string{
		{"alt_ladder_downshift_early", "alt_ladder_downshift_mid"},
	},
	RequireAll: []string{"alt_ladder_taker_sell", "alt_ladder_new_shorts"},
	ForbidRiskAny: []string{
		"high_volatility",
		"extreme_volatility",
		"alt_ladder_late_short_risk",
	},
	Zone: hunterV7ZoneGate{
		MaxPos:       hunterV7AltLadderSoftMaxZonePos,
		RequireKnown: true,
	},
	OI: hunterV7OIGate{
		MinChange1h: hunterV7AltLadderSoftMinOI1h,
		RequireCtx:  true,
	},
	Guards: []func(CandidateCoin) bool{hunterV7AltLadderSoftReleaseClear},
	Reason: "alt_ladder_short_reviewable_confirmed",
}

var altLadderShortSoftReleaseStandard = func() hunterV7TierRule {
	r := altLadderSoftReleaseShared
	r.Taker = hunterV7TakerGate{Kind: "confirmed_at_most", Threshold: hunterV7AltLadderSoftTakerStandard}
	r.Stop = hunterV7StopGate{MaxPct: hunterV7AltLadderSoftStopStandard, AllowUnknown: true}
	return r
}()

var altLadderShortSoftReleaseStrong = func() hunterV7TierRule {
	r := altLadderSoftReleaseShared
	r.Taker = hunterV7TakerGate{Kind: "confirmed_at_most", Threshold: hunterV7AltLadderSoftTakerStrong}
	r.Stop = hunterV7StopGate{MaxPct: hunterV7AltLadderSoftStopStrong, AllowUnknown: true}
	return r
}()

// Classic alt_ladder short reviewable channels (E2.2) — rebound-failure
// confirmed paths after soft-release rows miss.
var altLadderShortClassicFlow = [][]string{
	{"alt_ladder_new_shorts", "alt_ladder_long_flush", "alt_ladder_sell_volume"},
}
var altLadderShortClassicCloseThroughCodes = [][]string{
	{"alt_ladder_multi_cycle_close_through", "trigger_memory_confirmed"},
}

var altLadderShortClassicLate = hunterV7TierRule{
	MinAIPriority:     52,
	MinTimingScore:    58,
	RiskAtMost:        45,
	RequireConfirmAll: []string{"no_new_high_after_rejection"},
	RequireAll:        []string{"alt_ladder_taker_sell", "alt_ladder_downshift_late"},
	RequireAny:        append(append([][]string{}, altLadderShortClassicFlow...), altLadderShortClassicCloseThroughCodes...),
	ForbidRiskAny:     []string{"alt_ladder_late_short_risk"},
	Taker:             hunterV7TakerGate{Kind: "confirmed_at_most", Threshold: 0.46},
	Reason:            "alt_ladder_short_reviewable_confirmed",
}

var altLadderShortClassicCloseThrough = hunterV7TierRule{
	MinAIPriority:     52,
	MinTimingScore:    58,
	RiskAtMost:        45,
	RequireConfirmAll: []string{"no_new_high_after_rejection"},
	RequireAny:        append(append([][]string{}, altLadderShortClassicFlow...), altLadderShortClassicCloseThroughCodes...),
	ForbidAll:         []string{"alt_ladder_downshift_late"},
	ForbidRiskAny:     []string{"alt_ladder_late_short_risk"},
	Taker:             hunterV7TakerGate{Kind: "confirmed_at_most", Threshold: 0.48},
	Reason:            "alt_ladder_short_reviewable_confirmed",
}

var altLadderShortClassicEarly = hunterV7TierRule{
	MinAIPriority:     52,
	MinTimingScore:    58,
	RiskAtMost:        45,
	RequireConfirmAll: []string{"no_new_high_after_rejection"},
	RequireAll:        []string{"alt_ladder_taker_sell"},
	RequireAny:        altLadderShortClassicFlow,
	ForbidAll:         []string{"alt_ladder_downshift_late"},
	ForbidRiskAny:     []string{"alt_ladder_late_short_risk"},
	Taker:             hunterV7TakerGate{Kind: "confirmed_at_most", Threshold: 0.46},
	Reason:            "alt_ladder_short_reviewable_confirmed",
}

// alt_ladder_momentum_long Ready floors (E2.3). Residual late/OI/stop logic
// stays in hunterV7AltLadderLongExecutableExtras.
var altLadderLongReadyRules = []hunterV7TierRule{
	{
		MinAIPriority:     58,
		MinSetupScore:     58,
		MinTimingScore:    60,
		RiskBelow:         55,
		RequireInsideZone: true,
		Taker:             hunterV7TakerGate{Kind: "confirmed_at_least", Threshold: 0.55},
		RequireAll:        []string{"alt_ladder_taker_buy"},
		RequireAny:        [][]string{{"alt_ladder_oi_inflow", "alt_ladder_volume_expansion"}},
		ForbidRiskAny:     []string{"fresh_oi_absent"},
		Guards:            []func(CandidateCoin) bool{hunterV7AltLadderLongExecutableExtras},
		Reason:            "alt_ladder_long_ready_confirmed",
	},
}

// hunterV7AltLadderLongExecutableExtras carries the residual long-side gates
// that cannot be represented by a single tier-rule field. The common score,
// entry-zone, taker, and participation requirements live in
// altLadderLongReadyRules above.
func hunterV7AltLadderLongExecutableExtras(coin CandidateCoin) bool {
	if containsAnyStringValue(coin.V7RiskTags, []string{"alt_ladder_late_chase_risk", "high_volatility"}) &&
		!containsStringValue(coin.V7ReasonCodes, "alt_ladder_oi_inflow") {
		return false
	}
	if coin.V7DerivativesCtx != nil && coin.V7DerivativesCtx.OIChange4h < -3 &&
		!containsStringValue(coin.V7ReasonCodes, "alt_ladder_oi_inflow") {
		return false
	}
	if hunterV7AltLadderLateLongNeedsFreshFlow(coin) {
		return false
	}
	if containsStringValue(coin.V7RiskTags, "execution_stop_tightened") {
		if !hunterV7TakerBuyConfirmedAtLeast(coin, 0.58) {
			return false
		}
		if !containsStringValue(coin.V7ReasonCodes, "alt_ladder_oi_inflow") &&
			(coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.OIChange1h < 0) {
			return false
		}
	}
	return true
}

// hunterV7AltLadderShortConfirmWatchReason preserves the setup-specific
// rebound-pending WATCH while the required confirmation has not arrived.
func hunterV7AltLadderShortConfirmWatchReason(coin CandidateCoin, waitReason string) string {
	if hunterV7AltLadderShortReboundPending(coin, waitReason) {
		return "alt_ladder_short_rebound_pending"
	}
	return ""
}

// hunterV7RangeExpansionShortExhaustionPromptWait parks a deeply-fallen
// range-expansion SHORT for a retest when the move already looks exhausted.
func hunterV7RangeExpansionShortExhaustionPromptWait(coin CandidateCoin, _ local.V7ExecutionReadiness) string {
	if !strings.EqualFold(coin.Direction, "SHORT") {
		return ""
	}
	change24h := 0.0
	if coin.V7PriceContext != nil {
		change24h = coin.V7PriceContext.Change24h
	}
	if change24h <= -12 &&
		containsAnyStringValue(coin.V7RiskTags, []string{
			"event_chase_risk",
			"event_flow_confirmation_needed",
			"range_expansion_low_volume_followthrough",
			"short_covering_not_new_long_build",
		}) {
		return "range_expansion_short_exhaustion_retest_wait"
	}
	return ""
}

// hunterV7WhaleFlowDataPromptWait holds whale-flow entries until the prompt
// window has complete execution data.
func hunterV7WhaleFlowDataPromptWait(_ CandidateCoin, readiness local.V7ExecutionReadiness) string {
	if readiness.DataQuality != "complete_for_execution" ||
		readiness.BlockedGate == "prompt_data_quality" ||
		len(readiness.MissingHard) > 0 ||
		len(readiness.MissingExecution) > 0 {
		return "whale_flow_execution_data_wait"
	}
	return ""
}

// hunterV7WhaleFlowLongEntryGatesOK mirrors the trader-side whale-flow LONG
// protections. Prefer the table Zone/Taker fields (E3.1); this helper remains
// for direct unit tests and any residual call sites.
func hunterV7WhaleFlowLongEntryGatesOK(coin CandidateCoin) bool {
	if !strings.EqualFold(coin.Direction, "LONG") {
		return true
	}
	return hunterV7ZoneGateMatches(coin, hunterV7ZoneGate{MaxPos: hunterV7WhaleLongMaxZonePos}) &&
		hunterV7TakerBuyAtLeast(coin, hunterV7WhaleLongMinTaker)
}

func hunterV7EvalTierRules(coin CandidateCoin, rules []hunterV7TierRule) (bool, string) {
	for i := range rules {
		if !hunterV7TierRuleMatches(coin, &rules[i]) {
			continue
		}
		if rules[i].ReasonFunc != nil {
			if ok, reason := rules[i].ReasonFunc(coin); ok {
				return true, reason
			}
			continue
		}
		return true, rules[i].Reason
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
	case "confirmed_at_most":
		if !hunterV7TakerBuyConfirmedAtMost(coin, rule.Taker.Threshold) {
			return false
		}
	case "aligned":
		if !hunterV7TakerBuyAligned(coin) {
			return false
		}
	}
	if !hunterV7ZoneGateMatches(coin, rule.Zone) {
		return false
	}
	if !hunterV7OIGateMatches(coin, rule.OI) {
		return false
	}
	if !hunterV7StopGateMatches(coin, rule.Stop) {
		return false
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
	for _, tag := range rule.ForbidRiskAny {
		if containsStringValue(coin.V7RiskTags, tag) {
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

func hunterV7ZoneGateMatches(coin CandidateCoin, gate hunterV7ZoneGate) bool {
	if gate.MaxPos <= 0 && gate.MinPos <= 0 && !gate.RequireKnown {
		return true
	}
	pos, ok := hunterV7EntryZonePositionPct(coin)
	if !ok {
		return !gate.RequireKnown
	}
	if gate.MaxPos > 0 && pos > gate.MaxPos {
		return false
	}
	if gate.MinPos > 0 && pos < gate.MinPos {
		return false
	}
	return true
}

func hunterV7OIGateMatches(coin CandidateCoin, gate hunterV7OIGate) bool {
	if gate.MinChange1h == 0 && gate.MaxChange1h == 0 && gate.MinChange4h == 0 && !gate.RequireCtx {
		return true
	}
	if coin.V7DerivativesCtx == nil {
		return false
	}
	if gate.MinChange1h != 0 && coin.V7DerivativesCtx.OIChange1h < gate.MinChange1h {
		return false
	}
	if gate.MaxChange1h != 0 && coin.V7DerivativesCtx.OIChange1h > gate.MaxChange1h {
		return false
	}
	if gate.MinChange4h != 0 && coin.V7DerivativesCtx.OIChange4h < gate.MinChange4h {
		return false
	}
	return true
}

func hunterV7StopGateMatches(coin CandidateCoin, gate hunterV7StopGate) bool {
	if gate.MaxPct <= 0 && !gate.AllowUnknown {
		return true
	}
	distance := hunterV7StopDistancePct(coin)
	if distance <= 0 {
		return gate.AllowUnknown || gate.MaxPct <= 0
	}
	if gate.MaxPct > 0 && distance > gate.MaxPct {
		return false
	}
	return true
}

// hunterV7AltLadderSoftReleaseClear is the residual composite hard-block for
// soft-release rows (danger tags, funding∧stop-tightened, late-without-close-
// through, raw taker buy rebound). Threshold tags/zone/OI/stop live in the
// table rows above — do not add numeric literals here.
func hunterV7AltLadderSoftReleaseClear(coin CandidateCoin) bool {
	return !hunterV7AltLadderShortLayeredReleaseHardBlock(coin)
}
