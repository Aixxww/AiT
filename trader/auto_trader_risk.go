package trader

import (
	"context"
	"fmt"
	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/provider/local"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	positionProtectorFastInterval         = 15 * time.Second
	positionProtectorBaseInterval         = 30 * time.Second
	positionProtectorIdleInterval         = 60 * time.Second
	protectorTP1PnLPct                    = 6.0
	protectorTP2PnLPct                    = 12.0
	protectorTP1MinPriceMovePct           = 1.0
	protectorTP2MinPriceMovePct           = 1.5
	protectorTP0PnLPct                    = 8.0
	protectorTP0CloseRatio                = 0.35
	protectorTP1CloseRatio                = 0.40
	protectorTP2CloseRatio                = 0.50
	protectorTrailDrawdownPct             = 35.0
	protectorDynamicStopMinHoldDuration   = 10 * time.Minute
	protectorDynamicStopEarlyPeakPnLPct   = 8.0
	protectorDynamicStopEarlyPriceMovePct = 0.60
	protectorProfitFloorPeakPnLPct        = 5.0
	protectorProfitFloorBufferPnLPct      = 0.8
	protectorProfitFloorNetBufferPnLPct   = 3.0
	protectorProfitLockMidPeakPnLPct      = 10.0
	protectorProfitLockHighPeakPnLPct     = 15.0
	protectorProfitLockMidRatio           = 0.30
	protectorProfitLockHighRatio          = 0.45
	protectorProfitLockMidDrawdownPct     = 50.0
	protectorProfitLockHighDrawdownPct    = 45.0
	protectorPreTPPeakPnLPct              = 8.0
	protectorPreTPGivebackPct             = 80.0
	protectorNearTP1PeakRatio             = 0.95
	protectorNearTP1SecondChanceRatio     = 0.90
	protectorNearTP1GivebackPct           = 45.0
	protectorNearTP1LossExitPnLPct        = -5.0
	protectorPreTPMinCurrentPnLPct        = 3.0
	protectorPreTPMinHoldDuration         = 20 * time.Minute
	protectorEarlyLossCheckDuration       = 15 * time.Minute
	protectorEarlyLossExitPnLPct          = -8.0
	protectorHardLossExitPnLPct           = -12.0
	protectorBreakevenBufferPct           = 0.001
	protectorDefaultMinCloseNotional      = 12.0
	hunterV7MicroRefreshMaxSpreadPct      = 0.35
	hunterV7MicroRefreshMaxDriftPct       = 0.45
	hunterV7RESTRefreshTimeout            = 3500 * time.Millisecond
	hunterV7RESTRefreshMaxAgeMs           = int64(120_000)
	hunterV7RESTRefreshFlowFlipPct        = 0.50
)

type positionProtectionState struct {
	InitialQuantity   float64
	TP0Done           bool
	TP1Done           bool
	TP2Done           bool
	PeakPnLPct        float64
	ActiveStopLoss    float64
	DynamicStop       float64
	PlannedTakeProfit float64
	OpenedAt          time.Time
	LastActionAt      time.Time
	LastStopUpdateAt  time.Time
}

type protectionAction string

const (
	protectionNone          protectionAction = ""
	protectionTP0           protectionAction = "tp0"
	protectionTP1           protectionAction = "tp1"
	protectionTP2           protectionAction = "tp2"
	protectionTrailClose    protectionAction = "trail_close"
	protectionGivebackClose protectionAction = "giveback_close"
	protectionHardLossClose protectionAction = "hard_loss_close"
)

// startDrawdownMonitor starts drawdown monitoring
func (at *AutoTrader) startDrawdownMonitor() {
	at.monitorWg.Add(1)
	go func() {
		defer at.monitorWg.Done()

		nextInterval := positionProtectorBaseInterval
		logger.Infof("📊 Started adaptive position protector (idle=%s base=%s fast=%s)",
			positionProtectorIdleInterval, positionProtectorBaseInterval, positionProtectorFastInterval)

		for {
			select {
			case <-time.After(nextInterval):
				nextInterval = at.checkPositionDrawdown()
			case <-at.stopMonitorCh:
				logger.Info("⏹ Stopped position drawdown monitoring")
				return
			}
		}
	}()
}

// checkPositionDrawdown checks position drawdown situation
func (at *AutoTrader) checkPositionDrawdown() time.Duration {
	// Get current positions
	positions, err := at.trader.GetPositions()
	if err != nil {
		logger.Infof("❌ Drawdown monitoring: failed to get positions: %v", err)
		return positionProtectorBaseInterval
	}
	if len(positions) == 0 {
		return positionProtectorIdleInterval
	}

	nextInterval := positionProtectorBaseInterval
	plannedRiskByPosition := at.latestOpenDecisionRiskByPosition(50)
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // Short position quantity is negative, convert to positive
		}

		// Guard: skip if entry price is zero (prevents division by zero panic)
		if entryPrice <= 0 {
			logger.Warnf("⚠️ Drawdown monitoring: %s %s has zero entry price, skipping", symbol, side)
			continue
		}

		leverage := 10 // Default value
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		currentPnLPct := calculateLeveragedPnLPct(side, entryPrice, markPrice, leverage)
		currentPriceMovePct := calculateUnleveragedPnLPct(side, entryPrice, markPrice)

		// Construct unique position identifier (distinguish long/short)
		posKey := symbol + "_" + side

		openedAt := at.resolveProtectionPositionOpenedAt(symbol, side)
		at.ensurePeakPnLCacheInitialized(symbol, side, currentPnLPct, openedAt)
		at.UpdatePeakPnL(symbol, side, currentPnLPct)
		state := at.getOrCreateProtectionState(posKey, quantity, currentPnLPct, openedAt)
		if plannedRisk := plannedRiskByPosition[positionRiskKey(symbol, side)]; plannedRisk.stopLoss > 0 || plannedRisk.takeProfit > 0 {
			state.rememberActiveStopLoss(side, plannedRisk.stopLoss)
			state.rememberPlannedTakeProfit(side, plannedRisk.takeProfit)
		}
		if err := at.updateDynamicProtectionStop(symbol, side, quantity, entryPrice, markPrice, leverage, state); err != nil {
			logger.Infof("⚠️ Dynamic protection stop update failed (%s %s): %v", symbol, side, err)
		}
		action, drawdownPct := choosePositionProtectionAction(state, currentPnLPct, currentPriceMovePct)
		if action == protectionNone && shouldTriggerPlannedTP0Price(side, markPrice, currentPnLPct, state) {
			action = protectionTP0
			drawdownPct = protectionDrawdownPct(state, currentPnLPct)
		}
		if shouldUseFastProtectionInterval(state, currentPnLPct) {
			nextInterval = positionProtectorFastInterval
		}
		if action == protectionNone {
			if currentPnLPct > 3.0 {
				logger.Infof("📊 Position protector: %s %s | Profit %.2f%% | Peak %.2f%% | Drawdown %.2f%% | Age %s | TP1=%v TP2=%v",
					symbol, side, currentPnLPct, state.PeakPnLPct, drawdownPct, time.Since(state.OpenedAt).Truncate(time.Second), state.TP1Done, state.TP2Done)
			}
			continue
		}

		closedAll, err := at.executeProtectionAction(symbol, side, quantity, markPrice, entryPrice, action)
		if err != nil {
			logger.Infof("❌ Position protector failed (%s %s %s): %v", symbol, side, action, err)
			continue
		}
		at.markProtectionAction(posKey, action, closedAll)
	}
	return nextInterval
}

func (at *AutoTrader) updateDynamicProtectionStop(symbol, side string, quantity, entryPrice, markPrice float64, leverage int, state *positionProtectionState) error {
	if state == nil || quantity <= 0 || entryPrice <= 0 || markPrice <= 0 {
		return nil
	}
	if !state.LastStopUpdateAt.IsZero() && time.Since(state.LastStopUpdateAt) < positionProtectorBaseInterval {
		return nil
	}
	baseStop := protectionBaseStopFromRisk(side, entryPrice, leverage)
	if baseStop <= 0 {
		return nil
	}
	activeStop := mostProtectiveStop(side, state.ActiveStopLoss, state.DynamicStop)
	if activeStop > 0 && shouldDelayDynamicProtectionStop(state, calculateUnleveragedPnLPct(side, entryPrice, markPrice)) {
		return nil
	}
	maxFavorableDelta := state.PeakPnLPct / 100 / float64(leverage) * entryPrice
	if maxFavorableDelta < 0 {
		maxFavorableDelta = 0
	}
	isLong := side == "long"
	stop := DefaultDynamicStopManager().CalcDynamicStop(entryPrice, baseStop, markPrice, maxFavorableDelta, time.Since(state.OpenedAt), 50, isLong)
	floorStop := protectionProfitFloorStop(side, entryPrice, leverage, state.PeakPnLPct)
	candidateStop := 0.0
	if isStopOnProtectiveSide(side, stop, markPrice) {
		candidateStop = mostProtectiveStop(side, candidateStop, stop)
	}
	if isStopOnProtectiveSide(side, floorStop, markPrice) {
		candidateStop = mostProtectiveStop(side, candidateStop, floorStop)
	}
	if candidateStop <= 0 || !isMoreProtectiveStop(side, candidateStop, activeStop) {
		return nil
	}
	positionSide := "SHORT"
	if isLong {
		positionSide = "LONG"
	}
	if err := at.trader.CancelStopLossOrders(symbol); err != nil {
		return err
	}
	if err := at.trader.SetStopLoss(symbol, positionSide, quantity, candidateStop); err != nil {
		return err
	}
	state.DynamicStop = candidateStop
	state.ActiveStopLoss = candidateStop
	state.LastStopUpdateAt = time.Now()
	logger.Infof("🛡 Dynamic protection stop updated: %s %s | qty %.8f | stop %.8f | previous_stop %.8f | peak %.2f%%", symbol, side, quantity, candidateStop, activeStop, state.PeakPnLPct)
	return nil
}

func shouldDelayDynamicProtectionStop(state *positionProtectionState, currentPriceMovePct float64) bool {
	if state == nil || state.OpenedAt.IsZero() {
		return false
	}
	if time.Since(state.OpenedAt) >= protectorDynamicStopMinHoldDuration {
		return false
	}
	if state.PeakPnLPct >= protectorDynamicStopEarlyPeakPnLPct {
		return false
	}
	return currentPriceMovePct < protectorDynamicStopEarlyPriceMovePct
}

func (state *positionProtectionState) rememberActiveStopLoss(side string, stop float64) {
	if state == nil || stop <= 0 {
		return
	}
	state.ActiveStopLoss = mostProtectiveStop(side, state.ActiveStopLoss, stop)
}

func (state *positionProtectionState) rememberPlannedTakeProfit(side string, takeProfit float64) {
	if state == nil || takeProfit <= 0 {
		return
	}
	if state.PlannedTakeProfit <= 0 {
		state.PlannedTakeProfit = takeProfit
		return
	}
	if side == "long" && takeProfit < state.PlannedTakeProfit {
		state.PlannedTakeProfit = takeProfit
	}
	if side == "short" && takeProfit > state.PlannedTakeProfit {
		state.PlannedTakeProfit = takeProfit
	}
}

func mostProtectiveStop(side string, a, b float64) float64 {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if side == "long" {
		if b > a {
			return b
		}
		return a
	}
	if b < a {
		return b
	}
	return a
}

func protectionBaseStopFromRisk(side string, entryPrice float64, leverage int) float64 {
	if entryPrice <= 0 || leverage <= 0 {
		return 0
	}
	riskPct := 0.35 / float64(leverage)
	if riskPct <= 0 {
		return 0
	}
	if side == "long" {
		return entryPrice * (1 - riskPct)
	}
	return entryPrice * (1 + riskPct)
}

func protectionProfitFloorStop(side string, entryPrice float64, leverage int, peakPnLPct float64) float64 {
	if entryPrice <= 0 || leverage <= 0 || peakPnLPct < protectorProfitFloorPeakPnLPct {
		return 0
	}
	lockPnLPct := math.Max(protectorProfitFloorBufferPnLPct, protectorProfitFloorNetBufferPnLPct)
	switch {
	case peakPnLPct >= protectorProfitLockHighPeakPnLPct:
		lockPnLPct = math.Max(lockPnLPct, peakPnLPct*protectorProfitLockHighRatio)
	case peakPnLPct >= protectorProfitLockMidPeakPnLPct:
		lockPnLPct = math.Max(lockPnLPct, peakPnLPct*protectorProfitLockMidRatio)
	}
	priceMovePct := lockPnLPct / 100 / float64(leverage)
	if side == "long" {
		return entryPrice * (1 + priceMovePct)
	}
	return entryPrice * (1 - priceMovePct)
}

