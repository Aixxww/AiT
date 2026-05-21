# Hunter 自动交易 Agent v2 — 自进化策略架构

## 结论：✅ 技术可行，核心升级：策略自行迭代 + 智能追踪止盈

---

## 一、整体架构 (v2)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     Claude Code Agent (24/7 Loop)                       │
│                                                                          │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐   ┌──────────────────┐ │
│  │ CronLoop │──▶│ Hunter   │──▶│ AI 策略决策   │──▶│ Binance CLI      │ │
│  │ 5min轮询  │   │ /api/    │   │ 自适应提示词  │   │ binance-skills-  │ │
│  │          │   │ hunter/  │   │ + 历史战绩反馈 │   │ hub/binance      │ │
│  └──────────┘   │ coins    │   └──────┬───────┘   └────────┬─────────┘ │
│                  └──────────┘          │                     │           │
│                                        ▼                     ▼           │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    自进化引擎 (Self-Evolution Engine)              │   │
│  │                                                                   │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌──────────────────────────┐ │   │
│  │  │ 交易日志库   │  │ 参数调优器   │  │ 智能追踪止盈管理器        │ │   │
│  │  │ trades.jsonl │  │ tuner       │  │ Smart Trailing Manager   │ │   │
│  │  │ 每笔记录:    │  │ 每日分析:    │  │ 每1min检查:              │ │   │
│  │  │ - entry/exit │  │ - 胜率趋势   │  │ - ATR动态止盈            │ │   │
│  │  │ - reason     │  │ - 信号有效性  │  │ - 利润锁定阶梯           │ │   │
│  │  │ - tags       │  │ - 最佳时段   │  │ - 波动率自适应           │ │   │
│  │  │ - pnl        │  │ - 失败模式   │  │                          │ │   │
│  │  └─────────────┘  └─────────────┘  └──────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │              数据源层                                              │   │
│  │  AiT :8080   |   Binance API   |   NofxOS Quant   |   Telegram  │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 二、自进化引擎设计

### 2.1 交易日志 (trades.jsonl)

每笔交易完整记录，供后续分析优化：

```json
{
  "id": "t_20260521_001",
  "symbol": "NVDAUSDT",
  "direction": "long",
  "entry_price": 224.03,
  "entry_time": "2026-05-21T14:30:00Z",
  "exit_price": 231.50,
  "exit_time": "2026-05-21T16:45:00Z",
  "pnl_pct": 3.33,
  "pnl_usd": 166.50,
  "hold_minutes": 135,
  "leverage": 5,
  "hunter_score": 36.25,
  "hunter_tags": ["oi_price_aligned", "lsr_bullish", "lsr_extreme_bearish"],
  "resonance_count": 4,
  "entry_reason": "4柱共振: 支撑位+OI对齐+LSR反转+Taker买入",
  "exit_reason": "trailing_stop_hit",
  "peak_pnl_pct": 4.8,
  "trailing_stop_price": 230.20,
  "market_context": {
    "btc_rsi14": 62,
    "btc_trend": "bullish",
    "funding_rate": 0.008,
    "market_volatility": "medium"
  }
}
```

### 2.2 每日参数调优器

**触发**: 每天 UTC 00:05，分析过去 7 天交易数据

**分析维度**:

| 分析项 | 数据来源 | 调优动作 |
|--------|----------|----------|
| 信号有效性 | 按 hunter_tags 分组统计胜率 | 淘汰胜率<50%的标签组合 |
| 最佳时段 | 按小时统计胜率和盈亏 | 调整活跃交易时段 |
| 止盈表现 | peak_pnl vs exit_pnl 差值 | 调整追踪止盈参数 |
| 止损表现 | 被止损后价格是否反转 | 调整止损宽度 |
| 杠杆表现 | 按杠杆分组统计收益 | 调整默认杠杆 |
| BTC 关联 | BTC 跌时做多胜率 | 增加 BTC 环境过滤 |

**输出**: 更新 `strategy_params.json`，下一轮自动使用新参数

