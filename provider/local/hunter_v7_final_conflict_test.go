package local

import (
	"reflect"
	"testing"
)

func finalCfg() V7FinalConflictConfig {
	return V7FinalConflictConfig{
		Enabled: true, MinAIPriority: 60, TieWinner: V7DirShort, ContextNonVoting: true,
		ContextSetups: map[string]bool{"funding_reversal": true, "pre_distribution_watch": true},
	}
}

func nat(sym, setup string, dir V7Direction, pri float64) V7SignalOutput {
	return V7SignalOutput{Symbol: sym, SetupType: V7SetupType(setup), Direction: dir, AIPriority: pri, Status: V7StatusCandidate}
}

func t2(sym, setup string, dir V7Direction, pri float64) V7SignalOutput {
	s := nat(sym, setup, dir, pri)
	s.Stack, s.Status = V7StackT2, V7StatusWaitConfirm
	return s
}

func dirsBySymbol(out []V7SignalOutput) map[string]map[V7Direction]bool {
	m := map[string]map[V7Direction]bool{}
	for _, s := range out {
		if s.Status == V7StatusConflictWatch {
			continue // non-directional
		}
		if m[s.Symbol] == nil {
			m[s.Symbol] = map[V7Direction]bool{}
		}
		m[s.Symbol][s.Direction] = true
	}
	return m
}

func assertOneDirection(t *testing.T, out []V7SignalOutput) {
	t.Helper()
	for sym, d := range dirsBySymbol(out) {
		if d[V7DirLong] && d[V7DirShort] {
			t.Fatalf("%s still has both directions", sym)
		}
	}
}

// Measured leader #1 (funding_reversal, 23 co-fires): CONTEXT does not vote,
// so the T2 signal survives and the opposite CONTEXT signal is dropped.
func TestFinalConflictContextDoesNotVeto(t *testing.T) {
	in := []V7SignalOutput{nat("AUSDT", "funding_reversal", V7DirShort, 90), t2("AUSDT", "momentum_continuation_long", V7DirLong, 85)}
	out, res := ResolveV7FinalDirectionConflicts(in, finalCfg())
	if len(out) != 1 || out[0].SetupType != "momentum_continuation_long" {
		t.Fatalf("out = %+v", out)
	}
	if len(res) != 1 || res[0].Outcome != "context_dropped" || res[0].Dropped[0] != "native:funding_reversal:SHORT" {
		t.Fatalf("res = %+v", res)
	}
}

// With CONTEXT voting switched on (config), funding_reversal votes like any
// other signal and both strong sides become conflict_watch.
func TestFinalConflictContextVotesWhenConfigured(t *testing.T) {
	cfg := finalCfg()
	cfg.ContextNonVoting = false
	in := []V7SignalOutput{nat("AUSDT", "funding_reversal", V7DirShort, 90), t2("AUSDT", "momentum_continuation_long", V7DirLong, 85)}
	out, res := ResolveV7FinalDirectionConflicts(in, cfg)
	if len(out) != 1 || out[0].Status != V7StatusConflictWatch || res[0].Outcome != "conflict_watch" {
		t.Fatalf("out=%+v res=%+v", out, res)
	}
}

// Measured leader #2 (distribution_short, 21 co-fires): both strong ->
// one conflict_watch row (native rule), neither side tradable.
func TestFinalConflictBothStrongBecomesConflictWatch(t *testing.T) {
	in := []V7SignalOutput{nat("BUSDT", "distribution_short", V7DirShort, 65), t2("BUSDT", "ignition_early_long", V7DirLong, 76)}
	out, res := ResolveV7FinalDirectionConflicts(in, finalCfg())
	if len(out) != 1 || out[0].Status != V7StatusConflictWatch || out[0].Symbol != "BUSDT" {
		t.Fatalf("out = %+v", out)
	}
	if !reflect.DeepEqual(out[0].ReasonCodes, []string{"directional_conflict"}) {
		t.Fatalf("conflict row must be the native resolver's row, got %+v", out[0])
	}
	if res[0].Outcome != "conflict_watch" || len(res[0].Dropped) != 2 {
		t.Fatalf("res = %+v", res)
	}
}

// One side weak: the higher best AIPriority side is kept.
func TestFinalConflictWeakSideLoses(t *testing.T) {
	in := []V7SignalOutput{nat("CUSDT", "distribution_short", V7DirShort, 55), t2("CUSDT", "ignition_early_long", V7DirLong, 76)}
	out, res := ResolveV7FinalDirectionConflicts(in, finalCfg())
	if len(out) != 1 || out[0].Direction != V7DirLong || res[0].Outcome != "kept_long" {
		t.Fatalf("out=%+v res=%+v", out, res)
	}
	// T2 weak side loses too (symmetric rule).
	in = []V7SignalOutput{nat("CUSDT", "distribution_short", V7DirShort, 70), t2("CUSDT", "ignition_early_long", V7DirLong, 50)}
	out, res = ResolveV7FinalDirectionConflicts(in, finalCfg())
	if len(out) != 1 || out[0].Direction != V7DirShort || res[0].Outcome != "kept_short" {
		t.Fatalf("out=%+v res=%+v", out, res)
	}
}

