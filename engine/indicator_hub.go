package engine

import (
	"nofx/datafetch"
	"sort"
)

// ============================================================================
// Indicator Hub
// Orchestrates all three indicator layers (tech, quant, social).
// ============================================================================

// IndicatorHub manages indicator computation for all symbols.
type IndicatorHub struct {
	cfg   HubConfig
	store *datafetch.Store
}

// NewIndicatorHub creates a new IndicatorHub.
func NewIndicatorHub(store *datafetch.Store, cfg HubConfig) *IndicatorHub {
	return &IndicatorHub{
		cfg:   cfg,
		store: store,
	}
}

// ScoreSymbol computes all indicators for a single symbol snapshot.
func (h *IndicatorHub) ScoreSymbol(snap *datafetch.SymbolSnapshot) *IndicatorSet {
	return scoreSymbol(snap, h.cfg)
}

// ScoreAll computes indicators for all symbols in the current snapshot.
// Returns results sorted by FinalScore descending.
func (h *IndicatorHub) ScoreAll() []*IndicatorSet {
	snap := h.store.Current()
	if snap == nil || len(snap.Symbols) == 0 {
		return nil
	}

	sets := make([]*IndicatorSet, 0, len(snap.Symbols))

	for _, symSnap := range snap.Symbols {
		if symSnap == nil {
			continue
		}
		set := h.ScoreSymbol(symSnap)
		sets = append(sets, set)
	}

	// Sort by FinalScore descending
	sort.Slice(sets, func(i, j int) bool {
		return sets[i].FinalScore > sets[j].FinalScore
	})

	// Limit to TopN
	if h.cfg.TopNForScoring > 0 && len(sets) > h.cfg.TopNForScoring {
		sets = sets[:h.cfg.TopNForScoring]
	}

	return sets
}

// ScoreSymbols computes indicators for a specific list of symbols.
func (h *IndicatorHub) ScoreSymbols(symbols []string) []*IndicatorSet {
	snap := h.store.Current()
	if snap == nil {
		return nil
	}

	sets := make([]*IndicatorSet, 0, len(symbols))

	for _, sym := range symbols {
		symSnap, ok := snap.Symbols[sym]
		if !ok || symSnap == nil {
			continue
		}
		set := h.ScoreSymbol(symSnap)
		sets = append(sets, set)
	}

	sort.Slice(sets, func(i, j int) bool {
		return sets[i].FinalScore > sets[j].FinalScore
	})

	return sets
}
