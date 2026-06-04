package kernel

import (
	"encoding/json"
	"fmt"
	"nofx/market"
	"nofx/provider/local"
	"nofx/provider/nofxos"
	"nofx/store"
	"strings"
	"time"
)

// ============================================================================
// Prompt Building - System Prompt
// ============================================================================

// BuildSystemPrompt builds System Prompt according to strategy configuration
func (e *StrategyEngine) BuildSystemPrompt(accountEquity float64, variant string) string {
	var sb strings.Builder
	riskControl := e.config.RiskControl
	promptSections := e.config.PromptSections

	// 0. Data Dictionary & Schema (ensure AI understands all fields)
	lang := e.GetLanguage()
	schemaPrompt := GetSchemaPrompt(lang)
	sb.WriteString(schemaPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("---\n\n")

	// 1. Role definition (editable)
	if promptSections.RoleDefinition != "" {
		sb.WriteString(promptSections.RoleDefinition)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("# You are a professional cryptocurrency trading AI\n\n")
		sb.WriteString("Your task is to make trading decisions based on provided market data.\n\n")
	}

	// 2. Trading mode variant
	switch strings.ToLower(strings.TrimSpace(variant)) {
	case "aggressive":
		sb.WriteString("## Mode: Aggressive\n- Prioritize capturing trend breakouts, can build positions in batches when confidence ≥ 70\n- Allow higher positions, but must strictly set stop-loss and explain risk-reward ratio\n\n")
	case "conservative":
		sb.WriteString("## Mode: Conservative\n- Only open positions when multiple signals resonate\n- Prioritize cash preservation, must pause for multiple periods after consecutive losses\n\n")
	case "scalping":
		sb.WriteString("## Mode: Scalping\n- Focus on short-term momentum, smaller profit targets but require quick action\n- If price doesn't move as expected within two bars, immediately reduce position or stop-loss\n\n")
	}

	// 3. Hard constraints (risk control)
	btcEthPosValueRatio := riskControl.BTCETHMaxPositionValueRatio
	if btcEthPosValueRatio <= 0 {
		btcEthPosValueRatio = 5.0
	}
	altcoinPosValueRatio := riskControl.AltcoinMaxPositionValueRatio
	if altcoinPosValueRatio <= 0 {
		altcoinPosValueRatio = 1.0
	}

	sb.WriteString("# Hard Constraints (Risk Control)\n\n")
	sb.WriteString("## CODE ENFORCED (Backend validation, cannot be bypassed):\n")
	sb.WriteString(fmt.Sprintf("- Max Positions: %d coins simultaneously\n", riskControl.MaxPositions))
	sb.WriteString(fmt.Sprintf("- Position Value Limit (Altcoins): max %.0f USDT (= equity %.0f × %.1fx)\n",
		accountEquity*altcoinPosValueRatio, accountEquity, altcoinPosValueRatio))
	sb.WriteString(fmt.Sprintf("- Position Value Limit (BTC/ETH): max %.0f USDT (= equity %.0f × %.1fx)\n",
		accountEquity*btcEthPosValueRatio, accountEquity, btcEthPosValueRatio))
	sb.WriteString(fmt.Sprintf("- Max Margin Usage: ≤%.0f%%\n", riskControl.MaxMarginUsage*100))
	sb.WriteString(fmt.Sprintf("- Min Position Size: ≥%.0f USDT\n\n", riskControl.MinPositionSize))

	sb.WriteString("## AI GUIDED (Recommended, you should follow):\n")
	sb.WriteString(fmt.Sprintf("- Trading Leverage: Altcoins max %dx | BTC/ETH max %dx\n",
		riskControl.AltcoinMaxLeverage, riskControl.BTCETHMaxLeverage))
	sb.WriteString(fmt.Sprintf("- Risk-Reward Ratio: ≥1:%.1f (take_profit / stop_loss)\n", riskControl.MinRiskRewardRatio))
	sb.WriteString(fmt.Sprintf("- Min Confidence: ≥%d to open position\n\n", riskControl.MinConfidence))

	// Position sizing guidance
	sb.WriteString("## Position Sizing Guidance\n")
	sb.WriteString("Calculate `position_size_usd` based on your confidence and the Position Value Limits above:\n")
	sb.WriteString("- High confidence (≥85): Use 80-100%% of max position value limit\n")
	sb.WriteString("- Medium confidence (70-84): Use 50-80%% of max position value limit\n")
	sb.WriteString("- Low confidence (60-69): Use 30-50%% of max position value limit\n")
	sb.WriteString(fmt.Sprintf("- Example: With equity %.0f and BTC/ETH ratio %.1fx, max is %.0f USDT\n",
		accountEquity, btcEthPosValueRatio, accountEquity*btcEthPosValueRatio))
	sb.WriteString("- **DO NOT** just use available_balance as position_size_usd. Use the Position Value Limits!\n\n")

	// 4. Trading frequency (editable)
	if promptSections.TradingFrequency != "" {
		sb.WriteString(promptSections.TradingFrequency)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("# ⏱️ Trading Frequency Awareness\n\n")
		sb.WriteString("- Excellent traders: 2-4 trades/day ≈ 0.1-0.2 trades/hour\n")
		sb.WriteString("- >2 trades/hour = Overtrading\n")
		sb.WriteString("- Single position hold time ≥ 30-60 minutes\n")
		sb.WriteString("If you find yourself trading every period → standards too low; if closing positions < 30 minutes → too impatient.\n\n")
	}

	// 5. Entry standards (editable)
	if promptSections.EntryStandards != "" {
		sb.WriteString(promptSections.EntryStandards)
		sb.WriteString("\n\nYou have the following indicator data:\n")
		e.writeAvailableIndicators(&sb)
		sb.WriteString(fmt.Sprintf("\n**Confidence ≥ %d** required to open positions.\n\n", riskControl.MinConfidence))
	} else {
		sb.WriteString("# 🎯 Entry Standards (Strict)\n\n")
		sb.WriteString("Only open positions when multiple signals resonate. You have:\n")
		e.writeAvailableIndicators(&sb)
		sb.WriteString(fmt.Sprintf("\nFeel free to use any effective analysis method, but **confidence ≥ %d** required to open positions; avoid low-quality behaviors such as single indicators, contradictory signals, sideways consolidation, reopening immediately after closing, etc.\n\n", riskControl.MinConfidence))
	}

	// 6. Decision process (editable)
	if promptSections.DecisionProcess != "" {
		sb.WriteString(promptSections.DecisionProcess)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("# 📋 Decision Process\n\n")
		sb.WriteString("1. Check positions → Should we take profit/stop-loss\n")
		sb.WriteString("2. Scan candidate coins + multi-timeframe → Are there strong signals\n")
		sb.WriteString("3. Write chain of thought first, then output structured JSON\n\n")
	}

	// 7. Output format
	sb.WriteString("# Output Format (Strictly Follow)\n\n")
	sb.WriteString("**Must use XML tags <reasoning> and <decision> to separate chain of thought and decision JSON, avoiding parsing errors**\n\n")
	sb.WriteString("## Format Requirements\n\n")
	sb.WriteString("<reasoning>\n")
	sb.WriteString("Your chain of thought analysis...\n")
	sb.WriteString("- Briefly analyze your thinking process \n")
	sb.WriteString("</reasoning>\n\n")
	sb.WriteString("<decision>\n")
	sb.WriteString("Step 2: JSON decision array\n\n")
	sb.WriteString("```json\n[\n")
	// Use the actual configured position value ratio for BTC/ETH in the example
	examplePositionSize := accountEquity * btcEthPosValueRatio
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"confidence\": 85},\n",
		riskControl.BTCETHMaxLeverage, examplePositionSize))
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\"}\n")
	sb.WriteString("]\n```\n")
	sb.WriteString("</decision>\n\n")
	sb.WriteString("## Field Description\n\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | hold | wait\n")
	sb.WriteString(fmt.Sprintf("- `confidence`: 0-100 (opening recommended ≥ %d)\n", riskControl.MinConfidence))
	sb.WriteString("- Required when opening: leverage, position_size_usd, stop_loss, take_profit, confidence\n")
	sb.WriteString("- **IMPORTANT**: All numeric values must be calculated numbers, NOT formulas/expressions (e.g., use `27.76` not `3000 * 0.01`)\n\n")

	// 8. Custom Prompt
	if e.config.CustomPrompt != "" {
		sb.WriteString("# 📌 Personalized Trading Strategy\n\n")
		sb.WriteString(e.config.CustomPrompt)
		sb.WriteString("\n\n")
		sb.WriteString("Note: The above personalized strategy is a supplement to the basic rules and cannot violate the basic risk control principles.\n")
	}

	return sb.String()
}

