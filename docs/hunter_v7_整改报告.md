# Hunter v7 漏斗优化 — 整改实施报告

## 概述

本报告记录了 Hunter v7 信号引擎漏斗优化计划的**全部核心任务**的完整实施过程。
所有任务均已通过编译验证和单元测试，无回归。

**实施时间**: 2026-06-09
**涉及文件**: 20+ 个（含 signal records、mover audit、watch state、displacement setup、tag catalog 等新文件）
**编译状态**: ✅ `go build ./...` 通过
**测试状态**: ✅ `go test ./kernel/... ./provider/local/... ./store/...` 全部通过

---

## Phase 0 — 信号持久化与事实闭环基础设施

### Task 1.1: 新增 `hunter_v7_signal_records` 数据表与 Store ✅

**新建文件**: `store/hunter_v7_signal.go`

| 改动 | 说明 |
|------|------|
| `HunterV7SignalRecord` GORM 模型 | 包含 cycle_number, symbol, direction, setup_type, status, execution_quality, execution_tier, tier_reason, ai_priority, setup_score, timing_score, risk_score, liquidity_score, regime_fit_score, market_regime, reason_codes (JSON), risk_tags (JSON), entry_zone (JSON), oi_delta_1h, oi_delta_4h, funding_rate, taker_buy_15m, blocked_gate, raw_json 等全部字段 |
| `HunterV7SignalStore` | 实现 `CreateBatch(records []HunterV7SignalRecord)` 批量写入、`QueryByCycleNumber(cycle int)` 按周期查询、`QueryBySymbol(symbol string, limit int)` 按标的查询 |
| `store/store.go` | 在 `Store` struct 中添加 `hunterV7Signal` 子 store；在 `initTables()` 中添加 AutoMigrate |

### Task 1.2: 新增 `hunter_v7_mover_labels` 数据表 ✅

**新建文件**: `store/hunter_v7_mover.go`

| 改动 | 说明 |
|------|------|
| `HunterV7MoverLabel` GORM 模型 | 包含 trade_date, symbol, high_24h, low_24h, amplitude_24h, first_seen_at, first_watch_at, first_reviewable_at, first_executable_at, highest_tier, missed_stage 等字段 |
| `HunterV7MoverStore` | 实现 `Upsert(label HunterV7MoverLabel)` 和 `QueryByDate(date string)` |
| `store/store.go` | 注册并初始化 hunter_v7_mover_labels 表 |

### Task 1.3: 信号全量持久化 — ScoreHunterV7 钩子 + kernel 写入 ✅

**修改文件**: `provider/local/hunter_v7.go`, `provider/local/hunter_v7_types.go`, `kernel/engine.go`

| 改动 | 说明 |
|------|------|
| `V7SignalRecorder` 回调类型 | `type V7SignalRecorder func(cycleNumber int, signals []V7SignalOutput, regime V7MarketRegime)` |
| `V7Config.SignalRecorder` | 可选回调字段，provider 层不依赖 store |
| `ScoreHunterV7()` 末尾钩子 | 在返回前调用 `cfg.SignalRecorder(cycleNumber, signals, regime)` |
| `kernel/engine.go` 写入逻辑 | 调用 `ScoreHunterV7` 后，合并 V7 输出与 kernel tier 分类结果，计算 `blocked_gate`（标记信号被卡在哪一层：router_priority / kernel_tier_watch / execution_invalid_rr / execution_chase_risk 等），批量写入 `hunter_v7_signal_records` |

### Task 1.4: `cmd/hunter_v7_mover_audit` 每日大波动审计命令 ✅

**新建文件**: `cmd/hunter_v7_mover_audit/main.go`

| 功能 | 说明 |
|------|------|
| Binance Futures API 全市场 24h ticker | 拉取并筛选 amplitude_24h >= 20% / 30% / 50% 三档 |
| 反查信号记录 | 从 `hunter_v7_signal_records` 查询该标的是否出现过、首次出现时间、最高 tier |
| missed mover 报告 | 输出漏网标的及被卡层级（not_in_universe / module_no_match / router_filtered / tier_watch / llm_wait / backend_rejected） |
| 写入 mover_labels | 将审计结果写入 `hunter_v7_mover_labels` 供后续归因分析 |

### Task 1.5: LLM `blocked_reason_code` 结构化输出 ✅

**修改文件**: `kernel/engine_prompt.go`, `kernel/engine.go`

| 改动 | 说明 |
|------|------|
| `Decision.BlockedReasonCode` 字段 | 在 `Decision` struct 中新增 `BlockedReasonCode string \`json:"blocked_reason_code,omitempty"\`` |
| 策略 prompt（中英文双版本） | 在 hunter_v7 执行规则中要求 wait 决策必须输出 `blocked_reason_code`（枚举：entry_not_in_zone / rr_insufficient / confirmation_missing / oi_too_low / funding_crowded / account_risk / backend_guard_risk / no_reviewable_candidate） |
| 输出格式字段说明 | 在 hunter_v7 模式的 JSON 格式说明中新增 `blocked_reason_code` REQUIRED when action is `wait` 描述 |

