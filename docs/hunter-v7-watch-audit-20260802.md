# Hunter v7 最新币安合约信号复盘与优化方案

- 生成时间：2026-08-03 06:50:08 CST
- 数据源：Binance USDS-M Futures public REST，1m K 线 + 当前 ticker，未执行任何交易动作。
- 复盘信号：数据库 `hunter_v7_signal_records` 自 2026-08-02 22:15:00 UTC 起的执行层信号，以及核心形态 WATCH/BLOCKED 候选。
- 去重口径：同一 30 分钟内相同 `symbol + direction + setup_type` 只保留最高执行层记录，重复轮次只计入 duplicate_rows。
- 当前币安服务器时间：2026-08-03 06:50:02 CST。

## 结论

1. 执行层样本 3 个，15M/30M/1H 方向正收益率分别为 33.3%、0.0%、0.0%；当前正收益率 33.3%，平均当前 PnL -0.56%。
2. 按真实交易路径看，执行层 TP0 先于 SL 的有效保护/止盈率为 0.0%（0/3），其中 TP1/TP2 完整止盈 0 个，TP0 后保护 0 个，先打损 1 个。
3. 全形态观察样本 198 个，TP0 先于止损触发率 22.7%，先止损率 44.4%；说明筛选层仍能发现波动机会，但主开仓层必须继续过滤追高、反抽失败和先扫损后修复。
4. 当前价格一致性不能等同于实盘胜率：本次有 8 个样本先触发 SL 后才转为当前顺向，提示词应要求“等待回踩确认/二次确认”而不是扩大市价追入。

## 按形态统计

| 形态 | 样本 | EXEC/REVIEW/WATCH+ | 15M正向 | 30M正向 | 1H正向 | 当前正向 | TP0先于SL | SL先触发 | 当前均值 | MFE均值 | MAE均值 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| trend_breakout_long | 119 | 0/0/119 | 21.0% | 17.6% | 17.6% | 16.8% | 0.0% | 71.4% | -0.27% | +0.17% | -0.56% |
| funding_reversal | 53 | 0/0/53 | 47.2% | 58.5% | 58.5% | 56.6% | 50.9% | 3.8% | -0.01% | +0.54% | -0.52% |
| alt_ladder_breakdown_short | 13 | 0/0/13 | 76.9% | 76.9% | 76.9% | 76.9% | 69.2% | 0.0% | +0.95% | +1.74% | -0.44% |
| alt_ladder_momentum_long | 11 | 1/2/8 | 54.5% | 45.5% | 45.5% | 63.6% | 63.6% | 9.1% | +0.21% | +1.41% | -0.97% |
| range_expansion_event | 2 | 0/0/2 | 0.0% | 100.0% | 100.0% | 100.0% | 100.0% | 0.0% | +0.83% | +1.59% | -2.53% |

## 执行层统计

| 分组 | 样本 | EXEC/REVIEW/WATCH+ | 15M正向 | 30M正向 | 1H正向 | 当前正向 | TP0先于SL | SL先触发 | 当前均值 | MFE均值 | MAE均值 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| EXECUTABLE+REVIEWABLE | 3 | 1/2/0 | 33.3% | 0.0% | 0.0% | 33.3% | 0.0% | 33.3% | -0.56% | +0.58% | -1.26% |
| alt_ladder_momentum_long | 3 | 1/2/0 | 33.3% | 0.0% | 0.0% | 33.3% | 0.0% | 33.3% | -0.56% | +0.58% | -1.26% |

## 代表性标的

### 有效止盈样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| BANKUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +1.15% | +3.89% | +3.89% | +3.85% | +6.23% / +0.00% | WIN_TP1 | 2026-08-03 06:31:00 |
| STARUSDT | SHORT | funding_reversal | BLOCKED | +3.89% | +4.19% | +4.19% | +4.15% | +5.66% / -0.31% | WIN_TP0_ACTIVE | 2026-08-03 06:41:00 |
| AIOUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +2.14% | +1.15% | +1.15% | +1.40% | +3.16% / -0.63% | WIN_TP0_ACTIVE | 2026-08-03 06:39:00 |
| AIOUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +2.00% | +1.15% | +1.15% | +1.37% | +3.13% / -1.02% | WIN_TP0_ACTIVE | 2026-08-03 06:40:00 |
| EDENUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +2.30% | +0.83% | +0.83% | +0.81% | +2.67% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 06:38:00 |
| BICOUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +2.26% | +2.18% | +2.18% | +2.33% | +2.61% / -0.99% | WIN_TP0_ACTIVE | 2026-08-03 06:42:00 |
| HYPERUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +1.97% | +2.08% | +2.08% | +2.31% | +2.56% / -0.11% | WIN_TP0_ACTIVE | 2026-08-03 06:40:00 |
| BICOUSDT | LONG | alt_ladder_momentum_long | BLOCKED | -0.70% | +2.04% | +2.04% | +2.11% | +2.39% / -1.20% | WIN_TP0_ACTIVE | 2026-08-03 06:24:00 |
| FHEUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +0.97% | +1.05% | +1.05% | +1.09% | +1.69% / -0.08% | WIN_TP0_ACTIVE | 2026-08-03 06:37:00 |
| ONDOUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +1.14% | +0.96% | +0.96% | +0.96% | +1.69% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 06:28:00 |

