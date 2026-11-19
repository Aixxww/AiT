package trader

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"
)

// ---- config and units ----

func TestU14ProtectorDefaultsAreThePreU14Literals(t *testing.T) {
	c := store.EffectiveProtector(nil)
	roe := func(v float64) store.ProtectorLevel {
		return store.ProtectorLevel{Value: v, Unit: store.ProtectorUnitROEPct}
	}
	price := func(v float64) store.ProtectorLevel {
		return store.ProtectorLevel{Value: v, Unit: store.ProtectorUnitPricePct}
	}
	for name, c := range map[string][2]store.ProtectorLevel{
		"hard":      {c.HardLoss, roe(-12)},
		"early":     {c.EarlyLoss, roe(-8)},
		"floor":     {c.ProfitFloorPeak, roe(5)},
		"tp0":       {c.TP0, roe(8)},
		"tp1":       {c.TP1, roe(6)},
		"tp1_move":  {c.TP1MinMove, price(1)},
		"tp2":       {c.TP2, roe(12)},
		"tp2_move":  {c.TP2MinMove, price(1.5)},
		"near":      {c.NearTP1Peak, roe(6 * 0.95)},
		"second":    {c.NearTP1SecondChance, roe(6 * 0.90)},
		"base_risk": {c.DynamicStopBaseRisk, roe(35)},
		"micro":     {c.MicroTP0MinMove, price(0.55)},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %+v, want %+v", name, c[0], c[1])
		}
	}
	if c.BaseInterval() != 30*time.Second || c.PreTPMinHold() != 20*time.Minute || c.EarlyLossWindow() != 15*time.Minute || c.TimeDecayMaxFraction != 0.5 {
		t.Fatalf("durations / decay cap: %+v", c)
	}
	bad := store.EffectiveProtector(&store.ProtectorConfig{TP1: store.ProtectorLevel{Value: 1, Unit: "pips"}})
	if bad.TP1 != roe(6) {
		t.Fatal("an unknown unit must read as the default level")
	}
}

// A level in R fires at the R threshold of the position's planned stop and
// never fires without one.
func TestU14RUnitLevels(t *testing.T) {
	cfg := store.EffectiveProtector(&store.ProtectorConfig{
		TP1:        store.ProtectorLevel{Value: 0.5, Unit: store.ProtectorUnitR},
		TP1MinMove: store.ProtectorLevel{Value: 0, Unit: store.ProtectorUnitPricePct},
	})
	e := newProtectorEngine(cfg)
	now := time.Now()
	// L 10, planned stop 2% (ROE -20): 0.5R = 1% price = 10% ROE.
	state := &positionProtectionState{PlannedStopLossROEPct: -20, TP0Done: true, OpenedAt: now.Add(-time.Minute)}
	pos := state.protectorPos("long", 10)
	if a, _ := e.choose(state, pos, 9.9, 0.99, now); a == protectionTP1 {
		t.Fatal("0.495R must not fire a 0.5R TP1")
	}
	if a, _ := e.choose(state, pos, 10, 1.0, now); a != protectionTP1 {
		t.Fatalf("0.5R must fire TP1, got %q", a)
	}
	noPlan := &positionProtectionState{OpenedAt: now.Add(-time.Minute)}
	if a, _ := e.choose(noPlan, noPlan.protectorPos("long", 10), 30, 3, now); a == protectionTP1 {
		t.Fatal("an R level must not fire without a planned stop")
	}
	if got := e.rLevelsUnavailable(noPlan.protectorPos("long", 10)); !reflect.DeepEqual(got, []string{"tp1"}) {
		t.Fatalf("unavailable = %v", got)
	}
	// Defaults read in R for the same position: TP1 6% ROE = 0.3R.
	line := defaultProtectorEngine().levelsInR(pos)
	for _, want := range []string{"tp1 +0.300R", "hard_loss -0.600R", "early_loss -0.400R", "tp1_min_move +0.500R", "dynamic_stop_base_risk +1.750R"} {
		if !strings.Contains(line, want) {
			t.Errorf("levels in R missing %q: %s", want, line)
		}
	}
}

// ---- F29 ----

