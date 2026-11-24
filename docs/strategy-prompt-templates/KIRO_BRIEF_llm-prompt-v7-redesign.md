# Kiro Dispatch Brief — LLM Strategy Prompt Redesign for Hunter v7 (current version)

Date: 2026-11-24. Base branch: `t2/v7-selection` @ `461074e8` (GitHub Aixxww/AiT).
Owner request: design the best-fit LLM strategy prompt for the CURRENT v7 pipeline, using six strategy templates as design input. The current prompt is a 2026-07-27 doctrine plus per-strategy prompt sections; both have been patched incrementally while setup names, regimes and registered terms iterated. Your job is to reconcile them into one coherent, professional, precise, concise prompt the LLM can read without contradictions — win-rate-oriented.

## 1. What you are designing

Two artifacts that together form the system prompt for every decision cycle:

(a) The five-segment doctrine in `kernel/hunter_v7_prompt_doctrine.go`
(`hunterV7DoctrinePrompt`, 2026-07-27 rewrite).
(b) The v7 strategy's `prompt_sections` (`role_definition` /
`trading_frequency` / `entry_standards` / `decision_process`) plus
`custom_prompt` — currently the values in `strategy-06` (the 2026-11-24
v7 export in this folder).

Assembly order in `kernel/engine_prompt.go`, `BuildSystemPrompt`:
schema (§0) → `role_definition` (~line 41) → mode variant →
hard constraints from `risk_control` → `trading_frequency` (~130) →
`entry_standards` (~142) → `decision_process` (~155) → output
contract → `custom_prompt` (~201) → hunter_v7 preflight doctrine
(~220, `writeHunterV7ExecutionPreflightPrompt` →
`hunterV7DoctrinePrompt(lang, maxTPMovePct, minSLMovePct, maxDriftPct,
minRR)`).

