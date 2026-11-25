package trader

// #92 / audit cluster 2: fake-exchange tests for the lifecycle record, the
// fill read-back, the C7 book re-read, the per-position lock and the
// conditional-entry persistence.

import (
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// lifecycleFake: a Binance-like exchange (client-id opens, order status,
// side-scoped cancel, stop check) with scripted fills, a movable book, an
// optional crash right after the fill, and an event log for race tests.
type lifecycleFake struct {
	nakedTestTrader

	mu     sync.Mutex
	events []string

	bookPrice      float64
	bookAfterMode  float64 // book price once SetMarginMode returned (C7); 0 = unchanged
	fillQty        float64 // 0 = requested quantity
	fillPrice      float64
	reportFill     bool   // RESULT-style response carrying executedQty/avgPrice
	statusOverride string // GetOrderStatus status (default FILLED)
	statusQty      float64
	crashAfterFill bool

	closeGate    chan struct{} // non-nil: the first close blocks until closeGateRelease is closed
	closeEntered chan struct{}
	release      chan struct{}
	ownedCancels []string
}

func (f *lifecycleFake) log(e string) {
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
}

func (f *lifecycleFake) eventLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func (f *lifecycleFake) GetMarketPrice(string) (float64, error) {
	if f.bookPrice > 0 {
		return f.bookPrice, nil
	}
	return 100, nil
}

func (f *lifecycleFake) SetMarginMode(string, bool) error {
	if f.bookAfterMode > 0 {
		f.bookPrice = f.bookAfterMode
	}
	return nil
}

func (f *lifecycleFake) open(symbol, side string, qty float64) (map[string]interface{}, error) {
	f.opens++
	q := qty
	if f.fillQty > 0 {
		q = f.fillQty
	}
	amt := q
	if side == "short" {
		amt = -q
	}
	if q > 0 && !strings.EqualFold(f.statusOverride, "EXPIRED") {
		f.live = append(f.live, map[string]interface{}{"symbol": symbol, "side": side, "positionAmt": amt,
			"entryPrice": f.fillPrice, "markPrice": f.fillPrice, "leverage": 5.0})
	}
	if f.crashAfterFill {
		panic("process killed between fill and save")
	}
	order := map[string]interface{}{"orderId": int64(1000 + f.opens), "status": "NEW"}
	if f.reportFill {
		order["executedQty"], order["avgPrice"] = q, f.fillPrice
	}
	return order, nil
}

func (f *lifecycleFake) OpenLongWithClientID(symbol string, qty float64, _ int, _ string) (map[string]interface{}, error) {
	return f.open(symbol, "long", qty)
}
func (f *lifecycleFake) OpenShortWithClientID(symbol string, qty float64, _ int, _ string) (map[string]interface{}, error) {
	return f.open(symbol, "short", qty)
}

func (f *lifecycleFake) GetOrderStatus(string, string) (map[string]interface{}, error) {
	status := f.statusOverride
	if status == "" {
		status = "FILLED"
	}
	q := f.statusQty
	if q == 0 && f.fillQty > 0 && status != "EXPIRED" {
		q = f.fillQty
	}
	return map[string]interface{}{"status": status, "executedQty": q, "avgPrice": f.fillPrice}, nil
}

func (f *lifecycleFake) CancelOwnedProtectionOrders(symbol, positionSide string, stopsOnly bool) error {
	f.log("cancel-owned:" + positionSide)
	f.ownedCancels = append(f.ownedCancels, symbol+"|"+positionSide)
	return nil
}

func (f *lifecycleFake) ProtectiveStopOpen(symbol, side string) (bool, error) {
	f.log("stop-check")
	return f.stopOpen, f.stopOpenErr
}

func (f *lifecycleFake) closeWithGate(symbol, side string) (map[string]interface{}, error) {
	f.log("close-begin")
	f.mu.Lock()
	block := f.closeGate != nil
	f.closeGate = nil
	f.mu.Unlock()
	if block {
		release := f.closeGateRelease()
		if f.closeEntered != nil {
			close(f.closeEntered)
		}
		<-release
	}
	out, err := f.closeSide(symbol, side)
	f.log("close-end")
	return out, err
}

func (f *lifecycleFake) CloseLong(symbol string, _ float64) (map[string]interface{}, error) {
	return f.closeWithGate(symbol, "long")
}
func (f *lifecycleFake) CloseShort(symbol string, _ float64) (map[string]interface{}, error) {
	return f.closeWithGate(symbol, "short")
}

func newLifecycleTrader(t *testing.T, ft *lifecycleFake, st *store.Store) *AutoTrader {
	t.Helper()
	at := newNakedTestAutoTrader(&ft.nakedTestTrader, 3)
	at.trader = ft
	at.exchange = "binance" // orders are synced by OrderSync; no order-row writes here
	at.store = st
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5
	return at
}

func newLifecycleStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func lcDecision(sym string) *kernel.Decision {
	return &kernel.Decision{Symbol: sym, Action: "open_long", Leverage: 5, PositionSizeUSD: 100, Price: 100,
		StopLoss: 97.5, TakeProfit: 104, Confidence: 80, SelectedHunterV7SignalID: "sig-1"}
}

func lifecycleRows(t *testing.T, st *store.Store, traderID string) []store.PositionLifecycle {
	t.Helper()
	rows, err := st.Decision().ListLifecycles(traderID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return rows
}

// E3: the process dies after the exchange filled the order and before
// anything else was saved. The intent row carries the plan; the next start
// adopts the exchange position and places the fill-anchored stop and TP.
func TestLifecycle_CrashBetweenFillAndSaveIsReconciled(t *testing.T) {
	st := newLifecycleStore(t)
	ft := &lifecycleFake{fillPrice: 100.4, crashAfterFill: true}
	at := newLifecycleTrader(t, ft, st)

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected the simulated crash")
			}
		}()
		_ = at.placeOpenOrderWithLifecycle(lcDecision("AAAUSDT"), "long", 1, 100, 100, false, false, &store.DecisionAction{})
	}()
	rows := lifecycleRows(t, st, "p0")
	if len(rows) != 1 || rows[0].Status != store.LifecycleStatusIntent || rows[0].PlanStopLoss != 97.5 ||
		rows[0].PlanTakeProfit != 104 || rows[0].PlanQuantity != 1 || rows[0].SignalID != "sig-1" {
		t.Fatalf("intent row before the crash: %+v", rows)
	}
	if len(ft.stopLossCalls) != 0 {
		t.Fatal("no stop can have been placed before the crash")
	}
	// A second intent whose order never reached the exchange.
	if err := st.Decision().CreateLifecycle(&store.PositionLifecycle{TraderID: "p0", Symbol: "BBBUSDT", Side: "LONG",
		Status: store.LifecycleStatusIntent, PlanEntryPrice: 50, PlanStopLoss: 49}); err != nil {
		t.Fatal(err)
	}

	// Restart.
	ft.crashAfterFill = false
	at2 := newLifecycleTrader(t, ft, st)
	at2.reconcileLifecycleOnStartup(time.Now())

	rows = lifecycleRows(t, st, "p0")
	if rows[0].Status != store.LifecycleStatusOpen || rows[0].ExecutedQty != 1 || rows[0].AvgFillPrice != 100.4 || !rows[0].FillConfirmed {
		t.Fatalf("filled intent not adopted: %+v", rows[0])
	}
	if math.Abs(rows[0].StopLoss-97.9) > 1e-9 || math.Abs(rows[0].TakeProfit-104.4) > 1e-9 {
		t.Fatalf("stop/TP not fill-anchored: stop %.4f tp %.4f", rows[0].StopLoss, rows[0].TakeProfit)
	}
	if len(ft.stopLossCalls) != 1 || math.Abs(ft.stopLossCalls[0].stopPrice-97.9) > 1e-9 || ft.stopLossCalls[0].quantity != 1 {
		t.Fatalf("reconcile must place the missing stop from the fill: %+v", ft.stopLossCalls)
	}
	if rows[1].Status != store.LifecycleStatusFailed {
		t.Fatalf("intent without a position must be marked failed: %+v", rows[1])
	}
}

