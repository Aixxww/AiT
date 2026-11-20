# Tier A live validation: re-audit and improvement plan (2026-11-20)

Status: **plan only**. No code is changed by this document. Each unit below ships later as its own branch and PR, after owner review. The 144-round rerun is **not authorized**. It needs P4 fixed (TA-1) plus owner approval.

Inputs re-audited:
- `ANALYSIS.md` (the close-out of the 2026-11-19/20 Tier A run);
- `run-log.txt`, `logs/tunnel144.log`, and the `logs/run144.log` death excerpt.

Every code reference below is to `t2/v7-selection` at `db1cc60`.

## 0. Provenance: the binary that ran is not the code in the repo

`run-log.txt` says the harness was built from branch `t2/live-test-tier-a` at `db1cc60`, **plus an uncommitted 61-line diff**. The diff added a `--binance-url` flag, a "T2 wiring fix" and an `mk-test-strategy` helper. That branch does not exist on `origin`.

At `db1cc60`, `cmd/hunter_v7_validate` never enables the Task-2 module:
- `runValidation` builds `cfg := local.DefaultV7Config()` and never sets `cfg.T2`, `cfg.T2Store` or `cfg.T2Regime` (`cmd/hunter_v7_validate/main.go:398-422`);
- its fetcher config does not set `BinanceURL` or `IncludeT2Inputs` (`main.go:378-383`).

So everything T2-related in the evidence comes from code nobody has reviewed. Several verdicts below depend on how that diff does three things:
- creates and isolates the `V7T2EventStore`;
- points the backfill fetcher at the relay;
- enables `IncludeT2Inputs`.

**TA-0 lands that diff first.**

## 1. Audit verdicts, P1–P8

### P1: relay over-counted weight → 429s, and the `_reject()` crash. **Agree (outside the repo).**
- `relay.py` is not in this repo, so I can't check the fix in code. The evidence is internally consistent: 0×429 across 57,161 requests, and a peak of 682 weight/min.
- A related client-side fact: the harness has its own token bucket of **1,200 requests/min**, counting one token per request whatever its weight (`datafetch/http_client.go:177`). The relay's 2,000 weight/min cap and this limiter are two different budgets.

### P2: the first contract failure ("legacy module signals miss 3 fields"). **Partially agree.**
- **Root cause, agree.** T2 module signals never carry the context fields. `(*V7T2Signal).ToV7SignalOutput` (`provider/local/hunter_v7_t2_setups.go:1721-1765`) sets no `PriceCtx`, no `DerivativesCtx` and no `RequiredConfirms`. `RunV7T2Module` (`provider/local/hunter_v7_t2_adapter.go:194-262`) doesn't add them either.
- **Correction 1: "legacy".** INJUSDT and NAORISUSDT are T2 module signals (`Stack: "t2"`, `hunter_v7_t2_setups.go:1726`), not a legacy path.
- **Correction 2: "3 missing fields".** Only **2** of the three are counted in `missing_fields`:
  - `price_context` and `derivatives_context` are `omitempty` pointers (`provider/local/hunter_v7_types.go:449-450`). When nil, the JSON key is absent, and `validateFormat` counts a missing field (`main.go:1185-1189`).
  - `required_confirmations` has **no** `omitempty` (`hunter_v7_types.go:442`). A nil slice marshals as `null`, so the key is present and not counted as missing.
  - It counts only as an **executable gap**, and only for signals whose entry is not immediate (`main.go:1200-1203`, `needsConfirmations` `main.go:1232-1241`).
  - That matches the run-log's "missing_fields=4, executable_gaps=6" for 2 WATCH signals: 2×2 context gaps plus 2 confirmation gaps.
- **Correction 3: "data-dependent, not structural" (run-log) is wrong.** The gap is structural: every T2 output lacks the fields. Only whether the module fires is data-dependent.

### P3: attempt 2's process and tunnel vanished. **Agree it is unproven; nothing in the repo contradicts the analysis's leading hypothesis (a sandbox/VM event).**
- The harness exits non-zero on a round that fails 3 attempts: `log.Fatalf` at **`main.go:344`**. The pack cites `main.go:352`; that line number comes from the patched binary. At `db1cc60`, lines 352-353 are the run-summary `Fatalf`.
- A Fatalf prints its line before exiting. The log stopped *after* "retrying in 5s" and before any further line, so the process was killed while sleeping (`main.go:340`). That is consistent with an external kill, not a harness exit.
- The supervisor and tunnel scripts are not in the repo.

