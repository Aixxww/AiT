# Hunter v7 Binance 实时三轮全链路跟踪报告

生成时间：2026-07-31 05:06 CST
测试窗口：2026-07-31 04:27:47 / 04:33:38 / 04:39:30 CST
数据目录：`reports/hunter-v7-binance-live-3round-20260731/`
后跟踪窗口：20 分钟，每 30 秒刷新 1m candle
数据库窗口：`hunter_v7_signal_records.timestamp >= 2026-07-30 20:20:00 UTC`

## 1. 执行结论

- 三轮 Binance 实时拉数均完成，JSON/prompt 校验通过，AIT prompt 均包含 Hunter v7 结构化信号。
- 三轮输出信号 41 条：LONG=27，SHORT=14；三轮市场 regime 均为 `rotation`。
- prompt-final 可开仓口径：EXECUTABLE=1，REVIEWABLE=8，open-rate=9/41=22.0%。
- runtime 后端初筛口径：EXECUTABLE=1，REVIEWABLE=10，open-rate=11/41=26.8%。
- outcome 跟踪 13 条：WIN_TP0=3，STOP=5，ACTIVE=5；其中 STOP 里 3 条为正 PnL protected stop，2 条为 loss stop。
- 短窗口质量：TP0/protected stop/active profit=9/13=69.2%，loss stop=2/13=15.4%，平均 PnL=+0.066%，Avg MFE=+0.750%，Avg MAE=-0.560%。
- 结论：开仓率仍处于目标区间，短阶梯空头收紧后质量明显改善；当前主要风险从“开仓过多”转为“部分多头形态未能快速 TP0，仍需更强保护/分层”。

## 2. 三轮基础数据

| 轮次 | 时间 CST | Regime | Symbols | Universe | Fetch ms | REST errors | 输出信号 | LONG | SHORT | Runtime Open | Prompt Open |
|---:|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 04:27:47 | rotation | 523 | 212 | 50,860 | 2 | 13 | 8 | 5 | 4/13 = 30.8% | 3/13 = 23.1% |
| 2 | 04:33:38 | rotation | 523 | 210 | 51,101 | 2 | 16 | 12 | 4 | 4/16 = 25.0% | 3/16 = 18.8% |
| 3 | 04:39:30 | rotation | 523 | 208 | 50,699 | 2 | 12 | 7 | 5 | 3/12 = 25.0% | 3/12 = 25.0% |

数据源质量稳定：每轮 REST errors=2，错误率约 0.4%，与 2026-07-30 测试一致。该缺口不足以破坏主流程，但仍应继续记录具体 endpoint/symbol。

## 3. 形态路由开仓率

### 3.1 输出信号池口径

| Setup | 输出信号 | Runtime Open | 输出占比 | 结论 |
|---|---:|---:|---:|---|
| funding_reversal | 9 | 0 | 22.0% | 观察池定位保持正确，不污染主开仓率 |
| trend_breakout_long | 8 | 0 | 19.5% | 高召回但仍缺 close-through，保守合理 |
| alt_ladder_breakdown_short | 4 | 4 | 9.8% | P0-B 收紧后样本少但质量转正 |
| whale_flow_reversal | 4 | 2 | 9.8% | MFE 高，protected stop 有效，但仍需避免 WATCH 被 outcome 误跟踪 |
| mms_trend_ride_long | 3 | 3 | 7.3% | 仍是 REVIEWABLE 级别，短窗口未结案且当前偏弱 |
| intraday_scalp_long | 3 | 0 | 7.3% | 未进入 open-review |
| pre_breakout_watch | 3 | 0 | 7.3% | watch-only 正常 |
| alt_ladder_momentum_long | 2 | 2 | 4.9% | 本轮表现弱于昨日，需避免 rotation 下过度追高 |
| panic_reversal_long | 2 | 0 | 4.9% | 保守正确 |
| pullback_reversal_long | 2 | 0 | 4.9% | 保守正确 |
| pre_distribution_watch | 1 | 0 | 2.4% | watch-only 正常 |

### 3.2 Raw setup funnel 口径

Raw rows 共 632 条，其中 `module_no_match` 368 条；非 module raw rows 264 条，runtime open rows 11 条，raw open-rate=4.2%。

| Setup | Raw rows | Runtime Open | Raw 开仓率 | Watch | Rejected | No tier |
|---|---:|---:|---:|---:|---:|---:|
| trend_breakout_long | 153 | 0 | 0.0% | 5 | 3 | 145 |
| funding_reversal | 48 | 0 | 0.0% | 8 | 1 | 39 |
| mms_trend_ride_long | 21 | 3 | 14.3% | 0 | 0 | 18 |
| panic_reversal_long | 15 | 0 | 0.0% | 2 | 0 | 13 |
| alt_ladder_momentum_long | 6 | 2 | 33.3% | 0 | 0 | 4 |
| alt_ladder_breakdown_short | 4 | 4 | 100.0% | 0 | 0 | 0 |
| whale_flow_reversal | 4 | 2 | 50.0% | 2 | 0 | 0 |

