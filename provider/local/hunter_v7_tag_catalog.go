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
	"risk_filtered":                       tagDef("risk_filtered", "risk_tag", "risk", "neutral", V7TagActionRejectOnly, "Risk score exceeded the router filter; do not open."),
	"liquidity_filtered":                  tagDef("liquidity_filtered", "risk_tag", "risk", "neutral", V7TagActionRejectOnly, "Liquidity failed the router filter; do not open."),
	"extreme_volatility":                  tagDef("extreme_volatility", "risk_tag", "risk", "mixed", V7TagActionRejectOnly, "Volatility is extreme enough to invalidate normal execution assumptions."),
	"wash_volume_high":                    tagDef("wash_volume_high", "risk_tag", "risk", "neutral", V7TagActionRejectOnly, "Suspiciously high volume/trade-count pattern; avoid opening."),
	"do_not_market_chase":                 tagDef("do_not_market_chase", "risk_tag", "risk", "neutral", V7TagActionWaitOnly, "Move is extended; wait for re-entry instead of market chasing."),
	"funding_extreme":                     tagDef("funding_extreme", "risk_tag", "funding", "mixed", V7TagActionReduceOrWait, "Funding is extreme; crowded-direction entries are blocked, while counter-crowd panic reversals require live confirmation, smaller size, and RR validation."),
	"crowding_extreme":                    tagDef("crowding_extreme", "risk_tag", "funding", "mixed", V7TagActionWaitOnly, "Positioning is crowded; require unwind/reversal confirmation before any open."),
	"already_pumped_24h":                  tagDef("already_pumped_24h", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Long setup is late after a strong 24h pump; do not chase."),
	"funding_expensive":                   tagDef("funding_expensive", "risk_tag", "funding", "bullish", V7TagActionWaitOnly, "Long funding cost is too expensive for direct entry."),
	"lsr_extreme_long":                    tagDef("lsr_extreme_long", "risk_tag", "funding", "bearish", V7TagActionWaitOnly, "Long/short ratio shows extreme long crowding."),
	"taker_sell_during_accumulation":      tagDef("taker_sell_during_accumulation", "risk_tag", "flow", "bearish", V7TagActionWaitOnly, "Accumulation thesis conflicts with active sell flow."),
	"no_reclaim_signal":                   tagDef("no_reclaim_signal", "risk_tag", "price", "neutral", V7TagActionWaitOnly, "Reversal has not reclaimed a usable level; wait for reclaim."),
	"oi_up_price_down":                    tagDef("oi_up_price_down", "risk_tag", "oi", "bearish", V7TagActionWaitOnly, "Open interest rises while price falls; reversal long is not confirmed."),
	"not_near_short_retest_zone":          tagDef("not_near_short_retest_zone", "risk_tag", "price", "bearish", V7TagActionWaitOnly, "Short is away from the required retest/entry zone."),
	"not_near_long_reclaim_zone":          tagDef("not_near_long_reclaim_zone", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Long is away from the required reclaim/entry zone."),
	"late_short_after_deep_drop":          tagDef("late_short_after_deep_drop", "risk_tag", "price", "bearish", V7TagActionWaitOnly, "Short entry is late after a deep drop without fresh retest."),
	"short_after_fast_drop_without_flush": tagDef("short_after_fast_drop_without_flush", "risk_tag", "price", "bearish", V7TagActionWaitOnly, "Avoid shorting a fast drop before OI/crowding flush confirms."),
	"late_long_after_deep_pump":           tagDef("late_long_after_deep_pump", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Long entry is late after a deep pump."),
	"long_after_fast_pump_without_flush":  tagDef("long_after_fast_pump_without_flush", "risk_tag", "price", "bullish", V7TagActionWaitOnly, "Avoid chasing a fast pump without reset/flush."),
	"weak_4h_oi_flush":                    tagDef("weak_4h_oi_flush", "risk_tag", "oi", "mixed", V7TagActionWaitOnly, "4h OI flush is insufficient for funding-reversal short confirmation."),
	"oi_building_no_flush":                tagDef("oi_building_no_flush", "risk_tag", "oi", "mixed", V7TagActionWaitOnly, "OI is building without crowding flush; reversal needs more confirmation."),
	"momentum_confirmation_missing":       tagDef("momentum_confirmation_missing", "risk_tag", "flow", "bullish", V7TagActionWaitOnly, "Momentum setup lacks required near-term confirmation."),
	"momentum_overheated":                 tagDef("momentum_overheated", "risk_tag", "risk", "bullish", V7TagActionWaitOnly, "Momentum is overheated; wait for pullback/retest."),
	"momentum_chase_risk":                 tagDef("momentum_chase_risk", "risk_tag", "risk", "bullish", V7TagActionWaitOnly, "Momentum entry would be a chase unless re-entry conditions improve."),
	"needs_oi_confirmation":               tagDef("needs_oi_confirmation", "risk_tag", "oi", "mixed", V7TagActionRequiredConfirm, "OI data is missing or inconclusive; require OI/volume confirmation before open."),
	"displacement_rr_insufficient":        tagDef("displacement_rr_insufficient", "risk_tag", "risk", "bullish", V7TagActionRejectOnly, "Displacement target/stop geometry cannot provide minimum RR."),
	"rsi_extreme_with_crowded_funding":    tagDef("rsi_extreme_with_crowded_funding", "risk_tag", "risk", "bullish", V7TagActionWaitOnly, "RSI and funding are both crowded; wait for reset."),
	"execution_stop_tightened":            tagDef("execution_stop_tightened", "risk_tag", "risk", "neutral", V7TagActionReduceOrWait, "Router tightened stop to near structure; verify backend min stop and RR."),

	// Positive evidence tags used by tiering/prompt.
	"strong_reclaim":                tagDef("strong_reclaim", "reason_code", "price", "bullish", V7TagActionOpenSupport, "Clear reclaim after capitulation; supports panic-reversal long after RR checks."),
	"solid_reclaim":                 tagDef("solid_reclaim", "reason_code", "price", "bullish", V7TagActionEvidence, "Moderate reclaim evidence; still needs live confirmation."),
	"early_reclaim":                 tagDef("early_reclaim", "reason_code", "price", "bullish", V7TagActionEvidence, "Early reclaim signal; weaker than strong reclaim."),
	"selling_decelerating":          tagDef("selling_decelerating", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Sell pressure is slowing; supports reversal confirmation."),
	"selling_exhaustion":            tagDef("selling_exhaustion", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Sell pressure appears exhausted; supports reversal confirmation."),
	"taker_buy_aggressive":          tagDef("taker_buy_aggressive", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Aggressive buy taker flow; strong long confirmation."),
	"taker_buy_strong":              tagDef("taker_buy_strong", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Strong buy taker flow; supports long confirmation."),
	"taker_sustained_buy":           tagDef("taker_sustained_buy", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Sustained buy-side pressure; stronger than taker_strong_buy for momentum."),
	"taker_strong_buy":              tagDef("taker_strong_buy", "reason_code", "flow", "bullish", V7TagActionEvidence, "Buy flow is strong but not enough alone to override upper-zone chase risk."),
	"taker_weak_buy":                tagDef("taker_weak_buy", "reason_code", "flow", "bearish", V7TagActionWaitOnly, "Buy flow is weak; do not open long momentum/reversal unless later confirmed."),
	"confirmed_breakout":            tagDef("confirmed_breakout", "reason_code", "price", "bullish", V7TagActionOpenSupport, "Breakout has confirmed structurally; still verify entry/RR."),
	"taker_aggressive_buy":          tagDef("taker_aggressive_buy", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Aggressive buy flow confirms breakout/momentum."),
	"strong_4h_momentum":            tagDef("strong_4h_momentum", "reason_code", "price", "bullish", V7TagActionOpenSupport, "4h trend is strong enough to support selective momentum review."),
	"solid_4h_momentum":             tagDef("solid_4h_momentum", "reason_code", "price", "bullish", V7TagActionEvidence, "4h momentum exists but should not override chase/zone guards alone."),
	"strong_24h_momentum":           tagDef("strong_24h_momentum", "reason_code", "price", "bullish", V7TagActionEvidence, "24h momentum is strong; can also imply late-entry risk."),
	"micro_pullback":                tagDef("micro_pullback", "reason_code", "price", "bullish", V7TagActionEvidence, "Very shallow pullback in a trend; needs entry-zone check."),
	"shallow_pullback":              tagDef("shallow_pullback", "reason_code", "price", "bullish", V7TagActionEvidence, "Shallow pullback in a trend; needs entry-zone check."),
	"shallow_pullback_1h":           tagDef("shallow_pullback_1h", "reason_code", "price", "bullish", V7TagActionEvidence, "1h shallow pullback; do not treat upper-zone weakness as automatic entry."),
	"oi_massive_flush":              tagDef("oi_massive_flush", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Large OI flush supports capitulation/reversal."),
	"oi_heavy_flush":                tagDef("oi_heavy_flush", "reason_code", "oi", "bullish", V7TagActionOpenSupport, "Heavy OI flush supports capitulation/reversal."),
	"oi_declining":                  tagDef("oi_declining", "reason_code", "oi", "bullish", V7TagActionEvidence, "OI is declining; supports de-risking/flush context."),
	"oi_healthy_growth":             tagDef("oi_healthy_growth", "reason_code", "oi", "bullish", V7TagActionEvidence, "OI growth aligns with price; supports trend continuation."),
	"volume_expansion":              tagDef("volume_expansion", "reason_code", "flow", "mixed", V7TagActionEvidence, "Volume expansion confirms participation; direction must be checked separately."),
	"oi_delta_missing_displacement": tagDef("oi_delta_missing_displacement", "reason_code", "oi", "mixed", V7TagActionRequiredConfirm, "Displacement lacks OI delta; require OI/volume follow-through before open."),

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
