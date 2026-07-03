package datafetch

import (
	"fmt"
	"testing"
	"time"
)

func TestHandleAggTradeUsesRollingBuySellVolumeRatio(t *testing.T) {
	store := NewStore()
	snap := &Snapshot{
		Symbols: map[string]*SymbolSnapshot{
			"TESTUSDT": {Symbol: "TESTUSDT"},
		},
		CreatedAt: time.Now(),
	}
	store.Swap(snap)
	wm := NewWSManager("wss://fstream.binance.com", store, 1)

	base := time.Unix(1_700_000_000, 0)
	wm.handleAggTrade(snap, aggTradeJSON("TESTUSDT", 100, 1000, false, base))
	if got := snap.Symbols["TESTUSDT"].TakerBuyRatio; got != 1 {
		t.Fatalf("ratio after only taker buys = %v, want 1", got)
	}

	wm.handleAggTrade(snap, aggTradeJSON("TESTUSDT", 100, 1000, true, base.Add(time.Second)))
	if got := snap.Symbols["TESTUSDT"].TakerBuyRatio; got != 0.5 {
		t.Fatalf("ratio after equal buy/sell volume = %v, want 0.5", got)
	}

	wm.handleAggTrade(snap, aggTradeJSON("TESTUSDT", 101, 3000, true, base.Add(2*time.Second)))
	if got := snap.Symbols["TESTUSDT"].TakerBuyRatio; got < 0 || got > 1 {
		t.Fatalf("ratio = %v, want clamped to [0,1]", got)
	}
	if got := snap.Symbols["TESTUSDT"].Price; got != 101 {
		t.Fatalf("price = %v, want latest trade price 101", got)
	}
}

func TestTakerTradeWindowPrunesExpiredBuckets(t *testing.T) {
	w := newTakerTradeWindow(time.Minute)
	base := time.Unix(1_700_000_000, 0)
	w.Add(base, 100, true)
	w.Add(base.Add(30*time.Second), 100, false)
	if got := w.Ratio(); got != 0.5 {
		t.Fatalf("initial ratio = %v, want 0.5", got)
	}

	w.Add(base.Add(2*time.Minute), 300, false)
	if got := w.Ratio(); got != 0 {
		t.Fatalf("ratio after pruning old buys = %v, want 0", got)
	}
}

func aggTradeJSON(symbol string, price, qty float64, buyerIsMaker bool, ts time.Time) []byte {
	return []byte(fmt.Sprintf(`{"s":%q,"p":"%.8f","q":"%.8f","m":%t,"T":%d}`,
		symbol, price, qty, buyerIsMaker, ts.UnixMilli()))
}