```json
{
  "updated_at": "2026-05-21T00:05:00Z",
  "params": {
    "min_confidence": 75,
    "min_resonance": 3,
    "preferred_tags": ["oi_price_aligned", "lsr_reversal", "taker_sustained_buying"],
    "avoided_tags": ["lsr_bearish"],
    "preferred_hours_utc": [6, 7, 8, 13, 14, 15],
    "avoid_hours_utc": [0, 1, 2],
    "default_leverage": 5,
    "btc_filter": {"rsi14_below": 30, "action": "skip_long"},
    "trailing_atr_multiplier": 2.0,
    "trailing_profit_lock": [0.5, 0.7, 0.9],
    "stop_loss_pct": -3.5
  },
  "stats_7d": {
    "total_trades": 28,
    "win_rate": 0.71,
    "avg_win": 2.8,
    "avg_loss": -2.1,
    "profit_factor": 2.1,
    "max_drawdown": 8.5
  }
}
```

---

## 三、智能追踪止盈 (Smart Trailing Stop)

### 3.1 核心问题

传统追踪止损的困境：
- **太紧** → 正常波动被打掉，错过大行情
- **太松** → 利润大幅回吐

### 3.2 解决方案：三阶段动态追踪

```
利润增长 →→→→→→→→→→→→→→→→→→→→→→→→→→→→→→→→
         |  阶段1     |    阶段2      |   阶段3    |
         |  宽松期     |    锁定期     |   猎利期   |
         |  ATR×3.0   |    ATR×2.0   |   ATR×1.5  |
         |  0→+2%     |    +2%→+5%  |   >+5%     |
         |  给空间     |    锁利润     |   保利润    |
```

#### 阶段 1: 宽松期 (0% ~ +2%)

```python
trailing_distance = ATR14 × 3.0
# 目的: 给趋势足够展开空间
# 触发: 刚开仓，利润尚未确认
# 如果被打掉: 正常止损，不心疼
```

#### 阶段 2: 锁定期 (+2% ~ +5%)

```python
trailing_distance = ATR14 × 2.0
# 锁定阶梯: 利润每增1%，止盈线上移
lock_floor = entry_price + (peak_pnl × 0.5) × entry_price / 100
# 例: 峰值+4% → 锁定至少+2%利润
# 目的: 至少保住一半利润
```

#### 阶段 3: 猎利期 (>+5%)

```python
trailing_distance = ATR14 × 1.5
# 90%利润锁定: 几乎不给回吐空间
lock_floor = entry_price + (peak_pnl × 0.9) × entry_price / 100
# 例: 峰值+8% → 锁定至少+7.2%
# 目的: 大行情来了绝不吐回去
```

### 3.3 ATR 自适应 (波动率感知)

```python
# 实时 ATR 相对于历史均值的比例
atr_ratio = current_ATR14 / avg_ATR_7d

if atr_ratio > 1.5:    # 高波动期
    trailing_distance *= 1.3   # 放宽30%，避免假突破打掉
    log("⚠️ 高波动期，止盈放宽30%")

elif atr_ratio < 0.7:  # 低波动期
    trailing_distance *= 0.85  # 收紧15%，更快锁定利润
    log("📊 低波动期，止盈收紧15%")
```

### 3.4 利润保护阶梯

```python
# 实现逻辑 (每1分钟检查)
def update_trailing_stop(position):
    pnl_pct = (current_price - entry_price) / entry_price * 100 * leverage

    # 更新峰值
    if pnl_pct > position.peak_pnl:
        position.peak_pnl = pnl_pct

    # 确定阶段
    if position.peak_pnl < 2.0:
        # 阶段1: 宽松
        distance = atr14 * 3.0
        min_lock = entry_price  # 不锁定利润
    elif position.peak_pnl < 5.0:
        # 阶段2: 锁定
        distance = atr14 * 2.0
        lock_pct = position.peak_pnl * 0.5
        min_lock = entry_price * (1 + lock_pct / 100)
    else:
        # 阶段3: 猎利
        distance = atr14 * 1.5
        lock_pct = position.peak_pnl * 0.9
        min_lock = entry_price * (1 + lock_pct / 100)

    # ATR 波动率调整
    atr_ratio = current_atr / avg_atr_7d
    if atr_ratio > 1.5:
        distance *= 1.3

    # 新止盈价 = max(当前追踪价, 利润锁定价)
    new_stop = max(current_price - distance, min_lock)

    # 只能上移，不能下移
    if new_stop > position.trailing_stop:
        position.trailing_stop = new_stop
        return ("update", new_stop)

    # 是否触发
    if current_price <= position.trailing_stop:
        return ("close", current_price)

    return ("hold", None)
```

