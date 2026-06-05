下面是基于附件报告中对 `hunter.go / Hunter v6 default` 的源码级分析与实盘筛选表现，重新设计的一套 **多市场状态、多形态机会捕捉型选币引擎优化方案**。目标不是单纯寻找“均值回归”或“突破”，而是把选币引擎升级为：

> **面向合约市场的多形态 Alpha Signal Router：在暴涨、暴跌、震荡、单边趋势、回调、反弹、吸筹、派发、挤仓、资金费率博弈等不同市场状态中，识别可交易标的，并把信号结构化输出给 AI 交易决策引擎做二次确认。**

---

# 交易系统选币引擎优化设计方案

## 1. 当前 Hunter v6 的核心问题诊断

### 1.1 当前架构本质

从附件分析看，当前 Hunter v6 default 并不是真正意义上的“均值回归引擎”，而更像是：

> **结构位 + OI + 聪明钱 + 压缩态 的共振评分器**

当前核心公式大致为：

```text
base50 = max(Position, OI) × 0.65 + min(Position, OI) × 0.35
base25 = SmartMoney × 0.80
composite = clamp(base50 + base25, 0, 75)
finalScore = composite × Confirmation × ELP × WashTrade × Cooldown + Squeeze
```

这套逻辑在 **回调企稳、空头陷阱、吸筹末期** 等场景表现较好，但在以下行情中存在明显盲区：

| 行情类型 | 当前表现 | 问题 |
|---|---:|---|
| 强趋势连续拉升 | 弱 | 容易因为远离支撑、接近阻力而不给分 |
| 暴涨后二段拉升 | 弱 | 把所有大涨都视为追涨风险，无法识别龙头加速 |
| 暴跌后 V 反 | 弱 | ELP 对 24h 跌幅惩罚过重，可能错过恐慌反弹 |
| 高位派发做空 | 中弱 | 对拉高出货、资金费率套利识别不足 |
| 震荡区间高抛低吸 | 弱 | 缺少震荡 regime 识别和上下沿策略 |
| 山寨币庄家控盘 | 弱 | 对异常成交、主动买卖盘、盘口薄弱、资金费率扭曲建模不足 |
| 多周期趋势延续 | 弱 | 当前更偏“位置猎手”，不是趋势跟随引擎 |

---

## 2. 新版选币引擎总体架构

建议将 Hunter 从单一评分器升级为 **“市场状态识别 + 多策略信号路由 + 风险过滤 + AI 二次决策输入”** 的四层架构。

```text
┌──────────────────────────────────────────────┐
│ Layer 0: 全市场数据采集                       │
│ Binance Futures 600+ USDT Perp               │
│ K线 / OI / Funding / LSR / Taker / Volume     │
│ 涨跌幅榜 / 成交额榜 / 新币榜 / 异动榜          │
└──────────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────┐
│ Layer 1: 市场 Regime 识别                     │
│ 趋势 / 震荡 / 暴涨 / 暴跌 / 回调 / 反弹 / 压缩 │
└──────────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────┐
│ Layer 2: 多信号策略模块并行评分                │
│ A. 回调反弹 Long                              │
│ B. 空头挤仓 Long                              │
│ C. 趋势突破 Long                              │
│ D. 龙头加速 Long                              │
│ E. 恐慌 V 反 Long                             │
│ F. 吸筹突破 Long                              │
│ G. 高位派发 Short                             │
│ H. 多头挤仓 Short                             │
│ I. 震荡区间高抛低吸                           │
│ J. 资金费率套利/拥挤反转                      │
└──────────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────┐
│ Layer 3: 风险过滤与交易适配                    │
│ 流动性 / 刷量 / 盘口薄弱 / 资金费率极端 / 新币风险│
│ 波动率 / 滑点 / OI异常 / 冷却 / 多信号冲突      │
└──────────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────┐
│ Layer 4: 输出给 AI 交易引擎                    │
│ symbol / direction / setup_type / score       │
│ entry_zone / invalidation / TP / risk_tags    │
└──────────────────────────────────────────────┘
```

---

# 3. 核心设计原则

## 3.1 不再用单一总分判断所有行情

不同形态的交易机会，应该使用不同评分模型。

例如：

- 暴跌反弹不能用趋势突破逻辑。
- 强趋势加速不能用均值回归逻辑。
- 高位派发做空不能用低位支撑做多逻辑。
- 资金费率套利不能只看价格位置。

所以新版引擎应输出：

```json
{
  "symbol": "XXXUSDT",
  "direction": "LONG",
  "setup_type": "panic_reversal_long",
  "setup_score": 82,
  "market_regime": "panic_dump",
  "confidence": "B",
  "risk_level": "HIGH",
  "entry_mode": "wait_confirm",
  "reason_codes": [
    "24h_drop_extreme",
    "volume_capitulation",
    "oi_flush",
    "taker_buy_recovery",
    "near_daily_support"
  ]
}
```

而不是只输出一个 `finalScore`。

---

## 3.2 先识别市场状态，再选择信号模块

当前 Hunter 最大问题之一是：**不管行情状态如何，都用同一把尺子打分。**

新版应先做 Regime Detection。

### 市场状态分类

| Regime | 描述 | 适用策略 |
|---|---|---|
| `trend_up` | 单边上涨 | 趋势跟随、回调做多、龙头加速 |
| `trend_down` | 单边下跌 | 反弹做空、恐慌反转等待确认 |
| `range` | 区间震荡 | 高抛低吸、上下沿均值回归 |
| `panic_dump` | 恐慌暴跌 | V反、清算反弹、反向挤仓 |
| `mania_pump` | 疯狂拉升 | 龙头加速、追踪止盈、派发做空预警 |
| `compression` | 波动压缩 | 突破前夜、吸筹/派发识别 |
| `rotation` | 板块轮动 | 强弱排名、资金流向捕捉 |
| `mixed` | 分化行情 | 单币独立评分，不依赖大盘 |

---

# 4. 新版信号模块设计

下面是建议加入的核心信号模块。每个模块独立评分，最终由 Signal Router 汇总排序。

---

## 模块 A：回调企稳反弹 Long

### 适用场景

