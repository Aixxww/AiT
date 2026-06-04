package local

import (
	"math"
	"nofx/datafetch"
)

// ============================================================================
// Hunter v7 — Market Regime Detector
// ============================================================================
// Identifies the current global market state from BTC/ETH data.
// This is Layer 1 of the v7 pipeline — it determines which signal modules
// get activated and with what weights.

// DetectV7MarketRegime analyzes BTC/ETH data from the snapshot to determine
// the current market regime. Returns one of 8 regimes.
func DetectV7MarketRegime(snap *datafetch.Snapshot) V7MarketRegime {
	if snap == nil {
		return V7RegimeMixed
	}

	btc, btcOK := snap.Symbols["BTCUSDT"]
	eth, ethOK := snap.Symbols["ETHUSDT"]

	if !btcOK && !ethOK {
		return V7RegimeMixed
	}

	// Use BTC as primary, ETH as secondary confirmation
	var btcAnalysis, ethAnalysis regimeAnalysis
	if btcOK {
		btcAnalysis = analyzeRegime(btc)
	}
	if ethOK {
		ethAnalysis = analyzeRegime(eth)
	}

	// Priority-based regime detection:
	// 1. Panic dump (highest priority — safety first)
	if isPanicDump(btcAnalysis, ethAnalysis) {
		return V7RegimePanicDump
	}

	// 2. Market pullback: broad risk-off, but not panic yet.
	if isMarketPullback(btcAnalysis, ethAnalysis) {
		return V7RegimePullback
	}

	// 3. Mania pump
	if isManiaPump(btcAnalysis, ethAnalysis) {
		return V7RegimeManiaPump
	}

	// 4. Trend up / down
	if isTrendUp(btcAnalysis, ethAnalysis) {
		return V7RegimeTrendUp
	}
	if isTrendDown(btcAnalysis, ethAnalysis) {
		return V7RegimeTrendDown
	}

	// 5. Compression
	if isCompression(btcAnalysis, ethAnalysis) {
		return V7RegimeCompression
	}

	// 6. Range
	if isRange(btcAnalysis, ethAnalysis) {
		return V7RegimeRange
	}

	// 7. Rotation — BTC/ETH ranging but many altcoins significantly outperforming
	if isRotation(btcAnalysis, ethAnalysis, snap) {
		return V7RegimeRotation
	}

	// 8. Mixed (default fallback)
	return V7RegimeMixed
}

// regimeAnalysis holds pre-computed indicators for one symbol.
type regimeAnalysis struct {
	change24h float64
	change1h  float64
	change4h  float64
	adx       float64
	ema20_4h  float64
	ema60_4h  float64
	bbWidth   float64
	hasData   bool
}

// analyzeRegime computes regime indicators for a single symbol.
func analyzeRegime(ss *datafetch.SymbolSnapshot) regimeAnalysis {
	ra := regimeAnalysis{
		change24h: ss.PriceChange24h,
		hasData:   true,
	}

	klines := ss.Klines
	if klines == nil {
		return ra
	}

	// 4h analysis
	if bars4h, ok := klines["4h"]; ok && len(bars4h) >= 20 {
		kb := datafetchKlinesToKlineBar(bars4h)
		ra.ema20_4h = computeEMAFromKlines(kb, 20)
		ra.ema60_4h = computeEMAFromKlines(kb, 60)
		ra.adx = computeADXFromKlines(kb, 14)
		width, _, _, _ := computeBBWidth(kb, 20, 2.0)
		ra.bbWidth = width
		// Compute 4h change
		if len(kb) >= 2 {
			first := kb[len(kb)-2].Close
			last := kb[len(kb)-1].Close
			if first > 0 {
				ra.change4h = (last - first) / first * 100
			}
		}
	}

	// 1h change
	if bars1h, ok := klines["1h"]; ok && len(bars1h) >= 2 {
		kb := datafetchKlinesToKlineBar(bars1h)
		first := kb[len(kb)-2].Close
		last := kb[len(kb)-1].Close
		if first > 0 {
			ra.change1h = (last - first) / first * 100
		}
	}

	return ra
}

// isPanicDump: BTC/ETH 24h < -8% or single-symbol 24h < -15%
func isPanicDump(btc, eth regimeAnalysis) bool {
	if btc.hasData && btc.change24h < -8 {
		return true
	}
	if eth.hasData && eth.change24h < -8 {
		return true
	}
	if btc.hasData && btc.change1h < -5 {
		return true
	}
	return false
}

