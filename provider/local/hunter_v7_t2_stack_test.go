package local

import "testing"

// U2/P2a: Task-2 outputs are marked so downstream RR admissions can apply
// the T2 floor; native outputs stay unmarked.
func TestT2SignalOutputCarriesStack(t *testing.T) {
	sig := &V7T2Signal{Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", Score: 70, Entry: 100, Trigger: 101, Stop: 99, Target1: 102.8, Target2: 104}
	out := sig.ToV7SignalOutput(V7MarketRegime(""))
	if out.Stack != V7StackT2 {
		t.Fatalf("T2 output stack = %q, want %q", out.Stack, V7StackT2)
	}
	if (V7SignalOutput{}).Stack != "" {
		t.Fatalf("native default stack must be empty")
	}
}
