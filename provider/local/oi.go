package local

import (
	"fmt"
	"log"
	"sort"
	"time"

	"nofx/provider/nofxos"
)

// openInterestResp matches Binance /fapi/v1/openInterest response.
type openInterestResp struct {
	OpenInterest string `json:"openInterest"`
	Symbol       string `json:"symbol"`
	Time         int64  `json:"time"`
}

// oiHistEntry matches a single element of the
// GET /futures/data/openInterestHist?symbol=X&period=1h&limit=2 response.
type oiHistEntry struct {
	Symbol               string `json:"symbol"`
	SumOpenInterest      string `json:"sumOpenInterest"`
	SumOpenInterestValue string `json:"sumOpenInterestValue"`
	Timestamp            int64  `json:"timestamp"`
}

// GetOIRanking returns an OIRankingData with OI increase (TopPositions) and
// OI decrease (LowPositions) rankings built from Binance public data.
//
// Approach (pragmatic ≤52 requests):
// 1. Fetch all 24h tickers.
// 2. Filter USDT perps; sort by quoteVolume descending; keep top 50.
// 3. For each of those 50 symbols, fetch OI history (period=1h, limit=2).
// 4. Compute OI delta = (latest - oldest) / oldest * 100.
// 5. Sort by delta: biggest increase → TopPositions, biggest decrease → LowPositions.
// 6. Clamp to top |limit| entries in each bucket.
// 7. Cache for CacheTTLHistory (5 min).
func (c *Client) GetOIRanking(duration string, limit int) (*nofxos.OIRankingData, error) {
	if duration == "" {
		duration = "1h"
	}
	if limit <= 0 {
		limit = 20
	}

	cacheKey := fmt.Sprintf("oi_ranking_%s_%d", duration, limit)
	if hit, ok := c.cache.Get(cacheKey); ok {
		return hit.(*nofxos.OIRankingData), nil
	}

	result := &nofxos.OIRankingData{
		Duration:  duration,
		TimeRange: "latest",
		FetchedAt: time.Now(),
	}

	positions, err := c.fetchOIPositions(duration)
	if err != nil {
		log.Printf("⚠️  Local OI ranking: %v", err)
		return result, nil // Return empty rather than hard error
	}

	// Sort by OIDeltaPercent descending (biggest increase first)
	sort.SliceStable(positions, func(i, j int) bool {
		return positions[i].OIDeltaPercent > positions[j].OIDeltaPercent
	})

	half := limit
	if half > len(positions) {
		half = len(positions)
	}
	result.TopPositions = positions[:half]

	// Adjust low limit: use bottom of sorted slice
	if limit > len(positions) {
		limit = len(positions)
	}
	lowStart := len(positions) - limit
	if lowStart < 0 {
		lowStart = 0
	}
	// take bottom 'limit' entries (sorted ascending by OIDeltaPercent = biggest decreases)
	lowPositions := make([]nofxos.OIPosition, limit)
	for i := 0; i < limit && lowStart+i < len(positions); i++ {
		lowPositions[i] = positions[lowStart+i]
	}
	result.LowPositions = lowPositions

	c.cache.Set(cacheKey, result, CacheTTLHistory)

	log.Printf("✓ Local OI ranking: %d top, %d low (duration=%s)",
		len(result.TopPositions), len(result.LowPositions), duration)
	return result, nil
}

// GetOITopPositions is a convenience wrapper returning the first 20 OI
// increase positions (1h).
func (c *Client) GetOITopPositions() ([]nofxos.OIPosition, error) {
	data, err := c.GetOIRanking("1h", 20)
	if err != nil {
		return nil, err
	}
	return data.TopPositions, nil
}

// GetOILowPositions is a convenience wrapper returning the first 20 OI
// decrease positions (1h).
func (c *Client) GetOILowPositions() ([]nofxos.OIPosition, error) {
	data, err := c.GetOIRanking("1h", 20)
	if err != nil {
		return nil, err
	}
	return data.LowPositions, nil
}

// fetchOIPositions returns OIPosition entries for the top-50-volume USDT perps.
func (c *Client) fetchOIPositions(period string) ([]nofxos.OIPosition, error) {
	// Step 1 — all tickers
	url := c.BinanceURL + "/fapi/v1/ticker/24hr"
	var tickers []binanceTicker
	if err := c.fetchJSON(url, &tickers); err != nil {
		return nil, fmt.Errorf("fetch tickers: %w", err)
	}

	// Step 2 — filter USDT perps, sort by quoteVolume desc, take top 50
	type symVol struct {
		symbol  string
		qv      float64
		price   float64
		pctChg  float64
	}
	var filtered []symVol
	for _, t := range tickers {
		if !isUSDTPerp(t.Symbol) {
			continue
		}
		filtered = append(filtered, symVol{
			symbol: t.Symbol,
			qv:     parseFloat(t.QuoteVolume),
			price:  parseFloat(t.LastPrice),
			pctChg: parseFloat(t.PriceChangePercent),
		})
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].qv > filtered[j].qv
	})
	if len(filtered) > 50 {
		filtered = filtered[:50]
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	// Step 3 — fetch OI history (period, limit=2) for each symbol
	type oiDelta struct {
		sym          symVol
		currentOI    float64
		oiDelta      float64 // absolute change in base units
		oiDeltaVal   float64 // change in USDT value
		oiDeltaPct   float64 // percent change
	}

	var results []oiDelta
	for _, sv := range filtered {
		histURL := fmt.Sprintf("%s/futures/data/openInterestHist?symbol=%s&period=%s&limit=2",
			c.BinanceURL, sv.symbol, period)

		var hist []oiHistEntry
		if err := c.fetchJSON(histURL, &hist); err != nil {
			log.Printf("⚠️  Local OI: skip %s: %v", sv.symbol, err)
			continue
		}

		if len(hist) < 2 {
			continue
		}

		oldest := parseFloat(hist[0].SumOpenInterestValue)
		newest := parseFloat(hist[1].SumOpenInterestValue)
		currentOI := parseFloat(hist[1].SumOpenInterest)

		delta := newest - oldest
		deltaPct := 0.0
		if oldest > 0 {
			deltaPct = (delta / oldest) * 100
		}

		results = append(results, oiDelta{
			sym:        sv,
			currentOI:  currentOI,
			oiDelta:    delta,
			oiDeltaVal: delta,
			oiDeltaPct: deltaPct,
		})
	}

	if len(results) == 0 {
		return nil, nil
	}

	// Sort by deltaPct descending
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].oiDeltaPct > results[j].oiDeltaPct
	})

	// Build OIPosition slice
	positions := make([]nofxos.OIPosition, len(results))
	for i, r := range results {
		positions[i] = nofxos.OIPosition{
			Symbol:            r.sym.symbol,
			Rank:              i + 1,
			Price:             r.sym.price,
			CurrentOI:         r.currentOI,
			OIDelta:           r.oiDelta,
			OIDeltaPercent:    r.oiDeltaPct,
			OIDeltaValue:      r.oiDeltaVal,
			PriceDeltaPercent: r.sym.pctChg,
		}
	}

	return positions, nil
}
