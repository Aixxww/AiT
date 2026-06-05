package local

import (
	"fmt"
	"github.com/Aixxww/AiT/datafetch"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"
)

// SymbolCache holds all pre-fetched data for a single symbol, gathered in
// one parallel pass. This eliminates the ~20 sequential API calls per coin
// that the old scoring pipeline required.
type SymbolCache struct {
	Symbol          string
	Ticker          binanceTicker
	Price           float64
	Klines          map[string][]klineBar // key: "5m","15m","1h","4h","1d"
	OICurrent       float64               // current open interest in USDT notional
	OIDelta4h       float64               // OI change % over 4h
	OIDelta1h       float64               // OI change % over 1h (for spike detection)
	LSROldest       float64               // oldest long/short ratio in window
	LSRNewest       float64               // newest long/short ratio in window
	FundingRate     float64               // last funding rate
	OIHist1hChanges []float64             // period-over-period % changes from 13 hourly OI entries (for spike detection)
	FetchTime       time.Time
}

// Binance weight costs for the data we fetch per symbol.
const (
	weightKlines20   = 1 // limit<=100 costs 1 weight
	weightOIHist     = 2 // openInterestHist costs 2
	weightLSR        = 2 // topLongShortPositionRatio costs 2
	weightPremiumIdx = 1 // premiumIndex costs 1
	// Total per symbol: 5 klines + 3 OI + 1 LSR + 1 FR = 10 calls, 13 weight
	// (5 × 1) + (3 × 2) + (1 × 2) + (1 × 1) = 13
	weightPerSymbol = 13
)

// klineIntervals are the timeframes fetched for each symbol during scoring.
var klineIntervals = []string{"5m", "15m", "1h", "4h", "1d"}

// klineLimits maps each interval to the number of bars to fetch.
// 15m and 5m need 50 bars for BB squeeze detection; others need 20.
var klineLimits = map[string]int{
	"5m": 50, "15m": 50, "1h": 20, "4h": 20, "1d": 20,
}

// premiumIndexEntry matches the Binance /fapi/v1/premiumIndex response.
type premiumIndexEntry struct {
	Symbol          string `json:"symbol"`
	MarkPrice       string `json:"markPrice"`
	IndexPrice      string `json:"indexPrice"`
	LastFundingRate string `json:"lastFundingRate"`
	NextFundingTime int64  `json:"nextFundingTime"`
}

// fetchBulkPremiumIndex fetches ALL symbols' funding rate + mark price in ONE HTTP call.
// Returns a map keyed by symbol name for O(1) lookup.
func (c *Client) fetchBulkPremiumIndex() (map[string]premiumIndexEntry, error) {
	url := c.BinanceURL + "/fapi/v1/premiumIndex"
	var entries []premiumIndexEntry
	if err := c.fetchJSONParallel(url, &entries); err != nil {
		return nil, err
	}
	m := make(map[string]premiumIndexEntry, len(entries))
	for _, e := range entries {
		m[e.Symbol] = e
	}
	return m, nil
}

// fetchCurrentOIParallel is like fetchCurrentOI but without rate limiting.
// Safe for concurrent use from multiple goroutines.
func (c *Client) fetchCurrentOIParallel(symbol string, price float64) (float64, error) {
	url := fmt.Sprintf("%s/fapi/v1/openInterest?symbol=%s", c.BinanceURL, symbol)
	var oiResp struct {
		OpenInterest string `json:"openInterest"`
	}
	if err := c.fetchJSONParallel(url, &oiResp); err != nil {
		return 0, err
	}
	return parseFloat(oiResp.OpenInterest) * price, nil
}

