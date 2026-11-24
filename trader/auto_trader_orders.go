package trader

import (
	"errors"
	"fmt"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
	"time"
)

type maxLeverageProvider interface {
	GetMaxLeverage(symbol string) (int, error)
}

func (at *AutoTrader) resolveEffectiveLeverage(symbol string, requested int) int {
	if requested <= 0 {
		return requested
	}
	provider, ok := at.trader.(maxLeverageProvider)
	if !ok {
		return requested
	}
	maxLeverage, err := provider.GetMaxLeverage(symbol)
	if err != nil {
		logger.Infof("  ⚠️ Failed to resolve %s exchange max leverage, using requested %dx: %v", symbol, requested, err)
		return requested
	}
	if maxLeverage > 0 && requested > maxLeverage {
		logger.Infof("  ⚠️ %s leverage %dx exceeds exchange max %dx, using %dx for margin and order execution",
			symbol, requested, maxLeverage, maxLeverage)
		return maxLeverage
	}
	return requested
}

func (at *AutoTrader) resolveOpenExecutionPrice(symbol, side string, fallbackPrice float64) float64 {
	if gridTrader, ok := at.trader.(GridTrader); ok {
		bids, asks, err := gridTrader.GetOrderBook(symbol, 5)
		if err == nil {
			switch side {
			case "long":
				if len(asks) > 0 && len(asks[0]) > 0 && asks[0][0] > 0 {
					return asks[0][0]
				}
			case "short":
				if len(bids) > 0 && len(bids[0]) > 0 && bids[0][0] > 0 {
					return bids[0][0]
				}
			}
		} else {
			logger.Infof("  ⚠️ Failed to get %s order book for execution price validation: %v", symbol, err)
		}
	}

	if at.trader != nil {
		if price, err := at.trader.GetMarketPrice(symbol); err == nil && price > 0 {
			return price
		}
	}
	return fallbackPrice
}

// executeDecisionWithRecord executes AI decision and records detailed information
func (at *AutoTrader) executeDecisionWithRecord(ctx *kernel.Context, decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	switch decision.Action {
	case "open_long", "open_short":
		side := "long"
		if decision.Action == "open_short" {
			side = "short"
		}
		// F08: no pending entry is registered while the breaker blocks.
		if err := at.enforceDailyLossBreaker(decision.Symbol, decision.Action); err != nil {
			return err
		}
		// U9: an open on a conditional candidate becomes a pending entry.
		if decision.HunterV7ConditionalEntry {
			return at.registerHunterV7ConditionalEntry(ctx, decision, side, actionRecord, time.Now())
		}
		if side == "long" {
			return at.executeOpenLongWithRecord(ctx, decision, actionRecord)
		}
		return at.executeOpenShortWithRecord(ctx, decision, actionRecord)
	case "close_long":
		return at.executeCloseLongWithRecord(decision, actionRecord)
	case "close_short":
		return at.executeCloseShortWithRecord(decision, actionRecord)
	case "hold", "wait":
		// No execution needed, just record
		return nil
	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}
}

func (at *AutoTrader) recordEffectiveOpenContract(actionRecord *store.DecisionAction, decision *kernel.Decision, executionPrice, quantity, requestedPositionSize float64, tpWasCapped, positionWasReduced bool, side string) {
	if actionRecord == nil || decision == nil {
		return
	}
	actionRecord.Price = executionPrice
	actionRecord.StopLoss = decision.StopLoss
	actionRecord.TakeProfit = decision.TakeProfit
	actionRecord.EffectiveStopLoss = decision.StopLoss
	actionRecord.EffectiveTakeProfit = decision.TakeProfit
	actionRecord.TPWasCapped = tpWasCapped
	actionRecord.PositionWasReduced = positionWasReduced || (requestedPositionSize > 0 && decision.PositionSizeUSD > 0 && decision.PositionSizeUSD < requestedPositionSize)
	if quantity > 0 {
		actionRecord.Quantity = quantity
	}
	actionRecord.EffectivePositionSizeUSD = decision.PositionSizeUSD
	if actionRecord.EffectivePositionSizeUSD <= 0 && executionPrice > 0 && quantity > 0 {
		actionRecord.EffectivePositionSizeUSD = executionPrice * quantity
	}
	actionRecord.RiskAtStopUSD, actionRecord.RRAfterBackendRepair = effectiveOpenRiskMetrics(
		side,
		executionPrice,
		decision.StopLoss,
		decision.TakeProfit,
		actionRecord.EffectivePositionSizeUSD,
	)
	actionRecord.FinalRR = actionRecord.RRAfterBackendRepair
	actionRecord.FinalEffectiveTakeProfit = decision.TakeProfit
	actionRecord.FinalStopLoss = decision.StopLoss
}

