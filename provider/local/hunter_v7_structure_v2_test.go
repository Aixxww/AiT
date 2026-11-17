package local

import (
	"math"
	"testing"
)

// Spec v2 (owner-approved 2026-11-17) unit tests: V2-1 classes, V2-2
// targets (within-cap only, no clamping, TP1 fallback, extremes never
// TP1), V2-3 stops (S_A_floor push, every stop beyond S_struct), V2-4
// admission on S_A only, and the report-only reject details.

// v2Invariants checks the invariants every plan must satisfy.
func v2Invariants(t *testing.T, name string, in V7StructureInput, p *V7StructurePlan) {
	t.Helper()
	cfg := in.Config.withDefaults()
	dir := 1.0
	if in.Direction == V7DirShort {
		dir = -1.0
	}
	E := in.EntryPrice
	capPrice := E * (1 + dir*cfg.TPCapPct)
	for _, tc := range p.TargetCandidates {
		if dir*(tc.Price-capPrice) > 1e-12 {
			t.Fatalf("%s: menu target %+v lies beyond the cap %v", name, tc, capPrice)
		}
		if tc.Role == "TP1" && (tc.Method == "extreme_tfs" || tc.Method == "extreme_tfh") {
			t.Fatalf("%s: extreme %+v holds TP1", name, tc)
		}
	}
	for _, s := range p.StopCandidates { // G4
		if p.SStruct <= 0 || dir*(p.SStruct-s.Price) <= 0 {
			t.Fatalf("%s: stop %+v is not beyond S_struct %v", name, s, p.SStruct)
		}
	}
	if p.Feasible {
		if p.ChosenStop == nil || (p.ChosenStop.ID != "S_A" && p.ChosenStop.ID != "S_A_floor") {
			t.Fatalf("%s: admission stop must be S_A/S_A_floor, got %+v", name, p.ChosenStop)
		}
		if p.RRMeasTP1 < cfg.RRMin {
			t.Fatalf("%s: admitted with RR %v < %v", name, p.RRMeasTP1, cfg.RRMin)
		}
	}
}

func TestV2LongFixtureStopsBeyondStructure(t *testing.T) {
	in := longInput()
	p := BuildV7StructurePlan(in)
	v2Invariants(t, "long", in, p)
	if p.Class != "FAST" || p.TP1Fallback {
		t.Fatalf("class=%q fallback=%v", p.Class, p.TP1Fallback)
	}
	var sC *V7StopCandidate
	for i := range p.StopCandidates {
		if p.StopCandidates[i].ID == "S_C" {
			sC = &p.StopCandidates[i]
		}
	}
	// S_C = E*(1 - (max(d_min, d_A) + 0.10%)): 0.10% beyond S_A.
	if sC == nil || math.Abs((p.ChosenStop.DistPct+0.10)-sC.DistPct) > 1e-9 {
		t.Fatalf("S_C = %+v, want S_A distance + 0.10%%", sC)
	}
	if math.Abs(p.NearestCandidateDistPct-(p.TP0-100)) > 1e-9 {
		t.Fatalf("nearest candidate dist = %v, want %v", p.NearestCandidateDistPct, p.TP0-100)
	}
}

func TestV2StopPushedToFloorNeverInward(t *testing.T) {
	in := longInput()
	in.EntryPrice = 98.4 // S_A = 98 - buffer sits < 0.80% below E
	p := BuildV7StructurePlan(in)
	v2Invariants(t, "floor", in, p)
	var a *V7StopCandidate
	for i := range p.StopCandidates {
		if p.StopCandidates[i].ID == "S_A_floor" {
			a = &p.StopCandidates[i]
		}
		if p.StopCandidates[i].ID == "S_A" {
			t.Fatalf("plain S_A listed although inside d_min: %+v", p.StopCandidates[i])
		}
	}
	want := 98.4 * (1 - 0.008)
	if a == nil || math.Abs(a.Price-want) > 1e-9 {
		t.Fatalf("S_A_floor = %+v, want price %v", a, want)
	}
}