// Reconcile does not place a second stop when one is already open.
func TestLifecycle_ReconcileKeepsExistingStop(t *testing.T) {
	st := newLifecycleStore(t)
	ft := &lifecycleFake{fillPrice: 100}
	ft.stopOpen = true
	ft.live = []map[string]interface{}{{"symbol": "AAAUSDT", "side": "long", "positionAmt": 2.0, "entryPrice": 100.0}}
	at := newLifecycleTrader(t, ft, st)
	_ = st.Decision().CreateLifecycle(&store.PositionLifecycle{TraderID: "p0", Symbol: "AAAUSDT", Side: "LONG",
		Status: store.LifecycleStatusIntent, PlanEntryPrice: 100, PlanStopLoss: 98, PlanTakeProfit: 103})
	at.reconcileLifecycleOnStartup(time.Now())
	if len(ft.stopLossCalls) != 0 {
		t.Fatalf("stop already open: none must be placed, got %+v", ft.stopLossCalls)
	}
	if rows := lifecycleRows(t, st, "p0"); rows[0].Status != store.LifecycleStatusOpen || rows[0].ExecutedQty != 2 {
		t.Fatalf("row: %+v", rows[0])
	}
}

// C4: a partial fill (ACK response, fill read from the order status) is
// recorded and protected at the executed quantity and the average price.
func TestLifecycle_PartialFillUsesExecutedQtyAndAvgPrice(t *testing.T) {
	st := newLifecycleStore(t)
	ft := &lifecycleFake{fillQty: 0.6, fillPrice: 100.2, statusOverride: "PARTIALLY_FILLED", statusQty: 0.6}
	at := newLifecycleTrader(t, ft, st)
	rec := &store.DecisionAction{}
	d := lcDecision("AAAUSDT")
	if err := at.placeOpenOrderWithLifecycle(d, "long", 1, 100, 100, false, false, rec); err != nil {
		t.Fatalf("open: %v", err)
	}
	if rec.Quantity != 0.6 || rec.Price != 100.2 || !rec.PositionWasReduced {
		t.Fatalf("record must carry the fill, got qty %.4f price %.4f reduced %v", rec.Quantity, rec.Price, rec.PositionWasReduced)
	}
	if len(ft.stopLossCalls) != 1 || ft.stopLossCalls[0].quantity != 0.6 || math.Abs(ft.stopLossCalls[0].stopPrice-97.7) > 1e-9 {
		t.Fatalf("stop must use the executed qty and the fill-anchored price: %+v", ft.stopLossCalls)
	}
	if math.Abs(rec.FinalStopLoss-97.7) > 1e-9 || math.Abs(rec.FinalEffectiveTakeProfit-104.2) > 1e-9 {
		t.Fatalf("final stop/TP: %.4f / %.4f", rec.FinalStopLoss, rec.FinalEffectiveTakeProfit)
	}
	rows := lifecycleRows(t, st, "p0")
	if len(rows) != 1 || rows[0].Status != store.LifecycleStatusOpen || rows[0].ExecutedQty != 0.6 ||
		rows[0].AvgFillPrice != 100.2 || rows[0].PlanQuantity != 1 || !rows[0].FillConfirmed {
		t.Fatalf("row: %+v", rows)
	}
}

