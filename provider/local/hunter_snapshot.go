package local

import (
	"fmt"
	"github.com/Aixxww/AiT/datafetch"
	"math"
	"sort"
	"strconv"
)

// computePercentileOI computes a dynamic OI threshold from a sorted slice of OI values.
// percentile: 0.0-1.0 (e.g., 0.25 = 25th percentile = filter bottom 25%).
// Returns the OI value at the given percentile position.
func computePercentileOI(sortedOI []float64, percentile float64) float64 {
	n := len(sortedOI)
	if n == 0 {
		return 0
	}
	if percentile <= 0 {
		return sortedOI[0]
	}
	if percentile >= 1 {
		return sortedOI[n-1]
	}
	idx := int(float64(n-1) * percentile)
	return sortedOI[idx]
}

// ScoreHunterFromSnapshot computes Hunter 4-pillar scores from a datafetch.Snapshot.
// This replaces GetHunterList + BuildSymbolCaches — zero API calls, pure CPU.
// The cfg parameter is reserved for future OI threshold tuning; pass nil-safe defaults.
type HunterSnapshotConfig struct {
	MinOIValue float64 // default 500_000 (lowered from 2M — 100% filter rate in tests)
	MaxSymbols int     // default 50
}

// ScoreHunterFromSnapshot computes Hunter 4-pillar scores from a datafetch.Snapshot.
// This replaces GetHunterList + BuildSymbolCaches — zero API calls, pure CPU.
func ScoreHunterFromSnapshot(snap *datafetch.Snapshot, cfg HunterSnapshotConfig) []HunterCoinScore {
	if snap == nil || len(snap.Symbols) == 0 {
		return nil
	}

	type candidate struct {
		symbol string
		ss     *datafetch.SymbolSnapshot
		score  HunterCoinScore
	}

	var pool []candidate

	for sym, ss := range snap.Symbols {
		if !isUSDTPerp(sym) {
			continue
		}
		if ss.QuoteVolume24h <= 0 {
			continue
		}
		if ss.OI <= 0 {
			continue
		}

		c := candidate{symbol: sym, ss: ss}
		c.score.Symbol = sym
		c.score.Ticker = binanceTicker{
			Symbol:             sym,
			PriceChangePercent: ftoa(ss.PriceChange24h),
			LastPrice:          ftoa(ss.Price),
			Volume:             ftoa(ss.Volume24h),
			QuoteVolume:        ftoa(ss.QuoteVolume24h),
			Count:              ss.TradeCount24h,
			HighPrice:          ftoa(ss.HighPrice24h),
			LowPrice:           ftoa(ss.LowPrice24h),
		}

		pool = append(pool, c)
	}

	// Collect OI values for dynamic threshold
	oiValues := make([]float64, 0, len(pool))
	for _, c := range pool {
		if c.ss.OI > 0 {
			oiValues = append(oiValues, c.ss.OI)
		}
	}
	sort.Float64s(oiValues)

	// Dynamic OI threshold: use MinOIValue as absolute floor, then add percentile-based gate
	// Default: 25th percentile of pool OI — filters bottom 25% which are micro-cap noise
	absoluteFloor := 100_000.0 // $100K absolute minimum (below this = broken data)
	if cfg.MinOIValue > 0 {
		absoluteFloor = cfg.MinOIValue
	}
	percentileThreshold := computePercentileOI(oiValues, 0.25)
	oiThreshold := math.Max(absoluteFloor, percentileThreshold)
	if oiThreshold < 100_000 {
		oiThreshold = 100_000
	}

	// Log OI distribution for diagnostics
	if len(oiValues) > 0 {
		median := oiValues[len(oiValues)/2]
		fmt.Printf("[Hunter Snapshot] OI pool: n=%d min=$%.0f median=$%.0f max=$%.0f threshold=$%.0f (floor=$%.0f p25=$%.0f)\n",
			len(oiValues), oiValues[0], median, oiValues[len(oiValues)-1], oiThreshold, absoluteFloor, percentileThreshold)
	}

	// Score each candidate
	for i := range pool {
		c := &pool[i]
		ss := c.ss

		// Log individual OI for top symbols
		fmt.Printf("[Hunter OI] %s OI=$%.0f price=%.4f chg24h=%.2f%%\n",
			c.symbol, ss.OI, ss.Price, ss.PriceChange24h)

		// --- LONG direction ---
		longPos, longPosTags := snapPositionScore(ss, "long")
		longOI, longOITags := snapOISmartScore(ss, "long", oiThreshold)
		longSM, longSMTags := snapSmartMoneyScore(ss)
		longSq, longSqTags := snapSqueezeExplosion(ss)

		longBase50 := longPos*0.65 + longOI*0.35
		longBase25 := (longSM) * 0.80
		longComposite := clampScore(longBase50+longBase25, 0, 75)

		// Confirm filter
		for _, t := range longPosTags {
			if t == "near_support" || t == "near_support_15m" || t == "near_support_5m" {
				hasLSR := false
				for _, s := range longSMTags {
					if s == "lsr_reversal" || s == "lsr_bullish" {
						hasLSR = true
						break
					}
				}
				if !hasLSR {
					longComposite *= 0.70
					break
				}
			}
		}

		// ELP (Extreme Loss Protection)
		pct24h := math.Abs(ss.PriceChange24h)
		if pct24h > 20 {
			longComposite *= 0.10
		} else if pct24h > 15 {
			longComposite *= 0.25
		} else if pct24h > 10 {
			longComposite *= 0.50
		}

		// Wash multiplier
		washMod := snapWashMultiplier(ss)

		// Cooldown
		cdMod := globalCooldown.getCooldownMultiplier(c.symbol)

		longFinal := longComposite * washMod * cdMod
		longFinal += longSq

		// --- SHORT direction ---
		shortPos, shortPosTags := snapPositionScore(ss, "short")
		shortOI, shortOITags := snapOISmartScore(ss, "short", oiThreshold)
		shortSM, shortSMTags := snapSmartMoneyScoreShort(ss)
		shortSq, shortSqTags := snapSqueezeExplosion(ss)

		shortBase50 := shortPos*0.65 + shortOI*0.35
		shortBase25 := (shortSM) * 0.80
		shortComposite := clampScore(shortBase50+shortBase25, 0, 75)

		// ELP for short
		if pct24h > 20 {
			shortComposite *= 0.10
		} else if pct24h > 15 {
			shortComposite *= 0.25
		} else if pct24h > 10 {
			shortComposite *= 0.50
		}

		shortFinal := shortComposite * washMod * cdMod
		shortFinal += shortSq

		// Save both scores before direction picking
		c.score.LongFinalScore = longFinal
		c.score.LongTags = append(longPosTags, append(longOITags, append(longSMTags, longSqTags...)...)...)
		c.score.ShortFinalScore = shortFinal
		c.score.ShortTags = append(shortPosTags, append(shortOITags, append(shortSMTags, shortSqTags...)...)...)

		// Direction picking
		if longFinal >= shortFinal {
			c.score.Direction = "LONG"
			c.score.FinalScore = longFinal
			c.score.Tags = c.score.LongTags
			c.score.PositionScore = longPos
			c.score.OISmartScore = longOI
			c.score.SmartMoneyScore = longSM
		} else {
			c.score.Direction = "SHORT"
			c.score.FinalScore = shortFinal
			c.score.Tags = c.score.ShortTags
			c.score.PositionScore = shortPos
			c.score.OISmartScore = shortOI
			c.score.SmartMoneyScore = shortSM
		}

		c.score.CooldownMod = cdMod
		c.score.WashMod = washMod
		c.score.ShortPositionScore = shortPos
		c.score.ShortOISmartScore = shortOI
		c.score.ShortSmartMoneyScore = shortSM
	}

	// Filter by score threshold (≥15.0)
	var passed []candidate
	for _, c := range pool {
		if c.score.FinalScore >= 15.0 {
			passed = append(passed, c)
		}
	}

	// Sort by FinalScore descending
	sort.Slice(passed, func(i, j int) bool {
		return passed[i].score.FinalScore > passed[j].score.FinalScore
	})

	// Quality gate (宁缺勿滥)
	limit := 30
	if len(passed) > limit {
		passed = passed[:limit]
	}

	result := make([]HunterCoinScore, len(passed))
	for i, c := range passed {
		result[i] = c.score
	}
	return result
}

