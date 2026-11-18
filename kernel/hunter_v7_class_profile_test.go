package kernel

import (
	"encoding/json"
	"strings"
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

func profiledGeometry(p map[string]store.V7ClassRiskProfile) HunterV7ExecutionGeometry {
	g := defaultGeometry()
	g.T2MinSLMovePct = 0.5
	g.ClassProfiles = p
	return g
}

// Default profiles inherit: T2 gets its floors, native the strategy values.
func TestCandidateRiskInheritsStackValues(t *testing.T) {
	g := profiledGeometry(nil)
	rr, sl, tp := hunterV7CandidateRisk(CandidateCoin{V7Stack: local.V7StackT2, V7SignalClass: store.V7ClassTrigger}, g)
	if rr != 1.2 || sl != 0.5 || tp != g.MaxTPMovePct {
		t.Fatalf("t2 = %v/%v/%v", rr, sl, tp)
	}
	rr, sl, tp = hunterV7CandidateRisk(CandidateCoin{V7SignalClass: store.V7ClassImpulse}, g)
	if rr != g.MinRR || sl != g.MinSLMovePct || tp != g.MaxTPMovePct {
		t.Fatalf("native = %v/%v/%v", rr, sl, tp)
	}
}

func TestCandidateRiskUsesClassProfile(t *testing.T) {
	g := profiledGeometry(map[string]store.V7ClassRiskProfile{
		store.V7ClassTrigger: {T2: store.V7StackRiskProfile{MinRiskRewardRatio: 1.4, MinStopLossPriceMovePct: 0.8}},
		store.V7ClassImpulse: {Native: store.V7StackRiskProfile{MaxTakeProfitPriceMovePct: 1.0}},
	})
	rr, sl, _ := hunterV7CandidateRisk(CandidateCoin{V7Stack: local.V7StackT2, V7SignalClass: store.V7ClassTrigger}, g)
	if rr != 1.4 || sl != 0.8 {
		t.Fatalf("t2 trigger = %v/%v", rr, sl)
	}
	if rr, _, _ := hunterV7CandidateRisk(CandidateCoin{V7SignalClass: store.V7ClassTrigger}, g); rr != g.MinRR {
		t.Fatalf("native trigger must keep the strategy RR, got %v", rr)
	}
	// TP cap 1.0 is raised to the feasible minimum (2.0+0.5)*1.5+0.25 = 4.0.
	if _, _, tp := hunterV7CandidateRisk(CandidateCoin{V7SignalClass: store.V7ClassImpulse}, g); tp != 4.0 {
		t.Fatalf("feasible TP cap = %v, want 4.0", tp)
	}
}

// The backend RR check follows the class profile (sensitivity check for
// the golden replay: a profile that differs must change the verdict).
func TestBackendCappedRRUsesClassProfile(t *testing.T) {
	coin := stopCoin(local.V7StackT2, 1.0, 1.3) // RR 1.3
	coin.V7SignalClass = store.V7ClassTrigger
	g := profiledGeometry(nil)
	if r := hunterV7BackendCappedRRWaitReason(coin, g); r != "" {
		t.Fatalf("default profile: RR 1.3 passes 1.2, got %q", r)
	}
	g = profiledGeometry(map[string]store.V7ClassRiskProfile{store.V7ClassTrigger: {T2: store.V7StackRiskProfile{MinRiskRewardRatio: 1.4}}})
	if r := hunterV7BackendCappedRRWaitReason(coin, g); r != "backend_rr_infeasible" {
		t.Fatalf("TRIGGER/t2 floor 1.4 must fail RR 1.3, got %q", r)
	}
	coin.V7SignalClass = store.V7ClassImpulse
	if r := hunterV7BackendCappedRRWaitReason(coin, g); r != "" {
		t.Fatalf("other classes keep the default, got %q", r)
	}
}

// Assembly stamps T2 floors always (as before) and any value a profile
// changes; nothing else.
func TestAssemblyStampsProfileRiskOnlyWhenItDiffers(t *testing.T) {
	sigs := []local.V7SignalOutput{
		{SignalID: "t", Symbol: "A", Direction: local.V7DirLong, SetupType: "ignition_early_long", Status: local.V7StatusWaitConfirm, Stack: local.V7StackT2},
		{SignalID: "n", Symbol: "B", Direction: local.V7DirLong, SetupType: "leader_momentum_long", Status: local.V7StatusCandidate},
	}
	c := AssembleHunterV7CandidateCoins(sigs, "BOTH", profiledGeometry(nil))
	if c[0].V7RequiredRR != 1.2 || c[0].V7RequiredMinStop != 0.5 || c[0].V7RequiredMaxTP != 0 ||
		c[1].V7RequiredRR != 0 || c[1].V7RequiredMinStop != 0 || c[1].V7RequiredMaxTP != 0 {
		t.Fatalf("default stamps = %+v / %+v", c[0].V7RequiredRR, c[1].V7RequiredRR)
	}
	g := profiledGeometry(map[string]store.V7ClassRiskProfile{
		store.V7ClassImpulse: {Native: store.V7StackRiskProfile{MinRiskRewardRatio: 1.3, MaxTakeProfitPriceMovePct: 6}},
	})
	c = AssembleHunterV7CandidateCoins(sigs, "BOTH", g)
	if c[1].V7RequiredRR != 1.3 || c[1].V7RequiredMinStop != 0 || c[1].V7RequiredMaxTP != 6 {
		t.Fatalf("native profile stamps = %v/%v/%v", c[1].V7RequiredRR, c[1].V7RequiredMinStop, c[1].V7RequiredMaxTP)
	}
	var ds []Decision
	_ = json.Unmarshal([]byte(`[{"symbol":"B","action":"open_long","selected_hunter_v7_signal_id":"n","HunterV7MaxTPPct":99}]`), &ds)
	hydrateHunterV7DecisionExecutionPlans(ds, c)
	if ds[0].HunterV7MaxTPPct != 6 || ds[0].HunterV7MinRR != 1.3 {
		t.Fatalf("hydrated = %v/%v", ds[0].HunterV7MaxTPPct, ds[0].HunterV7MinRR)
	}
	raw, _ := json.Marshal(buildHunterV7ExecutionContract(c[1]))
	if !strings.Contains(string(raw), `"max_tp_pct":6`) {
		t.Fatalf("contract = %s", raw)
	}
	raw, _ = json.Marshal(buildHunterV7ExecutionContract(CandidateCoin{V7ExecutionTier: "REVIEWABLE"}))
	if strings.Contains(string(raw), "max_tp_pct") {
		t.Fatalf("default contract must omit max_tp_pct: %s", raw)
	}
}

// The per-setup relaxations come from config with the old values as
// defaults; a zero entry removes one.
func TestSetupGeometryOverridesFromConfig(t *testing.T) {
	g := profiledGeometry(nil)
	coin := CandidateCoin{V7SetupType: "intraday_scalp_long"}
	if out := hunterV7SetupExecutionGeometry(coin, g); out.MinRR != 1.0 || out.MaxTPMovePct != g.MaxTPMovePct {
		t.Fatalf("default intraday_scalp = %v/%v (cap 4.0 already >= 3)", out.MinRR, out.MaxTPMovePct)
	}
	if out := hunterV7SetupExecutionGeometry(CandidateCoin{V7SetupType: "displacement_momentum_long"}, g); out.MinRR != 1.35 || out.MaxTPMovePct != 12 {
		t.Fatalf("default displacement = %v/%v", out.MinRR, out.MaxTPMovePct)
	}
	g.SetupOverrides = store.RiskControlConfig{SetupGeometryOverrides: map[string]store.V7SetupGeometryOverride{"intraday_scalp_long": {}}}.EffectiveV7SetupGeometryOverrides()
	if out := hunterV7SetupExecutionGeometry(coin, g); out.MinRR != g.MinRR {
		t.Fatalf("removed override must keep the strategy RR, got %v", out.MinRR)
	}
}

func TestEngineGeometryCarriesClassProfiles(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.RiskControl.ClassProfiles = map[string]store.V7ClassRiskProfile{store.V7ClassReversal: {Native: store.V7StackRiskProfile{MinRiskRewardRatio: 1.7}}}
	g := NewStrategyEngine(cfg).hunterV7ExecutionGeometry()
	if g.ClassProfiles[store.V7ClassReversal].Native.MinRiskRewardRatio != 1.7 || len(g.ClassProfiles) != 4 || len(g.SetupOverrides) != 3 {
		t.Fatalf("geometry profiles = %+v / overrides %d", g.ClassProfiles, len(g.SetupOverrides))
	}
	if got := HunterV7ExecutionGeometryForConfig(cfg); got.MinRR != g.MinRR || got.T2MinRR != g.T2MinRR {
		t.Fatalf("exported builder differs from the engine")
	}
	if HunterV7ExecutionGeometryForConfig(nil).MinRR != 0 {
		t.Fatalf("nil config")
	}
}
