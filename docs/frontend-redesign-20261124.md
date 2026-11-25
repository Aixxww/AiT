# Frontend redesign: visual system, display names, info affordance, responsive rules (issue #81)

Date: 2026-11-24. Scope: presentation only. There are no backend, API, strategy-parameter or behaviour changes, and sorting and filtering semantics are unchanged.

This is PR 1 of a stacked series (see §7). It ships the system (tokens, components, display-name map, i18n scope) plus the two pages the owner named: Strategy Studio and the dashboard. It also covers the navigation, the traders page header and every Hunter v7 surface. Later PRs apply the same system to the remaining pages.

## 1. Design rationale

**Apple.com** contributes the layout rhythm:
- generous, consistent spacing;
- one dominant headline per section, with a short line of supporting copy and the detail one click away;
- quiet chrome: a translucent bar with `saturate(180%) blur(20px)`, hairline dividers and no decorative glow;
- spec-sheet "label over value" rows.

**Binance** contributes the data density:
- tabular monospace numerals;
- green and red only for direction and PnL;
- gold as the single accent for "act now";
- compact pills for state;
- dense tables that scroll horizontally on phones instead of collapsing data away.

The result keeps a trading terminal's information density while removing its noise. That noise included `SYSTEM_STATUS::ONLINE` strings, uppercase mono labels everywhere, coloured gradient icon tiles, blur blobs and nested borders.

## 2. Visual system (one system, on the existing CSS-variable tokens)

Everything lives in `web/src/index.css` under **"DESIGN SYSTEM LAYER"** and is exposed to Tailwind in `web/tailwind.config.js`. The four theme palettes (pro/glass × dark/light) stay the source of truth. The new tokens only *derive* from them, so no second design system exists and all four themes resolve automatically.

### Palette changes

These are value changes in the existing variables:
- **Pro dark** moves to Binance neutrals: background `#0b0e11`, surface `#14171c`, panel `#181c22`, and a hairline border at 7.5% white.
- **Pro light** moves to Apple neutrals: background `#f5f5f7`, foreground `#1d1d1f`, muted `#6e6e73`, and an 8% black hairline. The primary gold is `#c99400`, with dark text on gold for contrast.
- **Radii** move up one step: 6 / 8 / 12 / 16 px.

### Type roles

| Class | Size | Use |
|---|---|---|
| `t-display` | clamp 28–40px | Landing and hero only |
| `t-title` | clamp 20–26px | Page and section titles (one per card) |
| `t-headline` | 15px / 600 | Card headers, collapsible section titles |
| `t-label` | 14px / 550 | Field and row labels |
| `t-body` | 14px | Paragraphs, explanations |
| `t-caption` | 13px | One-line summaries, secondary values |
| `t-micro` | 11px | Fact labels, pills, timestamps |
| `t-num` | IBM Plex Mono, tabular | Every number that updates |

Rules:
- Sentence case everywhere.
- No uppercase or letter-spaced mono for labels. Mono is for numbers only.

### Spacing, elevation and radius

- **Spacing** uses a 4px base: 4 / 8 / 12 / 16 / 24 / 32 / 48 / 64.
- **Page gutter** is `clamp(16px, 2.5vw, 32px)`, applied through `.ait-page` and the header.
- **Elevation** has three levels:
  - `e1` cards (`.ait-card`);
  - `e2` popovers and menus;
  - `e3` dialogs.

  Nested surfaces use `.ait-card-inset`, which is flat, a different surface colour and a hairline, never a second shadow.
- **Radius roles:** `control` 8px for inputs and buttons, `card` 12px, `sheet` 16px, `pill` 999px.

### Colour discipline

- Gold (`primary`) is for the selected state and the primary action.
- Green and red are for direction and PnL only.
- Tier colours (gold, cyan, gray, dim) follow the existing tier contract.
- Everything else is foreground or muted on surfaces.

### Motion

framer-motion and CSS share one standard curve, `cubic-bezier(.2,.8,.2,1)`, at 120 / 200 / 320 ms. Motion is purposeful:
- the active navigation indicator (shared `layoutId`);
- menu and popover entry;
- the section chevron.

All durations drop to 0 under `prefers-reduced-motion`.

### Components added

| Component | File | Purpose |
|---|---|---|
| `InfoTip` | `components/ui/InfoTip.tsx` | Circled-info affordance (§4) |
| `SectionHeader` | `components/ui/SectionHeader.tsx` | Title + one-line summary + optional info + actions |
| `SwitchRow` | `components/ui/SwitchRow.tsx` | Setting row with a real `role="switch"`, summary and info |
| `.ait-chip`, `.ait-pill`, `.ait-segmented` | `index.css` | Selectable chips, state pills, segmented controls |
| `.ait-table-scroll` | `index.css` | Horizontal scroll with a sticky first column |
| `.ait-hover-reveal` | `index.css` | Hover-revealed controls, always visible on touch |