// C4: a RESULT response carries the fill directly; no status read needed.
func TestLifecycle_FillFromOrderResponse(t *testing.T) {
	st := newLifecycleStore(t)
	ft := &lifecycleFake{fillPrice: 99.8, reportFill: true}
	at := newLifecycleTrader(t, ft, st)
	rec := &store.DecisionAction{}
	if err := at.placeOpenOrderWithLifecycle(lcDecision("AAAUSDT"), "long", 1, 100, 100, false, false, rec); err != nil {
		t.Fatal(err)
	}
	if rec.Price != 99.8 || math.Abs(ft.stopLossCalls[0].stopPrice-97.3) > 1e-9 {
		t.Fatalf("price %.4f stop %+v", rec.Price, ft.stopLossCalls)
	}
	if rows := lifecycleRows(t, st, "p0"); !strings.Contains(rows[0].Note, "order") {
		t.Fatalf("fill source not recorded: %q", rows[0].Note)
	}
}

// C4: an order that ended with nothing executed and no position is not
// protected as if filled; the row is marked failed.
func TestLifecycle_TerminalOrderWithoutFillFails(t *testing.T) {
	st := newLifecycleStore(t)
	ft := &lifecycleFake{fillPrice: 100, statusOverride: "EXPIRED"} // open() adds no position for EXPIRED
	at := newLifecycleTrader(t, ft, st)
	err := at.placeOpenOrderWithLifecycle(lcDecision("AAAUSDT"), "long", 1, 100, 100, false, false, &store.DecisionAction{})
	if err == nil || !strings.Contains(err.Error(), "without a fill") {
		t.Fatalf("want a no-fill error, got %v", err)
	}
	if len(ft.stopLossCalls) != 0 {
		t.Fatal("no stop for an order that did not fill")
	}
	if rows := lifecycleRows(t, st, "p0"); rows[0].Status != store.LifecycleStatusFailed {
		t.Fatalf("row: %+v", rows[0])
	}
}