func TestV2TargetBeyondCapNotClampedAndTP1Fallback(t *testing.T) {
	in := longInput()
	in.Config = V7FASTStructureConfig()
	in.Config.TPCapPct = 0.03 // only the 1-touch 103 pivot is within cap
	p := BuildV7StructurePlan(in)
	v2Invariants(t, "cap3", in, p)
	if !p.TP1Fallback || math.Abs(p.TP1-p.TP0) > 1e-12 || p.TP1 > 103 {
		t.Fatalf("want TP1 = 1-touch pivot fallback (unclamped), got TP1=%v TP0=%v fallback=%v", p.TP1, p.TP0, p.TP1Fallback)
	}
	if p.Feasible || p.Reason != "backend_guard_risk" || p.RejectDetail != "s_a_fails_rr" {
		t.Fatalf("want backend_guard_risk/s_a_fails_rr, got %v %q/%q", p.Feasible, p.Reason, p.RejectDetail)
	}
}

func TestV2RejectDetails(t *testing.T) {
	in := longInput()
	in.Config = V7FASTStructureConfig()
	in.Config.TPCapPct = 0.02 // every candidate (nearest 2.84%) beyond cap
	p := BuildV7StructurePlan(in)
	if p.Reason != "rr_insufficient" || p.RejectDetail != "nearest_candidate_beyond_cap" || len(p.TargetCandidates) != 0 {
		t.Fatalf("cap 2%%: got %q/%q menu=%d", p.Reason, p.RejectDetail, len(p.TargetCandidates))
	}
	if p.NearestCandidateDistPct < 2.8 || p.NearestCandidateDistPct > 2.9 {
		t.Fatalf("nearest candidate dist = %v, want ~2.84", p.NearestCandidateDistPct)
	}

	in = longInput()
	in.EntryPrice = 120 // price discovery: nothing above E (G-v2-5: no projection)
	p = BuildV7StructurePlan(in)
	if p.Reason != "rr_insufficient" || p.RejectDetail != "no_candidate_in_direction" || p.TP1 != 0 {
		t.Fatalf("price discovery: got %q/%q TP1=%v", p.Reason, p.RejectDetail, p.TP1)
	}
}

func TestV2ShortMirror(t *testing.T) {
	sBars, hBars := longFixture()
	in := V7StructureInput{Direction: V7DirShort, EntryPrice: 100,
		DecisionTimeMs: fixStartMs + 200*fixStep15m, BarsTFS: mirrorBars(sBars), BarsTFH: mirrorBars(hBars)}
	p := BuildV7StructurePlan(in)
	v2Invariants(t, "short", in, p)
	long := BuildV7StructurePlan(longInput())
	if p.TP2 != 0 || len(p.TargetCandidates) != len(long.TargetCandidates) {
		t.Fatalf("short not a mirror: TP2=%v menu=%d vs %d", p.TP2, len(p.TargetCandidates), len(long.TargetCandidates))
	}
}

func retime(bars []V7StructureBar, startMs, stepMs int64) []V7StructureBar {
	out := make([]V7StructureBar, len(bars))
	for i, b := range bars {
		b.OpenTimeMs = startMs + int64(i)*stepMs
		out[i] = b
	}
	return out
}

func TestV2SwingClassAndConfig(t *testing.T) {
	c := V7SWINGStructureConfig()
	if c.Class != "SWING" || c.TFSIntervalMs != 3600000 || c.TFHIntervalMs != 4*3600000 ||
		c.TPCapPct != 0.10 || c.PivotKTFS != 2 || c.PivotKTFH != 2 || c.LookbackTFS != 168 ||
		c.LookbackTFH != 90 || c.NoiseHorizonBars != 2 || c.NoiseWindowBars != 168 {
		t.Fatalf("SWING constants differ from spec v2 V2-1: %+v", c)
	}
	f := V7FASTStructureConfig()
	if c.BufferATRMult != f.BufferATRMult || c.RRMin != f.RRMin || c.DMinFloor != f.DMinFloor || c.CostPct != f.CostPct {
		t.Fatalf("SWING must share every non-class constant with FAST")
	}
	for setup, want := range map[string]string{
		"ignition_early_long": "FAST", "momentum_continuation_short": "FAST",
		"bb_oi_accumulation_long": "SWING", "compression_oi_watch_short": "SWING",
		"confirmed_breakout_long": "SWING", "reclaim_breakout_short": "SWING",
		"trend_pullback_long": "SWING", "displacement_momentum_long": "FAST",
		"funding_reversal": "SWING", "unknown_setup": "FAST",
	} {
		if got := V7StructureClassForSetup(setup); got != want {
			t.Fatalf("class(%s) = %s, want %s", setup, got, want)
		}
	}
}

