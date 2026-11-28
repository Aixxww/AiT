package trader

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/engine"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Unit U9 (P0-2): confirmation-only execution-guard veto reroute into U9
// conditional entry. Covers:
//  1. The 3 live Group A shapes (BLESS/ICNT/PONS IMPULSE shorts, sole blocker =
//     5m_or_15m_close_below_trigger): enabled -> pending entry registered (not
//     market order, not plain rejection, distinct decision + lifecycle markers,
//     class-agnostic even with default Classes=[TRIGGER]); disabled -> fail-closed
//     rejection with distinct reroute attempt + disabled markers/logs.
//  2. All 4 U5 closed-bar trigger codes & direction mismatch guard.
//  3. Coexisting second blocker (second missing confirmation, hard RR block,
//     MinZonePosShort setup_guard, risk-tag combo, OI building, WATCH tier,
//     wait-only tag, missing trigger/invalid stop geometry) -> NO reroute.
//  4. Non-confirmation blocker alone (RR infeasible, non-trigger confirmation,
//     nil confirm summary) -> NO reroute.
//  5. Passing guard -> market open path untouched (order placed on exchange).
//  6. Daily-loss breaker semantics preserved (armed blocks reroute, shadow allows).
//  7. End-to-end U9 lifecycle on rerouted pending entries: fire on 1m close
//     through trigger (with real executeOpenShortWithRecord + fire-time guard),
//     fire-time stale-snapshot cancellation, cancel on invalidation touch,
//     expire after MaxPendingMinutes, and restart persistence round-trip.

type p02LiveShape struct {
	name         string
	symbol       string
	setupType    local.V7SetupType
	zone         local.V7PriceZone
	invalidation float64
	tp0, tp1     float64
	decisionSL   float64
	decisionTP   float64
	decisionPx   float64 // inside upper entry zone so MinZonePosShort passes & within 0.5% of trigger
	confidence   int
	required     []string
}

func p02LiveShapes() []p02LiveShape {
	// Exact geometries from AIK11 C16 (BLESS), C18 (ICNT), C31 (PONS) in
	// sig.jsonl / dr.jsonl (IMPULSE shorts, native V7TriggerPrice=0 so trigger
	// resolves from EntryZone.Lower). decisionPx sits in the upper half of
	// EntryZone so MinZonePosShort (45% / 40%) passes and drift to trigger is <= 0.5%.
	return []p02LiveShape{
		{
			name:         "C16_BLESSUSDT_breakdown_momentum_short",
			symbol:       "BLESSUSDT",
			setupType:    local.V7SetupBreakdownShort,
			zone:         local.V7PriceZone{Lower: 0.013816808611979915, Upper: 0.013949536417822414},
			invalidation: 0.014162876000000001,
			tp0:          0.013806906726227251,
			tp1:          0.01290182148192,
			decisionSL:   0.014162876,
			decisionTP:   0.013105630874714442,
			decisionPx:   0.013877, // zone_pos 45.35% >= 45%, drift to Lower 0.434% <= 0.45%
			confidence:   82,
			required:     []string{"5m_or_15m_close_below_trigger", "taker_buy_15m_lt_0_48", "no_immediate_reclaim_of_trigger_level"},
		},
		{
			name:         "C18_ICNTUSDT_alt_ladder_breakdown_short",
			symbol:       "ICNTUSDT",
			setupType:    local.V7SetupAltLadderShort,
			zone:         local.V7PriceZone{Lower: 0.29349855988945545, Upper: 0.2966459430940239},
			invalidation: 0.300979,
			tp0:          0.2903468526851314,
			tp1:          0.2757250939416751,
			decisionSL:   0.30098,
			decisionTP:   0.27902,
			decisionPx:   0.29480, // zone_pos 41.3% >= 40%, drift to Lower 0.441% <= 0.5%
			confidence:   76,
			required:     []string{"live_price_in_entry_zone", "taker_buy_15m_lt_0_48", "5m_or_15m_close_below_trigger", "taker_flow_not_flipping_against_direction"},
		},
		{
			name:         "C31_PONSUSDT_alt_ladder_breakdown_short",
			symbol:       "PONSUSDT",
			setupType:    local.V7SetupAltLadderShort,
			zone:         local.V7PriceZone{Lower: 0.1006455034011606, Upper: 0.10144534985465585},
			invalidation: 0.10291800000000001,
			tp0:          0.10011029745610295,
			tp1:          0.0952076579068888,
			decisionSL:   0.10292,
			decisionTP:   0.09623,
			decisionPx:   0.10100, // zone_pos 44.3% >= 40%, drift to Lower 0.351% <= 0.5%
			confidence:   74,
			required:     []string{"live_price_in_entry_zone", "taker_buy_15m_lt_0_48", "5m_or_15m_close_below_trigger", "taker_flow_not_flipping_against_direction"},
		},
	}
}

// p02OrderBookFake embeds *lifecycleFake so all client-order-ID / protection
// methods remain promoted while also satisfying GridTrader for micro-refresh.
type p02OrderBookFake struct {
	*lifecycleFake
	bids      [][]float64
	asks      [][]float64
	bookCalls int
	bookErr   error
}

func (f *p02OrderBookFake) PlaceLimitOrder(*LimitOrderRequest) (*LimitOrderResult, error) {
	return nil, nil
}
func (f *p02OrderBookFake) CancelOrder(string, string) error { return nil }
func (f *p02OrderBookFake) GetOrderBook(string, int) ([][]float64, [][]float64, error) {
	f.bookCalls++
	if f.bookErr != nil {
		return nil, nil, f.bookErr
	}
	return f.bids, f.asks, nil
}

func p02Trader(t *testing.T, id string, enabled bool, st *store.Store, bookPrice float64) (*AutoTrader, *lifecycleFake) {
	t.Helper()
	ft := &lifecycleFake{
		bookPrice:  bookPrice,
		fillPrice:  bookPrice,
		reportFill: true,
	}
	ft.balance = map[string]interface{}{
		"availableBalance":   800.0,
		"totalEquity":        1000.0,
		"totalWalletBalance": 1000.0,
	}
	at := newLifecycleTrader(t, ft, st)
	at.trader = &p02OrderBookFake{
		lifecycleFake: ft,
		bids:          [][]float64{{bookPrice, 1000}},
		asks:          [][]float64{{bookPrice * 1.0002, 1000}},
	}
	at.id = id
	at.callCount = 16
	at.isRunning = true
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	// Deliberately leave Classes empty -> EffectiveV7ConditionalEntry defaults
	// to [TRIGGER] ONLY, proving reroute is class-agnostic (IMPULSE reroutes).
	at.config.StrategyConfig.CoinSource.Hunter = &store.HunterConfig{
		V7ConditionalEntry: &store.V7ConditionalEntryConfig{Enabled: enabled},
	}
	at.config.StrategyConfig.RiskControl.MaxPositions = 2
	at.config.StrategyConfig.RiskControl.MinPositionSize = 12
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.2
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3.5
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5
	at.config.StrategyConfig.RiskControl.MaxSingleTradeLossPct = 6.0
	at.config.StrategyConfig.RiskControl.MaxMarginUsage = 0.7
	at.config.StrategyConfig.RiskControl.AltcoinMaxPositionValueRatio = 10
	at.config.StrategyConfig.RiskControl.BTCETHMaxPositionValueRatio = 10
	at.config.StrategyConfig.RiskControl.MinConfidence = 70
	return at, ft
}

