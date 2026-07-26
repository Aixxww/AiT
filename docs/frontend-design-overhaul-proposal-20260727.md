# AiT 前端设计美学改造方案（Frontend Design Overhaul Proposal）

- **日期**: 2026-07-27
- **范围**: `web/` 全部前端（React 交易操作台，服务 :3000）
- **性质**: 只读审阅产出的改造方案。本文档不伴随任何代码修改（web/ 目前有另一分支在做 lint 清理）。
- **审阅方法**: 全量静态扫描（160 个 TS/TSX 文件，约 43,000 行；`index.css` 1,891 行）+ 关键路径精读（`TraderDashboardPage`、`DecisionCard`、`StrategyStudioPage`、`CoinSourceEditor`、`PositionHistory`、`index.css`、`tailwind.config.js`、`themeStore.ts`）。

---

## 0. 结论摘要

AiT 前端**已经有一套相当完整的设计 token 基础设施**（CSS 变量 → Tailwind 语义类映射，四种主题模式，语义类使用达 1,313 处），这在同类项目中并不常见，是本次改造最大的资产。但它被三个问题严重稀释：

1. **三套视觉语言叠加共存**：Pro（Binance 金）、Glass（Apple 液态玻璃）、Cyber（青色霓虹 + 网格 + 扫描线），靠 CSS 选择器优先级互相竞争，其中 Cyber 皮肤大部分是死代码但其背景网格/渐变仍然全局生效——产品没有一个明确的视觉人格。
2. **token 体系被大面积绕过**：509 个硬编码 hex 色值、282 处内联 rgba()、339 处 Tailwind 原始调色板类（`text-red-400` 等）、917 处 `style={{}}` 内联样式、418 处向后兼容 `ait-*` 别名。token 存在，但约四成的颜色决策没有经过它。
3. **组件层完全缺位**：`components/ui/` 只有 3 个组件（input/select/alert-dialog），**没有 Button 组件**——232 个裸 `<button>` 各写各的类；CSS 里的 `.btn-*` 工具类在 TSX 中几乎无人使用（仅 `btn-icon` ×2），且 `.btn-outline` 在 `index.css` 中被定义了两次、样式互相冲突。

对照 Binance / TradingView / Bloomberg 级专业交易终端的标准，当前 UI 的主要美学债务是：**装饰过载**（霓虹辉光、CRT 扫描线、emoji 图标、卡片弹跳动效）、**数字排版不专业**（`tabular-nums` 全库仅 4 处、价格居中对齐、正文字体被设成等宽字体）、**语义色滥用**（涨跌绿红被挪用于成功/失败、置信度分档），以及**微字号泛滥**（≤12px 的字号使用约 700 处，含 `text-[8px]`）。

方案分三阶段：**P0 定语言、清 token**（4–6 人日）→ **P1 看板信息层级重构 + Hunter v7 信号四级 tier 色彩体系**（6–9 人日）→ **P2 组件库收敛 + 防回退护栏**（8–12 人日，可渐进）。每阶段附文件级落点与验收标准（§6）。

---

## 1. 技术栈确认（可实施性前提）

来源：`web/package.json`、`web/tailwind.config.js`、`web/src/index.css`、`web/src/stores/themeStore.ts`

| 项 | 现状 |
|---|---|
| 构建 | Vite 6 + TypeScript 5.8 + React 18.3 |
| 样式 | **Tailwind CSS 3.4**（`darkMode: ['selector', '[data-theme="dark"]']`）+ 单一全局 `index.css`（1,891 行），无 CSS Modules |
| Token | `index.css` 内 CSS 变量（`--color-*`）→ `tailwind.config.js` 语义色映射（`profit/loss/warning/info/primary/surface/panel/...`）✅ 已 token 化 |
| 组件库 | 无成品库。仅 `@radix-ui/react-alert-dialog` + `react-slot`；`class-variance-authority`、`tailwind-merge`、`clsx` 已安装（shadcn 式基建齐备，但只建了 3 个组件） |
| 图标 | `lucide-react`（68 个文件引用）——但同时 27 个文件、164 处直接用 emoji 当图标 |
| 图表 | `lightweight-charts` 5 + `recharts` 2；`utils/chartTheme.ts` 已消费 `--chart-*` token ✅ |
| 通知 | `sonner`，经 `lib/notify.ts` 统一封装 ✅（confirm 走 Radix AlertDialog） |
| 动效 | `framer-motion` + 自定义 keyframes（scan/glitch/shimmer/float） |
| 主题 | `themeStore.ts`：`pro/glass × dark/light` 四模式 + system，写 `data-theme` / `data-style` 到 `<html>` ✅ |
| i18n | 手写 `t(key, language)`，`en/zh/id` 三语（`i18n/translations.ts` 4,162 行 + `strategy-translations.ts` 914 行） |
| 字体 | Google Fonts `@import`（Inter + IBM Plex Mono，`index.css` 第 1–2 行）——render-blocking 外链 |

