package datafetch

import "testing"

func TestMergeCarriedKlinesKeepsSlowIntervals(t *testing.T) {
	current := &Snapshot{Symbols: map[string]*SymbolSnapshot{
		"BTCUSDT": {
			Symbol: "BTCUSDT",
			Klines: map[string][]Kline{
				"1d": {{Open: 100, Close: 110}},
				"4h": {{Open: 90, Close: 95}},
			},
		},
	}}
	fresh := &Snapshot{Symbols: map[string]*SymbolSnapshot{
		"BTCUSDT": {
			Symbol: "BTCUSDT",
			Klines: map[string][]Kline{
				"4h": {{Open: 96, Close: 99}},
			},
		},
	}}

	mergeCarriedKlines(current, fresh, []KlineInterval{{Interval: "4h", Limit: 100}})

	got := fresh.Symbols["BTCUSDT"].Klines
	if len(got["1d"]) != 1 || got["1d"][0].Close != 110 {
		t.Fatalf("1d kline was not carried over: %+v", got["1d"])
	}
	if len(got["4h"]) != 1 || got["4h"][0].Close != 99 {
		t.Fatalf("refreshed 4h kline was overwritten: %+v", got["4h"])
	}
}

func TestRefreshKlineIntervalsRefreshesStructuralKlinesEveryTenCycles(t *testing.T) {
	first := refreshKlineIntervals(FastKlineIntervals, 1)
	if hasKlineInterval(first, "4h") {
		t.Fatalf("cycle 1 should not include structural 4h refresh: %+v", first)
	}

	tenth := refreshKlineIntervals(FastKlineIntervals, 10)
	if !hasKlineInterval(tenth, "4h") {
		t.Fatalf("cycle 10 should include structural 4h refresh: %+v", tenth)
	}
	if countKlineInterval(tenth, "4h") != 1 {
		t.Fatalf("cycle 10 should include 4h only once: %+v", tenth)
	}
}

func TestRefreshSlowDerivativeDataEveryTenCycles(t *testing.T) {
	if refreshSlowDerivativeData(1) {
		t.Fatal("cycle 1 should carry slow derivatives")
	}
	if !refreshSlowDerivativeData(10) {
		t.Fatal("cycle 10 should refresh slow derivatives")
	}
}

func TestMergeCarriedSlowDerivatives(t *testing.T) {
	current := &Snapshot{Symbols: map[string]*SymbolSnapshot{
		"BTCUSDT": {
			Symbol:         "BTCUSDT",
			OISpikeData:    []float64{1.2, -0.4},
			OIDelta1h:      -0.4,
			OIDelta4h:      2.1,
			LongShortRatio: 1.8,
			LSRPrev:        1.7,
			LSROldest:      1.2,
		},
	}}
	fresh := &Snapshot{Symbols: map[string]*SymbolSnapshot{
		"BTCUSDT": {
			Symbol: "BTCUSDT",
			OI:     123,
		},
	}}

	mergeCarriedSlowDerivatives(current, fresh)

	got := fresh.Symbols["BTCUSDT"]
	if got.OI != 123 {
		t.Fatalf("current OI should not be overwritten, got %.2f", got.OI)
	}
	if got.OIDelta1h != -0.4 || got.OIDelta4h != 2.1 || got.LongShortRatio != 1.8 || got.LSRPrev != 1.7 || got.LSROldest != 1.2 {
		t.Fatalf("slow derivatives were not carried: %+v", got)
	}
	if len(got.OISpikeData) != 2 || got.OISpikeData[0] != 1.2 || got.OISpikeData[1] != -0.4 {
		t.Fatalf("OI spike data was not carried: %+v", got.OISpikeData)
	}
}

func hasKlineInterval(intervals []KlineInterval, interval string) bool {
	return countKlineInterval(intervals, interval) > 0
}

func countKlineInterval(intervals []KlineInterval, interval string) int {
	count := 0
	for _, ki := range intervals {
		if ki.Interval == interval {
			count++
		}
	}
	return count
}

func TestExcludedNonCryptoFuturesSymbolsIncludesTokenizedGold(t *testing.T) {
	if !excludedNonCryptoFuturesSymbol(exchangeSymbolRaw{Symbol: "XAUTUSDT", BaseAsset: "XAUT"}) {
		t.Fatal("XAUTUSDT should be excluded from Binance futures detail fetches")
	}
}

func TestExcludedNonCryptoFuturesSymbolsUsesExchangeInfoMetadata(t *testing.T) {
	cases := []exchangeSymbolRaw{
		{Symbol: "NEWSTOCKUSDT", BaseAsset: "NEWSTOCK", UnderlyingType: "STOCK"},
		{Symbol: "NEWETFUSDT", BaseAsset: "NEWETF", UnderlyingSubType: []string{"ETF"}},
		{Symbol: "NEWMETALUSDT", BaseAsset: "NEWMETAL", UnderlyingSubType: []string{"metal"}},
	}
	for _, tc := range cases {
		if !excludedNonCryptoFuturesSymbol(tc) {
			t.Fatalf("%s should be excluded by metadata", tc.Symbol)
		}
	}

	if excludedNonCryptoFuturesSymbol(exchangeSymbolRaw{Symbol: "BTCUSDT", BaseAsset: "BTC", UnderlyingType: "COIN"}) {
		t.Fatal("BTCUSDT should not be excluded")
	}
}
