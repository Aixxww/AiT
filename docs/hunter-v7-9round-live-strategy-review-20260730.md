# Hunter v7 9 轮 Binance 实时审校与可实施优化方案

生成时间：2026-07-30 07:35 CST
样本来源：

- 第一组 3 轮：`reports/hunter-v7-binance-live-3round-20260730/`
- 第二组 6 轮：`reports/hunter-v7-binance-live-6round-review2-20260730/`
- 数据库窗口：`hunter_v7_signal_records.timestamp >= 2026-07-29 21:57:00 UTC`

## 1. 目标口径

目标不是把所有信号都放宽成 open，而是在不牺牲止损率的前提下提升“高质量开仓候选率”：

- Prompt-final open-rate 目标：18%-25%。
- EXECUTABLE 占 open-review 目标：至少 35%，避免全部依赖 REVIEWABLE 人工复核。
- 短窗口质量目标：TP0/protected stop/active profit 合计占 tracked open-review 的 60% 以上。
- 亏损 STOP 目标：不超过 tracked open-review 的 25%-30%。
- 单 setup 最低样本：20 条 raw row 或 10 条输出信号后才允许自动调阈值。

本次 9 轮仍是短窗口，不能宣称长期胜率；但足够暴露路由与提示词的结构问题。

## 2. 9 轮全局统计

| 指标 | 数值 |
|---|---:|
| 总轮次 | 9 |
| 输出信号 | 86 |
| LONG / SHORT | 43 / 43 |
| Regime | trend_down=2, rotation=7 |
| Runtime tiers | EXECUTABLE=3, REVIEWABLE=19, WATCH=43, REJECTED=21 |
| Prompt-final tiers | EXECUTABLE=3, REVIEWABLE=17, WATCH=45, REJECTED=21 |
| Runtime open-rate | 22/86 = 25.6% |
| Prompt-final open-rate | 20/86 = 23.3% |
| REST partial coverage | 5/9 轮出现，合计 rest_errors=10 |

开仓率已经达到目标上沿，但盈利质量没有达到高优水平，主要问题集中在 `alt_ladder_breakdown_short` REVIEWABLE。

## 3. 第二组 6 轮重点统计

6 轮均为 `rotation`，更适合观察当前行情下的开仓质量。

| 指标 | 数值 |
|---|---:|
| 输出信号 | 52 |
| LONG / SHORT | 24 / 28 |
| Runtime tiers | EXECUTABLE=2, REVIEWABLE=13, WATCH=28, REJECTED=9 |
| Prompt-final tiers | EXECUTABLE=2, REVIEWABLE=12, WATCH=29, REJECTED=9 |
| Runtime open-rate | 15/52 = 28.8% |
| Prompt-final open-rate | 14/52 = 26.9% |
| tracked open-review | 15 |
| WIN/TP | 1 |
| protected stop | 1 |
| loss stop | 1 |
| active profit / active loss | 3 / 9 |
| Avg PnL / MFE / MAE | -0.436% / +0.571% / -0.863% |

结论：开仓候选率已经偏高，问题不是“开不出来”，而是 REVIEWABLE 短空质量不足和保护退出不够及时。

## 4. Setup 级审校

### 4.1 输出信号池口径

| Setup | 9 轮输出 | Open-review | 输出池开仓率 | 结论 |
|---|---:|---:|---:|---|
| alt_ladder_breakdown_short | 16 | 0 | 0.0% | JSON readiness 多为 WATCH，但 DB raw runtime 给出多个 REVIEWABLE；统计/持久化口径需收敛 |
| alt_ladder_momentum_long | 7 | 6 | 85.7% | 当前最优开仓来源，rotation 下应作为主力 EXECUTABLE |
| funding_reversal | 27 | 0 | 0.0% | 拆桶有效，应继续保持观察池 |
| panic_reversal_long | 12 | 0 | 0.0% | 当前保守合理，放宽会接刀 |
| mms_trend_ride_long | 4 | 0 | 0.0% | 输出池仍 WATCH，但 DB 有 REVIEWABLE，需统一分层口径 |
| whale_flow_reversal | 7 | 1 | 14.3% | 需要 TP0/保本保护，不能只靠原 stop |

