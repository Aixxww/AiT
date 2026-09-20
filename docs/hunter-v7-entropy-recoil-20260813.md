# Hunter v7 二次熵减方案（Lean Core Entropy Recoil）

> 日期：2026-08-13  
> 性质：**行为等价优先**的结构收敛——不是策略改版，不借机调阈值  
> 范围：`kernel/hunter_v7_tier_rules.go` / `kernel/engine.go`（分类特例谓词）/ 相关测试 / 可选 `trader` 守卫镜像  
> 依据：`docs/hunter-v7-lean-core-redesign-20260726.md` + completion report + 2026-08-13 审校  
> 原则：每单元独立可编译、可测、可提交、可回滚；**golden 双层硬门禁**；阈值变更必须显式记录

---

## 0. 一句话诊断

Lean Core（词汇 / 几何 / 规则表 / scaffold / 管线 / 守卫三段）已经把「六处平行 switch」收敛成表驱动骨架，但 7/26 之后多轮实盘补丁把 **alt_ladder soft-release、whale-flow 区位门、late-flow / MMS chase** 又写回 `engine.go` 的大段复合 `Guards`。表行成了入口指针，规则本体仍是代码——**公理四「调参=改数据」局部失效，「轻」最先回潮**。本方案只做一件事：把这些复合谓词压回可声明的规则数据，恢复「稳快准轻」。

---

## 1. 审校证据（现状量化）

### 1.1 相对 lean 完成点（`c8bf134` U6.3 cleanup → HEAD）

| 面 | 当时 | 现在 | Δ |
|---|---:|---:|---:|
| `kernel/engine.go` | 3144 | 3347 | **+203** |
| provider `hunter_v7_*`（非测试） | 12636 | 12999 | +363 |
| `auto_trader_risk.go` | 2368 | 2404 | +36 |
| `hunter_v7_tier_rules.go` | 653 | 676 | +23 |

相对 6 月审计基线仍有改善（`containsStringValue` ~165→108，`V7AIPriority` 比较 ~48→27），但对 lean 完成点已二次变重。

### 1.2 「表驱动空壳」病灶（本方案主靶）

| 谓词 | 位置 | 行量级 | 问题 |
|---|---|---:|---|
| `hunterV7AltLadderShortLayeredReleaseOK` + `HardBlock` | `engine.go` | ~60 | soft-release 阈值（zone≤45 / taker≤0.38|0.34 / stop≤2.25|2.45 / OI≥0.8）埋在控制流 |
| `hunterV7AltLadderShortReviewableOK` | `engine.go` | ~35 | 表 `Reviewable.Guards` 只挂函数名 |
| `hunterV7AltLadderLongExecutable` + `LateLongNeedsFreshFlow` | `engine.go` | ~70 | 分数底 + taker + reason/risk 组合本可落表 |
| `hunterV7WhaleFlowLongEntryGatesOK` | `tier_rules.go` | ~20 | zone≤45 / taker≥0.56 与 trader 守卫镜像，仍是手写 |
| `hunterV7MMSLongExecutableChaseBlock` / `FreshEnough` | `engine.go` | ~40 | 仍以 Guards 复合形式存在 |
| pre-classify 特例 | `classify*WithGeometry` 前段 | ~80 | extreme continuation / rebound pending 等未进表列 |

美学尺子（方案原文）：

> 新增 setup = 模块 + 表一行 + 确认码登记。  
> **调参 = 改表中一个数字。**

当前 soft-release 调参路径：改 `engine.go` → 改 fixture 名 → 改 doctrine 散文——**三处 churn**。

### 1.3 非本方案范围（刻意不做）

| 项 | 理由 |
|---|---|
| U5.1 / U6.6 字段分组与 TP 平铺 | completion 已裁决收益有限，且会动 prompt 字节 |
| 策略阈值再调优（开仓率/胜率） | 本方案零策略意图；阈值迁移必须行为等价 |
| doctrine 长文瘦身 | 可后续单独立项；不阻塞结构收敛 |
| dashboard / 前端 | 与分类内核无关 |
| 删除 outcome / continuation 保护 | 属执行层正确性；仅要求不再向 `engine.go` 分类器堆特例 |

