package market

import (
	"fmt"
	"github.com/Aixxww/AiT/datafetch"
)

// BuildDataFromSymbolSnapshot constructs market.Data from the shared
// datafetch snapshot without making network calls.
func BuildDataFromSymbolSnapshot(symbol string, ss *datafetch.SymbolSnapshot, timeframes []string, primaryTimeframe string, count int) (*Data, error) {
	symbol = Normalize(symbol)
	if ss == nil {
		return nil, fmt.Errorf("snapshot is nil for %s", symbol)
	}
	if len(ss.Klines) == 0 {
		return nil, fmt.Errorf("snapshot klines are empty for %s", symbol)
	}
	if primaryTimeframe == "" && len(timeframes) > 0 {
		primaryTimeframe = timeframes[0]
	}
	if primaryTimeframe == "" {
		for tf := range ss.Klines {
			primaryTimeframe = tf
			break
		}
	}
	timeframes = ensurePrimaryTimeframe(timeframes, primaryTimeframe)
	if count <= 0 {
		count = 30
	}

	timeframeData := make(map[string]*TimeframeSeriesData)
	var primaryKlines []Kline
	for _, tf := range timeframes {
		raw, ok := ss.Klines[tf]
		if !ok || len(raw) == 0 {
			continue
		}
		klines := convertSnapshotKlines(raw)
		if tf == primaryTimeframe {
			primaryKlines = klines
		}
		timeframeData[tf] = calculateTimeframeSeries(klines, tf, count)
	}

	if len(primaryKlines) == 0 {
		return nil, fmt.Errorf("primary timeframe %s klines not available in snapshot for %s", primaryTimeframe, symbol)
	}
	if isStaleData(primaryKlines, symbol) {
		return nil, fmt.Errorf("snapshot data for %s is stale", symbol)
	}

	currentPrice := primaryKlines[len(primaryKlines)-1].Close
	if ss.Price > 0 {
		currentPrice = ss.Price
	}
	// datafetch.OI comes from Binance openInterest and matches market.OIData's
	// existing contract-quantity semantics. Existing liquidity filters multiply
	// OI by price to estimate notional value.
	oiLatest := ss.OI

	return &Data{
		Symbol:        symbol,
		CurrentPrice:  currentPrice,
		PriceChange1h: calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 60),
		PriceChange4h: calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 240),
		CurrentEMA20:  calculateEMA(primaryKlines, 20),
		CurrentMACD:   calculateMACD(primaryKlines),
		CurrentRSI7:   calculateRSI(primaryKlines, 7),
		OpenInterest:  &OIData{Latest: oiLatest, Average: oiLatest},
		FundingRate:   ss.FundingRate,
		TimeframeData: timeframeData,
	}, nil
}

func ensurePrimaryTimeframe(timeframes []string, primary string) []string {
	seen := make(map[string]bool, len(timeframes)+1)
	result := make([]string, 0, len(timeframes)+1)
	if primary != "" {
		result = append(result, primary)
		seen[primary] = true
	}
	for _, tf := range timeframes {
		if tf == "" || seen[tf] {
			continue
		}
		result = append(result, tf)
		seen[tf] = true
	}
	return result
}

func convertSnapshotKlines(raw []datafetch.Kline) []Kline {
	klines := make([]Kline, 0, len(raw))
	for _, k := range raw {
		klines = append(klines, Kline{
			OpenTime:           k.OpenTime,
			Open:               k.Open,
			High:               k.High,
			Low:                k.Low,
			Close:              k.Close,
			Volume:             k.Volume,
			CloseTime:          k.CloseTime,
			TakerBuyBaseVolume: k.TakerBuy,
		})
	}
	return klines
}