func protectionPositionAge(state *positionProtectionState) time.Duration {
	if state == nil || state.OpenedAt.IsZero() {
		return 0
	}
	return time.Since(state.OpenedAt)
}

func isMoreProtectiveStop(side string, next, prev float64) bool {
	if next <= 0 {
		return false
	}
	if prev <= 0 {
		return true
	}
	minDelta := prev * 0.0002
	if minDelta <= 0 {
		minDelta = 0.00000001
	}
	if side == "long" {
		return next > prev+minDelta
	}
	return next < prev-minDelta
}

func isStopOnProtectiveSide(side string, stop, markPrice float64) bool {
	if stop <= 0 || markPrice <= 0 {
		return false
	}
	if side == "long" {
		return stop < markPrice
	}
	return stop > markPrice
}

func shouldUseFastProtectionInterval(state *positionProtectionState, currentPnLPct float64) bool {
	if state == nil {
		return false
	}
	nearTP1PeakPnLPct := protectorTP1PnLPct * protectorNearTP1PeakRatio
	return state.TP1Done || state.PeakPnLPct >= nearTP1PeakPnLPct || currentPnLPct >= nearTP1PeakPnLPct
}

func shouldTriggerPlannedTP0Price(side string, markPrice, currentPnLPct float64, state *positionProtectionState) bool {
	if state == nil || state.TP0Done || state.PlannedTakeProfit <= 0 || markPrice <= 0 || currentPnLPct <= 0 {
		return false
	}
	if side == "long" {
		return markPrice >= state.PlannedTakeProfit
	}
	if side == "short" {
		return markPrice <= state.PlannedTakeProfit
	}
	return false
}

func protectionDrawdownPct(state *positionProtectionState, currentPnLPct float64) float64 {
	if state == nil || state.PeakPnLPct <= 0 || currentPnLPct >= state.PeakPnLPct {
		return 0
	}
	return (state.PeakPnLPct - currentPnLPct) / state.PeakPnLPct * 100
}

func calculateLeveragedPnLPct(side string, entryPrice, markPrice float64, leverage int) float64 {
	if entryPrice <= 0 || leverage <= 0 {
		return 0
	}
	if side == "long" {
		return ((markPrice - entryPrice) / entryPrice) * float64(leverage) * 100
	}
	return ((entryPrice - markPrice) / entryPrice) * float64(leverage) * 100
}

func calculateUnleveragedPnLPct(side string, entryPrice, markPrice float64) float64 {
	if entryPrice <= 0 {
		return 0
	}
	if side == "long" {
		return ((markPrice - entryPrice) / entryPrice) * 100
	}
	return ((entryPrice - markPrice) / entryPrice) * 100
}

func choosePositionProtectionAction(state *positionProtectionState, currentPnLPct, currentPriceMovePct float64) (protectionAction, float64) {
	if state == nil {
		return protectionNone, 0
	}
	if currentPnLPct > state.PeakPnLPct {
		state.PeakPnLPct = currentPnLPct
	}
	drawdownPct := 0.0
	if state.PeakPnLPct > 0 && currentPnLPct < state.PeakPnLPct {
		drawdownPct = ((state.PeakPnLPct - currentPnLPct) / state.PeakPnLPct) * 100
	}
	age := protectionPositionAge(state)
	if currentPnLPct <= protectorHardLossExitPnLPct ||
		(age <= protectorEarlyLossCheckDuration && currentPnLPct <= protectorEarlyLossExitPnLPct) {
		return protectionHardLossClose, drawdownPct
	}
	if state.PeakPnLPct >= protectorProfitFloorPeakPnLPct && currentPnLPct <= 0 {
		return protectionGivebackClose, drawdownPct
	}
	if !state.TP0Done && state.PeakPnLPct >= protectorTP0PnLPct && currentPnLPct >= protectorTP0PnLPct {
		return protectionTP0, drawdownPct
	}
	if state.PeakPnLPct >= protectorProfitLockHighPeakPnLPct && currentPnLPct > 0 && drawdownPct >= protectorProfitLockHighDrawdownPct {
		return protectionGivebackClose, drawdownPct
	}
	if state.PeakPnLPct >= protectorProfitLockMidPeakPnLPct && currentPnLPct > 0 && drawdownPct >= protectorProfitLockMidDrawdownPct {
		return protectionGivebackClose, drawdownPct
	}
	if !state.TP1Done && currentPnLPct >= protectorTP1PnLPct && currentPriceMovePct >= protectorTP1MinPriceMovePct {
		return protectionTP1, drawdownPct
	}
	if state.TP0Done && !state.TP1Done && state.PeakPnLPct >= protectorProfitLockHighPeakPnLPct && currentPnLPct >= protectorTP0PnLPct {
		return protectionTP1, drawdownPct
	}
	if state.TP1Done && !state.TP2Done && currentPnLPct >= protectorTP2PnLPct && currentPriceMovePct >= protectorTP2MinPriceMovePct {
		return protectionTP2, drawdownPct
	}
	if state.TP1Done && state.PeakPnLPct >= protectorTP1PnLPct && currentPnLPct > 0 && drawdownPct >= protectorTrailDrawdownPct {
		return protectionTrailClose, drawdownPct
	}
	nearTP1PeakPnLPct := protectorTP1PnLPct * protectorNearTP1PeakRatio
	secondChancePnLPct := protectorTP1PnLPct * protectorNearTP1SecondChanceRatio
	if !state.TP1Done &&
		state.PeakPnLPct >= nearTP1PeakPnLPct &&
		currentPriceMovePct >= protectorTP1MinPriceMovePct &&
		currentPnLPct >= secondChancePnLPct &&
		time.Since(state.OpenedAt) >= protectorPreTPMinHoldDuration {
		return protectionGivebackClose, drawdownPct
	}
	if !state.TP1Done &&
		state.PeakPnLPct >= nearTP1PeakPnLPct &&
		time.Since(state.OpenedAt) >= protectorPreTPMinHoldDuration &&
		drawdownPct >= protectorNearTP1GivebackPct &&
		((currentPnLPct >= protectorPreTPMinCurrentPnLPct && currentPriceMovePct >= protectorTP1MinPriceMovePct) || currentPnLPct <= protectorNearTP1LossExitPnLPct) {
		return protectionGivebackClose, drawdownPct
	}
	if !state.TP1Done &&
		state.PeakPnLPct >= protectorPreTPPeakPnLPct &&
		currentPnLPct >= protectorPreTPMinCurrentPnLPct &&
		currentPriceMovePct >= protectorTP1MinPriceMovePct &&
		time.Since(state.OpenedAt) >= protectorPreTPMinHoldDuration &&
		drawdownPct >= protectorPreTPGivebackPct {
		return protectionGivebackClose, drawdownPct
	}
	return protectionNone, drawdownPct
}

func (at *AutoTrader) getOrCreateProtectionState(posKey string, quantity, currentPnLPct float64, openedAt time.Time) *positionProtectionState {
	at.protectionStateMutex.Lock()
	defer at.protectionStateMutex.Unlock()
	if at.protectionState == nil {
		at.protectionState = make(map[string]*positionProtectionState)
	}
	if openedAt.IsZero() {
		openedAt = time.Now()
	}
	state, ok := at.protectionState[posKey]
	if !ok {
		peakPnLPct := currentPnLPct
		at.peakPnLCacheMutex.RLock()
		if cachedPeak := at.peakPnLCache[posKey]; cachedPeak > peakPnLPct {
			peakPnLPct = cachedPeak
		}
		at.peakPnLCacheMutex.RUnlock()
		state = &positionProtectionState{
			InitialQuantity: quantity,
			PeakPnLPct:      peakPnLPct,
			OpenedAt:        openedAt,
		}
		at.protectionState[posKey] = state
		return state
	}
	if quantity > state.InitialQuantity {
		state.InitialQuantity = quantity
	}
	if state.OpenedAt.IsZero() || openedAt.Before(state.OpenedAt) {
		state.OpenedAt = openedAt
	}
	return state
}

func (at *AutoTrader) resolveProtectionPositionOpenedAt(symbol, side string) time.Time {
	posKey := symbol + "_" + side
	if at.store != nil {
		if dbPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, symbol, strings.ToUpper(side)); err == nil && dbPos != nil && dbPos.EntryTime > 0 {
			return time.UnixMilli(dbPos.EntryTime)
		}
	}
	if firstSeen, ok := at.positionFirstSeenTime[posKey]; ok && firstSeen > 0 {
		return time.UnixMilli(firstSeen)
	}
	now := time.Now()
	at.positionFirstSeenTime[posKey] = now.UnixMilli()
	return now
}

func (at *AutoTrader) markProtectionAction(posKey string, action protectionAction, closedAll bool) {
	at.protectionStateMutex.Lock()
	defer at.protectionStateMutex.Unlock()
	state := at.protectionState[posKey]
	if state == nil {
		return
	}
	if closedAll {
		delete(at.protectionState, posKey)
		return
	}
	state.LastActionAt = time.Now()
	switch action {
	case protectionTP0:
		state.TP0Done = true
	case protectionTP1:
		state.TP1Done = true
	case protectionTP2:
		state.TP2Done = true
	case protectionTrailClose, protectionGivebackClose, protectionHardLossClose:
		delete(at.protectionState, posKey)
	}
}

func (at *AutoTrader) executeProtectionAction(symbol, side string, quantity, markPrice, entryPrice float64, action protectionAction) (bool, error) {
	switch action {
	case protectionTP0:
		closeQty, closeAll := at.protectionCloseQuantity(quantity, markPrice, protectorTP0CloseRatio)
		logger.Infof("🟢 TP0 protection triggered: %s %s | close %.8f / %.8f | mark %.8f", symbol, side, closeQty, quantity, markPrice)
		if closeAll {
			return true, at.closeProtectedPosition(symbol, side, 0, string(action))
		}
		if err := at.closeProtectedPosition(symbol, side, closeQty, string(action)); err != nil {
			return false, err
		}
		if err := at.rebuildProtectionStops(symbol, side, quantity-closeQty, entryPrice); err != nil {
			logger.Infof("⚠️ Failed to rebuild stops after TP0 close (%s %s): %v", symbol, side, err)
		}
		return false, nil
	case protectionTP1:
		closeQty, closeAll := at.protectionCloseQuantity(quantity, markPrice, protectorTP1CloseRatio)
		logger.Infof("🟢 TP1 protection triggered: %s %s | close %.8f / %.8f | mark %.8f", symbol, side, closeQty, quantity, markPrice)
		if closeAll {
			return true, at.closeProtectedPosition(symbol, side, 0, string(action))
		}
		if err := at.closeProtectedPosition(symbol, side, closeQty, string(action)); err != nil {
			return false, err
		}
		if err := at.rebuildProtectionStops(symbol, side, quantity-closeQty, entryPrice); err != nil {
			logger.Infof("⚠️ Failed to rebuild stops after TP1 close (%s %s): %v", symbol, side, err)
		}
		return false, nil
	case protectionTP2:
		closeQty, closeAll := at.protectionCloseQuantity(quantity, markPrice, protectorTP2CloseRatio)
		logger.Infof("🟢 TP2 protection triggered: %s %s | close %.8f / %.8f | mark %.8f", symbol, side, closeQty, quantity, markPrice)
		if closeAll {
			return true, at.closeProtectedPosition(symbol, side, 0, string(action))
		}
		if err := at.closeProtectedPosition(symbol, side, closeQty, string(action)); err != nil {
			return false, err
		}
		if err := at.rebuildProtectionStops(symbol, side, quantity-closeQty, entryPrice); err != nil {
			logger.Infof("⚠️ Failed to rebuild stops after TP2 close (%s %s): %v", symbol, side, err)
		}
		return false, nil
	case protectionTrailClose, protectionGivebackClose, protectionHardLossClose:
		logger.Infof("🟢 Trail protection close triggered: %s %s | action=%s | mark %.8f", symbol, side, action, markPrice)
		return true, at.closeProtectedPosition(symbol, side, 0, string(action))
	default:
		return false, nil
	}
}