**结论**：改造不需要引入任何新依赖。CVA + tailwind-merge + Radix 已在依赖树里，P2 组件收敛可以直接按 shadcn 模式落地；P0/P1 纯粹是把已存在的 token 用起来。

---

## 2. 设计语言现状盘点（数据）

### 2.1 三套皮肤叠加：主题架构的结构性问题

`index.css` 中主题变量分四层定义，靠优先级竞争：

| 层 | 选择器 | 视觉语言 | 生效情况 |
|---|---|---|---|
| L1 | `:root`（13–105 行） | Cyber 缺省值（`#35e6ff` 青、`#ff4fd8` 粉） | 仅在主题属性未挂载的瞬间生效 |
| L2 | `[data-theme='dark'][data-style='pro']` 等 4 块（1113–1367 行） | Pro = Binance 金 `#f0b90b`；Glass = Apple 液态玻璃 | **优先级最高（0,2,0），实际生效的色板** |
| L3 | "AIT CYBER SKIN OVERRIDES" `[data-theme='dark']` / `[data-theme='light']`（1369–1468 行） | 青色霓虹全套色板 | 被 L2 压制，**绝大部分是死代码**；只有 L2 未定义的 `--app-background`（三层渐变）与 `--cyber-grid-opacity`（全局网格）泄漏生效 |
| L4 | 兼容别名 `[data-theme]`（1494–1527 行） | `--ait-gold`、`--binance-yellow` 等旧名 | 全部转发到 L2 变量 |

**后果**：即便用户选了 "Professional Dark"，页面底下仍铺着 Cyber 皮肤的青粉渐变 + 38px 霓虹网格（`body::before`，198–213 行），上面再叠 Binance 金的面板——两种世界观同屏。此外 `::selection` 是与任何主题无关的橙色 `rgba(255,88,0,.3)`（216–219 行）。

其它全局装饰（与"专业终端克制感"直接冲突）：
- `.crt-overlay`（CRT 扫描线）、`animate-scan`/`animate-glitch`/`shadow-neon` 等赛博效果（`tailwind.config.js` + `index.css` 107–180 行）；
- `DeepVoidBackground`（`components/common/DeepVoidBackground.tsx`）给看板再叠 surface/grid/scan 三层背景；
- 全局 `button:active { transform: scale(0.98) }`（444–447 行）——所有按钮一律弹跳，包括平仓这类高风险操作。

### 2.2 硬编码色值统计（token 旁路面）

统计口径见附录 A。均为 `web/src` 下 `*.ts/tsx`：

| 指标 | 数量 | 说明 |
|---|---:|---|
| hex 色值字面量 | **509 处 / 49 个文件** | Top：`#2B3139`×35、`#F0B90B`×30、`#0ECB81`×19、`#F6465D`×13——**恰好是 Binance 调色板本体被绕过 token 直接写死**，另有 `#A78BFA`×15、`#60A5FA`×26（大小写两种）等一次性紫/蓝 |
| 内联 `rgba()/rgb()` | 282 处 | 多为 `rgba(240,185,11,…)` 之类 token 已有等价物的重复 |
| Tailwind 原始调色板类 | **339 处** | `red`×70、`green`×56、`zinc`×35、`gray`×33、`yellow`×28、`emerald`×26、`blue`×25、`orange`×16、`purple`×15、`fuchsia`×8… 其中 emerald/green 混用、gray/zinc 混用，同一语义两套灰 |
| 语义 token 类（对照组） | 1,313 处 | `text-profit`、`bg-panel` 等——**主干是健康的** |
| 兼容别名 `ait-*` 类 | 418 处 | `ait-gold/ait-bg/ait-text/...`；**其中 `bg-ait-green`、`text-ait-red` 系列在 `tailwind.config.js` 中根本未定义**（只有 `ait-success/ait-danger`），属静默失效的死类——如 `TraderDashboardPage.tsx:400` 交易员在线状态点的绿色背景实际没有渲染，仅靠 box-shadow 撑着 |
| `style={{ ... }}` 内联样式 | **917 处** | `PositionHistory` 30 处、`TraderDashboardPage` 12 处、`DecisionCard` 几乎全部配色走内联 |
| 任意值 `[...]`（hex/px/rgba） | 318 处 | 含 16 种一次性字号（`text-[52px]`、`text-[27px]`…） |

