package engine

import (
	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/logger"
)

// ============================================================================
// Scoring Utilities and Normalization
// ============================================================================

// NeutralBandScorePenalty downgrades a signal whose bull/bear difference falls
// inside the neutral band (|diff| <= DirectionMargin) instead of vetoing it.
// The dominant side is kept as the direction, but FinalScore is multiplied by
// this factor so the router's MinScore quality gate keeps working ("downgrade
// not veto"). DirectionMargin itself stays configurable via HubConfig.
const NeutralBandScorePenalty = 0.8

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

// directionalScores computes the normalized weighted bull/bear totals used by
// both calcFinalScore and determineDirection, so the two stay in the same unit
// (0-100) and the same arithmetic.
func directionalScores(set *IndicatorSet, cfg HubConfig) (bull, bear float64) {
	normTBull := set.TechBullScore / 40 * 100
	normTBeat := set.TechBearScore / 40 * 100
	normQBull := set.QuantBullScore / 40 * 100
	normQBeat := set.QuantBearScore / 40 * 100
	normSBull := set.SocialBullScore / 20 * 100
	normSBeat := set.SocialBearScore / 20 * 100

	bull = normTBull*(cfg.TechWeight/100) +
		normQBull*(cfg.QuantWeight/100) +
		normSBull*(cfg.SocialWeight/100)

	bear = normTBeat*(cfg.TechWeight/100) +
		normQBeat*(cfg.QuantWeight/100) +
		normSBeat*(cfg.SocialWeight/100)

	return bull, bear
}

// calcFinalScore computes the weighted final score from all sub-scores.
// Sub-scores are normalized to 0-100 before weighting:
//
//	Tech  max 40 → /40*100,  Quant max 40 → /40*100,  Social max 20 → /20*100
func calcFinalScore(set *IndicatorSet, cfg HubConfig) float64 {
	bull, bear := directionalScores(set, cfg)

	dominant := bull
	if bear > bull {
		dominant = bear
	}

	return clamp(dominant, 0, 100)
}

// determineDirection determines trade direction based on bull/bear score difference.
// Uses the SAME normalized weighted scale as calcFinalScore so DirectionMargin
// operates in the same unit (0-100) as FinalScore.
//
// P1-1 "downgrade not veto": when the difference falls inside the neutral band
// (|diff| <= DirectionMargin) the dominant side is kept as direction and
// weak=true is returned so the caller can downgrade the signal's score; the
// signal is no longer discarded as "no direction". Only a perfect tie
// (diff == 0) still returns NEUTRAL (weak=false), which the router filters as
// before.
func determineDirection(set *IndicatorSet, cfg HubConfig) (dir int, weak bool) {
	bull, bear := directionalScores(set, cfg)
	diff := bull - bear

	if diff > cfg.DirectionMargin {
		return 1, false // LONG, decisive
	}
	if diff < -cfg.DirectionMargin {
		return -1, false // SHORT, decisive
	}
	if diff > 0 {
		return 1, true // LONG, neutral-band weak
	}
	if diff < 0 {
		return -1, true // SHORT, neutral-band weak
	}
	return 0, false // NEUTRAL: perfect tie
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
	// P1-1: a neutral-band direction keeps the dominant side and downgrades
	// FinalScore (×NeutralBandScorePenalty) instead of being vetoed, so the
	// router's MinScore gate still applies.
	applyDirectionAndScore(set, cfg)

	return set
}

// applyDirectionAndScore implements Step 5 of scoreSymbol: computes FinalScore
// and Direction. A neutral-band (weak) direction keeps the dominant side and
// downgrades FinalScore by NeutralBandScorePenalty so the router's MinScore
// gate still decides ("downgrade, not veto"); only a perfect tie (diff == 0)
// stays NEUTRAL and is filtered by the router as before.
func applyDirectionAndScore(set *IndicatorSet, cfg HubConfig) {
	set.FinalScore = calcFinalScore(set, cfg)
	dir, weak := determineDirection(set, cfg)
	if weak {
		before := set.FinalScore
		set.FinalScore = before * NeutralBandScorePenalty
		bull, bear := directionalScores(set, cfg)
		logger.Infof("[p1-1] neutral-band downgrade symbol=%s bull=%.2f bear=%.2f margin=%.2f dir=%d score=%.2f->%.2f (x%.2f)",
			set.Symbol, bull, bear, cfg.DirectionMargin, dir, before, set.FinalScore, NeutralBandScorePenalty)
	}
	set.Direction = dir
}
