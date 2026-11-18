# U11: payload and doctrine versioning, per-class LLM role, Task C pass 1, hybrid payload

Record for the redesign plan §6.3 U11 row and the "LLM payload" row of §6.2. The fields audit is `taskc/kiro-fields-audit-20261117.md` in ait-t2-data (head 77897c4, MD5 89f081ecee696faf1b8eecac5b155282).

**Owner decisions.**
- **D1 (a):** an `llm_role` field per candidate and per-class role text in the doctrine, descriptive only. The IMPULSE veto-with-timeout mechanism is not built.
- **D2 (a):** T2 setups are emitted as `t2_<setup>`. Config and registry accept both names, tooling maps upstream names, and the native side is not renamed.
- **D3 (b):** payload and doctrine changes sit behind config, default off. The payload golden verifies them with the config on.
- **Hybrid scope (a):** the hybrid payload is built in U11. The deletion list applies only to that payload version.

## Config

`hunter.v7_prompt` (nil = legacy everywhere):

| Key | Default | Meaning |
|---|---|---|
| `payload_version` | `legacy` | `hybrid_v2` selects the new payload and doctrine. Unknown values read as legacy. |
| `bars_per_timeframe` | 30 | Closed bars per timeframe. The unfinished bar is printed separately. |
| `timeframes` | 5m, 15m, 1h, 4h | Bar timeframes. With `hybrid_v2` the market-data fetch adds any the kline selection lacks and fetches `bars_per_timeframe + 1` bars. |
| `llm_roles` | IMPULSE full, TRIGGER decide_before_trigger, REVERSAL full, CONTEXT none | Per-class overrides. `decide_before_trigger` applies only to a conditional entry (a TRIGGER that has not reached its trigger); after the trigger the role is `full`. An unassigned class is `full`. |

`hunter.v7_t2_setup_prefix` (default false) emits T2 setup types as `t2_<setup>`. The emission is behind a flag so that no default output changes (D3). Lookups accept both names either way.

**Versions.**
- Payload versions are `legacy` and `hybrid_v2`.
- The doctrine version is `hunter-v7-doctrine-hybrid-v2` for hybrid_v2. The legacy doctrine keeps `none`, the value every pre-U11 record carries, so records of the same doctrine keep one tag value.
- The U5 `doctrine_version` tag now reads the effective version.

## What hybrid_v2 presents

**Primary input: raw bars.**
- Per timeframe: closed bars old to new, UTC, with OHLCV, ema20, ema50, macd, rsi7, rsi14 and the three boll bands. Series are right-aligned, with `-` before warm-up.
- The unfinished bar goes on its own line: `unfinished — do not use for levels`.
- `atr14_<tf>_price` is labelled "computed through the unfinished bar", because `calculateATR` includes it.
- The closed/unfinished split uses `Context.PromptTime` (zero = now).

**Appendix (`hunter_v7_signal_v2_json`).**
- **market:**
  - `price_at_prompt_build` is the single reference price of every derived number.
  - `oi_notional_usdt`, `oi_change_last_1h_pct`, `oi_change_last_4x1h_sum_pct`.
  - `funding_rate_per_settlement_frac`, `taker_buy_ratio_latest_15m_bar_frac`.
  - `snapshot_age_ms`, `price_age_ms`, `kline_1m_age_ms`, `kline_5m_age_ms`, `stale_threshold_ms`.
  - A zero aggregate is omitted, not shown as a fact.
- **signal:**
  - Identity: `signal_id`, `symbol`, `direction`, `setup_type`, `class`, `lifecycle_state` (T2), `status`, `cell_status`.
  - Levels: `entry_zone_lower_price` / `entry_zone_upper_price`, `trigger_price`, `invalidation_price`.
  - `zone_position_pct`: the single clamped version, LONG measured from the lower edge, SHORT from the upper edge, at `price_at_prompt_build`. It is absent for a single-price zone.
  - `invalidation_distance_pct` (signed).
  - Three named RRs, `nearest_target_rr_ratio`, `tp1_rr_ratio` and `tp2_rr_ratio`, with `rr_basis` (entry `price_at_prompt_build`, stop `invalidation_price`, uncapped).
  - `tp1_price` / `tp2_price`.
  - The M2 menus `stop_candidates` / `target_candidates` (IDs; M2 field names unchanged, per the dictionary) with `menu_basis`. The menus are reference levels in calculation mode, built by `BuildV7StructurePlan` from the closed bars of the setup's horizon class, measured from `price_at_prompt_build`.
- **execution:**
  - `tier`, `tier_reason`, `entry_permission`, `llm_role`, `doctrine_version`.
  - `limits`: `min_rr_ratio`, `min_stop_distance_pct`, `max_tp_distance_pct`, `max_entry_drift_pct`, `drift_reference`. These are the candidate's own values, the same ones the backend validates.