### 2.3 已确认的具体缺陷（P0 应顺手修复）

1. **失效的边框着色**（`components/trader/DecisionCard.tsx:103,123,136`）：`` border: `1px solid ${config.color}33` `` 中 `config.color` 是 `'var(--color-profit)'`，拼接产物 `var(--color-profit)33` 在 computed-value 阶段非法，**整条 border/背景声明被浏览器丢弃**——LONG/SHORT 卡片的语义色边框实际从未渲染。
2. **嵌套 button**（`DecisionCard.tsx:431–487` 等三处折叠区）：复制/下载 `<button>` 嵌在折叠切换 `<button>` 内部，非法 HTML，键盘/读屏行为不可预期。
3. **`.btn-outline` 双重定义**（`index.css:486` vs `:846`）：前者灰字灰框、后者金字金框，后者靠源码顺序获胜——前者是死代码。
4. **未定义即使用的类**：`bg-ait-green`、`text-ait-red`（`TraderDashboardPage.tsx:400,764`、`SquareHeatPanel.tsx:23,204`）。
5. **正文字体是等宽字体**：`:root` 的 `font-family` 以 `'IBM Plex Mono'` 开头（`index.css:89–95`），与 `tailwind.config.js` 中 `font-sans: Inter` 冲突——所有未显式声明 `font-sans` 的正文都在用 mono 排中文/英文段落，中文回退到系统字体，视觉割裂。
6. **Google Fonts 外链 `@import`**（`index.css:1–2`）：render-blocking，且部署环境若在中国大陆访问不稳定，字体失败时 mono 数字排版全面回退。
7. **主题修补 hack 段**（`index.css:1701–1718`）：`[data-theme] .text-muted-foreground.opacity-50 {...}` 系列选择器强行覆盖 utility 的透明度——这是 token 对比度没调好之后打在消费端的补丁，是"token 失去权威性"的直接证据。

### 2.4 组件一致性（变体蔓延统计）

| 组件类别 | 现状 |
|---|---|
| 按钮 | 无 `ui/button.tsx`。232 个裸 `<button>`，类名组合各异；CSS 端 `.btn-primary-glow/.btn-success/.btn-danger/.btn-outline(×2)/.btn-binance` 五套定义几乎无人消费（TSX 中仅 `btn-icon` ×2） |
| 面板/卡片 | 三套玻璃面板并存：`.glass`（blur 20px）、`.glass-panel`（blur 12px）、`.ait-glass`（blur var）+ 各页面手写 `style={{ background: 'linear-gradient(...)' }}` 渐变面板（如 `DecisionCard.tsx:100–105`、`TraderDashboardPage.tsx:380`） |
| 圆角 | `rounded`×331、`rounded-lg`×252、`rounded-full`×151、`rounded-xl`×118、`rounded-2xl`×26 混用；且 radius token 有三套冲突定义（Pro 主题块 4/6/8/12px → `[data-style='pro']` 又覆盖为 6/8/10/14px → glass 8–24px） |
| 图标 | lucide（68 文件）与 emoji（164 处/27 文件：📈💰🤖📋💾⏸️❌…）双轨。emoji 跨平台渲染不一致、不可着色、不可对齐，是专业终端观感的最大杀手之一 |
| 表单 | `ui/input.tsx`、`ui/select.tsx`（AiTSelect）已统一 ✅，但 Strategy Studio 各 Editor 中仍有大量手写 `<input className="...">` |
| Toast/确认 | `lib/notify.ts` 统一封装 ✅（现状良好，保持） |
| 字号 | `text-xs`×520、`text-sm`×376、`text-[10px]`×143、`text-[11px]`×24、`text-[9px]`×11、`text-[8px]`×2 + 16 种一次性任意字号。**≤12px 合计约 700 处**——不是"信息密度高"，而是层级失控后所有东西一起缩小 |
| 数字排版 | `font-mono` 233 处（好），但 `tabular-nums` 全库仅 4 处（2 个文件）；价格列多处 `text-center`（如 DecisionCard 交易明细格），不符合数字右对齐惯例 |

---

## 3. 信息架构与数据密度评估

