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
	PositionScore   float64 // Pillar S-A': position component [-35, 55]
	OISmartScore    float64 // Pillar S-A': OI change rate [0, 50]
	SmartMoneyScore float64 // Pillar S-B': LSR + Taker [0, 65]
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

// findHighLow scans a kline slice and returns the highest high and lowest low.
func findHighLow(klines []klineBar) (high, low float64) {
	high = klines[0].High
	low = klines[0].Low
	for _, k := range klines[1:] {
		if k.High > high {
			high = k.High
		}
		if k.Low < low {
			low = k.Low
		}
	}
	return
}

// computePositionScore uses 4h/1d/1h klines to assess support/resistance proximity.
// Near 4h support (within 1.5×ATR) → +25; near resistance → -15;
// near 1d support (within 2×ATR) → +15; near 1h support (within 1×ATR) → +10;
// already moved >50% in 24h → -20 (chase penalty).
func (c *Client) computePositionScore(symbol string, ticker binanceTicker) (float64, []string) {
	score := 0.0
	var tags []string

	klines4h, err := c.fetchKlines(symbol, "4h", 20)
	if err != nil || len(klines4h) < 15 {
		return 0, nil
	}

	atr4h := computeATR(klines4h, 14)
	if atr4h <= 0 {
		return 0, nil
	}

	currentPrice := klines4h[len(klines4h)-1].Close

	// --- 4h support/resistance (existing) ---
	high4h, low4h := findHighLow(klines4h)

	if currentPrice-low4h <= 1.5*atr4h {
		score += 25
		tags = append(tags, "near_support_4h")
	}
	if high4h-currentPrice <= 2*atr4h {
		score -= 15
		tags = append(tags, "near_resistance_4h")
	}

	// --- 1d support (wider threshold: 2×ATR derived from daily bars) ---
	klines1d, err1d := c.fetchKlines(symbol, "1d", 20)
	if err1d == nil && len(klines1d) >= 15 {
		atr1d := computeATR(klines1d, 14)
		if atr1d > 0 {
			_, low1d := findHighLow(klines1d)
			if currentPrice-low1d <= 2*atr1d {
				score += 15
				tags = append(tags, "near_support_1d")
			}
		}
	}

	// --- 1h support (tighter threshold: 1×ATR) ---
	klines1h, err1h := c.fetchKlines(symbol, "1h", 20)
	if err1h == nil && len(klines1h) >= 15 {
		atr1h := computeATR(klines1h, 14)
		if atr1h > 0 {
			_, low1h := findHighLow(klines1h)
			if currentPrice-low1h <= 1*atr1h {
				score += 10
				tags = append(tags, "near_support_1h")
			}
		}
	}

	// Chase penalty: >50% move in 24h
	pct24h := math.Abs(parseFloat(ticker.PriceChangePercent))
	if pct24h > 50 {
		score -= 20
		tags = append(tags, "chase_penalty")
	}

	return clamp(score, -35, 55), tags
}

// fetchCurrentOI fetches the current open interest for a symbol and multiplies
// by price to return the notional OI value in USDT.
func (c *Client) fetchCurrentOI(symbol string, price float64) (float64, error) {
	url := fmt.Sprintf("%s/fapi/v1/openInterest?symbol=%s", c.BinanceURL, symbol)
	var oiResp struct {
		OpenInterest string `json:"openInterest"`
	}
	if err := c.fetchJSON(url, &oiResp); err != nil {
		return 0, err
	}
	return parseFloat(oiResp.OpenInterest) * price, nil
}

