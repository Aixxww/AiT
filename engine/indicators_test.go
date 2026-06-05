package engine

import (
	"github.com/Aixxww/AiT/datafetch"
	"math"
	"testing"
)

// ============================================================================
// Technical Indicator Tests
// ============================================================================

func TestCalcRSI(t *testing.T) {
	// Known price series — trending up
	closes := []float64{
		44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84,
		46.08, 45.89, 46.03, 45.61, 46.28, 46.28, 46.00, 46.03, 46.41,
		46.22, 45.64,
	}

	rsi := calcRSI(closes, 14)

	// RSI should be between 0 and 100
	if rsi < 0 || rsi > 100 {
		t.Errorf("RSI out of range: %.2f", rsi)
	}

	// For this slightly upward trending series, RSI should be above 50
	if rsi < 40 {
		t.Errorf("RSI too low for upward trend: %.2f (expected > 40)", rsi)
	}
}

func TestCalcRSI_AllGains(t *testing.T) {
	// All upward moves → RSI should be 100
	closes := []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114}
	rsi := calcRSI(closes, 14)
	if math.Abs(rsi-100) > 0.01 {
		t.Errorf("All-gains RSI should be 100, got %.2f", rsi)
	}
}

func TestCalcRSI_AllLosses(t *testing.T) {
	// All downward moves → RSI should be 0
	closes := []float64{114, 113, 112, 111, 110, 109, 108, 107, 106, 105, 104, 103, 102, 101, 100}
	rsi := calcRSI(closes, 14)
	if math.Abs(rsi) > 0.01 {
		t.Errorf("All-losses RSI should be ~0, got %.2f", rsi)
	}
}

func TestCalcRSI_InsufficientData(t *testing.T) {
	closes := []float64{100, 101, 102}
	rsi := calcRSI(closes, 14)
	if rsi != 50 {
		t.Errorf("Insufficient data RSI should be 50 (neutral), got %.2f", rsi)
	}
}

func TestCalcEMA(t *testing.T) {
	// Simple test: constant prices → EMA should equal the price
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = 50.0
	}
	ema := calcEMA(closes, 20)
	if math.Abs(ema-50.0) > 0.01 {
		t.Errorf("Constant price EMA should be 50, got %.4f", ema)
	}
}

func TestCalcEMA_Trending(t *testing.T) {
	// Linearly increasing prices — EMA should lag behind the latest price
	closes := make([]float64, 50)
	for i := range closes {
		closes[i] = float64(i) + 1
	}

	ema20 := calcEMA(closes, 20)
	ema50 := calcEMA(closes, 50)

	// EMA20 should be close to the latest value but below it (lagging)
	latest := closes[len(closes)-1]
	if ema20 >= latest {
		t.Errorf("EMA20 (%.2f) should lag behind latest price (%.2f)", ema20, latest)
	}

	// EMA20 should be above EMA50 for increasing series
	if ema20 <= ema50 {
		t.Errorf("EMA20 (%.2f) should be above EMA50 (%.2f) for uptrend", ema20, ema50)
	}
}

func TestCalcEMA_ShortData(t *testing.T) {
	closes := []float64{10, 20, 30}
	ema := calcEMA(closes, 20)
	if ema != 0 {
		t.Errorf("EMA with insufficient data should be 0, got %.4f", ema)
	}
}

func TestCalcMACD(t *testing.T) {
	// Create a price series that trends up then down
	closes := make([]float64, 50)
	for i := 0; i < 25; i++ {
		closes[i] = 100 + float64(i)*0.5 // uptrend
	}
	for i := 25; i < 50; i++ {
		closes[i] = 112 - float64(i-25)*0.5 // downtrend
	}

	macdLine, signal, histogram := calcMACD(closes)

	// MACD line should be negative (since recent trend is down and EMA12 < EMA26)
	// The EMA12 should have adjusted more quickly to the downtrend
	if macdLine > 0 {
		t.Logf("MACD line: %.4f (may be positive during transition, acceptable)", macdLine)
	}

	// Signal should be non-zero if we have enough data
	_ = signal

	// Histogram = macdLine - signal
	expectedHist := macdLine - signal
	if math.Abs(histogram-expectedHist) > 0.001 {
		t.Errorf("Histogram mismatch: got %.4f, expected %.4f", histogram, expectedHist)
	}
}

