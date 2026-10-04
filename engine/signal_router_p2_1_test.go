package engine

import "testing"

// ============================================================================
// P2-1: Bollinger Band position bug fix
// Previously `price := set.BBMiddle` made pos always 0.5, so "Price near BB
// band" reasons could never trigger. buildSignalReasons now takes the actual
// price as a parameter.
// ============================================================================

func TestBuildSignalReasonsBBPositionNotConstant(t *testing.T) {
	base := &IndicatorSet{
		BBUpper:  110,
		BBMiddle: 100,
		BBLower:  90,
		RSI14:    50, // neutral: no RSI signal
	}

	contains := func(list []string, want string) bool {
		for _, s := range list {
			if s == want {
				return true
			}
		}
		return false
	}

	// Price near lower band: pos = (92-90)/20 = 0.10 < 0.15 → bull reason fires.
	bull, _, _ := buildSignalReasons(base, 92)
	if !contains(bull, "Price near BB lower band") {
		t.Errorf("price=92: expected 'Price near BB lower band', got bull=%v", bull)
	}

	// Price at middle: pos = 0.5 → neither BB reason fires.
	bull, bear, _ := buildSignalReasons(base, 100)
	if contains(bull, "Price near BB lower band") {
		t.Errorf("price=100: unexpected 'Price near BB lower band', bull=%v", bull)
	}
	if contains(bear, "Price near BB upper band") {
		t.Errorf("price=100: unexpected 'Price near BB upper band', bear=%v", bear)
	}

	// Price near upper band: pos = (108-90)/20 = 0.90 > 0.85 → bear reason fires.
	_, bear, _ = buildSignalReasons(base, 108)
	if !contains(bear, "Price near BB upper band") {
		t.Errorf("price=108: expected 'Price near BB upper band', got bear=%v", bear)
	}
}
