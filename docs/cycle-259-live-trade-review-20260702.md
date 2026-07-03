# AIT 周期 #259 后实盘交易复盘与优化方案

复盘时间：2026-07-02  
复盘范围：HHH 交易员周期 #259 到 #289，起点按 #259 修正。  
数据来源：`data/data.db` 的 `decision_records`、`trader_positions`、`trader_orders`、`hunter_v7_signal_records`，以及 `.logs/backend.log`、`data/ait_2026-07-02.log`。

## 结论摘要

从 #259 开始统计，实盘共开 7 笔，4 笔亏损、3 笔盈利，总已实现 PnL 为 -1.9981 USDT，手续费 0.3597 USDT。亏损集中在 VELVETUSDT、UBUSDT、GTCUSDT、IDOLUSDT。

核心问题不是单一“方向判断错误”。VELVET/UB 主要暴露入场质量、数据质量、极端波动追单与平仓来源标记不足；GTC/IDOL 则明确暴露浮盈保护失效：GTC 峰值盈利 5.99% ROE 后最终亏损，IDOL 峰值盈利 16.15% ROE 后最终亏损。

当前平仓记录的 `close_reason/source` 均为 `sync/sync`，数据库无法直接区分手动平仓、交易所条件单触发、外部操作或本地系统保护单。日志能证明 GTCUSDT 和 IDOLUSDT 是系统 `Trail protection close triggered` 后平仓；VELVETUSDT 和 UBUSDT 在本地记录里只看到交易所同步平仓，不能直接断言是 AI 主动平仓。

## 交易明细

| 周期 | 标的 | 方向 | 入场 UTC | 出场 UTC | 杠杆 | 入场价 | 出场价 | PnL | 关键结果 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| #259 | VELVETUSDT | LONG | 2026-07-01 20:10:04 | 20:50:25 | 10x | 1.534145 | 1.500300 | -1.2861 | 入场后连续持仓，最终接近 SL 价同步平仓 |
| #264 | UBUSDT | SHORT | 2026-07-01 21:00:05 | 21:06:10 | 20x | 0.085240 | 0.08676885 | -0.9815 | 6 分钟内反抽亏损，疑似交易所止损/外部平仓同步 |
| #268 | GTCUSDT | LONG | 2026-07-01 21:40:09 | 23:01:25 | 10x | 0.074410 | 0.073830 | -0.4044 | 峰值 +5.99% ROE 后回吐到亏损，被系统 giveback close |
| #279 | IDOLUSDT | SHORT | 2026-07-01 23:30:03 | 2026-07-02 00:10:08 | 20x | 0.01636244 | 0.01644575 | -0.2573 | 峰值 +16.15% ROE 后回吐到亏损，被系统 giveback close |
| #284 | SYNUSDT | LONG | 2026-07-02 00:20:01 | 00:25:26 | 10x | 0.518360 | 0.52214764 | +0.3371 | trail close 盈利 |
| #285 | SYNUSDT | LONG | 2026-07-02 00:30:02 | 00:34:45 | 10x | 0.518170 | 0.52228020 | +0.3987 | trail close 盈利 |
| #288 | VVVUSDT | SHORT | 2026-07-02 01:00:06 | 01:03:19 | 20x | 12.708000 | 12.656000 | +0.1955 | 快速盈利平仓 |

## 分标的复盘

### VELVETUSDT LONG (#259)

开仓理由是 `whale_flow_reversal`，信号评分高：priority 87.6、setup 99.0、timing 79.6、risk 0.0，且 AI 认为价格在入场区、15m 高于 EMA/VWAP、taker buy 0.538、OI 1h +7.74%、4h +8.19%。

问题在于三点：

1. 开仓前置修正已经显示执行环境不理想：止损距离因小账户风控被从 1.492 修正到 1.5003175，仓位从 120 USDT 降到 58.70 USDT。这说明信号结构本身与账户可承受风险不匹配。
2. 后续周期 #260-#263 持仓判断过度依赖 EMA/结构未破。#262 里 AI 已看到当前 PnL -14.88%，但仍把它归类为 pre-TP1 波动，继续 hold。
3. VELVET 多周期 K 线 later 出现 Binance zero-volume fallback 到 CoinAnk 的数据质量问题。虽然开仓时日志显示使用 SnapshotStore，但持仓管理期多次出现 Binance 5m/15m/1h/4h 全零量回退，说明该标的实时数据一致性风险偏高。

结论：这是“高评分反转信号 + 数据质量不稳定 + pre-TP1 亏损容忍过大”的组合失败。

### UBUSDT SHORT (#264)

UB 是 `range_expansion_event` 空单，开仓前 24h 跌幅约 -14.01%，taker sell aligned，信号从 REVIEWABLE 变成 EXECUTABLE。策略选择了追随下跌延续。

问题在于：

1. 24h 已大幅下跌后追空，容易遇到短线空头回补。后续信号也出现 UBUSDT LONG 且带 `short_covering_not_new_long_build`，说明当时反抽更像挤空/回补，而不是新的多头趋势。
2. 开仓后 6 分钟内被反抽到 0.08676885，超过动态止损更新后的 0.08673170 附近，亏损迅速扩大。
3. 当前 range expansion 对“极端跌幅后的追空”惩罚不足，只看到了事件延续，没有足够要求回踩/retest 或二次确认。

