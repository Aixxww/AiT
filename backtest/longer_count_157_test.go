package backtest

import (
	"testing"

	"github.com/Aixxww/AiT/store"
)

// #157: the backtest data feed reads the strategy's longer_count.
func TestBacktestLongerCount157(t *testing.T) {
	cfg := BacktestConfig{Symbols: []string{"BTCUSDT"}, Timeframes: []string{"5m", "4h"}}
	if got := backtestLongerCount(cfg); got != 10 {
		t.Fatalf("generated strategy longer_count = %d, want the default 10", got)
	}
	s := store.GetDefaultStrategyConfig("en")
	s.Indicators.Klines.LongerCount = 25
	cfg.SetLoadedStrategy(&s)
	if got := backtestLongerCount(cfg); got != 25 {
		t.Fatalf("loaded strategy longer_count = %d, want 25", got)
	}
}
