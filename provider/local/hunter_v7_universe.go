package local

import (
	"math"
	"sort"

	"github.com/Aixxww/AiT/datafetch"
)

// ============================================================================
// Hunter v7 — Universe Builder
// ============================================================================
// Constructs the multi-source candidate pool from a datafetch.Snapshot.
// Instead of the v6 approach (top 50 by volume), v7 merges multiple
// ranking dimensions to capture ~200-350 unique symbols.

// coreLiquiditySymbols are always included in the universe regardless of ranking.
var coreLiquiditySymbols = map[string]bool{
	"BTCUSDT":  true,
	"ETHUSDT":  true,
	"SOLUSDT":  true,
	"BNBUSDT":  true,
	"XRPUSDT":  true,
	"DOGEUSDT": true,
	"ADAUSDT":  true,
	"AVAXUSDT": true,
	"LINKUSDT": true,
	"DOTUSDT":  true,
}

// V7UniverseEntry holds a single symbol's universe data plus derived context.
type V7UniverseEntry struct {
	Ctx V7SymbolContext
	// Ranking dimensions used for dedup priority
	VolRank  int // Quote volume rank
	GainRank int // 24h gain rank
	LossRank int // 24h loss rank
	OIRank   int // |OI change| rank
	FundRank int // |funding| rank
	BestRank int // Best rank across all dimensions
}

