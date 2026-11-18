package kernel

import (
	"encoding/json"
	"strings"
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// stopCoin: price 100, stop at 100-riskPct, one target at 100+rewardPct.
func stopCoin(stack string, riskPct, rewardPct float64) CandidateCoin {
	c := rrCoin(stack, rewardPct)
	c.V7Invalidation = local.V7InvalidationRule{Price: 100 - riskPct}
	return c
}

// U4/P2b: the assembly RR check simulates the stop widening with the T2
// floor for T2 candidates and the strategy floor for native ones.
func TestBackendCappedRRUsesT2StopFloorForT2Only(t *testing.T) {
	g := defaultGeometry() // native MinSLMovePct 2.0, T2MinSLMovePct unset -> 0.5
	// Risk 1%, reward 1.8% (T2 TP1 at 1.8R): RR 1.8 at the 0.5 floor, but
	// 1.8/2.1 = 0.86 once widened to the native 2.0 floor.
	if r := hunterV7BackendCappedRRWaitReason(stopCoin(local.V7StackT2, 1.0, 1.8), g); r != "" {
		t.Fatalf("T2 1%% stop must keep its own distance under the 0.5%% floor, got %q", r)
	}
	if r := hunterV7BackendCappedRRWaitReason(stopCoin("", 1.0, 1.8), g); r != "backend_rr_infeasible" {
		t.Fatalf("native 1%% stop must still be widened to the 2%% floor, got %q", r)
	}
	// Risk 0.3% is widened to 0.5+0.10: reward 0.66 -> RR 1.1 < 1.2.
	if r := hunterV7BackendCappedRRWaitReason(stopCoin(local.V7StackT2, 0.3, 0.66), g); r != "backend_rr_infeasible" {
		t.Fatalf("T2 stop below the floor must be simulated at floor+buffer, got %q", r)
	}
	if r := hunterV7BackendCappedRRWaitReason(stopCoin(local.V7StackT2, 0.3, 0.78), g); r != "" {
		t.Fatalf("T2 0.3%% stop with reward 0.78 (RR 1.3 at 0.6) must pass, got %q", r)
	}
	// A configured T2 floor is used as is.
	g.T2MinSLMovePct = 1.5
	// 1% is widened to 1.6%: 1.8 / 1.6 = 1.125 < 1.2.
	if r := hunterV7BackendCappedRRWaitReason(stopCoin(local.V7StackT2, 1.0, 1.8), g); r != "backend_rr_infeasible" {
		t.Fatalf("configured T2 floor 1.5 must widen a 1%% stop and fail RR 1.2, got %q", r)
	}
}

func TestEngineGeometryCarriesT2StopFloorAndReviewableStates(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.Hunter = &store.HunterConfig{}
	g := NewStrategyEngine(cfg).hunterV7ExecutionGeometry()
	if g.T2MinSLMovePct != 0.5 || store.DefaultT2MinStopLossPriceMovePct != 0.5 {
		t.Fatalf("default T2MinSLMovePct = %.2f, want 0.5", g.T2MinSLMovePct)
	}
	if g.MinSLMovePct != 2.0 {
		t.Fatalf("native floor must stay 2.0, got %.2f", g.MinSLMovePct)
	}
	if got := g.T2ReviewableStates[store.V7ClassTrigger]; len(got) != 1 || got[0] != "TRIGGERED" {
		t.Fatalf("default TRIGGER states = %v", got)
	}
	cfg.RiskControl.T2MinStopLossPriceMovePct = 0.8
	cfg.CoinSource.Hunter.V7T2Lifecycle = &store.V7T2LifecycleConfig{ReviewableStates: map[string][]string{store.V7ClassTrigger: {"ARMED", "TRIGGERED"}}}
	g = NewStrategyEngine(cfg).hunterV7ExecutionGeometry()
	if g.T2MinSLMovePct != 0.8 {
		t.Fatalf("configured T2MinSLMovePct = %.2f, want 0.8", g.T2MinSLMovePct)
	}
	if got := g.T2ReviewableStates[store.V7ClassTrigger]; len(got) != 2 {
		t.Fatalf("override TRIGGER states = %v", got)
	}
	if got := g.T2ReviewableStates[store.V7ClassImpulse]; len(got) != 2 {
		t.Fatalf("classes without override keep defaults, IMPULSE = %v", got)
	}
}

func TestAssemblyStampsRequiredMinStopOnT2Only(t *testing.T) {
	sigs := []local.V7SignalOutput{
		{SignalID: "t2-1", Symbol: "AUSDT", Direction: local.V7DirLong, SetupType: "ignition_early_long", Status: local.V7StatusWaitConfirm, Stack: local.V7StackT2},
		{SignalID: "n-1", Symbol: "BUSDT", Direction: local.V7DirLong, SetupType: "leader_momentum_long", Status: local.V7StatusCandidate},
	}
	c := AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry())
	if c[0].V7RequiredMinStop != 0.5 || c[1].V7RequiredMinStop != 0 {
		t.Fatalf("min stop stamps = %.2f / %.2f, want 0.5 / 0", c[0].V7RequiredMinStop, c[1].V7RequiredMinStop)
	}
}

