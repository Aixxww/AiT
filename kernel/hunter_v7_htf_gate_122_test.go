package kernel

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

func htf122(coin CandidateCoin) CandidateCoin {
	coin.V7RiskTags = append(append([]string{}, coin.V7RiskTags...), local.V7HTFAgainstDirectionTag)
	return coin
}

// Trend family against 1h/4h never reaches open-review; the untagged twin
// (the with-trend path) is unchanged.
func TestHTF122TrendFamilyHeldAtWatch(t *testing.T) {
	coin := hunterV7PromptReadyLeaderCandidate("TRNDUSDT", 100) // leader_momentum_long: trend family
	before, _ := classifyHunterV7CandidateTier(coin)
	if before != "EXECUTABLE" && before != "REVIEWABLE" {
		t.Fatalf("control: with-trend twin tiers %s, want open-review", before)
	}
	tier, reason := classifyHunterV7CandidateTier(htf122(coin))
	if tier != "WATCH" || reason != hunterV7HTFTrendBlockedReason {
		t.Fatalf("against 1h/4h: %s/%s, want WATCH/%s", tier, reason, hunterV7HTFTrendBlockedReason)
	}
}

// Counter-trend family against 1h/4h is held at REVIEWABLE, never
// EXECUTABLE; a REVIEWABLE row stays REVIEWABLE; WATCH is untouched.
func TestHTF122CounterTrendCapAtReviewable(t *testing.T) {
	coin := htf122(CandidateCoin{V7SetupType: "pullback_reversal_long", Direction: "LONG"})
	if tier, reason := hunterV7HTFTierCap(coin, "EXECUTABLE", "r"); tier != "REVIEWABLE" || reason != hunterV7HTFCounterTrendReason {
		t.Fatalf("EXECUTABLE -> %s/%s, want REVIEWABLE/%s", tier, reason, hunterV7HTFCounterTrendReason)
	}
	if tier, reason := hunterV7HTFTierCap(coin, "REVIEWABLE", "r"); tier != "REVIEWABLE" || reason != "r" {
		t.Fatalf("REVIEWABLE -> %s/%s", tier, reason)
	}
	if tier, _ := hunterV7HTFTierCap(coin, "WATCH", "w"); tier != "WATCH" {
		t.Fatal("WATCH must stay WATCH")
	}
	// Ungated setups and untagged rows pass through.
	for _, c := range []CandidateCoin{
		htf122(CandidateCoin{V7SetupType: "range_expansion_event"}),
		{V7SetupType: "pullback_reversal_long"},
	} {
		if tier, _ := hunterV7HTFTierCap(c, "EXECUTABLE", "r"); tier != "EXECUTABLE" {
			t.Fatalf("%s tags=%v: capped to %s", c.V7SetupType, c.V7RiskTags, tier)
		}
	}
}

// The validator: a trend-family open against 1h/4h is rejected; a
// counter-trend open needs confidence >= 85; with-trend opens keep the
// strategy floor.
func TestHTF122ValidatorFloor(t *testing.T) {
	prices := map[string]float64{"XUSDT": 100}
	mk := func(conf int, gate string) *Decision {
		return &Decision{Symbol: "XUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 100, StopLoss: 98, TakeProfit: 103,
			Confidence: conf, HunterV7HTFGate: gate}
	}
	check := func(d *Decision) error {
		return validateDecision(d, 1000, 20, 20, 10, 10, nil, prices, FixedDecisionRRFloor(1.2), 75)
	}
	if err := check(mk(84, store.V7HTFFamilyCounterTrend)); err == nil || !strings.Contains(err.Error(), "counter-trend floor 85") {
		t.Fatalf("counter-trend at 84 must be rejected, got %v", err)
	}
	if err := check(mk(85, store.V7HTFFamilyCounterTrend)); err != nil {
		t.Fatalf("counter-trend at 85 must pass: %v", err)
	}
	if err := check(mk(95, store.V7HTFFamilyTrend)); err == nil || !strings.Contains(err.Error(), "htf_against_trend_family") {
		t.Fatalf("trend family against 1h/4h must be rejected at any confidence, got %v", err)
	}
	if err := check(mk(75, "")); err != nil {
		t.Fatalf("with-trend open at the strategy floor 75 must pass: %v", err)
	}
}

// Hydration carries the gate from the selected candidate and clears it for
// everything else.
func TestHTF122Hydration(t *testing.T) {
	cands := []CandidateCoin{
		htf122(CandidateCoin{V7SignalID: "ct", V7SetupType: "distribution_short"}),
		{V7SignalID: "ok", V7SetupType: "distribution_short"},
	}
	ds := []Decision{
		{Action: "open_short", SelectedHunterV7SignalID: "ct"},
		{Action: "open_short", SelectedHunterV7SignalID: "ok"},
		{Action: "wait", SelectedHunterV7SignalID: "ct", HunterV7HTFGate: "stale"},
	}
	hydrateHunterV7DecisionExecutionPlans(ds, cands)
	if ds[0].HunterV7HTFGate != store.V7HTFFamilyCounterTrend || ds[1].HunterV7HTFGate != "" || ds[2].HunterV7HTFGate != "" {
		t.Fatalf("hydrated gates = %q / %q / %q", ds[0].HunterV7HTFGate, ds[1].HunterV7HTFGate, ds[2].HunterV7HTFGate)
	}
}