func p02BuildCase(s p02LiveShape) (*kernel.Context, *kernel.Decision) {
	// Closed 5m/15m bars above EntryZone.Lower -> 5m_or_15m_close_below_trigger fails.
	tf5 := local.V7ExecutionTimeframeSummary{Timeframe: "5m", LastClose: s.decisionPx, LastCloseTimeUnix: 1764289200}
	tf15 := local.V7ExecutionTimeframeSummary{Timeframe: "15m", LastClose: s.decisionPx, LastCloseTimeUnix: 1764289200}
	cand := kernel.CandidateCoin{
		Symbol:             s.symbol,
		Direction:          "SHORT",
		V7SignalID:         "sig_" + s.symbol,
		V7SetupType:        string(s.setupType),
		V7SignalClass:      store.V7ClassImpulse,
		V7ExecutionTier:    "REVIEWABLE",
		V7TierReason:       "live_reviewable_5m_or_15m_close_below_trigger",
		V7Confidence:       "B",
		V7LiquidityScore:   95,
		V7EntryZone:        s.zone,
		V7Invalidation:     local.V7InvalidationRule{Price: s.invalidation},
		V7TP0Price:         s.tp0,
		V7TP1Price:         s.tp1,
		V7RequiredConfirms: append([]string{}, s.required...),
		// Already refreshed (fresh_* present) so test doesn't make live REST calls;
		// U5 closed-bar re-evaluation inside validateHunterV7RequiredConfirmations
		// still returns false because closed 5m/15m LastClose > EntryZone.Lower.
		V7ReasonCodes: []string{"fresh_micro_confirmed", "fresh_rest_confirmed"},
		V7PriceContext: &local.V7PriceContext{
			Last: s.decisionPx,
		},
		V7DerivativesCtx: &local.V7DerivativesContext{
			TakerBuy15m: 0.35,
			OIChange1h:  -0.5,
			OIChange4h:  -1.0,
		},
		V7ConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{
			"5m":  tf5,
			"15m": tf15,
		},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{
					Code:      "5m_or_15m_close_below_trigger",
					Passed:    false,
					Actual:    s.decisionPx,
					Threshold: s.zone.Lower,
					Severity:  local.V7ConfirmReviewWait,
					Reason:    "short trigger has not cleared entry-zone lower bound on closed 5m/15m bar",
					Evidence: &local.V7ConfirmBarEvidence{
						Code:          "5m_or_15m_close_below_trigger",
						Passed:        false,
						Timeframe:     "5m|15m",
						DecidingTF:    "5m",
						BarClose:      s.decisionPx,
						ReferenceEdge: s.zone.Lower,
						ClosedOnly:    true,
					},
				},
			},
		},
	}
	decision := &kernel.Decision{
		Symbol:                    s.symbol,
		Action:                    "open_short",
		Leverage:                  10,
		PositionSizeUSD:           50,
		Price:                     s.decisionPx,
		StopLoss:                  s.decisionSL,
		TakeProfit:                s.decisionTP,
		Confidence:                s.confidence,
		SelectedHunterV7SignalID:  cand.V7SignalID,
		SelectedHunterV7Setup:     string(s.setupType),
		SelectedHunterV7Tier:      "REVIEWABLE",
		HunterV7InvalidationPrice: s.invalidation,
		HunterV7TP0Price:          s.tp0,
		HunterV7TP1Price:          s.tp1,
	}
	return &kernel.Context{CandidateCoins: []kernel.CandidateCoin{cand}}, decision
}

func TestP02Reroute_ThreeLiveShapesEnabledAndDisabled(t *testing.T) {
	t0 := time.Date(2026, 11, 28, 0, 22, 24, 0, time.UTC)
	st := newLifecycleStore(t)

	for _, s := range p02LiveShapes() {
		t.Run(s.name+"/enabled_reroutes_to_pending", func(t *testing.T) {
			at, ft := p02Trader(t, "p02-en-"+s.symbol, true, st, s.decisionPx)
			ctx, d := p02BuildCase(s)

			// Verify default strategy Classes does NOT contain IMPULSE.
			if at.conditionalEntryConfig().ClassSet()[store.V7ClassImpulse] {
				t.Fatalf("expected default classes not to include IMPULSE, got %v", at.conditionalEntryConfig().Classes)
			}

			rec := &store.DecisionRecord{}
			act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
			ok := at.runGuardedDecision(ctx, d, &act, rec, t0)
			if !ok {
				t.Fatalf("expected reroute to succeed, got act=%+v log=%v", act, rec.ExecutionLog)
			}
			// Must NOT execute a market open on the exchange.
			if ft.opens != 0 {
				t.Fatalf("rerouted decision must not place market order: opens=%d", ft.opens)
			}
			// Must register exactly 1 pending entry at EntryZone.Lower.
			pending := at.pendingConditionalEntries()
			if len(pending) != 1 {
				t.Fatalf("expected 1 pending entry, got %d", len(pending))
			}
			p := pending[0]
			wantInval := math.Min(s.invalidation, s.decisionSL)
			if !p.ReroutedFromGuard || p.ReroutedBlockCode != "5m_or_15m_close_below_trigger" ||
				p.Trigger != s.zone.Lower || p.Invalidation != wantInval || p.Side != "short" {
				t.Fatalf("unexpected pending entry: %+v (want trigger=%.12f inval=%.12f)", p, s.zone.Lower, wantInval)
			}
			// DecisionAction observability markers.
			if !act.Success || !act.ConditionalPending || act.Price != s.zone.Lower ||
				act.BlockedReasonCode != store.V7BlockedReasonReroutedConditionalEntry ||
				act.ConditionalRerouteSource != "5m_or_15m_close_below_trigger" || act.Error != "" {
				t.Fatalf("unexpected actionRecord: %+v", act)
			}
			if !strings.Contains(strings.Join(rec.ExecutionLog, "\n"), store.V7BlockedReasonReroutedConditionalEntry) {
				t.Fatalf("execution log missing reroute marker: %v", rec.ExecutionLog)
			}
			// Persisted PositionLifecycle row must identify reroute distinctly.
			rows, err := st.Decision().ListLifecycles(at.id, store.LifecycleStatusPendingEntry)
			if err != nil || len(rows) != 1 {
				t.Fatalf("expected 1 persisted pending_entry row, got err=%v rows=%+v", err, rows)
			}
			if rows[0].EntrySource != store.LifecycleEntrySourceGuardConfirmReroute ||
				!strings.Contains(rows[0].Note, "5m_or_15m_close_below_trigger") ||
				rows[0].PlanEntryPrice != s.zone.Lower {
				t.Fatalf("persisted lifecycle missing reroute provenance: %+v", rows[0])
			}
			decoded, err := decodePendingEntry(rows[0].PendingPayload)
			if err != nil || !decoded.ReroutedFromGuard || decoded.ReroutedBlockCode != "5m_or_15m_close_below_trigger" {
				t.Fatalf("decoded payload missing reroute provenance: err=%v decoded=%+v", err, decoded)
			}
		})

		t.Run(s.name+"/disabled_fails_closed_with_distinct_log", func(t *testing.T) {
			at, ft := p02Trader(t, "p02-dis-"+s.symbol, false, st, s.decisionPx)
			ctx, d := p02BuildCase(s)

			rec := &store.DecisionRecord{}
			act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
			ok := at.runGuardedDecision(ctx, d, &act, rec, t0)
			if ok {
				t.Fatalf("expected fail-closed rejection when conditional entry disabled")
			}
			if ft.opens != 0 || len(at.pendingConditionalEntries()) != 0 {
				t.Fatalf("disabled reroute must not open or register: opens=%d pending=%d",
					ft.opens, len(at.pendingConditionalEntries()))
			}
			if act.Success || act.ConditionalPending ||
				act.BlockedReasonCode != store.V7BlockedReasonConditionalRerouteDisabled ||
				act.ConditionalRerouteSource != "5m_or_15m_close_below_trigger" {
				t.Fatalf("unexpected fail-closed actionRecord: %+v", act)
			}
			if !strings.Contains(act.Error, "5m_or_15m_close_below_trigger") ||
				!strings.Contains(act.Error, "conditional_entry_disabled") {
				t.Fatalf("fail-closed error must preserve guard veto + disabled reason: %q", act.Error)
			}
			joined := strings.Join(rec.ExecutionLog, "\n")
			if !strings.Contains(joined, store.V7BlockedReasonConditionalRerouteDisabled) ||
				!strings.Contains(joined, "5m_or_15m_close_below_trigger") {
				t.Fatalf("execution log must log reroute attempt + fail-closed outcome: %v", rec.ExecutionLog)
			}
		})
	}
}

