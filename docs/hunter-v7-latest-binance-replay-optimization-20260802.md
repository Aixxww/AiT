# Hunter v7 最新币安合约信号复盘与优化方案

- 生成时间：2026-08-02 18:45:12 CST
- 数据源：Binance USDS-M Futures public REST，1m K 线 + 当前 ticker，未执行任何交易动作。
- 复盘信号：数据库 `hunter_v7_signal_records` 自 2026-08-02 01:50:00 UTC 起的执行层信号，以及核心形态 WATCH/BLOCKED 候选。
- 去重口径：同一 30 分钟内相同 `symbol + direction + setup_type` 只保留最高执行层记录，重复轮次只计入 duplicate_rows。
- 当前币安服务器时间：2026-08-02 18:44:16 CST。

## 结论

1. 执行层样本 12 个，15M/30M/1H 方向正收益率分别为 66.7%、66.7%、58.3%；当前正收益率 58.3%，平均当前 PnL +2.94%。
2. 按真实交易路径看，执行层 TP0 先于 SL 的有效保护/止盈率为 66.7%（8/12），其中 TP1/TP2 完整止盈 5 个，TP0 后保护 3 个，先打损 4 个。
3. 全形态观察样本 83 个，TP0 先于止损触发率 66.3%，先止损率 31.3%；说明筛选层仍能发现波动机会，但主开仓层必须继续过滤追高、反抽失败和先扫损后修复。
4. 当前价格一致性不能等同于实盘胜率：本次有 7 个样本先触发 SL 后才转为当前顺向，提示词应要求“等待回踩确认/二次确认”而不是扩大市价追入。

## 按形态统计

| 形态 | 样本 | EXEC/REVIEW/WATCH+ | 15M正向 | 30M正向 | 1H正向 | 当前正向 | TP0先于SL | SL先触发 | 当前均值 | MFE均值 | MAE均值 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| funding_reversal | 22 | 0/0/22 | 31.8% | 31.8% | 36.4% | 54.5% | 50.0% | 45.5% | -0.29% | +2.96% | -4.02% |
| alt_ladder_momentum_long | 15 | 1/5/9 | 46.7% | 53.3% | 40.0% | 53.3% | 60.0% | 40.0% | +1.53% | +9.32% | -6.53% |
| alt_ladder_breakdown_short | 15 | 0/0/15 | 60.0% | 53.3% | 66.7% | 73.3% | 80.0% | 20.0% | +2.47% | +8.74% | -6.01% |
| range_expansion_event | 14 | 1/0/13 | 21.4% | 28.6% | 28.6% | 14.3% | 57.1% | 42.9% | -11.87% | +6.16% | -18.59% |
| trend_breakout_long | 6 | 0/1/5 | 83.3% | 83.3% | 50.0% | 50.0% | 83.3% | 16.7% | -0.82% | +2.34% | -3.21% |
| whale_flow_reversal | 6 | 0/2/4 | 100.0% | 50.0% | 66.7% | 66.7% | 100.0% | 0.0% | +0.67% | +5.34% | -3.28% |
| displacement_momentum_long | 5 | 0/2/3 | 80.0% | 60.0% | 80.0% | 100.0% | 80.0% | 0.0% | +3.00% | +3.91% | -0.21% |

## 执行层统计

| 分组 | 样本 | EXEC/REVIEW/WATCH+ | 15M正向 | 30M正向 | 1H正向 | 当前正向 | TP0先于SL | SL先触发 | 当前均值 | MFE均值 | MAE均值 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| EXECUTABLE+REVIEWABLE | 12 | 2/10/0 | 66.7% | 66.7% | 58.3% | 58.3% | 66.7% | 33.3% | +2.94% | +8.71% | -3.71% |
| alt_ladder_momentum_long | 6 | 1/5/0 | 50.0% | 66.7% | 50.0% | 66.7% | 50.0% | 50.0% | +7.14% | +12.54% | -3.76% |
| displacement_momentum_long | 2 | 0/2/0 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 0.0% | +4.70% | +5.67% | -0.27% |
| whale_flow_reversal | 2 | 0/2/0 | 100.0% | 50.0% | 100.0% | 50.0% | 100.0% | 0.0% | -1.83% | +5.24% | -3.12% |
| range_expansion_event | 1 | 1/0/0 | 0.0% | 0.0% | 0.0% | 0.0% | 0.0% | 100.0% | -7.60% | +4.37% | -8.04% |
| trend_breakout_long | 1 | 0/1/0 | 100.0% | 100.0% | 0.0% | 0.0% | 100.0% | 0.0% | -5.70% | +3.07% | -7.14% |

