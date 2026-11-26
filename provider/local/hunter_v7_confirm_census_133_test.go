package local

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// #133 / naming F-02: the required_confirmations census. Every code a
// module can emit is found from the AST (multi-line slices, helper
// functions, package constants, loops over helper results), and each must
// be either machine-verified (registered, with an evaluator case) or an
// explicit LLM checklist item. A code in neither state fails CI.

// v7RefreshSettledConfirms are machine codes settled by the trader's
// decision-time refresh, not by the provider evaluator.
var v7RefreshSettledConfirms = map[string]bool{"fresh_micro_confirmed": true}

// scanEmittedConfirmCodes returns every string a provider source file puts
// into a RequiredConfirms or NextConfirm field, with its emission sites.
// Strings in conditions (if / switch / case / comparisons) are not
// collected; calls to package functions are followed into their bodies.
func scanEmittedConfirmCodes(t *testing.T) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]*ast.FuncDecl{}
	consts := map[string]string{}
	var parsed []*ast.File
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		parsed = append(parsed, af)
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Body != nil {
					funcs[d.Name.Name] = d
				}
			case *ast.GenDecl:
				if d.Tok != token.CONST && d.Tok != token.VAR {
					continue
				}
				for _, sp := range d.Specs {
					vs := sp.(*ast.ValueSpec)
					for i, n := range vs.Names {
						if i < len(vs.Values) {
							if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
								v, _ := strconv.Unquote(bl.Value)
								consts[n.Name] = v
							}
						}
					}
				}
			}
		}
	}
	out := map[string][]string{}
	add := func(code, where string) {
		if strings.TrimSpace(code) != "" {
			out[code] = append(out[code], where)
		}
	}
	var collect func(n ast.Node, where string, seen map[string]bool)
	collect = func(n ast.Node, where string, seen map[string]bool) {
		ast.Inspect(n, func(x ast.Node) bool {
			switch x := x.(type) {
			case *ast.BinaryExpr:
				return false
			case *ast.IfStmt:
				if x.Init != nil {
					collect(x.Init, where, seen)
				}
				collect(x.Body, where, seen)
				if x.Else != nil {
					collect(x.Else, where, seen)
				}
				return false
			case *ast.SwitchStmt:
				collect(x.Body, where, seen)
				return false
			case *ast.CaseClause:
				for _, st := range x.Body {
					collect(st, where, seen)
				}
				return false
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					v, _ := strconv.Unquote(x.Value)
					add(v, where)
				}
			case *ast.Ident:
				if v, ok := consts[x.Name]; ok {
					add(v, where)
				}
			case *ast.CallExpr:
				if id, ok := x.Fun.(*ast.Ident); ok {
					if fd, ok := funcs[id.Name]; ok && !seen[id.Name] {
						seen[id.Name] = true
						collect(fd.Body, where+">"+id.Name, seen)
					}
				}
			}
			return true
		})
	}
	isField := func(e ast.Expr) bool {
		se, ok := e.(*ast.SelectorExpr)
		return ok && (se.Sel.Name == "RequiredConfirms" || se.Sel.Name == "NextConfirm")
	}
	for _, af := range parsed {
		fname := filepath.Base(fset.Position(af.Pos()).Filename)
		ast.Inspect(af, func(x ast.Node) bool {
			switch x := x.(type) {
			case *ast.AssignStmt:
				for i, l := range x.Lhs {
					if isField(l) && i < len(x.Rhs) {
						collect(x.Rhs[i], fname, map[string]bool{})
					}
				}
			case *ast.KeyValueExpr:
				if k, ok := x.Key.(*ast.Ident); ok && (k.Name == "RequiredConfirms" || k.Name == "NextConfirm") {
					collect(x.Value, fname, map[string]bool{})
				}
			case *ast.RangeStmt:
				found := false
				ast.Inspect(x.Body, func(y ast.Node) bool {
					if as, ok := y.(*ast.AssignStmt); ok {
						for _, l := range as.Lhs {
							if isField(l) {
								found = true
							}
						}
					}
					return true
				})
				if found {
					collect(x.X, fname+"(range)", map[string]bool{})
				}
			}
			return true
		})
	}
	return out
}

