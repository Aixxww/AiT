package local

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"nofx/market"
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

	// Short-direction scoring (mirror of long scoring)
	ShortPositionScore   float64 // Short position score [-35, 55]
	ShortOISmartScore    float64 // Short OI score [0, 50]
	ShortSmartMoneyScore float64 // Short smart money score [0, 65]
	ShortFinalScore      float64
	ShortTags            []string

	// Saved LONG scores (before direction picking overwrites FinalScore)
	LongFinalScore float64
	LongTags       []string

	Direction string // "LONG" or "SHORT"
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

// computeBBWidth calculates Bollinger Band Width as a percentage of the middle band.
// Returns (widthPct, upper, middle, lower). widthPct = (upper-lower)/middle * 100.
// Adapted from market/data_indicators.go:calculateBOLL() for klineBar.
func computeBBWidth(klines []klineBar, period int, multiplier float64) (widthPct, upper, middle, lower float64) {
	if len(klines) < period {
		return 0, 0, 0, 0
	}
	sum := 0.0
	start := len(klines) - period
	for i := start; i < len(klines); i++ {
		sum += klines[i].Close
	}
	middle = sum / float64(period)
	if middle == 0 {
		return 0, 0, 0, 0
	}
	variance := 0.0
	for i := start; i < len(klines); i++ {
		diff := klines[i].Close - middle
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(period))
	upper = middle + multiplier*stdDev
	lower = middle - multiplier*stdDev
	widthPct = (upper - lower) / middle * 100
	return widthPct, upper, middle, lower
}

// computeBBWidthSqueeze detects if current BB Width is at a rolling low (squeeze).
// Uses 15m klines (50 bars = 12.5h) for 24h context, falls back to 5m (50 bars = ~4h).
// Returns: (score [0-15], tags).
func (c *Client) computeBBWidthSqueeze(symbol string) (float64, []string) {
	// Try 15m first (50 bars = 12.5h, better for 24h context)
	klines15m, err := c.fetchKlines(symbol, "15m", 50)
	if err == nil && len(klines15m) >= 25 {
		widthPct, _, _, _ := computeBBWidth(klines15m, 20, 2.0)
		if widthPct > 0 {
			minWidth := math.MaxFloat64
			for i := 20; i <= len(klines15m); i++ {
				w, _, _, _ := computeBBWidth(klines15m[:i], 20, 2.0)
				if w > 0 && w < minWidth {
					minWidth = w
				}
			}
			// Current width within 10% of rolling minimum → squeeze
			if minWidth > 0 && widthPct <= minWidth*1.10 {
				return 15, []string{"bb_squeeze_15m"}
			}
		}
	}
	// Fallback: 5m klines (50 bars = ~4.2h)
	klines5m, err := c.fetchKlines(symbol, "5m", 50)
	if err == nil && len(klines5m) >= 25 {
		widthPct, _, _, _ := computeBBWidth(klines5m, 20, 2.0)
		if widthPct > 0 {
			minWidth := math.MaxFloat64
			for i := 20; i <= len(klines5m); i++ {
				w, _, _, _ := computeBBWidth(klines5m[:i], 20, 2.0)
				if w > 0 && w < minWidth {
					minWidth = w
				}
			}
			if minWidth > 0 && widthPct <= minWidth*1.10 {
				return 10, []string{"bb_squeeze_5m"}
			}
		}
	}
	return 0, nil
}