// --- Position Score (Pillar A) ---

func snapPositionScore(ss *datafetch.SymbolSnapshot, direction string) (float64, []string) {
	currentPrice := ss.Price
	if currentPrice <= 0 {
		return 0, nil
	}

	score := 0.0
	var tags []string

	// Multi-timeframe ATR-based support/resistance
	tfs := []struct {
		name       string
		multiplier float64
		weight     float64
	}{
		{"4h", 1.5, 0.35},
		{"1d", 2.0, 0.25},
		{"1h", 1.0, 0.20},
		{"15m", 1.0, 0.12},
		{"5m", 1.0, 0.08},
	}

	for _, tf := range tfs {
		klines, ok := ss.Klines[tf.name]
		if !ok || len(klines) < 15 {
			continue
		}
		atr := snapATR(klines, 14)
		if atr <= 0 {
			continue
		}
		high, low := snapHighLow(klines)

		if direction == "long" {
			if currentPrice-low <= tf.multiplier*atr {
				pts := 8.0 * tf.weight / 0.35
				score += pts
				tags = append(tags, "near_support_"+tf.name)
			}
		} else { // short
			if high-currentPrice <= tf.multiplier*atr {
				pts := 8.0 * tf.weight / 0.35
				score += pts
				tags = append(tags, "near_resistance_"+tf.name)
			}
		}
	}

	// Chase penalty
	pct24h := math.Abs(ss.PriceChange24h)
	if pct24h > 50 {
		score -= 20
		tags = append(tags, "chase_penalty")
	}

	return clampScore(score, -35, 55), tags
}