注意：raw open-rate 只用于筛选强度审计，不应与 prompt-final open-rate 混算。

## 4. 盈亏、止盈、止损跟踪

| Symbol | Round | Dir | Setup | Tier | Status | PnL% | MFE% | MAE% | 说明 |
|---|---:|---|---|---|---|---:|---:|---:|---|
| VIRTUALUSDT | 1 | LONG | mms_trend_ride_long | REVIEWABLE | ACTIVE | -0.248 | +0.101 | -0.248 | 低 MFE，继续观察 |
| UAIUSDT | 1 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | STOP | +0.987 | +1.999 | 0.000 | protected stop，优质 |
| AKEUSDT | 1 | LONG | alt_ladder_momentum_long | EXECUTABLE | STOP | -0.163 | +1.334 | -0.324 | 有 MFE 后回吐，保护可更早 |
| ESPORTSUSDT | 1 | LONG | whale_flow_reversal | REVIEWABLE | STOP | +0.429 | +1.687 | 0.000 | protected stop，优质 |
| CAPUSDT | 1 | LONG | whale_flow_reversal | WATCH | ACTIVE | +0.045 | +0.949 | -0.640 | WATCH 被跟踪，口径需修正 |
| APTUSDT | 2 | LONG | mms_trend_ride_long | REVIEWABLE | ACTIVE | -0.399 | 0.000 | -0.399 | 未形成 MFE |
| CAPUSDT | 2 | LONG | whale_flow_reversal | REVIEWABLE | STOP | +0.375 | +1.325 | 0.000 | protected stop，优质 |
| 1000BONKUSDT | 2 | LONG | mms_trend_ride_long | REVIEWABLE | ACTIVE | +0.098 | +0.269 | -0.005 | 轻微浮盈 |
| SYNUSDT | 2 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | WIN_TP0 | +0.246 | +0.396 | 0.000 | TP0 |
| MMTUSDT | 3 | LONG | alt_ladder_momentum_long | REVIEWABLE | STOP | -1.729 | 0.000 | -4.545 | 本轮最大拖累 |
| UAIUSDT | 3 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | ACTIVE | +0.440 | +0.828 | -0.698 | 浮盈，仍未 TP0 |
| PENGUUSDT | 3 | LONG | alt_ladder_momentum_long | no tier | WIN_TP0 | +0.594 | +0.647 | -0.357 | 被 outcome 跟踪但 DB tier 未写入 |
| SYNUSDT | 3 | SHORT | alt_ladder_breakdown_short | REVIEWABLE | WIN_TP0 | +0.176 | +0.212 | -0.064 | TP0 |

### 4.1 Outcome 汇总

| Status | Count | Profit | Loss | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|
| WIN_TP0 | 3 | 3 | 0 | +0.339 | +0.418 | -0.141 |
| STOP | 5 | 3 | 2 | -0.020 | +1.269 | -0.974 |
| ACTIVE | 5 | 3 | 2 | -0.013 | +0.429 | -0.398 |

闭环口径：

- 已结案 TP 胜率：3/8=37.5%。
- 已结案正收益率：6/8=75.0%（TP0 3 条 + protected stop 3 条）。
- 已结案 loss stop：2/8=25.0%。
- 全 tracked 正向/保护占比：9/13=69.2%。

### 4.2 Setup 汇总

| Setup | Count | TP0 | Protected Stop | Loss Stop | Active | Avg PnL% | Avg MFE% | Avg MAE% | 结论 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| alt_ladder_breakdown_short | 4 | 2 | 1 | 0 | 1 | +0.462 | +0.859 | -0.191 | 本轮最佳，P0-B 收紧有效 |
| whale_flow_reversal | 3 | 0 | 2 | 0 | 1 | +0.283 | +1.320 | -0.213 | MFE 强，保护有效 |
| mms_trend_ride_long | 3 | 0 | 0 | 0 | 3 | -0.183 | +0.123 | -0.217 | 未结案且 MFE 弱，暂不放宽 |
| alt_ladder_momentum_long | 3 | 1 | 0 | 2 | 0 | -0.433 | +0.660 | -1.742 | 本轮主拖累，需加追高/回撤保护 |

## 5. 策略问题审校

### P0：统计/注册口径仍需修正

本轮 outcome tracker 实际跟踪 13 条，但 DB `execution_tier in (EXECUTABLE, REVIEWABLE)` 只有 11 条。异常样本：

- CAPUSDT round1：DB tier=WATCH，但被 outcome tracker 跟踪。
- PENGUUSDT round3：DB tier 未写入，但被 outcome tracker 跟踪并 WIN_TP0。

这说明 P0-A 仍未完成：`signal.ExecutionReadiness`、DB `execution_tier`、prompt-final tier、outcome 注册条件仍存在口径不一致。

可实施修复：

