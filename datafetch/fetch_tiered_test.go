package datafetch

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Fix 05: two-tier fetch, weight limiter and /futures/data limiter.

func TestBinanceRequestWeight(t *testing.T) {
	cases := map[string]float64{
		"/fapi/v1/time":                                                   1,
		"/fapi/v1/openInterest?symbol=BTCUSDT":                            1,
		"/fapi/v1/klines?symbol=BTCUSDT&interval=1m&limit=60":             1,
		"/fapi/v1/klines?symbol=BTCUSDT&interval=5m&limit=50":             1,
		"/fapi/v1/klines?symbol=BTCUSDT&interval=1h&limit=100":            1,
		"/fapi/v1/klines?symbol=BTCUSDT&interval=1d&limit=50":             1,
		"/fapi/v1/klines?symbol=BTCUSDT&interval=15m&limit=120":           2,
		"/fapi/v1/klines?symbol=BTCUSDT&interval=1m&limit=1000":           5,
		"/fapi/v1/klines?symbol=BTCUSDT&interval=1m&limit=1500":           10,
		"/fapi/v1/klines?symbol=BTCUSDT&interval=1m":                      2,
		"/fapi/v1/depth?symbol=BTCUSDT&limit=100":                         5,
		"/fapi/v1/depth?symbol=BTCUSDT&limit=20":                          2,
		"/fapi/v1/depth?symbol=BTCUSDT&limit=500":                         10,
		"/fapi/v1/premiumIndex":                                           10,
		"/fapi/v1/premiumIndex?symbol=BTCUSDT":                            1,
		"/fapi/v1/ticker/24hr":                                            40,
		"/fapi/v1/ticker/24hr?symbol=BTCUSDT":                             1,
		"/fapi/v1/exchangeInfo":                                           1,
		"/futures/data/openInterestHist?symbol=BTCUSDT&period=5m":         0,
		"/futures/data/topLongShortPositionRatio?symbol=BTCUSDT&limit=12": 0,
	}
	for path, want := range cases {
		if got := binanceRequestWeight(path); got != want {
			t.Errorf("%s: weight %v, want %v", path, got, want)
		}
	}
}

func TestRateLimiterAcquireNAndBurstCap(t *testing.T) {
	// 100 tokens/s, burst 10.
	l := NewRateLimiterBurst(100, time.Second, 10)
	start := time.Now()
	l.AcquireN(10)
	if d := time.Since(start); d > 20*time.Millisecond {
		t.Fatalf("the full burst must pass at once, took %v", d)
	}
	start = time.Now()
	l.AcquireN(10)
	if d := time.Since(start); d < 70*time.Millisecond || d > 400*time.Millisecond {
		t.Fatalf("10 more tokens at 100/s must wait ~100ms, took %v", d)
	}
	// A request heavier than the bucket is capped at the bucket size.
	start = time.Now()
	l.AcquireN(50)
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("n above the burst must be capped at the bucket, took %v", d)
	}
	l.AcquireN(0)
	var nilL *RateLimiter
	nilL.AcquireN(1) // must not panic
	if l.maxTokens != 10 {
		t.Fatalf("burst %v", l.maxTokens)
	}
}

// The configured limiters: weight 1,800/min with a 10% burst (at most 1,980
// in any minute, under 2,400); /futures/data burst 40% + refill 60% per 5
// minutes (never more than the limit in any 5-minute window).
func TestFetcherLimiterConfig(t *testing.T) {
	c := newFetcherHTTPClient(FetcherConfig{WeightLimitPerMin: 1800, FuturesDataLimitPer5Min: 1000})
	if c.weightLimiter == nil || c.futuresDataLimiter == nil {
		t.Fatal("limiters not set")
	}
	w := c.weightLimiter
	if w.maxTokens != 180 || math.Abs(w.refillRate-30) > 1e-9 {
		t.Fatalf("weight limiter burst %v rate %v/s, want 180 and 30", w.maxTokens, w.refillRate)
	}
	if maxPerMin := w.maxTokens + w.refillRate*60; maxPerMin > 2400 {
		t.Fatalf("weight limiter admits %v per minute, over 2400", maxPerMin)
	}
	fd := c.futuresDataLimiter
	if maxPer5 := fd.maxTokens + fd.refillRate*300; math.Abs(maxPer5-1000) > 1e-6 {
		t.Fatalf("futures-data limiter admits %v per 5 min, want 1000", maxPer5)
	}
	if fd.maxTokens != 400 {
		t.Fatalf("futures-data burst %v, want 400", fd.maxTokens)
	}
}

