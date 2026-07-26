package local

import "math"

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

	s := newV7Signal(ctx, regime, V7SetupLeaderMomentumLong, V7DirLong, V7EntryMomentumTrailing, "A")
	snap := ctx.Snapshot

	// 1. Momentum Strength (0-25): absolute move magnitude.
	//    Sweet spot: 24h 15-40%, 4h 8-20%
	if ctx.Change24h >= 15 && ctx.Change24h <= 40 {
		s.add(15, "strong_24h_momentum")
	} else if ctx.Change24h >= 12 && ctx.Change24h <= 50 {
		s.add(10, "solid_24h_momentum")
	} else {
		s.add(5, "moderate_24h_momentum")
	}

	if ctx.Change4h >= 8 && ctx.Change4h <= 20 {
		s.add(10, "strong_4h_momentum")
	} else if ctx.Change4h > 6 {
		s.add(7, "solid_4h_momentum")
	}

	// 2. Relative Strength (0-25): how well it holds vs 1h movements.
	//    Positive 1h change = still pushing, not yet pulling back
	if ctx.Change1h > 2 {
		s.add(20, "accelerating_1h")
	} else if ctx.Change1h > 0.5 {
		s.add(15, "holding_1h")
	} else if ctx.Change1h > -1 {
		// Slight pullback — healthy, not a reversal
		s.add(10, "shallow_pullback_1h")
	} else {
		// Deeper pullback but still qualifies
		s.add(5, "moderate_pullback_1h")
	}

	// 3. OI Health (0-20): OI growth aligning with price = genuine demand
	if snap != nil {
		// OI increasing alongside price = new longs entering, sustainable trend
		if snap.OIDelta4h > 5 && snap.OIDelta4h < 60 {
			s.add(20, "oi_healthy_growth")
		} else if snap.OIDelta4h > 0 && snap.OIDelta4h < 80 {
			s.add(15, "oi_moderate_growth")
		} else if snap.OIDelta4h >= 0 {
			s.add(10, "oi_stable")
		} else {
			// OI declining while price up = squeeze, less sustainable
			s.add(5, "oi_declining_squeeze")
		}
	}

	// 4. Taker Sustain (0-15): buy-side aggression sustaining the move
	s.takerLadder(v7TakerLadders[V7SetupLeaderMomentumLong])

	// 5. Pullback Shallow (0-15): if pulling back, it should be contained
	if ctx.ATR1h > 0 && ctx.Change1h < 0 {
		pullbackATR := (-ctx.Change1h * ctx.CurrentPrice / 100) / ctx.ATR1h
		if pullbackATR < 0.5 {
			s.add(15, "micro_pullback")
		} else if pullbackATR < 1.0 {
			s.add(10, "shallow_pullback")
		} else if pullbackATR < 1.5 {
			s.add(5, "moderate_pullback")
		}
	} else {
		// Still pushing up — no pullback needed
		s.add(12, "no_pullback_still_running")
	}

	// Entry on shallow dips; invalidate on 1h structure break; targets ride
	// the momentum in ATR multiples.
	s.zoneATR(0.8, 0.3)
	if ctx.Low1h > 0 && ctx.ATR1h > 0 {
		s.invalidate(ctx.Low1h-ctx.ATR1h*0.3, "break_structure_momentum_lost")
	}
	if ctx.ATR1h > 0 {
		s.target(ctx.CurrentPrice+ctx.ATR1h*1.5, "momentum_target_1")
		s.target(ctx.CurrentPrice+ctx.ATR4h*1.0, "momentum_target_2")
	}

	// Timing score: leader momentum has its own confirmation codes, so the
	// generic pullback/reversal timing model would otherwise leave it WATCH-only.
	return s.finishWithTiming(30, calcLeaderMomentumTimingScore(s.sig, ctx))
}

func calcLeaderMomentumTimingScore(sig *V7SignalOutput, ctx *V7SymbolContext) float64 {
	if sig == nil || ctx == nil {
		return 0
	}
	timing := 0.0

	if sig.EntryZone.Lower > 0 && ctx.CurrentPrice >= sig.EntryZone.Lower && ctx.CurrentPrice <= sig.EntryZone.Upper {
		timing += 25
	}

	if ctx.TakerBuy15m >= 0.60 {
		timing += 20
	} else if ctx.TakerBuy15m >= 0.55 {
		timing += 15
	} else if ctx.TakerBuy15m >= 0.52 {
		timing += 10
	}

	if ctx.Snapshot != nil {
		if ctx.Snapshot.OIDelta4h > 5 && ctx.Snapshot.OIDelta4h < 60 {
			timing += 15
		} else if ctx.Snapshot.OIDelta4h > 0 && ctx.Snapshot.OIDelta4h < 80 {
			timing += 10
		}
		if ctx.Snapshot.OIDelta1h > 0 && ctx.Snapshot.OIDelta1h < 35 {
			timing += 5
		}
	}

	switch {
	case ctx.Change1h > 0.5 && ctx.Change1h <= 4:
		timing += 15
	case ctx.Change1h > 0 && ctx.Change1h <= 5:
		timing += 10
	case ctx.Change1h >= -1 && ctx.Change1h <= 0:
		timing += 12
	case ctx.Change1h > 4 && ctx.TakerBuy15m >= 0.60:
		timing += 8
	}

	if hasLeaderMomentumReason(sig, "micro_pullback") || hasLeaderMomentumReason(sig, "shallow_pullback") {
		timing += 12
	} else if hasLeaderMomentumReason(sig, "no_pullback_still_running") && ctx.Change1h <= 4 {
		timing += 8
	}

	if rr, ok := v7SignalRiskReward(sig, ctx.CurrentPrice); ok {
		if rr >= 1.8 {
			timing += 18
		} else if rr >= 1.5 {
			timing += 15
		} else if rr >= 1.2 {
			timing += 8
		}
	}

	if ctx.Change1h > 6 && ctx.TakerBuy15m < 0.60 {
		timing = math.Min(timing, 55)
	}
	if ctx.Change24h > 45 && ctx.Snapshot != nil && ctx.Snapshot.OIDelta4h > 60 {
		timing = math.Min(timing, 55)
	}

	return clampFloat(timing, 0, 100)
}

func hasLeaderMomentumReason(sig *V7SignalOutput, reason string) bool {
	if sig == nil {
		return false
	}
	for _, code := range sig.ReasonCodes {
		if code == reason {
			return true
		}
	}
	return false
}