### 3.5 实际效果示例

| 场景 | 传统止损 | 智能追踪 | 差异 |
|------|----------|----------|------|
| 开仓后涨到+4%然后回落到+1% | 止损在-3.5% (亏) | 锁定+2%利润平仓 (赚) | **+5.5%** |
| 开仓后涨到+8%然后回落到+5% | 止盈在固定+8%或回撤到+3% | 锁定+7.2%平仓 | **+4.2%** |
| 开仓后直接跌-3.5% | 止损-3.5% | 止损-3.5% | 0% |
| 开仓后涨+12%然后回落到+6% | 可能止盈+8%就走了 | 锁定+10.8%平仓 | **+2.8%** |

---

## 四、Binance Skills Hub 集成

### 4.1 官方 Skill

Binance Skills Hub (`github.com/binance/binance-skills-hub`) 提供官方 AI Agent 交易技能：

```bash
# 安装 (已存在于 /Users/aixx/.agents/skills/binance/)
npx skills add https://github.com/binance/binance-skills-hub
```

### 4.2 可用交易命令

| 操作 | binance-cli 命令 | 用途 |
|------|-----------------|------|
| 市价开仓 | `futures-usds new-order --type MARKET` | 快速进场 |
| 限价开仓 | `futures-usds new-order --type LIMIT` | 精确进场 |
| 追踪止损 | `futures-usds new-order --type TRAILING_STOP_MARKET --callback-rate X` | **智能止盈核心** |
| 止损单 | `futures-usds new-order --type STOP_MARKET` | 硬止损 |
| 止盈单 | `futures-usds new-order --type TAKE_PROFIT_MARKET` | 固定止盈 |
| 平仓 | `futures-usds new-order --reduce-only --type MARKET` | 平仓 |
| 批量下单 | `futures-usds place-multiple-orders` | 同时挂止损+止盈 |
| 查持仓 | `futures-usds position-information-v3` | 实时持仓数据 |
| 查账户 | `futures-usds account-information-v3` | 账户余额 |
| 查K线 | `futures-usds kline-candlestick-data` | 市场数据 |

### 4.3 追踪止损原生命令

Binance 原生 `TRAILING_STOP_MARKET` 与 Agent 智能追踪互补：

```bash
# Binance 原生追踪止损 (服务端执行，不怕断网)
binance-cli futures-usds new-order \
  --symbol NVDAUSDT \
  --side SELL \
  --type TRAILING_STOP_MARKET \
  --callback-rate 2.5 \
  --reduce-only

# callback-rate = 回调百分比
# 例: 价格从最高点回落 2.5% 触发平仓
```

**双层保护策略**:
1. **Layer 1**: Binance 服务端原生追踪止损 (callback-rate 随阶段调整) — 断网也能执行
2. **Layer 2**: Agent 智能追踪 (ATR 动态 + 利润锁定阶梯) — 更精细控制

---

## 五、自进化循环设计

### 5.1 主循环 (每 5 分钟)

