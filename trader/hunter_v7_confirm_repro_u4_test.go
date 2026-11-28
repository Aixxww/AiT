package trader

import (
	"fmt"
	"math"
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
	tf5, _ := local.V7TimeframeFactsFromBars("5m", mkClosedBars(5*time.Minute), tc.pScan)
	tf15, _ := local.V7TimeframeFactsFromBars("15m", mkClosedBars(15*time.Minute), tc.pScan)

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
		ClosedConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{"5m": tf5, "15m": tf15},
	}
	summary := local.EvaluateV7Confirmations(sig, symCtx, v7Cfg)

	coin := kernel.CandidateCoin{
		Symbol:              tc.symbol,
		Sources:             []string{"hunter_v7"},
		Direction:           "SHORT",
		V7SignalID:          sig.SignalID,
		V7SetupType:         string(tc.setupType),
		V7Status:            string(local.V7StatusCandidate),
		V7EntryMode:         string(local.V7EntryFastConfirm),
		V7ExecutionQuality:  string(local.V7ExecReady),
		V7Confidence:        "B",
		V7RiskLevel:         "LOW",
		V7AIPriority:        tc.aiPriority,
		V7SetupScore:        90,
		V7TimingScore:       tc.timingScore,
		V7RegimeFitScore:    80,
		V7LiquidityScore:    tc.liquidityScore,
		V7RiskScore:         0,
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
		V7ConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{"5m": tf5, "15m": tf15},
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

// TestU4ConfirmRequiredGuardRejectsThreePromotedOpens (updated for U5/M2b):
// Drives the real pipeline — scoring -> PrepareHunterV7Tiers -> BuildUserPrompt
// -> validateHunterV7ExecutionGuard — on C16 (BLESS), C18 (ICNT), and C31 (PONS)
// in BOTH regimes:
//  1. Forming-only dip (no closed 5m/15m bar below entry_zone_lower):
//     Prompt and guard AGREE the trigger confirmation is NOT passed (divergence=0).
//     Prompt stays REVIEWABLE/WATCH with passed_review=false, missing_review, and
//     bar_evidence; guard rejects the open.
//  2. Positive control (closed 5m bar genuinely closed below entry_zone_lower):
//     Prompt promotes to EXECUTABLE with passed_review=true + bar_evidence,
//     write-back delivers the identical updated summary to ctx.CandidateCoins,
//     and validateHunterV7ExecutionGuard ACCEPTS the open.
func TestU4ConfirmRequiredGuardRejectsThreePromotedOpens(t *testing.T) {
	at := u4GuardTrader()
	geom := kernel.HunterV7ExecutionGeometryForConfig(at.config.StrategyConfig)

	for _, tc := range u4GuardCases() {
		t.Run(tc.name+"/forming_only_dip_prompt_and_guard_agree_reject", func(t *testing.T) {
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

			if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
				t.Fatalf("PrepareHunterV7Tiers: %v", err)
			}

			prompt := at.strategyEngine.BuildUserPrompt(ctx)
			// U5/M2b: forming-only dip must NOT promote to EXECUTABLE or claim passed_review=true.
			if strings.Contains(prompt, "execution_tier=EXECUTABLE") {
				t.Fatalf("%s prompt must NOT promote forming-only dip to EXECUTABLE:\n%s", tc.name, prompt)
			}
			if !strings.Contains(prompt, "execution_tier="+tc.wantAssemTier) && tc.wantAssemTier != "WATCH" {
				t.Fatalf("%s prompt missing expected tier %s:\n%s", tc.name, tc.wantAssemTier, prompt)
			}
			if tc.wantAssemTier == "REVIEWABLE" {
				if !strings.Contains(prompt, `"passed_review":false`) ||
					!strings.Contains(prompt, `"missing_review"`) ||
					!strings.Contains(prompt, `"bar_evidence"`) {
					t.Fatalf("%s prompt payload must show passed_review=false + missing_review + bar_evidence:\n%s", tc.name, prompt)
				}
			}

			// Guard reads written-back ctx.CandidateCoins[0] and agrees: NOT passed -> rejects!
			guardCoin := &ctx.CandidateCoins[0]
			if guardCoin.V7ExecutionTier != tc.wantAssemTier {
				t.Fatalf("%s guardCoin tier = %q, want %q", tc.name, guardCoin.V7ExecutionTier, tc.wantAssemTier)
			}
			if guardCoin.V7ConfirmSummary == nil || guardCoin.V7ConfirmSummary.PassedReview {
				t.Fatalf("%s guardCoin summary must agree PassedReview=false", tc.name)
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
				SelectedHunterV7Tier:     tc.wantAssemTier,
			}
			if tc.wantAssemTier == "REVIEWABLE" {
				// Even after applying fresh refresh evidence, guard must still reject
				// because closed 5m/15m bars never crossed EntryZone.Lower.
				hunterV7RefreshEvidence{RestConfirmed: true, MicroConfirmed: true}.applyTo(guardCoin)
			}
			err := at.validateHunterV7ExecutionGuard(ctx, decision)
			if err == nil {
				t.Fatalf("%s: expected guard rejection on forming-only dip, got nil", tc.name)
			}
			if tc.wantAssemTier == "REVIEWABLE" && err.Error() != tc.wantGuardErr {
				t.Fatalf("%s guard error mismatch:\n got: %s\nwant: %s", tc.name, err.Error(), tc.wantGuardErr)
			}
			if tc.wantAssemTier == "WATCH" && !strings.Contains(err.Error(), "execution_tier=WATCH") {
				t.Fatalf("%s WATCH guard error mismatch: %v", tc.name, err)
			}
		})

		t.Run(tc.name+"/positive_control_closed_bar_prompt_and_guard_agree_accept", func(t *testing.T) {
			assemCoin := u4BuildAssembledCandidate(t, tc, geom)

			// Latest CLOSED 5m bar genuinely closed at tc.pLive <= EntryZone.Lower,
			// and live price then retested into the upper half of EntryZone
			// (zone_pos=60%, satisfying the separate MinZonePosShort guard while
			// proving live spot above EntryZone.Lower never fails a closed-bar trigger).
			pRetest := tc.zone.Lower + 0.60*(tc.zone.Upper-tc.zone.Lower)
			ctx := &kernel.Context{
				PromptTime:     tc.promptTimeUTC,
				CandidateCoins: []kernel.CandidateCoin{assemCoin},
				MarketDataMap: map[string]*market.Data{
					tc.symbol: {
						Symbol:       tc.symbol,
						CurrentPrice: pRetest,
						TimeframeData: map[string]*market.TimeframeSeriesData{
							"5m":  u4GuardSyntheticSeries("5m", 5*time.Minute, tc.promptTimeUTC, tc.pLive, pRetest),
							"15m": u4GuardSyntheticSeries("15m", 15*time.Minute, tc.promptTimeUTC, tc.pScan, pRetest),
						},
					},
				},
			}

			if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
				t.Fatalf("PrepareHunterV7Tiers: %v", err)
			}

			prompt := at.strategyEngine.BuildUserPrompt(ctx)
			if !strings.Contains(prompt, "execution_tier=EXECUTABLE") {
				t.Fatalf("%s positive control prompt did not promote to EXECUTABLE:\n%s", tc.name, prompt)
			}
			if !strings.Contains(prompt, `"passed_review":true`) {
				t.Fatalf("%s positive control prompt did not show passed_review=true:\n%s", tc.name, prompt)
			}
			if strings.Contains(prompt, `"missing_review"`) {
				t.Fatalf("%s positive control prompt unexpectedly contains missing_review:\n%s", tc.name, prompt)
			}
			if !strings.Contains(prompt, `"bar_evidence"`) || !strings.Contains(prompt, `"deciding_tf":"5m"`) {
				t.Fatalf("%s positive control prompt missing closed-bar evidence:\n%s", tc.name, prompt)
			}

			// Write-back delivered the exact updated summary to ctx.CandidateCoins[0].
			if ctx.CandidateCoins[0].V7ExecutionTier != "EXECUTABLE" ||
				ctx.CandidateCoins[0].V7ConfirmSummary == nil ||
				!ctx.CandidateCoins[0].V7ConfirmSummary.PassedReview {
				t.Fatalf("%s positive control write-back incomplete: tier=%q summary=%+v",
					tc.name, ctx.CandidateCoins[0].V7ExecutionTier, ctx.CandidateCoins[0].V7ConfirmSummary)
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
			if err := at.validateHunterV7ExecutionGuard(ctx, decision); err != nil {
				t.Fatalf("%s positive control: guard must ACCEPT genuinely closed-bar confirmation, got: %v", tc.name, err)
			}
		})
	}
}

