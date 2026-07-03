# KKK 开仓率与盈利能力优化实施报告

时间：2026-07-03  
对象：AIT 实盘交易员 KKK  
策略：猎手v7-HHH保守（`hhh-hunter-v7-conservative-20260703`）

## 1. 结论

KKK 近 30 多轮只成功开仓 `DEEPUSDT` 与 `AIGENSYNUSDT`，核心原因不是单一信号缺失，而是三类问题叠加：

- 开仓候选偏少：多数周期没有 `EXECUTABLE` / `REVIEWABLE`，系统直接跳过 AI 或只能 wait。
- 可开仓候选被误判或拦截：部分 `EXECUTABLE` 候选被 AI 以 `rr_insufficient` 等理由等待；另有开仓意图被价格漂移、入场区或高波动保护拦截。
- 成功开仓后过早退出：`DEEPUSDT` 与 `AIGENSYNUSDT` 均为 20x、小名义仓位，动态保护止损在开仓后 1-6 分钟内过早推到接近成本价，往返手续费后净收益转负。

本次优化没有放宽硬风控，而是把后端已验证的可执行条件明确同步给 AI，并修复动态保护止损的过早保本问题，目标是提高“有效开仓率”和“手续费后盈利留存”。

## 2. 数据证据

KKK 当前交易员记录：

- 交易员 ID：`d7b3b284_445bf32d-9add-4960-a925-29539fa66e69_deepseek_1783021529`
- 轮次：34 轮历史决策
- AI 开仓意图：4 次
- 成功成交：2 次，`DEEPUSDT`、`AIGENSYNUSDT`

关键开仓/失败记录：

| 周期 | 标的 | 动作 | 结果 | 原因 |
|---:|---|---|---|---|
| 4 | TAIKOUSDT | open_short | 失败 | 入场价漂移 0.603%，超过 0.500% |
| 13 | MUSDT | open_short | 拦截 | `high_volatility + execution_stop_tightened` wait-only 组合 |
| 26 | DEEPUSDT | open_short | 成功 | 后续 1.65 分钟后被交易所保护止损同步关闭 |
| 31 | AIGENSYNUSDT | open_short | 成功 | 后续 6.41 分钟后被交易所保护止损同步关闭 |
| 35 | WLDUSDT | open_long | 拦截 | `whale_flow_reversal` 入场区位置 72.3%，超过后端 45% 限制 |

已关闭仓位：

| 标的 | 方向 | 入场 | 出场 | 持仓 | realized_pnl | fee | 净效果 |
|---|---|---:|---:|---:|---:|---:|---|
| DEEPUSDT | SHORT | 0.01788 | 0.01787 | 1.65m | +0.00788 | 0.0140855 | 手续费后约 -0.0062 USDT |
| AIGENSYNUSDT | SHORT | 0.03008 | 0.03009 | 6.41m | -0.00435 | 0.01308696 | 手续费后约 -0.0174 USDT |

说明：DB 中另有 `HEIUSDT` 早前 `OPEN` 记录，但本次用户关注的近 30 多轮成交主线确认为 `DEEPUSDT` 与 `AIGENSYNUSDT`。

## 3. 根因分析

### 3.1 开仓率偏低

大多数周期没有进入 AI 开仓复核的候选：

- `EXECUTABLE=0` 且 `REVIEWABLE=0` 时，系统按 Hunter v7 漏斗直接 wait。
- 这是策略保守过滤导致的正常现象，但会降低实盘开仓频率。

可优化点不应是把 `WATCH` 直接放开，而是减少“已经可执行却被误判等待”的情况。

### 3.2 RR 判断与后端有效合约不一致

部分 `EXECUTABLE` 候选已经有后端 `confirmation_summary.rr` / `effective_rr` 验证，但 AI 仍用更主观的结构 RR 输出 `rr_insufficient`。

这会导致：

- 已通过后端确认的机会被跳过。
- AI 与交易引擎对同一候选的开仓条件不一致。
- 开仓率下降，但并没有提高实际风险收益质量。

### 3.3 动态保护止损过早收紧

`DEEPUSDT` 和 `AIGENSYNUSDT` 的共同特征：

- 小账户下名义仓位约 13-14 USDT。
- 20x 杠杆时，5.5% ROE 只对应约 0.275% 原始价格波动。
- 往返手续费与滑点足以吃掉“保本止损”的微小利润。
- 动态保护器在开仓后 1-6 分钟把止损推到接近成本价，交易所触发后 DB 以 `sync` 关闭。

