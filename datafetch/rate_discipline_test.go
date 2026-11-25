package datafetch

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// #87 rate discipline.

func TestRetryAfterParsing(t *testing.T) {
	h := http.Header{}
	if retryAfter(h) != 0 {
		t.Fatal("absent header must be 0")
	}
	h.Set("Retry-After", "3")
	if retryAfter(h) != 3*time.Second {
		t.Fatalf("seconds: %v", retryAfter(h))
	}
	h.Set("Retry-After", time.Now().Add(5*time.Second).UTC().Format(http.TimeFormat))
	if d := retryAfter(h); d < 3*time.Second || d > 6*time.Second {
		t.Fatalf("http date: %v", d)
	}
	h.Set("X-Mbx-Used-Weight-1m", "1234")
	if usedWeight1m(h) != 1234 {
		t.Fatal("used weight header")
	}
}

// A 429 with Retry-After pauses the whole client: the retry and any other
// request wait for it, and the call succeeds once the server recovers.
func TestHTTPClient429HonorsRetryAfter(t *testing.T) {
	var calls int64
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/time", func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{}`))
	})
	server := startT2TestServer(t, mux)
	c := NewHTTPClient(server.URL)
	st := &fetchStats{}
	start := time.Now()
	if _, err := c.doPublic(withFetchStats(context.Background(), st), "/fapi/v1/time"); err != nil {
		t.Fatalf("doPublic: %v", err)
	}
	if el := time.Since(start); el < 900*time.Millisecond {
		t.Fatalf("retry did not wait for Retry-After: %v", el)
	}
	if req, _, limited, _ := st.budget(); req != 2 || limited != 1 {
		t.Fatalf("requests %d rate-limited %d, want 2/1", req, limited)
	}
}

// 418 (IP ban) is never retried and pauses the client.
func TestHTTPClient418NoRetry(t *testing.T) {
	var calls int64
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/time", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(418)
	})
	server := startT2TestServer(t, mux)
	c := NewHTTPClient(server.URL)
	_, err := c.doPublic(context.Background(), "/fapi/v1/time")
	if err == nil || !strings.Contains(err.Error(), "418") {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("418 retried: %d calls", calls)
	}
	if c.pause.remaining() < time.Second {
		t.Fatal("418 did not pause the client")
	}
}

// The server-reported minute weight at the guard pauses until next minute.
func TestHTTPClientServerWeightGuard(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/time", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-MBX-USED-WEIGHT-1M", "2300")
		w.Write([]byte(`{}`))
	})
	server := startT2TestServer(t, mux)
	c := NewHTTPClient(server.URL)
	st := &fetchStats{}
	if _, err := c.doPublic(withFetchStats(context.Background(), st), "/fapi/v1/time"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, srv := st.budget(); srv != 2300 {
		t.Fatalf("server weight %d", srv)
	}
	if c.pause.remaining() <= 0 {
		t.Fatal("weight guard did not pause the client")
	}
	// A request issued while paused gives up when its deadline ends.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if c.pause.remaining() > 100*time.Millisecond {
		if _, err := c.doPublic(ctx, "/fapi/v1/time"); err == nil {
			t.Fatal("paused request ignored its deadline")
		}
	}
}

// The request carries ctx: a deadline cancels an in-flight call promptly.
func TestHTTPClientCancelsInFlight(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/time", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	})
	server := startT2TestServer(t, mux)
	c := NewHTTPClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.doPublic(ctx, "/fapi/v1/time"); err == nil {
		t.Fatal("expected deadline error")
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("in-flight call not cancelled: %v", el)
	}
	if !c.breaker.Allow() {
		t.Fatal("a caller deadline must not trip the breaker")
	}
}

// Sub-budgets cap one endpoint family below the global weight.
func TestEndpointSubBudgetsCapKlines(t *testing.T) {
	c := newFetcherHTTPClient(FetcherConfig{BinanceURL: "http://x", WeightLimitPerMin: 2000, EndpointSubBudgets: true})
	if len(c.subBudgets) != len(defaultSubBudgets) {
		t.Fatalf("sub-budgets %d", len(c.subBudgets))
	}
	var kl *RateLimiter
	for _, sb := range c.subBudgets {
		if sb.prefix == "/fapi/v1/klines" {
			kl = sb.l
		}
	}
	if kl == nil || kl.refillRate*60 > 2000*0.7+1 || kl.refillRate*60 < 2000*0.7-1 {
		t.Fatalf("klines sub-budget rate %.1f/min, want 1400", kl.refillRate*60)
	}
	// Burst + one minute of refill never exceeds the exchange cap.
	if c.weightLimiter.maxTokens+c.weightLimiter.refillRate*60 > BinanceWeightCapPerMin {
		t.Fatalf("global limiter can exceed %d/min", BinanceWeightCapPerMin)
	}
	// The burst is spent, so a klines request beyond it waits on the
	// sub-budget even though the global bucket still has room.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var err error
	for i := 0; i < 1000 && err == nil; i++ {
		err = c.acquire(ctx, "/fapi/v1/klines?symbol=A&interval=1m&limit=60")
	}
	if err == nil {
		t.Fatal("klines sub-budget never throttled")
	}
	if c.weightLimiter.tokens < 50 {
		t.Fatalf("global bucket drained by one family: %.0f tokens left", c.weightLimiter.tokens)
	}
}