// C7: the book moved during the pre-order REST calls; the re-read applies
// the drift check again and the order is not sent.
func TestLifecycle_BookReReadBeforeOrderRejectsDrift(t *testing.T) {
	st := newLifecycleStore(t)
	ft := &lifecycleFake{bookPrice: 100.1, bookAfterMode: 100.9, fillPrice: 100.9}
	at := newLifecycleTrader(t, ft, st)
	err := at.placeOpenOrderWithLifecycle(lcDecision("AAAUSDT"), "long", 1, 100.1, 100, false, false, &store.DecisionAction{})
	if err == nil || !strings.Contains(err.Error(), "book re-read before order") || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("want a drift rejection on the re-read, got %v", err)
	}
	if ft.opens != 0 || len(lifecycleRows(t, st, "p0")) != 0 {
		t.Fatalf("no order and no intent row may exist: opens=%d", ft.opens)
	}
	// In band after the re-read: the order goes out.
	ft2 := &lifecycleFake{bookPrice: 100.1, bookAfterMode: 100.3, fillPrice: 100.3}
	at2 := newLifecycleTrader(t, ft2, newLifecycleStore(t))
	if err := at2.placeOpenOrderWithLifecycle(lcDecision("AAAUSDT"), "long", 1, 100.1, 100, false, false, &store.DecisionAction{}); err != nil {
		t.Fatalf("in-band re-read must open: %v", err)
	}
	if ft2.opens != 1 {
		t.Fatalf("opens=%d", ft2.opens)
	}
}

// D6: the LLM close holds the position lock; the protector poll that
// observed the position before the close waits, then skips it. Its stop
// audit would otherwise close a second time / treat the side as naked.
func TestLifecycle_LLMCloseVsProtectorDoNotInterleave(t *testing.T) {
	ft := &lifecycleFake{closeGate: make(chan struct{}), closeEntered: make(chan struct{})}
	ft.live = []map[string]interface{}{{"symbol": "AAAUSDT", "side": "long", "positionAmt": 1.0, "entryPrice": 100.0}}
	at := newLifecycleTrader(t, ft, nil)
	e := defaultProtectorEngine()
	observedAt := time.Now()
	obs := p0Obs(100, 97, plannedPositionRisk{stopLoss: 97.5}) // beyond the stop, no stop open: the audit would close

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := at.llmClosePosition("AAAUSDT", "long"); err != nil {
			t.Errorf("llm close: %v", err)
		}
	}()
	<-ft.closeEntered // the LLM close is inside the exchange call, holding the lock
	protectorDone := make(chan struct{})
	go func() {
		at.protectObservedPosition(e, obs, observedAt)
		close(protectorDone)
	}()
	select {
	case <-protectorDone:
		t.Fatal("the protector ran while the LLM close held the position")
	case <-time.After(50 * time.Millisecond):
	}
	close(ft.closeGateRelease())
	wg.Wait()
	<-protectorDone

	ev := ft.eventLog()
	want := []string{"close-begin", "close-end", "cancel-owned:LONG"}
	if strings.Join(ev, ",") != strings.Join(want, ",") {
		t.Fatalf("events %v, want %v (protector must skip the closed position)", ev, want)
	}
	if ft.closes != 1 {
		t.Fatalf("closes=%d, want 1", ft.closes)
	}
	if len(at.NakedPositionAlerts()) != 0 {
		t.Fatal("no false naked alert after the LLM close")
	}
}

// D6 the other way round: a protector action in progress finishes before
// the LLM close touches the position.
func TestLifecycle_ProtectorActionCompletesBeforeLLMClose(t *testing.T) {
	ft := &lifecycleFake{closeGate: make(chan struct{}), closeEntered: make(chan struct{})}
	ft.live = []map[string]interface{}{{"symbol": "AAAUSDT", "side": "long", "positionAmt": 1.0, "entryPrice": 100.0}}
	at := newLifecycleTrader(t, ft, nil)
	e := defaultProtectorEngine()
	obs := p0Obs(100, 97, plannedPositionRisk{stopLoss: 97.5})

	protectorDone := make(chan struct{})
	go func() {
		at.protectObservedPosition(e, obs, time.Now())
		close(protectorDone)
	}()
	<-ft.closeEntered // protector emergency close in progress
	llmDone := make(chan struct{})
	go func() {
		_, _ = at.llmClosePosition("AAAUSDT", "long")
		close(llmDone)
	}()
	time.Sleep(50 * time.Millisecond)
	for _, e := range ft.eventLog() {
		if e == "cancel-owned:LONG" {
			t.Fatal("the LLM close ran inside the protector action")
		}
	}
	close(ft.closeGateRelease())
	<-protectorDone
	<-llmDone

	ev := ft.eventLog()
	open := 0
	for _, e := range ev {
		switch e {
		case "close-begin":
			open++
			if open > 1 {
				t.Fatalf("two closes interleaved: %v", ev)
			}
		case "close-end":
			open--
		}
	}
	if ev[len(ev)-1] != "cancel-owned:LONG" {
		t.Fatalf("the LLM close must cancel the side's owned protection last: %v", ev)
	}
}

