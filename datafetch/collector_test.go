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
