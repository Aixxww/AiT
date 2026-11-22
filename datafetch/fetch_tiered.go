package datafetch

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync/atomic"
	"time"
)

// Fix 05 (Tier A screening evaluation P0-2): two-tier fetch covering every
// symbol.
//
//   - Fast tier, every fetch, ALL symbols: open interest + FastKlines
//     (default 1m x60, 5m x50, 15m x120). About 4 requests / 5 weight per
//     symbol.
//   - T2 inputs, every fetch, the top T2InputsTopN symbols of the detail
//     selector (in its launch order) when IncludeT2Inputs is set: depth@100
//     and the 5m openInterestHist series, fresh every round. Other symbols
//     carry neither (the T2 detector fails closed for them, as before for
//     non-detail symbols).
//   - Slow tier, rolling: 1/SlowEvery of all symbols per fetch (stable
//     slot per symbol): 1h openInterestHist (OI spike/delta), top-trader
//     long/short ratio and SlowKlines (default 1h x100, 4h x100, 1d x50).
//     The other symbols carry their last slow values from this fetcher's
//     previous fetches. Slow fields are therefore up to SlowEvery fetches
//     old (the production DataCollector's design).
//
// Warm-up: a symbol has no slow data until its first slot (up to SlowEvery
// fetches after the fetcher is created, e.g. after a harness restart). It
// is counted in SnapshotMeta.SlowMissing.
//
// Launch order: the selector's top T2InputsTopN in its launch order, then
// every other symbol by quote volume, so a deadline truncation drops the
// least important symbols.

// TieredFetchConfig configures the two-tier fetch.
type TieredFetchConfig struct {
	SlowEvery    int             // N; default 4
	FastKlines   []KlineInterval // default 1m x60, 5m x50, 15m x120
	SlowKlines   []KlineInterval // default 1h x100, 4h x100, 1d x50
	T2InputsTopN int             // default FetcherConfig.TopNForDetail
}

// DefaultTieredFastKlines / DefaultTieredSlowKlines split DefaultKlineIntervals.
var (
	DefaultTieredFastKlines = []KlineInterval{{Interval: "1m", Limit: 60}, {Interval: "5m", Limit: 50}, {Interval: "15m", Limit: 120}}
	DefaultTieredSlowKlines = []KlineInterval{{Interval: "1h", Limit: 100}, {Interval: "4h", Limit: 100}, {Interval: "1d", Limit: 50}}
)

func (c TieredFetchConfig) withDefaults(topN int) TieredFetchConfig {
	if c.SlowEvery <= 0 {
		c.SlowEvery = 4
	}
	if len(c.FastKlines) == 0 {
		c.FastKlines = DefaultTieredFastKlines
	}
	if len(c.SlowKlines) == 0 {
		c.SlowKlines = DefaultTieredSlowKlines
	}
	if c.T2InputsTopN <= 0 {
		c.T2InputsTopN = topN
	}
	return c
}

// tieredState is the carried slow data between fetches.
type tieredState struct {
	fetches int
	slow    map[string]*slowFields
}

// slowFields is one symbol's slow-tier data. at is when it was fetched; when
// some fields were kept from an earlier fetch (a slow request failed), at is
// that earlier time, so the age is never understated.
type slowFields struct {
	at            time.Time
	oiSpike       []float64
	oiDelta1h     float64
	oiDelta4h     float64
	lsr, lsrPrev  float64
	lsrOldest     float64
	klines        map[string][]Kline
	hasOI, hasLSR bool
}

// tierSlot is a stable slot in 0..n-1 for a symbol.
func tierSlot(symbol string, n int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(symbol))
	return int(h.Sum32() % uint32(n))
}

// tieredOrder is the launch order: the selector's top n in its launch order,
// then the rest by quote volume (symbols arrive sorted by volume).
func tieredOrder(symbols []string, tickers map[string]*ticker24hrRaw, premiums map[string]*premiumIndexRaw, topN int) (order []string, top map[string]bool) {
	head := selectDetailSymbols(symbols, tickers, premiums, topN)
	top = make(map[string]bool, len(head))
	for _, s := range head {
		top[s] = true
	}
	order = append(order, head...)
	for _, s := range symbols {
		if !top[s] {
			order = append(order, s)
		}
	}
	return order, top
}

