package trader

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// condTrader: hunter_v7 strategy with conditional entry on, a stub
// exchange, a scripted 1m candle source and a recording executor.
func condTrader(enabled bool) (*AutoTrader, *contextTestTrader, *[]kernel.Decision) {
	at := t2StopTrader()
	at.id, at.callCount = "tr1", 7
	at.isRunning = true
	at.config.StrategyConfig.CoinSource.Hunter = &store.HunterConfig{V7ConditionalEntry: &store.V7ConditionalEntryConfig{Enabled: enabled}}
	ex := &contextTestTrader{}
	at.trader = ex
	var fired []kernel.Decision
	at.conditional.execute = func(_ *kernel.Context, d *kernel.Decision, _ string, _ *store.DecisionAction) error {
		fired = append(fired, *d)
		return nil
	}
	return at, ex, &fired
}

// Long, trigger 100, stop 99 (1%), TP 101.8 (RR 1.8), LLM price = trigger.
func condDecision() *kernel.Decision {
	return &kernel.Decision{Symbol: "IGNUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 30, Price: 100,
		StopLoss: 99, TakeProfit: 101.8, Confidence: 80, SelectedHunterV7SignalID: "ign-1",
		HunterV7MinRR: 1.2, HunterV7MinStopPct: 0.5, HunterV7TriggerPrice: 100, HunterV7ConditionalEntry: true, HunterV7InvalidationPrice: 99}
}

func condCtx() *kernel.Context {
	return &kernel.Context{CandidateCoins: []kernel.CandidateCoin{{Symbol: "IGNUSDT", Direction: "LONG", V7SignalID: "ign-1",
		V7SetupType: "ignition_early_long", V7ExecutionTier: "REVIEWABLE", V7ConditionalEntry: true, V7TriggerPrice: 100}}}
}

func bar(open time.Time, h, l, c float64) datafetch.Kline {
	return datafetch.Kline{OpenTime: open.UnixMilli(), CloseTime: open.Add(time.Minute).UnixMilli() - 1, High: h, Low: l, Close: c}
}

func TestConditionalEntryRegisterInsteadOfOpen(t *testing.T) {
	at, _, fired := condTrader(true)
	rec := &store.DecisionAction{}
	t0 := time.Date(2026, 11, 18, 10, 0, 30, 0, time.UTC)
	if err := at.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", rec, t0); err != nil {
		t.Fatalf("register: %v", err)
	}
	p := at.pendingConditionalEntries()
	if len(p) != 1 || !rec.ConditionalPending || rec.Price != 100 || len(*fired) != 0 {
		t.Fatalf("pending=%d rec=%+v fired=%d", len(p), rec, len(*fired))
	}
	if p[0].Decision.HunterV7IntentKey != "tr1|7|IGNUSDT|long|ign-1|conditional" ||
		!p[0].ExpiresAt.Equal(t0.Add(store.DefaultV7ConditionalEntryMaxPendingMinutes*time.Minute)) {
		t.Fatalf("pending = %+v", p[0])
	}
	if err := at.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", nil, t0); err == nil || !strings.Contains(err.Error(), "conditional_pending_exists") {
		t.Fatalf("duplicate pending must be rejected, got %v", err)
	}
}

func TestConditionalEntryValidatedAtTrigger(t *testing.T) {
	at, _, _ := condTrader(true)
	d := condDecision()
	d.Price = 98.9 // LLM gave the current price, not the trigger: drift 1.1% > 0.5%
	if err := at.registerHunterV7ConditionalEntry(condCtx(), d, "long", nil, time.Now()); err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("price far from the trigger must be rejected, got %v", err)
	}
	d = condDecision()
	d.TakeProfit = 100.5 // RR 0.5 at the trigger
	if err := at.registerHunterV7ConditionalEntry(condCtx(), d, "long", nil, time.Now()); err == nil {
		t.Fatalf("RR below the floor at the trigger must be rejected")
	}
	if len(at.pendingConditionalEntries()) != 0 {
		t.Fatalf("rejected entries must not be pending")
	}
}

// Fail closed: a conditional candidate is never entered at market.
func TestConditionalEntryDisabledFailsClosed(t *testing.T) {
	at, _, fired := condTrader(false)
	err := at.executeDecisionWithRecord(condCtx(), condDecision(), &store.DecisionAction{})
	if err == nil || !strings.Contains(err.Error(), "conditional_entry_disabled") || len(*fired) != 0 {
		t.Fatalf("got %v, fired %d", err, len(*fired))
	}
}

func TestConditionalEntryFiresOnceOnCloseThrough(t *testing.T) {
	at, _, fired := condTrader(true)
	t0 := time.Date(2026, 11, 18, 10, 0, 30, 0, time.UTC)
	m := t0.Truncate(time.Minute)
	at.conditional.bars = func(string) []datafetch.Kline {
		return []datafetch.Kline{
			bar(m.Add(-time.Minute), 102, 99.5, 101),    // closed before registration: ignored
			bar(m, 100.2, 99.4, 99.8),                   // touches 100.2 but closes below the trigger
			bar(m.Add(time.Minute), 100.6, 99.7, 100.3), // closes through the trigger -> fire
			bar(m.Add(2*time.Minute), 101, 100, 100.8),
			bar(m.Add(3*time.Minute), 101, 100, 100.9), // still forming at check time 2
		}
	}
	if err := at.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", nil, t0); err != nil {
		t.Fatal(err)
	}
	at.checkHunterV7ConditionalEntries(m.Add(time.Minute + 5*time.Second)) // only bar m completed: no fire
	if len(*fired) != 0 || len(at.pendingConditionalEntries()) != 1 {
		t.Fatalf("intrabar touch with a close below the trigger must not fire")
	}
	at.checkHunterV7ConditionalEntries(m.Add(2*time.Minute + 5*time.Second))
	if len(*fired) != 1 || len(at.pendingConditionalEntries()) != 0 {
		t.Fatalf("close through the trigger must fire once: fired=%d pending=%d", len(*fired), len(at.pendingConditionalEntries()))
	}
	if (*fired)[0].HunterV7IntentKey != "tr1|7|IGNUSDT|long|ign-1|conditional" {
		t.Fatalf("fired with intent key %q", (*fired)[0].HunterV7IntentKey)
	}
	at.callCount = 99
	at.checkHunterV7ConditionalEntries(m.Add(4*time.Minute + 5*time.Second))
	if len(*fired) != 1 {
		t.Fatalf("a fired entry must never execute again")
	}
}

