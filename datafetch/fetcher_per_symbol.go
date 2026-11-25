package datafetch

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sync/atomic"
	"time"
)

// perSymbolResult holds the result of fetching data for a single symbol.
type perSymbolResult struct {
	symbol string
	snap   *SymbolSnapshot
}

// fetchPerSymbolData fetches per-symbol data (OI, OI history, LSR, klines) for the
// top N symbols using a concurrent worker pool. It enriches SymbolSnapshots that
// were already partially built from bulk data.
func (f *DataFetcher) fetchPerSymbolData(
	ctx context.Context,
	symbols []string,
	detailSymbols []string,
	tickers map[string]*ticker24hrRaw,
	premiums map[string]*premiumIndexRaw,
	plan perSymbolPlan,
) (map[string]*SymbolSnapshot, detailCounts) {

	// Build partial snapshots from bulk data for ALL symbols first
	all := bulkBaseSnapshots(symbols, tickers, premiums)

	// Semaphore-based worker pool
	workers := f.cfg.MaxWorkers
	if workers <= 0 {
		workers = 5
	}
	sem := make(chan struct{}, workers)
	results := make(chan perSymbolResult, len(detailSymbols))
	var errCount, completed, skipped, slowDue int64

	for _, sym := range detailSymbols {
		sym := sym
		base := all[sym]
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			if ctx.Err() != nil {
				atomic.AddInt64(&skipped, 1) // TA-5: counted, no longer silent
				return
			}
			atomic.AddInt64(&completed, 1)
			klines, slow := plan.forSymbol(sym)
			if slow {
				atomic.AddInt64(&slowDue, 1)
			}
			ss, err := f.fetchOneSymbol(ctx, sym, base, klines, slow)
			if err != nil {
				atomic.AddInt64(&errCount, 1)
				if ss == nil {
					return
				}
			}
			results <- perSymbolResult{symbol: sym, snap: ss}
		}()
	}

	// Wait for all workers — close results channel after draining sem
	go func() {
		for i := 0; i < cap(sem); i++ {
			sem <- struct{}{}
		}
		close(results)
	}()

	for r := range results {
		all[r.symbol] = r.snap
	}

	return all, detailCounts{
		symbolErrors: int(atomic.LoadInt64(&errCount)),
		requested:    len(detailSymbols),
		completed:    int(atomic.LoadInt64(&completed)),
		skipped:      int(atomic.LoadInt64(&skipped)),
		slowDue:      int(atomic.LoadInt64(&slowDue)),
	}
}

// perSymbolPlan is what one detail fetch pass requests per symbol (#87).
type perSymbolPlan struct {
	klines     []KlineInterval
	slowKlines []KlineInterval
	slowDue    func(symbol string) bool
}

func (p perSymbolPlan) forSymbol(sym string) ([]KlineInterval, bool) {
	if p.slowDue == nil || !p.slowDue(sym) {
		return p.klines, false
	}
	if len(p.slowKlines) == 0 {
		return p.klines, true
	}
	out := make([]KlineInterval, 0, len(p.klines)+len(p.slowKlines))
	out = append(out, p.klines...)
	return appendMissingKlineIntervals(out, p.slowKlines...), true
}

// slowDueFunc maps FetchOptions to a per-symbol slow decision: SlowDue
// when set, else the legacy all-or-nothing RefreshSlowDerivatives.
func slowDueFunc(opts FetchOptions) func(string) bool {
	if opts.SlowDue != nil {
		return opts.SlowDue
	}
	all := opts.RefreshSlowDerivatives
	return func(string) bool { return all }
}