// computeOISpike detects abnormal OI increase in the last 1 hour.
// Uses Binance openInterestHist with 1h period, 13 data points (12 hours).
// Computes mean and stddev of 1h OI changes, flags if latest change > 2σ AND > 3%.
// Returns: (score [0-15], tags).
func (c *Client) computeOISpike(symbol string) (float64, []string) {
	url := fmt.Sprintf("%s/futures/data/openInterestHist?symbol=%s&period=1h&limit=13",
		c.BinanceURL, symbol)

	type oiEntry struct {
		SumOpenInterestValue string `json:"sumOpenInterestValue"`
		Timestamp            int64  `json:"timestamp"`
	}

	var entries []oiEntry
	if err := c.fetchJSON(url, &entries); err != nil || len(entries) < 4 {
		return 0, nil
	}

	// Compute period-over-period % changes
	var changes []float64
	for i := 1; i < len(entries); i++ {
		prev := parseFloat(entries[i-1].SumOpenInterestValue)
		curr := parseFloat(entries[i].SumOpenInterestValue)
		if prev > 0 {
			changes = append(changes, (curr-prev)/prev*100)
		}
	}
	if len(changes) < 3 {
		return 0, nil
	}

	latestChange := changes[len(changes)-1]
	// Compute mean and stddev of historical changes (excluding latest)
	histChanges := changes[:len(changes)-1]
	mean := 0.0
	for _, ch := range histChanges {
		mean += ch
	}
	mean /= float64(len(histChanges))
	variance := 0.0
	for _, ch := range histChanges {
		d := ch - mean
		variance += d * d
	}
	stddev := math.Sqrt(variance / float64(len(histChanges)))

	// Spike detection: latest change > mean + 2σ AND absolute change > 3%
	if stddev > 0 && latestChange > mean+2*stddev && latestChange > 3.0 {
		return 15, []string{"oi_spike_1h"}
	} else if latestChange > 5.0 {
		// Fallback: absolute threshold (>5% in 1h is notable)
		return 8, []string{"oi_surge_1h"}
	}
	return 0, nil
}

