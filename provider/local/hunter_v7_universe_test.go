package local

import (
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

func TestComputeBBWidthPercentileTreatsTightCurrentBandAsLowPercentile(t *testing.T) {
	var bars []klineBar
	for i := 0; i < 40; i++ {
		close := 100.0
		if i%2 == 0 {
			close = 92
		} else {
			close = 108
		}
		bars = append(bars, klineBar{Open: close, High: close + 2, Low: close - 2, Close: close, Volume: 1000})
	}
	for i := 0; i < 25; i++ {
		close := 100.0 + float64(i%3-1)*0.15
		bars = append(bars, klineBar{Open: close, High: close + 0.2, Low: close - 0.2, Close: close, Volume: 1000})
	}

	got := computeBBWidthPercentile(bars)
	if got >= 25 {
		t.Fatalf("tight current BB width percentile = %.2f, want < 25", got)
	}
}

func TestBuildV7UniverseKeepsHighAmplitudeWithoutOIDetail(t *testing.T) {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{
		"AMPUSDT": {
			Symbol:         "AMPUSDT",
			Price:          1,
			QuoteVolume24h: 1_000_000,
			HighPrice24h:   1.50,
			LowPrice24h:    1.00,
			OI:             0,
		},
		"QUIETUSDT": {
			Symbol:         "QUIETUSDT",
			Price:          1,
			QuoteVolume24h: 1_000_000,
			HighPrice24h:   1.05,
			LowPrice24h:    1.00,
			OI:             0,
		},
	}}

	universe := BuildV7Universe(snap)
	if !hasUniverseSymbol(universe, "AMPUSDT") {
		t.Fatalf("expected high-amplitude symbol without OI detail to stay in universe, got %+v", universe)
	}
	if hasUniverseSymbol(universe, "QUIETUSDT") {
		t.Fatalf("expected quiet symbol without OI detail to be excluded, got %+v", universe)
	}
}

func TestBuildV7UniverseKeepsHighDrawdownWithoutOIDetail(t *testing.T) {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{
		"SKYAIUSDT": {
			Symbol:         "SKYAIUSDT",
			Price:          0.03018,
			QuoteVolume24h: 1_000_000,
			HighPrice24h:   0.03300,
			LowPrice24h:    0.03018,
			PriceChange24h: -2.14,
			OI:             0,
		},
		"FLATUSDT": {
			Symbol:         "FLATUSDT",
			Price:          1.00,
			QuoteVolume24h: 1_000_000,
			HighPrice24h:   1.04,
			LowPrice24h:    0.99,
			PriceChange24h: -1.0,
			OI:             0,
		},
	}}

	universe := BuildV7Universe(snap)
	if !hasUniverseSymbol(universe, "SKYAIUSDT") {
		t.Fatalf("expected high-drawdown symbol without OI detail to stay in universe, got %+v", universe)
	}
	if hasUniverseSymbol(universe, "FLATUSDT") {
		t.Fatalf("expected flat symbol without OI detail to be excluded, got %+v", universe)
	}
}

func TestBuildV7UniverseConvertsOIQuantityToNotional(t *testing.T) {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{
		"OIUSDT": {
			Symbol:         "OIUSDT",
			Price:          2,
			QuoteVolume24h: 1_000_000,
			HighPrice24h:   2.10,
			LowPrice24h:    1.90,
			OI:             500_000,
		},
	}}

	universe := BuildV7Universe(snap)
	for _, ctx := range universe {
		if ctx.Symbol == "OIUSDT" {
			if ctx.Snapshot == nil || ctx.Snapshot.OI != 1_000_000 {
				t.Fatalf("v7 OI = %.2f, want notional 1000000", ctx.Snapshot.OI)
			}
			return
		}
	}
	t.Fatalf("OIUSDT not found in universe")
}

