# Hunter v7 币安实时 3 轮全链路复盘与可实施优化报告 - 2026-08-03

> 测试窗口：2026-08-03 06:22:17 / 06:27:58 / 06:33:37 CST  
> 数据源：Binance USDS-M Futures public REST，1m K 线 outcome 跟踪，只读未下单。  
> 主验证目录：`reports/hunter-v7-live-3round-5m-20260802-rerun`  
> WATCH 审计目录：`reports/hunter-v7-watch-audit-20260802`  
> 对照基线：`docs/hunter-v7-binance-live-3round-outcome-analysis-20260801.md`、`docs/hunter-v7-binance-live-3round-post30m-optimization-review-20260802.md`、`docs/hunter-v7-optimization-implementation-validation-20260802.md`

## 1. 结论

本轮不能按“单一形态一两轮表现”直接继续放宽或收紧。更可靠的判断是：

1. 数据质量有效：3/3 valid rounds，REST errors=0，universe coverage 39.5%-40.3%，可用于本轮开仓率与 outcome 评估。
2. 优化后主开仓率明显偏低：prompt-final open-review 为 1/30 = 3.3%，低于此前目标 8%-15%，也低于 2026-08-01 三轮 29.6% 与 2026-08-02 有效复审 3.7% 附近的低位。
3. 正式 outcome 只有 `alt_ladder_momentum_long` 2 条结案：1 条 `PROTECTED_STOP` +0.142%，1 条 `STOP` -0.947%，严格 TP 胜率 0%，保护/非亏损率 50%，平均 PnL -0.403%。
4. 之前的风险保护优化有正向效果：`PROTECTED_STOP` 已真实出现，PTBUSDT MFE +1.508% 后没有回吐成亏损止损。
5. 但开仓率被过度压缩，主要不是 Binance 数据质量问题，而是最终提示词/执行层对 `alt_ladder_breakdown_short`、`alt_ladder_momentum_long` 的二次门过严，以及 DB tier 与 prompt-final tier 口径仍有差异。
6. WATCH 审计显示不能简单整体放开：`trend_breakout_long` 119 个观察样本中 85 个先止损，拦截正确；但 `alt_ladder_breakdown_short` 13 个观察样本中 9 个 TP0 先于 SL、0 个先止损，存在显著误杀。

## 2. 主验证结果

命令：

```bash
HTTP_PROXY=http://127.0.0.1:7897 HTTPS_PROXY=http://127.0.0.1:7897 ALL_PROXY=http://127.0.0.1:7897 NO_PROXY=localhost,127.0.0.1,::1 TZ=Asia/Shanghai \
go run ./cmd/hunter_v7_validate \
  -rounds=3 \
  -round-interval=5m \
  -top-detail=220 \
  -max-workers=8 \
  -max-output=30 \
  -watch-output=8 \
  -min-priority=45 \
  -aggressive=true \
  -post-track-duration=20m \
  -post-track-interval=60s \
  -out-dir=reports/hunter-v7-live-3round-5m-20260802-rerun
```

| Round | Regime | Signals | Prompt open-review | Open-rate | REST errors | Universe | Coverage | Degraded |
|---:|---|---:|---:|---:|---:|---:|---:|---|
| 1 | rotation | 11 | 0 | 0.0% | 0 | 207 | 39.5% | false |
| 2 | rotation | 10 | 0 | 0.0% | 0 | 211 | 40.3% | false |
| 3 | rotation | 9 | 1 | 11.1% | 0 | 211 | 40.3% | false |
| 合计 | rotation | 30 | 1 | 3.3% | 0 | - | - | 3/3 valid |

补充口径：

- Runtime tier 曾出现 3 个开放候选：R1 `PTBUSDT LONG`、R1 `OPUSDT LONG`、R3 `PTBUSDT LONG`。
- Prompt-final tier 只保留 R3 `PTBUSDT LONG` 为 REVIEWABLE。
- DB execution_tier 明细有 3 条，但 R1 `PTBUSDT` 被 tracker 标记为 `DUPLICATE_CONTEXT`，真实 outcome 只结算 R1 `OPUSDT` 与 R3 `PTBUSDT`。

