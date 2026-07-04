# Hunter v7 Binance Live Full-Chain Optimization - 2026-07-04

## 1. Live Validation Scope

- Data source: Binance USD-M futures real-time REST snapshot.
- Validation command:
  `go run ./cmd/hunter_v7_validate -top-detail 220 -max-output 30 -watch-output 5 -min-priority 45 -aggressive=true -out-dir reports`
- First run report:
  `reports/hunter-v7-live-validation-report-20260704-085215.md`
- Post-change run report:
  `reports/hunter-v7-live-validation-report-20260704-090747.md`

## 2. Before / After Snapshot

| Metric | Before | After |
|---|---:|---:|
| Binance symbols | 524 | 524 |
| Hunter v7 universe | 136 | 215 |
| REST errors | 173 | 0 |
| Signals | 8 | 13 |
| Setup types | 5 | 8 |
| Runtime tiers | REVIEWABLE=2, WATCH=6 | REJECTED=2, REVIEWABLE=2, WATCH=9 |
| Prompt-final tiers | REVIEWABLE=2, WATCH=6 | REJECTED=2, REVIEWABLE=2, WATCH=9 |
| Validation issues | 0 | 0 |

The first run had partial Binance REST coverage. That reduced the effective universe and made "no openable candidate" look more like a strategy problem than a data coverage problem. The validator now reports REST coverage quality explicitly so future no-open cycles can be separated into data coverage, setup quality, and execution guard causes.

## 3. Full-Chain Diagnosis

1. Tiering was still conservative but mostly correct.
   - VELVET and XRP entered `REVIEWABLE` in the first run.
   - HMSTR and SKYAI entered `REVIEWABLE` in the post-change run.
   - No `EXECUTABLE` candidate appeared because all open-review candidates still needed live confirmation.

2. Range expansion SHORT needed a narrower reviewable bridge.
   - Strong event shorts such as GUA-like conditions should be visible to the LLM as `REVIEWABLE` when they only lack live micro confirmation.
   - Event shorts with `micro_reversal_against_signal`, `range_expansion_exhaustion`, `high_volatility`, or `funding_extreme` should remain `WATCH`.
   - The post-change run correctly kept GRASS and XPLUS in `WATCH` because both had `micro_reversal_against_signal`.

3. Required confirmations needed backend refresh parity.
   - The selector can promote live-confirmable `REVIEWABLE` candidates.
   - The trader guard must then require fresh REST and orderbook micro confirmation before any open.
   - Without that parity, a `REVIEWABLE` candidate could either be too loose or permanently blocked.

4. Report visibility was incomplete.
   - The prior report said no format/recognition issue, but did not flag `rest_errors=173`.
   - REST error rate and universe coverage are now first-class report fields and can become issues.

## 4. Implemented Changes

- `kernel/engine.go`
  - Added a restricted `range_expansion_event` live-reviewable path.
  - Allows only high-quality event candidates with aligned flow, healthy entry zone, acceptable readiness/window health, and no exhaustion/reversal danger tags.
  - Keeps high-volatility, micro-reversal, exhaustion, extreme funding, late-short, and poor-liquidity cases out of `REVIEWABLE`.

- `trader/auto_trader_risk.go`
  - Extended refresh-satisfiable confirmation codes for range expansion events.
  - Allows summary-missing `REVIEWABLE` confirmations to pass only after both `fresh_rest_confirmed` and `fresh_micro_confirmed`.
  - Forces guard refresh for `REVIEWABLE` candidates whose required confirmations are live-refreshable even when the original summary is absent.

- `cmd/hunter_v7_validate/main.go`
  - Added real-time data coverage quality to the markdown report.
  - Emits `binance_rest_partial_coverage` and `universe_coverage_low` issues when REST errors or universe shrinkage can distort signal interpretation.

- Tests
  - Added classification tests for range expansion live-reviewable promotion and exhaustion blocking.
  - Added trader guard tests for summary-missing confirmation gaps before and after fresh refresh.

## 5. Current Operating Interpretation

- `REVIEWABLE` is now the correct bucket for "can become openable inside this cycle after live confirmation".
- `WATCH` remains correct when:
  - the signal has micro reversal against direction,
  - event exhaustion is present,
  - funding or volatility risk is extreme,
  - liquidity is poor,
  - timing/window health is too low,
  - scalp geometry cannot satisfy backend RR/SL constraints.
- `EXECUTABLE=0` in a single cycle is acceptable when no candidate has fully passed required confirmations. The objective is to keep enough qualified `REVIEWABLE` candidates visible each cycle without bypassing backend refresh and risk guards.

## 6. Next Optimization Targets

1. Add 30m/60m MFE/MAE tracking for the potential pool so unmatched high-potential names can be converted into new modules only when post-signal movement proves useful.
2. Reduce Binance REST variance with retry/backoff or a second validation pass when `rest_errors` exceeds 20% of symbols.
3. Build a separate `range_expansion_event` after-confirm audit bucket to compare:
   - WATCH with `micro_reversal_against_signal`,
   - REVIEWABLE after fresh confirmation,
   - missed TP0/TP1 outcomes.
4. Consider a dedicated short-side event module only after several cycles show that `micro_reversal_against_signal` candidates still reach TP0/TP1 more often than they reverse.

