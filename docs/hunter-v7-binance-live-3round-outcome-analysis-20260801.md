# Hunter v7 Binance 实时三轮全链路跟踪报告

生成时间：2026-08-01 06:55 CST
测试窗口：2026-08-01 06:15:56 / 06:21:44 / 06:27:32 CST
数据目录：`reports/hunter-v7-binance-live-3round-20260801/`
后跟踪窗口：20 分钟，每 30 秒刷新 1m candle
数据库窗口：`hunter_v7_signal_records.timestamp >= 2026-07-31 22:10:00 UTC`

## 1. 执行结论

- 三轮 Binance 实时拉数均完成，JSON/prompt 校验通过，三轮 REST errors 均为 0。
- 三轮输出信号 27 条：LONG=8，SHORT=19；三轮市场 regime 均为 `trend_down`。
- prompt-final 可开仓口径：EXECUTABLE=0，REVIEWABLE=8，open-rate=8/27=29.6%，高于 18%-25% 目标区间。
- DB runtime 真实 open-review 也是 8 条，全部为 `alt_ladder_breakdown_short`，没有 EXECUTABLE。
- outcome tracker 跟踪 10 条：WIN_TP1=1，WIN_TP0=1，STOP=5，ACTIVE=3；其中 STOP 5 条全部为正收益 protected stop。
- 严格 DB open-review 口径下，`alt_ladder_breakdown_short` 8 条：TP0=1，protected stop=4，ACTIVE loss=3，loss stop=0；已结案正收益率=5/5=100%，全样本正向/保护占比=5/8=62.5%。
- 风险：剩余 ACTIVE 3 条全部浮亏，说明短阶梯空头在 trend_down 中开仓质量不错，但重复信号和未结案回撤仍需用去重与保护规则控制。

## 2. 三轮基础数据

| 轮次 | 时间 CST | Regime | Symbols | Universe | Fetch ms | REST errors | 输出信号 | LONG | SHORT | Runtime Open | Prompt Open |
|---:|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 06:15:56 | trend_down | 524 | 206 | 48,904 | 0 | 10 | 3 | 7 | 3/10 = 30.0% | 3/10 = 30.0% |
| 2 | 06:21:44 | trend_down | 524 | 208 | 48,208 | 0 | 8 | 2 | 6 | 2/8 = 25.0% | 2/8 = 25.0% |
| 3 | 06:27:32 | trend_down | 524 | 209 | 47,875 | 0 | 9 | 3 | 6 | 3/9 = 33.3% | 3/9 = 33.3% |

数据质量优于 2026-07-31：无 REST partial coverage，universe 稳定在 206-209。

## 3. 形态路由开仓率

### 3.1 输出信号池口径

| Setup | 输出信号 | Runtime Open | 输出占比 | 结论 |
|---|---:|---:|---:|---|
| alt_ladder_breakdown_short | 9 | 8 | 33.3% | trend_down 主开仓来源，质量较好但 open-rate 偏高 |
| funding_reversal | 9 | 0 | 33.3% | 继续观察池，不污染主开仓率 |
| panic_reversal_long | 8 | 0 | 29.6% | trend_down 中保守正确 |
| pre_distribution_watch | 1 | 0 | 3.7% | watch-only 正常 |

三轮没有 `mms_trend_ride_long`、`whale_flow_reversal`、`trend_breakout_long` 进入输出信号池；当前行情结构明显偏空，路由集中度高。

### 3.2 Raw setup funnel 口径

Raw rows 共 624 条，其中 `module_no_match` 388 条；非 module raw rows 236 条，runtime open rows 8 条，raw open-rate=3.4%。

| Setup | Raw rows | Runtime Open | Raw 开仓率 | Watch | Rejected | No tier |
|---|---:|---:|---:|---:|---:|---:|
| trend_breakout_long | 95 | 0 | 0.0% | 0 | 0 | 95 |
| funding_reversal | 79 | 0 | 0.0% | 5 | 4 | 70 |
| mms_trend_ride_long | 19 | 0 | 0.0% | 0 | 0 | 19 |
| alt_ladder_breakdown_short | 17 | 8 | 47.1% | 1 | 0 | 8 |
| panic_reversal_long | 11 | 0 | 0.0% | 4 | 4 | 3 |
| range_expansion_event | 6 | 0 | 0.0% | 0 | 0 | 6 |
| alt_ladder_momentum_long | 5 | 0 | 0.0% | 0 | 0 | 5 |

