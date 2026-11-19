package trader

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/store"
)

// F08: daily-loss breaker. Evaluated once per cycle on the cycle's account
// equity; the trip latches for the rest of the UTC day. In armed mode a
// tripped breaker blocks every new open (cycle opens, conditional-entry
// registrations and conditional fills) until the next UTC midnight. Closes,
// holds and the protector are never touched, and nothing is force-closed.
// In shadow mode the same evaluation is recorded and nothing is blocked.

// Evaluation statuses.
const (
	dailyLossStatusOff       = "off"
	dailyLossStatusOK        = "ok"
	dailyLossStatusNear      = "near_trip"
	dailyLossStatusTripped   = "tripped"
	dailyLossStatusNoEquity  = "no_equity"
	dailyLossStatusStaleData = "stale_equity"
)

type dailyLossBreaker struct {
	mu     sync.Mutex
	state  store.DailyLossBreakerState
	loaded bool
	// now is the clock (nil = time.Now); tests override it.
	now func() time.Time
}

type dailyLossEvaluation struct {
	Mode           string
	Status         string
	Day            string
	DayStartEquity float64
	Equity         float64
	LossPct        float64
	LimitPct       float64
	NearPct        float64
	Blocks         bool // armed and tripped: new opens are blocked
	BaselineSet    bool // the day-start equity was set by this evaluation
	JustTripped    bool
}

func (at *AutoTrader) dailyLossConfig() store.DailyLossBreakerConfig {
	if at.config.StrategyConfig == nil {
		return store.EffectiveDailyLossBreaker(nil)
	}
	return store.EffectiveDailyLossBreaker(at.config.StrategyConfig.RiskControl.DailyLossBreaker)
}

func (at *AutoTrader) dailyLossNow() time.Time {
	if at.dailyLoss.now != nil {
		return at.dailyLoss.now().UTC()
	}
	return time.Now().UTC()
}

// dailyLossLoadLocked restores the persisted state once. Caller holds mu.
func (at *AutoTrader) dailyLossLoadLocked() {
	if at.dailyLoss.loaded {
		return
	}
	at.dailyLoss.loaded = true
	if at.store == nil {
		return
	}
	raw, err := at.store.GetSystemConfig(store.DailyLossBreakerStateKey(at.id))
	if err != nil {
		logger.Warnf("⚠ daily-loss breaker: state load failed: %v", err)
		return
	}
	if raw == "" {
		return
	}
	var st store.DailyLossBreakerState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		logger.Warnf("⚠ daily-loss breaker: state decode failed: %v", err)
		return
	}
	at.dailyLoss.state = st
}

// dailyLossSaveLocked persists the state. Caller holds mu.
func (at *AutoTrader) dailyLossSaveLocked() {
	if at.store == nil {
		return
	}
	raw, err := json.Marshal(at.dailyLoss.state)
	if err == nil {
		err = at.store.SetSystemConfig(store.DailyLossBreakerStateKey(at.id), string(raw))
	}
	if err != nil {
		logger.Warnf("⚠ daily-loss breaker: state save failed: %v", err)
	}
}

// evaluateDailyLoss runs the breaker on one equity reading.
func (at *AutoTrader) evaluateDailyLoss(equity float64, stale bool) dailyLossEvaluation {
	cfg := at.dailyLossConfig()
	ev := dailyLossEvaluation{Mode: cfg.Mode, LimitPct: cfg.MaxDailyLossPct, NearPct: cfg.MaxDailyLossPct * cfg.NearTripFraction, Equity: equity}
	if cfg.Mode == store.DailyLossBreakerOff {
		ev.Status = dailyLossStatusOff
		return ev
	}
	now := at.dailyLossNow()
	today := store.DailyLossDay(now)
	ev.Day = today

	at.dailyLoss.mu.Lock()
	defer at.dailyLoss.mu.Unlock()
	at.dailyLossLoadLocked()
	st := &at.dailyLoss.state
	if st.Day != today {
		*st = store.DailyLossBreakerState{Day: today}
	}
	switch {
	case stale:
		ev.Status = dailyLossStatusStaleData
	case equity <= 0:
		ev.Status = dailyLossStatusNoEquity
	case st.DayStartEquity <= 0:
		st.DayStartEquity = equity
		ev.BaselineSet = true
		at.dailyLossSaveLocked()
	}
	ev.DayStartEquity = st.DayStartEquity
	if ev.Status == "" && st.DayStartEquity > 0 {
		ev.LossPct = (st.DayStartEquity - equity) / st.DayStartEquity * 100
		if !st.Tripped && ev.LossPct >= cfg.MaxDailyLossPct {
			st.Tripped, st.TrippedAt, st.TripLossPct = true, now, ev.LossPct
			ev.JustTripped = true
			at.dailyLossSaveLocked()
		}
	}
	switch {
	case st.Tripped:
		ev.Status = dailyLossStatusTripped // latched for the day
	case ev.Status != "":
	case ev.LossPct >= ev.NearPct:
		ev.Status = dailyLossStatusNear
	default:
		ev.Status = dailyLossStatusOK
	}
	ev.Blocks = st.Tripped && cfg.Mode == store.DailyLossBreakerArmed
	return ev
}