// bulkBaseSnapshots builds the partial snapshot of every symbol from the
// bulk ticker and premiumIndex data (shared by the single- and two-tier fetch).
func bulkBaseSnapshots(symbols []string, tickers map[string]*ticker24hrRaw, premiums map[string]*premiumIndexRaw) map[string]*SymbolSnapshot {
	all := make(map[string]*SymbolSnapshot, len(symbols))
	for _, sym := range symbols {
		ss := &SymbolSnapshot{
			Symbol: sym,
			Klines: make(map[string][]Kline),
			Social: SocialData{},
		}
		if t, ok := tickers[sym]; ok {
			ss.Price = parseFloat(t.LastPrice)
			ss.PriceChange24h = parseFloat(t.PriceChangePercent)
			ss.Volume24h = parseFloat(t.Volume)
			ss.QuoteVolume24h = parseFloat(t.QuoteVolume)
			ss.HighPrice24h = parseFloat(t.HighPrice)
			ss.LowPrice24h = parseFloat(t.LowPrice)
			ss.TradeCount24h = t.Count
		}
		if p, ok := premiums[sym]; ok {
			ss.MarkPrice = parseFloat(p.MarkPrice)
			ss.IndexPrice = parseFloat(p.IndexPrice)
			ss.FundingRate = parseFloat(p.LastFundingRate)
			ss.NextFundingTime = p.NextFundingTime
			if ss.IndexPrice > 0 {
				ss.Spread = (ss.MarkPrice - ss.IndexPrice) / ss.IndexPrice * 100
			}
		}
		ss.Timestamp = now()
		if ss.Price > 0 {
			ss.PriceAt = ss.Timestamp
		}
		all[sym] = ss
	}
	return all
}

// detailCounts summarises one per-symbol fetch pass (TA-5).
type detailCounts struct {
	symbolErrors int // symbols with at least one failed sub-request (RestErrors)
	requested    int
	completed    int
	skipped      int // ctx expired before the symbol's worker started
	slowDue      int // symbols whose slow fields were due (#87 L3)
}

// fetchOneSymbol fetches per-symbol data for a single symbol. OI history and
// top-trader LSR are 1h-granularity signals, so fast REST cycles may carry
// them from the previous snapshot instead of re-fetching them every 30 seconds.
func (f *DataFetcher) fetchOneSymbol(ctx context.Context, symbol string, base *SymbolSnapshot, klineIntervals []KlineInterval, refreshSlowDerivatives bool) (*SymbolSnapshot, error) {
	ss := base
	if ss == nil {
		ss = &SymbolSnapshot{
			Symbol: symbol,
			Klines: make(map[string][]Kline),
		}
	}

	var errs int

	// 1. Open Interest (weight: 1)
	oi, err := f.fetchOI(ctx, symbol)
	if err != nil {
		errs++
	} else {
		ss.OI = oi
	}

	if refreshSlowDerivatives {
		// 2. OI History — 13 hourly entries (weight: 2)
		oiHist, err := f.fetchOIHistory(ctx, symbol, "1h", 13)
		if err != nil {
			errs++
		} else {
			ss.SlowFetchedAt.OIHistory1h = now()
			ss.OISpikeData = oiHist
			if len(oiHist) >= 2 {
				ss.OIDelta1h = oiHist[len(oiHist)-1]
			}
			if len(oiHist) >= 5 {
				ss.OIDelta4h = 0
				for i := len(oiHist) - 4; i < len(oiHist); i++ {
					if i >= 0 {
						ss.OIDelta4h += oiHist[i]
					}
				}
			}
		}

		// 3. Long/Short Ratio — 12 entries for Hunter oldest/newest delta (weight: 2)
		lsrData, err := f.fetchLSR(ctx, symbol, 12)
		if err != nil {
			errs++
		} else {
			ss.SlowFetchedAt.LSR = now()
			if len(lsrData) >= 1 {
				ss.LongShortRatio = lsrData[len(lsrData)-1] // newest
				ss.LSROldest = lsrData[0]                   // oldest (for Hunter reversal)
			}
			if len(lsrData) >= 2 {
				ss.LSRPrev = lsrData[len(lsrData)-2]
			}
		}

		// 4. Task-2 module inputs (only when IncludeT2Inputs): order-book
		// depth (weight: 10 at limit=100 — the heaviest per-symbol call in
		// this package) and the 5m OI history series (weight: 2). They ride
		// this slow-derivative cadence deliberately: the 5m OI series gains
		// a new point only every 5 minutes, and per-cycle depth for every
		// detail symbol would add ~1000 weight per 30s cycle. A failed
		// fetch leaves the fields empty, which consumers read as
		// "unavailable" (fail-closed), never as a zero book/series.
		if f.cfg.IncludeT2Inputs {
			bids, asks, err := f.fetchDepth(ctx, symbol)
			if err != nil {
				errs++
			} else {
				ss.SlowFetchedAt.Depth = now()
				ss.DepthBids = bids
				ss.DepthAsks = asks
			}
			oi5m, err := f.fetchOIHistorySeries(ctx, symbol, "5m", 30)
			if err != nil {
				errs++
			} else {
				ss.SlowFetchedAt.OI5mHistory = now()
				ss.OI5mHistory = oi5m
			}
		}
	}

	// 4. Klines — configured timeframes (weight: 1 each)
	for _, ki := range klineIntervals {
		klines, err := f.fetchKlines(ctx, symbol, ki.Interval, ki.Limit)
		if err != nil {
			errs++
			continue
		}
		ss.Klines[ki.Interval] = klines
		if ss.KlineFetchedAt == nil {
			ss.KlineFetchedAt = make(map[string]time.Time)
		}
		ss.KlineFetchedAt[ki.Interval] = now()

		// Compute taker buy ratio from 1m klines if available
		if ki.Interval == "1m" && len(klines) > 0 {
			var totalVol, takerBuy float64
			for _, k := range klines {
				totalVol += k.Volume
				takerBuy += k.TakerBuy
			}
			if totalVol > 0 {
				ss.TakerBuyRatio = takerBuy / totalVol
			}
		}
	}

	// The bulk phase stamped this snapshot when the 24hr ticker batch was
	// parsed, but the detail phase runs for another 50-90s afterwards. Re-stamp
	// with the moment this symbol's own live data landed, and carry the latest
	// 1m close forward as the current price, so downstream freshness checks
	// measure real per-symbol age instead of the whole batch duration.
	if k1m := ss.Klines["1m"]; len(k1m) > 0 {
		if last := k1m[len(k1m)-1]; last.Close > 0 {
			ss.Price = last.Close
			ss.PriceAt = now()
		}
	}
	ss.Timestamp = now()

	totalFetches := 1 + len(klineIntervals)
	if refreshSlowDerivatives {
		totalFetches += 2
		if f.cfg.IncludeT2Inputs {
			totalFetches += 2
		}
	}
	if errs > 0 {
		return ss, fmt.Errorf("%s: %d/%d fetches failed", symbol, errs, totalFetches)
	}
	return ss, nil
}

