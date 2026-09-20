# Hunter v7 形态筛选机制、开仓胜率、提示词与止盈止损审校报告 - 2026-08-04

> 数据窗口：`hunter_v7_signal_records.timestamp >= now()-30d`  
> 数据源：本地 `data/data.db` + 已归档 Binance 实时验证报告 + 当前代码。  
> 统计口径：开仓率 = setup 输出行中最终 `EXECUTABLE/REVIEWABLE` 占比；胜率 = 最终 open-review 且已结案 outcome 中 `WIN_TP* / PROTECTED_STOP / 正收益 STOP` 占比。WATCH/REJECTED/raw 空 tier 不计入真实胜率。

## 1. 版本与机制主要变化

### 1.1 从旧版到 Hunter v7

旧版 Hunter 更偏“综合分/单一路径筛选”：按涨跌幅、成交额、OI、资金费率等综合打分，把高分标的交给策略引擎。问题是同一套评分难以同时处理突破、反转、拥挤、山寨阶梯、MMS 控盘、资金费率事件等不同机会。

Hunter v7 的核心变化：

- 从单分数改为多形态路由：不同 setup 有独立 Match/Score/Entry/Confirm。
- 从“选币列表”改为结构化信号协议：`hunter_v7_signal_json` 输出 setup、direction、tier、entry_zone、invalidation、TP0/TP1/TP2、reason/risk tags。
- 从立即开仓改为分层漏斗：`EXECUTABLE / REVIEWABLE / WATCH / REJECTED`。
- 从只看当轮改为跨轮状态机：pre/watch 信号可在后续 close-through、trigger-memory、retest 失败后升级。
- 从事后人工看收益改为长期 outcome 写回：`track_status / pnl / MFE / MAE / PROTECTED_STOP`。

### 1.2 近期几版主要演进

| 阶段 | 主要变化 | 解决的问题 | 新副作用 |
|---|---|---|---|
| v7 初版 | 11 类 setup、多池 universe、regime 权重、JSON prompt | 摆脱单一综合分 | setup 扩展后字符串/阈值开始分散 |
| Lean core 重构 | 统一 tag_semantics、tier 漏斗、几何/RR/确认码口径 | 减少补丁式重复判断 | 部分旧报告/旧 DB 口径仍混杂 |
| Champion audit | 新增 `relative_weakness_short`、相对强弱双向覆写 | 补强强市做空最弱者盲区 | 样本仍少，未形成主要开仓来源 |
| Alt-ladder 重构 | 新增 `alt_ladder_momentum_long` / `alt_ladder_breakdown_short` | 捕捉高振幅山寨 5%/15%/30% 阶梯 | early/mid/late 边界容易过拟合 |
| P0/P1 优化 | outcome 持续跟踪、TP0、保护止损、重复 thesis 去重 | 胜率统计更接近真实路径 | 短窗口 ACTIVE 仍需 post-track |
| 最近修正 | `alt_ladder_breakdown_short` 改为低区位+新空OI+低taker软升档；funding 拆桶；range 反抽失败确认；非 open 行不再写 ACTIVE | 避免伪短空和 outcome 分母污染 | trend_down 下 open-rate 偏保守 |

当前代码中的 setup 已超过早期架构文档的 11 类，实际包括：趋势突破、位移动量、leader momentum、alt ladder long/short、MMS、funding、range、panic/pullback/whale、relative weakness、distribution/long squeeze、intraday scalp、volatility squeeze 等 20+ 类。

## 2. 各形态机制摘要

| 家族 | 代表 setup | 主要开仓逻辑 | 当前定位 |
|---|---|---|---|
| 动量延续 | `leader_momentum_long`, `displacement_momentum_long`, `mms_trend_ride_long`, `intraday_scalp_long` | 顺势、taker/OI/量能同向，避免高位追单 | trend_up/compression 可用，但样本分化 |
| 突破压缩 | `trend_breakout_long`, `accumulation_breakout_long`, `volatility_squeeze_breakout`, `pre_breakout_watch` | close-through trigger 或 clean pullback 后升级 | 高召回、低开仓，主要做 watch |
| 阶梯山寨 | `alt_ladder_momentum_long`, `alt_ladder_breakdown_short` | 高振幅 alt 的 early/mid/late 分段；short 需新空OI+低taker+低区位 | long 容易追高，short 已收紧 |
| 修复反转 | `panic_reversal_long`, `pullback_reversal_long`, `whale_flow_reversal`, `funding_reversal` | reclaim、卖压衰竭、低位吸筹、拥挤反转 | whale 有价值；funding 默认观察池 |
| 弱势/派发 | `relative_weakness_short`, `distribution_short`, `breakdown_momentum_short`, `range_expansion_event` | 弱反弹进空、派发拒绝、破位延续、扩张事件 | trend_down 中 breakdown 表现较好 |
| Watch-only | `pre_squeeze_watch`, `pre_distribution_watch`, `accumulation_watch` | 只提供下一轮触发背景 | 不进入真实开仓率 |

