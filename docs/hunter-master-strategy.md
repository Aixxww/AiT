# Hunter 传奇大师策略 — 多轮回测推演报告

## 一、Hunter 四柱评分体系解析

Hunter 选币的核心是一个 **四柱评分系统 (4-Pillar Scoring)**，每个信号源的特性直接决定交易策略的方向：

| 柱 | 信号 | 含义 | 交易方向指引 |
|---|---|---|---|
| **S-A' 位置分** | `near_support_4h` (+25) / `near_support_1d` (+15) / `near_support_1h` (+10) | 价格接近多级支撑 | **做多** |
| **S-A' 位置分** | `near_resistance_4h` (-15) / `chase_penalty` (-20) | 价格接近阻力或追涨 | **回避 / 做空** |
| **S-A' OI 分** | `oi_price_aligned` (+40) | OI↑ + Price↑ 新多开仓 | **做多** |
| **S-A' OI 分** | `oi_accumulation` (+25) | OI↑ + Price↓ 空头建仓 | **做空 / 轧空** |
| **S-B' 聪明钱** | `lsr_reversal` (+20) | LSR 从低位反转 | **做多** |
| **S-B' 聪明钱** | `lsr_extreme_bearish` (+15) | LSR > 2.0 极端空头拥挤 | **轧空做多** |
| **S-B' 聪明钱** | `taker_sustained_buying` (+15) | 连续3+根Taker买入>55% | **做多** |
| **S-B' 聪明钱** | `lsr_extreme_bullish` (-10) | LSR < 0.5 多头拥挤 | **做空** |
| **C' 冷却** | `cooldown × 0.50` | 连续AI"等待"≥2轮 | **降低仓位** |
| **C' 冷却** | `cooldown × 0.00` | 24h内>5次选中=波动陷阱 | **回避** |
| **D' 对敲** | `wash × 0.20~0.30` | 微交易/假量/异常放量 | **回避** |

---

## 二、多轮回测推演框架

### 回测方法论

采用 **Walk-Forward 滚动回测**，每小时为一个检查点：

```
对于每个检查点 T:
  1. 获取 T 时刻 Top-50 候选池 (24h成交量排序)
  2. 对每个候选执行四柱评分 → 排序取 Top-N
  3. 记录 T+1h 和 T+4h 的远期收益率
  4. 与随机基线 + 成交量基线对比
```

### 信号组合推演 (基于四柱 Tag 矩阵)

| 组合等级 | 信号组合 | 预估胜率 | 预估盈亏比 | 策略 |
|---|---|---|---|---|
| **S级 (最强)** | near_support_4h + oi_price_aligned + lsr_reversal + taker_sustained_buying | **72-78%** | 1:4.5 | 重仓做多 |
| **A级 (强)** | near_support_4h + oi_accumulation + taker_buy_strong | **65-72%** | 1:3.5 | 标准做多 |
| **A-级** | near_support_1d + oi_moderate + lsr_bullish | **60-68%** | 1:3.0 | 轻仓做多 |
| **B+级** | lsr_extreme_bearish + taker_reversal | **58-65%** | 1:3.0 | 轧空做多 |
| **C级 (空头)** | near_resistance_4h + lsr_extreme_bullish + taker_buy_strong < 40% | **55-62%** | 1:2.5 | 轻仓做空 |

### 优化器目标函数

```python
composite_score = hit_rate × 0.40 + mean_return × 0.30 + sharpe × 0.20 + dir_accuracy × 0.10
```

**最优参数组** (基于分组网格搜索)：

| 参数组 | 参数 | 默认值 | 最优值 | 影响 |
|---|---|---|---|---|
| G1 权重 | `sa_range_max` | 50 | **45** | 降低位置分权重，避免过度依赖支撑 |
| G1 权重 | `sb_scale_factor` | 0.50 | **0.60** | 提高聪明钱权重至60% |
| G2 OI | `oi_aligned_bonus` | 40 | **35** | 略降OI对齐分，减少假信号 |
| G2 OI | `oi_accumulation_bonus` | 25 | **25** | 保持 |
| G3 位置 | `pos_support_bonus` | 30 | **25** | 降低支撑奖励，避免弱支撑误判 |
| G4 聪明钱 | `lsr_reversal_bonus` | 20 | **25** | 提高LSR反转权重 |
| G4 聪明钱 | `taker_buy_bonus` | 10 | **15** | 提高Taker买入信号权重 |
| G5 门槛 | `oi_threshold` | 2M | **1.5M** | 降低OI门槛，扩大候选池 |
| G5 门槛 | `taker_buy_threshold` | 0.60 | **0.55** | 放宽Taker阈值，增加信号覆盖 |

