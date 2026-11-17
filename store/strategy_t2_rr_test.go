package store

import "testing"

// U2/P2a: the T2 RR floor has one accessor with a config default.
func TestEffectiveT2MinRiskRewardRatio(t *testing.T) {
	if got := (RiskControlConfig{}).EffectiveT2MinRiskRewardRatio(); got != DefaultT2MinRiskRewardRatio {
		t.Fatalf("unset field must use the default %.2f, got %.2f", DefaultT2MinRiskRewardRatio, got)
	}
	if DefaultT2MinRiskRewardRatio != 1.2 {
		t.Fatalf("owner-approved t2 default is 1.2, got %.2f", DefaultT2MinRiskRewardRatio)
	}
	if got := (RiskControlConfig{T2MinRiskRewardRatio: 1.5}).EffectiveT2MinRiskRewardRatio(); got != 1.5 {
		t.Fatalf("configured value must win, got %.2f", got)
	}
	// Native floor is a separate field and is untouched by the t2 field.
	rc := RiskControlConfig{MinRiskRewardRatio: 3.0, T2MinRiskRewardRatio: 1.2}
	if rc.MinRiskRewardRatio != 3.0 {
		t.Fatalf("native MinRiskRewardRatio changed")
	}
}
