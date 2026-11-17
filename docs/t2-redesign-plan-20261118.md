# Hunter v7 + Task-2: screening, decision and execution chain redesign plan

Date: 2026-11-18. Base: `t2/v7-selection` @ `1658b70`. Author: Kiro. Status: PLAN ONLY. Nothing in this document is implemented. Each item needs owner approval before it is built.

## 0. How to read this plan

### 0.1 Evidence tags

Every design claim carries one of these tags:

- **[M:…]**: a measured number. The source is named in the brackets. Sources used:
  - `694` is the 694-event replay.
  - `M2v1` and `M2v2` are the structural geometry evals.
  - `MEAS1`, `MEAS2` and `MEAS3` are the three measurements Muse delivered on 2026-11-17: RR distribution, tiering, and native yield plus co-fire.
  - `LAT` is the latency figures the owner restated in the R2 brief.
  - `F-audit` is the F01–F26 audit.
  - `SSA` is the screening-stack audit, sections A–I.
  - `DOC:file:line` is a measured number in a repo report under `docs/`.
- **[C:file:line]**: a code fact. I read it in the tree at `1658b70`.
- **[D]**: arithmetic derived from tagged numbers. The derivation is shown.
- **[H]**: HYPOTHESIS. Each one names the pre-registered test that would settle it. No [H] item may become a gate until its test has passed.

