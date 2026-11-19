# F08: daily-loss breaker

Record for the redesign plan §6.3 F08 row (Task A remainder, "armed only in pilot/live, records in shadow"; required before any pilot) and OD-15.

**Owner decisions (F08).**
1. **Mode:** `risk_control.daily_loss_breaker.mode` is `off`, `shadow` (default) or `armed`. Shadow records only; armed enforces. Armed is for pilot and live only (Q5).
2. **Measurement:** daily loss is the drawdown from the day-start equity. The day boundary is UTC midnight; the legacy executor used Beijing time. `max_daily_loss_pct` defaults to 15% (Q5).
3. **Trigger action (armed):** block new opens until the next UTC day boundary. Existing positions are never force-closed.
4. **Logging and persistence:** every trip and near-trip evaluation is written to the decision record's execution log. The day-start equity is persisted so a restart does not reset it.

## What existed

`AutoTrader` had `dailyPnL`, `stopUntil`, a 24-hour rolling reset, and the `MaxDailyLoss` / `StopTradingTime` "hint" fields. Nothing ever accumulated `dailyPnL` or set `stopUntil`, so there was no working daily-loss breaker outside the grid trader. Those legacy fields are left as they are. F08 adds its own evaluator and does not reuse the `stopUntil` pause, which would also stop position management.

## Config

`risk_control.daily_loss_breaker` (nil = defaults; read through `store.EffectiveDailyLossBreaker`):

| Key | Default | Meaning |
|---|---|---|
| `mode` | `shadow` | `off`, `shadow` or `armed`. Any other value reads as `shadow`, never as `armed`. |
| `max_daily_loss_pct` | 15 | Trip level: (day-start equity − equity) / day-start equity × 100 |
| `near_trip_fraction` | 0.8 | A loss at or above this fraction of the limit is a near-trip (12% by default) |

## Behaviour

- **Evaluation.** Once per cycle on the cycle's account equity (`ctx.Account.TotalEquity`, which includes unrealized PnL), right after the equity snapshot. It therefore also runs in cycles without candidates.
  - **Baseline:** the first fresh, positive equity of a UTC day becomes that day's baseline.
  - **No baseline change:** stale account data (`AccountDataStale`) and missing equity neither set the baseline nor trip the breaker. They are reported as `stale_equity` / `no_equity`.
- **Trip.** When the loss reaches the limit the day is tripped. The trip latches for the rest of the UTC day, even if equity recovers, and clears at the next UTC midnight.
- **Armed.** While the day is tripped, every new open is rejected with a `daily_loss_breaker` error that names the resume time:
  - LLM `open_long` / `open_short` in the cycle;
  - U9 conditional-entry registration (no pending entry is created);
  - conditional fills, through the open executors.

  Closes, holds, waits and the protector are not touched, and nothing is force-closed. A conditional entry that reaches its trigger during a tripped day is dropped, as for any failed fill.
- **Shadow.** The same evaluation and log lines run, and the open path logs "would block". Nothing is blocked.
- **Off.** Nothing is evaluated, logged or persisted.
- **Execution log.** Each trip, near-trip and new day baseline is written to the cycle's decision record, with mode, status, day, day-start equity, equity, loss %, near level and limit. A tripped day logs one line per cycle.
- **Persistence.** The day's state (day, day-start equity, tripped, trip time, trip loss %) is stored per trader in `system_config` under `daily_loss_breaker:<trader id>`. A restart on the same UTC day keeps the baseline and the trip. A restart on a new UTC day starts a new baseline.
- **Sizing (OD-15).** With the owner's flat 8% risk and 2 positions, two full stop-outs on one day lose 16% and trip the 15% breaker. This is the interaction the plan notes.

## Tests (`trader/daily_loss_breaker_test.go`)

- defaults, and unknown mode → shadow;
- baseline, ok, near-trip at 12%, trip at 15%, latch after recovery, still blocked at 23:59 UTC, unblocked at 00:00 UTC, new baseline;
- the day is the UTC day, also for a clock in UTC+8;
- shadow trips and logs but never blocks;
- off does nothing;
- armed blocks `open_long` / `open_short`, conditional registration and both open executors, but not hold, wait, `close_long` or `close_short`;
- stale or missing equity does not move the baseline or trip;
- persistence across restarts on a SQLite store, per trader, with a new baseline on a new UTC day.

`go build ./...`, `go vet ./...` and `go test ./... -count=1` pass (30 packages).

**Not covered by a test:** the one-line call in `runCycle` that feeds the cycle's equity to the evaluator. The loop needs a full exchange context, so the evaluator, the log lines and the open-path gate are tested directly.