### 3.1 交易员看板（`pages/TraderDashboardPage.tsx`，1,137 行）

结构：Trader 头卡 → 4×StatCard 账户概览 → 双列（左：图表 Tabs + 当前持仓表；右：AI 决策流）→ 延迟挂载的历史面板。

- **扫读效率**：持仓用 `<table>` ✅ 选型正确；但整表 `text-xs`，表头与数据同灰度区间，且 side 徽章 `text-[10px]` + 发光 shadow，关键列（uPnL）与次要列（杠杆）字重相同——眼睛没有着陆点。
- **StatCard**（1071–1130 行）：数值 `text-2xl font-bold font-mono` 是对的；但右上角 emoji 水印（`opacity-5`、hover 变彩色）、hover 上浮 `translate-y-[-2px]`、`hover:bg-white/5` 都是消费级 dashboard 的手势。**账户权益是操作台的锚点数字，它不应该动。**
- **实时性可感知**：`lastUpdate` 字符串 + `animate-pulse` 圆点。没有"数值变化闪烁"（flash-on-change）、没有 stale 数据警示（连接断开 60s 后看板与正常时无任何视觉差别）——这在实盘操作台上是**风险级**缺口，不只是美学问题。
- **决策流（DecisionCard，726 行）**：卡片流选型正确（决策是叙事性、异构内容，不适合表格）。问题在卡内：
  - 折叠区标题色 `#a78bfa`（紫）/`#60a5fa`（蓝）硬编码，与任何主题 token 无关；
  - 置信度分档用 profit/loss 色（≥80 绿 / ≥60 金 / 其余红）——**把涨跌语义色挪用为质量分档**，与旁边真正的 LONG/SHORT 色互相污染；
  - 成功/失败状态点也用 profit/loss 绿红（`:143–150`）——同一屏上绿色同时表示"做多""执行成功""高置信度"三种语义；
  - 价格数字 `text-center`、无 `tabular-nums`，SL/TP 百分比与价格字号相同。

### 3.2 策略工作室（`pages/StrategyStudioPage.tsx` 1,308 行 + `strategy/*` 7 个 Editor）

- 深度表单场景，`CoinSourceEditor`（1,563 行）内部 source type 图标色全部硬编码（`#F0B90B/#0ECB81/#A855F7/#E879F9/#22D3EE`…九种），mixed 模式摘要字符串里混 emoji（`🔥SQ`、`🎯HN↑`）。
- 表单控件已部分统一（AiTSelect），但分区标题、开关、数字步进、标签 chip 每个 Editor 自成一体。**表单一致性是"配置正确性信心"的来源**——专业用户在配几十个阈值时，控件行为的可预测性直接影响出错率。
- `TokenEstimateBar`、`GridRiskPanel` 等反馈组件是好的 IA 实践，保留并 token 化即可。

### 3.3 表格 vs 卡片选型准则（P1 落地依据）

| 内容 | 现状 | 建议 |
|---|---|---|
| 当前持仓 / 历史成交 | 表格 ✅ | 保持表格；提升数字列排版（右对齐 + tabular-nums + 语义色仅作用于 PnL 列） |
| AI 决策流 | 卡片 ✅ | 保持卡片；卡内建立"行动行 → 参数网格 → 折叠证据链"三级结构 |
| 账户概览 | StatCard ✅ | 保持；去装饰、加 delta 徽章 |
| 未来 v7 信号面板 | （未建） | **表格/密集列表**，不是卡片——信号是同构、可比较、需要扫读排序的数据（见 §5） |

---

## 4. 专业交易终端美学基准对照

以 Binance（作战密度）、TradingView（图表克制）、Bloomberg Terminal（信息权威感）为参照系：

