# HHH Hunter v7 移动止盈复盘与修复记录 - 2026-06-10

## 结论

近几轮未开仓不是数据源筛选卡死。#159-#164 已连续出现 EXECUTABLE 候选与真实开仓：

- #159 BLESSUSDT open_long，后续保护器 TP1 平仓，净盈利。
- #161 KGENUSDT open_long，#162 hold，#163 AI 主动 close_long。
- #164 KGENUSDT open_long，后续保护器 TP1 平仓，净盈利。

本轮重点问题是 #163 的 KGENUSDT 微利平仓：它不是移动止盈保护器直接触发，而是 LLM 在持仓管理里主动输出 `close_long`。根因是提示词把 `Peak PnL -> 当前 PnL` 的回吐规则写得过强，LLM 将 Peak 4.20% 这种低于 near-TP1 的波动也按“回吐 >100% 必须退出”处理。

## 关键证据

### KGENUSDT #145

- Entry: 0.15732
- Exit: 0.15752
- Quantity: 89
- Leverage: 20x
- Realized PnL: +0.0178 USDT
- Fee: 0.01401038 USDT
- Holding: 1199s
- Decision: cycle #163 `close_long`

#163 决策理由摘要：

- Peak PnL 4.20% -> 当前 -0.87%
- 价格跌破 entry 且低于 5m EMA20
- 5m RSI 中性偏弱，15m/1h RSI 弱势
- LLM 判断“峰值回吐严重，应 close 保护本金”

保护器日志在此前只记录：

- Profit 4.20%
- Peak 4.20%
- TP1=false
- TP2=false

因此 #163 属于 AI close 过敏，不是保护器阈值直接误触发。

### KGENUSDT #146

- Entry: 0.15798639
- Exit: 0.15872
- Quantity: 97
- Leverage: 20x
- Realized PnL: +0.07115999 USDT
- Fee: 0.01536025 USDT
- Holding: 354s
- Exit source: position protector TP1

这笔是正常保护器 TP1 平仓。说明保护器在达到 6% 左右保护 TP1 时能正确介入。

## 修复

1. 系统提示词分层：
   - Peak PnL >= 5.7% 才属于 protection near-TP1。
   - near-TP1 后回撤 >=45% 或回到盈亏平衡/亏损，才优先视作移动止盈/减风险信号。
   - Peak PnL < 5.7% 明确标为 pre-TP1 波动，峰值回吐或微利/微亏本身不能单独触发 close。

2. 当前持仓上下文新增保护状态：
   - `protection_state=pre_tp1`：提示 LLM 不得仅凭 peak giveback 退出。
   - `protection_state=near_tp1_or_better`：允许移动止盈规则优先生效。

3. 保留结构性退出：
   - 即使 pre-TP1，只要计划 SL/硬失效触发，或 5m 与 15m 同时确认结构反转，仍允许 `close_long`/`close_short`。
   - 这样避免过敏平仓，同时不牺牲真正反转时的保护。

## 预期影响

- 降低 Peak 3%-5% 杠杆浮盈回吐后的微利/微亏平仓概率。
- 提高持仓给到 TP1 的机会，减少“刚要走出来就被 AI close”的情况。
- 不放松开仓硬风控，开仓率改善主要来自持仓不过早释放/反复重开造成的交易质量提升。

## 后续跟踪项

- 继续统计之后 10-20 个周期内 `close_long/close_short` 的触发来源：AI close、TP1/TP2 protector、SL/TP exchange order。
- 对每个 AI close 记录 Peak PnL 是否低于 5.7%，若仍出现 pre-TP1 微利 close，继续加后端 close guard。
- 对保护器 TP1 全平的小账户行为继续观察：当前小仓位因最小名义金额限制会全平，若后续净利润稳定但过早，可再引入分段降杠杆或更高 TP1。
