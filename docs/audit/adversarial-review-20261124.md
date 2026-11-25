# Adversarial audit of Hunter v7 against the standing design standard (issue #69)

Date: 2026-11-24. Base: `t2/v7-selection` @ `461074e8`. Audit only: no screening, router, tiering, prompt, execution or position-management behaviour was changed. Line numbers refer to `461074e8`.

Design standard under test (owner, 2026-11-20, `.kiro/steering/design-standard.md` on `t2/design-standard` @ `f938934`):

1. Every round screens all of the hundreds of available contracts, in any market regime or pattern.
2. It surfaces 3–5 targets whose direction the latest indicators can call, with RR ≥ 1.2, worth entering now.
3. The LLM receives the complete, latest indicators for those targets.
4. The LLM's JSON decision goes to the engine, which enters at market.

## Executive summary

**Verdict: partially.** Point 4 (LLM JSON → engine → market entry) is built and well guarded. Points 1–3 are not met as built.

Strongest evidence against:

1. **The universe is the union of fixed top-N lists, so it is neither "all" nor stable.** `BuildV7Universe` fills each list to N even when the ranked value is 0, and breaks ties by map order with an unstable sort (`provider/local/hunter_v7_universe.go:115-158`). On one snapshot with 420 valid symbols it returned 30 different universes in 30 calls, each with 195–210 symbols (test `TestAudit_A1_UniverseSelectionIsNondeterministic`). That size matches the "~195 pairs/round" measured in Tier A. The 350 cap (`:238-259`) is rarely what limits coverage.
2. **Nothing produces "3–5 worth entering".**
   - The router only pads to `MinOutput 3` with WATCH-grade rows (`provider/local/hunter_v7_router.go:431-469`, `hunter_v7_types.go:622-628`).
   - No code sets a minimum count of EXECUTABLE/REVIEWABLE candidates.
   - The recorded Tier A outcome (0.64 LLM-ready per round, 88% WATCH) cannot be reproduced here: the stored rerun data is not in this repository (see §E4).
3. **The LLM's view and the execution gate use different tiers, and "no data" counts as passing evidence.**
   - The prompt re-tiers every candidate at the live price, but that verdict is "recorded only" (`kernel/engine_prompt.go:707`). The AI-skip check (`trader/auto_trader_loop.go:414`) and the WATCH/REJECTED execution gate (`trader/auto_trader_risk.go:1612`) read the assembly tier (`TestAudit_B7_…`).
   - A `trend_breakout_long` with no taker data at all is EXECUTABLE at assembly; the same coin with measured weak taker is WATCH (`kernel/engine.go:2020-2046`, `TestAudit_A3_…`).

Evidence for:

- Every Hunter v7 open must match this cycle's candidate set and the signal contract: id, symbol, direction, setup and tier (`trader/auto_trader_risk.go:1593-1623`, `1839-1876`).
- The entry-drift check uses a freshly fetched order-book price, not the prompt price (`trader/auto_trader_orders.go:37-63`, `trader/auto_trader_risk.go:2253-2271`).
- The position protector runs in its own goroutine (`trader/auto_trader_risk.go:97-117`) and moves real exchange stops (`trader/auto_trader_protection_rebuild.go:83-114`).

The most serious safety finding is AUD-C5: some open paths end with a filled position and no exchange stop, and nothing re-arms it.

## Method

- Code reading of every module named in the issue, with file:line evidence quoted in the findings table.
- Adversarial unit tests, added in three files:
  - `kernel/audit_adversarial_20261124_test.go`
  - `trader/audit_adversarial_20261124_test.go`
  - `provider/local/audit_adversarial_20261124_test.go`

  Each test characterises the current behaviour. It passes today and carries a `FLAW` assertion that the fix must invert. The suite stays green, as required for an audit that changes no logic. Each test name starts with `TestAudit_<finding id>`.