// computeOISmartScore replaces 15M hard threshold with OI change rate analysis.
func (c *Client) computeOISmartScore(symbol string, ticker binanceTicker, currentOIValue float64) (float64, []string) {
	score := 0.0
	var tags []string

	// OI 4h delta
	oiDelta4h, err := c.fetchOIHist(symbol, "4h", 2)
	if err != nil {
		return 0, nil
	}

	oiValue := currentOIValue

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
			score += 40 // v3: increased from 25 — Round2 data shows 56.3% win rate, strongest signal
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

	// --- LSR Signal (12 bars for better trend detection) ---
	oldestRatio, newestRatio, err := c.fetchLSRHist(symbol, "1h", 12)
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

		// Extreme bearish sentiment (crowded short) — squeeze potential
		if newestRatio > 2.0 {
			score += 15
			tags = append(tags, "lsr_extreme_bearish")
		}

		// Extreme bullish sentiment (crowded long) — dump risk, penalize
		if newestRatio < 0.5 {
			score -= 10
			tags = append(tags, "lsr_extreme_bullish")
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

		// Consecutive strong buying (3+ bars > 55%)
		if len(ratios) >= 3 {
			strongBars := 0
			for _, r := range ratios {
				if r > 0.55 {
					strongBars++
				}
			}
			if strongBars >= 3 {
				score += 20 // v3: increased from 15 — aligned with Go taker_strong_bonus
				tags = append(tags, "taker_sustained_buying")
			}
		}

		// Taker reversal (was selling, now buying)
		if len(ratios) >= 4 && ratios[0] < 0.45 && ratios[len(ratios)-1] > 0.55 {
			score += 10
			tags = append(tags, "taker_reversal")
		}
	}

	return clamp(score, 0, 65), tags
}

// computeWashMultiplier applies triple wash trade detection.
func (c *Client) computeWashMultiplier(symbol string, ticker binanceTicker, klines []klineBar, currentPrice float64, currentOIValue float64) (float64, []string) {
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
	if qv > 0 && currentOIValue > 0 {
		oiVolRatio := currentOIValue / qv
		if oiVolRatio < 0.01 {
			multiplier *= 0.30
			tags = append(tags, "wash_fake_volume")
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

	usingFallback := binanceErr != nil

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
		currentPrice := parseFloat(p.ticker.LastPrice)

		// Fetch current OI once (shared by OI score and wash detection)
		currentOI, _ := c.fetchCurrentOI(sym, currentPrice)

		if usingFallback {
			// CoinGecko fallback: only compute OI Smart Score; skip kline-dependent pillars
			oiScore, oiTags := c.computeOISmartScore(sym, p.ticker, currentOI)
			p.score.OISmartScore = oiScore
			// Use QuoteVolume from CoinGecko as the primary ranking signal
			qv := parseFloat(p.ticker.QuoteVolume)
			baseScore := clamp(oiScore+math.Log10(math.Max(qv, 1))*2, 0, 75)
			composite := baseScore

			p.score.CooldownMod = globalCooldown.getCooldownMultiplier(sym)
			if p.score.CooldownMod == 0 {
				composite = 0
			}
			p.score.WashMod = 1.0 // no klines available for wash detection

			p.score.FinalScore = composite * p.score.CooldownMod
			p.score.Tags = oiTags
		} else {
			// Full Binance path: compute all 4 pillars
			// Pillar S-A': Position + OI Smart (50%)
			posScore, posTags := c.computePositionScore(sym, p.ticker)
			oiScore, oiTags := c.computeOISmartScore(sym, p.ticker, currentOI)
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

			// v3 Signal Confirmation Filter: near_support without confirming signals → 0.5x penalty
			// Round 2 data: near_support alone = 37.9% win rate / -3.03% avg return
			// near_support + OI/LSR confirmation = ~72.9% win rate
			allTags := append(append(append([]string{}, posTags...), oiTags...), smTags...)
			hasNearSupport := false
			for _, t := range allTags {
				if t == "near_support_4h" || t == "near_support_1d" || t == "near_support_1h" {
					hasNearSupport = true
					break
				}
			}
			if hasNearSupport {
				confirmingSignals := map[string]bool{
					"oi_accumulation": true, "oi_price_aligned": true, "oi_moderate": true,
					"lsr_reversal": true, "lsr_bullish": true,
					"taker_buy_strong": true, "taker_sustained_buying": true,
				}
				hasConfirmation := false
				for _, t := range allTags {
					if confirmingSignals[t] {
						hasConfirmation = true
						break
					}
				}
				if !hasConfirmation {
					composite *= 0.5
				}
			}

			// Pillar C': Smart Cooldown
			p.score.CooldownMod = globalCooldown.getCooldownMultiplier(sym)
			if p.score.CooldownMod == 0 {
				composite = 0
			}

			// Pillar D': Wash Trade
			washMod, washTags := c.computeWashMultiplier(sym, p.ticker, klines4h, currentPrice, currentOI)
			p.score.WashMod = washMod

			p.score.FinalScore = composite * p.score.CooldownMod * p.score.WashMod
			p.score.Tags = append(append(append(posTags, oiTags...), smTags...), washTags...)
		}
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
