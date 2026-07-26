package kernel

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
)

// hunterV7LiveConfirmMMSCandidate mirrors OPUSDT from the 2026-07-26 17:56
// verification round: every executable-score condition met, held at
// REVIEWABLE purely because the 5m close had not been machine-verified yet.
func hunterV7LiveConfirmMMSCandidate() CandidateCoin {
	return CandidateCoin{
		Symbol:             "OPUSDT",
		Direction:          "LONG",
		V7SetupType:        "mms_trend_ride_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       85.1,
		V7SetupScore:       100,
		V7TimingScore:      68,
		V7RiskScore:        0,
		V7LiquidityScore:   70,
		V7RiskLevel:        "LOW",
		V7EntryZone:        local.V7PriceZone{Lower: 0.70, Upper: 0.72},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.698},
		V7Targets:          []local.V7Target{{Price: 0.74, Reason: "trend_continuation"}},
		V7ReasonCodes:      []string{"mms_trend_ride", "mms_trend_continuation", "taker_buy_strong"},
		V7RequiredConfirms: []string{
			"5m_price_holds_ema20_or_trailing_support",
			"taker_flow_not_flipping_against_direction",
			"live_price_in_entry_zone",
		},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{Code: "5m_price_holds_ema20_or_trailing_support", Severity: local.V7ConfirmReviewWait},
			},
			ContextChecks: []local.V7ConfirmationCheck{
				{Code: "taker_flow_not_flipping_against_direction", Passed: true, Severity: local.V7ConfirmContext},
				{Code: "live_price_in_entry_zone", Passed: true, Severity: local.V7ConfirmContext},
			},
			RR: 2.1,
		},
		V7PriceContext:   &local.V7PriceContext{Last: 0.71, Change1h: 0.8, Change4h: 1.2},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.7114, OIChange1h: 1.4, OIChange4h: 2.0},
	}
}

func hunterV7LiveConfirmMarketData(close5m, ema20 float64) *market.Data {
	return &market.Data{
		Symbol:       "OPUSDT",
		CurrentPrice: 0.71,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"5m": {
				Timeframe:   "5m",
				Klines:      []market.KlineBar{{Close: close5m}},
				EMA20Values: []float64{ema20},
			},
		},
	}
}

func TestHunterV7LiveConfirmationsPromoteHeldReviewable(t *testing.T) {
	coin := hunterV7LiveConfirmMMSCandidate()
	data := hunterV7LiveConfirmMarketData(0.712, 0.708) // 5m close holds EMA20

	before, _ := classifyHunterV7CandidateTier(coin)
	if before != "REVIEWABLE" {
		t.Fatalf("pre-confirmation tier = %q, want REVIEWABLE", before)
	}

	updated := hunterV7ApplyLiveConfirmations(coin, data)
	if !updated.V7ConfirmSummary.PassedReview {
		t.Fatalf("expected review to pass after live confirmation, summary=%+v", updated.V7ConfirmSummary)
	}
	if !containsStringValue(updated.V7ReasonCodes, "live_confirmed_5m_price_holds_ema20_or_trailing_support") {
		t.Fatalf("missing live_confirmed reason code, got %v", updated.V7ReasonCodes)
	}

	after, reason := classifyHunterV7CandidateTier(updated)
	if after != "EXECUTABLE" {
		t.Fatalf("post-confirmation tier = %q (%s), want EXECUTABLE", after, reason)
	}
}

func TestHunterV7LiveConfirmationsKeepFailingCloseReviewable(t *testing.T) {
	coin := hunterV7LiveConfirmMMSCandidate()
	data := hunterV7LiveConfirmMarketData(0.705, 0.708) // 5m close lost EMA20

	updated := hunterV7ApplyLiveConfirmations(coin, data)
	if updated.V7ConfirmSummary.PassedReview {
		t.Fatal("failing 5m close must not clear the review gap")
	}
	if tier, _ := classifyHunterV7CandidateTier(updated); tier != "REVIEWABLE" {
		t.Fatalf("tier = %q, want REVIEWABLE while close is below EMA20", tier)
	}
}

