package kernel

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/engine"
	"github.com/Aixxww/AiT/store"
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

	inclusive, err := NewSnapshotEngine(cfg, datafetch.CollectorConfig{IncludeNonCryptoFutures: true})
	if err != nil {
		t.Fatalf("NewSnapshotEngine inclusive: %v", err)
	}
	if inclusive.GetStore() == first.GetStore() {
		t.Fatalf("expected non-crypto futures inclusion to create a separate SnapshotStore")
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

func TestSnapshotEngineWaitForSnapshotWaitsForInitialFetch(t *testing.T) {
	snapshotEngine := newSnapshotEngine(engine.DefaultHubConfig(), datafetch.CollectorConfig{})

	go func() {
		time.Sleep(50 * time.Millisecond)
		snapshotEngine.dataStore.Swap(&datafetch.Snapshot{
			Symbols: map[string]*datafetch.SymbolSnapshot{
				"BTCUSDT": {Symbol: "BTCUSDT", Price: 100},
			},
			CreatedAt: time.Now(),
		})
	}()

	snap := snapshotEngine.WaitForSnapshot(time.Second)
	if snap == nil {
		t.Fatalf("expected delayed snapshot, got nil")
	}
	if len(snap.Symbols) != 1 {
		t.Fatalf("snapshot symbols = %d, want 1", len(snap.Symbols))
	}
}

func TestSnapshotEngineWaitForFreshSnapshotSkipsStaleSnapshot(t *testing.T) {
	snapshotEngine := newSnapshotEngine(engine.DefaultHubConfig(), datafetch.CollectorConfig{})
	snapshotEngine.dataStore.Swap(&datafetch.Snapshot{
		Symbols: map[string]*datafetch.SymbolSnapshot{
			"OLDUSDT": {Symbol: "OLDUSDT", Price: 1},
		},
		CreatedAt: time.Now().Add(-time.Minute),
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		snapshotEngine.dataStore.Swap(&datafetch.Snapshot{
			Symbols: map[string]*datafetch.SymbolSnapshot{
				"NEWUSDT": {Symbol: "NEWUSDT", Price: 2},
			},
			CreatedAt: time.Now(),
		})
	}()

	snap := snapshotEngine.WaitForFreshSnapshot(time.Second, 500*time.Millisecond)
	if snap == nil {
		t.Fatalf("expected fresh snapshot, got nil")
	}
	if _, ok := snap.Symbols["NEWUSDT"]; !ok {
		t.Fatalf("expected fresh NEWUSDT snapshot, got %#v", snap.Symbols)
	}
}

func TestGetCandidateCoinsWithSnapshotErrorsWhenInitialSnapshotMissing(t *testing.T) {
	oldTimeout := snapshotReadyTimeout
	snapshotReadyTimeout = 50 * time.Millisecond
	t.Cleanup(func() { snapshotReadyTimeout = oldTimeout })

	strategyCfg := store.GetDefaultStrategyConfig("en")
	strategyCfg.CoinSource.SourceType = "hunter_v7"
	strategyEngine := NewStrategyEngine(&strategyCfg)
	snapshotEngine := newSnapshotEngine(engine.DefaultHubConfig(), datafetch.CollectorConfig{})
	strategyEngine.SetSnapshotEngine(snapshotEngine)

	_, err := strategyEngine.GetCandidateCoinsWithSnapshot()
	if err == nil {
		t.Fatalf("expected missing snapshot error")
	}
	if got := err.Error(); got != "snapshot not ready after waiting for initial DataCollector fetch" {
		t.Fatalf("error = %q", got)
	}
}

func TestGetCandidateCoinsWithSnapshotErrorsWhenSnapshotStale(t *testing.T) {
	oldTimeout := snapshotReadyTimeout
	snapshotReadyTimeout = 50 * time.Millisecond
	t.Cleanup(func() { snapshotReadyTimeout = oldTimeout })

	strategyCfg := store.GetDefaultStrategyConfig("en")
	strategyCfg.CoinSource.SourceType = "hunter_v7"
	strategyEngine := NewStrategyEngine(&strategyCfg)
	snapshotEngine := newSnapshotEngine(engine.DefaultHubConfig(), datafetch.CollectorConfig{})
	snapshotEngine.source.maxAge = 50 * time.Millisecond
	strategyEngine.SetSnapshotEngine(snapshotEngine)
	snapshotEngine.dataStore.Swap(&datafetch.Snapshot{
		Symbols: map[string]*datafetch.SymbolSnapshot{
			"BTCUSDT": {Symbol: "BTCUSDT", Price: 100},
		},
		CreatedAt: time.Now().Add(-time.Minute),
	})

	_, err := strategyEngine.GetCandidateCoinsWithSnapshot()
	if err == nil {
		t.Fatalf("expected stale snapshot error")
	}
	if got := err.Error(); got != "snapshot stale after waiting for DataCollector refresh" {
		t.Fatalf("error = %q", got)
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
