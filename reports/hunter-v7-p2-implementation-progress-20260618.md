# Hunter v7/v8 P2 实施进展记录

> 日期：2026-06-18 04:18:40 CST  
> 接管任务：检验 P0/P1，继续实施 P2  
> 当前结论：P0/P1 核心链路经测试验证通过；P2 已补齐「主题相关性输出过滤」「regime×setup matrix API」「板块轮动保守增强」三项可闭环能力。

## 一、P0/P1 验证结论

已执行并通过：

```bash
go test ./api ./datafetch ./provider/local ./store ./trader ./kernel
go test ./...
```

验证覆盖的关键链路：

- PnL tracker、1m K 线 high/low 判定、历史回放、REST backfill、active outcome 写入降频。
- AutoTrader 信号 recorder、EXECUTABLE/REVIEWABLE 注册跟踪、outcome 写回。
- Store outcome 聚合与 `/api/hunter/v7/outcomes` dry-run 调权报表。
- TimingBooster 后置重算 ExecutionQuality/ExecutionReadiness。
- Funding Fast-Track 实际放宽 zone gate。
- final risk/liquidity hard filter 后 readiness 同步。
- 动态保护止损保守改单逻辑。

## 二、本轮 P2 已完成

### P2-1 主题相关性输出过滤

文件：

- `provider/local/hunter_v7_correlation.go`
- `provider/local/hunter_v7_router.go`
- `provider/local/hunter_v7_types.go`
- `provider/local/hunter_v7_router_test.go`

实现：

- 在 `RouteDetailed()` 中将 `CorrelationFilter` 接入 confirmed/output 侧。
- 不过滤 `RawSignals`，保留全量原始信号用于漏斗归因、PnL 学习和 module no-match 统计。
- 新增配置 `V7Config.CorrelationMaxPerTheme`，默认每个主题最多 3 个信号。
- `CorrelationMaxPerTheme < 0` 可关闭相关性过滤。
- 若过滤后低于 `MinOutput`，按优先级补回信号并加 `correlation_floor_context` 标签，避免输出为空或过薄。

新增测试：

- `TestApplyV7CorrelationFilterCapsThemeButPreservesMinOutput`
- `TestApplyV7CorrelationFilterCanBackfillToMinOutput`

### P2-2 Regime×Setup Matrix API

文件：

- `provider/local/hunter_v7_matrix_report.go`
- `store/hunter_v7_signal.go`
- `api/handler_hunter.go`
- `api/server.go`
- `api/handler_hunter_test.go`

新增接口：

```http
GET /api/hunter/v7/matrix?days=7&regime=
```

响应能力：

- 从 `hunter_v7_signal_records` 读取指定窗口内所有持久化信号。
- 输出 per regime × setup 的：
  - `signal_count`
  - `exec_count`
  - `avg_priority`
  - `avg_setup_score`
  - `avg_timing_score`
- 支持 `regime` 查询参数过滤单一市场状态。
- matrix cells 按 regime/setup 稳定排序，便于前端展示和测试复现。

新增测试：

- `TestHandleHunterV7Matrix`

### P2-3 板块轮动保守增强

文件：

- `provider/local/hunter_v7_sector_rotation.go`
- `provider/local/hunter_v7_router.go`
- `provider/local/hunter_v7_router_test.go`

实现：

- 新增 `SectorRotationAnalyzer`，基于当前 futures universe 的本地主题分类统计：
  - AI
  - L1/L2
  - DeFi
  - Meme
  - Gaming
  - RWA
  - Storage
  - Exchange
- 仅在 `V7RegimeRotation` 下启用。
- 当某主题至少 2 个标的同步跑赢 universe 均值时，将该主题标记为 leader。
- 对 leader theme 的 LONG 信号做小幅增强：
  - `SetupScore +4`
  - `TimingScore +2`
  - reason codes 增加 `sector_rotation_leader` 与 `sector_theme_xxx`
- 不改变硬过滤、不直接把弱信号提升为可执行，避免在样本不足时过度拟合。

新增测试：

- `TestSectorRotationEnhancesLeaderThemeInRotationRegime`
- `TestSectorRotationDoesNotEnhanceOutsideRotationRegime`

### P2-4 OI 隐形建仓统一特征

文件：

