# Validation sampling rule: non-down-market coverage

**Status:** process rule (fix queue item 16, P2-16). No code is involved.
**Applies to:** every validation run whose results are used to change a rule or a threshold. Examples: earning a cell, loosening WATCH, lifting a shadow-only setup, moving an RR floor.

## 1. Why

Every validation sample so far came from a falling market. Results from one market phase only describe that phase. This matters most for directional setups: a short setup such as `ignition_early_short` will look better in a falling market than it would elsewhere. A result is only trusted beyond down markets if the sample covered non-down markets as well.

## 2. Which samples count as non-down-market

The unit is a **round**: one harness scan. Each round has one detected regime, chosen by `DetectV7MarketRegime` (`provider/local/hunter_v7_regime.go`) from BTC and ETH. It appears in:
- the round report as `snapshot.regime`;
- the run summary's Round table, Regime column;
- each signal row as `market_regime`.

The fee-net tool (`cmd/v7_fee_net_offline`) reports the regime mix from the rows' `market_regime`.

Each regime falls into one bucket:

| Bucket | Regimes | Rule in the detector (BTC or ETH) |
|---|---|---|
| **Down** | `panic_dump`, `market_pullback`, `trend_down` | 24h below −8% or 1h below −5%; 24h from −5% to −8%; 4h EMA20 below EMA60 with a negative 24h move |
| **Up** (non-down) | `mania_pump`, `trend_up` | 24h above +8% with 4h above +3%; 4h EMA20 above EMA60 with a positive 24h move |
| **Non-directional** (non-down) | `range`, `compression`, `rotation` | low ADX or narrow bands; or BTC and ETH flat while 10+ alts outperform |
| **Unclassified** | `mixed` | nothing else matched |

- **Non-down-market** means the Up and Non-directional buckets.
- `mixed` does **not** count toward non-down coverage, because it is the detector's fallback and can hide a weak decline.
- Only **accepted, non-degraded rounds** count. Rejected and degraded rounds are excluded, as they already are from the run's valid figures.

**Cross-check.** The detector checks for an uptrend before a downtrend, and it calls a round `compression` or `range` whenever ADX is low. So a round in a slowly falling market can still be labelled non-directional. A non-directional round therefore counts as non-down only if its `snapshot.btc_24h` is **above −2%**.

## 3. Check before trusting a run's results

Run these checks after the run and before reading any result. Write the outcome at the top of the analysis.

1. **Count rounds by bucket.** Use the run summary's Round table, or each round report's `snapshot.regime` and `snapshot.btc_24h`. Record the number and share of accepted rounds in Down, Up, Non-directional (after the cross-check) and Unclassified.
2. **Count episodes.** An episode is a run of consecutive accepted rounds in the same bucket. Consecutive rounds of one episode are one market state, not independent samples.
3. **Count scored rows by bucket.** Use the fee-net tool's `market_regimes` and its `market_regime` table in `summary.json`, from the **deduplicated sample**. For every cell or table a decision rests on, note n per bucket.
4. **Apply the coverage test.** A run passes only if all three of these hold:
   - at least **30% of accepted rounds** are non-down;
   - those non-down rounds come from at least **2 independent episodes** spanning **at least 2 different UTC days**;
   - for each cell a decision rests on, the non-down part reaches the cell rule's own sample floor (**n ≥ 30**, `cell_rule.min_samples`) in the deduplicated sample.
5. **Check the settings are comparable.** The run summary's `effective_flags` must show no `rounds_with_different_settings` across the rounds being pooled. If the settings changed mid-run, check coverage separately for each settings group.

## 4. What to do when the sample set is down-market only, or fails the test

Take these actions in order:

1. **Mark the results.** Put **`DOWN-MARKET-ONLY`** in the title and first line of every report, table or summary built from the run. Also state the bucket counts from section 3. The mark stays on any figure quoted elsewhere.
2. **Limit what the results may decide.** Down-market-only results must not be used to **loosen** anything globally:
   - no earning a cell;
   - no lifting a shadow-only admission;
   - no lowering a floor;
   - no relaxing WATCH.

   They may be used for a **down-market-scoped** observation, or as a reason to keep or tighten a rule, and only if the owner decides so explicitly.
3. **Widen the window, if stored data allows.** Add earlier stored runs from other periods to the same analysis, using `-from` / `-to` on the fee-net tool. Pool them only if their `effective_flags` match (section 3, step 5). Then repeat the check in section 3. The harness scans live markets and cannot recreate past scans, so widening only works with rows that already exist.
4. **Re-sample.** If widening isn't possible or still fails, schedule more runs and keep the result open until the coverage test passes. The market phase cannot be chosen, so this means continuing to run across several days. It does not mean re-running the same period.
5. **Never mix buckets silently.** When results are pooled across buckets, also show them per bucket. If the down and non-down figures point in different directions, report both and don't quote the pooled number on its own.

## 5. Current position

- The Tier A rerun sample (short ignition n=291, WATCH 1,312 rows, REVIEWABLE 104 rows) is from a falling market. When the fee-net tool is run on it, its answers to "is WATCH too strict?" and "should short ignition stay shadow-only?" are **`DOWN-MARKET-ONLY`** under this rule.
- Those answers can't justify lifting short ignition's shadow-only admission or loosening WATCH until a run passes the coverage test in section 3.
