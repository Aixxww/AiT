package trader

import (
	"encoding/json"
	"fmt"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
	"github.com/Aixxww/AiT/wallet"
	"strings"
	"time"
)

const maxDegradedContextCacheAge = 30 * time.Minute

// runCycle runs one trading cycle (using AI full decision-making)
func (at *AutoTrader) runCycle() error {
	at.callCount++
	cycleNumber := at.callCount
	startedAt := time.Now()
	defer func() {
		at.logInfof("✅ runCycle #%d returned after %s", cycleNumber, time.Since(startedAt).Round(time.Millisecond))
	}()

	logger.Info("\n" + strings.Repeat("=", 70) + "\n")
	logger.Infof("⏰ %s - AI decision cycle #%d", time.Now().Format("2006-01-02 15:04:05"), at.callCount)
	logger.Info(strings.Repeat("=", 70))

	// 0. Check if trader is stopped (early exit to prevent trades after Stop() is called)
	at.isRunningMutex.RLock()
	running := at.isRunning
	at.isRunningMutex.RUnlock()
	if !running {
		at.logInfof("⏹ Trader is stopped, aborting cycle #%d", at.callCount)
		return nil
	}

	// Check USDC balance periodically for claw402 users (every 10 cycles)
	if at.callCount%10 == 0 && store.IsClaw402Config(at.config.AIModel) {
		at.checkClaw402Balance()
	}

	// Create decision record
	record := &store.DecisionRecord{
		ExecutionLog: []string{},
		Success:      true,
	}

	// 1. Check if trading needs to be stopped
	if time.Now().Before(at.stopUntil) {
		remaining := at.stopUntil.Sub(time.Now())
		at.logWarnf("⏸ Risk control: Trading paused, remaining %.0f minutes", remaining.Minutes())
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Risk control paused, remaining %.0f minutes", remaining.Minutes())
		at.saveDecision(record)
		return nil
	}

	// 2. Reset daily P&L (reset every day)
	if time.Since(at.lastResetTime) > 24*time.Hour {
		at.dailyPnL = 0
		at.lastResetTime = time.Now()
		logger.Info("📅 Daily P&L reset")
	}

	// 4. Collect trading context
	ctx, err := at.buildTradingContext()
	if err != nil {
		at.logErrorf("failed to build trading context: %v", err)
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Failed to build trading context: %v", err)
		at.saveDecision(record)
		return fmt.Errorf("failed to build trading context: %w", err)
	}
	if ctx.IsDegraded {
		reason := strings.Join(ctx.DegradationReasons, "; ")
		at.logWarnf("⚠️ Trading context degraded: %s", reason)
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("⚠️ Trading context degraded: %s", reason))
	}

	// Save equity snapshot independently (decoupled from AI decision, used for drawing profit curve)
	// NOTE: Must be called BEFORE candidate coins check to ensure equity is always recorded
	at.saveEquitySnapshot(ctx)

	// If no candidate coins available, log but do not error
	if len(ctx.CandidateCoins) == 0 {
		at.logInfof("ℹ️ No candidate coins available, skipping this cycle")
		record.Success = true // Not an error, just no candidate coins
		record.CandidateCoins = []string{}
		record.DecisionJSON = `[
  {
    "symbol": "ALL",
    "action": "wait",
    "reasoning": "no_candidate_coins"
  }
]`
		record.Decisions = []store.DecisionAction{
			{
				Action:    "wait",
				Symbol:    "ALL",
				Reasoning: "no_candidate_coins",
				Timestamp: time.Now(),
				Success:   true,
			},
		}
		record.ExecutionLog = append(record.ExecutionLog, "No candidate coins available, cycle skipped")
		record.AccountState = store.AccountSnapshot{
			TotalBalance:          ctx.Account.TotalEquity,
			AvailableBalance:      ctx.Account.AvailableBalance,
			TotalUnrealizedProfit: ctx.Account.UnrealizedPnL,
			PositionCount:         ctx.Account.PositionCount,
			InitialBalance:        at.initialBalance,
		}
		at.saveDecision(record)
		return nil
	}

	logger.Info(strings.Repeat("=", 70))
	for _, coin := range ctx.CandidateCoins {
		record.CandidateCoins = append(record.CandidateCoins, coin.Symbol)
	}

	if at.shouldSkipHunterV7NoExecutable(ctx, record) {
		at.saveDecision(record)
		return nil
	}

	at.logInfof("📊 Account equity: %.2f USDT | Available: %.2f USDT | Positions: %d",
		ctx.Account.TotalEquity, ctx.Account.AvailableBalance, ctx.Account.PositionCount)

	// 5. Use strategy engine to call AI for decision
	at.logInfof("🤖 Requesting AI analysis and decision... [Strategy Engine]")
	aiDecision, err := kernel.GetFullDecisionWithStrategy(ctx, at.mcpClient, at.strategyEngine, "balanced")

	if aiDecision != nil && aiDecision.AIRequestDurationMs > 0 {
		record.AIRequestDurationMs = aiDecision.AIRequestDurationMs
		record.PromptTokens = aiDecision.PromptTokens
		record.CompletionTokens = aiDecision.CompletionTokens
		record.TotalTokens = aiDecision.TotalTokens
		at.logInfof("⏱️ AI call duration: %.2f seconds | tokens: in=%d out=%d total=%d",
			float64(record.AIRequestDurationMs)/1000,
			record.PromptTokens, record.CompletionTokens, record.TotalTokens)
		record.ExecutionLog = append(record.ExecutionLog,
			fmt.Sprintf("AI call duration: %d ms | prompt: %d tokens, output: %d tokens, total: %d",
				record.AIRequestDurationMs, record.PromptTokens, record.CompletionTokens, record.TotalTokens))
	}

	// Save chain of thought, decisions, and input prompt even if there's an error (for debugging)
	if aiDecision != nil {
		record.SystemPrompt = aiDecision.SystemPrompt // Save system prompt
		record.InputPrompt = aiDecision.UserPrompt
		record.CoTTrace = aiDecision.CoTTrace
		record.RawResponse = aiDecision.RawResponse // Save raw AI response for debugging
		if len(aiDecision.Decisions) > 0 {
			decisionJSON, _ := json.MarshalIndent(aiDecision.Decisions, "", "  ")
			record.DecisionJSON = string(decisionJSON)
		}
	}

	// Record AI charge (track cost regardless of decision outcome)
	if aiDecision != nil && at.store != nil {
		if chargeErr := at.store.AICharge().Record(at.id, at.aiModel, at.config.AIModel); chargeErr != nil {
			at.logWarnf("⚠️ Failed to record AI charge: %v", chargeErr)
		}
	}

	if err != nil {
		at.consecutiveAIFailures++
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Failed to get AI decision: %v", err)

		// Activate safe mode after 3 consecutive failures
		if at.consecutiveAIFailures >= 3 && !at.safeMode {
			at.safeMode = true
			at.safeModeReason = fmt.Sprintf("AI failed %d consecutive times: %v", at.consecutiveAIFailures, err)
			at.logErrorf("🛡️ SAFE MODE ACTIVATED — AI failed %d times in a row. No new positions will be opened. Existing positions are protected with current stop-loss settings.", at.consecutiveAIFailures)
			at.logErrorf("🛡️ Reason: %v", err)
			at.logErrorf("🛡️ Action: Will keep trying AI each cycle. Safe mode auto-deactivates when AI recovers.")
		}

		// Print system prompt and AI chain of thought (output even with errors for debugging)
		if aiDecision != nil {
			logger.Info("\n" + strings.Repeat("=", 70) + "\n")
			logger.Infof("📋 System prompt (error case)")
			logger.Info(strings.Repeat("=", 70))
			logger.Info(aiDecision.SystemPrompt)
			logger.Info(strings.Repeat("=", 70))

			if aiDecision.CoTTrace != "" {
				logger.Info("\n" + strings.Repeat("-", 70) + "\n")
				logger.Info("💭 AI chain of thought analysis (error case):")
				logger.Info(strings.Repeat("-", 70))
				logger.Info(aiDecision.CoTTrace)
				logger.Info(strings.Repeat("-", 70))
			}
		}

		at.saveDecision(record)

		// In safe mode, don't return error — keep the loop running to retry next cycle
		if at.safeMode {
			at.logWarnf("🛡️ Safe mode: skipping this cycle, will retry in %v", at.config.ScanInterval)
			return nil
		}

		return fmt.Errorf("failed to get AI decision: %w", err)
	}

	// AI succeeded — reset failure counter and deactivate safe mode
	if at.consecutiveAIFailures > 0 {
		at.logInfof("✅ AI recovered after %d consecutive failures", at.consecutiveAIFailures)
	}
	at.consecutiveAIFailures = 0
	if at.safeMode {
		at.logInfof("🛡️ SAFE MODE DEACTIVATED — AI is working again. Resuming normal trading.")
		at.safeMode = false
		at.safeModeReason = ""
	}

	// // 5. Print system prompt
	// logger.Infof("\n" + strings.Repeat("=", 70))
	// logger.Infof("📋 System prompt [template: %s]", at.systemPromptTemplate)
	// logger.Info(strings.Repeat("=", 70))
	// logger.Info(decision.SystemPrompt)
	// logger.Infof(strings.Repeat("=", 70) + "\n")

	// 6. Print AI chain of thought
	// logger.Infof("\n" + strings.Repeat("-", 70))
	// logger.Info("💭 AI chain of thought analysis:")
	// logger.Info(strings.Repeat("-", 70))
	// logger.Info(decision.CoTTrace)
	// logger.Infof(strings.Repeat("-", 70) + "\n")

	// 7. Print AI decisions
	// logger.Infof("📋 AI decision list (%d items):\n", len(kernel.Decisions))
	// for i, d := range kernel.Decisions {
	//     logger.Infof("  [%d] %s: %s - %s", i+1, d.Symbol, d.Action, d.Reasoning)
	//     if d.Action == "open_long" || d.Action == "open_short" {
	//        logger.Infof("      Leverage: %dx | Position: %.2f USDT | Stop loss: %.4f | Take profit: %.4f",
	//           d.Leverage, d.PositionSizeUSD, d.StopLoss, d.TakeProfit)
	//     }
	// }
	logger.Info()
	logger.Info(strings.Repeat("-", 70))
	// 8. Sort decisions: ensure close positions first, then open positions (prevent position stacking overflow)
	logger.Info(strings.Repeat("-", 70))

	// 8. Sort decisions: ensure close positions first, then open positions (prevent position stacking overflow)
	sortedDecisions := sortDecisionsByPriority(aiDecision.Decisions)

	logger.Info("🔄 Execution order (optimized): Close positions first → Open positions later")
	for i, d := range sortedDecisions {
		logger.Infof("  [%d] %s %s", i+1, d.Symbol, d.Action)
	}
	logger.Info()

	// Check if trader is stopped before executing any decisions (prevent trades after Stop())
	at.isRunningMutex.RLock()
	running = at.isRunning
	at.isRunningMutex.RUnlock()
	if !running {
		at.logInfof("⏹ Trader stopped before decision execution, aborting cycle #%d", at.callCount)
		return nil
	}

	// Safe mode: filter out open positions, only allow close/hold
	if at.safeMode {
		filtered := make([]kernel.Decision, 0)
		for _, d := range sortedDecisions {
			if d.Action == "open_long" || d.Action == "open_short" {
				at.logWarnf("🛡️ Safe mode: BLOCKED %s %s (no new positions allowed)", d.Action, d.Symbol)
				continue
			}
			filtered = append(filtered, d)
		}
		sortedDecisions = filtered
		if len(sortedDecisions) == 0 {
			at.logInfof("🛡️ Safe mode: all decisions were open positions, nothing to execute")
		}
	}

	if ctx.DisableOpenOrders {
		filtered := make([]kernel.Decision, 0, len(sortedDecisions))
		for _, d := range sortedDecisions {
			if d.Action == "open_long" || d.Action == "open_short" {
				at.logWarnf("⚠️ Degraded context: BLOCKED %s %s (open orders disabled)", d.Action, d.Symbol)
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("⚠️ Degraded context blocked %s %s", d.Symbol, d.Action))
				record.Decisions = append(record.Decisions, store.DecisionAction{
					Action:            d.Action,
					Symbol:            d.Symbol,
					Leverage:          d.Leverage,
					StopLoss:          d.StopLoss,
					TakeProfit:        d.TakeProfit,
					Confidence:        d.Confidence,
					Reasoning:         d.Reasoning,
					BlockedReasonCode: d.BlockedReasonCode,
					Trigger:           decisionTriggerForStore(d.Trigger),
					Timestamp:         time.Now().UTC(),
					Success:           false,
					Error:             "degraded trading context: open orders disabled",
				})
				continue
			}
			filtered = append(filtered, d)
		}
		sortedDecisions = filtered
	}

	// Execute decisions and record results
	for _, d := range sortedDecisions {
		// Check if trader is stopped before each decision (allow immediate stop during execution)
		at.isRunningMutex.RLock()
		running = at.isRunning
		at.isRunningMutex.RUnlock()
		if !running {
			at.logInfof("⏹ Trader stopped during decision execution, aborting remaining decisions")
			break
		}

		actionRecord := store.DecisionAction{
			Action:            d.Action,
			Symbol:            d.Symbol,
			Quantity:          0,
			Leverage:          d.Leverage,
			Price:             0,
			StopLoss:          d.StopLoss,
			TakeProfit:        d.TakeProfit,
			Confidence:        d.Confidence,
			Reasoning:         d.Reasoning,
			BlockedReasonCode: d.BlockedReasonCode,
			Trigger:           decisionTriggerForStore(d.Trigger),
			Timestamp:         time.Now().UTC(),
			Success:           false,
		}

		if err := at.validateHunterV7ExecutionGuard(ctx, &d); err != nil {
			at.logErrorf("❌ Hunter v7 execution guard blocked (%s %s): %v", d.Symbol, d.Action, err)
			actionRecord.Error = err.Error()
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("❌ %s %s blocked: %v", d.Symbol, d.Action, err))
		} else if err := at.executeDecisionWithRecord(ctx, &d, &actionRecord); err != nil {
			at.logErrorf("❌ Failed to execute decision (%s %s): %v", d.Symbol, d.Action, err)
			actionRecord.Error = err.Error()
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("❌ %s %s failed: %v", d.Symbol, d.Action, err))
		} else {
			actionRecord.Success = true
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ %s %s succeeded", d.Symbol, d.Action))
			if logLine := effectiveOpenContractLog(actionRecord); logLine != "" {
				record.ExecutionLog = append(record.ExecutionLog, logLine)
			}
			// Brief delay after successful execution
			time.Sleep(1 * time.Second)
		}

		record.Decisions = append(record.Decisions, actionRecord)
	}

	// 9. Save decision record
	if err := at.saveDecision(record); err != nil {
		at.logWarnf("⚠ Failed to save decision record: %v", err)
	}

	return nil
}