### 逆向/亏损样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| HYPERUSDT | LONG | trend_breakout_long | BLOCKED | -1.12% | -2.96% | -2.96% | -2.96% | +0.15% / -3.22% | STOP_FIRST | 2026-08-03 06:32:00 |
| BLESSUSDT | SHORT | funding_reversal | BLOCKED | -2.26% | -2.44% | -2.44% | -2.37% | +2.05% / -2.90% | STOP_FIRST | 2026-08-03 06:48:00 |
| MMTUSDT | LONG | funding_reversal | BLOCKED | -0.73% | -1.75% | -1.75% | -1.69% | +0.12% / -1.93% | ACTIVE_LOSS | - |
| USUSDT | LONG | trend_breakout_long | BLOCKED | -2.00% | -1.50% | -1.50% | -1.51% | +0.00% / -6.01% | STOP_FIRST | 2026-08-03 06:37:00 |
| PIEVERSEUSDT | LONG | alt_ladder_momentum_long | BLOCKED | -0.94% | -1.33% | -1.33% | -1.44% | +0.29% / -1.53% | ACTIVE_LOSS | - |
| DUSKUSDT | SHORT | funding_reversal | BLOCKED | -1.07% | -1.44% | -1.44% | -1.44% | +0.05% / -2.86% | STOP_FIRST | 2026-08-03 06:31:00 |
| ICNTUSDT | LONG | funding_reversal | BLOCKED | -0.31% | -1.15% | -1.15% | -1.15% | +0.23% / -1.30% | ACTIVE_LOSS | - |
| STRKUSDT | SHORT | funding_reversal | BLOCKED | -1.15% | -1.23% | -1.23% | -1.15% | +0.79% / -1.74% | PROTECTED_AFTER_TP0 | 2026-08-03 06:34:00 |
| BULLAUSDT | LONG | trend_breakout_long | BLOCKED | -0.74% | -1.08% | -1.08% | -1.13% | +0.20% / -1.18% | ACTIVE_LOSS | - |
| MYXUSDT | LONG | trend_breakout_long | BLOCKED | -1.03% | -1.11% | -1.11% | -1.11% | +0.07% / -1.43% | STOP_FIRST | 2026-08-03 06:28:00 |

### 先止损或同 K 歧义样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| ARXUSDT | LONG | trend_breakout_long | BLOCKED | -0.26% | -0.52% | -0.52% | -0.52% | +0.07% / -0.59% | STOP_FIRST | 2026-08-03 06:27:00 |
| SQDUSDT | LONG | trend_breakout_long | BLOCKED | -0.12% | +0.37% | +0.37% | +0.37% | +0.56% / -0.22% | STOP_FIRST | 2026-08-03 06:24:00 |
| TREEUSDT | LONG | trend_breakout_long | BLOCKED | -0.03% | -0.18% | -0.18% | -0.18% | +0.41% / -0.26% | STOP_FIRST | 2026-08-03 06:23:00 |
| LTCUSDT | LONG | trend_breakout_long | BLOCKED | -0.29% | -0.44% | -0.44% | -0.42% | +0.00% / -0.53% | STOP_FIRST | 2026-08-03 06:33:00 |
| SANDUSDT | LONG | trend_breakout_long | BLOCKED | -0.53% | -0.69% | -0.69% | -0.69% | +0.00% / -0.72% | STOP_FIRST | 2026-08-03 06:26:00 |
| ZILUSDT | LONG | trend_breakout_long | BLOCKED | -0.43% | -0.67% | -0.67% | -0.67% | +0.00% / -0.78% | STOP_FIRST | 2026-08-03 06:23:00 |
| FILUSDT | LONG | trend_breakout_long | BLOCKED | -0.47% | -0.44% | -0.44% | -0.44% | +0.03% / -0.65% | STOP_FIRST | 2026-08-03 06:26:00 |
| OPUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | -1.03% | -1.03% | -1.03% | -1.03% | +0.00% / -1.37% | STOP_FIRST | 2026-08-03 06:39:00 |
| SONICUSDT | LONG | trend_breakout_long | BLOCKED | -0.44% | -0.49% | -0.49% | -0.49% | +0.00% / -0.77% | STOP_FIRST | 2026-08-03 06:33:00 |
| HYPERUSDT | LONG | trend_breakout_long | BLOCKED | -1.12% | -2.96% | -2.96% | -2.96% | +0.15% / -3.22% | STOP_FIRST | 2026-08-03 06:32:00 |
| XVGUSDT | LONG | trend_breakout_long | BLOCKED | -0.26% | -0.47% | -0.47% | -0.42% | +0.05% / -0.73% | STOP_FIRST | 2026-08-03 06:23:00 |
| DUSKUSDT | SHORT | funding_reversal | BLOCKED | -1.07% | -1.44% | -1.44% | -1.44% | +0.05% / -2.86% | STOP_FIRST | 2026-08-03 06:31:00 |

