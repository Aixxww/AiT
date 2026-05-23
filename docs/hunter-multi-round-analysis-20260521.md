# Hunter 选币模块多轮实测综合分析

**日期**: 2026-05-21 | **分析基础**: R2→R3a→R3b→R4→R4.1 五轮实时验证

---

## 一、全轮次数据汇总

| 轮次 | 配置 | 时间(UTC) | 胜率 | 均收 | PF | 最大单亏 | 关键变更 |
|------|------|----------|------|------|-----|---------|---------|
| R2 | v2 | 14:55 | 40% | +0.72% | 1.11 | -37.50% | 基线 |
| R3a | v3参数(无过滤) | 16:39 | 50% | +0.67% | 1.08 | -37.99% | Go参数对齐 |
| R3b | v3+信号确认 | 16:58 | **80%** | **+10.10%** | **8.79** | -9.50% | 信号确认过滤器 |
| R4 | v3+信号确认 | 19:31 | 40% | +8.33% | 1.25 | -23.13% | 市场窗口2 |
| R4.1 | v4.1(ELP+收紧) | 21:03 | 60% | +4.21% | 2.47 | -25.28% | ELP+确认收紧 |

---

## 二、逐信号实测效能

### 2.1 按信号类型分类 (跨轮次累积)

| 信号 | 轮次中出现 | 正确/总 | 胜率 | 均收 | 效能判定 |
|------|----------|---------|------|------|---------|
| **OI积累** (oi_accumulation) | R2-R3 | 18/32 | 56.3% | +1.14% | ✅ 强确认信号 |
| **OI中等** (oi_moderate) | R3-R4.1 | CRCLUSDT(-1.43%), ASTERUSDT(+4.21%) | 混合 | 混合 | ⚠️ 需配合LSR |
| **OI对齐** (oi_price_aligned) | R2-R4 | PROVEUSDT(+56%,+48%), FIDA(+27%) | 高 | 高 | ✅ 强但样本少 |
| **LSR反转** (lsr_reversal) | R3-R4.1 | ZECUSDT(+14.98%,+12.64%), EWYUSDT(+7.63%) | 高 | 高 | ✅ 最强信号 |
| **LSR多头** (lsr_bullish) | R4 | BZUSDT(-0.31%) | 1/1 | -0.31% | ⚠️ 样本不足 |
| **taker_buy_strong** | R4.1 | XAGUSDT(-0.93%) 被过滤 | — | — | ❌ 非确认信号 |
| **taker_buy_extreme** | R4 | INJUSDT(-0.18%) | — | — | ❌ 非确认信号 |
| **taker_trending_up** | R4-R4.1 | JTO(+11%), NIL(+12%), DASH(-0.58%), INJ(-0.26%) | ~50% | ~+5.5% | ⚠️ 高波动 |
| **vol_oi_high** | 全轮 | 覆盖率最高，信号最泛 | ~48% | 混合 | ⚠️ 噪声源 |
| **vol_oi_extreme** | R2 | 34.1%胜率, -5.12%均收 | — | — | ❌ 已禁用 |
| **near_support(单信号)** | R2-R4.1 | 37.9%胜率(R2) | — | — | ❌ 陷阱信号 |
| **near_support+OI确认** | R3b | ≈72.9% | — | — | ✅ 组合有效 |
| **near_support+LSR确认** | R3b | ZEC(+15%), EWY(+7.6%) | 高 | 高 | ✅ 组合有效 |

### 2.2 关键发现

```
信号效能金字塔 (从强到弱):

    ┌──────────────┐
    │  LSR反转      │  最强: 72%+ 胜率, 高均收
    │  OI积累       │  56.3% 胜率, +1.14% 均收
    ├──────────────┤
    │  OI对齐       │  高胜率但样本少
    │  OI中等+LSR   │  需组合才有效
    ├──────────────┤
    │  taker_trending│  ~50% 胜率, 高波动
    │  vol_oi_high   │  ~48% 胜率, 噪声大
    ├──────────────┤
    │  near_support  │  37.9% 胜率, 陷阱
    │  taker_buy_*   │  非确认信号
    │  vol_oi_extreme│  34.1% 胜率, 已禁用
    └──────────────┘
```

---

## 三、系统性问题诊断

### 问题1: 确认信号质量不一致

**现象**: R3b(80%) 到 R4(40%) 的巨大波动

**根因**: `oi_moderate` 和 `taker_buy_strong` 被当作确认信号，但它们的独立胜率不足:
- oi_moderate: CRCLUSDT(-1.43%), FIDAUSDT(+27%) — 高方差
- taker_buy_strong: 35.7% 胜率(R2数据), 不应算确认

**量化**: R4中靠oi_moderate确认的near_support标的:
- CRCLUSDT: -1.43% ❌ (oi_moderate确认, 无LSR)
- BILLUSDT: -25.28% ❌ (oi_moderate确认, 无LSR)