func effectiveOpenContractLog(action store.DecisionAction) string {
	if action.Action != "open_long" && action.Action != "open_short" {
		return ""
	}
	if action.EffectivePositionSizeUSD <= 0 && action.EffectiveStopLoss <= 0 && action.EffectiveTakeProfit <= 0 {
		return ""
	}
	finalRR := action.FinalRR
	if finalRR <= 0 {
		finalRR = action.RRAfterBackendRepair
	}
	return fmt.Sprintf("effective_contract %s %s notional=%.2f qty=%.8f entry=%.8f sl=%.8f tp=%.8f tp_capped=%t position_reduced=%t risk_at_sl=%.2f final_rr=%.2f",
		action.Symbol,
		action.Action,
		action.EffectivePositionSizeUSD,
		action.Quantity,
		action.Price,
		action.EffectiveStopLoss,
		action.EffectiveTakeProfit,
		action.TPWasCapped,
		action.PositionWasReduced,
		action.RiskAtStopUSD,
		finalRR,
	)
}

func (at *AutoTrader) shouldSkipHunterV7NoExecutable(ctx *kernel.Context, record *store.DecisionRecord) bool {
	if ctx == nil || record == nil || !at.isHunterV7Strategy() || len(ctx.Positions) > 0 || len(ctx.CandidateCoins) == 0 {
		return false
	}

	execCount, reviewableCount, watchCount, rejectedCount := 0, 0, 0, 0
	for _, coin := range ctx.CandidateCoins {
		switch coin.V7ExecutionTier {
		case "EXECUTABLE":
			execCount++
		case "REVIEWABLE":
			reviewableCount++
		case "REJECTED":
			rejectedCount++
		default:
			watchCount++
		}
	}
	if execCount+reviewableCount > 0 {
		return false
	}

	topSummary := hunterV7TopCandidateSummary(ctx.CandidateCoins)
	reason := fmt.Sprintf("no_open_review_candidates watch=%d rejected=%d", watchCount, rejectedCount)
	if topSummary != "" {
		reason += " top=" + topSummary
	}
	at.logInfof("ℹ️ Hunter v7 has no EXECUTABLE/REVIEWABLE candidates (%s), skipping AI this cycle", reason)
	record.Success = true
	record.DecisionJSON = fmt.Sprintf(`[
  {
    "symbol": "ALL",
    "action": "wait",
    "reasoning": "%s"
  }
]`, reason)
	record.Decisions = []store.DecisionAction{
		{
			Action:    "wait",
			Symbol:    "ALL",
			Reasoning: reason,
			Timestamp: time.Now(),
			Success:   true,
		},
	}
	record.ExecutionLog = append(record.ExecutionLog, "Hunter v7 no EXECUTABLE/REVIEWABLE candidates; AI skipped")
	if topSummary != "" {
		record.ExecutionLog = append(record.ExecutionLog, "Hunter v7 top blocked candidate: "+topSummary)
	}
	record.AccountState = store.AccountSnapshot{
		TotalBalance:          ctx.Account.TotalEquity,
		AvailableBalance:      ctx.Account.AvailableBalance,
		TotalUnrealizedProfit: ctx.Account.UnrealizedPnL,
		PositionCount:         ctx.Account.PositionCount,
		MarginUsedPct:         ctx.Account.MarginUsedPct,
		InitialBalance:        at.initialBalance,
	}
	return true
}

