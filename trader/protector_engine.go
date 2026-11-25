package trader

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Aixxww/AiT/store"
)

// U14: the position protector ("model A", plan §7.1.2) as one engine over
// config (store.ProtectorConfig). Every PnL threshold is a level with a unit
// (roe_pct / price_pct / r). A level is compared in its own unit, so the
// defaults (today's values in today's units) reproduce the pre-U14 decisions
// exactly; each level and the position's state are also expressed in R
// (multiples of the planned stop distance) for the logs. A level in R needs
// the position's planned stop: without one the rule does not fire, and the
// protector logs that once per position.
//
// The engine holds the decision logic only; the live monitor
// (checkPositionDrawdown) and the replay walker (protectorWalk) both drive
// it through protectPosition, with the clock passed in.

type protectorEngine struct {
	cfg store.ProtectorConfig
	dyn *DynamicStopManager
}

func newProtectorEngine(cfg store.ProtectorConfig) *protectorEngine {
	return &protectorEngine{cfg: cfg, dyn: NewDynamicStopManager(DynamicStopConfig{
		HighVolThreshold:     cfg.HighVolThreshold,
		LowVolThreshold:      cfg.LowVolThreshold,
		HighVolStopWiden:     cfg.HighVolStopWiden,
		LowVolStopTighten:    cfg.LowVolStopTighten,
		TimeDecayEnabled:     !cfg.TimeDecayDisabled,
		TimeDecayAfter:       cfg.TimeDecayAfter(),
		TimeDecayRate:        cfg.TimeDecayRate,
		TimeDecayMaxFraction: cfg.TimeDecayMaxFraction,
		TrailingEnabled:      !cfg.TrailingDisabled,
		TrailingActivationR:  cfg.TrailingActivationR,
		TrailingDistanceR:    cfg.TrailingDistanceR,
	})}
}

// defaultProtectorEngine is the engine of the default config.
func defaultProtectorEngine() *protectorEngine {
	return newProtectorEngine(store.EffectiveProtector(nil))
}

// protectorEngine returns the trader's engine (strategy config, else the
// defaults).
func (at *AutoTrader) protectorEngine() *protectorEngine {
	if at == nil || at.config.StrategyConfig == nil {
		return defaultProtectorEngine()
	}
	return newProtectorEngine(store.EffectiveProtector(at.config.StrategyConfig.RiskControl.Protector))
}

// protectorPos is what a level needs to be read for one position.
type protectorPos struct {
	Side     string
	Leverage int
	// StopFrac is the planned stop distance as a fraction of entry
	// (s in R = ROE / (100 L s)); 0 = unknown.
	StopFrac float64
}

func (state *positionProtectionState) protectorPos(side string, leverage int) protectorPos {
	p := protectorPos{Side: side, Leverage: leverage}
	if state != nil && state.PlannedStopLossROEPct < 0 && leverage > 0 {
		p.StopFrac = -state.PlannedStopLossROEPct / 100 / float64(leverage)
	}
	return p
}

// metric expresses a PnL reading (ROE % and price move %) in a unit.
func (p protectorPos) metric(unit string, roePct, movePct float64) (float64, bool) {
	switch unit {
	case store.ProtectorUnitROEPct:
		return roePct, true
	case store.ProtectorUnitPricePct:
		return movePct, true
	case store.ProtectorUnitR:
		if p.StopFrac <= 0 {
			return 0, false
		}
		return movePct / 100 / p.StopFrac, true
	}
	return 0, false
}

// peakMetric is the peak (tracked in ROE) in a unit.
func (p protectorPos) peakMetric(unit string, peakROE float64) (float64, bool) {
	move := 0.0
	if p.Leverage > 0 {
		move = peakROE / float64(p.Leverage)
	}
	return p.metric(unit, peakROE, move)
}

func (p protectorPos) ge(l store.ProtectorLevel, roePct, movePct float64) bool {
	v, ok := p.metric(l.Unit, roePct, movePct)
	return ok && v >= l.Value
}

// minMoveOK is an optional minimum-move gate (#93 / AUD-D3): an unset level
// (no valid unit) always passes, which is the behaviour before #93.
func (p protectorPos) minMoveOK(l store.ProtectorLevel, roePct, movePct float64) bool {
	if !store.ValidProtectorUnit(l.Unit) {
		return true
	}
	return p.ge(l, roePct, movePct)
}

