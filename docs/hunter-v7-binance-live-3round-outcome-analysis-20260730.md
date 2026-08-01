# Hunter v7 Binance 实时三轮全链路跟踪报告

生成时间：2026-07-30 06:25 CST
测试窗口：2026-07-30 05:59:36 / 06:06:03 / 06:12:14 CST
数据目录：`reports/hunter-v7-binance-live-3round-20260730/`
数据库窗口：`hunter_v7_signal_records.timestamp >= 2026-07-29 21:57:00 UTC`

## 1. 执行结论

- 三轮 Binance 实时拉数均完成，JSON/prompt 结构校验均通过，`hunter_v7_signal_json` 已进入 AIT prompt。
- 三轮输出信号共 34 条：LONG=19，SHORT=15；市场从前两轮 `trend_down` 切到第三轮 `rotation`。
- prompt-final 可开仓口径：EXECUTABLE=1，REVIEWABLE=5，开仓候选率 17.6%。后端 runtime 口径为 EXECUTABLE=1，REVIEWABLE=6，开仓候选率 20.6%。
- 全 raw setup funnel 口径：非 `module_no_match` raw setup row 共 138 条，open-review row 共 7 条，漏斗开仓率 5.1%。这个口径更适合评估筛选强度。
- 短窗口 outcome：7 条 EXECUTABLE/REVIEWABLE 被跟踪，2 条 STOP，0 条 TP，4 条仍为浮盈 ACTIVE，1 条仍为浮亏 ACTIVE；当前平均 PnL=-0.119%，平均 MFE=+0.523%，平均 MAE=-0.446%。
- 本次样本太短，只能代表 10-15 分钟级别的实时触发质量，不能替代 30m/2h/24h 长期胜率。

## 2. 三轮基础数据

| 轮次 | 时间 CST | Regime | Symbols | Universe | Fetch ms | REST errors | 输出信号 | LONG | SHORT | Runtime Open | Prompt Open |
|---:|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 05:59:36 | trend_down | 523 | 224 | 98,328 | 2 | 13 | 7 | 6 | 3/13 = 23.1% | 2/13 = 15.4% |
| 2 | 06:06:03 | trend_down | 523 | 223 | 86,009 | 2 | 9 | 4 | 5 | 2/9 = 22.2% | 2/9 = 22.2% |
| 3 | 06:12:14 | rotation | 523 | 224 | 70,411 | 2 | 12 | 8 | 4 | 2/12 = 16.7% | 2/12 = 16.7% |

数据质量稳定但有轻微缺口：每轮 REST errors=2，错误率约 0.4%。这会影响少数 OI/LSR/Kline detail，但没有破坏主流程。

## 3. 形态路由开仓率

### 输出信号池口径

输出信号池是已经进入报告/prompt 的 34 条信号，更贴近 AIT 实际看到的候选。

| Setup | 输出信号 | Runtime Open | 开仓候选率 | 主要状态 |
|---|---:|---:|---:|---|
| alt_ladder_breakdown_short | 6 | 0 | 0.0% | 全部 WATCH，fast_confirm 但缺二次确认 |
| alt_ladder_momentum_long | 3 | 2 | 66.7% | 1 EXECUTABLE + 1 REVIEWABLE + 1 WATCH |
| funding_reversal | 9 | 0 | 0.0% | 全部 WATCH，继续保持拆桶后的观察池定位 |
| panic_reversal_long | 7 | 0 | 0.0% | REJECTED/WATCH，主要等待 reclaim 或 RR 修复 |
| displacement_momentum_long | 2 | 0 | 0.0% | WATCH |
| trend_breakout_long | 2 | 0 | 0.0% | WATCH |
| whale_flow_reversal | 2 | 0 | 0.0% | WATCH/REJECTED，DB runtime 有 1 条 REVIEWABLE 被跟踪 |
| intraday_scalp_long | 1 | 0 | 0.0% | WATCH |
| leader_momentum_long | 1 | 0 | 0.0% | REJECTED |
| pre_breakout_watch | 1 | 0 | 0.0% | WATCH |

### Raw setup funnel 口径

Raw setup row 包括未进入最终输出的 setup 命中，适合衡量筛选强度。

