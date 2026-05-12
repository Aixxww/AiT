// Package local provides a local data provider that replaces the NofxOS API
// (https://nofxos.ai) using Binance public futures endpoints.
//
// All public method signatures match nofxos.Client so the two are
// interchangeable at the call site once the engine references an interface.
package local

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Default Binance USDT-M Futures base URL (public, no auth required).
const (
	DefaultBinanceURL = "https://fapi.binance.com"
	DefaultTimeout    = 30 * time.Second
)

// Client is a Binance-backed local data provider.
// It implements the same public method set as nofxos.Client so it can be
// dropped into the strategy engine without changing call sites.
type Client struct {
	BinanceURL string
	Timeout    time.Duration
	cache      *Cache
	mu         sync.Mutex
	lastCall   time.Time
}

// NewClient creates a local data provider backed by Binance public APIs.
// binanceURL defaults to DefaultBinanceURL when empty.
func NewClient(binanceURL string) *Client {
	if strings.TrimSpace(binanceURL) == "" {
		binanceURL = DefaultBinanceURL
	}
	return &Client{
		BinanceURL: strings.TrimRight(binanceURL, "/"),
		Timeout:    DefaultTimeout,
		cache:      NewCache(),
		lastCall:   time.Time{},
	}
}

// rateLimit performs a simple time-based rate limit.
// Ensures at least ~120 ms between requests (~8 req/s) to stay well within
// Binance's public API limit of 1200 weight / minute.
func (c *Client) rateLimit() {
	c.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(c.lastCall)
	if elapsed < 120*time.Millisecond {
		time.Sleep(120*time.Millisecond - elapsed)
	}
	c.lastCall = time.Now()
	c.mu.Unlock()
}

// fetchJSON performs a rate-limited HTTP GET, reads the body, and
// unmarshals it into target. Uses a plain http.Client (no SSRF protection)
// because these are outbound calls to the public Binance API.
func (c *Client) fetchJSON(url string, target interface{}) error {
	c.rateLimit()

	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("local: GET %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("local: read body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("local: %s returned HTTP %d: %s", url, resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("local: JSON unmarshal failed for %s: %w", url, err)
	}

	return nil
}

// fetchBytes is like fetchJSON but returns the raw response body.
func (c *Client) fetchBytes(url string) ([]byte, error) {
	c.rateLimit()

	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("local: GET %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("local: read body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return body, fmt.Errorf("local: %s returned HTTP %d: %s", url, resp.StatusCode, string(body))
	}

	return body, nil
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// normalizeSymbol normalizes a symbol to XXXUSDT format (exported for reuse).
func normalizeSymbol(sym string) string {
	sym = strings.TrimSpace(strings.ToUpper(sym))
	if !strings.HasSuffix(sym, "USDT") {
		sym = sym + "USDT"
	}
	return sym
}

// isUSDTPerp returns true if symbol looks like a USDT perpetual contract.
// Keeps only symbols that end with USDT and contain only letters before it
// (e.g. BTCUSDT, 1000PEPEUSDT). Filters out special-deliveries and tokens
// like BTCUSDT_250627 (dated futures).
func isUSDTPerp(symbol string) bool {
	if !strings.HasSuffix(symbol, "USDT") {
		return false
	}
	base := strings.TrimSuffix(symbol, "USDT")
	if base == "" {
		return false
	}
	for _, r := range base {
		if !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// excludedMainstreamCoins is a set of high-cap coins that the strategy tiers
// typically filter out. These are still scored but flagged lower so they don't
// dominate the list.
var excludedMainstreamCoins = map[string]bool{
	"BTCUSDT": true, "ETHUSDT": true, "BNBUSDT": true,
	"SOLUSDT": true, "XRPUSDT": true, "ADAUSDT": true,
	"DOGEUSDT": true, "DOTUSDT": true, "AVAXUSDT": true,
	"MATICUSDT": true, "LINKUSDT": true, "LTCUSDT": true,
	"UNIUSDT": true, "ATOMUSDT": true, "NEARUSDT": true,
	"AAVEUSDT": true, "FILUSDT": true, "TRXUSDT": true,
	"EOSUSDT": true, "ETCUSDT": true, "FTMUSDT": true,
	"ALGOUSDT": true, "XLMUSDT": true, "HBARUSDT": true,
	"APTUSDT": true, "ARBUSDT": true, "OPUSDT": true,
	"SUIUSDT": true, "INJUSDT": true, "TIAUSDT": true,
}
