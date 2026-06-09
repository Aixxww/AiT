# Hunter v7 架构与标签语义治理方案

> 日期：2026-06-09
> 目标：用最小代码量降低 Hunter v7 的复杂度和 LLM 歧义，不做大拆大改，不重写策略模块。

## 1. 当前架构分层

Hunter v7 当前可稳定划分为 6 层：

| 层级 | 主要文件 | 职责 | 不应承担的职责 |
|---|---|---|---|
| 数据快照 | `datafetch/*` | 拉取 Binance 快照、tickers、OI、K 线 | 不做交易 setup 判断 |
| Universe | `provider/local/hunter_v7_universe.go` | 构建候选标的上下文、统一 OI notional、计算 amplitude/range expansion | 不决定是否开仓 |
| Setup modules | `provider/local/hunter_v7_mod_*.go` | 每个形态只负责 Match/Score，并输出 reason/risk/confirm 标签 | 不写 LLM prompt 规则 |
| Router | `provider/local/hunter_v7_router.go` / `hunter_v7_execution.go` | regime 权重、执行质量、watch/reviewable 输出、fallback | 不替代后端硬风控 |
| Kernel tier | `kernel/engine.go` | EXECUTABLE / REVIEWABLE / WATCH / REJECTED 分层 | 不生成市场原始信号 |
| Prompt/Execution | `kernel/engine_prompt.go` / `trader/*` | 将候选给 LLM，校验 open 决策，真实执行 | 不重新发明 setup 规则 |

关键原则：`provider/local` 负责“发现信号”，`kernel` 负责“给 LLM 分层解释”，`trader` 负责“执行与硬风控”。

## 2. 当前主要代码风险

1. 标签是裸字符串，分散在 setup、router、kernel、prompt 中。
2. 同类标签存在轻微命名差异，例如 `taker_buy_strong`、`taker_strong_buy`、`taker_sustained_buy`。
3. LLM 看到 `reason_codes` 和 `risk_tags` 时无法稳定区分：
   - 只是背景证据；
   - 必须等待确认；
   - 是否硬禁止开仓；
   - 是否允许进入 REVIEWABLE 复核。
4. 多个 helper 函数用字符串做业务判断，缺少中心定义，后续新增标签容易被 LLM 或 tier 逻辑误读。

## 3. 最小治理方案

本轮不重写所有字符串常量，避免大面积 churn。采用更稳的最小方案：

1. 新增 `provider/local/hunter_v7_tag_catalog.go`。
2. 对关键标签定义统一语义：
   - `source`: `reason_code` / `risk_tag` / `required_confirmation`
   - `category`: `price` / `flow` / `oi` / `funding` / `risk` / `tier` / `state`
   - `llm_action`: LLM 应采取的动作边界
   - `definition`: 简短定义
3. `kernel/engine_prompt.go` 在每个 `hunter_v7_signal_json` 中输出 `tag_semantics`。
4. 未定义标签默认输出 `unknown_context_only`，明确不能作为开仓许可。

这样做的好处：

- 不需要改动所有 setup 模块；
- 不影响现有策略输出；
- LLM 能拿到同一套语义定义；
- 后续可以逐步把高频标签替换成常量，而不是一次性大重构。

## 4. 标签动作语义

| `llm_action` | 含义 | LLM 行为 |
|---|---|---|
| `supports_open_after_core_checks` | 支持开仓的正向证据 | 仍需确认 entry zone、stop、RR、后端风控 |
| `evidence_only` | 背景证据 | 不能单独作为 open 理由 |
| `required_confirmation` | 必须满足的确认条件 | 未满足则 wait |
| `reviewable_only_if_live_confirmed` | 可进入复核但不可自动开 | 只有实时 K 线/资金流/RR 都满足才 open |
| `wait_only` | 等待/禁止直接开仓 | 输出 wait，除非后续周期升级 |
| `reject_only` | 硬拒绝 | 不参与开仓判断 |
| `context_only` | 背景上下文 | 不能影响开仓方向 |
| `reduce_size_or_wait` | 降仓或等待 | 需要更严格 RR 和仓位控制 |
| `unknown_context_only` | 未定义标签 | 默认仅作背景，不能作为 open 权限 |

## 5. 命名规范

后续新增标签必须遵守：

1. `reason_codes` 用事实证据命名：`strong_reclaim`、`oi_heavy_flush`、`taker_buy_aggressive`。
2. `risk_tags` 用风险/限制命名：`do_not_open_until_confirmed`、`not_near_long_reclaim_zone`。
3. `required_confirmations` 用可验证条件命名：`15m_close_above_vwap_or_ema20_or_entry_zone_upper`。
4. 不使用模糊词作为开仓许可，例如单独的 `good`、`strong`、`valid`。
5. 同一概念只能保留一个主命名：
   - 推荐：`taker_buy_aggressive` / `taker_buy_strong` / `taker_buy_neutral`
   - 避免新增：`taker_strong_buy` 这类倒装新标签
6. 新增会影响开仓/等待的标签，必须同时添加到 `hunter_v7_tag_catalog.go`。

## 6. 模块瘦身原则

避免屎山代码的具体规则：

1. Setup module 只做本形态的 `Match` 和 `Score`。
2. 不在 setup module 中写跨 setup 的分层策略。
3. 不在 prompt 中硬编码新 setup 的内部阈值。
4. 不在 trader 层读取 `reason_codes` 自行重判 setup。
5. 任何跨层共享语义都进 catalog 或明确的 helper，而不是散落字符串判断。
6. 只对“影响开仓资格”的逻辑加 helper；纯展示字段不抽象。

## 7. 本轮落地

- 新增 `provider/local/hunter_v7_tag_catalog.go`。
- 新增 `provider/local/hunter_v7_tag_catalog_test.go`。
- `hunter_v7_signal_json` 新增 `tag_semantics`。
- Candidate tier prompt 新增统一标签语义提示。
- 未定义标签默认标记为 `unknown_context_only`，防止 LLM 把未知 reason 当成开仓许可。

## 8. 后续建议

下一步不建议立刻重命名全仓所有标签。更稳的路线：

1. 先跑 1-2 天，统计 `tag_semantics.llm_action=unknown_context_only` 出现频率。
2. 对高频未知标签补 catalog 定义。
3. 只对高频且歧义严重的标签做兼容重命名。
4. 最后再考虑把核心标签从裸字符串迁移为 typed constants。
