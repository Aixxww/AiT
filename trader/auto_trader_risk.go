package trader

import (
	"fmt"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	positionProtectorFastInterval     = 15 * time.Second
	positionProtectorBaseInterval     = 30 * time.Second
	positionProtectorIdleInterval     = 60 * time.Second
	protectorTP1PnLPct                = 6.0
	protectorTP2PnLPct                = 12.0
	protectorTP1CloseRatio            = 0.40
	protectorTP2CloseRatio            = 0.50
	protectorTrailDrawdownPct         = 35.0
	protectorPreTPPeakPnLPct          = 8.0
	protectorPreTPGivebackPct         = 80.0
	protectorNearTP1PeakRatio         = 0.95
	protectorNearTP1SecondChanceRatio = 0.90
	protectorNearTP1GivebackPct       = 45.0
	protectorNearTP1LossExitPnLPct    = -5.0
	protectorPreTPMinCurrentPnLPct    = 3.0
	protectorPreTPMinHoldDuration     = 20 * time.Minute
	protectorBreakevenBufferPct       = 0.001
	protectorDefaultMinCloseNotional  = 12.0
)

type positionProtectionState struct {
	InitialQuantity float64
	TP1Done         bool
	TP2Done         bool
	PeakPnLPct      float64
	OpenedAt        time.Time
	LastActionAt    time.Time
}

type protectionAction string