上涨趋势中的技术性回调，价格回到 4h/1d 支撑区域，资金开始回流。

### 核心逻辑

```text
趋势仍未破坏
+ 价格回调到结构支撑
+ OI 未继续恐慌下降
+ Taker Buy 开始恢复
+ LSR 从极端空头或低位恢复
= 回调反弹 Long 候选
```

### 筛选条件

| 指标 | 条件 | 说明 |
|---|---|---|
| 24h 跌幅 | -3% ~ -18% | 不能是无波动，也不能是崩盘中 |
| 价格位置 | 距 4h 支撑 ≤ 1.5 ATR | 核心结构位 |
| 1d 趋势 | EMA20 > EMA60 或价格仍在 EMA60 上方 | 趋势未完全破坏 |
| OI 4h | -8% ~ +8% | 不希望 OI 继续爆炸式出逃 |
| Taker Buy | 当前 > 0.52 或 3根均值上升 | 主动买盘恢复 |
| LSR | 从低位回升，或多空比下降后企稳 | 空头力竭或多头回补 |
| 成交量 | 回调缩量，企稳放量 | 健康回调 |

### 评分建议

```text
Position Score       0 ~ 25
Trend Integrity      0 ~ 20
OI Stabilization     0 ~ 15
Taker Recovery       0 ~ 20
LSR Reversal         0 ~ 10
Volume Confirmation  0 ~ 10
Total                0 ~ 100
```

### 入选等级

| 分数 | 动作 |
|---:|---|
| ≥ 80 | 可进入 AI 二次决策，优先级高 |
| 65~80 | 等待 15m/1h 确认 |
| 50~65 | 加入观察池 |
| < 50 | 忽略 |

---

## 模块 B：空头挤仓 Short Squeeze Long

### 适用场景

低位或下跌末端，价格突然上涨，同时 OI 下降，说明空头被迫平仓。

### 核心逻辑

```text
价格上涨
+ OI 快速下降
+ Taker Buy 增强
+ LSR 从空头拥挤区回升
+ 成交量放大
= 空头挤仓 Long
```

### 筛选条件

| 指标 | 条件 |
|---|---|
| 1h 涨幅 | > 3% |
| 4h 涨幅 | > 5% |
| OI 1h | < -3% |
| OI 4h | < -8% |
| Taker Buy | > 0.55 |
| Volume | 当前成交量 > 20期均量 × 1.8 |
| 价格位置 | 最好仍未远离支撑超过 3 ATR |
| Funding | 不应过度正费率，避免多头已拥挤 |

### 风险过滤

如果满足以下条件，应降低 Long 权重：

| 风险 | 降权 |
|---|---:|
| 24h 已涨 > 25% | ×0.6 |
| LSR > 2.2 | ×0.5 |
| Funding > 0.05%/8h | ×0.5 |
| 价格接近 1d 阻力 ≤ 1 ATR | ×0.4 |

### 输出标签

```text
short_squeeze_long
oi_flush_up
forced_covering
do_not_chase_if_extended
```

---

## 模块 C：趋势突破 Long

### 适用场景

震荡压缩后向上突破，适合捕捉新一轮行情启动。

### 核心逻辑

```text
波动压缩
+ 价格突破箱体上沿
+ OI 上升
+ Taker Buy 强
+ 成交量放大
= 趋势突破 Long
```

### 筛选条件

| 指标 | 条件 |
|---|---|
| BB Width | 低于过去 20日分位数 25% |
| 价格 | 突破 20/50周期高点 |
| Volume | > 均量 × 2 |
| OI 1h | > +3% |
| OI 4h | > +8% |
| Taker Buy | > 0.56 |
| Funding | 不超过极端正费率 |
| 回踩 | 突破后不快速跌回箱体 |

### 评分结构

```text
Compression Score    0 ~ 20
Breakout Strength    0 ~ 25
OI Confirmation      0 ~ 20
Taker Confirmation   0 ~ 15
Volume Expansion     0 ~ 10
Resistance Clearance 0 ~ 10
Total                0 ~ 100
```

### 特别优化

当前 Hunter 中 Squeeze 加分是后置独立加分，建议升级为完整模块，而不是附加项。因为压缩突破本身就是一类独立交易机会。

---

## 模块 D：龙头加速 Long

### 适用场景

币安每日都有涨幅 20%~50% 甚至 100%+ 的山寨币。当前 Hunter 容易因“涨幅过大”直接惩罚，但实际上强势币经常会有二段、三段加速。

该模块专门捕捉：

> **强势龙头、板块资金抱团、机构/庄家持续拉升阶段。**

### 核心逻辑

```text
24h 强涨
+ 成交额排名快速上升
+ OI 同步增加但未极端拥挤
+ Taker Buy 持续强
+ 回调浅
+ 相对 BTC/ETH 强
= 龙头加速 Long
```

### 筛选条件

| 指标 | 条件 |
|---|---|
| 24h 涨幅 | 12% ~ 60% |
| 4h 涨幅 | > 6% |
| 1h 回撤 | 不超过最近涨幅的 38.2% |
| Volume | 24h 成交额进入 Top 100 或较昨日放大 3倍 |
| OI 4h | +8% ~ +40% |
| Taker Buy | 3根均值 > 0.56 |
| Funding | < 0.08%/8h |
| LSR | < 2.5，避免过度拥挤 |
| 相对强度 | 对 BTC、ETH 均显著强势 |

### 关键过滤

不能简单追所有大涨币，必须排除：

| 排除条件 | 原因 |
|---|---|
| 24h 涨幅 > 80% 且 OI 暴增 > 80% | 极可能末端诱多 |
| Taker Buy 下降但价格创新高 | 派发背离 |
| Funding 极端正 | 多头拥挤 |
| 盘口深度过薄 | 容易插针 |
| 5m 连续长上影 | 高位出货 |

### 输出标签

```text
leader_momentum_long
relative_strength_alpha
trend_acceleration
high_beta_risk
```

---

## 模块 E：恐慌 V 反 Long

### 适用场景

暴跌、清算、插针之后出现 V 型反弹机会。

当前 Hunter 的 ELP 对 24h 跌幅 >20% 几乎灭杀，这对保命是合理的，但会错过优质恐慌反弹。因此建议把 ELP 从“硬杀”改成“分流”：