func hunterV7TopCandidateSummary(coins []kernel.CandidateCoin) string {
	var top *kernel.CandidateCoin
	for i := range coins {
		if top == nil || coins[i].V7AIPriority > top.V7AIPriority {
			top = &coins[i]
		}
	}
	if top == nil || top.Symbol == "" {
		return ""
	}
	rrText := "n/a"
	if top.V7ConfirmSummary != nil && top.V7ConfirmSummary.RR > 0 {
		rrText = fmt.Sprintf("%.2f", top.V7ConfirmSummary.RR)
	}
	return fmt.Sprintf("%s/%s setup=%s shape=%s entry_signal=%s quality=%s priority=%.1f liq=%.0f rr=%s reason=%s",
		top.Symbol, top.Direction, top.V7SetupType, top.V7MarketShape, top.V7EntrySignal,
		top.V7ExecutionQuality, top.V7AIPriority, top.V7LiquidityScore, rrText, top.V7TierReason)
}

func (at *AutoTrader) degradedContextCacheAgeLimit() time.Duration {
	limit := at.config.ScanInterval * 2
	if limit <= 0 {
		limit = 2 * time.Minute
	}
	if limit > maxDegradedContextCacheAge {
		return maxDegradedContextCacheAge
	}
	return limit
}

func cloneInterfaceMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func clonePositionMaps(src []map[string]interface{}) []map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make([]map[string]interface{}, len(src))
	for i, item := range src {
		dst[i] = cloneInterfaceMap(item)
	}
	return dst
}

