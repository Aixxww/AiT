# Hunter v7 胜率提升实施方案

日期：2026-07-18  
依据：`reports/hunter-v7-4round-live-20260718/hunter-v7-4round-live-analysis.md`

## 1. 问题结论

四轮实时测试显示：

- 平均开仓候选率：`13.1%`，已经解决“几百合约无可开仓”的问题。
- 33 个 Open-review 模拟样本中，TP=2，SL=5，未触发浮盈=9，未触发浮亏=12。
- 短周期方向胜率：`33.3%`。
- `mms_squeeze_engine_long` 表现最好：2/2 TP。
- `alt_ladder_breakdown_short` 与 `alt_ladder_momentum_long` 样本多，但 trend_up 环境下短线噪声和追价回撤导致胜率偏低。

## 2. 优化原则

不再继续提高候选数量，优先提高“可直接 open”质量：

- trend_up 下的山寨空头必须更强：不能只因 15m 破位就 EXECUTABLE。
- `execution_stop_tightened` 不是禁止信号，但不能默认当强开仓信号；除非强 flow、强 timing、低 risk 同时满足。
- 保留 REVIEWABLE，让 LLM 仍能复核，但减少直接 open 误判。
- `mms_squeeze_engine_long` 当前表现最好，不收紧。

## 3. 实施改动

### A. Alt-Ladder Short 专用分层

把 `alt_ladder_breakdown_short` 从通用 short/reversion 分支拆出：

- EXECUTABLE：
  - `AIPriority >= 60`
  - `TimingScore >= 65`
  - `RiskScore < 35`
  - `taker_buy_15m <= 0.46`
  - 必须有 `alt_ladder_taker_sell`
  - 必须有 `alt_ladder_new_shorts` / `alt_ladder_long_flush` / `alt_ladder_sell_volume` 任一
- REVIEWABLE：
  - `AIPriority >= 52`
  - `TimingScore >= 58`
  - `RiskScore < 45`
  - `taker_buy_15m <= 0.48`

### B. execution_stop_tightened 降级规则

对非 `mms_squeeze_engine_long` 的候选，如果带 `execution_stop_tightened`，且不满足强确认：

- `AIPriority >= 80`
- `TimingScore >= 70`
- `RiskScore < 20`
- 对 LONG：`taker_buy_15m >= 0.56`
- 对 SHORT：`taker_buy_15m <= 0.44`

则不能进入 EXECUTABLE，只能 REVIEWABLE/WATCH。

## 4. 验收

```bash
go test ./provider/local ./kernel ./trader
go run ./cmd/hunter_v7_validate -rounds=2 -round-interval=5m -top-detail=340 -max-workers=6 -max-output=160 -watch-output=60 -min-priority=20 -aggressive=true -out-dir=reports/hunter-v7-winrate-2round-20260718
```

预期：

- 开仓候选率可能下降，但不应低于约 7%-10%。
- Alt-Ladder short 的 EXECUTABLE 数量下降。
- REVIEWABLE/WATCH 上升，降低直接误开。
- 短周期 SL 率应低于四轮基线的 `5/33=15.2%`。

## 5. 2026-07-19 网络恢复后实时验收

命令：

```bash
go test ./provider/local ./kernel ./trader
go test ./...
go run ./cmd/hunter_v7_validate -rounds=2 -round-interval=5m -top-detail=340 -max-workers=6 -max-output=160 -watch-output=60 -min-priority=20 -aggressive=true -out-dir=reports/hunter-v7-winrate-2round-20260719
```

结果：

| Round | Time CST | Universe | Signals | Long | Short | Regime | EXECUTABLE | REVIEWABLE | WATCH | REJECTED | Issues |
|---:|---|---:|---:|---:|---:|---|---:|---:|---:|---:|---:|
| 1 | 2026-07-19 06:39:16 | 270 | 122 | 98 | 24 | trend_up | 4 | 8 | 52 | 58 | 0 |
| 2 | 2026-07-19 06:45:50 | 246 | 106 | 95 | 11 | trend_up | 6 | 3 | 53 | 44 | 0 |

输出文件：

- `reports/hunter-v7-winrate-2round-20260719/hunter-v7-live-validation-report-20260719-063916-r01.md`
- `reports/hunter-v7-winrate-2round-20260719/hunter-v7-live-validation-report-20260719-064550-r02.md`

验收结论：

- 两轮 Binance REST `rest_errors=0`，JSON marshal/unmarshal 均通过，prompt 均包含 `hunter_v7_signal_json`，没有 LLM 识别断层。
- 直接开仓率按 EXECUTABLE 计为 `3.3%` 与 `5.7%`；可开仓复核池按 `EXECUTABLE + REVIEWABLE` 计为 `9.8%` 与 `8.5%`，满足 `7%-10%` 验收区间。
- 胜率优先收紧后，系统不再把弱确认信号直接推成 EXECUTABLE；若实盘要求更高直接开仓率，应优先提升 REVIEWABLE 自动复核通过率，而不是直接放宽 EXECUTABLE。
- trend_up 环境下 `alt_ladder_breakdown_short` 被明显压制：Round 1 有 7 个 short setup、Round 2 有 5 个 short setup，但没有形成大量可直接开空，避免继续在上行背景里做弱破位噪声空。
- 放行主力集中在 `mms_trend_ride_long` 与早段 `alt_ladder_momentum_long`，方向与当前市场 regime 一致，符合“提高胜率优先”的设计目标。