// evaluatorConfirmCases returns the codes evaluateV7KnownConfirmation has
// a case for: its own switch cases plus (#136) every code it delegates to a
// shared predicate (v7SharedConfirmPredicates and their scoring aliases).
func evaluatorConfirmCases(t *testing.T) map[string]bool {
	t.Helper()
	af, err := parser.ParseFile(token.NewFileSet(), "hunter_v7_confirmation.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, d := range af.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "evaluateV7KnownConfirmation" {
			continue
		}
		ast.Inspect(fd.Body, func(x ast.Node) bool {
			if cc, ok := x.(*ast.CaseClause); ok {
				for _, e := range cc.List {
					if bl, ok := e.(*ast.BasicLit); ok && bl.Kind == token.STRING {
						v, _ := strconv.Unquote(bl.Value)
						out[v] = true
					}
				}
			}
			return true
		})
	}
	for code := range v7SharedConfirmPredicates {
		out[code] = true
	}
	for code := range v7ConfirmPredicateAliases {
		out[code] = true
	}
	if len(out) < 30 {
		t.Fatalf("found only %d evaluator cases; the scan is broken", len(out))
	}
	return out
}

func TestConfirmCensus133_EveryEmittedCodeIsMachineOrChecklist(t *testing.T) {
	emitted := scanEmittedConfirmCodes(t)
	if len(emitted) < 55 {
		t.Fatalf("census found only %d codes; the scan is broken", len(emitted))
	}
	eval := evaluatorConfirmCases(t)
	var limbo, noEval []string
	machine, checklist := 0, 0
	for code, sites := range emitted {
		switch {
		case !V7ConfirmKnown(code):
			limbo = append(limbo, code+"  ("+strings.Join(sites, ", ")+")")
		case V7ConfirmMachineVerifiable(code):
			machine++
			if !eval[code] && !v7RefreshSettledConfirms[code] {
				noEval = append(noEval, code)
			}
		default:
			checklist++
		}
	}
	sort.Strings(limbo)
	sort.Strings(noEval)
	if len(limbo) > 0 {
		t.Fatalf("%d emitted required_confirmations code(s) are neither machine-verified nor checklist (register them in hunter_v7_vocab.go):\n%s", len(limbo), strings.Join(limbo, "\n"))
	}
	if len(noEval) > 0 {
		t.Fatalf("machine-verified code(s) without an evaluator case: %v", noEval)
	}
	t.Logf("census: %d emitted codes, %d machine-verified, %d LLM checklist", len(emitted), machine, checklist)
}

// The registries agree: every evaluator case is a registered machine code,
// every registered code is catalogued, and the refresh-only set is machine.
func TestConfirmCensus133_RegistriesAligned(t *testing.T) {
	for code := range evaluatorConfirmCases(t) {
		if !V7ConfirmMachineVerifiable(code) {
			t.Errorf("evaluator case %q is not a registered machine code", code)
		}
	}
	for code, spec := range v7ConfirmRegistry {
		if _, ok := hunterV7TagCatalog[code]; !ok {
			t.Errorf("registered code %q has no catalog definition", code)
		}
		if spec.Checklist && (spec.RefreshSatisfiable || spec.LiveReviewable) {
			t.Errorf("checklist code %q cannot be refresh-satisfiable or live-reviewable", code)
		}
	}
	for code := range v7RefreshSettledConfirms {
		if !V7ConfirmMachineVerifiable(code) || !V7ConfirmRefreshSatisfiable(code) {
			t.Errorf("refresh-settled %q must be a refresh-satisfiable machine code", code)
		}
	}
	// #136: the refresh-settled set is exactly the RefreshOwned registry rows.
	for code, spec := range v7ConfirmRegistry {
		if spec.RefreshOwned != v7RefreshSettledConfirms[code] {
			t.Errorf("%s: RefreshOwned=%v disagrees with the refresh-settled set", code, spec.RefreshOwned)
		}
	}
}

// The census itself would catch the codes the old line scanner missed.
func TestConfirmCensus133_ScannerFindsMultiLineAndFunctionCodes(t *testing.T) {
	emitted := scanEmittedConfirmCodes(t)
	for _, code := range []string{
		"enter_only_near_range_top",                 // defaultV7Confirmations (function)
		"15m_close_above_bb_upper_or_4h_resistance", // multi-line slice (pre-move radar)
		"oi_flush_or_failed_oi_rebuild",             // fundingReversalConfirmations
		"live_price_in_entry_zone",                  // package constant (T2 adapter)
		"5m_close_above_trigger",                    // NextConfirm (readiness)
		"long_trigger: break_high_with_oi_increase", // conflict row composite literal
	} {
		if len(emitted[code]) == 0 {
			t.Errorf("scanner missed %q", code)
		}
	}
	for _, notCode := range []string{"trend_breakout_long", "immediate", "wait_breakout"} {
		if len(emitted[notCode]) > 0 {
			t.Errorf("scanner collected condition value %q", notCode)
		}
	}
}
