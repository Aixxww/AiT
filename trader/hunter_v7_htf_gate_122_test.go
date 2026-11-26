package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
)

// #122: the execution guard backstops the HTF gate on the candidate itself.
func TestHTF122ExecutionGuard(t *testing.T) {
	run := func(setup, dir string, conf int, tagged bool) error {
		at := testRiskAutoTrader()
		at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
		c := kernel.CandidateCoin{Symbol: "HTFUSDT", Direction: dir, V7SetupType: setup, V7ExecutionTier: "REVIEWABLE",
			V7EntryZone: local.V7PriceZone{Lower: 99, Upper: 101}}
		if tagged {
			c.V7RiskTags = []string{local.V7HTFAgainstDirectionTag}
		}
		ctx := &kernel.Context{CandidateCoins: []kernel.CandidateCoin{c},
			MarketDataMap: map[string]*market.Data{"HTFUSDT": {Symbol: "HTFUSDT", CurrentPrice: 100}}}
		action := "open_long"
		if dir == "SHORT" {
			action = "open_short"
		}
		return at.validateHunterV7ExecutionGuard(ctx, &kernel.Decision{Symbol: "HTFUSDT", Action: action, Price: 100, Confidence: conf})
	}
	if err := run("leader_momentum_long", "LONG", 95, true); err == nil || !strings.Contains(err.Error(), "htf_against_trend_family") {
		t.Fatalf("trend family against 1h/4h: %v", err)
	}
	if err := run("distribution_short", "SHORT", 84, true); err == nil || !strings.Contains(err.Error(), "htf_against_counter_trend") {
		t.Fatalf("counter-trend at 84: %v", err)
	}
	// At 85 (and untagged) the HTF gate passes; any later rejection must not be the HTF gate.
	for _, c := range []struct {
		setup, dir string
		conf       int
		tagged     bool
	}{{"distribution_short", "SHORT", 85, true}, {"leader_momentum_long", "LONG", 75, false}} {
		if err := run(c.setup, c.dir, c.conf, c.tagged); err != nil && strings.Contains(err.Error(), "htf_against") {
			t.Fatalf("%s conf=%d tagged=%v blocked by the HTF gate: %v", c.setup, c.conf, c.tagged, err)
		}
	}
}