| Setup | Raw rows | Open rows | Raw 开仓率 | Watch | Rejected | 未展开 |
|---|---:|---:|---:|---:|---:|---:|
| funding_reversal | 55 | 0 | 0.0% | 5 | 4 | 46 |
| trend_breakout_long | 23 | 0 | 0.0% | 1 | 1 | 21 |
| mms_trend_ride_long | 15 | 0 | 0.0% | 0 | 0 | 15 |
| panic_reversal_long | 11 | 0 | 0.0% | 3 | 4 | 4 |
| alt_ladder_breakdown_short | 10 | 4 | 40.0% | 2 | 0 | 4 |
| intraday_scalp_long | 8 | 0 | 0.0% | 1 | 0 | 7 |
| alt_ladder_momentum_long | 7 | 2 | 28.6% | 1 | 0 | 4 |
| displacement_momentum_long | 2 | 0 | 0.0% | 1 | 1 | 0 |
| range_expansion_event | 2 | 0 | 0.0% | 0 | 0 | 2 |
| whale_flow_reversal | 2 | 1 | 50.0% | 0 | 1 | 0 |

## 4. 盈亏、止盈、止损跟踪

| Symbol | Dir | Setup | Tier | Status | 当前价 | PnL% | MFE% | MAE% | Stop | TP0 | TP1 | TP2 |
|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| GWEIUSDT | SHORT | alt_ladder_breakdown_short | REVIEWABLE | ACTIVE | 0.018360 | +0.441 | +0.821 | 0.000 | 0.018815 | 0.018142 | 0.017437 | 0.016602 |
| ONUSDT | LONG | whale_flow_reversal | REVIEWABLE | STOP | 0.320430 | -0.075 | +1.033 | -0.366 | 0.319298 | 0.326371 | 0.363205 | 0.398139 |
| ZAMAUSDT | SHORT | alt_ladder_breakdown_short | REVIEWABLE | ACTIVE | 0.057840 | +0.025 | +0.578 | -0.148 | 0.059021 | 0.056921 | 0.054207 | 0.051288 |
| UAIUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | ACTIVE | 0.418800 | +0.318 | +0.941 | -0.784 | 0.415942 | 0.427409 | 0.447638 | 0.471772 |
| BANKUSDT | SHORT | alt_ladder_breakdown_short | REVIEWABLE | STOP | 0.171500 | -1.728 | 0.000 | -1.816 | 0.172411 | 0.165828 | 0.141406 | 0.118945 |
| ACHUSDT | LONG | alt_ladder_momentum_long | EXECUTABLE | ACTIVE | 0.004864 | -0.009 | +0.073 | -0.009 | 0.004775 | 0.004948 | 0.005124 | 0.005384 |
| HOLOUSDT | SHORT | alt_ladder_breakdown_short | REVIEWABLE | ACTIVE | 0.077180 | +0.192 | +0.218 | 0.000 | 0.078929 | 0.076232 | 0.074764 | 0.072200 |

按 setup 聚合：

| Setup | 跟踪数 | TP | STOP | ACTIVE 浮盈 | ACTIVE 浮亏 | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| alt_ladder_breakdown_short | 4 | 0 | 1 | 3 | 0 | -0.268 | +0.404 | -0.491 |
| alt_ladder_momentum_long | 2 | 0 | 0 | 1 | 1 | +0.154 | +0.507 | -0.397 |
| whale_flow_reversal | 1 | 0 | 1 | 0 | 0 | -0.075 | +1.033 | -0.366 |

短窗口胜率口径：

- 已结案胜率：0/2 = 0.0%，因为两条结案均为 STOP。
- 含 ACTIVE 浮盈口径：4/7 = 57.1%，但 5 条未结案，可靠性低。
- 平均 PnL 为负，主要由 BANKUSDT REVIEWABLE 短空快速止损拖累；ONUSDT 曾有 +1.033% MFE 后回落触发 STOP，说明 TP0/保本推进或动态止损触发点仍可优化。

## 5. 形态观察

