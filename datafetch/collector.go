package datafetch

import (
	"context"
	"log"
	"time"
)

// CollectorConfig holds configuration for the DataCollector.
type CollectorConfig struct {
	BinanceURL     string // "https://fapi.binance.com"
	BinanceWSURL   string // "wss://fstream.binance.com"
	LunarCrushKey  string
	LunarCrushBase string        // "https://lunarcrush.com/api4"
	RestInterval   time.Duration // 30s
	SocialInterval time.Duration // 15min
	MaxWorkers     int           // 50
	TopNForDetail  int           // 100
	TopNForWS      int           // 30 (top symbols for WS kline/aggTrade streams)
}

// DataCollector is the main orchestrator that manages REST fetching,
// WebSocket updates, and social data collection.
type DataCollector struct {
	cfg     CollectorConfig
	store   *Store
	fetcher *DataFetcher
	ws      *WSManager
	social  *LunarCrushClient
}

// NewDataCollector creates a new DataCollector.
func NewDataCollector(cfg CollectorConfig, store *Store) *DataCollector {
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

	fetcher := NewDataFetcher(FetcherConfig{
		BinanceURL:    cfg.BinanceURL,
		MaxWorkers:    cfg.MaxWorkers,
		TopNForDetail: cfg.TopNForDetail,
	})

	ws := NewWSManager(cfg.BinanceWSURL, store, cfg.TopNForWS)

	var socialClient *LunarCrushClient
	if cfg.LunarCrushKey != "" {
		socialClient = NewLunarCrushClient(cfg.LunarCrushKey, cfg.LunarCrushBase)
	}

	return &DataCollector{
		cfg:     cfg,
		store:   store,
		fetcher: fetcher,
		ws:      ws,
		social:  socialClient,
	}
}

// Start launches all data collection goroutines:
// 1. Initial REST fetch (blocking, waits for first snapshot)
// 2. Background REST refresh loop (every RestInterval)
// 3. Background WebSocket (connects after first snapshot)
// 4. Background Social refresh (every SocialInterval)
func (dc *DataCollector) Start(ctx context.Context) {
	// 1. Initial REST fetch — blocking
	log.Println("datafetch: performing initial REST fetch...")
	snap, err := dc.fetcher.Fetch(ctx)
	if err != nil {
		log.Printf("datafetch: initial fetch error: %v", err)
		// Still try to continue with whatever we got
		if snap == nil {
			snap = &Snapshot{
				Symbols:   make(map[string]*SymbolSnapshot),
				CreatedAt: time.Now(),
				Meta:      SnapshotMeta{RestErrors: 1},
			}
		}
	}
	dc.store.Swap(snap)
	log.Printf("datafetch: initial snapshot: %d symbols, %v", snap.Meta.SymbolCount, snap.Meta.FetchDuration)

	symbols := dc.fetcher.Symbols()

	// 2. Background REST refresh loop
	go dc.restLoop(ctx)

	// 3. Background WebSocket
	go dc.wsLoop(ctx, symbols)

	// 4. Background Social refresh
	if dc.social != nil {
		go dc.socialLoop(ctx)
	}
}

// restLoop periodically fetches fresh data via REST and atomically swaps the snapshot.
func (dc *DataCollector) restLoop(ctx context.Context) {
	ticker := time.NewTicker(dc.cfg.RestInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap, err := dc.fetcher.Fetch(ctx)
			if err != nil {
				log.Printf("datafetch: REST refresh error: %v", err)
				if snap == nil {
					continue
				}
			}

			// Copy over WS/social data from current snapshot
			current := dc.store.Current()
			if current != nil {
				mergeWSData(current, snap)
			}

			dc.store.Swap(snap)
			log.Printf("datafetch: REST refresh: %d symbols, %v, %d errors",
				snap.Meta.SymbolCount, snap.Meta.FetchDuration, snap.Meta.RestErrors)
		}
	}
}

