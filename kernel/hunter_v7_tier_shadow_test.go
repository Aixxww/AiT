package kernel

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Shadow harness (U3.2): frozen copies of the legacy per-setup switches are
// replayed against the production entry points (which dispatch registered
// setups through hunterV7SetupTierSpecs). Any divergence — on the golden live
// cycle or on the synthetic threshold grid — fails the test. A setup migration
// (U3.3x) is only allowed to land with this test green; the harness itself is
// deleted in U3.4 together with the legacy switches.
//
// The legacy copies below are verbatim snapshots taken before any setup was
// migrated. Do NOT update them when tuning thresholds: threshold changes go
// into the rule table, and intentional behavior changes require re-freezing
// the copy in the same commit with an explicit note.

func hunterV7ReadyExecutableReasonLegacy(coin CandidateCoin) (bool, string) {
	switch coin.V7SetupType {
	case "panic_reversal_long":
		if coin.V7AIPriority >= 55 &&
			coin.V7SetupScore >= 45 &&
			coin.V7TimingScore >= 45 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.52) {
			return true, "panic_reversal_ready_core_ok"
		}
	case "funding_reversal":
		if strings.EqualFold(coin.Direction, "SHORT") &&
			coin.V7AIPriority >= 50 &&
			coin.V7TimingScore >= 60 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtMost(coin, 0.48) {
			return true, "funding_short_ready_core_ok"
		}
		if strings.EqualFold(coin.Direction, "LONG") &&
			coin.V7AIPriority >= 65 &&
			coin.V7TimingScore >= 65 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.52) {
			return true, "funding_long_ready_strong_confirm"
		}
	case "leader_momentum_long":
		if hunterV7LeaderMomentumUpperChaseWait(coin) {
			return false, ""
		}
		if coin.V7AIPriority >= 65 &&
			coin.V7SetupScore >= 70 &&
			coin.V7TimingScore >= 65 &&
			coin.V7RiskScore < 45 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) &&
			!containsStringValue(coin.V7ReasonCodes, "taker_weak_buy") {
			return true, "momentum_ready_strong_flow"
		}
	case "trend_breakout_long", "accumulation_breakout_long", "pullback_reversal_long", "short_squeeze_long":
		if coin.V7AIPriority >= 60 &&
			coin.V7SetupScore >= 60 &&
			coin.V7TimingScore >= 60 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) {
			return true, "long_setup_ready_confirmed"
		}
	case "mms_trend_ride_long", "mms_squeeze_engine_long":
		if coin.V7AIPriority >= 58 &&
			coin.V7SetupScore >= 60 &&
			coin.V7TimingScore >= 60 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) &&
			hunterV7MMSLongExecutableFreshEnough(coin) &&
			!hunterV7MMSLongExecutableChaseBlock(coin) {
			return true, "mms_long_ready_confirmed"
		}
	case "alt_ladder_momentum_long":
		if hunterV7AltLadderLongExecutable(coin) {
			return true, "alt_ladder_long_ready_confirmed"
		}
	case "displacement_momentum_long":
		if coin.V7AIPriority >= 55 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) {
			return true, "displacement_ready_confirmed"
		}
	case "range_expansion_event":
		if hunterV7ConfirmedRangeExpansionContinuation(coin, true) &&
			coin.V7AIPriority >= 65 &&
			coin.V7SetupScore >= 65 &&
			coin.V7TimingScore >= 60 &&
			coin.V7RiskScore < 35 {
			return true, "range_expansion_ready_confirmed_continuation"
		}
	case "alt_ladder_breakdown_short":
		if coin.V7AIPriority >= 60 &&
			coin.V7TimingScore >= 65 &&
			coin.V7RiskScore < 35 &&
			hunterV7TakerBuyAtMost(coin, 0.46) &&
			containsStringValue(coin.V7ReasonCodes, "alt_ladder_taker_sell") &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"alt_ladder_new_shorts", "alt_ladder_long_flush", "alt_ladder_sell_volume"}) {
			return true, "alt_ladder_short_ready_strong_confirmed"
		}
	case "distribution_short", "long_squeeze_short", "breakdown_momentum_short", "range_reversion":
		if coin.V7AIPriority >= 55 &&
			coin.V7TimingScore >= 55 &&
			coin.V7RiskScore < 55 {
			return true, "short_or_reversion_ready_confirmed"
		}
	default:
		if coin.V7AIPriority >= 60 && coin.V7TimingScore >= 60 && coin.V7RiskScore < 55 {
			return true, "execution_quality_ready"
		}
	}
	return false, ""
}

