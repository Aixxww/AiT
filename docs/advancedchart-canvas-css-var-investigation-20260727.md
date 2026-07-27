# AdvancedChart: canvas "直传 CSS 变量" 疑似 bug 排查 (证伪 + 记录)

- 日期: 2026-07-27
- 起源: `docs/frontend-design-overhaul-proposal-20260727.md` P1 收尾备注
  > AdvancedChart canvas 传 CSS 变量的非法色排查，修复或证伪并记录。
- 结论: **疑似 bug 为误报**。canvas 路径上的 `var(--color-*)` 颜色在浏览器实际运行中可被正确解析并绘制，原因见下文。
- 并存隐患（已在本提交修掉，**非 canvas 路径**）: 一处 CSS-in-JS 字符串拼接 `var(--color-accent) + '26'` 产出非法 CSS，背景色静默丢弃。已改写为合法 `color-mix(...)`。

## 1. 涉及代码

`web/src/components/charts/AdvancedChart.tsx` 用 `lightweight-charts` v5.1 渲染 K 线。它把 **裸 `var()` 字符串**直接交给库的 color 选项，例如：

```ts
// createChart  (layout/grid/crosshair/priceScale/timeScale)
background:  { color: 'var(--chart-bg)' }
textColor:   'var(--chart-text)'
grid.vertLines.color = 'var(--chart-grid)'
crosshair.vertLine.color = 'color-mix(in srgb, var(--color-primary) 50%, transparent)'

// addSeries Candlestick
upColor: 'var(--color-profit)'
downColor: 'var(--color-loss)'

// volume bars / order markers / price lines
color: 'var(--color-profit-border)'
markers:  { color: 'var(--color-profit)' }   // 红绿买卖点
priceLine: { color: 'var(--color-loss)' }     // 止损线
```

直觉上的"bug": canvas 2D 的 `ctx.fillStyle` 不解析 CSS `var()`，把这些字符串喂给 canvas 会得到非法色，图表可能整片黑/透明。**但库并不直接喂给 canvas。**

## 2. 证伪: `lightweight-charts` 的颜色解析路径

库内 `ColorParser._private__parseColor`（`node_modules/lightweight-charts/dist/...development.js`）在 JS 侧把任意颜色字符串先行解析为 `[r,g,b,a]` 数组，再以数值画到 canvas；**不把字符串原样透传给 `ctx.fillStyle`**。其唯一的浏览器入口:

```js
function getRgbStringViaBrowser(color) {
  const element = document.createElement('div')
  element.style.display = 'none'
  document.body.appendChild(element)        // ← 挂到 <body>，不是图表容器
  element.style.color = color                // = 'var(--color-profit)' 等
  const computed = window.getComputedStyle(element).color
  document.body.removeChild(element)
  return computed                            // 形如 'rgb(14, 203, 129)'
}
```

要点:
- `getComputedStyle(el).color` 在 **DOM 层** 解析 `var()`，继承 `:root` / `html` 上定义的自定义属性。
- `--color-*` 与 `--chart-*` 在 `web/src/index.css` 全部定义在 `:root` 与 `[data-theme][data-style]`（仍是 `html`/`:root` 范畴，全局可继承）。`element` 被显式 `appendChild(document.body)`，因此能拿到解析后的 `rgb(...)`。
- 解析后 `ColorParser` 缓存 `rgba` 四元组，后续传给 canvas 全是数值，`fillStyle` 永远拿到合法颜色。
- `color-mix(...)` 同路径也能被 `getComputedStyle` 解析为 `rgb(...)` (提议 P0 的 crosshair/水印用的就是它，与本文同一套机制)。

故**canvas 直传 `var()` 在浏览器实际运行下不会出现非法色**，疑似 bug 不成立。对照文件 `web/src/utils/chartTheme.ts` 之所以用 `getCSSVar()` 提前解析，是早期为兼容非 `:root` 作用域 / SSR / jsdom 等"无法继承"场景而做的保险写法；二者并不冲突，AdvancedChart 当前写法在运行期可正常工作。

## 3. 风险与已知边界 (记录, 不视为 canvas bug)

1. **jsdom / 无 DOM 环境**: `getRgbStringViaBrowser` 依赖 `document.body` + `getComputedStyle`。在 jsdom 单测若未注入 CSS 变量，`var()` 会解析为空串，触发 `ColorParser` 抛 `Failed to parse color`。当前无相关 AdvancedChart 单测,故无伪性回归;若未来加 AdvancedChart 单测,需在 setup 里给 `documentElement` 写入 `--color-profit` 等,或走 chartTheme.ts 的预先解析口径。
2. **作用域漂移**: 若未来将 `--color-*`/`--chart-*` 从 `:root` 下沉到仅图表容器内的子选择器,且容器未把变量再下发到 `<body>`，则 `getRgbStringViaBrowser` (挂在 body) 会拿不到值。**保持颜色变量在 `:root`/`[data-theme*]` 全层定义即可。**
3. **`lightweight-charts` 升级** 若改回直接透传字符串给 canvas (历史 v3 曾用过更便宜的解析), 本结论失效; 版本绑定为 5.x。

## 4. 同时修掉的并存隐患 (非 canvas)

`AdvancedChart.tsx` 指标开关按钮在 active 态使用:

```ts
background: showIndicatorPanel ? 'var(--color-accent)' + '26' : 'transparent'
```

`'var(--color-accent)' + '26'` 拼成字符串 `"var(--color-accent)26"`,**非法 CSS**(函数记号后直接拼字面数字既不是 `color-mix` 也不是 8 位 hex)。浏览器丢弃该声明,active 按钮实际无背景着色,视觉上"按下没回响"。`#RRGGBB + '26'` 这种 8 位 hex 写法只对纯色字面量成立,对 `var()` 函数值不成立。

修复(本提交):

```ts
background: showIndicatorPanel
  ? 'color-mix(in srgb, var(--color-accent) 15%, transparent)'
  : 'transparent'
```

`26/255 ≈ 15%`，用 `color-mix(in srgb, ... 15%, transparent)` 以合法语法表达同等的半透明 accent 背景，与本仓库 P0 收尾的其它 `color-mix` 调色一致 （ESLint 设计令牌护栏白名单含 `AdvancedChart.tsx`，此改动走的是护栏明文支持的合法表达,不触发护栏)。

全文检索确认该 `'var(--x)' + 'DD'` 拼接模式在仓库 `.tsx` 中仅此一处; 无兄弟修复点。

## 5. 复核口径 (重新审计时如何快速独立验证)

1. 在浏览器 DevTools 中,把 `AdvancedChart` 容器内任一 canvas 暂停/截图,验证 K 线红绿与网格色匹配 `--color-loss` / `--color-profit` / `--chart-grid` 而非纯黑。
2. 触发 `getRgbStringViaBrowser` 的等价调用:`document.body.appendChild((e=document.createElement('div')),e.style.color='var(--color-profit)',getComputedStyle(e).color)` → 应返回 `rgb(14, 203, 129)` (取自 `--color-profit: #0ecb81`)。
3. `npx eslint src/components/charts/AdvancedChart.tsx --max-warnings 0` → 0 错误 (本文件在护栏白名单内,且改动不引入 hex/palette/ait-* alias)。

## 6. 处置决议

- canvas 疑似 bug: **证伪，不改组件**，机理与边界记录于本文。
- 并存 alpha 拼接隐患: **已修复** (color-mix 15%), 走合法语义表达, 绿色护栏通过。