func (at *AutoTrader) cacheContextBalance(balance map[string]interface{}) {
	at.contextCacheMutex.Lock()
	defer at.contextCacheMutex.Unlock()
	at.lastContextBalance = cloneInterfaceMap(balance)
	at.lastContextBalanceAt = time.Now()
}

func (at *AutoTrader) cacheContextPositions(positions []map[string]interface{}) {
	at.contextCacheMutex.Lock()
	defer at.contextCacheMutex.Unlock()
	at.lastContextPositions = clonePositionMaps(positions)
	at.lastContextPositionsAt = time.Now()
}

func (at *AutoTrader) cachedContextBalance(maxAge time.Duration) (map[string]interface{}, time.Duration, bool) {
	at.contextCacheMutex.RLock()
	defer at.contextCacheMutex.RUnlock()
	if at.lastContextBalance == nil || at.lastContextBalanceAt.IsZero() {
		return nil, 0, false
	}
	age := time.Since(at.lastContextBalanceAt)
	if maxAge > 0 && age > maxAge {
		return nil, age, false
	}
	return cloneInterfaceMap(at.lastContextBalance), age, true
}

func (at *AutoTrader) cachedContextPositions(maxAge time.Duration) ([]map[string]interface{}, time.Duration, bool) {
	at.contextCacheMutex.RLock()
	defer at.contextCacheMutex.RUnlock()
	if at.lastContextPositions == nil || at.lastContextPositionsAt.IsZero() {
		return nil, 0, false
	}
	age := time.Since(at.lastContextPositionsAt)
	if maxAge > 0 && age > maxAge {
		return nil, age, false
	}
	return clonePositionMaps(at.lastContextPositions), age, true
}

func (at *AutoTrader) invalidateTraderPositionCacheForDecision() {
	type positionCacheInvalidator interface {
		InvalidatePositionCache()
	}
	if invalidator, ok := at.trader.(positionCacheInvalidator); ok {
		invalidator.InvalidatePositionCache()
	}
}