// fetchLSRHistParallel is like fetchLSRHist but without rate limiting.
// Safe for concurrent use from multiple goroutines.
func (c *Client) fetchLSRHistParallel(symbol, period string, limit int) (float64, float64, error) {
	url := fmt.Sprintf("%s/futures/data/topLongShortPositionRatio?symbol=%s&period=%s&limit=%d",
		c.BinanceURL, symbol, period, limit)
	var entries []lsrEntry
	if err := c.fetchJSONParallel(url, &entries); err != nil {
		return 0, 0, err
	}
	if len(entries) < 2 {
		return 0, 0, nil
	}
	oldest := parseFloat(entries[0].LongShortRatio)
	newest := parseFloat(entries[len(entries)-1].LongShortRatio)
	return oldest, newest, nil
}

// BuildSymbolCaches fetches all scoring data for each symbol using a 2-phase
// bulk+pool approach:
//
// Phase 1 (~200ms): Build skeletons from already-fetched tickers + one bulk
// premiumIndex call for ALL symbols' funding rates.
//
// Phase 2 (~2-3s): Per-symbol data (OI, LSR, klines) via 10 concurrent
// workers. Each worker spawns 9 parallel sub-goroutines (5 klines + 4 OI/LSR).
// Total concurrency: 10 workers × 9 goroutines = 90, competing for 15 semaphore slots.
// DisableKeepAlives on the shared HTTP client forces fresh TCP per request,
// preventing CDN edge cascade failures.
//
// The `rl` parameter is accepted for signature compatibility but ignored;
// concurrency is controlled by the worker pool size (10) and parallelSem (15).
func BuildSymbolCaches(
	c *Client,
	symbols []string,
	tickers map[string]binanceTicker,
	_ *BinanceRateLimiter,
) map[string]*SymbolCache {
	if len(symbols) == 0 {
		return nil
	}

	result := make(map[string]*SymbolCache, len(symbols))

	// === Phase 1: Build skeleton from already-fetched tickers ===
	// Tickers were already fetched by GetHunterList/GetAI500List
	// (ticker/24hr bulk call, ~629 symbols, already done).
	for _, sym := range symbols {
		ticker, ok := tickers[sym]
		if !ok {
			normalized := normalizeSymbol(sym)
			ticker, ok = tickers[normalized]
			if !ok {
				log.Printf("⚠️  SymbolCache: no ticker for %s, skipping", sym)
				continue
			}
			result[sym] = &SymbolCache{
				Symbol:    normalized,
				Ticker:    ticker,
				Price:     parseFloat(ticker.LastPrice),
				Klines:    make(map[string][]klineBar, len(klineIntervals)),
				FetchTime: time.Now(),
			}
		} else {
			result[sym] = &SymbolCache{
				Symbol:    sym,
				Ticker:    ticker,
				Price:     parseFloat(ticker.LastPrice),
				Klines:    make(map[string][]klineBar, len(klineIntervals)),
				FetchTime: time.Now(),
			}
		}
	}

	if len(result) == 0 {
		return result
	}

	// === Phase 1.5: Fetch bulk premiumIndex (funding rate for ALL symbols) ===
	// This is ONE HTTP call covering all 750+ symbols (~100ms).
	piMap, err := c.fetchBulkPremiumIndex()
	if err != nil {
		log.Printf("⚠️  SymbolCache: bulk premiumIndex failed (%v), funding rates will be zero", err)
	} else {
		for sym, sc := range result {
			if pi, ok := piMap[sym]; ok {
				sc.FundingRate = parseFloat(pi.LastFundingRate)
			}
		}
	}

	// === Phase 2: Per-symbol data via worker pool ===
	// Each worker fetches all per-symbol data in parallel sub-goroutines.
	// 10 workers × 8 sub-goroutines = 80 goroutines, competing for 20 semaphore slots.
	// This prevents Binance 429 errors and eliminates cascading retry delays.
	type job struct {
		symbol string
	}
	jobs := make(chan job, len(result))
	for sym := range result {
		jobs <- job{symbol: sym}
	}
	close(jobs)

	const workers = 10
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				sc := result[j.symbol]
				if sc == nil {
					continue
				}
				fetchSymbolDataPool(c, sc)
			}
		}()
	}
	wg.Wait()

	return result
}

