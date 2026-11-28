package kernel

import (
	"encoding/json"
	"math"
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
	mkClosedBars := func(iv time.Duration) []local.V7Bar {
		formingOpen := tc.promptTimeUTC.Truncate(iv)
		bars := make([]local.V7Bar, 25)
		for i := range bars {
			openAt := formingOpen.Add(-time.Duration(25-i) * iv)
			bars[i] = local.V7Bar{
				OpenTime:  openAt.UnixMilli(),
				CloseTime: openAt.Add(iv).UnixMilli() - 1,
				High:      tc.pScan * 1.001,
				Low:       tc.pScan * 0.999,
				Close:     tc.pScan,
				Volume:    1000,
			}
		}
		return bars
	}
	tf5, ok5 := local.V7TimeframeFactsFromBars("5m", mkClosedBars(5*time.Minute), tc.pScan)
	tf15, ok15 := local.V7TimeframeFactsFromBars("15m", mkClosedBars(15*time.Minute), tc.pScan)
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
		ClosedConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{"5m": tf5, "15m": tf15},
	}
	summary := local.EvaluateV7Confirmations(sig, symCtx, v7Cfg)

	coin := CandidateCoin{
		Symbol:              tc.symbol,
		Sources:             []string{"hunter_v7"},
		Direction:           "SHORT",
		V7SignalID:          sig.SignalID,
		V7SetupType:         string(tc.setupType),
		V7Status:            string(local.V7StatusCandidate),
		V7EntryMode:         string(tc.entryMode),
		V7ExecutionQuality:  string(local.V7ExecReady),
		V7Confidence:        "B",
		V7RiskLevel:         "LOW",
		V7AIPriority:        tc.aiPriority,
		V7SetupScore:        90,
		V7TimingScore:       tc.timingScore,
		V7RegimeFitScore:    80,
		V7LiquidityScore:    tc.liquidityScore,
		V7RiskScore:         tc.riskScore,
		V7EntryZone:         tc.zone,
		V7Invalidation:      local.V7InvalidationRule{Price: tc.invalidation},
		V7Targets:           []local.V7Target{{Price: tc.tp1}, {Price: tc.tp2}},
		V7TP0Price:          tc.tp0,
		V7TP1Price:          tc.tp1,
		V7TP2Price:          tc.tp2,
		V7RequiredConfirms:  append([]string{}, tc.required...),
		V7ReasonCodes:       append([]string{}, tc.reasonCodes...),
		V7ConfirmSummary:    &summary,
		V7PriceContext:      &local.V7PriceContext{Last: tc.pScan, Change1h: -3.5, Change4h: -6.0},
		V7DerivativesCtx:    &local.V7DerivativesContext{TakerBuy15m: tc.taker15m, OIChange1h: tc.oi1h, OIChange4h: tc.oi4h},
		V7ExecutionContext:  symCtx.ExecutionContext,
		V7ConfirmTimeframes: cloneV7TimeframeMap(symCtx.ClosedConfirmTimeframes),
	}
	coin.V7ExecutionTier, coin.V7TierReason = classifyHunterV7CandidateTierWithGeometry(coin, geom)
	return coin
}