> 暴跌币不进入普通 Long 模块，而是进入专门的 Panic Reversal 模块。

### 核心逻辑

```text
极端下跌
+ 清算成交量爆发
+ OI 大幅下降
+ 价格收回关键插针低点
+ Taker Buy 从极低恢复
= 恐慌 V 反 Long
```

### 筛选条件

| 指标 | 条件 |
|---|---|
| 24h 跌幅 | -15% ~ -45% |
| 1h 跌幅 | 曾经 < -8% |
| 插针 | 最低价偏离收盘价 > 2 ATR |
| OI | 1h/4h 大幅下降，说明杠杆被清洗 |
| Volume | 放量 > 均量 × 3 |
| Taker Buy | 从 <0.42 回升到 >0.50 |
| 价格 | 收回 15m EMA20 或 VWAP |
| BTC 环境 | BTC 不再继续破低，或 BTC 企稳 |

### 禁止入场条件

| 条件 | 原因 |
|---|---|
| 价格持续贴近最低点，无收回 | 还在下跌 |
| OI 继续上升但价格继续跌 | 空头加仓，不是清算完成 |
| Taker Buy 长时间 <0.45 | 无承接 |
| 24h 跌幅 >50% 且无重大消息澄清 | 可能归零/黑天鹅 |
| 交易所异常、新币流动性差 | 风险不可控 |

### 评分结构

```text
Capitulation Depth    0 ~ 20
OI Flush              0 ~ 20
Reclaim Strength      0 ~ 25
Taker Recovery        0 ~ 15
Volume Exhaustion     0 ~ 10
Market Stabilization  0 ~ 10
Total                 0 ~ 100
```

---

## 模块 F：吸筹突破 Long

### 适用场景

庄家或机构在低位横盘吸筹，OI 慢慢增加，波动压缩，等待突破。

### 核心逻辑

```text
低位横盘
+ 波动收缩
+ OI 缓慢上升
+ 价格不跌
+ 主动卖盘被吸收
= 吸筹突破前夜
```

### 筛选条件

| 指标 | 条件 |
|---|---|
| 价格区间 | 24h/48h 波动率降低 |
| BB Width | 处于 20日低分位 |
| OI | 12h/24h 缓慢增加 8%~30% |
| 价格 | 未明显上涨，或小幅抬高低点 |
| Taker Buy | 中性偏强 0.50~0.55 |
| Volume | 温和放量，不是爆量 |
| Funding | 中性 |
| LSR | 不极端拥挤 |

### 高质量信号

```text
range_compression
oi_accumulation
higher_lows
absorption
pre_breakout
```

### 适合输出给 AI 的交易建议

```text
entry_mode: wait_breakout_or_pullback
entry_trigger:
  - break range high with volume > 2x
  - OI 1h increase > 3%
  - taker buy > 0.56
invalidation:
  - break range low
  - OI dump with price dump
```

---

## 模块 G：高位派发 Short

### 适用场景

庄家或做市商拉高出货，高位放量滞涨，散户追多，资金费率升高，适合反向做空。

### 核心逻辑

```text
价格大涨后滞涨
+ OI 增加
+ Funding 极端正
+ LSR 极端多
+ Taker Buy 下降
+ 长上影/跌回突破位
= 高位派发 Short
```

### 筛选条件

| 指标 | 条件 |
|---|---|
| 24h 涨幅 | > 20% |
| 价格位置 | 接近 4h/1d 阻力，或远离 EMA20 > 3 ATR |
| OI 4h | +15% 以上 |
| Funding | > 0.05%/8h，越高越危险 |
| LSR | > 2.2 |
| Taker Buy | 从 >0.60 下降至 <0.50 |
| K线 | 长上影、吞没、跌回 VWAP |
| Volume | 放量但价格不再创新高 |

### 做空确认

不能因为大涨就直接做空，必须有“转弱确认”：

```text
1. 15m/1h 跌破上涨趋势线
2. 跌回 VWAP
3. Taker Sell > 0.55
4. OI 继续增加但价格下跌，说明空头/套保进入
5. 或 OI 开始下降且价格下跌，说明多头平仓踩踏
```

### 输出标签

```text
distribution_short
crowded_long_unwind
funding_short_candidate
blowoff_top
```

---

## 模块 H：多头挤仓 Long Squeeze Short

### 适用场景

高位多头拥挤，价格跌破关键位，多头被迫平仓，出现连续瀑布。

### 核心逻辑

```text
价格下跌
+ OI 快速下降
+ Funding 极端正转弱
+ LSR 极端多
+ Taker Sell 强
= 多头挤仓 Short
```

### 筛选条件

| 指标 | 条件 |
|---|---|
| 24h 涨幅背景 | 此前明显上涨 |
| 当前 1h 跌幅 | < -3% |
| OI 1h | < -3% |
| OI 4h | < -8% |
| LSR | > 2.0 |
| Funding | 曾极端正 |
| Taker Sell | > 0.55 |
| 价格 | 跌破 15m/1h 结构支撑 |

### 禁止做空条件

| 条件 | 原因 |
|---|---|
| 跌幅已超过 20% 且 OI 已大幅清洗 | 可能接近反弹 |
| Taker Buy 快速恢复 | 有承接 |
| 价格到达日线支撑 | 盈亏比变差 |

---

## 模块 I：震荡区间高抛低吸

### 适用场景

大盘无方向，个币在固定区间反复波动。

当前 Hunter 对震荡行情几乎沉默，但实际合约市场中，震荡区间可以通过上下沿捕捉短线机会。

### Regime 判断

```text
ADX < 20
+ BB Width 中低
+ 价格多次触碰区间上下沿
+ OI 无趋势性变化
= range regime
```

### Long 条件

| 指标 | 条件 |
|---|---|
| 价格 | 接近区间下沿 ≤ 0.8 ATR |
| RSI | < 35 后回升 |
| Taker Buy | 从低位回升 |
| OI | 无异常下跌 |
| 止损 | 跌破区间下沿 |

### Short 条件

