package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"sync"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

// TA-0: --binance-url reaches both fetchers the harness builds: the
// per-round snapshot fetcher and the outcome tracker's backfill fetcher.
func TestBinanceURLReachesBothFetchers(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Skipf("local TCP listener unavailable in this environment: %v", err)
	}
	listener.Close()

	var mu sync.Mutex
	hits := map[string]int{} // interval -> requests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fapi/v1/klines" {
			mu.Lock()
			hits[r.URL.Query().Get("interval")]++
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[[1700000000000,"1","2","0.5","1.5","10",1700000059999,"15","3","5","7.5","0"]]`))
	}))
	defer srv.Close()

	opts := validationOptions{binanceURL: srv.URL, topDetail: 10, maxWorkers: 2}
	round := datafetch.NewDataFetcher(roundFetcherConfig(opts, true, false))
	backfill := datafetch.NewDataFetcher(backfillFetcherConfig(opts))

	ctx := context.Background()
	if k, err := round.FetchKlines(ctx, "BTCUSDT", "5m", 1); err != nil || len(k) != 1 {
		t.Fatalf("round fetcher did not reach --binance-url: klines=%d err=%v", len(k), err)
	}
	if k, err := backfill.FetchKlines(ctx, "BTCUSDT", "1m", 1); err != nil || len(k) != 1 {
		t.Fatalf("backfill fetcher did not reach --binance-url: klines=%d err=%v", len(k), err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits["5m"] != 1 || hits["1m"] != 1 {
		t.Fatalf("mock server hits = %v, want one 5m (round) and one 1m (backfill)", hits)
	}
}

func TestFetcherConfigsCarryBinanceURLAndT2Inputs(t *testing.T) {
	opts := validationOptions{binanceURL: "http://relay.invalid:8899", topDetail: 220, maxWorkers: 4}
	rc := roundFetcherConfig(opts, true, false)
	if rc.BinanceURL != opts.binanceURL || !rc.IncludeT2Inputs || rc.TopNForDetail != 220 || rc.MaxWorkers != 4 {
		t.Fatalf("round fetcher config = %+v", rc)
	}
	if rc := roundFetcherConfig(opts, false, false); rc.IncludeT2Inputs {
		t.Fatal("T2 inputs must follow the strategy's v7_t2_enabled")
	}
	if bc := backfillFetcherConfig(opts); bc.BinanceURL != opts.binanceURL {
		t.Fatalf("backfill fetcher config BinanceURL = %q", bc.BinanceURL)
	}
	// Empty flag keeps the datafetch default (fapi.binance.com).
	if rc := roundFetcherConfig(validationOptions{}, false, false); rc.BinanceURL != "" {
		t.Fatalf("empty --binance-url must leave the default to datafetch, got %q", rc.BinanceURL)
	}
}

func TestBuildInfoVCS(t *testing.T) {
	sha, dirty := buildInfoVCS([]debug.BuildSetting{
		{Key: "vcs", Value: "git"},
		{Key: "vcs.revision", Value: "db1cc60abc"},
		{Key: "vcs.modified", Value: "true"},
	})
	if sha != "db1cc60abc" || !dirty {
		t.Fatalf("got sha=%q dirty=%v", sha, dirty)
	}
	sha, dirty = buildInfoVCS([]debug.BuildSetting{{Key: "vcs.modified", Value: "false"}})
	if sha != "unknown" || dirty {
		t.Fatalf("no revision: got sha=%q dirty=%v", sha, dirty)
	}
	// The live call never panics and always returns a non-empty sha.
	if sha, _ := harnessBuildInfo(); sha == "" {
		t.Fatal("harnessBuildInfo returned an empty sha")
	}
}
