package kernel

import (
	"math"
	"testing"
	"time"

	local "github.com/Aixxww/AiT/provider/local"
)

// Fix 14: the T2 TP1RR / TP2RR reach the candidate, the stored row and the
// prompt payload, and do not move any tier.
func TestT2TPRRRecordedNotTiered(t *testing.T) {
	sig := (&local.V7T2Signal{Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", Status: "WATCH", Score: 70,
		Trigger: 100, Entry: 100.15, Stop: 98.65, Target1: 102.85, Target2: 104.65}).ToV7SignalOutputWithLifecycle("", "TRIGGERED")
	if math.Abs(sig.TP1RR-1.8) > 1e-9 || math.Abs(sig.TP2RR-3.0) > 1e-9 {
		t.Fatalf("output RR %v / %v", sig.TP1RR, sig.TP2RR)
	}
	g := defaultGeometry()
	with := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "", g)
	without := sig
	without.TP1RR, without.TP2RR = 0, 0
	base := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{without}, "", g)
	if len(with) != 1 || len(base) != 1 {
		t.Fatal("assembly")
	}
	if with[0].V7ExecutionTier != base[0].V7ExecutionTier || with[0].V7TierReason != base[0].V7TierReason {
		t.Fatalf("tier moved with TP RR: %s/%s vs %s/%s", with[0].V7ExecutionTier, with[0].V7TierReason, base[0].V7ExecutionTier, base[0].V7TierReason)
	}
	if with[0].V7TP1RR != sig.TP1RR || with[0].V7TP2RR != sig.TP2RR {
		t.Fatalf("candidate RR %v / %v", with[0].V7TP1RR, with[0].V7TP2RR)
	}
	p := buildHunterV7PromptPayload(with[0])
	if p.TP1RR != sig.TP1RR || p.TP2RR != sig.TP2RR {
		t.Fatalf("prompt payload RR %v / %v", p.TP1RR, p.TP2RR)
	}
	recs := BuildHunterV7SignalRecords([]local.V7SignalOutput{sig}, with)
	db := BuildHunterV7SignalDBRecords(1, recs, time.Now().UTC(), "")
	if db[0].TP1RR != sig.TP1RR || db[0].TP2RR != sig.TP2RR {
		t.Fatalf("db RR %v / %v", db[0].TP1RR, db[0].TP2RR)
	}
}
