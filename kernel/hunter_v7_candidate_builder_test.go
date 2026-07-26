package kernel

// Hunter v7 candidate test builder (U3.6).
//
// v7Candidate builds a CandidateCoin fixture for the classifier tier tables
// with sensible defaults (LONG / status candidate) plus per-field mutators.
// The numeric values carried by the tier-table rows are real historical
// market samples: when adding rows, preserve fixture values verbatim.

import (
	"testing"

	"github.com/Aixxww/AiT/provider/local"
)

func v7Candidate(setup string, mutators ...func(*CandidateCoin)) CandidateCoin {
	coin := CandidateCoin{
		Symbol:      "TESTUSDT",
		Direction:   "LONG",
		V7SetupType: setup,
		V7Status:    "candidate",
	}
	for _, mutate := range mutators {
		mutate(&coin)
	}
	return coin
}

func withSymbol(symbol string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.Symbol = symbol }
}

func withDirection(direction string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.Direction = direction }
}

func withStatus(status string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7Status = status }
}

func withQuality(quality string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7ExecutionQuality = quality }
}

// withScores sets the five composite scores in fixed order:
// ai_priority, setup, timing, risk, liquidity.
func withScores(ai, setup, timing, risk, liq float64) func(*CandidateCoin) {
	return func(c *CandidateCoin) {
		c.V7AIPriority = ai
		c.V7SetupScore = setup
		c.V7TimingScore = timing
		c.V7RiskScore = risk
		c.V7LiquidityScore = liq
	}
}

func withAIPriority(v float64) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7AIPriority = v }
}

func withRegimeFit(v float64) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7RegimeFitScore = v }
}

func withRiskLevel(level string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7RiskLevel = level }
}

func withConfidence(confidence string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7Confidence = confidence }
}

func withRegime(regime string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7MarketRegime = regime }
}

func withShape(shape string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7MarketShape = shape }
}

func withEntrySignal(signal string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7EntrySignal = signal }
}

func withReasons(codes ...string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7ReasonCodes = codes }
}

func withRiskTags(tags ...string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7RiskTags = tags }
}

func withConfirms(confirms ...string) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7RequiredConfirms = confirms }
}

// withTaker sets only the 15m taker-buy ratio, creating the derivatives
// context when absent.
func withTaker(v float64) func(*CandidateCoin) {
	return func(c *CandidateCoin) {
		if c.V7DerivativesCtx == nil {
			c.V7DerivativesCtx = &local.V7DerivativesContext{}
		}
		c.V7DerivativesCtx.TakerBuy15m = v
	}
}

// withDerivatives sets the full derivatives context: OI 1h/4h change plus the
// 15m taker-buy ratio.
func withDerivatives(oi1h, oi4h, taker float64) func(*CandidateCoin) {
	return func(c *CandidateCoin) {
		c.V7DerivativesCtx = &local.V7DerivativesContext{
			OIChange1h:  oi1h,
			OIChange4h:  oi4h,
			TakerBuy15m: taker,
		}
	}
}

func withZone(lower, upper float64) func(*CandidateCoin) {
	return func(c *CandidateCoin) {
		c.V7EntryZone = local.V7PriceZone{Lower: lower, Upper: upper}
	}
}

func withInvalidation(price float64) func(*CandidateCoin) {
	return func(c *CandidateCoin) {
		c.V7Invalidation = local.V7InvalidationRule{Price: price}
	}
}

func withTargets(targets ...local.V7Target) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7Targets = targets }
}

func withPriceCtx(ctx *local.V7PriceContext) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7PriceContext = ctx }
}

func withConfirmSummary(summary *local.V7ConfirmationSummary) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7ConfirmSummary = summary }
}

func withReadiness(readiness *local.V7ExecutionReadiness) func(*CandidateCoin) {
	return func(c *CandidateCoin) { c.V7Readiness = readiness }
}

// v7TierCase is one row of a classifier tier table. Assertions mirror the
// original per-function tests exactly:
//   - wantTier / wantReason assert an exact match when non-empty.
//   - notTiers / notReasons each independently forbid a value (disjunction:
//     hitting any one of them fails).
//   - notPairTier + notPairReason forbid one exact tier+reason combination
//     (conjunction: fails only when both match together).
type v7TierCase struct {
	name          string
	coin          CandidateCoin
	wantTier      string
	wantReason    string
	notTiers      []string
	notReasons    []string
	notPairTier   string
	notPairReason string
}

func runHunterV7TierCases(t *testing.T, cases []v7TierCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTier(tt.coin)
			if tt.wantTier != "" && tier != tt.wantTier {
				t.Fatalf("tier = %q (%s), want %s", tier, reason, tt.wantTier)
			}
			if tt.wantReason != "" && reason != tt.wantReason {
				t.Fatalf("reason = %q (tier %s), want %s", reason, tier, tt.wantReason)
			}
			for _, not := range tt.notTiers {
				if tier == not {
					t.Fatalf("tier = %q (%s), must not be %s", tier, reason, not)
				}
			}
			for _, not := range tt.notReasons {
				if reason == not {
					t.Fatalf("reason = %q (tier %s), must not be %s", reason, tier, not)
				}
			}
			if tt.notPairTier != "" && tier == tt.notPairTier && reason == tt.notPairReason {
				t.Fatalf("tier/reason = %s/%s, this combination is forbidden", tier, reason)
			}
		})
	}
}
