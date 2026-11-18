package trader

// Data-free synthetic-path tests for the U0 model-A walk (u0fWalkA).

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
)

// synthLong builds 1m bars starting at t0 from (low, high, close) triples.
func synthLong(t0 int64, hlc [][3]float64) []replay.Bar {
	bars := make([]replay.Bar, len(hlc))
	prev := hlc[0][2]
	for i, x := range hlc {
		bars[i] = replay.Bar{OpenTime: t0 + int64(i)*60000, Open: prev, Low: x[0], High: x[1], Close: x[2]}
		prev = x[2]
	}
	return bars
}

func walkLong(t *testing.T, E, S, tp float64, lev int, bars []replay.Bar) u0fRow {
	t.Helper()
	defer func() { protectionNow = time.Now }()
	d := math.Abs(E - S)
	u := func(p float64) float64 { return (p - E) / d }
	fav := func(b replay.Bar) float64 { return b.High }
	adv := func(b replay.Bar) float64 { return b.Low }
	hit := func(px, lvl float64, favourable bool) bool {
		if favourable {
			return px >= lvl
		}
		return px <= lvl
	}
	var row u0fRow
	u0fWalkA("SYNUSDT", true, 1, "long", E, S, 0, 0, tp, lev, bars, 0, u, fav, adv, hit, d, &row)
	return row
}

const synthT0 = int64(1_790_000_000_000)

// fee in R for a full round trip at entry E and exit X: 0.05% per side.
func synthFeeR(E, X, d float64) float64 { return 0.0005 * (E + X) / d }

func TestU0WalkAExchangeStopFirst(t *testing.T) {
	// entry 100, stop 98 (2%), tp 104. Bar 1 trades through the stop.
	bars := synthLong(synthT0, [][3]float64{{99.8, 100.2, 100}, {97.5, 100.1, 97.9}})
	row := walkLong(t, 100, 98, 104, 10, bars)
	want := -1 - synthFeeR(100, 98, 2)
	if row.AExit != "stop" || math.Abs(row.A-want) > 1e-9 {
		t.Fatalf("exit=%s A=%.6f, want stop %.6f", row.AExit, row.A, want)
	}
}

func TestU0WalkAExchangeTakeProfit(t *testing.T) {
	// A clean run straight through the take-profit in one bar.
	bars := synthLong(synthT0, [][3]float64{{99.9, 100.1, 100}, {99.95, 104.5, 104.2}})
	row := walkLong(t, 100, 98, 104, 10, bars)
	want := 2 - synthFeeR(100, 104, 2)
	if row.AExit != "exchange_tp" || math.Abs(row.A-want) > 1e-9 {
		t.Fatalf("exit=%s A=%.6f, want exchange_tp %.6f", row.AExit, row.A, want)
	}
}

func TestU0WalkATP0PartialThenMovedStop(t *testing.T) {
	// 10x: TP0 fires at +8% ROE (+0.8% price) on a close; the rebuild moves
	// the remainder's stop to entry +0.10%; the next dip exits there.
	bars := synthLong(synthT0, [][3]float64{
		{99.9, 100.1, 100.0},
		{100.0, 100.9, 100.85}, // close +0.85% -> TP0 partial (35%)
		{100.0, 100.4, 100.05}, // low 100.0 <= 100.1 -> moved stop
	})
	row := walkLong(t, 100, 98, 104, 10, bars)
	if len(row.AActions) == 0 || row.AActions[0] != "tp0" {
		t.Fatalf("actions = %v, want tp0 first", row.AActions)
	}
	if row.AExit != "moved_stop" || row.AMoves == 0 {
		t.Fatalf("exit=%s moves=%d, want moved_stop", row.AExit, row.AMoves)
	}
	// 35% at 100.85 and 65% at 100.1, minus fees: a small positive R.
	gross := 0.35*0.425 + 0.65*0.05
	if row.A <= 0 || row.A >= gross {
		t.Fatalf("A=%.4f, want 0 < A < gross %.4f (fees deducted)", row.A, gross)
	}
}

func TestU0WalkAHoldsToWindowEnd(t *testing.T) {
	// No order or protector rule fires: marked at the last close.
	var hlc [][3]float64
	for i := 0; i < 30; i++ {
		hlc = append(hlc, [3]float64{99.95, 100.05, 100.02})
	}
	row := walkLong(t, 100, 98, 104, 10, synthLong(synthT0, hlc))
	if !strings.HasPrefix(row.AExit, "timeout") {
		t.Fatalf("exit=%s, want timeout", row.AExit)
	}
	want := 0.01 - synthFeeR(100, 100.02, 2)
	if math.Abs(row.A-want) > 1e-9 {
		t.Fatalf("A=%.6f, want %.6f", row.A, want)
	}
}