func effectiveOpenRiskMetrics(side string, entryPrice, stopLoss, takeProfit, positionSizeUSD float64) (float64, float64) {
	if entryPrice <= 0 || stopLoss <= 0 || takeProfit <= 0 || positionSizeUSD <= 0 {
		return 0, 0
	}
	riskDistance := 0.0
	rewardDistance := 0.0
	switch side {
	case "long":
		riskDistance = entryPrice - stopLoss
		rewardDistance = takeProfit - entryPrice
	case "short":
		riskDistance = stopLoss - entryPrice
		rewardDistance = entryPrice - takeProfit
	default:
		return 0, 0
	}
	if riskDistance <= 0 || rewardDistance <= 0 {
		return 0, 0
	}
	return positionSizeUSD * riskDistance / entryPrice, rewardDistance / riskDistance
}

// executeOpenLongWithRecord executes open long position and records detailed information
func (at *AutoTrader) executeOpenLongWithRecord(ctx *kernel.Context, decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	// F08: also covers conditional fills, which call this directly.
	if err := at.enforceDailyLossBreaker(decision.Symbol, decision.Action); err != nil {
		return err
	}
	logger.Infof("  📈 Open long: %s", decision.Symbol)
	requestedPositionSize := decision.PositionSizeUSD
	decisionPriceBeforeRepair := decision.Price
	if err := at.enforceHunterV7EntryContract(decision); err != nil {
		return err
	}

	// ⚠️ Get current positions for multiple checks
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("failed to get positions: %w", err)
	}

	// [CODE ENFORCED] Check max positions limit
	// AUD-C3: never below what this response has already opened.
	if err := at.enforceMaxPositions(at.effectivePositionCount(len(positions))); err != nil {
		return err
	}

	// Check if there's already a position in the same symbol and direction
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
			return fmt.Errorf("❌ %s already has long position, close it first", decision.Symbol)
		}
	}

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	executionPrice := at.resolveOpenExecutionPrice(decision.Symbol, "long", marketData.CurrentPrice)

	// Get balance (needed for multiple checks)
	balance, err := at.trader.GetBalance()
	if err != nil {
		return fmt.Errorf("failed to get account balance: %w", err)
	}
	availableBalance := 0.0
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Get equity for position value ratio check
	equity := 0.0
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		equity = eq
	} else if eq, ok := balance["totalWalletBalance"].(float64); ok && eq > 0 {
		equity = eq
	} else {
		equity = availableBalance // Fallback to available balance
	}

	// [CODE ENFORCED] Position Value Ratio Check: position_value <= equity × ratio
	adjustedPositionSize, wasCapped := at.enforcePositionValueRatio(decision.PositionSizeUSD, equity, decision.Symbol)
	if wasCapped {
		decision.PositionSizeUSD = adjustedPositionSize
	}
	positionWasReduced := wasCapped

	effectiveLeverage := at.resolveEffectiveLeverage(decision.Symbol, decision.Leverage)
	decision.Leverage = effectiveLeverage
	actionRecord.Leverage = effectiveLeverage

	// ⚠️ Auto-adjust position size if insufficient margin
	// Formula: totalRequired = positionSize/leverage + positionSize*0.001 + positionSize/leverage*0.01
	//        = positionSize * (1.01/leverage + 0.001)
	marginFactor := 1.01/float64(decision.Leverage) + 0.001
	maxAffordablePositionSize := availableBalance / marginFactor

	actualPositionSize := decision.PositionSizeUSD
	if actualPositionSize > maxAffordablePositionSize {
		// Use 98% of max to leave buffer for price fluctuation
		adjustedSize := maxAffordablePositionSize * 0.98
		logger.Infof("  ⚠️ Position size %.2f exceeds max affordable %.2f, auto-reducing to %.2f",
			actualPositionSize, maxAffordablePositionSize, adjustedSize)
		actualPositionSize = adjustedSize
		decision.PositionSizeUSD = actualPositionSize
		positionWasReduced = true
	}

	tpWasCapped := at.capTakeProfitToTP1(decision, executionPrice, "long")
	at.repairHunterV7OpenDecision(decision, executionPrice, "long")
	if err := at.validateOpenDecision(decision, executionPrice, "long"); err != nil {
		at.recordEffectiveOpenContract(actionRecord, decision, executionPrice, 0, requestedPositionSize, tpWasCapped, positionWasReduced, "long")
		return err
	}
	actualPositionSize = decision.PositionSizeUSD

	if adjustedSize, wasCapped, err := at.enforceSingleTradeLossLimit(decision, executionPrice, equity, "long"); err != nil {
		return err
	} else if wasCapped {
		actualPositionSize = adjustedSize
		decision.PositionSizeUSD = adjustedSize
		positionWasReduced = true
	}

	// [CODE ENFORCED] Minimum position size check
	if err := at.enforceMinPositionSize(decision.PositionSizeUSD); err != nil {
		at.recordEffectiveOpenContract(actionRecord, decision, executionPrice, 0, requestedPositionSize, tpWasCapped, positionWasReduced, "long")
		return err
	}

	if err := at.validateOpenDecision(decision, executionPrice, "long"); err != nil {
		at.recordEffectiveOpenContract(actionRecord, decision, executionPrice, 0, requestedPositionSize, tpWasCapped, positionWasReduced, "long")
		return err
	}

	if finalExecutionPrice, finalTPCapped, err := at.refreshHunterV7OpenPreflight(ctx, decision, executionPrice, decisionPriceBeforeRepair, "long"); err != nil {
		at.recordEffectiveOpenContract(actionRecord, decision, finalExecutionPrice, 0, requestedPositionSize, tpWasCapped || finalTPCapped, positionWasReduced, "long")
		return err
	} else {
		executionPrice = finalExecutionPrice
		tpWasCapped = tpWasCapped || finalTPCapped
	}

	// Calculate quantity with adjusted position size
	quantity := actualPositionSize / executionPrice
	actionRecord.Quantity = quantity
	actionRecord.Price = executionPrice
	at.recordEffectiveOpenContract(actionRecord, decision, executionPrice, quantity, requestedPositionSize, tpWasCapped, positionWasReduced, "long")

	// Set margin mode
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Infof("  ⚠️ Failed to set margin mode: %v", err)
		// Continue execution, doesn't affect trading
	}

	// Open position
	order, err := at.openMarketPosition(decision, "long", quantity)
	if err != nil {
		if errors.Is(err, errOpenOrderStateUnknown) {
			// AUD-C5: the order may have filled; protect it or close it.
			return at.handleOpenOrderStateUnknown(decision, "long", actionRecord, err)
		}
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	logger.Infof("  ✓ Position opened successfully, order ID: %v, quantity: %.4f", order["orderId"], quantity)

	at.ClearPeakPnLCache(decision.Symbol, "long")

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "open_long", quantity, executionPrice, decision.Leverage, 0)

	// Record position opening time
	posKey := decision.Symbol + "_long"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// Set stop loss and take profit
	// F03: the stop is mandatory. If it cannot be placed the position is
	// emergency-closed and verified flat; the take-profit is not placed.
	if err := at.trader.SetStopLoss(decision.Symbol, "LONG", quantity, decision.StopLoss); err != nil {
		return at.closeUnprotectedPosition(decision.Symbol, "long", quantity, fmt.Errorf("failed to set mandatory stop loss: %w", err))
	}
	// A missing take-profit leaves the position protected by its stop; it is
	// reported, not fatal.
	if err := at.trader.SetTakeProfit(decision.Symbol, "LONG", quantity, decision.TakeProfit); err != nil {
		logger.Errorf("  ⚠ Failed to set take profit for %s LONG (stop is in place): %v", decision.Symbol, err)
		actionRecord.Error = fmt.Sprintf("take_profit_not_placed: %v", err)
	}

	return nil
}

