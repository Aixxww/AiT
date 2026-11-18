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

type u0fExchange struct {
	contextTestTrader
	qty       float64
	stop, tp  float64
	mark      float64
	legs      []u0fLeg
	moves     int
	origStop  float64
	actionWhy string
}

func (x *u0fExchange) closeAt(q float64) {
	if q <= 0 || q > x.qty {
		q = x.qty
	}
	x.legs = append(x.legs, u0fLeg{q, x.mark, x.actionWhy})
	x.qty -= q
	if x.qty < 1e-12 {
		x.qty = 0
	}
}
func (x *u0fExchange) CloseLong(_ string, q float64) (map[string]interface{}, error) {
	x.closeAt(q)
	return map[string]interface{}{"orderId": int64(1)}, nil
}
func (x *u0fExchange) CloseShort(_ string, q float64) (map[string]interface{}, error) {
	x.closeAt(q)
	return map[string]interface{}{"orderId": int64(1)}, nil
}
func (x *u0fExchange) GetPositions() ([]map[string]interface{}, error) { return nil, nil }
func (x *u0fExchange) SetStopLoss(_, _ string, _, p float64) error {
	if p != x.stop {
		x.moves++
	}
	x.stop = p
	return nil
}
func (x *u0fExchange) CancelOwnedProtectionOrders(_, _ string, stopsOnly bool) error {
	x.stop = 0
	if !stopsOnly {
		x.tp = 0
	}
	return nil
}
func (x *u0fExchange) ProtectiveStopOpen(string, string) (bool, error) { return x.stop > 0, nil }

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

// u0fWalkA is the U0-F model-A walk, extracted verbatim so U0-H2 reuses it.
// Inputs: plan E/S/T1/T2 (T1/T2 = 0 means "no planned target"), exchange tp,
// leverage, side, the bars from the fill bar on, and the R helpers.
func u0fWalkA(symbol string, long bool, sgn float64, side string, E, S, T1, T2, tp float64, lev int, bars []replay.Bar, fillIdx int,
	u func(float64) float64, fav, adv func(replay.Bar) float64, hit func(float64, float64, bool) bool, d float64, row *u0fRow) {
	ev := struct{ Symbol string }{symbol}
	res := struct{ FillIdx int }{fillIdx}
	// ---- A: live protector ----
	{
		q0 := 1000.0 / E // $1000 notional (as protector_ab_sim_test.go)
		x := &u0fExchange{qty: q0, stop: S, origStop: S}
		x.tp = tp
		at := &AutoTrader{trader: x, protectionState: map[string]*positionProtectionState{}, peakPnLCache: map[string]float64{}, positionFirstSeenTime: map[string]int64{}}
		posKey := ev.Symbol + "_" + side
		simOpen := time.UnixMilli(bars[res.FillIdx].OpenTime)
		var simLastUpd time.Time
		exit := "timeout"
		for j := res.FillIdx; j < len(bars) && x.qty > 0; j++ {
			b := bars[j]
			// resting exchange orders, stop first
			if x.stop > 0 && hit(adv(b), x.stop, false) {
				x.mark, x.actionWhy = x.stop, "stop"
				if x.stop != S {
					x.actionWhy = "moved_stop"
				}
				x.closeAt(0)
				exit = x.actionWhy
				break
			}
			if x.tp > 0 && hit(fav(b), x.tp, true) {
				x.mark, x.actionWhy = x.tp, "exchange_tp"
				x.closeAt(0)
				exit = "exchange_tp"
				break
			}
			// protector poll at the bar close
			simNow := time.UnixMilli(b.OpenTime + 60000)
			protectionNow = func() time.Time { return simNow }
			off := time.Now().Sub(simNow)
			x.mark = b.Close
			cur := calculateLeveragedPnLPct(side, E, b.Close, lev)
			move := calculateUnleveragedPnLPct(side, E, b.Close)
			st := at.getOrCreateProtectionState(posKey, x.qty, cur, simOpen)
			st.OpenedAt = simOpen.Add(off)
			shifted := time.Time{}
			if !simLastUpd.IsZero() {
				shifted = simLastUpd.Add(off)
				st.LastStopUpdateAt = shifted
			}
			st.rememberActiveStopLoss(side, S)
			st.PlannedStopLossROEPct = calculateLeveragedPnLPct(side, E, S, lev)
			st.rememberPlannedTakeProfit(side, tp)
			st.rememberPlannedTarget(side, &st.PlannedTP1, T1)
			st.rememberPlannedTarget(side, &st.PlannedTP2, T2)
			x.actionWhy = "dynamic_stop_emergency"
			err := at.updateDynamicProtectionStop(ev.Symbol, side, x.qty, E, b.Close, lev, st)
			if !st.LastStopUpdateAt.Equal(shifted) && !st.LastStopUpdateAt.IsZero() {
				simLastUpd = simNow
			}
			if err != nil && x.qty == 0 {
				exit = "protector_close"
				break
			}
			action, _ := choosePositionProtectionAction(st, cur, move)
			if action == protectionNone && shouldTriggerPlannedTP0Price(side, b.Close, cur, st) {
				action = protectionTP0
			}
			if action == protectionNone && shouldTriggerPlannedTarget(side, b.Close, st.PlannedTP1, st.TP1Done) {
				action = protectionTP1
			}
			if action == protectionNone && shouldTriggerPlannedTarget(side, b.Close, st.PlannedTP2, st.TP2Done) {
				action = protectionTP2
			}
			st.OpenedAt = simOpen // keep sim clock in state between bars
			if action == protectionNone {
				continue
			}
			x.actionWhy = string(action)
			row.AActions = append(row.AActions, string(action))
			closedAll, _ := at.executeProtectionAction(ev.Symbol, side, x.qty, b.Close, E, action)
			at.markProtectionAction(posKey, action, closedAll)
			if x.qty == 0 {
				exit = "protector_close:" + string(action)
				break
			}
		}
		if x.qty > 0 {
			x.mark, x.actionWhy = bars[len(bars)-1].Close, "timeout"
			x.closeAt(0)
		}
		g := 0.0
		for k := range x.legs {
			x.legs[k].Frac /= q0
			g += x.legs[k].Frac * u(x.legs[k].Price)
		}
		row.A = g - u0fFee(x.legs, E, d)
		row.AExit = exit
		row.AMoves = x.moves
	}

	_ = long
}
