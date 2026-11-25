package datafetch

import (
	"context"
	"log"
	"sync"
	"time"
)

// CollectorConfig holds configuration for the DataCollector.
type CollectorConfig struct {
	BinanceURL              string // "https://fapi.binance.com"
	BinanceWSURL            string // "wss://fstream.binance.com"
	LunarCrushKey           string
	LunarCrushBase          string        // "https://lunarcrush.com/api4"
	RestInterval            time.Duration // 30s
	SocialInterval          time.Duration // 15min
	MaxWorkers              int           // 50
	TopNForDetail           int           // 100
	TopNForWS               int           // 30 (top symbols for WS kline/aggTrade streams)
	FastKlines              []KlineInterval
	IncludeNonCryptoFutures bool // include Binance futures stock/commodity/ETF/RWA perps; default false
	IncludeT2Inputs         bool // fetch depth + 5m OI history for detail symbols (task-2 module); default false

	// #87 layered fetch. Zero values take the defaults below.
	SlowEvery         int           // L3: each detail symbol's slow fields are refreshed once every SlowEvery rounds (10)
	SlowFieldMaxAge   time.Duration // L3/A5: carried slow fields older than this are cleared (3 x SlowEvery x RestInterval)
	WeightLimitPerMin float64       // rate discipline: operating weight per minute (DefaultOperatingWeightPerMin)
	FetchBudget       time.Duration // per-round deadline (RestInterval - 5s); weakest detail symbols are skipped first
}

// SlowRollingKlineIntervals are the structural klines refreshed with a
// symbol's slow fields (#87 L3): 4h every slow slot, plus 1d, which was
// previously fetched once at start and carried forever.
var SlowRollingKlineIntervals = []KlineInterval{{"4h", 100}, {"1d", 30}}

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
	if len(cfg.FastKlines) == 0 {
		cfg.FastKlines = FastKlineIntervals
	}
	if cfg.SlowEvery <= 0 {
		cfg.SlowEvery = 10
	}
	if cfg.SlowFieldMaxAge <= 0 {
		cfg.SlowFieldMaxAge = 3 * time.Duration(cfg.SlowEvery) * cfg.RestInterval
	}
	if cfg.WeightLimitPerMin <= 0 {
		cfg.WeightLimitPerMin = DefaultOperatingWeightPerMin
	}
	if cfg.FetchBudget <= 0 {
		cfg.FetchBudget = cfg.RestInterval - 5*time.Second
		if cfg.FetchBudget < 10*time.Second {
			cfg.FetchBudget = cfg.RestInterval
		}
	}

	fetcher := NewDataFetcher(FetcherConfig{
		BinanceURL:              cfg.BinanceURL,
		MaxWorkers:              cfg.MaxWorkers,
		TopNForDetail:           cfg.TopNForDetail,
		KlineIntervals:          DefaultKlineIntervals,
		IncludeNonCryptoFutures: cfg.IncludeNonCryptoFutures,
		IncludeT2Inputs:         cfg.IncludeT2Inputs,
		// #87 rate discipline: weight-based limiting at the operating
		// weight (cap 2400), per-endpoint sub-budgets, the /futures/data
		// per-IP limit, and a per-round deadline.
		WeightLimitPerMin:       cfg.WeightLimitPerMin,
		EndpointSubBudgets:      true,
		FuturesDataLimitPer5Min: DefaultFuturesDataLimitPer5Min,
		FetchBudget:             cfg.FetchBudget,
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
	// Full start-up fetch (all kline intervals, every slow field). It is
	// larger than one round, so it is not cut by the round budget; the weight
	// limiter still paces it.
	snap, err := dc.fetcher.FetchWithOptions(ctx, FetchOptions{
		KlineIntervals:         DefaultKlineIntervals,
		RefreshSlowDerivatives: true,
		NoBudget:               true,
	})
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
	cycle := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cycle++
			current := dc.store.Current()
			snap, err := dc.fetcher.FetchWithOptions(ctx, dc.roundOptions(current, cycle))
			if err != nil {
				log.Printf("datafetch: REST refresh error: %v", err)
				if snap == nil {
					continue
				}
			}

			// Copy over WS data and carried slow fields from the current snapshot.
			if current != nil {
				mergeWSData(current, snap)
				mergeCarriedKlines(current, snap, dc.cfg.FastKlines)
			}
			mergeCarriedSlowDerivatives(current, snap, dc.cfg.SlowFieldMaxAge)

			dc.store.Swap(snap)
			m := snap.Meta
			log.Printf("datafetch: REST refresh: %d symbols, %v, %d errors, detail %d/%d (skipped %d), slow due %d, requests %d, weight %d, 429/418 %d, server weight %d",
				m.SymbolCount, m.FetchDuration, m.RestErrors, m.DetailCompleted, m.DetailRequested, m.DetailSkippedDeadline,
				m.SlowDue, m.Requests, m.Weight, m.RateLimited, m.MaxServerWeight)
		}
	}
}

