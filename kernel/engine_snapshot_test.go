package kernel

import (
	"context"
	"math"
	"testing"
	"time"

	"nofx/datafetch"
	"nofx/engine"
	"nofx/store"
)

func TestSnapshotEngineRegistryReusesDataSource(t *testing.T) {
	snapshotDataRegistry.Lock()
	snapshotDataRegistry.sources = make(map[string]*snapshotDataSource)
	snapshotDataRegistry.Unlock()

	cfg := engine.DefaultHubConfig()
	first, err := NewSnapshotEngine(cfg, datafetch.CollectorConfig{})
	if err != nil {
		t.Fatalf("NewSnapshotEngine first: %v", err)
	}
	otherCfg := cfg
	otherCfg.MinScore = cfg.MinScore + 7
	second, err := NewSnapshotEngine(otherCfg, datafetch.CollectorConfig{})
	if err != nil {
		t.Fatalf("NewSnapshotEngine second: %v", err)
	}
	if first == second {
		t.Fatalf("expected different HubConfig to create a separate SnapshotEngine")
	}
	if first.GetStore() != second.GetStore() {
		t.Fatalf("expected same data config to reuse SnapshotStore")
	}

	other, err := NewSnapshotEngine(cfg, datafetch.CollectorConfig{LunarCrushKey: "different"})
	if err != nil {
		t.Fatalf("NewSnapshotEngine other: %v", err)
	}
	if other.GetStore() == first.GetStore() {
		t.Fatalf("expected different data config to create a separate SnapshotStore")
	}
}

func TestBuildMarketDataFromSnapshotUsesSharedStore(t *testing.T) {
	strategyCfg := store.GetDefaultStrategyConfig("en")
	strategyEngine := NewStrategyEngine(&strategyCfg)
	snapshotEngine := newSnapshotEngine(engine.DefaultHubConfig(), datafetch.CollectorConfig{})
	strategyEngine.SetSnapshotEngine(snapshotEngine)

	price := 120.0
	snapshotEngine.dataStore.Swap(&datafetch.Snapshot{
		Symbols: map[string]*datafetch.SymbolSnapshot{
			"BTCUSDT": {
				Symbol:      "BTCUSDT",
				Price:       price,
				OI:          123,
				FundingRate: 0.0001,
				Klines: map[string][]datafetch.Kline{
					"5m": makeSnapshotKlines(40, 100),
					"1h": makeSnapshotKlines(40, 90),
				},
			},
		},
		CreatedAt: time.Now(),
	})

	data, err := buildMarketDataFromSnapshot(strategyEngine, "BTC", []string{"5m", "1h"}, "5m", 30)
	if err != nil {
		t.Fatalf("buildMarketDataFromSnapshot: %v", err)
	}
	if data.Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %s, want BTCUSDT", data.Symbol)
	}
	if data.CurrentPrice != price {
		t.Fatalf("current price = %v, want %v", data.CurrentPrice, price)
	}
	if data.FundingRate != 0.0001 {
		t.Fatalf("funding rate = %v, want 0.0001", data.FundingRate)
	}
	if data.TimeframeData["5m"] == nil || data.TimeframeData["1h"] == nil {
		t.Fatalf("expected 5m and 1h timeframe data")
	}
	if data.OpenInterest == nil || math.Abs(data.OpenInterest.Latest-123) > 1e-9 {
		t.Fatalf("open interest latest = %v, want 123", data.OpenInterest)
	}
}

func TestSnapshotEngineStopIsIdempotent(t *testing.T) {
	source := newSnapshotDataSource(datafetch.CollectorConfig{})
	source.started = true
	source.refs = 1
	snapshotEngine := newSnapshotEngineWithSource(engine.DefaultHubConfig(), source)

	ctx, cancel := context.WithCancel(context.Background())
	snapshotEngine.started = true
	snapshotEngine.cancel = cancel

	snapshotEngine.Stop()
	snapshotEngine.Stop()

	if source.refs != 0 {
		t.Fatalf("source refs = %d, want 0", source.refs)
	}
	if snapshotEngine.started {
		t.Fatalf("snapshot engine still marked started after Stop")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatalf("snapshot engine Stop did not cancel scoring context")
	}
}

func makeSnapshotKlines(n int, start float64) []datafetch.Kline {
	klines := make([]datafetch.Kline, 0, n)
	base := time.Now().Add(-time.Duration(n) * time.Minute)
	for i := 0; i < n; i++ {
		open := start + float64(i)
		close := open + 0.5
		klines = append(klines, datafetch.Kline{
			OpenTime:  base.Add(time.Duration(i) * time.Minute).UnixMilli(),
			CloseTime: base.Add(time.Duration(i+1) * time.Minute).UnixMilli(),
			Open:      open,
			High:      close + 1,
			Low:       open - 1,
			Close:     close,
			Volume:    1000 + float64(i),
			TakerBuy:  500 + float64(i),
		})
	}
	return klines
}
