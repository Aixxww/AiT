# Hunter v7 Binance 实时三轮重跑跟踪与优化评估报告 - 2026-08-05

> 触发原因：上一轮 post-track 期间出现 Binance 1m K 线 EOF，且 ACTIVE 未结案，已作废，不纳入本报告。  
> 测试窗口：2026-08-05 05:35:14 / 05:41:06 / 05:46:58 CST。  
> 数据源：Binance USDS-M Futures public REST，只读未下单。  
> 目录：`reports/hunter-v7-live-3round-5m-redo-20260805`。  
> 命令：`TZ=Asia/Shanghai go run ./cmd/hunter_v7_validate -rounds=3 -round-interval=5m -top-detail=220 -max-workers=8 -max-output=30 -watch-output=8 -min-priority=45 -aggressive=true -post-track-duration=10m -post-track-interval=60s -out-dir=reports/hunter-v7-live-3round-5m-redo-20260805`

## 1. 核心结论

1. 本次重跑 3/3 valid rounds，三轮均为 `compression`，未 degraded；每轮 REST errors=1/524，错误率 0.2%，属于轻微部分覆盖，不影响主结论，但胜率结论需保守。
2. prompt-final 主开仓/复核率为 6/37 = 16.2%，较之前低开仓轮 3.3% 明显恢复，说明刚优化后没有继续压死开仓率。
3. 三轮均无 `EXECUTABLE`，全部开放面为 `REVIEWABLE`。系统恢复了候选池，但最终直接开仓门仍偏谨慎。
4. outcome tracker 跟踪 6 条 REVIEWABLE：4 条 `PROTECTED_STOP`，1 条 `BOTH_SAME_1M`，1 条 `ACTIVE` 浮亏；10 分钟内无硬亏损 `STOP`，但也无 TP0 partial/TP1/TP2 真实触发。
5. 优化效果总体正向：开仓率恢复、保护止损有效、funding 拆桶仍未污染主开仓率。但盈利质量还没有达到高优，主要问题是“可复核候选多，直接可执行信号少，TP0 触发不足”。

## 2. 三轮开仓率

以 validator 官方 run summary 的 prompt-final open-review 为准：

| Round | Time | Regime | Universe | Signals | Prompt Open-review | Open-rate | REST errors | Degraded |
|---:|---|---|---:|---:|---:|---:|---:|---|
| 1 | 05:35:14 | compression | 211 | 13 | 3 | 23.1% | 1 | false |
| 2 | 05:41:06 | compression | 207 | 12 | 1 | 8.3% | 1 | false |
| 3 | 05:46:58 | compression | 202 | 12 | 2 | 16.7% | 1 | false |
| 合计 | - | compression | - | 37 | 6 | 16.2% | - | 3/3 valid |

补充口径：

- raw/runtime open-review 曾显示 8 条，但 prompt-final 主开仓口径为 6 条。
- outcome tracker 实际跟踪 6 条 REVIEWABLE，其中包含 DB 层可跟踪 thesis；报告以 prompt-final 统计开仓率，以 tracker 统计盈亏路径。
- 三轮 `EXECUTABLE=0`，说明直接开仓率仍为 0%，当前所有机会都需要现场确认。

## 3. 各形态路由表现

| Setup | 三轮信号数 | Prompt-final open-review | 主表现 |
|---|---:|---:|---|
| `trend_breakout_long` | 9 | 3 | compression 下主要 review 来源，但仍需 close-through；未直接 EXECUTABLE 是合理保守 |
| `funding_reversal` | 9 | 0 | 拆桶继续有效，未污染主开仓率 |
| `whale_flow_reversal` | 6 | 1-2 | HEI/PUMP 有较好 MFE，但 taker 确认不足，适合 REVIEWABLE |
| `alt_ladder_momentum_long` | 6 | 1 | SKYAI 保护退出盈利，但 late-stage 风险仍高，不宜继续放宽 |
| `range_expansion_event` | 2 | 0 | raw 候选靠前，但 prompt-final 拦截；当前轮未证明可直接放开 |
| `mms_trend_ride_long` | 1 | 1 | DEXE 仍 ACTIVE 浮亏，是本轮最明显的质量问题 |
| `pre_distribution_watch` | 2 | 0 | 观察池，未进主开仓率 |

