package trader

// Fixed-behaviour tests for the two P0 findings of the adversarial audit
// (issue #73; docs/audit/adversarial-review-20261124.md, AUD-C5 and AUD-C3).
// The audit (#69) had no characterisation test for either finding (both were
// "code" findings); these tests assert the fixed behaviour.

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// ---------------------------------------------------------------- fakes ---

// nakedTestTrader: a Binance-like exchange with client-id opens, read-back,
// a stop check, a live position book and an optional 15 s position cache.
type nakedTestTrader struct {
	contextTestTrader

	live        []map[string]interface{} // the exchange's positions
	reportLag   bool                     // a fill is not reported by GetPositions yet
	cache       []map[string]interface{} // cached GetPositions view
	cacheValid  bool
	cacheUse    bool // emulate the 15 s cache (the clock never passes 15 s in a test)
	posReadErr  error
	positionsRd int

	openErr     error
	lookupErr   error
	opens       int
	pendingFill map[string]interface{}

	stopOpen    bool
	stopOpenErr error
	setStopErr  error
	closeErr    error
	closes      int
}

func (t *nakedTestTrader) GetPositions() ([]map[string]interface{}, error) {
	t.positionsRd++
	if t.posReadErr != nil {
		return nil, t.posReadErr
	}
	if t.cacheUse && t.cacheValid {
		return clonePositionMaps(t.cache), nil
	}
	view := clonePositionMaps(t.live)
	t.cache, t.cacheValid = view, true
	return clonePositionMaps(view), nil
}

func (t *nakedTestTrader) InvalidatePositionCache() {
	t.invalidatePositionsCalled++
	t.cacheValid = false
}

func (t *nakedTestTrader) fill(symbol, side string, qty float64) {
	pos := map[string]interface{}{"symbol": symbol, "side": side, "positionAmt": qty, "entryPrice": 100.0, "markPrice": 100.0, "leverage": 10.0}
	if t.reportLag {
		t.pendingFill = pos
		return
	}
	t.live = append(t.live, pos)
}

func (t *nakedTestTrader) OpenLongWithClientID(symbol string, qty float64, _ int, _ string) (map[string]interface{}, error) {
	t.opens++
	t.fill(symbol, "long", qty) // the order fills even when the response is lost
	if t.openErr != nil {
		return nil, t.openErr
	}
	return map[string]interface{}{"orderId": int64(t.opens)}, nil
}
func (t *nakedTestTrader) OpenShortWithClientID(symbol string, qty float64, _ int, _ string) (map[string]interface{}, error) {
	t.opens++
	t.fill(symbol, "short", qty)
	if t.openErr != nil {
		return nil, t.openErr
	}
	return map[string]interface{}{"orderId": int64(t.opens)}, nil
}
func (t *nakedTestTrader) LookupOrderByClientID(string, string) (map[string]interface{}, bool, error) {
	return nil, false, t.lookupErr
}
func (t *nakedTestTrader) IsAmbiguousOrderError(err error) bool { return errors.Is(err, errTimeout) }
func (t *nakedTestTrader) ClientOrderIDForIntent(key string) string {
	return "cid:" + key
}
func (t *nakedTestTrader) ProtectiveStopOpen(string, string) (bool, error) {
	return t.stopOpen, t.stopOpenErr
}
func (t *nakedTestTrader) SetStopLoss(symbol, positionSide string, quantity, stopPrice float64) error {
	if t.setStopErr != nil {
		return t.setStopErr
	}
	t.stopOpen = true
	return t.contextTestTrader.SetStopLoss(symbol, positionSide, quantity, stopPrice)
}
func (t *nakedTestTrader) closeSide(symbol, side string) (map[string]interface{}, error) {
	t.closes++
	if t.closeErr != nil {
		return nil, t.closeErr
	}
	kept := t.live[:0]
	for _, p := range t.live {
		if p["symbol"] == symbol && p["side"] == side {
			continue
		}
		kept = append(kept, p)
	}
	t.live = kept
	return map[string]interface{}{"orderId": int64(99)}, nil
}
func (t *nakedTestTrader) CloseLong(symbol string, _ float64) (map[string]interface{}, error) {
	return t.closeSide(symbol, "long")
}
func (t *nakedTestTrader) CloseShort(symbol string, _ float64) (map[string]interface{}, error) {
	return t.closeSide(symbol, "short")
}

func newNakedTestAutoTrader(ft *nakedTestTrader, maxPositions int) *AutoTrader {
	return &AutoTrader{
		id: "p0", name: "p0", callCount: 1, trader: ft,
		readBackSleep:         func(time.Duration) {},
		positionFirstSeenTime: map[string]int64{},
		peakPnLCache:          map[string]float64{},
		protectionState:       map[string]*positionProtectionState{},
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			CoinSource:  store.CoinSourceConfig{SourceType: "hunter_v7"},
			RiskControl: store.RiskControlConfig{MaxPositions: maxPositions},
		}},
	}
}

