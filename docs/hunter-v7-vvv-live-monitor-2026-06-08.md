# Hunter v7 / VVV Live Monitor - 2026-06-08

## Context

- Trader: `VVV`
- Strategy: `猎手v7`
- Service session: `tmux ait-dev`
- Focus: reduce WAIT/LLM idle cycles, remove stale execution blockers, and keep valid Hunter v7 opportunities executable.

## Live Cycle Findings

| Cycle | Result | Finding |
| --- | --- | --- |
| #222 | `MANTAUSDT open_short` blocked | Old `funding_reversal SHORT` zone-position guard rejected the trade. |
| #223 | AI skipped | Recent failed-open cooldown kept applying the stale guard rejection reason. |
| #224 | `DASHUSDT open_short` failed | LLM selected a trade with confidence 70, but global min confidence 75 rejected it. |
| #225 | Wait | Low-priority candidates still entered LLM because `ready` tier bypassed the priority floor. |
| #226 | AI skipped | After tier tightening, WATCH-only candidate skipped LLM correctly. |
| #227 | AI skipped | WATCH-only EPIC/ESPORTS skipped LLM correctly; no execution blocker observed. |
| #228 | `MANTAUSDT open_short` succeeded | Hunter v7 `funding_reversal SHORT` reached execution and Binance accepted the open. AI latency 8.593s. |
| #229 | `MANTAUSDT hold` succeeded | Existing MANTA position was held; prompt expanded because position context required multi-timeframe data. AI latency 5.887s. |
| #230 | AI skipped | No EXECUTABLE candidates after restart; Hunter v7 correctly skipped LLM with `ai_request_duration_ms=0`. |
| #231 | `BANKUSDT open_short` blocked | Ordinary `watch_only` had been upgraded into EXECUTABLE, then execution guard blocked `distribution_short` because zone position was only 42.1% vs min 60%. This was an avoidable 11.321s LLM call. |
| #232 | `LTCUSDT wait` | Funding fallback was still too wide: LTC entered LLM at priority 47.05 while OI was still building. AI correctly waited, but it cost 11.891s. |
| #233 | AI skipped | After tier tightening, ordinary watch_only and OI-building funding fallback were not sent to LLM; `ai_request_duration_ms=0`. |
| #234 | AI skipped | All candidates were WATCH. Top `BTWUSDT leader_momentum_long` had high setup score but timing only 30, so skipping AI was correct and avoided another low-confirmation WAIT. |
| #235 | AI skipped | Same leader-momentum watch pattern persisted: `BTWUSDT` setup 100 but timing 30; no EXECUTABLE and no LLM call. |
| #236 | `ALLOUSDT open_long` succeeded | Added setup-specific `leader_momentum_long` timing. Restart first cycle upgraded `BTWUSDT` from timing 30/WATCH to timing 68/candidate; AI was called and selected `ALLOUSDT` panic-reversal long. Binance opened 133 ALLO at 0.31636, 10x, with SL 0.30962 and TP 0.32858. |
| #237 | `BTWUSDT wait` | LLM correctly rejected an overheated momentum long: 15m RSI 80.9, funding 0.00124, weak volume, and too-tight invalidation. Root cause was router allowing extreme-funding leader momentum into EXECUTABLE context. |
| #238 | `UBUSDT open_long` succeeded | Added extreme-funding/overheated leader-momentum downgrade. BTW stayed `wait_confirm`; AI selected `UBUSDT` instead. Binance opened 308 UB at 0.12701, 10x, SL 0.12483, TP capped to 0.13248. |
| #239 | Context build failed | Binance server time sync accepted a very slow response and applied a bad offset, causing `code=-1021` timestamp failure. Added RTT/offset safety bounds. |
| #240 | AI skipped | Time-sync fix verified: server sync rtt 140ms, no timestamp failure; BEAT was `chase_risk` and no EXECUTABLE candidates, so AI skipped. Main loop now logs `runCycle returned` and `Initial trading cycle completed`. |

## MANTA Trade Review

- Entry: #228 `MANTAUSDT SHORT`, 20x, requested `75 USDT`, risk control capped actual exposure to about `42.79 USDT`.
- Binance fills: `552.8` contracts at about `0.07741`.
- Protection: TP1 closed about 40% at `0.07716`; stop was rebuilt for the remainder.
- Exit: trailing protection closed the remaining `331.7` contracts at about `0.07726-0.07727`.
- Result in DB: position `CLOSED`, realized PnL `0.102028`, fee `0.04274121`, leverage `20`.
- Sync correction: all 8 MANTA open/close order records now carry leverage `20`; the four trailing-close fills that were previously recorded as `1` were repaired locally.

## ALLO Trade Review

- Entry: #236 `ALLOUSDT LONG`, 10x, requested `75 USDT`, risk control capped exposure to about `42.15 USDT`.
- Binance fill: `133` contracts at `0.31636`.
- Exit: exchange stop closed `133` contracts at `0.30949`.
- Result in DB: position `CLOSED`, realized PnL `-0.91371`, fee `0.04161902`, leverage `10`.
- Lesson: entry worked mechanically, but the selected panic-reversal candidate failed quickly; future review should compare why LLM preferred ALLO over higher-priority leader momentum candidates.

## UB Trade Review

- Entry: #238 `UBUSDT LONG`, 10x, requested `78 USDT`, risk control capped exposure to about `39.22 USDT`.
- Binance fills: `209` + `99` contracts at `0.12701`, merged to `308` contracts.
- Exit: exchange stop closed `308` contracts at `0.12468`.
- Result in DB: position `CLOSED`, realized PnL `-0.71764`, fee `0.03876025`, leverage `10`.
- Lesson: routing and execution recovered after #237, but two recent long attempts both stopped out quickly. Next optimization should inspect whether leader momentum entries need a stricter micro-pullback/retest requirement or lower leverage/size in noisy rotation regimes.

## Fixes Applied

1. Ignored stale failed-open cooldown reasons from the removed `funding_reversal SHORT` zone upper/retest guard.
2. Added Hunter v7 effective open confidence floor: configured 75 remains for general strategies, Hunter v7 executable candidates can pass at confidence 70.
3. Updated Hunter v7 prompt confidence display to match the execution floor, removing the 75/70 contradiction.
4. Tightened `ready` candidate tiering: `execution_quality=ready` now still requires `ai_priority >= 50`; the existing `funding_reversal SHORT` 47+ timing fallback remains.
5. Split REST kline refresh: fast cycles now refresh `1m/5m/15m/1h`; `4h` structure refreshes every 10 REST cycles and is carried between fast cycles.
6. Added regression tests for the cooldown/tiering/confidence and refresh-tier behavior.
7. Added Binance sync leverage fallback: close fills now inherit leverage from active exchange position, then local open position, then latest local position. This prevents fully closed positions from writing trailing close orders as default `1x`.
8. Added unit tests for Binance synced-trade leverage resolution after the exchange-side position has already closed.
9. Removed generic `watch_only -> EXECUTABLE` promotion. Only `ready`, `near_confirm`, `candidate`, and the constrained funding-reversal SHORT fallback can become executable.
10. Added funding-reversal OI-building gate: any `funding_reversal` signal tagged `oi_building_no_flush` stays WATCH, preventing the LLM and execution guard from re-checking the same known blocker.
11. Added setup-specific `leader_momentum_long` timing scoring. The generic timing model did not recognize momentum reason codes, so strong leaders were stuck at the entry-zone-only score of 30. The new model uses taker buy strength, OI growth, 1h continuation/pullback quality, and best-target RR, with caps for late weak chase and crowded blow-off conditions.
12. Added `AutoTrader.Run` runtime exit/panic observability and scan-ticker logs, so a stopped main decision loop is visible instead of leaving only order sync/protector goroutines active.
13. Downgraded `leader_momentum_long` to `chase_risk` when `funding_extreme` or overheated 1h RSI is present. This keeps overheated leaders as WATCH/context but prevents repeats of #237 where LLM had to reject a known crowded momentum setup.
14. Hardened Binance server-time sync: offset is now calculated from request midpoint and ignored when RTT exceeds 5s or raw offset exceeds 60s. This prevents slow `/time` responses from poisoning signed request timestamps and causing `-1021` failures.

## Verification

- `go test ./trader ./kernel ./provider/local`
- `go test ./datafetch ./kernel ./trader ./provider/local`
- `go test ./trader/binance ./store`
- `go test ./kernel ./trader`
- `go test ./...`
- Service restarted with latest code in `ait-dev`.
- Post-fix REST refresh examples: `2.176451717s`, `2.717992168s`, `2.908813942s`, `4.583822773s`, `3.266016035s`, `3.862127218s`, `6.570375455s`.
- Latest restart initial snapshot took `4.30531643s`; first routine post-open REST refresh took `5.227015344s`.
- Binance order sync confirmed ALLO fill after transient EOF retries: `168482859 ALLOUSDT BUY qty=133 price=0.31636 fee=0.02103794 leverage=10 action=open_long`.
- Latest verified cycle: #238, `UBUSDT open_long` succeeded, `ai_request_duration_ms=12429`.
- Binance order sync confirmed UB fills: `227846025` qty `209` and `227846026` qty `99`, both at `0.12701`, merged to `308` contracts with leverage `10`.
- #240 verified the time-sync hardening: `raw_offset=-871ms`, `applied_offset=629ms`, `rtt=140ms`; no `-1021` after restart.

