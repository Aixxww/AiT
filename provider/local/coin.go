package local

import (
	"fmt"
	"log"
	"strings"

	"nofx/provider/nofxos"
)

// openInterestSingle matches GET /fapi/v1/openInterest?symbol=X response.
type openInterestSingle struct {
	OpenInterest string `json:"openInterest"`
	Symbol       string `json:"symbol"`
	Time         int64  `json:"time"`
}

// GetCoinData fetches quantitative data for a single coin from Binance
// public endpoints (24h ticker + current OI).
// The include parameter is accepted for API-compatibility but is ignored —
// all available data is always returned.
func (c *Client) GetCoinData(symbol string, include string) (*nofxos.QuantData, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if !strings.HasSuffix(sym, "USDT") {
		sym = sym + "USDT"
	}

	const cachePrefix = "coin_"
	cacheKey := cachePrefix + sym
	if hit, ok := c.cache.Get(cacheKey); ok {
		return hit.(*nofxos.QuantData), nil
	}

	result := &nofxos.QuantData{
		Symbol:      sym,
		PriceChange: make(map[string]float64),
		OI:          make(map[string]*nofxos.OIData),
	}

	// ---- 24h ticker ----
	tickerURL := fmt.Sprintf("%s/fapi/v1/ticker/24hr?symbol=%s", c.BinanceURL, sym)
	var t binanceTicker
	if err := c.fetchJSON(tickerURL, &t); err != nil {
		return nil, fmt.Errorf("local: ticker for %s: %w", sym, err)
	}

	result.Price = parseFloat(t.LastPrice)
	// priceChangePercent is in %; QuantData expects decimal (x100 in nofxos)
	pctRaw := parseFloat(t.PriceChangePercent) / 100
	result.PriceChange["24h"] = pctRaw
	// We can't compute 1h/4h/8h/12h from the 24h ticker; fill only what we know.
	// Use high/low as a crude intraday extreme indicator.
	high := parseFloat(t.HighPrice)
	low := parseFloat(t.LowPrice)
	if result.Price > 0 {
		result.PriceChange["intraday_range"] = (high - low) / result.Price
	}

	// ---- Current OI (single call) ----
	oiURL := fmt.Sprintf("%s/fapi/v1/openInterest?symbol=%s", c.BinanceURL, sym)
	var oiResp openInterestSingle
	if err := c.fetchJSON(oiURL, &oiResp); err != nil {
		log.Printf("⚠️  Local: OI for %s skipped: %v", sym, err)
		// Continue without OI — not a fatal error
	} else {
		oiVal := parseFloat(oiResp.OpenInterest)
		result.OI["binance"] = &nofxos.OIData{
			CurrentOI: oiVal,
		}
	}

	// Netflow is not available via public Binance endpoints.
	// Leave result.Netflow = nil (CheckForAI handles nil gracefully).

	c.cache.Set(cacheKey, result, CacheTTLTicker)
	return result, nil
}

// GetCoinDataBatch calls GetCoinData for every symbol in symbols and returns
// a map keyed by normalized symbol name. Failures for individual coins are
// logged and silently skipped.
// This signature matches nofxos.Client exactly (no error return).
func (c *Client) GetCoinDataBatch(symbols []string, include string) map[string]*nofxos.QuantData {
	result := make(map[string]*nofxos.QuantData)
	for _, sym := range symbols {
		data, err := c.GetCoinData(sym, include)
		if err != nil {
			log.Printf("⚠️  Local: GetCoinDataBatch skip %s: %v", sym, err)
			continue
		}
		if data != nil {
			result[normalizeSymbol(sym)] = data
		}
	}
	return result
}
