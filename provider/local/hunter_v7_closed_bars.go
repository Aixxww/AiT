package local

import (
	"time"

	"github.com/Aixxww/AiT/datafetch"
)

// U10: closed-bar features (plan §6.2 "Data: forming bar"). The datafetch
// klines end with the still-forming bar. V7ClosedBarSnapshot returns a view
// whose klines hold closed bars only: a bar is forming when its CloseTime is
// at or after the time its symbol's data was taken (SymbolSnapshot.Timestamp,
// else Snapshot.CreatedAt, else ref). The live price stays in Price, and the
// view sets ClosedBarsOnly so consumers take the current price from it (the
// forming bar is used for price only). The input is not modified.
func V7ClosedBarSnapshot(snap *datafetch.Snapshot, ref time.Time) *datafetch.Snapshot {
	if snap == nil {
		return nil
	}
	out := *snap
	out.Symbols = make(map[string]*datafetch.SymbolSnapshot, len(snap.Symbols))
	for sym, ss := range snap.Symbols {
		if ss == nil {
			out.Symbols[sym] = nil
			continue
		}
		at := ref
		if !snap.CreatedAt.IsZero() {
			at = snap.CreatedAt
		}
		if !ss.Timestamp.IsZero() {
			at = ss.Timestamp
		}
		c := *ss
		c.ClosedBarsOnly = true
		c.Klines = make(map[string][]datafetch.Kline, len(ss.Klines))
		for tf, bars := range ss.Klines {
			c.Klines[tf] = v7ClosedKlines(bars, at.UnixMilli())
		}
		out.Symbols[sym] = &c
	}
	return &out
}

// v7ClosedKlines drops trailing bars that had not closed at atMs.
func v7ClosedKlines(bars []datafetch.Kline, atMs int64) []datafetch.Kline {
	n := len(bars)
	for n > 0 && bars[n-1].CloseTime >= atMs {
		n--
	}
	return bars[:n:n]
}
