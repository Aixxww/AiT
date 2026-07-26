# Hunter v7 六轮实盘盈利印证与形态筛选优化报告

> 测验：2026-07-27 00:28-00:57，每 5 分钟一轮 × 6 轮，Binance USD-M 实时数据，regime=trend_up（全程）
> 盈利判定：前 2 轮开仓池（EXECUTABLE/REVIEWABLE 全 JSON）信号，以第 6 轮为截止用 1m K 线正向判定 TP0/TP/SL/浮盈亏 + MFE/MAE
> 数据：`reports/hunter-v7-6round-profit-20260727/`（原始 6 轮 + 自动分析）

## 1. 总览

| 指标 | 数值 | 判读 |
|---|---|---|
| 信号总量 | 69（每轮 10-14，SignalRate 5.5%） | 稳定，无轮次塌缩 |
| 开仓可复核池 | 20（OpenRate 29.0%，ExecRate 15.9%） | 深夜低波动段的正常水位 |
| 格式问题 / 执行缺口 / REST 错误 | 0 / 0 / 0 | 链路干净 |
| 数据新鲜度 | AgeP50 30.7-41.9s，StalePct 全 0 | 30-60s 决策预算内 |
| 形态覆盖 | 6 类 setup（动量 3 + 反转 2 + 短线 1） | trend_up 夜盘的合理路由集中 |

## 2. 盈利印证（6 个跟踪样本，~25 分钟前向窗口）

| 标的 | 形态 | Tier | 结果 | PnL | MFE/MAE | 判读 |
|---|---|---|---|---|---|---|
| SOONUSDT | alt_ladder | EXEC | **TP0** | 0.00%* | +1.54/-1.34 | TP0 微止盈按设计触发 |
| BANKUSDT(r2) | alt_ladder | EXEC | **TP0** | 0.00%* | +0.79/-0.61 | 同上 |
| BANKUSDT(r1) | trend_breakout | REVIEW | **TP0** | +0.31% | +1.45/-0.78 | 复核池样本，达 TP0 |
| ETHUSDT | displacement | REVIEW | 浮亏 | -0.49% | +0.00/-0.52 | 入场即回撤，未破止损 |
| DIAUSDT | whale_flow | EXEC | 浮亏 | -1.64% | +0.89/-2.16 | **实盘不可成交**（见 §4.1） |
| ZAMAUSDT | whale_flow | EXEC | 浮亏 | -0.71% | +0.58/-1.69 | **实盘不可成交**（见 §4.1） |

\* TP0 命中后按保本口径计。

**核心结论**：剔除两个实盘必被 trader 守卫否决的 whale_flow 假想仓位后，**真实可成交样本 4 个中 3 个命中 TP0（alt_ladder 2/2、trend_breakout 1/1）**，唯一浮亏 ETH -0.49% 未及止损。动量族"TP0 微止盈 + 保本推进"的执行语义在实测中成立。样本量小（n=6/一夜/单一 regime），只作机制验证，不作胜率结论——真实胜率验证路径见 §6。

## 3. 各形态筛选分析

| 形态 | 信号 | EXEC/REVIEW | 主要滞留原因 | 判读与处置 |
|---|---|---|---|---|
| alt_ladder_momentum_long | 18 (26%) | 2/0 | `taker_buy_15m_gt_0_52` 缺口×9、`backend_rr_infeasible`×3 | 健康：taker 现场确认缺口正是 live-reviewable 升级通路的目标；RR 不可行拦截为几何保护，两个放行样本均 TP0 |
| funding_reversal | 18 (26%) | 0/0 | `15m_close_below_vwap` 缺口×10 | trend_up 夜盘费率空单全部卡在等 VWAP 下破——逆势保护按设计工作；26% 信号量 0 产出是 regime 使然，成本仅 WATCH 摘要行（见 §5.2） |
| displacement_momentum_long | 13 (19%) | 3/1 | `momentum_not_exhausted`×2 | 正常：跟踪样本 ETH 属大市值低位移，模块本为高波动位移设计，出现在此为 regime 权重放行 |
| whale_flow_reversal | 11 (16%) | 6/0 | `taker_flow_confirms_long`×5 | **异常**：ExecRate 全场最高（54.5%）但全部处于 zone 56-67%，实盘 100% 被 trader 否决（§4.1）；本次已修 |
| trend_breakout_long | 5 (7%) | 0/2 | `5m_or_15m_close_through_breakout_level`×3 | 健康：突破确认缺口滞留合理，放行样本 TP0 |
| intraday_scalp_long | 4 (6%) | 0/0 | `5m_or_15m_close_above_trigger`×2 | 触发价未到，正常 |

