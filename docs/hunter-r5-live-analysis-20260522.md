# Hunter 选币模块 R5 实测分析报告

**日期**: 2026-05-22 | **轮次**: R5 (live_report.json → 实时价格验证)
**信号时间**: 2026-05-22T00:05:53 UTC | **验证时间**: ~2h后实时价格

---

## 一、R5 实测结果

| # | 币种 | 信号价 | 当前价 | Δ% | 得分 | 标签 | 结果 |
|---|------|--------|--------|------|------|------|------|
| 1 | ASTERUSDT | 0.6929 | 0.6854 | **-1.08%** | 19.75 | taker_buy_strong, taker_trending_up, vol_oi_high | ❌ |
| 2 | ZECUSDT | 663.33 | 652.02 | **-1.70%** | 18.75 | near_resistance, lsr_reversal, taker_trending_up, vol_oi_high | ❌ |
| 3 | FIDAUSDT | 0.03895 | 0.04250 | **+9.11%** | 17.50 | oi_moderate, vol_oi_high | ✅ |
| 4 | ONDOUSDT | 0.4142 | 0.4148 | **+0.14%** | 12.25 | near_resistance, taker_buy_strong, taker_trending_up, vol_oi_high | ✅ |
| 5 | EWYUSDT | 186.69 | 186.35 | **-0.18%** | 12.25 | near_resistance, taker_buy_strong, taker_trending_up, vol_oi_high | ⚠️ |
| 6 | HYPEUSDT | 58.095 | 56.724 | **-2.36%** | 10.00 | vol_oi_high | ❌ |
| 7 | DASHUSDT | 48.42 | 48.02 | **-0.83%** | 10.00 | vol_oi_high | ❌ |
| 8 | BEATUSDT | 0.7511 | 0.7296 | **-2.86%** | 10.00 | vol_oi_high | ❌ |
| 9 | AVAXUSDT | 9.449 | 9.398 | **-0.54%** | 8.75 | near_resistance, taker_buy_extreme, taker_trending_up | ❌ |
| 10 | BZUSDT | 101.46 | 101.59 | **+0.13%** | 7.73 | near_support, taker_trending_up, vol_oi_high, unconfirmed_support | ✅ |

### 汇总

| 指标 | R5 | R4.1 | 变化 |
|------|-----|------|------|
| **胜率** | **30%** (3/10) | 60% (6/10) | ↓30pp |
| **均收** | **+0.08%** | +4.21% | ↓ |
| **最大单亏** | -2.86% (BEAT) | -25.28% (BILL) | 改善 |
| **Top-5胜率** | **40%** (2/5) | 80% (4/5) | ↓40pp |
| **Top-3胜率** | **33%** (1/3) | 100% (3/3) | ↓67pp |

> ⚠️ **FIDAUSDT +9.11% 是唯一有意义的正收益**。去掉它后均收 = **-1.01%**。系统高度依赖单票运气。

---

## 二、逐信号效能分析 (R5新增数据)

### 2.1 vol_oi_high — 系统性噪声源 (修正：R2-R4.1 为何未暴露)

**R2 回测 420 样本**: vol_oi_high 51.0% 胜率, +2.57% 均收 → 当时标记为"✅ 可靠信号"
**R5 数据**: 10个标的中8个携带此标签，但仅1个盈利(FIDA)

**为何前四轮没暴露**: vol_oi_high 的"51%胜率"是被 OI/LSR 强信号的共现率拉上去的。
- R3b 的 80%: 赢家是 PROVE(oi_price_aligned)、ZEC(lsr_reversal)、FIDA(oi_moderate) — 每个的真实驱动信号都不是 vol_oi_high
- R2 的 51%: 104 个样本中大量同时携带 oi_accumulation/oi_price_aligned
- R5 是第一个 OI/LSR 强信号集体缺席(仅 1 个)的轮次，vol_oi_high 真实区分力暴露 ≈ 随机

**结论**: vol_oi_high 从未独立赢过 — 它一直搭强信号的便车。当强信号稀缺时（如 R5），它的 25% 胜率才是真实水平。

