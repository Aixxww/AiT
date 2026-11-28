package kernel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Unit U4 (M2a): mechanism reproduction tests for the three-way confirmation
// evaluator divergence on AIK11's 3 guard-rejected approvals (2026-11-28):
//   - C16 (sig sc14) BLESSUSDT breakdown_momentum_short
//   - C18 (sig sc16) ICNTUSDT  alt_ladder_breakdown_short
//   - C31 (sig sc29) PONSUSDT  alt_ladder_breakdown_short
//
// Historical 5m/15m kline series are NOT present in the postmortem data pack
// and are explicitly NOT claimed to be historical bars. Each fixture builds a
// synthetic kline series anchored to the case's recorded geometry from
// sig.jsonl + dr.jsonl (entry_zone, invalidation, tp1/tp2, recovered P_scan,
// and recorded/bounded P_live) so the real production functions can be driven
// without reimplementation.

type u4CaseFixture struct {
	name            string
	drCycle         int
	sigCycle        int
	symbol          string
	setupType       local.V7SetupType
	entryMode       local.V7EntryMode
	zone            local.V7PriceZone
	invalidation    float64
	tp0, tp1, tp2   float64
	pScan           float64 // exact scan price recovered from module zone formula (inv/pScan verified)
	pLive           float64 // synthetic live price <= zone.Lower (BLESS matches dist_zone_lower=+0.11%, dist_zone_upper=+1.07%)
	taker15m        float64 // recorded in dr.jsonl LLM reasoning
	oi1h            float64 // recorded in dr.jsonl LLM reasoning
	oi4h            float64
	aiPriority      float64
	timingScore     float64
	liquidityScore  float64
	riskScore       float64
	promptTimeUTC   time.Time
	required        []string
	reasonCodes     []string
	wantAssemTier   string
	wantAssemReason string
	wantLiveTier    string
	wantLiveReason  string
}

func u4LiveStrategyConfig() *store.StrategyConfig {
	return &store.StrategyConfig{
		Language:          "zh",
		PromptCompactMode: "hunter_v7_only",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
			Hunter: &store.HunterConfig{
				V7MaxOutput:     18,
				V7MinAIPriority: 50,
				V7Aggressive:    true,
				V7T2Enabled:     true,
			},
		},
		Indicators: store.IndicatorConfig{
			Klines: store.KlineConfig{
				PrimaryTimeframe:     "5m",
				PrimaryCount:         30,
				LongerTimeframe:      "4h",
				LongerCount:          10,
				EnableMultiTimeframe: true,
				SelectedTimeframes:   []string{"5m", "15m", "1h", "4h"},
			},
			EnableRawKlines: true,
			EnableEMA:       true,
			EnableVolume:    true,
		},
		RiskControl: store.RiskControlConfig{
			MaxPositions:              2,
			MinRiskRewardRatio:        1.2,
			MinStopLossPriceMovePct:   2.0,
			MaxTakeProfitPriceMovePct: 3.5,
			MaxEntryPriceDeviationPct: 0.5,
		},
	}
}