// fetchSymbolDataPool fetches all per-symbol data in parallel sub-goroutines.
// Uses fetchJSONParallel/fetchKlinesParallel which are throttled by parallelSem (15 slots).
// With 10 workers, total concurrency is 10 × 9 = 90 goroutines competing for 15 slots.
func fetchSymbolDataPool(c *Client, sc *SymbolCache) {
	symbol := sc.Symbol
	var wg sync.WaitGroup
	var mu sync.Mutex // protects sc fields written by parallel goroutines

	// --- Klines: 5 timeframes in parallel ---
	for _, interval := range klineIntervals {
		wg.Add(1)
		go func(iv string) {
			defer wg.Done()
			bars, err := c.fetchKlinesParallel(symbol, iv, klineLimits[iv])
			if err != nil {
				return
			}
			if len(bars) == 0 {
				return
			}
			// Filter zeroed bars
			filtered := make([]klineBar, 0, len(bars))
			for _, bar := range bars {
				if bar.Close != 0 || bar.Volume != 0 {
					filtered = append(filtered, bar)
				}
			}
			if len(filtered) == 0 {
				return
			}
			mu.Lock()
			sc.Klines[iv] = filtered
			mu.Unlock()
		}(interval)
	}

	// --- OI current ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		oi, err := c.fetchCurrentOIParallel(symbol, sc.Price)
		if err != nil {
			log.Printf("⚠️  OI fetch failed: %s %v", symbol, err)
			return
		}
		log.Printf("📊 OI Debug: %s raw=$%.0f price=%.6f", symbol, oi, sc.Price)
		mu.Lock()
		sc.OICurrent = oi
		mu.Unlock()
	}()

	// --- OI History: 4h delta ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		delta, err := c.fetchOIHistParallel(symbol, "4h", 24)
		if err != nil {
			return
		}
		mu.Lock()
		sc.OIDelta4h = delta
		mu.Unlock()
	}()

	// --- OI History: 1h delta (for OI spike detection) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		delta, err := c.fetchOIHistParallel(symbol, "1h", 4)
		if err != nil {
			return
		}
		mu.Lock()
		sc.OIDelta1h = delta
		mu.Unlock()
	}()

	// --- OI History for spike detection (1h, 13 entries) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		url := fmt.Sprintf("%s/futures/data/openInterestHist?symbol=%s&period=1h&limit=13",
			c.BinanceURL, symbol)
		type oiEntry struct {
			SumOpenInterestValue string `json:"sumOpenInterestValue"`
		}
		var entries []oiEntry
		err := c.fetchJSONParallel(url, &entries)
		if err != nil || len(entries) < 4 {
			return
		}

		var changes []float64
		for i := 1; i < len(entries); i++ {
			prev := parseFloat(entries[i-1].SumOpenInterestValue)
			curr := parseFloat(entries[i].SumOpenInterestValue)
			if prev > 0 {
				changes = append(changes, (curr-prev)/prev*100)
			}
		}

		mu.Lock()
		sc.OIHist1hChanges = changes
		mu.Unlock()
	}()

	// --- LSR (Long/Short Ratio, 1h, 12 entries) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		oldest, newest, err := c.fetchLSRHistParallel(symbol, "1h", 12)
		if err != nil {
			return
		}
		mu.Lock()
		sc.LSROldest = oldest
		sc.LSRNewest = newest
		mu.Unlock()
	}()

	wg.Wait()
}

// lastN returns the last n elements from bars. If len(bars) <= n, returns bars as-is.
func lastN(bars []klineBar, n int) []klineBar {
	if len(bars) <= n {
		return bars
	}
	return bars[len(bars)-n:]
}

