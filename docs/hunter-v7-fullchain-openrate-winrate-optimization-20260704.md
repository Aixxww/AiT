# Hunter v7 全链路开仓率、胜率与盈利优化更新

> 更新时间：2026-07-04  
> 关联报告：`reports/hunter-v7-fullchain-openrate-winrate-optimization-20260703.md`  
> 最新 live validation：`reports/hunter-v7-live-validation-report-20260704-045356.md`

## 结论

Hunter v7 的问题不是发现不到标的，而是高波动山寨信号在数据时效、资金流确认、range expansion 子形态、LLM 信号身份匹配和真实执行价复核上需要更硬的闭环。本轮更新的目标是：

1. 保持 universe 和 potential pool 对币安合约币种的高召回。
2. 只把新鲜、确认充分、RR 真实达标的候选交给交易引擎开仓。
3. 让每笔 Hunter v7 真实开仓都能追溯到唯一 `signal_id`。
4. 提前保护 TP0 利润，并降低 `sync` 平仓归因污染。

## 已实施

### 0. REVIEWABLE 扩容：高质量 near-confirm 进入现场复核

文件：

- `kernel/engine.go`
- `kernel/engine_prompt.go`
- `trader/auto_trader_risk.go`

最新实时验证显示分层正常但偏严格：`near_confirm` 候选只要缺少短周期收盘、taker flow 或 momentum 现场确认，就会被压到 WATCH，导致 prompt 没有可开仓复核对象。本轮新增 live-reviewable 通道：

- 低风险、高流动性、入场区可达、硬字段完整。
- 只缺现场可复核确认，例如 `directional_15m_close_long`、`5m_or_15m_close_through_breakout_level`、`taker_flow_confirms_long`、`momentum_not_exhausted`。
- 满足 setup floor 的 trend breakout、whale flow、displacement、pullback、range retest/continuation 可以进入 REVIEWABLE。
- late chase、do-not-market-chase、RR/entry zone/硬数据缺失、高危标签仍保持 WATCH/REJECT。

交易引擎对这类 REVIEWABLE 开仓会先执行 REST + orderbook micro-refresh；只有刷新后写入 `fresh_rest_confirmed` 和 `fresh_micro_confirmed`，这些现场确认缺口才允许通过。刷新失败、盘口漂移、资金流反向或 1m 反向移动仍会阻断。

### 1. Taker buy ratio 修复

文件：

- `datafetch/websocket.go`
- `datafetch/websocket_test.go`

WebSocket aggTrade 的 taker ratio 已从旧的 `old*0.99 + qty*0.01` 改成 5 分钟滚动主动买/主动卖成交量占比。大额成交不再把 ratio 推到 1 以上，`taker_buy_aligned`、`taker_sell_aligned`、whale flow、range expansion 的资金流确认更可信。

### 2. 信号身份与数据时效

文件：

- `provider/local/hunter_v7.go`
- `provider/local/hunter_v7_types.go`
- `provider/local/hunter_v7_universe.go`
- `provider/local/hunter_v7_router.go`
- `kernel/engine.go`
- `kernel/engine_prompt.go`

新增并贯通：

- `signal_id`
- `data_freshness`
- `stale_data_risk`
- `fresh_micro_confirmed`
- `fresh_rest_confirmed`

LLM 开仓时必须输出 `selected_hunter_v7_signal_id`，后端会按 signal id、symbol、direction、setup、tier 做结构化匹配。

### 3. Range expansion 子形态标签

文件：

- `provider/local/hunter_v7_mod_range_expansion_event.go`
- `provider/local/hunter_v7_tag_catalog.go`
- `provider/local/hunter_v7_mod_range_expansion_event_test.go`

保留历史 setup `range_expansion_event`，新增子形态标签：

- `range_expansion_continuation`
- `range_expansion_retest`
- `retest_confirmed`
- `range_expansion_exhaustion`
- `range_expansion_late_chase`
- `range_expansion_needs_retest`
- `late_event_chase`
- `velocity_decelerating`
- `micro_reversal_against_signal`

late chase 会降级为 chase risk，exhaustion 会要求 fresh confirmation，避免把事件延续和尾端追单混用一套开仓规则。

### 4. 下单前 micro-refresh

文件：

- `datafetch/fetcher_bulk.go`
- `datafetch/fetcher_per_symbol.go`
- `datafetch/collector.go`
- `kernel/engine_snapshot.go`
- `trader/auto_trader_risk.go`

Hunter v7 高风险开仓前会执行两类复核：

- Orderbook micro-refresh：best bid/ask、spread、可执行价漂移。
- REST 单标的刷新：最新 1m/5m/15m/1h K 线、price、mark price、funding、OI。

阻断规则包括：

- spread 超过 `0.35%`
- 高风险信号可执行价漂移超过 `0.45%`
- REST price 与 live/executable price 偏离超过 `0.45%`
- 最近 1m taker flow 强烈反向翻转
- 最新 1m K 线反向移动超过 `0.50%`
- 高波动或 stale 信号缺少 fresh confirmation