## 3. Outcome 跟踪

20 分钟 post-track，active 已全部收敛：

| Status | Count | Profit | Loss | Flat | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|---:|
| PROTECTED_STOP | 1 | 1 | 0 | 0 | +0.142 | +1.508 | 0.000 |
| STOP | 1 | 0 | 1 | 0 | -0.947 | +0.234 | -1.250 |

| Symbol | Round | Direction | Setup | DB Tier | Status | PnL% | MFE% | MAE% | 判断 |
|---|---:|---|---|---|---|---:|---:|---:|---|
| OPUSDT | 1 | LONG | alt_ladder_momentum_long | REVIEWABLE | STOP | -0.947 | +0.234 | -1.250 | 追动量失败，MFE 不足以保护 |
| PTBUSDT | 3 | LONG | alt_ladder_momentum_long | REVIEWABLE | PROTECTED_STOP | +0.142 | +1.508 | 0.000 | TP0/保本保护有效 |

评估：

- `PROTECTED_STOP` 拆分已发挥作用，应继续计为风控成功，不应与亏损 STOP 混算。
- 本轮 `alt_ladder_momentum_long` 的最终盈利质量不足：2 条中 1 条保护、1 条亏损，无 TP0/TP1 结案。
- R1 `PTBUSDT` 被去重为 duplicate，说明 30 分钟同 thesis 去重有效；但 DB/prompt/outcome 三个口径仍需要更清楚标识。

## 4. 各形态路由表现

### 4.1 主输出池口径

| Setup | 三轮输出 | Prompt open-review | 主结论 |
|---|---:|---:|---|
| alt_ladder_momentum_long | 4 | 1 | 唯一进入主执行层，但胜率不足，需更严格区分早/中/晚段与 funding 风险 |
| alt_ladder_breakdown_short | 4 | 0 | 当前过度压制，后续 WATCH 审计显示有明显 missed opportunity |
| funding_reversal | 9 | 0 | 拆桶正确，不污染主开仓率 |
| trend_breakout_long | 4 | 0 | 拦截基本正确，未见有效突破确认 |
| 其他 watch/reversal | 9 | 0 | breaker 连续干燥后冷却，符合保护逻辑 |

### 4.2 WATCH/missed opportunity 审计口径

审计命令：

```bash
node scripts/hunter_v7_latest_binance_replay.mjs \
  --since '2026-08-02 22:15:00' \
  --out-dir reports/hunter-v7-watch-audit-20260802 \
  --doc docs/hunter-v7-watch-audit-20260802.md \
  --proxy http://127.0.0.1:7897
```

同一 30 分钟内 `symbol + direction + setup_type` 去重后：

| Setup | 样本 | 当前正向 | TP0 先于 SL | 先止损 | 当前均值 | MFE 均值 | MAE 均值 | 判断 |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| trend_breakout_long | 119 | 16.8% | 0.0% | 71.4% | -0.27% | +0.17% | -0.56% | 拦截正确，不能放宽 |
| funding_reversal | 53 | 56.6% | 50.9% | 3.8% | -0.01% | +0.54% | -0.52% | 有事件机会，但应继续拆桶，避免污染主开仓率 |
| alt_ladder_breakdown_short | 13 | 76.9% | 69.2% | 0.0% | +0.95% | +1.74% | -0.44% | 误杀明显，应二次确认软升档 |
| alt_ladder_momentum_long | 11 | 63.6% | 63.6% | 9.1% | +0.21% | +1.41% | -0.97% | 有机会但波动大，需 TP0 快保护 |
| range_expansion_event | 2 | 100.0% | 100.0% | 0.0% | +0.83% | +1.59% | -2.53% | 样本太少，仅保留观察 |