func p0Decision(symbol string) *kernel.Decision {
	return &kernel.Decision{Symbol: symbol, Action: "open_long", Leverage: 10, Price: 100, StopLoss: 97.5, TakeProfit: 104, SelectedHunterV7SignalID: "sig-" + symbol}
}

// --------------------------------------------------------------- AUD-C5 ---

// An inconclusive read-back is treated as filled: the live position is found
// and the decision's stop is placed; the open is recorded as successful.
func TestAudit_C5_UnknownOpenStateAssumesFillAndPlacesStop(t *testing.T) {
	ft := &nakedTestTrader{openErr: errTimeout, lookupErr: errors.New("read-back 5xx")}
	at := newNakedTestAutoTrader(ft, 1)
	d := p0Decision("AAAUSDT")
	_, err := at.openMarketPosition(d, "long", 2)
	if !errors.Is(err, errOpenOrderStateUnknown) {
		t.Fatalf("want errOpenOrderStateUnknown, got %v", err)
	}
	rec := &store.DecisionAction{Quantity: 2}
	if err := at.handleOpenOrderStateUnknown(d, "long", rec, err); err != nil {
		t.Fatalf("a found position must be protected and reported as opened, got %v", err)
	}
	if len(ft.stopLossCalls) != 1 || ft.stopLossCalls[0].stopPrice != 97.5 || ft.stopLossCalls[0].positionSide != "LONG" || ft.stopLossCalls[0].quantity != 2 {
		t.Fatalf("stop not placed from the decision: %+v", ft.stopLossCalls)
	}
	if !strings.Contains(rec.Error, "order_state_unknown_assumed_filled") {
		t.Fatalf("record must carry the assumed-fill note, got %q", rec.Error)
	}
	if n := len(at.NakedPositionAlerts()); n != 0 {
		t.Fatalf("protected position must not stay alerted, got %d", n)
	}
}

// Unknown state and the stop cannot be placed: the position is closed.
func TestAudit_C5_UnknownOpenStateStopFailureClosesPosition(t *testing.T) {
	ft := &nakedTestTrader{openErr: errTimeout, lookupErr: errors.New("5xx"), setStopErr: errors.New("-2021 would trigger")}
	at := newNakedTestAutoTrader(ft, 1)
	d := p0Decision("AAAUSDT")
	_, err := at.openMarketPosition(d, "long", 2)
	err = at.handleOpenOrderStateUnknown(d, "long", &store.DecisionAction{}, err)
	if err == nil || !strings.Contains(err.Error(), "emergency-closed") || len(ft.live) != 0 {
		t.Fatalf("want emergency close, got err=%v live=%v", err, ft.live)
	}
}

// Unknown state and no position on the exchange: nothing to protect, no
// stop placed, the open is reported as failed.
func TestAudit_C5_UnknownOpenStateWithoutPositionPlacesNothing(t *testing.T) {
	ft := &nakedTestTrader{openErr: errTimeout, lookupErr: errors.New("5xx"), reportLag: true}
	at := newNakedTestAutoTrader(ft, 1)
	d := p0Decision("AAAUSDT")
	_, err := at.openMarketPosition(d, "long", 2)
	err = at.handleOpenOrderStateUnknown(d, "long", &store.DecisionAction{}, err)
	if err == nil || !strings.Contains(err.Error(), "no AAAUSDT long position") || len(ft.stopLossCalls) != 0 {
		t.Fatalf("got err=%v stops=%v", err, ft.stopLossCalls)
	}
}

// Unknown state and the position read fails: a naked-position alert is
// raised (surfaced in GetStatus), not swallowed.
func TestAudit_C5_UnknownOpenStateUnreadablePositionRaisesAlert(t *testing.T) {
	ft := &nakedTestTrader{openErr: errTimeout, lookupErr: errors.New("5xx")}
	at := newNakedTestAutoTrader(ft, 1)
	d := p0Decision("AAAUSDT")
	_, err := at.openMarketPosition(d, "long", 2)
	ft.posReadErr = errors.New("positionRisk 503")
	if err := at.handleOpenOrderStateUnknown(d, "long", &store.DecisionAction{}, err); err == nil {
		t.Fatal("an unverifiable open must not be reported as success")
	}
	alerts := at.NakedPositionAlerts()
	if len(alerts) != 1 || alerts[0].Symbol != "AAAUSDT" || alerts[0].Side != "long" {
		t.Fatalf("want one naked alert, got %+v", alerts)
	}
	status := at.GetStatus()
	if status["naked_position_count"] != 1 {
		t.Fatalf("GetStatus must surface the alert, got %v", status["naked_position_count"])
	}
}

