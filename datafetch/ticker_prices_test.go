package datafetch

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// Fix 04: FetchTickerPrices is one bulk ticker call returning last prices.
func TestFetchTickerPricesOneBulkCall(t *testing.T) {
	var hits int64
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/ticker/24hr", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		if r.URL.RawQuery != "" {
			t.Errorf("must be the bulk call, got query %q", r.URL.RawQuery)
		}
		fmt.Fprint(w, `[{"symbol":"AUSDT","lastPrice":"1.2345"},{"symbol":"牛来USDT","lastPrice":"0.0353"},{"symbol":"BADUSDT","lastPrice":"x"},{"symbol":"ZEROUSDT","lastPrice":"0"}]`)
	})
	server := startT2TestServer(t, mux)
	before := time.Now()
	prices, at, err := NewDataFetcher(FetcherConfig{BinanceURL: server.URL}).FetchTickerPrices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&hits) != 1 {
		t.Fatalf("calls %d, want 1", hits)
	}
	if len(prices) != 2 || prices["AUSDT"] != 1.2345 || prices["牛来USDT"] != 0.0353 {
		t.Fatalf("prices %v", prices)
	}
	if at.Before(before) {
		t.Fatal("received time")
	}
	var failed []string
	bad := NewDataFetcher(FetcherConfig{BinanceURL: server.URL + "/nope", ErrorSink: func(p string, err error) { failed = append(failed, p) }})
	if _, _, err := bad.FetchTickerPrices(context.Background()); err == nil || len(failed) != 1 {
		t.Fatalf("error path: err=%v sink=%v", err, failed)
	}
}