- `provider/local/hunter_v7_oi_accumulation.go`
- `provider/local/hunter_v7_mod_accumulation.go`
- `provider/local/hunter_v7_mod_squeeze_breakout.go`
- `provider/local/hunter_v7_mod_whale_flow.go`
- `provider/local/hunter_v7_tag_catalog.go`
- `provider/local/hunter_v7_oi_accumulation_test.go`

实现：

- 新增 `AssessV7OIAccumulation(ctx)`，统一评估：
  - 1h OI confirming
  - 4h stealth build
  - OI-price divergence
  - OI build without price markup
  - funding not crowded
  - BB compression
  - breakout volume burst
  - taker buy ratio > 0.55
  - balanced LSR accumulation
- 新增 `ApplyV7OIAccumulationEvidence(sig, ctx)`，给信号统一写入 reason codes：
  - `oi_invisible_accumulation_detected`
  - `oi_4h_stealth_build`
  - `oi_1h_confirming_accumulation`
  - `oi_build_without_price_markup`
  - `funding_not_crowded`
  - `bb_compressed`
  - `volume_burst_at_breakout`
  - `taker_buy_ratio_above_0.55`
  - `lsr_balanced_accumulation`
- 统一接入 accumulation、volatility squeeze breakout、whale flow 三个模块。
- 对 invisible accumulation 只做最多 6 分的小幅 `SetupScore` 增强，不绕过执行门槛。
- 修复此前共振评分需要的四件套标签分散在不同模块、难以稳定触发的问题。

新增测试：

- `TestAssessV7OIAccumulationDetectsInvisibleBuild`
- `TestAssessV7OIAccumulationDoesNotFlagCrowdedMarkup`
- `TestVolatilitySqueezeAppliesUnifiedOIResonanceTags`
- `TestWhaleFlowUsesUnifiedOIAccumulationTags`

### P2-5 Prompt v8 标签语义验收

文件：

- `provider/local/hunter_v7_tag_catalog.go`
- `kernel/engine_prompt_compact_test.go`

实现：

- 为 P2 新标签补充 tag catalog 定义，避免 prompt JSON 中显示 unknown/context-only：
  - `oi_invisible_accumulation_detected`
  - `oi_4h_stealth_build`
  - `oi_1h_confirming_accumulation`
  - `oi_build_without_price_markup`
  - `funding_not_crowded`
  - `bb_compressed`
  - `volume_burst_at_breakout`
  - `taker_buy_ratio_above_0.55`
  - `lsr_balanced_accumulation`
  - `sector_rotation_leader`
  - `correlation_floor_context`
- 新增测试确认 `formatHunterV7SignalJSON()` 保留：
  - `tag_semantics`
  - `execution_readiness`
  - `targets`
  - P2 标签的 `llm_action`

新增测试：

- `TestFormatHunterV7SignalJSONDefinesP2Tags`

## 三、本轮顺手修正/确认

- `MatrixReport` 增加稳定排序。
- `RouteDetailed()` P2 输出过滤放在 `filterV7SignalsForLLM()` 之后、`BuildV7PreMoveRadar()` 之前，确保 watch radar 不挤占 confirmed 多样性逻辑。
- `RawSignals` 仍追加 watches 和 module no-match，未被 correlation filter 影响。
- Prompt compact 路径已确认保留 `reason_codes / risk_tags / tag_semantics / execution_readiness / targets`，P2 新标签有显式语义。

## 四、P2 剩余任务拆解

| 任务 | 当前状态 | 下一步 |
|---|---:|---|
| OI 隐形建仓增强 | 已完成统一特征 MVP | 后续可加入 per-symbol OI 分位/斜率历史，但需要更长窗口缓存 |
| RegimeAdaptive 自动调权 | dry-run 已完成 | 需要 3-7 天 outcome 样本后再接每日定时应用；当前不建议直接改线上权重 |
| ML 辅助评分 | 未实施 | 依赖至少 2 周带 outcome 的训练样本；先导出特征表比写假模型更合理 |
| 盘口深度分析 | 未实施 | 依赖 WebSocket/orderbook 数据源；当前仓库无稳定盘口快照输入 |
| prompt v8 压缩 | P2 标签语义和核心字段已验收 | 后续可做 token 预算量化与字段裁剪 |
| 全链路自动闭环 | 未实施 | 依赖 RegimeAdaptive 从 dry-run 升级、风险开关、回滚策略 |

