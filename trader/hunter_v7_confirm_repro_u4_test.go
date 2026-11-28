package trader

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Unit U4 (M2a): end-to-end trader execution guard reproduction of the 3
// guard-rejected approvals on AIK11 (2026-11-28):
//   - C16 (sig sc14) BLESSUSDT breakdown_momentum_short
//   - C18 (sig sc16) ICNTUSDT  alt_ladder_breakdown_short
//   - C31 (sig sc29) PONSUSDT  alt_ladder_breakdown_short
//
// Historical 5m/15m klines are NOT in the data pack; synthetic bar series are
// constructed from each case's recorded geometry (sig.jsonl + dr.jsonl) and
// driven through the real PrepareHunterV7Tiers -> BuildUserPrompt ->
// validateHunterV7ExecutionGuard / validateHunterV7RequiredConfirmations path.

type u4GuardCase struct {
	name            string
	symbol          string
	setupType       local.V7SetupType
	zone            local.V7PriceZone
	invalidation    float64
	tp0, tp1, tp2   float64
	decisionSL      float64
	decisionTP      float64
	confidence      int
	pScan           float64
	pLive           float64
	taker15m        float64
	oi1h, oi4h      float64
	aiPriority      float64
	timingScore     float64
	liquidityScore  float64
	promptTimeUTC   time.Time
	required        []string
	reasonCodes     []string
	wantAssemTier   string
	wantAssemReason string
	wantGuardErr    string
}

func u4GuardTrader() *AutoTrader {
	at := testRiskAutoTrader()
	at.config.StrategyConfig.Language = "zh"
	at.config.StrategyConfig.PromptCompactMode = "hunter_v7_only"
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.CoinSource.Hunter = &store.HunterConfig{
		V7MaxOutput:     18,
		V7MinAIPriority: 50,
		V7Aggressive:    true,
		V7T2Enabled:     true,
	}
	at.config.StrategyConfig.RiskControl.MaxPositions = 2
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.2
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3.5
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5
	at.strategyEngine = kernel.NewStrategyEngine(at.config.StrategyConfig)
	return at
}