// TestU4ConfirmRequiredReviewableRefreshWaiverFailsOnStaleFacts (updated for U5/M2b):
// Verifies that the guard's refresh-waiver path uses the same closed-bar shared
// evaluator and stays consistent by construction:
//  1. When closed 5m/15m bars did NOT cross EntryZone.Lower, even if spot price
//     V7PriceContext.Last is updated below EntryZone.Lower AND fresh_rest/micro
//     refresh evidence is applied, the waiver returns false (spot never passes).
//  2. When no closed 5m/15m bars are available at decision time, the waiver fails
//     closed (returns false).
//  3. For bar-based directional_15m_close_short, both the live verifier (at
//     promptTimeUTC) and the guard agree (false) when only the trailing forming
//     15m bar dipped below EMA20/VWAP20.
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

	// Even if live spot price is below EntryZone.Lower AND refresh evidence passes,
	// closed bars are still at P_scan (> EntryZone.Lower) -> waiver MUST fail closed!
	cand.V7PriceContext.Last = bless.pLive
	ev := hunterV7RefreshEvidence{RestConfirmed: true, MicroConfirmed: true}
	ev.applyTo(&cand)

	if hunterV7ReviewableConfirmationSatisfiedByRefresh(&cand, "5m_or_15m_close_below_trigger") {
		t.Fatalf("U5/M2b: refresh waiver must NOT pass on spot price when closed bars stay above EntryZone.Lower")
	}

	decision := &kernel.Decision{Symbol: bless.symbol, Action: "open_short"}
	err := validateHunterV7RequiredConfirmations(&cand, decision)
	if err == nil || err.Error() != bless.wantGuardErr {
		t.Fatalf("validateHunterV7RequiredConfirmations after refresh:\n got: %v\nwant: %s", err, bless.wantGuardErr)
	}

	// No closed bars available at decision time -> waiver fails closed for every
	// shared bar/trigger predicate (U5b Fix 1 / Item 7 blind spot).
	noBarCand := cand
	noBarCand.V7ConfirmTimeframes = map[string]local.V7ExecutionTimeframeSummary{}
	noBarCand.V7ExecutionContext = nil
	for _, code := range []string{
		"5m_or_15m_close_below_trigger",
		"directional_15m_close_short",
		"5m_price_holds_ema20_or_trailing_support",
	} {
		passed, known := kernel.HunterV7VerifyConfirmation(noBarCand, nil, 0, code)
		if !known || passed {
			t.Fatalf("U5b: HunterV7VerifyConfirmation(noBarCand, %q) = (passed=%v, known=%v), want (false, true)", code, passed, known)
		}
		if hunterV7ReviewableConfirmationSatisfiedByRefresh(&noBarCand, code) {
			t.Fatalf("U5b: refresh waiver must fail closed for %q when no closed 5m/15m bars exist", code)
		}
	}

	// Bar-based code agreement on directional_15m_close_short (U5/M2b):
	// Closed 15m bars stay above EMA20/VWAP20/zone-mid while only the trailing
	// forming bar dipped below -> BOTH live verifier and guard agree: NOT passed.
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
	barCand.V7ConfirmTimeframes = map[string]local.V7ExecutionTimeframeSummary{"15m": scoringTF15}
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

	livePassed, liveKnown := kernel.HunterV7VerifyConfirmationAt(barCand, liveData, bless.pLive, bless.promptTimeUTC, "directional_15m_close_short")
	if !liveKnown || livePassed {
		t.Fatalf("U5/M2b: live directional_15m_close_short = (%v, %v), want (false, true) — forming bar must be excluded", livePassed, liveKnown)
	}
	guardErr := validateHunterV7RequiredConfirmations(&barCand, decision)
	wantBarErr := fmt.Sprintf("❌ [HUNTER V7 GUARD] %s %s blocked: required confirmation directional_15m_close_short failed (confirmation_missing)",
		bless.setupType, bless.symbol)
	if guardErr == nil || guardErr.Error() != wantBarErr {
		t.Fatalf("guard directional_15m_close_short after refresh:\n got: %v\nwant: %s", guardErr, wantBarErr)
	}
}

