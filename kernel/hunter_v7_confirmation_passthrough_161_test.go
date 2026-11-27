package kernel

// #161 acceptance tests: the single-missing-confirmation pass-through.
//
// The funnel's job is recall, not verdict: a candidate whose ONLY
// remaining blocker is exactly one missing required confirmation
// advances from WATCH to REVIEWABLE, flagged
// "confirmation_passthrough_<code>", and the LLM adjudicates with the
// full data pack. Two or more missing confirmations, any additional
// hard block, and shadow-only rows keep their pre-#161 outcomes.

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// passthrough161Coin is a displacement candidate that the tier rules
// tier REVIEWABLE (displacement_reviewable_needs_confirm) as soon as its
// required confirmations are present: the missing confirmation is the
// only thing the rules hold it on. The live-confirmable lane does not
// fire for it (risk score 50 fails that lane's open-rate floor,
// RiskBelow 45), so any promotion is the #161 pass-through's.
func passthrough161Coin(mutators ...func(*CandidateCoin)) CandidateCoin {
	base := []func(*CandidateCoin){
		withSymbol("PASS161USDT"),
		withQuality("near_confirm"),
		withScores(63, 82, 55, 50, 100),
		withRiskLevel("LOW"),
		withReasons("massive_vol_displacement", "oi_confirms_new_demand"),
		withConfirms("taker_flow_confirms_long"),
		withZone(0.0029, 0.0031),
		withPriceCtx(&local.V7PriceContext{Last: 0.00305}),
		withTaker(0.50),
		withConfirmSummary(&local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{Code: "taker_flow_confirms_long", Passed: false, Severity: local.V7ConfirmReviewWait},
			},
			EntryZonePosition: 80,
			RR:                4,
		}),
	}
	return v7Candidate("displacement_momentum_long", append(base, mutators...)...)
}

func classify161(coin CandidateCoin) (string, string) {
	return classifyHunterV7CandidateTier(withNeutralRRGeometry(coin))
}

// (a) Exactly one missing confirmation: advances to REVIEWABLE, and the
// flag travels in the LLM payload (tier_reason in both the full and the
// compact signal JSON; the confirmation summary names the same code).
func TestHunterV7PassThrough161SingleMissingAdvancesFlagged(t *testing.T) {
	coin := passthrough161Coin()
	tier, reason := classify161(coin)
	if tier != "REVIEWABLE" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE", tier, reason)
	}
	if reason != "confirmation_passthrough_taker_flow_confirms_long" {
		t.Fatalf("reason = %q, want confirmation_passthrough_taker_flow_confirms_long", reason)
	}

	coin.V7ExecutionTier, coin.V7TierReason = tier, reason
	payload := buildHunterV7PromptPayload(coin)
	full := marshalHunterV7JSON(payload)
	if !strings.Contains(full, `"tier_reason":"confirmation_passthrough_taker_flow_confirms_long"`) {
		t.Fatalf("full payload does not carry the pass-through flag: %s", full)
	}
	compact := marshalHunterV7JSON(payload.compactView())
	if !strings.Contains(compact, `"tier_reason":"confirmation_passthrough_taker_flow_confirms_long"`) {
		t.Fatalf("compact payload does not carry the pass-through flag: %s", compact)
	}
	if payload.ConfirmationSummary == nil || len(payload.ConfirmationSummary.MissingReview) != 1 ||
		payload.ConfirmationSummary.MissingReview[0].Code != "taker_flow_confirms_long" {
		t.Fatalf("payload confirmation summary does not name the missing code: %+v", payload.ConfirmationSummary)
	}
}

// (b) Two or more missing confirmations: stays in WATCH, unchanged.
func TestHunterV7PassThrough161TwoMissingStaysWatch(t *testing.T) {
	coin := passthrough161Coin(
		withConfirms("directional_15m_close_long", "taker_flow_confirms_long"),
		withConfirmSummary(&local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{Code: "directional_15m_close_long", Passed: false, Severity: local.V7ConfirmReviewWait},
				{Code: "taker_flow_confirms_long", Passed: false, Severity: local.V7ConfirmReviewWait},
			},
			EntryZonePosition: 80,
			RR:                4,
		}),
	)
	tier, reason := classify161(coin)
	if tier != "WATCH" {
		t.Fatalf("tier = %q (%s), want WATCH", tier, reason)
	}
	if reason != "confirmation_missing_directional_15m_close_long" {
		t.Fatalf("reason = %q, want confirmation_missing_directional_15m_close_long", reason)
	}
}

// (b') No confirmation summary at all: the wait reason names no code,
// so nothing can pass through; the pre-#161 outcome is unchanged.
func TestHunterV7PassThrough161MissingSummaryStaysWatch(t *testing.T) {
	coin := passthrough161Coin(withConfirmSummary(nil))
	tier, reason := classify161(coin)
	if tier != "WATCH" || reason != "confirmation_missing_summary" {
		t.Fatalf("tier/reason = %s/%s, want WATCH/confirmation_missing_summary", tier, reason)
	}
}

