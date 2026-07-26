# Hunter v7 高振幅山寨 Alt-Ladder 重构方案

日期：2026-07-18  
目标：在 Binance 合约数百个标的中，提高对 24h 振幅 20%+、涨跌幅 30%+ 山寨/强庄标的的分段捕捉能力，覆盖从 5% 启动、15% 中段、30% 末段的多空入场机会，同时保持 Hunter v7 不追末端、不忽略 RR、不放松流动性硬约束的设计哲学。

## 1. 问题复盘

现有 Hunter v7 已经通过 Universe 的 volume/gainer/loser/OI/funding/velocity/new-activity/amplitude 池把大部分高振幅币纳入候选，但路由层存在三个缺口：

- 早期启动缺口：`trend_breakout_long` 偏 Bollinger 压缩突破，`leader_momentum_long` 要求 24h 涨幅较高，`displacement_momentum_long` 要求 1h range expansion 足够大。很多山寨从 +5% 到 +12% 的初段并不满足这些强形态。
- 中段延续缺口：+12% 到 +30% 的币如果未回踩 EMA/VWAP，容易被统一打成 chase risk；但其中一部分有 OI、taker、量能同步，应该进入可复核小仓顺势，而不是全部 WATCH。
- 下跌续跌缺口：`breakdown_momentum_short` 已能捕捉较强下跌，但对 -5% 到 -10% 的早期破位偏谨慎；强庄币从高位回落时，应通过结构破位 + taker 卖压 + OI/量能确认捕捉第一段空头，而不是等到瀑布末端。

## 2. 设计原则

- 不用“涨幅大就追”。必须至少满足价格结构、taker、OI/volume 三类证据中的两类。
- 不用“跌多就空”。必须破 VWAP/EMA/BB 中轨之一，并确认卖压或 OI/量能。
- 分生命周期处理：
  - Stage 1：5%-12%，早期启动/破位，允许 REVIEWABLE 到 EXECUTABLE。
  - Stage 2：12%-25%，主趋势中段，要求更强 flow，允许小仓 open。
  - Stage 3：25%-45%，末端强趋势，默认 reduce-size-or-wait，只有买盘/卖盘与 OI 同向才可复核。
- 冲突不交给 LLM 猜：同 symbol 多空冲突仍由 `ResolveV7Conflicts` 降级 `conflict_watch`。
- 复用现有可测确认码：`live_price_in_entry_zone`、`taker_buy_15m_gt_0_52`、`taker_buy_15m_lt_0_48`、`oi_delta_1h_positive_or_quote_volume_expands`、`taker_flow_not_flipping_against_direction`。

## 3. 新增两条薄路由

### Route L: Alt Ladder Momentum Long

setup：`alt_ladder_momentum_long`  
定位：非核心高振幅山寨的 5%-45% 多头阶梯行情。

准入：

- 非核心主流币，`QuoteVolume24h >= 5M`。
- 启动条件：`Change24h >= 5`，且满足 `Change1h >= 1.2` / `Change4h >= 4` / `Velocity15m >= 0.8` 任一。
- 结构条件：价格在 `VWAP15m`、`EMA20_1h`、`BBMiddle15m` 之上至少一项。
- flow 条件至少两票：
  - `TakerBuy15m >= 0.52`
  - `OIDelta1h > 0.5` 或 `OIDelta4h > 2`
  - `VolumeBurst15m >= 1.1` 或 `VolumeBurst1h >= 1.2`
  - `Change1h >= 2.5`
- 排除：`Change1h > 12`、`Change24h > 45`、`RSI1h >= 86` 且 funding 拥挤。

输出：

- Stage 1：`alt_ladder_stage_early`
- Stage 2：`alt_ladder_stage_mid`
- Stage 3：`alt_ladder_stage_late`
- 末段或 1h 过快：`alt_ladder_late_chase_risk`，LLM 只能小仓或等回踩。

### Route M: Alt Ladder Breakdown Short

setup：`alt_ladder_breakdown_short`  
定位：非核心高振幅山寨从高位或弱势区的 -5% 到 -30% 破位续跌。

准入：

