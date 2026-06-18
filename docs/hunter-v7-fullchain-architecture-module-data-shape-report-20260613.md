# Hunter v7 全链路架构、模块、指标、标签与适用行情形态报告

> 日期：2026-06-13  
> 范围：数据源猎手 Hunter v7 从行情数据、Universe 构建、setup 模块、路由评分、AIT prompt、执行风控到实时测试反馈的完整链路。  
> 依据：当前代码实现、`docs/` 既有方案、2026-06-12 八轮实时测试与 20:49 CST 改造后复测。

## 1. 总体定位

Hunter v7 是一个多形态、多市场状态的候选标的发现与执行分层系统。它不是单一打分器，而是把实时行情拆成不同交易形态，再通过统一路由、风险评分、标签语义和后端执行风控，输出给 AIT 进行最终交易决策。

核心目标：

- 提高发现率：不只看成交量前排，也覆盖暴涨、暴跌、OI 异动、funding 异常、短线速度、冷启动活跃标的。
- 降低误开仓：信号模块只负责发现形态，开仓资格还必须经过 RR、entry zone、止损/止盈方向、流动性、taker flow、prompt tier 和 trader 后端风控。
- 让 LLM 可解释：通过 `reason_codes`、`risk_tags`、`required_confirmations`、`tag_semantics` 明确区分证据、风险和必需确认。
- 支持实时校准：通过 live validation 把 `EXECUTABLE / REVIEWABLE / WATCH / REJECTED` 与后续盈利跟踪闭环。

## 2. 交易链路总览

| 层级 | 主要文件 | 输入 | 输出 | 职责 |
|---|---|---|---|---|
| 数据快照 | `datafetch/*` | Binance futures ticker / premium / OI / LSR / taker / K线 | `datafetch.Snapshot` | 拉取原始行情，不做 setup 判断 |
| Universe | `provider/local/hunter_v7_universe.go` | Snapshot | `[]V7SymbolContext` | 多维候选池、技术指标、衍生品上下文 |
| Regime | `provider/local/hunter_v7_regime.go` | BTC/ETH + alt 表现 | `V7MarketRegime` | 判断大盘状态，给模块权重 |
| Setup modules | `provider/local/hunter_v7_mod_*.go` | `V7SymbolContext` + regime | `V7SignalOutput` | 按形态 Match/Score，生成 entry/stop/target 和标签 |
| Router | `provider/local/hunter_v7_router.go` | 全部模块输出 | raw / confirmed / watch signals | regime 加权、risk/liquidity、priority、冲突处理、输出裁剪 |
| Execution readiness | `provider/local/hunter_v7_execution.go`, `hunter_v7_readiness.go` | signal + live ctx | quality/readiness/confirm summary | RR、entry zone、执行窗口、缺字段和确认状态 |
| Kernel tier | `kernel/engine.go` | CandidateCoin | `EXECUTABLE/REVIEWABLE/WATCH/REJECTED` | 给 AIT 分层，拒绝危险标签和无效几何 |
| Prompt | `kernel/engine_prompt.go` | 分层候选 + 风控配置 | AIT prompt | 强制 tier 漏斗、blocked_reason、RR/SL/TP 规则 |
| Trader hard guard | `trader/auto_trader_risk.go` | AIT decision + live price | open/repair/reject | 后端最终执行校验和轻微几何修复 |
| Report/validation | `cmd/hunter_v7_validate`, `reports/*` | 实时数据 | raw/report/prompt | 记录信号、tier、格式问题、盈亏跟踪基线 |

## 3. 数据源与 Universe 设计

### 3.1 原始数据字段

`SymbolSnapshotData` 把 datafetch 中与 v7 有关的数据压平，避免模块直接依赖外部结构：

| 类别 | 字段 | 用途 |
|---|---|---|
| 价格/成交 | `Price`, `PriceChange24h`, `HighPrice24h`, `LowPrice24h`, `Volume24h`, `QuoteVolume24h`, `TradeCount24h` | 成交额、涨跌幅、24h 振幅、流动性和异常交易判断 |
| 衍生品 | `FundingRate`, `OI`, `OIDelta1h`, `OIDelta4h`, `LSR`, `LSRPrev`, `LSROldest`, `TakerBuy` | funding 拥挤、OI 流入/流出、LSR 多空拥挤、taker 方向 |
| K线派生 | `ATR1h/4h/1d/15m/5m`, `EMA20/60`, `RSI1h`, `ADX1h`, `BBWidth`, `VWAP15m` | stop/target、趋势、区间、压缩、反转和入场区 |
| 活跃度 | `Amplitude24h`, `RangeExpansion1h`, `Velocity5m/15m`, `VolumeBurst5m/15m` | 捕捉 mover、冷启动、爆发前活跃 |

