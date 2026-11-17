package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
)

// F01: a Hunter v7 open must target a symbol/direction from this cycle's
// candidate set, and a selected signal id must belong to the decision symbol.

func f01Context() *kernel.Context {
	return &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{{
			Symbol:          "AAAUSDT",
			Direction:       "LONG",
			V7SignalID:      "sig-aaa",
			V7SetupType:     "trend_breakout_long",
			V7ExecutionTier: "EXECUTABLE",
		}},
	}
}

func f01Trader() *AutoTrader {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	return at
}

func TestF01RejectsOpenOutsideCandidateSetWithoutSignalID(t *testing.T) {
	err := f01Trader().validateHunterV7ExecutionGuard(f01Context(), &kernel.Decision{Symbol: "ZZZUSDT", Action: "open_long"})
	if err == nil || !strings.Contains(err.Error(), "outside_candidate_set") {
		t.Fatalf("expected outside_candidate_set rejection, got: %v", err)
	}
}

func TestF01RejectsOpenAgainstCandidateDirection(t *testing.T) {
	err := f01Trader().validateHunterV7ExecutionGuard(f01Context(), &kernel.Decision{Symbol: "AAAUSDT", Action: "open_short"})
	if err == nil || !strings.Contains(err.Error(), "outside_candidate_set") {
		t.Fatalf("expected rejection for direction not in candidate set, got: %v", err)
	}
}

func TestF01RejectsSignalIDBelongingToAnotherSymbol(t *testing.T) {
	d := &kernel.Decision{Symbol: "ZZZUSDT", Action: "open_long", SelectedHunterV7SignalID: "sig-aaa"}
	err := f01Trader().validateHunterV7ExecutionGuard(f01Context(), d)
	if err == nil || !strings.Contains(err.Error(), "belongs to AAAUSDT") {
		t.Fatalf("expected symbol/signal mismatch rejection, got: %v", err)
	}
}

func TestF01RejectsUnknownSignalID(t *testing.T) {
	d := &kernel.Decision{Symbol: "AAAUSDT", Action: "open_long", SelectedHunterV7SignalID: "sig-missing"}
	err := f01Trader().validateHunterV7ExecutionGuard(f01Context(), d)
	if err == nil || !strings.Contains(err.Error(), "signal_contract_mismatch") {
		t.Fatalf("expected signal_contract_mismatch, got: %v", err)
	}
}

func TestF01NonOpenAndNonV7Unaffected(t *testing.T) {
	at := f01Trader()
	if err := at.validateHunterV7ExecutionGuard(f01Context(), &kernel.Decision{Symbol: "ZZZUSDT", Action: "close_long"}); err != nil {
		t.Fatalf("close must not be blocked by the candidate-set rule, got: %v", err)
	}
	at.config.StrategyConfig.CoinSource.SourceType = "hunter"
	if err := at.validateHunterV7ExecutionGuard(f01Context(), &kernel.Decision{Symbol: "ZZZUSDT", Action: "open_long"}); err != nil {
		t.Fatalf("non-v7 strategy must be unaffected, got: %v", err)
	}
}
