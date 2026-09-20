# Hunter v7 alt_ladder_breakdown_short 平衡软升档实施与最终两轮复核 - 2026-08-03

> 实施时间：2026-08-03 CST  
> 最终实时验证目录：`reports/hunter-v7-alt-ladder-short-soft-release-2round-final-20260803`  
> 数据源：Binance USDS-M Futures public REST，只读未下单。  
> 结论口径：不能用单轮行情把 `alt_ladder_breakdown_short` 继续压紧，也不能把 OI 下降的 flush 型短空直接并入主开仓。

## 1. 已实施内容

本次采用“二次确认软升档”，不是单轮过拟合式压紧：

- `alt_ladder_breakdown_short` 只允许 early/mid downshift 进入软升档评估。
- 必须低区位，且区位必须可计算：`entry_zone_position <= 45%`。区位缺失不再默认为 0，避免误放行。
- 必须有新空 OI：`alt_ladder_new_shorts` 且 `OI 1h >= 0.8%`。
- 标准软升档：`taker_buy_15m <= 0.38` 且 stop distance <= 2.25%。
- 强卖流软升档：`taker_buy_15m <= 0.34` 时，stop distance 可放宽到 2.45%。
- 高波动、late short risk、danger risk、`funding_elevated + execution_stop_tightened`、买盘反包、late 且未 close-through 仍硬拦。
- 软升档最多到 `REVIEWABLE-small`，不直接升级 `EXECUTABLE`。
- 中英文猎手 v7 策略提示词同步补充上述边界。
- 修复长期 outcome 写入副作用：后续 DB 入库时只有最终 `EXECUTABLE/REVIEWABLE` 行初始化为 `ACTIVE`，`WATCH/REJECTED/空 tier` 不再默认写 `track_status=ACTIVE`。

变更文件：

- `kernel/engine.go`
- `kernel/hunter_v7_prompt_doctrine.go`
- `kernel/engine_prompt_compact_test.go`
- `kernel/hunter_v7_signal_persistence.go`
- `kernel/hunter_v7_signal_persistence_test.go`

## 2. 最终两轮实时复核

命令：

```bash
HTTP_PROXY=http://127.0.0.1:7897 HTTPS_PROXY=http://127.0.0.1:7897 ALL_PROXY=http://127.0.0.1:7897 NO_PROXY=localhost,127.0.0.1,::1 TZ=Asia/Shanghai \
go run ./cmd/hunter_v7_validate \
  -rounds=2 \
  -round-interval=5m \
  -top-detail=220 \
  -max-workers=8 \
  -max-output=30 \
  -watch-output=8 \
  -min-priority=45 \
  -aggressive=true \
  -post-track-duration=15m \
  -post-track-interval=60s \
  -out-dir=reports/hunter-v7-alt-ladder-short-soft-release-2round-final-20260803
```

Run summary：

| Round | Regime | Signals | Prompt open-review | Open-rate | REST errors | Universe | Degraded |
|---:|---|---:|---:|---:|---:|---:|---|
| 1 | rotation | 8 | 0 | 0.0% | 0 | 209 | false |
| 2 | rotation | 10 | 1 | 10.0% | 0 | 208 | false |
| 合计 | rotation | 18 | 1 | 5.6% | 0 | - | 2/2 valid |

报告文件：

- `reports/hunter-v7-alt-ladder-short-soft-release-2round-final-20260803/hunter-v7-validation-run-summary-20260803-072215.md`
- `reports/hunter-v7-alt-ladder-short-soft-release-2round-final-20260803/hunter-v7-validation-outcomes-20260803-073531.md`

## 3. 盈亏与止盈止损跟踪

15 分钟 post-track 后无剩余 active，2 个 tracked outcome 均结案，均来自 `alt_ladder_momentum_long`，不是本次 short 分支。

| Symbol | Dir | Setup | Tier | Status | PnL% | MFE% | MAE% | 备注 |
|---|---|---|---|---|---:|---:|---:|---|
| BICOUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | PROTECTED_STOP | +0.247 | +1.579 | -2.007 | 先给浮盈后保护退出 |
| FHEUSDT | LONG | alt_ladder_momentum_long | REVIEWABLE | STOP | -2.168 | +0.214 | -2.437 | 未达 TP0，原始止损 |

聚合：

| Setup | Count | Protected | Stop | Win-rate | Avg PnL% | Avg MFE% | Avg MAE% |
|---|---:|---:|---:|---:|---:|---:|---:|
| alt_ladder_momentum_long | 2 | 1 | 1 | 50.0% | -0.960 | +0.897 | -2.222 |

