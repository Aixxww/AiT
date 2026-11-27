package market

import "testing"

// #157: indicators.klines.longer_count sets the longer-timeframe context's
// series points; the default (unset) is the previous fixed 10.
func TestLongerCount157(t *testing.T) {
	kl := make([]Kline, 80)
	for i := range kl {
		c := 100 + float64(i%7)
		kl[i] = Kline{Open: c, High: c + 1, Low: c - 1, Close: c, Volume: 10}
	}
	def, _ := BuildDataFromKlines("AUSDT", kl, kl)
	ten, _ := BuildDataFromKlinesWithLongerCount("AUSDT", kl, kl, 0)
	five, _ := BuildDataFromKlinesWithLongerCount("AUSDT", kl, kl, 5)
	if len(def.LongerTermContext.RSI14Values) != 10 || len(def.LongerTermContext.MACDValues) != 10 {
		t.Fatalf("default longer series = %d/%d, want 10/10", len(def.LongerTermContext.RSI14Values), len(def.LongerTermContext.MACDValues))
	}
	if len(ten.LongerTermContext.RSI14Values) != 10 {
		t.Fatal("longer_count 0 must mean the default 10")
	}
	if len(five.LongerTermContext.RSI14Values) != 5 || len(five.LongerTermContext.MACDValues) != 5 {
		t.Fatalf("longer_count 5 = %d/%d", len(five.LongerTermContext.RSI14Values), len(five.LongerTermContext.MACDValues))
	}
}