## 代表性标的

### 有效止盈样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| BLESSUSDT | LONG | range_expansion_event | WATCH | +8.12% | +5.98% | +19.10% | +38.61% | +45.00% / +0.00% | WIN_TP2 | 2026-08-02 10:07:00 |
| HOMEUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | +0.55% | +3.27% | +5.49% | +19.23% | +28.39% / -0.13% | WIN_TP2 | 2026-08-02 09:59:00 |
| HOMEUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +0.03% | +3.76% | +6.12% | +18.77% | +27.90% / -0.52% | WIN_TP2 | 2026-08-02 10:07:00 |
| HOMEUSDT | LONG | alt_ladder_momentum_long | WATCH | +0.52% | +1.25% | +4.71% | +9.78% | +18.22% / -1.76% | WIN_TP2 | 2026-08-02 13:19:00 |
| BEATUSDT | SHORT | alt_ladder_breakdown_short | WATCH | +1.04% | +2.77% | +6.16% | -0.65% | +16.73% / -6.91% | WIN_TP1 | 2026-08-02 10:13:00 |
| BEATUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +2.29% | +3.71% | +6.60% | -0.75% | +16.65% / -7.01% | WIN_TP1 | 2026-08-02 10:06:00 |
| KOMAUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -0.09% | +6.34% | +4.87% | +10.17% | +15.02% / -1.06% | WIN_TP0_ACTIVE | 2026-08-02 12:52:00 |
| KOMAUSDT | SHORT | whale_flow_reversal | WATCH | +4.87% | +4.30% | +9.24% | +9.63% | +14.52% / +0.00% | WIN_TP0_ACTIVE | 2026-08-02 12:53:00 |
| USUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -1.13% | -1.15% | +4.00% | +4.67% | +11.82% / -5.95% | WIN_TP1 | 2026-08-02 13:33:00 |
| GRVTUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +0.43% | +0.61% | +0.29% | +9.56% | +11.57% / -0.86% | WIN_TP1 | 2026-08-02 13:08:00 |

### 逆向/亏损样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| BLESSUSDT | SHORT | range_expansion_event | BLOCKED | -11.79% | -9.25% | -22.00% | -43.52% | +2.97% / -50.13% | PROTECTED_AFTER_TP0 | 2026-08-02 09:59:00 |
| BLESSUSDT | SHORT | range_expansion_event | BLOCKED | -6.36% | -7.95% | -5.79% | -28.64% | +0.00% / -34.57% | STOP_FIRST | 2026-08-02 12:45:00 |
| AKEUSDT | LONG | range_expansion_event | BLOCKED | -21.97% | -14.77% | -21.83% | -25.95% | +0.00% / -33.50% | STOP_FIRST | 2026-08-02 10:15:00 |
| AKEUSDT | LONG | range_expansion_event | BLOCKED | -5.62% | -18.88% | -15.91% | -24.50% | +4.38% / -32.19% | PROTECTED_AFTER_TP0 | 2026-08-02 09:59:00 |
| STARUSDT | SHORT | funding_reversal | REJECTED | -4.55% | -16.55% | -15.84% | -23.95% | +0.00% / -30.12% | STOP_FIRST | 2026-08-02 12:48:00 |
| 1000RATSUSDT | LONG | range_expansion_event | BLOCKED | -1.06% | -10.61% | -2.10% | -23.82% | +4.65% / -24.65% | STOP_FIRST | 2026-08-02 10:15:00 |
| 1000RATSUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +1.27% | +2.09% | +2.95% | -22.18% | +6.91% / -23.02% | PROTECTED_AFTER_TP0 | 2026-08-02 13:23:00 |
| STARUSDT | SHORT | funding_reversal | REJECTED | +2.11% | -2.69% | -7.68% | -22.16% | +2.50% / -28.24% | STOP_FIRST | 2026-08-02 12:50:00 |
| KOMAUSDT | LONG | range_expansion_event | BLOCKED | -0.64% | -16.58% | -16.59% | -19.54% | +1.48% / -23.89% | PROTECTED_AFTER_TP0 | 2026-08-02 10:06:00 |
| BEATUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +1.87% | +2.29% | -0.70% | -16.85% | +3.33% / -24.12% | PROTECTED_AFTER_TP0 | 2026-08-02 12:47:00 |