// TestU4_TriggerPredicateIgnoresClosedBarsAndUsesLivePrice (updated for U5/M2b):
// Verifies the unified closed-bar trigger semantics on the 3 recorded live cases:
//   - Case A (forming-only dip): ALL closed 5m/15m bars sit strictly above
//     entry_zone_lower while live/forming price dips below -> live verifier
//     returns (passed=false, known=true), never fooled by intrabar dips.
//   - Case B (positive control, mirror): closed 5m/15m bars closed strictly
//     below entry_zone_lower while live price bounces above -> live verifier
//     returns (passed=true, known=true), proving spot bounce never fails a
//     genuine closed-bar trigger.
//   - Single-TF closed pass (5m-only and 15m-only) + zero closed bars (known, failed).
func TestU4_TriggerPredicateIgnoresClosedBarsAndUsesLivePrice(t *testing.T) {
	cfg := u4LiveStrategyConfig()
	eng := NewStrategyEngine(cfg)
	geom := eng.hunterV7ExecutionGeometry()

	for _, tc := range u4RecordedCases() {
		t.Run(tc.name, func(t *testing.T) {
			coin := u4AssembleCandidate(t, tc, geom)

			// Case A (U5/M2b): closed 5m/15m bars all sit at P_scan (> EntryZone.Lower),
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
			passed, known := hunterV7VerifyLiveConfirmationAt(coin, formingOnly, tc.promptTimeUTC, "5m_or_15m_close_below_trigger")
			if !known || passed {
				t.Fatalf("%s forming-only dip (U5/M2b): got (passed=%v, known=%v), want (false, true) — spot/forming dip must never satisfy trigger", tc.name, passed, known)
			}
			// Verify bar evidence on the failed check.
			factsA := HunterV7ConfirmFactsAt(coin, formingOnly, tc.pLive, tc.promptTimeUTC)
			chkA, knownA, okA := local.V7EvaluateSharedConfirmation("5m_or_15m_close_below_trigger", factsA)
			if !okA || !knownA || chkA.Passed || chkA.Evidence == nil {
				t.Fatalf("%s forming-only evidence check: ok=%v known=%v passed=%v ev=%+v", tc.name, okA, knownA, chkA.Passed, chkA.Evidence)
			}
			want5mCloseUnix := tc.promptTimeUTC.Truncate(5 * time.Minute).Unix()
			if !chkA.Evidence.ClosedOnly || chkA.Evidence.Passed || chkA.Evidence.DecidingTF != "5m" ||
				chkA.Evidence.BarClose != tc.pScan || chkA.Evidence.ReferenceEdge != tc.zone.Lower ||
				chkA.Evidence.BarCloseTimeUnix != want5mCloseUnix {
				t.Fatalf("%s forming-only evidence mismatch: %+v (want deciding=5m close=%v edge=%v closeUnix=%d)",
					tc.name, chkA.Evidence, tc.pScan, tc.zone.Lower, want5mCloseUnix)
			}

			// Case B (positive control, U5/M2b): every closed 5m/15m bar closed BELOW
			// EntryZone.Lower (at P_live), while CurrentPrice + forming bar bounced to P_scan.
			closedBelowLiveAbove := u4MarketData(tc.symbol, tc.promptTimeUTC, tc.pLive, tc.pScan)
			for _, tf := range []string{"5m", "15m"} {
				closed := hunterV7ClosedKlines(closedBelowLiveAbove.TimeframeData[tf], tf, tc.promptTimeUTC.UnixMilli())
				if closed[len(closed)-1].Close > tc.zone.Lower {
					t.Fatalf("%s %s: mirror fixture bug: last closed bar must be <= zone.Lower", tc.name, tf)
				}
			}
			mirrorPassed, mirrorKnown := hunterV7VerifyLiveConfirmationAt(coin, closedBelowLiveAbove, tc.promptTimeUTC, "5m_or_15m_close_below_trigger")
			if !mirrorKnown || !mirrorPassed {
				t.Fatalf("%s positive control (closed below, live bounced above, U5/M2b): got (passed=%v, known=%v), want (true, true)", tc.name, mirrorPassed, mirrorKnown)
			}

			// Case C (15m-only closed pass): 5m closed above edge, 15m closed below edge -> passes via 15m.
			only15m := &market.Data{
				Symbol:       tc.symbol,
				CurrentPrice: tc.pScan,
				TimeframeData: map[string]*market.TimeframeSeriesData{
					"5m":  u4SyntheticSeries("5m", 5*time.Minute, tc.promptTimeUTC, 25, tc.pScan, tc.pScan),
					"15m": u4SyntheticSeries("15m", 15*time.Minute, tc.promptTimeUTC, 25, tc.pLive, tc.pScan),
				},
			}
			facts15 := HunterV7ConfirmFactsAt(coin, only15m, tc.pScan, tc.promptTimeUTC)
			chk15, known15, _ := local.V7EvaluateSharedConfirmation("5m_or_15m_close_below_trigger", facts15)
			if !known15 || !chk15.Passed || chk15.Evidence == nil || chk15.Evidence.DecidingTF != "15m" {
				t.Fatalf("%s 15m-only pass: known=%v passed=%v ev=%+v", tc.name, known15, chk15.Passed, chk15.Evidence)
			}

			// Case D (zero closed bars): only forming bars exist -> known=true, passed=false, DecidingTF="none".
			noClosed := &market.Data{
				Symbol:       tc.symbol,
				CurrentPrice: tc.pLive,
				TimeframeData: map[string]*market.TimeframeSeriesData{
					"5m":  u4SyntheticSeries("5m", 5*time.Minute, tc.promptTimeUTC, 0, tc.pLive, tc.pLive),
					"15m": u4SyntheticSeries("15m", 15*time.Minute, tc.promptTimeUTC, 0, tc.pLive, tc.pLive),
				},
			}
			factsNone := HunterV7ConfirmFactsAt(coin, noClosed, tc.pLive, tc.promptTimeUTC)
			chkNone, knownNone, _ := local.V7EvaluateSharedConfirmation("5m_or_15m_close_below_trigger", factsNone)
			if !knownNone || chkNone.Passed || chkNone.Evidence == nil || chkNone.Evidence.DecidingTF != "none" {
				t.Fatalf("%s zero closed bars: want (known=true, passed=false, deciding=none), got known=%v passed=%v ev=%+v",
					tc.name, knownNone, chkNone.Passed, chkNone.Evidence)
			}
		})
	}
}