## 3. 不同行情下开仓率

以下为 30 天 DB 输出池口径，`total` 为该 regime/setup 的非 `module_no_match` 输出行数，`open_rows` 为最终 `EXECUTABLE/REVIEWABLE`。

### Compression

| Setup | Total | Open | Open-rate |
|---|---:|---:|---:|
| alt_ladder_momentum_long | 20 | 8 | 40.0% |
| displacement_momentum_long | 7 | 2 | 28.6% |
| whale_flow_reversal | 11 | 3 | 27.3% |
| trend_breakout_long | 7 | 1 | 14.3% |
| range_expansion_event | 16 | 1 | 6.3% |
| alt_ladder_breakdown_short | 19 | 1 | 5.3% |
| funding_reversal | 23 | 0 | 0.0% |

### Rotation

| Setup | Total | Open | Open-rate |
|---|---:|---:|---:|
| range_expansion_event | 63 | 15 | 23.8% |
| alt_ladder_breakdown_short | 79 | 15 | 19.0% |
| alt_ladder_momentum_long | 53 | 10 | 18.9% |
| whale_flow_reversal | 28 | 2 | 7.1% |
| mms_trend_ride_long | 89 | 6 | 6.7% |
| leader_momentum_long | 19 | 1 | 5.3% |
| funding_reversal | 1036 | 0 | 0.0% |
| trend_breakout_long | 879 | 0 | 0.0% |

### Trend Down

| Setup | Total | Open | Open-rate |
|---|---:|---:|---:|
| breakdown_momentum_short | 40 | 9 | 22.5% |
| whale_flow_reversal | 23 | 5 | 21.7% |
| alt_ladder_breakdown_short | 191 | 16 | 8.4% |
| alt_ladder_momentum_long | 89 | 1 | 1.1% |
| funding_reversal | 567 | 0 | 0.0% |
| trend_breakout_long | 706 | 0 | 0.0% |
| range_expansion_event | 62 | 0 | 0.0% |

### Trend Up

| Setup | Total | Open | Open-rate |
|---|---:|---:|---:|
| mms_trend_ride_long | 10 | 5 | 50.0% |
| displacement_momentum_long | 108 | 25 | 23.1% |
| alt_ladder_momentum_long | 20 | 3 | 15.0% |
| range_expansion_event | 7 | 1 | 14.3% |
| leader_momentum_long | 87 | 6 | 6.9% |
| trend_breakout_long | 2427 | 125 | 5.2% |
| funding_reversal | 1183 | 0 | 0.0% |
| alt_ladder_breakdown_short | 37 | 0 | 0.0% |

## 4. 不同行情下胜率与盈利

样本仅统计最终 open-review 且已结案 outcome。`protected_or_win` 包含 `WIN_TP*`、`PROTECTED_STOP`、以及旧口径下正收益 `STOP`。

| Regime | Setup | Outcomes | Protected/Win | Rate | Loss Stop | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| compression | alt_ladder_momentum_long | 5 | 2 | 40.0% | 3 | -1.137 | +0.577 | -1.518 |
| compression | displacement_momentum_long | 2 | 2 | 100.0% | 0 | +0.308 | +0.850 | -0.121 |
| compression | trend_breakout_long | 1 | 1 | 100.0% | 0 | +1.200 | +1.242 | -0.004 |
| compression | whale_flow_reversal | 1 | 1 | 100.0% | 0 | +0.180 | +0.672 | 0.000 |
| rotation | range_expansion_event | 13 | 11 | 84.6% | 2 | +1.645 | +3.009 | -0.182 |
| rotation | alt_ladder_breakdown_short | 5 | 4 | 80.0% | 1 | -0.138 | +0.862 | -0.615 |
| rotation | alt_ladder_momentum_long | 7 | 3 | 42.9% | 4 | -0.569 | +0.941 | -1.509 |
| rotation | whale_flow_reversal | 2 | 2 | 100.0% | 0 | +0.402 | +1.506 | 0.000 |
| trend_down | breakdown_momentum_short | 9 | 8 | 88.9% | 1 | +0.156 | +1.273 | -0.388 |
| trend_down | alt_ladder_breakdown_short | 7 | 6 | 85.7% | 1 | +0.530 | +1.612 | -0.435 |
| trend_down | whale_flow_reversal | 3 | 2 | 66.7% | 1 | -0.025 | +0.803 | -0.723 |
| trend_up | range_expansion_event | 1 | 1 | 100.0% | 0 | +0.800 | +1.470 | 0.000 |