func hunterV7NearConfirmExecutableReasonLegacy(coin CandidateCoin) (bool, string) {
	switch coin.V7SetupType {
	case "panic_reversal_long":
		if coin.V7AIPriority >= 60 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.52) {
			return true, "panic_reversal_near_confirm_core_ok"
		}
	case "funding_reversal":
		if strings.EqualFold(coin.Direction, "SHORT") &&
			coin.V7AIPriority >= 55 &&
			coin.V7TimingScore >= 65 &&
			coin.V7RiskScore < 45 &&
			hunterV7TakerBuyAtMost(coin, 0.45) {
			return true, "funding_short_near_confirm_core_ok"
		}
	}
	return false, ""
}

func hunterV7ReviewableCandidateReasonLegacy(coin CandidateCoin) (bool, string) {
	if coin.V7RiskScore >= 65 || hunterV7DangerRiskTagBlocksOpenReview(coin) {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false, ""
	}
	if coin.V7ExecutionQuality == "near_confirm" &&
		coin.V7AIPriority >= 45 &&
		coin.V7SetupScore >= 45 &&
		coin.V7TimingScore >= 45 &&
		coin.V7RiskScore < 45 &&
		hunterV7TakerBuyAligned(coin) &&
		hunterV7ConfirmationSummaryReviewPassed(coin) {
		return true, "near_confirm_reviewable_micro_confirmed"
	}
	if containsStringValue(coin.V7ReasonCodes, "trigger_memory_confirmed") &&
		(coin.V7SetupType == "trend_breakout_long" || coin.V7SetupType == "accumulation_breakout_long") &&
		coin.V7AIPriority >= 45 &&
		coin.V7SetupScore >= 50 &&
		coin.V7RiskScore < 55 &&
		(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 70) &&
		hunterV7TakerBuyAligned(coin) {
		return true, "breakout_trigger_memory_confirmed_reviewable"
	}
	if coin.V7EntrySignal == "entry_trigger_near" &&
		(coin.V7SetupType == "trend_breakout_long" || coin.V7SetupType == "accumulation_breakout_long") &&
		coin.V7SetupScore >= 78 &&
		coin.V7AIPriority >= 45 &&
		coin.V7RiskScore < 55 &&
		(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 70) &&
		hunterV7TakerBuyAligned(coin) {
		return true, "breakout_trigger_near_reviewable"
	}
	if hunterV7BreakoutTriggerNearFlowReviewable(coin) {
		return true, "breakout_trigger_near_flow_reviewable"
	}
	switch coin.V7SetupType {
	case "panic_reversal_long":
		if containsStringValue(coin.V7ReasonCodes, "reviewable_floor_rescue") &&
			coin.V7AIPriority >= 45 &&
			coin.V7SetupScore >= 65 &&
			coin.V7TimingScore >= 35 &&
			coin.V7RiskScore < 45 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 50) &&
			hunterV7TakerBuyConfirmedAtLeast(coin, 0.52) {
			return true, "panic_reversal_reviewable_floor_live_confirm"
		}
		if coin.V7AIPriority >= 50 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 30 &&
			coin.V7RiskScore < 55 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 50) &&
			hunterV7TakerBuyConfirmedAtLeast(coin, 0.52) &&
			hunterV7PanicReversalHasHighWinReclaim(coin) {
			return true, "panic_reversal_reviewable_high_win_reclaim"
		}
		if coin.V7AIPriority >= 50 &&
			coin.V7SetupScore >= 38 &&
			coin.V7TimingScore >= 40 &&
			coin.V7RiskScore < 60 &&
			hunterV7PanicReversalCoreFlowOK(coin) {
			return true, "panic_reversal_reviewable_core_present"
		}
		if coin.V7AIPriority >= 45 &&
			coin.V7SetupScore >= 30 &&
			coin.V7TimingScore >= 45 &&
			coin.V7RiskScore < 35 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 50) &&
			containsStringValue(coin.V7ReasonCodes, "strong_reclaim") &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"taker_buy_strong", "taker_buy_aggressive"}) &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"selling_decelerating", "selling_exhaustion"}) {
			return true, "panic_reversal_reviewable_capitulation_floor"
		}
	case "funding_reversal":
		if strings.EqualFold(coin.Direction, "SHORT") &&
			coin.V7AIPriority >= 47 &&
			coin.V7TimingScore >= 55 &&
			coin.V7RiskScore < 60 &&
			hunterV7TakerBuyAtMost(coin, 0.50) {
			return true, "funding_short_reviewable_crowding_reversal"
		}
		if strings.EqualFold(coin.Direction, "LONG") &&
			coin.V7ExecutionQuality == "ready" &&
			coin.V7AIPriority >= 60 &&
			coin.V7TimingScore >= 60 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.52) {
			return true, "funding_long_reviewable_strong_only"
		}
	case "leader_momentum_long":
		if coin.V7ExecutionQuality == "ready" &&
			coin.V7AIPriority >= 70 &&
			coin.V7SetupScore >= 75 &&
			coin.V7TimingScore >= 65 &&
			coin.V7RiskScore < 40 &&
			hunterV7TakerBuyAtLeast(coin, 0.48) &&
			!containsStringValue(coin.V7ReasonCodes, "taker_weak_buy") {
			return true, "momentum_reviewable_strong_but_needs_flow_check"
		}
		if coin.V7ExecutionQuality == "ready" &&
			coin.V7AIPriority >= 75 &&
			coin.V7SetupScore >= 80 &&
			coin.V7TimingScore >= 62 &&
			coin.V7RiskScore < 25 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 50) &&
			hunterV7TakerBuyAtLeast(coin, 0.50) &&
			!containsStringValue(coin.V7ReasonCodes, "taker_weak_buy") {
			return true, "momentum_reviewable_ready_priority_floor"
		}
		if coin.V7AIPriority >= 72 &&
			coin.V7SetupScore >= 80 &&
			coin.V7TimingScore >= 62 &&
			coin.V7RiskScore < 25 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 80) &&
			hunterV7TakerBuyConfirmedAtLeast(coin, 0.52) &&
			hunterV7LeaderMomentumHasCleanPullback(coin) {
			return true, "momentum_reviewable_high_priority_pullback"
		}
		if coin.V7ExecutionQuality == "ready" &&
			coin.V7AIPriority >= 62 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 65 &&
			coin.V7RiskScore < 25 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 60) &&
			hunterV7TakerBuyConfirmedAtLeast(coin, 0.52) &&
			hunterV7LeaderMomentumHasCleanPullback(coin) &&
			hunterV7ConfirmationSummaryReviewPassed(coin) &&
			containsStringValue(coin.V7ReasonCodes, "strong_symbol_regime_override") &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"solid_4h_momentum", "strong_4h_momentum"}) &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"solid_24h_momentum", "strong_24h_momentum"}) &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"oi_healthy_growth", "oi_moderate_growth"}) {
			return true, "momentum_reviewable_confirmed_relative_strength"
		}
		if coin.V7ExecutionQuality == "ready" &&
			coin.V7AIPriority >= 62 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 65 &&
			coin.V7RiskScore < 25 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 80) &&
			hunterV7TakerBuyConfirmedAtLeast(coin, 0.50) &&
			hunterV7LeaderMomentumHasCleanPullback(coin) &&
			containsStringValue(coin.V7ReasonCodes, "strong_symbol_regime_override") &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"solid_4h_momentum", "strong_4h_momentum"}) &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"solid_24h_momentum", "strong_24h_momentum"}) &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"oi_healthy_growth", "oi_moderate_growth"}) {
			return true, "momentum_reviewable_relative_strength_floor"
		}
		if ok, reason := hunterV7LeaderMomentumFlexibleReviewableReason(coin); ok {
			return true, reason
		}
	case "pullback_reversal_long":
		if coin.V7AIPriority >= 48 &&
			coin.V7SetupScore >= 70 &&
			coin.V7TimingScore >= 55 &&
			coin.V7RiskScore < 45 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 50) &&
			hunterV7TakerBuyAtLeast(coin, 0.50) {
			return true, "pullback_reviewable_strong_structure"
		}
	case "trend_breakout_long", "accumulation_breakout_long":
		if coin.V7AIPriority >= 55 &&
			coin.V7SetupScore >= 58 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 50 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) {
			return true, "long_setup_reviewable_needs_realtime_confirm"
		}
		if coin.V7ExecutionQuality == "near_confirm" &&
			coin.V7AIPriority >= 45 &&
			coin.V7SetupScore >= 50 &&
			coin.V7TimingScore >= 45 &&
			coin.V7RiskScore < 35 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 50) &&
			containsStringValue(coin.V7ReasonCodes, "confirmed_breakout") &&
			containsStringValue(coin.V7ReasonCodes, "taker_aggressive_buy") {
			return true, "breakout_reviewable_confirmed_low_risk_floor"
		}
		if coin.V7ExecutionQuality == "near_confirm" &&
			coin.V7AIPriority >= 45 &&
			coin.V7SetupScore >= 38 &&
			coin.V7TimingScore >= 45 &&
			coin.V7RiskScore < 35 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 60) &&
			hunterV7TakerBuyAtLeast(coin, 0.52) &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"approaching_breakout", "breakout_attempt", "confirmed_breakout"}) &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"volume_adequate", "oi_increasing", "oi_stable_breakout"}) &&
			containsStringValue(coin.V7ReasonCodes, "clear_air_above") {
			return true, "breakout_reviewable_low_risk_pressure_floor"
		}
		if hunterV7TrendBreakoutStrongFlowReviewable(coin) {
			return true, "breakout_watch_strong_flow_reviewable"
		}
	case "mms_bottom_wake_long":
		if coin.V7AIPriority >= 45 &&
			coin.V7SetupScore >= 48 &&
			coin.V7RiskScore < 45 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 50) &&
			hunterV7TakerBuyAtLeast(coin, 0.48) {
			return true, "mms_bottom_wake_reviewable_breakout_required"
		}
	case "mms_trend_ride_long", "mms_squeeze_engine_long":
		if coin.V7AIPriority >= 50 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 55 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) {
			return true, "mms_long_reviewable_confirmed"
		}
	case "alt_ladder_momentum_long":
		if coin.V7AIPriority >= 50 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 52 &&
			coin.V7RiskScore < 55 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) {
			return true, "alt_ladder_long_reviewable_confirmed"
		}
	case "pre_breakout_watch", "pre_squeeze_watch", "pre_distribution_watch", "accumulation_watch":
		if hunterV7WatchUpgradedReviewable(coin) {
			return true, "watch_state_upgraded_reviewable"
		}
	case "displacement_momentum_long":
		if coin.V7AIPriority >= 48 &&
			coin.V7SetupScore >= 50 &&
			coin.V7TimingScore >= 40 &&
			coin.V7RiskScore < 55 &&
			(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 50) &&
			hunterV7TakerBuyAtLeast(coin, 0.50) {
			return true, "displacement_reviewable_needs_confirm"
		}
	case "range_expansion_event":
		if hunterV7ConfirmedRangeExpansionContinuation(coin, false) &&
			coin.V7AIPriority >= 60 &&
			coin.V7SetupScore >= 60 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 45 {
			return true, "range_expansion_reviewable_confirmed_continuation"
		}
	case "alt_ladder_breakdown_short":
		if coin.V7AIPriority >= 52 &&
			coin.V7TimingScore >= 58 &&
			coin.V7RiskScore <= 45 &&
			hunterV7TakerBuyAtMost(coin, 0.48) {
			return true, "alt_ladder_short_reviewable_confirmed"
		}
	case "distribution_short", "long_squeeze_short", "breakdown_momentum_short", "range_reversion":
		if coin.V7AIPriority >= 50 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 55 {
			return true, "short_or_reversion_reviewable"
		}
	}
	return false, ""
}

