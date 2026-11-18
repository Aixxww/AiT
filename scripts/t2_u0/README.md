# U0 evidence scripts (redesign plan U0)

These scripts reproduce every U0 table in `docs/t2-redesign-plan-20261118.md` §10.

## Data

- All data lives in the private repository `Aixxww/ait-t2-data`. It is never committed here.
- Set `U0_DATA` to its checkout. Before running, concatenate the split files:
  - `m3_snapshots/snapshots.p00..p07.jsonl` into `snapshots.jsonl`;
  - `live_db/decision_records.p*.jsonl` into `decision_records.jsonl`;
  - `live_db/hunter_v7_signal_records_trimmed.p*.jsonl` into `hunter_v7_signal_records_trimmed.jsonl`.
- Check every md5 against the manifests in that repository first.
- Run everything from one empty working directory, with `PYTHONPATH` pointing at this folder. The two live-DB JSONL files go in the working directory.

## Steps

| Step | Command (from the repo root unless noted) | Output |
|---|---|---|
| Structure eval + MEAS1 rows | `go run ./cmd/t2_m2_eval -data $U0_DATA/entry_study -lookback $U0_DATA/lookback -rr-min 1.5 -out m2v2_rr15.json -rr-stage-out rrstage.json` | `m2v2_rr15.json`, `rrstage.json` |
| Replay (model D, the 380 fills) | `go run ./cmd/t2_replay -data $U0_DATA/entry_study -out replay.json` | `replay.json` |
| T2 tiering + native stack | `go run ./cmd/t2_u0_native -data $U0_DATA/m3_snapshots/snapshots.jsonl -out u0b_rows.json -lsr top -taker raw` | `u0b_rows.json` |
| Live protector A (U0-F) | `U0F_RUN=1 U0F_DATA=$U0_DATA/entry_study U0F_OUT=u0f_rows.json go test ./trader -run 'TestU0F$'` | `u0f_rows.json` |
| Check 6 | `meas_check.py` | MEAS1 bit-identical; MEAS2 fields identical except the tier reason that U2 changes |
| U0-F | `u0f_tables.py`, `u0f_variants.py` | K/S/N groups, A − D, folds, variants E1–E6 |
| U0-A | `u0a_t2_calibration.py` | T2 score bands |
| U0-B | `u0b_native.py` | native yield, co-fire, calibration |
| U0-C | `u0c_delay.py` | delay cost and run-away |
| U0-D | `u0d_stop_floor.py` | early stop-out by stop / ATR |
| U0-E | `u0e_twins.py` | twin-pair arms |
| U0-F2 | `u0f2_cuts.py`, `u0f2_variants.py`, `u0f2_v2_robustness.py` | screened cuts, V1–V3 |
| U0-H | `u0h_attribution.py` | live exit attribution, writes `u0h_rows.json` |
| U0-H2 inputs | `u0h2_inputs.py` | `u0h2_trades.json`, using data.binance.vision archives cached in `./kl` |
| U5 acceptance | `go run ./cmd/t2_u5_tags -snapshots $U0_DATA/m3_snapshots/snapshots.jsonl -data $U0_DATA/entry_study -horizon-hours 48 -out u5_48h.json` (and the default 8h horizon), then `u5_check.py u5_48h.json u5_8h.json` | tags on every record; tracker fee-net R vs the U0-A walk |

## The single-run record

U0-H2 has been run exactly once. `u0h2_run.py` and `trader/u0_protector_live_harness_test.go` are kept as the frozen record and must not be re-run for a decision. See `docs/t2-u0-preregistrations.md`.

## Tests

- `python3 -m unittest test_walks`: synthetic paths, plus a check that the walk functions match the frozen run byte-for-byte.
- `go test ./trader -run TestU0Walk`: synthetic paths for model A.
