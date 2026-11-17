package local

import (
	"math"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

// t2AdapterDepthFixture: book around mark 100. In-band (±1%) totals:
// bids 99.99*10 + 99.95*20 = 2998.9; asks 100.01*5 + 100.05*10 = 1500.55.
// The 98.0 / 102.0 levels sit outside the band and must not count.
func t2AdapterDepthFixture() (bids, asks []datafetch.DepthLevel) {
	bids = []datafetch.DepthLevel{
		{Price: 99.99, Qty: 10}, {Price: 99.95, Qty: 20}, {Price: 98.0, Qty: 50},
	}
	asks = []datafetch.DepthLevel{
		{Price: 100.01, Qty: 5}, {Price: 100.05, Qty: 10}, {Price: 102.0, Qty: 50},
	}
	return bids, asks
}

// t2AdapterOIHistory: n points, notional value growing 1% per step from
// 10,000,000 (so Pct5m=1, Pct15m=1.01^3-1, Pct1h=1.01^12-1 in percent).
func t2AdapterOIHistory(n int) []datafetch.OIHistoryPoint {
	out := make([]datafetch.OIHistoryPoint, n)
	v := 10_000_000.0
	for i := range out {
		if i > 0 {
			v *= 1.01
		}
		out[i] = datafetch.OIHistoryPoint{
			Timestamp:            int64(i) * 300000,
			SumOpenInterest:      100000 + float64(i)*100,
			SumOpenInterestValue: v,
		}
	}
	return out
}

func t2AdapterBaseSnapshot() *datafetch.SymbolSnapshot {
	return &datafetch.SymbolSnapshot{
		Symbol:         "TESTUSDT",
		Price:          100,
		MarkPrice:      100,
		QuoteVolume24h: 20_000_000,
		PriceChange24h: 1.0,
		FundingRate:    0.0001,
		LongShortRatio: 1.2,
		OI:             100000,
		OIDelta1h:      0.5,
		Klines:         map[string][]datafetch.Kline{},
	}
}

func TestV7T2AdapterDepthAndOI5mWiring(t *testing.T) {
	ss := t2AdapterBaseSnapshot()
	ss.DepthBids, ss.DepthAsks = t2AdapterDepthFixture()
	ss.OI5mHistory = t2AdapterOIHistory(30)

	ts := BuildV7T2SnapshotFromDatafetch(ss)
	if ts == nil {
		t.Fatal("nil snapshot")
	}
	// Depth: available, band totals and spread per spec section 2.7.
	if !ts.Depth.Available {
		t.Fatal("depth must be available when the snapshot carries a book")
	}
	if math.Abs(ts.Depth.BidTotal-2998.9) > 1e-6 || math.Abs(ts.Depth.AskTotal-1500.55) > 1e-6 {
		t.Fatalf("band totals = %v/%v", ts.Depth.BidTotal, ts.Depth.AskTotal)
	}
	if math.Abs(ts.Depth.Ratio-2998.9/1500.55) > 1e-9 {
		t.Fatalf("ratio = %v", ts.Depth.Ratio)
	}
	if math.Abs(ts.Depth.SpreadBps-2.0) > 1e-9 {
		t.Fatalf("spread_bps = %v, want 2.0", ts.Depth.SpreadBps)
	}
	// OI: 5m series drives all three pcts (spec section 2.4).
	if !ts.OI5mAvailable || !ts.OIPct1hFrom5mSeries {
		t.Fatal("5m OI series must be marked available")
	}
	if math.Abs(ts.OI.Pct5m-1.0) > 1e-9 {
		t.Fatalf("oi_5m_pct = %v, want 1.0", ts.OI.Pct5m)
	}
	if want := (math.Pow(1.01, 3) - 1) * 100; math.Abs(ts.OI.Pct15m-want) > 1e-9 {
		t.Fatalf("oi_15m_pct = %v, want %v", ts.OI.Pct15m, want)
	}
	if want := (math.Pow(1.01, 12) - 1) * 100; math.Abs(ts.OI.Pct1h-want) > 1e-9 {
		t.Fatalf("oi_1h_pct = %v, want %v (from the 5m series, not the hourly fallback)", ts.OI.Pct1h, want)
	}
	if want := 10_000_000 * math.Pow(1.01, 29); math.Abs(ts.OI.NotionalUSD-want) > 1e-3 {
		t.Fatalf("oi_notional_usd = %v, want series tail %v", ts.OI.NotionalUSD, want)
	}
}

func TestV7T2AdapterMissingInputsStayUnavailable(t *testing.T) {
	ts := BuildV7T2SnapshotFromDatafetch(t2AdapterBaseSnapshot())
	if ts.Depth.Available {
		t.Fatal("no book in snapshot -> depth must stay unavailable")
	}
	if ts.OI5mAvailable || ts.OIPct1hFrom5mSeries {
		t.Fatal("no 5m series -> OI5mAvailable must stay false")
	}
	// Pre-existing hourly fallback is unchanged.
	if ts.OI.Pct1h != 0.5 {
		t.Fatalf("fallback oi_1h_pct = %v, want OIDelta1h 0.5", ts.OI.Pct1h)
	}
	if ts.OI.NotionalUSD != 100000*100 {
		t.Fatalf("fallback notional = %v, want OI*markPrice", ts.OI.NotionalUSD)
	}
}

func TestV7T2AdapterShortOI5mSeriesFailsClosed(t *testing.T) {
	ss := t2AdapterBaseSnapshot()
	ss.OI5mHistory = t2AdapterOIHistory(10) // < 13 selected points
	ts := BuildV7T2SnapshotFromDatafetch(ss)
	if ts.OI5mAvailable {
		t.Fatal("a series too short for the 12-bar oi_1h lookback must fail closed")
	}
	if ts.OI.Pct1h != 0.5 {
		t.Fatalf("short series must fall back to the hourly figure, got %v", ts.OI.Pct1h)
	}
}

func TestV7T2AdapterOIValueTailSelection(t *testing.T) {
	// Value field empty for the first 17 points, populated (1% growth)
	// for the last 13: the selected series is the 13-point positive
	// tail — exactly at the availability threshold, pcts computed over
	// the tail (production-verified selection, M3 report section 3.6).
	ss := t2AdapterBaseSnapshot()
	hist := t2AdapterOIHistory(30)
	for i := 0; i < 17; i++ {
		hist[i].SumOpenInterestValue = 0
	}
	ss.OI5mHistory = hist
	ts := BuildV7T2SnapshotFromDatafetch(ss)
	if !ts.OI5mAvailable {
		t.Fatal("13-point positive value tail must be available")
	}
	if math.Abs(ts.OI.Pct5m-1.0) > 1e-9 || math.Abs(ts.OI.Pct1h-(math.Pow(1.01, 12)-1)*100) > 1e-9 {
		t.Fatalf("tail pcts = %v / %v", ts.OI.Pct5m, ts.OI.Pct1h)
	}
	// One point shorter (12-point tail) -> below threshold, closed.
	hist2 := t2AdapterOIHistory(30)
	for i := 0; i < 18; i++ {
		hist2[i].SumOpenInterestValue = 0
	}
	ss.OI5mHistory = hist2
	if ts := BuildV7T2SnapshotFromDatafetch(ss); ts.OI5mAvailable {
		t.Fatal("12-point positive tail must fail closed")
	}
}

// t2AdapterMomentumSnapshot builds a datafetch snapshot whose adapted
// form satisfies the momentum_continuation_long gates (spec section 4.5)
// — the family that hard-gates on Depth.Available. The twin snapshot
// without depth/OI inputs must emit nothing (pre-fix behavior).
func t2AdapterMomentumSnapshot(withInputs bool) *datafetch.SymbolSnapshot {
	// 15m closes: [+0.3,+0.3,+0.3,-0.3] repeating from 88 -> uptrend,
	// RSI ~79 (<= 82), price above EMA20, EMA20 above EMA50.
	var closes15 []float64
	c := 88.0
	steps := []float64{0.3, 0.3, 0.3, -0.3}
	for i := 0; i < 80; i++ {
		c += steps[i%4]
		closes15 = append(closes15, c)
	}
	price := closes15[len(closes15)-1]
	mkBars := func(closes []float64, vol, taker float64) []datafetch.Kline {
		bars := make([]datafetch.Kline, len(closes))
		for i, cl := range closes {
			bars[i] = datafetch.Kline{
				Open: cl, High: cl + 0.05, Low: cl - 0.05, Close: cl,
				Volume: vol, TakerBuy: taker, CloseTime: int64(i),
			}
		}
		return bars
	}
	flat := func(n int, v float64) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = v
		}
		return out
	}
	rising1h := make([]float64, 80)
	for i := range rising1h {
		rising1h[i] = price - 5 + float64(i)*(5.0/79)
	}

	ss := &datafetch.SymbolSnapshot{
		Symbol:         "TESTUSDT",
		Price:          price,
		MarkPrice:      price,
		QuoteVolume24h: 20_000_000,
		PriceChange24h: 20,
		FundingRate:    0.0001,
		OI:             130000,
		OIDelta1h:      1.0,
		OISpikeData:    []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5},
		Klines: map[string][]datafetch.Kline{
			"5m":  mkBars(flat(120, price), 1000, 500),
			"15m": mkBars(closes15, 1000, 560), // taker share 0.56 on every bar
			"1h":  mkBars(rising1h, 1000, 500),
			"4h":  mkBars(flat(60, price), 1000, 500),
		},
	}
	if withInputs {
		ss.DepthBids = []datafetch.DepthLevel{
			{Price: price - 0.01, Qty: 10}, {Price: price - 0.05, Qty: 20},
		}
		ss.DepthAsks = []datafetch.DepthLevel{
			{Price: price + 0.01, Qty: 5}, {Price: price + 0.05, Qty: 10},
		}
		ss.OI5mHistory = t2AdapterOIHistory(30)
	}
	return ss
}

func TestV7T2AdapterEndToEndMomentumNeedsBothInputs(t *testing.T) {
	cfg := &V7T2Config{Enabled: true}

	withInputs := BuildV7T2SnapshotFromDatafetch(t2AdapterMomentumSnapshot(true))
	sig := EvaluateV7T2Snapshot(withInputs, cfg, nil)
	if sig == nil {
		t.Fatal("with depth + 5m OI supplied, the momentum fixture must emit")
	}
	if sig.SetupType != "momentum_continuation_long" || sig.Direction != "LONG" {
		t.Fatalf("got %s/%s", sig.SetupType, sig.Direction)
	}

	without := BuildV7T2SnapshotFromDatafetch(t2AdapterMomentumSnapshot(false))
	if sig := EvaluateV7T2Snapshot(without, cfg, nil); sig != nil {
		t.Fatalf("without depth/OI inputs the same snapshot must emit nothing, got %s", sig.SetupType)
	}
}
