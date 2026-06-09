package kernel

import "testing"

func TestParseFullDecisionResponseBackfillsMissingReasoningFromCoT(t *testing.T) {
	response := `<reasoning>
OPENUSDT short remains valid. Price is below planned SL and has not reached TP, so hold.
</reasoning>
<decision>
[
  {"symbol":"OPENUSDT","action":"hold","confidence":80}
]
</decision>`

	decision, err := parseFullDecisionResponse(response, 100, 20, 20, 10, 10, nil, false)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(decision.Decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decision.Decisions))
	}
	if decision.Decisions[0].Reasoning == "" {
		t.Fatalf("missing reasoning was not backfilled")
	}
	if decision.Decisions[0].Reasoning != "OPENUSDT short remains valid. Price is below planned SL and has not reached TP, so hold." {
		t.Fatalf("reasoning = %q", decision.Decisions[0].Reasoning)
	}
}
