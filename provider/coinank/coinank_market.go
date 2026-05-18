package coinank

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"nofx/provider/coinank/coinank_enum"
	"nofx/provider/nofxos"
)

// excludedMainstreamCoins is the same set used by local/client.go.
// Mainstream coins get a 0.70 penalty so mid/small caps rank higher.
var excludedMainstreamCoins = map[string]bool{
	"BTCUSDT": true, "ETHUSDT": true, "BNBUSDT": true,
	"SOLUSDT": true, "XRPUSDT": true, "ADAUSDT": true,
	"DOGEUSDT": true, "DOTUSDT": true, "AVAXUSDT": true,
	"MATICUSDT": true, "LINKUSDT": true, "LTCUSDT": true,
	"UNIUSDT": true, "ATOMUSDT": true, "NEARUSDT": true,
	"AAVEUSDT": true, "FILUSDT": true, "TRXUSDT": true,
	"EOSUSDT": true, "ETCUSDT": true, "FTMUSDT": true,
	"ALGOUSDT": true, "XLMUSDT": true, "HBARUSDT": true,
	"APTUSDT": true, "ARBUSDT": true, "OPUSDT": true,
	"SUIUSDT": true, "INJUSDT": true, "TIAUSDT": true,
}

// GetAI500Fallback fetches the CoinAnk Visual Screener (1h interval),
// scores each coin by |priceChg|*0.50 + normalized(oiChg)*0.25 + normalized(voChg)*0.25,
// applies a mainstream-coin penalty, and returns the top 30 as []nofxos.CoinData.
//
// This is intended as a fallback when the Binance ticker API is unavailable.
func (c *CoinankClient) GetAI500Fallback(ctx context.Context) ([]nofxos.CoinData, error) {
	// 1. Fetch Visual Screener data (1h interval)
	entries, err := c.VisualScreener(ctx, coinank_enum.Hour1)
	if err != nil {
		return nil, fmt.Errorf("coinank: VisualScreener failed: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("coinank: VisualScreener returned 0 entries")
	}

	// 2. Convert baseCoin to BASEUSDT format and collect for scoring
	type scoredEntry struct {
		symbol  string
		priceChg float64
		oiChg    float64
		voChg    float64
		score    float64
	}

	pool := make([]scoredEntry, 0, len(entries))
	for _, e := range entries {
		symbol := e.BaseCoin + "USDT"
		pool = append(pool, scoredEntry{
			symbol:   symbol,
			priceChg: e.PriceChg,
			oiChg:    e.OiChg,
			voChg:    e.VoChg,
		})
	}

	// 3. Compute log-space min/max for OI and Volume change normalization
	// Use absolute values for OI and Volume since we care about magnitude
	var minOILog, maxOILog float64
	var minVoLog, maxVoLog float64
	var minPctAbs, maxPctAbs float64
	first := true

	for _, p := range pool {
		pctAbs := math.Abs(p.priceChg)
		oiLog := math.Log10(math.Abs(p.oiChg) + 1)
		voLog := math.Log10(math.Abs(p.voChg) + 1)

		if first {
			minOILog, maxOILog = oiLog, oiLog
			minVoLog, maxVoLog = voLog, voLog
			minPctAbs, maxPctAbs = pctAbs, pctAbs
			first = false
		} else {
			if oiLog < minOILog { minOILog = oiLog }
			if oiLog > maxOILog { maxOILog = oiLog }
			if voLog < minVoLog { minVoLog = voLog }
			if voLog > maxVoLog { maxVoLog = voLog }
			if pctAbs < minPctAbs { minPctAbs = pctAbs }
			if pctAbs > maxPctAbs { maxPctAbs = pctAbs }
		}
	}

	pctAbsRange := maxPctAbs - minPctAbs
	oiLogRange := maxOILog - minOILog
	voLogRange := maxVoLog - minVoLog
	if pctAbsRange == 0 { pctAbsRange = 1 }
	if oiLogRange == 0 { oiLogRange = 1 }
	if voLogRange == 0 { voLogRange = 1 }

	// 4. Score each coin
	for i := range pool {
		p := &pool[i]
		pctAbs := math.Abs(p.priceChg)
		oiLog := math.Log10(math.Abs(p.oiChg) + 1)
		voLog := math.Log10(math.Abs(p.voChg) + 1)

		normPctAbs := ((pctAbs - minPctAbs) / pctAbsRange) * 100
		normOI := ((oiLog - minOILog) / oiLogRange) * 100
		normVO := ((voLog - minVoLog) / voLogRange) * 100

		score := normPctAbs*0.50 + normOI*0.25 + normVO*0.25
		if score < 0 { score = 0 }
		if score > 100 { score = 100 }

		// Mainstream coin penalty
		if excludedMainstreamCoins[p.symbol] {
			score *= 0.70
		}
		p.score = score
	}

	// 5. Sort descending by score
	sort.SliceStable(pool, func(i, j int) bool {
		return pool[i].score > pool[j].score
	})

	// 6. Return top 30 as []nofxos.CoinData
	topN := 30
	if len(pool) < topN {
		topN = len(pool)
	}

	now := time.Now().Unix()
	coins := make([]nofxos.CoinData, 0, topN)
	for i := 0; i < topN; i++ {
		p := pool[i]
		coins = append(coins, nofxos.CoinData{
			Pair:        p.symbol,
			Score:       p.score,
			StartTime:   now,
			StartPrice:  0, // no price available from screener
			LastScore:   p.score,
			MaxScore:    p.score,
			MaxPrice:    0,
			IsAvailable: true,
		})
	}

	return coins, nil
}
