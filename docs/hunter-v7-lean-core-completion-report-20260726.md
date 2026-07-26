# Hunter v7 精益内核重构 — 完成报告

> 日期：2026-07-26 | 方案：`docs/hunter-v7-lean-core-redesign-20260726.md`
> 提交范围：`ffa4bb6..HEAD`（46 个提交，均可独立 revert）
> 全程硬门禁：golden 双层（provider 信号层 + kernel prompt 层）逐字节不变；每单元 `go build/vet/test ./...` 全绿。

## 1. 完成状态总览

| Phase | 单元 | 状态 |
|---|---|---|
| 0 安全网 | U0.1 golden 回放 / U0.2 kernel prompt golden | ✅（前序会话） |
| 1 词汇内核 | U1.1-U1.8 | ✅（前序会话） |
| 2 几何原语 | U2.1-U2.3（U2.4 按记录并入 Phase 3） | ✅（前序会话） |
| 3 分类器收敛 | U3.1-U3.6 全部 | ✅ 本轮 |
| 4 序列化 | U4.1-U4.3 全部 | ✅ 本轮 |
| 5 结构收尾 | U5.2/U5.3/U5.4/U5.5 | ✅ 本轮 |
| 5 结构收尾 | U5.1 | ⏸ 推迟（见 §4） |
| 6 provider 收敛 | U6.1/U6.2（18 模块全部）/U6.5 | ✅ 本轮 |
| 6 provider 收敛 | U6.3/U6.4/U6.6 | ⏸ 推迟（见 §4） |

## 2. 本轮关键成果

### Phase 3 — 分类器规则表化
- **U3.1**：分类唯一化 + 修复真实 bug——候选构造点与 trader 反重复过滤此前用默认几何分类，prompt 路径用配置几何；RiskControl 非默认时同一候选两侧 tier 漂移。现构造点为唯一分类点（配置几何），三处 fallback 改读缓存，几何必传，validate CLI 对齐。回归测试钉死。
- **U3.2**：`hunterV7TierRule`/`hunterV7SetupTierSpec` 规则表 + 求值器；影子模式 = 4 个 legacy switch 冻结副本 × (golden 语料 + 56 万确定性阈值跨骑网格)；灵敏度自检证明 off-by-one 阈值必被抓（首版网格漏抓后重建为逐维扫描）。
- **U3.3a-p**：25 个 setup 全部迁入表（16 个提交，每步影子 diff=0）。调阈值=改表中一个数字；新 setup=表一行。`ReasonFunc` 行支持动态 reason。
- **U3.4**：实盘门禁（13 信号/0 格式问题/tier 分布与基线一致）通过后退役影子；求值器语义测试保留（风险边界含/不含、未知流动性放行、taker 缺数据 pass/fail 两种语义、ReasonFunc 落空续走）。
- **U3.5**：删除 kernel 漂移的 3 票 upper-chase 重投，provider 5 票版唯一权威（行为变更显式化，SKYAI fixture 双向钉死）；`PromptSemanticWaitReason` 并入表 `PromptWait` 列。
- **U3.6**：72 个纯分类测试 → 6 张表 90 行（candidateBuilder + 23 个 mutator）；转换用 DeepEqual 保真校验 90/90；测试文件 3773→2773 行，覆盖率 49.3% 持平。

### Phase 4 — 三编码单源
- `V7PromptPayload`（53 字段，tag 与字段序冻结）成为全量 JSON / 紧凑 JSON（声明式 mask）/ 执行紧凑文本的唯一来源。TP0 在文本编码保留 raw 语义（payload 版含 micro-TP0 fallback），差异就地注释。
- `PromptCompactMode` 坍缩为 bool 语义（`PromptCompactEnabled`），存储仍为 string 兼容旧配置；kernel/store 两个重复解释器与死掉的 "auto" 体积启发式删除。

### Phase 5 — 结构收尾
- **U5.2**：trader 守卫链三段化。两个 `validate*` 改造为纯 `verify*`；`fresh_micro_confirmed`/`fresh_rest_confirmed` 统一由 `hunterV7RefreshEvidence.applyTo` 显式铸造；`capHunterV7LowLiquidityPosition` 标注为唯一其余变异点。
- **U5.3**：导出 `kernel.AssembleHunterV7CandidateCoins`，validate CLI 删除已漂移的 64 行手工拷贝（缺 V7VWAP15m、缺 Long/Short 分数镜像）。
- **U5.4**：pnl_tracker(813 行)+dynamic_stop→`trader/`；matrix_report→`api/`；**删除** RegimeAdaptiveEngine（断裂反馈环：权重只进 HTTP dry-run 响应，router 永远读静态矩阵）与孤儿 CLI mover_audit(483 行)；`AssessV7OIAccumulation` 去导出；potential_pool 标注非生产消费。
- **U5.5**：20 个可结算 confirm 码单一登记（`v7ConfirmVocab` 联合表：结算 flags + 语义定义同行），init 派生结算注册表并回填 catalog。范围校正：U1.8 之后 catalog 本身已是 455 条注册表，真实重复仅这 20 码。