### 5. 后端强制信号合约和风险仓位映射

文件：

- `trader/auto_trader_risk.go`
- `trader/auto_trader_risk_test.go`

后端现在强制校验：

- `selected_hunter_v7_signal_id` 必填并匹配当前候选。
- direction、setup、tier 不得冲突。
- high-volatility stale signal 必须 fresh confirmed。
- required confirmations、RR、entry zone、live guard 仍必须通过。

风险标签会直接影响仓位和杠杆：

| 条件 | 仓位倍率 | 杠杆上限 |
| --- | ---: | ---: |
| `high_volatility` | 0.60x | 15x |
| `high_volatility + moderate_liquidity` | 0.33x | 10x |
| `execution_stop_tightened` | 0.50x | 10x |
| `range_expansion_exhaustion` 或 `velocity_decelerating` | 0.50x | 10x |
| `execution_tier=REVIEWABLE` | 0.50x | 10x |
| `stale_data_risk` 且无 fresh confirmation | 0x | 禁止开仓 |

### 6. TP0 与平仓归因

文件：

- `trader/auto_trader_risk.go`
- `store/position.go`
- `store/position_builder.go`
- `trader/bybit/order_sync.go`

保护器会读取最近 open decision 的计划 take profit；若 mark price 触达计划 TP 且 ROE 为正，会触发 TP0 减仓，并把剩余仓位保护推向保本缓冲。

平仓归因新增：

- pending close intent 保留本地保护器原因。
- `ClosePositionWithAccurateData` 不再轻易覆盖为 `sync`。
- `PositionBuilder.ProcessTradeWithLeverageAndCloseReason` 支持交易所 close reason。
- Bybit 同步可区分 `exchange_take_profit`、`exchange_stop_loss`、`exchange_liquidation`、`exchange_reduce_only`。

### 7. Final RR 审计字段

文件：

- `store/decision.go`
- `trader/auto_trader_orders.go`
- `trader/auto_trader_loop.go`

Decision action 新增：

- `final_rr`
- `final_effective_take_profit`
- `final_stop_loss`

开仓日志使用 `final_rr=`，方便复盘验证真实订单不存在低于 minRR 的开仓。

## 最新 live validation

2026-07-04 04:53:56 CST 运行：

```bash
go run ./cmd/hunter_v7_validate -top-detail 220 -max-output 30 -watch-output 5 -min-priority 45 -aggressive=true -out-dir reports
```

结果：

- snapshot：`symbols=524`，`universe=121`，`regime=trend_up`
- REST errors：`212`
- signals：`total=4`，`long=4`，`short=0`
- setups：`displacement_momentum_long=1`，`trend_breakout_long=2`，`whale_flow_reversal=1`
- runtime tiers：`REJECTED=1`，`WATCH=3`
- prompt-final tiers：`EXECUTABLE=0`，`REVIEWABLE=0`，`WATCH=3`，`REJECTED=1`
- JSON：marshal/unmarshal OK，缺字段 0，execution gaps 0
- issue：`single_direction_output` medium

解读：

- JSON、字段、prompt wait 链路正常。
- 没有 EXECUTABLE/REVIEWABLE，因此 prompt 不展开 `hunter_v7_signal_json` 是正常行为。
- 本轮单边 trend_up 市场只输出 LONG，不是执行阻断问题，但会降低机会多样性。
- `rest_errors=212` 说明该轮 Binance REST 数据覆盖质量不足，后续应重点监控 REST 错误来源和 universe 缩小原因。

针对该轮验证的调整：

- ZECUSDT 这类 trend breakout 若只缺 `5m_or_15m_close_through_breakout_level`，会进入 REVIEWABLE，让 LLM 和后端 micro-refresh 有机会复核开仓。
- MAGMAUSDT 这类 whale flow 若只缺方向性 15m 收盘，也可进入 REVIEWABLE。
- TLMUSDT 这类带 `chase_high_protection`、风险分较高的 displacement 仍不直接放宽，避免为提高开仓率牺牲胜率。

## 验证

已通过：

```bash
go test ./datafetch ./provider/local ./kernel ./trader ./store ./trader/bybit
go test ./...
```

## 剩余事项

1. 将 exchange order type close reason 映射扩展到 Binance、OKX、Bitget、Gate、KuCoin、Lighter、Aster。
2. 为 `final_rr`、signal age、TP0 touch、TP1 touch、MFE、MAE 建立独立统计查询或矩阵表。
3. 追踪 REST errors 对 universe 覆盖率的影响，必要时增加重试、分批权重控制和错误分类。
4. 按 setup/subtype/tag 聚合 24h/7d 胜率、TP0 realized rate、hard loss rate，用结果反向校准 REVIEWABLE/EXECUTABLE 阈值。
