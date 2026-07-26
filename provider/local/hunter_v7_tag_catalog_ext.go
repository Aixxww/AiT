package local

// ============================================================================
// Hunter v7 — Tag catalog reconciliation (U1.8, lean-core redesign)
// ============================================================================
// The 2026-07-26 audit found 186 tags emitted by live modules that were absent
// from the catalog: 47% of the vocabulary the LLM actually sees degraded to
// "unknown_context_only", stripping the decision layer of the semantics it
// needs to judge entry signals. This file registers every missing tag, grouped
// by domain. TestHunterV7TagCatalogCoversEmittedTags enforces that the emitted
// set stays a subset of the catalog from now on.
//
// llm_action assignment policy:
//   - descriptive market state       -> evidence_only
//   - directly supports the entry    -> supports_open_after_core_checks
//   - warns against the direction    -> reduce_size_or_wait / wait_only
//   - explicit wait/watch semantics  -> wait_only / context_only

var hunterV7TagCatalogExt = map[string]HunterV7TagDefinition{
	// ---- LSR (long/short ratio) ----
	"lsr_bullish":              tagDef("lsr_bullish", "reason_code", "funding", "bullish", V7TagActionEvidence, "Long/short ratio leans long without reaching crowded extremes."),
	"lsr_bullish_crowded":      tagDef("lsr_bullish_crowded", "reason_code", "funding", "mixed", V7TagActionEvidence, "Long/short ratio is bullish but approaching crowded territory; squeeze fuel for shorts, chase risk for longs."),
	"lsr_improving":            tagDef("lsr_improving", "reason_code", "funding", "bullish", V7TagActionEvidence, "Long/short ratio is recovering from a short-heavy reading."),
	"lsr_neutral_accumulation": tagDef("lsr_neutral_accumulation", "reason_code", "funding", "bullish", V7TagActionEvidence, "LSR stays neutral while price accumulates — positioning has not chased yet."),
	"lsr_recovering":           tagDef("lsr_recovering", "reason_code", "funding", "bullish", V7TagActionEvidence, "LSR turning up from its recent low, early repositioning."),
	"lsr_reversal":             tagDef("lsr_reversal", "reason_code", "funding", "mixed", V7TagActionEvidence, "LSR direction flipped versus the prior readings; positioning regime change."),
	"lsr_shifting":             tagDef("lsr_shifting", "reason_code", "funding", "mixed", V7TagActionEvidence, "LSR drifting between regimes; positioning signal not yet decisive."),
	"lsr_short_crowded":        tagDef("lsr_short_crowded", "reason_code", "funding", "bullish", V7TagActionEvidence, "Shorts are crowded by LSR; squeeze fuel if price holds."),
	"lsr_turning_up":           tagDef("lsr_turning_up", "reason_code", "funding", "bullish", V7TagActionEvidence, "LSR inflecting upward, supportive for reversal longs."),

	// ---- Open interest ----
	"oi_accumulation":             tagDef("oi_accumulation", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Open interest is being accumulated alongside the setup direction."),
	"oi_anomaly":                  tagDef("oi_anomaly", "reason_code", "oi", "mixed", V7TagActionEvidence, "OI moved abnormally versus its baseline; leverage event underway."),
	"oi_building":                 tagDef("oi_building", "reason_code", "oi", "mixed", V7TagActionEvidence, "OI is building; direction of the build decides whether it is fuel or crowding."),
	"oi_clearing":                 tagDef("oi_clearing", "reason_code", "oi", "mixed", V7TagActionEvidence, "OI unwinding; prior positioning being flushed out."),
	"oi_consistent_inflow":        tagDef("oi_consistent_inflow", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Multi-window OI inflow confirms sustained participation."),
	"oi_declining_long_flush":     tagDef("oi_declining_long_flush", "reason_code", "oi", "bearish", V7TagActionEvidence, "OI decline consistent with longs being flushed — supports crowding-reversal shorts."),
	"oi_declining_short_flush":    tagDef("oi_declining_short_flush", "reason_code", "oi", "bullish", V7TagActionEvidence, "OI decline consistent with shorts covering — supports squeeze longs."),
	"oi_declining_squeeze":        tagDef("oi_declining_squeeze", "reason_code", "oi", "mixed", V7TagActionEvidence, "OI declining while price squeezes; positions closing into the move."),
	"oi_elevated_still_building":  tagDef("oi_elevated_still_building", "reason_code", "oi", "mixed", V7TagActionEvidence, "OI already elevated and still growing; crowded but not yet unwinding."),
	"oi_expanding":                tagDef("oi_expanding", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "OI expanding with the move; fresh money participating."),
	"oi_flush":                    tagDef("oi_flush", "reason_code", "oi", "mixed", V7TagActionEvidence, "A leverage flush occurred; positioning reset."),
	"oi_flushing":                 tagDef("oi_flushing", "reason_code", "oi", "mixed", V7TagActionEvidence, "Flush in progress; wait for it to complete before counter-entries."),
	"oi_mild":                     tagDef("oi_mild", "reason_code", "oi", "neutral", V7TagActionEvidence, "OI change is mild; derivatives positioning is not the driver."),
	"oi_mild_buildup":             tagDef("oi_mild_buildup", "reason_code", "oi", "bullish", V7TagActionEvidence, "Gentle OI buildup; early-stage participation."),
	"oi_moderate_growth":          tagDef("oi_moderate_growth", "reason_code", "oi", "bullish", V7TagActionEvidence, "Moderate OI growth supporting the move without crowding."),
	"oi_neutral":                  tagDef("oi_neutral", "reason_code", "oi", "neutral", V7TagActionEvidence, "OI flat; no derivatives thrust behind the move."),
	"oi_stabilize":                tagDef("oi_stabilize", "reason_code", "oi", "neutral", V7TagActionEvidence, "OI stabilized after a swing; positioning settled."),
	"oi_stable":                   tagDef("oi_stable", "reason_code", "oi", "neutral", V7TagActionEvidence, "OI steady through the move; holders are not exiting."),
	"oi_stable_breakout":          tagDef("oi_stable_breakout", "reason_code", "oi", "bullish", V7TagActionEvidence, "Breakout with stable OI; spot-led move rather than leverage churn."),
	"compressed_oi_pre_breakout":  tagDef("compressed_oi_pre_breakout", "reason_code", "oi", "bullish", V7TagActionEvidence, "OI compressed during consolidation; energy stored for a breakout."),
	"continuous_oi_crowding":      tagDef("continuous_oi_crowding", "reason_code", "oi", "mixed", V7TagActionEvidence, "OI kept crowding without a flush; unwind risk accumulating."),
	"extreme_oi_surge":            tagDef("extreme_oi_surge", "reason_code", "oi", "mixed", V7TagActionEvidence, "OI surged to an extreme in a short window; violent two-way risk."),
	"heavy_oi_surge":              tagDef("heavy_oi_surge", "reason_code", "oi", "mixed", V7TagActionEvidence, "Heavy OI surge; leverage flooding in."),
	"mild_oi_accumulation":        tagDef("mild_oi_accumulation", "reason_code", "oi", "bullish", V7TagActionEvidence, "Slight OI accumulation; early interest."),
	"steady_oi_accumulation":      tagDef("steady_oi_accumulation", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Steady multi-hour OI accumulation; committed positioning."),
	"strong_oi_accumulation":      tagDef("strong_oi_accumulation", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Strong OI accumulation aligned with the setup."),
	"late_short_without_oi_flush": tagDef("late_short_without_oi_flush", "risk_tag", "oi", "bearish", V7TagActionWaitOnly, "Short is late and the crowded OI has not flushed; wait for the flush or a retest."),

	// ---- Taker flow ----
	"taker_buy_bias_before_breakout":    tagDef("taker_buy_bias_before_breakout", "reason_code", "flow", "bullish", V7TagActionEvidence, "Buy-side taker bias building before the breakout level."),
	"taker_buy_biased":                  tagDef("taker_buy_biased", "reason_code", "flow", "bullish", V7TagActionEvidence, "Taker flow leans buy-side."),
	"taker_buy_gt_0_52":                 tagDef("taker_buy_gt_0_52", "required_confirmation", "flow", "bullish", V7TagActionRequiredConfirm, "15m taker buy ratio must exceed 0.52. Alias of taker_buy_15m_gt_0_52."),
	"taker_buy_improving":               tagDef("taker_buy_improving", "reason_code", "flow", "bullish", V7TagActionEvidence, "Taker buy ratio improving across recent windows."),
	"taker_buy_recovering":              tagDef("taker_buy_recovering", "reason_code", "flow", "bullish", V7TagActionEvidence, "Taker buy ratio recovering from a sell-dominant stretch."),
	"taker_buy_recovery_before_squeeze": tagDef("taker_buy_recovery_before_squeeze", "reason_code", "flow", "bullish", V7TagActionEvidence, "Buy flow returning while shorts remain crowded; squeeze precondition."),
	"taker_buy_weakening":               tagDef("taker_buy_weakening", "reason_code", "flow", "bearish", V7TagActionEvidence, "Buy-side taker share fading; continuation support weakening."),
	"taker_buying_emerging":             tagDef("taker_buying_emerging", "reason_code", "flow", "bullish", V7TagActionEvidence, "Fresh taker buying emerging after weakness."),
	"taker_moderate_buy":                tagDef("taker_moderate_buy", "reason_code", "flow", "bullish", V7TagActionEvidence, "Moderate taker buy dominance."),
	"taker_neutral":                     tagDef("taker_neutral", "reason_code", "flow", "neutral", V7TagActionEvidence, "Taker flow balanced; no directional edge from flow."),
	"taker_neutral_bullish":             tagDef("taker_neutral_bullish", "reason_code", "flow", "bullish", V7TagActionEvidence, "Taker flow neutral with a slight buy tilt."),
	"taker_sell_dominant":               tagDef("taker_sell_dominant", "reason_code", "flow", "bearish", V7TagActionEvidence, "Sell-side takers dominate; headwind for longs, support for shorts."),
	"taker_sell_emerging":               tagDef("taker_sell_emerging", "reason_code", "flow", "bearish", V7TagActionEvidence, "Sell pressure starting to appear."),
	"taker_sell_mild":                   tagDef("taker_sell_mild", "reason_code", "flow", "bearish", V7TagActionEvidence, "Mild sell-side taker bias."),
	"taker_sell_neutral":                tagDef("taker_sell_neutral", "reason_code", "flow", "neutral", V7TagActionEvidence, "Sell pressure fading toward balance."),
	"taker_sell_strong":                 tagDef("taker_sell_strong", "reason_code", "flow", "bearish", V7TagActionEvidence, "Strong sell-side taker dominance."),
	"taker_selling":                     tagDef("taker_selling", "reason_code", "flow", "bearish", V7TagActionEvidence, "Net taker selling in the recent window."),
	"taker_selling_emerging":            tagDef("taker_selling_emerging", "reason_code", "flow", "bearish", V7TagActionEvidence, "Taker selling beginning to build."),
	"heavy_taker_selling":               tagDef("heavy_taker_selling", "reason_code", "flow", "bearish", V7TagActionEvidence, "Heavy taker selling; capitulation-grade flow."),
	"mild_taker_selling":                tagDef("mild_taker_selling", "reason_code", "flow", "bearish", V7TagActionEvidence, "Mild taker selling pressure."),
	"strong_taker_buy_reversal":         tagDef("strong_taker_buy_reversal", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Taker flow flipped decisively to buying at the reversal point."),
	"strong_taker_sell_reversal":        tagDef("strong_taker_sell_reversal", "reason_code", "flow", "bearish", V7TagActionOpenSupport, "Taker flow flipped decisively to selling at the reversal point; supports crowding shorts."),
	"sell_pressure_stalling":            tagDef("sell_pressure_stalling", "reason_code", "flow", "bullish", V7TagActionEvidence, "Selling losing intensity; precondition for reversal longs."),

	// ---- RSI ----
	"rsi5m_overbought":            tagDef("rsi5m_overbought", "reason_code", "price", "bearish", V7TagActionEvidence, "5m RSI overbought; short-term chase risk for longs."),
	"rsi_approaching_overbought":  tagDef("rsi_approaching_overbought", "reason_code", "price", "mixed", V7TagActionEvidence, "RSI nearing overbought; trend strong but extension building."),
	"rsi_approaching_oversold":    tagDef("rsi_approaching_oversold", "reason_code", "price", "mixed", V7TagActionEvidence, "RSI nearing oversold; downside momentum mature."),
	"rsi_deeply_overbought":       tagDef("rsi_deeply_overbought", "reason_code", "price", "bearish", V7TagActionEvidence, "RSI deeply overbought; do not market-chase longs."),
	"rsi_deeply_oversold":         tagDef("rsi_deeply_oversold", "reason_code", "price", "bullish", V7TagActionEvidence, "RSI deeply oversold; reversal-long precondition."),
	"rsi_overbought":              tagDef("rsi_overbought", "reason_code", "price", "bearish", V7TagActionEvidence, "RSI overbought."),
	"rsi_oversold":                tagDef("rsi_oversold", "reason_code", "price", "bullish", V7TagActionEvidence, "RSI oversold."),
	"rsi_recovering_from_extreme": tagDef("rsi_recovering_from_extreme", "reason_code", "price", "bullish", V7TagActionEvidence, "RSI turning up from an extreme low; momentum repair underway."),

	// ---- Volume ----
	"volume_adequate": tagDef("volume_adequate", "reason_code", "risk", "neutral", V7TagActionEvidence, "24h turnover adequate for normal position sizes."),
	"volume_decent":   tagDef("volume_decent", "reason_code", "risk", "neutral", V7TagActionEvidence, "Decent 24h turnover."),
	"volume_high":     tagDef("volume_high", "reason_code", "risk", "bullish", V7TagActionEvidence, "High 24h turnover; deep liquidity."),
	"volume_massive":  tagDef("volume_massive", "reason_code", "risk", "bullish", V7TagActionEvidence, "Very high 24h turnover; top-tier liquidity."),
	"volume_moderate": tagDef("volume_moderate", "reason_code", "risk", "neutral", V7TagActionEvidence, "Moderate 24h turnover; size positions accordingly."),
	"low_liquidity":   tagDef("low_liquidity", "risk_tag", "risk", "neutral", V7TagActionReduceOrWait, "Thin liquidity; slippage-sensitive, reduce size and prefer passive entries."),

	// ---- Range / structure ----
	"approaching_4h_support":       tagDef("approaching_4h_support", "reason_code", "price", "bullish", V7TagActionEvidence, "Price approaching 4h support."),
	"approaching_range_bottom":     tagDef("approaching_range_bottom", "reason_code", "price", "bullish", V7TagActionEvidence, "Price approaching the defined range bottom."),
	"approaching_range_top":        tagDef("approaching_range_top", "reason_code", "price", "bearish", V7TagActionEvidence, "Price approaching the defined range top."),
	"at_1d_high_breakout":          tagDef("at_1d_high_breakout", "reason_code", "price", "bullish", V7TagActionEvidence, "Price at/through the prior daily high."),
	"at_4h_high":                   tagDef("at_4h_high", "reason_code", "price", "bullish", V7TagActionEvidence, "Price at the 4h high."),
	"at_range_bottom":              tagDef("at_range_bottom", "reason_code", "price", "bullish", V7TagActionEvidence, "Price at the range bottom; reversion-long zone."),
	"at_range_top":                 tagDef("at_range_top", "reason_code", "price", "bearish", V7TagActionEvidence, "Price at the range top; reversion-short zone."),
	"breaks_1h_high":               tagDef("breaks_1h_high", "reason_code", "price", "bullish", V7TagActionEvidence, "Price broke the recent 1h high."),
	"breaks_4h_high":               tagDef("breaks_4h_high", "reason_code", "price", "bullish", V7TagActionEvidence, "Price broke the recent 4h high."),
	"defined_range":                tagDef("defined_range", "reason_code", "price", "neutral", V7TagActionEvidence, "A clean tradable range is defined."),
	"moderate_range":               tagDef("moderate_range", "reason_code", "price", "neutral", V7TagActionEvidence, "Range structure of moderate quality."),
	"strong_range":                 tagDef("strong_range", "reason_code", "price", "neutral", V7TagActionEvidence, "Well-tested range with multiple touches."),
	"weak_range":                   tagDef("weak_range", "reason_code", "price", "neutral", V7TagActionEvidence, "Range poorly defined; reversion edges unreliable."),
	"tight_range":                  tagDef("tight_range", "reason_code", "price", "neutral", V7TagActionEvidence, "Price compressed into a tight range."),
	"ultra_tight_range":            tagDef("ultra_tight_range", "reason_code", "price", "neutral", V7TagActionEvidence, "Extreme compression; expansion imminent."),
	"near_1d_support":              tagDef("near_1d_support", "reason_code", "price", "bullish", V7TagActionEvidence, "Price near daily support."),
	"near_4h_high":                 tagDef("near_4h_high", "reason_code", "price", "bullish", V7TagActionEvidence, "Price near the 4h high."),
	"near_4h_support":              tagDef("near_4h_support", "reason_code", "price", "bullish", V7TagActionEvidence, "Price near 4h support."),
	"near_bb_middle":               tagDef("near_bb_middle", "reason_code", "price", "neutral", V7TagActionEvidence, "Price near the Bollinger middle band."),
	"near_breakout_trigger":        tagDef("near_breakout_trigger", "reason_code", "price", "bullish", V7TagActionEvidence, "Price within reach of the breakout trigger."),
	"near_range_bottom":            tagDef("near_range_bottom", "reason_code", "price", "bullish", V7TagActionEvidence, "Price near the range bottom."),
	"near_range_top":               tagDef("near_range_top", "reason_code", "price", "bearish", V7TagActionEvidence, "Price near the range top."),
	"ideal_bb_width":               tagDef("ideal_bb_width", "reason_code", "price", "neutral", V7TagActionEvidence, "Bollinger width in the ideal pre-expansion band."),
	"above_ema60":                  tagDef("above_ema60", "reason_code", "price", "bullish", V7TagActionEvidence, "Price above EMA60."),
	"below_ema20_1h":               tagDef("below_ema20_1h", "reason_code", "price", "bearish", V7TagActionEvidence, "Price below the 1h EMA20."),
	"below_ema60_1h":               tagDef("below_ema60_1h", "reason_code", "price", "bearish", V7TagActionEvidence, "Price below the 1h EMA60."),
	"below_vwap_distribution":      tagDef("below_vwap_distribution", "reason_code", "price", "bearish", V7TagActionEvidence, "Price rejected below VWAP during distribution."),
	"far_from_ema20":               tagDef("far_from_ema20", "reason_code", "price", "mixed", V7TagActionEvidence, "Price extended far from EMA20; mean-reversion pull."),
	"extreme_ema_extension":        tagDef("extreme_ema_extension", "reason_code", "price", "mixed", V7TagActionEvidence, "Extreme extension from EMA; do not chase, wait for a pullback."),
	"moderate_resistance_distance": tagDef("moderate_resistance_distance", "reason_code", "price", "neutral", V7TagActionEvidence, "Meaningful but not blocking distance to overhead resistance."),
	"some_resistance_overhead":     tagDef("some_resistance_overhead", "reason_code", "price", "bearish", V7TagActionEvidence, "Resistance overhead limits immediate upside."),
	"uptrend_intact":               tagDef("uptrend_intact", "reason_code", "price", "bullish", V7TagActionEvidence, "Higher-timeframe uptrend structure intact."),

	// ---- Price action / momentum states ----
	"1h_green_shoot":              tagDef("1h_green_shoot", "reason_code", "price", "bullish", V7TagActionEvidence, "First 1h strength after a decline."),
	"extended_24h_gain":           tagDef("extended_24h_gain", "reason_code", "price", "mixed", V7TagActionEvidence, "24h gain already extended; late-entry risk."),
	"extreme_rally":               tagDef("extreme_rally", "reason_code", "price", "mixed", V7TagActionEvidence, "Rally at extreme magnitude; blow-off risk both ways."),
	"strong_rally":                tagDef("strong_rally", "reason_code", "price", "bullish", V7TagActionEvidence, "Strong sustained rally."),
	"overextended_rally":          tagDef("overextended_rally", "reason_code", "price", "bearish", V7TagActionEvidence, "Rally overextended versus its base; correction risk elevated."),
	"rally_stalling_near_high":    tagDef("rally_stalling_near_high", "reason_code", "price", "bearish", V7TagActionEvidence, "Momentum stalling right under the high; distribution risk."),
	"healthy_pullback":            tagDef("healthy_pullback", "reason_code", "price", "bullish", V7TagActionEvidence, "Orderly pullback on declining volume within an uptrend."),
	"moderate_pullback":           tagDef("moderate_pullback", "reason_code", "price", "bullish", V7TagActionEvidence, "Moderate pullback; trend intact."),
	"moderate_pullback_1h":        tagDef("moderate_pullback_1h", "reason_code", "price", "bullish", V7TagActionEvidence, "Moderate 1h pullback within the larger move."),
	"moderate_1h_drop":            tagDef("moderate_1h_drop", "reason_code", "price", "bearish", V7TagActionEvidence, "Moderate 1h decline."),
	"sharp_1h_drop":               tagDef("sharp_1h_drop", "reason_code", "price", "bearish", V7TagActionEvidence, "Sharp 1h decline; panic conditions forming."),
	"deep_capitulation":           tagDef("deep_capitulation", "reason_code", "price", "bullish", V7TagActionEvidence, "Deep capitulation-grade selloff; reversal-long precondition."),
	"heavy_capitulation":          tagDef("heavy_capitulation", "reason_code", "price", "bullish", V7TagActionEvidence, "Heavy capitulation with forced selling."),
	"moderate_capitulation":       tagDef("moderate_capitulation", "reason_code", "price", "bullish", V7TagActionEvidence, "Moderate capitulation; selling pressure maturing."),
	"price_bouncing_from_support": tagDef("price_bouncing_from_support", "reason_code", "price", "bullish", V7TagActionEvidence, "Price bouncing off identified support."),
	"price_flattening":            tagDef("price_flattening", "reason_code", "price", "neutral", V7TagActionEvidence, "Decline flattening out; base building."),
	"price_stalling_after_rally":  tagDef("price_stalling_after_rally", "reason_code", "price", "bearish", V7TagActionEvidence, "Advance stalling after a rally."),
	"price_turning_down":          tagDef("price_turning_down", "reason_code", "price", "bearish", V7TagActionEvidence, "Price rolling over from the recent high."),
	"price_turning_up":            tagDef("price_turning_up", "reason_code", "price", "bullish", V7TagActionEvidence, "Price turning up from the recent low."),
	"quiet_1h_price_action":       tagDef("quiet_1h_price_action", "reason_code", "price", "neutral", V7TagActionEvidence, "Quiet 1h tape; accumulation-friendly conditions."),
	"weak_bounce":                 tagDef("weak_bounce", "reason_code", "price", "bearish", V7TagActionEvidence, "Bounce lacks participation; continuation-short evidence."),
	"momentum_stalling":           tagDef("momentum_stalling", "reason_code", "price", "bearish", V7TagActionEvidence, "Momentum decelerating versus the impulse leg."),

	// ---- Funding / crowding ----
	"elevated_funding":                   tagDef("elevated_funding", "reason_code", "funding", "mixed", V7TagActionEvidence, "Funding elevated; crowding building."),
	"extreme_funding":                    tagDef("extreme_funding", "reason_code", "funding", "mixed", V7TagActionEvidence, "Funding at an extreme; squeeze/reversal fuel."),
	"very_high_funding":                  tagDef("very_high_funding", "reason_code", "funding", "mixed", V7TagActionEvidence, "Funding very high; crowded-direction entries penalized."),
	"high_funding":                       tagDef("high_funding", "reason_code", "funding", "mixed", V7TagActionEvidence, "Funding high; monitor crowding."),
	"funding_long_crowding":              tagDef("funding_long_crowding", "reason_code", "funding", "bearish", V7TagActionEvidence, "Positive funding shows crowded longs."),
	"long_crowding":                      tagDef("long_crowding", "reason_code", "funding", "bearish", V7TagActionEvidence, "Long positioning crowded."),
	"heavy_long_crowding":                tagDef("heavy_long_crowding", "reason_code", "funding", "bearish", V7TagActionEvidence, "Heavily crowded longs; flush risk."),
	"extreme_long_crowding":              tagDef("extreme_long_crowding", "reason_code", "funding", "bearish", V7TagActionEvidence, "Extreme long crowding; unwind can cascade."),
	"short_crowding":                     tagDef("short_crowding", "reason_code", "funding", "bullish", V7TagActionEvidence, "Short positioning crowded; squeeze fuel."),
	"heavy_short_crowding":               tagDef("heavy_short_crowding", "reason_code", "funding", "bullish", V7TagActionEvidence, "Heavily crowded shorts."),
	"extreme_short_crowding":             tagDef("extreme_short_crowding", "reason_code", "funding", "bullish", V7TagActionEvidence, "Extreme short crowding; violent squeeze potential."),
	"negative_funding_short_crowding":    tagDef("negative_funding_short_crowding", "reason_code", "funding", "bullish", V7TagActionEvidence, "Negative funding confirms crowded shorts."),
	"positive_funding_crowding":          tagDef("positive_funding_crowding", "reason_code", "funding", "bearish", V7TagActionEvidence, "Positive funding confirms crowded longs."),
	"crowded_longs_near_resistance":      tagDef("crowded_longs_near_resistance", "reason_code", "funding", "bearish", V7TagActionEvidence, "Crowded longs pressing into resistance; distribution-short context."),
	"momentum_crowded_long":              tagDef("momentum_crowded_long", "reason_code", "funding", "mixed", V7TagActionEvidence, "Momentum long already crowded by funding/LSR."),
	"short_crowding_without_breakdown":   tagDef("short_crowding_without_breakdown", "reason_code", "funding", "bullish", V7TagActionEvidence, "Shorts crowded but price refuses to break down; squeeze-long setup."),
	"funding_long_low_edge":              tagDef("funding_long_low_edge", "reason_code", "funding", "neutral", V7TagActionEvidence, "Funding-reversal long lacks edge at current readings."),
	"funding_long_needs_stronger_timing": tagDef("funding_long_needs_stronger_timing", "reason_code", "funding", "neutral", V7TagActionEvidence, "Funding long requires stronger timing confirmation before entry."),
	"funding_extreme_fast_track":         tagDef("funding_extreme_fast_track", "reason_code", "funding", "mixed", V7TagActionEvidence, "Extreme funding fast-tracked this signal past zone requirements; verify entry live."),
	"fast_tracked_funding":               tagDef("fast_tracked_funding", "reason_code", "funding", "mixed", V7TagActionEvidence, "Signal fast-tracked due to funding extremity."),
	"momentum_extreme_funding_wait":      tagDef("momentum_extreme_funding_wait", "risk_tag", "funding", "bearish", V7TagActionWaitOnly, "Momentum long blocked while funding is extreme; wait for funding to normalize."),
	"funding_short_weak_4h_flush_wait":   tagDef("funding_short_weak_4h_flush_wait", "risk_tag", "oi", "mixed", V7TagActionWaitOnly, "Funding short must wait: 4h OI flush is insufficient."),
	"trend_down_funding_long_watch_only": tagDef("trend_down_funding_long_watch_only", "risk_tag", "tier", "neutral", V7TagActionWaitOnly, "Funding-reversal long in a downtrend regime is watch-only."),

	// ---- Squeeze / compression ----
	"extreme_squeeze":             tagDef("extreme_squeeze", "reason_code", "oi", "bullish", V7TagActionEvidence, "Extreme squeeze conditions: crowding + trigger proximity."),
	"strong_squeeze":              tagDef("strong_squeeze", "reason_code", "oi", "bullish", V7TagActionEvidence, "Strong squeeze conditions."),
	"moderate_squeeze":            tagDef("moderate_squeeze", "reason_code", "oi", "bullish", V7TagActionEvidence, "Moderate squeeze conditions."),
	"mild_squeeze":                tagDef("mild_squeeze", "reason_code", "oi", "bullish", V7TagActionEvidence, "Mild squeeze conditions."),
	"deep_compression":            tagDef("deep_compression", "reason_code", "price", "neutral", V7TagActionEvidence, "Deep volatility compression; expansion energy stored."),
	"mild_compression":            tagDef("mild_compression", "reason_code", "price", "neutral", V7TagActionEvidence, "Mild volatility compression."),
	"volatility_squeeze_detected": tagDef("volatility_squeeze_detected", "reason_code", "price", "neutral", V7TagActionEvidence, "Bollinger/ATR squeeze detected; direction undecided."),
	"squeeze_chase_risk":          tagDef("squeeze_chase_risk", "risk_tag", "risk", "bullish", V7TagActionReduceOrWait, "Squeeze already running; chasing has poor RR — reduce size or wait for a retest."),
	"high_cascade_potential":      tagDef("high_cascade_potential", "reason_code", "oi", "bullish", V7TagActionEvidence, "Liquidation cascade potential is high if the trigger breaks."),
	"moderate_cascade_potential":  tagDef("moderate_cascade_potential", "reason_code", "oi", "bullish", V7TagActionEvidence, "Moderate liquidation cascade potential."),
	"low_cascade_potential":       tagDef("low_cascade_potential", "reason_code", "oi", "neutral", V7TagActionEvidence, "Limited cascade potential."),

	// ---- State / tier / process ----
	"accumulation_watch":                                 tagDef("accumulation_watch", "reason_code", "state", "bullish", V7TagActionContextOnly, "Accumulation watch item; not yet an entry signal."),
	"pre_distribution_watch":                             tagDef("pre_distribution_watch", "reason_code", "state", "bearish", V7TagActionContextOnly, "Pre-distribution watch item."),
	"pre_short_squeeze_watch":                            tagDef("pre_short_squeeze_watch", "reason_code", "state", "bullish", V7TagActionContextOnly, "Pre-squeeze watch item."),
	"pre_move_radar":                                     tagDef("pre_move_radar", "reason_code", "state", "neutral", V7TagActionContextOnly, "Radar-detected pre-move candidate; watch-only context."),
	"watch_only":                                         tagDef("watch_only", "risk_tag", "state", "neutral", V7TagActionWaitOnly, "Watch-only; cannot be opened this cycle."),
	"no_setup_matched":                                   tagDef("no_setup_matched", "reason_code", "state", "neutral", V7TagActionContextOnly, "Symbol entered the universe but matched no setup."),
	"watch_upgraded_executable":                          tagDef("watch_upgraded_executable", "reason_code", "state", "bullish", V7TagActionOpenSupport, "Cross-cycle state machine upgraded this watch signal to executable after repeated confirmation."),
	"watch_upgraded_reviewable":                          tagDef("watch_upgraded_reviewable", "reason_code", "state", "bullish", V7TagActionReviewableOnly, "Cross-cycle state machine upgraded this watch signal to reviewable; live confirmation still required."),
	"multi_cycle_confirmation":                           tagDef("multi_cycle_confirmation", "reason_code", "state", "bullish", V7TagActionOpenSupport, "Signal persisted and strengthened across multiple polling cycles."),
	"directional_conflict":                               tagDef("directional_conflict", "risk_tag", "state", "mixed", V7TagActionWaitOnly, "Long and short setups fired on the same symbol; stand aside until resolved."),
	"strong_symbol_regime_override":                      tagDef("strong_symbol_regime_override", "reason_code", "state", "bullish", V7TagActionEvidence, "Symbol outperforms BTC/ETH enough to override regime down-weighting."),
	"regime_against_direction":                           tagDef("regime_against_direction", "risk_tag", "state", "mixed", V7TagActionReduceOrWait, "Setup trades against the market regime; require stronger confirmation and smaller size."),
	"remote_target_only_context":                         tagDef("remote_target_only_context", "reason_code", "tier", "neutral", V7TagActionContextOnly, "Targets are far from price; context only, not an execution plan."),
	"invalid_rr_context_only":                            tagDef("invalid_rr_context_only", "reason_code", "tier", "neutral", V7TagActionContextOnly, "Geometry failed minimum RR; retained as context only."),
	"thin_rr_wait_confirm":                               tagDef("thin_rr_wait_confirm", "risk_tag", "tier", "neutral", V7TagActionReviewableOnly, "RR between 1.2 and 1.5; open only if live repricing restores RR >= 1.5."),
	"entry_zone_normalized":                              tagDef("entry_zone_normalized", "reason_code", "tier", "neutral", V7TagActionEvidence, "Entry zone was normalized (inverted bounds repaired) by the router."),
	"displacement_rr_revalidated_after_execution_repair": tagDef("displacement_rr_revalidated_after_execution_repair", "reason_code", "tier", "bullish", V7TagActionEvidence, "Displacement RR re-passed after backend stop/target repair."),
	"wait_reclaim_or_lower_zone_required":                tagDef("wait_reclaim_or_lower_zone_required", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Wait for a reclaim or a lower entry zone before the long."),
	"wait_zone_retest_required":                          tagDef("wait_zone_retest_required", "risk_tag", "price", "neutral", V7TagActionWaitOnly, "Wait for the entry zone retest before acting."),
	"oi_or_volume_expands_with_price":                    tagDef("oi_or_volume_expands_with_price", "required_confirmation", "oi", "bullish", V7TagActionRequiredConfirm, "OI or volume must expand together with the price move."),

	// ---- Market shape enum (emitted as reason codes via string(V7Shape*)) ----
	"shape_trend_breakout":            tagDef("shape_trend_breakout", "reason_code", "state", "bullish", V7TagActionEvidence, "Market shape: trending breakout structure."),
	"shape_clean_momentum":            tagDef("shape_clean_momentum", "reason_code", "state", "bullish", V7TagActionEvidence, "Market shape: clean directional momentum without churn."),
	"shape_pullback_continuation":     tagDef("shape_pullback_continuation", "reason_code", "state", "bullish", V7TagActionEvidence, "Market shape: pullback inside a continuing trend."),
	"shape_panic_reversal":            tagDef("shape_panic_reversal", "reason_code", "state", "bullish", V7TagActionEvidence, "Market shape: capitulation with reversal conditions."),
	"shape_funding_crowding_reversal": tagDef("shape_funding_crowding_reversal", "reason_code", "state", "mixed", V7TagActionEvidence, "Market shape: crowded positioning reversing."),
	"shape_distribution_short":        tagDef("shape_distribution_short", "reason_code", "state", "bearish", V7TagActionEvidence, "Market shape: distribution rolling over."),
	"shape_range_reversion":           tagDef("shape_range_reversion", "reason_code", "state", "neutral", V7TagActionEvidence, "Market shape: mean reversion inside a defined range."),
	"shape_compression_prebreakout":   tagDef("shape_compression_prebreakout", "reason_code", "state", "neutral", V7TagActionEvidence, "Market shape: volatility compression ahead of expansion."),
	"shape_noise_no_trade":            tagDef("shape_noise_no_trade", "reason_code", "state", "neutral", V7TagActionContextOnly, "Market shape: directionless noise; no tradeable structure."),

	// ---- Entry signal enum (emitted as reason codes via string(V7EntrySignal*)) ----
	"entry_open_now":          tagDef("entry_open_now", "reason_code", "tier", "bullish", V7TagActionOpenSupport, "Execution window is open now; entry conditions are live."),
	"entry_trigger_near":      tagDef("entry_trigger_near", "reason_code", "tier", "bullish", V7TagActionReviewableOnly, "Price is near the trigger; open only on live trigger confirmation."),
	"entry_pullback_wait":     tagDef("entry_pullback_wait", "reason_code", "tier", "neutral", V7TagActionWaitOnly, "Wait for a pullback into the entry zone."),
	"entry_breakout_wait":     tagDef("entry_breakout_wait", "reason_code", "tier", "neutral", V7TagActionWaitOnly, "Wait for the breakout level to be taken."),
	"entry_reclaim_wait":      tagDef("entry_reclaim_wait", "reason_code", "tier", "neutral", V7TagActionWaitOnly, "Wait for the reclaim level to hold."),
	"entry_rr_repairable":     tagDef("entry_rr_repairable", "reason_code", "tier", "neutral", V7TagActionReviewableOnly, "Raw RR is thin but backend stop/target repair can restore it; review with repaired geometry."),
	"entry_rr_invalid":        tagDef("entry_rr_invalid", "reason_code", "tier", "neutral", V7TagActionContextOnly, "Entry geometry cannot reach minimum RR; context only."),
	"entry_chase_risk":        tagDef("entry_chase_risk", "reason_code", "tier", "mixed", V7TagActionWaitOnly, "Entering here chases an extended move; wait for re-entry."),
	"entry_liquidity_blocked": tagDef("entry_liquidity_blocked", "reason_code", "tier", "neutral", V7TagActionContextOnly, "Liquidity too thin for execution; blocked."),
	"entry_no_trade":          tagDef("entry_no_trade", "reason_code", "tier", "neutral", V7TagActionContextOnly, "No tradeable entry in this cycle."),

	// ---- Required-confirmation templates (defaultV7Confirmations + modules) ----
	"15m_close_below_vwap":                          tagDef("15m_close_below_vwap", "required_confirmation", "price", "bearish", V7TagActionRequiredConfirm, "A 15m candle must close below VWAP."),
	"15m_close_below_vwap_or_ema20":                 tagDef("15m_close_below_vwap_or_ema20", "required_confirmation", "price", "bearish", V7TagActionRequiredConfirm, "A 15m candle must close below VWAP or EMA20."),
	"15m_reclaim_vwap":                              tagDef("15m_reclaim_vwap", "required_confirmation", "price", "bullish", V7TagActionRequiredConfirm, "A 15m candle must reclaim VWAP."),
	"15m_rejection_at_resistance_or_entry_zone":     tagDef("15m_rejection_at_resistance_or_entry_zone", "required_confirmation", "price", "bearish", V7TagActionRequiredConfirm, "A 15m rejection must print at resistance or inside the entry zone."),
	"funding_remains_negative_or_lsr_crowded_short": tagDef("funding_remains_negative_or_lsr_crowded_short", "required_confirmation", "funding", "bullish", V7TagActionRequiredConfirm, "Funding must stay negative or LSR must stay short-crowded (squeeze fuel intact)."),
	"funding_remains_positive_or_lsr_crowded_long":  tagDef("funding_remains_positive_or_lsr_crowded_long", "required_confirmation", "funding", "bearish", V7TagActionRequiredConfirm, "Funding must stay positive or LSR must stay long-crowded (unwind fuel intact)."),
	"no_failed_breakout_back_inside_range":          tagDef("no_failed_breakout_back_inside_range", "required_confirmation", "price", "bullish", V7TagActionRequiredConfirm, "The breakout must not fail back inside the range."),
	"no_immediate_loss_of_trigger_level":            tagDef("no_immediate_loss_of_trigger_level", "required_confirmation", "price", "bullish", V7TagActionRequiredConfirm, "Price must hold above the trigger level after breaking it."),
	"no_immediate_reclaim_of_trigger_level":         tagDef("no_immediate_reclaim_of_trigger_level", "required_confirmation", "price", "bearish", V7TagActionRequiredConfirm, "Price must stay below the broken level; an immediate reclaim invalidates the short."),
	"no_new_high_after_crowding_signal":             tagDef("no_new_high_after_crowding_signal", "required_confirmation", "price", "bearish", V7TagActionRequiredConfirm, "No new high may print after the crowding signal fired."),
	"no_new_low_after_crowding_signal":              tagDef("no_new_low_after_crowding_signal", "required_confirmation", "price", "bullish", V7TagActionRequiredConfirm, "No new low may print after the crowding signal fired."),
	"oi_flush_or_failed_oi_rebuild":                 tagDef("oi_flush_or_failed_oi_rebuild", "required_confirmation", "oi", "mixed", V7TagActionRequiredConfirm, "OI must flush, or a rebuild attempt must visibly fail."),
	"oi_stabilizes_or_short_covering_starts":        tagDef("oi_stabilizes_or_short_covering_starts", "required_confirmation", "oi", "bullish", V7TagActionRequiredConfirm, "OI must stabilize or short covering must begin."),
	"wait_for_directional_break":                    tagDef("wait_for_directional_break", "required_confirmation", "price", "neutral", V7TagActionRequiredConfirm, "Both directions fired; wait for a decisive break before choosing a side."),
	"long_trigger: break_high_with_oi_increase":     tagDef("long_trigger: break_high_with_oi_increase", "required_confirmation", "price", "bullish", V7TagActionRequiredConfirm, "Long side triggers on a high break accompanied by OI increase."),
	"short_trigger: lose_vwap_with_taker_sell":      tagDef("short_trigger: lose_vwap_with_taker_sell", "required_confirmation", "price", "bearish", V7TagActionRequiredConfirm, "Short side triggers on losing VWAP with taker selling."),

	// ---- Module no-match diagnostics ----
	"no_match_unknown":                tagDef("no_match_unknown", "reason_code", "state", "neutral", V7TagActionContextOnly, "No setup matched; unclassified conditions."),
	"no_match_low_liquidity":          tagDef("no_match_low_liquidity", "reason_code", "state", "neutral", V7TagActionContextOnly, "No setup matched; turnover below tradeable levels."),
	"no_match_low_amplitude":          tagDef("no_match_low_amplitude", "reason_code", "state", "neutral", V7TagActionContextOnly, "No setup matched; price range too quiet."),
	"no_match_late_move":              tagDef("no_match_late_move", "reason_code", "state", "neutral", V7TagActionContextOnly, "No setup matched; the move is already too extended to enter."),
	"no_match_pre_breakout_candidate": tagDef("no_match_pre_breakout_candidate", "reason_code", "state", "bullish", V7TagActionContextOnly, "No setup matched yet, but compression suggests a forming breakout."),
	"no_match_oi_flow_candidate":      tagDef("no_match_oi_flow_candidate", "reason_code", "state", "mixed", V7TagActionContextOnly, "No setup matched yet, but OI flow is notable."),
	"no_match_conflict_price_oi":      tagDef("no_match_conflict_price_oi", "reason_code", "state", "mixed", V7TagActionContextOnly, "No setup matched; price and OI disagree."),
	"no_match_mid_range_noise":        tagDef("no_match_mid_range_noise", "reason_code", "state", "neutral", V7TagActionContextOnly, "No setup matched; price is mid-range noise."),
}

// describeHunterV7PrefixTag resolves runtime-generated tag families that
// cannot be enumerated statically. Without this, a machine-verified live
// confirmation would reach the LLM described as "unknown, context only" —
// the opposite of what it is.
func describeHunterV7PrefixTag(tag, source string) (HunterV7TagDefinition, bool) {
	switch {
	case len(tag) > len("live_confirmed_") && tag[:len("live_confirmed_")] == "live_confirmed_":
		return tagDef(tag, source, "confirmation", "bullish", V7TagActionOpenSupport,
			"The required confirmation '"+tag[len("live_confirmed_"):]+"' was machine-verified against live market data in this cycle."), true
	case len(tag) > len("sector_theme_") && tag[:len("sector_theme_")] == "sector_theme_":
		return tagDef(tag, source, "state", "bullish", V7TagActionEvidence,
			"Symbol belongs to the '"+tag[len("sector_theme_"):]+"' sector theme currently showing broad relative strength."), true
	}
	return HunterV7TagDefinition{}, false
}

func init() {
	for name, def := range hunterV7TagCatalogExt {
		if _, exists := hunterV7TagCatalog[name]; exists {
			panic("hunter v7 tag catalog: duplicate definition for " + name)
		}
		hunterV7TagCatalog[name] = def
	}
}