func hunterV7OpenRateCandidateFloorLegacy(coin CandidateCoin) bool {
	switch coin.V7SetupType {
	case "trend_breakout_long", "accumulation_breakout_long":
		return coin.V7AIPriority >= 55 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 45 &&
			coin.V7RiskScore < 45 &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"approaching_breakout", "breakout_attempt", "confirmed_breakout"}) &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"volume_expansion", "volume_adequate", "oi_increasing", "oi_stable_breakout", "clear_air_above"})
	case "whale_flow_reversal":
		return coin.V7AIPriority >= 48 &&
			coin.V7SetupScore >= 48 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 45 &&
			containsStringValue(coin.V7ReasonCodes, "whale_flow_detected") &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"oi_1h_confirming_accumulation", "stealth_accumulation_breakout", "funding_not_crowded"})
	case "displacement_momentum_long":
		return coin.V7AIPriority >= 60 &&
			coin.V7SetupScore >= 70 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 45 &&
			!containsStringValue(coin.V7ReasonCodes, "chase_high_protection") &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"oi_confirms_new_demand", "taker_buy_aggressive", "taker_buy_aligned"})
	case "pullback_reversal_long":
		return coin.V7AIPriority >= 48 &&
			coin.V7SetupScore >= 60 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 45 &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"healthy_pullback", "near_4h_support", "strong_reclaim"})
	case "range_expansion_event":
		return coin.V7AIPriority >= 58 &&
			coin.V7SetupScore >= 58 &&
			coin.V7TimingScore >= 50 &&
			coin.V7RiskScore < 40 &&
			containsAnyStringValue(coin.V7ReasonCodes, []string{"range_expansion_continuation", "range_expansion_retest", "retest_confirmed"}) &&
			!containsAnyStringValue(coin.V7ReasonCodes, []string{"range_expansion_late_chase", "range_expansion_exhaustion"})
	default:
		return coin.V7AIPriority >= 60 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 55 &&
			coin.V7RiskScore < 40
	}
}

