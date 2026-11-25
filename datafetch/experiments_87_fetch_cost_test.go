package datafetch

// #87 measurement (gated by AIT_EXPERIMENTS=1): per-round request and weight
// cost of the REST plan before and after the layered fetch, and a sustained
// back-to-back run against a server that enforces Binance's 2400/min weight
// limit with 429 + Retry-After.
//
// Binance is not reachable from the sandbox (HTTP 451), so the "exchange"
// is a local fake with 540 USDT perps. Request counts and weights are the
// plan's own (request_weight.go); latency is not representative.
//
// Run: AIT_EXPERIMENTS=1 go test ./datafetch -run TestExperiment87 -v -count=1 -timeout 20m

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

type weightServer struct {
	noCap       bool // cost runs: count only, never 429
	mu          sync.Mutex
	window      time.Time
	used        int
	maxMinute   int
	rateLimited int
	requests    int
}

func (ws *weightServer) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		wt := int(binanceRequestWeight(path))
		ws.mu.Lock()
		now := time.Now().Truncate(time.Minute)
		if !now.Equal(ws.window) {
			ws.window, ws.used = now, 0
		}
		ws.requests++
		if !ws.noCap && ws.used+wt > BinanceWeightCapPerMin {
			ws.rateLimited++
			used := ws.used
			ws.mu.Unlock()
			w.Header().Set("X-MBX-USED-WEIGHT-1M", fmt.Sprint(used))
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		ws.used += wt
		if ws.used > ws.maxMinute {
			ws.maxMinute = ws.used
		}
		used := ws.used
		ws.mu.Unlock()
		if !ws.noCap {
			w.Header().Set("X-MBX-USED-WEIGHT-1M", fmt.Sprint(used))
		}
		next.ServeHTTP(w, r)
	})
}

func experimentSymbols(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("P%03dUSDT", i)
	}
	return out
}