// executeOpenShortWithRecord executes open short position and records detailed information
func (at *AutoTrader) executeOpenShortWithRecord(ctx *kernel.Context, decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	// F08: also covers conditional fills, which call this directly.
	if err := at.enforceDailyLossBreaker(decision.Symbol, decision.Action); err != nil {
		return err
	}
	logger.Infof("  📉 Open short: %s", decision.Symbol)
	requestedPositionSize := decision.PositionSizeUSD
	decisionPriceBeforeRepair := decision.Price
	if err := at.enforceHunterV7EntryContract(decision); err != nil {
		return err
	}

	// ⚠️ Get current positions for multiple checks
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("failed to get positions: %w", err)
	}

	// [CODE ENFORCED] Check max positions limit
	// AUD-C3: never below what this response has already opened.
	if err := at.enforceMaxPositions(at.effectivePositionCount(len(positions))); err != nil {
		return err
	}

	// Check if there's already a position in the same symbol and direction
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
			return fmt.Errorf("❌ %s already has short position, close it first", decision.Symbol)
		}
	}

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	executionPrice := at.resolveOpenExecutionPrice(decision.Symbol, "short", marketData.CurrentPrice)

	// Get balance (needed for multiple checks)
	balance, err := at.trader.GetBalance()
	if err != nil {
		return fmt.Errorf("failed to get account balance: %w", err)
	}
	availableBalance := 0.0
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Get equity for position value ratio check
	equity := 0.0
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		equity = eq
	} else if eq, ok := balance["totalWalletBalance"].(float64); ok && eq > 0 {
		equity = eq
	} else {
		equity = availableBalance // Fallback to available balance
	}

	// [CODE ENFORCED] Position Value Ratio Check: position_value <= equity × ratio
	adjustedPositionSize, wasCapped := at.enforcePositionValueRatio(decision.PositionSizeUSD, equity, decision.Symbol)
	if wasCapped {
		decision.PositionSizeUSD = adjustedPositionSize
	}
	positionWasReduced := wasCapped

	effectiveLeverage := at.resolveEffectiveLeverage(decision.Symbol, decision.Leverage)
	decision.Leverage = effectiveLeverage
	actionRecord.Leverage = effectiveLeverage

	// ⚠️ Auto-adjust position size if insufficient margin
	// Formula: totalRequired = positionSize/leverage + positionSize*0.001 + positionSize/leverage*0.01
	//        = positionSize * (1.01/leverage + 0.001)
	marginFactor := 1.01/float64(decision.Leverage) + 0.001
	maxAffordablePositionSize := availableBalance / marginFactor

	actualPositionSize := decision.PositionSizeUSD
	if actualPositionSize > maxAffordablePositionSize {
		// Use 98% of max to leave buffer for price fluctuation
		adjustedSize := maxAffordablePositionSize * 0.98
		logger.Infof("  ⚠️ Position size %.2f exceeds max affordable %.2f, auto-reducing to %.2f",
			actualPositionSize, maxAffordablePositionSize, adjustedSize)
		actualPositionSize = adjustedSize
		decision.PositionSizeUSD = actualPositionSize
		positionWasReduced = true
	}

	tpWasCapped := at.capTakeProfitToTP1(decision, executionPrice, "short")
	at.repairHunterV7OpenDecision(decision, executionPrice, "short")
	if err := at.validateOpenDecision(decision, executionPrice, "short"); err != nil {
		at.recordEffectiveOpenContract(actionRecord, decision, executionPrice, 0, requestedPositionSize, tpWasCapped, positionWasReduced, "short")
		return err
	}
	actualPositionSize = decision.PositionSizeUSD

	if adjustedSize, wasCapped, err := at.enforceSingleTradeLossLimit(decision, executionPrice, equity, "short"); err != nil {
		return err
	} else if wasCapped {
		actualPositionSize = adjustedSize
		decision.PositionSizeUSD = adjustedSize
		positionWasReduced = true
	}

	// [CODE ENFORCED] Minimum position size check
	if err := at.enforceMinPositionSize(decision.PositionSizeUSD); err != nil {
		at.recordEffectiveOpenContract(actionRecord, decision, executionPrice, 0, requestedPositionSize, tpWasCapped, positionWasReduced, "short")
		return err
	}

	if err := at.validateOpenDecision(decision, executionPrice, "short"); err != nil {
		at.recordEffectiveOpenContract(actionRecord, decision, executionPrice, 0, requestedPositionSize, tpWasCapped, positionWasReduced, "short")
		return err
	}

	if finalExecutionPrice, finalTPCapped, err := at.refreshHunterV7OpenPreflight(ctx, decision, executionPrice, decisionPriceBeforeRepair, "short"); err != nil {
		at.recordEffectiveOpenContract(actionRecord, decision, finalExecutionPrice, 0, requestedPositionSize, tpWasCapped || finalTPCapped, positionWasReduced, "short")
		return err
	} else {
		executionPrice = finalExecutionPrice
		tpWasCapped = tpWasCapped || finalTPCapped
	}

	// Calculate quantity with adjusted position size
	quantity := actualPositionSize / executionPrice
	actionRecord.Quantity = quantity
	actionRecord.Price = executionPrice
	at.recordEffectiveOpenContract(actionRecord, decision, executionPrice, quantity, requestedPositionSize, tpWasCapped, positionWasReduced, "short")

	// Set margin mode
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Infof("  ⚠️ Failed to set margin mode: %v", err)
		// Continue execution, doesn't affect trading
	}

	// Open position
	order, err := at.openMarketPosition(decision, "short", quantity)
	if err != nil {
		if errors.Is(err, errOpenOrderStateUnknown) {
			// AUD-C5: the order may have filled; protect it or close it.
			return at.handleOpenOrderStateUnknown(decision, "short", actionRecord, err)
		}
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	logger.Infof("  ✓ Position opened successfully, order ID: %v, quantity: %.4f", order["orderId"], quantity)

	at.ClearPeakPnLCache(decision.Symbol, "short")

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "open_short", quantity, executionPrice, decision.Leverage, 0)

	// Record position opening time
	posKey := decision.Symbol + "_short"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// Set stop loss and take profit
	// F03: the stop is mandatory. If it cannot be placed the position is
	// emergency-closed and verified flat; the take-profit is not placed.
	if err := at.trader.SetStopLoss(decision.Symbol, "SHORT", quantity, decision.StopLoss); err != nil {
		return at.closeUnprotectedPosition(decision.Symbol, "short", quantity, fmt.Errorf("failed to set mandatory stop loss: %w", err))
	}
	// A missing take-profit leaves the position protected by its stop; it is
	// reported, not fatal.
	if err := at.trader.SetTakeProfit(decision.Symbol, "SHORT", quantity, decision.TakeProfit); err != nil {
		logger.Errorf("  ⚠ Failed to set take profit for %s SHORT (stop is in place): %v", decision.Symbol, err)
		actionRecord.Error = fmt.Sprintf("take_profit_not_placed: %v", err)
	}

	return nil
}

