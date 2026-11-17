package kernel

import (
	"encoding/json"
	"strings"
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// rrCoin: price 100, stop 97.5 (risk 2.5%, above the 2% floor so the stop
// is not widened), one target at 100+reward -> RR = reward/2.5.
func rrCoin(stack string, rewardPct float64) CandidateCoin {
	return CandidateCoin{
		Symbol:         "AUSDT",
		Direction:      "LONG",
		V7SetupType:    "ignition_early_long",
		V7Stack:        stack,
		V7PriceContext: &local.V7PriceContext{Last: 100},
		V7Invalidation: local.V7InvalidationRule{Price: 97.5},
		V7Targets:      []local.V7Target{{Price: 100 + rewardPct}},
	}
}

func defaultGeometry() HunterV7ExecutionGeometry {
	g := HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true) // MinRR 1.5, MaxTP 4.0
	g.T2MinRR = 1.2
	return g
}

// The assembly RR check applies the T2 floor to T2 candidates only.
func TestBackendCappedRRUsesT2FloorForT2Only(t *testing.T) {
	g := defaultGeometry()
	// RR 1.3: between the T2 floor (1.2) and the native floor (1.5).
	if r := hunterV7BackendCappedRRWaitReason(rrCoin(local.V7StackT2, 3.25), g); r != "" {
		t.Fatalf("T2 candidate at RR 1.3 must pass the 1.2 floor, got %q", r)
	}
	if r := hunterV7BackendCappedRRWaitReason(rrCoin("", 3.25), g); r != "backend_rr_infeasible" {
		t.Fatalf("native candidate at RR 1.3 must still fail the 1.5 floor, got %q", r)
	}
	// RR 1.1 fails the T2 floor too.
	if r := hunterV7BackendCappedRRWaitReason(rrCoin(local.V7StackT2, 2.75), g); r != "backend_rr_infeasible" {
		t.Fatalf("T2 candidate at RR 1.1 must fail, got %q", r)
	}
	// Unset T2MinRR falls back to the store default, never to native 1.5.
	g.T2MinRR = 0
	if r := hunterV7BackendCappedRRWaitReason(rrCoin(local.V7StackT2, 3.25), g); r != "" {
		t.Fatalf("unset T2MinRR must use the store default %.2f, got %q", store.DefaultT2MinRiskRewardRatio, r)
	}
}

// The engine geometry carries the configured T2 floor.
func TestEngineGeometryCarriesT2Floor(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.RiskControl.T2MinRiskRewardRatio = 1.3
	if got := NewStrategyEngine(cfg).hunterV7ExecutionGeometry().T2MinRR; got != 1.3 {
		t.Fatalf("T2MinRR = %.2f, want 1.3", got)
	}
	cfg.RiskControl.T2MinRiskRewardRatio = 0
	if got := NewStrategyEngine(cfg).hunterV7ExecutionGeometry().T2MinRR; got != store.DefaultT2MinRiskRewardRatio {
		t.Fatalf("unset T2MinRR = %.2f, want default", got)
	}
}

// Assembly stamps the T2 floor on T2 candidates only.
func TestAssemblyStampsRequiredRROnT2Candidates(t *testing.T) {
	sigs := []local.V7SignalOutput{
		{SignalID: "t2-1", Symbol: "AUSDT", Direction: local.V7DirLong, SetupType: "ignition_early_long", Status: local.V7StatusWaitConfirm, Stack: local.V7StackT2},
		{SignalID: "n-1", Symbol: "BUSDT", Direction: local.V7DirLong, SetupType: "leader_momentum_long", Status: local.V7StatusCandidate},
	}
	cands := AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry())
	if len(cands) != 2 {
		t.Fatalf("got %d candidates", len(cands))
	}
	if cands[0].V7Stack != local.V7StackT2 || cands[0].V7RequiredRR != 1.2 {
		t.Fatalf("T2 candidate stack/RR = %q/%.2f, want t2/1.20", cands[0].V7Stack, cands[0].V7RequiredRR)
	}
	if cands[1].V7Stack != "" || cands[1].V7RequiredRR != 0 {
		t.Fatalf("native candidate must carry no override, got %q/%.2f", cands[1].V7Stack, cands[1].V7RequiredRR)
	}
}

