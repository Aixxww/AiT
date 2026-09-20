# Hunter v7 信号看板专业化分级实施方案

## 目标与边界

目标是让用户能够复盘“本轮如何筛选、为何准入/拦截、等待什么确认、TP/SL 结果是否可信”，而不是把高分或 `EXECUTABLE` 误读为保证盈利或自动下单。

本方案仅提升观察、审验与归因能力；不修改策略阈值、不触发实盘下单。开仓率、胜率和期望值只能在数据完整、定义一致、样本充分时评估，不能由 UI 保证。

## 当前基线

现有 `SignalPanel` 已按 `EXECUTABLE / REVIEWABLE / WATCH / REJECTED` 展示最新持久化轮次，包含方向、形态、四维评分、入场区间、失效价、目标、确认与风险标签、部分跟踪结果。

缺口是：全市场到信号的漏斗不可见、数据质量不可见、路由证据不完整、WATCH 缺少下一步、结果统计未充分区分歧义/ACTIVE/部分止盈，且尚无结构监控和形态×regime 归因。

## P0：本轮实施——可信可读的信号审验

### 范围

1. 增加“最新轮次信号漏斗”：`已路由 → 开仓复核 → 等待确认 → 已否决`。
   - 仅基于已持久化的最新轮次记录；UI 明确标注，不假称全市场覆盖。
2. 增加轮次时间与 `execution_readiness.data_quality` 状态。
3. 在 EXECUTABLE/REVIEWABLE 卡片增加“路由证据”：价格变化、OI 1h/4h 变化、资金费率、LSR、taker 比及 reason codes。
4. WATCH 增加首个 `required_confirmation`，让用户知道下一步等什么。
5. 增加 TP0/TP1/TP2、剩余 runner、已实现 PnL 的退出进度。
6. `ACTIVE` 与 `BOTH_SAME_1M` 不再计入看板平均 PnL；歧义数单独呈现。

### 配色与视觉规范

- 使用现有设计 token，避免以红绿表达信号质量：
  - `EXECUTABLE`：低饱和金色边线与数字，表示“允许审验”；
  - `REVIEWABLE`：低饱和青色，表示“需要确认”；
  - `WATCH`：中性石墨灰，表示“观察”；
  - `REJECTED`：低对比灰，表示“不可进入”。
- 红绿仅表达 LONG/SHORT 或已完成 PnL，且不用于 tier。
- 采用深色半透明面板、细边框、等宽数字和短标签；不使用大面积高饱和渐变、闪烁或夸张阴影。

### 验收

- `BOTH_SAME_1M` 出现时能在界面看到，且不影响平均 PnL。
- 每张可审阅卡至少展示一条路由证据或明确空状态。
- WATCH 至少有一条确认条件时，界面显示下一步。
- 无 API 数据时维持已有离线/空状态，不出现伪造质量结论。

## P1：后端周期元数据与完整执行审计

### 新增数据契约

为每个 Hunter v7 cycle 单独持久化并提供 `/api/hunter/v7/cycles/latest`：

```json
{
  "cycle_id": "...",
  "snapshot_at": "...",
  "symbol_count": 524,
  "detail_requested": 220,
  "detail_success": 216,
  "rest_errors": 2,
  "routeable_universe": 141,
  "raw_signals": 38,
  "tier_counts": {"EXECUTABLE": 1, "REVIEWABLE": 6, "WATCH": 22, "REJECTED": 9},
  "top_vetoes": [{"code": "late_chase", "count": 5}]
}
```

这解决 P0 漏斗只能表达“已持久化信号”的限制，允许真实展示：全市场 → 详情拉取成功 → 可路由 → 模块命中 → tier → 执行许可。

### Outcome 口径

- `WIN_TP0/1/2`、`PROTECTED_STOP`、`STOP`、`TIMEOUT`、`ACTIVE`、`BOTH_SAME_1M`、`INCOMPLETE_DATA` 分开统计。
- `BOTH_SAME_1M` 与 `INCOMPLETE_DATA` 从胜率、profit factor、期望值分母排除。
- 单列 `TP0 hit rate`、`runner TP1+ rate`、`protected exit rate`、`MFE/MAE`，不得用一个平均 PnL 取代。

### 验收

- 看板能显示快照年龄、detail 成功率、REST 错误和 routeable universe。
- 后端测试验证歧义、断网回填、部分止盈的统计分母。
- 任何 cycle 数据质量为 `STALE/PARTIAL` 时，执行许可区显示明确警告。

## P2：结构监控与形态归因

### 结构监控（独立于开仓信号）

新增“合约结构监控”表，展示：OI、OI 1h/4h/3d、价格 1h/24h/3d、资金费率、LSR、taker、成交额、可选的 MC/OI-MC。

`OI/MC` 只能是风险/拥挤度指标，不能单独决定多空或开仓。若接入市值数据必须同时展示 `source`、`updated_at`、`fallback`；第三方限流或数据陈旧时降级为不可用，不能静默填零。

### 形态×Regime 矩阵

基于既有 `hunter_v7_signal_records` 与 matrix/outcome 接口，增加：

- setup × regime × tier：信号数、可审阅率、拒绝主因；
- 仅无歧义 completed outcome：TP0、TP1+、STOP、MFE、MAE、期望值；
- 样本量门槛：少于 30 笔仅观察，30–59 影子建议，60+ 才允许受控策略调整。

## 实施顺序

1. P0 前端可读性与统计口径（本次）。
2. P1 cycle metadata、tracking gap、完整 outcome 分母。
3. P2 结构监控、形态矩阵、跨周期对照。
4. 每阶段仅在通过单元测试、接口契约测试及只读 Binance 验证后进入下一阶段。

## 文件范围

- P0：`web/src/components/hunter/SignalPanel.tsx`、`web/src/lib/api/hunter.ts`。
- P1：`store/hunter_v7_signal.go`、`api/handler_hunter.go`、验证器/Outcome tracker。
- P2：新增结构监控 API 与前端模块；不把第三方市值源耦合到 Hunter v7 下单链路。

## 2026-08-07 实施记录

已完成：

- P0 全部前端可视化项：持久化轮次漏斗、数据质量提示、路由证据、WATCH 下一步确认、TP 分段进度、歧义结果隔离。
- P1 的持久化轮次摘要：信号接口返回最新 cycle 的记录数、tier/status/data-quality 计数和主要 veto；界面使用该摘要而不是以分页结果推断否决数量。
- P1 的统计口径修正：`PROTECTED_STOP` 单列为保护退出，不再被统计为完整 `WIN_TP*`；`BOTH_SAME_1M` 仍排除在校准样本之外。
- P2 的 Binance-native 结构监控：在同一轮信号快照中展示价格 1h 变化、OI、OI 1h 变化、funding、LSR、taker，明确标为辅助证据而非单独开仓依据。

未伪造的数据：全市场 `symbol_count/detail_success/REST` 元数据、真实市值/OI-MC、3d OI/价格变化仍需要独立 cycle recorder 和具有 freshness/fallback 标识的市值源。它们不会由前端用缺失字段推算或填零。