// executeCloseLongWithRecord executes close long position and records detailed information
func (at *AutoTrader) executeCloseLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close long: %s", decision.Symbol)

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	// Normalize symbol for database lookup
	normalizedSymbol := market.Normalize(decision.Symbol)

	// Get entry price and quantity - prioritize local database for accurate quantity
	var entryPrice float64
	var quantity float64

	// First try to get from local database (more accurate for quantity)
	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, "LONG"); err == nil && openPos != nil {
			quantity = openPos.Quantity
			entryPrice = openPos.EntryPrice
			logger.Infof("  📊 Using local position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
		}
	}

	// Fallback to exchange API if local data not found
	if quantity == 0 {
		positions, err := at.trader.GetPositions()
		if err == nil {
			for _, pos := range positions {
				if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
					if ep, ok := pos["entryPrice"].(float64); ok {
						entryPrice = ep
					}
					if amt, ok := pos["positionAmt"].(float64); ok && amt > 0 {
						quantity = amt
					}
					break
				}
			}
		}
		logger.Infof("  📊 Using exchange position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
	}

	// Close position
	order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_long", quantity, marketData.CurrentPrice, 0, entryPrice)

	at.ClearPeakPnLCache(decision.Symbol, "long")

	logger.Infof("  ✓ Position closed successfully")
	return nil
}

