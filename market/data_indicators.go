package market

import "math"

// calculateEMA calculates EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	// Calculate SMA as initial EMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)

	// Calculate EMA
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}

	return ema
}

// calculateMACD calculates MACD
func calculateMACD(klines []Kline) float64 {
	if len(klines) < 26 {
		return 0
	}

	// Calculate 12-period and 26-period EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return ema12 - ema26
}

// calculateRSI calculates RSI
func calculateRSI(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	gains := 0.0
	losses := 0.0

	// Calculate initial average gain/loss
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// Use Wilder smoothing method to calculate subsequent RSI
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
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
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateATR calculates ATR
func calculateATR(klines []Kline, period int) float64 {
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

	// Calculate initial ATR
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

// ============================================================================
// ADX (Average Directional Index) — Market Trend Strength
// ============================================================================

// MarketRegime represents the current market environment classification.
type MarketRegime string

const (
	RegimeRanging  MarketRegime = "ranging"  // ADX < 20: sideways, mean-reversion dominates
	RegimeTrending MarketRegime = "trending" // ADX > 25: directional, trend-following dominates
	RegimeNeutral  MarketRegime = "neutral"  // 20 <= ADX <= 25: transitional
)

// MarketEnvironment holds the market regime classification result.
type MarketEnvironment struct {
	Regime  MarketRegime `json:"regime"`
	ADX     float64      `json:"adx"`
	PlusDI  float64      `json:"plus_di"`
	MinusDI float64      `json:"minus_di"`
	Symbol  string       `json:"symbol"`
}

// calculateADXWithDI computes ADX, +DI, and -DI using Wilder's smoothing method.
// period: typically 14. Requires at least period*2+1 bars for accurate results.
// Algorithm:
//  1. Compute +DM/-DM from consecutive highs/lows
//  2. Compute True Range
//  3. Wilder-smooth +DM, -DM, TR over `period` bars
//  4. DI+ = smoothed(+DM) / smoothed(TR) × 100
//  5. DI- = smoothed(-DM) / smoothed(TR) × 100
//  6. DX = |DI+ - DI-| / (DI+ + DI-) × 100
//  7. ADX = Wilder smooth of DX
func calculateADXWithDI(klines []Kline, period int) (adx, plusDI, minusDI float64) {
	if len(klines) < period*2+1 {
		return 0, 0, 0
	}

	n := len(klines)

	// Step 1-2: Compute +DM, -DM, TR for each bar (skip bar 0)
	plusDMs := make([]float64, n)
	minusDMs := make([]float64, n)
	trs := make([]float64, n)

	for i := 1; i < n; i++ {
		high := klines[i].High
		low := klines[i].Low
		prevHigh := klines[i-1].High
		prevLow := klines[i-1].Low
		prevClose := klines[i-1].Close

		upMove := high - prevHigh
		downMove := prevLow - low

		if upMove > downMove && upMove > 0 {
			plusDMs[i] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDMs[i] = downMove
		}

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)
		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// Step 3: Initial Wilder smooth (SMA seed over first `period` bars)
	smoothedPlusDM := 0.0
	smoothedMinusDM := 0.0
	smoothedTR := 0.0
	for i := 1; i <= period; i++ {
		smoothedPlusDM += plusDMs[i]
		smoothedMinusDM += minusDMs[i]
		smoothedTR += trs[i]
	}

	// Step 4-5: Compute DI+ and DI- from initial smooth
	if smoothedTR == 0 {
		return 0, 0, 0
	}
	plusDI = smoothedPlusDM / smoothedTR * 100
	minusDI = smoothedMinusDM / smoothedTR * 100

	// Step 6: First DX
	denom := plusDI + minusDI
	firstDX := 0.0
	if denom > 0 {
		firstDX = math.Abs(plusDI-minusDI) / denom * 100
	}

	// Step 7: Wilder smooth DX values to get ADX
	adxSum := firstDX
	dxCount := 1

	// Continue Wilder smoothing from period+1 to end
	for i := period + 1; i < n; i++ {
		smoothedPlusDM = smoothedPlusDM - smoothedPlusDM/float64(period) + plusDMs[i]
		smoothedMinusDM = smoothedMinusDM - smoothedMinusDM/float64(period) + minusDMs[i]
		smoothedTR = smoothedTR - smoothedTR/float64(period) + trs[i]

		if smoothedTR == 0 {
			continue
		}

		diPlus := smoothedPlusDM / smoothedTR * 100
		diMinus := smoothedMinusDM / smoothedTR * 100
		diDenom := diPlus + diMinus

		dx := 0.0
		if diDenom > 0 {
			dx = math.Abs(diPlus-diMinus) / diDenom * 100
		}

		adxSum += dx
		dxCount++

		// Update latest DI values
		plusDI = diPlus
		minusDI = diMinus
	}

	if dxCount > 0 {
		adx = adxSum / float64(dxCount)
	}

	return adx, plusDI, minusDI
}

// calculateADX returns just the ADX value (convenience wrapper).
func calculateADX(klines []Kline, period int) float64 {
	adx, _, _ := calculateADXWithDI(klines, period)
	return adx
}

// ExportCalculateADX exports ADX calculation for testing.
func ExportCalculateADX(klines []Kline, period int) (float64, float64, float64) {
	return calculateADXWithDI(klines, period)
}

// ClassifyMarketRegime determines the market environment based on ADX.
// ADX < 20 → ranging (mean-reversion favors)
// ADX > 25 → trending (trend-following favors)
// Otherwise → neutral (transitional)
func ClassifyMarketRegime(klines []Kline, period int, symbol string) *MarketEnvironment {
	adx, plusDI, minusDI := calculateADXWithDI(klines, period)

	regime := RegimeNeutral
	if adx < 20 {
		regime = RegimeRanging
	} else if adx > 25 {
		regime = RegimeTrending
	}

	return &MarketEnvironment{
		Regime:  regime,
		ADX:     adx,
		PlusDI:  plusDI,
		MinusDI: minusDI,
		Symbol:  symbol,
	}
}

// calculateBOLL calculates Bollinger Bands (upper, middle, lower)
// period: typically 20, multiplier: typically 2
func calculateBOLL(klines []Kline, period int, multiplier float64) (upper, middle, lower float64) {
	if len(klines) < period {
		return 0, 0, 0
	}

	// Calculate SMA (middle band)
	sum := 0.0
	for i := len(klines) - period; i < len(klines); i++ {
		sum += klines[i].Close
	}
	sma := sum / float64(period)

	// Calculate standard deviation
	variance := 0.0
	for i := len(klines) - period; i < len(klines); i++ {
		diff := klines[i].Close - sma
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(period))

	// Calculate bands
	middle = sma
	upper = sma + multiplier*stdDev
	lower = sma - multiplier*stdDev

	return upper, middle, lower
}

// calculateDonchian calculates Donchian channel (highest high, lowest low) for given period
func calculateDonchian(klines []Kline, period int) (upper, lower float64) {
	if len(klines) == 0 || period <= 0 {
		return 0, 0
	}

	// Use all available klines if period > len(klines)
	start := len(klines) - period
	if start < 0 {
		start = 0
	}

	upper = klines[start].High
	lower = klines[start].Low

	for i := start + 1; i < len(klines); i++ {
		if klines[i].High > upper {
			upper = klines[i].High
		}
		if klines[i].Low < lower {
			lower = klines[i].Low
		}
	}

	return upper, lower
}

// Box period constants (in 1h candles)
const (
	ShortBoxPeriod = 72  // 3 days of 1h candles
	MidBoxPeriod   = 240 // 10 days of 1h candles
	LongBoxPeriod  = 500 // ~21 days of 1h candles
)

// calculateBoxData calculates multi-period box data from klines
func calculateBoxData(klines []Kline, currentPrice float64) *BoxData {
	box := &BoxData{
		CurrentPrice: currentPrice,
	}

	if len(klines) == 0 {
		return box
	}

	box.ShortUpper, box.ShortLower = calculateDonchian(klines, ShortBoxPeriod)
	box.MidUpper, box.MidLower = calculateDonchian(klines, MidBoxPeriod)
	box.LongUpper, box.LongLower = calculateDonchian(klines, LongBoxPeriod)

	return box
}

// ========== Exported indicator calculation functions (for testing) ==========

// ExportCalculateEMA exports calculateEMA for testing
func ExportCalculateEMA(klines []Kline, period int) float64 {
	return calculateEMA(klines, period)
}

// ExportCalculateMACD exports calculateMACD for testing
func ExportCalculateMACD(klines []Kline) float64 {
	return calculateMACD(klines)
}

// ExportCalculateRSI exports calculateRSI for testing
func ExportCalculateRSI(klines []Kline, period int) float64 {
	return calculateRSI(klines, period)
}

// ExportCalculateATR exports calculateATR for testing
func ExportCalculateATR(klines []Kline, period int) float64 {
	return calculateATR(klines, period)
}

// ExportCalculateBOLL exports calculateBOLL for testing
func ExportCalculateBOLL(klines []Kline, period int, multiplier float64) (upper, middle, lower float64) {
	return calculateBOLL(klines, period, multiplier)
}

// ExportCalculateDonchian exports calculateDonchian for testing
func ExportCalculateDonchian(klines []Kline, period int) (float64, float64) {
	return calculateDonchian(klines, period)
}

// ExportCalculateBoxData exports calculateBoxData for testing
func ExportCalculateBoxData(klines []Kline, currentPrice float64) *BoxData {
	return calculateBoxData(klines, currentPrice)
}