// RefreshSymbol fetches fresh single-symbol execution data without running a
// full market snapshot cycle. It refreshes ticker, mark/funding, current OI,
// configured klines, and optionally slow derivative context.
func (f *DataFetcher) RefreshSymbol(ctx context.Context, symbol string, base *SymbolSnapshot, klineIntervals []KlineInterval, refreshSlowDerivatives bool) (*SymbolSnapshot, error) {
	if f == nil {
		return nil, fmt.Errorf("nil data fetcher")
	}
	if symbol == "" {
		return nil, fmt.Errorf("empty symbol")
	}
	if len(klineIntervals) == 0 {
		klineIntervals = FastKlineIntervals
	}

	ss := cloneSymbolSnapshot(base)
	if ss == nil {
		ss = &SymbolSnapshot{
			Symbol: symbol,
			Klines: make(map[string][]Kline),
		}
	}
	ss.Symbol = symbol

	var errs int
	ticker, err := f.fetchTicker(ctx, symbol)
	if err != nil {
		errs++
	} else {
		applyTickerToSnapshot(ss, ticker)
	}
	premium, err := f.fetchPremiumIndex(ctx, symbol)
	if err != nil {
		errs++
	} else {
		applyPremiumToSnapshot(ss, premium)
	}

	ss.Timestamp = now()
	refreshed, err := f.fetchOneSymbol(ctx, symbol, ss, klineIntervals, refreshSlowDerivatives)
	if err != nil {
		errs++
	}
	if errs > 0 {
		return refreshed, fmt.Errorf("%s: %d single-symbol refresh phases failed", symbol, errs)
	}
	return refreshed, nil
}

