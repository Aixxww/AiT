# Hunter v7 精益内核重构方案（Lean Core Redesign）

> 日期：2026-07-26
> 性质：架构收敛重构——行为等价或更优，不是策略改版
> 范围：`provider/local/hunter_v7_*` / `kernel/engine*.go` / `trader/auto_trader_risk.go` / `cmd/*`
> 依据：三路代码审计（provider / kernel / trader 逐层量化）+ 2026-06 以来 37 次补丁提交史 + 当日实盘验证数据
> 原则：**每一步都是独立可编译、可测试、可提交、可回滚的最小单元；全程有金标准回放测试兜底**

---

## 0. 一句话诊断

Hunter v7 的六层架构设计是对的（发现 → 分层 → 解释 → 执行），但两个月的补丁式迭代让**同一个判断在多处生长出不同的实现**：词汇失控（65 个确认码、约 174 个 tier reason、235 个标签，全是裸字符串）、规则弥散（48 个阈值簇内联在 6 个平行 switch 里）、几何原语重复（入场区位置 5 套实现、RR 4 套、taker 比较器 4 族且边界语义相反）。重构不是重写策略，而是**把弥散的判断收敛回单一事实源**。

---

## 1. 问题量化（审计证据）

### 1.1 体量与集中度

| 层 | 文件 | 行数 | v7 函数数 | v7 占比 |
|---|---|---:|---:|---:|
| kernel | `engine.go` | 3527 | 68 | 63.1% |
| kernel | `engine_prompt.go` | 2732 | 44 | 51.5% |
| kernel | `engine_prompt_compact_test.go` | 3756 | 97 测试 | 98.5% |
| kernel | `engine_prompt_live_confirm.go` | 233 | 8 | ~100% |
| trader | `auto_trader_risk.go` | 2360 | 40 | ~62%（877–2331 行） |
| trader | `auto_trader_risk_test.go` | 1749 | 36 测试 | — |
| provider | `hunter_v7_*.go` 共 64 文件 | **17433** | 46 生产文件 13765 行 | 100% |
| provider | 其中 21 个信号模块（18 文件） | 4773 | 27 个 Score 函数 | 生产层 35% |

**Hunter v7 相关代码总量 ≈ 27400 行**（provider 17433 + kernel v7 部分 ~6000 + trader v7 部分 ~2800 + cmd ~1300）。2026-06-01 以来 51 次提交中 **37 次**触及 Hunter v7 文件——补丁密度即熵增速度。18 个模块文件中 **15 个没有专属测试**：4773 行评分逻辑背后只有约 550 行模块级测试。

### 1.2 词汇失控（最严重的一类债务）

| 词汇域 | 总量 | 问题 |
|---|---:|---|
| required-confirmation 码 | **65** | 4 个独立 switch 各认得 30/18/12/11 个；**35 个码没有任何求值器**（落进 default 分支，永久不可满足但不阻断）；1 个死分支（`taker_buy_15m_gt_0_50` 无任何生产者）；同一谓词 3 种拼写（`taker_buy_gt_0_52` / `taker_buy_15m_gt_0_52` / `taker_buy_15m_stays_above_0_52`） |
| tier_reason | **~174**（89 固定 + ~85 模板展开） | **零常量声明**，全部字面量返回，LLM 与 `auto_trader_loop.go:1050` 直接消费 |
| tag catalog | **235 条**，9 种 `llm_action` | kernel 只在 6 个调用点消费其中 2 种 action（RejectOnly/WaitOnly），且查表前还有 7 个硬编码标签名的前置 switch——**目录是装饰，不是分发** |
| 目录反向覆盖 | 全库实际流通标签 **399 个** | **187 个（47%）未入目录**，在 prompt 里全部静默降级为 `unknown_context_only`——LLM 眼中一半的活词汇是"未知语义"；同时目录还背着 4 个零生产者的死条目和 ~23 个可疑条目 |
| 裸字符串标签检查 | **165 处**（非测试） | `containsStringValue` 类调用：`engine.go` 126 处、trader 32 处 |

**关键结构性缺陷**：`fresh_micro_confirmed` 这一个码有**三个独立的写入者**（`hunter_v7_router.go:269` 要求它、`engine_prompt_live_confirm.go:81` 铸造它、`auto_trader_risk.go:1294` 也铸造它），没有单一 owner。

### 1.3 规则弥散（tier 分类器）

- `classifyHunterV7CandidateTierWithGeometry`（`engine.go:661-785`）：125 行，圈复杂度 ~40，19 个字面量返回点。
- 它的传递闭包：**65 个函数 / 1905 行 = engine.go 全部函数体的 54%**。
- 23 个 setup-type 字符串在 engine.go 出现 **90 次**，散布在 **6 个平行 switch** + 约 12 个 `hunterV7<Setup><条件>` 专用谓词里——**没有 setup 注册表**，同一 setup 的规则要看 6+ 个地方。
- **48 个内联阈值簇**（`prio≥X && setup≥Y && timing≥Z && risk<W ...`），无一命名常量：
  - `V7AIPriority >=`：48 处、13 个不同值（45,47,48,50,52,55,58,60,62,65,70,72,75）
  - `V7SetupScore >=`：38 处、14 个不同值
  - `V7TimingScore >=`：46 处、11 个不同值
  - `V7RiskScore <`：49 处、11 个不同值（且 `<=45` 与 `<45` 混用）
  - taker 比较：**64 个调用点、18 种 (函数,阈值) 组合**（0.42→0.62）