## Current State

The pipeline is no longer stuck in blanket WAIT for executable candidates:

- Valid `open_short` decisions are reaching execution and can complete through protector-managed partial and trailing exits.
- Stale guard/cooldown blockers have been removed.
- Confidence 70 Hunter v7 opens will no longer be rejected by the old 75 floor.
- Low-priority WATCH candidates are skipped before LLM to reduce idle cost and latency.
- Ordinary WATCH-only candidates are no longer promoted into EXECUTABLE; this removes the prompt/guard contradiction that caused #231.
- Funding-reversal SHORT fallback now requires no OI-building blocker; this removes the #232 low-priority OI-building LLM wait path.
- Routine data snapshots are back in the ~2-5s range after removing 4h and heavier derivative history from every fast refresh.
- Account state after MANTA was flat; after #238 the system now has one open UB long.
- #234 confirms the stricter tiering is suppressing low-timing momentum candidates before LLM instead of creating repeated WAIT decisions.
- #235 confirms the repeated skip is caused by missing timing confirmation, not LLM refusal or execution guard conflict.
- #236 confirms the leader-momentum timing gap was real: `BTWUSDT` moved from WATCH-only to executable context after the timing model fix, and the system resumed opening trades. The AI selected `ALLOUSDT` instead of the top momentum leader, and execution completed through risk sizing, leverage setting, market entry, SL, TP, order sync, and protector tracking.
- #237 confirms the next layer of filtering needed to move from "opens again" to "opens cleaner": extreme funding on leader momentum should be handled in router/execution quality, not by LLM wait.
- #238 confirms that filter worked in live routing: `BTWUSDT` became `wait_confirm`, while cleaner leader momentum `UBUSDT` reached execution and opened successfully.
- Current account state after #240: no open positions. `UBUSDT` is closed at `-0.71764` realized PnL; `ALLOUSDT` is closed at `-0.91371` realized PnL.

Next item to monitor: confirm the next 10-minute scan ticker fires after #240, and inspect whether long entries in rotation regime need stricter confirmation after two quick stop-outs.

## 11:09 CST Follow-up: REVIEWABLE Path and OPENUSDT SHORT

### Live Cycles

| Cycle | Result | Finding |
| --- | --- | --- |
| #249 | `BSBUSDT open_short` succeeded | Funding SHORT path produced a small fast profit. |
| #250 | `BSBUSDT wait` | AI rejected repeat because backend-capped RR was only 1.45, below 1.5. |
| #251 | `EPICUSDT wait` | Correctly rejected overheated momentum: price far above entry zone, RSI overbought, RR poor. |
| #252 | `EPICUSDT open_long` blocked | Risk control rejected 3.423% decision/execution drift. User correctly noted live EPIC would have stopped out; later preflight rules now forbid treating broken momentum as a better entry. |
| #253-#257 | AI skipped | No `EXECUTABLE/REVIEWABLE`; WATCH-only candidates avoided LLM. |
| #258 | `MANTAUSDT wait` | Strong pullback path now reaches REVIEWABLE and calls LLM. Wait was correct: RR 1.27, 15m below VWAP/EMA20/BOLL mid. |
| #259 | `OPENUSDT wait` | Prompt contradiction fixed; LLM still waited on weak C-confidence funding SHORT with very low volume and recent OPEN loss. |
| #260 | `OPENUSDT open_short` succeeded | OPEN improved to priority 49.05, timing 72, taker_buy 0.408, OI flush, 15m below VWAP/EMA20. Position opened and synced. |

### Fixes Added

1. Strategy DB prompt fixed so `REVIEWABLE` is no longer a hard reject. Old wording said only `EXECUTABLE` could open; new wording says `EXECUTABLE/REVIEWABLE` may be reviewed, while `WATCH/REJECTED` cannot open directly.
2. Decision process now says flat account analyzes `EXECUTABLE` first and `REVIEWABLE` second.
3. Funding-reversal SHORT review fallback tightened from `ai_priority >= 45` to `>= 47`, preserving useful 47.5+ funding SHORT opportunities but blocking weaker 45.x C-confidence candidates like #259.
4. Added regression coverage for weak OPEN-like funding SHORT fallback.

### OPENUSDT #260 State

- Fill: `117` contracts SHORT at `0.1997`, 10x.
- Risk control reduced requested notional from `60.00` to about `23.45 USDT`.
- SL: `0.2040`; TP: `0.1920`.
- Approximate geometry from fill: `2.15%` stop, `3.85%` TP, RR about `1.79`.
- Position synced in DB as `OPEN`, fee `0.01168245`.
- Next item: follow whether OPEN exits by TP, SL, protector, or later LLM management. Outcome should decide the next funding SHORT adjustment.

## 11:27 CST Follow-up: Current Position Risk Context

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #261 | `OPENUSDT hold` | Direction was correct, but reasoning incorrectly implied missing TP/SL because current-position context did not include planned protective prices. |
| #262 | `OPENUSDT hold` | After restart with the fix, the prompt included `Planned SL 0.2040 | Planned TP 0.1920`; reasoning correctly held because price was still below SL and had not reached TP. |

### Fix Added

15. Added planned SL/TP propagation into current-position context. `PositionInfo` now includes `StopLoss` and `TakeProfit`; `buildTradingContext()` backfills them from recent successful open decisions; `formatPositionInfo()` prints them in the position line.
16. Added regression coverage so Hunter v7 compact position prompts include planned risk, and live context construction can recover SL/TP from decision records.

### Current State

- Services restarted in `tmux ait-dev`; dashboard remains available at `http://127.0.0.1:3000/dashboard`.
- Latest verified cycle: #262, `OPENUSDT hold`, confidence `85`, AI call `5.126s`, tokens `4774/449/5223`.
- `OPENUSDT SHORT` remains open: entry `0.1997`, qty `117`, leverage `10x`, planned SL `0.2040`, planned TP `0.1920`.
- Funding SHORT candidate quality cooled back to `ai_priority=45.8`, below the new review floor, so no additional open path is being promoted while the current position is active.

## 11:39 CST Follow-up: #263 and Parser Record Quality

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #263 | `OPENUSDT hold` | Hold remained correct. Prompt included planned SL/TP; CoT reasoned that the short is still valid, but JSON omitted `reasoning`, so DB decision_json stored an empty reason. |

### Current OPENUSDT State

- `OPENUSDT SHORT` remains open, entry `0.1997`, qty `117`, 10x.
- Latest account check around 11:38 CST showed unrealized PnL about `+0.1052 USDT`.
- Protector at 11:38:41 CST: profit `4.51%`, peak `4.51%`, drawdown `0.00%`, TP1/TP2 false.
- Protector at 11:40:41 CST: profit `4.01%`, peak `5.99%`, drawdown `33.14%`, TP1/TP2 false.
- No close fill has synced through 11:41:42 CST.
- TP1 threshold is `6.0%`; OPEN reached `5.99%`, so it narrowly missed automatic protection. Because the position is small, a TP1 trigger would likely close all due to minimum close-notional constraints.

### Data Source Note

- `OPENUSDT` Binance Kline calls returned all-zero candles on multiple timeframes during #262/#263.
- The market layer rejected those batches and fell back to CoinAnk; compact market data was still usable.
- This is a data-source quality item to monitor, not currently a strategy-tightening trigger.

### Fix Added

17. Added parser fallback for missing decision JSON `reasoning`: when `<reasoning>` exists and a decision omits `reasoning`, the parser stores a compact CoT summary into `Decision.Reasoning`.
18. Added `TestParseFullDecisionResponseBackfillsMissingReasoningFromCoT`.

### Verification / Deployment Note

- Verified with:

```bash
go test ./kernel ./trader
go test ./provider/local
```

- Service was not restarted for this parser-only fix because the active `OPENUSDT` position has a live protector peak of `4.51%`; restarting now would reset in-memory peak tracking. Deploy on the next safe restart or after the position closes.
- No protector threshold change was applied mid-trade; judge whether TP1 should be lowered only after the actual OPENUSDT exit is known.

## 11:48 CST Follow-up: #264 Hold and Executable Protection Semantics

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #264 | `OPENUSDT hold` | Hold remained directionally coherent, but the reasoning claimed "tighten stop to 0.2020" even though `hold` cannot modify stop orders. |

### #264 Details

- #264 ran at 11:46:41-11:46:57 CST.
- Hunter v7 scanned `99` symbols and produced `3` signals: `TRUTHUSDT`, `TONUSDT`, `SKYAIUSDT`.
- With max position already reached, candidates were background only. `TRUTHUSDT` was also skipped from full market expansion because OI was `2.30M USD`, below the effective `3.0M` threshold.
- Current position context was intact: `OPENUSDT SHORT`, entry `0.1997`, current about `0.1990`, qty `117`, leverage `10x`, planned SL `0.2040`, planned TP `0.1920`, peak PnL `5.99%`.
- The model held because short structure remained intact and TP/SL had not been reached.
- The non-executable "tighten stop" sentence exposed a prompt/schema mismatch: valid live actions are open, close, hold, and wait; `hold`/`wait` are no-op.

