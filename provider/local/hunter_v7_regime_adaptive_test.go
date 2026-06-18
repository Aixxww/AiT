package local

import "testing"

func TestRegimeAdaptiveEngineRecalculateWeightsFromInputGatesLowSamples(t *testing.T) {
	engine := NewRegimeAdaptiveEngine()
	base := engine.GetEffectiveWeight(V7RegimeCompression, V7SetupVolatilitySqueeze)

	engine.RecalculateWeightsFromInput([]SetupRegimeStatsInput{
		{
			Regime:  string(V7RegimeCompression),
			Setup:   string(V7SetupVolatilitySqueeze),
			Samples: 2,
			WinRate: 80,
			AvgPnL:  3,
		},
	})
	if got := engine.GetEffectiveWeight(V7RegimeCompression, V7SetupVolatilitySqueeze); got != base {
		t.Fatalf("weight changed on low samples: got %v want %v", got, base)
	}

	engine.RecalculateWeightsFromInput([]SetupRegimeStatsInput{
		{
			Regime:  string(V7RegimeCompression),
			Setup:   string(V7SetupVolatilitySqueeze),
			Samples: 8,
			WinRate: 70,
			AvgPnL:  2.5,
		},
	})
	if got := engine.GetEffectiveWeight(V7RegimeCompression, V7SetupVolatilitySqueeze); got <= base {
		t.Fatalf("weight = %v, want > base %v after high-quality samples", got, base)
	}
}

func TestRegimeAdaptiveEngineGetAdjustmentReport(t *testing.T) {
	engine := NewRegimeAdaptiveEngine()
	engine.RecalculateWeightsFromInput([]SetupRegimeStatsInput{
		{
			Regime:  string(V7RegimeCompression),
			Setup:   string(V7SetupVolatilitySqueeze),
			Samples: 8,
			WinRate: 70,
			AvgPnL:  2.5,
		},
	})

	report := engine.GetAdjustmentReport()
	if len(report) == 0 {
		t.Fatalf("expected non-empty adjustment report")
	}
	found := false
	for _, row := range report {
		if row.Regime == string(V7RegimeCompression) && row.Setup == string(V7SetupVolatilitySqueeze) {
			found = true
			if row.Adjustment <= 0 || row.EffectiveWeight <= row.BaseWeight {
				t.Fatalf("unexpected dry-run row: %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("expected compression/squeeze row in report")
	}
}