- 测试成本与分支数同构：3756 行测试、110 个内联 `CandidateCoin{...}` fixture、仅 2 个共享 helper——**每加一条 tier 规则 ≈ 40 行拷贝测试**。

### 1.4 几何原语重复（含 2 个潜在 bug）

| 原语 | 实现数 | 分歧 |
|---|---:|---|
| 入场区位置 % | **5** | provider 版 clamp 0-100；kernel 两处内联**不 clamp**；另有 ±3% 容差版和严格版（fail-open） |
| RR | **4** | provider 绝对值比、kernel 百分比归一版、backend-capped 第 4 变体；`MinRR=1.5` 两处独立字面量手工同步 |
| taker 比较器 | **4 族** | 缺数据时三种策略（fail-open true / false / (false,false)）；**边界语义相反**：`hunterV7TakerBuyAtLeast(0.52)` 在恰好 0.52 时通过，而 live-confirm 的 `taker_buy_15m_gt_0_52` 用严格 `>` 在同一请求路径里判失败 |
| 止损距离 % | 3 | kernel 复制 provider 版含 entry-zone 回退逻辑 |
| readiness 打分 | 3 | provider 计算 241 行 → kernel 在 prompt 处**覆写**（两套权重公式），provider 版成为死重 |

### 1.5 分层泄漏与重复分类

- **kernel 重新实现 provider 判定并已漂移**：`leaderMomentumUpperZoneChaseRisk`（provider，5 票制）vs `hunterV7LeaderMomentumUpperChaseWait`（kernel，3 票制）——同名谓词、两个答案。
- **同一候选被分类 3-5 次，且几何不一致**：`engine.go:605` 用默认几何、`engine_prompt.go:745` 用配置几何、两个 formatter 兜底再跑、trader/validate 又用默认几何。`backend_rr_infeasible` 依赖几何 → kernel 与 trader 可对同一候选给出不同 tier。
- **分类之后还有第二分类器**：`hunterV7TierFromPromptReadiness` + `hunterV7PromptSemanticWaitReason` 在 125 行分类器跑完后再改判。

### 1.6 trader 守卫链

`validateHunterV7ExecutionGuard`（`auto_trader_risk.go:1580-1686`）：107 行、15 个顺序门、20+ helper 散布在单文件 1450 行内。名为 `validate*` 却在第 7/11 步**变异**候选与决策（append reason code、改仓位）——校验与执行副作用混体。

### 1.7 cmd 工具重复

`cmd/` 共 3256 行、8 个工具。`hunter_v7_validate`（835 行）手工复制生产管线：`signalsToCandidates` 逐字段拷贝 ~40 个 V7 字段、自建 Context/AccountInfo，与 `BuildUserPrompt` 的生产装配是**两套独立装配器**——本次会话的"validate 缺 TimeframeData 导致线下验证不代表生产"问题即源于此。

### 1.8 provider 层（模块与管线）

**模块同构仪式代码**：21 个模块（18 文件、4773 行、Score 均值 135 行）共享同一套开头 8 字段结构体字面量 + 同一套 5 步收尾（clamp → 分数底线 → PriceCtx/DerivativesCtx → zone/invalidation/targets → TimingScore）。每模块 ~35-45 行纯仪式 → **~750-900 行重复**。两种标签追加习语并存：裸 `append` 270 处 vs `appendIfMissing` 58 处——裸 append 路径可向 LLM 输出重复标签。

**同一测量、三套代码三套词汇**（taker 阶梯为例）：
- `mod_breakout_long.go:117`：0.60/0.55/0.52 → `taker_aggressive_buy`/`taker_strong_buy`/`taker_moderate_buy`
- `mod_squeeze_long.go:79`：0.65/0.60/0.55（不同分值）→ 同一组标签名
- `mod_pullback.go:110`：0.55/0.52/0.50 → **第三套标签名** `taker_buy_strong`/`taker_buy_recovering`/`taker_buy_neutral`

**几何常数弥散**：entry zone 的 `CurrentPrice ± ATR15m*X` 出现 **10 处、7 组不同 (X,Y)**；`pad := price*X; max(pad, ATR*Y)` 习语 6 处 6 组常数；价格百分比 pad **40 处、18 个不同乘数**——无一共享 helper。

**阈值总弥散**：仅模块+router+execution 三处就有 **1085 个数字比较点**。taker 97 处（36 种谓词、13 个截断值 0.40→0.65）、OI 115 处、RSI 23 处 16 阈值、成交额 5 档硬编码两处各一套标签。中央配置 `V7SetupThresholds` 只覆盖 zone/priority **5 个字段**，taker/OI/RSI/volume/score 门槛零中央表示。

**RouteDetailed = 27 道有序变异**：171 行主函数、双重循环内 20 道 pass + 循环后 7 道，横跨 14 个文件；`AIPriority` 被写 3 次、`Status` 被 6 个 stage 变异；顺序约束只活在注释里（`:131`、`:171`、`:203`）。