- 非核心主流币，`QuoteVolume24h >= 5M`。
- 下跌启动：`Change1h <= -1.8` / `Change4h <= -4` / `Velocity15m <= -0.8` 任一。
- 结构破位：价格低于 `VWAP15m`、`EMA20_1h`、`BBMiddle15m` 至少一项。
- flow 条件至少两票：
  - `TakerBuy15m <= 0.48`
  - `OIDelta1h > 0.5` 新空进入，或 `OIDelta1h < -2` 多头挤出
  - `VolumeBurst15m >= 1.1`
  - `Change4h <= -5`
- 排除：`Change1h <= -14`，或接近 1d low 且 OI 已大幅流出。

输出：

- `alt_ladder_downshift_early`
- `alt_ladder_downshift_mid`
- `alt_ladder_downshift_late`
- 末段无 OI flush 时 `alt_ladder_late_short_risk`，等待反抽或继续放量确认。

## 4. 集成点

- `hunter_v7_types.go`：新增两个 setup。
- `hunter_v7_mod_alt_ladder.go`：新增多头/空头两个轻量模块。
- `hunter_v7_router.go`：注册在 displacement 与 MMS 之前，使其作为高振幅山寨专用补位层。
- `hunter_v7_weights.go`：rotation/mania/trend_up 强化多头，trend_down/pullback/rotation 强化空头。
- `hunter_v7_execution.go`：新增 shape、expectancy、RR/entry 语义。
- `hunter_v7_tag_catalog.go`：新增 alt ladder 标签语义。
- `kernel/engine.go`：新增 EXECUTABLE/REVIEWABLE 分类。
- `engine_prompt.go`：新增 LLM 剧本，明确 stage 1/2/3 的开仓纪律。
- `trader/auto_trader_risk.go`：新增 setup guard 默认 entry zone 约束。

## 5. 验收

```bash
go test ./provider/local ./kernel ./trader
go run ./cmd/hunter_v7_validate -rounds=1 -top-detail=280 -max-workers=6 -max-output=90 -watch-output=12 -min-priority=35 -aggressive=true -out-dir=reports/alt-ladder-v7-validate-20260718
```

验收标准：

- 单测覆盖 +8% 多头初段、+20% 多头中段、-8% 空头早期破位。
- live validation JSON / prompt 可序列化并含 `tag_semantics`。
- 同 symbol 多空冲突仍进入 `conflict_watch` 或保留单侧高优先级，不输出双向可开仓。
- 高振幅山寨路由增加 REVIEWABLE/EXECUTABLE 候选，但末端追价仍降级为 chase risk 或 reduce-size-or-wait。

## 6. 实施与实盘验证记录

实施日期：2026-07-18。

已完成：

- 新增 `alt_ladder_momentum_long` 与 `alt_ladder_breakdown_short` 两个 setup。
- 新增 `provider/local/hunter_v7_mod_alt_ladder.go`，用统一的 stage/flow/structure 逻辑覆盖山寨 +5/+15/+30 与 -5/-15/-30 生命周期。
- 路由注册在 displacement 后、MMS 前，作为高振幅山寨补位层。
- 新增 regime 权重、标签语义、LLM 剧本、tier 分类、交易 guard entry-zone 默认值。
- 新增合成测试覆盖早期多头、末段多头风险、早期空头破位。

验证结果：

```bash
go test ./provider/local ./kernel ./trader
# ok

go run ./cmd/hunter_v7_validate -rounds=1 -top-detail=280 -max-workers=6 -max-output=90 -watch-output=12 -min-priority=35 -aggressive=true -out-dir=reports/alt-ladder-v7-validate-20260718
# 2026-07-18 14:33 CST: symbols=524, universe=256, signals=42, EXECUTABLE=8, REVIEWABLE=1, WATCH=26, REJECTED=7, issues=0
```

本轮实盘输出：

- `alt_ladder_momentum_long=3`：GALAUSDT、TRADOORUSDT、BSBUSDT。
- `alt_ladder_breakdown_short=10`：AVAAIUSDT、BANKUSDT、STARUSDT、REUSDT、ESPORTSUSDT、TOSHIUSDT、BIRBUSDT、TREEUSDT、DOODUSDT 等。
- 总输出从上一轮同类验证的 20 个信号提升到 42 个信号，且 JSON/prompt 校验通过、执行性缺口为 0。

结论：Alt-Ladder 路由补上了高振幅山寨从启动到中段、从高位破位到下跌续跌的捕捉缺口；但仍保留 entry-zone、RR、taker/OI/volume 同向确认和末段风险标签，不以单纯涨跌幅放宽开仓。
