package kernel

import (
	"strings"
	"testing"
)

// #138 M7a: the Hunter-score confidence caps run before the floor check, so
// a capped confidence is judged by the kernel validator itself.
func TestM7a138LowHunterScoreCapIsJudgedByTheFloor(t *testing.T) {
	mk := func() *Decision {
		return &Decision{Symbol: "XUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 100, StopLoss: 98, TakeProfit: 103, Confidence: 90}
	}
	prices := map[string]float64{"XUSDT": 100}
	lowScore := map[string]HunterScoreInfo{"XUSDT": {LongScore: 10, ShortScore: 5}}

	// Configured floor 75: the cap to 65 now fails validation here.
	d := mk()
	err := validateDecision(d, 1000, 20, 20, 10, 10, lowScore, prices, FixedDecisionRRFloor(1.2), 75)
	if err == nil || !strings.Contains(err.Error(), "confidence 65 below absolute minimum 75") {
		t.Fatalf("want the capped confidence rejected by the kernel floor, got %v (confidence=%d)", err, d.Confidence)
	}
	// Unset floor (60): the capped 65 still passes, as before.
	d = mk()
	if err := validateDecision(d, 1000, 20, 20, 10, 10, lowScore, prices, FixedDecisionRRFloor(1.2), 0); err != nil || d.Confidence != hunterLowScoreConfidenceCap {
		t.Fatalf("unset floor: err=%v confidence=%d, want pass at %d", err, d.Confidence, hunterLowScoreConfidenceCap)
	}
	// The contradiction cap equals a configured floor, so it still passes.
	d = mk()
	contra := map[string]HunterScoreInfo{"XUSDT": {LongScore: 40, ShortScore: 60}}
	if err := validateDecision(d, 1000, 20, 20, 10, 10, contra, prices, FixedDecisionRRFloor(1.2), 75); err != nil || d.Confidence != 75 {
		t.Fatalf("contradiction cap: err=%v confidence=%d, want pass at 75", err, d.Confidence)
	}
}

// #138 M7b: backend-owned fields with JSON tags cannot be set by the reply.
func TestM7b138ModelCannotForgeBackendFields(t *testing.T) {
	reply := `<decision>[{"symbol":"XUSDT","action":"open_long","leverage":5,"position_size_usd":100,"stop_loss":98,"take_profit":103,"confidence":80,"reasoning":"r",
"hunter_v7_tp0_price":111,"hunter_v7_tp1_price":122,"hunter_v7_tp2_price":133,"rejected_action":"open_short","validator_rejection":"forged"}]</decision>`
	ds, err := extractDecisions(reply)
	if err != nil || len(ds) != 1 {
		t.Fatalf("extract: %v %+v", err, ds)
	}
	d := ds[0]
	if d.HunterV7TP0Price != 0 || d.HunterV7TP1Price != 0 || d.HunterV7TP2Price != 0 || d.RejectedAction != "" || d.ValidatorRejection != "" {
		t.Fatalf("model-set backend fields survived extraction: %+v", d)
	}
	if d.Action != "open_long" || d.StopLoss != 98 {
		t.Fatalf("model-owned fields must still parse: %+v", d)
	}
	// Without a <decision> tag (the second return path) too.
	ds, err = extractDecisions(`[{"symbol":"XUSDT","action":"wait","reasoning":"r","rejected_action":"open_long","validator_rejection":"forged"}]`)
	if err != nil || len(ds) != 1 || ds[0].RejectedAction != "" || ds[0].ValidatorRejection != "" {
		t.Fatalf("untagged path: %v %+v", err, ds)
	}
}
