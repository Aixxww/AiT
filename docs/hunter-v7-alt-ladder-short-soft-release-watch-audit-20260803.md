# Hunter v7 最新币安合约信号复盘与优化方案

- 生成时间：2026-08-03 07:07:42 CST
- 数据源：Binance USDS-M Futures public REST，1m K 线 + 当前 ticker，未执行任何交易动作。
- 复盘信号：数据库 `hunter_v7_signal_records` 自 2026-08-02 22:55:00 UTC 起的执行层信号，以及核心形态 WATCH/BLOCKED 候选。
- 去重口径：同一 30 分钟内相同 `symbol + direction + setup_type` 只保留最高执行层记录，重复轮次只计入 duplicate_rows。
- 当前币安服务器时间：2026-08-03 07:07:25 CST。

## 结论

1. 执行层样本 0 个，15M/30M/1H 方向正收益率分别为 0.0%、0.0%、0.0%；当前正收益率 0.0%，平均当前 PnL +0.00%。
2. 按真实交易路径看，执行层 TP0 先于 SL 的有效保护/止盈率为 0.0%（0/0），其中 TP1/TP2 完整止盈 0 个，TP0 后保护 0 个，先打损 0 个。
3. 全形态观察样本 115 个，TP0 先于止损触发率 14.8%，先止损率 22.6%；说明筛选层仍能发现波动机会，但主开仓层必须继续过滤追高、反抽失败和先扫损后修复。
4. 当前价格一致性不能等同于实盘胜率：本次有 2 个样本先触发 SL 后才转为当前顺向，提示词应要求“等待回踩确认/二次确认”而不是扩大市价追入。

## 按形态统计

| 形态 | 样本 | EXEC/REVIEW/WATCH+ | 15M正向 | 30M正向 | 1H正向 | 当前正向 | TP0先于SL | SL先触发 | 当前均值 | MFE均值 | MAE均值 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| trend_breakout_long | 47 | 0/0/47 | 42.6% | 42.6% | 42.6% | 40.4% | 4.3% | 51.1% | -0.02% | +0.18% | -0.20% |
| funding_reversal | 43 | 0/0/43 | 48.8% | 48.8% | 48.8% | 58.1% | 30.2% | 0.0% | +0.03% | +0.20% | -0.16% |
| alt_ladder_breakdown_short | 15 | 0/0/15 | 26.7% | 26.7% | 26.7% | 33.3% | 6.7% | 6.7% | -0.17% | +0.41% | -0.55% |
| range_expansion_event | 5 | 0/0/5 | 60.0% | 60.0% | 60.0% | 80.0% | 20.0% | 0.0% | +0.22% | +0.70% | -0.39% |
| alt_ladder_momentum_long | 4 | 0/0/4 | 75.0% | 75.0% | 75.0% | 75.0% | 0.0% | 25.0% | -1.03% | +0.50% | -2.00% |
| whale_flow_reversal | 1 | 0/0/1 | 0.0% | 0.0% | 0.0% | 0.0% | 0.0% | 0.0% | -0.10% | +0.01% | -0.23% |

## 执行层统计

| 分组 | 样本 | EXEC/REVIEW/WATCH+ | 15M正向 | 30M正向 | 1H正向 | 当前正向 | TP0先于SL | SL先触发 | 当前均值 | MFE均值 | MAE均值 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| EXECUTABLE+REVIEWABLE | 0 | 0/0/0 | 0.0% | 0.0% | 0.0% | 0.0% | 0.0% | 0.0% | +0.00% | +0.00% | +0.00% |

## 代表性标的

