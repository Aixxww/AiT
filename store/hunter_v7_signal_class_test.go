package store

import "testing"

func TestDefaultV7SignalClassRegistryApprovedVerdicts(t *testing.T) {
	reg := DefaultV7SignalClassRegistry()
	shadow := []string{
		"ignition_early_short",             // OD-27
		"momentum_continuation_watch_long", // OD-28
		"confirmed_breakout_long",          // OD-14
		"confirmed_breakout_short",         // OD-14
	}
	for _, s := range shadow {
		if !reg[s].ShadowOnly() {
			t.Errorf("%s must be shadow_only, got %+v", s, reg[s])
		}
	}
	want := map[string]string{
		"ignition_early_long":        V7ClassTrigger,
		"momentum_continuation_long": V7ClassImpulse,
		"funding_reversal":           V7ClassContext, // OD-13
		"mms_bottom_wake_long":       V7ClassReversal,
		"leader_momentum_long":       V7ClassImpulse,
		"trend_breakout_long":        V7ClassTrigger,
	}
	for s, c := range want {
		if reg[s].Class != c {
			t.Errorf("%s class = %q, want %q", s, reg[s].Class, c)
		}
	}
	if reg["trend_breakout_long"].ShadowOnly() {
		t.Errorf("native trend_breakout_long admission is held for owner confirmation (not enforced in U6a)")
	}
	if _, ok := reg["mms_trend_ride_long"]; ok {
		t.Errorf("mms_trend_ride_long is UNASSIGNED (no H.3 verdict)")
	}
}

func TestEffectiveV7SignalClassesOverrides(t *testing.T) {
	var nilCfg *HunterConfig
	if len(nilCfg.EffectiveV7SignalClasses()) != len(DefaultV7SignalClassRegistry()) {
		t.Fatalf("nil config must return the defaults")
	}
	h := &HunterConfig{V7SignalClasses: map[string]V7SignalClassEntry{
		"ignition_early_short": {Class: V7ClassTrigger},
		"new_setup":            {Class: V7ClassReversal, Provisional: true},
	}}
	reg := h.EffectiveV7SignalClasses()
	if reg["ignition_early_short"].ShadowOnly() {
		t.Fatalf("override must replace the entry")
	}
	if reg["new_setup"].Class != V7ClassReversal {
		t.Fatalf("override must add entries")
	}
	if !DefaultV7SignalClassRegistry()["ignition_early_short"].ShadowOnly() {
		t.Fatalf("overrides must not mutate the defaults")
	}
}
