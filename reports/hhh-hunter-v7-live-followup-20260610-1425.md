# HHH Hunter v7 Live Follow-up - 2026-06-10 14:25 CST

## Scope

- Followed the latest HHH live cycles after the LAB counter-trend losses.
- Checked whether the new idle cycles were caused by data starvation or by execution-tier gating.
- Reviewed the MOVE exits that closed with only small gross profit.

## Findings

### 1. The data source is alive; the idle point was tier gating

The feed kept refreshing normally: 525 symbols from REST, 135-symbol Hunter v7 universe, and live signal records were continuously written.

Before this patch, cycle 14 at `2026-06-10 14:14:50 CST` produced 9 output candidates but all were tiered as `WATCH`, so the backend skipped AI with:

`Hunter v7 no EXECUTABLE/REVIEWABLE candidates; AI skipped`

The best two missed candidates were `BABYUSDT` and `VELVETUSDT`, both `leader_momentum_long`, `ready`, low risk, high timing, high liquidity, and inside the entry zone. They were demoted to `WATCH/needs_confirmation` mainly because their setup scores did not meet the older hard open/review floors.

### 2. LAB blocking is now behaving as intended

LAB remained blocked after the earlier counter-trend fix. In the latest cycles it stayed `watch_only` or priority-filtered due to low timing, trend-down structure, and weak 4h/24h context. That is correct; relaxing LAB-style panic reversals would reintroduce the same knife-catch loss mode.

### 3. MOVE was not closed by LLM or structural reversal

The two MOVE closes were caused by the position protector, not by an AI `close_long`.

Root cause:

- PnL protection used leveraged PnL.
- At 20x leverage, a raw price move around 0.3% can display as about 6% PnL and trigger TP1.
- Small account notional made a 40% TP1 partial close invalid under min-notional logic, so TP1 became full close.
- The market close then filled near entry, leaving only tiny net profit after fees.

## Changes

### Open-rate improvement without reopening LAB risk

Added `momentum_reviewable_relative_strength_floor`:

- Applies only to `leader_momentum_long`.
- Promotes strong relative-strength momentum from `WATCH` to `REVIEWABLE`, not direct `EXECUTABLE`.
- Requires `ready`, low risk, high liquidity, timing >= 65, taker >= 0.50, 4h/24h momentum, OI growth, and `strong_symbol_regime_override`.

This lets the LLM review BABY/VELVET-like setups for live 5m/15m confirmation, entry zone, stop distance, and RR, while keeping weak/overheated momentum and panic-reversal knife catches blocked.

### Win-rate/net-profit protection

Added raw price-move floors to the position protector:

- TP1 protection requires leveraged PnL >= 6% and unleveraged price move >= 1.0%.
- TP2 protection requires leveraged PnL >= 12% and unleveraged price move >= 1.5%.
- Near-TP/giveback profit exits also require the 1.0% raw move floor unless the position has fallen into the configured material-loss exit path.

This prevents high leverage from converting tiny raw moves into premature full closes.

## Live Verification

After restart at `2026-06-10 14:24:49 CST`, initial cycle #204 no longer skipped AI:

- Hunter v7 produced 4 candidates.
- 1 candidate was `EXECUTABLE`.
- AI was called with 9,220 input tokens.
- Decision was `wait` with real blocker `confirmation_missing`.

The blocker was valid:

- `SLXUSDT` 5m close was still below EMA20.
- Stop distance was 1.96%, slightly below the 2.00% minimum.

So the system moved from silent idle to explicit review-and-block behavior.

## Verification Commands

- `go test ./kernel ./trader -run 'TestClassifyHunterV7CandidateTierAllowsRelativeStrengthMomentumReview|TestChoosePositionProtectionActionDoesNotTP1OnLeveragedMicroMove|TestCalculateUnleveragedPnLPctLongAndShort|TestChoosePositionProtectionAction' -count=1`
- `go test ./trader ./trader/binance ./kernel ./provider/local -count=1`
- `go build ./...`