func (p protectorPos) gt(l store.ProtectorLevel, roePct, movePct float64) bool {
	v, ok := p.metric(l.Unit, roePct, movePct)
	return ok && v > l.Value
}

func (p protectorPos) le(l store.ProtectorLevel, roePct, movePct float64) bool {
	v, ok := p.metric(l.Unit, roePct, movePct)
	return ok && v <= l.Value
}

func (p protectorPos) peakGE(l store.ProtectorLevel, peakROE float64) bool {
	v, ok := p.peakMetric(l.Unit, peakROE)
	return ok && v >= l.Value
}

// priceFrac is a level as a price distance from entry (fraction of entry),
// dividing at run time (the profit floor did so before U14).
func (p protectorPos) priceFrac(l store.ProtectorLevel) (float64, bool) {
	return p.priceFracWith(l, func(v float64) float64 { return v / 100 })
}

// priceFracConst is priceFrac with the percent divided as a decimal
// constant: the pre-U14 code folded these levels at compile time
// (0.55/100, 0.35), and 0.55/100 at run time differs by one ulp.
func (p protectorPos) priceFracConst(l store.ProtectorLevel) (float64, bool) {
	return p.priceFracWith(l, decimalPercent)
}

// decimalPercent is v/100 rounded once from the decimal value of v.
func decimalPercent(v float64) float64 {
	f, err := strconv.ParseFloat(strconv.FormatFloat(v, 'g', -1, 64)+"e-2", 64)
	if err != nil {
		return v / 100
	}
	return f
}

func (p protectorPos) priceFracWith(l store.ProtectorLevel, pct func(float64) float64) (float64, bool) {
	switch l.Unit {
	case store.ProtectorUnitROEPct:
		if p.Leverage <= 0 {
			return 0, false
		}
		return pct(l.Value) / float64(p.Leverage), true
	case store.ProtectorUnitPricePct:
		return pct(l.Value), true
	case store.ProtectorUnitR:
		if p.StopFrac <= 0 {
			return 0, false
		}
		return l.Value * p.StopFrac, true
	}
	return 0, false
}

// roeOf is a level in ROE % (the hard-loss backstop compares with the
// planned stop in ROE).
func (p protectorPos) roeOf(l store.ProtectorLevel) (float64, bool) {
	switch l.Unit {
	case store.ProtectorUnitROEPct:
		return l.Value, true
	case store.ProtectorUnitPricePct:
		return l.Value * float64(p.Leverage), p.Leverage > 0
	case store.ProtectorUnitR:
		if p.StopFrac <= 0 || p.Leverage <= 0 {
			return 0, false
		}
		return l.Value * 100 * float64(p.Leverage) * p.StopFrac, true
	}
	return 0, false
}

// inR is a level in R (for logs); ok is false without a planned stop.
func (p protectorPos) inR(l store.ProtectorLevel) (float64, bool) {
	if p.StopFrac <= 0 {
		return 0, false
	}
	switch l.Unit {
	case store.ProtectorUnitROEPct:
		if p.Leverage <= 0 {
			return 0, false
		}
		return l.Value / 100 / float64(p.Leverage) / p.StopFrac, true
	case store.ProtectorUnitPricePct:
		return l.Value / 100 / p.StopFrac, true
	case store.ProtectorUnitR:
		return l.Value, true
	}
	return 0, false
}

// roeInR is a ROE reading in R (for logs).
func (p protectorPos) roeInR(roePct float64) (float64, bool) {
	if p.StopFrac <= 0 || p.Leverage <= 0 {
		return 0, false
	}
	return roePct / 100 / float64(p.Leverage) / p.StopFrac, true
}