// TestU5bPartialTimeframe5mOnlyPreserves15mKnownFailureThroughGuard (Fix 2 / Item 2A):
// When prompt MarketDataMap carries ONLY "5m" (no "15m" series), a
// directional_15m_close_short that known-failed on closed 15m bars at scoring
// MUST stay failed through live apply, prompt payload, write-back, and the
// guard refresh waiver (never wiped or waived).
func TestU5bPartialTimeframe5mOnlyPreserves15mKnownFailureThroughGuard(t *testing.T) {
	at := u4GuardTrader()
	geom := kernel.HunterV7ExecutionGeometryForConfig(at.config.StrategyConfig)
	bless := u4GuardCases()[0]

	pBase := bless.zone.Upper * 1.005
	pClosedHigh := bless.zone.Upper * 1.015 // strictly > EMA20, > VWAP20, > zoneMid
	pRetest := bless.zone.Lower + 0.60*(bless.zone.Upper-bless.zone.Lower)

	mkBars := func(iv time.Duration, base, lastClose float64) []local.V7Bar {
		formingOpen := bless.promptTimeUTC.Truncate(iv)
		bars := make([]local.V7Bar, 25)
		for i := range bars {
			openAt := formingOpen.Add(-time.Duration(25-i) * iv)
			c := base
			if i == len(bars)-1 {
				c = lastClose
			}
			bars[i] = local.V7Bar{
				OpenTime:  openAt.UnixMilli(),
				CloseTime: openAt.Add(iv).UnixMilli() - 1,
				High:      c * 1.001,
				Low:       c * 0.999,
				Close:     c,
				Volume:    1000,
			}
		}
		return bars
	}
	tf5Scoring, ok5 := local.V7TimeframeFactsFromBars("5m", mkBars(5*time.Minute, bless.pScan, bless.pScan), bless.pScan)
	tf15Scoring, ok15 := local.V7TimeframeFactsFromBars("15m", mkBars(15*time.Minute, pBase, pClosedHigh), pBase)
	if !ok5 || !ok15 {
		t.Fatalf("failed to build scoring timeframe facts")
	}

	required := []string{"directional_15m_close_short", "taker_buy_15m_lt_0_48"}
	sig := &local.V7SignalOutput{
		SignalID:         "sig_" + bless.symbol + "_partial5m",
		Symbol:           bless.symbol,
		Direction:        local.V7DirShort,
		SetupType:        bless.setupType,
		Status:           local.V7StatusCandidate,
		EntryMode:        local.V7EntryFastConfirm,
		ExecutionQuality: local.V7ExecReady,
		Confidence:       "B",
		RiskLevel:        local.V7RiskLow,
		AIPriority:       bless.aiPriority,
		SetupScore:       90,
		TimingScore:      bless.timingScore,
		RegimeFitScore:   80,
		LiquidityScore:   bless.liquidityScore,
		EntryZone:        bless.zone,
		Invalidation:     local.V7InvalidationRule{Price: bless.invalidation},
		Targets:          []local.V7Target{{Price: bless.tp1}, {Price: bless.tp2}},
		RequiredConfirms: required,
		ReasonCodes:      append([]string{}, bless.reasonCodes...),
	}
	v7Cfg := local.DefaultV7Config()
	v7Cfg.MinRiskRewardRatio = geom.MinRR
	symCtx := &local.V7SymbolContext{
		Symbol:       bless.symbol,
		CurrentPrice: pRetest,
		TakerBuy15m:  bless.taker15m,
		Snapshot:     &local.SymbolSnapshotData{OIDelta1h: bless.oi1h, OIDelta4h: bless.oi4h},
		ExecutionContext: &local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes:  map[string]local.V7ExecutionTimeframeSummary{"5m": tf5Scoring, "15m": tf15Scoring},
		},
		ClosedConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{"5m": tf5Scoring, "15m": tf15Scoring},
	}
	summary := local.EvaluateV7Confirmations(sig, symCtx, v7Cfg)
	if summary.PassedReview || len(summary.MissingReview) != 1 || summary.MissingReview[0].Code != "directional_15m_close_short" {
		t.Fatalf("scoring fixture must fail directional_15m_close_short: %+v", summary)
	}
	if summary.MissingReview[0].Evidence == nil || summary.MissingReview[0].Evidence.DecidingTF != "15m" ||
		summary.MissingReview[0].Evidence.BarClose != pClosedHigh {
		t.Fatalf("scoring fixture evidence mismatch: %+v", summary.MissingReview[0].Evidence)
	}

	cand := u4BuildAssembledCandidate(t, bless, geom)
	cand.V7SignalID = sig.SignalID
	cand.V7RequiredConfirms = required
	cand.V7ConfirmSummary = &summary
	cand.V7ExecutionContext = symCtx.ExecutionContext
	cand.V7ConfirmTimeframes = map[string]local.V7ExecutionTimeframeSummary{"5m": tf5Scoring, "15m": tf15Scoring}
	cand.V7PriceContext.Last = pRetest
	cand.V7ExecutionTier, cand.V7TierReason = kernel.ClassifyHunterV7CandidateTierForRuntime(cand, geom)
	if cand.V7ExecutionTier != "REVIEWABLE" || cand.V7TierReason != "live_reviewable_directional_15m_close_short" {
		t.Fatalf("assembly = (%q, %q), want (REVIEWABLE, live_reviewable_directional_15m_close_short)", cand.V7ExecutionTier, cand.V7TierReason)
	}

	// Prompt market data carries ONLY 5m (no 15m series at all).
	ctx := &kernel.Context{
		PromptTime:     bless.promptTimeUTC,
		CandidateCoins: []kernel.CandidateCoin{cand},
		MarketDataMap: map[string]*market.Data{
			bless.symbol: {
				Symbol:       bless.symbol,
				CurrentPrice: pRetest,
				TimeframeData: map[string]*market.TimeframeSeriesData{
					"5m": u4GuardSyntheticSeries("5m", 5*time.Minute, bless.promptTimeUTC, bless.pLive, pRetest),
				},
			},
		},
	}

	if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
		t.Fatalf("PrepareHunterV7Tiers: %v", err)
	}
	prompt := at.strategyEngine.BuildUserPrompt(ctx)
	if strings.Contains(prompt, "execution_tier=EXECUTABLE") {
		t.Fatalf("5m-only prompt must NOT promote failed directional_15m_close_short to EXECUTABLE:\n%s", prompt)
	}
	if !strings.Contains(prompt, `"passed_review":false`) || !strings.Contains(prompt, `"deciding_tf":"15m"`) {
		t.Fatalf("5m-only prompt must preserve failed directional_15m_close_short and its 15m bar evidence:\n%s", prompt)
	}
	if strings.Contains(prompt, `"deciding_tf":"none"`) {
		t.Fatalf("5m-only prompt clobbered scoring 15m BarEvidence with deciding_tf=none:\n%s", prompt)
	}

	guardCoin := &ctx.CandidateCoins[0]
	if guardCoin.V7ConfirmSummary == nil || guardCoin.V7ConfirmSummary.PassedReview || len(guardCoin.V7ConfirmSummary.MissingReview) != 1 {
		t.Fatalf("write-back lost failed directional_15m_close_short: %+v", guardCoin.V7ConfirmSummary)
	}
	if ev := guardCoin.V7ConfirmSummary.MissingReview[0].Evidence; ev == nil || ev.DecidingTF != "15m" || ev.BarClose != pClosedHigh {
		t.Fatalf("write-back clobbered scoring 15m evidence: %+v", ev)
	}
	if got15, ok := guardCoin.V7ConfirmTimeframes["15m"]; !ok || got15.LastClose != pClosedHigh {
		t.Fatalf("write-back wiped scoring 15m closed-bar summary: %+v", guardCoin.V7ConfirmTimeframes)
	}
	if got5, ok := guardCoin.V7ConfirmTimeframes["5m"]; !ok || got5.LastClose != bless.pLive {
		t.Fatalf("write-back did not merge fresh 5m closed-bar summary: %+v", guardCoin.V7ConfirmTimeframes)
	}

	// Guard waiver MUST reject even after applying fresh refresh evidence.
	hunterV7RefreshEvidence{RestConfirmed: true, MicroConfirmed: true}.applyTo(guardCoin)
	if hunterV7ReviewableConfirmationSatisfiedByRefresh(guardCoin, "directional_15m_close_short") {
		t.Fatalf("U5b Fix 2: guard waiver accepted directional_15m_close_short when prompt carried only 5m")
	}
	decision := &kernel.Decision{
		Symbol:                   bless.symbol,
		Action:                   "open_short",
		Leverage:                 10,
		StopLoss:                 bless.decisionSL,
		TakeProfit:               bless.decisionTP,
		Confidence:               bless.confidence,
		SelectedHunterV7SignalID: sig.SignalID,
		SelectedHunterV7Setup:    string(bless.setupType),
		SelectedHunterV7Tier:     "REVIEWABLE",
	}
	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err == nil || !strings.Contains(err.Error(), "directional_15m_close_short failed (confirmation_missing)") {
		t.Fatalf("U5b Fix 2: expected guard rejection for directional_15m_close_short, got %v", err)
	}
}

