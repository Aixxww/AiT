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
// Evaluation logic itself still lives with each layer's data access
// (EvaluateV7Confirmations in the provider, hunterV7VerifyLiveConfirmation in
// the kernel); consistency between those evaluators and this registry is
// enforced by tests, and full evaluator unification is a later phase.

// V7ConfirmSpec describes how one required-confirmation code can be settled.
type V7ConfirmSpec struct {
	RefreshSatisfiable bool
	LiveReviewable     bool
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
	{"momentum_not_exhausted", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("momentum_not_exhausted", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Momentum setup must not show exhaustion before entry.")},
	{"taker_flow_confirms_long", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("taker_flow_confirms_long", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long setup needs taker flow to remain aligned before opening.")},
	{"taker_flow_confirms_short", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("taker_flow_confirms_short", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short setup needs taker flow to remain aligned before opening.")},
	{"taker_flow_not_flipping_against_direction", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
		tagDef("taker_flow_not_flipping_against_direction", "required_confirmation", "confirmation", "mixed", V7TagActionRequiredConfirm, "Taker flow must not flip against the signal direction.")},
	{"fresh_micro_confirmed", V7ConfirmSpec{RefreshSatisfiable: true, LiveReviewable: true},
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
	{"taker_buy_15m_gt_0_52", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("taker_buy_15m_gt_0_52", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long needs 15m taker buy ratio above 0.52.")},
	{"taker_buy_15m_lt_0_48", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("taker_buy_15m_lt_0_48", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short needs 15m taker buy ratio below 0.48.")},
	{"no_new_low_after_reclaim", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("no_new_low_after_reclaim", "required_confirmation", "confirmation", "bullish", V7TagActionRequiredConfirm, "Long reclaim remains valid only if price does not make a new low after reclaim.")},
	{"no_new_high_after_rejection", V7ConfirmSpec{RefreshSatisfiable: true},
		tagDef("no_new_high_after_rejection", "required_confirmation", "confirmation", "bearish", V7TagActionRequiredConfirm, "Short rejection remains valid only if price does not make a new high after rejection.")},
}

// v7ConfirmRegistry is derived from v7ConfirmVocab. Codes absent from it are
// context-only: no layer may claim to satisfy them automatically.
var v7ConfirmRegistry = map[string]V7ConfirmSpec{}

func init() {
	for _, entry := range v7ConfirmVocab {
		v7ConfirmRegistry[entry.code] = entry.spec
		hunterV7TagCatalog[entry.code] = entry.def
	}
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
