package trader

// U0-F harness (redesign plan U0-F / U0-F2). Runs the live position protector
// (model A, code at the worktree commit) over the 380 current-config fills
// of the 694-event replay, next to the plan walk (P) and the replay
// baseline exit (D). Gated by U0F_RUN=1; reads the entry-study cache from
// U0F_DATA and writes per-fill rows to U0F_OUT. The data is never committed.
//
// Pre-registered conventions:
//   - fill = replay Arm A limit fill (bar fillIdx, price = plan entry).
//   - leverage 10; fees taker 0.05%/side on every fill and exit leg
//     (OD-23), the same convention for P, A and D.
//   - exchange orders (stop, take-profit) rest on the exchange and fill
//     intrabar at their price; within a bar the stop is checked first.
//   - the protector polls once per 1m bar at the bar close (mark = close).
//     Live polls run every 15-30 s, so intrabar peaks are under-seen here.
//   - open-time orders: stop = plan stop; take-profit = plan TP1 capped by
//     the default strategy TP cap (ClosePosition, full close), as placed by
//     the executor; protector planned targets TP1/TP2 = plan TP1/TP2; no
//     TP0 (T2 candidates carry none), so the planned TP0 = take-profit.
//   - window = the cached bars (as the replay); remainder marked at the
//     last close. P: 50% at TP1, 50% at TP2, original stop never moves.

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
	"github.com/Aixxww/AiT/kernel"
)

type u0fLeg struct {
	Frac, Price float64
	Why         string
}

type u0fRow struct {
	Index     int      `json:"index"`
	Setup     string   `json:"setup"`
	Direction string   `json:"direction"`
	TrigMs    int64    `json:"trig_ms"`
	StopPct   float64  `json:"stop_pct"`
	RiskR     float64  `json:"fee_r_unit"`
	P         float64  `json:"p_r"`
	PFirst    string   `json:"p_first"`
	A         float64  `json:"a_r"`
	AExit     string   `json:"a_exit"`
	AActions  []string `json:"a_actions"`
	AMoves    int      `json:"a_stop_moves"`
	D         float64  `json:"d_r"`
	DOutcome  string   `json:"d_outcome"`
	// pullback depth (R) between first +0.25R and TP1, for TP1-reaching P paths
	PullbackR float64 `json:"pullback_r_before_tp1"`
	MinsToTP1 float64 `json:"mins_to_tp1"`
}

func u0fFee(legs []u0fLeg, E, d float64) float64 {
	n := E
	for _, l := range legs {
		n += l.Frac * l.Price
	}
	return 0.0005 * n / d
}

