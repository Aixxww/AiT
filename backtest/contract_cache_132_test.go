package backtest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
)

// #132 / Audit R2 H6: a round the prompt contract stopped before the AI
// call (FullDecision.AINotCalled) never reaches the AI cache and aborts the
// run; an AI round is cached as before.
func TestContractStoppedRoundLeavesAICacheUntouched(t *testing.T) {
	cache, err := LoadAICache(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: BacktestConfig{CacheAI: true, PromptVariant: "balanced"}, aiCache: cache}
	stopped := &kernel.FullDecision{AINotCalled: true, Decisions: []kernel.Decision{{Symbol: "ALL", Action: "wait",
		BlockedReasonCode: "backend_guard_risk", Reasoning: "prompt_contract_mismatch: test"}}}
	err = r.acceptAIDecision("k1", 1, stopped)
	if err == nil || !strings.Contains(err.Error(), "prompt_contract_mismatch: test") {
		t.Fatalf("a contract-stopped round must abort the run, got %v", err)
	}
	if len(cache.Entries) != 0 {
		t.Fatalf("aiCache written: %+v", cache.Entries)
	}
	// An AI round is cached and replayed.
	ok := &kernel.FullDecision{RawResponse: "<decision>[]</decision>", Decisions: []kernel.Decision{{Symbol: "ALL", Action: "wait"}}}
	if err := r.acceptAIDecision("k2", 2, ok); err != nil {
		t.Fatal(err)
	}
	if got, hit := r.cachedAIDecision("k2"); !hit || got.RawResponse != ok.RawResponse {
		t.Fatalf("AI round not cached: %v %+v", hit, got)
	}
	// Caching off: nothing stored, no error.
	r.cfg.CacheAI = false
	if err := r.acceptAIDecision("k3", 3, ok); err != nil || len(cache.Entries) != 1 {
		t.Fatalf("cache off: %v, entries %d", err, len(cache.Entries))
	}
}

// An entry a contract-stopped round wrote before the fix is never replayed.
func TestPoisonedCacheEntryIsAMiss(t *testing.T) {
	cache, err := LoadAICache(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Put("old", "balanced", 1, &kernel.FullDecision{AINotCalled: true}); err != nil {
		t.Fatal(err)
	}
	r := &Runner{aiCache: cache}
	if _, hit := r.cachedAIDecision("old"); hit {
		t.Fatal("a poisoned entry must read as a miss")
	}
	if _, hit := r.cachedAIDecision(""); hit {
		t.Fatal("empty key")
	}
}
