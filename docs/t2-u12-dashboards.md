# U12: per class × cell × regime dashboards

Record for the redesign plan §6.2 "Observability" row and the §6.3 U12 row (acceptance: per-class tables match U5 records).

**Owner decisions (all option a).**
1. **Delivery:** a new `GET /api/hunter/v7/cells` endpoint plus a dashboard panel next to `SignalPanel`, with EN/ZH/ID strings and vitest tests.
2. **Cell status:** the §5.3 rule is evaluated on the selected window and shown next to the stamped tag, labelled "no hysteresis". N_min and the CI level are new config keys, defaulting to 30 and 90%.
3. **Outcomes counted:** `prompt_open_review` by default, with the outcome source as a filter (all, watch_audit, shadow).
4. **Calibration drift:** a reliability table per `calibration_version` over the current window and the previous window of the same length. Drift is flagged when both windows have quantiles outside their CI.

The dashboard is read-only. Nothing in U12 feeds admission, tiering or sizing.

## Config

`hunter.v7_cell_rule` (nil = defaults; read through `store.EffectiveV7CellRule`):

| Key | Default | Meaning |
|---|---|---|
| `min_samples` | 30 | N_min of §5.3 (OD-8) |
| `ci_level` | 0.90 | Two-sided bootstrap CI level of mean fee-net R. Out-of-range values read as the default. |
| `bootstrap_resamples` | 5000 | As in the U7 protocol |
| `seed` | 20261118 | As in the U7 protocol |
| `quantiles` | 5 | Number of expected_r groups in the drift table |

Signal records carry no strategy id, so the endpoint reads one config: the system default strategy's `hunter.v7_cell_rule`, else the defaults. The effective values are returned in the response (`rule`).

## Endpoint

`GET /api/hunter/v7/cells?days=7&source=prompt_open_review`.
- **`days`:** default 7, capped at 180.
- **`source`:** one of `prompt_open_review` (default), `all`, `watch_audit` or `shadow`. Any other value returns 400.

**Rows that count:**
- the record's timestamp (signal time) falls inside the window;
- it has a resolved `fee_net_outcome`;
- it has a `fee_net_r`;
- it passes the source filter.

Resolved rows without a fee-net R (degenerate geometry, no anchor) are reported by outcome in `excluded_no_r` and are not in the tables.

**Tables:** `tables.classes`, `tables.cells` (class × RR band) and `tables.cell_regimes` (cell × `market_regime`). Empty tags show as `UNASSIGNED` / `unknown`. Each row reports:
- n, with TP1 / stop / none counts;
- the TP1-first rate;
- mean fee-net R and its bootstrap CI;
- `stamped_status`: the distinct stamped `cell_status` values;
- `evaluated_status`: EARNED when n ≥ N_min and the CI lower bound is > 0, BLOCKED when n ≥ N_min and the upper bound is < 0, PROBATION otherwise.

The response carries `hysteresis: "none"`. Each row's bootstrap uses a generator seeded with `seed`, so a row's CI does not depend on the other rows.

**Calibration drift:** one entry per `calibration_version`, built from records that carry both `expected_r` and `fee_net_r` (same source filter).
- **Windows:** the current window is [from, to]; the previous one is [from − days, from).
- **Quantiles:** records are sorted by expected_r (then time and id) and split into `quantiles` groups. Each group reports the predicted mean, the realized mean with its bootstrap CI, and `inside`.
- **Sufficiency:** a window with fewer rows than `quantiles` is marked insufficient.
- **Flag:** `drift` is true when both windows are sufficient and each has at least one quantile outside its CI.
- **Today:** `expected_r` is null on every record, because the shadow estimator is off. The table is therefore empty, and the panel says so.

**Shared bootstrap.** The bootstrap is `store.V7BootstrapMeanCI`, moved out of `cmd/t2_u7_falsify`. The U7 tool now calls it, and its output on the U5 48h rows is byte-identical to before.

## Panel

`web/src/components/hunter/CellsPanel.tsx` is mounted on `TraderDashboardPage` below `SignalPanel`.
- **Controls:** a window select (7 / 30 / 90 days), a source select and refresh.
- **Tables:** classes and cells open by default; the cell × regime table starts collapsed. Each row shows the stamped and the evaluated status.
- **Notes:** a rule note gives N_min, the CI level and "no hysteresis". The excluded-row line follows, then the drift section.
- **Strings:** EN/ZH/ID strings live under `v7Cells.*`.

## Verification

- **Acceptance** (`api/handler_hunter_cells_test.go`, `TestU12CellsMatchU5Records`):
  - **Fixture:** 402 U5-tagged records in in-memory sqlite. They cover 4 classes, 5 bands, 3 regimes and all sources, plus rows without R, unresolved rows, stale rows, and expected_r in both windows.
  - **Check:** through the handler, the per-class, per-cell and per cell × regime tables equal a direct group-by over the records, for every source filter. The compared values are n, the TP1/stop/none counts, mean R and the TP1 rate.
  - **Also checked:** the evaluated status follows the rule, and the drift current-window n matches.
- **U5 48h replay** (`cmd/t2_u12_cells` + an independent Python group-by):
  - all 694 events load;
  - the tables match on every level, for all four filters;
  - the `all` filter gives 615 rows, the U5 fee-net count;
  - `market_regime` is empty in this replay, so every row shows `unknown`.

  Cells at `prompt_open_review`:

  | Cell | n | TP1 first | Mean R | 90% CI | Evaluated |
  |---|---|---|---|---|---|
  | IMPULSE 1.2_1.5 | 57 | 54.4% | −0.106 | [−0.297, +0.086] | probation |
  | IMPULSE 1.5_2.0 | 50 | 46.0% | −0.105 | [−0.352, +0.144] | probation |
  | REVERSAL lt_1.0 | 1 | 100% | +1.370 | [+1.370, +1.370] | probation |
  | TRIGGER 1.2_1.5 | 114 | 54.4% | −0.097 | [−0.236, +0.043] | probation |
  | TRIGGER 1.5_2.0 | 5 | 80.0% | +0.422 | [−0.242, +1.087] | probation |

  50 resolved rows (38 degenerate_tp1, 12 degenerate_stop) have no R and are excluded. No cell is EARNED or BLOCKED.
- **Unit tests:** store tests cover the defaults, the bootstrap against the U7 inline computation (draw for draw), the status rule, the tables (filters, order, exclusions, CI independence) and drift. vitest covers the panel: tables, status badges, filters refetching, drift flag, and the empty and failed states.
- **Builds:**
  - Go: `go build ./...`, `go vet ./...` and `go test ./... -count=1` pass (30 packages).
  - Web: `vitest run` passes (16 files, 190 tests), and eslint and prettier are clean on the changed files.
  - **Pre-existing build issue:** `tsc` and `vite build` fail on a clean checkout, because `web/src/data/faqData.ts` is matched by the `.gitignore` rule `data/`, which predates U12. With a temporary stub for that file, `tsc` reports 0 errors and `vite build` succeeds. The stub is not committed.