### P4: the deterministic contract failure that ended the run. **Agree on the cause. Partially agree on the derived claims.**
- **Cause, agree.** `missing_fields = 2 × T2 signals` follows from the code above.
  - T2 outputs are appended after the router's output cap (`provider/local/hunter_v7.go:65-73`). Freshly-TRIGGERED events are re-emitted through the same converter (`hunter_v7_t2_adapter.go:243-254`). So every emitted T2 signal adds exactly 2.
  - `reportAcceptanceError` checks `MissingFieldCount > 0` first (`main.go:1447-1450`). The error message therefore never shows the executable-gap count.
  - So the 20/20 and 13/13 split is expected. A round passes only when the module emits nothing.
- **"Also missing required_confirmations": partially agree.** Only WATCH and observe-only T2 outputs (`wait_confirm`) need it. ARMED and non-observe-only TRIGGERED outputs are `entry_mode: immediate` (`hunter_v7_t2_setups.go:1745-1749`, `1778-1784`). For those, `needsConfirmations` returns false.
- **① "Supervisor: no backoff, no skip, no circuit breaker": agree.** The harness itself has no skip either. After 3 attempts (5 s / 10 s backoff, `main.go:337-341`) it calls Fatalf, and it always restarts at round 1 (`main.go:320`).
  - **Additional finding:** every restart reuses cycle numbers from 1. Signal ids are `cycle|symbol|direction|setup` (`provider/local/hunter_v7.go:137-138`), so restarts produce colliding ids and duplicate cycle-1 rows. That is the source of the 77 duplicate keys the analysis removed.
- **② "Each restart resets T2 lifecycle": agree.** `V7T2EventStore` is an in-memory map (`provider/local/hunter_v7_t2_lifecycle.go:87-110`).
  - The native watch state is lost too: `V7SignalStateManager` (`main.go:259`). So is the module dry-run breaker it carries (`provider/local/hunter_v7_module_breaker.go:44-90`).
- **③ "Failed attempts' raw JSON is written to reports144/": agree.**
  - The prompt is written at `main.go:451`, raw JSON at `main.go:490` and markdown at `main.go:493`.
  - All three happen before `reportAcceptanceError` at `main.go:496`.
- **New, unverifiable without the diff: attempt isolation.** The harness clones the native watch state per attempt and commits it only on success (`main.go:327-335`). `V7T2EventStore` has no Clone. If the diff passes one shared store, failed attempts still `Upsert` events, start the 60-minute cooldown and run `UpdateOutcomes` (`hunter_v7_t2_adapter.go:213`, `234`). That breaks the harness rule "a rejected attempt must not count as another watch cycle" (`main.go:328-329`).

### P4, behaviour claims in §3 of the analysis
1. **"WATCH → TRIGGERED in about 2 attempts": consistent with the code.** `UpdateOutcomes` marks any active event TRIGGERED once price reaches the entry (`hunter_v7_t2_lifecycle.go:280-295`). This counts attempts, not accepted cycles.
2. **"Restart resets to WATCH": agree** (see ②).
3. **"Flips between WATCH and TRIGGERED, so the state is recomputed every round with no memory": disagree with the stated mechanism.** The store is one-way per event:
   - once `TriggeredAt` is set it is never cleared;
   - a score upgrade keeps TRIGGERED (`hunter_v7_t2_lifecycle.go:145`);
   - a plain re-hit leaves the status unchanged (`:152-155`);
   - `UpdateOutcomes` only moves an event to TRIGGERED, INVALIDATED or EXPIRED;
   - a new event_id for the same symbol and direction is dropped for 60 minutes (`:166`).

   Within one process and one store, a TRIGGERED event therefore cannot be re-emitted as WATCH within 60 minutes. Two explanations fit the code:
   - (i) the flips span supervisor restarts, which create a fresh store;
   - (ii) the diff clones the store per attempt and discards it on failure, so every attempt of a failing round restarts from the pre-round state.

   Check: group the 73 T2 rows by harness start (`run144.log` restart markers) and by round/attempt.
4. **"TRIGGERED does not pass the ARMED score gate": agree, by design** (OD-4, `hunter_v7_t2_lifecycle.go:21-29`).
5. **"The failed-attempt prompt check fails because module signals lack fields" (§3, last line): not supported by the code.**
   - `validatePrompt` (`main.go:1244`) only searches the prompt text.
   - `buildPrompt` adds a market-data entry for every signal, with or without `PriceCtx` (`main.go:1103-1138`).
   - The prompt check only gates acceptance when there are open-review candidates (`main.go:1454-1462`).
   - **Related defect found:** for a T2 signal with nil `DerivativesCtx`, `buildPrompt` overwrites the real funding rate with 0 (`main.go:1115-1126`). TA-1 removes the cause.