// TestU4_FormingBarLeaksIntoTimeframeFactsForBarPredicates (updated for U5/M2b):
// Verifies that HunterV7ConfirmFactsAt / HunterV7VerifyConfirmationAt exclude
// the trailing forming bar everywhere confirmations are evaluated, so bar-reading
// predicates (directional_15m_close_short, 5m_price_holds_ema20_or_trailing_support)
// agree strictly with closed bars.
func TestU4_FormingBarLeaksIntoTimeframeFactsForBarPredicates(t *testing.T) {
	cfg := u4LiveStrategyConfig()
	eng := NewStrategyEngine(cfg)
	geom := eng.hunterV7ExecutionGeometry()

	for _, tc := range u4RecordedCases() {
		t.Run(tc.name, func(t *testing.T) {
			coin := u4AssembleCandidate(t, tc, geom)
			pBase := tc.zone.Upper * 1.005
			pClosedHigh := tc.zone.Upper * 1.015 // strictly > EMA20, > VWAP20, > zoneMid
			pFormingLow := tc.pLive              // strictly < EMA20, < VWAP20, < zoneMid

			mkSeries := func(tf string, iv time.Duration, ref time.Time) *market.TimeframeSeriesData {
				s := u4SyntheticSeries(tf, iv, ref, 25, pBase, pFormingLow)
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
					"5m":  mkSeries("5m", 5*time.Minute, tc.promptTimeUTC),
					"15m": mkSeries("15m", 15*time.Minute, tc.promptTimeUTC),
				},
			}

			// Verify closed-bar slice excludes the forming bar and ends at pClosedHigh.
			closed15 := hunterV7ClosedKlines(data.TimeframeData["15m"], "15m", tc.promptTimeUTC.UnixMilli())
			if len(closed15) != 25 || closed15[len(closed15)-1].Close != pClosedHigh {
				t.Fatalf("closed 15m bars mismatch: len=%d last=%v", len(closed15), closed15[len(closed15)-1].Close)
			}

			// U5/M2b: HunterV7ConfirmFactsAt excludes the trailing forming bar!
			liveFacts := HunterV7ConfirmFactsAt(coin, data, pFormingLow, tc.promptTimeUTC)
			if got := liveFacts.Timeframes["15m"].LastClose; got != pClosedHigh {
				t.Fatalf("HunterV7ConfirmFactsAt 15m LastClose = %v, want last closed bar %v (forming bar %v must be excluded)", got, pClosedHigh, pFormingLow)
			}
			if got := liveFacts.Timeframes["5m"].LastClose; got != pClosedHigh {
				t.Fatalf("HunterV7ConfirmFactsAt 5m LastClose = %v, want last closed bar %v (forming bar %v must be excluded)", got, pClosedHigh, pFormingLow)
			}
			livePassed15, liveKnown15 := HunterV7VerifyConfirmationAt(coin, data, pFormingLow, tc.promptTimeUTC, "directional_15m_close_short")
			if !liveKnown15 || livePassed15 {
				t.Fatalf("live directional_15m_close_short (U5/M2b) = (%v, %v), want (false, true) on closed bars", livePassed15, liveKnown15)
			}
			livePassed5, liveKnown5 := HunterV7VerifyConfirmationAt(coin, data, pFormingLow, tc.promptTimeUTC, "5m_price_holds_ema20_or_trailing_support")
			if !liveKnown5 || livePassed5 {
				t.Fatalf("live 5m_price_holds_ema20_or_trailing_support (SHORT, U5/M2b) = (%v, %v), want (false, true) on closed bars", livePassed5, liveKnown5)
			}

			// Also verify default time.Now() entrypoints exclude a live forming bar anchored to now.
			now := time.Now().UTC()
			nowData := &market.Data{
				Symbol:       tc.symbol,
				CurrentPrice: pFormingLow,
				TimeframeData: map[string]*market.TimeframeSeriesData{
					"5m":  mkSeries("5m", 5*time.Minute, now),
					"15m": mkSeries("15m", 15*time.Minute, now),
				},
			}
			nowFacts := HunterV7ConfirmFacts(coin, nowData, pFormingLow)
			if got := nowFacts.Timeframes["15m"].LastClose; got != pClosedHigh {
				t.Fatalf("HunterV7ConfirmFacts(now) 15m LastClose = %v, want last closed bar %v", got, pClosedHigh)
			}
		})
	}
}

