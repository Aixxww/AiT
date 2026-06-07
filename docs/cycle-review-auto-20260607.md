# AiT 周期复盘与修复汇总（2026-06-07）

来源报告：`reports/cycle-review-auto-20260607.md`
生成时间：2026-06-07 11:24:56 CST
交易员：`d7b3b284_445bf32d-9add-4960-a925-29539fa66e69_deepseek_1779165317`
周期范围：#155 - #164

## 复盘结论

最近 10 个周期全部成功完成，但全部输出 `wait`，没有新开仓。同期最近已平仓 9 笔，胜率 44.4%，净 PnL 为 -2.45 USDT，最大单笔亏损 -1.26 USDT，最大盈利 0.51 USDT。主要问题不是系统失败，而是入场质量、止损距离和候选信号执行条件需要更强约束。

| 指标 | 数值 |
|---|---:|
| 总周期 | 10 |
| 成功周期 | 10 |
| 失败周期 | 0 |
| 开仓次数 | 0 |
| Wait 次数 | 10 |
| 开仓率 | 0.0% |
| 平仓笔数 | 9 |
| 胜率 | 44.4% |
| 净 PnL | -2.45 USDT |
| 最大盈利 | 0.51 USDT |
| 最大亏损 | -1.26 USDT |
| 总 Token | 470101 |
| 平均 Token/周期 | 47010 |
| 最高 Token/周期 | 55233 |

## 本次 Qoder 优化

### 1. 周期复盘工具

新增 `cmd/cycle_review`，可从 SQLite 决策记录生成 Markdown/JSON 复盘报告，覆盖：

- 周期成功率、开仓率和 wait 比例。
- 每周期候选数、动作、token、耗时。
- 最近平仓 PnL、胜率、最大盈亏。
- execution guard 拦截统计。
- 基于开仓率、胜率、PnL 和失败率的自动建议。

### 2. Token 估算校准

新增历史 token 校准能力：

- `GET /api/strategies/token-calibration`
- `POST /api/strategies/estimate-tokens` 支持 `trader_id` 和 `calibration`
- `store.Decision().GetTokenCalibration(...)`
- `StrategyConfig.EstimateTokensWithCalibration(...)`

目的：用真实决策记录修正 prompt 估算误差，减少策略编辑页对 token 压缩判断的偏差。

### 3. 降级上下文保护

交易上下文获取余额或持仓失败时，会在短时间内使用缓存快照，并标记：

- `IsDegraded`
- `DegradationReasons`
- `AccountDataStale`
- `PositionDataStale`
- `DisableOpenOrders`

降级期间禁止新开仓，只允许 `hold`、`wait` 或风险降低型平仓，避免在账户/持仓数据不可靠时继续扩大风险。

### 4. 失败开仓冷却

新增最近失败开仓过滤：

- 最近 3 个周期内某 symbol 开仓被拒绝，则短期跳过该候选。
- 避免 stale signal、交易所拒单、风控拦截后立即重复尝试。

### 5. Binance 执行稳定性

增强 Binance U 本位执行：

- 签名请求加入 transient network retry。
- 时间戳错误时重新同步 server time 后重试。
- 日志脱敏签名参数。
- 杠杆设置可读取交易所限制并降级到有效杠杆。

## Hunter v7 Funding Reversal 修复

关联文档：[Hunter v7 Funding Reversal 风控修复报告](hunter-v7-funding-reversal-risk-fix-2026-06-07.md)

本次结合周期 #142/#146 复盘，补齐三层硬规则：

### 筛选层

- `funding_reversal SHORT` 在深跌后且 OI 仍 building 时过滤或降级。
- 对称处理深涨后追多。
- OI building 不再作为 funding reversal 加分项，改为扣分和风险标签。

### Prompt 层

- compact prompt 输出 `entry_zone_pos`、`oi_state`、VWAP、ATR、EMA20、近期高低点。
- 将 `warning` 升级为 `hard_rule=...; output_wait_only`。
- 明确 `position_size_usd` 是名义仓位，不是保证金，不能再乘一次杠杆。

### 执行层

- `funding_reversal` 默认需要 OI flush 或 failed rebuild。
- SHORT 需靠近区间上沿/retest，LONG 需靠近区间下沿/reclaim。
- Hunter v7 未显式配置时默认最小止损距离为 2%。
- 增加单笔止损亏损上限和 TP1 距离封顶。

## 验证

后端测试已通过：

```bash
go test ./provider/local ./kernel ./trader
go test ./...
```

服务已在 `ait-dev` tmux 会话重启：

- API：http://localhost:8080
- Web：http://localhost:3000
- `/api/health` 返回 `{"status":"ok","time":null}`

## 后续观察项

- 继续观察 `funding_reversal` hard rule 命中后价格是否继续反向验证，评估规则是否过严。
- 周期复盘中单独统计被 `DisableOpenOrders`、`failed-open cooldown`、Hunter v7 guard 拦截的候选。
- 若 wait 比例长期高于 90%，优先放宽非 funding reversal setup 的 timing，而不是放松 funding reversal 的 OI/区间硬规则。
