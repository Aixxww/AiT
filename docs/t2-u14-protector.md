# U14: protector in R units and config, one shared code path, F29–F31

Record for the redesign plan §6.3 U14 row (OD-25, OD-26; locked to defect fixes plus the R-unit/config refactor by the OD-22 close-out, §10.3).

**Owner decisions (U14).**
1. **Units:** every threshold moves into config as a value plus a unit (`roe_pct`, `price_pct` or `r`). The defaults are today's values in today's units. Thresholds and position state also appear in R. An R-unit rule does not fire without a planned stop, and this is logged.
2. **F29:** after a partial, the rebuild places the more protective of the breakeven stop and the current active stop, and saves it to state.
3. **F30:** giveback and trail closes each fire at most once per position.
4. **F31:** after the stop, the take-profit is re-placed at the open's effective take-profit price, sized to the remaining quantity.
5. **Shared path:** a pure step for the decisions; the live monitor is a thin adapter; the walker is non-test code calling the same step.

## Config: `risk_control.protector`

Read through `store.EffectiveProtector`; nil means all defaults. `store/protector.go` lists every field with its default.

**Levels with a unit.** These are `{value, unit}`, with the default in brackets:
- hard loss (−12 roe), early loss (−8 roe);
- profit floor peak (5 roe), buffers (0.8 / 3 roe);
- profit lock mid/high peaks (10 / 15 roe);
- TP0 (8 roe), TP1 (6 roe) with min move (1.0 price), TP2 (12 roe) with min move (1.5 price);
- near-TP1 peak (5.7 roe) and second chance (5.4 roe);
- near-TP1 loss exit (−5 roe), pre-TP min current (3 roe), pre-TP peak (8 roe);
- dynamic-stop early peak (8 roe) and early move (0.60 price);
- the synthetic base risk of the DynamicStopManager (35 roe);
- micro-TP0 min move (0.55 price);
- the status-log level (3 roe).

**Plain numbers:**
- poll intervals (15 / 30 / 60 s);
- default leverage 10;
- plan lookback of 50 records;
- hard-loss slack 1.1;
- the early-loss (15 min), pre-TP hold (20 min) and dynamic-stop early-hold (10 min) windows;
- drawdown percentages (50, 45, 35, 45, 80);
- profit-lock ratios (0.30, 0.45);
- close ratios (0.35, 0.40, 0.50, 0.5);
- breakeven buffers (0.001, 0.0015);
- min close notional 12;
- dynamic-stop cadence 30 s, ATR percentile 50, min stop delta 0.0002;
- the DynamicStopManager parameters: volatility 80/20/1.3/0.7, time decay after 30 min at 0.02 per minute capped at 0.5, trailing 0.5 / 0.4.

The decay cap was an inline literal and is now `DynamicStopConfig.TimeDecayMaxFraction`; 0 reads as 0.5.

**How levels are compared.**
- A level is compared in its own unit: ROE against the position's ROE, price against the price move, R against the move divided by the planned stop distance. The defaults therefore give exactly the pre-U14 decisions, with no rounding from converting units.
- Levels that the old code wrote as compile-time constants keep that rounding: the near-TP1 levels are separate levels rather than 6 × 0.95, and the 0.55% and 35% are divided as decimals.
- R = ROE / (100 · L · s), where s is the planned stop distance from the opening decision.
- On a position's first poll, the protector logs every level in R; actions and status lines show the position's R.
- Without a planned stop, the R-unit levels are listed once in a warning and never fire.

## Shared code path

- **`trader/protector_engine.go`:** the rules, stop candidates, rebuild stop, micro-TP0 and plan hydration, as pure functions of config, state, position, readings and a clock.
- **`trader/protector_step.go`:** `protectPosition`, one step per position. It covers state, hydration, the dynamic stop update, the decision, the action and the done-flags.
- **Live monitor:** `checkPositionDrawdown` reads positions and the plan and calls the step at `protectionNow()`.
- **`trader/protector_walk.go`:** the walker. Per 1m bar, resting orders fill first (stop before take-profit), then the step runs at the bar close time. The venue is `trader/protector_sim_venue.go`. The U0-F and U0-H2 harnesses now call the walker; their old test-only copy, with its copied price triggers, is gone.
- **Kept wrappers:** the old function names (`choosePositionProtectionAction`, `updateDynamicProtectionStop`, `executeProtectionAction`, `microTP0Price`, and the stop helpers) call the engine with the default or strategy config. This is why the existing tests are unchanged.

## Fixes