// hunterV7ShadowSetups is every setup the legacy switches know about, plus a
// synthetic unknown to pin the default branches.
var hunterV7ShadowSetups = []string{
	"panic_reversal_long", "funding_reversal", "leader_momentum_long",
	"trend_breakout_long", "accumulation_breakout_long", "pullback_reversal_long",
	"short_squeeze_long", "mms_trend_ride_long", "mms_squeeze_engine_long",
	"mms_bottom_wake_long", "alt_ladder_momentum_long", "displacement_momentum_long",
	"range_expansion_event", "alt_ladder_breakdown_short", "distribution_short",
	"long_squeeze_short", "breakdown_momentum_short", "range_reversion",
	"whale_flow_reversal", "pre_breakout_watch", "pre_squeeze_watch",
	"pre_distribution_watch", "accumulation_watch", "volatility_squeeze_breakout",
	"intraday_scalp_long", "some_future_setup",
}

// hunterV7ShadowReasonBundles feeds the reason-code requirements: empty, the
// full union of every code referenced by the legacy switches, and the union
// minus the forbid/guard codes.
var hunterV7ShadowReasonBundles = [][]string{
	nil,
	hunterV7ShadowAllReasonCodes,
	hunterV7ShadowPositiveReasonCodes,
}

var hunterV7ShadowAllReasonCodes = []string{
	"taker_weak_buy", "alt_ladder_taker_sell", "alt_ladder_new_shorts",
	"trigger_memory_confirmed", "reviewable_floor_rescue", "strong_reclaim",
	"taker_buy_strong", "selling_decelerating", "strong_symbol_regime_override",
	"solid_4h_momentum", "solid_24h_momentum", "oi_healthy_growth",
	"confirmed_breakout", "taker_aggressive_buy", "approaching_breakout",
	"volume_adequate", "clear_air_above", "whale_flow_detected",
	"oi_1h_confirming_accumulation", "chase_high_protection", "healthy_pullback",
	"range_expansion_continuation", "range_expansion_late_chase",
	"low_timing_watch_only", "taker_buy_aligned",
}

