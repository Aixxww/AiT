package kernel

import (
	"strings"
	"testing"
)

// #132 / Audit R2 H6: with hunter_v7 candidates the version header is
// required even when the user prompt lacks the Tier Summary marker.
func TestContract132_HeaderRequiredWithCandidates(t *testing.T) {
	cfg := u11FixtureConfig("en")
	sys, user := s2Render(cfg)
	e := NewStrategyEngine(cfg)
	if err := e.hunterV7PromptContractError(sys, user, 3); err != nil {
		t.Fatalf("a correct round must pass: %v", err)
	}
	stripped := strings.ReplaceAll(strings.ReplaceAll(user, "Tier Summary:", "Tiers:"), hunterV7PromptHeader(e.hunterV7PromptConfig()), "")
	if err := e.hunterV7PromptContractError(sys, stripped, 3); err == nil || !strings.Contains(err.Error(), "lacks the header") {
		t.Fatalf("candidates without the header must fail, got %v", err)
	}
	if err := e.hunterV7PromptContractError(sys, stripped, 0); err != nil {
		t.Fatalf("no candidates, no tier section: %v", err)
	}
	// The kernel check reads the same token set as the save-time check.
	h := u11HybridConfig("en")
	h.CustomPrompt = "weigh suggested_trigger first"
	hs, hu := s2Render(h)
	if err := NewStrategyEngine(h).hunterV7PromptContractError(hs, hu, 3); err == nil || !strings.Contains(err.Error(), "suggested_trigger") {
		t.Fatalf("hybrid with a legacy-only field: %v", err)
	}
}