- **F29:** `rebuildProtectionStopsWithBuffer` places the more protective of entry ± buffer and the position's active stop. It records the placed stop as `ActiveStopLoss`, so a profit-floor or trailing stop is no longer loosened. A later time-decay candidate below entry also no longer counts as tighter than the breakeven stop.
- **F30:** new `GivebackDone` and `TrailDone` flags. Each giveback rule and the trail rule close at most once per position. Hard loss, the stop and the take-profit still apply to the remainder.
- **F31:** hydration keeps the open's take-profit (`ExchangeTakeProfit`, the decision's effective take-profit). After the stop rebuild, `SetTakeProfit` re-places it for the remaining quantity, unless the mark is already beyond it. A failure is logged and not fatal; the stop protects the remainder, as at open.

## Verification

- **Existing protector tests pass unmodified.** These are auto_trader_risk_test, protector_p02, protector_microtp0, protector_ab_sim, the F27/F28 rebuild tests, the F03 tests and the U0 walk tests. The only test-file change is the U0 harness runner (`u0_protector_harness_test.go`): `u0fWalkA` now calls the production walker, and the old test-only exchange is removed.
- **New tests** (`trader/protector_u14_test.go`):
  - the defaults equal the old literals, and an unknown unit reads as the default;
  - R-unit levels fire at the R threshold and not without a plan;
  - the levels-in-R log;
  - F29 for long, short and the "planned stop only" case;
  - F30 for giveback and trail, with hard loss still active;
  - F31, including the exact order sequence (close → side cancel → stop → take-profit) and the skip when the take-profit is behind the mark;
  - **walker and live adapter on six synthetic paths** (long/short, TP0 partials, TP1/TP2/trail, early loss, late drift, near-TP1 giveback at 5×): identical order logs, stop moves and fills. The live side reads the plan from a decision record in SQLite;
  - the walker uses the bar clock.
- `go build ./...`, `go vet ./...` and `go test ./... -count=1` pass (30 packages).

## U0-F and U0-H2 re-run (for information; no decision attached)

**The refactor alone reproduces the pre-U14 harness exactly.** With the fixes switched off and the old harness clock emulated, the new walker's U0-F and U0-H2 outputs are byte-identical to the base outputs (which equal the packaged `u0f_rows.json` and `u0h2_A.json`).

**The old harness had a clock defect.** It shifted `OpenedAt` by (wall-clock now − simulated now) to fool `time.Since`, but the rules read `protectionNow()` (the simulated time) against that shifted value. In the rules the position age was therefore negative by weeks:
- the 15-minute early-loss exit (−8% ROE) stayed active for the whole walk;
- the 20-minute pre-TP1 rules could never fire.

The shared step uses one clock, so the re-run separates the clock correction from the fixes.

**U0-F** (380 fills; fee-net R; 90% bootstrap CI):

| Model A | mean A | A − P | A − D | hard-loss exits | trail closes | giveback closes | stop moves | exchange-TP exits |
|---|---|---|---|---|---|---|---|---|
| base (pre-U14 harness) | −0.176 | +0.017 [−0.080, +0.110] | −0.000 [−0.061, +0.058] | 81 | 171 | 105 | 986 | 2 |
| shared step, no fixes (clock corrected) | −0.159 | +0.034 [−0.063, +0.127] | +0.017 [−0.044, +0.075] | 44 | 171 | 105 | 996 | 3 |
| U14 (clock + F29–F31) | −0.156 | +0.037 [−0.060, +0.130] | +0.020 [−0.042, +0.079] | 44 | 31 | 40 | 628 | 40 |

- **Clock correction:** it changes 37 fills. All 37 were early-loss exits in the base (−8% ROE after the 15-minute window had passed). With the corrected clock they exit at the planned stop (29), a moved stop (7) or the take-profit (1).
- **Fixes:** they change 92 more fills. F30 removes the repeated trail and giveback closes (171 → 31, 105 → 40). F31 lets 40 remainders exit at the re-placed take-profit.
- **No CI excludes 0.**

**U0-H2** (136 live trades; V2 − A, the pre-registered gate is unchanged):

| Model A | mean A | V2 − A | P(mean > 0) | folds | gate |
|---|---|---|---|---|---|
| base | −0.088 | +0.078 [−0.058, +0.216] | 0.825 | +0.227 / +0.065 / −0.062 | FAIL |
| shared step, no fixes | −0.096 | +0.085 [−0.048, +0.222] | 0.848 | +0.234 / +0.079 / −0.062 | FAIL |
| U14 | −0.088 | +0.077 [−0.057, +0.215] | 0.824 | +0.225 / +0.064 / −0.062 | FAIL |

The OD-22 rule ("if A − D ≥ 0, keep A") and the U0-H2 verdict (FAIL) read the same on the re-run.

## Not changed

- protector semantics beyond F29–F31;
- `geometry_mode`;
- the peak-recovery lookback in `recoverPositionPeakPnLPctFromRecentPrompts` (20 records, −1 min), which restores state and is not a protector threshold;
- the outcome tracker and the replay exit model D.
