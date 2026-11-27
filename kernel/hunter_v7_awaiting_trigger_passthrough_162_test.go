package kernel

// #162 acceptance tests: the t2_awaiting_trigger pass-through.
//
// The funnel's job is recall, not verdict: a Task-2 candidate whose
// ONLY remaining blocker is its pending trigger advances from WATCH to
// REVIEWABLE, flagged "t2_awaiting_trigger_passthrough_<class>_<state>",
// and the LLM adjudicates the setup. Execution stays gated on the
// trigger cross: the row is stamped as a conditional entry, so an
// approved open registers as a U9 pending entry (and fails closed
// while conditional entry is disabled) instead of opening at market.
// Awaiting-trigger plus any other block, IMPULSE rows (score-gated, no
// price trigger), and shadow-only rows keep their pre-#162 outcomes.

import (
	"strings"
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// passthrough162Coin is a T2 candidate (price 100, stop 99, target 103:
// backend RR 3.0, above the T2 floor) carrying its recorded trigger
// price, as the production assembly stamps it (TriggerPrice = entry).
func passthrough162Coin(setup, state string, mutators ...func(*CandidateCoin)) CandidateCoin {
	c := t2Coin(setup, state)
	c.V7TriggerPrice = 100
	for _, m := range mutators {
		m(&c)
	}
	return c
}

func classify162(coin CandidateCoin) (string, string) {
	return classifyHunterV7CandidateTierWithGeometry(coin, defaultGeometry())
}

// (a) Sole-reason awaiting trigger: advances to REVIEWABLE, flagged
// with the class and the pending lifecycle state.
func TestHunterV7PassThrough162SoleReasonAdvancesFlagged(t *testing.T) {
	cases := []struct {
		name         string
		coin         CandidateCoin
		tier, reason string
	}{
		{"trigger watch", passthrough162Coin("ignition_early_long", "WATCH"),
			"REVIEWABLE", "t2_awaiting_trigger_passthrough_trigger_watch"},
		{"trigger armed", passthrough162Coin("bb_oi_accumulation_long", "ARMED"),
			"REVIEWABLE", "t2_awaiting_trigger_passthrough_trigger_armed"},
		{"reversal armed", passthrough162Coin("trend_pullback_short", "ARMED"),
			"REVIEWABLE", "t2_awaiting_trigger_passthrough_reversal_armed"},
		{"reversal watch", passthrough162Coin("trend_pullback_short", "WATCH"),
			"REVIEWABLE", "t2_awaiting_trigger_passthrough_reversal_watch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, reason := classify162(tc.coin)
			if tier != tc.tier || reason != tc.reason {
				t.Fatalf("%s/%s, want %s/%s", tier, reason, tc.tier, tc.reason)
			}
		})
	}
}

// (a) Through the production assembly: the flag travels in the LLM
// payload (tier_reason in both the full and the compact signal JSON),
// the execution contract names the pending trigger (entry permission
// conditional_on_trigger + the trigger price), the candidate is
// stamped as a conditional entry, and decision hydration carries that
// stamp — so an approved open takes the trader's conditional branch,
// never the immediate market path.
func TestHunterV7PassThrough162AssemblyFlagAndConditionalStamp(t *testing.T) {
	sig := t2TagSignal("ignition_early_long", local.V7DirLong, "WATCH")
	sig.Symbol = "PASS162USDT"
	sig.SignalID = "PASS162USDT"
	cands := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "BOTH", condGeometry(false))
	if len(cands) != 1 {
		t.Fatalf("assembly returned %d candidates", len(cands))
	}
	coin := cands[0]
	if coin.V7ExecutionTier != "REVIEWABLE" || coin.V7TierReason != "t2_awaiting_trigger_passthrough_trigger_watch" {
		t.Fatalf("assembled %s/%s", coin.V7ExecutionTier, coin.V7TierReason)
	}
	if !coin.V7ConditionalEntry {
		t.Fatal("pass-through candidate must be stamped as a conditional entry")
	}

	payload := buildHunterV7PromptPayload(coin)
	full := marshalHunterV7JSON(payload)
	if !strings.Contains(full, `"tier_reason":"t2_awaiting_trigger_passthrough_trigger_watch"`) {
		t.Fatalf("full payload does not carry the pass-through flag: %s", full)
	}
	compact := marshalHunterV7JSON(payload.compactView())
	if !strings.Contains(compact, `"tier_reason":"t2_awaiting_trigger_passthrough_trigger_watch"`) {
		t.Fatalf("compact payload does not carry the pass-through flag: %s", compact)
	}
	contract := buildHunterV7ExecutionContract(coin)
	if contract.EntryPermission != "conditional_on_trigger" || contract.TriggerPrice != 100 {
		t.Fatalf("contract = %+v, want conditional_on_trigger at trigger 100", contract)
	}

	ds := []Decision{{Symbol: coin.Symbol, Action: "open_long", SelectedHunterV7SignalID: coin.V7SignalID}}
	hydrateHunterV7DecisionExecutionPlans(ds, cands)
	if !ds[0].HunterV7ConditionalEntry || ds[0].HunterV7TriggerPrice != 100 {
		t.Fatalf("hydrated decision = %+v, want conditional entry at trigger 100", ds[0])
	}
}