func (at *AutoTrader) protectionCloseQuantity(quantity, markPrice, ratio float64) (float64, bool) {
	if quantity <= 0 || ratio <= 0 {
		return 0, true
	}
	closeQty := quantity * ratio
	minNotional := at.minProtectionCloseNotional()
	if closeQty*markPrice < minNotional || (quantity-closeQty)*markPrice < minNotional {
		return quantity, true
	}
	return closeQty, false
}

func (at *AutoTrader) minProtectionCloseNotional() float64 {
	if at.config.StrategyConfig != nil && at.config.StrategyConfig.RiskControl.MinPositionSize > 0 {
		return at.config.StrategyConfig.RiskControl.MinPositionSize
	}
	return protectorDefaultMinCloseNotional
}

func (at *AutoTrader) rebuildProtectionStops(symbol, side string, remainingQty, entryPrice float64) error {
	if remainingQty <= 0 {
		return nil
	}
	if err := at.trader.CancelStopOrders(symbol); err != nil {
		return err
	}
	stopPrice := entryPrice
	if side == "long" {
		stopPrice = entryPrice * (1 + protectorBreakevenBufferPct)
		return at.trader.SetStopLoss(symbol, "LONG", remainingQty, stopPrice)
	}
	stopPrice = entryPrice * (1 - protectorBreakevenBufferPct)
	return at.trader.SetStopLoss(symbol, "SHORT", remainingQty, stopPrice)
}

func (at *AutoTrader) closeProtectedPosition(symbol, side string, quantity float64, closeReason string) error {
	switch side {
	case "long":
		order, err := at.trader.CloseLong(symbol, quantity)
		if err != nil {
			return err
		}
		logger.Infof("✅ Protected close long succeeded, order ID: %v, qty: %.8f", order["orderId"], quantity)
	case "short":
		order, err := at.trader.CloseShort(symbol, quantity)
		if err != nil {
			return err
		}
		logger.Infof("✅ Protected close short succeeded, order ID: %v, qty: %.8f", order["orderId"], quantity)
	default:
		return fmt.Errorf("unknown position direction: %s", side)
	}
	at.markPendingProtectedClose(symbol, side, closeReason)
	return nil
}

func (at *AutoTrader) markPendingProtectedClose(symbol, side, closeReason string) {
	if at.store == nil || symbol == "" || closeReason == "" {
		return
	}
	pos, err := at.store.Position().GetOpenPositionBySymbol(at.id, symbol, strings.ToUpper(side))
	if err != nil || pos == nil {
		if err != nil {
			logger.Infof("⚠️ Failed to load position for protected close reason (%s %s): %v", symbol, side, err)
		}
		if err := at.store.Position().RecordPendingCloseIntent(at.id, symbol, side, closeReason, "system_protector"); err != nil {
			logger.Infof("⚠️ Failed to record pending protected close reason (%s %s %s): %v", symbol, side, closeReason, err)
		}
		return
	}
	if err := at.store.Position().MarkPositionCloseIntent(pos.ID, closeReason, "system_protector"); err != nil {
		logger.Infof("⚠️ Failed to mark protected close reason (%s %s %s): %v", symbol, side, closeReason, err)
	}
}

// emergencyClosePosition emergency close position function
func (at *AutoTrader) emergencyClosePosition(symbol, side string) error {
	switch side {
	case "long":
		order, err := at.trader.CloseLong(symbol, 0) // 0 = close all
		if err != nil {
			return err
		}
		logger.Infof("✅ Emergency close long position succeeded, order ID: %v", order["orderId"])
	case "short":
		order, err := at.trader.CloseShort(symbol, 0) // 0 = close all
		if err != nil {
			return err
		}
		logger.Infof("✅ Emergency close short position succeeded, order ID: %v", order["orderId"])
	default:
		return fmt.Errorf("unknown position direction: %s", side)
	}

	return nil
}

// GetPeakPnLCache gets peak profit cache
func (at *AutoTrader) GetPeakPnLCache() map[string]float64 {
	at.peakPnLCacheMutex.RLock()
	defer at.peakPnLCacheMutex.RUnlock()

	// Return a copy of the cache
	cache := make(map[string]float64)
	for k, v := range at.peakPnLCache {
		cache[k] = v
	}
	return cache
}

// UpdatePeakPnL updates peak profit cache
func (at *AutoTrader) UpdatePeakPnL(symbol, side string, currentPnLPct float64) {
	at.peakPnLCacheMutex.Lock()
	defer at.peakPnLCacheMutex.Unlock()

	posKey := symbol + "_" + side
	if peak, exists := at.peakPnLCache[posKey]; exists {
		// Update peak (if long, take larger value; if short, currentPnLPct is negative, also compare)
		if currentPnLPct > peak {
			at.peakPnLCache[posKey] = currentPnLPct
		}
	} else {
		// First time recording
		at.peakPnLCache[posKey] = currentPnLPct
	}
}

func (at *AutoTrader) ensurePeakPnLCacheInitialized(symbol, side string, currentPnLPct float64, openedAt time.Time) float64 {
	posKey := symbol + "_" + side
	at.peakPnLCacheMutex.RLock()
	if peak, exists := at.peakPnLCache[posKey]; exists {
		at.peakPnLCacheMutex.RUnlock()
		return peak
	}
	at.peakPnLCacheMutex.RUnlock()

	peak := currentPnLPct
	if recovered := at.recoverPositionPeakPnLPctFromRecentPrompts(symbol, side, openedAt); recovered > peak {
		peak = recovered
		logger.Infof("📈 Restored position peak PnL from recent decisions: %s %s peak=%.2f%%", symbol, side, peak)
	}

	at.peakPnLCacheMutex.Lock()
	if at.peakPnLCache == nil {
		at.peakPnLCache = make(map[string]float64)
	}
	if existing, exists := at.peakPnLCache[posKey]; exists && existing > peak {
		peak = existing
	} else {
		at.peakPnLCache[posKey] = peak
	}
	at.peakPnLCacheMutex.Unlock()
	return peak
}

func (at *AutoTrader) recoverPositionPeakPnLPctFromRecentPrompts(symbol, side string, openedAt time.Time) float64 {
	if at == nil || at.store == nil || symbol == "" || side == "" {
		return 0
	}
	records, err := at.store.Decision().GetLatestRecords(at.id, 20)
	if err != nil {
		logger.Infof("⚠️ Failed to recover peak PnL from decision history: %v", err)
		return 0
	}
	pattern := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(strings.ToUpper(symbol)) + `\s+` + regexp.QuoteMeta(strings.ToUpper(side)) + `\s+\|[^\n]*Peak PnL\s*([+-]?\d+(?:\.\d+)?)%`)
	peak := 0.0
	for _, record := range records {
		if record == nil || record.InputPrompt == "" {
			continue
		}
		if !openedAt.IsZero() && record.Timestamp.Before(openedAt.Add(-1*time.Minute)) {
			continue
		}
		matches := pattern.FindAllStringSubmatch(record.InputPrompt, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			value, err := strconv.ParseFloat(match[1], 64)
			if err == nil && value > peak {
				peak = value
			}
		}
	}
	return peak
}

// ClearPeakPnLCache clears peak cache for specified position
func (at *AutoTrader) ClearPeakPnLCache(symbol, side string) {
	at.peakPnLCacheMutex.Lock()
	defer at.peakPnLCacheMutex.Unlock()

	posKey := symbol + "_" + side
	delete(at.peakPnLCache, posKey)
	at.protectionStateMutex.Lock()
	defer at.protectionStateMutex.Unlock()
	delete(at.protectionState, posKey)
}

// ============================================================================
// Risk Control Helpers
// ============================================================================

// isBTCETH checks if a symbol is BTC or ETH
func isBTCETH(symbol string) bool {
	symbol = strings.ToUpper(symbol)
	return strings.HasPrefix(symbol, "BTC") || strings.HasPrefix(symbol, "ETH")
}

// enforcePositionValueRatio checks and enforces position value ratio limits (CODE ENFORCED)
// Returns the adjusted position size (capped if necessary) and whether the position was capped
// positionSizeUSD: the original position size in USD
// equity: the account equity
// symbol: the trading symbol
func (at *AutoTrader) enforcePositionValueRatio(positionSizeUSD float64, equity float64, symbol string) (float64, bool) {
	if at.config.StrategyConfig == nil {
		return positionSizeUSD, false
	}

	riskControl := at.config.StrategyConfig.RiskControl

	// Get the appropriate position value ratio limit
	var maxPositionValueRatio float64
	if isBTCETH(symbol) {
		maxPositionValueRatio = riskControl.BTCETHMaxPositionValueRatio
		if maxPositionValueRatio <= 0 {
			maxPositionValueRatio = 5.0 // Default: 5x for BTC/ETH
		}
	} else {
		maxPositionValueRatio = riskControl.AltcoinMaxPositionValueRatio
		if maxPositionValueRatio <= 0 {
			maxPositionValueRatio = 1.0 // Default: 1x for altcoins
		}
	}

	// Calculate max allowed position value = equity × ratio
	maxPositionValue := equity * maxPositionValueRatio

	// Check if position size exceeds limit
	if positionSizeUSD > maxPositionValue {
		logger.Infof("  ⚠️ [RISK CONTROL] Position %.2f USDT exceeds limit (equity %.2f × %.1fx = %.2f USDT max for %s), capping",
			positionSizeUSD, equity, maxPositionValueRatio, maxPositionValue, symbol)
		return maxPositionValue, true
	}

	return positionSizeUSD, false
}

// enforceMinPositionSize checks minimum position size (CODE ENFORCED)
func (at *AutoTrader) enforceMinPositionSize(positionSizeUSD float64) error {
	if at.config.StrategyConfig == nil {
		return nil
	}

	minSize := at.config.StrategyConfig.RiskControl.MinPositionSize
	if minSize <= 0 {
		minSize = 12 // Default: 12 USDT
	}

	if positionSizeUSD < minSize {
		return fmt.Errorf("❌ [RISK CONTROL] Position %.2f USDT below minimum (%.2f USDT)", positionSizeUSD, minSize)
	}
	return nil
}

// enforceMaxPositions checks maximum positions count (CODE ENFORCED)
func (at *AutoTrader) enforceMaxPositions(currentPositionCount int) error {
	if at.config.StrategyConfig == nil {
		return nil
	}

	maxPositions := at.config.StrategyConfig.RiskControl.MaxPositions
	if maxPositions <= 0 {
		maxPositions = 3 // Default: 3 positions
	}

	if currentPositionCount >= maxPositions {
		return fmt.Errorf("❌ [RISK CONTROL] Already at max positions (%d/%d)", currentPositionCount, maxPositions)
	}
	return nil
}

func (at *AutoTrader) isHunterV7Strategy() bool {
	if at == nil || at.config.StrategyConfig == nil {
		return false
	}
	return strings.EqualFold(at.config.StrategyConfig.CoinSource.SourceType, "hunter_v7")
}

func (at *AutoTrader) maxEntryPriceDeviationPct() float64 {
	if at == nil || at.config.StrategyConfig == nil {
		return 0
	}
	if pct := at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct; pct > 0 {
		return pct
	}
	if at.isHunterV7Strategy() {
		return 0.5
	}
	return 0
}

func (at *AutoTrader) maxSingleTradeLossPct() float64 {
	if at == nil || at.config.StrategyConfig == nil {
		return 0
	}
	if pct := at.config.StrategyConfig.RiskControl.MaxSingleTradeLossPct; pct > 0 {
		return pct
	}
	if at.isHunterV7Strategy() {
		return 8.0
	}
	return 0
}

func (at *AutoTrader) maxTakeProfitPriceMovePct() float64 {
	if at == nil || at.config.StrategyConfig == nil {
		return 0
	}
	riskControl := at.config.StrategyConfig.RiskControl
	if pct := at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct; pct > 0 {
		if at.isHunterV7Strategy() {
			return at.ensureHunterV7FeasibleTakeProfitCap(pct)
		}
		return pct
	}
	if at.isHunterV7Strategy() {
		pct := 3.0
		if riskControl.MinRiskRewardRatio > 0 || riskControl.MinStopLossPriceMovePct > 0 || riskControl.MaxEntryPriceDeviationPct > 0 {
			pct = at.ensureHunterV7FeasibleTakeProfitCap(pct)
		}
		return pct
	}
	return 0
}