func TestConditionalEntryInvalidationFirstAndExpiry(t *testing.T) {
	at, _, fired := condTrader(true)
	t0 := time.Date(2026, 11, 18, 10, 0, 30, 0, time.UTC)
	m := t0.Truncate(time.Minute)
	// One candle touches the stop and closes through the trigger: stop first.
	at.conditional.bars = func(string) []datafetch.Kline { return []datafetch.Kline{bar(m, 100.4, 98.9, 100.3)} }
	_ = at.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", nil, t0)
	at.checkHunterV7ConditionalEntries(m.Add(time.Minute + time.Second))
	if len(*fired) != 0 || len(at.pendingConditionalEntries()) != 0 {
		t.Fatalf("invalidation must cancel before the trigger")
	}
	// No trigger before expiry.
	at.conditional.bars = func(string) []datafetch.Kline { return nil }
	_ = at.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", nil, t0)
	at.checkHunterV7ConditionalEntries(t0.Add(store.DefaultV7ConditionalEntryMaxPendingMinutes * time.Minute))
	if len(*fired) != 0 || len(at.pendingConditionalEntries()) != 0 {
		t.Fatalf("expired entry must be dropped")
	}
}

func TestConditionalEntryDroppedWhenStoppedOrSafeMode(t *testing.T) {
	at, _, fired := condTrader(true)
	t0 := time.Date(2026, 11, 18, 10, 0, 30, 0, time.UTC)
	m := t0.Truncate(time.Minute)
	at.conditional.bars = func(string) []datafetch.Kline { return []datafetch.Kline{bar(m, 100.6, 99.8, 100.4)} }
	_ = at.registerHunterV7ConditionalEntry(condCtx(), condDecision(), "long", nil, t0)
	at.safeMode = true
	at.checkHunterV7ConditionalEntries(m.Add(time.Minute + time.Second))
	if len(*fired) != 0 {
		t.Fatalf("safe mode must not open")
	}
}

// F04 pattern for conditional orders: the pending entry's intent key gives
// one deterministic client id across cycles; an ambiguous result is read
// back and adopted, never resubmitted.
func TestF04ConditionalEntryIntentKey(t *testing.T) {
	ft := &intentTestTrader{openErr: errTimeout, lookups: []intentLookup{{order: map[string]interface{}{"orderId": int64(9), "status": "FILLED"}, found: true}}}
	at := intentTrader(ft)
	d := intentDecision()
	d.HunterV7IntentKey = "tr1|42|AAAUSDT|long|sig-1|conditional"
	at.callCount = 50 // the trigger fires in a later cycle
	order, err := at.openMarketPosition(d, "long", 1)
	if err != nil || order["adopted"] != true || len(ft.opens) != 1 || ft.opens[0] != "cid:tr1|42|AAAUSDT|long|sig-1|conditional" {
		t.Fatalf("order=%v err=%v opens=%v", order, err, ft.opens)
	}
	at.callCount = 51
	ft.openErr = nil
	_, _ = at.openMarketPosition(d, "long", 1)
	if ft.opens[1] != ft.opens[0] {
		t.Fatalf("the same pending intent must keep its client id across cycles")
	}
}

// U9 drift contract in the executor.
func TestDecisionDriftContract(t *testing.T) {
	at := t2StopTrader() // strategy drift 0.5
	d := &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 30, Price: 100,
		StopLoss: 98, TakeProfit: 106, Confidence: 80}
	if at.decisionMaxEntryPriceDeviationPct(d) != 0.5 || decisionDriftReferencePrice(d, d.Price) != 100 {
		t.Fatalf("defaults must be the strategy drift from the decision price")
	}
	if err := at.validateOpenDecision(d, 100.6, "long"); err == nil {
		t.Fatalf("0.6%% drift must fail the 0.5%% strategy limit")
	}
	d.HunterV7MaxDriftPct = 0.8
	if err := at.validateOpenDecision(d, 100.6, "long"); err != nil {
		t.Fatalf("class drift 0.8 must accept 0.6%%: %v", err)
	}
	d.HunterV7MaxDriftPct, d.HunterV7DriftReference, d.HunterV7TriggerPrice = 0, store.V7DriftReferenceTrigger, 100.4
	if err := at.validateOpenDecision(d, 100.6, "long"); err != nil {
		t.Fatalf("drift from the trigger 100.4 is 0.2%%: %v", err)
	}
	d.HunterV7TriggerPrice = 0
	if decisionDriftReferencePrice(d, d.Price) != 100 {
		t.Fatalf("trigger reference without a trigger price falls back to the decision price")
	}
	if at.microPriceConsistencyMaxPct() != 0.45 {
		t.Fatalf("consistency default = %v", at.microPriceConsistencyMaxPct())
	}
	at.config.StrategyConfig.RiskControl.MicroPriceConsistencyMaxPct = 0.3
	if at.microPriceConsistencyMaxPct() != 0.3 {
		t.Fatalf("consistency config not read")
	}
}
