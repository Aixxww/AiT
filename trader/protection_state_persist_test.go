package trader

// #93 / audit cluster 4: restart (D4) and take-profit (D5) tests.

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"
)

func persistTrader(t *testing.T, st *store.Store, v *protectorSimVenue) *AutoTrader {
	t.Helper()
	at := newProtectorWalkTrader(v, nil)
	at.id = "r1"
	at.store = st
	return at
}

// D4: TP0/giveback flags, the peak and the first-seen time survive a
// restart, and are not rebuilt from the last 20 prompts.
func TestProtectionStateSurvivesRestart(t *testing.T) {
	st := newLifecycleStore(t)
	v := newProtectorSimVenue("AAAUSDT", "long", 100, 10, 10, 98, 104)
	at := persistTrader(t, st, v)

	opened := at.resolveProtectionPositionOpenedAt("AAAUSDT", "long")
	state := at.getOrCreateProtectionState("AAAUSDT_long", 10, 2.0, opened)
	at.UpdatePeakPnL("AAAUSDT", "long", 7.5)
	state.PeakPnLPct = 7.5
	at.markProtectionAction("AAAUSDT_long", protectionTP0, false)
	at.markProtectionAction("AAAUSDT_long", protectionGivebackClose, false)

	rows := lifecycleRowsFor(t, st, "r1")
	if len(rows) != 1 || !rows[0].TP0Done || !rows[0].GivebackDone || rows[0].PeakPnLPct != 7.5 || rows[0].FirstSeenAt != opened.UnixMilli() {
		t.Fatalf("persisted row: %+v", rows)
	}

	// A prompt that mentions a much higher peak must be ignored.
	if err := st.Decision().LogDecision(&store.DecisionRecord{TraderID: "r1", CycleNumber: 9, Timestamp: time.Now(), Success: true,
		InputPrompt: "1. AAAUSDT LONG | Entry 100 Current 100.2 | PnL+2.00% | Peak PnL25.00% | Leverage 10x"}); err != nil {
		t.Fatal(err)
	}

	// Restart: a new trader on the same store.
	time.Sleep(5 * time.Millisecond)
	at2 := persistTrader(t, st, v)
	opened2 := at2.resolveProtectionPositionOpenedAt("AAAUSDT", "long")
	if !opened2.Equal(time.UnixMilli(opened.UnixMilli())) {
		t.Fatalf("first-seen time reset by the restart: %v vs %v", opened2, opened)
	}
	if peak := at2.ensurePeakPnLCacheInitialized("AAAUSDT", "long", 2.0, opened2); peak != 7.5 {
		t.Fatalf("peak after restart = %v, want 7.5 from the record (not 25 from a prompt)", peak)
	}
	state2 := at2.getOrCreateProtectionState("AAAUSDT_long", 5, 2.0, opened2)
	if !state2.TP0Done || !state2.GivebackDone || state2.PeakPnLPct != 7.5 || !state2.OpenedAt.Equal(opened2) {
		t.Fatalf("restored state: %+v", state2)
	}
	// TP0 does not fire again on the remainder.
	e := defaultProtectorEngine()
	cur := calculateLeveragedPnLPct("long", 100, 100.9, 10)
	if action, _ := e.choose(state2, protectorPos{Side: "long", Leverage: 10}, cur, 0.9, time.Now()); action == protectionTP0 {
		t.Fatal("TP0 re-fired on the remainder after a restart")
	}
}

// A new open gets its own row: closing and reopening does not inherit flags.
func TestProtectionStateNewOpenStartsClean(t *testing.T) {
	st := newLifecycleStore(t)
	v := newProtectorSimVenue("AAAUSDT", "long", 100, 10, 10, 98, 104)
	at := persistTrader(t, st, v)
	at.getOrCreateProtectionState("AAAUSDT_long", 10, 2.0, time.Now())
	at.markProtectionAction("AAAUSDT_long", protectionTP0, false)
	at.markLifecycleClosed("AAAUSDT", "long", "closed")
	at.ClearPeakPnLCache("AAAUSDT", "long")
	s := at.getOrCreateProtectionState("AAAUSDT_long", 10, 0.5, time.Now())
	if s.TP0Done {
		t.Fatal("a new position must not inherit the closed position's flags")
	}
}

func lifecycleRowsFor(t *testing.T, st *store.Store, traderID string) []store.PositionLifecycle {
	t.Helper()
	rows, err := st.Decision().ListLifecycles(traderID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// D5: the take-profit is never cancelled without a replacement. It comes
// from the state, then the lifecycle record, then the exchange's open orders.
func TestReprotectKeepsOrReplacesTakeProfit(t *testing.T) {
	st := newLifecycleStore(t)

	// (a) TP open on the exchange, nothing known in state or record: kept as is.
	v := newProtectorSimVenue("AAAUSDT", "long", 100, 10, 10, 98, 104)
	at := persistTrader(t, st, v)
	at.protectionState["AAAUSDT_long"] = &positionProtectionState{ActiveStopLoss: 98}
	v.mark = 100.9
	if closed, err := at.executeProtectionAction("AAAUSDT", "long", 10, 100.9, 100, protectionTP0); closed || err != nil {
		t.Fatalf("tp0: %v %v", closed, err)
	}
	if v.tp != 104 {
		t.Fatalf("the open take-profit was removed: tp %.4f orders %v", v.tp, v.orders)
	}
	for _, o := range v.orders {
		if strings.Contains(o, "stop+tp") || strings.HasPrefix(o, "tp ") {
			t.Fatalf("take-profit cancelled or duplicated: %v", v.orders)
		}
	}

	// (b) No TP open, none in state, the lifecycle record has one: re-placed from the record.
	v2 := newProtectorSimVenue("BBBUSDT", "long", 100, 10, 10, 98, 0)
	at2 := persistTrader(t, st, v2)
	if err := st.Decision().CreateLifecycle(&store.PositionLifecycle{TraderID: "r1", Symbol: "BBBUSDT", Side: "long",
		Status: store.LifecycleStatusOpen, TakeProfit: 104.5}); err != nil {
		t.Fatal(err)
	}
	at2.protectionState["BBBUSDT_long"] = &positionProtectionState{ActiveStopLoss: 98}
	v2.mark = 100.9
	at2.executeProtectionAction("BBBUSDT", "long", 10, 100.9, 100, protectionTP0)
	if v2.tp != 104.5 {
		t.Fatalf("take-profit not re-placed from the lifecycle record: tp %.4f orders %v", v2.tp, v2.orders)
	}

	// (c) No TP anywhere: nothing placed, nothing cancelled, the stop protects.
	v3 := newProtectorSimVenue("CCCUSDT", "long", 100, 10, 10, 98, 0)
	at3 := persistTrader(t, st, v3)
	at3.protectionState["CCCUSDT_long"] = &positionProtectionState{ActiveStopLoss: 98}
	v3.mark = 100.9
	at3.executeProtectionAction("CCCUSDT", "long", 10, 100.9, 100, protectionTP0)
	if v3.tp != 0 || v3.stop <= 0 {
		t.Fatalf("no take-profit known: tp %.4f stop %.4f", v3.tp, v3.stop)
	}
}
