# Hunter v7 optimization design: patch vs refactor (issue #80, Phase 2)

Date: 2026-11-24. Base: `t2/v7-selection` @ `ac1d5b2` (after #76 and #78 merged). Design only: this PR adds this document and two measurement-only tests that skip unless `AIT_EXPERIMENTS=1`. No production code changes.

Inputs:
- The audit `docs/audit/adversarial-review-20261124.md` (#69, PR #72). Finding IDs below are the audit's `AUD-*` IDs without the prefix. Audit line numbers refer to `461074e8`. Where code moved since then, this document names the function.
- Fixes already merged: C5 and C3 (#73, PR #76), D7 and the regime enum part of A2 (#74, PR #78).
- Open PRs that change the picture:
  - PR #79 (#75, custom models): no effect on this design.
  - PR #82 (#77, config is authoritative). It removes the hunter_v7 70 confidence cap (`effectiveMinOpenConfidence` in `kernel/engine_prompt.go` and `trader/auto_trader_risk.go`), the hardcoded 1.5 executable RR floor (`local.V7MinExecutableRR`), and the `MaxPositions = 3` clamp in `store.ClampLimits`. It also makes the kernel's 60 confidence floor follow `min_confidence`. At `ac1d5b2` all of these still exist. Rows below that depend on #82 say so.

## 1. Headline recommendation

**Patch overall. Do two bounded refactors and patch everything else.**

1. **Refactor: tier authority (cluster 1).** Today each cycle produces two tier verdicts. The assembly tier drives the AI-skip check and the execution gate. The prompt-final tier is what the LLM sees, and it is "recorded only" (`kernel/engine_prompt.go:708`). Patching each consumer separately would keep two verdicts that can drift apart again. The refactor makes one function compute the final tier once per cycle and write it back to `ctx.CandidateCoins`. The AI skip, the decision validator and the trader gate then read only that value. The change is narrow: about four call sites plus the write-back. It does not rewrite tiering.
2. **Refactor: one per-position lifecycle record (clusters 2 and 4).** C4 (no fill read-back), E3 (decision saved after the orders), C6 (conditional entries in memory only), D4 (protection state in memory only), D5 (TP lost when the plan is missing) and D6 (LLM close racing the protector) have the same root cause. No persisted object owns "this position: intended plan, actual fill, protection state". Patching them one at a time would add six separate stores and locks. The refactor adds one persisted record per position, written before the order and updated after the fill and by the protector, and one per-position lock. Both the open path and the protector use it.
3. **Patch: screening and regime determinism (cluster 3), LLM I/O strictness (cluster 5), and the remaining items in clusters 1, 2 and 4.** Each fix is local to one function, and each already has an adversarial test that flips when the fix lands.

Why not a full refactor of the pipeline? The safety-critical paths work and are guarded: the signal-contract match, the fresh-price drift check and the independent protector. The two P0s are already fixed (#76). The experiments below do not show a measured outcome problem that a rewrite would fix. X1, the fee-net evidence, is blocked. X4 found 0 tier divergences on the one reachable round. A rewrite would also invalidate the frozen goldens that currently protect behaviour.

Order of work: cluster 1, then the cluster 2/4 lifecycle record, then cluster 5, then cluster 3. Cluster 3 changes which symbols are screened, so it changes outcomes. It should land only with the X3 coverage logging in place and fee-net evidence (X1) in hand.

## 2. Per-cluster decisions

Each cluster lists the decision, the files and functions involved, the tests that must flip or be added, and what must not change.

### Cluster 1: tier authority (B7, A3, B2, B4, B5) — refactor B7, patch the rest

| ID | Decision | Where | Test |
|---|---|---|---|
| B7 | Refactor. Add one `finalizeHunterV7Tiers(ctx)` called before the AI-skip check. It runs today's `hunterV7PromptItems` pass (live price, live confirmations, readiness, `PromptWait`) and writes `Tier`/`Reason` back into `ctx.CandidateCoins[i].V7ExecutionTier`/`V7TierReason`. It keeps the assembly verdict in a new `V7AssembledTier` field for the record. `writeHunterV7TieredCandidatePrompt` then renders the stored result and does not re-tier. | `kernel/engine_prompt.go` (`writeHunterV7TieredCandidatePrompt`, `hunterV7PromptItems`); `trader/auto_trader_loop.go` (AI skip); `trader/auto_trader_risk.go` (`validateHunterV7ExecutionGuard` tier gate) | Invert `TestAudit_B7_PromptTierAndExecutionTierDiverge`. Keep the X4 test as a regression metric. The prompt golden must stay byte-identical. |
| A3 | Patch. Missing derivatives or liquidity data demotes to REVIEWABLE (fail closed one step). It does not pass and it does not reject. Delete the dead `AllowMissing`/`RequireConfirmAll` fields, or make them real in the same change. | `kernel/engine.go` `hunterV7TakerBuyAtLeast/AtMost`; `kernel/hunter_v7_tier_rules.go` `MinLiquidity` | Invert both `TestAudit_A3_*`. |
| B2 | Patch. Pass the final tier counts (from B7) to the validator. Reject `no_reviewable_candidate` when E+R > 0. Stamp the flat-skip record with `no_reviewable_candidate`. | `kernel/engine_analysis.go` validator; `trader/auto_trader_loop.go` skip record | Invert `TestAudit_B2_*`. |
| B4 | Closed by PR #82 once merged: one floor (`min_confidence`) in kernel and trader, with 60 only as the unset fallback. | `kernel/engine_position.go`; `trader/auto_trader_risk.go` | `TestAudit_B4_*` was updated in #82. |
| B5 | Patch. Validate RR at the backend trigger (`HunterV7TriggerPrice`), not the model's `trigger.trigger_price`. Re-check RR at fill for conditional entries (joins the C6 fix). | `kernel/engine_position.go` RR block | Invert `TestAudit_B5_*`. |

Must not change: the signal-contract match (id, symbol, direction, setup, tier), the tier rule tables, and the frozen golden prompt.

### Cluster 2: transactional open path (C1, C2, C4, C6, C7, E3, D6) — refactor the lifecycle record, patch the price checks

| ID | Decision | Where | Test |
|---|---|---|---|
| E3, C4 | Refactor: lifecycle record. Before ordering, write a decision-intent row with the plan (entry, stop, TP, size, signal id). After the order, read back `executedQty`/`avgPrice` and update the row. Compute stop and TP distances from the fill. On startup, rows still in "intent" are reconciled against exchange positions. | `trader/auto_trader_orders.go` open paths; `trader/binance/futures_orders.go` (result type); `store/decision.go` (new status column or table) | New fake-exchange tests: crash between fill and save; partial fill. |
| D6 | Refactor: a per-position mutex taken by the LLM close path and by the protector (partial, rebuild, close). After an LLM close, cancel the protection orders this position owns. | `trader/auto_trader_orders.go` close path; `trader/protector_engine.go`; `trader/auto_trader_protection_rebuild.go` | New race test with a fake exchange. |
| C6 | Patch on top of the record. Re-run `validateHunterV7ExecutionGuard` at fire time on the latest snapshot. Persist pending entries in the record. On restart, re-arm them or cancel them visibly. | `trader/hunter_v7_conditional_entry.go` | New test. |
| C1 | Patch. Measure drift against the LLM's original price in every repair pass (`decisionPriceBeforeRepair`), so passes cannot stack. Keep favourable re-anchoring, bounded by the same limit. | `trader/auto_trader_risk.go` `repairHunterV7DecisionPrice` | Invert `TestAudit_C1_*`. Keep `TestAudit_C1b_*`. |
| C2 | Patch. In `geometry_repair=reject`, `capTakeProfitToTP1` rejects instead of capping. | `trader/auto_trader_risk.go` | Invert `TestAudit_C2_*`. |
| C7 | Patch. Re-read the book after the last pre-order REST call and apply the drift check again. A marketable limit order with a drift band is a later, separate decision for the owner, because it changes fill behaviour. | `trader/auto_trader_orders.go` | New test. |

Must not change: the C5/C3 fixes from #76 (per-poll stop audit, assume-filled, open gate, cache invalidation), the fresh order-book drift reference, and `ClosePosition(true)` stops.

### Cluster 3: screening and regime determinism (A1, A2, A5, A6, E1) — patch

| ID | Decision | Where | Test |
|---|---|---|---|
| A1 | Patch. Use `sort.SliceStable` with a symbol tie-break, skip zero-value fillers, and break ties in the cap step by symbol. Log `ranked/valid` coverage each round. Whether to rank all valid symbols is an owner decision. It changes cost and outcomes, so it waits for X1 evidence. | `provider/local/hunter_v7_universe.go` `BuildV7Universe` | Invert `TestAudit_A1_*`. X3 must then report 1 distinct set. |
| A2 | Partly done in #78 (`trend_continuation` in the code enum). Remaining patch: add a BB-width percentile to `isCompression`, test range before compression, and add 2-round hysteresis. The per-symbol trend context before `regime_against_direction` is a separate, later change. | `provider/local/hunter_v7_regime.go` | Invert `TestAudit_A2_*`. |
| A5 | Patch. Stamp a fetch time on each slow field (OIΔ, LSR, depth, 1h/4h klines) in the collector. Expose the age in the signal and gate on it in tiering. | `datafetch/collector.go`; `kernel/engine_snapshot.go` | New test. |
| E1 | Patch. Use backend ages only and ignore the model's `signal_age_ms`. Apply an age gate to every tier, with per-class limits from config. | `trader/auto_trader_risk.go` freshness guard | Invert `TestAudit_E1_*`. |
| A6 | Design decision, not code yet. Define the funnel target as a count of E+R per round. Measure it (E4 summary fields), and set it only with X1 evidence. The duplicated `MaxOutput`/`HunterLimit` wiring (`kernel/engine.go:554-581` and `2844-2872`) moves into the single config resolver (§5). | `kernel/engine.go` | Covered by §5. |

Must not change before X1: the universe list sizes, the regime thresholds, and anything else that changes which candidates reach the LLM. The down-market-only sample cannot justify loosening WATCH.

### Cluster 4: protection-state persistence (D1, D2, D3, D4, D5) — uses the cluster 2 record, patches the rules

| ID | Decision | Where | Test |
|---|---|---|---|
| D4 | Refactor, through the lifecycle record. Persist TP0/TP1/giveback flags, peak PnL and the first-seen time per position. Load them on start. Do not rebuild them from the last 20 prompts. | `trader/auto_trader_risk.go` protector state | New restart test. |
| D5 | Patch, through the record. The TP comes from the record, falling back to the exchange's open orders. Never cancel a TP without a replacement. | `trader/auto_trader_protection_rebuild.go` | New test. |
| D1 | Patch. Clamp the rebuild stop to the protective side of mark with the existing `isStopOnProtectiveSide`. If clamping is not possible, close the 50% deliberately rather than through the error path. | `trader/protector_engine.go` `rebuildStop` | Invert `TestAudit_D1_*`. |
| D2 | Patch. Accept int and float leverage. Treat ≤ 0 as unknown: use the default and log a warning. | `trader/auto_trader_risk.go`; `trader/gate/trader_account.go` | Invert `TestAudit_D2_*`. |
| D3 | Patch, behind config. Add a minimum price move for TP0 and the profit floor (as TP1 has), as a protector field the owner sets. Default it to the current behaviour so nothing changes until the owner chooses a value. | `store/protector.go`; `trader/protector_engine.go` | Update `TestAudit_D3_*` to assert the new floor when it is set. |

Must not change: the protector goroutine's independence from the decision loop, the -12% ROE backstop, and the doctrine text aligned with the protector in #78.

### Cluster 5: LLM I/O strictness (B1, B3, B6, B8) — patch

| ID | Decision | Where | Test |
|---|---|---|---|
| B1 | Patch. "No structured decision" is an AI failure with reason `no_structured_decision`. It counts toward safe mode. | `kernel/engine_analysis.go`; `trader/auto_trader_loop.go` | Invert `TestAudit_B1_*`. |
| B3 | Patch. Reject a reply with more than one `<decision>` block, and log both. | `kernel/engine_analysis.go` | Invert `TestAudit_B3_*`. |
| B6 | Patch. Restrict symbols to `[A-Z0-9]` at ingestion, and escape newlines in reason strings in the plain-text formatters. | `datafetch` ingestion; `kernel/engine_prompt.go` formatters | Invert `TestAudit_B6_*`. |
| B8 | Patch (measurement). Record `chosen_rank` and the E+R count on each decision record. This is the X2 instrumentation. | `kernel/engine_analysis.go`; `store/decision.go` | New test. |

Must not change: the streaming decision path from #68 and the JSON payload escaping.

### Deliberately left as they are

- **E4** is a reporting gap in the validation harness, not trading behaviour. Patch the run summary to emit per-round E/R averages, tier shares and the `DOWN-MARKET-ONLY` label when the harness is next touched.
- The whale confirm pool stays shadow-only and the entropy budget stays a lint test.
- Short ignition stays shadow-only until X1 runs on a non-down-market sample.
- The marketable-limit entry (C7 option) and ranking all valid symbols (A1 option) are listed above as owner decisions, not part of this plan.

## 3. Experiments X1–X5

| ID | Status | Result or blocker |
|---|---|---|
| X1 fee-net on the rerun DB | **Blocked** | No rerun DB is reachable. Checked: the sandbox filesystem, the repository, all remote branches including `origin/t2/tier-a-fixes`, GitHub releases and Actions artifacts. The command is ready: `go run ./cmd/v7_fee_net_offline -db <rerun.db> -out reports/fee-net-rerun -horizon 2h -tiers WATCH,REVIEWABLE,EXECUTABLE`. Needed: the owner provides the DB file. No numbers are given here, so none are invented. |
| X2 rubber-stamp rate | **Blocked; design only** | Needs at least 200 stored decisions with `chosen_rank`. That field does not exist yet (B8), and no stored decisions are reachable. The B8 patch is the instrumentation. Run it on paper decisions after it lands. |
| X3 universe stability | **Run** (synthetic and fixture-derived inputs) | See §3.1. |
| X4 tier split-brain rate | **Run on N=1 round** | See §3.2. |
| X5 naked-position drill | **Blocked; design only** | Needs Binance testnet credentials and owner approval. Production Binance is out of scope. Design: on testnet, inject (a) a `SetStopLoss` failure and (b) an inconclusive read-back, both through a fault-injecting exchange wrapper. Record the time from fill to the first protective stop, or to a close. Pass if the #76 per-poll audit re-arms within its 20 s grace plus one poll. |

### 3.1 X3 results

Test: `provider/local/experiments_x3_universe_stability_test.go`. Command: `AIT_EXPERIMENTS=1 go test ./provider/local -run TestExperimentX3 -v -count=1`. 50 calls of `BuildV7Universe` per input.

| Input | Label | Valid symbols | Distinct sets in 50 runs | Size | Coverage (size/valid) | Mean Jaccard | Min Jaccard | Always in / ever in |
|---|---|---|---|---|---|---|---|---|
| S1: AUD-A1 fixture, 420 symbols, only volume differs | SYNTHETIC | 420 | 48 | 192–209 | 45.7–49.8% | 0.661 | 0.568 | 150 / 420 |
| S2: 450 symbols, seeded values, funding pinned at 0.0001 for ~75%, OIΔ only for 200 | SYNTHETIC | 450 | 47 | 321–333 | 71.3–74.0% | 0.875 | 0.824 | 300 / 450 |
| S3: the 201 symbols of `universe-20260726` turned back into a snapshot, no klines | FIXTURE-DERIVED (real data, post-universe subset) | 200 | 17 | 193–195 | 96.5–97.5% | 0.971 | 0.945 | 189 / 201 |

Reading:
- Selection is nondeterministic on every input. Even on real data (S3), about 6% of the set changes between identical calls.
- Instability grows with ties. In S1, every symbol appears in some run, but only the 150 volume leaders appear in every run.
- No input is a stored full-market snapshot, because none exists in the repository. Real-market magnitudes are therefore unmeasured. S3 understates instability because it is already a filtered set.

### 3.2 X4 results

Test: `kernel/experiments_x4_tier_split_test.go`. Command: `AIT_EXPERIMENTS=1 go test ./kernel -run TestExperimentX4 -v -count=1`. Same path as `TestHunterV7GoldenPrompt`.

- Round: `universe-20260726` (N=1). The router output 13 candidates.
- Assembly tiers: REVIEWABLE 2, WATCH 9, REJECTED 2. Prompt-final tiers: identical.
- Diverged: **0 / 13**. E/R membership flips (the cases where the trader gate would decide differently): **0**.
- Sensitivity (SYNTHETIC): moving every live price by −2, −1, −0.5, +0.5, +1 and +2% before the prompt pass also gave 0 / 13 each time.
- Why zero: all 9 WATCH rows are `confirmation_missing_*`, for example `confirmation_missing_5m_or_15m_close_through_breakout_level` and `confirmation_missing_taker_buy_15m_gt_0_52`. The fixture's market data carries only price and 1h/4h change, with no klines and no derivatives. The live-confirmation step therefore cannot settle any confirmation, and the zone checks are not the binding constraint on this round.

Conclusion: the split brain is real (it is shown on a constructed fixture by `TestAudit_B7_*`), but its rate on real rounds is unmeasured. The fixture cannot measure it. The X4 test should be re-run on stored rounds with full market data once those exist. Stored `HunterV7SignalRecord` rows already persist both tiers (fix 12).

## 4. Known open items

| Item | Status | Evidence |
|---|---|---|
| Fee-net tool (#55) not run on the rerun DB | **Still open, blocked** | Same blocker as X1. Both questions remain unanswered: whether WATCH is too strict, and whether short ignition should stay shadow-only. |
| #52: 12 vs 8 workers, slow 1h layer "~26 min" | **Unverified** | `datafetch/fetch_tiered.go` defaults `MaxWorkers` to 5. The collector uses 50. The harness flag `--max-workers` defaults to 8 (`cmd/hunter_v7_validate/main.go:223`). No "26 min" constant exists, so it is a measurement this repository cannot reproduce. |
| Down-market-only sample | **Confirmed, still applies** | `docs/hunter-v7-validation-sampling-rule.md` §4. Any WATCH or short-ignition conclusion must carry `DOWN-MARKET-ONLY`. |
| 0.64 LLM-ready per round, 88% WATCH | **Not reproducible** | AUD-E4: neither figure is a code field, and the stored output is absent. X4 on the one available round gives 2/13 E+R (15%) and 9/13 WATCH (69%), N=1. It is not comparable to Tier A. |
| AUD-C5, AUD-C3 (P0) | **Fixed** | PR #76, merged. |
| AUD-D7 | **Fixed** | PR #78, merged (doctrine describes the protector). |
| AUD-A2 regime enum | **Partly fixed** | PR #78 added `trend_continuation`. The threshold and hysteresis work remains (cluster 3). |
| AUD-B4 | **Fixed in open PR #82** | One confidence floor from `min_confidence`. |

## 5. Task 4: config-surface audit

Principle (owner): frontend config is the source of truth for anything that controls trading behaviour. Every row below was checked in code at `ac1d5b2`. Rows marked **UNVERIFIED** could not be fully traced and need a follow-up read before they are relied on.

Column "Honored?" answers whether a value saved in the frontend reaches the code that acts on it, unchanged.

### 5.1 Owner-named parameters

| Parameter | Code location and value | Frontend field | Honored end-to-end today? | If frontend and code disagree |
|---|---|---|---|---|
| Polling interval | Lives on the **trader** record, not the strategy: `store/trader.go:29` `scan_interval_minutes` (DB default 3). `api/handler_trader.go:415-418` (create) and `:599-605` (update) force a minimum of 3 with no maximum. `manager/trader_manager.go:650` converts it to `ScanInterval`, which drives the decision ticker in `trader/auto_trader.go`. | `TraderConfigModal.tsx:518-537`, min 3, max 60 (HTML attribute only; `onChange` enforces only `max(3, …)`). | **Yes**, for 3 and above. | Below 3: silently raised to 3 by both UI and API. Above 60: the UI attribute warns, but the API accepts it. One strategy shared by several traders can run at different cadences, because the interval is per trader. The market collector runs on its own fixed 30 s cadence (`datafetch` `RestInterval`), independent of this value. |
| Max open positions | `store/strategy.go:23` `MaxPositions = 3` and `ClampLimits` (`:80-82`) clamp anything above 3. The trader enforces it in `enforceMaxPositions` (`trader/auto_trader_risk.go:726`), with `maxPositionsLimit` defaulting 0 → 3 (`trader/auto_trader_open_gate.go:43-45`). The prompt uses it at `kernel/engine_prompt.go` (`positionLimitReached`). | `RiskControlEditor.tsx:60-66`, min 1, max 3, fallback 3. | **Yes for 1–3. Values above 3 are clamped silently.** PR #82 removes the clamp and the 0→3 default, and fails loudly when the value is missing. | Owner sets 5 → code runs 3 and the UI shows 5 if saved through JSON import. Owner sets 1 (the stated choice) → honored. |
| RR floor | Several values exist at once:<br>• Strategy `risk_control.min_risk_reward_ratio`: the default config sets 1.2 (`store/strategy.go:730`).<br>• Kernel unset fallback `legacyMinRiskRewardRatio = 3.0` (`kernel/engine_position.go:17`).<br>• T2 floor `DefaultT2MinRiskRewardRatio = 1.2` (`store/strategy.go:112`).<br>• Hardcoded router/tier floors `V7MinAbsoluteRR = 1.2` and `V7MinExecutableRR = 1.5` (`provider/local/hunter_v7_geom.go:20,24`), used in `hunter_v7_execution.go` and as `hunterV7BackendMinRR` in `kernel/engine.go:2078`.<br>• The literals 1.2/1.5 repeated in `hunter_v7_confirmation.go:65,73`.<br>• Per-class and per-setup overrides (`class_profiles`, `setup_geometry_overrides`).<br>• Template presets: 2.7, 1.5, 2.5, 2, 3, 1.2, 1.2 (`docs/strategy-prompt-templates/strategy-01…07`). The 2.5 is strategy-03's preset. | `RiskControlEditor.tsx:280-290`: min 1, max 10, **step 0.5**, fallback **3**. | **Partly.** The configured value gates the LLM decision (kernel validator and trader `decisionMinRiskRewardRatio`). The router and tiering use the hardcoded 1.5, so a candidate at RR 1.3 cannot become EXECUTABLE even when the owner sets 1.2. PR #82 replaces the 1.5 with the configured value (`EffectiveMinRR()`). | Owner sets 1.2 → the LLM may open at 1.2, but the tiers still demand 1.5, so fewer candidates are offered than intended. A strategy with the field unset gets 3.0 in the kernel but 1.2 from the default config, depending on whether the default was applied. The UI spinner cannot reach 1.2 (step 0.5 marks it invalid), and an empty field shows and saves 3. |
| Minimum confidence | Strategy `risk_control.min_confidence`. Hunter v7 caps any configured value above 70 at 70 (`kernel/engine_prompt.go:259`, `trader/auto_trader_risk.go:2355`). The kernel hard floor is 60 (`kernel/engine_position.go:157`), and the contradiction cap rewrites confidence to 70 (`:174,181`). | `RiskControlEditor.tsx:447-460`, range slider from min 50, fallback 75. | **No for hunter_v7 above 70.** Owner sets 75 → the code uses 70. PR #82 removes the cap and makes 60 the unset fallback only. | A frontend value of 75 displays 75 while orders at 70–74 are accepted. |
| Position sizing | Strategy `btc_eth_max_position_value_ratio` / `altcoin_max_position_value_ratio`, with trader fallbacks 5.0 / 1.0 when unset. `min_position_size`, fallback 12. Leverage `btc_eth_max_leverage` / `altcoin_max_leverage`, where 0 means unlimited in the trader (see D2). `max_margin_usage` is in the UI, but the trader has only a hardcoded margin factor `1.01/leverage + 0.001` (`trader/auto_trader_orders.go:222,398`). **UNVERIFIED** whether the kernel reads `max_margin_usage`. The confidence-based size bands (80–100% / 50–80%) exist only as prompt text (`kernel/engine_prompt.go` ~118-130). No code enforces them. | `RiskControlEditor.tsx`: value ratios 0.5–10, leverage up to 20, margin usage 10–100%, min position size ≥ 10. | **Partly.** Ratios, leverage and minimum size are honored. Margin usage is shown but apparently not enforced. Size-by-confidence is advisory only. | Owner lowers margin usage to 30% → nothing changes in execution (if the UNVERIFIED reading holds). Owner sets min size 10 in the UI while a stored 0 falls back to 12 in code. |

### 5.2 Other trading-behaviour parameters found

Grouped by layer. "Proposed field" means no frontend input exists today. Values in "JSON only" can be set only through strategy JSON import.

**Screening and router (provider/kernel)**

| Parameter | Code location and value | Frontend field | Honored? | If they disagree |
|---|---|---|---|---|
| Candidates passed to the LLM | `v7_max_output` (default 30, `DefaultV7Config`) and `hunter_limit`. `ClampLimits` (called on the live path at `kernel/engine_analysis.go:57`) forces `hunter_limit` into 1–10, and `kernel/engine.go:577-579` uses min(max_output, hunter_limit). | CoinSourceEditor: `hunter_limit` fallback 10 (4 copies of the input), `v7_max_output` fallback 30. | **No above 10.** | Owner sets 30 → 10 candidates. |
| Watch output, minimum AI priority, aggressive | `v7_watch_output` 5, `v7_min_ai_priority` 55, `v7_aggressive` | CoinSourceEditor (`v7_min_ai_priority` 55, `v7_aggressive` default true) | Yes (copied in `kernel/engine.go:554-565`, duplicated at `2844-2872`). | The two copies can drift (A6). |
| Router minimum output, fallback priority, correlation cap | `MinOutput` 3, `FallbackMinAIPriority` 45, `CorrelationMaxPerTheme` 3 (`DefaultV7Config`) | None. Proposed fields. | Code-only; never copied from config. | Not configurable. |
| Setup thresholds | `DefaultSetupThresholds()` in the provider. `risk_control.setup_guard` is read only by the trader (`trader/auto_trader_risk.go:2120`). | JSON only | **Split.** The provider ignores the strategy's setup guard. | The router admits what the trader later blocks, or the reverse. |
| Universe list sizes and cap | Top-N 150 / 50 / 50 / 50 / 50 / 80 / 80 / 80, amplitude ≥ 12%, cap 350 (`hunter_v7_universe.go:135-158, 238-254`) | None | Code-only | Not configurable (cluster 3 decision). |
| Regime thresholds | ADX > 22 trend, < 25 compression, < 20 range; abs(24h change) < 5; alt outperformance +8 with count ≥ 10 (`hunter_v7_regime.go`) | None | Code-only | Not configurable. |
| Tier-rule floors per family | `kernel/hunter_v7_tier_rules.go` (taker, liquidity, zone, timing per setup). `v7_min_ai_priority` does not feed them. | None | Code-only | Not configurable. |
| Router status, timing, risk and chase gates | `hunter_v7_router.go:112,162,168,367,546,618-650`; `hunter_v7_execution.go`; `hunter_v7_risk.go` bands; timing booster thresholds | None | Code-only | Not configurable. |
| Open-review window | 8 / 6 / +4 (`kernel/engine_prompt.go:977-987`) | None | Code-only | — |
| Reachability band | 1.03 / 0.97 (`kernel/engine.go:1664-1679`) | None | Code-only | — |
| Geometry fallbacks | TP 3.0%, stop 2.0%, drift 0.5%, feasibility +0.25 (`kernel/engine.go:778-794`); `hunterV7BackendMinStopPct` 2.0; repair buffer 0.10 | `max_take_profit_price_move_pct`, `min_stop_loss_price_move_pct` and `max_entry_price_deviation_pct` exist in config. Only entry deviation has a UI input (0.1–2). | Config values honored when set; fallbacks apply when unset. | An unset field silently gets the fallback, and the UI shows nothing. |
| Data cadence | Collector `RestInterval` 30 s; slow derivatives refreshed every 10 cycles (`datafetch/collector.go:207-218`); snapshot max age 60 s (fallback 1 min); one-shot 5 min; wait timeout 3 min (`kernel/engine.go:23`) | None | Code-only | Slow fields can be ~5 min old (A5). |

**Execution and position management (trader/store)**

| Parameter | Code location and value | Frontend field | Honored? | If they disagree |
|---|---|---|---|---|
| Geometry repair mode | `geometry_repair`, where "" means repair | JSON only. Proposed: a select (repair / reject). | Yes, except C2 | The default silently rewrites price, stop and TP (C1). |
| Drift limits | Fallback 0.5%; favourable drift accepted up to 4.0%; repair band +0.10 capped at 0.80 | `max_entry_price_deviation_pct` (UI 0.1–2); the rest code-only | Partly | Favourable drift up to 4% passes whatever the UI says. |
| Wide-stop narrowing, stop edge-miss repair, TP cap, RR repair target | 0.35 / 3.20; edge-miss constants; TP cap fallback 3.0 plus feasibility raise; RR target +0.03 | None | Code-only | — |
| Single-trade loss | `max_single_trade_loss_pct`, fallback 8.0 | JSON only. Proposed field. | Yes when set | — |
| Micro-price and freshness | Micro spread 0.35; REST refresh 3500 ms / 120 s / 0.50%; freshness gate 45 s; `micro_price_consistency_max_pct` | JSON only for the config field; the rest code-only | Partly | — |
| Whale guard | 45 / 0.56 (`trader/auto_trader_risk.go:1999-2003`) | None | Code-only | — |
| Low-liquidity size tiers | Trader constants | None | Code-only | — |
| Failed-open cooldown | Next 3 decision records, keyed by symbol only (`store/decision.go:373`) | None | Code-only | A blocked short also cools down the long. |
| Recent-loss cooldown | **UNVERIFIED** (function body not traced) | None | Unknown | — |
| Daily-loss breaker | `daily_loss_breaker`: 15% / 0.8, default mode shadow. A second daily-loss knob also exists. | JSON only. Proposed: one field set. | Shadow by default | Two knobs with different meanings can both be set. |
| Conditional entry | `v7_conditional_entry`: 120 min / 10 s | JSON only | Yes | C6 |
| Protector | `protector`, about 60 fields, read through `EffectiveProtector` | JSON only. Proposed: a protector panel with the TP0 ratio, ROE levels, breakeven offset and giveback. | Yes | Leverage changes silently rescale ROE levels (D3). |
| Protector alias constants and loop intervals | `trader/auto_trader_risk.go:23-43` | None | **UNVERIFIED** whether the live loop uses them | — |
| Emergency close, naked-stop grace, read-back | 3 attempts; 20 s grace (#76); 2 × 1.5 s read-back | None | Code-only | Safety list (§5.3). |
| Other hunter_config blocks | `v7_t2_lifecycle`, `v7_data_timing`, `v7_prompt`, `v7_expected_r`, `v7_cell_rule`, `v7_yield_target`, `t2_*`, `class_profiles`, `setup_geometry_overrides` | JSON only | Yes when set, through their `Effective*` readers | Invisible in the UI. A saved UI edit keeps them only because `updateConfig` merges sections (StrategyStudioPage). |

### 5.3 Safety hard limits (owner decides)

These are candidates to stay in code, outside frontend control, or to be frontend-visible but bounded. The owner decides each one.

| Limit | Location and value | Why it is a candidate |
|---|---|---|
| Stop and TP invariants (stop on the protective side, TP beyond entry, `ClosePosition(true)`) | `trader/auto_trader_risk.go`, `trader/protector_engine.go` | A wrong value means an unprotected or self-closing position. |
| Stop `WorkingType` (`CONTRACT_PRICE`) | Binance order plumbing | An enum the exchange enforces. A wrong value rejects every stop. |
| Naked-stop grace 20 s, per-poll stop audit, emergency close 3 attempts | #76 code | The P0 C5 backstop. |
| -12% ROE protector market close | `store/protector.go` | The last backstop when stops fail. |
| Leverage ceiling (UI 20; 0 means unlimited in the trader) | `RiskControlEditor.tsx`; trader | "0 = unlimited" is a safety hole (D2). |
| Max positions upper bound | `store.MaxPositions = 3` (removed by #82) | After #82 there is no upper bound. The owner may want one as a safety limit, separate from the trading setting. |
| Token-budget clamps (`MaxCandidateCoins` 10, `MaxTimeframes` 4, kline count 10–30) | `store/strategy.go:20-26` | Protect the model context window and cost, not trading logic. |
| Margin factor `1.01/leverage + 0.001` | `trader/auto_trader_orders.go:222,398` | Prevents insufficient-margin rejects. |

### 5.4 Proposed unified config model

1. **One resolver.** Add `store.ResolveHunterV7(cfg) ResolvedConfig`, called once per cycle. It returns every effective value with its source (`config`, `default`, `safety_limit`). The kernel and the trader read only from it. This removes the duplicated `V7Config` build (`kernel/engine.go:554-581` and `2844-2872`) and the trader-only `setup_guard`.
2. **Explicit "unset" vs "off".** Use pointer or tagged fields where 0 is meaningful (leverage, cooldowns, breaker). Unset uses a documented default. 0 or `off` disables. No field uses 0 to mean both.
3. **Validate instead of clamping silently.** Out-of-range values fail the save in the API with a message (PR #82 does this for max positions). Safety limits from §5.3 are the only bounds, and the UI shows them.
4. **Frontend fields for backend-only knobs.** Add inputs, in order of trading impact: RR floor with step 0.1 and no fallback to 3; `geometry_repair`; `max_single_trade_loss_pct`; TP/stop move limits; protector core; daily-loss breaker; `setup_guard`; candidate count with the 10 cap shown. Code-only constants that are tuning parameters (regime, tier floors, universe sizes) move into config only after X1 evidence, with today's values as defaults.
5. **Scan interval ownership.** Keep `scan_interval_minutes` on the trader record, because it controls the trader's loop. Show the attached traders' intervals on the strategy page so the shared-strategy case is visible. Add an API maximum that matches the UI's 60.
6. **Show the effective values.** The resolver output (value and source) goes into each decision record and into a read-only "effective config" view in Strategy Studio, so a disagreement between frontend and code is visible.

Interplay with PR #82: #82 already implements items 2 and 3 for RR, max positions and confidence. The resolver in item 1 should be built on #82's `EffectiveMinRR()` and `ValidateMaxPositions()`.

## 6. Validation for this PR

- `go build ./...`: see the PR description.
- `go test ./... -count=1`: see the PR description. The experiment tests skip without `AIT_EXPERIMENTS=1`.
- The experiment runs reported in §3 used `AIT_EXPERIMENTS=1`.