func (at *AutoTrader) ensureHunterV7FeasibleTakeProfitCap(pct float64) float64 {
	if at == nil || at.config.StrategyConfig == nil || !at.isHunterV7Strategy() {
		return pct
	}
	riskControl := at.config.StrategyConfig.RiskControl
	return kernel.HunterV7EffectiveExecutionGeometry(
		pct,
		riskControl.MinStopLossPriceMovePct,
		riskControl.MaxEntryPriceDeviationPct,
		riskControl.MinRiskRewardRatio,
		true,
	).MaxTPMovePct
}

func (at *AutoTrader) minStopLossPriceMovePct() float64 {
	if at == nil || at.config.StrategyConfig == nil {
		return 0
	}
	if pct := at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct; pct > 0 {
		return pct
	}
	if at.isHunterV7Strategy() {
		return 2.0
	}
	return 0
}

func (at *AutoTrader) capTakeProfitToTP1(decision *kernel.Decision, currentPrice float64, side string) bool {
	maxMovePct := at.maxTakeProfitPriceMovePct()
	if decision == nil || maxMovePct <= 0 || currentPrice <= 0 || decision.TakeProfit <= 0 {
		return false
	}
	maxMove := currentPrice * maxMovePct / 100
	switch side {
	case "long":
		maxTP := currentPrice + maxMove
		if decision.TakeProfit > maxTP {
			logger.Infof("  ⚠️ [RISK CONTROL] %s take_profit %.8f is too far from entry %.8f (max %.2f%%); capping TP1 to %.8f",
				decision.Symbol, decision.TakeProfit, currentPrice, maxMovePct, maxTP)
			decision.TakeProfit = maxTP
			return true
		}
	case "short":
		minTP := currentPrice - maxMove
		if decision.TakeProfit < minTP {
			logger.Infof("  ⚠️ [RISK CONTROL] %s take_profit %.8f is too far from entry %.8f (max %.2f%%); capping TP1 to %.8f",
				decision.Symbol, decision.TakeProfit, currentPrice, maxMovePct, minTP)
			decision.TakeProfit = minTP
			return true
		}
	}
	return false
}

func (at *AutoTrader) enforceSingleTradeLossLimit(decision *kernel.Decision, currentPrice, equity float64, side string) (float64, bool, error) {
	maxLossPct := at.maxSingleTradeLossPct()
	if decision == nil || maxLossPct <= 0 || currentPrice <= 0 || equity <= 0 || decision.PositionSizeUSD <= 0 || decision.StopLoss <= 0 {
		return decision.PositionSizeUSD, false, nil
	}

	var riskDistance float64
	switch side {
	case "long":
		riskDistance = currentPrice - decision.StopLoss
	case "short":
		riskDistance = decision.StopLoss - currentPrice
	default:
		return decision.PositionSizeUSD, false, nil
	}
	if riskDistance <= 0 {
		return decision.PositionSizeUSD, false, nil
	}

	stopLossMovePct := riskDistance / currentPrice
	estimatedLoss := decision.PositionSizeUSD * stopLossMovePct
	maxLossUSD := equity * maxLossPct / 100
	if estimatedLoss <= maxLossUSD {
		return decision.PositionSizeUSD, false, nil
	}

	maxPositionSize := maxLossUSD / stopLossMovePct
	if maxPositionSize <= 0 {
		return decision.PositionSizeUSD, false, fmt.Errorf("❌ [RISK CONTROL] Invalid stop-loss risk for %s", decision.Symbol)
	}
	logger.Infof("  ⚠️ [RISK CONTROL] %s estimated SL loss %.2f USDT exceeds %.2f%% equity cap %.2f USDT; reducing position %.2f → %.2f USDT",
		decision.Symbol, estimatedLoss, maxLossPct, maxLossUSD, decision.PositionSizeUSD, maxPositionSize)
	return maxPositionSize, true, nil
}

func (at *AutoTrader) repairHunterV7OpenDecision(decision *kernel.Decision, currentPrice float64, side string) bool {
	if decision == nil || !at.isHunterV7Strategy() || currentPrice <= 0 {
		return false
	}
	if decision.Action != "open_long" && decision.Action != "open_short" {
		return false
	}

	changed := false
	if at.repairHunterV7DecisionPrice(decision, currentPrice, side) {
		changed = true
	}
	if at.repairHunterV7StopLossDistance(decision, currentPrice, side) {
		changed = true
	}
	if at.repairHunterV7RiskReward(decision, currentPrice, side) {
		changed = true
	}
	if changed {
		logger.Infof("  🛠️ [HUNTER V7 PREFLIGHT] %s repaired for execution: price=%.8f stop_loss=%.8f take_profit=%.8f",
			decision.Symbol, decision.Price, decision.StopLoss, decision.TakeProfit)
	}
	return changed
}

func (at *AutoTrader) repairHunterV7DecisionPrice(decision *kernel.Decision, currentPrice float64, side string) bool {
	if decision == nil || currentPrice <= 0 {
		return false
	}
	if decision.Price <= 0 {
		decision.Price = currentPrice
		logger.Infof("  🛠️ [HUNTER V7 PREFLIGHT] %s missing decision price; using execution price %.8f",
			decision.Symbol, currentPrice)
		return true
	}

	maxDriftPct := at.maxEntryPriceDeviationPct()
	if maxDriftPct <= 0 {
		return false
	}
	deviationPct := math.Abs(currentPrice-decision.Price) / decision.Price * 100
	if deviationPct <= maxDriftPct {
		return false
	}
	favorableDrift := (side == "long" && currentPrice < decision.Price) || (side == "short" && currentPrice > decision.Price)
	if favorableDrift && deviationPct <= 4.0 {
		logger.Infof("  🛠️ [HUNTER V7 PREFLIGHT] %s favorable entry drift %.3f%% > %.3f%%; refreshing decision price %.8f → %.8f and revalidating live geometry",
			decision.Symbol, deviationPct, maxDriftPct, decision.Price, currentPrice)
		decision.Price = currentPrice
		return true
	}

	repairMaxPct := maxDriftPct + 0.10
	if repairMaxPct > 0.80 {
		repairMaxPct = 0.80
	}
	if deviationPct > repairMaxPct {
		return false
	}

	logger.Infof("  🛠️ [HUNTER V7 PREFLIGHT] %s small entry drift %.3f%% > %.3f%% but <= %.3f%% repair band; refreshing decision price %.8f → %.8f",
		decision.Symbol, deviationPct, maxDriftPct, repairMaxPct, decision.Price, currentPrice)
	decision.Price = currentPrice
	return true
}

func (at *AutoTrader) repairHunterV7StopLossDistance(decision *kernel.Decision, currentPrice float64, side string) bool {
	if decision == nil || currentPrice <= 0 || decision.StopLoss <= 0 {
		return false
	}
	if at.isHunterV7Strategy() && decision.Price > 0 {
		maxDriftPct := at.maxEntryPriceDeviationPct()
		if maxDriftPct > 0 {
			deviationPct := math.Abs(currentPrice-decision.Price) / decision.Price * 100
			favorableDrift := (side == "long" && currentPrice < decision.Price) || (side == "short" && currentPrice > decision.Price)
			repairMaxPct := maxDriftPct + 0.10
			if repairMaxPct > 0.80 {
				repairMaxPct = 0.80
			}
			if deviationPct > repairMaxPct && !(favorableDrift && deviationPct <= 4.0) {
				return false
			}
		}
	}
	minStopPct := at.minStopLossPriceMovePct()
	if minStopPct <= 0 {
		return false
	}

	var stopMovePct float64
	switch side {
	case "long":
		stopMovePct = (currentPrice - decision.StopLoss) / currentPrice * 100
	case "short":
		stopMovePct = (decision.StopLoss - currentPrice) / currentPrice * 100
	default:
		return false
	}
	if stopMovePct >= minStopPct {
		if at.isHunterV7Strategy() && stopMovePct > minStopPct+0.35 && stopMovePct <= 3.20 {
			targetStopPct := minStopPct + 0.10
			oldStop := decision.StopLoss
			switch side {
			case "long":
				decision.StopLoss = currentPrice * (1 - targetStopPct/100)
			case "short":
				decision.StopLoss = currentPrice * (1 + targetStopPct/100)
			}
			logger.Infof("  🛠️ [HUNTER V7 PREFLIGHT] %s stop distance %.3f%% is too wide for small-account execution; adjusting stop_loss %.8f → %.8f",
				decision.Symbol, stopMovePct, oldStop, decision.StopLoss)
			return true
		}
		return false
	}
	// If live price has already crossed the proposed stop/invalidation, the
	// setup is broken. Do not invent a new stop and chase the failed signal.
	if stopMovePct <= 0 {
		return false
	}
	// Only repair edge misses. For Hunter v7, the LLM can produce a valid stop
	// at decision price that becomes slightly too tight after allowed entry
	// drift; repair that execution-drift miss while still rejecting broken stops.
	repairFloorPct := minStopPct - 0.15
	if at.isHunterV7Strategy() {
		if maxDriftPct := at.maxEntryPriceDeviationPct(); maxDriftPct > 0 {
			repairFloorPct = math.Min(repairFloorPct, minStopPct-maxDriftPct-0.10)
		}
		if stopMovePct >= 0.75 {
			repairFloorPct = math.Min(repairFloorPct, 0.75)
		}
	}
	if stopMovePct < repairFloorPct {
		return false
	}

	targetStopPct := minStopPct + 0.03
	if at.isHunterV7Strategy() {
		targetStopPct = minStopPct + 0.10
	}
	oldStop := decision.StopLoss
	switch side {
	case "long":
		decision.StopLoss = currentPrice * (1 - targetStopPct/100)
	case "short":
		decision.StopLoss = currentPrice * (1 + targetStopPct/100)
	}
	logger.Infof("  🛠️ [HUNTER V7 PREFLIGHT] %s stop distance edge miss %.3f%% < %.3f%%; adjusting stop_loss %.8f → %.8f",
		decision.Symbol, stopMovePct, minStopPct, oldStop, decision.StopLoss)
	return true
}

func (at *AutoTrader) repairHunterV7RiskReward(decision *kernel.Decision, currentPrice float64, side string) bool {
	if decision == nil || currentPrice <= 0 || decision.StopLoss <= 0 || decision.TakeProfit <= 0 {
		return false
	}
	minRR := 0.0
	if at.config.StrategyConfig != nil {
		minRR = at.config.StrategyConfig.RiskControl.MinRiskRewardRatio
	}
	if minRR <= 0 {
		return false
	}

	riskDistance := 0.0
	rewardDistance := 0.0
	switch side {
	case "long":
		riskDistance = currentPrice - decision.StopLoss
		rewardDistance = decision.TakeProfit - currentPrice
	case "short":
		riskDistance = decision.StopLoss - currentPrice
		rewardDistance = currentPrice - decision.TakeProfit
	default:
		return false
	}
	if riskDistance <= 0 || rewardDistance <= 0 {
		return false
	}
	ratio := rewardDistance / riskDistance
	if ratio >= minRR {
		return false
	}

	requiredReward := riskDistance * (minRR + 0.03)
	maxMovePct := at.maxTakeProfitPriceMovePct()
	maxReward := math.Inf(1)
	if maxMovePct > 0 {
		maxReward = currentPrice * maxMovePct / 100
	}
	if requiredReward > maxReward {
		return false
	}

	oldTP := decision.TakeProfit
	switch side {
	case "long":
		decision.TakeProfit = currentPrice + requiredReward
	case "short":
		decision.TakeProfit = currentPrice - requiredReward
	}
	logger.Infof("  🛠️ [HUNTER V7 PREFLIGHT] %s RR %.2f < %.2f; adjusting take_profit %.8f → %.8f",
		decision.Symbol, ratio, minRR, oldTP, decision.TakeProfit)
	return true
}

func (at *AutoTrader) refreshHunterV7OpenPreflight(ctx *kernel.Context, decision *kernel.Decision, currentPrice, decisionPriceBeforeRepair float64, side string) (float64, bool, error) {
	if decision == nil || !at.isHunterV7Strategy() || (decision.Action != "open_long" && decision.Action != "open_short") {
		return currentPrice, false, nil
	}
	finalPrice := at.resolveOpenExecutionPrice(decision.Symbol, side, currentPrice)
	if finalPrice <= 0 {
		finalPrice = currentPrice
	}
	evidence, err := at.collectHunterV7RefreshEvidence(ctx, decision, side, finalPrice, decisionPriceBeforeRepair)
	if err != nil {
		return finalPrice, false, err
	}
	evidence.applyTo(hunterV7CandidateForDecision(ctx, decision))

	tpWasCapped := at.capTakeProfitToTP1(decision, finalPrice, side)
	at.repairHunterV7OpenDecision(decision, finalPrice, side)
	if err := at.validateOpenDecision(decision, finalPrice, side); err != nil {
		return finalPrice, tpWasCapped, err
	}
	if err := at.validateHunterV7LiveOpenGuard(ctx, decision, side, finalPrice, decisionPriceBeforeRepair); err != nil {
		return finalPrice, tpWasCapped, err
	}
	return finalPrice, tpWasCapped, nil
}

