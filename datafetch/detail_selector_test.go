package datafetch

import "testing"

func TestSelectDetailSymbolsIncludesMoversAndFundingOutliers(t *testing.T) {
	symbols := []string{
		"VOL1USDT", "VOL2USDT", "VOL3USDT", "VOL4USDT", "VOL5USDT",
		"AMPUSDT", "GAINERUSDT", "LOSERUSDT", "FUNDUSDT",
	}
	tickers := map[string]*ticker24hrRaw{
		"VOL1USDT":   {Symbol: "VOL1USDT", QuoteVolume: "100000000", PriceChangePercent: "1"},
		"VOL2USDT":   {Symbol: "VOL2USDT", QuoteVolume: "90000000", PriceChangePercent: "1"},
		"VOL3USDT":   {Symbol: "VOL3USDT", QuoteVolume: "80000000", PriceChangePercent: "1"},
		"VOL4USDT":   {Symbol: "VOL4USDT", QuoteVolume: "70000000", PriceChangePercent: "1"},
		"VOL5USDT":   {Symbol: "VOL5USDT", QuoteVolume: "60000000", PriceChangePercent: "1"},
		"AMPUSDT":    {Symbol: "AMPUSDT", QuoteVolume: "100000", PriceChangePercent: "2", HighPrice: "2.00", LowPrice: "1.00"},
		"GAINERUSDT": {Symbol: "GAINERUSDT", QuoteVolume: "100000", PriceChangePercent: "48"},
		"LOSERUSDT":  {Symbol: "LOSERUSDT", QuoteVolume: "100000", PriceChangePercent: "-42"},
		"FUNDUSDT":   {Symbol: "FUNDUSDT", QuoteVolume: "100000", PriceChangePercent: "0"},
	}
	premiums := map[string]*premiumIndexRaw{
		"FUNDUSDT": {Symbol: "FUNDUSDT", LastFundingRate: "0.0020"},
	}

	got := selectDetailSymbols(symbols, tickers, premiums, 7)

	for _, want := range []string{"AMPUSDT", "GAINERUSDT", "LOSERUSDT", "FUNDUSDT"} {
		if !containsSymbol(got, want) {
			t.Fatalf("expected %s in detail pool, got %+v", want, got)
		}
	}
}

func containsSymbol(symbols []string, want string) bool {
	for _, sym := range symbols {
		if sym == want {
			return true
		}
	}
	return false
}
