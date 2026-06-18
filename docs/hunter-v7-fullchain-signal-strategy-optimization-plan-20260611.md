# Hunter v7 全链路信号召回与策略胜率改造方案

> 日期：2026-06-11  
> 范围：`datafetch` 实时快照、Hunter v7 universe/router/setup/watch、kernel tier/prompt、LLM 决策解析、trader 执行风控、signal/mover 归因。  
> 目标：让 Hunter v7 在趋势跟随、回调延续、突破、挤压、恐慌反转、资金费率反转、区间回归、顶部派发等行情下更稳定地捕捉入场窗口，同时保持后端硬风控，不用“盲目放宽规则”换开仓率。

## 1. 结论摘要

当前 Hunter v7 不是缺少形态模块，而是“信号发现态”和“真实可执行态”之间存在多处状态不一致：

1. `provider/local` 已经能输出多形态信号，但 `kernel` 的 tier 分类、prompt 实时数据、trader 执行 guard 没有共享同一套实时执行状态。
2. `kernel/engine_prompt.go` 会用实时价格覆盖 `V7PriceContext.Last`，但默认继续使用 scoring 阶段缓存的 `V7ExecutionTier`，导致 prompt 层可能拿旧 tier 评估新价格。
3. `kernel/engine.go` 的 `hunterV7BackendCappedRRWaitReason()` 使用硬编码 TP cap，而 `kernel/engine_prompt.go` 和 `trader/auto_trader_risk.go` 已经按配置动态放大可行 TP cap；这会造成“prompt/trader 允许、kernel tier 提前判 WATCH”的隐性不一致。
4. `hunterV7CompactMissingFields()` 把部分实时字段缺失转成 `missing_fields_rule=wait_unless_all_required_confirmations_are_visible`，容易让 LLM 把局部数据缺失扩大成全局 wait。
5. `writeHunterV7TieredCandidatePrompt()` 只完整展开前 5 个 EXECUTABLE/REVIEWABLE，行情集中爆发时第 6 个以后缺少 `hunter_v7_signal_json`，LLM 难以比较。
6. `V7SignalStateManager.applyUpgrade()` 实际已经注入 `watch_upgraded_reviewable` 与 `multi_cycle_confirmation`，因此 watch 升级问题不在字段缺失，而在升级条件、实时 taker/OI 校验和后续 tier 兜底仍偏窄。
7. 系统已有 `hunter_v7_signal_records` 与 `hunter_v7_mover_labels`，后续调参必须用召回率、分层通过率、开仓后表现做闭环，不应继续凭单轮样本手改阈值。

改造方向：新增统一的 `Execution Readiness` 层，作为 provider 信号与 kernel/trader 之间的共享执行态。它不替代硬风控，只负责回答三个问题：

- 现在是否仍在入场窗口？
- 当前缺的是硬阻断、复核确认，还是仅上下文字段？
- 如果不开仓，卡在哪一层，下一轮需要看到什么才升级？

## 2. 当前真实链路

| 层级 | 主要文件 | 当前职责 | 关键风险 |
|---|---|---|---|
| 数据采集 | `datafetch/*` | ticker、premium index、OI、LSR、K线、WS 快照 | detail quota、REST 限速、字段新鲜度不直接进入 tier |
| Universe | `provider/local/hunter_v7_universe.go` | 多维候选池、OI notional、amplitude/range expansion | 高振幅召回已改善，但缺少字段 freshness score |
| Regime | `provider/local/hunter_v7_regime.go` | BTC/ETH 大盘 regime | setup 权重是静态矩阵，未按波动率自适应 |
| Setup modules | `provider/local/hunter_v7_mod_*.go` | 输出形态、分数、reason/risk/confirm | 各模块的 entry zone 与 required confirms 粒度不一致 |
| Router | `provider/local/hunter_v7_router.go` | regime 权重、risk/liquidity、priority、output/fallback | `context_only_low_priority` 可能让 fallback reviewable 失效 |
| Execution finalize | `provider/local/hunter_v7_execution.go` | RR、止损修复、确认摘要、execution quality | 使用 scoring 快照，不感知 prompt/trader 的最新价格 |
| Watch state | `provider/local/hunter_v7_signal_state.go` | 跨周期 watch 升级 | 只升级 4 类 watch，且升级后仍可能被 taker/zone 卡住 |
| Kernel tier | `kernel/engine.go` | EXECUTABLE/REVIEWABLE/WATCH/REJECTED | 与 prompt/trader 风控几何存在重复和不一致 |
| Prompt | `kernel/engine_prompt.go` | 候选展开、tag semantics、实时 compact data | 实时价格覆盖后不强制重分层；partial data wait 过强 |
| LLM parse | `kernel/engine_analysis.go` | JSON 解析、wait reason 校验 | wait reason 枚举较粗，难以精确反哺具体 gate |
| Trader guard | `trader/auto_trader_risk.go` | 最大持仓、价格漂移、SL/TP/RR、setup guard | guard 拒单未反写 signal record，闭环不完整 |
| 归因存储 | `store/hunter_v7_signal.go`, `store/hunter_v7_mover.go` | 信号与 mover 审计 | 尚未形成自动阈值校准与漏斗日报 |