func applyTickerToSnapshot(ss *SymbolSnapshot, t *ticker24hrRaw) {
	if ss == nil || t == nil {
		return
	}
	ss.Price = parseFloat(t.LastPrice)
	ss.PriceChange24h = parseFloat(t.PriceChangePercent)
	ss.Volume24h = parseFloat(t.Volume)
	ss.QuoteVolume24h = parseFloat(t.QuoteVolume)
	ss.HighPrice24h = parseFloat(t.HighPrice)
	ss.LowPrice24h = parseFloat(t.LowPrice)
	ss.TradeCount24h = t.Count
}

func applyPremiumToSnapshot(ss *SymbolSnapshot, p *premiumIndexRaw) {
	if ss == nil || p == nil {
		return
	}
	ss.MarkPrice = parseFloat(p.MarkPrice)
	ss.IndexPrice = parseFloat(p.IndexPrice)
	ss.FundingRate = parseFloat(p.LastFundingRate)
	ss.NextFundingTime = p.NextFundingTime
	if ss.IndexPrice > 0 {
		ss.Spread = (ss.MarkPrice - ss.IndexPrice) / ss.IndexPrice * 100
	}
}

func cloneSymbolSnapshot(src *SymbolSnapshot) *SymbolSnapshot {
	if src == nil {
		return nil
	}
	cp := *src
	if src.Klines != nil {
		cp.Klines = make(map[string][]Kline, len(src.Klines))
		for interval, klines := range src.Klines {
			copied := make([]Kline, len(klines))
			copy(copied, klines)
			cp.Klines[interval] = copied
		}
	} else {
		cp.Klines = make(map[string][]Kline)
	}
	if src.OISpikeData != nil {
		cp.OISpikeData = make([]float64, len(src.OISpikeData))
		copy(cp.OISpikeData, src.OISpikeData)
	}
	if src.DepthBids != nil {
		cp.DepthBids = make([]DepthLevel, len(src.DepthBids))
		copy(cp.DepthBids, src.DepthBids)
	}
	if src.DepthAsks != nil {
		cp.DepthAsks = make([]DepthLevel, len(src.DepthAsks))
		copy(cp.DepthAsks, src.DepthAsks)
	}
	if src.OI5mHistory != nil {
		cp.OI5mHistory = make([]OIHistoryPoint, len(src.OI5mHistory))
		copy(cp.OI5mHistory, src.OI5mHistory)
	}
	cp.KlineFetchedAt = copyTimeMap(src.KlineFetchedAt)
	return &cp
}

