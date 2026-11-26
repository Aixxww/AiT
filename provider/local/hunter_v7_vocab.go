package local

// ============================================================================
// Hunter v7 — Typed confirmation vocabulary (U1.1, lean-core redesign)
// ============================================================================
// Required-confirmation codes used to live in four independent switch
// statements (provider evaluator, kernel live-review whitelist, kernel live
// verifier, trader refresh whitelist) that covered 30/11/12/18 codes each.
// Adding a code meant remembering all four sites; forgetting one produced a
// confirmation that one layer required and another could never satisfy.
//
// This registry is the single membership source. Each spec says how a code can
// be settled:
//
//	RefreshSatisfiable — the trader's decision-time REST/orderbook refresh can
//	                     satisfy it (was: hunterV7ConfirmationCanBeSatisfiedByRefresh)
//	LiveReviewable     — a candidate missing only this code may drop to
//	                     REVIEWABLE instead of WATCH, for live re-checking
//	                     (was: hunterV7ConfirmationCanBeLiveReviewed)
//
// Evaluation (#136): every code verified both at scoring time and against
// live data has ONE shared predicate (hunter_v7_confirm_predicates.go) that
// the scoring evaluator, the kernel live verifier and the trader's refresh
// recheck all call. LiveReviewable is not hand-picked: it must equal
// (live-verifiable AND RefreshSatisfiable) OR RefreshOwned, which
// TestV7ConfirmLiveReviewableMatchesVerifiers enforces.

// V7ConfirmSpec describes how one required-confirmation code can be settled.
type V7ConfirmSpec struct {
	RefreshSatisfiable bool
	LiveReviewable     bool
	// RefreshOwned (#136) marks a code whose verifier is the trader's
	// decision-time refresh itself (fresh_micro_confirmed): no predicate can
	// settle it earlier, and the refresh guard is its single owner.
	RefreshOwned bool
	// Checklist (#133) marks an LLM-checklist-only code: the prompt shows it
	// in required_confirmations for the LLM to verify on the bars, but no
	// backend layer checks or enforces it (it never blocks a tier or an
	// open). Every other registered code is machine-verified.
	Checklist bool
}