代表性 missed opportunity：

| Symbol | Direction | Setup | 15M | 30M | Current | MFE/MAE | 结果 |
|---|---|---|---:|---:|---:|---:|---|
| BANKUSDT | SHORT | alt_ladder_breakdown_short | +1.15% | +3.89% | +3.85% | +6.23% / 0.00% | WIN_TP1 |
| HYPERUSDT | SHORT | alt_ladder_breakdown_short | +1.97% | +2.08% | +2.31% | +2.56% / -0.11% | WIN_TP0_ACTIVE |
| BICOUSDT | LONG | alt_ladder_momentum_long | +2.26% | +2.18% | +2.33% | +2.61% / -0.99% | WIN_TP0_ACTIVE |
| EDENUSDT | LONG | alt_ladder_momentum_long | +2.30% | +0.83% | +0.81% | +2.67% / 0.00% | WIN_TP0_ACTIVE |

这些样本不计入真实胜率，但说明“当前主开仓率 3.3%”并非纯粹市场无机会，而是部分二次确认规则和 prompt-final 门槛过严。

## 5. 对之前优化效果的评估

### 有效项

1. 数据质量门生效：本轮 3/3 valid，避免了 2026-08-02 degraded 轮次误读。
2. `PROTECTED_STOP` 独立统计生效：PTBUSDT MFE +1.508% 后以 +0.142% 保护退出，没有被归类为亏损止损。
3. duplicate thesis 去重生效：R1/R3 PTBUSDT 没有被当作两笔完整独立交易放大分母。
4. `funding_reversal` 拆桶有效：三轮输出 9 个 funding 候选，均未进入主开仓率，避免了事件型反转污染主胜率。
5. `trend_breakout_long` 过滤有效：WATCH 审计中该形态先止损比例高，当前严格确认必要。

### 副作用

1. 开仓率过低：valid prompt-final open-review 只有 3.3%，低于目标 8%-15%。
2. `alt_ladder_breakdown_short` 被过度压制：主输出 4 个均未开放，WATCH 审计 13 个同族样本却有 69.2% TP0-before-SL。
3. `alt_ladder_momentum_long` 的 backend/prompt 过滤不稳定：R1 runtime 显示 EXECUTABLE，但 prompt-final 因 `backend_rr_infeasible` 降为 WATCH；同时 DB 仍有 REVIEWABLE 被跟踪，说明三层口径仍需治理。
4. 正式盈利质量未达高优：2 个真实 outcome 平均 PnL -0.403%，没有 TP0/TP1 结案，不能宣称胜率改善。

## 6. 可实施优化方案

### P0-A：统一三层开仓率分母

当前存在 runtime tier、prompt-final tier、DB execution_tier 三套口径。建议：

- 主开仓率只以 prompt-final `EXECUTABLE + REVIEWABLE` 为分子。
- Outcome tracker 只统计 prompt-final 开放候选；DB raw tier 若被 prompt 降级，进入 `watch_audit`，不进入真实 outcome。
- 报告同时输出：
  - `runtime_open_review_rate`
  - `prompt_final_open_review_rate`
  - `tracked_trade_thesis_count`
  - `duplicate_context_count`

预期影响：胜率分母不再被 DB/prompt 口径差异污染，开仓率变化可被准确归因。

### P0-B：alt_ladder_breakdown_short 二次确认软升档

不能直接放开全部 short ladder。建议只在以下条件全部满足时，从 WATCH 升到 REVIEWABLE-small：

- `alt_ladder_downshift_early` 或 `alt_ladder_downshift_mid`。
- `alt_ladder_taker_sell` 或 `taker_buy_15m <= 0.34`。
- `alt_ladder_new_shorts` 且 `oi_delta_1h >= 0.8%`。
- `entry_zone_position <= 45%`。
- 最近 5m/15m 未重新收回 trigger 上方；若尚未收盘击穿，则只给 REVIEWABLE，不给 EXECUTABLE。
- `stop_distance_pct <= 2.2%`，且 TP0-after-fee 至少覆盖 0.50%。