### Protector Observation

- OPENUSDT peaked at `5.99%`, just below `protectorTP1PnLPct = 6.0`.
- Later protector logs showed giveback:
  - 11:46:12 CST: profit `4.01%`, drawdown `33.14%`.
  - 11:46:42 CST: profit `3.51%`, drawdown `41.50%`.
  - 11:47:42 CST: profit `3.17%`, drawdown `47.02%`.
- This is a candidate for future near-TP1 giveback protection, but not enough evidence to change global protector constants mid-trade.

### Fix Added

19. Added prompt constraint: `hold` and `wait` do not change SL/TP, leverage, or position size.
20. The model must use `close_long`/`close_short` if profit protection requires an executable action; otherwise it may hold but must not claim stop tightening.

### Verification / Deployment Note

```bash
go test ./kernel ./trader
```

- Passed.
- Not deployed yet because service restart would reset the in-memory protector state for the open `OPENUSDT` short.

## 11:52 CST Follow-up: Near-TP1 Giveback Protection

### Additional Live Evidence

- `OPENUSDT SHORT` remained open after #264.
- The peak remained `5.99%`, but TP1 requires `6.0%`, so no TP1 action fired.
- Protector/account observations after #264:
  - 11:48:12 CST: profit `3.00%`, peak `5.99%`, drawdown `49.85%`, TP1=false.
  - 11:51:26 CST: unrealized PnL about `+0.0234 USDT`.
  - 11:52:28 CST: unrealized PnL about `+0.0318 USDT`.
- No close fills synced through 11:52:42 CST.

### Fix Added

21. Added near-TP1 giveback close logic for the protector:
   - peak PnL >= `protectorTP1PnLPct * 0.95` (`5.70%` with current TP1 `6.0%`);
   - current PnL >= `3.0%`;
   - hold time >= `20m`;
   - drawdown from peak >= `45%`;
   - action: `protectionGivebackClose`.
22. Fast protector interval now begins at the near-TP1 zone, not only after exact TP1.

### Verification / Deployment Note

```bash
go test ./trader -run 'TestChoosePositionProtectionAction|TestShouldUseFastProtectionInterval'
go test ./kernel ./trader
```

- Passed.
- Not deployed yet. Restart only after `OPENUSDT` closes or when explicitly accepting reset of the current in-memory protector peak.

## 12:01 CST Follow-up: #265 and Near-TP1 LLM Exit Rule

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #265 | `OPENUSDT hold` | The position had already peaked near TP1 and was back near/lower than breakeven, but the model still held and again claimed stop tightening inside a no-op `hold`. |

### #265 Details

- #265 ran at 11:56:41-11:56:58 CST.
- Hunter v7 scanned `99` symbols and produced `5` signals: `TONUSDT`, `TAGUSDT`, `SKYAIUSDT`, `WLDUSDT`, `PORTALUSDT`.
- Because `OPENUSDT SHORT` was still open and `max_positions=1`, candidate symbols were background only.
- Decision JSON: `OPENUSDT hold`.
- Reasoning flaw: it cited planned SL `0.2040` and TP `0.1920`, then said "tighten stop to 0.2030" while still outputting `hold`. The live executor cannot modify stops from a hold action.

### Live Evidence After #265

- `OPENUSDT SHORT` remained open; no close fills synced through 12:00:41 CST.
- DB equity snapshots:
  - 11:46:41 CST: unrealized PnL `+0.08184704 USDT`.
  - 11:56:41 CST: unrealized PnL `-0.02338490 USDT`.
- API account checks:
  - 11:57:02 CST: unrealized PnL `-0.01538620 USDT`.
  - 11:57:32 CST: unrealized PnL `-0.01169369 USDT`.
  - 11:58:03 CST: unrealized PnL `+0.00174111 USDT`.
  - 11:58:33 CST: unrealized PnL `-0.00616449 USDT`.
  - 11:59:34 CST: unrealized PnL `-0.01169267 USDT`.
  - 12:00:35 CST: unrealized PnL `-0.05755813 USDT`.
  - 12:01:35 CST: unrealized PnL `-0.04464200 USDT`.
  - 12:02:06 CST: unrealized PnL `-0.06106145 USDT`.
- Interpretation: `Peak PnL 5.99%` was operationally close to TP1 `6.0%`; giving back to flat/loss should be treated as risk-reducing close territory unless a strong continuation signal is explicit.

### Fix Added

23. Added Hunter v7 position-management prompt rule:
   - If `Peak PnL` reached near TP1, e.g. `>=5.7%` when TP1 is `6.0%`,
   - and the trade gives back `>=45%` from the peak or current PnL crosses to breakeven/loss,
   - output executable `close_long`/`close_short` unless there is a very explicit continuation signal.
   - Do not use `hold` to claim stop tightening.
24. Added regression coverage in the Hunter v7 system prompt test for the near-TP1 giveback close rule.

### Verification / Deployment Note

```bash
go test ./kernel ./trader
```

- Passed.
- This fix is not active in the running process until restart.
- Running service is still intentionally not restarted while `OPENUSDT` is open, because restart would reset the current in-memory protector peak. Next expected scan cycle is around 12:06:41 CST if the 10-minute loop continues normally.

## 12:10 CST Follow-up: #266 and Restart-Safe Peak Protection

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #266 | `OPENUSDT hold` | The old running logic still held even with `Peak PnL5.99%` and current PnL `-1.80%`; CoT miscomputed the giveback as a small `-7.79%` move instead of `>100%` peak giveback. |

### #266 Details

- #266 ran at 12:06:41-12:06:57 CST.
- Hunter v7 scanned `99` symbols and produced `2` signals: `TONUSDT`, `ENAUSDT`.
- The current position line in the prompt was:
  - `OPENUSDT SHORT`, entry `0.1997`, current `0.2001`, PnL `-1.80%`, `Peak PnL5.99%`, planned SL `0.2040`, planned TP `0.1920`, holding duration `40 min`.
- Decision JSON: `OPENUSDT hold`, confidence `80`.
- Correct interpretation: `(5.99 - -1.80) / 5.99 = 130.1%` giveback from peak. This is a full missed-TP giveback plus loss, so it should be an executable close/protection case under the new rules.

### Additional Live Evidence

- `OPENUSDT SHORT` remained open; no close fills synced through 12:09:41 CST.
- API account checks:
  - 12:07:40 CST: unrealized PnL `-0.04044557 USDT`.
  - 12:08:11 CST: unrealized PnL `-0.06553542 USDT`.
  - 12:08:41 CST: unrealized PnL `-0.05291127 USDT`.
  - 12:09:11 CST: unrealized PnL `-0.01027194 USDT`.
  - 12:09:42 CST: unrealized PnL `-0.04677084 USDT`.

### Fix Added

25. Added restart-safe peak recovery:
   - if in-memory peak cache is empty, recover `Peak PnL` from recent `decision_records.input_prompt`;
   - match same trader, symbol, side, and only records at/after current DB open time;
   - recovered peak feeds both LLM position prompt and automatic protector state.
26. Extended near-TP1 protector:
   - if near-TP1 peak has occurred and drawdown is `>=45%`, protector closes while current PnL is still `>=3%`;
   - if the trade has already crossed into loss, automatic close now requires material loss (`<= -5.0%` leveraged PnL), avoiding mechanical liquidation on tiny post-giveback noise.
27. Clarified prompt math:
   - giveback is `(Peak PnL - Current PnL) / Peak PnL`;
   - positive peak to negative current PnL is `>100%` giveback.

### Verification / Deployment Note

```bash
go test ./trader -run 'TestEnsurePeakPnLCacheInitializedRestoresFromRecentPrompt|TestChoosePositionProtectionAction|TestShouldUseFastProtectionInterval'
go test ./kernel ./trader
```

- Passed.
- Not active until service restart.
- The earlier deployment concern is now reduced: after restart, the new code should recover `OPENUSDT SHORT` peak `5.99%` from #264-#266 prompts instead of starting protector memory from the current lower PnL.

## 12:13 CST Follow-up: #266 Recovery and Protector Retune

### Live Evidence

- #266's hold was poor reasoning quality because the model miscomputed giveback from `Peak PnL5.99%` to current `-1.80%`.
- However, the live position later recovered:
  - 12:10:12 CST: unrealized PnL `-0.02338652 USDT`.
  - 12:10:43 CST: unrealized PnL `-0.05038107 USDT`.
  - 12:11:13 CST: unrealized PnL `-0.04048219 USDT`.
  - 12:11:43 CST: unrealized PnL `-0.01356995 USDT`.
  - 12:12:14 CST: unrealized PnL `-0.00551417 USDT`.
  - 12:12:43 CST protector: profit `4.07%`, peak `5.99%`, drawdown `32.14%`, TP1=false.
  - 12:12:44 CST: unrealized PnL `+0.09773132 USDT`.
- No close fills synced through 12:12:42 CST.

### Retune

28. Narrowed automatic near-TP1 loss exit:
   - small post-giveback loss like `-1.80%` should not be mechanically closed without market context;
   - material post-giveback loss threshold is now `protectorNearTP1LossExitPnLPct = -5.0`.
