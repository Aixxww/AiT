package datafetch

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// startT2TestServer mirrors the collector_test.go fixture pattern.
func startT2TestServer(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local listener unavailable in sandbox: %v", err)
	}
	server := httptest.NewUnstartedServer(mux)
	server.Listener = ln
	server.Start()
	t.Cleanup(server.Close)
	return server
}

// oiHistJSON renders n openInterestHist entries: value grows 1% per step
// from 1,000,000; contracts grow from 500 by 5 per step.
func oiHistJSON(n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		value := 1_000_000.0
		for j := 0; j < i; j++ {
			value *= 1.01
		}
		fmt.Fprintf(&b, `{"symbol":"TESTUSDT","sumOpenInterest":"%v","sumOpenInterestValue":"%v","timestamp":%d}`,
			500+5*i, value, 1700000000000+int64(i)*300000)
	}
	b.WriteString("]")
	return b.String()
}

func TestFetchDepthParsesRecordedBook(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/depth", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Errorf("depth limit = %s, want 100 (scanner parity)", got)
		}
		fmt.Fprint(w, `{"lastUpdateId":12345,"bids":[["99.99","10.5"],["99.95","20"],["99.90","1"]],"asks":[["100.01","5"],["100.05","10.25"]]}`)
	})
	server := startT2TestServer(t, mux)

	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL})
	bids, asks, err := f.fetchDepth(context.Background(), "TESTUSDT")
	if err != nil {
		t.Fatalf("fetchDepth: %v", err)
	}
	if len(bids) != 3 || len(asks) != 2 {
		t.Fatalf("levels = %d bids / %d asks", len(bids), len(asks))
	}
	if bids[0].Price != 99.99 || bids[0].Qty != 10.5 {
		t.Fatalf("best bid = %+v", bids[0])
	}
	if asks[0].Price != 100.01 || asks[1].Qty != 10.25 {
		t.Fatalf("asks = %+v", asks)
	}
}

func TestFetchOIHistorySeriesParsesEntries(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/futures/data/openInterestHist", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("period"); got != "5m" {
			t.Errorf("period = %s, want 5m", got)
		}
		fmt.Fprint(w, oiHistJSON(30))
	})
	server := startT2TestServer(t, mux)

	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL})
	pts, err := f.fetchOIHistorySeries(context.Background(), "TESTUSDT", "5m", 30)
	if err != nil {
		t.Fatalf("fetchOIHistorySeries: %v", err)
	}
	if len(pts) != 30 {
		t.Fatalf("points = %d, want 30", len(pts))
	}
	if pts[0].SumOpenInterest != 500 || pts[0].SumOpenInterestValue != 1_000_000 {
		t.Fatalf("first point = %+v", pts[0])
	}
	if pts[29].Timestamp != 1700000000000+29*300000 {
		t.Fatalf("last timestamp = %d", pts[29].Timestamp)
	}
	if !(pts[29].SumOpenInterestValue > pts[28].SumOpenInterestValue) {
		t.Fatal("series must stay oldest-first")
	}
}

// t2SymbolMux serves every endpoint fetchOneSymbol touches, counting
// depth calls so the gating test can prove the flag controls them.
func t2SymbolMux(depthHits *int64, depthStatus int) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/openInterest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"symbol":"TESTUSDT","openInterest":"1000","time":1700000000000}`)
	})
	mux.HandleFunc("/futures/data/openInterestHist", func(w http.ResponseWriter, r *http.Request) {
		// 1h variant feeds fetchOIHistory (deltas); 5m feeds the series.
		fmt.Fprint(w, oiHistJSON(30))
	})
	mux.HandleFunc("/futures/data/topLongShortPositionRatio", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"symbol":"TESTUSDT","longShortRatio":"1.10","timestamp":1700000000000}]`)
	})
	mux.HandleFunc("/fapi/v1/depth", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(depthHits, 1)
		if depthStatus != http.StatusOK {
			w.WriteHeader(depthStatus)
			fmt.Fprint(w, `{"code":-1000,"msg":"boom"}`)
			return
		}
		fmt.Fprint(w, `{"lastUpdateId":1,"bids":[["99.99","10"]],"asks":[["100.01","5"]]}`)
	})
	return mux
}

