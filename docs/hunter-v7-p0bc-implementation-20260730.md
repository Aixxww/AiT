# Hunter v7 P0-B/P0-C 实施记录

生成时间：2026-07-30

## 1. 本轮已落地

### P0-C validate 持续 outcome 跟踪

落点：`cmd/hunter_v7_validate/main.go`

- 新增参数：
  - `--post-track-duration`
  - `--post-track-interval`
  - `--track-active-only`
- 多轮验证结束后可继续刷新 ACTIVE outcome，不再只依赖每轮末尾的一次 tick。
- 为 validate tracker 增加最新 1m candle source，用最新 candle close/high/low 继续裁决 TP/SL。
- 每次验证结束输出 outcome JSON 与 Markdown：
  - `hunter-v7-validation-outcomes-<timestamp>.json`
  - `hunter-v7-validation-outcomes-<timestamp>.md`
- 汇总维度包含状态分布、setup 聚合、ACTIVE/TP/STOP/TIMEOUT、Avg PnL/MFE/MAE。

### P0-B alt_ladder_breakdown_short REVIEWABLE 收紧

落点：

- `kernel/hunter_v7_tier_rules.go`
- `kernel/engine.go`
- `kernel/engine_prompt_compact_test.go`

已实施规则：

- `alt_ladder_breakdown_short` 的 REVIEWABLE 规则增加 `hunterV7AltLadderShortReviewableOK` guard。
- 非 late 分支必须具备：
  - `alt_ladder_taker_sell`
  - `alt_ladder_new_shorts` / `alt_ladder_long_flush` / `alt_ladder_sell_volume` 至少一项
  - 确认 taker_buy_15m <= 0.46
- 已有跨轮 close-through 的信号可用 taker_buy_15m <= 0.48 放行，但仍需 OI/量能流向腿。
- `alt_ladder_downshift_late` 或 `alt_ladder_late_short_risk` 分支必须同时具备：
  - `trigger_memory_confirmed` 或 `alt_ladder_multi_cycle_close_through`
  - `alt_ladder_taker_sell`
  - OI/量能流向腿
  - 确认 taker_buy_15m <= 0.46
  - 无 `alt_ladder_late_short_risk`

回归测试覆盖：

- late short taker=0.47 且无 close-through，不再得到 `alt_ladder_short_reviewable_confirmed`。
- late short 有 `trigger_memory_confirmed` + `alt_ladder_multi_cycle_close_through` 且 taker<=0.46，可 REVIEWABLE。
- early short 具备强卖流时仍保留 REVIEWABLE，避免误伤有效召回。

## 2. 本轮未直接改动

- P0-A open-rate 统计口径统一：涉及 DB/API/matrix 字段定义，建议单独实施和迁移验收。
- P1-A 真实持仓 TP0/保本提前保护：`trader/auto_trader_risk.go` 已有 TP0/动态止损，但报告建议的“TP0 距离 60% 或 0.8R 即推进保护”会改变真实交易减仓/止损行为，需单独回测或灰度。
- 同 symbol/setup/direction 30 分钟去重注册 outcome：本轮未改 tracker 注册策略，避免影响历史 outcome 统计。
- blocked gate 新码 `alt_ladder_short_retest_pending` / `alt_ladder_short_reclaim_risk` / `duplicate_signal_context_only`：本轮先用 tier guard 收紧，不做 schema/矩阵展示扩展。

## 3. 验收

已执行：

```bash
go test -count=1 ./kernel
go test -count=1 ./cmd/hunter_v7_validate ./provider/local ./trader ./kernel
git diff --check
```

结果均通过。

## 4. 下一步建议

1. 用 `--post-track-duration=30m --post-track-interval=30s` 跑 6 轮实时验证，重点观察 `alt_ladder_breakdown_short` 的 REVIEWABLE 数量和 STOP 率是否下降。
2. 单独实施 P0-A，把 prompt-output open-rate 与 raw setup funnel 分栏，防止 DB raw REVIEWABLE 污染主开仓率。
3. 在回测或小仓灰度后再实施 P1-A，把真实持仓保护推进到 TP0 60%/0.8R。
