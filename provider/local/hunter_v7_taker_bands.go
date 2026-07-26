package local

// Hunter v7 taker scoring bands (U6.4).
//
// Each module's taker ladder was a hand-rolled descending else-if chain over
// TakerBuy15m. The chains become data rows here: first matching row wins,
// mirroring else-if semantics exactly. Tags stay in the historical spelling
// until the U6.3 emission flip; when that lands, only this table changes.
//
// The rows live in a per-setup map rather than inside V7SetupThresholds
// because the ladder is scoring-time data (module layer), while
// V7SetupThresholds gates execution-time policy — merging them would couple
// two different consumers to one table for cosmetic reasons.

// V7TakerBandRow is one ladder rung: match when TakerBuy15m clears Threshold
// (strictly when Exclusive, else inclusive), award Points and record Tag.
// A Threshold of 0 with Exclusive=false is the unconditional tail rung.
type V7TakerBandRow struct {
	Threshold float64
	Exclusive bool
	Points    float64
	Tag       string
}

// v7TakerLadders holds the per-setup taker scoring ladders migrated so far.
// Setups absent here keep their taker logic inside the module (guards,
// derived ratios, or non-ladder shapes).
var v7TakerLadders = map[V7SetupType][]V7TakerBandRow{
	V7SetupShortSqueezeLong: {
		{Threshold: 0.65, Exclusive: true, Points: 25, Tag: "taker_aggressive_buy"},
		{Threshold: 0.60, Exclusive: true, Points: 20, Tag: "taker_strong_buy"},
		{Threshold: 0.55, Exclusive: true, Points: 15, Tag: "taker_moderate_buy"},
	},
	V7SetupTrendBreakoutLong: {
		{Threshold: 0.60, Exclusive: true, Points: 15, Tag: "taker_aggressive_buy"},
		{Threshold: 0.55, Exclusive: true, Points: 12, Tag: "taker_strong_buy"},
		{Threshold: 0.52, Exclusive: true, Points: 8, Tag: "taker_moderate_buy"},
		{Points: 3},
	},
	V7SetupLeaderMomentumLong: {
		{Threshold: 0.60, Exclusive: true, Points: 15, Tag: "taker_sustained_buy"},
		{Threshold: 0.55, Exclusive: true, Points: 12, Tag: "taker_strong_buy"},
		{Threshold: 0.50, Exclusive: true, Points: 8, Tag: "taker_neutral_buy"},
		{Points: 3, Tag: "taker_weak_buy"},
	},
	V7SetupDisplacementLong: {
		{Threshold: 0.55, Points: 15, Tag: "taker_buy_aggressive"},
		{Threshold: 0.52, Points: 10, Tag: "taker_buy_aligned"},
		{Threshold: 0.50, Points: 5, Tag: "taker_buy_neutral"},
	},
	V7SetupPanicReversalLong: {
		{Threshold: 0.58, Exclusive: true, Points: 15, Tag: "taker_buy_aggressive"},
		{Threshold: 0.54, Exclusive: true, Points: 12, Tag: "taker_buy_strong"},
		{Threshold: 0.51, Exclusive: true, Points: 8, Tag: "taker_buy_recovering"},
		{Threshold: 0.48, Exclusive: true, Points: 3, Tag: "taker_buy_neutral"},
	},
	V7SetupPullbackLong: {
		{Threshold: 0.55, Exclusive: true, Points: 15, Tag: "taker_buy_strong"},
		{Threshold: 0.52, Exclusive: true, Points: 10, Tag: "taker_buy_recovering"},
		{Threshold: 0.50, Exclusive: true, Points: 5, Tag: "taker_buy_neutral"},
	},
}

// takerLadder scores the context's TakerBuy15m against the rows: the first
// matching rung contributes its points and tag, non-matching ladders add
// nothing (identical to the original else-if chains).
func (s *v7SignalScaffold) takerLadder(rows []V7TakerBandRow) *v7SignalScaffold {
	value := s.ctx.TakerBuy15m
	for _, row := range rows {
		matched := value >= row.Threshold
		if row.Exclusive {
			matched = value > row.Threshold
		}
		if !matched {
			continue
		}
		if row.Tag != "" {
			return s.add(row.Points, row.Tag)
		}
		s.score += row.Points
		return s
	}
	return s
}