func (e *StrategyEngine) writeAvailableIndicators(sb *strings.Builder) {
	indicators := e.config.Indicators
	kline := indicators.Klines

	sb.WriteString(fmt.Sprintf("- %s price series", kline.PrimaryTimeframe))
	if kline.EnableMultiTimeframe {
		sb.WriteString(fmt.Sprintf(" + %s K-line series\n", kline.LongerTimeframe))
	} else {
		sb.WriteString("\n")
	}

	if indicators.EnableEMA {
		sb.WriteString("- EMA indicators")
		if len(indicators.EMAPeriods) > 0 {
			sb.WriteString(fmt.Sprintf(" (periods: %v)", indicators.EMAPeriods))
		}
		sb.WriteString("\n")
	}

	if indicators.EnableMACD {
		sb.WriteString("- MACD indicators\n")
	}

	if indicators.EnableRSI {
		sb.WriteString("- RSI indicators")
		if len(indicators.RSIPeriods) > 0 {
			sb.WriteString(fmt.Sprintf(" (periods: %v)", indicators.RSIPeriods))
		}
		sb.WriteString("\n")
	}

	if indicators.EnableATR {
		sb.WriteString("- ATR indicators")
		if len(indicators.ATRPeriods) > 0 {
			sb.WriteString(fmt.Sprintf(" (periods: %v)", indicators.ATRPeriods))
		}
		sb.WriteString("\n")
	}

	if indicators.EnableBOLL {
		sb.WriteString("- Bollinger Bands (BOLL) - Upper/Middle/Lower bands")
		if len(indicators.BOLLPeriods) > 0 {
			sb.WriteString(fmt.Sprintf(" (periods: %v)", indicators.BOLLPeriods))
		}
		sb.WriteString("\n")
	}

	if indicators.EnableVolume {
		sb.WriteString("- Volume data\n")
	}

	if indicators.EnableOI {
		sb.WriteString("- Open Interest (OI) data\n")
	}

	if indicators.EnableFundingRate {
		sb.WriteString("- Funding rate\n")
	}

	if len(e.config.CoinSource.StaticCoins) > 0 || e.config.CoinSource.UseAI500 || e.config.CoinSource.UseOITop {
		sb.WriteString("- AI500 / OI_Top filter tags (if available)\n")
	}

	if indicators.EnableQuantData {
		sb.WriteString("- Quantitative data (institutional/retail fund flow, position changes, multi-period price changes)\n")
	}
}

// ============================================================================
// Prompt Building - User Prompt
// ============================================================================

