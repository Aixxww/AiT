package replay

import "testing"

// base is aligned to 1m/5m/15m/1h/4h boundaries (multiple of 4h in ms).
const base int64 = 3600000 * 1000

// synthBars builds n consecutive 1-minute bars starting at base.
// Bar i: open=100+i, high=101+i, low=99+i, close=100.5+i.
func synthBars(n int) []Bar {
	bars := make([]Bar, n)
	for i := 0; i < n; i++ {
		f := float64(i)
		bars[i] = Bar{
			OpenTime: base + int64(i)*MinuteMs,
			Open:     100 + f, High: 101 + f, Low: 99 + f, Close: 100.5 + f,
		}
	}
	return bars
}

func TestAggregateOHLC(t *testing.T) {
	bars := synthBars(10)
	got := Aggregate(bars, Interval5m)
	if len(got) != 2 {
		t.Fatalf("Aggregate(10x1m, 5m) = %d bars, want 2", len(got))
	}
	// First bucket folds bars 0..4: open 100, high 101+4=105, low 99, close 104.5.
	want0 := Bar{OpenTime: base, Open: 100, High: 105, Low: 99, Close: 104.5}
	if got[0] != want0 {
		t.Errorf("bucket0 = %+v, want %+v", got[0], want0)
	}
	// Second bucket folds bars 5..9.
	want1 := Bar{OpenTime: base + 5*MinuteMs, Open: 105, High: 110, Low: 104, Close: 109.5}
	if got[1] != want1 {
		t.Errorf("bucket1 = %+v, want %+v", got[1], want1)
	}
	// 15m aggregation of 10 minutes is a single (partial) bucket.
	got15 := Aggregate(bars, Interval15m)
	if len(got15) != 1 || got15[0].Open != 100 || got15[0].Close != 109.5 || got15[0].High != 110 || got15[0].Low != 99 {
		t.Errorf("Aggregate(10x1m, 15m) = %+v, want single folded bucket", got15)
	}
}

// TestSnapshotNoLookahead is the core point-in-time guarantee: a
// snapshot at t contains only bars fully closed at or before t, and
// appending future bars never changes it.
func TestSnapshotNoLookahead(t *testing.T) {
	bars := synthBars(60) // one hour of 1m bars

	// At t = base+30m exactly: the 5m bucket ending at base+30m has
	// just closed (close <= t counts), the next one has not.
	at30 := SnapshotAt(bars, base+30*MinuteMs, Interval5m)
	if len(at30) != 6 {
		t.Fatalf("SnapshotAt(+30m, 5m) = %d bars, want 6", len(at30))
	}
	for _, b := range at30 {
		if b.CloseTime(Interval5m) > base+30*MinuteMs {
			t.Errorf("bar %+v closes after t", b)
		}
	}
	// One millisecond earlier the last bucket is still open.
	at29 := SnapshotAt(bars, base+30*MinuteMs-1, Interval5m)
	if len(at29) != 5 {
		t.Errorf("SnapshotAt(+30m-1ms, 5m) = %d bars, want 5", len(at29))
	}
	// Prefix stability: snapshot over a truncated series equals the
	// snapshot over the full series (future bars cannot leak in).
	truncated := SnapshotAt(bars[:31], base+30*MinuteMs, Interval5m)
	if len(truncated) != len(at30) {
		t.Fatalf("prefix stability: truncated snapshot has %d bars, full has %d", len(truncated), len(at30))
	}
	for i := range at30 {
		if truncated[i] != at30[i] {
			t.Errorf("prefix stability: bar %d differs: %+v vs %+v", i, truncated[i], at30[i])
		}
	}
	// Mutating future bars must not change the snapshot either.
	mutated := synthBars(60)
	for i := 31; i < 60; i++ {
		mutated[i].High = 99999
		mutated[i].Close = 99999
	}
	after := SnapshotAt(mutated, base+30*MinuteMs, Interval5m)
	for i := range at30 {
		if after[i] != at30[i] {
			t.Errorf("future mutation leaked into snapshot at bar %d: %+v vs %+v", i, after[i], at30[i])
		}
	}
	// Before the first bucket closes there is nothing.
	if got := SnapshotAt(bars, base+5*MinuteMs-1, Interval5m); len(got) != 0 {
		t.Errorf("SnapshotAt(+5m-1ms, 5m) = %d bars, want 0", len(got))
	}
	// Higher timeframes: at +1h, exactly one 1h bar and zero 4h bars.
	snaps := Snapshots(bars, base+60*MinuteMs)
	if len(snaps[Interval1h]) != 1 {
		t.Errorf("Snapshots(+1h)[1h] = %d bars, want 1", len(snaps[Interval1h]))
	}
	if len(snaps[Interval4h]) != 0 {
		t.Errorf("Snapshots(+1h)[4h] = %d bars, want 0", len(snaps[Interval4h]))
	}
	if len(snaps[Interval15m]) != 4 {
		t.Errorf("Snapshots(+1h)[15m] = %d bars, want 4", len(snaps[Interval15m]))
	}
}