## 3. 改造目标

### 3.1 交易目标

- 提升开仓率：增加真实 `EXECUTABLE/REVIEWABLE` 候选数量，而不是把 `WATCH` 直接放给 LLM 开仓。
- 提升胜率：不同形态使用不同确认组合，避免趋势跟随和均值回归共用同一套 taker/zone 阈值。
- 降低误杀：实时价格、RR、入场区、数据缺失的判断在 provider/kernel/prompt/trader 中保持一致。
- 降低误触发：保留 trader 硬风控，不放宽最大持仓、价格漂移、SL/TP 方向、最小 RR。

### 3.2 工程目标

- 每个信号都能解释自己卡在哪一层：`universe`、`module_match`、`router_priority`、`execution_geometry`、`kernel_tier`、`prompt_data_quality`、`llm_wait`、`trader_guard`。
- 每轮输出可观测：setup 分布、tier 分布、blocked gate 分布、mover recall。
- 所有阈值先集中到 setup profile/adaptive profile，不继续散落在 prompt 字符串和 helper 函数里。

## 4. 目标架构：双态漏斗

现有链路应改成“双态”：

```text
Discovery Funnel
  Snapshot
    -> Universe
    -> Regime
    -> Setup Match/Score
    -> Router priority
    -> Watch/Confirmed signals

Execution Readiness Funnel
  Signal + latest market data
    -> freshness check
    -> live price/zone check
    -> setup-specific confirmation profile
    -> execution geometry check
    -> tier classification
    -> prompt expansion
    -> trader guard
```

核心原则：

- `Discovery` 负责“发现可能有机会的形态”，允许更宽召回。
- `Execution Readiness` 负责“现在能不能进”，必须使用最新价格和同一套风控几何。
- `WATCH` 不是失败，而是状态机资产；只有满足升级条件才进入 `REVIEWABLE`。
- `REVIEWABLE` 是 LLM 可以复核的候选，不等于自动开仓。
- `EXECUTABLE` 是系统认为已经具备核心执行条件的候选，但仍必须通过 trader 硬风控。

## 5. P0：必须先修的一致性问题

### 5.1 Prompt 层实时价后强制重分层

文件：`kernel/engine_prompt.go`

当前逻辑：

```go
coin = hunterV7CandidateWithLiveMarketPrice(coin, data)
tier, reason := coin.V7ExecutionTier, coin.V7TierReason
if tier == "" {
    tier, reason = classifyHunterV7CandidateTier(coin)
}
```

问题：`coin.V7ExecutionTier` 通常已由 `kernel/engine.go` 在 scoring 后写入，因此 prompt 层不会用实时价格重新判断。

改造：

```go
coin = hunterV7CandidateWithLiveMarketPrice(coin, data)
tier, reason := classifyHunterV7CandidateTier(coin)
coin.V7ExecutionTier = tier
coin.V7TierReason = reason
```

验收：

- 新增测试：scoring price 在 zone 内、prompt live price 出 zone 后，tier 应降级或 reason 改变。
- 新增测试：scoring price 出 zone、prompt live price 回 zone 后，符合条件的信号可升回 REVIEWABLE。

