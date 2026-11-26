package local

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// #136: LiveReviewable is not hand-picked. A candidate missing only code X
// may be parked REVIEWABLE for later settling only when something can still
// settle X: a shared live predicate the trader refresh recheck re-runs
// (live-verifiable AND refresh-satisfiable), or the trader refresh owning
// the code outright (RefreshOwned).
func TestV7ConfirmLiveReviewableMatchesVerifiers(t *testing.T) {
	for code, spec := range v7ConfirmRegistry {
		want := (V7ConfirmLiveVerifiable(code) && spec.RefreshSatisfiable) || spec.RefreshOwned
		if spec.LiveReviewable != want {
			t.Errorf("%s: LiveReviewable=%v, want %v (live-verifiable=%v refresh-satisfiable=%v refresh-owned=%v)",
				code, spec.LiveReviewable, want, V7ConfirmLiveVerifiable(code), spec.RefreshSatisfiable, spec.RefreshOwned)
		}
	}
}

// Every shared predicate belongs to a registered machine code, and every
// alias points at a shared predicate.
func TestV7SharedConfirmPredicatesRegistered(t *testing.T) {
	for code := range v7SharedConfirmPredicates {
		if !V7ConfirmMachineVerifiable(code) {
			t.Errorf("shared predicate %q is not a registered machine code", code)
		}
	}
	for alias, target := range v7ConfirmPredicateAliases {
		if !V7ConfirmMachineVerifiable(alias) {
			t.Errorf("alias %q is not a registered machine code", alias)
		}
		if _, ok := v7SharedConfirmPredicates[target]; !ok {
			t.Errorf("alias %q points at %q, which has no shared predicate", alias, target)
		}
		if V7ConfirmLiveVerifiable(alias) {
			t.Errorf("alias %q must not be live-verifiable on its own", alias)
		}
	}
}

// The scoring switch must not re-grow its own case for a shared code: that
// is exactly how the two definitions drifted.
func TestV7ScoringSwitchHasNoSharedCodeCase(t *testing.T) {
	af, err := parser.ParseFile(token.NewFileSet(), "hunter_v7_confirmation.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range af.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "evaluateV7KnownConfirmation" {
			continue
		}
		ast.Inspect(fd.Body, func(x ast.Node) bool {
			cc, ok := x.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				if bl, ok := e.(*ast.BasicLit); ok && bl.Kind == token.STRING {
					v, _ := strconv.Unquote(bl.Value)
					if _, shared := v7SharedConfirmPredicates[v]; shared {
						t.Errorf("evaluateV7KnownConfirmation has its own case for shared code %q", v)
					}
					if _, alias := v7ConfirmPredicateAliases[v]; alias {
						t.Errorf("evaluateV7KnownConfirmation has its own case for alias %q", v)
					}
				}
			}
			return true
		})
	}
}

// Missing inputs are unknown, never a pass: a SHORT taker code with no taker
// reading used to pass at scoring time (0 <= 0.48).
func TestV7SharedConfirmUnknownInputsFailClosed(t *testing.T) {
	for _, code := range []string{"taker_flow_confirms_short", "taker_buy_15m_lt_0_48", "taker_buy_15m_lt_0_45", "taker_flow_not_flipping_against_direction"} {
		check, known, ok := V7EvaluateSharedConfirmation(code, V7ConfirmFacts{Direction: V7DirShort, Price: 100})
		if !ok || known || check.Passed {
			t.Errorf("%s with no taker reading: ok=%v known=%v passed=%v, want ok, unknown, not passed", code, ok, known, check.Passed)
		}
	}
	sig := &V7SignalOutput{Direction: V7DirShort, RequiredConfirms: []string{"taker_flow_confirms_short"}}
	summary := EvaluateV7Confirmations(sig, &V7SymbolContext{CurrentPrice: 100}, DefaultV7Config())
	if summary.PassedReview {
		t.Fatalf("scoring passed taker_flow_confirms_short without a taker reading: %+v", summary)
	}
}

// VolumeBurst15m keeps its scoring-time definition after the refactor.
func TestV7VolumeBurstDefinition(t *testing.T) {
	vols := []float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 15}
	if got := V7VolumeBurst(vols, 8); got != 1.5 {
		t.Fatalf("V7VolumeBurst = %v, want 1.5 (last / mean of the prior 8)", got)
	}
	if got := V7VolumeBurst([]float64{10, 12}, 8); got != 0 {
		t.Fatalf("V7VolumeBurst with <3 bars = %v, want 0", got)
	}
}