func TestHydrateSetsHunterV7MinStopPctFromCandidateOnly(t *testing.T) {
	var ds []Decision
	if err := json.Unmarshal([]byte(`[{"symbol":"AUSDT","action":"open_long","selected_hunter_v7_signal_id":"t2-1","HunterV7MinStopPct":0.01},
		{"symbol":"CUSDT","action":"open_long","selected_hunter_v7_signal_id":"missing"}]`), &ds); err != nil {
		t.Fatal(err)
	}
	if ds[0].HunterV7MinStopPct != 0 {
		t.Fatalf("model JSON must not set HunterV7MinStopPct")
	}
	ds[1].HunterV7MinStopPct = 9
	hydrateHunterV7DecisionExecutionPlans(ds, []CandidateCoin{{Symbol: "AUSDT", V7SignalID: "t2-1", V7RequiredMinStop: 0.5}})
	if ds[0].HunterV7MinStopPct != 0.5 || ds[1].HunterV7MinStopPct != 0 {
		t.Fatalf("hydrated = %.2f / %.2f, want 0.5 / 0", ds[0].HunterV7MinStopPct, ds[1].HunterV7MinStopPct)
	}
}

func TestExecutionContractMinStopOnlyForT2(t *testing.T) {
	if c := buildHunterV7ExecutionContract(CandidateCoin{V7ExecutionTier: "REVIEWABLE", V7RequiredMinStop: 0.5}); c.MinStopPct != 0.5 {
		t.Fatalf("T2 contract min_stop_pct = %.2f", c.MinStopPct)
	}
	raw, _ := json.Marshal(buildHunterV7ExecutionContract(CandidateCoin{V7ExecutionTier: "REVIEWABLE"}))
	if strings.Contains(string(raw), "min_stop_pct") {
		t.Fatalf("native contract must omit min_stop_pct: %s", raw)
	}
}

func TestPreflightPromptT2StopNote(t *testing.T) {
	mk := func(t2 bool) string {
		cfg := &store.StrategyConfig{}
		cfg.CoinSource.SourceType = "hunter_v7"
		cfg.CoinSource.Hunter = &store.HunterConfig{V7T2Enabled: t2}
		var sb strings.Builder
		NewStrategyEngine(cfg).writeHunterV7ExecutionPreflightPrompt(&sb)
		return sb.String()
	}
	if strings.Contains(mk(false), "min_stop_pct") {
		t.Fatalf("native prompt must not carry the T2 stop note")
	}
	if p := mk(true); !strings.Contains(p, "min_stop_pct") || !strings.Contains(p, "0.50%") {
		t.Fatalf("T2 prompt must carry the T2 stop note")
	}
}

// t2Coin is a T2 candidate that passes every shared check (wide RR).
func t2Coin(setup, state string, tags ...string) CandidateCoin {
	c := stopCoin(local.V7StackT2, 1.0, 3.0)
	c.V7SetupType = setup
	c.V7LifecycleState = state
	c.V7Status = "candidate"
	c.V7RiskTags = tags
	if e, ok := store.DefaultV7SignalClassRegistry()[setup]; ok {
		c.V7SignalClass, c.V7ClassAdmission = e.Class, e.Admission
	}
	return c
}

