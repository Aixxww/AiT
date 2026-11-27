package local

import (
	"encoding/json"
	"testing"
)

// #163 acceptance tests: module_no_match records must carry per-module
// outcomes (module + stage + first failing condition's code) for exactly
// the modules that were evaluated, and no other signal may carry them.

// attributionFixture163 is a symbol no module can turn into a signal:
// pullback_reversal_long matches but scores 15 (< the 30 floor), every
// other module fails Match on an identifiable first condition.
func attributionFixture163() V7SymbolContext {
	return V7SymbolContext{
		Symbol:            "FIXTUREUSDT",
		CurrentPrice:      100,
		ATR4h:             2,
		Low4h:             97.6, // (100-97.6)/2 = 1.2 ATR above the 4h low
		Change24h:         -4,
		BBWidthPercentile: 50,
		TakerBuy15m:       0.5,
		RSI1h:             50,
		// Snapshot deliberately nil.
	}
}

func findNoMatch163(t *testing.T, result V7RouteResult, symbol string) *V7SignalOutput {
	t.Helper()
	for i := range result.RawSignals {
		sig := &result.RawSignals[i]
		if sig.Symbol == symbol && sig.SetupType == V7SetupModuleNoMatch {
			return sig
		}
	}
	t.Fatalf("no module_no_match record for %s in raw signals: %+v", symbol, result.RawSignals)
	return nil
}

func TestModuleAttribution163_NoMatchRecordCarriesOutcomes(t *testing.T) {
	cfg := DefaultV7Config()
	result := NewV7Router().RouteDetailed([]V7SymbolContext{attributionFixture163()}, V7RegimeTrendUp, cfg)

	sig := findNoMatch163(t, result, "FIXTUREUSDT")
	if len(sig.ModuleOutcomes) == 0 {
		t.Fatal("module_no_match record carries no module outcomes")
	}
	// Under trend_up every one of the 22 modules has weight >= 0.4, so all
	// of them were evaluated and must be recorded.
	if len(sig.ModuleOutcomes) != 22 {
		t.Fatalf("outcomes = %d, want 22 (all modules evaluated under trend_up)", len(sig.ModuleOutcomes))
	}

	byModule := make(map[string]V7ModuleOutcome, len(sig.ModuleOutcomes))
	for _, oc := range sig.ModuleOutcomes {
		if oc.Module == "" {
			t.Fatalf("outcome with empty module: %+v", oc)
		}
		switch oc.Stage {
		case V7ModuleStageMatch, V7ModuleStageScore:
			if oc.Code == "" {
				t.Fatalf("non-matched outcome without reason code: %+v", oc)
			}
		case V7ModuleStageMatched:
			if oc.Code != "" {
				t.Fatalf("matched outcome carrying a failure code: %+v", oc)
			}
		default:
			t.Fatalf("outcome with unknown stage: %+v", oc)
		}
		if _, dup := byModule[oc.Module]; dup {
			t.Fatalf("duplicate outcome for module %s", oc.Module)
		}
		byModule[oc.Module] = oc
	}

	want := map[string]V7ModuleOutcome{
		// Matched, but the scaffold floor dropped it: 15 points < 30.
		"pullback_reversal_long":      {Stage: V7ModuleStageScore, Code: v7CodeScoreBelowFloor},
		"trend_breakout_long":         {Stage: V7ModuleStageMatch, Code: "bb_not_compressed"},
		"short_squeeze_long":          {Stage: V7ModuleStageMatch, Code: "snapshot_missing"},
		"volatility_squeeze_breakout": {Stage: V7ModuleStageMatch, Code: "bb_not_compressed"},
		"whale_flow_reversal":         {Stage: V7ModuleStageMatch, Code: "snapshot_missing"},
		"range_reversion":             {Stage: V7ModuleStageMatch, Code: "bb_width_missing"},
		"funding_reversal":            {Stage: V7ModuleStageMatch, Code: "snapshot_missing"},
	}
	for module, w := range want {
		got, ok := byModule[module]
		if !ok {
			t.Fatalf("no outcome recorded for module %s", module)
		}
		if got.Stage != w.Stage || got.Code != w.Code {
			t.Fatalf("module %s outcome = {%s %s}, want {%s %s}", module, got.Stage, got.Code, w.Stage, w.Code)
		}
	}

	raw, err := json.Marshal(sig.ModuleOutcomes)
	if err != nil {
		t.Fatalf("marshal outcomes: %v", err)
	}
	t.Logf("module_outcomes compact JSON for fixture record: %d bytes over %d modules", len(raw), len(sig.ModuleOutcomes))
}

func TestModuleAttribution163_OnlyEvaluatedModulesRecorded(t *testing.T) {
	cfg := DefaultV7Config()
	// Under panic_dump, leader_momentum_long has regime weight 0.1 < 0.2:
	// the router never evaluates it, so it must not appear in the record.
	result := NewV7Router().RouteDetailed([]V7SymbolContext{attributionFixture163()}, V7RegimePanicDump, cfg)

	sig := findNoMatch163(t, result, "FIXTUREUSDT")
	for _, oc := range sig.ModuleOutcomes {
		if oc.Module == "leader_momentum_long" {
			t.Fatalf("unevaluated module recorded: %+v", oc)
		}
	}
	if len(sig.ModuleOutcomes) == 0 || len(sig.ModuleOutcomes) >= 22 {
		t.Fatalf("outcomes = %d, want between 1 and 21 under panic_dump", len(sig.ModuleOutcomes))
	}
}

func TestModuleAttribution163_MatchedSignalsCarryNoOutcomes(t *testing.T) {
	// A symbol that leader_momentum_long turns into a real signal (mirrors
	// the chase-protection fixture in hunter_v7_router_test.go).
	ctx := V7SymbolContext{
		Symbol:       "CHASEUSDT",
		CurrentPrice: 100,
		Change1h:     4,
		Change4h:     9,
		Change24h:    22,
		ATR1h:        2,
		ATR4h:        5,
		High1h:       102,
		Low1h:        96,
		High4h:       104,
		Low4h:        94,
		TakerBuy15m:  0.54,
		RSI5m:        86,
		Snapshot: &SymbolSnapshotData{
			QuoteVolume24h: 80_000_000,
			TradeCount24h:  120_000,
			OI:             20_000_000,
			OIDelta1h:      4,
			OIDelta4h:      12,
			FundingRate:    0.0001,
			LSR:            1.1,
		},
	}
	cfg := DefaultV7Config()
	cfg.MinAIPriority = 1
	cfg.FallbackMinAIPriority = 1
	cfg.MaxOutput = 20

	result := NewV7Router().RouteDetailed([]V7SymbolContext{ctx}, V7RegimeRotation, cfg)

	foundLeader := false
	for _, group := range [][]V7SignalOutput{result.RawSignals, result.ConfirmedSignals, result.OutputSignals} {
		for i := range group {
			sig := &group[i]
			if sig.SetupType == V7SetupModuleNoMatch {
				if sig.Symbol == "CHASEUSDT" {
					t.Fatal("CHASEUSDT produced a signal but also got a module_no_match record")
				}
				continue
			}
			if len(sig.ModuleOutcomes) != 0 {
				t.Fatalf("non-synthetic signal carries module outcomes: %s/%s", sig.Symbol, sig.SetupType)
			}
			if sig.Symbol == "CHASEUSDT" && sig.SetupType == V7SetupLeaderMomentumLong {
				foundLeader = true
			}
		}
	}
	if !foundLeader {
		t.Fatal("leader momentum signal for CHASEUSDT not found; fixture no longer exercises the matched path")
	}
}