// --- OI Smart Score (Pillar A' ) ---

func snapOISmartScore(ss *datafetch.SymbolSnapshot, direction string, oiThreshold float64) (float64, []string) {
	score := 0.0
	var tags []string

	// Compute OI 4h delta from OISpikeData
	oiDelta4h := snapOIDelta4h(ss)
	oiValue := ss.OI
	priceDir := ss.PriceChange24h

	// OI threshold: use dynamic threshold passed from caller
	threshold := oiThreshold

	if oiValue < threshold {
		tags = append(tags, "oi_too_low")
		return 0, tags
	}

	if direction == "long" {
		// OI Short Squeeze: OI↓ + Price↑ = 空头清算
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
				score += 40
				tags = append(tags, "oi_accumulation")
			}
		} else if math.Abs(oiDelta4h) > 5 {
			score += 15
			tags = append(tags, "oi_moderate")
		}
	} else {
		// SHORT: OI Long Squeeze: OI↓ + Price↓ = long liquidation cascade
		if oiDelta4h < -10 && priceDir < 0 {
			score += 45
			tags = append(tags, "oi_long_squeeze")
		} else if oiDelta4h < -5 && priceDir < 0 {
			score += 20
			tags = append(tags, "oi_long_squeeze_moderate")
		} else if math.Abs(oiDelta4h) > 10 {
			if (priceDir < 0 && oiDelta4h < 0) || (priceDir > 0 && oiDelta4h > 0) {
				score += 40
				tags = append(tags, "oi_price_aligned_short")
			}
			if oiDelta4h > 0 && priceDir > 0 {
				score += 40
				tags = append(tags, "oi_distribution")
			}
		} else if math.Abs(oiDelta4h) > 5 {
			score += 15
			tags = append(tags, "oi_moderate_short")
		}
	}

	return clampScore(score, 0, 50), tags
}

// --- Smart Money Score (Pillar B) ---