func TestP02Reroute_AllClosedBarTriggerCodesAndDirectionCheck(t *testing.T) {
	t0 := time.Date(2026, 11, 28, 1, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		code      string
		action    string
		dir       string
		wantRoute bool
	}{
		{"short_close_below_trigger", "5m_or_15m_close_below_trigger", "open_short", "SHORT", true},
		{"long_close_above_trigger", "5m_or_15m_close_above_trigger", "open_long", "LONG", true},
		{"long_close_above_entry_zone_alias", "5m_or_15m_close_above_entry_zone", "open_long", "LONG", true},
		{"long_close_through_breakout_level", "5m_or_15m_close_through_breakout_level", "open_long", "LONG", true},
		{"short_close_through_breakout_level", "5m_or_15m_close_through_breakout_level", "open_short", "SHORT", true},
		{"direction_mismatch_long_with_below_trigger", "5m_or_15m_close_below_trigger", "open_long", "LONG", false},
		{"direction_mismatch_short_with_above_trigger", "5m_or_15m_close_above_trigger", "open_short", "SHORT", false},
		{"direction_mismatch_short_with_above_zone", "5m_or_15m_close_above_entry_zone", "open_short", "SHORT", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			at, _ := p02Trader(t, "p02-code-"+tc.name, true, nil, 100.0)
			zone := local.V7PriceZone{Lower: 99.7, Upper: 100.3}
			sl, tp, trig := 97.5, 105.0, zone.Upper
			if tc.dir == "SHORT" {
				sl, tp, trig = 102.5, 95.0, zone.Lower
			}
			cand := kernel.CandidateCoin{
				Symbol:             "TESTUSDT",
				Direction:          tc.dir,
				V7SignalID:         "sig-test",
				V7SetupType:        "trend_breakout_long",
				V7SignalClass:      store.V7ClassTrigger,
				V7ExecutionTier:    "REVIEWABLE",
				V7Confidence:       "B",
				V7EntryZone:        zone,
				V7TriggerPrice:     trig,
				V7Invalidation:     local.V7InvalidationRule{Price: sl},
				V7RequiredConfirms: []string{tc.code},
				V7ReasonCodes:      []string{"fresh_micro_confirmed", "fresh_rest_confirmed"},
				V7PriceContext:     &local.V7PriceContext{Last: 100.0},
				V7ConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{
					"5m":  {Timeframe: "5m", LastClose: 100.0},
					"15m": {Timeframe: "15m", LastClose: 100.0},
				},
				V7ConfirmSummary: &local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: false,
					MissingReview: []local.V7ConfirmationCheck{
						{Code: tc.code, Passed: false, Severity: local.V7ConfirmReviewWait},
					},
				},
			}
			d := &kernel.Decision{
				Symbol:                    "TESTUSDT",
				Action:                    tc.action,
				Leverage:                  5,
				PositionSizeUSD:           50,
				Price:                     100.0,
				StopLoss:                  sl,
				TakeProfit:                tp,
				Confidence:                80,
				SelectedHunterV7SignalID:  "sig-test",
				HunterV7TriggerPrice:      trig,
				HunterV7InvalidationPrice: sl,
			}
			ctx := &kernel.Context{CandidateCoins: []kernel.CandidateCoin{cand}}
			act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
			ok := at.runGuardedDecision(ctx, d, &act, &store.DecisionRecord{}, t0)
			if ok != tc.wantRoute {
				t.Fatalf("ok=%v, want %v (act=%+v)", ok, tc.wantRoute, act)
			}
			if tc.wantRoute {
				if !act.ConditionalPending || act.BlockedReasonCode != store.V7BlockedReasonReroutedConditionalEntry ||
					act.ConditionalRerouteSource != tc.code {
					t.Fatalf("unexpected rerouted action: %+v", act)
				}
			} else if act.ConditionalPending || act.ConditionalRerouteSource != "" || len(at.pendingConditionalEntries()) != 0 {
				t.Fatalf("direction mismatch must NOT reroute: act=%+v pending=%d", act, len(at.pendingConditionalEntries()))
			}
		})
	}
}

func TestP02Reroute_CoexistingSecondBlockerPreventsReroute(t *testing.T) {
	t0 := time.Date(2026, 11, 28, 0, 22, 24, 0, time.UTC)
	bless := p02LiveShapes()[0]

	cases := []struct {
		name   string
		mutate func(ctx *kernel.Context, d *kernel.Decision)
	}{
		{
			name: "second_missing_review_confirmation",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7DerivativesCtx.TakerBuy15m = 0.55 // genuinely fails <= 0.48
				ctx.CandidateCoins[0].V7ConfirmSummary.MissingReview = append(
					ctx.CandidateCoins[0].V7ConfirmSummary.MissingReview,
					local.V7ConfirmationCheck{Code: "taker_buy_15m_lt_0_48", Passed: false, Severity: local.V7ConfirmReviewWait},
				)
			},
		},
		{
			name: "non_required_review_check_short_entry_zone_retest",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7ConfirmSummary.MissingReview = append(
					ctx.CandidateCoins[0].V7ConfirmSummary.MissingReview,
					local.V7ConfirmationCheck{Code: "short_entry_zone_retest", Passed: false, Severity: local.V7ConfirmReviewWait},
				)
			},
		},
		{
			name: "coexisting_hard_block_rr_min",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7ConfirmSummary.PassedHard = false
				ctx.CandidateCoins[0].V7ConfirmSummary.MissingHard = []local.V7ConfirmationCheck{
					{Code: local.V7ConfirmRiskRewardMinRR, Passed: false, Severity: local.V7ConfirmHardBlock},
				}
			},
		},
		{
			name: "coexisting_post_confirm_min_zone_pos_short",
			mutate: func(ctx *kernel.Context, d *kernel.Decision) {
				// breakdown_momentum_short BuiltinSetupGuards MinZonePosShort=45;
				// price near lower edge -> zone_pos ~ 10% < 45%.
				lowPx := bless.zone.Lower + 0.10*(bless.zone.Upper-bless.zone.Lower)
				d.Price = lowPx
				ctx.CandidateCoins[0].V7PriceContext.Last = lowPx
			},
		},
		{
			name: "coexisting_post_confirm_risk_tag_combo",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7RiskTags = []string{"high_volatility", "execution_stop_tightened"}
			},
		},
		{
			name: "coexisting_wait_only_risk_tag",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7RiskTags = []string{"do_not_market_chase"}
			},
		},
		{
			name: "coexisting_watch_tier",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7ExecutionTier = "WATCH"
				ctx.CandidateCoins[0].V7TierReason = "alt_ladder_short_rebound_pending"
			},
		},
		{
			name: "missing_trigger_price_and_empty_zone",
			mutate: func(ctx *kernel.Context, d *kernel.Decision) {
				ctx.CandidateCoins[0].V7TriggerPrice = 0
				ctx.CandidateCoins[0].V7EntryZone = local.V7PriceZone{}
				d.HunterV7TriggerPrice = 0
			},
		},
		{
			name: "invalid_invalidation_geometry_wrong_side",
			mutate: func(ctx *kernel.Context, d *kernel.Decision) {
				// SHORT invalidation below trigger is invalid geometry.
				badInval := bless.zone.Lower * 0.98
				ctx.CandidateCoins[0].V7Invalidation.Price = badInval
				d.HunterV7InvalidationPrice = badInval
				d.StopLoss = badInval
			},
		},
		{
			name: "coexisting_context_check_failure_in_required",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7RequiredConfirms = append(ctx.CandidateCoins[0].V7RequiredConfirms, "bb_width_expansion_starts")
				ctx.CandidateCoins[0].V7ConfirmSummary.ContextChecks = []local.V7ConfirmationCheck{
					{Code: "bb_width_expansion_starts", Passed: false, Severity: local.V7ConfirmContext},
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			at, ft := p02Trader(t, "p02-coexist-"+tc.name, true, nil, bless.decisionPx)
			ctx, d := p02BuildCase(bless)
			tc.mutate(ctx, d)

			rec := &store.DecisionRecord{}
			act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
			ok := at.runGuardedDecision(ctx, d, &act, rec, t0)
			if ok {
				t.Fatalf("expected rejection with no reroute, got ok=true act=%+v", act)
			}
			if act.ConditionalPending || act.ConditionalRerouteSource != "" ||
				act.BlockedReasonCode != "" || len(at.pendingConditionalEntries()) != 0 || ft.opens != 0 {
				t.Fatalf("coexisting blocker must stay plain guard veto without reroute: act=%+v pending=%d opens=%d",
					act, len(at.pendingConditionalEntries()), ft.opens)
			}
			if act.Error == "" {
				t.Fatalf("expected plain guard error recorded on action")
			}
		})
	}
}