| 原则 | 基准做法 | AiT 现状 | 差距 |
|---|---|---|---|
| 暗色为主、低饱和底 | 近黑冷灰底（Binance `#181A20`、TV `#131722`），面板与底色差 ΔL 很小 | Pro Dark 底色 `#0a0d10` ✅ 本身合格，但被 Cyber 渐变背景 + 霓虹网格覆盖 | 移除装饰层即达标 |
| 语义色只给涨跌/风险 | 绿红只出现在价格变动、PnL、买卖盘 | 绿红同时用于成功/失败、置信度、在线状态 | 需要语义色使用守则 |
| 品牌色克制 | Binance 金只出现在 CTA 与高亮 | `--color-primary` 金被用于 CLOSE 动作、置信度中档、边框 hover、扫描线… | 收敛到 CTA/选中态/tier-1 |
| 等宽数字 | 所有价格/数量 tabular，右对齐，小数位按 tick size 固定 | `font-mono` 覆盖尚可但 `tabular-nums` 仅 4 处；对齐混乱 | P0 全局解决 |
| 密度靠层级不靠缩字号 | Bloomberg 全大写 label 9–10px + 数据 12–13px，两级分明 | ≤12px 约 700 处、且 label 与数据同级 | P1 定义双层密度刻度 |
| 动效仅服务数据变化 | 价格变动 flash（背景 200–400ms 衰减），无卡片位移 | 卡片 hover 上浮/缩放/发光遍地，数据变化反而无反馈 | P1 反转：删装饰动效、加数据动效 |
| 图标系统单一 | 单套线性图标 | lucide + emoji 双轨 | P1 统一 lucide |

---

## 5. 目标设计语言定义

### 5.1 一个决定：Pro 是唯一权威语言

**Professional Dark 定为产品默认与设计基准**；Pro Light 保留（合规/白天场景）；Glass 降级为"实验性外观"不再投入维护，或直接删除（推荐删除，减少 4 套色板 ×2 皮肤的测试矩阵）；**Cyber 皮肤层（`index.css:1369–1468` + 网格/扫描线/CRT/glitch 全套）整体移除**。赛博元素若要保留品牌记忆点，只允许留在 Landing 页（`components/landing/` 自成一体，不影响操作台）。

### 5.2 Token 规范化（在现有 `--color-*` 上收口，不推倒重来）

- 色板保持 Pro Dark 现值（`#f0b90b / #0ecb81 / #f6465d / #0a0d10` 系）——它已经是 Binance 级配色，问题从来不是色板本身；
- 新增缺失的 token：`--color-neutral-badge`（HOLD/WAIT 用，替代硬编码 `rgba(132,142,156,.15)`）、`--color-purple`/`--color-blue`（DecisionCard 折叠区、CoinSourceEditor 源类型等"分类色"需求，给 2–3 个受控的分类色而不是放任 15 种紫蓝）；
- radius 收敛为一套：`4/6/8/12px`（面板 8、控件 6、chip 4、模态 12），删除 `[data-style]` 的二次覆盖；
- 字号刻度收敛为 7 级：`10(label)/11(dense-data)/12(body-dense)/13(body)/14(emphasis)/18(section)/24(hero-number)`，禁止新的任意值字号；
- 数字排版基线：`.tabular` 工具类（`font-variant-numeric: tabular-nums`）挂到所有价格/数量/百分比，表格数字列一律右对齐；
- 正文字体修正：`:root` font-family 改为 Inter 优先，mono 仅经 `font-mono` 显式使用；字体 self-host（`@fontsource/inter` + `@fontsource/ibm-plex-mono` 或 `public/fonts/` + `font-display: swap`）。

### 5.3 Hunter v7 信号面板：四级 tier 色彩体系（P1 核心新增）

前端目前没有任何 `EXECUTABLE/REVIEWABLE/WATCH/REJECTED` 的表示（全库 grep 无命中），这是从零定义的机会。**关键设计决策：tier 是"信号质量分层"，不是方向也不是盈亏——因此 tier 色严禁使用涨跌绿红**。方向（LONG/SHORT）作为独立的绿/红小徽章叠加在 tier 行上，两套语义正交，互不污染。

tier 用"亮度 + 品牌色阶梯"表达注意力优先级：

```css
/* 提案：加入 index.css Pro Dark 块 */
--tier-executable:        var(--color-primary);            /* #f0b90b 金——唯一实心高亮 */
--tier-executable-bg:     rgba(240, 185, 11, 0.12);
--tier-reviewable:        var(--color-accent);             /* #00d4e8 青——次级，需要人看 */
--tier-reviewable-bg:     rgba(0, 212, 232, 0.10);
--tier-watch:             var(--color-muted-fg);           /* #8b95a5 灰——观察中 */
--tier-watch-bg:          rgba(139, 149, 165, 0.08);
--tier-rejected:          var(--color-disabled-fg);        /* #4a5568 暗灰——已否决 */
--tier-rejected-bg:       transparent;                     /* 无底色，仅描边/删除线 */
```