// (a) When conditional entry IS enabled for the class, the pre-existing
// U9 branch still claims the row first: the verdict is the conditional
// reason, not the pass-through flag.
func TestHunterV7PassThrough162ConditionalBranchKeepsPrecedence(t *testing.T) {
	tier, reason := classifyHunterV7CandidateTierWithGeometry(passthrough162Coin("ignition_early_long", "WATCH"), condGeometry(true))
	if tier != "REVIEWABLE" || reason != "t2_trigger_conditional_watch" {
		t.Fatalf("%s/%s, want REVIEWABLE/t2_trigger_conditional_watch", tier, reason)
	}
}

// (b) Awaiting-trigger PLUS another block stays in WATCH, unchanged.
func TestHunterV7PassThrough162PlusAnotherBlockStaysWatch(t *testing.T) {
	noTargets := passthrough162Coin("ignition_early_long", "WATCH", func(c *CandidateCoin) {
		c.V7Targets = nil
	})
	cases := []struct {
		name         string
		coin         CandidateCoin
		tier, reason string
	}{
		// The correlation-floor outer cap would demote a promotion back
		// to WATCH under its own reason; the pass-through refuses, so
		// the row keeps t2_awaiting_trigger.
		{"correlation floor tag", passthrough162Coin("ignition_early_long", "WATCH", func(c *CandidateCoin) {
			c.V7RiskTags = []string{local.V7CorrelationFloorContextTag}
		}), "WATCH", "t2_awaiting_trigger"},
		// Backend RR not computable: same cap-avoidance as #161.
		{"rr not computable", noTargets, "WATCH", "t2_awaiting_trigger"},
		// Backend RR below the T2 floor: the shared wait fires before
		// the T2 rule ever runs (pre-existing gate, untouched).
		{"rr below floor", passthrough162Coin("ignition_early_long", "WATCH", func(c *CandidateCoin) {
			c.V7Targets = []local.V7Target{{Price: 101.0}} // RR 1.0 < 1.2
		}), "WATCH", "backend_rr_infeasible"},
		// Observe-only rows keep their own earlier branch.
		{"observe only", passthrough162Coin("reclaim_breakout_long", "WATCH", func(c *CandidateCoin) {
			c.V7RiskTags = []string{"t2_observe_only"}
		}), "WATCH", "t2_observe_only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, reason := classify162(tc.coin)
			if tier != tc.tier || reason != tc.reason {
				t.Fatalf("%s/%s, want %s/%s", tier, reason, tc.tier, tc.reason)
			}
		})
	}
}

// (b) The bounded exclusions: rows whose pending condition is not a
// price trigger the conditional machinery can enforce, or that have
// no trigger to enforce, stay in WATCH unchanged.
func TestHunterV7PassThrough162BoundedExclusionsStayWatch(t *testing.T) {
	narrowed := defaultGeometry()
	narrowed.T2ReviewableStates = map[string][]string{store.V7ClassTrigger: {"ARMED"}}
	cases := []struct {
		name     string
		coin     CandidateCoin
		geometry HunterV7ExecutionGeometry
	}{
		// IMPULSE WATCH is a score admission gate, not a price trigger.
		{"impulse watch", passthrough162Coin("momentum_continuation_watch_short", "WATCH"), defaultGeometry()},
		// No recorded trigger price: the conditional registration
		// cannot engage.
		{"no trigger price", t2Coin("ignition_early_long", "WATCH"), defaultGeometry()},
		// TRIGGERED row held only by a narrowed reviewable-states
		// override: the trigger already fired, no deferral.
		{"triggered but states narrowed", passthrough162Coin("ignition_early_long", "TRIGGERED"), narrowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTierWithGeometry(tc.coin, tc.geometry)
			if tier != "WATCH" || reason != "t2_awaiting_trigger" {
				t.Fatalf("%s/%s, want WATCH/t2_awaiting_trigger", tier, reason)
			}
		})
	}
}

// (c) Shadow-only outcomes are byte-identical to pre-#162: a shadow
// row in a pre-trigger state keeps WATCH/t2_awaiting_trigger (the
// pass-through refuses it, and the shadow cap never touches WATCH),
// and a shadow row promoted by a reviewable state still lands on
// WATCH/class_shadow_only.
func TestHunterV7PassThrough162ShadowOnlyUnchanged(t *testing.T) {
	cases := []struct {
		name         string
		coin         CandidateCoin
		tier, reason string
	}{
		{"shadow watch", passthrough162Coin("ignition_early_short", "WATCH"), "WATCH", "t2_awaiting_trigger"},
		{"shadow armed", passthrough162Coin("ignition_early_short", "ARMED"), "WATCH", "t2_awaiting_trigger"},
		{"shadow triggered", passthrough162Coin("ignition_early_short", "TRIGGERED"), "WATCH", "class_shadow_only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, reason := classify162(tc.coin)
			if tier != tc.tier || reason != tc.reason {
				t.Fatalf("%s/%s, want %s/%s", tier, reason, tc.tier, tc.reason)
			}
		})
	}
}
