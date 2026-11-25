package datafetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Rate Limiter (token bucket)
// ---------------------------------------------------------------------------

// RateLimiter implements a simple token bucket rate limiter.
type RateLimiter struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	mu         sync.Mutex
	lastRefill time.Time
}

// NewRateLimiter creates a rate limiter that allows maxTokens per minute.
func NewRateLimiter(maxPerMin float64) *RateLimiter {
	return &RateLimiter{
		tokens:     maxPerMin,
		maxTokens:  maxPerMin,
		refillRate: maxPerMin / 60.0,
		lastRefill: time.Now(),
	}
}

// NewRateLimiterBurst creates a limiter refilling perWindow tokens per
// window with at most burst tokens banked (fix 05). The most it can admit in
// any window is burst + perWindow, so a hard cap L is respected with
// burst + perWindow <= L.
func NewRateLimiterBurst(perWindow float64, window time.Duration, burst float64) *RateLimiter {
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{
		tokens:     burst,
		maxTokens:  burst,
		refillRate: perWindow / window.Seconds(),
		lastRefill: time.Now(),
	}
}

// Acquire blocks until one token is available.
func (r *RateLimiter) Acquire() { r.AcquireN(1) }

// AcquireN blocks until n tokens are available (n is capped at the bucket
// size so a request heavier than the burst can still pass). n <= 0 returns
// at once.
func (r *RateLimiter) AcquireN(n float64) {
	if r == nil || n <= 0 {
		return
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
			return
		}
		wait := time.Duration((need - r.tokens) / r.refillRate * float64(time.Second))
		r.mu.Unlock()
		time.Sleep(wait)
	}
}

// ---------------------------------------------------------------------------
// Circuit Breaker
// ---------------------------------------------------------------------------

const (
	cbClosed   int32 = 0
	cbOpen     int32 = 1
	cbHalfOpen int32 = 2
)

// CircuitBreaker prevents cascading failures by tripping after N consecutive errors.
type CircuitBreaker struct {
	state       int32 // atomic
	failures    int32 // atomic
	threshold   int32
	openTimeout time.Duration
	lastTrip    time.Time
	mu          sync.Mutex
}

// NewCircuitBreaker creates a breaker that opens after threshold failures
// and stays open for openTimeout.
func NewCircuitBreaker(threshold int32, openTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:   threshold,
		openTimeout: openTimeout,
	}
}

// Allow returns true if requests are allowed through the breaker.
func (cb *CircuitBreaker) Allow() bool {
	state := atomic.LoadInt32(&cb.state)
	switch state {
	case cbClosed:
		return true
	case cbOpen:
		cb.mu.Lock()
		if time.Since(cb.lastTrip) > cb.openTimeout {
			atomic.StoreInt32(&cb.state, cbHalfOpen)
			cb.mu.Unlock()
			return true
		}
		cb.mu.Unlock()
		return false
	case cbHalfOpen:
		return true
	}
	return true
}

// Success records a successful call.
func (cb *CircuitBreaker) Success() {
	atomic.StoreInt32(&cb.failures, 0)
	atomic.StoreInt32(&cb.state, cbClosed)
}

// Failure records a failed call. Trips the breaker if threshold is reached.
func (cb *CircuitBreaker) Failure() {
	n := atomic.AddInt32(&cb.failures, 1)
	if n >= cb.threshold {
		cb.mu.Lock()
		cb.lastTrip = time.Now()
		atomic.StoreInt32(&cb.state, cbOpen)
		cb.mu.Unlock()
	}
}

// ---------------------------------------------------------------------------
// HTTP Client
// ---------------------------------------------------------------------------

// HTTPClient wraps http.Client with rate limiting and circuit breaking.
type HTTPClient struct {
	inner   *http.Client
	limiter *RateLimiter
	breaker *CircuitBreaker
	baseURL string
	// errorSink (TA-6): see FetcherConfig.ErrorSink.
	errorSink func(path string, err error)

	// Fix 05 (optional): weight-based limiting instead of one token per
	// request. weightLimiter is charged binanceRequestWeight(path);
	// futuresDataLimiter is charged 1 per /futures/data request (those
	// endpoints carry no IP weight and have their own per-IP limit).
	weightLimiter      *RateLimiter
	futuresDataLimiter *RateLimiter

	// #87 rate discipline (rate_discipline.go). subBudgets are only set
	// with the weight limiter; pause and counters always exist.
	subBudgets  []subBudgetLimiter
	pause       pauseGate
	counters    rateCounters
	weightGuard int64 // server-reported weight that pauses the client; 0 = DefaultServerWeightGuard
}