// BuildV7Universe constructs the multi-source candidate pool from a Snapshot.
// Returns a slice of V7SymbolContext ready for signal module processing.
func BuildV7Universe(snap *datafetch.Snapshot) []V7SymbolContext {
	if snap == nil || len(snap.Symbols) == 0 {
		return nil
	}

	// Step 1: Filter to valid USDT perps
	type rawEntry struct {
		sym string
		ss  *datafetch.SymbolSnapshot
	}
	var raw []rawEntry
	for sym, ss := range snap.Symbols {
		if !isUSDTPerp(sym) {
			continue
		}
		if excludedTokenizedAssets[sym] {
			continue
		}
		if ss.QuoteVolume24h <= 0 {
			continue
		}
		// Ordinary routed setups need derivatives detail, but high-amplitude
		// symbols and fast new-activity movers must stay visible for mover
		// attribution even before OI detail has been fetched by the detail selector.
		if ss.OI <= 0 &&
			symbolAmplitude24h(ss) < 12 &&
			symbolVelocityScore(ss) < 2.0 &&
			symbolNewActivityScore(ss) < 3.0 {
			continue
		}
		raw = append(raw, rawEntry{sym: sym, ss: ss})
	}

	if len(raw) == 0 {
		return nil
	}

	// Step 2: Rank by multiple dimensions
	type ranked struct {
		sym   string
		ss    *datafetch.SymbolSnapshot
		vol   float64 // quote volume
		gain  float64 // 24h change
		oiChg float64 // |OI change 1h|
		fund  float64 // |funding rate|
		vel   float64 // short-term price velocity
		act   float64 // short-term activity burst
	}

	var entries []ranked
	for _, r := range raw {
		entries = append(entries, ranked{
			sym:   r.sym,
			ss:    r.ss,
			vol:   r.ss.QuoteVolume24h,
			gain:  r.ss.PriceChange24h,
			oiChg: math.Abs(r.ss.OIDelta1h),
			fund:  math.Abs(r.ss.FundingRate),
			vel:   symbolVelocityScore(r.ss),
			act:   symbolNewActivityScore(r.ss),
		})
	}

	// Collect unique symbols from each top-N list
	selected := make(map[string]*datafetch.SymbolSnapshot)
	priorities := make(map[string]int) // best rank across dimensions

	addFromRanking := func(data []ranked, field func(r ranked) float64, topN int, dimName string) {
		sorted := make([]ranked, len(data))
		copy(sorted, data)
		sort.Slice(sorted, func(i, j int) bool {
			return field(sorted[i]) > field(sorted[j])
		})
		for i, r := range sorted {
			if i >= topN {
				break
			}
			if _, exists := selected[r.sym]; !exists {
				selected[r.sym] = r.ss
				priorities[r.sym] = i + 1
			} else if i+1 < priorities[r.sym] {
				priorities[r.sym] = i + 1
			}
		}
	}

	addFromRanking(entries, func(r ranked) float64 { return r.vol }, 150, "volume")
	addFromRanking(entries, func(r ranked) float64 { return r.gain }, 50, "gainers")
	// Losers: sort by most negative change
	losers := make([]ranked, len(entries))
	copy(losers, entries)
	sort.Slice(losers, func(i, j int) bool {
		return losers[i].gain < losers[j].gain
	})
	for i, r := range losers {
		if i >= 50 {
			break
		}
		if _, exists := selected[r.sym]; !exists {
			selected[r.sym] = r.ss
			priorities[r.sym] = i + 1
		} else if i+1 < priorities[r.sym] {
			priorities[r.sym] = i + 1
		}
	}
	addFromRanking(entries, func(r ranked) float64 { return r.oiChg }, 50, "oi_change")
	addFromRanking(entries, func(r ranked) float64 { return r.fund }, 50, "funding")
	addFromRanking(entries, func(r ranked) float64 { return r.vel }, 80, "velocity")
	addFromRanking(entries, func(r ranked) float64 { return r.act }, 80, "new_activity")

	// Amplitude pool: symbols with 24h amplitude >= 12%
	for _, r := range entries {
		if r.ss.LowPrice24h <= 0 {
			continue
		}
		amp := (r.ss.HighPrice24h - r.ss.LowPrice24h) / r.ss.LowPrice24h * 100
		if amp >= 12 {
			if _, exists := selected[r.sym]; !exists {
				selected[r.sym] = r.ss
				priorities[r.sym] = 200 // amplitude pool priority
			}
		}
	}

	// Range expansion pool: symbols with large 1h true range relative to recent median
	for _, r := range entries {
		if r.ss.Klines == nil {
			continue
		}
		bars1h, ok := r.ss.Klines["1h"]
		if !ok || len(bars1h) < 5 {
			continue
		}
		kb := datafetchKlinesToKlineBar(bars1h)
		latestTR := trueRange(kb[len(kb)-1], kb[len(kb)-2])
		lookback := 20
		if len(kb)-1 < lookback {
			lookback = len(kb) - 1
		}
		if lookback < 3 {
			continue
		}
		var trs []float64
		for i := len(kb) - lookback; i < len(kb)-1; i++ {
			trs = append(trs, trueRange(kb[i], kb[i-1]))
		}
		medianTR := medianFloat64(trs)
		if medianTR > 0 && latestTR/medianTR >= 2.2 {
			if _, exists := selected[r.sym]; !exists {
				selected[r.sym] = r.ss
				priorities[r.sym] = 200
			}
		}
	}

	// Velocity pool: symbols with fresh 5m/15m displacement before they become
	// top-volume names. This improves recall for cold-start movers.
	for _, r := range entries {
		if r.vel < 2.0 {
			continue
		}
		if _, exists := selected[r.sym]; !exists {
			selected[r.sym] = r.ss
			priorities[r.sym] = 210
		}
	}

	// New-activity pool: recently quiet symbols with sudden short-term volume.
	for _, r := range entries {
		if r.act < 3.0 {
			continue
		}
		if _, exists := selected[r.sym]; !exists {
			selected[r.sym] = r.ss
			priorities[r.sym] = 220
		}
	}

	// Always include core liquidity symbols
	for sym := range coreLiquiditySymbols {
		if ss, ok := snap.Symbols[sym]; ok && ss.QuoteVolume24h > 0 {
			if _, exists := selected[sym]; !exists {
				selected[sym] = ss
				priorities[sym] = 999 // lowest priority (just inclusion)
			}
		}
	}

	// Cap at 350
	if len(selected) > 350 {
		// Keep the 350 with best priority
		type prioEntry struct {
			sym  string
			prio int
		}
		var sorted []prioEntry
		for sym, prio := range priorities {
			sorted = append(sorted, prioEntry{sym: sym, prio: prio})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].prio < sorted[j].prio
		})
		selected = make(map[string]*datafetch.SymbolSnapshot)
		for i, pe := range sorted {
			if i >= 350 {
				break
			}
			selected[pe.sym] = snap.Symbols[pe.sym]
		}
	}

	// Step 3: Build SymbolContext for each selected symbol
	var universe []V7SymbolContext
	for sym, ss := range selected {
		ctx := buildSymbolContext(sym, ss, snap)
		if ctx != nil {
			universe = append(universe, *ctx)
		}
	}

	return universe
}

