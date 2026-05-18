package local

import (
	"fmt"
	"log"
	"sort"
	"strings"
)

// CoinGecko free API (no key required, ~30 calls/min).
const coinGeckoDerivativesURL = "https://api.coingecko.com/api/v3/derivatives"

// coinGeckoDerivative is the subset of fields we use from the
// CoinGecko /api/v3/derivatives response.
type coinGeckoDerivative struct {
	Market       string  `json:"market"`        // e.g. "Binance Futures"
	Symbol       string  `json:"symbol"`        // e.g. "BTC/USDT:USDT"
	Price        float64 `json:"price"`         // last price (number, not string)
	IndexPrice   float64 `json:"index_price"`   // index price
	OpenInterest float64 `json:"open_interest"` // OI in quote currency
}

// fetchCoinGeckoDerivatives calls the CoinGecko derivatives endpoint,
// filters to Binance USDT perpetual contracts, converts to binanceTicker
// structs (price/volume/OI populated, other fields zeroed), sorts by OI
// descending, and returns up to maxPool entries.
//
// Only symbols present in validSymbols are kept (when non-empty), ensuring
// the pool is restricted to known Binance-listed contracts.
func (c *Client) fetchCoinGeckoDerivatives(validSymbols map[string]bool, maxPool int) ([]binanceTicker, error) {
	var raw []coinGeckoDerivative
	if err := c.fetchJSON(coinGeckoDerivativesURL, &raw); err != nil {
		return nil, fmt.Errorf("coingecko: fetch derivatives failed: %w", err)
	}

	var tickers []binanceTicker
	for _, d := range raw {
		if !strings.Contains(strings.ToLower(d.Market), "binance") {
			continue
		}
		symbol := cgSymbolToBinance(d.Symbol)
		if symbol == "" {
			continue
		}
		if !isUSDTPerp(symbol) {
			continue
		}
		// If we have a valid symbol list, enforce it
		if validSymbols != nil && !validSymbols[symbol] {
			continue
		}
		if d.Price <= 0 {
			continue
		}

		// QuoteVolume approximation: OI is the best proxy we have from CoinGecko
		oiValue := d.OpenInterest
		tickers = append(tickers, binanceTicker{
			Symbol:      symbol,
			LastPrice:   fmt.Sprintf("%.8f", d.Price),
			QuoteVolume: fmt.Sprintf("%.2f", oiValue),
			// PriceChangePercent, Volume, HighPrice, LowPrice, Count
			// are unavailable from CoinGecko derivatives endpoint.
			// Scoring pillars that depend on them will return 0 gracefully.
		})
	}

	// Sort by OI descending (QuoteVolume proxy)
	sort.SliceStable(tickers, func(i, j int) bool {
		return parseFloat(tickers[i].QuoteVolume) > parseFloat(tickers[j].QuoteVolume)
	})

	if maxPool > 0 && len(tickers) > maxPool {
		tickers = tickers[:maxPool]
	}

	log.Printf("🔗 CoinGecko fallback: loaded %d Binance USDT perp tickers (filtered from %d total derivatives)",
		len(tickers), len(raw))
	return tickers, nil
}

// fetchBinanceUSDTPerpSymbols calls the Binance /fapi/v1/exchangeInfo endpoint
// to get the definitive list of active USDT perpetual contract symbols.
// This endpoint has a separate rate-limit budget from /fapi/v1/ticker/24hr
// and may still work when the ticker endpoint is banned.
func (c *Client) fetchBinanceUSDTPerpSymbols() (map[string]bool, error) {
	url := c.BinanceURL + "/fapi/v1/exchangeInfo"

	type exchangeSymbol struct {
		Symbol string `json:"symbol"`
		Status string `json:"status"`
	}
	type exchangeInfo struct {
		Symbols []exchangeSymbol `json:"symbols"`
	}

	var info exchangeInfo
	if err := c.fetchJSON(url, &info); err != nil {
		return nil, fmt.Errorf("binance: fetch exchangeInfo failed: %w", err)
	}

	result := make(map[string]bool, len(info.Symbols))
	for _, s := range info.Symbols {
		if s.Status == "TRADING" && isUSDTPerp(s.Symbol) {
			result[s.Symbol] = true
		}
	}

	log.Printf("🔗 Binance exchangeInfo: found %d active USDT perpetual symbols", len(result))
	return result, nil
}

// cgSymbolToBinance converts a CoinGecko derivatives symbol like
// "BTC/USDT:USDT" to Binance format "BTCUSDT".
// Returns empty string if the format doesn't match.
func cgSymbolToBinance(cgSymbol string) string {
	// Expected format: "BASE/USDT:USDT" or "BASE/USDT"
	parts := strings.Split(cgSymbol, "/")
	if len(parts) != 2 {
		return ""
	}
	base := strings.TrimSpace(parts[0])
	quotePart := strings.TrimSpace(parts[1])

	// quotePart may be "USDT:USDT" or just "USDT"
	quoteFields := strings.Split(quotePart, ":")
	quote := quoteFields[0]

	if !strings.EqualFold(quote, "USDT") {
		return ""
	}

	return strings.ToUpper(base) + "USDT"
}