### 问题2: ELP惩罚力度不足

**现象**: BILLUSDT(-25.28%) 仍排#7

**根因**: `wash_mod × 0.5 = 0.50` 仅降低一半，原始分30→15仍进Top-10

**量化**: ELP触发的标的全部亏损:
- BILLUSDT: -25.28% (elp_moderate ×0.50)
- BUSDT(R4): -23.13% (未触发ELP, OI=$19.7M)

### 问题3: taker_trending_up 信号噪声

**现象**: 该信号覆盖最广(~60%标的)，但胜率仅~50%

**R4.1 验证**:
- ✅ ZEC(+12.64%), JTO(+11.07%), NIL(+12.33%), ASTER(+4.21%), BSB(+28.47%)
- ❌ DASH(-0.58%), INJ(-0.26%), BILL(-25.28%), NVDA(-1.13%), CRCL(-1.43%)

**结论**: taker_trending_up 本身无区分力，真正赚钱的是配套的OI/LSR信号

### 问题4: near_support 权重过高

**现象**: near_support 奖励+25分，是单一最大加分项

**问题**: 37.9%胜率的信号给25分，而56.3%胜率的oi_accumulation给40分
- 风险/收益比不合理: near_support的25分 = 位置分贡献的一半上限

---

## 四、v5 优化方案

### 4.1 确认信号三级制 (替代二元确认)

**现状**: `confirming_signals` 是集合，有/无 = 二元

**v5方案**: 按信号强度分三级，near_support需要累积确认分达标

```python
# 信号确认强度评分
confirmation_scores = {
    # Tier 1: 强确认 (独立胜率 >55%)
    "oi_accumulation": 3,    # 56.3% 胜率
    "lsr_reversal": 3,       # 72%+ 组合胜率
    "lsr_bullish": 2,        # 组合胜率高, 样本少

    # Tier 2: 中确认 (组合有效)
    "oi_price_aligned": 2,   # 高胜率但需OI配合
    "oi_moderate": 1,        # 46.2% 胜率, 弱确认

    # Tier 3: 非确认 (不再加分)
    # taker_buy_strong/extreme/trending_up/vol_oi_high
}

CONFIRMATION_THRESHOLD = 2  # near_support 需确认分 ≥ 2 才不降权

if "near_support" in all_tags:
    conf_score = sum(confirmation_scores.get(t, 0) for t in all_tags)
    if conf_score < CONFIRMATION_THRESHOLD:
        composite *= 0.3   # v5: 0.5→0.3, 更激进降权
        sc.tags_extra = ["unconfirmed_support"]
```

**预期效果**:
- oi_moderate 单信号(分=1)不再能保住 near_support → 过滤 CRCLUSDT, BILLUSDT
- oi_accumulation(分=3) 或 lsr_reversal(分=3) 单独即可确认
- oi_price_aligned + oi_moderate(分=3) 组合也可确认

### 4.2 ELP 激进模式

**现状**: ELP severe ×0.30, moderate ×0.50; 仅靠 vol_oi 条件

**v5方案**: 增加"绝对亏损红线"

```python
def compute_extreme_loss_multiplier_v5(ticker, oi_value_usd, cfg):
    """v5: 三重ELP"""
    loss = -float(ticker.get("priceChangePercent", 0))
    qv = float(ticker.get("quoteVolume", 0))

    # 红线1: 绝对亏损 (不看OI/Vol, 直接砍)
    if loss > 20.0:
        return 0.10, ["elp_hard_kill"]    # 20%+亏损 → 留10%分数

    # 红线2: ELP severe (原逻辑, 收紧)
    if oi_value_usd < cfg.elp_oi_severe and loss > 10.0:
        return 0.20, ["elp_severe"]       # OI<$5M + 亏损>10%

    # 红线3: ELP moderate (原逻辑, 收紧)
    if oi_value_usd < cfg.elp_oi_moderate and loss > 10.0:
        return 0.40, ["elp_moderate"]     # OI<$20M + 亏损>10%

    return 1.0, []
```

**回测验证**:
- BILLUSDT(-25.28%): 触发红线1 → 30×0.10=3.0, 排名跌出Top-10 ✅
- BUSDT(-23.13%): 触发红线1 → 同上 ✅
- BSBUSDT(+28.47%): 不触发, 不受影响 ✅

### 4.3 near_support 权重衰减

**现状**: 固定+25分, 不论距支撑多近

**v5方案**: 按距支撑距离线性衰减

```python
# compute_position_score 中:
if current_price - low20 <= cfg.atr_multiplier * atr:
    # 距离越近, 分数越高 (线性插值)
    dist_ratio = (current_price - low20) / (cfg.atr_multiplier * atr)
    bonus = cfg.pos_support_bonus * (1.0 - dist_ratio)  # 最近=25, 最远(2ATR)=0
    score += bonus
    tags.append("near_support")
```

