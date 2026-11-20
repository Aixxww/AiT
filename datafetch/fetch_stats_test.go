package datafetch

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TA-5: per-fetch observability counters.

// statsMux serves a full Fetch for the given symbols. LSR requests for any
// symbol in lsrFail return HTTP 400 (non-retryable: exactly one failed
// request each).
func statsMux(symbols []string, lsrFail map[string]bool) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/exchangeInfo", func(w http.ResponseWriter, r *http.Request) {
		parts := make([]string, 0, len(symbols))
		for _, s := range symbols {
			parts = append(parts, fmt.Sprintf(`{"symbol":%q,"status":"TRADING","contractType":"PERPETUAL","quoteAsset":"USDT","underlyingType":"COIN"}`, s))
		}
		fmt.Fprintf(w, `{"symbols":[%s]}`, strings.Join(parts, ","))
	})
	mux.HandleFunc("/fapi/v1/ticker/24hr", func(w http.ResponseWriter, r *http.Request) {
		parts := make([]string, 0, len(symbols))
		for i, s := range symbols {
			parts = append(parts, fmt.Sprintf(`{"symbol":%q,"lastPrice":"100","priceChangePercent":"1","quoteVolume":"%d","volume":"1000","highPrice":"101","lowPrice":"99","count":10}`, s, 1_000_000*(len(symbols)-i)))
		}
		fmt.Fprintf(w, `[%s]`, strings.Join(parts, ","))
	})
	mux.HandleFunc("/fapi/v1/premiumIndex", func(w http.ResponseWriter, r *http.Request) {
		parts := make([]string, 0, len(symbols))
		for _, s := range symbols {
			parts = append(parts, fmt.Sprintf(`{"symbol":%q,"markPrice":"100","indexPrice":"100","lastFundingRate":"0.0001","nextFundingTime":0}`, s))
		}
		fmt.Fprintf(w, `[%s]`, strings.Join(parts, ","))
	})
	mux.HandleFunc("/fapi/v1/openInterest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"symbol":%q,"openInterest":"1000","time":1700000000000}`, r.URL.Query().Get("symbol"))
	})
	mux.HandleFunc("/futures/data/openInterestHist", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, oiHistJSON(13))
	})
	mux.HandleFunc("/futures/data/topLongShortPositionRatio", func(w http.ResponseWriter, r *http.Request) {
		if lsrFail[r.URL.Query().Get("symbol")] {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"code":-1121,"msg":"Invalid symbol."}`)
			return
		}
		fmt.Fprint(w, `[{"symbol":"X","longShortRatio":"1.10","timestamp":1700000000000}]`)
	})
	mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[[1700000000000,"100","101","99","100","10",1700000059999,"1000","5","5","500","0"]]`)
	})
	return mux
}

func statsFetcher(url string) *DataFetcher {
	return NewDataFetcher(FetcherConfig{
		BinanceURL:     url,
		TopNForDetail:  3,
		MaxWorkers:     2,
		KlineIntervals: []KlineInterval{{Interval: "1m", Limit: 1}},
	})
}

func TestFetchMetaCountsDetailAndFailedRequests(t *testing.T) {
	syms := []string{"AUSDT", "BUSDT", "CUSDT"}
	server := startT2TestServer(t, statsMux(syms, map[string]bool{"BUSDT": true}))
	snap, err := statsFetcher(server.URL).Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	m := snap.Meta
	if m.DetailRequested != 3 || m.DetailCompleted != 3 || m.DetailSkippedDeadline != 0 {
		t.Fatalf("detail requested/completed/skipped = %d/%d/%d, want 3/3/0", m.DetailRequested, m.DetailCompleted, m.DetailSkippedDeadline)
	}
	if m.FailedRequests != 1 || m.RestErrors != 1 {
		t.Fatalf("failed requests %d, rest errors %d, want 1/1 (one LSR 400 on BUSDT)", m.FailedRequests, m.RestErrors)
	}
	if m.FetchDuration <= 0 {
		t.Fatal("fetch duration not set")
	}
}

// A context that has already expired: the bulk phase still runs (it does
// not read ctx; behaviour unchanged), every detail symbol is skipped, and
// the skips are now counted instead of silent.
func TestFetchMetaCountsDeadlineSkips(t *testing.T) {
	syms := []string{"AUSDT", "BUSDT", "CUSDT"}
	server := startT2TestServer(t, statsMux(syms, nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snap, err := statsFetcher(server.URL).Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	m := snap.Meta
	if m.DetailRequested != 3 || m.DetailCompleted != 0 || m.DetailSkippedDeadline != 3 {
		t.Fatalf("detail requested/completed/skipped = %d/%d/%d, want 3/0/3", m.DetailRequested, m.DetailCompleted, m.DetailSkippedDeadline)
	}
	if m.RestErrors != 0 || m.FailedRequests != 0 {
		t.Fatalf("skips must not be counted as REST errors: rest %d failed %d", m.RestErrors, m.FailedRequests)
	}
	if ss := snap.Symbols["AUSDT"]; ss == nil || ss.OI != 0 || len(ss.Klines) != 0 {
		t.Fatalf("a skipped symbol keeps bulk-only data: %+v", ss)
	}
}

// Counters are per fetch: two fetches on the same fetcher do not mix.
func TestFetchMetaCountersArePerFetch(t *testing.T) {
	syms := []string{"AUSDT", "BUSDT"}
	server := startT2TestServer(t, statsMux(syms, map[string]bool{"AUSDT": true, "BUSDT": true}))
	f := statsFetcher(server.URL)
	for i := 0; i < 2; i++ {
		snap, err := f.Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if snap.Meta.FailedRequests != 2 {
			t.Fatalf("fetch %d: failed requests %d, want 2 (not cumulative)", i, snap.Meta.FailedRequests)
		}
	}
}

func TestFetchStatsNilSafe(t *testing.T) {
	var st *fetchStats
	st.recordFailure("/x", fmt.Errorf("boom"))
	if st.failed() != 0 || fetchStatsFrom(context.Background()) != nil || fetchStatsFrom(nil) != nil {
		t.Fatal("nil stats must be inert")
	}
}