### P5: 1,205 relay `UnicodeEncodeError`s. **Agree the mechanism is unproven. One citation is wrong.**
- **Wrong citation.** The analysis says the harness absorbs these through `fetchJSONParallel` / `isRetryableNetError`. Those are in `provider/local/ai500.go:484` / `:534`, which is not the harness path.
- **What the harness actually uses:** `datafetch` `doPublic` (`datafetch/http_client.go:194-246`) with `isRetryable` (`:282-296`). It makes up to 3 retries with 600 / 1,200 / 2,400 ms sleeps, and EOF, reset and broken pipe are retryable. The conclusion (absorbed by retries) still holds.
- **The harness can't see these errors.** Per-symbol errors are counted and thrown away (`datafetch/fetcher_per_symbol.go:77-81`). No URL is ever logged.
- **Unknown routing.** At `db1cc60` the tracker's backfill fetcher has no `BinanceURL`, so it defaults to `fapi.binance.com` (`main.go:304-309`). Whether backfill traffic went through the relay depends on the diff. That matters for the "errors grow with backfill" correlation.
- No root cause is asserted here; §5 is the capture plan.

### P6: "fetch 183–214 s exceeds the 180 s interval and the 3-minute context". **Partially agree. The measured number is not fetch time, and the deadline works differently from what the analysis assumes.**
- **`fetch_ms` is round time.** It is `time.Since(start)` taken after strategy load, fetch, scoring, assembly and prompt build (`main.go:364`, `461`). The real fetch time is `snap.Meta.FetchDuration` (`datafetch/fetcher.go:155`), which is not reported.
- **What the 3-minute context covers.** It starts at `runValidation` entry (`main.go:366`). It is checked **only** before a per-symbol worker starts (`fetcher_per_symbol.go:74`). HTTP requests ignore it (`c.inner.Get(url)`, `http_client.go:208`) and use the 15 s client timeout (`main.go:381` → `fetcher.go:59`). Scoring and prompt build ignore it too.
- **What happens at the deadline.** Detail symbols not yet started are **silently skipped**: not counted in `RestErrors`, not logged, left with bulk-only data. The round never fails on the deadline. That is why "rounds still completed".
- **The 180 s interval is a sleep after each round** (`main.go:347-350`), not a deadline. Cadence is round time + 180 s, which matches the measured 6:05–6:35.
- Details are in §6.

### P7: the tunnel loop vanished after the harness died. **Agree (outside the repo).** Same signature as P3. The scripts are not in the repo; TA-4 brings them in.

### P8: `fee_net_outcome` empty on all 2,045 rows. **Agree, with the root cause confirmed in code: a harness wiring gap.**
- The harness outcome callback (`main.go:285-302`) passes no `FeeNetOutcome`, `FeeNetR`, `FeeNetEntry` or `DriftAtEntryPct`.
- The live path does pass them (`trader/auto_trader.go:536-539`).
- `UpdateTrackOutcome` writes the fee-net columns only when `FeeNetOutcome != ""` (`store/hunter_v7_signal.go:225`).
- So even outcomes the tracker computed were never stored.
- The harness tracker also never sets `FeeNetCostPct` from the strategy's outcome tags. It falls back to the default (`trader/hunter_v7_pnl_tracker.go:273`); live sets it at `trader/auto_trader.go:486`.
- Tracking only EXECUTABLE/REVIEWABLE signals is the documented flag behaviour (`main.go:235`), not a bug.

### Other corrections to the numbers
- **`rest_errors` counts symbols, not requests.** It is the number of detail symbols with ≥1 failed sub-request (`fetcher_per_symbol.go:218-227`).
  - The rate's denominator is all 515 symbols (`main.go:1422`), but only detail symbols make per-symbol calls.
  - "≤0.39%" is therefore a per-symbol figure over the wrong base. Over roughly 220 detail symbols it is about 0.9%. Still low.
- **The "180 s stale threshold" isn't applied in the harness.** The harness has no price-age gate. The live freshness guard is `MaxSnapshotAge` (`kernel/engine_snapshot.go:274-279`), and the harness doesn't use it.
- **`ROUTE_EVALUATION` is not in the repo.** Its 100–120 s fetch assumption can't be checked.

## 2. P4 decision analysis