1. `funding_reversal` 拆桶生效。三轮输出 9 条、raw 55 条，但没有进入 open-review，说明它没有继续污染主开仓率。当前定位应保持为 reversal watch pool，除非出现 retest failed + taker sell + no new high 的组合确认。
2. `alt_ladder_breakdown_short` 召回很强，但 prompt 输出池仍多为 WATCH；DB runtime 对 raw rows 给出 4 条 REVIEWABLE，其中 BANKUSDT 已 STOP。跨轮二次确认需要继续压实“重复出现 != 自动升级”，尤其 late downshift 和 taker 未翻空时应保持 WATCH。
3. `alt_ladder_momentum_long` 是本次唯一出现 EXECUTABLE 的形态，rotation regime 下更容易放行。短窗口表现温和正，但 ACHUSDT 刚触发时几乎贴近成本，仍需等待 TP0 命中率。
4. `panic_reversal_long` 召回稳定但没有开仓，主要受 low timing、entry_rr_invalid、wait_reclaim 限制。当前保守是合理的；放宽会增加接刀风险。
5. `whale_flow_reversal` 出现 1 条 REVIEWABLE 并 STOP，虽然 MFE 达 +1.033%，但未锁盈。该类信号需要把 TP0/保本推进前移，避免“先到浮盈后回到止损”的低效率。
6. `range_expansion_event` raw 只出现 2 条且未展开，新增反抽失败确认没有造成过度放行；后续要继续观测它是否过度压低机会覆盖。

## 6. 可实施优化项

### P0：修复 outcome 跟踪口径

- 当前 validate 在每轮末尾 tick 一次 outcome，命令结束后不会持续轮询，导致 ACTIVE 的 TP/SL 后续变化不会自动写回。
- 建议给 `cmd/hunter_v7_validate` 增加 `--post-track-duration`，默认 20-30 分钟；三轮结束后继续每 30-60 秒刷新 1m candle，直到 ACTIVE 结案或超时。
- 报告端增加“已结案胜率”和“含浮盈胜率”两栏，避免短窗口误判策略胜率。

### P0：alt_ladder_breakdown_short 二次确认继续收紧

- BANKUSDT REVIEWABLE 快速 STOP，暴露 late downshift 短空在反抽阶段仍可能被放行。
- 建议：当 reason 含 `alt_ladder_downshift_late` 时，必须同时满足 `5m_or_15m_close_below_trigger`、`taker_buy_15m < 0.46`、最近 1m/5m 不出现 reclaim；否则强制 WATCH。
- 对 `alt_ladder_breakdown_short` 增加 `reviewable_late_retest_pending` blocked gate，便于矩阵单独统计。

### P1：whale_flow_reversal 增加 TP0 保护优先级

- ONUSDT MFE +1.033% 后 STOP，说明该形态在短窗口内有“先给保护利润后回撤”的特征。
- 建议：当 REVIEWABLE 的 MFE 超过 0.8R 或达到 TP0 的 60% 距离时，将跟踪止损推进到 entry 附近；若 taker 从 strong/recovering 转 neutral，优先 close/reduce，而不是等原 stop。

### P1：funding_reversal 保持观察池，不进入主开仓率

- 本次 funding 输出 9 条，raw 55 条，但 open=0，符合拆桶目标。
- 建议继续把 funding 作为 reversal watch pool 单独报告，不纳入主 open-rate 分母；只有 `retest_failed + no_new_high_after_rejection + taker_sell_strong + 15m below VWAP` 同时满足时才进入 REVIEWABLE。

### P2：增加 per-setup 样本最小门槛

- 三轮只有 34 条输出，很多 setup 样本小于 3，不适合直接调阈值。
- 建议矩阵报告增加 `min_sample=20` 标记；小样本只输出观察，不自动调参。

### P2：REST partial coverage 降噪

- 每轮 REST errors 固定为 2，短期不影响流程，但会让局部 OI/LSR 缺失。
- 建议报告记录具体 endpoint/symbol，并对缺数据导致的 WATCH/REJECTED 增加 `data_gap_blocked` 归因，避免把数据源缺口误判为策略保守。

## 7. 下一轮验证建议

建议下一轮用同样参数跑 6 轮，每轮 5 分钟，并在结束后继续 outcome 跟踪 30 分钟：

```bash
go run ./cmd/hunter_v7_validate \
  -rounds=6 \
  -round-interval=5m \
  -top-detail=220 \
  -max-workers=8 \
  -max-output=60 \
  -watch-output=20 \
  -min-priority=40 \
  -aggressive=true \
  -persist-signals=true \
  -track-outcomes=true \
  -out-dir=reports/hunter-v7-binance-live-6round-20260730
```

验收标准：

- prompt-final open-rate 稳定在 15%-25%，但 STOP 率不高于 ACTIVE+TP 的 35%。
- `alt_ladder_breakdown_short` late 分支 STOP 率下降，early/mid 分支保留召回。
- `funding_reversal` 继续不进入主开仓池，除非四项确认完整。
- outcome 报告必须能区分 TP0、TP1、STOP、ACTIVE，不再只看即时浮盈。