// fetchOneSymbolTiered fetches one symbol's fast tier, its T2 inputs when in
// the top set, and its slow tier when due; otherwise it carries prev.
// It returns the snapshot, the slow data to carry (nil = none yet) and
// whether the slow data was refreshed this time.
func (f *DataFetcher) fetchOneSymbolTiered(ctx context.Context, symbol string, base *SymbolSnapshot, tc TieredFetchConfig, inTop, slowDue bool, prev *slowFields) (*SymbolSnapshot, *slowFields, bool, error) {
	ss := base
	if ss == nil {
		ss = &SymbolSnapshot{Symbol: symbol, Klines: make(map[string][]Kline)}
	}
	var errs, total int
	try := func(err error) bool {
		total++
		if err != nil {
			errs++
			return false
		}
		return true
	}

	// Fast tier.
	if oi, err := f.fetchOI(ctx, symbol); try(err) {
		ss.OI = oi
	}
	for _, ki := range tc.FastKlines {
		k, err := f.fetchKlines(ctx, symbol, ki.Interval, ki.Limit)
		if !try(err) {
			continue
		}
		ss.Klines[ki.Interval] = k
		if ki.Interval == "1m" && len(k) > 0 {
			var vol, buy float64
			for _, b := range k {
				vol += b.Volume
				buy += b.TakerBuy
			}
			if vol > 0 {
				ss.TakerBuyRatio = buy / vol
			}
		}
	}

	// T2 inputs: fresh every fetch, top set only, never carried. A failed
	// fetch leaves the field empty (the T2 detector fails closed).
	if f.cfg.IncludeT2Inputs && inTop {
		if bids, asks, err := f.fetchDepth(ctx, symbol); try(err) {
			ss.DepthBids, ss.DepthAsks = bids, asks
		}
		if oi5m, err := f.fetchOIHistorySeries(ctx, symbol, "5m", 30); try(err) {
			ss.OI5mHistory = oi5m
		}
	}

	// Slow tier.
	next, refreshed := prev, false
	if slowDue {
		sf := &slowFields{at: now(), klines: map[string][]Kline{}}
		if hist, err := f.fetchOIHistory(ctx, symbol, "1h", 13); try(err) {
			refreshed = true
			sf.hasOI, sf.oiSpike = true, hist
			if len(hist) >= 2 {
				sf.oiDelta1h = hist[len(hist)-1]
			}
			if len(hist) >= 5 {
				for i := len(hist) - 4; i < len(hist); i++ {
					sf.oiDelta4h += hist[i]
				}
			}
		}
		if lsr, err := f.fetchLSR(ctx, symbol, 12); try(err) {
			refreshed = true
			sf.hasLSR = true
			if len(lsr) >= 1 {
				sf.lsr, sf.lsrOldest = lsr[len(lsr)-1], lsr[0]
			}
			if len(lsr) >= 2 {
				sf.lsrPrev = lsr[len(lsr)-2]
			}
		}
		for _, ki := range tc.SlowKlines {
			k, err := f.fetchKlines(ctx, symbol, ki.Interval, ki.Limit)
			if !try(err) {
				continue
			}
			refreshed = true
			sf.klines[ki.Interval] = k
		}
		if refreshed {
			// Keep previous values for any slow request that failed this
			// time; the age is then the older fetch's.
			if prev != nil {
				kept := false
				if !sf.hasOI && prev.hasOI {
					sf.hasOI, sf.oiSpike, sf.oiDelta1h, sf.oiDelta4h = true, prev.oiSpike, prev.oiDelta1h, prev.oiDelta4h
					kept = true
				}
				if !sf.hasLSR && prev.hasLSR {
					sf.hasLSR, sf.lsr, sf.lsrPrev, sf.lsrOldest = true, prev.lsr, prev.lsrPrev, prev.lsrOldest
					kept = true
				}
				for iv, k := range prev.klines {
					if _, ok := sf.klines[iv]; !ok {
						sf.klines[iv] = k
						kept = true
					}
				}
				if kept {
					sf.at = prev.at
				}
			}
			next = sf
		}
	}
	if next != nil {
		if next.hasOI {
			ss.OISpikeData, ss.OIDelta1h, ss.OIDelta4h = next.oiSpike, next.oiDelta1h, next.oiDelta4h
		}
		if next.hasLSR {
			ss.LongShortRatio, ss.LSRPrev, ss.LSROldest = next.lsr, next.lsrPrev, next.lsrOldest
		}
		for iv, k := range next.klines {
			ss.Klines[iv] = k
		}
	}

	if k1m := ss.Klines["1m"]; len(k1m) > 0 {
		if last := k1m[len(k1m)-1]; last.Close > 0 {
			ss.Price = last.Close
		}
	}
	ss.Timestamp = now()
	if errs > 0 {
		return ss, next, refreshed, fmt.Errorf("%s: %d/%d fetches failed", symbol, errs, total)
	}
	return ss, next, refreshed, nil
}