// hunterV7RefreshEvidence is the Stage B output of the guard chain: what the
// live REST/orderbook refresh proved about the candidate. Collecting evidence
// never mutates anything; applyTo is the single Stage C entry that mints the
// fresh_* confirmation codes the later pure checks consume (U5.2).
type hunterV7RefreshEvidence struct {
	RestConfirmed  bool
	MicroConfirmed bool
}

func (ev hunterV7RefreshEvidence) applyTo(candidate *kernel.CandidateCoin) {
	if candidate == nil {
		return
	}
	if ev.RestConfirmed {
		candidate.V7ReasonCodes = appendIfMissingString(candidate.V7ReasonCodes, "fresh_rest_confirmed")
		candidate.V7DataFreshness.PriceAgeMs = 0
	}
	if ev.MicroConfirmed {
		candidate.V7ReasonCodes = appendIfMissingString(candidate.V7ReasonCodes, "fresh_micro_confirmed")
	}
}

// collectHunterV7RefreshEvidence runs the REST and orderbook micro-refresh
// verifications in their historical order and reports what they confirmed.
func (at *AutoTrader) collectHunterV7RefreshEvidence(ctx *kernel.Context, decision *kernel.Decision, side string, livePrice, decisionPriceBeforeRepair float64) (hunterV7RefreshEvidence, error) {
	var evidence hunterV7RefreshEvidence
	restConfirmed, err := at.verifyHunterV7RESTMicroRefresh(ctx, decision, side, livePrice)
	if err != nil {
		return evidence, err
	}
	evidence.RestConfirmed = restConfirmed
	microConfirmed, err := at.verifyHunterV7MicroRefresh(ctx, decision, side, livePrice, decisionPriceBeforeRepair)
	if err != nil {
		return evidence, err
	}
	evidence.MicroConfirmed = microConfirmed
	return evidence, nil
}

func (at *AutoTrader) verifyHunterV7MicroRefresh(ctx *kernel.Context, decision *kernel.Decision, side string, livePrice, decisionPriceBeforeRepair float64) (bool, error) {
	if ctx == nil || decision == nil || !at.isHunterV7Strategy() {
		return false, nil
	}
	candidate := hunterV7CandidateForDecision(ctx, decision)
	if candidate == nil || candidate.V7SetupType == "" {
		return false, nil
	}
	needsFreshBook := hunterV7NeedsFreshMicroRefresh(candidate)
	book, ok := at.hunterV7OrderBookMicroSnapshot(decision.Symbol, side)
	if !ok {
		if needsFreshBook {
			return false, fmt.Errorf("❌ [HUNTER V7 MICRO] %s %s blocked: fresh orderbook micro-refresh unavailable for high-risk signal (fresh_micro_confirmed_missing)",
				candidate.V7SetupType, decision.Symbol)
		}
		return false, nil
	}
	if book.SpreadPct > hunterV7MicroRefreshMaxSpreadPct {
		return false, fmt.Errorf("❌ [HUNTER V7 MICRO] %s %s blocked: orderbook spread %.3f%% exceeds %.3f%% (slippage_risk)",
			candidate.V7SetupType, decision.Symbol, book.SpreadPct, hunterV7MicroRefreshMaxSpreadPct)
	}
	referencePrice := decisionPriceBeforeRepair
	if referencePrice <= 0 {
		referencePrice = decision.Price
	}
	if referencePrice > 0 {
		driftPct := math.Abs(book.ExecutablePrice-referencePrice) / referencePrice * 100
		maxDrift := hunterV7MicroRefreshMaxDriftPct
		if cfgDrift := at.maxEntryPriceDeviationPct(); cfgDrift > 0 && cfgDrift < maxDrift {
			maxDrift = cfgDrift
		}
		if needsFreshBook && driftPct > maxDrift {
			return false, fmt.Errorf("❌ [HUNTER V7 MICRO] %s %s blocked: executable price drift %.3f%% exceeds %.3f%% after micro-refresh (entry_drift)",
				candidate.V7SetupType, decision.Symbol, driftPct, maxDrift)
		}
	}
	if livePrice > 0 {
		bookDriftPct := math.Abs(book.ExecutablePrice-livePrice) / livePrice * 100
		if bookDriftPct > hunterV7MicroRefreshMaxDriftPct {
			return false, fmt.Errorf("❌ [HUNTER V7 MICRO] %s %s blocked: orderbook executable %.8f diverges from live price %.8f by %.3f%% (micro_price_mismatch)",
				candidate.V7SetupType, decision.Symbol, book.ExecutablePrice, livePrice, bookDriftPct)
		}
	}
	return true, nil
}

func (at *AutoTrader) verifyHunterV7RESTMicroRefresh(ctx *kernel.Context, decision *kernel.Decision, side string, livePrice float64) (bool, error) {
	if ctx == nil || decision == nil || !at.isHunterV7Strategy() {
		return false, nil
	}
	candidate := hunterV7CandidateForDecision(ctx, decision)
	if candidate == nil || candidate.V7SetupType == "" || !hunterV7NeedsFreshRESTRefresh(candidate) {
		return false, nil
	}
	se := at.snapshotEngine
	if se == nil && at.strategyEngine != nil {
		se = at.strategyEngine.GetSnapshotEngine()
	}
	if se == nil {
		return false, nil
	}

	refreshCtx, cancel := context.WithTimeout(context.Background(), hunterV7RESTRefreshTimeout)
	defer cancel()
	refreshed, err := se.RefreshSymbolSnapshot(refreshCtx, decision.Symbol, datafetch.FastKlineIntervals, false)
	if err != nil {
		return false, fmt.Errorf("❌ [HUNTER V7 REST] %s %s blocked: symbol REST micro-refresh failed before open: %v",
			candidate.V7SetupType, decision.Symbol, err)
	}
	if err := hunterV7ValidateRESTSnapshot(candidate, decision, side, livePrice, refreshed); err != nil {
		return false, err
	}
	return true, nil
}

func hunterV7NeedsFreshRESTRefresh(candidate *kernel.CandidateCoin) bool {
	if candidate == nil {
		return false
	}
	return hunterV7NeedsFreshMicroRefresh(candidate) ||
		containsStringValue(candidate.V7RiskTags, "event_flow_confirmation_needed") ||
		containsStringValue(candidate.V7RiskTags, "range_expansion_low_volume_followthrough") ||
		containsStringValue(candidate.V7RiskTags, "short_covering_not_new_long_build")
}

func hunterV7ValidateRESTSnapshot(candidate *kernel.CandidateCoin, decision *kernel.Decision, side string, livePrice float64, ss *datafetch.SymbolSnapshot) error {
	if candidate == nil || decision == nil || ss == nil {
		return fmt.Errorf("❌ [HUNTER V7 REST] %s blocked: missing refreshed symbol snapshot (fresh_rest_missing)", decision.Symbol)
	}
	if ageMs, ok := latestKlineAgeMs(ss, "1m"); !ok || ageMs > hunterV7RESTRefreshMaxAgeMs {
		return fmt.Errorf("❌ [HUNTER V7 REST] %s %s blocked: refreshed 1m kline age %dms exceeds %dms (fresh_rest_stale)",
			candidate.V7SetupType, decision.Symbol, ageMs, hunterV7RESTRefreshMaxAgeMs)
	}
	restPrice := ss.Price
	if restPrice <= 0 {
		restPrice = ss.MarkPrice
	}
	if restPrice > 0 && livePrice > 0 {
		driftPct := math.Abs(restPrice-livePrice) / livePrice * 100
		if driftPct > hunterV7MicroRefreshMaxDriftPct {
			return fmt.Errorf("❌ [HUNTER V7 REST] %s %s blocked: REST price %.8f diverges from executable/live price %.8f by %.3f%% (rest_price_mismatch)",
				candidate.V7SetupType, decision.Symbol, restPrice, livePrice, driftPct)
		}
	}
	if ratio, ok := takerBuyRatioFromRecentKlines(ss, "1m", 5); ok {
		if side == "long" && ratio <= 0.38 {
			return fmt.Errorf("❌ [HUNTER V7 REST] %s %s LONG blocked: latest 1m taker_buy_ratio %.3f shows aggressive sell-flow flip (flow_flip)",
				candidate.V7SetupType, decision.Symbol, ratio)
		}
		if side == "short" && ratio >= 0.62 {
			return fmt.Errorf("❌ [HUNTER V7 REST] %s %s SHORT blocked: latest 1m taker_buy_ratio %.3f shows aggressive buy-flow flip (flow_flip)",
				candidate.V7SetupType, decision.Symbol, ratio)
		}
	}
	if movePct, ok := latestKlineMovePct(ss, "1m"); ok {
		if side == "long" && movePct <= -hunterV7RESTRefreshFlowFlipPct {
			return fmt.Errorf("❌ [HUNTER V7 REST] %s %s LONG blocked: latest 1m move %.3f%% reversed against signal (micro_reversal)",
				candidate.V7SetupType, decision.Symbol, movePct)
		}
		if side == "short" && movePct >= hunterV7RESTRefreshFlowFlipPct {
			return fmt.Errorf("❌ [HUNTER V7 REST] %s %s SHORT blocked: latest 1m move %.3f%% reversed against signal (micro_reversal)",
				candidate.V7SetupType, decision.Symbol, movePct)
		}
	}
	return nil
}

func latestKlineAgeMs(ss *datafetch.SymbolSnapshot, interval string) (int64, bool) {
	if ss == nil || ss.Klines == nil {
		return 0, false
	}
	klines := ss.Klines[interval]
	if len(klines) == 0 {
		return 0, false
	}
	latest := klines[len(klines)-1]
	refMs := latest.OpenTime
	if latest.CloseTime > 0 && latest.CloseTime < time.Now().UnixMilli() {
		refMs = latest.CloseTime
	}
	if refMs <= 0 {
		return 0, false
	}
	ageMs := time.Now().UnixMilli() - refMs
	if ageMs < 0 {
		ageMs = 0
	}
	return ageMs, true
}

func takerBuyRatioFromRecentKlines(ss *datafetch.SymbolSnapshot, interval string, limit int) (float64, bool) {
	if ss == nil || ss.Klines == nil || limit <= 0 {
		return 0, false
	}
	klines := ss.Klines[interval]
	if len(klines) == 0 {
		return 0, false
	}
	start := len(klines) - limit
	if start < 0 {
		start = 0
	}
	var totalVol, takerBuy float64
	for _, k := range klines[start:] {
		totalVol += k.Volume
		takerBuy += k.TakerBuy
	}
	if totalVol <= 0 {
		return 0, false
	}
	return takerBuy / totalVol, true
}

func latestKlineMovePct(ss *datafetch.SymbolSnapshot, interval string) (float64, bool) {
	if ss == nil || ss.Klines == nil {
		return 0, false
	}
	klines := ss.Klines[interval]
	if len(klines) == 0 {
		return 0, false
	}
	latest := klines[len(klines)-1]
	if latest.Open <= 0 || latest.Close <= 0 {
		return 0, false
	}
	return (latest.Close - latest.Open) / latest.Open * 100, true
}

type hunterV7OrderBookMicro struct {
	Bid             float64
	Ask             float64
	ExecutablePrice float64
	SpreadPct       float64
}

func (at *AutoTrader) hunterV7OrderBookMicroSnapshot(symbol, side string) (hunterV7OrderBookMicro, bool) {
	gridTrader, ok := at.trader.(GridTrader)
	if !ok || gridTrader == nil {
		return hunterV7OrderBookMicro{}, false
	}
	bids, asks, err := gridTrader.GetOrderBook(symbol, 5)
	if err != nil || len(bids) == 0 || len(asks) == 0 || len(bids[0]) == 0 || len(asks[0]) == 0 {
		if err != nil {
			logger.Infof("  ⚠️ [HUNTER V7 MICRO] Failed to get %s orderbook: %v", symbol, err)
		}
		return hunterV7OrderBookMicro{}, false
	}
	bid := bids[0][0]
	ask := asks[0][0]
	if bid <= 0 || ask <= 0 || ask < bid {
		return hunterV7OrderBookMicro{}, false
	}
	mid := (bid + ask) / 2
	if mid <= 0 {
		return hunterV7OrderBookMicro{}, false
	}
	executable := ask
	if side == "short" {
		executable = bid
	}
	return hunterV7OrderBookMicro{
		Bid:             bid,
		Ask:             ask,
		ExecutablePrice: executable,
		SpreadPct:       (ask - bid) / mid * 100,
	}, true
}