// A failed emergency close raises the alert instead of only returning text.
func TestAudit_C5_FailedEmergencyCloseRaisesAlert(t *testing.T) {
	ft := &nakedTestTrader{closeErr: errors.New("-1001 disconnected")}
	ft.live = []map[string]interface{}{{"symbol": "AAAUSDT", "side": "long", "positionAmt": 1.0}}
	at := newNakedTestAutoTrader(ft, 1)
	err := at.closeUnprotectedPosition("AAAUSDT", "long", 1, errors.New("failed to set mandatory stop loss"))
	if !errors.Is(err, errNakedEmergencyCloseFailed) || !strings.Contains(err.Error(), "emergency close FAILED") {
		t.Fatalf("got %v", err)
	}
	if len(at.NakedPositionAlerts()) != 1 {
		t.Fatal("failed emergency close must raise a naked-position alert")
	}
}

func p0Obs(entry, mark float64, plan plannedPositionRisk) protectorObs {
	now := time.Now()
	return protectorObs{Symbol: "AAAUSDT", Side: "long", Entry: entry, Mark: mark, Qty: 1, Leverage: 10,
		Now: now, OpenedAt: now.Add(-time.Hour), Plan: plan}
}

// Per-poll audit: a position with no exchange stop gets the plan's stop.
func TestAudit_C5_PollAuditRearmsMissingStopFromPlan(t *testing.T) {
	ft := &nakedTestTrader{}
	at := newNakedTestAutoTrader(ft, 1)
	at.raiseNakedPositionAlert("AAAUSDT", "long", "earlier failure")
	if closed := at.auditProtectiveStop(defaultProtectorEngine(), p0Obs(100, 100.5, plannedPositionRisk{stopLoss: 97.5})); closed {
		t.Fatal("must re-arm, not close")
	}
	if len(ft.stopLossCalls) != 1 || ft.stopLossCalls[0].stopPrice != 97.5 {
		t.Fatalf("plan stop not re-armed: %+v", ft.stopLossCalls)
	}
	if len(at.NakedPositionAlerts()) != 0 {
		t.Fatal("re-armed stop must resolve the alert")
	}
}

// Without a plan the stop is re-armed at the protector's hard-loss distance
// (-12% ROE at 10x = 1.2% below entry).
func TestAudit_C5_PollAuditRearmsDefaultDistanceWithoutPlan(t *testing.T) {
	ft := &nakedTestTrader{}
	at := newNakedTestAutoTrader(ft, 1)
	at.auditProtectiveStop(defaultProtectorEngine(), p0Obs(100, 100, plannedPositionRisk{}))
	if len(ft.stopLossCalls) != 1 || math.Abs(ft.stopLossCalls[0].stopPrice-98.8) > 1e-9 {
		t.Fatalf("want default stop 98.8, got %+v", ft.stopLossCalls)
	}
}

// Price already beyond the stop the position should have had: closed.
func TestAudit_C5_PollAuditClosesWhenPriceIsBeyondStop(t *testing.T) {
	ft := &nakedTestTrader{}
	ft.live = []map[string]interface{}{{"symbol": "AAAUSDT", "side": "long", "positionAmt": 1.0}}
	at := newNakedTestAutoTrader(ft, 1)
	if !at.auditProtectiveStop(defaultProtectorEngine(), p0Obs(100, 97, plannedPositionRisk{stopLoss: 97.5})) {
		t.Fatal("want the position closed")
	}
	if ft.closes != 1 || len(ft.live) != 0 || len(ft.stopLossCalls) != 0 {
		t.Fatalf("closes=%d live=%v stops=%v", ft.closes, ft.live, ft.stopLossCalls)
	}
}

// Stop present: nothing placed. Stop check failing: alert raised. Young
// position: skipped (the open path is still placing its stop).
func TestAudit_C5_PollAuditStopPresentCheckFailureAndGrace(t *testing.T) {
	ft := &nakedTestTrader{stopOpen: true}
	at := newNakedTestAutoTrader(ft, 1)
	at.auditProtectiveStop(defaultProtectorEngine(), p0Obs(100, 100, plannedPositionRisk{stopLoss: 97.5}))
	if len(ft.stopLossCalls) != 0 {
		t.Fatal("must not place a stop when one is open")
	}
	ft.stopOpenErr = errors.New("openOrders 503")
	at.auditProtectiveStop(defaultProtectorEngine(), p0Obs(100, 100, plannedPositionRisk{stopLoss: 97.5}))
	if len(at.NakedPositionAlerts()) != 1 {
		t.Fatal("an unverifiable stop must raise the alert")
	}
	ft.stopOpenErr, ft.stopOpen = nil, false
	young := p0Obs(100, 100, plannedPositionRisk{stopLoss: 97.5})
	young.OpenedAt = young.Now.Add(-5 * time.Second)
	at.auditProtectiveStop(defaultProtectorEngine(), young)
	if len(ft.stopLossCalls) != 0 {
		t.Fatal("a position inside the grace window must not be re-armed by the poll")
	}
}