| 指标 | 条件 |
|---|---|
| 价格 | 接近区间上沿 ≤ 0.8 ATR |
| RSI | > 65 后回落 |
| Taker Sell | 增强 |
| Funding | 偏正更好 |
| 止损 | 突破区间上沿 |

### 输出标签

```text
range_long
range_short
mean_reversion
low_trend_environment
```

---

## 模块 J：资金费率/拥挤度反转

### 适用场景

币安合约市场经常出现机构或做市商通过拉盘/砸盘赚取资金费率的行为。资金费率、LSR、OI、价格的组合可以捕捉拥挤反转。

### Short 候选

| 指标 | 条件 |
|---|---|
| Funding | > 0.05%/8h |
| LSR | > 2.2 |
| OI | 持续增加 |
| 价格 | 涨幅大但动能衰减 |
| Taker Buy | 下降 |
| K线 | 高位滞涨或跌回 VWAP |

### Long 候选

| 指标 | 条件 |
|---|---|
| Funding | < -0.03%/8h |
| LSR | < 0.8 |
| OI | 高位但价格不再下跌 |
| Taker Buy | 回升 |
| 价格 | 收回支撑位 |
| K线 | 低位拒绝下跌 |

### 评分结构

```text
Funding Extreme      0 ~ 25
Crowding             0 ~ 25
Price Divergence     0 ~ 20
OI Structure         0 ~ 15
Taker Reversal       0 ~ 15
Total                0 ~ 100
```

---

# 5. 全局市场 Regime 识别设计

新版引擎应先识别全局市场环境，再动态调整各模块权重。

## 5.1 BTC/ETH 大盘状态

| 状态 | 判断 |
|---|---|
| 大盘强势 | BTC/ETH 4h EMA20 > EMA60，且涨跌幅为正 |
| 大盘回调 | BTC/ETH 24h -2% ~ -8%，但未破 1d 关键结构 |
| 大盘恐慌 | BTC/ETH 24h < -8% 或 1h 瀑布 |
| 大盘震荡 | ADX < 20，价格在区间中部 |
| 大盘反弹 | BTC/ETH 从低点反弹 > 2%，Taker Buy 恢复 |

## 5.2 模块权重动态调整

| 市场状态 | 提高权重 | 降低权重 |
|---|---|---|
| 大盘强势 | 龙头加速、趋势突破、回调做多 | 高位做空 |
| 大盘回调 | 回调反弹、吸筹突破 | 盲目追涨 |
| 大盘恐慌 | 恐慌 V 反、清算反弹 | 普通支撑做多 |
| 大盘震荡 | 区间高抛低吸 | 趋势突破 |
| 大盘分化 | 相对强度、板块轮动 | 大盘同步逻辑 |
| 大盘顶部疑似 | 高位派发、资金费率反转 | 龙头追多 |

---

# 6. 新版评分系统设计

## 6.1 从单一分数改为多维评分

建议每个候选输出以下评分：

```json
{
  "symbol": "XXXUSDT",
  "direction": "LONG",
  "setup_type": "leader_momentum_long",
  "setup_score": 84,
  "risk_score": 37,
  "liquidity_score": 78,
  "timing_score": 66,
  "regime_fit_score": 91,
  "ai_priority": 82
}
```

### 分数含义

| 字段 | 含义 |
|---|---|
| `setup_score` | 该形态本身的机会强度 |
| `risk_score` | 风险分，越高越危险 |
| `liquidity_score` | 流动性与滑点质量 |
| `timing_score` | 当前是否适合立刻入场 |
| `regime_fit_score` | 当前大盘环境是否支持该形态 |
| `ai_priority` | 给 AI 交易引擎的综合优先级 |

---

## 6.2 推荐综合排序公式

```text
ai_priority =
  setup_score × 0.35
+ timing_score × 0.20
+ regime_fit_score × 0.20
+ liquidity_score × 0.15
- risk_score × 0.10
```

也可以根据交易风格动态调整：

### 激进模式

```text
setup_score 40%
timing_score 25%
regime_fit 15%
liquidity 10%
risk 10%
```

### 稳健模式

```text
setup_score 30%
timing_score 20%
regime_fit 20%
liquidity 15%
risk 15%
```

---

# 7. 风险过滤系统重构

## 7.1 ELP 不再硬杀所有暴跌币

当前 ELP 对暴跌币几乎直接打死。建议改成：

```text
如果 24h 跌幅 > 20%：
  不进入普通 mean_reversion_long
  转入 panic_reversal_long 模块
```

也就是说：

> **暴跌不是不能交易，而是必须用暴跌专用模型交易。**

## 7.2 涨幅过大也不直接否定

如果 24h 涨幅 > 20%，当前逻辑容易视为追涨风险。新版应分流：

```text
如果强涨 + OI健康 + Taker强 + Funding不极端：
  进入 leader_momentum_long

如果强涨 + OI暴增 + Funding极端 + Taker衰减：
  进入 distribution_short
```

同样的大涨，可能是：

1. 龙头加速；
2. 空头挤仓；
3. 庄家派发；
4. 资金费率诱多。

不能只靠涨幅判断。

---

## 7.3 刷量检测升级

当前刷量检测主要看：

```text
OI/Volume 比例
微交易数量
异常放量
```

建议增加：

| 新增检测 | 说明 |
|---|---|
| 成交额突然暴增但盘口深度不增加 | 可能是假活跃 |
| Taker Buy/Sell 快速来回极端波动 | 可能是对倒 |
| 价格不动但成交量异常 | 洗量 |
| OI 无变化但成交额爆炸 | 高频刷量 |
| 5m 成交额远超 1h 均值但无结构突破 | 诱导交易 |

### 刷量风险等级

| 等级 | 条件 | 处理 |
|---|---|---|
| Low | 无异常 | 正常 |
| Medium | 1个异常 | 降权 20% |
| High | 2个异常 | 降权 50% |
| Extreme | 3个以上异常 | 不输出交易信号，只输出风险提示 |

---

# 8. 候选池扩展方案

当前报告显示候选池只取成交额 Top 50，这会错过很多当日暴涨暴跌的山寨币。

建议候选池改为多源合并：