func hunterV7NeedsFreshMicroRefresh(candidate *kernel.CandidateCoin) bool {
	if candidate == nil {
		return false
	}
	return strings.EqualFold(candidate.V7SetupType, "range_expansion_event") ||
		strings.EqualFold(candidate.V7ExecutionTier, "REVIEWABLE") ||
		containsStringValue(candidate.V7RiskTags, "high_volatility") ||
		containsStringValue(candidate.V7RiskTags, "stale_data_risk") ||
		containsStringValue(candidate.V7RiskTags, "range_expansion_exhaustion") ||
		containsStringValue(candidate.V7RiskTags, "velocity_decelerating") ||
		containsStringValue(candidate.V7RiskTags, "micro_reversal_against_signal")
}

func appendIfMissingString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (at *AutoTrader) validateHunterV7LiveOpenGuard(ctx *kernel.Context, decision *kernel.Decision, side string, livePrice, decisionPriceBeforeRepair float64) error {
	if ctx == nil || decision == nil || !at.isHunterV7Strategy() {
		return nil
	}
	candidate := hunterV7CandidateForDecision(ctx, decision)
	if candidate == nil || candidate.V7SetupType == "" {
		return nil
	}
	if err := validateHunterV7DecisionReasoningDirection(decision); err != nil {
		return err
	}
	if err := validateHunterV7WhaleFlowGuard(candidate, decision, livePrice); err != nil {
		return err
	}
	if !strings.EqualFold(candidate.V7SetupType, "range_expansion_event") || side != "short" || decision.Action != "open_short" {
		return nil
	}
	if decisionPriceBeforeRepair > 0 && livePrice > decisionPriceBeforeRepair {
		reboundPct := (livePrice/decisionPriceBeforeRepair - 1) * 100
		if reboundPct >= 0.30 {
			return fmt.Errorf("❌ [HUNTER V7 GUARD] range_expansion_event %s SHORT blocked: live price rebounded %.3f%% above AI decision price after preflight repair (rebound_risk_wait)",
				decision.Symbol, reboundPct)
		}
	}
	if candidate.V7Readiness != nil && candidate.V7Readiness.EntryZonePos > 80 {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] range_expansion_event %s SHORT blocked: entry_zone_position %.1f%% exceeds 80%% late-short limit (rebound_risk_wait)",
			decision.Symbol, candidate.V7Readiness.EntryZonePos)
	}
	if candidate.V7ConfirmSummary != nil && candidate.V7ConfirmSummary.EntryZonePosition > 80 {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] range_expansion_event %s SHORT blocked: confirmation entry_zone_position %.1f%% exceeds 80%% late-short limit (rebound_risk_wait)",
			decision.Symbol, candidate.V7ConfirmSummary.EntryZonePosition)
	}
	if tf, ok := hunterV7CandidateExecutionTimeframe(candidate, "15m"); ok && tf.HasEMA20 && tf.CloseVsEMA20Pct <= -10 {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] range_expansion_event %s SHORT blocked: 15m close %.2f%% below EMA20, late-short exhaustion risk (rebound_risk_wait)",
			decision.Symbol, tf.CloseVsEMA20Pct)
	}
	if taker := hunterV7CandidateTakerBuy15m(candidate); taker >= 0.48 {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] range_expansion_event %s SHORT blocked: taker_buy_15m %.3f no longer confirms short continuation (confirmation_missing)",
			decision.Symbol, taker)
	}
	return nil
}

func hunterV7CandidateExecutionTimeframe(candidate *kernel.CandidateCoin, tf string) (local.V7ExecutionTimeframeSummary, bool) {
	if candidate == nil || candidate.V7ExecutionContext == nil {
		return local.V7ExecutionTimeframeSummary{}, false
	}
	summary, ok := candidate.V7ExecutionContext.Timeframes[tf]
	return summary, ok && summary.CandleCount > 0
}

func validateHunterV7DecisionReasoningDirection(decision *kernel.Decision) error {
	if decision == nil || decision.Action != "open_short" {
		return nil
	}
	reasoning := strings.ToLower(decision.Reasoning)
	if reasoning == "" {
		return nil
	}
	conflictingAbove := strings.Contains(reasoning, "15m close above vwap/ema20") ||
		strings.Contains(reasoning, "15m close above vwap") ||
		strings.Contains(reasoning, "15m close above ema20")
	if conflictingAbove && !strings.Contains(reasoning, "not above") && !strings.Contains(reasoning, "below") {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s SHORT blocked: reasoning treats 15m close above VWAP/EMA20 as short confirmation (direction_confirmation_conflict)",
			decision.Symbol)
	}
	if strings.Contains(reasoning, "15m close above vwap/ema20 true") || strings.Contains(reasoning, "15m close above vwap/ema20 ✓") {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s SHORT blocked: reasoning conflicts with short required confirmation (direction_confirmation_conflict)",
			decision.Symbol)
	}
	return nil
}

func (at *AutoTrader) validateHunterV7ExecutionGuard(ctx *kernel.Context, decision *kernel.Decision) error {
	if ctx == nil || decision == nil || !at.isHunterV7Strategy() {
		return nil
	}
	if decision.Action != "open_short" && decision.Action != "open_long" {
		return nil
	}

	if decision.Action == "open_short" && hunterV7MMSSqueezeShortBanActive(ctx, decision.Symbol) {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s SHORT blocked: MMS squeeze-engine short ban active", decision.Symbol)
	}

	candidate := hunterV7CandidateForDecision(ctx, decision)
	if candidate == nil {
		if strings.TrimSpace(decision.SelectedHunterV7SignalID) != "" {
			return fmt.Errorf("❌ [HUNTER V7 GUARD] %s blocked: selected_hunter_v7_signal_id %q does not match any current candidate (signal_contract_mismatch)",
				decision.Symbol, decision.SelectedHunterV7SignalID)
		}
		return nil
	}
	if candidate.V7SetupType == "" {
		return nil
	}

	// Stage A (pure eligibility): tier gate, tag action gate, signal contract.
	if strings.EqualFold(candidate.V7ExecutionTier, "WATCH") || strings.EqualFold(candidate.V7ExecutionTier, "REJECTED") {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: execution_tier=%s (%s)",
			candidate.V7SetupType, decision.Symbol, candidate.V7ExecutionTier, candidate.V7TierReason)
	}
	for _, tag := range candidate.V7RiskTags {
		action, ok := local.HunterV7TagLLMAction(tag)
		if !ok {
			continue
		}
		if action == local.V7TagActionWaitOnly || action == local.V7TagActionRejectOnly {
			return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: risk tag %s is %s",
				candidate.V7SetupType, decision.Symbol, tag, action)
		}
	}

	if err := validateHunterV7DecisionSignalContract(candidate, decision); err != nil {
		return err
	}
	// Stage B (live evidence, no mutation) + Stage C (explicit apply): the
	// fresh_* codes minted here are what the pure freshness/confirmation
	// checks below consume.
	if hunterV7OpenNeedsGuardRefresh(candidate) {
		side := "long"
		if decision.Action == "open_short" {
			side = "short"
		}
		price := hunterV7DecisionReferencePrice(ctx, candidate, decision)
		evidence, err := at.collectHunterV7RefreshEvidence(ctx, decision, side, price, decision.Price)
		if err != nil {
			return err
		}
		evidence.applyTo(candidate)
	}
	// Stage A again (pure, evidence-aware): freshness, required confirmations
	// and tag combos re-evaluated against the applied evidence.
	if err := validateHunterV7SignalFreshness(candidate, decision); err != nil {
		return err
	}
	if err := validateHunterV7RequiredConfirmations(candidate, decision); err != nil {
		return err
	}
	if err := validateHunterV7RiskTagCombos(candidate, decision); err != nil {
		return err
	}

	// Stage C (explicit position mutation).
	at.capHunterV7LowLiquidityPosition(decision, candidate)

	// Look up per-setup guard thresholds from strategy config
	guard := at.setupGuardForSetup(candidate.V7SetupType)
	price := hunterV7DecisionReferencePrice(ctx, candidate, decision)
	if price > 0 {
		if err := validateHunterV7WhaleFlowGuard(candidate, decision, price); err != nil {
			return err
		}
	}

	if guard == nil {
		return nil // no guard configured for this setup
	}

	// OI flush check (for setups that require it, e.g. funding_reversal)
	if guard.RequireOIFlush {
		oiState := hunterV7FundingReversalOIState(candidate)
		if oiState == "building" {
			if decision.Action == "open_short" {
				return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s SHORT blocked: OI is still building; wait for OI flush or failed rebuild", candidate.V7SetupType, decision.Symbol)
			}
			if decision.Action == "open_long" {
				return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s LONG blocked: OI is still building; wait for OI reset or failed rebuild", candidate.V7SetupType, decision.Symbol)
			}
		}
	}

	// Zone position checks
	if pos, ok := hunterV7EntryZonePositionPct(candidate, price); ok {
		if decision.Action == "open_short" && guard.MinZonePosShort > 0 && int(pos) < guard.MinZonePosShort {
			return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s SHORT blocked: price not near entry-zone upper/retest area (zone_pos %.1f%%, min %d%%)",
				candidate.V7SetupType, decision.Symbol, pos, guard.MinZonePosShort)
		}
		if decision.Action == "open_long" && guard.MaxZonePosLong < 100 && int(pos) > guard.MaxZonePosLong {
			return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s LONG blocked: price not near entry-zone lower/reclaim area (zone_pos %.1f%%, max %d%%)",
				candidate.V7SetupType, decision.Symbol, pos, guard.MaxZonePosLong)
		}
	}

	return nil
}

func hunterV7OpenNeedsGuardRefresh(candidate *kernel.CandidateCoin) bool {
	if candidate == nil {
		return false
	}
	if containsStringValue(candidate.V7RiskTags, "stale_data_risk") && !containsStringValue(candidate.V7ReasonCodes, "fresh_micro_confirmed") {
		return true
	}
	if strings.EqualFold(candidate.V7ExecutionTier, "REVIEWABLE") &&
		hunterV7HasOnlyLiveReviewableConfirmationGaps(candidate) &&
		(!containsStringValue(candidate.V7ReasonCodes, "fresh_micro_confirmed") ||
			!containsStringValue(candidate.V7ReasonCodes, "fresh_rest_confirmed")) {
		return true
	}
	if strings.EqualFold(candidate.V7ExecutionTier, "REVIEWABLE") &&
		candidate.V7ConfirmSummary == nil &&
		hunterV7RequiredConfirmationsCanBeSatisfiedByRefresh(candidate) &&
		(!containsStringValue(candidate.V7ReasonCodes, "fresh_micro_confirmed") ||
			!containsStringValue(candidate.V7ReasonCodes, "fresh_rest_confirmed")) {
		return true
	}
	return false
}

func validateHunterV7RequiredConfirmations(candidate *kernel.CandidateCoin, decision *kernel.Decision) error {
	if candidate == nil || decision == nil || len(candidate.V7RequiredConfirms) == 0 {
		return nil
	}
	summary := candidate.V7ConfirmSummary
	if summary == nil {
		if hunterV7RequiredConfirmationsSatisfiedByRefresh(candidate) {
			return nil
		}
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: required confirmations cannot be machine-verified (confirmation_missing)",
			candidate.V7SetupType, decision.Symbol)
	}
	required := map[string]struct{}{}
	for _, code := range candidate.V7RequiredConfirms {
		if code != "" {
			required[code] = struct{}{}
		}
	}
	if len(required) == 0 {
		return nil
	}
	for _, check := range summary.MissingHard {
		if _, ok := required[check.Code]; ok {
			return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: required confirmation %s failed (confirmation_missing)",
				candidate.V7SetupType, decision.Symbol, check.Code)
		}
	}
	for _, check := range summary.MissingReview {
		if _, ok := required[check.Code]; ok {
			if hunterV7ReviewableConfirmationSatisfiedByRefresh(candidate, check.Code) {
				continue
			}
			return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: required confirmation %s failed (confirmation_missing)",
				candidate.V7SetupType, decision.Symbol, check.Code)
		}
	}
	for _, check := range summary.ContextChecks {
		if _, ok := required[check.Code]; ok {
			if hunterV7ReviewableConfirmationSatisfiedByRefresh(candidate, check.Code) {
				continue
			}
			return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: required confirmation %s is context-only/unverified (confirmation_missing)",
				candidate.V7SetupType, decision.Symbol, check.Code)
		}
	}
	return nil
}

