# Hunter v7 开仓率与盈利率优化续步实施报告 - 2026-08-02

> 实施时间：2026-08-02 CST  
> 验证目录：`reports/hunter-v7-optimization-smoke-20260802`

## 1. 审校结论

上一轮实测暴露的主要问题不是“开仓率不足”，而是盈利统计与执行保护口径不够细：

- `STOP` 混入正收益保护退出，导致胜率/止损率被低估。
- LONG continuation 家族出现 MFE 后回吐，尤其 `alt_ladder_momentum_long` 的 UAIUSDT 先有浮盈后转止损。
- `range_expansion_event` LONG 在 entry zone 高位且 OI 1h 为负时，容易是短回补而不是新多延续，1000SATSUSDT 的 -6.119% 后续 MAE 证明该条件需要降级。
- degraded 轮次仍会输出 all open-rate，容易被误读成正式胜率验收。

## 2. 已实施优化

### 筛选机制

新增 `range_expansion_long_negative_oi_high_zone_wait`：

- 仅作用于 `range_expansion_event` LONG。
- 条件：
  - entry_zone_position > 60%
  - OI 1h < 0
  - 无 `oi_confirms_new_longs` / `oi_building` / `oi_increasing`
- 结果：从 EXECUTABLE/REVIEWABLE 降为 WATCH，等待回踩或新多 OI。

目标是减少 1000SATSUSDT 这类高位事件追多亏损，不压低有真实新多 OI 的开仓。

### 执行保护

新增 `PROTECTED_STOP` outcome 状态：

- LONG：stop >= signal entry 记为 `PROTECTED_STOP`。
- SHORT：stop <= signal entry 记为 `PROTECTED_STOP`。
- `PROTECTED_STOP` 在 setup stats 中计入 Wins，不计入 Loss Stops。

目标是把 ENAUSDT、LINKUSDT、1000PEPEUSDT 这类正收益保护退出从亏损止损中拆出来，修正盈利率统计。

### 保本覆盖

MFE >= 0.60% 后保本保护扩展为双向 continuation：

- SHORT：
  - `alt_ladder_breakdown_short`
  - `breakdown_momentum_short`
  - `relative_weakness_short`
  - `range_expansion_event`
- LONG：
  - `alt_ladder_momentum_long`
  - `displacement_momentum_long`
  - `whale_flow_reversal`
  - `range_expansion_event`

目标是将 UAIUSDT、MMTUSDT 这类“先有可保护浮盈，后转大亏”的路径改为保本/保护退出。

### 策略提示词

Hunter v7 doctrine 增加 range expansion LONG 纪律：

- entry_zone_position > 60% 且 1h OI 为负时，视为短回补而非新多延续。
- 必须等待回踩或新多 OI，不能按事件延续直接开仓。

### 验证报告口径

当 `valid_rounds=0` 时，run summary 自动输出：

`INVALID_SAMPLE_DO_NOT_USE_FOR_WINRATE`

目标是避免 degraded 三轮的 all open-rate 被误读为正式胜率/开仓率验收。

## 3. 测试验证

已通过：

- `go test ./trader -run 'TestSignalOutcomeTracker'`
- `go test ./kernel -run 'TestClassifyHunterV7CandidateTierAllowsConfirmedRangeExpansionDespiteChaseProtection|TestClassifyHunterV7CandidateTierBlocksRangeExpansionLongNegativeOIHighZone|TestClassifyHunterV7CandidateTier'`
- `go test ./cmd/hunter_v7_validate -run 'TestFormatRunSummaryMarksAllDegradedSampleInvalidForWinRate|TestValidatePromptParsesFinalTierSummary'`
- `go test ./kernel ./trader ./cmd/hunter_v7_validate`
- `git diff --check`

新增覆盖：

- range expansion LONG 高位负 OI 降级为 WATCH。
- LONG continuation MFE 后保本。
- SHORT continuation MFE 后保本状态改为 `PROTECTED_STOP`。
- validator 全 degraded 样本输出 invalid win-rate 标记。

## 4. Binance Smoke

命令：