### 3.2 Universe 多维入池逻辑

Hunter v7 不只取成交额前 N，而是合并多个 top list：

| 入池维度 | 规则 | 目的 |
|---|---|---|
| 核心流动性 | BTC/ETH/SOL/BNB/XRP/DOGE/ADA/AVAX/LINK/DOT 常驻 | 保留大盘和高流动性参考 |
| 成交额 | quote volume top 150 | 主流活跃币 |
| 涨幅 | 24h gain top 50 | momentum / distribution / displacement |
| 跌幅 | 24h loss top 50 | panic reversal / squeeze short |
| OI 异动 | `abs(OIDelta1h)` top 50 | 杠杆拥挤、挤压、资金流入 |
| Funding 异常 | `abs(FundingRate)` top 50 | funding reversal |
| Velocity | 5m/15m 速度 top 80 或绝对速度 >=2% | 捕捉短线突然启动 |
| New activity | volume burst top 80 或 burst >=3x | 捕捉冷启动放量 |
| Amplitude | 24h amplitude >=12% | 高波动 mover |
| Range expansion | 最新 1h TR / 20h median TR >=2.2 | displacement |
| 上限 | 最多 350 个 | 控制成本和 prompt 噪声 |

### 3.3 Pool 分类

每个 symbol 被归到主 pool：

| Pool | 触发条件 | 用途 |
|---|---|---|
| `core_liquidity` | 核心币种 | 大盘与稳定候选 |
| `panic` | 24h 跌幅 < -15% | 恐慌反转/长挤压 |
| `hot_alt` | 24h 涨幅 > 12% 或默认 | 动量、派发、热点 |
| `velocity` | 5m/15m 速度绝对值 >=2% | 短线位移 |
| `new_activity` | 5m/15m volume burst >=3x | 冷启动活跃 |
| `squeeze` | `abs(OIDelta1h) > 10` | 杠杆挤压 |
| `funding` | `abs(FundingRate) > 0.0005` | funding reversal |

## 4. 市场 Regime 判定与模块权重

Regime 基于 BTC/ETH 为主，alt 相对强弱为辅。优先级是安全优先：panic > pullback > mania > trend > compression > range > rotation > mixed。

| Regime | 主要判断 | 最适合模块 |
|---|---|---|
| `panic_dump` | BTC/ETH 24h < -8% 或 BTC 1h < -5% | `panic_reversal_long`, `short_squeeze_long`, `funding_reversal`, `long_squeeze_short` |
| `market_pullback` | BTC/ETH 同步 -5%~-8%，或 BTC -5% 且 ETH <-3% | `panic_reversal_long`, `long_squeeze_short`, `funding_reversal`, `accumulation_breakout_long` |
| `mania_pump` | BTC/ETH 24h >8% 且 4h >3% | `leader_momentum_long`, `funding_reversal`, `distribution_short`, `displacement_momentum_long` |
| `trend_up` | 4h EMA20>EMA60，24h 正，ADX 或双币确认 | `trend_breakout_long`, `leader_momentum_long`, `pullback_reversal_long`, `displacement_momentum_long` |
| `trend_down` | 4h EMA20<EMA60，24h 负，ADX 或双币确认 | `panic_reversal_long`, `distribution_short`, `long_squeeze_short`, `funding_reversal` |
| `compression` | BTC/ETH 均低 ADX 且 BB width 有效 | `trend_breakout_long`, `accumulation_breakout_long`, `displacement_momentum_long` |
| `range` | ADX<20 且 24h 波动 <5% | `range_reversion`, `funding_reversal`, `accumulation_breakout_long` |
| `rotation` | BTC/ETH 中性，10+ alt 跑赢基准 8% | `leader_momentum_long`, `displacement_momentum_long`, `distribution_short`, `funding_reversal` |
| `mixed` | 无清晰状态 | 默认权重 |

权重机制：

- `setup_score *= regime_weight`。
- `regime_fit_score = weight * 67`。
- 若 symbol 4h 相对 BTC/ETH 超额 >6%、流动性 >=50、taker buy >=0.50，可触发 `strong_symbol_regime_override`，把被压低的权重至少提升到 0.8。