func copyTimeMap(src map[string]time.Time) map[string]time.Time {
	if src == nil {
		return nil
	}
	out := make(map[string]time.Time, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// fetchOI fetches current open interest for a symbol (USDT value).
func (f *DataFetcher) fetchOI(ctx context.Context, symbol string) (float64, error) {
	path := fmt.Sprintf("/fapi/v1/openInterest?symbol=%s", url.QueryEscape(symbol))
	raw, err := fetchJSON[oiRaw](ctx, f.client, path)
	if err != nil {
		return 0, err
	}
	return parseFloat(raw.OpenInterest), nil
}

// fetchOIHistory fetches OI history entries and returns period-over-period % changes.
func (f *DataFetcher) fetchOIHistory(ctx context.Context, symbol, period string, limit int) ([]float64, error) {
	path := fmt.Sprintf("/futures/data/openInterestHist?symbol=%s&period=%s&limit=%d", url.QueryEscape(symbol), period, limit)
	entries, err := fetchJSON[[]oiHistEntry](ctx, f.client, path)
	if err != nil {
		return nil, err
	}
	if len(entries) < 2 {
		return nil, nil
	}
	deltas := make([]float64, 0, len(entries)-1)
	for i := 1; i < len(entries); i++ {
		prev := parseFloat(entries[i-1].SumOpenInterestValue)
		curr := parseFloat(entries[i].SumOpenInterestValue)
		if prev > 0 {
			deltas = append(deltas, (curr-prev)/prev*100)
		} else {
			deltas = append(deltas, 0)
		}
	}
	return deltas, nil
}

// fetchDepth fetches an order-book depth snapshot (GET /fapi/v1/depth,
// limit=100 — the same request the task-2 scanner made). Levels are
// returned best-first, exactly as the endpoint orders them.
func (f *DataFetcher) fetchDepth(ctx context.Context, symbol string) ([]DepthLevel, []DepthLevel, error) {
	path := fmt.Sprintf("/fapi/v1/depth?symbol=%s&limit=100", url.QueryEscape(symbol))
	raw, err := fetchJSON[depthRaw](ctx, f.client, path)
	if err != nil {
		return nil, nil, err
	}
	parse := func(rows [][]string) []DepthLevel {
		out := make([]DepthLevel, 0, len(rows))
		for _, r := range rows {
			if len(r) < 2 {
				continue
			}
			out = append(out, DepthLevel{Price: parseFloat(r[0]), Qty: parseFloat(r[1])})
		}
		return out
	}
	return parse(raw.Bids), parse(raw.Asks), nil
}

// fetchOIHistorySeries fetches raw openInterestHist entries (oldest-first)
// for the given period. Unlike fetchOIHistory it returns the parsed series
// itself — notional value AND contract count — because the task-2 detector
// applies its own series selection (spec section 2.4: positive notional
// values preferred, contract count as fallback).
func (f *DataFetcher) fetchOIHistorySeries(ctx context.Context, symbol, period string, limit int) ([]OIHistoryPoint, error) {
	path := fmt.Sprintf("/futures/data/openInterestHist?symbol=%s&period=%s&limit=%d", url.QueryEscape(symbol), period, limit)
	entries, err := fetchJSON[[]oiHistEntry](ctx, f.client, path)
	if err != nil {
		return nil, err
	}
	out := make([]OIHistoryPoint, 0, len(entries))
	for _, e := range entries {
		out = append(out, OIHistoryPoint{
			Timestamp:            e.Timestamp,
			SumOpenInterest:      parseFloat(e.SumOpenInterest),
			SumOpenInterestValue: parseFloat(e.SumOpenInterestValue),
		})
	}
	return out, nil
}

// fetchLSR fetches top trader long/short ratio entries.
func (f *DataFetcher) fetchLSR(ctx context.Context, symbol string, limit int) ([]float64, error) {
	path := fmt.Sprintf("/futures/data/topLongShortPositionRatio?symbol=%s&period=1h&limit=%d", url.QueryEscape(symbol), limit)
	entries, err := fetchJSON[[]lsrEntry](ctx, f.client, path)
	if err != nil {
		return nil, err
	}
	out := make([]float64, 0, len(entries))
	for _, e := range entries {
		out = append(out, parseFloat(e.LongShortRatio))
	}
	return out, nil
}

// fetchKlines fetches klines from Binance Futures and parses them.
func (f *DataFetcher) fetchKlines(ctx context.Context, symbol, interval string, limit int) ([]Kline, error) {
	path := fmt.Sprintf("/fapi/v1/klines?symbol=%s&interval=%s&limit=%d", url.QueryEscape(symbol), interval, limit)
	body, err := f.client.doPublic(ctx, path)
	if err != nil {
		return nil, err
	}
	// Klines come as nested arrays — parse manually
	var raw [][]interface{}
	if err := jsonUnmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("klines unmarshal %s %s: %w", symbol, interval, err)
	}
	return parseKlines(raw), nil
}

// FetchKlines fetches Binance Futures klines for a single symbol/interval.
// It is intentionally narrow so higher-level services can backfill short
// execution windows without running a full snapshot cycle.
func (f *DataFetcher) FetchKlines(ctx context.Context, symbol, interval string, limit int) ([]Kline, error) {
	if f == nil {
		return nil, fmt.Errorf("nil data fetcher")
	}
	if limit <= 0 {
		limit = 100
	}
	return f.fetchKlines(ctx, symbol, interval, limit)
}

func init() {
	// Suppress unused import warning for log
	_ = log.Println
}
