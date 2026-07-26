package kernel

import (
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
	hunterV7ProtectionTP1MinRawMovePct  = 1.0
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
		sb.WriteString(fmt.Sprintf("- Before any open, calculate effective_take_profit using the backend cap %.2f%% from executable/current price, then calculate effective_rr from that capped TP and your stop_loss. Do not justify an open with an uncapped far TP1. If effective_rr < %.2f, output wait with `blocked_reason_code=rr_insufficient`.\n",
			maxTPMovePct, riskControl.MinRiskRewardRatio))
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
		sb.WriteString("- Required when opening a Hunter v7 candidate: copy `hunter_v7_signal_json.signal_id` into `selected_hunter_v7_signal_id`, and include `selected_hunter_v7_tier`, `selected_hunter_v7_setup`, `effective_rr_after_cap`, and `signal_age_ms` when available.\n")
		sb.WriteString("- Required when waiting despite an EXECUTABLE/REVIEWABLE Hunter v7 candidate: include `blocked_signal_symbol` for the best blocked candidate.\n")
		sb.WriteString("- `trigger` (REQUIRED for Hunter v7 wait on `entry_trigger_near`, `entry_reclaim_wait`, or `entry_pullback_wait`): object with `trigger_price`, `required_close`, `expires_in_bars`, `action_if_triggered`. Use `hunter_v7_signal_json.suggested_trigger` when present instead of hiding the trigger in reasoning.\n")
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
		sb.WriteString("\n\n# Hunter v7 专业执行框架\n\n")
		sb.WriteString("## 1. 角色与目标\n")
		sb.WriteString("你不是通用技术分析助手，而是 Hunter v7 多形态执行员：把 `hunter_v7_signal_json` 的结构化信号转化为 open / wait / close。目标是在不突破后端硬风控的前提下，提高每轮轮询的可开仓信号发现率；开仓率来自更好的候选分流、触发识别和 RR 复核，不来自低流动性、无确认追价或忽略 reject_only/wait_only。\n\n")
		sb.WriteString("## 2. 候选漏斗\n")
		sb.WriteString("按 EXECUTABLE → REVIEWABLE → WATCH → REJECTED 处理。EXECUTABLE/REVIEWABLE 必须完整审验；WATCH 只提供下一轮触发背景；REJECTED 禁止开仓。无持仓且存在开仓复核候选时，必须在最佳 open 与一个明确 blocked_reason 之间二选一，不得用市场情绪、账户回撤或 WATCH 缺失整体否决。若 Tier Summary 没有 EXECUTABLE/REVIEWABLE，才允许 `no_reviewable_candidate`。\n")
		sb.WriteString(fmt.Sprintf("后端几何边界：允许价格漂移 %.2f%%、最小止损 %.2f%%、有效 RR %.2f、TP cap %.2f%%。开仓 stop_loss 不要只贴最低止损；优先留出 drift buffer，除非结构止损和 RR 明确通过。\n\n",
			maxDriftPct, minSLMovePct, minRR, maxTPMovePct))
		sb.WriteString(fmt.Sprintf("开仓前必须先把 take_profit 视为后端有效 TP：long=min(你的TP, price*(1+%.2f%%))，short=max(你的TP, price*(1-%.2f%%))；再用该 effective_take_profit 重算 effective_rr。若 capped 后 effective_rr < %.2f，必须 wait + `blocked_reason_code=rr_insufficient`，不得引用未封顶远端 TP1 作为开仓理由。\n",
			maxTPMovePct, maxTPMovePct, minRR))
		sb.WriteString(fmt.Sprintf("若 execution_tier=EXECUTABLE 且 hunter_v7_signal_json.confirmation_summary.passed_review=true，并且 confirmation_summary.rr/effective_rr 已达到 %.2f，默认视为后端 RR 已验证；不得重新臆造更严格的结构 RR。只有明确列出当前可执行价、后端 capped effective_take_profit、stop_loss 后仍低于 %.2f，才允许 wait + `blocked_reason_code=rr_insufficient`。\n",
			minRR, minRR))
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
		sb.WriteString("whale_flow_reversal LONG 不允许追 entry_zone 上半区；若 entry_zone_position >45%，必须 wait 或选择下一个合格 EXECUTABLE/REVIEWABLE 候选，避免触发后端入场区保护。\n")
		sb.WriteString("range_expansion_event SHORT 是高波动事件追空，必须防反弹：若 15m close 低于 EMA20 超过 10%、entry_zone_position >80%、实时价相对决策价已反弹超过 0.30%，或 required confirmation 不是明确的 below VWAP/EMA20/entry_zone_lower，必须 wait + `blocked_reason_code=backend_guard_risk` 或 `confirmation_missing`。\n")
		sb.WriteString("open_short 绝对不得把 `15m close above VWAP/EMA20`、`close above VWAP` 或 `close above EMA20` 解释为 SHORT 确认通过；出现这种方向语义冲突时必须 wait + `blocked_reason_code=confirmation_missing`。\n")
		sb.WriteString("EXECUTABLE 的 required_confirmations 必须在 confirmation_summary 中明确通过。REVIEWABLE 若只缺现场可复核确认（短周期收盘、taker flow、momentum_not_exhausted），必须用最新 K线/资金流/盘口确认后才可 open；否则 wait + `blocked_reason_code=confirmation_missing`。\n")
		sb.WriteString("不得把 required_confirmations 写成“留给 LLM/context 交叉确认”；若 confirmation_summary.context_checks 里出现 required confirmation，视为未验证，不允许 open。\n")
		sb.WriteString("高波动山寨开仓的 take_profit 必须使用 5m-30m 内最近有效 TP（TP0/近端 TP1），不得为了满足 RR 引用远端 TP1/TP2；若最近有效 TP capped 后 RR 不足，必须 wait。\n")
		sb.WriteString("凡 signal risk_tags 含 regime_against_direction 且 hunter_v7_signal_json.confirmation_summary.passed_review=false，必须 wait；这是逆势开仓的统一硬边界，不得用高 priority、单个 5m EMA 穿越、taker 强或 RR 足够覆盖失败确认。\n")
		sb.WriteString("panic_reversal_long 在 trend_down/逆势且 24h 深跌、4h 仍下行时，5m close 仅轻微站上 EMA20 或 entry_zone_mid 不足以开仓；必须等待更强结构确认（优先 15m 收复 EMA20/VWAP 或连续 5m 站稳）和 taker_buy_15m 明显增强，否则输出 confirmation_missing。\n")
		sb.WriteString("panic_reversal_long 若处于 trend_down/逆势且 hunter_v7_signal_json.confirmation_summary.passed_review=false，必须 wait；不得把单个 5m EMA 穿越、taker_buy 强或 RR 足够当作覆盖该失败确认的开仓理由。\n")
		sb.WriteString("若 signal risk_tags 含 already_pumped_24h、funding_expensive、lsr_extreme_long、taker_sell_during_accumulation、no_reclaim_signal、oi_up_price_down、late_*_without_flush 或 not_near_*_zone，这些是 wait-only 风险语义；不得把高 priority/score 当作开仓理由覆盖这些标签。\n")
		sb.WriteString("持仓管理：保护器以杠杆后 ROE 为准。Peak PnL >=5% 后，盈亏平衡/亏损优先视为退出或减风险信号；Peak >=10% 后若从峰值回撤约 50%，Peak >=15% 后若回撤约 45%，除非 5m/15m 与资金流同时给出明确延续证据，否则输出可执行的 `close_long`/`close_short`。当前 PnL >=8% 可接受 TP0 减仓 30%-50% 并推保本；开仓后前 15 分钟 PnL <=-8% 或任意时刻 PnL <=-12% 是硬止损/失效信号。不得只因未到原始 SL/TP 而 `hold`，也不得用 `hold` 声称已经收紧止损。\n")
		sb.WriteString("## 3. 多形态交易剧本\n")
		sb.WriteString("- trend_breakout_long / accumulation_breakout_long：只做 confirmed breakout、trigger_memory_confirmed 或 clean pullback continuation。`entry_trigger_near` 只是接近触发，不是开仓；未出现 5m/15m close through trigger 时 wait + `blocked_reason_code=confirmation_missing` + `trigger`。\n")
		sb.WriteString("- displacement_momentum_long：只做位移初段或回踩延续，不追末端。`displacement_rr_insufficient` 与 `displacement_rr_repaired_needs_review` 只能 REVIEWABLE；必须重算 backend-capped effective_rr>=1.5、VWAP/尾随支撑守住、taker_buy_15m>0.52、OI 或成交量扩张。若 `displacement_chase_risk_overextended` 存在，优先等回踩，不得市场价追高。\n")
		sb.WriteString("- leader_momentum_long / short_squeeze_long：只做 clean momentum。需要强 4h/24h 相对强势、taker_sustained_buy 或 taker_buy_aggressive、OI healthy growth，或中下沿浅回踩守住。weak upper-zone pullbacks 不是优质入场；no_pullback_still_running + upper-zone + OI/量能/VWAP/RSI 任两项弱化时，视为顶部追单风险，必须 wait。\n")
		sb.WriteString("- alt_ladder_momentum_long / alt_ladder_breakdown_short：专门处理非核心高振幅山寨从 5%→15%→30% 的阶梯行情。Stage early/mid 可复核顺势；Stage late 或 `alt_ladder_late_*_risk` 只有在 1h OI 新增、成交量放大且 live 仍在 entry zone 时才可复核，否则必须 wait；`alt_ladder_stage_extreme` / `alt_ladder_extreme_continuation_watch` 只跟踪 EMA25/VWAP 回踩或重新放量，不得现价追多。必须有结构位 + taker + OI/volume 至少两项同向，不能只因涨跌幅大而 open。\n")
		sb.WriteString("- breakdown_momentum_short：捕捉下跌动量/破位延续机会，不要求 funding/LSR 极端。需要 1h 或 15m 下跌动量、价格跌破 VWAP/EMA/15m 中轨、taker_buy_15m<=0.50、OI 或成交量至少一项确认，并且不能贴近日内/日线支撑末端追空。\n")
		sb.WriteString("- mms_bottom_wake_long：暗涌底吸/横盘吸筹，只做小盘代理通过、72h 压缩、1h 放量、4h OI 潜伏增持的 REVIEWABLE；未 5m/15m 突破确认时不得 open。\n")
		sb.WriteString("- mms_trend_ride_long：主升生命线缩量回踩，只做 15m EMA7>EMA25>EMA99、低点触 EMA25 且收回、缩量回踩、taker 未反向的顺势多；必须在 entry zone 内且至少一项 1h/4h 动量未转弱，否则视为 wait 或 review，不得直接开仓；远离 EMA25 变 chase_risk。\n")
		sb.WriteString("- mms_squeeze_engine_long：顺势轧空，不是摸顶信号。Top LSR 高、价格与 OI 同涨、买盘/量能确认时可复核追多；`mms_do_not_short_squeeze` 或 `mms_short_ban_active` 出现时，同 symbol 禁止开空。\n")
		sb.WriteString("- panic_reversal_long：只做恐慌后的 reclaim 修复，不接下跌中的刀。需要 reclaim、selling exhaustion/deceleration、taker_buy_15m>=0.52、OI flush/declining、no_new_low 和 RR 通过；若 trend_down/逆势且 confirmation_summary.passed_review=false，必须 wait。\n")
		sb.WriteString("- funding_reversal SHORT：只做多头拥挤后的近端反转，不追深跌。需要 funding/LSR 拥挤、taker_buy_15m<0.48、retest failure/no_new_high、OI flush/mixed 与 RR 通过。funding_reversal LONG 必须有强 reclaim 与 15m 收复，不能和 SHORT 对称放宽。\n\n")
		sb.WriteString("## 4. 开仓审验顺序\n")
		sb.WriteString("逐个候选按顺序检查：market_shape 是否匹配 setup_type；entry_signal 是否已触发；当前价是否在 entry_zone/触发容差/回踩确认区；required_confirmations 是否通过；OI/taker/volume 至少两项同向；stop_loss/take_profit/RR/price drift 能否通过后端；同 symbol 冷却和最近亏损是否允许。`entry_open_now` 只表示进入审验，不等于必须开；`entry_rr_repairable` 必须重算当前 RR；`entry_chase_risk` 默认 wait，只有回踩支撑和 flow 未反转时才可小仓。\n\n")
		sb.WriteString("短周期保护：若 `hunter_v7_signal_json.tp0_price` 存在，开仓时把它作为第一保护目标；到 TP0 后减仓 30%-50%，并把剩余仓位止损移动到 breakeven 或 5m EMA20/VWAP trailing。若 `position_size_hint=small_size_or_wait_pullback`，只能小仓或等回踩；若 `position_size_hint=wait_pullback_price_too_far_from_entry_zone`，不得现价开仓。\n\n")
		sb.WriteString("几何修复纪律：若 EXECUTABLE 候选的唯一 blocker 是 stop_loss 距离小于最小值，且结构失效位未被击穿、confirmation_summary 已通过、TP cap 内仍能满足 RR，则不要直接 wait；把 stop_loss 调整到至少最小止损+0.10% 的合法方向，按 RR 重算 take_profit，并用较小 position_size_usd 开仓。若调整后 RR、TP cap、price drift 或确认任一不通过，才输出 `backend_guard_risk`。\n\n")
		sb.WriteString("## 5. 输出纪律与持仓管理\n")
		sb.WriteString("wait 必须输出 `blocked_reason_code`：`entry_not_in_zone`、`rr_insufficient`、`confirmation_missing`、`oi_too_low`、`funding_crowded`、`account_risk`、`backend_guard_risk`、`no_reviewable_candidate`。`trigger` 必须用于 `entry_trigger_near`、`entry_reclaim_wait`、`entry_pullback_wait`。`hold` 和 `wait` 不会修改 SL/TP；需要减风险时输出 `close_long`/`close_short`。\n")
		sb.WriteString("持仓管理：若 Peak PnL reached protection near-TP1（保护 TP1 6%，near-TP1 为 Peak PnL >=5.7%，且 raw_move >=1.0%），随后 gives back >=45% from the peak 或当前 PnL 回到盈亏平衡/亏损，优先输出风险降低 close；不得只因未到 SL/TP 而 hold，也不得 do not use `hold` to claim stop tightening。若错过 near-TP1 后回到 TP1 的 90% 以上，是 second protection chance。若仍是 pre-TP1/micro-profit noise，close only on planned SL/hard invalidation or when both 5m and 15m confirm structural reversal；crossing from a positive peak to negative PnL is >100% giveback，但微利状态不能只靠比例退出。\n")
		return
	}
	sb.WriteString("\n\n# Hunter v7 Execution Rules\n\n")
	sb.WriteString("## 1. Role And Objective\n")
	sb.WriteString("You are the Hunter v7 execution operator, not a generic chart analyst. Convert structured `hunter_v7_signal_json` into open / wait / close. Improve per-poll openable-signal discovery through better funneling, trigger recognition, and RR revalidation, not by bypassing risk, trading low-liquidity names, or overriding reject_only/wait_only semantics.\n\n")
	sb.WriteString("## 2. Candidate Funnel\n")
	sb.WriteString("Evaluate EXECUTABLE → REVIEWABLE → WATCH → REJECTED. Fully judge every EXECUTABLE/REVIEWABLE candidate; WATCH is context only and REJECTED is forbidden. If flat and an open-review candidate exists, choose the best open or provide one precise blocked_reason. Use `no_reviewable_candidate` only when Tier Summary has EXECUTABLE=0 and REVIEWABLE=0.\n")
	sb.WriteString(fmt.Sprintf("Backend geometry: allowed drift %.2f%%, minimum stop %.2f%%, effective RR %.2f, TP cap %.2f%%. Do not place stop_loss barely above the minimum; keep a drift buffer when structure and RR allow.\n\n",
		maxDriftPct, minSLMovePct, minRR, maxTPMovePct))
	sb.WriteString(fmt.Sprintf("Before opening, treat take_profit as the backend-effective TP: long=min(your TP, price*(1+%.2f%%)), short=max(your TP, price*(1-%.2f%%)); recalculate effective_rr from that capped TP and stop_loss. If capped effective_rr < %.2f, wait with `blocked_reason_code=rr_insufficient`; do not cite an uncapped far TP1 as the open rationale.\n",
		maxTPMovePct, maxTPMovePct, minRR))
	sb.WriteString(fmt.Sprintf("If execution_tier=EXECUTABLE, hunter_v7_signal_json.confirmation_summary.passed_review=true, and confirmation_summary.rr/effective_rr is already >= %.2f, treat backend RR as validated; do not invent a stricter structural RR. Use `rr_insufficient` only when you explicitly recompute from executable price, backend-capped effective_take_profit, and stop_loss and it is still below %.2f.\n",
		minRR, minRR))
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
	sb.WriteString("For whale_flow_reversal LONG, do not chase the upper half of entry_zone; if entry_zone_position is >45%, wait or select the next valid EXECUTABLE/REVIEWABLE candidate to avoid the backend entry-zone guard.\n")
	sb.WriteString("range_expansion_event SHORT is a high-volatility chase-short setup. If 15m close is more than 10% below EMA20, entry_zone_position is >80%, live price has rebounded more than 0.30% above the decision price, or required confirmation is not explicitly below VWAP/EMA20/entry_zone_lower, wait with `blocked_reason_code=backend_guard_risk` or `confirmation_missing`.\n")
	sb.WriteString("For open_short, NEVER treat `15m close above VWAP/EMA20`, `close above VWAP`, or `close above EMA20` as passed SHORT confirmation. That direction conflict must wait with `blocked_reason_code=confirmation_missing`.\n")
	sb.WriteString("For EXECUTABLE candidates, every required_confirmation must be explicitly passed in confirmation_summary. For REVIEWABLE candidates with only live-checkable gaps (short-term candle close, taker flow, momentum_not_exhausted), open only after latest candle/flow/orderbook confirmation; otherwise wait with `blocked_reason_code=confirmation_missing`.\n")
	sb.WriteString("Do not describe required_confirmations as left to LLM/context cross-checking; if confirmation_summary.context_checks contains a required confirmation, treat it as context-only/unverified and do not open unless it is a REVIEWABLE live-checkable gap confirmed by fresh data.\n")
	sb.WriteString("For high-volatility altcoins, take_profit must be the nearest effective 5m-30m target (TP0/near TP1). Do not cite far TP1/TP2 only to satisfy RR. If the nearest effective capped TP does not pass RR, wait.\n")
	sb.WriteString("For any signal with risk_tags containing regime_against_direction and hunter_v7_signal_json.confirmation_summary.passed_review=false, wait. This is the unified counter-trend open boundary; do not override the failed confirmation with high priority, one 5m EMA reclaim, strong taker flow, or acceptable RR.\n")
	sb.WriteString("For panic_reversal_long in trend_down/counter-trend with a deep 24h dump and still-negative 4h structure, a small 5m close above EMA20 or entry_zone_mid is not enough to open; require stronger structure confirmation, preferably a 15m reclaim of EMA20/VWAP or consecutive 5m holds, plus clearly stronger taker_buy_15m. Otherwise output confirmation_missing.\n")
	sb.WriteString("For panic_reversal_long in trend_down/counter-trend, if hunter_v7_signal_json.confirmation_summary.passed_review=false, wait; do not override that failed confirmation with a single 5m EMA reclaim, strong taker buy, or acceptable RR.\n")
	sb.WriteString("If signal risk_tags include already_pumped_24h, funding_expensive, lsr_extreme_long, taker_sell_during_accumulation, no_reclaim_signal, oi_up_price_down, late_*_without_flush, or not_near_*_zone, those are wait-only risk semantics; do not override them with high priority/score.\n")
	sb.WriteString("Position management: the protector uses leveraged ROE. Once Peak PnL >=5%, breakeven/loss is first an exit or risk-reduction signal. Once Peak >=10%, about 50% giveback from peak is a protection close signal; once Peak >=15%, about 45% giveback is a protection close signal unless 5m/15m structure and flow both give explicit continuation evidence. Current PnL >=8% can justify TP0 partial close of 30%-50% and moving the stop to breakeven. In the first 15 minutes, PnL <=-8% is a hard invalidation signal; at any age, PnL <=-12% is hard stop territory. Do not `hold` only because original SL/TP has not been reached, and do not use `hold` to claim stop tightening.\n")
	sb.WriteString("## 3. Multi-Setup Playbooks\n")
	sb.WriteString("- trend_breakout_long / accumulation_breakout_long: trade only confirmed breakout, trigger_memory_confirmed, or clean pullback continuation. `entry_trigger_near` is not an entry; without a 5m/15m close through trigger, wait with `blocked_reason_code=confirmation_missing` and include `trigger`.\n")
	sb.WriteString("- displacement_momentum_long: trade early displacement or supported pullback continuation, not late chase. `displacement_rr_insufficient` and `displacement_rr_repaired_needs_review` are REVIEWABLE only; require backend-capped effective_rr>=1.5, VWAP/trailing support hold, taker_buy_15m>0.52, and OI/volume expansion. If displacement_chase_risk_overextended is present, prefer pullback confirmation.\n")
	sb.WriteString("- leader_momentum_long / short_squeeze_long: require clean momentum, strong 4h/24h relative strength, sustained/aggressive taker buy, healthy OI growth, or a mid/lower-zone shallow pullback. weak upper-zone pullbacks are not quality entries; no_pullback_still_running + upper-zone + any two weak OI/volume/VWAP/RSI conditions is top-chase risk and must wait.\n")
	sb.WriteString("- alt_ladder_momentum_long / alt_ladder_breakdown_short: specialized high-amplitude non-core alt lifecycle routes from 5%→15%→30%. Stage early/mid can be reviewed with trend; Stage late or `alt_ladder_late_*_risk` requires reduced size or a pullback/retest, and only becomes executable again when 1h OI/volume shows fresh continuation; `alt_ladder_stage_extreme` / `alt_ladder_extreme_continuation_watch` tracks EMA25/VWAP pullback or renewed volume only and never permits current-price chasing. Require structure + taker + OI/volume alignment; never open from move size alone.\n")
	sb.WriteString("- breakdown_momentum_short: capture downside momentum / breakdown continuation without requiring extreme funding or LSR. Require 1h or 15m downside impulse, price below VWAP/EMA/15m mid band, taker_buy_15m<=0.50, at least one OI or volume confirmation, and avoid chasing into daily support after an exhausted crash.\n")
	sb.WriteString("- mms_bottom_wake_long: quiet small-cap accumulation. REVIEWABLE only until a 5m/15m breakout/reclaim confirms; require 72h compression, 1h volume wake, 4h OI stealth inflow, and no active sell pressure.\n")
	sb.WriteString("- mms_trend_ride_long: trend-life-line retest. Require 15m EMA7>EMA25>EMA99, low touches EMA25 and close reclaims, low-volume retest, and taker flow not against long. If far above EMA25 or 1h/4h momentum both turn negative, treat as wait or review instead of open.\n")
	sb.WriteString("- mms_squeeze_engine_long: squeeze-fuel continuation, not a top-short signal. Top LSR high, price and OI rising together, and buy/volume confirmation allow long review; `mms_do_not_short_squeeze` or `mms_short_ban_active` blocks same-symbol shorts.\n")
	sb.WriteString("- panic_reversal_long: trade reclaim repair after capitulation, not falling knives. Require reclaim, selling exhaustion/deceleration, taker_buy_15m>=0.52, OI flush/decline, no_new_low, and RR. If counter-trend and confirmation_summary.passed_review=false, wait.\n")
	sb.WriteString("- funding_reversal SHORT: trade crowded-long reversal near retest, not late deep drops. Require funding/LSR crowding, taker_buy_15m<0.48, retest failure/no_new_high, OI flush/mixed, and RR. funding_reversal LONG needs strong reclaim and 15m recovery; do not mirror SHORT loosely.\n\n")
	sb.WriteString("## 4. Open Review Order\n")
	sb.WriteString("For every EXECUTABLE/REVIEWABLE candidate, check in order: market_shape matches setup_type; entry_signal is triggered; current price is in entry_zone / trigger tolerance / pullback confirmation zone; required_confirmations pass; at least two of OI/taker/volume align; stop_loss/take_profit/RR/price drift pass backend guards; same-symbol cooldown and recent losses allow a new open. `entry_open_now` means review now, not must-open. `entry_rr_repairable` requires current RR recalculation. `entry_chase_risk` defaults to wait unless pullback support holds and flow has not reversed.\n\n")
	sb.WriteString("Short-cycle protection: if `hunter_v7_signal_json.tp0_price` exists, use it as the first protection target; after TP0 reduce 30%-50% and move the remaining stop to breakeven or 5m EMA20/VWAP trailing. If `position_size_hint=small_size_or_wait_pullback`, use reduced size or wait; if `position_size_hint=wait_pullback_price_too_far_from_entry_zone`, do not open at market.\n\n")
	sb.WriteString("Geometry repair discipline: when an EXECUTABLE candidate's only blocker is stop_loss distance below the minimum, and the structural invalidation is not already broken, confirmation_summary has passed, and RR still passes within the TP cap, do not immediately wait. Move stop_loss to at least minimum stop + 0.10% in the valid direction, recalculate take_profit from RR, and open with a smaller position_size_usd. Output `backend_guard_risk` only if the repaired geometry fails RR, TP cap, price drift, or confirmation checks.\n\n")
	sb.WriteString("## 5. Output Discipline And Position Management\n")
	sb.WriteString("Wait decisions MUST include one `blocked_reason_code`: `entry_not_in_zone`, `rr_insufficient`, `confirmation_missing`, `oi_too_low`, `funding_crowded`, `account_risk`, `backend_guard_risk`, `no_reviewable_candidate`. `trigger` is required for `entry_trigger_near`, `entry_reclaim_wait`, or `entry_pullback_wait`. `hold` and `wait` do not change SL/TP; output `close_long`/`close_short` when risk reduction is needed.\n")
	sb.WriteString("Position management: if Peak PnL reached protection near-TP1 (protection TP1 is 6%; near-TP1 means Peak PnL >=5.7% AND raw_move >=1.0%) and then gives back >=45% from the peak or crosses to breakeven/loss, treat it first as an exit/risk-reduction signal. Do not `hold` only because SL/TP has not been reached, and do not use `hold` to claim stop tightening. A later return above 90% of TP1 is a second protection chance only when raw_move >=1.0%. If Peak PnL is below 5.7% or raw_move <1.0%, it is pre-TP1/micro-profit noise: close only on planned SL/hard invalidation or when both 5m and 15m confirm structural reversal. crossing from a positive peak to negative PnL is >100% giveback, but micro-profit state cannot exit from that ratio alone.\n")
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
	geometry := e.hunterV7ExecutionGeometry()
	return geometry.MaxTPMovePct, geometry.MinSLMovePct, geometry.MaxDriftPct
}

