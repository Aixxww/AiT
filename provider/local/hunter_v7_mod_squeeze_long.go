package local

// ============================================================================
// Module B: Short Squeeze Long
// ============================================================================
// Catches forced short-covering events where:
//   - Price surging (1h > 3%) while OI dropping (1h < -3%)
//   - Taker buy ratio elevated (15m > 0.55)
// This is a fast, aggressive setup — shorts are being liquidated and price
// is gapping up. Entry must be quick before the squeeze exhausts itself.

type shortSqueezeLongModule struct{}

func (m *shortSqueezeLongModule) Name() string           { return "short_squeeze_long" }
func (m *shortSqueezeLongModule) SetupType() V7SetupType { return V7SetupShortSqueezeLong }
func (m *shortSqueezeLongModule) Direction() V7Direction { return V7DirLong }

func (m *shortSqueezeLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx.CurrentPrice <= 0 {
		return false
	}
	snap := ctx.Snapshot
	if snap == nil {
		return false
	}
	// Price surging on 1h
	if ctx.Change1h <= 3 {
		return false
	}
	// OI dropping on 1h → forced covering / short liquidations
	if snap.OIDelta1h >= -3 {
		return false
	}
	// Taker buy dominant — buyers stepping into the squeeze
	if ctx.TakerBuy15m <= 0.55 {
		return false
	}
	return true
}

func (m *shortSqueezeLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupShortSqueezeLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryFastConfirm,
		Confidence:   "A",
		MarketRegime: regime,
	}

	snap := ctx.Snapshot
	var score float64

	// 1. Squeeze Strength (0-30): how severe is the forced covering?
	//    Price up + OI down = classic squeeze. Magnitude matters.
	priceSurge := ctx.Change1h
	oiDrop := -snap.OIDelta1h // positive number = OI decline magnitude

	if priceSurge >= 8 && oiDrop >= 8 {
		score += 30
		sig.ReasonCodes = append(sig.ReasonCodes, "extreme_squeeze")
	} else if priceSurge >= 5 && oiDrop >= 5 {
		score += 25
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_squeeze")
	} else if priceSurge >= 3 && oiDrop >= 3 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_squeeze")
	} else {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "mild_squeeze")
	}

	// 2. Taker Momentum (0-25): aggressive buying driving the squeeze
	if ctx.TakerBuy15m > 0.65 {
		score += 25
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_aggressive_buy")
	} else if ctx.TakerBuy15m > 0.60 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_strong_buy")
	} else if ctx.TakerBuy15m > 0.55 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_moderate_buy")
	}

	// 3. Volume Expansion (0-20): 24h quote volume as proxy
	if snap.QuoteVolume24h > 0 {
		// Use absolute volume as a rough proxy; higher = more participation
		if snap.QuoteVolume24h > 5e8 { // > $500M
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "volume_massive")
		} else if snap.QuoteVolume24h > 1e8 { // > $100M
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "volume_high")
		} else if snap.QuoteVolume24h > 2e7 { // > $20M
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "volume_moderate")
		}
	}

	// 4. LSR Shift (0-15): long/short ratio turning bullish during squeeze
	if snap.LSR > 1.0 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "lsr_bullish")
		if snap.LSR > snap.LSRPrev {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "lsr_shifting")
		}
	} else if snap.LSR > snap.LSROldest {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "lsr_recovering")
	}

	// 5. Position Clearance (0-10): OI has room to rebuild (squeeze still has legs)
	if snap.OIDelta4h < -5 && snap.OIDelta4h > -40 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_clearing")
	} else if snap.OIDelta4h < 0 {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "oi_declining")
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	// --- Post-scoring penalties ---

	// Already pumped too much in 24h — squeeze may be exhausted
	if ctx.Change24h > 25 {
		sig.SetupScore *= 0.6
		sig.RiskTags = append(sig.RiskTags, "already_pumped_24h")
	}

	// LSR too skewed long — everyone already long, squeeze risk reversed
	if snap.LSR > 2.2 {
		sig.SetupScore *= 0.5
		sig.RiskTags = append(sig.RiskTags, "lsr_extreme_long")
	}

	// Funding too positive — expensive to stay long
	if snap.FundingRate > 0.0005 { // 0.05%
		sig.SetupScore *= 0.5
		sig.RiskTags = append(sig.RiskTags, "funding_expensive")
	}

	// Re-clamp after penalties
	sig.SetupScore = clampFloat(sig.SetupScore, 0, 100)

	if sig.SetupScore < 30 {
		return nil
	}

	// Build price context
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: tight, must confirm fast
	if ctx.ATR15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.CurrentPrice - ctx.ATR15m*0.3,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.5,
		}
	}

	// Invalidation: below the 1h low — if we retrace that far, squeeze failed
	if ctx.Low1h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.Low1h - ctx.ATR15m*0.3,
			Reason: "break_1h_low_squeeze_failed",
		}
	}

	// Targets: continuation + extension
	if ctx.ATR1h > 0 {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR1h*1.5, Reason: "squeeze_continuation"})
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR4h*1.0, Reason: "squeeze_extension"})
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
}
