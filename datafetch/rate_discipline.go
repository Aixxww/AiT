package datafetch

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Rate discipline (#87, docs/audit/optimization-design-20261124.md cluster 3).
//
// Binance USDⓈ-M futures allow 2400 request weight per minute per IP. The
// collector operates at 2000 (DefaultOperatingWeightPerMin) so a burst, a
// second process on the same IP or an unmeasured weight change still leaves
// headroom. On top of the global weight bucket:
//   - per-endpoint sub-budgets, so one endpoint family (klines) cannot starve
//     the bulk calls or the finalist refresh;
//   - Retry-After on 429 / 418 pauses every request of the client, not only
//     the one that was rejected;
//   - the server-reported X-MBX-USED-WEIGHT-1M pauses the client until the
//     next minute when it reaches the guard (DefaultServerWeightGuard);
//   - retries back off exponentially with full jitter;
//   - the circuit breaker is unchanged.

const (
	// BinanceWeightCapPerMin is the exchange's per-IP weight limit.
	BinanceWeightCapPerMin = 2400
	// DefaultOperatingWeightPerMin is the weight the collector plans for.
	DefaultOperatingWeightPerMin = 2000
	// DefaultFuturesDataLimitPer5Min is the /futures/data per-IP limit.
	DefaultFuturesDataLimitPer5Min = 1000
	// DefaultServerWeightGuard: when the exchange reports this much weight
	// used in the current minute, the client waits for the next minute.
	DefaultServerWeightGuard = 2200

	retryBaseDelay = 300 * time.Millisecond
	retryMaxDelay  = 10 * time.Second
	// maxRetryAfter bounds a server-requested pause, so a bogus header cannot
	// stall the collector for hours (418 bans are reported, not slept out).
	maxRetryAfter = 2 * time.Minute
)

// endpointSubBudget is the share of the operating weight one endpoint
// family may use per minute. Shares add up to more than 1 on purpose: they
// cap a family, the global bucket caps the sum.
type endpointSubBudget struct {
	prefix string
	share  float64
}

// defaultSubBudgets: klines dominate the detail fetch (about 5 weight per
// symbol per round), depth is the heaviest single request, and the bulk
// ticker / premiumIndex calls must always find room.
var defaultSubBudgets = []endpointSubBudget{
	{prefix: "/fapi/v1/klines", share: 0.70},
	{prefix: "/fapi/v1/depth", share: 0.25},
	{prefix: "/fapi/v1/openInterest", share: 0.15},
	{prefix: "/fapi/v1/ticker/24hr", share: 0.10},
	{prefix: "/fapi/v1/premiumIndex", share: 0.10},
	{prefix: "/fapi/v1/exchangeInfo", share: 0.05},
}

type subBudgetLimiter struct {
	prefix string
	l      *RateLimiter
}

func newSubBudgetLimiters(operatingPerMin float64) []subBudgetLimiter {
	out := make([]subBudgetLimiter, 0, len(defaultSubBudgets))
	for _, b := range defaultSubBudgets {
		per := operatingPerMin * b.share
		out = append(out, subBudgetLimiter{prefix: b.prefix, l: NewRateLimiterBurst(per, time.Minute, per*0.1)})
	}
	return out
}

// AcquireNCtx is AcquireN that gives up when ctx ends (deadline-aware
// fetches must not sit in the limiter past their budget).
func (r *RateLimiter) AcquireNCtx(ctx context.Context, n float64) error {
	if r == nil || n <= 0 {
		return nil
	}
	for {
		r.mu.Lock()
		need := math.Min(n, r.maxTokens)
		now := time.Now()
		elapsed := now.Sub(r.lastRefill).Seconds()
		r.lastRefill = now
		r.tokens = math.Min(r.maxTokens, r.tokens+elapsed*r.refillRate)
		if r.tokens >= need {
			r.tokens -= need
			r.mu.Unlock()
			return nil
		}
		wait := time.Duration((need - r.tokens) / r.refillRate * float64(time.Second))
		r.mu.Unlock()
		if err := sleepCtx(ctx, wait); err != nil {
			return err
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if ctx == nil {
		time.Sleep(d)
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// pauseGate holds the client-wide pause set by Retry-After or the
// server-reported weight guard.
type pauseGate struct {
	mu    sync.Mutex
	until time.Time
}

func (g *pauseGate) pauseFor(d time.Duration) {
	if d <= 0 {
		return
	}
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	g.mu.Lock()
	if u := time.Now().Add(d); u.After(g.until) {
		g.until = u
	}
	g.mu.Unlock()
}

func (g *pauseGate) remaining() time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Until(g.until)
}

func (g *pauseGate) wait(ctx context.Context) error {
	return sleepCtx(ctx, g.remaining())
}

// retryAfter parses a Retry-After header (seconds or HTTP date). 0 = absent.
func retryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

// usedWeight1m reads X-MBX-USED-WEIGHT-1M (Binance's own per-minute count).
func usedWeight1m(h http.Header) int64 {
	v := h.Get("X-Mbx-Used-Weight-1m")
	if v == "" {
		v = h.Get("X-Mbx-Used-Weight")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// untilNextMinute is the wait until Binance's weight window rolls over.
func untilNextMinute(now time.Time) time.Duration {
	return now.Truncate(time.Minute).Add(time.Minute).Sub(now)
}

var jitterSrc = struct {
	mu sync.Mutex
	r  *rand.Rand
}{r: rand.New(rand.NewSource(time.Now().UnixNano()))}

// backoffDelay is exponential backoff with full jitter: uniform in
// [0, min(max, base*2^attempt)].
func backoffDelay(attempt int) time.Duration {
	ceiling := retryBaseDelay << uint(attempt)
	if ceiling > retryMaxDelay || ceiling <= 0 {
		ceiling = retryMaxDelay
	}
	jitterSrc.mu.Lock()
	f := jitterSrc.r.Float64()
	jitterSrc.mu.Unlock()
	return time.Duration(f * float64(ceiling))
}

// rateCounters are client-lifetime counters (the per-fetch ones live in
// fetchStats).
type rateCounters struct {
	rateLimited    int64 // 429 responses
	banned         int64 // 418 responses
	lastUsedWeight int64 // last X-MBX-USED-WEIGHT-1M seen
}

func (c *rateCounters) observeUsedWeight(n int64) {
	if n > 0 {
		atomic.StoreInt64(&c.lastUsedWeight, n)
	}
}