const (
	protectionNone          protectionAction = ""
	protectionTP1           protectionAction = "tp1"
	protectionTP2           protectionAction = "tp2"
	protectionTrailClose    protectionAction = "trail_close"
	protectionGivebackClose protectionAction = "giveback_close"
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

		// Construct unique position identifier (distinguish long/short)
		posKey := symbol + "_" + side

		openedAt := at.resolveProtectionPositionOpenedAt(symbol, side)
		at.ensurePeakPnLCacheInitialized(symbol, side, currentPnLPct, openedAt)
		at.UpdatePeakPnL(symbol, side, currentPnLPct)
		state := at.getOrCreateProtectionState(posKey, quantity, currentPnLPct, openedAt)
		action, drawdownPct := choosePositionProtectionAction(state, currentPnLPct)
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

func shouldUseFastProtectionInterval(state *positionProtectionState, currentPnLPct float64) bool {
	if state == nil {
		return false
	}
	nearTP1PeakPnLPct := protectorTP1PnLPct * protectorNearTP1PeakRatio
	return state.TP1Done || state.PeakPnLPct >= nearTP1PeakPnLPct || currentPnLPct >= nearTP1PeakPnLPct
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

func choosePositionProtectionAction(state *positionProtectionState, currentPnLPct float64) (protectionAction, float64) {
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
	if !state.TP1Done && currentPnLPct >= protectorTP1PnLPct {
		return protectionTP1, drawdownPct
	}
	if state.TP1Done && !state.TP2Done && currentPnLPct >= protectorTP2PnLPct {
		return protectionTP2, drawdownPct
	}
	if state.TP1Done && state.PeakPnLPct >= protectorTP1PnLPct && currentPnLPct > 0 && drawdownPct >= protectorTrailDrawdownPct {
		return protectionTrailClose, drawdownPct
	}
	nearTP1PeakPnLPct := protectorTP1PnLPct * protectorNearTP1PeakRatio
	secondChancePnLPct := protectorTP1PnLPct * protectorNearTP1SecondChanceRatio
	if !state.TP1Done &&
		state.PeakPnLPct >= nearTP1PeakPnLPct &&
		currentPnLPct >= secondChancePnLPct &&
		time.Since(state.OpenedAt) >= protectorPreTPMinHoldDuration {
		return protectionGivebackClose, drawdownPct
	}
	if !state.TP1Done &&
		state.PeakPnLPct >= nearTP1PeakPnLPct &&
		time.Since(state.OpenedAt) >= protectorPreTPMinHoldDuration &&
		drawdownPct >= protectorNearTP1GivebackPct &&
		(currentPnLPct >= protectorPreTPMinCurrentPnLPct || currentPnLPct <= protectorNearTP1LossExitPnLPct) {
		return protectionGivebackClose, drawdownPct
	}
	if !state.TP1Done &&
		state.PeakPnLPct >= protectorPreTPPeakPnLPct &&
		currentPnLPct >= protectorPreTPMinCurrentPnLPct &&
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
	case protectionTP1:
		state.TP1Done = true
	case protectionTP2:
		state.TP2Done = true
	case protectionTrailClose, protectionGivebackClose:
		delete(at.protectionState, posKey)
	}
}

func (at *AutoTrader) executeProtectionAction(symbol, side string, quantity, markPrice, entryPrice float64, action protectionAction) (bool, error) {
	switch action {
	case protectionTP1:
		closeQty, closeAll := at.protectionCloseQuantity(quantity, markPrice, protectorTP1CloseRatio)
		logger.Infof("🟢 TP1 protection triggered: %s %s | close %.8f / %.8f | mark %.8f", symbol, side, closeQty, quantity, markPrice)
		if closeAll {
			return true, at.closeProtectedPosition(symbol, side, 0)
		}
		if err := at.closeProtectedPosition(symbol, side, closeQty); err != nil {
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
			return true, at.closeProtectedPosition(symbol, side, 0)
		}
		if err := at.closeProtectedPosition(symbol, side, closeQty); err != nil {
			return false, err
		}
		if err := at.rebuildProtectionStops(symbol, side, quantity-closeQty, entryPrice); err != nil {
			logger.Infof("⚠️ Failed to rebuild stops after TP2 close (%s %s): %v", symbol, side, err)
		}
		return false, nil
	case protectionTrailClose, protectionGivebackClose:
		logger.Infof("🟢 Trail protection close triggered: %s %s | action=%s | mark %.8f", symbol, side, action, markPrice)
		return true, at.closeProtectedPosition(symbol, side, 0)
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

func (at *AutoTrader) closeProtectedPosition(symbol, side string, quantity float64) error {
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
	return nil
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
	minRR := riskControl.MinRiskRewardRatio
	if minRR <= 0 {
		minRR = 1.5
	}
	minStopPct := at.minStopLossPriceMovePct()
	if minStopPct <= 0 {
		minStopPct = 2.0
	}
	maxDriftPct := at.maxEntryPriceDeviationPct()
	if maxDriftPct <= 0 {
		maxDriftPct = 0.5
	}
	minFeasiblePct := (minStopPct + maxDriftPct) * minRR
	minFeasiblePct += 0.25 // execution/rounding buffer in percentage points
	if pct < minFeasiblePct {
		return minFeasiblePct
	}
	return pct
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

func (at *AutoTrader) validateHunterV7ExecutionGuard(ctx *kernel.Context, decision *kernel.Decision) error {
	if ctx == nil || decision == nil || !at.isHunterV7Strategy() {
		return nil
	}
	if decision.Action != "open_short" && decision.Action != "open_long" {
		return nil
	}

	candidate := hunterV7CandidateForDecision(ctx, decision)
	if candidate == nil || candidate.V7SetupType == "" {
		return nil
	}

	// Look up per-setup guard thresholds from strategy config
	guard := at.setupGuardForSetup(candidate.V7SetupType)
	if guard == nil {
		return nil // no guard configured for this setup
	}

	price := hunterV7DecisionReferencePrice(ctx, candidate, decision)
	if price <= 0 {
		return nil
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
	case "range_reversion":
		return &setupGuardDefaults{MinZonePosShort: 55, MaxZonePosLong: 45, RequireOIFlush: false}
	case "pullback_reversal_long":
		return &setupGuardDefaults{MinZonePosShort: 0, MaxZonePosLong: 50, RequireOIFlush: false}
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
	if candidate == nil || price <= 0 {
		return 0, false
	}
	lower := candidate.V7EntryZone.Lower
	upper := candidate.V7EntryZone.Upper
	if lower <= 0 || upper <= lower {
		return 0, false
	}
	return (price - lower) / (upper - lower) * 100, true
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
