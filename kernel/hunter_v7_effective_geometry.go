package kernel

import (
	"sort"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Fix 15 (P2-15): the backend geometry as the assembly actually applies
// it, resolved through the same functions the tier rules call
// (hunterV7T2MinRR, hunterV7CandidateRisk, hunterV7ClassRegistry, ...), so
// a report can state what was in effect. Read only: nothing decides from it.

// HunterV7EffectiveClassRisk is one class x stack's effective backend risk
// values (class profile over the stack value; per-setup overrides are
// listed separately in SetupOverrides).
type HunterV7EffectiveClassRisk struct {
	Class          string  `json:"class"`
	Stack          string  `json:"stack"`
	MinRR          float64 `json:"min_rr"`
	MinSLMovePct   float64 `json:"min_sl_move_pct"`
	MaxTPMovePct   float64 `json:"max_tp_move_pct"`
	MaxDriftPct    float64 `json:"max_drift_pct"`
	DriftReference string  `json:"drift_reference"`
}

// HunterV7EffectiveGeometry is the resolved geometry.
type HunterV7EffectiveGeometry struct {
	NativeMinRR        float64                                  `json:"native_min_rr"`
	NativeMinSLMovePct float64                                  `json:"native_min_sl_move_pct"`
	MaxTPMovePct       float64                                  `json:"max_tp_move_pct"`
	MaxDriftPct        float64                                  `json:"max_drift_pct"`
	T2MinRR            float64                                  `json:"t2_min_rr"`
	T2MinSLMovePct     float64                                  `json:"t2_min_sl_move_pct"`
	ClassRisk          []HunterV7EffectiveClassRisk             `json:"class_risk"`
	SetupOverrides     map[string]store.V7SetupGeometryOverride `json:"setup_overrides"`
	ShadowOnlySetups   []string                                 `json:"shadow_only_setups"`
	T2ReviewableStates map[string][]string                      `json:"t2_reviewable_states"`
	ConditionalClasses []string                                 `json:"conditional_entry_classes"`
}

// HunterV7EffectiveGeometryOf resolves g.
func HunterV7EffectiveGeometryOf(g HunterV7ExecutionGeometry) HunterV7EffectiveGeometry {
	out := HunterV7EffectiveGeometry{
		NativeMinRR: g.MinRR, NativeMinSLMovePct: g.MinSLMovePct, MaxTPMovePct: g.MaxTPMovePct, MaxDriftPct: g.MaxDriftPct,
		T2MinRR: hunterV7T2MinRR(g), T2MinSLMovePct: hunterV7T2MinSLMovePct(g),
		T2ReviewableStates: hunterV7T2ReviewableStates(g),
		ConditionalClasses: []string{},
		ShadowOnlySetups:   []string{},
	}
	out.SetupOverrides = g.SetupOverrides
	if out.SetupOverrides == nil {
		out.SetupOverrides = store.DefaultV7SetupGeometryOverrides()
	}
	registry := hunterV7ClassRegistry(g)
	classSet := map[string]bool{}
	for setup, e := range registry {
		if e.Admission == store.V7AdmissionShadowOnly {
			out.ShadowOnlySetups = append(out.ShadowOnlySetups, setup)
		}
		if e.Class != "" {
			classSet[e.Class] = true
		}
	}
	for c := range g.ClassProfiles {
		classSet[c] = true
	}
	for c, on := range g.ConditionalClasses {
		if on {
			out.ConditionalClasses = append(out.ConditionalClasses, c)
		}
	}
	sort.Strings(out.ShadowOnlySetups)
	sort.Strings(out.ConditionalClasses)
	classes := make([]string, 0, len(classSet))
	for c := range classSet {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	for _, c := range classes {
		for _, stack := range []string{"", local.V7StackT2} {
			coin := CandidateCoin{V7Stack: stack, V7SignalClass: c}
			minRR, minSL, maxTP := hunterV7CandidateRisk(coin, g)
			drift, ref := hunterV7CandidateDrift(coin, g)
			name := store.V7StackNative
			if stack == local.V7StackT2 {
				name = store.V7StackT2
			}
			out.ClassRisk = append(out.ClassRisk, HunterV7EffectiveClassRisk{Class: c, Stack: name, MinRR: minRR, MinSLMovePct: minSL,
				MaxTPMovePct: maxTP, MaxDriftPct: drift, DriftReference: ref})
		}
	}
	return out
}