// TestU5bTwoWayLiveRecheckDemotesTriggerWhenNextBarClosesBackAcrossEdge (Fix 4 / Medium 1):
// A trigger code passes at scoring on closed 5m bar N (<= EntryZone.Lower), then
// before prompt time bar N+1 closes back above EntryZone.Lower. Live apply MUST
// demote the code into MissingReview, set PassedReview=false, update BarEvidence
// to bar N+1, render it missing in the payload, and the guard MUST reject.
func TestU5bTwoWayLiveRecheckDemotesTriggerWhenNextBarClosesBackAcrossEdge(t *testing.T) {
	at := u4GuardTrader()
	geom := kernel.HunterV7ExecutionGeometryForConfig(at.config.StrategyConfig)
	bless := u4GuardCases()[0]

	// Scoring at bar N: closed 5m bar N closed at bless.pLive (<= EntryZone.Lower),
	// 15m closed at bless.pScan (> EntryZone.Lower), spot retesting inside zone.
	pRetest := bless.zone.Lower + 0.60*(bless.zone.Upper-bless.zone.Lower)
	scoringTime := bless.promptTimeUTC.Add(-5 * time.Minute)
	mkScoringBars := func(iv time.Duration, ref time.Time, closeVal float64) []local.V7Bar {
		formingOpen := ref.Truncate(iv)
		bars := make([]local.V7Bar, 25)
		for i := range bars {
			openAt := formingOpen.Add(-time.Duration(25-i) * iv)
			bars[i] = local.V7Bar{
				OpenTime:  openAt.UnixMilli(),
				CloseTime: openAt.Add(iv).UnixMilli() - 1,
				High:      closeVal * 1.001,
				Low:       closeVal * 0.999,
				Close:     closeVal,
				Volume:    1000,
			}
		}
		return bars
	}
	tf5BarN, _ := local.V7TimeframeFactsFromBars("5m", mkScoringBars(5*time.Minute, scoringTime, bless.pLive), bless.pLive)
	tf15BarN, _ := local.V7TimeframeFactsFromBars("15m", mkScoringBars(15*time.Minute, scoringTime, bless.pScan), bless.pScan)

	sig := &local.V7SignalOutput{
		SignalID:         "sig_" + bless.symbol + "_demote",
		Symbol:           bless.symbol,
		Direction:        local.V7DirShort,
		SetupType:        bless.setupType,
		Status:           local.V7StatusCandidate,
		EntryMode:        local.V7EntryFastConfirm,
		ExecutionQuality: local.V7ExecReady,
		Confidence:       "B",
		RiskLevel:        local.V7RiskLow,
		AIPriority:       bless.aiPriority,
		SetupScore:       90,
		TimingScore:      bless.timingScore,
		RegimeFitScore:   80,
		LiquidityScore:   bless.liquidityScore,
		EntryZone:        bless.zone,
		Invalidation:     local.V7InvalidationRule{Price: bless.invalidation},
		Targets:          []local.V7Target{{Price: bless.tp1}, {Price: bless.tp2}},
		RequiredConfirms: append([]string{}, bless.required...),
		ReasonCodes:      append([]string{}, bless.reasonCodes...),
	}
	v7Cfg := local.DefaultV7Config()
	v7Cfg.MinRiskRewardRatio = geom.MinRR
	symCtx := &local.V7SymbolContext{
		Symbol:       bless.symbol,
		CurrentPrice: pRetest,
		TakerBuy15m:  bless.taker15m,
		Snapshot:     &local.SymbolSnapshotData{OIDelta1h: bless.oi1h, OIDelta4h: bless.oi4h},
		ExecutionContext: &local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes:  map[string]local.V7ExecutionTimeframeSummary{"5m": tf5BarN, "15m": tf15BarN},
		},
		ClosedConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{"5m": tf5BarN, "15m": tf15BarN},
	}
	summary := local.EvaluateV7Confirmations(sig, symCtx, v7Cfg)
	if !summary.PassedReview || len(summary.MissingReview) != 0 {
		t.Fatalf("scoring fixture on bar N must pass trigger confirmation: %+v", summary)
	}

	cand := u4BuildAssembledCandidate(t, bless, geom)
	cand.V7SignalID = sig.SignalID
	cand.V7ConfirmSummary = &summary
	cand.V7ExecutionContext = symCtx.ExecutionContext
	cand.V7ConfirmTimeframes = map[string]local.V7ExecutionTimeframeSummary{"5m": tf5BarN, "15m": tf15BarN}
	cand.V7PriceContext.Last = pRetest
	cand.V7ExecutionTier, cand.V7TierReason = kernel.ClassifyHunterV7CandidateTierForRuntime(cand, geom)
	if cand.V7ExecutionTier != "EXECUTABLE" {
		t.Fatalf("assembly on passing bar N = (%q, %q), want EXECUTABLE", cand.V7ExecutionTier, cand.V7TierReason)
	}

	// By promptTimeUTC (bar N+1 closed), the latest closed 5m bar closed BACK
	// ABOVE EntryZone.Lower at bless.pScan (while 15m is also at bless.pScan).
	ctx := &kernel.Context{
		PromptTime:     bless.promptTimeUTC,
		CandidateCoins: []kernel.CandidateCoin{cand},
		MarketDataMap: map[string]*market.Data{
			bless.symbol: {
				Symbol:       bless.symbol,
				CurrentPrice: pRetest,
				TimeframeData: map[string]*market.TimeframeSeriesData{
					"5m":  u4GuardSyntheticSeries("5m", 5*time.Minute, bless.promptTimeUTC, bless.pScan, pRetest),
					"15m": u4GuardSyntheticSeries("15m", 15*time.Minute, bless.promptTimeUTC, bless.pScan, pRetest),
				},
			},
		},
	}

	if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
		t.Fatalf("PrepareHunterV7Tiers: %v", err)
	}
	prompt := at.strategyEngine.BuildUserPrompt(ctx)
	if strings.Contains(prompt, "execution_tier=EXECUTABLE") {
		t.Fatalf("U5b Fix 4: prompt must demote candidate from EXECUTABLE after bar N+1 closed back across edge:\n%s", prompt)
	}
	if !strings.Contains(prompt, `"passed_review":false`) ||
		!strings.Contains(prompt, `"missing_review":[{"code":"5m_or_15m_close_below_trigger"`) {
		t.Fatalf("U5b Fix 4: prompt payload must show passed_review=false and demoted trigger in missing_review:\n%s", prompt)
	}

	guardCoin := &ctx.CandidateCoins[0]
	if guardCoin.V7ExecutionTier != "REVIEWABLE" || guardCoin.V7ConfirmSummary == nil || guardCoin.V7ConfirmSummary.PassedReview {
		t.Fatalf("U5b Fix 4: write-back must demote candidate: tier=%q summary=%+v", guardCoin.V7ExecutionTier, guardCoin.V7ConfirmSummary)
	}
	if len(guardCoin.V7ConfirmSummary.MissingReview) != 1 || guardCoin.V7ConfirmSummary.MissingReview[0].Code != "5m_or_15m_close_below_trigger" {
		t.Fatalf("U5b Fix 4: expected demoted trigger in MissingReview, got %+v", guardCoin.V7ConfirmSummary.MissingReview)
	}
	wantBarN1CloseUnix := bless.promptTimeUTC.Truncate(5 * time.Minute).Unix()
	var barEv *local.V7ConfirmBarEvidence
	for i := range guardCoin.V7ConfirmSummary.BarEvidence {
		if guardCoin.V7ConfirmSummary.BarEvidence[i].Code == "5m_or_15m_close_below_trigger" {
			barEv = &guardCoin.V7ConfirmSummary.BarEvidence[i]
			break
		}
	}
	if barEv == nil || barEv.Passed || barEv.BarClose != bless.pScan || barEv.BarCloseTimeUnix != wantBarN1CloseUnix {
		t.Fatalf("U5b Fix 4: BarEvidence must reflect failing bar N+1: got %+v (want close=%v unix=%d)", barEv, bless.pScan, wantBarN1CloseUnix)
	}

	// Guard MUST reject even after fresh refresh evidence.
	hunterV7RefreshEvidence{RestConfirmed: true, MicroConfirmed: true}.applyTo(guardCoin)
	decision := &kernel.Decision{
		Symbol:                   bless.symbol,
		Action:                   "open_short",
		Leverage:                 10,
		StopLoss:                 bless.decisionSL,
		TakeProfit:               bless.decisionTP,
		Confidence:               bless.confidence,
		SelectedHunterV7SignalID: sig.SignalID,
		SelectedHunterV7Setup:    string(bless.setupType),
		SelectedHunterV7Tier:     "REVIEWABLE",
	}
	err := at.validateHunterV7ExecutionGuard(ctx, decision)
	if err == nil || err.Error() != bless.wantGuardErr {
		t.Fatalf("U5b Fix 4: expected guard rejection after demotion:\n got: %v\nwant: %s", err, bless.wantGuardErr)
	}
}