### 有效止盈样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| BEATUSDT | LONG | range_expansion_event | BLOCKED | -0.09% | -0.09% | -0.09% | +0.09% | +1.46% / -0.18% | WIN_TP0_ACTIVE | 2026-08-03 07:02:00 |
| OPGUSDT | SHORT | funding_reversal | BLOCKED | +1.21% | +1.21% | +1.21% | +1.21% | +1.43% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |
| AKEUSDT | LONG | trend_breakout_long | BLOCKED | +0.25% | +0.25% | +0.25% | +0.39% | +1.29% / -0.02% | WIN_TP0_ACTIVE | 2026-08-03 07:06:00 |
| GWEIUSDT | LONG | trend_breakout_long | BLOCKED | +0.18% | +0.18% | +0.18% | +0.18% | +0.55% / -0.55% | WIN_TP0_ACTIVE | 2026-08-03 07:06:00 |
| LUMIAUSDT | SHORT | funding_reversal | REJECTED | +0.29% | +0.29% | +0.29% | +0.36% | +0.47% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:02:00 |
| TSTUSDT | SHORT | funding_reversal | BLOCKED | +0.18% | +0.18% | +0.18% | +0.18% | +0.45% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |
| HEIUSDT | SHORT | funding_reversal | BLOCKED | +0.15% | +0.15% | +0.15% | +0.15% | +0.38% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |
| PENGUUSDT | SHORT | funding_reversal | BLOCKED | +0.32% | +0.32% | +0.32% | +0.29% | +0.32% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |
| BANKUSDT | SHORT | alt_ladder_breakdown_short | WATCH | -0.20% | -0.20% | -0.20% | -0.13% | +0.31% / -0.65% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |
| SUIUSDT | SHORT | funding_reversal | BLOCKED | +0.29% | +0.29% | +0.29% | +0.28% | +0.29% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |

### 逆向/亏损样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| FHEUSDT | LONG | alt_ladder_momentum_long | BLOCKED | -4.42% | -4.42% | -4.42% | -4.97% | +0.00% / -5.17% | STOP_FIRST | 2026-08-03 07:04:00 |
| AIOUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -2.17% | -2.17% | -2.17% | -1.95% | +0.20% / -2.62% | STOP_FIRST | 2026-08-03 07:04:00 |
| SYNUSDT | LONG | funding_reversal | BLOCKED | -1.44% | -1.44% | -1.44% | -1.22% | +0.00% / -1.66% | ACTIVE_LOSS | - |
| 1000RATSUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -0.94% | -0.94% | -0.94% | -0.85% | +1.08% / -1.24% | ACTIVE_LOSS | - |
| SIRENUSDT | LONG | trend_breakout_long | BLOCKED | -0.59% | -0.59% | -0.59% | -0.63% | +0.07% / -0.63% | STOP_FIRST | 2026-08-03 07:01:00 |
| SLXUSDT | LONG | funding_reversal | BLOCKED | -0.63% | -0.63% | -0.63% | -0.61% | +0.00% / -0.68% | ACTIVE_LOSS | - |
| MUBARAKUSDT | SHORT | funding_reversal | BLOCKED | -0.56% | -0.56% | -0.56% | -0.56% | +0.08% / -1.12% | ACTIVE_LOSS | - |
| BEATUSDT | LONG | trend_breakout_long | BLOCKED | -0.64% | -0.64% | -0.64% | -0.52% | +0.00% / -0.70% | ACTIVE_LOSS | - |
| PIEVERSEUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -0.60% | -0.60% | -0.60% | -0.49% | +0.42% / -0.60% | ACTIVE_LOSS | - |
| COLLECTUSDT | LONG | trend_breakout_long | BLOCKED | -0.47% | -0.47% | -0.47% | -0.47% | +0.00% / -0.52% | STOP_FIRST | 2026-08-03 07:01:00 |

