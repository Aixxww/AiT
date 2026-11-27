package market

import "sort"

// Indicator periods (#156). The strategy's indicators.ema_periods /
// rsi_periods / atr_periods / boll_periods drive the per-timeframe series
// the prompt prints; the system prompt's indicator list states the same
// effective periods, so the two cannot diverge.

// IndicatorPeriods lists the configured periods per indicator.
type IndicatorPeriods struct {
	EMA  []int
	RSI  []int
	ATR  []int
	BOLL []int
}

// DefaultIndicatorPeriods are the periods the timeframe series used before
// #156 (EMA 20/50, RSI 7/14, ATR 14, BOLL 20), also the store defaults.
func DefaultIndicatorPeriods() IndicatorPeriods {
	return IndicatorPeriods{EMA: []int{20, 50}, RSI: []int{7, 14}, ATR: []int{14}, BOLL: []int{20}}
}

// Effective drops non-positive and duplicate periods (keeping the user's
// order) and falls back to the default list for an indicator left empty.
func (p IndicatorPeriods) Effective() IndicatorPeriods {
	d := DefaultIndicatorPeriods()
	return IndicatorPeriods{
		EMA:  effectivePeriodList(p.EMA, d.EMA),
		RSI:  effectivePeriodList(p.RSI, d.RSI),
		ATR:  effectivePeriodList(p.ATR, d.ATR),
		BOLL: effectivePeriodList(p.BOLL, d.BOLL),
	}
}

func effectivePeriodList(in, def []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(in))
	for _, v := range in {
		if v > 0 && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return append([]int(nil), def...)
	}
	return out
}

// BOLLBands is one Bollinger period's band series.
type BOLLBands struct {
	Upper  []float64 `json:"upper"`
	Middle []float64 `json:"middle"`
	Lower  []float64 `json:"lower"`
}

// bollMultiplier is the Bollinger standard-deviation multiplier (not a
// period; unchanged).
const bollMultiplier = 2.0

// DataOption configures a market-data build.
type DataOption func(*dataOptions)

type dataOptions struct {
	periods IndicatorPeriods
}

// WithIndicatorPeriods sets the indicator periods of a build; empty lists
// fall back to DefaultIndicatorPeriods.
func WithIndicatorPeriods(p IndicatorPeriods) DataOption {
	return func(o *dataOptions) { o.periods = p }
}

func resolveDataOptions(opts []DataOption) dataOptions {
	var o dataOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	o.periods = o.periods.Effective()
	return o
}

// currentIndicatorValues computes the latest EMA / RSI per configured
// period on the primary timeframe.
func currentIndicatorValues(klines []Kline, p IndicatorPeriods) (emas, rsis map[int]float64) {
	emas = make(map[int]float64, len(p.EMA))
	for _, n := range p.EMA {
		emas[n] = calculateEMA(klines, n)
	}
	rsis = make(map[int]float64, len(p.RSI))
	for _, n := range p.RSI {
		rsis[n] = calculateRSI(klines, n)
	}
	return emas, rsis
}

// fillPeriodSeries computes the configured per-period series of one
// timeframe over klines[start:], with the same warm-up rules the fixed
// series use: EMA(n) and BOLL(n) from bar n-1, RSI(n) from bar n.
func fillPeriodSeries(data *TimeframeSeriesData, klines []Kline, start int, p IndicatorPeriods) {
	data.Periods = p
	data.EMASeries = make(map[int][]float64, len(p.EMA))
	data.RSISeries = make(map[int][]float64, len(p.RSI))
	data.ATRValues = make(map[int]float64, len(p.ATR))
	data.BOLLSeries = make(map[int]BOLLBands, len(p.BOLL))
	for i := start; i < len(klines); i++ {
		for _, n := range p.EMA {
			if i >= n-1 {
				data.EMASeries[n] = append(data.EMASeries[n], calculateEMA(klines[:i+1], n))
			}
		}
		for _, n := range p.RSI {
			if i >= n {
				data.RSISeries[n] = append(data.RSISeries[n], calculateRSI(klines[:i+1], n))
			}
		}
		for _, n := range p.BOLL {
			if i >= n-1 {
				u, m, l := calculateBOLL(klines[:i+1], n, bollMultiplier)
				b := data.BOLLSeries[n]
				b.Upper, b.Middle, b.Lower = append(b.Upper, u), append(b.Middle, m), append(b.Lower, l)
				data.BOLLSeries[n] = b
			}
		}
	}
	for _, n := range p.ATR {
		data.ATRValues[n] = calculateATR(klines, n)
	}
}

// SortedPeriods returns the keys of a period map in ascending order (for
// callers without the configured order).
func SortedPeriods[V any](m map[int]V) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