```text
Candidate Universe =
  Top 150 by 24h quote volume
+ Top 50 by 24h gainers
+ Top 50 by 24h losers
+ Top 50 by volume change
+ Top 50 by OI change
+ Top 50 by funding absolute value
+ Top 30 newly listed / high volatility symbols
```

去重后大约 200~350 个标的，而不是固定 50 个。

## 8.1 候选池分层

| 池子 | 用途 |
|---|---|
| `core_liquidity_pool` | BTC/ETH/SOL/BNB 等主流，适合稳健策略 |
| `hot_alt_pool` | 当日强势山寨，适合龙头加速 |
| `panic_pool` | 当日暴跌，适合 V 反 |
| `squeeze_pool` | OI 异动，适合挤仓 |
| `funding_pool` | 资金费率极端，适合拥挤反转 |
| `new_listing_pool` | 新币高波动，单独高风险处理 |

---

# 9. 输出给 AI 交易引擎的数据结构

建议 Hunter 不再只输出排行榜，而是输出结构化交易上下文。

```json
{
  "timestamp": "2026-06-04T09:06:00+08:00",
  "symbol": "HOMEUSDT",
  "direction": "LONG",
  "setup_type": "panic_reversal_long",
  "market_regime": "market_pullback",
  "setup_score": 76,
  "risk_score": 68,
  "liquidity_score": 52,
  "timing_score": 61,
  "regime_fit_score": 84,
  "ai_priority": 70,
  "entry_mode": "wait_confirm",
  "entry_zone": {
    "lower": 0.038,
    "upper": 0.0395
  },
  "invalidation": {
    "price": 0.0345,
    "reason": "break_daily_low"
  },
  "targets": [
    {
      "price": 0.043,
      "reason": "mean_reversion_vwap"
    },
    {
      "price": 0.045,
      "reason": "prior_supply_zone"
    }
  ],
  "reason_codes": [
    "near_4h_support",
    "lsr_reversal",
    "taker_recovery",
    "high_volatility"
  ],
  "risk_tags": [
    "wash_volume_medium",
    "elp_moderate",
    "small_cap_high_beta"
  ],
  "required_confirmations": [
    "15m_close_above_vwap",
    "taker_buy_gt_0.52",
    "oi_stabilize_next_cycle"
  ]
}
```

---

# 10. 代码架构建议

## 10.1 当前 hunter.go 建议拆分

如果 `hunter.go` 已经超过 1500 行，建议模块化：

```text
/provider/local/hunter/
  hunter.go                    // 主入口
  universe.go                  // 候选池构建
  regime.go                    // 市场状态识别
  indicators.go                // 指标计算
  scoring.go                   // 通用评分接口
  risk.go                      // 风险过滤
  output.go                    // 输出结构
  modules/
    pullback_long.go
    short_squeeze_long.go
    trend_breakout_long.go
    leader_momentum_long.go
    panic_reversal_long.go
    accumulation_breakout.go
    distribution_short.go
    long_squeeze_short.go
    range_reversion.go
    funding_reversal.go
```

---

## 10.2 核心接口设计

```go
type SignalModule interface {
    Name() string
    Direction() Direction
    Match(ctx MarketContext, s SymbolData) bool
    Score(ctx MarketContext, s SymbolData) SignalScore
}
```

```go
type SignalScore struct {
    Symbol          string
    Direction       string
    SetupType       string
    SetupScore      float64
    RiskScore       float64
    LiquidityScore  float64
    TimingScore     float64
    RegimeFitScore  float64
    AIPriority      float64
    ReasonCodes     []string
    RiskTags        []string
    EntryMode       string
    EntryZone       PriceZone
    Invalidation    InvalidationRule
    Targets         []Target
    RequiredConfirm []string
}
```

---

## 10.3 Signal Router

所有模块并行运行，然后由 Router 汇总：

```go
func RouteSignals(ctx MarketContext, data []SymbolData, modules []SignalModule) []SignalScore {
    var results []SignalScore

    for _, s := range data {
        for _, m := range modules {
            if m.Match(ctx, s) {
                score := m.Score(ctx, s)
                score.AIPriority = CalcAIPriority(score)
                if PassRiskFilter(score) {
                    results = append(results, score)
                }
            }
        }
    }

    results = DeduplicateConflicts(results)
    results = SortByAIPriority(results)
    return results
}
```

---

# 11. 多信号冲突处理

同一个币可能同时出现多个信号，例如：

- `leader_momentum_long`
- `distribution_short`
- `funding_reversal_short`

这说明市场处于博弈关键点。不要粗暴选最高分，应输出冲突状态。

## 冲突判断

| 情况 | 处理 |
|---|---|
| Long 分数高，Short 分数低 | 输出 Long |
| Short 分数高，Long 分数低 | 输出 Short |
| Long/Short 都高 | 输出 `conflict_watch` |
| 两边都低 | 忽略 |

### 冲突信号输出

```json
{
  "symbol": "STOUSDT",
  "status": "conflict_watch",
  "long_setup": "short_squeeze_long",
  "long_score": 68,
  "short_setup": "distribution_short",
  "short_score": 74,
  "decision": "wait_for_break_or_reject",
  "trigger_long": "break_high_with_oi_increase",
  "trigger_short": "lose_vwap_with_taker_sell"
}
```

---

# 12. 针对附件案例的新版判断示例

## 12.1 HOMEUSDT

当前 Hunter 识别到支撑位与 LSR 反转，但被 ELP 与刷量风险削弱。新版不应简单丢弃，而应把它从普通回调模块分流到 `panic_reversal_long` 与 `pullback_reversal_long` 的交集观察池。

```text
direction: LONG
primary_setup: panic_reversal_long
secondary_setup: pullback_reversal_long
status: wait_confirm
risk_level: HIGH
entry_mode: conditional
```

必须满足的确认条件：

```text
1. 15m 收回 VWAP 或 15m EMA20。
2. Taker Buy 从 0.50 附近上升到 > 0.52，且至少连续 2 根 15m K 线不回落。
3. OI 4h 不再继续下降超过 -5%，最好进入 -3% ~ +5% 的企稳区间。
4. 价格不再刷新 20日低点，或插针后快速收回。
5. wash 风险不升级为 High/Extreme。
```

失效条件：