// buildSymbolContext derives all technical indicators for a single symbol.
func buildSymbolContext(sym string, ss *datafetch.SymbolSnapshot, snap *datafetch.Snapshot) *V7SymbolContext {
	ctx := &V7SymbolContext{
		Symbol:       sym,
		CurrentPrice: ss.Price,
		Change24h:    ss.PriceChange24h,
	}

	// Build lightweight snapshot data
	ctx.Snapshot = &SymbolSnapshotData{
		Price:          ss.Price,
		PriceChange24h: ss.PriceChange24h,
		Volume24h:      ss.Volume24h,
		QuoteVolume24h: ss.QuoteVolume24h,
		HighPrice24h:   ss.HighPrice24h,
		LowPrice24h:    ss.LowPrice24h,
		TradeCount24h:  ss.TradeCount24h,
		FundingRate:    ss.FundingRate,
		OI:             symbolOINotional(ss),
		OIDelta1h:      ss.OIDelta1h,
		OIDelta4h:      ss.OIDelta4h,
		LSR:            ss.LongShortRatio,
		LSRPrev:        ss.LSRPrev,
		LSROldest:      ss.LSROldest,
		TakerBuy:       ss.TakerBuyRatio,
	}

	// Convert datafetch klines to klineBar for existing utility functions
	klines := ss.Klines
	if klines == nil {
		return ctx
	}

	// 4h klines
	if bars4h, ok := klines["4h"]; ok && len(bars4h) >= 15 {
		kb4h := datafetchKlinesToKlineBar(bars4h)
		ctx.ATR4h = computeATR(kb4h, 14)
		if len(kb4h) > 0 {
			ctx.CurrentPrice = kb4h[len(kb4h)-1].Close
		}
		high, low := findHighLow(kb4h)
		ctx.High4h = high
		ctx.Low4h = low
		// EMA approximation
		ctx.EMA20_4h = computeEMAFromKlines(kb4h, 20)
		ctx.EMA60_4h = computeEMAFromKlines(kb4h, 60)
	}

	// 1d klines
	if bars1d, ok := klines["1d"]; ok && len(bars1d) >= 15 {
		kb1d := datafetchKlinesToKlineBar(bars1d)
		ctx.ATR1d = computeATR(kb1d, 14)
		high, low := findHighLow(kb1d)
		ctx.High1d = high
		ctx.Low1d = low
	}

	// 1h klines
	if bars1h, ok := klines["1h"]; ok && len(bars1h) >= 15 {
		kb1h := datafetchKlinesToKlineBar(bars1h)
		ctx.ATR1h = computeATR(kb1h, 14)
		high, low := findHighLow(kb1h)
		ctx.High1h = high
		ctx.Low1h = low
		ctx.EMA20_1h = computeEMAFromKlines(kb1h, 20)
		ctx.EMA60_1h = computeEMAFromKlines(kb1h, 60)
		ctx.RSI1h = computeRSIFromKlines(kb1h, 14)
		ctx.ADX1h = computeADXFromKlines(kb1h, 14)
		// Latest taker buy ratio from 1h
		if last := kb1h[len(kb1h)-1]; last.Volume > 0 {
			ctx.TakerBuy1h = last.TakerBuyBaseVolume / last.Volume
		}
	}

	// 15m klines
	if bars15m, ok := klines["15m"]; ok && len(bars15m) >= 20 {
		kb15m := datafetchKlinesToKlineBar(bars15m)
		ctx.ATR15m = computeATR(kb15m, 14)
		// BB Width
		width, upper, middle, lower := computeBBWidth(kb15m, 20, 2.0)
		ctx.BBWidth15m = width
		ctx.BBUpper15m = upper
		ctx.BBMiddle15m = middle
		ctx.BBLower15m = lower
		// BB percentile
		ctx.BBWidthPercentile = computeBBWidthPercentile(kb15m)
		// Latest taker buy ratio
		if last := kb15m[len(kb15m)-1]; last.Volume > 0 {
			ctx.TakerBuy15m = last.TakerBuyBaseVolume / last.Volume
		}
		// VWAP approximation (last 20 bars)
		ctx.VWAP15m = computeVWAP(kb15m)
	}

	// 5m klines (for BB width fallback)
	if bars5m, ok := klines["5m"]; ok && len(bars5m) >= 20 {
		kb5m := datafetchKlinesToKlineBar(bars5m)
		ctx.ATR5m = computeATR(kb5m, 14)
		w5, _, _, _ := computeBBWidth(kb5m, 20, 2.0)
		ctx.BBWidth5m = w5
	}

	// Compute 1h/4h change
	if bars1h, ok := klines["1h"]; ok && len(bars1h) >= 2 {
		kb1h := datafetchKlinesToKlineBar(bars1h)
		first := kb1h[len(kb1h)-2].Close
		last := kb1h[len(kb1h)-1].Close
		if first > 0 {
			ctx.Change1h = (last - first) / first * 100
		}
	}
	if bars4h, ok := klines["4h"]; ok && len(bars4h) >= 2 {
		kb4h := datafetchKlinesToKlineBar(bars4h)
		first := kb4h[len(kb4h)-2].Close
		last := kb4h[len(kb4h)-1].Close
		if first > 0 {
			ctx.Change4h = (last - first) / first * 100
		}
	}

	// Amplitude 24h: (High - Low) / Low * 100
	ctx.Amplitude24h = symbolAmplitude24h(ss)
	ctx.Velocity5m = symbolKlineVelocityPct(ss, "5m")
	ctx.Velocity15m = symbolKlineVelocityPct(ss, "15m")
	ctx.VolumeBurst5m = symbolKlineVolumeBurst(ss, "5m", 12)
	ctx.VolumeBurst15m = symbolKlineVolumeBurst(ss, "15m", 8)

	// Range expansion: latest 1h true range / median of last 20 1h true ranges
	if bars1h, ok := klines["1h"]; ok && len(bars1h) >= 5 {
		kb1h := datafetchKlinesToKlineBar(bars1h)
		latestTR := trueRange(kb1h[len(kb1h)-1], kb1h[len(kb1h)-2])
		lookback := 20
		if len(kb1h)-1 < lookback {
			lookback = len(kb1h) - 1
		}
		if lookback >= 3 {
			var trs []float64
			for i := len(kb1h) - lookback; i < len(kb1h)-1; i++ {
				trs = append(trs, trueRange(kb1h[i], kb1h[i-1]))
			}
			medianTR := medianFloat64(trs)
			if medianTR > 0 {
				ctx.RangeExpansion1h = latestTR / medianTR
			}
		}
	}

	// Classify pool after all derived pool metrics are available.
	ctx.PoolType = classifyPool(ctx)

	return ctx
}