// dailyLossLogLine is the execution-log line of an evaluation, or "" when
// the evaluation is neither a trip, a near-trip nor a new baseline.
func dailyLossLogLine(ev dailyLossEvaluation) string {
	head := fmt.Sprintf("daily-loss breaker [%s] %s: day %s start equity %.2f, equity %.2f, loss %.2f%% (near %.2f%%, limit %.2f%%)",
		ev.Mode, ev.Status, ev.Day, ev.DayStartEquity, ev.Equity, ev.LossPct, ev.NearPct, ev.LimitPct)
	switch ev.Status {
	case dailyLossStatusTripped:
		if ev.Blocks {
			return "⛔ " + head + "; new opens blocked until the next UTC midnight"
		}
		return "⚠ " + head + "; shadow: would block new opens until the next UTC midnight"
	case dailyLossStatusNear:
		return "⚠ " + head
	}
	if ev.BaselineSet {
		return fmt.Sprintf("daily-loss breaker [%s]: day %s start equity set to %.2f", ev.Mode, ev.Day, ev.DayStartEquity)
	}
	return ""
}

// recordDailyLossEvaluation evaluates the breaker on the cycle's equity and
// writes trips, near-trips and new baselines to the execution log.
func (at *AutoTrader) recordDailyLossEvaluation(equity float64, stale bool, record *store.DecisionRecord) dailyLossEvaluation {
	ev := at.evaluateDailyLoss(equity, stale)
	if line := dailyLossLogLine(ev); line != "" {
		if record != nil {
			record.ExecutionLog = append(record.ExecutionLog, line)
		}
		if ev.Status == dailyLossStatusTripped || ev.Status == dailyLossStatusNear {
			at.logWarnf("%s", line)
		} else {
			at.logInfof("%s", line)
		}
	}
	return ev
}

// enforceDailyLossBreaker is the open-path gate. It reads the latched state
// of the current UTC day: armed + tripped returns an error; shadow +
// tripped only logs that it would have blocked.
func (at *AutoTrader) enforceDailyLossBreaker(symbol, action string) error {
	cfg := at.dailyLossConfig()
	if cfg.Mode == store.DailyLossBreakerOff {
		return nil
	}
	now := at.dailyLossNow()
	at.dailyLoss.mu.Lock()
	at.dailyLossLoadLocked()
	st := at.dailyLoss.state
	at.dailyLoss.mu.Unlock()
	if !st.Tripped || st.Day != store.DailyLossDay(now) {
		return nil
	}
	if cfg.Mode != store.DailyLossBreakerArmed {
		logger.Warnf("  ⚠ daily-loss breaker [shadow]: would block %s %s (loss %.2f%% >= %.2f%% on %s)", action, symbol, st.TripLossPct, cfg.MaxDailyLossPct, st.Day)
		return nil
	}
	return fmt.Errorf("❌ daily-loss breaker [armed]: %s %s blocked: day %s loss %.2f%% reached the %.2f%% limit; new opens resume after %s UTC (daily_loss_breaker)",
		action, symbol, st.Day, st.TripLossPct, cfg.MaxDailyLossPct, nextUTCMidnight(now).Format(time.RFC3339))
}

func nextUTCMidnight(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC)
}
