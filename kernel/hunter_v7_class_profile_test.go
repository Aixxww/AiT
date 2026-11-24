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

// setupSig is a native signal at price 100, stop 98 (2%, at the native
// floor so it is not widened) and TP1 at 100+rewardPct.
func setupSig(id, setup string, rewardPct float64) local.V7SignalOutput {
	return local.V7SignalOutput{
		SignalID: id, Symbol: id + "USDT", Direction: local.V7DirLong, SetupType: local.V7SetupType(setup), Status: local.V7StatusCandidate,
		PriceCtx: &local.V7PriceContext{Last: 100}, EntryZone: local.V7PriceZone{Lower: 100, Upper: 100},
		Invalidation: local.V7InvalidationRule{Price: 98}, Targets: []local.V7Target{{Price: 100 + rewardPct}}, TP1Price: 100 + rewardPct,
	}
}

// Owner decision on U6: a per-setup override reaches the validator and the
// executor, not only the kernel RR check. intraday_scalp_long at RR 1.1
// passes the kernel check (cap 1.0) and is stamped 1.0, so the validator and
// (through hydration) the executor use 1.0 instead of the strategy 1.5.
func TestSetupOverrideReachesValidatorAndHydration(t *testing.T) {
	g := profiledGeometry(nil) // strategy RR 1.5, stop 2.0, TP cap 4.0
	sigs := []local.V7SignalOutput{
		setupSig("SCALP", "intraday_scalp_long", 2.2),       // RR 1.1
		setupSig("DISP", "displacement_momentum_long", 2.8), // RR 1.4
		setupSig("LEAD", "leader_momentum_long", 2.2),       // RR 1.1, no override
	}
	c := AssembleHunterV7CandidateCoins(sigs, "BOTH", g)
	if c[0].V7TierReason == "backend_rr_infeasible" || c[0].V7RequiredRR != 1.0 || c[0].V7RequiredMaxTP != 0 {
		t.Fatalf("scalp = %s/%s rr=%v tp=%v", c[0].V7ExecutionTier, c[0].V7TierReason, c[0].V7RequiredRR, c[0].V7RequiredMaxTP)
	}
	if c[1].V7TierReason == "backend_rr_infeasible" || c[1].V7RequiredRR != 1.35 || c[1].V7RequiredMaxTP != 12 {
		t.Fatalf("displacement = %s rr=%v tp=%v", c[1].V7TierReason, c[1].V7RequiredRR, c[1].V7RequiredMaxTP)
	}
	if c[2].V7TierReason != "backend_rr_infeasible" || c[2].V7RequiredRR != 0 {
		t.Fatalf("no-override setup = %s rr=%v", c[2].V7TierReason, c[2].V7RequiredRR)
	}

	floor := hunterV7DecisionRRFloor(1.5, c)
	priceMap := map[string]float64{"SCALPUSDT": 100}
	mk := func(id string) *Decision {
		return &Decision{Symbol: "SCALPUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 100, StopLoss: 98, TakeProfit: 102.2,
			Confidence: 80, Reasoning: "t", SelectedHunterV7SignalID: id}
	}
	if err := validateDecision(mk("SCALP"), 10000, 20, 10, 10.0, 1.5, nil, priceMap, floor, 0); err != nil {
		t.Fatalf("validator must accept RR 1.1 for intraday_scalp_long: %v", err)
	}
	if err := validateDecision(mk(""), 10000, 20, 10, 10.0, 1.5, nil, priceMap, floor, 0); err == nil {
		t.Fatalf("without the selected signal the strategy floor 1.5 applies")
	}
	ds := []Decision{*mk("SCALP"), *mk("DISP")}
	ds[1].Symbol = "DISPUSDT"
	hydrateHunterV7DecisionExecutionPlans(ds, c)
	if ds[0].HunterV7MinRR != 1.0 || ds[0].HunterV7MaxTPPct != 0 || ds[1].HunterV7MinRR != 1.35 || ds[1].HunterV7MaxTPPct != 12 {
		t.Fatalf("hydrated = %v/%v, %v/%v", ds[0].HunterV7MinRR, ds[0].HunterV7MaxTPPct, ds[1].HunterV7MinRR, ds[1].HunterV7MaxTPPct)
	}
	// Removing the override in config removes the stamp: one source of truth.
	g.SetupOverrides = store.RiskControlConfig{SetupGeometryOverrides: map[string]store.V7SetupGeometryOverride{"intraday_scalp_long": {}}}.EffectiveV7SetupGeometryOverrides()
	c = AssembleHunterV7CandidateCoins(sigs[:1], "BOTH", g)
	if c[0].V7TierReason != "backend_rr_infeasible" || c[0].V7RequiredRR != 0 {
		t.Fatalf("override removed = %s rr=%v", c[0].V7TierReason, c[0].V7RequiredRR)
	}
}