## 5. 核心类型与字段说明

### 5.1 `V7SignalOutput`

| 字段 | 含义 |
|---|---|
| `symbol`, `direction`, `setup_type`, `status` | 基础标识 |
| `setup_score` | 形态本身强度 |
| `timing_score` | 当前是否处于可执行窗口 |
| `risk_score` / `risk_level` | 风险评分和 LOW/MEDIUM/HIGH/EXTREME |
| `liquidity_score` | 成交额、OI、交易笔数、OI/volume 健康度 |
| `regime_fit_score` | 当前大盘状态对形态的支持度 |
| `ai_priority` | 最终排序分 |
| `entry_mode` | 入场方式：immediate、wait_confirm、breakout、momentum trailing 等 |
| `execution_quality` | ready / near_confirm / watch_only / chase_risk / invalid_rr |
| `entry_zone` | 建议入场区间 |
| `invalidation` | 形态失效/止损参考 |
| `targets` | 止盈目标列表 |
| `reason_codes` | 正向或中性证据 |
| `risk_tags` | 风险、限制或硬拒绝标签 |
| `required_confirmations` | 不能省略的开仓前确认 |
| `confirm_summary` | 确认检查结果 |
| `execution_readiness` | raw readiness tier、ready score、窗口健康度、缺字段 |

### 5.2 分数公式

默认 `AIPriority`：

```text
setup_score * 0.35
+ timing_score * 0.20
+ regime_fit_score * 0.20
+ liquidity_score * 0.15
- risk_score * 0.10
+ setup_expectancy_bonus
+ execution_quality_bonus
```

Aggressive 模式：

```text
setup_score * 0.40
+ timing_score * 0.25
+ regime_fit_score * 0.15
+ liquidity_score * 0.10
- risk_score * 0.10
```

### 5.3 风险评分

| 风险项 | 加分 | 标签 |
|---|---:|---|
| 24h 绝对波动 >50% | +30 | `extreme_volatility` |
| 24h 绝对波动 >25% | +15 | `high_volatility` |
| 24h 涨幅 >60% | +20 | `extended_24h_gain`, `do_not_market_chase` |
| funding 绝对值 >0.001 | +30 | `funding_extreme` |
| funding 绝对值 >0.0005 | +15 | `funding_elevated` |
| LSR >2.5 或 <0.4 | +15 | `crowding_extreme` |
| LSR >2.0 或 <0.6 | +8 | `crowding_elevated` |
| 疑似刷量 | +25 | `wash_volume_high` |
| quote volume <5M | +15 | `low_liquidity` |
| quote volume <10M | +8 | `moderate_liquidity` |
| 逆大盘方向 | +15 | `regime_against_direction` |
| OI 1h 异动 >30 | +15 | `oi_anomaly` |

风险等级：

- `LOW`: <=30
- `MEDIUM`: <=55
- `HIGH`: <=75
- `EXTREME`: >75

### 5.4 流动性评分

| 项 | 评分 |
|---|---:|
| quote volume >50M / >20M / >10M / else | 35 / 25 / 15 / 5 |
| OI >5M / >1M / >0.5M | 30 / 20 / 10 |
| trade count >100k / >10k / else | 15 / 10 / 5 |
| OI/quote volume 在 0.01~0.5 | +20，否则 +5 |

流动性 <30 会被 router 过滤，prompt review 通常要求 >=50。

## 6. Setup 模块与适用行情形态

### 6.1 模块总览

