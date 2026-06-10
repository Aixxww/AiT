package kernel

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/aitos"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// ============================================================================
// Prompt Building - System Prompt
// ============================================================================

const (
	hunterV7ProtectionTP1PnLPct         = 6.0
	hunterV7ProtectionNearTP1PeakPnLPct = hunterV7ProtectionTP1PnLPct * 0.95
)

// BuildSystemPrompt builds System Prompt according to strategy configuration
func (e *StrategyEngine) BuildSystemPrompt(accountEquity float64, variant string) string {
	var sb strings.Builder
	riskControl := e.config.RiskControl
	promptSections := e.config.PromptSections
	minOpenConfidence := e.effectiveMinOpenConfidence(riskControl.MinConfidence)

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
	sb.WriteString(fmt.Sprintf("- Min Position Size: ≥%.0f USDT\n", riskControl.MinPositionSize))
	if riskControl.MinRiskRewardRatio > 0 {
		sb.WriteString(fmt.Sprintf("- Risk-Reward Ratio: ≥1:%.1f, calculated from unleveraged price distances\n", riskControl.MinRiskRewardRatio))
	}
	maxTPMovePct, minSLMovePct, maxDriftPct := e.effectiveExecutionGeometry()
	if maxDriftPct > 0 {
		sb.WriteString(fmt.Sprintf("- Max Entry Price Drift: ≤%.2f%% from your output price to executable price\n", maxDriftPct))
	}
	if minSLMovePct > 0 {
		sb.WriteString(fmt.Sprintf("- Min Stop-Loss Distance: ≥%.2f%% from executable/current price\n", minSLMovePct))
	}
	if maxTPMovePct > 0 {
		sb.WriteString(fmt.Sprintf("- Max Take-Profit Distance: ≤%.2f%% from executable/current price after backend cap\n", maxTPMovePct))
	}
	if riskControl.MinRiskRewardRatio > 0 && minSLMovePct > 0 && maxTPMovePct > 0 {
		minRewardPct := minSLMovePct * riskControl.MinRiskRewardRatio
		sb.WriteString(fmt.Sprintf("- Feasible open geometry: reward distance must be ≥%.2f%% for the minimum %.2f%% stop; if capped TP or price drift makes backend RR < %.2f, output wait\n",
			minRewardPct, minSLMovePct, riskControl.MinRiskRewardRatio))
		if strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") && maxDriftPct > 0 {
			sb.WriteString(fmt.Sprintf("- Hunter v7 stop buffer: prefer stop distance ≥%.2f%% from current/executable price when TP cap still preserves RR; avoid stops only barely above %.2f%% because allowed %.2f%% entry drift can make them fail backend validation\n",
				minSLMovePct+maxDriftPct, minSLMovePct, maxDriftPct))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## AI GUIDED (Recommended, you should follow):\n")
	sb.WriteString(fmt.Sprintf("- Trading Leverage: Altcoins max %dx | BTC/ETH max %dx\n",
		riskControl.AltcoinMaxLeverage, riskControl.BTCETHMaxLeverage))
	sb.WriteString(fmt.Sprintf("- Min Confidence: ≥%d to open position\n\n", minOpenConfidence))

	// Position sizing guidance
	sb.WriteString("## Position Sizing Guidance\n")
	sb.WriteString("Calculate `position_size_usd` based on your confidence and the Position Value Limits above:\n")
	sb.WriteString("- `position_size_usd` is the order notional / position value in USDT, not margin. Margin used is approximately `position_size_usd / leverage`.\n")
	sb.WriteString("- Do not multiply `position_size_usd` by leverage again when checking position value, loss at stop, or take-profit distance.\n")
	sb.WriteString("- High confidence (≥85): Use 80-100%% of max position value limit\n")
	if minOpenConfidence <= 70 {
		sb.WriteString("- Medium confidence (70-84): Use 50-80%% of max position value limit\n")
	} else {
		sb.WriteString(fmt.Sprintf("- Medium confidence (%d-84): Use 50-80%% of max position value limit\n", minOpenConfidence))
	}
	if minOpenConfidence <= 60 {
		sb.WriteString("- Low confidence (60-69): Use 30-50%% of max position value limit\n")
	} else {
		sb.WriteString(fmt.Sprintf("- Confidence below %d must output wait; do not open by reducing position size.\n", minOpenConfidence))
	}
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
		sb.WriteString(fmt.Sprintf("\n**Confidence ≥ %d** required to open positions.\n\n", minOpenConfidence))
	} else {
		sb.WriteString("# 🎯 Entry Standards (Strict)\n\n")
		sb.WriteString("Only open positions when multiple signals resonate. You have:\n")
		e.writeAvailableIndicators(&sb)
		sb.WriteString(fmt.Sprintf("\nFeel free to use any effective analysis method, but **confidence ≥ %d** required to open positions; avoid low-quality behaviors such as single indicators, contradictory signals, sideways consolidation, reopening immediately after closing, etc.\n\n", minOpenConfidence))
	}

	// 6. Decision process (editable)
	if promptSections.DecisionProcess != "" {
		sb.WriteString(promptSections.DecisionProcess)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("# 📋 Decision Process\n\n")
		sb.WriteString("1. Check positions → Should we take profit/stop-loss\n")
		sb.WriteString("2. Scan candidate coins + multi-timeframe → Are there strong signals\n")
		sb.WriteString("3. Write a concise decision audit first, then output structured JSON\n\n")
	}

	// 7. Output format
	sb.WriteString("# Output Format (Strictly Follow)\n\n")
	sb.WriteString("**Must use XML tags <reasoning> and <decision> to separate a concise decision audit and decision JSON, avoiding parsing errors**\n\n")
	sb.WriteString("## Format Requirements\n\n")
	sb.WriteString("<reasoning>\n")
	sb.WriteString("Concise decision audit only. No lengthy chain-of-thought.\n")
	sb.WriteString("- Use at most 6 short bullet lines; focus on pass/fail blockers and executable price/RR.\n")
	sb.WriteString("</reasoning>\n\n")
	sb.WriteString("<decision>\n")
	sb.WriteString("Step 2: JSON decision array\n\n")
	sb.WriteString("```json\n[\n")
	// Use the actual configured position value ratio for BTC/ETH in the example
	examplePositionSize := accountEquity * btcEthPosValueRatio
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"price\": 95000, \"stop_loss\": 97000, \"take_profit\": 91000, \"confidence\": 85},\n",
		riskControl.BTCETHMaxLeverage, examplePositionSize))
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\"}\n")
	sb.WriteString("]\n```\n")
	sb.WriteString("</decision>\n\n")
	sb.WriteString("Keep the JSON `reasoning` field under 320 characters. For Hunter v7 wait decisions, put the blocker in `blocked_reason_code` and keep explanation concise.\n\n")
	sb.WriteString("## Field Description\n\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | hold | wait\n")
	sb.WriteString(fmt.Sprintf("- `confidence`: 0-100 (opening recommended ≥ %d)\n", minOpenConfidence))
	sb.WriteString("- Required when opening: leverage, position_size_usd, price, stop_loss, take_profit, confidence\n")
	sb.WriteString("- `position_size_usd` is already notional exposure. Do not output `margin × leverage × leverage`; use only the intended notional exposure.\n")
	sb.WriteString("- `price` for open orders must be the current executable reference price used for RR checks, not a distant target or stale signal price\n")
	sb.WriteString("- `hold` and `wait` are no-op actions: they do not change stop-loss, take-profit, leverage, or position size. If profit protection requires action, output a risk-reducing `close_long`/`close_short`; otherwise output `hold` without claiming that stops were tightened.\n")
	if strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") {
		sb.WriteString("- `blocked_reason_code` (REQUIRED when action is `wait`): one of `entry_not_in_zone`, `rr_insufficient`, `confirmation_missing`, `oi_too_low`, `funding_crowded`, `account_risk`, `backend_guard_risk`, `no_reviewable_candidate`. Do NOT use free-text reasoning to replace this field.\n")
		sb.WriteString("- `no_reviewable_candidate` is valid ONLY when Tier Summary has EXECUTABLE=0 and REVIEWABLE=0. If any EXECUTABLE/REVIEWABLE exists, use the real blocker such as `entry_not_in_zone`, `rr_insufficient`, `confirmation_missing`, or `backend_guard_risk`.\n")
	}
	sb.WriteString("- **IMPORTANT**: All numeric values must be calculated numbers, NOT formulas/expressions (e.g., use `27.76` not `3000 * 0.01`)\n\n")

	// 8. Custom Prompt
	if e.config.CustomPrompt != "" {
		sb.WriteString("# 📌 Personalized Trading Strategy\n\n")
		sb.WriteString(e.config.CustomPrompt)
		sb.WriteString("\n\n")
		sb.WriteString("Note: The above personalized strategy is a supplement to the basic rules and cannot violate the basic risk control principles.\n")
	}
	if strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") {
		e.writeHunterV7ExecutionPreflightPrompt(&sb)
	}

	return sb.String()
}

