package kernel

import (
	"strings"

	"github.com/Aixxww/AiT/market"
)

// #94 / AUD-B8: X2 instrumentation. The open-review pool (EXECUTABLE +
// REVIEWABLE) is shown to the LLM in prompt order (tier, readiness,
// ai_priority). Recording where the pick sits in that order, and how many
// E+R candidates there were, makes the rubber-stamp measurement possible:
// compare P(pick == #1) with the share of #1 among passable candidates.

func isHunterV7OpenReviewTier(tier string) bool {
	return strings.EqualFold(tier, "EXECUTABLE") || strings.EqualFold(tier, "REVIEWABLE")
}

// annotateHunterV7ChosenRank sets Decision.HunterV7ChosenRank on every open
// and returns the round's E+R count and the first open's rank.
//
// An open is matched by its selected_hunter_v7_signal_id; without one, by a
// symbol that appears exactly once among the E+R candidates. Rank is
// 1-based; -1 means the open's signal is not an E+R candidate of this
// prompt; 0 (non-opens, or no open) means nothing was chosen.
func annotateHunterV7ChosenRank(decisions []Decision, promptFinal []HunterV7PromptFinalTier) (erCount, chosenRank int) {
	byID := map[string]int{}
	bySymbol := map[string]int{}
	symbolSeen := map[string]int{}
	for _, t := range promptFinal {
		if !isHunterV7OpenReviewTier(t.Tier) {
			continue
		}
		erCount++
		if t.SignalID != "" {
			if _, dup := byID[t.SignalID]; !dup {
				byID[t.SignalID] = erCount
			}
		}
		sym := market.Normalize(t.Symbol)
		symbolSeen[sym]++
		bySymbol[sym] = erCount
	}
	for i := range decisions {
		d := &decisions[i]
		if d.Action != "open_long" && d.Action != "open_short" {
			d.HunterV7ChosenRank = 0
			continue
		}
		rank := -1
		if id := strings.TrimSpace(d.SelectedHunterV7SignalID); id != "" {
			if r, ok := byID[id]; ok {
				rank = r
			}
		} else if sym := market.Normalize(d.Symbol); symbolSeen[sym] == 1 {
			rank = bySymbol[sym]
		}
		d.HunterV7ChosenRank = rank
		if chosenRank == 0 {
			chosenRank = rank
		}
	}
	return erCount, chosenRank
}
