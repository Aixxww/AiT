package kernel

import (
	"math"
	"time"

	"github.com/Aixxww/AiT/market"
	local "github.com/Aixxww/AiT/provider/local"
)

// #111 S5 (review item D): opt-in candle anatomy for the legacy payload.
//
// With v7_prompt.anatomy_bars = N > 0, each expanded legacy candidate's
// execution_context.timeframes[tf] (the timeframes the context already
// carries, 5m and 15m) gains the shape of the last N CLOSED bars at the
// prompt time, oldest first. A bar is closed when open time + interval <=
// prompt time; the forming bar is never included, whatever
// v7_data_timing.feature_bars says (owner decision Q1: features from closed
// candles only). The values come from the prompt-time market data, so the
// sequence ends at the latest closed bar. The prompt payload gets a copy of
// the execution context; the candidate itself is not modified.

// hunterV7AnatomyATRPeriod is the ATR period of range_atr.
const hunterV7AnatomyATRPeriod = 14

// hunterV7ClosedKlines returns the bars of a series closed at nowMs.
func hunterV7ClosedKlines(series *market.TimeframeSeriesData, tf string, nowMs int64) []market.KlineBar {
	iv := hunterV7V2TFInterval(tf).Milliseconds()
	if series == nil || iv <= 0 {
		return nil
	}
	out := make([]market.KlineBar, 0, len(series.Klines))
	for _, k := range series.Klines {
		if k.Time+iv <= nowMs {
			out = append(out, k)
		}
	}
	return out
}

// hunterV7ClosedATR is the mean true range of the last period closed bars
// (0 without period+1 bars).
func hunterV7ClosedATR(bars []market.KlineBar, period int) float64 {
	if period <= 0 || len(bars) < period+1 {
		return 0
	}
	sum := 0.0
	for i := len(bars) - period; i < len(bars); i++ {
		prevClose := bars[i-1].Close
		tr := math.Max(bars[i].High-bars[i].Low, math.Max(math.Abs(bars[i].High-prevClose), math.Abs(bars[i].Low-prevClose)))
		sum += tr
	}
	return sum / float64(period)
}

func hunterV7Round(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(v*p) / p
}

// hunterV7CandleAnatomy is the shape of the last n closed bars, oldest first.
func hunterV7CandleAnatomy(closed []market.KlineBar, n int) []local.V7CandleAnatomy {
	if n <= 0 || len(closed) == 0 {
		return nil
	}
	atr := hunterV7ClosedATR(closed, hunterV7AnatomyATRPeriod)
	start := len(closed) - n
	if start < 0 {
		start = 0
	}
	out := make([]local.V7CandleAnatomy, 0, len(closed)-start)
	for i := start; i < len(closed); i++ {
		b := closed[i]
		a := local.V7CandleAnatomy{BarsAgo: len(closed) - 1 - i, Dir: "flat"}
		switch {
		case b.Close > b.Open:
			a.Dir = "bull"
		case b.Close < b.Open:
			a.Dir = "bear"
		}
		if rng := b.High - b.Low; rng > 0 {
			a.BodyPct = hunterV7Round(math.Abs(b.Close-b.Open)/rng*100, 0)
			a.UpperWickPct = hunterV7Round((b.High-math.Max(b.Open, b.Close))/rng*100, 0)
			a.LowerWickPct = hunterV7Round((math.Min(b.Open, b.Close)-b.Low)/rng*100, 0)
			a.ClosePosPct = hunterV7Round((b.Close-b.Low)/rng*100, 0)
			if atr > 0 {
				a.RangeATR = hunterV7Round(rng/atr, 2)
			}
		}
		out = append(out, a)
	}
	return out
}

// hunterV7ExecutionContextWithAnatomy returns a copy of ctx whose
// timeframes carry the anatomy of the last n closed bars, or ctx itself when
// there is nothing to add.
func hunterV7ExecutionContextWithAnatomy(ctx *local.V7ExecutionContext, data *market.Data, n int, promptTime time.Time) *local.V7ExecutionContext {
	if ctx == nil || data == nil || n <= 0 || len(ctx.Timeframes) == 0 {
		return ctx
	}
	out := &local.V7ExecutionContext{DataQuality: ctx.DataQuality, Timeframes: make(map[string]local.V7ExecutionTimeframeSummary, len(ctx.Timeframes))}
	nowMs := promptTime.UnixMilli()
	added := false
	for tf, summary := range ctx.Timeframes {
		if anatomy := hunterV7CandleAnatomy(hunterV7ClosedKlines(data.TimeframeData[tf], tf, nowMs), n); len(anatomy) > 0 {
			summary.Anatomy = anatomy
			added = true
		}
		out.Timeframes[tf] = summary
	}
	if !added {
		return ctx
	}
	return out
}
