# Hunter 妖币猎杀模式 (Breakout Mode) 设计规格

## 概述

在现有 Hunter 选币引擎中新增 `strategy_mode: "breakout"` 分支，将 Hunter 从"情绪雷达"升级为"波动率追踪器"。通过 BB Width 分层漏斗、阻力位条件豁免、Squeeze 支柱增强、单骑破阵门控四个维度改造，使 Hunter 能精准定位"极度缩口、即将爆发"的妖币标的。

## 改动范围

| 文件 | 改动类型 | 改动内容 |
|------|---------|---------|
| `store/strategy.go` | 修改 | HunterConfig 新增 4 字段 + 2 常量 |
| `web/src/types/strategy.ts` | 修改 | HunterConfig 新增 4 字段 |
| `provider/local/hunter.go` | 修改 | Phase 1 硬过滤、Phase 2 加分、near_resistance 豁免、E' cap 提升、门控单骑破阵 |
| `provider/local/cache.go` | 修改 | 新增 BBWidthPercentileCache |
| `web/src/components/strategy/CoinSourceEditor.tsx` | 修改 | 策略模式切换 UI |

总改动量：约 +330 行，5 个文件。

---

## 第一部分：BB Width 分层漏斗

### Phase 1：硬过滤 + 单骑破阵特权

在 `GetHunterList()` Top-50 候选生成之后、评分循环之前插入。

逻辑：
- `strategy_mode == "breakout"` 时启用
- 遍历候选，调用 `getBBWidthPercentile(symbol)` 获取 24h BB Width 百分位
- `percentile > 25%` → 检查单骑破阵特权（`1h OI delta > 15%`）
  - 满足 → 强制放行
  - 不满足 → 硬剔除，不进入评分
- `percentile <= 25%` → 正常进入评分

### Phase 2：E' 支柱加分（仅 breakout 模式）

在 `computeSqueezeExplosionPillar()` 内部追加 breakout 分支，必须有 `config.StrategyMode == "breakout"` 前置条件：

| 条件 | 加分 | 标签 |
|------|------|------|
| percentile <= 5% | +20 | bb_width_extreme_squeeze |
| percentile <= 10% | +12 | bb_width_tight |
| percentile <= 20% | +5 | bb_width_narrow |

### 缓存层

- `BBWidthPercentileCache` TTL = 180s（3分钟）
- `getBBWidthPercentile(symbol)` 先查缓存，miss 时调用 `computeBBWidthPercentile()`
- `preheatBBWidthCache(symbols)` 在入口处批量预热（goroutine 并发，实盘如出现 timeout 则加 semaphore 限流）
- `computeBBWidthPercentile()`: 拉取 5m klines × 288 bars（24h），逐 bar 计算 BB Width（period=20, mult=2.0），返回当前 Width 在序列中的排名百分位

---

## 第二部分：near_resistance 条件豁免 + Squeeze 支柱增强

### 2.1 near_resistance_4h 条件豁免

改动位置：`computePositionScore()` (LONG) 和 `computeShortPositionScore()` (SHORT)

LONG 方向：当 `near_resistance_4h` 触发时（high4h - price <= 2*ATR14），检查是否有 BB squeeze 信号（bb_squeeze_15m / bb_squeeze_5m / bb_width_extreme_squeeze / bb_width_tight）：
- 有 squeeze → 豁免 -25 惩罚，打标签 `near_resistance_exempt`
- 无 squeeze → 正常扣分

SHORT 方向：对称逻辑，`near_support_4h_penalize` 同样被 squeeze 信号豁免。

**依赖顺序**：`computePositionScore()` 先于 `computeSqueezeExplosionPillar()` 执行。解决方案：在评分循环中先预计算 BB squeeze 标签（`computeBBTags()`），注入 tags，再执行 Position Score。

