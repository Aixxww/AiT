# Hunter v7 实时信号 JSON 与 AIT 识别验证报告

> 生成时间：2026-06-20 11:02:25 CST
> 原始 JSON：`reports/hunter-v7-live-validation-raw-20260620-110225.json`
> Prompt 预览：`reports/hunter-v7-live-prompt-20260620-110225.txt`

## 1. 结论

- JSON 结构可序列化/反序列化，核心字段完整，AIT prompt 已包含 `hunter_v7_signal_json`，AI 可以直接识别 v7 标签。
- 本轮实时输出 9 个信号：LONG=6，SHORT=3，setup=5 类。
- 市场 regime=rotation，BTC 24h=+0.49%，ETH 24h=-0.15%。

## 2. JSON / Prompt 校验

| 项目 | 结果 |
|---|---|
| JSON marshal | true |
| JSON unmarshal | true |
| 缺字段数 | 0 |
| 执行性缺口 | 0 |
| Prompt 含 v7 JSON | true |
| Prompt bytes | 19780 |

## 3. 实时信号

| # | Symbol | Dir | Setup | Status | Priority | Timing | Risk | Entry | Reasons |
|---:|---|---|---|---|---:|---:|---:|---|---|
| 1 | EIGENUSDT | LONG | `whale_flow_reversal` | `candidate` | 85.9 | 76.1 | 0.0 | `wait_confirm` | whale_flow_detected, stealth_accumulation_breakout, oi_1h_confirming_accumulation, funding_not_crowded |
| 2 | EDGEUSDT | LONG | `leader_momentum_long` | `candidate` | 72.4 | 64.0 | 8.0 | `momentum_with_trailing_stop` | solid_24h_momentum, solid_4h_momentum, shallow_pullback_1h, oi_healthy_growth, taker_neutral_buy, micro_pullback, chase_high_protection |
| 3 | AXSUSDT | LONG | `leader_momentum_long` | `wait_confirm` | 68.6 | 90.0 | 30.0 | `momentum_with_trailing_stop` | strong_24h_momentum, strong_4h_momentum, holding_1h, oi_healthy_growth, taker_sustained_buy, no_pullback_still_running, sector_rotation_leader, sector_theme_gaming, momentum_extreme_funding_wait, momentum_rsi_overheated_wait |
| 4 | COLLECTUSDT | SHORT | `funding_reversal` | `wait_confirm` | 66.5 | 80.0 | 23.0 | `wait_price_reversal` | elevated_funding, long_crowding, price_turning_down, oi_declining_long_flush, strong_taker_sell_reversal, wait_zone_retest_required, funding_short_weak_4h_flush_wait |
| 5 | MANTAUSDT | SHORT | `funding_reversal` | `wait_confirm` | 62.0 | 72.0 | 30.0 | `wait_price_reversal` | elevated_funding, extreme_long_crowding, price_turning_down, strong_taker_sell_reversal, wait_zone_retest_required |
| 6 | ATUSDT | SHORT | `funding_reversal` | `wait_confirm` | 60.6 | 72.0 | 30.0 | `wait_price_reversal` | elevated_funding, extreme_long_crowding, price_flattening, strong_taker_sell_reversal, wait_zone_retest_required, funding_short_weak_4h_flush_wait |
| 7 | ESPORTSUSDT | LONG | `panic_reversal_long` | `candidate` | 60.3 | 55.0 | 45.0 | `wait_reclaim` | deep_capitulation, oi_declining, taker_buy_neutral, selling_decelerating, 1h_green_shoot, rsi_recovering_from_extreme |
| 8 | VELODROMEUSDT | LONG | `intraday_scalp_long` | `wait_confirm` | 58.0 | 90.6 | 15.0 | `fast_confirm` | intraday_scalp_entry, strong_5m_velocity, scalp_backend_geometry_context |
| 9 | METUSDT | LONG | `leader_momentum_long` | `wait_confirm` | 55.0 | 49.0 | 0.0 | `momentum_with_trailing_stop` | solid_24h_momentum, strong_4h_momentum, shallow_pullback_1h, oi_healthy_growth, taker_weak_buy, micro_pullback, chase_high_protection, leader_momentum_timing_watch_only |

## 4. 机会覆盖

- setup 分布：funding_reversal=3, intraday_scalp_long=1, leader_momentum_long=3, panic_reversal_long=1, whale_flow_reversal=1
- status 分布：candidate=3, wait_confirm=6
- entry_mode 分布：fast_confirm=1, momentum_with_trailing_stop=3, wait_confirm=1, wait_price_reversal=3, wait_reclaim=1
- runtime tier 分布（后端初筛）：EXECUTABLE=1, REJECTED=2, WATCH=6
- prompt-final tier 分布（AIT 最终可执行口径）：EXECUTABLE=0, REJECTED=2, REVIEWABLE=1, WATCH=6
- 覆盖家族：momentum=true, reversal=true, squeeze=false, range=false, funding=true, accumulation=false, distribution=false

## 5. 问题清单

- 未发现格式或识别阻断问题。
