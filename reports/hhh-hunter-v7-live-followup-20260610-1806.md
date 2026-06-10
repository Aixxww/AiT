# HHH Hunter v7 Live Follow-up - 2026-06-10 18:06 CST

## Scope

Review HHH recent live cycles after the Hunter v7 confirmation-summary rollout and reduce idle cycles without relaxing hard risk controls.

## Recent HHH Decision Funnel

Latest HHH records:

| Decision cycle | Time CST | AI tokens | Result |
|---|---:|---:|---|
| #226 | 2026-06-10 17:59:54 | 0 | `no_open_review_candidates watch=1 rejected=2` |
| #225 | 2026-06-10 17:49:54 | 0 | `no_open_review_candidates watch=2 rejected=1` |
| #224 | 2026-06-10 17:39:54 | 0 | `no_open_review_candidates watch=5 rejected=0` |
| #223 | 2026-06-10 17:29:54 | 0 | `no_open_review_candidates watch=5 rejected=0` |
| #222 | 2026-06-10 17:19:54 | 0 | `no_open_review_candidates watch=4 rejected=2` |
| #221 | 2026-06-10 17:10:10 | 9716 | LLM reviewed `BASUSDT` and waited |
| #220 | 2026-06-10 17:00:14 | 0 | `no_open_review_candidates watch=2 rejected=3` |
| #219 | 2026-06-10 16:55:16 | 11800 | LLM reviewed `BSBUSDT` and waited |

The main problem in this window is not an empty universe or data source freeze. Hunter v7 continues to find candidates, but many cycles still skip the LLM because kernel tiering has no `EXECUTABLE` or `REVIEWABLE` candidate.

## #226 Root Cause

At 2026-06-10 17:59:54, Hunter v7 produced 141-symbol universe and 3 LLM-facing output candidates.

Key non-module candidates:

| Symbol | Setup | Provider quality | Kernel tier | Reason |
|---|---|---|---|---|
| `MAGMAUSDT` LONG | `leader_momentum_long` | `ready` | `WATCH` | `needs_confirmation` |
| `HOMEUSDT` LONG | `displacement_momentum_long` | `ready` | `REJECTED` | `displacement_rr_insufficient` |
| `WLFIUSDT` LONG | `displacement_momentum_long` | `ready` | `REJECTED` | `displacement_rr_insufficient` |

`MAGMAUSDT` details:

- `ai_priority=64.8`
- `setup_score=65.6`
- `timing_score=78`
- `risk_score=15`
- `liquidity_score=65`
- `taker_buy_15m=0.527`
- `reason_codes` include `strong_24h_momentum`, `solid_4h_momentum`, `accelerating_1h`, `oi_healthy_growth`, `strong_symbol_regime_override`
- `confirmation_summary.passed_hard=true`
- `confirmation_summary.passed_review=true`
- `confirmation_summary.rr=3.09`

This candidate was not good enough for `EXECUTABLE`, but it was strong enough for LLM review. The previous relative-strength review floor required higher liquidity or stronger static setup score and did not consume the new `confirmation_summary`, so it fell through to `needs_confirmation`.

## Code Fix

Added a new controlled `REVIEWABLE` path for confirmed relative-strength leader momentum:

- setup must be `leader_momentum_long`
- `execution_quality=ready`
- `ai_priority >= 62`
- `setup_score >= 55`
- `timing_score >= 65`
- `risk_score < 25`
- liquidity must be at least moderate (`>=60` when present)
- taker buy must be confirmed (`>=0.52`)
- no `taker_weak_buy`
- relative-strength evidence must be present
- OI growth evidence must be present
- `confirmation_summary.passed_hard=true`
- `confirmation_summary.passed_review=true`

New tier reason:

`momentum_reviewable_confirmed_relative_strength`

This improves open-review rate without allowing:

- displacement RR failures
- weak-taker momentum
- high-risk tags
- unconfirmed entry-zone/taker checks
- pure single-symbol exceptions

## Micro-profit Exit Status

The previous BSBUSDT micro-profit close issue is addressed by the prompt-side raw-move gate:

- near-TP1 now requires `Peak PnL >= 5.7%` and unleveraged `raw_move >= 1.0%`
- positions below that raw move remain `pre_tp1/micro-profit noise`
- peak giveback alone cannot justify close in pre-TP1 state

No new micro-profit close appeared after this prompt fix in the reviewed window.

