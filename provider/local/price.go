package local

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"nofx/provider/nofxos"
)

// GetPriceRanking returns top gainers and losers based on Binance 24h
// ticker data.  The durations parameter is accepted for API-compatibility
// but only 24h is meaningful when all we have is the 24h ticker.
// For a richer implementation with 1h / 4h klines we would need additional
// endpoints — left as future work.
//
// Results are cached for CacheTTLTicker (60 s).
func (c *Client) GetPriceRanking(durations string, limit int) (*nofxos.PriceRankingData, error) {
	if strings.TrimSpace(durations) == "" {
		durations = "1h"
	}
	if limit <= 0 {
		limit = 10
	}

	cacheKey := fmt.Sprintf("price_%s_%d", durations, limit)
	if hit, ok := c.cache.Get(cacheKey); ok {
		return hit.(*nofxos.PriceRankingData), nil
	}

	// Fetch all tickers
	url := c.BinanceURL + "/fapi/v1/ticker/24hr"
	var tickers []binanceTicker
	if err := c.fetchJSON(url, &tickers); err != nil {
		return nil, fmt.Errorf("local: PriceRanking fetch tickers failed: %w", err)
	}

	type scored struct {
		symbol string
		price  float64
		pct    float64 // raw decimal: 0.07 == 7 %
	}

	var pool []scored
	for _, t := range tickers {
		if !isUSDTPerp(t.Symbol) {
			continue
		}
		pct := parseFloat(t.PriceChangePercent) / 100 // ticker uses %, struct uses decimal
		pool = append(pool, scored{
			symbol: t.Symbol,
			price:  parseFloat(t.LastPrice),
			pct:    pct,
		})
	}

	if len(pool) == 0 {
		empty := &nofxos.PriceRankingData{Durations: map[string]*nofxos.PriceRankingDuration{}}
		return empty, nil
	}

	// Sort by pct descending for gains, ascending for losses
	makeDuration := func(tag string) *nofxos.PriceRankingDuration {
		// Top gainers
		sortedDesc := make([]scored, len(pool))
		copy(sortedDesc, pool)
		sort.SliceStable(sortedDesc, func(i, j int) bool {
			return sortedDesc[i].pct > sortedDesc[j].pct
		})
		n := limit
		if n > len(sortedDesc) {
			n = len(sortedDesc)
		}
		top := make([]nofxos.PriceRankingItem, n)
		for i := 0; i < n; i++ {
			top[i] = nofxos.PriceRankingItem{
				Pair:       tag + "_" + sortedDesc[i].symbol,
				Symbol:     sortedDesc[i].symbol,
				PriceDelta: sortedDesc[i].pct,
				Price:      sortedDesc[i].price,
			}
		}

		// Bottom losers (most negative)
		sortedAsc := make([]scored, len(pool))
		copy(sortedAsc, pool)
		sort.SliceStable(sortedAsc, func(i, j int) bool {
			return sortedAsc[i].pct < sortedAsc[j].pct
		})
		low := make([]nofxos.PriceRankingItem, n)
		for i := 0; i < n; i++ {
			low[i] = nofxos.PriceRankingItem{
				Pair:       tag + "_" + sortedAsc[i].symbol,
				Symbol:     sortedAsc[i].symbol,
				PriceDelta: sortedAsc[i].pct,
				Price:      sortedAsc[i].price,
			}
		}
		return &nofxos.PriceRankingDuration{Top: top, Low: low}
	}

	// Build entries for each requested duration tag
	// In a real implementation each duration would fetch klines for that period.
	// For now, all durations share the same 24h ticker snapshot.
	durationTags := strings.Split(durations, ",")
	durMap := make(map[string]*nofxos.PriceRankingDuration)
	for _, tag := range durationTags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		durMap[tag] = makeDuration(tag)
	}

	result := &nofxos.PriceRankingData{
		Durations: durMap,
		FetchedAt: time.Now(),
	}

	c.cache.Set(cacheKey, result, CacheTTLTicker)

	log.Printf("✓ Local PriceRanking: %d durations, pool=%d symbols",
		len(durMap), len(pool))
	return result, nil
}
