# Hunter v7 Outcome Denominator and Dedupe Implementation - 2026-08-01

## 背景

2026-08-01 三轮币安实时跟踪显示，长期 outcome tracker 把部分没有最终 DB `EXECUTABLE/REVIEWABLE` tier 的信号纳入了真实结案分母，同时同一 `symbol/setup/direction` 在连续轮次重复出现时会被当作多笔独立交易统计。这会同时扭曲开仓率和胜率：

- `TLMUSDT alt_ladder_momentum_long`、`TAKEUSDT range_expansion_event` 只有 prompt/readiness 层可跟踪特征，但无最终 DB tier，不能计入真实交易 outcome。
- `GRVTUSDT alt_ladder_breakdown_short` 跨 3 轮重复出现，本质是同一 short thesis，应保留最新上下文，旧轮次只作为 duplicate audit。

## 已落地

1. 真实 outcome tracking 只使用最终 DB tier。
   - `HunterV7ShouldTrackSignal` 只接受 `rec.Tier == EXECUTABLE/REVIEWABLE`。
   - 移除 fallback 到 `Signal.ExecutionReadiness.Tier` 的逻辑，避免 WATCH/no-tier 污染主分母。

2. 30 分钟同 thesis 去重。
   - thesis key: `symbol + setup_type + direction`。
   - 30 分钟内新信号替换旧 active tracking，旧记录写入 `DUPLICATE_CONTEXT`。
   - 更早的重复信号会被忽略，不覆盖当前 active thesis。
   - 超过 30 分钟的同 thesis 可并行跟踪；旧信号结案时不会删除最新 thesis 索引。

3. DB 聚合排除 duplicate audit。
   - `OutcomeWindowStats` 与 `SetupRegimeOutcomeStats` 只统计 `WIN_TP0/WIN_TP1/WIN_TP2/STOP/TIMEOUT`。
   - `DUPLICATE_CONTEXT` 保留在 DB 审计层，不进入胜率、均值 PnL、setup/regime 聚合。

4. `alt_ladder_breakdown_short` 跨轮升级增加反抽失败确认。
   - 跨轮 trigger memory 升级必须同时满足：
     - `5m_or_15m_close_below_trigger`
     - `no_new_high_after_rejection`
   - 升级后的信号会把 `no_new_high_after_rejection` 写入 `RequiredConfirms`，提示词和 DB 原始快照可见。

## 预期影响

- 主开仓率分母会收窄到真正可执行/可复核的 DB 信号，WATCH/no-tier 只做机会池审计。
- 胜率不再被重复轮次放大或稀释，连续 3 轮同 thesis 只保留最新 active context。
- `alt_ladder_breakdown_short` 的 REVIEWABLE/OPEN_NOW 升级更保守，预计减少反抽造成的 active 浮亏和保护止损触发。

## 验证

已通过：

```bash
go test -count=1 ./provider/local ./trader ./kernel ./store ./cmd/hunter_v7_validate
```

新增/更新测试覆盖：

- final DB tier 才进入 outcome tracking。
- 30 分钟内同 thesis 新信号替换旧信号。
- 30 分钟内旧重复信号不覆盖当前 active 信号。
- 超过 30 分钟并行跟踪时，旧信号结案不清除最新 thesis 索引。
- `DUPLICATE_CONTEXT` 不进入 DB outcome 聚合。
- `alt_ladder_breakdown_short` 缺少 `no_new_high_after_rejection` 时不跨轮升级。
