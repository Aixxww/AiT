# Hunter v7 实时信号 JSON 与 AIT 识别验证报告

> 生成时间：2026-06-18 21:15:29 CST
> 原始 JSON：`reports/hunter-v7-live-validation-raw-20260618-211529.json`
> Prompt 预览：`reports/hunter-v7-live-prompt-20260618-211529.txt`

## 1. 结论

- JSON 结构可序列化/反序列化，核心字段完整，AIT prompt 已包含 `hunter_v7_signal_json`，AI 可以直接识别 v7 标签。
- 本轮实时输出 7 个信号：LONG=6，SHORT=1，setup=4 类。
- 市场 regime=rotation，BTC 24h=-1.05%，ETH 24h=-0.69%。

## 2. JSON / Prompt 校验

| 项目 | 结果 |
|---|---|
| JSON marshal | true |
| JSON unmarshal | true |
| 缺字段数 | 0 |
| 执行性缺口 | 0 |
| Prompt 含 v7 JSON | true |
| Prompt bytes | 20014 |

## 3. 实时信号

| # | Symbol | Dir | Setup | Status | Priority | Timing | Risk | Entry | Reasons |
|---:|---|---|---|---|---:|---:|---:|---|---|
| 1 | XLMUSDT | LONG | `whale_flow_reversal` | `candidate` | 83.7 | 79.2 | 0.0 | `wait_confirm` | whale_flow_detected, stealth_accumulation_breakout, oi_invisible_accumulation_detected, oi_1h_confirming_accumulation, oi_build_without_price_markup, funding_not_crowded, taker_buy_ratio_above_0.55, lsr_balanced_accumulation |
| 2 | VELVETUSDT | LONG | `whale_flow_reversal` | `candidate` | 78.1 | 75.5 | 0.0 | `wait_confirm` | whale_flow_detected, stealth_accumulation_breakout, oi_4h_stealth_build, oi_1h_confirming_accumulation, funding_not_crowded, volume_burst_at_breakout |
| 3 | BANKUSDT | SHORT | `funding_reversal` | `wait_confirm` | 59.6 | 72.0 | 30.0 | `wait_price_reversal` | elevated_funding, extreme_long_crowding, price_flattening, strong_taker_sell_reversal, wait_zone_retest_required |
| 4 | RIFUSDT | LONG | `intraday_scalp_long` | `wait_confirm` | 59.6 | 83.0 | 0.0 | `fast_confirm` | intraday_scalp_entry, strong_5m_velocity, scalp_backend_geometry_context |
| 5 | OPUSDT | LONG | `pre_breakout_watch` | `wait_confirm` | 38.7 | 20.0 | 0.0 | `wait_breakout` | funding_not_crowded, compressed_oi_pre_breakout, near_breakout_trigger, watch_only_no_direct_open |
| 6 | HYPEUSDT | LONG | `pre_breakout_watch` | `wait_confirm` | 38.1 | 20.0 | 0.0 | `wait_breakout` | compressed_oi_pre_breakout, near_breakout_trigger, watch_only_no_direct_open |
| 7 | USELESSUSDT | LONG | `pre_breakout_watch` | `wait_confirm` | 35.9 | 20.0 | 0.0 | `wait_breakout` | compressed_oi_pre_breakout, near_breakout_trigger, watch_only_no_direct_open |

## 4. 机会覆盖

- setup 分布：funding_reversal=1, intraday_scalp_long=1, pre_breakout_watch=3, whale_flow_reversal=2
- status 分布：candidate=2, wait_confirm=5
- entry_mode 分布：fast_confirm=1, wait_breakout=3, wait_confirm=2, wait_price_reversal=1
- tier 分布：EXECUTABLE=1, REJECTED=1, WATCH=5
- 覆盖家族：momentum=false, reversal=false, squeeze=false, range=false, funding=true, accumulation=false, distribution=false

## 5. 问题清单

- 未发现格式或识别阻断问题。
