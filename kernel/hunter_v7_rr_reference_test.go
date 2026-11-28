package kernel

import (
	"math"
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
)

// Fix 06: T2 PriceCtx.Last carries the scan price, and the backend RR /
// stop-distance reference of a T2 candidate stays on the entry-zone edge
// through V7RRReferencePrice.

func TestT2RRReferencePriceIsTheZoneEdge(t *testing.T) {
	zone := local.V7PriceZone{Lower: 99, Upper: 100}
	cases := []struct {
		stack, dir string
		want       float64
	}{
		{local.V7StackT2, "LONG", 100},
		{local.V7StackT2, "SHORT", 99},
		{local.V7StackT2, "", 0},
		{"", "LONG", 0},
		{"", "SHORT", 0},
	}
	for _, c := range cases {
		if got := hunterV7T2RRReferencePrice(c.stack, c.dir, zone); got != c.want {
			t.Errorf("stack %q dir %q: %v, want %v", c.stack, c.dir, got, c.want)
		}
	}
	if got := hunterV7T2RRReferencePrice(local.V7StackT2, "LONG", local.V7PriceZone{Lower: 99}); got != 0 {
		t.Errorf("unset upper edge: %v, want 0", got)
	}
}

// rrRefSignal: LONG zone 99-100, stop 97.5, TP1 103.25. From the zone top
// (the old fallback) RR = 3.25/2.5 = 1.3, which passes the 1.2 T2 floor.
// From a scan price of 101 (above the zone) RR = 2.25/3.5 = 0.64, which
// would fail it.
func rrRefSignal(stack string, last float64) local.V7SignalOutput {
	return local.V7SignalOutput{
		Symbol:         "AUSDT",
		Stack:          stack,
		LifecycleState: "TRIGGERED",
		Direction:      local.V7DirLong,
		SetupType:      "ignition_early_long",
		Status:         local.V7StatusWaitConfirm,
		EntryMode:      local.V7EntryImmediate,
		EntryZone:      local.V7PriceZone{Lower: 99, Upper: 100},
		Invalidation:   local.V7InvalidationRule{Price: 97.5},
		Targets:        []local.V7Target{{Price: 103.25}},
		TP1Price:       103.25,
		PriceCtx:       &local.V7PriceContext{Last: last},
	}
}

func assembleOne(t *testing.T, sig local.V7SignalOutput) CandidateCoin {
	t.Helper()
	cc := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "", defaultGeometry())
	if len(cc) != 1 {
		t.Fatalf("assembled %d candidates", len(cc))
	}
	return cc[0]
}

// A T2 candidate's RR geometry, tier and reason do not depend on Last.
func TestT2AssemblyRRIgnoresScanPrice(t *testing.T) {
	g := defaultGeometry()
	zero := assembleOne(t, rrRefSignal(local.V7StackT2, 0))
	scan := assembleOne(t, rrRefSignal(local.V7StackT2, 101))
	if scan.V7RRReferencePrice != 100 || zero.V7RRReferencePrice != 100 {
		t.Fatalf("V7RRReferencePrice %v / %v, want the zone top 100", scan.V7RRReferencePrice, zero.V7RRReferencePrice)
	}
	if scan.V7PriceContext.Last != 101 {
		t.Fatal("Last must keep the scan price")
	}
	if hunterV7ReferencePrice(scan) != 100 || hunterV7StopDistancePct(scan) != hunterV7StopDistancePct(zero) {
		t.Fatalf("reference %v stop %v/%v", hunterV7ReferencePrice(scan), hunterV7StopDistancePct(scan), hunterV7StopDistancePct(zero))
	}
	if r := hunterV7BackendCappedRRWaitReason(scan, g); r != "" {
		t.Fatalf("T2 RR from the zone top is 1.3 and must pass, got %q", r)
	}
	if scan.V7ExecutionTier != zero.V7ExecutionTier || scan.V7TierReason != zero.V7TierReason {
		t.Fatalf("tier moved with Last: %s/%s vs %s/%s", scan.V7ExecutionTier, scan.V7TierReason, zero.V7ExecutionTier, zero.V7TierReason)
	}
	if rw, _ := hunterV7DecisionRWin(scan, 0); rw != func() float64 { r, _ := hunterV7DecisionRWin(zero, 0); return r }() {
		t.Fatal("decision R must not move with Last")
	}
	// Sensitivity: without the reference the scan price would fail the floor.
	scan.V7RRReferencePrice = 0
	if r := hunterV7BackendCappedRRWaitReason(scan, g); r != "backend_rr_infeasible" {
		t.Fatalf("without V7RRReferencePrice the scan price must fail the RR floor, got %q", r)
	}
}