func (e *StrategyEngine) hunterV7ExecutionGeometry() HunterV7ExecutionGeometry {
	if e == nil {
		return HunterV7ExecutionGeometry{}
	}
	riskControl := e.config.RiskControl
	return HunterV7EffectiveExecutionGeometry(
		riskControl.MaxTakeProfitPriceMovePct,
		riskControl.MinStopLossPriceMovePct,
		riskControl.MaxEntryPriceDeviationPct,
		riskControl.MinRiskRewardRatio,
		strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7"),
	)
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
	geometry := e.hunterV7ExecutionGeometry()
	for _, coin := range ctx.CandidateCoins {
		if positionSymbols[market.Normalize(coin.Symbol)] {
			continue
		}
		data, ok := ctx.MarketDataMap[coin.Symbol]
		if !ok {
			continue
		}
		coin = hunterV7CandidateWithLiveMarketPrice(coin, data)
		// Settle live-confirmable required confirmations before tiering, so a
		// candidate whose only gap was "nobody has looked at the latest close
		// yet" is not parked in REVIEWABLE until after the LLM has seen it.
		coin = hunterV7ApplyLiveConfirmations(coin, data)
		tier, reason := classifyHunterV7CandidateTierWithGeometry(coin, geometry)
		coin.V7ExecutionTier = tier
		coin.V7TierReason = reason
		readiness := hunterV7PromptExecutionReadiness(coin, data, tier, reason)
		coin.V7Readiness = &readiness
		tier, reason = hunterV7TierFromPromptReadiness(coin, tier, reason, readiness)
		coin.V7ExecutionTier = tier
		coin.V7TierReason = reason
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
		ri := 0.0
		rj := 0.0
		if items[i].Coin.V7Readiness != nil {
			ri = items[i].Coin.V7Readiness.ReadyScore
		}
		if items[j].Coin.V7Readiness != nil {
			rj = items[j].Coin.V7Readiness.ReadyScore
		}
		if ri != rj {
			return ri > rj
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
			sb.WriteString("2. REVIEWABLE 可复核：EXECUTABLE 全部被 blocked 后才评估 REVIEWABLE。REVIEWABLE 允许存在短周期收盘/资金流/momentum 等现场确认缺口；只有最新 K线、资金流、盘口、入场区、近端止损和 RR 均确认后才允许 open，否则给出 blocked_reason_code。\n")
			sb.WriteString("3. WATCH 只做背景：WATCH 候选不参与开仓决策，只提供市场背景。禁止把 WATCH 候选的整体缺失作为对所有 EXECUTABLE/REVIEWABLE 的全局 wait。\n")
			sb.WriteString("4. REJECTED 禁止：REJECTED 候选不得参与任何开仓判断。\n")
			sb.WriteString("5. 动量/位移/放量突破/区间扩张/鲸鱼流反转开仓后必须执行 TP0 分批止盈语义：TP0 减仓 30%-50%，止损推保本，并用 5m EMA20、15m VWAP 或 0.8-1.2 ATR15m 跟踪剩余仓位；TP0 不等同于 targets[0]，targets[0] 仍是 TP1。当前 JSON 只有单个 `take_profit` 字段时，必须输出“后端 cap 后仍满足 RR 的最近有效 TP”，不得输出会被 cap 后 RR 不足的远端 TP1；若近端有效 TP 不存在则 wait。\n")
			sb.WriteString("6. 若 execution_tier=EXECUTABLE 且 data_quality=complete_for_execution，`regime_against_direction` 不能作为唯一 wait/降级理由；确认通过时仅降低仓位，确认失败仍按 confirmation_missing wait。\n")
			sb.WriteString("7. EXECUTABLE 且 confirmation_summary.passed_review=true、confirmation_summary.rr/effective_rr 已达标时，优先给出保守仓位 open；不得用未列明计算过程的 `rr_insufficient` 覆盖后端已验证 RR。\n")
			sb.WriteString("8. whale_flow_reversal LONG 若 entry_zone_position >45%，该候选会被后端保护拦截；必须 wait 或继续评估下一个候选。\n")
			sb.WriteString("9. range_expansion_event SHORT 若 15m close 极端低于 EMA20、entry_zone_position >80%、实时价已反弹，或把 above VWAP/EMA20 当作 SHORT 确认，必须 wait；这是后端硬保护。\n")
			sb.WriteString("10. open_long/open_short 必须从候选的 hunter_v7_signal_json.signal_id 复制到 `selected_hunter_v7_signal_id`；后端会按 signal_id/symbol/direction/setup/tier 做结构化匹配。\n")
			sb.WriteString("wait 决策必须输出 `blocked_reason_code`（枚举值），不得用自然语言 reasoning 代替。账户回撤只影响仓位/冷却，不得作为所有候选的全局 wait 否决。\n\n")
		} else {
			sb.WriteString("Decision policy (strict tier funnel):\n")
			sb.WriteString("1. EXECUTABLE first: evaluate ALL EXECUTABLE candidates first. If a candidate passes hard risk controls, setup core confirmation, and RR, output open; otherwise provide a precise `blocked_reason_code`.\n")
			sb.WriteString("2. REVIEWABLE next: only after all EXECUTABLE candidates are blocked, evaluate REVIEWABLE. REVIEWABLE may have live-checkable gaps such as short-term candle close, taker flow, or momentum; open only when latest candles, flow, orderbook, entry zone, near structural stop, and RR confirm; otherwise provide `blocked_reason_code`.\n")
			sb.WriteString("3. WATCH is context only: WATCH candidates do not participate in open decisions. Do not use the absence of WATCH candidates as a global wait veto on EXECUTABLE/REVIEWABLE.\n")
			sb.WriteString("4. REJECTED is forbidden: REJECTED candidates must not influence any open decision.\n")
			sb.WriteString("5. For momentum/displacement/range-expansion/breakout/whale-flow reversal opens, use TP0 partial-profit semantics: reduce 30%-50% at TP0, move stop to breakeven, then trail with 5m EMA20, 15m VWAP, or 0.8-1.2 ATR15m. TP0 is not targets[0]; targets[0] remains TP1. Because the current JSON schema has one `take_profit` field, output the nearest effective TP that still passes RR after backend cap; do not output a far TP1 that becomes RR-insufficient after capping. If no near effective TP passes, wait.\n")
			sb.WriteString("6. If execution_tier=EXECUTABLE and data_quality=complete_for_execution, `regime_against_direction` alone must not downgrade to wait; reduce size when confirmation passed, but still wait when confirmation_summary.passed_review=false.\n")
			sb.WriteString("7. When an EXECUTABLE candidate has confirmation_summary.passed_review=true and confirmation_summary.rr/effective_rr already passes, prefer a conservative open; do not override backend-validated RR with `rr_insufficient` unless your explicit recomputation shows it fails.\n")
			sb.WriteString("8. A whale_flow_reversal LONG with entry_zone_position >45% will be rejected by the backend guard; wait or continue to the next candidate.\n")
			sb.WriteString("9. A range_expansion_event SHORT must wait if 15m close is extremely below EMA20, entry_zone_position is >80%, live price has rebounded, or the reasoning treats above VWAP/EMA20 as SHORT confirmation; this is backend-enforced.\n")
			sb.WriteString("10. For open_long/open_short, copy hunter_v7_signal_json.signal_id into `selected_hunter_v7_signal_id`; the backend will match signal_id/symbol/direction/setup/tier structurally.\n")
			sb.WriteString("Wait decisions MUST include `blocked_reason_code` (enum field); free-text reasoning is not a substitute. Account drawdown affects sizing/cooldown only, not a global wait veto for all candidates.\n\n")
		}
	} else {
		if e.GetLanguage() == LangChinese {
			sb.WriteString("Decision policy: 当前没有 EXECUTABLE/REVIEWABLE 候选。不要强行开仓；只允许 wait 或管理已有持仓。\n\n")
		} else {
			sb.WriteString("Decision policy: No EXECUTABLE/REVIEWABLE candidates are available. Do not force a new open; only wait or manage existing positions.\n\n")
		}
	}

	openReviewLimit := hunterV7OpenReviewExpansionLimit(execCount+reviewableCount, len(ctx.Positions), e.config.RiskControl.MaxPositions)
	expanded := hunterV7SelectExpandedOpenReview(items, openReviewLimit)
	sb.WriteString(fmt.Sprintf("### Open-review candidates (full context, max %d)\n\n", openReviewLimit))
	displayedOpenReview := 0
	if positionLimitReached {
		sb.WriteString("- None (position limit reached; open-review candidates are summary-only below)\n\n")
	} else {
		for idx, item := range items {
			if item.Tier != "EXECUTABLE" && item.Tier != "REVIEWABLE" {
				continue
			}
			if !expanded[idx] {
				compact := e.formatHunterV7CompactSignalJSON(item.Coin)
				if compact != "" {
					sb.WriteString(fmt.Sprintf("- %s %s tier=%s compact_execution_json=%s\n",
						item.Coin.Symbol, item.Coin.Direction, item.Tier, compact))
				} else {
					sb.WriteString(fmt.Sprintf("- %s %s setup=%s ai_priority=%.1f reason=%s (compact only; lower priority)\n",
						item.Coin.Symbol, item.Coin.Direction, item.Coin.V7SetupType, item.Coin.V7AIPriority, item.Reason))
				}
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
			sb.WriteString(fmt.Sprintf("- %s %s tier=%s setup=%s status=%s quality=%s%s ai_priority=%.1f risk=%.0f reason=%s\n",
				item.Coin.Symbol, item.Coin.Direction, item.Tier, item.Coin.V7SetupType, item.Coin.V7Status,
				item.Coin.V7ExecutionQuality, hunterV7ShapeEntrySignalSuffix(item.Coin), item.Coin.V7AIPriority, item.Coin.V7RiskScore, item.Reason))
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
			sb.WriteString(fmt.Sprintf("- %s %s setup=%s status=%s quality=%s%s ai_priority=%.1f risk=%.0f reason=%s\n",
				item.Coin.Symbol, item.Coin.Direction, item.Coin.V7SetupType, item.Coin.V7Status,
				item.Coin.V7ExecutionQuality, hunterV7ShapeEntrySignalSuffix(item.Coin), item.Coin.V7AIPriority, item.Coin.V7RiskScore, item.Reason))
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

func hunterV7TierFromPromptReadiness(coin CandidateCoin, tier, reason string, readiness local.V7ExecutionReadiness) (string, string) {
	switch readiness.Tier {
	case local.V7ReadinessRejected:
		if tier != "REJECTED" {
			return "REJECTED", "prompt_readiness_" + readiness.Reason
		}
	case local.V7ReadinessWatch:
		if tier == "EXECUTABLE" || tier == "REVIEWABLE" {
			return "WATCH", "prompt_readiness_" + readiness.Reason
		}
	case local.V7ReadinessReviewable:
		if tier == "EXECUTABLE" {
			return "REVIEWABLE", "prompt_readiness_" + readiness.Reason
		}
	}
	if spec, ok := hunterV7SetupTierSpecs[coin.V7SetupType]; ok && spec.PromptWait != nil {
		if semanticReason := spec.PromptWait(coin, readiness); semanticReason != "" {
			if tier == "EXECUTABLE" || tier == "REVIEWABLE" {
				return "WATCH", semanticReason
			}
		}
	}
	return tier, reason
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

// hunterV7OpenReviewExpansionLimit sizes the full-context window. A wide
// open-review pool is the main reason good candidates never reach the LLM, so
// the window grows once the pool gets large.
func hunterV7OpenReviewExpansionLimit(openReviewCount, positionCount, maxPositions int) int {
	limit := 8
	if positionCount > 0 && (maxPositions <= 0 || positionCount < maxPositions) {
		limit = 6
	}
	if openReviewCount > 20 {
		limit += 4
	}
	return limit
}

// hunterV7SelectExpandedOpenReview picks which open-review candidates get full
// JSON. EXECUTABLE claims slots first, remaining slots go to REVIEWABLE by
// priority order, and a final pass guarantees each distinct setup keeps at
// least one full sample so a dominant route cannot crowd out every other route.
func hunterV7SelectExpandedOpenReview(items []hunterV7PromptCandidate, limit int) map[int]bool {
	selected := make(map[int]bool, limit)
	routes := make(map[string]bool)

	claim := func(tier string) {
		for idx, item := range items {
			if len(selected) >= limit {
				return
			}
			if selected[idx] || item.Tier != tier {
				continue
			}
			selected[idx] = true
			routes[item.Coin.V7SetupType] = true
		}
	}
	claim("EXECUTABLE")
	claim("REVIEWABLE")
	for idx, item := range items {
		if item.Tier != "EXECUTABLE" && item.Tier != "REVIEWABLE" {
			continue
		}
		if selected[idx] || routes[item.Coin.V7SetupType] {
			continue
		}
		selected[idx] = true
		routes[item.Coin.V7SetupType] = true
	}
	return selected
}

func hunterV7PromptExecutionReadiness(coin CandidateCoin, data *market.Data, tier, reason string) local.V7ExecutionReadiness {
	readiness := local.V7ExecutionReadiness{
		Tier:         local.V7ReadinessTier(tier),
		Reason:       reason,
		ReadyScore:   coin.V7AIPriority,
		WindowHealth: math.Min(100, math.Max(0, coin.V7TimingScore)),
		EntryZonePos: -1,
		DataQuality:  "complete",
		NextConfirm:  append([]string{}, coin.V7RequiredConfirms...),
	}
	if coin.V7ExecutionContext != nil && coin.V7ExecutionContext.DataQuality == "complete_for_execution" {
		readiness.DataQuality = "complete_for_execution"
	}
	if readiness.Tier == "" {
		readiness.Tier = local.V7ReadinessWatch
	}
	if coin.V7Readiness != nil {
		readiness.ReadyScore = coin.V7Readiness.ReadyScore
		readiness.WindowHealth = coin.V7Readiness.WindowHealth
		readiness.NextConfirm = append([]string{}, coin.V7Readiness.NextConfirm...)
	}

	price := 0.0
	if data != nil {
		price = data.CurrentPrice
	}
	if price <= 0 && coin.V7PriceContext != nil {
		price = coin.V7PriceContext.Last
	}
	if pos, ok := local.V7ZonePositionPct(coin.V7EntryZone, price); ok {
		readiness.EntryZonePos = pos
		readiness.PriceDeviation = hunterV7EntryZoneDeviationPct(price, coin.V7EntryZone)
		readiness.WindowHealth = hunterV7PromptWindowHealth(coin, pos)
	}

	missing := hunterV7CompactMissingFieldGroups(data, &coin)
	readiness.MissingHard = missing.Hard
	readiness.MissingExecution = missing.Execution
	readiness.MissingContext = missing.Context
	if missing.hasMissing() {
		readiness.DataQuality = "partial"
	}
	if reason == "backend_rr_infeasible" {
		readiness.Tier = local.V7ReadinessWatch
		readiness.Reason = reason
		readiness.BlockedGate = "execution_geometry"
		readiness.ReadyScore = math.Min(readiness.ReadyScore, 55)
		return readiness
	}
	switch {
	case len(missing.Hard) > 0:
		readiness.BlockedGate = "prompt_data_quality"
		if readiness.Tier == local.V7ReadinessExecutable {
			readiness.Tier = local.V7ReadinessReviewable
			readiness.Reason = missing.Hard[0] + "_missing"
		}
	case len(missing.Execution) > 0:
		readiness.BlockedGate = "confirmation_missing"
		if readiness.Tier == local.V7ReadinessExecutable {
			readiness.Tier = local.V7ReadinessReviewable
			readiness.Reason = missing.Execution[0] + "_missing"
		}
	case tier == "WATCH":
		readiness.BlockedGate = "kernel_tier"
	case tier == "REJECTED":
		readiness.BlockedGate = "kernel_tier_rejected"
	}
	readiness.ReadyScore = hunterV7PromptReadyScore(coin, readiness.WindowHealth, missing)
	return readiness
}

func hunterV7EntryZoneDeviationPct(price float64, zone local.V7PriceZone) float64 {
	if price <= 0 || zone.Lower <= 0 || zone.Upper <= zone.Lower {
		return 0
	}
	if price >= zone.Lower && price <= zone.Upper {
		return 0
	}
	if price < zone.Lower {
		return (price - zone.Lower) / zone.Lower * 100
	}
	return (price - zone.Upper) / zone.Upper * 100
}

func hunterV7PromptWindowHealth(coin CandidateCoin, zonePos float64) float64 {
	score := 35.0
	if zonePos >= 0 && zonePos <= 100 {
		score += 35
		if zonePos >= 15 && zonePos <= 85 {
			score += 10
		}
	} else if math.Abs(zonePos) <= 140 {
		score += 12
	}
	if strings.EqualFold(coin.Direction, "SHORT") {
		if hunterV7TakerBuyAtMost(coin, 0.50) {
			score += 12
		}
	} else if hunterV7TakerBuyAtLeast(coin, 0.50) {
		score += 12
	}
	if coin.V7ConfirmSummary != nil && coin.V7ConfirmSummary.PassedReview {
		score += 8
	}
	return math.Min(100, math.Max(0, score))
}

func hunterV7PromptReadyScore(coin CandidateCoin, windowHealth float64, missing hunterV7MissingFieldGroups) float64 {
	score := coin.V7SetupScore*0.25 + coin.V7TimingScore*0.20 + coin.V7RegimeFitScore*0.10 +
		coin.V7LiquidityScore*0.05 + coin.V7AIPriority*0.20 + windowHealth*0.20
	score -= math.Max(0, coin.V7RiskScore-35) * 0.35
	score -= float64(len(missing.Execution)) * 5
	score -= float64(len(missing.Context)) * 2
	score -= float64(len(missing.Hard)) * 20
	return math.Min(100, math.Max(0, score))
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

func (e *StrategyEngine) formatHunterV7CompactSignalJSON(coin CandidateCoin) string {
	// Compact JSON is a mask over the full prompt payload (U4.2): same
	// source struct, subset view, so the two encodings cannot drift.
	return marshalHunterV7JSON(buildHunterV7PromptPayload(coin).compactView())
}

func (e *StrategyEngine) formatHunterV7SignalJSON(coin CandidateCoin) string {
	// Reads the cached tier verdict; the payload struct owns field order and
	// tags (U4.1).
	return marshalHunterV7JSON(buildHunterV7PromptPayload(coin))
}

// hunterV7ShapeEntrySignalSuffix renders market shape and entry signal only when
// the router populated them, so summary lines stay compact for older signals.
func hunterV7ShapeEntrySignalSuffix(coin CandidateCoin) string {
	suffix := ""
	if coin.V7MarketShape != "" {
		suffix += " shape=" + coin.V7MarketShape
	}
	if coin.V7EntrySignal != "" {
		suffix += " entry_signal=" + coin.V7EntrySignal
	}
	return suffix
}

// hunterV7EffectiveTP0Price prefers the router-supplied TP0 from the take-profit
// ladder and falls back to the derived micro-TP0 for momentum setups that ship
// without an explicit ladder.
func hunterV7EffectiveTP0Price(coin CandidateCoin) float64 {
	if coin.V7TP0Price > 0 {
		return coin.V7TP0Price
	}
	return hunterV7TP0Price(coin)
}

func hunterV7EffectiveTP0DistancePct(coin CandidateCoin) float64 {
	if coin.V7TPPlan != nil && coin.V7TPPlan.TP0DistancePct > 0 {
		return coin.V7TPPlan.TP0DistancePct
	}
	return hunterV7TP0DistancePct(coin)
}

func hunterV7EffectiveMoveStopToBreakeven(coin CandidateCoin) bool {
	if coin.V7TPPlan != nil && coin.V7TPPlan.MoveStopToBreakeven {
		return true
	}
	return hunterV7TP0Price(coin) > 0
}

func hunterV7TP0Price(coin CandidateCoin) float64 {
	price := hunterV7PromptReferencePrice(coin)
	distancePct := hunterV7TP0DistancePct(coin)
	if price <= 0 || distancePct <= 0 {
		return 0
	}
	if strings.EqualFold(coin.Direction, "SHORT") {
		return price * (1 - distancePct/100)
	}
	if strings.EqualFold(coin.Direction, "LONG") {
		return price * (1 + distancePct/100)
	}
	return 0
}

func hunterV7TP0DistancePct(coin CandidateCoin) float64 {
	if coin.V7ExecutionTier != "EXECUTABLE" && coin.V7ExecutionTier != "REVIEWABLE" {
		return 0
	}
	switch coin.V7SetupType {
	case "mms_trend_ride_long", "mms_squeeze_engine_long", "alt_ladder_momentum_long", "alt_ladder_breakdown_short", "displacement_momentum_long", "breakdown_momentum_short":
	default:
		return 0
	}
	if containsAnyStringValue(coin.V7RiskTags, []string{"high_volatility", "extreme_volatility", "alt_ladder_late_chase_risk"}) {
		return 0.65
	}
	return 0.5
}

func hunterV7PositionSizeHint(coin CandidateCoin) string {
	if coin.V7ExecutionTier != "EXECUTABLE" && coin.V7ExecutionTier != "REVIEWABLE" {
		return ""
	}
	if coin.V7SetupType != "mms_trend_ride_long" && coin.V7SetupType != "mms_squeeze_engine_long" {
		return ""
	}
	geo := buildHunterV7ExecutionGeometry(coin)
	if geo == nil || geo.InsideEntryZone {
		return "normal_if_backend_rr_and_confirmations_pass"
	}
	if geo.DistanceToEntryPct > 0.75 {
		return "wait_pullback_price_too_far_from_entry_zone"
	}
	if geo.DistanceToEntryPct > 0.35 {
		return "small_size_or_wait_pullback"
	}
	return "normal_if_backend_rr_and_confirmations_pass"
}

func hunterV7PromptReferencePrice(coin CandidateCoin) float64 {
	if coin.V7PriceContext != nil && coin.V7PriceContext.Last > 0 {
		return coin.V7PriceContext.Last
	}
	if coin.V7EntryZone.Lower > 0 && coin.V7EntryZone.Upper > 0 {
		return (coin.V7EntryZone.Lower + coin.V7EntryZone.Upper) / 2
	}
	return 0
}

func buildHunterV7SuggestedTrigger(coin CandidateCoin) *DecisionTrigger {
	lower, upper := coin.V7EntryZone.Lower, coin.V7EntryZone.Upper
	if lower <= 0 || upper <= 0 {
		return nil
	}
	if lower > upper {
		lower, upper = upper, lower
	}

	switch coin.V7EntrySignal {
	case "entry_trigger_near":
		if strings.EqualFold(coin.Direction, "SHORT") {
			return &DecisionTrigger{
				TriggerPrice:      lower,
				RequiredClose:     "5m_or_15m_close_below_trigger",
				ExpiresInBars:     2,
				ActionIfTriggered: "review_for_open_with_rr_flow_and_backend_guard",
			}
		}
		return &DecisionTrigger{
			TriggerPrice:      upper,
			RequiredClose:     "5m_or_15m_close_through_breakout_level",
			ExpiresInBars:     2,
			ActionIfTriggered: "review_for_open_with_rr_flow_and_backend_guard",
		}
	case "entry_reclaim_wait", "entry_pullback_wait":
		mid := (lower + upper) / 2
		if strings.EqualFold(coin.Direction, "SHORT") {
			return &DecisionTrigger{
				TriggerPrice:      mid,
				RequiredClose:     "5m_close_below_ema20_or_entry_zone_mid",
				ExpiresInBars:     2,
				ActionIfTriggered: "review_for_open_after_failed_retest_with_rr_flow_and_backend_guard",
			}
		}
		return &DecisionTrigger{
			TriggerPrice:      mid,
			RequiredClose:     "5m_close_above_ema20_or_entry_zone_mid",
			ExpiresInBars:     2,
			ActionIfTriggered: "review_for_open_after_reclaim_with_rr_flow_no_new_low_and_backend_guard",
		}
	default:
		return nil
	}
}

type hunterV7ExecutionGeometry struct {
	CurrentPrice         float64 `json:"current_price,omitempty"`
	EntryZoneLower       float64 `json:"entry_zone_lower,omitempty"`
	EntryZoneUpper       float64 `json:"entry_zone_upper,omitempty"`
	EntryZonePositionPct float64 `json:"entry_zone_position_pct,omitempty"`
	DistanceToEntryPct   float64 `json:"distance_to_entry_pct,omitempty"`
	InsideEntryZone      bool    `json:"inside_entry_zone"`
	StopDistancePct      float64 `json:"stop_distance_pct,omitempty"`
	NearestTargetPrice   float64 `json:"nearest_target_price,omitempty"`
	NearestTargetRRPct   float64 `json:"nearest_target_rr,omitempty"`
	RemoteTargetOnly     bool    `json:"remote_target_only,omitempty"`
}

func buildHunterV7ExecutionGeometry(coin CandidateCoin) *hunterV7ExecutionGeometry {
	price := 0.0
	if coin.V7PriceContext != nil {
		price = coin.V7PriceContext.Last
	}
	if price <= 0 || coin.V7EntryZone.Lower <= 0 || coin.V7EntryZone.Upper <= 0 {
		return nil
	}
	lower, upper := coin.V7EntryZone.Lower, coin.V7EntryZone.Upper
	if lower > upper {
		lower, upper = upper, lower
	}
	geo := &hunterV7ExecutionGeometry{
		CurrentPrice:    price,
		EntryZoneLower:  lower,
		EntryZoneUpper:  upper,
		InsideEntryZone: price >= lower && price <= upper,
	}
	if upper > lower {
		geo.EntryZonePositionPct = (price - lower) / (upper - lower) * 100
	}
	if !geo.InsideEntryZone {
		if price < lower {
			geo.DistanceToEntryPct = (lower - price) / price * 100
		} else {
			geo.DistanceToEntryPct = (price - upper) / price * 100
		}
	}
	risk := hunterV7RiskDistance(coin, price)
	if risk > 0 {
		geo.StopDistancePct = risk / price * 100
	}
	nearestReward := 0.0
	for _, target := range coin.V7Targets {
		reward := hunterV7TargetReward(coin.Direction, price, target.Price)
		if reward <= 0 {
			continue
		}
		if nearestReward == 0 || reward < nearestReward {
			nearestReward = reward
			geo.NearestTargetPrice = target.Price
		}
	}
	if nearestReward > 0 && risk > 0 {
		geo.NearestTargetRRPct = nearestReward / risk
		geo.RemoteTargetOnly = nearestReward/price*100 > 8.0
	}
	return geo
}

func hunterV7RiskDistance(coin CandidateCoin, price float64) float64 {
	if price <= 0 || coin.V7Invalidation.Price <= 0 {
		return 0
	}
	if strings.EqualFold(coin.Direction, "SHORT") {
		return coin.V7Invalidation.Price - price
	}
	return price - coin.V7Invalidation.Price
}

func hunterV7TargetReward(direction string, price, targetPrice float64) float64 {
	if price <= 0 || targetPrice <= 0 {
		return 0
	}
	if strings.EqualFold(direction, "SHORT") {
		return price - targetPrice
	}
	return targetPrice - price
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
	rawMovePct := hunterV7PositionRawMovePct(pos)
	if pos.UnrealizedPnLPct <= -12 {
		return fmt.Sprintf("protection_state=hard_loss peak_pnl=%.2f%% current_pnl=%+.2f%% raw_move=%+.2f%%; hard stop territory, prioritize close unless exchange state already closing.\n",
			pos.PeakPnLPct, pos.UnrealizedPnLPct, rawMovePct)
	}
	if pos.PeakPnLPct >= 15 {
		return fmt.Sprintf("protection_state=high_profit_lock peak_pnl=%.2f%% current_pnl=%+.2f%% raw_move=%+.2f%%; protect roughly 40-50%% of peak or close on material giveback unless continuation is explicit.\n",
			pos.PeakPnLPct, pos.UnrealizedPnLPct, rawMovePct)
	}
	if pos.PeakPnLPct >= 10 {
		return fmt.Sprintf("protection_state=mid_profit_lock peak_pnl=%.2f%% current_pnl=%+.2f%% raw_move=%+.2f%%; protect roughly 25-35%% of peak or close on major giveback unless continuation is explicit.\n",
			pos.PeakPnLPct, pos.UnrealizedPnLPct, rawMovePct)
	}
	if pos.PeakPnLPct >= 5 {
		return fmt.Sprintf("protection_state=breakeven_floor peak_pnl=%.2f%% current_pnl=%+.2f%% raw_move=%+.2f%%; do not let a 5%%+ peak turn into net loss without explicit continuation evidence.\n",
			pos.PeakPnLPct, pos.UnrealizedPnLPct, rawMovePct)
	}
	return fmt.Sprintf("protection_state=pre_profit_floor peak_pnl=%.2f%% current_pnl=%+.2f%% raw_move=%+.2f%%; close on planned SL/hard invalidation or confirmed 5m+15m structural reversal.\n",
		pos.PeakPnLPct, pos.UnrealizedPnLPct, rawMovePct)
}

func hunterV7PositionRawMovePct(pos PositionInfo) float64 {
	if pos.EntryPrice <= 0 || pos.MarkPrice <= 0 {
		return 0
	}
	if strings.EqualFold(pos.Side, "short") {
		return (pos.EntryPrice - pos.MarkPrice) / pos.EntryPrice * 100
	}
	return (pos.MarkPrice - pos.EntryPrice) / pos.EntryPrice * 100
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
	return e.config.PromptCompactEnabled()
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
	// Same source as the JSON encodings (U4.2). TP0 stays on the raw signal
	// fields deliberately: the compact line only reports a router-supplied
	// TP0, while the payload's TP0Price falls back to the derived micro-TP0.
	p := buildHunterV7PromptPayload(*coin)

	sb.WriteString("Hunter v7 execution compact: ")
	parts := []string{
		fmt.Sprintf("setup=%s", p.SetupType),
		fmt.Sprintf("entry_mode=%s", p.EntryMode),
		fmt.Sprintf("execution_quality=%s", p.ExecutionQuality),
		fmt.Sprintf("confidence=%s", p.Confidence),
		fmt.Sprintf("timing=%.0f", p.TimingScore),
		fmt.Sprintf("risk=%.0f", p.RiskScore),
	}
	if pos, ok := local.V7ZonePositionPct(p.EntryZone, price); ok {
		parts = append(parts,
			fmt.Sprintf("entry_zone_pos=%.1f%%", pos),
			fmt.Sprintf("dist_zone_upper=%+.2f%%", pctMove(price, p.EntryZone.Upper)),
			fmt.Sprintf("dist_zone_lower=%+.2f%%", pctMove(price, p.EntryZone.Lower)),
			fmt.Sprintf("zone_location=%s", entryZoneLocation(pos)),
		)
	}
	if p.Invalidation.Price > 0 {
		parts = append(parts, fmt.Sprintf("invalidation_dist=%+.2f%%", pctMove(price, p.Invalidation.Price)))
	}
	if coin.V7TP0Price > 0 {
		parts = append(parts, fmt.Sprintf("tp0_dist=%+.2f%%", pctMove(price, coin.V7TP0Price)))
		if coin.V7TP0RR > 0 {
			parts = append(parts, fmt.Sprintf("tp0_rr=%.2f", coin.V7TP0RR))
		}
	}
	if p.TP1Price > 0 {
		parts = append(parts, fmt.Sprintf("tp1_dist=%+.2f%%", pctMove(price, p.TP1Price)))
	}
	if p.TP2Price > 0 {
		parts = append(parts, fmt.Sprintf("tp2_dist=%+.2f%%", pctMove(price, p.TP2Price)))
	}
	if p.TakeProfitPlan != nil {
		parts = append(parts,
			fmt.Sprintf("tp0_reduce=%.0f-%.0f%%", p.TakeProfitPlan.TP0ReducePctMin, p.TakeProfitPlan.TP0ReducePctMax),
			fmt.Sprintf("tp0_breakeven=%t", p.TakeProfitPlan.MoveStopToBreakeven),
			fmt.Sprintf("trailing_stop=%s", hunterV7CompactTrailingPlan(p.TakeProfitPlan)),
		)
	}
	if len(p.Targets) > 0 && p.Targets[0].Price > 0 {
		parts = append(parts, fmt.Sprintf("target1_dist=%+.2f%%", pctMove(price, p.Targets[0].Price)))
	}
	if p.DerivativesContext != nil {
		parts = append(parts,
			fmt.Sprintf("oi_1h=%+.2f%%", p.DerivativesContext.OIChange1h),
			fmt.Sprintf("oi_4h=%+.2f%%", p.DerivativesContext.OIChange4h),
			fmt.Sprintf("taker15m=%.3f", p.DerivativesContext.TakerBuy15m),
		)
		if p.SetupType == "funding_reversal" {
			parts = append(parts, fmt.Sprintf("oi_state=%s", fundingReversalOIState(p.DerivativesContext.OIChange1h, p.DerivativesContext.OIChange4h)))
		}
	}
	vwap15m := hunterV7CompactVWAP15m(data, coin)
	if tf15 := data.TimeframeData["15m"]; tf15 != nil {
		if tfSummary := executionTFCompact("15m", price, tf15, vwap15m); tfSummary != "" {
			parts = append(parts, tfSummary)
		}
	} else if tfSummary := executionContextTFCompact("15m", coin); tfSummary != "" {
		parts = append(parts, tfSummary)
	}
	if tf5 := data.TimeframeData["5m"]; tf5 != nil {
		if tfSummary := executionTFCompact("5m", price, tf5, 0); tfSummary != "" {
			parts = append(parts, tfSummary)
		}
	} else if tfSummary := executionContextTFCompact("5m", coin); tfSummary != "" {
		parts = append(parts, tfSummary)
	}
	if warning := hunterV7ExecutionWarning(price, coin); warning != "" {
		parts = append(parts, "warning="+warning)
	}
	if rule := hunterV7ExecutionHardRule(price, coin); rule != "" {
		parts = append(parts, "hard_rule="+rule)
	}
	missing := hunterV7CompactMissingFieldGroups(data, coin)
	if missing.hasMissing() {
		parts = append(parts, "compact_data_quality=partial")
		if len(missing.Hard) > 0 {
			parts = append(parts,
				fmt.Sprintf("missing_hard=%s", strings.Join(missing.Hard, ",")),
				"missing_hard_rule=wait",
			)
		}
		if len(missing.Execution) > 0 {
			parts = append(parts,
				fmt.Sprintf("missing_execution=%s", strings.Join(missing.Execution, ",")),
				"missing_execution_rule=review_or_wait_for_setup_confirmation",
				fmt.Sprintf("conditional_open_if=%s", hunterV7ConditionalOpenChecklist(coin, missing.Execution)),
			)
		}
		if len(missing.Context) > 0 {
			parts = append(parts,
				fmt.Sprintf("missing_context=%s", strings.Join(missing.Context, ",")),
				"missing_context_rule=do_not_global_wait_reduce_confidence_or_size",
			)
		}
	} else {
		dataQuality := "complete"
		if p.ExecutionContext != nil && p.ExecutionContext.DataQuality == "complete_for_execution" {
			dataQuality = "complete_for_execution"
		}
		parts = append(parts, "compact_data_quality="+dataQuality)
	}
	sb.WriteString(strings.Join(parts, " | "))
	sb.WriteString("\n")
	return sb.String()
}

func hunterV7CompactTrailingPlan(plan *local.V7TakeProfitPlan) string {
	if plan == nil {
		return ""
	}
	basis := strings.Join(plan.TrailingBasis, "_or_")
	if basis == "" {
		basis = plan.TrailingStopMode
	}
	if plan.TrailingDistancePctMin > 0 && plan.TrailingDistancePctMax > 0 {
		return fmt.Sprintf("%s_%.1f-%.1f%%", basis, plan.TrailingDistancePctMin, plan.TrailingDistancePctMax)
	}
	return basis
}

func hunterV7ConditionalOpenChecklist(coin *CandidateCoin, missingExecution []string) string {
	items := make([]string, 0, len(missingExecution)+2)
	if coin == nil {
		for _, field := range missingExecution {
			if field != "" {
				items = append(items, field+"_resolved")
			}
		}
		items = append(items, "entry_zone_valid", "invalidation_and_rr_valid")
		return strings.Join(items, "+")
	}
	items = make([]string, 0, len(missingExecution)+len(coin.V7RequiredConfirms)+2)
	for _, field := range missingExecution {
		if field == "" {
			continue
		}
		items = append(items, field+"_resolved")
	}
	for _, confirm := range coin.V7RequiredConfirms {
		if confirm == "" {
			continue
		}
		items = append(items, confirm+"_visible")
	}
	items = append(items, "entry_zone_valid", "invalidation_and_rr_valid")
	return strings.Join(items, "+")
}

type hunterV7MissingFieldGroups struct {
	Hard      []string
	Execution []string
	Context   []string
}

func (m hunterV7MissingFieldGroups) hasMissing() bool {
	return len(m.Hard) > 0 || len(m.Execution) > 0 || len(m.Context) > 0
}

func hunterV7CompactMissingFields(data *market.Data, coin *CandidateCoin) []string {
	groups := hunterV7CompactMissingFieldGroups(data, coin)
	out := make([]string, 0, len(groups.Hard)+len(groups.Execution)+len(groups.Context))
	out = append(out, groups.Hard...)
	out = append(out, groups.Execution...)
	out = append(out, groups.Context...)
	return out
}

func hunterV7CompactMissingFieldGroups(data *market.Data, coin *CandidateCoin) hunterV7MissingFieldGroups {
	groups := hunterV7MissingFieldGroups{}
	if coin == nil {
		return groups
	}
	if coin.V7EntryZone.Lower <= 0 || coin.V7EntryZone.Upper <= coin.V7EntryZone.Lower {
		groups.Hard = append(groups.Hard, "entry_zone")
	}
	if coin.V7Invalidation.Price <= 0 {
		groups.Hard = append(groups.Hard, "invalidation")
	}
	if len(coin.V7Targets) == 0 || coin.V7Targets[0].Price <= 0 {
		groups.Hard = append(groups.Hard, "target1")
	}
	if coin.V7DerivativesCtx == nil {
		groups.Execution = append(groups.Execution, "derivatives_context")
	} else if coin.V7DerivativesCtx.TakerBuy15m <= 0 {
		groups.Execution = append(groups.Execution, "taker_buy_15m")
	}
	if data == nil {
		groups.Hard = append(groups.Hard, "market_data")
		return groups
	}
	tf15 := data.TimeframeData["15m"]
	if tf15 == nil || len(tf15.Klines) == 0 {
		if summary, ok := hunterV7ExecutionContextTimeframe(coin, "15m"); ok {
			if !summary.HasATR {
				groups.Context = append(groups.Context, "15m_atr")
			}
			if !summary.HasEMA20 {
				groups.Execution = append(groups.Execution, "15m_ema20")
			}
		} else {
			groups.Execution = append(groups.Execution, "15m_kline")
		}
	} else {
		if tf15.ATR14 <= 0 {
			groups.Context = append(groups.Context, "15m_atr")
		}
		if _, ok := lastFloat(tf15.EMA20Values); !ok {
			groups.Execution = append(groups.Execution, "15m_ema20")
		}
	}
	if hunterV7RequiresVWAP(coin) && hunterV7CompactVWAP15m(data, coin) <= 0 {
		groups.Execution = append(groups.Execution, "15m_vwap")
	}
	tf5 := data.TimeframeData["5m"]
	if tf5 == nil || len(tf5.Klines) == 0 {
		if !hunterV7ExecutionContextHasKline(coin, "5m") {
			groups.Execution = append(groups.Execution, "5m_kline")
		}
	}
	return groups
}

func executionContextTFCompact(label string, coin *CandidateCoin) string {
	if coin == nil || coin.V7ExecutionContext == nil {
		return ""
	}
	tf, ok := coin.V7ExecutionContext.Timeframes[label]
	if !ok || tf.CandleCount == 0 {
		return ""
	}
	parts := []string{
		fmt.Sprintf("%s_recent_high3=%.6f", label, tf.RecentHigh3),
		fmt.Sprintf("%s_recent_low3=%.6f", label, tf.RecentLow3),
	}
	if tf.HasATR {
		parts = append(parts,
			fmt.Sprintf("%s_atr_pct=%.2f%%", label, tf.ATRPct),
			fmt.Sprintf("%s_min_stop_0_8atr=%.2f%%", label, tf.MinStop08ATRPct),
		)
	}
	if tf.HasEMA20 {
		parts = append(parts, fmt.Sprintf("%s_close_vs_ema20=%+.2f%%", label, tf.CloseVsEMA20Pct))
	}
	if tf.HasVWAP20 {
		parts = append(parts,
			fmt.Sprintf("%s_vwap20=%.6f", label, tf.VWAP20),
			fmt.Sprintf("%s_close_vs_vwap20=%+.2f%%", label, tf.CloseVsVWAP20Pct),
			fmt.Sprintf("%s_close_below_vwap20=%t", label, tf.CloseVsVWAP20Pct < 0),
			fmt.Sprintf("%s_close_above_vwap20=%t", label, tf.CloseVsVWAP20Pct > 0),
		)
	}
	if tf.CandleCount >= 6 {
		parts = append(parts,
			fmt.Sprintf("%s_no_new_high=%t", label, tf.NoNewHigh),
			fmt.Sprintf("%s_no_new_low=%t", label, tf.NoNewLow),
		)
		if tf.VolumeVsAvg5 > 0 {
			parts = append(parts, fmt.Sprintf("%s_vol_vs_avg5=%.2fx", label, tf.VolumeVsAvg5))
		}
	}
	return strings.Join(parts, " ")
}

func hunterV7ExecutionContextHasKline(coin *CandidateCoin, tf string) bool {
	if coin == nil || coin.V7ExecutionContext == nil {
		return false
	}
	summary, ok := coin.V7ExecutionContext.Timeframes[tf]
	return ok && summary.CandleCount > 0
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
		if summary, ok := hunterV7ExecutionContextTimeframe(coin, "15m"); ok && summary.HasVWAP20 {
			return summary.VWAP20
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

func hunterV7ExecutionContextTimeframe(coin *CandidateCoin, tf string) (local.V7ExecutionTimeframeSummary, bool) {
	if coin == nil || coin.V7ExecutionContext == nil {
		return local.V7ExecutionTimeframeSummary{}, false
	}
	summary, ok := coin.V7ExecutionContext.Timeframes[tf]
	if !ok || summary.CandleCount == 0 {
		return local.V7ExecutionTimeframeSummary{}, false
	}
	return summary, true
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
		if pos, ok := local.V7ZonePositionPct(coin.V7EntryZone, price); ok && pos <= 45 {
			warnings = append(warnings, "short_near_zone_lower")
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