### 先止损或同 K 歧义样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| EULUSDT | LONG | alt_ladder_momentum_long | WATCH | -0.24% | -3.37% | -4.98% | -9.93% | +1.94% / -12.18% | STOP_FIRST | 2026-08-02 10:19:00 |
| 1000SATSUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -4.56% | -3.10% | +0.64% | +3.56% | +4.01% / -8.94% | STOP_FIRST | 2026-08-02 10:05:00 |
| SUIUSDT | SHORT | funding_reversal | BLOCKED | -0.69% | -0.66% | -0.83% | -0.72% | +0.18% / -1.60% | STOP_FIRST | 2026-08-02 10:07:00 |
| ONUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +0.58% | -3.64% | -4.36% | -4.09% | +1.81% / -7.99% | STOP_FIRST | 2026-08-02 10:17:00 |
| PUMPUSDT | LONG | alt_ladder_momentum_long | EXECUTABLE | -1.71% | -0.92% | -1.31% | -5.87% | +0.13% / -5.96% | STOP_FIRST | 2026-08-02 10:04:00 |
| ORDIUSDT | SHORT | funding_reversal | BLOCKED | -2.15% | -1.48% | +0.70% | +0.61% | +1.14% / -4.63% | STOP_FIRST | 2026-08-02 10:12:00 |
| ONUSDT | LONG | alt_ladder_momentum_long | BLOCKED | -2.81% | -2.97% | -3.40% | -3.74% | +2.05% / -7.66% | STOP_FIRST | 2026-08-02 10:18:00 |
| FILUSDT | SHORT | funding_reversal | BLOCKED | -0.96% | -0.61% | -0.76% | -1.13% | +0.00% / -1.94% | STOP_FIRST | 2026-08-02 10:11:00 |
| LTCUSDT | SHORT | funding_reversal | WATCH | -0.38% | -0.11% | -0.18% | -0.34% | +0.00% / -0.72% | STOP_FIRST | 2026-08-02 14:48:00 |
| SUIUSDT | SHORT | funding_reversal | BLOCKED | -1.01% | -0.73% | -0.87% | -0.88% | +0.00% / -1.76% | STOP_FIRST | 2026-08-02 10:07:00 |
| TAOUSDT | SHORT | funding_reversal | BLOCKED | -0.86% | -0.48% | -0.46% | -0.01% | +0.24% / -1.60% | STOP_FIRST | 2026-08-02 10:07:00 |
| UAIUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | -2.00% | -1.63% | -2.67% | +0.55% | +5.29% / -3.94% | STOP_FIRST | 2026-08-02 10:27:00 |

### 先打损后行情修复样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| BLESSUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | -4.32% | +4.12% | +7.18% | +27.95% | +33.85% / -5.80% | STOP_FIRST | 2026-08-02 12:26:00 |
| KOMAUSDT | SHORT | range_expansion_event | BLOCKED | +4.49% | +2.48% | +6.24% | +8.20% | +13.17% / -3.26% | STOP_FIRST | 2026-08-02 12:45:00 |
| BTWUSDT | SHORT | alt_ladder_breakdown_short | WATCH | +0.46% | -3.60% | -1.75% | +4.73% | +5.68% / -5.37% | STOP_FIRST | 2026-08-02 12:50:00 |
| BTWUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -4.60% | -3.61% | -0.92% | +3.99% | +4.95% / -6.19% | STOP_FIRST | 2026-08-02 12:45:00 |
| 1000SATSUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -4.56% | -3.10% | +0.64% | +3.56% | +4.01% / -8.94% | STOP_FIRST | 2026-08-02 10:05:00 |
| ORDIUSDT | SHORT | funding_reversal | BLOCKED | -2.15% | -1.48% | +0.70% | +0.61% | +1.14% / -4.63% | STOP_FIRST | 2026-08-02 10:12:00 |
| UAIUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | -2.00% | -1.63% | -2.67% | +0.55% | +5.29% / -3.94% | STOP_FIRST | 2026-08-02 10:27:00 |

