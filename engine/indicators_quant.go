package engine

import "github.com/Aixxww/AiT/datafetch"

// ============================================================================
// Quantitative Indicator Calculations
// Derives derivatives/market-structure signals from SymbolSnapshot data.
// Fields OI, OIDelta1h, LongShortRatio, LSRPrev, TakerBuyRatio etc.
// are directly on SymbolSnapshot (no Quant sub-struct).
// ============================================================================

// computeQuantIndicators computes all quant indicators from the snapshot.
func computeQuantIndicators(snap *datafetch.SymbolSnapshot) *IndicatorSet {
	set := &IndicatorSet{Symbol: snap.Symbol}
	if snap == nil {
		return set
	}

	// --- OI Score ---
	// OIDelta1h is OI change % over 1h.
	// OI↑ + Price↑ = bullish (+25), OI↑ + Price↓ = bearish (-25)
	oiDelta := snap.OIDelta1h
	priceDelta := snap.PriceChange24h // use 24h as proxy; or 1h if available

	if oiDelta > 0.5 {
		if priceDelta > 0.1 {
			set.OIScore = 25
		} else if priceDelta < -0.1 {
			set.OIScore = -25
		} else {
			set.OIScore = 5
		}
	} else if oiDelta < -0.5 {
		set.OIScore = 0
	}

	// --- OI Spike Score ---
	absDelta := abs(oiDelta)
	if absDelta > 5 {
		set.OISpikeScore = 100
	} else if absDelta > 2 {
		set.OISpikeScore = normalize(absDelta, 2, 5)
	} else if absDelta > 1 {
		set.OISpikeScore = normalize(absDelta, 1, 2) * 0.3
	}

	// --- Funding Rate Score ---
	// Negative FR = bullish (shorts pay longs), positive FR = bearish
	// Normal range: -0.01% to +0.01% (-0.0001 to +0.0001) → no score
	// Elevated: 0.01-0.03% → moderate bearish; >0.03% → strong bearish
	fr := snap.FundingRate
	if fr < -0.0001 {
		// Elevated negative funding → bullish
		set.FundingScore = clamp(-abs(fr)*1000, -100, 0)
	} else if fr > 0.0003 {
		// Extreme positive funding → strong bearish
		set.FundingScore = clamp(fr*1500, 0, 100)
	} else if fr > 0.0001 {
		// Mild positive funding → weak bearish
		set.FundingScore = clamp(fr*800, 0, 30)
	}

	// --- Long/Short Ratio Score ---
	// LSR < 0.8 and rising = bullish, LSR > 1.2 and falling = bearish
	lsr := snap.LongShortRatio
	lsrPrev := snap.LSRPrev
	if lsr > 0 {
		lsrDelta := lsr - lsrPrev
		if lsr < 0.8 && lsrDelta > 0 {
			set.LSRScore = normalize(0.8-lsr, 0, 0.5) * 60
		} else if lsr > 1.2 && lsrDelta < 0 {
			set.LSRScore = -normalize(lsr-1.2, 0, 0.5) * 60
		} else if lsr < 0.7 {
			set.LSRScore = 30
		} else if lsr > 1.3 {
			set.LSRScore = -30
		}
	}

	// --- Taker Buy Ratio Score ---
	// > 0.55 = bullish, < 0.45 = bearish
	if snap.TakerBuyRatio > 0.55 {
		set.TakerScore = normalize(snap.TakerBuyRatio, 0.55, 0.7) * 60
	} else if snap.TakerBuyRatio > 0 && snap.TakerBuyRatio < 0.45 {
		set.TakerScore = -normalize(0.45-snap.TakerBuyRatio, 0, 0.15) * 60
	}

	// --- Volume Anomaly Score ---
	// Volume24h as proxy; OISpikeData for volume anomaly if available
	if len(snap.OISpikeData) > 3 {
		// Use the latest OISpikeData entries as volume proxy
		latest := snap.OISpikeData[len(snap.OISpikeData)-1]
		avg := 0.0
		for _, v := range snap.OISpikeData {
			avg += abs(v)
		}
		avg /= float64(len(snap.OISpikeData))
		if avg > 0 && abs(latest) > avg*2 {
			set.VolumeScore = normalize(abs(latest)/avg, 2, 5) * 100
		}
	}

	return set
}

// scoreQuantBull returns a bullish quant score (0-40).
func scoreQuantBull(set *IndicatorSet, cfg HubConfig) float64 {
	score := 0.0
	if cfg.OIScoreEnabled && set.OIScore > 0 {
		score += set.OIScore / 25 * 12 // OIScore range [-25,+25], max contribution 12
	}
	if cfg.OISpikeEnabled && set.OISpikeScore > 50 && set.OIScore > 0 {
		score += set.OISpikeScore / 100 * 5
	}
	if cfg.FundingEnabled && set.FundingScore < 0 {
		score += abs(set.FundingScore) / 100 * 8 // FundingScore range [0,100], max contribution 8
	}
	if cfg.LSREnabled && set.LSRScore > 0 {
		score += set.LSRScore / 60 * 5 // LSRScore range [-60,+60], max contribution 5
	}
	if cfg.TakerEnabled && set.TakerScore > 0 {
		score += set.TakerScore / 60 * 5 // TakerScore range [-60,+60], max contribution 5
	}
	if cfg.VolumeEnabled && set.VolumeScore > 50 && set.OIScore > 0 {
		score += set.VolumeScore / 100 * 5
	}
	return clamp(score, 0, 40)
}

// scoreQuantBear returns a bearish quant score (0-40).
func scoreQuantBear(set *IndicatorSet, cfg HubConfig) float64 {
	score := 0.0
	if cfg.OIScoreEnabled && set.OIScore < 0 {
		score += abs(set.OIScore) / 25 * 12 // OIScore range [-25,+25], max contribution 12
	}
	if cfg.OISpikeEnabled && set.OISpikeScore > 50 && set.OIScore < 0 {
		score += set.OISpikeScore / 100 * 5
	}
	if cfg.FundingEnabled && set.FundingScore > 0 {
		score += set.FundingScore / 100 * 8 // FundingScore range [0,100], max contribution 8
	}
	if cfg.LSREnabled && set.LSRScore < 0 {
		score += abs(set.LSRScore) / 60 * 5 // LSRScore range [-60,+60], max contribution 5
	}
	if cfg.TakerEnabled && set.TakerScore < 0 {
		score += abs(set.TakerScore) / 60 * 5 // TakerScore range [-60,+60], max contribution 5
	}
	if cfg.VolumeEnabled && set.VolumeScore > 50 && set.OIScore < 0 {
		score += set.VolumeScore / 100 * 5
	}
	return clamp(score, 0, 40)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
