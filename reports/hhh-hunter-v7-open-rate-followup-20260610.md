# HHH Hunter v7 Open-Rate Follow-up - 2026-06-10

## Scope

复盘 HHH 周期 #178-#189，重点跟踪：

- 为什么 #178 出现数据源 `candidate` 但策略层 `no_open_review_candidates`。
- #179/#181 开仓是否说明漏斗恢复。
- #180 为什么对已平仓 LABUSDT 输出 `hold`。
- LABUSDT 连续两次逆 trend_down 开多被止损的根因。

## Findings

### 1. #178 不是数据源筛选卡死

#178 后端日志显示 Hunter v7 数据源已经给出候选：

- `SIRENUSDT LONG panic_reversal_long status=candidate execution_quality=ready`
- `KATUSDT LONG displacement_momentum_long status=candidate execution_quality=ready`

但落库后：

- SIREN 被内核分层降为 `WATCH needs_confirmation`
- KAT 被 `displacement_rr_insufficient` 拒绝

根因是内核把 `funding_extreme` 当作无条件危险标签，导致 SIREN 这类“极端资金费率 + 恐慌反转”的强确认信号无法进入 EXECUTABLE/REVIEWABLE。这个标签在趋势动量里应偏危险，但在 counter-crowd panic reversal 中可能是反转条件的一部分，不能无条件硬拦截。

### 2. #171-#177 多数等待是合理风控

这些周期里 LLM 已经收到 EXECUTABLE/REVIEWABLE 候选，主要等待原因是：

- 5m close 未站上 EMA20 或 entry_zone_mid。
- taker_buy_15m 不足。
- stop distance/RR 组合不达标。

因此这一段不是“没有候选”，而是确认门槛生效。问题集中在 #178 的标签语义与内核分层不一致。

### 3. #180 暴露持仓缓存滞后

时间线：

- #179 于 2026-06-10 10:21:40 自动开 LABUSDT LONG。
- 交易所在 10:29:54 触发平仓，实际已关闭。
- #180 于 10:29:57 构建 AI context 时仍读到 10:29:43 的 Binance position cache，于是 LLM 对已关闭仓位输出 `hold`。
- 10:30:43 订单同步才补录 close trades。

根因是自动决策上下文复用交易所 15 秒持仓缓存，在 SL/TP 刚触发的边界窗口可能传入幽灵持仓。

### 4. LABUSDT 连续两轮逆势止损

LABUSDT 两笔亏损：

- #179: 2026-06-10 10:21:40 LONG entry 9.167, exit 8.971, realized PnL -0.392, held 493s。
- #181: 2026-06-10 10:37:58 LONG entry 9.073, exit 8.884, realized PnL -0.378, held 1634s。

共同特征：

- market_regime/trend context 仍是 `trend_down`。
- 24h 跌幅约 23%-24%，4h 结构仍为负。
- timing 只有 45，属于边缘可执行。
- LLM 把 5m close 轻微站上 EMA20 当成确认，其中 #181 仅高约 0.15%。
- taker_buy_15m 约 0.55-0.58，未达到强反转级别。
- 止损距离贴近后端 2% 最小边界，行情继续下探时容错不足。

结论：这不是移动止盈过敏，也不是数据源问题，而是 `panic_reversal_long` 在 trend_down 深跌环境下的开仓确认过松。5m EMA 微弱 reclaim 不足以证明 15m/1h 结构反转。

## Changes

### Hunter v7 分层

- 新增 `hunterV7DangerRiskTagBlocksOpenReview`，将危险标签检查改为上下文感知。
- `funding_extreme` 在强确认 `panic_reversal_long` 中不再无条件阻断：
  - 必须是 `candidate + ready`
  - risk < 55
  - priority/setup/timing 达标
  - liquidity >= 50
  - taker_buy_15m >= 0.52
  - 具备 `strong_reclaim`、卖压衰竭、OI 下降/flush 证据
- 弱确认的 `funding_extreme` 仍保持 WATCH。

### Prompt/tag 语义

- `funding_extreme` 从 `wait_only` 调整为 `reduce_size_or_wait`。
- Prompt policy 明确 `reduce_size_or_wait` 只有在 live entry-zone、flow、stop、RR 均确认后才允许保守仓位开仓。

### 持仓快照

- Binance Futures 新增 `InvalidatePositionCache()`。
- 每轮 AI context 构建前强制清一次交易所持仓缓存，确保决策使用最新仓位。

### Trend-down panic reversal 收紧

- 新增 `panic_reversal_trend_down_structure_wait` 分层原因。
- 对 `trend_down/regime_against_direction + 24h 深跌 + 4h 仍下行 + timing<=45` 的 panic reversal long 降级为 WATCH。
- 只有强反转例外才允许继续 EXECUTABLE：
  - taker_buy_15m >= 0.62
  - 1h 反弹 >= 3%
  - 4h 下跌已明显收敛
  - `strong_reclaim` + 卖压衰竭 + OI flush/declining 同时成立
- Prompt 同步要求：trend_down 深跌反转不能只靠 5m EMA 微幅站上，优先等待 15m EMA/VWAP 收复或连续 5m 站稳。

### Same-symbol 连亏冷却

- Hunter v7 候选新增同 symbol+方向冷却：
  - 60 分钟内同 symbol+方向出现 2 次以上亏损，且每次 PnL% <= -5%，该候选直接从开仓池移除。
- 该规则用于阻止 LABUSDT 这种连续止损后马上第三次重开。

## Live Verification

重启后 #181 曾验证开仓漏斗恢复，但随后 LABUSDT 再次止损，说明“开仓率”已恢复，主要问题转为“逆势反转质量不足”。

当前最新验证：

- #189 已无持仓，订单同步显示 `Position symbols found: 0`。
- 新分层会把 #181 这类 LABUSDT 信号降级为 WATCH `panic_reversal_trend_down_structure_wait`。
- LABUSDT 在两次止损后会触发 same-symbol recent-loss cooldown，防止第三次快速重开。

## Verification Commands

- `go test ./provider/local ./kernel -run 'HunterV7|ClassifyHunterV7CandidateTier|DescribeHunterV7Tags' -count=1`
- `go test ./trader ./trader/binance ./kernel ./provider/local -count=1`
- `go build ./...`