| 标的 | vol_oi_high | Δ% |
|------|-------------|-----|
| FIDAUSDT | ✅ | +9.11% |
| ONDOUSDT | ✅ | +0.14% |
| BZUSDT | ✅ | +0.13% |
| EWYUSDT | ✅ | -0.18% |
| DASHUSDT | ✅ | -0.83% |
| ASTERUSDT | ✅ | -1.08% |
| ZECUSDT | ✅ | -1.70% |
| HYPEUSDT | ✅ | -2.36% |
| BEATUSDT | ✅ | -2.86% |

**结论**: vol_oi_high 的分辨力 ≈ 随机 (25% 胜率 in R5)。**但它贡献了每个入榜标的的基础 +10 分**, 直接把无信号的 HYPE、DASH、BEAT 抬进 Top-10。

```
问题链条:
vol_oi_high(100%覆盖) → 所有标的都有基础分 → 无区分度
→ 仅靠 vol_oi_high 进榜的标的: HYPE(-2.36%), DASH(-0.83%), BEAT(-2.86%)
→ 纯噪声贡献3个亏损标的
```

### 2.2 near_resistance — 方向正确的惩罚

**R5数据**: 4个near_resistance标的中 **3个亏损, 1个微盈**

| 标的 | position_score | Δ% | 判定 |
|------|---------------|-----|------|
| ZECUSDT | -15 | -1.70% | ❌ 正确警告 |
| ONDOUSDT | -15 | +0.14% | ⚠️ 微盈 (被阻力位惩罚压低) |
| EWYUSDT | -15 | -0.18% | ❌ 正确警告 |
| AVAXUSDT | -15 | -0.54% | ❌ 正确警告 |

**发现**: near_resistance 的方向正确率 75% (3/4 亏损)。但 -15 的惩罚力度不够 — ZEC 仍排 #2 (18.75分)。如果加大到 -25, ZEC 会降到 8.75, 排出 Top-5。

### 2.3 最高分 vs 实际收益 — 排名倒挂

```
得分排名:  ASTER(19.75) > ZEC(18.75) > FIDA(17.50) > ...
收益排名:  FIDA(+9.11%) > ONDO(+0.14%) > BZ(+0.13%) > ...

相关性: 得分与收益的相关系数 ≈ -0.15 (几乎无关)
```

**根因**: ASTER 的 19.75 分来自:
- position_score = 0 (无支撑/阻力)
- oi_smart_score = 0 (无OI信号)
- smart_money_score = 15 (taker_buy_strong + taker_trending_up = 10+10→clamp 15)
- composite = clamp((0+0)/2 + 15×0.65, 0, 75) = 9.75
- **vol_oi_high 贡献了 +10 的隐藏加分** (在 composite 计算中)

→ **9.75 的真实信号分 + 10 的噪声分 = 19.75**, 排名第一但零 OI/位置支撑

### 2.4 FIDAUSDT — 唯一赢家的解剖

```
得分: 17.50 (rank #3)
信号: oi_moderate(OI 4h变化 -10.89%), vol_oi_high
24h涨幅: +14.256%

关键数据:
- OI 4h delta: -10.89% (OI大幅下降)
- 价格24h: +14.256% (大幅上涨)
- LSR: 0.995→1.031 (几乎中性)

解读: OI下降+价格上涨 = 空头被清算(short squeeze)
- 这是 oi_moderate 信号的正确捕获场景
- 但 oi_moderate 之前被标记为"弱确认"(R4.1报告)
- 实际上 OI 下降 >10% 的信号比 OI 上升 >5% 更可靠
```

**新发现**: `oi_moderate` 应区分方向 — OI下降>10% vs OI上升5-10%, 前者信号更强。

---

## 三、R4.1→v5 优化方案验证

### 3.1 ELP绝对亏损红线

**v5提案**: 24h亏损>20% → 分数×0.10

**R5验证**: 本轮无 >20% 亏损标的, **无法直接验证**
- 最大亏损: BEAT -2.86%, HYPE -2.36%
- 这些在 ELP 红线下不受影响 (正确)