---

## 2. 设计原则（承接 Lean P1–P4）

| ID | 原则 | 本方案落地 |
|---|---|---|
| **P1** | 单一事实源 | soft-release / zone / OI / stop 门槛只存在于规则表字段 |
| **P2** | 规则即数据 | 扩展 `hunterV7TierRule` 声明力，消灭「Guards 黑盒承载全部语义」 |
| **P3** | 词汇类型化 | soft-release reason 码常量化；新增码必须进注册/测试 |
| **P4** | 校验无副作用 | 求值器纯函数；不在匹配路径铸造 tag |
| **P5** | **熵预算**（新增） | CI 断言：`engine.go` 内 `hunterV7*Executable\|*Release\|*Chase*` 复合谓词数量与总行数有上限；超限失败 |

验收美学（完成定义）：

1. 改 soft-release 的 zone/taker/stop/OI **只改 `hunter_v7_tier_rules.go` 表字段**；  
2. `engine.go` 不再包含上述阈值字面量；  
3. 现有 soft-release / whale-flow 分类测试与 golden **行为等价**（允许 reason 字符串 alias 级差异，须逐条审）；  
4. 熵预算测试绿灯。

---

## 3. 目标形态

### 3.1 扩展规则字段（向后兼容）

在现有 `hunterV7TierRule` 上增加**可选**声明字段（零值 = 不约束，保持旧行语义）：

```go
type hunterV7ZoneGate struct {
    MaxPos       float64 // entry_zone_position <= MaxPos
    MinPos       float64 // >= MinPos；0 = 不设
    RequireKnown bool    // 区位不可算则失败（修 soft-release「缺失当 0」的坑）
}

type hunterV7OIGate struct {
    MinChange1h float64 // DerivativesCtx.OIChange1h >=
    MaxChange1h float64 // <= ；0 = 不设上限
    MinChange4h float64
    RequireCtx  bool    // 缺 derivatives 则失败
}

type hunterV7StopGate struct {
    MaxPct float64 // stop distance % <= MaxPct；0 = 不检查
}

// 挂到 hunterV7TierRule：
//   Zone  hunterV7ZoneGate
//   OI    hunterV7OIGate
//   Stop  hunterV7StopGate
//   RequireReasonAny / RequireRiskAny 已有 RequireAny 可复用；
//   若需「reason 与 OI 同时成立」用 RequireAll + OI 组合表达。
```

`hunterV7TierRuleMatches` 在现有分数/taker/Require* 之后求值 Zone/OI/Stop——**单一匹配器增强，不新增第二分类器**。

### 3.2 Soft-release 表化形态（alt_ladder_breakdown_short）

现状：`Reviewable` 一行 + `Guards: hunterV7AltLadderShortReviewableOK`（内部再调 LayeredRelease）。

目标：拆成**有序 Reviewable 行**（首个命中生效，与现求值器一致）：

| 行 | 语义 | 声明要点 |
|---|---|---|
| R1 标准软升档 | early/mid + zone≤45 known + new_shorts + OI1h≥0.8 + taker≤0.38 + stop≤2.25 | Zone/OI/Stop/Taker/RequireAny stages + Forbid hard-block tags |
| R2 强卖流软升档 | 同上但 taker≤0.34、stop≤2.45 | 同结构不同阈值 |
| R3 原 close-through / taker_sell 复核通道 | 保留现有非 soft 路径 | 尽量用 Require* + Taker；残余 late 组合用**一个**具名 Guard |

Hard-block（高波动 / late 无 close-through / taker>0.42 / funding_elevated∧stop_tightened）→ `ForbidAll` / `ForbidAny` 扩展或行前 `HardBlocks []string` 列。