```bash
HTTP_PROXY=http://127.0.0.1:7897 HTTPS_PROXY=http://127.0.0.1:7897 ALL_PROXY=http://127.0.0.1:7897 NO_PROXY=localhost,127.0.0.1,::1 TZ=Asia/Shanghai \
go run ./cmd/hunter_v7_validate \
  -rounds=1 \
  -top-detail=220 \
  -max-workers=8 \
  -max-output=30 \
  -watch-output=8 \
  -min-priority=45 \
  -aggressive=true \
  -post-track-duration=5m \
  -post-track-interval=60s \
  -out-dir=reports/hunter-v7-optimization-smoke-20260802
```

结果：

- Signals: 4
- Open-review: 1
- all open-review rate: 25.0%
- valid_rounds: 0
- REST error rate: 35.9%
- universe coverage: 18.1%
- run summary 已输出 `INVALID_SAMPLE_DO_NOT_USE_FOR_WINRATE`

Outcome：

- tracked: 2
- active: 1
- STOP: 1，PnL -2.012%
- ACTIVE: 1，PnL +0.213%

本轮仍为 degraded，只证明链路可运行和报告口径生效，不能用于正式胜率验收。

## 5. 后续验收目标

## 6. 两轮实时验证复核

> 验证目录：`reports/hunter-v7-optimization-2round-20260802`  
> 参数：2 轮，轮间 5 分钟，15 分钟 post-track。

Run summary：

| Round | Regime | Signals | Open-review | Open-rate | REST errors | Universe | Degraded |
|---:|---|---:|---:|---:|---:|---:|---|
| 1 | compression | 7 | 2 | 28.6% | 0 | 99 | true |
| 2 | compression | 8 | 1 | 12.5% | 0 | 99 | true |
| 合计 | - | 15 | 3 | 20.0% | 0 | - | true |

限制：

- valid_rounds = 0。
- 两轮 REST errors = 0，但 universe coverage 仍为 18.9%，低于 30% 阈值。
- run summary 已正确输出 `INVALID_SAMPLE_DO_NOT_USE_FOR_WINRATE`。

Outcome：

| Status | Count | Profit | Loss | Flat | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|---:|
| ACTIVE | 1 | 1 | 0 | 0 | +0.620 | +1.340 | -0.550 |
| PROTECTED_STOP | 1 | 0 | 0 | 1 | 0.000 | +1.358 | -0.368 |
| WIN_TP0 | 1 | 1 | 0 | 0 | +1.200 | +1.242 | -0.004 |

复核结论：

- `PROTECTED_STOP` 已在真实 post-track 中出现，说明 MFE 后保本保护和状态拆分生效。
- 本轮 completed loss stop = 0，上一轮“正收益保护退出被记为 STOP”的产品与统计问题已修正。
- all open-review rate 20.0%，说明优化后没有继续压死开仓；但因 valid_rounds=0，不能作为正式开仓率验收。
- 本轮没有输出 `range_expansion_event` LONG，因此高位负 OI 降级规则尚未在实时样本中命中；该规则已有单测覆盖。

## 7. 产品设计规范化实施

前端 Hunter v7 信号面板同步做了产品级展示规范：

- 新增 outcome badge，显示 `PROTECTED_STOP`、WIN/STOP/ACTIVE 与 PnL。
- tier 仍保持质量分层色彩，不与盈亏红绿混用。
- `PROTECTED_STOP` 使用中性保护态视觉，与 `EXECUTABLE/REVIEWABLE/WATCH` 的 tier 体系分离。
- WATCH 行与 actionable 卡片都显示 outcome，便于用户快速识别保护退出与真实亏损。
- 中/英/印尼语文案同步补充，避免界面出现缺失 key。

## 8. 后续验收目标

在有效数据窗口重新跑 3 轮 + 30m post-track：

- valid_rounds >= 2/3
- valid open-review rate 8%-15%
- `PROTECTED_STOP` 独立统计，不再混入 loss stop
- range_expansion_event LONG 高位负 OI 样本不进入 EXECUTABLE
- MFE >= 0.60% 后转亏损 STOP 数量为 0
