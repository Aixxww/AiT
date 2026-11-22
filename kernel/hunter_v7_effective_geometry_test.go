package kernel

import (
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Fix 15: the reported geometry is the one the tier rules apply.

func TestEffectiveGeometryDefaults(t *testing.T) {
	g := HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true)
	e := HunterV7EffectiveGeometryOf(g)
	if e.NativeMinRR != local.V7MinExecutableRR || e.T2MinRR != store.DefaultT2MinRiskRewardRatio || e.T2MinSLMovePct != store.DefaultT2MinStopLossPriceMovePct {
		t.Fatalf("defaults %+v", e)
	}
	found := false
	for _, s := range e.ShadowOnlySetups {
		found = found || s == "ignition_early_short"
	}
	if !found {
		t.Fatalf("shadow-only setups %v lack ignition_early_short", e.ShadowOnlySetups)
	}
	if len(e.ClassRisk) == 0 || len(e.SetupOverrides) == 0 || len(e.T2ReviewableStates) == 0 {
		t.Fatalf("class risk / overrides / reviewable states missing: %+v", e)
	}
}

// Behavioural check: an RR just above the reported floor passes the
// backend RR check, just below fails, for both stacks.
func TestEffectiveGeometryFloorsMatchTierRule(t *testing.T) {
	g := defaultGeometry() // native 1.5, T2 1.2
	e := HunterV7EffectiveGeometryOf(g)
	for _, c := range []struct {
		stack string
		floor float64
	}{{"", e.NativeMinRR}, {local.V7StackT2, e.T2MinRR}} {
		if r := hunterV7BackendCappedRRWaitReason(rrCoin(c.stack, (c.floor+0.05)*2.5), g); r != "" {
			t.Fatalf("stack %q: RR above the reported floor %.2f failed: %s", c.stack, c.floor, r)
		}
		if r := hunterV7BackendCappedRRWaitReason(rrCoin(c.stack, (c.floor-0.05)*2.5), g); r != "backend_rr_infeasible" {
			t.Fatalf("stack %q: RR below the reported floor %.2f passed", c.stack, c.floor)
		}
	}
}

// A class profile shows up in the per-class values, and the configured
// conditional classes and a custom registry are reported.
func TestEffectiveGeometryReportsConfiguredValues(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.Hunter = &store.HunterConfig{}
	cfg.RiskControl.MinRiskRewardRatio = 1.2
	cfg.RiskControl.T2MinRiskRewardRatio = 1.3
	cfg.RiskControl.ClassProfiles = map[string]store.V7ClassRiskProfile{store.V7ClassTrigger: {T2: store.V7StackRiskProfile{MinRiskRewardRatio: 1.6}}}
	g := HunterV7ExecutionGeometryForConfig(cfg)
	g.ConditionalClasses = map[string]bool{store.V7ClassTrigger: true, store.V7ClassImpulse: false}
	e := HunterV7EffectiveGeometryOf(g)
	if e.NativeMinRR != 1.2 || e.T2MinRR != 1.3 {
		t.Fatalf("floors %v / %v", e.NativeMinRR, e.T2MinRR)
	}
	var trig, imp float64
	for _, c := range e.ClassRisk {
		if c.Stack == store.V7StackT2 && c.Class == store.V7ClassTrigger {
			trig = c.MinRR
		}
		if c.Stack == store.V7StackT2 && c.Class == store.V7ClassImpulse {
			imp = c.MinRR
		}
	}
	if trig != 1.6 || imp != 1.3 {
		t.Fatalf("per-class T2 floors TRIGGER %v IMPULSE %v, want 1.6 / 1.3", trig, imp)
	}
	if len(e.ConditionalClasses) != 1 || e.ConditionalClasses[0] != store.V7ClassTrigger {
		t.Fatalf("conditional classes %v", e.ConditionalClasses)
	}
}