本轮有 2 个 watch 信号被 state manager 跨轮升级，说明跨轮确认机制在工作；但 prompt-final 仍将部分 raw candidate 留在 WATCH，未出现过度放宽。

## 4. 盈亏、止盈止损、保护止盈

10 分钟 post-track 结果：

| Status | Count | Profit | Loss | Flat | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|---:|
| `PROTECTED_STOP` | 4 | 3 | 0 | 1 | +0.226 | +0.820 | -0.068 |
| `BOTH_SAME_1M` | 1 | 1 | 0 | 0 | +1.986 | +2.969 | 0.000 |
| `ACTIVE` | 1 | 0 | 1 | 0 | -0.902 | +0.003 | -1.031 |

逐笔明细：

| Symbol | Dir | Setup | Tier | Status | PnL% | MFE% | MAE% | 判断 |
|---|---|---|---|---|---:|---:|---:|---|
| SKYAIUSDT | LONG | `alt_ladder_momentum_long` | REVIEWABLE | PROTECTED_STOP | +0.669 | +1.608 | 0.000 | late ladder 有收益但应继续保守仓 |
| DEXEUSDT | LONG | `mms_trend_ride_long` | REVIEWABLE | ACTIVE | -0.902 | +0.003 | -1.031 | retest hold 质量不足，需降权 |
| GRVTUSDT | LONG | `trend_breakout_long` | REVIEWABLE | PROTECTED_STOP | +0.203 | +0.478 | -0.092 | 小幅保护，未到 TP0 |
| SIRENUSDT | LONG | `trend_breakout_long` | REVIEWABLE | PROTECTED_STOP | +0.033 | +0.349 | -0.059 | 保护有效，但盈利弹性弱 |
| HEIUSDT | LONG | `whale_flow_reversal` | REVIEWABLE | BOTH_SAME_1M | +1.986 | +2.969 | 0.000 | 有强弹性，但同 1m 双触发需中性口径 |
| PUMPUSDT | LONG | `whale_flow_reversal` | REVIEWABLE | PROTECTED_STOP | +0.000 | +0.844 | -0.121 | 保护止损有效，未兑现 TP0 |

关键读数：

- 无硬亏损 STOP，是本轮最正向的风控结果。
- `PROTECTED_STOP` 生效，但 4 条均未触发 TP0 partial，说明保护逻辑有效、止盈兑现不足。
- `BOTH_SAME_1M` 不能简单算满胜，应按中性口径单列；HEIUSDT 方向判断对，但高波动路径需要更细 1m 内路径处理。
- DEXEUSDT 是本轮主要负面样本：`mms_trend_ride_long` 的低量回踩/EMA retest 未转化为 MFE，应加强现场确认。

## 5. 对刚优化效果的评估

### 正向影响

1. 开仓率从此前过紧状态恢复到 16.2%，位于目标 8%-15% 略上方，但考虑本轮全为 REVIEWABLE，实际风险仍可控。
2. `funding_reversal` 仍为 WATCH/事件桶，三轮 9 个 funding 输出没有进入主开仓率，拆桶策略没有回退。
3. `alt_ladder_momentum_long` 未被压死，且 SKYAI 走出 +1.608% MFE、+0.669% 保护退出；但 late-stage 标签仍正确要求保守。
4. `whale_flow_reversal` 出现弹性样本，HEIUSDT MFE +2.969%，说明该形态在 compression 环境有增益潜力。
5. 无硬亏损 STOP，说明保护止损/保本退出没有破坏胜率。

