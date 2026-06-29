package local

import "testing"

func TestBuildV7PotentialPoolRanksUnmatchedHighPotentialSymbols(t *testing.T) {
	universe := []V7SymbolContext{
		{
			Symbol:         "BTCUSDT",
			CurrentPrice:   60000,
			Change4h:       1,
			Amplitude24h:   3,
			Velocity5m:     0.2,
			Velocity15m:    0.4,
			VolumeBurst5m:  1.1,
			VolumeBurst15m: 1.2,
		},
		{
			Symbol:         "HOTUSDT",
			CurrentPrice:   1,
			Change4h:       13,
			Amplitude24h:   28,
			Velocity5m:     3.4,
			Velocity15m:    6.2,
			VolumeBurst5m:  3.4,
			VolumeBurst15m: 2.8,
			Snapshot:       &SymbolSnapshotData{OIDelta1h: 14, OIDelta4h: 22, FundingRate: 0.0018, LSR: 1.4},
			TakerBuy15m:    0.57,
		},
		{
			Symbol:         "MIDUSDT",
			CurrentPrice:   2,
			Change4h:       4,
			Amplitude24h:   12,
			Velocity5m:     1.2,
			Velocity15m:    2,
			VolumeBurst5m:  1.7,
			VolumeBurst15m: 1.6,
			Snapshot:       &SymbolSnapshotData{OIDelta1h: 5},
		},
	}
	raw := []V7SignalOutput{
		{Symbol: "MIDUSDT", SetupType: V7SetupLeaderMomentumLong},
		{Symbol: "HOTUSDT", SetupType: V7SetupModuleNoMatch},
	}

	pool := BuildV7PotentialPool(universe, raw, 2)
	if len(pool) != 2 {
		t.Fatalf("pool len = %d, want 2", len(pool))
	}
	if pool[0].Symbol != "HOTUSDT" {
		t.Fatalf("top symbol = %s, want HOTUSDT: %+v", pool[0].Symbol, pool)
	}
	if pool[0].MatchedModule {
		t.Fatalf("HOTUSDT should be unmatched despite module_no_match raw signal: %+v", pool[0])
	}
	if !pool[1].MatchedModule || len(pool[1].MatchedSetups) != 1 {
		t.Fatalf("MIDUSDT should carry matched setup metadata: %+v", pool[1])
	}
	if pool[0].OpportunityPotentialScore <= pool[1].OpportunityPotentialScore {
		t.Fatalf("expected HOTUSDT score > MIDUSDT score: %+v", pool)
	}
	if len(pool[0].TrackingWindows) != 2 || pool[0].TrackingWindows[0] != "30m" || pool[0].TrackingWindows[1] != "60m" {
		t.Fatalf("tracking windows missing: %+v", pool[0].TrackingWindows)
	}
}

func TestBuildV7PotentialPoolDefaultsToTop20(t *testing.T) {
	universe := make([]V7SymbolContext, 0, 25)
	for i := 0; i < 25; i++ {
		universe = append(universe, V7SymbolContext{
			Symbol:         string(rune('A'+i)) + "USDT",
			CurrentPrice:   1,
			Change4h:       float64(i),
			Amplitude24h:   25,
			Velocity5m:     2,
			Velocity15m:    3,
			VolumeBurst5m:  2,
			VolumeBurst15m: 2,
		})
	}

	pool := BuildV7PotentialPool(universe, nil, 0)
	if len(pool) != 20 {
		t.Fatalf("pool len = %d, want default top 20", len(pool))
	}
}