呈现规则：
- **EXECUTABLE**：金色左边框(2px) + 金 bg 徽章 + 数值全亮——整屏唯一"喊你行动"的颜色；
- **REVIEWABLE**：青色描边徽章，无底色浸染；
- **WATCH**：灰徽章，行内容正常亮度；
- **REJECTED**：整行前景降为 `--color-disabled-fg`，veto 原因码（如 `fresh_oi_absent`、`mms_weak_continuation_review_only`）以 `font-mono text-[11px]` 灰 chip 展示——**否决原因是 v7 精益内核的一等公民数据，必须可见而非隐藏**；
- 面板形态：密集表格（列：symbol / 方向徽章 / tier / RR / zone% / 确认码 chips / 时间），tier 可排序可过滤，行高 32–36px，默认按 tier→时间排序；
- 与后端对齐：确认码前缀族（`live_confirmed_*` 等）直接以 chip 呈现原始码，hover 出目录释义（对接 455 词条确认码目录）。

### 5.4 语义色使用守则（写入文档与 lint 规则）

| 颜色 | 允许的语义 | 禁止 |
|---|---|---|
| profit 绿 / loss 红 | 价格涨跌、PnL、LONG/SHORT 方向 | 成功/失败、置信度、在线状态、tier |
| primary 金 | CTA、选中态、EXECUTABLE tier | 普通信息强调、CLOSE 动作 |
| warning 琥珀 | 风险提示、stale 数据、降级运行 | — |
| info 蓝 | 中性信息、链接 | — |
| 成功/失败（操作结果） | 用 toast/勾叉图标 + 中性色文本 | 占用绿红 |

---

## 6. 三阶段实施路线

> 前置协调：**web/ 当前有大范围 lint 清理未提交（git status 显示 30+ 文件 modified）。P0 必须等该分支落地后开工**，否则全是冲突。以下工作量按 1 名熟悉本库的工程师估算。

### P0 — 设计语言定夺 + Token 收口（4–6 人日）

| # | 事项 | 文件落点 | 说明 |
|---|---|---|---|
| 0.1 | 移除 Cyber 皮肤层与全局装饰 | `index.css`（:root 缺省值改为 Pro Dark 值；删 1369–1468 行 override 块、`body::before` 网格、`.crt-overlay`/`.tech-border`/scan/glitch/neon 相关段）；`tailwind.config.js`（删 scanlines/glitch/neon 扩展）；`components/common/DeepVoidBackground.tsx`（退化为纯容器或删除，9 处调用点同步） | Landing 页若要保留赛博风，把这些样式搬进 `components/landing/` 局部作用域 |
| 0.2 | 字体修正 + self-host | `index.css:1–2,89–95`、新增 `@fontsource` 依赖或 `public/fonts/` | 正文 Inter、`font-mono` 显式化；顺带修 `::selection` 用 `--color-primary-dim` |
| 0.3 | 清 509 hex / 339 raw-palette / 418 `ait-*` | 49 个含 hex 的文件；重点：`DecisionCard.tsx`、`CoinSourceEditor.tsx`、`PositionHistory.tsx`、`ModelConfigModal.tsx`、`chartTheme.ts` 白名单除外 | 机械替换到语义 token；emerald→profit、red→loss、zinc/gray→muted 系;`ait-*` 别名全部替换后**删除 tailwind.config.js 兼容段与 index.css L4 转发段** |
| 0.4 | 修 §2.3 缺陷清单 | `DecisionCard.tsx:103,123,136`（改用 `color-mix()` 或预置 `-border` token）、`:431–487`（嵌套 button 拆为 div[role] + 独立按钮）、`index.css:486`（删重复 `.btn-outline`）、`bg-ait-green` 等死类 | 每项都是确定性 bug，随清理顺手修 |
| 0.5 | 数字排版基线 | `index.css` 加 `.tabular` 基类；`utils/format.ts` 集中价格格式化（DecisionCard 里的本地 `formatPrice` 合并进去） | 全库价格/数量挂 tabular-nums + 右对齐 |
| 0.6 | radius/字号刻度收敛声明 | `index.css` token 段 + 本文档 §5.2 作为规范 | 本阶段只定规范并处理新增代码，存量 `rounded-xl` 等在 P2 迁移时顺带收敛 |

**验收标准**：
- `grep -rE '#[0-9a-fA-F]{3,8}' web/src --include='*.tsx' --include='*.ts'` 命中数从 509 降到 ≤ 30（白名单：`chartTheme.ts`、品牌/交易所 logo 色、`ExchangeIcons`/`ModelIcons`）；
- raw palette 类命中从 339 → 0；`ait-*` 类从 418 → 0 且配置中别名删除；
- 四主题模式（或决定删 Glass 后的两模式）截图对比：核心四页（Dashboard/Studio/Market/Agent）无回归；
- Lighthouse：字体外链请求为 0。