---

## Phase 1 — 提升召回与升级能力

### Task 2.1: 新增 amplitude/range universe 入口 ✅

**修改文件**: `provider/local/hunter_v7_universe.go`, `provider/local/hunter_v7_types.go`

| 改动 | 说明 |
|------|------|
| `V7SymbolContext` 新增字段 | `Amplitude24h float64` — (High24h - Low24h) / Low24h × 100；`RangeExpansion1h float64` — 1h trueRange / median 20h trueRange |
| 大振幅池 | `amplitude_24h >= 12%` 的标的独立进入 universe，priority=200 |
| 区间扩张池 | `trueRange_1h / medianTrueRange_20h >= 2.2` 的标的独立进入 universe，priority=200 |
| `buildSymbolContext` 计算 | 新增 Amplitude24h 和 RangeExpansion1h 的计算逻辑 |
| 辅助函数 | 新增 `trueRange(bars []klineBar) float64` 和 `medianFloat64(vals []float64) float64` |

### Task 2.2: Watch 升级状态机 ✅

**新建文件**: `provider/local/hunter_v7_signal_state.go`
**修改文件**: `provider/local/hunter_v7.go`, `provider/local/hunter_v7_types.go`, `kernel/engine.go`

| 改动 | 说明 |
|------|------|
| 状态枚举 | `V7WatchSeen` → `V7WatchStrengthening` → `V7WatchNearConfirm` → `V7WatchReviewable` → `V7WatchExecutable` → `V7WatchExpired` / `V7WatchFailed` |
| `V7SignalStateManager` | 按 symbol+setup 维护跨周期 watch 状态，`Process()` 方法接收信号并返回升级后的信号 |
| 升级条件（4 种 watch setup） | pre_breakout_watch: 连续 2 轮 strengthening + OI building → near_confirm；pre_squeeze_watch: BB 收缩 + OI 稳定 → near_confirm；pre_distribution_watch: LSR 极端 + taker sell 增强 → near_confirm；accumulation_watch: 区间收窄 + OI 回升 → near_confirm |
| REVIEWABLE 升级 | 达到 REVIEWABLE 时自动将 `ExecutionQuality` 升级为 `V7ExecNearConfirm`，Confidence 升为 "B" |
| kernel 集成 | `StrategyEngine` 持有 `v7WatchState *V7SignalStateManager`，跨周期持久化 |

### Task 2.3: 新增 displacement/impulse 大波动模块 ✅

**新建文件**: `provider/local/hunter_v7_mod_displacement.go`
**修改文件**: `provider/local/hunter_v7_types.go`, `provider/local/hunter_v7_router.go`, `provider/local/hunter_v7_weights.go`, `kernel/engine.go`

| 改动 | 说明 |
|------|------|
| `V7SetupDisplacementLong` | 新增 setup 类型常量 `"displacement_momentum_long"` |
| Match 条件 | RangeExpansion1h >= 2.0, Change1h > 0, OIDelta1h >= 1, FundingRate < 0.001, TakerBuy15m >= 0.48 |
| Score 5 维度 | displacement magnitude (0-30), price momentum (0-20), 4h range break (0-20), OI confirmation (0-15), taker flow (0-15) |
| Anti-chase 护栏 | `displacementChaseCheck()`: 1h > 8% 且 price > VWAP + 2.5×ATR → chase_risk；RSI1h > 82 且 funding 极端 → watch_only |
| RR 验证 | `displacementRRValid()`: reward/risk >= 1.5 |
| Router 注册 | 在 `NewV7Router()` 中注册 `&displacementMomentumLongModule{}`（第 11 个模块） |
| Regime 权重 | trend_up=1.1, trend_down=0.6, range=0.7, panic_dump=0.3, pullback=0.5, mania_pump=1.2, compression=1.2, rotation=1.0, mixed=1.0 |
| kernel tier 分类 | EXECUTABLE: priority>=55, setup>=55, timing>=50, risk<55, taker>=0.50；REVIEWABLE: priority>=48, setup>=50, timing>=40, risk<55, liquidity>=50 |
| DefaultSetupThresholds | MinAIPriority=50 |

### Task 2.4: Regime strong-symbol override ✅

**修改文件**: `provider/local/hunter_v7_router.go`