// U4/P2c: the T2 tier rule is keyed on class and lifecycle state and never
// falls through to needs_confirmation.
func TestT2TierRuleByClassAndState(t *testing.T) {
	g := defaultGeometry()
	cases := []struct {
		name         string
		coin         CandidateCoin
		tier, reason string
	}{
		{"impulse armed", t2Coin("momentum_continuation_long", "ARMED"), "REVIEWABLE", "t2_impulse_armed"},
		{"impulse triggered", t2Coin("momentum_continuation_short", "TRIGGERED"), "REVIEWABLE", "t2_impulse_triggered"},
		{"impulse watch", t2Coin("momentum_continuation_watch_short", "WATCH"), "WATCH", "t2_awaiting_trigger"},
		{"trigger watch", t2Coin("ignition_early_long", "WATCH"), "WATCH", "t2_awaiting_trigger"},
		{"trigger armed", t2Coin("bb_oi_accumulation_long", "ARMED"), "WATCH", "t2_awaiting_trigger"},
		{"trigger triggered", t2Coin("ignition_early_long", "TRIGGERED"), "REVIEWABLE", "t2_trigger_triggered"},
		{"reversal armed", t2Coin("trend_pullback_short", "ARMED"), "WATCH", "t2_awaiting_trigger"},
		{"reversal triggered", t2Coin("trend_pullback_short", "TRIGGERED"), "REVIEWABLE", "t2_reversal_triggered"},
		{"observe-only triggered", t2Coin("reclaim_breakout_long", "TRIGGERED", "t2_observe_only"), "WATCH", "t2_observe_only"},
		{"shadow-only triggered", t2Coin("ignition_early_short", "TRIGGERED"), "WATCH", "class_shadow_only"},
		{"unclassified", t2Coin("no_such_t2_setup", "TRIGGERED"), "WATCH", "t2_unclassified"},
	}
	for _, tc := range cases {
		tier, reason := classifyHunterV7CandidateTierWithGeometry(tc.coin, g)
		if tier != tc.tier || reason != tc.reason {
			t.Errorf("%s: %s/%s, want %s/%s", tc.name, tier, reason, tc.tier, tc.reason)
		}
	}
	// CONTEXT has no reviewable state.
	ctx := t2Coin("ignition_early_long", "TRIGGERED")
	ctx.V7SignalClass = store.V7ClassContext
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(ctx, g); tier != "WATCH" || reason != "t2_class_not_reviewable" {
		t.Errorf("context: %s/%s", tier, reason)
	}
	// Config override: TRIGGER reviewable at ARMED.
	g.T2ReviewableStates = store.EffectiveV7T2ReviewableStates(&store.V7T2LifecycleConfig{ReviewableStates: map[string][]string{store.V7ClassTrigger: {"ARMED", "TRIGGERED"}}})
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(t2Coin("bb_oi_accumulation_long", "ARMED"), g); tier != "REVIEWABLE" || reason != "t2_trigger_armed" {
		t.Errorf("override: %s/%s", tier, reason)
	}
}

// Shared checks still win over the T2 rule.
func TestT2TierRuleAfterSharedChecks(t *testing.T) {
	g := defaultGeometry()
	rr := t2Coin("ignition_early_long", "TRIGGERED")
	rr.V7Targets = []local.V7Target{{Price: 101.0}} // RR 1.0 < 1.2
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(rr, g); tier != "WATCH" || reason != "backend_rr_infeasible" {
		t.Errorf("rr: %s/%s", tier, reason)
	}
	cw := t2Coin("ignition_early_long", "TRIGGERED")
	cw.V7Status = "conflict_watch"
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(cw, g); tier != "WATCH" || reason != "conflict_watch" {
		t.Errorf("conflict: %s/%s", tier, reason)
	}
	f := t2Coin("ignition_early_long", "TRIGGERED")
	f.V7Status = "filtered"
	if tier, _ := classifyHunterV7CandidateTierWithGeometry(f, g); tier != "REJECTED" {
		t.Errorf("filtered: %s", tier)
	}
}

// OD-11: a coin that is EXECUTABLE as a native candidate is at most
// REVIEWABLE on the T2 stack.
func TestT2TierCappedAtReviewable(t *testing.T) {
	g := defaultGeometry()
	ready := hunterV7PromptReadyLeaderCandidate("LEADUSDT", 100)
	if tier, _ := classifyHunterV7CandidateTierWithGeometry(ready, g); tier != "EXECUTABLE" {
		t.Skipf("fixture is %s, not EXECUTABLE", tier)
	}
	ready.V7Stack = local.V7StackT2
	ready.V7SignalClass = store.V7ClassImpulse
	ready.V7LifecycleState = "ARMED"
	if tier, reason := classifyHunterV7CandidateTierWithGeometry(ready, g); tier != "REVIEWABLE" {
		t.Fatalf("T2 must cap at REVIEWABLE, got %s/%s", tier, reason)
	}
}
