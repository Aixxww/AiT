package local

import (
	"math"
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixture builders
// ---------------------------------------------------------------------------

// flatBars builds n bars with constant close/high/low (constant TR).
func flatBars(startMs, stepMs int64, n int, close, high, low float64) []V7StructureBar {
	bars := make([]V7StructureBar, n)
	for i := range bars {
		bars[i] = V7StructureBar{
			OpenTimeMs: startMs + int64(i)*stepMs,
			Open:       close, High: high, Low: low, Close: close,
		}
	}
	return bars
}

const (
	fixStartMs = int64(1700000000000)
	fixStep15m = int64(900000)
	fixStep1h  = int64(3600000)
)

// longFixture builds the LONG geometry fixture (E = 100):
//   - 15m: 200 flat bars (close 100, high 100.4, low 99.6) with sculpted
//     pivot low 96.0 @140, pivot low 98.0 @150, pivot high 103.0 @160,
//     pivot highs 105.0 @170 and @180 (cluster, 2 touches).
//   - 1h: 200 flat bars (close 100, high 100.6, low 99.4) with pivot
//     highs 104.6 @140 and 104.8 @160 (a wall within the 5% TP cap)
//     and 108.0 @150 and 108.3 @170 (a second wall, 2 touches).
func longFixture() (sBars, hBars []V7StructureBar) {
	sBars = flatBars(fixStartMs, fixStep15m, 200, 100, 100.4, 99.6)
	sBars[140] = V7StructureBar{OpenTimeMs: fixStartMs + 140*fixStep15m, Open: 98.4, High: 98.8, Low: 96.0, Close: 97.2}
	sBars[150] = V7StructureBar{OpenTimeMs: fixStartMs + 150*fixStep15m, Open: 99.2, High: 99.6, Low: 98.0, Close: 98.6}
	sBars[160] = V7StructureBar{OpenTimeMs: fixStartMs + 160*fixStep15m, Open: 101.0, High: 103.0, Low: 102.2, Close: 102.6}
	sBars[170] = V7StructureBar{OpenTimeMs: fixStartMs + 170*fixStep15m, Open: 103.0, High: 105.0, Low: 104.2, Close: 104.6}
	sBars[180] = V7StructureBar{OpenTimeMs: fixStartMs + 180*fixStep15m, Open: 103.0, High: 105.0, Low: 104.2, Close: 104.6}

	// The 1h series shares the 15m timeline: its last bar closes at
	// the same decision time (start + 200*15m).
	hStart := fixStartMs + 200*fixStep15m - 200*fixStep1h
	hBars = flatBars(hStart, fixStep1h, 200, 100, 100.6, 99.4)
	hBars[140] = V7StructureBar{OpenTimeMs: hStart + 140*fixStep1h, Open: 101.0, High: 104.6, Low: 104.0, Close: 104.3}
	hBars[160] = V7StructureBar{OpenTimeMs: hStart + 160*fixStep1h, Open: 101.0, High: 104.8, Low: 104.0, Close: 104.5}
	hBars[150] = V7StructureBar{OpenTimeMs: hStart + 150*fixStep1h, Open: 104.0, High: 108.0, Low: 103.4, Close: 106.0}
	hBars[170] = V7StructureBar{OpenTimeMs: hStart + 170*fixStep1h, Open: 104.0, High: 108.3, Low: 103.4, Close: 106.0}
	return sBars, hBars
}

// mirrorBars mirrors a series around 100 (price -> 200 - price),
// swapping highs and lows, for SHORT fixtures.
func mirrorBars(bars []V7StructureBar) []V7StructureBar {
	out := make([]V7StructureBar, len(bars))
	for i, b := range bars {
		out[i] = V7StructureBar{
			OpenTimeMs: b.OpenTimeMs,
			Open:       200 - b.Open,
			High:       200 - b.Low,
			Low:        200 - b.High,
			Close:      200 - b.Close,
		}
	}
	return out
}

func longInput() V7StructureInput {
	sBars, hBars := longFixture()
	return V7StructureInput{
		Direction:      V7DirLong,
		EntryPrice:     100,
		DecisionTimeMs: fixStartMs + 200*fixStep15m, // all bars closed
		BarsTFS:        sBars,
		BarsTFH:        hBars,
	}
}

// ---------------------------------------------------------------------------
// Primitive tests
// ---------------------------------------------------------------------------

func TestV7PivotsFixture(t *testing.T) {
	highs := []float64{1, 2, 3, 2, 1, 2, 3, 4, 3, 2, 1}
	bars := make([]V7StructureBar, len(highs))
	for i, h := range highs {
		bars[i] = V7StructureBar{OpenTimeMs: int64(i), High: h, Low: -h, Close: 0}
	}
	ph := v7Pivots(bars, 2, true)
	if len(ph) != 2 || ph[0].Price != 3 || ph[1].Price != 4 {
		t.Fatalf("pivot highs = %+v, want [3 4]", ph)
	}
	pl := v7Pivots(bars, 2, false)
	if len(pl) != 2 || pl[0].Price != -3 || pl[1].Price != -4 {
		t.Fatalf("pivot lows = %+v, want [-3 -4]", pl)
	}
	// Left side is strict: an equal left neighbour kills the pivot.
	eq := []float64{1, 3, 3, 2, 1}
	bars2 := make([]V7StructureBar, len(eq))
	for i, h := range eq {
		bars2[i] = V7StructureBar{OpenTimeMs: int64(i), High: h, Low: -h}
	}
	if got := v7Pivots(bars2, 2, true); len(got) != 0 {
		t.Fatalf("equal-left pivots = %+v, want none", got)
	}
	// Right side is non-strict: an equal right neighbour keeps it.
	eqR := []float64{1, 2, 3, 3, 1}
	bars3 := make([]V7StructureBar, len(eqR))
	for i, h := range eqR {
		bars3[i] = V7StructureBar{OpenTimeMs: int64(i), High: h, Low: -h}
	}
	if got := v7Pivots(bars3, 2, true); len(got) != 1 || got[0].Price != 3 {
		t.Fatalf("equal-right pivots = %+v, want [3]", got)
	}
}

func TestV7ATRConstantTR(t *testing.T) {
	bars := flatBars(0, 900000, 40, 100, 100.4, 99.6) // TR = 0.8 every bar
	if got := v7ATR(bars, 14); math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("ATR = %v, want 0.8", got)
	}
	if got := v7ATR(bars[:10], 14); got != 0 {
		t.Fatalf("short-series ATR = %v, want 0", got)
	}
}

