package local

import (
	"math"
	"nofx/datafetch"
	"nofx/provider/nofxos"
	"sort"
	"time"
)

// ScoreAI500FromSnapshot computes AI500 scores from a datafetch.Snapshot.
// This replaces GetAI500List which made its own Binance API calls.
// Zero API calls — pure in-memory computation from pre-fetched data.
func ScoreAI500FromSnapshot(snap *datafetch.Snapshot, limit int) ([]nofxos.CoinData, error) {
	if snap == nil || len(snap.Symbols) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 30
	}

	type scored struct {
		symbol    string
		ss        *datafetch.SymbolSnapshot
		pctAbs    float64
		qvLog     float64
		cntLog    float64
		rawScore  float64
	}

	// ---- Step 1: Collect raw values ----
	var pool []scored
	for sym, ss := range snap.Symbols {
		if !isUSDTPerp(sym) {
			continue
		}
		if ss.QuoteVolume24h <= 0 {
			continue
		}
		pool = append(pool, scored{
			symbol: sym,
			ss:     ss,
			pctAbs: math.Abs(ss.PriceChange24h),
			qvLog:  math.Log10(ss.QuoteVolume24h + 1),
			cntLog: math.Log10(float64(ss.TradeCount24h) + 1),
		})
	}

	if len(pool) == 0 {
		return nil, nil
	}

	// ---- Step 2: Min-max normalization ----
	minPct, maxPct := pool[0].pctAbs, pool[0].pctAbs
	minQV, maxQV := pool[0].qvLog, pool[0].qvLog
	minCnt, maxCnt := pool[0].cntLog, pool[0].cntLog

	for _, p := range pool {
		if p.pctAbs < minPct { minPct = p.pctAbs }
		if p.pctAbs > maxPct { maxPct = p.pctAbs }
		if p.qvLog < minQV { minQV = p.qvLog }
		if p.qvLog > maxQV { maxQV = p.qvLog }
		if p.cntLog < minCnt { minCnt = p.cntLog }
		if p.cntLog > maxCnt { maxCnt = p.cntLog }
	}

	pctRange := maxPct - minPct
	qvRange := maxQV - minQV
	cntRange := maxCnt - minCnt
	if pctRange == 0 { pctRange = 1 }
	if qvRange == 0 { qvRange = 1 }
	if cntRange == 0 { cntRange = 1 }

	// ---- Step 3: Compute scores ----
	for i := range pool {
		p := &pool[i]
		normPct := (p.pctAbs - minPct) / pctRange * 100
		normQV := (p.qvLog - minQV) / qvRange * 100
		normCnt := (p.cntLog - minCnt) / cntRange * 100

		score := normPct*0.50 + normQV*0.25 + normCnt*0.25
		score = clamp01(score, 0, 100)

		// Wash-trading penalty
		if p.ss.TradeCount24h > 5_000_000 {
			score *= 0.60
		}
		// Mainstream coin discount
		if excludedMainstreamCoins[p.symbol] {
			score *= 0.70
		}

		// Signal bonus from snapshot data
		bonus := ai500SignalBonus(p.ss)
		score += bonus

		p.rawScore = score
	}

	// ---- Step 4: Sort and build result ----
	sort.Slice(pool, func(i, j int) bool {
		return pool[i].rawScore > pool[j].rawScore
	})

	topN := limit
	if topN > len(pool) {
		topN = len(pool)
	}

	now := time.Now().Unix()
	coins := make([]nofxos.CoinData, 0, topN)
	for i := 0; i < topN; i++ {
		p := pool[i]
		coins = append(coins, nofxos.CoinData{
			Pair:            p.symbol,
			Score:           p.rawScore,
			StartTime:       now,
			StartPrice:      p.ss.Price,
			LastScore:       p.rawScore,
			MaxScore:        p.rawScore,
			MaxPrice:        p.ss.Price,
			IncreasePercent: p.ss.PriceChange24h,
			IsAvailable:     true,
		})
	}

	return coins, nil
}

// ai500SignalBonus computes signal bonuses from snapshot data.
// Mirrors computeSignalBonus but reads from SymbolSnapshot instead of API calls.
func ai500SignalBonus(ss *datafetch.SymbolSnapshot) float64 {
	bonus := 0.0

	// RSI14 oversold (< 30): +15
	klines1h, ok := ss.Klines["1h"]
	if ok && len(klines1h) >= 15 {
		rsi := rsiFromKlines(klines1h, 14)
		if rsi > 0 && rsi < 30 {
			bonus += 15
		}
	}

	// Volume breakout: latest vol > 5-bar avg × 2: +10
	if ok && len(klines1h) >= 6 {
		latestVol := klines1h[len(klines1h)-1].Volume
		avgVol := 0.0
		for _, k := range klines1h[len(klines1h)-6 : len(klines1h)-1] {
			avgVol += k.Volume
		}
		avgVol /= 5
		if avgVol > 0 && latestVol > avgVol*2 {
			bonus += 10
		}
	}

	// OI increase + price increase: +10
	if ss.OIDelta1h > 0 && ss.PriceChange24h > 0 {
		bonus += 10
	}

	// Funding rate signal
	if ss.FundingRate < -0.0005 {
		bonus += 15 // extreme bearish crowding
	} else if ss.FundingRate < 0 {
		bonus += 8 // shorts paying
	} else if ss.FundingRate > 0.001 {
		bonus -= 10 // overleveraged longs
	}

	return bonus
}

// rsiFromKlines computes RSI from datafetch.Kline using Wilder's smoothing.
func rsiFromKlines(klines []datafetch.Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
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
		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
	}

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

func clamp01(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
