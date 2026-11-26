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

// The live verifier has no predicates of its own (#136): every code it can
// settle is one of local's shared confirmation predicates
// (local.V7ConfirmLiveVerifiable), evaluated here on facts built from the
// prompt cycle's market data. Scoring time and the trader's decision-time
// recheck call the same predicates, so a code cannot pass live at a weaker
// threshold than its scoring definition.

// hunterV7VerifyLiveConfirmation checks one confirmation against live prompt
// data. The second return reports whether this code is one we know how to
// verify at all with the data at hand; unknown codes are never treated as
// satisfied.
func hunterV7VerifyLiveConfirmation(coin CandidateCoin, data *market.Data, code string) (passed bool, known bool) {
	if data == nil {
		return false, false
	}
	price := data.CurrentPrice
	if price <= 0 && coin.V7PriceContext != nil {
		price = coin.V7PriceContext.Last
	}
	return HunterV7VerifyConfirmation(coin, data, price, code)
}

// HunterV7VerifyConfirmation evaluates a live-verifiable confirmation code's
// shared predicate for a candidate at price. Timeframe facts come from data
// when given (the prompt cycle's bars); with data nil they come from the
// candidate's scoring-time execution context (the trader's decision-time
// recheck, which holds no bars). known is false for a code without a shared
// predicate or when the predicate's inputs are unavailable.
func HunterV7VerifyConfirmation(coin CandidateCoin, data *market.Data, price float64, code string) (passed, known bool) {
	if !local.V7ConfirmLiveVerifiable(code) {
		return false, false
	}
	check, known, _ := local.V7EvaluateSharedConfirmation(code, HunterV7ConfirmFacts(coin, data, price))
	return known && check.Passed, known
}

// HunterV7ConfirmFacts builds the shared predicate facts for a candidate.
func HunterV7ConfirmFacts(coin CandidateCoin, data *market.Data, price float64) local.V7ConfirmFacts {
	if price <= 0 && coin.V7PriceContext != nil {
		price = coin.V7PriceContext.Last
	}
	f := local.V7ConfirmFacts{
		Direction: local.V7Direction(strings.ToUpper(strings.TrimSpace(coin.Direction))),
		SetupType: local.V7SetupType(coin.V7SetupType),
		Price:     price,
		EntryZone: coin.V7EntryZone,
	}
	if d := coin.V7DerivativesCtx; d != nil {
		f.TakerBuy15m = d.TakerBuy15m
		f.OIDelta1h, f.HasOIDelta1h = d.OIChange1h, true
	}
	if data == nil {
		if coin.V7ExecutionContext != nil {
			f.Timeframes = coin.V7ExecutionContext.Timeframes
		}
		return f
	}
	f.Timeframes = make(map[string]local.V7ExecutionTimeframeSummary, 2)
	for _, tf := range []string{"15m", "5m"} {
		series := data.TimeframeData[tf]
		if series == nil || len(series.Klines) == 0 {
			continue
		}
		bars := make([]local.V7Bar, len(series.Klines))
		volumes := make([]float64, len(series.Klines))
		for i, k := range series.Klines {
			bars[i] = local.V7Bar{High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume}
			volumes[i] = k.Volume
		}
		ema20 := 0.0
		if n := len(series.EMA20Values); n > 0 {
			ema20 = series.EMA20Values[n-1]
		}
		if summary, ok := local.V7TimeframeFactsFromBars(tf, bars, ema20); ok {
			f.Timeframes[tf] = summary
		}
		if tf == "15m" {
			f.VolumeBurst15m = local.V7VolumeBurst(volumes, local.V7VolumeBurst15mLookback)
		}
	}
	return f
}

func appendReasonCodeIfMissing(codes []string, code string) []string {
	for _, existing := range codes {
		if existing == code {
			return codes
		}
	}
	return append(codes, code)
}