// u5cBuild200Raw15mKlines builds 200 raw 15m klines (199 closed + 1 trailing
// forming bar at promptTime) where the last closed bar (index 198) satisfies
// directional_15m_close_short ONLY via the VWAP20 leg when lastClosedClose is
// below the high-volume VWAP20 spike yet strictly above EMA20 and zoneMid.
func u5cBuild200Raw15mKlines(promptTime time.Time, zone local.V7PriceZone, lastClosedClose, formingClose float64) []market.Kline {
	const iv = 15 * time.Minute
	formingOpen := promptTime.Truncate(iv)
	zoneMid := (zone.Lower + zone.Upper) / 2
	base := zoneMid * 0.995      // low baseline so EMA20 stays below lastClosedClose
	spike := zone.Upper * 1.12   // high-volume spike inside trailing 20 bars to lift VWAP20
	out := make([]market.Kline, 200)
	for i := 0; i < 199; i++ {
		openAt := formingOpen.Add(-time.Duration(199-i) * iv)
		c := base * (1 + 0.0002*float64(i%3))
		vol := 10.0 + float64(i%5)
		if i == 180 {
			// Single high-volume spike inside trailing 20 bars: lifts VWAP20
			// well above lastClosedClose while decaying out of EMA20 by bar 198.
			c = spike
			vol = 100000.0
		} else if i == 198 {
			c = lastClosedClose
			vol = 100.0
		}
		out[i] = market.Kline{
			OpenTime:  openAt.UnixMilli(),
			CloseTime: openAt.Add(iv).UnixMilli() - 1,
			Open:      c * 0.9995,
			High:      c * 1.001,
			Low:       c * 0.999,
			Close:     c,
			Volume:    vol,
		}
	}
	out[199] = market.Kline{
		OpenTime:  formingOpen.UnixMilli(),
		CloseTime: formingOpen.Add(iv).UnixMilli() - 1,
		Open:      lastClosedClose,
		High:      math.Max(lastClosedClose, formingClose) * 1.001,
		Low:       math.Min(lastClosedClose, formingClose) * 0.999,
		Close:     formingClose,
		Volume:    50.0,
	}
	return out
}

func u5cKlinesToV7Bars(kl []market.Kline) []local.V7Bar {
	out := make([]local.V7Bar, len(kl))
	for i, k := range kl {
		out[i] = local.V7Bar{
			OpenTime:  k.OpenTime,
			CloseTime: k.CloseTime,
			High:      k.High,
			Low:       k.Low,
			Close:     k.Close,
			Volume:    k.Volume,
		}
	}
	return out
}

