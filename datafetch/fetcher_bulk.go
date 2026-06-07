package datafetch

import (
	"context"
	"fmt"
	"strings"
)

// fetchAllTickers fetches 24hr ticker data for ALL USDT-M futures symbols in one call.
func (f *DataFetcher) fetchAllTickers(ctx context.Context) (map[string]*ticker24hrRaw, error) {
	path := "/fapi/v1/ticker/24hr"
	raw, err := fetchJSON[[]ticker24hrRaw](f.client, path)
	if err != nil {
		return nil, fmt.Errorf("fetchAllTickers: %w", err)
	}
	out := make(map[string]*ticker24hrRaw, len(raw))
	for i := range raw {
		out[raw[i].Symbol] = &raw[i]
	}
	return out, nil
}

// fetchAllPremiumIndex fetches premium index (mark price, funding rate) for ALL symbols.
func (f *DataFetcher) fetchAllPremiumIndex(ctx context.Context) (map[string]*premiumIndexRaw, error) {
	path := "/fapi/v1/premiumIndex"
	raw, err := fetchJSON[[]premiumIndexRaw](f.client, path)
	if err != nil {
		return nil, fmt.Errorf("fetchAllPremiumIndex: %w", err)
	}
	out := make(map[string]*premiumIndexRaw, len(raw))
	for i := range raw {
		out[raw[i].Symbol] = &raw[i]
	}
	return out, nil
}

// fetchExchangeInfo fetches the full exchange info and returns trading USDT perpetual symbols.
func (f *DataFetcher) fetchExchangeInfo(ctx context.Context) ([]string, error) {
	path := "/fapi/v1/exchangeInfo"
	raw, err := fetchJSON[exchangeInfoRaw](f.client, path)
	if err != nil {
		return nil, fmt.Errorf("fetchExchangeInfo: %w", err)
	}
	var symbols []string
	for _, s := range raw.Symbols {
		if s.Status == "TRADING" && s.ContractType == "PERPETUAL" && strings.HasSuffix(s.Symbol, "USDT") {
			if excludedNonCryptoFuturesSymbol(s) {
				continue
			}
			symbols = append(symbols, s.Symbol)
		}
	}
	return symbols, nil
}

func excludedNonCryptoFuturesSymbol(s exchangeSymbolRaw) bool {
	if excludedNonCryptoFuturesSymbols[s.Symbol] {
		return true
	}
	if s.UnderlyingType != "" && s.UnderlyingType != "COIN" {
		return true
	}
	if excludedNonCryptoBaseAssets[s.BaseAsset] {
		return true
	}
	for _, subtype := range s.UnderlyingSubType {
		if excludedNonCryptoUnderlyingSubtypes[strings.ToUpper(subtype)] {
			return true
		}
	}
	return false
}

var excludedNonCryptoFuturesSymbols = map[string]bool{
	"CLUSDT":   true, // crude oil
	"XAUUSDT":  true, // gold
	"XAUTUSDT": true, // Tether Gold
	"XAGUSDT":  true, // silver
	"EWYUSDT":  true, // Korea ETF
	"NVDAUSDT": true,
	"MUUSDT":   true,
	"INTCUSDT": true,
	"PAXGUSDT": true, // Pax Gold
	"SPCXUSDT": true, // S&P 500
	"BABAUSDT": true,
	"TSLAUSDT": true,
}

var excludedNonCryptoBaseAssets = map[string]bool{
	"CL":   true,
	"XAU":  true,
	"XAUT": true,
	"XAG":  true,
	"PAXG": true,
	"EWY":  true,
	"NVDA": true,
	"MU":   true,
	"INTC": true,
	"SPCX": true,
	"BABA": true,
	"TSLA": true,
}

var excludedNonCryptoUnderlyingSubtypes = map[string]bool{
	"COMMODITY": true,
	"METAL":     true,
	"STOCK":     true,
	"EQUITY":    true,
	"ETF":       true,
	"INDEX":     true,
	"RWA":       true,
}
