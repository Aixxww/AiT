# 实盘亏损交易决策根因复盘

> 生成时间：2026-06-06 10:01 CST
> 数据来源：`data/data.db`、`data/ait_2026-06-05.log`
> 覆盖范围：`trader_positions` 全部已平仓仓位、`trader_orders`、`trader_fills`、`decision_records`

## 1. 总览

数据库内共有 `104` 个已平仓仓位，其中 `39` 个净亏损仓位。总净盈亏仍为 `+4.551419 USDT`，但亏损端合计 `-29.611325 USDT`，盈利端合计 `+34.162744 USDT`。

按交易员统计：

| 交易员 | 亏损仓位 | 毛亏损 | 手续费 | 净亏损 | 可回溯性 |
|---|---:|---:|---:|---:|---|
| VVV / hunter v7 | 11 | -16.291050 | 0.976052 | -17.267102 | 决策链完整 |
| CCC | 8 | -6.040444 | 0.197076 | -6.237520 | 多为历史同步记录 |
| old deepseek | 5 | -3.228504 | 0.105144 | -3.333648 | 可部分回溯 |
| old mimo | 6 | -1.063640 | 0.150592 | -1.214232 | 可部分回溯 |
| old e0b78 | 6 | -1.136770 | 0.076485 | -1.213255 | 可部分回溯 |
| DD | 3 | -0.293400 | 0.052168 | -0.345568 | 可部分回溯 |

最大亏损集中在 VVV 的猎手 v7 实盘，且日志和 AI 推理完整，因此主要根因应以 VVV 为准。

## 2. VVV 亏损单逐笔归因

| 仓位 | 周期 | 开仓时间 CST | 持仓 | 净亏损 | 直接触发 | 根因 |
|---|---:|---|---:|---:|---|---|
| GUAUSDT LONG | #86 | 06-06 07:59:10 | 2.1m | -6.0315 | 止损成交 | 追高短挤压，5m RSI7 83.7、1h 已涨 10.79%，仍按 A 信号开多 |
| LABUSDT LONG | #91 | 06-06 09:13:45 | 3.8m | -3.1253 | 止损成交 | 选中非首位候选，低位反弹确认不足，止损过近，开仓后快速跌破 |
| LABUSDT LONG | #81 | 06-06 06:43:46 | 30.0m | -2.7005 | 止损/结构失效 | 把“在 entry_zone 内”当作 reclaim，实际仍低于 15m EMA20 |
| INJUSDT LONG | #30 | 06-05 17:59:50 | 8.1m | -1.0835 | 止损成交 | panic reversal 中用很近结构止损制造 RR，波动容忍不足 |
| SEIUSDT LONG | #76 | 06-06 05:29:02 | 36.0m | -0.8966 | 止损/回落 | C 信号用过紧止损，虽然确认项满足，但价格处在 panic_dump 反弹初段 |
| FILUSDT LONG | #49 | 06-05 22:43:57 | 11.0m | -0.8625 | 止损成交 | short_squeeze_long 的 timing_score 只有 30，被 A 置信和高 RR 掩盖 |
| WLFIUSDT LONG | #59 | 06-06 01:14:13 | 2.6m | -0.6675 | 止损成交 | C 信号，原始 invalidation RR 仅 0.61，改用极近止损后开仓 |
| ASTERUSDT SHORT | #60 | 06-06 01:29:09 | 33.0m | -0.6459 | 止损成交 | C 信号 RR 仅约 1.71，低于 C 级应有门槛，仍开空 |
| ROBOUSDT SHORT | #38 | 06-05 19:59:15 | 86.6m | -0.6200 | 反弹打止损 | C 信号，taker_buy=0.449 只是压线，OI 先前仍 buildup，确认质量偏弱 |
| ICPUSDT LONG | #63 | 06-06 02:14:08 | 1.7m | -0.4767 | 止损成交 | 开仓后 2 分钟内打止损，止损小于正常 15m 波动缓冲 |
| NEARUSDT LONG | #5/#6 | 06-05 08:56:22 | 14.7m | -0.1571 | AI 主动平仓 | #5 把 entry_zone 当 reclaim；#6 又用 14 分钟走势快速否定前一轮开仓 |

