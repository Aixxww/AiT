# Hunter 选币引擎复盘评估：UNIUSDT 空单止损事件

**日期**: 2026-05-23
**标的**: UNIUSDT | **方向**: SHORT | **入场价**: 3.32 | **数量**: 5 UNI
**事件**: 价格触低 3.286 后反弹至 3.378+，空单被止损

---

## 一、交易复盘时间线

| 时间 (CST) | 事件 | 价格 | 说明 |
|------------|------|------|------|
| 05-22 20:00 | 4h 开盘 | 3.638 | 日内高点附近横盘 |
| 05-23 00:00 | 4h 大阴 | 3.591→3.471 | 跌穿 3.55 支撑，量能放大 7.2M |
| 05-23 07:00 | **放量杀跌** | 3.496→3.400 | **7.17M 成交量**，TakerBuy 49%，多头止损盘涌出 |
| 05-23 08:00 | **恐慌低点** | 3.401→**3.286** | **5.56M 成交量**，日内最低，TakerBuy 仅 44.8% |
| 05-23 09:00 | **空单入场** | **3.32** | 低点反弹后追空，TakerBuy 42.5% |
| 05-23 09:xx | 反弹启动 | 3.32→3.36+ | 低点已过，空头被套 |
| 05-23 11:00 | 当前 | 3.378 | 浮亏 1.75%，止损触发 |

**关键数据**:
- 24h 跌幅: -7.17% (3.631 → 3.378)
- 24h 高低: 3.682 / 3.286 (振幅 12%)
- OI: $69.2M (4h 增长 +8.23%)
- 资金费率: -0.000023 (空头付费)
- LSR: 4.44 → 2.32 (-47.6%) (头部交易员 81.7% 做多)

---

## 二、Hunter 引擎评分实测 (当前实时)

### SHORT 方向评分明细

| 模块 | 分数 | Tags | 分析 |
|------|------|------|------|
| **Position Score** | **-20** | near_support_4h_penalize(-25), near_resistance_5m(+5) | 价格在 4h 低点 3.286 附近，空头负分 |
| **OI Score** | **15** | oi_moderate_short | OI 增 +8.23%，价格跌，但未触发 distribution (+40) |
| **Smart Money** | **60** | lsr_bearish_reversal(+20), lsr_bearish_strong(+10), extreme_bearish_penalize(-10), taker_sell_strong(+10), taker_trending_down(+10), sustained_selling(+20) | LSR -47.6% 大幅下降 + Taker 持续卖出 |
| **Composite** | **36.5** | base50=-2.5, base25=39.0 | 智能资金撑起分数，但位势拖后腿 |

### LONG 方向评分明细 (对比)

| 模块 | 分数 | Tags |
|------|------|------|
| **Position Score** | **+50** | near_support_4h(+25), near_support_1d(+15), near_support_1h(+10) |
| **OI Score** | **40** | oi_accumulation (OI↑ + Price↓) |
| **Smart Money** | **15** | lsr_extreme_bearish (LSR>2.0) |
| **Composite** | **54.8** | base50=45, base25=9.75 |

### Hunter 当前决策: **LONG 54.8 > SHORT 36.5 → 选 LONG**

> **核心矛盾**: 同一时刻，Hunter 推荐做多，但实际开了空单被止损。说明交易决策与 Hunter 引擎输出不一致，或入场时评分条件不同。

---

## 三、止损根因分析

### 根因 1: 在支撑位追空 (最致命)

```
4h 区间: High 3.690 / Low 3.286 / Close 3.378
Entry 3.32 = 距低点仅 1.02%，位于 0.9x ATR (0.0985) 范围内
```

- 价格从 3.682 暴跌至 3.286 (-12%)，在低点反弹后追空
- 4h 支撑位 3.286 已经被测试且守住了
- **Hunter 引擎在当前位置的 Position Score 为 -25 (near_support_4h_penalize)，正确识别了这个风险**

### 根因 2: LSR 极端多头仓位 → 反弹风险极高

- 头部交易员 81.7% 做多 (LSR 4.44)
- **如此极端的多头集中度意味着大量止损单堆积在下方**
- 价格触低 3.286 后，多头止损盘被触发后**空头回补**推动反弹
- 这是典型的"止损猎杀后 V 反"模式

### 根因 3: 放量下杀 = 空头能量耗尽

| 4h Bar | 成交量 | TakerBuy% | 信号 |
|--------|--------|-----------|------|
| 05-23 00:00 | 7.25M | 48.7% | 放量下跌开始 |
| 05-23 04:00 | 9.18M | 48.9% | **最大成交量** |
| 05-23 08:00 | 7.42M | 44.1% | **低点放量，卖压减弱** |

- 最后一根 4h (08:00) 成交量 7.42M 但 TakerBuy 仅 44.1%
- **卖压已大幅减弱** (从 48.9% 降到 44.1%)，说明空头已弹尽粮绝
- 此时追空 = 在空头力竭时入场

### 根因 4: 资金费率方向不利

