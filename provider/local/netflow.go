package local

import (
	"fmt"
	"log"
	"sort"
	"time"

	"nofx/provider/nofxos"
)

// GetNetFlowRanking returns a NetFlowRankingData built as a proxy from
// Binance 24h ticker data.
//
// NofxOS NetFlow separates "institution" vs "personal" futures fund flow.
// Binance does not expose that split publicly, so we approximate:
//
//	Institution flow proxy: top/bottom symbols by quoteVolume (smart money
//	  concentrates in high-volume pairs).
//	Personal flow proxy:   top/bottom symbols by trade count (retail activity
//	  correlates with number of trades).
//
// sign convention (matches nofxos):
//
//	positive amount = net inflow (buy pressure)
//	negative amount = net outflow (sell pressure)
//
// result is cached for CacheTTLTicker (60 s).
func (c *Client) GetNetFlowRanking(duration string, limit int) (*nofxos.NetFlowRankingData, error) {
	if duration == "" {
		duration = "1h"
	}
	if limit <= 0 {
		limit = 10
	}

	cacheKey := fmt.Sprintf("netflow_%s_%d", duration, limit)
	if hit, ok := c.cache.Get(cacheKey); ok {
		return hit.(*nofxos.NetFlowRankingData), nil
	}

	result := &nofxos.NetFlowRankingData{
		Duration:  duration,
		TimeRange: "latest",
		FetchedAt: time.Now(),
	}

	// Fetch all tickers once
	url := c.BinanceURL + "/fapi/v1/ticker/24hr"
	var tickers []binanceTicker
	if err := c.fetchJSON(url, &tickers); err != nil {
		log.Printf("⚠️  Local NetFlow: fetch tickers failed: %v", err)
		return result, nil
	}

	type flowItem struct {
		symbol   string
		price    float64
		qv       float64 // quote volume (institution proxy)
		count    float64  // trades (retail proxy)
		pctChg   float64
	}

	var items []flowItem
	for _, t := range tickers {
		if !isUSDTPerp(t.Symbol) {
			continue
		}
		pct := parseFloat(t.PriceChangePercent)
		// Determine flow sign from price direction
		// price up  → inflow (positive)
		// price down → outflow (negative)
		qv := parseFloat(t.QuoteVolume)
		cnt := float64(t.Count)
		if pct < 0 {
			qv = -qv
			cnt = -cnt
		}
		items = append(items, flowItem{
			symbol: t.Symbol,
			price:  parseFloat(t.LastPrice),
			qv:     qv,
			count:  cnt,
			pctChg: pct,
		})
	}

	// Helper to pick top-N and bottom-N from a slice sorted by a getter
	topBottom := func(data []flowItem, getter func(flowItem) float64, n int) (top, low []nofxos.NetFlowPosition) {
		sorted := make([]flowItem, len(data))
		copy(sorted, data)
		sort.SliceStable(sorted, func(i, j int) bool {
			return getter(sorted[i]) > getter(sorted[j])
		})
		if n > len(sorted) {
			n = len(sorted)
		}
		for i := 0; i < n; i++ {
			it := sorted[i]
			top = append(top, nofxos.NetFlowPosition{
				Rank:   i + 1,
				Symbol: it.symbol,
				Amount: getter(it),
				Price:  it.price,
			})
		}
		// Bottom-N (ascending order)
		sort.SliceStable(sorted, func(i, j int) bool {
			return getter(sorted[i]) < getter(sorted[j])
		})
		lowN := n
		if lowN > len(sorted) {
			lowN = len(sorted)
		}
		for i := 0; i < lowN; i++ {
			it := sorted[i]
			low = append(low, nofxos.NetFlowPosition{
				Rank:   i + 1,
				Symbol: it.symbol,
				Amount: getter(it),
				Price:  it.price,
			})
		}
		return
	}

	// Institution: ranked by quoteVolume (signed by price direction)
	instGetQV := func(it flowItem) float64 { return it.qv }
	result.InstitutionFutureTop, result.InstitutionFutureLow = topBottom(items, instGetQV, limit)

	// Personal: ranked by trade count (signed by price direction)
	persGetCount := func(it flowItem) float64 { return it.count }
	result.PersonalFutureTop, result.PersonalFutureLow = topBottom(items, persGetCount, limit)

	c.cache.Set(cacheKey, result, CacheTTLTicker)

	log.Printf("✓ Local NetFlow: inst_in=%d inst_out=%d retail_in=%d retail_out=%d (duration=%s)",
		len(result.InstitutionFutureTop), len(result.InstitutionFutureLow),
		len(result.PersonalFutureTop), len(result.PersonalFutureLow), duration)
	return result, nil
}