var hunterV7ShadowPositiveReasonCodes = []string{
	"alt_ladder_taker_sell", "alt_ladder_new_shorts", "trigger_memory_confirmed",
	"reviewable_floor_rescue", "strong_reclaim", "taker_buy_strong",
	"selling_decelerating", "strong_symbol_regime_override", "solid_4h_momentum",
	"solid_24h_momentum", "oi_healthy_growth", "confirmed_breakout",
	"taker_aggressive_buy", "approaching_breakout", "volume_adequate",
	"clear_air_above", "whale_flow_detected", "oi_1h_confirming_accumulation",
	"healthy_pullback", "range_expansion_continuation", "taker_buy_aligned",
}

var hunterV7ShadowCorpusCache []CandidateCoin

func hunterV7ShadowCorpus(t *testing.T) []CandidateCoin {
	t.Helper()
	if hunterV7ShadowCorpusCache != nil {
		return hunterV7ShadowCorpusCache
	}
	coins := make([]CandidateCoin, 0, 1200000)

	// Real signals from the frozen live cycle.
	fixturePath := filepath.Join("..", "provider", "local", "testdata", "golden", "universe-20260726.json")
	fixture, err := local.LoadV7GoldenFixture(fixturePath)
	if err != nil {
		t.Fatalf("load golden fixture: %v", err)
	}
	result := local.NewV7Router().RouteDetailed(fixture.Universe, fixture.Regime, fixture.Config)
	engine := newHunterV7ShadowEngine()
	coins = append(coins, engine.hunterV7SignalsToCandidateCoins(result.OutputSignals, "BOTH")...)

	// Deterministic synthetic grid straddling every threshold in the legacy
	// switches.
	type variant struct{ quality, entrySignal, status string }
	variants := []variant{
		{"", "", "candidate"},
		{"ready", "", "candidate"},
		{"near_confirm", "", "candidate"},
		{"", "entry_open_now", "candidate"},
		{"near_confirm", "entry_trigger_near", "candidate"},
		{"ready", "", "wait_confirm"},
	}
	// Cross tuples give interaction coverage; the axis sweeps below guarantee
	// every threshold that appears in the legacy switches is straddled (value
	// exactly at the floor and one just below), so a single mis-transcribed
	// number in a table row cannot survive the shadow diff. The sensitivity
	// self-check for this corpus lives in TestHunterV7TierShadowSensitivity.
	crossTuples := [][5]float64{
		// {AIPriority, SetupScore, TimingScore, RiskScore, LiquidityScore}
		{44, 35, 30, 20, 0},
		{45, 45, 45, 44, 50},
		{47, 50, 55, 59, 60},
		{48, 60, 40, 54, 45},
		{50, 55, 30, 54, 50},
		{52, 58, 52, 45, 65},
		{55, 58, 50, 49, 70},
		{58, 65, 55, 34, 75},
		{60, 70, 60, 54, 0},
		{62, 75, 62, 24, 85},
		{65, 80, 65, 44, 60},
		{70, 75, 66, 39, 50},
		{72, 80, 62, 24, 80},
		{75, 82, 64, 20, 90},
	}
	axisValues := [5][]float64{
		{43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 54, 55, 57, 58, 59, 60, 61, 62, 64, 65, 69, 70, 71, 72, 74, 75},
		{29, 30, 34, 35, 37, 38, 44, 45, 47, 48, 49, 50, 54, 55, 57, 58, 59, 60, 64, 65, 69, 70, 74, 75, 77, 78, 79, 80, 82},
		{29, 30, 34, 35, 39, 40, 44, 45, 49, 50, 51, 52, 54, 55, 57, 58, 59, 60, 61, 62, 64, 65, 66},
		{20, 24, 25, 34, 35, 39, 40, 44, 45, 46, 49, 50, 54, 55, 56, 59, 60, 64, 65},
		{0, 45, 49, 50, 55, 59, 60, 65, 69, 70, 75, 79, 80, 85, 90},
	}
	sweepBases := [][5]float64{
		{75, 82, 66, 20, 90}, // passes every floor: sweeping one axis isolates it
		{50, 55, 50, 54, 50}, // mid tuple: catches floors above the mid values
	}
	sweepTuples := make([][5]float64, 0, 256)
	for _, base := range sweepBases {
		for axis, values := range axisValues {
			for _, value := range values {
				tuple := base
				tuple[axis] = value
				sweepTuples = append(sweepTuples, tuple)
			}
		}
	}
	// Full taker resolution rides on the cross tuples; the score sweeps only
	// need one passing taker per direction-gate family to isolate their axis.
	crossTakers := []float64{0, 0.44, 0.45, 0.455, 0.46, 0.47, 0.48, 0.50, 0.51, 0.52, 0.53}
	sweepTakers := []float64{0.44, 0.53}
	directions := []string{"LONG", "SHORT"}

	appendGrid := func(tuples [][5]float64, takers []float64) {
		for _, setup := range hunterV7ShadowSetups {
			for _, dir := range directions {
				for _, v := range variants {
					for _, scores := range tuples {
						for _, taker := range takers {
							for bundleIdx, bundle := range hunterV7ShadowReasonBundles {
								coin := CandidateCoin{
									Symbol:             fmt.Sprintf("S%s%d", dir[:1], bundleIdx),
									Direction:          dir,
									V7SetupType:        setup,
									V7Status:           v.status,
									V7ExecutionQuality: v.quality,
									V7EntrySignal:      v.entrySignal,
									V7AIPriority:       scores[0],
									V7SetupScore:       scores[1],
									V7TimingScore:      scores[2],
									V7RiskScore:        scores[3],
									V7LiquidityScore:   scores[4],
									V7ReasonCodes:      bundle,
								}
								if taker > 0 {
									coin.V7DerivativesCtx = &local.V7DerivativesContext{TakerBuy15m: taker}
								}
								coins = append(coins, coin)
							}
						}
					}
				}
			}
		}
	}
	appendGrid(crossTuples, crossTakers)
	appendGrid(sweepTuples, sweepTakers)
	hunterV7ShadowCorpusCache = coins
	return coins
}