### 关键读数

1. `funding_reversal` 输出极多但 open=0，拆桶正确。它是事件背景，不应进主开仓率。
2. `trend_breakout_long` 在 trend_up/rotation 大量召回但开仓率低，说明它更像 watch/trigger pool，不是当前主开仓来源。
3. `range_expansion_event` 在 rotation 的 outcome 最好，但历史复盘也显示它在极端高波动下有大 MAE；必须保留反抽失败/高位 OI 过滤。
4. `alt_ladder_breakdown_short` 早期曾是亏损源，收紧后 trend_down/rotation 的保护胜率改善，但最近两轮 open=0，说明当前门槛偏保守但更干净。
5. `alt_ladder_momentum_long` 在 compression/rotation 的平均 PnL 为负，是当前最需要防追高的 long 分支。

## 5. 猎手 v7 策略提示词审校

当前提示词文件：`kernel/hunter_v7_prompt_doctrine.go`。

### 优点

- 已不是散乱补丁，而是五段结构：角色、数据契约、决策漏斗、风险仓位、输出契约。
- 明确 `hunter_v7_signal_json` 为唯一事实源，减少 LLM 自行发明证据。
- 对 `EXECUTABLE/REVIEWABLE/WATCH/REJECTED` 的行为边界较清晰。
- 各家族剧本基本专业：动量不追末端、突破要 close-through、反转不接刀、funding 不对称放宽、range short 要反抽失败。
- 输出契约要求 `selected_hunter_v7_signal_id`、单一 `blocked_reason_code`、trigger 对象，利于后端校验。

### 问题

提示词仍有“补丁化残留”：

- §3 各家族段落过长，把大量阈值写进自然语言，如 `taker<=0.34`、`OI>=0.8%`、`zone<=45%`。这些更适合在规则表/JSON contract 中表达。
- 同一段同时覆盖 setup 剧本、风控、例外、软升档、禁止动作，LLM 可读但不够可验证。
- Prompt 承担了部分本该由后端 rule-table 强制的事情，未来每修一条规则就容易继续往 prompt 加一句。
- “专业精准”的方向应是：prompt 只讲交易原则和 JSON 字段使用方法；硬阈值由 `tier_reason / required_confirmations / tag_semantics` 给出，不在自然语言反复手写。

结论：当前提示词比早期补丁堆叠明显专业，但还没有完全达到“规则即数据、prompt 只做执行纪律”的理想形态。

## 6. 止盈止损与保护逻辑

### 6.1 信号层

每个 Hunter v7 信号包含：

- `entry_zone.lower/upper`
- `invalidation.price`：结构失效价，作为原始 SL
- `tp0_price`：第一保护目标
- `tp1_price / tp2_price` 或 `targets[]`
- `take_profit_plan`：TP0 减仓 30%-50%、移动止损到保本或 5m EMA20/VWAP

### 6.2 Outcome tracker

当前 `SignalOutcomeTracker` 的核心逻辑：

- 注册对象：只跟踪最终 `EXECUTABLE/REVIEWABLE`；WATCH 只可作为 missed opportunity audit。
- 去重：同 `symbol/setup/direction` 在 30 分钟内只保留最新 active thesis，旧记录标记 `DUPLICATE_CONTEXT`。
- 数据：优先用 1m candle high/low 判定 TP/SL，缺口用 REST backfill 补齐。
- 判定顺序：先检查 stop，再检查 TP2/TP1/TP0。
- TP0：默认先减 35%，剩余仓继续跟踪，动态止损推至入场价。
- 动态止损：MFE 达阈值后，对 eligible setup 推保本；continuation 类 short/long 有 breakeven eligibility。
- `PROTECTED_STOP`：LONG 的 stop >= entry，或 SHORT 的 stop <= entry 时，止损被记为保护退出，不计入亏损 stop。
- Timeout：默认 8 小时。

一个需要注意的技术细节：同一根 1m candle 同时触发 TP 与 SL 时，当前代码先判 stop。该口径保守，但会低估某些高波动策略的 TP-first 胜率。

### 6.3 实盘保护器

`auto_trader_risk.go` 还有持仓级保护：

- TP0：杠杆 ROE 约 `>=8%` 或触达计划 TP0，减仓 35%。
- TP1/TP2：计划目标或 ROE 阈值，分别减仓 40%/50%。
- Peak >= 5% 后若回到保本/亏损，优先减险。
- Peak >= 10% 回吐约 50%、Peak >= 15% 回吐约 45%，触发保护性平仓。
- 开仓 15 分钟内 PnL <= -8% 或任意时刻 <= -12%，硬亏损退出。
- TP0 后剩余仓以保本/动态止损/EMA/VWAP 追踪。

