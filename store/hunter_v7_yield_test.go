package store

import (
	"math"
	"testing"
	"time"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// Exact Poisson intervals against tabulated values (Garwood, 90%).
func TestU13PoissonInterval(t *testing.T) {
	for _, c := range []struct {
		k      int
		lo, hi float64
	}{
		{0, 0, 2.9957},
		{2, 0.3554, 6.2958},
		{4, 1.3663, 9.1535},
		{10, 5.4254, 16.9622},
	} {
		lo, hi := V7PoissonInterval(c.k, 0.90)
		if !near(lo, c.lo, 1e-3) || !near(hi, c.hi, 1e-3) {
			t.Errorf("k=%d: [%.4f, %.4f], want [%.4f, %.4f]", c.k, lo, hi, c.lo, c.hi)
		}
	}
}

func TestU13RV24(t *testing.T) {
	cfg := EffectiveV7YieldTarget(nil)
	if cfg.RV24Tier(0.249) != V7RV24TierLow || cfg.RV24Tier(0.25) != V7RV24TierMid || cfg.RV24Tier(0.35) != V7RV24TierHigh {
		t.Fatal("tier edges must be LOW < 0.25 <= MID < 0.35 <= HIGH")
	}
	// Alternating +-1% log returns: population std = 1% -> RV24 = 1.0.
	closes := []float64{100}
	for i := 0; i < 30; i++ {
		closes = append(closes, closes[len(closes)-1]*math.Exp([]float64{0.01, -0.01}[i%2]))
	}
	if rv := V7RV24FromCloses(closes, 24); !near(rv, 1.0, 1e-9) {
		t.Fatalf("rv = %v", rv)
	}
	// Series: 40 closed hours, forming bar priced by the latest closed 5m bar.
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	var h1, m5 []V7PriceBar
	for i := 0; i < 40; i++ {
		h1 = append(h1, V7PriceBar{OpenMs: base + int64(i)*3_600_000, Close: 100 * (1 + 0.001*float64(i%3))})
	}
	for i := 0; i < 40*12+12; i++ {
		m5 = append(m5, V7PriceBar{OpenMs: base + int64(i)*300_000, Close: 100 + float64(i)/1000})
	}
	s := NewV7RV24Series(cfg, h1, m5, 300_000)
	at := base + 30*3_600_000 + 7*60_000 // 07 minutes into hour 30
	var want []float64
	for i := 1; i < 30; i++ {
		want = append(want, h1[i].Close)
	}
	want = append(want, m5[30*12].Close) // the 5m bar 30:00-30:05 closed at 30:05
	rv, ok := s.At(at)
	if !ok || !near(rv, V7RV24FromCloses(want, 24), 1e-12) {
		t.Fatalf("At = %v %v, want %v", rv, ok, V7RV24FromCloses(want, 24))
	}
	if _, ok := s.At(base + 10*3_600_000); ok {
		t.Fatal("fewer than 25 bars must report !ok")
	}
	if s.Tier(base+10*3_600_000) != V7RV24TierNone {
		t.Fatal("no tier without bars")
	}
	r := s.RoundsPerTier(base+30*3_600_000, base+31*3_600_000)
	total := 0
	for _, n := range r {
		total += n
	}
	if total != 12 {
		t.Fatalf("one hour = 12 rounds, got %v", r)
	}
}

func TestU13YieldDefaultsAndRows(t *testing.T) {
	cfg := EffectiveV7YieldTarget(nil)
	for _, stage := range []string{V7YieldStageT2Triggered, V7YieldStageM2Admitted(1.2), V7YieldStageM2Admitted(1.5)} {
		for _, tier := range V7RV24Tiers {
			if v, ok := cfg.Target(stage, tier); !ok || v <= 0 {
				t.Fatalf("%s %s target missing", stage, tier)
			}
		}
	}
	// The defaults are the measured lower bounds: LOW 177 / 3814 rounds.
	lo, _ := V7PoissonInterval(177, 0.90)
	if v, _ := cfg.Target(V7YieldStageT2Triggered, V7RV24TierLow); !near(v, lo/3814, 1e-15) {
		t.Fatalf("default %v vs recomputed %v", v, lo/3814)
	}
	// Threshold over the study rounds reproduces the measured G3 values.
	rounds := map[string]int{V7RV24TierLow: 3814, V7RV24TierMid: 4678, V7RV24TierHigh: 5332}
	if th, ok := V7YieldThreshold(V7YieldStageM2Admitted(1.5), rounds, cfg); !ok || !near(th, 2.539, 1e-3) {
		t.Fatalf("rr1.5 threshold %v", th)
	}
	if th, _ := V7YieldThreshold(V7YieldStageM2Admitted(1.2), rounds, cfg); !near(th, 8.632, 1e-3) {
		t.Fatalf("rr1.2 threshold %v", th)
	}
	if _, ok := V7YieldThreshold("unknown_stage", rounds, cfg); ok {
		t.Fatal("a stage without targets has no threshold")
	}
	over := EffectiveV7YieldTarget(&V7YieldTargetConfig{Targets: map[string]map[string]float64{V7YieldStageT2Triggered: {V7RV24TierHigh: 0.1}}})
	if v, _ := over.Target(V7YieldStageT2Triggered, V7RV24TierHigh); v != 0.1 {
		t.Fatal("override")
	}
	if v, _ := over.Target(V7YieldStageT2Triggered, V7RV24TierLow); v <= 0 {
		t.Fatal("other tiers keep their defaults")
	}
	row := BuildV7YieldRow(V7YieldStageT2Triggered, V7RV24TierHigh, 2, 1000, over)
	if row.Rate != 0.002 || !row.BelowTarget || row.Target == nil || *row.Expected != 100 {
		t.Fatalf("row = %+v", row)
	}
	if r := BuildV7YieldRow(V7YieldStageT2Triggered, V7RV24TierHigh, 100, 1000, over); r.BelowTarget {
		t.Fatal("a rate at the target is not below it")
	}
}

func TestU13T2TriggeredEventsDedupe(t *testing.T) {
	tp := func(v float64) *float64 { return &v }
	t0 := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	rec := func(sec int, sym, setup, state string, trig float64) HunterV7SignalRecord {
		return HunterV7SignalRecord{Timestamp: t0.Add(time.Duration(sec) * time.Second), Symbol: sym, SetupType: setup, Direction: "LONG",
			Stack: V7StackT2, LifecycleStateAtDecision: state, TriggerPrice: tp(trig)}
	}
	evs := V7T2TriggeredEvents([]HunterV7SignalRecord{
		rec(300, "A", "ignition_early_long", "TRIGGERED", 1),
		rec(0, "A", "t2_ignition_early_long", "TRIGGERED", 1), // same event, prefixed name, earlier
		rec(600, "A", "ignition_early_long", "TRIGGERED", 2),  // new trigger level
		rec(10, "B", "ignition_early_long", "WATCH", 1),
		{Timestamp: t0, Symbol: "C", SetupType: "x", Direction: "LONG", Stack: V7StackNative, LifecycleStateAtDecision: "TRIGGERED"},
	})
	if len(evs) != 2 || !evs[0].Time.Equal(t0) {
		t.Fatalf("events = %+v", evs)
	}
}