func TestV7NoiseH(t *testing.T) {
	// Ramp: +1% per bar, lag 2 -> every sample = 1.01^2 - 1 = 2.01%.
	closes := make([]float64, 100)
	for i := range closes {
		closes[i] = 100 * math.Pow(1.01, float64(i))
	}
	if got := v7NoiseH(closes, 2); math.Abs(got-0.0201) > 1e-6 {
		t.Fatalf("N_H ramp = %v, want ~0.0201", got)
	}
	// Flat closes -> 0.
	flat := make([]float64, 100)
	for i := range flat {
		flat[i] = 100
	}
	if got := v7NoiseH(flat, 2); got != 0 {
		t.Fatalf("N_H flat = %v, want 0", got)
	}
}

func TestV7ClusterPivotsWall(t *testing.T) {
	pivots := []v7Pivot{{Price: 108.0}, {Price: 108.3}, {Price: 120.0}}
	clusters := v7ClusterPivots(pivots, 0.6)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(clusters))
	}
	if clusters[0].Touches != 2 || math.Abs(clusters[0].Level-108.15) > 1e-9 {
		t.Fatalf("wall cluster = %+v, want touches 2 level 108.15 (median)", clusters[0])
	}
	if clusters[1].Touches != 1 {
		t.Fatalf("single cluster = %+v, want touches 1", clusters[1])
	}
}

// ---------------------------------------------------------------------------
// Builder tests
// ---------------------------------------------------------------------------

