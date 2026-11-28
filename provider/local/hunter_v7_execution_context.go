package local

import "math"

func buildV7ExecutionContextFromKlines(klines map[string][]klineBar, price float64) *V7ExecutionContext {
	if len(klines) == 0 {
		return nil
	}
	out := &V7ExecutionContext{
		DataQuality: "partial",
		Timeframes:  make(map[string]V7ExecutionTimeframeSummary, 2),
	}
	for _, tf := range []string{"15m", "5m"} {
		summary, ok := buildV7ExecutionTimeframeSummary(tf, klines[tf], price)
		if ok {
			out.Timeframes[tf] = summary
		}
	}
	if len(out.Timeframes) == 0 {
		return nil
	}
	if _, ok15 := out.Timeframes["15m"]; ok15 {
		if _, ok5 := out.Timeframes["5m"]; ok5 {
			out.DataQuality = "complete_for_execution"
		}
	}
	return out
}

func buildV7ExecutionTimeframeSummary(tf string, bars []klineBar, price float64) (V7ExecutionTimeframeSummary, bool) {
	if len(bars) == 0 {
		return V7ExecutionTimeframeSummary{}, false
	}
	last := bars[len(bars)-1]
	refPrice := price
	if refPrice <= 0 {
		refPrice = last.Close
	}
	out := V7ExecutionTimeframeSummary{
		Timeframe:         tf,
		CandleCount:       len(bars),
		LastClose:         last.Close,
		LastCloseTimeUnix: v7BarCloseTimeUnix(last.OpenTime, last.CloseTime, tf),
	}
	if refPrice <= 0 {
		return out, true
	}

	recent := lastV7Klines(bars, 3)
	out.RecentHigh3, out.RecentLow3 = findHighLow(recent)

	if len(bars) >= 20 {
		ema20 := computeEMAFromKlines(bars, 20)
		if ema20 > 0 {
			out.HasEMA20 = true
			out.EMA20 = ema20
			out.CloseVsEMA20Pct = pctDiff(last.Close, ema20)
		}
		vwap20 := computeVWAP(lastV7Klines(bars, 20))
		if vwap20 > 0 {
			out.HasVWAP20 = true
			out.VWAP20 = vwap20
			out.CloseVsVWAP20Pct = pctDiff(last.Close, vwap20)
		}
	}
	if len(bars) >= 15 {
		atr := computeATR(bars, 14)
		if atr > 0 {
			out.HasATR = true
			out.ATRPct = atr / refPrice * 100
			out.MinStop08ATRPct = atr * 0.8 / refPrice * 100
		}
	}
	if len(bars) >= 6 {
		prev := bars[:len(bars)-1]
		prevRecent := lastV7Klines(prev, 5)
		prevHigh, prevLow := findHighLow(prevRecent)
		if prevHigh > 0 {
			out.NoNewHigh = last.High <= prevHigh
		}
		if prevLow > 0 {
			out.NoNewLow = last.Low >= prevLow
		}
		if avgVol := avgV7KlineVolume(prevRecent); avgVol > 0 {
			out.VolumeVsAvg5 = last.Volume / avgVol
		}
	}
	return out, true
}

func lastV7Klines(values []klineBar, n int) []klineBar {
	if n <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) <= n {
		return values
	}
	return values[len(values)-n:]
}

func avgV7KlineVolume(values []klineBar) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, v := range values {
		total += v.Volume
	}
	return total / float64(len(values))
}

func pctDiff(value, base float64) float64 {
	if base == 0 {
		return 0
	}
	return (value - base) / math.Abs(base) * 100
}

// v7TimeframeIntervalMs returns the candle duration in milliseconds for
// standard Hunter v7 timeframes, or 0 if unrecognized.
func v7TimeframeIntervalMs(tf string) int64 {
	switch tf {
	case "1m":
		return 60_000
	case "3m":
		return 180_000
	case "5m":
		return 300_000
	case "15m":
		return 900_000
	case "30m":
		return 1_800_000
	case "1h":
		return 3_600_000
	case "4h":
		return 14_400_000
	case "1d":
		return 86_400_000
	default:
		return 0
	}
}

// v7BarCloseTimeUnix normalizes a bar's close timestamp to Unix seconds so
// datafetch.Kline (CloseTime = open + interval - 1 ms) and market.KlineBar
// (Time = open ms) produce identical close timestamps for the same closed bar.
func v7BarCloseTimeUnix(openMs, closeMs int64, tf string) int64 {
	if closeMs > 0 {
		if closeMs%1000 == 999 {
			return (closeMs + 1) / 1000
		}
		return closeMs / 1000
	}
	if openMs > 0 {
		if ivMs := v7TimeframeIntervalMs(tf); ivMs > 0 {
			return (openMs + ivMs) / 1000
		}
	}
	return 0
}
