# AIT 实盘交易周期 #95/#96 复盘评估报告

生成时间：2026-06-06  
复盘对象：交易员 VVV，策略：猎手v7，当前实例 `d7b3b284_445bf32d-9add-4960-a925-29539fa66e69_openai_1780617186`  
数据来源：`data/data.db` 中 `decision_records`、`trader_orders`、`trader_positions`、`trader_equity_snapshots`

## 1. 总结结论

周期 #95、#96 均为有效实盘决策链路，数据源有返回候选币，LLM 调用成功，交易引擎执行成功，最终均选择 `WAIT`，没有新开仓，也没有订单失败。

本轮复盘判断：这两轮不是数据源问题，也不是交易引擎执行问题，而是候选信号未达到策略开仓硬条件。#95 全部候选都是 C 级信号，最高优先级 TRXUSDT 也只有 `ai_priority=63.1`、`timing_score=47`，不满足 C 级开仓门槛。#96 出现 A 级 GUAUSDT，但 `timing_score=30` 且保守 RR 仅约 `0.95`，低于最低 `1.5`，拒绝开仓是合理的。

评估：这两轮 WAIT 偏审慎，但不是误判。若强行放开，实际会把系统重新推向前面亏损订单中出现过的“追入波动尾段、止损距离过宽、盈亏比不足”的问题。

## 2. 周期概览

| 周期 | 时间 UTC | 时间 CST | 状态 | 候选币数 | AI耗时 | Token | 执行结果 |
| --- | --- | --- | --- | ---: | ---: | ---: | --- |
| #95 | 2026-06-06 03:03:54 | 2026-06-06 11:03:54 | 成功 | 9 个候选，实际决策 7 个 | 14.875s | 15,519 | 7 个 WAIT |
| #96 | 2026-06-06 03:12:24 | 2026-06-06 11:12:24 | 成功 | 10 个候选，实际决策 7 个 | 16.946s | 15,799 | 7 个 WAIT |

两轮权益快照：

| 时间 UTC | 权益 | 余额 | 未实现盈亏 | 持仓数 | 保证金占用 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 2026-06-06 03:03:26 | 14.80915995 | 14.80915995 | 0 | 0 | 0% |
| 2026-06-06 03:11:53 | 14.80826768 | 14.80826768 | 0 | 0 | 0% |

周期 #95/#96 前后没有新订单记录，最近一笔已关闭仓位是 2026-06-06 02:09:53 的 GUAUSDT 空单平仓，已实现盈利约 `+0.3689 USDT`。

## 3. 周期 #95 逐币复盘

| 候选 | 方向 | 置信 | ai_priority | timing_score | 状态 | 主要拒绝原因 |
| --- | --- | --- | ---: | ---: | --- | --- |
| TRXUSDT | LONG | C | 63.1 | 47 | wait_confirm | timing < 60；C 级要求 ai_priority >= 65 且 timing >= 70；等待反转确认 |
| XMRUSDT | LONG | C | 61.1 | 62 | candidate | C 级 ai_priority 与 timing 均不达标；15m/1h 仍在 EMA20 下方 |
| ICPUSDT | SHORT | C | 58.2 | 47 | wait_confirm | timing < 60；C 级优先级不足；等待确认 |
| LTCUSDT | SHORT | C | 56.4 | 72 | candidate | ai_priority 55-60 区间只允许 A/B；C 级不允许开仓 |
| BSBUSDT | SHORT | C | 55.9 | 67 | candidate | C 级不允许；taker sell 只略过阈值，OI 回落很弱 |
| SEIUSDT | LONG | C | 52.7 | 60 | candidate | ai_priority 50-55 只观察；C 级优先级不足 |
| CLOUSDT | SHORT | C | 51.5 | 34 | wait_confirm | timing 极低；ai_priority 只观察；OI 仍在快速增加，未见有效 flush |

### #95 评估

#95 的候选池有数据，但质量集中在 C 级，且很多只是 `candidate/wait_confirm`。策略没有从 C 级低优先级标的中强行找交易，符合当前“宁可错过，不接低质量反转”的设定。

最接近开仓的是 TRXUSDT，但它的 `timing_score=47`，并且仍是 `wait_confirm`，说明 v7 数据源本身也没有确认方向触发。此时开仓属于提前赌反转，不符合猎手v7当前定位。

## 4. 周期 #96 逐币复盘

