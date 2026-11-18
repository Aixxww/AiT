# U10: closed-bar features, cycle alignment, snapshot freshness gate

Record for redesign plan §6.2 (three "Data" rows) and the §6.3 U10 row.

**Owner decisions (2026-11-18).**
- **Acceptance:** option 2, a partial parity test on the data that exists. Recording live snapshots (option 1) and a harness-only delivery (option 3) were not approved.
- **Defaults:** all three changes are off by default.
- **OD-21:** enabling cycle alignment stays gated on the parity test.

**What is built** (`hunter.v7_data_timing`):
- **`feature_bars`** (`forming` default | `closed`). `closed` scores a closed-bar view (`V7ClosedBarSnapshot`).
  - A bar is forming when its close time is at or after the time its symbol's data was taken.
  - The view keeps the live price, which both the native context (`CurrentPrice`) and the T2 frames (`K5…K4h.Price`) then use. The forming bar is used for price only.
  - Both stacks are affected.
- **`cycle_alignment`** (`wall_clock` default | `aligned`). `aligned` runs hunter_v7 scans at each `align_interval_minutes` (5) bar close plus `settle_seconds` (30, one collector REST refresh), instead of the ScanInterval ticker. Grid strategies are unaffected.
- **`freshness_budget_seconds[class]`** (empty = gate off). The execution guard rejects an open (`snapshot_age_budget`) when the candidate's data is older than its class budget. Unknown age passes. No budget value is adopted.

**Partial parity test** (`scripts/t2_u0/u10_parity.py`).
- **Sample:** 300 live snapshots (unique timestamp and symbol), drawn with seed 20261118 from the 78,073 records whose change fields are non-zero.
- **Method:** 1m klines from the data.binance.vision archives. τ is searched over T − 0…5 min, the same window for both modes.
- **Units:** |recorded − recomputed| in percentage points.

| Field | Mode | p50 | p90 | Share ≤ 0.05 pp |
|---|---|---|---|---|
| change_1h | **forming** | **0.021** | 0.130 | **69.0%** |
| change_1h | closed | 1.039 | 4.417 | 2.1% |
| change_4h | **forming** | **0.022** | 0.114 | **76.1%** |
| change_4h | closed | 2.291 | 8.165 | 1.7% |

- **Which mode matches live:** the live computation is the forming-bar mode. The remaining error fits minute-resolution reconstruction and a snapshot taken up to a few minutes before the record time: at τ = T alone, the forming p50 is 0.056 and 0.139.
- **Size of the forming-bar effect,** |forming − closed| at τ = T: change_1h p50 1.11 pp, p90 4.54, mean 1.95; change_4h p50 2.43 pp, p90 8.22, mean 4.98.
- **change_24h** (ticker based, independent of the mode, context only): p50 0.025 pp, and 93.7% within 0.2 pp.
- **Limitation:** OI, funding rate and every score have no archive source and are not checked. No data was invented for them.
- **What this test is not:** it confirms that the replay reproduces the live price features in forming mode. It is not a live-vs-replay parity of closed-mode features, which have no live counterpart.
