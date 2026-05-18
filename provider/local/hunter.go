package local

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"nofx/provider/nofxos"
)

// HunterCoinScore holds intermediate scoring for one coin in the Hunter system.
type HunterCoinScore struct {
	Symbol          string
	PositionScore   float64 // Pillar S-A': position component [-35, 30]
	OISmartScore    float64 // Pillar S-A': OI change rate [0, 50]
	SmartMoneyScore float64 // Pillar S-B': LSR + Taker [0, 50]
	CooldownMod     float64 // Pillar C': multiplier (0.0 / 0.50 / 1.0)
	WashMod         float64 // Pillar D': multiplier (0.20–1.0)
	FinalScore      float64
	Ticker          binanceTicker
	Tags            []string
}

// computeATR computes Average True Range using Wilder's smoothing.
func computeATR(klines []klineBar, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}
	var trSum float64
	for i := 1; i <= period; i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close
		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		trSum += tr
	}
	atr := trSum / float64(period)
	for i := period + 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close
		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		atr = (atr*float64(period-1) + tr) / float64(period)
	}
	return atr
}

// computePositionScore uses 4h klines to assess support/resistance proximity.
// Near 4h support (within 1.5×ATR) → +25; near resistance → -15;
// already moved >50% in 24h → -20 (chase penalty).
func (c *Client) computePositionScore(symbol string, ticker binanceTicker) (float64, []string) {
	score := 0.0
	var tags []string

	klines4h, err := c.fetchKlines(symbol, "4h", 20)
	if err != nil || len(klines4h) < 15 {
		return 0, nil
	}

	atr := computeATR(klines4h, 14)
	if atr <= 0 {
		return 0, nil
	}

	currentPrice := klines4h[len(klines4h)-1].Close

	// Find 20-bar high/low
	high20 := klines4h[0].High
	low20 := klines4h[0].Low
	for _, k := range klines4h[1:] {
		if k.High > high20 {
			high20 = k.High
		}
		if k.Low < low20 {
			low20 = k.Low
		}
	}

	// Near support
	if currentPrice-low20 <= 1.5*atr {
		score += 25
		tags = append(tags, "near_support")
	}

	// Near resistance
	if high20-currentPrice <= 2*atr {
		score -= 15
		tags = append(tags, "near_resistance")
	}

	// Chase penalty: >50% move in 24h
	pct24h := math.Abs(parseFloat(ticker.PriceChangePercent))
	if pct24h > 50 {
		score -= 20
		tags = append(tags, "chase_penalty")
	}

	return clamp(score, -35, 30), tags
}

// computeOISmartScore replaces 15M hard threshold with OI change rate analysis.
func (c *Client) computeOISmartScore(symbol string, ticker binanceTicker) (float64, []string) {
	score := 0.0
	var tags []string

	// OI 4h delta
	oiDelta4h, err := c.fetchOIHist(symbol, "4h", 2)
	if err != nil {
		return 0, nil
	}

	// Check absolute OI value
	url := fmt.Sprintf("%s/fapi/v1/openInterest?symbol=%s", c.BinanceURL, symbol)
	var oiResp struct {
		OpenInterest string `json:"openInterest"`
	}
	if err := c.fetchJSON(url, &oiResp); err != nil {
		return 0, nil
	}
	currentOI := parseFloat(oiResp.OpenInterest)
	currentPrice := parseFloat(ticker.LastPrice)
	oiValue := currentOI * currentPrice

	// Apply cooldown-based OI threshold reduction
	threshold := 2_000_000.0 * globalCooldown.getOIThresholdReduction(symbol)

	if oiValue < threshold {
		globalCooldown.recordOIFilter(symbol)
		tags = append(tags, "oi_too_low")
		return 0, tags
	}

	priceDir := parseFloat(ticker.PriceChangePercent)

	if math.Abs(oiDelta4h) > 10 {
		if (priceDir > 0 && oiDelta4h > 0) || (priceDir < 0 && oiDelta4h < 0) {
			score += 40
			tags = append(tags, "oi_price_aligned")
		}
		if oiDelta4h > 0 && priceDir < 0 {
			score += 25
			tags = append(tags, "oi_accumulation")
		}
	} else if math.Abs(oiDelta4h) > 5 {
		score += 15
		tags = append(tags, "oi_moderate")
	}

	return clamp(score, 0, 50), tags
}