func snapSmartMoneyScore(ss *datafetch.SymbolSnapshot) (float64, []string) {
	score := 0.0
	var tags []string

	// LSR Signal
	oldestRatio := ss.LSRPrev
	newestRatio := ss.LongShortRatio

	if oldestRatio > 0 {
		lsrDeltaPct := ((newestRatio - oldestRatio) / oldestRatio) * 100

		if oldestRatio < 0.9 && newestRatio > oldestRatio {
			score += 20
			tags = append(tags, "lsr_reversal")
		} else if newestRatio > 1.1 && newestRatio > oldestRatio {
			score += 15
			tags = append(tags, "lsr_bullish")
		} else if newestRatio < 0.8 {
			score += 10
			tags = append(tags, "lsr_bearish")
		}

		if lsrDeltaPct > 10 {
			score += 10
			tags = append(tags, "lsr_surge")
		}

		if newestRatio > 2.0 {
			score += 15
			tags = append(tags, "lsr_crowded_long")
		}
	}

	// Taker buy/sell signal (from 4h klines)
	taker4h := snapTakerBuyRatio(ss, "4h")
	if taker4h > 0.60 {
		score += 10
		tags = append(tags, "taker_buy_4h")
	}

	// 15m taker micro signal
	taker15m := snapTakerBuyRatio(ss, "15m")
	if taker15m > 0.58 {
		score += 5
		tags = append(tags, "taker_buy_15m")
	} else if taker15m < 0.40 {
		score += 15
		tags = append(tags, "micro_sell_rush")
	}

	return clampScore(score, 0, 65), tags
}

func snapSmartMoneyScoreShort(ss *datafetch.SymbolSnapshot) (float64, []string) {
	score := 0.0
	var tags []string

	oldestRatio := ss.LSRPrev
	newestRatio := ss.LongShortRatio

	if oldestRatio > 0 {
		lsrDeltaPct := ((newestRatio - oldestRatio) / oldestRatio) * 100

		// Bearish reversal: LSR was high (longs dominated), now dropping
		if newestRatio < oldestRatio && oldestRatio > 0.9 {
			score += 20
			tags = append(tags, "lsr_bearish_reversal")
		}
		if newestRatio < 0.8 && newestRatio < oldestRatio {
			score += 15
			tags = append(tags, "lsr_bearish_strong")
		}
		if lsrDeltaPct < -10 {
			score += 10
			tags = append(tags, "lsr_bearish_surge")
		}
		if newestRatio > 2.0 {
			score += 15
			tags = append(tags, "lsr_crowded_long_favor_short")
		}
	}

	// Taker: high sell pressure = bearish
	taker4h := snapTakerBuyRatio(ss, "4h")
	if taker4h < 0.40 {
		score += 10
		tags = append(tags, "taker_sell_4h")
	}

	taker15m := snapTakerBuyRatio(ss, "15m")
	if taker15m < 0.42 {
		score += 5
		tags = append(tags, "taker_sell_15m")
	}

	return clampScore(score, 0, 65), tags
}

// --- Squeeze Explosion (Pillar E) ---

func snapSqueezeExplosion(ss *datafetch.SymbolSnapshot) (float64, []string) {
	bbScore, bbTags := snapBBWidthSqueeze(ss)
	oiScore, oiTags := snapOISpike(ss)

	totalScore := bbScore + oiScore
	allTags := append(append([]string{}, bbTags...), oiTags...)

	if bbScore > 0 && oiScore > 0 {
		totalScore += 5
		allTags = append(allTags, "squeeze_explosion_synergy")
	}

	if totalScore > 25 {
		totalScore = 25
	}
	return totalScore, allTags
}