这说明本轮 open-review 有恢复，但盈利质量还不达高优：`BICOUSDT` 能保护退出，`FHEUSDT` 未形成有效 TP0。后续不应只追 open-rate，还要约束 late/high-vol alt-ladder long 的 MAE。

## 4. alt_ladder_breakdown_short 复核

最终两轮主输出 short 候选：

| Round | Symbol | OI 1h | Taker buy 15m | Entry zone pos | Stop dist | Risk tags | 结论 |
|---:|---|---:|---:|---:|---:|---|---|
| 1 | STARUSDT | -4.800% | 0.390 | 31.8% | 2.00% | execution_stop_tightened | OI 下降，非新空；不放行正确 |
| 1 | ZAMAUSDT | -2.005% | 0.382 | 31.8% | 2.20% | - | OI 下降，非新空；不放行正确 |
| 1 | DEXEUSDT | -3.449% | 0.433 | 31.8% | 2.00% | execution_stop_tightened | taker 偏高且 OI 下降；不放行正确 |
| 2 | KAITOUSDT | -2.549% | 0.426 | 31.8% | 2.20% | - | taker 偏高且 OI 下降；不放行正确 |

扩展观察：

- 本轮 short 分支主要是 `alt_ladder_long_flush / sell_volume + OI下降`，不是 `alt_ladder_new_shorts + OI正增`。
- `FHEUSDT SHORT` 在第一轮潜在池出现 OI +1.104%，但 taker_buy_15m=0.407，高于标准软升档 0.38，且不属于最终 prompt open-review；不应拿来证明 short 分支可继续放宽。
- 因此，本次没有证据支持新增 `long_flush + OI下降` 主开仓通道。它可以进入 watch/audit pool，但不能进入真实 open-review 分母。

## 5. 对开仓率与胜率的影响

- 开仓率：最终两轮 `1/18 = 5.6%`，第二轮单轮达到 `1/10 = 10.0%`。相比上一组 `0/15`，没有继续压死，但恢复来自 long 分支，不来自 short 分支。
- short 分支：没有合格“低区位 + 新空 OI + 低 taker + 可控 stop”样本，因此未释放开仓。这不是过紧，而是避免把出清式下跌误当作新空延续。
- 盈利率：2 个 outcome 为 1 protected / 1 stop，结案胜率 50.0%，平均 PnL -0.960%。样本太少，不能作为稳定胜率结论，但足以说明盈利质量未达高优。
- 副作用：旧逻辑会让非开仓行长期残留 `track_status=ACTIVE`，可能污染长期 outcome 视图；已从代码层修复后续写入，历史数据建议单独迁移清理。

## 6. 可实施优化建议

1. 保留当前 `alt_ladder_breakdown_short` 软升档规则，不继续降低 OI/taker 门槛。
2. 新增 `alt_ladder_flush_watch_pool`，只审计 `long_flush + OI下降` 的 15m/30m/1h MFE、MAE、TP0-before-SL，不进入真实 open-review。
3. 给 validation outcome summary 增加逐笔 details 列表：symbol、direction、setup、tier、status、entry、exit、TP0、SL、PnL、MFE、MAE，减少每次靠 DB 手工复盘。
4. 对 `alt_ladder_momentum_long` 增加 late/high-vol 盈利质量门槛：late stage 或 high_volatility 时，要求 entry_zone_position <= 55% 或 5m VWAP reclaim，否则降为 WATCH。
5. 历史 DB 清洗建议：将 `execution_tier NOT IN ('EXECUTABLE','REVIEWABLE')` 且 `track_status='ACTIVE'` 的旧记录清空或迁移为 audit-only，避免长期 API 把非开仓样本混入 active。

## 7. 验证

已通过：

```bash
go test ./kernel -run 'TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes|TestBuildHunterV7PromptPayloadIncludesTP0Plan|TestFormatCompactMarketDataAddsHunterV7ExecutionContext'
go test ./kernel ./trader
node --check scripts/hunter_v7_latest_binance_replay.mjs
git diff --check
```

当前判断：本次优化落地后，`alt_ladder_breakdown_short` 不再是粗暴硬拦，但也没有被一轮行情误放宽。后续提升开仓率的方向应来自“分桶观察池 + 多轮稳定证据”，不是把 OI 下降的 flush 型短空直接纳入主开仓。