func u4GuardSyntheticSeries(tf string, iv time.Duration, promptTime time.Time, closedPrice, formingClose float64) *market.TimeframeSeriesData {
	formingOpen := promptTime.Truncate(iv)
	const nClosed = 25
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

func u4BuildAssembledCandidate(t *testing.T, tc u4GuardCase, geom kernel.HunterV7ExecutionGeometry) kernel.CandidateCoin {
	t.Helper()
	closedBars := make([]local.V7Bar, 25)
	for i := range closedBars {
		closedBars[i] = local.V7Bar{High: tc.pScan * 1.001, Low: tc.pScan * 0.999, Close: tc.pScan, Volume: 1000}
	}
	tf5, _ := local.V7TimeframeFactsFromBars("5m", closedBars, tc.pScan)
	tf15, _ := local.V7TimeframeFactsFromBars("15m", closedBars, tc.pScan)

	sig := &local.V7SignalOutput{
		SignalID:         "sig_" + tc.symbol,
		Symbol:           tc.symbol,
		Direction:        local.V7DirShort,
		SetupType:        tc.setupType,
		Status:           local.V7StatusCandidate,
		EntryMode:        local.V7EntryFastConfirm,
		ExecutionQuality: local.V7ExecReady,
		Confidence:       "B",
		RiskLevel:        local.V7RiskLow,
		AIPriority:       tc.aiPriority,
		SetupScore:       90,
		TimingScore:      tc.timingScore,
		RegimeFitScore:   80,
		LiquidityScore:   tc.liquidityScore,
		RiskScore:        0,
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

	coin := kernel.CandidateCoin{
		Symbol:             tc.symbol,
		Sources:            []string{"hunter_v7"},
		Direction:          "SHORT",
		V7SignalID:         sig.SignalID,
		V7SetupType:        string(tc.setupType),
		V7Status:           string(local.V7StatusCandidate),
		V7EntryMode:        string(local.V7EntryFastConfirm),
		V7ExecutionQuality: string(local.V7ExecReady),
		V7Confidence:       "B",
		V7RiskLevel:        "LOW",
		V7AIPriority:       tc.aiPriority,
		V7SetupScore:       90,
		V7TimingScore:      tc.timingScore,
		V7RegimeFitScore:   80,
		V7LiquidityScore:   tc.liquidityScore,
		V7RiskScore:        0,
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
	coin.V7ExecutionTier, coin.V7TierReason = kernel.ClassifyHunterV7CandidateTierForRuntime(coin, geom)
	return coin
}

func u4GuardCases() []u4GuardCase {
	return []u4GuardCase{
		{
			name:            "C16_BLESSUSDT",
			symbol:          "BLESSUSDT",
			setupType:       local.V7SetupBreakdownShort,
			zone:            local.V7PriceZone{Lower: 0.013816808611979915, Upper: 0.013949536417822414},
			invalidation:    0.014162876000000001,
			tp0:             0.013806906726227251,
			tp1:             0.01290182148192,
			tp2:             0.012116740655535068,
			decisionSL:      0.014162876,
			decisionTP:      0.013105630874714442,
			confidence:      82,
			pScan:           0.013858,
			pLive:           0.013802,
			taker15m:        0.390,
			oi1h:            -0.44,
			oi4h:            -1.0,
			aiPriority:      93.085,
			timingScore:     70,
			liquidityScore:  100,
			promptTimeUTC:   time.Date(2026, 11, 28, 0, 22, 14, 0, time.UTC),
			required:        []string{"5m_or_15m_close_below_trigger", "taker_buy_15m_lt_0_48", "no_immediate_reclaim_of_trigger_level"},
			reasonCodes:     []string{"solid_1h_downside_momentum", "below_vwap_breakdown", "heavy_taker_selling"},
			wantAssemTier:   "REVIEWABLE",
			wantAssemReason: "live_reviewable_5m_or_15m_close_below_trigger",
			wantGuardErr:    "❌ [HUNTER V7 GUARD] breakdown_momentum_short BLESSUSDT blocked: required confirmation 5m_or_15m_close_below_trigger failed (confirmation_missing)",
		},
		{
			name:            "C18_ICNTUSDT",
			symbol:          "ICNTUSDT",
			setupType:       local.V7SetupAltLadderShort,
			zone:            local.V7PriceZone{Lower: 0.29349855988945545, Upper: 0.2966459430940239},
			invalidation:    0.300979,
			tp0:             0.2903468526851314,
			tp1:             0.2757250939416751,
			tp2:             0.26024736790162345,
			decisionSL:      0.30098,
			decisionTP:      0.27902,
			confidence:      76,
			pScan:           0.294500,
			pLive:           0.293300,
			taker15m:        0.343,
			oi1h:            -2.65,
			oi4h:            -11.0,
			aiPriority:      88.085,
			timingScore:     66,
			liquidityScore:  90,
			promptTimeUTC:   time.Date(2026, 11, 28, 0, 42, 14, 0, time.UTC),
			required:        []string{"live_price_in_entry_zone", "taker_buy_15m_lt_0_48", "5m_or_15m_close_below_trigger", "taker_flow_not_flipping_against_direction"},
			reasonCodes:     []string{"alt_ladder_breakdown_short", "alt_ladder_downshift_mid", "alt_ladder_taker_sell", "alt_ladder_long_flush"},
			wantAssemTier:   "WATCH",
			wantAssemReason: "alt_ladder_short_rebound_pending",
			wantGuardErr:    "❌ [HUNTER V7 GUARD] alt_ladder_breakdown_short ICNTUSDT blocked: required confirmation 5m_or_15m_close_below_trigger failed (confirmation_missing)",
		},
		{
			name:            "C31_PONSUSDT",
			symbol:          "PONSUSDT",
			setupType:       local.V7SetupAltLadderShort,
			zone:            local.V7PriceZone{Lower: 0.1006455034011606, Upper: 0.10144534985465585},
			invalidation:    0.10291800000000001,
			tp0:             0.10011029745610295,
			tp1:             0.0952076579068888,
			tp2:             0.09053744293007326,
			decisionSL:      0.10292,
			decisionTP:      0.09623,
			confidence:      74,
			pScan:           0.100900,
			pLive:           0.100500,
			taker15m:        0.334,
			oi1h:            2.04,
			oi4h:            1.5,
			aiPriority:      92.085,
			timingScore:     70,
			liquidityScore:  100,
			promptTimeUTC:   time.Date(2026, 11, 28, 2, 52, 14, 0, time.UTC),
			required:        []string{"live_price_in_entry_zone", "taker_buy_15m_lt_0_48", "5m_or_15m_close_below_trigger", "taker_flow_not_flipping_against_direction"},
			reasonCodes:     []string{"alt_ladder_breakdown_short", "alt_ladder_downshift_mid", "alt_ladder_taker_sell", "alt_ladder_new_shorts"},
			wantAssemTier:   "REVIEWABLE",
			wantAssemReason: "live_reviewable_5m_or_15m_close_below_trigger",
			wantGuardErr:    "❌ [HUNTER V7 GUARD] alt_ladder_breakdown_short PONSUSDT blocked: required confirmation 5m_or_15m_close_below_trigger failed (confirmation_missing)",
		},
	}
}

// TestU4ConfirmRequiredGuardRejectsThreePromotedOpens drives the full live
// cycle path — scoring -> PrepareHunterV7Tiers -> BuildUserPrompt ->
// validateHunterV7ExecutionGuard — on synthetic bar series for C16 (BLESS),
// C18 (ICNT), and C31 (PONS) where closed 5m/15m bars stay above
// entry_zone_lower while live/forming price dips below. It reproduces the
// exact three-way divergence observed in AIK11's live run:
//   - Assembly: confirmation_missing (REVIEWABLE/WATCH)
//   - Prompt payload: EXECUTABLE, passed_review=true (no missing_review)
//   - Guard: exact dr.jsonl rejection text on 5m_or_15m_close_below_trigger
func TestU4ConfirmRequiredGuardRejectsThreePromotedOpens(t *testing.T) {
	at := u4GuardTrader()
	geom := kernel.HunterV7ExecutionGeometryForConfig(at.config.StrategyConfig)

	for _, tc := range u4GuardCases() {
		t.Run(tc.name, func(t *testing.T) {
			assemCoin := u4BuildAssembledCandidate(t, tc, geom)
			if assemCoin.V7ExecutionTier != tc.wantAssemTier || assemCoin.V7TierReason != tc.wantAssemReason {
				t.Fatalf("assembly = (%q, %q), want (%q, %q)",
					assemCoin.V7ExecutionTier, assemCoin.V7TierReason, tc.wantAssemTier, tc.wantAssemReason)
			}

			ctx := &kernel.Context{
				PromptTime:     tc.promptTimeUTC,
				CandidateCoins: []kernel.CandidateCoin{assemCoin},
				MarketDataMap: map[string]*market.Data{
					tc.symbol: {
						Symbol:       tc.symbol,
						CurrentPrice: tc.pLive,
						TimeframeData: map[string]*market.TimeframeSeriesData{
							"5m":  u4GuardSyntheticSeries("5m", 5*time.Minute, tc.promptTimeUTC, tc.pScan, tc.pLive),
							"15m": u4GuardSyntheticSeries("15m", 15*time.Minute, tc.promptTimeUTC, tc.pScan, tc.pLive),
						},
					},
				},
			}

			// Step 1: PrepareHunterV7Tiers (same call auto_trader_loop.go:135 makes).
			if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
				t.Fatalf("PrepareHunterV7Tiers: %v", err)
			}

			// Step 2: BuildUserPrompt (same payload shown to the LLM).
			prompt := at.strategyEngine.BuildUserPrompt(ctx)
			if !strings.Contains(prompt, "execution_tier=EXECUTABLE") {
				t.Fatalf("%s prompt did not promote candidate to EXECUTABLE:\n%s", tc.name, prompt)
			}
			if !strings.Contains(prompt, `"passed_review":true`) {
				t.Fatalf("%s prompt payload did not claim passed_review=true", tc.name)
			}
			if strings.Contains(prompt, `"missing_review"`) {
				t.Fatalf("%s prompt payload unexpectedly contains missing_review", tc.name)
			}

			// Step 3: Guard check on ctx.CandidateCoins[0].
			// Because V7ExecutionTier was promoted to EXECUTABLE, the refresh
			// hook is skipped altogether (auto_trader_risk.go:1902).
			if hunterV7OpenNeedsGuardRefresh(&ctx.CandidateCoins[0]) {
				t.Fatalf("%s: expected hunterV7OpenNeedsGuardRefresh=false for EXECUTABLE candidate", tc.name)
			}

			decision := &kernel.Decision{
				Symbol:                   tc.symbol,
				Action:                   "open_short",
				Leverage:                 10,
				StopLoss:                 tc.decisionSL,
				TakeProfit:               tc.decisionTP,
				Confidence:               tc.confidence,
				SelectedHunterV7SignalID: assemCoin.V7SignalID,
				SelectedHunterV7Setup:    string(tc.setupType),
				SelectedHunterV7Tier:     "EXECUTABLE",
			}
			err := at.validateHunterV7ExecutionGuard(ctx, decision)
			if err == nil {
				t.Fatalf("%s: expected guard rejection, got nil", tc.name)
			}
			if err.Error() != tc.wantGuardErr {
				t.Fatalf("%s guard error mismatch:\n got: %s\nwant: %s", tc.name, err.Error(), tc.wantGuardErr)
			}
		})
	}
}

// TestU4ConfirmRequiredReviewableRefreshWaiverFailsOnStaleFacts pins the second
// guard trap (auto_trader_risk.go:1394-1405, 2011-2031): even if a candidate
// stays REVIEWABLE (so guard refresh runs and mints fresh_rest_confirmed +
// fresh_micro_confirmed), hunterV7RefreshEvidence.applyTo NEVER updates
// V7PriceContext.Last or V7ExecutionContext.Timeframes. Consequently,
// hunterV7ReviewableConfirmationSatisfiedByRefresh calls
// kernel.HunterV7VerifyConfirmation(*candidate, nil, 0, code), which re-evaluates
// on stale P_scan / stale scoring-time Timeframes and rejects the waiver.
func TestU4ConfirmRequiredReviewableRefreshWaiverFailsOnStaleFacts(t *testing.T) {
	at := u4GuardTrader()
	geom := kernel.HunterV7ExecutionGeometryForConfig(at.config.StrategyConfig)
	bless := u4GuardCases()[0]

	cand := u4BuildAssembledCandidate(t, bless, geom)
	if cand.V7ExecutionTier != "REVIEWABLE" {
		t.Fatalf("fixture: want REVIEWABLE, got %q", cand.V7ExecutionTier)
	}
	if !hunterV7OpenNeedsGuardRefresh(&cand) {
		t.Fatalf("fixture: expected REVIEWABLE candidate with live-reviewable gap to request guard refresh")
	}

	// Apply successful REST + orderbook refresh evidence.
	ev := hunterV7RefreshEvidence{RestConfirmed: true, MicroConfirmed: true}
	ev.applyTo(&cand)

	// The refresh minted both umbrella codes, yet did NOT update V7PriceContext.Last.
	if cand.V7PriceContext.Last != bless.pScan {
		t.Fatalf("applyTo unexpectedly changed V7PriceContext.Last: got %v, want stale %v", cand.V7PriceContext.Last, bless.pScan)
	}

	// Waiver STILL returns false because HunterV7VerifyConfirmation(*cand, nil, 0, code)
	// evaluates P_scan (0.013858) <= EntryZone.Lower (0.0138168) -> false.
	if hunterV7ReviewableConfirmationSatisfiedByRefresh(&cand, "5m_or_15m_close_below_trigger") {
		t.Fatalf("expected refresh waiver to return false on stale P_scan, got true")
	}

	decision := &kernel.Decision{Symbol: bless.symbol, Action: "open_short"}
	err := validateHunterV7RequiredConfirmations(&cand, decision)
	if err == nil || err.Error() != bless.wantGuardErr {
		t.Fatalf("validateHunterV7RequiredConfirmations after refresh:\n got: %v\nwant: %s", err, bless.wantGuardErr)
	}

	// Bar-based code divergence on the same refresh-waiver path:
	// Scoring-time V7ExecutionContext.Timeframes["15m"] had LastClose above
	// EMA20/VWAP20/zone-mid (failing directional_15m_close_short), while live
	// prompt market data has a forming bar below them (passing live verifier).
	pBase := bless.zone.Upper * 1.005
	pClosedHigh := bless.zone.Upper * 1.015
	scoringBars := make([]local.V7Bar, 25)
	for i := range scoringBars {
		scoringBars[i] = local.V7Bar{High: pBase, Low: pBase, Close: pBase, Volume: 1000}
	}
	scoringBars[24].Close = pClosedHigh
	scoringBars[24].High = pClosedHigh
	scoringTF15, _ := local.V7TimeframeFactsFromBars("15m", scoringBars, pBase)

	barCand := cand
	barCand.V7RequiredConfirms = []string{"directional_15m_close_short"}
	barCand.V7ExecutionContext = &local.V7ExecutionContext{
		DataQuality: "complete_for_execution",
		Timeframes:  map[string]local.V7ExecutionTimeframeSummary{"15m": scoringTF15},
	}
	barCand.V7ConfirmSummary = &local.V7ConfirmationSummary{
		PassedHard:   true,
		PassedReview: false,
		MissingReview: []local.V7ConfirmationCheck{
			{Code: "directional_15m_close_short", Passed: false, Severity: local.V7ConfirmReviewWait},
		},
	}

	liveData := &market.Data{
		Symbol:       bless.symbol,
		CurrentPrice: bless.pLive,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"15m": func() *market.TimeframeSeriesData {
				s := u4GuardSyntheticSeries("15m", 15*time.Minute, bless.promptTimeUTC, pBase, bless.pLive)
				s.Klines[24].Close = pClosedHigh
				s.Klines[24].High = pClosedHigh
				s.EMA20Values[24] = pBase
				s.EMA20Values[25] = pBase
				return s
			}(),
		},
	}

	livePassed, liveKnown := kernel.HunterV7VerifyConfirmation(barCand, liveData, bless.pLive, "directional_15m_close_short")
	if !liveKnown || !livePassed {
		t.Fatalf("live directional_15m_close_short = (%v, %v), want (true, true) on forming bar", livePassed, liveKnown)
	}
	guardErr := validateHunterV7RequiredConfirmations(&barCand, decision)
	wantBarErr := fmt.Sprintf("❌ [HUNTER V7 GUARD] %s %s blocked: required confirmation directional_15m_close_short failed (confirmation_missing)",
		bless.setupType, bless.symbol)
	if guardErr == nil || guardErr.Error() != wantBarErr {
		t.Fatalf("guard directional_15m_close_short after refresh:\n got: %v\nwant: %s", guardErr, wantBarErr)
	}
}