评分循环重组：
```
Step 1: computeBBTags(symbol) → 预计算 BB squeeze 标签
Step 2: computePositionScore(symbol, bbTags, config) → 可读取 bbTags 做豁免
Step 3: computeOISmartScore(symbol)
Step 4: computeSmartMoneyScore(symbol)
Step 5: computeSqueezeExplosionPillar(symbol, bbTags, config) → 含 breakout 加分
```

### 2.2 Squeeze 支柱 cap 提升

breakout 模式下 E' 支柱 cap 从 50 → 70。

理论最高：bb_squeeze_15m(+25) + oi_spike(+25) + synergy(+10) + bb_width_extreme(+20) = 80 → cap 70。

breakout 模式总分上限：50(pos+oi) + 42.25(sm) + 70(squeeze) = 162.25（vs default 125）。

---

## 第三部分：宁缺勿滥门控改造 — 单骑破阵旁路

改动位置：`GetHunterList()` 门控判断处

### 现有逻辑（保留）
```
longStrongCount >= 2 || shortStrongCount >= 2 → pass
```

### 新增单骑破阵旁路（仅 breakout 模式）

条件（全部满足才触发）：
1. `1h OI delta > 15%`
2. 至少触发 bb_squeeze 或 bb_width_tight 以上
3. `FinalScore >= minHunterScore` (15.0)
4. `WashMultiplier > 0.20`（排除刷量币）

触发时：
- 无视群体门控，强制放行
- 日志输出 `[Hunter/breakout] 单骑破阵放行: %s score=%.1f oi_delta=%.2f%%`
- 输出标签 `lone_breaker_pass`

输出处理：
- 正常门控通过 → 正常输出
- 单骑破阵通过 → 输出该币 + 其他 FinalScore >= 30 的候选

---

## 第四部分：HunterConfig 改造 + 前端 UI

### Go 后端 Config 扩展

`HunterConfig` 新增字段：
```go
StrategyMode           string   `json:"strategy_mode"`           // "default" | "breakout"
BBWidthCoarseFilter    float64  `json:"bb_width_coarse_filter"`  // 默认 25
BBWidthCacheTTL        int      `json:"bb_width_cache_ttl"`      // 默认 180
OILoneBreakerThreshold float64  `json:"oi_lone_breaker_threshold"` // 默认 15
```

新增常量：
```go
StrategyModeDefault  = "default"
StrategyModeBreakout = "breakout"
```

注意：`BBWidthCoarseFilter`、`BBWidthCacheTTL`、`OILoneBreakerThreshold` 第一版不暴露到前端，仅预留扩展。

### 前端 UI

在 Hunter 高级选项面板最顶部新增策略模式切换：
- 🎯 默认模式：均值回归 + 情绪反转
- 🔥 妖币猎杀：波动率挤压 → 爆发射射

breakout 模式激活时显示功能说明浮层：
- BB Width 漏斗预筛（仅最低 25% 分位进入评分）
- 阻力位/支撑位惩罚被 squeeze 信号豁免
- Squeeze 爆发支柱上限提升至 70 分
- OI >15% + squeeze = 单骑破阵放行

---

## 数据流总览

```
Top-50 候选
    │
    ▼
Phase 1: BB Width 硬过滤 (breakout only)
    │  percentile > 25% → DROP (除非 OI>15% 单骑破阵)
    │
    ▼
评分循环
    ├─ Step 1: computeBBTags() → 预计算 squeeze 标签
    ├─ Step 2: Position Score (squeeze 在场 → 豁免 -25)
    ├─ Step 3: OI Smart Score (不变)
    ├─ Step 4: Smart Money Score (不变)
    └─ Step 5: Squeeze/Explosion (cap 70, +bb_width 加分)
    │
    ▼
宁缺勿滥门控
    │  标准: LONG≥2 || SHORT≥2
    │  旁路: OI>15% + squeeze + score≥15 + wash>0.20
    │
    ▼
输出 Top-30 + 信号标签 + lone_breaker_pass 标记
```

---

*设计日期: 2026-05-24*
*实现方案: A — 模式分支嵌入现有函数*
*影响范围: Hunter 选币引擎，5 个文件，约 330 行改动*
