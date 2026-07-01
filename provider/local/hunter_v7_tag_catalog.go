package local

// HunterV7TagDefinition describes how a tag should be interpreted by the
// downstream LLM and execution gate. It is intentionally metadata-only: signal
// modules may keep emitting plain strings while the prompt layer gets a stable
// semantic dictionary.
type HunterV7TagDefinition struct {
	Tag        string `json:"tag"`
	Source     string `json:"source"`   // reason_code, risk_tag, required_confirmation
	Category   string `json:"category"` // price, flow, oi, funding, risk, tier, state, confirmation
	Polarity   string `json:"polarity"` // bullish, bearish, neutral, mixed
	LLMAction  string `json:"llm_action"`
	Definition string `json:"definition"`
}

const (
	V7TagActionEvidence        = "evidence_only"
	V7TagActionRequiredConfirm = "required_confirmation"
	V7TagActionWaitOnly        = "wait_only"
	V7TagActionRejectOnly      = "reject_only"
	V7TagActionReviewableOnly  = "reviewable_only_if_live_confirmed"
	V7TagActionContextOnly     = "context_only"
	V7TagActionReduceOrWait    = "reduce_size_or_wait"
	V7TagActionOpenSupport     = "supports_open_after_core_checks"
	V7TagActionUnknown         = "unknown_context_only"
)