// acquire charges the weight table to the weight limiter and 1 per
// /futures/data request to its own limiter.
func TestHTTPClientAcquireCharges(t *testing.T) {
	c := newFetcherHTTPClient(FetcherConfig{WeightLimitPerMin: 1800, FuturesDataLimitPer5Min: 1000})
	c.acquire(context.Background(), "/fapi/v1/depth?symbol=BTCUSDT&limit=100")
	if got := c.weightLimiter.tokens; got < 174.9 || got > 175.5 {
		t.Fatalf("depth must cost 5 weight: tokens %v", got)
	}
	if c.futuresDataLimiter.tokens != 400 {
		t.Fatalf("a /fapi request must not charge the futures-data limiter: %v", c.futuresDataLimiter.tokens)
	}
	c.acquire(context.Background(), "/futures/data/openInterestHist?symbol=BTCUSDT&period=5m&limit=30")
	if got := c.futuresDataLimiter.tokens; got < 398.9 || got > 399.1 {
		t.Fatalf("a /futures/data request costs 1: tokens %v", got)
	}
	if got := c.weightLimiter.tokens; got < 174.9 || got > 175.5 {
		t.Fatalf("/futures/data carries no IP weight: tokens %v", got)
	}
}

// Live callers set none of the fix 05 fields: the legacy limiter and the
// single-tier fetch are unchanged.
func TestTieredOffByDefault(t *testing.T) {
	f := NewDataFetcher(FetcherConfig{})
	if f.cfg.Tiered != nil || f.client.weightLimiter != nil || f.client.futuresDataLimiter != nil {
		t.Fatal("fix 05 must be opt-in")
	}
	c := newFetcherHTTPClient(FetcherConfig{})
	before := c.limiter.tokens
	c.acquire(context.Background(), "/fapi/v1/depth?symbol=BTCUSDT&limit=100")
	if got := before - c.limiter.tokens; got < 0.9 || got > 1.1 {
		t.Fatalf("legacy limiter must charge 1 per request, charged %v", got)
	}
}

func TestSlowDueOncePerCycle(t *testing.T) {
	tc := TieredFetchConfig{}.withDefaults(220)
	if tc.SlowEvery != 4 || tc.T2InputsTopN != 220 {
		t.Fatalf("defaults %+v", tc)
	}
	for i := 0; i < 600; i++ {
		sym := fmt.Sprintf("S%dUSDT", i)
		due := 0
		for k := 0; k < tc.SlowEvery; k++ {
			if slowDueFor(sym, 7+k, tc) {
				due++
			}
		}
		if due != 1 {
			t.Fatalf("%s due %d times in %d consecutive fetches", sym, due, tc.SlowEvery)
		}
	}
}

// tieredRecorder records per-symbol requests against statsMux plus depth.
type tieredRecorder struct {
	mu   sync.Mutex
	hits []string // "<path>|<symbol>|<interval or period>"
	fail map[string]bool
}