### 5.2 Kernel RR cap 与 prompt/trader 统一

文件：`kernel/engine.go`, `kernel/engine_prompt.go`, `trader/auto_trader_risk.go`

当前状态：

- prompt 的 `effectiveExecutionGeometry()` 会根据 `MinRiskRewardRatio`、`MinStopLossPriceMovePct`、`MaxEntryPriceDeviationPct` 动态提高 `maxTPMovePct`。
- trader 的 `ensureHunterV7FeasibleTakeProfitCap()` 也有同样逻辑。
- kernel 的 `hunterV7BackendCappedRRWaitReason()` 仍使用硬编码 `hunterV7BackendMaxTPPct = 4.0`。

改造：

- 将 Hunter v7 execution geometry 抽成共享 helper，或在 `CandidateCoin` 写入 `V7ExecutionGeometry`。
- `hunterV7BackendCappedRRWaitReason()` 不再使用硬编码 TP cap，而使用与 prompt/trader 一致的 effective cap。
- 若短期不能传入 `StrategyEngine`，至少新增 `classifyHunterV7CandidateTierWithGeometry(coin, geometry)`，由 runtime/prompt 调用。

验收：

- 当 `MinRiskRewardRatio=2.0`、`MinStopLossPriceMovePct=2.0`、`MaxEntryPriceDeviationPct=0.5` 时，kernel/prompt/trader 计算出的 TP cap 都应为 `>=5.25%`。
- 不再出现 prompt 告诉 LLM 可行、kernel tier 提前标记 `backend_rr_infeasible` 的情况。

### 5.3 Missing fields 从全局 wait 改成分级 data quality

文件：`kernel/engine_prompt.go`

当前 `hunterV7CompactMissingFields()` 把 `taker_buy_15m`、`15m_atr`、`15m_vwap` 等缺失统一输出为：

```text
missing_fields_rule=wait_unless_all_required_confirmations_are_visible
```

改造：

- 分成三类：
  - `hard_missing`: `entry_zone`、`invalidation`、`target1`、`market_data`
  - `execution_missing`: 当前 setup 必需的 live confirmation，如 funding reversal 的 VWAP、taker flow
  - `context_missing`: ATR/EMA 等辅助字段
- 只有 `hard_missing` 必须禁止 open。
- `execution_missing` 对 `EXECUTABLE` 降为 `REVIEWABLE` 或 wait；对 `WATCH` 仅记录上下文。
- `context_missing` 不应触发全局 wait，只应降低 confidence 或提示 LLM 做更保守的仓位。

验收：

- `REVIEWABLE` 候选缺少 `15m_atr` 不再被 prompt 描述成必须 wait。
- `funding_reversal` 缺少 VWAP 时仍保持严格复核，但 blocked reason 应是 `confirmation_missing` 或更细的 `vwap_missing`，不是全局 no-op。

### 5.4 Open-review 展开数量动态化

文件：`kernel/engine_prompt.go`

当前完整展开上限固定为 5。

改造：

- 默认展开：
  - 所有 `EXECUTABLE`
  - `REVIEWABLE` 按 `AIPriority + readiness_score` 展开至动态上限
- 上限建议：
  - 无持仓：`min(8, exec+reviewable)`
  - 有持仓但未满仓：`min(6, exec+reviewable)`
  - 接近 token 限制时保留 top 5，但把其余候选输出 compact JSON，而不是单行摘要。

验收：

- 当 6 个以上 open-review 候选出现时，第 6 个至少有 compact execution JSON，不能只有自然语言摘要。

## 6. P1：Execution Readiness 统一模型

新增文件建议：`provider/local/hunter_v7_readiness.go`

### 6.1 数据结构

