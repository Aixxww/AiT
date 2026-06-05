package local

// ============================================================================
// Hunter v7 — Conflict Resolver
// ============================================================================
// When the same symbol gets both LONG and SHORT signals with high scores,
// it means the market is at a critical decision point. Instead of blindly
// picking the higher score, we output a conflict_watch status that tells
// the AI engine to wait for directional confirmation.

// ResolveV7Conflicts processes signals to detect and resolve direction conflicts.
func ResolveV7Conflicts(signals []V7SignalOutput) []V7SignalOutput {
	// Group by symbol
	type symbolSignals struct {
		longs  []*V7SignalOutput
		shorts []*V7SignalOutput
	}

	groups := make(map[string]*symbolSignals)
	for i := range signals {
		s := &signals[i]
		if groups[s.Symbol] == nil {
			groups[s.Symbol] = &symbolSignals{}
		}
		if s.Direction == V7DirLong {
			groups[s.Symbol].longs = append(groups[s.Symbol].longs, s)
		} else {
			groups[s.Symbol].shorts = append(groups[s.Symbol].shorts, s)
		}
	}

	var result []V7SignalOutput

	for sym, grp := range groups {
		_ = sym
		bestLong := bestSignal(grp.longs)
		bestShort := bestSignal(grp.shorts)

		switch {
		case bestLong != nil && bestShort == nil:
			// Only long signals
			result = append(result, *bestLong)

		case bestLong == nil && bestShort != nil:
			// Only short signals
			result = append(result, *bestShort)

		case bestLong != nil && bestShort != nil:
			// Both directions — check for conflict
			if bestLong.AIPriority > 60 && bestShort.AIPriority > 60 {
				// High conflict: mark as conflict_watch
				conflict := V7SignalOutput{
					Symbol:         bestLong.Symbol,
					Direction:      bestLong.Direction, // primary direction
					SetupType:      bestLong.SetupType,
					Status:         V7StatusConflictWatch,
					SetupScore:     maxFloat(bestLong.SetupScore, bestShort.SetupScore),
					AIPriority:     (bestLong.AIPriority + bestShort.AIPriority) / 2,
					MarketRegime:   bestLong.MarketRegime,
					Confidence:     "C",
					ReasonCodes:    []string{"directional_conflict"},
					RiskTags:       []string{"do_not_market_chase"},
					EntryMode:      V7EntryWaitConfirm,
					PriceCtx:       bestLong.PriceCtx,
					DerivativesCtx: bestLong.DerivativesCtx,
					RequiredConfirms: []string{
						"wait_for_directional_break",
						"long_trigger: break_high_with_oi_increase",
						"short_trigger: lose_vwap_with_taker_sell",
					},
				}
				result = append(result, conflict)
			} else if bestLong.AIPriority > bestShort.AIPriority {
				result = append(result, *bestLong)
			} else {
				result = append(result, *bestShort)
			}

		default:
			// No signals for this symbol
		}
	}

	return result
}

// bestSignal returns the signal with highest AIPriority from a slice.
func bestSignal(signals []*V7SignalOutput) *V7SignalOutput {
	if len(signals) == 0 {
		return nil
	}
	best := signals[0]
	for _, s := range signals[1:] {
		if s.AIPriority > best.AIPriority {
			best = s
		}
	}
	return best
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