**Naming conventions.**
- Enum values carry a domain prefix: `tier_`, `status_`, `lifecycle_`, `class_`, `cell_`, `role_`.
- T2 setup types are presented as `t2_<setup>` whatever the emission flag.
- The validator compares setups in canonical form and strips `tier_` from `selected_hunter_v7_tier`, so copied values match.

**Not presented** (hybrid spec B: "其余全删"):
- reason_codes, risk_tags, required_confirmations and tag_semantics;
- execution_quality, position_size_hint, suggested_trigger and the TP0 fields;
- tp time windows and methods;
- price_context (vwap, atr, change);
- the compact market data, quant data, the tag policy block and the market-environment regime line.

**Doctrine v2 (EN + ZH)**, file `kernel/hunter_v7_prompt_doctrine_v2.go`:
- §1 carries the role text.
- §2 is the data contract: bars first, then the three namespaces with units and the reference price.
- §3 is the funnel, with structure read from the bars.
- §4 is geometry from `execution.limits` fields, with no numeric thresholds in prose.
- §5 is the output contract with v2 field paths.
- Wording fixes ①–⑤ of the audit are applied.
- Dropped: the family playbooks and the position-protection ladder. Their thresholds and tag names are deleted or not presented.

## Pass 1 renames (presented surface, hybrid_v2 only)

| Legacy key | hybrid_v2 key | Note |
|---|---|---|
| price_context.last (overwritten with the prompt-build price in the prompt path) / compact `price` | market.price_at_prompt_build | dictionary |
| derivatives_context.oi_value | market.oi_notional_usdt | **deviation:** dictionary says `oi_contracts`; see N15 below |
| derivatives_context.oi_change_1h / oi_change_4h | market.oi_change_last_1h_pct / oi_change_last_4x1h_sum_pct | not in the dictionary; named from the computation |
| derivatives_context.funding_rate | market.funding_rate_per_settlement_frac | dictionary |
| derivatives_context.taker_buy_ratio_15m | market.taker_buy_ratio_latest_15m_bar_frac | dictionary |
| data_freshness.* | market.*_ms | dictionary (already unit-suffixed) |
| entry_zone.lower / upper | signal.entry_zone_lower_price / upper_price | dictionary |
| execution_contract.trigger_price | signal.trigger_price | dictionary |
| execution_geometry / readiness / compact zone positions (5 variants) | signal.zone_position_pct | dictionary, single clamped version |
| compact invalidation_dist / stop_distance_pct | signal.invalidation_distance_pct | dictionary, signed |
| nearest_target_rr / tp1_rr / tp2_rr | signal.nearest_target_rr_ratio / tp1_rr_ratio / tp2_rr_ratio | **deviation:** `structural_` omitted (N06: TP1/TP2 are not structural in legacy geometry); `confirmation_rr_ratio` dropped to keep three RRs (fix ①: module estimate, not admission) |
| execution_tier / status | execution.tier / signal.status | enum prefix |
| execution_contract.entry_permission | execution.entry_permission | namespace |
| execution_contract.min_rr / min_stop_pct / max_tp_pct / max_drift_pct / drift_reference | execution.limits.* | units in the name; values now always present |
| reason code risk_score_gte_65 (and any tier reason) | risk_penalty_0_100_gte_65 | dictionary `risk_penalty_0_100`; a flow label inside a reason becomes `taker_flow_gate` |

**N15 does not hold on the hunter_v7 path.** `derivatives_context.oi_value` is `symbolOINotional` (`hunter_v7_universe.go`), which is contracts × price, so the unit is USDT. This was already true at fafb65a. `oi_contracts` would mislabel the value, so the name follows the dictionary's unit rule (`_usdt`) instead.

**OI change.** In the hunter_v7 path (datafetch `fetcher_per_symbol.go`), `OIDelta1h` is the last 1h step of the OI history and `OIDelta4h` is the sum of the last four 1h percent steps, not one 4h change.

**Not renamed:** signal_id, symbol, direction, setup_type and the M2 menu fields (dictionary).

**Tag semantics.** In hybrid_v2 no tags are presented, so pass 1 has no tag-semantics surface. The only reason-code surface is `tier_reason` and the REJECTED counts, and both pass through the renames above.

**Dictionary entries with no hybrid_v2 surface:** `market.vwap_20x15m`, `market.atr14_1h_price` / `atr14_4h_price` and `market.change_since_prev_*_close_pct`. The bars carry the inputs, and spec B does not list these fields.

**`market.last_4h_close`** is not presented. In the prompt path `price_context.last` is overwritten with the prompt-build price (`hunterV7CandidateWithLiveMarketPrice`), so it would repeat `price_at_prompt_build`; the 4h bars carry the closes.

## Deletion list: 11 vs 12

The audit (spec C) lists twelve items:
- the ten field names setup_score, timing_score, ai_priority, confidence, risk_score, liquidity_score, regime_fit_score, market_regime, market_shape and entry_signal;
- "flow labels";
- "compact execution summary line".

