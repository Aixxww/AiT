package trader

import (
	"errors"
	"strings"
	"time"

	"github.com/Aixxww/AiT/logger"
)

// U14: one protector step for one position, shared by the live monitor
// (checkPositionDrawdown) and the replay walker (protectorWalk). The clock
// is the step's Now, so a walker over historical bars runs the same code as
// the live poll.

// protectorObs is one observation of a position.
type protectorObs struct {
	Symbol, Side string
	Entry, Mark  float64
	Qty          float64
	Leverage     int
	Now          time.Time
	OpenedAt     time.Time
	Plan         plannedPositionRisk
}

// protectorOutcome is what one step did.
type protectorOutcome struct {
	Action    protectionAction
	ClosedAll bool
	Err       error
	FastPoll  bool
}

// protectPosition runs one protector step: state, plan hydration, dynamic
// stop update, rule decision, action and done-flags.
func (at *AutoTrader) protectPosition(e *protectorEngine, o protectorObs) protectorOutcome {
	cur := calculateLeveragedPnLPct(o.Side, o.Entry, o.Mark, o.Leverage)
	move := calculateUnleveragedPnLPct(o.Side, o.Entry, o.Mark)
	posKey := o.Symbol + "_" + o.Side
	state := at.getOrCreateProtectionState(posKey, o.Qty, cur, o.OpenedAt)
	e.hydrate(state, o.Side, o.Entry, o.Leverage, o.Plan)
	pos := state.protectorPos(o.Side, o.Leverage)
	at.logProtectorLevels(e, state, o, pos)

	if err := at.updateDynamicProtectionStopAt(e, o.Symbol, o.Side, o.Qty, o.Entry, o.Mark, o.Leverage, state, o.Now); err != nil {
		if errors.Is(err, errProtectionEmergencyClosed) {
			logger.Errorf("🚨 Dynamic protection stop update emergency-closed %s %s: %v", o.Symbol, o.Side, err)
			at.markProtectionAction(posKey, protectionHardLossClose, true)
			return protectorOutcome{Action: protectionHardLossClose, ClosedAll: true, Err: err}
		}
		logger.Infof("⚠️ Dynamic protection stop update failed (%s %s): %v", o.Symbol, o.Side, err)
	}
	action, dd := e.chooseWithPrices(state, pos, cur, move, o.Mark, o.Now)
	out := protectorOutcome{Action: action, FastPoll: e.fastPoll(state, pos, cur, move)}
	curR, rOK := pos.roeInR(cur)
	peakR, _ := pos.roeInR(state.PeakPnLPct)
	if action == protectionNone {
		if pos.gt(e.cfg.LogMinPnL, cur, move) {
			if rOK {
				logger.Infof("📊 Position protector: %s %s | Profit %.2f%% (%+.3fR) | Peak %.2f%% (%+.3fR) | Drawdown %.2f%% | Age %s | TP1=%v TP2=%v",
					o.Symbol, o.Side, cur, curR, state.PeakPnLPct, peakR, dd, o.Now.Sub(state.OpenedAt).Truncate(time.Second), state.TP1Done, state.TP2Done)
			} else {
				logger.Infof("📊 Position protector: %s %s | Profit %.2f%% | Peak %.2f%% | Drawdown %.2f%% | Age %s | TP1=%v TP2=%v",
					o.Symbol, o.Side, cur, state.PeakPnLPct, dd, o.Now.Sub(state.OpenedAt).Truncate(time.Second), state.TP1Done, state.TP2Done)
			}
		}
		return out
	}
	if rOK {
		logger.Infof("🛡 Protector %s on %s %s at %+.3fR (peak %+.3fR, drawdown %.2f%%)", action, o.Symbol, o.Side, curR, peakR, dd)
	}
	closedAll, err := at.executeProtectionActionWith(e, o.Symbol, o.Side, o.Qty, o.Mark, o.Entry, action)
	if err != nil {
		logger.Infof("❌ Position protector failed (%s %s %s): %v", o.Symbol, o.Side, action, err)
		out.Err = err
		return out
	}
	at.markProtectionAction(posKey, action, closedAll)
	out.ClosedAll = closedAll
	return out
}

// logProtectorLevels logs, once per position, every level in R and the
// R-unit levels that cannot fire without a planned stop.
func (at *AutoTrader) logProtectorLevels(e *protectorEngine, state *positionProtectionState, o protectorObs, pos protectorPos) {
	if state == nil {
		return
	}
	if !state.levelsLogged && pos.StopFrac > 0 {
		state.levelsLogged = true
		logger.Infof("🛡 Protector levels for %s %s in R (leverage %d, planned stop %.3f%%): %s",
			o.Symbol, o.Side, pos.Leverage, pos.StopFrac*100, e.levelsInR(pos))
	}
	if missing := e.rLevelsUnavailable(pos); len(missing) > 0 && !state.rUnavailableLogged {
		state.rUnavailableLogged = true
		logger.Warnf("⚠ Protector %s %s has no planned stop: R-unit levels do not fire (%s)", o.Symbol, o.Side, strings.Join(missing, ", "))
	}
}
