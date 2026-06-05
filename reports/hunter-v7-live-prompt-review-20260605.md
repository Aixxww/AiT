# 猎手 v7 近轮实盘复盘与提示词优化报告

> 生成时间：2026-06-05
> 复盘对象：交易员 `VVV`，策略 `猎手v7`
> 数据来源：`data/data.db`、`data/ait_2026-06-05.log`、`.logs/backend.log`
> 覆盖周期：实盘周期 #4 - #15，重点分析 #5 - #10

## 1. 核心结论

猎手 v7 当前的主要问题已经不是“筛不出币”，而是三类边界需要明确分层处理：

1. 数据源层：v7 候选筛选链路已经恢复，#11 - #15 均能从实时快照筛出候选币，并送入 LLM 决策。
2. 执行层：#8 的 `GUAUSDT` 失败来自币安单币种杠杆上限，#10 的失败来自首轮 DataCollector 快照等待不足；这两项不属于提示词错误。
3. 提示词层：#5 开多 `NEARUSDT` 的方向并非明显错误，但 #6 过早平仓、#7 低盈亏比重复开多，说明提示词需要更强的“持仓状态机、再入场冷静期、RR 硬过滤、entry_mode 确认”规则。

压缩效果已经明显：#5/#6 旧输入平均约 `100,140 tokens`，#7 以后可用周期平均约 `14,986 tokens`，降幅约 `85.0%`。后续优化应保持精简，不建议重新引入大段 K 线原文。

## 2. 当前策略配置快照

| 项目 | 当前值 |
|---|---:|
| 数据源 | `hunter_v7` |
| 候选数量 | `hunter_limit=10` |
| v7 最大输出 | `v7_max_output=30` |
| AI 优先级下限 | `v7_min_ai_priority=50` |
| 激进模式 | `v7_aggressive=true` |
| Token 压缩 | `prompt_compact_mode=hunter_v7_only`，后端兼容为当前数据源压缩 |
| 轮询周期 | `15 min` |
| 最大持仓数 | `1` |
| 风控 RR 下限 | `1.5` |
| 最小信心 | `75` |
| BTC/ETH 杠杆上限 | `20x` |
| 山寨杠杆上限 | `20x` |

当前 `custom_prompt` 已包含 v7 主信号、RR 下限、禁止追价、持仓状态机等规则；`prompt_sections` 也已开始拆分为角色定义、交易频率、开仓标准、决策流程和附加提示。下一步不需要再写长提示词，而应继续强化冲突消解和执行条件。

## 3. 近轮实盘时间线

| 周期 | 时间(CST) | 候选/决策 | 结果 | Tokens | 复盘结论 |
|---:|---|---|---|---:|---|
| #4 | 08:44:22 | 无候选 | `No candidate coins available` | 0 | 当时 v7 筛选/保存配置未稳定，后续已恢复候选输出 |
| #5 | 08:56:22 | `NEARUSDT open_long` | 开多成功 | 106,086 | 恐慌反弹多头信号，方向不算明显错误，但 prompt 过大 |
| #6 | 09:11:05 | `NEARUSDT close_long` | 平多成功 | 94,193 | 15 分钟后关闭，实际成交均价 2.235，低于入场 2.242 |
| #7 | 09:23:45 | `NEARUSDT open_long` | 风控拒绝，RR=0.65 | 13,882 | LLM 再次想做同标的，但执行层正确拦截 |
| #8 | 10:52:03 | `GUAUSDT open_long` | 杠杆设置失败 | 16,974 | 交易所单币杠杆限制未被提前夹紧，属于执行层问题 |
| #9 | 11:06:10 | 多标的 `wait` | 成功等待 | 16,276 | 压缩后决策更克制 |
| #10 | 12:16:00 | 快照未准备 | 构建上下文失败 | 0 | 首轮 Binance REST 快照超过原 30s 等待 |
| #11 | 12:48:57 | 10 个候选全 `wait` | 成功 | 16,396 | 数据链路恢复，开始正常等待 |
| #12 | 13:03:15 | `NEAR/PUMP/SUI/ZEC wait` | 成功 | 11,084 | NEAR 仍入池，但未再追多 |
| #13 | 13:18:07 | 8 个候选全 `wait` | 成功 | 12,944 | 无强确认，等待合理 |
| #14 | 13:33:54 | 9 个候选全 `wait` | 成功 | 16,331 | 弱信号环境下未交易 |
| #15 | 13:48:32 | 9 个候选全 `wait` | 成功 | 15,999 | v7 筛选 + 压缩 + LLM 决策链路跑通 |

## 4. NEARUSDT 交易复盘

实盘成交记录：

| 动作 | 时间(CST) | 数量 | 成交价 | 费用 | 结果 |
|---|---|---:|---:|---:|---|
| 开多 | 08:56:22 | 17 | 2.242 | 0.019057 | 建仓成功 |
| 平多 | 09:11:05 | 17 | 2.235 | 0.0190 合计 | 实现亏损约 -0.119 USDT，含费约 -0.157 USDT |

