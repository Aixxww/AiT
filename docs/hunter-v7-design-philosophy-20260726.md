# Hunter v7 筛选模式：设计哲学、模块逻辑与全链路原理

> 2026-07-26 | 基于精益内核重构完成版（含 U6.3/6.4 alias 期）的实地代码分析
> 使命：在币安 500+ 合约中，跨行情形态（主流币与 20%-300%+ 振幅山寨/meme），每轮筛出**当下可入场**的高胜率标的，并在数据→LLM→执行的 30-60s 滞后下保住开仓率与胜率。

## 1. 设计哲学：五条公理

**公理一：证据标签是子系统间的通用语言。**
模块不输出"结论"，输出**带语义登记的证据**（reason_codes/risk_tags/required_confirmations）。455 条目录 + 20 个可结算 confirm 码单一登记，每个标签有 source/category/polarity/llm_action 五维语义。信号的可信度由证据组合决定，任何一层（分类器/LLM/守卫）都能独立复核，而非信任上游结论。这就是"标签能被 LLM 有效识别"的根基：**识别的不是名字，是登记过的语义**。

**公理二：三个时钟，各自负责各自的新鲜度。**
- 信号时钟（采集时点）：模块打分、readiness、per-symbol 新鲜度戳；
- prompt 时钟（构建时点）：`hunterV7ApplyLiveConfirmations` 用最新收盘/资金流**重新结算** confirm 码，tier 重判——采集时"差一根 K线确认"的候选不会白白蹲在 REVIEWABLE；
- 决策时钟（下单时点）：trader 守卫链 Stage B 用 REST/orderbook 补证，铸造 `fresh_micro/rest_confirmed`。
30-60s 滞后不是被"容忍"，而是被三次重新结算**吸收**。实测两轮 P50=29-34s、0 stale。

**公理三：tier 漏斗保护开仓率，确认体系保护胜率。**
EXECUTABLE 必须先评（禁止全局 wait）→ REVIEWABLE 只允许"现场可复核的缺口"→ WATCH 只做背景 → REJECTED 禁入。开仓率靠三条升级通路托底：open-rate floor（分数+证据达标即可入复核池）、live-reviewable 升级（缺口仅剩"没人看最新收盘"时）、prompt 期实时确认升级（实测每轮 0→1、2→3 生效）。胜率靠反向闸门：hard confirms 未过不开、taker 阶梯弱流不单独成立、RR 后端 cap 复算、反追高/拥挤/过热 wait tag、逆势 regime 硬边界。

**公理四：调参是改数据，不是改代码。**
25 个 setup 的分级 = 规则表行；taker 打分 = 档位表行；管线顺序 = pass 切片；模块仪式 = scaffold。行情适应性变更（阈值/档位/顺序）不再触碰控制流，golden 双层逐字节回放保证任何数据变更的行为差异都显式化。

**公理五：宁可少开，不开脏单——但"脏"必须可证明。**
所有拒绝都有 tier_reason 码；WATCH/REJECTED 的整体缺失不得用作对 EXECUTABLE 的全局 wait 理由（prompt 政策第 3 条）。拒绝不可证明时（如 provider 未打 chase tag），kernel 不再自行重投（U3.5：provider 5 票版唯一权威）。

## 2. 功能模块逻辑：漏斗七段

```
523 合约
 │ ① 宇宙构建: 六池并集 (hot_alt 高振幅山寨 / core_liquidity 主流 / funding 费率极端
 │              / panic 恐慌 / squeeze 挤压 / new_activity 新活跃) ≈ 200-210
 │ ② 模块匹配: 21+ setup 模块, regime 权重 <0.2 跳过 + 干旱熔断器;
 │              Match 快筛 → Score 打分 (scaffold 仪式: 证据阶梯 + 几何 + finish 契约)
 │ ③ 增强管线: 16 道具名 pass —— regime 权重→流动性→强势币覆写→风险→多周期 TP
 │              →timing 助推→funding 快车道→板块轮动→共振→执行质量→新鲜度→AI 优先级→双过滤→readiness
 │ ④ 冲突消解 + LLM 过滤 + 预动雷达(watch) + 潜力池(审计用, 非生产)
 │ ⑤ tier 分类: 规则表 (Ready/NearConfirm/Reviewable/OpenRateFloor/PromptWait 五列)
 │              × 配置几何 (单一分类点, 构造时缓存 verdict)
 │ ⑥ prompt 装配: 实时价注入→live confirm 重结算→tier 重判→readiness→分层展开
 │              (EXECUTABLE/REVIEWABLE 全 JSON, WATCH 摘要行, REJECTED 只计数)
 │ ⑦ LLM 决策 → trader 守卫链 (A 纯校验: tier/tag/契约/新鲜度/必需确认
 │              → B 实时补证: REST+orderbook → C 显式应用: 铸码/缩仓) → 下单
```

