package local

import (
	"reflect"
	"strings"
	"testing"
)

// Fix 15: the reported T2 settings are the ones the module applies.

func TestT2EffectiveSettingsDefaults(t *testing.T) {
	var nilCfg *V7T2Config
	e := nilCfg.EffectiveSettings()
	if e.Enabled || len(e.EnabledSetups) != len(V7T2SetupTypes) || e.EnabledSetupsFrom != "all (empty list)" {
		t.Fatalf("nil config %+v", e)
	}
	if e.MinAlertScore != 82 || e.MinWatchScore != 72 || e.ArmedExpiryHours != 4 || e.WatchExpiryHours != 2 ||
		e.CooldownMinutes != 60 || !e.TriggerEnabled || e.TriggeredSurfaceMinutes != 15 {
		t.Fatalf("scanner constants %+v", e)
	}
	if len(e.ObserveOnlySetups) != len(v7T2ObserveOnlySetups) || e.ObserveOnlyFrom != "scanner list" {
		t.Fatalf("observe-only %v", e.ObserveOnlySetups)
	}
	// Every reported observe-only setup is forced by the module, and no other.
	for _, s := range V7T2SetupTypes {
		sig := &V7T2Signal{SetupType: s}
		v7t2ApplyObserveOnly(sig, V7T2LifecycleParams{})
		listed := false
		for _, o := range e.ObserveOnlySetups {
			listed = listed || o == s
		}
		if sig.ObserveOnly != listed {
			t.Fatalf("%s: module observe-only %v, reported %v", s, sig.ObserveOnly, listed)
		}
	}
}

func TestT2EffectiveSettingsConfigured(t *testing.T) {
	var list []string
	for _, s := range V7T2SetupTypes {
		if !strings.HasPrefix(s, "bb_oi_") {
			list = append(list, s)
		}
	}
	list[0] = "t2_" + list[0] // prefixed names are accepted
	list = append(list, "no_such_setup")
	c := &V7T2Config{Enabled: true, EnabledSetups: list, SetupPrefix: true,
		Lifecycle: V7T2LifecycleParams{MinAlertScore: 85, CooldownMinutes: 30, DisableTrigger: true, ObserveOnlyOverride: true}}
	e := c.EffectiveSettings()
	if !e.Enabled || e.EnabledSetupsFrom != "configured list" || len(e.EnabledSetups) != len(V7T2SetupTypes)-2 ||
		!reflect.DeepEqual(e.DisabledSetups, []string{"bb_oi_accumulation_long", "bb_oi_accumulation_short"}) ||
		!reflect.DeepEqual(e.UnknownConfiguredSetups, []string{"no_such_setup"}) {
		t.Fatalf("setups %+v", e)
	}
	for _, s := range e.EnabledSetups {
		if !c.familyEnabled(s) {
			t.Fatalf("%s reported enabled but the module drops it", s)
		}
	}
	if e.MinAlertScore != 85 || e.CooldownMinutes != 30 || e.TriggerEnabled || len(e.ObserveOnlySetups) != 0 ||
		e.ObserveOnlyFrom != "configured override" || !e.SetupPrefix {
		t.Fatalf("lifecycle %+v", e)
	}
}