func TestU14F29RebuildNeverLoosensAndRecordsTheStop(t *testing.T) {
	v := newProtectorSimVenue("AAAUSDT", "long", 100, 10, 10, 100.3, 104)
	at := newProtectorWalkTrader(v, nil)
	at.protectionState["AAAUSDT_long"] = &positionProtectionState{ActiveStopLoss: 100.3, DynamicStop: 100.3}
	if closed, err := at.rebuildProtectionStopsWithBuffer("AAAUSDT", "long", 6.5, 100, 0.001); closed || err != nil {
		t.Fatalf("rebuild: %v %v", closed, err)
	}
	if v.stop != 100.3 || at.protectionState["AAAUSDT_long"].ActiveStopLoss != 100.3 {
		t.Fatalf("a +0.15R floor stop was replaced by %.4f", v.stop)
	}
	// (b) only the planned stop in state: the breakeven stop is placed and
	// recorded, so a later candidate below entry is not "more protective".
	v2 := newProtectorSimVenue("BBBUSDT", "long", 100, 10, 10, 98, 104)
	at2 := newProtectorWalkTrader(v2, nil)
	st := &positionProtectionState{ActiveStopLoss: 98, OpenedAt: time.Now().Add(-50 * time.Minute)}
	at2.protectionState["BBBUSDT_long"] = st
	if _, err := at2.rebuildProtectionStopsWithBuffer("BBBUSDT", "long", 6.5, 100, 0.001); err != nil {
		t.Fatal(err)
	}
	if v2.stop != 100.1 || st.ActiveStopLoss != 100.1 {
		t.Fatalf("breakeven not recorded: venue %.4f state %.4f", v2.stop, st.ActiveStopLoss)
	}
	before := len(v2.orders)
	// A time-decayed base stop (below entry) is not tighter than 100.1.
	if err := at2.updateDynamicProtectionStopAt(at2.protectorEngine(), "BBBUSDT", "long", 6.5, 100, 100.05, 10, st, time.Now()); err != nil {
		t.Fatal(err)
	}
	if v2.stop != 100.1 || len(v2.orders) != before {
		t.Fatalf("a stop below the breakeven was placed: %.4f %v", v2.stop, v2.orders[before:])
	}
	// Short side: the more protective is the lower stop.
	v3 := newProtectorSimVenue("CCCUSDT", "short", 100, 10, 10, 99.7, 96)
	at3 := newProtectorWalkTrader(v3, nil)
	at3.protectionState["CCCUSDT_short"] = &positionProtectionState{ActiveStopLoss: 99.7}
	at3.rebuildProtectionStopsWithBuffer("CCCUSDT", "short", 6.5, 100, 0.0015)
	if v3.stop != 99.7 {
		t.Fatalf("short rebuild loosened to %.4f", v3.stop)
	}
}

// ---- F30 ----

func TestU14F30GivebackAndTrailFireOnce(t *testing.T) {
	now := time.Now()
	st := &positionProtectionState{PeakPnLPct: 6, OpenedAt: now.Add(-time.Hour)}
	if a, _ := choosePositionProtectionAction(st, -0.5, -0.05); a != protectionGivebackClose {
		t.Fatalf("first giveback: %q", a)
	}
	at := newProtectorWalkTrader(newProtectorSimVenue("A", "long", 100, 1, 10, 98, 0), nil)
	at.protectionState["A_long"] = st
	at.markProtectionAction("A_long", protectionGivebackClose, false)
	if !st.GivebackDone {
		t.Fatal("giveback done-flag not set")
	}
	if a, _ := choosePositionProtectionAction(st, -0.5, -0.05); a == protectionGivebackClose {
		t.Fatal("giveback must not repeat")
	}
	// Hard loss still fires after a giveback.
	if a, _ := choosePositionProtectionAction(st, -13, -1.3); a != protectionHardLossClose {
		t.Fatalf("hard loss after giveback: %q", a)
	}
	tr := &positionProtectionState{PeakPnLPct: 10, TP1Done: true, OpenedAt: now.Add(-time.Hour)}
	if a, _ := choosePositionProtectionAction(tr, 6, 0.6); a != protectionTrailClose {
		t.Fatalf("first trail: %q", a)
	}
	at.protectionState["B_long"] = tr
	at.markProtectionAction("B_long", protectionTrailClose, false)
	if a, _ := choosePositionProtectionAction(tr, 6, 0.6); a == protectionTrailClose {
		t.Fatal("trail must not repeat")
	}
}

// ---- F31 ----

