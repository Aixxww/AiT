# Hunter v7 Global Signal and Exit Design - 2026-06-10

## Objective

Improve HHH open rate and win rate without hard-coding single trade examples into strategy rules.

The correct direction is not to add symbol-specific exceptions such as LAB, MOVE, U, or BSB. The system needs one coherent pipeline:

1. Pattern modules describe market structure.
2. A shared confirmation engine evaluates whether the structure is tradable now.
3. A single execution tier classifier maps signals to `EXECUTABLE`, `REVIEWABLE`, `WATCH`, or `REJECTED`.
4. Backend guards verify live price, stop, TP, RR, and data quality before order placement.
5. Position exits use the same profit-protection semantics in both mechanical protector and LLM prompt.

## Current Pipeline

Current flow:

`Snapshot -> Universe -> Pattern Modules -> finalizeV7SignalForExecution -> Router -> Kernel Tier -> Prompt -> LLM -> Backend Guard -> Order`

The architecture is directionally right, but the semantics are split:

- `RequiredConfirms` are strings, mostly interpreted by the LLM, not by the data layer.
- `ExecutionQuality` and kernel `execution_tier` repeat similar ideas with different thresholds.
- Some modules label `ready` before shared confirmations are proven.
- Router rescue logic can expose weak candidates as context but does not explain the exact missing confirmation.
- Position protector was updated to require raw price movement, but the LLM prompt previously still classified near-TP1 only by leveraged Peak PnL.

## Root Causes

### 1. Signals fail at open time because confirmation is not a first-class object

The modules produce reason codes and required confirmation names, but there is no shared evaluator that outputs:

- confirmation passed or failed
- current value
- threshold
- severity: hard block, reviewable wait, or context only
- repairability: can backend adjust price/SL/TP or must wait

Because of this, a signal may be `ready` in provider/local, then become `confirmation_missing` in LLM, or be blocked later by backend geometry.

### 2. Pattern-specific gates are scattered

The same concepts exist in multiple places:

- provider module `Match` and `Score`
- `finalizeV7SignalForExecution`
- router rescue floor
- kernel tier classifier
- trader backend guard
- prompt instructions

This makes it easy to create conflicts: one layer promotes while another demotes.

### 3. Open rate is harmed by binary promotion

Some patterns are too strict at the executable boundary. A candidate that is strong but missing one live check should become `REVIEWABLE`, not disappear into `WATCH` where AI is skipped.

This is especially true for:

- relative-strength leader momentum
- funding reversal near retest/reclaim
- displacement momentum with valid live RR
- accumulation breakout after multi-cycle confirmation

### 4. Micro-profit closes are a shared semantics issue

There are two exit actors:

- mechanical position protector
- LLM position management

Both must use the same definition of “protectable profit”.

Leveraged PnL alone is not enough. At 20x, a 0.3% raw move looks like 6% PnL and can trigger premature TP logic. Small account notional can also turn a partial close into full close due to minimum order size.

## Proposed Code Architecture

### P1: Add structured confirmation evaluation

Add a provider-local confirmation package:

- `provider/local/hunter_v7_confirmation.go`
- `type V7ConfirmationCheck`
- `type V7ConfirmationResult`
- `EvaluateV7Confirmations(sig, ctx, cfg) V7ConfirmationSummary`

Example fields:

```go
type V7ConfirmationCheck struct {
    Code        string
    Passed      bool
    Actual      float64
    Threshold   float64
    Severity    V7ConfirmSeverity // hard_block, review_wait, context
    Reason      string
}

type V7ConfirmationSummary struct {
    PassedHard        bool
    PassedReview      bool
    MissingHard       []V7ConfirmationCheck
    MissingReview     []V7ConfirmationCheck
    EntryZonePosition float64
    StopDistancePct   float64
    RewardPct         float64
    CappedRewardPct   float64
    RR                float64
}
```

This evaluator should understand the existing confirmation codes:

- `live_price_in_entry_zone`
- `5m_close_above_ema20_or_entry_zone_mid`
- `5m_price_holds_ema20_or_trailing_support`
- `taker_buy_15m_gt_0_52`
- `taker_buy_15m_lt_0_48`
- `no_new_low_after_reclaim`
- `no_new_high_after_rejection`
- `momentum_not_exhausted`
- `taker_flow_not_flipping_against_direction`
- `oi_continues_inflow`
- `bb_width_expansion_starts`

Status: implemented as the first global-code step.

Current implementation:

- adds `V7ConfirmationCheck`, `V7ConfirmationSummary`, and `EvaluateV7Confirmations`
- attaches `confirmation_summary` to every `V7SignalOutput`
- carries the summary into `CandidateCoin`
- outputs the summary in `hunter_v7_signal_json`
- stores the summary automatically through existing raw signal JSON
- keeps unknown or candle-pattern-specific confirmations as `context` instead of hard-blocking opens

This is intentionally not a threshold-loosening patch. It gives the router, LLM, and reports one shared explanation of why a candidate is hard-blocked, review-waiting, or only context.

### P2: Replace scattered threshold checks with a setup policy table

