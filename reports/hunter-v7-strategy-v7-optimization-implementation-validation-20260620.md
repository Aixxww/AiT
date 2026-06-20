# Hunter v7 / Strategy v7 优化实施与实时验证报告

> 生成时间：2026-06-20 11:20 CST  
> 范围：修复 2026-06-19 两轮实时验证暴露的执行分层、Prompt 识别、冲突观察、几何约束问题  
> 最新实时验证：`reports/hunter-v7-live-validation-report-20260620-110225.md`

## 1. 本轮结论

- P0/P1 遗留问题已通过单测、全量 Go 测试和一轮币安实时数据验证。
- 最新验证 `issues=0`，`missing_field_count=0`，`executable_gap_count=0`。
- Prompt 最终分层已和 `execution_readiness` 对齐，不再出现 `EXECUTABLE` 同时缺少 5m/15m K 线执行数据的矛盾。
- 验证器现在同时展示 `runtime tier` 与 `prompt-final tier`，避免把后端初筛可执行误解为 AIT 最终可开仓。
- 本轮实时行情下最终直开开仓率为 `0/9 = 0%`，可人工复核开仓率为 `1/9 = 11.1%`。这是严格执行门控后的结果，不应强行放大为可开仓信号。

## 2. 已修复问题

| 问题 | 原现象 | 修复状态 | 代码位置 |
|---|---|---|---|
| Prompt 最终 `EXECUTABLE` 与 readiness 缺执行 K 线矛盾 | DEXE/EVAA/FOLKS 等缺 5m/15m kline 仍可能被展示为可执行 | 已修复 | `kernel/engine.go`, `kernel/engine_prompt.go` |
| `conflict_watch` 被误计为执行性缺口 | BASEDUSDT 缺 invalidation/targets 被计入 `missing_invalidation`/`missing_targets` | 已修复 | `cmd/hunter_v7_validate/main.go` |
| 压缩爆发类被全局 RR 几何误杀 | REUSDT raw-ready，但 Prompt 被 `backend_rr_infeasible` 阻断，后续触达 TP0/TP1 | 已修复 | `kernel/engine.go` |
| `wait_only` 标签直开风险 | `no_pullback_still_running` 等标签可能没有阻断直接 EXECUTABLE | 已修复 | `kernel/engine.go` |
| 验证报告 tier 口径误导 | console/markdown 只展示 runtime tier，和 Prompt Tier Summary 不一致 | 已修复 | `cmd/hunter_v7_validate/main.go` |

## 3. 实施细节

### 3.1 缺执行数据降级

- 新增 `hunterV7ReadinessMissingExecutionWaitReason`。
- 当 `V7Readiness.MissingExecution` 非空时，运行时分层直接降为 `WATCH`。
- Prompt 构建阶段再次基于 `hunterV7PromptExecutionReadiness` 同步最终分层：
  - `Rejected` -> `REJECTED`
  - `Watch` -> `WATCH`
  - `Reviewable` 可将 `EXECUTABLE` 降为 `REVIEWABLE`

### 3.2 wait-only 只阻断直开

- 新增 `hunterV7DirectOpenWaitOnlyReason`。
- 当前阻断直开但保留可复核路径的标签：
  - reason code：`no_pullback_still_running`, `chase_high_protection`, `momentum_rsi_overheated_wait`
  - risk tag：`momentum_confirmation_missing`, `momentum_overheated`, `momentum_chase_risk`, `do_not_market_chase`

### 3.3 setup-specific 几何约束

- `volatility_squeeze_breakout` / `displacement_momentum_long`：
  - `MinRR <= 1.35`
  - `MaxTPMovePct >= 12`
- `intraday_scalp_long`：
  - `MinRR <= 1.0`
  - `MaxTPMovePct >= 3`
- 目的：避免用普通趋势模块的全局 RR 上限误杀 squeeze/displacement 的短时弹性机会。

### 3.4 验证器分层口径

- `validatePrompt` 现在解析 Prompt 中的 `Tier Summary`。
- 报告同时输出：
  - `runtime tier 分布（后端初筛）`
  - `prompt-final tier 分布（AIT 最终可执行口径）`

## 4. 测试结果

| 命令 | 结果 |
|---|---|
| `go test ./kernel ./cmd/hunter_v7_validate` | 通过 |
| `go test ./api ./datafetch ./provider/local ./store ./trader ./kernel ./cmd/hunter_v7_validate` | 通过 |
| `go test ./...` | 通过 |

