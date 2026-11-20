package local

import (
	"encoding/json"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

// TA-1: Task-2 outputs satisfy the hunter_v7 signal contract
// (price_context, derivatives_context, required_confirmations) without
// changing score, status, entry mode or lifecycle. price_context.last stays
// 0 by owner decision (the kernel would use it as the RR reference price).

func ta1Snapshot(syms ...*datafetch.SymbolSnapshot) *datafetch.Snapshot {
	m := map[string]*datafetch.SymbolSnapshot{}
	for _, ss := range syms {
		m[ss.Symbol] = ss
	}
	return &datafetch.Snapshot{Symbols: m, CreatedAt: u1T0}
}

// ta1AssertContextFromSnapshot checks the contexts equal what the native
// builders produce from the same snapshot (nothing fabricated or altered),
// except price_context.last, which must be 0.
func ta1AssertContextFromSnapshot(t *testing.T, out V7SignalOutput, snap *datafetch.Snapshot) {
	t.Helper()
	ss := snap.Symbols[out.Symbol]
	ctx := buildSymbolContext(out.Symbol, ss, snap)
	wantP, wantD := buildPriceCtx(ctx), buildDerivCtx(ctx)
	wantP.Last = 0
	if out.PriceCtx == nil || out.DerivativesCtx == nil {
		t.Fatalf("%s: contexts must be set", out.Symbol)
	}
	if out.PriceCtx.Last != 0 {
		t.Fatalf("%s: price_context.last = %v, must stay 0 in this unit", out.Symbol, out.PriceCtx.Last)
	}
	if *out.PriceCtx != *wantP {
		t.Fatalf("%s price_context = %+v, want %+v", out.Symbol, *out.PriceCtx, *wantP)
	}
	if *out.DerivativesCtx != *wantD {
		t.Fatalf("%s derivatives_context = %+v, want %+v", out.Symbol, *out.DerivativesCtx, *wantD)
	}
	if out.DerivativesCtx.FundingRate != ss.FundingRate {
		t.Fatalf("%s funding %v, want snapshot funding %v", out.Symbol, out.DerivativesCtx.FundingRate, ss.FundingRate)
	}
	if out.DerivativesCtx.OIValue != symbolOINotional(ss) {
		t.Fatalf("%s oi_value %v, want %v", out.Symbol, out.DerivativesCtx.OIValue, symbolOINotional(ss))
	}
	if v7HasTag(out.DataGaps, V7T2DataGapContextUnavailable) {
		t.Fatalf("%s: context gap set although the symbol is in the snapshot", out.Symbol)
	}
}

func ta1AssertConfirmRule(t *testing.T, out V7SignalOutput) {
	t.Helper()
	has := v7HasTag(out.RequiredConfirms, "live_price_in_entry_zone")
	if out.EntryMode == V7EntryImmediate && len(out.RequiredConfirms) != 0 {
		t.Fatalf("%s %s: immediate output must not get confirmations: %v", out.Symbol, out.LifecycleState, out.RequiredConfirms)
	}
	if out.EntryMode != V7EntryImmediate && (!has || len(out.RequiredConfirms) != 1) {
		t.Fatalf("%s %s: non-immediate output must require exactly live_price_in_entry_zone: %v", out.Symbol, out.LifecycleState, out.RequiredConfirms)
	}
}

// End to end through RunV7T2Module: a real detector fire carries contexts
// built from the snapshot, and its score/status/entry mode are unchanged.
func TestTA1RunV7T2ModuleFillsContractFields(t *testing.T) {
	ss := t2AdapterMomentumSnapshot(true)
	snap := ta1Snapshot(ss)
	cfg := &V7T2Config{Enabled: true}

	sig := EvaluateV7T2Snapshot(BuildV7T2SnapshotFromDatafetch(ss), cfg, nil)
	if sig == nil {
		t.Fatal("fixture must fire")
	}
	plain := sig.ToV7SignalOutputWithLifecycle("", sig.EffectiveStatus())

	out := RunV7T2Module(snap, cfg, NewV7T2EventStore(), nil, "", u1T0)
	if len(out) != 1 {
		t.Fatalf("emitted %d outputs, want 1", len(out))
	}
	o := out[0]
	ta1AssertContextFromSnapshot(t, o, snap)
	ta1AssertConfirmRule(t, o)
	if o.SetupScore != plain.SetupScore || o.AIPriority != plain.AIPriority || o.Status != plain.Status ||
		o.EntryMode != plain.EntryMode || o.LifecycleState != plain.LifecycleState || o.Confidence != plain.Confidence ||
		o.Invalidation != plain.Invalidation || o.TriggerPrice != plain.TriggerPrice || len(o.Targets) != len(plain.Targets) {
		t.Fatalf("TA-1 changed decision fields:\n got  %+v\n want %+v", o, plain)
	}
}

// Every lifecycle state: contexts filled, confirmation only when the
// entry is not immediate (WATCH, observe-only TRIGGERED).
func TestTA1ConfirmationOnlyOnNonImmediateOutputs(t *testing.T) {
	ss := &datafetch.SymbolSnapshot{Symbol: "AUSDT", Price: 100, FundingRate: -0.0002, OI: 5000, OIDelta1h: 1.5, LongShortRatio: 1.1, LSROldest: 0.9}
	snap := ta1Snapshot(ss)
	obs := u1Sig("trend_pullback_long", "LONG", 94)
	obs.ObserveOnly = true
	cases := []struct {
		name      string
		sig       *V7T2Signal
		state     string
		immediate bool
	}{
		{"watch", u1Sig("ignition_early_long", "LONG", 70), "WATCH", false},
		{"armed", func() *V7T2Signal { s := u1Sig("momentum_continuation_long", "LONG", 90); s.Status = "ARMED"; return s }(), "ARMED", true},
		{"triggered", u1Sig("ignition_early_long", "LONG", 70), "TRIGGERED", true},
		{"observe-only triggered", obs, "TRIGGERED", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := c.sig.ToV7SignalOutputWithLifecycle("", c.state)
			before := o
			ApplyV7T2ContractFields(&o, snap)
			if (o.EntryMode == V7EntryImmediate) != c.immediate {
				t.Fatalf("entry mode %s, immediate want %v", o.EntryMode, c.immediate)
			}
			if o.Status != before.Status || o.EntryMode != before.EntryMode || o.LifecycleState != before.LifecycleState || o.SetupScore != before.SetupScore {
				t.Fatal("decision fields changed")
			}
			ta1AssertContextFromSnapshot(t, o, snap)
			ta1AssertConfirmRule(t, o)
		})
	}
}