// TestU5cRegressionA200Raw15mKlinesPrimaryCount20PreservesVWAP20ThroughGuard
// verifies Regression (a): 200 raw 15m klines sliced at default PrimaryCount=20
// (19 closed + 1 forming) via market.BuildDataFromPreFetched preserve full-
// history VWAP20 so a directional_15m_close_short satisfied at scoring ONLY via
// the VWAP20 leg is NOT demoted at prompt time, its BarEvidence is preserved,
// and the execution guard accepts.
func TestU5cRegressionA200Raw15mKlinesPrimaryCount20PreservesVWAP20ThroughGuard(t *testing.T) {
	at := u4GuardTrader()
	geom := kernel.HunterV7ExecutionGeometryForConfig(at.config.StrategyConfig)
	bless := u4GuardCases()[0]
	zoneMid := (bless.zone.Lower + bless.zone.Upper) / 2
	pRetest := bless.zone.Lower + 0.60*(bless.zone.Upper-bless.zone.Lower)

	// Last closed 15m close sits above zoneMid and above EMA20, yet below VWAP20.
	lastClosed15m := bless.zone.Upper * 1.025
	raw15m := u5cBuild200Raw15mKlines(bless.promptTimeUTC, bless.zone, lastClosed15m, pRetest)
	scoringTF15, ok := local.V7TimeframeFactsFromBars("15m", u5cKlinesToV7Bars(raw15m[:199]), 0)
	if !ok || !scoringTF15.HasVWAP20 || !scoringTF15.HasEMA20 {
		t.Fatalf("scoring 15m facts missing indicators: %+v", scoringTF15)
	}
	// Pin the exact precondition: ONLY the VWAP20 leg satisfies directional_15m_close_short.
	if scoringTF15.CloseVsVWAP20Pct > 0 || scoringTF15.CloseVsEMA20Pct <= 0 || scoringTF15.LastClose <= zoneMid {
		t.Fatalf("precondition failed: want Close<VWAP20, Close>EMA20, Close>zoneMid(%v); got %+v", zoneMid, scoringTF15)
	}

	// Build 200 raw 5m klines for market.BuildDataFromPreFetched as well.
	raw5m := make([]market.Kline, 200)
	formingOpen5m := bless.promptTimeUTC.Truncate(5 * time.Minute)
	for i := 0; i < 199; i++ {
		openAt := formingOpen5m.Add(-time.Duration(199-i) * 5 * time.Minute)
		c := pRetest * (1 + 0.0002*float64(i%3))
		raw5m[i] = market.Kline{
			OpenTime:  openAt.UnixMilli(),
			CloseTime: openAt.Add(5*time.Minute).UnixMilli() - 1,
			Open:      c,
			High:      c * 1.001,
			Low:       c * 0.999,
			Close:     c,
			Volume:    1000 + float64(i),
		}
	}
	raw5m[199] = market.Kline{
		OpenTime:  formingOpen5m.UnixMilli(),
		CloseTime: formingOpen5m.Add(5*time.Minute).UnixMilli() - 1,
		Open:      pRetest,
		High:      pRetest * 1.001,
		Low:       pRetest * 0.999,
		Close:     pRetest,
		Volume:    500,
	}
	scoringTF5, _ := local.V7TimeframeFactsFromBars("5m", u5cKlinesToV7Bars(raw5m[:199]), 0)

	sig := &local.V7SignalOutput{
		SignalID:         "sig_" + bless.symbol + "_u5c_reg_a",
		Symbol:           bless.symbol,
		Direction:        local.V7DirShort,
		SetupType:        bless.setupType,
		Status:           local.V7StatusCandidate,
		EntryMode:        local.V7EntryFastConfirm,
		ExecutionQuality: local.V7ExecReady,
		Confidence:       "B",
		RiskLevel:        local.V7RiskLow,
		AIPriority:       bless.aiPriority,
		SetupScore:       90,
		TimingScore:      bless.timingScore,
		RegimeFitScore:   80,
		LiquidityScore:   bless.liquidityScore,
		EntryZone:        bless.zone,
		Invalidation:     local.V7InvalidationRule{Price: bless.invalidation},
		Targets:          []local.V7Target{{Price: bless.tp1}, {Price: bless.tp2}},
		RequiredConfirms: []string{"directional_15m_close_short", "taker_buy_15m_lt_0_48", "live_price_in_entry_zone"},
		ReasonCodes:      append([]string{}, bless.reasonCodes...),
	}
	v7Cfg := local.DefaultV7Config()
	v7Cfg.MinRiskRewardRatio = geom.MinRR
	symCtx := &local.V7SymbolContext{
		Symbol:       bless.symbol,
		CurrentPrice: pRetest,
		TakerBuy15m:  bless.taker15m,
		Snapshot:     &local.SymbolSnapshotData{OIDelta1h: bless.oi1h, OIDelta4h: bless.oi4h},
		ExecutionContext: &local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes:  map[string]local.V7ExecutionTimeframeSummary{"5m": scoringTF5, "15m": scoringTF15},
		},
		ClosedConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{"5m": scoringTF5, "15m": scoringTF15},
	}
	summary := local.EvaluateV7Confirmations(sig, symCtx, v7Cfg)
	if !summary.PassedReview || len(summary.MissingReview) != 0 {
		t.Fatalf("scoring must pass directional_15m_close_short via VWAP20 leg: %+v", summary)
	}

	// Slice 200 raw klines at production default PrimaryCount=20 -> 19 closed + 1 forming.
	promptMarketData, err := market.BuildDataFromPreFetched(
		bless.symbol,
		&market.PreFetchedData{TimeframeKlines: map[string][]market.Kline{"15m": raw15m, "5m": raw5m}},
		[]string{"15m", "5m"},
		"15m",
		20,
	)
	if err != nil {
		t.Fatalf("BuildDataFromPreFetched: %v", err)
	}
	promptMarketData.CurrentPrice = pRetest
	if len(promptMarketData.TimeframeData["15m"].Klines) != 20 {
		t.Fatalf("expected PrimaryCount=20 retained 15m klines, got %d", len(promptMarketData.TimeframeData["15m"].Klines))
	}

	cand := u4BuildAssembledCandidate(t, bless, geom)
	cand.V7SignalID = sig.SignalID
	cand.V7RequiredConfirms = append([]string{}, sig.RequiredConfirms...)
	cand.V7ConfirmSummary = &summary
	cand.V7ExecutionContext = symCtx.ExecutionContext
	cand.V7ConfirmTimeframes = map[string]local.V7ExecutionTimeframeSummary{"5m": scoringTF5, "15m": scoringTF15}
	cand.V7PriceContext.Last = pRetest
	cand.V7ExecutionTier, cand.V7TierReason = kernel.ClassifyHunterV7CandidateTierForRuntime(cand, geom)
	if cand.V7ExecutionTier != "EXECUTABLE" {
		t.Fatalf("assembly tier = (%q, %q), want EXECUTABLE", cand.V7ExecutionTier, cand.V7TierReason)
	}

	ctx := &kernel.Context{
		PromptTime:     bless.promptTimeUTC,
		CandidateCoins: []kernel.CandidateCoin{cand},
		MarketDataMap:  map[string]*market.Data{bless.symbol: promptMarketData},
	}
	if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
		t.Fatalf("PrepareHunterV7Tiers: %v", err)
	}
	prompt := at.strategyEngine.BuildUserPrompt(ctx)
	if !strings.Contains(prompt, "execution_tier=EXECUTABLE") || !strings.Contains(prompt, `"passed_review":true`) {
		t.Fatalf("Regression (a): candidate must stay EXECUTABLE with passed_review=true under PrimaryCount=20:\n%s", prompt)
	}

	guardCoin := &ctx.CandidateCoins[0]
	if guardCoin.V7ExecutionTier != "EXECUTABLE" || !guardCoin.V7ConfirmSummary.PassedReview || len(guardCoin.V7ConfirmSummary.MissingReview) != 0 {
		t.Fatalf("Regression (a): directional_15m_close_short falsely demoted: tier=%q summary=%+v",
			guardCoin.V7ExecutionTier, guardCoin.V7ConfirmSummary)
	}
	if tf15 := guardCoin.V7ConfirmTimeframes["15m"]; !tf15.HasVWAP20 || math.Abs(tf15.VWAP20-scoringTF15.VWAP20) > 1e-12 {
		t.Fatalf("Regression (a): 15m VWAP20 lost after PrimaryCount=20 slice: got %+v, want VWAP20=%v", tf15, scoringTF15.VWAP20)
	}
	var dirEv *local.V7ConfirmBarEvidence
	for i := range guardCoin.V7ConfirmSummary.BarEvidence {
		if guardCoin.V7ConfirmSummary.BarEvidence[i].Code == "directional_15m_close_short" {
			dirEv = &guardCoin.V7ConfirmSummary.BarEvidence[i]
			break
		}
	}
	if dirEv == nil || !dirEv.Passed || dirEv.DecidingTF != "15m" || !dirEv.ClosedOnly ||
		math.Abs(dirEv.ReferenceEdge-scoringTF15.VWAP20) > 1e-9 || dirEv.BarCloseTimeUnix != scoringTF15.LastCloseTimeUnix {
		t.Fatalf("Regression (a): BarEvidence not preserved: got %+v", dirEv)
	}

	// Execution guard MUST accept.
	decision := &kernel.Decision{
		Symbol:                   bless.symbol,
		Action:                   "open_short",
		Leverage:                 10,
		StopLoss:                 bless.decisionSL,
		TakeProfit:               bless.decisionTP,
		Confidence:               bless.confidence,
		SelectedHunterV7SignalID: sig.SignalID,
		SelectedHunterV7Setup:    string(bless.setupType),
		SelectedHunterV7Tier:     "EXECUTABLE",
	}
	if err := at.validateHunterV7ExecutionGuard(ctx, decision); err != nil {
		t.Fatalf("Regression (a): expected guard to accept, got %v", err)
	}

	// Also verify Fix B independently of Fix C same-bar carry-forward: advance
	// promptTime by +15m (genuinely NEW closed 15m bar N+1, 200 raw klines sliced
	// at PrimaryCount=20, still satisfied ONLY via the VWAP20 leg).
	nextPromptTime := bless.promptTimeUTC.Add(15 * time.Minute)
	raw15mNext := u5cBuild200Raw15mKlines(nextPromptTime, bless.zone, lastClosed15m, pRetest)
	wantNextTF15, _ := local.V7TimeframeFactsFromBars("15m", u5cKlinesToV7Bars(raw15mNext[:199]), 0)
	if wantNextTF15.LastCloseTimeUnix == scoringTF15.LastCloseTimeUnix {
		t.Fatalf("expected distinct LastCloseTimeUnix on next 15m bar")
	}
	nextMarketData, err := market.BuildDataFromPreFetched(
		bless.symbol,
		&market.PreFetchedData{TimeframeKlines: map[string][]market.Kline{"15m": raw15mNext, "5m": raw5m}},
		[]string{"15m", "5m"},
		"15m",
		20,
	)
	if err != nil {
		t.Fatalf("BuildDataFromPreFetched (bar N+1): %v", err)
	}
	nextMarketData.CurrentPrice = pRetest
	ctxNext := &kernel.Context{
		PromptTime:     nextPromptTime,
		CandidateCoins: []kernel.CandidateCoin{cand},
		MarketDataMap:  map[string]*market.Data{bless.symbol: nextMarketData},
	}
	if err := kernel.PrepareHunterV7Tiers(ctxNext, at.strategyEngine); err != nil {
		t.Fatalf("PrepareHunterV7Tiers (bar N+1): %v", err)
	}
	_ = at.strategyEngine.BuildUserPrompt(ctxNext)
	guardNext := &ctxNext.CandidateCoins[0]
	if guardNext.V7ExecutionTier != "EXECUTABLE" || !guardNext.V7ConfirmSummary.PassedReview ||
		!guardNext.V7ConfirmTimeframes["15m"].HasVWAP20 ||
		math.Abs(guardNext.V7ConfirmTimeframes["15m"].VWAP20-wantNextTF15.VWAP20) > 1e-12 {
		t.Fatalf("Regression (a) bar N+1: Fix B must recover VWAP20 on new closed bar under PrimaryCount=20: tier=%q tf15=%+v summary=%+v",
			guardNext.V7ExecutionTier, guardNext.V7ConfirmTimeframes["15m"], guardNext.V7ConfirmSummary)
	}
	if err := at.validateHunterV7ExecutionGuard(ctxNext, decision); err != nil {
		t.Fatalf("Regression (a) bar N+1: expected guard to accept, got %v", err)
	}
}

