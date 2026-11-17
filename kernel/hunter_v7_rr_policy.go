package kernel

// DecisionRRFloor returns the RR admission floor the validator applies to
// one decision. A value <= 0 means "unset" and the validator falls back to
// legacyMinRiskRewardRatio, as before.
type DecisionRRFloor func(d *Decision) float64

// FixedDecisionRRFloor applies one floor to every decision (the pre-U2
// behaviour: RiskControl.MinRiskRewardRatio).
func FixedDecisionRRFloor(minRR float64) DecisionRRFloor {
	return func(*Decision) float64 { return minRR }
}

// hunterV7DecisionRRFloor applies the selected candidate's own floor when
// it carries one (Task-2 candidates, U2/P2a: CandidateCoin.V7RequiredRR,
// set at assembly from RiskControlConfig.EffectiveT2MinRiskRewardRatio) and
// the strategy floor otherwise. Matching is by signal id only, the same rule
// hydrateHunterV7DecisionExecutionPlans uses, so the validator and the
// executor always see the same floor for a decision.
func hunterV7DecisionRRFloor(strategyMinRR float64, candidates []CandidateCoin) DecisionRRFloor {
	byID := make(map[string]float64, len(candidates))
	for _, c := range candidates {
		if c.V7SignalID != "" && c.V7RequiredRR > 0 {
			byID[c.V7SignalID] = c.V7RequiredRR
		}
	}
	return func(d *Decision) float64 {
		if d != nil {
			if rr, ok := byID[d.SelectedHunterV7SignalID]; ok {
				return rr
			}
		}
		return strategyMinRR
	}
}