| Setup | 方向 | Entry mode | 核心行情形态 | 主要门槛 | 最适用 Regime |
|---|---|---|---|---|---|
| `pullback_reversal_long` | LONG | wait_confirm | 上升趋势中的健康回调 | 24h -3%~-18%，接近 4h/1d 支撑，趋势仍在 | trend_up, range, pullback |
| `short_squeeze_long` | LONG | fast_confirm | 空头被挤压的急拉 | 1h >3%，OI 1h <-3，taker buy >0.55 | panic_dump 后、pullback 反弹 |
| `trend_breakout_long` | LONG | breakout_or_pullback | 低波动压缩后的突破 | BBWidthPercentile <25，接近/突破 BB 上轨或高点，OI/taker/volume 配合 | compression, trend_up, mania |
| `leader_momentum_long` | LONG | momentum_with_trailing_stop | 强势龙头延续或浅回踩 | 24h 12%~60%，4h >6%，taker 不弱，OI 健康 | rotation, trend_up, mania |
| `panic_reversal_long` | LONG | wait_reclaim | 恐慌下跌后的 V 型修复 | 24h -15%~-45%，OI flush、reclaim、taker buy 恢复 | panic_dump, trend_down, pullback |
| `accumulation_breakout_long` | LONG | wait_breakout | 压缩吸筹后突破 | BBWidthPercentile <25，OI 4h +8%~30%，24h 涨幅不过热 | compression, range, pullback |
| `distribution_short` | SHORT | wait_reject | 暴涨后的派发/顶部回落 | 24h >20%，OI/funding/LSR 拥挤，taker sell 或动能停滞 | mania, trend_down, rotation |
| `long_squeeze_short` | SHORT | fast_confirm | 多头踩踏下跌 | 24h >0 后 1h <-3，OI 1h <-3，taker buy 不恢复 | trend_down, pullback, panic_dump |
| `range_reversion` | LONG/SHORT | range_edge_only | 区间上下沿回归 | ADX 低，BB 宽度中性，靠近 1h range 边界，RSI/taker 确认 | range, compression |
| `funding_reversal` | LONG/SHORT | wait_price_reversal | funding/LSR 拥挤反转 | funding >0.0003 或 <-0.0003，LSR 极端，价格停滞/反向 | range, mania, pullback, panic |
| `displacement_momentum_long` | LONG | momentum_with_trailing_stop / wait_confirm | 1h 大位移 + 买盘/OI 配合 | RangeExpansion1h >=2，1h 正涨，OI/taker 支持，funding 不极端 | rotation, compression, trend_up, mania |
| Watch modules | LONG/SHORT | wait_* | 爆发前观察 | pre_breakout、accumulation、pre_squeeze、pre_distribution | 用于提前跟踪，不直接开仓 |

### 6.2 模块细节

#### Pullback Reversal Long

- 适用：趋势未破、短线回调到支撑，等待买盘恢复。
- 关键证据：`near_4h_support`, `near_1d_support`, `uptrend_intact`, `rsi_oversold`, `oi_stable`, `taker_buy_recovering`。
- 风险：如果趋势结构不在、taker 不恢复，容易变成下跌中继。
- Entry/exit：等待确认；stop 在 4h low 附近，target 指向 VWAP 或 ATR 反弹。

#### Short Squeeze Long

- 适用：价格快速上冲、OI 快速下降，说明空头被动回补。
- 关键门槛：`Change1h > 3`, `OIDelta1h < -3`, `TakerBuy15m > 0.55`。
- 风险标签：`already_pumped_24h`, `funding_expensive`, `lsr_extreme_long`。
- 执行：快确认模式，必须防止涨后追多。

#### Trend Breakout Long

- 适用：低波动压缩后向上突破。
- 关键门槛：`BBWidthPercentile < 25`，并结合 BB 上轨、4h high、OI、taker、volume。
- 关键证据：`extreme_compression`, `breakout_attempt`, `confirmed_breakout`, `oi_increasing`, `taker_aggressive_buy`, `clear_air_above`。
- 实测问题：若 backend RR 因 TP cap 或 stop 过远不足，会从 reviewable 降为 WATCH。

#### Leader Momentum Long

- 适用：轮动或趋势中跑赢市场的龙头。
- 关键门槛：24h +12%~+60%，4h >+6%。
- 关键证据：`strong_24h_momentum`, `strong_4h_momentum`, `accelerating_1h`, `oi_healthy_growth`, `taker_sustained_buy`, `micro_pullback`。
- 风控重点：过热、funding 极端、RSI 高、无回踩时不能盲追。
- 2026-06-12 实测：Round 1 `XPLUSUSDT` REVIEWABLE 后方向跟踪到 Round 2 +13.23%、Round 3 +17.04%，说明方向发现有效，但 prompt/live tier 正确避免后续追高。

#### Panic Reversal Long

- 适用：恐慌下跌后出现 OI flush、卖压衰竭和 reclaim。
- 关键门槛：24h -15%~-45%，深跌但不是无结构崩盘。
- 关键证据：`heavy_capitulation`, `oi_heavy_flush`, `strong_reclaim`, `selling_exhaustion`, `taker_buy_aggressive`。
- 执行：必须等 reclaim 和 taker buy 强化；逆 trend_down 时单个 5m EMA 站上不够。

#### Accumulation Breakout Long