func newFetcherHTTPClient(cfg FetcherConfig) *HTTPClient {
	c := NewHTTPClientWithTimeout(cfg.BinanceURL, cfg.Timeout)
	c.errorSink = cfg.ErrorSink
	if cfg.WeightLimitPerMin > 0 {
		// burst 10% of the rate: at most 1.1 x the rate in any minute.
		c.weightLimiter = NewRateLimiterBurst(cfg.WeightLimitPerMin, time.Minute, cfg.WeightLimitPerMin*0.1)
		if cfg.EndpointSubBudgets {
			c.subBudgets = newSubBudgetLimiters(cfg.WeightLimitPerMin)
		}
	}
	if cfg.FuturesDataLimitPer5Min > 0 {
		// burst 40% + refill 60% per 5 min: never more than the limit in
		// any 5-minute window.
		l := float64(cfg.FuturesDataLimitPer5Min)
		c.futuresDataLimiter = NewRateLimiterBurst(l*0.6, 5*time.Minute, l*0.4)
	}
	return c
}

// acquire charges the limiters for one attempt of path and waits out any
// client-wide pause. It returns early (with ctx's error) when ctx ends.
func (c *HTTPClient) acquire(ctx context.Context, path string) error {
	if err := c.pause.wait(ctx); err != nil {
		return err
	}
	if strings.HasPrefix(path, "/futures/data/") && c.futuresDataLimiter != nil {
		if err := c.futuresDataLimiter.AcquireNCtx(ctx, 1); err != nil {
			return err
		}
	}
	if c.weightLimiter != nil {
		w := binanceRequestWeight(path)
		key := endpointKey(path)
		for _, sb := range c.subBudgets {
			if key == sb.prefix {
				if err := sb.l.AcquireNCtx(ctx, w); err != nil {
					return err
				}
				break
			}
		}
		return c.weightLimiter.AcquireNCtx(ctx, w)
	}
	return c.limiter.AcquireNCtx(ctx, 1)
}

// NewHTTPClient creates an HTTP client with connection pooling, rate limiting,
// and circuit breaking for Binance API calls.
func NewHTTPClient(baseURL string) *HTTPClient {
	return NewHTTPClientWithTimeout(baseURL, 10*time.Second)
}

// NewHTTPClientWithTimeout creates an HTTP client with an explicit request
// timeout for Binance API calls.
func NewHTTPClientWithTimeout(baseURL string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	// A/B test (2026-09-14): retain idle connections through the stable local
	// Trojan proxy. Concurrency, circuit breaking, and connection limits remain
	// unchanged; this only avoids needless TCP/TLS setup on repeated requests.
	transport := &http.Transport{
		Proxy: binanceProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        400,
		MaxIdleConnsPerHost: 200,
		IdleConnTimeout:     90 * time.Second,
		MaxConnsPerHost:     200,
		TLSHandshakeTimeout: timeout,
		ForceAttemptHTTP2:   true,
		DisableKeepAlives:   false,
	}
	inner := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
	return &HTTPClient{
		inner:   inner,
		limiter: NewRateLimiter(1200), // 1200 tokens/min for Binance
		breaker: NewCircuitBreaker(5, 30*time.Second),
		baseURL: baseURL,
	}
}

func binanceProxyFromEnvironment(req *http.Request) (*url.URL, error) {
	if proxyURL := strings.TrimSpace(os.Getenv("AIT_BINANCE_PROXY_URL")); proxyURL != "" {
		return url.Parse(proxyURL)
	}
	return http.ProxyFromEnvironment(req)
}