**禁止**：再写第三个 `LayeredReleaseV2` 函数。

### 3.3 Long executable / whale-flow

- `alt_ladder_momentum_long` Ready：分数与 taker 底进表；late fresh-flow / stop_tightened 升档进第二 Ready 行或 `RequireAny`；最多保留 **1** 个具名 Guard（若 OI 缺席语义无法表化）。  
- `whale_flow_reversal` Ready：`Zone{MaxPos:45, RequireKnown:false}` + `Taker{at_least, 0.56}` 替换 `hunterV7WhaleFlowLongEntryGatesOK`；缺数据放行语义用 `RequireKnown:false` + taker 缺省放行（与现注释一致）。

### 3.4 Pre-classify 特例收编

`classifyHunterV7CandidateTierWithGeometry` 前段的：

- `alt_ladder_extreme_continuation_watch`  
- `alt_ladder_short_rebound_pending`  

升格为 `hunterV7SetupTierSpec` 的可选列：

```go
EarlyWatch []hunterV7TierRule // 在通用 REJECTED/WATCH 闸门之后、Ready 之前求值
```

消灭「表外隐式优先级」。

### 3.5 熵预算门禁

新增 `kernel/hunter_v7_entropy_budget_test.go`：

| 指标 | 上限（本方案结束后） | 测量方式 |
|---|---:|---|
| `engine.go` 中 `func hunterV7(AltLadder\|Whale\|MMS).*` 复合门函数数 | ≤ 3（过渡期 ≤ 6） | AST/正则扫描 |
| 上述函数合计行数 | ≤ 80（过渡期 ≤ 200） | 同上 |
| `hunter_v7_tier_rules.go` 内字面量阈值（0.3x / zone 45 等） | **只允许出现在表字面量**，禁止出现在 helper 函数体 | 文件分区扫描 |
| 新增 setup 不得向 `engine.go` 增加 `hunterV7<Setup>*` 谓词 | 测试：diff 白名单 | 可选后续 |

---

## 4. 实施路线图（最小单元）

> 每个单元 = 可独立合并的一步。验收：`go test ./kernel/ ./provider/local/ -count=1` 相关包绿 + **provider/kernel golden 不变**（除非单元注明允许 alias 并更新 golden 且提交说明写明）。  
> 依赖：E0 → E1 → E2a/b → E3 → E4 → E5；E6 可与 E3 后并行。

### Phase E0 — 冻结与影子（必须先做）

| 单元 | 内容 | 验收 |
|---|---|---|
| **E0.1** | 编写本方案；在 `docs/` 落盘；completion 交叉链接 | 文档存在 |
| **E0.2** | 影子测试：对 golden fixture + 现有 alt_ladder / whale compact 测试候选，并行跑「旧 Guards」与「新声明匹配器（先空实现=委托旧函数）」——diff 必须为 0 | `TestHunterV7GateShadow_*` 绿 |
| **E0.3** | 熵预算测试以**当前值**为基线上限写入（先锁现状，防止继续恶化） | 超基线即红 |

### Phase E1 — 匹配器增强（行为零变更）

| 单元 | 内容 | 验收 |
|---|---|---|
| **E1.1** | 落地 `hunterV7ZoneGate` / `OIGate` / `StopGate` 字段 + `hunterV7TierRuleMatches` 求值；无表行使用新字段 | golden 不变；单测覆盖 known/unknown zone、缺 OI、stop 边界 |
| **E1.2** | 扩展 `ForbidAll` 已有能力文档化；若缺「任意命中即禁」则加 `ForbidAny`（仅当 soft-release hard-block 需要） | 单测；golden 不变 |

### Phase E2 — alt_ladder 表化（核心）