// BuildUserPrompt builds User Prompt based on strategy configuration
func (e *StrategyEngine) BuildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// System status
	sb.WriteString(fmt.Sprintf("Time: %s | Period: #%d | Runtime: %d minutes\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// BTC market
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("BTC: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// Market environment classification (ADX-based regime detection)
	if ctx.MarketEnv != nil {
		env := ctx.MarketEnv
		regimeLabel := "Transitional (balanced risk, standard position sizing)"
		switch env.Regime {
		case market.RegimeRanging:
			regimeLabel = "Mean-Reversion Dominated (favor Hunter/Sniff signals, avoid trend-chasing)"
		case market.RegimeTrending:
			regimeLabel = "Trend-Following Dominated (favor momentum/surge signals, avoid counter-trend entries)"
		}
		if e.GetLanguage() == LangChinese {
			switch env.Regime {
			case market.RegimeRanging:
				regimeLabel = "均值回归主导 (偏向Hunter/Sniff信号，避免追涨杀跌)"
			case market.RegimeTrending:
				regimeLabel = "趋势追踪主导 (偏向动量/追浪信号，避免逆势操作)"
			default:
				regimeLabel = "过渡期 (平衡风控，标准仓位)"
			}
		}
		sb.WriteString(fmt.Sprintf("## Market Environment: %s (ADX=%.1f, +DI=%.1f, -DI=%.1f)\n\n",
			strings.ToUpper(string(env.Regime)), env.ADX, env.PlusDI, env.MinusDI))
		sb.WriteString(fmt.Sprintf("Guidance: %s\n\n", regimeLabel))
	}

	// Account information
	sb.WriteString(fmt.Sprintf("Account: Equity %.2f | Balance %.2f (%.1f%%) | PnL %+.2f%% | Margin %.1f%% | Positions %d\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	// Recently completed orders (placed before positions to ensure visibility)
	if len(ctx.RecentOrders) > 0 {
		sb.WriteString("## Recent Completed Trades\n")
		for i, order := range ctx.RecentOrders {
			resultStr := "Profit"
			if order.RealizedPnL < 0 {
				resultStr = "Loss"
			}
			sb.WriteString(fmt.Sprintf("%d. %s %s | Entry %.4f Exit %.4f | %s: %+.2f USDT (%+.2f%%) | %s→%s (%s)\n",
				i+1, order.Symbol, order.Side,
				order.EntryPrice, order.ExitPrice,
				resultStr, order.RealizedPnL, order.PnLPct,
				order.EntryTime, order.ExitTime, order.HoldDuration))
		}
		sb.WriteString("\n")
	}

	// Historical trading statistics (helps AI understand past performance)
	if ctx.TradingStats != nil && ctx.TradingStats.TotalTrades > 0 {
		// Get language from strategy config
		lang := e.GetLanguage()

		// Win/Loss ratio
		var winLossRatio float64
		if ctx.TradingStats.AvgLoss > 0 {
			winLossRatio = ctx.TradingStats.AvgWin / ctx.TradingStats.AvgLoss
		}

		if lang == LangChinese {
			sb.WriteString("## 历史交易统计\n")
			sb.WriteString(fmt.Sprintf("总交易: %d 笔 | 盈利因子: %.2f | 夏普比率: %.2f | 盈亏比: %.2f\n",
				ctx.TradingStats.TotalTrades,
				ctx.TradingStats.ProfitFactor,
				ctx.TradingStats.SharpeRatio,
				winLossRatio))
			sb.WriteString(fmt.Sprintf("总盈亏: %+.2f USDT | 平均盈利: +%.2f | 平均亏损: -%.2f | 最大回撤: %.1f%%\n",
				ctx.TradingStats.TotalPnL,
				ctx.TradingStats.AvgWin,
				ctx.TradingStats.AvgLoss,
				ctx.TradingStats.MaxDrawdownPct))

			// Performance hints based on profit factor, sharpe, and drawdown
			if ctx.TradingStats.ProfitFactor >= 1.5 && ctx.TradingStats.SharpeRatio >= 1 {
				sb.WriteString("表现: 良好 - 保持当前策略\n")
			} else if ctx.TradingStats.ProfitFactor < 1 {
				sb.WriteString("表现: 需改进 - 提高盈亏比，优化止盈止损\n")
			} else if ctx.TradingStats.MaxDrawdownPct > 30 {
				sb.WriteString("表现: 风险偏高 - 减少仓位，控制回撤\n")
			} else {
				sb.WriteString("表现: 正常 - 有优化空间\n")
			}
		} else {
			sb.WriteString("## Historical Trading Statistics\n")
			sb.WriteString(fmt.Sprintf("Total Trades: %d | Profit Factor: %.2f | Sharpe: %.2f | Win/Loss Ratio: %.2f\n",
				ctx.TradingStats.TotalTrades,
				ctx.TradingStats.ProfitFactor,
				ctx.TradingStats.SharpeRatio,
				winLossRatio))
			sb.WriteString(fmt.Sprintf("Total PnL: %+.2f USDT | Avg Win: +%.2f | Avg Loss: -%.2f | Max Drawdown: %.1f%%\n",
				ctx.TradingStats.TotalPnL,
				ctx.TradingStats.AvgWin,
				ctx.TradingStats.AvgLoss,
				ctx.TradingStats.MaxDrawdownPct))

			// Performance hints based on profit factor, sharpe, and drawdown
			if ctx.TradingStats.ProfitFactor >= 1.5 && ctx.TradingStats.SharpeRatio >= 1 {
				sb.WriteString("Performance: GOOD - maintain current strategy\n")
			} else if ctx.TradingStats.ProfitFactor < 1 {
				sb.WriteString("Performance: NEEDS IMPROVEMENT - improve win/loss ratio, optimize TP/SL\n")
			} else if ctx.TradingStats.MaxDrawdownPct > 30 {
				sb.WriteString("Performance: HIGH RISK - reduce position size, control drawdown\n")
			} else {
				sb.WriteString("Performance: NORMAL - room for optimization\n")
			}
		}
		sb.WriteString("\n")
	}

	// Position information
	if len(ctx.Positions) > 0 {
		sb.WriteString("## Current Positions\n")
		for i, pos := range ctx.Positions {
			sb.WriteString(e.formatPositionInfo(i+1, pos, ctx))
		}
	} else {
		sb.WriteString("Current Positions: None\n\n")
	}

	// Candidate coins (exclude coins already in positions to avoid duplicate data)
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		// Normalize symbol to handle both "ETH" and "ETHUSDT" formats
		normalizedSymbol := market.Normalize(pos.Symbol)
		positionSymbols[normalizedSymbol] = true
	}

	sb.WriteString(fmt.Sprintf("## Candidate Coins (%d coins)\n\n", len(ctx.MarketDataMap)))
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		// Skip if this coin is already a position (data already shown in positions section)
		normalizedCoinSymbol := market.Normalize(coin.Symbol)
		if positionSymbols[normalizedCoinSymbol] {
			continue
		}

		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		displayedCount++

		sourceTags := e.formatCoinSourceTag(coin.Sources)
		directionTag := ""
		if coin.Direction == "LONG" || coin.Direction == "SHORT" {
			directionTag = fmt.Sprintf(" [%s]", coin.Direction)
		}
		sb.WriteString(fmt.Sprintf("### %d. %s%s%s\n\n", displayedCount, coin.Symbol, directionTag, sourceTags))

		if coin.V7SetupType != "" {
			if signalJSON := e.formatHunterV7SignalJSON(coin); signalJSON != "" {
				sb.WriteString(fmt.Sprintf("hunter_v7_signal_json: %s\n", signalJSON))
			}
			sb.WriteString(fmt.Sprintf("Hunter v7 Signal Router: setup=%s | status=%s | regime=%s | entry_mode=%s | confidence=%s | risk=%s\n",
				coin.V7SetupType, coin.V7Status, coin.V7MarketRegime, coin.V7EntryMode, coin.V7Confidence, coin.V7RiskLevel))
			sb.WriteString(fmt.Sprintf("Hunter v7 Scores: ai_priority=%.1f | setup=%.1f | timing=%.1f | regime_fit=%.1f | liquidity=%.1f | risk=%.1f\n",
				coin.V7AIPriority, coin.V7SetupScore, coin.V7TimingScore, coin.V7RegimeFitScore, coin.V7LiquidityScore, coin.V7RiskScore))
			if len(coin.V7ReasonCodes) > 0 {
				sb.WriteString(fmt.Sprintf("Hunter v7 reasons: %s\n", strings.Join(coin.V7ReasonCodes, ", ")))
			}
			if len(coin.V7RiskTags) > 0 {
				sb.WriteString(fmt.Sprintf("Hunter v7 risk tags: %s\n", strings.Join(coin.V7RiskTags, ", ")))
			}
			if len(coin.V7RequiredConfirms) > 0 {
				sb.WriteString(fmt.Sprintf("Hunter v7 required confirmations before entry: %s\n", strings.Join(coin.V7RequiredConfirms, ", ")))
			}
			if coin.V7EntryZone.Lower > 0 || coin.V7EntryZone.Upper > 0 {
				sb.WriteString(fmt.Sprintf("Hunter v7 entry zone: %.6f ~ %.6f\n", coin.V7EntryZone.Lower, coin.V7EntryZone.Upper))
			}
			if coin.V7Invalidation.Price > 0 {
				sb.WriteString(fmt.Sprintf("Hunter v7 invalidation: %.6f (%s)\n", coin.V7Invalidation.Price, coin.V7Invalidation.Reason))
			}
			if len(coin.V7Targets) > 0 {
				var targets []string
				for _, target := range coin.V7Targets {
					targets = append(targets, fmt.Sprintf("%.6f(%s)", target.Price, target.Reason))
				}
				sb.WriteString(fmt.Sprintf("Hunter v7 targets: %s\n", strings.Join(targets, ", ")))
			}
			if coin.V7PriceContext != nil {
				pc := coin.V7PriceContext
				sb.WriteString(fmt.Sprintf("Hunter v7 price context: last=%.6f | 1h=%+.2f%% | 4h=%+.2f%% | 24h=%+.2f%% | ATR1h=%.6f | ATR4h=%.6f\n",
					pc.Last, pc.Change1h, pc.Change4h, pc.Change24h, pc.ATR1h, pc.ATR4h))
			}
			if coin.V7DerivativesCtx != nil {
				dc := coin.V7DerivativesCtx
				sb.WriteString(fmt.Sprintf("Hunter v7 derivatives: OI=%.0f | OI1h=%+.2f%% | OI4h=%+.2f%% | funding=%.5f | LSR %.3f→%.3f | taker15m=%.3f\n",
					dc.OIValue, dc.OIChange1h, dc.OIChange4h, dc.FundingRate, dc.LSROldest, dc.LSRNewest, dc.TakerBuy15m))
			}
			if coin.V7Status == "wait_confirm" || coin.V7Status == "conflict_watch" || coin.V7RiskLevel == "HIGH" || coin.V7RiskLevel == "EXTREME" {
				sb.WriteString("Hunter v7 execution rule: do not enter immediately; wait for the listed confirmations or directional trigger.\n")
			}
			sb.WriteString("\n")
		}

		// Show Hunter signal tags for AI context
		if len(coin.SignalTags) > 0 && coin.Direction != "" {
			sb.WriteString(fmt.Sprintf("Hunter signals: %s\n\n", strings.Join(coin.SignalTags, ", ")))
		}
		// RUSH alert: highlight extreme 15m taker pressure for AI attention
		for _, tag := range coin.SignalTags {
			if tag == "MICRO_BUY_RUSH" || tag == "MICRO_SELL_RUSH" {
				sb.WriteString(fmt.Sprintf("⚡ ALERT: %s detected on 15m — strong short-term directional pressure!\n\n", tag))
			}
		}
		// Capital confirmation tier (three-level gate)
		if coin.CapitalTier != "" && coin.CapitalTier != "Untiered" {
			tierLabel := coin.CapitalTier
			if e.GetLanguage() == LangChinese {
				switch coin.CapitalLevel {
				case 3:
					tierLabel = "Tier-S 高置信信号"
				case 2:
					tierLabel = "Tier-A 中等确认"
				case 1:
					tierLabel = "Tier-B 低置信 (仅补位)"
				}
			}
			sb.WriteString(fmt.Sprintf("Capital Tier: %s (Level %d)\n\n", tierLabel, coin.CapitalLevel))
		}
		// Show Hunter bidirectional scores for AI decision context
		if coin.LongScore > 0 || coin.ShortScore > 0 {
			selectedScore := coin.LongScore
			if coin.Direction == "SHORT" {
				selectedScore = coin.ShortScore
			}
			sb.WriteString(fmt.Sprintf("Hunter Score: LONG %.1f | SHORT %.1f | Selected: %s (%.1f)\n", coin.LongScore, coin.ShortScore, coin.Direction, selectedScore))
			// Warn if Hunter recommends opposite direction or score is low
			if coin.Direction == "SHORT" && coin.LongScore > coin.ShortScore {
				sb.WriteString(fmt.Sprintf("⚠️ Hunter WARNING: LONG score (%.1f) > SHORT score (%.1f). Consider switching direction or reducing confidence.\n", coin.LongScore, coin.ShortScore))
			} else if coin.Direction == "LONG" && coin.ShortScore > coin.LongScore {
				sb.WriteString(fmt.Sprintf("⚠️ Hunter WARNING: SHORT score (%.1f) > LONG score (%.1f). Consider switching direction or reducing confidence.\n", coin.ShortScore, coin.LongScore))
			}
			if selectedScore < 10 {
				sb.WriteString(fmt.Sprintf("🛑 Hunter REJECT: Score (%.1f) below 10. Do NOT trade.\n", selectedScore))
			} else if selectedScore < 20 {
				sb.WriteString(fmt.Sprintf("⚠️ Hunter LOW: Score (%.1f). Reduce position to 25%% max.\n", selectedScore))
			} else if selectedScore < 30 {
				sb.WriteString(fmt.Sprintf("⚡ Hunter MODERATE: Score (%.1f). Standard position.\n", selectedScore))
			} else {
				sb.WriteString(fmt.Sprintf("🔥 Hunter STRONG: Score (%.1f). Full conviction.\n", selectedScore))
			}
			// ADX direction validation: warn if Hunter direction conflicts with market trend
			if ctx.MarketEnv != nil && ctx.MarketEnv.ADX >= 20 {
				_, adxWarning := ValidateDirectionWithADX(coin.Direction, ctx.MarketEnv.ADX, ctx.MarketEnv.PlusDI, ctx.MarketEnv.MinusDI)
				if adxWarning != "" {
					sb.WriteString(fmt.Sprintf("⚠️ %s — ADX=%.1f (+DI=%.1f, -DI=%.1f). Reduce confidence or reconsider direction.\n",
						adxWarning, ctx.MarketEnv.ADX, ctx.MarketEnv.PlusDI, ctx.MarketEnv.MinusDI))
				}
			}
			sb.WriteString("\n")
		}
		sb.WriteString(e.formatMarketData(marketData))

		if ctx.QuantDataMap != nil {
			if quantData, hasQuant := ctx.QuantDataMap[coin.Symbol]; hasQuant {
				sb.WriteString(e.formatQuantData(quantData))
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Get language for market data formatting
	nofxosLang := nofxos.LangEnglish
	if e.GetLanguage() == LangChinese {
		nofxosLang = nofxos.LangChinese
	}

	// OI Ranking data (market-wide open interest changes)
	if ctx.OIRankingData != nil {
		sb.WriteString(nofxos.FormatOIRankingForAI(ctx.OIRankingData, nofxosLang))
	}

	// NetFlow Ranking data (market-wide fund flow)
	if ctx.NetFlowRankingData != nil {
		sb.WriteString(nofxos.FormatNetFlowRankingForAI(ctx.NetFlowRankingData, nofxosLang))
	}

	// Price Ranking data (market-wide gainers/losers)
	if ctx.PriceRankingData != nil {
		sb.WriteString(nofxos.FormatPriceRankingForAI(ctx.PriceRankingData, nofxosLang))
	}

	sb.WriteString("---\n\n")
	sb.WriteString("Now please analyze and output your decision (Chain of Thought + JSON)\n")

	return sb.String()
}

func (e *StrategyEngine) formatHunterV7SignalJSON(coin CandidateCoin) string {
	type v7SignalForAI struct {
		Symbol                string                      `json:"symbol"`
		Direction             string                      `json:"direction"`
		SetupType             string                      `json:"setup_type"`
		Status                string                      `json:"status"`
		MarketRegime          string                      `json:"market_regime"`
		EntryMode             string                      `json:"entry_mode"`
		Confidence            string                      `json:"confidence"`
		RiskLevel             string                      `json:"risk_level"`
		AIPriority            float64                     `json:"ai_priority"`
		SetupScore            float64                     `json:"setup_score"`
		TimingScore           float64                     `json:"timing_score"`
		RegimeFitScore        float64                     `json:"regime_fit_score"`
		LiquidityScore        float64                     `json:"liquidity_score"`
		RiskScore             float64                     `json:"risk_score"`
		ReasonCodes           []string                    `json:"reason_codes"`
		RiskTags              []string                    `json:"risk_tags"`
		RequiredConfirmations []string                    `json:"required_confirmations"`
		EntryZone             local.V7PriceZone           `json:"entry_zone"`
		Invalidation          local.V7InvalidationRule    `json:"invalidation"`
		Targets               []local.V7Target            `json:"targets"`
		PriceContext          *local.V7PriceContext       `json:"price_context,omitempty"`
		DerivativesContext    *local.V7DerivativesContext `json:"derivatives_context,omitempty"`
	}

	payload := v7SignalForAI{
		Symbol:                coin.Symbol,
		Direction:             coin.Direction,
		SetupType:             coin.V7SetupType,
		Status:                coin.V7Status,
		MarketRegime:          coin.V7MarketRegime,
		EntryMode:             coin.V7EntryMode,
		Confidence:            coin.V7Confidence,
		RiskLevel:             coin.V7RiskLevel,
		AIPriority:            coin.V7AIPriority,
		SetupScore:            coin.V7SetupScore,
		TimingScore:           coin.V7TimingScore,
		RegimeFitScore:        coin.V7RegimeFitScore,
		LiquidityScore:        coin.V7LiquidityScore,
		RiskScore:             coin.V7RiskScore,
		ReasonCodes:           coin.V7ReasonCodes,
		RiskTags:              coin.V7RiskTags,
		RequiredConfirmations: coin.V7RequiredConfirms,
		EntryZone:             coin.V7EntryZone,
		Invalidation:          coin.V7Invalidation,
		Targets:               coin.V7Targets,
		PriceContext:          coin.V7PriceContext,
		DerivativesContext:    coin.V7DerivativesCtx,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func (e *StrategyEngine) formatPositionInfo(index int, pos PositionInfo, ctx *Context) string {
	var sb strings.Builder

	holdingDuration := ""
	if pos.UpdateTime > 0 {
		durationMs := time.Now().UnixMilli() - pos.UpdateTime
		durationMin := durationMs / (1000 * 60)
		if durationMin < 60 {
			holdingDuration = fmt.Sprintf(" | Holding Duration %d min", durationMin)
		} else {
			durationHour := durationMin / 60
			durationMinRemainder := durationMin % 60
			holdingDuration = fmt.Sprintf(" | Holding Duration %dh %dm", durationHour, durationMinRemainder)
		}
	}

	positionValue := pos.Quantity * pos.MarkPrice
	if positionValue < 0 {
		positionValue = -positionValue
	}

	sb.WriteString(fmt.Sprintf("%d. %s %s | Entry %.4f Current %.4f | Qty %.4f | Position Value %.2f USDT | PnL%+.2f%% | PnL Amount%+.2f USDT | Peak PnL%.2f%% | Leverage %dx | Margin %.0f | Liq Price %.4f%s\n\n",
		index, pos.Symbol, strings.ToUpper(pos.Side),
		pos.EntryPrice, pos.MarkPrice, pos.Quantity, positionValue, pos.UnrealizedPnLPct, pos.UnrealizedPnL, pos.PeakPnLPct,
		pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration))

	if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
		sb.WriteString(e.formatMarketData(marketData))

		if ctx.QuantDataMap != nil {
			if quantData, hasQuant := ctx.QuantDataMap[pos.Symbol]; hasQuant {
				sb.WriteString(e.formatQuantData(quantData))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (e *StrategyEngine) formatCoinSourceTag(sources []string) string {
	if len(sources) > 1 {
		// Multiple signal source combination
		hasAI500 := false
		hasOITop := false
		hasOILow := false
		hasHyperAll := false
		hasHyperMain := false
		hasHunter := false
		for _, s := range sources {
			switch s {
			case "ai500":
				hasAI500 = true
			case "oi_top":
				hasOITop = true
			case "oi_low":
				hasOILow = true
			case "hyper_all":
				hasHyperAll = true
			case "hyper_main":
				hasHyperMain = true
			case "hunter":
				hasHunter = true
			}
		}
		if hasAI500 && hasOITop {
			return " (AI500+OI_Top dual signal)"
		}
		if hasAI500 && hasOILow {
			return " (AI500+OI_Low dual signal)"
		}
		if hasOITop && hasOILow {
			return " (OI_Top+OI_Low)"
		}
		if hasHyperMain && hasAI500 {
			return " (HyperMain+AI500)"
		}
		if hasHunter && hasAI500 {
			return " (Hunter+AI500)"
		}
		if hasHunter {
			return " (Hunter+multi)"
		}
		if hasHyperAll || hasHyperMain {
			return " (Hyperliquid)"
		}
		return " (Multiple sources)"
	} else if len(sources) == 1 {
		switch sources[0] {
		case "ai500":
			return " (AI500)"
		case "oi_top":
			return " (OI_Top OI increase)"
		case "oi_low":
			return " (OI_Low OI decrease)"
		case "static":
			return " (Manual selection)"
		case "hyper_all":
			return " (Hyperliquid All)"
		case "hyper_main":
			return " (Hyperliquid Top20)"
		case "hunter":
			return " (Hunter)"
		}
	}
	return ""
}

// ============================================================================
// Market Data Formatting
// ============================================================================

func (e *StrategyEngine) formatMarketData(data *market.Data) string {
	var sb strings.Builder
	indicators := e.config.Indicators

	// Clearly label the coin symbol
	sb.WriteString(fmt.Sprintf("=== %s Market Data ===\n\n", data.Symbol))
	sb.WriteString(fmt.Sprintf("current_price = %.4f", data.CurrentPrice))

	if indicators.EnableEMA {
		sb.WriteString(fmt.Sprintf(", current_ema20 = %.3f", data.CurrentEMA20))
	}

	if indicators.EnableMACD {
		sb.WriteString(fmt.Sprintf(", current_macd = %.3f", data.CurrentMACD))
	}

	if indicators.EnableRSI {
		sb.WriteString(fmt.Sprintf(", current_rsi7 = %.3f", data.CurrentRSI7))
	}

	sb.WriteString("\n\n")

	if indicators.EnableOI || indicators.EnableFundingRate {
		sb.WriteString(fmt.Sprintf("Additional data for %s:\n\n", data.Symbol))

		if indicators.EnableOI && data.OpenInterest != nil {
			sb.WriteString(fmt.Sprintf("Open Interest: Latest: %.2f Average: %.2f\n\n",
				data.OpenInterest.Latest, data.OpenInterest.Average))
		}

		if indicators.EnableFundingRate {
			sb.WriteString(fmt.Sprintf("Funding Rate: %.2e\n\n", data.FundingRate))
		}
	}

	if len(data.TimeframeData) > 0 {
		timeframeOrder := []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d", "3d", "1w"}
		for _, tf := range timeframeOrder {
			if tfData, ok := data.TimeframeData[tf]; ok {
				sb.WriteString(fmt.Sprintf("=== %s Timeframe (oldest → latest) ===\n\n", strings.ToUpper(tf)))
				e.formatTimeframeSeriesData(&sb, tfData, indicators)
			}
		}
	} else {
		// Compatible with old data format
		if data.IntradaySeries != nil {
			klineConfig := indicators.Klines
			sb.WriteString(fmt.Sprintf("Intraday series (%s intervals, oldest → latest):\n\n", klineConfig.PrimaryTimeframe))

			if len(data.IntradaySeries.MidPrices) > 0 {
				sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.IntradaySeries.MidPrices)))
			}

			if indicators.EnableEMA && len(data.IntradaySeries.EMA20Values) > 0 {
				sb.WriteString(fmt.Sprintf("EMA indicators (20-period): %s\n\n", formatFloatSlice(data.IntradaySeries.EMA20Values)))
			}

			if indicators.EnableMACD && len(data.IntradaySeries.MACDValues) > 0 {
				sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.IntradaySeries.MACDValues)))
			}

			if indicators.EnableRSI {
				if len(data.IntradaySeries.RSI7Values) > 0 {
					sb.WriteString(fmt.Sprintf("RSI indicators (7-Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI7Values)))
				}
				if len(data.IntradaySeries.RSI14Values) > 0 {
					sb.WriteString(fmt.Sprintf("RSI indicators (14-Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI14Values)))
				}
			}

			if indicators.EnableVolume && len(data.IntradaySeries.Volume) > 0 {
				sb.WriteString(fmt.Sprintf("Volume: %s\n\n", formatFloatSlice(data.IntradaySeries.Volume)))
			}

			if indicators.EnableATR {
				sb.WriteString(fmt.Sprintf("3m ATR (14-period): %.3f\n\n", data.IntradaySeries.ATR14))
			}
		}

		if data.LongerTermContext != nil && indicators.Klines.EnableMultiTimeframe {
			sb.WriteString(fmt.Sprintf("Longer-term context (%s timeframe):\n\n", indicators.Klines.LongerTimeframe))

			if indicators.EnableEMA {
				sb.WriteString(fmt.Sprintf("20-Period EMA: %.3f vs. 50-Period EMA: %.3f\n\n",
					data.LongerTermContext.EMA20, data.LongerTermContext.EMA50))
			}

			if indicators.EnableATR {
				sb.WriteString(fmt.Sprintf("3-Period ATR: %.3f vs. 14-Period ATR: %.3f\n\n",
					data.LongerTermContext.ATR3, data.LongerTermContext.ATR14))
			}

			if indicators.EnableVolume {
				sb.WriteString(fmt.Sprintf("Current Volume: %.3f vs. Average Volume: %.3f\n\n",
					data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))
			}

			if indicators.EnableMACD && len(data.LongerTermContext.MACDValues) > 0 {
				sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
			}

			if indicators.EnableRSI && len(data.LongerTermContext.RSI14Values) > 0 {
				sb.WriteString(fmt.Sprintf("RSI indicators (14-Period): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
			}
		}
	}

	return sb.String()
}

func (e *StrategyEngine) formatTimeframeSeriesData(sb *strings.Builder, data *market.TimeframeSeriesData, indicators store.IndicatorConfig) {
	if len(data.Klines) > 0 {
		sb.WriteString("Time(UTC)      Open      High      Low       Close     Volume\n")
		for i, k := range data.Klines {
			t := time.Unix(k.Time/1000, 0).UTC()
			timeStr := t.Format("01-02 15:04")
			marker := ""
			if i == len(data.Klines)-1 {
				marker = "  <- current"
			}
			sb.WriteString(fmt.Sprintf("%-14s %-9.4f %-9.4f %-9.4f %-9.4f %-12.2f%s\n",
				timeStr, k.Open, k.High, k.Low, k.Close, k.Volume, marker))
		}
		sb.WriteString("\n")
	} else if len(data.MidPrices) > 0 {
		sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.MidPrices)))
		if indicators.EnableVolume && len(data.Volume) > 0 {
			sb.WriteString(fmt.Sprintf("Volume: %s\n\n", formatFloatSlice(data.Volume)))
		}
	}

	if indicators.EnableEMA {
		if len(data.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA20: %s\n", formatFloatSlice(data.EMA20Values)))
		}
		if len(data.EMA50Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA50: %s\n", formatFloatSlice(data.EMA50Values)))
		}
	}

	if indicators.EnableMACD && len(data.MACDValues) > 0 {
		sb.WriteString(fmt.Sprintf("MACD: %s\n", formatFloatSlice(data.MACDValues)))
	}

	if indicators.EnableRSI {
		if len(data.RSI7Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI7: %s\n", formatFloatSlice(data.RSI7Values)))
		}
		if len(data.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI14: %s\n", formatFloatSlice(data.RSI14Values)))
		}
	}

	if indicators.EnableATR && data.ATR14 > 0 {
		sb.WriteString(fmt.Sprintf("ATR14: %.4f\n", data.ATR14))
	}

	if indicators.EnableBOLL && len(data.BOLLUpper) > 0 {
		sb.WriteString(fmt.Sprintf("BOLL Upper: %s\n", formatFloatSlice(data.BOLLUpper)))
		sb.WriteString(fmt.Sprintf("BOLL Middle: %s\n", formatFloatSlice(data.BOLLMiddle)))
		sb.WriteString(fmt.Sprintf("BOLL Lower: %s\n", formatFloatSlice(data.BOLLLower)))
	}

	sb.WriteString("\n")
}

func (e *StrategyEngine) formatQuantData(data *QuantData) string {
	if data == nil {
		return ""
	}

	indicators := e.config.Indicators
	if !indicators.EnableQuantOI && !indicators.EnableQuantNetflow {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 %s Quantitative Data:\n", data.Symbol))

	if len(data.PriceChange) > 0 {
		sb.WriteString("Price Change: ")
		timeframes := []string{"5m", "15m", "1h", "4h", "12h", "24h"}
		parts := []string{}
		for _, tf := range timeframes {
			if v, ok := data.PriceChange[tf]; ok {
				parts = append(parts, fmt.Sprintf("%s: %+.4f%%", tf, v*100))
			}
		}
		sb.WriteString(strings.Join(parts, " | "))
		sb.WriteString("\n")
	}

	if indicators.EnableQuantNetflow && data.Netflow != nil {
		sb.WriteString("Fund Flow (Netflow):\n")
		timeframes := []string{"5m", "15m", "1h", "4h", "12h", "24h"}

		if data.Netflow.Institution != nil {
			if data.Netflow.Institution.Future != nil && len(data.Netflow.Institution.Future) > 0 {
				sb.WriteString("  Institutional Futures:\n")
				for _, tf := range timeframes {
					if v, ok := data.Netflow.Institution.Future[tf]; ok {
						sb.WriteString(fmt.Sprintf("    %s: %s\n", tf, formatFlowValue(v)))
					}
				}
			}
			if data.Netflow.Institution.Spot != nil && len(data.Netflow.Institution.Spot) > 0 {
				sb.WriteString("  Institutional Spot:\n")
				for _, tf := range timeframes {
					if v, ok := data.Netflow.Institution.Spot[tf]; ok {
						sb.WriteString(fmt.Sprintf("    %s: %s\n", tf, formatFlowValue(v)))
					}
				}
			}
		}

		if data.Netflow.Personal != nil {
			if data.Netflow.Personal.Future != nil && len(data.Netflow.Personal.Future) > 0 {
				sb.WriteString("  Retail Futures:\n")
				for _, tf := range timeframes {
					if v, ok := data.Netflow.Personal.Future[tf]; ok {
						sb.WriteString(fmt.Sprintf("    %s: %s\n", tf, formatFlowValue(v)))
					}
				}
			}
			if data.Netflow.Personal.Spot != nil && len(data.Netflow.Personal.Spot) > 0 {
				sb.WriteString("  Retail Spot:\n")
				for _, tf := range timeframes {
					if v, ok := data.Netflow.Personal.Spot[tf]; ok {
						sb.WriteString(fmt.Sprintf("    %s: %s\n", tf, formatFlowValue(v)))
					}
				}
			}
		}
	}

	if indicators.EnableQuantOI && len(data.OI) > 0 {
		for exchange, oiData := range data.OI {
			if len(oiData.Delta) > 0 {
				sb.WriteString(fmt.Sprintf("Open Interest (%s):\n", exchange))
				for _, tf := range []string{"5m", "15m", "1h", "4h", "12h", "24h"} {
					if d, ok := oiData.Delta[tf]; ok {
						sb.WriteString(fmt.Sprintf("    %s: %+.4f%% (%s)\n", tf, d.OIDeltaPercent, formatFlowValue(d.OIDeltaValue)))
					}
				}
			}
		}
	}

	return sb.String()
}

func formatFlowValue(v float64) string {
	sign := ""
	if v >= 0 {
		sign = "+"
	}
	absV := v
	if absV < 0 {
		absV = -absV
	}
	if absV >= 1e9 {
		return fmt.Sprintf("%s%.2fB", sign, v/1e9)
	} else if absV >= 1e6 {
		return fmt.Sprintf("%s%.2fM", sign, v/1e6)
	} else if absV >= 1e3 {
		return fmt.Sprintf("%s%.2fK", sign, v/1e3)
	}
	return fmt.Sprintf("%s%.2f", sign, v)
}

func formatFloatSlice(values []float64) string {
	strValues := make([]string, len(values))
	for i, v := range values {
		strValues[i] = fmt.Sprintf("%.4f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}