```go
type V7ReadinessTier string

const (
    V7ReadinessExecutable V7ReadinessTier = "EXECUTABLE"
    V7ReadinessReviewable V7ReadinessTier = "REVIEWABLE"
    V7ReadinessWatch      V7ReadinessTier = "WATCH"
    V7ReadinessRejected   V7ReadinessTier = "REJECTED"
)

type V7ExecutionReadiness struct {
    Tier              V7ReadinessTier `json:"tier"`
    Reason            string          `json:"reason"`
    ReadyScore         float64         `json:"ready_score"`
    WindowHealth       float64         `json:"window_health"`
    EntryZonePosition  float64         `json:"entry_zone_position"`
    PriceDeviationPct  float64         `json:"price_deviation_pct"`
    DataQuality        string          `json:"data_quality"` // complete/partial/stale
    MissingHard        []string        `json:"missing_hard,omitempty"`
    MissingExecution   []string        `json:"missing_execution,omitempty"`
    MissingContext     []string        `json:"missing_context,omitempty"`
    BlockedGate        string          `json:"blocked_gate,omitempty"`
    NextConfirmations  []string        `json:"next_confirmations,omitempty"`
}
```

### 6.2 Readiness 评分

建议公式：

```text
ready_score =
  setup_quality      * 0.25
  + timing_quality   * 0.20
  + flow_alignment   * 0.20
  + zone_health      * 0.15
  + rr_health        * 0.10
  + liquidity_health * 0.05
  + data_freshness   * 0.05
  - risk_penalty
```

注意：

- `ready_score` 用于排序与候选展开，不直接覆盖 trader 风控。
- `risk_extreme`、`liquidity_filtered`、SL/TP 方向错误仍是硬拒绝。
- 不同 setup 的权重可覆盖，例如回归类更重 zone/timing，趋势类更重 flow/momentum。

### 6.3 WindowHealth

```text
window_health =
  price_in_or_near_zone * 0.35
  + zone_depth          * 0.20
  + price_velocity_ok   * 0.15
  + taker_flow_aligned  * 0.15
  + confirmation_count  * 0.15
```

实现要点：

- 价格速度先用 snapshot 两轮价格差或 1m kline 近 3 根估算，不需要一开始引入复杂 WS 状态。
- `ZoneExitETA` 可以后置；第一版只做 `velocity_ok` 与 `distance_to_zone_edge`。
- 对 short 使用 taker buy 低位确认，对 long 使用 taker buy 高位确认。

## 7. P1：按形态重做 REVIEWABLE/EXECUTABLE 放行表

当前 `hunterV7ReviewableCandidateReason()` 中不同 setup 的条件差异较大，但还没有显式 profile。建议新增 setup profile：

```go
type V7SetupExecutionProfile struct {
    SetupType              V7SetupType
    Direction              V7Direction
    MinExecutableReady     float64
    MinReviewableReady     float64
    MinFlowLong            float64
    MaxFlowShort           float64
    RequirePassedReview    bool
    AllowCounterTrend      bool
    ZonePolicy             string // inside, near_lower, near_upper, breakout, retest
    MissingDataPolicy      string // hard_wait, reviewable_only, context_only
}
```

### 7.1 趋势跟随类

适用 setup：

- `leader_momentum_long`
- `trend_breakout_long`
- `displacement_momentum_long`
- `accumulation_breakout_long`

放行原则：

- 不要求价格一定在 entry zone 下沿；突破/位移可以用 `breakout_retest` 或 `trailing_entry`。
- 必须有至少两个趋势确认：4h/24h 相对强度、taker flow、OI 正增长、volume burst、breakout close。
- RSI/funding 过热时不直接拒绝，而是转 `REVIEWABLE + reduce_size_or_wait`，除非出现 `funding_extreme`、`momentum_overheated`、`chase_risk` 且无回踩。

建议 REVIEWABLE 条件：

```text
execution_quality in ready/near_confirm
ready_score >= 62
risk_score < 55
liquidity_score >= 50 or unknown
taker_buy_15m >= 0.49 if available
has any: oi_healthy_growth, oi_moderate_growth, oi_explosive_growth
has any: solid_4h_momentum, strong_4h_momentum, confirmed_breakout, taker_sustained_buy
```

### 7.2 回调延续类

适用 setup：

- `pullback_reversal_long`
- trend_up/range 中的 lower-zone reclaim

放行原则：

- 核心是“回踩到位 + 重新站回结构”，不是单纯 taker 强。
- entry zone 上沿之外不追，除非 `strong_4h_momentum + taker_sustained_buy` 明确成立。

建议：

