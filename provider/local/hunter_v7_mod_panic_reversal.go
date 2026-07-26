package local

// ============================================================================
// Module E: Panic V-Reversal Long
// ============================================================================
// Catches capitulation-driven V-shaped reversals after sharp selloffs.
// This module looks for:
//   - 24h drop between -15% and -45% (deep capitulation)
//   - OI heavily flushed (leveraged longs wiped out, cleansing washout)
//   - Price showing early reclaim signals (bounce off the lows)
//   - Taker buy recovering (smart money stepping into the wreckage)
//   - Volume exhaustion (selling pressure subsiding)

type panicReversalLongModule struct{}

func (m *panicReversalLongModule) Name() string           { return "panic_reversal_long" }
func (m *panicReversalLongModule) SetupType() V7SetupType { return V7SetupPanicReversalLong }
func (m *panicReversalLongModule) Direction() V7Direction { return V7DirLong }

func (m *panicReversalLongModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	// Must have price data
	if ctx.CurrentPrice <= 0 || ctx.ATR4h <= 0 {
		return false
	}
	// 24h drop between -15% and -45%
	if ctx.Change24h > -15 || ctx.Change24h < -45 {
		return false
	}
	// Forbid: 24h drop > 50% (extreme crash, too dangerous)
	if ctx.Change24h < -50 {
		return false
	}
	// OI must have dropped (leveraged positions flushed out)
	snap := ctx.Snapshot
	if snap != nil {
		if snap.OIDelta4h >= 0 {
			return false
		}
	}
	return true
}

func (m *panicReversalLongModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	snap := ctx.Snapshot

	s := newV7Signal(ctx, regime, V7SetupPanicReversalLong, V7DirLong, V7EntryWaitReclaim, "B")

	// 1. Capitulation Depth (0-20): deeper = more cleansed.
	drop := ctx.Change24h // negative value
	if drop <= -30 && drop >= -45 {
		s.add(20, "deep_capitulation")
	} else if drop <= -22 && drop > -30 {
		s.add(16, "heavy_capitulation")
	} else if drop <= -15 && drop > -22 {
		s.add(12, "moderate_capitulation")
	}

	// 2. OI Flush (0-20): massive leverage cleanup.
	if snap != nil {
		oiDrop := snap.OIDelta4h // should be negative
		if oiDrop <= -30 {
			s.add(20, "oi_massive_flush")
		} else if oiDrop <= -20 {
			s.add(16, "oi_heavy_flush")
		} else if oiDrop <= -12 {
			s.add(12, "oi_flush")
		} else if oiDrop < 0 {
			s.add(6, "oi_declining")
		}
		// Forbid: OI increasing while price drops (new shorts piling in, not cleansing)
		if oiDrop > 0 && ctx.Change24h < -15 {
			s.riskTag("oi_up_price_down")
			s.score -= 10
		}
	}

	// 3. Reclaim Strength (0-25): price bouncing from the lows.
	if ctx.ATR4h > 0 && ctx.Low4h > 0 {
		distFromLow := (ctx.CurrentPrice - ctx.Low4h) / ctx.ATR4h
		if distFromLow >= 1.5 {
			s.add(25, "strong_reclaim")
		} else if distFromLow >= 1.0 {
			s.add(20, "solid_reclaim")
		} else if distFromLow >= 0.5 {
			s.add(15, "early_reclaim")
		} else if distFromLow > 0.2 {
			s.add(8, "weak_bounce")
		} else {
			// Forbid: price at the absolute low with no reclaim
			s.riskTag("no_reclaim_signal")
			s.score -= 5
		}
	}

	// 4. Taker Recovery (0-15): smart money stepping in.
	s.takerLadder(v7TakerLadders[V7SetupPanicReversalLong])

	// 5. Volume Exhaustion (0-10): selling pressure subsiding.
	if snap != nil {
		// If 1h OI delta is less negative than 4h, selling is decelerating
		if snap.OIDelta1h > snap.OIDelta4h/4 && snap.OIDelta4h < -10 {
			s.add(10, "selling_exhaustion")
		} else if snap.OIDelta1h > snap.OIDelta4h/3 {
			s.add(6, "selling_decelerating")
		}
	}

	// 6. Market Stabilization (0-10): broader market showing signs of life.
	if ctx.Change1h > 0 && ctx.Change1h < 5 {
		s.add(6, "1h_green_shoot")
	}
	if ctx.RSI1h > 0 && ctx.RSI1h > 25 && ctx.RSI1h < 40 {
		s.add(4, "rsi_recovering_from_extreme")
	}

	// Entry on the bounce; invalidate below the capitulation low; targets are
	// the V-reversal reclaim magnets.
	s.zoneATR(0.3, 0.8)
	if ctx.Low4h > 0 {
		s.invalidate(ctx.Low4h-ctx.ATR4h*0.2, "break_capitulation_low")
	}
	if ctx.VWAP15m > ctx.CurrentPrice {
		s.target(ctx.VWAP15m, "vwap_reclaim")
	}
	if ctx.ATR4h > 0 {
		s.target(ctx.CurrentPrice+ctx.ATR4h*2.5, "atr_v_reversal_target")
	}
	if ctx.EMA20_4h > ctx.CurrentPrice {
		s.target(ctx.EMA20_4h, "ema20_reclaim")
	}

	return s.finish(30)
}