观察：

- #5 的 v7 结构是 `panic_reversal_long`，属于下跌趋势中的恐慌反弹，不应按趋势追多处理。
- #6 的平仓发生在 15 分钟后，当前输入中 Binance NEAR 多周期 K 线曾出现全零成交量并回退到 CoinAnk，说明当轮数据质量和状态切换都偏敏感。
- #7 再次对 `NEARUSDT` 开多被 RR 风控拒绝，证明执行层的 RR 下限有效，但 prompt 仍需要明确“刚亏损平仓后的同向再入场冷静期”。
- 订单同步把一个 17 张平仓拆成多笔撮合成交是正常现象；数据库订单表记录为 3+3+3+3+5，多笔 trade 不是重复下单。

结论：#5 的多头方向不应简单判为错误；更大的问题是 #6 没有足够明确地用“setup 是否失效”来管理持仓，而是让 LLM 在下一轮重新评价并快速否定前一轮开仓。

## 5. 失败归因

### #4 无候选

当时 v7 配置保存和筛选链路仍在修复中。现在数据库显示 `v7_min_ai_priority=50`、`v7_aggressive=true` 已保存成功，#11 - #15 均能输出候选币，说明这个问题已不再复现。

### #8 杠杆失败

错误：

```text
GUAUSDT open_long failed: failed to set leverage: <APIError> code=-4028, msg=Leverage 20 is not valid
```

归因：币安部分山寨合约最大杠杆低于策略给出的 `20x`。当前代码已增加交易所最大杠杆查询和夹紧：

- `trader/binance/futures_positions.go`：查询 leverage bracket，并在 `SetLeverage` 前夹紧。
- `trader/auto_trader_orders.go`：下单前解析有效杠杆，超过交易所上限时自动降到上限。

提示词层仍应建议“山寨币默认 5x - 10x，除非系统明确提供更高可用上限”，避免 LLM 频繁输出无效高杠杆。

### #10 快照未准备

错误：

```text
Failed to build trading context: failed to get candidate coins: snapshot not ready after waiting for initial DataCollector fetch
```

归因：服务重启后，交易周期立即触发；DataCollector 首轮 Binance REST 快照耗时约 1m30s，原等待 30s 不足。当前 `snapshotReadyTimeout` 已提高到 `3 * time.Minute`。重启后的日志显示 v7 能等待快照刷新并完成筛选。

这不是提示词问题；提示词无法修复数据快照未就绪。

## 6. 提示词表现评估

做得好的部分：

- 能识别 v7 的结构化信号，并按候选币输出决策。
- 压缩后 token 成本显著下降，且 #9、#11 - #15 没有出现无意义交易冲动。
- 执行层 RR 风控能拦截 #7 这类低盈亏比开仓。

需要优化的部分：

- 持仓管理规则仍需更硬：已有仓位时，优先判断“继续验证、失效、止盈、时间止损”，不要每轮从零重新评估。
- 反转类 LONG 需要更强调“不追价”和“只用近端结构止损计算 RR”；不能机械用 v7 的远端 invalidation，也不能把 panic reversal 当趋势多。
- 刚亏损平仓后的同币同向再入场需要冷静期，除非 v7 优先级显著提升并出现新的确认结构。
- `ai_priority >= 50` 是入池阈值，不应等同开仓阈值。开仓建议提高到 `>=55`，若逆大级别 regime 或属于 `wait_reclaim`，建议 `>=60`。
- 对 `wait_confirm`、`wait_reclaim`、`wait_price_reversal` 的解释要更短更硬：未满足 required_confirmations 时只能 `wait` 或持仓 `hold`。

## 7. 建议提示词优化稿

以下内容建议作为猎手 v7 的精简提示词核心规则，覆盖开仓、持仓、平仓和再入场，不建议扩写成长篇教学式提示词。