// roundOptions is one REST round's fetch plan (#87): fast klines for every
// detail symbol, and slow derivatives + structural klines for the detail
// symbols whose rolling slot is due (or that have no slow data yet).
func (dc *DataCollector) roundOptions(current *Snapshot, cycle int) FetchOptions {
	every := dc.cfg.SlowEvery
	return FetchOptions{
		KlineIntervals:     dc.cfg.FastKlines,
		SlowKlineIntervals: SlowRollingKlineIntervals,
		SlowDue: func(symbol string) bool {
			if tierSlot(symbol, every) == cycle%every {
				return true
			}
			if current == nil {
				return true
			}
			ss := current.Symbols[symbol]
			// Warm-up / retry: a detail symbol without slow data fetches it now.
			return ss == nil || (ss.SlowFetchedAt.OIHistory1h.IsZero() && len(ss.OISpikeData) == 0)
		},
	}
}

// RefreshResult is one symbol's outcome in RefreshSymbols (#87 L4).
type RefreshResult struct {
	Symbol string
	Snap   *SymbolSnapshot // nil when nothing could be fetched
	Err    error
	At     time.Time // when the refreshed data landed (zero on failure)
}

// RefreshSymbols refreshes several symbols in parallel under one deadline
// (ctx) and patches the current snapshot once, so concurrent single-symbol
// patches cannot overwrite each other (#87 L4 finalist refresh). Symbols
// whose refresh fails or misses the deadline keep their snapshot values.
func (dc *DataCollector) RefreshSymbols(ctx context.Context, symbols []string, klineIntervals []KlineInterval) []RefreshResult {
	if dc == nil || dc.fetcher == nil || len(symbols) == 0 {
		return nil
	}
	current := dc.store.Current()
	out := make([]RefreshResult, len(symbols))
	var wg sync.WaitGroup
	for i, sym := range symbols {
		var base *SymbolSnapshot
		if current != nil && current.Symbols != nil {
			base = current.Symbols[sym]
		}
		wg.Add(1)
		go func(i int, sym string, base *SymbolSnapshot) {
			defer wg.Done()
			ss, err := dc.fetcher.RefreshSymbol(ctx, sym, base, klineIntervals, false)
			r := RefreshResult{Symbol: sym, Err: err}
			if err == nil && ss != nil {
				r.Snap, r.At = ss, now()
			}
			out[i] = r
		}(i, sym, base)
	}
	wg.Wait()

	latest := dc.store.Current()
	if latest == nil {
		return out
	}
	next := copySnapshot(latest)
	patched := 0
	for _, r := range out {
		if r.Snap != nil {
			next.Symbols[r.Symbol] = r.Snap
			patched++
		}
	}
	if patched > 0 {
		dc.store.Swap(next)
	}
	return out
}