// Exact tie below the threshold: the configured tie winner wins.
func TestFinalConflictTieWinner(t *testing.T) {
	in := []V7SignalOutput{nat("DUSDT", "distribution_short", V7DirShort, 50), t2("DUSDT", "ignition_early_long", V7DirLong, 50)}
	out, _ := ResolveV7FinalDirectionConflicts(in, finalCfg())
	if len(out) != 1 || out[0].Direction != V7DirShort {
		t.Fatalf("default tie winner SHORT, out=%+v", out)
	}
	cfg := finalCfg()
	cfg.TieWinner = V7DirLong
	out, _ = ResolveV7FinalDirectionConflicts(in, cfg)
	if len(out) != 1 || out[0].Direction != V7DirLong {
		t.Fatalf("configured tie winner LONG, out=%+v", out)
	}
}

// A router conflict_watch row absorbs the T2 signal.
func TestFinalConflictRouterConflictRowAbsorbsT2(t *testing.T) {
	row := nat("EUSDT", "leader_momentum_long", V7DirLong, 70)
	row.Status = V7StatusConflictWatch
	in := []V7SignalOutput{row, t2("EUSDT", "momentum_continuation_short", V7DirShort, 88)}
	out, res := ResolveV7FinalDirectionConflicts(in, finalCfg())
	if len(out) != 1 || out[0].Status != V7StatusConflictWatch || res[0].Outcome != "absorbed_into_conflict_watch" {
		t.Fatalf("out=%+v res=%+v", out, res)
	}
}

// Scope: native-only symbols are untouched (the router already resolved
// them), same-direction pairs are untouched, disabled pass is a no-op.
func TestFinalConflictScope(t *testing.T) {
	in := []V7SignalOutput{
		nat("NATUSDT", "leader_momentum_long", V7DirLong, 70),
		nat("NATUSDT", "pre_distribution_watch", V7DirShort, 40), // native-only: kept as before
		nat("SAMEUSDT", "leader_momentum_long", V7DirLong, 70),
		t2("SAMEUSDT", "momentum_continuation_long", V7DirLong, 85), // same direction: kept
	}
	out, res := ResolveV7FinalDirectionConflicts(in, finalCfg())
	if !reflect.DeepEqual(out, in) || len(res) != 0 {
		t.Fatalf("out=%+v res=%+v", out, res)
	}
	cfg := finalCfg()
	cfg.Enabled = false
	conflicting := []V7SignalOutput{nat("X", "distribution_short", V7DirShort, 65), t2("X", "ignition_early_long", V7DirLong, 76)}
	if out, _ := ResolveV7FinalDirectionConflicts(conflicting, cfg); !reflect.DeepEqual(out, conflicting) {
		t.Fatalf("disabled pass must be a no-op")
	}
}

// Determinism and the one-direction guarantee over a mixed batch; order of
// the surviving signals is preserved.
func TestFinalConflictDeterministicGuarantee(t *testing.T) {
	in := []V7SignalOutput{
		nat("AUSDT", "funding_reversal", V7DirShort, 90),
		t2("AUSDT", "momentum_continuation_long", V7DirLong, 85),
		nat("BUSDT", "distribution_short", V7DirShort, 65),
		nat("KEEPUSDT", "leader_momentum_long", V7DirLong, 70),
		t2("BUSDT", "ignition_early_long", V7DirLong, 76),
		t2("CUSDT", "ignition_early_long", V7DirLong, 76),
		nat("CUSDT", "distribution_short", V7DirShort, 55),
		nat("CUSDT", "funding_reversal", V7DirShort, 80),
	}
	first, res1 := ResolveV7FinalDirectionConflicts(in, finalCfg())
	for i := 0; i < 20; i++ {
		again, res2 := ResolveV7FinalDirectionConflicts(in, finalCfg())
		if !reflect.DeepEqual(first, again) || !reflect.DeepEqual(res1, res2) {
			t.Fatalf("non-deterministic result")
		}
	}
	assertOneDirection(t, first)
	var syms []string
	for _, s := range first {
		syms = append(syms, s.Symbol)
	}
	if !reflect.DeepEqual(syms, []string{"AUSDT", "BUSDT", "KEEPUSDT", "CUSDT"}) {
		t.Fatalf("order = %v", syms)
	}
}

// The native resolver is unchanged by the shared conflict-row builder.
func TestNativeResolverUnchanged(t *testing.T) {
	in := []V7SignalOutput{nat("ZUSDT", "leader_momentum_long", V7DirLong, 70), nat("ZUSDT", "distribution_short", V7DirShort, 65)}
	out := ResolveV7Conflicts(in)
	if len(out) != 1 || out[0].Status != V7StatusConflictWatch || out[0].AIPriority != 67.5 || out[0].SetupType != "leader_momentum_long" {
		t.Fatalf("native conflict row changed: %+v", out)
	}
}