**~1270 行（生产层 9.2%）错位或死亡**：
- `hunter_v7_pnl_tracker.go`（813 行，包内最大非路由文件）只被 `trader/auto_trader.go` 使用——放错包；
- `hunter_v7_dynamic_stop.go`（147 行）同样只服务 trader；
- `hunter_v7_matrix_report.go`（117 行）只被 `api/handler_hunter.go` 只读消费；
- `hunter_v7_regime_adaptive.go`（192 行）是**断裂的反馈环**：计算自适应权重、序列化进 HTTP 响应后丢弃，router 永远直接读静态 `regimeWeightMatrix`——被遗弃的实验；
- `hunter_v7_potential_pool.go` 仅被 validate 工具消费，无生产/LLM 消费者；`cmd/hunter_v7_mover_audit` 孤儿 CLI。

**`V7SignalOutput` 45 字段的所有权混乱**：模块只负责 15 个（33%），其余 30 个由后置 pass 填写；13 个 TP 平铺字段是 `TPPlan` 的反规范化副本；4 个模块不填 `EntryZone`、4 个不填 `PriceCtx`/`TimingScore`，下游门禁必须防御零值——**模块契约不可检查**。

---

## 2. 设计哲学

重构由四条第一性原理驱动，每条直接对应上面一类病灶：

**P1 单一事实源（Single Source of Truth）**
每个判断——"taker 够不够强"、"价格在入场区哪里"、"RR 是否达标"、"这个确认码怎么验"——在整个代码库里**只允许存在一个实现**。层与层之间传递的是判断的*结果*（类型化的码），不是判断的*重复执行*。

**P2 数据即规则（Rules as Data）**
阈值矩阵是数据，不是控制流。48 个 if 簇收敛为一张表之后：表本身就是文档、diff 就是评审、测试就是表驱动、调参不再产生代码 churn。两个月来 37 次提交里的大多数"调阈值"补丁，在表驱动下是**一行数据变更**。

**P3 词汇类型化（Typed Vocabulary）**
所有跨层流通的字符串（确认码、reason code、risk tag、tier reason）升格为类型化常量 + 中央注册表。注册表携带每个码的全部元数据：语义、求值器、可否被刷新满足、LLM 动作。**编译器与穷尽性测试代替 grep 来保证一致性**——"加一个确认码但忘了在第 4 个 switch 里注册"这类 bug 从此在结构上不可能发生。

**P4 校验无副作用（Pure Validation）**
`validate*` 函数只返回判断，不变异输入。变异（铸造 reason code、缩仓、修几何）集中在显式的 `apply*` 阶段。这让守卫链可以单测、可以重排、可以并行推理。

**美学标准**（判断"这次重构做完了没有"的尺子）：
新增一个 setup 形态，应当只需要三件事——①一个模块文件（Match/Score），②规则表加一行，③新确认码在注册表登记。不需要碰 kernel、不需要碰 trader、不需要碰 prompt 序列化。

---

## 3. 目标架构

### 3.1 分层不变，边界收紧

六层职责保持 2026-06-09 治理方案的定义，但**层间契约从"字符串+约定"升级为"类型+注册表"**：

```
datafetch     → Snapshot                    （不变）
universe      → []V7SymbolContext           （不变）
regime        → V7MarketRegime              （不变）
modules(21)   → V7SignalOutput              ← 共享构造骨架，只声明差异
router        → RouteResult                 ← 增强链显式管线化
verdict       → V7Verdict{Tier,Reason,...}  ← 新：分类一次、缓存、全链共用
prompt        → 单一 payload 类型 + mask     ← 三个序列化器合一
trader guard  → 纯校验链 + 显式 apply        ← 副作用分离
```

### 3.2 新增的三个内核文件（全部在 `provider/local/`，避免循环依赖）

| 文件 | 职责 | 替代 |
|---|---|---|
| `hunter_v7_vocab.go` | 类型化词汇：`ConfirmCode`/`ReasonCode`/`RiskTag`/`TierReason` 常量 + 注册表（含求值器绑定） | 4 个确认码 switch、165 处裸字符串检查的字面量、tag catalog 的分发缺位 |
| `hunter_v7_geom.go` | 几何原语单源：`EntryZonePos`（clamp 语义显式）、`RiskReward`（百分比归一）、`StopDistancePct`、`TakerCmp`（阈值+比较方向+缺数据策略三参数显式） | 5 套 zone、4 套 RR、4 族 taker、3 套 stop |
| `hunter_v7_tier_rules.go` | 表驱动 tier 规则：`map[V7SetupType]SetupTierSpec` + 一个 ~80 行的通用求值器 | 48 个阈值簇、6 个平行 switch、12 个专用谓词、125 行分类器主体 |

### 3.3 词汇注册表设计（P3 的落地形态）

```go
type ConfirmCode string

type ConfirmSpec struct {
    Code        ConfirmCode
    Evaluate    func(Evidence) (passed, known bool) // nil = 显式声明为 context_only
    RefreshOK   bool   // 可被 trader REST/orderbook 刷新满足
    LiveReview  bool   // 可被 kernel prompt 前实时核销
    Definition  string // LLM 语义（替代 tag catalog 中对应条目）
}

var confirmRegistry = map[ConfirmCode]ConfirmSpec{ ... } // 65 → 收敛后约 40 条
```