// (c) Shadow-only rows: byte-identical outcome. The pass-through never
// fires for a shadow-only candidate, so the row keeps exactly the
// pre-#161 verdict (WATCH with the confirmation_missing reason) instead
// of being promoted and then capped under a different reason.
func TestHunterV7PassThrough161ShadowOnlyByteIdentical(t *testing.T) {
	coin := passthrough161Coin()
	coin.V7ClassAdmission = store.V7AdmissionShadowOnly

	if ok, reason := hunterV7SingleMissingConfirmationPassThrough(coin, HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true)); ok {
		t.Fatalf("pass-through fired for a shadow-only candidate: %s", reason)
	}
	tier, reason := classify161(coin)
	if tier != "WATCH" || reason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("tier/reason = %s/%s, want the pre-#161 WATCH/confirmation_missing_taker_flow_confirms_long", tier, reason)
	}
}

// (d) One missing confirmation PLUS another hard block: no advance.
// chase_risk quality is a hard wait later in the rule order
// (chase_risk_wait_reentry); the pass-through probe sees it and the
// candidate keeps its original WATCH verdict and reason.
func TestHunterV7PassThrough161MissingPlusHardBlockStaysWatch(t *testing.T) {
	coin := passthrough161Coin(withQuality("chase_risk"))
	tier, reason := classify161(coin)
	if tier != "WATCH" {
		t.Fatalf("tier = %q (%s), want WATCH", tier, reason)
	}
	if reason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("reason = %q, want confirmation_missing_taker_flow_confirms_long", reason)
	}
}

// (e) One missing confirmation PLUS an execution-contract hard block:
// no advance. chase_high_protection is a wait-only reason code, so it
// lands in execution_contract.hard_blocks — but hard_blocks is
// prompt-only metadata (hybrid_v2 strips it; no post-LLM validator or
// execution guard reads it), so the pass-through must not promote on
// the assumption that the LLM will enforce it. The live-confirmable
// lane is untouched: it still refuses the row itself.
func TestHunterV7PassThrough161ChaseProtectedStaysWatch(t *testing.T) {
	coin := passthrough161Coin(
		withReasons("massive_vol_displacement", "oi_confirms_new_demand", "chase_high_protection"),
	)
	if ok, reason := hunterV7LiveConfirmableReviewableReason(coin, "confirmation_missing_taker_flow_confirms_long"); ok {
		t.Fatalf("live-confirmable lane promoted a chase-protected candidate: %s", reason)
	}
	tier, reason := classify161(coin)
	if tier != "WATCH" || reason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("tier/reason = %s/%s, want the pre-#161 WATCH/confirmation_missing_taker_flow_confirms_long", tier, reason)
	}
	// The chase code does reach the payload as contract metadata —
	// which is exactly why promotion must not rely on it: nothing
	// downstream enforces hard_blocks.
	coin.V7ExecutionTier, coin.V7TierReason = tier, reason
	contract := buildHunterV7PromptPayload(coin).ExecutionContract
	found := false
	for _, block := range contract.HardBlocks {
		if block == "chase_high_protection" {
			found = true
		}
	}
	if !found {
		t.Fatalf("execution contract hard_blocks lost chase_high_protection: %v", contract.HardBlocks)
	}
}

// (e') Same exclusion for low_timing_watch_only, the other wait-only
// reason code the executable path's direct-open guard names: one
// missing confirmation, but the row stays in WATCH unchanged.
func TestHunterV7PassThrough161LowTimingWatchOnlyStaysWatch(t *testing.T) {
	coin := passthrough161Coin(
		withReasons("massive_vol_displacement", "oi_confirms_new_demand", "low_timing_watch_only"),
	)
	tier, reason := classify161(coin)
	if tier != "WATCH" || reason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("tier/reason = %s/%s, want the pre-#161 WATCH/confirmation_missing_taker_flow_confirms_long", tier, reason)
	}
	if blocks := hunterV7HardBlockEntries(coin); len(blocks) != 1 || blocks[0] != "low_timing_watch_only" {
		t.Fatalf("hard block entries = %v, want [low_timing_watch_only]", blocks)
	}
}

// (f) The single missing code must be satisfiable at decision-time
// refresh. oi_delta_1h_positive_or_quote_volume_expands is registered
// RefreshSatisfiable=false: the trader's refresh recheck can never
// clear it, so a REVIEWABLE promotion would be an unopenable trap.
// The row stays in WATCH with its original reason (the CHILLGUYUSDT
// shape from the golden universe).
func TestHunterV7PassThrough161NonRefreshSatisfiableStaysWatch(t *testing.T) {
	coin := passthrough161Coin(
		withConfirms("oi_delta_1h_positive_or_quote_volume_expands"),
		withConfirmSummary(&local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{Code: "oi_delta_1h_positive_or_quote_volume_expands", Passed: false, Severity: local.V7ConfirmReviewWait},
			},
			EntryZonePosition: 80,
			RR:                4,
		}),
	)
	tier, reason := classify161(coin)
	if tier != "WATCH" || reason != "confirmation_missing_oi_delta_1h_positive_or_quote_volume_expands" {
		t.Fatalf("tier/reason = %s/%s, want WATCH/confirmation_missing_oi_delta_1h_positive_or_quote_volume_expands", tier, reason)
	}
}

