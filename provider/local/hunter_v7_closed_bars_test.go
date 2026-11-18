package local

import (
	"math"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
)

func hourBars(t0 time.Time, closes ...float64) []datafetch.Kline {
	var out []datafetch.Kline
	for i, c := range closes {
		o := t0.Add(time.Duration(i) * time.Hour)
		out = append(out, datafetch.Kline{OpenTime: o.UnixMilli(), CloseTime: o.Add(time.Hour).UnixMilli() - 1, Open: c, High: c, Low: c, Close: c, Volume: 1})
	}
	return out
}

func TestV7ClosedBarSnapshotDropsFormingBars(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	at := t0.Add(2*time.Hour + 20*time.Minute) // third hour bar is forming
	ss := &datafetch.SymbolSnapshot{Symbol: "AUSDT", Price: 104, Timestamp: at,
		Klines: map[string][]datafetch.Kline{"1h": hourBars(t0, 100, 102, 104)}}
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{"AUSDT": ss}, CreatedAt: at.Add(time.Hour)}
	v := V7ClosedBarSnapshot(snap, at.Add(5*time.Hour))
	got := v.Symbols["AUSDT"]
	if len(got.Klines["1h"]) != 2 || !got.ClosedBarsOnly || got.Price != 104 {
		t.Fatalf("view: %d bars, closedOnly=%v, price=%v", len(got.Klines["1h"]), got.ClosedBarsOnly, got.Price)
	}
	if len(ss.Klines["1h"]) != 3 || ss.ClosedBarsOnly {
		t.Fatalf("input must not be modified")
	}
	// Reference precedence: symbol timestamp, else snapshot time, else ref.
	ss.Timestamp = time.Time{}
	if n := len(V7ClosedBarSnapshot(snap, at).Symbols["AUSDT"].Klines["1h"]); n != 3 {
		t.Fatalf("snapshot time (all three closed by then) must be used, got %d", n)
	}
	snap.CreatedAt = time.Time{}
	if n := len(V7ClosedBarSnapshot(snap, at).Symbols["AUSDT"].Klines["1h"]); n != 2 {
		t.Fatalf("ref fallback, got %d", n)
	}
}

// Native features: change_1h moves from forming-vs-last-closed to
// last-closed-vs-previous-closed; the current price stays live.
func TestV7ClosedBarNativeContext(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	at := t0.Add(2*time.Hour + 20*time.Minute)
	ss := &datafetch.SymbolSnapshot{Symbol: "AUSDT", Price: 104, Timestamp: at,
		Klines: map[string][]datafetch.Kline{"1h": hourBars(t0, 100, 102, 104)}}
	forming := buildSymbolContext("AUSDT", ss, &datafetch.Snapshot{})
	closed := buildSymbolContext("AUSDT", V7ClosedBarSnapshot(&datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{"AUSDT": ss}}, at).Symbols["AUSDT"], &datafetch.Snapshot{})
	if math.Abs(forming.Change1h-(104.0-102)/102*100) > 1e-9 || math.Abs(closed.Change1h-2) > 1e-9 {
		t.Fatalf("change_1h forming %v closed %v", forming.Change1h, closed.Change1h)
	}
	if closed.CurrentPrice != 104 {
		t.Fatalf("closed view must keep the live price, got %v", closed.CurrentPrice)
	}
}

func TestV7ClosedBarT2FramePrices(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	at := t0.Add(2*time.Hour + 20*time.Minute)
	ss := &datafetch.SymbolSnapshot{Symbol: "AUSDT", Price: 104.5, Timestamp: at,
		Klines: map[string][]datafetch.Kline{"5m": hourBars(t0, 100, 102, 104), "1h": hourBars(t0, 100, 102, 104)}}
	v := V7ClosedBarSnapshot(&datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{"AUSDT": ss}}, at).Symbols["AUSDT"]
	s := BuildV7T2SnapshotFromDatafetch(v)
	if s.K5.Price != 104.5 || s.K1h.Price != 104.5 || s.Price() != 104.5 {
		t.Fatalf("frame prices = %v / %v", s.K5.Price, s.K1h.Price)
	}
	if f := BuildV7T2SnapshotFromDatafetch(ss); f.K5.Price != 104 {
		t.Fatalf("forming default must keep the last bar close, got %v", f.K5.Price)
	}
}