- `Evidence` 是统一证据载体（klines/EMA/taker/OI/zone/price），provider、kernel、trader 三层用**同一求值器 + 各自时点的 Evidence** 得到一致判断——把"三层三个 switch"变成"一个函数三次调用"。
- 穷尽性测试：遍历所有模块的 `RequiredConfirms` 产出，任何未注册的码使测试失败；任何注册但无生产者的码同样失败（防死码）。当前 35 个无求值器的码在迁移时**逐个决策**：绑定求值器 / 显式标记 context_only / 删除。
- `fresh_micro_confirmed` 收归**单一 owner**：只有 trader 的刷新路径可铸造；kernel 前置核销改为铸造独立的 `live_confirmed_*` 族（本周已实现），注册表里显式声明两者关系。

### 3.4 tier 规则表设计（P2 的落地形态）

```go
type ScoreFloor struct {
    Priority, Setup, Timing float64 // >=；0 表示不设门槛
    RiskBelow               float64 // <
    LiquidityAtLeast        float64 // >=；0 = 不要求
    Taker                   *TakerRequirement // 比较方向+阈值+缺数据策略
}

type SetupTierSpec struct {
    ExecFloor    ScoreFloor           // EXECUTABLE 硬底
    ReviewFloors []ReviewRule         // 有序：首个命中的 REVIEWABLE 通道
    ExtraGates   []GateFunc           // 少数无法表化的谓词（显式具名注册，如 MMS 双帧共振）
}
```

- 48 簇中约 40 簇可直接落表；其余（panic_reversal 四通道、leader_momentum 五通道这类带 reason-code 组合条件的）用 `ReviewRule.RequireCodes []ReasonCode` 表达，仍是数据。
- 确实无法表化的少数谓词（约 6-8 个，如 `hunterV7MMSLongExecutableChaseBlock`）作为**具名 GateFunc 注册进表**，而不是散落在 switch 里——查一个 setup 的全部规则 = 读表中一行。
- 通用求值器取代 125 行分类器主体；`hunterV7TierFromPromptReadiness` 的改判逻辑合并为表的最后一列（`PromptOverrides`），消灭"第二分类器"。
- **对齐审计发现的不一致**：`47 vs 48`、`<=45 vs <45`、只出现一次的 `timing 52` 这类毛刺在落表时统一并在提交信息中逐条记录（行为变更显式化）。

### 3.5 分类唯一化

- 删除 0 参 `classifyHunterV7CandidateTier`；几何参数必传，来源唯一（`e.hunterV7ExecutionGeometry()`，读策略配置）。
- 每候选**分类一次**，结果写入 `V7Verdict{Tier, Reason TierReason, ClassifiedWithGeometry}`；prompt formatter、trader、validate 全部读缓存，不再兜底重跑。修复"kernel 与 trader 对同一候选因几何不同而 tier 不同"的正确性缺陷。

### 3.6 序列化合一

- `v7SignalForAI`（53 字段的函数内局部类型）提升为包级 `V7PromptPayload`；
- compact JSON（14 字段，是全量的严格子集）= 同一类型 + field mask；
- `formatHunterV7ExecutionCompact`（第三种 k=v 文本编码）= 同一类型的文本 encoder；
- schema 变更从"改三处三种编码"变为"改一处"。
- `PromptCompactMode` 五值枚举实际只有 on/off 两种行为 → 收敛为 bool（保留配置兼容读取）。

### 3.7 trader 守卫链重组（P4）

15 个顺序门重组为三段：

```
Stage A  资格校验（纯函数）：tier 门、tag action 门、信号契约、freshness、必需确认
Stage B  实时补证（有 IO，无变异）：REST/orderbook 刷新 → 产出 Evidence
Stage C  应用（显式变异）：铸造 fresh_micro_confirmed、缩仓（borderline/低流动性/风险 tag）、几何修复
```

每段独立可测；A 段直接复用词汇注册表求值器（与 kernel 同一实现）。

### 3.8 模块骨架与模块级阈值表

21 个模块的同构构造代码提取为 `signalScaffold`：

```go
type scaffold struct{ sig *V7SignalOutput; ctx *V7SymbolContext }

func newSignal(ctx, regime, setup, dir, entryMode, confidence) *scaffold  // 统一 8 字段头
func (s *scaffold) zoneATR(below, above float64) *scaffold                // 10 处 7 组常数 → 一个模板
func (s *scaffold) zonePad(pctFloor, atrMult float64) *scaffold           // 6 处 pad 习语 → 一个模板
func (s *scaffold) reason(codes ...ReasonCode) *scaffold                  // 一律 appendIfMissing（消灭 270:58 分裂）
func (s *scaffold) takerLadder(band TakerBand) *scaffold                  // 三套 taker 阶梯 → 一个函数 + 数据化档位；统一标签词汇
func (s *scaffold) finish(minScore float64) *V7SignalOutput               // 统一 5 步收尾 + 契约检查
```

