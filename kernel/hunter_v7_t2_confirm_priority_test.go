package kernel

import (
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
)

// TA-1 kernel unit: a Task-2 candidate's tier reason must survive the
// required-confirmation check. Before the fix, any T2 candidate carrying
// required confirmations (and, like every T2 candidate, no confirmation
// summary) was tiered WATCH/confirmation_missing_summary before the T2
// class lifecycle rule ran.
func TestT2TierReasonSurvivesRequiredConfirmations(t *testing.T) {
	g := defaultGeometry()
	rrInfeasible := t2Coin("ignition_early_long", "WATCH")
	rrInfeasible.V7Targets = []local.V7Target{{Price: 101.0}} // RR 1.0 < 1.2
	cases := []struct {
		name         string
		coin         CandidateCoin
		tier, reason string
	}{
		{"trigger watch", t2Coin("ignition_early_long", "WATCH"), "WATCH", "t2_awaiting_trigger"},
		{"impulse watch", t2Coin("momentum_continuation_watch_short", "WATCH"), "WATCH", "t2_awaiting_trigger"},
		{"observe-only watch", t2Coin("reclaim_breakout_long", "WATCH", "t2_observe_only"), "WATCH", "t2_observe_only"},
		{"observe-only triggered", t2Coin("reclaim_breakout_long", "TRIGGERED", "t2_observe_only"), "WATCH", "t2_observe_only"},
		{"rr infeasible watch", rrInfeasible, "WATCH", "backend_rr_infeasible"},
		{"shadow-only triggered", t2Coin("ignition_early_short", "TRIGGERED"), "WATCH", "class_shadow_only"},
		{"trigger triggered", t2Coin("ignition_early_long", "TRIGGERED"), "REVIEWABLE", "t2_trigger_triggered"},
		{"unclassified", t2Coin("no_such_t2_setup", "WATCH"), "WATCH", "t2_unclassified"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTierWithGeometry(tc.coin, g)
			if tier != tc.tier || reason != tc.reason {
				t.Fatalf("without confirmations: %s/%s, want %s/%s", tier, reason, tc.tier, tc.reason)
			}
			with := tc.coin
			with.V7RequiredConfirms = []string{"live_price_in_entry_zone"}
			with.V7ConfirmSummary = nil // T2 candidates never carry one
			tier, reason = classifyHunterV7CandidateTierWithGeometry(with, g)
			if tier != tc.tier || reason != tc.reason {
				t.Fatalf("with required confirmations: %s/%s, want %s/%s (T2 reason must not be replaced)", tier, reason, tc.tier, tc.reason)
			}
		})
	}
}

// Native candidates keep the existing behaviour: required confirmations
// without a summary still wait on confirmation_missing_summary.
func TestNativeRequiredConfirmationWaitUnchanged(t *testing.T) {
	g := defaultGeometry()
	c := stopCoin("", 1.0, 3.0)
	c.V7SetupType = "leader_momentum_long"
	c.V7Status = "candidate"
	c.V7RequiredConfirms = []string{"live_price_in_entry_zone"}
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(c, g); tier != "WATCH" || reason != "confirmation_missing_summary" {
		t.Fatalf("native: %s/%s, want WATCH/confirmation_missing_summary", tier, reason)
	}
}

// Through the production assembly: a T2 WATCH output carrying the TA-1
// confirmation keeps t2_awaiting_trigger, and the confirmation still
// reaches the candidate (and so the prompt's must_confirm).
func TestAssemblyKeepsT2ReasonWithRequiredConfirmations(t *testing.T) {
	sig := (&local.V7T2Signal{Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", Status: "WATCH",
		Score: 70, Trigger: 100, Entry: 100, Stop: 99, Target1: 101.8, Target2: 103}).ToV7SignalOutputWithLifecycle("", "WATCH")
	base := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "BOTH", defaultGeometry())
	sig.RequiredConfirms = []string{"live_price_in_entry_zone"}
	with := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "BOTH", defaultGeometry())
	if len(base) != 1 || len(with) != 1 {
		t.Fatalf("assembly returned %d/%d candidates", len(base), len(with))
	}
	if with[0].V7ExecutionTier != base[0].V7ExecutionTier || with[0].V7TierReason != base[0].V7TierReason {
		t.Fatalf("tier changed by confirmations: %s/%s -> %s/%s",
			base[0].V7ExecutionTier, base[0].V7TierReason, with[0].V7ExecutionTier, with[0].V7TierReason)
	}
	if with[0].V7TierReason == "confirmation_missing_summary" {
		t.Fatal("T2 reason replaced by confirmation_missing_summary")
	}
	if len(with[0].V7RequiredConfirms) != 1 || with[0].V7RequiredConfirms[0] != "live_price_in_entry_zone" {
		t.Fatalf("required confirmations not carried: %v", with[0].V7RequiredConfirms)
	}
}