func u4RecordedCases() []u4CaseFixture {
	return []u4CaseFixture{
		{
			name:            "C16_BLESSUSDT",
			drCycle:         16,
			sigCycle:        14,
			symbol:          "BLESSUSDT",
			setupType:       local.V7SetupBreakdownShort,
			entryMode:       local.V7EntryFastConfirm,
			zone:            local.V7PriceZone{Lower: 0.013816808611979915, Upper: 0.013949536417822414},
			invalidation:    0.014162876000000001,
			tp0:             0.013806906726227251,
			tp1:             0.01290182148192,
			tp2:             0.012116740655535068,
			pScan:           0.013858, // inv/pScan = 1.02200000 exact
			pLive:           0.013802, // gives dist_zone_lower=+0.11%, dist_zone_upper=+1.07%
			taker15m:        0.390,
			oi1h:            -0.44,
			oi4h:            -1.0,
			aiPriority:      93.085,
			timingScore:     70,
			liquidityScore:  100,
			riskScore:       0,
			promptTimeUTC:   time.Date(2026, 11, 28, 0, 22, 14, 0, time.UTC),
			required:        []string{"5m_or_15m_close_below_trigger", "taker_buy_15m_lt_0_48", "no_immediate_reclaim_of_trigger_level"},
			reasonCodes:     []string{"solid_1h_downside_momentum", "below_vwap_breakdown", "heavy_taker_selling"},
			wantAssemTier:   "REVIEWABLE",
			wantAssemReason: "live_reviewable_5m_or_15m_close_below_trigger",
			wantLiveTier:    "EXECUTABLE",
			wantLiveReason:  "short_or_reversion_ready_confirmed",
		},
		{
			name:            "C18_ICNTUSDT",
			drCycle:         18,
			sigCycle:        16,
			symbol:          "ICNTUSDT",
			setupType:       local.V7SetupAltLadderShort,
			entryMode:       local.V7EntryFastConfirm,
			zone:            local.V7PriceZone{Lower: 0.29349855988945545, Upper: 0.2966459430940239},
			invalidation:    0.300979,
			tp0:             0.2903468526851314,
			tp1:             0.2757250939416751,
			tp2:             0.26024736790162345,
			pScan:           0.294500, // inv/pScan = 1.02200000 exact; raw RR(tp1)=2.90 exact
			pLive:           0.293300, // below_zone, dist_zone_upper=+1.14% (matches "距区上沿+1.1...")
			taker15m:        0.343,
			oi1h:            -2.65,
			oi4h:            -11.0,
			aiPriority:      88.085,
			timingScore:     66, // explicitly quoted in dr.jsonl C18 reasoning
			liquidityScore:  90,
			riskScore:       0,
			promptTimeUTC:   time.Date(2026, 11, 28, 0, 42, 14, 0, time.UTC),
			required:        []string{"live_price_in_entry_zone", "taker_buy_15m_lt_0_48", "5m_or_15m_close_below_trigger", "taker_flow_not_flipping_against_direction"},
			reasonCodes:     []string{"alt_ladder_breakdown_short", "alt_ladder_downshift_mid", "alt_ladder_taker_sell", "alt_ladder_long_flush"},
			wantAssemTier:   "WATCH",
			wantAssemReason: "alt_ladder_short_rebound_pending",
			wantLiveTier:    "EXECUTABLE",
			wantLiveReason:  "alt_ladder_short_ready_strong_confirmed",
		},
		{
			name:            "C31_PONSUSDT",
			drCycle:         31,
			sigCycle:        29,
			symbol:          "PONSUSDT",
			setupType:       local.V7SetupAltLadderShort,
			entryMode:       local.V7EntryFastConfirm,
			zone:            local.V7PriceZone{Lower: 0.1006455034011606, Upper: 0.10144534985465585},
			invalidation:    0.10291800000000001,
			tp0:             0.10011029745610295,
			tp1:             0.0952076579068888,
			tp2:             0.09053744293007326,
			pScan:           0.100900, // inv/pScan = 1.02000000 exact
			pLive:           0.100500, // synthetic within proven live band [0.1000013, 0.1006455]
			taker15m:        0.334,
			oi1h:            2.04,
			oi4h:            1.5,
			aiPriority:      92.085,
			timingScore:     70,
			liquidityScore:  100,
			riskScore:       0,
			promptTimeUTC:   time.Date(2026, 11, 28, 2, 52, 14, 0, time.UTC),
			required:        []string{"live_price_in_entry_zone", "taker_buy_15m_lt_0_48", "5m_or_15m_close_below_trigger", "taker_flow_not_flipping_against_direction"},
			reasonCodes:     []string{"alt_ladder_breakdown_short", "alt_ladder_downshift_mid", "alt_ladder_taker_sell", "alt_ladder_new_shorts"},
			wantAssemTier:   "REVIEWABLE",
			wantAssemReason: "live_reviewable_5m_or_15m_close_below_trigger",
			wantLiveTier:    "EXECUTABLE",
			wantLiveReason:  "alt_ladder_short_ready_strong_confirmed",
		},
	}
}