func (r *tieredRecorder) mux(symbols []string) http.Handler {
	inner := statsMux(symbols, nil)
	inner.HandleFunc("/fapi/v1/depth", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, `{"bids":[["99.9","10"]],"asks":[["100.1","10"]]}`)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		if sym := q.Get("symbol"); sym != "" {
			key := req.URL.Path + "|" + sym + "|" + q.Get("interval") + q.Get("period")
			r.mu.Lock()
			r.hits = append(r.hits, key)
			failing := r.fail[req.URL.Path]
			r.mu.Unlock()
			if failing {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"code":-1121,"msg":"Invalid symbol."}`)
				return
			}
		}
		inner.ServeHTTP(w, req)
	})
}

func (r *tieredRecorder) take() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	for _, h := range r.hits {
		out[h]++
	}
	r.hits = nil
	return out
}

func tieredSymbols(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("S%02dUSDT", i)
	}
	return out
}

func TestTieredFetchCoversEverySymbol(t *testing.T) {
	syms := tieredSymbols(12)
	rec := &tieredRecorder{}
	mux := http.NewServeMux()
	mux.Handle("/", rec.mux(syms))
	server := startT2TestServer(t, mux)
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, TopNForDetail: 4, MaxWorkers: 3, IncludeT2Inputs: true,
		Tiered: &TieredFetchConfig{SlowEvery: 3}})

	slowSeen := map[string]int{}
	for fetch := 0; fetch < 3; fetch++ {
		snap, err := f.Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		hits := rec.take()
		m := snap.Meta
		if !m.Tiered || m.SlowEvery != 3 || m.FetchIndex != fetch || m.T2InputsTopN != 4 {
			t.Fatalf("fetch %d meta %+v", fetch, m)
		}
		if m.DetailRequested != 12 || m.DetailCompleted != 12 || m.DetailSkippedDeadline != 0 {
			t.Fatalf("every symbol must be requested and fetched: %+v", m)
		}
		if m.SlowRefreshed+m.SlowCarried+m.SlowMissing != m.DetailCompleted {
			t.Fatalf("slow counts must add up: %+v", m)
		}
		top := map[string]bool{}
		for _, s := range syms {
			ss := snap.Symbols[s]
			// Fast tier: every symbol, every fetch.
			for _, iv := range []string{"1m", "5m", "15m"} {
				if hits["/fapi/v1/klines|"+s+"|"+iv] != 1 || len(ss.Klines[iv]) == 0 {
					t.Fatalf("fetch %d %s: fast kline %s missing", fetch, s, iv)
				}
			}
			if hits["/fapi/v1/openInterest|"+s+"|"] != 1 || ss.OI == 0 {
				t.Fatalf("fetch %d %s: open interest missing", fetch, s)
			}
			// T2 inputs: the top set only, fresh, never carried.
			depth := hits["/fapi/v1/depth|"+s+"|"]
			oi5 := hits["/futures/data/openInterestHist|"+s+"|5m"]
			if depth != oi5 || depth > 1 {
				t.Fatalf("fetch %d %s: depth %d 5m OI %d", fetch, s, depth, oi5)
			}
			if depth == 1 {
				top[s] = true
				if len(ss.DepthBids) == 0 || len(ss.OI5mHistory) == 0 {
					t.Fatalf("fetch %d %s: top symbol without T2 inputs", fetch, s)
				}
			} else if len(ss.DepthBids) != 0 || len(ss.OI5mHistory) != 0 {
				t.Fatalf("fetch %d %s: T2 inputs outside the top set (carried?)", fetch, s)
			}
			// Slow tier: due symbols only.
			slow := hits["/futures/data/openInterestHist|"+s+"|1h"]
			lsr := hits["/futures/data/topLongShortPositionRatio|"+s+"|1h"]
			if slow != lsr || slow != hits["/fapi/v1/klines|"+s+"|1h"] || slow != hits["/fapi/v1/klines|"+s+"|4h"] || slow != hits["/fapi/v1/klines|"+s+"|1d"] {
				t.Fatalf("fetch %d %s: slow requests not fetched together", fetch, s)
			}
			if (slow == 1) != slowDueFor(s, fetch, f.cfg.Tiered.withDefaults(4)) {
				t.Fatalf("fetch %d %s: slow fetched=%d against its slot", fetch, s, slow)
			}
			slowSeen[s] += slow
			// Carry: once its slot has passed, a symbol has slow data.
			if slowSeen[s] > 0 && (len(ss.Klines["1h"]) == 0 || len(ss.Klines["4h"]) == 0 || len(ss.Klines["1d"]) == 0 || ss.LongShortRatio == 0 || len(ss.OISpikeData) == 0) {
				t.Fatalf("fetch %d %s: slow fields not carried", fetch, s)
			}
			if slowSeen[s] == 0 && (len(ss.Klines["1h"]) != 0 || ss.LongShortRatio != 0) {
				t.Fatalf("fetch %d %s: slow fields before the first slot", fetch, s)
			}
		}
		if len(top) != 4 {
			t.Fatalf("fetch %d: %d symbols with T2 inputs, want 4", fetch, len(top))
		}
		// Warm-up: missing = symbols whose slot has not come yet.
		missing := 0
		for _, s := range syms {
			if slowSeen[s] == 0 {
				missing++
			}
		}
		if m.SlowMissing != missing {
			t.Fatalf("fetch %d: SlowMissing %d, want %d", fetch, m.SlowMissing, missing)
		}
	}
	for _, s := range syms {
		if slowSeen[s] != 1 {
			t.Fatalf("%s slow-fetched %d times in 3 fetches, want exactly 1", s, slowSeen[s])
		}
	}
}

// The top T2-input set launches first, in the detail selector's order.
func TestTieredLaunchOrder(t *testing.T) {
	syms := tieredSymbols(8)
	var mu sync.Mutex
	var order []string
	rec := &tieredRecorder{}
	inner := rec.mux(syms)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fapi/v1/openInterest" {
			mu.Lock()
			order = append(order, r.URL.Query().Get("symbol"))
			mu.Unlock()
		}
		inner.ServeHTTP(w, r)
	})
	server := startT2TestServer(t, mux)
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, TopNForDetail: 3, MaxWorkers: 1, Tiered: &TieredFetchConfig{}})
	if _, err := f.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(order) != 8 {
		t.Fatalf("launched %d symbols, want 8: %v", len(order), order)
	}
	// statsMux: S00 has the highest volume; equal change/amplitude, so the
	// selector picks by volume.
	if fmt.Sprint(order) != fmt.Sprint(syms) {
		t.Fatalf("launch order %v, want %v", order, syms)
	}
	tickers := map[string]*ticker24hrRaw{"A": {QuoteVolume: "1"}, "B": {QuoteVolume: "3"}, "C": {QuoteVolume: "2"}}
	got, top := tieredOrder([]string{"B", "C", "A"}, tickers, nil, 1)
	if fmt.Sprint(got) != "[B C A]" || !top["B"] || len(top) != 1 {
		t.Fatalf("tieredOrder %v %v", got, top)
	}
}

// A failed slow request keeps the previous value and its (older) age; a
// slow pass where every request failed keeps prev as is; with no prev the
// symbol stays missing.
func TestTieredSlowFailureKeepsPrevious(t *testing.T) {
	syms := []string{"AUSDT"}
	rec := &tieredRecorder{fail: map[string]bool{"/futures/data/topLongShortPositionRatio": true}}
	mux := http.NewServeMux()
	mux.Handle("/", rec.mux(syms))
	server := startT2TestServer(t, mux)
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, MaxWorkers: 1})
	tc := TieredFetchConfig{}.withDefaults(1)
	old := time.Now().Add(-20 * time.Minute)
	prev := &slowFields{at: old, hasLSR: true, lsr: 2.5, lsrPrev: 2.4, lsrOldest: 2.0, klines: map[string][]Kline{}}

	ss, next, refreshed, err := f.fetchOneSymbolTiered(context.Background(), "AUSDT", nil, tc, false, true, prev)
	if err == nil {
		t.Fatal("the failed LSR request must be reported")
	}
	if !refreshed || next == prev || !next.hasOI || len(next.klines["1h"]) == 0 {
		t.Fatalf("the rest of the slow tier must refresh: refreshed=%v next=%+v", refreshed, next)
	}
	if ss.LongShortRatio != 2.5 || ss.LSRPrev != 2.4 || ss.LSROldest != 2.0 || !next.at.Equal(old) {
		t.Fatalf("LSR must be carried with its age: lsr=%v at=%v", ss.LongShortRatio, next.at)
	}

	rec.mu.Lock()
	rec.fail = map[string]bool{"/futures/data/topLongShortPositionRatio": true, "/futures/data/openInterestHist": true, "/fapi/v1/klines": true}
	rec.mu.Unlock()
	_, next, refreshed, _ = f.fetchOneSymbolTiered(context.Background(), "AUSDT", nil, tc, false, true, prev)
	if refreshed || next != prev {
		t.Fatal("an all-failed slow pass must keep prev unchanged")
	}
	_, next, refreshed, _ = f.fetchOneSymbolTiered(context.Background(), "AUSDT", nil, tc, false, true, nil)
	if refreshed || next != nil {
		t.Fatal("no prev and no slow data = missing")
	}
}

// SlowMaxAge reports the oldest carried slow data (faked clock).
func TestTieredSlowMaxAge(t *testing.T) {
	syms := tieredSymbols(6)
	rec := &tieredRecorder{}
	mux := http.NewServeMux()
	mux.Handle("/", rec.mux(syms))
	server := startT2TestServer(t, mux)
	base := time.Now()
	var mu sync.Mutex
	clock := base
	orig := now
	now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	t.Cleanup(func() { now = orig })
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, MaxWorkers: 2, Tiered: &TieredFetchConfig{SlowEvery: 2}})
	var ages []time.Duration
	for i := 0; i < 3; i++ {
		mu.Lock()
		clock = base.Add(time.Duration(i) * 6 * time.Minute)
		mu.Unlock()
		snap, err := f.Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ages = append(ages, snap.Meta.SlowMaxAge)
	}
	// Fetch 0 refreshes slot 0 (age 0); fetch 1 carries slot 0 (6m); fetch 2
	// refreshes slot 0 and carries slot 1 (6m).
	want := []time.Duration{0, 6 * time.Minute, 6 * time.Minute}
	slot0 := 0
	for _, s := range syms {
		if tierSlot(s, 2) == 0 {
			slot0++
		}
	}
	if slot0 == 0 || slot0 == len(syms) {
		t.Skip("symbol set does not cover both slots")
	}
	for i := range want {
		if ages[i] != want[i] {
			t.Fatalf("SlowMaxAge %v, want %v", ages, want)
		}
	}
}

// SetErrorSink redirects later failures (the harness reuses one fetcher).
func TestSetErrorSink(t *testing.T) {
	syms := []string{"AUSDT"}
	rec := &tieredRecorder{fail: map[string]bool{"/futures/data/topLongShortPositionRatio": true}}
	mux := http.NewServeMux()
	mux.Handle("/", rec.mux(syms))
	server := startT2TestServer(t, mux)
	var a, b []string
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, MaxWorkers: 1, Tiered: &TieredFetchConfig{SlowEvery: 1},
		ErrorSink: func(p string, _ error) { a = append(a, p) }})
	if _, err := f.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.SetErrorSink(func(p string, _ error) { b = append(b, p) })
	if _, err := f.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("sinks got %v / %v, want one failure each", a, b)
	}
}
