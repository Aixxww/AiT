package engine

import (
	"math"
	"nofx/datafetch"
)

// ============================================================================
// Technical Indicator Calculations
// These mirror the calculations in market/data_indicators.go but operate on
// datafetch.Kline and return results suitable for the scoring engine.
// ============================================================================

// calcRSI calculates RSI using Wilder's smoothing method.
func calcRSI(closes []float64, period int) float64 {
	if len(closes) <= period {
		return 50 // neutral default
	}

	gains := 0.0
	losses := 0.0

	for i := 1; i <= period; i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	for i := period + 1; i < len(closes); i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + (-change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

// calcEMA calculates Exponential Moving Average.
func calcEMA(closes []float64, period int) float64 {
	if len(closes) < period {
		return 0
	}

	// SMA seed for first `period` values
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	ema := sum / float64(period)

	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(closes); i++ {
		ema = (closes[i]-ema)*multiplier + ema
	}

	return ema
}

// calcEMAFromEMA calculates EMA incrementally from a previous EMA value.
// Used internally for MACD signal line.
func calcEMAFromEMA(closes []float64, period int, startIdx int) float64 {
	if startIdx+period > len(closes) {
		return 0
	}

	sum := 0.0
	for i := startIdx; i < startIdx+period; i++ {
		sum += closes[i]
	}
	ema := sum / float64(period)

	multiplier := 2.0 / float64(period+1)
	for i := startIdx + period; i < len(closes); i++ {
		ema = (closes[i]-ema)*multiplier + ema
	}

	return ema
}

// calcMACD calculates MACD (12/26/9).
// Returns macdLine (EMA12 - EMA26), signal line (9-period EMA of MACD), histogram.
func calcMACD(closes []float64) (macdLine, signal, histogram float64) {
	if len(closes) < 26 {
		return 0, 0, 0
	}

	ema12 := calcEMA(closes, 12)
	ema26 := calcEMA(closes, 26)
	macdLine = ema12 - ema26

	// For signal line, we need a MACD series.
	// Build MACD values starting from index 25 (where EMA26 first available).
	macdSeries := make([]float64, 0, len(closes)-25)

	// Recompute rolling EMAs to build the MACD series
	// Seed EMA12 at index 11
	sum12 := 0.0
	for i := 0; i < 12; i++ {
		sum12 += closes[i]
	}
	ema12Val := sum12 / 12.0

	sum26 := 0.0
	for i := 0; i < 26; i++ {
		sum26 += closes[i]
	}
	ema26Val := sum26 / 26.0

	mult12 := 2.0 / 13.0
	mult26 := 2.0 / 27.0

	// Update EMA12 from index 12 to 25
	for i := 12; i < 26; i++ {
		ema12Val = (closes[i]-ema12Val)*mult12 + ema12Val
	}

	// First MACD point at index 25
	macdSeries = append(macdSeries, ema12Val-ema26Val)

	// Continue from index 26
	for i := 26; i < len(closes); i++ {
		ema12Val = (closes[i]-ema12Val)*mult12 + ema12Val
		ema26Val = (closes[i]-ema26Val)*mult26 + ema26Val
		macdSeries = append(macdSeries, ema12Val-ema26Val)
	}

	// Signal line = 9-period EMA of MACD series
	if len(macdSeries) < 9 {
		signal = 0
	} else {
		sum := 0.0
		for i := 0; i < 9; i++ {
			sum += macdSeries[i]
		}
		sig := sum / 9.0
		mult := 2.0 / 10.0
		for i := 9; i < len(macdSeries); i++ {
			sig = (macdSeries[i]-sig)*mult + sig
		}
		signal = sig
	}

	histogram = macdLine - signal
	return
}

// calcBollinger calculates Bollinger Bands (SMA ± mult×σ).
func calcBollinger(closes []float64, period, mult int) (upper, middle, lower, width float64) {
	if len(closes) < period {
		return 0, 0, 0, 0
	}

	// SMA from the last `period` closes
	sum := 0.0
	start := len(closes) - period
	for i := start; i < len(closes); i++ {
		sum += closes[i]
	}
	sma := sum / float64(period)

	// Standard deviation
	variance := 0.0
	for i := start; i < len(closes); i++ {
		diff := closes[i] - sma
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(period))

	middle = sma
	upper = sma + float64(mult)*stdDev
	lower = sma - float64(mult)*stdDev

	width = 0.0
	if middle > 0 {
		width = (upper - lower) / middle * 100 // bandwidth as percentage
	}

	return
}

// calcATR calculates Average True Range from kline data.
func calcATR(klines []datafetch.Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	trs := make([]float64, len(klines))
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// Initial ATR = SMA of first `period` true ranges
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	atr := sum / float64(period)

	// Wilder smoothing
	for i := period + 1; i < len(klines); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}

	return atr
}

// klineCloses extracts close prices from a kline slice.
func klineCloses(klines []datafetch.Kline) []float64 {
	closes := make([]float64, len(klines))
	for i, k := range klines {
		closes[i] = k.Close
	}
	return closes
}

// computeTechIndicators computes all technical indicators from the symbol's 1h klines.
func computeTechIndicators(snap *datafetch.SymbolSnapshot) *IndicatorSet {
	set := &IndicatorSet{}

	if snap == nil {
		return set
	}

	set.Symbol = snap.Symbol

	klines1h := snap.Klines["1h"]
	if len(klines1h) == 0 {
		return set
	}

	closes := klineCloses(klines1h)

	// RSI 14
	set.RSI14 = calcRSI(closes, 14)

	// MACD 12/26/9
	set.MACDLine, set.MACDSignal, set.MACDHist = calcMACD(closes)

	// Bollinger Bands 20/2
	set.BBUpper, set.BBMiddle, set.BBLower, set.BBWidth = calcBollinger(closes, 20, 2)

	// EMAs
	set.EMA20 = calcEMA(closes, 20)
	set.EMA50 = calcEMA(closes, 50)
	set.EMA200 = calcEMA(closes, 200)

	// ATR 14
	set.ATR14 = calcATR(klines1h, 14)

	return set
}

// ============================================================================
// Technical Scoring Functions
// ============================================================================

// scoreTechBull returns a bullish technical score (0-40).
func scoreTechBull(set *IndicatorSet, cfg HubConfig) float64 {
	score := 0.0

	// RSI scoring
	if cfg.RSIEnabled {
		if set.RSI14 < 30 {
			score += 8 // oversold
		} else if set.RSI14 < 40 {
			score += 4 // near oversold
		} else if set.RSI14 > 70 {
			score -= 8 // overbought, bearish for bull
		}
	}

	// MACD histogram scoring
	if cfg.MACDEnabled {
		if set.MACDHist > 0 {
			score += 8 // bullish momentum
		} else if set.MACDHist < 0 {
			score -= 4 // bearish momentum
		}
	}

	// Bollinger Band position scoring
	if cfg.BBEnabled && set.BBMiddle > 0 && set.BBLower > 0 {
		price := set.BBMiddle
		bbRange := set.BBUpper - set.BBLower
		if bbRange > 0 {
			position := (price - set.BBLower) / bbRange
			if position < 0.1 {
				score += 6 // near lower band — potential bounce
			} else if position > 0.9 {
				score -= 6 // near upper band — potential reversal
			}
		}
	}

	// EMA alignment scoring
	if cfg.EMAEnabled && set.EMA20 > 0 && set.EMA50 > 0 && set.EMA200 > 0 {
		if set.EMA20 > set.EMA50 && set.EMA50 > set.EMA200 {
			score += 10 // perfect bull alignment
		} else if set.EMA20 < set.EMA50 && set.EMA50 < set.EMA200 {
			score -= 10 // bear alignment, bearish for bull
		}
	}

	// ATR in moderate range (not extreme volatility)
	if cfg.ATREnabled && set.ATR14 > 0 && set.BBMiddle > 0 {
		atrPct := set.ATR14 / set.BBMiddle * 100
		if atrPct > 1 && atrPct < 5 {
			score += 3 // healthy volatility
		}
	}

	return clamp(score, 0, 40)
}

// scoreTechBear returns a bearish technical score (0-40).
func scoreTechBear(set *IndicatorSet, cfg HubConfig) float64 {
	score := 0.0

	// RSI scoring (bearish perspective)
	if cfg.RSIEnabled {
		if set.RSI14 > 70 {
			score += 8 // overbought
		} else if set.RSI14 > 60 {
			score += 4 // near overbought
		} else if set.RSI14 < 30 {
			score -= 8 // oversold, bullish for bear
		}
	}

	// MACD histogram scoring (bearish perspective)
	if cfg.MACDEnabled {
		if set.MACDHist < 0 {
			score += 8 // bearish momentum
		} else if set.MACDHist > 0 {
			score -= 4 // bullish momentum
		}
	}

	// Bollinger Band position scoring (bearish perspective)
	if cfg.BBEnabled && set.BBMiddle > 0 && set.BBUpper > 0 {
		price := set.BBMiddle
		bbRange := set.BBUpper - set.BBLower
		if bbRange > 0 {
			position := (price - set.BBLower) / bbRange
			if position > 0.9 {
				score += 6 // near upper band — potential drop
			} else if position < 0.1 {
				score -= 6 // near lower band — bull bounce
			}
		}
	}

	// EMA alignment scoring (bearish perspective)
	if cfg.EMAEnabled && set.EMA20 > 0 && set.EMA50 > 0 && set.EMA200 > 0 {
		if set.EMA20 < set.EMA50 && set.EMA50 < set.EMA200 {
			score += 10 // bear alignment
		} else if set.EMA20 > set.EMA50 && set.EMA50 > set.EMA200 {
			score -= 10 // bull alignment, bullish for bear
		}
	}

	// ATR in moderate range (bearish can use volatile conditions)
	if cfg.ATREnabled && set.ATR14 > 0 && set.BBMiddle > 0 {
		atrPct := set.ATR14 / set.BBMiddle * 100
		if atrPct > 1 && atrPct < 5 {
			score += 3
		}
	}

	return clamp(score, 0, 40)
}