```text
1. 跌破 20日低点并且 15m 无收回。
2. OI 继续下降，同时价格继续下跌，说明多头撤退未结束。
3. Taker Buy 再次跌破 0.45，说明无主动承接。
4. 24h 跌幅扩大到 -25% 以下且无插针收回结构。
```

新版输出示例：

```json
{
  "symbol": "HOMEUSDT",
  "direction": "LONG",
  "setup_type": "panic_reversal_long",
  "setup_score": 72,
  "risk_score": 74,
  "liquidity_score": 48,
  "timing_score": 58,
  "regime_fit_score": 80,
  "ai_priority": 62,
  "entry_mode": "wait_confirm",
  "reason_codes": [
    "near_4h_support",
    "large_24h_drop",
    "lsr_reversal",
    "taker_neutral_recovery"
  ],
  "risk_tags": [
    "wash_volume_medium",
    "high_volatility",
    "small_cap_high_beta"
  ],
  "required_confirmations": [
    "15m_close_above_vwap",
    "taker_buy_gt_0_52",
    "oi_stabilize_next_cycle"
  ]
}
```

## 12.2 STOUSDT

当前 Hunter 识别到 OI 下降 + 价格暴涨，属于空头挤仓；但 24h 涨幅过高、LSR 极端偏多，普通 Long 追入的风险明显偏大。新版应输出冲突观察，而不是直接给 Long。

```text
long_setup: short_squeeze_long
short_setup: distribution_short / funding_reversal_short
status: conflict_watch
decision: wait_for_continuation_or_rejection
```

继续做多触发：

```text
1. 突破日内高点后不快速回落。
2. OI 从下降转为温和上升，1h OI > +3%。
3. Taker Buy 重新站上 0.56。
4. Funding 未超过 0.05%/8h。
```

反向做空触发：

```text
1. 15m/1h 跌回 VWAP。
2. Taker Buy 从 >0.55 跌破 0.45。
3. 价格不创新高但 OI 增加，形成高位派发。
4. Funding 升高，LSR 继续 > 2.2。
```

新版输出示例：

```json
{
  "symbol": "STOUSDT",
  "status": "conflict_watch",
  "long_setup": "short_squeeze_long",
  "long_score": 66,
  "short_setup": "distribution_short",
  "short_score": 70,
  "decision": "wait_for_break_or_reject",
  "trigger_long": "new_high_with_oi_rebuild_and_taker_buy",
  "trigger_short": "lose_vwap_with_taker_sell_and_crowded_longs",
  "risk_tags": [
    "extended_24h_gain",
    "crowded_longs",
    "do_not_market_chase"
  ]
}
```

## 12.3 NOKUSDT

NOK 是典型的支撑附近但资金确认不足。新版应归入 `pullback_reversal_long` 观察池，不能进入即时交易池。

```text
setup_type: pullback_reversal_long
status: watch
entry_mode: wait_support_confirmation
```

升级条件：

```text
1. 价格二次测试支撑不破。
2. Taker Buy 回到 > 0.52。
3. LSR 从高位下降后稳定，避免死多头继续踩踏。
4. OI 下降速度放缓，或由负转正。
```

降级条件：

```text
1. 跌破支撑且 OI 同步下降。
2. LSR 仍高但价格继续走低，说明多头被动扛单。
3. 15m 连续 Taker Sell 强。
```

## 12.4 BTC / ETH / SOL

蓝筹币不应与高波动山寨币使用同一套出入场逻辑。它们更适合 `core_liquidity_pool`，输出给 AI 的用途是大盘 regime 判断与低杠杆网格/趋势基准，而不是高 beta 信号。

```text
pool: core_liquidity_pool
setup_type: market_anchor / defensive_pullback
entry_mode: scale_in_or_wait_confirm
risk_level: LOW_TO_MEDIUM
```

建议用于：

```text
1. 判断全市场风险开关。
2. 作为山寨信号的 regime filter。
3. 提供 BTC/ETH 相对强弱基准。
4. 当 BTC/ETH 仍破低时，降低全部山寨 Long 权重。
```

---

# 13. 实现级参数矩阵

本章给出可直接迁移到代码配置的初始参数。所有参数应进入配置文件或策略版本表，避免写死在 `hunter.go`。

## 13.1 候选池参数

| 参数 | 建议值 | 说明 |
|---|---:|---|
| `top_quote_volume_n` | 150 | 替代当前 Top 50，覆盖主流流动性 |
| `top_gainers_n` | 50 | 捕捉龙头加速与派发 |
| `top_losers_n` | 50 | 捕捉恐慌 V 反 |
| `top_volume_change_n` | 50 | 捕捉突然活跃标的 |
| `top_oi_change_n` | 50 | 捕捉挤仓与吸筹 |
| `top_abs_funding_n` | 50 | 捕捉资金费率拥挤 |
| `new_listing_n` | 30 | 新币独立高风险池 |
| `min_quote_volume_24h` | 10,000,000 USDT | 普通池最低成交额 |
| `min_oi_value` | max(100,000, P10) | 沿用动态 OI 门槛 |
| `max_symbols_after_union` | 350 | 控制 API 成本与延迟 |

## 13.2 Regime 参数

| Regime | 硬条件 | 软确认 |
|---|---|---|
| `trend_up` | BTC 或 ETH 4h EMA20 > EMA60，价格在 EMA20 上方 | ADX > 22，回撤浅 |
| `trend_down` | BTC 或 ETH 4h EMA20 < EMA60，价格在 EMA20 下方 | 反弹不过 VWAP/EMA20 |
| `range` | ADX < 20，20期高低点宽度稳定 | 上下沿至少各触碰 2 次 |
| `panic_dump` | BTC 24h < -8% 或 单币 24h < -15% | 放量、OI 清洗、插针 |
| `mania_pump` | 单币 24h > +20%，成交额显著放大 | OI 增加、Taker Buy 强 |
| `compression` | BB Width 低于 20日 25% 分位 | 成交量未萎缩、OI 温升 |
| `rotation` | 10个以上山寨强于 BTC/ETH 超 8% | 板块成交额集中上升 |
| `mixed` | BTC/ETH 横盘但山寨分化 | 涨跌幅榜两端同时活跃 |

## 13.3 方向模块准入门槛

