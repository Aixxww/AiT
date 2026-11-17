package store

import "testing"

func TestFinalConflictDefaults(t *testing.T) {
	var h *HunterConfig
	if h.EffectiveV7FinalConflictMinAIPriority() != 60 || h.EffectiveV7FinalConflictTieWinner() != "SHORT" || !h.V7ContextNonVoting() {
		t.Fatalf("defaults must be 60 / SHORT / CONTEXT non-voting")
	}
	h = &HunterConfig{V7FinalConflictMinAIPriority: 70, V7FinalConflictTieWinner: "long", V7ConflictContextVotes: true}
	if h.EffectiveV7FinalConflictMinAIPriority() != 70 || h.EffectiveV7FinalConflictTieWinner() != "LONG" || h.V7ContextNonVoting() {
		t.Fatalf("config values must apply")
	}
}
