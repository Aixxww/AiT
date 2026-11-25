package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// #105 / AUD-B4: the trader's open check reads the same resolved floor as the
// kernel validator (store.ResolveMinConfidence). Configured 75 rejects
// confidence 70; unset resolves to 60. The kernel half of the same table is
// kernel.TestAudit_B4_KernelConfidenceFloorIs60 (kernel cannot be tested
// from here without exporting its validator).
func TestConfidenceFloorSingleSourced(t *testing.T) {
	open := func(conf int) *kernel.Decision {
		return &kernel.Decision{Symbol: "XUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 100,
			Price: 100, StopLoss: 98, TakeProfit: 104, Confidence: conf}
	}
	for _, tc := range []struct {
		configured, floor int
	}{{75, 75}, {0, 60}} {
		at := testRiskAutoTrader()
		at.config.StrategyConfig.RiskControl.MinConfidence = tc.configured
		at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.2
		if got := at.effectiveMinOpenConfidence(tc.configured); got != tc.floor {
			t.Fatalf("configured %d: trader floor %d, want %d", tc.configured, got, tc.floor)
		}
		if r := store.ResolveHunterV7(at.config.StrategyConfig); r.MinConfidence != tc.floor {
			t.Fatalf("configured %d: resolver %d, want %d", tc.configured, r.MinConfidence, tc.floor)
		}
		below := tc.floor - 1
		if tc.configured == 75 {
			below = 70
		}
		err := at.validateOpenDecision(open(below), 100, "long")
		if err == nil || !strings.Contains(err.Error(), "below minimum") {
			t.Fatalf("configured %d: trader must reject confidence %d, got %v", tc.configured, below, err)
		}
		if err := at.validateOpenDecision(open(tc.floor), 100, "long"); err != nil && strings.Contains(err.Error(), "Confidence") {
			t.Fatalf("configured %d: trader must accept confidence %d: %v", tc.configured, tc.floor, err)
		}
	}
}
