package datafetch

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

// TA-6: the ErrorSink sees every request that failed after its retries,
// with its path and query, on full fetches and on single calls.
func TestErrorSinkReceivesFailedRequestPathAndQuery(t *testing.T) {
	syms := []string{"AUSDT", "BUSDT"}
	server := startT2TestServer(t, statsMux(syms, map[string]bool{"BUSDT": true}))
	var mu sync.Mutex
	var got []string
	cfg := FetcherConfig{BinanceURL: server.URL, TopNForDetail: 2, MaxWorkers: 2,
		KlineIntervals: []KlineInterval{{Interval: "1m", Limit: 1}},
		ErrorSink: func(path string, err error) {
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				t.Error("sink called without an error")
			}
			got = append(got, path)
		}}
	snap, err := NewDataFetcher(cfg).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := "/futures/data/topLongShortPositionRatio?symbol=BUSDT&period=1h&limit=12"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("sink paths %v, want [%s]", got, want)
	}
	if snap.Meta.FailedRequests != len(got) {
		t.Fatalf("FailedRequests %d != sink calls %d", snap.Meta.FailedRequests, len(got))
	}
	// Single call outside FetchWithOptions (tracker backfill path).
	got = nil
	cfg.BinanceURL = server.URL + "/missing-prefix"
	if _, err := NewDataFetcher(cfg).FetchKlines(context.Background(), "AUSDT", "1m", 5); err == nil {
		t.Fatal("expected a 404")
	}
	if len(got) != 1 || got[0] != "/fapi/v1/klines?symbol=AUSDT&interval=1m&limit=5" {
		t.Fatalf("backfill sink paths %v", got)
	}
}

// A dropped connection (the relay-side failure mode) is retried, then
// reported once with its path.
func TestErrorSinkAfterRetriesOnDroppedConnection(t *testing.T) {
	mux := http.NewServeMux()
	var hits int64
	mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("no hijacker")
		}
		c, _, _ := hj.Hijack()
		c.(*net.TCPConn).SetLinger(0)
		c.Close() // reset: client sees EOF / connection reset
	})
	server := startT2TestServer(t, mux)
	var got []string
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, ErrorSink: func(p string, err error) { got = append(got, p) }})
	if _, err := f.FetchKlines(context.Background(), "XUSDT", "5m", 3); err == nil {
		t.Fatal("expected failure")
	}
	if n := atomic.LoadInt64(&hits); n != 4 {
		t.Fatalf("server hits %d, want 4 (1 + 3 retries)", n)
	}
	if len(got) != 1 || got[0] != "/fapi/v1/klines?symbol=XUSDT&interval=5m&limit=3" {
		t.Fatalf("sink paths %v, want one record after retries", got)
	}
}