- Out of scope:
  - No live trading, no shadow phase, no Tier A rerun.
  - The fee-net tool (#55) and the rerun DB were not run: the DB is not in the repository or the sandbox.
  - Proposed experiments are described in §6 instead.

## Per-area verdicts

| Area | Verdict | Summary |
|---|---|---|
| A. Screening | **Flaw** | Coverage is set by top-N list sizes and is nondeterministic (A1). There is one global BTC/ETH regime, with compression over-firing (A2). Missing data passes taker gates (A3). Slow fields are carried without an age (A5). Nothing guarantees a count (A6). |
| B. Evidence → LLM | **Gap** | The structural guard is strong, but the prompt tier and the execution tier diverge (B7). The parser accepts prose as a successful wait (B1), the first decision block wins (B3), and `no_reviewable_candidate` is unchecked (B2). Plain-text prompt lines are not escaped (B6). Nothing measures rubber-stamping (B8). |
| C. Execution | **Flaw** | The drift is checked on a fresh price, but repair mode re-anchors it twice (C1) and the TP cap ignores reject mode (C2). The position cache can let max_positions be exceeded (C3). Fills are not confirmed (C4). Naked positions after an unknown order state are not re-armed (C5). Conditional fills skip the execution guard (C6). |
| D. Position management | **Gap** | Real stop movement exists (dynamic trail + breakeven rebuild). Defects: wrong-side rebuild after a giveback (D1), leverage 0 or an int leverage disables or mis-scales ROE rules (D2), ROE TP0 at high leverage (D3), state lost on restart (D4), TP silently removed (D5), LLM close vs protector race (D6). The claimed TP0 protocol differs from the code (D7). |
| E. Cross-cutting | **Gap** | The freshness guard trusts the model's `signal_age_ms` (E1). The decision record is written after the orders (E3). Headline numbers are not reproducible from the repo (E4). AI-failure handling is acceptable but tied to B1. |

## Findings table

Severity: **P0** = money-losing or safety-critical. **P1** = design deviation that changes outcomes. **P2** = robustness or polish.

Repro column:
- "test" = an adversarial unit test reproduces it.
- "code" = established by code reading only.
- "re-check after #67/#68" = interacts with a pending PR.

| ID | Area | Sev | file:line | Claim vs reality | Repro | Fix direction (do not implement here) |
|---|---|---|---|---|---|---|
| AUD-C5 | C/D | **P0** | `trader/auto_trader_open_intent.go:55-104`; `trader/auto_trader_protection.go:35-39`; `trader/auto_trader_protection_rebuild.go:108-110` | Claim: every open is protected or closed. Reality: when the open-order read-back is inconclusive, the code returns `order_state_unknown` without placing a stop, although the order may have filled. When the emergency close fails, the code only returns "position may be unprotected". No poll checks for a missing stop and re-arms it. The only backstop is the protector's polled market close at -12% ROE (30 s cadence). | code | Run a per-poll "stop present?" audit that re-arms a stop from the plan or a default distance. Treat `order_state_unknown` as "assume filled": query the position and place a stop. Alert on an unresolved naked position. |
| AUD-C3 | C | **P0** | `trader/binance/futures.go:82` (15 s position cache); `trader/auto_trader_orders.go:162-168`, `333-339`; only `trader/auto_trader_loop.go:561-564` invalidates the cache | Claim: max_positions = 1 is code-enforced. Reality: each open re-reads `GetPositions()`, which is cached for 15 s and is not invalidated after an order. Two opens in one response, executed within 15 s, both see 0 positions. No kernel cap exists on the number of opens per response. | code (needs a fake trader test) | Invalidate the cache after every order and count the opens accepted this cycle. Cap opens per response at the free slots before execution. |
| AUD-A3 | A | P1 | `kernel/engine.go:2020-2046` (`hunterV7TakerBuyAtLeast/AtMost` return true when the derivatives context is nil or `TakerBuy15m <= 0`); `kernel/hunter_v7_tier_rules.go:806-894`, `834` (`MinLiquidity` skipped when the score is 0) | Claim: tiers are driven by data indicators. Reality: missing taker or liquidity data passes the gate. A `trend_breakout_long` with no derivatives context is EXECUTABLE (`long_setup_ready_confirmed`); the same coin with taker 0.40 is WATCH. The prompt's readiness pass caps it at REVIEWABLE (`prompt_readiness_derivatives_context_missing`), but the trader reads the assembly tier (see B7). | test `TestAudit_A3_*` | Fail closed, or demote to REVIEWABLE, on missing data for rows that gate on it. Make the dead `AllowMissing` / `RequireConfirmAll` fields (`hunter_v7_tier_rules.go:79-86`, `913-930`) real or delete them. |
| AUD-B7 | B | P1 | `kernel/engine_prompt.go:707` ("recorded only"), `:857-866` (re-tiering); `trader/auto_trader_loop.go:414` (skip), `trader/auto_trader_risk.go:1612` (gate), both reading `ctx.CandidateCoins[].V7ExecutionTier` | Claim: the LLM adjudicates the candidates the engine will execute. Reality: there are two tier verdicts per cycle. Fixture: assembly WATCH, prompt REVIEWABLE; the trader still sees WATCH. Consequences:<br>(a) When flat and the assembly tier has no E/R candidate, the AI is skipped, so candidates promoted at the prompt are never seen.<br>(b) An open on a candidate the prompt promoted is blocked as WATCH.<br>(c) A candidate the prompt demoted to WATCH, but that was EXECUTABLE at assembly, passes the gate. | test `TestAudit_B7_*` | Make the prompt-final tier authoritative: write it back to `ctx.CandidateCoins`, and run the AI-skip check and the execution gate on it. Re-check after #67 merges. |
| AUD-A1 | A | P1 | `provider/local/hunter_v7_universe.go:115-158` (top-N lists filled regardless of value; `sort.Slice` is unstable), `:238-259` (cap tie-break over a map) | Claim: every round screens the hundreds of pairs. Reality: about 200 symbols, chosen as the union of lists. Membership changes run to run on the same snapshot: 30 calls gave 30 distinct sets of 195–210. Side-pool members (priority 200–220) and core majors (999) are cut first when the cap applies. | test `TestAudit_A1_*` | Rank all valid symbols (or a documented, deterministic subset). Skip zero-value list fillers. Use a stable sort with a symbol tie-break. Log coverage per round. |
| AUD-A2 | A | P1 | `provider/local/hunter_v7_regime.go:17-79` (BTC/ETH only, priority order), `:218-229` (`isCompression`: any `bbWidth > 0` with ADX < 25), `:232-237` (`isRange` checked after) | Claim: per-regime rules. Reality: one global regime from BTC/ETH, with no hysteresis. Compression has no band-width threshold and shadows range: any quiet non-trending tape is "compression". Range fires only if one major has ADX ≥ 25. An alt trending against BTC gets `regime_against_direction` (+15 risk, `hunter_v7_risk.go:73-77`). The issue's regime list (`trend_continuation`) does not exist in code; there are 9 values. | test `TestAudit_A2_*` | Add a BB-width percentile to compression, reorder compression and range, add hysteresis. Consider a per-symbol trend context before applying `regime_against_direction`. Fix the canonical regime list (same question as #67 Q2). |
| AUD-C1 | C | P1 | `trader/auto_trader_risk.go:986-1030` (`repairHunterV7DecisionPrice`), called at `trader/auto_trader_orders.go:235/406` and again at `trader/auto_trader_risk.go:1198` | Claim: orders far from the measured entry price are rejected. Reality: in the default `geometry_repair` mode each pass re-anchors `decision.Price` to the live price when the drift is within max + 0.10 (≤ 0.80). Favourable drift up to 4% is always re-anchored. Two passes stack: an LLM price of 100 executed against a reference of 101.10 is 1.10% adverse drift accepted with a 0.50% limit. A move through the model's own stop is still rejected (locked by `TestAudit_C1b_*`). | test `TestAudit_C1_*` | Measure drift against the LLM's original price (`decisionPriceBeforeRepair`) in every pass, or make reject mode the default for hunter_v7. |
| AUD-C2 | C | P2 | `trader/auto_trader_risk.go:869-893` (no reject-mode check) vs the contract at `:934-951` | Claim: reject mode never rewrites price, stop or TP. Reality: `capTakeProfitToTP1` still rewrites the TP (110 → 104). | test `TestAudit_C2_*` | Reject instead of capping when `geometry_repair=reject`. |
| AUD-C4 | C | P1 | `trader/binance/futures_orders.go:108-114` (result has no `executedQty`/`avgPrice`); `trader/auto_trader_decision.go:244-284` (Binance returns early: "synced by OrderSync") | Claim: the decision is executed as recorded. Reality: the requested quantity and the pre-order price are recorded, with no fill confirmation. Stops use `ClosePosition(true)`, so a partial fill stays covered. The record's size, RR and risk can be wrong. | code | Read back the fill (`executedQty`, `avgPrice`) before recording and before computing stop and TP. |
| AUD-C6 | C | P2 | `trader/hunter_v7_conditional_entry.go:202-243` | Conditional entries fire through the open path, so drift is checked at fire time. They do not re-run `validateHunterV7ExecutionGuard` (tier, tags, freshness). They count only positions for max_positions. Pending entries live in memory and are lost on restart. | code | Re-run the guard at fire time on the latest snapshot. Persist pending entries or cancel them visibly on restart. |
| AUD-C7 | C | P2 | `trader/auto_trader_orders.go:149-318` (about 10–15 sequential REST calls, plus a 5 s sleep when leverage changes, `trader/binance/futures_positions.go:258-259`); `trader/auto_trader_loop.go:366` (1 s sleep per success) | The race window is the LLM call (10–60 s or more) plus several seconds of REST calls after the last price check, then a bare MARKET order with no price protection. | code | Re-check the book after the last pre-order call. Consider a marketable limit with a drift band. |
| AUD-B1 | B/E | P1 | `kernel/engine_analysis.go:674-688` (no JSON → synthetic wait, err = nil); `trader/auto_trader_loop.go:~211-219` (success resets `consecutiveAIFailures`) | A prose-only (or truncated) model reply is a successful wait. It resets the failure counter and safe mode, and hides a broken model or gateway. Re-check after #68: the stream path's JSON fallback reduces truncation but not prose replies. | test `TestAudit_B1_*` | Treat "no structured decision" as an AI failure, with a distinct reason. |
| AUD-B3 | B | P2 | `kernel/engine_analysis.go:648-709` (first `<decision>` match; non-greedy `\[\s*\{.*?\}\s*\]`) | An example `<decision>` block in the reasoning is executed instead of the real one. | test `TestAudit_B3_*` | Take the last block, or reject a reply that contains more than one. |
| AUD-B2 | B | P2 | `kernel/engine_analysis.go:515-525`, `566-577` | `no_reviewable_candidate` is accepted while REVIEWABLE > 0. The validator has no tier input. When flat with E = R = 0 the AI is skipped entirely (`trader/auto_trader_loop.go:407-455`), and that synthetic wait carries no `blocked_reason_code`. | test `TestAudit_B2_*` | Pass the prompt-final tier counts to the validator. Stamp the skip record with `no_reviewable_candidate`. |
| AUD-B4 | B | P2 | `kernel/engine_position.go:157-160` (`minConfidenceHard = 60`); `trader/auto_trader_risk.go:2279-2282` | The prompt says 70. The kernel floor is 60. The trader enforces `effectiveMinOpenConfidence` only when the strategy sets `MinConfidence`. | test `TestAudit_B4_*` | Use one effective floor in both layers. |
| AUD-B5 | B | P2 | `kernel/engine_position.go:110-123` | The kernel RR uses the model's `trigger.trigger_price` as the entry, so a trigger next to the stop inflates RR. The trader re-checks market opens at the live price (`auto_trader_risk.go:2296-2335`). Conditional entries are validated at the trigger and fill at market after the 1m close. | test `TestAudit_B5_*` | Validate RR at the backend's trigger (`HunterV7TriggerPrice`), not the model's, and re-check at fill. |
| AUD-B6 | B | P2 | `kernel/engine_prompt.go:754-761`, `784-786`, `806-808`, `~1148` (`%s` with Symbol, setup and reason) | Plain-text candidate lines are not escaped; a newline in a symbol or reason starts a new prompt line. The JSON payload is escaped (`hunter_v7_prompt_payload.go:278-284`). Exploitability depends on exchange symbol sanitisation, which was not found. | test `TestAudit_B6_*` | Sanitise exchange strings at ingestion (charset `[A-Z0-9]`), or escape in the formatters. |
| AUD-B8 | B | P2 | `kernel/engine_prompt.go:881-898` (sort: tier, ReadyScore, ai_priority), `:744-757` (only the top N expanded) | It is not possible to tell whether the LLM adjudicates or rubber-stamps: the order is priority-biased by design, and no metric compares the pick with rank #1. | code | Record the rank of the chosen candidate per decision. See experiment X2. |
| AUD-D1 | D | P1 | `trader/protector_engine.go:268-300` (giveback at `cur <= 0`), `:534-545` (`rebuildStop` = entry × (1 + 0.15%), no mark check); `trader/auto_trader_protection_rebuild.go:83-114` | The staged 50% giveback rebuilds a LONG stop above mark. The exchange rejects it, the old stop is already cancelled, and the path falls into emergency close. The result is a 100% close through the error path after a naked window. | test `TestAudit_D1_*` | Clamp the rebuild stop to the protective side of mark (the dynamic path already has `isStopOnProtectiveSide`), or close fully by design. |
| AUD-D2 | D | P1 | `trader/auto_trader_risk.go:149-152`, `311-321`; `trader/gate/trader_account.go:93` (int leverage) | Leverage 0 gives ROE 0 and switches off hard-loss, early-loss, TP0, giveback and profit-floor rules. A −15% move takes no action. `DefaultLeverage` applies only when the value is not a float64. A parsed 0.0 never reaches it, and the Gate adapter's int leverage always gets the default 10. Binance v2 `positionRisk` does return leverage, so this is latent on Binance. | test `TestAudit_D2_*` | Accept int and float, and treat ≤ 0 as unknown → default plus a warning. |
| AUD-D3 | D | P2 | `store/protector.go:155-170` (ROE levels, `TP0CloseRatio 0.35`); `trader/protector_engine.go:268-350` | ROE levels shrink with leverage. At 20x, TP0 closes 35% on a 0.4% move (about 4× the round-trip taker fee). The profit floor locks 3% ROE = 0.15% price. No fee or slippage model in breakeven. | test `TestAudit_D3_*` | Express TP0 and the floor in price % or R for high leverage, or add a minimum price move like TP1's. |
| AUD-D4 | D | P1 | `trader/auto_trader_risk.go:337-384` (in-memory state, `positionFirstSeenTime` = now), `:608-640` (peak recovered from the last 20 prompts only) | After a restart: TP0/TP1/giveback flags reset, so partials can fire again on the reduced remainder. The peak can reset, so a +15% peak restarted at +3% loses its giveback and lock rules. The early-loss and pre-TP clocks restart. The exchange stop survives. | code | Persist protection state (flags, peak, open time) per position, or rebuild it from the order and fill history. |
| AUD-D5 | D | P2 | `trader/auto_trader_protection_rebuild.go:126-149`; `trader/auto_trader_loop.go:965-1008` (plan from the last 50 records) | When the plan is not found (restart, or more than 50 records), `ExchangeTakeProfit = 0`. After a partial, the rebuild cancels the TP (`stopsOnly=false`) and never re-places it. | code | Re-place the TP read from the exchange, or skip TP cancellation when no replacement exists. |
| AUD-D6 | D | P2 | `trader/auto_trader_orders.go:491-555` (LLM close + `ClearPeakPnLCache`); no lock shared with the protector goroutine | An LLM close and a protector partial or rebuild can interleave. Effects: reduce-only rejections, false "unprotected" emergency loops, an orphan `closePosition` stop on a flat side, and state recreated after a clear. | code | Lock per position for close and protect actions, and cancel owned protection after an LLM close. |
| AUD-D7 | D | P2 | `store/protector.go:170` (TP0 35%); `trader/hunter_v7_dynamic_stop.go:32-49` (trail 0.4R of a synthetic 35% ROE base risk); `kernel/hunter_v7_prompt_doctrine.go` S4 | Claim (issue §D, doctrine S4): reduce 30–50% at TP0, then trail at breakeven or 5m EMA20/VWAP. Reality: a fixed 35% partial, breakeven + 0.10%, a synthetic 0.4R trail and %-of-peak locks. No EMA20/VWAP trail exists. The prompt tells the LLM about a mechanism the backend does not run. Re-check after #67: the redesigned doctrine keeps this sentence. | code | Align the doctrine with the protector, or implement the EMA20/VWAP trail. |
| AUD-E1 | E | P1 | `trader/auto_trader_risk.go:1877-1899` | The pre-order freshness guard prefers the model's `signal_age_ms`. `signal_age_ms: 1` passes a 120 s-old high-volatility signal. Ordinary candidates have no age gate at all. | test `TestAudit_E1_*` | Use backend ages only. Apply an age gate to every tier, with per-class limits. |
| AUD-A5 | A/E | P2 | `datafetch/collector.go:207-220`, `238-257` (slow fields carried for 9 of 10 cycles, about 4.5 min at 30 s); `kernel/engine_snapshot.go:343-351` (freshness = `snap.CreatedAt` only) | OIΔ1h/4h, LSR, depth and 1h/4h structural klines can be about 5 min old while the snapshot reads as fresh. Tiering and the universe's OI ranking use them. The harness's slower layer (#52, "~26 min") was not verified. | code | Stamp a per-field fetch time, and expose and gate on its age. |
| AUD-A6 | A | P1 | `provider/local/hunter_v7_router.go:431-469`; `kernel/engine.go:555-581` (duplicated at `2846-2872`) | No mechanism targets "3–5 entry-worthy signals". The floor pads with wait-confirm rows; prompt demotions have no floor. The `MaxOutput`/`HunterLimit` wiring is duplicated and can drift. | code | Define the funnel target explicitly (count of E/R per round), measure it, and decide with fee-net evidence (X1). |
| AUD-E3 | E | P2 | `trader/auto_trader_loop.go:373` (`saveDecision` after all orders) | A crash between fill and save leaves a live position with no decision record, so the plan lookup (D5) fails. | code | Write an intent record before ordering and update it after. |
| AUD-E4 | E | P2 | `cmd/hunter_v7_validate/main.go:1219-1262` (`buildValidationRunSummary`, `reportOpenReviewCount`), `:1800` (`parsePromptTierCounts`) | The headline Tier A figures cannot be recomputed here because the stored rerun output is not in the repo. "0.64/round" is not a code field; it equals (2 + 90)/144, so it was derived by hand. "88% WATCH" is not a field either. The harness never calls the LLM (`buildPrompt`, `main.go:1603-1623`), so "LLM-ready" means prompt-final E + R, not LLM decisions. | code | Emit per-round averages and tier shares in the run summary, plus the down-market label (validation sampling rule §4). |

## Answers to the issue's specific questions

**A. Screening**

- **False positives.**
  - AUD-A3: missing data passes the taker and liquidity gates.
  - `OpenRateFloor` rows carry no taker, zone, OI or stop gate (`kernel/hunter_v7_tier_rules.go`, e.g. the breakout `OpenRateFloor`).
- **Misclassified regimes.** AUD-A2.
- **Stale data scored as fresh.** AUD-A5 and AUD-E1.
- **Sample bias.** The Tier A rerun sample is DOWN-MARKET-ONLY. Under `docs/hunter-v7-validation-sampling-rule.md` §4 it cannot justify loosening WATCH or lifting the short-ignition shadow-only status (rows: short ignition n=291, WATCH 1,312, REVIEWABLE 104).
- **WATCH pile-up: too strict or a real signal?** Undecided. Mechanisms that inflate WATCH regardless of signal quality:
  - Prompt-time zone and readiness demotions at the live price (`kernel/engine_prompt.go:925-940`).
  - The max-of-two stale zone positions in `hunterV7EntryZonePositionPct` (`kernel/engine.go:3474-3495`). A position of exactly 0 is treated as unknown, and the larger, more extended reading wins.
  - Router padding with wait-confirm rows.

  The fee-net question needs X1.
- **Entropy budget.** It is a code-size lint test only (`kernel/hunter_v7_entropy_budget_test.go`), with no runtime effect.
- **Whale confirm pool.** Shadow only (`kernel/hunter_v7_whale_confirm_pool.go:3-14`). It cannot promote signals.

**B. Adjudication**

- **Can the LLM override backend-verified RR?** Not upward. In the default repair mode the backend instead rewrites the LLM's price, stop and TP to meet RR (`trader/auto_trader_risk.go:957-985`), so the engine does not execute the LLM's target as written (AUD-C1). A stricter LLM can only wait.
- **Contradictions between doctrine, sections and output contract.** Listed in the #67 notes (C1–C10). Re-check after #67 merges.
- **Can data fields steer the decision?** Through unescaped plain-text lines (AUD-B6), and through the model-supplied age (AUD-E1).
- **Is `no_reviewable_candidate` the only wait path when E = R = 0?** When flat, the AI is skipped and the record has no code (AUD-B2). With positions open, the LLM is called and any code is accepted.

**C. Execution**

- **Fresh or stale price for the deviation check?** Fresh order-book price (good). In repair mode the reference is re-anchored (AUD-C1).
- **Race window.** About 15–90 s from the price snapshot to the order (AUD-C7).
- **Rejected entry.** Not retried or queued. It is logged in the decision record. `GetRecentFailedOpenSymbols` (`store/decision.go:373`) then marks the symbol as failed-open for the next 3 decision records, keyed by symbol only, so a blocked short also cools down the long.
- **Partial fills.** Recorded at the requested size (AUD-C4).

**D. Position management**

- **Is there a real trailing stop?** Yes: real exchange stops are moved (`trader/auto_trader_risk.go:177-197`). But it is not the TP0 protocol described in the doctrine (AUD-D7).
- **Can a winner round-trip after TP0?** Yes, in two ways:
  - Breakeven + 0.10% is about the round-trip fee.
  - A failed rebuild keeps the old, wider stop while `TP0Done` is set, and nothing retries the breakeven stop (`trader/auto_trader_risk.go:431`, `472`).
- **Stop placement fails after the fill.** Emergency close with 3 attempts; then naked (AUD-C5).
- **Exchange rejects the stop.** AUD-D1.
- **Restart.** AUD-D4.
- **#56 tracker fix.** This is the validation harness's `RESTART_ORPHANED` path (`cmd/hunter_v7_validate/tracker_resume.go:20-41`) and is separate from live recovery. Remaining ways for a row to stay ACTIVE:
  - a state file that predates fix 09 (`:87`);
  - runs without `--resume`;
  - rows written by live paths.

**E. Cross-cutting**

- **Model call failure.** 3 consecutive failures turn on safe mode, which blocks opens (`trader/auto_trader_loop.go:~137-256`). The protector is independent of the decision loop. B1 undermines the counter. Re-check after #68 merges: the streaming fix changes the error surface for WorkBuddy.

## Known open items (verified, not re-litigated)

| Item | Status found |
|---|---|
| Fee-net tool (#55) not run on the rerun DB | Confirmed: the DB is not in this repository or sandbox, so the tool could not be run. Both questions (is WATCH too strict; should short ignition stay shadow-only) remain unanswered. |
| #52: 12 vs 8 workers, slow 1h layer ~26 min | Not verified. `datafetch/fetch_tiered.go` defaults `MaxWorkers` to 5 and the collector to 50. The harness flag is `--max-workers` (`cmd/hunter_v7_validate/main.go:223`, default 8). No "26 min" constant exists; it is a measurement. |
| Down-market-only sample | Confirmed by the validation sampling rule. Any WATCH/short-ignition conclusion from it must carry `DOWN-MARKET-ONLY`. |
| 0.64 LLM-ready per round, 88% WATCH | Not reproducible from the repo (AUD-E4). Code-level causes are listed under A above. |

## Proposed experiments (not run)

- **X1. Fee-net on the stored rerun DB (#55).**
  1. `go run ./cmd/v7_fee_net_offline -db <rerun.db> -out reports/fee-net-rerun -horizon 2h -tiers WATCH,REVIEWABLE,EXECUTABLE` (the DB is opened read-only; flags at `cmd/v7_fee_net_offline/main.go:42-55`).
  2. Group by prompt-final tier and by `short_ignition`.
  3. Label every output `DOWN-MARKET-ONLY`.
  4. Repeat on a non-down-market sample once the coverage test (#16 rule) passes.
- **X2. Rubber-stamp measurement.**
  1. Log `chosen_rank` (the position of the selected `signal_id` in the prompt order) and the number of E/R candidates per decision.
  2. Over at least 200 live or paper decisions, compare P(choose #1) with the share of #1 among the passable candidates.
- **X3. Universe stability.** Run `BuildV7Universe` 50× on one stored snapshot and report the Jaccard overlap of the sets and the coverage (symbols ranked / valid symbols).
- **X4. Tier split-brain rate.** Over the stored rounds, count the candidates whose prompt-final tier differs from the assembly tier (`ApplyHunterV7PromptFinalTiers` already persists both, fix 12).
- **X5. Naked-position drill.** On testnet, inject a `SetStopLoss` failure and a read-back failure, then record the time to protection or close.

## Adversarial tests added

| Test | Finding |
|---|---|
| `kernel.TestAudit_A3_MissingTakerDataPassesTierTakerGates`, `TestAudit_A3_TrendBreakoutExecutableWithoutTakerData` | A3 |
| `kernel.TestAudit_B1_ProseOnlyResponseIsSilentSuccessfulWait` | B1 |
| `kernel.TestAudit_B2_NoReviewableCandidateNotCheckedAgainstTiers` | B2 |
| `kernel.TestAudit_B3_FirstDecisionBlockWins` | B3 |
| `kernel.TestAudit_B4_KernelConfidenceFloorIs60` | B4 |
| `kernel.TestAudit_B5_TriggerPriceInflatesKernelRR` | B5 |
| `kernel.TestAudit_B6_PlainTextCandidateLinesAreNotEscaped` | B6 |
| `kernel.TestAudit_B7_PromptTierIsExecutionTier` (inverted by #86) | B7 |
| `trader.TestAudit_C1_RepairModeDriftStacksAcrossPasses` | C1 |
| `trader.TestAudit_C1b_DriftThroughStopIsStillRejected` | locks correct behaviour |
| `trader.TestAudit_C2_TakeProfitCapIgnoresRejectMode` | C2 |
| `trader.TestAudit_E1_FreshnessGuardTrustsModelSignalAge` | E1 |
| `trader.TestAudit_D1_GivebackRebuildsStopOnTheWrongSideOfMark` | D1 |
| `trader.TestAudit_D2_ZeroLeverageSwitchesOffROERules` | D2 |
| `trader.TestAudit_D3_TP0At20xFiresOnFourTenthsPercentMove` | D3 |
| `local.TestAudit_A1_UniverseSelectionIsNondeterministic` | A1 |
| `local.TestAudit_A2_CompressionShadowsRangeAndIgnoresBandWidth` | A2 |

Findings without a test (C3, C4, C5, C6, C7, D4–D7, A5, A6, E3, E4) need a fake exchange with failure injection or stored run data. They are marked "code" in the table.