// FreshlyTriggered re-emissions (detector silent on this scan) are covered,
// and a re-emitted event whose symbol left the snapshot gets empty contexts
// plus the t2_context_unavailable gap, never fabricated values.
func TestTA1FreshlyTriggeredReemissionCovered(t *testing.T) {
	st, ev := u1Store(t, u1Sig("ignition_early_long", "LONG", 70))
	cfg := &V7T2Config{Enabled: true}

	// Scan at +3 min: price reaches the entry, the detector does not fire
	// (no klines), so the output is the FreshlyTriggered re-emission.
	present := ta1Snapshot(&datafetch.SymbolSnapshot{Symbol: "AUSDT", Price: 100, FundingRate: 0.0003, OI: 2000})
	out := RunV7T2Module(present, cfg, st, nil, "", at(3))
	if ev.Status != "TRIGGERED" || len(out) != 1 || out[0].LifecycleState != "TRIGGERED" {
		t.Fatalf("re-emission expected: status=%s out=%+v", ev.Status, out)
	}
	ta1AssertContextFromSnapshot(t, out[0], present)
	ta1AssertConfirmRule(t, out[0])

	// Scan at +6 min: the symbol is no longer in the snapshot.
	absent := ta1Snapshot(&datafetch.SymbolSnapshot{Symbol: "BUSDT", Price: 5})
	out = RunV7T2Module(absent, cfg, st, nil, "", at(6))
	if len(out) != 1 || out[0].Symbol != "AUSDT" {
		t.Fatalf("re-emission of an absent symbol expected, got %+v", out)
	}
	o := out[0]
	if o.PriceCtx == nil || o.DerivativesCtx == nil || *o.PriceCtx != (V7PriceContext{}) || *o.DerivativesCtx != (V7DerivativesContext{}) {
		t.Fatalf("absent symbol must get empty contexts: %+v %+v", o.PriceCtx, o.DerivativesCtx)
	}
	if !v7HasTag(o.DataGaps, V7T2DataGapContextUnavailable) ||
		!v7HasTag(o.DataGaps, V7T2DataGapGlobalLSR) || !v7HasTag(o.DataGaps, V7T2DataGapMarketCap) {
		t.Fatalf("data gaps %v", o.DataGaps)
	}
}

// The JSON keys the harness contract requires are present on T2 outputs.
func TestTA1OutputJSONCarriesContractKeys(t *testing.T) {
	o := u1Sig("ignition_early_long", "LONG", 70).ToV7SignalOutputWithLifecycle("", "WATCH")
	ApplyV7T2ContractFields(&o, ta1Snapshot(&datafetch.SymbolSnapshot{Symbol: "AUSDT", Price: 100}))
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"price_context", "derivatives_context", "required_confirmations"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing JSON key %s", k)
		}
	}
	if m["required_confirmations"] == nil {
		t.Fatal("required_confirmations must not be null on a WATCH output")
	}
}

func TestTA1ApplyIsIdempotentAndNilSafe(t *testing.T) {
	ApplyV7T2ContractFields(nil, nil)
	o := u1Sig("ignition_early_long", "LONG", 70).ToV7SignalOutputWithLifecycle("", "WATCH")
	ApplyV7T2ContractFields(&o, nil)
	ApplyV7T2ContractFields(&o, nil)
	if len(o.RequiredConfirms) != 1 {
		t.Fatalf("confirmations duplicated: %v", o.RequiredConfirms)
	}
	n := 0
	for _, g := range o.DataGaps {
		if g == V7T2DataGapContextUnavailable {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("context gap duplicated: %v", o.DataGaps)
	}
}
