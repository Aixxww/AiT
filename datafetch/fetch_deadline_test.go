package datafetch

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"
)

// Fix 03: launch order and the fetch budget.

func orderTickers() ([]string, map[string]*ticker24hrRaw, map[string]*premiumIndexRaw) {
	symbols := []string{}
	tickers := map[string]*ticker24hrRaw{}
	add := func(sym, vol, chg, hi, lo string) {
		symbols = append(symbols, sym)
		tickers[sym] = &ticker24hrRaw{Symbol: sym, QuoteVolume: vol, PriceChangePercent: chg, HighPrice: hi, LowPrice: lo}
	}
	for i := 1; i <= 8; i++ {
		add(fmt.Sprintf("VOL%dUSDT", i), fmt.Sprintf("%d", 1000000000-i*1000000), "1", "1.01", "1.00")
	}
	add("AMP1USDT", "1000", "0", "2.0", "1.0")
	add("AMP2USDT", "1000", "0", "1.8", "1.0")
	add("GAIN1USDT", "1000", "40", "1.01", "1.00")
	add("GAIN2USDT", "1000", "30", "1.01", "1.00")
	add("LOSE1USDT", "1000", "-40", "1.01", "1.00")
	add("LOSE2USDT", "1000", "-30", "1.01", "1.00")
	add("FUND1USDT", "1000", "0", "1.01", "1.00")
	add("FUND2USDT", "1000", "0", "1.01", "1.00")
	premiums := map[string]*premiumIndexRaw{
		"FUND1USDT": {Symbol: "FUND1USDT", LastFundingRate: "0.003"},
		"FUND2USDT": {Symbol: "FUND2USDT", LastFundingRate: "-0.002"},
	}
	return symbols, tickers, premiums
}

// The strongest name of every quota launches first; the selected set is the
// same as the quota selection (only the order changes).
func TestSelectDetailSymbolsLaunchOrderRoundRobin(t *testing.T) {
	symbols, tickers, premiums := orderTickers()
	got := selectDetailSymbols(symbols, tickers, premiums, 14)
	// limit 14: volume 5, amplitude 2, gainers 2, losers 2, funding 1, fill 2.
	if len(got) != 14 {
		t.Fatalf("selected %d, want 14", len(got))
	}
	first := map[string]bool{}
	for _, s := range got[:6] {
		first[s] = true
	}
	for _, want := range []string{"VOL1USDT", "AMP1USDT", "GAIN1USDT", "LOSE1USDT", "FUND1USDT"} {
		if !first[want] {
			t.Fatalf("top of each quota must launch in the first round-robin pass: %v", got[:6])
		}
	}
	if got[0] != "VOL1USDT" || got[1] != "AMP1USDT" || got[2] != "GAIN1USDT" || got[3] != "LOSE1USDT" || got[4] != "FUND1USDT" {
		t.Fatalf("first pass order %v", got[:6])
	}
	// The weakest ranks launch last: the 5th volume name and the 2nd fill name.
	last := got[len(got)-1]
	if last != "VOL5USDT" {
		t.Fatalf("last launched %s, want VOL5USDT (rank 4 of the volume quota)", last)
	}
	// Same set as the quota selection.
	wantSet := []string{"VOL1USDT", "VOL2USDT", "VOL3USDT", "VOL4USDT", "VOL5USDT", "AMP1USDT", "AMP2USDT",
		"GAIN1USDT", "GAIN2USDT", "LOSE1USDT", "LOSE2USDT", "FUND1USDT", "VOL6USDT", "VOL7USDT"}
	g := append([]string(nil), got...)
	sort.Strings(g)
	sort.Strings(wantSet)
	for i := range g {
		if g[i] != wantSet[i] {
			t.Fatalf("selected set changed: %v, want %v", g, wantSet)
		}
	}
}

// Per-symbol workers start in launch order (one worker makes it observable).
func TestDetailWorkersStartInLaunchOrder(t *testing.T) {
	syms := []string{"AUSDT", "BUSDT", "CUSDT", "DUSDT"}
	inner := statsMux(syms, nil)
	var mu sync.Mutex
	var order []string
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
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, TopNForDetail: 10, MaxWorkers: 1,
		KlineIntervals: []KlineInterval{{Interval: "1m", Limit: 1}}})
	if _, err := f.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	// statsMux gives AUSDT the highest volume ... DUSDT the lowest.
	want := []string{"AUSDT", "BUSDT", "CUSDT", "DUSDT"}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("worker start order %v, want %v", order, want)
	}
}

// FetchBudget starts at the fetch call: an expired budget skips every
// detail symbol; a generous one skips none; and time spent before the call
// (the harness used to start its 3-minute clock at round entry) does not
// count against it.
func TestFetchBudgetMeasuredFromFetchStart(t *testing.T) {
	syms := []string{"AUSDT", "BUSDT", "CUSDT"}
	server := startT2TestServer(t, statsMux(syms, nil))
	mk := func(budget time.Duration) *DataFetcher {
		return NewDataFetcher(FetcherConfig{BinanceURL: server.URL, TopNForDetail: 3, MaxWorkers: 1, FetchBudget: budget,
			KlineIntervals: []KlineInterval{{Interval: "1m", Limit: 1}}})
	}
	snap, err := mk(time.Nanosecond).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.DetailSkippedDeadline != 3 || snap.Meta.DetailCompleted != 0 {
		t.Fatalf("expired budget: skipped %d completed %d, want 3/0", snap.Meta.DetailSkippedDeadline, snap.Meta.DetailCompleted)
	}
	f := mk(500 * time.Millisecond)
	time.Sleep(700 * time.Millisecond) // longer than the budget, before the fetch starts
	snap, err = f.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.DetailSkippedDeadline != 0 || snap.Meta.DetailCompleted != 3 {
		t.Fatalf("budget must start at the fetch: skipped %d completed %d", snap.Meta.DetailSkippedDeadline, snap.Meta.DetailCompleted)
	}
	// 0 keeps the old behaviour (caller context only).
	snap, _ = mk(0).Fetch(context.Background())
	if snap.Meta.DetailSkippedDeadline != 0 {
		t.Fatal("no budget must not skip")
	}
}