### 先打损后行情修复样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| SQDUSDT | LONG | trend_breakout_long | BLOCKED | -0.12% | +0.37% | +0.37% | +0.37% | +0.56% / -0.22% | STOP_FIRST | 2026-08-03 06:24:00 |
| SQDUSDT | LONG | trend_breakout_long | BLOCKED | +0.31% | +0.25% | +0.25% | +0.31% | +0.49% / -0.22% | STOP_FIRST | 2026-08-03 06:35:00 |
| MIRAUSDT | LONG | trend_breakout_long | BLOCKED | +0.29% | +0.34% | +0.34% | +0.29% | +0.56% / -0.10% | STOP_FIRST | 2026-08-03 06:34:00 |
| MIRAUSDT | LONG | trend_breakout_long | BLOCKED | +0.15% | +0.29% | +0.29% | +0.24% | +0.51% / -0.19% | STOP_FIRST | 2026-08-03 06:28:00 |
| ARBUSDT | LONG | trend_breakout_long | BLOCKED | +0.15% | +0.16% | +0.16% | +0.16% | +0.18% / -0.28% | STOP_FIRST | 2026-08-03 06:37:00 |
| DOTUSDT | LONG | trend_breakout_long | BLOCKED | +0.13% | +0.13% | +0.13% | +0.13% | +0.20% / -0.19% | STOP_FIRST | 2026-08-03 06:34:00 |
| BANUSDT | LONG | trend_breakout_long | BLOCKED | +0.08% | +0.10% | +0.10% | +0.10% | +0.25% / -0.08% | STOP_FIRST | 2026-08-03 06:23:00 |
| ARBUSDT | LONG | trend_breakout_long | BLOCKED | -0.16% | +0.04% | +0.04% | +0.04% | +0.06% / -0.41% | STOP_FIRST | 2026-08-03 06:37:00 |

### TP0 后保护样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| KOMAUSDT | LONG | range_expansion_event | BLOCKED | -0.89% | +0.82% | +0.82% | +0.87% | +1.91% / -4.20% | PROTECTED_AFTER_TP0 | 2026-08-03 06:23:00 |
| STRKUSDT | SHORT | funding_reversal | BLOCKED | -1.15% | -1.23% | -1.23% | -1.15% | +0.79% / -1.74% | PROTECTED_AFTER_TP0 | 2026-08-03 06:34:00 |

### WATCH/BLOCKED 错失机会复核池

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| STARUSDT | SHORT | funding_reversal | BLOCKED | +3.89% | +4.19% | +4.19% | +4.15% | +5.66% / -0.31% | WIN_TP0_ACTIVE | 2026-08-03 06:41:00 |
| BANKUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +1.15% | +3.89% | +3.89% | +3.85% | +6.23% / +0.00% | WIN_TP1 | 2026-08-03 06:31:00 |
| BICOUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +2.26% | +2.18% | +2.18% | +2.33% | +2.61% / -0.99% | WIN_TP0_ACTIVE | 2026-08-03 06:42:00 |
| HYPERUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +1.97% | +2.08% | +2.08% | +2.31% | +2.56% / -0.11% | WIN_TP0_ACTIVE | 2026-08-03 06:40:00 |
| BICOUSDT | LONG | alt_ladder_momentum_long | BLOCKED | -0.70% | +2.04% | +2.04% | +2.11% | +2.39% / -1.20% | WIN_TP0_ACTIVE | 2026-08-03 06:24:00 |
| AIOUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +2.14% | +1.15% | +1.15% | +1.40% | +3.16% / -0.63% | WIN_TP0_ACTIVE | 2026-08-03 06:39:00 |
| AIOUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +2.00% | +1.15% | +1.15% | +1.37% | +3.13% / -1.02% | WIN_TP0_ACTIVE | 2026-08-03 06:40:00 |
| GWEIUSDT | LONG | trend_breakout_long | BLOCKED | +1.50% | +1.25% | +1.25% | +1.19% | +1.63% / -0.06% | ACTIVE_PROFIT | - |
| GWEIUSDT | LONG | trend_breakout_long | BLOCKED | +0.19% | +1.19% | +1.19% | +1.12% | +1.56% / -0.25% | ACTIVE_PROFIT | - |
| FHEUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +0.97% | +1.05% | +1.05% | +1.09% | +1.69% / -0.08% | WIN_TP0_ACTIVE | 2026-08-03 06:37:00 |
| LAUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +0.73% | +1.04% | +1.04% | +0.99% | +1.28% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 06:47:00 |
| ONDOUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +1.14% | +0.96% | +0.96% | +0.96% | +1.69% / +0.00% | WIN_TP0_ACTIVE | 2026-08-03 06:28:00 |