## 3. Display-name map

The map lives in one file, `web/src/i18n/displayNames.ts`, keyed per language (`zh`, `en`). Every surface calls `displayName(kind, code, lang)`, `setupLabel()` or `tagLabel()`.

| Kind | Source of truth (checked by `displayNames.test.ts`, which reads the Go files) | Example |
|---|---|---|
| Setup types (all 43) | `provider/local/hunter_v7_types.go`, `hunter_v7_t2_setups.go` | `momentum_continuation_long` → 动量延续 · 做多 / Momentum Continuation · Long |
| Families | doctrine §3 | `reversal` → 反转修复族 / Reversal Repair |
| Regimes (10) | `V7MarketRegimes()` | `trend_continuation` → 趋势延续 / Trend Continuation |
| Tiers | `V7Tier` | `REVIEWABLE` → 待复核 / Reviewable |
| Actions | `validActions` | `open_short` → 开空 / Open Short |
| Direction / side | — | `LONG` → 做多 / Long |
| `blocked_reason_code` (8) | `hunterV7BlockedReasonCodes` | `no_reviewable_candidate` → 无可复核候选 / No reviewable candidate |
| Source types | coin source enum | `hunter_v7` → 猎手 v7 / Hunter v7 |
| Tags (472 in the catalog) | `hunter_v7_tag_catalog*.go` | `confirmation_missing_15m_close_below_vwap` → 待确认：15分钟收盘低于 VWAP / Awaiting: 15m close below VWAP |
| Tag metadata | catalog `category` / `polarity` / `llm_action` | `wait_only` → 只能等待 / Wait only |
| Track status, data quality, signal class, RV24 tier, RR band | panel enums | `PROTECTED_STOP` → 保护止损 / Protected stop |

How tags are rendered:
1. Tags are too many to hand-write, so they resolve through phrase overrides first.
2. Then come the known prefixes (`confirmation_missing_`, `live_reviewable_`, …) and `_lt_N` / `_gt_N`.
3. Last is a word dictionary.

Timeframes render as 15分钟 / 15m. Market acronyms (OI, VWAP, EMA20, RSI, RR, LSR, BB, ATR, MMS) stay acronyms in both languages, as on Binance's Chinese UI.

The test enforces two properties for every catalog tag:
- no rendered label contains `snake_case`;
- no Chinese label contains an untranslated English word.

A new backend code therefore still renders readable text, and the test names any word the dictionary is missing.

**Rule for future code:** never render a backend code directly. Add it to the map, or let `humanizeCode` render it.

## 4. Info affordance pattern

The section header shows the title plus a one-line summary. The long explanation goes behind `InfoTip`, a circled `lucide` `Info` glyph next to the title, built on **Radix Popover** (`@radix-ui/react-popover`).

| Input | Behaviour |
|---|---|
| Mouse | Opens on hover after 120 ms and closes 160 ms after the pointer leaves the trigger and the panel. A click pins it open. |
| Touch / pen | Opens on tap and closes on a second tap, a tap outside, or Escape. On coarse pointers the glyph keeps its size and gets a 44px hit area. |
| Keyboard | Opens with Enter or Space; Escape closes it; focus returns to the trigger. |

`InfoTip` can also wrap a custom trigger. Tag chips (`VetoChip`) use this, so tapping a chip shows its catalog explanation; before, it was a native `title` that phones never show.

Applied in this PR to:
- the Strategy Studio page header;
- the Hunter v7 coin-source block;
- the Task-2 module;
- the screening switches;
- tag chips;
- the Square Heat error detail.

Hover-only controls are covered too. The strategy list's export / duplicate / delete actions use `.ait-hover-reveal`, so they are always visible on touch devices.

## 5. Responsive rules

Breakpoints are checked at 360 / 768 / 1280 / 1920. Rules:
1. **No horizontal page overflow.** The screenshot harness asserts `scrollWidth === clientWidth` on every capture; 24 of 24 passed.
2. **Multi-pane tools collapse to one column below `xl`.** In Strategy Studio:
   - the strategy list becomes a horizontal scroller on phones and tablets, and a 240px column from `lg`;
   - the editor and the preview share one column behind an **Edit / Preview & test** segmented control below `xl`, and appear side by side from `xl` (the preview pane is 420px).

   Before this change the page used fixed 192px + 420px columns, which at 360px clipped the editor out of view entirely.