- 适用：低波动压缩、OI 稳步流入、taker 中性偏多。
- 关键门槛：`BBWidthPercentile <25`, `OIDelta4h 8~30`, `Change24h <=8`。
- 风险：如果 taker sell 明显，标记 `taker_sell_during_accumulation`，不能当作突破买点。

#### Distribution Short

- 适用：暴涨后顶部派发，尤其 OI/funding/LSR 拥挤且动能停滞。
- 关键门槛：24h >20%，一般需要 OI surge 或极端 crowding。
- 关键证据：`extreme_rally`, `at_4h_high`, `heavy_oi_surge`, `taker_sell_dominant`, `momentum_stalling`。
- 执行：通常要求 `wait_reject`，价格必须靠近 short retest zone。实测中 COAI/TRUMP 多次因 `wait_zone_retest_required` 留在 WATCH。

#### Long Squeeze Short

- 适用：前期上涨后突然下跌，OI 快速下降，多头踩踏。
- 关键门槛：24h 正、1h <-3、OI 1h <-3，taker buy 不得恢复。
- 风险：若 taker buy >0.54 代表卖压衰竭，模块拒绝。

#### Range Reversion

- 适用：低 ADX、波动不极端、靠近 1h range 上下沿。
- Long：靠近 range bottom，RSI<35，taker buy 恢复。
- Short：靠近 range top，RSI>65，taker sell 强。
- 不适合：趋势强、OI 方向性过强、区间被突破。

#### Funding Reversal

- 适用：资金费率/LSR 极端拥挤后的反向交易。
- Short：funding 正且多头拥挤，价格转弱，taker sell 强。
- Long：funding 负或短仓拥挤，价格止跌，taker buy 恢复。
- 关键风险：`oi_building_no_flush` 表示 OI 仍在堆积，没有 flush，不能直接反向开仓。
- 实测结论：funding/distribution shorts 常被 retest zone、OI flush 不足拦住，这是合理保守。

#### Displacement Momentum Long

- 适用：1h true range 相对过去 20h median 明显放大，同时价格向上、OI/taker 支持。
- 关键门槛：`RangeExpansion1h >= 2.0`, `Change1h > 0`, `OIDelta1h >=1` 或缺 OI 时要求高 amplitude 与 taker >=0.52，funding <=0.001，taker buy 不低于 0.48。
- 关键证据：`massive_vol_displacement`, `strong_1h_impulse`, `above_vwap_15m`, `oi_confirms_new_demand`, `taker_buy_aggressive`。
- 近期改造：不再只用 T1 判断 RR；使用最佳正向 target，并生成 `displacement_rr_extension`, `displacement_range_expansion_run`, `4h_high_retest` 等目标。
- 实测结果：Round 8 有 7 个 displacement 因 `displacement_rr_insufficient` 被拒；改造后 20:49 CST 复测产生 2 个 displacement EXECUTABLE，REJECTED=0。

### 6.3 Watch 模块

Watch 模块不是直接开仓模块，主要用于提前放入观察池：

| Watch setup | 目标 | 升级条件 |
|---|---|---|
| `pre_breakout_watch` | 压缩接近突破 | 多周期确认、突破 close、taker/OI 配合 |
| `accumulation_watch` | 压缩吸筹未突破 | OI 持续流入、价格守区间 |
| `pre_squeeze_watch` | 挤压前兆 | crowding 与方向触发 |
| `pre_distribution_watch` | 派发前兆 | 高位滞涨、拥挤、卖盘出现 |

Watch 可通过多周期确认升级 REVIEWABLE，但默认不能直接开仓。

## 7. Entry Mode 与默认确认

| Entry mode | 典型 setup | 默认确认 |
|---|---|---|
| `immediate` | 少量强信号 | 无默认确认，但仍过后端风控 |
| `breakout_or_pullback` | trend breakout | 5m/15m close through breakout、OI/volume 扩张、无假突破 |
| `fast_confirm` | squeeze | 5m/15m close 过 trigger、taker 方向确认、不立即丢 trigger |
| `wait_reclaim` | panic/pullback | live price in zone、5m reclaim、taker buy >0.52、无新低 |
| `wait_breakout` | accumulation/pre-breakout | 5m/15m close above entry zone、OI inflow、BB 扩张 |
| `wait_reject` | distribution short | 5m/15m 阻力拒绝、taker sell、无新高 |
| `range_edge_only` | range reversion | range 边界入场、边界反应、range 外止损 |
| `wait_price_reversal` | funding reversal | live price in zone、EMA/zone 反向确认、taker 阈值 |
| `momentum_with_trailing_stop` | leader/displacement | 5m hold EMA/trailing support、momentum not exhausted、taker 不反向 |