对应模块：`provider/local/hunter_v7_mod_alt_ladder.go`、`kernel/hunter_v7_prompt_doctrine.go`。

预期影响：把 BANK/HYPER/AIO/ONDO 这类低 MAE、顺向短阶梯从 missed opportunity 拉回小仓复核，同时继续拦截反抽未失败的追空。

### P0-C：alt_ladder_momentum_long 只开放“低 funding + 强流 + 可保护”的早中段

本轮正式亏损来自 OPUSDT LONG，MFE 仅 +0.234%，说明仅凭 early/mid 与 taker 达标仍不够。建议：

- `funding_elevated` 出现时，LONG 默认降一档；除非 taker_buy_15m >= 0.58 且 OI 1h >= 1.5%。
- `entry_zone_position > 65%` 且 TP0 距离不足 0.50% 时，不允许 EXECUTABLE，只能 REVIEWABLE-small 或 WATCH。
- TP0 触达后强制 30%-50% 减仓，并把剩余止损推进到 entry 或 5m EMA20/VWAP。

对应模块：`provider/local/hunter_v7_mod_alt_ladder.go`、`trader/hunter_v7_pnl_tracker.go`、prompt doctrine。

预期影响：保留 PTBUSDT 这类能触发保护的机会，减少 OPUSDT 式 MFE 不足即回撤的动量追单。

### P1：funding_reversal 建立事件型二次确认，不纳入主开仓率

WATCH 审计显示 funding 有短线 TP0 机会，但均值接近 0，且事件分布强依赖单币拥挤。建议：

- 继续不计入主开仓率。
- 建立独立 `funding_event_watch_pool`：
  - 15m close below VWAP。
  - taker_buy_15m <= 0.42。
  - OI 与价格反向，或 OI 降温 + 价格拒绝新高。
  - funding/crowding 极值持续两轮以上。
- 只输出 event-review，不进入普通 open-review KPI。

### P1：trend_breakout_long 不放宽，只补触发价审计

119 个 WATCH 审计样本中 85 个先止损，说明当前突破确认门正确。建议只补：

- 输出 `suggested_trigger`、`required_close`、`expires_in_bars` 的长期 outcome。
- 若触发后 15m/30m 未站稳，再回写 `false_breakout_watch`，用于后续阈值学习。

### P2：长期 outcome 增加 missed opportunity 表

建议新增或扩展长期 outcome：

- `tracked_source`: `prompt_open_review` / `watch_audit` / `rejected_audit`
- `direction_accuracy_15m/30m/1h`
- `tp0_before_sl`
- `first_event`
- `duplicate_thesis_key`
- `would_have_opened_rule_version`

这样后续优化可基于多日、多行情 regime 的累计统计，不再依赖单轮人工复盘。

## 7. 下一轮验收标准

继续至少 3 组、每组 3 轮，每轮 5 分钟，且只用 valid rounds：

- `prompt_final_open_review_rate`: 8%-15%。
- `strict_terminal_win_rate`: >= 50%。
- `protected_or_win_rate`: >= 70%。
- `loss_stop_rate`: <= 25%。
- `avg_loss_stop`: >= -0.8%。
- `MFE >= 0.60% 后亏损 STOP`: 0。
- `alt_ladder_breakdown_short` 的 REVIEWABLE 不超过该 setup 输出的 25%-35%，但 missed opportunity TP0-before-SL 不能连续高于 50%。

当前结论：不要因为一轮 `alt_ladder_breakdown_short` WATCH 表现好就整体放开；应实施“小仓二次确认软升档 + 统一分母 + 长期 missed opportunity outcome”。这能提升开仓率，同时不牺牲已修复的胜率与保护止损质量。