## 3. 共同根因

### 3.1 把 `entry_zone` 当作确认，而不是交易区

多次推理里出现同一个模式：价格在 `entry_zone` 内，即判定 `15m_reclaim_vwap_or_entry_zone` 成立。但 `entry_zone` 只表示可观察区，不等同于 reclaim 已完成。

典型案例：

- NEAR #5：AI 先指出价格低于 15m EMA20，随后又因“在 entry_zone 内”改判确认满足。
- LAB #81：AI 明确记录价格仍低于 15m EMA20，却因为在 entry_zone 内判定 reclaim 成立。

根因：提示词里没有把 `wait_reclaim` 的“收盘确认”定义成硬条件，LLM 可以用 entry_zone 替代确认。

### 3.2 为了满足 RR，临时改用过近止损

很多候选如果使用 v7 invalidation，RR 不达标；AI 改用 5m/15m 近端低点甚至 entry_zone 边缘做 SL，从而让 RR 看起来很好，但实际止损距离小于正常波动，导致几分钟内被打。

典型案例：

- WLFI：用 invalidation 计算 RR 只有 `0.61`，AI 自己承认不达标；类似逻辑在其他周期仍被绕开。
- ICP：1.7 分钟打止损。
- GUA：2.1 分钟打止损，亏损成为全库最大单笔。
- LAB #91：3.8 分钟打止损。

根因：执行层只校验 RR 数字，没有校验“止损是否足够合理”。提示词也没有强制 `SL distance >= ATR/波动下限`。

### 3.3 忽略 `timing_score`，过度相信 `confidence` 和 `ai_priority`

亏损单里多次出现 `timing_score` 很低仍开仓：

- FIL：`confidence=A`，但 `timing_score=30`。
- GUA：`confidence=A`，但 `timing_score=30`，同时 5m RSI7=83.7、1h 已涨 10.79%。
- LAB：`timing_score=40`，仍按 panic reversal 开多。

根因：LLM 把 `ai_priority` 当成开仓优先级，而没有把 `timing_score` 当作追价/等待过滤器。v7 的筛选只是“候选发现”，不是“立即开仓许可”。

### 3.4 对 C 级信号过宽

ROBO、SEI、WLFI、ASTER 都是 C 信号或 C 级逻辑，仍多次开仓。尤其 funding reversal 里，AI 经常用 LSR 拥挤替代 funding 方向，或接受 4/5 且边界很薄的确认。

典型案例：

- ROBO：`taker_buy_15m=0.449`，只是刚好低于 0.45；OI 条件早期并不强。
- ASTER：AI 计算 RR 约 1.71，并承认 C 信号不理想，但仍选择交易。
- SEI：C 信号在 panic_dump 中开多，随后 36 分钟止损。

根因：C 信号缺少更严格的硬门槛，例如 `ai_priority >= 65`、`timing_score >= 70`、`RR >= 2.0`、`5/5 confirmations`。

### 3.5 冷静期只按同币同向，不按同形态连续亏损

系统和提示词已经有“同币亏损冷静期”的意识，但实盘亏损主要来自同一类形态连续交易：panic_dump / panic_reversal / funding_reversal 反复开仓。

例如 WLFI、ASTER、ICP、FIL、SEI、LAB 都是 panic_dump 环境里的反转/拥挤反转。单币不同，但形态风险相同。

根因：冷静期粒度太细。需要按 `setup_type + market_regime + direction` 增加组合冷静期，而不只是 symbol。

### 3.6 盈利后放大交易冲动