The count of 11 matches the eleven field-like entries. The 12th is the compact execution summary line (N34), which is a prompt line rather than a field. All 12 are enforced (`TestU11DeletedFieldsNeverReachHybridPrompt`):
- the ten names as case-insensitive substrings;
- every catalog `flow_` tag, plus any token starting with `flow_` (setup names such as `whale_flow_reversal` are not tokens that start with `flow_`);
- the markers `Hunter v7 execution compact`, `compact_execution_json` and `execution_compact`.

**Scope of the scan.**
- The test scans the hybrid_v2 user prompt and the hybrid_v2 doctrine with no exemption.
- It also scans a stress variant with positions at the limit (open-disabled summary), a held-position block and a REJECTED `risk_score_gte_65` candidate.
- The rest of the system prompt is scanned for every entry except `confidence`. There, `confidence` is the LLM's own output field (Decision.Confidence: the output schema, the sizing guidance and the min-confidence line), not the module's deleted label.
- The detector is shown to be live on the legacy prompt of the same fixture.

## Pass 2 (code / DB identifiers): all deferred

The dictionary marks entries "code" or "prompt 表面" but marks none "safe", so no code or DB identifier is renamed. Deferred, with reasons:
- **The Go fields behind the presented values** (V7DerivativesContext.OIValue / OIChange1h / OIChange4h / FundingRate / TakerBuy15m, V7PriceContext.*, V7DataFreshness.*). Their JSON tags are the legacy prompt surface, which must stay byte-identical.
- **`signal.zone_position_pct` as one code value, `signal.invalidation_distance_pct`, the named RR set and confirmation_rr** ("code" in the dictionary). Five zone-position and seven RR computations feed guards and validators (for example the whale-flow guard and setup_guard zone limits). Unifying them changes admission, which needs its own unit and acceptance.
- **`execution.llm_entry_price` / `executable_price` split, `execution.readiness_score_0_100` vs `quality_band`, reason_codes / risk_tags mutual exclusion, and `signal.flow_taker_buy_threshold_frac`.** These are code changes to the decision record, readiness and tag catalog, outside the prompt surface.
- **DB columns** (`hunter_v7_signal` records: setup_score, ai_priority, confidence and others). These are stored history; renaming them needs a migration and keeps every existing record readable only through aliases.

## D2 alias handling

- **Emission.** With the flag on, `RunV7T2Module` renames the emitted outputs only. The module's own checklist, observe-only list and lifecycle keep the scanner names.
- **Lookups.** These read `store.LookupV7Setup` / `V7CanonicalSetupType`, so either name works:
  - registry (`EffectiveV7SignalClasses`, keys canonical);
  - setup geometry overrides;
  - setup_guard;
  - the final-conflict CONTEXT set;
  - the T2 checklist and observe-only lists;
  - the structure class and family;
  - the U5 family tag;
  - the validator's setup comparison.
- **Precedence.** When a config lists the same setup under both names, the `t2_` name wins.
- **Tooling.** The cmd tools compare upstream rows against the internal `V7T2Signal` (scanner names). The golden key and the U7 setup field are canonicalized.

## Verification

- **Legacy prompt goldens** (`kernel/testdata/hunter_v7_prompt_legacy_{en,zh}.golden`). They were recorded on the unchanged base 38c1ce2 (first commit of the branch) and are byte-identical after U11. The existing `golden-prompt-20260726` replay is unchanged.
  - The data dictionary block is cut from the system-prompt golden because `GetSchemaPrompt` ranges over maps. That nondeterminism predates U11.
- **Payload goldens** (`hunter_v7_prompt_hybrid_v2_{en,zh}.golden`) cover five candidates: a native leader, a TRIGGER conditional entry, an IMPULSE, a REVERSAL and a native WATCH. Config on.
- **U6 golden replay** (`t2_u0_native`, 694 events, 4,164 rows, 6,132 candidates): byte-identical to the base run and to `u6_golden_u6_fix.json`.
  - With the new `-golden-t2-prefix` flag (T2 signals assembled under `t2_` names) it is also byte-identical: class, admission, tier, reason and the risk stamps are unchanged by the prefix.
- **u0b and conflict outputs:** byte-identical to base.
- **Prompt size.**
  - An expanded candidate is 16.7 KB in hybrid_v2 vs 3.8 KB in legacy. The fixture user prompt is 68.0 KB vs 17.8 KB.
  - With the default window of 8 expanded candidates, the bars dominate the prompt.

## Open points

- **Required confirmations.** With spec B, `required_confirmations` is not presented. The LLM judges confirmation from the bars, while `entry_permission` and `tier_reason` remain. Adding it back is one field if the owner wants it.
- **Positions block.** The held-position block still uses the legacy compact market line (`oi_latest`, `funding=`). It is outside the candidate payload, carries no deleted entry, and is unchanged in U11.
- **M2 menu depth.** The menus are built from the bars the prompt fetches (31 per timeframe). That is shorter than the FAST lookbacks (96 × 15m, 168 × 1h), so menus can be sparser than in the structure geometry mode.
