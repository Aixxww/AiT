package datafetch

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"
)

// FetcherConfig holds configuration for the DataFetcher.
type FetcherConfig struct {
	BinanceURL    string        // default "https://fapi.binance.com"
	MaxWorkers    int           // default 50
	TopNForDetail int           // default 100 (only top N get OI/LSR/Klines)
	Timeout       time.Duration // default 10s
}

// DataFetcher performs one complete REST data fetch cycle.
type DataFetcher struct {
	cfg     FetcherConfig
	client  *HTTPClient
	symbols []string // cached symbol list, refreshed on each Fetch
}

// NewDataFetcher creates a new DataFetcher.
func NewDataFetcher(cfg FetcherConfig) *DataFetcher {
	if cfg.BinanceURL == "" {
		cfg.BinanceURL = "https://fapi.binance.com"
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 50
	}
	if cfg.TopNForDetail <= 0 {
		cfg.TopNForDetail = 100
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &DataFetcher{
		cfg:    cfg,
		client: NewHTTPClient(cfg.BinanceURL),
	}
}

// Fetch does one complete data fetch cycle and returns a Snapshot.
//
// Phase 1: 3 concurrent bulk APIs (tickers, premiumIndex, exchangeInfo)
// Phase 2: 50-worker per-symbol data for top N
func (f *DataFetcher) Fetch(ctx context.Context) (*Snapshot, error) {
	start := time.Now()

	// Phase 1: Bulk fetches (concurrent)
	type bulkResult struct {
		tickers   map[string]*ticker24hrRaw
		premiums  map[string]*premiumIndexRaw
		symbols   []string
		tickerErr error
		premErr   error
		symErr    error
	}

	ch := make(chan bulkResult, 1)
	go func() {
		var r bulkResult

		// Fetch sequentially to avoid Binance CDN connection resets on concurrent requests.
		// The provider/local package also fetches sequentially and works reliably.
		r.tickers, r.tickerErr = f.fetchAllTickers(ctx)
		r.premiums, r.premErr = f.fetchAllPremiumIndex(ctx)
		r.symbols, r.symErr = f.fetchExchangeInfo(ctx)

		ch <- r
	}()

	bulk := <-ch

	restErrors := 0
	if bulk.tickerErr != nil {
		restErrors++
		log.Printf("datafetch: ticker fetch error: %v", bulk.tickerErr)
	}
	if bulk.premErr != nil {
		restErrors++
		log.Printf("datafetch: premiumIndex fetch error: %v", bulk.premErr)
	}
	if bulk.symErr != nil {
		return nil, fmt.Errorf("datafetch: exchangeInfo fetch failed: %w", bulk.symErr)
	}

	if bulk.tickers == nil {
		bulk.tickers = make(map[string]*ticker24hrRaw)
	}
	if bulk.premiums == nil {
		bulk.premiums = make(map[string]*premiumIndexRaw)
	}

	f.symbols = bulk.symbols

	// Sort symbols by quote volume descending so top N = highest volume
	sort.Slice(f.symbols, func(i, j int) bool {
		vi := quoteVolume(bulk.tickers[f.symbols[i]])
		vj := quoteVolume(bulk.tickers[f.symbols[j]])
		return vi > vj
	})

	// Phase 2: Per-symbol data for top N
	snapshots, perErrs := f.fetchPerSymbolData(ctx, f.symbols, bulk.tickers, bulk.premiums)
	restErrors += perErrs

	snap := &Snapshot{
		Symbols:   snapshots,
		CreatedAt: time.Now(),
		Meta: SnapshotMeta{
			SymbolCount:   len(f.symbols),
			FetchDuration: time.Since(start),
			RestErrors:    restErrors,
		},
	}

	return snap, nil
}

// quoteVolume extracts quoteVolume as float64 from a ticker, for sorting.
func quoteVolume(t *ticker24hrRaw) float64 {
	if t == nil {
		return 0
	}
	f, _ := strconv.ParseFloat(t.QuoteVolume, 64)
	return f
}

// Symbols returns the cached symbol list from the last Fetch.
func (f *DataFetcher) Symbols() []string {
	return f.symbols
}