func TestP02Reroute_NonConfirmationBlockerAloneDoesNotReroute(t *testing.T) {
	t0 := time.Date(2026, 11, 28, 0, 22, 24, 0, time.UTC)
	bless := p02LiveShapes()[0]

	cases := []struct {
		name   string
		mutate func(ctx *kernel.Context, d *kernel.Decision)
	}{
		{
			name: "rr_infeasible_hard_block_alone",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7RequiredConfirms = []string{local.V7ConfirmRiskRewardMinRR}
				ctx.CandidateCoins[0].V7ConfirmSummary = &local.V7ConfirmationSummary{
					PassedHard:   false,
					PassedReview: false,
					MissingHard:  []local.V7ConfirmationCheck{{Code: local.V7ConfirmRiskRewardMinRR, Passed: false, Severity: local.V7ConfirmHardBlock}},
				}
			},
		},
		{
			name: "taker_confirmation_alone",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7DerivativesCtx.TakerBuy15m = 0.55
				ctx.CandidateCoins[0].V7ConfirmSummary = &local.V7ConfirmationSummary{
					PassedHard:    true,
					PassedReview:  false,
					MissingReview: []local.V7ConfirmationCheck{{Code: "taker_buy_15m_lt_0_48", Passed: false, Severity: local.V7ConfirmReviewWait}},
				}
			},
		},
		{
			name: "directional_15m_close_short_not_trigger_code",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7RequiredConfirms = []string{"directional_15m_close_short"}
				ctx.CandidateCoins[0].V7ConfirmSummary = &local.V7ConfirmationSummary{
					PassedHard:    true,
					PassedReview:  false,
					MissingReview: []local.V7ConfirmationCheck{{Code: "directional_15m_close_short", Passed: false, Severity: local.V7ConfirmReviewWait}},
				}
			},
		},
		{
			name: "nil_confirm_summary",
			mutate: func(ctx *kernel.Context, _ *kernel.Decision) {
				ctx.CandidateCoins[0].V7ReasonCodes = nil
				ctx.CandidateCoins[0].V7ConfirmSummary = nil
			},
		},
		{
			name: "drift_at_trigger_exceeded_fails_registration_closed",
			mutate: func(ctx *kernel.Context, d *kernel.Decision) {
				// Decision price is > 0.5% above EntryZone.Lower -> validateOpenDecision
				// at trigger rejects registration; no pending entry may be created.
				d.Price = bless.zone.Upper // ~0.96% above Lower
				ctx.CandidateCoins[0].V7PriceContext.Last = bless.zone.Upper
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			at, ft := p02Trader(t, "p02-coexist-"+tc.name, true, nil, bless.decisionPx)
			ctx, d := p02BuildCase(bless)
			tc.mutate(ctx, d)

			act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
			ok := at.runGuardedDecision(ctx, d, &act, &store.DecisionRecord{}, t0)
			if ok || act.ConditionalPending || len(at.pendingConditionalEntries()) != 0 || ft.opens != 0 {
				t.Fatalf("non-eligible case %q must NOT register or open: ok=%v act=%+v pending=%d opens=%d",
					tc.name, ok, act, len(at.pendingConditionalEntries()), ft.opens)
			}
		})
	}
}

func TestP02Reroute_PassingGuardTakesNormalMarketPathUntouched(t *testing.T) {
	bless := p02LiveShapes()[0]
	stubCoinAnkMarketTransport(t, bless.decisionPx)
	st := newLifecycleStore(t)
	at, ft := p02Trader(t, "p02-pass-mkt", true, st, bless.decisionPx)

	ctx, d := p02BuildCase(bless)
	// Closed 5m bar genuinely closed below EntryZone.Lower -> confirmation passes!
	ctx.CandidateCoins[0].V7ConfirmSummary = &local.V7ConfirmationSummary{
		PassedHard:   true,
		PassedReview: true,
	}
	ctx.CandidateCoins[0].V7ExecutionTier = "EXECUTABLE"
	d.SelectedHunterV7Tier = "EXECUTABLE"

	rec := &store.DecisionRecord{}
	act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
	t0 := time.Date(2026, 11, 28, 0, 22, 24, 0, time.UTC)
	if ok := at.runGuardedDecision(ctx, d, &act, rec, t0); !ok {
		t.Fatalf("passing guard should open at market: act=%+v log=%v", act, rec.ExecutionLog)
	}
	if ft.opens != 1 || len(ft.live) != 1 || ft.live[0]["side"] != "short" {
		t.Fatalf("expected 1 live market short open, got opens=%d live=%+v", ft.opens, ft.live)
	}
	if act.ConditionalPending || act.BlockedReasonCode != "" || act.ConditionalRerouteSource != "" ||
		len(at.pendingConditionalEntries()) != 0 {
		t.Fatalf("passing guard must NOT reroute: act=%+v pending=%d", act, len(at.pendingConditionalEntries()))
	}
}

func TestP02Reroute_DailyLossBreakerSemanticsPreserved(t *testing.T) {
	bless := p02LiveShapes()[0]
	t0 := time.Date(2026, 11, 28, 0, 22, 24, 0, time.UTC)
	st := newLifecycleStore(t)

	for _, mode := range []string{store.DailyLossBreakerArmed, store.DailyLossBreakerShadow} {
		t.Run(mode, func(t *testing.T) {
			at, ft := p02Trader(t, "p02-brk-"+mode, true, st, bless.decisionPx)
			at.config.StrategyConfig.RiskControl.DailyLossBreaker = &store.DailyLossBreakerConfig{Mode: mode}
			clock := t0
			at.dailyLoss.now = func() time.Time { return clock }
			tripBreaker(t, at)

			ctx, d := p02BuildCase(bless)
			rec := &store.DecisionRecord{}
			act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
			ok := at.runGuardedDecision(ctx, d, &act, rec, t0)

			if mode == store.DailyLossBreakerArmed {
				if ok || act.ConditionalPending || len(at.pendingConditionalEntries()) != 0 || ft.opens != 0 {
					t.Fatalf("armed breaker must block reroute: ok=%v act=%+v pending=%d", ok, act, len(at.pendingConditionalEntries()))
				}
				if act.BlockedReasonCode != store.V7BlockedReasonConditionalRerouteFailed ||
					!strings.Contains(act.Error, "daily_loss_breaker") {
					t.Fatalf("expected breaker failure marker, got act=%+v", act)
				}
			} else {
				if !ok || !act.ConditionalPending || len(at.pendingConditionalEntries()) != 1 {
					t.Fatalf("shadow breaker must allow reroute: ok=%v act=%+v pending=%d", ok, act, len(at.pendingConditionalEntries()))
				}
			}
		})
	}
}