多轮 AI 推理把“近期盈利”“账户 PnL 很好”当成可交易背景。例如 #30 记录 “Recent trades are profitable. Good momentum.” #86 记录 “PnL +300.61%. Good to trade.” 这些不是市场信号，却会降低等待门槛。

根因：提示词没有明确禁止用账户盈利状态提高开仓倾向。账户状态只能用于仓位大小和风险收缩，不能作为信号增强。

### 3.7 手续费和小波段收益吞噬

部分仓位毛盈亏接近 0，但手续费后转亏，例如 XLM、BSB、EDEN 早期记录。高频小波段如果 TP1 太近，实际胜率需要非常高，否则手续费拖累明显。

根因：RR 计算没有把双边手续费、滑点和最小有效收益纳入硬过滤。

## 4. 不是主要根因但需要注意的问题

1. `trader_positions.leverage` 多数显示 1，但 AI 决策和订单执行日志里有 5x/10x。仓位表的 leverage 字段可能不是可靠的风险分析字段，实际风险应按名义仓位和权益计算。
2. GUA 持仓期间出现 Binance K 线全零并回退 CoinAnk 的日志。这不是开仓根因，但会影响持仓判断质量。
3. 早期 CCC/DD/old trader 的亏损很多是同步仓位，部分缺少完整策略上下文，不应把这些直接归咎于猎手 v7。

## 5. 根治建议

### 提示词硬规则

1. `entry_mode=wait_reclaim`：必须看到 5m 或 15m 收盘重新站上 VWAP/EMA20/entry_zone 上沿之一；只是在 entry_zone 内不能算 reclaim。
2. `timing_score < 60`：禁止开仓；`60-70` 只能轻仓且必须 A/B 信号，C 信号直接 wait。
3. `confidence=C`：必须满足 `ai_priority >= 65`、`timing_score >= 70`、`RR >= 2.0`、`5/5 confirmations`，否则 wait。
4. squeeze long 若 `5m RSI7 > 80` 或 `1h change > 8%`，只能等回踩确认，禁止追入。
5. 账户近期盈利不能作为开仓理由；连续盈利后反而应降低仓位，防止过度交易。
6. 同一 `market_regime + setup_type + direction` 出现亏损后，至少等待 2 个周期，除非 ai_priority 提升 8 分以上且 timing_score >= 75。

### 执行层硬规则

1. 增加止损合理性校验：`abs(entry - stop_loss)` 不得小于 `max(0.5 * ATR15m, 0.25 * ATR1h, 2 * spread_or_tick_buffer)`。
2. 增加单笔权益风险上限：止损触发的预计亏损不得超过账户权益的 3%-5%。
3. RR 计算加入双边手续费和滑点，TP1 到 entry 的距离必须覆盖至少 3 倍交易成本。
4. 如果 v7 status 是 `wait_confirm`，LLM 不得自行升级为开仓。
5. 对 `timing_score`、`status`、`required_confirmations` 做执行层二次校验，不能只依赖 LLM 自律。

## 6. 最终判断

当前亏损不是单纯“方向判断错”，而是“开仓条件过宽 + 止损设计过紧 + 追反转过早”的组合问题。

猎手 v7 能筛到有波动的标的，但 LLM 在二次确认时经常把候选信号当成可开仓信号，尤其在 panic_dump / short_squeeze / funding_reversal 场景下，为了通过 RR 校验主动选择很近的结构止损。这样一旦反弹或挤压没有立即延续，就会在 1-5 分钟内被止损，形成大额亏损。

优先修复顺序：

1. 先在执行层增加 `timing_score`、`status`、`SL distance`、`single-trade equity risk` 硬校验。
2. 再更新猎手 v7 提示词，禁止用 entry_zone 替代 reclaim，禁止 C 信号低门槛开仓。
3. 最后再评估是否调整 v7 筛选阈值；现在主要问题不在候选筛选，而在候选到开仓的确认层。
