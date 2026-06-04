package local

import "math"

// ============================================================================
// Hunter v7 — Risk Scorer
// ============================================================================
// Computes a composite risk score [0, 100] for each signal.
// Higher = more dangerous. Used to filter and tag signals.

// AssessV7Risk computes the risk score for a signal output.
func AssessV7Risk(sig *V7SignalOutput, ctx *V7SymbolContext) float64 {
	var risk float64

	// 1. Volatility risk
	if ctx.Change24h != 0 {
		absChange := math.Abs(ctx.Change24h)
		if absChange > 50 {
			risk += 30
			sig.RiskTags = append(sig.RiskTags, "extreme_volatility")
		} else if absChange > 25 {
			risk += 15
			sig.RiskTags = append(sig.RiskTags, "high_volatility")
		}
		if ctx.Change24h > 60 {
			risk += 20
			sig.RiskTags = append(sig.RiskTags, "extended_24h_gain")
			sig.RiskTags = append(sig.RiskTags, "do_not_market_chase")
		}
	}

	// 2. Funding rate risk
	snap := ctx.Snapshot
	if snap != nil {
		absFund := math.Abs(snap.FundingRate)
		if absFund > 0.001 { // >0.10%
			risk += 30
			sig.RiskTags = append(sig.RiskTags, "funding_extreme")
		} else if absFund > 0.0005 { // >0.05%
			risk += 15
			sig.RiskTags = append(sig.RiskTags, "funding_elevated")
		}

		// 3. Crowding risk (LSR extreme)
		if snap.LSR > 2.5 || snap.LSR < 0.4 {
			risk += 15
			sig.RiskTags = append(sig.RiskTags, "crowding_extreme")
		} else if snap.LSR > 2.0 || snap.LSR < 0.6 {
			risk += 8
			sig.RiskTags = append(sig.RiskTags, "crowding_elevated")
		}
	}

	// 4. Wash trade risk
	if snap != nil && snap.TradeCount24h > 1_000_000 && snap.QuoteVolume24h > 0 {
		avgTradeSize := snap.QuoteVolume24h / float64(snap.TradeCount24h)
		if avgTradeSize < 5 {
			risk += 25
			sig.RiskTags = append(sig.RiskTags, "wash_volume_high")
		}
	}

	// 5. Liquidity risk
	if snap != nil && snap.QuoteVolume24h < 5_000_000 { // < $5M daily volume
		risk += 15
		sig.RiskTags = append(sig.RiskTags, "low_liquidity")
	} else if snap != nil && snap.QuoteVolume24h < 10_000_000 {
		risk += 8
		sig.RiskTags = append(sig.RiskTags, "moderate_liquidity")
	}

	// 6. Regime vs direction risk
	if (sig.Direction == V7DirLong && sig.MarketRegime == V7RegimeTrendDown) ||
		(sig.Direction == V7DirShort && sig.MarketRegime == V7RegimeTrendUp) {
		risk += 15
		sig.RiskTags = append(sig.RiskTags, "regime_against_direction")
	}

	// 7. OI anomaly risk
	if snap != nil {
		oiDelta := math.Abs(snap.OIDelta1h)
		if oiDelta > 30 {
			risk += 15
			sig.RiskTags = append(sig.RiskTags, "oi_anomaly")
		}
	}

	return clampFloat(risk, 0, 100)
}

// ClassifyV7RiskLevel returns the risk level string.
func ClassifyV7RiskLevel(riskScore float64) V7RiskLevel {
	switch {
	case riskScore <= 30:
		return V7RiskLow
	case riskScore <= 55:
		return V7RiskMedium
	case riskScore <= 75:
		return V7RiskHigh
	default:
		return V7RiskExtreme
	}
}

// AssessLiquidityScore computes a liquidity quality score [0, 100].
// Higher = better liquidity.
func AssessLiquidityScore(ctx *V7SymbolContext) float64 {
	snap := ctx.Snapshot
	if snap == nil {
		return 30 // Conservative default
	}

	var score float64

	// 24h quote volume (max score at > $50M)
	if snap.QuoteVolume24h > 50_000_000 {
		score += 35
	} else if snap.QuoteVolume24h > 20_000_000 {
		score += 25
	} else if snap.QuoteVolume24h > 10_000_000 {
		score += 15
	} else {
		score += 5
	}

	// OI value (max score at > $5M)
	if snap.OI > 5_000_000 {
		score += 30
	} else if snap.OI > 1_000_000 {
		score += 20
	} else if snap.OI > 500_000 {
		score += 10
	}

	// Trade count (normal activity indicator)
	if snap.TradeCount24h > 100_000 {
		score += 15
	} else if snap.TradeCount24h > 10_000 {
		score += 10
	} else {
		score += 5
	}

	// OI/Volume ratio sanity (not wash traded)
	if snap.QuoteVolume24h > 0 && snap.OI > 0 {
		ratio := snap.OI / snap.QuoteVolume24h
		if ratio > 0.01 && ratio < 0.5 {
			score += 20 // Healthy ratio
		} else {
			score += 5 // Suspicious
		}
	}

	return clampFloat(score, 0, 100)
}