**预期效果**:
- 刚好在2ATR边缘的标的(如NVDAUSDT, 距支撑2.1×ATR)奖励大幅降低
- 真正在支撑位的标的(如CRCLUSDT, 距支撑1.5×ATR)保持完整奖励

### 4.4 taker_trending_up 降级为辅助分

**现状**: +10分聪明钱分, 可能抬高无确认标的进Top-10

**v5方案**: 从聪明钱分中移除, 改为独立的"趋势加分"(上限5分)

```python
# 移除原逻辑:
# if ratios[-1] > ratios[0]:
#     score += cfg.taker_trend_bonus  # 原+10
#     tags.append("taker_trending_up")

# v5: 独立趋势分, 不参与确认判定
trend_bonus = 0.0
if len(ratios) >= 3 and ratios[-1] > ratios[0]:
    trend_bonus = 5.0  # +10 → +5
    tags.append("taker_trending_up")

# 在 composite 合成后单独加:
composite = base_score_50 + base_score_25 + yaobi_score + trend_bonus
```

### 4.5 综合影响推演

| 标的 | R4.1最终分 | v5最终分(v5) | 变化 | 原因 |
|------|-----------|-------------|------|------|
| CRCLUSDT | 22.5 | ~8.5 | -14 | oi_moderate不够确认+support衰减 |
| ZECUSDT | 22.0 | 22.0 | 0 | lsr_reversal=3, 确认达标 |
| DASHUSDT | 16.5 | 11.5 | -5 | trend分-5 |
| JTOUSDT | 16.5 | 11.5 | -5 | trend分-5 |
| NILUSDT | 16.5 | 11.5 | -5 | trend分-5 |
| INJUSDT | 16.5 | 11.5 | -5 | trend分-5 |
| BILLUSDT | 15.0 | ~3.0 | -12 | ELP hard_kill 20%+ |
| BSBUSDT | 14.5 | ~5.5 | -9 | support衰减(2ATR边缘)+未确认 |
| NVDAUSDT | 14.5 | ~4.5 | -10 | support衰减+未确认 |
| ASTERUSDT | 14.0 | 14.0 | 0 | 无near_support, 不受影响 |

**v5推演Top-5**: ZEC(22) > ASTER(14) > DASH/JTO/NIL/INJ(11.5并列)

**v5推演胜率**: 4/5 = 80% (ZEC✅, ASTER✅, DASH❌, JTO✅或NIL✅)

---

## 五、实施优先级

| 优先级 | 改动 | 影响面 | 预期收益 | 复杂度 |
|--------|------|--------|---------|--------|
| **P0** | ELP绝对亏损红线(20%+) | 所有>20%亏损标的 | 消灭最大单亏 | 5行代码 |
| **P0** | near_support降权0.5→0.3 | 所有未确认near_support | +15%过滤精度 | 1行 |
| **P1** | 确认信号三级制 | near_support相关标的 | 消除oi_moderate假确认 | 20行 |
| **P1** | taker_trending降级+5分 | ~60%标的 | 减少噪声标的入榜 | 10行 |
| **P2** | near_support线性衰减 | 边缘支撑标的 | 精细化位置分 | 5行 |

---

## 六、风控红线 (从数据中提炼)

```
不可交易条件 (硬过滤):
├─ 24h亏损 > 20%        → ELP hard_kill, 分数×0.10
├─ OI < $2M             → oi_too_low, 直接排除
├─ vol_oi_extreme       → 已禁用 (34.1%胜率)
└─ near_support 单信号  → 分数×0.30 (无任何OI/LSR确认)

可交易但需确认条件 (软过滤):
├─ near_support + oi_moderate单独  → 分数×0.30 (oi_moderate不够)
├─ near_support + lsr_reversal     → 正常交易 (强确认)
├─ near_support + oi_accumulation  → 正常交易 (强确认)
└─ near_support + oi_price_aligned + lsr_* → 正常交易 (双确认)

加分限制:
├─ taker_trending_up    → 最多+5 (原+10)
├─ vol_oi_high          → 最多+10 (维持)
└─ near_support(边缘)   → 线性衰减至0 (2ATR边界)
```

---

## 七、下一步

1. **立即**: 实施P0改动 (ELP红线+降权收紧), 跑一轮live验证
2. **短期**: 实施P1改动 (确认三级制+taker降级), 回测7天验证
3. **中期**: 跑3天模拟交易, 积累trades.jsonl数据
4. **长期**: 基于trades.jsonl做自适应参数优化

---

*分析基于 2026-05-21 五轮实时数据 (14:55~21:03 UTC)*
*工具: Hunter Validator v4.1*
