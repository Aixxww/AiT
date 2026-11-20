# AiT standing design standard (owner, 2026-11-20)

Highest standard for AiT architecture-logic design and the yardstick for every design decision in this repo. When a proposal or instruction conflicts with it, flag the conflict to the owner instead of silently following it.

Recorded in full (Chinese original and English rendering) in `docs/t2-redesign-plan-20261118.md`, section "Standing design standard".

> 数据源筛选规则设计原则是，在每一次轮询中，发掘捕捉出当下几百个合约对中，不管是什么行情形态，能通过当前最新数据指标预判出方向且能有较高盈利RR 1.2或以上，值得当下入场的三五个信号标的。能将完整将最新数据指标喂给LLM，而LLM根据策略提示词分析出最终目标，以json输出出交易引擎以市价入场。

Checklist for any design change:

1. Every round screens all of the hundreds of available contracts, in any market regime or pattern.
2. It surfaces 3–5 targets whose direction the latest indicators can call, with RR ≥ 1.2, worth entering now.
3. The LLM receives the complete, latest indicators for those targets.
4. The LLM's JSON decision goes to the engine, which enters at market.
