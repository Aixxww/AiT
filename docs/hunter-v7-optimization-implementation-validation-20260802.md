# Hunter v7 开仓率/胜率优化实施与验证报告 - 2026-08-02

> 实施时间：2026-08-02 CST  
> 实时验证目录：`reports/hunter-v7-binance-live-3round-post30m-optimized-20260802`  
> 基准问题报告：`docs/hunter-v7-binance-live-3round-post30m-optimization-review-20260802.md`

## 1. 本次已实施

### P0：alt_ladder_breakdown_short 分层放行

已将此前“一缺反抽失败确认即硬 WATCH”的逻辑改为分层：

- 继续硬拦：
  - `high_volatility` / `extreme_volatility`
  - `alt_ladder_late_short_risk`
  - `funding_elevated` + `execution_stop_tightened`
  - `alt_ladder_downshift_late` 且无 `alt_ladder_multi_cycle_close_through` / `trigger_memory_confirmed`
  - `taker_buy_15m > 0.42`
- 允许软放行 REVIEWABLE：
  - `alt_ladder_downshift_early` 或 `alt_ladder_downshift_mid`
  - liquidity >= 70
  - stop distance <= 2.2%
  - `taker_buy_15m <= 0.38`，或 `alt_ladder_new_shorts` + OI 1h > 1.0% + `taker_buy_15m <= 0.42`

这会释放 GRVT/BANK 这类低 taker、新空参与、止损距离可控的样本，同时继续拦截 CAP/BTW/BEAT 这类 taker 反向或波动风险未解除的样本。

### P0：short continuation 家族 MFE 后保本

保本保护从仅覆盖 `alt_ladder_breakdown_short` 扩展为：

- `alt_ladder_breakdown_short`
- `breakdown_momentum_short`
- `relative_weakness_short`
- `range_expansion_event` SHORT

当前规则：

- MFE >= 0.60% 后，动态止损推进到 signal entry breakeven。
- 该规则直接覆盖上一轮 MMTUSDT 这类先有 +0.656% MFE、随后回撤到 -2% STOP 的问题。

### P1：提示词 TP0/保本字段同步

`relative_weakness_short` 与 `range_expansion_event` SHORT 现在会输出 TP0 与 `move_stop_to_breakeven=true` 语义，保证策略提示词和后端 outcome tracker 对齐。`range_expansion_event` LONG 未被扩大影响。

## 2. 代码验证

已通过：

- `go test ./kernel -run 'TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes|TestBuildHunterV7PromptPayloadIncludesTP0Plan|TestFormatCompactMarketDataAddsHunterV7ExecutionContext'`
- `go test ./trader -run 'TestSignalOutcomeTrackerAltLadderShortBreakevenAfterMFE|TestSignalOutcomeTrackerShortContinuationBreakevenAfterMFE|TestHunterV7LiveOpenGuardRejectsRangeExpansionShortReboundFromDecisionPrice'`
- `go test ./kernel ./trader`
- `git diff --check`

新增测试覆盖：

- 早期 alt-ladder short 在 `taker_buy_15m <= 0.38` 下软放行 REVIEWABLE。
- 中期 alt-ladder short 在 `alt_ladder_new_shorts` + OI 1h > 1.0% + taker <= 0.42 下软放行 REVIEWABLE。
- 缺 close trigger 但满足低 taker 分层放行时，可进入 live-reviewable。
- `breakdown_momentum_short` / `relative_weakness_short` / `range_expansion_event` SHORT 在 MFE >= 0.60% 后触发 breakeven stop。

## 3. Binance 实时回归

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
  -post-track-duration=30m \
  -post-track-interval=60s \
  -out-dir=reports/hunter-v7-binance-live-3round-post30m-optimized-20260802
