# Hunter v7 实时入场信号改造计划

> 依据：`reports/hunter_v7_root_cause_map.svg` 六大根因诊断图  
> 目标：解决多轮实盘持续 `no_open_review_candidates watch=N rejected=0`，让几百个 Binance USDT 合约中稳定产出可复核、可入场候选，同时不牺牲后端风控与胜率。

## 0. 当前判断

持续 `watch=6 rejected=0` 的根因不是“全市场没有机会”，而是 Hunter v7 的机会漏斗过窄：

1. Universe 没有覆盖足够多的冷启动/大振幅/区间扩张标的。
2. Watch 信号缺少跨周期升级，雷达只会“看”，不会进入 `REVIEWABLE`。
3. Router 与 kernel 分层对 `EXECUTABLE/REVIEWABLE` 的 floor 太弱，导致 AI 被跳过。
4. OI/流动性门槛按大账户标准设计，小账户会误杀可交易标的。
5. LLM prompt 和后端风控已经在改，但数据源不给 open-review 候选时，LLM 没有机会判断。
6. 归因数据不足，无法量化每个标的漏在 universe/module/router/kernel/LLM/backend 哪一层。

原则：不能为了开仓率强行开垃圾单；必须把 `WATCH -> REVIEWABLE -> EXECUTABLE` 的升级条件做宽，但保留后端 `RR / drift / SL / max loss / min notional` 硬风控。

## 1. P0 立即改造

### 1.1 每轮 open-review floor

新增 Hunter v7 运行时兜底：当 router 输出全是 WATCH 且没有 REJECTED 风险时，必须从 WATCH 中救援 2-3 个 `REVIEWABLE`，而不是直接 `no_open_review_candidates`。

救援条件：

- `risk_score < 55`
- `liquidity_score >= 50`
- 方向 flow 不反向：LONG `taker_buy_15m >= 0.50`，SHORT `taker_buy_15m <= 0.50`
- entry zone 可触达：价格在 zone 内或距离 zone 不超过约 3%
- 后端几何可行：TP cap 下 RR 预估不低于 1.5，或可通过最小止损修复达到

输出标记：

- `reason_codes += reviewable_floor_rescue`
- `risk_tags += fallback_reviewable_needs_live_confirm`
- `execution_quality = near_confirm`
- `status = candidate`

### 1.2 用实时 market price 参与分层

在 prompt / kernel 分层前，将 `V7PriceContext.Last` 刷新为最新 compact market price。避免 v7 signal 旧价显示可开，但最新价已经跌出 entry zone 或 RR 不可行。

### 1.3 后端几何提前过滤

在 kernel tiering 中新增：

- `backend_rr_infeasible`
- `entry_zone_live_miss`
- `confirmation_live_miss`

目的：减少假 `EXECUTABLE`，把 AI 预算留给真正可开/可复核对象。

### 1.4 AI 输出限速

Hunter v7 决策输出限制为短审计 + JSON，避免 20 秒响应导致 momentum 标的执行价漂移。

## 2. P1 数据源改造

### 2.1 Universe 增加冷启动与大波动池

在 `hunter_v7_universe.go` 增加独立候选池：

- `amplitude_pool`: 24h 振幅、4h 振幅、1h 振幅排名
- `range_expansion_pool`: 1h/15m ATR 或 high-low range 相对过去中位数扩张
- `velocity_pool`: 5m/15m 连续变动、成交量突增、盘口价差稳定
- `new_activity_pool`: 原本低成交额但最近 15m/1h 成交额突增

每个池至少贡献若干 symbol，合并去重后再进入 setup modules。

### 2.2 小账户自适应流动性门

将固定 OI / quote volume 门槛改为按账户目标仓位自适应：

- 当前 HHH equity 约 5-6 USDT，实际最小名义仓位约 12 USDT。
- 对 12-50 USDT 名义仓位，不应使用百万 USDT 级 OI 门槛作为硬门。
- 以“可成交、滑点可控、非极端异常”为主，而不是大账户深度标准。

### 2.3 Watch 状态机升级

连续 N 轮 watch 时，根据强化条件升级：

- 连续 2-3 轮 entry zone 距离收敛
- taker flow 连续改善
- OI 从 flush 到 rebuild / 或拥挤减弱
- 15m close 接近 required confirmation
- 未创新低/新高，结构有效

升级为 `REVIEWABLE`，并记录 `watch_upgraded_reviewable`、`multi_cycle_confirmation`。

## 3. P2 模块和提示词校准

### 3.1 Setup 阈值按 regime 自适应

同一 setup 不应在 trend_down / squeeze / volatile_reversal / range 中共用同一 priority floor。

调整方向：

- `panic_reversal_long`: trend_down 中允许更多 `REVIEWABLE`，但必须要求 flow / no new low。
- `leader_momentum_long`: 只在 pullback 或 renewed breakout 放宽，避免追高。
- `funding_reversal`: OI flush/failed rebuild 权重提高，单纯 funding crowding 不开。
- `pre_breakout_watch`: 连续压缩 + volume/flow 变化时升级 reviewable。

### 3.2 LLM 决策从“保守 wait”改为“最佳候选裁决”

当有 `REVIEWABLE` 时，prompt 强制 LLM 做二选一：

- 满足 live zone / flow / RR / stop：输出 open。
- 不满足：输出唯一结构化 `blocked_reason_code`。

禁止使用“市场不确定、账户回撤、等待更多确认”作为全局否决。

## 4. P3 归因闭环

每轮持久化以下漏斗数据：

- universe 输入 symbol 总数与各 pool 命中数
- 每个 setup 输出数量与 status
- router 过滤原因
- kernel tier 与 tier_reason
- LLM blocked_reason_code
- backend 拒单原因
- 成交后 PnL / MFE / MAE

目标是每天能回答：

1. 今日 20%+ 振幅标的 Hunter v7 是否提前看到？
2. 看到后漏在哪层？
3. 哪些 `setup/tag/tier_reason` 真正盈利？
4. 哪些 rescue 候选提高开仓率但损害胜率？

## 5. 验收指标

短期 P0 验收：

- 连续 `no_open_review_candidates` 不超过 2 轮。
- 每轮至少有 1-3 个 `REVIEWABLE/EXECUTABLE`，除非全市场数据异常或已有持仓满仓。
- AI skipped 比例下降，LLM blocked_reason_code 覆盖率保持 95%+。
- backend drift failed / RR failed 不上升。

中期 P1/P2 验收：

- 20%+ daily mover 的 4h 提前召回率 > 60%。
- `REVIEWABLE -> LLM` 覆盖率 > 90%。
- `REVIEWABLE -> open` 成功执行率提升，但单笔最大亏损仍受控。
- 按 setup/tag 统计的真实胜率不低于当前基线。

## 6. 实施顺序

1. P0：open-review floor + 实时价格刷新 + backend geometry tiering。
2. P1：universe 增加 amplitude/range/velocity/new_activity pools。
3. P1：watch state upgrade 使用连续周期状态。
4. P2：setup/regime 阈值自适应。
5. P3：补全漏斗归因表和日报。

本轮先实现 P0，目标是立刻停止“几百个合约只输出 watch、AI 被跳过”的结构性问题。