func snapBBWidthSqueeze(ss *datafetch.SymbolSnapshot) (float64, []string) {
	// Try 15m first (50 bars = 12.5h)
	klines15m := ss.Klines["15m"]
	if len(klines15m) >= 25 {
		widthPct := snapBBWidth(klines15m, 20, 2.0)
		if widthPct > 0 {
			minWidth := math.MaxFloat64
			for i := 20; i <= len(klines15m); i++ {
				w := snapBBWidth(klines15m[:i], 20, 2.0)
				if w > 0 && w < minWidth {
					minWidth = w
				}
			}
			if minWidth > 0 && widthPct <= minWidth*1.10 {
				return 15, []string{"bb_squeeze_15m"}
			}
		}
	}

	// Fallback: 5m
	klines5m := ss.Klines["5m"]
	if len(klines5m) >= 25 {
		widthPct := snapBBWidth(klines5m, 20, 2.0)
		if widthPct > 0 {
			minWidth := math.MaxFloat64
			for i := 20; i <= len(klines5m); i++ {
				w := snapBBWidth(klines5m[:i], 20, 2.0)
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

func snapOISpike(ss *datafetch.SymbolSnapshot) (float64, []string) {
	changes := ss.OISpikeData
	if len(changes) < 3 {
		return 0, nil
	}

	latestChange := changes[len(changes)-1]
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

	if stddev > 0 && latestChange > mean+2*stddev && latestChange > 3.0 {
		return 15, []string{"oi_spike_1h"}
	} else if latestChange > 5.0 {
		return 8, []string{"oi_surge_1h"}
	}
	return 0, nil
}

// --- Wash Multiplier ---

func snapWashMultiplier(ss *datafetch.SymbolSnapshot) float64 {
	mult := 1.0

	// High trade count with low average order size
	avgOrderSize := 0.0
	if ss.TradeCount24h > 0 {
		avgOrderSize = ss.QuoteVolume24h / float64(ss.TradeCount24h)
	}
	if ss.TradeCount24h > 1_000_000 && avgOrderSize < 5 {
		mult *= 0.20
	}

	// OI/Vol ratio check
	if ss.Volume24h > 0 && ss.OI > 0 {
		oiVolRatio := ss.OI / ss.Volume24h
		if oiVolRatio < 0.01 {
			mult *= 0.30
		}
	}

	// Volume spike detection (>10x normal)
	if ss.Volume24h > 0 {
		klines1h := ss.Klines["1h"]
		if len(klines1h) >= 20 {
			avgVol := 0.0
			for _, k := range klines1h[:20] {
				avgVol += k.Volume
			}
			avgVol /= 20
			if avgVol > 0 && ss.Volume24h > avgVol*24*10 {
				mult *= 0.40
			}
		}
	}

	return mult
}

// --- Helper functions ---

func snapOIDelta4h(ss *datafetch.SymbolSnapshot) float64 {
	data := ss.OISpikeData
	if len(data) < 4 {
		return 0
	}
	// Sum last 4 period-over-period changes ≈ 4h delta
	delta := 0.0
	for i := len(data) - 4; i < len(data); i++ {
		if i >= 0 {
			delta += data[i]
		}
	}
	return delta
}

func snapTakerBuyRatio(ss *datafetch.SymbolSnapshot, tf string) float64 {
	klines, ok := ss.Klines[tf]
	if !ok || len(klines) == 0 {
		return 0.5 // neutral default
	}
	var totalVol, takerBuy float64
	for _, k := range klines {
		totalVol += k.Volume
		takerBuy += k.TakerBuy
	}
	if totalVol > 0 {
		return takerBuy / totalVol
	}
	return 0.5
}

func snapATR(klines []datafetch.Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}
	trSum := 0.0
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

func snapHighLow(klines []datafetch.Kline) (high, low float64) {
	if len(klines) == 0 {
		return 0, 0
	}
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

func snapBBWidth(klines []datafetch.Kline, period int, multiplier float64) float64 {
	if len(klines) < period {
		return 0
	}
	sum := 0.0
	start := len(klines) - period
	for i := start; i < len(klines); i++ {
		sum += klines[i].Close
	}
	middle := sum / float64(period)
	if middle == 0 {
		return 0
	}
	variance := 0.0
	for i := start; i < len(klines); i++ {
		diff := klines[i].Close - middle
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(period))
	upper := middle + multiplier*stdDev
	lower := middle - multiplier*stdDev
	return (upper - lower) / middle * 100
}

func clampScore(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ftoa converts float64 to string for binanceTicker fields.
func ftoa(f float64) string {
	if f == 0 {
		return "0"
	}
	// Use strconv to avoid fmt import overhead
	return strconv.FormatFloat(f, 'f', 8, 64)
}

// excludedTokenizedAssets lists tokenized stocks/forex/commodities to exclude.
// (Reuses the set defined in hunter.go via the isUSDTPerp filter)