// closeGateRelease returns the channel the blocked close waits on.
func (f *lifecycleFake) closeGateRelease() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.release == nil {
		f.release = make(chan struct{})
	}
	return f.release
}

// ---- C6: conditional entries ------------------------------------------------

func condStoreTrader(t *testing.T, st *store.Store, enabled bool) (*AutoTrader, *[]kernel.Decision) {
	at, _, fired := condTrader(enabled)
	at.store = st
	return at, fired
}

func lifecycleByStatus(t *testing.T, st *store.Store, status string) []store.PositionLifecycle {
	t.Helper()
	rows, err := st.Decision().ListLifecycles("tr1", status)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// A pending entry survives a restart: it is re-armed with its full contract
// (including the hydrated json:"-" fields); a candle that closed through
// the trigger before the re-arm does not fire it, a later one does.
func TestConditionalEntry_RearmedAfterRestart(t *testing.T) {
	st := newLifecycleStore(t)
	at, _ := condStoreTrader(t, st, true)
	t0 := time.Date(2026, 11, 18, 10, 0, 30, 0, time.UTC)
	if err := at.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", &store.DecisionAction{}, t0); err != nil {
		t.Fatal(err)
	}
	if rows := lifecycleByStatus(t, st, store.LifecycleStatusPendingEntry); len(rows) != 1 || rows[0].PendingPayload == "" {
		t.Fatalf("pending entry not persisted: %+v", rows)
	}

	// Restart two minutes later.
	at2, fired := condStoreTrader(t, st, true)
	m := t0.Truncate(time.Minute)
	restart := m.Add(2*time.Minute + 10*time.Second)
	at2.restorePendingConditionalEntries(restart)
	p := at2.pendingConditionalEntries()
	if len(p) != 1 || p[0].Decision.HunterV7TriggerPrice != 100 || p[0].Decision.HunterV7IntentKey != "tr1|7|IGNUSDT|long|ign-1|conditional" ||
		!p[0].Decision.HunterV7ConditionalEntry || p[0].Candidate.V7ExecutionTier != "REVIEWABLE" || !p[0].ArmedAt.Equal(restart) {
		t.Fatalf("re-armed entry lost its contract: %+v", p)
	}
	at2.conditional.bars = func(string) []datafetch.Kline {
		return []datafetch.Kline{
			bar(m.Add(time.Minute), 100.6, 99.7, 100.3), // through the trigger while stopped: must not fire
			bar(m.Add(2*time.Minute), 100.2, 99.6, 99.9),
			bar(m.Add(3*time.Minute), 100.7, 99.8, 100.5), // after the re-arm: fires
		}
	}
	at2.checkHunterV7ConditionalEntries(m.Add(3*time.Minute + 5*time.Second))
	if len(*fired) != 0 {
		t.Fatal("a candle from before the re-arm must not fire the entry")
	}
	at2.checkHunterV7ConditionalEntries(m.Add(4*time.Minute + 5*time.Second))
	if len(*fired) != 1 {
		t.Fatalf("fired=%d, want 1 after the re-arm", len(*fired))
	}
	if rows := lifecycleByStatus(t, st, store.LifecycleStatusEntryFired); len(rows) != 1 {
		t.Fatal("fired entry row not updated")
	}
}

