package store

import "strings"

// U3 (P3) final direction pass configuration (owner decision OD-12:
// conflict_watch policy + CONTEXT non-voting).

// DefaultV7FinalConflictMinAIPriority: both best sides above this become one
// conflict_watch row. 60 is the native resolver's value (ResolveV7Conflicts),
// so the final pass applies the same rule across native and T2 signals.
const DefaultV7FinalConflictMinAIPriority = 60.0

// DefaultV7FinalConflictTieWinner: on an exact AIPriority tie the short side
// wins, as in the native resolver.
const DefaultV7FinalConflictTieWinner = "SHORT"

// EffectiveV7FinalConflictMinAIPriority returns the conflict_watch threshold.
func (h *HunterConfig) EffectiveV7FinalConflictMinAIPriority() float64 {
	if h != nil && h.V7FinalConflictMinAIPriority > 0 {
		return h.V7FinalConflictMinAIPriority
	}
	return DefaultV7FinalConflictMinAIPriority
}

// EffectiveV7FinalConflictTieWinner returns "LONG" or "SHORT".
func (h *HunterConfig) EffectiveV7FinalConflictTieWinner() string {
	if h != nil && strings.EqualFold(strings.TrimSpace(h.V7FinalConflictTieWinner), "LONG") {
		return "LONG"
	}
	return DefaultV7FinalConflictTieWinner
}

// V7ContextNonVoting reports whether CONTEXT-class signals are excluded from
// conflict votes (default true, OD-12).
func (h *HunterConfig) V7ContextNonVoting() bool {
	return h == nil || !h.V7ConflictContextVotes
}
