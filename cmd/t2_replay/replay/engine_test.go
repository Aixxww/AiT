package replay

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// studyDir locates the entry-study cache relative to this package
// (~/workspace/ait/cmd/t2_replay/replay -> ~/workspace/ait-t2-data).
// Tests that need the real cache skip when it is absent.
func studyDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "ait-t2-data", "entry_study")
	if _, err := os.Stat(filepath.Join(dir, "events.json")); err != nil {
		t.Skipf("entry-study cache not present at %s", dir)
	}
	return dir
}

// Golden values from the Python reference run (shadow_exec.decide +
// sim_live_replay.simulate over the study cache), events 0-2.
var golden = []struct {
	consumed float64
	intake   string
	outcome  string
	netR     float64
}{
	{0.24573780129335665, "FILLED", "TIME_STOP", 1.3944},
	{0.33604574824642913, "FILLED", "TIME_STOP", 0.1091},
	{0.047261635478796284, "FILLED", "STOP", -1.0401},
}

func TestGoldenConsumedAndSettlement(t *testing.T) {
	dir := studyDir(t)
	events, err := LoadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range golden {
		bars, err := LoadBars(dir, i)
		if err != nil {
			t.Fatal(err)
		}
		dec := Decide(events[i], bars)
		if dec.Consumed == nil {
			t.Fatalf("event %d: Decide returned NO_DATA", i)
		}
		if math.Abs(*dec.Consumed-g.consumed) > 1e-9 {
			t.Errorf("event %d: consumed = %.15f, want %.15f", i, *dec.Consumed, g.consumed)
		}
		res := Simulate(events[i], bars)
		if res.Intake != g.intake || res.Outcome != g.outcome {
			t.Errorf("event %d: intake/outcome = %s/%s, want %s/%s", i, res.Intake, res.Outcome, g.intake, g.outcome)
		}
		if math.Abs(res.NetR-g.netR) > 1e-3 {
			t.Errorf("event %d: net_r = %.4f, want %.4f", i, res.NetR, g.netR)
		}
	}
}

// TestFullReplayBands runs the whole 694-event cache and checks the
// Arm A aggregates against the Python reference within the M1
// acceptance tolerance (±10% per band net R, ±6R absolute for the
// negative band; ±10% on fill counts).
func TestFullReplayBands(t *testing.T) {
	dir := studyDir(t)
	events, err := LoadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RunAll(dir, events)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NEvents != 694 || rep.NOk != 694 {
		t.Fatalf("events ok = %d/%d, want 694/694", rep.NOk, rep.NEvents)
	}
	if rep.All.Fills != 440 {
		t.Errorf("all fills = %d, want 440", rep.All.Fills)
	}
	if math.Abs(rep.All.NetR-(-66.67)) > 6.67 {
		t.Errorf("all net_r = %.2f, want -66.67 ±10%%", rep.All.NetR)
	}
	want := map[string]struct {
		n, fills int
		netR     float64
		tol      float64
	}{
		"neg":    {133, 133, -58.57, 6.0},
		"b0_05":  {326, 227, 7.46, 0.75},
		"b05_10": {98, 20, 4.47, 0.45},
		"ge1":    {59, 0, 0.0, 0.01},
	}
	for _, b := range rep.Bands {
		w := want[b.Name]
		if b.N != w.n {
			t.Errorf("band %s: n = %d, want %d", b.Name, b.N, w.n)
		}
		if math.Abs(float64(b.Fills-w.fills)) > math.Max(1, 0.10*float64(w.fills)) {
			t.Errorf("band %s: fills = %d, want %d ±10%%", b.Name, b.Fills, w.fills)
		}
		if math.Abs(b.NetR-w.netR) > w.tol {
			t.Errorf("band %s: net_r = %.2f, want %.2f ±%.2f", b.Name, b.NetR, w.netR, w.tol)
		}
	}
}

// TestLimitFillScanProximity locks the same-bar proximity rule: when
// entry and TP1 are both touched in one bar, the fill is a RUN_AWAY
// if the bar open is closer to TP1 than to the entry.
func TestLimitFillScanProximity(t *testing.T) {
	ev := Event{
		Direction: "LONG", Setup: "bb_oi_accumulation", // slow family: 120m TTL
		TrigMs: base, Entry: 100, Stop: 95, TP1: 110, TP2: 115,
	}
	mkBars := func(open float64) []Bar {
		bars := make([]Bar, 40)
		for i := range bars {
			bars[i] = Bar{OpenTime: base + int64(i+1)*MinuteMs, Open: 105, High: 106, Low: 104, Close: 105}
		}
		// Bar at i_act (=1, first bar at/after trig+60s) touches both
		// entry (low<=100) and TP1 (high>=110).
		bars[1] = Bar{OpenTime: base + 2*MinuteMs, Open: open, High: 111, Low: 99, Close: 105}
		return bars
	}
	// Open 108: |108-110|=2 < |108-100|=8 -> RUN_AWAY.
	if st, _ := LimitFillScan(ev, mkBars(108), 1); st != "RUN_AWAY" {
		t.Errorf("open near TP1: status = %s, want RUN_AWAY", st)
	}
	// Open 101: |101-110|=9 > |101-100|=1 -> FILLED at index 1.
	if st, idx := LimitFillScan(ev, mkBars(101), 1); st != "FILLED" || idx != 1 {
		t.Errorf("open near entry: status = %s idx %d, want FILLED/1", st, idx)
	}
	// INVALIDATED branch: with ordinary plan geometry (stop beyond
	// entry) a stop touch implies an entry touch in the same bar, so
	// this branch never fires on the 694-event reference set (0
	// occurrences). Lock the branch semantics with an inverted-stop
	// synthetic: stop at 102, bar low 101 touches stop but not entry.
	inv := ev
	inv.Stop = 102
	bars := mkBars(101)
	bars[1] = Bar{OpenTime: base + 2*MinuteMs, Open: 101, High: 101.5, Low: 101, Close: 101}
	if st, _ := LimitFillScan(inv, bars, 1); st != "INVALIDATED" {
		t.Errorf("stop touched, entry not: status = %s, want INVALIDATED", st)
	}
}
