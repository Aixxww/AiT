# Hunter v7 MMS-Router 三路强控盘筛选实施方案

日期：2026-07-18  
目标：将 MMS-Router 的三个筛选路由融入 AiT 数据源 `hunter_v7` 筛选模式，使 Hunter v7 能覆盖币安合约中“小盘/高控盘/强动量”标的在不同生命周期下的入场机会，并让交易引擎 LLM 能稳定识别标签、确认条件和禁止动作。

## 1. 设计校正

原方案的“MarketCap < 50M”不能直接落地到 Binance Futures 公共数据源，因为 `/fapi` 不提供实时市值。实施时使用 Binance-native 小盘代理指标：

- 非核心大币：排除 `BTC/ETH/BNB/SOL/XRP/DOGE/ADA/TRX/AVAX/LINK/DOT/MATIC/LTC/BCH/UNI/ETC/ATOM/FIL/NEAR/APT/ARB/OP/SUI` 等核心流动性标的的 MMS 专用小盘门槛。
- 24h quote volume：优先 `3M - 80M`，低于 `3M` 视为流动性不足，高于 `120M` 视为大盘/大众拥挤，不再按 MMS 小盘处理。
- OI notional：优先 `300k - 20M`；过低无法执行，过高说明已经大众化或过度拥挤。
- 交易活跃度：`TradeCount24h >= 20k` 或短周期成交量 burst 达标，避免死盘。

MMS 只做 “可追踪的控盘行为”，不把主观叙事写进规则。所有路由都必须输出结构化 `reason_codes`、`risk_tags`、`required_confirmations`，由 Hunter v7 tier 漏斗和执行 guard 二次审验。

## 2. 数据源映射

| 需求 | Binance / AiT 可用字段 | 实施字段 |
|---|---|---|
| 1m / 15m / 1h / 4h K线 | `/fapi/v1/klines` | `Klines["1m","15m","1h","4h"]` |
| 成交量与 taker | K线 `volume`, `takerBuyBaseVolume` | `VolumeBurst15m`, `VolumeBurst1h`, `TakerBuy15m` |
| OI 当前值 | `/fapi/v1/openInterest` | `Snapshot.OI` 转 notional |
| OI 变化 | `/futures/data/openInterestHist` | `OIDelta1h`, `OIDelta4h` |
| 大户多空比 | `/futures/data/topLongShortPositionRatio` | `Snapshot.LSR`, `LSRPrev`, `LSROldest` |
| 全市场用户多空比 | 暂未入库 | 第一阶段不阻塞，使用 `LSR + taker + OI/price` 代理；后续可扩展 `globalLongShortUserRatio` |
| 市值 | Binance 不提供 | 使用 quote volume、OI notional、核心币排除作为代理 |

需要新增到 `V7SymbolContext` 的派生字段：

- `EMA7_15m`, `EMA25_15m`, `EMA99_15m`
- `EMA25_1h`
- `StdRatio1h72`
- `VolumeBurst1h`
- `Last15mLow`, `Last15mClose`
- `TopLongShortRatio` 可直接复用 `Snapshot.LSR`

## 3. 三个 MMS 路由

### Route A: Bottom Wake

定位：暗涌底吸 / 横盘吸筹。  
对应 setup：`mms_bottom_wake_long`。  
输出倾向：`REVIEWABLE` 为主，只有确认突破或实时 reclaim 才能 open。

准入条件：

- 小盘代理通过：非核心大币，`QuoteVolume24h` 与 `OI notional` 在可执行区间。
- 72 根 1h 收盘价压缩：`StdRatio1h72 < 0.025`，优选 `<0.020`。
- 成交量唤醒：`VolumeBurst1h >= 2.5`，优选 `>=3.0`。
- OI 潜伏增持：`OIDelta4h >= 12%`，优选 `>=15%`。
- 价格未明显启动：`-2.5% < Change4h < 6%`。
- taker 不可明显卖压：`TakerBuy15m >= 0.48`。

核心标签：

- `mms_bottom_wake`
- `mms_quiet_accumulation`
- `mms_oi_stealth_inflow`
- `mms_volume_wake`
- `mms_small_cap_proxy`

风险标签：

- `mms_liquidity_too_low`：reject-only
- `mms_breakout_not_confirmed`：reviewable-only