// doPublic performs a rate-limited, circuit-breaker-protected GET request
// to a Binance public endpoint. Returns the response body.
//
// Retries transient network errors and 429 with exponential backoff and
// full jitter (#87). A 429 or 418 with Retry-After pauses every request of
// this client for that long; 418 (IP ban) is never retried. The
// server-reported X-MBX-USED-WEIGHT-1M pauses the client until the next
// minute once it reaches the guard. The request carries ctx, so a fetch
// deadline (FetchBudget, the finalist refresh deadline) cancels in-flight
// calls. The circuit breaker records one failure per call, after retries.
func (c *HTTPClient) doPublic(ctx context.Context, path string) (body []byte, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	stats := fetchStatsFrom(ctx)
	callStart := time.Now()
	defer func() {
		stats.recordCall(path, time.Since(callStart), err != nil) // fix 02 (measurement only)
		if err != nil {
			fetchStatsFrom(ctx).recordFailure(path, err)
			if c.errorSink != nil {
				c.errorSink(path, err)
			}
		}
	}()
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("circuit breaker open for %s", path)
	}

	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoffDelay(attempt)); err != nil {
				return nil, fmt.Errorf("GET %s: %w", path, err)
			}
		}
		waitStart := time.Now()
		if err := c.acquire(ctx, path); err != nil {
			return nil, fmt.Errorf("GET %s: %w", path, err)
		}
		limiterWait := time.Since(waitStart)
		stats.recordRequest(binanceRequestWeight(path))

		url := c.baseURL + path
		netStart := time.Now()
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if reqErr != nil {
			return nil, fmt.Errorf("GET %s: %w", url, reqErr)
		}
		resp, err := c.inner.Do(req)
		if err != nil {
			stats.recordAttempt(path, limiterWait, time.Since(netStart))
			lastErr = fmt.Errorf("GET %s: %w", url, err)
			if ctx.Err() != nil {
				// The caller's deadline, not the network: do not retry and
				// do not count it against the breaker.
				return nil, lastErr
			}
			if attempt < maxRetries && isRetryable(err) {
				continue
			}
			break
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		stats.recordAttempt(path, limiterWait, time.Since(netStart))
		c.observeServerWeight(resp.Header, stats)
		if err != nil {
			lastErr = fmt.Errorf("read body %s: %w", url, err)
			if ctx.Err() != nil {
				return nil, lastErr
			}
			if attempt < maxRetries && isRetryable(err) {
				continue
			}
			break
		}

		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			atomic.AddInt64(&c.counters.rateLimited, 1)
			stats.recordRateLimited()
			wait := retryAfter(resp.Header)
			if wait <= 0 {
				wait = backoffDelay(attempt + 1)
			}
			c.pause.pauseFor(wait)
			lastErr = fmt.Errorf("GET %s: rate limited (429, retry after %s)", url, wait.Round(time.Millisecond))
			if attempt < maxRetries {
				continue
			}
		case 418:
			// IP ban: stop this client for the advertised time; no retry.
			atomic.AddInt64(&c.counters.banned, 1)
			stats.recordRateLimited()
			c.pause.pauseFor(retryAfter(resp.Header))
			c.breaker.Failure()
			return nil, fmt.Errorf("GET %s: HTTP 418 (IP banned, retry after %s)", url, retryAfter(resp.Header))
		case http.StatusOK:
			c.breaker.Success()
			return body, nil
		default:
			c.breaker.Failure()
			return nil, fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, truncateBody(body, 200))
		}
		break
	}

	c.breaker.Failure()
	return nil, lastErr
}

// observeServerWeight records X-MBX-USED-WEIGHT-1M and pauses the client
// until the next minute when it reaches the guard.
func (c *HTTPClient) observeServerWeight(h http.Header, stats *fetchStats) {
	n := usedWeight1m(h)
	if n <= 0 {
		return
	}
	c.counters.observeUsedWeight(n)
	stats.recordServerWeight(n)
	guard := c.weightGuard
	if guard <= 0 {
		guard = DefaultServerWeightGuard
	}
	if n >= guard {
		c.pause.pauseFor(untilNextMinute(time.Now()))
	}
}

// fetchJSON performs a GET request and unmarshals the JSON response into target.
func fetchJSON[T any](ctx context.Context, c *HTTPClient, path string) (T, error) {
	var zero T
	body, err := c.doPublic(ctx, path)
	if err != nil {
		return zero, err
	}
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return zero, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseFloat parses a Binance string price field to float64.
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// truncateBody truncates a byte slice for error messages.
func truncateBody(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}

// isRetryable returns true if the error is a transient network issue
// (connection reset, timeout, EOF) that should be retried.
func isRetryable(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "TLS handshake timeout") ||
		strings.Contains(s, "Client.Timeout exceeded") ||
		strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "EOF") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "server closed idle connection")
}

// parseKlines parses the raw kline response from Binance into Kline structs.
// Binance returns klines as arrays of arrays: [openTime, open, high, low, close, volume, closeTime, ...]
func parseKlines(raw [][]interface{}) []Kline {
	out := make([]Kline, 0, len(raw))
	for _, r := range raw {
		if len(r) < 8 {
			continue
		}
		k := Kline{
			OpenTime:  toInt64(r[0]),
			Open:      toFloat64(r[1]),
			High:      toFloat64(r[2]),
			Low:       toFloat64(r[3]),
			Close:     toFloat64(r[4]),
			Volume:    toFloat64(r[5]),
			CloseTime: toInt64(r[6]),
		}
		// Taker buy base volume is at index 9
		if len(r) >= 10 {
			k.TakerBuy = toFloat64(r[9])
		}
		// Skip all-zero klines
		if k.Close == 0 && k.Volume == 0 {
			continue
		}
		out = append(out, k)
	}
	return out
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return int64(f)
	default:
		return 0
	}
}

// jsonUnmarshal is a convenience wrapper around json.Unmarshal.
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