// (g) Probe fidelity, summary side: whale_flow_reversal defines no
// Reviewable tier rules, so a near_confirm row's only REVIEWABLE path
// is near_confirm_reviewable_micro_confirmed, which reads the
// confirmation summary's PassedHard/PassedReview directly. The probe
// must present the single missing confirmation as present in the
// summary too, or this shape silently stays in WATCH.
func TestHunterV7PassThrough161NearConfirmWhalePromotes(t *testing.T) {
	coin := v7Candidate("whale_flow_reversal",
		withSymbol("WHALE161USDT"),
		withQuality("near_confirm"),
		withScores(85, 90, 82, 0, 100),
		withRiskLevel("LOW"),
		withZone(0.03736, 0.03898),
		withInvalidation(0.03741),
		withTargets(local.V7Target{Price: 0.04419}),
		withPriceCtx(&local.V7PriceContext{Last: 0.038102}),
		withTaker(0.55),
		withConfirms("directional_15m_close_long", "taker_flow_confirms_long"),
		withConfirmSummary(&local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{Code: "taker_flow_confirms_long", Passed: false, Severity: local.V7ConfirmReviewWait},
			},
			RR: 2.0,
		}),
	)
	tier, reason := classify161(coin)
	if tier != "REVIEWABLE" || reason != "confirmation_passthrough_taker_flow_confirms_long" {
		t.Fatalf("tier/reason = %s/%s, want REVIEWABLE/confirmation_passthrough_taker_flow_confirms_long", tier, reason)
	}
}

// (h) Probe fidelity, readiness side: range_expansion_event's Ready
// and Reviewable rules both gate on
// hunterV7ConfirmedRangeExpansionContinuation, which reads the
// confirmation summary AND V7Readiness.Tier. The stamped readiness is
// WATCH solely because of the single missing confirmation
// (BlockedGate/Reason say so), so the probe must re-derive it with
// the confirmation present, or this shape stays in WATCH too.
func TestHunterV7PassThrough161RangeExpansionPromotes(t *testing.T) {
	coin := v7Candidate("range_expansion_event",
		withSymbol("RANGE161USDT"),
		withQuality("ready"),
		withScores(70, 75, 65, 20, 100),
		withRiskLevel("LOW"),
		withZone(100, 110),
		withPriceCtx(&local.V7PriceContext{Last: 105}),
		withTaker(0.58),
		withConfirms("taker_flow_confirms_long"),
		withConfirmSummary(&local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{Code: "taker_flow_confirms_long", Passed: false, Severity: local.V7ConfirmReviewWait},
			},
			RR: 3,
		}),
		withReadiness(&local.V7ExecutionReadiness{
			Tier:         local.V7ReadinessWatch,
			Reason:       "confirmation_missing_taker_flow_confirms_long",
			BlockedGate:  "confirmation_missing",
			ReadyScore:   80,
			WindowHealth: 80,
			EntryZonePos: 50,
			NextConfirm:  []string{"taker_flow_confirms_long"},
		}),
	)
	tier, reason := classify161(coin)
	if tier != "REVIEWABLE" || reason != "confirmation_passthrough_taker_flow_confirms_long" {
		t.Fatalf("tier/reason = %s/%s, want REVIEWABLE/confirmation_passthrough_taker_flow_confirms_long", tier, reason)
	}
}

// (i) Outer-cap reason preservation: a single-missing candidate
// carrying the correlation-floor context tag is context only. The
// pass-through short-circuits, so the row keeps its pre-#161 WATCH
// reason instead of being promoted and then capped under
// correlation_floor_context.
func TestHunterV7PassThrough161CorrelationFloorReasonPreserved(t *testing.T) {
	coin := passthrough161Coin(withRiskTags(local.V7CorrelationFloorContextTag))
	tier, reason := classify161(coin)
	if tier != "WATCH" || reason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("tier/reason = %s/%s, want the pre-#161 WATCH/confirmation_missing_taker_flow_confirms_long", tier, reason)
	}
}

// (i') Same preservation for the backend-capped RR cap: with no
// computable backend RR (no invalidation/targets), the pass-through
// short-circuits and the row keeps its pre-#161 reason instead of
// being promoted and then capped under rr_unknown.
func TestHunterV7PassThrough161BackendRRReasonPreserved(t *testing.T) {
	coin := passthrough161Coin()
	tier, reason := classifyHunterV7CandidateTier(coin)
	if tier != "WATCH" || reason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("tier/reason = %s/%s, want the pre-#161 WATCH/confirmation_missing_taker_flow_confirms_long", tier, reason)
	}
}