// GetSymbolCachesForEval fetches tickers, filters to top N USDT perps by quote volume,
// and builds SymbolCaches with full per-symbol data (OI, LSR, klines, funding rate).
// This is used by engine_eval to bypass the datafetch HTTP client which has
// persistent connection reset issues with Binance's CDN.
func (c *Client) GetSymbolCachesForEval(topN int) (map[string]*SymbolCache, error) {
	// Fetch tickers using the rate-limited fetchJSON path (proven reliable)
	url := c.BinanceURL + "/fapi/v1/ticker/24hr"
	var tickers []binanceTicker
	if err := c.fetchJSON(url, &tickers); err != nil {
		return nil, fmt.Errorf("GetSymbolCachesForEval: fetch tickers failed: %w", err)
	}

	// Filter to USDT perps and sort by quote volume
	type symQV struct {
		symbol string
		qv     float64
	}
	var pool []symQV
	tickerMap := make(map[string]binanceTicker, len(tickers))
	for _, t := range tickers {
		if !isUSDTPerp(t.Symbol) {
			continue
		}
		qv := parseFloat(t.QuoteVolume)
		if qv <= 0 {
			continue
		}
		pool = append(pool, symQV{t.Symbol, qv})
		tickerMap[t.Symbol] = t
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].qv > pool[j].qv })
	if len(pool) > topN {
		pool = pool[:topN]
	}
	symbols := make([]string, len(pool))
	for i, p := range pool {
		symbols[i] = p.symbol
	}

	log.Printf("✅ GetSymbolCachesForEval: %d USDT perps, building caches for top %d", len(pool), len(symbols))

	// Build caches (fetches OI, LSR, klines, funding rate per symbol)
	caches := BuildSymbolCaches(c, symbols, tickerMap, nil)
	return caches, nil
}

// CachesToSnapshot converts SymbolCaches (from GetSymbolCachesForEval or
// BuildSymbolCaches) into a datafetch.Snapshot that IndicatorHub can score.
// This bridges the local.Client data path with the new engine's scoring pipeline.
func CachesToSnapshot(caches map[string]*SymbolCache) *datafetch.Snapshot {
	snap := &datafetch.Snapshot{
		Symbols:   make(map[string]*datafetch.SymbolSnapshot, len(caches)),
		CreatedAt: time.Now(),
		Meta: datafetch.SnapshotMeta{
			SymbolCount: len(caches),
		},
	}

	for sym, sc := range caches {
		if sc == nil {
			continue
		}
		t := sc.Ticker
		priceChange, _ := strconv.ParseFloat(t.PriceChangePercent, 64)
		vol24h, _ := strconv.ParseFloat(t.Volume, 64)
		qv24h, _ := strconv.ParseFloat(t.QuoteVolume, 64)
		high24h, _ := strconv.ParseFloat(t.HighPrice, 64)
		low24h, _ := strconv.ParseFloat(t.LowPrice, 64)

		ss := &datafetch.SymbolSnapshot{
			Symbol:         sym,
			Price:          sc.Price,
			Timestamp:      sc.FetchTime,
			PriceChange24h: priceChange,
			Volume24h:      vol24h,
			QuoteVolume24h: qv24h,
			HighPrice24h:   high24h,
			LowPrice24h:    low24h,
			TradeCount24h:  t.Count,

			FundingRate:    sc.FundingRate,
			OI:             sc.OICurrent,
			OIDelta1h:      sc.OIDelta1h,
			OIDelta4h:      sc.OIDelta4h,
			OISpikeData:    sc.OIHist1hChanges,
			LongShortRatio: sc.LSRNewest,
			LSRPrev:        sc.LSROldest,
		}

		// Convert klines
		ss.Klines = make(map[string][]datafetch.Kline, len(sc.Klines))
		for iv, bars := range sc.Klines {
			klines := make([]datafetch.Kline, len(bars))
			for i, bar := range bars {
				klines[i] = datafetch.Kline{
					OpenTime: bar.OpenTime,
					Open:     bar.Open,
					High:     bar.High,
					Low:      bar.Low,
					Close:    bar.Close,
					Volume:   bar.Volume,
					TakerBuy: bar.TakerBuyBaseVolume,
				}
			}
			ss.Klines[iv] = klines
		}

		snap.Symbols[sym] = ss
	}

	return snap
}