**风险**：替换量大且机械，易碰 lint 分支冲突（已通过排期规避）；Glass 删除需产品确认（若保留，验收矩阵 ×2）。

### P1 — 看板信息层级重构 + v7 tier 体系（6–9 人日）

| # | 事项 | 文件落点 | 说明 |
|---|---|---|---|
| 1.1 | tier token + `SignalTierBadge`/`VetoChip` 组件 | `index.css`（§5.3 token）、新建 `components/hunter/SignalTierBadge.tsx`、`components/hunter/VetoChip.tsx` | 先于面板落地，作为 v7 信号面板的设计合同；Storybook 缺席时在 `pages/` 加一个 dev-only preview 路由 |
| 1.2 | Hunter v7 信号面板骨架 | 新建 `components/hunter/SignalPanel.tsx`（密集表格，§5.3 呈现规则），接入点 `TraderDashboardPage` 或独立路由 | 与后端 v7 契约对齐（confirmation codes / RR / zone%）；本期可先接 mock |
| 1.3 | StatCard 重做 | `TraderDashboardPage.tsx:1071–1130` | 去 emoji 水印/hover 位移；结构：label(11px uppercase muted) → 数值(24px mono tabular) → delta 徽章（仅 PnL 用语义色）；加 stale 态（数据 >60s 灰化 + warning 边） |
| 1.4 | DecisionCard 信息层级 | `DecisionCard.tsx` 全文件 | 行动行（symbol + 方向徽章 + 置信度**中性灰阶**）→ 参数网格（右对齐 mono，SL/TP 色仅作用于数值本身）→ 证据链折叠区（lucide 图标 + token 色替代 `#a78bfa/#60a5fa`，emoji 全部移除）；成功/失败改为图标 + 中性文本 |
| 1.5 | 持仓/历史表格排版 | `TraderDashboardPage.tsx:681–` 持仓表、`PositionHistory.tsx:832–` | 表头 10px uppercase / 数据 12px 两级；数字列右对齐 tabular；PnL 列唯一彩色列；side 徽章去 glow |
| 1.6 | flash-on-change 数据动效 | 新建 `hooks/useFlashOnChange.ts`，应用于 StatCard 数值、持仓 uPnL、信号面板价格 | 背景色 300ms 衰减（profit/loss 10% 透明度），替代被删除的装饰动效，建立"实时感" |
| 1.7 | 图标统一 | 27 个含 emoji 文件（重点 `DecisionCard`、`StatCard`、`CoinSourceEditor` 摘要串、`translations.ts` 里的文案 emoji 酌情保留） | UI 结构性图标一律 lucide；文案性 emoji（toast 内）允许保留 |

**验收标准**：
- 信号面板以 mock 数据渲染四 tier 各 ≥1 行，截图评审通过"30cm 扫读测试"（1 秒内定位唯一 EXECUTABLE 行）；
- Dashboard 关键数字全部 tabular + 右对齐（抽查截图叠栅格）；
- UI 图标 emoji 命中（结构性位置）为 0；
- 断流 60s 后看板呈现可辨识的 stale 态。

**风险**：DecisionCard/表格是高频页面，重构需灰度（保留旧组件一个迭代，路由开关切换）；tier 色需与 Hunter v7 后端产出字段最终对齐（EXECUTABLE 判定口径以 kernel 为准，前端不做二次判定）。

### P2 — 组件库收敛 + 防回退护栏（8–12 人日，可渐进/穿插）

