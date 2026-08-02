# Hunter v7 币安实时 3 轮 + 30m Outcome 优化效果复审报告 - 2026-08-02

> 测试时间：2026-08-02 07:22-08:03 CST  
> 数据目录：`reports/hunter-v7-binance-live-3round-post30m-20260802`  
> 命令：`go run ./cmd/hunter_v7_validate -rounds=3 -round-interval=5m -top-detail=220 -max-workers=8 -max-output=30 -watch-output=8 -min-priority=45 -aggressive=true -post-track-duration=30m -post-track-interval=60s -out-dir=reports/hunter-v7-binance-live-3round-post30m-20260802`

## 1. 结论

本轮三轮数据质量全部有效：REST errors=0，universe coverage 40.2%-41.0%，没有 degraded round。优化后的 valid-round open-review rate 为 1/27 = 3.7%，较 2026-08-02 06:35 轮的 4/21 = 19.0% 明显下降。

唯一真实跟踪标的是 `MMTUSDT SHORT breakdown_momentum_short`，30 分钟内触发 STOP，PnL -2.032%，MFE +0.656%，MAE -2.236%。严格结案胜率为 0/1 = 0%。

核心判断：`alt_ladder_breakdown_short` 的反抽失败门有效降低了直接开仓率，并拦截了部分会止损的样本；但当前规则过度保守，把全部 8 条 alt-ladder short 都拦为 WATCH，其中事后测算有 2 条可达 TP0、3 条仍保持浮盈。需要从“硬拦截”调整为“高风险硬拦 + 强流/低风险软放行 + MFE 后保本保护”。

## 2. 三轮开仓率

| Round | Regime | Signals | Open-review | Open-rate | REST errors | Universe | Degraded |
|---:|---|---:|---:|---:|---:|---:|---|
| 1 | trend_down | 9 | 0 | 0.0% | 0 | 214 | false |
| 2 | trend_down | 9 | 0 | 0.0% | 0 | 211 | false |
| 3 | trend_down | 9 | 1 | 11.1% | 0 | 215 | false |
| 合计 | trend_down | 27 | 1 | 3.7% | 0 | - | false |

新 validator 的 degraded round 统计生效，本轮没有异常轮次，因此 valid open-review rate 与 all open-review rate 一致。

## 3. 各形态路由与开仓

| Setup | Final rows | Open-review | WATCH | REJECTED | 主要拦截原因 |
|---|---:|---:|---:|---:|---|
| alt_ladder_breakdown_short | 8 | 0 | 8 | 0 | `alt_ladder_short_rebound_pending` |
| breakdown_momentum_short | 1 | 1 | 0 | 0 | 唯一进入 REVIEWABLE |
| funding_reversal | 9 | 0 | 6 | 3 | `15m_close_below_vwap` / `liquidity_lt_50` |
| panic_reversal_long | 3 | 0 | 0 | 3 | `liquidity_lt_50` |
| pre_distribution_watch | 3 | 0 | 3 | 0 | `confirmation_missing_summary` |
| whale_flow_reversal | 3 | 0 | 3 | 0 | `taker_flow_confirms_long` |

`funding_reversal` 继续保持 watch/rejected 拆桶，未污染主开仓率。`alt_ladder_breakdown_short` 被新门完全收紧，是本轮开仓率下降的主因。

## 4. 真实跟踪标的

| Symbol | Setup | Dir | Tier | Entry zone | Stop | TP0 | TP1 | TP2 | Status | PnL% | MFE% | MAE% |
|---|---|---|---|---|---:|---:|---:|---:|---|---:|---:|---:|
| MMTUSDT | breakdown_momentum_short | SHORT | REVIEWABLE | 0.1825059-0.1840980 | 0.1870260 | 0.1809001 | 0.1685325 | 0.1567170 | STOP | -2.032 | +0.656 | -2.236 |

问题：

- MMTUSDT 曾达到 MFE +0.656%，但当前 breakeven 保护只覆盖 `alt_ladder_breakdown_short`，没有保护 `breakdown_momentum_short`。
- 该样本说明“短空 continuation 类信号 MFE >= 0.60% 后未及时保护”会把可保护浮盈转成 -2% 止损。

## 5. alt-ladder 被拦样本事后测算

对 8 条 `alt_ladder_short_rebound_pending` 拉取信号后 1m K 线，用相同 entry/stop/TP0 规则估算若旧规则放行的结果：

