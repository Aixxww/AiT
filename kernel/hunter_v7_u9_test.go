package kernel

import (
	"encoding/json"
	"strings"
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

func condGeometry(enabled bool) HunterV7ExecutionGeometry {
	g := defaultGeometry()
	g.T2MinSLMovePct = 0.5
	if enabled {
		g.ConditionalClasses = store.EffectiveV7ConditionalEntry(&store.V7ConditionalEntryConfig{Enabled: true}).ClassSet()
	}
	return g
}

func condSig(setup string, dir local.V7Direction, state string, tags ...string) local.V7SignalOutput {
	s := t2TagSignal(setup, dir, state)
	s.RiskTags = tags
	return s
}

func TestU9ConditionalTierRule(t *testing.T) {
	sigs := []local.V7SignalOutput{
		condSig("ignition_early_long", local.V7DirLong, "WATCH"),
		condSig("ignition_early_long", local.V7DirLong, "TRIGGERED"),
		condSig("momentum_continuation_watch_short", local.V7DirShort, "WATCH"),
		condSig("ignition_early_short", local.V7DirShort, "WATCH"),
		condSig("reclaim_breakout_long", local.V7DirLong, "WATCH", "t2_observe_only"),
	}
	for i := range sigs {
		sigs[i].Symbol = string(rune('A'+i)) + "USDT"
		sigs[i].SignalID = sigs[i].Symbol
	}
	off := AssembleHunterV7CandidateCoins(sigs, "BOTH", condGeometry(false))
	// U9 with conditional entry disabled: no t2_trigger_conditional_*
	// verdict. Since #162 the sole-reason pre-trigger row still advances
	// — flagged t2_awaiting_trigger_passthrough_* and stamped conditional,
	// so an approved open fails closed at registration (never at market).
	if off[0].V7ExecutionTier != "REVIEWABLE" || off[0].V7TierReason != "t2_awaiting_trigger_passthrough_trigger_watch" || !off[0].V7ConditionalEntry {
		t.Fatalf("disabled: %s/%s cond=%v", off[0].V7ExecutionTier, off[0].V7TierReason, off[0].V7ConditionalEntry)
	}
	on := AssembleHunterV7CandidateCoins(sigs, "BOTH", condGeometry(true))
	want := []struct {
		tier, reason string
		cond         bool
	}{
		{"REVIEWABLE", "t2_trigger_conditional_watch", true},
		{"REVIEWABLE", "t2_trigger_triggered", false},
		{"WATCH", "t2_awaiting_trigger", false}, // IMPULSE not enabled
		{"WATCH", "class_shadow_only", false},
		{"WATCH", "t2_observe_only", false},
	}
	for i, w := range want {
		c := on[i]
		if c.V7ExecutionTier != w.tier || c.V7TierReason != w.reason || c.V7ConditionalEntry != w.cond {
			t.Errorf("%s/%s: %s/%s cond=%v, want %s/%s %v", c.V7SetupType, c.V7LifecycleState, c.V7ExecutionTier, c.V7TierReason, c.V7ConditionalEntry, w.tier, w.reason, w.cond)
		}
		if c.V7TriggerPrice != 100 {
			t.Errorf("trigger price not carried")
		}
	}
	ct := buildHunterV7ExecutionContract(on[0])
	if ct.EntryPermission != "conditional_on_trigger" || ct.TriggerPrice != 100 {
		t.Fatalf("contract = %+v", ct)
	}
	raw, _ := json.Marshal(buildHunterV7ExecutionContract(on[1]))
	if strings.Contains(string(raw), "trigger_price") || strings.Contains(string(raw), "max_drift_pct") || strings.Contains(string(raw), "drift_reference") {
		t.Fatalf("non-conditional contract must omit U9 fields: %s", raw)
	}
	ds := []Decision{{Symbol: on[0].Symbol, Action: "open_long", SelectedHunterV7SignalID: on[0].V7SignalID}}
	hydrateHunterV7DecisionExecutionPlans(ds, on)
	if !ds[0].HunterV7ConditionalEntry || ds[0].HunterV7TriggerPrice != 100 || ds[0].HunterV7InvalidationPrice != 99 {
		t.Fatalf("hydrated = %+v", ds[0])
	}
}

func TestU9DriftStamps(t *testing.T) {
	sigs := []local.V7SignalOutput{condSig("ignition_early_long", local.V7DirLong, "TRIGGERED")}
	c := AssembleHunterV7CandidateCoins(sigs, "BOTH", condGeometry(false))
	if c[0].V7RequiredMaxDrift != 0 || c[0].V7DriftReference != "" {
		t.Fatalf("default must stamp no drift: %+v", c[0].V7RequiredMaxDrift)
	}
	g := condGeometry(false)
	g.ClassProfiles = map[string]store.V7ClassRiskProfile{store.V7ClassTrigger: {T2: store.V7StackRiskProfile{MaxEntryPriceDeviationPct: 0.8, DriftReference: store.V7DriftReferenceTrigger}}}
	c = AssembleHunterV7CandidateCoins(sigs, "BOTH", g)
	if c[0].V7RequiredMaxDrift != 0.8 || c[0].V7DriftReference != store.V7DriftReferenceTrigger {
		t.Fatalf("profile drift = %v %q", c[0].V7RequiredMaxDrift, c[0].V7DriftReference)
	}
	ds := []Decision{{Symbol: c[0].Symbol, Action: "open_long", SelectedHunterV7SignalID: c[0].V7SignalID}}
	hydrateHunterV7DecisionExecutionPlans(ds, c)
	if ds[0].HunterV7MaxDriftPct != 0.8 || ds[0].HunterV7DriftReference != store.V7DriftReferenceTrigger || ds[0].HunterV7TriggerPrice != 100 {
		t.Fatalf("hydrated = %+v", ds[0])
	}
	if ct := buildHunterV7ExecutionContract(c[0]); ct.MaxDriftPct != 0.8 || ct.DriftReference != "trigger" {
		t.Fatalf("contract = %+v", ct)
	}
	// Trigger reference without a trigger price falls back to decision.
	sigs[0].TriggerPrice = 0
	if c = AssembleHunterV7CandidateCoins(sigs, "BOTH", g); c[0].V7DriftReference != "" {
		t.Fatalf("no trigger price must not stamp a trigger reference")
	}
}

func TestU9EngineWiringAndPrompt(t *testing.T) {
	mk := func(ce *store.V7ConditionalEntryConfig) (HunterV7ExecutionGeometry, string) {
		cfg := &store.StrategyConfig{}
		cfg.CoinSource.SourceType = "hunter_v7"
		cfg.CoinSource.Hunter = &store.HunterConfig{V7T2Enabled: true, V7ConditionalEntry: ce}
		e := NewStrategyEngine(cfg)
		var sb strings.Builder
		e.writeHunterV7ExecutionPreflightPrompt(&sb)
		return e.hunterV7ExecutionGeometry(), sb.String()
	}
	g, p := mk(nil)
	if len(g.ConditionalClasses) != 0 || strings.Contains(p, "conditional") {
		t.Fatalf("default must be off: %v", g.ConditionalClasses)
	}
	g, p = mk(&store.V7ConditionalEntryConfig{Enabled: true})
	if !g.ConditionalClasses[store.V7ClassTrigger] || len(g.ConditionalClasses) != 1 || !strings.Contains(p, "conditional_on_trigger") || !strings.Contains(p, "120 minutes") {
		t.Fatalf("enabled wiring: %v / prompt note missing", g.ConditionalClasses)
	}
}

// U10: closed-bar features are opt-in.
func TestU10ClosedBarFeaturesWiring(t *testing.T) {
	if hunterV7ClosedBarFeatures(nil) || hunterV7ClosedBarFeatures(&store.HunterConfig{}) {
		t.Fatalf("default must keep the forming bar")
	}
	if !hunterV7ClosedBarFeatures(&store.HunterConfig{V7DataTiming: &store.V7DataTimingConfig{FeatureBars: store.V7FeatureBarsClosed}}) {
		t.Fatalf("closed must be selectable")
	}
}
