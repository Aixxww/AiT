package local

import (
	"strings"

	"github.com/Aixxww/AiT/store"
)

// U3 (P3): one direction per symbol per cycle, covering native-vs-T2 pairs.
//
// The router resolves native conflicts (ResolveV7Conflicts) before the
// Task-2 module appends its signals, so a native signal and a T2 signal on
// the same symbol in opposite directions both reached the LLM (measured:
// 62/694 snapshots at final-signal level). ResolveV7FinalDirectionConflicts
// is a final pass over the merged list, run only when the T2 module is on
// and only on symbols that carry a T2 signal, so native-only output is
// unchanged.
//
// Rule (owner decision OD-12: conflict_watch + CONTEXT non-voting):
//   - CONTEXT-class signals (e.g. funding_reversal, OD-13) do not vote. A
//     CONTEXT signal opposite the surviving direction is dropped; it never
//     suppresses a tradable signal.
//   - Voting signals in both directions: take the best long and best short
//     by AIPriority. If both exceed MinAIPriority, every voting signal of the
//     symbol is replaced by one conflict_watch row (the native resolver's
//     rule and row). Otherwise the side with the higher best AIPriority is
//     kept and the other side dropped; on an exact tie TieWinner wins.
//   - A router conflict_watch row already on the symbol absorbs the T2
//     signals (the router judged both native sides strong).
//   - No voting signal (CONTEXT only) and both directions: the CONTEXT
//     signals are resolved among themselves with the same rule.

// V7FinalConflictConfig configures the final pass.
type V7FinalConflictConfig struct {
	Enabled          bool
	MinAIPriority    float64         // both best sides above this -> conflict_watch
	TieWinner        V7Direction     // exact AIPriority tie winner (V7DirShort or V7DirLong)
	ContextNonVoting bool            // CONTEXT signals never suppress voting signals
	ContextSetups    map[string]bool // setup types of the CONTEXT class
}

// V7ConflictResolution records one resolved symbol for diagnostics and the
// acceptance re-run (win/loss breakdown by family pair).
type V7ConflictResolution struct {
	Symbol  string   `json:"symbol"`
	Outcome string   `json:"outcome"` // conflict_watch | kept_long | kept_short | absorbed_into_conflict_watch | context_dropped
	Kept    []string `json:"kept,omitempty"`
	Dropped []string `json:"dropped,omitempty"`
}

func v7ConflictLabel(s V7SignalOutput) string {
	stack := s.Stack
	if stack == "" {
		stack = "native"
	}
	return stack + ":" + string(s.SetupType) + ":" + string(s.Direction)
}

// ResolveV7FinalDirectionConflicts applies the final one-direction pass.
// Signal order is preserved; a conflict_watch row takes the position of the
// symbol's first voting signal.
func ResolveV7FinalDirectionConflicts(signals []V7SignalOutput, cfg V7FinalConflictConfig) ([]V7SignalOutput, []V7ConflictResolution) {
	if !cfg.Enabled || len(signals) == 0 {
		return signals, nil
	}
	bySymbol := map[string][]int{}
	var order []string
	for i, s := range signals {
		if _, ok := bySymbol[s.Symbol]; !ok {
			order = append(order, s.Symbol)
		}
		bySymbol[s.Symbol] = append(bySymbol[s.Symbol], i)
	}

	drop := make([]bool, len(signals))
	replace := map[int]V7SignalOutput{}
	var resolutions []V7ConflictResolution

	for _, sym := range order {
		idx := bySymbol[sym]
		hasT2 := false
		for _, i := range idx {
			if signals[i].Stack == V7StackT2 {
				hasT2 = true
				break
			}
		}
		if !hasT2 {
			continue // native-only symbol: the router already resolved it
		}
		if res, ok := resolveV7FinalSymbol(signals, idx, cfg, drop, replace); ok {
			res.Symbol = sym
			resolutions = append(resolutions, res)
		}
	}

	out := make([]V7SignalOutput, 0, len(signals))
	for i, s := range signals {
		if r, ok := replace[i]; ok {
			out = append(out, r)
			continue
		}
		if !drop[i] {
			out = append(out, s)
		}
	}
	return out, resolutions
}