- 资金费率 -0.000023 → **空头向多头付费**
- 市场本身偏多头，做空持有成本高
- Hunter 引擎**未使用资金费率作为信号** (HunterConfig 中 `EnableFundingRateSignal` 存在但未接入)

---

## 四、Hunter 引擎结构性缺陷评估

### 🔴 严重缺陷 (Critical)

#### 1. LSR > 2.0 惩罚逻辑反转 (BUG)

```go
// hunter.go computeShortSmartMoneyScore() line 539-543
if newestRatio > 2.0 {
    score -= 10
    tags = append(tags, "lsr_extreme_bearish_penalize")
}
```

**问题**: LSR = Long/Short Ratio > 2.0 意味着 **头部交易员 67%+ 做多** (极端看多)。
- 这应该是做空的**有利条件** (拥挤多头 → 挤空/崩盘风险)
- 但引擎给 SHORT 方向 **-10 惩罚**
- 同时，LONG 方向 line 253 给 `lsr_extreme_bearish` +15 加分 — **命名混乱，语义错误**

**影响**: 本次 UNIUSDT LSR=4.44 时，引擎不该惩罚 SHORT，反而应该加分。

**修复建议**: 
```go
// SHORT 方向: LSR > 2.0 = 拥挤多头 → 利好做空
if newestRatio > 2.0 {
    score += 15  // 不是 -= 10
    tags = append(tags, "lsr_crowded_long_favor_short")
}
```

#### 2. Position Score 不区分"已测试支撑"vs"未测试支撑"

```go
// computeShortPositionScore() line 386-393
if high4h - currentPrice <= 2*atr4h { score += 25 } // near resistance
if currentPrice - low4h <= 1.5*atr4h { score -= 25 } // near support penalty
```

**问题**: 简单的距离判断，不考虑：
- 支撑是否已被测试 (已测试 = 更强 = 更大的惩罚)
- 支撑强度 (多次测试同一价位 vs 第一次触及)
- 价格是从上方还是下方接近支撑
- 支撑是否刚被突破又回踩

**本次影响**: 价格 3.32 距离 4h 低点 3.286 仅 1%，被判定为 near_support → -25 惩罚。虽然惩罚方向正确，但缺乏对"刚从低点反弹"这一关键信息的捕捉。

#### 3. 不检测放量止损/V 反模式

08:00 的 4h K 线特征:
- 成交量 7.42M (20 根中第 3 大)
- 下影线: 3.401→3.286→3.378 (下影线占实体 63%)
- **这是典型的止损猎杀 + V 反形态**

Hunter 完全没有检测:
- 放量下跌后的长下影线 (反转信号)
- 止损盘集中区域 (OI spike at low)
- 量价背离 (价格新低但卖量递减)

### 🟡 中等缺陷 (Medium)

#### 4. 资金费率未接入评分

```go
// store/strategy.go HunterConfig
EnableFundingRateSignal bool  // 存在定义但未实现
```

- 当前资金费率 -0.000023 (空头付费) → 做空有持有成本
- Hunter API 已经通过 `/fapi/v1/premiumIndex` 获取 funding rate
- 但评分引擎**完全没用这个数据**

**建议**: 负费率 → 做空 -5~10 分；正费率 → 做空 +5~10 分

#### 5. 4h K 线开发中信号的时间窗口问题

- 入场时 08:00-12:00 的 4h bar 还在形成中
- `findHighLow()` 用的是已形成的最高/最低价
- **如果在 09:48 入场，4h 低点 3.286 还是"正在进行的极端值"而非"确认的支撑"**
- 应该区分"已收盘 K 线"和"进行中 K 线"的权重

#### 6. CoinGecko Fallback 模式降级严重

```go
// hunter.go line 662-668
if usingFallback {
    // 只计算 OI Smart Score，跳过 Position 和 Smart Money
    baseScore = clamp(oiScore + math.Log10(qv)*2, 0, 75)
    washMod = 1.0 // 无 klines，关闭洗盘检测
}
```

- Binance IP 被封时 (历史日志中有 HTTP 418 记录)，降级到 CoinGecko
- 降级后**只用 OI + Volume 两个指标**，丢失 Position Score 和 Smart Money
- 但最终评分仍然参与排名 → **降级后的高分标的可能质量很差**

### 🟢 低优先级 (Low)

#### 7. 缺少市场相关性考虑

- UNIUSDT 跌幅与 BTC/ETH 走势相关
- 引擎独立评估每个标的，不考虑大盘方向
- 如果 BTC 当时也在暴跌，追空 UNI 的风险更高 (系统性风险)

#### 8. 缺少动态止损建议

- Hunter 只输出评分和方向，不输出建议止损价
- ATR-based SL (1.5×ATR = 0.148) 在高波动时太窄
- 低点反弹 5% 就能触发止损

---

## 五、量化评分修正 (如果修复上述 BUG)