func TestFetchOneSymbolT2InputsGatedByFlag(t *testing.T) {
	// Flag OFF (default): no depth request, no series stored — the
	// pre-existing fetch behavior is byte-identical.
	var hitsOff int64
	serverOff := startT2TestServer(t, t2SymbolMux(&hitsOff, http.StatusOK))
	fOff := NewDataFetcher(FetcherConfig{BinanceURL: serverOff.URL})
	ssOff, err := fOff.fetchOneSymbol(context.Background(), "TESTUSDT", nil, nil, true)
	if err != nil {
		t.Fatalf("flag off fetchOneSymbol: %v", err)
	}
	if hitsOff != 0 {
		t.Fatalf("flag off must not call depth, got %d hits", hitsOff)
	}
	if len(ssOff.DepthBids) != 0 || len(ssOff.DepthAsks) != 0 || len(ssOff.OI5mHistory) != 0 {
		t.Fatalf("flag off snapshot carries t2 inputs: %+v", ssOff)
	}

	// Flag ON: both series land on the snapshot.
	var hitsOn int64
	serverOn := startT2TestServer(t, t2SymbolMux(&hitsOn, http.StatusOK))
	fOn := NewDataFetcher(FetcherConfig{BinanceURL: serverOn.URL, IncludeT2Inputs: true})
	ssOn, err := fOn.fetchOneSymbol(context.Background(), "TESTUSDT", nil, nil, true)
	if err != nil {
		t.Fatalf("flag on fetchOneSymbol: %v", err)
	}
	if hitsOn != 1 {
		t.Fatalf("flag on must call depth once per slow refresh, got %d", hitsOn)
	}
	if len(ssOn.DepthBids) != 1 || len(ssOn.DepthAsks) != 1 {
		t.Fatalf("depth not stored: %+v", ssOn)
	}
	if len(ssOn.OI5mHistory) != 30 {
		t.Fatalf("OI 5m series = %d points, want 30", len(ssOn.OI5mHistory))
	}

	// Flag ON but slow refresh off: still nothing (cadence gate).
	var hitsFast int64
	serverFast := startT2TestServer(t, t2SymbolMux(&hitsFast, http.StatusOK))
	fFast := NewDataFetcher(FetcherConfig{BinanceURL: serverFast.URL, IncludeT2Inputs: true})
	ssFast, err := fFast.fetchOneSymbol(context.Background(), "TESTUSDT", nil, nil, false)
	if err != nil {
		t.Fatalf("fast-cycle fetchOneSymbol: %v", err)
	}
	if hitsFast != 0 || len(ssFast.OI5mHistory) != 0 {
		t.Fatal("t2 inputs must ride the slow-derivative cadence only")
	}
}

func TestFetchOneSymbolT2DepthErrorStaysUnavailable(t *testing.T) {
	var hits int64
	server := startT2TestServer(t, t2SymbolMux(&hits, http.StatusInternalServerError))
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, IncludeT2Inputs: true})
	ss, err := f.fetchOneSymbol(context.Background(), "TESTUSDT", nil, nil, true)
	if err == nil {
		t.Fatal("a failed depth fetch must surface in the error count")
	}
	if ss == nil {
		t.Fatal("snapshot must still be returned (partial data)")
	}
	if len(ss.DepthBids) != 0 || len(ss.DepthAsks) != 0 {
		t.Fatalf("failed depth fetch must leave the book empty (unavailable): %+v", ss)
	}
	// The OI series is an independent fetch: it still lands.
	if len(ss.OI5mHistory) != 30 {
		t.Fatalf("OI series should survive a depth failure: %d points", len(ss.OI5mHistory))
	}
}

func TestMergeCarriedSlowDerivativesCarriesT2Inputs(t *testing.T) {
	cur := &Snapshot{Symbols: map[string]*SymbolSnapshot{
		"TESTUSDT": {
			Symbol:      "TESTUSDT",
			DepthBids:   []DepthLevel{{Price: 99.99, Qty: 10}},
			DepthAsks:   []DepthLevel{{Price: 100.01, Qty: 5}},
			OI5mHistory: []OIHistoryPoint{{Timestamp: 1, SumOpenInterestValue: 100}},
		},
	}}
	fresh := &Snapshot{Symbols: map[string]*SymbolSnapshot{
		"TESTUSDT": {Symbol: "TESTUSDT"},
	}}
	mergeCarriedSlowDerivatives(cur, fresh)
	got := fresh.Symbols["TESTUSDT"]
	if len(got.DepthBids) != 1 || len(got.DepthAsks) != 1 || len(got.OI5mHistory) != 1 {
		t.Fatalf("t2 inputs not carried: %+v", got)
	}
	// Carried slices must be copies, not aliases of the old snapshot.
	got.DepthBids[0].Qty = -1
	if cur.Symbols["TESTUSDT"].DepthBids[0].Qty == -1 {
		t.Fatal("carried depth aliases the previous snapshot")
	}
}
