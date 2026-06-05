package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/engine"
	"github.com/Aixxww/AiT/logger"
	"sync"
	"time"
)

// SnapshotEngine wraps the new engine.MainEngine to provide signals
// compatible with the existing AutoTrader flow.
type SnapshotEngine struct {
	mainEngine *engine.MainEngine
	dataStore  *datafetch.Store
	source     *snapshotDataSource

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
}

type snapshotDataSource struct {
	store     *datafetch.Store
	collector *datafetch.DataCollector
	maxAge    time.Duration

	mu      sync.Mutex
	started bool
	refs    int
}

var snapshotDataRegistry = struct {
	sync.Mutex
	sources map[string]*snapshotDataSource
}{
	sources: make(map[string]*snapshotDataSource),
}

// NewSnapshotEngine creates a SnapshotEngine with the given configs.
// The market data source is shared in-process by data config, while each
// SnapshotEngine keeps its own scoring engine so strategies can use different
// HubConfig values without duplicating Binance REST/WS collection.
func NewSnapshotEngine(cfg engine.HubConfig, dataCfg datafetch.CollectorConfig) (*SnapshotEngine, error) {
	source, err := getSnapshotDataSource(dataCfg)
	if err != nil {
		return nil, err
	}

	return newSnapshotEngineWithSource(cfg, source), nil
}

func newSnapshotEngine(cfg engine.HubConfig, dataCfg datafetch.CollectorConfig) *SnapshotEngine {
	source := newSnapshotDataSource(dataCfg)
	return newSnapshotEngineWithSource(cfg, source)
}

func newSnapshotEngineWithSource(cfg engine.HubConfig, source *snapshotDataSource) *SnapshotEngine {
	mainEngine := engine.NewMainEngine(source.store, cfg)
	return &SnapshotEngine{
		mainEngine: mainEngine,
		dataStore:  source.store,
		source:     source,
	}
}

func getSnapshotDataSource(dataCfg datafetch.CollectorConfig) (*snapshotDataSource, error) {
	key, err := snapshotDataSourceKey(dataCfg)
	if err != nil {
		return nil, err
	}

	snapshotDataRegistry.Lock()
	defer snapshotDataRegistry.Unlock()
	if source, ok := snapshotDataRegistry.sources[key]; ok {
		return source, nil
	}

	source := newSnapshotDataSource(dataCfg)
	snapshotDataRegistry.sources[key] = source
	return source, nil
}

func newSnapshotDataSource(dataCfg datafetch.CollectorConfig) *snapshotDataSource {
	dataCfg = normalizeCollectorConfig(dataCfg)
	store := datafetch.NewStore()
	collector := datafetch.NewDataCollector(dataCfg, store)
	return &snapshotDataSource{
		store:     store,
		collector: collector,
		maxAge:    dataCfg.RestInterval * 2,
	}
}

func snapshotDataSourceKey(dataCfg datafetch.CollectorConfig) (string, error) {
	keyPayload := normalizeCollectorConfig(dataCfg)
	b, err := json.Marshal(keyPayload)
	if err != nil {
		return "", fmt.Errorf("marshal snapshot data source key: %w", err)
	}
	return string(b), nil
}

func normalizeCollectorConfig(cfg datafetch.CollectorConfig) datafetch.CollectorConfig {
	if cfg.BinanceURL == "" {
		cfg.BinanceURL = "https://fapi.binance.com"
	}
	if cfg.BinanceWSURL == "" {
		cfg.BinanceWSURL = "wss://fstream.binance.com"
	}
	if cfg.RestInterval <= 0 {
		cfg.RestInterval = 30 * time.Second
	}
	if cfg.SocialInterval <= 0 {
		cfg.SocialInterval = 15 * time.Minute
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 50
	}
	if cfg.TopNForDetail <= 0 {
		cfg.TopNForDetail = 100
	}
	if cfg.TopNForWS <= 0 {
		cfg.TopNForWS = 30
	}
	return cfg
}