29. Kept LLM prompt strict:
   - near-TP1 full giveback is still an exit/risk-reduction signal;
   - `hold` must cite explicit continuation evidence and cannot rely only on "SL/TP not touched".

### Verification

```bash
go test ./trader -run 'TestEnsurePeakPnLCacheInitializedRestoresFromRecentPrompt|TestChoosePositionProtectionAction|TestShouldUseFastProtectionInterval'
go test ./kernel ./trader
```

- Passed.

## 12:19 CST Follow-up: #267 Second-Chance Near-TP1

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #267 | `OPENUSDT hold` | The trade returned to `+5.54%` after the earlier near-TP1 miss and full giveback, but old logic still waited for exact TP1. |

### #267 Details

- #267 ran at 12:16:41-12:16:56 CST.
- Hunter v7 produced `OPENUSDT`, `TRXUSDT`, `ENAUSDT`; `TRXUSDT` was excluded and `ENAUSDT` stayed background while `max_positions=1`.
- Position prompt:
  - `OPENUSDT SHORT`, entry `0.1997`, current `0.1986`, PnL `+5.54%`, peak `5.99%`, planned SL `0.2040`, planned TP `0.1920`, holding duration `50 min`.
- Decision: `OPENUSDT hold`, confidence `80`.

### Live Evidence

- No close fills synced through 12:18:41 CST.
- Protector path:
  - 12:15:57 CST: profit `5.80%`, peak `5.99%`, drawdown `3.16%`.
  - 12:16:27 CST: profit `5.51%`, peak `5.99%`, drawdown `8.07%`.
  - 12:16:41 CST cycle snapshot: unrealized PnL `+0.12786685 USDT`.
  - 12:16:57 CST: profit `5.01%`, peak `5.99%`, drawdown `16.42%`.
  - 12:17:27 CST: profit `4.80%`, peak `5.99%`, drawdown `19.90%`.
  - 12:17:57 CST: profit `4.49%`, peak `5.99%`, drawdown `25.05%`.
  - 12:18:58 CST: profit `4.51%`, peak `5.99%`, drawdown `24.78%`.
- Interpretation: this was a second chance to protect a trade that had already missed TP1 once. Waiting for exact `6.0%` again left the position exposed to another fade.

### Fix Added

30. Added second-chance near-TP1 protection:
   - prior peak >= `95%` of TP1;
   - current PnL >= `90%` of TP1;
   - hold time >= `20m`;
   - action: close/protect via `protectionGivebackClose`.
31. Added regression coverage for the #267 shape and non-trigger cases.
32. Updated Hunter v7 prompt to treat missed near-TP1 recovery above 90% TP1 as a second protection chance.

### Verification

```bash
go test ./trader -run 'TestChoosePositionProtectionAction|TestShouldUseFastProtectionInterval'
go test ./kernel ./trader
```

- Passed.
- Not active until service restart.

## 12:28 CST Follow-up: #260 Closed, Service Restarted, #268 Opened

### #260 Final Result

- `OPENUSDT SHORT` finally closed by protector at 12:20:28 CST.
- DB position id `125`: entry `0.1997`, exit `0.1984`, qty `117`, leverage `10x`, status `CLOSED`.
- Realized PnL: `+0.1521 USDT`; total fee: `0.02328885`.
- Close order id `478`: `OPENUSDT BUY SHORT MARKET FILLED`, avg fill `0.1984`, commission `0.0116064`.
- Verdict: the selection was acceptable and profitable; the weakness was profit management after a near-TP1 miss, not the funding-short entry itself.

### Deployment

- After #260 closed, service was restarted in `tmux ait-dev` at 12:24 CST.
- Backend/frontend are listening on `:8080` and `:3000`; VVV auto-restored from DB.
- The previously tested fixes are now live:
  - `hold`/`wait` cannot pretend to tighten SL/TP;
  - near-TP1 giveback is an LLM exit/risk-reduction signal;
  - protector can recover peak PnL after restart;
  - near-TP1 giveback and second-chance protection are active.

### Live Cycle #268

| Cycle | Result | Finding |
| --- | --- | --- |
| #268 | `OPENUSDT open_short` | Restarted Hunter v7 did not return to WAIT-only behavior. It produced 3 candidates and opened a repeated `funding_reversal SHORT` after LLM review and risk-size reduction. |

Details:

- Initial snapshot: `525` symbols in `4.574751904s`.
- Hunter v7: `99` symbols -> `3` signals.
- Candidate set: `OPENUSDT`, `DASHUSDT`, `LTCUSDT`.
- LLM decision: `OPENUSDT open_short`, confidence `75`, requested notional `60 USDT`, leverage `20x`.
- Risk control:
  - reduced notional `60.00 -> 27.56 USDT` because estimated SL loss exceeded 6% equity cap;
  - capped TP from `0.19070000` to `0.19094400`;
  - adjusted SL from `0.20260000` to `0.20293767` because stop distance edge was below 2%.
- Synced fills:
  - order `479`: `54 @ 0.1989`, fee `0.0053703`;
  - order `480`: `85 @ 0.1988`, fee `0.008449`.
- DB position id `126`: `OPENUSDT SHORT`, avg entry `0.19883885`, qty `139`, leverage `20x`, fee `0.0138193`, status `OPEN`.
- As of 12:27 CST, no close fill is synced.

### Current Interpretation

- The core "open rate vs WAIT空转" issue is improved: REVIEWABLE candidates are reaching the LLM and executable orders again.
- #260 validates keeping selective `funding_reversal SHORT`.
- #268 needs close monitoring because it is another OPEN short shortly after a profitable OPEN short; repeated same-symbol entries are acceptable only while taker sell/OI flush remain intact.
- Do not loosen global thresholds further. The next optimization should be driven by #268's realized path and whether the new near-TP1 protection exits earlier than the old exact-TP1 behavior.

## 12:48 CST Follow-up: #268 TP1 Protector Close and #270 Skip

### #268 Final Result

- `OPENUSDT SHORT` closed at 12:35:21 CST by protector.
- DB position id `126`: avg entry `0.19883885`, exit `0.1981`, qty `139`, leverage `20x`, status `CLOSED`.
- Realized PnL: `+0.1027 USDT`; total fee: `0.02758725`.
- Close order id `481`: `OPENUSDT BUY SHORT MARKET FILLED`, avg fill `0.1981`, commission `0.01376795`.
- Equity after closing was flat at about `8.6199 USDT`, up from the 12:24 flat equity `8.5447 USDT`.

### Management Evidence

- #269 ran while #268 was still open.
- LLM held at about `+5.43%` PnL because the short was still structurally valid and TP had not been hit.
- Protector then handled the trade:
  - 12:35:06 CST: profit `5.98%`, peak `5.98%`, age `10m`, TP1=false.
  - 12:35:21 CST: TP1 protection triggered at mark `0.19817291`.
  - Because the position was small, TP1 partial logic closed all `139` contracts.
- Verdict: no new protection defect. The LLM held just under TP1, but the live protector captured profit quickly without waiting for a drawdown.

### #270 Flat Scan

| Cycle | Result | Finding |
| --- | --- | --- |
| #270 | AI skipped | Valid skip: only WATCH candidates remained. BLESS was `chase_risk`; LTC/ENA funding shorts lacked enough review quality. |

Details:

- Hunter v7 scanned `99` symbols and produced `3` signals:
  - `BLESSUSDT LONG leader_momentum_long`, setup `88.8`, timing `40`, quality `chase_risk`, priority `49.6`;
  - `LTCUSDT SHORT funding_reversal`, priority `47.0`, `oi_building`;
  - `ENAUSDT SHORT funding_reversal`, priority `46.2`.
- Execution record: `no_open_review_candidates watch=3 rejected=0`, AI skipped.
- Interpretation: this is useful pre-LLM filtering, not bad WAIT空转. It avoids spending LLM calls on known chase/oi-building blockers.

### Updated Strategy Stance

- Keep `funding_reversal SHORT` REVIEWABLE path active; #260 and #268 were both profitable after the prompt/router fixes.
- Do not loosen global thresholds. #270 shows the router is still blocking low-quality chase risk.
- Do not add same-symbol cooldown yet. The repeat OPEN short was profitable and confirmed by taker sell/OI flush; a cooldown would have blocked this edge.
- Continue monitoring whether future profitable shorts get protected at TP1 or near-TP1 without full giveback.

## 12:58 CST Follow-up: #271 Leader Momentum WATCH Alignment

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #271 | AI skipped | Valid skip: BLESS had `leader_momentum_long` setup quality but only `timing=48`, so it should remain WATCH rather than being sent to LLM. |

### #271 Details

- #271 ran at 12:54:35 CST with no open position.
- Hunter v7 output:
  - `BLESSUSDT LONG leader_momentum_long`: setup `74.4`, risk `15`, timing `48`, liquidity `85`, priority `57.8`, status `candidate`.
  - `DASHUSDT SHORT funding_reversal`: setup `55`, risk `15`, timing `72`, liquidity `55`, priority `48.0`, status `wait_confirm`.
  - `EIGENUSDT LONG pre_breakout_watch`: setup `73`, risk `0`, timing `20`, liquidity `65`, priority `43.5`, status `wait_confirm`.
