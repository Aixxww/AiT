# Hunter v7 LLM strategy prompt redesign (issue #67)

Date: 2026-11-24. Base: `kiro/llm-prompt-v7-redesign` @ `d6b4f07b` (which is `t2/v7-selection` @ `461074e8` plus the brief and templates).

Deliverables:

| # | Artifact | File |
|---|---|---|
| 1 | Five-segment doctrine, ZH + EN | `kernel/hunter_v7_prompt_doctrine.go` |
| 2 | Redesigned `prompt_sections` + `custom_prompt` | `docs/strategy-prompt-templates/strategy-07-hunter-v7-redesigned.json` |
| 3 | These notes | `docs/strategy-prompt-templates/REDESIGN-NOTES.md` |
| 4 | Golden updates | `kernel/testdata/hunter_v7_prompt_legacy_{en,zh}.golden` (doctrine), `kernel/testdata/hunter_v7_prompt_hybrid_v2_{en,zh}.golden` (the `%%` fix only) |

Only prompt text and the wiring that injects it changed. Screening, router, tiering, execution and every config value are unchanged. `TestRedesignedTemplateChangesPromptTextOnly` checks that strategy-07 equals strategy-06 except for `prompt_sections` and `custom_prompt`.

Merging changes the system prompt of every running hunter_v7 strategy on the legacy doctrine. It does not change what any strategy is allowed to trade. When to deploy is the owner's call. The brief sets the acceptance path as offline evidence plus code review; there is no shadow phase.

## 1. Token budget

Method: `BuildSystemPrompt(1000, "")` for the strategy JSON, zh and en, with T2 enabled as in strategy-06. Tokens use the codebase estimator (bytes / 4). `TestStrategyTemplateSystemPromptDump` reproduces the numbers with `PROMPT_DUMP_DIR=<dir> go test ./kernel/ -run TestStrategyTemplateSystemPromptDump -v`.

| | zh bytes | en bytes | zh ~tokens | en ~tokens |
|---|---|---|---|---|
| Before: base code + strategy-06 | 22,173 | 23,146 | 5,543 | 5,786 |
| After: new code + strategy-07 | 20,689 | 21,705 | 5,172 | 5,426 |
| Change | -1,484 (-6.7%) | -1,441 (-6.2%) | -371 | -360 |
| Data dictionary, unchanged | 2,976 | 2,968 | | |
| Doctrine + T2 notes, before / after | 9,477 / 9,387 | 10,458 / 10,411 | | |

The doctrine stays about the same size. It now also carries the displacement review rule, which moved out of the strategy sections. The savings come from removing duplicates: strategy sections and doctrine, Hard Constraints and doctrine S4, Output Format and doctrine S5.

Per candidate: the user prompt is unchanged. `kernel/testdata/golden-prompt-20260726.txt` (2 full-context candidates, 9 WATCH, 2 REJECTED) is byte-identical, and `TestHunterV7GoldenPrompt` passes untouched. One full-context candidate therefore stays at about 2,250 tokens, as in the brief.

The brief quotes 18,336 bytes for the 2026-11-24 system prompt. I could not reproduce that figure from the repository. The closest number is 19,197 bytes for the zh prompt without the data dictionary. All comparisons here use the method above, on the same inputs before and after.

## 2. Where each rule lives (stated once)

| Block (assembly order) | Owner | Holds |
|---|---|---|
| Data dictionary (§0) | code, unchanged | field meanings |
| `role_definition` | strategy JSON | charter: input-only decisions, the three parts every trade needs, wait is a valid result |
| Hard Constraints | code | the limits (max positions, RR floor, drift, stop minimum, TP cap, feasible geometry, confidence) |
| `trading_frequency` | strategy JSON | cycle rules, with no minute cadence |
| `entry_standards` | strategy JSON | global hard boundaries and candidate priority / tie-break |
| `decision_process` | strategy JSON | per-candidate check order, with the blocked_reason_code for each check |
| Output Format | code, unchanged | the parser contract (actions, fields, `blocked_reason_code` enum, trigger object) |
| `custom_prompt` | strategy JSON | cross-family discipline borrowed from the templates |
| Doctrine S1–S5 | code | role, data contract, funnel + family playbooks, procedure for risk/sizing/geometry/position management, and additions to the output contract |