| 改动 | 说明 |
|------|------|
| Strong-symbol 覆盖 | 在 regime weight < 0.8 且 symbol 非 BTC/ETH 时，若 symbolRS (= Change4h - BTC/ETH baseline) > 6% 且 LiquidityScore >= 50 且 TakerBuy15m >= 0.50，将 SetupScore 重算为 `SetupScore/weight*0.8`，RegimeFitScore 重算为 `0.8*67`，并添加 `strong_symbol_regime_override` reason code |
| `computeBTCETHBaseline4h()` | 从 universe 中提取 BTC/ETH 最大 4h 变化作为基准线 |

---

## Phase 2 — OI 自适应与风险标签精细化

### Task 3.1: OI 自适应阈值 ✅

**修改文件**: `kernel/engine_analysis.go`, `kernel/engine.go`, `provider/local/hunter_v7_types.go`, `provider/local/hunter_v7_router.go`

| 改动 | 说明 |
|------|------|
| `V7SignalOutput.QuoteVolume24h` | 新增字段，从 `ctx.Snapshot.QuoteVolume24h` 在 router Route 中传播 |
| `CandidateCoin.V7QuoteVolume24h` | 新增字段，构建 CandidateCoin 时从 V7SignalOutput 填充 |
| 自适应公式 | `adaptiveThreshold = max(accountMaxNotional/10, $500K floor, quoteVolume24h * 0.002)` |
| EXECUTABLE 阈值 | 完整自适应阈值，最低 1.0M |
| REVIEWABLE 阈值 | 自适应阈值 × 0.6，最低 0.5M |
| Watch/context 阈值 | 自适应阈值作为 cap，不高于默认值 |
| 账户维度 | BTC/ETH 使用 `BTCETHMaxPositionValueRatio`，山寨使用 `AltcoinMaxPositionValueRatio` |

### Task 3.2: 高风险信号分级处理 ✅

**修改文件**: `kernel/engine.go`

| 改动 | 说明 |
|------|------|
| `hunterV7ChaseRiskReviewableReason` 扩展 | 新增 `displacement_momentum_long` 支持（priority>=45, setup>=55, timing>=40）；新增通用规则：entry zone 可达 + taker 对齐 → REVIEWABLE |
| `hunterV7EntryZoneReachable()` | 新辅助函数：LONG 方向 price <= zone_upper*1.03，SHORT 方向 price >= zone_lower*0.97 |
| `hunterV7TakerBuyAligned()` | 新辅助函数：检查 taker flow 是否与方向一致 |
| `hunterV7HighRSIVolumeReviewable()` | 新函数：高 RSI + volume_expansion/oi_massive_flush/oi_heavy_flush + taker 对齐 → REVIEWABLE with `position_reduce` |
| 确认突破强制 REVIEWABLE | confirmed_breakout + taker_aggressive_buy + taker >= 0.52 + risk < 55 → REVIEWABLE（作为 fallback，在现有 reviewable 规则之后） |
| tier 分类流程调整 | 在 `hunterV7HighRiskSignalWaitReason` 返回 watch reason 时，先检查 `hunterV7HighRSIVolumeReviewable`，通过则 REVIEWABLE 而非 WATCH |

---

## Phase 3 — 策略提示词结构化改造

### Task 4: 策略提示词结构化改造 ✅

**修改文件**: `kernel/engine_prompt.go`, `kernel/engine_prompt_compact_test.go`

| 改动 | 说明 |
|------|------|
| Preflight 新增 `## Tier 漏斗决策原则` | 中英文双版本：严格按 EXECUTABLE → REVIEWABLE → WATCH(背景) → REJECTED(禁止) 顺序决策，不得因账户回撤/市场情绪跳过评估 |
| Preflight 新增 `## blocked_reason_code 强制要求` | 中英文双版本：wait 必须有且只有一个 blocked_reason_code，**绝对不得**用自然语言 reasoning 代替 |
| Preflight 新增 `## 账户回撤规则` | 中英文双版本：账户回撤**只用于**(1)仓位大小、(2)重复交易冷却、(3)同 symbol 冷却；**绝对禁止**作为全局 wait 否决 |
| Decision policy 升级 | 从单行文本升级为 4 条编号漏斗规则，每条明确 EXECUTABLE/REVIEWABLE/WATCH/REJECTED 的决策边界 |
| 测试适配 | 更新 `TestBuildUserPromptUsesHunterV7CandidateTiers` 断言以匹配新的结构化 decision policy 文本 |

### Task 4.1: 策略库五段提示词本地同步 ✅

**修改位置**: 本地 SQLite `data/data.db` 中的 `猎手v7` 策略记录

| 改动 | 说明 |
|------|------|
| 五段字段同步 | 已同步 `role_definition`、`trading_frequency`、`entry_standards`、`decision_process`、`custom_prompt` |
| 核心边界 | 强制按 EXECUTABLE / REVIEWABLE / WATCH / REJECTED 漏斗复核；WATCH 只作背景，不允许直接开仓 |
| 结构化 wait | wait 必须输出枚举化 `blocked_reason_code`，不得用自然语言 reasoning 替代 |
| 标签语义 | 明确遵守 `tag_semantics.llm_action`，`wait_only` 等待、`reject_only` 拒绝、`unknown_context_only` 仅作背景 |
| 本地备份 | `data/data.db.bak-hunter-v7-5prompt-20260609-0815` |