func TestBuildV7UniverseKeepsVelocityMoverWithoutOIDetail(t *testing.T) {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{
		"FASTUSDT": {
			Symbol:         "FASTUSDT",
			Price:          1.05,
			QuoteVolume24h: 500_000,
			HighPrice24h:   1.06,
			LowPrice24h:    1.00,
			OI:             0,
			Klines: map[string][]datafetch.Kline{
				"5m": {
					testKline(1.00, 100),
					testKline(1.03, 120),
					testKline(1.06, 140),
				},
			},
		},
	}}

	universe := BuildV7Universe(snap)
	for _, ctx := range universe {
		if ctx.Symbol == "FASTUSDT" {
			if ctx.PoolType != V7PoolVelocity {
				t.Fatalf("pool = %s, want %s", ctx.PoolType, V7PoolVelocity)
			}
			if ctx.Velocity5m <= 2 {
				t.Fatalf("velocity5m = %.2f, want > 2", ctx.Velocity5m)
			}
			return
		}
	}
	t.Fatalf("FASTUSDT not found in universe")
}

func TestBuildV7UniverseKeepsNewActivityMoverWithoutOIDetail(t *testing.T) {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{
		"ACTIVEUSDT": {
			Symbol:         "ACTIVEUSDT",
			Price:          1.01,
			QuoteVolume24h: 300_000,
			HighPrice24h:   1.02,
			LowPrice24h:    1.00,
			OI:             0,
			Klines: map[string][]datafetch.Kline{
				"5m": {
					testKline(1.00, 100),
					testKline(1.00, 100),
					testKline(1.00, 100),
					testKline(1.01, 700),
				},
			},
		},
	}}

	universe := BuildV7Universe(snap)
	for _, ctx := range universe {
		if ctx.Symbol == "ACTIVEUSDT" {
			if ctx.PoolType != V7PoolNewActivity {
				t.Fatalf("pool = %s, want %s", ctx.PoolType, V7PoolNewActivity)
			}
			if ctx.VolumeBurst5m < 3 {
				t.Fatalf("volume burst 5m = %.2f, want >= 3", ctx.VolumeBurst5m)
			}
			return
		}
	}
	t.Fatalf("ACTIVEUSDT not found in universe")
}

func TestBuildV7AttributionSummaryCountsFunnel(t *testing.T) {
	summary := buildV7AttributionSummary(
		[]V7SymbolContext{
			{Symbol: "AUSDT", PoolType: V7PoolVelocity},
			{Symbol: "BUSDT", PoolType: V7PoolNewActivity},
		},
		[]V7SignalOutput{
			{
				Symbol:           "AUSDT",
				SetupType:        V7SetupPanicReversalLong,
				Status:           V7StatusCandidate,
				ExecutionQuality: V7ExecReady,
			},
			{
				Symbol:           "BUSDT",
				SetupType:        V7SetupPreBreakoutWatch,
				Status:           V7StatusWaitConfirm,
				ExecutionQuality: V7ExecWatchOnly,
			},
		},
		[]V7SignalOutput{
			{Symbol: "AUSDT", SetupType: V7SetupPanicReversalLong},
		},
	)

	if summary.UniverseTotal != 2 {
		t.Fatalf("universe total = %d, want 2", summary.UniverseTotal)
	}
	if summary.PoolCounts[string(V7PoolVelocity)] != 1 || summary.PoolCounts[string(V7PoolNewActivity)] != 1 {
		t.Fatalf("pool counts = %+v", summary.PoolCounts)
	}
	if summary.SetupCounts[string(V7SetupPanicReversalLong)] != 1 ||
		summary.StatusCounts[string(V7StatusWaitConfirm)] != 1 ||
		summary.QualityCounts[string(V7ExecReady)] != 1 ||
		summary.OutputCounts[string(V7SetupPanicReversalLong)] != 1 {
		t.Fatalf("unexpected attribution counts: %+v", summary)
	}
}

func testKline(close, volume float64) datafetch.Kline {
	return datafetch.Kline{
		Open:     close,
		High:     close,
		Low:      close,
		Close:    close,
		Volume:   volume,
		TakerBuy: volume * 0.55,
	}
}

func hasUniverseSymbol(universe []V7SymbolContext, symbol string) bool {
	for _, ctx := range universe {
		if ctx.Symbol == symbol {
			return true
		}
	}
	return false
}