新增/更新测试覆盖：

- `TestClassifyHunterV7CandidateTierDemotesMissingExecutionReadiness`
- `TestClassifyHunterV7CandidateTierBlocksWaitOnlyReasonCodes`
- `TestClassifyHunterV7CandidateTierKeepsSqueezeFeasibleWithExtendedTargets`
- `TestBuildUserPromptDemotesExecutableWhenPromptReadinessMissingExecution`
- `TestValidateFormatSkipsExecutionGeometryForConflictWatch`
- `TestValidatePromptParsesFinalTierSummary`

## 5. 最新实时验证

命令：

```bash
go run ./cmd/hunter_v7_validate --rounds 1 --top-detail 220 --max-workers 8 --max-output 40 --watch-output 5 --min-priority 45 --aggressive=true --out-dir reports
```

输出文件：

- 原始 JSON：`reports/hunter-v7-live-validation-raw-20260620-110225.json`
- Markdown 报告：`reports/hunter-v7-live-validation-report-20260620-110225.md`
- Prompt 预览：`reports/hunter-v7-live-prompt-20260620-110225.txt`

实时数据摘要：

| 指标 | 值 |
|---|---:|
| Binance symbols | 522 |
| Hunter universe | 216 |
| REST errors | 0 |
| Market regime | rotation |
| BTC 24h | +0.49% |
| ETH 24h | -0.15% |
| 输出信号 | 9 |
| LONG / SHORT | 6 / 3 |
| 格式/识别 issues | 0 |

分层结果：

| 口径 | 分布 | 含义 |
|---|---|---|
| runtime tier | `EXECUTABLE=1, REJECTED=2, WATCH=6` | 后端初筛，尚未叠加 Prompt 级执行数据完整性 |
| prompt-final tier | `EXECUTABLE=0, REVIEWABLE=1, WATCH=6, REJECTED=2` | AIT 最终可执行口径 |

Prompt 核对：

- `Tier Summary: EXECUTABLE=0 | REVIEWABLE=1 | WATCH=6 | REJECTED=2`
- `EIGENUSDT` 被降为 `REVIEWABLE`，原因：`prompt_readiness_15m_kline_missing`
- 同一标的的 `missing_execution=["15m_kline","5m_kline"]` 已保留在 JSON 中，AI 可以明确知道不能直接市价追单。

## 6. 开仓率、胜率与盈利率判断

本轮实时验证是修复后的一轮即时数据校验，不包含 30 分钟或 2 小时后的完整 TP/SL 跟踪窗口，因此不能宣称真实胜率或盈利率。

当前可确认指标：

| 指标 | 本轮值 | 判断 |
|---|---:|---|
| Prompt-final 直开率 | 0/9 = 0% | 当前行情下无完全满足执行门控标的 |
| Prompt-final 复核开仓候选率 | 1/9 = 11.1% | EIGENUSDT 需补齐 5m/15m 执行确认 |
| 格式阻断率 | 0/9 = 0% | JSON/Prompt 链路已打通 |
| 误直开率 | 0/9 = 0% | 缺执行数据不再进入 EXECUTABLE |

专业判断：

- 系统现在更适合“宁可少开，不误开”的执行标准。
- 这会短期降低开仓率，但会显著降低 Prompt 因缺 K 线、缺确认、追高标签而误判直开的概率。
- 要验证 80% 开仓率/胜率盈利率，必须依赖已规划的 1m K 线生命周期跟踪、TP0/TP1/TP2 分层命中统计和至少 20-50 轮样本。当前一轮实时验证只能证明链路正确，不能证明长期胜率。

## 7. 下一步优化建议

1. 将 `prompt-final tier` 作为后续开仓率统计的唯一分母口径，避免 runtime 初筛和 AIT 最终执行混用。
2. 对 REVIEWABLE 候选增加自动补数机制：缺 5m/15m kline 时尝试二次拉取，而不是直接等待下一轮。
3. 继续推进 P2 的 1m 跟踪闭环，把每个 prompt-final 候选记录为 `NO_OPEN` / `OPEN_TP0` / `OPEN_TP1` / `SL` / `TIMEOUT`。
4. 对 squeeze/displacement 新几何规则继续用 20 轮以上样本做 MFE/MAE 复核，避免过度放宽后产生追高假突破。