- `finish` 内建**模块契约检查**：`EntryZone`/`Invalidation`/`Targets`/`PriceCtx` 缺失时 panic-in-test / log-in-prod——消灭"4 个模块不填 zone、下游防御零值"的隐性契约。
- taker/OI/RSI/volume 的评分档位收进 `V7SetupThresholds` 扩展表（现有 5 字段 → 补 `TakerBand`/`OIBand`/`RSIBand`/`VolumeTiers`），模块间差异变成数据差异。
- 预计单模块从 ~150-390 行降到 ~60-120 行；三套 taker 标签词汇统一为一套。

### 3.10 路由管线：顺序即数据

`RouteDetailed` 的 27 道变异 pass 改为显式 stage 切片：

```go
var v7SignalPasses = []signalPass{
    {"regime_weight",  applyRegimeWeight},
    {"liquidity",      applyLiquidity},
    {"multi_tp",       ApplyMultiTimeframeTP},
    {"timing_booster", ...},
    // ... 顺序约束从注释升格为数组次序；每个 pass 具名、可单测、可插拔
}
```

- 6 个内联片段（字段传播、regime 乘法、强势覆盖、resonance 加成、过滤标记）具名化；
- `AIPriority` 三次写入合并为一个 `finalizePriority` pass；
- 熔断器（现有）与新增 pass 的挂载点即此切片——**扩展入口显式化**。

### 3.9 命名规范（统一标签命名法）

新增/迁移词汇统一为 `{域}_{指标}[_{周期}]_{关系}[_{参数}]` 小写蛇形：

| 域前缀 | 含义 | 示例 |
|---|---|---|
| `flow_` | taker/成交流 | `flow_taker15m_ge_052`（迁移自 `taker_buy_15m_gt_0_52`，注册表保留 alias 供 LLM 兼容期） |
| `oi_` | 持仓量 | `oi_1h_pos_or_volume_expands` |
| `px_` | 价格/K线结构 | `px_5m_holds_ema20` |
| `zone_` | 入场区几何 | `zone_price_inside` |
| `risk_` | 风险约束 | `risk_taker_borderline` |
| `state_` | 跨周期状态 | `state_do_not_open_until_confirmed` |

迁移策略：**注册表内建 alias 映射**（旧名 → 新名），prompt 输出新名 + `tag_semantics` 继续解释；两个实盘验证周期后删除旧名。不搞一次性全库重命名（churn 与风险不成比例）。

---

## 4. 实施路线图（最小单元拆解）

> 每个单元 = 一个提交。验收三件套：`go build ./... && go test ./...` 全绿 + **金标准回放 diff 为空**（U0.1 建立）+ 涉及行为变更时跑一轮 `hunter_v7_validate` 实盘对比。
> 依赖关系：Phase 0 先行；Phase 1-2 可并行；Phase 3 依赖 1+2；Phase 4-5 依赖 3；Phase 6 依赖 1，可与 3-5 交错推进。
> 总量约 60 个提交单元；按当前节奏（每会话 5-8 单元）预计 8-10 个工作会话完成，任意中间态可上线运行。

### Phase 0 — 安全网（必须最先做）

| 单元 | 内容 | 文件 | 验收 |
|---|---|---|---|
| **U0.1** | **金标准回放测试**：将一份完整实盘 raw snapshot（已有：`reports/hunter-v7-verify-final-20260726/`）固化为 testdata fixture；新增 `hunter_v7_golden_replay_test.go`：snapshot → ScoreHunterV7Detailed → 分类 → 断言全部信号的 {setup, tier, reason, 分数, tags, confirms} 与 golden JSON 逐字段一致。提供 `-update` flag 重新生成 | `provider/local/testdata/`、新测试文件 | 回放通过；手工改一个阈值能让它失败（灵敏度自检） |
| U0.2 | 把 U0.1 扩展到 kernel 层：同一 fixture 走 BuildUserPrompt，快照 prompt 文本的 tier summary 与 open-review 段 | `kernel/` 测试 | 同上 |

### Phase 1 — 词汇内核（行为零变更）

| 单元 | 内容 | 验收 |
|---|---|---|
| U1.1 | `hunter_v7_vocab.go`：`ConfirmCode` 类型 + 65 个常量 + `ConfirmSpec` 注册表骨架（求值器先留 nil，语义从 tag catalog 迁移） | 编译过；golden 不变 |
| U1.2 | 穷尽性测试：扫描全部模块 `RequiredConfirms` 生产 vs 注册表；输出当前 35 个孤儿码清单为测试基线（`knownOrphans` 显式列表，新增孤儿即失败） | 测试固化现状 |
| U1.3 | 统一求值器：把 `evaluateV7KnownConfirmation`（provider, 30 码）的逻辑迁入注册表 `Evaluate` 字段；provider 调用点切换 | golden 不变 |
| U1.4 | kernel 两个 switch（`CanBeLiveReviewed` 11 码、`VerifyLiveConfirmation` 12 码）改读注册表 `LiveReview` 标志 + 统一求值器；**修复**：5 个"声明可复核但无法验证"的码显式降级、删除死分支 `taker_buy_15m_gt_0_50` | golden 不变；差异码逐个记录 |
| U1.5 | trader `CanBeSatisfiedByRefresh`（18 码）改读注册表 `RefreshOK` 标志 | golden 不变 |
| U1.6 | 拼写收敛：`taker_buy_gt_0_52` 等 3 变体经 alias 归一；`fresh_micro_confirmed` 铸造点收归 trader 单一 owner（kernel 保持 `live_confirmed_*` 族） | golden 允许码名 alias 级差异，逐条审 |
| U1.7 | `TierReason` 类型 + 89 个固定值常量化 + 5 个模板族改为类型化构造函数 | golden 不变 |
| U1.8 | **目录对账**：新增测试断言"流通标签集 ⊆ 注册表"；187 个未入目录标签逐批登记（按 §3.9 命名法，旧名 alias）；4 个零生产者死条目删除、23 个可疑条目逐个裁决 | 测试固化；prompt `tag_semantics` 不再出现 `unknown_context_only`（白名单例外除外） |

