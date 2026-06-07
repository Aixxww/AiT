# Hunter v7 Funding Reversal 风控修复报告

日期：2026-06-07
关联周期：#142 修复，#146 复盘
范围：Hunter v7 `funding_reversal` 筛选层、AI prompt、执行层风控

## 事故要点

周期 #142 中，筛选层给出 `funding_reversal SHORT` 候选，但当时信号仍存在几个不应开仓的特征：

- 价格已深跌后继续追空，靠近 entry zone 下沿。
- OI 仍在 building，没有出现 funding reversal 做空更需要的 OI flush 或 failed rebuild。
- prompt 只输出 `warning=short_near_zone_lower`、`oi_state=building`，没有升级为硬性拦截。
- AI 把 `position_size_usd` 当成 margin 后又乘了一次 leverage，导致名义仓位风险理解错误。
- 止损距执行价约 1.5%，HUSDT 在 36 秒反弹中触发止损，属于短线噪声可扫损距离。

## 已落地修复

### 1. 筛选层：降低/剔除 late chase

文件：

- `provider/local/hunter_v7_mod_funding_reversal.go`
- `provider/local/hunter_v7_types.go`
- `provider/local/hunter_v7_router.go`

修复：

- `funding_reversal SHORT` 在深跌后、OI 仍增加时直接过滤或降分。
- 对称增加 `funding_reversal LONG` 在深涨后、OI 仍增加时的 late chase 保护。
- OI building 不再作为 funding reversal 加分项，改为扣分并输出 `oi_building_no_flush` 风险标签。
- Router 支持 per-setup threshold，`funding_reversal` 默认要求更严格的区间位置和 OI flush 约束。

### 2. Prompt 层：warning 升级为 hard rule

文件：

- `kernel/engine_prompt.go`
- `kernel/engine.go`
- `kernel/engine_prompt_compact_test.go`

修复：

- compact prompt 输出 Hunter v7 execution compact，上下文包含：
  - `entry_zone_pos`
  - `zone_location`
  - `invalidation_dist`
  - `target1_dist`
  - `oi_state`
  - 15m/5m 近期高低点、ATR、EMA20、VWAP
- 对 `funding_reversal` 输出明确硬规则：
  - OI building 时：`no_open_short_until_oi_flush_or_failed_rebuild`
  - SHORT 不在区间上沿/retest：`no_short_below_zone_upper_wait_retest_rejection`
  - 深跌后无 OI flush：`no_chase_short_after_fast_drop_without_oi_flush`
  - 末尾附加 `output_wait_only`
- 系统 prompt 明确：
  - `position_size_usd` 是订单名义价值，不是保证金。
  - 风险、盈亏和仓位价值检查不能再次乘 leverage。

### 3. 执行层：硬拦截和最小止损距离

文件：

- `trader/auto_trader_loop.go`
- `trader/auto_trader_orders.go`
- `trader/auto_trader_risk.go`
- `trader/auto_trader_risk_test.go`
- `store/strategy.go`

修复：

- 执行决策前先跑 Hunter v7 guard。
- `funding_reversal` 默认 guard：
  - SHORT 需要 `entry_zone_pos >= 65%`
  - LONG 需要 `entry_zone_pos <= 35%`
  - 需要 OI flush 或 failed rebuild，拒绝 OI building。
- Hunter v7 默认最小止损距离为 2%，避免 1 根噪声 K 线扫损。
- `MinStopLossPriceMovePct` 默认值保持 0，仅 Hunter v7 在未显式配置时自动使用 2%，避免误伤普通策略。
- 增加单笔止损亏损上限和 TP1 距离封顶，减少 AI 给出远 TP、窄 SL 形成虚高 RR 的情况。

## 验证结果

已通过：

```bash
go test ./provider/local ./kernel ./trader
go test ./...
```

覆盖用例包括：

- late short + building OI 在筛选层被过滤。
- prompt compact 输出 `oi_state`、zone 位置、VWAP/ATR 和 warning。
- Hunter v7 执行层拒绝 OI building 的 `funding_reversal SHORT`。
- Hunter v7 执行层拒绝靠近区间下沿的 `funding_reversal SHORT`。
- Hunter v7 执行层拒绝止损距离过近的开仓。

## 重启状态

已重启 `ait-dev` tmux 会话：

- 后端：`go run main.go`
- 前端：`npm run dev`
- Web Dashboard：http://localhost:3000
- API：http://localhost:8080

非沙箱健康检查结果：

```text
GET /api/health -> {"status":"ok","time":null}
GET / on :3000 -> HTTP 200
```

## 后续建议

- 对 funding reversal 增加实际成交后的复盘字段：开仓时 `oi_state`、`entry_zone_pos`、`stop_loss_dist_pct`、`hard_rule_hit`。
- 把 C 级 `funding_reversal` 的开仓默认动作固定为 `wait`，只有 B 级以上且 OI flush/failed rebuild 成立时才允许模型输出开仓。
- 周期复盘中单独统计 `hard_rule` 命中次数，确认是否明显减少 late chase 开仓。