### Phase 6 — provider 收敛
- **U6.1**：`v7SignalScaffold`——8 字段头、add/reason/riskTag（一律 appendIfMissing）、zoneATR/zonePad、invalidate/target、finish 五步收尾 + **输出契约检查**（缺 zone/invalidation/targets/price_context 显式触发 hook）。
- **U6.2**：18/18 模块迁入（模块层 5326→4662 行）。诚实的部分迁移并留痕：whale_flow 动态方向翻转、range_expansion 的 timing 后置降级、funding_reversal 的 clamp 后惩罚与方向分支 zone、alt_ladder 的预算分助手等保留手写。tag 覆盖扫描器学会 scaffold 惯用法（断言不变、提取更强）。
- **U6.5**：RouteDetailed 的 16 道逐信号增强 pass → 具名 `[]v7SignalPass`（顺序即数据）；AIPriority 三写合一为单一 pass。

## 3. 量化收益（ffa4bb6..HEAD，剔除 testdata）

| 指标 | 前 | 后 |
|---|---|---|
| Go 代码净变化 | — | −629 行（5058+/5687−），其中测试 −216 |
| kernel/engine.go | 3527 | 3144（分类逻辑已大半移入 623 行规则表） |
| provider 模块层 | 5326 | 4662 |
| 分类器测试文件 | 3773 | 2773（97 函数→31，其中 6 张表 90 行） |
| 高风险重复实现 | 4 处分类点×2 套几何、3 票/5 票漂移对、3 套序列化、64 行 CLI 拷贝、双 confirm 登记 | 归零 |
| 覆盖率 | kernel 49.3% | kernel 49.5%、provider 49.9%、trader 32.1%（不降） |

注：净行数低于方案预估 −4750，主因是 U3.2/U6.1 新增了规则表、scaffold、payload 等结构资产（约 +960 行），以及 §4 推迟单元未计入。结构性目标（重复归零、调参=改数据、新 setup=表一行、新模块≈scaffold 骨架）均已达成。

## 4. 推迟单元与理由（已用户裁决）

- **U6.3/U6.4（taker 词汇合一 + 档位数据化）**：方案自带 alias 过渡期约束（"两个实盘周期后才删旧名"），本质是跨会话渐进变更，也是唯一允许 golden 标签级差异的单元。待实盘周期配合时执行；届时路径：`TakerBand` 入 `V7SetupThresholds` 扩展 → scaffold `takerLadder` → `tag_semantics` 双名映射 → 两个实盘周期后删旧名。
- **U5.1/U6.6（结构分组 + TP 平铺去重）**：实地核查推翻方案前提——`V7TakeProfitPlan` 仅含 TP0Price/TP0DistancePct，"12 个重复 TP 平铺字段"并无重复存储；真去重需 TP 阶梯重设计并改变 prompt 字节布局，属新设计决策。Phase 4 的 payload 单源化已消除这两单元针对的序列化漂移风险，剩余收益仅命名分组。

## 5. 验证记录

- 每单元：`go build ./... && go vet ./... && go test ./...` + golden 双层。
- Phase 3 出口实盘门禁：`reports/hunter-v7-phase3-gate-20260726/`（13 信号、EXECUTABLE=2/3、REVIEWABLE=2、WATCH=5/4、REJECTED=4、0 格式问题，与 20:40 基线同量级）。
- 终审：`scripts/hunter_v7_full_chain_audit.sh --live` **AUDIT PASSED**（22:46）——gofmt/build/vet/golden×2/全量测试/覆盖率全绿 + 实盘 validate 轮（`reports/hunter-v7-audit-20260726-224523/`）：16 信号覆盖 10 类 setup、0 格式问题、0 执行缺口；tier 分布 EXECUTABLE=1 / REVIEWABLE=1 / WATCH=10 / REJECTED=4，与基线同量级。**runtime 与 prompt-final tier 分布首次完全一致**（基线时代两口径存在差异）——U3.1 几何统一修复的实盘直接证据。
