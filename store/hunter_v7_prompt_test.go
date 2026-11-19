package store

import "testing"

func TestU11PromptConfigDefaults(t *testing.T) {
	d := EffectiveV7Prompt(nil)
	if d.HybridV2() || d.BarsPerTimeframe != 30 || len(d.Timeframes) != 4 || d.DoctrineVersion() != V7DoctrineVersionNone {
		t.Fatalf("defaults = %+v", d)
	}
	if d.LLMRoles[V7ClassTrigger] != V7LLMRoleDecideBeforeTrigger || d.LLMRoles[V7ClassContext] != V7LLMRoleNone {
		t.Fatalf("roles = %v", d.LLMRoles)
	}
	h := EffectiveV7Prompt(&V7PromptConfig{PayloadVersion: V7PayloadVersionHybridV2, BarsPerTimeframe: 20})
	if !h.HybridV2() || h.BarsPerTimeframe != 20 || h.DoctrineVersion() != V7DoctrineVersionHybridV2 {
		t.Fatalf("hybrid = %+v", h)
	}
}

// D2 (a): config and registry accept both names.
func TestU11SetupAliases(t *testing.T) {
	if V7CanonicalSetupType("t2_ignition_early_long") != "ignition_early_long" || V7CanonicalSetupType("leader_momentum_long") != "leader_momentum_long" {
		t.Fatal("canonical")
	}
	m := map[string]int{"ignition_early_long": 1, "t2_trend_pullback_short": 2}
	for setup, want := range map[string]int{"ignition_early_long": 1, "t2_ignition_early_long": 1, "trend_pullback_short": 2, "t2_trend_pullback_short": 2} {
		if got, ok := LookupV7Setup(m, setup); !ok || got != want {
			t.Errorf("lookup %s = %d %v", setup, got, ok)
		}
	}
	if _, ok := LookupV7Setup(m, "leader_momentum_long"); ok {
		t.Fatal("unknown setup must miss")
	}
	// A registry override under the t2_ name replaces the canonical entry.
	h := &HunterConfig{V7SignalClasses: map[string]V7SignalClassEntry{"t2_ignition_early_long": {Class: V7ClassImpulse}}}
	reg := h.EffectiveV7SignalClasses()
	if reg["ignition_early_long"].Class != V7ClassImpulse {
		t.Fatalf("override under t2_ name not applied: %+v", reg["ignition_early_long"])
	}
	if _, ok := reg["t2_ignition_early_long"]; ok {
		t.Fatal("registry keys must be canonical")
	}
	// Both names: the prefixed (new) name wins.
	both := canonicalV7SetupKeys(map[string]int{"x_long": 1, "t2_x_long": 2})
	if both["x_long"] != 2 || len(both) != 1 {
		t.Fatalf("both names = %v", both)
	}
	rc := RiskControlConfig{SetupGeometryOverrides: map[string]V7SetupGeometryOverride{"t2_bb_oi_accumulation_long": {MinRRCap: 1.1}}}
	if o, ok := LookupV7Setup(rc.EffectiveV7SetupGeometryOverrides(), "bb_oi_accumulation_long"); !ok || o.MinRRCap != 1.1 {
		t.Fatalf("setup override under t2_ name: %+v %v", o, ok)
	}
	if V7Family("t2_ignition_early_long") != "ignition_early" || V7Family("ignition_early_short") != "ignition_early" {
		t.Fatal("family must ignore the t2_ prefix")
	}
}