### 先止损或同 K 歧义样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| COLLECTUSDT | LONG | trend_breakout_long | BLOCKED | -0.47% | -0.47% | -0.47% | -0.47% | +0.00% / -0.52% | STOP_FIRST | 2026-08-03 07:01:00 |
| XVSUSDT | LONG | trend_breakout_long | BLOCKED | +0.04% | +0.04% | +0.04% | +0.04% | +0.22% / +0.00% | STOP_FIRST | 2026-08-03 07:01:00 |
| FETUSDT | LONG | trend_breakout_long | BLOCKED | +0.00% | +0.00% | +0.00% | +0.00% | +0.07% / -0.21% | STOP_FIRST | 2026-08-03 07:01:00 |
| ATOMUSDT | LONG | trend_breakout_long | BLOCKED | -0.08% | -0.08% | -0.08% | +0.00% | +0.16% / -0.08% | STOP_FIRST | 2026-08-03 07:01:00 |
| TUSDT | LONG | trend_breakout_long | BLOCKED | -0.28% | -0.28% | -0.28% | -0.22% | +0.09% / -0.43% | STOP_FIRST | 2026-08-03 07:01:00 |
| MIRAUSDT | LONG | trend_breakout_long | BLOCKED | -0.02% | -0.02% | -0.02% | -0.02% | +0.00% / -0.34% | STOP_FIRST | 2026-08-03 07:01:00 |
| AIOUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -2.17% | -2.17% | -2.17% | -1.95% | +0.20% / -2.62% | STOP_FIRST | 2026-08-03 07:04:00 |
| SONICUSDT | LONG | trend_breakout_long | BLOCKED | -0.11% | -0.11% | -0.11% | -0.11% | +0.11% / -0.11% | STOP_FIRST | 2026-08-03 07:01:00 |
| XNYUSDT | LONG | trend_breakout_long | BLOCKED | -0.36% | -0.36% | -0.36% | -0.36% | +0.00% / -0.38% | STOP_FIRST | 2026-08-03 07:01:00 |
| STBLUSDT | LONG | trend_breakout_long | BLOCKED | -0.12% | -0.12% | -0.12% | -0.12% | +0.08% / -0.20% | STOP_FIRST | 2026-08-03 07:01:00 |
| ARBUSDT | LONG | trend_breakout_long | BLOCKED | -0.16% | -0.16% | -0.16% | -0.16% | +0.05% / -0.17% | STOP_FIRST | 2026-08-03 07:01:00 |
| XPINUSDT | LONG | trend_breakout_long | BLOCKED | +0.13% | +0.13% | +0.13% | +0.13% | +0.13% / -0.13% | STOP_FIRST | 2026-08-03 07:01:00 |

### 先打损后行情修复样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| XPINUSDT | LONG | trend_breakout_long | BLOCKED | +0.13% | +0.13% | +0.13% | +0.13% | +0.13% / -0.13% | STOP_FIRST | 2026-08-03 07:01:00 |
| XVSUSDT | LONG | trend_breakout_long | BLOCKED | +0.04% | +0.04% | +0.04% | +0.04% | +0.22% / +0.00% | STOP_FIRST | 2026-08-03 07:01:00 |

### WATCH/BLOCKED 错失机会复核池

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| OPGUSDT | SHORT | funding_reversal | BLOCKED | +1.21% | +1.21% | +1.21% | +1.21% | +1.43% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |
| SQDUSDT | LONG | trend_breakout_long | BLOCKED | +0.86% | +0.86% | +0.86% | +0.86% | +1.29% / +0.00% | ACTIVE_PROFIT | - |
| PORTALUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +0.85% | +0.85% | +0.85% | +0.85% | +1.23% / +0.00% | ACTIVE_PROFIT | - |
| GRVTUSDT | LONG | range_expansion_event | BLOCKED | +0.60% | +0.60% | +0.60% | +0.67% | +0.90% / -0.04% | ACTIVE_PROFIT | - |
| AKEUSDT | LONG | trend_breakout_long | BLOCKED | +0.25% | +0.25% | +0.25% | +0.39% | +1.29% / -0.02% | WIN_TP0_ACTIVE | 2026-08-03 07:06:00 |
| LUMIAUSDT | SHORT | funding_reversal | REJECTED | +0.29% | +0.29% | +0.29% | +0.36% | +0.47% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:02:00 |
| PENGUUSDT | SHORT | funding_reversal | BLOCKED | +0.32% | +0.32% | +0.32% | +0.29% | +0.32% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |
| SUIUSDT | SHORT | funding_reversal | BLOCKED | +0.29% | +0.29% | +0.29% | +0.28% | +0.29% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |
| LTCUSDT | SHORT | funding_reversal | BLOCKED | +0.22% | +0.22% | +0.22% | +0.22% | +0.25% / -0.02% | WIN_TP0_ACTIVE | 2026-08-03 07:06:00 |
| LDOUSDT | SHORT | funding_reversal | WATCH | +0.22% | +0.22% | +0.22% | +0.22% | +0.22% / -0.09% | WIN_TP0_ACTIVE | 2026-08-03 07:02:00 |
| GWEIUSDT | LONG | trend_breakout_long | BLOCKED | +0.18% | +0.18% | +0.18% | +0.18% | +0.55% / -0.55% | WIN_TP0_ACTIVE | 2026-08-03 07:06:00 |
| TSTUSDT | SHORT | funding_reversal | BLOCKED | +0.18% | +0.18% | +0.18% | +0.18% | +0.45% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 07:01:00 |