The contract's purpose (`main.go:1442-1444`) is to block persistence of signals whose data is broken. A signal must carry the context the AI decision layer needs: price, ATR and changes; OI, funding, LSR and taker; and what to wait for. The design plan's U5 acceptance says the same for every candidate.

### (a) Fill in the three fields on T2 outputs
- **What changes:** T2 signals carry the same descriptive context as native signals, computed from the same snapshot:
  - `price_context` and `derivatives_context` come from `buildSymbolContext` → `buildPriceCtx` / `buildDerivCtx` (`provider/local/hunter_v7_universe.go:274`, `hunter_v7_mod_pullback.go:162-187`).
  - `required_confirmations` goes only on non-immediate outputs.
- **Lifecycle semantics:** score, status, entry/stop/targets, event ids and the state machine are unchanged. The two context fields are read-only additions.
- **The one semantic effect is `required_confirmations`.** The kernel reads it:
  - `hunterV7RequiredConfirmationWaitReason` returns `confirmation_missing_summary` when a candidate has codes but no confirmation summary (`kernel/engine.go:920-926`). T2 signals bypass `EvaluateV7Confirmations`, so they have no summary.
  - T2 outputs that need confirmations are `wait_confirm`. Under the U4/OD-11 T2 tier rule those land at WATCH.
  - The expected tier is therefore unchanged, but this must be **proved by a golden comparison** (TA-1 acceptance). If any tier changes, the unit stops and goes back to the owner.
- **The contract's purpose is kept**, and it now holds for the stack Tier A is meant to validate.
- **It also fixes a real gap outside the harness:** the live prompt receives T2 candidates without price or derivatives context (the same converter), and the harness prompt shows funding 0 for them.

### (b) Relax the contract for module signals
- **Lifecycle semantics:** unchanged.
- **The contract's purpose is defeated for exactly the stack under test.** T2 rows would reach the DB, and T2 candidates the prompt, without context. The defect stays in the live path.
- It also creates two acceptance standards inside one signal list.

### (c) Supervisor skip-on-failure plus a deterministic-failure breaker
- **On its own it doesn't fix P4.** Every round in which T2 fires would be skipped. The accepted data set would then be "rounds without T2" **by construction**: zero T2 evidence and a selection bias against the very event under test. After 23:31 every round had T2 signals, so it would either skip everything or trip the breaker.
- **Lifecycle semantics:** if skipped rounds don't advance state (the harness rule), T2 events never progress in accepted data. If they do advance, accepted data contains progress from unaccepted rounds.
- **It is still needed as robustness** against transient and unknown failures (TA-2).

### Recommendation: **(a)**, delivered as TA-1. Ship (c)'s robustness separately as TA-2, together with persistence (TA-3).
(a) is the only option that leaves T2 evidence in the accepted data and keeps the contract meaningful. It also removes a defect that would reach the live prompt once T2 is enabled. Do not adopt (b).

## 3. Implementation units

Order and rerun gate:
- **Before any rerun:** TA-0 → TA-1 → TA-2 → TA-3 → TA-4.
- **Recommended before a rerun (cheap):** TA-5, TA-6, TA-7.
- The rerun itself is a separate owner decision.

Each unit runs `go build ./... && go vet ./... && go test ./... -count=1` with `GOTOOLCHAIN=go1.25.3+auto`, plus the unit tests named below. No unit touches live trading, `geometry_mode`, the live-test strategy or `data/test.db`.

### TA-0: Land the Tier A harness diff
- **Problem:** the evidence came from an unreviewed 61-line diff (§0).
- **Change:** the owner or tester provides the diff. It is committed as-is to its own branch and reviewed:
  - `--binance-url`;
  - T2 wiring (`cfg.T2`, `cfg.T2Store`, `IncludeT2Inputs`);
  - `mk-test-strategy`.
- **Review questions:**
  - Where is the `V7T2EventStore` created, and is it shared across attempts?
  - Does the backfill fetcher get `--binance-url`?
  - Is `T2Regime` set?
- **Also:** every report gets `harness_git_sha` and `harness_dirty` (from `debug.ReadBuildInfo` VCS settings).
- **Acceptance:**
  - the reviewed diff, with no behaviour change beyond the description;
  - a test that `--binance-url` reaches both fetchers;
  - reports carry the build info.
- **Rollback:** revert the PR.

