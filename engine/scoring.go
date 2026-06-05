package engine

import (
	"github.com/Aixxww/AiT/datafetch"
)

// ============================================================================
// Scoring Utilities and Normalization
// ============================================================================

// normalize clamps value to [min, max] and returns a 0-100 normalized score.
func normalize(value, min, max float64) float64 {
	if max <= min {
		return 0
	}
	v := clamp(value, min, max)
	return (v - min) / (max - min) * 100
}

// clamp restricts value to [min, max].
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// calcFinalScore computes the weighted final score from all sub-scores.
// Sub-scores are normalized to 0-100 before weighting:
//
//	Tech  max 40 → /40*100,  Quant max 40 → /40*100,  Social max 20 → /20*100
func calcFinalScore(set *IndicatorSet, cfg HubConfig) float64 {
	// Normalize each sub-score to 0-100 first
	normTBull := set.TechBullScore / 40 * 100
	normTBeat := set.TechBearScore / 40 * 100
	normQBull := set.QuantBullScore / 40 * 100
	normQBeat := set.QuantBearScore / 40 * 100
	normSBull := set.SocialBullScore / 20 * 100
	normSBeat := set.SocialBearScore / 20 * 100

	bullTotal := normTBull*(cfg.TechWeight/100) +
		normQBull*(cfg.QuantWeight/100) +
		normSBull*(cfg.SocialWeight/100)

	bearTotal := normTBeat*(cfg.TechWeight/100) +
		normQBeat*(cfg.QuantWeight/100) +
		normSBeat*(cfg.SocialWeight/100)

	dominant := bullTotal
	if bearTotal > bullTotal {
		dominant = bearTotal
	}

	return clamp(dominant, 0, 100)
}

// determineDirection determines trade direction based on bull/bear score difference.
// Uses the SAME normalized weighted scale as calcFinalScore so DirectionMargin
// operates in the same unit (0-100) as FinalScore.
func determineDirection(set *IndicatorSet, cfg HubConfig) int {
	normTBull := set.TechBullScore / 40 * 100
	normTBeat := set.TechBearScore / 40 * 100
	normQBull := set.QuantBullScore / 40 * 100
	normQBeat := set.QuantBearScore / 40 * 100
	normSBull := set.SocialBullScore / 20 * 100
	normSBeat := set.SocialBearScore / 20 * 100

	bull := normTBull*(cfg.TechWeight/100) + normQBull*(cfg.QuantWeight/100) + normSBull*(cfg.SocialWeight/100)
	bear := normTBeat*(cfg.TechWeight/100) + normQBeat*(cfg.QuantWeight/100) + normSBeat*(cfg.SocialWeight/100)
	diff := bull - bear

	if diff > cfg.DirectionMargin {
		return 1 // LONG
	}
	if diff < -cfg.DirectionMargin {
		return -1 // SHORT
	}
	return 0 // NEUTRAL
}

// determineGrade assigns a Grade based on the final score.
func determineGrade(score float64, cfg HubConfig) Grade {
	if score >= cfg.GradeSThreshold {
		return GradeS
	}
	if score >= cfg.GradeAThreshold {
		return GradeA
	}
	if score >= cfg.GradeBThreshold {
		return GradeB
	}
	return GradeC
}

// calcSLTP calculates Stop Loss and Take Profit levels.
// For LONG: SL below entry, TPs above entry.
// For SHORT: SL above entry, TPs below entry.
func calcSLTP(snap *datafetch.SymbolSnapshot, direction int, atr float64, cfg HubConfig) (sl, tp1, tp2, tp3 float64) {
	if snap == nil || atr <= 0 || direction == 0 {
		return 0, 0, 0, 0
	}

	entry := snap.Price

	switch direction {
	case 1: // LONG
		sl = entry - atr*cfg.StopLossATR
		tp1 = entry + atr*cfg.TP1ATR
		tp2 = entry + atr*cfg.TP2ATR
		tp3 = entry + atr*cfg.TP3ATR
	case -1: // SHORT
		sl = entry + atr*cfg.StopLossATR
		tp1 = entry - atr*cfg.TP1ATR
		tp2 = entry - atr*cfg.TP2ATR
		tp3 = entry - atr*cfg.TP3ATR
	}

	// Ensure positive prices
	if sl < 0 {
		sl = 0
	}
	if tp1 < 0 {
		tp1 = 0
	}
	if tp2 < 0 {
		tp2 = 0
	}
	if tp3 < 0 {
		tp3 = 0
	}

	return
}

// scoreSymbol performs complete scoring for a single symbol snapshot.
func scoreSymbol(snap *datafetch.SymbolSnapshot, cfg HubConfig) *IndicatorSet {
	// Step 1: Compute technical indicators
	techSet := computeTechIndicators(snap)

	// Step 2: Compute quant indicators
	quantSet := computeQuantIndicators(snap)

	// Step 3: Compute social indicators
	socialSet := computeSocialIndicators(snap)

	// Merge all into one IndicatorSet
	set := &IndicatorSet{
		Symbol: snap.Symbol,

		// Technical
		RSI14:      techSet.RSI14,
		MACDLine:   techSet.MACDLine,
		MACDSignal: techSet.MACDSignal,
		MACDHist:   techSet.MACDHist,
		BBUpper:    techSet.BBUpper,
		BBMiddle:   techSet.BBMiddle,
		BBLower:    techSet.BBLower,
		BBWidth:    techSet.BBWidth,
		EMA20:      techSet.EMA20,
		EMA50:      techSet.EMA50,
		EMA200:     techSet.EMA200,
		ATR14:      techSet.ATR14,

		// Quant
		OIScore:      quantSet.OIScore,
		OISpikeScore: quantSet.OISpikeScore,
		FundingScore: quantSet.FundingScore,
		LSRScore:     quantSet.LSRScore,
		TakerScore:   quantSet.TakerScore,
		VolumeScore:  quantSet.VolumeScore,

		// Social
		SocialHeatScore: socialSet.SocialHeatScore,
		SocialSentiment: socialSet.SocialSentiment,
		SocialVolumePct: socialSet.SocialVolumePct,
	}

	// Step 4: Score all sub-components
	set.TechBullScore = scoreTechBull(set, cfg)
	set.TechBearScore = scoreTechBear(set, cfg)
	set.QuantBullScore = scoreQuantBull(set, cfg)
	set.QuantBearScore = scoreQuantBear(set, cfg)
	set.SocialBullScore = scoreSocialBull(set)
	set.SocialBearScore = scoreSocialBear(set)

	// Step 5: Final score and direction
	set.FinalScore = calcFinalScore(set, cfg)
	set.Direction = determineDirection(set, cfg)

	return set
}
