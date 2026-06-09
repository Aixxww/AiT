package local

// ============================================================================
// Hunter v7 — Watch Signal State Machine
// ============================================================================
// Tracks watch signals across cycles and promotes them through escalating
// states when confirmation conditions accumulate over consecutive rounds.
//
// State progression:
//   WATCH_SEEN → WATCH_STRENGTHENING → NEAR_CONFIRM → REVIEWABLE → EXECUTABLE
//   Any state → EXPIRED (signal disappeared for 3+ cycles)
//   Any state → FAILED (invalidation price broken)

// V7WatchState is the escalating state of a watch signal across cycles.
type V7WatchState string

const (
	V7WatchSeen          V7WatchState = "WATCH_SEEN"
	V7WatchStrengthening V7WatchState = "WATCH_STRENGTHENING"
	V7WatchNearConfirm   V7WatchState = "NEAR_CONFIRM"
	V7WatchReviewable    V7WatchState = "REVIEWABLE"
	V7WatchExecutable    V7WatchState = "EXECUTABLE"
	V7WatchExpired       V7WatchState = "EXPIRED"
	V7WatchFailed        V7WatchState = "FAILED"
)

// v7WatchEntry tracks a single watch signal across scoring cycles.
type v7WatchEntry struct {
	Key        string // symbol|setup
	Symbol     string
	Setup      V7SetupType
	Direction  V7Direction
	State      V7WatchState
	FirstSeen  int // cycle number first seen
	LastSeen   int // cycle number last seen
	SeenCount  int // consecutive cycles seen
	LastSignal V7SignalOutput
}

// V7SignalStateManager maintains cross-cycle state for watch signals.
type V7SignalStateManager struct {
	entries   map[string]*v7WatchEntry
	lastCycle int
	expireGap int // cycles without sighting before expiry (default 3)
}

// NewV7SignalStateManager creates a new state manager.
func NewV7SignalStateManager() *V7SignalStateManager {
	return &V7SignalStateManager{
		entries:   make(map[string]*v7WatchEntry),
		expireGap: 3,
	}
}

// Process takes the full ScoreHunterV7 output for a cycle and updates watch
// states. It returns upgraded signals that have reached REVIEWABLE or higher.
func (m *V7SignalStateManager) Process(signals []V7SignalOutput, cycleNumber int) []V7SignalOutput {
	m.lastCycle = cycleNumber

	// Build current-cycle watch map
	currentWatches := make(map[string]*V7SignalOutput)
	for i := range signals {
		sig := &signals[i]
		if !isWatchSetup(sig.SetupType) {
			continue
		}
		key := sig.Symbol + "|" + string(sig.SetupType)
		currentWatches[key] = sig
	}

	// Update existing entries: expire missing, upgrade present
	for key, entry := range m.entries {
		if entry.State == V7WatchExpired || entry.State == V7WatchFailed {
			continue
		}
		sig, present := currentWatches[key]
		if !present {
			// Not seen this cycle
			if cycleNumber-entry.LastSeen >= m.expireGap {
				entry.State = V7WatchExpired
			}
			continue
		}
		entry.LastSeen = cycleNumber
		entry.SeenCount++
		entry.LastSignal = *sig
		m.upgradeState(entry, sig)
	}

	// Register new watches
	for key, sig := range currentWatches {
		if _, exists := m.entries[key]; !exists {
			m.entries[key] = &v7WatchEntry{
				Key:        key,
				Symbol:     sig.Symbol,
				Setup:      sig.SetupType,
				Direction:  sig.Direction,
				State:      V7WatchSeen,
				FirstSeen:  cycleNumber,
				LastSeen:   cycleNumber,
				SeenCount:  1,
				LastSignal: *sig,
			}
		}
	}

	// Collect upgraded signals (REVIEWABLE or EXECUTABLE)
	var upgraded []V7SignalOutput
	for _, entry := range m.entries {
		if entry.State == V7WatchReviewable || entry.State == V7WatchExecutable {
			sig := entry.LastSignal
			m.applyUpgrade(&sig, entry)
			upgraded = append(upgraded, sig)
		}
	}
	return upgraded
}

// upgradeState advances the watch state based on cumulative evidence.
func (m *V7SignalStateManager) upgradeState(entry *v7WatchEntry, sig *V7SignalOutput) {
	switch entry.State {
	case V7WatchSeen:
		if entry.SeenCount >= 2 && watchStrengtheningConditions(sig) {
			entry.State = V7WatchStrengthening
		}
	case V7WatchStrengthening:
		if entry.SeenCount >= 3 && watchNearConfirmConditions(sig, entry) {
			entry.State = V7WatchNearConfirm
		}
		// Can skip to NEAR_CONFIRM if strong conditions met in fewer cycles
		if watchStrongUpgradeSignal(sig) && entry.SeenCount >= 2 {
			entry.State = V7WatchNearConfirm
		}
	case V7WatchNearConfirm:
		if watchReviewableTrigger(sig, entry) {
			entry.State = V7WatchReviewable
		}
	case V7WatchReviewable:
		// REVIEWABLE stays until it either executes or expires naturally
		// After 5+ cycles at REVIEWABLE without execution, degrade
		if entry.SeenCount > 10 {
			entry.State = V7WatchExpired
		}
	}
}

