package kernel

import (
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// IndicatorPeriodsFor maps a strategy's indicator settings to the effective
// market periods (#156): empty lists fall back to market's defaults.
func IndicatorPeriodsFor(ind store.IndicatorConfig) market.IndicatorPeriods {
	return market.IndicatorPeriods{
		EMA:  ind.EMAPeriods,
		RSI:  ind.RSIPeriods,
		ATR:  ind.ATRPeriods,
		BOLL: ind.BOLLPeriods,
	}.Effective()
}

// IndicatorPeriods returns the engine strategy's effective indicator periods.
func (e *StrategyEngine) IndicatorPeriods() market.IndicatorPeriods {
	if e == nil || e.config == nil {
		return market.DefaultIndicatorPeriods()
	}
	return IndicatorPeriodsFor(e.config.Indicators)
}

// IndicatorPeriodsOption is the market build option carrying them (nil-safe).
func (e *StrategyEngine) IndicatorPeriodsOption() market.DataOption {
	return market.WithIndicatorPeriods(e.IndicatorPeriods())
}