### Phase 2 — 几何原语（修 2 个已知语义 bug）

| 单元 | 内容 | 验收 |
|---|---|---|
| U2.1 | `hunter_v7_geom.go`：`EntryZonePos`（单实现，clamp 0-100，容差作参数）；kernel 5 处调用点切换，删除本地副本 | golden 不变（unclamped→clamped 的差异点逐个确认） |
| U2.2 | `TakerCmp{Dir, Threshold, MissingPolicy}` 单实现；64 个调用点分批切换（每批一个提交）；**显式修复** 0.52 边界相反判定——统一为 `>=`/`<=`，注册表码名里的 `gt/lt` 语义随 alias 更新 | 边界值单测；golden 差异仅限恰好压线的候选，逐个审 |
| U2.3 | `RiskReward` + `StopDistancePct` 单实现（百分比归一制）；`MinRR` 收归单一常量，provider/kernel 两处字面量删除 | golden 不变 |
| U2.4 | readiness 单源化：删除 kernel 的 `hunterV7PromptWindowHealth`/`hunterV7PromptReadyScore` 覆写，provider 公式为准（或迁 prompt 公式入 provider——以 golden diff 更小者定） | golden 差异记录 |

### Phase 3 — 分类器收敛（核心阶段）

| 单元 | 内容 | 验收 |
|---|---|---|
| U3.1 | 分类唯一化：删 0 参入口、几何必传、`V7Verdict` 缓存、4 处重复分类点改读缓存。**修复 kernel/trader 几何不一致** | golden：validate 路径 tier 变化逐个审（这是修 bug，允许变化） |
| U3.2 | `hunter_v7_tier_rules.go`：定义 `SetupTierSpec` 表 + 通用求值器；**影子模式**——新表与旧代码并行跑，diff 非空即测试失败（用 golden fixture + 110 个现存测试 fixture 双重喂入） | 影子 diff = 0 |
| U3.3a-u | **逐 setup 迁移**（21 个单元，每个一提交）：把该 setup 的全部 switch 分支 + 专用谓词迁入表行/GateFunc，删除旧分支。顺序：先简单的（distribution/range_reversion）后复杂的（panic_reversal、leader_momentum） | 每步影子 diff=0 + golden 不变 |
| U3.4 | 删除影子模式与旧 48 簇残骸；`hunterV7ReviewableCandidateReason`（255 行）等 6 个 switch 函数移除 | engine.go 预计 −~1600 行 |
| U3.5 | kernel 重复判定删除：`hunterV7LeaderMomentumUpperChaseWait`（漂移的 3 票版）删除，消费 provider 已产出的 `momentum_upper_zone_chase` tag；`PromptSemanticWaitReason` 并入表 `PromptOverrides` 列 | golden：leader_momentum 候选 tier 差异逐个审（以 provider 5 票版为准） |
| U3.6 | 测试表驱动化：110 个内联 fixture → `candidateBuilder` + 规则表驱动断言；分类器测试文件预计 3756 → ~1200 行 | 覆盖率不降（`go test -cover` 对比） |

### Phase 4 — 序列化与 prompt

| 单元 | 内容 | 验收 |
|---|---|---|
| U4.1 | `V7PromptPayload` 包级化（53 字段），`formatHunterV7SignalJSON` 改为薄包装 | prompt 逐字节不变（U0.2） |
| U4.2 | compact JSON = mask；`formatHunterV7ExecutionCompact` 同源文本 encoder | 同上 |
| U4.3 | `PromptCompactMode` → bool（兼容读旧配置值） | 同上 |

### Phase 5 — 结构收尾

| 单元 | 内容 | 验收 |
|---|---|---|
| U5.1 | `CandidateCoin` 分组：45 个 V7 字段 → 嵌入 `V7Signal`（不可变 payload，删 12 个与 `V7TPPlan` 重复的 TP 平铺字段）+ `V7Verdict`（派生态）。JSON tag 保持不变以兼容存量 store | golden + prompt 不变 |
| U5.2 | trader 守卫链三段化（§3.7）：纯校验 / 补证 / 显式 apply；`validate*` 全部去副作用 | trader 测试全绿 + 一轮实盘 validate |
| U5.3 | `cmd/hunter_v7_validate` 改用生产装配器（导出 kernel 侧 assembler），删除 `signalsToCandidates` 手工拷贝与自建 Context | validate 输出与生产 prompt 结构一致 |
| U5.4 | 包归位：`hunter_v7_pnl_tracker.go`(813) + `hunter_v7_dynamic_stop.go`(147) → `trader/`；`hunter_v7_matrix_report.go`(117) → `api/` 侧；**删除** `hunter_v7_regime_adaptive.go`(192, 断裂反馈环) 与 `cmd/hunter_v7_mover_audit`；`potential_pool` 保留（validate 消费）但显式标注非生产路径 | 编译过、无引用残留 |
| U5.5 | tag catalog 收编：`DescribeHunterV7Tags` 改读 vocab 注册表生成；旧 catalog map 删除 | prompt `tag_semantics` 输出等价 |