func TestV2SwingBuilderUsesHourlyAnd4hBars(t *testing.T) {
	sBars, hBars := longFixture()
	start := fixStartMs
	in := V7StructureInput{
		Direction: V7DirLong, EntryPrice: 100,
		BarsTFS: retime(sBars, start, 3600000), BarsTFH: retime(hBars, start, 4*3600000),
		DecisionTimeMs: start + 200*3600000,
		Config:         V7SWINGStructureConfig(),
	}
	p := BuildV7StructurePlan(in)
	v2Invariants(t, "swing", in, p)
	if p.Class != "SWING" {
		t.Fatalf("class = %q", p.Class)
	}
	for _, tc := range p.TargetCandidates {
		if tc.SourceTF != "1h" && tc.SourceTF != "4h" {
			t.Fatalf("SWING target sourced from %q", tc.SourceTF)
		}
	}
	// No lookahead: bars after the decision time change nothing.
	in2 := in
	in2.BarsTFS = append(append([]V7StructureBar{}, in.BarsTFS...),
		V7StructureBar{OpenTimeMs: in.DecisionTimeMs, Open: 100, High: 150, Low: 50, Close: 140})
	if q := BuildV7StructurePlan(in2); q.TP1 != p.TP1 || q.Feasible != p.Feasible || q.SStruct != p.SStruct {
		t.Fatalf("future bar changed the SWING plan")
	}
}

func TestV2IntegrationSwingNeeds4hBars(t *testing.T) {
	sBars, hBars := longFixture()
	h1 := retime(sBars, fixStartMs, 3600000)
	h4 := retime(hBars, fixStartMs, 4*3600000)
	mk := func() *V7SignalOutput {
		return &V7SignalOutput{Symbol: "FIXUSDT", Direction: V7DirLong, SetupType: V7SetupFundingReversal,
			EntryZone: V7PriceZone{Lower: 99, Upper: 101}, Invalidation: V7InvalidationRule{Price: 99}, TP0Price: 101}
	}
	// SWING without 4h bars: structure not handled, legacy plan stands.
	sig := mk()
	ApplyV7TakeProfitPlan(sig, &V7SymbolContext{CurrentPrice: 100, GeometryMode: "structure", StructureBars1h: h1})
	if sig.TPPlan == nil || sig.TPPlan.StatsBucket != "tp0_tp1_no_sl" {
		t.Fatalf("SWING without 4h bars must fall back to legacy, got %+v", sig.TPPlan)
	}
	// SWING with 1h + 4h bars: the structure path handles it.
	sig = mk()
	ApplyV7TakeProfitPlan(sig, &V7SymbolContext{CurrentPrice: 100, GeometryMode: "structure", StructureBars1h: h1, StructureBars4h: h4})
	want := BuildV7StructurePlan(V7StructureInput{Direction: V7DirLong, EntryPrice: 100, BarsTFS: h1, BarsTFH: h4,
		DecisionTimeMs: h1[len(h1)-1].OpenTimeMs + 3600000, Family: v7StructureFamilyForSetup(V7SetupFundingReversal),
		InvalidationPrice: 99, Config: V7SWINGStructureConfig()})
	if want.Feasible {
		if sig.TPPlan == nil || sig.TPPlan.StatsBucket != "tp0_tp1_no_sl_structure" || math.Abs(sig.TP1Price-want.TP1) > 1e-9 {
			t.Fatalf("SWING structure plan not applied: %+v", sig.TPPlan)
		}
	} else if sig.TPPlan != nil {
		t.Fatalf("infeasible SWING structure plan must leave TPPlan nil, got %+v", sig.TPPlan)
	}
}