// u4SyntheticSeries builds nClosed strictly closed bars at closedPrice plus
// 1 trailing forming (unclosed) bar at formingClose anchored to promptTime.
func u4SyntheticSeries(tf string, iv time.Duration, promptTime time.Time, nClosed int, closedPrice, formingClose float64) *market.TimeframeSeriesData {
	formingOpen := promptTime.Truncate(iv)
	total := nClosed + 1
	klines := make([]market.KlineBar, total)
	ema20 := make([]float64, total)
	for i := 0; i < nClosed; i++ {
		openAt := formingOpen.Add(-time.Duration(nClosed-i) * iv)
		klines[i] = market.KlineBar{
			Time:   openAt.UnixMilli(),
			Open:   closedPrice,
			High:   closedPrice * 1.001,
			Low:    closedPrice * 0.999,
			Close:  closedPrice,
			Volume: 1000,
		}
		ema20[i] = closedPrice
	}
	klines[nClosed] = market.KlineBar{
		Time:   formingOpen.UnixMilli(),
		Open:   closedPrice,
		High:   closedPrice,
		Low:    formingClose,
		Close:  formingClose,
		Volume: 1000,
	}
	ema20[nClosed] = closedPrice
	return &market.TimeframeSeriesData{
		Timeframe:   tf,
		Klines:      klines,
		EMA20Values: ema20,
		ATR14:       closedPrice * 0.005,
	}
}

// u4MarketData builds prompt-cycle market data with both 5m and 15m series
// whose closed bars settle at closedPrice and whose trailing forming bar +
// CurrentPrice sit at livePrice.
func u4MarketData(symbol string, promptTime time.Time, closedPrice, livePrice float64) *market.Data {
	return &market.Data{
		Symbol:       symbol,
		CurrentPrice: livePrice,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"5m":  u4SyntheticSeries("5m", 5*time.Minute, promptTime, 25, closedPrice, livePrice),
			"15m": u4SyntheticSeries("15m", 15*time.Minute, promptTime, 25, closedPrice, livePrice),
		},
	}
}