确认条件：

- `5m_or_15m_close_through_breakout_level`
- `oi_or_volume_expands_with_price`
- `live_price_in_entry_zone`

### Route B: Trend Ride

定位：主升趋势中缩量回踩生命线。  
对应 setup：`mms_trend_ride_long`。  
输出倾向：可进入 `EXECUTABLE`，但必须是缩量回踩后收回 EMA25，而不是高位直接追。

准入条件：

- 15m 风扇均线：`EMA7_15m > EMA25_15m > EMA99_15m`。
- EMA25 上行：`EMA25_15m > EMA25_15m(prev)`，第一阶段用 `EMA25_15m > EMA99_15m` 与 1h/4h 正动量代理。
- 回踩 EMA25：`Last15mLow <= EMA25_15m * 1.006` 且 `Last15mClose > EMA25_15m`。
- 缩量清洗：`VolumeBurst15m <= 0.85`。
- 趋势未末端过热：`RSI1h < 78`，且若 `entry_zone_position > 70%` 必须降级 wait。
- taker 不反向：`TakerBuy15m >= 0.50`。

核心标签：

- `mms_trend_ride`
- `mms_ema_fan_bullish`
- `mms_ema25_retest_hold`
- `mms_low_volume_retest`

风险标签：

- `mms_ema_retest_not_held`：wait-only
- `mms_trend_ride_chase_risk`：wait-only

确认条件：

- `5m_price_holds_ema20_or_trailing_support`
- `taker_flow_not_flipping_against_direction`
- `live_price_in_entry_zone`

### Route C: Squeeze Engine

定位：散户恐高做空、大户锁仓推动的顺势轧空。  
对应 setup：`mms_squeeze_engine_long`。  
输出倾向：同时提供 long 机会和 short 禁令。该路由不是左侧摸顶信号。

准入条件：

- 大户多头优势：`TopLongShortRatio >= 1.55`，优选 `>=1.60`。
- 价格与 OI 同步上行：`OIDelta1h >= 8%` 且 `Change1h >= 2.5%`，优选 `10% / 3%`。
- 买盘或轧空结构：`TakerBuy15m >= 0.54` 或 `VolumeBurst15m >= 1.3`。
- 非低流动性：`QuoteVolume24h >= 5M`。
- 禁止做空：若 `TopLongShortRatio > 1.35` 且 `1h close` 未跌破 `EMA25_1h`，任何 short 应被 wait-only 标签拦截。

核心标签：

- `mms_squeeze_engine`
- `mms_top_trader_long_lock`
- `mms_oi_price_squeeze_fuel`
- `mms_short_ban_active`

标签/风控语义：

- `mms_short_ban_active`：reason_code / context-only，用于告诉 LLM 和执行 guard：同 symbol 不得反手开空，但不阻断本路由的顺势多复核。
- `mms_do_not_short_squeeze`：wait-only，保留给显式空头禁令场景；一旦出现在同 symbol 候选，LLM 和执行 guard 均禁止开空。
- `mms_squeeze_late_chase`：reduce-size-or-wait。

确认条件：

- `5m_or_15m_close_above_trigger`
- `taker_buy_15m_gt_0_52`
- `oi_delta_1h_positive_or_quote_volume_expands`

风向逆转做空阈值：

- 只有当 `1h close < EMA25_1h` 且 `TopLongShortRatio < 1.10`，才允许其他 short 模块恢复做空复核。

## 4. 集成点

1. `provider/local/hunter_v7_types.go`
   - 新增三个 setup type。
   - 新增 context 派生字段。

2. `provider/local/hunter_v7_universe.go`
   - 计算 EMA7/25/99、EMA25_1h、StdRatio1h72、VolumeBurst1h、Last15mLow/Close。
   - 保持 Binance-native 数据，不引入不可用 market cap。

3. `provider/local/hunter_v7_mod_mms.go`
   - 新增三个 V7 module：Bottom Wake、Trend Ride、Squeeze Engine。

4. `provider/local/hunter_v7_router.go`
   - 注册三个 MMS modules。

5. `provider/local/hunter_v7_weights.go`
   - 按 regime 配置权重：
     - compression/range/rotation 强化 Bottom Wake；
     - trend_up/rotation 强化 Trend Ride；
     - mania/rotation/trend_up 强化 Squeeze Engine；
     - trend_down/panic 降低三个 long 路由。

