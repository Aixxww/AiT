package local

import (
	"math"
	"testing"
)

// Fix 14 (P2-14): T2 outputs carry TP1RR / TP2RR from the setup's own
// entry, stop and targets (|TP - entry| / |entry - stop|, the native
// formula), and leave them empty (0) where the geometry cannot give one.

func TestV7T2PlannedRR(t *testing.T) {
	cases := []struct {
		name                  string
		dir                   string
		entry, stop, tp, want float64
	}{
		{"long", "LONG", 100, 98, 103.6, 1.8},
		{"short", "SHORT", 100, 102, 94, 3.0},
		{"long stop above entry", "LONG", 100, 101, 103, 0},
		{"long target below entry", "LONG", 100, 98, 99, 0},
		{"short stop below entry", "SHORT", 100, 99, 97, 0},
		{"short target above entry", "SHORT", 100, 102, 101, 0},
		{"zero risk", "LONG", 100, 100, 103, 0},
		{"missing target", "LONG", 100, 98, 0, 0},
		{"missing entry", "LONG", 0, 98, 103, 0},
		{"missing stop", "SHORT", 100, 0, 97, 0},
		{"missing stop long", "LONG", 100, 0, 103, 0},
		{"unknown direction", "", 100, 98, 103, 0},
	}
	for _, c := range cases {
		if got := v7T2PlannedRR(c.dir, c.entry, c.stop, c.tp); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// The scanner builds TP1 at 1.8R and TP2 at 3.0R from its entry; the
// output reports exactly that, for every lifecycle state.
func TestT2OutputCarriesTPRR(t *testing.T) {
	for _, dir := range []string{"LONG", "SHORT"} {
		e, s, t1, t2 := v7t2TriggerBasedPlan(100, 1.5, 100, dir)
		sig := &V7T2Signal{Symbol: "AUSDT", Direction: dir, SetupType: "ignition_early_" + map[string]string{"LONG": "long", "SHORT": "short"}[dir],
			Status: "WATCH", Score: 70, Trigger: 100, Entry: e, Stop: s, Target1: t1, Target2: t2}
		for _, state := range []string{"", "WATCH", "ARMED", "TRIGGERED"} {
			o := sig.ToV7SignalOutputWithLifecycle("", state)
			if math.Abs(o.TP1RR-v7T2MinRR) > 1e-9 || math.Abs(o.TP2RR-v7T2TP2Multiple) > 1e-9 {
				t.Fatalf("%s %q: TP1RR %v TP2RR %v, want %v / %v", dir, state, o.TP1RR, o.TP2RR, v7T2MinRR, v7T2TP2Multiple)
			}
			if o.TP1Price != t1 || o.TP2Price != t2 {
				t.Fatal("target prices changed")
			}
		}
	}
	// Broken geometry is left empty, not invented.
	bad := &V7T2Signal{Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", Entry: 100, Stop: 100, Target1: 101.8, Target2: 103}
	if o := bad.ToV7SignalOutput(""); o.TP1RR != 0 || o.TP2RR != 0 {
		t.Fatalf("zero-risk signal: %v / %v", o.TP1RR, o.TP2RR)
	}
}