func symbolVelocityScore(ss *datafetch.SymbolSnapshot) float64 {
	v5 := math.Abs(symbolKlineVelocityPct(ss, "5m"))
	v15 := math.Abs(symbolKlineVelocityPct(ss, "15m"))
	if v15 > v5 {
		return v15
	}
	return v5
}

func symbolNewActivityScore(ss *datafetch.SymbolSnapshot) float64 {
	b5 := symbolKlineVolumeBurst(ss, "5m", 12)
	b15 := symbolKlineVolumeBurst(ss, "15m", 8)
	if b15 > b5 {
		return b15
	}
	return b5
}

func symbolKlineVelocityPct(ss *datafetch.SymbolSnapshot, interval string) float64 {
	if ss == nil || ss.Klines == nil {
		return 0
	}
	bars := ss.Klines[interval]
	if len(bars) < 2 {
		return 0
	}
	prev := bars[len(bars)-2].Close
	last := bars[len(bars)-1].Close
	if prev <= 0 {
		return 0
	}
	return (last - prev) / prev * 100
}

func symbolKlineVolumeBurst(ss *datafetch.SymbolSnapshot, interval string, lookback int) float64 {
	if ss == nil || ss.Klines == nil || lookback <= 0 {
		return 0
	}
	bars := ss.Klines[interval]
	if len(bars) < 3 {
		return 0
	}
	last := bars[len(bars)-1].Volume
	start := len(bars) - 1 - lookback
	if start < 0 {
		start = 0
	}
	if start >= len(bars)-1 {
		return 0
	}
	sum := 0.0
	count := 0
	for i := start; i < len(bars)-1; i++ {
		sum += bars[i].Volume
		count++
	}
	if count == 0 {
		return 0
	}
	avg := sum / float64(count)
	if avg <= 0 {
		return 0
	}
	return last / avg
}