// u4AssembleCandidate runs the real scoring-time EvaluateV7Confirmations and
// assembly-time classifyHunterV7CandidateTierWithGeometry for a case at P_scan,
// matching the scanner snapshot recorded in sig.jsonl.
func u4AssembleCandidate(t *testing.T, tc u4CaseFixture, geom HunterV7ExecutionGeometry) CandidateCoin {
	t.Helper()
	closedBars := make([]local.V7Bar, 25)
	for i := range closedBars {
		closedBars[i] = local.V7Bar{
			High:   tc.pScan * 1.001,
			Low:    tc.pScan * 0.999,
			Close:  tc.pScan,
			Volume: 1000,
		}
	}
	tf5, ok5 := local.V7TimeframeFactsFromBars("5m", closedBars, tc.pScan)
	tf15, ok15 := local.V7TimeframeFactsFromBars("15m", closedBars, tc.pScan)
	if !ok5 || !ok15 {
		t.Fatalf("%s: failed to build scoring timeframe facts", tc.name)
	}
	sig := &local.V7SignalOutput{
		SignalID:         "sig_" + tc.symbol,
		Symbol:           tc.symbol,
		Direction:        local.V7DirShort,
		SetupType:        tc.setupType,
		Status:           local.V7StatusCandidate,
		EntryMode:        tc.entryMode,
		ExecutionQuality: local.V7ExecReady,
		Confidence:       "B",
		RiskLevel:        local.V7RiskLow,
		AIPriority:       tc.aiPriority,
		SetupScore:       90,
		TimingScore:      tc.timingScore,
		RegimeFitScore:   80,
		LiquidityScore:   tc.liquidityScore,
		RiskScore:        tc.riskScore,
		EntryZone:        tc.zone,
		Invalidation:     local.V7InvalidationRule{Price: tc.invalidation},
		Targets:          []local.V7Target{{Price: tc.tp1}, {Price: tc.tp2}},
		RequiredConfirms: append([]string{}, tc.required...),
		ReasonCodes:      append([]string{}, tc.reasonCodes...),
	}
	v7Cfg := local.DefaultV7Config()
	v7Cfg.MinRiskRewardRatio = geom.MinRR
	symCtx := &local.V7SymbolContext{
		Symbol:         tc.symbol,
		CurrentPrice:   tc.pScan,
		TakerBuy15m:    tc.taker15m,
		VolumeBurst15m: 1.5,
		Snapshot:       &local.SymbolSnapshotData{OIDelta1h: tc.oi1h, OIDelta4h: tc.oi4h},
		ExecutionContext: &local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes:  map[string]local.V7ExecutionTimeframeSummary{"5m": tf5, "15m": tf15},
		},
	}
	summary := local.EvaluateV7Confirmations(sig, symCtx, v7Cfg)

	coin := CandidateCoin{
		Symbol:             tc.symbol,
		Sources:            []string{"hunter_v7"},
		Direction:          "SHORT",
		V7SignalID:         sig.SignalID,
		V7SetupType:        string(tc.setupType),
		V7Status:           string(local.V7StatusCandidate),
		V7EntryMode:        string(tc.entryMode),
		V7ExecutionQuality: string(local.V7ExecReady),
		V7Confidence:       "B",
		V7RiskLevel:        "LOW",
		V7AIPriority:       tc.aiPriority,
		V7SetupScore:       90,
		V7TimingScore:      tc.timingScore,
		V7RegimeFitScore:   80,
		V7LiquidityScore:   tc.liquidityScore,
		V7RiskScore:        tc.riskScore,
		V7EntryZone:        tc.zone,
		V7Invalidation:     local.V7InvalidationRule{Price: tc.invalidation},
		V7Targets:          []local.V7Target{{Price: tc.tp1}, {Price: tc.tp2}},
		V7TP0Price:         tc.tp0,
		V7TP1Price:         tc.tp1,
		V7TP2Price:         tc.tp2,
		V7RequiredConfirms: append([]string{}, tc.required...),
		V7ReasonCodes:      append([]string{}, tc.reasonCodes...),
		V7ConfirmSummary:   &summary,
		V7PriceContext:     &local.V7PriceContext{Last: tc.pScan, Change1h: -3.5, Change4h: -6.0},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: tc.taker15m, OIChange1h: tc.oi1h, OIChange4h: tc.oi4h},
		V7ExecutionContext: symCtx.ExecutionContext,
	}
	coin.V7ExecutionTier, coin.V7TierReason = classifyHunterV7CandidateTierWithGeometry(coin, geom)
	return coin
}