1. `persistValidationSignals` 注册 outcome 时只允许 DB 最终 `execution_tier in (EXECUTABLE, REVIEWABLE)`。
2. 若为了 missed opportunity 审计跟踪 WATCH，必须写入独立 `watch_audit` bucket，不进入真实胜率分母。
3. outcome summary 增加 `tracked_source`：`runtime_open_review`、`prompt_final_open_review`、`watch_audit`。

### P0：alt_ladder_breakdown_short 收紧有效，但需去重

本轮 `alt_ladder_breakdown_short` 4 条 tracked：

- TP0 2 条：SYNUSDT round2/round3。
- protected stop 1 条：UAIUSDT round1。
- ACTIVE profit 1 条：UAIUSDT round3。
- loss stop 0 条。

这与 2026-07-30 的主亏损源表现相反，说明晚期/未击穿短空被压住后，保留下来的 close-through 短空质量较高。

仍需补：

1. 同 symbol/setup/direction 30 分钟内只保留最新 open-review；SYNUSDT 连续两轮 TP0 说明信号有效，但 outcome 统计不应重复扩大胜率。
2. UAIUSDT round1/round3 重复出现，应在报告中聚合为同一交易论题，避免重复计算风险暴露。

### P1：alt_ladder_momentum_long 本轮弱化

本轮 `alt_ladder_momentum_long`：

- AKEUSDT EXECUTABLE：MFE +1.334% 后 STOP -0.163%。
- MMTUSDT REVIEWABLE：STOP -1.729%，MAE -4.545%。
- PENGUUSDT：WIN_TP0 +0.594%，但 DB tier 未写入。

结论：rotation regime 下不能盲目把 `alt_ladder_momentum_long` 作为主升开仓源。需要增加：

1. MFE 达 TP0 60% 或 0.8R 时，至少推进 stop 到 entry 附近。
2. 对 1m/5m 快速回撤或跌破 VWAP 的 alt ladder long，立即从 EXECUTABLE 降为 REVIEWABLE/WAIT。
3. `alt_ladder_stage_mid/late` 若 OI 没有继续净流入，不允许仅凭 taker_buy 保持 EXECUTABLE。

### P1：mms_trend_ride_long 暂不放宽

本轮 3 条 mms 全部 ACTIVE，平均 PnL=-0.183%，Avg MFE 仅 +0.123%。它的 MAE 不大，但推进速度不足。

建议：

- 保持 REVIEWABLE，不升 EXECUTABLE。
- 增加最短验证：5m candle 重新站上 EMA25/VWAP 且 taker_buy_15m >= 0.54 后才允许 prompt open。
- 若 10-15 分钟 MFE < 0.2% 且 price 低于入场中位，输出 `mms_no_followthrough_wait`。

### P1：whale_flow_reversal 保护机制有效

whale 3 条中 2 条 protected stop 正收益，1 条 WATCH active 小浮盈。MFE 平均 +1.320%，MAE 低。

建议：

- whale 仍只允许低位 entry-zone 与 taker>=0.56 的 REVIEWABLE/EXECUTABLE。
- 对 MFE>1% 的 whale，继续保持快速保护止损；该机制已有效避免 ONUSDT 类利润回吐问题。

### P2：funding_reversal 继续观察池

三轮输出 9 条，raw 48 条，open=0。第 3 轮触发 funding dry breaker cooldown，说明拆桶机制继续有效。

不要放宽 funding。只有以下四确认齐全才允许 REVIEWABLE：

- 15m close below VWAP；
- taker_sell strong；
- retest failed；
- no_new_high_after_rejection。

## 6. 可实施优化清单

### 立即实施

1. 修正 outcome 注册口径：真实胜率只跟踪 DB 最终 EXECUTABLE/REVIEWABLE；WATCH 只进 missed opportunity audit。
2. 增加同 symbol/setup/direction 去重：30 分钟内重复 open-review 只保留最新记录，旧记录标记 `duplicate_signal_context_only`。
3. `alt_ladder_momentum_long` 增加 MFE 保护：MFE>=0.8R 或 TP0 距离 60% 时，推动保本/保护止损。

### 下一轮验证

1. 用相同参数跑 6 轮，post-track 30 分钟，验证短阶梯空头是否持续保持 loss stop <=25%。
2. 单独统计 `mms_trend_ride_long` 的 10-15 分钟 MFE 分布，未超过 0.2% 的样本不允许 prompt open。
3. 对 `whale_flow_reversal` 保持现有保护，重点观察 protected stop 是否继续贡献正收益。

## 7. 高优目标状态

- 开仓率：达标，prompt-final 22.0%，处于 18%-25% 目标区间。
- 盈利质量：短窗口达标，TP0/protected/active profit=69.2%，loss stop=15.4%。
- 最大未解问题：统计与 outcome 注册口径仍不一致；未修正前，胜率只能作为验证器内存 tracker 的短窗口质量，不可直接等同生产交易胜率。