6. `provider/local/hunter_v7_tag_catalog.go`
   - 新增 MMS 标签语义，确保 LLM 不把 unknown tag 当开仓许可。

7. `kernel/engine.go`
   - 三个 setup 纳入 tier 分类。
   - `mms_do_not_short_squeeze` 出现时，禁止同 symbol short 进入 open-review。

8. `kernel/engine_prompt.go`
   - Hunter v7 多形态剧本加入 MMS 三路。

9. `trader/auto_trader_risk.go`
   - 下单前 guard 阻断 wait-only/reject-only 标签已存在；补 MMS short-ban 特例，防止 LLM 对同 symbol 开空。

## 5. 验收标准

必须通过：

```bash
go test ./provider/local ./kernel ./trader
go run ./cmd/hunter_v7_validate -rounds=1 -top-detail=260 -max-workers=6 -max-output=80 -watch-output=10 -min-priority=35 -aggressive=true -out-dir=reports/mms-v7-validate-20260718
```

预期：

- 单测可合成命中 `mms_bottom_wake_long` / `mms_trend_ride_long` / `mms_squeeze_engine_long`，并验证 tier 分类与同 symbol short-ban。
- live validation 在行情满足 MMS 生命周期时可看到对应 setup 输出；若当轮市场为单边下跌或无小盘控盘结构，MMS 多头不强行输出，避免为提高开仓率而制造低胜率信号。
- Prompt 中 `hunter_v7_signal_json` 带完整 `tag_semantics`。
- `mms_do_not_short_squeeze` 为 wait-only，LLM 或执行 guard 均不能绕过开空禁令。

## 6. 实施落地记录

实施日期：2026-07-18。

已完成：

- `datafetch/types.go`：15m K线拉取深度从 50 提升到 120，支撑 EMA99_15m。
- `provider/local/hunter_v7_types.go`：新增三个 MMS setup 与 15m/1h 派生字段。
- `provider/local/hunter_v7_universe.go`：新增 EMA7/25/99、EMA25_1h、72h 标准差压缩、1h 成交量 burst、15m 最新 low/close。
- `provider/local/hunter_v7_mod_mms.go`：新增 Bottom Wake、Trend Ride、Squeeze Engine 三个模块；结构止损只在距离不超过 3.5% 时采用，目标距离封顶 7.5%，避免被后端 TP/RR 几何误杀。
- `provider/local/hunter_v7_router.go` / `hunter_v7_weights.go`：完成模块注册与 regime 权重。
- `provider/local/hunter_v7_tag_catalog.go`：新增 MMS reason/risk/confirmation 标签语义；`mms_short_ban_active` 为 context-only，`mms_do_not_short_squeeze` 为 wait-only。
- `kernel/engine.go` / `engine_prompt.go`：新增 MMS tier 分类、LLM 执行剧本、同 symbol squeeze short-ban 识别。
- `trader/auto_trader_risk.go`：下单前阻断 WATCH/REJECTED、wait-only/reject-only 风险标签、confirmation_summary 未通过候选，并加入 MMS squeeze 同 symbol 空单禁令。
- 测试覆盖：新增 MMS 三模块命中测试、tier 分层测试、交易 guard short-ban 测试。

验证结果：

```bash
go test ./provider/local ./kernel ./trader
# ok

go run ./cmd/hunter_v7_validate -rounds=1 -top-detail=260 -max-workers=6 -max-output=80 -watch-output=10 -min-priority=35 -aggressive=true -out-dir=reports/mms-v7-validate-20260718
# 2026-07-18 04:09 CST: symbols=524, universe=233, signals=20, EXECUTABLE=2, WATCH=13, REJECTED=5
# 本轮 regime=rotation，输出 LONG=0 / SHORT=20，setup=breakdown_momentum_short + funding_reversal。
```

本轮实时数据没有 MMS 多头命中，主要原因是市场当轮输出为单方向空头机会：`breakdown_momentum_short` 与 `funding_reversal` 占满可执行/观察候选。该结果符合 MMS 设计哲学：三条路由只捕捉小盘控盘吸筹、主升缩量回踩、顺势轧空，不在缺少多头结构时强造多单。