## 4. 盈亏、止盈、止损跟踪

### 4.1 DB open-review 明细

| Symbol | Round | Dir | Setup | Tier | Status | PnL% | MFE% | MAE% | 说明 |
|---|---:|---|---|---|---|---:|---:|---:|---|
| ONUSDT | 1 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | WIN_TP0 | +0.490 | +0.958 | -0.212 | TP0 |
| BANKUSDT | 1 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | STOP | +0.081 | +1.117 | -0.506 | protected stop |
| GRVTUSDT | 1 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | STOP | +1.609 | +2.871 | 0.000 | protected stop |
| GRVTUSDT | 2 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | STOP | +1.303 | +2.578 | 0.000 | protected stop，重复论题 |
| MUSDT | 2 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | ACTIVE | -0.275 | +0.667 | -0.623 | 未结案浮亏 |
| GRVTUSDT | 3 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | STOP | +1.382 | +2.240 | 0.000 | protected stop，重复论题 |
| PRLUSDT | 3 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | ACTIVE | -1.446 | +0.126 | -1.607 | 未结案浮亏，需反抽风控 |
| ICNTUSDT | 3 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | ACTIVE | -0.678 | +0.190 | -1.112 | 未结案浮亏 |

### 4.2 Tracker 额外样本

outcome tracker 还记录了两条未写最终 DB tier 的样本：

| Symbol | Round | Dir | Setup | DB Tier | Status | PnL% | MFE% | MAE% | 说明 |
|---|---:|---|---|---|---|---:|---:|---:|---|
| TLMUSDT | 2 | LONG | alt_ladder_momentum_long | no tier | STOP | +0.519 | +1.332 | 0.000 | 不能计入真实开仓胜率 |
| TAKEUSDT | 3 | LONG | range_expansion_event | no tier | WIN_TP1 | +2.444 | +2.674 | -0.119 | 不能计入真实开仓胜率，但说明 range 扩展有机会 |

这延续了 2026-07-31 发现的 P0 口径问题：tracker 注册条件比 DB 最终 open-review 口径更宽。

## 5. Outcome 汇总

### 5.1 Tracker 总口径

| Status | Count | Profit | Loss | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|
| WIN_TP1 | 1 | 1 | 0 | +2.444 | +2.674 | -0.119 |
| WIN_TP0 | 1 | 1 | 0 | +0.490 | +0.958 | -0.212 |
| STOP | 5 | 5 | 0 | +0.978 | +2.028 | -0.101 |
| ACTIVE | 3 | 0 | 3 | -0.800 | +0.327 | -1.114 |

Tracker 总平均：Avg PnL=+0.543%，Avg MFE=+1.475%，Avg MAE=-0.418%。

### 5.2 严格 DB open-review 口径

| Setup | Count | TP0 | Protected Stop | Loss Stop | Active Loss | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| alt_ladder_breakdown_short | 8 | 1 | 4 | 0 | 3 | +0.308 | +1.343 | -0.508 |

闭环口径：

- 已结案胜率：1/5=20.0%（只算 TP0/TP1）。
- 已结案正收益率：5/5=100.0%（TP0 1 条 + protected stop 4 条）。
- 已结案 loss stop：0/5=0.0%。
- 全 open-review 正向/保护占比：5/8=62.5%。
- unresolved ACTIVE：3/8=37.5%，且全部浮亏。

## 6. 策略问题审校

### P0：开仓率略高，且集中在单一路由

prompt-final open-rate=29.6%，超过目标上沿 25%。原因不是多形态放宽，而是 trend_down 下 `alt_ladder_breakdown_short` 集中放行。

可接受部分：

- 已结案没有 loss stop；
- protected stop 平均 PnL 明显为正；
- `funding_reversal`、`panic_reversal_long` 没有被放宽。

风险部分：

- 8 条真实 open-review 中 3 条仍 ACTIVE 浮亏；
- GRVTUSDT 连续 3 轮重复入池，放大统计和风险暴露；
- PRLUSDT/ICNTUSDT MFE 很低但 MAE 已明显扩大，说明 late short/反抽风控还不够。

