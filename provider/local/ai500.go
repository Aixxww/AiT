package local

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
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
// computes a composite score emphasising volatility over raw volume,
// and returns the top 30 as []nofxos.CoinData.
//
// Algorithm (log-compressed, anti-wash-trading):
//
//	pctAbs_norm = (|change%| - min_abs) / (max_abs - min_abs) * 100
//	vol_norm    = (log10(qv)  - min_log) / (max_log - min_log) * 100
//	act_norm    = (log10(cnt) - min_log) / (max_log - min_log) * 100
//
// score = pctAbs_norm*0.50 + vol_norm*0.25 + act_norm*0.25
//
// Coins with trade count > 5M (wash-trading bots) receive a 0.6x penalty.
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
		log.Printf("⚠️  AI500: Binance ticker failed (%v), trying CoinGecko fallback", err)
		cgTickers, cgErr := c.fetchCoinGeckoDerivatives(nil, 50)
		if cgErr != nil {
			return nil, fmt.Errorf("AI500: both Binance and CoinGecko failed (binance: %v, coingecko: %v)", err, cgErr)
		}
		if len(cgTickers) == 0 {
			return nil, fmt.Errorf("AI500: CoinGecko returned 0 tickers")
		}
		tickers = cgTickers
		log.Printf("🔗 AI500: using CoinGecko fallback with %d tickers", len(tickers))
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

	// Collect raw values for log-based normalisation
	var pool []scoredTicker

	for _, t := range tickers {
		if !isUSDTPerp(t.Symbol) {
			continue
		}

		pct := parseFloat(t.PriceChangePercent)
		qv := parseFloat(t.QuoteVolume)
		cnt := float64(t.Count)

		st := scoredTicker{ticker: t, pct: pct, qv: qv, count: cnt}
		pool = append(pool, st)
	}

	if len(pool) == 0 {
		log.Printf("ℹ️  Local AI500: 0 USDT perps found")
		return []nofxos.CoinData{}, nil
	}

	// Compute log-space min/max for volume and activity
	var minQVLog, maxQVLog float64
	var minCntLog, maxCntLog float64
	var minPctAbs, maxPctAbs float64
	first := true

	for _, p := range pool {
		pctAbs := math.Abs(p.pct)
		qvLog := math.Log10(p.qv + 1)
		cntLog := math.Log10(p.count + 1)

		if first {
			minQVLog, maxQVLog = qvLog, qvLog
			minCntLog, maxCntLog = cntLog, cntLog
			minPctAbs, maxPctAbs = pctAbs, pctAbs
			first = false
		} else {
			if qvLog < minQVLog {
				minQVLog = qvLog
			}
			if qvLog > maxQVLog {
				maxQVLog = qvLog
			}
			if cntLog < minCntLog {
				minCntLog = cntLog
			}
			if cntLog > maxCntLog {
				maxCntLog = cntLog
			}
			if pctAbs < minPctAbs {
				minPctAbs = pctAbs
			}
			if pctAbs > maxPctAbs {
				maxPctAbs = pctAbs
			}
		}
	}

	pctAbsRange := maxPctAbs - minPctAbs
	qvLogRange := maxQVLog - minQVLog
	cntLogRange := maxCntLog - minCntLog
	if pctAbsRange == 0 {
		pctAbsRange = 1
	}
	if qvLogRange == 0 {
		qvLogRange = 1
	}
	if cntLogRange == 0 {
		cntLogRange = 1
	}

	// ---- compute scores ----
	// New formula (anti-wash-trading):
	//   |price_change| volatility  50%  — captures real directional moves
	//   log(volume)                25%  — log compresses outlier volumes
	//   log(activity)              25%  — log compresses outlier trade counts
	//   Wash-trading penalty: coins with count > 5M get 0.6x penalty
	for i := range pool {
		p := &pool[i]
		pctAbs := math.Abs(p.pct)
		qvLog := math.Log10(p.qv + 1)
		cntLog := math.Log10(p.count + 1)

		normPctAbs := ((pctAbs - minPctAbs) / pctAbsRange) * 100
		normQVLog := ((qvLog - minQVLog) / qvLogRange) * 100
		normCntLog := ((cntLog - minCntLog) / cntLogRange) * 100

		score := clamp(normPctAbs*0.50+normQVLog*0.25+normCntLog*0.25, 0, 100)

		// Wash-trading penalty: extremely high trade counts (e.g. 5M+ for LAB/BILL)
		// These are algorithmic market-making, not organic trading interest
		if p.count > 5_000_000 {
			score *= 0.60
		}

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

	// ---- signal bonus: detect RSI oversold, volume breakout, OI-price divergence ----
	c.applySignalBonus(coins)

	// Re-sort after signal bonus
	sort.SliceStable(coins, func(i, j int) bool {
		return coins[i].Score > coins[j].Score
	})

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

// applySignalBonus fetches 1h klines + OI history for each coin and applies
// signal-based score bonuses. Called after the initial scoring to boost coins
// that show actionable technical signals.
//
// Signals detected (each adds bonus points):
//   - RSI14 < 30 (oversold bounce setup): +15
//   - Volume breakout (latest 1h vol > 5-bar avg × 2): +10
//   - OI increase + price increase (bullish accumulation): +10
//   - Funding rate negative (shorts paying longs): +8 to +15
//   - Funding rate overheated (overleveraged longs): -10
//
// Parallelised: up to 5 concurrent API calls via semaphore.
func (c *Client) applySignalBonus(coins []nofxos.CoinData) {
	if len(coins) == 0 {
		return
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // max 5 concurrent API calls

	for i := range coins {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c.computeSignalBonus(&coins[idx])
		}(i)
	}
	wg.Wait()
}

// computeSignalBonus computes signal bonuses for a single coin.
// Safe for concurrent use (uses fetchJSONParallel which creates its own http.Client).
func (c *Client) computeSignalBonus(coin *nofxos.CoinData) {
	symbol := coin.Pair
	bonus := 0.0
	var tags []string

	// Fetch 1h klines (20 bars for RSI14 + volume avg)
	klines, err := c.fetchKlinesParallel(symbol, "1h", 20)
	if err != nil || len(klines) < 15 {
		return
	}

	// Signal 1: RSI14 oversold (< 30)
	rsi14 := computeRSI(klines, 14)
	if rsi14 > 0 && rsi14 < 30 {
		bonus += 15
		tags = append(tags, "rsi_oversold")
	}

	// Signal 2: Volume breakout
	if len(klines) >= 6 {
		latestVol := klines[len(klines)-1].Volume
		avgVol := 0.0
		for _, k := range klines[len(klines)-6 : len(klines)-1] {
			avgVol += k.Volume
		}
		avgVol /= 5
		if avgVol > 0 && latestVol > avgVol*2 {
			bonus += 10
			tags = append(tags, "vol_breakout")
		}
	}

	// Signal 3: OI increase + price increase
	oiDelta, err := c.fetchOIHistParallel(symbol, "1h", 4)
	if err == nil && oiDelta > 0 {
		priceStart := klines[len(klines)-4].Close
		priceEnd := klines[len(klines)-1].Close
		if priceStart > 0 && priceEnd > priceStart {
			bonus += 10
			tags = append(tags, "oi_up_price_up")
		}
	}

	// Signal 4: Funding rate signal
	// Negative funding = shorts paying longs = potential squeeze
	url := fmt.Sprintf("%s/fapi/v1/premiumIndex?symbol=%s", c.BinanceURL, symbol)
	var fundingResp struct {
		LastFundingRate string `json:"lastFundingRate"`
	}
	if err := c.fetchJSONParallel(url, &fundingResp); err == nil {
		fr := parseFloat(fundingResp.LastFundingRate)
		if fr < -0.0005 { // -0.05%: extreme bearish crowding
			bonus += 15
			tags = append(tags, "funding_negative")
		} else if fr < 0 { // negative: shorts paying
			bonus += 8
			tags = append(tags, "funding_bearish")
		} else if fr > 0.001 { // +0.1%: overleveraged longs
			bonus -= 10
			tags = append(tags, "funding_overheated")
		}
	}

	if bonus > 0 {
		coin.Score += bonus
		coin.SignalTags = tags
		log.Printf("🎯 %s signal bonus: +%.0f %v (new score: %.1f)", symbol, bonus, tags, coin.Score)
	}
}

// fetchJSONParallel is like fetchJSON but without rate limiting.
// Safe for concurrent use from multiple goroutines (each creates its own http.Client).
func (c *Client) fetchJSONParallel(url string, target interface{}) error {
	httpClient := &http.Client{Timeout: c.Timeout}
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("local: GET %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("local: read body %s failed: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("local: %s returned HTTP %d: %s", url, resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("local: JSON unmarshal failed for %s: %w", url, err)
	}
	return nil
}

// fetchKlinesParallel fetches klines using fetchJSONParallel (no rate limiting).
// Safe for concurrent use.
func (c *Client) fetchKlinesParallel(symbol, interval string, limit int) ([]klineBar, error) {
	url := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
		c.BinanceURL, symbol, interval, limit)

	var raw [][]interface{}
	if err := c.fetchJSONParallel(url, &raw); err != nil {
		return nil, err
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

// fetchOIHistParallel fetches OI history using fetchJSONParallel (no rate limiting).
// Safe for concurrent use.
func (c *Client) fetchOIHistParallel(symbol, period string, limit int) (float64, error) {
	url := fmt.Sprintf("%s/futures/data/openInterestHist?symbol=%s&period=%s&limit=%d",
		c.BinanceURL, symbol, period, limit)

	type oiEntry struct {
		SumOpenInterestValue string `json:"sumOpenInterestValue"`
	}

	var entries []oiEntry
	if err := c.fetchJSONParallel(url, &entries); err != nil {
		return 0, err
	}
	if len(entries) < 2 {
		return 0, nil
	}

	oldest := parseFloat(entries[0].SumOpenInterestValue)
	newest := parseFloat(entries[len(entries)-1].SumOpenInterestValue)
	if oldest > 0 {
		return (newest - oldest) / oldest * 100, nil
	}
	return 0, nil
}

// computeRSI calculates RSI for the given period using Wilder's smoothing method.
func computeRSI(klines []klineBar, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}

	var gains, losses float64
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses -= change // make positive
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// Wilder's smoothing for remaining bars
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
	}

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}