## 4. 已实施优化（本次提交）

### 4.1 whale_flow Ready 门收紧（规则表一行 + 一个 guard）
实测证据：6 次 whale_flow EXECUTABLE 展开全部处于 entry_zone 56-67% 位置，而 trader 硬守卫（`auto_trader_risk.go` whale LONG 门）在 zone>45% 一律拦截——分类器用泛化 ready 兜底门（`execution_quality_ready`，无 taker/无 zone 检查）把**注定不可成交**的候选推进 EXECUTABLE，白耗 LLM 复核与开仓槽，且跟踪的两个假想仓位均深度浮亏（MAE -2.16%/-1.69%），说明该拦截本身是对的。
修复：whale_flow 的 Ready 行加 `hunterV7WhaleFlowLongEntryGatesOK` guard（LONG：zone≤45% 且 taker_buy_15m≥0.56，镜像 trader 门；SHORT 不受影响；缺数据放行留给决策时复核），tier_reason 改为 `whale_flow_ready_zone_and_flow_ok`。效果：此类候选停留 WATCH，EXECUTABLE 槽位只给真正可成交者——**提高有效开仓率的精度**。

### 4.2 提示词：泛化 ready 门审验行
tag policy 新增：`tier_reason=execution_quality_ready` 的候选只过了泛化分数门、没有 setup 专属闸门——开仓前必须额外核对 entry_zone_position 与实时 taker；数据不可得时保守仓或 wait。覆盖 volatility_squeeze/intraday 等仍走泛化门的形态与未来新形态。

## 5. 待裁决优化（需策略教义决定，未擅动）

### 5.1 【重要】trader whale LONG 三道门自相矛盾
`validateHunterV7WhaleFlowGuard`：①zone>45% 拦截；②taker<0.56 拦截；③价格低于 zone 中点拦截。①与③联合覆盖全部价格位置（≤45% 必在中点下方触发③，≥50% 触发①，45-50% 双拦）——**zone 数据完整时 whale LONG 永远开不出**，仅 zone 数据缺失时能靠 taker 单腿通过。两道门同一提交引入（4f8c797），属先天矛盾。
建议：二选一教义——(a) 低吸教义：删③保①②（与 prompt 政策第 8 条一致，推荐）；(b) 确认教义：删①、③反向为"价格须站上中点"并同步改政策第 8 条与本次的分类器 guard。**涉及实盘开仓行为，请裁决后我再实施。**

### 5.2 funding_reversal 的 trend_up 噪声：建议维持现状
18 信号 0 产出但成本仅 WATCH 摘要行（无全 JSON 展开），且这是逆势保护的正确形态；等 regime 转向后它就是主力形态。收紧 Match 或降权会伤害 regime 切换时的响应速度。用 §6 的 outcomes 数据两周后复核。

### 5.3 alias 映射删除
翻转后实盘已确认 0 旧名流通；下个会话删 `v7TakerTagAliases` 与 payload 端调用（纯清理）。

## 6. 真实胜率验证路径（超出单夜测验的部分）

单夜 6 轮只能验证机制，不能给出胜率。已具备的长期回路：
1. `/hunter/v7/outcomes?days=N`：按 setup×regime 输出 30m 窗口 WIN_TP0 与 2h 窗口 WIN_TP1 统计（signal store 自动跟踪每个信号的真实后续走势）；
2. 建议以 2026-07-27 为界拉 before/after 各 7 天对比：重构+词汇统一+whale 门修复对 ExecRate 与 WIN_TP0/TP1 的净效应；
3. 潜力池强制跟踪（validate 报告 §6）持续记录"未命中模块的高分标的"，用于发现漏筛形态。
