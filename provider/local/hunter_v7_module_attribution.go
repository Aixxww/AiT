package local

// ============================================================================
// Hunter v7 — per-module Match/Score failure attribution (#163)
// ============================================================================
// Instrumentation only: zero behavior change. In the Tier A rerun DB,
// module_no_match was the decisive outcome for 31,551 candidate evaluations
// with no record of WHICH module rejected the symbol or on which condition.
// The router now records one V7ModuleOutcome per module it actually
// evaluates for a symbol (regime-weight-gated and breaker-skipped modules
// are not evaluated and are not recorded), and the outcomes ride on the
// module_no_match synthetic signal into the persisted record's RawJSON.
//
// Single source of truth: the verdict and the reason come from ONE
// evaluation path. Every module's Match/Score delegates to its
// ExplainMatch/ExplainScore, whose guard clauses return the first failing
// condition's code at the exact `return false` / `return nil` site that
// decided the outcome. There is no parallel re-implementation of module
// conditions anywhere.

// V7ModuleOutcome is one module's evaluation outcome for one symbol.
// JSON keys are single letters on purpose: a module_no_match record can
// carry up to 22 entries and the field must stay small (reason codes,
// not prose).
type V7ModuleOutcome struct {
	Module string `json:"m"`           // setup type of the module evaluated
	Stage  string `json:"s"`           // match / score / matched (see constants)
	Code   string `json:"c,omitempty"` // first failing condition's reason code; "" when matched
}

// Stage values for V7ModuleOutcome.
const (
	V7ModuleStageMatch   = "match"   // rejected by a Match() hard condition
	V7ModuleStageScore   = "score"   // Match passed; Score() returned nil (floor or Score-internal gate)
	V7ModuleStageMatched = "matched" // module produced a signal for the symbol
)

// v7CodeScoreBelowFloor is the shared reason code for the A-34 module
// score floors: a module matched, scored, and its SetupScore landed below
// the module's minimum (scaffold finish/finishWithTiming, and the
// hand-written floor in funding_reversal).
const v7CodeScoreBelowFloor = "score_below_floor"

// v7ModuleExplainer is implemented by every routing module. ExplainMatch
// returns the Match verdict and, on failure, the first failing condition's
// reason code. ExplainScore returns the Score result and, when it is nil,
// the reason: the scaffold score floor (v7CodeScoreBelowFloor) or a
// Score-internal gate's code. When ExplainScore is called on a symbol the
// module does not match, it returns the ExplainMatch code; the router
// never observes that case because it explains Match first.
type v7ModuleExplainer interface {
	ExplainMatch(ctx *V7SymbolContext, regime V7MarketRegime) (bool, string)
	ExplainScore(ctx *V7SymbolContext, regime V7MarketRegime) (*V7SignalOutput, string)
}

// v7MatchExplained evaluates a module's Match with attribution, falling
// back to the plain verdict (empty code) for any module that does not
// implement the explainer interface.
func v7MatchExplained(mod V7SignalModule, ctx *V7SymbolContext, regime V7MarketRegime) (bool, string) {
	if ex, ok := mod.(v7ModuleExplainer); ok {
		return ex.ExplainMatch(ctx, regime)
	}
	return mod.Match(ctx, regime), ""
}

// v7ScoreExplained is v7MatchExplained for Score.
func v7ScoreExplained(mod V7SignalModule, ctx *V7SymbolContext, regime V7MarketRegime) (*V7SignalOutput, string) {
	if ex, ok := mod.(v7ModuleExplainer); ok {
		return ex.ExplainScore(ctx, regime)
	}
	return mod.Score(ctx, regime), ""
}