func TestV7StructurePlanLong(t *testing.T) {
	plan := BuildV7StructurePlan(longInput())
	if !plan.Feasible {
		t.Fatalf("plan infeasible: %s (d=[%.2f,%.2f] nh=%.2f)", plan.Reason, plan.DMinPct, plan.DMaxPct, plan.NoiseHPct)
	}
	if plan.SStruct != 98.0 {
		t.Fatalf("S_struct = %v, want 98", plan.SStruct)
	}
	if plan.NoiseHPct != 0 {
		t.Fatalf("N_H = %v%%, want 0", plan.NoiseHPct)
	}
	if math.Abs(plan.DMinPct-0.8) > 1e-9 {
		t.Fatalf("d_min = %v%%, want 0.8", plan.DMinPct)
	}
	// Stop menu: S_A and S_C present; S_A is the first admissible.
	ids := map[string]bool{}
	for _, s := range plan.StopCandidates {
		ids[s.ID] = true
	}
	if !ids["S_A"] || !ids["S_C"] {
		t.Fatalf("stop menu = %+v, want S_A and S_C", plan.StopCandidates)
	}
	if plan.ChosenStop == nil || plan.ChosenStop.ID != "S_A" {
		t.Fatalf("chosen stop = %+v, want S_A", plan.ChosenStop)
	}
	// S_A = S_struct - 0.30 * ATR(TF_s).
	wantSA := 98.0 - 0.30*plan.ATRTFS
	if math.Abs(plan.ChosenStop.Price-wantSA) > 1e-9 {
		t.Fatalf("S_A price = %v, want %v", plan.ChosenStop.Price, wantSA)
	}
	// Targets: TP0 = 103 pivot front-run; TP1 = the within-cap 104.7
	// wall; TP2 = the 108 wall, capped at E * 1.05 = 105. The 105
	// TF_s cluster (2 touches) is a non-role menu entry.
	if plan.TP0 < 102.5 || plan.TP0 > 103.0 {
		t.Fatalf("TP0 = %v, want ~102.9", plan.TP0)
	}
	if plan.TP1 < 104.3 || plan.TP1 > 104.6 {
		t.Fatalf("TP1 = %v, want ~104.5 (wall)", plan.TP1)
	}
	if math.Abs(plan.TP2-105.0) > 1e-9 {
		t.Fatalf("TP2 = %v, want capped 105", plan.TP2)
	}
	if len(plan.TargetCandidates) != 4 {
		t.Fatalf("target menu = %+v, want 4 entries", plan.TargetCandidates)
	}
	roles := map[string]string{}
	for _, tc := range plan.TargetCandidates {
		roles[tc.Role] = tc.Method
	}
	if roles["TP1"] != "wall_tfh" || roles["TP2"] != "wall_tfh" {
		t.Fatalf("roles = %+v", roles)
	}
	if plan.RRMeasTP1 < 1.5 {
		t.Fatalf("rr_meas_tp1 = %v, want >= 1.5", plan.RRMeasTP1)
	}
}

func TestV7StructurePlanShortMirror(t *testing.T) {
	sBars, hBars := longFixture()
	in := V7StructureInput{
		Direction:      V7DirShort,
		EntryPrice:     100,
		DecisionTimeMs: fixStartMs + 200*fixStep15m,
		BarsTFS:        mirrorBars(sBars),
		BarsTFH:        mirrorBars(hBars),
	}
	plan := BuildV7StructurePlan(in)
	if !plan.Feasible {
		t.Fatalf("short plan infeasible: %s", plan.Reason)
	}
	if plan.SStruct != 102.0 {
		t.Fatalf("short S_struct = %v, want 102", plan.SStruct)
	}
	long := BuildV7StructurePlan(longInput())
	if math.Abs(plan.ChosenStop.Price-(200-long.ChosenStop.Price)) > 1e-9 {
		t.Fatalf("short stop %v is not the mirror of long stop %v", plan.ChosenStop.Price, long.ChosenStop.Price)
	}
	if math.Abs(plan.TP1-(200-long.TP1)) > 1e-9 {
		t.Fatalf("short TP1 %v is not the mirror of long TP1 %v", plan.TP1, long.TP1)
	}
}

func TestV7StructurePlanRRInfeasible(t *testing.T) {
	in := longInput()
	// Move the 2-touch cluster from 105 to 101.5: TP1 gets too close
	// for any stop to reach RR 1.5.
	in.BarsTFS[170].High = 101.5
	in.BarsTFS[180].High = 101.5
	plan := BuildV7StructurePlan(in)
	if plan.Feasible {
		t.Fatalf("plan should be infeasible, got %+v", plan.ChosenStop)
	}
	if plan.Reason != "backend_guard_risk" {
		t.Fatalf("reason = %q, want backend_guard_risk", plan.Reason)
	}
}

func TestV7StructureNoLookahead(t *testing.T) {
	in := longInput()
	base := BuildV7StructurePlan(in)

	// Append wild future bars beyond the decision time; the plan must
	// not change at all.
	futS := append(append([]V7StructureBar{}, in.BarsTFS...),
		V7StructureBar{OpenTimeMs: fixStartMs + 200*fixStep15m, Open: 500, High: 1000, Low: 1, Close: 500},
		V7StructureBar{OpenTimeMs: fixStartMs + 201*fixStep15m, Open: 500, High: 1000, Low: 1, Close: 500})
	futH := append(append([]V7StructureBar{}, in.BarsTFH...),
		V7StructureBar{OpenTimeMs: fixStartMs + 200*fixStep1h, Open: 500, High: 1000, Low: 1, Close: 500})
	in2 := in
	in2.BarsTFS, in2.BarsTFH = futS, futH
	withFuture := BuildV7StructurePlan(in2)
	if !reflect.DeepEqual(base, withFuture) {
		t.Fatalf("future bars changed the plan:\nbase = %+v\nfuture = %+v", base, withFuture)
	}

	// Truncating all bars after the decision time changes nothing either.
	in3 := in
	in3.BarsTFS = in.BarsTFS[:200]
	in3.BarsTFH = in.BarsTFH[:200]
	trunc := BuildV7StructurePlan(in3)
	if !reflect.DeepEqual(base, trunc) {
		t.Fatalf("truncated input changed the plan")
	}
}