说明：`data/data.db` 属于本地运行库，已按 `.gitignore` 排除，不随 GitHub 推送。仓库侧通过 `kernel/engine_prompt.go` 和本报告保留可追溯规则。

---

## 文件清单

### 新建文件

| 文件路径 | 用途 |
|----------|------|
| `store/hunter_v7_signal.go` | hunter_v7_signal_records 表 GORM 模型与 Store |
| `store/hunter_v7_mover.go` | hunter_v7_mover_labels 表 GORM 模型与 Store |
| `provider/local/hunter_v7_signal_state.go` | Watch 信号跨周期升级状态机 |
| `provider/local/hunter_v7_mod_displacement.go` | displacement_momentum_long 信号模块 |
| `provider/local/hunter_v7_tag_catalog.go` | 标签语义 catalog 与 LLM 行为边界 |
| `provider/local/hunter_v7_tag_catalog_test.go` | 标签语义 catalog 测试 |
| `cmd/hunter_v7_mover_audit/main.go` | 每日大波动召回审计命令 |

### 修改文件

| 文件路径 | 改动摘要 |
|----------|----------|
| `store/store.go` | 注册两个新子 store + AutoMigrate |
| `kernel/engine.go` | Decision.BlockedReasonCode 字段、V7WatchStateManager 集成、CandidateCoin 新增 V7QuoteVolume24h、tier 分类扩展（chase_risk/general/highRSI/confirmed_breakout）、信号持久化写入 |
| `kernel/engine_analysis.go` | OI 自适应阈值公式 |
| `kernel/engine_prompt.go` | blocked_reason_code prompt（中英文）、tier 漏斗决策原则、账户回撤规则、decision policy 结构化 |
| `kernel/engine_prompt_compact_test.go` | 测试断言适配新 prompt 文本 |
| `provider/local/hunter_v7.go` | V7SignalRecorder 钩子、WatchStateManager 集成 |
| `provider/local/hunter_v7_types.go` | V7SetupDisplacementLong、V7SignalOutput.QuoteVolume24h、V7Config.WatchStateManager/SignalRecorder、Amplitude24h/RangeExpansion1h 字段、displacement 阈值 |
| `provider/local/hunter_v7_universe.go` | 大振幅池(>=12%)、区间扩张池(>=2.2x)、buildSymbolContext 新字段、trueRange/medianFloat64 辅助函数 |
| `provider/local/hunter_v7_router.go` | 注册 displacement 模块、strong-symbol override、computeBTCETHBaseline4h、QuoteVolume24h 传播 |
| `provider/local/hunter_v7_weights.go` | displacement_momentum_long 在 9 个 regime 的权重配置 |
| `cmd/hunter_v7_mover_audit/main.go` | 每日大波动召回审计命令 |
| `cmd/hunter_v7_validate/main.go` | 低并发、多轮间隔、安全轮测 Binance REST；报告输出 tier 分布 |
| `datafetch/detail_selector.go` | 为高振幅标的预留 detail 拉取配额 |
| `provider/local/hunter_v7_tag_catalog.go` | 统一关键标签定义、分类和 LLM action |

---

## 验证结果

```
$ go build ./...
# 编译通过，无错误

$ go test ./provider/local/... -count=1 -timeout 60s
ok      github.com/Aixxww/AiT/provider/local    1.394s

$ go test ./kernel/... -count=1 -timeout 60s
ok      github.com/Aixxww/AiT/kernel    4.008s

$ go test ./store/... -count=1 -timeout 60s
ok      github.com/Aixxww/AiT/store     0.977s
```

---

## 关键设计决策

| 决策 | 理由 |
|------|------|
| Provider-Store 解耦（回调注入） | provider 层不直接依赖 store 层，通过 `V7SignalRecorder` 回调实现持久化，保持架构清晰 |
| Watch 状态管理器挂在 StrategyEngine | 跨周期持久化需要持有引用，通过 V7Config 传递给 ScoreHunterV7 |
| Strong-symbol override 仅在 weight < 0.8 时激活 | 防止低质量标的在弱 regime 下误触发，需 liquidity >= 50 且 taker 对齐 |
| OI 自适应使用 max 而非 sum | 确保任何单一维度（账户/标的/成交量）都不低于最低安全水位 |
| confirmed_breakout force-reviewable 放在现有规则之后 | 作为 fallback 兜底，不覆盖已有的更精确 reviewable 规则 |
| 提示词使用 ## 二级标题分隔规则 | 结构化章节让 LLM 更容易定位和遵循每条规则 |
