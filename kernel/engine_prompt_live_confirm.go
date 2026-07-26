package kernel

import (
	"strings"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
)

// Hunter v7 required confirmations that can only be judged against a live
// short-timeframe close used to sit unresolved until the trader refreshed the
// symbol at decision time — which happens *after* the LLM has already been
// shown the candidate. A signal whose only gap was that timing therefore never
// reached EXECUTABLE, no matter how strong it was.
//
// The prompt cycle already loads klines and indicators for every candidate, so
// those confirmations can be settled here instead, against data we are holding
// anyway. No extra network calls, and the tier is decided with the same
// evidence the trader would later use.
//
// Verification is deliberately conservative: a confirmation is only cleared
// when its inputs are present and unambiguous. Anything unrecognised or
// missing data is left untouched, so this can add executable candidates but
// never launder an unverified one.

// hunterV7ApplyLiveConfirmations settles the refresh-satisfiable required
// confirmations for a candidate and returns the updated copy.
func hunterV7ApplyLiveConfirmations(coin CandidateCoin, data *market.Data) CandidateCoin {
	if data == nil || coin.V7SetupType == "" || len(coin.V7RequiredConfirms) == 0 {
		return coin
	}
	summary := coin.V7ConfirmSummary
	if summary == nil {
		return coin
	}

	required := make(map[string]struct{}, len(coin.V7RequiredConfirms))
	for _, code := range coin.V7RequiredConfirms {
		if code != "" {
			required[code] = struct{}{}
		}
	}
	if len(required) == 0 {
		return coin
	}

	cleared := make([]string, 0, 4)
	clear := func(code string) bool {
		if _, ok := required[code]; !ok {
			return false
		}
		// The verifier itself is the safety gate: it reports known=false for any
		// code it cannot evaluate from the data at hand, so an unrecognised or
		// under-specified confirmation is never cleared.
		passed, known := hunterV7VerifyLiveConfirmation(coin, data, code)
		if known && passed {
			cleared = append(cleared, code)
		}
		return known && passed
	}

	// Work on a deep copy: the summary is shared by pointer with the candidate
	// slice the caller still owns.
	updated := *summary
	updated.MissingReview = hunterV7FilterChecks(summary.MissingReview, clear)
	updated.ContextChecks = hunterV7PassChecks(summary.ContextChecks, clear)
	if len(cleared) == 0 {
		return coin
	}
	if len(updated.MissingReview) == 0 {
		updated.PassedReview = true
	}

	coin.V7ConfirmSummary = &updated
	for _, code := range cleared {
		coin.V7ReasonCodes = appendReasonCodeIfMissing(coin.V7ReasonCodes, "live_confirmed_"+code)
	}
	// Deliberately NOT minted here: fresh_micro_confirmed / fresh_rest_confirmed.
	// Those codes attest that the trader's decision-time orderbook/REST refresh
	// ran, and the trader guard is their single owner
	// (validateHunterV7MicroRefresh / validateHunterV7RESTMicroRefresh). This
	// pass settles individual confirmations from prompt-cycle data and says so
	// with per-code live_confirmed_* stamps; claiming the umbrella codes as
	// well would tell downstream readers a refresh happened that never did.
	return coin
}

// hunterV7FilterChecks drops checks that the live pass has settled.
func hunterV7FilterChecks(checks []local.V7ConfirmationCheck, clear func(string) bool) []local.V7ConfirmationCheck {
	if len(checks) == 0 {
		return checks
	}
	kept := make([]local.V7ConfirmationCheck, 0, len(checks))
	for _, check := range checks {
		if clear(check.Code) {
			continue
		}
		kept = append(kept, check)
	}
	return kept
}

// hunterV7PassChecks marks context checks that the live pass has settled.
func hunterV7PassChecks(checks []local.V7ConfirmationCheck, clear func(string) bool) []local.V7ConfirmationCheck {
	if len(checks) == 0 {
		return checks
	}
	updated := make([]local.V7ConfirmationCheck, len(checks))
	copy(updated, checks)
	for i := range updated {
		if updated[i].Passed {
			continue
		}
		if clear(updated[i].Code) {
			updated[i].Passed = true
			updated[i].Reason = "live_confirmed_from_prompt_market_data"
		}
	}
	return updated
}