func (e *StrategyEngine) writeHunterV7ExecutionPreflightPrompt(sb *strings.Builder) {
	maxTPMovePct, minSLMovePct, maxDriftPct := e.effectiveExecutionGeometry()
	minRR := e.config.RiskControl.MinRiskRewardRatio
	if minRR <= 0 {
		minRR = 1.5
	}
	if e.GetLanguage() == LangChinese {
		sb.WriteString("\n\n# Hunter v7 执行规则\n\n")
		sb.WriteString("## Tier 漏斗决策原则\n")
		sb.WriteString("严格按 EXECUTABLE → REVIEWABLE → WATCH(背景) → REJECTED(禁止) 顺序决策。不得因账户回撤、市场情绪或 WATCH 候选缺失而跳过 EXECUTABLE/REVIEWABLE 评估。\n\n")
		sb.WriteString(fmt.Sprintf("完整判断 `EXECUTABLE` 与 `REVIEWABLE` 候选；`WATCH` 只作背景，`REJECTED` 不参与开仓。无持仓且存在开仓复核候选时，在最佳 open 与明确 blocked_reason 之间二选一。后端允许轻微修复：价格漂移 %.2f%% 内、SL 边界 %.2f%%、RR %.2f 且 TP 在 %.2f%% 可达范围内。\n",
			maxDriftPct, minSLMovePct, minRR, maxTPMovePct))
		sb.WriteString("## blocked_reason_code 强制要求\n")
		sb.WriteString("wait 决策必须输出 `blocked_reason_code` 字段（枚举值：`entry_not_in_zone`、`rr_insufficient`、`confirmation_missing`、`oi_too_low`、`funding_crowded`、`account_risk`、`backend_guard_risk`、`no_reviewable_candidate`）。绝对不得用自然语言 reasoning 代替此字段。如果 wait，必须有且只有一个 blocked_reason_code。\n")
		sb.WriteString("`no_reviewable_candidate` 只能在 Tier Summary 显示 EXECUTABLE=0 且 REVIEWABLE=0 时使用；只要存在任一 EXECUTABLE/REVIEWABLE，就必须写真实阻断原因，例如 `entry_not_in_zone`、`rr_insufficient`、`confirmation_missing` 或 `backend_guard_risk`。\n")
		sb.WriteString("## 账户回撤规则\n")
		sb.WriteString("账户总回撤和最近亏损只用于：(1) 仓位大小调整、(2) 重复交易冷却、(3) 同 symbol 冷却。绝对禁止把“账户处于回撤”当作所有 EXECUTABLE/REVIEWABLE 候选的全局 wait 理由。若候选满足硬风控、setup 核心确认和 RR，应给出保守仓位 open。\n")
		if minSLMovePct > 0 && maxDriftPct > 0 {
			sb.WriteString(fmt.Sprintf("开仓 stop_loss 不要只贴着 %.2f%% 最小距离；优先留到约 %.2f%% 以上，除非结构止损不允许且 RR 仍明确通过。\n",
				minSLMovePct, minSLMovePct+maxDriftPct))
		}
		sb.WriteString("动量/突破类（如 leader_momentum_long、trend_breakout_long）若实时价跌破 signal stop/invalidation、entry_zone 下沿，或 5m 动量从强转弱，不得把回落视为更好入场，必须 wait。\n")
		sb.WriteString("leader_momentum_long 若处于 1h 回落/浅回踩，但实时价仍在 entry_zone 上沿且 taker buy 未明显增强，默认 wait，等待回踩到中下沿或重新放量突破；不要把 zone_upper 的弱回踩当优质追多。\n")
		sb.WriteString("若 signal risk_tags 含 already_pumped_24h、funding_expensive、lsr_extreme_long、taker_sell_during_accumulation、no_reclaim_signal、oi_up_price_down、late_*_without_flush 或 not_near_*_zone，这些是 wait-only 风险语义；不得把高 priority/score 当作开仓理由覆盖这些标签。\n")
		sb.WriteString("持仓管理：若 Peak PnL 已接近保护 TP1（当前保护 TP1 6%，near-TP1 为 Peak PnL >=5.7%），随后从峰值回撤 >=45% 或当前 PnL 回到盈亏平衡/亏损，必须优先视为退出/减风险信号；除非有非常明确的延续证据，否则输出可执行的 `close_long`/`close_short`，不得只因未到 SL/TP 而 `hold`，也不得用 `hold` 声称收紧止损。若曾经错过 near-TP1，之后又回到 TP1 的 90% 以上，这是二次保护机会，应优先 close/reduce，而不是再次等待精确 TP1。若 Peak PnL 低于 5.7%，它属于 pre-TP1 波动，峰值回吐或回到微利/微亏本身不是移动止盈触发；只有计划 SL/硬失效，或 5m 与 15m 同时确认结构反转（跌破入场/EMA20、低点下移、买盘/OI 走弱）才可 close。峰值回撤按 `(Peak PnL - Current PnL) / Peak PnL` 计算；正峰值后跌到负 PnL 代表回吐超过 100%，但 pre-TP1 不能仅凭该比例退出。\n")
		return
	}
	sb.WriteString("\n\n# Hunter v7 Execution Rules\n\n")
	sb.WriteString("## Tier Funnel Decision Principle\n")
	sb.WriteString("Evaluate strictly in order: EXECUTABLE → REVIEWABLE → WATCH (context) → REJECTED (forbidden). Never skip EXECUTABLE/REVIEWABLE evaluation due to account drawdown, market sentiment, or absence of WATCH candidates.\n\n")
	sb.WriteString(fmt.Sprintf("Fully judge `EXECUTABLE` and `REVIEWABLE` candidates; `WATCH` is context and `REJECTED` is not open-eligible. If flat and an open-review candidate exists, choose the best open or provide one precise blocked_reason. Backend may lightly repair drift %.2f%%, SL edge %.2f%%, and RR %.2f when TP stays within %.2f%%.\n",
		maxDriftPct, minSLMovePct, minRR, maxTPMovePct))
	sb.WriteString("## blocked_reason_code Requirement\n")
	sb.WriteString("Wait decisions MUST include a `blocked_reason_code` field (enum: `entry_not_in_zone`, `rr_insufficient`, `confirmation_missing`, `oi_too_low`, `funding_crowded`, `account_risk`, `backend_guard_risk`, `no_reviewable_candidate`). ABSOLUTELY do NOT substitute free-text reasoning for this field. If wait, there must be exactly one blocked_reason_code.\n")
	sb.WriteString("Use `no_reviewable_candidate` ONLY when Tier Summary shows EXECUTABLE=0 and REVIEWABLE=0. If any EXECUTABLE/REVIEWABLE exists, use the real blocker such as `entry_not_in_zone`, `rr_insufficient`, `confirmation_missing`, or `backend_guard_risk`.\n")
	sb.WriteString("## Account Drawdown Rule\n")
	sb.WriteString("Account drawdown and recent losses are ONLY for: (1) position sizing, (2) repeat-trade cooldown, (3) same-symbol cooldown. It is FORBIDDEN to use 'account is in drawdown' as a global wait veto for every EXECUTABLE/REVIEWABLE candidate. If a candidate passes hard risk controls, setup confirmation, and RR, open with conservative size.\n")
	if minSLMovePct > 0 && maxDriftPct > 0 {
		sb.WriteString(fmt.Sprintf("Do not place stop_loss barely above the %.2f%% minimum; prefer roughly %.2f%%+ stop distance when structure and RR allow, because allowed %.2f%% entry drift can otherwise fail backend validation.\n",
			minSLMovePct, minSLMovePct+maxDriftPct, maxDriftPct))
	}
	sb.WriteString("For momentum/breakout setups such as leader_momentum_long or trend_breakout_long, if live price breaks below signal stop/invalidation, below entry_zone.lower, or 5m momentum flips from strong to weak, do not treat the pullback as a better entry; wait.\n")
	sb.WriteString("For leader_momentum_long, if it is a 1h pullback/shallow pullback but live price is still near entry_zone.upper and taker buy is not clearly strengthening, wait for a mid/lower-zone pullback or renewed high-volume breakout; do not treat weak upper-zone pullbacks as quality long entries.\n")
	sb.WriteString("If signal risk_tags include already_pumped_24h, funding_expensive, lsr_extreme_long, taker_sell_during_accumulation, no_reclaim_signal, oi_up_price_down, late_*_without_flush, or not_near_*_zone, those are wait-only risk semantics; do not override them with high priority/score.\n")
	sb.WriteString("Position management: if Peak PnL reached protection near-TP1 (protection TP1 is 6%; near-TP1 means Peak PnL >=5.7%) and then gives back >=45% from the peak or current PnL crosses to breakeven/loss, treat it first as an exit/risk-reduction signal; output an executable risk-reducing `close_long`/`close_short` unless there is a very explicit continuation signal. Do not `hold` only because SL/TP has not been reached, and do not use `hold` to claim stop tightening. If a missed near-TP1 trade later returns above 90% of TP1, treat it as a second protection chance and prefer close/reduce rather than waiting again for exact TP1. If Peak PnL is below 5.7%, it is pre-TP1 noise: peak giveback or returning to tiny profit/loss is not a trailing-take-profit trigger by itself; close only on planned SL/hard invalidation or when both 5m and 15m confirm structural reversal (entry/EMA20 lost, lower lows, weakening buy flow/OI). Peak giveback is `(Peak PnL - Current PnL) / Peak PnL`; crossing from a positive peak to negative PnL is >100% giveback, but pre-TP1 cannot exit from that ratio alone.\n")
}

