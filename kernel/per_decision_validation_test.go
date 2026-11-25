package kernel

import (
	"strings"
	"testing"
)

// F06: one invalid decision must not void the batch, and validation failures
// must not be returned as an error (which the trader counts as an AI failure).

const f06Response = "<reasoning>r</reasoning><decision>```json\n[" +
	`{"symbol":"AAAUSDT","action":"close_long","reasoning":"take profit"},` +
	`{"symbol":"BBBUSDT","action":"open_long","leverage":5,"position_size_usd":100,"stop_loss":98,"take_profit":101,"confidence":80,"reasoning":"rr too low"},` +
	`{"symbol":"CCCUSDT","action":"open_long","leverage":5,"position_size_usd":100,"stop_loss":98,"take_profit":104,"confidence":80,"reasoning":"rr ok"}` +
	"]\n```</decision>"

func TestF06InvalidDecisionDemotedBatchKept(t *testing.T) {
	priceMap := map[string]float64{"BBBUSDT": 100, "CCCUSDT": 100}
	fd, err := parseFullDecisionResponse(f06Response, 10000, 20, 10, 10, 1.5, nil, false, priceMap, FixedDecisionRRFloor(1.5), 0)
	if err != nil {
		t.Fatalf("validation failure must not be returned as an error, got %v", err)
	}
	if len(fd.Decisions) != 3 {
		t.Fatalf("expected 3 decisions kept, got %d", len(fd.Decisions))
	}
	if fd.Decisions[0].Action != "close_long" || fd.Decisions[0].ValidatorRejection != "" {
		t.Fatalf("valid close must be untouched: %+v", fd.Decisions[0])
	}
	b := fd.Decisions[1]
	if b.Action != "wait" || b.RejectedAction != "open_long" || b.BlockedReasonCode != "backend_guard_risk" ||
		b.BlockedSignalSymbol != "BBBUSDT" || !strings.Contains(b.ValidatorRejection, "risk/reward") {
		t.Fatalf("RR 0.5 open must be demoted to wait with audit fields, got %+v", b)
	}
	if fd.Decisions[2].Action != "open_long" || fd.Decisions[2].ValidatorRejection != "" {
		t.Fatalf("valid open (RR 2.0 vs min 1.5) must survive, got %+v", fd.Decisions[2])
	}
}

func TestF06HunterV7BadWaitCodeDemotedNotFatal(t *testing.T) {
	resp := "<decision>```json\n[" +
		`{"symbol":"AAAUSDT","action":"wait","blocked_reason_code":"taker_weak","reasoning":"echo"},` +
		`{"symbol":"BBBUSDT","action":"wait","blocked_reason_code":"rr_insufficient","reasoning":"ok"}` +
		"]\n```</decision>"
	fd, err := parseFullDecisionResponse(resp, 10000, 20, 10, 10, 1.5, nil, true, nil, FixedDecisionRRFloor(1.5), 0)
	if err != nil {
		t.Fatalf("bad wait code must not be fatal, got %v", err)
	}
	a := fd.Decisions[0]
	if a.BlockedReasonCode != "backend_guard_risk" || !strings.Contains(a.ValidatorRejection, "taker_weak") {
		t.Fatalf("echoed code must be replaced and recorded, got %+v", a)
	}
	if fd.Decisions[1].BlockedReasonCode != "rr_insufficient" || fd.Decisions[1].ValidatorRejection != "" {
		t.Fatalf("valid wait must be untouched, got %+v", fd.Decisions[1])
	}
}

func TestF06NoJSONKeepsExistingSafeWait(t *testing.T) {
	// #94 / AUD-B1 changed the pre-F06 fallback: a response without a JSON
	// array is no longer an ALL/wait fallback but an AI failure
	// (no_structured_decision). It is still not a validator rejection.
	fd, err := parseFullDecisionResponse("no json here", 10000, 20, 10, 10, 1.5, nil, true, nil, FixedDecisionRRFloor(1.5), 0)
	if err == nil || AIFailureReason(err) != AIFailureNoStructuredDecision {
		t.Fatalf("expected no_structured_decision, got %v / %+v", err, fd)
	}
	if fd == nil || len(fd.Decisions) != 0 {
		t.Fatalf("no fallback decision may be produced, got %+v", fd)
	}
}