// hunterV7LiveConfirmVerifiers maps each confirmation code the kernel can
// settle pre-prompt to its verifier. Membership is data so a test can assert
// every key is a registered confirmation code — the class of bug where a
// verifier exists for a code no module produces (taker_buy_15m_gt_0_50 was one
// such dead branch) or a producible code silently lacks a verifier.
var hunterV7LiveConfirmVerifiers = map[string]func(coin CandidateCoin, data *market.Data, isLong bool, price float64) (passed, known bool){
	"live_price_in_entry_zone": func(coin CandidateCoin, _ *market.Data, _ bool, price float64) (bool, bool) {
		zone := coin.V7EntryZone
		if price <= 0 || zone.Lower <= 0 || zone.Upper <= zone.Lower {
			return false, false
		}
		return price >= zone.Lower && price <= zone.Upper, true
	},

	"taker_buy_15m_gt_0_52": func(coin CandidateCoin, _ *market.Data, _ bool, _ float64) (bool, bool) {
		return hunterV7TakerAtLeast(coin, 0.52)
	},
	"taker_buy_15m_lt_0_48": func(coin CandidateCoin, _ *market.Data, _ bool, _ float64) (bool, bool) {
		return hunterV7TakerAtMostLive(coin, 0.48)
	},
	"taker_buy_15m_lt_0_45": func(coin CandidateCoin, _ *market.Data, _ bool, _ float64) (bool, bool) {
		return hunterV7TakerAtMostLive(coin, 0.45)
	},

	"taker_flow_not_flipping_against_direction": func(coin CandidateCoin, _ *market.Data, isLong bool, _ float64) (bool, bool) {
		taker, ok := hunterV7LiveTakerBuy(coin)
		if !ok {
			return false, false
		}
		if isLong {
			return taker >= 0.50, true
		}
		return taker <= 0.50, true
	},
	"taker_flow_confirms_long": func(coin CandidateCoin, _ *market.Data, _ bool, _ float64) (bool, bool) {
		return hunterV7TakerAtLeast(coin, 0.52)
	},
	"taker_flow_confirms_short": func(coin CandidateCoin, _ *market.Data, _ bool, _ float64) (bool, bool) {
		return hunterV7TakerAtMostLive(coin, 0.48)
	},

	"5m_price_holds_ema20_or_trailing_support": func(_ CandidateCoin, data *market.Data, isLong bool, _ float64) (bool, bool) {
		return hunterV7CloseHoldsEMA20(data, "5m", isLong)
	},
	"directional_15m_close_long": func(_ CandidateCoin, data *market.Data, _ bool, _ float64) (bool, bool) {
		return hunterV7CloseHoldsEMA20(data, "15m", true)
	},
	"directional_15m_close_short": func(_ CandidateCoin, data *market.Data, _ bool, _ float64) (bool, bool) {
		return hunterV7CloseHoldsEMA20(data, "15m", false)
	},

	"oi_delta_1h_positive_or_quote_volume_expands": func(coin CandidateCoin, _ *market.Data, _ bool, _ float64) (bool, bool) {
		if coin.V7DerivativesCtx == nil {
			return false, false
		}
		return coin.V7DerivativesCtx.OIChange1h > 0, true
	},
}

// hunterV7VerifyLiveConfirmation checks one confirmation against live prompt
// data. The second return reports whether this code is one we know how to
// verify at all; unknown codes are never treated as satisfied.
func hunterV7VerifyLiveConfirmation(coin CandidateCoin, data *market.Data, code string) (passed bool, known bool) {
	verifier, ok := hunterV7LiveConfirmVerifiers[code]
	if !ok {
		return false, false
	}
	isLong := strings.EqualFold(coin.Direction, "LONG")
	price := data.CurrentPrice
	if price <= 0 && coin.V7PriceContext != nil {
		price = coin.V7PriceContext.Last
	}
	return verifier(coin, data, isLong, price)
}

func hunterV7LiveTakerBuy(coin CandidateCoin) (float64, bool) {
	if coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.TakerBuy15m <= 0 {
		return 0, false
	}
	return coin.V7DerivativesCtx.TakerBuy15m, true
}

// Boundary semantics are inclusive (>=/<=), matching the provider's
// v7TakerConfirmationCheck and the tier classifier's hunterV7TakerBuyAtLeast.
// The strict > used here previously meant a reading of exactly 0.52 failed
// taker_buy_15m_gt_0_52 in this pass while passing every other gate on the
// same request path.
func hunterV7TakerAtLeast(coin CandidateCoin, threshold float64) (bool, bool) {
	taker, ok := hunterV7LiveTakerBuy(coin)
	if !ok {
		return false, false
	}
	return taker >= threshold, true
}

func hunterV7TakerAtMostLive(coin CandidateCoin, threshold float64) (bool, bool) {
	taker, ok := hunterV7LiveTakerBuy(coin)
	if !ok {
		return false, false
	}
	return taker <= threshold, true
}

// hunterV7CloseHoldsEMA20 checks the latest close of a timeframe against its
// EMA20, which is what "price holds support" means for a long and the mirror
// for a short.
func hunterV7CloseHoldsEMA20(data *market.Data, timeframe string, isLong bool) (bool, bool) {
	if data == nil || data.TimeframeData == nil {
		return false, false
	}
	tf, ok := data.TimeframeData[timeframe]
	if !ok || tf == nil || len(tf.Klines) == 0 || len(tf.EMA20Values) == 0 {
		return false, false
	}
	close := tf.Klines[len(tf.Klines)-1].Close
	ema := tf.EMA20Values[len(tf.EMA20Values)-1]
	if close <= 0 || ema <= 0 {
		return false, false
	}
	if isLong {
		return close >= ema, true
	}
	return close <= ema, true
}

func appendReasonCodeIfMissing(codes []string, code string) []string {
	for _, existing := range codes {
		if existing == code {
			return codes
		}
	}
	return append(codes, code)
}
