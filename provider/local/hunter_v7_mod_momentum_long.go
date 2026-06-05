package local

// ============================================================================
// Module D: Leader Momentum Long
// ============================================================================
// Catches strong trending leaders that are pulling back slightly but still
// showing dominant momentum:
//   - 24h gain 12%~60% (strong trend, not yet exhausted)
//   - 4h gain > 6% (trend continuing)
//   - Exclude blow-off tops (24h >80% AND OI >80%)
//   - Exclude taker divergence (price new high but taker declining)
// Entry uses trailing stop logic — ride the momentum with a tight leash.

type leaderMomentumLongModule struct{}

func (m *leaderMomentumLongModule) Name() string           { return "leader_momentum_long" }
func (m *leaderMomentumLongModule) SetupType() V7SetupType { return V7SetupLeaderMomentumLong }
func (m *leaderMomentumLongModule) Direction() V7Direction { return V7DirLong }

func (m *leaderMomentumLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx.CurrentPrice <= 0 {
		return false
	}
	// 24h gain between 12% and 60% — strong but not blow-off
	if ctx.Change24h < 12 || ctx.Change24h > 60 {
		return false
	}
	// 4h gain > 6% — trend still running
	if ctx.Change4h <= 6 {
		return false
	}
	// Exclude blow-off top: 24h >80% AND OI explosion >80%
	snap := ctx.Snapshot
	if snap != nil {
		if ctx.Change24h > 80 && snap.OIDelta4h > 80 {
			return false
		}
		// Exclude taker divergence: price near 24h high but taker buy declining
		if snap.HighPrice24h > 0 && ctx.CurrentPrice >= snap.HighPrice24h*0.99 && ctx.TakerBuy15m < 0.48 {
			return false
		}
	}
	return true
}

func (m *leaderMomentumLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	sig := &V7SignalOutput{
		Symbol:       ctx.Symbol,
		Direction:    V7DirLong,
		SetupType:    V7SetupLeaderMomentumLong,
		Status:       V7StatusCandidate,
		EntryMode:    V7EntryMomentumTrailing,
		Confidence:   "A",
		MarketRegime: regime,
	}

	snap := ctx.Snapshot
	var score float64

	// 1. Momentum Strength (0-25): absolute move magnitude
	//    Sweet spot: 24h 15-40%, 4h 8-20%
	if ctx.Change24h >= 15 && ctx.Change24h <= 40 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_24h_momentum")
	} else if ctx.Change24h >= 12 && ctx.Change24h <= 50 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "solid_24h_momentum")
	} else {
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_24h_momentum")
	}

	if ctx.Change4h >= 8 && ctx.Change4h <= 20 {
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "strong_4h_momentum")
	} else if ctx.Change4h > 6 {
		score += 7
		sig.ReasonCodes = append(sig.ReasonCodes, "solid_4h_momentum")
	}

	// 2. Relative Strength (0-25): how well it holds vs 1h movements
	//    Positive 1h change = still pushing, not yet pulling back
	if ctx.Change1h > 2 {
		score += 20
		sig.ReasonCodes = append(sig.ReasonCodes, "accelerating_1h")
	} else if ctx.Change1h > 0.5 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "holding_1h")
	} else if ctx.Change1h > -1 {
		// Slight pullback — healthy, not a reversal
		score += 10
		sig.ReasonCodes = append(sig.ReasonCodes, "shallow_pullback_1h")
	} else {
		// Deeper pullback but still qualifies
		score += 5
		sig.ReasonCodes = append(sig.ReasonCodes, "moderate_pullback_1h")
	}

	// 3. OI Health (0-20): OI growth aligning with price = genuine demand
	if snap != nil {
		// OI increasing alongside price = new longs entering, sustainable trend
		if snap.OIDelta4h > 5 && snap.OIDelta4h < 60 {
			score += 20
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_healthy_growth")
		} else if snap.OIDelta4h > 0 && snap.OIDelta4h < 80 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_moderate_growth")
		} else if snap.OIDelta4h >= 0 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_stable")
		} else {
			// OI declining while price up = squeeze, less sustainable
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "oi_declining_squeeze")
		}
	}

	// 4. Taker Sustain (0-15): buy-side aggression sustaining the move
	if ctx.TakerBuy15m > 0.60 {
		score += 15
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_sustained_buy")
	} else if ctx.TakerBuy15m > 0.55 {
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_strong_buy")
	} else if ctx.TakerBuy15m > 0.50 {
		score += 8
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_neutral_buy")
	} else {
		score += 3
		sig.ReasonCodes = append(sig.ReasonCodes, "taker_weak_buy")
	}

	// 5. Pullback Shallow (0-15): if pulling back, it should be contained
	if ctx.ATR1h > 0 && ctx.Change1h < 0 {
		pullbackATR := (-ctx.Change1h * ctx.CurrentPrice / 100) / ctx.ATR1h
		if pullbackATR < 0.5 {
			score += 15
			sig.ReasonCodes = append(sig.ReasonCodes, "micro_pullback")
		} else if pullbackATR < 1.0 {
			score += 10
			sig.ReasonCodes = append(sig.ReasonCodes, "shallow_pullback")
		} else if pullbackATR < 1.5 {
			score += 5
			sig.ReasonCodes = append(sig.ReasonCodes, "moderate_pullback")
		}
	} else {
		// Still pushing up — no pullback needed
		score += 12
		sig.ReasonCodes = append(sig.ReasonCodes, "no_pullback_still_running")
	}

	sig.SetupScore = clampFloat(score, 0, 100)

	if sig.SetupScore < 30 {
		return nil
	}

	// Build price context
	sig.PriceCtx = buildPriceCtx(ctx)
	sig.DerivativesCtx = buildDerivCtx(ctx)

	// Entry zone: momentum trailing — enter on shallow dips
	if ctx.ATR15m > 0 {
		sig.EntryZone = V7PriceZone{
			Lower: ctx.CurrentPrice - ctx.ATR15m*0.8,
			Upper: ctx.CurrentPrice + ctx.ATR15m*0.3,
		}
	}

	// Invalidation: break of structure — below recent 1h low or significant ATR move
	if ctx.Low1h > 0 && ctx.ATR1h > 0 {
		sig.Invalidation = V7InvalidationRule{
			Price:  ctx.Low1h - ctx.ATR1h*0.3,
			Reason: "break_structure_momentum_lost",
		}
	}

	// Targets: ride the momentum — 1h ATR multiples
	if ctx.ATR1h > 0 {
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR1h*1.5, Reason: "momentum_target_1"})
		sig.Targets = append(sig.Targets, V7Target{Price: ctx.CurrentPrice + ctx.ATR4h*1.0, Reason: "momentum_target_2"})
	}

	// Timing score
	sig.TimingScore = calcTimingScore(sig, ctx)

	return sig
}
