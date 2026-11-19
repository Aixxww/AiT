# U13: regime-conditional yield target

Record for the redesign plan §6.2 "Yield target" row, OD-20 and the §6.3 U13 row (target set from per-regime fire rates).

**Owner decisions (U13).**
1. **Regime:** the scanner's BTC RV24 tier (spec §6.1): LOW < 0.25, MID < 0.35, HIGH ≥ 0.35.
2. **Data:** BTCUSDT 1h klines for 2026-09-27 to 2026-11-15 from the public data.binance.vision archive, with RV24 recomputed by Kiro. There is no data package from Muse.
3. **Placement:** the targets replace the fixed G3 of `t2_m2_eval`. A read-only per-regime yield table is added to the U12 cells endpoint and panel.
4. **Target:** measure first with the fixed method below. The target is the 90% Poisson lower bound of the measured rate.

The owner accepted the precision limit: G3 counts 9 events (19 at RR 1.2), about 2 to 8 per regime.

## Method (fixed before measuring)

- **Klines:** the futures BTCUSDT archive (the scanner reads `fapi`). Files are monthly for 2026-09 and 2026-10 and daily for 2026-11-01 to 2026-11-15, in 1h, 5m and 1m. All 51 zips match their published SHA-256 `.CHECKSUM` files. They live in the sandbox and are never committed.
- **RV24 at time t** follows the scanner rule `btc_trailing_rv24`. Its 30 klines end with the forming 1h bar, so the series is:
  - the 29 latest closed 1h bars;
  - plus the forming bar, priced at the close of the latest closed 5m bar.

  RV24 is the population std of the last 24 log returns × 100. Fewer than 25 bars means no tier, as the scanner fails open. The method uses 5m for the forming-bar price because the live dashboard has to fetch the same series; 1m is reported as a sensitivity check.
- **Check:** recompute RV24 at `payload.ts` for the 284 snapshots that carry the scanner's own `regime_rv24`.
- **Rounds:** the 13,824 five-minute rounds of 2026-09-28 to 2026-11-15 UTC (48 × 288). Each round is labelled with its tier at its start.
- **Events and stages:** each event is labelled with its tier at its trigger time. The stages are:
  - all T2 triggered events (694);
  - `t2_m2_eval` structure-admitted events at RR 1.2 (19);
  - `t2_m2_eval` structure-admitted events at RR 1.5 (9).
- **Rate and target:** the rate per tier is events / rounds, with the exact two-sided 90% Poisson (Garwood) interval. The target is the lower bound.
- **Reproduce:** `go run ./cmd/t2_u13_yield -btc-dir <zips> -data <entry_study> -snapshots <snapshots.jsonl> -m2 <m2 rr1.2 json>,<m2 rr1.5 json> -out u13.json`

## Results

**RV24 check (284 snapshots with a recorded value):**

| Forming-bar price | Tier agreement | \|diff\| p50 | p90 | max | Within 0.005 |
|---|---|---|---|---|---|
| 5m (method) | 283 / 284 | 0.001 | 0.006 | 0.076 | 242 |
| 1m (sensitivity) | 284 / 284 | 0.000 | 0.002 | 0.028 | 274 |

- **The one disagreement:** 2026-11-03T01:52+08:00, recorded 0.249 (LOW) against 0.258 (MID). It sits 0.001 under the LOW/MID edge.
- **Round labels:** the two forming-bar prices give the same tier on all 13,824 rounds. At a 5-minute boundary the latest 5m and 1m closes are the same price.
- **Event labels:** 3 of the 694 event labels (taken at trigger time) differ between 5m and 1m.

**Rounds per tier:** LOW 3,814, MID 4,678, HIGH 5,332. No round lacks a tier.

**Rates and targets** (events per 5-minute round; the target is the 90% lower bound):

| Stage | Tier | Events | Rate | 90% CI | Target |
|---|---|---|---|---|---|
| t2_triggered | LOW | 177 | 0.046408 | [0.040823, 0.052569] | 0.040823 |
| t2_triggered | MID | 243 | 0.051945 | [0.046588, 0.057770] | 0.046588 |
| t2_triggered | HIGH | 274 | 0.051388 | [0.046390, 0.056795] | 0.046390 |
| m2v2_admitted_rr1.20 | LOW | 8 | 0.002098 | [0.001044, 0.003785] | 0.001044 |
| m2v2_admitted_rr1.20 | MID | 7 | 0.001496 | [0.000702, 0.002811] | 0.000702 |
| m2v2_admitted_rr1.20 | HIGH | 4 | 0.000750 | [0.000256, 0.001717] | 0.000256 |
| m2v2_admitted_rr1.50 | LOW | 4 | 0.001049 | [0.000358, 0.002400] | 0.000358 |
| m2v2_admitted_rr1.50 | MID | 3 | 0.000641 | [0.000175, 0.001657] | 0.000175 |
| m2v2_admitted_rr1.50 | HIGH | 2 | 0.000375 | [0.000067, 0.001181] | 0.000067 |