3. **Grids step down.** 4 columns become 2 and then 1. Examples: source types use 2 / 3 / 4 columns; risk-control, indicator and stat cards go from 2 columns to 1 below `sm`.
4. **Tables never hide data.** Below `md`, dense tables become `.ait-table-scroll` with a sticky first column. The dashboard positions table previously *hid* 7 of 11 columns on phones.
5. **Touch targets** are at least 44px on coarse pointers (chips, segmented controls, menu rows).
6. **Mobile navigation:** a full-height sheet with large rows, a zh/en segmented switch, and theme / logout / settings / mode buttons. Page scroll is locked while it is open.

## 6. i18n scope: zh + en

- `Language = 'en' | 'zh'`, in one place (`i18n/translations.ts`). To add ko/ja later, extend the type, add a dictionary block, and add a row to `LANGUAGES` in `HeaderBar.tsx`.
- **Removed:**
  - the Indonesian dictionary (1,477 lines);
  - the `id` options in HeaderBar, Header and LanguageSwitcher;
  - the SetupPage `id` labels;
  - the context check.

  The dead Spanish (`es`) entries in `strategy-translations.ts` (262 entries plus `esStrategy`) were also removed, since no UI path could select them.
- `t()` now falls back to English for a key missing in Chinese, instead of showing the key path.
- Hardcoded `language === 'zh' ? … : …` strings moved into the dictionaries in the files touched here (CoinSourceEditor, HeaderBar, SignalPanel, DecisionCard, TraderDashboard, SquareHeat, AITraders header). The remaining CoinSourceEditor ternary only maps the prop to the `Language` type.

## 7. Stacked series and what is deliberately unchanged in PR 1

**PR 1 (this PR):**
- the system;
- the display-name map;
- zh/en only;
- HeaderBar;
- Strategy Studio (page, CoinSourceEditor, responsive Indicator and Risk grids);
- the dashboard (header, status strip, stat cards, positions, decisions, Square Heat);
- the Hunter v7 signal and cell panels;
- DecisionCard;
- PositionHistory sides;
- the traders page header.

**PR 2 and later:**
- Settings and the model config modals, traders lists and modals, backtest, strategy market, data, FAQ, competition, auth and 404 pages.
- The remaining terminal-style literal strings found by grep: `RegisterPage` (`NEW_USER`, `CREATE_ACCOUNT`), `PageNotFound` (`SIGNAL_LOST`), `StrategyMarketPage` (`PUBLIC_ACCESS`, `NO_INDICATORS`), and the landing `TerminalHero`.
- Their info affordances.
- Settings tabs, which currently wrap at 1280px.

The "PR 2 and later" list is split into two stacked PRs:

- **PR 2 (pages):** Settings (tabs, all tab bodies), the traders page config grid, strategy market, data, FAQ, auth (login, register, reset password, sign-in overlay, onboarding mode selector), 404, landing `TerminalHero`, and the `PositionHistory` "per page" leftover from PR 1. Screenshots: `docs/frontend-redesign/screenshots/pr2/`.
- **PR 3 (modals and lists):** `ModelConfigModal`, `ExchangeConfigModal`, `TelegramConfigModal`, `TraderConfigModal`, `TradersList`, backtest (raw `action` and coin-source codes), competition, and the transport-encryption notice in `WebCryptoEnvironmentCheck`.

**Unchanged on purpose:**
- The tier colour contract.
- The chart components (lightweight-charts / recharts theming).
- The glass theme's own radius scale.
- Prompt text, logs and JSON shown verbatim in `<pre>` blocks and decision execution logs (marked `data-raw`), because they are data, not labels.
- The trader id shown as a `title` tooltip.
- Exchange-native symbols (BTCUSDT).

## 8. Verification

- `npm run build` (tsc + vite): green.
- `npm test` (vitest): 18 files and 200 tests pass. New: `displayNames.test.ts`. Updated: the SignalPanel and CellsPanel tests now assert user-facing labels and tap-to-open explanations.
- `npm run lint` (eslint with prettier): 0 problems. The 12 prettier errors that predated this work, in auth and settings files, were fixed by formatting only.
- **Raw-identifier grep over rendered strings** comes from the screenshot harness: a TreeWalker over all text nodes plus `title`, `aria-label`, `placeholder` and `<option>`, excluding `<pre>`, `<code>` and `[data-raw]`.
  - Before: dashboard 28 codes, Strategy Studio 16.
  - After: Strategy Studio 0. Dashboard 0 apart from the trader id in a `title` attribute.
  - /traders, /settings, /backtest, /strategy-market, /competition, /data and /faq: 0.
- **Screenshots** are in `docs/frontend-redesign/screenshots/{before,after}/`. Before: dashboard and Strategy Studio at 1280 and 360, light and dark. After: the same, plus 768, 1920, English, and the info affordance opened by tap at 360.
- **Screenshot data:** auth, strategies and the tag catalog come from a local backend. Dashboard trader, account, positions, decisions and signals are illustrative fixtures injected at the network layer, the same for before and after. No live account was used.