// computeSqueezeExplosionPillar combines BB Width squeeze and OI spike into Pillar E'.
// Synergy bonus: if both signals fire, extra +5 points.
// Returns: (score [0-25], tags).
func (c *Client) computeSqueezeExplosionPillar(symbol string) (float64, []string) {
	bbScore, bbTags := c.computeBBWidthSqueeze(symbol)
	oiScore, oiTags := c.computeOISpike(symbol)

	totalScore := bbScore + oiScore
	allTags := append(append([]string{}, bbTags...), oiTags...)

	// Synergy bonus: both squeeze + OI spike present
	if bbScore > 0 && oiScore > 0 {
		totalScore += 5
		allTags = append(allTags, "squeeze_explosion_synergy")
	}

	if totalScore > 25 {
		totalScore = 25
	}
	return totalScore, allTags
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
		score -= 25 // v6: -15→-25, R5验证75%方向正确
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

	// v7: --- 15m support (short-term swing, 1×ATR) ---
	klines15m, err15m := c.fetchKlines(symbol, "15m", 20)
	if err15m == nil && len(klines15m) >= 15 {
		atr15m := computeATR(klines15m, 14)
		if atr15m > 0 {
			_, low15m := findHighLow(klines15m)
			if currentPrice-low15m <= 1*atr15m {
				score += 8
				tags = append(tags, "near_support_15m")
			}
		}
	}

	// v7: --- 5m support (micro swing / entry timing, 1×ATR) ---
	klines5m, err5m := c.fetchKlines(symbol, "5m", 20)
	if err5m == nil && len(klines5m) >= 15 {
		atr5m := computeATR(klines5m, 14)
		if atr5m > 0 {
			_, low5m := findHighLow(klines5m)
			if currentPrice-low5m <= 1*atr5m {
				score += 5
				tags = append(tags, "near_support_5m")
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

	// v6: OI Short Squeeze 检测 (OI↓ + 价格↑ = 空头清算)
	if oiDelta4h < -10 && priceDir > 0 {
		score += 45
		tags = append(tags, "oi_short_squeeze")
	} else if oiDelta4h < -5 && priceDir > 0 {
		score += 20
		tags = append(tags, "oi_squeeze_moderate")
	} else if math.Abs(oiDelta4h) > 10 {
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

		// Extreme bullish sentiment (crowded longs) — momentum follow for LONG
		// LSR > 2.0 = top traders 67%+ long = crowd consensus, favor long
		if newestRatio > 2.0 {
			score += 15
			tags = append(tags, "lsr_crowded_long")
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

// computeShortPositionScore mirrors computePositionScore for SHORT direction.
// Near 4h resistance (within 2×ATR) → +25; near support → -25;
// near 1d resistance → +15; near 1h resistance → +10;
// near 15m resistance → +8; near 5m resistance → +5 (swing entry timing);
// chase penalty same as long.
func (c *Client) computeShortPositionScore(symbol string, ticker binanceTicker) (float64, []string) {
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

	// --- 4h resistance/support (mirrored) ---
	high4h, low4h := findHighLow(klines4h)

	if high4h-currentPrice <= 2*atr4h {
		score += 25 // Near resistance = short opportunity
		tags = append(tags, "near_resistance_4h")
	}
	if currentPrice-low4h <= 1.5*atr4h {
		score -= 25 // Near support = risk for short (mirror penalty)
		tags = append(tags, "near_support_4h_penalize")
	}

	// --- 1d resistance ---
	klines1d, err1d := c.fetchKlines(symbol, "1d", 20)
	if err1d == nil && len(klines1d) >= 15 {
		atr1d := computeATR(klines1d, 14)
		if atr1d > 0 {
			high1d, _ := findHighLow(klines1d)
			if high1d-currentPrice <= 2*atr1d {
				score += 15
				tags = append(tags, "near_resistance_1d")
			}
		}
	}

	// --- 1h resistance ---
	klines1h, err1h := c.fetchKlines(symbol, "1h", 20)
	if err1h == nil && len(klines1h) >= 15 {
		atr1h := computeATR(klines1h, 14)
		if atr1h > 0 {
			high1h, _ := findHighLow(klines1h)
			if high1h-currentPrice <= 1*atr1h {
				score += 10
				tags = append(tags, "near_resistance_1h")
			}
		}
	}

	// --- 15m resistance (short-term swing) ---
	klines15m, err15m := c.fetchKlines(symbol, "15m", 20)
	if err15m == nil && len(klines15m) >= 15 {
		atr15m := computeATR(klines15m, 14)
		if atr15m > 0 {
			high15m, _ := findHighLow(klines15m)
			if high15m-currentPrice <= 1*atr15m {
				score += 8
				tags = append(tags, "near_resistance_15m")
			}
		}
	}

	// --- 5m resistance (micro swing / entry timing) ---
	klines5m, err5m := c.fetchKlines(symbol, "5m", 20)
	if err5m == nil && len(klines5m) >= 15 {
		atr5m := computeATR(klines5m, 14)
		if atr5m > 0 {
			high5m, _ := findHighLow(klines5m)
			if high5m-currentPrice <= 1*atr5m {
				score += 5
				tags = append(tags, "near_resistance_5m")
			}
		}
	}

	// Chase penalty: >50% move in 24h (same as long)
	pct24h := math.Abs(parseFloat(ticker.PriceChangePercent))
	if pct24h > 50 {
		score -= 20
		tags = append(tags, "chase_penalty")
	}

	return clamp(score, -35, 55), tags
}

// computeShortOISmartScore mirrors computeOISmartScore for SHORT direction.
// OI↑ + Price↑ = smart money building shorts (distribution) → +40
// OI↓ + Price↓ = long liquidation cascade → +45
func (c *Client) computeShortOISmartScore(symbol string, ticker binanceTicker, currentOIValue float64) (float64, []string) {
	score := 0.0
	var tags []string

	oiDelta4h, err := c.fetchOIHist(symbol, "4h", 2)
	if err != nil {
		return 0, nil
	}

	oiValue := currentOIValue
	// v7: 做空侧独立OI门槛 $1.5M（低于做多$2M，捕捉更多短周期机会）
	threshold := 1_500_000.0 * globalCooldown.getOIThresholdReduction(symbol)

	if oiValue < threshold {
		globalCooldown.recordOIFilter(symbol)
		tags = append(tags, "oi_too_low")
		return 0, tags
	}

	priceDir := parseFloat(ticker.PriceChangePercent)

	// OI Long Squeeze: OI↓ + Price↓ = long liquidation cascade
	if oiDelta4h < -10 && priceDir < 0 {
		score += 45
		tags = append(tags, "oi_long_squeeze")
	} else if oiDelta4h < -5 && priceDir < 0 {
		score += 20
		tags = append(tags, "oi_long_squeeze_moderate")
	} else if math.Abs(oiDelta4h) > 10 {
		// Price-aligned: OI↓+Price↑ or OI↑+Price↓ = same direction move
		if (priceDir < 0 && oiDelta4h < 0) || (priceDir > 0 && oiDelta4h > 0) {
			score += 40
			tags = append(tags, "oi_price_aligned_short")
		}
		// Distribution: OI↑ + Price↑ = smart money building short positions
		if oiDelta4h > 0 && priceDir > 0 {
			score += 40
			tags = append(tags, "oi_distribution")
		}
	} else if math.Abs(oiDelta4h) > 5 {
		score += 15
		tags = append(tags, "oi_moderate_short")
	}

	return clamp(score, 0, 50), tags
}

// computeShortSmartMoneyScore mirrors computeSmartMoneyScore for SHORT direction.
func (c *Client) computeShortSmartMoneyScore(symbol string, klines []klineBar) (float64, []string) {
	score := 0.0
	var tags []string

	// --- LSR Signal (mirrored) ---
	oldestRatio, newestRatio, err := c.fetchLSRHist(symbol, "1h", 12)
	if err == nil && oldestRatio > 0 {
		lsrDeltaPct := ((newestRatio - oldestRatio) / oldestRatio) * 100

		// Bearish reversal: was bullish (>1.1), now falling
		if oldestRatio > 1.1 && newestRatio < oldestRatio {
			score += 20
			tags = append(tags, "lsr_bearish_reversal")
		}
		// Strong bearish momentum
		if lsrDeltaPct < -10 {
			score += 10
			tags = append(tags, "lsr_bearish_strong")
		}
		// Bullish momentum (opposing short — weak signal)
		if lsrDeltaPct > 10 {
			score += 5
			tags = append(tags, "lsr_bullish_weak")
		}

		// Extreme bullish (crowded longs) — dump risk, favor SHORT
		if newestRatio < 0.5 {
			score += 15
			tags = append(tags, "lsr_extreme_bullish_short")
		}

		// Extreme bullish (crowded longs) — squeeze/dump potential for SHORT
		// LSR > 2.0 = top traders 67%+ long = favor short positions
		if newestRatio > 2.0 {
			score += 15
			tags = append(tags, "lsr_crowded_long_favor_short")
		}
	}

	// --- Taker Signal (mirrored) ---
	if len(klines) >= 5 {
		latest := klines[len(klines)-1]
		if latest.Volume > 0 {
			takerBuyRatio := latest.TakerBuyBaseVolume / latest.Volume
			// Strong selling (taker buy < 40%)
			if takerBuyRatio < 0.40 {
				score += 10
				tags = append(tags, "taker_sell_strong")
			}
		}

		// TakerBuyRatio trending DOWN over 4 bars
		var ratios []float64
		for _, k := range klines[len(klines)-4:] {
			if k.Volume > 0 {
				ratios = append(ratios, k.TakerBuyBaseVolume/k.Volume)
			}
		}
		if len(ratios) >= 3 && ratios[len(ratios)-1] < ratios[0] {
			score += 10
			tags = append(tags, "taker_trending_down")
		}

		// Consecutive strong selling (3+ bars < 45% taker buy)
		if len(ratios) >= 3 {
			strongBars := 0
			for _, r := range ratios {
				if r < 0.45 {
					strongBars++
				}
			}
			if strongBars >= 3 {
				score += 20
				tags = append(tags, "taker_sustained_selling")
			}
		}

		// Taker reversal for short (was buying >0.55, now selling <0.45)
		if len(ratios) >= 4 && ratios[0] > 0.55 && ratios[len(ratios)-1] < 0.45 {
			score += 10
			tags = append(tags, "taker_reversal_short")
		}
	}

	return clamp(score, 0, 65), tags
}

// GetHunterList fetches all USDT perps, computes 4-pillar hunter scores,
// and returns top 30 as []nofxos.CoinData.
func (c *Client) GetHunterList() ([]nofxos.CoinData, error) {
	const cacheKey = "hunter_list_v2" // v2: includes short-direction scoring
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
		if !isUSDTPerp(t.Symbol) || excludedTokenizedAssets[t.Symbol] {
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
			qv := parseFloat(p.ticker.QuoteVolume)
			baseScore := clamp(oiScore+math.Log10(math.Max(qv, 1))*2, 0, 75)
			composite := baseScore

			// Short OI scoring (CoinGecko fallback)
			shortOIScore, shortOITags := c.computeShortOISmartScore(sym, p.ticker, currentOI)
			p.score.ShortOISmartScore = shortOIScore
			shortBaseScore := clamp(shortOIScore+math.Log10(math.Max(qv, 1))*2, 0, 75)
			shortComposite := shortBaseScore

			p.score.CooldownMod = globalCooldown.getCooldownMultiplier(sym)
			if p.score.CooldownMod == 0 {
				composite = 0
				shortComposite = 0
			}
			p.score.WashMod = 1.0 // no klines available for wash detection

			p.score.FinalScore = composite * p.score.CooldownMod
			p.score.ShortFinalScore = shortComposite * p.score.CooldownMod
			p.score.Tags = oiTags
			p.score.ShortTags = shortOITags

			// Pick dominant direction
			if p.score.ShortFinalScore > p.score.FinalScore {
				p.score.FinalScore = p.score.ShortFinalScore
				p.score.Tags = p.score.ShortTags
				p.score.Direction = "SHORT"
			} else {
				p.score.Direction = "LONG"
			}
		} else {
			// Full Binance path: compute all 4 pillars for BOTH directions

			// Fetch shared data
			klines4h, _ := c.fetchKlines(sym, "4h", 20)

			// ===== LONG DIRECTION =====
			posScore, posTags := c.computePositionScore(sym, p.ticker)
			oiScore, oiTags := c.computeOISmartScore(sym, p.ticker, currentOI)
			p.score.PositionScore = posScore
			p.score.OISmartScore = oiScore
			baseScore50 := clamp((posScore+oiScore)/2, -35, 50)

			smScore, smTags := c.computeSmartMoneyScore(sym, klines4h)
			p.score.SmartMoneyScore = smScore
			baseScore25 := smScore * 0.65

			composite := clamp(baseScore50+baseScore25, 0, 75)

			// Long Signal Confirmation Filter
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

			// Long ELP
			pct24h := parseFloat(p.ticker.PriceChangePercent)
			loss24h := -pct24h
			var longELPTags []string
			if loss24h > 20.0 {
				composite *= 0.10
				longELPTags = append(longELPTags, "elp_hard_kill")
			} else if loss24h > 10.0 && currentOI < 5_000_000 {
				composite *= 0.30
				longELPTags = append(longELPTags, "elp_severe")
			} else if loss24h > 10.0 && currentOI < 20_000_000 {
				composite *= 0.50
				longELPTags = append(longELPTags, "elp_moderate")
			}

			// Pillar C/D (shared)
			p.score.CooldownMod = globalCooldown.getCooldownMultiplier(sym)
			if p.score.CooldownMod == 0 {
				composite = 0
			}
			washMod, washTags := c.computeWashMultiplier(sym, p.ticker, klines4h, currentPrice, currentOI)
			p.score.WashMod = washMod

			p.score.FinalScore = composite * p.score.CooldownMod * p.score.WashMod
			p.score.Tags = append(append(append(append(posTags, oiTags...), smTags...), longELPTags...), washTags...)

			// ===== SHORT DIRECTION =====
			shortPosScore, shortPosTags := c.computeShortPositionScore(sym, p.ticker)
			shortOIScore, shortOITags := c.computeShortOISmartScore(sym, p.ticker, currentOI)
			p.score.ShortPositionScore = shortPosScore
			p.score.ShortOISmartScore = shortOIScore
			shortBase50 := clamp((shortPosScore+shortOIScore)/2, -35, 50)

			shortSMScore, shortSMTags := c.computeShortSmartMoneyScore(sym, klines4h)
			p.score.ShortSmartMoneyScore = shortSMScore
			shortBase25 := shortSMScore * 0.65

			shortComposite := clamp(shortBase50+shortBase25, 0, 75)

			// Short Signal Confirmation Filter: near_resistance without confirmation → 0.5x
			allShortTags := append(append(append([]string{}, shortPosTags...), shortOITags...), shortSMTags...)
			hasNearResistance := false
			for _, t := range allShortTags {
				if t == "near_resistance_4h" || t == "near_resistance_1d" || t == "near_resistance_1h" {
					hasNearResistance = true
					break
				}
			}
			if hasNearResistance {
				shortConfirming := map[string]bool{
					"oi_distribution": true, "oi_price_aligned_short": true, "oi_moderate_short": true,
					"lsr_bearish_reversal": true, "lsr_bearish_strong": true,
					"taker_sell_strong": true, "taker_sustained_selling": true,
				}
				hasShortConfirmation := false
				for _, t := range allShortTags {
					if shortConfirming[t] {
						hasShortConfirmation = true
						break
					}
				}
				if !hasShortConfirmation {
					shortComposite *= 0.5
				}
			}

			// Short ELP: v7 — gain>15%无条件0.30x（更敏感，无需OI条件）
			var shortELPTags []string
			if pct24h > 20.0 {
				shortComposite *= 0.10
				shortELPTags = append(shortELPTags, "elp_short_hard_kill")
			} else if pct24h > 15.0 {
				shortComposite *= 0.30
				shortELPTags = append(shortELPTags, "elp_short_severe")
			} else if pct24h > 10.0 && currentOI < 20_000_000 {
				shortComposite *= 0.50
				shortELPTags = append(shortELPTags, "elp_short_moderate")
			}

			p.score.ShortFinalScore = shortComposite * p.score.CooldownMod * p.score.WashMod
			p.score.ShortTags = append(append(append(append(shortPosTags, shortOITags...), shortSMTags...), shortELPTags...), washTags...)

			// ===== PILLAR E': Squeeze & Explosion Potential (direction-agnostic) =====
			squeezeScore, squeezeTags := c.computeSqueezeExplosionPillar(sym)
			p.score.FinalScore += squeezeScore
			p.score.Tags = append(p.score.Tags, squeezeTags...)
			p.score.ShortFinalScore += squeezeScore
			p.score.ShortTags = append(p.score.ShortTags, squeezeTags...)

			// ===== SAVE BIDIRECTIONAL SCORES BEFORE PICKING =====
			p.score.LongFinalScore = p.score.FinalScore
			p.score.LongTags = append([]string{}, p.score.Tags...) // deep copy

			// ===== PICK DOMINANT DIRECTION =====
			if p.score.ShortFinalScore > p.score.FinalScore {
				p.score.FinalScore = p.score.ShortFinalScore
				p.score.Tags = p.score.ShortTags
				p.score.Direction = "SHORT"
			} else {
				p.score.Direction = "LONG"
			}
		}
	}

	// Filter out zero scores
	var filtered []candidate
	for _, p := range pool {
		if p.score.FinalScore > 0 {
			filtered = append(filtered, p)
		}
	}

	// Hard floor: below this threshold, coin doesn't qualify for AI analysis
	const minHunterScore = 15.0
	var thresholdFiltered []candidate
	for _, p := range filtered {
		if p.score.FinalScore >= minHunterScore {
			thresholdFiltered = append(thresholdFiltered, p)
		} else {
			log.Printf("🔽 Hunter: %s score %.1f < %.1f threshold, dropped", p.score.Symbol, p.score.FinalScore, minHunterScore)
		}
	}
	filtered = thresholdFiltered

	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].score.FinalScore > filtered[j].score.FinalScore
	})

	// v6: 宁缺勿滥门控 — Top-10 中强信号标的不足则观望
	// Long strong signals
	longStrongSignals := map[string]bool{
		"oi_accumulation": true, "oi_price_aligned": true, "oi_moderate": true,
		"oi_short_squeeze": true, "oi_squeeze_moderate": true,
		"lsr_reversal": true, "lsr_bullish": true,
		"bb_squeeze_15m": true, "bb_squeeze_5m": true,
		"oi_spike_1h": true, "squeeze_explosion_synergy": true,
	}
	// Short strong signals
	shortStrongSignals := map[string]bool{
		"oi_distribution": true, "oi_price_aligned_short": true, "oi_moderate_short": true,
		"oi_long_squeeze": true, "oi_long_squeeze_moderate": true,
		"lsr_bearish_reversal": true, "lsr_bearish_strong": true,
		"bb_squeeze_15m": true, "bb_squeeze_5m": true,
		"oi_spike_1h": true, "squeeze_explosion_synergy": true,
	}
	// v7: 宁缺勿滥分离 — 双方向各自独立≥2个强信号，避免单方向占满
	checkTop := 10
	if len(filtered) < checkTop {
		checkTop = len(filtered)
	}
	longStrongCount, shortStrongCount := 0, 0
	for i := 0; i < checkTop; i++ {
		hasLong, hasShort := false, false
		for _, t := range filtered[i].score.Tags {
			if longStrongSignals[t] {
				hasLong = true
			}
			if shortStrongSignals[t] {
				hasShort = true
			}
		}
		if hasLong {
			longStrongCount++
		}
		if hasShort {
			shortStrongCount++
		}
	}
	// 任一方向≥2即通过（OR逻辑，宁缺勿滥但不扼杀单方向机会）
	if longStrongCount < 2 && shortStrongCount < 2 {
		log.Printf("⚠️  Hunter 宁缺勿滥: LONG强信号%d个 SHORT强信号%d个 (各自需≥2), 观望", longStrongCount, shortStrongCount)
		return nil, nil
	}
	log.Printf("✅ Hunter 宁缺勿滥通过: LONG强信号%d个, SHORT强信号%d个", longStrongCount, shortStrongCount)

	topN := 30
	if len(filtered) < topN {
		topN = len(filtered)
	}
	now := time.Now().Unix()
	coins := make([]nofxos.CoinData, 0, topN)
	longCount, shortCount := 0, 0
	for i := 0; i < topN; i++ {
		p := filtered[i]
		price := parseFloat(p.ticker.LastPrice)
		if p.score.Direction == "SHORT" {
			shortCount++
		} else {
			longCount++
		}
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
			Direction:       p.score.Direction,
			LongScore:       p.score.LongFinalScore,
			ShortScore:      p.score.ShortFinalScore,
			LongTags:        p.score.LongTags,
			ShortTags:       p.score.ShortTags,
		})
	}

	c.cache.Set(cacheKey, coins, CacheTTLHunter)
	source := "Binance"
	if binanceErr != nil {
		source = "CoinGecko fallback"
	}
	log.Printf("🎯 Hunter (%s): scored %d perps, returning top %d (LONG:%d SHORT:%d)", source, len(pool), topN, longCount, shortCount)
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

// HunterCoinMeta carries direction, signal tags, and bidirectional scores for each Hunter coin.
type HunterCoinMeta struct {
	Direction  string
	SignalTags []string
	LongScore  float64
	ShortScore float64
	LongTags   []string
	ShortTags  []string
}

// GetHunterCoinsWithData returns top N symbols, pre-fetched kline data, and direction metadata.
func (c *Client) GetHunterCoinsWithData(limit int) ([]string, map[string]*market.PreFetchedData, map[string]*HunterCoinMeta, error) {
	coins, err := c.GetHunterList()
	if err != nil {
		return nil, nil, nil, err
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > len(coins) {
		limit = len(coins)
	}

	symbols := make([]string, 0, limit)
	preFetched := make(map[string]*market.PreFetchedData, limit)
	coinMeta := make(map[string]*HunterCoinMeta, limit)

	for i := 0; i < limit; i++ {
		sym := normalizeSymbol(coins[i].Pair)
		coinMeta[sym] = &HunterCoinMeta{
			Direction:  coins[i].Direction,
			SignalTags: coins[i].SignalTags,
			LongScore:  coins[i].LongScore,
			ShortScore: coins[i].ShortScore,
			LongTags:   coins[i].LongTags,
			ShortTags:  coins[i].ShortTags,
		}
		symbols = append(symbols, sym)

		tfKlines := make(map[string][]market.Kline)
		for _, tf := range []string{"5m", "1h", "4h"} {
			bars, err := c.fetchKlines(sym, tf, 200)
			if err != nil {
				log.Printf("⚠️  Hunter pre-fetch klines failed for %s %s: %v", sym, tf, err)
				continue
			}
			klines := make([]market.Kline, 0, len(bars))
			for _, b := range bars {
				klines = append(klines, market.Kline{
					OpenTime:            b.OpenTime,
					Open:                b.Open,
					High:                b.High,
					Low:                 b.Low,
					Close:               b.Close,
					Volume:              b.Volume,
					Trades:              int(b.Trades),
					TakerBuyBaseVolume:  b.TakerBuyBaseVolume,
					TakerBuyQuoteVolume: b.TakerBuyQuoteVol,
				})
			}
			tfKlines[tf] = klines
		}

		if len(tfKlines) > 0 {
			preFetched[sym] = &market.PreFetchedData{
				TimeframeKlines: tfKlines,
			}
		}
	}

	return symbols, preFetched, coinMeta, nil
}