func symbolAmplitude24h(ss *datafetch.SymbolSnapshot) float64 {
	if ss == nil || ss.LowPrice24h <= 0 || ss.HighPrice24h <= 0 {
		return 0
	}
	return (ss.HighPrice24h - ss.LowPrice24h) / ss.LowPrice24h * 100
}

func symbolOINotional(ss *datafetch.SymbolSnapshot) float64 {
	if ss == nil || ss.OI <= 0 {
		return 0
	}
	price := ss.Price
	if price <= 0 {
		price = ss.MarkPrice
	}
	if price <= 0 {
		return 0
	}
	return ss.OI * price
}

// classifyPool assigns a symbol to its primary candidate pool.
func classifyPool(ctx *V7SymbolContext) V7PoolType {
	sym := ctx.Symbol
	// Core liquidity: major pairs
	if coreLiquiditySymbols[sym] {
		return V7PoolCoreLiquidity
	}
	// Panic: significant drop
	if ctx.Change24h < -15 {
		return V7PoolPanic
	}
	// Hot alt: significant gain
	if ctx.Change24h > 12 {
		return V7PoolHotAlt
	}
	// Velocity: fresh short-term price displacement.
	if math.Abs(ctx.Velocity5m) >= 2.0 || math.Abs(ctx.Velocity15m) >= 2.0 {
		return V7PoolVelocity
	}
	// New activity: recent volume is several times its short-term baseline.
	if ctx.VolumeBurst5m >= 3.0 || ctx.VolumeBurst15m >= 3.0 {
		return V7PoolNewActivity
	}
	// Squeeze: OI anomaly
	if ctx.Snapshot != nil && math.Abs(ctx.Snapshot.OIDelta1h) > 10 {
		return V7PoolSqueeze
	}
	// Funding extreme
	if ctx.Snapshot != nil && math.Abs(ctx.Snapshot.FundingRate) > 0.0005 {
		return V7PoolFunding
	}
	// Default
	return V7PoolHotAlt
}

// datafetchKlinesToKlineBar converts datafetch.Kline slice to local klineBar slice.
func datafetchKlinesToKlineBar(dk []datafetch.Kline) []klineBar {
	bars := make([]klineBar, len(dk))
	for i, k := range dk {
		bars[i] = klineBar{
			OpenTime:           k.OpenTime,
			Open:               k.Open,
			High:               k.High,
			Low:                k.Low,
			Close:              k.Close,
			Volume:             k.Volume,
			TakerBuyBaseVolume: k.TakerBuy,
		}
	}
	return bars
}

// computeEMAFromKlines computes an EMA over kline bars.
func computeEMAFromKlines(klines []klineBar, period int) float64 {
	if len(klines) < period {
		return 0
	}
	multiplier := 2.0 / float64(period+1)
	// Start with SMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}
	return ema
}

// computeRSIFromKlines computes RSI using Wilder's smoothing.
func computeRSIFromKlines(klines []klineBar, period int) float64 {
	if len(klines) < period+1 {
		return 50 // neutral
	}
	var gains, losses float64
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}
	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) - change) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