Add:

- `provider/local/hunter_v7_setup_policy.go`

Each setup gets a policy:

```go
type V7SetupPolicy struct {
    SetupType              V7SetupType
    MinReviewPriority      float64
    MinExecutablePriority  float64
    MinSetupScoreReview    float64
    MinSetupScoreExec      float64
    MinTimingReview        float64
    MinTimingExec          float64
    MaxRiskReview          float64
    MaxRiskExec            float64
    MinLiquidity           float64
    ConfirmationProfile    string
    CounterTrendStrictness V7CounterTrendStrictness
}
```

The kernel tier classifier should consume this policy and the structured confirmation summary instead of embedding new one-off setup rules.

### P3: Make `ExecutionQuality` data-driven

`finalizeV7SignalForExecution` should become:

1. normalize target and invalidation
2. evaluate structured confirmations
3. compute risk geometry
4. assign quality:
   - `ready`: hard confirmations pass, RR/stop pass, setup policy exec thresholds pass
   - `near_confirm`: only review-level confirmations pass
   - `watch_only`: missing live confirmation or weak timing
   - `chase_risk`: price too extended or flow deteriorating
   - `invalid_rr`: geometry cannot pass even after allowed repair

This prevents module-level `ready` from meaning different things across patterns.

### P4: Preserve open rate through controlled `REVIEWABLE`, not looser `EXECUTABLE`

When no `EXECUTABLE` exists, the system should prefer 1-2 high-quality `REVIEWABLE` candidates instead of skipping AI, but only if:

- risk is not high/extreme
- liquidity is adequate
- RR is feasible after TP cap
- required missing items are review-level, not hard blocks
- no recent same-symbol loss cooldown
- not a late chase in trend-down

This is a global open-rate fix because it increases AI review opportunities without allowing blind opens.

### P5: Add open-block attribution metrics

Extend `hunter_v7_signal_records` or add a JSON field in `raw_json`:

- `confirm_summary`
- `missing_hard_confirmations`
- `missing_review_confirmations`
- `geometry_rr`
- `geometry_stop_pct`
- `geometry_reward_pct`
- `tier_decision_path`

Then dashboards/reports can answer:

- how many candidates were blocked by entry zone
- how many by 5m EMA
- how many by RR
- how many by taker flow
- how many by data quality

This replaces subjective post-hoc review with measurable funnel diagnostics.

## Exit Design

### Unified protectable profit rule

Define:

```go
type ProfitProtectionState struct {
    LeveragedPnLPct float64
    RawMovePct      float64
    PeakLeveredPct  float64
    PeakRawMovePct  float64
    NetAfterFeesPct float64
    Protectable     bool
    Reason          string
}
```

Protectable profit should require:

- leveraged PnL >= TP threshold
- raw move >= raw threshold
- estimated net after fees > minimum net edge
- position age or structure maturity is sufficient, unless hard SL/invalidated

Current immediate fix:

- mechanical protector requires raw move >= 1.0% for TP1 and >= 1.5% for TP2
- LLM prompt now marks near-TP1 only when raw move >= 1.0%

Next improvement:

- track `PeakRawMovePct` in `positionProtectionState`
- include `raw_move`, `peak_raw_move`, and `net_after_fee_estimate` in position prompt
- reject LLM close decisions that cite only peak giveback while `protection_state=pre_tp1`

### Data quality for position management

If live K-lines are invalid or fallback data is used for an open position, the LLM should not close a profitable position solely on weak indicators from that fallback. Add a `data_quality` flag to compact market data:

- `primary_ok`
- `fallback_used`
- `zeroed_klines_rejected`
- `stale_risk`

Backend guard for closes:

- allow close on hard SL/manual risk
- allow close on strong 5m+15m structural reversal with good data
- if data is degraded and position is micro-profit/pre-TP1, prefer `hold`

## Implementation Order

### Phase 1

- Unify LLM prompt protection state with mechanical protector raw-move threshold.
- Add tests for micro-profit Peak PnL where raw move < 1%.
- Add report/query showing micro closes by raw move and net after fees.

Status: implemented for the LLM prompt and prompt tests in this change; mechanical raw-move protection was already added in the preceding fix.

### Phase 2

- Add structured confirmation evaluator.
- Start with 5m/15m EMA, entry zone, taker, RR, stop distance.
- Store summary in raw signal JSON.

Status: implemented for RR, stop distance, entry-zone position, live entry-zone, taker thresholds, entry-zone midpoint proxies, breakout boundary checks, OI/volume inflow, BB expansion context, and momentum exhaustion. Full candle-close semantics remain delegated to live market data/LLM until 5m/15m close fields are available in `V7SymbolContext`.

### Phase 3

- Refactor `finalizeV7SignalForExecution` to consume confirmation summary.
- Refactor kernel tier classifier to use setup policy + confirmation summary.

### Phase 4

- Add backend close guard for degraded data and pre-TP1 micro-profit closes.
- Track peak raw move in protection state.

### Phase 5

- Add dashboard/report aggregation by block reason and confirmation failure.