// buildTradingContext builds trading context
func (at *AutoTrader) buildTradingContext() (*kernel.Context, error) {
	var degradationReasons []string
	accountDataStale := false
	positionDataStale := false
	cacheAgeLimit := at.degradedContextCacheAgeLimit()

	// 1. Get account information
	balance, err := at.trader.GetBalance()
	if err != nil {
		cached, age, ok := at.cachedContextBalance(cacheAgeLimit)
		if !ok {
			return nil, fmt.Errorf("failed to get account balance: %w", err)
		}
		balance = cached
		accountDataStale = true
		degradationReasons = append(degradationReasons, fmt.Sprintf("account balance API failed (%v), using %.0fs cached balance", err, age.Seconds()))
	} else {
		at.cacheContextBalance(balance)
	}

	// Get account fields
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0
	totalEquity := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Use totalEquity directly if provided by trader (more accurate)
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		totalEquity = eq
	} else {
		// Fallback: Total Equity = Wallet balance + Unrealized profit
		totalEquity = totalWalletBalance + totalUnrealizedProfit
	}

	// 2. Get position information
	at.invalidateTraderPositionCacheForDecision()
	positions, err := at.trader.GetPositions()
	if err != nil {
		cached, age, ok := at.cachedContextPositions(cacheAgeLimit)
		if !ok {
			return nil, fmt.Errorf("failed to get positions: %w", err)
		}
		positions = cached
		positionDataStale = true
		degradationReasons = append(degradationReasons, fmt.Sprintf("positions API failed (%v), using %.0fs cached positions", err, age.Seconds()))
	} else {
		at.cacheContextPositions(positions)
	}

	var positionInfos []kernel.PositionInfo
	totalMarginUsed := 0.0
	plannedRiskByPosition := at.latestOpenDecisionRiskByPosition(50)

	// Current position key set (for cleaning up closed position records)
	currentPositionKeys := make(map[string]bool)

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // Short position quantity is negative, convert to positive
		}

		// Skip closed positions (quantity = 0), prevent "ghost positions" from being passed to AI
		if quantity == 0 {
			continue
		}

		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		// Calculate margin used (estimated)
		leverage := 10 // Default value, should actually be fetched from position info
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed

		// Calculate P&L percentage (based on margin, considering leverage)
		pnlPct := calculatePnLPercentage(unrealizedPnl, marginUsed)

		// Get position open time from exchange (preferred) or fallback to local tracking
		posKey := symbol + "_" + side
		currentPositionKeys[posKey] = true

		var updateTime int64
		// Priority 1: Get from database (trader_positions table) - most accurate
		if at.store != nil {
			if dbPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, symbol, side); err == nil && dbPos != nil {
				if dbPos.EntryTime > 0 {
					updateTime = dbPos.EntryTime
				}
			}
		}
		// Priority 2: Get from exchange API (Bybit: createdTime, OKX: createdTime)
		if updateTime == 0 {
			if createdTime, ok := pos["createdTime"].(int64); ok && createdTime > 0 {
				updateTime = createdTime
			}
		}
		// Priority 3: Fallback to local tracking
		if updateTime == 0 {
			if _, exists := at.positionFirstSeenTime[posKey]; !exists {
				at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()
			}
			updateTime = at.positionFirstSeenTime[posKey]
		}

		// Get peak profit rate for this position
		openedAt := time.Time{}
		if updateTime > 0 {
			openedAt = time.UnixMilli(updateTime)
		}
		at.ensurePeakPnLCacheInitialized(symbol, side, pnlPct, openedAt)
		at.peakPnLCacheMutex.RLock()
		peakPnlPct := at.peakPnLCache[posKey]
		at.peakPnLCacheMutex.RUnlock()

		plannedRisk := plannedRiskByPosition[positionRiskKey(symbol, side)]
		positionInfos = append(positionInfos, kernel.PositionInfo{
			Symbol:           symbol,
			Side:             side,
			EntryPrice:       entryPrice,
			MarkPrice:        markPrice,
			Quantity:         quantity,
			Leverage:         leverage,
			UnrealizedPnL:    unrealizedPnl,
			UnrealizedPnLPct: pnlPct,
			PeakPnLPct:       peakPnlPct,
			LiquidationPrice: liquidationPrice,
			MarginUsed:       marginUsed,
			StopLoss:         plannedRisk.stopLoss,
			TakeProfit:       plannedRisk.takeProfit,
			UpdateTime:       updateTime,
		})
	}

	// Clean up closed position records
	for key := range at.positionFirstSeenTime {
		if !currentPositionKeys[key] {
			delete(at.positionFirstSeenTime, key)
			parts := strings.Split(key, "_")
			if len(parts) == 2 {
				at.ClearPeakPnLCache(parts[0], parts[1])
			}
		}
	}

	// 3. Use strategy engine to get candidate coins (must have strategy engine)
	var candidateCoins []kernel.CandidateCoin
	if at.strategyEngine == nil {
		at.logWarnf("⚠️ No strategy engine configured, skipping candidate coins")
	} else {
		// Use new SnapshotEngine if available, otherwise legacy
		coins, err := at.strategyEngine.GetCandidateCoinsWithSnapshot()
		if err != nil {
			at.logWarnf("⚠️ Failed to get candidate coins: %v", err)
			return nil, fmt.Errorf("failed to get candidate coins: %w", err)
		} else {
			candidateCoins = coins
			logger.Infof("📋 [%s] Strategy engine fetched candidate coins: %d", at.name, len(candidateCoins))

			// Anti-repeat filter: remove coins that have been "wait"ed for 3+ consecutive cycles
			if at.store != nil && len(candidateCoins) > 0 {
				staleWaits := at.store.Decision().GetRecentWaitSymbols(at.id, 3, 3)
				if len(staleWaits) > 0 {
					filtered := make([]kernel.CandidateCoin, 0, len(candidateCoins))
					for _, coin := range candidateCoins {
						if count, stale := staleWaits[coin.Symbol]; stale {
							if !shouldSkipCandidateForRepeatedWait(coin, count) {
								filtered = append(filtered, coin)
								continue
							}
							logger.Infof("🔄 [%s] Anti-repeat: skipping %s (waited %d recent cycles)", at.name, coin.Symbol, count)
						} else {
							filtered = append(filtered, coin)
						}
					}
					if len(filtered) > 0 {
						logger.Infof("🔄 [%s] Anti-repeat filtered: %d → %d candidates", at.name, len(candidateCoins), len(filtered))
						candidateCoins = filtered
					} else {
						logger.Infof("🔄 [%s] Anti-repeat: all candidates were stale, keeping originals", at.name)
					}
				}
			}

			// Failed-open cooldown: if an open order was rejected recently, do not
			// immediately retry the same symbol while the signal may still be stale.
			if at.store != nil && len(candidateCoins) > 0 {
				failedOpens := at.store.Decision().GetRecentFailedOpenSymbols(at.id, 3)
				if len(failedOpens) > 0 {
					filtered := make([]kernel.CandidateCoin, 0, len(candidateCoins))
					for _, coin := range candidateCoins {
						if reason, failed := failedOpens[coin.Symbol]; failed {
							if shouldIgnoreStaleFailedOpenCooldown(reason) {
								filtered = append(filtered, coin)
								continue
							}
							logger.Infof("🧊 [%s] Failed-open cooldown: skipping %s (recent open rejected: %s)", at.name, coin.Symbol, reason)
						} else {
							filtered = append(filtered, coin)
						}
					}
					logger.Infof("🧊 [%s] Failed-open cooldown filtered: %d → %d candidates", at.name, len(candidateCoins), len(filtered))
					candidateCoins = filtered
				}
			}
		}
	}

	// 4. Calculate total P&L
	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	// 5. Get leverage from strategy config
	strategyConfig := at.strategyEngine.GetConfig()
	btcEthLeverage := strategyConfig.RiskControl.BTCETHMaxLeverage
	altcoinLeverage := strategyConfig.RiskControl.AltcoinMaxLeverage
	logger.Infof("📋 [%s] Strategy leverage config: BTC/ETH=%dx, Altcoin=%dx", at.name, btcEthLeverage, altcoinLeverage)

	// 6. Build context
	ctx := &kernel.Context{
		CurrentTime:        time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		RuntimeMinutes:     int(time.Since(at.startTime).Minutes()),
		CallCount:          at.callCount,
		IsDegraded:         len(degradationReasons) > 0,
		DegradationReasons: degradationReasons,
		AccountDataStale:   accountDataStale,
		PositionDataStale:  positionDataStale,
		DisableOpenOrders:  len(degradationReasons) > 0,
		BTCETHLeverage:     btcEthLeverage,
		AltcoinLeverage:    altcoinLeverage,
		Account: kernel.AccountInfo{
			TotalEquity:      totalEquity,
			AvailableBalance: availableBalance,
			UnrealizedPnL:    totalUnrealizedProfit,
			TotalPnL:         totalPnL,
			TotalPnLPct:      totalPnLPct,
			MarginUsed:       totalMarginUsed,
			MarginUsedPct:    marginUsedPct,
			PositionCount:    len(positionInfos),
		},
		Positions:      positionInfos,
		CandidateCoins: candidateCoins,
	}

	// 7. Add recent closed trades (if store is available)
	if at.store != nil {
		// Get recent 10 closed trades for AI context
		recentTrades, err := at.store.Position().GetRecentTrades(at.id, 10)
		if err != nil {
			at.logWarnf("⚠️ Failed to get recent trades: %v", err)
		} else {
			logger.Infof("📊 [%s] Found %d recent closed trades for AI context", at.name, len(recentTrades))
			for _, trade := range recentTrades {
				// Convert Unix timestamps to formatted strings for AI readability
				entryTimeStr := ""
				if trade.EntryTime > 0 {
					entryTimeStr = time.Unix(trade.EntryTime, 0).UTC().Format("01-02 15:04 UTC")
				}
				exitTimeStr := ""
				if trade.ExitTime > 0 {
					exitTimeStr = time.Unix(trade.ExitTime, 0).UTC().Format("01-02 15:04 UTC")
				}

				ctx.RecentOrders = append(ctx.RecentOrders, kernel.RecentOrder{
					Symbol:       trade.Symbol,
					Side:         trade.Side,
					EntryPrice:   trade.EntryPrice,
					ExitPrice:    trade.ExitPrice,
					RealizedPnL:  trade.RealizedPnL,
					PnLPct:       trade.PnLPct,
					EntryTime:    entryTimeStr,
					ExitTime:     exitTimeStr,
					HoldDuration: trade.HoldDuration,
				})
			}
			if at.isHunterV7Strategy() && len(ctx.CandidateCoins) > 0 {
				filtered, blocked := at.filterHunterV7RecentLossCooldown(ctx.CandidateCoins, recentTrades)
				if blocked > 0 {
					logger.Infof("🧊 [%s] Hunter v7 recent-loss cooldown filtered: %d → %d candidates", at.name, len(ctx.CandidateCoins), len(filtered))
					ctx.CandidateCoins = filtered
				}
			}
		}
		// Get trading statistics for AI context
		stats, err := at.store.Position().GetFullStats(at.id)
		if err != nil {
			at.logWarnf("⚠️ Failed to get trading stats: %v", err)
		} else if stats == nil {
			at.logWarnf("⚠️ GetFullStats returned nil")
		} else if stats.TotalTrades == 0 {
			at.logWarnf("⚠️ GetFullStats returned 0 trades")
		} else {
			ctx.TradingStats = &kernel.TradingStats{
				TotalTrades:    stats.TotalTrades,
				WinRate:        stats.WinRate,
				ProfitFactor:   stats.ProfitFactor,
				SharpeRatio:    stats.SharpeRatio,
				TotalPnL:       stats.TotalPnL,
				AvgWin:         stats.AvgWin,
				AvgLoss:        stats.AvgLoss,
				MaxDrawdownPct: stats.MaxDrawdownPct,
			}
			logger.Infof("📈 [%s] Trading stats: %d trades, %.1f%% win rate, PF=%.2f, Sharpe=%.2f, DD=%.1f%%",
				at.name, stats.TotalTrades, stats.WinRate, stats.ProfitFactor, stats.SharpeRatio, stats.MaxDrawdownPct)
		}
	} else {
		at.logWarnf("⚠️ Store is nil, cannot get recent trades")
	}

	// 8. Get quantitative data (if enabled in strategy config)
	if strategyConfig.Indicators.EnableQuantData {
		// Collect symbols to query (candidate coins + position coins)
		symbolsToQuery := make(map[string]bool)
		for _, coin := range candidateCoins {
			symbolsToQuery[coin.Symbol] = true
		}
		for _, pos := range positionInfos {
			symbolsToQuery[pos.Symbol] = true
		}

		symbols := make([]string, 0, len(symbolsToQuery))
		for sym := range symbolsToQuery {
			symbols = append(symbols, sym)
		}

		logger.Infof("📊 [%s] Fetching quantitative data for %d symbols...", at.name, len(symbols))
		ctx.QuantDataMap = at.strategyEngine.FetchQuantDataBatch(symbols)
		logger.Infof("📊 [%s] Successfully fetched quantitative data for %d symbols", at.name, len(ctx.QuantDataMap))
	}

	// 9. Get OI ranking data (market-wide position changes)
	if strategyConfig.Indicators.EnableOIRanking {
		logger.Infof("📊 [%s] Fetching OI ranking data...", at.name)
		ctx.OIRankingData = at.strategyEngine.FetchOIRankingData()
		if ctx.OIRankingData != nil {
			logger.Infof("📊 [%s] OI ranking data ready: %d top, %d low positions",
				at.name, len(ctx.OIRankingData.TopPositions), len(ctx.OIRankingData.LowPositions))
		}
	}

	// 10. Get NetFlow ranking data (market-wide fund flow)
	if strategyConfig.Indicators.EnableNetFlowRanking {
		logger.Infof("💰 [%s] Fetching NetFlow ranking data...", at.name)
		ctx.NetFlowRankingData = at.strategyEngine.FetchNetFlowRankingData()
		if ctx.NetFlowRankingData != nil {
			logger.Infof("💰 [%s] NetFlow ranking data ready: inst_in=%d, inst_out=%d",
				at.name, len(ctx.NetFlowRankingData.InstitutionFutureTop), len(ctx.NetFlowRankingData.InstitutionFutureLow))
		}
	}

	// 11. Get Price ranking data (market-wide gainers/losers)
	if strategyConfig.Indicators.EnablePriceRanking {
		logger.Infof("📈 [%s] Fetching Price ranking data...", at.name)
		ctx.PriceRankingData = at.strategyEngine.FetchPriceRankingData()
		if ctx.PriceRankingData != nil {
			logger.Infof("📈 [%s] Price ranking data ready for %d durations",
				at.name, len(ctx.PriceRankingData.Durations))
		}
	}

	return ctx, nil
}