// computeSmartMoneyScore combines LSR signal and Taker buy/sell signal.
func (c *Client) computeSmartMoneyScore(symbol string, klines []klineBar) (float64, []string) {
	score := 0.0
	var tags []string

	// --- LSR Signal ---
	oldestRatio, newestRatio, err := c.fetchLSRHist(symbol, "1h", 4)
	if err == nil && oldestRatio > 0 {
		lsrDeltaPct := ((newestRatio - oldestRatio) / oldestRatio) * 100

		if oldestRatio < 0.9 && newestRatio > oldestRatio {
			score += 20
			tags = append(tags, "lsr_reversal")
		}
		if lsrDeltaPct > 10 {
			score += 10
			tags = append(tags, "lsr_bullish")
		}
		if lsrDeltaPct < -10 {
			score += 10
			tags = append(tags, "lsr_bearish")
		}
	}

	// --- Taker Signal ---
	if len(klines) >= 5 {
		latest := klines[len(klines)-1]
		if latest.Volume > 0 {
			takerBuyRatio := latest.TakerBuyBaseVolume / latest.Volume
			if takerBuyRatio > 0.60 {
				score += 10
				tags = append(tags, "taker_buy_strong")
			}
		}

		// TakerBuyRatio trending up over 4 bars
		var ratios []float64
		for _, k := range klines[len(klines)-4:] {
			if k.Volume > 0 {
				ratios = append(ratios, k.TakerBuyBaseVolume/k.Volume)
			}
		}
		if len(ratios) >= 3 && ratios[len(ratios)-1] > ratios[0] {
			score += 10
			tags = append(tags, "taker_trending_up")
		}
	}

	return clamp(score, 0, 50), tags
}

// computeWashMultiplier applies triple wash trade detection.
func (c *Client) computeWashMultiplier(symbol string, ticker binanceTicker, klines []klineBar, currentPrice float64) (float64, []string) {
	multiplier := 1.0
	var tags []string

	trades := ticker.Count
	qv := parseFloat(ticker.QuoteVolume)

	// Check 1: trades > 1M AND avg_trade_size < $5
	if trades > 1_000_000 && qv > 0 {
		avgTradeSize := qv / float64(trades)
		if avgTradeSize < 5 {
			multiplier *= 0.20
			tags = append(tags, "wash_micro_trades")
		}
	}

	// Check 2: OI/Volume ratio < 0.01
	url := fmt.Sprintf("%s/fapi/v1/openInterest?symbol=%s", c.BinanceURL, symbol)
	var oiResp struct {
		OpenInterest string `json:"openInterest"`
	}
	if err := c.fetchJSON(url, &oiResp); err == nil {
		oi := parseFloat(oiResp.OpenInterest) * currentPrice
		if qv > 0 && oi > 0 {
			oiVolRatio := oi / qv
			if oiVolRatio < 0.01 {
				multiplier *= 0.30
				tags = append(tags, "wash_fake_volume")
			}
		}
	}

	// Check 3: 3+ volume spikes >10x average in last 20 bars
	if len(klines) >= 20 {
		var avgVol float64
		for _, k := range klines[:15] {
			avgVol += k.Volume
		}
		avgVol /= 15
		spikes := 0
		if avgVol > 0 {
			for _, k := range klines[15:] {
				if k.Volume > avgVol*10 {
					spikes++
				}
			}
		}
		if spikes >= 3 {
			multiplier *= 0.30
			tags = append(tags, "wash_volume_spikes")
		}
	}

	return multiplier, tags
}

