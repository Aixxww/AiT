package local

import "log"

// ============================================================================
// Hunter v7 — Regime-aware module circuit breaker
// ============================================================================
// On 2026-07-26 (trend_up) funding_reversal and intraday_scalp_long produced
// 41.7% of all routed signals and zero open-review conversions across four
// consecutive rounds — every signal was emitted, scored, and then blocked by
// the same regime/geometry gates. A module that is regime-disfavoured AND has
// gone dry for several consecutive cycles is skipped until conditions change,
// releasing scoring work and prompt budget for routes that convert.
//
// Guard rails:
//   - Only modules whose regime weight is below the disfavour threshold are
//     ever skipped; a prime-regime module always runs regardless of history.
//   - Dry counts are evidence-based: they advance only on cycles where the
//     module actually emitted signals and none reached an open-review quality.
//   - Every few skipped cycles the module gets a probe round so a market turn
//     re-opens it within minutes rather than never.
//   - All breaker state resets on a regime change: dryness observed in
//     trend_up says nothing about panic_dump.

const (
	// v7BreakerDisfavourWeight marks a module as regime-disfavoured. 0.75
	// covers the observed dead weight (funding_reversal 0.7 in trend_up and
	// every counter-trend short at 0.4-0.6) while leaving neutral 1.0 routes
	// untouched.
	v7BreakerDisfavourWeight = 0.75
	// v7BreakerDryRounds is how many consecutive dry cycles trip the breaker.
	v7BreakerDryRounds = 3
	// v7BreakerProbeAfterSkips is how many cycles a tripped module sits out
	// before it gets one probe cycle to prove conditions changed.
	v7BreakerProbeAfterSkips = 3
)

type v7ModuleBreakerEntry struct {
	dryRounds int // consecutive scored cycles with signals but zero conversion
	skips     int // consecutive skipped cycles since the last probe
}

// beginBreakerCycle resets breaker state when the market regime changes.
func (m *V7SignalStateManager) beginBreakerCycle(regime V7MarketRegime) {
	if m.moduleBreaker == nil {
		m.moduleBreaker = make(map[V7SetupType]*v7ModuleBreakerEntry)
	}
	if regime != m.breakerRegime {
		m.moduleBreaker = make(map[V7SetupType]*v7ModuleBreakerEntry)
		m.breakerRegime = regime
	}
}

// ShouldSkipModule reports whether a regime-disfavoured module is on cooldown
// this cycle. Call once per module per cycle: skipped cycles are counted here
// so that every v7BreakerProbeAfterSkips+1-th cycle runs as a probe.
func (m *V7SignalStateManager) ShouldSkipModule(regime V7MarketRegime, setup V7SetupType) bool {
	if m == nil {
		return false
	}
	m.beginBreakerCycle(regime)
	entry := m.moduleBreaker[setup]
	if entry == nil || entry.dryRounds < v7BreakerDryRounds {
		return false
	}
	if entry.skips >= v7BreakerProbeAfterSkips {
		entry.skips = 0 // probe cycle: let the module run and re-earn its slot
		return false
	}
	entry.skips++
	return true
}

// RecordModuleConversions updates dry-spell counts from a cycle's raw module
// output. A module converts when at least one of its signals reached an
// open-review execution quality; a cycle with no output for a module is no
// evidence either way and leaves its count unchanged.
func (m *V7SignalStateManager) RecordModuleConversions(regime V7MarketRegime, signals []V7SignalOutput) {
	if m == nil {
		return
	}
	m.beginBreakerCycle(regime)

	emitted := make(map[V7SetupType]bool)
	converted := make(map[V7SetupType]bool)
	for i := range signals {
		sig := &signals[i]
		emitted[sig.SetupType] = true
		if sig.Status == V7StatusCandidate &&
			(sig.ExecutionQuality == V7ExecReady || sig.ExecutionQuality == V7ExecNearConfirm) {
			converted[sig.SetupType] = true
		}
	}

	for setup := range emitted {
		entry := m.moduleBreaker[setup]
		if entry == nil {
			entry = &v7ModuleBreakerEntry{}
			m.moduleBreaker[setup] = entry
		}
		if converted[setup] {
			entry.dryRounds = 0
			entry.skips = 0
			continue
		}
		entry.dryRounds++
		if entry.dryRounds == v7BreakerDryRounds {
			log.Printf("⛔ Hunter v7 breaker: %s dry for %d cycles in regime=%s, cooling down",
				setup, entry.dryRounds, regime)
		}
	}
}