// TestU4_TriggerPredicateIgnoresClosedBarsAndUsesLivePrice pins Mechanism 1:
// 5m_or_15m_close_below_trigger (v7PriceThroughEdgePredicate in
// provider/local/hunter_v7_confirm_predicates.go:115,250) NEVER inspects
// f.Timeframes (neither closed nor forming bar closes). It compares live
// f.Price <= f.EntryZone.Lower. Consequently:
//   - Case A (forming-only dip): ALL closed 5m/15m bars sit strictly above
//     entry_zone_lower while live/forming price dips below -> live verifier
//     returns passed=true (false closed-bar confirmation).
//   - Case B (mirror): ALL closed 5m/15m bars closed strictly below
//     entry_zone_lower while live price bounces above -> live verifier returns
//     passed=false (ignoring the genuinely closed breakdown bars).
func TestU4_TriggerPredicateIgnoresClosedBarsAndUsesLivePrice(t *testing.T) {
	cfg := u4LiveStrategyConfig()
	eng := NewStrategyEngine(cfg)
	geom := eng.hunterV7ExecutionGeometry()

	for _, tc := range u4RecordedCases() {
		t.Run(tc.name, func(t *testing.T) {
			coin := u4AssembleCandidate(t, tc, geom)

			// Case A: closed 5m/15m bars all sit at P_scan (> EntryZone.Lower),
			// only the forming bar + CurrentPrice dipped to P_live (<= EntryZone.Lower).
			formingOnly := u4MarketData(tc.symbol, tc.promptTimeUTC, tc.pScan, tc.pLive)
			for _, tf := range []string{"5m", "15m"} {
				closed := hunterV7ClosedKlines(formingOnly.TimeframeData[tf], tf, tc.promptTimeUTC.UnixMilli())
				if len(closed) != 25 {
					t.Fatalf("%s %s: expected 25 closed bars, got %d", tc.name, tf, len(closed))
				}
				if closed[len(closed)-1].Close <= tc.zone.Lower {
					t.Fatalf("%s %s: fixture bug: last closed bar must be > zone.Lower", tc.name, tf)
				}
			}
			passed, known := hunterV7VerifyLiveConfirmation(coin, formingOnly, "5m_or_15m_close_below_trigger")
			if !known || !passed {
				t.Fatalf("%s forming-only dip: got (passed=%v, known=%v), want (true, true) — predicate evaluates live CurrentPrice <= EntryZone.Lower without checking closed bars", tc.name, passed, known)
			}

			// Case B (mirror): every closed 5m/15m bar closed BELOW EntryZone.Lower
			// (at P_live), while CurrentPrice + forming bar bounced to P_scan (> EntryZone.Lower).
			closedBelowLiveAbove := u4MarketData(tc.symbol, tc.promptTimeUTC, tc.pLive, tc.pScan)
			for _, tf := range []string{"5m", "15m"} {
				closed := hunterV7ClosedKlines(closedBelowLiveAbove.TimeframeData[tf], tf, tc.promptTimeUTC.UnixMilli())
				if closed[len(closed)-1].Close > tc.zone.Lower {
					t.Fatalf("%s %s: mirror fixture bug: last closed bar must be <= zone.Lower", tc.name, tf)
				}
			}
			mirrorPassed, mirrorKnown := hunterV7VerifyLiveConfirmation(coin, closedBelowLiveAbove, "5m_or_15m_close_below_trigger")
			if !mirrorKnown || mirrorPassed {
				t.Fatalf("%s mirror (closed below, live above): got (passed=%v, known=%v), want (false, true) — confirms predicate ignores closed bars entirely", tc.name, mirrorPassed, mirrorKnown)
			}
		})
	}
}

