# U7: E[R] estimator falsification tests F-1..F-3 (pre-registration)

Frozen 2026-11-18, before any F-1..F-3 number or any E[R] prediction was computed on the 694. Its sha256 is reported in the U7 PR. Any change to it after the run invalidates the test. The test is run exactly once. The single result is recorded in the PR and appended below the line at the end of this file.

Source of the protocol: redesign plan §2.4 (estimator, falsification table), §5.3 (cell rule), §6.3 U7 row, OD-2 (eval conventions), OD-8 (cell parameters).

## Data (fixed in advance)

- **Events:** the 694 T2 events (`entry_study/events.json`) with their 694 M3 snapshots.
- **Per-event record:** the U5 acceptance replay `cmd/t2_u5_tags -horizon-hours 48` at the U7 base commit (`02fed72`, PR #22 merge). This is the run whose fee-net R is identical to the U0-A walk on 615/615 usable events. The record used is the final-state record: the TRIGGERED record when the event reaches TRIGGERED, else the created-state record.
- **Usable events:** `fee_net_outcome` ∈ {tp1, stop, none}, giving 615. The 79 degenerate entries are excluded, as in U0-A.
- **Realized R:** the tracker's `fee_net_r` (OD-2: full exit at TP1 or stop, stop first, cost 0.0015·E inside the R unit; `none` = 0).
- **Planned R at TP1, R_win:** `V7FeeNetRRAtTP1(fee_net_entry, stop, tp1, 0.0015)`.
- **Cell inputs:** from the record's tags: `signal_class`, `family`, `direction`, `rr_band` (decision-time band).
- **Score:** the T2 scanner score (`events.json` `score`), the input of the current 82 threshold.
- **Folds (OD-2):** trigger-time tertiles over all 694 events (`common.trigger_folds`). Usable counts are 213 / 203 / 199.
- **Ranking universe U_f:** the usable events of fold f whose final-state `outcome_source` is `prompt_open_review`, meaning the current pipeline sends them to the LLM. Sizes are 78 / 77 / 72 (227 in all). Shadow-only and observe-only families are never admitted, so they are not ranked.

## Estimator (as implemented in `kernel/hunter_v7_expected_r.go`; parameters from `store.EffectiveV7ExpectedR` defaults)

- **Outcomes:** each training row is tp1, stop or none.
- **Hierarchy:** global → class → cell. The cell key is family × direction × RR band. Direction is split by default (`pool_directions` false), because the plan's family rows treat long and short separately and admit them differently (OD-27, OD-28).
- **Shrunk rates:** for o ∈ {tp1, stop}, with one k for both levels:
  - p_global = n_o / n
  - p_class = (n_o,class + k·p_global) / (n_class + k)
  - p_cell = (n_o,cell + k·p_class) / (n_cell + k)

  An unseen cell uses its class; an unseen class uses the global rate.
- **Estimate:** E[R] = p_tp1·R_win − p_stop·1 − c, with c = `extra_cost_r` = 0, since OD-2 already puts the cost inside R.
- **Lower bound:** LB = E[R] − z·sqrt(Var / n_eff), where:
  - Var = p_tp1·R_win² + p_stop − (p_tp1·R_win − p_stop)²;
  - n_eff = n_cell + k;
  - z = Φ⁻¹(`lower_bound_confidence` = 0.90).
- **Choice of k:** from `k_grid` = [0.5, 1, 2, 5, 10, 20, 50, 100, 200], by leave-one-fold-out mean squared error of E[R] against realized R. The first minimum in grid order wins. Raw scores are not inputs (§2.4 rule; U0-A and U0-B showed no fold-stable monotone relation).

## Held-out prediction (no leakage)

For each test fold f:
1. The training set is the usable events of the other two folds, all classes and sources.
2. k is chosen inside the training set: train on one training fold and score on the other, both ways, and sum the squared errors.
3. The estimator is fitted on both training folds with that k.
4. Every event of fold f is predicted.

## Tests

The admission count is m_f = ceil(`admit_fraction` · |U_f|), with `admit_fraction` = 0.5. Ties are broken by earlier trigger time.

| ID | Procedure | Pass |
|---|---|---|
| F-1 | Per fold: the E[R] set is the top m_f of U_f by LB; the score set is the top m_f by score. Compare the mean realized R of the two sets. | E[R] set mean > score set mean in at least 2 of 3 folds |
| F-2 | Pool the held-out predictions over U (227). Sort by predicted E[R] ascending and split into 5 quintiles (row i goes to quintile ⌊5i/n⌋). For each quintile, take the realized mean R and its 90% bootstrap CI (5,000 resamples). The quintile passes if the mean predicted E[R] lies inside the CI. | at least 4 of 5 quintiles |
| F-3 | Observed: the pooled mean realized R of the three fold E[R] sets. Null: 1,000 draws, each taking m_f events uniformly without replacement from every U_f, pooled. p = (1 + #{draw mean ≥ observed}) / 1,001. | p < 0.10 |

**Seed:** 20261118 (`seed`), from Go `math/rand` with one source. The bootstrap uses `seed`, and the F-3 draws use `seed`+1.

**F-2 interpretation.** The plan's wording is "realized inside the 90% bootstrap CI of predicted". A point prediction has no sampling interval, so the interval is taken over the realized quintile mean, and the check is that the predicted mean lies inside it. This is the same condition stated from the other side.

## Decision rule (plan §2.4)

- Switching E[R] to ranking needs F-1 and F-3 both to pass. U7 does not switch it in any case: the estimator stays shadow-only (logged, not gating).
- If F-1 or F-3 fails, it stays shadow-only, and ranking remains arrival order.
- F-2 is reported. It does not decide the switch on its own.
- Nothing in this protocol is tuned after the run.

---
Result: appended after the single run. The text above this line is the frozen protocol: sha256 `e649fb3224fdbec0e466b89f682ae49235730a57958af042159c470ee2204f16` as a file ending with the placeholder line "Result: (appended after the single run)". The hash was checked before the run.

**Run** (2026-11-18, once):
- **Input:** `cmd/t2_u5_tags -horizon-hours 48` at `02fed72`. It reproduces U5: recall 694/694, tracker R identical to the U0-A walk on 615/615 usable events, mean −0.1152.
- **Command:** `cmd/t2_u7_falsify`. Output `u7_result.json`, sha256 `d4d67dba094db52bf79f2ddf18d9191faa2d719070e552fb53bbb2de024d97cf`, kept in the sandbox and not committed.
- **Sizes:** 615 usable events; universe 227 (78 / 77 / 72).

| fold | k (nested) | train n | \|U_f\| | m_f | E[R] set mean R | score set mean R | E[R] wins |
|---|---|---|---|---|---|---|---|
| 1 | 1 | 402 | 78 | 39 | −0.3717 | +0.1308 | no |
| 2 | 200 | 412 | 77 | 39 | −0.3540 | −0.2545 | no |
| 3 | 200 | 416 | 72 | 36 | −0.0320 | −0.0646 | yes |

**F-1: 1 of 3 folds. FAIL.**

| quintile | n | mean predicted | realized mean | 90% bootstrap CI | inside |
|---|---|---|---|---|---|
| Q1 | 46 | −0.4008 | −0.0261 | [−0.1568, +0.0985] | no |
| Q2 | 45 | −0.2106 | +0.1801 | [+0.0064, +0.3463] | no |
| Q3 | 46 | −0.0430 | +0.0443 | [−0.1758, +0.2684] | yes |
| Q4 | 45 | +0.2035 | −0.3943 | [−0.6361, −0.1235] | no |
| Q5 | 45 | +0.8424 | −0.2248 | [−0.5283, +0.1013] | no |

**F-2: 1 of 5 quintiles. FAIL.** Realized R falls as predicted E[R] rises, so the estimator is anti-calibrated on held-out data.

**F-3:** observed pooled mean −0.2583 against a random-set null mean of −0.0826; p = 0.9980. **FAIL.**

**Decision (rule above):** F-1 and F-3 fail, so the estimator stays shadow-only and ranking remains arrival order. Nothing is tuned after the run.

**Descriptive note (not part of the test; it decides nothing).** By predicted quintile, the realized TP1-first rate is about 0.64 / 0.76 / 0.57 / 0.29 / 0.26. The estimator scales a cell-level TP1 rate by each signal's own R_win. On these events, the signals with a far TP1 (large R_win) reach TP1 far less often than their cell rate, which is the near-TP1 / far-stop asymmetry recorded in MEAS1. A model that does not let p depend on R_win within a cell cannot represent this. Any successor estimator needs its own new pre-registration.
