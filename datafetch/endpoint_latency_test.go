package datafetch

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// Fix 02: per-endpoint latency p50/p95 (net, limiter wait, call).

func TestPercentileMsNearestRank(t *testing.T) {
	ms := func(v ...int) []time.Duration {
		out := make([]time.Duration, len(v))
		for i, x := range v {
			out[i] = time.Duration(x) * time.Millisecond
		}
		return out
	}
	d := ms(50, 10, 40, 20, 30, 60, 70, 80, 90, 100) // unsorted on purpose
	if percentileMs(d, 50) != 50 || percentileMs(d, 95) != 100 || percentileMs(d, 100) != 100 {
		t.Fatalf("p50 %d p95 %d max %d", percentileMs(d, 50), percentileMs(d, 95), percentileMs(d, 100))
	}
	if percentileMs(nil, 50) != 0 || percentileMs(ms(7), 95) != 7 {
		t.Fatal("edge cases")
	}
	if d[0] != 50*time.Millisecond {
		t.Fatal("input must not be reordered")
	}
	if endpointKey("/fapi/v1/klines?symbol=X&interval=1m") != "/fapi/v1/klines" || endpointKey("/fapi/v1/time") != "/fapi/v1/time" {
		t.Fatal("endpointKey")
	}
}

// A full Fetch reports one entry per endpoint with call/attempt counts that
// match the requests made, and network latency that reflects the server.
func TestFetchMetaEndpointLatency(t *testing.T) {
	syms := []string{"AUSDT", "BUSDT", "CUSDT"}
	inner := statsMux(syms, map[string]bool{"BUSDT": true})
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fapi/v1/klines" {
			time.Sleep(40 * time.Millisecond) // slow endpoint
		}
		inner.ServeHTTP(w, r)
	})
	server := startT2TestServer(t, mux)
	snap, err := statsFetcher(server.URL).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	el := snap.Meta.EndpointLatency
	for _, k := range []string{"/fapi/v1/exchangeInfo", "/fapi/v1/ticker/24hr", "/fapi/v1/premiumIndex", "/fapi/v1/openInterest",
		"/futures/data/openInterestHist", "/futures/data/topLongShortPositionRatio", "/fapi/v1/klines"} {
		if _, ok := el[k]; !ok {
			t.Fatalf("no latency entry for %s: %v", k, el)
		}
	}
	k := el["/fapi/v1/klines"]
	if k.Calls != 3 || k.Attempts != 3 || k.Failed != 0 {
		t.Fatalf("klines counts %+v, want 3/3/0 (one 1m kline per detail symbol)", k)
	}
	if k.NetP50Ms < 40 || k.CallP50Ms < k.NetP50Ms {
		t.Fatalf("klines latency %+v: net p50 must reflect the 40 ms server delay and call >= net", k)
	}
	if fast := el["/fapi/v1/openInterest"]; fast.NetP95Ms >= 40 {
		t.Fatalf("openInterest net p95 %d ms should be below the klines delay", fast.NetP95Ms)
	}
	lsr := el["/futures/data/topLongShortPositionRatio"]
	if lsr.Calls != 3 || lsr.Failed != 1 || lsr.Attempts != 3 { // the HTTP 400 is not retried
		t.Fatalf("lsr counts %+v, want calls 3 failed 1 attempts 3", lsr)
	}
	total := 0
	for _, e := range el {
		total += e.Failed
	}
	if total != snap.Meta.FailedRequests {
		t.Fatalf("per-endpoint failed %d != FailedRequests %d", total, snap.Meta.FailedRequests)
	}
}

// Retries show up as attempts > calls; the call time includes the backoff.
func TestEndpointLatencyCountsRetries(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) {
		c, _, _ := w.(http.Hijacker).Hijack()
		c.(*net.TCPConn).SetLinger(0)
		c.Close()
	})
	server := startT2TestServer(t, mux)
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL})
	st := &fetchStats{}
	if _, err := f.FetchKlines(withFetchStats(context.Background(), st), "XUSDT", "1m", 1); err == nil {
		t.Fatal("expected failure")
	}
	e := st.endpointLatency()["/fapi/v1/klines"]
	if e.Calls != 1 || e.Attempts != 4 || e.Failed != 1 {
		t.Fatalf("counts %+v, want calls 1 attempts 4 failed 1", e)
	}
	if e.CallP50Ms < 4000 { // 0.6 + 1.2 + 2.4 s backoff
		t.Fatalf("call time %d ms must include the retry backoff", e.CallP50Ms)
	}
}

// Limiter wait is measured separately from the network.
func TestEndpointLatencySeparatesLimiterWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[]`)) })
	server := startT2TestServer(t, mux)
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL})
	f.client.limiter = &RateLimiter{tokens: 0, maxTokens: 1, refillRate: 20, lastRefill: time.Now()} // ~50 ms per token
	st := &fetchStats{}
	ctx := withFetchStats(context.Background(), st)
	for i := 0; i < 4; i++ {
		if _, err := f.FetchKlines(ctx, "XUSDT", "1m", 1); err != nil {
			t.Fatal(err)
		}
	}
	e := st.endpointLatency()["/fapi/v1/klines"]
	if e.LimiterWaitP50Ms < 30 || e.NetP95Ms >= e.LimiterWaitP50Ms {
		t.Fatalf("limiter wait p50 %d ms should dominate net p95 %d ms", e.LimiterWaitP50Ms, e.NetP95Ms)
	}
}

func TestEndpointLatencyNilSafe(t *testing.T) {
	var st *fetchStats
	st.recordAttempt("/x", time.Millisecond, time.Millisecond)
	st.recordCall("/x", time.Millisecond, false)
	if st.endpointLatency() != nil || (&fetchStats{}).endpointLatency() != nil {
		t.Fatal("nil / empty stats must return nil")
	}
}