// RefreshSymbol refreshes one symbol through REST and patches the current
// atomic snapshot. It is intended for execution preflight checks where waiting
// for the next 30s REST cycle would make high-velocity signals stale.
func (dc *DataCollector) RefreshSymbol(ctx context.Context, symbol string, klineIntervals []KlineInterval, refreshSlowDerivatives bool) (*SymbolSnapshot, error) {
	if dc == nil || dc.fetcher == nil {
		return nil, nil
	}
	current := dc.store.Current()
	var base *SymbolSnapshot
	if current != nil && current.Symbols != nil {
		base = current.Symbols[symbol]
	}
	refreshed, err := dc.fetcher.RefreshSymbol(ctx, symbol, base, klineIntervals, refreshSlowDerivatives)
	if refreshed == nil {
		return nil, err
	}

	var next *Snapshot
	if current != nil {
		next = copySnapshot(current)
	} else {
		next = &Snapshot{
			Symbols:   make(map[string]*SymbolSnapshot),
			CreatedAt: now(),
			Meta:      SnapshotMeta{SymbolCount: 1},
		}
	}
	if next.Symbols == nil {
		next.Symbols = make(map[string]*SymbolSnapshot)
	}
	next.Symbols[symbol] = refreshed
	if next.Meta.SymbolCount <= 0 {
		next.Meta.SymbolCount = len(next.Symbols)
	}
	if err != nil {
		next.Meta.RestErrors++
	}
	dc.store.Swap(next)
	return refreshed, err
}

func appendMissingKlineIntervals(base []KlineInterval, extra ...KlineInterval) []KlineInterval {
	seen := make(map[string]bool, len(base)+len(extra))
	for _, ki := range base {
		seen[ki.Interval] = true
	}
	for _, ki := range extra {
		if seen[ki.Interval] {
			continue
		}
		base = append(base, ki)
		seen[ki.Interval] = true
	}
	return base
}

// mergeCarriedSlowDerivatives carries each slow field group a symbol did
// not fetch this round from the current snapshot, with its original fetch
// time (#87 L3). A group older than maxAge is cleared instead (AUD-A5):
// consumers read an empty field as unavailable, never as fresh. maxAge <= 0
// keeps everything.
func mergeCarriedSlowDerivatives(current, fresh *Snapshot, maxAge time.Duration) {
	if fresh == nil {
		return
	}
	t := now()
	for sym, newSS := range fresh.Symbols {
		if newSS == nil {
			continue
		}
		var curSS *SymbolSnapshot
		if current != nil {
			curSS = current.Symbols[sym]
		}
		if curSS != nil {
			if newSS.SlowFetchedAt.OIHistory1h.IsZero() && len(newSS.OISpikeData) == 0 {
				newSS.OISpikeData = append([]float64{}, curSS.OISpikeData...)
				newSS.OIDelta1h = curSS.OIDelta1h
				newSS.OIDelta4h = curSS.OIDelta4h
				newSS.SlowFetchedAt.OIHistory1h = curSS.SlowFetchedAt.OIHistory1h
			}
			if newSS.SlowFetchedAt.LSR.IsZero() && newSS.LongShortRatio == 0 {
				newSS.LongShortRatio = curSS.LongShortRatio
				newSS.LSRPrev = curSS.LSRPrev
				newSS.LSROldest = curSS.LSROldest
				newSS.SlowFetchedAt.LSR = curSS.SlowFetchedAt.LSR
			}
			if newSS.SlowFetchedAt.Depth.IsZero() && len(newSS.DepthBids) == 0 && len(newSS.DepthAsks) == 0 {
				newSS.DepthBids = append([]DepthLevel{}, curSS.DepthBids...)
				newSS.DepthAsks = append([]DepthLevel{}, curSS.DepthAsks...)
				newSS.SlowFetchedAt.Depth = curSS.SlowFetchedAt.Depth
			}
			if newSS.SlowFetchedAt.OI5mHistory.IsZero() && len(newSS.OI5mHistory) == 0 {
				newSS.OI5mHistory = append([]OIHistoryPoint{}, curSS.OI5mHistory...)
				newSS.SlowFetchedAt.OI5mHistory = curSS.SlowFetchedAt.OI5mHistory
			}
		}
		if maxAge > 0 {
			degradeStaleSlowFields(newSS, t, maxAge)
		}
	}
}