// levels lists the config levels by name (logs, R-availability check).
func (e *protectorEngine) levels() []struct {
	Name  string
	Level store.ProtectorLevel
} {
	c := e.cfg
	return []struct {
		Name  string
		Level store.ProtectorLevel
	}{
		{"hard_loss", c.HardLoss}, {"early_loss", c.EarlyLoss},
		{"profit_floor_peak", c.ProfitFloorPeak}, {"profit_floor_buffer", c.ProfitFloorBuffer}, {"profit_floor_net_buffer", c.ProfitFloorNetBuffer},
		{"profit_lock_mid_peak", c.ProfitLockMidPeak}, {"profit_lock_high_peak", c.ProfitLockHighPeak},
		{"near_tp1_peak", c.NearTP1Peak}, {"near_tp1_second_chance", c.NearTP1SecondChance},
		{"tp0", c.TP0}, {"tp1", c.TP1}, {"tp1_min_move", c.TP1MinMove}, {"tp2", c.TP2}, {"tp2_min_move", c.TP2MinMove},
		{"near_tp1_loss_exit", c.NearTP1LossExit}, {"pre_tp_min_current", c.PreTPMinCurrent}, {"pre_tp_peak", c.PreTPPeak},
		{"dynamic_stop_early_peak", c.DynamicStopEarlyPeak}, {"dynamic_stop_early_move", c.DynamicStopEarlyMove},
		{"dynamic_stop_base_risk", c.DynamicStopBaseRisk}, {"micro_tp0_min_move", c.MicroTP0MinMove}, {"log_min_pnl", c.LogMinPnL},
	}
}

// levelsInR is the log line of every level in R for a position.
func (e *protectorEngine) levelsInR(p protectorPos) string {
	parts := make([]string, 0, 24)
	for _, l := range e.levels() {
		if r, ok := p.inR(l.Level); ok {
			parts = append(parts, fmt.Sprintf("%s %+.3fR", l.Name, r))
		}
	}
	return strings.Join(parts, ", ")
}

