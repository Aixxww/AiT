package local

import "testing"

func breakerDrySignal(setup V7SetupType) V7SignalOutput {
	return V7SignalOutput{
		Symbol:           "TESTUSDT",
		SetupType:        setup,
		Status:           V7StatusWaitConfirm,
		ExecutionQuality: V7ExecWatchOnly,
	}
}

func breakerConvertedSignal(setup V7SetupType) V7SignalOutput {
	return V7SignalOutput{
		Symbol:           "TESTUSDT",
		SetupType:        setup,
		Status:           V7StatusCandidate,
		ExecutionQuality: V7ExecReady,
	}
}

// The 2026-07-26 pattern: funding_reversal emitted watch_only signals four
// cycles straight in trend_up. Three dry cycles trip the breaker, the module
// sits out three cycles, then gets a probe cycle.
func TestModuleBreakerTripsAfterConsecutiveDryCycles(t *testing.T) {
	m := NewV7SignalStateManager()
	setup := V7SetupFundingReversal

	for cycle := 0; cycle < v7BreakerDryRounds; cycle++ {
		if m.ShouldSkipModule(V7RegimeTrendUp, setup) {
			t.Fatalf("cycle %d: breaker tripped before %d dry cycles", cycle, v7BreakerDryRounds)
		}
		m.RecordModuleConversions(V7RegimeTrendUp, []V7SignalOutput{breakerDrySignal(setup)})
	}

	skips := 0
	for cycle := 0; cycle < v7BreakerProbeAfterSkips; cycle++ {
		if !m.ShouldSkipModule(V7RegimeTrendUp, setup) {
			t.Fatalf("skip cycle %d: expected cooldown skip", cycle)
		}
		skips++
	}
	if m.ShouldSkipModule(V7RegimeTrendUp, setup) {
		t.Fatalf("after %d skips the next cycle must be a probe", skips)
	}
}

func TestModuleBreakerResetsOnConversion(t *testing.T) {
	m := NewV7SignalStateManager()
	setup := V7SetupFundingReversal

	for i := 0; i < v7BreakerDryRounds-1; i++ {
		m.RecordModuleConversions(V7RegimeTrendUp, []V7SignalOutput{breakerDrySignal(setup)})
	}
	// One converted signal wipes the dry spell even alongside dry ones.
	m.RecordModuleConversions(V7RegimeTrendUp, []V7SignalOutput{
		breakerDrySignal(setup),
		breakerConvertedSignal(setup),
	})
	for i := 0; i < v7BreakerDryRounds-1; i++ {
		m.RecordModuleConversions(V7RegimeTrendUp, []V7SignalOutput{breakerDrySignal(setup)})
		if m.ShouldSkipModule(V7RegimeTrendUp, setup) {
			t.Fatal("dry count must restart from zero after a conversion")
		}
	}
}

func TestModuleBreakerClearsOnRegimeChange(t *testing.T) {
	m := NewV7SignalStateManager()
	setup := V7SetupFundingReversal

	for i := 0; i < v7BreakerDryRounds; i++ {
		m.RecordModuleConversions(V7RegimeTrendUp, []V7SignalOutput{breakerDrySignal(setup)})
	}
	if !m.ShouldSkipModule(V7RegimeTrendUp, setup) {
		t.Fatal("breaker should be tripped in trend_up")
	}
	// panic_dump is funding_reversal's prime regime; trend_up dryness is not
	// evidence there.
	if m.ShouldSkipModule(V7RegimePanicDump, setup) {
		t.Fatal("regime change must clear breaker state")
	}
}

func TestModuleBreakerNoEmissionIsNoEvidence(t *testing.T) {
	m := NewV7SignalStateManager()
	setup := V7SetupFundingReversal

	for i := 0; i < v7BreakerDryRounds*2; i++ {
		// Cycles where the module emitted nothing at all.
		m.RecordModuleConversions(V7RegimeTrendUp, nil)
	}
	if m.ShouldSkipModule(V7RegimeTrendUp, setup) {
		t.Fatal("cycles without emission must not advance the dry count")
	}
}

func TestModuleBreakerNilManagerIsInert(t *testing.T) {
	var m *V7SignalStateManager
	if m.ShouldSkipModule(V7RegimeTrendUp, V7SetupFundingReversal) {
		t.Fatal("nil manager must never skip")
	}
	m.RecordModuleConversions(V7RegimeTrendUp, nil) // must not panic
}
