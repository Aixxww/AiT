package local

// ============================================================================
// Module: Relative Weakness Short
// ============================================================================
// Shorts the weakest names in a tape that is not falling: symbols bleeding
// steadily against the market with persistent sell flow — no panic candle, no
// 20%+ rally to distribute, no ladder structure, which is exactly the gap the
// 2026-07-27 missed-opportunity audit exposed (24 unmatched high-potential
// pool entries; the deepest clean movers — SCR -8.8%, PRL -6.6%, EUL -4.9%,
// PIEVERSE -4.0%, LPT -3.0% forward MFE as shorts — were all steady bleeders
// no existing short module's Match accepted).
//
// Doctrine: grind-down continuation, not knife-catching — capitulation-speed
// drops are left to panic/breakdown modules, rallies to distribution.

type relativeWeaknessShortModule struct{}

func (m *relativeWeaknessShortModule) Name() string           { return "relative_weakness_short" }
func (m *relativeWeaknessShortModule) SetupType() V7SetupType { return V7SetupRelativeWeaknessShort }
func (m *relativeWeaknessShortModule) Direction() V7Direction { return V7DirShort }

func (m *relativeWeaknessShortModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx.CurrentPrice <= 0 || ctx.ATR15m <= 0 || ctx.Symbol == "BTCUSDT" || ctx.Symbol == "ETHUSDT" {
		return false
	}
	// Persistent bleed: down on 4h and 24h, still leaking on 1h.
	if ctx.Change4h > -3 || ctx.Change24h > -2 || ctx.Change1h > 0.5 {
		return false
	}
	// Grind, not capitulation: fast crashes belong to panic/breakdown modules.
	if ctx.Change1h < -5 {
		return false
	}
	// Sell flow must dominate; missing taker data does not qualify.
	if ctx.TakerBuy15m <= 0 || ctx.TakerBuy15m > 0.48 {
		return false
	}
	return true
}

func (m *relativeWeaknessShortModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}
	snap := ctx.Snapshot

	s := newV7Signal(ctx, regime, V7SetupRelativeWeaknessShort, V7DirShort, V7EntryWaitReject, "C")

	// 1. Weakness depth (0-30): how hard it bleeds across frames.
	if ctx.Change4h < -8 {
		s.add(18, "rs_deep_4h_bleed")
	} else if ctx.Change4h < -5 {
		s.add(12, "rs_solid_4h_bleed")
	} else {
		s.add(6, "rs_mild_4h_bleed")
	}
	if ctx.Change24h < -8 {
		s.add(12, "rs_deep_24h_bleed")
	} else if ctx.Change24h < -4 {
		s.add(8, "rs_solid_24h_bleed")
	}

	// 2. Persistence (0-20): all frames aligned down, still leaking now.
	if ctx.Change1h < -1 && ctx.Change4h < -3 && ctx.Change24h < -2 {
		s.add(12, "rs_all_frames_down")
	}
	if ctx.Velocity15m < -0.3 {
		s.add(8, "rs_still_leaking_15m")
	}

	// 3. Sell flow (0-20): the unified taker ladder, short side.
	if ctx.TakerBuy15m <= 0.42 {
		s.add(20, "flow_taker_sell_dominant")
	} else if ctx.TakerBuy15m <= 0.45 {
		s.add(14, "flow_taker_sell_strong")
	} else {
		s.add(8, "flow_taker_sell_emerging")
	}

	// 4. Structure (0-20): below VWAP and below the 4h mean.
	if ctx.VWAP15m > 0 && ctx.CurrentPrice < ctx.VWAP15m {
		s.add(10, "rs_below_vwap")
	}
	if ctx.EMA20_4h > 0 && ctx.CurrentPrice < ctx.EMA20_4h {
		s.add(10, "rs_below_ema20_4h")
	}

	// 5. Derivatives confirmation (0-10): shorts building or longs trapped.
	if snap != nil {
		if snap.OIDelta1h > 1.5 {
			s.add(10, "rs_new_shorts_building")
		} else if snap.OIDelta1h > -1 {
			s.add(5, "rs_oi_stable_decline")
		}
		if snap.FundingRate < -0.0003 {
			s.riskTag("rs_crowded_short_funding")
		}
	}

	// Entry on a weak bounce into the zone; invalidate above the 1h high
	// (reclaim kills the grind thesis); target the ATR extension lower.
	s.zonePad(0.35, 0.5)
	if ctx.High1h > 0 {
		s.invalidate(ctx.High1h+ctx.ATR15m*0.5, "reclaim_above_1h_high")
	} else {
		s.invalidate(ctx.CurrentPrice+ctx.ATR15m*1.2, "reclaim_above_grind_channel")
	}
	if ctx.Low1h > 0 && ctx.Low1h < ctx.CurrentPrice {
		s.target(ctx.Low1h, "retest_1h_low")
	}
	s.target(ctx.CurrentPrice-ctx.ATR4h*1.5, "atr_extension_lower")

	s.sig.RequiredConfirms = []string{
		"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
		"taker_buy_15m_lt_0_48",
	}
	return s.finish(40)
}