// End to end through the protector poll: a naked position is re-armed, and
// the alert of a position that is gone is dropped.
func TestAudit_C5_ProtectorPollRunsTheStopAudit(t *testing.T) {
	ft := &nakedTestTrader{}
	ft.live = []map[string]interface{}{{"symbol": "AAAUSDT", "side": "long", "positionAmt": 1.0, "entryPrice": 100.0, "markPrice": 100.2, "leverage": 10.0}}
	at := newNakedTestAutoTrader(ft, 1)
	at.positionFirstSeenTime["AAAUSDT_long"] = time.Now().Add(-time.Hour).UnixMilli()
	at.raiseNakedPositionAlert("BBBUSDT", "short", "stale alert")
	at.checkPositionDrawdown()
	if len(ft.stopLossCalls) == 0 {
		t.Fatal("the poll must re-arm the missing stop")
	}
	if len(at.NakedPositionAlerts()) != 0 {
		t.Fatalf("alerts left: %+v", at.NakedPositionAlerts())
	}
}

// --------------------------------------------------------------- AUD-C3 ---

// Two opens in one response within the 15 s cache window: the first fills,
// the second must be blocked by max_positions = 1. Runs the open path's
// position check (orders.go) and the real openMarketPosition against an
// exchange that caches positions for 15 s.
func TestAudit_C3_DoubleOpenWithinCacheWindowIsBlocked(t *testing.T) {
	for _, lag := range []bool{false, true} {
		ft := &nakedTestTrader{cacheUse: true, reportLag: lag}
		at := newNakedTestAutoTrader(ft, 1)
		at.beginCycleOpenGate(0)
		openOnce := func(symbol string) error {
			positions, err := at.trader.GetPositions()
			if err != nil {
				return err
			}
			if err := at.enforceMaxPositions(at.effectivePositionCount(len(positions))); err != nil {
				return err
			}
			_, err = at.openMarketPosition(p0Decision(symbol), "long", 1)
			return err
		}
		if err := openOnce("AAAUSDT"); err != nil {
			t.Fatalf("lag=%v: first open: %v", lag, err)
		}
		if ft.invalidatePositionsCalled == 0 {
			t.Fatalf("lag=%v: the position cache must be invalidated after the order", lag)
		}
		err := openOnce("BBBUSDT")
		if err == nil || !strings.Contains(err.Error(), "max positions") {
			t.Fatalf("lag=%v: second open must be blocked by max_positions=1, got %v (opens=%d)", lag, err, ft.opens)
		}
		if ft.opens != 1 {
			t.Fatalf("lag=%v: %d orders reached the exchange, want 1", lag, ft.opens)
		}
		at.endCycleOpenGate()
	}
}

// Before execution the response is capped at the free slots; the excess
// open is recorded as blocked, closes and waits are kept.
func TestAudit_C3_ResponseOpensCappedAtFreeSlots(t *testing.T) {
	at := newNakedTestAutoTrader(&nakedTestTrader{}, 1)
	ds := []kernel.Decision{
		{Symbol: "CCCUSDT", Action: "close_long"},
		{Symbol: "AAAUSDT", Action: "open_long"},
		{Symbol: "BBBUSDT", Action: "open_short"},
		{Symbol: "ALL", Action: "wait"},
	}
	record := &store.DecisionRecord{}
	kept := at.capResponseOpens(&kernel.Context{}, ds, record)
	if len(kept) != 3 || kept[1].Symbol != "AAAUSDT" || kept[2].Action != "wait" {
		t.Fatalf("kept %+v", kept)
	}
	if len(record.Decisions) != 1 || record.Decisions[0].Symbol != "BBBUSDT" || record.Decisions[0].Success ||
		!strings.Contains(record.Decisions[0].Error, "max_positions_response_cap") {
		t.Fatalf("dropped open not recorded: %+v", record.Decisions)
	}
	// With the slot already used by an open position, no open is kept.
	kept = at.capResponseOpens(&kernel.Context{Positions: []kernel.PositionInfo{{Symbol: "DDDUSDT"}}}, ds, &store.DecisionRecord{})
	for _, d := range kept {
		if isOpenAction(d.Action) {
			t.Fatalf("no free slot, but kept %s %s", d.Symbol, d.Action)
		}
	}
	// A pending conditional entry reserves its slot as well.
	at.conditional.pending = map[string]*hunterV7PendingEntry{"x": {}}
	if _, dropped, free := at.capOpensToFreeSlots(ds, 0); free != 0 || len(dropped) != 2 {
		t.Fatalf("pending entry must use the slot: free=%d dropped=%d", free, len(dropped))
	}
}