### TA-1: T2 outputs satisfy the signal contract (P4, option a)
- **Root cause:** `ToV7SignalOutput` (`hunter_v7_t2_setups.go:1721-1765`) never sets `PriceCtx`, `DerivativesCtx` or `RequiredConfirms`.
- **Change**, in `provider/local/hunter_v7_t2_adapter.go` `RunV7T2Module`. For each emitted output, including FreshlyTriggered re-emissions:
  1. `ctx := buildSymbolContext(sym, snap.Symbols[sym], snap)`;
  2. `out.PriceCtx = buildPriceCtx(ctx)`;
  3. `out.DerivativesCtx = buildDerivCtx(ctx)`.
  - When `snap.Symbols[sym]` is nil (only possible for a re-emitted event whose symbol left the snapshot), set both to empty structs. Add `t2_context_unavailable` to `DataGaps`. Never fabricate values.
  - For outputs whose `EntryMode != immediate`, set `RequiredConfirms = []string{"live_price_in_entry_zone"}`. That is the native code matching the T2 rule "the event triggers when price reaches the entry". It is a known code in `EvaluateV7Confirmations` (`provider/local/hunter_v7_confirmation.go:143`).
  - Do the work in the adapter, not `ToV7SignalOutput`, because the converter has no snapshot. Score, status, entry mode, tiers and lifecycle stay untouched.
- **Tests (new `provider/local/hunter_v7_t2_contract_test.go`):**
  - for a fixture snapshot that fires WATCH, ARMED and TRIGGERED T2 events, every output has non-nil contexts whose values equal the snapshot's (last price, funding, OI);
  - only non-immediate outputs carry the confirmation;
  - a FreshlyTriggered re-emission is covered;
  - a harness-level test in `cmd/hunter_v7_validate/main_test.go` runs `validateFormat` on those outputs and gets `MissingFieldCount == 0` and `ExecutableGapCount == 0`.
- **Golden (acceptance, blocking):**
  - re-run the U1/U4 T2 replay (`cmd/t2_u1_lifecycle` on the U0 corpus);
  - compare per-signal `(symbol, setup, direction, lifecycle_state, status, tier, tier_reason)` before and after;
  - identical tiers and lifecycle are required, and `tier_reason` may differ only as `needs_confirmation` → `confirmation_missing_summary` on WATCH rows;
  - any other difference stops the unit for an owner decision.
- **Rollback:** revert. The fields are additive, and the DB schema is unchanged.

### TA-2: Harness round-failure handling and circuit breaker (P4 ①③)
- **Problem:** round failure → `log.Fatalf` (`main.go:344`) → restart at round 1. Failed-attempt artifacts are mixed in with accepted ones.
- **Change (`cmd/hunter_v7_validate/main.go`):**
  - **Classify failures.** `runValidation` returns a typed error:
    - `transient`: fetch errors, `snapshot quality rejected` (`main.go:1432-1440`), write failures;
    - `contract`: `reportAcceptanceError`, carrying a signature such as `missing_fields:price_context,derivatives_context` (the sorted set of missing field names plus gap codes, without symbols).
  - **Retry policy.**
    - Transient: up to 3 attempts with backoff of 10 s, 30 s, 60 s plus ±20% jitter.
    - Contract: if attempts 1 and 2 fail with the same signature, stop retrying. A re-fetch only changes the market, not the code path.
  - **Skip instead of exit.** A round that exhausts its attempts is recorded in the run summary as `rejected` with its class and signature, and the run continues with the next round.
    - Skipped rounds do not advance native watch state or T2 state (the existing `Clone`/`ReplaceFrom` rule, extended to T2 in TA-3).
    - `--skip-failed-rounds` (default **true** for `--rounds > 1`).
  - **Circuit breaker.** Exit with code **3** (deterministic) when either:
    - `--breaker-consecutive` (default 3) consecutive rounds were rejected with the same contract signature; or
    - more than 50% of the last 10 rounds were contract-rejected.

    Exit with code **2** when `--breaker-transient` (default 6) consecutive rounds were rejected as transient. Code **1** stays for config and setup errors.
  - **Artifacts.** Failed-attempt prompt, raw and markdown files go to `<out-dir>/rejected/` with `-a<attempt>` in the name, plus `"accepted": false` in the raw JSON. Accepted rounds are unchanged.
- **Tests:**
  - table tests for classification and signature equality;
  - a fake `runValidation` (injected function) proving:
    - a run with deterministic contract failures stops at round 3 with exit 3;
    - a single transient failure is retried and continues;
    - a skipped round leaves watch state unchanged;
    - rejected artifacts land in `rejected/`.
