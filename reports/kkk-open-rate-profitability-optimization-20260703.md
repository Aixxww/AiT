# KKK 开仓率、快速平仓与盈利能力优化报告

时间：2026-07-03  
项目：`/Users/aixx/Code/AiT`  
交易员：KKK  
交易员 ID：`d7b3b284_445bf32d-9add-4960-a925-29539fa66e69_deepseek_1783021529`  
策略：`hhh-hunter-v7-conservative-20260703`  
当前状态：`is_running=0`  
数据来源：`data/data.db`、`.logs/backend.log`、`decision_records`、`trader_positions`

## 1. 总结结论

KKK 最近的快速平仓分为两类：

1. `DEEPUSDT`、`AIGENSYNUSDT` 是小仓位高杠杆下动态保护止损过早推到接近成本价，手续费后转亏。该问题此前已通过“开仓早期延迟动态止损”和“手续费后利润地板”修复。
2. `TACUSDT`、`LABUSDT`、`BLESSUSDT` 是更严重的方向质量问题：`range_expansion_event SHORT` 在高波动事件里追空放行过宽，AI reasoning 与后端方向确认存在不一致，交易引擎修复 price/SL/TP/RR 后缺少最后一次 live price 反弹风险否决。

本轮主因不是模型连接失败，也不是 TP 过远。DeepSeek 三次请求均成功返回并解析；TP 已被后端 cap。`hard_loss_close` 是亏损后的保护动作，不是亏损根因。真正需要优化的是开仓前筛选、提示词方向约束和交易引擎下单前二次确认。

## 2. 关键交易证据

| 标的 | 周期 | 方向 | 开仓价 | 平仓价 | 持仓 | 原始反向波动 | 杠杆 | 毛 PnL | 手续费 | 扣费后估算 | DB close_reason |
| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| BLESSUSDT | 63 | SHORT | 0.008513 | 0.008630 | 0.58m | -1.3744% | 20x | -0.197145 | 0.014443 | -0.211588 | hard_loss_close |
| TACUSDT | 70 | SHORT | 0.034231 | 0.034667 | 2.35m | -1.2737% | 10x | -0.168732 | 0.013332 | -0.182064 | hard_loss_close |
| LABUSDT | 72 | SHORT | 7.198 | 7.295 | 0.55m | -1.3476% | 10x | -0.194000 | 0.014493 | -0.208493 | sync |
| OUSDT | 78 | SHORT | 0.5382 | 0.5433 | 1.84m | -0.9476% | 10x | -0.132600 | 0.014060 | -0.146660 | hard_loss_close |

模型调用证据：

| 周期 | 标的 | 动作 | AI 耗时 | tokens | 结果 |
| ---: | --- | --- | ---: | ---: | --- |
| 63 | BLESSUSDT | open_short | 7.05s | 17842 | 成功解析并执行 |
| 70 | TACUSDT | open_short | 6.43s | 14653 | 成功解析并执行 |
| 72 | LABUSDT | open_short | 6.03s | 14741 | 成功解析并执行 |
| 78 | OUSDT | open_short | 9.33s | 15142 | 成功解析并执行 |

## 3. 根因分析

### 3.1 筛选层：range_expansion_event SHORT 放行过宽

`range_expansion_event` 可以捕捉事件延续，但 TAC/LAB/BLESS/OUS 都是高波动环境下的事件追空或快速延续空。该类信号只满足 RR 和几何条件还不够，必须额外防止：

- 极端下跌后的 late short；
- 15m close 远低于 EMA20 后的技术反弹；
- 同一标的短时间 LONG/SHORT 快速翻向；
- trend_up 或 regime_against_direction 背景下的事件追空；
- taker flow 从空头确认转为中性或反弹。

### 3.2 提示词层：方向语义没有硬约束

BLESS 的 AI reasoning 出现 SHORT 决策却引用 `15m close above VWAP/EMA20 true` 作为确认的语义冲突。提示词此前强调 EXECUTABLE 优先、RR 通过优先开仓，但没有把“SHORT 不能用 above VWAP/EMA20 当确认”写成硬规则。

### 3.3 交易引擎：修复几何后缺少 live 二次否决

交易引擎已能修复：

- 入场价漂移；
- 止损距离；
- TP cap；
- RR；
- 单笔亏损上限缩仓。

不足是修复后没有再次按 live price 判断方向是否已失效。LAB 就是典型：AI 决策价 `7.146`，执行有效入场约 `7.243`，对 SHORT 来说虽然是“更高价开空”的几何优势，但在高波动事件里也可能代表反弹已经启动。

### 3.4 平仓保护：hard_loss_close 正常工作

三笔仓位在开仓后 0.55-2.35 分钟平仓，是因为 10x/20x 杠杆下，约 1.27%-1.37% 的原始反向波动已经放大到 -12% 至 -27% ROE。放宽 hard loss 只会扩大亏损，不应作为优化方向。

### 3.5 记录同步：LAB close_reason 保真缺陷

LAB 日志显示本地保护触发 `hard_loss_close`，但 DB 记录为 `sync`。原因是开仓、保护平仓、order sync 建仓/关仓发生得太快，本地保护触发时 DB 里还没有 OPEN position，导致 close intent 无处保存。