These values are the config defaults (`store/hunter_v7_yield_defaults.go`, full precision). A test recomputes the LOW triggered default from 177 / 3,814 and the two G3 thresholds below.

- **Triggered events:** the rate is about the same in the three tiers (14 to 15 per day).
- **Admitted events:** admitted counts fall from LOW to HIGH, but the intervals overlap heavily.

## G3 in `t2_m2_eval`

With `-btc-dir`, G3 is "admitted ≥ Σ over tiers of rounds × target" for the stage of the run's `-rr-min`. The RV24 constants and targets come from `-yield-config` (a `v7_yield_target` JSON block), else the defaults. The report adds:
- `gates.g3_mode = "regime_target"`;
- `gates.g3_regime_target`, with rounds, targets, admitted per tier and the threshold;
- `gates.g3_admitted_ge_regime_target`.

The legacy `g3_admitted_ge_170` is still reported for reference. Without `-btc-dir` the JSON output is byte-identical to before; this was checked at RR 1.2 and 1.5 against the base `ec9f3fc`. Stdout differs only in the order of the reject-detail lines, which comes from map iteration and predates U13.

| Run | Admitted | Regime G3 threshold | G3 | G1 P(TP1 first) > 40% | G2 stop→TP1 < 35% | Verdict |
|---|---|---|---|---|---|---|
| rr-min 1.2 | 19 | 8.632 | pass | 31.6%, fail | 33.3%, pass | **FAIL** (was UNRESOLVED) |
| rr-min 1.5 | 9 | 2.539 | pass | 22.2%, fail | 33.3%, pass | **FAIL** (was UNRESOLVED) |

**This changes the M2v2 verdict.** Under the fixed 170, G3 was unreachable and the verdict stayed UNRESOLVED. Under the regime target G3 passes, so G1 and G2 are judged, and G1 fails at both floors.

The target was set from the same 48 days, so passing G3 on this data is close to built in: each threshold is a sum of lower bounds below the observed counts. On these data the gate is a pass by construction. It becomes a real check only on a later window.

## Live yield table (cells endpoint and panel)

`GET /api/hunter/v7/cells` now also returns `yield` for the selected window, read-only.
- **Rounds:** the window's 5-minute rounds are labelled with the same RV24 series. BTCUSDT futures 1h and 5m klines come through `market.GetKlinesRange` (fapi, then the archive), cached for 5 minutes.
- **Events:** T2 records at lifecycle TRIGGERED, deduplicated into events by symbol, canonical setup, direction and trigger price. The event time is the first record's time.
- **Rows:** each tier reports rounds, events, rate and 90% interval, plus the `t2_triggered` target and the expected count. `below_target` is set when the interval's upper bound lies under the target.
- **When unavailable:** the endpoint reports `available: false` with a reason when the window has no T2 records (T2 module off) or the kline fetch fails. The rest of the endpoint is unaffected.
- **Panel:** `CellsPanel` shows the table in events per day, with the tier rule, the interval level and the below-target flag. Strings are in EN/ZH/ID.

Config: `hunter.v7_yield_target` holds the RV24 constants, round length, CI level and `targets` (stage → tier → rate). Like the U12 rule, the endpoint reads it from the system default strategy, else the defaults.

**What the live comparison is.** The live stage is the T2 module's TRIGGERED events after dedup. The targets come from the scanner's 694 triggered events. The two counts are close in definition but not identical, so a below-target flag is a prompt to investigate, not a verdict. The admitted-stage targets have no live counterpart, because `geometry_mode` stays legacy. They are used only by `t2_m2_eval`.

## Tests

- **Store:**
  - exact Poisson intervals against tabulated values;
  - RV24 from closes, the forming-bar series, tier edges and round counts;
  - defaults, which reproduce the measured lower bound and the G3 thresholds;
  - overrides, the below-target flag and event dedup.
- **API:** the cells endpoint's yield table on synthetic BTC series (a LOW period, then HIGH) with T2 records: counts per tier, dedup, CI, below-target. Plus the unavailable paths (no T2 records, fetch failure), with no network.
- **Web:** the vitest panel tests cover the yield table, the below-target flag, the unavailable state and the per-day conversion. `vitest run` gives 16 files and 193 tests.
- **Go:** `go build ./...`, `go vet ./...` and `go test ./... -count=1` pass (30 packages).
- **Web build:** with the pre-existing gitignored `web/src/data/faqData.ts` stubbed (as in U12; not committed), `tsc` reports 0 errors and `vite build` succeeds.