func experimentMux(symbols []string) *http.ServeMux {
	mux := statsMux(symbols, nil)
	mux.HandleFunc("/fapi/v1/depth", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"bids":[["99","1"]],"asks":[["101","1"]]}`)
	})
	return mux
}

type roundCost struct{ requests, weight, slowDue int }

func summarize(t *testing.T, label string, rounds []roundCost) {
	t.Helper()
	var sr, sw, maxW, maxR int
	ws := make([]int, 0, len(rounds))
	for _, r := range rounds {
		sr += r.requests
		sw += r.weight
		ws = append(ws, r.weight)
		if r.weight > maxW {
			maxW = r.weight
		}
		if r.requests > maxR {
			maxR = r.requests
		}
	}
	sort.Ints(ws)
	n := len(rounds)
	t.Logf("EXP87 %s: rounds=%d mean_requests=%d mean_weight=%d max_requests=%d max_weight=%d min_weight=%d weight_per_min_at_30s(max 2 rounds)=%d",
		label, n, sr/n, sw/n, maxR, maxW, ws[0], 2*maxW)
}

func TestExperiment87_PerRoundCost(t *testing.T) {
	if os.Getenv("AIT_EXPERIMENTS") != "1" {
		t.Skip("measurement-only experiment; set AIT_EXPERIMENTS=1 to run")
	}
	syms := experimentSymbols(540)
	ws := &weightServer{noCap: true}
	mux := http.NewServeMux()
	mux.Handle("/", ws.wrap(experimentMux(syms)))
	server := startT2TestServer(t, mux)
	// Cost runs count the plan; pacing is measured by the sustained test.
	unthrottle := func(f *DataFetcher) {
		f.client.limiter = NewRateLimiter(1e9)
		f.client.weightLimiter = nil
		f.client.subBudgets = nil
		f.client.futuresDataLimiter = nil
	}

	for _, t2 := range []bool{false, true} {
		// BEFORE: the base collector plan (legacy limiter, all slow fields
		// and 4h klines on every 10th round, nothing on the others).
		before := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, MaxWorkers: 50, TopNForDetail: 100,
			KlineIntervals: DefaultKlineIntervals, IncludeT2Inputs: t2})
		unthrottle(before)
		var bRounds []roundCost
		for cycle := 1; cycle <= 20; cycle++ {
			kl := append([]KlineInterval{}, FastKlineIntervals...)
			slow := cycle%10 == 0
			if slow {
				kl = appendMissingKlineIntervals(kl, StructuralKlineIntervals...)
			}
			snap, err := before.FetchWithOptions(context.Background(), FetchOptions{KlineIntervals: kl, RefreshSlowDerivatives: slow})
			if err != nil {
				t.Fatal(err)
			}
			bRounds = append(bRounds, roundCost{snap.Meta.Requests, snap.Meta.Weight, 0})
		}
		summarize(t, fmt.Sprintf("BEFORE t2_inputs=%v", t2), bRounds)

		// AFTER: the #87 collector plan (rolling slow slot, 1d included,
		// weight limiter + sub-budgets), merges applied between rounds.
		store := NewStore()
		dc := NewDataCollector(CollectorConfig{BinanceURL: server.URL, IncludeT2Inputs: t2}, store)
		unthrottle(dc.fetcher)
		first, err := dc.fetcher.Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		store.Swap(first)
		var aRounds []roundCost
		for cycle := 1; cycle <= 20; cycle++ {
			cur := store.Current()
			snap, err := dc.fetcher.FetchWithOptions(context.Background(), dc.roundOptions(cur, cycle))
			if err != nil {
				t.Fatal(err)
			}
			mergeCarriedKlines(cur, snap, dc.cfg.FastKlines)
			mergeCarriedSlowDerivatives(cur, snap, dc.cfg.SlowFieldMaxAge)
			store.Swap(snap)
			aRounds = append(aRounds, roundCost{snap.Meta.Requests, snap.Meta.Weight, snap.Meta.SlowDue})
		}
		summarize(t, fmt.Sprintf("AFTER  t2_inputs=%v", t2), aRounds)
		due := 0
		for _, r := range aRounds {
			due += r.slowDue
		}
		t.Logf("EXP87 AFTER t2_inputs=%v: mean slow-due symbols per round=%.1f (initial full fetch excluded: requests=%d weight=%d)",
			t2, float64(due)/float64(len(aRounds)), first.Meta.Requests, first.Meta.Weight)
	}
	t.Logf("EXP87 finalist refresh (L4, 5 symbols, fast klines): requests=%d weight=%d per round", 5*7, 5*8)
}

// Sustained back-to-back rounds (no 30s pause, so far above the real
// cadence): the client-side limiter must keep the server's per-minute weight
// under 2400 with no 429.
func TestExperiment87_SustainedNo429(t *testing.T) {
	if os.Getenv("AIT_EXPERIMENTS") != "1" {
		t.Skip("measurement-only experiment; set AIT_EXPERIMENTS=1 to run")
	}
	syms := experimentSymbols(540)
	ws := &weightServer{}
	mux := http.NewServeMux()
	mux.Handle("/", ws.wrap(experimentMux(syms)))
	server := startT2TestServer(t, mux)
	store := NewStore()
	dc := NewDataCollector(CollectorConfig{BinanceURL: server.URL, IncludeT2Inputs: true}, store)
	start := time.Now()
	rounds := 0
	clientLimited := 0
	for time.Since(start) < 150*time.Second {
		rounds++
		cur := store.Current()
		snap, err := dc.fetcher.FetchWithOptions(context.Background(), dc.roundOptions(cur, rounds))
		if err != nil {
			t.Fatal(err)
		}
		clientLimited += snap.Meta.RateLimited
		if cur != nil {
			mergeCarriedKlines(cur, snap, dc.cfg.FastKlines)
		}
		mergeCarriedSlowDerivatives(cur, snap, dc.cfg.SlowFieldMaxAge)
		store.Swap(snap)
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	t.Logf("EXP87 sustained back-to-back: %d rounds in %v, server requests=%d, server max weight in one minute=%d (cap %d), server 429s=%d, client-seen 429/418=%d",
		rounds, time.Since(start).Round(time.Second), ws.requests, ws.maxMinute, BinanceWeightCapPerMin, ws.rateLimited, clientLimited)
	if ws.rateLimited > 0 {
		t.Fatalf("429 storm: %d rate-limited responses", ws.rateLimited)
	}
}
