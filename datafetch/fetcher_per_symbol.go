package datafetch

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
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
	klineIntervals []KlineInterval,
	refreshSlowDerivatives bool,
) (map[string]*SymbolSnapshot, int) {

	// Build partial snapshots from bulk data for ALL symbols first
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
		all[sym] = ss
	}

	// Semaphore-based worker pool
	workers := f.cfg.MaxWorkers
	if workers <= 0 {
		workers = 5
	}
	sem := make(chan struct{}, workers)
	results := make(chan perSymbolResult, len(detailSymbols))
	var errCount int64

	for _, sym := range detailSymbols {
		sym := sym
		base := all[sym]
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			ss, err := f.fetchOneSymbol(ctx, sym, base, klineIntervals, refreshSlowDerivatives)
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

	return all, int(atomic.LoadInt64(&errCount))
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
			if len(lsrData) >= 1 {
				ss.LongShortRatio = lsrData[len(lsrData)-1] // newest
				ss.LSROldest = lsrData[0]                   // oldest (for Hunter reversal)
			}
			if len(lsrData) >= 2 {
				ss.LSRPrev = lsrData[len(lsrData)-2]
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
		}
	}
	ss.Timestamp = now()

	totalFetches := 1 + len(klineIntervals)
	if refreshSlowDerivatives {
		totalFetches += 2
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
	return &cp
}

// fetchOI fetches current open interest for a symbol (USDT value).
func (f *DataFetcher) fetchOI(ctx context.Context, symbol string) (float64, error) {
	path := fmt.Sprintf("/fapi/v1/openInterest?symbol=%s", symbol)
	raw, err := fetchJSON[oiRaw](f.client, path)
	if err != nil {
		return 0, err
	}
	return parseFloat(raw.OpenInterest), nil
}

// fetchOIHistory fetches OI history entries and returns period-over-period % changes.
func (f *DataFetcher) fetchOIHistory(ctx context.Context, symbol, period string, limit int) ([]float64, error) {
	path := fmt.Sprintf("/futures/data/openInterestHist?symbol=%s&period=%s&limit=%d", symbol, period, limit)
	entries, err := fetchJSON[[]oiHistEntry](f.client, path)
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

// fetchLSR fetches top trader long/short ratio entries.
func (f *DataFetcher) fetchLSR(ctx context.Context, symbol string, limit int) ([]float64, error) {
	path := fmt.Sprintf("/futures/data/topLongShortPositionRatio?symbol=%s&period=1h&limit=%d", symbol, limit)
	entries, err := fetchJSON[[]lsrEntry](f.client, path)
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
	path := fmt.Sprintf("/fapi/v1/klines?symbol=%s&interval=%s&limit=%d", symbol, interval, limit)
	body, err := f.client.doPublic(path)
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
