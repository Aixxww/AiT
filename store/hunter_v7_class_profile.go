package store

// U6 (remainder after U6a): per-class risk profiles (redesign plan §5.1)
// and the per-setup geometry overrides (F13 literals, plan §8), in config at
// current values.
//
// A class profile holds, per stack, the RR floor, the minimum stop distance
// and the TP1 distance cap of the signals of that class. A zero field
// inherits the stack value it replaces, so the default (no profiles) is
// exactly today's behaviour:
//   - native: RiskControlConfig.MinRiskRewardRatio, MinStopLossPriceMovePct
//     and MaxTakeProfitPriceMovePct (with the existing hunter_v7 defaults);
//   - t2: EffectiveT2MinRiskRewardRatio (U2), EffectiveT2MinStopLossPriceMovePct
//     (U4) and the same TP cap as native.
// U9 adds the drift contract (§3.3, §5.1 "drift contract" row): the entry
// drift limit and the price it is measured from. Stop construction, target
// model and exit profile stay out (geometry changes, or closed by OD-22 a).

// V7ClassProfileVersion identifies the default profile content.
const V7ClassProfileVersion = "profiles-v1-current-values"

// V7StackRiskProfile is one stack's values inside a class profile.
type V7StackRiskProfile struct {
	MinRiskRewardRatio        float64 `json:"min_risk_reward_ratio,omitempty"`
	MinStopLossPriceMovePct   float64 `json:"min_stop_loss_price_move_pct,omitempty"`
	MaxTakeProfitPriceMovePct float64 `json:"max_take_profit_price_move_pct,omitempty"`
	// U9 drift contract. MaxEntryPriceDeviationPct is the entry drift limit
	// (0 inherits RiskControlConfig.MaxEntryPriceDeviationPct).
	// DriftReference is the price the drift is measured from: "" or
	// "decision" (the LLM's price; owner default) or "trigger" (the
	// signal's trigger price; falls back to the decision price when the
	// signal carries none).
	MaxEntryPriceDeviationPct float64 `json:"max_entry_price_deviation_pct,omitempty"`
	DriftReference            string  `json:"drift_reference,omitempty"`
}

// Drift references (U9).
const (
	V7DriftReferenceDecision = "decision"
	V7DriftReferenceTrigger  = "trigger"
)

// IsZero reports whether every field inherits.
func (p V7StackRiskProfile) IsZero() bool {
	return p.MinRiskRewardRatio <= 0 && p.MinStopLossPriceMovePct <= 0 && p.MaxTakeProfitPriceMovePct <= 0 &&
		p.MaxEntryPriceDeviationPct <= 0 && (p.DriftReference == "" || p.DriftReference == V7DriftReferenceDecision)
}

// V7ClassRiskProfile is RiskControlConfig.ClassProfiles[class].
type V7ClassRiskProfile struct {
	Native V7StackRiskProfile `json:"native,omitempty"`
	T2     V7StackRiskProfile `json:"t2,omitempty"`
}

// ForStack returns the profile block of a stack (V7StackT2 or native).
func (p V7ClassRiskProfile) ForStack(stack string) V7StackRiskProfile {
	if stack == V7StackT2 {
		return p.T2
	}
	return p.Native
}

// DefaultV7ClassProfiles returns one profile per class with every field
// inheriting (current values).
func DefaultV7ClassProfiles() map[string]V7ClassRiskProfile {
	return map[string]V7ClassRiskProfile{
		V7ClassImpulse:  {},
		V7ClassTrigger:  {},
		V7ClassReversal: {},
		V7ClassContext:  {},
	}
}

// EffectiveV7ClassProfiles applies RiskControlConfig.ClassProfiles on top of
// the defaults (whole-entry replacement per class).
func (rc RiskControlConfig) EffectiveV7ClassProfiles() map[string]V7ClassRiskProfile {
	out := DefaultV7ClassProfiles()
	for class, p := range rc.ClassProfiles {
		out[class] = p
	}
	return out
}

// V7SetupGeometryOverride relaxes the backend RR check for one setup type
// (moved from the hunterV7SetupExecutionGeometry literals, F13):
// MinRRCap lowers the RR floor to at most this value, MaxTPFloorPct raises
// the TP1 distance cap to at least this value. 0 = no change.
type V7SetupGeometryOverride struct {
	MinRRCap      float64 `json:"min_rr_cap,omitempty"`
	MaxTPFloorPct float64 `json:"max_tp_floor_pct,omitempty"`
}

// DefaultV7SetupGeometryOverrides are the TP-distance relaxations the
// kernel hard-coded before U6. #138 M9: no default carries a MinRRCap. A
// default may never lower the strategy RR floor; a setup RR floor below it
// exists only when the strategy opts in explicitly through
// RiskControlConfig.SetupGeometryOverrides (the former defaults were 1.35 for
// the first two rows and 1.0 for intraday_scalp_long).
func DefaultV7SetupGeometryOverrides() map[string]V7SetupGeometryOverride {
	return map[string]V7SetupGeometryOverride{
		"volatility_squeeze_breakout": {MaxTPFloorPct: 12},
		"displacement_momentum_long":  {MaxTPFloorPct: 12},
		"intraday_scalp_long":         {MaxTPFloorPct: 3},
	}
}

// EffectiveV7SetupGeometryOverrides applies
// RiskControlConfig.SetupGeometryOverrides on top of the defaults. A zero
// entry removes the setup's override. Keys are canonical setup types
// (U11, D2 (a)); look entries up with LookupV7Setup.
func (rc RiskControlConfig) EffectiveV7SetupGeometryOverrides() map[string]V7SetupGeometryOverride {
	out := DefaultV7SetupGeometryOverrides()
	for setup, o := range canonicalV7SetupKeys(rc.SetupGeometryOverrides) {
		if o.MinRRCap <= 0 && o.MaxTPFloorPct <= 0 {
			delete(out, setup)
			continue
		}
		out[setup] = o
	}
	return out
}

// V7EffectiveProfileVersion is the profile_version tag (plan §5.4): the
// class registry version and the profile version, with "+config" when the
// strategy overrides either.
func V7EffectiveProfileVersion(rc RiskControlConfig, h *HunterConfig) string {
	v := V7SignalClassRegistryVersion + "/" + V7ClassProfileVersion
	custom := len(rc.ClassProfiles) > 0 || len(rc.SetupGeometryOverrides) > 0
	if h != nil && len(h.V7SignalClasses) > 0 {
		custom = true
	}
	if custom {
		v += "+config"
	}
	return v
}
