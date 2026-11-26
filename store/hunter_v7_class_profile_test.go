package store

import "testing"

func TestV7ClassProfilesDefaultInheritAndOverride(t *testing.T) {
	d := RiskControlConfig{}.EffectiveV7ClassProfiles()
	if len(d) != 4 {
		t.Fatalf("default profiles = %d classes", len(d))
	}
	for c, p := range d {
		if !p.Native.IsZero() || !p.T2.IsZero() {
			t.Fatalf("default profile %s must inherit everything: %+v", c, p)
		}
	}
	rc := RiskControlConfig{ClassProfiles: map[string]V7ClassRiskProfile{V7ClassTrigger: {T2: V7StackRiskProfile{MinRiskRewardRatio: 1.4}}}}
	p := rc.EffectiveV7ClassProfiles()
	if p[V7ClassTrigger].ForStack(V7StackT2).MinRiskRewardRatio != 1.4 || !p[V7ClassTrigger].ForStack(V7StackNative).IsZero() || !p[V7ClassImpulse].T2.IsZero() {
		t.Fatalf("override = %+v", p)
	}
}

func TestV7SetupGeometryOverrides(t *testing.T) {
	d := RiskControlConfig{}.EffectiveV7SetupGeometryOverrides()
	// #138 M9: defaults relax the TP cap only; no default lowers the RR floor.
	if d["intraday_scalp_long"] != (V7SetupGeometryOverride{MaxTPFloorPct: 3}) ||
		d["displacement_momentum_long"] != (V7SetupGeometryOverride{MaxTPFloorPct: 12}) || len(d) != 3 {
		t.Fatalf("defaults = %+v", d)
	}
	for setup, o := range d {
		if o.MinRRCap != 0 {
			t.Errorf("default override %s carries MinRRCap %.2f; an RR floor below the strategy floor must be explicit config", setup, o.MinRRCap)
		}
	}
	rc := RiskControlConfig{SetupGeometryOverrides: map[string]V7SetupGeometryOverride{
		"intraday_scalp_long": {}, "range_reversion": {MinRRCap: 1.1},
	}}
	o := rc.EffectiveV7SetupGeometryOverrides()
	if _, ok := o["intraday_scalp_long"]; ok || o["range_reversion"].MinRRCap != 1.1 || len(o) != 3 {
		t.Fatalf("overrides = %+v", o)
	}
}

func TestV7EffectiveProfileVersion(t *testing.T) {
	base := V7SignalClassRegistryVersion + "/" + V7ClassProfileVersion
	if v := V7EffectiveProfileVersion(RiskControlConfig{}, &HunterConfig{}); v != base {
		t.Fatalf("default = %s", v)
	}
	if v := V7EffectiveProfileVersion(RiskControlConfig{ClassProfiles: map[string]V7ClassRiskProfile{V7ClassImpulse: {}}}, nil); v != base+"+config" {
		t.Fatalf("profiles override = %s", v)
	}
	if v := V7EffectiveProfileVersion(RiskControlConfig{}, &HunterConfig{V7SignalClasses: map[string]V7SignalClassEntry{"x": {Class: V7ClassImpulse}}}); v != base+"+config" {
		t.Fatalf("registry override = %s", v)
	}
}