- **Acceptance:** tests pass; `--rounds 1` behaviour is unchanged except that artifacts move to `rejected/`.
- **Rollback:** revert. Or pass `--skip-failed-rounds=false` to keep the old exit-on-failure behaviour, with exit code 1 kept for that path.

### TA-3: Resume and state persistence across restarts (P4 ②, duplicate cycle ids)
- **Problem:**
  - T2 lifecycle, native watch state and the module breaker live in memory;
  - restarts start again at round 1 and cycle 1, which causes id collisions and duplicate rows.
- **Change:**
  - **Serialisation of the two state stores:**
    - `provider/local/hunter_v7_t2_lifecycle.go`: `(*V7T2EventStore) Clone()`, `ReplaceFrom()`, `MarshalState() ([]byte, error)` and `LoadState([]byte) error`. Serialise `events` and `latestBySymDir` (as event ids), each event's fields and its `Signal` payload, with `schema_version: 1`.
    - `provider/local/hunter_v7_signal_state.go`: `ExportState()` / `ImportState()` for the watch entries and breaker counters (the unexported state that `Clone` copies today).
  - **Harness (`main.go`):**
    - `--run-id` (required with `--resume`) and `--state-file` (default `<out-dir>/state-<run-id>.json`).
    - After each **accepted** round, atomically write (temp file, fsync, rename): `{schema_version, run_id, harness_git_sha, last_round, next_cycle_number, watch_state, t2_state, saved_at}`.
    - `--resume` loads the file and continues from `last_round + 1`, with cycle numbers continuing from `next_cycle_number`.
    - Resume is refused when `run_id` or `schema_version` don't match. Resuming across a code change (sha mismatch) needs `--resume-allow-sha-change`.
    - T2 attempts use `Clone()` and commit with `ReplaceFrom()` only on success, the same as watch state (`main.go:327-335`).
  - **Time semantics:** stored timestamps are absolute. After downtime, the first `UpdateOutcomes` expires or ends events by wall-clock time, which is correct: expiry is time-based (`hunter_v7_t2_lifecycle.go:269-275`). A WATCH→TRIGGERED progression is kept across a restart only while the event has not expired (2 h WATCH and 60 min cooldown by default).
- **Tests:**
  - a round-trip test showing the restored store gives identical `Upsert`/`UpdateOutcomes`/`FreshlyTriggered` results to the original over a 5-scan sequence;
  - TRIGGERED survives a save and load;
  - a failed attempt doesn't change the committed store;
  - a harness test showing that resume continues cycle numbers without colliding signal ids;
  - a corrupt or mismatched state file is refused.
- **Acceptance:** tests pass; without `--resume` the behaviour is unchanged.
- **Out of scope:** persistence of the live engine's T2 store (`kernel/engine.go:535-538`). It is tied to live enablement, which is parked.
- **Rollback:** revert. The state files are side artifacts under `out-dir`.

### TA-4: Supervisor and tunnel scripts in the repo (P3, P7, P4 ①)
- **Problem:**
  - the supervisor restarts blindly (5 times, no backoff);
  - it cannot tell exit 127 (missing binary) from a crash;
  - it lives outside the repo.
- **Change:** add `scripts/tier_a/supervise.sh` and `scripts/tier_a/tunnel.sh`.
  - **Preflight:** the binary is executable at a persistent path, which is checked before every start (no `/tmp`). `flock` on `<out-dir>/.lock` prevents a second supervisor.
  - **Exit codes:**
    - `0` → done;
    - `3` (deterministic breaker) or `1` (config) → **stop, no restart**, write `STOPPED_DETERMINISTIC`;
    - `2` or a signal/crash → restart with `--resume`;
    - `126`/`127` → stop (environment).
  - **Backoff** between restarts: 30 s × 2^n, capped at 10 min. At most 5 restarts in any 60-minute window, with the counter reset after 2 accepted rounds.
  - **Health gate before each restart:** the relay must answer (`curl -fsS --max-time 5 <binance-url>/fapi/v1/time`). If it doesn't, wait without spending a restart.
  - **Heartbeat:** the harness writes `<out-dir>/heartbeat` (`round`, `phase`, `ts`) each phase. The supervisor logs when the heartbeat is more than 15 min old.
  - **Logging:** every start, exit and decision goes to `supervisor.log`, with UTC timestamps and exit codes.
- **Tests:** `scripts/tier_a/supervise_test.sh` (bats-free POSIX shell) drives a fake harness that exits 0/1/2/3/127 and asserts the decisions; it is called from a Go test via `os/exec` and skipped when `bash` is unavailable.
- **Acceptance:** all decision paths are covered; nothing contacts Binance.
- **Rollback:** delete the scripts.