func TestU14F31TakeProfitReplacedForTheRemainder(t *testing.T) {
	v := newProtectorSimVenue("AAAUSDT", "long", 100, 10, 10, 98, 104)
	at := newProtectorWalkTrader(v, nil)
	at.protectionState["AAAUSDT_long"] = &positionProtectionState{ActiveStopLoss: 98, ExchangeTakeProfit: 104}
	v.mark = 100.9
	closedAll, err := at.executeProtectionAction("AAAUSDT", "long", 10, 100.9, 100, protectionTP0)
	if closedAll || err != nil {
		t.Fatalf("tp0: %v %v", closedAll, err)
	}
	if v.tp != 104 || v.stop != 100.1 || v.qty != 6.5 {
		t.Fatalf("after TP0: tp %.4f stop %.4f qty %.4f orders %v", v.tp, v.stop, v.qty, v.orders)
	}
	want := []string{"close 3.5@100.9", "cancel side stop+tp", "stop 100.1", "tp 104"}
	if !reflect.DeepEqual(v.orders, want) {
		t.Fatalf("orders = %v, want %v", v.orders, want)
	}
	// Mark already beyond the take-profit: not re-placed.
	v2 := newProtectorSimVenue("BBBUSDT", "long", 100, 10, 10, 98, 101)
	at2 := newProtectorWalkTrader(v2, nil)
	at2.protectionState["BBBUSDT_long"] = &positionProtectionState{ActiveStopLoss: 98, ExchangeTakeProfit: 101}
	at2.executeProtectionAction("BBBUSDT", "long", 10, 101.2, 100, protectionTP1)
	if v2.tp != 0 {
		t.Fatalf("a take-profit behind the mark was placed: %.4f", v2.tp)
	}
	// Hydration records the open's take-profit.
	st := &positionProtectionState{}
	defaultProtectorEngine().hydrate(st, "long", 100, 10, plannedPositionRisk{stopLoss: 98, takeProfit: 104})
	if st.ExchangeTakeProfit != 104 {
		t.Fatalf("hydrate: %+v", st)
	}
}

// ---- shared code path: walker and live adapter ----

func u14Bars(t0 int64, hlc [][3]float64) []protectorWalkBar {
	out := make([]protectorWalkBar, len(hlc))
	prev := hlc[0][2]
	for i, x := range hlc {
		out[i] = protectorWalkBar{OpenMs: t0 + int64(i)*60000, Open: prev, Low: x[0], High: x[1], Close: x[2]}
		prev = x[2]
	}
	return out
}

// u14Live drives the live adapter (checkPositionDrawdown) bar by bar on the
// same simulated venue: resting orders fill first, then one poll at the bar
// close with the plan read from a decision record in a SQLite store.
func u14Live(t *testing.T, in protectorWalkInput) protectorWalkResult {
	t.Helper()
	st, err := store.New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatal(err)
	}
	v := newProtectorSimVenue(in.Symbol, in.Side, in.Entry, in.Qty, in.Leverage, in.Stop, in.TakeProfit)
	at := newProtectorWalkTrader(v, in.Config)
	at.id, at.store = "u14-live", st
	action := "open_long"
	if in.Side == "short" {
		action = "open_short"
	}
	if err := st.Decision().LogDecision(&store.DecisionRecord{TraderID: at.id, CycleNumber: 1, Timestamp: time.UnixMilli(in.Bars[0].OpenMs).Add(-time.Minute), Success: true,
		Decisions: []store.DecisionAction{{Action: action, Symbol: in.Symbol, StopLoss: in.Stop, TakeProfit: in.TakeProfit,
			HunterV7TP0Price: in.TP0, HunterV7TP1Price: in.TP1, HunterV7TP2Price: in.TP2, Success: true}}}); err != nil {
		t.Fatal(err)
	}
	at.positionFirstSeenTime[in.Symbol+"_"+in.Side] = in.Bars[in.FillIdx].OpenMs
	defer func() { protectionNow = time.Now }()
	res := protectorWalkResult{Exit: "timeout"}
	for j := in.FillIdx; j < len(in.Bars) && v.qty > 0; j++ {
		b := in.Bars[j]
		if why, ok := v.fillResting(b, in.Side == "long"); ok {
			res.Exit = why
			break
		}
		simNow := time.UnixMilli(walkBarMs(b))
		protectionNow = func() time.Time { return simNow }
		v.mark = b.Close
		at.checkPositionDrawdown()
	}
	if v.qty > 0 {
		v.mark = in.Bars[len(in.Bars)-1].Close
		v.closeAt(0)
	}
	res.Legs, res.StopMoves, res.Orders = v.legs, v.moves, v.orders
	return res
}

