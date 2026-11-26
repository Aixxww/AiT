package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/provider/local"
)

// #136: the decision-time refresh waives a REVIEWABLE confirmation gap only
// when the code's shared predicate does not still fail on the candidate's
// facts. Before, the refresh waived any refresh-satisfiable gap outright, so
// a code could open below its scoring threshold.
func refresh136Candidate(setup string, taker float64) *kernel.CandidateCoin {
	return &kernel.CandidateCoin{
		Symbol:           "REFUSDT",
		Direction:        "LONG",
		V7SetupType:      setup,
		V7ExecutionTier:  "REVIEWABLE",
		V7EntryZone:      local.V7PriceZone{Lower: 99, Upper: 101},
		V7PriceContext:   &local.V7PriceContext{Last: 100},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: taker, OIChange1h: 1},
		V7ReasonCodes:    []string{"fresh_micro_confirmed", "fresh_rest_confirmed"},
	}
}

func TestRefresh136WaiverRechecksSharedPredicate(t *testing.T) {
	cases := []struct {
		name  string
		setup string
		taker float64
		code  string
		want  bool
	}{
		{"whale_0.53_below_0.56_not_waived", "whale_flow_reversal", 0.53, "taker_flow_confirms_long", false},
		{"whale_0.57_waived", "whale_flow_reversal", 0.57, "taker_flow_confirms_long", true},
		{"other_0.50_below_0.52_not_waived", "leader_momentum_long", 0.50, "taker_flow_confirms_long", false},
		{"other_0.52_waived", "leader_momentum_long", 0.52, "taker_flow_confirms_long", true},
		{"price_in_zone_trigger_not_cleared", "leader_momentum_long", 0.55, "5m_or_15m_close_above_trigger", false},
		// No shared predicate: the refresh waiver is unchanged.
		{"non_shared_code_still_waived", "leader_momentum_long", 0.50, "no_new_low_after_reclaim", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hunterV7ReviewableConfirmationSatisfiedByRefresh(refresh136Candidate(tc.setup, tc.taker), tc.code)
			if got != tc.want {
				t.Fatalf("waived=%v, want %v", got, tc.want)
			}
		})
	}
	// Without the refresh evidence nothing is waived (unchanged).
	c := refresh136Candidate("leader_momentum_long", 0.60)
	c.V7ReasonCodes = nil
	if hunterV7ReviewableConfirmationSatisfiedByRefresh(c, "taker_flow_confirms_long") {
		t.Fatal("waived without fresh_micro_confirmed / fresh_rest_confirmed")
	}
}