// Start launches data collection and scoring engine.
func (se *SnapshotEngine) Start(ctx context.Context) {
	if se == nil {
		return
	}

	se.mu.Lock()
	if se.started {
		se.mu.Unlock()
		logger.Info("✅ SnapshotEngine scoring already running")
		return
	}
	runCtx, cancel := context.WithCancel(context.Background())
	se.started = true
	se.cancel = cancel
	se.mu.Unlock()

	se.source.Start()
	go se.mainEngine.Start(runCtx, 30*time.Second)
	if ctx != nil {
		if done := ctx.Done(); done != nil {
			go func() {
				select {
				case <-done:
					se.Stop()
				case <-runCtx.Done():
				}
			}()
		}
	}

	logger.Info("✅ SnapshotEngine started (shared data source + strategy scoring engine)")
}

// Stop releases this engine's reference to the shared data source. The shared
// collector is process-lifetime so one trader cannot cancel another trader's
// data feed.
func (se *SnapshotEngine) Stop() {
	if se == nil {
		return
	}
	se.mu.Lock()
	if !se.started {
		se.mu.Unlock()
		return
	}
	cancel := se.cancel
	se.cancel = nil
	se.started = false
	se.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	se.source.Release()
}

func (s *snapshotDataSource) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.refs++
	if s.started {
		refs := s.refs
		s.mu.Unlock()
		logger.Infof("✅ Snapshot data source already running (shared refs=%d)", refs)
		return
	}
	s.started = true
	refs := s.refs
	s.mu.Unlock()

	go s.collector.Start(context.Background())
	logger.Infof("✅ Snapshot data source started (shared refs=%d)", refs)
}

func (s *snapshotDataSource) Release() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.refs > 0 {
		s.refs--
	}
	refs := s.refs
	s.mu.Unlock()
	logger.Infof("✅ Snapshot data source released (shared refs=%d)", refs)
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

// MaxSnapshotAge returns the freshness guard for REST-backed snapshots.
func (se *SnapshotEngine) MaxSnapshotAge() time.Duration {
	if se == nil || se.source == nil || se.source.maxAge <= 0 {
		return time.Minute
	}
	return se.source.maxAge
}

// WaitForSnapshot waits briefly for the collector's first snapshot.
// Startup cycles can race the initial Binance REST fetch; without this wait,
// snapshot-based sources are reported as "no candidates" before data exists.
func (se *SnapshotEngine) WaitForSnapshot(timeout time.Duration) *datafetch.Snapshot {
	if se == nil || se.dataStore == nil {
		return nil
	}
	if snap := se.GetSnapshot(); snap != nil && len(snap.Symbols) > 0 {
		return snap
	}
	if timeout <= 0 {
		return nil
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if snap := se.GetSnapshot(); snap != nil && len(snap.Symbols) > 0 {
				return snap
			}
			if time.Now().After(deadline) {
				return nil
			}
		}
	}
}

// WaitForFreshSnapshot waits for a non-empty snapshot that is recent enough for
// a live trading decision. If REST refresh stalls, callers must fail the cycle
// instead of scoring stale market data.
func (se *SnapshotEngine) WaitForFreshSnapshot(timeout, maxAge time.Duration) *datafetch.Snapshot {
	if se == nil || se.dataStore == nil {
		return nil
	}
	if snap := se.GetSnapshot(); snapshotIsFresh(snap, maxAge) {
		return snap
	}
	if timeout <= 0 {
		return nil
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if snap := se.GetSnapshot(); snapshotIsFresh(snap, maxAge) {
				return snap
			}
			if time.Now().After(deadline) {
				return nil
			}
		}
	}
}

func snapshotIsFresh(snap *datafetch.Snapshot, maxAge time.Duration) bool {
	if snap == nil || len(snap.Symbols) == 0 || snap.CreatedAt.IsZero() {
		return false
	}
	if maxAge <= 0 {
		return true
	}
	return time.Since(snap.CreatedAt) <= maxAge
}

// GetEngine returns the underlying MainEngine (for diagnostics).
func (se *SnapshotEngine) GetEngine() *engine.MainEngine {
	return se.mainEngine
}