### Phase 6 — provider 层收敛（依赖 Phase 1 词汇；可与 Phase 3-5 交错）

| 单元 | 内容 | 验收 |
|---|---|---|
| U6.1 | `signalScaffold` 骨架（§3.8）：头/尾仪式 + zoneATR/zonePad 模板 + `reason()` 统一 appendIfMissing + `finish()` 契约检查 | golden 不变 |
| U6.2a-r | **逐模块迁移到骨架**（18 文件，每文件一提交；顺序：小模块先行 mms/alt_ladder → 大模块 funding_reversal 收尾） | 每步 golden 不变 |
| U6.3 | taker 阶梯统一：`takerLadder(TakerBand)` + 三套标签词汇合一（alias 期照 §3.9）；档位入 `V7SetupThresholds` 扩展 | golden 允许标签 alias 级差异 |
| U6.4 | OI/RSI/volume 评分档位数据化，入同一扩展表 | golden 不变 |
| U6.5 | `RouteDetailed` 管线化（§3.10）：27 道 pass → 具名 stage 切片；`AIPriority` 三写合一 | golden 不变；stage 单测 |
| U6.6 | `V7SignalOutput` 收敛：13 个 TP 平铺字段并入 `TPPlan`（与 U5.1 的 CandidateCoin 侧同步做）；评估 `ModuleSignal`/`EnrichedSignal` 拆分（可选，视 U6.2 后剩余复杂度） | golden + prompt 不变（JSON tag 兼容层过渡） |

### 里程碑与预期收益

| 里程碑 | 完成单元 | 预期削减 | 结构性收益 |
|---|---|---|---|
| M1 安全网 | U0.* | +300 行（测试） | 全部后续步骤可验证 |
| M2 词汇统一 | U1.* | −~500 行 | 加确认码 = 注册表一条；孤儿码/未注册标签结构性绝迹；LLM 不再看到 47% 的 `unknown_context_only` |
| M3 几何统一 | U2.* | −~250 行 | 2 个边界 bug 修复；阈值单源 |
| M4 规则表化 | U3.* | −~2100 行（engine.go 3527→~1900；测试 3756→~1200） | 调参 = 改数据；新 setup = 表一行 |
| M5 序列化+结构 | U4-5.* | −~700 行（含 1270 行归位/删除的净减部分） | 三序列化合一、守卫可测、工具与生产同源、评分包纯净 |
| M6 provider 收敛 | U6.* | −~1200 行（模块仪式 −800、管线/结构 −400） | 新模块 ≈ 60 行；三套 taker 词汇合一；管线顺序即数据 |
| **合计** | | **−~4750 行（约占 v7 总量 27400 的 17%），高风险重复实现归零** | 见 §5 扩展点对照表 |

---

## 5. 扩展点（预留的增减入口）

| 扩展场景 | 改动点（重构后） | 今天需要改的地方（对照） |
|---|---|---|
| 新增 setup 形态 | 模块文件 + `SetupTierSpec` 表一行 + 新确认码注册 | 模块文件 + 6 个 switch + 4 个确认码 switch + tag catalog + 测试 fixture ~40 行/条 |
| 调 tier 阈值 | 表中一个数字 | 找到 48 簇中的正确一簇 + 改对应测试 fixture |
| 新增确认码 | 注册表一条（含求值器） | 4 个 switch + catalog + 各层字面量 |
| 新增风险 tag 及仓位策略 | 注册表一条 + sizing 表一行 | catalog + trader apply 函数 + kernel 检查点 |
| 新增 regime | 权重矩阵一列（现有机制保留） | 同左（此处现状已合理） |
| 禁用某模块 | 权重置 <0.2 或熔断器（现有机制保留） | 同左 |
| 新增 prompt 字段 | `V7PromptPayload` 一个字段 | 3 个序列化器 × 3 种编码 |

---

## 6. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 重构引入行为漂移 | U0.1 金标准回放是硬门禁；任何 golden diff 必须逐条人工确认并写入提交信息（"行为变更显式化"） |
| 表驱动丢失原有分支的隐式优先级 | U3.2 影子模式：新旧并行、diff 非空即失败，跑满全部 110 个存量 fixture 后才允许切换 |
| alias 期 LLM 对新旧码名混淆 | `tag_semantics` 同时输出新旧名映射；两个实盘周期后才删旧名 |
| 21 个 setup 迁移中途搁置 | 每个 setup 独立提交、影子模式常驻，任意中间态都是可运行的混合态 |
| 与实盘运行冲突 | 全程只在 main 分支小步提交（延续现有工作流）；每个 Phase 结束跑一轮 `hunter_v7_validate` 与前基线对比 tier 分布/ExecRate |

