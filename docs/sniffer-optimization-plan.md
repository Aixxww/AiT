# Sniffer Gate 2 优化方案 — 替代 `bb_squeeze_15m` 的弹性压缩信号系统

## 一、问题诊断

### 当前 Gate 2 的致命缺陷

```
Gate 2: bb_squeeze_15m REQUIRED (刚性，二值)
  └─ 当前BB Width ≤ 滚动最小值 × 1.10 才触发
  └─ 意味着只有最窄的 ~10% 波动率窗口才会通过
```

**实测数据** (2026-06-03 20:42 BTC -3.3%):
- 9个标的进入Sniffer → **6个被Gate 2拦截** (67%)
- 原因: BTC大跌 → 全市场波动率飙升 → 无任何标的满足BB压缩
- **结果: Sniffer在最需要工作的高波动环境 = 完全失效**

### BB Squeeze 本质问题

BB Squeeze 检测的是"**波动率冰点**" — 价格极度收敛的时刻。但庄家/做市商的提前埋伏有多种形态:

| 埋伏形态 | BB状态 | 当前Gate2 | 漏网? |
|----------|--------|-----------|-------|
| 波动率冰点+OI悄悄增加 | 压缩 ✅ | 通过 | — |
| OI爆发性增长+价格不动 | 正常 ❌ | **拦截** | ⚠️ |
| 成交量堆积+价格窄幅 | 正常偏窄 ❌ | **拦截** | ⚠️ |
| 散户恐慌抛售+庄家接盘 | 扩张 ❌ | **拦截** | ⚠️ |
| 突破前最后的洗盘 | 正常 ❌ | **拦截** | ⚠️ |

**核心问题: Gate 2 把"低波动率"等同于"庄家埋伏"，但实际上庄家可以在任何波动率环境下埋伏。**

---

## 二、优化方案: 弹性压缩信号评分系统

### 设计哲学

将 Gate 2 从"单条件二值门控"升级为"**多信号弹性评分**"：

- **不只看波动率** → 看 OI、成交量、taker行为的综合压缩/异常
- **不要求全部满足** → 2分即可通过，组合灵活
- **保持信号质量** → 3分 = 高确信，2分 = 标准确信

### Gate 2 重新设计

```
旧 Gate 2: bb_squeeze_15m == true  → 通过 / 不通过

新 Gate 2: compression_score ≥ 2  → 通过
  ├─ bb_squeeze_15m            → +3 (波动率冰点)
  ├─ bb_squeeze_5m             → +2 (短期波动率压缩)
  ├─ oi_spike_1h               → +2 (OI异常增长)
  ├─ oi_surge_1h               → +1 (OI温和增长)
  ├─ oi_accumulation           → +2 (OI增+价格跌 = 经典吸筹)
  ├─ oi_distribution           → +2 (SHORT方向，OI增+价格涨 = 派发)
  ├─ range_compression         → +2 (NEW: 成交量堆积+价格窄幅)
  └─ squeeze_explosion_synergy → +1 (BB+OI双信号协同)
```

### 通过矩阵示例

| 场景 | 信号组合 | 分数 | 通过? |
|------|----------|------|-------|
| 经典冰点 | bb_squeeze_15m | 3 | ✅ |
| OI吸筹 | oi_accumulation | 2 | ✅ |
| OI爆发 | oi_spike_1h | 2 | ✅ |
| 短期压缩+OI | bb_squeeze_5m + oi_surge_1h | 3 | ✅ |
| 冰点+OI协同 | bb_squeeze_15m + oi_spike_1h | 3+2+1=6 | ✅ |
| 量价背离(新) | range_compression | 2 | ✅ |
| 仅温和OI | oi_surge_1h | 1 | ❌ (不够) |
| 无任何信号 | — | 0 | ❌ |

---

## 三、新增信号: `range_compression` (量价背离压缩)

### 原理

庄家吸筹的核心特征: **大量买入但不让价格上涨**。
- 成交量高于平均 → 活跃度增加
- 价格区间窄于平均 → 有人在控制价格
- 两者同时出现 → **量增价不动 = 隐性吸筹**

### 检测算法

```go
// 基于已有 SymbolCache 中的 kline 数据，无需额外API调用
func detectRangeCompression(sc *SymbolCache) (float64, []string) {
    klines1h := sc.Klines["1h"]
    if len(klines1h) < 10 { return 0, nil }

    recent := lastN(klines1h, 3)  // 最近3根1H K线
    history := klines1h[:len(klines1h)-3]  // 历史数据

    // 计算近期平均振幅 (High-Low)/Close
    recentRange := avgRange(recent)   // e.g., 2.5%
    histRange := avgRange(history)    // e.g., 5.0%

    // 计算近期平均成交量
    recentVol := avgVolume(recent)
    histVol := avgVolume(history)

    // 条件1: 近期振幅 < 历史均值的60% (价格收窄)
    rangeCompressed := recentRange < histRange * 0.60

    // 条件2: 近期成交量 > 历史均值的80% (量不萎缩)
    volActive := recentVol > histVol * 0.80

    if rangeCompressed && volActive {
        return 2, []string{"range_compression"}
    }
    return 0, nil
}
```

### 为什么这能捕捉庄家行为