func TestP02Reroute_EndToEndFireCancelExpireAndRestart(t *testing.T) {
	bless := p02LiveShapes()[0]
	t0 := time.Date(2026, 11, 28, 0, 22, 24, 0, time.UTC)
	m := t0.Truncate(time.Minute)
	st := newLifecycleStore(t)

	t.Run("fires_through_real_open_path_and_keeps_provenance", func(t *testing.T) {
		// Trigger = 0.0138168; firing 1m bar closes at 0.013816 (<= trigger,
		// within 0.45% micro consistency & 0.5% drift of decisionPx=0.013877 -> 0.440%).
		fireClose := 0.013816
		stubCoinAnkMarketTransport(t, fireClose)
		at, ft := p02Trader(t, "p02-e2e-fire", true, st, fireClose)
		ctx, d := p02BuildCase(bless)

		act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
		if !at.runGuardedDecision(ctx, d, &act, &store.DecisionRecord{}, t0) {
			t.Fatalf("reroute registration failed: %+v", act)
		}
		if ft.opens != 0 || len(at.pendingConditionalEntries()) != 1 {
			t.Fatalf("expected 1 pending entry, 0 opens")
		}

		at.conditional.bars = func(string) []datafetch.Kline {
			return []datafetch.Kline{
				bar(m, 0.013890, 0.013810, 0.013840),                  // intrabar dip below trigger, closed above -> no fire
				bar(m.Add(time.Minute), 0.013845, 0.013805, fireClose), // 1m close <= trigger -> fire!
			}
		}
		at.conditional.latest = func(string) *datafetch.SymbolSnapshot {
			return &datafetch.SymbolSnapshot{Symbol: bless.symbol, Price: fireClose, Timestamp: time.Now().Add(-2 * time.Second)}
		}

		// After bar m only: no fire yet.
		at.checkHunterV7ConditionalEntries(m.Add(time.Minute + 5*time.Second))
		if ft.opens != 0 || len(at.pendingConditionalEntries()) != 1 {
			t.Fatalf("intrabar touch without 1m close must not fire")
		}

		// After bar m+1 closes below trigger: fires through real executeOpenShortWithRecord!
		at.checkHunterV7ConditionalEntries(m.Add(2*time.Minute + 5*time.Second))
		if len(at.pendingConditionalEntries()) != 0 {
			t.Fatalf("fired entry must leave pending queue")
		}
		if ft.opens != 1 || len(ft.live) != 1 || ft.live[0]["side"] != "short" || len(ft.stopLossCalls) != 1 {
			t.Fatalf("expected real short open + stop on exchange, got opens=%d live=%+v stops=%d",
				ft.opens, ft.live, len(ft.stopLossCalls))
		}
		firedRows, err := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryFired)
		if err != nil || len(firedRows) != 1 || firedRows[0].EntrySource != store.LifecycleEntrySourceGuardConfirmReroute {
			t.Fatalf("fired pending lifecycle must retain entry_source provenance: err=%v rows=%+v", err, firedRows)
		}
		recs, err := st.Decision().GetLatestRecords(at.id, 5)
		if err != nil || len(recs) == 0 || len(recs[0].Decisions) != 1 ||
			!recs[0].Decisions[0].Success || recs[0].Decisions[0].ConditionalRerouteSource != "5m_or_15m_close_below_trigger" {
			t.Fatalf("fire-time decision record missing success/provenance: err=%v recs=%+v", err, recs)
		}
	})

	t.Run("cancels_on_invalidation_before_trigger", func(t *testing.T) {
		at, ft := p02Trader(t, "p02-e2e-cancel", true, st, bless.decisionPx)
		ctx, d := p02BuildCase(bless)
		if !at.runGuardedDecision(ctx, d, &store.DecisionAction{Action: d.Action, Symbol: d.Symbol}, &store.DecisionRecord{}, t0) {
			t.Fatalf("reroute failed")
		}
		// Candle touches invalidation (High >= 0.0141629) AND closes below trigger: stop-first cancels.
		at.conditional.bars = func(string) []datafetch.Kline {
			return []datafetch.Kline{bar(m, bless.invalidation+0.0001, 0.013800, 0.013805)}
		}
		at.checkHunterV7ConditionalEntries(m.Add(time.Minute + 5*time.Second))
		if ft.opens != 0 || len(at.pendingConditionalEntries()) != 0 {
			t.Fatalf("invalidation touch must cancel without firing: opens=%d pending=%d", ft.opens, len(at.pendingConditionalEntries()))
		}
		rows, _ := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryCancelled)
		if len(rows) != 1 || rows[0].EntrySource != store.LifecycleEntrySourceGuardConfirmReroute ||
			!strings.Contains(rows[0].Note, "invalidation") {
			t.Fatalf("cancelled lifecycle unexpected: %+v", rows)
		}
	})

	t.Run("expires_after_max_pending_minutes", func(t *testing.T) {
		at, ft := p02Trader(t, "p02-e2e-expire", true, st, bless.decisionPx)
		ctx, d := p02BuildCase(bless)
		if !at.runGuardedDecision(ctx, d, &store.DecisionAction{Action: d.Action, Symbol: d.Symbol}, &store.DecisionRecord{}, t0) {
			t.Fatalf("reroute failed")
		}
		at.conditional.bars = func(string) []datafetch.Kline { return nil }
		at.checkHunterV7ConditionalEntries(t0.Add(store.DefaultV7ConditionalEntryMaxPendingMinutes * time.Minute))
		if ft.opens != 0 || len(at.pendingConditionalEntries()) != 0 {
			t.Fatalf("expired entry must be dropped")
		}
		rows, _ := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryCancelled)
		if len(rows) != 1 || rows[0].EntrySource != store.LifecycleEntrySourceGuardConfirmReroute ||
			!strings.Contains(rows[0].Note, "expired") {
			t.Fatalf("expired lifecycle unexpected: %+v", rows)
		}
	})

	t.Run("fire_time_guard_still_blocks_stale_snapshot", func(t *testing.T) {
		at, ft := p02Trader(t, "p02-e2e-stale", true, st, 0.013816)
		ctx, d := p02BuildCase(bless)
		// U9b Finding 1 (un-masked): keep V7ExecutionTier="REVIEWABLE" and keep
		// t0 V7ReasonCodes (fresh_micro_confirmed, fresh_rest_confirmed). The
		// fire-time guard must still block the 60s-stale snapshot because
		// fireTimeCandidate strips t0 refresh stamps.
		ctx.CandidateCoins[0].V7RiskTags = []string{"high_volatility"}

		if !at.runGuardedDecision(ctx, d, &store.DecisionAction{Action: d.Action, Symbol: d.Symbol}, &store.DecisionRecord{}, t0) {
			t.Fatalf("reroute failed")
		}
		at.conditional.bars = func(string) []datafetch.Kline {
			return []datafetch.Kline{bar(m, 0.013850, 0.013805, 0.013816)}
		}
		at.conditional.latest = func(string) *datafetch.SymbolSnapshot {
			return &datafetch.SymbolSnapshot{Symbol: bless.symbol, Price: 0.013816, Timestamp: time.Now().Add(-60 * time.Second)}
		}
		at.checkHunterV7ConditionalEntries(m.Add(time.Minute + 5*time.Second))
		if ft.opens != 0 {
			t.Fatalf("stale fire-time snapshot must block execution, got opens=%d", ft.opens)
		}
		rows, _ := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryCancelled)
		if len(rows) != 1 || !strings.Contains(rows[0].Note, "execution guard at fire time") ||
			!strings.Contains(rows[0].Note, "stale_data_risk") {
			t.Fatalf("expected fire-time stale_data_risk guard cancellation, got %+v", rows)
		}
	})

	t.Run("fire_time_nil_snapshot_does_not_trust_t0_freshness", func(t *testing.T) {
		at, ft := p02Trader(t, "p02-e2e-nilsnap", true, st, 0.013816)
		ctx, d := p02BuildCase(bless)
		ctx.CandidateCoins[0].V7RiskTags = []string{"high_volatility"}
		ctx.CandidateCoins[0].V7DataFreshness.PriceAgeMs = 0

		if !at.runGuardedDecision(ctx, d, &store.DecisionAction{Action: d.Action, Symbol: d.Symbol}, &store.DecisionRecord{}, t0) {
			t.Fatalf("reroute failed")
		}
		pending := at.pendingConditionalEntries()
		if len(pending) != 1 {
			t.Fatalf("expected 1 pending entry, got %d", len(pending))
		}
		at.conditional.latest = func(string) *datafetch.SymbolSnapshot { return nil }
		// Even immediately after registration (small elapsed), missing snapshot
		// must not retain PriceAgeMs == 0 as fresh.
		fc := at.fireTimeCandidate(&pending[0], t0.Add(2*time.Second))
		if fc.V7DataFreshness.PriceAgeMs <= 45_000 || fc.V7DataFreshness.SnapshotAgeMs <= 45_000 {
			t.Fatalf("nil fire-time snapshot must not look fresh: %+v", fc.V7DataFreshness)
		}
		at.conditional.bars = func(string) []datafetch.Kline {
			return []datafetch.Kline{bar(m, 0.013850, 0.013805, 0.013816)}
		}
		at.checkHunterV7ConditionalEntries(m.Add(time.Minute + 5*time.Second))
		if ft.opens != 0 {
			t.Fatalf("nil fire-time snapshot on high_volatility must block execution, got opens=%d", ft.opens)
		}
		rows, _ := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryCancelled)
		if len(rows) != 1 || !strings.Contains(rows[0].Note, "execution guard at fire time") {
			t.Fatalf("expected fire-time guard cancellation, got %+v", rows)
		}
	})

	t.Run("fire_time_reruns_refresh_for_second_waived_confirmation", func(t *testing.T) {
		fireClose := 0.013816
		stubCoinAnkMarketTransport(t, fireClose)

		var restCalls int64
		mux := http.NewServeMux()
		mux.HandleFunc("/fapi/v1/ticker/24hr", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"symbol":%q,"lastPrice":"%.8f","priceChangePercent":"-1.5","volume":"1000","quoteVolume":"13.8","highPrice":"%.8f","lowPrice":"%.8f","count":100}`,
				bless.symbol, fireClose, fireClose*1.01, fireClose*0.99)
		})
		mux.HandleFunc("/fapi/v1/premiumIndex", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"symbol":%q,"markPrice":"%.8f","indexPrice":"%.8f","lastFundingRate":"0.0001","nextFundingTime":1764289200000}`,
				bless.symbol, fireClose, fireClose)
		})
		mux.HandleFunc("/fapi/v1/openInterest", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"openInterest":"5000"}`)
		})
		mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&restCalls, 1)
			nowMs := time.Now().UnixMilli()
			// Recent 1m bar with neutral move (0%) and bearish taker buy ratio (0.40 < 0.62).
			fmt.Fprintf(w, `[[%d,"%.8f","%.8f","%.8f","%.8f","100",%d,"0","0","40","0","0"]]`,
				nowMs-30_000, fireClose, fireClose, fireClose, fireClose, nowMs+30_000)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		se, err := kernel.NewSnapshotEngine(engine.HubConfig{}, datafetch.CollectorConfig{BinanceURL: srv.URL})
		if err != nil {
			t.Fatalf("NewSnapshotEngine: %v", err)
		}

		for _, tc := range []struct {
			name       string
			failAtFire bool
			wantOpens  int
		}{
			{"refresh_succeeds_at_fire_time", false, 1},
			{"refresh_fails_at_fire_time_blocks_instead_of_reusing_t0_stamps", true, 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				at, ft := p02Trader(t, "p02-e2e-waive-"+tc.name, true, st, fireClose)
				at.snapshotEngine = se
				bookFake := at.trader.(*p02OrderBookFake)
				ctx, d := p02BuildCase(bless)
				// Add a second refresh-waived confirmation (no_new_high_after_rejection)
				// and clear initial V7ReasonCodes so t0 guard runs live REST + book
				// refresh, mints t0 fresh_* stamps, waives no_new_high_after_rejection,
				// and reroutes on sole blocker 5m_or_15m_close_below_trigger.
				ctx.CandidateCoins[0].V7ReasonCodes = nil
				ctx.CandidateCoins[0].V7RequiredConfirms = append(ctx.CandidateCoins[0].V7RequiredConfirms, "no_new_high_after_rejection")
				ctx.CandidateCoins[0].V7ConfirmSummary.MissingReview = append(
					ctx.CandidateCoins[0].V7ConfirmSummary.MissingReview,
					local.V7ConfirmationCheck{Code: "no_new_high_after_rejection", Passed: false, Severity: local.V7ConfirmReviewWait},
				)

				restBeforeReg := atomic.LoadInt64(&restCalls)
				act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
				if !at.runGuardedDecision(ctx, d, &act, &store.DecisionRecord{}, t0) {
					t.Fatalf("expected t0 refresh to waive no_new_high_after_rejection and reroute, got act=%+v", act)
				}
				callsAfterReg := bookFake.bookCalls
				restAfterReg := atomic.LoadInt64(&restCalls)
				if callsAfterReg == 0 || restAfterReg <= restBeforeReg {
					t.Fatalf("expected t0 guard refresh to run book+REST refresh (book=%d, rest=%d->%d)",
						callsAfterReg, restBeforeReg, restAfterReg)
				}
				if tc.failAtFire {
					bookFake.bookErr = errors.New("orderbook unavailable at fire time")
				}
				at.conditional.bars = func(string) []datafetch.Kline {
					return []datafetch.Kline{bar(m, 0.013850, 0.013805, fireClose)}
				}
				at.conditional.latest = func(string) *datafetch.SymbolSnapshot {
					return &datafetch.SymbolSnapshot{Symbol: bless.symbol, Price: fireClose, Timestamp: time.Now().Add(-2 * time.Second)}
				}
				at.checkHunterV7ConditionalEntries(m.Add(time.Minute + 5*time.Second))
				if bookFake.bookCalls <= callsAfterReg || atomic.LoadInt64(&restCalls) <= restAfterReg {
					t.Fatalf("fire-time guard must re-run book+REST refresh evidence collection (book=%d afterReg=%d, rest=%d afterReg=%d)",
						bookFake.bookCalls, callsAfterReg, atomic.LoadInt64(&restCalls), restAfterReg)
				}
				if ft.opens != tc.wantOpens {
					t.Fatalf("opens=%d, want %d", ft.opens, tc.wantOpens)
				}
				if tc.failAtFire {
					rows, _ := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryCancelled)
					if len(rows) != 1 || !strings.Contains(rows[0].Note, "execution guard at fire time") {
						t.Fatalf("expected fire-time guard cancellation when refresh fails, got %+v", rows)
					}
				}
			})
		}
	})

	t.Run("fire_time_settles_context_checks_and_clones_reason_codes", func(t *testing.T) {
		at, _ := p02Trader(t, "p02-e2e-ctxchk", true, st, bless.decisionPx)
		ctx, d := p02BuildCase(bless)
		cand := ctx.CandidateCoins[0]
		cand.V7ReasonCodes = []string{"alt_ladder_taker_sell", "fresh_micro_confirmed", "fresh_rest_confirmed"}
		cand.V7ConfirmSummary.ContextChecks = []local.V7ConfirmationCheck{
			{Code: "5m_or_15m_close_below_trigger", Passed: false, Severity: local.V7ConfirmContext},
			{Code: "bb_width_expansion_starts", Passed: false, Severity: local.V7ConfirmContext},
		}
		p := &hunterV7PendingEntry{
			Decision:          *d,
			Candidate:         cand,
			Side:              "short",
			Trigger:           bless.zone.Lower,
			Invalidation:      bless.decisionSL,
			CreatedAt:         t0,
			ReroutedFromGuard: true,
			ReroutedBlockCode: "5m_or_15m_close_below_trigger",
			FiredBarClose:     0.013810,
			FiredBarCloseTime: m.Add(time.Minute).UnixMilli(),
		}
		at.conditional.latest = func(string) *datafetch.SymbolSnapshot {
			return &datafetch.SymbolSnapshot{Symbol: bless.symbol, Price: 0.013810, Timestamp: t0.Add(time.Minute)}
		}
		fc := at.fireTimeCandidate(p, t0.Add(time.Minute+2*time.Second))
		if containsStringValue(fc.V7ReasonCodes, "fresh_micro_confirmed") ||
			containsStringValue(fc.V7ReasonCodes, "fresh_rest_confirmed") ||
			!containsStringValue(fc.V7ReasonCodes, "alt_ladder_taker_sell") {
			t.Fatalf("unexpected fire-time V7ReasonCodes: %v", fc.V7ReasonCodes)
		}
		// Must not mutate p.Candidate.V7ReasonCodes in place.
		if !containsStringValue(p.Candidate.V7ReasonCodes, "fresh_micro_confirmed") {
			t.Fatalf("fireTimeCandidate mutated stored p.Candidate.V7ReasonCodes: %v", p.Candidate.V7ReasonCodes)
		}
		if len(fc.V7ConfirmSummary.ContextChecks) != 2 ||
			!fc.V7ConfirmSummary.ContextChecks[0].Passed ||
			fc.V7ConfirmSummary.ContextChecks[0].Evidence == nil ||
			fc.V7ConfirmSummary.ContextChecks[1].Passed {
			t.Fatalf("expected only ReroutedBlockCode settled in ContextChecks, got %+v", fc.V7ConfirmSummary.ContextChecks)
		}
	})

	t.Run("survives_restart_and_fires_after_rearm", func(t *testing.T) {
		fireClose := 0.013816
		stubCoinAnkMarketTransport(t, fireClose)
		at1, _ := p02Trader(t, "p02-e2e-restart", true, st, fireClose)
		ctx, d := p02BuildCase(bless)
		if !at1.runGuardedDecision(ctx, d, &store.DecisionAction{Action: d.Action, Symbol: d.Symbol}, &store.DecisionRecord{}, t0) {
			t.Fatalf("reroute failed")
		}

		// Simulate restart 2 minutes later.
		at2, ft2 := p02Trader(t, "p02-e2e-restart", true, st, fireClose)
		restartAt := m.Add(2*time.Minute + 10*time.Second)
		at2.restorePendingConditionalEntries(restartAt)
		restored := at2.pendingConditionalEntries()
		if len(restored) != 1 || !restored[0].ReroutedFromGuard ||
			restored[0].ReroutedBlockCode != "5m_or_15m_close_below_trigger" {
			t.Fatalf("restored pending entry lost reroute provenance: %+v", restored)
		}
		at2.conditional.bars = func(string) []datafetch.Kline {
			return []datafetch.Kline{
				bar(m.Add(3*time.Minute), 0.013840, 0.013805, fireClose),
			}
		}
		at2.conditional.latest = func(string) *datafetch.SymbolSnapshot {
			return &datafetch.SymbolSnapshot{Symbol: bless.symbol, Price: fireClose, Timestamp: time.Now().Add(-2 * time.Second)}
		}
		at2.checkHunterV7ConditionalEntries(m.Add(4*time.Minute + 5*time.Second))
		if ft2.opens != 1 || len(at2.pendingConditionalEntries()) != 0 {
			t.Fatalf("restarted rerouted entry failed to fire: opens=%d pending=%d", ft2.opens, len(at2.pendingConditionalEntries()))
		}
		firedRows, _ := st.Decision().ListLifecycles(at2.id, store.LifecycleStatusEntryFired)
		if len(firedRows) != 1 || firedRows[0].EntrySource != store.LifecycleEntrySourceGuardConfirmReroute {
			t.Fatalf("fired row after restart missing provenance: %+v", firedRows)
		}
	})
}

func TestP02ClosedBarTriggerCodeSetCompleteness(t *testing.T) {
	wantTrue := map[string]bool{
		"5m_or_15m_close_below_trigger":           true,
		"5m_or_15m_close_above_trigger":           true,
		"5m_or_15m_close_through_breakout_level":  true,
		"5m_or_15m_close_above_entry_zone":        true,
	}
	for code := range wantTrue {
		if !local.V7ConfirmIsClosedBarTrigger(code) {
			t.Errorf("expected V7ConfirmIsClosedBarTrigger(%q)=true", code)
		}
	}
	// Every other live-verifiable confirmation code must return false.
	for _, code := range local.V7ConfirmLiveVerifiableCodes() {
		if got := local.V7ConfirmIsClosedBarTrigger(code); got != wantTrue[code] {
			t.Errorf("V7ConfirmIsClosedBarTrigger(%q) = %v, want %v", code, got, wantTrue[code])
		}
	}
	// Representative non-trigger codes (indicator closes, patterns, flow, checklist).
	for _, nonTrigger := range []string{
		"directional_15m_close_long",
		"directional_15m_close_short",
		"5m_price_holds_ema20_or_trailing_support",
		"15m_close_below_vwap_or_ema20",
		"15m_close_below_vwap",
		"5m_close_below_ema20_or_entry_zone_mid",
		"no_new_high_after_rejection",
		"taker_buy_15m_lt_0_48",
		"live_price_in_entry_zone",
		"risk_reward_min_rr",
		"5m_close_above_trigger", // LLM checklist only, not machine-verified
		"15m_breakout_close",     // LLM checklist only, not machine-verified
		fmt.Sprintf("%s", ""),
	} {
		if local.V7ConfirmIsClosedBarTrigger(nonTrigger) {
			t.Errorf("expected V7ConfirmIsClosedBarTrigger(%q)=false", nonTrigger)
		}
	}
}

func TestP02Reroute_TriggerAnchoredToConfirmationEdgeAndTighterStop(t *testing.T) {
	t0 := time.Date(2026, 11, 28, 1, 0, 0, 0, time.UTC)
	m := t0.Truncate(time.Minute)
	st := newLifecycleStore(t)

	t.Run("trend_pullback_short_inverted_trigger_not_rerouted", func(t *testing.T) {
		at, ft := p02Trader(t, "p02-tp-short", true, st, 100.0)
		// trend_pullback_short shape: V7TriggerPrice=100 (entry near EMA20) while
		// V7EntryZone=[95, 100.27] (predicate evaluates zone.Lower=95).
		cand := kernel.CandidateCoin{
			Symbol:             "TPSUSDT",
			Direction:          "SHORT",
			V7SignalID:         "sig-tps",
			V7SetupType:        "trend_pullback_short",
			V7SignalClass:      store.V7ClassTrigger,
			V7ExecutionTier:    "REVIEWABLE",
			V7Confidence:       "B",
			V7EntryZone:        local.V7PriceZone{Lower: 95.0, Upper: 100.27},
			V7TriggerPrice:     100.0,
			V7Invalidation:     local.V7InvalidationRule{Price: 102.5},
			V7RequiredConfirms: []string{"5m_or_15m_close_below_trigger"},
			V7ReasonCodes:      []string{"fresh_micro_confirmed", "fresh_rest_confirmed"},
			V7PriceContext:     &local.V7PriceContext{Last: 100.0},
			V7ConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{
				"5m":  {Timeframe: "5m", LastClose: 100.0},
				"15m": {Timeframe: "15m", LastClose: 100.0},
			},
			V7ConfirmSummary: &local.V7ConfirmationSummary{
				PassedHard:   true,
				PassedReview: false,
				MissingReview: []local.V7ConfirmationCheck{
					{Code: "5m_or_15m_close_below_trigger", Passed: false, Actual: 100.0, Threshold: 95.0, Severity: local.V7ConfirmReviewWait},
				},
			},
		}
		d := &kernel.Decision{
			Symbol:                    "TPSUSDT",
			Action:                    "open_short",
			Leverage:                  5,
			PositionSizeUSD:           50,
			Price:                     100.0,
			StopLoss:                  102.5,
			TakeProfit:                95.5,
			Confidence:                80,
			SelectedHunterV7SignalID:  "sig-tps",
			HunterV7TriggerPrice:      100.0,
			HunterV7InvalidationPrice: 102.5,
		}
		act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
		ok := at.runGuardedDecision(&kernel.Context{CandidateCoins: []kernel.CandidateCoin{cand}}, d, &act, &store.DecisionRecord{}, t0)
		if ok || act.ConditionalPending || act.ConditionalRerouteSource != "" ||
			act.BlockedReasonCode != "" || len(at.pendingConditionalEntries()) != 0 || ft.opens != 0 {
			t.Fatalf("trend_pullback_short with loose/inverted trigger must NOT reroute: ok=%v act=%+v pending=%d opens=%d",
				ok, act, len(at.pendingConditionalEntries()), ft.opens)
		}
	})

	t.Run("trend_pullback_long_inverted_trigger_not_rerouted", func(t *testing.T) {
		cand := &kernel.CandidateCoin{
			V7EntryZone:    local.V7PriceZone{Lower: 99.73, Upper: 105.0},
			V7TriggerPrice: 100.0, // loose/inverted for LONG (100 < edge 105)
			V7Invalidation: local.V7InvalidationRule{Price: 97.5},
		}
		d := &kernel.Decision{
			Action:                    "open_long",
			StopLoss:                  97.5,
			HunterV7TriggerPrice:      100.0,
			HunterV7InvalidationPrice: 97.5,
		}
		if trig, inval, ok := hunterV7RerouteGeometry(cand, d, "long"); ok {
			t.Fatalf("expected LONG loose/inverted trigger to be ineligible, got trig=%.4f inval=%.4f", trig, inval)
		}
	})

	t.Run("stricter_explicit_trigger_registers_at_predicate_edge", func(t *testing.T) {
		at, _ := p02Trader(t, "p02-stricter-trig", true, st, 100.0)
		zone := local.V7PriceZone{Lower: 99.7, Upper: 100.3}
		cand := kernel.CandidateCoin{
			Symbol:             "STRUSDT",
			Direction:          "SHORT",
			V7SignalID:         "sig-str",
			V7SetupType:        "breakdown_momentum_short",
			V7SignalClass:      store.V7ClassTrigger,
			V7ExecutionTier:    "REVIEWABLE",
			V7Confidence:       "B",
			V7EntryZone:        zone,
			V7TriggerPrice:     99.4, // stricter than zone.Lower=99.7
			V7Invalidation:     local.V7InvalidationRule{Price: 102.5},
			V7RequiredConfirms: []string{"5m_or_15m_close_below_trigger"},
			V7ReasonCodes:      []string{"fresh_micro_confirmed", "fresh_rest_confirmed"},
			V7PriceContext:     &local.V7PriceContext{Last: 100.0},
			V7ConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{
				"5m":  {Timeframe: "5m", LastClose: 100.0},
				"15m": {Timeframe: "15m", LastClose: 100.0},
			},
			V7ConfirmSummary: &local.V7ConfirmationSummary{
				PassedHard:    true,
				PassedReview:  false,
				MissingReview: []local.V7ConfirmationCheck{{Code: "5m_or_15m_close_below_trigger", Passed: false, Severity: local.V7ConfirmReviewWait}},
			},
		}
		d := &kernel.Decision{
			Symbol:                    "STRUSDT",
			Action:                    "open_short",
			Leverage:                  5,
			PositionSizeUSD:           50,
			Price:                     100.0,
			StopLoss:                  102.5,
			TakeProfit:                95.0,
			Confidence:                80,
			SelectedHunterV7SignalID:  "sig-str",
			HunterV7TriggerPrice:      99.4,
			HunterV7InvalidationPrice: 102.5,
		}
		act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
		if !at.runGuardedDecision(&kernel.Context{CandidateCoins: []kernel.CandidateCoin{cand}}, d, &act, &store.DecisionRecord{}, t0) {
			t.Fatalf("expected stricter explicit trigger to reroute at predicate edge, got act=%+v", act)
		}
		pending := at.pendingConditionalEntries()
		if len(pending) != 1 || pending[0].Trigger != zone.Lower || act.Price != zone.Lower {
			t.Fatalf("expected registered trigger == zone.Lower (%.4f), got pending=%+v act=%+v", zone.Lower, pending, act)
		}
	})

	t.Run("native_breakdown_zero_trigger_reroutes_at_zone_lower", func(t *testing.T) {
		bless := p02LiveShapes()[0]
		at, _ := p02Trader(t, "p02-native-zero", true, st, bless.decisionPx)
		ctx, d := p02BuildCase(bless)
		if ctx.CandidateCoins[0].V7TriggerPrice != 0 || d.HunterV7TriggerPrice != 0 {
			t.Fatalf("precondition failed: native shape must have zero explicit trigger")
		}
		act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
		if !at.runGuardedDecision(ctx, d, &act, &store.DecisionRecord{}, t0) {
			t.Fatalf("expected native breakdown with V7TriggerPrice=0 to reroute, got act=%+v", act)
		}
		pending := at.pendingConditionalEntries()
		if len(pending) != 1 || pending[0].Trigger != bless.zone.Lower {
			t.Fatalf("expected trigger == zone.Lower (%.12f), got %+v", bless.zone.Lower, pending)
		}
	})

	t.Run("tighter_llm_stop_becomes_pending_invalidation_and_cancels_on_1m_cross", func(t *testing.T) {
		bless := p02LiveShapes()[0]
		at, ft := p02Trader(t, "p02-tight-stop", true, st, bless.decisionPx)
		ctx, d := p02BuildCase(bless)
		// Existing candidate/decision invalidation is 0.014162876; LLM places a
		// tighter stop at 0.01411 (>2% above trigger 0.0138168 so MinStopLoss passes).
		tightStop := 0.01411
		d.StopLoss = tightStop

		act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
		if !at.runGuardedDecision(ctx, d, &act, &store.DecisionRecord{}, t0) {
			t.Fatalf("expected reroute with tighter LLM stop to succeed, got act=%+v", act)
		}
		pending := at.pendingConditionalEntries()
		if len(pending) != 1 || pending[0].Invalidation != tightStop {
			t.Fatalf("expected pending invalidation == tighter LLM stop %.8f, got %+v", tightStop, pending)
		}
		// 1m candle crosses LLM stop (High=0.01412 >= 0.01411) WITHOUT reaching
		// candidate invalidation (0.014162876), even while closing below trigger:
		// stop-first must cancel the pending entry.
		at.conditional.bars = func(string) []datafetch.Kline {
			return []datafetch.Kline{bar(m, 0.01412, 0.013800, 0.013810)}
		}
		at.checkHunterV7ConditionalEntries(m.Add(time.Minute + 5*time.Second))
		if ft.opens != 0 || len(at.pendingConditionalEntries()) != 0 {
			t.Fatalf("1m bar crossing tighter LLM stop must cancel pending entry: opens=%d pending=%d",
				ft.opens, len(at.pendingConditionalEntries()))
		}
		rows, _ := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryCancelled)
		if len(rows) != 1 || !strings.Contains(rows[0].Note, "invalidation") {
			t.Fatalf("expected invalidation cancellation lifecycle row, got %+v", rows)
		}
	})

	t.Run("long_tighter_llm_stop_uses_max", func(t *testing.T) {
		cand := &kernel.CandidateCoin{
			V7EntryZone:    local.V7PriceZone{Lower: 99.5, Upper: 100.0},
			V7Invalidation: local.V7InvalidationRule{Price: 97.0},
		}
		d := &kernel.Decision{
			Action:                    "open_long",
			StopLoss:                  97.8, // tighter than 97.0 for LONG
			HunterV7InvalidationPrice: 97.0,
		}
		trig, inval, ok := hunterV7RerouteGeometry(cand, d, "long")
		if !ok || trig != 100.0 || inval != 97.8 {
			t.Fatalf("got trig=%.4f inval=%.4f ok=%v, want 100.0 / 97.8 / true", trig, inval, ok)
		}
	})
}

func TestP02Reroute_ValidationOrderAndConsumerFiltering(t *testing.T) {
	t0 := time.Date(2026, 11, 28, 0, 22, 24, 0, time.UTC)
	bless := p02LiveShapes()[0]

	t.Run("drift_failure_with_enabled_false_records_failed_not_disabled", func(t *testing.T) {
		at, ft := p02Trader(t, "p02-val-order", false, nil, bless.decisionPx)
		ctx, d := p02BuildCase(bless)
		// Decision price at zone.Upper (~0.96% above trigger zone.Lower) exceeds 0.5% drift.
		d.Price = bless.zone.Upper
		ctx.CandidateCoins[0].V7PriceContext.Last = bless.zone.Upper

		act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
		rec := &store.DecisionRecord{}
		ok := at.runGuardedDecision(ctx, d, &act, rec, t0)
		if ok || act.Success || act.ConditionalPending || ft.opens != 0 || len(at.pendingConditionalEntries()) != 0 {
			t.Fatalf("expected fail-closed rejection, got ok=%v act=%+v", ok, act)
		}
		if act.BlockedReasonCode != store.V7BlockedReasonConditionalRerouteFailed {
			t.Fatalf("infeasible trigger with enabled=false must record %q, got %q (err=%q)",
				store.V7BlockedReasonConditionalRerouteFailed, act.BlockedReasonCode, act.Error)
		}
		if act.ConditionalRerouteSource != "5m_or_15m_close_below_trigger" || !strings.Contains(act.Error, "drift") {
			t.Fatalf("expected drift error with reroute source preserved, got act=%+v", act)
		}
	})

	t.Run("trigger_drift_reference_hydrated_before_validateOpenDecision", func(t *testing.T) {
		// Native candidate with V7TriggerPrice=0 and HunterV7DriftReference="trigger":
		// even when decision.Price is >0.5% above zone.Lower, hydrating
		// storedDecision.HunterV7TriggerPrice = trigger BEFORE validateOpenDecision
		// measures drift from trigger (0.0%), allowing registration.
		at, _ := p02Trader(t, "p02-trig-ref", true, nil, bless.decisionPx)
		ctx, d := p02BuildCase(bless)
		d.Price = bless.zone.Upper // ~0.96% above Lower
		ctx.CandidateCoins[0].V7PriceContext.Last = bless.zone.Upper
		d.HunterV7DriftReference = store.V7DriftReferenceTrigger

		act := store.DecisionAction{Action: d.Action, Symbol: d.Symbol}
		if !at.runGuardedDecision(ctx, d, &act, &store.DecisionRecord{}, t0) {
			t.Fatalf("expected trigger-referenced drift check to pass after trigger hydration, got act=%+v", act)
		}
		if !act.ConditionalPending || len(at.pendingConditionalEntries()) != 1 {
			t.Fatalf("expected 1 pending entry, got act=%+v pending=%d", act, len(at.pendingConditionalEntries()))
		}
	})

	t.Run("latestOpenDecisionRiskByPosition_ignores_conditional_pending", func(t *testing.T) {
		st := newLifecycleStore(t)
		at, _ := p02Trader(t, "p02-risk-filter", true, st, bless.decisionPx)

		// Cycle 1: real filled short on BLESSUSDT (SL=0.0145, TP=0.0125).
		if err := st.Decision().LogDecision(&store.DecisionRecord{
			TraderID:    at.id,
			CycleNumber: 1,
			Success:     true,
			Decisions: []store.DecisionAction{
				{
					Action:     "open_short",
					Symbol:     "BLESSUSDT",
					StopLoss:   0.0145,
					TakeProfit: 0.0125,
					Success:    true,
				},
			},
		}); err != nil {
			t.Fatalf("log cycle 1: %v", err)
		}
		// Cycle 2: newer pending conditional registrations (Success=true, ConditionalPending=true)
		// on both BLESSUSDT (different SL/TP) and ICNTUSDT (no filled open).
		if err := st.Decision().LogDecision(&store.DecisionRecord{
			TraderID:    at.id,
			CycleNumber: 2,
			Success:     true,
			Decisions: []store.DecisionAction{
				{
					Action:             "open_short",
					Symbol:             "BLESSUSDT",
					StopLoss:           0.0141,
					TakeProfit:         0.0131,
					Success:            true,
					ConditionalPending: true,
				},
				{
					Action:             "open_short",
					Symbol:             "ICNTUSDT",
					StopLoss:           0.3009,
					TakeProfit:         0.2790,
					Success:            true,
					ConditionalPending: true,
				},
			},
		}); err != nil {
			t.Fatalf("log cycle 2: %v", err)
		}

		risks := at.latestOpenDecisionRiskByPosition(10)
		gotBless, ok := risks[positionRiskKey("BLESSUSDT", "SHORT")]
		if !ok || gotBless.stopLoss != 0.0145 || gotBless.takeProfit != 0.0125 {
			t.Fatalf("expected filled Cycle 1 SL/TP (0.0145/0.0125), got ok=%v risk=%+v", ok, gotBless)
		}
		if _, exists := risks[positionRiskKey("ICNTUSDT", "SHORT")]; exists {
			t.Fatalf("pending-only conditional registration must NOT populate latestOpenDecisionRiskByPosition: %+v", risks)
		}
	})
}