func (e *StrategyEngine) effectiveMinOpenConfidence(configured int) int {
	if configured <= 0 {
		return configured
	}
	if strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") && configured > 70 {
		return 70
	}
	return configured
}

func (e *StrategyEngine) effectiveExecutionGeometry() (maxTPMovePct, minSLMovePct, maxDriftPct float64) {
	if e == nil {
		return 0, 0, 0
	}
	riskControl := e.config.RiskControl
	maxTPMovePct = riskControl.MaxTakeProfitPriceMovePct
	minSLMovePct = riskControl.MinStopLossPriceMovePct
	maxDriftPct = riskControl.MaxEntryPriceDeviationPct

	if strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") {
		if maxTPMovePct <= 0 {
			maxTPMovePct = 3.0
		}
		if minSLMovePct <= 0 {
			minSLMovePct = 2.0
		}
		if maxDriftPct <= 0 {
			maxDriftPct = 0.5
		}
		minRR := riskControl.MinRiskRewardRatio
		if minRR <= 0 {
			minRR = 1.5
		}
		minFeasibleTPPct := (minSLMovePct + maxDriftPct) * minRR
		minFeasibleTPPct += 0.25 // execution/rounding buffer in percentage points
		if maxTPMovePct < minFeasibleTPPct {
			maxTPMovePct = minFeasibleTPPct
		}
	}

	return maxTPMovePct, minSLMovePct, maxDriftPct
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

	if ctx.IsDegraded {
		reasons := strings.Join(ctx.DegradationReasons, "; ")
		if reasons == "" {
			reasons = "account or position data is stale"
		}
		sb.WriteString("## Trading Context Degraded\n")
		sb.WriteString(fmt.Sprintf("Reason: %s\n", reasons))
		sb.WriteString("Open orders are disabled for this cycle. Only hold, wait, or risk-reducing close decisions are allowed.\n\n")
	}

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

	if strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") {
		e.writeHunterV7TieredCandidatePrompt(&sb, ctx, positionSymbols)
	} else {
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
				if coin.V7Status == "wait_confirm" || coin.V7Status == "conflict_watch" || coin.V7RiskLevel == "HIGH" || coin.V7RiskLevel == "EXTREME" || coin.V7ExecutionQuality == "watch_only" {
					sb.WriteString("v7_execution_gate: wait for required confirmations / directional trigger before entry.\n")
				}
				if coin.V7ExecutionQuality == "watch_only" || containsStringValue(coin.V7RiskTags, "do_not_open_until_confirmed") {
					sb.WriteString("v7_watch_only_policy: do not open directly from this signal; output wait unless it is upgraded by a confirmed setup in a later cycle.\n")
				}
				sb.WriteString("\n")
			}

			// Show Hunter signal tags for AI context
			if coin.V7SetupType == "" && len(coin.SignalTags) > 0 && coin.Direction != "" {
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
			if e.shouldCompactCandidatePrompt(coin, ctx) {
				sb.WriteString(e.formatCompactMarketData(marketData, &coin))
			} else {
				sb.WriteString(e.formatMarketData(marketData))
			}

			if ctx.QuantDataMap != nil {
				if quantData, hasQuant := ctx.QuantDataMap[coin.Symbol]; hasQuant {
					sb.WriteString(e.formatQuantData(quantData))
				}
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")

	// Get language for market data formatting
	aitosLang := aitos.LangEnglish
	if e.GetLanguage() == LangChinese {
		aitosLang = aitos.LangChinese
	}

	// OI Ranking data (market-wide open interest changes)
	if ctx.OIRankingData != nil {
		sb.WriteString(aitos.FormatOIRankingForAI(ctx.OIRankingData, aitosLang))
	}

	// NetFlow Ranking data (market-wide fund flow)
	if ctx.NetFlowRankingData != nil {
		sb.WriteString(aitos.FormatNetFlowRankingForAI(ctx.NetFlowRankingData, aitosLang))
	}

	// Price Ranking data (market-wide gainers/losers)
	if ctx.PriceRankingData != nil {
		sb.WriteString(aitos.FormatPriceRankingForAI(ctx.PriceRankingData, aitosLang))
	}

	sb.WriteString("---\n\n")
	sb.WriteString("Now output a concise decision audit and the decision JSON.\n")

	return sb.String()
}

type hunterV7PromptCandidate struct {
	Coin   CandidateCoin
	Data   *market.Data
	Tier   string
	Reason string
}

func (e *StrategyEngine) writeHunterV7TieredCandidatePrompt(sb *strings.Builder, ctx *Context, positionSymbols map[string]bool) {
	items := make([]hunterV7PromptCandidate, 0, len(ctx.CandidateCoins))
	for _, coin := range ctx.CandidateCoins {
		if positionSymbols[market.Normalize(coin.Symbol)] {
			continue
		}
		data, ok := ctx.MarketDataMap[coin.Symbol]
		if !ok {
			continue
		}
		coin = hunterV7CandidateWithLiveMarketPrice(coin, data)
		tier, reason := coin.V7ExecutionTier, coin.V7TierReason
		if tier == "" {
			tier, reason = classifyHunterV7CandidateTier(coin)
		}
		items = append(items, hunterV7PromptCandidate{
			Coin:   coin,
			Data:   data,
			Tier:   tier,
			Reason: reason,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		ti := hunterV7TierRank(items[i].Tier)
		tj := hunterV7TierRank(items[j].Tier)
		if ti != tj {
			return ti < tj
		}
		return items[i].Coin.V7AIPriority > items[j].Coin.V7AIPriority
	})

	execCount, reviewableCount, watchCount, rejectedCount := 0, 0, 0, 0
	for _, item := range items {
		switch item.Tier {
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

	positionLimitReached := e.config.RiskControl.MaxPositions > 0 && len(ctx.Positions) >= e.config.RiskControl.MaxPositions
	sb.WriteString(fmt.Sprintf("## Hunter v7 Candidate Tiers (%d total)\n\n", len(items)))
	sb.WriteString(fmt.Sprintf("Tier Summary: EXECUTABLE=%d | REVIEWABLE=%d | WATCH=%d | REJECTED=%d\n\n", execCount, reviewableCount, watchCount, rejectedCount))
	sb.WriteString(local.HunterV7PromptTagPolicy())
	sb.WriteString("\n\n")
	if positionLimitReached {
		if e.GetLanguage() == LangChinese {
			sb.WriteString("Decision policy: 当前持仓数量已达到 Max Positions；候选只作背景摘要，不展开、不要求逐个决策，除非先明确 close 现有仓位，否则禁止新开仓。\n\n")
		} else {
			sb.WriteString("Decision policy: Current positions have reached Max Positions; candidates are summary-only context. Do not expand or decide each candidate, and do not open unless an existing position is explicitly closed first.\n\n")
		}
	} else if execCount+reviewableCount > 0 {
		if e.GetLanguage() == LangChinese {
			sb.WriteString("Decision policy (严格 tier 漏斗):\n")
			sb.WriteString("1. EXECUTABLE 优先：必须首先评估所有 EXECUTABLE 候选。满足硬风控 + setup 核心确认 + RR 的，给出 open；否则给出精确 blocked_reason_code。\n")
			sb.WriteString("2. REVIEWABLE 可复核：EXECUTABLE 全部被 blocked 后才评估 REVIEWABLE。只有实时 K线/资金流确认入场区、近端止损和 RR 后才允许 open，否则给出 blocked_reason_code。\n")
			sb.WriteString("3. WATCH 只做背景：WATCH 候选不参与开仓决策，只提供市场背景。禁止把 WATCH 候选的整体缺失作为对所有 EXECUTABLE/REVIEWABLE 的全局 wait。\n")
			sb.WriteString("4. REJECTED 禁止：REJECTED 候选不得参与任何开仓判断。\n")
			sb.WriteString("wait 决策必须输出 `blocked_reason_code`（枚举值），不得用自然语言 reasoning 代替。账户回撤只影响仓位/冷却，不得作为所有候选的全局 wait 否决。\n\n")
		} else {
			sb.WriteString("Decision policy (strict tier funnel):\n")
			sb.WriteString("1. EXECUTABLE first: evaluate ALL EXECUTABLE candidates first. If a candidate passes hard risk controls, setup core confirmation, and RR, output open; otherwise provide a precise `blocked_reason_code`.\n")
			sb.WriteString("2. REVIEWABLE next: only after all EXECUTABLE candidates are blocked, evaluate REVIEWABLE. Open only when live candles/flow confirm entry zone, near structural stop, and RR; otherwise provide `blocked_reason_code`.\n")
			sb.WriteString("3. WATCH is context only: WATCH candidates do not participate in open decisions. Do not use the absence of WATCH candidates as a global wait veto on EXECUTABLE/REVIEWABLE.\n")
			sb.WriteString("4. REJECTED is forbidden: REJECTED candidates must not influence any open decision.\n")
			sb.WriteString("Wait decisions MUST include `blocked_reason_code` (enum field); free-text reasoning is not a substitute. Account drawdown affects sizing/cooldown only, not a global wait veto for all candidates.\n\n")
		}
	} else {
		if e.GetLanguage() == LangChinese {
			sb.WriteString("Decision policy: 当前没有 EXECUTABLE/REVIEWABLE 候选。不要强行开仓；只允许 wait 或管理已有持仓。\n\n")
		} else {
			sb.WriteString("Decision policy: No EXECUTABLE/REVIEWABLE candidates are available. Do not force a new open; only wait or manage existing positions.\n\n")
		}
	}

	sb.WriteString("### Open-review candidates (full context, max 5)\n\n")
	displayedOpenReview := 0
	if positionLimitReached {
		sb.WriteString("- None (position limit reached; open-review candidates are summary-only below)\n\n")
	} else {
		for _, item := range items {
			if item.Tier != "EXECUTABLE" && item.Tier != "REVIEWABLE" {
				continue
			}
			if displayedOpenReview >= 5 {
				sb.WriteString(fmt.Sprintf("- %s %s setup=%s ai_priority=%.1f reason=%s (not expanded; lower priority)\n",
					item.Coin.Symbol, item.Coin.Direction, item.Coin.V7SetupType, item.Coin.V7AIPriority, item.Reason))
				continue
			}
			displayedOpenReview++
			e.writeHunterV7ExpandedCandidate(sb, item, displayedOpenReview, ctx)
		}
	}
	if displayedOpenReview == 0 && !positionLimitReached {
		sb.WriteString("- None\n\n")
	}

	if positionLimitReached {
		sb.WriteString("### Open-disabled candidate summary\n\n")
		displayedBlocked := 0
		for _, item := range items {
			if item.Tier == "REJECTED" {
				continue
			}
			displayedBlocked++
			if displayedBlocked > 6 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s %s tier=%s setup=%s status=%s quality=%s ai_priority=%.1f risk=%.0f reason=%s\n",
				item.Coin.Symbol, item.Coin.Direction, item.Tier, item.Coin.V7SetupType, item.Coin.V7Status,
				item.Coin.V7ExecutionQuality, item.Coin.V7AIPriority, item.Coin.V7RiskScore, item.Reason))
		}
		if displayedBlocked == 0 {
			sb.WriteString("- None\n")
		}
		if execCount+reviewableCount+watchCount > displayedBlocked {
			sb.WriteString(fmt.Sprintf("- ... %d more non-rejected candidates omitted\n", execCount+reviewableCount+watchCount-displayedBlocked))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("### WATCH candidates (summary only; not direct open)\n\n")
		displayedWatch := 0
		for _, item := range items {
			if item.Tier != "WATCH" {
				continue
			}
			displayedWatch++
			if displayedWatch > 6 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s %s setup=%s status=%s quality=%s ai_priority=%.1f risk=%.0f reason=%s\n",
				item.Coin.Symbol, item.Coin.Direction, item.Coin.V7SetupType, item.Coin.V7Status,
				item.Coin.V7ExecutionQuality, item.Coin.V7AIPriority, item.Coin.V7RiskScore, item.Reason))
		}
		if displayedWatch == 0 {
			sb.WriteString("- None\n")
		}
		if watchCount > displayedWatch {
			sb.WriteString(fmt.Sprintf("- ... %d more WATCH candidates omitted\n", watchCount-displayedWatch))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### REJECTED candidates (count only)\n\n")
	if rejectedCount == 0 {
		sb.WriteString("- None\n\n")
		return
	}
	reasonCounts := make(map[string]int)
	for _, item := range items {
		if item.Tier == "REJECTED" {
			reasonCounts[item.Reason]++
		}
	}
	reasons := make([]string, 0, len(reasonCounts))
	for reason := range reasonCounts {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		sb.WriteString(fmt.Sprintf("- %s: %d\n", reason, reasonCounts[reason]))
	}
	sb.WriteString("\n")
}

func hunterV7CandidateWithLiveMarketPrice(coin CandidateCoin, data *market.Data) CandidateCoin {
	if coin.V7SetupType == "" || data == nil || data.CurrentPrice <= 0 {
		return coin
	}
	priceCtx := local.V7PriceContext{}
	if coin.V7PriceContext != nil {
		priceCtx = *coin.V7PriceContext
	}
	priceCtx.Last = data.CurrentPrice
	coin.V7PriceContext = &priceCtx
	return coin
}

func hunterV7TierRank(tier string) int {
	switch tier {
	case "EXECUTABLE":
		return 1
	case "REVIEWABLE":
		return 2
	case "WATCH":
		return 3
	case "REJECTED":
		return 4
	default:
		return 5
	}
}

func (e *StrategyEngine) writeHunterV7ExpandedCandidate(sb *strings.Builder, item hunterV7PromptCandidate, index int, ctx *Context) {
	coin := item.Coin
	coin.V7ExecutionTier = item.Tier
	coin.V7TierReason = item.Reason
	sourceTags := e.formatCoinSourceTag(coin.Sources)
	directionTag := ""
	if coin.Direction == "LONG" || coin.Direction == "SHORT" {
		directionTag = fmt.Sprintf(" [%s]", coin.Direction)
	}
	sb.WriteString(fmt.Sprintf("#### %d. %s%s%s\n\n", index, coin.Symbol, directionTag, sourceTags))
	sb.WriteString(fmt.Sprintf("execution_tier=%s tier_reason=%s\n", item.Tier, item.Reason))
	if signalJSON := e.formatHunterV7SignalJSON(coin); signalJSON != "" {
		sb.WriteString(fmt.Sprintf("hunter_v7_signal_json: %s\n", signalJSON))
	}
	sb.WriteString("\n")
	sb.WriteString(e.formatCompactMarketData(item.Data, &coin))
	if ctx.QuantDataMap != nil {
		if quantData, hasQuant := ctx.QuantDataMap[coin.Symbol]; hasQuant {
			sb.WriteString(e.formatQuantData(quantData))
		}
	}
	sb.WriteString("\n")
}

func (e *StrategyEngine) formatHunterV7SignalJSON(coin CandidateCoin) string {
	if coin.V7SetupType != "" && coin.V7ExecutionTier == "" {
		coin.V7ExecutionTier, coin.V7TierReason = classifyHunterV7CandidateTier(coin)
	}

	type v7SignalForAI struct {
		Symbol                string                        `json:"symbol"`
		Direction             string                        `json:"direction"`
		SetupType             string                        `json:"setup_type"`
		ExecutionTier         string                        `json:"execution_tier,omitempty"`
		TierReason            string                        `json:"tier_reason,omitempty"`
		Status                string                        `json:"status"`
		MarketRegime          string                        `json:"market_regime"`
		EntryMode             string                        `json:"entry_mode"`
		ExecutionQuality      string                        `json:"execution_quality,omitempty"`
		ExecutionPolicy       string                        `json:"execution_policy,omitempty"`
		DoNotOpenUntilConfirm bool                          `json:"do_not_open_until_confirmed,omitempty"`
		Confidence            string                        `json:"confidence"`
		RiskLevel             string                        `json:"risk_level"`
		AIPriority            float64                       `json:"ai_priority"`
		SetupScore            float64                       `json:"setup_score"`
		TimingScore           float64                       `json:"timing_score"`
		RegimeFitScore        float64                       `json:"regime_fit_score"`
		LiquidityScore        float64                       `json:"liquidity_score"`
		RiskScore             float64                       `json:"risk_score"`
		ReasonCodes           []string                      `json:"reason_codes"`
		RiskTags              []string                      `json:"risk_tags"`
		RequiredConfirmations []string                      `json:"required_confirmations"`
		TagSemantics          []local.HunterV7TagDefinition `json:"tag_semantics,omitempty"`
		EntryZone             local.V7PriceZone             `json:"entry_zone"`
		Invalidation          local.V7InvalidationRule      `json:"invalidation"`
		Targets               []local.V7Target              `json:"targets"`
		PriceContext          *local.V7PriceContext         `json:"price_context,omitempty"`
		DerivativesContext    *local.V7DerivativesContext   `json:"derivatives_context,omitempty"`
	}

	payload := v7SignalForAI{
		Symbol:                coin.Symbol,
		Direction:             coin.Direction,
		SetupType:             coin.V7SetupType,
		ExecutionTier:         coin.V7ExecutionTier,
		TierReason:            coin.V7TierReason,
		Status:                coin.V7Status,
		MarketRegime:          coin.V7MarketRegime,
		EntryMode:             coin.V7EntryMode,
		ExecutionQuality:      coin.V7ExecutionQuality,
		ExecutionPolicy:       hunterV7ExecutionPolicy(coin),
		DoNotOpenUntilConfirm: hunterV7DoNotOpenUntilConfirmed(coin),
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
		TagSemantics:          local.DescribeHunterV7Tags(coin.V7ReasonCodes, coin.V7RiskTags, coin.V7RequiredConfirms),
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

func hunterV7ExecutionPolicy(coin CandidateCoin) string {
	if coin.V7ExecutionTier == "EXECUTABLE" {
		return "open_allowed_after_core_checks"
	}
	if coin.V7ExecutionTier == "REVIEWABLE" {
		return "reviewable_open_allowed_only_if_live_confirmed"
	}
	if coin.V7ExecutionQuality == "watch_only" || containsStringValue(coin.V7RiskTags, "do_not_open_until_confirmed") {
		return "do_not_open_until_confirmed"
	}
	if coin.V7Status == "wait_confirm" || coin.V7Status == "conflict_watch" {
		return "wait_required_confirmations"
	}
	return ""
}

func hunterV7DoNotOpenUntilConfirmed(coin CandidateCoin) bool {
	if (coin.V7ExecutionTier == "EXECUTABLE" || coin.V7ExecutionTier == "REVIEWABLE") && !containsStringValue(coin.V7RiskTags, "do_not_open_until_confirmed") {
		return false
	}
	return coin.V7ExecutionQuality == "watch_only" || containsStringValue(coin.V7RiskTags, "do_not_open_until_confirmed")
}

func containsStringValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

	plannedRisk := ""
	plannedRiskParts := make([]string, 0, 2)
	if pos.StopLoss > 0 {
		plannedRiskParts = append(plannedRiskParts, fmt.Sprintf("Planned SL %.4f", pos.StopLoss))
	}
	if pos.TakeProfit > 0 {
		plannedRiskParts = append(plannedRiskParts, fmt.Sprintf("Planned TP %.4f", pos.TakeProfit))
	}
	if len(plannedRiskParts) > 0 {
		plannedRisk = " | " + strings.Join(plannedRiskParts, " | ")
	}

	sb.WriteString(fmt.Sprintf("%d. %s %s | Entry %.4f Current %.4f | Qty %.4f | Position Value %.2f USDT | PnL%+.2f%% | PnL Amount%+.2f USDT | Peak PnL%.2f%% | Leverage %dx | Margin %.0f | Liq Price %.4f%s%s\n\n",
		index, pos.Symbol, strings.ToUpper(pos.Side),
		pos.EntryPrice, pos.MarkPrice, pos.Quantity, positionValue, pos.UnrealizedPnLPct, pos.UnrealizedPnL, pos.PeakPnLPct,
		pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, plannedRisk, holdingDuration))

	if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
		if strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") {
			sb.WriteString("position_management_compact: use this summary for hold/close decisions; do not reassess existing positions as new entries.\n")
			sb.WriteString(formatHunterV7PositionProtectionHint(pos))
			sb.WriteString(e.formatCompactMarketData(marketData, nil))
		} else {
			sb.WriteString(e.formatMarketData(marketData))
		}

		if ctx.QuantDataMap != nil {
			if quantData, hasQuant := ctx.QuantDataMap[pos.Symbol]; hasQuant {
				sb.WriteString(e.formatQuantData(quantData))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatHunterV7PositionProtectionHint(pos PositionInfo) string {
	if pos.PeakPnLPct >= hunterV7ProtectionNearTP1PeakPnLPct {
		return fmt.Sprintf("protection_state=near_tp1_or_better peak_pnl=%.2f%% near_tp1_threshold=%.2f%%; peak giveback may be a trailing-exit signal if continuation is not explicit.\n",
			pos.PeakPnLPct, hunterV7ProtectionNearTP1PeakPnLPct)
	}
	return fmt.Sprintf("protection_state=pre_tp1 peak_pnl=%.2f%% below_near_tp1_threshold=%.2f%%; peak giveback alone is not a trailing-exit trigger. Close only on planned SL/hard invalidation or confirmed 5m+15m structural reversal.\n",
		pos.PeakPnLPct, hunterV7ProtectionNearTP1PeakPnLPct)
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

func (e *StrategyEngine) shouldCompactCandidatePrompt(coin CandidateCoin, ctx *Context) bool {
	mode := e.config.PromptCompactMode
	if mode == "" {
		mode = "current_source"
	}

	switch mode {
	case "off":
		return false
	case "current_source", "hunter_v7_only":
		return true
	case "all_candidates":
		return true
	case "auto":
		if coin.V7SetupType != "" {
			return true
		}
		klineCount := e.config.Indicators.Klines.PrimaryCount
		if klineCount <= 0 {
			klineCount = 30
		}
		timeframeCount := len(ctx.Timeframes)
		if timeframeCount == 0 {
			timeframeCount = len(e.config.Indicators.Klines.SelectedTimeframes)
		}
		if timeframeCount == 0 {
			timeframeCount = 1
		}
		return len(ctx.CandidateCoins)*timeframeCount*klineCount >= 600
	default:
		return true
	}
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

func (e *StrategyEngine) formatCompactMarketData(data *market.Data, coin *CandidateCoin) string {
	var sb strings.Builder
	indicators := e.config.Indicators

	sb.WriteString(fmt.Sprintf("=== %s compact market data ===\n", data.Symbol))
	parts := []string{fmt.Sprintf("price=%.6f", data.CurrentPrice)}
	if indicators.EnableEMA && data.CurrentEMA20 > 0 {
		parts = append(parts, fmt.Sprintf("ema20=%.6f", data.CurrentEMA20))
	}
	if indicators.EnableMACD {
		parts = append(parts, fmt.Sprintf("macd=%.4f", data.CurrentMACD))
	}
	if indicators.EnableRSI && data.CurrentRSI7 > 0 {
		parts = append(parts, fmt.Sprintf("rsi7=%.1f", data.CurrentRSI7))
	}
	if indicators.EnableOI && data.OpenInterest != nil {
		parts = append(parts, fmt.Sprintf("oi_latest=%.2f oi_avg=%.2f", data.OpenInterest.Latest, data.OpenInterest.Average))
	}
	if indicators.EnableFundingRate {
		parts = append(parts, fmt.Sprintf("funding=%.5f", data.FundingRate))
	}
	sb.WriteString(strings.Join(parts, " | "))
	sb.WriteString("\n")

	if len(data.TimeframeData) > 0 {
		timeframeOrder := []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d", "3d", "1w"}
		for _, tf := range timeframeOrder {
			if tfData, ok := data.TimeframeData[tf]; ok {
				if summary := compactTimeframeSummary(tfData, indicators); summary != "" {
					sb.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(tf), summary))
				}
			}
		}
	}
	if coin != nil && coin.V7SetupType != "" {
		if summary := e.formatHunterV7ExecutionCompact(data, coin); summary != "" {
			sb.WriteString(summary)
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

func (e *StrategyEngine) formatHunterV7ExecutionCompact(data *market.Data, coin *CandidateCoin) string {
	var sb strings.Builder
	price := data.CurrentPrice
	if price <= 0 && coin.V7PriceContext != nil {
		price = coin.V7PriceContext.Last
	}
	if price <= 0 {
		return ""
	}

	sb.WriteString("Hunter v7 execution compact: ")
	parts := []string{
		fmt.Sprintf("setup=%s", coin.V7SetupType),
		fmt.Sprintf("entry_mode=%s", coin.V7EntryMode),
		fmt.Sprintf("execution_quality=%s", coin.V7ExecutionQuality),
		fmt.Sprintf("confidence=%s", coin.V7Confidence),
		fmt.Sprintf("timing=%.0f", coin.V7TimingScore),
		fmt.Sprintf("risk=%.0f", coin.V7RiskScore),
	}
	if coin.V7EntryZone.Lower > 0 && coin.V7EntryZone.Upper > coin.V7EntryZone.Lower {
		pos := (price - coin.V7EntryZone.Lower) / (coin.V7EntryZone.Upper - coin.V7EntryZone.Lower) * 100
		parts = append(parts,
			fmt.Sprintf("entry_zone_pos=%.1f%%", pos),
			fmt.Sprintf("dist_zone_upper=%+.2f%%", pctMove(price, coin.V7EntryZone.Upper)),
			fmt.Sprintf("dist_zone_lower=%+.2f%%", pctMove(price, coin.V7EntryZone.Lower)),
			fmt.Sprintf("zone_location=%s", entryZoneLocation(pos)),
		)
	}
	if coin.V7Invalidation.Price > 0 {
		parts = append(parts, fmt.Sprintf("invalidation_dist=%+.2f%%", pctMove(price, coin.V7Invalidation.Price)))
	}
	if len(coin.V7Targets) > 0 && coin.V7Targets[0].Price > 0 {
		parts = append(parts, fmt.Sprintf("target1_dist=%+.2f%%", pctMove(price, coin.V7Targets[0].Price)))
	}
	if coin.V7DerivativesCtx != nil {
		parts = append(parts,
			fmt.Sprintf("oi_1h=%+.2f%%", coin.V7DerivativesCtx.OIChange1h),
			fmt.Sprintf("oi_4h=%+.2f%%", coin.V7DerivativesCtx.OIChange4h),
			fmt.Sprintf("taker15m=%.3f", coin.V7DerivativesCtx.TakerBuy15m),
		)
		if coin.V7SetupType == "funding_reversal" {
			parts = append(parts, fmt.Sprintf("oi_state=%s", fundingReversalOIState(coin.V7DerivativesCtx.OIChange1h, coin.V7DerivativesCtx.OIChange4h)))
		}
	}
	vwap15m := hunterV7CompactVWAP15m(data, coin)
	if tf15 := data.TimeframeData["15m"]; tf15 != nil {
		if tfSummary := executionTFCompact("15m", price, tf15, vwap15m); tfSummary != "" {
			parts = append(parts, tfSummary)
		}
	}
	if tf5 := data.TimeframeData["5m"]; tf5 != nil {
		if tfSummary := executionTFCompact("5m", price, tf5, 0); tfSummary != "" {
			parts = append(parts, tfSummary)
		}
	}
	if warning := hunterV7ExecutionWarning(price, coin); warning != "" {
		parts = append(parts, "warning="+warning)
	}
	if rule := hunterV7ExecutionHardRule(price, coin); rule != "" {
		parts = append(parts, "hard_rule="+rule)
	}
	if missing := hunterV7CompactMissingFields(data, coin); len(missing) > 0 {
		parts = append(parts,
			"compact_data_quality=partial",
			fmt.Sprintf("missing=%s", strings.Join(missing, ",")),
			"missing_fields_rule=wait_unless_all_required_confirmations_are_visible",
		)
	} else {
		parts = append(parts, "compact_data_quality=complete")
	}
	sb.WriteString(strings.Join(parts, " | "))
	sb.WriteString("\n")
	return sb.String()
}

func hunterV7CompactMissingFields(data *market.Data, coin *CandidateCoin) []string {
	if coin == nil {
		return nil
	}
	missing := make([]string, 0, 8)
	if coin.V7EntryZone.Lower <= 0 || coin.V7EntryZone.Upper <= coin.V7EntryZone.Lower {
		missing = append(missing, "entry_zone")
	}
	if coin.V7Invalidation.Price <= 0 {
		missing = append(missing, "invalidation")
	}
	if len(coin.V7Targets) == 0 || coin.V7Targets[0].Price <= 0 {
		missing = append(missing, "target1")
	}
	if coin.V7DerivativesCtx == nil {
		missing = append(missing, "derivatives_context")
	} else if coin.V7DerivativesCtx.TakerBuy15m <= 0 {
		missing = append(missing, "taker_buy_15m")
	}
	if data == nil {
		return append(missing, "market_data")
	}
	tf15 := data.TimeframeData["15m"]
	if tf15 == nil || len(tf15.Klines) == 0 {
		missing = append(missing, "15m_kline")
	} else {
		if tf15.ATR14 <= 0 {
			missing = append(missing, "15m_atr")
		}
		if _, ok := lastFloat(tf15.EMA20Values); !ok {
			missing = append(missing, "15m_ema20")
		}
	}
	if hunterV7RequiresVWAP(coin) && hunterV7CompactVWAP15m(data, coin) <= 0 {
		missing = append(missing, "15m_vwap")
	}
	tf5 := data.TimeframeData["5m"]
	if tf5 == nil || len(tf5.Klines) == 0 {
		missing = append(missing, "5m_kline")
	}
	return missing
}

func executionTFCompact(label string, price float64, data *market.TimeframeSeriesData, vwap float64) string {
	if data == nil || len(data.Klines) == 0 {
		return ""
	}
	last := data.Klines[len(data.Klines)-1]
	recent := lastNKlines(data.Klines, 3)
	hi, lo := highLow(recent)
	parts := []string{
		fmt.Sprintf("%s_recent_high3=%.6f", label, hi),
		fmt.Sprintf("%s_recent_low3=%.6f", label, lo),
	}
	if data.ATR14 > 0 {
		parts = append(parts, fmt.Sprintf("%s_atr_pct=%.2f%%", label, data.ATR14/price*100))
		parts = append(parts, fmt.Sprintf("%s_min_stop_0_8atr=%.2f%%", label, data.ATR14*0.8/price*100))
	}
	if ema20, ok := lastFloat(data.EMA20Values); ok && ema20 > 0 {
		parts = append(parts, fmt.Sprintf("%s_close_vs_ema20=%+.2f%%", label, pctMove(ema20, last.Close)))
	}
	if vwap > 0 {
		parts = append(parts,
			fmt.Sprintf("%s_vwap20=%.6f", label, vwap),
			fmt.Sprintf("%s_close_vs_vwap20=%+.2f%%", label, pctMove(vwap, last.Close)),
			fmt.Sprintf("%s_close_below_vwap20=%t", label, last.Close < vwap),
			fmt.Sprintf("%s_close_above_vwap20=%t", label, last.Close > vwap),
		)
	}
	if upper, ok := lastFloat(data.BOLLUpper); ok && upper > 0 {
		mid, _ := lastFloat(data.BOLLMiddle)
		lower, _ := lastFloat(data.BOLLLower)
		parts = append(parts, fmt.Sprintf("%s_close_vs_boll_mid=%+.2f%%", label, pctMove(mid, last.Close)))
		parts = append(parts, fmt.Sprintf("%s_close_vs_boll_lower=%+.2f%%", label, pctMove(lower, last.Close)))
		parts = append(parts, fmt.Sprintf("%s_close_vs_boll_upper=%+.2f%%", label, pctMove(upper, last.Close)))
	}
	if len(data.Klines) >= 6 {
		prev := data.Klines[:len(data.Klines)-1]
		prevRecent := lastNKlines(prev, 5)
		prevHi, prevLo := highLow(prevRecent)
		if prevHi > 0 {
			parts = append(parts, fmt.Sprintf("%s_no_new_high=%t", label, last.High <= prevHi))
		}
		if prevLo > 0 {
			parts = append(parts, fmt.Sprintf("%s_no_new_low=%t", label, last.Low >= prevLo))
		}
		if avgVol := averageVolume(prevRecent); avgVol > 0 {
			parts = append(parts, fmt.Sprintf("%s_vol_vs_avg5=%.2fx", label, last.Volume/avgVol))
		}
	}
	return strings.Join(parts, " ")
}

func hunterV7CompactVWAP15m(data *market.Data, coin *CandidateCoin) float64 {
	if coin != nil {
		if coin.V7VWAP15m > 0 {
			return coin.V7VWAP15m
		}
		if coin.V7PriceContext != nil && coin.V7PriceContext.VWAP15m > 0 {
			return coin.V7PriceContext.VWAP15m
		}
	}
	if data == nil {
		return 0
	}
	tf15 := data.TimeframeData["15m"]
	if tf15 == nil || len(tf15.Klines) == 0 {
		return 0
	}
	return vwapFromKlines(lastNKlines(tf15.Klines, 20))
}

func hunterV7RequiresVWAP(coin *CandidateCoin) bool {
	if coin == nil {
		return false
	}
	for _, confirm := range coin.V7RequiredConfirms {
		if strings.Contains(strings.ToLower(confirm), "vwap") {
			return true
		}
	}
	return strings.EqualFold(coin.V7SetupType, "funding_reversal")
}

func hunterV7ExecutionWarning(price float64, coin *CandidateCoin) string {
	if coin == nil {
		return ""
	}
	if coin.V7SetupType == "funding_reversal" && strings.EqualFold(coin.Direction, "SHORT") {
		warnings := make([]string, 0, 3)
		if strings.EqualFold(coin.V7Confidence, "C") && coin.V7AIPriority < 60 {
			warnings = append(warnings, "C_conf_ai_lt_60")
		}
		if coin.V7EntryZone.Lower > 0 && coin.V7EntryZone.Upper > coin.V7EntryZone.Lower {
			pos := (price - coin.V7EntryZone.Lower) / (coin.V7EntryZone.Upper - coin.V7EntryZone.Lower) * 100
			if pos <= 45 {
				warnings = append(warnings, "short_near_zone_lower")
			}
		}
		if coin.V7DerivativesCtx != nil && fundingReversalOIState(coin.V7DerivativesCtx.OIChange1h, coin.V7DerivativesCtx.OIChange4h) == "mixed" {
			warnings = append(warnings, "oi_mixed")
		}
		if coin.V7DerivativesCtx != nil && fundingReversalOIState(coin.V7DerivativesCtx.OIChange1h, coin.V7DerivativesCtx.OIChange4h) == "building" {
			warnings = append(warnings, "oi_building_no_flush")
		}
		if len(warnings) > 0 {
			return strings.Join(warnings, ",")
		}
	}
	return ""
}

func hunterV7ExecutionHardRule(price float64, coin *CandidateCoin) string {
	if coin == nil {
		return ""
	}
	if coin.V7SetupType != "funding_reversal" {
		return ""
	}
	rules := make([]string, 0, 4)
	oiState := ""
	if coin.V7DerivativesCtx != nil {
		oiState = fundingReversalOIState(coin.V7DerivativesCtx.OIChange1h, coin.V7DerivativesCtx.OIChange4h)
	}
	if strings.EqualFold(coin.Direction, "SHORT") {
		if oiState == "building" {
			rules = append(rules, "no_open_short_until_oi_flush_or_failed_rebuild")
		}
		if coin.V7PriceContext != nil && coin.V7PriceContext.Change1h < -5 && oiState != "flush" && oiState != "failed_rebuild_or_declining" {
			rules = append(rules, "no_chase_short_after_fast_drop_without_oi_flush")
		}
	}
	if strings.EqualFold(coin.Direction, "LONG") {
		if coin.V7PriceContext != nil && coin.V7PriceContext.Change1h > 5 && oiState != "flush" && oiState != "failed_rebuild_or_declining" {
			rules = append(rules, "no_chase_long_after_fast_pump_without_oi_reset")
		}
	}
	if len(rules) == 0 {
		return ""
	}
	return strings.Join(rules, ",") + "; output_wait_only"
}

func fundingReversalOIState(oi1h, oi4h float64) string {
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

func entryZoneLocation(pos float64) string {
	switch {
	case pos < 0:
		return "below_zone"
	case pos <= 35:
		return "zone_lower"
	case pos <= 65:
		return "zone_middle"
	case pos <= 100:
		return "zone_upper"
	default:
		return "above_zone"
	}
}

func pctMove(from, to float64) float64 {
	if from == 0 {
		return 0
	}
	return (to/from - 1) * 100
}

func lastNKlines(values []market.KlineBar, n int) []market.KlineBar {
	if len(values) <= n {
		return values
	}
	return values[len(values)-n:]
}

func highLow(values []market.KlineBar) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	hi := values[0].High
	lo := values[0].Low
	for _, v := range values[1:] {
		hi = math.Max(hi, v.High)
		lo = math.Min(lo, v.Low)
	}
	return hi, lo
}

func averageVolume(values []market.KlineBar) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v.Volume
	}
	return sum / float64(len(values))
}

func vwapFromKlines(values []market.KlineBar) float64 {
	if len(values) == 0 {
		return 0
	}
	var totalPV, totalVolume float64
	for _, v := range values {
		if v.Volume <= 0 {
			continue
		}
		typical := (v.High + v.Low + v.Close) / 3
		totalPV += typical * v.Volume
		totalVolume += v.Volume
	}
	if totalVolume <= 0 {
		return 0
	}
	return totalPV / totalVolume
}

func compactTimeframeSummary(data *market.TimeframeSeriesData, indicators store.IndicatorConfig) string {
	if data == nil {
		return ""
	}
	parts := make([]string, 0, 8)
	if len(data.Klines) > 0 {
		last := data.Klines[len(data.Klines)-1]
		parts = append(parts, fmt.Sprintf("last_o/h/l/c=%.6f/%.6f/%.6f/%.6f", last.Open, last.High, last.Low, last.Close))
		parts = append(parts, fmt.Sprintf("vol=%.2f", last.Volume))
		if len(data.Klines) >= 4 {
			prev := data.Klines[len(data.Klines)-4]
			if prev.Close != 0 {
				parts = append(parts, fmt.Sprintf("chg3=%+.2f%%", (last.Close/prev.Close-1)*100))
			}
		}
	} else if len(data.MidPrices) > 0 {
		last := data.MidPrices[len(data.MidPrices)-1]
		parts = append(parts, fmt.Sprintf("last=%.6f", last))
	}
	if indicators.EnableEMA {
		if v, ok := lastFloat(data.EMA20Values); ok {
			parts = append(parts, fmt.Sprintf("ema20=%.6f", v))
		}
		if v, ok := lastFloat(data.EMA50Values); ok {
			parts = append(parts, fmt.Sprintf("ema50=%.6f", v))
		}
	}
	if indicators.EnableMACD {
		if v, ok := lastFloat(data.MACDValues); ok {
			parts = append(parts, fmt.Sprintf("macd=%.4f", v))
		}
	}
	if indicators.EnableRSI {
		if v, ok := lastFloat(data.RSI7Values); ok {
			parts = append(parts, fmt.Sprintf("rsi7=%.1f", v))
		}
		if v, ok := lastFloat(data.RSI14Values); ok {
			parts = append(parts, fmt.Sprintf("rsi14=%.1f", v))
		}
	}
	if indicators.EnableATR && data.ATR14 > 0 {
		parts = append(parts, fmt.Sprintf("atr14=%.6f", data.ATR14))
	}
	if indicators.EnableBOLL {
		if upper, ok := lastFloat(data.BOLLUpper); ok {
			mid, _ := lastFloat(data.BOLLMiddle)
			lower, _ := lastFloat(data.BOLLLower)
			parts = append(parts, fmt.Sprintf("boll=%.6f/%.6f/%.6f", upper, mid, lower))
		}
	}
	return strings.Join(parts, " | ")
}

func lastFloat(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	return values[len(values)-1], true
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
