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

	snap := ctx.Snapshot

	s := newV7Signal(ctx, regime, V7SetupShortSqueezeLong, V7DirLong, V7EntryFastConfirm, "A")

	// 1. Squeeze Strength (0-30): how severe is the forced covering?
	//    Price up + OI down = classic squeeze. Magnitude matters.
	priceSurge := ctx.Change1h
	oiDrop := -snap.OIDelta1h // positive number = OI decline magnitude

	if priceSurge >= 8 && oiDrop >= 8 {
		s.add(30, "extreme_squeeze")
	} else if priceSurge >= 5 && oiDrop >= 5 {
		s.add(25, "strong_squeeze")
	} else if priceSurge >= 3 && oiDrop >= 3 {
		s.add(20, "moderate_squeeze")
	} else {
		s.add(10, "mild_squeeze")
	}

	// 2. Taker Momentum (0-25): aggressive buying driving the squeeze.
	s.takerLadder(v7TakerLadders[V7SetupShortSqueezeLong])

	// 3. Volume Expansion (0-20): 24h quote volume as proxy.
	if snap.QuoteVolume24h > 0 {
		// Use absolute volume as a rough proxy; higher = more participation
		if snap.QuoteVolume24h > 5e8 { // > $500M
			s.add(20, "volume_massive")
		} else if snap.QuoteVolume24h > 1e8 { // > $100M
			s.add(15, "volume_high")
		} else if snap.QuoteVolume24h > 2e7 { // > $20M
			s.add(10, "volume_moderate")
		}
	}

	// 4. LSR Shift (0-15): long/short ratio turning bullish during squeeze.
	if snap.LSR > 1.0 {
		s.add(10, "lsr_bullish")
		if snap.LSR > snap.LSRPrev {
			s.add(5, "lsr_shifting")
		}
	} else if snap.LSR > snap.LSROldest {
		s.add(8, "lsr_recovering")
	}

	// 5. Position Clearance (0-10): OI has room to rebuild (squeeze still has legs).
	if snap.OIDelta4h < -5 && snap.OIDelta4h > -40 {
		s.add(10, "oi_clearing")
	} else if snap.OIDelta4h < 0 {
		s.add(5, "oi_declining")
	}

	// --- Post-scoring penalties (clamp first, multiply, finish re-clamps) ---

	s.score = clampFloat(s.score, 0, 100)

	// Already pumped too much in 24h — squeeze may be exhausted
	if ctx.Change24h > 25 {
		s.score *= 0.6
		s.riskTag("already_pumped_24h")
	}

	// LSR too skewed long — everyone already long, squeeze risk reversed
	if snap.LSR > 2.2 {
		s.score *= 0.5
		s.riskTag("lsr_extreme_long")
	}

	// Funding too positive — expensive to stay long
	if snap.FundingRate > 0.0005 { // 0.05%
		s.score *= 0.5
		s.riskTag("funding_expensive")
	}

	// Entry tight and fast; invalidate below the 1h low — if we retrace that
	// far, the squeeze failed; targets ride the continuation.
	s.zoneATR(0.3, 0.5)
	if ctx.Low1h > 0 {
		s.invalidate(ctx.Low1h-ctx.ATR15m*0.3, "break_1h_low_squeeze_failed")
	}
	if ctx.ATR1h > 0 {
		s.target(ctx.CurrentPrice+ctx.ATR1h*1.5, "squeeze_continuation")
		s.target(ctx.CurrentPrice+ctx.ATR4h*1.0, "squeeze_extension")
	}

	return s.finish(30)
}
