# Hunter v7 实时信号 JSON 与 AIT 识别验证报告

> 生成时间：2026-06-04 20:57:23 CST
> 原始 JSON：`reports/hunter-v7-live-validation-raw-20260604-205723.json`
> Prompt 预览：`reports/hunter-v7-live-prompt-20260604-205723.txt`

## 1. 结论

- JSON 结构可序列化/反序列化，核心字段完整，AIT prompt 已包含 `hunter_v7_signal_json`，AI 可以直接识别 v7 标签。
- 本轮实时输出 24 个信号：LONG=18，SHORT=6，setup=2 类。
- 市场 regime=trend_down，BTC 24h=-4.91%，ETH 24h=-5.30%。

## 2. JSON / Prompt 校验

| 项目 | 结果 |
|---|---|
| JSON marshal | true |
| JSON unmarshal | true |
| 缺字段数 | 0 |
| 执行性缺口 | 0 |
| Prompt 含 v7 JSON | true |
| Prompt bytes | 71972 |

## 3. 实时信号

| # | Symbol | Dir | Setup | Status | Priority | Timing | Risk | Entry | Reasons |
|---:|---|---|---|---|---:|---:|---:|---|---|
| 1 | MYXUSDT | LONG | `panic_reversal_long` | `candidate` | 65.1 | 45.0 | 23.0 | `wait_reclaim` | heavy_capitulation, oi_declining, strong_reclaim, taker_buy_strong, selling_decelerating, 1h_green_shoot |
| 2 | FETUSDT | LONG | `panic_reversal_long` | `candidate` | 61.2 | 40.0 | 15.0 | `wait_reclaim` | moderate_capitulation, oi_declining, strong_reclaim, taker_buy_recovering, selling_decelerating, 1h_green_shoot, rsi_recovering_from_extreme |
| 3 | TRADOORUSDT | LONG | `panic_reversal_long` | `candidate` | 60.6 | 45.0 | 23.0 | `wait_reclaim` | moderate_capitulation, oi_declining, strong_reclaim, taker_buy_strong, selling_decelerating, 1h_green_shoot, rsi_recovering_from_extreme |
| 4 | ESPORTSUSDT | LONG | `panic_reversal_long` | `candidate` | 55.6 | 45.0 | 30.0 | `wait_reclaim` | moderate_capitulation, oi_declining, solid_reclaim, taker_buy_strong, 1h_green_shoot, rsi_recovering_from_extreme |
| 5 | SEIUSDT | LONG | `panic_reversal_long` | `candidate` | 55.6 | 45.0 | 45.0 | `wait_reclaim` | moderate_capitulation, oi_declining, early_reclaim, taker_buy_strong, selling_decelerating, 1h_green_shoot, rsi_recovering_from_extreme |
| 6 | PIEVERSEUSDT | LONG | `panic_reversal_long` | `candidate` | 55.4 | 30.0 | 15.0 | `wait_reclaim` | moderate_capitulation, oi_declining, strong_reclaim, selling_decelerating, 1h_green_shoot, rsi_recovering_from_extreme |
| 7 | USELESSUSDT | LONG | `panic_reversal_long` | `candidate` | 55.3 | 30.0 | 15.0 | `wait_reclaim` | moderate_capitulation, oi_declining, strong_reclaim, taker_buy_neutral, selling_decelerating, 1h_green_shoot, rsi_recovering_from_extreme |
| 8 | WUSDT | LONG | `panic_reversal_long` | `candidate` | 55.3 | 30.0 | 15.0 | `wait_reclaim` | moderate_capitulation, oi_declining, early_reclaim, taker_buy_aggressive, selling_decelerating, 1h_green_shoot, rsi_recovering_from_extreme |
| 9 | NILUSDT | LONG | `panic_reversal_long` | `candidate` | 54.7 | 45.0 | 15.0 | `wait_reclaim` | moderate_capitulation, oi_declining, early_reclaim, taker_buy_strong, 1h_green_shoot, rsi_recovering_from_extreme |
| 10 | APTUSDT | LONG | `funding_reversal` | `wait_confirm` | 54.7 | 55.0 | 23.0 | `wait_price_reversal` | elevated_funding, price_bouncing_from_support, oi_rising_short_cover, strong_taker_buy_reversal |
| 11 | AIAUSDT | LONG | `panic_reversal_long` | `candidate` | 53.7 | 30.0 | 60.0 | `wait_reclaim` | heavy_capitulation, oi_flush, early_reclaim, taker_buy_neutral, selling_exhaustion, 1h_green_shoot, rsi_recovering_from_extreme |
| 12 | FILUSDT | LONG | `funding_reversal` | `candidate` | 53.0 | 67.0 | 38.0 | `wait_price_reversal` | high_funding, price_bouncing_from_support, oi_stable, taker_buying_emerging |
| 13 | GRASSUSDT | LONG | `panic_reversal_long` | `candidate` | 52.4 | 30.0 | 15.0 | `wait_reclaim` | moderate_capitulation, oi_declining, strong_reclaim, taker_buy_neutral, selling_decelerating, rsi_recovering_from_extreme |
| 14 | PUMPUSDT | SHORT | `funding_reversal` | `wait_confirm` | 52.4 | 55.0 | 15.0 | `wait_price_reversal` | elevated_funding, extreme_long_crowding, price_flattening, oi_mild_buildup, taker_selling_emerging |
| 15 | EDUUSDT | LONG | `panic_reversal_long` | `candidate` | 51.1 | 30.0 | 23.0 | `wait_reclaim` | moderate_capitulation, oi_declining, solid_reclaim, taker_buy_neutral, selling_decelerating, 1h_green_shoot, rsi_recovering_from_extreme |
| 16 | FORMUSDT | SHORT | `funding_reversal` | `wait_confirm` | 50.8 | 47.0 | 15.0 | `wait_price_reversal` | elevated_funding, extreme_long_crowding, price_turning_down, strong_taker_sell_reversal |
| 17 | XAUTUSDT | SHORT | `funding_reversal` | `wait_confirm` | 50.8 | 47.0 | 15.0 | `wait_price_reversal` | elevated_funding, extreme_long_crowding, price_turning_down, oi_mild_buildup, strong_taker_sell_reversal |
| 18 | BSBUSDT | SHORT | `funding_reversal` | `candidate` | 50.4 | 67.0 | 15.0 | `wait_price_reversal` | elevated_funding, heavy_long_crowding, price_flattening, taker_selling_emerging |
| 19 | PLAYUSDT | LONG | `panic_reversal_long` | `candidate` | 50.1 | 30.0 | 15.0 | `wait_reclaim` | moderate_capitulation, oi_declining, strong_reclaim, selling_decelerating |
| 20 | PORTALUSDT | LONG | `panic_reversal_long` | `candidate` | 49.5 | 30.0 | 45.0 | `wait_reclaim` | heavy_capitulation, oi_declining, strong_reclaim, taker_buy_neutral, rsi_recovering_from_extreme |
| 21 | MONUSDT | SHORT | `funding_reversal` | `candidate` | 49.2 | 60.0 | 15.0 | `wait_price_reversal` | elevated_funding, heavy_long_crowding, oi_mild_buildup, strong_taker_sell_reversal |
| 22 | XMRUSDT | LONG | `funding_reversal` | `wait_confirm` | 49.0 | 47.0 | 23.0 | `wait_price_reversal` | elevated_funding, heavy_short_crowding, price_turning_up, oi_stable, strong_taker_buy_reversal |
| 23 | BANANAS31USDT | SHORT | `funding_reversal` | `wait_confirm` | 45.6 | 35.0 | 23.0 | `wait_price_reversal` | elevated_funding, extreme_long_crowding, price_flattening, oi_mild_buildup, strong_taker_sell_reversal |
| 24 | CHZUSDT | LONG | `funding_reversal` | `wait_confirm` | 45.2 | 42.0 | 38.0 | `wait_price_reversal` | high_funding, price_bouncing_from_support, oi_stable, taker_buying_emerging |

## 4. 机会覆盖

- setup 分布：funding_reversal=10, panic_reversal_long=14
- status 分布：candidate=17, wait_confirm=7
- entry_mode 分布：wait_price_reversal=10, wait_reclaim=14
- 覆盖家族：momentum=false, reversal=true, squeeze=false, range=false, funding=true, accumulation=false, distribution=false

## 5. 问题清单

- 未发现格式或识别阻断问题。