func (cfg V7FinalConflictConfig) isContext(s V7SignalOutput) bool {
	if !cfg.ContextNonVoting {
		return false
	}
	ctx, _ := store.LookupV7Setup(cfg.ContextSetups, string(s.SetupType))
	return ctx
}

func resolveV7FinalSymbol(signals []V7SignalOutput, idx []int, cfg V7FinalConflictConfig, drop []bool, replace map[int]V7SignalOutput) (V7ConflictResolution, bool) {
	var voting, context []int
	conflictRow := -1
	for _, i := range idx {
		switch {
		case signals[i].Status == V7StatusConflictWatch && conflictRow < 0:
			conflictRow = i
		case cfg.isContext(signals[i]):
			context = append(context, i)
		default:
			voting = append(voting, i)
		}
	}
	res := V7ConflictResolution{}
	dropIdx := func(i int) {
		drop[i] = true
		res.Dropped = append(res.Dropped, v7ConflictLabel(signals[i]))
	}

	// A router conflict_watch row absorbs everything else on the symbol.
	if conflictRow >= 0 {
		if len(voting)+len(context) == 0 {
			return res, false
		}
		for _, i := range append(append([]int{}, voting...), context...) {
			dropIdx(i)
		}
		res.Outcome = "absorbed_into_conflict_watch"
		res.Kept = []string{v7ConflictLabel(signals[conflictRow])}
		return res, true
	}

	// CONTEXT only: resolve the CONTEXT signals among themselves.
	if len(voting) == 0 {
		voting, context = context, nil
	}
	bestLong, bestShort := -1, -1
	for _, i := range voting {
		switch signals[i].Direction {
		case V7DirLong:
			if bestLong < 0 || signals[i].AIPriority > signals[bestLong].AIPriority {
				bestLong = i
			}
		case V7DirShort:
			if bestShort < 0 || signals[i].AIPriority > signals[bestShort].AIPriority {
				bestShort = i
			}
		}
	}

	var winner V7Direction
	switch {
	case bestLong >= 0 && bestShort >= 0:
		l, s := signals[bestLong], signals[bestShort]
		if l.AIPriority > cfg.MinAIPriority && s.AIPriority > cfg.MinAIPriority {
			first := voting[0]
			replace[first] = newV7ConflictWatchSignal(&l, &s)
			for _, i := range voting {
				if i != first {
					drop[i] = true
				}
				res.Dropped = append(res.Dropped, v7ConflictLabel(signals[i]))
			}
			for _, i := range context {
				dropIdx(i)
			}
			res.Outcome = "conflict_watch"
			return res, true
		}
		switch {
		case l.AIPriority > s.AIPriority:
			winner = V7DirLong
		case s.AIPriority > l.AIPriority:
			winner = V7DirShort
		default:
			winner = cfg.TieWinner
			if winner != V7DirLong {
				winner = V7DirShort
			}
		}
	case bestLong >= 0:
		winner = V7DirLong
	case bestShort >= 0:
		winner = V7DirShort
	default:
		return res, false
	}

	votingDropped := false
	for _, i := range voting {
		if signals[i].Direction == winner {
			res.Kept = append(res.Kept, v7ConflictLabel(signals[i]))
		} else {
			dropIdx(i)
			votingDropped = true
		}
	}
	for _, i := range context {
		if signals[i].Direction != winner {
			dropIdx(i)
		}
	}
	switch {
	case len(res.Dropped) == 0:
		return res, false // one direction already: nothing to resolve
	case votingDropped:
		res.Outcome = "kept_" + strings.ToLower(string(winner))
	default:
		res.Outcome = "context_dropped"
	}
	return res, true
}