| 模块 | 准入分 | AI 优先级 | 入场模式 |
|---|---:|---:|---|
| `pullback_reversal_long` | 60 | 65 | `wait_confirm` |
| `short_squeeze_long` | 65 | 68 | `fast_confirm` |
| `trend_breakout_long` | 70 | 72 | `breakout_or_pullback` |
| `leader_momentum_long` | 72 | 75 | `momentum_with_trailing_stop` |
| `panic_reversal_long` | 70 | 65 | `wait_reclaim` |
| `accumulation_breakout_long` | 65 | 68 | `wait_breakout` |
| `distribution_short` | 72 | 72 | `wait_reject` |
| `long_squeeze_short` | 68 | 70 | `fast_confirm` |
| `range_reversion` | 62 | 60 | `range_edge_only` |
| `funding_reversal` | 70 | 68 | `wait_price_reversal` |

## 13.4 全局降权矩阵

| 风险条件 | 降权 |
|---|---:|
| `wash_medium` | `ai_priority × 0.85` |
| `wash_high` | `ai_priority × 0.50` |
| `wash_extreme` | 不输出交易信号 |
| `funding_extreme_against_direction` | `setup_score × 0.70` |
| `liquidity_score < 40` | `ai_priority × 0.70` |
| `spread_or_slippage_high` | `timing_score × 0.60` |
| `market_regime_against_direction` | `regime_fit_score <= 45` |
| `conflict_long_short_high` | 转为 `conflict_watch` |
| `cooldown_recent_failed_signal` | `ai_priority × 0.70` |
| `cooldown_repeated_5x_24h` | 暂停 24h |

---

# 14. 风险分、流动性分、时机分设计

## 14.1 Risk Score

`risk_score` 越高代表越危险，不应与 `setup_score` 混在一起。

```text
risk_score =
  volatility_risk
+ funding_risk
+ crowding_risk
+ wash_risk
+ liquidity_risk
+ regime_risk
+ event_risk
```

| 子项 | 评分 |
|---|---:|
| 24h 振幅 > 25% | +15 |
| 24h 振幅 > 50% | +30 |
| Funding 绝对值 > 0.05%/8h | +15 |
| Funding 绝对值 > 0.10%/8h | +30 |
| LSR > 2.5 或 < 0.5 | +15 |
| OI/Volume 异常 | +15~30 |
| 盘口深度不足 | +10~25 |
| BTC regime 与方向相反 | +15 |
| 新币上市不足 7 天 | +20 |

风险等级：

| risk_score | 等级 | 处理 |
|---:|---|---|
| 0~30 | Low | 正常输出 |
| 31~55 | Medium | 降低仓位建议 |
| 56~75 | High | 只允许条件确认 |
| >75 | Extreme | 不给入场，只输出观察 |

## 14.2 Liquidity Score

```text
liquidity_score =
  quote_volume_score
+ oi_depth_score
+ spread_score
+ orderbook_depth_score
- wash_penalty
```

| 指标 | 高分条件 |
|---|---|
| 24h 成交额 | > 50M USDT |
| OI 名义价值 | > 5M USDT |
| 买卖价差 | < 0.05% |
| 订单簿 0.5% 深度 | 足够覆盖计划仓位 20倍 |
| 成交笔均值 | 不过度微小 |

建议硬过滤：

```text
liquidity_score < 35:
  不输出自动交易候选，只输出人工观察。
```

## 14.3 Timing Score

`timing_score` 判断是否适合现在入场，不等于方向是否正确。

高 timing 的条件：

```text
1. 价格已到模块定义的 entry zone。
2. 15m 方向信号已确认。
3. OI/Taker/Funding 至少两个维度同步。
4. 距离失效价不远，盈亏比 >= 1.8。
5. 没有刚刚完成极端单边拉升或瀑布。
```

低 timing 的典型场景：

```text
1. 形态很好，但价格已远离入场区。
2. 挤仓已经完成，继续追入盈亏比差。
3. 派发信号出现，但还没有跌破 VWAP。
4. 吸筹明显，但尚未突破。
```

---

# 15. AI 交易引擎二次决策协议

Hunter 的职责不是直接下单，而是给 AI 交易引擎提供结构化、可审计、可追踪的候选信号。AI 决策层必须知道“为什么入选、还差什么确认、错了在哪里失效”。

## 15.1 必填字段

```json
{
  "version": "hunter_v7_signal_router",
  "timestamp": "2026-06-04T09:06:00+08:00",
  "symbol": "XXXUSDT",
  "pool": "hot_alt_pool",
  "direction": "LONG",
  "status": "candidate",
  "setup_type": "leader_momentum_long",
  "market_regime": "mania_pump",
  "scores": {
    "setup": 84,
    "risk": 42,
    "liquidity": 76,
    "timing": 68,
    "regime_fit": 86,
    "ai_priority": 77
  },
  "price_context": {
    "last": 0.1234,
    "change_1h": 4.8,
    "change_4h": 11.2,
    "change_24h": 32.5,
    "atr_1h": 0.004,
    "atr_4h": 0.011
  },
  "derivatives_context": {
    "oi_value": 18000000,
    "oi_change_1h": 4.2,
    "oi_change_4h": 18.7,
    "funding_rate": 0.0003,
    "lsr_oldest": 1.35,
    "lsr_newest": 1.72,
    "taker_buy_ratio_15m": 0.58
  },
  "trade_plan": {
    "entry_mode": "wait_pullback",
    "entry_zone": [0.118, 0.122],
    "invalidation": 0.112,
    "targets": [0.132, 0.145],
    "required_confirmations": [
      "15m_hold_vwap",
      "oi_1h_positive",
      "taker_buy_gt_0_56"
    ]
  },
  "reason_codes": [
    "relative_strength_alpha",
    "oi_price_aligned",
    "taker_sustained_buying",
    "volume_expansion"
  ],
  "risk_tags": [
    "high_beta",
    "extended_24h_gain"
  ]
}
```

## 15.2 AI 决策层推荐规则

