package trader

import (
	"errors"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// #77: the trader enforces the strategy's max_positions and min_confidence
// as configured (owner: 1 and 75).

func ownerConfigTrader() *AutoTrader {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.RiskControl.MaxPositions = 1
	at.config.StrategyConfig.RiskControl.MinConfidence = 75
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.2
	return at
}

func TestMaxPositionsUsesConfiguredOne(t *testing.T) {
	at := ownerConfigTrader()
	if err := at.enforceMaxPositions(0); err != nil {
		t.Fatalf("0 of 1: %v", err)
	}
	if err := at.enforceMaxPositions(1); err == nil || !strings.Contains(err.Error(), "(1/1)") {
		t.Fatalf("1 of 1 must be blocked at the configured limit, got %v", err)
	}
	at.config.StrategyConfig.RiskControl.MaxPositions = 3
	if err := at.enforceMaxPositions(2); err != nil {
		t.Fatalf("a configured 3 (= the safety ceiling) must be honored: %v", err)
	}
	if n, err := at.maxPositionsLimit(); err != nil || n != 3 {
		t.Fatalf("limit %d %v", n, err)
	}
	// #95 §5.3 item 6: above the distinct safety ceiling (3, carried over
	// from the pre-#82 bound, owner to confirm) opens fail loudly; the
	// value is never clamped silently.
	at.config.StrategyConfig.RiskControl.MaxPositions = 5
	if err := at.enforceMaxPositions(0); err == nil || !errors.Is(err, store.ErrMaxPositionsAboveSafetyCeiling) {
		t.Fatalf("a configured 5 must block opens with the ceiling error, got %v", err)
	}
	if _, err := at.maxPositionsLimit(); !errors.Is(err, store.ErrMaxPositionsAboveSafetyCeiling) {
		t.Fatalf("limit must report the ceiling error, got %v", err)
	}
}

func TestMaxPositionsMissingFailsLoudly(t *testing.T) {
	at := ownerConfigTrader()
	at.config.StrategyConfig.RiskControl.MaxPositions = 0
	err := at.enforceMaxPositions(0)
	if err == nil || !strings.Contains(err.Error(), "max_positions") {
		t.Fatalf("missing max_positions must block with a validation error, got %v", err)
	}
	// The response cap drops every open with the same error.
	kept, dropped, _ := at.capOpensToFreeSlots([]kernel.Decision{{Symbol: "A", Action: "open_long"}, {Symbol: "ALL", Action: "wait"}}, 0)
	if len(dropped) != 1 || len(kept) != 1 || kept[0].Action != "wait" {
		t.Fatalf("kept %+v dropped %+v", kept, dropped)
	}
	record := &store.DecisionRecord{}
	at.capResponseOpens(&kernel.Context{}, []kernel.Decision{{Symbol: "A", Action: "open_long"}}, record)
	if len(record.Decisions) != 1 || !strings.Contains(record.Decisions[0].Error, "max_positions") {
		t.Fatalf("record %+v", record.Decisions)
	}
}

func TestMinConfidenceUsesConfigured75(t *testing.T) {
	at := ownerConfigTrader()
	if got := at.effectiveMinOpenConfidence(75); got != 75 {
		t.Fatalf("effective %d, want 75", got)
	}
	d := &kernel.Decision{Symbol: "DASHUSDT", Action: "open_short", Leverage: 5, PositionSizeUSD: 40, Price: 36.3, StopLoss: 37.03, TakeProfit: 34.85, Confidence: 74}
	if err := at.validateOpenDecision(d, 36.3, "short"); err == nil || !strings.Contains(err.Error(), "below minimum 75") {
		t.Fatalf("74 must fail the configured 75, got %v", err)
	}
}
