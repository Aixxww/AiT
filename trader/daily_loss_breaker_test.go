package trader

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// breakerTrader is a trader with a mode and a settable UTC clock.
func breakerTrader(mode string, clock *time.Time) *AutoTrader {
	at := t2StopTrader()
	at.id = "f08-trader"
	at.config.StrategyConfig.RiskControl.DailyLossBreaker = &store.DailyLossBreakerConfig{Mode: mode}
	at.dailyLoss.now = func() time.Time { return *clock }
	return at
}

func TestF08ConfigDefaults(t *testing.T) {
	d := store.EffectiveDailyLossBreaker(nil)
	if d.Mode != store.DailyLossBreakerShadow || d.MaxDailyLossPct != 15 || d.NearTripFraction != 0.8 {
		t.Fatalf("defaults = %+v", d)
	}
	if c := store.EffectiveDailyLossBreaker(&store.DailyLossBreakerConfig{Mode: "ARMED"}); c.Mode != store.DailyLossBreakerShadow {
		t.Fatal("an unknown mode must read as shadow, never armed")
	}
	at := t2StopTrader()
	if at.dailyLossConfig().Mode != store.DailyLossBreakerShadow {
		t.Fatal("a strategy without the block runs in shadow")
	}
}

// Baseline at the first equity of the UTC day, near-trip at 80% of the
// limit, trip at the limit, latched after a recovery, reset at UTC midnight.
func TestF08TripLatchAndUTCReset(t *testing.T) {
	clock := time.Date(2026, 11, 20, 0, 5, 0, 0, time.UTC)
	at := breakerTrader(store.DailyLossBreakerArmed, &clock)
	rec := &store.DecisionRecord{}

	ev := at.recordDailyLossEvaluation(1000, false, rec)
	if !ev.BaselineSet || ev.DayStartEquity != 1000 || ev.Status != dailyLossStatusOK || len(rec.ExecutionLog) != 1 {
		t.Fatalf("baseline: %+v log=%v", ev, rec.ExecutionLog)
	}
	clock = clock.Add(time.Hour)
	if ev = at.recordDailyLossEvaluation(950, false, rec); ev.Status != dailyLossStatusOK || len(rec.ExecutionLog) != 1 {
		t.Fatalf("5%% loss must be ok and unlogged: %+v", ev)
	}
	if ev = at.recordDailyLossEvaluation(880, false, rec); ev.Status != dailyLossStatusNear || ev.Blocks || !strings.Contains(rec.ExecutionLog[1], "near_trip") {
		t.Fatalf("12%% loss must be a logged near-trip: %+v %v", ev, rec.ExecutionLog)
	}
	if err := at.enforceDailyLossBreaker("SOLUSDT", "open_long"); err != nil {
		t.Fatalf("near-trip must not block: %v", err)
	}
	if ev = at.recordDailyLossEvaluation(850, false, rec); ev.Status != dailyLossStatusTripped || !ev.JustTripped || !ev.Blocks {
		t.Fatalf("15%% loss must trip: %+v", ev)
	}
	if !strings.Contains(rec.ExecutionLog[2], "new opens blocked until the next UTC midnight") {
		t.Fatalf("trip log = %q", rec.ExecutionLog[2])
	}
	// Recovery does not untrip the day.
	if ev = at.recordDailyLossEvaluation(990, false, rec); ev.Status != dailyLossStatusTripped || !ev.Blocks {
		t.Fatalf("trip must latch: %+v", ev)
	}
	err := at.enforceDailyLossBreaker("SOLUSDT", "open_long")
	if err == nil || !strings.Contains(err.Error(), "daily_loss_breaker") || !strings.Contains(err.Error(), "2026-11-21T00:00:00Z") {
		t.Fatalf("armed trip must block until the next UTC midnight: %v", err)
	}
	// 23:59 UTC is still the same day; 00:00 UTC is a new day.
	clock = time.Date(2026, 11, 20, 23, 59, 0, 0, time.UTC)
	if at.enforceDailyLossBreaker("SOLUSDT", "open_long") == nil {
		t.Fatal("still blocked before UTC midnight")
	}
	clock = time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	if err := at.enforceDailyLossBreaker("SOLUSDT", "open_long"); err != nil {
		t.Fatalf("a new UTC day unblocks: %v", err)
	}
	if ev = at.recordDailyLossEvaluation(990, false, rec); !ev.BaselineSet || ev.DayStartEquity != 990 || ev.Status != dailyLossStatusOK {
		t.Fatalf("new day baseline: %+v", ev)
	}
}

