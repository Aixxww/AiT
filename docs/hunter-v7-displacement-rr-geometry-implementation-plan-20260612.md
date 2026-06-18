# Hunter v7 Displacement RR Geometry Implementation Plan

> Date: 2026-06-12  
> Scope: `displacement_momentum_long` execution geometry, live tier conversion, and profitability follow-up  
> Principle: improve valid openable signal supply by fixing target/stop geometry, not by bypassing backend risk controls.

## 1. Background

The 8-round live validation showed that the previous `leader_momentum_long` reviewability issue was materially improved. The system produced several valid review candidates, and the Round 1 XPLUS long direction later tracked +13.23% in Round 2 and +17.04% in Round 3.

The remaining blocker moved from signal discovery to execution geometry. In Round 8, multiple `displacement_momentum_long` candidates had strong raw setup/readiness characteristics, but prompt/live tiering rejected them because the module emitted `displacement_rr_insufficient`.

Observed Round 8 pattern:

- Displacement candidates appeared in quantity during rotational momentum.
- Most were rejected before becoming REVIEWABLE because local module RR validation used only the first target.
- The first target was often too close relative to a structural 1h-low stop, even when later continuation targets were directionally valid.
- Backend and trader risk checks already recalculate RR from live price and capped geometry, so the module-level early reject was overly lossy.

## 2. Problem Statement

`displacement_momentum_long` currently combines:

- Pullback entry zone around current price.
- Structural invalidation below 1h low or a fallback ATR stop.
- T1 at only `current + 1.5 ATR`.
- Local RR validation against T1 only.

This creates a false negative path:

1. Strong momentum candidate forms.
2. Structural stop is wider than the first continuation target.
3. Local module marks `invalid_rr`.
4. `displacement_rr_insufficient` is a reject-only risk tag.
5. Prompt/live classification rejects the symbol even if later extension targets or tightened execution stop would be feasible.

The desired behavior is:

- Reject only when no positive target can satisfy minimum RR after realistic geometry repair.
- Preserve liquidity, chase-risk, backend RR, stop-loss and take-profit direction checks as hard gates.

## 3. Objectives

- Reduce false `displacement_rr_insufficient` rejections caused by T1-only validation.
- Generate displacement targets that match continuation behavior: scalable T1/T2/T3 instead of a single conservative first target.
- Use structural invalidation where reasonable, but avoid stops so wide that the setup becomes mathematically impossible before backend execution can evaluate it.
- Keep all backend and auto-trader hard controls unchanged.
- Validate by unit tests, full build, and one fresh live run.

## 4. Non-Goals

- Do not lower global RR requirements.
- Do not make low-liquidity displacement symbols openable.
- Do not remove `displacement_rr_insufficient` as a reject-only tag.
- Do not force multiple open signals when the market lacks valid geometry.
- Do not change short-side funding/distribution confirmation gates in this phase.

## 5. Implementation Plan

### Phase A - Geometry Refactor

Implement a shared displacement geometry path:

- Entry zone:
  - Keep pullback/trailing intent.
  - Anchor lower bound to VWAP when available.
  - Keep upper bound near current price to avoid chase entries.

- Invalidation:
  - Prefer recent structural low when it is within a feasible execution-risk envelope.
  - If the 1h low is too far, use a near-structure execution stop derived from price percentage and ATR.
  - Tag tightened structural fallback through the invalidation reason instead of hiding the adjustment.

- Targets:
  - T1: practical first continuation target.
  - T2: RR-feasible continuation extension.
  - T3: volatility/range expansion extension.
  - Include 4h high only if it is still above current price.
  - Sort and deduplicate valid long targets.

### Phase B - RR Validation

Update local displacement RR validation:

- Use the best positive target, not only T1.
- Validate from entry-zone midpoint with fallback to current price.
- Require minimum RR >= 1.5.
- Attach `displacement_rr_repaired` when the geometry passes via later target or tightened stop.
- Keep `displacement_rr_insufficient` only when all targets fail.

### Phase C - Tests

Add focused tests:

- Strong displacement with wide first target problem passes when a later target provides RR.
- Infeasible displacement still receives `displacement_rr_insufficient`.
- Final execution scoring remains able to produce non-invalid RR after target promotion/tightening.

### Phase D - Verification

Run:

```bash
go test ./provider/local ./kernel ./trader
go test ./...
go build ./...
```

Then run a fresh live validation:

```bash
go run ./cmd/hunter_v7_validate --rounds 1 --top-detail 220 --max-workers 8 --max-output 40
```

Evaluation metrics:

- `displacement_rr_insufficient` count should fall when targets are geometrically feasible.
- REVIEWABLE/EXECUTABLE displacement candidates must still pass backend RR and live-price checks.
- No low-liquidity or invalid SL/TP candidates should become openable.
- Report must include whether any open-review signal has follow-up tracking baseline.

## 6. Acceptance Criteria

- Unit tests pass for provider/local, kernel, and trader packages.
- Full `go test ./...` passes.
- Full `go build ./...` passes.
- Fresh live run completes with valid artifacts.
- Any newly openable displacement signal must have:
  - valid long target above live price,
  - valid invalidation below live price,
  - backend RR >= configured threshold,
  - liquidity score above prompt threshold or explicit backend acceptance,
  - no reject-only risk tag.

## 7. Rollback Criteria

Rollback or tighten if live validation shows:

- Displacement REVIEWABLE candidates frequently fail backend RR.
- Low-liquidity names become REVIEWABLE through geometry repair.
- Targets are unrealistically far relative to configured max take-profit distance.
- Follow-up tracking shows immediate invalidation or systematic direction failure.