// applyUpgrade modifies the signal output to reflect the upgraded state.
func (m *V7SignalStateManager) applyUpgrade(sig *V7SignalOutput, entry *v7WatchEntry) {
	switch entry.State {
	case V7WatchReviewable:
		sig.ExecutionQuality = V7ExecNearConfirm
		sig.Confidence = "B"
		sig.Status = V7StatusCandidate
		sig.RiskTags = removeTag(sig.RiskTags, "do_not_open_until_confirmed")
		sig.RiskTags = removeTag(sig.RiskTags, "watch_only")
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "watch_upgraded_reviewable")
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "multi_cycle_confirmation")
	case V7WatchExecutable:
		sig.ExecutionQuality = V7ExecReady
		sig.Confidence = "A"
		sig.Status = V7StatusCandidate
		sig.RiskTags = removeTag(sig.RiskTags, "do_not_open_until_confirmed")
		sig.RiskTags = removeTag(sig.RiskTags, "watch_only")
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "watch_upgraded_executable")
	}
}

// watchStrengtheningConditions checks if a watch signal is accumulating evidence.
func watchStrengtheningConditions(sig *V7SignalOutput) bool {
	if sig == nil {
		return false
	}
	switch sig.SetupType {
	case V7SetupPreBreakoutWatch:
		// BB still compressed + OI still building
		return containsV7String(sig.ReasonCodes, "compressed_oi_pre_breakout") ||
			containsV7String(sig.ReasonCodes, "oi_4h_stealth_build")

	case V7SetupAccumulationWatch:
		// OI continues building without price markup
		return containsV7String(sig.ReasonCodes, "oi_build_without_price_markup") ||
			containsV7String(sig.ReasonCodes, "oi_1h_confirming_accumulation")

	case V7SetupPreSqueezeWatch:
		// Short crowding persists, sell pressure stalling
		return containsV7String(sig.ReasonCodes, "negative_funding_short_crowding") ||
			containsV7String(sig.ReasonCodes, "sell_pressure_stalling")

	case V7SetupPreDistribution:
		// Crowded longs persist near resistance
		return containsV7String(sig.ReasonCodes, "crowded_longs_near_resistance") ||
			containsV7String(sig.ReasonCodes, "funding_long_crowding")
	}
	return false
}

// watchNearConfirmConditions checks for near-confirmation evidence.
func watchNearConfirmConditions(sig *V7SignalOutput, entry *v7WatchEntry) bool {
	if sig == nil {
		return false
	}
	switch sig.SetupType {
	case V7SetupPreBreakoutWatch:
		// Taker buy improving + still near trigger
		return containsV7String(sig.ReasonCodes, "taker_buy_bias_before_breakout") ||
			containsV7String(sig.ReasonCodes, "taker_buy_improving")

	case V7SetupAccumulationWatch:
		// BB width starting to expand or taker buy confirming
		return containsV7String(sig.ReasonCodes, "quiet_1h_price_action") &&
			containsV7String(sig.ReasonCodes, "oi_1h_confirming_accumulation")

	case V7SetupPreSqueezeWatch:
		// Taker buy recovering + price holding
		return containsV7String(sig.ReasonCodes, "taker_buy_recovery_before_squeeze")

	case V7SetupPreDistribution:
		// Taker buy weakening + rally stalling
		return containsV7String(sig.ReasonCodes, "taker_buy_weakening") &&
			containsV7String(sig.ReasonCodes, "rally_stalling_near_high")
	}
	return false
}

// watchStrongUpgradeSignal checks for strong signals that can fast-track upgrade.
func watchStrongUpgradeSignal(sig *V7SignalOutput) bool {
	if sig == nil {
		return false
	}
	// Multiple confirming conditions present simultaneously
	confirmCount := 0
	strongCodes := []string{
		"taker_buy_bias_before_breakout", "taker_buy_recovery_before_squeeze",
		"oi_4h_stealth_build", "funding_not_crowded",
		"lsr_short_crowded", "funding_long_crowding",
	}
	for _, code := range strongCodes {
		if containsV7String(sig.ReasonCodes, code) {
			confirmCount++
		}
	}
	return confirmCount >= 2
}

// watchReviewableTrigger checks if a near-confirm watch should become reviewable.
func watchReviewableTrigger(sig *V7SignalOutput, entry *v7WatchEntry) bool {
	if sig == nil {
		return false
	}
	// Minimum: seen for 4+ cycles total
	if entry.SeenCount < 4 {
		return false
	}

	switch sig.SetupType {
	case V7SetupPreBreakoutWatch:
		// Price at or above trigger + taker buy aligned
		if sig.PriceCtx != nil && sig.EntryZone.Upper > 0 &&
			sig.PriceCtx.Last >= sig.EntryZone.Upper*0.995 {
			return true
		}
		return containsV7String(sig.ReasonCodes, "taker_buy_bias_before_breakout") &&
			entry.SeenCount >= 5

	case V7SetupAccumulationWatch:
		// BB starting to expand + OI continues
		return entry.SeenCount >= 4 &&
			containsV7String(sig.ReasonCodes, "oi_1h_confirming_accumulation")

	case V7SetupPreSqueezeWatch:
		// Price reclaims VWAP + taker buy recovering
		return containsV7String(sig.ReasonCodes, "taker_buy_recovery_before_squeeze") &&
			entry.SeenCount >= 4

	case V7SetupPreDistribution:
		// Clear rejection at resistance + taker sell
		return containsV7String(sig.ReasonCodes, "taker_buy_weakening") &&
			containsV7String(sig.ReasonCodes, "rally_stalling_near_high") &&
			entry.SeenCount >= 4
	}
	return false
}

// isWatchSetup returns true for the 4 pre-move watch setup types.
func isWatchSetup(setup V7SetupType) bool {
	switch setup {
	case V7SetupPreBreakoutWatch, V7SetupPreSqueezeWatch,
		V7SetupPreDistribution, V7SetupAccumulationWatch:
		return true
	}
	return false
}

// removeTag removes a tag from a slice (first occurrence only).
func removeTag(tags []string, tag string) []string {
	for i, t := range tags {
		if t == tag {
			return append(tags[:i], tags[i+1:]...)
		}
	}
	return tags
}