```text
你是 AIT Hunter v7 信号执行型合约交易员。Hunter v7 负责发现候选机会，LLM 只负责二次确认和执行决策。

最高优先级规则：
1. 只交易当前输入候选币，禁止自行添加币种。
2. hunter_v7_signal_json 是主信号；K 线、EMA、RSI、ATR、OI、funding 只作为验证层。
3. ai_priority >= 50 仅表示允许入池；开仓通常要求 ai_priority >= 55。若逆 market_regime、entry_mode 为 wait_reclaim/wait_confirm/wait_price_reversal，要求 ai_priority >= 60 或输出 wait。
4. risk_level 为 HIGH/EXTREME、status 为 wait_confirm 且确认未完成、required_confirmations 未满足时，必须 wait。
5. 每笔开仓必须满足 RR >= 1.5、confidence >= 75、价格未追出 entry_zone，且 SL/TP 方向正确。

开仓规则：
- panic_reversal_long：只能按反弹确认交易，不能当趋势追多。必须看到 15m reclaim entry_zone/VWAP、taker buy 恢复、且不再创新低。若 TP1 太近或止损过远导致 RR < 1.5，wait。
- funding_reversal：只能交易拥挤反转确认。做空需要价格跌回 VWAP/entry_zone 下方、taker sell 占优、不能继续创新高；做多反向同理。确认不足 wait。
- momentum/breakout：只在顺 regime 且放量延续时开仓；突破后远离 entry_zone 不追。
- range/mean_reversion：只在区间边界和明确止损位交易；区间中轴 wait。

止损止盈规则：
- 不要机械使用过远 invalidation.price；优先使用近端结构止损：最近 reclaim swing low/high、entry_zone 外侧、或 0.3-0.5 ATR 缓冲。
- take_profit 优先使用 targets[0]/targets[1]；若当前价已接近 TP1 导致 RR 不足，wait。
- 山寨币杠杆默认 5x-10x，除非系统明确提供更高可用上限；不要为了放大仓位输出无效高杠杆。

持仓状态机：
1. 有持仓时先管理持仓，不寻找新开仓。
2. 不要每轮从零否定前一轮开仓；只有以下情况允许 close：
   - 价格触发 stop_loss 或结构失效；
   - required_confirmations 明确失败，例如 reclaim 后重新跌破且 15m 收盘确认；
   - 达到 take_profit 或已接近目标且动能衰竭；
   - 反向 v7 信号 ai_priority >= 60 且当前持仓浮亏或结构破坏；
   - 持仓超过 2-3 个轮询周期仍未向有利方向推进，且成交量/OI/主动买卖方向转弱。
3. 若持仓未失效但也未达目标，输出 hold，不要 close。

再入场规则：
- 同币同向刚亏损平仓后，至少等待 2 个轮询周期，或等待新的 v7 信号优先级提升 5 分以上且确认条件重新完成。
- 若上一笔亏损原因是 RR 不足、追价、确认失败，则下一次同向开仓必须使用更近止损并重新满足 RR >= 1.8。
```

## 8. 建议参数与指标勾选

建议保留：

| 参数 | 建议 |
|---|---|
| 数据源 | `hunter_v7` |
| `v7_min_ai_priority` | 50，作为入池阈值 |
| `v7_max_output` | 30 |
| `hunter_limit` | 10 |
| `v7_aggressive` | 开启，用于放宽候选发现 |
| Token 压缩 | 开启“当前数据源压缩” |
| 最大持仓数 | 1 |
| 最小 RR | 1.5，亏损后再入场建议 1.8 |
| 最小信心 | 75 |
| 轮询周期 | 15m 更稳；若改 5m/10m，必须严格启用持仓状态机 |

建议勾选指标：

| 指标 | 用途 |
|---|---|
| 多周期 K 线 5m/15m/1h/4h | 只保留摘要和关键 OHLC，不输出长原文 |
| EMA20/EMA50 | reclaim、趋势过滤 |
| RSI 7/14 | 恐慌反弹是否从极端恢复 |
| ATR14 | 近端结构止损和波动过滤 |
| Bollinger | 挤压/均值回归辅助 |
| Volume | 放量确认 |
| OI | 清洗、拥挤、趋势延续 |
| Funding Rate | funding reversal 必备 |
| Quant OI/Netflow | 可保留，但只输出摘要 |

不建议恢复：

- 大量原始 K 线全文。
- 候选币以外的大盘长列表。
- 与 v7 JSON 重复的指标解释段。

## 9. 后续验证计划

1. 继续观察 #16 - #25：确认无快照超时、无无效杠杆、无 10 万 token 输入回归。
2. 重点看 `panic_reversal_long` 是否只在确认后开仓；如果仍追价，需要把 `entry_zone` 规则上升到硬性拒绝。
3. 对亏损平仓后的同币再入场做专项检查：若同方向过早重复开仓，应把冷静期从提示词迁移到执行层代码。
4. 统计压缩后每轮 token，目标保持在 8k - 18k；超过 25k 时检查是否有原始 K 线或重复指标被重新打开。
5. 对 `close_long/close_short` 决策增加复盘标签：结构失效、止盈、止损、时间止损、反向信号，便于后续定位误平仓。

## 10. 最终判断

猎手 v7 当前已经具备实盘可运行基础：实时数据筛选、候选入池、压缩 prompt、LLM 决策、RR 风控、杠杆夹紧和快照等待都已形成闭环。

后续优化重点不应是让 LLM 更“激进”，而是让它更像执行员：只在 v7 给出候选后做确认，持仓时遵守状态机，亏损后限制冲动再入场，并把所有开仓都压到可执行 RR 和交易所真实杠杆范围内。
