package local

import "testing"

// U11, D2 (a): Task-2 setup types are emitted as t2_<setup> when the
// prefix is on; the module's own lists accept both names.
func TestU11T2PrefixAtEmission(t *testing.T) {
	out := []V7SignalOutput{{SetupType: "ignition_early_long"}, {SetupType: "t2_trend_pullback_short"}}
	v7T2PrefixSetupTypes(out)
	if out[0].SetupType != "t2_ignition_early_long" || out[1].SetupType != "t2_trend_pullback_short" {
		t.Fatalf("prefixed = %v / %v", out[0].SetupType, out[1].SetupType)
	}

	cfg := &V7T2Config{EnabledSetups: []string{"t2_ignition_early_long", "bb_oi_accumulation_long"}}
	for setup, want := range map[string]bool{"ignition_early_long": true, "bb_oi_accumulation_long": true, "trend_pullback_long": false} {
		if got := cfg.familyEnabled(setup); got != want {
			t.Errorf("familyEnabled(%s) = %v, want %v", setup, got, want)
		}
	}
	p := V7T2LifecycleParams{ObserveOnlyOverride: true, ObserveOnlySetups: []string{"t2_reclaim_breakout_short"}}
	if !p.observeOnly("reclaim_breakout_short") || p.observeOnly("reclaim_breakout_long") {
		t.Fatal("observe-only list must accept the t2_ name")
	}
}

// The structure class and family read the canonical name, so a prefixed
// SWING family (bb_oi_, trend_, ...) keeps its class.
func TestU11StructureClassIgnoresPrefix(t *testing.T) {
	for _, s := range []string{"trend_pullback_short", "bb_oi_accumulation_long"} {
		if V7StructureClassForSetup("t2_"+s) != V7StructureClassForSetup(s) || V7StructureClassForSetup(s) != "SWING" {
			t.Errorf("%s: class differs under the t2_ name", s)
		}
	}
	if V7StructureFamilyForSetup("t2_trend_pullback_short") != V7FamilyPullbackReversal {
		t.Fatal("family must read the canonical name")
	}
}

// CONTEXT setups match under either name in the final direction pass.
func TestU11FinalConflictContextSetupAlias(t *testing.T) {
	cfg := V7FinalConflictConfig{ContextNonVoting: true, ContextSetups: map[string]bool{"compression_oi_watch_long": true}}
	if !cfg.isContext(V7SignalOutput{SetupType: "t2_compression_oi_watch_long"}) {
		t.Fatal("prefixed CONTEXT setup must stay non-voting")
	}
}
