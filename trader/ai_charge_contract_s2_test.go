package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// #111 S2 review fix: a round the prompt contract stopped (no AI call,
// kernel.FullDecision.AINotCalled) books no AI charge; a round that called
// the AI books exactly one, whatever its outcome. The kernel side (the
// contract-stopped decision carries AINotCalled) is asserted in
// kernel.TestS2GetFullDecisionFailsClosedWithoutCallingAI.
func TestS2ContractStoppedRoundBooksNoAICharge(t *testing.T) {
	st, err := store.New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatalf("store init failed: %v", err)
	}
	at := &AutoTrader{id: "s2-charge", aiModel: "deepseek", store: st}
	count := func() int {
		t.Helper()
		charges, _, err := st.AICharge().GetCharges(at.id, "all")
		if err != nil {
			t.Fatalf("get charges: %v", err)
		}
		return len(charges)
	}

	stopped := &kernel.FullDecision{
		AINotCalled: true,
		Decisions:   []kernel.Decision{{Symbol: "ALL", Action: "wait", BlockedReasonCode: "backend_guard_risk", Reasoning: "prompt_contract_mismatch: test"}},
	}
	at.recordAICharge(stopped)
	at.recordAICharge(nil)
	if n := count(); n != 0 {
		t.Fatalf("contract-stopped / nil rounds booked %d AI charges, want 0", n)
	}

	called := &kernel.FullDecision{RawResponse: "<decision>[]</decision>", Decisions: []kernel.Decision{{Symbol: "ALL", Action: "wait"}}}
	at.recordAICharge(called)
	if n := count(); n != 1 {
		t.Fatalf("an AI round booked %d charges, want exactly 1", n)
	}
	charges, total, _ := st.AICharge().GetCharges(at.id, "all")
	if charges[0].Model != "deepseek" || total != store.GetModelPrice("deepseek") {
		t.Fatalf("charge %+v total %v", charges[0], total)
	}
}

// The same marker keeps a contract-stopped round from counting as an AI
// recovery: the failure counter and safe mode stay as they were; a round
// the AI answered still resets them.
func TestS2ContractStoppedRoundIsNotAIRecovery(t *testing.T) {
	at := &AutoTrader{id: "s2-safe", consecutiveAIFailures: 3, safeMode: true, safeModeReason: "AI failed 3 consecutive times"}
	at.noteAISuccess(&kernel.FullDecision{AINotCalled: true})
	if at.consecutiveAIFailures != 3 || !at.safeMode || at.safeModeReason == "" {
		t.Fatalf("contract-stopped round changed AI health: failures %d safeMode %v", at.consecutiveAIFailures, at.safeMode)
	}
	at.noteAISuccess(&kernel.FullDecision{RawResponse: "<decision>[]</decision>"})
	if at.consecutiveAIFailures != 0 || at.safeMode || at.safeModeReason != "" {
		t.Fatalf("an AI round must reset AI health: failures %d safeMode %v", at.consecutiveAIFailures, at.safeMode)
	}
}
