package kernel

import (
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

func TestAssemblyStampsSignalClass(t *testing.T) {
	sigs := []local.V7SignalOutput{
		{SignalID: "a", Symbol: "AUSDT", Direction: local.V7DirShort, SetupType: "ignition_early_short", Status: local.V7StatusWaitConfirm, Stack: local.V7StackT2},
		{SignalID: "b", Symbol: "BUSDT", Direction: local.V7DirLong, SetupType: "leader_momentum_long", Status: local.V7StatusCandidate},
		{SignalID: "c", Symbol: "CUSDT", Direction: local.V7DirLong, SetupType: "mms_trend_ride_long", Status: local.V7StatusCandidate},
	}
	c := AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry())
	if c[0].V7SignalClass != store.V7ClassTrigger || c[0].V7ClassAdmission != store.V7AdmissionShadowOnly {
		t.Fatalf("ignition_early_short = %q/%q", c[0].V7SignalClass, c[0].V7ClassAdmission)
	}
	if c[1].V7SignalClass != store.V7ClassImpulse || c[1].V7ClassAdmission != "" {
		t.Fatalf("leader_momentum_long = %q/%q", c[1].V7SignalClass, c[1].V7ClassAdmission)
	}
	if c[2].V7SignalClass != "" {
		t.Fatalf("unassigned setup must carry no class, got %q", c[2].V7SignalClass)
	}
}

// The shadow_only cap turns a reviewable/executable tier into WATCH and
// leaves everything else alone.
func TestShadowOnlyAdmissionCapsTier(t *testing.T) {
	g := defaultGeometry()
	ready := hunterV7PromptReadyLeaderCandidate("LEADUSDT", 100)
	baseTier, baseReason := classifyHunterV7CandidateTierRules(ready, g)
	if baseTier != "EXECUTABLE" && baseTier != "REVIEWABLE" {
		t.Fatalf("fixture must be open-reviewable, got %s/%s", baseTier, baseReason)
	}
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(ready, g); tier != baseTier || reason != baseReason {
		t.Fatalf("tradable admission changed tier: %s/%s -> %s/%s", baseTier, baseReason, tier, reason)
	}
	ready.V7ClassAdmission = store.V7AdmissionShadowOnly
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(ready, g); tier != "WATCH" || reason != "class_shadow_only" {
		t.Fatalf("shadow_only must cap at WATCH, got %s/%s", tier, reason)
	}
	// A WATCH or REJECTED tier keeps its own reason.
	watch := CandidateCoin{Symbol: "W", Direction: "LONG", V7SetupType: "ignition_early_short", V7Status: "filtered", V7ClassAdmission: store.V7AdmissionShadowOnly}
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(watch, g); tier != "REJECTED" || reason != "status_filtered" {
		t.Fatalf("non-open tiers must be untouched, got %s/%s", tier, reason)
	}
}

// The default shadow_only T2 families never rise above WATCH. Since U4 a
// classed T2 candidate in a reviewable lifecycle state reaches REVIEWABLE,
// so the fixture uses TRIGGERED (reviewable for every class) and expects the
// U6a cap to turn it into WATCH/class_shadow_only. (Before U4 this test
// compared against an empty registry; an empty registry now means
// t2_unclassified.)
func TestDefaultShadowFamiliesStayWatch(t *testing.T) {
	var sigs []local.V7SignalOutput
	for i, setup := range []string{"ignition_early_short", "momentum_continuation_watch_long", "confirmed_breakout_long", "confirmed_breakout_short"} {
		dir := local.V7DirLong
		if setup == "ignition_early_short" || setup == "confirmed_breakout_short" {
			dir = local.V7DirShort
		}
		sigs = append(sigs, local.V7SignalOutput{SignalID: setup, Symbol: string(rune('A'+i)) + "USDT", Direction: dir,
			SetupType: local.V7SetupType(setup), Status: local.V7StatusCandidate, Stack: local.V7StackT2, LifecycleState: "TRIGGERED", LiquidityScore: 80})
	}
	for _, c := range AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry()) {
		if c.V7ExecutionTier != "WATCH" || c.V7TierReason != "class_shadow_only" {
			t.Errorf("%s: %s/%s, want WATCH/class_shadow_only", c.V7SetupType, c.V7ExecutionTier, c.V7TierReason)
		}
	}
}

func TestEngineGeometryCarriesClassRegistry(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.Hunter = &store.HunterConfig{V7SignalClasses: map[string]store.V7SignalClassEntry{
		"leader_momentum_long": {Class: store.V7ClassImpulse, Admission: store.V7AdmissionShadowOnly},
	}}
	reg := NewStrategyEngine(cfg).hunterV7ExecutionGeometry().Classes
	if !reg["leader_momentum_long"].ShadowOnly() || !reg["ignition_early_short"].ShadowOnly() {
		t.Fatalf("engine registry must merge overrides over defaults")
	}
}