### TP0 后保护样本

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| KOMAUSDT | LONG | range_expansion_event | BLOCKED | -0.64% | -16.58% | -16.59% | -19.54% | +1.48% / -23.89% | PROTECTED_AFTER_TP0 | 2026-08-02 10:06:00 |
| BLESSUSDT | SHORT | range_expansion_event | BLOCKED | -11.79% | -9.25% | -22.00% | -43.52% | +2.97% / -50.13% | PROTECTED_AFTER_TP0 | 2026-08-02 09:59:00 |
| AKEUSDT | LONG | range_expansion_event | BLOCKED | -5.62% | -18.88% | -15.91% | -24.50% | +4.38% / -32.19% | PROTECTED_AFTER_TP0 | 2026-08-02 09:59:00 |
| EPICUSDT | LONG | whale_flow_reversal | WATCH | +0.57% | -0.40% | -0.06% | +0.96% | +2.83% / -7.98% | PROTECTED_AFTER_TP0 | 2026-08-02 10:10:00 |
| BTWUSDT | LONG | range_expansion_event | BLOCKED | -0.11% | -0.01% | +1.46% | -13.11% | +2.35% / -13.98% | PROTECTED_AFTER_TP0 | 2026-08-02 10:44:00 |
| PUMPUSDT | LONG | whale_flow_reversal | WATCH | +1.29% | +0.85% | +0.22% | -4.11% | +2.01% / -4.19% | PROTECTED_AFTER_TP0 | 2026-08-02 10:21:00 |
| EULUSDT | LONG | alt_ladder_momentum_long | WATCH | -3.22% | -2.59% | -0.99% | -10.22% | +1.61% / -12.47% | PROTECTED_AFTER_TP0 | 2026-08-02 10:07:00 |
| EPICUSDT | LONG | alt_ladder_momentum_long | BLOCKED | -0.20% | +0.13% | -0.50% | +0.19% | +2.05% / -8.68% | PROTECTED_AFTER_TP0 | 2026-08-02 10:10:00 |
| AEVOUSDT | LONG | trend_breakout_long | REVIEWABLE | +2.01% | +1.87% | -0.77% | -5.70% | +3.07% / -7.14% | PROTECTED_AFTER_TP0 | 2026-08-02 12:54:00 |
| BEATUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +1.87% | +2.29% | -0.70% | -16.85% | +3.33% / -24.12% | PROTECTED_AFTER_TP0 | 2026-08-02 12:47:00 |
| 1000RATSUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +1.27% | +2.09% | +2.95% | -22.18% | +6.91% / -23.02% | PROTECTED_AFTER_TP0 | 2026-08-02 13:23:00 |
| ONUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | +2.40% | +0.70% | -0.09% | -2.43% | +3.44% / -6.40% | PROTECTED_AFTER_TP0 | 2026-08-02 12:46:00 |

### WATCH/BLOCKED 错失机会复核池

| 标的 | 方向 | 形态 | 层级 | 15M | 30M | 1H | 当前 | MFE/MAE | 结果 | 首事件时间 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| BLESSUSDT | LONG | range_expansion_event | WATCH | +8.12% | +5.98% | +19.10% | +38.61% | +45.00% / +0.00% | WIN_TP2 | 2026-08-02 10:07:00 |
| HOMEUSDT | LONG | alt_ladder_momentum_long | BLOCKED | +0.03% | +3.76% | +6.12% | +18.77% | +27.90% / -0.52% | WIN_TP2 | 2026-08-02 10:07:00 |
| KOMAUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | -0.09% | +6.34% | +4.87% | +10.17% | +15.02% / -1.06% | WIN_TP0_ACTIVE | 2026-08-02 12:52:00 |
| HOMEUSDT | LONG | alt_ladder_momentum_long | WATCH | +0.52% | +1.25% | +4.71% | +9.78% | +18.22% / -1.76% | WIN_TP2 | 2026-08-02 13:19:00 |
| KOMAUSDT | SHORT | whale_flow_reversal | WATCH | +4.87% | +4.30% | +9.24% | +9.63% | +14.52% / +0.00% | WIN_TP0_ACTIVE | 2026-08-02 12:53:00 |
| GRVTUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +0.43% | +0.61% | +0.29% | +9.56% | +11.57% / -0.86% | WIN_TP1 | 2026-08-02 13:08:00 |
| COTIUSDT | SHORT | alt_ladder_breakdown_short | BLOCKED | +0.76% | +2.82% | +3.46% | +8.50% | +8.66% / -0.63% | WIN_TP1 | 2026-08-02 10:15:00 |
| KOMAUSDT | SHORT | range_expansion_event | BLOCKED | +4.49% | +2.48% | +6.24% | +8.20% | +13.17% / -3.26% | STOP_FIRST | 2026-08-02 12:45:00 |
| PHAROSUSDT | SHORT | funding_reversal | REJECTED | -0.40% | +0.93% | +3.79% | +7.40% | +8.43% / -0.88% | WIN_TP1 | 2026-08-02 13:29:00 |
| APRUSDT | SHORT | funding_reversal | REJECTED | -0.56% | -1.03% | -0.34% | +6.59% | +7.19% / -1.46% | WIN_TP1 | 2026-08-02 15:18:00 |
| LIGHTUSDT | SHORT | funding_reversal | REJECTED | +2.79% | +3.05% | +2.72% | +6.16% | +7.71% / -0.06% | WIN_TP1 | 2026-08-02 10:11:00 |
| LIGHTUSDT | SHORT | funding_reversal | REJECTED | +3.18% | +3.31% | +3.37% | +6.04% | +7.59% / -0.19% | WIN_TP1 | 2026-08-02 10:11:00 |