func newHunterV7ShadowEngine() *StrategyEngine {
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.SourceType = "hunter_v7"
	return NewStrategyEngine(&cfg)
}

// TestHunterV7TierRuleShadowDiff replays the whole corpus through the
// production gates (table dispatch for registered setups, legacy switch
// otherwise) and the frozen legacy copies. diff must be empty — this is the
// hard gate for every U3.3 setup migration.
func TestHunterV7TierRuleShadowDiff(t *testing.T) {
	coins := hunterV7ShadowCorpus(t)
	diffs := 0
	for i := range coins {
		coin := coins[i]

		gotOK, gotReason := hunterV7ReadyExecutableReason(coin)
		wantOK, wantReason := hunterV7ReadyExecutableReasonLegacy(coin)
		if gotOK != wantOK || gotReason != wantReason {
			diffs++
			if diffs <= 20 {
				t.Errorf("ready diff: setup=%s dir=%s q=%s coin#%d got=(%v,%q) want=(%v,%q)",
					coin.V7SetupType, coin.Direction, coin.V7ExecutionQuality, i, gotOK, gotReason, wantOK, wantReason)
			}
		}

		gotOK, gotReason = hunterV7NearConfirmExecutableReason(coin)
		wantOK, wantReason = hunterV7NearConfirmExecutableReasonLegacy(coin)
		if gotOK != wantOK || gotReason != wantReason {
			diffs++
			if diffs <= 20 {
				t.Errorf("near_confirm diff: setup=%s dir=%s coin#%d got=(%v,%q) want=(%v,%q)",
					coin.V7SetupType, coin.Direction, i, gotOK, gotReason, wantOK, wantReason)
			}
		}

		gotOK, gotReason = hunterV7ReviewableCandidateReason(coin)
		wantOK, wantReason = hunterV7ReviewableCandidateReasonLegacy(coin)
		if gotOK != wantOK || gotReason != wantReason {
			diffs++
			if diffs <= 20 {
				t.Errorf("reviewable diff: setup=%s dir=%s q=%s coin#%d got=(%v,%q) want=(%v,%q)",
					coin.V7SetupType, coin.Direction, coin.V7ExecutionQuality, i, gotOK, gotReason, wantOK, wantReason)
			}
		}

		gotFloor := hunterV7OpenRateCandidateFloor(coin)
		wantFloor := hunterV7OpenRateCandidateFloorLegacy(coin)
		if gotFloor != wantFloor {
			diffs++
			if diffs <= 20 {
				t.Errorf("open_rate_floor diff: setup=%s coin#%d got=%v want=%v",
					coin.V7SetupType, i, gotFloor, wantFloor)
			}
		}
	}
	if diffs > 0 {
		t.Fatalf("shadow diff non-empty: %d divergences across %d coins", diffs, len(coins))
	}
	t.Logf("shadow corpus: %d coins, 0 divergences (registered setups: %d)", len(coins), len(hunterV7SetupTierSpecs))
}

