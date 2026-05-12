package local

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"nofx/provider/nofxos"
)

// binanceTicker holds the fields we actually use from the
// Binance /fapi/v1/ticker/24hr response.
type binanceTicker struct {
	Symbol             string `json:"symbol"`
	PriceChangePercent string `json:"priceChangePercent"`
	LastPrice          string `json:"lastPrice"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
	Count              int64  `json:"count"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
}

// GetAI500List fetches all USDT-perpetual 24h tickers from Binance,
// computes a simplified composite score (price change + volume + activity),
// and returns the top 30 as []nofxos.CoinData.
//
// Algorithm (min-max normalised):
//
//	price_norm   = (change% - min_change%) / (max_change% - min_change%) * 100
//	volume_norm  = (quoteVol  - min_qv)    / (max_qv     - min_qv    ) * 100
//	activity_norm= (count     - min_count)  / (max_count   - min_count)  * 100
//
// score = price_norm*0.40 + volume_norm*0.35 + activity_norm*0.25
//
// Mainstream coins are down-weighted by 30% so the list favours smaller,
// higher-beta names that are more relevant to the AI trading strategy.
func (c *Client) GetAI500List() ([]nofxos.CoinData, error) {
	// ---- cache check ----
	const cacheKey = "ai500_list"
	if hit, ok := c.cache.Get(cacheKey); ok {
		return hit.([]nofxos.CoinData), nil
	}

	// ---- fetch tickers ----
	url := c.BinanceURL + "/fapi/v1/ticker/24hr"
	var tickers []binanceTicker
	if err := c.fetchJSON(url, &tickers); err != nil {
		return nil, fmt.Errorf("local: AI500 fetch tickers failed: %w", err)
	}

	// ---- filter to USDT perps ----
	type scoredTicker struct {
		ticker     binanceTicker
		pct        float64
		qv         float64
		count      float64
		normPct    float64
		normQV     float64
		normCount  float64
		finalScore float64
	}

	// Collect raw values for min-max normalisation
	var pool []scoredTicker
	var minPct, maxPct float64
	var minQV, maxQV float64
	var minCnt, maxCnt float64
	first := true

	for _, t := range tickers {
		if !isUSDTPerp(t.Symbol) {
			continue
		}

		pct := parseFloat(t.PriceChangePercent)
		qv := parseFloat(t.QuoteVolume)
		cnt := float64(t.Count)

		st := scoredTicker{ticker: t, pct: pct, qv: qv, count: cnt}
		pool = append(pool, st)

		if first {
			minPct, maxPct = pct, pct
			minQV, maxQV = qv, qv
			minCnt, maxCnt = cnt, cnt
			first = false
		} else {
			if pct < minPct {
				minPct = pct
			}
			if pct > maxPct {
				maxPct = pct
			}
			if qv < minQV {
				minQV = qv
			}
			if qv > maxQV {
				maxQV = qv
			}
			if cnt < minCnt {
				minCnt = cnt
			}
			if cnt > maxCnt {
				maxCnt = cnt
			}
		}
	}

	if len(pool) == 0 {
		log.Printf("ℹ️  Local AI500: 0 USDT perps found")
		return []nofxos.CoinData{}, nil
	}

	pctRange := maxPct - minPct
	qvRange := maxQV - minQV
	cntRange := maxCnt - minCnt

	// Avoid division by zero
	if pctRange == 0 {
		pctRange = 1
	}
	if qvRange == 0 {
		qvRange = 1
	}
	if cntRange == 0 {
		cntRange = 1
	}

	// ---- compute scores ----
	for i := range pool {
		p := &pool[i]
		p.normPct = ((p.pct - minPct) / pctRange) * 100
		p.normQV = ((p.qv - minQV) / qvRange) * 100
		p.normCount = ((p.count - minCnt) / cntRange) * 100

		score := clamp(p.normPct*0.40+p.normQV*0.35+p.normCount*0.25, 0, 100)

		// Down-weight mainstream coins so the list is richer in mid/small caps
		if excludedMainstreamCoins[p.ticker.Symbol] {
			score *= 0.70
		}
		p.finalScore = score
	}

	// ---- sort descending by score ----
	sort.SliceStable(pool, func(i, j int) bool {
		return pool[i].finalScore > pool[j].finalScore
	})

	// ---- build result (top 30) ----
	topN := 30
	if len(pool) < topN {
		topN = len(pool)
	}

	now := time.Now().Unix()
	coins := make([]nofxos.CoinData, 0, topN)
	for i := 0; i < topN; i++ {
		p := pool[i]
		price := parseFloat(p.ticker.LastPrice)
		coins = append(coins, nofxos.CoinData{
			Pair:            p.ticker.Symbol,
			Score:           p.finalScore,
			StartTime:       now,
			StartPrice:      price,
			LastScore:       p.finalScore,
			MaxScore:        p.finalScore,
			MaxPrice:        price,
			IncreasePercent: p.pct, // raw 24h % change
			IsAvailable:     true,
		})
	}

	// ---- cache ----
	c.cache.Set(cacheKey, coins, CacheTTLScore)

	log.Printf("✓ Local AI500: computed scores for %d USDT perps, returning top %d",
		len(pool), topN)
	return coins, nil
}

// GetTopRatedCoins returns the top N coin symbols sorted by composite AI500 score.
func (c *Client) GetTopRatedCoins(limit int) ([]string, error) {
	coins, err := c.GetAI500List()
	if err != nil {
		return nil, err
	}

	if limit > len(coins) {
		limit = len(coins)
	}
	if limit <= 0 {
		limit = 30
	}

	// Ensure coins are sorted (GetAI500List already sorts, but be safe)
	sort.SliceStable(coins, func(i, j int) bool {
		return coins[i].Score > coins[j].Score
	})

	symbols := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		symbols = append(symbols, normalizeSymbol(coins[i].Pair))
	}
	return symbols, nil
}

// GetAvailableCoins returns all coin symbols in the AI500 list.
func (c *Client) GetAvailableCoins() ([]string, error) {
	coins, err := c.GetAI500List()
	if err != nil {
		return nil, err
	}

	symbols := make([]string, 0, len(coins))
	for _, coin := range coins {
		if coin.IsAvailable {
			symbols = append(symbols, normalizeSymbol(coin.Pair))
		}
	}
	return symbols, nil
}

// parseFloat safely converts a string to float64; returns 0 on failure.
func parseFloat(s string) float64 {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