```
┌─ Step 1: 环境感知 ─────────────────────────────────────┐
│ 读取 strategy_params.json (自进化参数)                   │
│ BTC 大盘状态: RSI14, MACD, 趋势方向                      │
│ 账户状态: 保证金%, 持仓数, 日盈亏                         │
│ 市场波动率: ATR/历史ATR 比值                              │
│                                                         │
│ 决策门:                                                  │
│ - 保证金 > 60% → 只管持仓，不开新仓                       │
│ - 日亏损 > 5% → 当日停止交易                              │
│ - BTC RSI14 < 30 → 跳过做多 (params中的btc_filter)       │
│ - 当前时段在 avoided_hours → 跳过                        │
└─────────────────────────────────────────────────────────┘
          │
          ▼
┌─ Step 2: Hunter 选币 + 信号捕获 ─────────────────────────┐
│ GET :8080/api/hunter/coins → Top-10 带 tags               │
│                                                          │
│ 过滤 (基于自进化参数):                                     │
│ - 排除 wash_* 标签                                        │
│ - 排除冷却中 (cooldown < 1.0)                             │
│ - 排除 24h > 40%                                          │
│ - 优先 preferred_tags (历史胜率高)                         │
│ - 排除 avoided_tags (历史胜率低)                           │
│ - 对 Top-5 获取详细市场数据                                │
└──────────────────────────────────────────────────────────┘
          │
          ▼
┌─ Step 3: AI 策略决策 (含历史战绩反馈) ────────────────────┐
│ 构建增强策略提示词:                                        │
│                                                          │
│ [基础策略] + [自进化参数] + [近7天战绩] + [信号有效性统计]  │
│                                                          │
│ 示例提示词片段:                                            │
│ "近7天胜率71%，盈利因子2.1。                               │
│  有效信号: oi_price_aligned(82%胜率), lsr_reversal(75%)   │
│  低效信号: lsr_bearish(45%胜率)→ 已降权                    │
│  最佳时段: UTC 6-8, 13-15                                 │
│  当前 BTC RSI14=62, 趋势偏多 → 适合做多                    │
│  ATR比值=1.2, 波动正常 → 标准止盈参数"                     │
│                                                          │
│ 输出: JSON 决策                                            │
│ - 信心度 ≥ params.min_confidence                          │
│ - 共振数 ≥ params.min_resonance                           │
│ - 盈亏比 ≥ 1:3.5                                          │
└──────────────────────────────────────────────────────────┘
          │
          ▼
┌─ Step 4: 持仓管理 (智能追踪止盈) ─────────────────────────┐
│ 对每个持仓:                                               │
│                                                          │
│ 1. 获取 ATR14 (当前 + 7日均值)                             │
│ 2. 计算当前阶段 (宽松/锁定/猎利)                           │
│ 3. 执行 update_trailing_stop()                            │
│    → 需要更新? 修改 Binance 追踪止损 callback-rate         │
│    → 需要平仓? 执行 reduce-only 市价平仓                  │
│                                                          │
│ 同时检查:                                                  │
│ - 硬止损 -3.5% (最终防线)                                  │
│ - 反向信号出现 → 考虑平仓                                  │
│ - 持仓超过 4 小时且无盈利 → 评估是否平仓                   │
└──────────────────────────────────────────────────────────┘
          │
          ▼
┌─ Step 5: 执行交易 ────────────────────────────────────────┐
│ 开仓流程:                                                 │
│ 1. 设杠杆 → binance-cli futures-usds change-initial-...  │
│ 2. 市价开仓 → binance-cli futures-usds new-order MARKET  │
│ 3. 挂硬止损 → STOP_MARKET at -3.5%                       │
│ 4. 挂初始追踪 → TRAILING_STOP_MARKET callback-rate=2.5%  │
│                                                          │
│ 平仓流程:                                                 │
│ 1. binance-cli futures-usds new-order reduce-only MARKET │
│ 2. 记录交易到 trades.jsonl                                 │
│ 3. 推送 Telegram 通知                                     │
└──────────────────────────────────────────────────────────┘
```

### 5.2 日进化循环 (每天 UTC 00:05)

```
┌─ 日报生成 ───────────────────────────────────────────┐
│ 读取 trades.jsonl (最近7天)                           │
│ 计算: 胜率, 盈亏比, 最大回撤, 各信号胜率               │
│ 生成 Telegram 日报                                    │
└──────────────────────────────────────────────────────┘
          │
          ▼
┌─ 参数调优 ───────────────────────────────────────────┐
│ 信号有效性分析:                                        │
│ - 按 hunter_tags 分组 → 淘汰 <50% 胜率的标签           │
│ - 保留 >65% 胜率的标签到 preferred_tags                │
│                                                       │
│ 时段分析:                                              │
│ - 按小时统计 → 调整 preferred_hours / avoid_hours     │
│                                                       │
│ 止盈参数调优:                                          │
│ - 分析 peak_pnl vs exit_pnl → 调整 ATR 倍数           │
│ - 分析被打止损后价格反转率 → 调整止损宽度              │
│                                                       │
│ 杠杆调优:                                              │
│ - 按杠杆分组统计 → 选择最优杠杆                        │
│                                                       │
│ 输出: 更新 strategy_params.json                        │
└──────────────────────────────────────────────────────┘
          │
          ▼
┌─ 策略健康检查 ───────────────────────────────────────┐
│ - 连续亏损 > 3笔? → 暂停 + 告警                      │
│ - 7天胜率 < 50%? → 切换保守模式                      │
│ - 最大回撤 > 15%? → 减半仓位                         │
│ - 盈利因子 > 2.0? → 可适度放大仓位                    │
└──────────────────────────────────────────────────────┘
```