- Runtime saved `ALL wait` with `no_open_review_candidates watch=3 rejected=0`; no LLM call and no order.

### Interpretation

- BLESS was not a missed trade. `leader_momentum_long` needs stronger live confirmation than setup score alone; timing `48` is too weak for review/open.
- This is the same risk class as the EPIC warning: a strong-looking momentum name can still be a bad immediate entry if live timing is broken or under-confirmed.
- The confusing part was semantic: provider output could still say `status=candidate` for low-timing leader momentum while runtime tiering correctly converted it to WATCH.

### Fix Added

33. Provider-side execution quality now downgrades low-timing leader momentum:
   - `leader_momentum_long` with `TimingScore < 60` becomes `watch_only`;
   - status becomes `wait_confirm`;
   - reason/tag added: `leader_momentum_timing_watch_only`, `momentum_confirmation_missing`.
34. Added a regression test matching the #271 BLESS shape.

### Verification

```bash
go test ./provider/local -run 'TestV7ExecutionQualityDowngradesLowTimingLeaderMomentum|TestLeaderMomentum'
go test ./kernel ./trader ./provider/local
```

- Passed.

### Current State

- Latest verified live trade remains #268, closed profitably at `+0.1027 USDT`.
- Latest verified flat scan is #271, and the skip is valid.
- No global threshold loosening is justified. The current work is to keep candidate semantics honest while preserving the proven `funding_reversal SHORT` review path.

## 13:03 CST Follow-up: Restart Deployment and #272

### Deployment

- Restarted `tmux ait-dev` while VVV was flat.
- Backend/frontend are listening on `:8080` and `:3000`.
- The low-timing leader momentum provider alignment is live.
- Initial data snapshot after restart: `525` symbols in `15.211706758s`.

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #272 | AI skipped | Valid post-restart skip: raw Hunter v7 had 14 signals, but all were `watch_only`; no LLM call and no order. |

### #272 Details

- Route diagnostics:
  - `raw=14`, `eligible=14`, `confirmed=0`, `watches=0`, `output=0`;
  - `ready=0`, `near=0`, `watch=14`, `invalid_rr=0`, `filtered=0`;
  - top raw signal: `TRXUSDT LONG funding_reversal`, `status=wait_confirm`, `quality=watch_only`, priority `42.1`, risk `0.0`, timing `72.0`.
- Final Hunter v7 output: `0 signals`.
- DB decision: `ALL wait`, reason `no_candidate_coins`, `prompt_tokens=0`, `total_tokens=0`.

### Interpretation

- This is not LLM WAIT空转. The system skipped LLM because no signal met review/open quality.
- The route diagnostic now exposes the raw/watch-only reason path, which is exactly what was missing when the dashboard only showed empty output.
- Next checkpoint: the next scheduled cycle after warmed snapshots, to verify real REVIEWABLE candidates still reach LLM while low-timing momentum stays WATCH.

## 13:14 CST Follow-up: #273 Warmed Snapshot Validation

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #273 | AI skipped | Valid skip: warmed snapshot produced FHE/BLESS leader momentum, but both had weak timing and remained WATCH. |

### #273 Details

- #273 ran at 13:11:26 CST.
- Route diagnostics:
  - `raw=16`, `eligible=16`, `output=2`;
  - `ready=0`, `near=1`, `watch=14`, `invalid_rr=1`, `filtered=0`;
  - top raw signal: `FHEUSDT LONG leader_momentum_long`, `status=wait_confirm`, `quality=chase_risk`, priority `48.8`, risk `15.0`, timing `45.0`.
- Final output:
  - `FHEUSDT LONG leader_momentum_long`: setup `92.4`, risk `15`, timing `45`, liquidity `50`, priority `48.8`, status `wait_confirm`.
  - `BLESSUSDT LONG leader_momentum_long`: setup `68.4`, risk `15`, timing `53`, liquidity `85`, priority `48.7`, status `wait_confirm`.
- Runtime decision: `ALL wait`, reason `no_open_review_candidates watch=2 rejected=0`.
- LLM tokens: `0`; no order; account remains flat.

### Interpretation

- The provider alignment worked: low-timing leader momentum now presents as `wait_confirm`, not as a misleading immediate candidate.
- This is a valid no-trade cycle, not LLM WAIT空转.
- Do not loosen leader momentum after #273; FHE/BLESS are exactly the kind of under-confirmed momentum that can stop quickly if chased.

### Current State

- Services are running in `tmux ait-dev`.
- Dashboard: `http://127.0.0.1:3000/dashboard`.
- VVV remains flat; latest closed trade is still #268 `OPENUSDT SHORT`, `+0.1027 USDT`.

## 13:24 CST Follow-up: #274 REVIEWABLE Validation

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #274 | LLM wait | Valid skip: BANK funding-short reached REVIEWABLE and LLM, but entry location/quality were not good enough; SKYAI panic-long stayed WATCH. |

### #274 Details

- #274 ran at 13:21:26 CST with no open position.
- Hunter v7 output:
  - `BANKUSDT SHORT funding_reversal`: setup `50.0`, risk `8`, timing `67`, liquidity `85`, priority `48.5`, status `wait_confirm`.
  - `SKYAIUSDT LONG panic_reversal_long`: setup `30.5`, risk `8`, timing `45`, liquidity `85`, priority `48.2`, status `wait_confirm`.
- Prompt tier summary:
  - `EXECUTABLE=0`;
  - `REVIEWABLE=1`;
  - `WATCH=1`;
  - `REJECTED=0`.
- LLM returned:
  - `BANKUSDT wait`;
  - confidence `55`;
  - prompt/output/total tokens `6053/2233/8286`.
- No order was executed.

### Interpretation

- This is not the previous LLM WAIT空转 problem. The router promoted one selective `funding_reversal SHORT` to REVIEWABLE, and the LLM rejected it for concrete execution reasons.
- BANK short had valid structure, but price was near the lower edge of the short zone, `execution_quality=watch_only`, confidence `C`, and 4h trend remained bullish. Opening there would be late chase risk.
- SKYAI does not justify loosening `panic_reversal_long`: setup `30.5` and timing `45` are far below the high-win historical panic-reversal profile.

### Data Quality Note

- A REST refresh immediately before the scan logged `525 symbols, 13.501614311s, 1 errors`.
- SnapshotStore still provided market data for both candidates, and the next refresh logged `0 errors`.
- Keep tracking refresh latency/errors, but do not change strategy thresholds based on this single non-blocking incident.

### Optimization Record

- No strategy/config change from #274.
- Keep the selective `funding_reversal SHORT` review path open.
- Keep low-quality `panic_reversal_long` candidates as WATCH/context only.
- Continue monitoring for a true panic-reversal or a funding-short retest near a better entry zone before opening.

## 13:49 CST Follow-up: #275 Tight-Stop Preflight and #277 PARTIUSDT SHORT

### Live Cycles

| Cycle | Result | Finding |
| --- | --- | --- |
| #275 | `BANKUSDT open_short` blocked | LLM opened a REVIEWABLE funding short, but live stop distance was only `1.35%`, below Hunter v7 minimum `2.00%`. This cost a `15.457s` LLM call and then failed at execution guard. |
| #276 | LLM wait | CLO and OPEN were both rejected by AI; CLO explicitly had stop distance `1.64% < 2.00%`, OPEN was still a low-priority lower-zone funding short. |
| #277 | `PARTIUSDT open_short` succeeded | After restart, funding short path opened successfully. Borderline stop distance `1.943%` was repaired to `2.03%`, position size was risk-capped, and Binance accepted the trade. |

### #275 Root Cause

- The REVIEWABLE funding-short fallback only checked priority, timing, risk, liquidity, and taker sell pressure.
- It did not pre-check whether the structured signal invalidation could satisfy the Hunter v7 minimum stop-distance rule.
- BANK therefore reached LLM and produced an `open_short` with `stop_loss=0.03831` against live executable price `0.03780`, leaving only `1.35%` stop distance after drift refresh.
- Execution guard correctly rejected the order; the issue was that the prompt tier allowed a known infeasible geometry candidate to consume LLM and attempt execution.

### Fix Added

21. Added Hunter v7 candidate-tier precheck for `funding_reversal`: when `price_context.last`/entry-zone fallback and `invalidation.price` imply stop distance below `2.00%`, the candidate is downgraded to `WATCH` with reason `funding_reversal_stop_too_tight`.
22. This preserves the execution-layer hard guard and the preflight edge repair path: #277 showed a `1.943%` stop edge miss can still be repaired to `0.05539209`, while materially tight #275-style `1.35%` setups are filtered before LLM/open.
23. Added regression coverage: `TestClassifyHunterV7CandidateTierBlocksFundingShortTightStop`.

### Verification / Deployment

```bash
go test ./kernel ./trader ./provider/local
go test ./...
```

- Services restarted in `tmux ait-dev` at 13:46 CST.
- API health check after restart: `{"status":"ok","time":null}`.
- Initial snapshot after restart: `525 symbols`, `15.46058229s`.
- #277 decision tokens: `6086/1072/7158`, AI call `9.843s`.
- PARTIUSDT execution: short `490` contracts at `0.0543`, leverage `7x`, SL `0.05539`, TP `0.05212`.
- Order sync confirmed two fills: `291` + `199` contracts, both at `0.054300`, total fee `0.0133035`.

