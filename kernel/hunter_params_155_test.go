package kernel

import (
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/store"
)

// #155: the snapshot scorer the kernel runs for source_type hunter reads the
// strategy's Hunter advanced-filter settings (min_trade_count as the probe).
func TestHunter155KernelPassesSettingsToSnapshotScorer(t *testing.T) {
	kl := func(n int, start, end, vol float64) []datafetch.Kline {
		out := make([]datafetch.Kline, n)
		for i := range out {
			c := start + (end-start)*float64(i)/float64(n-1)
			out[i] = datafetch.Kline{Open: c, High: c + 0.5, Low: c - 0.5, Close: c, Volume: vol, TakerBuy: vol / 2}
		}
		return out
	}
	snap := &datafetch.Snapshot{CreatedAt: time.Now(), Symbols: map[string]*datafetch.SymbolSnapshot{
		"KRNUSDT": {
			Symbol: "KRNUSDT", Price: 100, PriceChange24h: 2, QuoteVolume24h: 50_000_000, Volume24h: 500_000,
			TradeCount24h: 200_000, OI: 20_000_000, OISpikeData: []float64{1, 1, 1, 1, 3, 3, 3, 3},
			LongShortRatio: 1.2, LSRPrev: 1.0,
			Klines: map[string][]datafetch.Kline{
				"5m": kl(50, 101, 100, 1000), "15m": kl(50, 103, 100, 1000),
				"1h": kl(20, 106, 100, 10_000), "4h": kl(20, 110, 100, 10_000), "1d": kl(20, 130, 100, 10_000),
			},
		},
	}}
	run := func(h *store.HunterConfig) int {
		cfg := store.GetDefaultStrategyConfig("en")
		cfg.CoinSource.SourceType = "hunter"
		cfg.CoinSource.Hunter = h
		got, err := NewStrategyEngine(&cfg).scoreFromSnapshot(snap)
		if err != nil {
			t.Fatal(err)
		}
		return len(got)
	}
	if run(nil) != 1 || run(&store.HunterConfig{MinTradeCount: 100_000}) != 1 {
		t.Fatal("fixture must be selected by default and with a 100k floor")
	}
	if run(&store.HunterConfig{MinTradeCount: 300_000}) != 0 {
		t.Fatal("min_trade_count 300k must drop the 200k-trade symbol through the kernel path")
	}
}
