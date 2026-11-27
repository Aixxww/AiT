package kernel

import (
	"math"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

func fx156Data(t *testing.T, periods ...market.IndicatorPeriods) *market.Data {
	t.Helper()
	raw := make([]datafetch.Kline, 120)
	for i := range raw {
		c := 100 + 5*math.Sin(float64(i)/4) + float64(i)*0.1
		raw[i] = datafetch.Kline{OpenTime: int64(i) * 900_000, CloseTime: int64(i)*900_000 + 899_999, Open: c - 0.3, High: c + 1, Low: c - 1, Close: c, Volume: 1000 + float64(i)}
	}
	var opts []market.DataOption
	if len(periods) > 0 {
		opts = append(opts, market.WithIndicatorPeriods(periods[0]))
	}
	d, err := market.BuildDataFromSymbolSnapshot("PERUSDT", &datafetch.SymbolSnapshot{Symbol: "PERUSDT", Price: 100,
		Klines: map[string][]datafetch.Kline{"15m": raw}}, []string{"15m"}, "15m", 40, opts...)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return d
}

func fx156Engine(ind func(*store.IndicatorConfig)) *StrategyEngine {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableEMA, cfg.Indicators.EnableRSI, cfg.Indicators.EnableATR, cfg.Indicators.EnableBOLL = true, true, true, true
	if ind != nil {
		ind(&cfg.Indicators)
	}
	return NewStrategyEngine(&cfg)
}

// With the default periods the timeframe block is byte-identical to the
// pre-#156 output (the fixed-field fallback the printers keep).
func TestIndicatorPeriods156DefaultPromptUnchanged(t *testing.T) {
	e := fx156Engine(nil)
	d := fx156Data(t, e.IndicatorPeriods())
	tf := d.TimeframeData["15m"]
	var withMaps, fixed strings.Builder
	e.formatTimeframeSeriesData(&withMaps, tf, e.config.Indicators)
	legacy := *tf
	legacy.EMASeries, legacy.RSISeries, legacy.ATRValues, legacy.BOLLSeries = nil, nil, nil, nil
	e.formatTimeframeSeriesData(&fixed, &legacy, e.config.Indicators)
	if withMaps.String() != fixed.String() {
		t.Fatalf("default periods changed the timeframe block:\n--- configured\n%s\n--- fixed\n%s", withMaps.String(), fixed.String())
	}
	for _, want := range []string{"EMA20:", "EMA50:", "RSI7:", "RSI14:", "ATR14:", "BOLL Upper:"} {
		if !strings.Contains(withMaps.String(), want) {
			t.Errorf("default block lacks %q", want)
		}
	}
}

// A non-default configuration changes both the series and the prompt, and
// the system prompt's indicator list names exactly the computed periods.
func TestIndicatorPeriods156ConfiguredPromptMatchesSeries(t *testing.T) {
	e := fx156Engine(func(ind *store.IndicatorConfig) {
		ind.EMAPeriods, ind.RSIPeriods, ind.ATRPeriods, ind.BOLLPeriods = []int{9, 21}, []int{21}, []int{7}, []int{10}
	})
	d := fx156Data(t, e.IndicatorPeriods())
	var sb strings.Builder
	e.formatTimeframeSeriesData(&sb, d.TimeframeData["15m"], e.config.Indicators)
	block := sb.String()
	for _, want := range []string{"EMA9:", "EMA21:", "RSI21:", "ATR7:", "BOLL Upper:"} {
		if !strings.Contains(block, want) {
			t.Errorf("configured block lacks %q:\n%s", want, block)
		}
	}
	for _, gone := range []string{"EMA20:", "EMA50:", "RSI7:", "RSI14:", "ATR14:"} {
		if strings.Contains(block, gone) {
			t.Errorf("configured block still prints %q", gone)
		}
	}
	compact := e.formatCompactMarketData(d, nil)
	if !strings.Contains(compact, "ema9=") || !strings.Contains(compact, "rsi21=") || strings.Contains(compact, "rsi7=") {
		t.Errorf("compact line does not follow the configured periods: %s", compact)
	}
	sys := e.BuildSystemPrompt(1000, "")
	for _, want := range []string{"EMA indicators (periods: [9 21])", "RSI indicators (periods: [21])", "ATR indicators (periods: [7])", "(periods: [10])"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt indicator list lacks %q", want)
		}
	}
}