### Current State

- VVV now has one open position: `PARTIUSDT SHORT`, entry `0.0543`, qty `490`, leverage `7x`.
- Latest synced local position id: `127`, status `OPEN`.
- Next item: monitor whether PARTIUSDT exits by TP/SL/protector, and confirm future BANK/CLO-like funding shorts with stop distance far below `2%` remain WATCH before LLM.

## 14:28 CST Follow-up: PARTIUSDT Stop-out and Weak 4h OI Flush Filter

### Live Cycles

| Cycle | Result | Finding |
| --- | --- | --- |
| #278 | `PARTIUSDT hold` | AI held the short while price had moved against entry but remained below planned SL. Reasoning noted peak-to-loss giveback, but no executable close was chosen. |
| #279 | `PARTIUSDT hold` | AI continued to hold: 15m/1h were still below EMA20 and price had not reached planned SL/TP. |
| #280 | AI skipped | After PARTI closed, scan returned to flat state with no `EXECUTABLE/REVIEWABLE` candidates; AI skipped with `prompt_tokens=0`. |

### PARTIUSDT Result

- Entry: #277 `PARTIUSDT SHORT`, fill `490` contracts at `0.0543`, leverage `7x`.
- Planned protection: SL repaired to about `0.05539`, TP about `0.05212`.
- Exit: exchange stop closed the position at average `0.05540809`.
- DB result: position `127` is `CLOSED`, realized PnL `-0.54296`, fee `0.02687848`, close reason `sync`.
- Hold cycles: #278 and #279 did not create execution errors; the loss was a signal-quality problem, not an order-management failure.

### Root Cause

This should not be treated as a blanket failure of the `funding_reversal SHORT` path. The profitable shorts had deeper higher-timeframe unwind:

- `MANTAUSDT` #228: 4h OI flush about `-6.13`, closed `+0.102028`.
- `BSBUSDT` #249: 4h OI flush about `-14.50`, strong taker sell, closed `+0.17108`.
- `OPENUSDT` #260/#268: better confirmation; #268 had 4h OI about `-3.15`, closed `+0.1027`.

PARTI was weaker:

- Confidence was only `C`.
- It carried `not_near_short_retest_zone`, meaning price was not at the preferred short retest area.
- 4h OI flush was shallow: `OIDelta4h=-0.50`, while 1h OI was already `-3.80`.
- Volume confirmation was weak: `15m_vol_vs_avg5=0.16x`, `5m_vol_vs_avg5=0.90x`.

Interpretation: the setup had short-term deleveraging, but not enough 4h crowd flush to justify opening a lower-zone C-grade funding short. The pattern is "late/lower-zone weak flush", not "funding short unusable".

### Fix Added

24. Added provider-side downgrade for weak funding shorts:
   - setup `funding_reversal`, direction `SHORT`, confidence `C`;
   - has `not_near_short_retest_zone`;
   - missing snapshot or `OIDelta4h > -1.0`;
   - downgraded to `WATCH`, reason `funding_short_weak_4h_flush_wait`, tag `weak_4h_oi_flush`.
25. This keeps PARTI-like lower-zone weak-flush candidates out of LLM/open review while preserving stronger 4h-flush funding shorts such as MANTA, BSB, and OPEN.
26. Added regression coverage:
   - `TestFundingShortWeak4hFlushNearLowerZoneStaysWatch`;
   - `TestFundingShortStrong4hFlushAvoidsWeakFlushTag`.

### Verification / Deployment

```bash
go test ./provider/local
go test ./kernel ./trader ./provider/local
go test ./...
```

- Passed.
- Service restarted in `tmux ait-dev` after PARTI was flat.
- Backend health check after restart returned `200`.
- Backend and frontend are listening on `:8080` and `:3000`.
- Current DB status: VVV has no open positions; latest equity snapshot is `8.05053051`, balance `8.05053051`, position count `0`.
- Account return from initial `10.71079974` to latest `8.05053051` is about `-24.84%`.

### Runtime Status Note

- After the restart, the service loaded trader `VVV`, but `traders.is_running=0`; no auto-start log appeared.
- This means deployment is live, but the trading loop is not currently confirmed running.
- Do not evaluate new-cycle strategy behavior until VVV is explicitly started again or the DB running flag is restored before a restart.

### Next Monitoring Items

- Confirm future PARTI-like candidates are downgraded before LLM with `weak_4h_oi_flush`.
- Confirm strong funding shorts with deeper 4h OI flush still reach `REVIEWABLE` or `EXECUTABLE`.
- Continue separating strategy-quality losses from mechanical execution failures: #277 opened and stopped correctly, while #275 was a preflight geometry failure now filtered earlier.

## 14:31 CST Follow-up: VVV Restart Confirmed and #281 First Cycle

### Runtime

- VVV was started from the API/UI at 14:26:58 CST.
- DB status is now `traders.is_running=1`, scan interval `10m`.
- Logs confirmed:
  - `AI-driven automatic trading system started`;
  - initial Binance order sync ran;
  - `SnapshotEngine started`;
  - initial decision cycle completed;
  - automatic scan loop entered.
- Account after restart: equity about `8.05`, available about `8.05`, no open positions, account PnL about `-24.84%/-24.85%`.

### #281 Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #281 | AI skipped | Valid first-cycle skip after restart: 3 WATCH candidates, 0 `EXECUTABLE/REVIEWABLE`, no LLM call, no order. |

### #281 Candidate Review

Hunter v7 scanned `98` symbols and output 3 signals:

- `BSBUSDT LONG leader_momentum_long`: setup `80.4`, risk `8`, timing `63`, liquidity `100`, priority `75.2`, status `candidate`.
- `VELVETUSDT LONG leader_momentum_long`: setup `80.4`, risk `15`, timing `63`, liquidity `85`, priority `73.0`, status `candidate`.
- `LTCUSDT LONG trend_breakout_long`: setup `61.6`, risk `8`, timing `45`, liquidity `90`, priority `52.1`, status `candidate`.

Runtime tiering saved `ALL wait` with `no_open_review_candidates watch=3 rejected=0`, `prompt_tokens=0`, `total_tokens=0`.

### Interpretation

- This is not LLM WAIT 空转; the router skipped before AI.
- BSB/VELVET had strong setup and priority, but leader-momentum open/review currently requires stronger timing than `63`; this is consistent with the post-EPIC/low-timing momentum tightening.
- LTC timing `45` is too weak for trend-breakout review.
- No change is justified from #281 alone. It confirms the restarted loop is active and the current stricter momentum gate is working.

### Next Monitoring Items

- Watch the next scheduled scan around 14:37 CST for whether warmed snapshots produce any valid `REVIEWABLE/EXECUTABLE`.
- If BSB/VELVET timing strengthens to `65+` with clean flow, verify they can reach review/open; if timing stays below threshold, keep them WATCH.
- Confirm `weak_4h_oi_flush` appears on future PARTI-like C-grade funding shorts before they enter LLM.

## 14:37 CST Follow-up: #282 Warmed Skip and #260-#282 Conversion Review

### Live Cycle

| Cycle | Result | Finding |
| --- | --- | --- |
| #282 | AI skipped | Valid warmed-cycle skip: BLUAI/BSB were WATCH only, with 0 `EXECUTABLE/REVIEWABLE`; no LLM call, no order. |

### #282 Details

- #282 ran at 14:36:58-14:36:59 CST after VVV had restarted and the scan ticker fired normally.
- Account was flat: equity about `8.04975914`, available about `8.04975914`, no open positions.
- Hunter v7 returned 2 candidate symbols: `BLUAIUSDT`, `BSBUSDT`.
- Runtime decision: `ALL wait`, reason `no_open_review_candidates watch=2 rejected=0`.
- LLM tokens: `0`; cycle saved successfully; no Binance order sync changes.

### #260-#282 Metrics

| Metric | Value |
| --- | ---: |
| Cycles reviewed | 23 |
| LLM-called cycles | 16 |
| Pre-LLM skipped cycles | 7 |
| Avg LLM duration when called | `9.48s` |
| Total prompt tokens | `86,655` |
| Total LLM tokens | `101,467` |
| Open-short intents | 4 |
| Successful opens | 3 |
| Execution-blocked opens | 1 |
| Closed live trades | 3 |
| Trade win rate | `66.7%` |
| Realized PnL, trades #125-#127 | `-0.28816 USDT` |

### Trade Split

| Position | Setup | Result | Lesson |
| ---: | --- | ---: | --- |
| #125 `OPENUSDT SHORT` | funding reversal | `+0.15210` | REVIEWABLE funding-short path valid; management was initially too passive near TP1. |
| #126 `OPENUSDT SHORT` | funding reversal | `+0.10270` | Re-entry worked and protector closed correctly around TP1. |
| #127 `PARTIUSDT SHORT` | funding reversal | `-0.54296` | Loss came from weak 4h OI flush / lower-zone C-grade short, now filtered by `weak_4h_oi_flush`. |

### Optimization Decision