| 单元 | 内容 | 验收 |
|---|---|---|
| **E2.1** | soft-release R1/R2 落表；`LayeredReleaseOK/HardBlock` 改为薄包装调用同一匹配器（或删除，表直接表达）；保留影子 diff=0 | compact 测试中 soft-release 三案全绿；golden 不变 |
| **E2.2** | `AltLadderShortReviewableOK` 非 soft 分支表化；函数删除或 ≤15 行残留 | `engine.go` alt_ladder short 特例行数下降；影子退役本函数 |
| **E2.3** | `AltLadderLongExecutable` + late fresh-flow 表化；Ready 多行 | 同上；long 相关分类测试绿 |

### Phase E3 — whale-flow / MMS

| 单元 | 内容 | 验收 |
|---|---|---|
| **E3.1** | whale Ready 用 Zone+Taker 替换 `WhaleFlowLongEntryGatesOK`；删除函数 | trader 守卫仍为执行时权威；分类侧与守卫数值一致（单测钉死 45/0.56） |
| **E3.2** | MMS chase/fresh Guards 能表化则表化，否则保留 **一个** 具名 Gate 并计入熵白名单 | 预算测试更新白名单 |

### Phase E4 — EarlyWatch 收编

| 单元 | 内容 | 验收 |
|---|---|---|
| **E4.1** | `EarlyWatch` 列 + 迁移 extreme continuation / rebound pending | `classify*` 前段删对应 if；行为等价测试 |

### Phase E5 — 收紧预算与清理

| 单元 | 内容 | 验收 |
|---|---|---|
| **E5.1** | 熵预算从「基线锁」改为「目标上限」（§3.5）；删除死函数与重复字面量 | 预算绿；`gofmt`/`go vet` |
| **E5.2** | 更新 `hunter-v7-lean-core-completion-report` 或本方案附录「完成记录」；链到 design philosophy 公理四 | 文档一致 |
| **E5.3** | （可选）跑 `scripts/hunter_v7_full_chain_audit.sh` 或一轮 `hunter_v7_validate` 对比 tier 分布 | 与最近基线同量级；差异逐条记录 |

### Phase E6 — 防再堆叠（流程）

| 单元 | 内容 | 验收 |
|---|---|---|
| **E6.1** | 在 `agents.md` 或 `docs/hunter-v7-design-philosophy` 增补：**实盘补丁若改分类阈值，必须改表字段；禁止新增 `engine.go` hunterV7 setup 谓词**（白名单外 PR 拒） | 文档 + 预算测试即强制 |
| **E6.2** | soft-release / whale 数值若需策略变更：单独 PR，标题标明「行为变更」，禁止夹带结构重构 | 流程约定 |

---

## 5. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 表化丢失隐式优先级（soft vs close-through） | E0.2 影子全程；E2 按行迁移，每行独立提交 |
| `RequireKnown` 改变「区位缺失」语义 | soft-release 现码已要求 known；迁移时对照 `missing zone data blocks soft release` 测试钉死 |
| trader 与 kernel 门槛再漂移 | E3.1 单测共享常量 `hunterV7WhaleFlowLongMaxZonePos = 45` 等，trader 改读同一常量（若循环依赖则放 `provider/local` 几何包） |
| golden 误更新掩盖漂移 | 默认禁止 `-update-golden`；仅 alias 级差异允许，提交说明列 diff |
| 一次改太大 | 严格按 E0→E5 单元；任意中态可上线 |

回滚粒度 = 单元粒度。

---

## 6. 验证方案

1. **静态**：每单元 `go test ./kernel/ -count=1`；涉及 provider 时加 `./provider/local/`。  
2. **Golden**：`TestHunterV7GoldenReplay` + `TestHunterV7GoldenPrompt` 必须过。  
3. **影子**：E0.2–E2 期间新旧并行；diff≠0 失败。  
4. **定点**：`TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes` 下 soft-release / missing-zone 用例。  
5. **预算**：`TestHunterV7EntropyBudget`。  
6. **实盘（E5.3 可选）**：与 `reports/hunter-v7-*` 最近成功轮次比 SignalRate / tier 分布；本方案预期 **同量级、无系统性开仓率跳变**。