Native repo reports (`DOC:`) are gross price-move percentages without fees. No report states fee deduction (see §1.4 #7). They are therefore not comparable with the fee-net R walks of the 694. Where this plan sets them side by side, it only compares directions, never magnitudes.

### 0.2 Gaps in what I hold

1. **The 694 data is not in the sandbox.** I searched the whole filesystem for the M3 `snapshots.jsonl`, the entry-study `events.json` and the 1m klines. None of them are there. So I cannot compute the new tables R1(a), R3 and R4 myself. I have specified them as pre-registered analyses in Unit U0 (§6), with acceptance checks that tie them to numbers already measured. Wherever a table is required but not yet computed, the plan says "PENDING U0" and does not guess a value.
2. **SSA §H.3 and §I are not in my working context.** The per-family lag verdict table (SSA §H.3) and the section-I yield text were delivered earlier in this session as chat text, and that text was later compacted out of my context. Where this plan depends on them it cites them by reference. U0 re-derives the H.3 table from the 694 so that the class assignments in §3 can be checked against it. I ask Muse to attach the H.3 table to the PR so that check can happen now.
3. **The T2 scanner spec is missing.** `T2_SCANNER_RULES_SPEC.md` is not in any commit on any branch. The R1 brief states that WATCH had its own trigger-promotion path in the original scanner. This plan takes that as owner-provided fact. The trigger details (which price counts, what a cross is, whether the event ends on trigger) are still open questions (OD-3).

---

## 1. Executive summary

### 1.1 Five findings

1. **The 82 threshold is not a calibrated threshold; it is a fatal one, and it blocks most of the opportunities the geometry eval admits.**
   - The port maps "score < 82" to WATCH [C:provider/local/hunter_v7_t2_setups.go:887-892]. WATCH has no onward path, because TRIGGERED is out of scope [C:provider/local/hunter_v7_t2_lifecycle.go:20-23].
   - 516 of 694 events (74.4%) are score-capped to WATCH at function time [M:MEAS2]. That includes all 396 ignition events, because ignition's score ceiling of 78 is below 82 by construction [C:hunter_v7_t2_setups.go:1304-1374].
   - At the owner's RR floor of 1.2, structural geometry admits 19 events [M:MEAS1]. At least 14 of those 19 (73.7%) are ignition [D: 7 of the original 9 plus 7 of the 10 in the [1.2,1.5) band]. The port can never review any of them.
   - The score axis itself was measured anti-predictive on the 694 (owner brief, R1). So no level of the same score is defensible as a gate. The problem is the axis, not the number 82.
2. **One generic geometry gate is applied to classes whose stop scale differs.**
   - The assembly check floors risk at 2.0% + 0.10 [C:kernel/engine.go:1898-1956].
   - T2 stops cluster near 1.0%, and 95.6% are below 2% [M:MEAS2]. That clash produces 522 assembly rejects [M:MEAS2].
   - The 2% floor was introduced after one incident: a funding-reversal stop about 1.5% away was hit by a 36-second bounce [M:DOC:hunter-v7-funding-reversal-risk-fix-2026-06-07.md:15,77]. That is n=1, and the floor was then applied to every family.
   - Momentum scores 0 of 218 under structural geometry [M:M2v2]. A structural-only gate cannot fit the impulse class.
3. **Latency is a priced cost.** A one-bar entry delay cost 18.6R on the same population: −61.5R at the trigger bar vs −80.1R at the decision bar [M:LAT]. The run-away rate rises with the consumed band: 0%, 32%, 85% [M:LAT].
   - Today's chain scans on a plain 3-minute wall-clock ticker that is not aligned to bar closes [C:store/trader.go:29, trader/auto_trader.go:717].
   - It also includes the still-forming bar in its features [C:datafetch/http_client.go:300-322].
   - Note that both entry conventions lose money on that population. The measurement prices delay; it does not establish an edge.
4. **Most measured direction conflicts are not twin conflicts.**
   - Of the 62 final-level opposite-direction co-fires [M:MEAS3], the two leading native families are funding_reversal (23) and distribution_short (21). Neither is a twin of any T2 family.
   - So merging the four twin pairs can remove at most 18 of the 62 (29%) [D: 62 − 23 − 21, assuming the two leader counts don't overlap]. That is an upper bound.
   - funding_reversal produced 0 opens out of 2,809 native outputs over 30 days, across all regimes [M:DOC:hunter-v7-route-prompt-outcome-analysis-20260804.md §3: 1183 + 1036 + 567 + 23]. It is context, not a trade class. Under the native resolver's current rule (both sides > 60 → `conflict_watch`) [C:provider/local/hunter_v7_conflict.go:55], it can veto a tradable T2 signal.
5. **Nothing in either stack is calibrated against outcomes.**
   - No repo report contains a score-band or AIPriority-band vs outcome analysis [M:DOC search, all reports].
   - The native AIPriority is a hand-weighted sum with hand bonuses [C:provider/local/hunter_v7_router.go:395-417].
   - 48 inline AIPriority checks use 13 distinct values [M:DOC:hunter-v7-lean-core-redesign-20260726.md:52].
   - The largest RR-band cell on the T2 side has n=13 [M:MEAS1]. No cell has enough samples to earn a looser gate on evidence. The owner's RR 1.2 setting is therefore a policy decision under probation (OD-9), not an evidence-earned one.

### 1.2 Proposed design in one paragraph

Each family is assigned to one of four signal **classes**:

| Class | Members | Character |
|---|---|---|
| IMPULSE | momentum and displacement families | fast continuation, lag-sensitive |
| TRIGGER | breakout, ignition, accumulation and compression families | pending at a recorded level, enter on the cross |
| REVERSAL | pullback, panic, distribution and reversion families | counter-move, needs confirmation |
| CONTEXT | funding-event and pre-watch families | information only, never a candidate |

Each class gets its own path:

- a lifecycle (P1's TRIGGERED stage is the TRIGGER-class engine);
- a geometry model (structural, chase-ATR or hybrid);
- a latency budget and freshness contract, expressed through the drift contract (D3 forbids gating on the consumed share);
- an LLM role.

The single global score threshold is replaced by three things:

1. Lifecycle reachability: a signal is reviewable when its class lifecycle says so, for example on trigger.
2. A per-cell admission status: EARNED, PROBATION or BLOCKED for each class × RR band, using a pre-registered, sample-size-aware rule.
3. Ranking by a calibrated expected-R estimate. It runs in shadow until its own falsification test passes.

Every admitted signal is tagged with class, family, band, profile version, calibration version and doctrine version, so forward outcomes keep the table honest. A single final conflict pass guarantees one direction per symbol per cycle, and CONTEXT signals do not vote in it.

### 1.3 What proceeds now and what changes

- **P1 (lifecycle reachability) continues.** Every design here needs it. The R1 fact (the original scanner had a WATCH trigger path) resolves my earlier question D1 in favour of option A, WATCH→TRIGGERED on price. The data access and trigger-detail questions remain (OD-1, OD-3).
- **P2a (RR 1.2 config plumbing) continues.** One overlap is flagged: the field should be readable per class later. §7 gives a shape that needs no rework.
- **P2b (the stop-floor fix) is the one in-flight design this plan would replace.** R4 replaces the generic floor with per-class stop construction. I recommend shipping P2b only in a minimal form: one t2-scoped field read by both the assembly check and the executor. That form is forward-compatible with per-class profiles. I recommend against building any other stop-floor logic under P2b. See §7, overlap O-3.
- **P3 continues.** It is the safety net. This plan adds one design input: CONTEXT-class signals must not suppress a trade-class signal (OD-12).

### 1.4 What else is wrong with the native stack (R1, "beyond naming")

Each item cites its evidence.

1. **There is no outcome calibration anywhere.** No report has score bands vs win rate [M:DOC search]. AIPriority = 0.35·setup + 0.20·timing + 0.20·regime_fit + 0.15·liquidity − 0.10·risk, plus hand bonuses. Examples of those bonuses: funding_reversal LONG −8, −4 more in trend_down, and −4 more if timing < 65; panic_reversal +6/+2 [C:hunter_v7_router.go:395-417, hunter_v7_execution.go:604-664].
2. **Threshold sprawl.**
   - There are 48 inline AIPriority checks with 13 distinct values [M:DOC:lean-core-redesign:52].
   - There are 1,085 numeric comparison points in the modules, router and execution code [M:DOC:lean-core-redesign:94].
   - Per-setup geometry overrides are literals: squeeze and displacement use MinRR 1.35 and MaxTP 12; scalp uses 1.0 and 3 [C:kernel/engine.go:1961-1979].
3. **The score mixes quality with a regime prior.** SetupScore is multiplied by a regime weight between 0.0 and 1.5 before ranking [C:hunter_v7_router.go:70-175]. So the same setup gets a different "quality" score in different regimes, and scores are not commensurate across families or regimes.
4. **Three tier ledgers disagree.** Runtime tier, prompt-final tier and DB tier diverge in measured rounds:
   - 0731: 13 signals tracked vs 11 in the DB [M:DOC:binance-live-3round-outcome-analysis-20260731.md §5].
   - 0803: runtime showed 3 open candidates, prompt-final showed 1 of 30 [M:DOC:live-3round-5m-optimization-review-20260803.md §2].
5. **One global stop floor came from n=1.** See finding 2.
6. **Recall noise.**
   - trend_breakout_long produced 2,427 outputs in trend_up and 125 opens (5.2%) [M:DOC:route-prompt §3]. Its WATCH audit shows TP0-first 0.0% and SL-first 71.4% (n=119) [M:DOC:live-3round-5m-optimization-review-20260803.md §4].
   - funding_reversal produced 0 opens out of 2,809 [M:DOC:route-prompt §3].
   - Both still occupy candidate slots and conflict resolution.
7. **Native evidence is gross, and the tracker leans pessimistic.** No native report deducts fees [M:DOC search]. When TP and SL fall in the same 1m candle, the tracker scores SL first [M:DOC:route-prompt §6.2]. So native outcomes cannot be compared with the fee-net R walks of the 694.
8. **Prompt capacity is mismatched.** MaxOutput is 30, but at most 8 candidates are expanded in full [C:kernel/engine_prompt.go:880-887]. T2 outputs bypass MinAIPriority, MaxOutput, the correlation filter and ResolveV7Conflicts entirely, because they are appended after the router [C:provider/local/hunter_v7.go:57-67].
9. **Data timing.**
   - The forming bar is included [C:datafetch/http_client.go:300-322].
   - The cycle is a 3-minute wall-clock ticker with no alignment to bar closes [C:trader/auto_trader.go:717].
   - Data age P50 is 30.7–41.9 s [M:DOC:hunter-v7-6round-live-trial-optimization-20260727.md:14].
   - Fetch takes 47.9–98.3 s per round [M:DOC:3round-0730/0731/0801 §2].
   - T2 features are computed on whatever the snapshot holds. M3 recall (694/694) was measured on stored payloads, not on live forming-bar snapshots. Live/replay parity is unmeasured [H, test in U10].
10. **T2 state is volatile.** The T2 event store is in memory and per engine, so it is lost on restart [C:hunter_v7_t2_lifecycle.go:66-81]. `InvalidateActivity` exists but is never called [C:hunter_v7_t2_lifecycle.go:162-179].

---

## 2. R1: scoring and the 82 problem

### 2.1 Facts

| Family (T2) | Score ceiling | Source |
|---|---|---|
| bb_oi_accumulation | 100 | C:hunter_v7_t2_setups.go:1070-1180 |
| secondary (confirmed_breakout, reclaim_breakout, trend_pullback) | 94 | C:…:1186-1297 |
| momentum_continuation (armed flow) | 96 | C:…:1002-1025 |
| momentum_continuation_watch (no armed flow) | 79 | C:…:1002-1025; measured 79.0 [M:MEAS2] |
| ignition | 78 (62 + 8 + 8); measured 77.5 | C:…:1304-1374; M:MEAS2 |
| compression_oi_watch | 76 | C:…:1382-1454; measured ≤76.0 [M:MEAS2] |

The tiering constants are all literals: ARMED 82, WATCH 72, ARMED lifetime 4h, WATCH lifetime 2h, cooldown 60 min [C:hunter_v7_t2_setups.go:96-112, hunter_v7_t2_lifecycle.go:150-152]. The observe-only list has 5 setups and is also a literal [C:hunter_v7_t2_setups.go:87-93].

Native has no 82 score threshold. Its two 82s are RSI checks [C:hunter_v7_mod_displacement.go:126, hunter_v7_mod_mms.go:220]. The native visibility gate is MinAIPriority 55 [C:provider/local/hunter_v7_types.go:567-577].

### 2.2 R1(a): calibration analysis

**What exists now:**

| Stack | Score vs outcome by band | Status |
|---|---|---|
| T2 | Owner brief states the score is anti-predictive on the 694. I do not hold the band table. | Band table is PENDING U0-A |
| Native | None exists in any repo report [M:DOC search] | PENDING U0-B. The available sample is the 331 native final signals on the 694 snapshots [M:MEAS3], walked on the same klines. That sample is conditional on T2 trigger moments, so it is a biased sample of native fires. Universe-wide native calibration needs a native replay corpus, which does not exist in fee-net form (OD-17). |

The RR-band evidence below is already measured [M:MEAS1]. It feeds R4 directly.

| RR band | n | TP1-first | Mean fee-net R | Source |
|---|---|---|---|---|
| ≥ 1.5 (original admits) | 9 | — | ≈ +0.084 | [D] (19 × 0.174 − 10 × 0.255) / 9 |
| [1.2, 1.5) | 10 | 40.0% | +0.255 | M:MEAS1 |
| [1.2, 1.5), ignition | 7 | — | +0.651 | M:MEAS1 |
| [1.2, 1.5), non-ignition | 3 | — | ≈ −0.669 | [D] (10 × 0.255 − 7 × 0.651) / 3 |
| [1.0, 1.2) | 13 | 23.1% | −0.357 | M:MEAS1 |
| floor 1.2 admitted set | 19 | — | +0.174 | M:MEAS1 |
| floor 1.0 admitted set | 32 | — | −0.042 | M:MEAS1. Consistency check: 19 × 0.174 + 13 × (−0.357) = −1.335; −1.335 / 32 = −0.042 ✓ [D] |

Sampling noise, assuming a per-trade SD of about 1R [H]. U0 must replace this with bootstrap CIs:
- the floor-1.2 mean is about 0.76 SE above zero;
- the gap between the [1.2,1.5) and [1.0,1.2) bands is about 1.5 SE;
- the ignition cell is about 1.7 SE above zero.

No cell is distinguishable from zero at conventional confidence.

**Pre-registered U0-A specification (T2):** for each family, and for the pooled 694, report:
- score bands of width 5 points, falling back to deciles if a band has n < 15;
- for each band: n, TP1-first rate, mean fee-net R with the owner-approved R convention (OD-2), a 90% bootstrap CI, and the Spearman ρ of score vs R.

Repeat the same table within each fold of the owner-approved fold scheme (OD-2) to check stability.

**Pre-registered U0-B specification (native):** the same table using AIPriority and SetupScore, for the 331 native final signals [M:MEAS3]. Each signal is walked from its snapshot time on the same 1m klines, with geometry taken from the native signal's own invalidation and targets.

### 2.3 R1(b): verdict on 82

**Verdict: 82 is not defensible as an admission gate in the port, at any level.** The evidence:

- **Construction.** Three families can never reach 82 (§2.1). In the original scanner they were not disenfranchised, because WATCH could promote on trigger (owner brief). The port removed that path [C:hunter_v7_t2_lifecycle.go:20-23].
- **Missed-opportunity count.** 516 of 694 are score-capped to WATCH and 63 more are forced to WATCH by the observe-only list [M:MEAS2]. That is 579 of 694 (83.4%) that the port can never review. Among structurally admissible events, at least 14 of 19 at RR ≥ 1.2 are blocked (73.7%) [D, §1.1]. That set includes the best measured cell, ignition [1.2,1.5) at +0.651R (n=7) [M:MEAS1].
- **Axis.** The score is measured anti-predictive (owner brief). A higher threshold on an anti-predictive axis selects worse events, and a lower one selects more of the same. So lowering 82 to some other number is a textbook move with no evidence behind it, and this plan does not propose one.

**Recommendation:**

1. Now, via P1: give every T2 family a path through the WATCH→TRIGGERED lifecycle, so 82 stops being fatal. Keep 82 only as a stored label (`armed`) for comparison with the original scanner.
2. Retire 82 as an admission input once U7's falsification test has passed. At that point admission is decided by class lifecycle + cell status (§5), and ranking by calibrated E[R] (§2.4).

### 2.4 R1(c): scoring redesign

**Commensurate scale.** Every signal gets an estimated expected fee-net R:

  E[R] = p·R_win − (1 − p)·1 − c

- p = P(TP1 before SL), estimated per (class, family, RR band).
- R_win is the planned fee-net R at TP1.
- c is the fee cost in R, using the owner's convention (OD-2).

This puts every family in the same units, R, which is what both the position slots and the LLM are choosing among.

**Estimator.** Given the sample sizes, start simple:
- an empirical-Bayes shrunk mean of realized fee-net R per (family × RR band) cell, shrunk toward the class mean, which in turn is shrunk toward the global mean;
- weights n/(n + k), with k chosen by leave-one-fold-out [H].

Raw scores (T2 score, AIPriority) are excluded as inputs unless U0 shows a monotone, fold-stable relation, measured as Spearman ρ > 0 in every fold. The T2 score is currently anti-predictive (owner brief), so it is excluded by this rule.

**What replaces the single global threshold:**

1. **Reachability:** the class lifecycle (§3) decides whether a signal is reviewable now, for example TRIGGERED for the TRIGGER class.
2. **Admission:** the cell status (§5.3). EARNED cells are admitted. PROBATION cells are admitted only by explicit owner policy, such as the RR 1.2 decision, and are tagged. BLOCKED cells are shadow-only.
3. **Ranking:** among admitted signals, rank by the lower bound of E[R]. This matters because the owner runs at most 2 positions.

**Forward maintenance:**
- outcome tagging per §5.4;
- re-estimation on a fixed cadence: every 25 new terminal outcomes in a cell, or monthly, whichever comes first (recommendation, OD-8);
- each fit is versioned (`calibration_version`) and stored with every decision that used it;
- drift monitor: a reliability table of predicted vs realized mean R by quintile on the most recent window. Two consecutive windows outside the bootstrap CI raise an alert and freeze the cell at PROBATION.

**Falsification (pre-registered; settles [H] for this design).** On the owner-approved time-split folds (OD-2), at equal admission count:

| ID | Test |
|---|---|
| F-1 | E[R]-ranked admission must beat score-ranked admission on held-out mean fee-net R in at least 2 of 3 folds. |
| F-2 | Calibration: predicted vs realized mean R by quintile, with realized inside the 90% bootstrap CI of predicted in at least 4 of 5 quintiles. |
| F-3 | E[R]-ranked must beat random admission of the same size (1,000 draws, one-sided p < 0.10). |

If F-1 or F-3 fails, the estimator stays shadow-only. Admission then remains class lifecycle + cell status, with arrival-order ranking. The plan does not fall back to a score threshold.

---

## 3. R2: signal taxonomy and per-class paths

### 3.1 Class definitions

A class is a group of families with a shared lag profile and shared geometry logic.

| Class | Lag profile | Geometry model | Entry mechanism | Evidence for the class split |
|---|---|---|---|---|
| IMPULSE | Run-away after detection. Delay is costly. | Chase-ATR, a stop scaled to volatility from the entry; hybrid where structure is near [H] | Market now, inside a drift contract | Momentum 0/218 under structural geometry [M:M2v2]. 1-bar delay costs 18.6R [M:LAT]. Run-away 0/32/85% by consumed band [M:LAT]. |
| TRIGGER | Pending at a recorded level. Time to decide before the cross. | Structural: stop beyond the level, targets at structure (spec v2) | Fires when price crosses the recorded trigger. The backend executes; the decision is made beforehand. | Ignition 7/396 admitted under structural geometry vs momentum 0/218 [M:M2v2]. SWING 2/80 [M:M2v2]. Trigger-bar entry beats decision-bar entry by 18.6R [M:LAT]. |
| REVERSAL | Counter-move. Needs confirmation (reclaim or reject). | Structural, with the stop beyond the extreme | After confirmation, inside a drift contract | Native modules use wait_reclaim / wait_reject entry modes [C:hunter_v7_types.go:98, DOC:fullchain-architecture-20260613 §6.1]. No lag measurement exists yet [H, U0-C]. |
| CONTEXT | n/a | none | Never a candidate. Annotates other signals. | funding_reversal 0/2,809 opens [M:DOC:route-prompt §3]. The pre_* watch types are watch-only by design [C:hunter_v7_signal_state.go:575-582]. |

### 3.2 Class assignment for every family

The "Evidence" column cites what decides the assignment. Rows marked [H] are provisional until U0-C, the per-family delay-cost and run-away replay for both stacks, confirms them. U0-C must also be checked against SSA §H.3 (gap noted in §0.2).

**T2 families** (setup types per C:hunter_v7_t2_setups.go):

| Family / setup | Class | Evidence |
|---|---|---|
| momentum_continuation_* (incl. _watch) | IMPULSE | 0/218 structural [M:M2v2]. 46 RR rejects touched TP1 first 78.3% of the time yet averaged −0.031R, a near-TP1 / far-stop asymmetry [M:MEAS1]. Trigger = current price [C:…:917-1067]. |
| ignition_* | TRIGGER | 7/396 structural admits, and the best cell is +0.651R at [1.2,1.5) [M:M2v2, MEAS1]. The trigger is K15 resistance/support [C:…:1304-1374]. |
| bb_oi_accumulation_* | TRIGGER | The trigger is K15 resistance/support [C]. Part of SWING 2/80 [M:M2v2]. Per-family split PENDING U0. |
| confirmed_breakout_* | TRIGGER | Trigger at a level [C]. Both directions are on the observe-only list [C:…:87-93]. |
| reclaim_breakout_* | TRIGGER | Trigger at a level [C]. The long side is observe-only. |
| trend_pullback_* | REVERSAL [H] | Entry on a pullback within a trend. The long side is observe-only. Lag profile PENDING U0-C. |
| compression_oi_watch_* | TRIGGER | Price is 0.1–1.5% short of the trigger [C:…:1382-1454]. |

**Native families** (setup types per C:hunter_v7_types.go:36-67, router C:hunter_v7_router.go:23-51):

| Family / setup | Class | Evidence |
|---|---|---|
| leader_momentum_long | IMPULSE | XPLUS moved +13.23% and then +17.04% while the chase gates held it back [M:DOC:fullchain-architecture-20260613 §12.1]. Entry mode momentum_with_trailing_stop [DOC §6.1]. Lag [H]. |
| displacement_momentum_long | IMPULSE | Its RR geometry fix turned 7 RR blocks into 2 EXECUTABLE and 0 REJECTED [M:DOC:fullchain-architecture §12.2]. Replay n=5, TP0-first 80%, gross mean +3.00% [M:DOC:latest-binance-replay-20260802]. Lag [H]. |
| alt_ladder_momentum_long | IMPULSE | Replay n=15, TP0-first 60% / SL-first 40%, MAE −6.53% [M:DOC:latest-replay]. 30-day DB mean −1.137% (compression, n=5) and −0.569% (rotation, n=7) [M:DOC:route-prompt §4]. |
| breakdown_momentum_short | IMPULSE | 30-day DB trend_down n=9, 88.9% protected/win, +0.156% [M:DOC:route-prompt §4]. Lag [H]. |
| alt_ladder_breakdown_short | IMPULSE [H] | WATCH audit n=13, TP0-first 69.2%, SL-first 0% [M:DOC:live-3round-5m-0803 §4]. It uses close-through confirmation, so it may be TRIGGER. U0-C decides. |
| short_squeeze_long, long_squeeze_short | IMPULSE [H] | fast_confirm entry mode [DOC:fullchain-architecture §6.1]. No outcome data. |
| range_expansion_event | IMPULSE [H] | Conflicting evidence: 30-day DB rotation n=13 at +1.645% [M:DOC:route-prompt §4] vs replay n=14 at −11.87% with MAE −18.59% [M:DOC:latest-replay]. Tail risk. U0-C decides. |
| trend_breakout_long | TRIGGER | Close-through requirement. WATCH audit n=119, TP0-first 0.0%, SL-first 71.4% [M:DOC:0803 §4]. |
| accumulation_breakout_long | TRIGGER | wait_breakout entry mode [DOC §6.1]. No measured outcomes [M:DOC search]. |
| volatility_squeeze_breakout | TRIGGER | Squeeze breakout. It has a single REJECTED row in the docs [M:DOC:3round-0802]. |
| intraday_scalp_long | TRIGGER [H] | Held back by `5m_or_15m_close_above_trigger` twice [M:DOC:6round-0727]. No outcomes. |
| mms_* (bottom_wake, trend_ride, squeeze_engine) | [H] UNASSIGNED | Fixed timing scores of 68 and 72 [C:hunter_v7_mod_mms.go:176,242]. mms_trend_ride outcomes n=3–6, weak [M:DOC:3round-0731, 9round]. U0-C decides. |
| panic_reversal_long, pullback_reversal_long | REVERSAL | wait_reclaim / wait_confirm entry modes [C, DOC §6.1]. No outcomes. |
| distribution_short | REVERSAL | wait_reject entry mode. No measured numbers [M:DOC search]. It is the #2 co-fire leader with 21 [M:MEAS3]. |
| range_reversion | REVERSAL | range_edge_only entry mode [DOC §6.1]. |
| whale_flow_reversal | REVERSAL [H] | Replay n=6, TP0-first 100% [M:DOC:latest-replay]. 6 of 11 EXEC cases were judged "not fillable live" [M:DOC:6round-0727]. |
| relative_weakness_short | REVERSAL [H] | No outcomes. |
| funding_reversal | CONTEXT | 0 of 2,809 opens [M:DOC:route-prompt §3]. WATCH audit n=53, gross mean −0.01% [M:DOC:0803 §4]. Replay n=22, gross mean −0.29% [M:DOC:latest-replay]. The #1 co-fire leader with 23 [M:MEAS3]. |
| pre_breakout / pre_squeeze / pre_distribution / accumulation_watch | CONTEXT | Watch-only by design [C:hunter_v7_signal_state.go:575-582]. |

### 3.3 Per-class path

Every limit in this table is config. Values marked [H] are set from the U0-C measurement, not chosen here.

| | IMPULSE | TRIGGER | REVERSAL | CONTEXT |
|---|---|---|---|---|
| Screening path | Detector → freshness check → chase-ATR geometry → cell gate → conflict pass → candidate | Detector → T2/V7 event store (WATCH / ARMED with the 2h / 4h lifetimes) → TRIGGERED on price cross → structural geometry (spec v2) → cell gate → conflict pass → candidate | Detector → confirmation (native state manager, cycle-based) → structural geometry → cell gate → conflict pass → candidate | Detector → annotation on candidates for the same symbol |
| Latency budget | Detection to order inside one bar of the signal timeframe. Budget value [H, U0-C]. The 18.6R one-bar cost [M:LAT] sets the order of magnitude. | Zero bars after the trigger cross. Achieving it needs a resting conditional entry or a trigger check at bar close (U9). | Budget [H, U0-C] | n/a |
| Freshness contract (via the drift contract, D3) | Maximum adverse drift between signal price and fill, as a fraction of the planned stop distance. Value [H], set from the consumed-band run-away curve (0/32/85% [M:LAT]) re-expressed as drift. | Drift measured from the trigger price, not the detection price | Drift measured from the confirmation price | n/a |
| Geometry | Chase-ATR / hybrid [H, U0-D] | Structural, M2v2 spec | Structural | none |
| LLM role | Veto-only, with a timeout: no answer within budget means no trade. [H]: the delay cost suggests a full LLM round trip is too slow. U0-C measures how many bars an LLM round trip spends. | Full decision at ARMED/WATCH, made before the trigger. The backend executes on the cross without a second LLM call. | Full decision | Not involved as a candidate. Visible as context. |

---

## 4. R3: the four duplicate twins

### 4.1 Pair facts (from code)

| Pair | T2 side | Native side | Material difference |
|---|---|---|---|
| A. bb_oi_accumulation vs accumulation_breakout_long | Long and short. Score ≤ 100. Trigger at K15 level [C:t2_setups.go:1070-1180]. | Long only. BBWidthPercentile < 25, OI 4h +8–30% [DOC:fullchain-architecture §6.1]. BreakoutLong tier spec [C:hunter_v7_tier_rules.go:246-247]. | T2 covers the short side. No native outcomes [M:DOC search]. |
| B. momentum_continuation vs leader_momentum_long | Both directions. abs(chg24h) 15–120. Trigger = price [C:…:917-1067]. | Long only. 24h 12–60%, 4h > 6% [DOC §6.1]. Open-rate 5.3% (rotation) and 6.9% (trend_up) [M:DOC:route-prompt §3]. No outcome rows. | T2 covers the short side. Both are IMPULSE. |
| C. compression_oi_watch vs volatility_squeeze_breakout | bbwr-based. Price 0.1–1.5% short of the trigger. Hard WATCH; the long side is observe-only [C:…:1382-1454, 87-93]. | Different compression measure (module C:hunter_v7_mod_squeeze_breakout.go). DefaultOnly tier spec. Geometry MinRR 1.35, MaxTP 12 [C:engine.go:1961-1979]. | The measures are not the same, so this is not a true duplicate. |
| D. confirmed_breakout vs trend_breakout_long | Both directions are observe-only [C:…:87-93]. | WATCH audit n=119, TP0-first 0.0%, SL-first 71.4% [M:DOC:0803 §4] | Both sides have been demoted by their own maintainers or by evidence. |
| Name collision: "reclaim" | `reclaim_breakout_*` is a setup type: a breakout after reclaiming a level | No native setup type. "reclaim" appears in panic-reversal reason codes (`strong_reclaim` and others), the `wait_reclaim` entry mode and `entry_reclaim_wait` [C:hunter_v7_mod_panic_reversal.go:85-94, hunter_v7_types.go:98,130] | Same word, different concept. This is a naming fix (Task C, OD-18), not a merge. |

### 4.2 Pre-registered comparison protocol (U0-E)

**Overlap set.** For each pair, take the 694 snapshots with symbol-level co-presence in MEAS3: T2 fired, and the native twin did or did not fire on the same snapshot. Walk the native signal on the same klines with the same R convention. Use each signal's own geometry, and also a common geometry: the M2v2 structural plan for that snapshot.

**Arms:**
1. both fire, same direction;
2. both fire, opposite direction;
3. only T2 fires;
4. only native fires. This arm is limited: MEAS3 only covers snapshots where T2 fired. Universe-wide native-only fires need a native corpus (OD-17).

**Metric:** directional win rate (TP1 before SL), mean fee-net R, and 90% bootstrap CI, per arm.

**Decision rule:**

| Verdict | Condition |
|---|---|
| keep A / keep B | n ≥ 30 per compared arm, and the winner's mean R exceeds the loser's with a 90% CI of the difference above 0 |
| fuse | conditions where agreement (arm 1) beats both single-fire arms by the same test |
| keep both with division of labor | otherwise, when the sides cover different directions or regimes |
| loser disposition | shadow-only: still computed, tagged and walked. Retire after two further consecutive evaluation windows with the same verdict. |

### 4.3 Provisional verdicts

These are [H] until U0-E has run.

| Pair | Provisional verdict | Reason |
|---|---|---|
| A | Keep both, division of labor: T2 owns the short side; the long side is decided by U0-E | Native has no short side and no outcome evidence [M:DOC search] |
| B | One IMPULSE class path fed by both detectors. Per symbol/direction, keep the signal with the higher E[R] lower bound. T2 owns the short side. | Same class. Native has no outcome rows. |
| C | Keep both as distinct TRIGGER detectors | The compression measures differ. U0-E checks agreement. |
| D | Both shadow-only, unless U0-E finds a cell with a positive lower bound | 0.0% TP0-first at n=119 on the native side [M:DOC:0803], and the T2 side is demoted on both directions in the port [C] |

### 4.4 Conflicts that disappear by design

- **Twin merging:** at most 18 of 62 final-level co-fires (29%) can be twin-related [D, §1.1]. The exact count is PENDING U0-E.
- **Reclassifying funding_reversal as CONTEXT:** this removes up to 23 of 62 from the LLM-facing candidate conflicts [M:MEAS3]. Under OD-12, the CONTEXT signal then annotates instead of competing.
- **What remains:** distribution_short (REVERSAL) vs T2 continuation, 21 cases [M:MEAS3]. These are genuine thesis conflicts, and P3's backend rule resolves them.

---

## 5. R4: classified risk, RR floors and outcome tagging

### 5.1 Per-class risk profiles (strategy config)

Proposed schema, under `RiskControl.ClassProfiles[class]` and versioned as `profile_version`:

| Field | IMPULSE | TRIGGER | REVERSAL |
|---|---|---|---|
| stop_construction | `atr_multiple` / `hybrid`, multiple [H, U0-D] | `structural`: beyond the trigger level, spec v2 | `structural`: beyond the extreme |
| min_stop_pct (noise floor) | [H, U0-D] | [H, U0-D] | native default 2.0, unchanged until U0-D covers native |
| min_rr | t2: 1.2 (owner) / native: unchanged | t2: 1.2 (owner) / native: unchanged | native: unchanged |
| target_model | R-multiple. T2 currently uses TP1 1.8R and TP2 3.0R [C:hunter_v7_t2_setups.go:96-112] | structural targets within the TP cap, spec v2 | structural |
| max_tp_pct | current effective value. Today 4.0 at defaults [D: (2.0 + 0.5) × 1.5 + 0.25; C:engine.go:623-650] | same | same |
| drift contract | §3.3 | §3.3 | §3.3 |

Initial values reproduce current behaviour, except for the owner's RR 1.2 for t2. U6 has a golden-replay acceptance check that enforces this.

**Stop noise floor [H].** The current 2% is backed by n=1 (§1.1). The pre-registered test, U0-D, runs on the 694 walks:
- compute the early stop-out rate (stopped within the first 3 bars of the signal timeframe) by stop-distance / ATR band;
- the class floor is the smallest band whose early stop-out rate is not higher than that of the next wider band at 90% confidence.

No floor value is proposed until this test has run.

### 5.2 Class × band outcome table (calibration source)

Cells are class × RR band. The bands are [1.0,1.2), [1.2,1.5), [1.5,2.0) and ≥ 2.0. Each cell reports n, TP1-first rate, mean fee-net R and the 90% bootstrap CI.

What is filled today [M:MEAS1, D]:

| Cell | n | Mean R | Status under §5.3 |
|---|---|---|---|
| TRIGGER (ignition) × [1.2,1.5) | 7 | +0.651 | PROBATION (n < N_min) |
| all × [1.2,1.5) | 10 | +0.255 | PROBATION |
| all × [1.0,1.2) | 13 | −0.357 | PROBATION. It is the worst band, but n < N_min. |
| all × ≥ 1.5 | 9 | ≈ +0.084 [D] | PROBATION |
| IMPULSE (momentum RR rejects) × all bands | 46 | −0.031 | PROBATION. n ≥ 30, but this is a pooled reject sample and not a band cell. U0 splits it. |

The full table is PENDING U0-A.

### 5.3 How a cell earns a looser or tighter gate (pre-registered)

- **EARNED:** n ≥ N_min and the 90% bootstrap lower bound of mean fee-net R is > 0. The cell is admitted at its band.
- **BLOCKED:** n ≥ N_min and the 90% upper bound is < 0. The cell is shadow-only.
- **PROBATION:** everything else. Admitted only under an explicit owner policy, such as RR 1.2 for t2. Tagged `cell_status=probation`. Sizing per OD-15.
- **N_min:** I recommend 30 (OD-8). Re-evaluate on the cadence in §2.4.
- **Hysteresis:** a cell changes status only after two consecutive evaluations agree.

Today no cell is EARNED or BLOCKED; the largest band cell has n=13. **So the RR 1.2 setting is an owner policy under probation.** I recommend a pre-registered review trigger (OD-9): when the t2 [1.2,1.5) cell reaches n ≥ 30, the floor reverts to 1.5 if the 90% upper bound is below 0.

### 5.4 Tagging every admitted signal

Extend the existing `HunterV7SignalRecord`, which already has `setup_type`, scores and `track_*` outcome columns [C:store/hunter_v7_signal.go:18-70], rather than adding a new table. New columns:

- `signal_class`, `family`, `stack` (native / t2)
- `rr_band`, `stop_atr_band`, `cell_status`
- `profile_version`, `calibration_version`, `doctrine_version`
- `lifecycle_state_at_decision` (WATCH / ARMED / TRIGGERED / …)
- `trigger_price`, `drift_at_entry_pct`, `decision_path` (llm_full / llm_veto / none)
- `fee_net_r` (owner R convention), `outcome_source` (prompt_open_review / watch_audit / shadow)

The `watch_audit` bucket and the `tracked_source` split were already proposed in repo docs [DOC:live-3round-5m-0803 §6 P0-A, P2]. They keep shadow and WATCH outcomes out of the real win-rate denominator.

### 5.5 Position sizing per class (PROPOSED; owner decision OD-15)

The current owner setting is a flat 8% risk with at most 2 positions. One arithmetic fact matters here: two full stop-outs on the same day at 8% each lose 16%. That exceeds the F08 daily-loss default of 15% [D], so the breaker would trip after two losses.

Options:
- flat, as today;
- by cell status: EARNED 1.0×, PROBATION 0.5×, BLOCKED 0;
- by class.

Recommendation: by cell status. It is the only option tied to evidence.

---

## 6. R5: full chain and engine adaptation

### 6.1 Target architecture

```
[Data layer]
  REST/WS klines, OI, depth, funding
  - features on CLOSED bars only; forming bar used for price only        (U10)
  - every snapshot stamped: data_age_ms, bar_close_ts
  - cycle aligned to the 5m/15m close + settle delay                      (U10)
        |
        v
[Detectors]  native modules (22) ... T2 families (14 setup types)
        |  raw signals + family id
        v
[Class registry]  family -> class, profile_version (config)               (U6)
        |
        +--> CONTEXT ------------------------------------+ annotations
        |                                                |
        +--> IMPULSE: freshness check -> chase-ATR geom  |
        +--> TRIGGER: event store WATCH/ARMED -> TRIGGERED on cross (P1/U1) -> structural geom
        +--> REVERSAL: confirmation (native state mgr) -> structural geom
        v                                                |
[Cell gate]  class x band status EARNED/PROBATION/BLOCKED (U5/U6)
        v                                                |
[E[R] estimator]  calibrated, ranks; shadow until F-1..F-3 pass (U7)
        v                                                |
[Conflict pass]  one direction per symbol per cycle; CONTEXT non-voting (P3/U3)
        v                                                v
[Assembly / tiers]  per-class tier rules (t2: REVIEWABLE cap, OD-11)      (U4)
        v
[LLM]  per class: full / veto-with-timeout / none; doctrine_version in payload (U11)
        v
[Executor]  per-class drift contract; TRIGGER resting conditional entry (U9)
            deterministic IDs (F04), stop-failure close (F03),
            owned side-scoped cancel (F26), daily-loss breaker (F08)
        v
[Outcome tracker]  fee-net R, tags (5.4), watch_audit bucket              (U5)
        v
[Calibration store]  periodic re-estimation, drift monitor -> cell gate + E[R]  (U7)
        v
[Observability]  per class / cell / regime dashboards                    (U12)
```

### 6.2 Chain changes beyond F01–F26

| Area | Change | Evidence | Unit |
|---|---|---|---|
| Data: forming bar | Compute features on closed bars and use the forming bar only for price | Forming bar included [C:datafetch/http_client.go:300-322]. Live/replay parity unmeasured [H] | U10 |
| Data: cycle alignment | Align the scan to bar close + settle delay instead of a 3-minute wall-clock ticker | [C:trader/auto_trader.go:717]. 1-bar delay costs 18.6R [M:LAT] | U10 |
| Data: snapshot age | Gate by class: IMPULSE rejects a snapshot older than its latency budget | Data age P50 30.7–41.9 s [M:DOC:6round-0727:14]. Fetch 47.9–98.3 s [M:DOC:3round reports] | U10 |
| Data: dirty symbols | Keep isUSDTPerp. Report the 16 exclusions per run. | 678/694 admitted [M:MEAS3] | U0 |
| Data: T2 inputs | Record GlobalLSR (fixed at 1.0) and market cap (0) as known gaps in tags | [C:hunter_v7_t2_adapter.go:9-49] | U5 |
| Assembly | Per-class tier rules. T2 readiness context, or the explicit t2 rule (D3 / OD-11). Remove the `needs_confirmation` fallthrough for T2. | 172 needs_confirmation, 522 rr_infeasible, 0/694 surfaced [M:MEAS2]. Fallthrough at [C:kernel/engine.go:782] | U4 |
| Router coverage | T2 enters ResolveV7Conflicts, or a final pass after it, plus MaxOutput accounting | T2 appended after the router [C:hunter_v7.go:57-67] | U3 |
| Lifecycle persistence | Persist the T2 event store, or rebuild it on start. Wire `InvalidateActivity`. | In memory, never invalidated [C:hunter_v7_t2_lifecycle.go:66-81,162-179] | U1 |
| LLM payload | Add `signal_class`, `lifecycle_state`, `cell_status` and `doctrine_version`. Per-class role text. Thresholds move out of prose into fields. | Doctrine has no version string [C:kernel/hunter_v7_prompt_doctrine.go:14]. Threshold prose in §3 [DOC:route-prompt §5] | U11 + Task C |
| Execution: drift | Per-class drift contract in config. Replace the literal 0.45. | `hunterV7MicroRefreshMaxDriftPct` 0.45 literal [C:trader/auto_trader_risk.go:63] (F13) | U9 |
| Execution: trigger entry | Resting conditional entry at the trigger for the TRIGGER class, or a trigger check at bar close. Needs the F04 idempotency. | Entry today is market only [C:trader/binance/futures_orders.go:43] | U9 |
| Yield target | G3 (≥ 170) becomes a regime-conditional target (SSA §I), set from per-regime fire rates measured in U0 | G3 unreachable: 19 at 1.2, 90 even at 0.5 [M:MEAS1] | U13 |
| Observability | Dashboards per class × cell × regime: n, fee-net R, CI, cell status, calibration drift | §5.4 tags | U12 |

### 6.3 Migration sequence

Acceptance is offline replay plus code review, because the owner cancelled the forward shadow phase. Each unit ships as one branch and one PR. Config replaces literals. `geometry_mode` default stays `legacy`. No pilot or live enablement.

| Unit | Content | Acceptance criteria | Measurement gate before the next dependent unit |
|---|---|---|---|
| U0 | Evidence harness + data access. Tables U0-A (T2 calibration), U0-B (native calibration), U0-C (per-family delay cost and run-away, both stacks), U0-D (stop/ATR early stop-out), U0-E (twins arms) | Reproduces bit-identically: M2v2 9/694; MEAS1 19/32/90 and median 0.336; MEAS2 522/172 and 516/63; MEAS3 62/114 | Owner approves the eval conventions (OD-2) before U0 runs |
| U1 = P1 | T2 TRIGGERED stage in the isolated store. Tiering constants and the observe-only list move into config at current values. | Re-run of the 694: recall 694/694; WATCH/ARMED/TRIGGERED counts per family; ignition demonstrably reaches TRIGGERED (needs forward klines, OD-1) | none (independent) |
| U2 = P2a | RR floor 1.2 for t2 via config: validator, M2v2 admission and assembly read one value | M2v2 re-run admits 19 at 1.2 [M:MEAS1], or the difference is explained. No 1.5 literal left on the t2 path. Native unchanged. | none |
| U3 = P3 | One direction per symbol per cycle, final pass, policy in config (OD-12) | 0 unresolved opposite pairs on the 694. Win/loss breakdown by family pair. | none |
| U4 = P2b/c (minimal) | One t2-scoped stop-floor field read by both assembly and executor (value per OD-10). T2 tier rule (OD-11). | Re-run: number of T2 signals reaching REVIEWABLE, by remaining reject reason | U0-D before any per-class floor value |
| U5 | Tag schema (§5.4) + tracker fee-net R + watch_audit bucket | Every replayed candidate carries all tags. Tracker fee-net R matches the U0 walk on the 694. | — |
| U6 | Class registry + per-class profiles in config at current values | Golden replay: identical candidate set and tiers before and after | U0-C confirms class assignments (§3.2 [H] rows) |
| U7 | E[R] estimator in shadow (logged, not gating) | F-1..F-3 computed on folds and reported | Switching it to ranking requires F-1 and F-3 to pass |
| U8 | Apply the twins verdicts (retire / shadow) | U0-E verdicts applied. Conflict count re-measured (§4.4). | U0-E |
| U9 | Per-class drift contract. TRIGGER-class conditional entry. | Replay delay cost per class (from U0-C) does not regress. Idempotency tests (F04 pattern) cover conditional orders. | U0-C |
| U10 | Closed-bar features, cycle alignment, snapshot-age gating | Live/replay parity test: feature diff on recorded live snapshots vs a replay of the same bars [H] | Owner decision OD-21 (affects native cadence) |
| U11 | Payload/doctrine versioning and per-class LLM role; Task C naming pass 1 in the same vocabulary | Deletion test (Task C) plus payload golden | OD-6, OD-7, OD-18 |
| U12 | Dashboards | Per-class tables match U5 records | U5 |
| U13 | Regime-conditional yield target | Target set from U0 per-regime fire rates | U0 |
| F08 | Daily-loss breaker (Task A remainder) | As specified earlier: armed only in pilot/live, records in shadow | Before any pilot |

---

## 7. Overlap map

| In-flight or queued item | Relation | Detail |
|---|---|---|
| F08 (daily-loss breaker) | Independent | Required before any pilot. Interacts with sizing: 8% × 2 = 16% > 15% (OD-15). |
| P1 (lifecycle) | Builds-on | It becomes the TRIGGER-class engine. Its output should carry `lifecycle_state` (§5.4); a small addition in P1. |
| P2a (RR 1.2) | Builds-on | **O-1:** implement it as a t2-scoped value read through one accessor, so U6 can later switch the accessor to a per-class profile lookup without touching callers. |
| P2b (stop floor) | **Superseded in design (O-3)** | R4 replaces the generic floor with per-class stop construction. Recommendation: ship only one t2-scoped `min_stop_loss_price_move_pct` field read by both layers, through the same accessor pattern as O-1. Build no ATR or class logic under P2b. |
| P2c (readiness / needs_confirmation) | Builds-on | The t2 tier rule (REVIEWABLE cap, `t2_awaiting_trigger`) carries straight into the per-class tier rules. **O-4:** key the rule on class, not on the setup-string prefix. |
| P3 (one direction) | Builds-on | **O-5:** add a CONTEXT non-voting option to the policy now (OD-12). The twins merge removes at most 18 of 62 conflicts, so P3 stays necessary. |
| Task B (Nofx ports: Compile semantics, throttle constants into config) | Independent | No dependency. Throttles are execution-side. |
| Task C (naming pass 1) | **Builds-on; resequence (O-6)** | Naming should use the class/family vocabulary and fix the reclaim collision. Recommendation: run Task C after OD-6, so nothing is renamed twice. |
| Task D (M2v2 spec, merged) | Builds-on | It becomes the TRIGGER-class geometry. The pending M2 A/B decision becomes per-class (OD-16). |
| F13 (hidden literals) | Builds-on | U1 (tiering constants), U6 (geometry overrides) and U9 (drift 0.45) move F13 literals into config. |
| Earlier open questions D1–D4, B1, B2 | Folded in | D1 is resolved by the R1 fact (option A). B1 becomes OD-1, B2 becomes OD-3, D2 becomes OD-10, D3 becomes OD-11, D4 becomes OD-12. |

---

## 8. Owner decision list

Each entry gives the options, my recommendation, and the evidence behind it.

| ID | Decision | Options | Recommendation | Evidence |
|---|---|---|---|---|
| OD-1 | Data access for the 694 snapshots and forward klines | (a) put them in the sandbox (private repo or release asset); (b) Muse runs the harness I ship | (a) | They are not in the sandbox (§0.2). U0, U1 and U4 acceptance depend on them. |
| OD-2 | Eval conventions (still pending) | fold tertiles; fee-net R full-exit convention; null for zero-denominator ratios; default output filename | Approve all four as previously proposed | They are needed for every U0 table and for F-1..F-3 |
| OD-3 | T2 trigger details | which price (last trade vs bar close); what a cross is (touch vs close beyond); does TRIGGERED end the event; must the stop be unbroken | Paste spec §5.2–5.3. If unavailable: 15m close beyond the level, event ends on trigger, stop must be unbroken. That is the stricter choice, and none of these values is evidence-backed. | Spec not in the repo (§0.2) |
| OD-4 | Role of 82 | (a) trigger path now, 82 kept as a label, retired as admission after U7; (b) keep 82 as admission plus the trigger path; (c) per-family thresholds | (a) | 579/694 unreviewable; at least 14/19 admissible blocked; score anti-predictive (§2.3) |
| OD-5 | Observe-only list (5 setups, 63 events) | keep the values, move them to config; or exempt | Keep, move to config | It was deliberate in the original scanner [C:t2_setups.go:87-93]. No outcome evidence yet for those 63. |
| OD-6 | Class taxonomy and assignments | approve §3.1–3.2, with the [H] rows provisional until U0-C | Approve | §3.2 evidence column |
| OD-7 | LLM role per class | IMPULSE veto-with-timeout vs full decision; TRIGGER decide-before-trigger | Veto-with-timeout for IMPULSE, decide-before-trigger for TRIGGER, subject to U0-C | 18.6R one-bar cost [M:LAT] |
| OD-8 | Cell rule parameters | N_min 20 / 30 / 50; 90% vs 95% CI; re-estimation cadence | N_min 30, 90% bootstrap, every 25 outcomes or monthly | Largest band cell today has n=13 [M:MEAS1] |
| OD-9 | RR 1.2 review trigger | none; revert to 1.5 if the [1.2,1.5) cell upper bound < 0 at n ≥ 30 | Pre-register the revert rule | Cell gaps are about 1.5 SE (§2.2) |
| OD-10 | t2 stop-floor value and scope (minimal P2b) | value; assembly only vs both layers | Both layers, one field. The value is yours until U0-D; if you want a starting value, 0.5% sits below the ~1.0% cluster | 95.6% < 2%, 522 rejects [M:MEAS2]. The 2% floor comes from n=1 [M:DOC:funding-reversal-risk-fix] |
| OD-11 | Highest tier for T2 | REVIEWABLE cap vs EXECUTABLE allowed | REVIEWABLE cap until cells are EARNED | No EARNED cell (§5.3) |
| OD-12 | Conflict policy (P3) | conflict_watch (inherit native); prefer_native; prefer_t2; higher_priority; plus a CONTEXT non-voting flag | conflict_watch + CONTEXT non-voting | funding_reversal 0/2,809 opens yet leads co-fires with 23 [M:DOC, MEAS3] |
| OD-13 | funding_reversal class | CONTEXT vs REVERSAL | CONTEXT | 0/2,809 opens; gross means −0.01% / −0.29% [M:DOC] |
| OD-14 | confirmed_breakout + trend_breakout_long | shadow-only pending U0-E vs keep tradable | Shadow-only | TP0-first 0.0% at n=119 [M:DOC:0803]; T2 side observe-only [C] |
| OD-15 | Sizing per class/cell | flat 8%; by cell status; by class | By cell status (EARNED 1.0×, PROBATION 0.5×) | 8% × 2 = 16% > F08 15% [D] |
| OD-16 | M2 A/B (loosen spec vs accept strict) | per class: TRIGGER accepts strict spec v2; IMPULSE gets chase-ATR (U0-D) | Per-class split | Ignition 7/396 vs momentum 0/218 [M:M2v2] |
| OD-17 | Forward calibration data, given the shadow phase is cancelled | (a) record tags and outcomes in shadow mode without trading; (b) wait for pilot/live; (c) build a native fee-net replay corpus | (a) + (c) | No fee-net native corpus exists [M:DOC search]. MEAS3 covers T2 moments only. |
| OD-18 | Task C sequencing and T2 naming | run now vs after OD-6; `t2_` prefix for T2 setup types; reclaim rename | After OD-6. Add the prefix and fix the reclaim rename. | Reclaim collision (§4.1) |
| OD-19 | F22 broker prefix | (pending from before) | — | F-audit |
| OD-20 | G3 yield target | keep ≥ 170; regime-conditional (SSA §I) | Regime-conditional, set from U0 | 170 unreachable: 19 at 1.2, 90 at 0.5 [M:MEAS1] |
| OD-21 | Cycle alignment to bar close (also changes native cadence) | keep the 3-minute wall clock; align to 5m close + settle | Align, gated on the U10 parity test | [C:auto_trader.go:717]; 18.6R one-bar cost [M:LAT] |