// v7ConfirmVocab is the single home for every settleable confirmation code
// (U5.5): one row carries both the cross-layer settlement flags and the
// prompt-facing semantic definition. init() feeds the settlement registry and
// the tag catalog from it, so a code cannot exist in one and not the other.
var v7ConfirmVocab = []struct {
	code string
	spec V7ConfirmSpec
	def  HunterV7TagDefinition
}{
	{"directional_15m_close_long", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("directional_15m_close_long", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long setup needs a directionally supportive 15m close before opening.")},
	{"directional_15m_close_short", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("directional_15m_close_short", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short setup needs a directionally supportive 15m close before opening.")},
	{"5m_or_15m_close_through_breakout_level", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("5m_or_15m_close_through_breakout_level", "required_confirmation", "confirmation", "mixed", V7TagActionRequiredConfirm, "Breakout entry needs either a 5m or 15m close through the breakout level.")},
	{"5m_or_15m_close_above_trigger", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("5m_or_15m_close_above_trigger", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Fast long confirmation needs a 5m or 15m close above the trigger.")},
	{"5m_or_15m_close_below_trigger", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("5m_or_15m_close_below_trigger", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Fast short confirmation needs a 5m or 15m close below the trigger.")},
	{"5m_price_holds_ema20_or_trailing_support", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("5m_price_holds_ema20_or_trailing_support", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Momentum trailing entry needs 5m price to hold EMA20 or the provided trailing support.")},
	{"momentum_not_exhausted", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("momentum_not_exhausted", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Momentum setup must not show exhaustion before entry.")},
	{"taker_flow_confirms_long", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("taker_flow_confirms_long", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long setup needs taker flow to remain aligned before opening.")},
	{"taker_flow_confirms_short", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("taker_flow_confirms_short", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short setup needs taker flow to remain aligned before opening.")},
	{"taker_flow_not_flipping_against_direction", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("taker_flow_not_flipping_against_direction", "required_confirmation", "confirmation", "mixed", V7TagActionRequiredConfirm, "Taker flow must not flip against the signal direction.")},
	{"fresh_micro_confirmed", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true, RefreshOwned: true},
		tagDef("fresh_micro_confirmed", "required_confirmation", "confirmation", "neutral", V7TagActionRequiredConfirm, "Latest symbol-level micro refresh confirms price, flow, entry zone, and RR after the original signal.")},
	{"live_price_in_entry_zone", V7ConfirmSpec{},
		tagDef("live_price_in_entry_zone", "required_confirmation", "confirmation", "mixed", V7TagActionRequiredConfirm, "Live price must remain inside the signal entry zone before opening.")},
	{"taker_buy_15m_lt_0_45", V7ConfirmSpec{},
		tagDef("taker_buy_15m_lt_0_45", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short reversal needs stronger sell flow: 15m taker buy ratio below 0.45.")},
	{"oi_delta_1h_positive_or_quote_volume_expands", V7ConfirmSpec{},
		tagDef("oi_delta_1h_positive_or_quote_volume_expands", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "OI delta or quote volume must expand with price before opening displacement long.")},
	{"15m_close_above_vwap_or_ema20_or_entry_zone_upper", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("15m_close_above_vwap_or_ema20_or_entry_zone_upper", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long confirmation requires a 15m close above VWAP/EMA20 or entry-zone upper bound.")},
	{"15m_close_below_vwap_or_ema20_or_entry_zone_lower", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("15m_close_below_vwap_or_ema20_or_entry_zone_lower", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short confirmation requires a 15m close below VWAP/EMA20 or entry-zone lower bound.")},
	{"taker_buy_15m_gt_0_52", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("taker_buy_15m_gt_0_52", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long needs 15m taker buy ratio above 0.52.")},
	{"taker_buy_15m_lt_0_48", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("taker_buy_15m_lt_0_48", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short needs 15m taker buy ratio below 0.48.")},
	{"no_new_low_after_reclaim", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("no_new_low_after_reclaim", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long reclaim remains valid only if price does not make a new low after reclaim.")},
	{"no_new_high_after_rejection", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("no_new_high_after_rejection", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short rejection remains valid only if price does not make a new high after rejection.")},
}

// v7ConfirmCodes registers the remaining emitted codes (#133 / naming
// F-02). Machine rows are evaluated by evaluateV7KnownConfirmation; their
// catalog definitions live in the tag catalog (def nil) unless given here.
// Checklist rows are LLM-only (see V7ConfirmSpec.Checklist).
var v7ConfirmCodes = []struct {
	code string
	spec V7ConfirmSpec
	def  *HunterV7TagDefinition
}{
	// Machine-verified (evaluator cases in hunter_v7_confirmation.go).
	{"5m_close_above_ema20_or_entry_zone_mid", V7ConfirmSpec{}, nil},
	{"5m_close_below_ema20_or_entry_zone_mid", V7ConfirmSpec{}, nil},
	{"5m_or_15m_close_above_entry_zone", V7ConfirmSpec{}, nil},
	{"5m_or_15m_reclaim_from_range_bottom", V7ConfirmSpec{}, nil},
	{"5m_or_15m_rejection_at_resistance_or_entry_zone", V7ConfirmSpec{}, nil},
	{"5m_or_15m_rejection_at_support_or_entry_zone", V7ConfirmSpec{}, nil},
	{"5m_or_15m_rejection_from_range_top", V7ConfirmSpec{}, nil},
	{"15m_rejection_at_resistance_or_entry_zone", V7ConfirmSpec{}, nil},
	{"15m_close_below_vwap_or_ema20", V7ConfirmSpec{}, nil},
	{"15m_close_below_vwap", V7ConfirmSpec{}, nil},
	{"15m_reclaim_vwap", V7ConfirmSpec{}, nil},
	{"price_holds_vwap_or_trailing_support", V7ConfirmSpec{}, nil},
	{"no_new_high_after_crowding_signal", V7ConfirmSpec{}, nil},
	{"no_new_low_after_crowding_signal", V7ConfirmSpec{}, nil},
	{"oi_or_volume_expands_with_price", V7ConfirmSpec{}, nil},
	{"risk_level_not_extreme", V7ConfirmSpec{}, nil},
	{"taker_buy_15m_stays_above_0_52", V7ConfirmSpec{}, nil},
	{"lsr_turning_up", V7ConfirmSpec{}, nil},
	{"oi_stabilize", V7ConfirmSpec{}, nil},
	{"15m_close_above_vwap_or_ema20", V7ConfirmSpec{}, v7ConfirmDef("15m_close_above_vwap_or_ema20", "bullish", "Long needs a 15m close above VWAP20 or EMA20.")},
	{"bb_width_expansion_starts", V7ConfirmSpec{}, v7ConfirmDef("bb_width_expansion_starts", "neutral", "Bollinger band width must start expanding before entry.")},
	{"no_new_high_after_reversal_signal", V7ConfirmSpec{}, v7ConfirmDef("no_new_high_after_reversal_signal", "bearish", "Short reversal remains valid only if 5m/15m make no new high after the reversal signal.")},
	{"no_new_low_after_rejection", V7ConfirmSpec{}, v7ConfirmDef("no_new_low_after_rejection", "bullish", "Long rejection remains valid only if 5m/15m make no new low after the rejection.")},
	{"no_new_low_after_reversal_signal", V7ConfirmSpec{}, v7ConfirmDef("no_new_low_after_reversal_signal", "bullish", "Long reversal remains valid only if 5m/15m make no new low after the reversal signal.")},
	{"oi_1h_remains_positive_after_breakout", V7ConfirmSpec{}, v7ConfirmDef("oi_1h_remains_positive_after_breakout", "bullish", "1h open interest change must stay positive after the breakout.")},
	{"oi_continues_inflow", V7ConfirmSpec{}, v7ConfirmDef("oi_continues_inflow", "bullish", "Open interest (or short-term volume) must keep expanding.")},

	// LLM checklist only: no backend check. Entry/stop placement
	// instructions, time-based conditions, crowding and OI-state judgements
	// with no machine definition, and the WATCH-only conflict / pre-move
	// rows (which never open).
	{"enter_only_near_range_bottom", v7Checklist, v7ChecklistDef("enter_only_near_range_bottom", "bullish", "Range long: enter only near the range bottom.")},
	{"enter_only_near_range_top", v7Checklist, v7ChecklistDef("enter_only_near_range_top", "bearish", "Range short: enter only near the range top.")},
	{"stop_below_range_low", v7Checklist, v7ChecklistDef("stop_below_range_low", "bullish", "Range long: place the stop below the range low.")},
	{"stop_above_range_high", v7Checklist, v7ChecklistDef("stop_above_range_high", "bearish", "Range short: place the stop above the range high.")},
	{"no_failed_breakout_back_inside_range", v7Checklist, nil},
	{"no_immediate_loss_of_trigger_level", v7Checklist, nil},
	{"no_immediate_reclaim_of_trigger_level", v7Checklist, nil},
	{"funding_remains_negative_or_lsr_crowded_short", v7Checklist, nil},
	{"funding_remains_positive_or_lsr_crowded_long", v7Checklist, nil},
	{"oi_flush_or_failed_oi_rebuild", v7Checklist, nil},
	{"oi_stabilizes_or_short_covering_starts", v7Checklist, nil},
	{"oi_stops_building_or_short_covering_starts", v7Checklist, v7ChecklistDef("oi_stops_building_or_short_covering_starts", "bearish", "Open interest must stop building, or short covering must start, before entry.")},
	{"15m_close_above_bb_upper_or_entry_zone", v7Checklist, v7ChecklistDef("15m_close_above_bb_upper_or_entry_zone", "bullish", "Watch setup: a 15m close above the upper Bollinger band or the entry zone.")},
	{"15m_close_above_bb_upper_or_4h_resistance", v7Checklist, v7ChecklistDef("15m_close_above_bb_upper_or_4h_resistance", "bullish", "Watch setup: a 15m close above the upper Bollinger band or 4h resistance.")},
	{"5m_close_above_trigger", v7Checklist, v7ChecklistDef("5m_close_above_trigger", "bullish", "Watch setup: a 5m close above the trigger.")},
	{"15m_breakout_close", v7Checklist, v7ChecklistDef("15m_breakout_close", "bullish", "Watch setup: a 15m close through the breakout level.")},
	{"wait_for_directional_break", v7Checklist, nil},
	{"long_trigger: break_high_with_oi_increase", v7Checklist, nil},
	{"short_trigger: lose_vwap_with_taker_sell", v7Checklist, nil},
}

var v7Checklist = V7ConfirmSpec{Checklist: true}

func v7ConfirmDef(code, direction, def string) *HunterV7TagDefinition {
	d := tagDef(code, "required_confirmation", "confirmation", direction, V7TagActionRequiredConfirm, def)
	return &d
}

func v7ChecklistDef(code, direction, def string) *HunterV7TagDefinition {
	return v7ConfirmDef(code, direction, def+" LLM checklist: verify it on the bars; the backend does not check it.")
}

// v7ConfirmRegistry is derived from v7ConfirmVocab. Codes absent from it are
// context-only: no layer may claim to satisfy them automatically.
var v7ConfirmRegistry = map[string]V7ConfirmSpec{}

func init() {
	for _, entry := range v7ConfirmVocab {
		v7ConfirmRegistry[entry.code] = entry.spec
		hunterV7TagCatalog[entry.code] = entry.def
	}
	for _, entry := range v7ConfirmCodes {
		v7ConfirmRegistry[entry.code] = entry.spec
		if entry.def != nil {
			hunterV7TagCatalog[entry.code] = *entry.def
		}
	}
}

// V7ConfirmMachineVerifiable reports whether a backend layer checks this
// code (#133): registered and not checklist-only. Only these codes can
// keep a candidate in WATCH or block an open; any other code (checklist
// or unregistered) is an LLM checklist item and never blocks.
func V7ConfirmMachineVerifiable(code string) bool {
	spec, ok := v7ConfirmRegistry[code]
	return ok && !spec.Checklist
}

// V7MachineConfirms returns the machine-verifiable codes of a list, in
// order.
func V7MachineConfirms(codes []string) []string {
	var out []string
	for _, c := range codes {
		if c != "" && V7ConfirmMachineVerifiable(c) {
			out = append(out, c)
		}
	}
	return out
}

// V7ConfirmRefreshSatisfiable reports whether the trader's decision-time
// refresh can settle this confirmation code.
func V7ConfirmRefreshSatisfiable(code string) bool {
	return v7ConfirmRegistry[code].RefreshSatisfiable
}

// V7ConfirmLiveReviewable reports whether a candidate missing only this code
// may be tiered REVIEWABLE for live re-checking instead of WATCH.
func V7ConfirmLiveReviewable(code string) bool {
	return v7ConfirmRegistry[code].LiveReviewable
}

// V7ConfirmKnown reports whether a confirmation code is registered at all.
// Layers use this to reject verifiers or producers for unregistered codes.
func V7ConfirmKnown(code string) bool {
	_, ok := v7ConfirmRegistry[code]
	return ok
}