### TA-5: Fetch and deadline observability (P6)
- **Change (no behaviour change for live callers):**
  - `datafetch.SnapshotMeta`: add `DetailRequested`, `DetailCompleted`, `DetailSkippedDeadline`, `FailedRequests` (count of requests, beside the existing per-symbol `RestErrors`), and `PerEndpointLatencyMs` (p50/p95 per endpoint family).
  - Count the skipped symbols at `fetcher_per_symbol.go:74`.
  - Harness report: add `fetch_ms` = `snap.Meta.FetchDuration`, rename today's value to `round_ms` (keeping the old key as a deprecated alias), and add the new meta fields. Add a medium issue `detail_truncated_by_deadline` when `DetailSkippedDeadline > 0`.
  - `--round-timeout` flag (default `3m`, unchanged).
- **Tests:** a fetcher test with a fake server and a short context showing that skipped symbols are counted and that `RestErrors` is unchanged.
- **Acceptance:** existing datafetch, kernel and trader tests pass unchanged; the new fields are populated.
- **Rollback:** revert. The fields are additive.

### TA-6: Capture one failing request (P5)
Harness side only. The relay stays untouched until the owner decides; see §5.
- `datafetch`: optional `FetcherConfig.ErrorSink func(path string, err error)`, called with the request path and query (public endpoints, no keys) for each failed request after its retries are spent.
- The harness enables it and writes the first 50 per round plus a 1% sample to `<out-dir>/fetch-errors.jsonl`, with `ts`, `round`, `path`, `error` and `attempt`.
- **Tests:** a fake server that resets connections → the sink receives the exact path.
- **Rollback:** revert. It is off by default for every other caller.

### TA-7: Fee-net outcomes in the harness (P8)
- **Change (`main.go:285-302`):** pass `FeeNetOutcome`, `FeeNetR`, `FeeNetEntry` and `DriftAtEntryPct` as `trader/auto_trader.go:536-539` does. Set the tracker's `FeeNetCostPct` from `store.EffectiveV7OutcomeTags(strategy.Hunter.V7OutcomeTags)`, as live does at `trader/auto_trader.go:483-486`.
- **Tests:** a harness callback test with a fake store; a terminal outcome writes `fee_net_outcome`.
- **Acceptance:** tests pass. On a replayed tracker sequence, fee-net values equal the live tracker's.
- **Rollback:** revert.

## 4. Supervisor redesign (concrete)

**State machine:**
`PREFLIGHT → RUNNING → (exit) → DECIDE → {DONE | STOPPED | WAIT_HEALTH → BACKOFF → RUNNING}`

| Harness exit | Meaning | Supervisor action |
|---|---|---|
| 0 | all rounds done | DONE |
| 1 | config/setup error | STOPPED (no restart) |
| 2 | transient breaker (6 consecutive transient rejections) | WAIT_HEALTH, BACKOFF, restart `--resume` |
| 3 | deterministic breaker (same contract signature on 3 consecutive rounds, or >50% of last 10) | STOPPED, write `STOPPED_DETERMINISTIC` with the signature |
| 126/127 | binary missing / not executable | STOPPED (environment) |
| signal / other | crash or kill | WAIT_HEALTH, BACKOFF, restart `--resume` |

**Inside the harness:**
- **Per-round retries:** transient failures are retried at 10 s / 30 s / 60 s with jitter. A contract failure is retried once; a second failure with the same signature ends the round.
- **Skip:** a rejected round is recorded and skipped. State is not advanced.

**Supervisor limits:**
- **Restart budget:** 5 per rolling 60 minutes.
- **Backoff:** 30 s · 2^n, capped at 600 s.
- **Reset:** after 2 accepted rounds.

**T2 continuity:**
- **Saved state:** after every accepted round the harness atomically saves the native watch state, the T2 event store and the next cycle number (TA-3).
- **On restart:** it resumes them, so WATCH→TRIGGERED progress survives restarts.
- **Expiry:** events expire by wall-clock time as designed.
- **Failed attempts** never mutate the committed state: they work on a clone, which is committed only on success.

## 5. P5 diagnosis plan: capture before concluding

No root cause is assumed. The symbols in the evidence are all ASCII, so the non-ASCII bytes must come from somewhere not yet observed.