## 五、当前风险

1. `SectorRotationAnalyzer` 使用静态符号主题映射，覆盖范围有限；新币或跨主题币会落入 `other`，不会被增强。
2. 相关性过滤只控制输出拥挤，不代表真实相关系数；若后续有 K 线收益率矩阵，可替换为统计相关性。
3. P2 的高胜率目标仍依赖 P1 outcome 样本验证；当前所有新增逻辑均保持保守，不承诺未验证胜率。
4. 工作树仍有大量既有 tracked/untracked Hunter v7/v8 文件，本轮未回滚任何既有改动。

## 六、后续建议顺序

1. 增加 `/api/hunter/v7/matrix` 前端/运维展示入口，和 `/outcomes` 放在同一诊断面板。
2. 做 prompt token 预算量化：记录 compact prompt token/字符数，确认最大 8 个 open-review 候选下仍可控。
3. 累积实盘 outcome 后再把 RegimeAdaptive 从 dry-run 升级到可控自动调权。
4. 若接入 orderbook WebSocket，再实施盘口深度分析；当前不建议写无数据源的假盘口评分。

## 七、2026-06-18 继续实施记录

本轮新增并验证：

```bash
go test ./provider/local
go test ./kernel
go test ./api ./datafetch ./provider/local ./store ./trader ./kernel
```

结果：全部通过。

## 八、2026-06-18 21:17 修复与实时复验记录

针对 `reports/hunter-v7-live-validation-analysis-20260618-043507.md` 暴露的三项问题完成修复：

1. Prompt tier 与 router readiness 一致性
   - `chase_risk` 不再通过 `hunterV7ChaseRiskReviewableReason()` 或 leader momentum flexible review 通道提升为 `REVIEWABLE`。
   - `leader_momentum_long` 含 `momentum_confirmation_missing / momentum_overheated / momentum_chase_risk / momentum_rsi_overheated_wait` 时保持 `WATCH`，等待回踩或重新确认。

2. Intraday scalp 执行几何
   - `intraday_scalp_long` 在当前全局后端几何（SL >= 2%、RR 约束）下强制输出为 `watch_only / wait_confirm`。
   - 新增 `scalp_backend_geometry_context` 与 `scalp_global_geometry_incompatible`，避免 raw executable 与后端 RR 不可行之间反复冲突。

3. Prompt 标签语义
   - 补齐 live 中出现的 momentum 标签：`accelerating_1h`、`taker_neutral_buy`、`no_pullback_still_running`、`chase_high_protection`、`low_timing_watch_only`、`leader_momentum_timing_watch_only`、`momentum_rsi_overheated_wait`。
   - 补齐本轮 whale-flow live 触发的 `whale_flow_detected`、`stealth_accumulation_breakout`，降低 `unknown_context_only` 对核心信号解释的影响。

新增/调整测试：

```bash
go test ./provider/local ./kernel
go test ./api ./datafetch ./provider/local ./store ./trader ./kernel
go test ./...
```

结果：全部通过。

实时复验命令：

```bash
go run ./cmd/hunter_v7_validate --rounds 1 --top-detail 220 --max-workers 8 --max-output 40 --watch-output 5 --min-priority 45 --aggressive=true --out-dir reports
```

实时复验输出：

- `reports/hunter-v7-live-validation-raw-20260618-211529.json`
- `reports/hunter-v7-live-validation-report-20260618-211529.md`
- `reports/hunter-v7-live-prompt-20260618-211529.txt`

复验结论：

- Binance Futures REST 抓取成功，`rest_errors=0`。
- 本轮输出 7 个信号，tier 分布为 `EXECUTABLE=1 / WATCH=5 / REJECTED=1`，`issues=0`。
- `RIFUSDT intraday_scalp_long` 已按预期变为 `status=wait_confirm / execution_quality=watch_only`，并携带 `scalp_backend_geometry_context`、`scalp_global_geometry_incompatible`。
- 本轮未再出现上一轮 `USUSDT` 类 `chase_risk` 动量信号被 prompt tier 抬升为 `REVIEWABLE` 的问题。
