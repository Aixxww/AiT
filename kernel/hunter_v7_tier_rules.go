package kernel

import "strings"

// Hunter v7 tier rules (U3.2)
//
// Each setup's EXECUTABLE/REVIEWABLE gating collapses into ordered rule rows:
// a row matches when every populated field passes, and the first match wins.
// Setups registered in hunterV7SetupTierSpecs are evaluated from the table;
// unregistered setups keep the legacy switch branches until their migration
// unit (U3.3a-u) lands. The shadow test in hunter_v7_tier_shadow_test.go
// replays frozen copies of the legacy switches against the table and fails on
// any divergence.

type hunterV7TakerGate struct {
	// Kind selects the taker predicate: "" (no gate), "at_least", "at_most",
	// "confirmed_at_least" (missing data fails instead of passing), "aligned"
	// (direction-dependent default thresholds).
	Kind      string
	Threshold float64
}

type hunterV7TierRule struct {
	// Identity matchers; empty means "any".
	Direction   string
	Quality     string
	Status      string
	EntrySignal string

	// Score floors; zero means "no floor". RiskBelow is exclusive (<),
	// RiskAtMost inclusive (<=) — the legacy branches use both shapes.
	MinAIPriority  float64
	MinSetupScore  float64
	MinTimingScore float64
	RiskBelow      float64
	RiskAtMost     float64
	// MinLiquidity encodes the recurring "(liquidity == 0 || liquidity >= X)"
	// pattern: unknown liquidity passes, known liquidity must clear the floor.
	MinLiquidity float64

	Taker hunterV7TakerGate

	// Reason-code requirements over V7ReasonCodes.
	RequireAll []string
	RequireAny [][]string
	ForbidAll  []string

	// Guards carry the setup-specific predicates that are not yet (or not
	// worth) data-encoding; all must pass.
	Guards []func(CandidateCoin) bool

	// Reason is the tier reason emitted when the rule matches.
	Reason string
}

type hunterV7SetupTierSpec struct {
	// Ready gates EXECUTABLE when execution quality is "ready" or the entry
	// signal is "entry_open_now"; NearConfirm gates EXECUTABLE for
	// "near_confirm"/candidate status; Reviewable gates REVIEWABLE.
	Ready       []hunterV7TierRule
	NearConfirm []hunterV7TierRule
	Reviewable  []hunterV7TierRule
	// OpenRateFloor gates live-reviewable escalation
	// (hunterV7OpenRateCandidateFloor); nil keeps the shared default floor.
	OpenRateFloor []hunterV7TierRule
}

// hunterV7SetupTierSpecs is the per-setup rule registry. Populated setup by
// setup in U3.3; each entry replaces every legacy switch branch for that setup.
var hunterV7SetupTierSpecs = map[string]hunterV7SetupTierSpec{}

func hunterV7EvalTierRules(coin CandidateCoin, rules []hunterV7TierRule) (bool, string) {
	for i := range rules {
		if hunterV7TierRuleMatches(coin, &rules[i]) {
			return true, rules[i].Reason
		}
	}
	return false, ""
}

func hunterV7TierRuleMatches(coin CandidateCoin, rule *hunterV7TierRule) bool {
	if rule.Direction != "" && !strings.EqualFold(coin.Direction, rule.Direction) {
		return false
	}
	if rule.Quality != "" && coin.V7ExecutionQuality != rule.Quality {
		return false
	}
	if rule.Status != "" && coin.V7Status != rule.Status {
		return false
	}
	if rule.EntrySignal != "" && coin.V7EntrySignal != rule.EntrySignal {
		return false
	}
	if coin.V7AIPriority < rule.MinAIPriority {
		return false
	}
	if coin.V7SetupScore < rule.MinSetupScore {
		return false
	}
	if coin.V7TimingScore < rule.MinTimingScore {
		return false
	}
	if rule.RiskBelow > 0 && coin.V7RiskScore >= rule.RiskBelow {
		return false
	}
	if rule.RiskAtMost > 0 && coin.V7RiskScore > rule.RiskAtMost {
		return false
	}
	if rule.MinLiquidity > 0 && coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < rule.MinLiquidity {
		return false
	}
	switch rule.Taker.Kind {
	case "at_least":
		if !hunterV7TakerBuyAtLeast(coin, rule.Taker.Threshold) {
			return false
		}
	case "at_most":
		if !hunterV7TakerBuyAtMost(coin, rule.Taker.Threshold) {
			return false
		}
	case "confirmed_at_least":
		if !hunterV7TakerBuyConfirmedAtLeast(coin, rule.Taker.Threshold) {
			return false
		}
	case "aligned":
		if !hunterV7TakerBuyAligned(coin) {
			return false
		}
	}
	for _, code := range rule.RequireAll {
		if !containsStringValue(coin.V7ReasonCodes, code) {
			return false
		}
	}
	for _, group := range rule.RequireAny {
		if !containsAnyStringValue(coin.V7ReasonCodes, group) {
			return false
		}
	}
	for _, code := range rule.ForbidAll {
		if containsStringValue(coin.V7ReasonCodes, code) {
			return false
		}
	}
	for _, guard := range rule.Guards {
		if !guard(coin) {
			return false
		}
	}
	return true
}