### 副作用与不足

1. `EXECUTABLE=0`：开仓面恢复主要体现在 REVIEWABLE，交易引擎直接开仓仍不足。
2. TP0 partial exits=0：短期收益兑现能力不够，保护止损多但实际减仓止盈没有触发。
3. `mms_trend_ride_long` 质量偏弱：DEXE 10 分钟后仍 ACTIVE 浮亏 -0.902%，MFE 只有 +0.003%。
4. `range_expansion_event` raw 候选靠前但 prompt-final 全拦，说明反抽失败/追高保护偏保守；本轮样本不支持立刻放宽，只能继续观察。
5. 数据质量仍有轻微 REST errors=1/轮，应继续要求 valid_rounds=3/3 后才做调参判断。

## 6. 可实施优化方案

### P0：执行面口径治理

1. 报告和前端同时展示三层口径：`runtime_open_review`、`prompt_final_open_review`、`tracked_outcome_count`。
2. 主开仓率只用 prompt-final `EXECUTABLE + REVIEWABLE`；真实胜率只用 tracker terminal outcome；raw/runtime 只用于漏斗诊断。
3. 对 `BOTH_SAME_1M` 单列中性状态，不计入严格 TP 胜率，也不计入亏损止损。

### P1：提高 REVIEWABLE 到 EXECUTABLE 的质量，而不是继续扩大候选池

1. `trend_breakout_long`：保持 close-through 必要条件；若连续两轮同 symbol 接近 upper trigger 且 taker/OI 扩张，可允许小仓 `REVIEWABLE+`，但不直接 EXECUTABLE。
2. `whale_flow_reversal`：增加 “MFE 快速达到 0.8%-1.0% 时保护止损/减仓建议” 的策略提示，因为本轮 HEI/PUMP 的弹性来自快速 MFE，而非稳态趋势。
3. `alt_ladder_momentum_long`：late-stage 仍只允许保守仓；如果 entry_zone_position > 60 且 5m close_vs_vwap 已转弱，不升 EXECUTABLE。
4. `mms_trend_ride_long`：增加 5m EMA20/VWAP hold 的硬确认，若 `close_vs_vwap20 < 0` 且 `volume_vs_avg5 < 0.8`，从 REVIEWABLE 降为 WATCH。
5. `range_expansion_event`：不因本轮 raw 靠前就放宽；继续要求反抽失败/不创新高/方向 taker 对齐，至少再观察 6-9 轮。

### P2：止盈保护优化

1. 对 MFE >= 0.8% 但未到 TP0 的 REVIEWABLE 样本，允许策略提示建议 “减风险/推紧保护” 而非等待完整 TP0。
2. TP0 距离过远的 setup 应输出 `tp0_distance_pct` 风险提示：本轮多条 TP0 在 1.2%-2.18%，compression 环境下不易触发。
3. 对 `PROTECTED_STOP` 的报告增加 “未触发 TP0 但保护退出” 子类，避免误读为已完成止盈。

## 7. 下一步建议

当前不建议继续整体放宽。更合理的下一步是：

1. 修 `mms_trend_ride_long` 的 VWAP/EMA hold 确认，避免 DEXE 这类低 MFE 浮亏。
2. 增强 `whale_flow_reversal` 的快保护逻辑，把 HEI/PUMP 这类 MFE 转化为实际 TP0 或保护收益。
3. 保持 funding 拆桶和 range event 拦截，不因短期 open-rate 目标牺牲胜率。
4. 再跑至少 2 组 3 轮，在 trend_up/trend_down regime 下分别确认本次优化是否稳定。

结论：本轮重跑证明优化后开仓/复核率已恢复，且没有引入明显硬止损；但高质量盈利还不充分。当前优化方向应从“扩大候选”转为“提升 REVIEWABLE 到 EXECUTABLE 的确认质量”和“把 MFE 转成 TP0/保护收益”。