// TestU5cRegressionCNew15mClosedBarEvaluablyFailingVWAP20StillDemotes verifies
// Regression (c): when a genuinely NEW 15m closed bar (sliced at PrimaryCount=20
// from 200 raw klines, carrying full-history EMA20 and VWAP20) closes strictly
// ABOVE VWAP20, EMA20, and zoneMid, Fix 4 still demotes the previously-passed
// directional_15m_close_short into MissingReview and the guard rejects.
func TestU5cRegressionCNew15mClosedBarEvaluablyFailingVWAP20StillDemotes(t *testing.T) {
	at := u4GuardTrader()
	geom := kernel.HunterV7ExecutionGeometryForConfig(at.config.StrategyConfig)
	bless := u4GuardCases()[0]
	pRetest := bless.zone.Lower + 0.60*(bless.zone.Upper-bless.zone.Lower)

	// Scoring at bar N (15m earlier): directional_15m_close_short passed via VWAP20.
	scoringTime := bless.promptTimeUTC.Add(-15 * time.Minute)
	lastClosedN := bless.zone.Upper * 1.025
	raw15mN := u5cBuild200Raw15mKlines(scoringTime, bless.zone, lastClosedN, pRetest)
	scoringTF15, ok := local.V7TimeframeFactsFromBars("15m", u5cKlinesToV7Bars(raw15mN[:199]), 0)
	if !ok || !scoringTF15.HasVWAP20 || scoringTF15.CloseVsVWAP20Pct > 0 {
		t.Fatalf("scoring bar N precondition failed: %+v", scoringTF15)
	}
	scoringTF5, _ := local.V7TimeframeFactsFromBars("5m", u5cKlinesToV7Bars(raw15mN[:199]), 0)

	sig := &local.V7SignalOutput{
		SignalID:         "sig_" + bless.symbol + "_u5c_reg_c",
		Symbol:           bless.symbol,
		Direction:        local.V7DirShort,
		SetupType:        bless.setupType,
		Status:           local.V7StatusCandidate,
		EntryMode:        local.V7EntryFastConfirm,
		ExecutionQuality: local.V7ExecReady,
		Confidence:       "B",
		RiskLevel:        local.V7RiskLow,
		AIPriority:       bless.aiPriority,
		SetupScore:       90,
		TimingScore:      bless.timingScore,
		RegimeFitScore:   80,
		LiquidityScore:   bless.liquidityScore,
		EntryZone:        bless.zone,
		Invalidation:     local.V7InvalidationRule{Price: bless.invalidation},
		Targets:          []local.V7Target{{Price: bless.tp1}, {Price: bless.tp2}},
		RequiredConfirms: []string{"directional_15m_close_short", "taker_buy_15m_lt_0_48", "live_price_in_entry_zone"},
		ReasonCodes:      append([]string{}, bless.reasonCodes...),
	}
	v7Cfg := local.DefaultV7Config()
	v7Cfg.MinRiskRewardRatio = geom.MinRR
	symCtx := &local.V7SymbolContext{
		Symbol:       bless.symbol,
		CurrentPrice: pRetest,
		TakerBuy15m:  bless.taker15m,
		Snapshot:     &local.SymbolSnapshotData{OIDelta1h: bless.oi1h, OIDelta4h: bless.oi4h},
		ExecutionContext: &local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes:  map[string]local.V7ExecutionTimeframeSummary{"5m": scoringTF5, "15m": scoringTF15},
		},
		ClosedConfirmTimeframes: map[string]local.V7ExecutionTimeframeSummary{"5m": scoringTF5, "15m": scoringTF15},
	}
	summary := local.EvaluateV7Confirmations(sig, symCtx, v7Cfg)
	if !summary.PassedReview || len(summary.MissingReview) != 0 {
		t.Fatalf("scoring bar N must pass: %+v", summary)
	}

	// By promptTimeUTC (bar N+1 closed), the new closed 15m bar closes FAR ABOVE
	// VWAP20, EMA20, and zoneMid -> genuinely fails all three legs of directional_15m_close_short.
	failingCloseN1 := bless.zone.Upper * 1.25
	raw15mN1 := u5cBuild200Raw15mKlines(bless.promptTimeUTC, bless.zone, failingCloseN1, pRetest)
	checkTF15N1, _ := local.V7TimeframeFactsFromBars("15m", u5cKlinesToV7Bars(raw15mN1[:199]), 0)
	if !checkTF15N1.HasVWAP20 || !checkTF15N1.HasEMA20 ||
		checkTF15N1.CloseVsVWAP20Pct <= 0 || checkTF15N1.CloseVsEMA20Pct <= 0 {
		t.Fatalf("bar N+1 must fail both VWAP20 and EMA20 legs: %+v", checkTF15N1)
	}

	promptMarketData, err := market.BuildDataFromPreFetched(
		bless.symbol,
		&market.PreFetchedData{TimeframeKlines: map[string][]market.Kline{"15m": raw15mN1}},
		[]string{"15m"},
		"15m",
		20,
	)
	if err != nil {
		t.Fatalf("BuildDataFromPreFetched: %v", err)
	}
	promptMarketData.CurrentPrice = pRetest

	cand := u4BuildAssembledCandidate(t, bless, geom)
	cand.V7SignalID = sig.SignalID
	cand.V7RequiredConfirms = append([]string{}, sig.RequiredConfirms...)
	cand.V7ConfirmSummary = &summary
	cand.V7ExecutionContext = symCtx.ExecutionContext
	cand.V7ConfirmTimeframes = map[string]local.V7ExecutionTimeframeSummary{"5m": scoringTF5, "15m": scoringTF15}
	cand.V7PriceContext.Last = pRetest
	cand.V7ExecutionTier, cand.V7TierReason = kernel.ClassifyHunterV7CandidateTierForRuntime(cand, geom)
	if cand.V7ExecutionTier != "EXECUTABLE" {
		t.Fatalf("assembly tier = (%q, %q), want EXECUTABLE", cand.V7ExecutionTier, cand.V7TierReason)
	}

	ctx := &kernel.Context{
		PromptTime:     bless.promptTimeUTC,
		CandidateCoins: []kernel.CandidateCoin{cand},
		MarketDataMap:  map[string]*market.Data{bless.symbol: promptMarketData},
	}
	if err := kernel.PrepareHunterV7Tiers(ctx, at.strategyEngine); err != nil {
		t.Fatalf("PrepareHunterV7Tiers: %v", err)
	}
	prompt := at.strategyEngine.BuildUserPrompt(ctx)
	if strings.Contains(prompt, "execution_tier=EXECUTABLE") {
		t.Fatalf("Regression (c): genuinely failing new 15m bar must demote EXECUTABLE:\n%s", prompt)
	}

	guardCoin := &ctx.CandidateCoins[0]
	if guardCoin.V7ExecutionTier != "REVIEWABLE" || guardCoin.V7ConfirmSummary.PassedReview ||
		len(guardCoin.V7ConfirmSummary.MissingReview) != 1 ||
		guardCoin.V7ConfirmSummary.MissingReview[0].Code != "directional_15m_close_short" {
		t.Fatalf("Regression (c): expected demotion of directional_15m_close_short, got tier=%q summary=%+v",
			guardCoin.V7ExecutionTier, guardCoin.V7ConfirmSummary)
	}
	var dirEv *local.V7ConfirmBarEvidence
	for i := range guardCoin.V7ConfirmSummary.BarEvidence {
		if guardCoin.V7ConfirmSummary.BarEvidence[i].Code == "directional_15m_close_short" {
			dirEv = &guardCoin.V7ConfirmSummary.BarEvidence[i]
			break
		}
	}
	if dirEv == nil || dirEv.Passed || dirEv.DecidingTF != "15m" ||
		dirEv.BarClose != failingCloseN1 || dirEv.BarCloseTimeUnix != checkTF15N1.LastCloseTimeUnix {
		t.Fatalf("Regression (c): BarEvidence must reflect failing bar N+1: %+v", dirEv)
	}

	// Execution guard MUST reject even after refresh.
	hunterV7RefreshEvidence{RestConfirmed: true, MicroConfirmed: true}.applyTo(guardCoin)
	decision := &kernel.Decision{
		Symbol:                   bless.symbol,
		Action:                   "open_short",
		Leverage:                 10,
		StopLoss:                 bless.decisionSL,
		TakeProfit:               bless.decisionTP,
		Confidence:               bless.confidence,
		SelectedHunterV7SignalID: sig.SignalID,
		SelectedHunterV7Setup:    string(bless.setupType),
		SelectedHunterV7Tier:     "REVIEWABLE",
	}
	if err := at.validateHunterV7ExecutionGuard(ctx, decision); err == nil ||
		!strings.Contains(err.Error(), "directional_15m_close_short failed (confirmation_missing)") {
		t.Fatalf("Regression (c): expected guard rejection for directional_15m_close_short, got %v", err)
	}
}