### P0：outcome 注册口径必须修正

本轮真实 DB open-review 为 8 条，但 tracker 跟踪 10 条。TLMUSDT/TAKEUSDT 未写最终 tier 却进入 outcome，这会污染胜率。

立即修复：

1. `persistValidationSignals` 注册 outcome 时只使用 DB 最终 `execution_tier in ('EXECUTABLE','REVIEWABLE')`。
2. 若需要审计 WATCH/no-tier 的 missed opportunity，单独写入 `watch_audit`，不进入真实胜率。
3. outcome summary 增加 `tracked_source` 字段，分开输出 `runtime_open_review` 与 `watch_audit`。

### P0：重复信号去重必须落地

GRVTUSDT 在三轮中连续出现并均结案为 protected stop。交易角度这是同一空头论题，不应按 3 次独立胜率计算。

立即修复：

1. 同 symbol/setup/direction 30 分钟内只保留最新一条 active outcome。
2. 新信号若与旧信号方向相同且 entry zone 重叠，更新旧 outcome 的 latest signal metadata，而不是新增记录。
3. 报告展示 `unique_trade_thesis_count` 与 `raw_signal_count` 两个口径。

### P1：alt_ladder_breakdown_short 可保留，但要加反抽失败二次门

当前 guard 收紧有效，但 ACTIVE 浮亏样本暴露出“确认后反抽”的问题：

- PRLUSDT：PnL -1.446%，MFE +0.126%，MAE -1.607%。
- ICNTUSDT：PnL -0.678%，MFE +0.190%，MAE -1.112%。

可实施升级：

1. REVIEWABLE 允许，但 prompt open 前必须补 `1m/5m no reclaim trigger`。
2. 若入池后 10 分钟 MFE < 0.25% 且 MAE < -0.8%，自动标记 `alt_ladder_short_reclaim_risk`，下一轮降为 WATCH。
3. 对同一标的连续出现，只有价格继续低于上一轮 trigger 且 taker_buy_15m <=0.46，才允许保持 open-review。

### P1：range_expansion_event 需要回收为“机会审计”而非主胜率

TAKEUSDT `range_expansion_event` 未写 DB tier，却在 tracker 中 WIN_TP1 +2.444%。这说明 range 扩展模块有潜在机会，但当前持久化/注册口径未闭环。

建议：

- 不直接放宽主开仓；
- 先把 no-tier range WIN_TP1 归入 missed opportunity；
- 下一轮验证 `range_expansion_event` 是否具备 `retest_confirmed + no_new_high_after_rejection + flow aligned` 后再进入 REVIEWABLE。

### P2：funding_reversal 继续保持观察池

三轮输出 9 条、raw 79 条，open=0，第 3 轮还触发 funding dry breaker cooldown。结论不变：funding crowding 不是开仓理由。

## 7. 可实施优化清单

### 立即实施

1. 修正 outcome 注册分母：只统计 DB 最终 EXECUTABLE/REVIEWABLE；WATCH/no-tier 进入 audit bucket。
2. 增加 30 分钟同 symbol/setup/direction 去重，避免 GRVTUSDT 这类重复论题放大胜率。
3. 给 `alt_ladder_breakdown_short` 增加反抽失败后续门：
   - `no_reclaim_trigger_1m_5m`
   - `mfe_10m_min_0_25`
   - `mae_10m_not_below_minus_0_8`
4. 报告增加 `unique_trade_thesis_count`、`duplicate_context_count`、`watch_audit_count`。

### 下一轮验证

1. 用相同参数跑 3-6 轮，post-track 30 分钟。
2. 验证严格 DB open-review 口径下：
   - prompt-final open-rate 回落到 18%-25%；
   - loss stop <=25%；
   - unique thesis 正收益率 >=60%。
3. 单独观察 `range_expansion_event` missed opportunity，不进入主开仓率。

## 8. 高优目标状态

- 数据链路：达标，三轮 REST errors=0。
- 开仓率：偏高，29.6%，原因是 trend_down 下短阶梯集中放行。
- 盈利质量：短窗口达标，严格 DB open-review 已结案正收益率 100%，loss stop 0。
- 最大问题：胜率统计仍受重复论题和 no-tier tracker 污染；先修口径，再继续判断是否需要调策略阈值。