| 修正项 | 原始 SHORT 分 | 修正后 | 变化 |
|--------|-------------|--------|------|
| LSR > 2.0 修正 (−10→+15) | 60 | 75→65 (cap) | +5 |
| 资金费率接入 (负费率惩罚) | — | -5 | -5 |
| 放量下影线检测 | — | (新增信号) | — |
| **修正后 SHORT Composite** | **36.5** | **~43** | +6.5 |
| **LONG Composite** | **54.8** | **54.8** | 不变 |

即使修正后，**Hunter 仍然会选 LONG (54.8) 而不是 SHORT (43)**。
说明这次开空与 Hunter 评分系统无关 — **是人工判断或其他系统决策。**

---

## 六、改进建议优先级

### P0: 立即修复

| # | 问题 | 文件 | 改动 |
|---|------|------|------|
| 1 | LSR > 2.0 SHORT 惩罚反转 | `hunter.go:539-543` | `score -= 10` → `score += 15` |
| 2 | LSR 标签命名修正 | `hunter.go:540` | `lsr_extreme_bearish_penalize` → `lsr_crowded_long_favor_short` |

### P1: 本迭代修复

| # | 问题 | 改动量 |
|---|------|--------|
| 3 | 接入资金费率信号 | 小 (~20行) |
| 4 | 4h 进行中 K 线降权 (developing candle) | 中 (~40行) |
| 5 | 长下影线 / 放量反转检测 | 中 (~50行) |

### P2: 下个迭代

| # | 问题 | 改动量 |
|---|------|--------|
| 6 | 支撑/阻力强度评分 (多次测试 vs 首次) | 大 |
| 7 | CoinGecko fallback 评分降级标志 | 小 |
| 8 | 大盘相关性信号 | 大 |
| 9 | 动态止损建议 (基于 ATR + 支撑位) | 中 |

---

## 七、结论

### 本次 UNIUSDT 空单止损是**人为判断错误**，不是 Hunter 引擎的直接决策

**核心错误**: 在价格暴跌 12% 后、触及 4h 支撑低点 3.286 后，追空入场 3.32。
这是经典的"在地板上做空"错误。

**Hunter 引擎评估**: 
- 当前评分系统会选 **LONG (54.8)** 而不是 SHORT (36.5)
- **Position Score -25 (near_support_4h_penalize) 正确识别了支撑位风险**
- 但存在 LSR 语义反转 BUG，应修正

**最关键改进**: 在手动/其他系统开仓前，应参考 Hunter 的实时评分作为"第二意见"。
如果 Hunter 对标的打分 < 30 或推荐方向与计划方向相反，应作为强警告信号。

---

## 七、回测验证 (7天 × 42 检查点)

### 回测结论: Hunter 推荐 LONG，不是 SHORT

| 指标 | 数值 |
|------|------|
| 回测范围 | 2026-05-16 ~ 2026-05-23 (7天) |
| 检查点 | 41 个 (每4小时) |
| Hunter 选 SHORT | 21/41 (51.2%) |
| Hunter 选 LONG | 20/41 (48.8%) |
| SHORT 1h 胜率 | 42.9% (9/21) |
| SHORT 4h 胜率 | **61.9%** (13/21, avg +0.22%) |
| LONG 1h 胜率 | >50% (数据待确认) |

### 入场窗口评分 (05-23 07:36 CST ≈ 开仓时间)

| 方向 | Position Score | OI Score | SM Score | Composite |
|------|---------------|----------|----------|-----------|
| SHORT | **-25** (near_support_4h_penalize) | 0 | 10 | **0.0** |
| LONG | +25 (near_support_4h) | +25 (oi_accumulation) | 0 | **32.5** |

**Hunter 决策: LONG 32.5 > SHORT 0.0 → 选 LONG**

### LSR Bug 回测无法量化

回测使用 `[]` 空 LSR 数据 (Binance 不提供历史 LSR API)，因此 LSR 惩罚逻辑 (score -= 10) 从未被触发。

但从**实时数据**分析:
- 入场时 LSR = 4.44 (头部交易员 81.7% 做多)
- 当前逻辑: newestRatio > 2.0 → score -= 10 (惩罚 SHORT)
- 修复逻辑: newestRatio > 2.0 → score += 15 (奖励 SHORT)
- **差异: 25 分 Smart Money 分**
- 修复后 SHORT Composite: 36.5 → 约 53 (仍低于 LONG 54.8)

### 结论更新

1. **Hunter 引擎正确: 推荐 LONG 而非 SHORT** — Position Score -25 正确识别支撑位风险
2. **LSR Bug 存在但非本次止损主因** — 即使修复，Hunter 仍选 LONG (54.8 > 53)
3. **止损根因: 人工在支撑位追空** — Hunter 的 Position Score 系统已给出 -25 警告
4. **SHORT 方向本身可行** — 4h 胜率 61.9%，但需等价格远离支撑位再入场

---

*报告生成时间: 2026-05-23 11:30 CST*
*回测时间: 2026-05-23 11:45 CST*
*数据来源: Binance Futures API 实时 + AiT SQLite (data.db) + hunter_validator 回测*