type plannedPositionRisk struct {
	stopLoss   float64
	takeProfit float64
}

func (at *AutoTrader) latestOpenDecisionRiskByPosition(limit int) map[string]plannedPositionRisk {
	risks := make(map[string]plannedPositionRisk)
	if at == nil || at.store == nil {
		return risks
	}
	if limit <= 0 {
		limit = 50
	}

	records, err := at.store.Decision().GetLatestRecords(at.id, limit)
	if err != nil {
		at.logWarnf("⚠️ Failed to load recent open decision SL/TP: %v", err)
		return risks
	}

	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if record == nil || !record.Success {
			continue
		}
		for _, decision := range record.Decisions {
			side, ok := openDecisionSide(decision.Action)
			if !ok || !decision.Success || decision.Symbol == "" {
				continue
			}
			if decision.StopLoss <= 0 && decision.TakeProfit <= 0 {
				continue
			}
			key := positionRiskKey(decision.Symbol, side)
			if _, exists := risks[key]; exists {
				continue
			}
			risks[key] = plannedPositionRisk{
				stopLoss:   decision.StopLoss,
				takeProfit: decision.TakeProfit,
			}
		}
	}

	return risks
}

func openDecisionSide(action string) (string, bool) {
	switch strings.ToLower(action) {
	case "open_long":
		return "LONG", true
	case "open_short":
		return "SHORT", true
	default:
		return "", false
	}
}