## 4. 已实施优化

### 4.1 提示词增加 range_expansion_event SHORT 防反弹规则

文件：`kernel/engine_prompt.go`

新增规则：

- `range_expansion_event SHORT` 若 15m close 低于 EMA20 超过 10%，必须 wait。
- `entry_zone_position >80%` 必须 wait。
- 实时价相对 AI 决策价反弹超过 0.30%，必须 wait。
- `open_short` 不得把 `15m close above VWAP/EMA20`、`close above VWAP` 或 `close above EMA20` 当作 SHORT 确认。

### 4.2 交易引擎增加 Hunter v7 live pre-open guard

文件：

- `trader/auto_trader_loop.go`
- `trader/auto_trader_orders.go`
- `trader/auto_trader_risk.go`

新增行为：

- 主循环把 Hunter v7 candidate context 传入执行链路。
- 下单前重新获取 live execution price。
- 使用 live price 重新 cap TP、修复 price/SL/TP/RR、验证几何。
- 对 `range_expansion_event SHORT` 增加硬拒绝：
  - live price 比 AI 决策价反弹 `>=0.30%`；
  - 15m close 低于 EMA20 `>=10%`；
  - entry zone position `>80%`；
  - taker_buy_15m `>=0.48` 不再支持空头延续；
  - reasoning 出现 SHORT/above VWAP-EMA20 方向冲突。

这会直接覆盖 LAB 的 `7.146 -> 7.243` 反弹追空问题，并兜底 BLESS 的方向语义冲突。

### 4.3 修复 protected close reason 同步保真

文件：

- `store/position.go`
- `store/position_builder.go`
- `trader/auto_trader_risk.go`

新增 `position_close_intents` 表：

- 如果保护器触发平仓时 DB 尚未同步出 OPEN position，先按 `trader_id + symbol + side` 记录 pending close intent。
- order sync 后续通过 `PositionBuilder` 创建并关闭 position 时，优先应用 pending intent。
- pending intent 只在 15 分钟内有效，避免未来同币同向新仓误用旧 close_reason。
- LAB 这类“开仓和平仓都早于第一次 order sync”的场景，最终 DB 应记录为 `hard_loss_close/system_protector`，不再落成 `sync/sync`。

### 4.4 保留此前动态保护止损修复

此前已实施：

- 开仓后前 10 分钟延迟动态保护止损上移；
- 只有峰值 ROE `>=8%` 或原始有利波动 `>=0.60%` 才允许提前锁盈；
- 利润地板改为至少锁定约 `3%` 杠杆收益，避免手续费后仍亏。

这部分针对 DEEP/AIGENSYN 的“微利保本但扣费后亏损”。

## 5. 新增测试

新增/更新覆盖：

- `TestHunterV7LiveOpenGuardRejectsShortReasoningDirectionConflict`
- `TestHunterV7LiveOpenGuardRejectsRangeExpansionShortReboundFromDecisionPrice`
- `TestHunterV7LiveOpenGuardRejectsRangeExpansionShortDeepBelowEMA`
- `TestPositionBuilderAppliesPendingProtectedCloseIntent`
- `TestPositionBuilderIgnoresExpiredPendingProtectedCloseIntent`

测试命令：

```bash
go test ./store
go test ./trader
go test ./kernel
go test ./...
```

结果：全部通过。

## 6. 服务状态

已用最新代码重启本地 AIT 服务：

- tmux 会话：`ait`
- 后端监听：`http://localhost:8080`
- 前端监听：`http://localhost:3000`
- 后端日志已记录 `/api/health` 200
- KKK 在数据库中仍为 `is_running=0`，本次重启未自动启动交易员

## 7. 剩余实施建议

P0 已完成：

- 提示词方向约束；
- 执行前 live price 二次确认；
- `range_expansion_event SHORT` 追空反弹硬拦截；
- pending protected close reason 保真。

P1 建议继续观察后实施：

- 同一 symbol 最近 2-3 轮 LONG/SHORT 快速翻向时降级到 REVIEWABLE/WATCH；
- `range_expansion_event SHORT` 在 `trend_up + regime_against_direction` 下要求更强的 15m 重新跌破确认；
- 连续 hard_loss 后对同 setup 类型做全局冷却；
- 小账户费用感知过滤：预期 TP0 毛收益低于双边手续费 2.5-3 倍时直接 wait。

## 8. 后续观察指标

建议跟踪 KKK 后续 5-10 轮：

- 是否还有 `range_expansion_event SHORT` 在 15m 极端低于 EMA20 后开仓；
- 是否出现 `direction_confirmation_conflict` 或 `rebound_risk_wait` 拦截；
- 是否还有开仓后 2 分钟内 `hard_loss_close`；
- LAB 类快速同步仓位是否能正确记录 `hard_loss_close/system_protector`；
- 扣费后净 PnL 是否改善，而不是只提升毛 PnL 或开仓率。

结论：本轮优化不放宽 hard loss，也不盲目提高开仓率，而是减少低质量 EXECUTABLE 进入实盘，特别是高波动事件追空的方向错误和执行前反弹风险。