### 4.2 DB raw setup funnel 口径

第二组 6 轮 raw rows：

| Setup | Raw rows | Open rows | Raw open-rate | 风险 |
|---|---:|---:|---:|---|
| funding_reversal | 126 | 0 | 0.0% | 大量背景信号，不应进主分母 |
| trend_breakout_long | 64 | 0 | 0.0% | 重复出现但不升级，可能过保守或缺 trigger 细分 |
| mms_trend_ride_long | 25 | 3 | 12.0% | 新增可用多头候选，但样本仍小 |
| alt_ladder_breakdown_short | 23 | 10 | 43.5% | REVIEWABLE 过多且最终 PnL 差，是首要收紧对象 |
| alt_ladder_momentum_long | 16 | 2 | 12.5% | DB open 低于输出池，需要检查持久化映射口径 |

## 5. Outcome 审校

第二组 6 轮 tracked open-review：

| Setup | Tier | Status | N | Avg PnL | Avg MFE | Avg MAE | 结论 |
|---|---|---|---:|---:|---:|---:|---|
| alt_ladder_breakdown_short | REVIEWABLE | ACTIVE | 8 | -0.694% | +0.388% | -1.099% | 当前主亏损源 |
| alt_ladder_breakdown_short | REVIEWABLE | STOP | 2 | -1.050% | +0.851% | -1.506% | 1 亏损 stop，1 保护 stop |
| alt_ladder_momentum_long | EXECUTABLE | WIN_TP0 | 1 | +0.637% | +1.717% | 0.000% | 最优正样本 |
| alt_ladder_momentum_long | EXECUTABLE | ACTIVE | 1 | -0.224% | +0.794% | -0.669% | 有回撤，需要 TP0/保护推进 |
| mms_trend_ride_long | REVIEWABLE | ACTIVE | 3 | +0.233% | +0.415% | -0.159% | 可作为次级多头候选 |

9 轮合并 tracked open-review：

- 总 tracked=22。
- WIN/TP=1，protected stop=1，loss stop=3，active profit=7，active loss=10。
- Avg PnL=-0.335%，Avg MFE=+0.556%，Avg MAE=-0.731%。

结论：开仓率已足够，盈利率未达标。下一步必须降低 `alt_ladder_breakdown_short` 的 REVIEWABLE 放行率，并把有 MFE 的信号更早推进 TP0/保本保护。

## 6. 发现的问题

### P0-1：alt_ladder_breakdown_short REVIEWABLE 放行过宽

负样本：

- HOMEUSDT SHORT：STOP -2.559%，MFE 仅 +0.139%，MAE -3.013%。
- REUSDT SHORT 多次重复 REVIEWABLE，最终多个样本转为浮亏。
- BTWUSDT / RIFUSDT / DIAUSDT 短空在反抽后 MAE 扩大。

问题本质：

- `fast_confirm` 不等于可开仓。
- 高优先级与 setup_score 高，掩盖了“反抽失败确认不足”。
- 同 symbol/setup 多轮重复出现，被统计为多个可开仓 row，放大了风险暴露。

### P0-2：输出信号 tier 与 DB runtime tier 存在口径差异

例子：

- 输出信号池中 `alt_ladder_breakdown_short` 多为 WATCH，但 DB raw rows 有 10 条 REVIEWABLE。
- 输出信号池中 `alt_ladder_momentum_long` open-rate 很高，但 DB open rows 偏少。

风险：

- 报告中的 open-rate、prompt-final open-rate、DB outcome open-rate 可能指向不同对象。
- 后续自动调参若使用错误分母，会把 WATCH 或 raw row 误认为真实开仓。

### P0-3：outcome 跟踪周期不足

validate 当前只在每轮末尾 tick，命令结束后 ACTIVE 不再持续更新。

影响：

- TP0/STOP 后续触发可能漏记。
- ACTIVE 浮盈/浮亏被固定在短窗口状态，胜率不稳定。

### P1-1：TP0/保护退出应成为盈利率核心

正样本：