---

## 7. 预期收益

| 项 | 预期 |
|---|---|
| `engine.go` | −150～250 行（alt_ladder / whale / early-watch 特例迁出） |
| 调 soft-release | 1 处表字段，不再改 helper |
| 熵 | 复合门函数 ≤3；「轻」可测 |
| 风险 | 行为等价；不改变实盘策略意图 |

**不做行数崇拜**：允许规则表 +30～80 行声明数据；净收益看 `engine.go` 控制流与调参路径。

---

## 8. 本会话执行顺序（给续作 agent）

1. 落盘本文档（E0.1）✅  
2. 实施 E0.3 熵基线锁 + E1.1 匹配器字段（行为零变更）  
3. 实施 E2.1 soft-release 表化（核心可感收益）  
4. 能进则 E3.1 whale；否则停下并写完成附录  
5. 全程不改策略阈值；golden 红则修实现不改期望（除非可证明旧期望错误）

---

## 9. 附录：关键字面量清单（迁移核对表）

摘自现码，迁移后这些数字**只应出现在表字面量**（或共享命名常量）：

| 常量建议名 | 值 | 现用途 |
|---|---:|---|
| `AltLadderSoftMaxZonePos` | 45 | soft-release 区位 |
| `AltLadderSoftMinLiq` | 70 | soft-release 流动性 |
| `AltLadderSoftTakerStandard` | 0.38 | 标准卖流 |
| `AltLadderSoftTakerStrong` | 0.34 | 强卖流 |
| `AltLadderSoftStopStandard` | 2.25 | 标准止损宽 |
| `AltLadderSoftStopStrong` | 2.45 | 强流止损宽 |
| `AltLadderSoftMinOI1h` | 0.8 | 新空 OI |
| `AltLadderSoftHardTakerBuy` | 0.42 | hard-block 买盘反包 |
| `WhaleLongMaxZonePos` | 45 | whale LONG 区位 |
| `WhaleLongMinTaker` | 0.56 | whale LONG taker |

Hard-block tags：`high_volatility` / `extreme_volatility` / `alt_ladder_late_short_risk` / danger 族 / (`funding_elevated` ∧ `execution_stop_tightened`)。

---

## 10. 与 Lean Core 文档关系

- 本方案 = Lean Core 的 **Phase E（Entropy Recoil）**，不替代原 Phase 0–6。  
- 原推迟项 U5.1/U6.6 仍保持推迟。  
- 完成后应在 `docs/hunter-v7-lean-core-completion-report-20260726.md` 末尾追加「2026-08-13 Entropy Recoil」一节，或另建 completion 短报。

---

## 11. 本轮已实施记录（2026-08-13 会话）

| 单元 | 状态 | 落点 |
|---|---|---|
| E0.1 方案落盘 | ✅ | 本文档 |
| E0.3 熵基线锁 | ✅ | `kernel/hunter_v7_entropy_budget_test.go`（复合门 ≤16 / 行合计 ≤280；禁 soft-release 字面量回流 `engine.go`） |
| E1.1 Zone/OI/Stop + ForbidRiskAny + `confirmed_at_most` | ✅ | `kernel/hunter_v7_tier_rules.go` 匹配器；`hunter_v7_tier_gate_test.go` |
| E2.1 soft-release 表化 | ✅ | `altLadderShortSoftReleaseStandard/Strong` 有序 Reviewable 行；阈值常量化；`LayeredReleaseOK` 改为委托表匹配；classic 通道保留 |
| E3.1 whale LONG Zone+Taker | ✅ | Ready 分 LONG/SHORT 两行；删除表内 Guard 依赖 |
| 测验 | ✅ | `go test ./kernel/ ./provider/local/ -count=1` 全绿 |

**未完（续作）**：E2.2/E2.3（long executable / classic short 进一步表化）、E4 EarlyWatch、E5 收紧预算至目标上限、E6 流程写入 `agents.md`。