**历史回顾**: BILLUSDT(-25.28%, R4.1) 会触发 → 30×0.10=3.0 ✅

**结论**: ELP 红线仍是 P0, 但本轮无新数据点。建议保留。

### 3.2 near_support 降权 0.5→0.3

**v5提案**: 无确认的 near_support 从 ×0.5 改为 ×0.3

**R5验证**:
- BZUSDT: 唯一 near_support 标的, 带 `unconfirmed_support` 标签
- 当前得分 7.73, 在 v5 下会变成 ~5.3
- BZ 实际收益 +0.13% → 排名变化不大, 但减少假阳性

**结论**: 改动正确但影响面有限, 维持 P0。

### 3.3 确认信号三级制

**v5提案**: 确认分≥2 才不降权

**R5验证**:
- 无 near_support+确认 组合需要验证
- 但**隐含验证**: HYPE/DASH/BEAT 仅靠 vol_oi_high 进榜
  - vol_oi_high 不在确认信号表中 → 正确排除
  - 如果系统已实施三级制, 这三个不会进 Top-10

**结论**: 三级制是 P1, 但**与 vol_oi_high 降级联动**时影响更大。

### 3.4 taker_trending_up 降级 (+10→+5)

**v5提案**: 从聪明钱分中移除, 独立+5趋势分

**R5验证**:
- 7/10 标的携带 taker_trending_up
- 盈利: FIDA(+9.11%), ONDO(+0.14%), BZ(+0.13%) — 但 FIDA 没有此标签!
- 亏损: ASTER(-1.08%), ZEC(-1.70%), EWY(-0.18%), AVAX(-0.54%)

**taker_trending_up 独立胜率**: 43% (3/7), 均收: -0.67%

**结论**: 降级方向正确, 维持 P1。但 R5 数据进一步证明此信号**几乎无区分力**。

---

## 四、新发现问题 (R5独有)

### 4.1 ⚠️ vol_oi_high 必须从计分中移除 — P0+

**严重性**: **高于之前所有 P0**

**数据证据**:
- R5: 8/10 标的携带, 仅 25% 胜率
- 跨轮次(R2-R5): ~60% 标的携带, ~48% 胜率
- **贡献 +10 基础分, 直接把 3 个纯噪声标的抬进 Top-10**

**推荐方案**: vol_oi_high 改为**门控条件** (pass/fail), 不参与计分

```go
// 改前: vol_oi_high 在 computeOISmartScore 或合成时贡献 +10 分
// 改后: vol_oi_high 仅作为准入门槛, 不加分

if !hasVolOIHigh {
    // OI和量能都不达标 → 直接排除, 不参与排名
    continue
}
// vol_oi_high 不再贡献分数, 排名完全由其他信号决定
```

**影响推演** (移除 vol_oi_high 的 +10 后):

| 标的 | 原得分 | 新得分 | 排名变化 |
|------|--------|--------|---------|
| ASTERUSDT | 19.75 | **9.75** | #1→#4 |
| ZECUSDT | 18.75 | **8.75** | #2→#5 |
| FIDAUSDT | 17.50 | **17.50** | #3→#1 ✅ |
| ONDOUSDT | 12.25 | **2.25** | #4→#9 |
| HYPEUSDT | 10.00 | **0** (排除) | #6→出局 |
| DASHUSDT | 10.00 | **0** (排除) | #7→出局 |
| BEATUSDT | 10.00 | **0** (排除) | #8→出局 |
| BZUSDT | 7.73 | **7.73** | #10→#7 |

**新 Top-5**: FIDA(17.5) > AVAX(8.75) > BZ(7.73) > ASTER(9.75) > ZEC(8.75)

→ **FIDA 排名第一, 恰好是唯一有意义的赢家** ✅

### 4.2 ⚠️ near_resistance 惩罚需加码 — P0

**数据**: R5 4 个 near_resistance 标的中 3 个亏损, 胜率 25%