func TestHunterV7LiveConfirmationsIgnoreUnknownCodes(t *testing.T) {
	coin := hunterV7LiveConfirmMMSCandidate()
	coin.V7RequiredConfirms = append(coin.V7RequiredConfirms, "some_new_module_confirmation")
	coin.V7ConfirmSummary.MissingReview = append(coin.V7ConfirmSummary.MissingReview,
		local.V7ConfirmationCheck{Code: "some_new_module_confirmation", Severity: local.V7ConfirmReviewWait})
	data := hunterV7LiveConfirmMarketData(0.712, 0.708)

	updated := hunterV7ApplyLiveConfirmations(coin, data)
	if updated.V7ConfirmSummary.PassedReview {
		t.Fatal("unknown confirmation code must stay unresolved")
	}
	for _, code := range updated.V7ReasonCodes {
		if code == "fresh_micro_confirmed" || code == "fresh_rest_confirmed" {
			t.Fatalf("umbrella freshness code %q claimed while a gap remains", code)
		}
	}
	// The known check should still have been settled individually.
	if !containsStringValue(updated.V7ReasonCodes, "live_confirmed_5m_price_holds_ema20_or_trailing_support") {
		t.Fatalf("known confirmation should still clear, got %v", updated.V7ReasonCodes)
	}
}

func TestHunterV7LiveConfirmationsDoNotMutateSharedSummary(t *testing.T) {
	coin := hunterV7LiveConfirmMMSCandidate()
	shared := coin.V7ConfirmSummary
	data := hunterV7LiveConfirmMarketData(0.712, 0.708)

	_ = hunterV7ApplyLiveConfirmations(coin, data)

	if shared.PassedReview {
		t.Fatal("original summary mutated: PassedReview flipped on shared pointer")
	}
	if len(shared.MissingReview) != 1 || shared.MissingReview[0].Code != "5m_price_holds_ema20_or_trailing_support" {
		t.Fatalf("original MissingReview mutated: %+v", shared.MissingReview)
	}
}

func TestHunterV7LiveConfirmationsMissingDataLeavesCandidateUntouched(t *testing.T) {
	coin := hunterV7LiveConfirmMMSCandidate()
	// No 5m timeframe data at all.
	data := &market.Data{Symbol: "OPUSDT", CurrentPrice: 0.71, TimeframeData: map[string]*market.TimeframeSeriesData{}}

	updated := hunterV7ApplyLiveConfirmations(coin, data)
	if updated.V7ConfirmSummary.PassedReview {
		t.Fatal("missing timeframe data must not clear the review gap")
	}
	for _, code := range updated.V7ReasonCodes {
		if strings.HasPrefix(code, "live_confirmed_5m_price_holds") {
			t.Fatalf("cleared confirmation without data: %v", updated.V7ReasonCodes)
		}
	}
}

// A reading exactly on the threshold must produce the same verdict in the
// pre-prompt live pass as in the provider evaluator and the tier classifier.
// Before U2.2 this pass alone used strict comparison and disagreed at the
// boundary on the same request path.
func TestHunterV7LiveTakerBoundaryIsInclusive(t *testing.T) {
	coin := hunterV7LiveConfirmMMSCandidate()
	coin.V7DerivativesCtx.TakerBuy15m = 0.52

	if passed, known := hunterV7VerifyLiveConfirmation(coin, hunterV7LiveConfirmMarketData(0.712, 0.708), "taker_buy_15m_gt_0_52"); !known || !passed {
		t.Fatalf("taker exactly 0.52 must pass gt_0_52 inclusively (passed=%v known=%v)", passed, known)
	}
	coin.V7DerivativesCtx.TakerBuy15m = 0.48
	if passed, known := hunterV7VerifyLiveConfirmation(coin, hunterV7LiveConfirmMarketData(0.712, 0.708), "taker_buy_15m_lt_0_48"); !known || !passed {
		t.Fatalf("taker exactly 0.48 must pass lt_0_48 inclusively (passed=%v known=%v)", passed, known)
	}
}
