package trader

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"
)

// #128 / Audit R2 H3: startup reconciliation closes an intent row only when
// the emergency close of an unprotectable position actually flattened it.
func reconcileNoStopFixture(t *testing.T, closeErr error) (*lifecycleFake, *store.Store) {
	t.Helper()
	st := newLifecycleStore(t)
	ft := &lifecycleFake{fillPrice: 100}
	ft.live = []map[string]interface{}{{"symbol": "AAAUSDT", "side": "long", "positionAmt": 1.0, "entryPrice": 100.0}}
	ft.setStopErr = errors.New("stop rejected: -2021")
	ft.closeErr = closeErr
	if err := st.Decision().CreateLifecycle(&store.PositionLifecycle{TraderID: "p0", Symbol: "AAAUSDT", Side: "LONG",
		Status: store.LifecycleStatusIntent, PlanEntryPrice: 100, PlanStopLoss: 98, PlanTakeProfit: 103}); err != nil {
		t.Fatal(err)
	}
	newLifecycleTrader(t, ft, st).reconcileLifecycleOnStartup(time.Now())
	return ft, st
}

func TestReconcile128_CloseFailureLeavesRowOpen(t *testing.T) {
	ft, st := reconcileNoStopFixture(t, errors.New("close timeout"))
	if ft.closes == 0 {
		t.Fatal("an emergency close must have been attempted")
	}
	rows := lifecycleRows(t, st, "p0")
	if len(rows) != 1 || rows[0].Status == store.LifecycleStatusClosed {
		t.Fatalf("a live unprotected position must not be marked CLOSED: %+v", rows)
	}
	if rows[0].Status != store.LifecycleStatusOpen || !strings.Contains(rows[0].Note, "UNPROTECTED") || !strings.Contains(rows[0].Note, "close timeout") {
		t.Fatalf("row must stay open with the close error recorded: status %s note %q", rows[0].Status, rows[0].Note)
	}
	if len(ft.live) != 1 {
		t.Fatal("fixture: the position is still live")
	}
	// A later restart still sees the row (open, not closed).
	open, err := st.Decision().ListLifecycles("p0", store.LifecycleStatusOpen)
	if err != nil || len(open) != 1 {
		t.Fatalf("open rows %d (%v)", len(open), err)
	}
}

func TestReconcile128_CloseSuccessClosesRow(t *testing.T) {
	ft, st := reconcileNoStopFixture(t, nil)
	rows := lifecycleRows(t, st, "p0")
	if len(rows) != 1 || rows[0].Status != store.LifecycleStatusClosed || rows[0].Note != "closed at reconcile: no stop" {
		t.Fatalf("a flattened position must close the row as before: %+v", rows)
	}
	if len(ft.live) != 0 {
		t.Fatal("fixture: the position must be gone")
	}
}

// Sibling found by the #128 grep: the grid emergency exit discarded the
// CloseLong/CloseShort errors after cancelling every grid order. A failed
// close now raises the naked-position alert.
func TestReconcile128_GridEmergencyExitCloseFailureAlerts(t *testing.T) {
	for _, c := range []struct {
		closeErr error
		alerts   int
	}{{errors.New("rate limited"), 1}, {nil, 0}} {
		ft := &nakedTestTrader{closeErr: c.closeErr}
		ft.live = []map[string]interface{}{{"symbol": "GRIDUSDT", "side": "short", "positionAmt": -2.0}}
		at := newNakedTestAutoTrader(ft, 3)
		at.config.StrategyConfig.GridConfig = &store.GridStrategyConfig{Symbol: "GRIDUSDT"}
		at.gridState = NewGridState(at.config.StrategyConfig.GridConfig)
		if err := at.emergencyExit("test"); err != nil {
			t.Fatalf("emergencyExit: %v", err)
		}
		alerts := at.NakedPositionAlerts()
		if len(alerts) != c.alerts {
			t.Fatalf("closeErr=%v: %d naked alerts, want %d (%+v)", c.closeErr, len(alerts), c.alerts, alerts)
		}
		if c.alerts == 1 && (alerts[0].Side != "short" || !strings.Contains(alerts[0].Reason, "rate limited")) {
			t.Fatalf("alert %+v", alerts[0])
		}
	}
}
