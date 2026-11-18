package store

import "testing"

func TestEffectiveV7DataTimingDefaultsOff(t *testing.T) {
	d := EffectiveV7DataTiming(nil)
	if d.FeatureBars != V7FeatureBarsForming || d.CycleAlignment != V7CycleWallClock || d.AlignIntervalMinutes != 5 || d.SettleSeconds != 30 || len(d.FreshnessBudgetSeconds) != 0 {
		t.Fatalf("defaults = %+v", d)
	}
	if b := EffectiveV7DataTiming(&V7DataTimingConfig{FeatureBars: "bogus", CycleAlignment: "bogus"}); b.FeatureBars != V7FeatureBarsForming || b.CycleAlignment != V7CycleWallClock {
		t.Fatalf("unknown values must fall back to the defaults: %+v", b)
	}
	o := EffectiveV7DataTiming(&V7DataTimingConfig{FeatureBars: V7FeatureBarsClosed, CycleAlignment: V7CycleAligned, AlignIntervalMinutes: 15, SettleSeconds: 10,
		FreshnessBudgetSeconds: map[string]float64{V7ClassImpulse: 60}})
	if o.FeatureBars != V7FeatureBarsClosed || o.CycleAlignment != V7CycleAligned || o.AlignIntervalMinutes != 15 || o.SettleSeconds != 10 || o.FreshnessBudgetSeconds[V7ClassImpulse] != 60 {
		t.Fatalf("overrides = %+v", o)
	}
}
