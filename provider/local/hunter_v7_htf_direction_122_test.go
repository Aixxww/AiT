package local

import (
	"testing"

	"github.com/Aixxww/AiT/store"
)

func TestHTF122BiasAndDirection(t *testing.T) {
	for _, c := range []struct {
		price, e20, e60 float64
		want            int
	}{
		{110, 105, 100, 1}, {90, 95, 100, -1}, {102, 105, 100, 0}, {110, 95, 100, 0}, {0, 105, 100, 0}, {110, 0, 100, 0},
	} {
		if got := V7HTFBias(c.price, c.e20, c.e60); got != c.want {
			t.Errorf("V7HTFBias(%v,%v,%v)=%d want %d", c.price, c.e20, c.e60, got, c.want)
		}
	}
	ctx := func(e1, e4 [2]float64) *V7SymbolContext {
		return &V7SymbolContext{CurrentPrice: 100, EMA20_1h: e1[0], EMA60_1h: e1[1], EMA20_4h: e4[0], EMA60_4h: e4[1]}
	}
	down, up, mixed := [2]float64{102, 105}, [2]float64{98, 95}, [2]float64{98, 105}
	cases := []struct {
		name   string
		e1, e4 [2]float64
		want   int
	}{
		{"both_down", down, down, -1}, {"both_up", up, up, 1},
		{"4h_down_1h_mixed", mixed, down, 0}, {"1h_down_4h_up", down, up, 0}, {"no_data", [2]float64{}, [2]float64{}, 0},
	}
	for _, c := range cases {
		if got := V7HTFDirection(ctx(c.e1, c.e4)); got != c.want {
			t.Errorf("%s: V7HTFDirection=%d want %d", c.name, got, c.want)
		}
	}
}

// The pass tags only gated setups, only when both 1h and 4h oppose.
func TestHTF122PassTagsGatedSetupsAgainstBothTimeframes(t *testing.T) {
	bear := &V7SymbolContext{CurrentPrice: 100, EMA20_1h: 102, EMA60_1h: 105, EMA20_4h: 102, EMA60_4h: 105}
	for _, c := range []struct {
		setup V7SetupType
		dir   V7Direction
		want  bool
	}{
		{V7SetupLeaderMomentumLong, V7DirLong, true},                  // trend family, against
		{V7SetupPullbackLong, V7DirLong, true},                        // counter-trend family, against
		{V7SetupBreakdownShort, V7DirShort, false},                    // with the HTF
		{V7SetupRangeExpansion, V7DirLong, false},                     // in neither list
		{V7SetupPreDistribution, V7DirShort, false},                   // in neither list
		{V7SetupType("momentum_continuation_long"), V7DirLong, false}, // T2: not covered
	} {
		sig := &V7SignalOutput{SetupType: c.setup, Direction: c.dir}
		applyV7HTFDirectionTag(sig, bear)
		if got := containsString(sig.RiskTags, V7HTFAgainstDirectionTag); got != c.want {
			t.Errorf("%s %s: tagged=%v want %v", c.setup, c.dir, got, c.want)
		}
	}
	// The pass is wired into the router pipeline, last.
	passes := buildV7SignalPasses(nil, V7RegimeRotation, V7Config{})
	if passes[len(passes)-1].name != "htf_direction" {
		t.Fatalf("htf_direction must be the last router pass, got %s", passes[len(passes)-1].name)
	}
	if action, ok := HunterV7TagLLMAction(V7HTFAgainstDirectionTag); !ok || action != V7TagActionReviewableOnly {
		t.Fatalf("catalog action = %q/%v", action, ok)
	}
}

// The family table is the S1 split and names only registered native setups.
func TestHTF122FamilyTable(t *testing.T) {
	fam := store.V7HTFFamilies()
	trend := []string{"leader_momentum_long", "short_squeeze_long", "displacement_momentum_long", "mms_trend_ride_long", "mms_squeeze_engine_long", "intraday_scalp_long",
		"trend_breakout_long", "accumulation_breakout_long", "volatility_squeeze_breakout", "pre_breakout_watch", "pre_squeeze_watch",
		"alt_ladder_momentum_long", "alt_ladder_breakdown_short", "breakdown_momentum_short", "relative_weakness_short"}
	counter := []string{"panic_reversal_long", "pullback_reversal_long", "whale_flow_reversal", "funding_reversal", "mms_bottom_wake_long", "accumulation_watch",
		"distribution_short", "long_squeeze_short", "range_reversion"}
	for _, s := range trend {
		if fam[s] != store.V7HTFFamilyTrend {
			t.Errorf("%s: family %q, want trend", s, fam[s])
		}
	}
	for _, s := range counter {
		if fam[s] != store.V7HTFFamilyCounterTrend {
			t.Errorf("%s: family %q, want counter_trend", s, fam[s])
		}
	}
	if len(fam) != len(trend)+len(counter) {
		t.Errorf("table has %d rows, want %d", len(fam), len(trend)+len(counter))
	}
	for _, s := range []string{"range_expansion_event", "pre_distribution_watch"} {
		if fam[s] != "" {
			t.Errorf("%s must be in neither list", s)
		}
	}
	reg := store.DefaultV7SignalClassRegistry()
	for s := range fam {
		if _, ok := reg[s]; !ok && s != "mms_trend_ride_long" && s != "mms_squeeze_engine_long" {
			t.Errorf("%s is not a known native setup", s)
		}
	}
	if store.V7CounterTrendMinConfidence != 85 {
		t.Fatalf("counter-trend floor = %d, want the owner-set 85", store.V7CounterTrendMinConfidence)
	}
}