## 可实施优化方案

1. `alt_ladder_momentum_long`：保持“中段 + OI 正增 + taker > 0.52”作为主通道，但把 `alt_ladder_stage_late + high_volatility/funding_elevated` 组合降为 REVIEW/WATCH，并要求 TP0 触发后立即保本；本次样本里该形态早段有止盈能力，但追高样本 MFE 与 MAE 摆动过大。
2. `range_expansion_event`：LONG 必须增加“反抽失败未出现”的二次确认：高位 entry_zone_position、1h OI 转负或 reason 含 `short_covering_not_new_long_build` 时，不进入主开仓率；只有 5m 回踩 VWAP/EMA20 后重新放量站回，才升到 REVIEWABLE。
3. `whale_flow_reversal`：当前逻辑对 taker 不足的拦截总体合理，但需要把 `OI 快速增加 + 价格未充分标记 + funding_not_crowded` 作为“错失机会复核池”；若 15m/30m 后价格仍在入场带上方且 taker 回到 0.54-0.56，再允许二次确认升档。
4. `alt_ladder_breakdown_short`：跨轮二次确认应以“下一轮低点下移/5m EMA20 下压/买盘 taker 未反包”为核心；若首轮只是挤压后的短线回撤，不应直接开空。当前复盘应把 BLOCKED/WATCH 的顺向下跌标的纳入 missed-opportunity 队列，而不是放宽主开仓门槛。
5. `funding_reversal`：继续从主开仓率拆桶，单独做事件型观察。只有 funding 极值、OI 与价格反向、taker 已转向三项同时满足时才进入 REVIEW；否则仅参与风控解释，不参与执行候选。
6. 执行提示词：开仓指令里加入“TP0 距离小于 0.45R 或 stop_distance > TP0_distance * 2 时禁止市价追入”，优先等待回踩入场；这能减少先触发 SL 的单边追高样本。
7. 统计口径：`PROTECTED_AFTER_TP0` 计为风控有效，不计入亏损止损；但若同形态连续出现 TP0 后快速回撤，策略提示词应从“可追随”改成“只做 TP0 快进快出”。
8. 数据质量：上一轮 validator 有 universe coverage 低的问题，实盘开仓率评估必须保留 valid_rounds 门槛；低覆盖轮只能用于形态复盘，不能用于放宽执行层阈值。

## 落地优先级

1. P0：在 `provider/local/hunter_v7_mod_range_expansion_event.go` 把 LONG 的高位 + OI 转负/非新多建仓改为硬降档；提示词同步要求“反抽失败未出现 + 回踩后重新放量站回”。
2. P0：在 `kernel/hunter_v7_prompt_doctrine.go` 或提示词 payload 中，把开仓许可改成双阈值：`entry_zone_position <= 72` 且 `TP0_distance >= 0.45R`；不满足时只能 REVIEW/WATCH。
3. P1：在 `provider/local/hunter_v7_mod_alt_ladder.go` 和策略提示词中，对 `alt_ladder_stage_late + high_volatility/funding_elevated` 降档，并把 TP0 后保本写成强制动作。
4. P1：在 `provider/local/hunter_v7_mod_whale_flow.go` 增加二次确认池：首轮 taker 不足但 OI 明显累积的样本，下一轮 taker 回升且价格未跌破入场带再升档。
5. P2：在 `cmd/hunter_v7_validate` 和长期 outcome 表中增加 `direction_accuracy_15m/30m/1h/current`、`tp0_before_sl_rate`、`protected_after_tp0_rate`，让开仓率优化不和盈利质量混在一个指标里。

## 附件

- 明细 JSON：reports/hunter-v7-latest-binance-replay-20260802/hunter-v7-latest-binance-replay-20260802T104512.json
- 明细 CSV：reports/hunter-v7-latest-binance-replay-20260802/hunter-v7-latest-binance-replay-20260802T104512.csv

