# Hunter v7 执行前风控与平仓归因更新

> 更新时间：2026-07-04  
> 关联复盘：`reports/kkk-open-rate-profitability-optimization-20260703.md`

## 背景

KKK 实盘复盘显示，`range_expansion_event SHORT` 在高波动事件中容易出现 late short、技术反弹和方向确认语义冲突。典型问题包括：

- AI 输出 `open_short`，但 reasoning 把 `15m close above VWAP/EMA20` 当作空头确认。
- 交易引擎修复 price / SL / TP / RR 后，缺少最后一次基于实时价格的方向失效检查。
- 开仓和平仓都发生在第一次 order sync 前，本地保护器触发的 `hard_loss_close` 无法落到 DB，最终 close_reason 被同步链路写成 `sync`。

本次更新不放宽 hard loss，也不直接提高开仓频率，而是减少低质量 EXECUTABLE 进入实盘，并让平仓归因可追溯。

## 执行前 live guard

交易引擎在执行 Hunter v7 open 决策前会再次读取实时执行价，并重新执行：

1. TP1 cap。
2. price / stop_loss / take_profit / RR 修复。
3. 基础开仓几何校验。
4. Hunter v7 live guard。

`range_expansion_event SHORT` 新增硬拦截条件：

| 条件 | 处理 |
|---|---|
| live price 比 AI 决策价反弹 `>= 0.30%` | reject open，返回 `rebound_risk_wait` |
| 15m close 低于 EMA20 `>= 10%` | reject open，避免极端 late short |
| entry zone position `> 80%` | reject open，避免追 zone 尾部 |
| `taker_buy_15m >= 0.48` | reject open，视为空头延续确认不足 |
| SHORT reasoning 引用 `close above VWAP/EMA20` 作为确认 | reject open，返回 `direction_confirmation_conflict` |

实现入口：

- `trader/auto_trader_orders.go`
  - `executeDecisionWithRecord(ctx, ...)`
  - `executeOpenLongWithRecord(ctx, ...)`
  - `executeOpenShortWithRecord(ctx, ...)`
- `trader/auto_trader_risk.go`
  - `refreshHunterV7OpenPreflight`
  - `validateHunterV7LiveOpenGuard`
  - `validateHunterV7DecisionReasoningDirection`

## Prompt 约束

`kernel/engine_prompt.go` 已同步后端硬规则，要求策略 LLM：

- 不得把 `15m close above VWAP/EMA20`、`close above VWAP`、`close above EMA20` 解释为 SHORT 确认通过。
- `range_expansion_event SHORT` 若 15m close 极端低于 EMA20、entry_zone_position 过高、实时价反弹，必须 wait。
- wait 决策必须输出结构化 `blocked_reason_code`。

这能降低 AI 与交易引擎规则不一致导致的无效开仓或无效拦截。

## 平仓归因修复

新增表：`position_close_intents`

用途：当保护器触发平仓，但 DB 里尚未同步出 OPEN position 时，先记录一个 pending close intent。后续 order sync 通过 `PositionBuilder` 创建并关闭仓位时，会把 pending intent 应用到最终 `trader_positions.close_reason/source`。

关键行为：

- 匹配键：`trader_id + symbol + side`
- 默认 source：`system_protector`
- pending intent 有效期：15 分钟
- 过期 intent 不会污染未来同币同向新仓

实现入口：

- `store/position.go`
  - `PositionCloseIntent`
  - `RecordPendingCloseIntent`
  - `ApplyPendingCloseIntent`
- `store/position_builder.go`
  - full close 前应用 pending intent
- `trader/auto_trader_risk.go`
  - `markPendingProtectedClose`

## 测试覆盖

新增/更新测试：

- `TestHunterV7LiveOpenGuardRejectsShortReasoningDirectionConflict`
- `TestHunterV7LiveOpenGuardRejectsRangeExpansionShortReboundFromDecisionPrice`
- `TestHunterV7LiveOpenGuardRejectsRangeExpansionShortDeepBelowEMA`
- `TestPositionBuilderAppliesPendingProtectedCloseIntent`
- `TestPositionBuilderIgnoresExpiredPendingProtectedCloseIntent`

验证命令：

```bash
go test ./store
go test ./trader
go test ./kernel
go test ./...
```

## 观察指标

后续实盘建议重点观察：

- `direction_confirmation_conflict` 是否继续出现。
- `rebound_risk_wait` 是否拦截 late short。
- `range_expansion_event SHORT` 是否仍在 15m 极端低于 EMA20 后成交。
- 快速保护平仓是否正确记录为 `hard_loss_close/system_protector`。
- 扣费后净 PnL 是否改善，而不是只看开仓率或毛 PnL。
