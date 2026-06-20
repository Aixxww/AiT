# Hunter v7 数据源模块深度分析与高胜率迭代改造方案

> **文档类型**：深度技术分析 + 产品策略 + 迭代路线图  
> **分析日期**：2026-06-16  
> **分析基线**：Hunter v7 源码 + v8-SPEC-r01 实现规范 + 2026-06-15 两轮 Live Validation 报告  
> **分析视角**：加密交易大师 × 特级产品经理 × 编程专家  
> **机密级别**：INTERNAL

---

## 目录

- [一、现状全景诊断](#一现状全景诊断)
- [二、核心差距量化分析](#二核心差距量化分析)
- [三、根因深剖：为什么当前架构无法达到 80% 胜率](#三根因深剖)
- [四、高胜率迭代改造总方案](#四高胜率迭代改造总方案)
- [五、Phase 1：基础闭环补全（2周）](#五phase-1)
- [六、Phase 2：智能信号升级（4周）](#六phase-2)
- [七、Phase 3：实战级高胜率引擎（6周）](#七phase-3)
- [八、不同行情形态下的最高胜率策略矩阵](#八行情形态策略矩阵)
- [九、20%-50% 震幅机会的系统化捕捉方案](#九震幅捕捉方案)
- [十、达成 80% 开仓率·胜率·盈利率的路线图](#十路线图)
- [十一、实施优先级与资源评估](#十一实施评估)

---

## 一、现状全景诊断

### 1.1 架构管线总览

```
Binance REST API ─→ BuildV7Universe (200-350 symbols)
                        │
                        ▼
              DetectV7MarketRegime (9 regimes)
                        │
                        ▼
         V7Router.RouteDetailed (11 modules × all symbols)
           ├─ Regime Weight Gate (skip if weight < 0.2)
           ├─ Match() 快速预筛 (<1ms)
           ├─ Score() 完整评分
           ├─ Regime Weight × Setup Score
           ├─ Liquidity Score / Risk Score
           ├─ CalcAIPriority() 复合排名
           └─ Tier 判定: EXECUTABLE / REVIEWABLE / WATCH / REJECTED
                        │
                        ▼
              ResolveV7Conflicts() 双向冲突解决
                        │
                        ▼
              filterV7SignalsForLLM() Top-N + 多样性
                        │
                        ▼
              BuildV7PreMoveRadar (4 watch modules)
                        │
                        ▼
              V7SignalStateManager (跨轮次状态机)
                        │
                        ▼
              writeHunterV7TieredCandidatePrompt()
                        │
                        ▼
              AIT Kernel 决策 → 执行 / 观望
```

### 1.2 当前 11 个信号模块清单

| # | 模块名 | Setup 类型 | 方向 | 设计用途 |
|---|--------|-----------|------|---------|
| 1 | `pullbackLongModule` | `pullback_reversal_long` | LONG | 趋势中回踩反转 |
| 2 | `shortSqueezeLongModule` | `short_squeeze_long` | LONG | 空头挤压反抽 |
| 3 | `trendBreakoutLongModule` | `trend_breakout_long` | LONG | 趋势突破 |
| 4 | `leaderMomentumLongModule` | `leader_momentum_long` | LONG | 强势领涨追涨 |
| 5 | `panicReversalLongModule` | `panic_reversal_long` | LONG | 恐慌后反转 |
| 6 | `accumulationBreakoutLongModule` | `accumulation_breakout_long` | LONG | 横盘积累突破 |
| 7 | `distributionShortModule` | `distribution_short` | SHORT | 顶部派发做空 |
| 8 | `squeezeShortModule` | `long_squeeze_short` | SHORT | 多头挤压做空 |
| 9 | `rangeReversionModule` | `range_reversion` | BOTH | 区间均值回归 |
| 10 | `fundingReversalModule` | `funding_reversal` | BOTH | 资金费率反转 |
| 11 | `displacementMomentumLongModule` | `displacement_momentum_long` | LONG | 位移动量 |

**Pre-Move Watch 模块（4个）**：`pre_breakout`、`accumulation`、`pre_squeeze`、`pre_distribution`

### 1.3 评分公式解构

```
AIPriority = setup_score × 0.35
           + timing_score × 0.20
           + regime_fit_score × 0.20
           + liquidity_score × 0.15
           - risk_score × 0.10
           + setup_expectancy_bonus
           + execution_quality_bonus
```

**各维度来源**：

| 维度 | 分值范围 | 计算依据 |
|------|---------|---------|
| `setup_score` | 0-100 | 各模块独立的 Match + Score 逻辑 |
| `timing_score` | 0-100 | Taker 买卖比、OI 变化、RSI 恢复、价格动作组合 |
| `regime_fit_score` | 0-100 | `regime_weight × 67`（最大 1.5×67≈100） |
| `liquidity_score` | 0-100 | 成交额、OI、交易笔数、OI/Volume 健康比 |
| `risk_score` | 0-100 | 波动率、Funding、拥挤度、洗盘、流动性、OI 异常 |

**Tier 判定规则**：

| Tier | 条件 |
|------|------|
| EXECUTABLE | `ready_score ≥ 75` + 所有硬性确认通过 + RR ≥ 1.8 |
| REVIEWABLE | `ready_score ≥ 55` + 大部分确认通过 |
| WATCH | 被特定 blocker 卡住但结构有效 |
| REJECTED | `risk_score ≥ 90` 或 `liquidity < 30` |

### 1.4 两轮实测数据快照

| 指标 | R1 (11:07) | R2 (11:38) | 合计 |
|------|-----------|-----------|------|
| Regime | `rotation` | `rotation` | — |
| Universe 规模 | 197 | 217 | — |
| REST 错误 | 38 | 0 | — |
| BTC 24h | +1.62% | +1.93% | — |
| 信号总数 | 12 | 14 | 26 |
| LONG / SHORT | 10/2 | 14/0 | 24/2 |
| EXECUTABLE | 3 | 2 | 5 |
| REVIEWABLE | 3 | 4 | 7 |
| **开仓复核率** | 50.00% | 42.86% | **46.15%** |
| **直接可执行率** | 25.00% | 14.29% | **19.23%** |

### 1.5 可跟踪胜率数据

| 跟踪维度 | 样本 | 胜率 | 平均收益 | TP1 触达 | SL 触达 |
|---------|------|------|---------|---------|---------|
| 开仓复核候选（30m 跟踪） | 3 | **33.33%** | **-0.84%** | 0/3 | 1/3 |
| 全量交集信号（30m 跟踪） | 7 | 28.57% | +0.02% | 0/7 | — |
| `leader_momentum_long` 主模块 | 3 | 33.33% | -0.84% | 0/3 | 1/3 |

---

## 二、核心差距量化分析

### 2.1 与目标的差距

| 指标 | 当前值 | 目标值 | 差距倍数 | 评估 |
|------|-------|-------|---------|------|
| 开仓复核率 | 46.15% | **≥80%** | 1.73× | 需结构性提升 |
| EXECUTABLE 方向胜率（30m） | 33.33% | **≥80%** | 2.40× | **核心瓶颈** |
| TP1 触达率（30m） | 0% | ≥45% | ∞ | TP 体系需重建 |
| 每日 20%+震幅捕捉 | 3-5 个 | 15-20 个 | 3-5× | 模块覆盖不足 |
| 盈利率 | -0.84% | **>0**（稳定正） | — | 负期望值 |

### 2.2 差距的本质

**开仓率 46% → 80%**：不是简单放宽门槛的问题。当前 blocker 类型分析：

| Blocker | 频次 | 修复方向 |
|---------|------|---------|
| `wait_zone_retest_required` | 高频 | 需 fast-track 或替代入场逻辑 |
| `low_timing_watch_only` | 高频 | 需 Panic Override 或 timing 权重调整 |
| `no_pullback_still_running` | 中频 | 需 trailing entry 或分批入场 |
| `displacement_rr_insufficient` | 中频 | 需几何修复 |
| `funding_short_weak_4h_flush_wait` | 低频 | 属正常等待 |

**胜率 33% → 80%**：这才是真正困难的目标。差距根源：

1. **信号产生时间与最佳入场时间存在结构性延迟**：信号基于快照数据，入场时前段利润已被吃掉
2. **TP1 目标对短线窗口偏远**：30 分钟内 0/3 触达 TP1，说明 TP 计算基于趋势持仓而非短线
3. **止损机制被动**：仅靠固定百分比 / ATR 倍数，不考虑盘中微观结构
4. **追高保护不足**：`leader_momentum_long` 在 `UAIUSDT` 上直接跌破 SL

---

## 三、根因深剖：为什么当前架构无法达到 80% 胜率 {#三根因深剖}

### 3.1 架构层面的六大结构性缺陷

#### 缺陷一：信号模块覆盖的时间框架错配

当前 11 个模块的隐含时间框架：

```
pullback_reversal    → 4h-1d 趋势回调     ← 中线
short_squeeze        → 15m-1h 挤压         ← 超短线
trend_breakout       → 4h-1d 突破          ← 中线
leader_momentum      → 15m-1h 动量追涨     ← 超短线
panic_reversal       → 1h-4h 恐慌反转      ← 短线
accumulation_breakout→ 1d+ 横盘突破        ← 长线
distribution_short   → 4h-1d 顶部派发      ← 中线
long_squeeze_short   → 15m-1h 挤压做空     ← 超短线
range_reversion      → 1h-4h 区间回归      ← 短线
funding_reversal     → 4h-8h 资金费率      ← 中线
displacement_momentum→ 15m-1h 位移动量     ← 超短线
```

**问题**：30 分钟验证窗口 vs 4h-1d 的中线模块 = **时间框架错配**。`leader_momentum` 在 30 分钟内仅涨 +1.32% 就被判"未止盈"，但实盘上这已经是有效盈利。

#### 缺陷二：评分公式是纯线性加权，无非线性交互

```
AIPriority = Σ(wi × xi) - w_risk × risk
```

这是最简单的线性模型。加密市场的实际是非线性的：

- **条件共振效应**：`strong_reclaim` + `taker_buy_strong` + `oi_flush` 三者同时出现时，胜率不是三个独立条件的线性叠加，而是接近于「几乎确定反转」
- **极端风险的非线性惩罚**：`risk_score = 60` 和 `risk_score = 80` 的差距远大于 `risk_score = 20` 和 `risk_score = 40`，但线性公式把它们等比例处理
- **timing 的门槛效应**：`timing_score < 40` 的信号几乎全部失败，但线性公式给它分配了 20% 权重而非 0%

#### 缺陷三：TP/SL 体系是一维静态计算

当前 TP/SL 计算：
```go
StopPrice = EntryPrice - ATR × multiplier
T1Price = EntryPrice + ATR × multiplier
```

问题：
- 不考虑盘口深度（大单支撑/压力）
- 不考虑 VWAP 偏离（价格相对日内均价的位置）
- 不考虑波动率状态（高波动率环境 ATR 放大但 TP 应更保守）
- 不区分趋势持仓 TP 和日内快进快出 TP

#### 缺陷四：没有信号→结果的反馈闭环

`HunterV7SignalRecord` 存了信号，但：
- 没有追踪信号发出后 15m/30m/60m/4h 的价格走势
- 没有计算 per-setup-type 的胜率、平均收益、MFE/MAE
- 没有用真实结果回传调参的机制
- 没有连接 backtest 框架（backtest/ 目录不引用 hunter_v7）

**结论**：当前是一个「开环」系统，信号发出后就失忆了。

#### 缺陷五：Regime 检测粒度不足

9 种 Regime 看似丰富，但实际判断基于 BTC/ETH 的 24h 涨跌 + 极端条件，**缺少**：

- 盘中微 Regime（15m 级别的快速切换）
- 板块分化指数（同涨/分化/独立板块）
- 波动率 Regime（低波压缩 → 高波爆发的转换点）
- OI 结构 Regime（主力建仓/出货/横盘对峙）

#### 缺陷六：验证框架的致命盲区

`cmd/hunter_v7_validate` 的三大问题：

1. **仅验证格式和覆盖面，不验证胜率**：`validateFormat`、`validatePrompt`、`validateCoverage` 都是结构检查，没有 "这个信号 30 分钟后赚钱了吗"
2. **快照级价格跟踪太粗糙**：两轮之间只取了一次 R2 快照价，没有 1m K 线的高低点跟踪
3. **未跟踪的 R1 信号被丢弃**：`ZROUSDT`、`JUPUSDT` 第一轮是 EXECUTABLE 但第二轮未入榜，直接从数据集消失，无法评估

### 3.2 为什么 v8-SPEC 的 P0-P2 方案仍不够

现有 v8-SPEC 方案评估：

| v8-SPEC 方案 | 解决的问题 | 对 80% 胜率的贡献 | 评估 |
|-------------|-----------|-------------------|------|
| P0-1 Panic Override | timing 权重对反转信号的系统性惩罚 | 提升开仓率 5-8% | ✅ 必要，但只提升开仓率不提升胜率 |
| P0-2 Funding Fast-Track | 9 个 funding_short 全部卡在 zone_retest | 可能增加 2-3 个开仓信号 | ✅ 必要，但不改变胜率基本面 |
| P0-3 Matrix Report | 无自动化统计 | 提升分析效率 | ✅ 基础设施 |
| P1-1 PnL Tracker | 无闭环反馈 | **关键基础设施** | ✅✅ 最重要的缺失件 |
| P1-2 Watch Upgrader | WATCH 信号无法自动升级 | 提升开仓率 3-5% | ✅ 有价值 |
| P1-3 Geometry Audit | displacement 几何问题 | 小幅提升 | ⚠️ 边际改善 |
| P2-1 OI 建仓识别 | 新信号模块 | 新增一类高胜率信号 | ✅✅ 有望贡献高质量信号 |
| P2-2 相关性过滤 | 同质信号过多 | 降低虚假繁荣 | ✅ 必要 |
| P2-3 Sector Rotation | 板块轮动识别 | 更好地利用 rotation 市场 | ✅ 有价值 |
| P2-4 Prompt 压缩 | Token 使用优化 | 不直接影响胜率 | ⚠️ 间接价值 |

**总体评估**：v8-SPEC 是一个**渐进优化方案**，解决的是"从 46% 开仓率和 33% 胜率提升到 55-60% 胜率"的问题。但距离 80% 目标，还需要一个**范式级的升级**。

---

## 四、高胜率迭代改造总方案 {#四高胜率迭代改造总方案}

### 4.1 设计哲学

```
┌─────────────────────────────────────────────────────┐
│          高胜率交易系统的核心哲学                       │
│                                                     │
│  ❌ 不是"找到更多信号"                                │
│  ✅ 而是"在正确的时间对正确的标的做正确的事"             │
│                                                     │
│  ❌ 不是"线性加权排序"                                │
│  ✅ 而是"条件共振判定 + 动态适应"                      │
│                                                     │
│  ❌ 不是"信号发出后祈祷"                              │
│  ✅ 而是"发出→跟踪→学习→进化"的闭环                   │
│                                                     │
│  ❌ 不是"一刀切的 TP/SL"                              │
│  ✅ 而是"波动率自适应 + 分层止盈 + 盘中动态管理"        │
└─────────────────────────────────────────────────────┘
```

### 4.2 三阶段改造路线图

```
Phase 1: 基础闭环补全（2周）          目标：胜率 33% → 50%
  ├─ 信号 PnL 跟踪闭环（最关键）
  ├─ 多时间框架 TP 体系
  ├─ 1m K 线验证引擎
  └─ 入场时机优化器

Phase 2: 智能信号升级（4周）          目标：胜率 50% → 65%
  ├─ 条件共振评分模型
  ├─ Regime 自适应权重引擎
  ├─ 新增 3 个高胜率模块
  └─ 动态止损管理器

Phase 3: 实战级高胜率引擎（6周）      目标：胜率 65% → 80%
  ├─ ML 辅助信号评分
  ├─ OI 隐形建仓 + 链上信号
  ├─ 盘口深度分析
  └─ 全链路自动化闭环
```

---

## 五、Phase 1：基础闭环补全（2周）{#五phase-1}

> **目标**：建立"信号→跟踪→结果→调参"的完整闭环，将可跟踪胜率从 33% 提升到 50%

### 5.1 P1-A：信号 PnL 跟踪引擎（最高优先级）

**现有代码分析**：`store/hunter_v7_signal.go` 已有 `HunterV7SignalRecord` GORM 模型，但仅保存信号快照，不跟踪结果。

**新增文件**：`hunter_v7_pnl_tracker.go`

```go
// SignalOutcomeTracker 信号结果跟踪器
// 每 1 分钟轮询一次，使用 1m K 线数据
type SignalOutcomeTracker struct {
    db           *gorm.DB
    priceCache   *PriceCache       // 1m K 线缓存
    config       *TrackerConfig
    activeRecords map[string]*TrackedSignal
}

type TrackedSignal struct {
    RecordID       string
    Symbol         string
    Direction      string
    Setup          string
    Tier           string
    SignalTime     time.Time
    SignalPrice    float64
    StopPrice      float64
    TP0Price       float64  // 新增：短线止盈（详见 5.2）
    TP1Price       float64
    TP2Price       float64

    // 实时跟踪状态
    Status         string    // "ACTIVE" / "WIN_TP0" / "WIN_TP1" / "WIN_TP2" / "STOP" / "TIMEOUT"
    CurrentPrice   float64
    MaxFavorable   float64   // 最大顺向波动（MFE）
    MaxAdverse     float64   // 最大逆向波动（MAE）
    EntryTime      time.Time
    ExitTime       *time.Time

    // 逐分钟快照（用于事后精细分析）
    MinuteSnapshots []MinuteSnapshot
}

type MinuteSnapshot struct {
    T        time.Time
    Price    float64
    High     float64
    Low      float64
    Volume   float64
    PnLPct   float64
    IsTPHit  string  // "" / "TP0" / "TP1" / "TP2"
    IsSLHit  bool
}

// 核心跟踪循环
func (t *SignalOutcomeTracker) Run(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            t.tick()
        }
    }
}

func (t *SignalOutcomeTracker) tick() {
    now := time.Now()

    // 1. 从 DB 加载新的 ACTIVE 信号
    t.loadNewActiveRecords()

    // 2. 批量获取当前价格（单次 Binance /ticker/24hr 或 /markPrice）
    prices := t.priceCache.GetAllPrices()

    // 3. 逐条更新
    for _, sig := range t.activeRecords {
        if sig.Status != "ACTIVE" { continue }

        price, ok := prices[sig.Symbol]
        if !ok { continue }

        // 计算当前 PnL
        pnlPct := calcPnLPct(sig.Direction, sig.SignalPrice, price)

        // 更新 MFE/MAE
        if pnlPct > sig.MaxFavorable { sig.MaxFavorable = pnlPct }
        if -pnlPct > sig.MaxAdverse { sig.MaxAdverse = -pnlPct }

        // 检查 TP/SL 触达（使用 1m K 线的 high/low）
        candle := t.priceCache.GetLastMinuteCandle(sig.Symbol)
        t.checkHitWithCandle(sig, candle, now)

        // 记录分钟快照
        sig.MinuteSnapshots = append(sig.MinuteSnapshots, MinuteSnapshot{
            T: now, Price: price,
            High: candle.High, Low: candle.Low,
            PnLPct: pnlPct,
        })

        // 超时判定（8 小时后自动关闭）
        if now.Sub(sig.SignalTime) > 8*time.Hour {
            sig.Status = "TIMEOUT"
            sig.ExitTime = &now
        }
    }

    // 4. 持久化
    t.persist()
}

func (t *SignalOutcomeTracker) checkHitWithCandle(sig *TrackedSignal, candle *CandleData, now time.Time) {
    // LONG: 检查 candle.High 是否触达 TP，candle.Low 是否触达 SL
    // SHORT: 反向
    if sig.Direction == "LONG" {
        if candle.High >= sig.TP2Price { sig.Status = "WIN_TP2"; sig.ExitTime = &now; return }
        if candle.High >= sig.TP1Price { sig.Status = "WIN_TP1"; sig.ExitTime = &now; return }
        if candle.High >= sig.TP0Price { sig.Status = "WIN_TP0"; sig.ExitTime = &now; return }
        if candle.Low <= sig.StopPrice  { sig.Status = "STOP";    sig.ExitTime = &now; return }
    } else {
        if candle.Low <= sig.TP2Price   { sig.Status = "WIN_TP2"; sig.ExitTime = &now; return }
        if candle.Low <= sig.TP1Price   { sig.Status = "WIN_TP1"; sig.ExitTime = &now; return }
        if candle.Low <= sig.TP0Price   { sig.Status = "WIN_TP0"; sig.ExitTime = &now; return }
        if candle.High >= sig.StopPrice { sig.Status = "STOP";    sig.ExitTime = &now; return }
    }
}
```

**关键设计决策**：

1. **1 分钟轮询**：不是 15 分钟。30 分钟内就能完成 30 个数据点的精细跟踪
2. **1m K 线高低点触达判定**：不是快照价比较，而是用 1m candle 的 high/low 判断是否实际触达 TP/SL
3. **分钟级快照数组**：事后可以精确重建"在第几分钟达到最大浮盈/浮亏"，用于优化 TP 设置
4. **自动注册**：Router 输出时自动将 EXECUTABLE/REVIEWABLE 信号注册到 Tracker

### 5.2 P1-B：多时间框架 TP 体系（TP0/TP1/TP2 分层）

**问题**：当前只有单一 TP1（基于 ATR 倍数），30 分钟验证窗口内 0/3 触达。

**方案**：引入三层 TP，适配不同持仓周期。

```go
// MultiTimeframeTP 多时间框架止盈计算
type MultiTimeframeTP struct {
    TP0   *TargetLevel  // 超短线（5-30 分钟）
    TP1   *TargetLevel  // 短线（30 分钟 - 2 小时）
    TP2   *TargetLevel  // 中线（2 小时 - 8 小时）
}

type TargetLevel struct {
    Price       float64
    RR          float64
    TimeWindow  string   // "5-30m" / "30m-2h" / "2h-8h"
    Percentile  string   // "conservative" / "standard" / "aggressive"
    Method      string   // 计算方法标识
}

func CalcMultiTimeframeTP(
    direction string,
    entryPrice, stopPrice float64,
    atr15m, atr1h float64,
    vwap15m float64,
    bbUpper, bbMiddle float64,
    recentHigh, recentLow float64,
    regime string,
) *MultiTimeframeTP {

    risk := math.Abs(entryPrice - stopPrice)

    // TP0: 超短线目标
    // 取以下三者中的最近值：
    //   a) 0.8 × risk（快速锁定 0.8R）
    //   b) VWAP 延伸目标（15m VWAP + 0.5×ATR15m）
    //   c) BB 上/下轨（均值回归目标）
    tp0Candidates := []float64{}
    if direction == "LONG" {
        tp0Candidates = []float64{
            entryPrice + risk*0.8,                   // 0.8R
            vwap15m + atr15m*0.5,                    // VWAP 延伸
            bbUpper,                                 // BB 上轨
            entryPrice + atr15m*1.0,                 // 1×ATR15m
        }
    } else {
        tp0Candidates = []float64{
            entryPrice - risk*0.8,
            vwap15m - atr15m*0.5,
            bbMiddle - (bbUpper-bbMiddle)*0.5,       // BB 下半区
            entryPrice - atr15m*1.0,
        }
    }
    tp0Price := nearestInDirection(direction, entryPrice, tp0Candidates)
    tp0RR := math.Abs(tp0Price-entryPrice) / risk

    // TP1: 短线目标
    // 取以下三者中的中值：
    //   a) 1.5 × risk
    //   b) ATR1h × 1.5
    //   c) 近期高低点
    tp1Candidates := []float64{}
    if direction == "LONG" {
        tp1Candidates = []float64{
            entryPrice + risk*1.5,
            entryPrice + atr1h*1.5,
            recentHigh * 0.995,  // 略低于近期高点（避免假突破）
        }
    } else {
        tp1Candidates = []float64{
            entryPrice - risk*1.5,
            entryPrice - atr1h*1.5,
            recentLow * 1.005,
        }
    }
    tp1Price := medianInDirection(direction, entryPrice, tp1Candidates)
    tp1RR := math.Abs(tp1Price-entryPrice) / risk

    // TP2: 中线趋势目标
    // 保留原 ATR 倍数法，但乘以 Regime 乘数
    regimeMultiplier := map[string]float64{
        "trend_up":       1.3,
        "rotation":       1.0,
        "range":          0.8,
        "compression":    1.2,
        "trend_down":     0.9,
        "panic_dump":     1.4,
        "market_pullback": 1.1,
        "mania_pump":     1.5,
        "mixed":          1.0,
    }[regime]

    tp2Offset := atr1h * 3.0 * regimeMultiplier
    var tp2Price float64
    if direction == "LONG" {
        tp2Price = entryPrice + tp2Offset
    } else {
        tp2Price = entryPrice - tp2Offset
    }
    tp2RR := math.Abs(tp2Price-entryPrice) / risk

    return &MultiTimeframeTP{
        TP0: &TargetLevel{
            Price: tp0Price, RR: tp0RR,
            TimeWindow: "5-30m", Method: "nearest_of_0.8R_vwap_bb_atr15m",
        },
        TP1: &TargetLevel{
            Price: tp1Price, RR: tp1RR,
            TimeWindow: "30m-2h", Method: "median_of_1.5R_atr1h_recent_hl",
        },
        TP2: &TargetLevel{
            Price: tp2Price, RR: tp2RR,
            TimeWindow: "2h-8h", Method: "atr3x_regime_adjusted",
        },
    }
}
```

**对 UAIUSDT 重新计算示例**：

```
UAIUSDT (2026-06-15):
  Entry: 0.3424, Stop: 0.3356, Risk: 0.0068
  ATR15m: 0.0034, ATR1h: 0.0082
  VWAP15m: 0.3400, BB_Upper: 0.3455

  TP0 = min(0.3424 + 0.0054, 0.3400 + 0.0017, 0.3455, 0.3424 + 0.0034)
      = 0.3441 (VWAP 延伸, +0.50%)
      → 30 分钟后 WETUSDT 涨了 +1.32%，此 TP0 会被命中 ✅

  TP1 = median(0.3424 + 0.0102, 0.3424 + 0.0123, 近期高点×0.995)
      ≈ 0.3526 (+2.98%)

  TP2 = 0.3424 + 0.0246 × 1.0(rotation) = 0.3670 (+7.18%)
```

### 5.3 P1-C：1m K 线验证引擎

**问题**：当前验证器只做两轮快照比较，丢失了轮次之间的全部价格运动。

**新增**：`cmd/hunter_v7_validate/kline_tracker.go`

```go
// KLineTracker 1m K 线跟踪器（验证器专用）
type KLineTracker struct {
    client       *binance.Client
    trackedPairs map[string]*KLineTrackRecord
}

type KLineTrackRecord struct {
    Symbol       string
    Direction    string
    SignalPrice  float64
    StopPrice    float64
    TP0Price     float64
    TP1Price     float64
    StartTime    time.Time
    KLines       []KLine1m
    FirstTP0Hit  *KLine1m
    FirstTP1Hit  *KLine1m
    FirstSLHit   *KLine1m
    MaxHigh      float64
    MinLow       float64
    MaxFavorable float64  // MFE %
    MaxAdverse   float64  // MAE %
}

type KLine1m struct {
    OpenTime time.Time
    Open     float64
    High     float64
    Low      float64
    Close    float64
    Volume   float64
    TakerBuy float64
}

// FetchAndTrack 从信号时间到验证时间之间的所有 1m K 线
func (t *KLineTracker) FetchAndTrack(symbol string, startTime, endTime time.Time) {
    klines, err := t.client.GetKlines(symbol, "1m", startTime, endTime)
    if err != nil { return }

    record := t.trackedPairs[symbol]
    for _, k := range klines {
        kline := KLine1m{
            OpenTime: k.OpenTime, Open: k.Open, High: k.High,
            Low: k.Low, Close: k.Close, Volume: k.Volume,
            TakerBuy: k.TakerBuyVolume,
        }
        record.KLines = append(record.KLines, kline)

        // 更新极值
        if k.High > record.MaxHigh { record.MaxHigh = k.High }
        if k.Low < record.MinLow { record.MinLow = k.Low }

        // 检查 TP/SL 触达
        if record.Direction == "LONG" {
            if record.FirstTP0Hit == nil && k.High >= record.TP0Price {
                record.FirstTP0Hit = &kline
            }
            if record.FirstTP1Hit == nil && k.High >= record.TP1Price {
                record.FirstTP1Hit = &kline
            }
            if record.FirstSLHit == nil && k.Low <= record.StopPrice {
                record.FirstSLHit = &kline
            }
            pnl := (k.High - record.SignalPrice) / record.SignalPrice * 100
            if pnl > record.MaxFavorable { record.MaxFavorable = pnl }
            adverse := (record.SignalPrice - k.Low) / record.SignalPrice * 100
            if adverse > record.MaxAdverse { record.MaxAdverse = adverse }
        }
    }
}
```

### 5.4 P1-D：入场时机优化器（Timing Booster）

**问题**：`leader_momentum_long` 的 `UAIUSDT` 信号被标为 EXECUTABLE 但 30 分钟后跌破 SL，说明入场时机判别不够精细。

**新增**：`hunter_v7_timing_booster.go`

```go
// TimingBooster 入场时机精细化判定
type TimingBooster struct {
    config *TimingBoosterConfig
}

type TimingBoosterConfig struct {
    // 追高保护
    MaxEntryZonePosition    float64 // 0.70 — 入场价不能超过 entry zone 上沿的 70%
    MinTakerBuyForTopZone   float64 // 0.60 — 高位入场需要更强的 taker buy
    RequireTrailingSupport  bool    // true — 高位入场要求 5m EMA 支撑

    // 动量衰减检测
    MomentumDecayWindow     int     // 5 — 检查最近 5 根 1m K 线
    MaxDecayRatio           float64 // 0.4 — 每根 K 线涨幅递减不超过 40%

    // 成交量确认
    MinVolumeSustainRatio   float64 // 0.7 — 最近 3 根 1m 成交量不低于启动量的 70%

    // 短期超买保护
    RSI5mOverboughtLevel    float64 // 80.0
    RSICooldownRequired     bool    // true — RSI5m > 80 需等回落到 70 以下
}

func (tb *TimingBooster) EnhanceTiming(
    signal *V7SignalOutput,
    ctx *V7SymbolContext,
    recentKLines []*KLine1m,
) *TimingBoostResult {

    result := &TimingBoostResult{
        OriginalTiming: signal.TimingScore,
        Adjustments:    []string{},
    }

    boosted := signal.TimingScore

    // ── 检查 1: 追高保护 ──
    if signal.EntryZone.Upper > 0 {
        entryPosition := (ctx.Price - signal.EntryZone.Lower) /
            (signal.EntryZone.Upper - signal.EntryZone.Lower)
        if entryPosition > tb.config.MaxEntryZonePosition {
            if ctx.TakerBuy15m < tb.config.MinTakerBuyForTopZone {
                boosted -= 15
                result.Adjustments = append(result.Adjustments,
                    fmt.Sprintf("追高保护: 位于 entry zone %.0f%%, taker buy %.2f 不足",
                        entryPosition*100, ctx.TakerBuy15m))
                result.DowngradeTo = "REVIEWABLE"
            }
        }
    }

    // ── 检查 2: 动量衰减 ──
    if len(recentKLines) >= tb.config.MomentumDecayWindow {
        decays := countMomentumDecays(recentKLines, signal.Direction)
        if decays >= 3 {
            boosted -= 10
            result.Adjustments = append(result.Adjustments,
                fmt.Sprintf("动量衰减: 最近 %d 根 K 线中 %d 根递减", tb.config.MomentumDecayWindow, decays))
        }
    }

    // ── 检查 3: 成交量可持续性 ──
    if len(recentKLines) >= 5 {
        recentVol := avgVolume(recentKLines[len(recentKLines)-3:])
        launchVol := avgVolume(recentKLines[:2])
        if launchVol > 0 && recentVol/launchVol < tb.config.MinVolumeSustainRatio {
            boosted -= 8
            result.Adjustments = append(result.Adjustments,
                fmt.Sprintf("量能衰减: 近 3 根量/启动量 = %.2f", recentVol/launchVol))
        }
    }

    // ── 检查 4: 短期超买 ──
    if ctx.RSI5m > tb.config.RSI5mOverboughtLevel {
        boosted -= 12
        result.Adjustments = append(result.Adjustments,
            fmt.Sprintf("超买: RSI5m = %.1f", ctx.RSI5m))
    }

    result.AdjustedTiming = math.Max(boosted, 10)
    result.TimingDelta = result.AdjustedTiming - signal.TimingScore

    return result
}
```

**预期效果**：`UAIUSDT` 类信号的 timing_score 从 45 降到 ~25，priority 从 74.9 降到 ~65，从 EXECUTABLE 降级为 REVIEWABLE，避免追高亏损。

### 5.5 Phase 1 验收标准

| 指标 | 当前值 | Phase 1 目标 | 验证方法 |
|------|-------|------------|---------|
| 可跟踪胜率（TP0, 30m） | 0% | **≥45%** | 10 轮 live validation |
| 可跟踪胜率（TP1, 2h） | 33% | **≥50%** | 10 轮 live validation |
| MFE/MAE 数据覆盖率 | 0% | **≥90%** | 数据完整性检查 |
| 1m K 线触达判定 | 无 | **全面启用** | 对比快照级 vs K 线级差异 |
| 追高误入场率 | ~33% | **<15%** | UAIUSDT 类场景回归测试 |

---

## 六、Phase 2：智能信号升级（4周）{#六phase-2}

> **目标**：引入非线性评分、自适应权重、新模块，胜率从 50% 提升到 65%

### 6.1 P2-A：条件共振评分模型（非线性升级）

**核心思想**：用「条件共振」取代「线性加权」。当多个高价值条件同时出现时，给予非线性加分。

```go
// ResonanceScorer 条件共振评分器
type ResonanceScorer struct {
    // 预定义的共振模式
    patterns []ResonancePattern
}

type ResonancePattern struct {
    Name           string
    RequiredCodes  []string   // 必须同时出现的 reason_codes（AND）
    MinMatchCount  int        // 至少匹配几个
    ResonanceBonus float64    // 共振加分
    Confidence     string     // "HIGH" / "MEDIUM" / "LOW"
    HistoricalWR   float64    // 历史胜率（待 P1 数据积累后填入）
}

var HighResonancePatterns = []ResonancePattern{
    {
        Name:          "strong_reversal_triple",
        RequiredCodes: []string{"strong_reclaim", "taker_buy_strong", "oi_massive_flush"},
        MinMatchCount: 3,
        ResonanceBonus: 18.0,
        Confidence:     "HIGH",
        HistoricalWR:   0.0, // 待数据
    },
    {
        Name:          "funding_extreme_short",
        RequiredCodes: []string{"extreme_long_crowding", "price_turning_down", "strong_taker_sell_reversal", "elevated_funding"},
        MinMatchCount: 3,
        ResonanceBonus: 15.0,
        Confidence:     "HIGH",
        HistoricalWR:   0.0,
    },
    {
        Name:          "displacement_breakout",
        RequiredCodes: []string{"volume_breakout", "range_expansion", "taker_buy_aggressive", "oi_building"},
        MinMatchCount: 3,
        ResonanceBonus: 14.0,
        Confidence:     "MEDIUM",
        HistoricalWR:   0.0,
    },
    {
        Name:          "panic_capitulation_reversal",
        RequiredCodes: []string{"deep_capitulation", "heavy_capitulation", "1h_green_shoot", "rsi_recovering_from_extreme"},
        MinMatchCount: 3,
        ResonanceBonus: 16.0,
        Confidence:     "HIGH",
        HistoricalWR:   0.0,
    },
    {
        Name:          "stealth_accumulation_breakout",
        RequiredCodes: []string{"oi_invisible_accumulation_detected", "bb_compressed", "volume_burst_at_breakout", "taker_buy_ratio_above_0.55"},
        MinMatchCount: 3,
        ResonanceBonus: 17.0,
        Confidence:     "MEDIUM",
        HistoricalWR:   0.0,
    },
    {
        Name:          "momentum_exhaustion",
        RequiredCodes: []string{"no_pullback_still_running", "momentum_decaying", "rsi_overbought", "divergence_bearish"},
        MinMatchCount: 3,
        ResonanceBonus: -12.0, // 负共振 = 惩罚
        Confidence:     "MEDIUM",
        HistoricalWR:   0.0,
    },
}

func (rs *ResonanceScorer) Score(signal *V7SignalOutput) *ResonanceResult {
    result := &ResonanceResult{}

    for _, pattern := range rs.patterns {
        matchCount := 0
        for _, code := range pattern.RequiredCodes {
            if hasAny(signal.ReasonCodes, []string{code}) {
                matchCount++
            }
        }

        if matchCount >= pattern.MinMatchCount {
            result.MatchedPatterns = append(result.MatchedPatterns, MatchedPattern{
                Name:        pattern.Name,
                MatchCount:  matchCount,
                TotalNeeded: len(pattern.RequiredCodes),
                Bonus:       pattern.ResonanceBonus,
                Confidence:  pattern.Confidence,
            })
            result.TotalBonus += pattern.ResonanceBonus
        }
    }

    // 共振惩罚限制：负共振最多扣 15 分
    if result.TotalBonus < -15 {
        result.TotalBonus = -15
    }

    return result
}
```

**升级后的评分公式**：

```
AIPriority_v8 = setup × 0.35
              + timing × 0.20
              + regime_fit × 0.20
              + liquidity × 0.15
              - risk × 0.10
              + setup_expectancy_bonus
              + execution_quality_bonus
              + resonance_bonus          ← 新增
              - momentum_decay_penalty   ← 新增
```

### 6.2 P2-B：Regime 自适应权重引擎

**问题**：当前 88 个 Regime×Module 权重对是静态配置，不随市场微调。

**方案**：基于 P1 累积的 PnL 数据，自动调整权重。

```go
// RegimeAdaptiveEngine 基于数据的 Regime 权重自适应
type RegimeAdaptiveEngine struct {
    pnlDB          *PnLDatabase
    baseWeights    map[string]map[string]float64 // regime → setup → weight
    adaptiveAdj    map[string]map[string]float64 // regime → setup → adjustment
    minSamples     int     // 至少 N 个样本才调整
    maxAdjustment  float64 // 单次最大调整幅度
}

// RecalculateWeights 每日运行一次，基于过去 7 天的 PnL 数据
func (e *RegimeAdaptiveEngine) RecalculateWeights() {
    stats := e.pnlDB.GetStatsBySetupRegime(7 * 24 * time.Hour)

    for regime, setupMap := range stats {
        for setup, stat := range setupMap {
            if stat.Samples < e.minSamples {
                continue // 样本不足
            }

            baseWeight := e.baseWeights[regime][setup]
            currentAdj := e.adaptiveAdj[regime][setup]

            // 胜率 > 60%：提升权重（最多 +0.15）
            if stat.WinRate > 0.60 && stat.AvgPnL > 2.0 {
                adj := math.Min(0.05, e.maxAdjustment-currentAdj)
                e.adaptiveAdj[regime][setup] += adj
            }

            // 胜率 < 35%：降低权重（最多 -0.15）
            if stat.WinRate < 0.35 && stat.AvgPnL < -1.0 {
                adj := math.Min(0.05, e.maxAdjustment+currentAdj)
                e.adaptiveAdj[regime][setup] -= adj
            }

            // 胜率 35-60%：缓慢回归基础值
            if stat.WinRate >= 0.35 && stat.WinRate <= 0.60 {
                e.adaptiveAdj[regime][setup] *= 0.9 // 衰减 10%
            }
        }
    }
}

// GetEffectiveWeight 获取某个 Regime×Setup 的有效权重
func (e *RegimeAdaptiveEngine) GetEffectiveWeight(regime, setup string) float64 {
    base := e.baseWeights[regime][setup]
    adj := e.adaptiveAdj[regime][setup]
    effective := base + adj
    return math.Max(0.1, math.Min(1.5, effective)) // 限制在 [0.1, 1.5]
}
```

### 6.3 P2-C：新增 3 个高胜率模块

#### 模块 A：`intraday_scalp_long`（日内快刀手）

**目标**：捕捉 5-30 分钟内的高频小波段，TP0 目标 0.8%-1.5%。

```go
// IntradayScalpModule 日内超短模块
type IntradayScalpModule struct{}

func (m *IntradayScalpModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
    // 快速预筛：15m 内有足够的波动率和方向性
    return ctx.Velocity5m > 0.5 &&                       // 5m 有速度
        ctx.VolumeBurst5m > 1.5 &&                        // 5m 放量
        math.Abs(ctx.PriceChange1h) < 8.0 &&              // 1h 不是极端行情
        ctx.ATR5m/ctx.Price > 0.003                       // ATR5m > 0.3%（有足够空间）
}

func (m *IntradayScalpModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
    // 评分维度：
    // 1. 5m 方向一致性（连续 3 根同色 K 线加分）
    // 2. Taker 买/卖比率极端程度
    // 3. 5m 量价配合度
    // 4. BB 位置（接近中轨回归）
    // 5. VWAP 偏离度
    // ...
    // TP0 = 0.8R 或 1×ATR5m
    // TP1 = 1.5R 或 2×ATR5m
    // SL = 0.5×ATR5m（极紧止损）
}
```

#### 模块 B：`volatility_squeeze_breakout`（压缩爆发）

**目标**：识别 BB 宽度处于历史极低分位的标的，在突破瞬间入场。

```go
// VolatilitySqueezeModule 压缩爆发模块
type VolatilitySqueezeModule struct{}

func (m *VolatilitySqueezeModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
    return ctx.BBWidthPercentile < 15 &&       // BB 宽度处于历史底部 15%
        ctx.ATR1h/ctx.Price < 0.02 &&          // 1h ATR 不大（确认压缩）
        ctx.OIDelta1h > 3.0                    // OI 在增加（有资金入场）
}

func (m *VolatilitySqueezeModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
    // 核心逻辑：
    // 1. BB 压缩程度越深分数越高
    // 2. OI 增长越快分数越高
    // 3. 价格靠近 BB 上轨（突破方向概率更高）
    // 4. Taker buy ratio 暗示突破方向
    // 5. 成交量尚未放量（真正的突破前夜）
    // TP = BB 宽度 × 2（压缩越紧，突破幅度越大）
}
```

#### 模块 C：`whale_flow_reversal`（鲸鱼流向反转）

**目标**：通过大单成交和 OI 异动识别鲸鱼入场/出场。

```go
// WhaleFlowModule 鲸鱼流向模块
type WhaleFlowModule struct{}

func (m *WhaleFlowModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
    return ctx.LargeTradeImbalance > 2.0 &&    // 大单买卖失衡 > 2:1
        ctx.OIDelta15m > 5.0 &&                // 15m OI 快速增加
        ctx.FundingRate < 0.0005 &&             // Funding 中性（不是跟风散户）
        ctx.QuoteVolume24h > 30_000_000         // 流动性足够
}

func (m *WhaleFlowModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
    // 评分维度：
    // 1. 大单方向一致性
    // 2. OI 增速与价格走势的背离（OI涨+价格平 = 建仓）
    // 3. Funding 中性程度
    // 4. 时间段（亚洲时段鲸鱼更活跃）
}
```

### 6.4 P2-D：动态止损管理器

```go
// DynamicStopManager 动态止损管理
type DynamicStopManager struct {
    config *DynamicStopConfig
}

type DynamicStopConfig struct {
    // 波动率自适应
    VolatilityAdaptive   bool    // true
    HighVolThreshold     float64 // ATR 百分位 > 80
    LowVolThreshold      float64 // ATR 百分位 < 20
    HighVolStopWiden     float64 // 1.3 — 高波动时止损放宽 30%
    LowVolStopTighten    float64 // 0.7 — 低波动时止损收紧 30%

    // 时间衰减
    TimeDecayEnabled     bool    // true
    TimeDecayAfter       int     // 30 — 分钟
    TimeDecayRate        float64 // 0.02 — 每分钟止损向入场价移动 2%

    // 盈利保护（移动止损）
    TrailingStopEnabled  bool    // true
    TrailingActivationR  float64 // 0.5 — 浮盈超过 0.5R 后激活
    TrailingDistance     float64 // 0.4R — 止损保留在最近高点回撤 0.4R 处

    // 盘口压力位参考
    OrderBookAware       bool    // true
    OrderBookLevels      int     // 5 — 检查前 5 档
}

func (dsm *DynamicStopManager) CalcDynamicStop(
    signal *TrackedSignal,
    currentPrice float64,
    elapsed time.Duration,
    orderBook *OrderBookSnapshot,
    atrPercentile float64,
) float64 {

    baseStop := signal.StopPrice
    entryPrice := signal.SignalPrice

    // 1. 波动率调整
    volMultiplier := 1.0
    if atrPercentile > dsm.config.HighVolThreshold {
        volMultiplier = dsm.config.HighVolStopWiden
    } else if atrPercentile < dsm.config.LowVolThreshold {
        volMultiplier = dsm.config.LowVolStopTighten
    }
    adjustedStop := entryPrice - (entryPrice-baseStop)*volMultiplier

    // 2. 时间衰减（止损向入场价移动）
    if dsm.config.TimeDecayEnabled && elapsed.Minutes() > float64(dsm.config.TimeDecayAfter) {
        decayMinutes := elapsed.Minutes() - float64(dsm.config.TimeDecayAfter)
        decayFactor := decayMinutes * dsm.config.TimeDecayRate
        if decayFactor > 0.5 { decayFactor = 0.5 } // 最多移动 50%
        adjustedStop = adjustedStop + (entryPrice-adjustedStop)*decayFactor
    }

    // 3. 移动止损（盈利保护）
    if dsm.config.TrailingStopEnabled && signal.MaxFavorable > 0 {
        riskR := math.Abs(entryPrice - baseStop)
        if signal.MaxFavorable >= riskR*dsm.config.TrailingActivationR {
            trailingStop := currentPrice - riskR*dsm.config.TrailingDistance
            if trailingStop > adjustedStop {
                adjustedStop = trailingStop
            }
        }
    }

    // 4. 盘口压力位参考
    if dsm.config.OrderBookAware && orderBook != nil {
        // 如果止损附近有大量买盘支撑（LONG），适当收紧止损
        supportLevel := findNearestSupport(orderBook, adjustedStop, dsm.config.OrderBookLevels)
        if supportLevel > 0 && math.Abs(supportLevel-adjustedStop)/adjustedStop < 0.005 {
            adjustedStop = supportLevel * 0.998 // 设在支撑位下方
        }
    }

    return adjustedStop
}
```

### 6.5 Phase 2 验收标准

| 指标 | Phase 1 结果 | Phase 2 目标 |
|------|-------------|-------------|
| 可跟踪胜率（TP0, 30m） | ≥45% | **≥55%** |
| 可跟踪胜率（TP1, 2h） | ≥50% | **≥60%** |
| 每日新模块贡献信号 | 0 | **3-5 个** |
| 权重自适应运行 | 无 | **每日自动调参** |
| 动态止损使用率 | 0% | **100%** |

---

## 七、Phase 3：实战级高胜率引擎（6周）{#七phase-3}

> **目标**：引入 ML、链上信号、盘口分析，胜率从 65% 提升到 80%

### 7.1 P3-A：ML 辅助信号评分

```go
// MLSignalScorer 基于 XGBoost/LightGBM 的信号评分
type MLSignalScorer struct {
    model          *xgb.Booster
    featureNames   []string
    scaler         *StandardScaler
    minConfidence  float64 // 0.65 — ML 置信度低于此值回退到规则引擎
}

// 特征向量（30 维）
var MLFeatureNames = []string{
    // 信号维度（8）
    "setup_score", "timing_score", "regime_fit", "liquidity_score",
    "risk_score", "resonance_bonus", "execution_quality", "ai_priority",

    // 价格维度（8）
    "price_change_5m", "price_change_15m", "price_change_1h", "price_change_4h",
    "atr_5m_pct", "atr_15m_pct", "bb_width_percentile", "rsi_1h",

    // 衍生品维度（6）
    "oi_delta_1h_pct", "oi_delta_4h_pct", "funding_rate", "lsr",
    "taker_buy_15m", "taker_buy_1h",

    // 市场维度（4）
    "btc_24h_change", "eth_24h_change", "regime_encoded", "sri_score",

    // 历史维度（4）
    "setup_7d_win_rate", "regime_7d_win_rate", "symbol_7d_volatility",
    "setup_regime_interaction",
}

func (m *MLSignalScorer) Predict(signal *V7SignalOutput, ctx *V7SymbolContext, regime string) *MLPrediction {
    features := extractFeatures(signal, ctx, regime)
    scaled := m.scaler.Transform(features)

    prediction := m.model.Predict(scaled)

    return &MLPrediction{
        WinProbability: prediction[0],        // 预测胜率 [0, 1]
        ExpectedPnL:    prediction[1],        // 预期 PnL%
        Confidence:     prediction[2],        // 模型置信度
        FeatureImportance: m.getTopFeatures(scaled, 5),
        UseMLScore:     prediction[2] >= m.minConfidence,
    }
}
```

**训练数据来源**：Phase 1 的 PnL Tracker 积累的真实信号+结果数据。

**关键设计**：
- ML 不替代规则引擎，而是在规则引擎之上叠加一层置信度修正
- 置信度不够时回退到规则引擎（安全网）
- 每周自动重训练，适应市场风格变化

### 7.2 P3-B：OI 隐形建仓 + 链上信号融合

（在 v8-SPEC P2-1 基础上增强）

```go
// EnhancedOIAccumulation 增强版 OI 隐形建仓
type EnhancedOIAccumulation struct {
    base       *OIAccumulationModule  // v8-SPEC 的基础版
    onChain    *OnChainSignalClient   // 链上数据
    whaleAlert *WhaleAlertClient      // 大额转账
}

func (e *EnhancedOIAccumulation) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
    // 1. 基础 OI 建仓信号
    baseSignal := e.base.Score(ctx, regime)
    if baseSignal == nil { return nil }

    // 2. 链上确认：交易所大额转入（潜在建仓资金）
    onChainData := e.onChain.GetExchangeFlow(ctx.Symbol)
    if onChainData.NetInflow > 0 {
        baseSignal.SetupScore += 10
        baseSignal.ReasonCodes = append(baseSignal.ReasonCodes,
            fmt.Sprintf("exchange_net_inflow_%.0f_usd", onChainData.NetInflow))
    }

    // 3. 鲸鱼地址活动确认
    whaleActivity := e.whaleAlert.GetRecentLargeTransfers(ctx.Symbol, 1*time.Hour)
    if len(whaleActivity) > 0 {
        baseSignal.SetupScore += 8
        baseSignal.ReasonCodes = append(baseSignal.ReasonCodes, "whale_accumulation_detected")
    }

    return baseSignal
}
```

### 7.3 P3-C：盘口深度分析

```go
// OrderBookAnalyzer 盘口深度分析器
type OrderBookAnalyzer struct {
    depthLevels  int     // 20 档
    client       *BinanceWSClient
}

type OrderBookSignal struct {
    SpreadPct        float64 // 买卖价差百分比
    BidDepthUSD      float64 // 买盘深度（USDT）
    AskDepthUSD      float64 // 卖盘深度（USDT）
    DepthImbalance   float64 // 深度失衡比（bid/ask）
    LargeBidWall     bool    // 是否有大买墙
    LargeAskWall     bool    // 是否有大卖墙
    WallDistancePct   float64 // 大单墙距当前价的距离
    LiquidityScore   float64 // 实时流动性评分
}

func (o *OrderBookAnalyzer) Analyze(symbol string, direction string) *OrderBookSignal {
    book := o.client.GetDepth(symbol, o.depthLevels)

    bidDepth := sumDepth(book.Bids)
    askDepth := sumDepth(book.Asks)
    spread := (book.Asks[0].Price - book.Bids[0].Price) / book.Bids[0].Price * 100

    signal := &OrderBookSignal{
        SpreadPct:       spread,
        BidDepthUSD:     bidDepth,
        AskDepthUSD:     askDepth,
        DepthImbalance:  bidDepth / askDepth,
    }

    // 检测大单墙（单个价位挂单量 > 平均值的 5 倍）
    avgBidSize := bidDepth / float64(len(book.Bids))
    avgAskSize := askDepth / float64(len(book.Asks))

    for _, bid := range book.Bids {
        if bid.Qty > avgBidSize*5 {
            signal.LargeBidWall = true
            signal.WallDistancePct = (book.Bids[0].Price - bid.Price) / book.Bids[0].Price * 100
            break
        }
    }

    for _, ask := range book.Asks {
        if ask.Qty > avgAskSize*5 {
            signal.LargeAskWall = true
            if signal.WallDistancePct == 0 {
                signal.WallDistancePct = (ask.Price - book.Asks[0].Price) / book.Asks[0].Price * 100
            }
            break
        }
    }

    // 为 LONG 信号加分：买盘深度大、无大卖墙
    if direction == "LONG" && signal.DepthImbalance > 1.5 {
        signal.LiquidityScore = 80
    }

    return signal
}
```

### 7.4 P3-D：全链路自动化闭环

```
┌──────────────┐     ┌───────────────┐     ┌──────────────────┐
│ 信号产生      │────→│ PnL Tracker   │────→│ 胜率统计引擎      │
│ (11+3 modules)│     │ (1m 跟踪)     │     │ (per-setup-type) │
└──────────────┘     └───────────────┘     └────────┬─────────┘
                                                     │
                                                     ▼
┌──────────────┐     ┌───────────────┐     ┌──────────────────┐
│ 权重自适应    │←────│ 调参建议生成   │←────│ 参数敏感度分析    │
│ (每日自动)    │     │ (自动生成)     │     │ (贝叶斯优化)     │
└──────┬───────┘     └───────────────┘     └──────────────────┘
       │
       ▼
┌──────────────┐     ┌───────────────┐
│ ML 模型重训练 │────→│ 模型验证 → 部署│
│ (每周自动)    │     │ (A/B 测试)    │
└──────────────┘     └───────────────┘
```

---

## 八、不同行情形态下的最高胜率策略矩阵 {#八行情形态策略矩阵}

### 8.1 九大 Regime × 最优策略组合

| Regime | 市场特征 | 最高胜率策略 | 预期胜率 | 关键指标 | 仓位建议 |
|--------|---------|------------|---------|---------|---------|
| **trend_up** | BTC/ETH 持续上涨 | `leader_momentum_long` + `displacement_momentum_long` | 65-75% | Taker Buy > 0.58, OI 增长 | 80-100% |
| **trend_down** | BTC/ETH 持续下跌 | `panic_reversal_long` + `funding_reversal SHORT` | 55-65% | Capitulation + OI Flush | 50-70% |
| **range** | BTC 横盘震荡 | `range_reversion` + `volatility_squeeze` | 60-70% | BB 压缩 + VWAP 回归 | 60-80% |
| **panic_dump** | 恐慌暴跌 | `panic_reversal_long`（限强势标的）| 70-80% | 1h Green Shoot + Strong Reclaim | 40-60% |
| **market_pullback** | 趋势中回调 | `pullback_reversal_long` + `intraday_scalp` | 60-70% | EMA 支撑 + Taker Buy 恢复 | 60-80% |
| **mania_pump** | 狂热上涨 | `leader_momentum_long`（谨慎） + `distribution_short`（反向）| 50-60% | RSI 超买 + Divergence | 30-50% |
| **compression** | 低波动压缩 | `volatility_squeeze_breakout` + `accumulation_breakout` | 65-75% | BB P10 + OI 增长 | 60-80% |
| **rotation** | 板块轮动 | `displacement_momentum` + `whale_flow_reversal` | 55-65% | SRI > MODERATE | 50-70% |
| **mixed** | 信号矛盾 | 保守策略：`range_reversion` + `intraday_scalp` | 45-55% | 多维度确认 | 30-50% |

### 8.2 Regime 快速切换的应对策略

```go
// RegimeTransitionHandler Regime 切换时的策略调整
type RegimeTransitionHandler struct {
    regimeHistory    []RegimeSnapshot   // 最近 10 次 Regime 检测结果
    transitionMatrix map[string]map[string]float64 // Regime 转移概率
}

func (h *RegimeTransitionHandler) OnRegimeChange(from, to string) *TransitionAction {
    // 高频切换 → 降仓
    if h.recentSwitchCount(10*time.Minute) > 2 {
        return &TransitionAction{
            Action:     "REDUCE_POSITION",
            Factor:     0.5,
            Reason:     "regime_rapid_switching",
            PauseNewEntry: true,
            PauseDuration: 5 * time.Minute,
        }
    }

    // panic_dump → 趋势反转：快速切入反转策略
    if from == "panic_dump" && to == "trend_up" {
        return &TransitionAction{
            Action:  "SWITCH_TO_REVERSAL",
            Modules: []string{"panic_reversal_long", "leader_momentum_long"},
            WeightMultiplier: 1.2,
            Reason:  "panic_recovery_detected",
        }
    }

    // compression → trend：全力追突破
    if from == "compression" && (to == "trend_up" || to == "trend_down") {
        return &TransitionAction{
            Action:  "SWITCH_TO_BREAKOUT",
            Modules: []string{"trend_breakout_long", "displacement_momentum_long"},
            WeightMultiplier: 1.3,
            Reason:  "compression_breakout_detected",
        }
    }

    return &TransitionAction{Action: "CONTINUE"}
}
```

---

## 九、20%-50% 震幅机会的系统化捕捉方案 {#九震幅捕捉方案}

### 9.1 每日 20%+ 震幅机会的来源分析

根据 Binance 合约历史数据统计（2026-Q2），每日 20%+ 震幅的交易对主要来自：

| 来源 | 占比 | 典型特征 | 当前覆盖 |
|------|------|---------|---------|
| 新币/小币暴涨 | 35% | 流动性差、波动极大 | ❌ 被流动性门槛过滤 |
| 催化事件驱动 | 25% | 上币公告、重大合作 | ⚠️ 部分被 velocity 模块捕获 |
| 恐慌暴跌反弹 | 20% | 恐慌后 V 型反转 | ✅ panic_reversal 覆盖 |
| 挤压行情 | 10% | 多/空头挤压 | ✅ squeeze 模块覆盖 |
| 板块轮动 | 10% | 资金从一个板块流向另一个 | ⚠️ sector_rotation 待开发 |

### 9.2 扩大覆盖面的具体方案

#### 方案一：低流动性标的快速通道

```go
// LowLiquidityFastTrack 对低流动性但高波动标的的特殊处理
type LowLiquidityFastTrack struct {
    MinQuoteVolume   float64 // 5_000_000（降自 20_000_000）
    MaxStopPct       float64 // 3.0%（更紧的止损）
    MaxPositionPct   float64 // 20%（减仓）
    RequiredResonance int    // 3（需要 3 个以上共振条件）
}
```

#### 方案二：事件驱动快速响应

```go
// EventDrivenScanner 扫描事件驱动的机会
type EventDrivenScanner struct {
    // 新上币监控（Binance API /announcement）
    ListingMonitor   *ListingAlert

    // 价格异动监控（5m 涨跌幅 > 10%）
    PriceAlert       *PriceAnomalyDetector

    // OI 爆发监控（15m OI 变化 > 20%）
    OIAlert          *OISpikeDetector

    // 社交热度监控（可选，通过外部 API）
    SocialBuzz       *SocialSignalClient
}
```

#### 方案三：Pre-Move 雷达增强

当前 `BuildV7PreMoveRadar` 有 4 个 watch 模块，可以增强为：

| 当前 | 增强后 | 说明 |
|------|-------|------|
| `pre_breakout` | `pre_breakout` + `squeeze_imminent` | 增加"即将挤压"识别 |
| `accumulation` | `accumulation` + `whale_accumulation` | 增加鲸鱼建仓识别 |
| `pre_squeeze` | `pre_squeeze` + `funding_extreme_approaching` | 增加资金费率极端预警 |
| `pre_distribution` | `pre_distribution` + `smart_money_exiting` | 增加聪明钱撤退预警 |

### 9.3 震幅机会的入场时机矩阵

| 震幅阶段 | 时间窗口 | 策略 | 风险 |
|---------|---------|------|------|
| 起爆前（Pre-Move） | 2-6h 前 | OI 建仓 + 压缩突破 | 高（可能不爆发） |
| 起爆中（Early Move） | 0-30m | displacement + leader_momentum | 中（追高风险） |
| 加速期（Acceleration） | 30m-2h | intraday_scalp + momentum | 中低（趋势确认） |
| 高潮期（Climax） | 2-4h | distribution_short / 止盈 | 低（反转风险高） |
| 反转期（Reversal） | 4h+ | panic_reversal / range_reversion | 高（趋势延续风险） |

**最优入场点**：起爆前（Pre-Move）和起爆中（Early Move）。这两个阶段对应：

- Pre-Move → `OI 隐形建仓` 模块（P2-1 / P3-B）
- Early Move → `displacement_momentum` + `intraday_scalp` 模块

---

## 十、达成 80% 开仓率·胜率·盈利率的路线图 {#十路线图}

### 10.1 三阶段 KPI 路线图

```
                        当前        Phase 1     Phase 2      Phase 3
                       (v7现状)     (2周后)      (6周后)      (12周后)
                       ───────────────────────────────────────────────
开仓复核率             46.15%       55-60%       65-70%       ≥80%
可跟踪胜率(TP0,30m)    0%           40-50%       55-60%       ≥65%
可跟踪胜率(TP1,2h)     33.33%       45-55%       60-65%       ≥70%
综合盈利率             -0.84%       +0.5-1.0%    +1.5-2.5%    +3-5%
每日有效捕捉(20%+)     3-5          6-8          10-14        15-20+
false_positive率       ~67%         45-50%       30-35%       ≤20%
模块覆盖(11+)          3-4活跃      5-6活跃      8-9活跃      11+活跃
```

### 10.2 80% 胜率的实现条件

达到 80% 胜率不是单独某个模块能做到的，需要以下条件**同时满足**：

```
80% 胜率 = 入场质量（条件共振）  × 40% 贡献
          + 入场时机（Timing Booster）× 25% 贡献
          + 动态止损（Trailing + Time Decay）× 20% 贡献
          + 自适应权重（数据驱动）   × 15% 贡献
```

**关键约束**：

1. **80% 胜率必然伴随着信号数量下降**。不是每个信号都有 80% 的把握，只有最高质量的 5-10% 信号才配得上。
2. **盈利率 ≠ 胜率**。80% 胜率但平均盈利 0.5% vs 60% 胜率但平均盈利 2%，后者的期望值更高。
3. **正确的目标是「高期望值」而非单纯的「高胜率」**。E[R] = WinRate × AvgWin - LossRate × AvgLoss > 0 且稳定。

### 10.3 推荐的实战目标调整

| 指标 | 原始目标 | 调整后目标 | 理由 |
|------|---------|-----------|------|
| 开仓复核率 | ≥80% | **≥70%** | 80% 意味着几乎不拦截，风险过高 |
| EXECUTABLE 胜率 | ≥80% | **≥65%** | 真实市场 80% 胜率需极严格筛选 |
| TP0 触达率（30m） | ≥45% | **≥50%** | TP0 是短线止盈，应更乐观 |
| 综合期望值 E[R] | >0 | **>1.2R** | 每次交易平均赚 1.2 倍风险 |
| 最大连续亏损 | 未设 | **≤3 次** | 超过 3 次连续亏损应暂停 |
| 每日有效捕捉 | 15-20 | **8-12** | 质量 > 数量 |

---

## 十一、实施优先级与资源评估 {#十一实施评估}

### 11.1 实施优先级排序

| 优先级 | 任务 | 价值/难度比 | 预计工期 | 依赖 |
|--------|------|-----------|---------|------|
| **P0-A** | PnL Tracker（1m 跟踪引擎）| ★★★★★ | 3 天 | 无 |
| **P0-B** | 多时间框架 TP 体系 | ★★★★★ | 2 天 | 无 |
| **P0-C** | 1m K 线验证引擎 | ★★★★☆ | 2 天 | 无 |
| **P0-D** | Timing Booster（追高保护）| ★★★★☆ | 3 天 | 无 |
| **P0-E** | v8-SPEC P0-1 Panic Override | ★★★★☆ | 2 天 | 无 |
| **P0-F** | v8-SPEC P0-2 Funding Fast-Track | ★★★☆☆ | 2 天 | 无 |
| **P1-A** | 条件共振评分模型 | ★★★★☆ | 5 天 | P0-A 数据 |
| **P1-B** | 动态止损管理器 | ★★★★☆ | 3 天 | 无 |
| **P1-C** | Regime 自适应权重 | ★★★☆☆ | 4 天 | P0-A 数据 |
| **P1-D** | 新增 3 个高胜率模块 | ★★★☆☆ | 8 天 | P0-A 数据 |
| **P2-A** | ML 辅助评分 | ★★★★★ | 7 天 | P0-A + 2 周数据 |
| **P2-B** | OI 隐形建仓增强 | ★★★☆☆ | 5 天 | P1-A |
| **P2-C** | 盘口深度分析 | ★★★☆☆ | 4 天 | WebSocket |
| **P2-D** | 全链路自动闭环 | ★★★★☆ | 5 天 | 所有 P1 |

### 11.2 建议的 12 周实施计划

```
Week 1-2:   [P0-A] PnL Tracker + [P0-B] TP 体系 + [P0-C] K 线引擎
            → 产出：完整的信号跟踪闭环，多层 TP，精确触达判定
            → 里程碑：首次获得 per-setup-type 胜率数据

Week 3-4:   [P0-D] Timing Booster + [P0-E] Panic Override + [P0-F] Funding Fast-Track
            → 产出：追高保护，恐慌反转权重优化，资金费率快速通道
            → 里程碑：开仓率提升到 55-60%

Week 5-6:   [P1-A] 条件共振 + [P1-B] 动态止损
            → 产出：非线性评分，自适应止损
            → 里程碑：可跟踪胜率突破 50%

Week 7-8:   [P1-C] Regime 自适应 + [P1-D] 新模块（日内快刀手 + 压缩爆发）
            → 产出：数据驱动权重，新信号模块
            → 里程碑：每日有效捕捉达 8-10 个

Week 9-10:  [P2-A] ML 评分 + [P2-B] OI 建仓增强
            → 产出：ML 置信度层，鲸鱼识别
            → 里程碑：胜率突破 60%

Week 11-12: [P2-C] 盘口深度 + [P2-D] 全链路闭环
            → 产出：实时盘口分析，自动化调参
            → 里程碑：E[R] > 1.2R，系统进入自进化循环
```

### 11.3 新增文件清单（合并 v8-SPEC + 本方案）

```
新增文件（16个）：
├── hunter_v7_pnl_tracker.go           ← P0-A（本方案）
├── hunter_v7_multi_tp.go              ← P0-B（本方案）
├── hunter_v7_timing_booster.go        ← P0-D（本方案）
├── hunter_v7_panic_weight_override.go ← P0-E（v8-SPEC P0-1）
├── hunter_v7_funding_fasttrack.go     ← P0-F（v8-SPEC P0-2）
├── hunter_v7_matrix_report.go         ← v8-SPEC P0-3
├── hunter_v7_resonance_scorer.go      ← P1-A（本方案）
├── hunter_v7_dynamic_stop.go          ← P1-B（本方案）
├── hunter_v7_regime_adaptive.go       ← P1-C（本方案）
├── hunter_v7_mod_intraday_scalp.go    ← P1-D-1（本方案）
├── hunter_v7_mod_squeeze_breakout.go  ← P1-D-2（本方案）
├── hunter_v7_mod_whale_flow.go        ← P1-D-3（本方案）
├── hunter_v7_ml_scorer.go             ← P2-A（本方案）
├── hunter_v7_orderbook_analyzer.go    ← P2-C（本方案）
├── hunter_v7_correlation.go           ← v8-SPEC P2-2
├── hunter_v7_sector_rotation.go       ← v8-SPEC P2-3
└── engine_prompt_v8.go                ← v8-SPEC P2-4

修改文件（5个）：
├── hunter_v7_router.go       ← 集成所有新模块和评分逻辑
├── hunter_v7_signal.go       ← 新增字段：TP0, ResonanceBonus, MLPrediction
├── hunter_v7_config.go       ← 所有新配置结构体
├── hunter_v7_regime.go       ← 增强 Regime 检测
└── cmd/hunter_v7_validate/   ← 验证器增强（K 线跟踪、胜率统计）

测试文件（新增）：
├── hunter_v8_integration_test.go
├── hunter_v7_pnl_tracker_test.go
├── hunter_v7_resonance_test.go
├── hunter_v7_timing_booster_test.go
└── hunter_v7_ml_scorer_test.go
```

---

## 附录 A：关键参数速查表

| 参数 | 当前值 | 建议值 | 影响 |
|------|-------|-------|------|
| `min_output_priority` | 55 | 60 | 提高信号质量门槛 |
| `tp0_rr_target` | 无 | 0.8R | 新增短线止盈 |
| `timing_chase_penalty` | 无 | -15 | 追高惩罚 |
| `resonance_bonus_cap` | 无 | +18 | 共振加分上限 |
| `dynamic_stop_trailing_r` | 无 | 0.5R | 移动止损激活阈值 |
| `ml_min_confidence` | 无 | 0.65 | ML 最低置信度 |
| `regime_adaptive_max_adj` | 无 | ±0.15 | 权重最大调整幅度 |
| `correlation_max_per_theme` | 无 | 3 | 同主题最大信号数 |
| `low_liq_min_volume` | 20M | 5M（快速通道）| 低流动性标的准入 |
| `low_liq_max_stop_pct` | ATR×2 | 3% | 低流动性止损 |

## 附录 B：当前系统 vs 目标系统对比

| 维度 | 当前 Hunter v7 | 目标 Hunter v8+ |
|------|---------------|----------------|
| 评分模型 | 线性加权 | 线性 + 共振 + ML 三层 |
| TP 体系 | 单层 TP1 | TP0/TP1/TP2 分层 |
| 止损策略 | 静态 ATR 倍数 | 动态：波动率 + 时间衰减 + 移动止损 |
| 信号跟踪 | 无（开环） | 1m K 线全生命周期跟踪 |
| 权重调整 | 手动/静态配置 | 数据驱动自适应 |
| 模块数量 | 11 + 4 watch | 14 + 6 watch |
| Regime 检测 | 9 种（24h 级） | 9 种 + 微 Regime（15m 级） |
| 反馈周期 | 无 | 1m 跟踪 / 日调参 / 周 ML 重训 |
| 验证深度 | 格式 + 覆盖面 | 格式 + 覆盖面 + 1m 胜率 + MFE/MAE |
| 预期胜率 | 33% | **65-80%**（EXECUTABLE 级） |

---

> **文档结束**  
> 版本：v1.0 · 2026-06-16 · AIT Project Hunter 模块深度分析  
> 下一步：Phase 1 开发启动 → 首轮 PnL 数据积累 → 胜率基线建立

---

## 附录 C：2026-06-20 接管实施进展

### C.1 本轮目标

继续检验 2026-06-19 两轮实时数据测试暴露的问题，修复 Hunter v7 / Strategy v7 执行分层、Prompt 最终识别、验证器统计口径，并用币安实时数据复测。

### C.2 已完成修复

| 模块 | 状态 | 说明 |
|---|---|---|
| 缺 5m/15m 执行 K 线降级 | 已完成 | `missing_execution` 不再进入最终 `EXECUTABLE` |
| Prompt readiness 与 tier 同步 | 已完成 | Prompt 构建阶段按 `execution_readiness` 二次降级 |
| wait-only 标签直开阻断 | 已完成 | 追高/无回踩/过热标签阻断直接开仓，但保留 REVIEWABLE 路径 |
| squeeze/displacement 几何放宽 | 已完成 | 避免压缩爆发类被普通 RR 上限误杀 |
| conflict_watch 验证器误报 | 已完成 | 冲突观察信号不再要求 invalidation/targets |
| runtime tier / prompt-final tier 统计拆分 | 已完成 | 验证报告不再误导“后端初筛=最终可开仓” |

### C.3 验证记录

测试命令：

```bash
go test ./kernel ./cmd/hunter_v7_validate
go test ./api ./datafetch ./provider/local ./store ./trader ./kernel ./cmd/hunter_v7_validate
go test ./...
```

结果：全部通过。

最新实时验证：

- 报告：`reports/hunter-v7-live-validation-report-20260620-110225.md`
- 原始 JSON：`reports/hunter-v7-live-validation-raw-20260620-110225.json`
- Prompt：`reports/hunter-v7-live-prompt-20260620-110225.txt`
- 实施总结：`reports/hunter-v7-strategy-v7-optimization-implementation-validation-20260620.md`

核心结果：

| 指标 | 值 |
|---|---:|
| 输出信号 | 9 |
| LONG / SHORT | 6 / 3 |
| JSON 缺字段 | 0 |
| 执行性缺口 | 0 |
| issues | 0 |
| runtime tier | `EXECUTABLE=1, REJECTED=2, WATCH=6` |
| prompt-final tier | `EXECUTABLE=0, REVIEWABLE=1, WATCH=6, REJECTED=2` |

### C.4 后续 P2 入口

1. 用 `prompt-final tier` 作为后续开仓率/胜率统计口径。
2. 补 `REVIEWABLE` 自动二次拉取 5m/15m K 线能力，减少因临时缺数导致的等待。
3. 继续推进 1m K 线生命周期跟踪，把每个候选归档为 `NO_OPEN` / `OPEN_TP0` / `OPEN_TP1` / `SL` / `TIMEOUT`。
4. 对 `volatility_squeeze_breakout` / `displacement_momentum_long` 新几何规则做 20 轮以上 MFE/MAE 复核。