1. **Harness side (TA-6, in the repo).** Record every failed request path and query after retries, with the error text. This shows which endpoints and symbols fail, and whether the failures come from the round fetcher or the tracker backfill.
2. **Relay side (owner-run; I change nothing on huawei-sg).** Proposed patch to `relay.py`'s forward handler, inside the `except UnicodeEncodeError` (and generic `except`) branch at the `urlopen` call (`relay.py:154`):
   - log `repr(self.requestline)`, `repr(self.path)`, `repr(target_url)` and `self.client_address`;
   - log the header **names** only, plus `repr()` of any header **value** containing non-ASCII bytes;
   - include the UTC timestamp and the exception's `start`/`end` offsets with the slice `repr(s[start-10:end+10])`;
   - log the first 50 occurrences in full, then 1 in 100;
   - no values beyond what the error itself touches, and no query values for non-public paths (none are used).
3. **Correlate.** Match relay timestamps against `fetch-errors.jsonl` and the harness heartbeat phase (`round_fetch` vs `tracker_backfill`).
4. **Capture run.** A 2-round harness run with TA-6 enabled, only if the owner authorises it. It is enough to capture an occurrence; no 144-round rerun.
5. **Then** write the root cause with the captured bytes and fix it in the right component.

## 6. Fetch time (P6): the actual budget

- **Context.** One `context.WithTimeout(3m)` per **attempt**, starting at `runValidation` entry (`main.go:366`). Strategy load runs inside it.
- **What the context gates:** only the start of each per-symbol worker (`fetcher_per_symbol.go:74`).
  - **Not** the bulk calls: `fetchAllTickers`, `fetchAllPremiumIndex` and `fetchExchangeInfo` are sequential and use no context (`fetcher.go:107-109`, `http_client.go:208`).
  - **Not** in-flight requests: they use the 15 s per-request client timeout (`main.go:381`) and up to 3 retries.
  - **Not** scoring or prompt build.
- **Inter-round sleep.** 180 s **after** the round (`main.go:347-350`), not a deadline.
- **Does 183–214 s break a real deadline? No hard one.** The round never fails on the 3-minute timeout. But two things follow:
  - if the fetch phase passed the deadline, the remaining detail symbols were **silently dropped**, with no error and no count;
  - the reported 183–214 s is round time, so the true fetch time and any truncation are unmeasured (TA-5 measures both).
- **Throughput.** By the evidence's own numbers (57,161 relay requests over about 36 attempts), each attempt made about 1,600 requests in about 190 s with `--max-workers 4`. That is roughly 0.45 s per request per worker.
  - Fetch is bounded by latency times concurrency, not by weight: the peak was 682 of 2,000 weight/min.
  - The harness default is `--max-workers 8` (`main.go:223`). Doubling the workers would roughly double the peak weight to about 1,360/min, which stays under both caps.
  - That flag change is the owner's call for any rerun.
- **One more check.** A fully completed detail fetch should produce about 9 requests per symbol, or 11 with T2 inputs. If about 220 detail symbols are expected, the observed request count looks low, which would fit deadline truncation. I can't settle this because the detail-symbol count and the T2-input flag are in the uncommitted diff. TA-5 answers it directly.

## 7. What NOT to change

- The validation contract's required fields and gates (`main.go:1157-1229`, `1445-1464`). Option (b) is rejected.
- T2 lifecycle semantics: event ids, cooldown, expiry, the TRIGGERED rule, observe-only, the U4/OD-11 tier cap.
- Native scoring, routing, assembly, tiering and `geometry_mode` (stays `legacy`).
- `datafetch` request behaviour for live callers: context handling of in-flight requests, retry policy, the rate limiter. TA-5 and TA-6 only add counters and an opt-in sink.
- The live engine's T2 store and any live persistence (live and pilot enablement stay parked).
- The live-test strategy and `data/test.db`.
- The relay on huawei-sg and anything on aix. The relay stays running until the owner decides; §5 step 2 is a proposal for the owner.
- No 144-round rerun until TA-0 to TA-4 are merged and the owner approves.
- Parked items stay parked: U9 conditional entry, U7 E[R] ranking, pilot/live enablement and F08 arming, Task C pass 2.

## 8. Open items for the owner

1. Approve **(a)** for P4 (TA-1).
2. Provide the 61-line Tier A harness diff (TA-0).
3. Approve the unit order and whether each unit branches from `t2/v7-selection` or stacks on `t2/tier-a-fixes`.
4. Decide on the relay logging patch (§5 step 2) and the relay's future.
5. Rerun parameters once TA-0 to TA-4 are in: `--max-workers` (4 vs 8) and `--post-track-duration` (so the ACTIVE tracked signals finish).