var hunterV7TagCatalog = map[string]HunterV7TagDefinition{
	// Tier/state tags.
	"watch_only_no_direct_open":              tagDef("watch_only_no_direct_open", "reason_code", "state", "neutral", V7TagActionWaitOnly, "Signal is watch-only context and cannot be opened directly in this cycle."),
	"do_not_open_until_confirmed":            tagDef("do_not_open_until_confirmed", "risk_tag", "state", "neutral", V7TagActionWaitOnly, "Direct opening is blocked until the listed required confirmations are met in a later cycle."),
	"context_only_low_priority":              tagDef("context_only_low_priority", "risk_tag", "tier", "neutral", V7TagActionContextOnly, "Low-priority fallback context; do not use this tag alone as an opening reason."),
	"fallback_reviewable_needs_live_confirm": tagDef("fallback_reviewable_needs_live_confirm", "risk_tag", "tier", "neutral", V7TagActionReviewableOnly, "Router rescued this signal into the review pool; live price/flow/RR confirmation is mandatory."),
	"reviewable_floor_rescue":                tagDef("reviewable_floor_rescue", "reason_code", "tier", "neutral", V7TagActionReviewableOnly, "Signal was promoted to avoid an empty review pool; it is not automatically executable."),
	"candidate_floor_context":                tagDef("candidate_floor_context", "reason_code", "tier", "neutral", V7TagActionContextOnly, "Candidate was included as minimum context despite lower priority."),
	"module_no_match":                        tagDef("module_no_match", "risk_tag", "state", "neutral", V7TagActionRejectOnly, "Symbol entered universe but no Hunter v7 setup matched."),

	// Hard/wait risk tags.
	"risk_filtered":                            tagDef("risk_filtered", "risk_tag", "risk", "neutral", V7TagActionRejectOnly, "Risk score exceeded the router filter; do not open."),
	"liquidity_filtered":                       tagDef("liquidity_filtered", "risk_tag", "risk", "neutral", V7TagActionRejectOnly, "Liquidity failed the router filter; do not open."),
	"extreme_volatility":                       tagDef("extreme_volatility", "risk_tag", "risk", "mixed", V7TagActionRejectOnly, "Volatility is extreme enough to invalidate normal execution assumptions."),
	"wash_volume_high":                         tagDef("wash_volume_high", "risk_tag", "risk", "neutral", V7TagActionRejectOnly, "Suspiciously high volume/trade-count pattern; avoid opening."),
	"do_not_market_chase":                      tagDef("do_not_market_chase", "risk_tag", "risk", "neutral", V7TagActionWaitOnly, "Move is extended; wait for re-entry instead of market chasing."),
	"funding_extreme":                          tagDef("funding_extreme", "risk_tag", "funding", "mixed", V7TagActionReduceOrWait, "Funding is extreme; crowded-direction entries are blocked, while counter-crowd panic reversals require live confirmation, smaller size, and RR validation."),
	"funding_elevated":                         tagDef("funding_elevated", "risk_tag", "funding", "mixed", V7TagActionReduceOrWait, "Funding is elevated; require live confirmation and conservative sizing."),
	"crowding_extreme":                         tagDef("crowding_extreme", "risk_tag", "funding", "mixed", V7TagActionWaitOnly, "Positioning is crowded; require unwind/reversal confirmation before any open."),
	"high_volatility":                          tagDef("high_volatility", "risk_tag", "risk", "mixed", V7TagActionReduceOrWait, "Volatility is high; use smaller size and require valid live entry/stop/RR."),
	"moderate_liquidity":                       tagDef("moderate_liquidity", "risk_tag", "risk", "neutral", V7TagActionReduceOrWait, "Liquidity is moderate; avoid oversizing and verify slippage-sensitive entries."),
	"already_pumped_24h":                       tagDef("already_pumped_24h", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Long setup is late after a strong 24h pump; do not chase."),
	"funding_expensive":                        tagDef("funding_expensive", "risk_tag", "funding", "bullish", V7TagActionWaitOnly, "Long funding cost is too expensive for direct entry."),
	"crowding_elevated":                        tagDef("crowding_elevated", "risk_tag", "funding", "mixed", V7TagActionReduceOrWait, "Crowding is elevated; require live entry, flow, and RR confirmation before any open."),
	"lsr_extreme_long":                         tagDef("lsr_extreme_long", "risk_tag", "funding", "bearish", V7TagActionWaitOnly, "Long/short ratio shows extreme long crowding."),
	"taker_sell_during_accumulation":           tagDef("taker_sell_during_accumulation", "risk_tag", "flow", "bearish", V7TagActionWaitOnly, "Accumulation thesis conflicts with active sell flow."),
	"no_reclaim_signal":                        tagDef("no_reclaim_signal", "risk_tag", "price", "neutral", V7TagActionWaitOnly, "Reversal has not reclaimed a usable level; wait for reclaim."),
	"oi_up_price_down":                         tagDef("oi_up_price_down", "risk_tag", "oi", "bearish", V7TagActionWaitOnly, "Open interest rises while price falls; reversal long is not confirmed."),
	"not_near_short_retest_zone":               tagDef("not_near_short_retest_zone", "risk_tag", "price", "bearish", V7TagActionWaitOnly, "Short is away from the required retest/entry zone."),
	"not_near_long_reclaim_zone":               tagDef("not_near_long_reclaim_zone", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Long is away from the required reclaim/entry zone."),
	"late_short_after_deep_drop":               tagDef("late_short_after_deep_drop", "risk_tag", "price", "bearish", V7TagActionWaitOnly, "Short entry is late after a deep drop without fresh retest."),
	"short_after_fast_drop_without_flush":      tagDef("short_after_fast_drop_without_flush", "risk_tag", "price", "bearish", V7TagActionWaitOnly, "Avoid shorting a fast drop before OI/crowding flush confirms."),
	"late_long_after_deep_pump":                tagDef("late_long_after_deep_pump", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Long entry is late after a deep pump."),
	"long_after_fast_pump_without_flush":       tagDef("long_after_fast_pump_without_flush", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Avoid chasing a fast pump without reset/flush."),
	"weak_4h_oi_flush":                         tagDef("weak_4h_oi_flush", "risk_tag", "oi", "mixed", V7TagActionWaitOnly, "4h OI flush is insufficient for funding-reversal short confirmation."),
	"oi_building_no_flush":                     tagDef("oi_building_no_flush", "risk_tag", "oi", "mixed", V7TagActionWaitOnly, "OI is building without crowding flush; reversal needs more confirmation."),
	"momentum_confirmation_missing":            tagDef("momentum_confirmation_missing", "risk_tag", "flow", "bullish", V7TagActionWaitOnly, "Momentum setup lacks required near-term confirmation."),
	"momentum_overheated":                      tagDef("momentum_overheated", "risk_tag", "risk", "bullish", V7TagActionWaitOnly, "Momentum is overheated; wait for pullback/retest."),
	"momentum_chase_risk":                      tagDef("momentum_chase_risk", "risk_tag", "risk", "bullish", V7TagActionWaitOnly, "Momentum entry would be a chase unless re-entry conditions improve."),
	"scalp_global_geometry_incompatible":       tagDef("scalp_global_geometry_incompatible", "risk_tag", "risk", "neutral", V7TagActionWaitOnly, "Intraday scalp target/stop geometry is incompatible with the global backend stop/RR policy; use as context only."),
	"needs_oi_confirmation":                    tagDef("needs_oi_confirmation", "risk_tag", "oi", "mixed", V7TagActionRequiredConfirm, "OI data is missing or inconclusive; require OI/volume confirmation before open."),
	"displacement_rr_insufficient":             tagDef("displacement_rr_insufficient", "risk_tag", "risk", "bullish", V7TagActionRejectOnly, "Displacement target/stop geometry cannot provide minimum RR."),
	"displacement_rr_repaired":                 tagDef("displacement_rr_repaired", "risk_tag", "risk", "bullish", V7TagActionReduceOrWait, "Displacement first target is thin, but later continuation target provides minimum RR; verify live backend geometry before opening."),
	"event_flow_confirmation_needed":           tagDef("event_flow_confirmation_needed", "risk_tag", "flow", "mixed", V7TagActionRequiredConfirm, "High-amplitude event lacks aligned taker flow; require live flow confirmation before opening."),
	"event_chase_risk":                         tagDef("event_chase_risk", "risk_tag", "risk", "mixed", V7TagActionWaitOnly, "High-amplitude event is timing-late or too fast; wait for retest or renewed confirmation."),
	"short_covering_not_new_long_build":        tagDef("short_covering_not_new_long_build", "risk_tag", "oi", "bullish", V7TagActionReduceOrWait, "Event long may be driven by short covering because OI is falling on both 1h and 4h; require fresh OI or volume expansion before opening."),
	"range_expansion_low_volume_followthrough": tagDef("range_expansion_low_volume_followthrough", "risk_tag", "flow", "mixed", V7TagActionReduceOrWait, "Range expansion lacks 15m volume follow-through; require renewed volume/taker confirmation or wait for retest."),
	"rsi_extreme_with_crowded_funding":         tagDef("rsi_extreme_with_crowded_funding", "risk_tag", "risk", "bullish", V7TagActionWaitOnly, "RSI and funding are both crowded; wait for reset."),
	"execution_stop_tightened":                 tagDef("execution_stop_tightened", "risk_tag", "risk", "neutral", V7TagActionReduceOrWait, "Router tightened stop to near structure; verify backend min stop and RR."),

	// Positive evidence tags used by tiering/prompt.
	"strong_reclaim":                           tagDef("strong_reclaim", "reason_code", "price", "bullish", V7TagActionOpenSupport, "Clear reclaim after capitulation; supports panic-reversal long after RR checks."),
	"solid_reclaim":                            tagDef("solid_reclaim", "reason_code", "price", "bullish", V7TagActionEvidence, "Moderate reclaim evidence; still needs live confirmation."),
	"early_reclaim":                            tagDef("early_reclaim", "reason_code", "price", "bullish", V7TagActionEvidence, "Early reclaim signal; weaker than strong reclaim."),
	"selling_decelerating":                     tagDef("selling_decelerating", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Sell pressure is slowing; supports reversal confirmation."),
	"selling_exhaustion":                       tagDef("selling_exhaustion", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Sell pressure appears exhausted; supports reversal confirmation."),
	"taker_buy_aggressive":                     tagDef("taker_buy_aggressive", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Aggressive buy taker flow; strong long confirmation."),
	"taker_buy_strong":                         tagDef("taker_buy_strong", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Strong buy taker flow; supports long confirmation."),
	"taker_buy_aligned":                        tagDef("taker_buy_aligned", "reason_code", "flow", "bullish", V7TagActionEvidence, "Buy-side taker flow is aligned with a long setup."),
	"taker_buy_neutral":                        tagDef("taker_buy_neutral", "reason_code", "flow", "mixed", V7TagActionEvidence, "Taker flow is not bearish but still needs live confirmation."),
	"taker_sustained_buy":                      tagDef("taker_sustained_buy", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Sustained buy-side pressure; stronger than taker_strong_buy for momentum."),
	"taker_strong_buy":                         tagDef("taker_strong_buy", "reason_code", "flow", "bullish", V7TagActionEvidence, "Buy flow is strong but not enough alone to override upper-zone chase risk."),
	"taker_weak_buy":                           tagDef("taker_weak_buy", "reason_code", "flow", "bearish", V7TagActionWaitOnly, "Buy flow is weak; do not open long momentum/reversal unless later confirmed."),
	"confirmed_breakout":                       tagDef("confirmed_breakout", "reason_code", "price", "bullish", V7TagActionOpenSupport, "Breakout has confirmed structurally; still verify entry/RR."),
	"taker_aggressive_buy":                     tagDef("taker_aggressive_buy", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Aggressive buy flow confirms breakout/momentum."),
	"moderate_vol_displacement":                tagDef("moderate_vol_displacement", "reason_code", "price", "bullish", V7TagActionEvidence, "1h range expansion is meaningfully above baseline; supports displacement context."),
	"strong_vol_displacement":                  tagDef("strong_vol_displacement", "reason_code", "price", "bullish", V7TagActionEvidence, "1h range expansion is strong; supports displacement continuation review."),
	"massive_vol_displacement":                 tagDef("massive_vol_displacement", "reason_code", "price", "bullish", V7TagActionOpenSupport, "1h range expansion is extreme; supports displacement continuation after RR and confirmation checks."),
	"strong_1h_impulse":                        tagDef("strong_1h_impulse", "reason_code", "price", "bullish", V7TagActionOpenSupport, "Strong positive 1h impulse confirms long displacement direction."),
	"solid_1h_impulse":                         tagDef("solid_1h_impulse", "reason_code", "price", "bullish", V7TagActionEvidence, "Positive 1h impulse supports long displacement direction."),
	"early_1h_displacement":                    tagDef("early_1h_displacement", "reason_code", "price", "bullish", V7TagActionEvidence, "Early positive displacement; requires stronger follow-through before opening."),
	"above_vwap_15m":                           tagDef("above_vwap_15m", "reason_code", "price", "bullish", V7TagActionEvidence, "Price is above 15m VWAP, supporting long-side control."),
	"oi_confirms_new_demand":                   tagDef("oi_confirms_new_demand", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Open interest is increasing with price, supporting new demand."),
	"oi_moderate_inflow":                       tagDef("oi_moderate_inflow", "reason_code", "oi", "bullish", V7TagActionEvidence, "Open interest inflow is positive but moderate."),
	"oi_minimal_confirm":                       tagDef("oi_minimal_confirm", "reason_code", "oi", "mixed", V7TagActionEvidence, "OI confirmation is minimal; require live flow confirmation."),
	"strong_4h_momentum":                       tagDef("strong_4h_momentum", "reason_code", "price", "bullish", V7TagActionOpenSupport, "4h trend is strong enough to support selective momentum review."),
	"solid_4h_momentum":                        tagDef("solid_4h_momentum", "reason_code", "price", "bullish", V7TagActionEvidence, "4h momentum exists but should not override chase/zone guards alone."),
	"strong_24h_momentum":                      tagDef("strong_24h_momentum", "reason_code", "price", "bullish", V7TagActionEvidence, "24h momentum is strong; can also imply late-entry risk."),
	"solid_24h_momentum":                       tagDef("solid_24h_momentum", "reason_code", "price", "bullish", V7TagActionEvidence, "24h momentum is positive but not enough alone to override entry-zone or chase guards."),
	"moderate_24h_momentum":                    tagDef("moderate_24h_momentum", "reason_code", "price", "bullish", V7TagActionEvidence, "24h momentum is moderate context for trend continuation."),
	"accelerating_1h":                          tagDef("accelerating_1h", "reason_code", "price", "bullish", V7TagActionEvidence, "1h momentum is accelerating; supports trend context but does not override overheated/chase guards."),
	"holding_1h":                               tagDef("holding_1h", "reason_code", "price", "bullish", V7TagActionEvidence, "1h structure is holding during momentum continuation."),
	"micro_pullback":                           tagDef("micro_pullback", "reason_code", "price", "bullish", V7TagActionEvidence, "Very shallow pullback in a trend; needs entry-zone check."),
	"shallow_pullback":                         tagDef("shallow_pullback", "reason_code", "price", "bullish", V7TagActionEvidence, "Shallow pullback in a trend; needs entry-zone check."),
	"shallow_pullback_1h":                      tagDef("shallow_pullback_1h", "reason_code", "price", "bullish", V7TagActionEvidence, "1h shallow pullback; do not treat upper-zone weakness as automatic entry."),
	"taker_neutral_buy":                        tagDef("taker_neutral_buy", "reason_code", "flow", "mixed", V7TagActionEvidence, "Taker buy flow is neutral to mildly supportive; require stronger live confirmation before opening."),
	"no_pullback_still_running":                tagDef("no_pullback_still_running", "reason_code", "price", "bullish", V7TagActionWaitOnly, "Momentum is still running without pullback; avoid market chasing until a valid re-entry forms."),
	"chase_high_protection":                    tagDef("chase_high_protection", "reason_code", "risk", "bullish", V7TagActionWaitOnly, "Price is near chase-high protection; wait for pullback or fresh confirmation."),
	"low_timing_watch_only":                    tagDef("low_timing_watch_only", "reason_code", "state", "neutral", V7TagActionWaitOnly, "Timing score is too low for direct execution in this cycle."),
	"leader_momentum_timing_watch_only":        tagDef("leader_momentum_timing_watch_only", "reason_code", "state", "bullish", V7TagActionWaitOnly, "Leader momentum lacks sufficient timing quality for direct opening."),
	"momentum_rsi_overheated_wait":             tagDef("momentum_rsi_overheated_wait", "reason_code", "risk", "bullish", V7TagActionWaitOnly, "Momentum RSI is overheated; wait for reset, pullback, or retest."),
	"scalp_backend_geometry_context":           tagDef("scalp_backend_geometry_context", "reason_code", "risk", "neutral", V7TagActionWaitOnly, "Scalp setup is retained as context because backend global stop/RR geometry does not support direct opening."),
	"intraday_scalp_entry":                     tagDef("intraday_scalp_entry", "reason_code", "price", "bullish", V7TagActionContextOnly, "Intraday scalp pattern is present but must obey global execution geometry before opening."),
	"strong_5m_velocity":                       tagDef("strong_5m_velocity", "reason_code", "price", "bullish", V7TagActionEvidence, "5m directional velocity is strong; useful for timing context only."),
	"oi_massive_flush":                         tagDef("oi_massive_flush", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Large OI flush supports capitulation/reversal."),
	"oi_heavy_flush":                           tagDef("oi_heavy_flush", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Heavy OI flush supports capitulation/reversal."),
	"oi_declining":                             tagDef("oi_declining", "reason_code", "oi", "bullish", V7TagActionEvidence, "OI is declining; supports de-risking/flush context."),
	"oi_healthy_growth":                        tagDef("oi_healthy_growth", "reason_code", "oi", "bullish", V7TagActionEvidence, "OI growth aligns with price; supports trend continuation."),
	"oi_invisible_accumulation_detected":       tagDef("oi_invisible_accumulation_detected", "reason_code", "oi", "bullish", V7TagActionEvidence, "OI is building while price markup remains limited; supports stealth accumulation context."),
	"oi_4h_stealth_build":                      tagDef("oi_4h_stealth_build", "reason_code", "oi", "bullish", V7TagActionEvidence, "4h OI is building without broad price markup."),
	"oi_1h_confirming_accumulation":            tagDef("oi_1h_confirming_accumulation", "reason_code", "oi", "bullish", V7TagActionEvidence, "1h OI confirms ongoing accumulation."),
	"oi_build_without_price_markup":            tagDef("oi_build_without_price_markup", "reason_code", "oi", "bullish", V7TagActionEvidence, "Positioning is building while price remains relatively contained."),
	"funding_not_crowded":                      tagDef("funding_not_crowded", "reason_code", "funding", "bullish", V7TagActionEvidence, "Funding is not crowded, supporting cleaner accumulation or breakout continuation."),
	"whale_flow_detected":                      tagDef("whale_flow_detected", "reason_code", "oi", "bullish", V7TagActionEvidence, "Large-position flow pattern is detected; it is supporting evidence and still needs execution confirmation."),
	"stealth_accumulation_breakout":            tagDef("stealth_accumulation_breakout", "reason_code", "oi", "bullish", V7TagActionEvidence, "Stealth accumulation appears to be transitioning into breakout behavior; verify live price and flow before opening."),
	"bb_compressed":                            tagDef("bb_compressed", "reason_code", "price", "bullish", V7TagActionEvidence, "Bollinger width is compressed; expansion risk is elevated after confirmation."),
	"volume_burst_at_breakout":                 tagDef("volume_burst_at_breakout", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Volume is expanding at or near breakout, supporting follow-through after entry/RR checks."),
	"taker_buy_ratio_above_0.55":               tagDef("taker_buy_ratio_above_0.55", "reason_code", "flow", "bullish", V7TagActionEvidence, "Taker buy ratio is above 0.55, supporting long-side pressure."),
	"lsr_balanced_accumulation":                tagDef("lsr_balanced_accumulation", "reason_code", "funding", "bullish", V7TagActionEvidence, "Long/short ratio is balanced during accumulation, reducing crowding risk."),
	"sector_rotation_leader":                   tagDef("sector_rotation_leader", "reason_code", "flow", "bullish", V7TagActionEvidence, "Symbol belongs to a local theme with broad relative strength in rotation regime."),
	"correlation_floor_context":                tagDef("correlation_floor_context", "risk_tag", "tier", "neutral", V7TagActionContextOnly, "Signal was restored after theme de-duplication only to preserve minimum context."),
	"volume_expansion":                         tagDef("volume_expansion", "reason_code", "flow", "mixed", V7TagActionEvidence, "Volume expansion confirms participation; direction must be checked separately."),
	"oi_delta_missing_displacement":            tagDef("oi_delta_missing_displacement", "reason_code", "oi", "mixed", V7TagActionRequiredConfirm, "Displacement lacks OI delta; require OI/volume follow-through before open."),
	"displacement_extension_rr_valid":          tagDef("displacement_extension_rr_valid", "reason_code", "risk", "bullish", V7TagActionEvidence, "A later displacement continuation target restores minimum RR while the first target remains a partial/profit-management level."),
	"range_expansion_event":                    tagDef("range_expansion_event", "reason_code", "price", "mixed", V7TagActionEvidence, "24h amplitude plus short-term range expansion marks an event mover for directional review."),
	"amplitude_24h_extreme":                    tagDef("amplitude_24h_extreme", "reason_code", "price", "mixed", V7TagActionEvidence, "24h amplitude is extreme; candidate is active but must avoid chasing."),
	"amplitude_24h_major":                      tagDef("amplitude_24h_major", "reason_code", "price", "mixed", V7TagActionEvidence, "24h amplitude is major enough to require event-specific review."),
	"amplitude_24h_event":                      tagDef("amplitude_24h_event", "reason_code", "price", "mixed", V7TagActionEvidence, "24h amplitude is elevated and paired with short-term expansion."),
	"massive_range_expansion_event":            tagDef("massive_range_expansion_event", "reason_code", "price", "mixed", V7TagActionOpenSupport, "1h range expansion is massive; supports opening only after live direction, entry, stop, and RR checks."),
	"strong_range_expansion_event":             tagDef("strong_range_expansion_event", "reason_code", "price", "mixed", V7TagActionEvidence, "1h range expansion is strong and should be evaluated as an event mover."),
	"moderate_range_expansion_event":           tagDef("moderate_range_expansion_event", "reason_code", "price", "mixed", V7TagActionEvidence, "1h range expansion is moderate; requires more live confirmation."),
	"event_continuation_long":                  tagDef("event_continuation_long", "reason_code", "price", "bullish", V7TagActionOpenSupport, "High-amplitude event has bullish follow-through; verify entry and RR before opening long."),
	"event_breakdown_short":                    tagDef("event_breakdown_short", "reason_code", "price", "bearish", V7TagActionOpenSupport, "High-amplitude event has bearish breakdown follow-through; verify entry and RR before opening short."),
	"event_directional_followthrough":          tagDef("event_directional_followthrough", "reason_code", "price", "mixed", V7TagActionEvidence, "Event has directional follow-through but still needs live confirmation."),
	"event_followthrough_quality_insufficient": tagDef("event_followthrough_quality_insufficient", "reason_code", "flow", "mixed", V7TagActionWaitOnly, "Event direction exists, but OI/volume quality is insufficient for direct execution."),
	"volume_burst_15m":                         tagDef("volume_burst_15m", "reason_code", "flow", "mixed", V7TagActionEvidence, "15m volume burst confirms active participation in the event move."),
	"volume_burst_5m":                          tagDef("volume_burst_5m", "reason_code", "flow", "mixed", V7TagActionEvidence, "5m volume burst supports short-term event timing."),
	"taker_sell_aligned":                       tagDef("taker_sell_aligned", "reason_code", "flow", "bearish", V7TagActionEvidence, "Sell-side taker flow aligns with a short setup."),

	// Required confirmations.
	"15m_close_above_vwap_or_ema20_or_entry_zone_upper": tagDef("15m_close_above_vwap_or_ema20_or_entry_zone_upper", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long confirmation requires a 15m close above VWAP/EMA20 or entry-zone upper bound."),
	"15m_close_below_vwap_or_ema20_or_entry_zone_lower": tagDef("15m_close_below_vwap_or_ema20_or_entry_zone_lower", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short confirmation requires a 15m close below VWAP/EMA20 or entry-zone lower bound."),
	"live_price_in_entry_zone":                          tagDef("live_price_in_entry_zone", "required_confirmation", "confirmation", "mixed", V7TagActionRequiredConfirm, "Live price must remain inside the signal entry zone before opening."),
	"5m_close_above_ema20_or_entry_zone_mid":            tagDef("5m_close_above_ema20_or_entry_zone_mid", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long reversal needs a 5m close above EMA20 or above the entry-zone midpoint."),
	"5m_close_below_ema20_or_entry_zone_mid":            tagDef("5m_close_below_ema20_or_entry_zone_mid", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short reversal needs a 5m close below EMA20 or below the entry-zone midpoint."),
	"5m_or_15m_close_through_breakout_level":            tagDef("5m_or_15m_close_through_breakout_level", "required_confirmation", "confirmation", "mixed", V7TagActionRequiredConfirm, "Breakout entry needs either a 5m or 15m close through the breakout level."),
	"5m_or_15m_close_above_trigger":                     tagDef("5m_or_15m_close_above_trigger", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Fast long confirmation needs a 5m or 15m close above the trigger."),
	"5m_or_15m_close_below_trigger":                     tagDef("5m_or_15m_close_below_trigger", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Fast short confirmation needs a 5m or 15m close below the trigger."),
	"5m_or_15m_close_above_entry_zone":                  tagDef("5m_or_15m_close_above_entry_zone", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Breakout wait mode needs a 5m or 15m close above the entry-zone upper bound."),
	"5m_or_15m_rejection_at_resistance_or_entry_zone":   tagDef("5m_or_15m_rejection_at_resistance_or_entry_zone", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short rejection needs 5m or 15m failure at resistance or the entry zone."),
	"5m_or_15m_rejection_at_support_or_entry_zone":      tagDef("5m_or_15m_rejection_at_support_or_entry_zone", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long rejection/reclaim needs 5m or 15m defense at support or the entry zone."),
	"5m_or_15m_rejection_from_range_top":                tagDef("5m_or_15m_rejection_from_range_top", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Range short needs 5m or 15m rejection from the range top."),
	"5m_or_15m_reclaim_from_range_bottom":               tagDef("5m_or_15m_reclaim_from_range_bottom", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Range long needs 5m or 15m reclaim from the range bottom."),
	"5m_price_holds_ema20_or_trailing_support":          tagDef("5m_price_holds_ema20_or_trailing_support", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Momentum trailing entry needs 5m price to hold EMA20 or the provided trailing support."),
	"taker_buy_15m_gt_0_52":                             tagDef("taker_buy_15m_gt_0_52", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long needs 15m taker buy ratio above 0.52."),
	"taker_buy_15m_lt_0_48":                             tagDef("taker_buy_15m_lt_0_48", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short needs 15m taker buy ratio below 0.48."),
	"taker_buy_15m_lt_0_45":                             tagDef("taker_buy_15m_lt_0_45", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short reversal needs stronger sell flow: 15m taker buy ratio below 0.45."),
	"no_new_low_after_reclaim":                          tagDef("no_new_low_after_reclaim", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long reclaim remains valid only if price does not make a new low after reclaim."),
	"no_new_high_after_rejection":                       tagDef("no_new_high_after_rejection", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short rejection remains valid only if price does not make a new high after rejection."),
	"price_holds_trailing_support":                      tagDef("price_holds_trailing_support", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Momentum long must hold trailing support instead of breaking structure."),
	"momentum_not_exhausted":                            tagDef("momentum_not_exhausted", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Momentum setup must not show exhaustion before entry."),
	"taker_flow_not_flipping_against_direction":         tagDef("taker_flow_not_flipping_against_direction", "required_confirmation", "confirmation", "mixed", V7TagActionRequiredConfirm, "Taker flow must not flip against the signal direction."),
	"price_holds_vwap_or_trailing_support":              tagDef("price_holds_vwap_or_trailing_support", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Displacement long must hold VWAP or trailing support."),
	"taker_buy_15m_stays_above_0_52":                    tagDef("taker_buy_15m_stays_above_0_52", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Displacement long needs taker buy ratio to stay above 0.52."),
	"oi_delta_1h_positive_or_quote_volume_expands":      tagDef("oi_delta_1h_positive_or_quote_volume_expands", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "OI delta or quote volume must expand with price before opening displacement long."),
	"directional_15m_close_long":                        tagDef("directional_15m_close_long", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long setup needs a directionally supportive 15m close before opening."),
	"directional_15m_close_short":                       tagDef("directional_15m_close_short", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short setup needs a directionally supportive 15m close before opening."),
	"taker_flow_confirms_long":                          tagDef("taker_flow_confirms_long", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long setup needs taker flow to remain aligned before opening."),
	"taker_flow_confirms_short":                         tagDef("taker_flow_confirms_short", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short setup needs taker flow to remain aligned before opening."),
	"risk_level_not_extreme":                            tagDef("risk_level_not_extreme", "required_confirmation", "confirmation", "neutral", V7TagActionRequiredConfirm, "Risk level must remain below extreme before opening."),
}

func tagDef(tag, source, category, polarity, action, definition string) HunterV7TagDefinition {
	return HunterV7TagDefinition{
		Tag:        tag,
		Source:     source,
		Category:   category,
		Polarity:   polarity,
		LLMAction:  action,
		Definition: definition,
	}
}

// DescribeHunterV7Tags returns semantic definitions for tags present in a
// signal. Unknown tags are included as context-only entries so the LLM does not
// infer hidden open permission from an undefined label.
func DescribeHunterV7Tags(reasonCodes, riskTags, requiredConfirmations []string) []HunterV7TagDefinition {
	out := make([]HunterV7TagDefinition, 0, len(reasonCodes)+len(riskTags)+len(requiredConfirmations))
	seen := make(map[string]bool)
	add := func(tag, source string) {
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		if def, ok := hunterV7TagCatalog[tag]; ok {
			if def.Source == "" {
				def.Source = source
			}
			out = append(out, def)
			return
		}
		out = append(out, tagDef(tag, source, "unspecified", "neutral", V7TagActionUnknown, "No explicit catalog definition yet; treat as context only, not as open permission."))
	}
	for _, tag := range reasonCodes {
		add(tag, "reason_code")
	}
	for _, tag := range riskTags {
		add(tag, "risk_tag")
	}
	for _, tag := range requiredConfirmations {
		add(tag, "required_confirmation")
	}
	return out
}

// HunterV7TagLLMAction returns the catalogued LLM action for a tag.
// Unknown tags are context-only and must not be treated as execution blockers.
func HunterV7TagLLMAction(tag string) (string, bool) {
	def, ok := hunterV7TagCatalog[tag]
	if !ok {
		return V7TagActionUnknown, false
	}
	return def.LLMAction, true
}

// HunterV7PromptTagPolicy is a concise rule block for the LLM. The detailed
// per-signal definitions are emitted through tag_semantics.
func HunterV7PromptTagPolicy() string {
	return "Tag semantics: reason_codes are evidence, risk_tags constrain or block execution, and required_confirmations are not optional. llm_action=wait_only/reject_only blocks opening; reduce_size_or_wait allows opening only after live entry-zone, flow, stop, and RR confirmation with conservative size; reviewable_only_if_live_confirmed requires live entry-zone, flow, stop, and RR confirmation; unknown_context_only is context and never open permission."
}