// TestU4_FinalizeTiersWriteBackSplitBrainOnThreeCases (updated for U5/M2b):
// Verifies zero divergence across assembly, live re-tier, payload, and guard
// write-back on all 3 live cases:
//  1. Forming-only dip (no closed 5m/15m bar below trigger): EVERY stage agrees
//     5m_or_15m_close_below_trigger is NOT passed, tier stays REVIEWABLE/WATCH,
//     prompt payload and ctx.CandidateCoins carry identical summaries and bar
//     evidence, and guard recheck returns (false, true).
//  2. Positive control (closed 5m bar below trigger): live re-tier clears the
//     confirmation, promotes to EXECUTABLE, writes FULL updated state back to
//     ctx.CandidateCoins defensively (no slice/pointer aliasing), payload and
//     guard agree PassedReview=true with bar evidence, and guard recheck
//     returns (true, true).
func TestU4_FinalizeTiersWriteBackSplitBrainOnThreeCases(t *testing.T) {
	cfg := u4LiveStrategyConfig()
	eng := NewStrategyEngine(cfg)
	geom := eng.hunterV7ExecutionGeometry()

	for _, tc := range u4RecordedCases() {
		t.Run(tc.name+"/forming_only_dip_agrees_not_passed", func(t *testing.T) {
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

			// U5/M2b: prompt stays at assembly tier/reason (no false promotion!).
			if promptItem.Tier != tc.wantAssemTier || promptItem.Reason != tc.wantAssemReason {
				t.Fatalf("%s prompt-final tier = (%q, %q), want (%q, %q)",
					tc.name, promptItem.Tier, promptItem.Reason, tc.wantAssemTier, tc.wantAssemReason)
			}
			if hunterV7ConfirmationSummaryReviewPassed(promptItem.Coin) {
				t.Fatalf("%s prompt item PassedReview = true, want false on forming-only dip", tc.name)
			}
			if containsStringValue(promptItem.Coin.V7ReasonCodes, "live_confirmed_5m_or_15m_close_below_trigger") {
				t.Fatalf("%s prompt item must NOT stamp live_confirmed_5m_or_15m_close_below_trigger on forming-only dip", tc.name)
			}
			payload := buildHunterV7PromptPayload(promptItem.Coin)
			rawJSON, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			sJSON := string(rawJSON)
			if !strings.Contains(sJSON, `"passed_review":false`) ||
				!strings.Contains(sJSON, `"missing_review"`) ||
				!strings.Contains(sJSON, `"bar_evidence"`) ||
				!strings.Contains(sJSON, `"closed_only":true`) {
				t.Fatalf("%s payload JSON did not carry failed verdict + bar_evidence: %s", tc.name, sJSON)
			}

			// Full write-back: guardCoin matches promptItem.Coin (zero divergence!).
			if guardCoin.V7ExecutionTier != promptItem.Tier || guardCoin.V7TierReason != promptItem.Reason {
				t.Fatalf("%s guardCoin tier=(%q,%q) != prompt=(%q,%q)", tc.name, guardCoin.V7ExecutionTier, guardCoin.V7TierReason, promptItem.Tier, promptItem.Reason)
			}
			if hunterV7ConfirmationSummaryReviewPassed(guardCoin) {
				t.Fatalf("%s guardCoin.V7ConfirmSummary PassedReview = true, want false", tc.name)
			}
			guardMissing := hunterV7MissingRequiredConfirmations(guardCoin)
			if len(guardMissing) != 1 || guardMissing[0] != "5m_or_15m_close_below_trigger" {
				t.Fatalf("%s guardCoin missing confirms = %v, want [5m_or_15m_close_below_trigger]", tc.name, guardMissing)
			}
			if guardCoin.V7PriceContext == nil || guardCoin.V7PriceContext.Last != tc.pLive {
				t.Fatalf("%s guardCoin.V7PriceContext.Last = %v, want written-back P_live %v", tc.name, guardCoin.V7PriceContext, tc.pLive)
			}
			if guardCoin.V7ConfirmSummary == nil || len(guardCoin.V7ConfirmSummary.BarEvidence) == 0 {
				t.Fatalf("%s guardCoin missing written-back BarEvidence", tc.name)
			}
			recheckPassed, recheckKnown := HunterV7VerifyConfirmation(guardCoin, nil, 0, "5m_or_15m_close_below_trigger")
			if !recheckKnown || recheckPassed {
				t.Fatalf("%s guard recheck HunterV7VerifyConfirmation(guardCoin, nil, 0, code) = (%v, %v), want (false, true)",
					tc.name, recheckPassed, recheckKnown)
			}
		})

		t.Run(tc.name+"/positive_control_closed_bar_promotes_and_writes_back", func(t *testing.T) {
			assemCoin := u4AssembleCandidate(t, tc, geom)

			// Latest CLOSED 5m bar genuinely closed below EntryZone.Lower at tc.pLive.
			confirmedData := &market.Data{
				Symbol:       tc.symbol,
				CurrentPrice: tc.pLive,
				TimeframeData: map[string]*market.TimeframeSeriesData{
					"5m":  u4SyntheticSeries("5m", 5*time.Minute, tc.promptTimeUTC, 25, tc.pLive, tc.pLive),
					"15m": u4SyntheticSeries("15m", 15*time.Minute, tc.promptTimeUTC, 25, tc.pScan, tc.pLive),
				},
			}
			ctx := &Context{
				PromptTime:     tc.promptTimeUTC,
				CandidateCoins: []CandidateCoin{assemCoin},
				MarketDataMap:  map[string]*market.Data{tc.symbol: confirmedData},
			}
			eng.FinalizeHunterV7Tiers(ctx)

			promptItem := ctx.hunterV7Final.items[0]
			guardCoin := ctx.CandidateCoins[0]

			if promptItem.Tier != tc.wantLiveTier || promptItem.Reason != tc.wantLiveReason {
				t.Fatalf("%s positive control prompt-final tier = (%q, %q), want (%q, %q)",
					tc.name, promptItem.Tier, promptItem.Reason, tc.wantLiveTier, tc.wantLiveReason)
			}
			if !hunterV7ConfirmationSummaryReviewPassed(promptItem.Coin) {
				t.Fatalf("%s positive control prompt PassedReview = false, want true", tc.name)
			}
			// Full write-back to ctx.CandidateCoins[0]: identical tier, summary, price, reason codes, closed timeframes.
			if guardCoin.V7ExecutionTier != tc.wantLiveTier || guardCoin.V7TierReason != tc.wantLiveReason {
				t.Fatalf("%s positive control guardCoin tier = (%q, %q), want (%q, %q)",
					tc.name, guardCoin.V7ExecutionTier, guardCoin.V7TierReason, tc.wantLiveTier, tc.wantLiveReason)
			}
			if !hunterV7ConfirmationSummaryReviewPassed(guardCoin) {
				t.Fatalf("%s positive control guardCoin PassedReview = false, want true", tc.name)
			}
			if missing := hunterV7MissingRequiredConfirmations(guardCoin); len(missing) != 0 {
				t.Fatalf("%s positive control guardCoin missing confirms = %v, want empty", tc.name, missing)
			}
			if guardCoin.V7PriceContext == nil || guardCoin.V7PriceContext.Last != tc.pLive {
				t.Fatalf("%s positive control guardCoin.V7PriceContext.Last = %v, want %v", tc.name, guardCoin.V7PriceContext, tc.pLive)
			}
			if !containsStringValue(guardCoin.V7ReasonCodes, "live_confirmed_5m_or_15m_close_below_trigger") {
				t.Fatalf("%s positive control guardCoin missing live_confirmed_5m_or_15m_close_below_trigger in %v", tc.name, guardCoin.V7ReasonCodes)
			}
			// Check bar evidence is carried on both prompt and guard summaries.
			var foundEv *local.V7ConfirmBarEvidence
			for i := range guardCoin.V7ConfirmSummary.BarEvidence {
				ev := &guardCoin.V7ConfirmSummary.BarEvidence[i]
				if ev.Code == "5m_or_15m_close_below_trigger" {
					foundEv = ev
					break
				}
			}
			if foundEv == nil || !foundEv.Passed || !foundEv.ClosedOnly || foundEv.DecidingTF != "5m" ||
				foundEv.BarClose != tc.pLive || foundEv.ReferenceEdge != tc.zone.Lower || foundEv.BarCloseTimeUnix <= 0 {
				t.Fatalf("%s positive control BarEvidence mismatch: %+v", tc.name, foundEv)
			}
			// Defensive copy check: mutating guardCoin must not alias promptItem.Coin.
			guardCoin.V7ReasonCodes[0] = "mutated_guard_only"
			if promptItem.Coin.V7ReasonCodes[0] == "mutated_guard_only" {
				t.Fatalf("%s write-back aliased V7ReasonCodes slice with prompt item", tc.name)
			}
			// Guard recheck on written-back closed timeframes agrees: (true, true).
			recheckPassed, recheckKnown := HunterV7VerifyConfirmation(guardCoin, nil, 0, "5m_or_15m_close_below_trigger")
			if !recheckKnown || !recheckPassed {
				t.Fatalf("%s positive control guard recheck = (%v, %v), want (true, true)", tc.name, recheckPassed, recheckKnown)
			}
		})
	}
}