## 8. Execution Quality 与 Readiness

### 8.1 Execution Quality

| Quality | 含义 | 典型处理 |
|---|---|---|
| `ready` | RR/几何/确认基础较好 | 可进入 EXECUTABLE 或 REVIEWABLE |
| `near_confirm` | 形态接近，但仍需实时确认 | 常进入 REVIEWABLE |
| `watch_only` | 不适合本轮开仓 | WATCH |
| `chase_risk` | 有追高/追空风险 | 仅极少数低风险可 REVIEWABLE |
| `invalid_rr` | 几何无效 | REJECTED 或 WATCH，不开仓 |

### 8.2 RR 与止损几何

模块先给出 `entry_zone`、`invalidation`、`targets`。Router finalize 会：

- 移除或后移已过期 target。
- 对过宽 stop 进行 near-structure tightening。
- 计算最佳 target 的 RR。
- RR <1.2 标记 `invalid_rr_context_only`。
- RR 1.2~1.5 标记 `thin_rr_wait_confirm`。
- 通常 RR >=1.5 且 timing 达标才可能 ready。

Trader 后端默认 Hunter v7 几何：

- max entry drift 默认 0.5%。
- min stop distance 默认 2.0%。
- max take profit distance 默认 3.0%，但会通过 `HunterV7EffectiveExecutionGeometry` 保证与 min RR 可行。
- min RR 来自配置，实测 prompt 通常要求 >=1.5。
- 后端允许轻微修复 price / stop / take profit，但不会修复已失效方向、无效 stop 或不可达 RR。

### 8.3 Execution Readiness

Readiness 计算：

```text
ready_score =
setup * 0.25
+ timing * 0.20
+ flow * 0.20
+ window_health * 0.15
+ rr_score * 0.10
+ liquidity * 0.05
+ freshness * 0.05
- max(0, risk_score - 35) * 0.35
```

Readiness tier：

- hard missing 字段：`REJECTED`
- `invalid_rr`: `REJECTED`
- `risk_score >=65`: `REJECTED`
- 缺 execution/context 字段：`REVIEWABLE` + partial data
- `ready` 且 ready_score >=70 且 window_health >=60：`EXECUTABLE`
- `ready/near_confirm` 且 ready_score >=55：`REVIEWABLE`
- `chase_risk`: `WATCH`

## 9. Kernel Tier 漏斗

Kernel 把候选分为：

| Tier | 含义 | 开仓资格 |
|---|---|---|
| `EXECUTABLE` | 系统认为可进入开仓评估 | 仍需后端风控和 AIT 输出正确 SL/TP/RR |
| `REVIEWABLE` | 可复核，不自动开 | 必须实时确认 entry zone、flow、RR、stop |
| `WATCH` | 背景观察 | 不开仓 |
| `REJECTED` | 硬拒绝 | 禁止开仓 |

硬拒绝/降级关键规则：

- `risk_score >=65` 不进入 open review。
- 流动性 <50 通常不能 REVIEWABLE。
- `displacement_rr_insufficient`, `risk_filtered`, `liquidity_filtered`, `module_no_match`, `extreme_volatility`, `wash_volume_high` 属于拒绝或强风险语义。
- `oi_building_no_flush` 对 funding reversal 是关键拦截。
- `not_near_*_zone`, `late_*_without_flush`, `momentum_overheated`, `do_not_market_chase` 通常使候选必须 wait。

## 10. 标签语义体系

Hunter v7 标签分三类：

| 类型 | 字段 | 含义 |
|---|---|---|
| 证据 | `reason_codes` | 说明为什么出现该形态 |
| 风险 | `risk_tags` | 限制、降仓、等待或拒绝原因 |
| 必需确认 | `required_confirmations` | 开仓前必须验证的条件 |

LLM action 语义：

| `llm_action` | 行为边界 |
|---|---|
| `supports_open_after_core_checks` | 支持开仓，但必须过 entry/stop/RR/后端 |
| `evidence_only` | 证据，不可单独开仓 |
| `required_confirmation` | 未满足则 wait |
| `reviewable_only_if_live_confirmed` | 仅实时确认后可开 |
| `wait_only` | 当前周期不直接开 |
| `reject_only` | 禁止开 |
| `context_only` | 背景 |
| `reduce_size_or_wait` | 降仓或等待，需更严格确认 |
| `unknown_context_only` | 未定义标签，默认不能作为开仓许可 |