Required coverage: role definition, trading frequency, entry standards,
decision process, additional prompts (`custom_prompt`). Design goals
(owner's words): win-rate-oriented, professional, precise, concise,
readable by the LLM, zero internal contradictions.

## 2. Design inputs (in this folder)

- `strategy-01-oi-ai-trader-gpt-5-2.json` — OI AI Trader (GPT-5.2),
  2025-12-25: profit-oriented charter, stateless inference,
  market-environment priority rule, edge-persistence holding model,
  5-minute loop.
- `strategy-02-yao-bi-lie-shou.json` — 妖币猎手, 2026-05-25: altcoin
  momentum, Path A squeeze breakout / Path B dip-buy, 30-minute time
  stop, volume/OI/funding trap warnings, hard stop-loss formulas.
- `strategy-03-omni-game-theory-sniper-v3.json` — Omni-Market Game
  Theory Sniper V3, 2026-03-03: SMC + order flow + game theory,
  4 playbooks, regime TREND/EXTREME/CHAOTIC.
- `strategy-04-larry-williams.json` — Larry Williams, 2025-12-25:
  AI500 hot-coin scan, squeeze breakout, relative strength vs BTC,
  stop-loss = entry ± 2×ATR.
- `strategy-05-gaussian-grid-x5.json` — Gaussian Grid x5,
  2026-05-14: `strategy_type` is `grid_trading` — NOT a directional
  prompt. Borrow only generic discipline (e.g. the overtrading guard)
  if consistent; do not import grid mechanics.
- `strategy-06-hunter-v7-current.json` — Hunter v7, 2026-11-24: the
  live v7 export. This is the BASELINE you are improving, not one
  more equal option.

Note: staged copies are redacted — third-party API keys were removed
from source URLs. Do not restore or use those credentials; coin-source
URLs are not part of the prompt design.

## 3. Canonical ground truth — do not contradict

- Funnel: 500+ contracts → six-pool universe → 20+ setup modules →
  rule-table tiering → tiered expansion. The LLM audits
  EXECUTABLE/REVIEWABLE structured evidence into open / wait / close;
  it does not re-analyze the market.
- Tiers: EXECUTABLE / REVIEWABLE / WATCH / REJECTED. Funnel order:
  every EXECUTABLE first, then REVIEWABLE; WATCH is next-cycle trigger
  context only; REJECTED never participates. Only EXECUTABLE is
  immediately actionable; REVIEWABLE is a prepared candidate, not a
  market-entry permission.
- Setup families (doctrine §3 — the ONLY `setup_type` values the prompt
  may name): momentum continuation (`leader_momentum_long`,
  `displacement_momentum_long`, `mms_trend_ride_long`,
  `mms_squeeze_engine_long`, `intraday_scalp_long`); breakout
  (`trend_breakout_long`, `accumulation_breakout_long`,
  `volatility_squeeze_breakout`, `pre_breakout_watch`,
  `pre_squeeze_watch`); ladder (`alt_ladder_momentum_long`,
  `alt_ladder_breakdown_short`); reversal repair
  (`panic_reversal_long`, `pullback_reversal_long`,
  `whale_flow_reversal`, `funding_reversal`, `mms_bottom_wake_long`,
  `accumulation_watch`); weakness/distribution
  (`relative_weakness_short`, `distribution_short`,
  `breakdown_momentum_short`, `long_squeeze_short`,
  `range_reversion`, `range_expansion_event`,
  `pre_distribution_watch`).
- `market_regime` registered values: `trend_up`, `trend_down`,
  `range`, `trend_continuation`.
- `llm_action` five levels: `wait_only` / `reject_only` block
  opening; `reduce_size_or_wait` allows only conservative size after
  live confirmation; `reviewable_only_if_live_confirmed` requires a
  live re-check; `supports_open_after_core_checks` supports opening;
  `evidence_only` / `context_only` is evidence, not permission.
  `tag_semantics` is the ONLY label-semantics source; unknown tags are
  background only.
- Unified taker ladder: `flow_taker_buy_aggressive` > `strong` >
  `moderate`/`aligned` > `recovering` > `neutral` > `weak` (short
  side `flow_taker_sell_*` symmetric).
- Backend authority: `execution_contract` (`entry_permission`,
  `must_confirm`, `hard_blocks`, `size_policy`, `exit_policy`)
  overrides any numeric example in a family playbook. Three freshness
  clocks: scored at capture, re-settled at prompt build, re-verified
  by the backend before ordering. When
  `confirmation_summary.passed_review=true` and `rr/effective_rr ≥
  minRR`, backend RR is validated — do not invent a stricter
  structural RR.
- Hard numbers (live config, strategy-06): min RR **1.2**;
  `max_positions` **1**; `min_confidence` 75 configured but
  `effectiveMinOpenConfidence` caps hunter_v7 at **70**
  (`kernel/engine_prompt.go`); `v7_max_output` 18;
  `v7_min_ai_priority` 50; `max_entry_price_deviation_pct` 0.5;
  `max_single_trade_loss_pct` 6; `max_take_profit_price_move_pct`
  3.5. TP0 protocol: reduce 30–50% at `tp0_price`, trail the rest at
  breakeven or 5m EMA20/VWAP.
- Output contract (§5, validated by `kernel/engine_analysis.go`):
  `action` ∈ {`open_long`, `open_short`, `close_long`,
  `close_short`, `hold`, `wait`}; an open copies
  `hunter_v7_signal_json.signal_id` →
  `selected_hunter_v7_signal_id` plus tier/setup,
  `effective_rr_after_cap`, `signal_age_ms`; a wait carries EXACTLY
  ONE `blocked_reason_code` ∈ {`entry_not_in_zone`,
  `rr_insufficient`, `confirmation_missing`, `oi_too_low`,
  `funding_crowded`, `account_risk`, `backend_guard_risk`,
  `no_reviewable_candidate`}; `no_reviewable_candidate` only when
  the Tier Summary shows EXECUTABLE=0 and REVIEWABLE=0; a wait
  despite an open-review candidate must include
  `blocked_signal_symbol`; waits on `entry_trigger_near` /
  `entry_reclaim_wait` / `entry_pullback_wait` must include a trigger
  object (`trigger_price`, `required_close`, `expires_in_bars`,
  `action_if_triggered`), reusing `suggested_trigger` when present;
  all numbers must be computed values, not formulas.
- Prompt budget (measured 2026-11-24): system prompt 18,336 bytes
  (~4,584 tokens at chars/4, the codebase's own estimator); one
  full-context candidate ≈ 2,250 tokens; a 2-candidate round ≈
  10,000 tokens total. Keep the redesign within roughly the same
  budget — concise is a requirement, not a preference. Do not bloat
  the doctrine to "cover" the templates.

## 4. Conflicts in the templates — resolve, do not copy

| Conflict | Templates say | Current truth | Resolution |
|---|---|---|---|
| RR floor | 01 config 2.7 vs 01 text 1:2; 03: 2.5; 04: 2; 02: 1.5; 05: 3 | 1.2 (config + DB) | Use 1.2 as the floor. Keep per-setup effective-RR gates where code enforces them (e.g. displacement ≥ 1.5). |
| Max positions | 3 / 3 / 5 / 3 | 1 | Use 1. |
| Cadence | 01 "5-minute loop"; 03 "15-minute loop"; 05 "2–4 trades/day" | No fixed minute cadence exists in the decision path (`trader/` has no fixed decision interval) | Write cycle-based frequency rules. Do not hardcode a minute cadence. |
| Regime names | 01 TREND/RANGE/TRANSITION/CHAOTIC; 03 TREND/EXTREME/CHAOTIC | `trend_up` / `trend_down` / `range` / `trend_continuation` | Use registered values only. |
| Setup names | 03/04 "Long Squeeze / Short Squeeze / Volatility Squeeze Breakout" | Doctrine §3 families only | Use §3 `setup_type` values only. |
| Output format | 03/04 OPEN_LONG / OPEN_SHORT / WAIT / CLOSE_POSITION with free-form fields | §5 output contract (validated by the parser) | Use the §5 contract. |
| Grid mechanics | 05 is a grid strategy | Directional v7 prompt | Out of scope; generic discipline only. |

## 5. Deliverables

1. Revised `kernel/hunter_v7_prompt_doctrine.go` (ZH + EN semantic
   equals, same five-segment structure; if a cleaner structure is
   justified, say why, remove the old one, update all call sites).
2. Revised v7 `prompt_sections` + `custom_prompt` as a strategy JSON:
   `docs/strategy-prompt-templates/strategy-07-hunter-v7-redesigned.json`
   (same shape as strategy-06; only `prompt_sections` +
   `custom_prompt` change — no config numbers change unless the
   design note justifies it and the owner approves).
3. `docs/strategy-prompt-templates/REDESIGN-NOTES.md`: what was
   borrowed from each template, what was rejected and why, the
   conflict-resolution table, and the token-budget measurement
   (system prompt bytes before/after, per-candidate tokens).
4. Update the golden prompt test (`kernel/engine_prompt_golden_test.go`)
   if the prompt changes: re-record with the update flag and show the
   diff in the PR. Branch + PR per repo flow; report
   `go build ./...` and `go test ./...` per unit.

## 6. Constraints

- Prompt text only (+ the wiring needed to inject it). Do NOT change
  screening, router, tiering, or execution logic.
- No silent parameter tuning: thresholds change only via spec revision
  with stated evidence. Strategy parameters are CONFIG, never code
  literals.
- Every rule stated exactly once; ZH and EN must be semantic equals.
- No live-trading changes; no shadow phase (owner cancelled it).
  Offline evidence + code review is the acceptance path.
- If an input is missing, say so explicitly instead of reconstructing
  from memory.