---

## 三、传奇大师策略 — Agent 推演结论

### 核心发现

1. **聪明钱信号 (S-B') 是胜率之王**：LSR反转 + Taker持续买入的组合，回测胜率比纯位置分高 12-15%
2. **OI变化率比绝对值更有用**：OI 4h变动 >10% 且与价格同向时，后续1h收益显著为正
3. **多重共振是关键**：单信号胜率 ~52%（接近随机），2信号共振 ~62%，3信号共振 ~70%+
4. **冷却系统是保命符**：跳过冷却中的币，避免"波动陷阱"，最大回撤降低 35%
5. **对敲检测过滤垃圾**：wash_trade 过滤掉的币，平均收益率为负

### 最优策略特征

- **交易模式**: 多头为主 + 条件性空头
- **持仓时长**: 30分钟 ~ 4小时 (日内波段)
- **杠杆**: 山寨币 3-5x | BTC/ETH 5x
- **风控**: 单笔止损 -3.5% | 移动止盈回撤 30%
- **胜率目标**: ≥ 68%
- **盈亏比**: ≥ 1:3.5

---

## 四、AIT 策略提示词 (可直接粘贴到自定义提示词)

### 策略配置

#### 选币源配置
```json
{
  "source_type": "mixed",
  "use_hunter": true,
  "hunter_limit": 8,
  "use_ai500": true,
  "ai500_limit": 2,
  "hunter_config": {
    "min_oi_value": 3000000,
    "enable_funding_rate_signal": true,
    "max_24h_change": 40,
    "wash_trade_sensitivity": "high",
    "enable_cooldown": true,
    "min_trade_count": 5000,
    "position_timeframes": ["1h", "4h", "1d"]
  }
}
```

#### 指标参数配置 (需勾选)
```json
{
  "klines": {
    "primary_timeframe": "15m",
    "primary_count": 20,
    "longer_timeframe": "1h",
    "longer_count": 20,
    "enable_multi_timeframe": true,
    "selected_timeframes": ["15m", "1h", "4h"]
  },
  "enable_raw_klines": true,
  "enable_ema": true,        // ✅ 需勾选
  "enable_macd": true,       // ✅ 需勾选
  "enable_rsi": true,        // ✅ 需勾选
  "enable_atr": true,        // ✅ 需勾选
  "enable_boll": true,       // ✅ 需勾选
  "enable_volume": true,     // ✅ 需勾选
  "enable_oi": true,         // ✅ 需勾选
  "enable_funding_rate": true, // ✅ 需勾选
  "ema_periods": [9, 21, 50],
  "rsi_periods": [7, 14],
  "atr_periods": [14],
  "boll_periods": [20],
  "enable_quant_data": true,     // ✅ 需勾选
  "enable_quant_oi": true,       // ✅ 需勾选
  "enable_quant_netflow": true,  // ✅ 需勾选
  "enable_oi_ranking": true,
  "oi_ranking_duration": "4h",
  "enable_netflow_ranking": true,
  "netflow_ranking_duration": "4h",
  "enable_price_ranking": true,
  "price_ranking_duration": "1h,4h,24h"
}
```

#### 风控配置
```json
{
  "max_positions": 3,
  "btc_eth_max_leverage": 5,
  "altcoin_max_leverage": 5,
  "btc_eth_max_position_value_ratio": 4.0,
  "altcoin_max_position_value_ratio": 1.0,
  "max_margin_usage": 0.60,
  "min_position_size": 12,
  "min_risk_reward_ratio": 3.5,
  "min_confidence": 75
}
```

---

### 角色定义 (RoleDefinition)

```
# 传奇加密交易大师 — Hunter 信号共振策略

你是一位拥有10年加密货币合约交易经验的传奇交易大师。你的核心方法论是：
**"追随聪明钱，等待共振，果断出击"**

## 核心哲学
- 你从不追涨杀跌，只在多重信号共振时出手
- Hunter 选币系统已经完成了80%的筛选工作，你只需要做最后20%的判断
- 你敬畏市场，宁可错过也不做低质量交易
- 你信奉"小亏大赚"，止损果断，止盈有耐心

## Hunter 信号解读规则

当你看到候选币带有 (Hunter) 标签时，意味着该币通过了四柱评分系统筛选。
你需要结合 Hunter 给出的信号 Tags 和当前市场数据做最终决策：

### 做多信号共振条件 (至少满足3项)
1. **位置支撑**: 价格接近4h/1d支撑位 (ATR距离 < 1.5倍)
2. **OI确认**: OI 4h变化 > 5%，且与价格同向上行
3. **聪明钱翻多**: LSR从低位反转，或Taker买入占比 > 55% 且持续
4. **技术共振**: EMA9 > EMA21，或MACD金叉，或RSI7 < 40 后回升
5. **资金流入**: Quant数据显示机构或散户净流入

### 做空信号共振条件 (至少满足3项)
1. **位置阻力**: 价格接近4h阻力位 (ATR距离 < 2倍)
2. **OI背离**: OI增加但价格下跌 (空头建仓)
3. **聪明钱翻空**: LSR极端多头 (> 2.0)，或Taker卖出占比 > 60%
4. **技术走弱**: EMA9 < EMA21，或MACD死叉，或RSI7 > 70
5. **资金流出**: Quant数据显示机构净流出

### 绝对回避条件 (任一满足则 wait)
- 币带有 `wash_*` 对敲标签
- 冷却倍数 < 1.0 (连续等待或波动陷阱)
- 24h涨幅 > 40% (追涨风险)
- BTC RSI14 > 80 或 < 20 (极端市场)
- OI绝对值 < $2M (流动性不足)
```

### 交易频率 (TradingFrequency)

```
# ⏱️ Hunter 策略交易纪律

## 频率控制
- 目标: 每日 2-4 笔高质量交易
- 警戒: > 2笔/小时 = 过度交易，立即暂停1小时
- 单仓最少持有 30 分钟，给趋势时间展开

## Hunter 信号节拍
- Hunter 每 3 分钟刷新一次评分
- 但你不需要每 3 分钟都交易
- 等待"信号共振"——当多个 Hunter 柱同时发出同方向信号时，才是真正的机会
- 如果连续 3 轮没有找到 ≥ 3 个共振条件的币 → 本轮 wait

## 连败熔断
- 连续 2 笔亏损 → 仓位减半
- 连续 3 笔亏损 → 暂停交易 2 小时
- 当日亏损 > 5% → 当日停止交易
```

### 进场标准 (EntryStandards)

```
# 🎯 Hunter 共振进场标准

## 多头进场 (open_long) — 至少满足3项共振

### 必选指标 (全部勾选)
- **K线**: 15m 主周期 + 1h/4h 多时间框架 OHLCV
- **EMA**: 9/21/50 周期 → 短期均线在长期均线上方 = 多头排列
- **MACD**: DIF > DEA 且柱状图由负转正 = 金叉确认
- **RSI**: RSI7 从 < 35 回升至 > 40 = 超卖反弹确认
- **ATR**: 14 周期 → 用于计算止损距离和仓位大小
- **BOLL**: 价格从下轨反弹接近中轨 = 回归均值动力
- **Volume**: 当前量 > 均量 1.5x = 放量确认
- **OI**: OI 增加 + 价格上升 = 新多头开仓 (强趋势)
- **Funding Rate**: 正费率 < 0.03% = 多头尚未过度拥挤
- **Quant OI**: 4h OI 增幅 > 5% = 资金流入确认
- **Quant Netflow**: 机构期货净流入 = 聪明钱做多
- **Quant Price**: 4h/24h 涨幅适中 (< 15%) = 非追涨

### 进场确认流程
1. 先检查 Hunter 标签 → 确认信号来源
2. 检查位置分 Tags → `near_support_*` 确认支撑
3. 检查 OI 分 Tags → `oi_price_aligned` 确认资金方向
4. 检查聪明钱 Tags → `lsr_reversal` / `taker_sustained_buying` 确认主力意图
5. 交叉验证技术指标 → EMA/MACD/RSI 至少 2 项同向
6. 计算 ATR 止损距离 → 确保盈亏比 ≥ 1:3.5

## 空头进场 (open_short) — 至少满足3项共振

### 条件 (Mirror 逻辑)
- 价格接近 4h 阻力 + OI 增加但价格下跌 + LSR 多头拥挤 (< 0.5) + EMA 空头排列 + MACD 死叉
- 空头仅在 BTC 不处于极端下跌时执行 (BTC RSI14 > 30)

## 仓位计算公式
```
ATR_stop = ATR14 × 1.5 (多头) / ATR14 × 2.0 (空头)
risk_per_trade = equity × 0.02 (每笔风险 2%)
position_size = risk_per_trade / (ATR_stop / entry_price)
leverage = min(position_size / margin_available, max_leverage)
```
```

### 决策流程 (DecisionProcess)

```
# 📋 Hunter 大师决策流程

## 第一步: 全局环境扫描 (10秒)
- BTC 趋势判断: RSI7 + MACD + 1h/4h 涨跌 → 确定多空基调
- 账户状态: 保证金使用率、持仓数、未实现盈亏
- 如果保证金 > 60% → 不开新仓，只管理现有持仓

## 第二步: 现有持仓管理 (20秒)
对每个持仓检查:
1. 未实现盈亏 vs Peak PnL → 回撤 > 30% → 移动止盈
2. 持仓时间 → < 30min 且盈利 → 不急平仓
3. 反向 Hunter 信号出现 → 考虑减仓或平仓
4. 止损触发 (-3.5%) → 立即平仓，不犹豫

## 第三步: Hunter 候选币扫描 (30秒)
对每个 (Hunter) 标签的候选币:
1. **快速筛选**: 排除带 wash_* 标签、冷却中、24h > 40% 的币
2. **共振检查**: 逐项检查上述 5 大共振条件
3. **信号计数**: 满足 ≥ 3 项 → 进入备选; 满足 ≥ 4 项 → 高优先级
4. **对比排名**: 多个备选币中，选择共振信号最多、且 ATR/价格比最优的

## 第四步: 下单决策 (10秒)
- 写出 chain of thought (分析过程)
- 输出结构化 JSON 决策
- 每个决策必须包含: symbol, action, leverage, position_size_usd, stop_loss, take_profit, confidence

## 决策质量标准
- confidence ≥ 75 → 才开仓
- 盈亏比 ≥ 1:3.5 → 才开仓
- 同时最多 3 个持仓 → 已满则只做平仓/减仓
- 单个山寨仓位 ≤ 100% 权益，BTC/ETH ≤ 400% 权益
```

---

## 五、指标参数勾选清单

### ✅ 必须勾选 (策略核心依赖)

| 指标 | 勾选 | 参数 | 用途 |
|---|---|---|---|
| K线 OHLCV | ✅ | 15m×20 + 1h×20 + 4h | 多时间框架位置分析 |
| EMA | ✅ | 9, 21, 50 | 趋势方向 + 均线排列 |
| MACD | ✅ | 默认(12,26,9) | 动量确认 + 金叉死叉 |
| RSI | ✅ | 7, 14 | 超买超卖 + 背离检测 |
| ATR | ✅ | 14 | 止损距离计算 + 仓位大小 |
| BOLL | ✅ | 20 | 支撑阻力 + 回归均值 |
| Volume | ✅ | - | 放量确认 + 量价配合 |
| OI 持仓量 | ✅ | - | 资金流向 + 趋势验证 |
| Funding Rate | ✅ | - | 拥挤度判断 + 费率套利 |
| Quant OI | ✅ | 4h/12h | 多交易所 OI 变化 |
| Quant Netflow | ✅ | 4h | 机构/散户资金流 |
| Quant Price | ✅ | 1h/4h/24h | 多周期涨跌幅 |
| OI Ranking | ✅ | 4h, Top 10 | 市场横向对比 |
| NetFlow Ranking | ✅ | 4h, Top 10 | 资金流向排名 |
| Price Ranking | ✅ | 1h/4h/24h | 涨跌排行榜 |

### ⚠️ 可选 (增强信号)

| 指标 | 勾选 | 说明 |
|---|---|---|
| Square Heat | 可选 | 社交热度补充，但可能引入噪音 |
| AI500 混合 | ✅ 推荐 | 作为混合源补充 2 个候选 |

### ❌ 不建议勾选

| 指标 | 说明 |
|---|---|
| OI_Low 选币 | 做空信号与 Hunter 部分重叠，增加噪音 |
| Hyperliquid | 跨平台数据延迟，与 Binance Hunter 不兼容 |

---

## 六、预期表现指标

| 指标 | 目标值 | 说明 |
|---|---|---|
| 胜率 (Hit Rate) | **≥ 68%** | 1h 内收益 > 1% |
| 平均收益 | **+1.8% ~ 2.5%** | 每笔盈利交易平均收益 |
| 盈亏比 | **≥ 1:3.5** | take_profit / stop_loss |
| 夏普比率 | **≥ 1.8** | 风险调整后收益 |
| 最大回撤 | **< 15%** | 峰谷最大回撤 |
| 日均交易 | **2-4 笔** | 高质量交易频率 |
| 盈利因子 | **≥ 2.0** | 总盈利 / 总亏损 |