// TestU4_FormingBarLeaksIntoTimeframeFactsForBarPredicates pins Mechanism 2:
// For shared predicates that DO read f.Timeframes (directional_15m_close_short,
// 5m_price_holds_ema20_or_trailing_support), HunterV7ConfirmFacts
// (kernel/engine_prompt_live_confirm.go:180-200) hands the full Klines slice —
// including the trailing unclosed forming bar — to local.V7TimeframeFactsFromBars,
// which unconditionally sets LastClose = bars[len(bars)-1].Close.
func TestU4_FormingBarLeaksIntoTimeframeFactsForBarPredicates(t *testing.T) {
	cfg := u4LiveStrategyConfig()
	eng := NewStrategyEngine(cfg)
	geom := eng.hunterV7ExecutionGeometry()

	for _, tc := range u4RecordedCases() {
		t.Run(tc.name, func(t *testing.T) {
			coin := u4AssembleCandidate(t, tc, geom)
			// Closed 5m & 15m bars all sit ABOVE zone upper (pAbove > zone.Upper > zoneMid,
			// so closed bars have LastClose == EMA20 == VWAP20 > zoneMid -> directional_15m_close_short
			// would FAIL if computed on closed bars, Wait: CloseVsEMA20Pct==0 passes <=0!
			// So make closed bars sit strictly ABOVE EMA20/VWAP20 and zoneMid by placing
			// the last closed bar above the earlier closed bars' EMA20/VWAP20).
			pBase := tc.zone.Upper * 1.005
			pClosedHigh := tc.zone.Upper * 1.015 // strictly > EMA20, > VWAP20, > zoneMid
			pFormingLow := tc.pLive              // strictly < EMA20, < VWAP20, < zoneMid

			mkSeries := func(tf string, iv time.Duration) *market.TimeframeSeriesData {
				s := u4SyntheticSeries(tf, iv, tc.promptTimeUTC, 25, pBase, pFormingLow)
				// Lift the last CLOSED bar (index 24) above pBase so its close is strictly above EMA20 & VWAP20.
				s.Klines[24].Close = pClosedHigh
				s.Klines[24].High = pClosedHigh
				s.EMA20Values[24] = pBase
				s.EMA20Values[25] = pBase
				return s
			}
			data := &market.Data{
				Symbol:       tc.symbol,
				CurrentPrice: pFormingLow,
				TimeframeData: map[string]*market.TimeframeSeriesData{
					"5m":  mkSeries("5m", 5*time.Minute),
					"15m": mkSeries("15m", 15*time.Minute),
				},
			}

			// Verify closed-bar slice excludes the forming bar and ends at pClosedHigh.
			closed15 := hunterV7ClosedKlines(data.TimeframeData["15m"], "15m", tc.promptTimeUTC.UnixMilli())
			if len(closed15) != 25 || closed15[len(closed15)-1].Close != pClosedHigh {
				t.Fatalf("closed 15m bars mismatch: len=%d last=%v", len(closed15), closed15[len(closed15)-1].Close)
			}
			closedBars15 := make([]local.V7Bar, len(closed15))
			for i, k := range closed15 {
				closedBars15[i] = local.V7Bar{High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume}
			}
			closedSummary15, _ := local.V7TimeframeFactsFromBars("15m", closedBars15, pBase)
			closedFacts := local.V7ConfirmFacts{
				Direction:  local.V7DirShort,
				SetupType:  tc.setupType,
				Price:      pFormingLow,
				EntryZone:  tc.zone,
				Timeframes: map[string]local.V7ExecutionTimeframeSummary{"15m": closedSummary15},
			}
			closedCheck, closedKnown, _ := local.V7EvaluateSharedConfirmation("directional_15m_close_short", closedFacts)
			if !closedKnown || closedCheck.Passed {
				t.Fatalf("expected directional_15m_close_short to FAIL on strictly closed bars (LastClose=%v vs EMA20=%v), got passed=%v known=%v",
					closedSummary15.LastClose, pBase, closedCheck.Passed, closedKnown)
			}

			// Live path: HunterV7ConfirmFacts includes the trailing forming bar!
			liveFacts := HunterV7ConfirmFacts(coin, data, pFormingLow)
			if got := liveFacts.Timeframes["15m"].LastClose; got != pFormingLow {
				t.Fatalf("HunterV7ConfirmFacts 15m LastClose = %v, want forming bar close %v", got, pFormingLow)
			}
			if got := liveFacts.Timeframes["5m"].LastClose; got != pFormingLow {
				t.Fatalf("HunterV7ConfirmFacts 5m LastClose = %v, want forming bar close %v", got, pFormingLow)
			}
			livePassed15, liveKnown15 := HunterV7VerifyConfirmation(coin, data, pFormingLow, "directional_15m_close_short")
			if !liveKnown15 || !livePassed15 {
				t.Fatalf("live directional_15m_close_short = (%v, %v), want (true, true) via forming bar leak", livePassed15, liveKnown15)
			}
			livePassed5, liveKnown5 := HunterV7VerifyConfirmation(coin, data, pFormingLow, "5m_price_holds_ema20_or_trailing_support")
			if !liveKnown5 || !livePassed5 {
				t.Fatalf("live 5m_price_holds_ema20_or_trailing_support (SHORT) = (%v, %v), want (true, true) via forming bar leak", livePassed5, liveKnown5)
			}
		})
	}
}