重要标签示例：

| 标签 | 类型 | 语义 |
|---|---|---|
| `taker_buy_aggressive` | reason | 买盘强，可支持 long |
| `strong_reclaim` | reason | 恐慌后强 reclaim |
| `oi_confirms_new_demand` | reason | OI 随价格增长，支持新需求 |
| `displacement_extension_rr_valid` | reason | displacement 后续目标修复 RR |
| `execution_stop_tightened` | risk | stop 被执行层收紧，需确认 RR |
| `displacement_rr_insufficient` | risk | displacement 几何无效，reject-only |
| `oi_building_no_flush` | risk | 反转没有 flush，wait-only |
| `momentum_overheated` | risk | 动量过热，等待回踩 |
| `not_near_short_retest_zone` | risk | 空头不在可入场 retest zone |
| `5m_price_holds_ema20_or_trailing_support` | confirmation | 动量入场必须守住短线支撑 |

## 11. AIT Prompt 与交易决策规则

Prompt 侧强制：

- 严格按 EXECUTABLE → REVIEWABLE → WATCH → REJECTED 评估。
- WATCH 只作背景，REJECTED 不参与开仓。
- `wait` 必须输出 `blocked_reason_code`。
- 只要存在 EXECUTABLE/REVIEWABLE，不能用 `no_reviewable_candidate`。
- 账户回撤只影响仓位、冷却和重复交易，不能成为全局 wait 借口。
- momentum/breakout 若跌破 signal stop、entry zone 下沿或 5m 动量转弱，不能把回落视为更好入场。
- panic reversal 逆势时，单个 5m reclaim 不足以覆盖 failed review。

blocked reason 枚举：

- `entry_not_in_zone`
- `rr_insufficient`
- `confirmation_missing`
- `oi_too_low`
- `funding_crowded`
- `account_risk`
- `backend_guard_risk`
- `no_reviewable_candidate`

## 12. 实时测试结论

### 12.1 2026-06-12 八轮测试摘要

| 轮次 | 时间 CST | 结果 | 主要结论 |
|---:|---|---|---|
| 1 | 05:46 | EXECUTABLE 0 / REVIEWABLE 2 | leader momentum 放宽后 XPLUS、COAI 进入复核 |
| 2 | 11:51 | REVIEWABLE 1 | XPLUS 从 Round 1 信号价跟踪 +13.23%，方向正确但不追高 |
| 3 | 12:30 | REVIEWABLE 1 | XPLUS 跟踪 +17.04%，RR/追高 gate 保持有效 |
| 4 | 12:49 | REVIEWABLE 0 / REJECTED 2 | displacement 因 RR 几何被拒，开始暴露问题 |
| 5 | 13:11 | WATCH 5 | REST errors 132，不用于调参 |
| 6 | 16:22 | REVIEWABLE 1 | low-risk breakout floor 有效但仍偏保守 |
| 7 | 16:34 | REVIEWABLE 0 / REJECTED 2 | GMX displacement 因 RR/低流动性拒绝 |
| 8 | 16:44 | REVIEWABLE 0 / REJECTED 8 | 7 个 displacement long 被 `displacement_rr_insufficient` 阻塞 |

关键发现：

- `leader_momentum_long` 的 REVIEWABLE 条件过重问题已缓解，方向发现有效。
- 新瓶颈转移到 execution geometry，尤其 displacement 原来只看第一目标 T1 导致误杀。
- funding/distribution short 多数等待 retest zone 或 OI flush，属于合理保守。

### 12.2 改造后复测：2026-06-12 20:49 CST

实时结果：

- Snapshot: 521 symbols，universe 220。
- Regime: `rotation`。
- BTC 24h +0.79%，ETH 24h +0.87%。
- Signals: 11，总计 LONG 6 / SHORT 5。
- Tier: `EXECUTABLE=2`, `REVIEWABLE=3`, `WATCH=6`, `REJECTED=0`。
- JSON/prompt OK，issues=0，executable gaps=0。

Top signals：