| 候选 | 方向 | 置信 | ai_priority | timing_score | 估算 RR | 主要拒绝原因 |
| --- | --- | --- | ---: | ---: | ---: | --- |
| GUAUSDT | LONG | A | 67.0 | 30 | 0.95 | A 级但 timing 极低；15m 已大涨，追入 RR < 1.5 |
| ICPUSDT | SHORT | C | 64.5 | 72 | 0.35 | C 级 ai_priority 略低于 65；止损 invalidation 过远，RR 极差 |
| TAUSDT | SHORT | C | 59.8 | 75 | 0.39 | C 级优先级不足；虽然确认较多，但止损距离过宽 |
| TRXUSDT | LONG | C | 59.7 | 45 | 未开仓 | wait_confirm；timing < 60；C 级不足 |
| XPLUSDT | SHORT | C | 57.4 | 72 | 0.33 | C 级优先级不足；RR 极差 |
| LTCUSDT | SHORT | C | 56.4 | 72 | 0.22 | C 级优先级不足；RR 极差 |
| SEIUSDT | LONG | C | 52.7 | 60 | 1.57 | RR 高于 1.5 但 C 级要求 >= 2.0；ai_priority 与 timing 均不足 |

### #96 评估

#96 是更关键的一轮，因为 GUAUSDT 出现了 A 级信号，表面看像可开仓。但 AI 拒绝的两个理由都成立：

1. `timing_score=30`，说明 v7 判断当前并不是最佳入场时机。
2. 当前价约 `0.7739`，TP1 `0.8617`，失效价 `0.6811`，RR 约 `0.95`，低于策略最小 `1.5`。

GUAUSDT 在 5m RSI7 已到 `73.9`，15m 单根涨幅明显，且 4h 仍处在下跌结构内。此时做多更像追短线 squeeze 后半段，而不是低位确认反转。结合前面 GUA/LAB 等亏损单的根因，拒绝这笔交易是合理的风控动作。

## 5. 数据源还是提示词问题？

判断：不是数据源断链。

证据：

- #95 返回候选：TRX、XMR、ROBO、ICP、LTC、BSB、SEI、CLO、IN。
- #96 返回候选：GUA、ICP、IN、SLX、TA、TRX、XPL、LTC、ROBO、SEI。
- 两轮候选列表发生变化，且同一标的的价格、评分、资金费率、taker buy、OI 等字段都有更新。
- `decision_records.success=1`，AI 调用耗时和 token 统计完整。
- `execution_log` 显示逐币 `wait succeeded`，不是候选为空、快照未就绪、余额失败或解析失败。

判断：也不是交易引擎执行问题。

证据：

- 两轮没有 open_long/open_short 决策，因此没有订单创建。
- `trader_orders` 在 03:03-03:12 UTC 附近没有新增订单。
- 账户空仓、保证金占用 0%，不存在因已有仓位导致无法开仓。

真正原因：策略硬门槛阻止了低质量机会，尤其是 C 级信号、低 timing、低 RR 和过宽 invalidation。

## 6. 需要注意的结构性问题

1. v7 候选里部分 `invalidation` 明显过远，导致 RR 被极度压低。例如 ICP、LTC、XPLUS 的 SHORT 信号，止损价离当前价过远，AI 按保守 RR 拒绝是正确的，但这也说明数据源给出的失效位更像大级别结构失效，而不是高频波段止损。

2. `Hunter STRONG` 的展示文案容易误导。#95 里 55-63 分的 C 级候选仍显示 `Hunter STRONG / Full conviction`，但提示词规则又要求 C 级严格过滤。这会造成语义冲突：数据源文本像强信号，策略规则却要求等待。建议后续把 v7 展示文案改成按区间区分：`Observe / Candidate / Strong / Triggered`。

3. #96 的 GUAUSDT 是典型“方向可能对，但入场性价比不够”。如果未来想捕捉这类 squeeze，应当通过 TP1/移动保护和更近的结构止损做独立实验，而不是直接放松全局 RR 门槛。

## 7. 优化建议

短期不建议因为 #95/#96 全 WAIT 而大幅放松猎手v7提示词。当前 WAIT 质量较高，避免了低 RR 追单。

建议做三项小幅优化：

1. 对 A 级 squeeze 信号增加“二次入场条件”，而不是直接开仓：当 `timing_score < 60` 但 `confidence=A`、`risk_score<=10` 时，要求价格回踩到 entry_zone 下半区或重新站上 5m EMA 后再开。

2. 对高波动山寨币 RR 使用“近端结构止损 + TP1/TP2”评估：如果 v7 invalidation 过远，允许 AI 同时计算一个 closer structural stop，但必须写明失效依据，且不得把止损设在噪音区。

3. 把报告中提到的 `Hunter STRONG` 文案与开仓门槛对齐，避免 LLM 在“强信号文案”和“C 级禁止开仓规则”之间反复拉扯。

## 8. 最终评估

周期 #95：WAIT 合理，候选池整体偏弱，未见可执行开仓信号。  
周期 #96：WAIT 合理，GUAUSDT 方向信号强但入场赔率不合格，其它候选更弱。  

综合结论：最新两轮没有暴露数据源或执行链路故障，反而说明优化后的策略在低 RR、低 timing、C 级候选面前能守住纪律。后续重点不应是简单放松，而是针对 A 级 squeeze 设计更细的回踩/二次确认入场规则，以及修正 v7 高级别 invalidation 导致高频 RR 失真的问题。
