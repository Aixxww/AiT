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

func hasUniverseSymbol(universe []V7SymbolContext, symbol string) bool {
	for _, ctx := range universe {
		if ctx.Symbol == symbol {
			return true
		}
	}
	return false
}