结论：这是“下跌末端追空 + 短线回补风险未被降级”的失败。

### GTCUSDT LONG (#268)

GTC 是 `displacement_momentum_long`，入场时带 `low_liquidity` 风险标签。它不是完全错误的方向判断：持仓期间最高到 Peak PnL 5.99%，接近 near-TP1 阈值 5.70%。

关键问题在退出：

1. #276 决策已显示 Peak PnL 5.99% 超过 near-TP1 阈值，但因为 `raw_move=-0.03% < 1.00%`，AI 仍输出 hold。
2. 系统保护最终在 mark 0.07387000 触发 `giveback_close`，实际成交 0.07383，低于入场 0.07441，导致 -0.4044 USDT。
3. 动态保护止损在盈利后仍未锁到保本线以上。对于 10x 合约，峰值 5.99% ROE 已经足够至少锁保本或小幅盈利。

结论：方向有过盈利窗口，但利润保护规则没有把 ROE/MFE 转化成硬保护，导致盈利单变亏损单。

### IDOLUSDT SHORT (#279)

IDOL 是 `range_expansion_event` 空单，信号强度高：setup 90.2、timing 87.0、ready 85.2，但带 `high_volatility`、`moderate_liquidity`、`execution_stop_tightened`。

关键问题更明显：

1. 持仓约 10 分钟后 Peak PnL 达到 16.15%，但 TP1 仍为 false，系统没有半仓止盈或保底止盈。
2. #280-#283 AI 持续 hold，理由反复依赖 “raw_move 未达 1%” 和 “未触及 TP1/SL”，忽略了 20x 下 16.15% ROE 的回吐风险。
3. 系统不断把 short 动态保护止损从 0.01664878 下调到 0.01659259，但该价格仍高于入场均价 0.01636244，不能锁定净盈利。最终 mark 0.01640824 触发 giveback close，成交 0.01644/0.01645，亏损 -0.2573 USDT。
4. Binance K 线在持仓期也出现 IDOLUSDT 多周期 zero-volume fallback，进一步放大了高波动标的的跟踪风险。

结论：这是本轮最典型的“浮盈保护失效”。开仓方向曾正确，但止盈/移动保护没有跟上。

## 形态模块开仓率与跟踪胜率

统计范围：`hunter_v7_signal_records` 在 2026-07-01 20:00:00 至 2026-07-02 01:20:00 UTC，排除 `filtered`。

| 模块 | 信号数 | EXECUTABLE 数 | 开仓率 | 跟踪胜率 | 复盘判断 |
| --- | ---: | ---: | ---: | ---: | --- |
| whale_flow_reversal | 74 | 50 | 67.6% | 8.6% | 过度放行，VELVET 属于该类；需要强制数据质量与价格确认 |
| range_expansion_event | 291 | 46 | 15.8% | 39.6% | 数量最多，UB/IDOL 属于该类；需要区分早期突破、末端追单、短回补 |
| displacement_momentum_long | 37 | 4 | 10.8% | 18.2% | GTC 属于该类；低流动性下 TP/保护要更主动 |
| panic_reversal_long | 89 | 0 | 0.0% | 50.0% | 没有开仓，但跟踪胜率不差，可作为反向挤压识别辅助 |
| trend_breakout_long | 186 | 0 | 0.0% | 7.7% | 当前过滤保守是合理的 |
| funding_reversal | 214 | 0 | 0.0% | 无有效闭环 | 候选很多但不开仓，需要减少噪音或拆分 watch-only |

模块层面最需要处理的是 `whale_flow_reversal` 的开仓率过高但跟踪胜率极低，以及 `range_expansion_event` 对末端追涨追跌的误判。

## 根因归纳

1. 持仓保护以 TP1 价格触达和 `raw_move` 为核心，缺少基于杠杆 ROE/MFE 的强制锁利规则。
2. AI 持仓提示词允许在 -8% 到 -15% ROE 时继续用 EMA/VWAP 未破来解释 hold，导致 pre-TP1 亏损容忍过大。
3. `range_expansion_event` 对 24h 极端涨跌后的追单风险识别不足，尤其是已经跌超 10%-15% 后继续追空。
4. `whale_flow_reversal` 评分过度依赖 OI/主动买入，缺少数据源一致性、二次回踩和真实成交量确认。
5. 平仓来源记录不足，`sync/sync` 无法满足实盘复盘；需要记录系统主动 close、交易所 SL/TP、手动/外部平仓。

## 优化方案

### P0：立即修复

