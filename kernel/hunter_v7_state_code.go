package kernel

import (
	"fmt"
	"math"
	"strings"

	"github.com/Aixxww/AiT/logger"
	local "github.com/Aixxww/AiT/provider/local"
)

// #111 S4 (review item B): a one-line state code per Hunter v7 candidate,
// for logs, DB rows and a quick human scan. It is never part of the prompt
// (the LLM already gets every field it carries).
//
// Format:
//
//	V7|<SYMBOL>|<SIDE>|<setup_type>|<market_regime>|P<ai_priority>|S<setup>/T<timing>/F<regime_fit>/L<liquidity>/R<risk>|<tier>|<status or lifecycle_state>
//
// Scores are rounded to integers. A Task-2 row appends
// |tf=<structure_tf>/<higher_tf> (its structure timeframes); a native row
// has no per-signal timeframe field, so it carries none. An empty value is
// "-". Example (golden 2026-07-26):
//
//	V7|OPUSDT|LONG|mms_trend_ride_long|trend_up|P85|S100/T68/F80/L60/R0|REVIEWABLE|candidate

// hunterV7StateCodeFields are the values a state code is built from.
type hunterV7StateCodeFields struct {
	Symbol, Direction, SetupType, Regime string
	AIPriority                           float64
	Setup, Timing, RegimeFit, Liquidity  float64
	Risk                                 float64
	Tier                                 string
	Status, LifecycleState               string
	T2                                   bool
}

func hunterV7StateCode(f hunterV7StateCodeFields) string {
	dash := func(s string) string {
		if s = strings.TrimSpace(s); s == "" {
			return "-"
		}
		return s
	}
	n := func(v float64) int { return int(math.Round(v)) }
	state := f.Status
	if f.T2 && f.LifecycleState != "" {
		state = f.LifecycleState
	}
	code := fmt.Sprintf("V7|%s|%s|%s|%s|P%d|S%d/T%d/F%d/L%d/R%d|%s|%s",
		dash(f.Symbol), dash(strings.ToUpper(f.Direction)), dash(f.SetupType), dash(f.Regime),
		n(f.AIPriority), n(f.Setup), n(f.Timing), n(f.RegimeFit), n(f.Liquidity), n(f.Risk),
		dash(f.Tier), dash(state))
	if f.T2 {
		cfg := local.V7StructureConfigForSetup(f.SetupType)
		code += fmt.Sprintf("|tf=%s/%s", dash(hunterV7V2TFLabel(cfg.TFSIntervalMs)), dash(hunterV7V2TFLabel(cfg.TFHIntervalMs)))
	}
	return code
}

// hunterV7CandidateStateCode is the state code of an assembled candidate
// at the given tier.
func hunterV7CandidateStateCode(c CandidateCoin, tier string) string {
	return hunterV7StateCode(hunterV7StateCodeFields{
		Symbol: c.Symbol, Direction: c.Direction, SetupType: c.V7SetupType, Regime: c.V7MarketRegime,
		AIPriority: c.V7AIPriority, Setup: c.V7SetupScore, Timing: c.V7TimingScore, RegimeFit: c.V7RegimeFitScore,
		Liquidity: c.V7LiquidityScore, Risk: c.V7RiskScore,
		Tier: tier, Status: c.V7Status, LifecycleState: c.V7LifecycleState, T2: c.V7Stack == local.V7StackT2,
	})
}

// hunterV7SignalStateCode is the state code of a router signal at the
// given tier (the persisted record's tier).
func hunterV7SignalStateCode(s local.V7SignalOutput, tier string) string {
	return hunterV7StateCode(hunterV7StateCodeFields{
		Symbol: s.Symbol, Direction: string(s.Direction), SetupType: string(s.SetupType), Regime: string(s.MarketRegime),
		AIPriority: s.AIPriority, Setup: s.SetupScore, Timing: s.TimingScore, RegimeFit: s.RegimeFitScore,
		Liquidity: s.LiquidityScore, Risk: s.RiskScore,
		Tier: tier, Status: string(s.Status), LifecycleState: s.LifecycleState, T2: s.Stack == local.V7StackT2,
	})
}

// logHunterV7StateCodes logs one state code per prompt-final candidate.
func logHunterV7StateCodes(items []hunterV7PromptCandidate) {
	for _, it := range items {
		logger.Infof("hunter_v7 state: %s", hunterV7CandidateStateCode(it.Coin, it.Tier))
	}
}