// TestU5bLiveApplyTwoWayAndPartialTimeframeInteractions verifies Fix 2 and Fix 4
// edge cases directly in hunterV7ApplyLiveConfirmationsAt:
//  1. Simultaneous clear + demote: a missing code clears while an already-passed
//     trigger code fails on fresh bar N+1 -> demoted into MissingReview and
//     PassedReview stays false (never erroneously flipped true).
//  2. Absent timeframe & non-bar codes: when prompt data carries only 5m, an
//     already-passed directional_15m_close_short keeps its scoring verdict and
//     15m BarEvidence, and a passed non-bar code (live_price_in_entry_zone) is
//     not re-evaluated/demoted.
//  3. Present-but-zero-closed-bars timeframe deletes only that timeframe while
//     preserving the absent timeframe's scoring closed-bar summary.
func TestU5bLiveApplyTwoWayAndPartialTimeframeInteractions(t *testing.T) {
	cfg := u4LiveStrategyConfig()
	eng := NewStrategyEngine(cfg)
	geom := eng.hunterV7ExecutionGeometry()
	bless := u4RecordedCases()[0]

	t.Run("simultaneous_clear_and_demote_keeps_passed_review_false", func(t *testing.T) {
		coin := u4AssembleCandidate(t, bless, geom)
		// Suppose 5m_or_15m_close_below_trigger PASSED at scoring (bar N),
		// while taker_buy_15m_lt_0_48 was in MissingReview; by prompt time
		// taker passes (0.39 <= 0.48) while bar N+1 closed back above Lower.
		coin.V7ConfirmSummary = &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{Code: "taker_buy_15m_lt_0_48", Passed: false, Severity: local.V7ConfirmReviewWait},
			},
			BarEvidence: []local.V7ConfirmBarEvidence{
				{Code: "5m_or_15m_close_below_trigger", Passed: true, Timeframe: "5m|15m", DecidingTF: "5m", BarClose: bless.pLive, ReferenceEdge: bless.zone.Lower, ClosedOnly: true},
			},
		}
		data := u4MarketData(bless.symbol, bless.promptTimeUTC, bless.pScan, bless.pLive)
		updated := hunterV7ApplyLiveConfirmationsAt(coin, data, bless.promptTimeUTC)
		if updated.V7ConfirmSummary.PassedReview {
			t.Fatalf("PassedReview must stay false when one code cleared and another bar code demoted: %+v", updated.V7ConfirmSummary)
		}
		if len(updated.V7ConfirmSummary.MissingReview) != 1 || updated.V7ConfirmSummary.MissingReview[0].Code != "5m_or_15m_close_below_trigger" {
			t.Fatalf("expected only demoted trigger in MissingReview, got %+v", updated.V7ConfirmSummary.MissingReview)
		}
		if len(updated.V7ConfirmSummary.BarEvidence) != 1 || updated.V7ConfirmSummary.BarEvidence[0].Passed {
			t.Fatalf("BarEvidence must reflect demoted failing bar N+1: %+v", updated.V7ConfirmSummary.BarEvidence)
		}
	})

	t.Run("absent_tf_and_non_bar_codes_keep_scoring_verdict", func(t *testing.T) {
		coin := u4AssembleCandidate(t, bless, geom)
		scoring15Ev := local.V7ConfirmBarEvidence{
			Code:             "directional_15m_close_short",
			Passed:           true,
			Timeframe:        "15m",
			DecidingTF:       "15m",
			BarCloseTimeUnix: bless.promptTimeUTC.Truncate(15 * time.Minute).Unix(),
			BarClose:         bless.pLive,
			ReferenceEdge:    (bless.zone.Lower + bless.zone.Upper) / 2,
			ClosedOnly:       true,
		}
		coin.V7RequiredConfirms = []string{"directional_15m_close_short", "live_price_in_entry_zone"}
		coin.V7ConfirmSummary = &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: true,
			BarEvidence:  []local.V7ConfirmBarEvidence{scoring15Ev},
		}
		// Prompt data carries ONLY 5m (no 15m), and spot price moved outside EntryZone.
		data := &market.Data{
			Symbol:       bless.symbol,
			CurrentPrice: bless.zone.Upper * 1.05, // outside zone -> non-bar code must NOT be demoted
			TimeframeData: map[string]*market.TimeframeSeriesData{
				"5m": u4SyntheticSeries("5m", 5*time.Minute, bless.promptTimeUTC, 25, bless.pScan, bless.zone.Upper*1.05),
			},
		}
		updated := hunterV7ApplyLiveConfirmationsAt(coin, data, bless.promptTimeUTC)
		if !updated.V7ConfirmSummary.PassedReview || len(updated.V7ConfirmSummary.MissingReview) != 0 {
			t.Fatalf("codes without fresh TF or non-bar codes must keep scoring verdict: %+v", updated.V7ConfirmSummary)
		}
		if len(updated.V7ConfirmSummary.BarEvidence) != 1 || updated.V7ConfirmSummary.BarEvidence[0] != scoring15Ev {
			t.Fatalf("scoring 15m BarEvidence must remain intact: got %+v, want %+v", updated.V7ConfirmSummary.BarEvidence, scoring15Ev)
		}
	})

	t.Run("zero_closed_bars_on_present_5m_deletes_only_5m_and_keeps_15m", func(t *testing.T) {
		coin := u4AssembleCandidate(t, bless, geom)
		want15Close := coin.V7ConfirmTimeframes["15m"].LastClose
		data := &market.Data{
			Symbol:       bless.symbol,
			CurrentPrice: bless.pLive,
			TimeframeData: map[string]*market.TimeframeSeriesData{
				"5m": u4SyntheticSeries("5m", 5*time.Minute, bless.promptTimeUTC, 0, bless.pLive, bless.pLive),
			},
		}
		facts := HunterV7ConfirmFactsAt(coin, data, bless.pLive, bless.promptTimeUTC)
		if _, has5 := facts.Timeframes["5m"]; has5 {
			t.Fatalf("present 5m with 0 closed bars must be removed from facts.Timeframes, got %+v", facts.Timeframes)
		}
		if got15, has15 := facts.Timeframes["15m"]; !has15 || got15.LastClose != want15Close {
			t.Fatalf("absent 15m must retain scoring summary (want LastClose=%v), got %+v", want15Close, facts.Timeframes)
		}
	})
}