- `EXECUTABLE`: 价格在 zone 内或不超过 upper 0.8%，taker >= 0.50，5m/15m reclaim，RR 可行。
- `REVIEWABLE`: 价格在 upper 1.5% 内，确认缺 1 项，但 flow/结构至少有一项已经转强。

### 7.3 恐慌反转/均值回归类

适用 setup：

- `panic_reversal_long`
- `range_reversion`
- `funding_reversal` 的逆拥挤反转

放行原则：

- 逆势交易必须更重视 `passed_review`，不能用高 priority 覆盖结构失败。
- 但 `passed_review` 必须基于最新价重算，不能只使用 scoring 快照。
- panic reversal 的核心确认是：卖压衰竭、强 reclaim、taker 回补、OI 不再 price-down build。

建议：

- `panic_reversal_long` 在 trend_down 中：
  - `passed_review=false`: 只能 WATCH/REVIEWABLE，不可 EXECUTABLE。
  - `passed_review=true + strong_reclaim + taker >= 0.52 + risk < 55`: 可 REVIEWABLE/EXECUTABLE。
- `range_reversion`：
  - range regime 权重高，但必须靠近区间边界。
  - 允许 taker flow 中性，不要求趋势类强 flow。

### 7.4 空头类

适用 setup：

- `distribution_short`
- `long_squeeze_short`
- `funding_reversal` SHORT
- `range_reversion` SHORT

放行原则：

- short 的 flow 条件应统一成 taker buy <= 阈值，而不是复用 long 的 strong taker。
- funding short 必须区分 OI building、flush、failed rebuild。
- 顶部派发 short 需要靠近上沿/retest，不能在急跌后追空。

建议：

- `funding_reversal SHORT`:
  - `OI building`: WATCH，除非出现 failed rebuild 或 flush。
  - `mixed OI + price failed retest + taker <= 0.50`: REVIEWABLE。
  - `flush + taker <= 0.48 + near retest`: EXECUTABLE。
- `distribution_short`:
  - `taker_buy_weakening + rally_stalling_near_high + zone_pos >= 60`: REVIEWABLE。

### 7.5 Watch/Pre-move 类

适用 setup：

- `pre_breakout_watch`
- `accumulation_watch`
- `pre_squeeze_watch`
- `pre_distribution_watch`

当前 `V7SignalStateManager.applyUpgrade()` 字段注入是正确的，下一步不是修注入，而是提高升级后的可执行解释力：

- 升级为 `REVIEWABLE` 时写入 `NextConfirmations`，说明下一轮要看什么。
- `watchReviewableTrigger()` 不只看 seen count，还看最近两轮 score/flow 是否改善。
- 对 `pre_breakout_watch`，价格接近 trigger 后，允许进入 `REVIEWABLE`，但只有 5m/15m close 触发后才能 `EXECUTABLE`。

## 8. P2：数据源与快照新鲜度

### 8.1 Freshness score

新增字段建议：

```go
type V7DataFreshness struct {
    PriceAgeSec       int `json:"price_age_sec"`
    Kline5mAgeSec     int `json:"kline_5m_age_sec"`
    Kline15mAgeSec    int `json:"kline_15m_age_sec"`
    OIAgeSec          int `json:"oi_age_sec"`
    FundingAgeSec     int `json:"funding_age_sec"`
    TakerFlowAgeSec   int `json:"taker_flow_age_sec"`
}
```

第一版可以只在 validation/report 中输出，不进入交易判断。第二版再让 stale data 降低 `ready_score`。

### 8.2 Detail quota 继续细化

已有改造把 amplitude、velocity、new activity 纳入 universe/detail。建议下一步把 detail quota 明确成可配置：

```text
volume_top:       35%
amplitude_top:    15%
velocity_top:     15%
oi_delta_top:     15%
funding_outlier:  10%
core_liquidity:   10%
```

验收：

- 每轮 validation 输出 detail source contribution。
- 24h amplitude >= 12% 的 mover，至少进入 universe；amplitude >= 20% 的 mover，优先进入 detail。

### 8.3 避免 REST 高频轮测

延续现有原则：

- `cmd/hunter_v7_validate` 默认低并发。
- 多轮验证必须 `--round-interval >= 90s`。
- 实盘优先复用 Collector/SnapshotStore，不要 prompt 阶段重复拉取 Binance。