因此原先的“保本保护”在小仓位高杠杆环境中实际不是净盈利保护，而是容易造成手续费后亏损。

### 3.4 后端 guard 未完全同步给提示词

重启后新增一轮 KKK 决策中，AI 选择了 `WLDUSDT` whale flow 多单，但被后端拦截：

- `entry_zone_position=72.3%`
- 后端 guard 要求 `whale_flow_reversal LONG` 不追 zone 上半区，超过 45% 拦截

这说明提示词需要显式包含后端关键 guard，否则 AI 会把机会浪费在必然被交易引擎拦截的候选上。

## 4. 已实施优化

### 4.1 动态保护止损增加早期延迟

文件：`trader/auto_trader_risk.go`

新增约束：

- 开仓后前 10 分钟，如果已有交易所保护止损，不再轻易上移动态止损。
- 早期只有满足以下任一条件才允许提前锁盈：
  - 峰值 ROE ≥ 8%
  - 原始价格有利波动 ≥ 0.60%
- 避免 `DEEPUSDT` / `AIGENSYNUSDT` 这种微利刚出现就被保本止损扫掉。

### 4.2 利润地板改为手续费后净收益保护

文件：`trader/auto_trader_risk.go`

新增 `protectorProfitFloorNetBufferPnLPct=3.0`：

- 原利润地板偏向接近成本价。
- 新利润地板至少锁定约 3% 杠杆收益。
- 目的不是扩大风险，而是避免保护止损触发后手续费后仍为负。

### 4.3 提示词对齐后端 RR 验证

文件：`kernel/engine_prompt.go`

新增规则：

- 若 `execution_tier=EXECUTABLE`
- 且 `confirmation_summary.passed_review=true`
- 且 `confirmation_summary.rr/effective_rr` 已达到最小 RR

则默认视为后端 RR 已验证。AI 不得重新臆造更严格的结构 RR。只有明确列出当前可执行价、后端 capped TP、stop_loss 后仍低于阈值，才允许输出 `rr_insufficient`。

### 4.4 提示词同步 whale flow 入场区后端 guard

文件：`kernel/engine_prompt.go`

新增规则：

- `whale_flow_reversal LONG` 不允许追 entry zone 上半区。
- 若 `entry_zone_position >45%`，必须 wait 或选择下一个合格 `EXECUTABLE` / `REVIEWABLE` 候选。
- 避免类似 WLDUSDT 第 35 周期的“AI 选择后端必拦截候选”。

### 4.5 回归测试

新增/更新测试：

- `TestDynamicProtectionStopDelaysEarlyProfitFloorForSmallMove`
  - 复现 `DEEPUSDT`：开仓 2 分钟、峰值 5.59%、已有保护止损，不应提前重写止损。
- `TestDynamicProtectionStopAllowsEarlyLockAfterTP0Peak`
  - 早期若已达到 TP0/8% ROE 区域，仍允许保护利润。
- `TestProtectionStopHelpersShort`
  - 校验空单利润地板至少锁定手续费后净收益缓冲。
- Prompt 测试覆盖：
  - 后端 RR 已通过时不得臆造 `rr_insufficient`。
  - `entry_zone_position >45%` 的 whale flow long 必须与后端 guard 一致。

## 5. 测试结果

已执行：

```bash
go test ./kernel ./trader
go test ./...
```

结果：

- `kernel` 通过
- `trader` 通过
- 全量 `go test ./...` 通过

## 6. 服务重启

已使用最新代码重启 AIT。

运行状态：

- tmux 会话：`ait`
- 后端：`http://localhost:8080`
- 前端：`http://localhost:3000`
- 后端健康检查：`{"status":"ok","time":null}`
- 监听确认：
  - backend `main` 监听 `*:8080`
  - frontend `node` 监听 `*:3000`

## 7. 后续观察指标

建议重点跟踪 KKK 后续 3-6 个周期：

- `EXECUTABLE` / `REVIEWABLE` 候选数量是否稳定。
- AI 是否继续对后端已通过 RR 的候选输出 `rr_insufficient`。
- 是否还有开仓意图被后端 guard 拦截，尤其是 entry zone、价格漂移、tight stop。
- 新成交仓位是否能持有超过 10 分钟，且保护止损触发后是否能覆盖手续费。
- `execution_log` 中 `effective_contract` 的实际 SL/TP/RR 是否与 AI reasoning 一致。

本次优化的原则是：不把 WATCH 候选硬放开，不降低必要确认，而是减少 AI 与后端执行规则不一致导致的无效等待和无效开仓，并修复小仓位动态止损造成的手续费后亏损。