func hunterV7RequiredConfirmationsSatisfiedByRefresh(candidate *kernel.CandidateCoin) bool {
	if candidate == nil || !strings.EqualFold(candidate.V7ExecutionTier, "REVIEWABLE") || len(candidate.V7RequiredConfirms) == 0 {
		return false
	}
	if !containsStringValue(candidate.V7ReasonCodes, "fresh_micro_confirmed") ||
		!containsStringValue(candidate.V7ReasonCodes, "fresh_rest_confirmed") {
		return false
	}
	return hunterV7RequiredConfirmationsCanBeSatisfiedByRefresh(candidate)
}

func hunterV7RequiredConfirmationsCanBeSatisfiedByRefresh(candidate *kernel.CandidateCoin) bool {
	if candidate == nil || len(candidate.V7RequiredConfirms) == 0 {
		return false
	}
	for _, code := range candidate.V7RequiredConfirms {
		if code == "" {
			continue
		}
		if !hunterV7ConfirmationCanBeSatisfiedByRefresh(code) {
			return false
		}
	}
	return true
}

func hunterV7HasOnlyLiveReviewableConfirmationGaps(candidate *kernel.CandidateCoin) bool {
	if candidate == nil || candidate.V7ConfirmSummary == nil {
		return false
	}
	if len(candidate.V7ConfirmSummary.MissingHard) > 0 {
		return false
	}
	found := false
	for _, check := range candidate.V7ConfirmSummary.MissingReview {
		if !hunterV7ConfirmationCanBeSatisfiedByRefresh(check.Code) {
			return false
		}
		found = true
	}
	for _, check := range candidate.V7ConfirmSummary.ContextChecks {
		if !check.Passed {
			if !hunterV7ConfirmationCanBeSatisfiedByRefresh(check.Code) {
				return false
			}
			found = true
		}
	}
	return found
}

func hunterV7ReviewableConfirmationSatisfiedByRefresh(candidate *kernel.CandidateCoin, code string) bool {
	if candidate == nil || !strings.EqualFold(candidate.V7ExecutionTier, "REVIEWABLE") {
		return false
	}
	if !hunterV7ConfirmationCanBeSatisfiedByRefresh(code) {
		return false
	}
	return containsStringValue(candidate.V7ReasonCodes, "fresh_micro_confirmed") &&
		containsStringValue(candidate.V7ReasonCodes, "fresh_rest_confirmed")
}

func hunterV7ConfirmationCanBeSatisfiedByRefresh(code string) bool {
	return local.V7ConfirmRefreshSatisfiable(code)
}

func validateHunterV7DecisionSignalContract(candidate *kernel.CandidateCoin, decision *kernel.Decision) error {
	if candidate == nil || decision == nil {
		return nil
	}
	selectedID := strings.TrimSpace(decision.SelectedHunterV7SignalID)
	if candidate.V7SignalID != "" && selectedID == "" {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s blocked: selected_hunter_v7_signal_id is required for Hunter v7 candidate %q (signal_contract_missing)",
			decision.Symbol, candidate.V7SignalID)
	}
	if selectedID != "" && candidate.V7SignalID != "" && selectedID != candidate.V7SignalID {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s blocked: selected_hunter_v7_signal_id %q does not match backend candidate %q (signal_contract_mismatch)",
			decision.Symbol, selectedID, candidate.V7SignalID)
	}
	wantDirection := ""
	if decision.Action == "open_long" {
		wantDirection = "LONG"
	} else if decision.Action == "open_short" {
		wantDirection = "SHORT"
	}
	if wantDirection != "" && candidate.Direction != "" && !strings.EqualFold(candidate.Direction, wantDirection) {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s blocked: decision direction %s conflicts with Hunter v7 signal direction %s (signal_contract_mismatch)",
			decision.Symbol, wantDirection, candidate.Direction)
	}
	if selectedSetup := strings.TrimSpace(decision.SelectedHunterV7Setup); selectedSetup != "" && candidate.V7SetupType != "" && !strings.EqualFold(selectedSetup, candidate.V7SetupType) {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s blocked: selected setup %q conflicts with Hunter v7 setup %q (signal_contract_mismatch)",
			decision.Symbol, selectedSetup, candidate.V7SetupType)
	}
	if selectedTier := strings.TrimSpace(decision.SelectedHunterV7Tier); selectedTier != "" && candidate.V7ExecutionTier != "" && !strings.EqualFold(selectedTier, candidate.V7ExecutionTier) {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s blocked: selected tier %q conflicts with Hunter v7 tier %q (signal_contract_mismatch)",
			decision.Symbol, selectedTier, candidate.V7ExecutionTier)
	}
	return nil
}

func validateHunterV7SignalFreshness(candidate *kernel.CandidateCoin, decision *kernel.Decision) error {
	if candidate == nil || decision == nil {
		return nil
	}
	ageMs := decision.SignalAgeMs
	if ageMs <= 0 {
		if candidate.V7DataFreshness.PriceAgeMs > 0 {
			ageMs = candidate.V7DataFreshness.PriceAgeMs
		} else {
			ageMs = candidate.V7DataFreshness.SnapshotAgeMs
		}
	}
	if ageMs <= 0 {
		return nil
	}
	highVelocity := containsStringValue(candidate.V7RiskTags, "high_volatility") ||
		containsStringValue(candidate.V7RiskTags, "stale_data_risk") ||
		strings.EqualFold(candidate.V7SetupType, "range_expansion_event")
	if highVelocity && ageMs > 45_000 && !containsStringValue(candidate.V7ReasonCodes, "fresh_micro_confirmed") {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: high-volatility signal age %dms exceeds 45000ms without fresh_micro_confirmed (stale_data_risk)",
			candidate.V7SetupType, decision.Symbol, ageMs)
	}
	return nil
}

func validateHunterV7RiskTagCombos(candidate *kernel.CandidateCoin, decision *kernel.Decision) error {
	if candidate == nil || decision == nil {
		return nil
	}
	tags := candidate.V7RiskTags
	if containsStringValue(tags, "high_volatility") && containsStringValue(tags, "execution_stop_tightened") {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: high_volatility + execution_stop_tightened requires wait-only confirmation (wait_only_risk_combo)",
			candidate.V7SetupType, decision.Symbol)
	}
	if containsStringValue(tags, "high_volatility") && strings.EqualFold(candidate.V7Confidence, "C") {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: high_volatility + confidence=C requires wait-only confirmation (wait_only_risk_combo)",
			candidate.V7SetupType, decision.Symbol)
	}
	if containsStringValue(tags, "stale_data_risk") && !containsStringValue(candidate.V7ReasonCodes, "fresh_micro_confirmed") {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] %s %s blocked: stale_data_risk requires fresh_micro_confirmed before open (stale_data_risk)",
			candidate.V7SetupType, decision.Symbol)
	}
	applyHunterV7RiskSizingPolicy(candidate, decision)
	return nil
}

func applyHunterV7RiskSizingPolicy(candidate *kernel.CandidateCoin, decision *kernel.Decision) {
	if candidate == nil || decision == nil {
		return
	}
	tags := candidate.V7RiskTags
	sizeMultiplier := 1.0
	leverageCap := 0
	reasons := make([]string, 0, 3)

	apply := func(multiplier float64, cap int, reason string) {
		if multiplier > 0 && multiplier < sizeMultiplier {
			sizeMultiplier = multiplier
		}
		if cap > 0 && (leverageCap == 0 || cap < leverageCap) {
			leverageCap = cap
		}
		if reason != "" {
			reasons = append(reasons, reason)
		}
	}

	if containsStringValue(tags, "high_volatility") {
		apply(0.60, 15, "high_volatility")
	}
	if containsStringValue(tags, "moderate_liquidity") && containsStringValue(tags, "high_volatility") {
		apply(1.0/3.0, 10, "high_volatility+moderate_liquidity")
	}
	if containsStringValue(tags, "execution_stop_tightened") {
		apply(0.50, 10, "execution_stop_tightened")
	}
	if containsStringValue(tags, "taker_buy_borderline") {
		apply(0.60, 0, "taker_buy_borderline")
	}
	if containsStringValue(tags, "range_expansion_exhaustion") || containsStringValue(tags, "velocity_decelerating") {
		apply(0.50, 10, "event_exhaustion_or_deceleration")
	}
	if strings.EqualFold(candidate.V7ExecutionTier, "REVIEWABLE") {
		apply(0.50, 10, "reviewable_tier")
	}

	if sizeMultiplier < 1 && decision.PositionSizeUSD > 0 {
		oldSize := decision.PositionSizeUSD
		decision.PositionSizeUSD = oldSize * sizeMultiplier
		logger.Infof("  ⚠️ [HUNTER V7 GUARD] %s %s risk sizing %.2fx (%s): %.2f → %.2f",
			candidate.V7SetupType, decision.Symbol, sizeMultiplier, strings.Join(reasons, ","), oldSize, decision.PositionSizeUSD)
	}
	if leverageCap > 0 && decision.Leverage > leverageCap {
		oldLev := decision.Leverage
		decision.Leverage = leverageCap
		logger.Infof("  ⚠️ [HUNTER V7 GUARD] %s %s leverage capped %dx → %dx (%s)",
			candidate.V7SetupType, decision.Symbol, oldLev, decision.Leverage, strings.Join(reasons, ","))
	}
}

func validateHunterV7WhaleFlowGuard(candidate *kernel.CandidateCoin, decision *kernel.Decision, price float64) error {
	if candidate == nil || decision == nil || candidate.V7SetupType != "whale_flow_reversal" || decision.Action != "open_long" {
		return nil
	}
	if pos, ok := hunterV7EntryZonePositionPct(candidate, price); ok && pos > 45 {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] whale_flow_reversal %s LONG blocked: entry zone position %.1f%% exceeds 45%% confirmation limit",
			decision.Symbol, pos)
	}
	if taker := hunterV7CandidateTakerBuy15m(candidate); taker > 0 && taker < 0.56 {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] whale_flow_reversal %s LONG blocked: taker_buy_15m %.3f below 0.560",
			decision.Symbol, taker)
	}
	mid := hunterV7EntryZoneMidPrice(candidate)
	if mid > 0 && price < mid {
		return fmt.Errorf("❌ [HUNTER V7 GUARD] whale_flow_reversal %s LONG blocked: price %.8f below entry-zone mid %.8f",
			decision.Symbol, price, mid)
	}
	return nil
}

func hunterV7CandidateTakerBuy15m(candidate *kernel.CandidateCoin) float64 {
	if candidate == nil {
		return 0
	}
	if candidate.V7DerivativesCtx != nil && candidate.V7DerivativesCtx.TakerBuy15m > 0 {
		return candidate.V7DerivativesCtx.TakerBuy15m
	}
	return 0
}

func hunterV7EntryZoneMidPrice(candidate *kernel.CandidateCoin) float64 {
	if candidate == nil || candidate.V7EntryZone.Lower <= 0 || candidate.V7EntryZone.Upper <= candidate.V7EntryZone.Lower {
		return 0
	}
	return (candidate.V7EntryZone.Lower + candidate.V7EntryZone.Upper) / 2
}