func TestF08UTCMidnightNotLocalTime(t *testing.T) {
	// 2026-11-20 23:30 at UTC+8 is 15:30 UTC: still 2026-11-20 in UTC.
	cst := time.FixedZone("UTC+8", 8*3600)
	clock := time.Date(2026, 11, 21, 7, 30, 0, 0, cst) // 2026-11-20 23:30 UTC
	at := breakerTrader(store.DailyLossBreakerArmed, &clock)
	if ev := at.evaluateDailyLoss(1000, false); ev.Day != "2026-11-20" {
		t.Fatalf("day = %s, want the UTC day", ev.Day)
	}
}

func TestF08ShadowNeverBlocks(t *testing.T) {
	clock := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	at := breakerTrader(store.DailyLossBreakerShadow, &clock)
	at.trader = &contextTestTrader{}
	rec := &store.DecisionRecord{}
	at.recordDailyLossEvaluation(1000, false, rec)
	ev := at.recordDailyLossEvaluation(700, false, rec)
	if ev.Status != dailyLossStatusTripped || ev.Blocks || !strings.Contains(rec.ExecutionLog[len(rec.ExecutionLog)-1], "shadow: would block") {
		t.Fatalf("shadow trip: %+v %v", ev, rec.ExecutionLog)
	}
	if err := at.enforceDailyLossBreaker("SOLUSDT", "open_long"); err != nil {
		t.Fatalf("shadow must not block: %v", err)
	}
	if err := at.executeDecisionWithRecord(condCtx(), &kernel.Decision{Symbol: "SOLUSDT", Action: "open_long"}, &store.DecisionAction{}); err != nil && strings.Contains(err.Error(), "daily_loss_breaker") {
		t.Fatalf("shadow blocked an open: %v", err)
	}
}

func TestF08OffDoesNothing(t *testing.T) {
	clock := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	at := breakerTrader(store.DailyLossBreakerOff, &clock)
	rec := &store.DecisionRecord{}
	at.recordDailyLossEvaluation(1000, false, rec)
	if ev := at.recordDailyLossEvaluation(100, false, rec); ev.Status != dailyLossStatusOff || len(rec.ExecutionLog) != 0 || at.dailyLoss.state.Day != "" {
		t.Fatalf("off: %+v %v", ev, rec.ExecutionLog)
	}
}