## 7. 当前主要问题

1. 开仓率在不同 regime 中分化明显：trend_down 最近两轮 open=0，说明保守性偏强；但历史数据中 trend_down 的 `breakdown_momentum_short` 和 `alt_ladder_breakdown_short` outcome 质量较好，不能简单继续压紧。
2. `alt_ladder_momentum_long` 的盈利质量不足：compression/rotation 中平均 PnL 为负，MAE 明显大于 MFE，需要更强防追高。
3. `trend_breakout_long` 召回过大但开仓率低：它是 watch pool，不应在主 UI 或统计中被误读为低效开仓形态。
4. `range_expansion_event` 历史盈利好，但尾部风险高：需要保持反抽失败确认和高位 OI 过滤。
5. 提示词仍承载过多阈值补丁：应把阈值迁回 typed rule/table。
6. Outcome 明细报告还不够：validator 聚合有了，但逐笔 symbol/setup/entry/exit/TP0/SL 仍需 DB 查询。

## 8. 优化方案建议

### P0：统计和报告

1. validation outcome summary 增加逐笔明细：symbol、dir、setup、tier、entry、exit、TP0、SL、status、PnL、MFE、MAE。
2. API 和前端继续分栏显示：
   - output funnel open-rate
   - real open-review outcome
   - watch missed opportunity
   - raw setup funnel
3. 所有自动调参只使用 `EXECUTABLE/REVIEWABLE + terminal outcome`，不得使用 WATCH 复盘直接改主阈值。

### P1：形态机制

1. `alt_ladder_momentum_long`：
   - late/high_volatility/funding_elevated 时必须 entry_zone_position <= 55%。
   - MFE >= 0.6% 或达到 TP0 距离 60% 时推保护止损。
   - OI 未继续净流入时，stage_mid/late 不得仅凭 taker buy 升级。
2. `alt_ladder_breakdown_short`：
   - 保留当前软升档，不再降低 taker/OI。
   - 新建 `alt_ladder_flush_watch_pool`，累计 OI 下降的 flush 型短空，至少 6-9 轮验证后再考虑 micro 分支。
3. `breakdown_momentum_short`：
   - trend_down 下 outcome 质量好，可作为主 short 提升方向。
   - 增加 weak rebound into zone 入场，不在下跌末端市价追空。
4. `range_expansion_event`：
   - 保持高位 OI 负增、反抽未失败、zone_position>60% 的硬降档。
   - LONG 只在回踩 VWAP/EMA20 后重新放量站回时 open。
5. `funding_reversal`：
   - 继续不纳入主开仓率。
   - 只有 funding 极值 + OI/价格背离 + taker 反转 + retest failed 四项齐全才 REVIEWABLE。

### P2：提示词治理

1. 把家族阈值从自然语言迁入机器字段：`required_confirmations`、`tag_semantics`、`tier_reason`。
2. 提示词保留五段框架，但将 §3 改成“原则+字段解释”，避免每次新增规则都往 prompt 加补丁。
3. 为每个 setup 输出固定 playbook schema：
   - `entry_permission`
   - `must_confirm`
   - `hard_blocks`
   - `size_policy`
   - `exit_policy`
4. 增加 prompt contract 单测：给定同一 `hunter_v7_signal_json`，LLM 必须返回唯一 blocked_reason 或 open，不允许泛化 wait。

### P3：止盈止损

1. 对 1m 同时触发 TP/SL 的 candle，新增保守/中性两种报告口径：
   - conservative：先 SL
   - neutral：按 candle path unknown，记为 BOTH_SAME_1M
2. TP0 后 runner 继续跟踪已实现，建议在报告中展示 realized + unrealized 分解。
3. 对 MFE>1% 后亏损 STOP 的样本单独告警，作为动态止损调优输入。

## 9. 结论

Hunter v7 当前已经不是单纯补丁堆叠，而是多形态、分层、可追踪的专业筛选系统。但仍有两个需要继续收敛的地方：

- 规则还不够“数据化”：部分阈值仍写在提示词和散落 guard 中。
- 盈利质量还不够稳定：不同 regime/setup 差异大，不能只追 open-rate。

下一步应以 `breakdown_momentum_short`、`range_expansion_event`、`whale_flow_reversal` 的高质量样本作为增益方向，以 `alt_ladder_momentum_long` 防追高和 `alt_ladder_breakdown_short` watch 分桶作为风控方向，避免为了开仓率牺牲胜率。
