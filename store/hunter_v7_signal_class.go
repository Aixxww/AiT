package store

// U6a: signal-class registry (redesign plan §3, owner decisions OD-6,
// OD-13, OD-14, OD-27, OD-28; revision 3 cross-check against SSA §H.3).
//
// The registry maps a setup type to its class and its admission. It is
// config: HunterConfig.V7SignalClasses overrides individual entries on top of
// DefaultV7SignalClassRegistry. Classes drive per-class behaviour in later
// units (U3 CONTEXT non-voting, U4 tier rule, U6b profiles); in U6a only the
// shadow_only admission is enforced, as a cap that turns an EXECUTABLE or
// REVIEWABLE tier into WATCH. Every default shadow_only entry is already
// WATCH today, so U6a changes no tier.

// Signal classes (plan §3.1).
const (
	V7ClassImpulse  = "IMPULSE"
	V7ClassTrigger  = "TRIGGER"
	V7ClassReversal = "REVERSAL"
	V7ClassContext  = "CONTEXT"
)

// Admissions.
const (
	V7AdmissionTradable   = "tradable"
	V7AdmissionShadowOnly = "shadow_only" // computed, tagged and walked; never above WATCH
)

// V7SignalClassRegistryVersion identifies the default registry content.
const V7SignalClassRegistryVersion = "classes-v1-plan-rev3"

// V7SignalClassEntry is one registry row.
type V7SignalClassEntry struct {
	Class       string `json:"class"`
	Admission   string `json:"admission,omitempty"`   // "" = tradable
	Provisional bool   `json:"provisional,omitempty"` // [H] row, provisional until U0-C
}

// ShadowOnly reports whether the entry is shadow-only.
func (e V7SignalClassEntry) ShadowOnly() bool { return e.Admission == V7AdmissionShadowOnly }

// DefaultV7SignalClassRegistry returns the approved assignments. Setup
// types absent from the map are UNASSIGNED (mms_trend_ride_long,
// mms_squeeze_engine_long: no H.3 verdict, U0-C decides).
func DefaultV7SignalClassRegistry() map[string]V7SignalClassEntry {
	t := func(class string) V7SignalClassEntry { return V7SignalClassEntry{Class: class} }
	h := func(class string) V7SignalClassEntry { return V7SignalClassEntry{Class: class, Provisional: true} }
	shadow := func(class string) V7SignalClassEntry {
		return V7SignalClassEntry{Class: class, Admission: V7AdmissionShadowOnly}
	}
	return map[string]V7SignalClassEntry{
		// Task-2 (T2) setup types.
		"momentum_continuation_long":        t(V7ClassImpulse),
		"momentum_continuation_short":       t(V7ClassImpulse),
		"momentum_continuation_watch_long":  shadow(V7ClassImpulse), // OD-28 (H.3: filter out)
		"momentum_continuation_watch_short": t(V7ClassImpulse),
		"ignition_early_long":               t(V7ClassTrigger),
		"ignition_early_short":              shadow(V7ClassTrigger), // OD-27 (H.3: filter or regime-gate)
		"bb_oi_accumulation_long":           t(V7ClassTrigger),
		"bb_oi_accumulation_short":          t(V7ClassTrigger),
		"compression_oi_watch_long":         t(V7ClassTrigger), // observe-only list still applies (OD-5)
		"compression_oi_watch_short":        t(V7ClassTrigger),
		"confirmed_breakout_long":           shadow(V7ClassTrigger), // OD-14, pending U0-E
		"confirmed_breakout_short":          shadow(V7ClassTrigger), // OD-14, pending U0-E
		"reclaim_breakout_long":             t(V7ClassTrigger),      // observe-only list still applies (OD-5)
		"reclaim_breakout_short":            t(V7ClassTrigger),
		"trend_pullback_long":               h(V7ClassReversal), // observe-only list still applies (OD-5)
		"trend_pullback_short":              t(V7ClassReversal), // rev 3: [H] resolved by H.3

		// Native setup types.
		"leader_momentum_long":       t(V7ClassImpulse),
		"displacement_momentum_long": t(V7ClassImpulse),
		"alt_ladder_momentum_long":   t(V7ClassImpulse),
		"breakdown_momentum_short":   t(V7ClassImpulse),
		"alt_ladder_breakdown_short": h(V7ClassImpulse),
		"short_squeeze_long":         h(V7ClassImpulse),
		"long_squeeze_short":         h(V7ClassImpulse),
		"range_expansion_event":      h(V7ClassImpulse),
		// trend_breakout_long is the native twin in OD-14; its admission is
		// left tradable here pending owner confirmation that OD-14 covers the
		// native side (enforcing it changes native trading).
		"trend_breakout_long":         t(V7ClassTrigger),
		"accumulation_breakout_long":  t(V7ClassTrigger),
		"volatility_squeeze_breakout": t(V7ClassTrigger),
		"intraday_scalp_long":         h(V7ClassTrigger),
		"panic_reversal_long":         t(V7ClassReversal),
		"pullback_reversal_long":      t(V7ClassReversal),
		"distribution_short":          t(V7ClassReversal),
		"range_reversion":             t(V7ClassReversal),
		"whale_flow_reversal":         h(V7ClassReversal),
		"relative_weakness_short":     h(V7ClassReversal),
		"mms_bottom_wake_long":        h(V7ClassReversal), // rev 3: H.3 pass
		"funding_reversal":            t(V7ClassContext),  // OD-13
		"pre_breakout_watch":          t(V7ClassContext),
		"pre_squeeze_watch":           t(V7ClassContext),
		"pre_distribution_watch":      t(V7ClassContext),
		"accumulation_watch":          t(V7ClassContext),
	}
}

// EffectiveV7SignalClasses returns the default registry with this
// strategy's overrides applied (an override replaces the whole entry).
func (h *HunterConfig) EffectiveV7SignalClasses() map[string]V7SignalClassEntry {
	reg := DefaultV7SignalClassRegistry()
	if h != nil {
		for setup, e := range h.V7SignalClasses {
			reg[setup] = e
		}
	}
	return reg
}