各段职责边界的设计要点：
- **②与⑤分离**：模块只负责"这像不像一个 setup"（打分+证据），"现在能不能开"由规则表统一裁决。因此新增模块不会引入新的开仓逻辑分叉。
- **③的顺序即数据**：强势币覆写必须在流动性之后（读它）、执行质量定型必须在全部增强之后（读最终分）——这些约束现在显式写在 pass 切片的注释与顺序里。
- **⑦的 signal_id 契约**：LLM 开仓必须复制 `signal_id`，后端按 id/symbol/direction/setup/tier 结构化匹配——LLM 无法"发明"一个不存在的开仓对象，识别执行的闭环由此闭合。

## 3. 全链路标签识别与执行的保证机制

| 环节 | 机制 | 失效防护 |
|---|---|---|
| 发射 | 模块经 scaffold `add/reason/riskTag`（一律去重） | finish 契约检查：缺 zone/invalidation/targets/price_ctx 显式报警 |
| 登记 | 455 条目录 + confirm 联合表单一登记 | 覆盖测试：流通标签 ⊆ 注册表（扫描器识别 scaffold 惯用法）；未登记标签在 prompt 显示 unknown_context_only 并被测试拦截 |
| 词汇 | flow_taker_* 统一阶梯（U6.3），prompt 出口翻译 | alias 映射兜底：任何漏网旧名在 payload 边界仍被规范化 |
| 语义 | tag_semantics 随每个展开候选内联输出 | llm_action 五级（evidence/open_support/wait/reject/reduce）+ 阶梯审验规则写入 tag policy |
| 决策 | 分层漏斗政策 10 条（EXECUTABLE 优先、WATCH 禁做全局 wait、TP0 分批语义…） | 后端守卫链独立复核 LLM 输出：tier 门、tag action 门、必需确认、RR、zone 位置 |
| 执行 | fresh_* 码由守卫链 Stage C 单一铸造 | validate* 全部无副作用（U5.2），铸码点唯一可审计 |

## 4. 开仓率与胜率的对偶设计

**提高开仓率的机构**（防"该开不开"）：
1. prompt 期实时确认升级（吸收滞后，两轮实测生效）；
2. open-rate floor：证据+分数达标的候选保底进入复核池；
3. 反重复冷却豁免：EXECUTABLE ready 高分与 REVIEWABLE 高 timing 候选不被 anti-repeat 过滤误杀；
4. tag policy 明确"强流+确认通过→正常仓开仓"，禁止 LLM 用未列明计算的 rr_insufficient / 旧名混淆 / WATCH 池噪声制造假 wait；
5. 后端已验证 RR 优先于 LLM 臆造的结构 RR（政策第 7 条）。

**提高胜率的机构**（防"开出脏单"）：
1. taker 阶梯审验：中档必须现场 taker 复核、weak/neutral 不单独成立、逆向弱流减仓或 wait；
2. hard confirms 不可豁免；review confirms 只能由三个时钟之一真实结算；
3. 反追高家族（chase_high_protection / momentum_upper_zone_chase / 缓冲带 0.6x 仓位）；
4. regime 逆势硬边界 + 熔断器让干旱模块歇轮；
5. 守卫链几何复核（zone 位置、止损距离、OI flush、鲸鱼流零位）。

两者的张力靠**分级放行**调和：不是"开/不开"二元，而是 正常仓 → 保守仓（reduce_size）→ 现场复核后开 → wait 四档。

## 5. 当前状态与下一步

- alias 期周期 1（23:07）已实测：prompt 含 flow_ 标签与审验策略行，tier 行为不变。
- 周期 2 通过后执行**发射翻转**：模块发射、kernel 规则表、provider 内部匹配器原子切换到 flow_ 词汇（alias 映射保留为 payload 边界兜底一个会话，旧 catalog 条目删除）；golden 双层重录并逐条审。
- 注意边界：`provider/local/hunter.go` 为旧版猎手（独立词汇），不参与 v7 词汇翻转。
- 后续观测建议：以 `hunter_v7_signals` 存储的 30m/2h outcome 统计（`/hunter/v7/outcomes` 端点）按 setup×regime 跟踪翻转前后的 ExecRate 与 WIN_TP0/TP1 率，用真实成交回路验证"更高开仓率与胜率"，而非仅凭单轮 tier 分布。