// GetHunterList fetches all USDT perps, computes 4-pillar hunter scores,
// and returns top 30 as []nofxos.CoinData.
func (c *Client) GetHunterList() ([]nofxos.CoinData, error) {
	const cacheKey = "hunter_list"
	if hit, ok := c.cache.Get(cacheKey); ok {
		return hit.([]nofxos.CoinData), nil
	}

	// --- Step 1: Try Binance primary source ---
	url := c.BinanceURL + "/fapi/v1/ticker/24hr"
	var tickers []binanceTicker
	var binanceErr error
	if err := c.fetchJSON(url, &tickers); err != nil {
		binanceErr = err
		log.Printf("⚠️  Hunter: Binance ticker failed (%v), falling back to CoinGecko", err)
	}

	if binanceErr != nil {
		// --- Step 2: Build valid Binance symbol pool ---
		// Try Binance exchangeInfo first (separate rate-limit budget)
		validSymbols, symErr := c.fetchBinanceUSDTPerpSymbols()
		if symErr != nil {
			log.Printf("⚠️  Hunter: Binance exchangeInfo also failed (%v), using CoinGecko symbol list", symErr)
			validSymbols = nil // fetchCoinGeckoDerivatives will accept all Binance CG entries
		}

		// --- Step 3: Fetch CoinGecko derivatives, filter to Binance perps ---
		cgTickers, cgErr := c.fetchCoinGeckoDerivatives(validSymbols, 50)
		if cgErr != nil {
			return nil, fmt.Errorf("hunter: both Binance and CoinGecko failed (binance: %v, coingecko: %v)", binanceErr, cgErr)
		}
		if len(cgTickers) == 0 {
			return nil, fmt.Errorf("hunter: CoinGecko returned 0 Binance USDT perp tickers")
		}
		tickers = cgTickers
	}

	// --- Build candidate pool (shared path) ---
	type candidate struct {
		ticker binanceTicker
		score  HunterCoinScore
	}
	var pool []candidate
	for _, t := range tickers {
		if !isUSDTPerp(t.Symbol) {
			continue
		}
		pool = append(pool, candidate{ticker: t})
	}
	sort.SliceStable(pool, func(i, j int) bool {
		return parseFloat(pool[i].ticker.QuoteVolume) > parseFloat(pool[j].ticker.QuoteVolume)
	})
	if len(pool) > 50 {
		pool = pool[:50]
	}

	for i := range pool {
		p := &pool[i]
		sym := p.ticker.Symbol
		p.score.Symbol = sym
		p.score.Ticker = p.ticker

		// Pillar S-A': Position + OI Smart (50%)
		posScore, posTags := c.computePositionScore(sym, p.ticker)
		oiScore, oiTags := c.computeOISmartScore(sym, p.ticker)
		p.score.PositionScore = posScore
		p.score.OISmartScore = oiScore
		baseScore50 := clamp((posScore+oiScore)/2, -35, 50)

		// Fetch 4h klines for reuse in S-B' and D'
		klines4h, _ := c.fetchKlines(sym, "4h", 20)

		// Pillar S-B': Smart Money (25%)
		smScore, smTags := c.computeSmartMoneyScore(sym, klines4h)
		p.score.SmartMoneyScore = smScore
		baseScore25 := smScore * 0.65

		composite := clamp(baseScore50+baseScore25, 0, 75)

		// Pillar C': Smart Cooldown
		p.score.CooldownMod = globalCooldown.getCooldownMultiplier(sym)
		if p.score.CooldownMod == 0 {
			composite = 0
		}

		// Pillar D': Wash Trade
		currentPrice := parseFloat(p.ticker.LastPrice)
		washMod, washTags := c.computeWashMultiplier(sym, p.ticker, klines4h, currentPrice)
		p.score.WashMod = washMod

		p.score.FinalScore = composite * p.score.CooldownMod * p.score.WashMod
		p.score.Tags = append(append(append(posTags, oiTags...), smTags...), washTags...)
	}

	// Filter out zero scores
	var filtered []candidate
	for _, p := range pool {
		if p.score.FinalScore > 0 {
			filtered = append(filtered, p)
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].score.FinalScore > filtered[j].score.FinalScore
	})

	topN := 30
	if len(filtered) < topN {
		topN = len(filtered)
	}
	now := time.Now().Unix()
	coins := make([]nofxos.CoinData, 0, topN)
	for i := 0; i < topN; i++ {
		p := filtered[i]
		price := parseFloat(p.ticker.LastPrice)
		coins = append(coins, nofxos.CoinData{
			Pair:            p.ticker.Symbol,
			Score:           p.score.FinalScore,
			StartTime:       now,
			StartPrice:      price,
			LastScore:       p.score.FinalScore,
			MaxScore:        p.score.FinalScore,
			MaxPrice:        price,
			IncreasePercent: parseFloat(p.ticker.PriceChangePercent),
			IsAvailable:     true,
			SignalTags:      p.score.Tags,
		})
	}

	c.cache.Set(cacheKey, coins, CacheTTLHunter)
	source := "Binance"
	if binanceErr != nil {
		source = "CoinGecko fallback"
	}
	log.Printf("🎯 Hunter (%s): scored %d perps, returning top %d", source, len(pool), topN)
	return coins, nil
}

// GetHunterTopRatedCoins returns top N symbols from Hunter scoring.
func (c *Client) GetHunterTopRatedCoins(limit int) ([]string, error) {
	coins, err := c.GetHunterList()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > len(coins) {
		limit = len(coins)
	}
	symbols := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		symbols = append(symbols, normalizeSymbol(coins[i].Pair))
	}
	return symbols, nil
}