- No new threshold change from #282.
- Keep low-timing `leader_momentum_long` as WATCH until timing/flow reaches review quality.
- Keep selective `funding_reversal SHORT` REVIEWABLE path active, but preserve the new weak 4h flush downgrade after PARTI.
- Treat WATCH-only 0-token cycles as successful cost control, not harmful WAIT空转.

### Current State

- VVV is running: `traders.is_running=1`, scan interval `10m`.
- Current account is flat, no open positions.
- Latest equity snapshot: `8.04975914` at 14:36:59 CST.
- Latest meaningful defect remains PARTI-like weak funding shorts, already addressed by provider-side downgrade.

## Optimization Addendum: Clean Momentum REVIEWABLE Recall

### Reason

The recent records show that Hunter v7 is not failing at the universe level: raw scans still produce multiple setup candidates. The bottleneck is runtime tiering. After the EPIC/UB lessons, `leader_momentum_long` was tightened so strongly that high-priority but slightly sub-ready names such as #281 `BSBUSDT` and `VELVETUSDT` became WATCH-only and skipped the LLM entirely.

This reduced bad chase entries, but it also lowered open-review coverage too much.

### Implementation

Added a narrow `leader_momentum_long` REVIEWABLE path:

- `ai_priority >= 72`;
- `setup_score >= 80`;
- `timing_score >= 62`;
- `risk_score < 25`;
- known liquidity at least `80`;
- 15m taker buy is present and at least `0.52`;
- reason codes show clean continuation/pullback;
- no weak taker, extreme funding, crowded, overheated, or chase-risk tags.

New tier reason: `momentum_reviewable_high_priority_pullback`.

### Expected Effect

- Increase LLM review opportunities when a true high-priority momentum leader appears.
- Keep ordinary low-timing momentum as WATCH.
- Keep previous loss patterns blocked: weak-taker UB-like momentum, overheated EPIC-like momentum, and PARTI-like weak funding shorts.

### Verification

```bash
go test ./kernel
go test ./provider/local ./trader
go test ./...
```

All passed.

Detailed implementation record: `reports/hunter-v7-clean-momentum-reviewable-20260608.md`.

## Root Fix Addendum: Restore #20-#83 High-Win Recall

### Root Cause

The #20-#83 high-win phase had a different architecture balance:

- Hunter v7 provided broader recall, usually 5-10 candidates.
- LLM performed setup-specific confirmation and still produced `wait` when candidates were weak.
- The strongest edge was `panic_reversal_long`, including some `timing_score=30-40` candidates when reclaim/taker/OI evidence was strong.

Later fixes correctly blocked real execution failures, but also moved too much of the LLM review job into runtime WATCH filters. As a result, many historically valid candidates never reached the LLM, causing low open rate without necessarily improving win rate.

### Fix Implemented

1. Restored a narrow #20-#83 panic-reversal review pool:
   - `panic_reversal_long`;
   - `ai_priority >= 50`;
   - `setup_score >= 55`;
   - `timing_score >= 30`;
   - strong/solid/early reclaim;
   - visible 15m taker buy `>= 0.52`;
   - at least three high-win confirmations from taker recovery, OI flush/decline, selling exhaustion/deceleration, 1h green shoot, RSI recovery, or A/B confidence.
   - New tier reason: `panic_reversal_reviewable_high_win_reclaim`.

2. Separated mixed-OI funding shorts from pure OI building:
   - pure OI building stays WATCH;
   - mixed OI (`1h OI > 0`, `4h OI < 0`) can be REVIEWABLE only with strong taker sell and long crowding.

3. Kept the clean momentum REVIEWABLE path from the previous addendum.

### Verification

```bash
go test ./kernel
go test ./provider/local ./trader
go test ./...
```

All passed.

Detailed root-fix report: `reports/hunter-v7-cycle20-83-regression-root-fix-20260608.md`.

### Strategy Prompt Sync

The live strategy prompt in `data/data.db` was updated to match the runtime root fix.

- Before backup: `reports/strategy-hunter-v7-before-prompt-root-fix-20260608.json`
- After export: `reports/strategy-hunter-v7-after-prompt-root-fix-20260608.json`
- Key correction: removed the contradiction where `panic_reversal_long timing_score < 45` defaulted to wait. The prompt now says `panic_reversal_reviewable_high_win_reclaim` can be reviewed at timing `30-40` if reclaim, taker buy, OI/selling exhaustion, and RR confirm.
- REVIEWABLE now explicitly means "可开仓复核池", not automatic wait.

## 15:25 CST Follow-up: #287 REVIEWABLE -> LLM -> Wait

### Chain Result

| Step | Result |
| --- | --- |
| Hunter v7 recall | 5 candidates |
| Tier split | `EXECUTABLE=2`, `REVIEWABLE=1`, `WATCH=2`, `REJECTED=0` |
| LLM call | Yes, `25.224s`, `10094` prompt tokens, `2513` completion tokens |
| Decision | `ALLOUSDT wait`, `BLESSUSDT wait`, `GPSUSDT wait` |
| Orders | None |
| Positions | None |
| PnL | `0`, no trade opened |

### Candidate Review

| Symbol | Tier | Setup | LLM Decision | Assessment |
| --- | --- | --- | --- | --- |
| `ALLOUSDT LONG` | `EXECUTABLE` / `momentum_ready_strong_flow` | `leader_momentum_long` | wait | Reasonable wait. It was only `0.07%` above entry-zone upper, but multi-timeframe RSI was overbought (`1h RSI7=82.7`), micro volume was weak (`5m vol_vs_avg5=0.20x`, `15m=0.11x`), and momentum showed `15m_no_new_high=true`. This is an overheat/low-volume momentum-chase risk. |
| `BLESSUSDT LONG` | `EXECUTABLE` / `momentum_ready_strong_flow` | `leader_momentum_long` | wait | Correct hard wait. Current-to-invalidation distance was `1.94%`, below the Hunter v7 min stop distance `2.00%`. Opening would likely fail execution guard or produce bad geometry. |
| `GPSUSDT SHORT` | `REVIEWABLE` / `funding_short_reviewable_crowding_reversal` | `funding_reversal` | wait | Correct wait. It reached the LLM as intended, but price was already below the short entry zone (`entry_zone_pos=-84.7%`), RSI was deeply oversold (`15m RSI7=18.6`, `1h RSI7=10.6`), confidence was `C`, and `not_near_short_retest_zone` was present. This was a late chase, not a retest entry. |
| `BSBUSDT LONG` | `WATCH` | `leader_momentum_long` | not expanded | Correctly kept as WATCH due to `chase_risk_wait_reentry`. |
| `BANKUSDT LONG` | `WATCH` | `leader_momentum_long` | not expanded | Correctly kept as WATCH due to `needs_confirmation`. |

### Diagnosis

#287 validates the prompt/code alignment fix: `REVIEWABLE` candidates are no longer skipped before LLM. The LLM did not open because the only REVIEWABLE candidate was a poor late-entry short, while both EXECUTABLE momentum longs had valid execution-quality concerns.

No further loosening is recommended from #287. The next improvement should be measurement, not threshold relaxation: track realized PnL by `tier_reason` once a post-fix `REVIEWABLE` candidate actually opens.

## 16:55 CST Follow-up: #288-#297 Signal Quality Fix

### Recent Cycle Summary

| Cycle | Tier split | LLM / execution result | Diagnosis |
| --- | --- | --- | --- |
| #288 | `EXECUTABLE=2`, `REVIEWABLE=0`, `WATCH=5` | ALLO/VELVET wait | Open-review path worked, but momentum candidates were extended/low-quality. |
| #289 | no open-review | skipped | Valid skip. |
| #290 | `EXECUTABLE=2`, `REVIEWABLE=0`, `WATCH=4`, `REJECTED=1` | VELVET/BLUAI wait | Open-review path worked; LLM rejected execution quality. |
| #291 | `EXECUTABLE=2`, `REVIEWABLE=0`, `WATCH=6` | MEGA/VELVET wait | Open-review path worked; LLM rejected execution quality. |
| #292-#294 | no open-review | skipped | Valid skips, but confirms recall is still unstable in current market regime. |
| #295 | `EXECUTABLE=0`, `REVIEWABLE=1`, `WATCH=0`, `REJECTED=1` | DASH open_short blocked | Important failure: weak C-grade funding short reached LLM, AI output confidence `65`, backend rejected because Hunter v7 effective minimum is `70`. |
| #296 | `EXECUTABLE=0`, `REVIEWABLE=2`, `WATCH=3`, `REJECTED=1` | CLO/BTW wait | Good behavior: REVIEWABLE reached LLM, but both were not clean enough to open. |
| #297 | `EXECUTABLE=0`, `REVIEWABLE=1` | BTW wait | Correct wait: short setup had confirmations, but RSI was deeply oversold and RR was marginal. |

### Root Cause Found

The current problem is not the old "no REVIEWABLE reaches LLM" failure. The path is now mostly working.

Two newer quality issues were found:

1. System prompt mismatch:
   - hard/open minimum for Hunter v7 is effectively `confidence >= 70`;
   - prompt still said "Low confidence (60-69): Use 30-50%";
   - #295 followed that stale sizing rule and emitted `confidence=65`, which the backend correctly rejected.

