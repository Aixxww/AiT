package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// #119 (Audit R2 H2, AUD-E1): the U10 snapshot budget reads backend ages
// only. When the model echoes a signal_age_ms that disagrees with the
// backend age, the backend value wins in both directions.
func TestSignalAge119BudgetUsesBackendAgeOnly(t *testing.T) {
	at := t2StopTrader()
	at.config.StrategyConfig.CoinSource.Hunter = &store.HunterConfig{V7DataTiming: &store.V7DataTimingConfig{
		FreshnessBudgetSeconds: map[string]float64{store.V7ClassImpulse: 60}}}
	cand := &kernel.CandidateCoin{V7SetupType: "leader_momentum_long", V7SignalClass: store.V7ClassImpulse}

	// Stale backend data, model claims 1 ms: still rejected (was the bypass).
	cand.V7DataFreshness.PriceAgeMs = 90_000
	err := at.validateHunterV7SnapshotBudget(cand, &kernel.Decision{Symbol: "AUSDT", Action: "open_long", SignalAgeMs: 1})
	if err == nil || !strings.Contains(err.Error(), "90000ms") {
		t.Fatalf("model-echoed 1 ms must not bypass a 90 s backend age, got %v", err)
	}
	// Fresh backend data, model claims 10 min: passes (the echo cannot block either).
	cand.V7DataFreshness.PriceAgeMs = 5_000
	if err := at.validateHunterV7SnapshotBudget(cand, &kernel.Decision{Symbol: "AUSDT", Action: "open_long", SignalAgeMs: 600_000}); err != nil {
		t.Fatalf("model-echoed age must not override a fresh backend age: %v", err)
	}
	// Snapshot age is the backend fallback when the price age is unknown.
	cand.V7DataFreshness.PriceAgeMs, cand.V7DataFreshness.SnapshotAgeMs = 0, 120_000
	if err := at.validateHunterV7SnapshotBudget(cand, &kernel.Decision{Symbol: "AUSDT", Action: "open_long", SignalAgeMs: 1}); err == nil {
		t.Fatal("snapshot age fallback must gate when the price age is unknown")
	}
}

// Both age gates share the one backend helper.
func TestSignalAge119SharedBackendHelper(t *testing.T) {
	c := &kernel.CandidateCoin{}
	c.V7DataFreshness.PriceAgeMs, c.V7DataFreshness.SnapshotAgeMs = 7, 9
	if got := hunterV7BackendDataAgeMs(c); got != 7 {
		t.Fatalf("price age first: %d", got)
	}
	c.V7DataFreshness.PriceAgeMs = 0
	if got := hunterV7BackendDataAgeMs(c); got != 9 {
		t.Fatalf("snapshot fallback: %d", got)
	}
	if hunterV7BackendDataAgeMs(nil) != 0 {
		t.Fatal("nil candidate")
	}
}