// computeADXFromKlines computes Average Directional Index.
func computeADXFromKlines(klines []klineBar, period int) float64 {
	if len(klines) < period*2 {
		return 0
	}

	type di struct {
		pDM float64
		nDM float64
		tr  float64
	}

	var diData []di
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevHigh := klines[i-1].High
		prevLow := klines[i-1].Low
		prevClose := klines[i-1].Close

		upMove := high - prevHigh
		downMove := prevLow - low

		var pDM, nDM float64
		if upMove > downMove && upMove > 0 {
			pDM = upMove
		}
		if downMove > upMove && downMove > 0 {
			nDM = downMove
		}

		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		diData = append(diData, di{pDM: pDM, nDM: nDM, tr: tr})
	}

	if len(diData) < period {
		return 0
	}

	// Wilder's smoothing
	sumPDM := 0.0
	sumNDM := 0.0
	sumTR := 0.0
	for i := 0; i < period; i++ {
		sumPDM += diData[i].pDM
		sumNDM += diData[i].nDM
		sumTR += diData[i].tr
	}
	smoothPDM := sumPDM
	smoothNDM := sumNDM
	smoothTR := sumTR

	var dxSum float64
	dxCount := 0

	for i := period; i < len(diData); i++ {
		smoothPDM = smoothPDM - smoothPDM/float64(period) + diData[i].pDM
		smoothNDM = smoothNDM - smoothNDM/float64(period) + diData[i].nDM
		smoothTR = smoothTR - smoothTR/float64(period) + diData[i].tr

		if smoothTR > 0 {
			pDI := smoothPDM / smoothTR * 100
			nDI := smoothNDM / smoothTR * 100
			diSum := pDI + nDI
			if diSum > 0 {
				dx := math.Abs(pDI-nDI) / diSum * 100
				dxSum += dx
				dxCount++
			}
		}
	}

	if dxCount == 0 {
		return 0
	}
	return dxSum / float64(dxCount)
}

// computeBBWidthPercentile computes the current BB width as a percentile
// of the rolling range. Lower = more compressed (better for breakout detection).
func computeBBWidthPercentile(klines []klineBar) float64 {
	if len(klines) < 25 {
		return 50
	}
	currentWidth, _, _, _ := computeBBWidth(klines, 20, 2.0)
	if currentWidth == 0 {
		return 50
	}

	// Compute rolling BB widths
	var widths []float64
	for i := 20; i <= len(klines); i++ {
		w, _, _, _ := computeBBWidth(klines[:i], 20, 2.0)
		if w > 0 {
			widths = append(widths, w)
		}
	}
	if len(widths) == 0 {
		return 50
	}

	// Percentile rank in ascending width order: lower means the current band
	// width is tighter than most recent widths, i.e. stronger compression.
	count := 0
	for _, w := range widths {
		if w <= currentWidth {
			count++
		}
	}
	return float64(count) / float64(len(widths)) * 100
}

// computeVWAP computes a simple Volume-Weighted Average Price over kline bars.
func computeVWAP(klines []klineBar) float64 {
	if len(klines) == 0 {
		return 0
	}
	var totalPV, totalV float64
	for _, k := range klines {
		typical := (k.High + k.Low + k.Close) / 3
		totalPV += typical * k.Volume
		totalV += k.Volume
	}
	if totalV == 0 {
		return 0
	}
	return totalPV / totalV
}

// getQuoteVolume24h safely extracts 24h quote volume from a ticker string.
func getQuoteVolume24h(qv string) float64 {
	return parseFloat(qv)
}

// trueRange computes the true range of a bar relative to the previous bar's close.
func trueRange(bar, prev klineBar) float64 {
	hl := bar.High - bar.Low
	hc := math.Abs(bar.High - prev.Close)
	lc := math.Abs(bar.Low - prev.Close)
	return math.Max(hl, math.Max(hc, lc))
}

// medianFloat64 returns the median of a float64 slice (copies and sorts).
func medianFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}