回滚粒度 = 单元粒度：每个提交独立 revert 不破坏编译。

---

## 7. 验证方案

1. **静态**：`go build ./... && go vet ./... && go test ./...` 每单元强制。
2. **回放**：U0.1/U0.2 金标准（provider 信号层 + kernel prompt 层）每单元强制。
3. **影子**：Phase 3 期间新旧分类器并行 diff（测试内），diff=0 才可删旧。
4. **实盘**：每 Phase 结束跑 `go run ./cmd/hunter_v7_validate -rounds 3 -round-interval 5m -max-output 160 -watch-output 60 -min-priority 20`，用 `scripts/hunter_v7_live_analysis.mjs` 对比：SignalRate / tier 分布 / ExecRate / StalePct / FullCover 与重构前基线（`reports/hunter-v7-3round-5m-live-20260726/` + `reports/hunter-v7-verify-final-20260726/`）。
5. **覆盖率**：U3.6 前后 `go test -cover ./provider/local/ ./kernel/ ./trader/` 对比，不允许下降。

---

## 8. 附录：provider 层清点数据（U5.4/U6.* 执行清单）

### 8.1 归位/删除清单

| 文件 | 行数 | 现状 | 处置 |
|---|---:|---|---|
| `hunter_v7_pnl_tracker.go` | 813 | 只被 `trader/auto_trader.go:197,459,461` 使用 | 迁 `trader/` |
| `hunter_v7_dynamic_stop.go` | 147 | 只被 `trader/auto_trader_risk.go:208` 与 pnl_tracker 使用 | 迁 `trader/` |
| `hunter_v7_matrix_report.go` | 117 | 只被 `api/handler_hunter.go:183` 只读消费 | 迁 api 侧或独立 reporting 包 |
| `hunter_v7_regime_adaptive.go` | 192 | 断裂反馈环：结果只进 HTTP 响应，router 永远读静态矩阵；`GetEffectiveWeight` 仅自测引用 | **删除**（连同 64 行测试；若未来要自适应权重，走 U6.5 的 stage 插槽重做） |
| `hunter_v7_potential_pool.go` | 180 | 生产路径产出但仅 `cmd/hunter_v7_validate` 消费 | 保留，文件头标注非生产消费 |
| `hunter_v7_oi_accumulation.go` | 127 | `ApplyV7OIAccumulationEvidence` 3 模块在用；`AssessV7OIAccumulation` 仅自测引用 | 收窄导出面 |
| `cmd/hunter_v7_mover_audit/` | 483 | 孤儿 CLI，无外部引用 | 删除 |

### 8.2 tag catalog 死条目（U1.8 直接删除）

`mms_ema_retest_not_held`、`mms_liquidity_too_low`、`mms_trend_ride_chase_risk`、`price_holds_trailing_support`（4 条零生产者）；另 23 条可疑条目（`alt_ladder_downshift_mid/late`、`fresh_rest_confirmed`、8 个 `5m_or_15m_*` 模板等）逐条裁决。

### 8.3 未入目录的 187 个流通标签（U1.8 登记清单示例）

`extreme_squeeze / mild_squeeze / moderate_squeeze`、`lsr_reversal / lsr_improving / lsr_shifting / lsr_recovering / lsr_bullish`、`healthy_pullback`、`deep_capitulation`、`extreme_funding`、`heavy_taker_selling`、`at_range_top / at_range_bottom`、`invalid_rr_context_only`、`entry_zone_normalized`、`fast_tracked_funding`、`multi_cycle_confirmation` 等——完整清单由 U1.8 的对账测试第一次运行时自动产出并固化为迁移 checklist。

### 8.4 关键重复实现坐标（U2/U3.5 执行锚点）

- 入场区位置：`hunter_v7_execution.go:597`（clamp 版，保留）vs `engine.go:3421`、`engine_prompt.go:1098`（未 clamp，删）vs `engine.go:1757`（±3% 容差版，参数化并入）vs `engine.go:3482`（严格版，参数化并入）
- RR：`hunter_v7_execution.go:217`（保留、百分比归一化改造）vs `engine.go:1040`、`engine.go:2175`、`engine.go:2140`（删/薄封装）
- taker 比较器：`engine.go:2112/2119/2126/2133`、`engine_prompt_live_confirm.go:188/196`、`hunter_v7_confirmation.go:383` → 收敛为 `hunter_v7_geom.go` 单实现
- leader chase 漂移对：`hunter_v7_execution.go:117-162`（5 票，保留）vs `engine.go:3401-3441`（3 票，删除）
- readiness 覆写：`hunter_v7_readiness.go:44/163` vs `engine_prompt.go:1100/1135/1151/1174`（U2.4 单源化）
- `fresh_micro_confirmed` 三写入点：`hunter_v7_router.go:269`（要求方）、`engine_prompt_live_confirm.go:81`、`auto_trader_risk.go:1294`（U1.6 收归 trader 单一 owner）
- 确认码 4 switch：`hunter_v7_confirmation.go:140-292`（30 码）、`auto_trader_risk.go:1821`（18 码）、`engine.go:927`（11 码）、`engine_prompt_live_confirm.go:131`（12 码）→ 注册表单源