// slowDueFor decides whether a symbol's slow tier is fetched this time: each
// symbol exactly once every SlowEvery fetches.
func slowDueFor(symbol string, fetchIndex int, tc TieredFetchConfig) bool {
	return tierSlot(symbol, tc.SlowEvery) == fetchIndex%tc.SlowEvery
}

// tieredMeta is the SnapshotMeta part of one tiered fetch.
type tieredMeta struct {
	slowEvery, fetchIndex, topN             int
	slowRefreshed, slowCarried, slowMissing int
	slowMaxAge                              time.Duration
}

type tieredResult struct {
	symbol    string
	snap      *SymbolSnapshot
	next      *slowFields
	refreshed bool
}

// fetchTieredData is the two-tier per-symbol pass over every symbol
// (f.symbols, sorted by quote volume). Every symbol is requested; the
// launch order is tieredOrder's.
func (f *DataFetcher) fetchTieredData(ctx context.Context, tickers map[string]*ticker24hrRaw, premiums map[string]*premiumIndexRaw) (map[string]*SymbolSnapshot, detailCounts, tieredMeta) {
	tc := f.cfg.Tiered.withDefaults(f.cfg.TopNForDetail)

	f.tieredMu.Lock()
	defer f.tieredMu.Unlock()
	if f.tiered == nil {
		f.tiered = &tieredState{slow: make(map[string]*slowFields)}
	}
	st := f.tiered
	fetchIndex := st.fetches
	st.fetches++

	order, top := tieredOrder(f.symbols, tickers, premiums, tc.T2InputsTopN)
	all := bulkBaseSnapshots(f.symbols, tickers, premiums)

	workers := f.cfg.MaxWorkers
	if workers <= 0 {
		workers = 5
	}
	sem := make(chan struct{}, workers)
	results := make(chan tieredResult, len(order))
	var errCount, completed, skipped int64

	for _, sym := range order {
		sym := sym
		base := all[sym]
		prev := st.slow[sym] // read here: the map is only written below, after the pool
		due := slowDueFor(sym, fetchIndex, tc)
		inTop := top[sym]
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			if ctx.Err() != nil {
				atomic.AddInt64(&skipped, 1)
				return
			}
			atomic.AddInt64(&completed, 1)
			ss, next, refreshed, err := f.fetchOneSymbolTiered(ctx, sym, base, tc, inTop, due, prev)
			if err != nil {
				atomic.AddInt64(&errCount, 1)
			}
			results <- tieredResult{symbol: sym, snap: ss, next: next, refreshed: refreshed}
		}()
	}
	go func() {
		for i := 0; i < cap(sem); i++ {
			sem <- struct{}{}
		}
		close(results)
	}()

	tm := tieredMeta{slowEvery: tc.SlowEvery, fetchIndex: fetchIndex}
	if f.cfg.IncludeT2Inputs {
		tm.topN = len(top)
	}
	for r := range results {
		all[r.symbol] = r.snap
		switch {
		case r.next == nil:
			tm.slowMissing++
			continue
		case r.refreshed:
			tm.slowRefreshed++
		default:
			tm.slowCarried++
		}
		st.slow[r.symbol] = r.next
		if age := now().Sub(r.next.at); age > tm.slowMaxAge {
			tm.slowMaxAge = age
		}
	}
	return all, detailCounts{
		symbolErrors: int(atomic.LoadInt64(&errCount)),
		requested:    len(order),
		completed:    int(atomic.LoadInt64(&completed)),
		skipped:      int(atomic.LoadInt64(&skipped)),
	}, tm
}