## Verification

Commands passed:

```bash
go test ./kernel -run 'TestClassifyHunterV7CandidateTierAllows.*Momentum|TestClassifyHunterV7CandidateTierBlocksDirtyMomentumReview|TestFormatHunterV7SignalJSONMarksWatchOnlyAsDoNotOpen' -count=1
go test ./trader ./trader/binance ./kernel ./provider/local -count=1
go build ./...
```

## Next Watchpoint

For the next HHH cycle, expect at least one clean confirmed relative-strength momentum candidate to reach LLM as `REVIEWABLE` when the same structure appears. If the LLM still waits, that is an execution-quality decision; if backend blocks, inspect stop/RR/live price guard instead of further loosening candidate tiering.

## 18:14 CST Runtime Check

After restart, HHH cycle #228 no longer skipped the AI:

- candidates reached prompt
- `prompt_tokens=9485`
- `total_tokens=9715`
- LLM reviewed `MAGMAUSDT`
- LLM decision: `wait`

The wait was correct. The live `MAGMAUSDT` candidate had:

- `execution_quality=chase_risk`
- `risk_tags=["regime_against_direction","execution_stop_tightened","momentum_overheated"]`
- `confirmation_summary.passed_review=false`
- failed confirmation: `momentum_not_exhausted`
- RSI actual was above the 78 threshold

This proved the open-review path was restored, but also exposed one extra token-waste path: generic chase-risk review allowed overheated leader momentum into `REVIEWABLE` even though the confirmation summary already said review failed.

Follow-up code fix:

- `hunterV7ChaseRiskReviewableReason` now blocks `leader_momentum_long` when `momentum_overheated`, `momentum_rsi_overheated_wait`, or `confirmation_summary.passed_review=false`.
- The confirmed relative-strength path remains active only when `confirmation_summary.passed_hard=true` and `confirmation_summary.passed_review=true`.

Expected behavior after this follow-up:

- clean confirmed relative-strength leaders still reach `REVIEWABLE`
- overheated chase-risk leaders stay `WATCH`
- HHH avoids both old 0-token candidate starvation and avoidable LLM calls on obvious wait-only overheat

## 20:00 CST Counter-trend Loss Review

New losing trades after the open-rate fixes:

| Position | Entry CST | Exit CST | Net PnL | Setup |
|---|---:|---:|---:|---|
| `CLOUSDT LONG` | 2026-06-10 18:28:39 | 2026-06-10 18:53:38 | `-0.094804` | `panic_reversal_long` |
| `BUSDT LONG` | 2026-06-10 18:58:40 | 2026-06-10 19:46:12 | `-0.324297` | `panic_reversal_long` |

Both were counter-trend long entries:

- setup: `panic_reversal_long`
- market/regime context: `trend_down` / `regime_against_direction`
- kernel tier before fix: `EXECUTABLE`
- LLM opened because 5m/taker/RR looked acceptable

The structured confirmation summary shows why these should not have been executable:

`CLOUSDT` at 18:28:

- `confirmation_summary.passed_review=false`
- missing review check: `5m_close_above_ema20_or_entry_zone_mid`
- data-layer actual price was below entry-zone midpoint

`BUSDT` at 18:58:

- `confirmation_summary.passed_review=false`
- missing review check: `5m_close_above_ema20_or_entry_zone_mid`
- later hold decisions show the trade was quickly trapped below 5m/15m structure

Root cause:

`panic_reversal_ready_core_ok` did not consume `confirmation_summary`. In trend-down counter-trend conditions, that let a provider-level `ready` signal become `EXECUTABLE` even when the shared confirmation evaluator had already marked the setup as not review-passed. The LLM then overrode the failed structured confirmation by reasoning from a single 5m EMA/taker/RR check.

Follow-up code fix:

- Added global `countertrend_confirmation_wait`.
- For any setup with `risk_tags` containing `regime_against_direction`, if `confirmation_summary.passed_review=false`, kernel tier is forced to `WATCH`.
- Updated Hunter v7 prompt: counter-trend candidates with `confirmation_summary.passed_review=false` must wait and cannot be overridden by high priority, single 5m EMA reclaim, strong taker buy, or acceptable RR.

This keeps high-quality counter-trend reversals possible only when the structured confirmation summary actually passes. It directly targets the counter-trend loss mode without banning all panic reversal longs or adding symbol-specific exceptions.
