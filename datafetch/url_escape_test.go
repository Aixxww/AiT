package datafetch

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Fix 01 (Tier A P5): per-symbol URLs escape the symbol, so a non-ASCII
// contract name (e.g. 牛来USDT) is sent as percent-encoded ASCII and
// decodes back to the exact symbol on the server.
func TestPerSymbolRequestsEscapeNonASCIISymbol(t *testing.T) {
	const sym = "牛来USDT"
	var mu sync.Mutex
	seen := map[string]string{} // path -> raw query
	mux := t2SymbolMux(new(int64), http.StatusOK)
	wrap := http.NewServeMux()
	wrap.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = r.URL.RawQuery
		mu.Unlock()
		if got := r.URL.Query().Get("symbol"); got != sym {
			t.Errorf("%s: server decoded symbol %q, want %q", r.URL.Path, got, sym)
		}
		if r.URL.Path == "/fapi/v1/klines" {
			fmt.Fprint(w, `[[1700000000000,"1","2","0.5","1.5","10",1700000059999,"15","3","5","7.5","0"]]`)
			return
		}
		mux.ServeHTTP(w, r)
	})
	server := startT2TestServer(t, wrap)
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL, IncludeT2Inputs: true})
	if _, err := f.fetchOneSymbol(context.Background(), sym, nil, []KlineInterval{{Interval: "1m", Limit: 1}}, true); err != nil {
		t.Fatalf("fetchOneSymbol(%s): %v", sym, err)
	}
	want := []string{"/fapi/v1/openInterest", "/futures/data/openInterestHist", "/futures/data/topLongShortPositionRatio", "/fapi/v1/depth", "/fapi/v1/klines"}
	for _, p := range want {
		raw, ok := seen[p]
		if !ok {
			t.Fatalf("no request to %s", p)
		}
		if !strings.Contains(raw, "symbol=%E7%89%9B%E6%9D%A5USDT") {
			t.Fatalf("%s raw query %q: symbol not percent-encoded", p, raw)
		}
		for i := 0; i < len(raw); i++ {
			if raw[i] >= 0x80 {
				t.Fatalf("%s raw query %q carries non-ASCII bytes", p, raw)
			}
		}
	}
}

// ASCII symbols are byte-identical after escaping (no behaviour change for
// every symbol that already worked).
func TestPerSymbolRequestsASCIISymbolUnchanged(t *testing.T) {
	var got []string
	mux := http.NewServeMux()
	mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.RawQuery)
		fmt.Fprint(w, `[]`)
	})
	server := startT2TestServer(t, mux)
	f := NewDataFetcher(FetcherConfig{BinanceURL: server.URL})
	if _, err := f.FetchKlines(context.Background(), "1000PEPEUSDT", "5m", 3); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "symbol=1000PEPEUSDT&interval=5m&limit=3" {
		t.Fatalf("raw query %v", got)
	}
}