Duplicates removed:

| Rule | Was stated in | Now only in |
|---|---|---|
| TP cap → effective_rr → `rr_insufficient` | Hard Constraints, doctrine §4 | doctrine S4 (legacy doctrine only; non-v7 and hybrid_v2 prompts keep the Hard Constraints line) |
| Stop buffer = min stop + drift | Hard Constraints (v7-only line), doctrine §4 | doctrine S4 (same gating) |
| `blocked_reason_code` enum, `no_reviewable_candidate`, signal id copy, `blocked_signal_symbol`, trigger object, computed numbers | Output Format, doctrine §5 | Output Format; S5 adds only "exactly one", structural matching, and what a REVIEWABLE wait states |
| hold/wait are no-ops | Output Format, doctrine §4 | Output Format |
| Tier funnel order | `trading_frequency`, `decision_process`, `custom_prompt`, doctrine §3 | doctrine S3 |
| `tag_semantics` is the only label source; llm_action levels | `custom_prompt`, `entry_standards`, doctrine §2 | doctrine S2 |
| Family entry rules (breakout, displacement, leader, panic, funding) | `entry_standards`, doctrine §3 (with different numbers) | doctrine S3 |

## 3. Contradictions in the current prompt and how they were resolved

| # | Contradiction (strategy-06 + 2026-07-27 doctrine) | Resolution |
|---|---|---|
| C1 | §2 says REVIEWABLE "is not a market-entry permission" and must output a blocker. §4 lists "open-after-live-recheck (REVIEWABLE gaps settled)" as a release grade. The payload's `execution_policy` is `reviewable_open_allowed_only_if_live_confirmed`. | S2: REVIEWABLE opens only when every `must_confirm` / `required_confirmations` item is visibly satisfied in this cycle's data; otherwise it waits. S5 says what a REVIEWABLE wait must state. |
| C2 | `entry_standards` names `short_squeeze_long`, which is outside the brief's canonical list. | Not named. S3 adds a generic rule: a setup_type outside the families has no playbook, so use `execution_contract` + `tag_semantics` only, at conservative size at most. See open question Q1. Superseded by #74: it is now in the momentum family. |
| C3 | Per-family rules appear in both `entry_standards` and doctrine §3. Displacement review (capped effective_rr ≥ 1.5, VWAP, taker > 0.52, OI/volume) existed only in the sections. | Merged into S3. The displacement floor is the configured min RR (#77; it was `local.V7MinExecutableRR` = 1.5), and the confirmations are the `required_confirmations` that `repairV7DisplacementRRTagForExecution` attaches, so no taker literal is repeated. |
| C4 | Stop rule: Hard Constraints prefer ≥ min + drift (2.50%); §4 repairs the stop to "minimum + 0.10%" (2.10%), which is inside the drift band the first rule warns about. | S4: aim for the drift buffer and widen the stop toward it when repairing; min + 0.10% is the absolute floor. Both numbers are unchanged. |
| C5 | Two sizing schemes: confidence bands (Position Sizing Guidance) and release grades (§4), with "conservative size" undefined. | S4: the release grade picks a point within the confidence band. Normal uses the band; conservative and open-after-live-recheck use the low end of the band. No new numbers. |
| C6 | Alt-ladder text tells the LLM to place candidates into tiers ("may enter REVIEWABLE-small", "upgrade to EXECUTABLE", "mark reviewable_retest_pending"). The LLM cannot assign tiers, and no output field exists for that tag. | Rewritten as LLM actions: while REVIEWABLE, small size at most; normal size needs a 5m/15m close below the trigger with sell flow intact. The soft-release admission numbers (zone ≤ 45, OI ≥ 0.8%, taker ≤ 0.38) are tier gates enforced by `hunterV7AltLadderSoft*`, so the prose no longer restates them. The stop tolerance the LLM does choose (≤ 2.45% when taker ≤ 0.34) is kept and comes from the constants. |
| C7 | "funding_reversal defaults to WATCH in compression" is also tier wording. | Phrased as an LLM action with the same conditions: under `market_regime=compression`, open only if all four hold, otherwise wait. |
| C8 | Position-sizing lines printed a literal `%%` ("Use 80-100%% of max position value limit"). They are plain `WriteString` calls, not `Sprintf`. | Fixed in `BuildSystemPrompt`. This is the only change to the hybrid_v2 goldens. |
| C9 | The protection ladder in §4 duplicated the protector's defaults as literals. A strategy with its own `risk_control.protector` would see the wrong numbers. | S4 renders the ladder from `store.EffectiveProtector(risk_control.protector)` with each level's unit. The defaults render the same values as before. `TestHunterV7DoctrineProtectionLadderFollowsConfig` covers this. |
| C10 | `custom_prompt` "每轮必须主动寻找可开仓形态" (actively look for an open every cycle) reads as a push to trade, against "不要因为连续 wait 而降低标准" (do not lower the bar after repeated waits). | `role_definition`: the target is win rate, not trade count, and wait is a normal result. S1 keeps "a complete evidence chain gets an open or exactly one blocker". |

## 4. Template by template

| Template | Borrowed | Rejected, and why |
|---|---|---|
| 01 OI AI Trader (GPT-5.2) | Stateless inference, adapted: decide only from this input, and prior cycles reach you only as fields (`trigger_memory_*`, cooldowns, positions); in `role_definition`. Every trade needs thesis + invalidation + RR; in `role_definition`. Edge-persistence hold test "if flat now, would this still justify an open?"; in S4 position management. No averaging down; in `custom_prompt`. Re-entry needs a new edge; in `trading_frequency` item 5. | 5-minute loop (no fixed cadence exists). RR 2.7 / 1:2 (floor is 1.2). max_positions 3. TREND/RANGE/TRANSITION/CHAOTIC regime labels. "Do not exit because profitable / no fear of giveback", which conflicts with the backend protector's giveback closes. Re-classifying the 15m regime (the backend already does it, and `regime_against_direction` carries it). |
| 02 妖币猎手 | Volume/OI trap warning (a break on shrinking volume or falling OI is a trap → wait), in `custom_prompt`. "Prefer missing a trade to taking a wrong one" spirit, in `role_definition`. | 30-minute time stop (would be a new parameter; the protector owns exits). Hand-written SL/TP formulas (the router computes geometry). RR 1.5. Funding threshold 0.05% (a new parameter). 4h S/R proximity (no field in the payload). Skipping BTC/ETH (that is coin-source config, not prompt). |
| 03 Omni Game Theory Sniper V3 | Nothing new. Its "who is trapped" view is already covered by `funding_reversal` / `long_squeeze_short` and the crowding tags. | Regime TREND/EXTREME/CHAOTIC. Playbook names "Long Squeeze / Short Squeeze / Liquidation Wick". OPEN_LONG/WAIT/CLOSE_POSITION output. RR 2.5. max_positions 3. 15-minute loop. |
| 04 Larry Williams | Trap warning (same rule as 02, stated once). One best candidate per cycle, in `trading_frequency` item 3. | 5-minute scanner. "Volatility Squeeze Breakout" as a free-form name (the registered `volatility_squeeze_breakout` is in S3). Stop = entry ± 2×ATR (the router computes the stop). RR 2. max_positions 5. Relative strength vs BTC as a separate rule (already in `leader_momentum_long` / `relative_weakness_short`). |
| 05 Gaussian Grid x5 | Overtrading guard as a cycle rule: repeated waits never lower the bar, and the same failed structure is not re-entered. | 2–4 trades/day, hold ≥ 30–60 min (minute and day cadences). Grid mechanics. RR 3. max_positions 3. |
| 06 Hunter v7 current (baseline) | All doctrine content, the candidate priority order, global hard boundaries, per-candidate checklist and the "give the next trigger" rule. | Duplicates (§2 above), `short_squeeze_long` (C2), the urge-to-trade wording (C10). |

## 5. Template conflicts (brief §4), final resolution

| Conflict | Resolution in the redesign |
|---|---|
| RR floor | Hard Constraints and S4 use `risk_control.min_risk_reward_ratio` (1.2). The displacement review floor is the same configured min RR (#77 removed the 1.5 code floor). The Task-2 per-candidate `min_rr` note is unchanged. No template RR is used. |
| Max positions | Hard Constraints (`max_positions` = 1). `trading_frequency` refers to that limit without restating the number. |
| Cadence | Cycle-based rules only (`trading_frequency`). `TestRedesignedSystemPromptVocabulary` forbids minute/day cadence phrases. |
| Regime names | Updated in #74: S2 lists the code's `V7MarketRegime` values (including the canonical four), and the funding rule names `compression` and `trend_continuation`. TREND/CHAOTIC/TRANSITION are forbidden by the test. |
| Setup names | The 25 S3 values plus `short_squeeze_long` (momentum family, #74). The test checks that all 26 are present and that `module_no_match` and the template playbook names are absent. |
| Output format | The Output Format block + S5. The template enums (OPEN_LONG, …) are forbidden by the test. |
| Grid mechanics | Not imported. |

## 6. Where each number comes from

From config or the backend constant (rendered, never typed):

| Prompt value | Source |
|---|---|
| RR floor, TP cap, min stop, drift, stop buffer | `risk_control` via `effectiveExecutionGeometry()` |
| Displacement review RR | `risk_control.min_risk_reward_ratio` (#77; was `local.V7MinExecutableRR` = 1.50) |
| whale_flow LONG zone ≤ 45, taker ≥ 0.56 | `hunterV7WhaleLongMaxZonePos`, `hunterV7WhaleLongMinTaker` (tier rule and trader guard) |
| alt_ladder_breakdown_short stop ≤ 2.45% when taker ≤ 0.34 | `hunterV7AltLadderSoftStopStrong`, `hunterV7AltLadderSoftTakerStrong` |
| Protection ladder 5 / 10·50 / 15·45 / 8 / −8·15 min / −12 / 5.7 / 1 | `store.EffectiveProtector(risk_control.protector)` with units |
| TP0 reduce range | the candidate's `take_profit_plan.tp0_reduce_pct_min/max` |

Prose thresholds kept unchanged that have no single config or constant source (no silent tuning; owner decision Q3): panic taker ≥ 0.52; funding SHORT taker < 0.48; compression funding taker < 0.45; breakdown taker ≤ 0.50; range_expansion zone > 60%, 15m close > 10% below EMA20, live rebound > 0.30%; whale MFE ≈ 0.8%; stop-repair floor min + 0.10%; ladder stages 5% → 15% → 30%; mms EMA7/25/99 structure. Several of these equal literals inside the tier table or `engine.go` guards (for example 0.52, 0.48, 0.45, 60, 0.30), but there is no named constant to render from.

#111 S1 (prompt hard rules) adds two prose numbers, both the existing Position Sizing band boundaries in `BuildSystemPrompt` ("High confidence (≥85)", the medium band ending at 84): the high band (≥85) needs every confirmation visible and no shown level before take_profit, and a counter-trend setup against the 1h/4h direction opens only in that band. The 1h/4h and 5m/15m timeframe names in the HTF rule are prose as well.

#122 adds the code half of that rule. The floor 85 has one source, `store.V7CounterTrendMinConfidence`, and the family split is `store.V7HTFFamily`. The provider tags a gated setup `htf_against_direction` when its 1h and 4h both oppose it (EMA20/EMA60 with price on the same side). The kernel holds a trend family at WATCH and a counter-trend family at REVIEWABLE. The decision validator rejects a trend-family open and a counter-trend open below 85, and the trader guard repeats both checks.

## 7. Non-numeric behaviour changes for review

These are prompt-level rule changes. No backend gate changed.

1. Trap warning (02/04): a breakout or breakdown on shrinking volume or falling OI → wait, `confirmation_missing`.
2. No averaging down (01).
3. Candidate tie-break inside a tier: higher `effective_rr_after_cap`, then fresher data (lower `price_age_ms`, else `snapshot_age_ms`; #119: `signal_age_ms` was never in either payload).
4. A setup_type outside S3 → conservative size at most. (`short_squeeze_long` moved into the momentum family in #74, Q1.)
5. Conservative / live-recheck opens → the low end of the confidence band (C5).
6. Holding outside the protector levels: close only on the planned stop, hard invalidation or a 5m+15m structural reversal, using the "if flat now" test. Before, this exit rule applied only to micro-profit noise.
7. `decision_process` maps each failed check to a `blocked_reason_code`.
8. Re-entering a symbol after a close needs a fresh trigger confirmation in this cycle, and the same failed structure is not re-entered.

## 8. Open questions for the owner

- **Q1, `short_squeeze_long`: resolved (issue #74).** Owner decision: it belongs to the momentum family. Doctrine S3 now lists it there with its own playbook: forced short covering, open only while the covering is live; `squeeze_chase_risk` / `already_pumped_24h` / `lsr_extreme_long` / `funding_expensive` mean wait; invalidation is the 1h low; take the nearest target. strategy-07 `entry_standards` names it as a momentum-family member, and the vocabulary test expects it. The generic "no playbook" rule for other out-of-family setups is unchanged.
- **Q2, regime vocabulary: resolved (issue #74).** Owner decision: change the code, not the canonical list. `trend_continuation` is now a `V7MarketRegime` value. It is detected when both majors keep the same 4h EMA20/EMA60 structure, the compression condition holds, and trend_up/trend_down did not confirm. That is a strict subset of what was labelled compression, so module gating, weights and TP multipliers stay those of compression. Doctrine S2 lists every code value (`local.V7MarketRegimes()`), and the funding_reversal compression rule also covers trend_continuation. `TestHunterV7DoctrineRegimeListMatchesCode` checks the agreement.
- **Q3, prose thresholds in §6.** Keep, move to config, or drop?
- **Q4, budget baseline.** 18,336 bytes in the brief vs 22,173 measured here (§1).
- **Q5, hybrid_v2 doctrine.** `hunter_v7_prompt_doctrine_v2.go` is out of scope and unchanged apart from the shared `%%` fix.

## 9. Tests

| Test | Guards |
|---|---|
| `TestRedesignedTemplateChangesPromptTextOnly` | strategy-07 config == strategy-06 config |
| `TestRedesignedTemplateSystemPromptBudget` | strategy-07 system prompt ≤ strategy-06, zh and en |
| `TestRedesignedSystemPromptVocabulary` | all 25 S3 setup types and 8 blocker codes present; foreign setups, regimes, enums and cadences absent |
| `TestHunterV7DoctrineLanguageParity` | ZH and EN doctrine name the same identifiers and numbers |
| `TestHunterV7DoctrineProtectionLadderFollowsConfig` | the ladder follows `risk_control.protector`, including units |
| `TestBuildSystemPromptShowsEffectiveHunterV7RiskGeometry` (updated) | geometry numbers, doctrine phrases, no duplicate TP-cap/stop-buffer line, no literal `%%` |
| `TestU11LegacyPromptGolden` (re-recorded) | full legacy prompt, zh and en |
| `TestHunterV7GoldenPrompt` (unchanged) | the user prompt is byte-identical |

## 10. Follow-up #74: TP0 / trailing alignment (AUD-D7)

S4 used to tell the model "reduce 30–50% at TP0, then trail at breakeven or 5m EMA20/VWAP", which the protector does not do. Issue #74 chose option A: the doctrine now describes the protector. The values are rendered from `store.EffectiveProtector`:
- the TP0 partial ratio (35% by default);
- breakeven + 0.1%;
- the dynamic trail (activates at 0.5R MFE and trails 0.4R, with R = the protector's base risk, 35% ROE);
- the profit locks.

The candidate's `take_profit_plan` reduce range and `trailing_basis` are labelled planning hints. The reasoning is in the #74 PR.