func positionRiskKey(symbol, side string) string {
	return strings.ToUpper(symbol) + "_" + strings.ToUpper(side)
}

// sortDecisionsByPriority sorts decisions: close positions first, then open positions, finally hold/wait
// This avoids position stacking overflow when changing positions
func sortDecisionsByPriority(decisions []kernel.Decision) []kernel.Decision {
	if len(decisions) <= 1 {
		return decisions
	}

	// Define priority
	getActionPriority := func(action string) int {
		switch action {
		case "close_long", "close_short":
			return 1 // Highest priority: close positions first
		case "open_long", "open_short":
			return 2 // Second priority: open positions later
		case "hold", "wait":
			return 3 // Lowest priority: wait
		default:
			return 999 // Unknown actions at the end
		}
	}

	// Copy decision list
	sorted := make([]kernel.Decision, len(decisions))
	copy(sorted, decisions)

	// Sort by priority
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if getActionPriority(sorted[i].Action) > getActionPriority(sorted[j].Action) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

func shouldSkipCandidateForRepeatedWait(coin kernel.CandidateCoin, waitCount int) bool {
	if waitCount <= 0 {
		return false
	}
	if coin.V7SetupType == "" {
		return true
	}
	// Candidates from the strategy engine always carry the tier verdict
	// classified with the engine-configured geometry; an empty tier means the
	// coin never went through construction and falls through to skip.
	tier := coin.V7ExecutionTier
	if tier == "EXECUTABLE" && coin.V7ExecutionQuality == "ready" && coin.V7AIPriority >= 55 {
		return false
	}
	if tier == "REVIEWABLE" && coin.V7EntrySignal == "entry_trigger_near" && coin.V7AIPriority >= 45 && coin.V7RiskScore < 55 {
		return false
	}
	if tier == "REVIEWABLE" && coin.V7AIPriority >= 58 && coin.V7TimingScore >= 55 {
		return false
	}
	return true
}

func decisionTriggerForStore(trigger *kernel.DecisionTrigger) *store.DecisionTrigger {
	if trigger == nil {
		return nil
	}
	return &store.DecisionTrigger{
		TriggerPrice:      trigger.TriggerPrice,
		RequiredClose:     trigger.RequiredClose,
		ExpiresInBars:     trigger.ExpiresInBars,
		ActionIfTriggered: trigger.ActionIfTriggered,
	}
}

func (at *AutoTrader) filterHunterV7RecentLossCooldown(candidates []kernel.CandidateCoin, recentTrades []store.RecentTrade) ([]kernel.CandidateCoin, int) {
	if len(candidates) == 0 || len(recentTrades) == 0 {
		return candidates, 0
	}
	lossesByKey := make(map[string]int)
	now := time.Now().Unix()
	const cooldownWindowSec int64 = 60 * 60
	for _, trade := range recentTrades {
		if trade.Symbol == "" || trade.Side == "" || trade.ExitTime <= 0 {
			continue
		}
		if now-trade.ExitTime > cooldownWindowSec {
			continue
		}
		if trade.PnLPct > -5 {
			continue
		}
		key := market.Normalize(trade.Symbol) + "|" + strings.ToUpper(trade.Side)
		lossesByKey[key]++
	}
	if len(lossesByKey) == 0 {
		return candidates, 0
	}
	filtered := make([]kernel.CandidateCoin, 0, len(candidates))
	blocked := 0
	for _, coin := range candidates {
		key := market.Normalize(coin.Symbol) + "|" + strings.ToUpper(coin.Direction)
		if lossesByKey[key] >= 2 {
			blocked++
			logger.Infof("🧊 [%s] Hunter v7 same-symbol loss cooldown: skipping %s %s after %d recent losses",
				at.name, coin.Symbol, coin.Direction, lossesByKey[key])
			continue
		}
		filtered = append(filtered, coin)
	}
	return filtered, blocked
}

func shouldIgnoreStaleFailedOpenCooldown(reason string) bool {
	reason = strings.ToLower(reason)
	return strings.Contains(reason, "price not near entry-zone upper/retest area")
}

// checkClaw402Balance checks USDC balance and logs warnings if low
func (at *AutoTrader) checkClaw402Balance() {
	scanMinutes := int(at.config.ScanInterval.Minutes())
	if scanMinutes <= 0 {
		scanMinutes = 3
	}
	dailyCost, _ := store.EstimateRunway(1.0, at.config.CustomModelName, scanMinutes)
	logger.Infof("💰 [%s] Estimated daily AI cost: ~$%.2f (model: %s, interval: %dm)",
		at.name, dailyCost, at.config.CustomModelName, scanMinutes)

	if at.claw402WalletAddr != "" {
		balance, err := wallet.QueryUSDCBalance(at.claw402WalletAddr)
		if err != nil {
			at.logWarnf("⚠️ Failed to query USDC balance: %v", err)
			return
		}

		if balance < 1.0 {
			at.logWarnf("⚠️ Low USDC balance: $%.2f — AI may stop soon!", balance)
		}
		if balance <= 0 {
			at.logErrorf("🚨 USDC balance is ZERO — AI calls will fail!")
		}

		runway := float64(0)
		if dailyCost > 0 {
			runway = balance / dailyCost
		}
		logger.Infof("💰 [%s] USDC Balance: $%.2f | Daily AI cost: ~$%.2f | Runway: ~%.1f days",
			at.name, balance, dailyCost, runway)
	}
}
