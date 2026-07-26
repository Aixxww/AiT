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

// v7ConfirmRegistry lists every confirmation code with cross-layer settlement
// semantics. Codes absent from this map are context-only: no layer may claim
// to satisfy them automatically.
var v7ConfirmRegistry = map[string]V7ConfirmSpec{
	// Short-timeframe closes and structure checks — verifiable against live
	// klines in both the kernel pre-prompt pass and the trader refresh.
	"directional_15m_close_long":                {RefreshSatisfiable: true, LiveReviewable: true},
	"directional_15m_close_short":               {RefreshSatisfiable: true, LiveReviewable: true},
	"5m_or_15m_close_through_breakout_level":    {RefreshSatisfiable: true, LiveReviewable: true},
	"5m_or_15m_close_above_trigger":             {RefreshSatisfiable: true, LiveReviewable: true},
	"5m_or_15m_close_below_trigger":             {RefreshSatisfiable: true, LiveReviewable: true},
	"5m_price_holds_ema20_or_trailing_support":  {RefreshSatisfiable: true, LiveReviewable: true},
	"momentum_not_exhausted":                    {RefreshSatisfiable: true, LiveReviewable: true},
	"taker_flow_confirms_long":                  {RefreshSatisfiable: true, LiveReviewable: true},
	"taker_flow_confirms_short":                 {RefreshSatisfiable: true, LiveReviewable: true},
	"taker_flow_not_flipping_against_direction": {RefreshSatisfiable: true, LiveReviewable: true},
	"fresh_micro_confirmed":                     {RefreshSatisfiable: true, LiveReviewable: true},

	// Known codes with no automatic settlement path yet: the kernel can verify
	// them against live prompt data, but neither the refresh whitelist nor the
	// reviewable-promotion path claims them. Registered so verifier coverage
	// is checkable; flags deliberately zero to preserve current behavior.
	"live_price_in_entry_zone":                     {},
	"taker_buy_15m_lt_0_45":                        {},
	"oi_delta_1h_positive_or_quote_volume_expands": {},

	// Refresh-only codes: the trader can settle them at decision time, but a
	// missing one does not by itself qualify a candidate for REVIEWABLE.
	"15m_close_above_vwap_or_ema20_or_entry_zone_upper": {RefreshSatisfiable: true},
	"15m_close_below_vwap_or_ema20_or_entry_zone_lower": {RefreshSatisfiable: true},
	"taker_buy_15m_gt_0_52":                             {RefreshSatisfiable: true},
	"taker_buy_15m_lt_0_48":                             {RefreshSatisfiable: true},
	"no_new_low_after_reclaim":                          {RefreshSatisfiable: true},
	"no_new_high_after_rejection":                       {RefreshSatisfiable: true},
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