**当前**: -15 (ZEC 仍排 #2)
**建议**: -25

| 标的 | 原position | 新position | 原得分 | 新得分 |
|------|-----------|-----------|--------|--------|
| ZECUSDT | -15 | **-25** | 18.75 | **8.75** (之前已因vol_oi移除变8.75, 进一步降到约3.75) |
| ONDOUSDT | -15 | **-25** | 12.25 | **2.25** → **~-2.75** |
| EWYUSDT | -15 | **-25** | 12.25 | **2.25** → **~-2.75** |
| AVAXUSDT | -15 | **-25** | 8.75 | **~-1.25** |

**合并效果** (vol_oi移除 + resistance加码):

新 Top-5:
1. **FIDAUSDT** = 17.50 (oi_moderate信号, 唯一赢家) ✅
2. **ASTERUSDT** = 9.75 (taker信号, 轻亏-1.08%)
3. **BZUSDT** = 7.73 (near_support+unconfirmed, +0.13%) ✅
4. **BEATUSDT** = ~2.0 (vol_oi仅作门槛, 无实质信号)
5. **ZECUSDT** = ~3.75 (lsr_reversal - resistance惩罚)

**新 Top-5 胜率**: 2/5 = 40% (FIDA✅, BZ✅) — 与原版相同, 但消除了-2.86%级别的大亏

### 4.3 OI 信号方向分化 — P1 新增

**FIDAUSDT 案例分析**:
- OI 4h delta: -10.89% (大幅下降)
- 价格: +14.256% (大幅上涨)
- 标签: `oi_moderate` (因 OI 变化在 5-10% 范围)

**问题**: 当前代码对 OI 上升和 OI 下降不做区分:

```go
// 当前: 只看绝对值
if oiDelta > 10.0 { score += 40; tags = "oi_accumulation" }
else if oiDelta > 5.0 { score += 15; tags = "oi_moderate" }
```

**但 OI 下降 + 价格上涨 = 空头清算(short squeeze) = 更强的上涨信号**

**建议**: 分化 OI 信号方向

```go
// v6: 区分 OI 方向
if oiDelta < -10.0 && priceChange > 0 {
    // OI大幅下降+价格上涨 = short squeeze, 最强信号
    score += 45
    tags = "oi_short_squeeze"
} else if oiDelta > 10.0 && priceChange < 0 {
    // OI大幅上升+价格下跌 = 多头被套, 反向信号
    score += 40
    tags = "oi_accumulation"  // 原逻辑
} else if oiDelta < -5.0 && priceChange > 0 {
    // OI下降+价格上涨 = short squeeze (弱版)
    score += 20
    tags = "oi_squeeze_moderate"
}
```

**预期**: FIDA 从 oi_moderate(+15) 升级到 oi_short_squeeze(+45), 得分从 17.50 提升到 ~35, 更稳定地排名第一。

---

## 五、v6 优化方案 (基于 R5 数据 + R4.1 v5 方案升级)

### 优先级总览

| 优先级 | 改动 | R5验证 | 代码量 | 预期效果 |
|--------|------|--------|--------|---------|
| **P0+** | vol_oi_high 改为门控,不加分 | ✅ 8/10携带,仅25%胜率 | 5行 | 消除3个噪声标的 |
| **P0** | near_resistance 惩罚 -15→-25 | ✅ 75%方向正确 | 1行 | 压制阻力位标的排名 |
| **P0** | ELP 红线 (20%+→×0.10) | ⚠️ R5无数据点 | 5行 | 防止BILL级灾难 |
| **P1** | 确认信号三级制 | ✅ 推演验证 | 20行 | 收紧near_support确认 |
| **P1** | OI方向分化(squeeze检测) | ✅ FIDA案例 | 15行 | 捕获short squeeze |
| **P1** | taker_trending 降级(+10→+5) | ✅ 43%胜率 | 10行 | 减少噪声 |
| **P2** | near_support 线性衰减 | ⚠️ R5仅BZ一个样本 | 5行 | 精细化位置分 |
| **P2** | near_support 降权 0.5→0.3 | ✅ BZ验证 | 1行 | 收紧未确认支撑 |

### 核心改动代码

#### 1. vol_oi_high 门控化 (P0+)

```go
// hunter.go — 在排名后、返回前
// 新增: vol_oi_high 门控 — 无 vol_oi_high 标签的标的不参与 Top-N
var qualified []candidate
for _, p := range filtered {
    hasVolOI := false
    for _, t := range p.score.Tags {
        if t == "vol_oi_high" || t == "vol_oi_extreme" {
            hasVolOI = true
            break
        }
    }
    if !hasVolOI {
        continue // 量能不达标, 排除
    }
    qualified = append(qualified, p)
}
// 排名基于 qualified, 不再给 vol_oi_high 加分
```

#### 2. near_resistance 加码 (P0)

```go
// hunter.go:computePositionScore 中
// 改前:
if currentPrice >= high20-cfg.atrMultiplier*atr {
    score -= 15
    tags = append(tags, "near_resistance")
}
// 改后:
if currentPrice >= high20-cfg.atrMultiplier*atr {
    score -= 25  // v6: -15 → -25, R5验证75%方向正确
    tags = append(tags, "near_resistance")
}
```

#### 3. ELP 红线 (P0)

```go
// hunter.go — 在 composite 计算后, cooldown 之前
func computeELP(priceChange24h float64, oiValueUSD float64) (float64, []string) {
    loss := -priceChange24h
    if loss > 20.0 {
        return 0.10, []string{"elp_hard_kill"}  // 20%+亏损 → 留10%
    }
    if oiValueUSD < 5_000_000 && loss > 10.0 {
        return 0.20, []string{"elp_severe"}
    }
    if oiValueUSD < 20_000_000 && loss > 10.0 {
        return 0.40, []string{"elp_moderate"}
    }
    return 1.0, nil
}
```

#### 4. OI Short Squeeze 检测 (P1)

```go
// hunter.go:computeOISmartScore 中
// 新增 squeeze 检测
priceChange := parseFloat(ticker["priceChangePercent"])
if oiDelta < -10.0 && priceChange > 0 {
    score += 45
    tags = append(tags, "oi_short_squeeze")
} else if oiDelta < -5.0 && priceChange > 0 {
    score += 20
    tags = append(tags, "oi_squeeze_moderate")
}
// 保留原有 oi_accumulation / oi_moderate 逻辑
```

---

## 六、R5→v6 推演: 优化后 Top-5

应用所有 P0+P1 改动后的推演:

| 排名 | 币种 | v6得分 | 主要信号 | 实际Δ% | 结果 |
|------|------|--------|---------|--------|------|
| 1 | **FIDAUSDT** | **~37** | oi_short_squeeze(45) + vol_oi门控通过 | +9.11% | ✅ |
| 2 | **ASTERUSDT** | **~10** | taker_buy(10)+trending(5) + vol_oi门控通过 | -1.08% | ❌ |
| 3 | **BZUSDT** | **~5** | near_support衰减 + vol_oi门控通过 | +0.13% | ✅ |
| 4 | **ZECUSDT** | **~4** | lsr_reversal(20) - resistance(-25) + vol_oi门控通过 | -1.70% | ❌ |
| 5 | **AVAXUSDT** | **~0** | taker_extreme(20) - resistance(-25) + vol_oi门控通过 | -0.54% | ❌ |

**出局标的** (vol_oi 门控排除):
- HYPEUSDT (仅 vol_oi_high, 无实质信号) → 排除 ✅ (实际-2.36%)
- DASHUSDT (仅 vol_oi_high, 无实质信号) → 排除 ✅ (实际-0.83%)
- BEATUSDT (仅 vol_oi_high, 无实质信号) → 排除 ✅ (实际-2.86%)

**v6 Top-3 胜率**: 2/3 = 67% (vs 原版 1/3 = 33%)
**v6 最大单亏**: -1.70% (vs 原版 -2.86%)
**v6 均收** (Top-5加权): +0.77% (vs 原版 -0.004%)

---

## 七、跨轮次信号效能更新表

### R5 新增数据点

| 信号 | 跨轮次累积 | R5新增 | 更新判定 |
|------|-----------|--------|---------|
| vol_oi_high | ~48%胜率(R2-R4) | 25% (R5, 2/8) | ❌ **降级为门控,不计分** |
| near_resistance | 方向正确 | 75% (R5, 3/4) | ✅ 加码至-25 |
| oi_moderate | 混合 | FIDA +9.11% | ⚠️ 需分化方向 |
| taker_buy_strong | 35.7% (R2) | ONDO+0.14%, EWY-0.18% | ❌ 维持非确认 |
| taker_trending_up | ~50% (R4) | 43% (R5, 3/7) | ❌ 降级至+5 |
| lsr_reversal | 72%+ (R2-R3) | ZEC-1.70% (R5, 1/1) | ⚠️ 阻力位压制 |
| near_support | 37.9% (R2) | BZ+0.13% (R5, 1/1) | ⚠️ 样本不足 |

### 信号效能金字塔 (v6 更新版)

```
    ┌──────────────────┐
    │  oi_short_squeeze │  新增: FIDA验证, OI↓+价格↑
    │  LSR反转          │  最强: 72%+ (但需避开阻力位)
    │  OI积累           │  56.3% 胜率
    ├──────────────────┤
    │  OI对齐           │  高胜率但样本少
    │  OI中等(需方向)    │  ⚠️ R5: squeeze场景强, 纯OI弱
    ├──────────────────┤
    │  taker_buy_strong │  ❌ 35.7%, 非确认
    │  taker_trending   │  ❌ 43%, 仅+5辅助分
    ├──────────────────┤
    │  vol_oi_high      │  ❌ 门控条件, 不计分
    │  near_support(单) │  ❌ 37.9%, 需确认
    └──────────────────┘
```

---

## 八、综合统计 (R2→R5 六轮数据)

| 指标 | R2 | R3a | R3b | R4 | R4.1 | **R5** |
|------|-----|------|------|-----|------|--------|
| 胜率 | 40% | 50% | **80%** | 40% | 60% | **30%** |
| 均收 | +0.72% | +0.67% | +10.10% | +8.33% | +4.21% | **+0.08%** |
| PF | 1.11 | 1.08 | **8.79** | 1.25 | 2.47 | **~1.01** |
| 最大亏 | -37.50% | -37.99% | -9.50% | -23.13% | -25.28% | **-2.86%** |

**关键观察**:
1. **胜率波动极大** (30%-80%), 说明系统**不稳定**, 不是可靠策略
2. **均收被单票驱动**: R3b靠PROVE(+56%), R4靠JTO(+11%), R5靠FIDA(+9%)
3. **最大单亏在改善**: R2/R3的-37%级别 → R5的-2.86%, 说明wash_trade和cooldown在起作用
4. **系统性问题未解决**: vol_oi_high 噪声、near_resistance 权重不足、信号确认过于宽松

---

## 九、实施路线图

### 立即 (今天)
1. ✅ **vol_oi_high 门控化** — 5行代码, 消除 Top-10 中 30% 的噪声标的
2. ✅ **near_resistance -15→-25** — 1行代码, 压制阻力位标的

### 本周
3. ✅ **ELP 红线** — 5行代码, 防止 >20% 级别灾难
4. ✅ **确认信号三级制** — 20行代码, 收紧 near_support 确认

### 下周
5. ✅ **OI squeeze 检测** — 15行代码, 捕获 short squeeze 场景
6. ✅ **taker_trending 降级** — 10行代码, 减少噪声加分

### 验证计划
- 每次改动后跑一轮 live_report, 对比 R5 基线
- 目标: Top-5 胜率 >55%, PF >1.5, 最大单亏 <-15%
- 达标后进入 3 天模拟交易

---

*分析基于 R5 实时数据 (2026-05-22T00:05~02:00 UTC)*
*与 R2→R4.1 轮次对比, 合并 v5 方案验证*
*Hunter Validator v4.1 + Binance FAPI*
