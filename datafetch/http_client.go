package datafetch

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
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

// Acquire blocks until one token is available.
func (r *RateLimiter) Acquire() {
	for {
		r.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(r.lastRefill).Seconds()
		r.lastRefill = now
		r.tokens = math.Min(r.maxTokens, r.tokens+elapsed*r.refillRate)
		if r.tokens >= 1.0 {
			r.tokens -= 1.0
			r.mu.Unlock()
			return
		}
		// How long until 1 token is available?
		wait := time.Duration((1.0 - r.tokens) / r.refillRate * float64(time.Second))
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
}

// NewHTTPClient creates an HTTP client with connection pooling, rate limiting,
// and circuit breaking for Binance API calls.
func NewHTTPClient(baseURL string) *HTTPClient {
	// Use DisableKeepAlives to force fresh DNS resolution per request.
	// provider/local uses fetchJSON which creates a new http.Client per call,
	// getting different (working) CloudFront edge IPs. Shared connection pools
	// get stuck on broken edges. DisableKeepAlives mimics this behavior.
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        400,
		MaxIdleConnsPerHost: 200,
		IdleConnTimeout:     90 * time.Second,
		MaxConnsPerHost:     200,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
		DisableKeepAlives:   true,
	}
	inner := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
	return &HTTPClient{
		inner:   inner,
		limiter: NewRateLimiter(1200), // 1200 tokens/min for Binance
		breaker: NewCircuitBreaker(5, 30*time.Second),
		baseURL: baseURL,
	}
}

// doPublic performs a rate-limited, circuit-breaker-protected GET request
// to a Binance public endpoint. Returns the response body.
// Retries on transient network errors (connection reset, timeout, EOF).
// Circuit breaker failure is only recorded after ALL retries are exhausted.
func (c *HTTPClient) doPublic(path string) ([]byte, error) {
	log.Printf("[doPublic] DEBUG: retry-enabled v2 path=%s", path)
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("circuit breaker open for %s", path)
	}

	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(300*(1<<attempt)) * time.Millisecond)
		}
		c.limiter.Acquire()

		url := c.baseURL + path
		resp, err := c.inner.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("GET %s: %w", url, err)
			if attempt < maxRetries && isRetryable(err) {
				continue
			}
			break
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read body %s: %w", url, err)
			if attempt < maxRetries && isRetryable(err) {
				continue
			}
			break
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("GET %s: rate limited (429)", url)
			if attempt < maxRetries {
				continue
			}
			break
		}

		if resp.StatusCode != http.StatusOK {
			c.breaker.Failure()
			return nil, fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, truncateBody(body, 200))
		}

		c.breaker.Success()
		return body, nil
	}

	c.breaker.Failure()
	return nil, lastErr
}

// fetchJSON performs a GET request and unmarshals the JSON response into target.
func fetchJSON[T any](c *HTTPClient, path string) (T, error) {
	var zero T
	body, err := c.doPublic(path)
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
	s := err.Error()
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "i/o timeout") ||
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