func u14Paths() map[string]protectorWalkInput {
	t0 := int64(1_790_000_000_000)
	flat := func(n int, c float64) [][3]float64 {
		var out [][3]float64
		for i := 0; i < n; i++ {
			out = append(out, [3]float64{c - 0.03, c + 0.03, c})
		}
		return out
	}
	long := func(name string, hlc [][3]float64, tp1, tp2 float64) protectorWalkInput {
		return protectorWalkInput{Symbol: name, Side: "long", Entry: 100, Stop: 98, TakeProfit: 104, TP1: tp1, TP2: tp2, Leverage: 10, Qty: 10, Bars: u14Bars(t0, hlc)}
	}
	paths := map[string]protectorWalkInput{
		// TP0 partial, profit floor, then a dip through the moved stop.
		"tp0_then_moved_stop": long("AUSDT", [][3]float64{{99.9, 100.1, 100}, {100, 100.9, 100.85}, {100.4, 100.95, 100.6}, {100.0, 100.4, 100.05}}, 0, 0),
		// A run through TP1 and TP2 by price, then a pullback (trail).
		"tp1_tp2_trail": long("BUSDT", append([][3]float64{{99.9, 100.1, 100}, {100, 101.3, 101.2}, {101, 102.2, 102.1}, {101.8, 102.4, 102.3}},
			append(flat(3, 102.2), [3]float64{101.3, 102.2, 101.4}, [3]float64{100.9, 101.5, 101.0})...), 101.2, 102.0),
		// Early loss inside 15 minutes.
		"early_loss": long("CUSDT", [][3]float64{{99.9, 100.1, 100}, {99.1, 100, 99.15}}, 0, 0),
		// Slow bleed: the early-loss window has passed after 15 minutes.
		"late_drift": long("DUSDT", append(flat(17, 100), [3]float64{99.15, 99.25, 99.2}, [3]float64{98.9, 99.2, 98.95}), 0, 0),
	}
	// Near-TP1 second chance at leverage 5: a +5.75% ROE peak, then +5.5%
	// ROE with a 1.1% move held past the 20-minute hold.
	near := long("EUSDT", append([][3]float64{{99.9, 100.1, 100}, {100, 101.2, 101.15}}, flat(22, 101.1)...), 0, 0)
	near.Leverage = 5
	paths["near_tp1_giveback"] = near
	short := paths["tp0_then_moved_stop"]
	short.Symbol, short.Side, short.Stop, short.TakeProfit = "SUSDT", "short", 102, 96
	short.Bars = u14Bars(t0, [][3]float64{{99.9, 100.1, 100}, {99.1, 100, 99.15}, {99.05, 99.6, 99.4}, {99.6, 100, 99.95}})
	paths["short_tp0_then_moved_stop"] = short
	return paths
}

// The live adapter and the walker run the same protector step: on every
// synthetic path they produce the same orders, stop moves and fills.
func TestU14WalkerMatchesLiveAdapter(t *testing.T) {
	for name, in := range u14Paths() {
		w := protectorWalk(in)
		l := u14Live(t, in)
		if !reflect.DeepEqual(w.Orders, l.Orders) || w.StopMoves != l.StopMoves || len(w.Legs) != len(l.Legs) {
			t.Errorf("%s: walker orders %v / live %v (moves %d / %d)", name, w.Orders, l.Orders, w.StopMoves, l.StopMoves)
			continue
		}
		for k := range w.Legs {
			if w.Legs[k].Qty != l.Legs[k].Qty || w.Legs[k].Price != l.Legs[k].Price {
				t.Errorf("%s leg %d: walker %+v live %+v", name, k, w.Legs[k], l.Legs[k])
			}
		}
		if len(w.Actions) == 0 && w.Exit == "timeout" && name != "late_drift" {
			t.Errorf("%s: path exercised nothing (%+v)", name, w)
		}
	}
}

// The walker's clock is the bar time: the early-loss window closes after 15
// minutes and the 20-minute pre-TP1 rules can fire (before U14 the U0-F
// walk read a shifted open time there, so neither held).
func TestU14WalkerUsesTheBarClock(t *testing.T) {
	p := u14Paths()
	if r := protectorWalk(p["early_loss"]); r.Exit != "protector_close:hard_loss_close" {
		t.Fatalf("early loss: %+v", r)
	}
	if r := protectorWalk(p["late_drift"]); strings.Contains(r.Exit, "hard_loss") || len(r.Actions) != 0 {
		t.Fatalf("-8%% ROE after 15 minutes must not be an early-loss exit: %+v", r)
	}
	r := protectorWalk(p["near_tp1_giveback"])
	if len(r.Actions) == 0 || r.Actions[0] != string(protectionGivebackClose) {
		t.Fatalf("near-TP1 giveback after 20 minutes: %+v", r)
	}
}