| Hunter 输出 | AI 动作 |
|---|---|
| `status=candidate` 且 `ai_priority >= 75` | 进入深度策略分析 |
| `status=wait_confirm` | 只生成条件单计划，不直接入场 |
| `status=conflict_watch` | 输出多空触发条件，不下方向结论 |
| `risk_score > 75` | 禁止自动入场 |
| `liquidity_score < 35` | 禁止自动入场 |
| `timing_score < 55` | 等待下一轮 Hunter |
| `required_confirmations` 未满足 | 不下单 |

---

# 16. 代码迁移路线

## 16.1 v7.0：最小可落地版本

目标是在不大改交易执行层的前提下，先把候选池和 setup_type 做起来。

改动：

```text
1. 新增 universe.go，构建多源候选池。
2. 保留当前 HunterCoinScore，但新增 SetupType、RiskScore、AIPriority 字段。
3. 把当前 LONG/SHORT 双向评分包装成 legacy_structural_long/short 模块。
4. 新增 leader_momentum_long、panic_reversal_long、distribution_short 三个高价值模块。
5. 输出 reason_codes、risk_tags、required_confirmations。
```

验收：

```text
1. 候选池从 Top50 扩展到 200~350。
2. 每个输出标的都有 setup_type。
3. 大涨币不再被简单丢弃，而是分流到 momentum 或 distribution。
4. 暴跌币不再被 ELP 直接灭杀，而是分流到 panic_reversal。
```

## 16.2 v7.1：完整 Signal Router

改动：

```text
1. 引入 SignalModule 接口。
2. 模块 A~J 全部独立 Match/Score。
3. 新增 MarketContext 与 SymbolData。
4. Router 汇总、冲突检测、去重排序。
5. Sniffer 改造成其中一个高置信压缩模块，而不是独立后置过滤器。
```

验收：

```text
1. 同一标的可以同时输出 Long/Short setup，但 Router 能识别冲突。
2. 所有模块可单元测试。
3. 不同 regime 下模块权重动态变化。
4. AI 输入不再依赖单一 finalScore。
```

## 16.3 v7.2：回测与自适应参数

改动：

```text
1. 每个 setup_type 独立记录命中率、最大回撤、MFE/MAE。
2. 参数进入配置表，可按市场状态切换。
3. 引入最近 30/90/180 天表现反馈。
4. 对不同池子使用不同准入阈值。
```

验收：

```text
1. 能按 setup_type 输出胜率、盈亏比、平均持仓时间。
2. 能识别近期失效模块并自动降权。
3. 能识别近期强势模块并提高 AI 优先级。
```

---

# 17. 测试与验收指标

## 17.1 单元测试

每个模块至少覆盖：

```text
1. 满足硬条件时 Match 返回 true。
2. 缺失关键确认时 Match 返回 false 或 EntryMode=wait_confirm。
3. 风险条件触发时 risk_score 上升。
4. 方向冲突时 Router 输出 conflict_watch。
5. wash_extreme、liquidity_low 能阻止自动交易候选。
```

## 17.2 回测指标

按 setup_type 分开统计，不允许只看总收益。

| 指标 | 目标 |
|---|---:|
| Top 10 候选 24h MFE | 高于随机池 |
| Top 10 候选 24h MAE | 低于同波动随机池 |
| setup 命中率 | 独立统计 |
| 平均盈亏比 | > 1.3 起步 |
| 最大连续失败次数 | 用于 cooldown |
| 冲突信号后方向选择准确率 | 用于 Router 优化 |

## 17.3 实盘影子运行

上线前建议影子运行 7~14 天：

```text
1. 不下单，只记录 Hunter v6 与 v7 候选差异。
2. 每 3 分钟记录一次 Top 30。
3. 对比 1h/4h/24h 后续表现。
4. 标记 v6 错过但 v7 捕捉到的暴涨/暴跌机会。
5. 标记 v7 捕捉但风险过滤不足的失败案例。
```

关键看板：

```text
candidate_count_by_pool
candidate_count_by_setup_type
avg_ai_priority_by_setup_type
hit_rate_by_setup_type_1h_4h_24h
mfe_mae_by_setup_type
conflict_watch_resolution
wash_filter_blocked_count
liquidity_filter_blocked_count
```

---

# 18. 最终设计结论

新版 Hunter 不应继续被定义为 default 均值回归选币器，而应升级为：

> **面向 Binance USDT 永续合约的多形态 Alpha Signal Router。**

核心变化是四点：

1. **候选池从 Top50 成交额扩展为多源异动池**：成交额、涨幅、跌幅、成交额变化、OI 变化、资金费率、新币全部纳入。
2. **从单一 finalScore 改为 setup_type 多模块评分**：暴涨、暴跌、趋势、震荡、吸筹、派发、挤仓、资金费率拥挤分别用不同模型。
3. **风险控制从硬杀改为分流与降权**：暴跌进入 panic 模块，大涨进入 momentum/distribution 模块，刷量和流动性才是真正硬过滤。
4. **输出从排行榜改为 AI 可决策上下文**：不仅告诉 AI “哪个币分高”，还告诉 AI “是什么形态、为什么入选、还差什么确认、在哪里失效”。

这样设计后，系统才能覆盖以下机会：

| 市场形态 | 新版捕捉模块 |
|---|---|
| 回调企稳 | `pullback_reversal_long` |
| 空头挤仓 | `short_squeeze_long` |
| 压缩突破 | `trend_breakout_long` |
| 龙头二段加速 | `leader_momentum_long` |
| 暴跌 V 反 | `panic_reversal_long` |
| 低位吸筹 | `accumulation_breakout_long` |
| 高位派发 | `distribution_short` |
| 多头踩踏 | `long_squeeze_short` |
| 震荡上下沿 | `range_reversion` |
| 资金费率拥挤反转 | `funding_reversal` |

一句话总结：

> **Hunter v7 的目标不是预测市场一定上涨或下跌，而是在任何市场状态下识别“有结构、有资金、有盈亏比、有确认路径”的可交易标的，并把不完整信号明确标记为等待、冲突或风险。**

---

*本文档基于 `provider/local/hunter.go` 当前实现、`hunter-v6-default-realtime-analysis-20260604.md` 实测报告，以及 2026-06-04 Hunter v6 default 筛选表现整理。本文仅为交易系统工程设计方案，不构成投资建议。*
