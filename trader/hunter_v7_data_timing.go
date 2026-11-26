package trader

import (
	"fmt"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/store"
)

// U10: cycle alignment and the snapshot freshness gate (plan §6.2). Both
// are off by default (owner decision); see store.V7DataTimingConfig.

func (at *AutoTrader) dataTimingConfig() store.V7DataTimingConfig {
	if at == nil || at.config.StrategyConfig == nil || at.config.StrategyConfig.CoinSource.Hunter == nil {
		return store.EffectiveV7DataTiming(nil)
	}
	return store.EffectiveV7DataTiming(at.config.StrategyConfig.CoinSource.Hunter.V7DataTiming)
}

// nextAlignedScan is the next scan time after now: the next close of an
// intervalMinutes bar (UTC-aligned) plus settle.
func nextAlignedScan(now time.Time, intervalMinutes int, settle time.Duration) time.Time {
	step := time.Duration(intervalMinutes) * time.Minute
	next := now.Truncate(step).Add(settle)
	for !next.After(now) {
		next = next.Add(step)
	}
	return next
}

// scanChannel returns the channel the main loop waits on for the next scan
// and a function that releases it. Wall clock (default) is the ScanInterval
// ticker; aligned mode (hunter_v7 only, opt-in) is a timer to the next bar
// close plus the settle delay.
func (at *AutoTrader) scanChannel(ticker *time.Ticker, isGrid bool, now time.Time) (<-chan time.Time, func()) {
	cfg := at.dataTimingConfig()
	if isGrid || !at.isHunterV7Strategy() || cfg.CycleAlignment != store.V7CycleAligned {
		return ticker.C, func() {}
	}
	next := nextAlignedScan(now, cfg.AlignIntervalMinutes, time.Duration(cfg.SettleSeconds*float64(time.Second)))
	timer := time.NewTimer(next.Sub(now))
	return timer.C, func() { timer.Stop() }
}

// hunterV7BackendDataAgeMs is the age of the data behind a candidate from
// backend sources only (#119, AUD-E1): price age, else snapshot age; 0 =
// unknown. The model's signal_age_ms is an echo of prompt text and is never
// an input. Both age gates (the U10 snapshot budget and the high-velocity
// freshness check) read this one function.
func hunterV7BackendDataAgeMs(candidate *kernel.CandidateCoin) int64 {
	if candidate == nil {
		return 0
	}
	if candidate.V7DataFreshness.PriceAgeMs > 0 {
		return candidate.V7DataFreshness.PriceAgeMs
	}
	return candidate.V7DataFreshness.SnapshotAgeMs
}

// logHunterV7EchoedAgeMismatch records, for diagnostics only, a model-echoed
// signal_age_ms that disagrees with the backend age. It never changes a
// verdict.
func logHunterV7EchoedAgeMismatch(candidate *kernel.CandidateCoin, decision *kernel.Decision, backendMs int64) {
	if candidate == nil || decision == nil || decision.SignalAgeMs <= 0 || decision.SignalAgeMs == backendMs {
		return
	}
	logger.Infof("ℹ [HUNTER V7] %s: model-echoed signal_age_ms=%d ignored; backend data age %dms is authoritative (AUD-E1)",
		decision.Symbol, decision.SignalAgeMs, backendMs)
}

// validateHunterV7SnapshotBudget is the U10 freshness gate: an open on a
// candidate whose class has a budget is rejected when its data is older.
// No budget configured (default) = no gate. Unknown age passes, as in the
// existing freshness check.
func (at *AutoTrader) validateHunterV7SnapshotBudget(candidate *kernel.CandidateCoin, decision *kernel.Decision) error {
	if candidate == nil || decision == nil {
		return nil
	}
	budget := at.dataTimingConfig().FreshnessBudgetSeconds[candidate.V7SignalClass]
	if budget <= 0 {
		return nil
	}
	ageMs := hunterV7BackendDataAgeMs(candidate)
	logHunterV7EchoedAgeMismatch(candidate, decision, ageMs)
	if ageMs <= 0 || float64(ageMs) <= budget*1000 {
		return nil
	}
	return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: snapshot age %dms exceeds the %s latency budget %.0fs (snapshot_age_budget)",
		candidate.V7SetupType, decision.Symbol, ageMs, candidate.V7SignalClass, budget)
}