## 可实施优化方案

1. `alt_ladder_momentum_long`：保持“中段 + OI 正增 + taker > 0.52”作为主通道，但把 `alt_ladder_stage_late + high_volatility/funding_elevated` 组合降为 REVIEW/WATCH，并要求 TP0 触发后立即保本；本次样本里该形态早段有止盈能力，但追高样本 MFE 与 MAE 摆动过大。
2. `range_expansion_event`：LONG 必须增加“反抽失败未出现”的二次确认：高位 entry_zone_position、1h OI 转负或 reason 含 `short_covering_not_new_long_build` 时，不进入主开仓率；只有 5m 回踩 VWAP/EMA20 后重新放量站回，才升到 REVIEWABLE。
3. `alt_ladder_breakdown_short`：跨轮二次确认应以“下一轮低点下移/5m EMA20 下压/买盘 taker 未反包”为核心；若首轮只是挤压后的短线回撤，不应直接开空。当前复盘应把 BLOCKED/WATCH 的顺向下跌标的纳入 missed-opportunity 队列，而不是放宽主开仓门槛。
4. `funding_reversal`：继续从主开仓率拆桶，单独做事件型观察。只有 funding 极值、OI 与价格反向、taker 已转向三项同时满足时才进入 REVIEW；否则仅参与风控解释，不参与执行候选。
5. 执行提示词：开仓指令里加入“TP0 距离小于 0.45R 或 stop_distance > TP0_distance * 2 时禁止市价追入”，优先等待回踩入场；这能减少先触发 SL 的单边追高样本。
6. 统计口径：`PROTECTED_AFTER_TP0` 计为风控有效，不计入亏损止损；但若同形态连续出现 TP0 后快速回撤，策略提示词应从“可追随”改成“只做 TP0 快进快出”。
7. 数据质量：上一轮 validator 有 universe coverage 低的问题，实盘开仓率评估必须保留 valid_rounds 门槛；低覆盖轮只能用于形态复盘，不能用于放宽执行层阈值。

## 落地优先级

1. P0：在 `provider/local/hunter_v7_mod_range_expansion_event.go` 把 LONG 的高位 + OI 转负/非新多建仓改为硬降档；提示词同步要求“反抽失败未出现 + 回踩后重新放量站回”。
2. P0：在 `kernel/hunter_v7_prompt_doctrine.go` 或提示词 payload 中，把开仓许可改成双阈值：`entry_zone_position <= 72` 且 `TP0_distance >= 0.45R`；不满足时只能 REVIEW/WATCH。
3. P1：在 `provider/local/hunter_v7_mod_alt_ladder.go` 和策略提示词中，对 `alt_ladder_stage_late + high_volatility/funding_elevated` 降档，并把 TP0 后保本写成强制动作。
4. P1：在 `provider/local/hunter_v7_mod_whale_flow.go` 增加二次确认池：首轮 taker 不足但 OI 明显累积的样本，下一轮 taker 回升且价格未跌破入场带再升档。
5. P2：在 `cmd/hunter_v7_validate` 和长期 outcome 表中增加 `direction_accuracy_15m/30m/1h/current`、`tp0_before_sl_rate`、`protected_after_tp0_rate`，让开仓率优化不和盈利质量混在一个指标里。

## 附件

- 明细 JSON：reports/hunter-v7-watch-audit-20260802/hunter-v7-latest-binance-replay-20260802T225008.json
- 明细 CSV：reports/hunter-v7-watch-audit-20260802/hunter-v7-latest-binance-replay-20260802T225008.csv