// Restart after expiry, or with conditional entry disabled: the entry is
// cancelled visibly (row status + reason, decision record), never dropped.
func TestConditionalEntry_CancelledVisiblyOnRestart(t *testing.T) {
	st := newLifecycleStore(t)
	at, _ := condStoreTrader(t, st, true)
	t0 := time.Date(2026, 11, 18, 10, 0, 30, 0, time.UTC)
	_ = at.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", &store.DecisionAction{}, t0)

	at2, _ := condStoreTrader(t, st, true)
	at2.restorePendingConditionalEntries(t0.Add(store.DefaultV7ConditionalEntryMaxPendingMinutes*time.Minute + time.Minute))
	if len(at2.pendingConditionalEntries()) != 0 {
		t.Fatal("an expired entry must not be re-armed")
	}
	rows := lifecycleByStatus(t, st, store.LifecycleStatusEntryCancelled)
	if len(rows) != 1 || !strings.Contains(rows[0].Note, "expired while the trader was stopped") {
		t.Fatalf("cancelled row: %+v", rows)
	}
	recs, err := st.Decision().GetLatestRecords("tr1", 5)
	if err != nil || len(recs) != 1 || !strings.Contains(strings.Join(recs[0].ExecutionLog, " "), "cancelled at restart") {
		t.Fatalf("visible decision record missing: %v %+v", err, recs)
	}

	// Disabled at restart.
	st2 := newLifecycleStore(t)
	at3, _ := condStoreTrader(t, st2, true)
	_ = at3.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", &store.DecisionAction{}, t0)
	at4, _ := condStoreTrader(t, st2, false)
	at4.restorePendingConditionalEntries(t0.Add(time.Minute))
	if rows := lifecycleByStatus(t, st2, store.LifecycleStatusEntryCancelled); len(rows) != 1 || !strings.Contains(rows[0].Note, "disabled") {
		t.Fatalf("disabled restart must cancel visibly: %+v", rows)
	}
}

// C6: the execution guard runs again at fire time on the latest snapshot.
// A high-volatility candidate whose snapshot is 60 s old at the trigger is
// blocked and the entry is cancelled; with a fresh snapshot it fires.
func TestConditionalEntry_GuardRerunAtFireTime(t *testing.T) {
	for _, tc := range []struct {
		name    string
		age     time.Duration
		wantRun bool
	}{{"stale snapshot blocks", 60 * time.Second, false}, {"fresh snapshot fires", 2 * time.Second, true}} {
		t.Run(tc.name, func(t *testing.T) {
			st := newLifecycleStore(t)
			at, fired := condStoreTrader(t, st, true)
			ctx := condCtx()
			ctx.CandidateCoins[0].V7RiskTags = []string{"high_volatility"}
			t0 := time.Date(2026, 11, 18, 10, 0, 30, 0, time.UTC)
			m := t0.Truncate(time.Minute)
			if err := at.registerHunterV7ConditionalEntry(ctx, condDecision(), "long", &store.DecisionAction{}, t0); err != nil {
				t.Fatal(err)
			}
			at.conditional.bars = func(string) []datafetch.Kline { return []datafetch.Kline{bar(m, 100.6, 99.8, 100.4)} }
			at.conditional.latest = func(string) *datafetch.SymbolSnapshot {
				return &datafetch.SymbolSnapshot{Symbol: "IGNUSDT", Price: 100.4, Timestamp: time.Now().Add(-tc.age)}
			}
			at.checkHunterV7ConditionalEntries(m.Add(time.Minute + time.Second))
			if got := len(*fired) == 1; got != tc.wantRun {
				t.Fatalf("executed=%v, want %v", got, tc.wantRun)
			}
			if !tc.wantRun {
				rows := lifecycleByStatus(t, st, store.LifecycleStatusEntryCancelled)
				if len(rows) != 1 || !strings.Contains(rows[0].Note, "execution guard at fire time") {
					t.Fatalf("blocked fire must cancel the entry with the guard reason: %+v", rows)
				}
			}
		})
	}
}

func TestPendingEntryPayloadRoundTrip(t *testing.T) {
	d := condDecision()
	d.HunterV7IntentKey = "k"
	in := &hunterV7PendingEntry{Decision: *d, Candidate: condCtx().CandidateCoins[0], Side: "long", Trigger: 100, Invalidation: 99,
		CreatedAt: time.Unix(100, 0).UTC(), ExpiresAt: time.Unix(1000, 0).UTC()}
	payload, err := encodePendingEntry(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := decodePendingEntry(payload)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision.HunterV7MinRR != 1.2 || out.Decision.HunterV7InvalidationPrice != 99 || out.Decision.HunterV7IntentKey != "k" ||
		out.Candidate.V7SetupType != "ignition_early_long" || !out.ExpiresAt.Equal(in.ExpiresAt) {
		t.Fatalf("round trip lost fields: %+v", out)
	}
	if _, err := decodePendingEntry("not-base64!"); err == nil {
		t.Fatal("bad payload must error")
	}
}