func TestV7StructureSize(t *testing.T) {
	cfg := V7FASTStructureConfig()
	// risk 800 USD, denom = 2.45 + 0.15 = 2.60 -> qty_risk 307.7,
	// qty_cap = 10000/100 = 100 binds.
	got := V7StructureSize(10000, 0.08, 10000, 100, 97.55, 0.001, 0, cfg)
	if math.Abs(got-100) > 1e-9 {
		t.Fatalf("size = %v, want 100 (cap binds)", got)
	}
	// Cheaper entry, small equity: qty_cap = 10000/10 = 1000 binds
	// over qty_risk = 800/(0.245+0.015) = 3076.9.
	got = V7StructureSize(10000, 0.08, 10000, 10, 9.755, 0.001, 0, cfg)
	if math.Abs(got-1000) > 1e-9 {
		t.Fatalf("size = %v, want 1000 (cap binds)", got)
	}
	// Larger equity: neither cap binds, qty = floor_step(qty_risk).
	got = V7StructureSize(10000, 0.08, 100000, 10, 9.755, 0.001, 0, cfg)
	if math.Abs(got-3076.923) > 0.01 {
		t.Fatalf("size = %v, want ~3076.923", got)
	}
	// LLM cap binds.
	got = V7StructureSize(10000, 0.08, 10000, 10, 9.755, 0.001, 50, cfg)
	if math.Abs(got-50) > 1e-9 {
		t.Fatalf("size = %v, want 50 (llm cap)", got)
	}
}

// ---------------------------------------------------------------------------
// geometry_mode flag integration
// ---------------------------------------------------------------------------

func TestV7GeometryModeDefaultLegacy(t *testing.T) {
	if got := V7CurrentGeometryMode(); got != V7GeometryModeLegacy {
		t.Fatalf("default geometry mode = %q, want legacy", got)
	}
	SetV7GeometryMode("bogus")
	if got := V7CurrentGeometryMode(); got != V7GeometryModeLegacy {
		t.Fatalf("bogus mode changed the flag: %q", got)
	}
}

func TestV7StructurePlanIntegration(t *testing.T) {
	sBars, hBars := longFixture()
	mkSig := func() *V7SignalOutput {
		return &V7SignalOutput{
			Symbol:       "FIXUSDT",
			Direction:    V7DirLong,
			SetupType:    V7SetupRangeExpansion,
			EntryZone:    V7PriceZone{Lower: 99, Upper: 101},
			Invalidation: V7InvalidationRule{Price: 99}, // nearer than the 98 anchor: anchor stands
			TP0Price:     101, // legacy placeholder; structure must override
		}
	}

	// Legacy mode (default): the structure bars are ignored, the
	// legacy plan is built from the placeholder TP0.
	ctxLegacy := &V7SymbolContext{CurrentPrice: 100, StructureBars15m: sBars, StructureBars1h: hBars}
	sig := mkSig()
	ApplyV7TakeProfitPlan(sig, ctxLegacy)
	if sig.TPPlan == nil || sig.TPPlan.StatsBucket != "tp0_tp1_no_sl" {
		t.Fatalf("legacy plan = %+v, want legacy bucket", sig.TPPlan)
	}
	if sig.Invalidation.Price != 99 {
		t.Fatalf("legacy mode changed invalidation: %v", sig.Invalidation.Price)
	}

	// Structure mode via per-context override.
	ctxStruct := &V7SymbolContext{
		CurrentPrice: 100, GeometryMode: "structure",
		StructureBars15m: sBars, StructureBars1h: hBars,
	}
	sig2 := mkSig()
	ApplyV7TakeProfitPlan(sig2, ctxStruct)
	if sig2.TPPlan == nil || sig2.TPPlan.StatsBucket != "tp0_tp1_no_sl_structure" {
		t.Fatalf("structure plan = %+v, want structure bucket", sig2.TPPlan)
	}
	plan := BuildV7StructurePlan(longInput())
	if math.Abs(sig2.TP1Price-plan.TP1) > 1e-9 {
		t.Fatalf("sig TP1 = %v, want structure TP1 %v", sig2.TP1Price, plan.TP1)
	}
	if plan.ChosenStop == nil || math.Abs(sig2.Invalidation.Price-plan.ChosenStop.Price) > 1e-9 {
		t.Fatalf("sig invalidation = %v, want chosen stop %v", sig2.Invalidation.Price, plan.ChosenStop)
	}
}
