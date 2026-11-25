package kernel

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/engine"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// #87 L4: finalists are chosen best-first, refreshed in parallel under one
// deadline, and a symbol that misses the deadline keeps its snapshot data
// with a stamped age.

func TestSelectHunterV7Finalists(t *testing.T) {
	ctx := &Context{
		Positions: []PositionInfo{{Symbol: "HELDUSDT"}},
		CandidateCoins: []CandidateCoin{
			{Symbol: "W1USDT", V7SetupType: "x", V7ExecutionTier: "WATCH", V7AIPriority: 99},
			{Symbol: "E1USDT", V7SetupType: "x", V7ExecutionTier: "EXECUTABLE", V7AIPriority: 60},
			{Symbol: "R1USDT", V7SetupType: "x", V7ExecutionTier: "REVIEWABLE", V7AIPriority: 80},
			{Symbol: "E2USDT", V7SetupType: "x", V7ExecutionTier: "EXECUTABLE", V7AIPriority: 70},
			{Symbol: "HELDUSDT", V7SetupType: "x", V7ExecutionTier: "EXECUTABLE", V7AIPriority: 90},
			{Symbol: "NODATAUSDT", V7SetupType: "x", V7ExecutionTier: "EXECUTABLE", V7AIPriority: 95},
		},
		MarketDataMap: map[string]*market.Data{"W1USDT": {}, "E1USDT": {}, "R1USDT": {}, "E2USDT": {}, "HELDUSDT": {}},
	}
	got := selectHunterV7Finalists(ctx, 3)
	if strings.Join(got, ",") != "E2USDT,E1USDT,R1USDT" {
		t.Fatalf("finalists %v", got)
	}
}

func TestRefreshHunterV7FinalistsDeadlineAndFallback(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	mux := http.NewServeMux()
	slow := func(r *http.Request) bool { return strings.Contains(r.URL.RawQuery, "SLOWUSDT") }
	block := func(r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}
	mux.HandleFunc("/fapi/v1/ticker/24hr", func(w http.ResponseWriter, r *http.Request) {
		if slow(r) {
			block(r)
			return
		}
		fmt.Fprint(w, `{"symbol":"X","lastPrice":"105","priceChangePercent":"1","volume":"10","quoteVolume":"1000","highPrice":"106","lowPrice":"99","count":1}`)
	})
	mux.HandleFunc("/fapi/v1/premiumIndex", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"markPrice":"105","indexPrice":"105","lastFundingRate":"0.0001","nextFundingTime":1}`)
	})
	mux.HandleFunc("/fapi/v1/openInterest", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"openInterest":"10"}`) })
	mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[[%d,"104","106","103","105","100",%d,"0","0","0","60","0"]]`, nowMs-30_000, nowMs+30_000)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local listener unavailable: %v", err)
	}
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	se := newSnapshotEngine(engine.HubConfig{}, datafetch.CollectorConfig{BinanceURL: srv.URL})
	old := time.Now().Add(-40 * time.Second)
	se.dataStore.Swap(&datafetch.Snapshot{CreatedAt: old, Symbols: map[string]*datafetch.SymbolSnapshot{
		"FASTUSDT": {Symbol: "FASTUSDT", Price: 100, PriceAt: old, Klines: map[string][]datafetch.Kline{}},
		"SLOWUSDT": {Symbol: "SLOWUSDT", Price: 100, PriceAt: old, Klines: map[string][]datafetch.Kline{}},
	}})
	cfg := store.StrategyConfig{CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"}}
	eng := NewStrategyEngine(&cfg)
	eng.SetSnapshotEngine(se)

	prev := hunterV7FinalistDeadline
	hunterV7FinalistDeadline = 400 * time.Millisecond
	defer func() { hunterV7FinalistDeadline = prev }()

	ctx := &Context{
		CandidateCoins: []CandidateCoin{
			{Symbol: "FASTUSDT", V7SetupType: "x", V7ExecutionTier: "EXECUTABLE"},
			{Symbol: "SLOWUSDT", V7SetupType: "x", V7ExecutionTier: "REVIEWABLE"},
		},
		MarketDataMap: map[string]*market.Data{
			"FASTUSDT": {Symbol: "FASTUSDT", CurrentPrice: 100},
			"SLOWUSDT": {Symbol: "SLOWUSDT", CurrentPrice: 100},
		},
	}
	start := time.Now()
	refreshHunterV7Finalists(ctx, eng, []string{"1m"}, "1m", 10)
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("deadline not enforced: %v", el)
	}
	byS := map[string]HunterV7FinalistRefresh{}
	for _, f := range ctx.HunterV7Finalists {
		byS[f.Symbol] = f
	}
	if f := byS["FASTUSDT"]; !f.Refreshed || ctx.MarketDataMap["FASTUSDT"].CurrentPrice != 105 {
		t.Fatalf("fast finalist not refreshed: %+v price %.2f", f, ctx.MarketDataMap["FASTUSDT"].CurrentPrice)
	}
	f := byS["SLOWUSDT"]
	if f.Refreshed || ctx.MarketDataMap["SLOWUSDT"].CurrentPrice != 100 {
		t.Fatalf("slow finalist must fall back to snapshot data: %+v", f)
	}
	if f.DataAge < 30*time.Second {
		t.Fatalf("fallback age not stamped: %v", f.DataAge)
	}
	if se.GetSnapshot().Symbols["FASTUSDT"].Price != 105 {
		t.Fatal("refreshed finalist not patched into the store")
	}
}