## 9. P2：Prompt 与 LLM 决策协议优化

### 9.1 Prompt 展示新的 readiness

`hunter_v7_signal_json` 增加：

```json
{
  "execution_readiness": {
    "tier": "REVIEWABLE",
    "ready_score": 68.5,
    "window_health": 0.72,
    "blocked_gate": "confirmation_missing",
    "next_confirmations": ["5m_close_above_entry_zone", "taker_buy_15m_gt_0_52"]
  }
}
```

LLM 不再自己从大量 reason/risk tag 推断主因，而是优先读取 readiness。

### 9.2 blocked_reason_code 细化但保持兼容

现有枚举：

- `entry_not_in_zone`
- `rr_insufficient`
- `confirmation_missing`
- `oi_too_low`
- `funding_crowded`
- `account_risk`
- `backend_guard_risk`
- `no_reviewable_candidate`

建议新增内部 `blocked_gate_detail`，不破坏外部枚举：

```json
{
  "blocked_reason_code": "confirmation_missing",
  "blocked_gate_detail": "taker_flow_missing"
}
```

第一版可以只在 reasoning 或 signal record 中保存，避免修改所有解析逻辑。

## 10. P3：交易执行与反馈闭环

### 10.1 trader guard 结果反写

当前 `validateHunterV7ExecutionGuard()` 和 `validateOpenDecision()` 拒单后，拒单原因主要在日志里。建议：

- 将 trader guard 拒单写入 `hunter_v7_signal_records` 或新增 `hunter_v7_execution_attempts`。
- 字段包括：`decision_action`、`decision_price`、`execution_price`、`guard_stage`、`guard_reason`、`sl_pct`、`tp_pct`、`rr`。

这样可以区分：

- Hunter 没召回；
- Hunter 召回但 tier 卡住；
- LLM wait；
- LLM open 但 trader guard 拒单；
- 实际成交后亏损。

### 10.2 不放宽硬风控

以下保持硬边界：

- `MaxPositions`
- 价格漂移 `MaxEntryPriceDeviationPct`
- SL/TP 方向正确性
- 最小止损距离
- 最小 RR
- 单笔最大亏损
- setup guard 中的 OI flush 与 zone position

开仓率提升应来自：

- 更好召回；
- 更准确 tier；
- 更完整 prompt；
- 更少不一致误杀；
- 更细的 REVIEWABLE 复核。

## 11. P3：可观测性与日报

基于现有 `hunter_v7_signal_records`、`hunter_v7_mover_labels`，新增每日报告：

### 11.1 漏斗指标

```text
universe_total
module_matched_total
router_output_total
ready_count
near_confirm_count
watch_count
executable_count
reviewable_count
llm_open_count
trader_guard_reject_count
actual_order_count
```

### 11.2 形态覆盖

按 setup 输出：

- 出现次数
- REVIEWABLE 率
- EXECUTABLE 率
- LLM open 率
- trader guard 通过率
- 1h/4h/24h 后收益分布
- 平均最大顺向波动 MAE/MFE

### 11.3 Mover recall

按 amplitude 分桶：

- 12-20%
- 20-35%
- 35%+

统计：

- first_seen lead time
- first_watch lead time
- first_reviewable lead time
- missed_stage
- common blocked_gate

## 12. 分阶段实施路线

### P0：一致性修复，1-2 天

| 文件 | 改动 | 验收 |
|---|---|---|
| `kernel/engine_prompt.go` | 实时价覆盖后强制重分层 | prompt tier 与 live price 一致 |
| `kernel/engine.go` | RR cap 与 prompt/trader 对齐 | `MinRR=2.0` 不再被硬编码 4% 误杀 |
| `kernel/engine_prompt.go` | missing fields 分级 | partial data 不再全局 wait |
| `kernel/engine_prompt.go` | open-review 动态展开 | 6+ 候选仍有结构化信息 |

### P1：Execution Readiness，3-5 天