// SHORT uses the zone bottom.
func TestT2AssemblyRRReferenceShort(t *testing.T) {
	sig := rrRefSignal(local.V7StackT2, 98)
	sig.Direction, sig.SetupType = local.V7DirShort, "ignition_early_short"
	sig.Invalidation = local.V7InvalidationRule{Price: 101.5}
	sig.Targets, sig.TP1Price = []local.V7Target{{Price: 95.75}}, 95.75
	cc := assembleOne(t, sig)
	if cc.V7RRReferencePrice != 99 || hunterV7ReferencePrice(cc) != 99 {
		t.Fatalf("SHORT reference %v / %v, want the zone bottom 99", cc.V7RRReferencePrice, hunterV7ReferencePrice(cc))
	}
}

// Native candidates are unchanged: no reference field, RR from Last.
func TestNativeAssemblyRRStillUsesLast(t *testing.T) {
	cc := assembleOne(t, rrRefSignal("", 101))
	if cc.V7RRReferencePrice != 0 || hunterV7ReferencePrice(cc) != 101 {
		t.Fatalf("native reference field %v, reference %v, want 0 and Last 101", cc.V7RRReferencePrice, hunterV7ReferencePrice(cc))
	}
	want := (101 - 97.5) / 101 * 100
	if got := hunterV7StopDistancePct(cc); math.Abs(got-want) > 1e-9 {
		t.Fatalf("native stop distance %v, want %v (from Last)", got, want)
	}
}

// The recorded RR band of a T2 signal does not move with Last.
func TestT2SignalTagsRRBandIgnoresScanPrice(t *testing.T) {
	opts := DefaultHunterV7RecordTagOptions()
	zero := hunterV7SignalTags(rrRefSignal(local.V7StackT2, 0), nil, "WATCH", opts)
	scan := hunterV7SignalTags(rrRefSignal(local.V7StackT2, 101), nil, "WATCH", opts)
	if zero.RRBand != scan.RRBand || zero.StopATRBand != scan.StopATRBand {
		t.Fatalf("RR band %q -> %q, stop band %q -> %q", zero.RRBand, scan.RRBand, zero.StopATRBand, scan.StopATRBand)
	}
	native0 := hunterV7SignalTags(rrRefSignal("", 0), nil, "WATCH", opts)
	native := hunterV7SignalTags(rrRefSignal("", 101), nil, "WATCH", opts)
	if native0.RRBand == native.RRBand {
		t.Fatalf("native RR band must still follow Last (%q both); the fixture lost its sensitivity", native.RRBand)
	}
}