### 5.3 CronCreate 配置

```python
# 主交易循环: 每5分钟
CronCreate:
  cron: "*/5 * * * *"
  prompt: |
    [Hunter Agent v2 主循环]
    1. 读取 ~/.openclaw/workspace/hunter-agent/strategy_params.json
    2. GET http://localhost:8080/api/hunter/coins
    3. 基于自进化参数过滤选币
    4. AI策略决策 (含近7天战绩反馈)
    5. 执行交易 (binance-cli)
    6. 更新持仓追踪止盈
    7. 记录到 trades.jsonl

# 智能追踪止盈: 每1分钟
CronCreate:
  cron: "*/1 * * * *"
  prompt: |
    [智能追踪止盈检查]
    1. binance-cli futures-usds position-information-v3
    2. 对每个持仓:
       - 获取当前ATR14
       - 计算当前阶段 (宽松/锁定/猎利)
       - 更新或触发追踪止盈
    3. 硬止损检查: PnL < -3.5% → 立即平仓

# 日进化: 每天 UTC 00:05
CronCreate:
  cron: "5 0 * * *"
  prompt: |
    [日进化循环]
    1. 读取 trades.jsonl (7天)
    2. 分析信号有效性 → 更新 preferred_tags / avoided_tags
    3. 分析时段表现 → 更新 trading_hours
    4. 分析止盈表现 → 调整 trailing 参数
    5. 更新 strategy_params.json
    6. 生成日报 → Telegram
    7. 健康检查 → 必要时暂停或调整
```

---

## 六、风险控制 (升级版)

| 规则 | 阈值 | 动作 | 自进化联动 |
|------|------|------|-----------|
| 硬止损 | -3.5% PnL | 立即平仓 | 记录→分析反转率 |
| 智能追踪 | 三阶段动态 | ATR自适应平仓 | 记录→优化ATR倍数 |
| 保证金警戒 | > 60% | 禁止新开仓 | — |
| 连败熔断 | 连续3笔亏损 | 暂停2h + 告警 | 触发紧急参数回退 |
| 日亏损 | > 5% | 当日停止 | 记录→分析原因 |
| 最大回撤 | > 15% (7天) | 仓位减半 | 切换保守模式 |
| 策略失效 | 胜率<50%持续3天 | 暂停 + 告警 | 触发全量参数重置 |

---

## 七、实施路线 (更新)

### Phase 1: 基础设施 ✅ 已完成

```
✅ /api/hunter/coins 端点
✅ Binance 速率限制修复
✅ Square Monitor 开关
✅ 策略文档
```

### Phase 2: Agent 核心 (待开始)

```
□ 安装 binance-cli
□ 配置 Binance API keys (先 testnet)
□ 创建 ~/.openclaw/workspace/hunter-agent/ 目录
□ 实现 trades.jsonl 交易日志
□ 实现 strategy_params.json 参数管理
□ 实现主交易循环 (5min)
□ 实现智能追踪止盈 (1min)
□ 测试: testnet 模拟交易
```

### Phase 3: 自进化引擎

```
□ 实现每日参数调优器
□ 实现信号有效性分析
□ 实现时段分析
□ 实现策略健康检查
□ Telegram 日报
□ 7天 testnet 验证
```

### Phase 4: 实盘

```
□ 切换 Mainnet (小仓位 equity×0.2)
□ 观察 7 天
□ 对比自进化前后表现
□ 逐步放大仓位
```

---

## 八、预期表现

| 指标 | v1 (静态策略) | v2 (自进化) |
|------|--------------|-------------|
| 胜率 | 65-70% | 70-75% (信号筛选优化) |
| 盈亏比 | 1:3.5 | 1:4+ (智能追踪止盈) |
| 最大回撤 | 15% | <10% (动态风控) |
| 策略适应 | 无 | 7天滚动优化 |
| 止盈效率 | 固定/粗略 | ATR自适应三阶段 |
| 被打止损率 | 30% | <15% (波动率感知) |