2. Weak funding-short noise:
   - DASH had `confidence=C`, `ai_priority=50.05`, `not_near_short_retest_zone`, `weak_4h_oi_flush`, and `funding_short_weak_4h_flush_wait`;
   - runtime tiering still promoted it to `REVIEWABLE`;
   - this spent an LLM call and produced an invalid open attempt.

### Fix Implemented

1. Prompt confidence guidance now uses the same effective minimum as backend execution.
   - For Hunter v7, `Min Confidence: >=70`.
   - Confidence below `70` must output `wait`; it cannot be opened by shrinking position size.
   - Removed the misleading low-confidence `60-69` open sizing guidance when the strategy requires `>=70`.

2. Added a DASH-like weak funding short downgrade:
   - `funding_reversal SHORT`;
   - `confidence=C` or `ai_priority < 60`;
   - `not_near_short_retest_zone`;
   - plus `weak_4h_oi_flush` or `funding_short_weak_4h_flush_wait`;
   - now becomes `WATCH` with reason `funding_short_weak_4h_flush_retest_wait`.

This keeps the #20-#83 high-win recall restoration intact for panic-reversal and strong funding shorts, while removing a known low-quality C-grade funding short edge case.

### Verification and Deployment

```bash
go test ./kernel
go test ./provider/local ./trader
go test ./...
```

All passed.

The dev service was restarted at `16:54-16:55 CST`; backend log confirms VVV reloaded strategy `猎手v7` and initialized successfully.

## 23:29 CST Follow-up: #300-#310 Momentum Quality and Attribution Fix

### Post-Restart Cycle Review

| Cycle | Tier split | Result | Assessment |
| --- | --- | --- | --- |
| #300 | `EXECUTABLE=1` | `SAHARAUSDT wait` | Correct wait: stop distance only `1.55%`, below hard `2.00%`. |
| #301 | `REVIEWABLE=1` | `BTWUSDT wait` | Correct wait: RR below `1.5`, deeply oversold, poor short entry timing. |
| #302 | no open-review | skipped | Valid skip. |
| #303 | `REVIEWABLE=1` | `EPICUSDT wait` | Valid LLM review; not a primary setup and quality was insufficient. |
| #304-#305 | no open-review | skipped | Valid skips. |
| #306 | `EXECUTABLE=1` | `AIOTUSDT open_long` blocked | Signal path worked, but Binance rejected leverage request with timestamp/recvWindow error. Separately, the signal itself had momentum-quality concerns: zone upper, low 15m volume, elevated funding, 1h/4h RSI overheat. |
| #307-#309 | no open-review | skipped | Valid skips. |
| #310 | `EXECUTABLE=1` | `SAHARAUSDT open_long`, then stopped | Open succeeded, but closed at `0.03863` for `-0.30832` USDT. This exposed that `leader_momentum_long` was too willing to treat upper-zone shallow pullbacks as executable. |

### Attribution Correction

Two ALLOUSDT SHORT trades appeared in VVV positions:

| Time | Result |
| --- | --- |
| 18:44-18:48 CST | `ALLOUSDT SHORT`, `-2.88394` USDT |
| 19:14-19:15 CST | `ALLOUSDT SHORT`, `+0.78062` USDT |

These trades do **not** have matching `decision_records` open decisions. Backend logs only show Binance `order_sync` importing external fills, and the order records have empty `client_order_id`.

Conclusion: these ALLO trades must be excluded from Hunter v7 signal-quality attribution unless a matching AI decision is found. They affected VVV equity and the recent-trades context shown to the LLM, but they are not evidence that Hunter v7 selected ALLO.

### Root Cause Found

The main remaining Hunter v7 quality issue is `leader_momentum_long` over-promoting late upper-zone pullbacks:

- `SAHARAUSDT` at #310:
  - `entry_zone_pos=79.7%`;
  - `Change1h=-0.78%`;
  - `taker_buy_15m=0.553`;
  - `15m_no_new_high=true`;
  - `15m_vol_vs_avg5=0.61x`;
  - opened, then stopped out.
- `AIOTUSDT` at #306:
  - `entry_zone_pos=93.1%`;
  - `Change1h=-0.89%`;
  - `taker_buy_15m=0.510`;
  - `15m_vol_vs_avg5=0.25x`;
  - `1h/4h RSI > 80`;
  - backend failed before order due timestamp, but the signal was also a poor quality momentum chase.

### Fix Implemented

Added a runtime downgrade for late leader-momentum pullbacks:

- setup is `leader_momentum_long`;
- direction is LONG;
- price is at upper part of entry zone (`entry_zone_pos >= 70%`);
- signal is a 1h/shallow/micro pullback;
- taker buy is not clearly strong (`taker_buy_15m < 0.58`);
- downgrade to `WATCH` with reason `momentum_late_pullback_zone_upper_wait`.

The system prompt also now explicitly tells the LLM:

- do not treat weak upper-zone pullbacks as quality long entries;
- wait for mid/lower-zone pullback or renewed high-volume breakout.

### Verification and Deployment

```bash
go test ./kernel
go test ./provider/local ./trader
go test ./...
```

All passed.

The dev service was restarted again at `23:29 CST`; backend log confirms VVV reloaded strategy `猎手v7`.

## 23:50 CST Follow-up: Full Hunter v7 Pattern/Tag Audit

### Audit Conclusion

The remaining risk was not that Hunter v7 lacked enough modules. The bigger issue was that several modules already emitted correct danger tags, but final tiering did not always treat those tags as open-blocking. That could let high `ai_priority`, `setup_score`, or generic `REVIEWABLE` rules override the actual market warning and hand a misleading candidate to the strategy LLM.

### Pattern Review

| Setup | Keep / tighten | Decision |
| --- | --- | --- |
| `panic_reversal_long` | Keep high-win reclaim path | `strong/solid/early_reclaim` plus taker/OI exhaustion remains `REVIEWABLE`; `no_reclaim_signal` and `oi_up_price_down` are now wait-only. |
| `funding_reversal SHORT` | Keep #20-#83 style strong crowding reversal | Strong sell flow with real flush/failed rebuild remains reviewable; late short after a fast/deep drop without OI reset is now wait-only. |
| `funding_reversal LONG` | Tight only | Long side still needs stronger timing; late long after pump without OI reset is wait-only. |
| `leader_momentum_long` | Keep strong-flow leader entries | Prior upper-zone weak-pullback fix remains; strong taker flow can still pass, weak upper-zone pullbacks stay WATCH. |
| `short_squeeze_long` | Tighten exhaustion tags | `already_pumped_24h`, `funding_expensive`, `lsr_extreme_long` now block open-review. |
| `accumulation_breakout_long` | Tighten sell-flow contradiction | `taker_sell_during_accumulation` now means wait-only, not reviewable breakout. |
| `pullback_reversal_long` | Keep strong support structure | Clean support/taker recovery stays `REVIEWABLE`; `not_near_long_reclaim_zone` blocks open-review. |
| `distribution_short` / `long_squeeze_short` / `range_reversion` | Tighten entry location | If the signal is not near the required short retest/range zone, it stays WATCH instead of generic `short_or_reversion_reviewable`. |

### Fix Implemented

1. Added `hunterV7HighRiskSignalWaitReason()` in runtime tiering:
   - `panic_reversal_no_reclaim_wait`
   - `panic_reversal_oi_up_price_down_wait`
   - `funding_reversal_late_chase_no_flush`
   - `short_squeeze_crowded_or_exhausted_wait`
   - `accumulation_sell_flow_wait`
   - `pullback_long_reclaim_zone_wait`
   - `short_reversion_retest_zone_wait`

2. Expanded hard danger tags used before `EXECUTABLE` / `REVIEWABLE` promotion:
   - `already_pumped_24h`
   - `lsr_extreme_long`
   - `funding_expensive`
   - `late_short_after_deep_drop`
   - `short_after_fast_drop_without_flush`
   - `late_long_after_deep_pump`
   - `long_after_fast_pump_without_flush`
   - `taker_sell_during_accumulation`
   - `no_reclaim_signal`
   - `oi_up_price_down`
   - `not_near_long_reclaim_zone`

3. Updated Hunter v7 execution prompt so the LLM is told these risk tags are wait-only semantics and must not be overridden by high priority/score.

4. Added regression coverage for six high-risk false-open paths:
   - late funding short chase without OI flush;
   - exhausted short squeeze long;
   - accumulation breakout with active sell flow;
   - distribution/range short away from retest zone;
   - panic long without reclaim;
   - pullback long above reclaim zone.

### Verification

```bash
go test ./kernel
go test ./provider/local ./trader
go test ./...
```

All passed.

The dev service was restarted at `23:45 CST`; backend log confirms VVV reloaded strategy `猎手v7`. Current VVV account state before restart and after startup check: no `OPEN` positions.

### Operating Impact

This preserves the recovered open-rate channels from #20-#83 style behavior: strong panic reclaim, strong funding short reversal, clean pullback, and clean momentum leaders. The change removes the main class of LLM-misleading candidates: signals where Hunter v7 already knows the structure is late, crowded, unreclaimed, sell-flow contradictory, or away from the required entry zone.