- UAIUSDT `alt_ladder_momentum_long` EXECUTABLE：WIN_TP0，PnL +0.637%，MFE +1.717%。
- VELVETUSDT `alt_ladder_breakdown_short`：protected stop +0.459%，MFE +1.564%。

问题：

- 多个信号有明显 MFE，但若不推进保护，容易回吐到亏损 STOP。

### P1-2：mms_trend_ride_long 具备次级候选潜力

6 轮中 `mms_trend_ride_long` REVIEWABLE active 平均 PnL +0.233%，MAE -0.159%，优于 alt_ladder_breakdown_short。

限制：

- 样本仅 3 条 tracked，不足以直接升为主开仓。
- 应先作为 REVIEWABLE 强化，而不是直接 EXECUTABLE。

### P1-3：funding_reversal 拆桶正确

9 轮输出 27 条，open=0；6 轮 raw 126 条，open=0。

结论：

- funding 当前是背景/观察池，不应纳入主开仓率。
- 若要放行，必须走四确认：retest_failed、no_new_high_after_rejection、taker_sell_strong、15m below VWAP。

## 7. 可实施改造清单

### P0-A：统一 open-rate 统计口径

落点：

- `kernel/hunter_v7_signal_persistence.go`
- `api/hunter_v7_matrix.go`
- `cmd/hunter_v7_validate/main.go`

实施：

1. 增加 `signal_bucket`：`prompt_output`、`raw_setup`、`module_no_match`。
2. 增加 `open_eligibility`：`executable`、`reviewable_track_only`、`watch_only`、`rejected`、`raw_unexpanded`。
3. 矩阵默认 open-rate 只计算 prompt-output 的 EXECUTABLE + REVIEWABLE。
4. raw setup funnel 单独展示，不与 prompt-final open-rate 混算。

验收：

- 同一轮 runtime tier、prompt-final tier、DB matrix 三者分母一致或明确拆栏。

### P0-B：alt_ladder_breakdown_short 降级规则

落点：

- `provider/local/hunter_v7_signal_state.go`
- `provider/local/hunter_v7_mod_alt_ladder.go`
- `kernel/hunter_v7_prompt_doctrine.go`

实施规则：

1. `alt_ladder_downshift_late` 一律不得直接 REVIEWABLE；默认 WATCH。
2. 只有同时满足以下条件才允许 REVIEWABLE：
   - 连续两轮出现同 symbol/setup/direction；
   - 5m 或 15m close below trigger；
   - taker_buy_15m < 0.46；
   - 当前价未 reclaim trigger；
   - 1m/5m 最近 candle 没有强反抽。
3. 同 symbol/setup/direction 在 30 分钟内只保留最新一条 open-review，旧记录只做 context，不重复注册 outcome。
4. 新增 blocked gate：`alt_ladder_short_retest_pending`、`alt_ladder_short_reclaim_risk`、`duplicate_signal_context_only`。

验收：

- HOMEUSDT/REUSDT/BTWUSDT 这类重复短空不再多次进入 REVIEWABLE。
- alt_ladder_breakdown_short 的 loss stop 率低于 30%。

### P0-C：validate 增加持续 outcome 跟踪

落点：

- `cmd/hunter_v7_validate/main.go`

新增参数：

```bash
--post-track-duration=30m
--post-track-interval=30s
--track-active-only=true
```

实施：

1. 多轮结束后不立即退出。
2. 对 ACTIVE 的 EXECUTABLE/REVIEWABLE 每 30 秒刷新 1m candles。
3. 直到 TP0/TP1/STOP/timeout 后写回 DB。
4. Markdown 报告追加 final outcome snapshot。

验收：

- 报告不再只给短窗口浮盈/浮亏。
- 能输出 closed win-rate、protected-stop rate、active unresolved rate。

### P1-A：TP0/保本推进成为强制风控

落点：

- `trader/auto_trader_risk.go`
- `trader/*dynamic_stop*`
- `kernel/hunter_v7_prompt_doctrine.go`

规则：

1. MFE >= 0.8R 或价格达到 TP0 距离的 60%，立即把 stop 推到 entry 附近。
2. MFE >= 1.0R 且 taker 从强转中性，优先 reduce/close。
3. REVIEWABLE 入场默认小仓，只有 TP0 后才允许保留剩余仓位。