// The validator applies the selected candidate's floor; everything else
// keeps the strategy floor.
func TestValidatorUsesSelectedCandidateRRFloor(t *testing.T) {
	cands := []CandidateCoin{{Symbol: "AUSDT", V7SignalID: "t2-1", V7RequiredRR: 1.2}, {Symbol: "BUSDT", V7SignalID: "n-1"}}
	floor := hunterV7DecisionRRFloor(1.5, cands)
	priceMap := map[string]float64{"AUSDT": 100, "BUSDT": 100}
	mk := func(sym, id string) *Decision {
		// risk 2% (SL 98), reward 2.6% (TP 102.6) -> RR 1.3
		return &Decision{Symbol: sym, Action: "open_long", Leverage: 5, PositionSizeUSD: 100, StopLoss: 98, TakeProfit: 102.6,
			Confidence: 80, Reasoning: "t", SelectedHunterV7SignalID: id}
	}
	if err := validateDecision(mk("AUSDT", "t2-1"), 10000, 20, 10, 10.0, 1.5, nil, priceMap, floor); err != nil {
		t.Fatalf("T2 decision at RR 1.3 must pass the 1.2 floor: %v", err)
	}
	if err := validateDecision(mk("BUSDT", "n-1"), 10000, 20, 10, 10.0, 1.5, nil, priceMap, floor); err == nil {
		t.Fatalf("native decision at RR 1.3 must fail the 1.5 floor")
	}
	if err := validateDecision(mk("AUSDT", ""), 10000, 20, 10, 10.0, 1.5, nil, priceMap, floor); err == nil {
		t.Fatalf("decision without a signal id must keep the strategy floor")
	}
	if err := validateDecision(mk("AUSDT", "unknown"), 10000, 20, 10, 10.0, 1.5, nil, priceMap, floor); err == nil {
		t.Fatalf("unknown signal id must keep the strategy floor")
	}
}

// Hydration copies the floor from the selected candidate, clears any stale
// value, and a model response can never supply it.
func TestHydrateSetsHunterV7MinRRFromCandidateOnly(t *testing.T) {
	var ds []Decision
	if err := json.Unmarshal([]byte(`[{"symbol":"AUSDT","action":"open_long","selected_hunter_v7_signal_id":"t2-1","hunter_v7_min_rr":0.1,"HunterV7MinRR":0.1},
		{"symbol":"CUSDT","action":"open_long","selected_hunter_v7_signal_id":"missing"}]`), &ds); err != nil {
		t.Fatal(err)
	}
	if ds[0].HunterV7MinRR != 0 {
		t.Fatalf("model JSON must not set HunterV7MinRR, got %.2f", ds[0].HunterV7MinRR)
	}
	ds[1].HunterV7MinRR = 9 // stale value must be cleared
	hydrateHunterV7DecisionExecutionPlans(ds, []CandidateCoin{{Symbol: "AUSDT", V7SignalID: "t2-1", V7RequiredRR: 1.2}})
	if ds[0].HunterV7MinRR != 1.2 {
		t.Fatalf("hydrated floor = %.2f, want 1.2", ds[0].HunterV7MinRR)
	}
	if ds[1].HunterV7MinRR != 0 {
		t.Fatalf("unmatched decision must carry no floor, got %.2f", ds[1].HunterV7MinRR)
	}
}

// The prompt contract shows min_rr for T2 candidates only.
func TestExecutionContractMinRROnlyForT2(t *testing.T) {
	t2 := buildHunterV7ExecutionContract(CandidateCoin{V7ExecutionTier: "REVIEWABLE", V7RequiredRR: 1.2})
	if t2.MinRR != 1.2 {
		t.Fatalf("T2 contract min_rr = %.2f", t2.MinRR)
	}
	raw, _ := json.Marshal(buildHunterV7ExecutionContract(CandidateCoin{V7ExecutionTier: "REVIEWABLE"}))
	if strings.Contains(string(raw), "min_rr") {
		t.Fatalf("native contract must omit min_rr: %s", raw)
	}
}

// The prompt note appears only when the Task-2 module is enabled, so native
// prompts are byte-identical to before.
func TestPreflightPromptT2NoteOnlyWhenT2Enabled(t *testing.T) {
	mk := func(t2 bool) string {
		cfg := &store.StrategyConfig{}
		cfg.CoinSource.SourceType = "hunter_v7"
		cfg.CoinSource.Hunter = &store.HunterConfig{V7T2Enabled: t2}
		var sb strings.Builder
		NewStrategyEngine(cfg).writeHunterV7ExecutionPreflightPrompt(&sb)
		return sb.String()
	}
	native, t2 := mk(false), mk(true)
	if strings.Contains(native, "Task-2 candidates") {
		t.Fatalf("native prompt must not carry the T2 note")
	}
	if !strings.Contains(t2, "Task-2 candidates") || !strings.Contains(t2, "1.20") {
		t.Fatalf("T2 prompt must carry the T2 floor note")
	}
	if !strings.HasPrefix(t2, native) {
		t.Fatalf("T2 note must only append to the native preflight text")
	}
}
