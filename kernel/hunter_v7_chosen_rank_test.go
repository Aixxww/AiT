package kernel

import "testing"

// #94 / AUD-B8: chosen_rank is the pick's 1-based position among the
// prompt's E+R candidates (prompt order), with the round's E+R count.
func TestHunterV7ChosenRankAndERCount(t *testing.T) {
	final := []HunterV7PromptFinalTier{
		{Symbol: "AAAUSDT", SetupType: "trend_breakout_long", SignalID: "a", Tier: "EXECUTABLE"},
		{Symbol: "WWWUSDT", SetupType: "pre_breakout_watch", SignalID: "w", Tier: "WATCH"},
		{Symbol: "BBBUSDT", SetupType: "pullback_long", SignalID: "b", Tier: "REVIEWABLE"},
		{Symbol: "CCCUSDT", SetupType: "panic_reversal_long", SignalID: "c", Tier: "REVIEWABLE"},
		{Symbol: "RRRUSDT", SetupType: "x", SignalID: "r", Tier: "REJECTED"},
	}
	cases := []struct {
		name      string
		decisions []Decision
		wantRank  int
		wantPer   []int
	}{
		{"pick #1", []Decision{{Symbol: "AAAUSDT", Action: "open_long", SelectedHunterV7SignalID: "a"}}, 1, []int{1}},
		{"pick #3 skips WATCH in the order", []Decision{{Symbol: "CCCUSDT", Action: "open_long", SelectedHunterV7SignalID: "c"}}, 3, []int{3}},
		{"symbol fallback without signal id", []Decision{{Symbol: "BBBUSDT", Action: "open_long"}}, 2, []int{2}},
		{"open outside E+R", []Decision{{Symbol: "WWWUSDT", Action: "open_long", SelectedHunterV7SignalID: "w"}}, -1, []int{-1}},
		{"wait only", []Decision{{Symbol: "ALL", Action: "wait"}}, 0, []int{0}},
		{"first open wins the round rank", []Decision{{Symbol: "ALL", Action: "wait"}, {Symbol: "BBBUSDT", Action: "open_long", SelectedHunterV7SignalID: "b"}, {Symbol: "AAAUSDT", Action: "open_short", SelectedHunterV7SignalID: "a"}}, 2, []int{0, 2, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			er, rank := annotateHunterV7ChosenRank(tc.decisions, final)
			if er != 3 || rank != tc.wantRank {
				t.Fatalf("er=%d rank=%d, want 3/%d", er, rank, tc.wantRank)
			}
			for i, d := range tc.decisions {
				if d.HunterV7ChosenRank != tc.wantPer[i] {
					t.Fatalf("decision %d rank %d, want %d", i, d.HunterV7ChosenRank, tc.wantPer[i])
				}
			}
		})
	}
	if er, rank := annotateHunterV7ChosenRank(nil, nil); er != 0 || rank != 0 {
		t.Fatalf("empty prompt: er=%d rank=%d", er, rank)
	}
}