| Symbol | Direction | Setup | Tier | Price | 说明 |
|---|---|---|---|---:|---|
| KMNOUSDT | LONG | displacement_momentum_long | EXECUTABLE | 0.018800 | 1h 位移、OI 新需求、taker aggressive |
| ENJUSDT | LONG | displacement_momentum_long | EXECUTABLE | 0.032000 | massive displacement，但 funding/RSI 标签要求更谨慎 |
| AINUSDT | LONG | leader_momentum_long | REVIEWABLE | 0.112530 | chase risk，只能等回踩确认 |
| ASTERUSDT | LONG | trend_breakout_long | REVIEWABLE/WATCH summary | n/a | breakout 形态，但 backend RR summary 仍需确认 |
| ICPUSDT | LONG | trend_breakout_long | REVIEWABLE | n/a | 需突破 entry zone 上沿 |

结论：

- displacement geometry 改造有效：Round 8 的 7 个 RR 拦截，在复测中转为 2 个 EXECUTABLE、0 个 REJECTED。
- 这不是降低风控，而是修复模块 stop/target 几何，让后端硬风控有机会评估真实可交易性。
- 后续需要继续跟踪 KMNO/ENJ 的 unleveraged direction PnL、是否触及 stop context，以及是否出现系统性过远 target。

## 13. 适用于筛选的行情形态矩阵

| 行情状态 | 优先筛选 | 次级筛选 | 避免 |
|---|---|---|---|
| 大盘强趋势上涨 | trend_breakout_long, leader_momentum_long, pullback_reversal_long | displacement_momentum_long, short_squeeze_long | distribution_short 逆势裸空 |
| 大盘震荡但 alt 活跃 | leader_momentum_long, displacement_momentum_long | distribution_short, funding_reversal, trend_breakout_long | 低流动性高波动追单 |
| 压缩待突破 | trend_breakout_long, accumulation_breakout_long, pre_breakout_watch | range_reversion, displacement_momentum_long | 过早按 momentum 开仓 |
| 恐慌下跌 | panic_reversal_long, short_squeeze_long, funding_reversal | long_squeeze_short | pullback/trend breakout 抄底过早 |
| 温和回调 | panic_reversal_long, pullback_reversal_long, accumulation_breakout_long | funding_reversal, short_squeeze_long | leader momentum 无回踩追高 |
| 狂热拉升 | leader_momentum_long, displacement_momentum_long | distribution_short, funding_reversal | funding/LSR 极端时追多 |
| 区间震荡 | range_reversion, funding_reversal | accumulation_breakout_long | trend breakout 低确认假突破 |
| 顶部派发 | distribution_short, funding_reversal short | pre_distribution_watch | 不在 retest zone 的追空 |

## 14. 当前架构优点与风险

### 优点

- 多维 Universe 提升了 mover 和冷启动标的发现率。
- setup 模块职责清楚，便于单独调参。
- regime 权重能让同一形态在不同大盘状态下动态降权/升权。
- 标签 catalog 降低 LLM 误读风险。
- readiness/tier/prompt/trader 多层风控避免“发现信号=直接开仓”。
- 实时报告已经能定位瓶颈是发现、分层、几何还是后端风控。

### 风险

- 标签仍有部分历史裸字符串，需持续统计 unknown 高频标签。
- 一些 setup 的 REVIEWABLE 路径仍较复杂，需防止条件堆叠导致过度保守。
- 后端 TP cap 与模块远目标之间需要持续对齐，否则会出现 raw ready 但 backend RR infeasible。
- funding/distribution short 依赖 retest zone 和 OI flush，可能错过单边快速下跌，但这是高胜率取舍。
- 实时测试样本仍有限，需要跨 regime 连续跟踪，而不是只看 rotation 日内样本。

## 15. 后续建议

1. 增加自动统计：每轮输出 `setup_type × tier × blocker` 矩阵。
2. 增加盈利跟踪：对 EXECUTABLE/REVIEWABLE 建立 15m、30m、60m、120m unleveraged PnL 和 stop-threat 记录。
3. 增加 tag unknown 报告：高频 unknown 自动列入 catalog 待补清单。
4. 对 `backend_rr_infeasible` 做 setup 维度归因，区分 stop 过宽、TP cap、live drift、target 过近。
5. funding/distribution short 增加“错过收益”统计，判断 retest zone 是否过严。
6. 按 regime 回测/实测分组，不用 rotation 的结果外推到 panic、trend_down、range。
7. 每次放宽 REVIEWABLE 条件必须配套后续 PnL 和 invalidation 统计，避免只提高信号数量。