func TestU0F(t *testing.T) {
	if os.Getenv("U0F_RUN") != "1" {
		t.Skip("U0F_RUN=1 to run")
	}
	dir := os.Getenv("U0F_DATA")
	events, err := replay.LoadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	tpCap := kernel.HunterV7EffectiveExecutionGeometry(3.0, 0, 0.5, 3.0, true).MaxTPMovePct
	const lev = 10
	var rows []u0fRow
	defer func() { protectionNow = time.Now }()
	for i, ev := range events {
		if replay.ObserveSetups[ev.Setup] {
			continue
		}
		bars, err := replay.LoadBars(dir, i)
		if err != nil {
			t.Fatal(err)
		}
		res := replay.Simulate(ev, bars)
		if res.Intake != "FILLED" {
			continue
		}
		long := ev.Direction == "LONG"
		sgn := 1.0
		side := "long"
		if !long {
			sgn, side = -1, "short"
		}
		E, S, T1, T2 := ev.Entry, ev.Stop, ev.TP1, ev.TP2
		d := math.Abs(E - S)
		u := func(p float64) float64 { return sgn * (p - E) / d }
		fav := func(b replay.Bar) float64 {
			if long {
				return b.High
			}
			return b.Low
		}
		adv := func(b replay.Bar) float64 {
			if long {
				return b.Low
			}
			return b.High
		}
		hit := func(px, lvl float64, favourable bool) bool {
			if favourable == long {
				return px >= lvl
			}
			return px <= lvl
		}
		row := u0fRow{Index: i, Setup: ev.Setup, Direction: ev.Direction, TrigMs: ev.TrigMs, StopPct: d / E * 100, RiskR: d}

		// ---- P: plan walk ----
		{
			var legs []u0fLeg
			q := 1.0
			tp1 := false
			first := "none"
			reached := false
			minAfter := 0.0
			peakAfter := 0.0
			for j := res.FillIdx; j < len(bars) && q > 0; j++ {
				b := bars[j]
				if hit(adv(b), S, false) {
					legs = append(legs, u0fLeg{q, S, "stop"})
					q = 0
					if first == "none" {
						first = "stop"
					}
					break
				}
				f := u(fav(b))
				if reached && !tp1 {
					if peakAfter-u(adv(b)) > minAfter {
						minAfter = peakAfter - u(adv(b))
					}
				}
				if f >= 0.25 && !reached {
					reached = true
				}
				if reached && f > peakAfter {
					peakAfter = f
				}
				if !tp1 && hit(fav(b), T1, true) {
					tp1 = true
					legs = append(legs, u0fLeg{0.5, T1, "tp1"})
					q -= 0.5
					if first == "none" {
						first = "tp1"
						row.PullbackR = minAfter
						row.MinsToTP1 = float64(b.OpenTime-bars[res.FillIdx].OpenTime) / 60000
					}
				}
				if tp1 && hit(fav(b), T2, true) {
					legs = append(legs, u0fLeg{q, T2, "tp2"})
					q = 0
				}
			}
			if q > 0 {
				legs = append(legs, u0fLeg{q, bars[len(bars)-1].Close, "timeout"})
			}
			g := 0.0
			for _, l := range legs {
				g += l.Frac * u(l.Price)
			}
			row.P = g - u0fFee(legs, E, d)
			row.PFirst = first
		}

		// ---- A: live protector ----
		{
			tpA := T1
			if c := E * (1 + sgn*tpCap/100); sgn*(tpA-c) > 0 {
				tpA = c
			}
			u0fWalkA(ev.Symbol, long, sgn, side, E, S, T1, T2, tpA, lev, bars, res.FillIdx, u, fav, adv, hit, d, &row)
		}
		// ---- D: replay baseline, same fee convention ----
		row.D = res.GrossR - 0.0005*2*E/d
		row.DOutcome = res.Outcome
		rows = append(rows, row)
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].Index < rows[b].Index })
	raw, _ := json.Marshal(rows)
	if err := os.WriteFile(os.Getenv("U0F_OUT"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("fills %d", len(rows))
}

// u0fWalkA is the U0-F model-A walk. U14: it runs the production walker
// (protector_walk.go), which drives the live protector step
// (protectPosition) with the bar close time as the clock; before U14 this
// was a test-only loop that shifted the protector's stored times instead.
// Inputs: plan E/S/T1/T2 (T1/T2 = 0 means "no planned target"), exchange tp,
// leverage, side, the bars from the fill bar on, and the R helpers.
func u0fWalkA(symbol string, long bool, sgn float64, side string, E, S, T1, T2, tp float64, lev int, bars []replay.Bar, fillIdx int,
	u func(float64) float64, fav, adv func(replay.Bar) float64, hit func(float64, float64, bool) bool, d float64, row *u0fRow) {
	q0 := 1000.0 / E // $1000 notional (as protector_ab_sim_test.go)
	wb := make([]protectorWalkBar, len(bars))
	for k, b := range bars {
		wb[k] = protectorWalkBar{OpenMs: b.OpenTime, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close}
	}
	res := protectorWalk(protectorWalkInput{Symbol: symbol, Side: side, Entry: E, Stop: S, TakeProfit: tp, TP1: T1, TP2: T2,
		Leverage: lev, Qty: q0, Bars: wb, FillIdx: fillIdx})
	legs := make([]u0fLeg, len(res.Legs))
	g := 0.0
	for k, l := range res.Legs {
		legs[k] = u0fLeg{Frac: l.Qty / q0, Price: l.Price, Why: l.Why}
		g += legs[k].Frac * u(l.Price)
	}
	row.A = g - u0fFee(legs, E, d)
	row.AExit = res.Exit
	row.AActions = res.Actions
	row.AMoves = res.StopMoves
	_, _, _, _, _ = long, sgn, fav, adv, hit
}