## 可实施优化方案

1. `alt_ladder_momentum_long`：保持“中段 + OI 正增 + taker > 0.52”作为主通道，但把 `alt_ladder_stage_late + high_volatility/funding_elevated` 组合降为 REVIEW/WATCH，并要求 TP0 触发后立即保本；本次样本里该形态早段有止盈能力，但追高样本 MFE 与 MAE 摆动过大。
2. `range_expansion_event`：LONG 必须增加“反抽失败未出现”的二次确认：高位 entry_zone_position、1h OI 转负或 reason 含 `short_covering_not_new_long_build` 时，不进入主开仓率；只有 5m 回踩 VWAP/EMA20 后重新放量站回，才升到 REVIEWABLE。
3. `whale_flow_reversal`：当前逻辑对 taker 不足的拦截总体合理，但需要把 `OI 快速增加 + 价格未充分标记 + funding_not_crowded` 作为“错失机会复核池”；若 15m/30m 后价格仍在入场带上方且 taker 回到 0.54-0.56，再允许二次确认升档。
4. `alt_ladder_breakdown_short`：跨轮二次确认应以“下一轮低点下移/5m EMA20 下压/买盘 taker 未反包”为核心；若首轮只是挤压后的短线回撤，不应直接开空。当前复盘应把 BLOCKED/WATCH 的顺向下跌标的纳入 missed-opportunity 队列，而不是放宽主开仓门槛。
5. `funding_reversal`：继续从主开仓率拆桶，单独做事件型观察。只有 funding 极值、OI 与价格反向、taker 已转向三项同时满足时才进入 REVIEW；否则仅参与风控解释，不参与执行候选。
6. 统计口径：`PROTECTED_AFTER_TP0` 计为风控有效，不计入亏损止损；但若同形态连续出现 TP0 后快速回撤，策略提示词应从“可追随”改成“只做 TP0 快进快出”。
7. 数据质量：上一轮 validator 有 universe coverage 低的问题，实盘开仓率评估必须保留 valid_rounds 门槛；低覆盖轮只能用于形态复盘，不能用于放宽执行层阈值。

## 落地优先级

1. P0：在 `provider/local/hunter_v7_mod_range_expansion_event.go` 把 LONG 的高位 + OI 转负/非新多建仓改为硬降档；提示词同步要求“反抽失败未出现 + 回踩后重新放量站回”。
2. P0：在 `kernel/hunter_v7_prompt_doctrine.go` 或提示词 payload 中，把开仓许可改成双阈值：`entry_zone_position <= 72` 且 `TP0_distance >= 0.45R`；不满足时只能 REVIEW/WATCH。
3. P1：在 `provider/local/hunter_v7_mod_alt_ladder.go` 和策略提示词中，对 `alt_ladder_stage_late + high_volatility/funding_elevated` 降档，并把 TP0 后保本写成强制动作。
4. P1：在 `provider/local/hunter_v7_mod_whale_flow.go` 增加二次确认池：首轮 taker 不足但 OI 明显累积的样本，下一轮 taker 回升且价格未跌破入场带再升档。
5. P2：在 `cmd/hunter_v7_validate` 和长期 outcome 表中增加 `direction_accuracy_15m/30m/1h/current`、`tp0_before_sl_rate`、`protected_after_tp0_rate`，让开仓率优化不和盈利质量混在一个指标里。

## 附件

- 明细 JSON：reports/hunter-v7-alt-ladder-short-soft-release-watch-audit-20260803/hunter-v7-latest-binance-replay-20260802T230742.json
- 明细 CSV：reports/hunter-v7-alt-ladder-short-soft-release-watch-audit-20260803/hunter-v7-latest-binance-replay-20260802T230742.csv

