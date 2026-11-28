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
		at := v7SnapshotRefTime(ss, snap, ref)
		c := *ss
		c.ClosedBarsOnly = true
		c.Klines = make(map[string][]datafetch.Kline, len(ss.Klines))
		for tf, bars := range ss.Klines {
			c.Klines[tf] = v7ClosedKlinesForTF(bars, tf, at.UnixMilli())
		}
		out.Symbols[sym] = &c
	}
	return &out
}

// v7SnapshotRefTime resolves the point-in-time reference used to distinguish
// closed bars from the trailing forming bar: SymbolSnapshot.Timestamp, else
// Snapshot.CreatedAt, else fallback.
func v7SnapshotRefTime(ss *datafetch.SymbolSnapshot, snap *datafetch.Snapshot, fallback time.Time) time.Time {
	at := fallback
	if snap != nil && !snap.CreatedAt.IsZero() {
		at = snap.CreatedAt
	}
	if ss != nil && !ss.Timestamp.IsZero() {
		at = ss.Timestamp
	}
	return at
}

// v7ClosedKlines drops trailing bars that had not closed at atMs.
func v7ClosedKlines(bars []datafetch.Kline, atMs int64) []datafetch.Kline {
	return v7ClosedKlinesForTF(bars, "", atMs)
}

// v7ClosedKlinesForTF drops trailing bars that had not closed at atMs. When
// CloseTime is populated (> 0) a bar is forming if CloseTime >= atMs; when
// CloseTime is 0 but OpenTime and tf interval are known, a bar is forming if
// OpenTime + interval > atMs.
func v7ClosedKlinesForTF(bars []datafetch.Kline, tf string, atMs int64) []datafetch.Kline {
	ivMs := v7TimeframeIntervalMs(tf)
	n := len(bars)
	for n > 0 {
		b := bars[n-1]
		if b.CloseTime > 0 {
			if b.CloseTime >= atMs {
				n--
				continue
			}
			break
		}
		if b.OpenTime > 0 && ivMs > 0 && b.OpenTime+ivMs > atMs {
			n--
			continue
		}
		break
	}
	return bars[:n:n]
}