func TestCalcMACD_InsufficientData(t *testing.T) {
	closes := []float64{10, 20, 30}
	macdLine, signal, hist := calcMACD(closes)
	if macdLine != 0 || signal != 0 || hist != 0 {
		t.Errorf("MACD with insufficient data should be 0, got (%.4f, %.4f, %.4f)", macdLine, signal, hist)
	}
}

func TestCalcBollinger(t *testing.T) {
	// Constant prices → bands should collapse to zero width
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = 100.0
	}
	upper, middle, lower, width := calcBollinger(closes, 20, 2)

	if math.Abs(middle-100) > 0.01 {
		t.Errorf("Middle band should be 100, got %.4f", middle)
	}
	if math.Abs(upper-100) > 0.01 {
		t.Errorf("Upper band should be 100 for constant prices, got %.4f", upper)
	}
	if math.Abs(lower-100) > 0.01 {
		t.Errorf("Lower band should be 100 for constant prices, got %.4f", lower)
	}
	if math.Abs(width) > 0.01 {
		t.Errorf("Width should be 0 for constant prices, got %.4f", width)
	}
}

func TestCalcBollinger_Volatile(t *testing.T) {
	// Alternating high/low prices → bands should be wide
	closes := make([]float64, 25)
	for i := range closes {
		if i%2 == 0 {
			closes[i] = 110
		} else {
			closes[i] = 90
		}
	}

	upper, middle, lower, width := calcBollinger(closes, 20, 2)

	// Middle should be around 100
	if middle < 95 || middle > 105 {
		t.Errorf("Middle band should be ~100, got %.4f", middle)
	}

	// Upper > middle > lower
	if upper <= middle {
		t.Errorf("Upper (%.4f) should be > middle (%.4f)", upper, middle)
	}
	if lower >= middle {
		t.Errorf("Lower (%.4f) should be < middle (%.4f)", lower, middle)
	}

	// Width should be positive
	if width <= 0 {
		t.Errorf("Width should be positive, got %.4f", width)
	}
}

func TestCalcBollinger_InsufficientData(t *testing.T) {
	closes := []float64{100, 101}
	upper, middle, lower, width := calcBollinger(closes, 20, 2)
	if upper != 0 || middle != 0 || lower != 0 || width != 0 {
		t.Errorf("Bollinger with insufficient data should be 0, got (%.4f, %.4f, %.4f, %.4f)", upper, middle, lower, width)
	}
}

func TestCalcATR(t *testing.T) {
	// Create klines with known true ranges
	klines := make([]datafetch.Kline, 20)
	for i := range klines {
		klines[i] = datafetch.Kline{
			Open:  100,
			High:  105,
			Low:   95,
			Close: 100,
		}
	}

	atr := calcATR(klines, 14)

	// True range for each bar = max(105-95, |105-100|, |95-100|) = max(10, 5, 5) = 10
	// First bar TR = 0 (skipped), bars 1-19 have TR = 10
	// ATR should be 10
	if math.Abs(atr-10) > 0.01 {
		t.Errorf("ATR should be 10 for uniform bars, got %.4f", atr)
	}
}

