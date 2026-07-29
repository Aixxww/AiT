package datafetch

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

func TestDataCollectorRefreshSymbolPatchesSingleSnapshot(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/ticker/24hr", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"symbol":"BTCUSDT","lastPrice":"101","priceChangePercent":"2.5","volume":"10","quoteVolume":"1010","highPrice":"105","lowPrice":"95","count":123}`)
	})
	mux.HandleFunc("/fapi/v1/premiumIndex", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"symbol":"BTCUSDT","markPrice":"101.2","indexPrice":"101","lastFundingRate":"0.0002","nextFundingTime":123456789}`)
	})
	mux.HandleFunc("/fapi/v1/openInterest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"openInterest":"456"}`)
	})
	mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[[%d,"100","102","99","101","100",%d,"0","0","0","60","0"]]`, nowMs-30_000, nowMs+30_000)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local listener unavailable in sandbox: %v", err)
	}
	server := httptest.NewUnstartedServer(mux)
	server.Listener = ln
	server.Start()
	defer server.Close()

	store := NewStore()
	store.Swap(&Snapshot{
		CreatedAt: time.Now().Add(-time.Minute),
		Meta:      SnapshotMeta{SymbolCount: 2},
		Symbols: map[string]*SymbolSnapshot{
			"BTCUSDT": {
				Symbol:         "BTCUSDT",
				Price:          99,
				OIDelta1h:      1.2,
				OIDelta4h:      3.4,
				LongShortRatio: 1.1,
				Klines:         map[string][]Kline{"4h": {{Open: 90, Close: 100}}},
			},
			"ETHUSDT": {Symbol: "ETHUSDT", Price: 2000},
		},
	})
	dc := NewDataCollector(CollectorConfig{BinanceURL: server.URL}, store)

	refreshed, err := dc.RefreshSymbol(context.Background(), "BTCUSDT", []KlineInterval{{Interval: "1m", Limit: 1}}, false)
	if err != nil {
		t.Fatalf("RefreshSymbol: %v", err)
	}
	if refreshed.Price != 101 || refreshed.FundingRate != 0.0002 || refreshed.OI != 456 {
		t.Fatalf("refreshed fields = %+v", refreshed)
	}
	if refreshed.OIDelta1h != 1.2 || refreshed.OIDelta4h != 3.4 || refreshed.LongShortRatio != 1.1 {
		t.Fatalf("slow derivatives should be carried from base snapshot: %+v", refreshed)
	}
	current := store.Current()
	if current.Symbols["ETHUSDT"].Price != 2000 {
		t.Fatalf("other symbols should remain patched into snapshot: %+v", current.Symbols["ETHUSDT"])
	}
	if len(current.Symbols["BTCUSDT"].Klines["1m"]) != 1 {
		t.Fatalf("1m kline not refreshed: %+v", current.Symbols["BTCUSDT"].Klines)
	}
}

func TestFetchExchangeInfoCanIncludeNonCryptoFutures(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/exchangeInfo", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"symbols":[
			{"symbol":"BTCUSDT","status":"TRADING","contractType":"PERPETUAL","baseAsset":"BTC","underlyingType":"COIN"},
			{"symbol":"NVDAUSDT","status":"TRADING","contractType":"PERPETUAL","baseAsset":"NVDA","underlyingType":"STOCK"},
			{"symbol":"XAUUSDT","status":"TRADING","contractType":"PERPETUAL","baseAsset":"XAU","underlyingSubType":["metal"]},
			{"symbol":"ETHUSDC","status":"TRADING","contractType":"PERPETUAL","baseAsset":"ETH","underlyingType":"COIN"},
			{"symbol":"OLDUSDT","status":"SETTLING","contractType":"PERPETUAL","baseAsset":"OLD","underlyingType":"COIN"}
		]}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	filteredFetcher := NewDataFetcher(FetcherConfig{BinanceURL: server.URL})
	filtered, err := filteredFetcher.fetchExchangeInfo(context.Background())
	if err != nil {
		t.Fatalf("filtered fetchExchangeInfo: %v", err)
	}
	if len(filtered) != 1 || filtered[0] != "BTCUSDT" {
		t.Fatalf("default exchangeInfo symbols = %+v, want [BTCUSDT]", filtered)
	}

	inclusiveFetcher := NewDataFetcher(FetcherConfig{
		BinanceURL:              server.URL,
		IncludeNonCryptoFutures: true,
	})
	inclusive, err := inclusiveFetcher.fetchExchangeInfo(context.Background())
	if err != nil {
		t.Fatalf("inclusive fetchExchangeInfo: %v", err)
	}
	want := []string{"BTCUSDT", "NVDAUSDT", "XAUUSDT"}
	if fmt.Sprint(inclusive) != fmt.Sprint(want) {
		t.Fatalf("inclusive exchangeInfo symbols = %+v, want %+v", inclusive, want)
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