```

结果：

| Round | Regime | Signals | Open-review | Open-rate | REST errors | Universe | Degraded |
|---:|---|---:|---:|---:|---:|---:|---|
| 1 | compression | 8 | 3 | 37.5% | 0 | 99 | true |
| 2 | trend_down | 9 | 0 | 0.0% | 0 | 99 | true |
| 3 | compression | 10 | 5 | 50.0% | 183 | 93 | true |
| 合计 | - | 27 | 8 | 29.6% | 183 | - | true |

重要限制：

- valid_rounds = 0。
- 前两轮 universe coverage 仅 18.9%，第 3 轮 universe coverage 17.7% 且 REST error rate 34.9%。
- 因此本轮只能作为实时 smoke 回归，不能作为严格胜率验收样本。

## 4. Outcome 结果

30 分钟 post-track：

- tracked = 7
- active = 0
- WIN_TP0 = 1
- STOP = 6
- status 口径平均 PnL：
  - STOP avg PnL = -0.845%
  - WIN_TP0 avg PnL = +0.445%

按 setup：

| Setup | Count | Wins | Loss Stops | Protected Stops | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|---:|
| alt_ladder_momentum_long | 3 | 1 | 2 | 0 | -1.224 | 0.432 | -1.570 |
| displacement_momentum_long | 2 | 0 | 0 | 2 | +0.308 | 0.850 | -0.121 |
| range_expansion_event | 1 | 0 | 1 | 0 | -1.750 | 0.693 | -1.936 |
| whale_flow_reversal | 1 | 0 | 0 | 1 | +0.180 | 0.672 | 0.000 |

观察：

- 开仓率从上一轮严格有效样本的 3.7% 回升到 all-round 29.6%，说明分层放行不会继续把开仓率压死。
- 但由于 3/3 degraded，本轮 open-rate 不能直接作为正式生产指标。
- 本轮没有满足软放行条件的 `alt_ladder_breakdown_short`：BEATUSDT taker_buy_15m 为 0.48/0.65，且无新空 OI 支持，继续 WATCH 是符合新规则的。
- `range_expansion_event` LONG 仍出现 -1.75% STOP；这不属于本次 short continuation 保本修复范围，下一步应单独审查 range long 的追价/回踩确认。

## 5. 优化效果判断

### 正向

- 开仓率压死问题已从规则层解除，但仍保留高风险硬拦。
- MMTUSDT 暴露的 short continuation MFE 后回吐问题已在 tracker 层修复，并有单测覆盖。
- 提示词 TP0/保本字段与 tracker 执行口径已同步。
- range_expansion_event SHORT 的反抽失败确认已有路由与 live guard 双层保护，本次未削弱。

### 未完成验收

- 本轮 Binance 实时数据质量不足，无法得出“胜率已提升”的严格结论。
- 本轮亏损主要来自 LONG 侧：`alt_ladder_momentum_long` 与 `range_expansion_event` LONG，不是本次 short continuation 修复的直接目标。
- 需要在 universe coverage >= 30%、REST error rate <= 20% 的有效轮次中复测 3 轮以上。

## 6. 追加一轮当前价复核

> 复核时间：2026-08-02 12:06:22 CST  
> 口径：对本次 7 条实际 tracked open-review 记录，直接调 Binance futures 当前价与信号后 1m K 线，重算当前 PnL、区间 MFE/MAE、TP0/SL 触达。

| ID | Round | Symbol | Dir | Setup | Tier | Signal | Current | Current PnL% | High Since | Low Since | Recalc MFE% | Recalc MAE% | TP0 | SL | DB Status | DB PnL% |
|---:|---:|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---|---|---|---:|
| 96080 | 1 | HOMEUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | 0.00670200 | 0.00697400 | +4.058 | 0.00728800 | 0.00669300 | +8.744 | -0.134 | Y | N | WIN_TP0 | +0.445 |
| 96082 | 1 | ENAUSDT | LONG | displacement_momentum_long | REVIEWABLE | 0.08221000 | 0.08540000 | +3.880 | 0.08548000 | 0.08187000 | +3.978 | -0.414 | Y | N | STOP/protected | +0.285 |
| 96098 | 1 | PUMPUSDT | LONG | alt_ladder_momentum_long | EXECUTABLE | 0.00228300 | 0.00223400 | -2.146 | 0.00228600 | 0.00221800 | +0.131 | -2.847 | N | Y | STOP | -2.082 |
| 96278 | 3 | UAIUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | 0.54610000 | 0.54710000 | +0.183 | 0.55900000 | 0.52640000 | +2.362 | -3.607 | Y | Y | STOP | -2.033 |
| 96282 | 3 | 1000PEPEUSDT | LONG | whale_flow_reversal | REVIEWABLE | 0.00286410 | 0.00287180 | +0.269 | 0.00289580 | 0.00284470 | +1.107 | -0.677 | N | N | STOP/protected | +0.180 |
| 96293 | 3 | 1000SATSUSDT | LONG | range_expansion_event | EXECUTABLE | 0.00001144 | 0.00001127 | -1.486 | 0.00001149 | 0.00001074 | +0.437 | -6.119 | N | Y | STOP | -1.750 |
| 96300 | 3 | LINKUSDT | LONG | displacement_momentum_long | REVIEWABLE | 8.23000000 | 8.28800000 | +0.705 | 8.30900000 | 8.21900000 | +0.960 | -0.134 | N | N | STOP/protected | +0.330 |

复核判断：

- `HOMEUSDT` 的 WIN_TP0 结论被当前价强化，信号后最高 MFE 已扩大到 +8.744%。
- `ENAUSDT`、`LINKUSDT`、`1000PEPEUSDT` 的 DB 状态显示为 STOP，但 exit/stop 均在 entry 上方，实质是 protected stop/保本上移后的正收益退出；这类状态命名应继续拆分为 `PROTECTED_STOP_PROFIT`，避免统计上被误读为亏损止损。
- `UAIUSDT` 先触 TP0 再触 SL，说明 LONG 侧也需要 TP0 后强制减仓+保本，不能只依赖最终 stop 结案。
- `PUMPUSDT` 与 `1000SATSUSDT` 的亏损被当前价继续印证；其中 `1000SATSUSDT` 信号后 MAE 扩大到 -6.119%，range expansion LONG 需要新增回踩失败/负 OI 降级。

## 7. 下一步可实施项

1. 将 validator 在 `valid_rounds=0` 时自动输出 `INVALID_SAMPLE_DO_NOT_USE_FOR_WINRATE`，避免误读 all open-rate。
2. 对 `alt_ladder_momentum_long` 增加 late/high-volatility 下的 MFE>=0.60% breakeven 或 TP0 强制减仓。
3. 对 `range_expansion_event` LONG 增加“回踩确认失败/entry_zone_position > 60 且 OI 1h 为负”的 REVIEWABLE 降级。
4. 将 universe coverage 低于 30% 的原因拆成 endpoint/market-type/filter 三类，便于定位是代理波动还是筛选 universe 过窄。
5. 在有效数据窗口再次执行 3 轮 + 30m post-track，验收目标保持：
   - valid open-review rate 8%-15%
   - strict terminal win rate >= 50%
   - MFE>=0.60% 后亏损 STOP = 0
   - average loss stop 控制在 -0.8% 以内
