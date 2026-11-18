# U0 pre-registrations and single-run record

This document records the pre-registered single test of the U0 programme (U0-H2) verbatim, the owner ruling on its one input deviation, and its result.
- The analysis code is in `scripts/t2_u0/` and the gated harness tests are in `trader/u0_protector_*_test.go`.
- The data lives in the private `ait-t2-data` repository and is never committed here.

## U0-H2 pre-registration (frozen text, verbatim)

The file `U0-H2_PREREGISTRATION.md` had sha256 `24289cdd3bb681468c155eff105e52d98022c20a3a74c26dc4c778387bb10916` when frozen, and the hash still matched when the test ran.

> # U0-H2: pre-registered single test, live protector A against the V2 offset trail, on live trades
> 
> Frozen 2026-11-18, before any A-against-V2 number was computed on live data. This file's sha256 is reported in-session. Any change to it after the run invalidates the test.
> 
> ## Question
> 
> On real live trades, does the V2 offset trail beat the incumbent live protector A by enough to justify changing U14's semantics?
> 
> ## Sample (fixed in advance; no subsets used for the decision)
> 
> - **Definition.** Every live position in `ait-t2-data/live_db` (head 6598602) that meets three conditions:
>   - it was opened by a successful bot `open_*` decision in `decision_records`;
>   - it matches a `trader_positions` row by trader_id, symbol and side, with entry within 180 s of the decision timestamp;
>   - it is CLOSED and the decision carries a stop_loss.
> - **Size.** U0-H found 138 such trades.
> - **Exclusions.** A trade is dropped only when the 1m path below can't be built. Every drop is counted and reported.
> 
> ## Inputs per trade
> 
> | Input | Value |
> |---|---|
> | Entry | Actual `trader_positions.entry_price` at `entry_time` |
> | Stop | The decision's stop_loss |
> | Take-profit | The decision's take_profit, capped at 7.75% from entry, as in U0-F |
> | Leverage | The **decision** leverage. The position-row leverage of 1 on 67 rows is a sync artifact: the decision and position values disagree on exactly those rows. |
> | Price path | Binance USD-M 1m klines from data.binance.vision daily archives, from the entry minute to entry + 12 h, matching the 694 cache window |
> | Fees | Taker 0.05% per side on every leg (OD-23) |
> | R unit | abs(entry − stop) |
> 
> ## Arms
> 
> - **A:** the live protector at head `3311417`, through the U0-F harness unchanged: exchange stop and TP inside the bar, stop checked first, protector polled at each 1m close, $1000 notional.
> - **V2:** exactly as frozen in U0-F2.
>   - Exchange stop at the plan stop and exchange TP as above.
>   - Once the peak (intrabar favourable extreme) is ≥ 1.0 R, the stop is set to peak − 0.5 R and only ratchets up. Each update applies from the next bar.
>   - Full exit. No partials.
> - Both arms use the same paths, entries, orders and fees. Anything still open at the end of the window is marked at the last close.
> 
> ## Statistic and gate (thresholds as in T-EXIT)
> 
> - **ΔR** = R(V2) − R(A) per trade.
> - **Pass** requires all three:
>   - mean ΔR ≥ +0.05;
>   - P(mean ΔR > 0) ≥ 0.9, from 5000 bootstrap resamples with seed 20261118;
>   - mean ΔR > 0 in each of the three entry-time tertile folds.
> - **One test, run once.** No other cut, parameter or variant enters the decision.
> 
> ## Decision rule
> 
> - **PASS:** V2 becomes a candidate for a NEW owner decision on U14's semantics.
> - **FAIL:** OD-22 (a) is confirmed on live data, and U14 proceeds with defect fixes only (F29–F31 plus the R-unit/config refactor, no change in behaviour).
> 
> ## Reported for context only (not part of the decision)
> 
> - **Sim fidelity:** mean of sim-A minus the actual realized fee-net R, and their correlation. If the absolute mean difference is above 0.15 R, the result is labelled LOW FIDELITY, but the gate verdict still stands as computed.
> - Mean ΔR by live exit mechanism (U0-H attribution), and V2 − P and A − P.
> - **Known biases, declared in advance:**
>   - A is polled once a minute, while live polling runs every 15–30 s.
>   - V2 uses intrabar highs and lows.
>   - The live protector code in May–July 2026 differs from head A.

## Addendum: owner ruling on an input deviation

Written before any A or V2 number was computed. sha256 `435126269ab0a34d95d31c7a734c4bee9f3835e6d8ca084c0d988bf22176a4f4`.

> # U0-H2 addendum: owner ruling on an input deviation (2026-11-18)
> 
> Written before any A or V2 number was computed on live data. The frozen pre-registration file
> `U0-H2_PREREGISTRATION.md` is unchanged (sha256 24289cdd3bb681468c155eff105e52d98022c20a3a74c26dc4c778387bb10916).
> 
> ## The deviation
> 
> The pre-registration did not cover this case. On 2 of the 138 trades, the decision's stop_loss was on the wrong side of the actual entry for a LONG:
> 
> | Position | Symbol | Side | Entry (UTC) | Entry price | Decision stop |
> |---|---|---|---|---|---|
> | 9 | UBUSDT | LONG | 2026-05-14 02:08 | 0.2088 | 0.224 |
> | 11 | BILLUSDT | LONG | 2026-05-14 18:35 | 0.20696 | 0.208 |
> 
> ## Ruling
> 
> Owner option (a): exclude both trades as invalid inputs and run once with n = 136. Everything else stays as frozen:
> - same arms;
> - same gates (mean V2 − A ≥ +0.05 R, P ≥ 0.9 from 5000 bootstrap resamples with seed 20261118, all three entry-time tertiles positive);
> - same fee model.
> 
> The tertiles are taken over the 136 included trades.

## Result (single run, n = 136)

| Gate | Value | Pass |
|---|---|---|
| Mean V2 − A ≥ +0.05 R | +0.0775, 90% CI [−0.058, +0.216] | yes |
| P(mean > 0) ≥ 0.9 | 0.825 | **no** |
| All three entry-time tertiles > 0 | +0.227 / +0.065 / −0.062 | **no** |
| **Verdict** | | **FAIL** |

**Totals over 136 trades:**

| Arm | Total | Per trade |
|---|---|---|
| A | −12.01 R | −0.088 |
| V2 | −1.46 R | −0.011 |
| P (context only) | +3.28 R | +0.024 |
| Actual live outcome | −13.56 R | −0.100 |

**Sim fidelity:** mean simulated A minus realized is +0.011 R, with correlation +0.57. That is within the 0.15 R limit.

**The three declared biases:**
- once-a-minute polling of A: most likely favours V2;
- V2's intrabar extremes: favour V2;
- the change in live protector code since the trades ran: direction unknown, but small given the fidelity result.

On net the biases favour V2, which still failed.

**Decision (owner, 2026-11-18):** OD-22 (a) is confirmed on live data. The protector question is closed, and U14 is locked to the defect-only scope.