// TestU4_FinalizeTiersWriteBackSplitBrainOnThreeCases pins Mechanism 3 (the
// proximate cause of all 3 guard vetoes on C16, C18, C31):
//   1. At assembly (P_scan), 5m_or_15m_close_below_trigger fails because module
//      entry zones place EntryZone.Lower strictly below P_scan.
//   2. At prompt-final (FinalizeHunterV7Tiers -> hunterV7PromptItems), live
//      P_live <= EntryZone.Lower clears 5m_or_15m_close_below_trigger on the
//      prompt item's deep-copied V7ConfirmSummary (PassedReview=true) and
//      promotes the item to EXECUTABLE.
//   3. finalizeHunterV7Tiers (kernel/hunter_v7_final_tier.go:106-108) writes
//      ONLY V7ExecutionTier, V7TierReason, and V7RRVerdict back to
//      ctx.CandidateCoins — leaving ctx.CandidateCoins[i].V7ConfirmSummary
//      (PassedReview=false, MissingReview=[5m_or_15m_close_below_trigger]) and
//      V7PriceContext.Last (P_scan) stale!
//   4. The LLM payload renders from ctx.hunterV7Final.items (passed_review=true,
//      EXECUTABLE), while the guard reads ctx.CandidateCoins[i] (EXECUTABLE +
//      stale MissingReview + stale P_scan) and fails HunterV7VerifyConfirmation
//      with (data=nil, price=0).
func TestU4_FinalizeTiersWriteBackSplitBrainOnThreeCases(t *testing.T) {
	cfg := u4LiveStrategyConfig()
	eng := NewStrategyEngine(cfg)
	geom := eng.hunterV7ExecutionGeometry()

	for _, tc := range u4RecordedCases() {
		t.Run(tc.name, func(t *testing.T) {
			assemCoin := u4AssembleCandidate(t, tc, geom)

			// 1. Assembly stage matches sig.jsonl.
			if assemCoin.V7ExecutionTier != tc.wantAssemTier || assemCoin.V7TierReason != tc.wantAssemReason {
				t.Fatalf("%s assembly tier = (%q, %q), want (%q, %q)",
					tc.name, assemCoin.V7ExecutionTier, assemCoin.V7TierReason, tc.wantAssemTier, tc.wantAssemReason)
			}
			missingAssem := hunterV7MissingRequiredConfirmations(assemCoin)
			if len(missingAssem) != 1 || missingAssem[0] != "5m_or_15m_close_below_trigger" {
				t.Fatalf("%s assembly missing confirms = %v, want [5m_or_15m_close_below_trigger]", tc.name, missingAssem)
			}
			if hunterV7ConfirmationSummaryReviewPassed(assemCoin) {
				t.Fatalf("%s assembly PassedReview = true, want false", tc.name)
			}

			// 2. Prompt-final live re-tier with forming-only dip below EntryZone.Lower.
			ctx := &Context{
				PromptTime:     tc.promptTimeUTC,
				CandidateCoins: []CandidateCoin{assemCoin},
				MarketDataMap: map[string]*market.Data{
					tc.symbol: u4MarketData(tc.symbol, tc.promptTimeUTC, tc.pScan, tc.pLive),
				},
			}
			eng.FinalizeHunterV7Tiers(ctx)

			if ctx.hunterV7Final == nil || len(ctx.hunterV7Final.items) != 1 {
				t.Fatalf("%s: expected 1 finalized prompt item", tc.name)
			}
			promptItem := ctx.hunterV7Final.items[0]
			guardCoin := ctx.CandidateCoins[0]

			// Prompt payload side: promoted to EXECUTABLE with PassedReview=true.
			if promptItem.Tier != tc.wantLiveTier || promptItem.Reason != tc.wantLiveReason {
				t.Fatalf("%s prompt-final tier = (%q, %q), want (%q, %q)",
					tc.name, promptItem.Tier, promptItem.Reason, tc.wantLiveTier, tc.wantLiveReason)
			}
			if !hunterV7ConfirmationSummaryReviewPassed(promptItem.Coin) {
				t.Fatalf("%s prompt item PassedReview = false, want true", tc.name)
			}
			if !containsStringValue(promptItem.Coin.V7ReasonCodes, "live_confirmed_5m_or_15m_close_below_trigger") {
				t.Fatalf("%s prompt item missing live_confirmed_5m_or_15m_close_below_trigger in %v", tc.name, promptItem.Coin.V7ReasonCodes)
			}
			payload := buildHunterV7PromptPayload(promptItem.Coin)
			rawJSON, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			if !strings.Contains(string(rawJSON), `"passed_review":true`) || strings.Contains(string(rawJSON), `"missing_review"`) {
				t.Fatalf("%s payload JSON did not show clean passed_review=true: %s", tc.name, string(rawJSON))
			}

			// Split-brain on ctx.CandidateCoins[0] (what the trader guard reads):
			// V7ExecutionTier IS updated to EXECUTABLE, while V7ConfirmSummary,
			// V7PriceContext, and V7ReasonCodes remain at their stale scoring values!
			if guardCoin.V7ExecutionTier != tc.wantLiveTier {
				t.Fatalf("%s guardCoin.V7ExecutionTier = %q, want %q", tc.name, guardCoin.V7ExecutionTier, tc.wantLiveTier)
			}
			if hunterV7ConfirmationSummaryReviewPassed(guardCoin) {
				t.Fatalf("%s guardCoin.V7ConfirmSummary unexpectedly updated: PassedReview=true", tc.name)
			}
			guardMissing := hunterV7MissingRequiredConfirmations(guardCoin)
			if len(guardMissing) != 1 || guardMissing[0] != "5m_or_15m_close_below_trigger" {
				t.Fatalf("%s guardCoin missing confirms = %v, want stale [5m_or_15m_close_below_trigger]", tc.name, guardMissing)
			}
			if guardCoin.V7PriceContext == nil || guardCoin.V7PriceContext.Last != tc.pScan {
				t.Fatalf("%s guardCoin.V7PriceContext.Last = %v, want stale P_scan %v", tc.name, guardCoin.V7PriceContext, tc.pScan)
			}
			if containsStringValue(guardCoin.V7ReasonCodes, "live_confirmed_5m_or_15m_close_below_trigger") {
				t.Fatalf("%s guardCoin unexpectedly carries live_confirmed code", tc.name)
			}

			// Guard refresh-recheck call (data=nil, price=0) evaluates stale P_scan > EntryZone.Lower -> false!
			recheckPassed, recheckKnown := HunterV7VerifyConfirmation(guardCoin, nil, 0, "5m_or_15m_close_below_trigger")
			if !recheckKnown || recheckPassed {
				t.Fatalf("%s guard recheck HunterV7VerifyConfirmation(guardCoin, nil, 0, code) = (%v, %v), want (false, true)",
					tc.name, recheckPassed, recheckKnown)
			}

			// Bonus observation for alt_ladder cases (ICNT, PONS): hunterV7ApplyLiveConfirmations
			// only clears failing checks; it NEVER re-evaluates live_price_in_entry_zone when
			// P_live drops below EntryZone.Lower!
			if tc.setupType == local.V7SetupAltLadderShort {
				inZonePassed, inZoneKnown := hunterV7VerifyLiveConfirmation(promptItem.Coin, ctx.MarketDataMap[tc.symbol], "live_price_in_entry_zone")
				if !inZoneKnown || inZonePassed {
					t.Fatalf("%s: expected live_price_in_entry_zone to be false at P_live=%v < zone.Lower=%v", tc.name, tc.pLive, tc.zone.Lower)
				}
				if len(promptItem.Coin.V7ConfirmSummary.MissingReview) != 0 {
					t.Fatalf("%s: expected MissingReview empty (live_price_in_entry_zone never re-checked on live drift), got %+v", tc.name, promptItem.Coin.V7ConfirmSummary.MissingReview)
				}
			}
		})
	}
}
