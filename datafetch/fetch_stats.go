package datafetch

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TA-5 (docs/tier-a-improvement-plan-20261120.md): per-fetch observability.
//
// FetchWithOptions attaches a fetchStats to its context; doPublic records
// every request that failed after its retries. The counters are per fetch
// call, so concurrent fetches on one DataFetcher never mix. Request
// behaviour (retries, rate limiting, timeouts) is unchanged.
//
// Fix 02 adds per-endpoint latency (the TA-5 p50/p95 item). For every
// request doPublic records, per endpoint path (query stripped):
//   - net:     each HTTP attempt, from GET to body read (network + upstream);
//   - limiter: the client token-bucket wait before each attempt;
//   - call:    the whole call, including retries and their backoff.
// Splitting the three separates relay/upstream latency from our own rate
// limiter, which is what the two-tier fetch design needs to size N and the
// limit.

type fetchStatsKey struct{}

type fetchStats struct {
	failedRequests int64

	// #87: per-fetch request budget accounting. requests counts HTTP
	// attempts (retries included), weight their Binance request weight
	// (request_weight.go), rateLimited the 429/418 responses, and
	// maxServerWeight the highest X-MBX-USED-WEIGHT-1M seen.
	requests        int64
	weight          int64
	rateLimited     int64
	maxServerWeight int64

	mu        sync.Mutex
	endpoints map[string]*endpointSamples
}

type endpointSamples struct {
	calls, attempts, failed int
	net, limiter, call      []time.Duration
}

func withFetchStats(ctx context.Context, st *fetchStats) context.Context {
	return context.WithValue(ctx, fetchStatsKey{}, st)
}

func fetchStatsFrom(ctx context.Context) *fetchStats {
	if ctx == nil {
		return nil
	}
	st, _ := ctx.Value(fetchStatsKey{}).(*fetchStats)
	return st
}

// recordFailure counts one request that failed after its retries. Nil-safe.
func (st *fetchStats) recordFailure(path string, err error) {
	if st == nil {
		return
	}
	atomic.AddInt64(&st.failedRequests, 1)
}

// recordRequest counts one HTTP attempt and its weight. Nil-safe.
func (st *fetchStats) recordRequest(weight float64) {
	if st == nil {
		return
	}
	atomic.AddInt64(&st.requests, 1)
	atomic.AddInt64(&st.weight, int64(math.Round(weight)))
}

// recordRateLimited counts one 429 / 418 response. Nil-safe.
func (st *fetchStats) recordRateLimited() {
	if st == nil {
		return
	}
	atomic.AddInt64(&st.rateLimited, 1)
}

// recordServerWeight keeps the highest server-reported minute weight.
func (st *fetchStats) recordServerWeight(n int64) {
	if st == nil {
		return
	}
	for {
		cur := atomic.LoadInt64(&st.maxServerWeight)
		if n <= cur || atomic.CompareAndSwapInt64(&st.maxServerWeight, cur, n) {
			return
		}
	}
}

// budget returns the per-fetch request accounting.
func (st *fetchStats) budget() (requests, weight, rateLimited, maxServerWeight int) {
	if st == nil {
		return 0, 0, 0, 0
	}
	return int(atomic.LoadInt64(&st.requests)), int(atomic.LoadInt64(&st.weight)),
		int(atomic.LoadInt64(&st.rateLimited)), int(atomic.LoadInt64(&st.maxServerWeight))
}

func (st *fetchStats) failed() int {
	if st == nil {
		return 0
	}
	return int(atomic.LoadInt64(&st.failedRequests))
}

// endpointKey is the request path without its query string.
func endpointKey(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

func (st *fetchStats) samples(path string) *endpointSamples {
	if st.endpoints == nil {
		st.endpoints = map[string]*endpointSamples{}
	}
	k := endpointKey(path)
	s := st.endpoints[k]
	if s == nil {
		s = &endpointSamples{}
		st.endpoints[k] = s
	}
	return s
}

// recordAttempt records one HTTP attempt's limiter wait and network time.
// Nil-safe.
func (st *fetchStats) recordAttempt(path string, limiterWait, net time.Duration) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.samples(path)
	s.attempts++
	s.limiter = append(s.limiter, limiterWait)
	s.net = append(s.net, net)
}

// recordCall records one whole call (retries included). Nil-safe.
func (st *fetchStats) recordCall(path string, total time.Duration, failed bool) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.samples(path)
	s.calls++
	if failed {
		s.failed++
	}
	s.call = append(s.call, total)
}

// EndpointLatency summarises one endpoint over one fetch (fix 02).
// Milliseconds, nearest-rank percentiles.
type EndpointLatency struct {
	Calls    int `json:"calls"`
	Attempts int `json:"attempts"` // > calls when retries happened
	Failed   int `json:"failed"`   // calls that failed after retries

	NetP50Ms int64 `json:"net_p50_ms"` // per attempt: GET until body read
	NetP95Ms int64 `json:"net_p95_ms"`
	NetMaxMs int64 `json:"net_max_ms"`

	LimiterWaitP50Ms int64 `json:"limiter_wait_p50_ms"` // per attempt: token-bucket wait
	LimiterWaitP95Ms int64 `json:"limiter_wait_p95_ms"`

	CallP50Ms int64 `json:"call_p50_ms"` // per call: limiter + network + retry backoff
	CallP95Ms int64 `json:"call_p95_ms"`
}

func (st *fetchStats) endpointLatency() map[string]EndpointLatency {
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.endpoints) == 0 {
		return nil
	}
	out := make(map[string]EndpointLatency, len(st.endpoints))
	for k, s := range st.endpoints {
		out[k] = EndpointLatency{
			Calls: s.calls, Attempts: s.attempts, Failed: s.failed,
			NetP50Ms: percentileMs(s.net, 50), NetP95Ms: percentileMs(s.net, 95), NetMaxMs: percentileMs(s.net, 100),
			LimiterWaitP50Ms: percentileMs(s.limiter, 50), LimiterWaitP95Ms: percentileMs(s.limiter, 95),
			CallP50Ms: percentileMs(s.call, 50), CallP95Ms: percentileMs(s.call, 95),
		}
	}
	return out
}

// percentileMs is the nearest-rank percentile (p in 1..100) in milliseconds;
// 0 for no samples.
func percentileMs(d []time.Duration, p float64) int64 {
	if len(d) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	rank := int(math.Ceil(p / 100 * float64(len(s))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(s) {
		rank = len(s)
	}
	return s[rank-1].Milliseconds()
}