// Armed blocks every open path (cycle opens, conditional registration and
// the open executors that conditional fills call) and no other action.
func TestF08ArmedBlocksOpensOnly(t *testing.T) {
	clock := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	at, _, fired := condTrader(true)
	at.config.StrategyConfig.RiskControl.DailyLossBreaker = &store.DailyLossBreakerConfig{Mode: store.DailyLossBreakerArmed}
	at.dailyLoss.now = func() time.Time { return clock }
	at.evaluateDailyLoss(1000, false)
	if ev := at.evaluateDailyLoss(800, false); !ev.Blocks {
		t.Fatalf("setup: %+v", ev)
	}
	isBreaker := func(err error) bool { return err != nil && strings.Contains(err.Error(), "daily_loss_breaker") }
	for _, action := range []string{"open_long", "open_short"} {
		if err := at.executeDecisionWithRecord(condCtx(), &kernel.Decision{Symbol: "SOLUSDT", Action: action}, &store.DecisionAction{}); !isBreaker(err) {
			t.Fatalf("%s: %v", action, err)
		}
	}
	// Conditional entry: no pending entry is registered.
	if err := at.executeDecisionWithRecord(condCtx(), condDecision(), &store.DecisionAction{}); !isBreaker(err) || len(at.pendingConditionalEntries()) != 0 || len(*fired) != 0 {
		t.Fatalf("conditional registration: %v pending=%d", err, len(at.pendingConditionalEntries()))
	}
	// Conditional fills call the open executors directly.
	if err := at.executeOpenLongWithRecord(condCtx(), condDecision(), &store.DecisionAction{}); !isBreaker(err) {
		t.Fatalf("open long executor: %v", err)
	}
	if err := at.executeOpenShortWithRecord(condCtx(), &kernel.Decision{Symbol: "SOLUSDT", Action: "open_short"}, &store.DecisionAction{}); !isBreaker(err) {
		t.Fatalf("open short executor: %v", err)
	}
	for _, action := range []string{"hold", "wait", "close_long", "close_short"} {
		if err := at.executeDecisionWithRecord(condCtx(), &kernel.Decision{Symbol: "SOLUSDT", Action: action}, &store.DecisionAction{}); isBreaker(err) {
			t.Fatalf("%s must not be blocked: %v", action, err)
		}
	}
}

func TestF08StaleOrMissingEquityDoesNotMoveTheBaseline(t *testing.T) {
	clock := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	at := breakerTrader(store.DailyLossBreakerArmed, &clock)
	if ev := at.evaluateDailyLoss(0, false); ev.Status != dailyLossStatusNoEquity || ev.BaselineSet {
		t.Fatalf("no equity: %+v", ev)
	}
	if ev := at.evaluateDailyLoss(500, true); ev.Status != dailyLossStatusStaleData || ev.BaselineSet || ev.Blocks {
		t.Fatalf("stale: %+v", ev)
	}
	if ev := at.evaluateDailyLoss(1000, false); !ev.BaselineSet || ev.DayStartEquity != 1000 {
		t.Fatalf("first fresh equity sets the baseline: %+v", ev)
	}
	if ev := at.evaluateDailyLoss(100, true); ev.Status != dailyLossStatusStaleData || ev.Blocks {
		t.Fatalf("stale equity must not trip: %+v", ev)
	}
}

// The day-start equity and the trip survive a restart of the same trader.
func TestF08PersistsAcrossRestart(t *testing.T) {
	st, err := store.New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	clock := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	a := breakerTrader(store.DailyLossBreakerArmed, &clock)
	a.store = st
	a.evaluateDailyLoss(1000, false)

	clock = clock.Add(2 * time.Hour)
	b := breakerTrader(store.DailyLossBreakerArmed, &clock) // restart
	b.store = st
	ev := b.evaluateDailyLoss(900, false)
	if ev.BaselineSet || ev.DayStartEquity != 1000 || ev.LossPct != 10 {
		t.Fatalf("restart must keep the baseline: %+v", ev)
	}
	b.evaluateDailyLoss(840, false) // trips at 16%

	c := breakerTrader(store.DailyLossBreakerArmed, &clock) // restart after the trip
	c.store = st
	if err := c.enforceDailyLossBreaker("SOLUSDT", "open_long"); err == nil {
		t.Fatal("a restart must keep the trip for the day")
	}
	other := breakerTrader(store.DailyLossBreakerArmed, &clock)
	other.id = "another-trader"
	other.store = st
	if err := other.enforceDailyLossBreaker("SOLUSDT", "open_long"); err != nil {
		t.Fatalf("state is per trader: %v", err)
	}
	clock = time.Date(2026, 11, 21, 0, 1, 0, 0, time.UTC)
	d := breakerTrader(store.DailyLossBreakerArmed, &clock)
	d.store = st
	if ev := d.evaluateDailyLoss(840, false); !ev.BaselineSet || ev.DayStartEquity != 840 || ev.Blocks {
		t.Fatalf("a restart on a new UTC day starts a new baseline: %+v", ev)
	}
}