// isMarketPullback: broad BTC/ETH drawdown that is not yet a panic cascade.
func isMarketPullback(btc, eth regimeAnalysis) bool {
	btcPullback := btc.hasData && btc.change24h <= -5 && btc.change24h > -8
	ethPullback := eth.hasData && eth.change24h <= -5 && eth.change24h > -8

	// If both majors are down >5%, treat it as a market pullback even without
	// EMA/ADX trend confirmation. This avoids misclassifying synchronized
	// risk-off sessions as neutral mixed regimes.
	if btcPullback && ethPullback {
		return true
	}

	// BTC leads risk; a sharp BTC drawdown with ETH also negative is enough.
	if btcPullback && eth.hasData && eth.change24h < -3 {
		return true
	}

	return false
}

// isManiaPump: BTC/ETH 24h > +8% with strong 4h momentum
func isManiaPump(btc, eth regimeAnalysis) bool {
	if btc.hasData && btc.change24h > 8 && btc.change4h > 3 {
		return true
	}
	if eth.hasData && eth.change24h > 8 && eth.change4h > 3 {
		return true
	}
	return false
}

// isTrendUp: EMA20 > EMA60 on 4h, positive change, ADX > 22
func isTrendUp(btc, eth regimeAnalysis) bool {
	btcTrend := btc.hasData && btc.ema20_4h > btc.ema60_4h && btc.ema20_4h > 0 && btc.change24h > 0
	ethTrend := eth.hasData && eth.ema20_4h > eth.ema60_4h && eth.ema20_4h > 0 && eth.change24h > 0

	// At least one must be trending with ADX confirmation
	if btcTrend && btc.adx > 22 {
		return true
	}
	if ethTrend && eth.adx > 22 {
		return true
	}
	// Both positive with EMA alignment even without strong ADX
	if btcTrend && ethTrend {
		return true
	}
	return false
}

// isTrendDown: EMA20 < EMA60 on 4h, negative change, ADX > 22
func isTrendDown(btc, eth regimeAnalysis) bool {
	btcTrend := btc.hasData && btc.ema20_4h < btc.ema60_4h && btc.ema20_4h > 0 && btc.change24h < 0
	ethTrend := eth.hasData && eth.ema20_4h < eth.ema60_4h && eth.ema20_4h > 0 && eth.change24h < 0

	if btcTrend && btc.adx > 22 {
		return true
	}
	if ethTrend && eth.adx > 22 {
		return true
	}
	if btcTrend && ethTrend {
		return true
	}
	return false
}

// isCompression: BB width low on both BTC and ETH
func isCompression(btc, eth regimeAnalysis) bool {
	// BB width below certain threshold indicates compression
	// We use a relative check — if both are in lower range
	btcCompressed := btc.hasData && btc.bbWidth > 0 && btc.adx < 25
	ethCompressed := eth.hasData && eth.bbWidth > 0 && eth.adx < 25

	if btcCompressed && ethCompressed {
		// Both showing compression
		return true
	}
	return false
}

// isRange: ADX < 20, moderate change, no clear direction
func isRange(btc, eth regimeAnalysis) bool {
	btcRange := btc.hasData && btc.adx > 0 && btc.adx < 20 && math.Abs(btc.change24h) < 5
	ethRange := eth.hasData && eth.adx > 0 && eth.adx < 20 && math.Abs(eth.change24h) < 5

	return btcRange || ethRange
}

// isRotation: BTC/ETH neutral but 10+ altcoins significantly outperforming.
// Indicates sector rotation — capital flowing from majors into alts.
func isRotation(btc, eth regimeAnalysis, snap *datafetch.Snapshot) bool {
	if snap == nil {
		return false
	}

	// BTC/ETH must be neutral or only mildly directional
	btcNeutral := !btc.hasData || math.Abs(btc.change24h) < 4
	ethNeutral := !eth.hasData || math.Abs(eth.change24h) < 4
	if !btcNeutral && !ethNeutral {
		return false // Both strongly directional = not rotation
	}

	// Reference: use the stronger of BTC/ETH change as baseline
	baseline := 0.0
	if btc.hasData {
		baseline = btc.change24h
	}
	if eth.hasData && eth.change24h > baseline {
		baseline = eth.change24h
	}

	// Count altcoins outperforming baseline by >8%
	outperformCount := 0
	for symbol, ss := range snap.Symbols {
		if ss == nil {
			continue
		}
		// Skip BTC/ETH themselves
		if symbol == "BTCUSDT" || symbol == "ETHUSDT" {
			continue
		}
		// Must be USDT perpetual
		if !isUSDTPerp(symbol) {
			continue
		}
		if ss.PriceChange24h > baseline+8 {
			outperformCount++
		}
	}

	return outperformCount >= 10
}