1. **散户恐慌时**: 量增 + 波动率飙升 → `range_compression` 不触发 ✅ 正确排除
2. **庄家吸筹时**: 量增 + 价格窄幅震荡 → `range_compression` 触发 ✅ 正确捕捉
3. **无人问津时**: 量缩 + 价格不动 → `range_compression` 不触发 ✅ 正确排除

---

## 四、代码改动清单

### 改动 1: `provider/local/hunter.go` — 新增 `detectRangeCompression`

在 `computeSqueezeExplosionPillar()` 附近添加新函数:
- 使用已有 SymbolCache 的 1H K线数据
- 计算近期(3根) vs 历史的振幅/成交量比
- 返回分数(0或2) + tag `range_compression`
- 调用点: 在 `computeSqueezeExplosionPillar()` 中集成

### 改动 2: `provider/local/hunter_sniffer.go` — Gate 2 弹性化

替换 `filterLongAmbush` 和 `filterShortDistribution` 中的 Gate 2:

```go
// 旧: 刚性BB Squeeze检查
if !hasTag(meta.LongTags, "bb_squeeze_15m") {
    return nil
}

// 新: 弹性压缩信号评分
compressionScore := computeCompressionScore(meta.LongTags)
if compressionScore < 2 {
    return nil
}
```

新增 `computeCompressionScore()` 函数:

```go
func computeCompressionScore(tags []string) int {
    score := 0
    tagWeights := map[string]int{
        "bb_squeeze_15m":            3,
        "bb_squeeze_5m":             2,
        "oi_spike_1h":               2,
        "oi_surge_1h":               1,
        "oi_accumulation":           2,
        "oi_distribution":           2,
        "range_compression":         2,
        "squeeze_explosion_synergy": 1,
    }
    for _, tag := range tags {
        if w, ok := tagWeights[tag]; ok {
            score += w
        }
    }
    return score
}
```

### 改动 3: `provider/local/hunter_sniffer.go` — Gate 3 信号扩展

当前 Gate 3 只认3-4个信号，扩展为包含新的 OI/量价信号:

```go
// LONG Ambush 信号扩展
allLongSignals := []string{
    "oi_accumulation",           // OI增+价格跌 (经典)
    "taker_sustained_buying",    // 持续主动买入
    "lsr_reversal",              // 多空比反转
    "oi_spike_1h",               // OI异常增长 (NEW)
    "range_compression",         // 量价背离 (NEW)
}

// SHORT Distribution 信号扩展
allShortSignals := []string{
    "oi_distribution",           // OI增+价格涨 (经典)
    "taker_sustained_selling",   // 持续主动卖出
    "lsr_bearish_reversal",      // 空头反转
    "lsr_bearish_strong",        // 强空头
    "oi_spike_1h",               // OI异常增长 (NEW)
    "range_compression",         // 量价背离 (NEW)
}
```

### 改动 4: `provider/local/hunter_sniffer.go` — Gate 3 可选化

当 Gate 2 分数≥3 (高确信) 时，Gate 3 可以放宽:
- Gate 2 分数 ≥ 3: Gate 3 可选 (压缩信号本身已包含智慧钱信息)
- Gate 2 分数 == 2: Gate 3 必须 (标准流程)

---

## 五、预期效果

### 覆盖率对比

| 场景 | 旧Gate2 | 新Gate2 |
|------|---------|---------|
| BTC横盘低波动 | ✅ 有bb_squeeze | ✅ 有bb_squeeze |
| BTC大跌高波动 | ❌ 无bb_squeeze | ✅ 有oi_spike/range_compression |
| 山寨OI爆发 | ❌ 无bb_squeeze | ✅ 有oi_spike |
| 庄家吸筹中 | ❌ 无bb_squeeze | ✅ 有oi_accumulation/range_compression |
| 牛市突破前 | ❌ 无bb_squeeze | ✅ 有oi_spike + squeeze_synergy |

### 假阳性控制

- 2分门槛 → 必须有至少一个实质信号 (不能是oi_surge_1h单独通过)
- Gate 4 墙体过滤 + Gate 5 洗盘过滤 不变 → 继续排除危险标的
- Gate 3 智慧钱信号 → 低确信时仍需额外确认

### 与现有Gate 3/4/5的协同

```
新Gate 2 (压缩≥2) → Gate 3 (智慧钱信号) → Gate 4 (墙体) → Gate 5 (洗盘)
     ↓                    ↓                    ↓              ↓
  "有异常吗?"        "是庄家行为吗?"      "有阻力吗?"     "是假量吗?"
```

---

## 六、风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| oi_spike 单独通过 → 可能是散户跟风 | Gate 3 仍要求智慧钱信号确认 (oi_spike≥3时可选) |
| range_compression 误报 → 盘整期触发 | 要求成交量>历史均值80% → 排除无人问津的盘整 |
| 太多标的通过 → 选择困难 | 保持Gate 1 (score≥20) + Gate 4/5 不变 → 仍有严格过滤 |

---

## 七、实施优先级

| 优先级 | 改动 | 工作量 | 影响 |
|--------|------|--------|------|
| **P0** | Gate 2 弹性化 (压缩评分) | 30行 | 核心: 解决BB Squeeze过严问题 |
| **P1** | range_compression 新信号 | 40行 | 新增量价背离检测 |
| **P2** | Gate 3 信号扩展 | 10行 | 纳入oi_spike等新信号 |
| **P3** | Gate 3 高确信可选化 | 15行 | 提升高确信标的通过率 |