// U6b (M5): pullback-side widening on T2 entry zones keeps the breakout edge
// pinned, leaving hunterV7T2RRReferencePrice, assembly RR, stop distance, tier,
// and StopATRBand/RRBand byte-identical for both LONG and SHORT.
func TestU6bWidenedT2ZonePreservesRRReferenceAndStopATRBand(t *testing.T) {
	g := defaultGeometry()
	opts := DefaultHunterV7RecordTagOptions()

	cases := []struct {
		name  string
		dir   string
		setup string
		trig  float64
		entry float64
		stop  float64
		tp1   float64
		tp2   float64
		last  float64
	}{
		{
			name: "LONG", dir: "LONG", setup: "ignition_early_long",
			trig: 99.9, entry: 100.0, stop: 99.0, tp1: 101.8, tp2: 103.0, last: 99.7,
		},
		{
			name: "SHORT", dir: "SHORT", setup: "momentum_continuation_short",
			trig: 100.1, entry: 100.0, stop: 101.0, tp1: 98.2, tp2: 97.0, last: 100.3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t2Sig := &local.V7T2Signal{
				Symbol: "AUSDT", Direction: tc.dir, SetupType: tc.setup,
				Status: "ARMED", Score: 85,
				Trigger: tc.trig, Entry: tc.entry, Stop: tc.stop,
				Target1: tc.tp1, Target2: tc.tp2,
				ATR15mPct: 1.6, // widens pullback side to 0.50% (stop-capped)
			}
			widened := t2Sig.ToV7SignalOutputWithLifecycle(local.V7RegimeMixed, "TRIGGERED")
			widened.LiquidityScore = 85
			widened.PriceCtx = &local.V7PriceContext{Last: tc.last}

			// Legacy narrow zone [min(Entry, Trigger), max(Entry, Trigger)] with identical ATR15mPct.
			narrow := widened
			narrow.EntryZone = local.V7PriceZone{
				Lower: math.Min(tc.entry, tc.trig),
				Upper: math.Max(tc.entry, tc.trig),
			}
			if widened.EntryZone == narrow.EntryZone {
				t.Fatalf("fixture sanity: expected widened zone to differ from legacy narrow zone (%+v)", widened.EntryZone)
			}

			// Reference price invariant.
			refNarrow := hunterV7T2RRReferencePrice(narrow.Stack, string(narrow.Direction), narrow.EntryZone)
			refWidened := hunterV7T2RRReferencePrice(widened.Stack, string(widened.Direction), widened.EntryZone)
			if refNarrow != tc.entry || refWidened != refNarrow {
				t.Fatalf("hunterV7T2RRReferencePrice changed: narrow=%v widened=%v want=%v", refNarrow, refWidened, tc.entry)
			}

			// Assembly RR, stop distance, and tier invariant.
			cNarrow := assembleOne(t, narrow)
			cWidened := assembleOne(t, widened)
			if cWidened.V7RRReferencePrice != cNarrow.V7RRReferencePrice {
				t.Fatalf("V7RRReferencePrice changed: %v -> %v", cNarrow.V7RRReferencePrice, cWidened.V7RRReferencePrice)
			}
			rrNarrow, floorNarrow, okNarrow := hunterV7BackendCappedRR(cNarrow, g)
			rrWidened, floorWidened, okWidened := hunterV7BackendCappedRR(cWidened, g)
			if okNarrow != okWidened || floorNarrow != floorWidened || math.Abs(rrNarrow-rrWidened) > 1e-12 {
				t.Fatalf("assembly RR changed: (%v,%v,%v) -> (%v,%v,%v)", rrNarrow, floorNarrow, okNarrow, rrWidened, floorWidened, okWidened)
			}
			if math.Abs(hunterV7StopDistancePct(cNarrow)-hunterV7StopDistancePct(cWidened)) > 1e-12 {
				t.Fatalf("stop distance changed: %v -> %v", hunterV7StopDistancePct(cNarrow), hunterV7StopDistancePct(cWidened))
			}
			if cNarrow.V7ExecutionTier != cWidened.V7ExecutionTier || cNarrow.V7TierReason != cWidened.V7TierReason {
				t.Fatalf("tier changed: %s/%s -> %s/%s", cNarrow.V7ExecutionTier, cNarrow.V7TierReason, cWidened.V7ExecutionTier, cWidened.V7TierReason)
			}

			// Signal tags (RRBand + StopATRBand) invariant.
			tagsNarrow := hunterV7SignalTags(narrow, &cNarrow, cNarrow.V7ExecutionTier, opts)
			tagsWidened := hunterV7SignalTags(widened, &cWidened, cWidened.V7ExecutionTier, opts)
			if tagsNarrow.RRBand != tagsWidened.RRBand || tagsNarrow.StopATRBand != tagsWidened.StopATRBand {
				t.Fatalf("tags changed: RRBand %q->%q, StopATRBand %q->%q",
					tagsNarrow.RRBand, tagsWidened.RRBand, tagsNarrow.StopATRBand, tagsWidened.StopATRBand)
			}
			if tagsWidened.StopATRBand == "" || tagsWidened.StopATRBand == "unknown" {
				t.Fatalf("expected concrete StopATRBand, got %q", tagsWidened.StopATRBand)
			}
		})
	}
}
