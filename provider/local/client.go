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

// klineBar represents a single Binance kline (candlestick).
type klineBar struct {
	OpenTime  int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Trades             int64
	TakerBuyBaseVolume float64
	TakerBuyQuoteVol   float64
}

// fetchKlines fetches klines from Binance Futures API.
// interval: "1m", "5m", "15m", "1h", "4h", etc.
// limit: number of klines (max 1500).
func (c *Client) fetchKlines(symbol, interval string, limit int) ([]klineBar, error) {
	url := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
		c.BinanceURL, symbol, interval, limit)

	var raw [][]interface{}
	if err := c.fetchJSON(url, &raw); err != nil {
		return nil, fmt.Errorf("local: klines fetch failed for %s: %w", symbol, err)
	}

	bars := make([]klineBar, 0, len(raw))
	for _, r := range raw {
		if len(r) < 8 {
			continue
		}
		bar := klineBar{
			OpenTime: int64(toFloat(r[0])),
			Open:     toFloat(r[1]),
			High:     toFloat(r[2]),
			Low:      toFloat(r[3]),
			Close:    toFloat(r[4]),
			Volume:   toFloat(r[5]),
			Trades:   int64(toFloat(r[8])),
		}
		if len(r) >= 11 {
			bar.TakerBuyBaseVolume = toFloat(r[9])
			bar.TakerBuyQuoteVol = toFloat(r[10])
		}
		bars = append(bars, bar)
	}
	return bars, nil
}

// toFloat converts an interface{} (from JSON number) to float64.
func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

// fetchOIHist fetches OI history for a symbol from Binance.
func (c *Client) fetchOIHist(symbol, period string, limit int) (deltaPercent float64, err error) {
	url := fmt.Sprintf("%s/futures/data/openInterestHist?symbol=%s&period=%s&limit=%d",
		c.BinanceURL, symbol, period, limit)

	type oiEntry struct {
		Symbol               string `json:"symbol"`
		SumOpenInterest      string `json:"sumOpenInterest"`
		SumOpenInterestValue string `json:"sumOpenInterestValue"`
		Timestamp            int64  `json:"timestamp"`
	}

	var entries []oiEntry
	if err := c.fetchJSON(url, &entries); err != nil {
		return 0, err
	}
	if len(entries) < 2 {
		return 0, nil
	}

	oldest := parseFloat(entries[0].SumOpenInterestValue)
	newest := parseFloat(entries[len(entries)-1].SumOpenInterestValue)
	if oldest > 0 {
		deltaPercent = (newest - oldest) / oldest * 100
	}
	return deltaPercent, nil
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

// v6: excludedTokenizedAssets filters out tokenized commodities/stocks
// that behave differently from crypto and shouldn't be scored by Hunter.
var excludedTokenizedAssets = map[string]bool{
	"CLUSDT": true,    // 原油
	"XAUUSDT": true,   // 黄金
	"XAGUSDT": true,   // 白银
	"EWYUSDT": true,   // 韩国ETF
	"NVDAUSDT": true,  // 英伟达
	"MUUSDT": true,    // 美光
	"INTCUSDT": true,  // 英特尔
	"PAXGUSDT": true,  // Pax Gold
	"SPCXUSDT": true,  // S&P 500
	"BABAUSDT": true,  // 阿里巴巴
	"TSLAUSDT": true,  // 特斯拉
	"NATGASUSDT": true, // 天然气
}
