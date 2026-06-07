# AiT Session Review - 2026-06-07

## Scope

This session reviewed and fixed live Hunter v7 trading issues observed around cycles #147-#177, plus dashboard/configuration page stutter.

## Findings

### Candidate Pool

- Cycles #147 and #148 were skipped because Hunter v7 produced only `TRXUSDT`, and `TRXUSDT` was listed in the strategy exclusion list.
- The upstream universe was not the full 500+ contract list. The shared snapshot fetcher enriches only the top detailed symbols with OI/LSR/K-line data, and Hunter v7 requires OI to build its universe.
- Per-setup default priority thresholds were being used as candidate visibility filters, which could hide valid context even when the strategy set `v7_min_ai_priority=50`.

### Funding Reversal

- Treating funding reversal `OI building` as a positive score was incorrect for reversal entries.
- The corrected behavior is to downgrade `OI building` and require OI flush or failed rebuild for execution. Deep-drop short chasing and deep-pump long chasing remain filtered.
- Low-priority but non-filtered signals can now be passed as `wait_confirm` context when the primary candidate pool is thin.

### Live Open Failures

Recent failed opens were mainly caused by execution risk geometry:

- `max_take_profit_price_move_pct=3.0`
- Hunter v7 minimum stop distance `2.0%`
- minimum RR `1.5`
- max entry drift `0.5%`

This left no execution buffer. Once TP was capped or price drifted, backend RR often dropped below 1.5.

Observed examples:

- `BEATUSDT`, `TRUMPUSDT`, `BTWUSDT`: backend RR below minimum after TP cap.
- `ICPUSDT`, `JTOUSDT`, `PARTIUSDT`: stop-loss distance below Hunter v7 minimum.
- `BEATUSDT`: entry drift exceeded 0.5%.
- One `BEATUSDT` attempt failed on transient Binance `positionRisk` EOF.

### Dashboard Stutter

- Dashboard, strategy, and config pages were doing too much work at initial navigation.
- Heavy route chunks, modals, chart panels, position history, and Square heat panels loaded eagerly.
- Account/positions/decision polling intervals were also overlapping with chart refreshes.

## Fixes

### Hunter v7 Candidate Visibility

- Candidate filtering now uses global `V7Config.MinAIPriority` for LLM visibility.
- Default minimum output is 3 candidates when enough non-filtered signals exist.
- Backfilled context candidates are marked:
  - `wait_confirm`
  - `candidate_floor_context`
  - `context_only_low_priority`
- `V7StatusFiltered` signals are never backfilled.

### Execution Risk Geometry

- Hunter v7 effective TP cap now auto-raises when configured TP cap conflicts with min stop, RR, and entry drift.
- Current effective cap is 4.0%.
- The live strategy DB was updated to `max_take_profit_price_move_pct=4.0`.
- System prompt now explicitly lists:
  - max entry drift
  - minimum stop distance
  - effective max TP distance
  - feasible open geometry

### Frontend Performance

- Added route chunk preloading after authentication.
- Lazy-loaded dashboard chart tabs, position history, Square heat panel, config modals, and strategy editors.
- Deferred non-critical dashboard panels after trader changes.
- Increased heavy polling intervals to reduce request/render bursts.
- Extracted grid config defaults into a lightweight module to avoid importing the grid editor just for defaults.

## Verification

- `go test ./...`
- `npm run build`
- `npm run test -- --run`
- AIT restarted successfully:
  - Backend: `http://localhost:8080`
  - Frontend: `http://localhost:3000`

## Operational Notes

- AIT service is running after restart.
- The `VVV` trader was left stopped (`is_running=0`); live trading was not restarted automatically.
- `.qoder/` and `ait_server` are local/untracked artifacts and are intentionally excluded from commits.
