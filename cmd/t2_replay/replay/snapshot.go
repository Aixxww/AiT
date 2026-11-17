package replay

// Point-in-time multi-timeframe snapshots over 1-minute bars.
//
// Semantics: at time t the snapshot for an interval contains exactly
// the interval bars that had FULLY closed at or before t, built only
// from 1-minute bars whose own close (OpenTime+1m) is <= t. A partially
// formed trailing bucket is never emitted, so a snapshot taken at t is
// a strict prefix property: appending bars after t can never change it.
// This is the no-lookahead foundation for the later milestones
// (structure geometry / prefilter features read snapshots at signal
// time and must not see the future).

// Snapshot intervals in milliseconds.
const (
	Interval5m  int64 = 5 * MinuteMs
	Interval15m int64 = 15 * MinuteMs
	Interval1h  int64 = 60 * MinuteMs
	Interval4h  int64 = 240 * MinuteMs
)

// SnapshotIntervals lists the supported aggregation intervals.
var SnapshotIntervals = []int64{Interval5m, Interval15m, Interval1h, Interval4h}

// Aggregate groups consecutive 1-minute bars into epoch-aligned buckets
// of intervalMs (bucket start = OpenTime - OpenTime%intervalMs) and
// folds each bucket into a single OHLC bar. Input must be sorted by
// OpenTime (LoadBars guarantees this). Buckets are emitted in time
// order; a bucket is emitted even if some of its 1-minute constituents
// are missing from the input (gaps fold what exists).
func Aggregate(bars []Bar, intervalMs int64) []Bar {
	if intervalMs <= 0 || len(bars) == 0 {
		return nil
	}
	out := make([]Bar, 0, len(bars)*int(MinuteMs)/int(intervalMs)+1)
	var cur *Bar
	for _, b := range bars {
		start := b.OpenTime - b.OpenTime%intervalMs
		if cur == nil || cur.OpenTime != start {
			out = append(out, Bar{OpenTime: start, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close})
			cur = &out[len(out)-1]
			continue
		}
		if b.High > cur.High {
			cur.High = b.High
		}
		if b.Low < cur.Low {
			cur.Low = b.Low
		}
		cur.Close = b.Close
	}
	return out
}

// SnapshotAt returns the intervalMs bars fully closed at or before t:
// only 1-minute bars with close <= t are used, and only buckets whose
// own close (bucketStart+intervalMs) is <= t are returned.
func SnapshotAt(bars []Bar, t int64, intervalMs int64) []Bar {
	if intervalMs <= 0 {
		return nil
	}
	closed := make([]Bar, 0, len(bars))
	for _, b := range bars {
		if b.CloseTime(MinuteMs) <= t {
			closed = append(closed, b)
		}
	}
	agg := Aggregate(closed, intervalMs)
	out := agg[:0]
	for _, b := range agg {
		if b.CloseTime(intervalMs) <= t {
			out = append(out, b)
		}
	}
	return out
}

// Snapshots returns SnapshotAt for every supported interval, keyed by
// interval length in milliseconds.
func Snapshots(bars []Bar, t int64) map[int64][]Bar {
	out := make(map[int64][]Bar, len(SnapshotIntervals))
	for _, iv := range SnapshotIntervals {
		out[iv] = SnapshotAt(bars, t, iv)
	}
	return out
}