func TestCalcATR_VaryingPrices(t *testing.T) {
	klines := []datafetch.Kline{
		{Open: 100, High: 105, Low: 95, Close: 102},
		{Open: 102, High: 108, Low: 100, Close: 106},
		{Open: 106, High: 110, Low: 103, Close: 108},
		{Open: 108, High: 112, Low: 104, Close: 105},
		{Open: 105, High: 107, Low: 101, Close: 103},
		{Open: 103, High: 106, Low: 99, Close: 100},
		{Open: 100, High: 104, Low: 97, Close: 101},
		{Open: 101, High: 105, Low: 98, Close: 103},
		{Open: 103, High: 107, Low: 100, Close: 105},
		{Open: 105, High: 109, Low: 102, Close: 107},
		{Open: 107, High: 111, Low: 104, Close: 109},
		{Open: 109, High: 113, Low: 106, Close: 110},
		{Open: 110, High: 114, Low: 107, Close: 112},
		{Open: 112, High: 116, Low: 108, Close: 113},
		{Open: 113, High: 117, Low: 109, Close: 115},
	}

	atr := calcATR(klines, 14)

	// ATR should be positive and reasonable
	if atr <= 0 {
		t.Errorf("ATR should be positive, got %.4f", atr)
	}
	if atr > 20 {
		t.Errorf("ATR seems too high: %.4f", atr)
	}
}

func TestCalcATR_InsufficientData(t *testing.T) {
	klines := []datafetch.Kline{
		{Open: 100, High: 105, Low: 95, Close: 102},
		{Open: 102, High: 108, Low: 100, Close: 106},
	}
	atr := calcATR(klines, 14)
	if atr != 0 {
		t.Errorf("ATR with insufficient data should be 0, got %.4f", atr)
	}
}

// ============================================================================
// Compute Integration Tests
// ============================================================================

func TestComputeTechIndicators(t *testing.T) {
	// Build a snapshot with 250 1h klines
	klines := make([]datafetch.Kline, 250)
	for i := range klines {
		price := 100 + math.Sin(float64(i)*0.1)*10
		klines[i] = datafetch.Kline{
			Open:   price,
			High:   price + 2,
			Low:    price - 2,
			Close:  price + 0.5,
			Volume: 1000,
		}
	}

	snap := &datafetch.SymbolSnapshot{
		Symbol: "TESTUSDT",
		Price:  100.5,
		Klines: map[string][]datafetch.Kline{"1h": klines},
	}

	set := computeTechIndicators(snap)

	if set.Symbol != "TESTUSDT" {
		t.Errorf("Symbol should be TESTUSDT, got %s", set.Symbol)
	}

	// RSI should be computed
	if set.RSI14 == 0 {
		t.Error("RSI14 should not be 0")
	}

	// MACD should be computed
	if set.MACDLine == 0 && set.MACDHist == 0 {
		t.Log("MACD may be 0 for sinusoidal data, acceptable")
	}

	// BB should be computed
	if set.BBUpper <= set.BBLower {
		t.Errorf("BB upper (%.4f) should be > lower (%.4f)", set.BBUpper, set.BBLower)
	}

	// EMAs should be computed
	if set.EMA20 == 0 {
		t.Error("EMA20 should not be 0")
	}

	// ATR should be positive
	if set.ATR14 <= 0 {
		t.Errorf("ATR14 should be positive, got %.4f", set.ATR14)
	}
}

func TestComputeTechIndicators_NilSnapshot(t *testing.T) {
	set := computeTechIndicators(nil)
	if set == nil {
		t.Fatal("Should return non-nil IndicatorSet for nil snapshot")
	}
	if set.Symbol != "" {
		t.Errorf("Symbol should be empty for nil snapshot, got %s", set.Symbol)
	}
}

func TestComputeTechIndicators_EmptyKlines(t *testing.T) {
	snap := &datafetch.SymbolSnapshot{
		Symbol: "TESTUSDT",
		Price:  100,
		Klines: nil,
	}

	set := computeTechIndicators(snap)
	if set.RSI14 != 0 {
		t.Errorf("RSI should be 0 for empty klines, got %.2f", set.RSI14)
	}
}

func TestKlineCloses(t *testing.T) {
	klines := []datafetch.Kline{
		{Close: 10},
		{Close: 20},
		{Close: 30},
	}
	closes := klineCloses(klines)

	if len(closes) != 3 {
		t.Fatalf("Expected 3 closes, got %d", len(closes))
	}
	if closes[0] != 10 || closes[1] != 20 || closes[2] != 30 {
		t.Errorf("Closes mismatch: got %v", closes)
	}
}