// degradeStaleSlowFields clears slow field groups older than maxAge. The
// fetch time is kept, so the age stays visible.
func degradeStaleSlowFields(ss *SymbolSnapshot, t time.Time, maxAge time.Duration) {
	stale := func(at time.Time) bool { return !at.IsZero() && t.Sub(at) > maxAge }
	if stale(ss.SlowFetchedAt.OIHistory1h) {
		ss.OISpikeData, ss.OIDelta1h, ss.OIDelta4h = nil, 0, 0
	}
	if stale(ss.SlowFetchedAt.LSR) {
		ss.LongShortRatio, ss.LSRPrev, ss.LSROldest = 0, 0, 0
	}
	if stale(ss.SlowFetchedAt.Depth) {
		ss.DepthBids, ss.DepthAsks = nil, nil
	}
	if stale(ss.SlowFetchedAt.OI5mHistory) {
		ss.OI5mHistory = nil
	}
}

// mergeCarriedKlines carries the klines a symbol did not fetch this round
// (structural 4h / 1d between its slow slots), with their fetch times. Fast
// intervals are never carried: a symbol that left the detail set must not
// show old 1m bars as current.
func mergeCarriedKlines(current, fresh *Snapshot, fast []KlineInterval) {
	if current == nil || fresh == nil {
		return
	}
	fastSet := make(map[string]bool, len(fast))
	for _, ki := range fast {
		fastSet[ki.Interval] = true
	}
	for sym, newSS := range fresh.Symbols {
		curSS, ok := current.Symbols[sym]
		if !ok || curSS == nil || newSS == nil {
			continue
		}
		if newSS.Klines == nil {
			newSS.Klines = make(map[string][]Kline)
		}
		for interval, klines := range curSS.Klines {
			if fastSet[interval] || len(klines) == 0 || len(newSS.Klines[interval]) > 0 {
				continue
			}
			carried := make([]Kline, len(klines))
			copy(carried, klines)
			newSS.Klines[interval] = carried
			if at, ok := curSS.KlineFetchedAt[interval]; ok {
				if newSS.KlineFetchedAt == nil {
					newSS.KlineFetchedAt = make(map[string]time.Time)
				}
				newSS.KlineFetchedAt[interval] = at
			}
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
		// If WS has newer price data, prefer it. #87 L5: compare the price
		// write time (WS mark price updates it every second for every
		// symbol); fall back to the snapshot timestamp for old data.
		if curSS.PriceAt.After(newSS.PriceAt) || (curSS.PriceAt.IsZero() && newSS.PriceAt.IsZero() && curSS.Timestamp.After(newSS.Timestamp)) {
			newSS.Price = curSS.Price
			newSS.PriceAt = curSS.PriceAt
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
		if v.DepthBids != nil {
			cp.DepthBids = make([]DepthLevel, len(v.DepthBids))
			copy(cp.DepthBids, v.DepthBids)
		}
		if v.DepthAsks != nil {
			cp.DepthAsks = make([]DepthLevel, len(v.DepthAsks))
			copy(cp.DepthAsks, v.DepthAsks)
		}
		if v.OI5mHistory != nil {
			cp.OI5mHistory = make([]OIHistoryPoint, len(v.OI5mHistory))
			copy(cp.OI5mHistory, v.OI5mHistory)
		}
		cp.KlineFetchedAt = copyTimeMap(v.KlineFetchedAt)
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
