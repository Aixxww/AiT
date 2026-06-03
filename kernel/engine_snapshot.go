package kernel

import (
	"context"
	"fmt"
	"nofx/datafetch"
	"nofx/engine"
	"nofx/logger"
	"time"
)

// SnapshotEngine wraps the new engine.MainEngine to provide signals
// compatible with the existing AutoTrader flow.
type SnapshotEngine struct {
	mainEngine *engine.MainEngine
	dataStore  *datafetch.Store
	collector  *datafetch.DataCollector
}

// NewSnapshotEngine creates a SnapshotEngine with the given configs.
func NewSnapshotEngine(cfg engine.HubConfig, dataCfg datafetch.CollectorConfig) (*SnapshotEngine, error) {
	store := datafetch.NewStore()
	collector := datafetch.NewDataCollector(dataCfg, store)
	mainEngine := engine.NewMainEngine(store, cfg)
	return &SnapshotEngine{
		mainEngine: mainEngine,
		dataStore:  store,
		collector:  collector,
	}, nil
}

// Start launches data collection and scoring engine.
func (se *SnapshotEngine) Start(ctx context.Context) {
	se.collector.Start(ctx)
	se.mainEngine.Start(ctx, 30*time.Second)
	logger.Info("✅ SnapshotEngine started (data collection + scoring engine)")
}

// GetTradeSignals returns the latest trade signals from the engine.
func (se *SnapshotEngine) GetTradeSignals() []*engine.TradeSignal {
	return se.mainEngine.GetLatestSignals()
}

// ConvertSignalsToCandidateCoins converts TradeSignals to the existing CandidateCoin
// format for backward compatibility with the AutoTrader's buildTradingContext.
func (se *SnapshotEngine) ConvertSignalsToCandidateCoins(signals []*engine.TradeSignal) []CandidateCoin {
	var coins []CandidateCoin
	for _, sig := range signals {
		if sig.Direction == 0 { // NEUTRAL, skip
			continue
		}
		dirStr := "LONG"
		if sig.Direction < 0 {
			dirStr = "SHORT"
		}
		sources := []string{fmt.Sprintf("indicator_hub_%s", sig.Grade.String())}
		coins = append(coins, CandidateCoin{
			Symbol:      sig.Symbol,
			Sources:     sources,
			Direction:   dirStr,
			TradeSignal: sig,
		})
	}
	return coins
}

// GetStore returns the underlying data store (for diagnostics).
func (se *SnapshotEngine) GetStore() *datafetch.Store {
	return se.dataStore
}

// GetSnapshot returns the latest datafetch.Snapshot from the atomic store.
// Used by StrategyEngine to feed snapshot-based Hunter/AI500 scorers.
func (se *SnapshotEngine) GetSnapshot() *datafetch.Snapshot {
	return se.dataStore.Current()
}

// GetEngine returns the underlying MainEngine (for diagnostics).
func (se *SnapshotEngine) GetEngine() *engine.MainEngine {
	return se.mainEngine
}