// executeCloseShortWithRecord executes close short position and records detailed information
func (at *AutoTrader) executeCloseShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close short: %s", decision.Symbol)

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	// Normalize symbol for database lookup
	normalizedSymbol := market.Normalize(decision.Symbol)

	// Get entry price and quantity - prioritize local database for accurate quantity
	var entryPrice float64
	var quantity float64

	// First try to get from local database (more accurate for quantity)
	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, "SHORT"); err == nil && openPos != nil {
			quantity = openPos.Quantity
			entryPrice = openPos.EntryPrice
			logger.Infof("  📊 Using local position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
		}
	}

	// Fallback to exchange API if local data not found
	if quantity == 0 {
		positions, err := at.trader.GetPositions()
		if err == nil {
			for _, pos := range positions {
				if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
					if ep, ok := pos["entryPrice"].(float64); ok {
						entryPrice = ep
					}
					if amt, ok := pos["positionAmt"].(float64); ok {
						quantity = -amt // positionAmt is negative for short
					}
					break
				}
			}
		}
		logger.Infof("  📊 Using exchange position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
	}

	// Close position
	order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_short", quantity, marketData.CurrentPrice, 0, entryPrice)

	at.ClearPeakPnLCache(decision.Symbol, "short")

	logger.Infof("  ✓ Position closed successfully")
	return nil
}