| # | 事项 | 文件落点 | 说明 |
|---|---|---|---|
| 2.1 | `ui/button.tsx`（CVA） | 新建；variants: `primary/secondary/ghost/outline/destructive` × `sm/md`；`danger` 动作（平仓/删除）强制 `destructive` | CVA + tailwind-merge 已在依赖中；同时删除 `index.css` 全部 `.btn-*` 死类与全局 `button:active` transform |
| 2.2 | `ui/panel.tsx`、`ui/badge.tsx`、`ui/stat.tsx`、`ui/data-table.tsx` | 新建；Panel 吸收 `.glass/.glass-panel/.ait-glass` 三合一 | Badge 吸收全库 40+ 处手写 pill；DataTable 定死表头/行高/数字列规范 |
| 2.3 | 232 个裸 button 渐进迁移 | 全库，按页面分 4 批（Dashboard → Studio → Modals → 其余） | 每批一个 PR，配截图 diff |
| 2.4 | 删除 `index.css` 修补 hack 段与冗余层 | `index.css:1601–1718`（`.bg-background` override、`text-muted-foreground.opacity-*` 补丁）等 | 组件收敛后这些补丁的存在理由消失；目标 index.css 从 1,891 行降到 ≤ 900 行 |
| 2.5 | 防回退护栏 | `eslint.config.js`：`no-restricted-syntax` 禁止 JSX 中新 hex 字面量;禁用 raw palette 类的自定义规则（或 `eslint-plugin-tailwindcss` 的 `no-arbitrary-value` 白名单制）；PR 模板加设计检查项 | 没有护栏，509 个 hex 两个月就会长回来 |
| 2.6 | 表单控件补全 | `ui/` 增 `switch/stepper/chip-input`，Strategy Studio 各 Editor 迁移 | 深表单一致性收尾，工作量最大但可无限期分批 |

**验收标准**：
- `components/ui/` ≥ 8 个基元组件，Dashboard + Studio 两页裸 `<button>` 为 0；
- `index.css` ≤ 900 行且无 `.btn-*`/hack 段；
- lint 护栏上线后新增 PR 无法引入 hex/raw-palette（CI 红灯验证一次）；
- 全库 `style={{}}` 从 917 降到 ≤ 300（图表/动态定位类豁免）。

**风险**：迁移周期长，需接受"新旧并存"的中间态；建议每批迁移绑定业务迭代顺带做，不单开大分支。

---

## 7. 工作量与优先级总览

| 阶段 | 人日 | 用户可感知收益 | 依赖 |
|---|---|---|---|
| P0 | 4–6 | 视觉噪音大幅下降、主题一致、数字排版专业化、7 个实际 bug 修复 | lint 分支合并 |
| P1 | 6–9 | 看板扫读效率、实时感、v7 信号面板设计合同就绪 | P0；v7 后端字段契约 |
| P2 | 8–12（可分批） | 长期一致性、开发提速、防回退 | P0（P1 可并行） |

**若只能做一件事**：P0.1 + P0.3（删 Cyber 层 + token 收口）。它以最小代价把产品从"三种风格打架"变成"一个克制的专业终端"，并为 v7 信号面板提供干净的落地面。

---

## 附录 A：统计口径（可复现）

均在 `web/src` 下执行，2026-07-27 快照：

```bash
# hex 字面量（509 / 49 文件）
grep -rEoh '#[0-9a-fA-F]{3,8}\b' --include='*.tsx' --include='*.ts' . | wc -l
# 内联 rgba（282）
grep -rEo 'rgba?\(' --include='*.tsx' --include='*.ts' . | wc -l
# raw palette 类（339）
grep -rEoh '(text|bg|border|ring|from|to|via|shadow|fill|stroke)-(slate|gray|zinc|...|rose)-[0-9]{2,3}(/[0-9]+)?' --include='*.tsx' . | wc -l
# 语义 token 类（1313）
grep -rEoh '(text|bg|border)-(profit|loss|warning|info|primary|muted|surface|panel|foreground|background)\b' --include='*.tsx' . | wc -l
# ait-* 别名（418）
grep -rEoh 'ait-(gold|bg|accent|text|success|danger)[a-z-]*' --include='*.tsx' . | wc -l
# 内联样式（917）
grep -ro 'style={{' --include='*.tsx' . | wc -l
# 裸 button（232）
grep -ro '<button' --include='*.tsx' . | wc -l
# emoji（164 / 27 文件）
grep -rP '[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]' --include='*.tsx' -o . | wc -l
# tabular-nums（4 处 / 2 文件）；字号直方图见 §2.4
```

## 附录 B：本次审阅精读文件清单

- `web/tailwind.config.js`、`web/src/index.css`、`web/src/stores/themeStore.ts`、`web/package.json`
- `web/src/components/trader/DecisionCard.tsx`、`web/src/pages/TraderDashboardPage.tsx`（含 StatCard）
- `web/src/components/strategy/CoinSourceEditor.tsx`、`web/src/pages/StrategyStudioPage.tsx`（结构）
- `web/src/components/trader/PositionHistory.tsx`（表格段）、`web/src/components/common/DeepVoidBackground.tsx`
- `web/src/lib/notify.ts`、`web/src/components/ui/{input,select,alert-dialog}.tsx`、`web/src/utils/chartTheme.ts`（引用确认）
