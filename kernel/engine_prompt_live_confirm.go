package kernel

import (
	"strings"
	"time"

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
// confirmations for a candidate at time.Now() and returns the updated copy.
func hunterV7ApplyLiveConfirmations(coin CandidateCoin, data *market.Data) CandidateCoin {
	return hunterV7ApplyLiveConfirmationsAt(coin, data, time.Now())
}

// hunterV7ApplyLiveConfirmationsAt settles required confirmations against
// strictly closed bars at at (the cycle's prompt time, U5/M2b) and stamps both
// the closed-bar confirmation timeframes and per-check/summary bar evidence.
func hunterV7ApplyLiveConfirmationsAt(coin CandidateCoin, data *market.Data, at time.Time) CandidateCoin {
	if data == nil || coin.V7SetupType == "" {
		return coin
	}
	price := data.CurrentPrice
	if price <= 0 && coin.V7PriceContext != nil {
		price = coin.V7PriceContext.Last
	}
	var priorTFs map[string]local.V7ExecutionTimeframeSummary
	if coin.V7ConfirmTimeframes != nil {
		priorTFs = coin.V7ConfirmTimeframes
	} else if coin.V7ExecutionContext != nil && coin.V7ExecutionContext.Timeframes != nil {
		priorTFs = coin.V7ExecutionContext.Timeframes
	}
	facts := HunterV7ConfirmFactsAt(coin, data, price, at)
	// Stamp the per-timeframe merged closed-bar confirmation summaries whenever
	// prompt market data carries 5m or 15m series (preserving the absent
	// timeframe's scoring summary, and removing a present timeframe that yields
	// zero closed bars) so write-back gives the guard the exact same bar facts.
	// If data carries neither series, preserve scoring's closed timeframes.
	if data.TimeframeData["5m"] != nil || data.TimeframeData["15m"] != nil {
		coin.V7ConfirmTimeframes = cloneV7TimeframeMap(facts.Timeframes)
	}

	if len(coin.V7RequiredConfirms) == 0 {
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

	// Work on a deep copy: the summary is shared by pointer with the candidate
	// slice the caller still owns.
	updated := summary.Clone()
	cleared := make([]string, 0, 4)
	demoted := make([]string, 0, 2)
	evidenceChanged := false
	seenRequired := make(map[string]struct{}, len(required))
	for _, cur := range updated.MissingHard {
		if _, isReq := required[cur.Code]; isReq {
			seenRequired[cur.Code] = struct{}{}
		}
	}

	evalCode := func(code string) (check local.V7ConfirmationCheck, known, ok bool) {
		if _, isReq := required[code]; !isReq || !local.V7ConfirmLiveVerifiable(code) {
			return local.V7ConfirmationCheck{}, false, false
		}
		seenRequired[code] = struct{}{}
		// For bar/trigger codes, do not re-evaluate (nor overwrite scoring-time
		// BarEvidence with DecidingTF="none") when prompt data did not supply
		// any series for the code's timeframe(s).
		if barTFs := local.V7ConfirmBarTimeframes(code); len(barTFs) > 0 && !hunterV7DataHasFreshTimeframe(data, barTFs) {
			return local.V7ConfirmationCheck{}, false, false
		}
		truncated := hunterV7CodeIndicatorTruncated(code, facts.Timeframes, priorTFs)
		check, known, ok = local.V7EvaluateSharedConfirmation(code, facts)
		if truncated && (!ok || !known || !check.Passed) {
			// Fresh series has closed bar(s) but lacks an indicator (EMA20/VWAP20)
			// that prior scoring facts had: keep scoring's verdict and BarEvidence
			// rather than clobbering them with truncated-window output.
			return local.V7ConfirmationCheck{}, false, false
		}
		return check, known, ok
	}

	if len(updated.MissingReview) > 0 {
		kept := make([]local.V7ConfirmationCheck, 0, len(updated.MissingReview))
		for _, cur := range updated.MissingReview {
			check, known, ok := evalCode(cur.Code)
			if !ok {
				kept = append(kept, cur)
				continue
			}
			if known && check.Evidence != nil {
				ev := *check.Evidence
				updated.UpsertBarEvidence(ev)
				evidenceChanged = true
			}
			if known && check.Passed {
				cleared = append(cleared, cur.Code)
				continue
			}
			if known {
				cur.Actual = check.Actual
				if check.Threshold != 0 {
					cur.Threshold = check.Threshold
				}
				if check.Reason != "" {
					cur.Reason = check.Reason
				}
				if check.Evidence != nil {
					ev := *check.Evidence
					cur.Evidence = &ev
				}
			}
			kept = append(kept, cur)
		}
		updated.MissingReview = kept
	}

	for i := range updated.ContextChecks {
		if updated.ContextChecks[i].Passed {
			continue
		}
		check, known, ok := evalCode(updated.ContextChecks[i].Code)
		if !ok {
			continue
		}
		if known && check.Evidence != nil {
			ev := *check.Evidence
			updated.UpsertBarEvidence(ev)
			updated.ContextChecks[i].Evidence = &ev
			evidenceChanged = true
		}
		if known && check.Passed {
			updated.ContextChecks[i].Passed = true
			updated.ContextChecks[i].Reason = "live_confirmed_from_prompt_market_data"
			cleared = append(cleared, updated.ContextChecks[i].Code)
		}
	}

	// Two-way live re-check for already-passed required bar/trigger codes
	// (U5b Fix 4): when prompt data supplied fresh series for the code's
	// timeframe(s), re-evaluate against the fresh closed-bar facts. A still-
	// passing check updates BarEvidence; a known failure demotes the code into
	// MissingReview (setting PassedReview=false and upserting failing evidence).
	// Codes whose timeframe had no fresh data keep their scoring verdict and
	// evidence; non-bar codes are not re-evaluated here.
	for _, code := range coin.V7RequiredConfirms {
		if code == "" {
			continue
		}
		if _, already := seenRequired[code]; already {
			continue
		}
		if !local.V7ConfirmLiveVerifiable(code) {
			continue
		}
		barTFs := local.V7ConfirmBarTimeframes(code)
		if len(barTFs) == 0 || !hunterV7DataHasFreshTimeframe(data, barTFs) {
			continue
		}
		seenRequired[code] = struct{}{}
		if hunterV7CodeIndicatorTruncated(code, facts.Timeframes, priorTFs) {
			// Fresh series has closed bar(s) but lacks an indicator (EMA20/VWAP20)
			// that prior facts had: keep scoring's passed verdict and BarEvidence
			// instead of fabricating a demotion from a truncated prompt window.
			continue
		}
		check, known, ok := local.V7EvaluateSharedConfirmation(code, facts)
		if !ok || !known {
			continue
		}
		if check.Passed {
			if check.Evidence != nil {
				updated.UpsertBarEvidence(*check.Evidence)
				evidenceChanged = true
			}
			continue
		}
		if check.Severity == "" {
			check.Severity = local.V7ConfirmReviewWait
		}
		if check.Evidence != nil {
			ev := *check.Evidence
			check.Evidence = &ev
			updated.UpsertBarEvidence(ev)
		}
		updated.MissingReview = append(updated.MissingReview, check)
		demoted = append(demoted, code)
		evidenceChanged = true
	}

	if len(cleared) == 0 && len(demoted) == 0 && !evidenceChanged {
		return coin
	}
	if len(demoted) > 0 || len(updated.MissingReview) > 0 {
		updated.PassedReview = false
	} else if len(cleared) > 0 && updated.PassedHard {
		updated.PassedReview = true
	}

	coin.V7ConfirmSummary = updated
	if len(cleared) > 0 || len(demoted) > 0 {
		codes := cloneStringSlice(coin.V7ReasonCodes)
		for _, code := range cleared {
			codes = appendReasonCodeIfMissing(codes, "live_confirmed_"+code)
		}
		if len(demoted) > 0 {
			codes = removeLiveConfirmedReasonCodes(codes, demoted)
		}
		coin.V7ReasonCodes = codes
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

// The live verifier has no predicates of its own (#136): every code it can
// settle is one of local's shared confirmation predicates
// (local.V7ConfirmLiveVerifiable), evaluated here on facts built from the
// prompt cycle's closed bars (U5/M2b). Scoring time, live re-tier, and the
// trader's decision-time recheck all call the same predicates on closed bars.

// hunterV7VerifyLiveConfirmation checks one confirmation against live prompt
// data at time.Now().
func hunterV7VerifyLiveConfirmation(coin CandidateCoin, data *market.Data, code string) (passed bool, known bool) {
	return hunterV7VerifyLiveConfirmationAt(coin, data, time.Now(), code)
}

// hunterV7VerifyLiveConfirmationAt checks one confirmation against closed bars
// in data at at. The second return reports whether this code is one we know how
// to verify at all with the data at hand; unknown codes are never treated as
// satisfied.
func hunterV7VerifyLiveConfirmationAt(coin CandidateCoin, data *market.Data, at time.Time, code string) (passed bool, known bool) {
	if data == nil {
		return false, false
	}
	price := data.CurrentPrice
	if price <= 0 && coin.V7PriceContext != nil {
		price = coin.V7PriceContext.Last
	}
	return HunterV7VerifyConfirmationAt(coin, data, price, at, code)
}

// HunterV7VerifyConfirmation evaluates a live-verifiable confirmation code's
// shared predicate for a candidate at price using time.Now() as the closed-bar
// cutoff.
func HunterV7VerifyConfirmation(coin CandidateCoin, data *market.Data, price float64, code string) (passed, known bool) {
	return HunterV7VerifyConfirmationAt(coin, data, price, time.Now(), code)
}

// HunterV7VerifyConfirmationAt evaluates a live-verifiable confirmation code's
// shared predicate for a candidate at price with closed-bar cutoff at.
// Timeframe facts come from closed bars in data when given; with data nil they
// come from the candidate's closed-bar V7ConfirmTimeframes (falling back to
// V7ExecutionContext.Timeframes when unset).
func HunterV7VerifyConfirmationAt(coin CandidateCoin, data *market.Data, price float64, at time.Time, code string) (passed, known bool) {
	if !local.V7ConfirmLiveVerifiable(code) {
		return false, false
	}
	check, known, _ := local.V7EvaluateSharedConfirmation(code, HunterV7ConfirmFactsAt(coin, data, price, at))
	return known && check.Passed, known
}

// HunterV7ConfirmFacts builds the shared predicate facts for a candidate using
// time.Now() as the closed-bar cutoff.
func HunterV7ConfirmFacts(coin CandidateCoin, data *market.Data, price float64) local.V7ConfirmFacts {
	return HunterV7ConfirmFactsAt(coin, data, price, time.Now())
}

// HunterV7ConfirmFactsAt builds the shared predicate facts for a candidate,
// filtering data.TimeframeData[tf].Klines to strictly closed bars at at
// (reusing hunterV7ClosedKlines, U5/M2b).
func HunterV7ConfirmFactsAt(coin CandidateCoin, data *market.Data, price float64, at time.Time) local.V7ConfirmFacts {
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
	// Seed from existing closed-bar summaries (falling back to execution
	// context when V7ConfirmTimeframes is unset) so partial prompt data
	// carrying only one of 5m/15m preserves the other timeframe's summary
	// instead of wiping the map (U5b Fix 2).
	var tfMap map[string]local.V7ExecutionTimeframeSummary
	if coin.V7ConfirmTimeframes != nil {
		tfMap = cloneV7TimeframeMap(coin.V7ConfirmTimeframes)
	} else if coin.V7ExecutionContext != nil && coin.V7ExecutionContext.Timeframes != nil {
		tfMap = cloneV7TimeframeMap(coin.V7ExecutionContext.Timeframes)
	}
	if data == nil {
		f.Timeframes = tfMap
		return f
	}
	if at.IsZero() {
		at = time.Now()
	}
	nowMs := at.UnixMilli()
	if tfMap == nil {
		tfMap = make(map[string]local.V7ExecutionTimeframeSummary, 2)
	}
	for _, tf := range []string{"15m", "5m"} {
		series := data.TimeframeData[tf]
		if series == nil {
			// Timeframe absent from prompt data: leave existing entry untouched.
			continue
		}
		if len(series.Klines) == 0 {
			delete(tfMap, tf)
			continue
		}
		closed := hunterV7ClosedKlines(series, tf, nowMs)
		if len(closed) == 0 {
			delete(tfMap, tf)
			continue
		}
		ivMs := hunterV7V2TFInterval(tf).Milliseconds()
		bars := make([]local.V7Bar, len(closed))
		volumes := make([]float64, len(closed))
		for i, k := range closed {
			closeTimeMs := int64(0)
			if k.Time > 0 && ivMs > 0 {
				closeTimeMs = k.Time + ivMs
			}
			bars[i] = local.V7Bar{
				OpenTime:  k.Time,
				CloseTime: closeTimeMs,
				High:      k.High,
				Low:       k.Low,
				Close:     k.Close,
				Volume:    k.Volume,
			}
			volumes[i] = k.Volume
		}
		ema20 := hunterV7ClosedEMA20(series, closed)
		if summary, ok := local.V7TimeframeFactsFromBars(tf, bars, ema20); ok {
			if !summary.HasVWAP20 {
				if vwap20 := hunterV7ClosedVWAP20(series, closed); vwap20 > 0 && summary.LastClose > 0 {
					summary.HasVWAP20 = true
					summary.VWAP20 = vwap20
					summary.CloseVsVWAP20Pct = (summary.LastClose - vwap20) / vwap20 * 100
				}
			}
			// Fix C: if the fresh closed bar is the exact same bar as the prior
			// summary (same LastCloseTimeUnix > 0), inherit indicator fields that
			// scoring computed over full history when the fresh window lacks them.
			if prev, hadPrev := tfMap[tf]; hadPrev && prev.LastCloseTimeUnix > 0 && summary.LastCloseTimeUnix == prev.LastCloseTimeUnix {
				if !summary.HasEMA20 && prev.HasEMA20 {
					summary.HasEMA20 = true
					summary.EMA20 = prev.EMA20
					summary.CloseVsEMA20Pct = prev.CloseVsEMA20Pct
				}
				if !summary.HasVWAP20 && prev.HasVWAP20 {
					summary.HasVWAP20 = true
					summary.VWAP20 = prev.VWAP20
					summary.CloseVsVWAP20Pct = prev.CloseVsVWAP20Pct
				}
			}
			tfMap[tf] = summary
		} else {
			delete(tfMap, tf)
		}
		if tf == "15m" {
			f.VolumeBurst15m = local.V7VolumeBurst(volumes, local.V7VolumeBurst15mLookback)
		}
	}
	f.Timeframes = tfMap
	return f
}

// hunterV7ClosedBarIndex resolves the index of the last closed bar within
// series.Klines (matching by Time first, falling back to a backward Time scan
// when lastClosedTime > 0, or len(series.Klines)-1 when all klines are closed).
func hunterV7ClosedBarIndex(series *market.TimeframeSeriesData, closed []market.KlineBar) int {
	if series == nil || len(closed) == 0 || len(series.Klines) == 0 {
		return -1
	}
	lastClosedTime := closed[len(closed)-1].Time
	if idx := len(closed) - 1; idx < len(series.Klines) && series.Klines[idx].Time == lastClosedTime {
		return idx
	}
	if lastClosedTime > 0 {
		for i := len(series.Klines) - 1; i >= 0; i-- {
			if series.Klines[i].Time == lastClosedTime {
				return i
			}
		}
	}
	if len(closed) == len(series.Klines) {
		return len(series.Klines) - 1
	}
	return -1
}

// hunterV7ClosedSeriesValue maps the last closed bar's index in series.Klines
// into a right-aligned indicator slice (such as EMA20Values or VWAP20Values).
func hunterV7ClosedSeriesValue(series *market.TimeframeSeriesData, closed []market.KlineBar, values []float64) float64 {
	if series == nil || len(closed) == 0 || len(values) == 0 {
		return 0
	}
	idx := hunterV7ClosedBarIndex(series, closed)
	if idx < 0 {
		return 0
	}
	valIdx := len(values) - (len(series.Klines) - idx)
	if valIdx < 0 || valIdx >= len(values) {
		return 0
	}
	return values[valIdx]
}

// hunterV7ClosedEMA20 returns the EMA20 value aligned to the last closed bar,
// or 0 if no closed-aligned EMA20 is available (so V7TimeframeFactsFromBars
// recomputes EMA20 strictly from closed bars when len >= 20).
func hunterV7ClosedEMA20(series *market.TimeframeSeriesData, closed []market.KlineBar) float64 {
	if series == nil {
		return 0
	}
	return hunterV7ClosedSeriesValue(series, closed, series.EMA20Values)
}

// hunterV7ClosedVWAP20 returns the full-history 20-bar VWAP aligned to the last
// closed bar, or 0 if unavailable.
func hunterV7ClosedVWAP20(series *market.TimeframeSeriesData, closed []market.KlineBar) float64 {
	if series == nil {
		return 0
	}
	return hunterV7ClosedSeriesValue(series, closed, series.VWAP20Values)
}

// hunterV7CodeIndicatorTruncated reports whether freshTFs has closed bar(s)
// for code's timeframe but lacks an indicator (EMA20 / VWAP20) that code reads
// and that priorTFs had. Zero closed bars return false so missing closed bars
// still fail closed everywhere.
func hunterV7CodeIndicatorTruncated(code string, freshTFs, priorTFs map[string]local.V7ExecutionTimeframeSummary) bool {
	if len(priorTFs) == 0 {
		return false
	}
	switch code {
	case "5m_price_holds_ema20_or_trailing_support":
		fresh5, ok5 := freshTFs["5m"]
		prior5, had5 := priorTFs["5m"]
		return ok5 && fresh5.LastClose > 0 && !fresh5.HasEMA20 && had5 && prior5.HasEMA20
	case "directional_15m_close_long", "directional_15m_close_short":
		fresh15, ok15 := freshTFs["15m"]
		prior15, had15 := priorTFs["15m"]
		if !ok15 || fresh15.LastClose <= 0 || !had15 {
			return false
		}
		missingEMA := !fresh15.HasEMA20 && prior15.HasEMA20
		missingVWAP := !fresh15.HasVWAP20 && prior15.HasVWAP20
		return missingEMA || missingVWAP
	default:
		return false
	}
}

func hunterV7DataHasFreshTimeframe(data *market.Data, tfs []string) bool {
	if data == nil || len(tfs) == 0 {
		return false
	}
	for _, tf := range tfs {
		if data.TimeframeData[tf] != nil {
			return true
		}
	}
	return false
}

func appendReasonCodeIfMissing(codes []string, code string) []string {
	for _, existing := range codes {
		if existing == code {
			return codes
		}
	}
	return append(codes, code)
}

func removeLiveConfirmedReasonCodes(codes []string, demoted []string) []string {
	if len(codes) == 0 || len(demoted) == 0 {
		return codes
	}
	drop := make(map[string]struct{}, len(demoted))
	for _, code := range demoted {
		drop["live_confirmed_"+code] = struct{}{}
	}
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if _, rm := drop[c]; !rm {
			out = append(out, c)
		}
	}
	return out
}
