package kernel

import (
	"fmt"

	"github.com/Aixxww/AiT/market"
)

// Prompt views of the configured indicator series (#156). Each helper reads
// the per-period series the market build computed from the strategy's
// periods; data built without them (hand-made fixtures, the legacy
// single-timeframe builders) falls back to the fixed fields, which is the
// pre-#156 output.

type promptSeries struct {
	period int
	values []float64
}

type promptBands struct {
	label string // "" for a single BOLL period (the indicator list names it)
	bands market.BOLLBands
}

func promptEMASeries(d *market.TimeframeSeriesData) []promptSeries {
	if d.EMASeries == nil {
		return []promptSeries{{20, d.EMA20Values}, {50, d.EMA50Values}}
	}
	out := make([]promptSeries, 0, len(d.Periods.EMA))
	for _, p := range d.Periods.EMA {
		out = append(out, promptSeries{p, d.EMASeries[p]})
	}
	return out
}

func promptRSISeries(d *market.TimeframeSeriesData) []promptSeries {
	if d.RSISeries == nil {
		return []promptSeries{{7, d.RSI7Values}, {14, d.RSI14Values}}
	}
	out := make([]promptSeries, 0, len(d.Periods.RSI))
	for _, p := range d.Periods.RSI {
		out = append(out, promptSeries{p, d.RSISeries[p]})
	}
	return out
}

type promptValue struct {
	period int
	value  float64
}

func promptATRValues(d *market.TimeframeSeriesData) []promptValue {
	if d.ATRValues == nil {
		return []promptValue{{14, d.ATR14}}
	}
	out := make([]promptValue, 0, len(d.Periods.ATR))
	for _, p := range d.Periods.ATR {
		out = append(out, promptValue{p, d.ATRValues[p]})
	}
	return out
}

func promptBOLLBands(d *market.TimeframeSeriesData) []promptBands {
	if d.BOLLSeries == nil {
		return []promptBands{{"", market.BOLLBands{Upper: d.BOLLUpper, Middle: d.BOLLMiddle, Lower: d.BOLLLower}}}
	}
	out := make([]promptBands, 0, len(d.Periods.BOLL))
	for _, p := range d.Periods.BOLL {
		label := ""
		if len(d.Periods.BOLL) > 1 {
			label = fmt.Sprintf("%d", p)
		}
		out = append(out, promptBands{label, d.BOLLSeries[p]})
	}
	return out
}

// promptCurrentEMA / promptCurrentRSI: the primary-timeframe value of the
// first configured period, else the fixed EMA20 / RSI7.
func promptCurrentEMA(d *market.Data) (int, float64) {
	if p := d.IndicatorPeriods.EMA; len(p) > 0 && d.CurrentEMAs != nil {
		return p[0], d.CurrentEMAs[p[0]]
	}
	return 20, d.CurrentEMA20
}

func promptCurrentRSI(d *market.Data) (int, float64) {
	if p := d.IndicatorPeriods.RSI; len(p) > 0 && d.CurrentRSIs != nil {
		return p[0], d.CurrentRSIs[p[0]]
	}
	return 7, d.CurrentRSI7
}