1. 增加利润地板锁：当 Peak ROE >= 5% 时，active SL 必须至少锁到保本 + 手续费 + 滑点缓冲；Peak ROE >= 10% 时至少锁定峰值的 25%-35%；Peak ROE >= 15% 时至少锁定峰值的 40%-50%，或强制部分止盈。
2. 增加 pre-TP1 硬亏损规则：入场后 10-15 分钟内 ROE <= -8% 且无强反转确认时减仓/平仓；ROE <= -12% 时禁止仅凭 EMA 未破继续 hold。
3. 修复 giveback close：触发回吐保护时，若曾经 Peak ROE >= 5%，不得允许最终保护价落在净亏损区。
4. 平仓来源打标：本地系统触发的 `giveback_close`、`trail_close`、`stop_loss`、`take_profit`、`manual_or_external` 必须写入 `close_reason/source`。

### P1：信号筛选修正

1. `range_expansion_event` 增加末端追单过滤：24h 跌幅超过 10%-15% 后追空，必须等待反抽失败/retest 或二次放量跌破；否则降级到 WATCH。
2. `whale_flow_reversal` 增加数据质量门槛：若 Binance 多周期 K 线 zero-volume 或 fallback，不能给 EXECUTABLE，只能 WATCH/REVIEWABLE。
3. 对 `high_volatility`、`low_liquidity`、`execution_stop_tightened` 组合加权惩罚，限制杠杆或要求更短 TP0。
4. 对小账户风控修正后的单子增加执行降级：只要止损被大幅修正且仓位被压缩超过 40%，AI 需重新评估 RR 是否仍可开仓。

### P2：止盈结构

1. 引入 TP0/半仓止盈：高波动 20x 标的 Peak ROE >= 6%-8% 或触达 TP0 时，先止盈 30%-50%。
2. near-TP 不只看目标价触达，也要看 ROE/MFE。IDOL 这类 20x 单，16.15% ROE 不应继续等待完整 TP1。
3. 高波动标的使用时间衰减：盈利 10 分钟后若无法继续创新高，应主动收缩目标或锁定利润。

### P3：提示词与复盘字段

1. 持仓提示词增加硬约束：当当前 ROE <= -8% 或 Peak ROE >= 5% 后回吐超过 50%，AI 必须优先解释为何不平仓。
2. 决策记录增加 `current_roe`、`peak_roe`、`mfe`、`mae`、`giveback_pct`、`close_source`，让复盘不再依赖日志反推。
3. 看板展示每笔交易的 Peak ROE、MFE/MAE、回吐幅度、实际平仓来源。

### P4：验证与校准

1. 用 #259-#289 加最近 100 个信号窗口做回放，对 P0/P1 阈值做离线对比。
2. 分模块统计胜率和收益质量，重点校准 `whale_flow_reversal` 与 `range_expansion_event`。
3. 增加两轮实时数据测试：观察优化后是否减少末端追单，并验证 GTC/IDOL 类浮盈回吐能否被锁住。

## 预期效果

P0 落地后，GTC/IDOL 这类“曾经明显盈利后变亏损”的情况应显著减少。P1 落地后，VELVET/UB 这类高评分但数据质量或末端追单风险较高的开仓会被降级。P2-P4 用于提升实盘收益质量和复盘可解释性。

## 2026-07-02 实施记录

### 已完成 P0

1. 保护器新增 ROE 利润地板：Peak ROE >=5% 后生成保本缓冲止损；Peak >=10% 锁定约 30% 峰值；Peak >=15% 锁定约 45% 峰值。
2. 新增硬亏损退出：入场前 15 分钟 ROE <= -8% 或任意时间 ROE <= -12% 触发 `hard_loss_close`。
3. 新增 giveback 强约束：Peak ROE >=5% 后若当前 ROE 回到 0 或以下，优先触发 `giveback_close`。
4. 平仓来源打标：保护器触发的 `tp0`、`tp1`、`tp2`、`trail_close`、`giveback_close`、`hard_loss_close` 会先写入 OPEN 仓位；交易同步关闭仓位时保留本地 `close_reason/source`。

### 已完成 P1/P2

1. `range_expansion_event` 空单若 24h 已跌超 12% 且仍带追单/确认不足标签，提示层降级为 WATCH，等待反抽失败或二次确认。
2. `whale_flow_reversal` 若执行数据不完整或缺少关键成交流字段，提示层降级为 WATCH，不再直接交给 AI 开仓。
3. 保护器新增 TP0：Peak/Current ROE >=8% 时先止盈 35%，剩余仓位重建保本止损。

### 已完成 P3/P4 基础项

1. 交易员提示词的持仓管理规则已从旧版 near-TP1/raw move 逻辑改为 ROE 峰值锁盈、TP0 和硬亏损规则。
2. 当前持仓摘要新增 `hard_loss`、`breakeven_floor`、`mid_profit_lock`、`high_profit_lock` 等保护状态提示。
3. 增加回归测试覆盖保护器 TP0、硬亏损、空单利润地板、平仓来源保留、range expansion 追空降级、whale flow 数据不足降级。

### 待验证

1. 用最新代码重启 AIT 后观察后续实盘周期，重点看 GTC/IDOL 类浮盈回吐是否能被 TP0/利润地板提前处理。
2. 下一轮实时测试继续统计 `range_expansion_event` 和 `whale_flow_reversal` 的 EXECUTABLE 数、实际开仓率、TP0 触发率和净胜率。
