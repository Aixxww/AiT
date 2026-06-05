package engine

import "github.com/Aixxww/AiT/datafetch"

// computeSocialIndicators computes social indicators from the snapshot.
func computeSocialIndicators(snap *datafetch.SymbolSnapshot) *IndicatorSet {
	set := &IndicatorSet{Symbol: snap.Symbol}
	if snap == nil {
		return set
	}
	s := snap.Social
	set.SocialHeatScore = clamp(s.HeatScore, 0, 100)
	set.SocialSentiment = clamp(s.Sentiment, 0, 100) // 0-100 scale
	if s.VolumeChange > 0 {
		set.SocialVolumePct = clamp(s.VolumeChange, 0, 100)
	}
	return set
}

func scoreSocialBull(set *IndicatorSet) float64 {
	score := 0.0
	if set.SocialHeatScore > 70 {
		score += 5
	} else if set.SocialHeatScore > 50 {
		score += 2
	}
	if set.SocialSentiment > 60 {
		score += 5
	} else if set.SocialSentiment > 40 {
		score += 2
	} else if set.SocialSentiment < 20 {
		score -= 3
	}
	if set.SocialVolumePct > 50 {
		score += 4
	} else if set.SocialVolumePct > 20 {
		score += 2
	}
	if set.SocialHeatScore > 80 {
		score += 3
	}
	return clamp(score, 0, 20)
}

func scoreSocialBear(set *IndicatorSet) float64 {
	score := 0.0
	if set.SocialHeatScore > 85 {
		score += 4
	}
	if set.SocialSentiment < 30 {
		score += 5
	} else if set.SocialSentiment < 40 {
		score += 2
	} else if set.SocialSentiment > 70 {
		score -= 3
	}
	if set.SocialVolumePct > 50 && set.SocialSentiment < 40 {
		score += 5
	}
	if set.SocialHeatScore > 80 && set.SocialSentiment < 50 {
		score += 4
	}
	return clamp(score, 0, 20)
}