// rLevelsUnavailable names the R-unit levels a position without a planned
// stop cannot evaluate.
func (e *protectorEngine) rLevelsUnavailable(p protectorPos) []string {
	if p.StopFrac > 0 {
		return nil
	}
	var out []string
	for _, l := range e.levels() {
		if l.Level.Unit == store.ProtectorUnitR {
			out = append(out, l.Name)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Decision rules
// ---------------------------------------------------------------------------

func protectorDrawdown(state *positionProtectionState, currentPnLPct float64) float64 {
	if state.PeakPnLPct > 0 && currentPnLPct < state.PeakPnLPct {
		return ((state.PeakPnLPct - currentPnLPct) / state.PeakPnLPct) * 100
	}
	return 0
}

// choose applies the protector rules in order (first match wins) at now.
// It raises the state's peak. Giveback and trail rules fire at most once
// per position (F30).
func (e *protectorEngine) choose(state *positionProtectionState, p protectorPos, cur, move float64, now time.Time) (protectionAction, float64) {
	if state == nil {
		return protectionNone, 0
	}
	c := e.cfg
	if cur > state.PeakPnLPct {
		state.PeakPnLPct = cur
	}
	peak := state.PeakPnLPct
	dd := protectorDrawdown(state, cur)
	// age is 0 for an unknown open time (rule 1); the pre-TP1 rules read
	// the raw difference, as before U14.
	age := time.Duration(0)
	if !state.OpenedAt.IsZero() {
		age = now.Sub(state.OpenedAt)
	}
	rawAge := now.Sub(state.OpenedAt)

	// 1. Hard loss (P0-2: a wider planned stop pushes the backstop out).
	if thr, ok := p.roeOf(c.HardLoss); ok {
		if state.PlannedStopLossROEPct < thr {
			thr = state.PlannedStopLossROEPct * c.HardLossSlackMultiplier
		}
		if cur <= thr || (age <= c.EarlyLossWindow() && p.le(c.EarlyLoss, cur, move)) {
			return protectionHardLossClose, dd
		}
	} else if age <= c.EarlyLossWindow() && p.le(c.EarlyLoss, cur, move) {
		return protectionHardLossClose, dd
	}
	giveback := !state.GivebackDone
	// 2. Giveback to zero.
	if giveback && p.peakGE(c.ProfitFloorPeak, peak) && cur <= 0 {
		return protectionGivebackClose, dd
	}
	// 3. TP0 by level.
	if !state.TP0Done && p.peakGE(c.TP0, peak) && p.ge(c.TP0, cur, move) && p.minMoveOK(c.TP0MinMove, cur, move) {
		return protectionTP0, dd
	}
	// 4-5. High / mid profit lock.
	if giveback && p.peakGE(c.ProfitLockHighPeak, peak) && cur > 0 && dd >= c.ProfitLockHighDrawdownPct {
		return protectionGivebackClose, dd
	}
	if giveback && p.peakGE(c.ProfitLockMidPeak, peak) && cur > 0 && dd >= c.ProfitLockMidDrawdownPct {
		return protectionGivebackClose, dd
	}
	// 6-7. TP1.
	if !state.TP1Done && p.ge(c.TP1, cur, move) && p.ge(c.TP1MinMove, cur, move) {
		return protectionTP1, dd
	}
	if state.TP0Done && !state.TP1Done && p.peakGE(c.ProfitLockHighPeak, peak) && p.ge(c.TP0, cur, move) {
		return protectionTP1, dd
	}
	// 8. TP2.
	if state.TP1Done && !state.TP2Done && p.ge(c.TP2, cur, move) && p.ge(c.TP2MinMove, cur, move) {
		return protectionTP2, dd
	}
	// 9. Trail after TP1.
	if !state.TrailDone && state.TP1Done && p.peakGE(c.TP1, peak) && cur > 0 && dd >= c.TrailDrawdownPct {
		return protectionTrailClose, dd
	}
	// 10-12. Pre-TP1 givebacks.
	nearTP1Peak := c.NearTP1Peak
	secondChance := c.NearTP1SecondChance
	if giveback && !state.TP1Done &&
		p.peakGE(nearTP1Peak, peak) &&
		p.ge(c.TP1MinMove, cur, move) &&
		p.ge(secondChance, cur, move) &&
		rawAge >= c.PreTPMinHold() {
		return protectionGivebackClose, dd
	}
	if giveback && !state.TP1Done &&
		p.peakGE(nearTP1Peak, peak) &&
		rawAge >= c.PreTPMinHold() &&
		dd >= c.NearTP1GivebackPct &&
		((p.ge(c.PreTPMinCurrent, cur, move) && p.ge(c.TP1MinMove, cur, move)) || p.le(c.NearTP1LossExit, cur, move)) {
		return protectionGivebackClose, dd
	}
	if giveback && !state.TP1Done &&
		p.peakGE(c.PreTPPeak, peak) &&
		p.ge(c.PreTPMinCurrent, cur, move) &&
		p.ge(c.TP1MinMove, cur, move) &&
		rawAge >= c.PreTPMinHold() &&
		dd >= c.PreTPGivebackPct {
		return protectionGivebackClose, dd
	}
	return protectionNone, dd
}

// chooseWithPrices is choose followed by the planned-price triggers (TP0,
// TP1, TP2), the full decision of one poll.
func (e *protectorEngine) chooseWithPrices(state *positionProtectionState, p protectorPos, cur, move, mark float64, now time.Time) (protectionAction, float64) {
	action, dd := e.choose(state, p, cur, move, now)
	if action == protectionNone && shouldTriggerPlannedTP0Price(p.Side, mark, cur, state) {
		action = protectionTP0
		dd = protectionDrawdownPct(state, cur)
	}
	if action == protectionNone && shouldTriggerPlannedTarget(p.Side, mark, state.PlannedTP1, state.TP1Done) {
		action = protectionTP1
	}
	if action == protectionNone && shouldTriggerPlannedTarget(p.Side, mark, state.PlannedTP2, state.TP2Done) {
		action = protectionTP2
	}
	return action, dd
}

// fastPoll reports whether the next poll uses the fast interval.
func (e *protectorEngine) fastPoll(state *positionProtectionState, p protectorPos, cur, move float64) bool {
	if state == nil {
		return false
	}
	near := e.cfg.NearTP1Peak
	return state.TP1Done || p.peakGE(near, state.PeakPnLPct) || p.ge(near, cur, move)
}

// markDone records a non-closing action's done-flag (F30: giveback and
// trail too).
func markProtectionDone(state *positionProtectionState, action protectionAction) {
	switch action {
	case protectionTP0:
		state.TP0Done = true
	case protectionTP1:
		state.TP1Done = true
	case protectionTP2:
		state.TP2Done = true
	case protectionGivebackClose:
		state.GivebackDone = true
	case protectionTrailClose:
		state.TrailDone = true
	}
}

// partial returns an action's close ratio and the breakeven buffer of the
// stop placed after it.
func (e *protectorEngine) partial(action protectionAction) (ratio, buffer float64, ok bool) {
	c := e.cfg
	switch action {
	case protectionTP0:
		return c.TP0CloseRatio, c.BreakevenBufferFrac, true
	case protectionTP1:
		return c.TP1CloseRatio, c.BreakevenBufferFrac, true
	case protectionTP2:
		return c.TP2CloseRatio, c.BreakevenBufferFrac, true
	case protectionTrailClose, protectionGivebackClose:
		return c.GivebackCloseRatio, c.GivebackBreakevenBufferFrac, true
	}
	return 0, 0, false
}

// ---------------------------------------------------------------------------
// Stops
// ---------------------------------------------------------------------------

func (e *protectorEngine) baseStop(p protectorPos, entry float64) float64 {
	if entry <= 0 || p.Leverage <= 0 {
		return 0
	}
	risk, ok := p.priceFracConst(e.cfg.DynamicStopBaseRisk)
	if !ok || risk <= 0 {
		return 0
	}
	if p.Side == "long" {
		return entry * (1 - risk)
	}
	return entry * (1 + risk)
}

func (e *protectorEngine) profitFloorStop(p protectorPos, entry, peakROE float64) float64 {
	c := e.cfg
	if entry <= 0 || p.Leverage <= 0 || !p.peakGE(c.ProfitFloorPeak, peakROE) {
		return 0
	}
	// #93 / AUD-D3: an owner-set minimum peak price move (unset = none).
	if store.ValidProtectorUnit(c.ProfitFloorMinMove.Unit) && !p.peakGE(c.ProfitFloorMinMove, peakROE) {
		return 0
	}
	lock := 0.0
	for _, l := range []store.ProtectorLevel{c.ProfitFloorBuffer, c.ProfitFloorNetBuffer} {
		if f, ok := p.priceFrac(l); ok && f > lock {
			lock = f
		}
	}
	ratio := 0.0
	switch {
	case p.peakGE(c.ProfitLockHighPeak, peakROE):
		ratio = c.ProfitLockHighRatio
	case p.peakGE(c.ProfitLockMidPeak, peakROE):
		ratio = c.ProfitLockMidRatio
	}
	if ratio > 0 {
		if f, ok := p.priceFrac(store.ProtectorLevel{Value: peakROE * ratio, Unit: store.ProtectorUnitROEPct}); ok && f > lock {
			lock = f
		}
	}
	if lock <= 0 {
		return 0
	}
	if p.Side == "long" {
		return entry * (1 + lock)
	}
	return entry * (1 - lock)
}

func (e *protectorEngine) moreProtective(side string, next, prev float64) bool {
	if next <= 0 {
		return false
	}
	if prev <= 0 {
		return true
	}
	minDelta := prev * e.cfg.MinStopDeltaFrac
	if minDelta <= 0 {
		minDelta = 0.00000001
	}
	if side == "long" {
		return next > prev+minDelta
	}
	return next < prev-minDelta
}

func (e *protectorEngine) delayDynamicStop(state *positionProtectionState, p protectorPos, move float64, now time.Time) bool {
	if state == nil || state.OpenedAt.IsZero() {
		return false
	}
	if now.Sub(state.OpenedAt) >= e.cfg.DynamicStopEarlyHold() {
		return false
	}
	if p.peakGE(e.cfg.DynamicStopEarlyPeak, state.PeakPnLPct) {
		return false
	}
	return !p.ge(e.cfg.DynamicStopEarlyMove, 0, move)
}

// dynamicStopCandidate is the stop the dynamic update would place now, or
// 0 when none is due (cadence, early delay, or not tighter).
func (e *protectorEngine) dynamicStopCandidate(state *positionProtectionState, p protectorPos, qty, entry, mark float64, now time.Time) (candidate, active float64) {
	if state == nil || qty <= 0 || entry <= 0 || mark <= 0 {
		return 0, 0
	}
	if !state.LastStopUpdateAt.IsZero() && now.Sub(state.LastStopUpdateAt) < e.cfg.DynamicStopMinUpdate() {
		return 0, 0
	}
	base := e.baseStop(p, entry)
	if base <= 0 {
		return 0, 0
	}
	active = mostProtectiveStop(p.Side, state.ActiveStopLoss, state.DynamicStop)
	if active > 0 && e.delayDynamicStop(state, p, calculateUnleveragedPnLPct(p.Side, entry, mark), now) {
		return 0, active
	}
	maxFav := state.PeakPnLPct / 100 / float64(p.Leverage) * entry
	if maxFav < 0 {
		maxFav = 0
	}
	stop := e.dyn.CalcDynamicStop(entry, base, mark, maxFav, now.Sub(state.OpenedAt), e.cfg.DynamicStopATRPercentile, p.Side == "long")
	floor := e.profitFloorStop(p, entry, state.PeakPnLPct)
	cand := 0.0
	if isStopOnProtectiveSide(p.Side, stop, mark) {
		cand = mostProtectiveStop(p.Side, cand, stop)
	}
	if isStopOnProtectiveSide(p.Side, floor, mark) {
		cand = mostProtectiveStop(p.Side, cand, floor)
	}
	if cand <= 0 || !e.moreProtective(p.Side, cand, active) {
		return 0, active
	}
	return cand, active
}

// rebuildStop is the stop placed after a partial: the breakeven stop, or
// the active stop when that is more protective (F29: never loosen).
func rebuildStop(state *positionProtectionState, side string, entry, buffer float64) float64 {
	stop := entry * (1 - buffer)
	if side == "long" {
		stop = entry * (1 + buffer)
	}
	if state != nil {
		stop = mostProtectiveStop(side, stop, mostProtectiveStop(side, state.ActiveStopLoss, state.DynamicStop))
	}
	return stop
}

// microTP0 caps the planned TP0 distance (P1-4).
func (e *protectorEngine) microTP0(side string, entry, stopLoss, plannedTP0 float64) float64 {
	if entry <= 0 || plannedTP0 <= 0 {
		return plannedTP0
	}
	stopDist := 0.0
	if stopLoss > 0 {
		if side == "long" {
			stopDist = entry - stopLoss
		} else {
			stopDist = stopLoss - entry
		}
	}
	if stopDist < 0 {
		stopDist = 0
	}
	minMove := 0.0
	if stopDist > 0 || e.cfg.MicroTP0MinMove.Unit != store.ProtectorUnitR {
		p := protectorPos{Side: side, StopFrac: stopDist / entry}
		if f, ok := p.priceFracConst(e.cfg.MicroTP0MinMove); ok {
			minMove = f
		}
	}
	microDist := math.Max(minMove*entry, e.cfg.MicroTP0StopDistRatio*stopDist)
	var plannedDist float64
	if side == "long" {
		plannedDist = plannedTP0 - entry
	} else {
		plannedDist = entry - plannedTP0
	}
	if plannedDist <= 0 || microDist >= plannedDist {
		return plannedTP0
	}
	if side == "long" {
		return entry + microDist
	}
	return entry - microDist
}

// hydrate applies the planned stop and targets of the opening decision.
func (e *protectorEngine) hydrate(state *positionProtectionState, side string, entry float64, leverage int, plan plannedPositionRisk) {
	if state == nil || !(plan.stopLoss > 0 || plan.takeProfit > 0 || plan.tp0Price > 0 || plan.tp1Price > 0 || plan.tp2Price > 0) {
		return
	}
	state.rememberActiveStopLoss(side, plan.stopLoss)
	if plan.stopLoss > 0 && entry > 0 && leverage > 0 {
		state.PlannedStopLossROEPct = calculateLeveragedPnLPct(side, entry, plan.stopLoss, leverage)
	}
	if plan.tp0Price > 0 {
		state.rememberPlannedTakeProfit(side, e.microTP0(side, entry, plan.stopLoss, plan.tp0Price))
	} else {
		state.rememberPlannedTakeProfit(side, plan.takeProfit)
	}
	state.rememberPlannedTarget(side, &state.PlannedTP1, plan.tp1Price)
	state.rememberPlannedTarget(side, &state.PlannedTP2, plan.tp2Price)
	// F31: the open's take-profit, re-placed for the remainder after a
	// partial.
	if plan.takeProfit > 0 {
		state.ExchangeTakeProfit = plan.takeProfit
	}
}