| 文件 | 改动 | 验收 |
|---|---|---|
| `provider/local/hunter_v7_readiness.go` | 新增 readiness 结构与评分 | 每个 v7 signal 都有 blocked gate |
| `provider/local/hunter_v7_execution.go` | finalize 后计算 readiness | ready_score 与 tier 可解释 |
| `kernel/engine.go` | tier 使用 readiness 优先 | helper 条件减少重复 |
| `kernel/engine_prompt.go` | 输出 readiness JSON | LLM 读取统一执行态 |

### P2：Setup profile 与动态阈值，1 周

| 文件 | 改动 | 验收 |
|---|---|---|
| `provider/local/hunter_v7_profiles.go` | setup execution profile | 趋势/回归/short 使用不同门槛 |
| `provider/local/hunter_v7_weights.go` | regime + volatility 自适应 | 高波动时不开无确认追单 |
| `provider/local/hunter_v7_signal_state.go` | watch 升级看 score/flow 改善 | watch 升级更少空转 |

### P3：数据新鲜度与归因，1 周

| 文件 | 改动 | 验收 |
|---|---|---|
| `datafetch/*` | freshness metadata | 报告展示字段年龄 |
| `store/*` 或新增 execution attempts | trader guard 反写 | 能查到 LLM open 后为何未成交 |
| `cmd/hunter_v7_validate` | readiness/tier/gate 报告 | 单次报告即可定位瓶颈 |
| `cmd/hunter_v7_mover_audit` | setup/tier lead time | mover recall 可量化 |

### P4：回测与参数校准，持续

| 模块 | 内容 |
|---|---|
| 回放 | 使用历史 signal records/mover labels 回放 readiness 阈值 |
| 参数 | 按 setup 优化 `MinReviewableReady`、flow 阈值、zone 容忍度 |
| 风控 | 按 setup 评估不同仓位折扣，不改硬 SL/TP/RR |

## 13. 验收标准

### 13.1 静态验收

```bash
go test ./provider/local ./kernel ./trader
go test ./cmd/hunter_v7_validate
go build ./...
```

### 13.2 实时低频验收

```bash
go run ./cmd/hunter_v7_validate \
  --rounds 3 \
  --round-interval 120s \
  --top-detail 220 \
  --max-workers 8 \
  --max-output 40 \
  --watch-output 8 \
  --min-priority 45 \
  --aggressive \
  --out-dir reports
```

### 13.3 业务指标

短期目标：

- 每轮至少稳定出现 1-3 个 `REVIEWABLE`，且不全部来自 `funding_reversal`。
- prompt tier 与 runtime/trader guard 的拒单原因一致。
- `backend_rr_infeasible` 因配置不一致造成的误杀降为 0。
- `missing_fields_rule` 不再成为 EXECUTABLE/REVIEWABLE 的全局 wait。

中期目标：

- mover first_seen recall 提升。
- mover first_reviewable lead time 可量化。
- LLM open 后 trader guard reject 率下降。
- 按 setup 统计的 open 后 MFE/MAE 改善。

## 14. 实施注意事项

1. 不建议直接把 `MinAIPriority` 大幅下调来提高开仓率；这会增加 prompt 噪音。
2. 不建议把 `WATCH` 候选直接交给 LLM 开仓；应先通过 watch state/readiness 升级。
3. 不建议在 prompt 中继续增加大量自然语言特例；跨层规则应沉到 readiness/profile/helper。
4. 不建议一次性重写所有 setup 模块；先统一执行态，再逐个 setup profile 校准。
5. 所有参数调整都必须通过 signal records 与 mover audit 复盘，至少观察 1-2 天样本。

## 15. 最小落地清单

最快可见效的最小版本：

1. `engine_prompt.go`：实时价后强制 `classifyHunterV7CandidateTier()`。
2. `engine.go`：RR cap 与 prompt/trader effective geometry 对齐。
3. `engine_prompt.go`：missing fields 分级，不再全部触发 wait。
4. `engine_prompt.go`：open-review 展开从固定 5 改成动态。
5. `provider/local/hunter_v7_readiness.go`：先只实现 `ready_score`、`window_health`、`blocked_gate` 三个字段。
6. validation report 输出 `ready_score/window_health/blocked_gate` 分布。

这 6 步完成后，再按 setup profile 精修阈值，避免在当前状态不一致的基础上继续叠加复杂规则。