| Symbol | Round | Hypothetical status | PnL% | MFE% | MAE% | 评估 |
|---|---:|---|---:|---:|---:|---|
| GRVTUSDT | 1 | WIN_TP0 | +0.322 | +1.692 | -0.653 | 被误杀，强流低风险 |
| BTWUSDT | 1 | ACTIVE | +1.192 | +2.538 | -1.741 | 浮盈但波动大 |
| CAPUSDT | 1 | STOP | -2.157 | +1.275 | -2.863 | 拦截正确；若有保本保护可减亏 |
| CAPUSDT | 2 | STOP | -2.204 | +0.620 | -2.707 | 拦截正确；触发保本阈值 |
| BANKUSDT | 2 | ACTIVE | +0.923 | +1.489 | -0.344 | 被误杀，低 MAE |
| BTWUSDT | 2 | STOP | -2.091 | +0.000 | -2.504 | 拦截正确 |
| CAPUSDT | 3 | STOP | -2.097 | +0.500 | -2.831 | 拦截正确 |
| GRVTUSDT | 3 | WIN_TP0 | +1.115 | +1.463 | +0.000 | 被误杀，强流低风险 |

事后测算口径下：

- TP0：2/8
- STOP：3/8
- ACTIVE 浮盈：3/8
- 若把 ACTIVE 浮盈算为未结案，严格 TP0 胜率 2/(2+3)=40%。
- 若加入 MFE>=0.60% 后 breakeven，CAP round1/2 的 -2% STOP 可被显著改善，alt-ladder 的风险收益会更接近可用。

## 6. 优化效果评估

### 已产生正向效果

1. 数据质量控制有效。
   - 本轮 3/3 valid rounds，run summary 明确输出 valid open-review rate。
   - 之前首轮 REST 异常污染统计的问题已解决。

2. outcome 分母继续正确。
   - final open-review=1，tracked=1。
   - WATCH/no-tier 没有进入真实 outcome。

3. alt-ladder 反抽失败门确实降低亏损暴露。
   - CAPUSDT、BTWUSDT 等旧规则可能放行的止损样本被拦截。

### 新暴露问题

1. 开仓率被压得过低。
   - 从同日上一轮 19.0% 降到 3.7%。
   - alt-ladder 从 4/6 放行降到 0/8 放行。

2. 胜率没有提升，反而由替代形态贡献亏损。
   - 唯一 open-review 是 `breakdown_momentum_short`，结案 STOP。
   - 当前严格胜率 0/1。

3. breakeven 保护覆盖面不足。
   - MMTUSDT MFE +0.656% 后仍 -2.032% STOP。
   - CAPUSDT 事后测算也多次先有 MFE 后止损，说明保护逻辑应覆盖 short continuation 家族。

## 7. 可实施优化方案

### P0：alt-ladder 从硬拦截改为分层放行

当前规则：

- 缺 `no_new_high_after_rejection` 一律 WATCH。

建议改为：

- 硬拦截：
  - `high_volatility`
  - `funding_elevated` + `execution_stop_tightened`
  - `alt_ladder_downshift_late` 且无 `alt_ladder_multi_cycle_close_through`
  - taker_buy_15m > 0.42
- 软放行 REVIEWABLE：
  - `alt_ladder_downshift_early/mid`
  - 无 danger risk tag
  - liquidity >= 70
  - taker_buy_15m <= 0.38，或 `alt_ladder_new_shorts` 且 OI 1h > 1%
  - stop distance <= 2.2%
  - 必须启用 MFE>=0.60% breakeven

这样 GRVT 类强流低风险样本可进入 REVIEWABLE，CAP/BTW late-high-volatility 样本继续拦截。

### P0：breakeven 保护扩展到 short continuation 家族

当前只覆盖：

- `alt_ladder_breakdown_short`

建议扩展到：

- `alt_ladder_breakdown_short`
- `breakdown_momentum_short`
- `relative_weakness_short`
- `range_expansion_event` SHORT

规则：

- MFE >= 0.60% 后 stop 推到 entry。
- 若 MFE >= 1.20%，stop 推到 +0.25% locked profit。
- 对 high-volatility short，阈值降到 0.50%。

### P1：breakdown_momentum_short REVIEWABLE 加反抽失败门或保护门

MMTUSDT 的亏损说明：仅缺 `5m_or_15m_close_below_trigger` 时 live-reviewable 不够安全。

建议：

- 缺 `no_new_high_after_rejection` 时，`breakdown_momentum_short` 只能 REVIEWABLE-small，不进入主开仓池。
- 或要求 stop distance <= 1.6%，否则 WATCH。
- 若仍放行，必须套用 continuation breakeven。

### P1：WATCH missed-opportunity 长期写入

本轮事后测算说明 WATCH 样本里有 TP0 与浮盈机会。建议 validator/live 将最终 WATCH 的高优先级信号写入长期 missed-opportunity outcome：

- 不计入主胜率。
- 记录 30m/60m MFE、MAE、TP0 touch、would_stop。
- 用于调参，而不是靠人工事后脚本。

## 8. 下一轮验证目标

实施 P0 后复测 3 轮 + 30m：

- valid open-review rate 回到 8%-15%。
- alt-ladder open-review 不超过 30% 的 raw alt-ladder 输出。
- strict terminal win rate >= 50%。
- loss STOP 平均亏损控制在 -0.8% 以内。
- MFE>=0.60% 后亏损 STOP 数量为 0。