验收：

- VELVETUSDT 类样本优先 protected stop，而不是利润回吐。
- ONUSDT 类 MFE>1% 后不再回到亏损 STOP。

### P1-B：alt_ladder_momentum_long 作为主提升方向

落点：

- `provider/local/hunter_v7_mod_alt_ladder.go`
- `provider/local/hunter_v7_tier_rules.go`
- `kernel/hunter_v7_prompt_doctrine.go`

实施：

1. rotation regime 下，`alt_ladder_momentum_long` 若 setup_score>=95、timing>=75、liquidity>=75、risk<=30，允许 EXECUTABLE。
2. 若 MFE 达 TP0 阈值，强制保护退出。
3. 对 late stage 加 `small_size_or_wait_pullback`，避免追高。

验收：

- EXECUTABLE 不低于每 3 轮 1 条。
- TP0/protected stop 率高于 60%。

### P1-C：mms_trend_ride_long 小步放宽为 REVIEWABLE

落点：

- `provider/local/hunter_v7_mod_mms.go`
- `kernel/hunter_v7_prompt_doctrine.go`

实施：

1. EMA7>EMA25>EMA99 且回踩 EMA25/VWAP 收回；
2. taker_buy_15m >= 0.54；
3. risk<=20，liquidity>=55；
4. 只允许 REVIEWABLE，不直接 EXECUTABLE，直到样本 >=20 且 positive/protected rate 达标。

验收：

- mms_trend_ride_long REVIEWABLE 每 6 轮 2-4 条；
- MAE 显著低于 alt_ladder_breakdown_short。

### P2：继续保持 funding_reversal 观察池

落点：

- `api/hunter_v7_matrix.go`
- `kernel/hunter_v7_prompt_doctrine.go`

实施：

1. funding 统计在 `reversal_watch_pool`，不纳入主 open-rate。
2. prompt 明确 funding crowding 不构成开仓理由。
3. 只有四确认齐全才进入 REVIEWABLE。

验收：

- funding 不污染主开仓率。
- 极端拥挤行情下仍可召回 watch context。

## 8. 策略提示词补充建议

需要加入或加强以下条款：

1. REVIEWABLE 不是 open 权限，必须写出缺失确认；只有 EXECUTABLE 可默认执行。
2. `alt_ladder_breakdown_short` 重复出现不能自动升级；若未 close below trigger 或 taker_buy_15m 未低于 0.46，必须 wait。
3. 同 symbol/setup/direction 的重复信号只允许选择最新一条，旧信号作为 context。
4. `alt_ladder_momentum_long` 在 rotation 中是主开仓形态，但 late stage 必须小仓或等回踩。
5. MFE 达 0.8R/TP0 60% 后，优先保护利润；不得让已给利润的 REVIEWABLE 回到亏损止损。
6. funding_reversal 只做观察池，四确认不齐不得 open。
7. mms_trend_ride_long 只允许 REVIEWABLE，小样本期不得直接当作 EXECUTABLE。

## 9. 高优目标达成路径

当前状态：

- 开仓率：已达到或略高于目标。
- 盈利率：未达标，主要受 alt_ladder_breakdown_short 拖累。

实施顺序：

1. 先做 P0-A/P0-C，修正统计和 outcome 跟踪，否则无法可靠评价胜率。
2. 再做 P0-B，压低 alt_ladder_breakdown_short 的伪 REVIEWABLE。
3. 同时做 P1-A，把 TP0/保护退出作为硬风控。
4. 然后做 P1-B/P1-C，把开仓率增长来源转到 alt_ladder_momentum_long 与 mms_trend_ride_long。
5. funding_reversal 保持拆桶，不参与开仓率优化。

预期效果：

- prompt-final open-rate 从当前 23%-27% 收敛到 18%-24%。
- EXECUTABLE 占比从当前偏低提升到 35% 左右。
- loss stop 率下降，protected stop/TP0 占比上升。
- 平均 PnL 从负值转为接近 0 或正值，再通过长窗口验证 TP1/TP2 贡献。