// TestU5cRegressionBSuffixAlignedEMA20AndTruncatedIndicators verifies Fixes A
// and C for 5m EMA20 confirmations:
//   - Regression (b): suffix-aligned EMA20Values (len(Klines)=20,
//     len(EMA20Values)=19, 19 closed + 1 forming 5m bars) recovers the last
//     closed bar's EMA20 (EMA20Values[17], never the forming bar's
//     EMA20Values[18]), does not demote a scoring-passed
//     5m_price_holds_ema20_or_trailing_support, and preserves DecidingTF="5m"
//     BarEvidence.
//   - Fix C same-bar carry-forward: when the fresh closed bar has the same
//     LastCloseTimeUnix as scoring's summary and lacks EMA20/VWAP20,
//     HunterV7ConfirmFactsAt inherits them from the prior summary.
//   - Fix C truncated-window no-fabricated-demotion: when a synthetic caller
//     provides <20 closed 5m bars for a new bar without EMA20Values, neither a
//     scoring-passed code is demoted nor a scoring-failed code's DecidingTF="5m"
//     BarEvidence is clobbered with DecidingTF="none", while 0 closed bars
//     still demote and fail closed.
//   - Regression (c) 5m leg: when a genuinely NEW closed bar with evaluable
//     suffix-aligned EMA20Values closes below EMA20, Fix 4 still demotes it.
func TestU5cRegressionBSuffixAlignedEMA20AndTruncatedIndicators(t *testing.T) {
	promptTime := time.Date(2026, 11, 28, 12, 12, 30, 0, time.UTC)
	formingOpen := promptTime.Truncate(5 * time.Minute)      // 12:10 UTC
	lastClosedCloseUnix := formingOpen.Unix()                 // 12:10 UTC (bar 18: 12:05..12:10)
	priorClosedCloseUnix := formingOpen.Add(-5 * time.Minute).Unix() // 12:05 UTC (bar N)

	build5mSeries := func(lastClosedClose, closedEMA20, formingEMA20 float64, includeEMA bool) *market.TimeframeSeriesData {
		klines := make([]market.KlineBar, 20)
		for i := 0; i < 19; i++ {
			openAt := formingOpen.Add(-time.Duration(19-i) * 5 * time.Minute)
			c := 100.0
			if i == 18 {
				c = lastClosedClose
			}
			klines[i] = market.KlineBar{
				Time:   openAt.UnixMilli(),
				Open:   c,
				High:   c + 0.2,
				Low:    c - 0.2,
				Close:  c,
				Volume: 1000,
			}
		}
		// Bar 19: trailing forming bar (openAt == 12:10, close == 12:15 > 12:12:30).
		klines[19] = market.KlineBar{
			Time:   formingOpen.UnixMilli(),
			Open:   lastClosedClose,
			High:   lastClosedClose + 0.5,
			Low:    lastClosedClose - 0.5,
			Close:  lastClosedClose - 2.0,
			Volume: 500,
		}
		var emaVals []float64
		if includeEMA {
			// Suffix-aligned: len(EMA20Values) = 19 for len(Klines) = 20.
			// EMA20Values[k] aligns to Klines[k+1]; so Klines[18] (last closed)
			// maps to EMA20Values[17] and Klines[19] (forming) to EMA20Values[18].
			emaVals = make([]float64, 19)
			for i := range emaVals {
				emaVals[i] = 100.0
			}
			emaVals[17] = closedEMA20
			emaVals[18] = formingEMA20
		}
		return &market.TimeframeSeriesData{
			Timeframe:   "5m",
			Klines:      klines,
			EMA20Values: emaVals,
		}
	}

	make5mCoin := func(passed bool, priorCloseUnix int64) CandidateCoin {
		scoringEv := local.V7ConfirmBarEvidence{
			Code:             "5m_price_holds_ema20_or_trailing_support",
			Passed:           passed,
			Timeframe:        "5m",
			DecidingTF:       "5m",
			BarCloseTimeUnix: priorCloseUnix,
			BarClose:         101.0,
			ReferenceEdge:    100.0,
			ClosedOnly:       true,
		}
		summary := &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: passed,
			BarEvidence:  []local.V7ConfirmBarEvidence{scoringEv},
		}
		if !passed {
			summary.MissingReview = []local.V7ConfirmationCheck{{
				Code:     "5m_price_holds_ema20_or_trailing_support",
				Passed:   false,
				Severity: local.V7ConfirmReviewWait,
				Reason:   "5m close is below EMA20",
				Evidence: &scoringEv,
			}}
		}
		return CandidateCoin{
			Symbol:             "OPUSDT",
			Direction:          "LONG",
			V7SetupType:        "mms_trend_ride_long",
			V7RequiredConfirms: []string{"5m_price_holds_ema20_or_trailing_support"},
			V7ConfirmSummary:   summary,
			V7ConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{
				"5m": {
					Timeframe:         "5m",
					CandleCount:       200,
					LastClose:         101.0,
					LastCloseTimeUnix: priorCloseUnix,
					HasEMA20:          true,
					EMA20:             100.0,
					CloseVsEMA20Pct:   1.0,
				},
			},
			V7PriceContext: &local.V7PriceContext{Last: 101.0},
		}
	}

	t.Run("regression_b_suffix_aligned_ema20_recovers_last_closed_bar_and_preserves_pass", func(t *testing.T) {
		coin := make5mCoin(true, priorClosedCloseUnix)
		// Last closed bar (101.5) > closedEMA20 (100.5), while formingEMA20 (105.0)
		// is above close so reading the forming slot would wrongly fail.
		series := build5mSeries(101.5, 100.5, 105.0, true)
		closed := hunterV7ClosedKlines(series, "5m", promptTime.UnixMilli())
		if len(closed) != 19 {
			t.Fatalf("len(closed) = %d, want 19", len(closed))
		}
		if gotEMA := hunterV7ClosedEMA20(series, closed); math.Abs(gotEMA-100.5) > 1e-12 {
			t.Fatalf("hunterV7ClosedEMA20 = %v, want 100.5 (EMA20Values[17])", gotEMA)
		}

		data := &market.Data{
			Symbol:        coin.Symbol,
			CurrentPrice:  101.5,
			TimeframeData: map[string]*market.TimeframeSeriesData{"5m": series},
		}
		updated := hunterV7ApplyLiveConfirmationsAt(coin, data, promptTime)
		if !updated.V7ConfirmSummary.PassedReview || len(updated.V7ConfirmSummary.MissingReview) != 0 {
			t.Fatalf("suffix-aligned EMA20 must not demote scoring-passed 5m code: %+v", updated.V7ConfirmSummary)
		}
		if len(updated.V7ConfirmSummary.BarEvidence) != 1 {
			t.Fatalf("expected 1 BarEvidence entry, got %+v", updated.V7ConfirmSummary.BarEvidence)
		}
		ev := updated.V7ConfirmSummary.BarEvidence[0]
		if !ev.Passed || ev.DecidingTF != "5m" || !ev.ClosedOnly ||
			ev.BarClose != 101.5 || math.Abs(ev.ReferenceEdge-100.5) > 1e-9 || ev.BarCloseTimeUnix != lastClosedCloseUnix {
			t.Fatalf("BarEvidence clobbered or wrong: got %+v", ev)
		}
	})

	t.Run("fix_c_same_bar_carry_forward_inherits_ema20_without_series_values", func(t *testing.T) {
		coin := make5mCoin(true, lastClosedCloseUnix)
		// Same closed bar (12:10 UTC), 19 closed + 1 forming, NO EMA20Values.
		series := build5mSeries(101.0, 0, 0, false)
		data := &market.Data{
			Symbol:        coin.Symbol,
			CurrentPrice:  101.0,
			TimeframeData: map[string]*market.TimeframeSeriesData{"5m": series},
		}
		facts := HunterV7ConfirmFactsAt(coin, data, 101.0, promptTime)
		tf5, ok := facts.Timeframes["5m"]
		if !ok || !tf5.HasEMA20 || tf5.EMA20 != 100.0 || tf5.CloseVsEMA20Pct != 1.0 {
			t.Fatalf("same-bar carry-forward failed: %+v", tf5)
		}
		updated := hunterV7ApplyLiveConfirmationsAt(coin, data, promptTime)
		if !updated.V7ConfirmSummary.PassedReview || len(updated.V7ConfirmSummary.MissingReview) != 0 {
			t.Fatalf("same-bar carry-forward must keep passed verdict: %+v", updated.V7ConfirmSummary)
		}
	})

	t.Run("fix_c_truncated_new_bar_without_ema20_never_fabricates_demotion_or_clobbers_evidence", func(t *testing.T) {
		// (i) Previously passed -> must NOT be demoted, must keep DecidingTF="5m".
		passCoin := make5mCoin(true, priorClosedCloseUnix)
		series := build5mSeries(101.5, 0, 0, false)
		data := &market.Data{
			Symbol:        passCoin.Symbol,
			CurrentPrice:  101.5,
			TimeframeData: map[string]*market.TimeframeSeriesData{"5m": series},
		}
		gotPass := hunterV7ApplyLiveConfirmationsAt(passCoin, data, promptTime)
		if !gotPass.V7ConfirmSummary.PassedReview || len(gotPass.V7ConfirmSummary.MissingReview) != 0 {
			t.Fatalf("truncated 5m series without EMA20Values must not demote passed code: %+v", gotPass.V7ConfirmSummary)
		}
		if len(gotPass.V7ConfirmSummary.BarEvidence) != 1 || gotPass.V7ConfirmSummary.BarEvidence[0].DecidingTF != "5m" {
			t.Fatalf("scoring DecidingTF=5m BarEvidence must not be clobbered: %+v", gotPass.V7ConfirmSummary.BarEvidence)
		}

		// (ii) Previously failed on real 5m bar -> must NOT clobber Reason or BarEvidence to DecidingTF="none".
		failCoin := make5mCoin(false, priorClosedCloseUnix)
		gotFail := hunterV7ApplyLiveConfirmationsAt(failCoin, data, promptTime)
		if len(gotFail.V7ConfirmSummary.MissingReview) != 1 ||
			gotFail.V7ConfirmSummary.MissingReview[0].Reason != "5m close is below EMA20" ||
			gotFail.V7ConfirmSummary.MissingReview[0].Evidence == nil ||
			gotFail.V7ConfirmSummary.MissingReview[0].Evidence.DecidingTF != "5m" {
			t.Fatalf("truncated 5m series must not clobber scoring failure evidence: %+v", gotFail.V7ConfirmSummary.MissingReview)
		}

		// (iii) Zero closed bars STILL fails closed and demotes!
		zeroData := &market.Data{
			Symbol:       passCoin.Symbol,
			CurrentPrice: 101.5,
			TimeframeData: map[string]*market.TimeframeSeriesData{
				"5m": u4SyntheticSeries("5m", 5*time.Minute, promptTime, 0, 101.5, 101.5),
			},
		}
		gotZero := hunterV7ApplyLiveConfirmationsAt(passCoin, zeroData, promptTime)
		if gotZero.V7ConfirmSummary.PassedReview || len(gotZero.V7ConfirmSummary.MissingReview) != 1 {
			t.Fatalf("zero closed bars must still demote and fail closed: %+v", gotZero.V7ConfirmSummary)
		}
	})

	t.Run("regression_c_new_closed_5m_bar_below_suffix_ema20_still_demotes", func(t *testing.T) {
		coin := make5mCoin(true, priorClosedCloseUnix)
		// Genuinely new closed bar closes at 99.0 < closedEMA20 (100.5).
		series := build5mSeries(99.0, 100.5, 98.0, true)
		data := &market.Data{
			Symbol:        coin.Symbol,
			CurrentPrice:  99.0,
			TimeframeData: map[string]*market.TimeframeSeriesData{"5m": series},
		}
		updated := hunterV7ApplyLiveConfirmationsAt(coin, data, promptTime)
		if updated.V7ConfirmSummary.PassedReview || len(updated.V7ConfirmSummary.MissingReview) != 1 ||
			updated.V7ConfirmSummary.MissingReview[0].Code != "5m_price_holds_ema20_or_trailing_support" {
			t.Fatalf("genuinely failing new 5m closed bar must still demote (Fix 4): %+v", updated.V7ConfirmSummary)
		}
		ev := updated.V7ConfirmSummary.BarEvidence[0]
		if ev.Passed || ev.DecidingTF != "5m" || ev.BarClose != 99.0 || math.Abs(ev.ReferenceEdge-100.5) > 1e-9 || ev.BarCloseTimeUnix != lastClosedCloseUnix {
			t.Fatalf("demoted BarEvidence must reflect failing new bar: %+v", ev)
		}
	})
}