// TestHunterV7TierShadowSensitivity proves the shadow corpus has teeth: a
// deliberately mis-transcribed table row (floor off by one) must produce
// divergences. If this fails after corpus changes, the grid lost its
// threshold-straddling coverage and the shadow diff is no longer trustworthy.
func TestHunterV7TierShadowSensitivity(t *testing.T) {
	// some_future_setup only exists in the shadow corpus, so it can never be
	// migrated for real — the probe targets the legacy default ready branch
	// (floor 60) with an off-by-one floor of 61.
	const setup = "some_future_setup"
	if _, exists := hunterV7SetupTierSpecs[setup]; exists {
		t.Fatalf("sensitivity probe needs an unregistered setup; %s is already migrated — switch the probe to another setup", setup)
	}
	hunterV7SetupTierSpecs[setup] = hunterV7SetupTierSpec{
		Ready: []hunterV7TierRule{
			{MinAIPriority: 61, MinTimingScore: 60, RiskBelow: 55, Reason: "execution_quality_ready"},
		},
	}
	defer delete(hunterV7SetupTierSpecs, setup)

	diffs := 0
	for _, coin := range hunterV7ShadowCorpus(t) {
		if coin.V7SetupType != setup {
			continue
		}
		gotOK, gotReason := hunterV7ReadyExecutableReason(coin)
		wantOK, wantReason := hunterV7ReadyExecutableReasonLegacy(coin)
		if gotOK != wantOK || gotReason != wantReason {
			diffs++
		}
	}
	if diffs == 0 {
		t.Fatalf("off-by-one table row produced zero divergences — shadow corpus lost its sensitivity")
	}
	t.Logf("sensitivity ok: off-by-one row caught with %d divergences", diffs)
}

// TestHunterV7TierSpecShape validates registered specs: every rule must emit a
// reason (OpenRateFloor rules excepted — the caller only consumes the match).
func TestHunterV7TierSpecShape(t *testing.T) {
	for setup, spec := range hunterV7SetupTierSpecs {
		for listName, rules := range map[string][]hunterV7TierRule{
			"Ready": spec.Ready, "NearConfirm": spec.NearConfirm, "Reviewable": spec.Reviewable,
		} {
			for i, rule := range rules {
				if rule.Reason == "" {
					t.Errorf("%s %s rule %d has no reason", setup, listName, i)
				}
			}
		}
	}
}