func containsStringValue(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func hunterV7MMSSqueezeShortBanActive(ctx *kernel.Context, symbol string) bool {
	if ctx == nil || symbol == "" {
		return false
	}
	for i := range ctx.CandidateCoins {
		candidate := &ctx.CandidateCoins[i]
		if !strings.EqualFold(candidate.Symbol, symbol) {
			continue
		}
		if hunterV7StringSliceContains(candidate.V7RiskTags, "mms_do_not_short_squeeze") ||
			hunterV7StringSliceContains(candidate.V7ReasonCodes, "mms_short_ban_active") {
			return true
		}
	}
	return false
}

func hunterV7StringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hunterV7ConfirmationCheckCodes(checks []local.V7ConfirmationCheck) string {
	if len(checks) == 0 {
		return "missing_confirmation_detail"
	}
	codes := make([]string, 0, len(checks))
	for _, check := range checks {
		code := strings.TrimSpace(check.Code)
		if code == "" {
			continue
		}
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return "missing_confirmation_detail"
	}
	return strings.Join(codes, ",")
}

func (at *AutoTrader) capHunterV7LowLiquidityPosition(decision *kernel.Decision, candidate *kernel.CandidateCoin) {
	if decision == nil || candidate == nil || decision.PositionSizeUSD <= 0 || candidate.V7QuoteVolume24h <= 0 {
		return
	}
	capUSD := hunterV7LiquidityPositionCapUSD(candidate.V7QuoteVolume24h)
	if capUSD <= 0 || decision.PositionSizeUSD <= capUSD {
		return
	}
	oldSize := decision.PositionSizeUSD
	decision.PositionSizeUSD = capUSD
	logger.Infof("  🧯 [HUNTER V7 LIQUIDITY CAP] %s quote_volume_24h=%.0f caps position %.2f → %.2f USDT",
		decision.Symbol, candidate.V7QuoteVolume24h, oldSize, decision.PositionSizeUSD)
}

func hunterV7LiquidityPositionCapUSD(quoteVolume24h float64) float64 {
	switch {
	case quoteVolume24h <= 0:
		return 0
	case quoteVolume24h < 5_000_000:
		return quoteVolume24h * 0.000005
	case quoteVolume24h < 10_000_000:
		return quoteVolume24h * 0.000010
	case quoteVolume24h < 25_000_000:
		return quoteVolume24h * 0.000020
	case quoteVolume24h < 50_000_000:
		return quoteVolume24h * 0.000040
	default:
		return 0
	}
}

// setupGuardForSetup returns the guard config for a given setup type.
// Returns nil if no guard is configured (setup is unconstrained).
func (at *AutoTrader) setupGuardForSetup(setupType string) *setupGuardDefaults {
	// First check strategy config overrides
	if at.config.StrategyConfig != nil && at.config.StrategyConfig.RiskControl.SetupGuard != nil {
		if sg, ok := at.config.StrategyConfig.RiskControl.SetupGuard[setupType]; ok {
			return &setupGuardDefaults{
				MinZonePosShort: sg.MinZonePosShort,
				MaxZonePosLong:  sg.MaxZonePosLong,
				RequireOIFlush:  sg.RequireOIFlush,
			}
		}
	}
	// Built-in defaults for well-known setups
	return builtinSetupGuardDefaults(setupType)
}

type setupGuardDefaults struct {
	MinZonePosShort int
	MaxZonePosLong  int
	RequireOIFlush  bool
}

// builtinSetupGuardDefaults returns hardcoded guard defaults for known setup types.
// Setups not listed here return nil (no guard).
func builtinSetupGuardDefaults(setupType string) *setupGuardDefaults {
	switch setupType {
	case "funding_reversal":
		return &setupGuardDefaults{MinZonePosShort: 0, MaxZonePosLong: 35, RequireOIFlush: true}
	case "distribution_short":
		return &setupGuardDefaults{MinZonePosShort: 60, MaxZonePosLong: 100, RequireOIFlush: false}
	case "long_squeeze_short":
		return &setupGuardDefaults{MinZonePosShort: 60, MaxZonePosLong: 100, RequireOIFlush: false}
	case "breakdown_momentum_short":
		return &setupGuardDefaults{MinZonePosShort: 45, MaxZonePosLong: 100, RequireOIFlush: false}
	case "alt_ladder_breakdown_short":
		return &setupGuardDefaults{MinZonePosShort: 40, MaxZonePosLong: 100, RequireOIFlush: false}
	case "range_reversion":
		return &setupGuardDefaults{MinZonePosShort: 55, MaxZonePosLong: 45, RequireOIFlush: false}
	case "pullback_reversal_long":
		return &setupGuardDefaults{MinZonePosShort: 0, MaxZonePosLong: 50, RequireOIFlush: false}
	case "alt_ladder_momentum_long":
		return &setupGuardDefaults{MinZonePosShort: 0, MaxZonePosLong: 72, RequireOIFlush: false}
	case "mms_bottom_wake_long":
		return &setupGuardDefaults{MinZonePosShort: 0, MaxZonePosLong: 65, RequireOIFlush: false}
	case "mms_trend_ride_long":
		return &setupGuardDefaults{MinZonePosShort: 0, MaxZonePosLong: 70, RequireOIFlush: false}
	case "mms_squeeze_engine_long":
		return &setupGuardDefaults{MinZonePosShort: 0, MaxZonePosLong: 75, RequireOIFlush: false}
	default:
		return nil
	}
}

func hunterV7CandidateForDecision(ctx *kernel.Context, decision *kernel.Decision) *kernel.CandidateCoin {
	if ctx == nil || decision == nil {
		return nil
	}
	wantDirection := ""
	if decision.Action == "open_short" {
		wantDirection = "SHORT"
	} else if decision.Action == "open_long" {
		wantDirection = "LONG"
	}
	if selectedID := strings.TrimSpace(decision.SelectedHunterV7SignalID); selectedID != "" {
		for i := range ctx.CandidateCoins {
			candidate := &ctx.CandidateCoins[i]
			if candidate.V7SignalID == selectedID {
				return candidate
			}
		}
		return nil
	}
	for i := range ctx.CandidateCoins {
		candidate := &ctx.CandidateCoins[i]
		if !strings.EqualFold(candidate.Symbol, decision.Symbol) {
			continue
		}
		if wantDirection != "" && candidate.Direction != "" && !strings.EqualFold(candidate.Direction, wantDirection) {
			continue
		}
		return candidate
	}
	return nil
}

func hunterV7DecisionReferencePrice(ctx *kernel.Context, candidate *kernel.CandidateCoin, decision *kernel.Decision) float64 {
	if decision != nil && decision.Price > 0 {
		return decision.Price
	}
	if ctx != nil && decision != nil {
		if data := ctx.MarketDataMap[decision.Symbol]; data != nil && data.CurrentPrice > 0 {
			return data.CurrentPrice
		}
	}
	if candidate != nil && candidate.V7PriceContext != nil && candidate.V7PriceContext.Last > 0 {
		return candidate.V7PriceContext.Last
	}
	return 0
}

func hunterV7FundingReversalOIState(candidate *kernel.CandidateCoin) string {
	if candidate == nil || candidate.V7DerivativesCtx == nil {
		return ""
	}
	oi1h := candidate.V7DerivativesCtx.OIChange1h
	oi4h := candidate.V7DerivativesCtx.OIChange4h
	if oi1h < -0.2 && oi4h <= 0 {
		return "flush"
	}
	if oi1h <= 0 && oi4h < -0.5 {
		return "failed_rebuild_or_declining"
	}
	if oi1h > 0 && oi4h < 0 {
		return "mixed"
	}
	if oi1h > 0 && oi4h >= 0 {
		return "building"
	}
	return "neutral"
}

func hunterV7EntryZonePositionPct(candidate *kernel.CandidateCoin, price float64) (float64, bool) {
	if candidate == nil {
		return 0, false
	}
	return local.V7ZonePositionPct(candidate.V7EntryZone, price)
}

// validateOpenDecision enforces non-negotiable safety checks before any open order.
func (at *AutoTrader) validateOpenDecision(decision *kernel.Decision, currentPrice float64, side string) error {
	if decision == nil {
		return fmt.Errorf("❌ [RISK CONTROL] Decision is nil")
	}
	if currentPrice <= 0 {
		return fmt.Errorf("❌ [RISK CONTROL] Invalid current price %.8f for %s", currentPrice, decision.Symbol)
	}
	if decision.PositionSizeUSD <= 0 {
		return fmt.Errorf("❌ [RISK CONTROL] Position size must be positive for %s", decision.Symbol)
	}
	if decision.Leverage <= 0 {
		return fmt.Errorf("❌ [RISK CONTROL] Leverage must be positive for %s", decision.Symbol)
	}

	maxPriceDeviationPct := at.maxEntryPriceDeviationPct()
	if maxPriceDeviationPct > 0 {
		if decision.Price <= 0 {
			if at.isHunterV7Strategy() {
				return fmt.Errorf("❌ [RISK CONTROL] Decision price is required for Hunter v7 open order on %s", decision.Symbol)
			}
		} else {
			deviationPct := math.Abs(currentPrice-decision.Price) / decision.Price * 100
			if deviationPct > maxPriceDeviationPct {
				return fmt.Errorf("❌ [RISK CONTROL] Entry price drift %.3f%% exceeds max %.3f%% for %s (decision %.8f, execution %.8f)",
					deviationPct, maxPriceDeviationPct, decision.Symbol, decision.Price, currentPrice)
			}
		}
	}

	var minRiskRewardRatio float64
	if at.config.StrategyConfig != nil {
		riskControl := at.config.StrategyConfig.RiskControl

		minConfidence := at.effectiveMinOpenConfidence(riskControl.MinConfidence)
		if minConfidence > 0 && decision.Confidence < minConfidence {
			return fmt.Errorf("❌ [RISK CONTROL] Confidence %d below minimum %d for %s",
				decision.Confidence, minConfidence, decision.Symbol)
		}

		maxLeverage := riskControl.AltcoinMaxLeverage
		if isBTCETH(decision.Symbol) {
			maxLeverage = riskControl.BTCETHMaxLeverage
		}
		if maxLeverage > 0 && decision.Leverage > maxLeverage {
			return fmt.Errorf("❌ [RISK CONTROL] Leverage %dx exceeds max %dx for %s",
				decision.Leverage, maxLeverage, decision.Symbol)
		}

		minRiskRewardRatio = riskControl.MinRiskRewardRatio
	}

	if decision.StopLoss <= 0 || decision.TakeProfit <= 0 {
		return fmt.Errorf("❌ [RISK CONTROL] Stop loss and take profit are required for %s", decision.Symbol)
	}

	var riskDistance, rewardDistance float64
	switch side {
	case "long":
		if !(decision.StopLoss < currentPrice && currentPrice < decision.TakeProfit) {
			return fmt.Errorf("❌ [RISK CONTROL] Invalid LONG SL/TP for %s: stop_loss %.8f < current %.8f < take_profit %.8f required",
				decision.Symbol, decision.StopLoss, currentPrice, decision.TakeProfit)
		}
		riskDistance = currentPrice - decision.StopLoss
		rewardDistance = decision.TakeProfit - currentPrice
	case "short":
		if !(decision.TakeProfit < currentPrice && currentPrice < decision.StopLoss) {
			return fmt.Errorf("❌ [RISK CONTROL] Invalid SHORT SL/TP for %s: take_profit %.8f < current %.8f < stop_loss %.8f required",
				decision.Symbol, decision.TakeProfit, currentPrice, decision.StopLoss)
		}
		riskDistance = decision.StopLoss - currentPrice
		rewardDistance = currentPrice - decision.TakeProfit
	default:
		return fmt.Errorf("❌ [RISK CONTROL] Unknown open side %s for %s", side, decision.Symbol)
	}

	if riskDistance <= 0 || rewardDistance <= 0 {
		return fmt.Errorf("❌ [RISK CONTROL] Invalid risk/reward distance for %s", decision.Symbol)
	}

	if minStopLossMovePct := at.minStopLossPriceMovePct(); minStopLossMovePct > 0 {
		stopLossMovePct := riskDistance / currentPrice * 100
		if stopLossMovePct < minStopLossMovePct {
			return fmt.Errorf("❌ [RISK CONTROL] Stop-loss distance %.2f%% below minimum %.2f%% for %s",
				stopLossMovePct, minStopLossMovePct, decision.Symbol)
		}
	}

	if minRiskRewardRatio > 0 {
		ratio := rewardDistance / riskDistance
		if ratio < minRiskRewardRatio {
			return fmt.Errorf("❌ [RISK CONTROL] Risk-reward ratio %.2f below minimum %.2f for %s",
				ratio, minRiskRewardRatio, decision.Symbol)
		}
	}

	return nil
}

func (at *AutoTrader) effectiveMinOpenConfidence(configured int) int {
	if configured <= 0 {
		return configured
	}
	if at.isHunterV7Strategy() && configured > 70 {
		return 70
	}
	return configured
}

// getSideFromAction converts order action to side (BUY/SELL)
func getSideFromAction(action string) string {
	switch action {
	case "open_long", "close_short":
		return "BUY"
	case "open_short", "close_long":
		return "SELL"
	default:
		return "BUY"
	}
}