// wsLoop connects to Binance WebSocket and patches the snapshot in real-time.
func (dc *DataCollector) wsLoop(ctx context.Context, symbols []string) {
	// Wait a bit for first snapshot to be available
	time.Sleep(2 * time.Second)

	snap := dc.store.Current()
	if snap == nil {
		log.Println("datafetch/ws: no snapshot available, skipping WS")
		return
	}

	// Use the latest symbol list
	dc.ws.Start(symbols)
}

// socialLoop periodically fetches social data for top symbols.
func (dc *DataCollector) socialLoop(ctx context.Context) {
	// Fetch social data for top 50 symbols initially, then refresh every SocialInterval
	dc.fetchSocialData(ctx)

	ticker := time.NewTicker(dc.cfg.SocialInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dc.fetchSocialData(ctx)
		}
	}
}

// fetchSocialData fetches social data for the top symbols and patches the snapshot.
func (dc *DataCollector) fetchSocialData(ctx context.Context) {
	snap := dc.store.Current()
	if snap == nil {
		return
	}

	// Build a new snapshot with social data
	newSnap := copySnapshot(snap)
	newSnap.Meta.SocialFresh = true

	count := 0
	for sym, ss := range newSnap.Symbols {
		if ctx.Err() != nil {
			break
		}
		// Only fetch for symbols with klines (top N)
		if len(ss.Klines) == 0 {
			continue
		}
		if count >= 50 {
			break
		}

		sd, err := dc.social.FetchCoinMetrics(ctx, sym)
		if err != nil {
			continue // skip silently
		}
		ss.Social = *sd
		count++
	}

	dc.store.Swap(newSnap)
	log.Printf("datafetch: social refresh: updated %d symbols", count)
}

// mergeWSData copies WebSocket-derived data from current snapshot into new REST snapshot.
func mergeWSData(current, fresh *Snapshot) {
	for sym, newSS := range fresh.Symbols {
		curSS, ok := current.Symbols[sym]
		if !ok {
			continue
		}
		// If WS has newer price data, prefer it
		if curSS.Timestamp.After(newSS.Timestamp) {
			newSS.Price = curSS.Price
			newSS.MarkPrice = curSS.MarkPrice
			newSS.IndexPrice = curSS.IndexPrice
			newSS.Spread = curSS.Spread
		}
		// Carry over social data (REST doesn't fetch it)
		if curSS.Social.UpdatedAt.After(newSS.Social.UpdatedAt) {
			newSS.Social = curSS.Social
		}
		// Carry over taker buy ratio from WS
		if curSS.TakerBuyRatio > 0 && newSS.TakerBuyRatio == 0 {
			newSS.TakerBuyRatio = curSS.TakerBuyRatio
		}
	}
	fresh.Meta.WSConnected = current.Meta.WSConnected
}

// copySnapshot creates a deep copy of a Snapshot.
func copySnapshot(src *Snapshot) *Snapshot {
	dst := &Snapshot{
		Symbols:   make(map[string]*SymbolSnapshot, len(src.Symbols)),
		Meta:      src.Meta,
		CreatedAt: src.CreatedAt,
	}
	for k, v := range src.Symbols {
		cp := *v
		if v.Klines != nil {
			cp.Klines = make(map[string][]Kline, len(v.Klines))
			for ki, kv := range v.Klines {
				klineCopy := make([]Kline, len(kv))
				copy(klineCopy, kv)
				cp.Klines[ki] = klineCopy
			}
		}
		if v.OISpikeData != nil {
			cp.OISpikeData = make([]float64, len(v.OISpikeData))
			copy(cp.OISpikeData, v.OISpikeData)
		}
		dst.Symbols[k] = &cp
	}
	return dst
}

// now returns the current time (overridable for testing).
var now = func() time.Time { return time.Now() }

func init() {
	// Suppress unused import
	_ = log.Println
}
