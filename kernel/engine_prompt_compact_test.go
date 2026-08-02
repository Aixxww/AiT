package kernel

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

func TestFormatCompactMarketDataAddsHunterV7ExecutionContext(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Indicators: store.IndicatorConfig{
			EnableEMA:         true,
			EnableRSI:         true,
			EnableATR:         true,
			EnableBOLL:        true,
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
		},
	})

	data := &market.Data{
		Symbol:       "TESTUSDT",
		CurrentPrice: 0.02059,
		OpenInterest: &market.OIData{
			Latest:  811787030,
			Average: 811787030,
		},
		FundingRate: -0.0003171,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"15m": {
				Klines: []market.KlineBar{
					{Open: 0.02090, High: 0.02095, Low: 0.02076, Close: 0.02080, Volume: 900000},
					{Open: 0.02080, High: 0.02088, Low: 0.02070, Close: 0.02075, Volume: 950000},
					{Open: 0.02075, High: 0.02084, Low: 0.02066, Close: 0.02070, Volume: 1000000},
					{Open: 0.02070, High: 0.02082, Low: 0.02062, Close: 0.02068, Volume: 1050000},
					{Open: 0.02068, High: 0.02079, Low: 0.02060, Close: 0.02064, Volume: 1100000},
					{Open: 0.02075, High: 0.02078, Low: 0.02059, Close: 0.02062, Volume: 1250387},
				},
				EMA20Values: []float64{0.020726},
				EMA50Values: []float64{0.020690},
				RSI7Values:  []float64{41.3},
				RSI14Values: []float64{44.9},
				ATR14:       0.000206,
				BOLLUpper:   []float64{0.021211},
				BOLLMiddle:  []float64{0.020799},
				BOLLLower:   []float64{0.020387},
			},
			"5m": {
				Klines: []market.KlineBar{
					{Open: 0.02080, High: 0.02084, Low: 0.02072, Close: 0.02076, Volume: 300000},
					{Open: 0.02076, High: 0.02079, Low: 0.02070, Close: 0.02072, Volume: 320000},
					{Open: 0.02072, High: 0.02076, Low: 0.02066, Close: 0.02068, Volume: 350000},
					{Open: 0.02068, High: 0.02072, Low: 0.02062, Close: 0.02065, Volume: 420000},
					{Open: 0.02065, High: 0.02069, Low: 0.02060, Close: 0.02064, Volume: 500000},
					{Open: 0.02064, High: 0.02067, Low: 0.02059, Close: 0.02061, Volume: 592075},
				},
				EMA20Values: []float64{0.020682},
				EMA50Values: []float64{0.020775},
				RSI7Values:  []float64{37.0},
				RSI14Values: []float64{43.4},
				ATR14:       0.000105,
				BOLLUpper:   []float64{0.020818},
				BOLLMiddle:  []float64{0.020660},
				BOLLLower:   []float64{0.020501},
			},
		},
	}
	coin := &CandidateCoin{
		Symbol:        "TESTUSDT",
		Direction:     "SHORT",
		V7SetupType:   "funding_reversal",
		V7EntryMode:   "wait_price_reversal",
		V7Confidence:  "C",
		V7AIPriority:  55.55,
		V7TimingScore: 72,
		V7RiskScore:   8,
		V7EntryZone: local.V7PriceZone{
			Lower: 0.02054814579968599,
			Upper: 0.020713090333856682,
		},
		V7Invalidation: local.V7InvalidationRule{Price: 0.030595858946843073},
		V7Targets:      []local.V7Target{{Price: 0.018170940354379522, Reason: "funding_reversal_target"}},
		V7DerivativesCtx: &local.V7DerivativesContext{
			OIChange1h:  0.760143689991916,
			OIChange4h:  -0.7364081823665821,
			TakerBuy15m: 0.409006171689245,
		},
	}

	out := engine.formatCompactMarketData(data, coin)
	mustContain := []string{
		"Hunter v7 execution compact:",
		"entry_zone_pos=",
		"zone_location=zone_lower",
		"oi_state=mixed",
		"15m_recent_high3=",
		"15m_min_stop_0_8atr=",
		"15m_vwap20=",
		"15m_close_vs_vwap20=",
		"15m_close_below_vwap20=",
		"15m_no_new_high=true",
		"warning=C_conf_ai_lt_60,short_near_zone_lower,oi_mixed",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Fatalf("compact output missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestFormatCompactMarketDataUsesHunterV7ExecutionContextFallback(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Indicators: store.IndicatorConfig{
			EnableEMA:         true,
			EnableATR:         true,
			EnableBOLL:        true,
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
		},
	})
	data := &market.Data{
		Symbol:       "TESTUSDT",
		CurrentPrice: 100,
		OpenInterest: &market.OIData{
			Latest:  1000,
			Average: 900,
		},
	}
	coin := &CandidateCoin{
		Symbol:             "TESTUSDT",
		Direction:          "LONG",
		V7SetupType:        "range_expansion_event",
		V7EntryMode:        "fast_confirm",
		V7ExecutionQuality: "ready",
		V7Confidence:       "B",
		V7TimingScore:      74,
		V7RiskScore:        18,
		V7EntryZone:        local.V7PriceZone{Lower: 99, Upper: 101},
		V7Invalidation:     local.V7InvalidationRule{Price: 97},
		V7Targets:          []local.V7Target{{Price: 106, Reason: "tp1_median_1.5R_atr_structure"}},
		V7RequiredConfirms: []string{"15m_close_above_vwap_or_ema20_or_entry_zone_upper"},
		V7DerivativesCtx:   &local.V7DerivativesContext{OIChange1h: 2.5, OIChange4h: 4.0, TakerBuy15m: 0.56},
		V7ExecutionContext: &local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes: map[string]local.V7ExecutionTimeframeSummary{
				"15m": {
					Timeframe:        "15m",
					CandleCount:      24,
					LastClose:        100,
					RecentHigh3:      101.2,
					RecentLow3:       98.8,
					HasEMA20:         true,
					CloseVsEMA20Pct:  1.1,
					HasATR:           true,
					ATRPct:           1.4,
					MinStop08ATRPct:  1.12,
					HasVWAP20:        true,
					VWAP20:           99.2,
					CloseVsVWAP20Pct: 0.81,
					NoNewHigh:        false,
					NoNewLow:         true,
					VolumeVsAvg5:     2.2,
				},
				"5m": {
					Timeframe:       "5m",
					CandleCount:     24,
					LastClose:       100,
					RecentHigh3:     100.8,
					RecentLow3:      99.3,
					HasEMA20:        true,
					CloseVsEMA20Pct: 0.7,
					HasATR:          true,
					ATRPct:          0.6,
					MinStop08ATRPct: 0.48,
					NoNewHigh:       true,
					NoNewLow:        true,
					VolumeVsAvg5:    1.7,
				},
			},
		},
	}

	out := engine.formatCompactMarketData(data, coin)
	for _, want := range []string{
		"15m_recent_high3=101.200000",
		"15m_vwap20=99.200000",
		"5m_recent_low3=99.300000",
		"compact_data_quality=complete",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("compact output missing %q\noutput:\n%s", want, out)
		}
	}
	for _, notWant := range []string{"missing_execution=15m_kline", "missing_execution=5m_kline", "missing_execution=15m_vwap", "missing_execution=15m_ema20"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("compact output should not report %q missing\noutput:\n%s", notWant, out)
		}
	}
}

func TestBuildSystemPromptShowsEffectiveHunterV7RiskGeometry(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
		RiskControl: store.RiskControlConfig{
			MaxPositions:                 1,
			BTCETHMaxLeverage:            20,
			AltcoinMaxLeverage:           20,
			BTCETHMaxPositionValueRatio:  10,
			AltcoinMaxPositionValueRatio: 10,
			MaxMarginUsage:               0.7,
			MinPositionSize:              12,
			MinRiskRewardRatio:           1.5,
			MinConfidence:                75,
			MaxEntryPriceDeviationPct:    0.5,
			MaxTakeProfitPriceMovePct:    3.0,
			MinStopLossPriceMovePct:      2.0,
		},
	})

	prompt := engine.BuildSystemPrompt(10.25, "")
	for _, want := range []string{
		"Max Entry Price Drift: ≤0.50%",
		"Min Stop-Loss Distance: ≥2.00%",
		"Max Take-Profit Distance: ≤4.00%",
		"Feasible open geometry: reward distance must be ≥3.00%",
		"calculate effective_take_profit using the backend cap 4.00%",
		"If effective_rr < 1.50",
		"cap take_profit first (long=min(TP, price*(1+",
		"recompute effective_rr from the capped TP and stop_loss; below 1.50 means wait + rr_insufficient",
		"an uncapped far TP never justifies an open",
		"rr/effective_rr >= 1.50, backend RR is validated",
		"do not invent a stricter structural RR",
		"Min Confidence: ≥70 to open position",
		"Confidence below 70 must output wait; do not open by reducing position size.",
		"`confidence`: 0-100 (opening recommended ≥ 70)",
		"Hunter v7 Strategy Execution Framework (Five Segments)",
		"choose between the best open and one precise blocked_reason",
		"no_pullback_still_running at the upper zone",
		"entry_zone_position <=45% and taker>=0.56",
		"a candidate with a complete evidence chain must get either an open or one precise blocked_reason_code",
		"open-after-live-recheck (REVIEWABLE gaps settled by fresh data)",
		"evidence_only/context_only is evidence, not permission",
		"reviewable_only_if_live_confirmed requires a live re-check",
		"high-volatility alts must target the nearest effective TP (TP0/near TP1)",
		"regime_against_direction and passed_review=false waits",
		"Peak>=15% with ~45% giveback",
		"<=-12% anytime is hard invalidation",
		"hold and wait never change SL/TP",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Low confidence (60-69): Use 30-50%") {
		t.Fatalf("prompt should not imply confidence 60-69 may open under Hunter v7 min confidence\n%s", prompt)
	}
}

func TestFormatPositionInfoAddsHunterV7ProtectionState(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
	})
	ctx := &Context{
		MarketDataMap: map[string]*market.Data{
			"TESTUSDT": {
				Symbol:       "TESTUSDT",
				CurrentPrice: 100,
			},
		},
	}

	preTP1 := engine.formatPositionInfo(1, PositionInfo{
		Symbol:           "TESTUSDT",
		Side:             "long",
		EntryPrice:       100,
		MarkPrice:        100.1,
		Quantity:         1,
		UnrealizedPnLPct: 2.0,
		PeakPnLPct:       4.2,
		Leverage:         20,
	}, ctx)
	if !strings.Contains(preTP1, "protection_state=pre_profit_floor") ||
		!strings.Contains(preTP1, "confirmed 5m+15m structural reversal") {
		t.Fatalf("pre-profit-floor position hint missing:\n%s", preTP1)
	}

	breakeven := engine.formatPositionInfo(1, PositionInfo{
		Symbol:           "TESTUSDT",
		Side:             "long",
		EntryPrice:       100,
		MarkPrice:        101.1,
		Quantity:         1,
		UnrealizedPnLPct: 6.0,
		PeakPnLPct:       5.8,
		Leverage:         20,
	}, ctx)
	if !strings.Contains(breakeven, "protection_state=breakeven_floor") ||
		!strings.Contains(breakeven, "do not let a 5%+ peak turn into net loss") {
		t.Fatalf("breakeven-floor position hint missing:\n%s", breakeven)
	}

	microProfit := engine.formatPositionInfo(1, PositionInfo{
		Symbol:           "TESTUSDT",
		Side:             "long",
		EntryPrice:       100,
		MarkPrice:        100.3,
		Quantity:         1,
		UnrealizedPnLPct: 6.0,
		PeakPnLPct:       5.8,
		Leverage:         20,
	}, ctx)
	if !strings.Contains(microProfit, "protection_state=breakeven_floor") ||
		!strings.Contains(microProfit, "raw_move=+0.30%") {
		t.Fatalf("micro-profit position should stay breakeven-floor:\n%s", microProfit)
	}
}

func TestBuildUserPromptUsesHunterV7CandidateTiers(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
		Indicators: store.IndicatorConfig{
			EnableRSI:         true,
			EnableATR:         true,
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
		},
	})
	ctx := &Context{
		CurrentTime:    "2026-06-08 12:00:00",
		RuntimeMinutes: 20,
		CallCount:      84,
		Account: AccountInfo{
			TotalEquity:      1000,
			AvailableBalance: 1000,
		},
		CandidateCoins: []CandidateCoin{
			{
				Symbol:             "READYUSDT",
				Sources:            []string{"hunter"},
				Direction:          "LONG",
				V7SetupType:        "trend_breakout_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       65,
				V7SetupScore:       70,
				V7TimingScore:      65,
				V7RiskScore:        30,
				V7LiquidityScore:   80,
				V7EntryMode:        "direct_breakout",
				V7Confidence:       "B",
				V7EntryZone:        local.V7PriceZone{Lower: 0.99, Upper: 1.02},
				V7Invalidation:     local.V7InvalidationRule{Price: 0.988},
				V7Targets:          []local.V7Target{{Price: 1.08}},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.56},
			},
			{
				Symbol:             "WATCHUSDT",
				Sources:            []string{"hunter"},
				Direction:          "SHORT",
				V7SetupType:        "pre_breakout_watch",
				V7Status:           "wait_confirm",
				V7ExecutionQuality: "watch_only",
				V7AIPriority:       70,
				V7RiskScore:        20,
				V7LiquidityScore:   80,
				V7RiskTags:         []string{"do_not_open_until_confirmed"},
			},
			{
				Symbol:             "BADUSDT",
				Sources:            []string{"hunter"},
				Direction:          "LONG",
				V7SetupType:        "late_chase",
				V7Status:           "filtered",
				V7ExecutionQuality: "invalid_rr",
				V7AIPriority:       90,
				V7RiskScore:        75,
				V7LiquidityScore:   80,
			},
		},
		MarketDataMap: map[string]*market.Data{
			"READYUSDT": hunterV7PromptTestMarketData("READYUSDT", 1.01),
			"WATCHUSDT": hunterV7PromptTestMarketData("WATCHUSDT", 2.02),
			"BADUSDT":   hunterV7PromptTestMarketData("BADUSDT", 3.03),
		},
	}

	prompt := engine.BuildUserPrompt(ctx)
	for _, want := range []string{
		"## Hunter v7 Candidate Tiers (3 total)",
		"Tier Summary: EXECUTABLE=1 | REVIEWABLE=0 | WATCH=1 | REJECTED=1",
		"Tag semantics: reason_codes are evidence, risk_tags constrain or block execution",
		"Decision policy: apply the Hunter v7 five-segment framework",
		"audit every EXECUTABLE first, then REVIEWABLE",
		"output the best open or exactly one blocked_reason_code",
		"### Open-review candidates (full context, max 8)",
		"#### 1. READYUSDT [LONG] (Hunter)",
		"execution_tier=EXECUTABLE tier_reason=long_setup_ready_confirmed",
		"\"execution_tier\":\"EXECUTABLE\"",
		"=== READYUSDT compact market data ===",
		"Hunter v7 execution compact:",
		"### WATCH candidates (summary only; not direct open)",
		"- WATCHUSDT SHORT setup=pre_breakout_watch status=wait_confirm quality=watch_only",
		"### REJECTED candidates (count only)",
		"- status_filtered: 1",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("tiered prompt missing %q\n%s", want, prompt)
		}
	}
	for _, notWant := range []string{
		"#### 2. WATCHUSDT",
		"=== WATCHUSDT compact market data ===",
		"BADUSDT LONG setup=",
		"=== BADUSDT compact market data ===",
	} {
		if strings.Contains(prompt, notWant) {
			t.Fatalf("tiered prompt should not contain %q\n%s", notWant, prompt)
		}
	}
}

func TestBuildUserPromptReclassifiesHunterV7TierAfterLivePriceUpdate(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
	})
	coin := hunterV7PromptReadyLeaderCandidate("LIVEOUTUSDT", 100)
	coin.V7ExecutionTier = "EXECUTABLE"
	coin.V7TierReason = "cached_from_scoring"
	ctx := &Context{
		CandidateCoins: []CandidateCoin{coin},
		MarketDataMap: map[string]*market.Data{
			"LIVEOUTUSDT": hunterV7PromptTestMarketData("LIVEOUTUSDT", 104),
		},
	}

	prompt := engine.BuildUserPrompt(ctx)
	for _, want := range []string{
		"Tier Summary: EXECUTABLE=0 | REVIEWABLE=0 | WATCH=1 | REJECTED=0",
		"- LIVEOUTUSDT LONG setup=leader_momentum_long status=candidate quality=ready ai_priority=73.0 risk=15 reason=backend_rr_infeasible",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "cached_from_scoring") {
		t.Fatalf("prompt used stale cached tier reason:\n%s", prompt)
	}
}

func TestBuildUserPromptCanUpgradeHunterV7TierAfterLivePriceReturnsToWindow(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
	})
	coin := hunterV7PromptReadyLeaderCandidate("LIVEINUSDT", 104)
	coin.V7ExecutionTier = "WATCH"
	coin.V7TierReason = "backend_rr_infeasible"
	ctx := &Context{
		CandidateCoins: []CandidateCoin{coin},
		MarketDataMap: map[string]*market.Data{
			"LIVEINUSDT": hunterV7PromptTestMarketData("LIVEINUSDT", 100),
		},
	}

	prompt := engine.BuildUserPrompt(ctx)
	for _, want := range []string{
		"Tier Summary: EXECUTABLE=1 | REVIEWABLE=0 | WATCH=0 | REJECTED=0",
		"execution_tier=EXECUTABLE tier_reason=momentum_ready_strong_flow",
		"\"execution_readiness\"",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
}

func TestBuildUserPromptDemotesExecutableWhenPromptReadinessMissingExecution(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
	})
	coin := hunterV7PromptReadyLeaderCandidate("MISSKUSDT", 100)
	data := hunterV7PromptTestMarketData("MISSKUSDT", 100)
	delete(data.TimeframeData, "15m")
	delete(data.TimeframeData, "5m")
	ctx := &Context{
		CandidateCoins: []CandidateCoin{coin},
		MarketDataMap:  map[string]*market.Data{"MISSKUSDT": data},
	}

	prompt := engine.BuildUserPrompt(ctx)
	for _, want := range []string{
		"Tier Summary: EXECUTABLE=0 | REVIEWABLE=1 | WATCH=0 | REJECTED=0",
		"execution_tier=REVIEWABLE tier_reason=prompt_readiness_15m_kline_missing",
		"\"missing_execution\":[\"15m_kline\",\"5m_kline\"]",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
}

func TestBuildUserPromptEmitsCompactJSONForOverflowOpenReviewCandidates(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
	})
	ctx := &Context{
		CandidateCoins: make([]CandidateCoin, 0, 9),
		MarketDataMap:  map[string]*market.Data{},
	}
	for i := 0; i < 9; i++ {
		symbol := strings.ToUpper("OR" + string(rune('A'+i)) + "USDT")
		ctx.CandidateCoins = append(ctx.CandidateCoins, hunterV7PromptReadyLeaderCandidate(symbol, 100))
		ctx.MarketDataMap[symbol] = hunterV7PromptTestMarketData(symbol, 100)
	}

	prompt := engine.BuildUserPrompt(ctx)
	if !strings.Contains(prompt, "### Open-review candidates (full context, max 8)") {
		t.Fatalf("prompt missing dynamic limit:\n%s", prompt)
	}
	if strings.Count(prompt, "#### ") != 8 {
		t.Fatalf("expanded count = %d, want 8\n%s", strings.Count(prompt, "#### "), prompt)
	}
	if !strings.Contains(prompt, "compact_execution_json=") || !strings.Contains(prompt, "\"execution_readiness\"") {
		t.Fatalf("overflow candidate should include compact execution JSON:\n%s", prompt)
	}
}

func TestFormatHunterV7ExecutionCompactGradesMissingFields(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{})
	data := hunterV7PromptTestMarketData("PARTIALUSDT", 100)
	data.TimeframeData["15m"].ATR14 = 0
	coin := hunterV7PromptReadyLeaderCandidate("PARTIALUSDT", 100)

	out := engine.formatHunterV7ExecutionCompact(data, &coin)
	if !strings.Contains(out, "missing_context=15m_atr") ||
		!strings.Contains(out, "missing_context_rule=do_not_global_wait_reduce_confidence_or_size") {
		t.Fatalf("compact output missing context-grade missing field:\n%s", out)
	}
	if strings.Contains(out, "missing_fields_rule=wait_unless_all_required_confirmations_are_visible") {
		t.Fatalf("compact output should not use global missing-fields wait rule:\n%s", out)
	}
}

func TestBuildUserPromptCompactsHunterV7PositionMarketData(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
		Indicators: store.IndicatorConfig{
			EnableEMA:         true,
			EnableRSI:         true,
			EnableATR:         true,
			EnableBOLL:        true,
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
		},
	})
	ctx := &Context{
		CurrentTime:    "2026-06-08 12:10:00",
		RuntimeMinutes: 10,
		CallCount:      85,
		Account: AccountInfo{
			TotalEquity:      100,
			AvailableBalance: 80,
			PositionCount:    1,
		},
		Positions: []PositionInfo{
			{
				Symbol:           "OPENUSDT",
				Side:             "long",
				EntryPrice:       0.2077,
				MarkPrice:        0.2071,
				Quantity:         171,
				UnrealizedPnLPct: -3.9,
				UnrealizedPnL:    -0.09,
				PeakPnLPct:       -3.6,
				Leverage:         15,
				MarginUsed:       2.4,
				LiquidationPrice: 0.1563,
				StopLoss:         0.2033,
				TakeProfit:       0.2140,
				UpdateTime:       1780879472271,
			},
		},
		MarketDataMap: map[string]*market.Data{
			"OPENUSDT": hunterV7PromptTestMarketData("OPENUSDT", 0.2071),
		},
	}

	prompt := engine.BuildUserPrompt(ctx)
	for _, want := range []string{
		"position_management_compact",
		"Planned SL 0.2033",
		"Planned TP 0.2140",
		"=== OPENUSDT compact market data ===",
		"5M:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("position prompt missing %q\n%s", want, prompt)
		}
	}
	for _, notWant := range []string{
		"=== OPENUSDT Market Data ===",
		"Time(UTC)",
		"oldest",
	} {
		if strings.Contains(prompt, notWant) {
			t.Fatalf("position prompt should be compact and not contain %q\n%s", notWant, prompt)
		}
	}
}

func TestBuildUserPromptDoesNotExpandHunterV7CandidatesAtPositionLimit(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
		},
		RiskControl: store.RiskControlConfig{
			MaxPositions: 1,
		},
		Indicators: store.IndicatorConfig{
			EnableRSI:         true,
			EnableATR:         true,
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
		},
	})
	ctx := &Context{
		CurrentTime:    "2026-06-08 12:20:00",
		RuntimeMinutes: 20,
		CallCount:      86,
		Account: AccountInfo{
			TotalEquity:      100,
			AvailableBalance: 80,
			PositionCount:    1,
		},
		Positions: []PositionInfo{
			{
				Symbol:    "OPENUSDT",
				Side:      "long",
				MarkPrice: 0.2071,
				Quantity:  171,
				Leverage:  15,
			},
		},
		CandidateCoins: []CandidateCoin{
			{
				Symbol:             "READYUSDT",
				Sources:            []string{"hunter"},
				Direction:          "LONG",
				V7SetupType:        "trend_breakout_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       65,
				V7SetupScore:       70,
				V7TimingScore:      65,
				V7RiskScore:        20,
				V7LiquidityScore:   90,
				V7RiskLevel:        "LOW",
				V7EntryZone:        local.V7PriceZone{Lower: 0.99, Upper: 1.02},
				V7Invalidation:     local.V7InvalidationRule{Price: 0.98},
				V7Targets:          []local.V7Target{{Price: 1.08}},
				V7PriceContext:     &local.V7PriceContext{Last: 1.01},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.56},
			},
		},
		MarketDataMap: map[string]*market.Data{
			"OPENUSDT":  hunterV7PromptTestMarketData("OPENUSDT", 0.2071),
			"READYUSDT": hunterV7PromptTestMarketData("READYUSDT", 1.01),
		},
	}

	prompt := engine.BuildUserPrompt(ctx)
	for _, want := range []string{
		"Current positions have reached Max Positions",
		"### Open-disabled candidate summary",
		"READYUSDT LONG tier=WATCH",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("position-limit prompt missing %q\n%s", want, prompt)
		}
	}
	for _, notWant := range []string{
		"#### 1. READYUSDT",
		"=== READYUSDT compact market data ===",
		"\"symbol\":\"READYUSDT\"",
	} {
		if strings.Contains(prompt, notWant) {
			t.Fatalf("position-limit prompt should not expand candidate %q\n%s", notWant, prompt)
		}
	}
}

func TestFormatHunterV7SignalJSONMarksWatchOnlyAsDoNotOpen(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{})
	coin := CandidateCoin{
		Symbol:             "WATCHUSDT",
		Direction:          "LONG",
		V7SetupType:        "pre_breakout_watch",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7RiskTags:         []string{"pre_move_radar", "do_not_open_until_confirmed"},
		V7ReasonCodes:      []string{"watch_only_no_direct_open"},
		V7RequiredConfirms: []string{"15m_close_above_bb_upper_or_4h_resistance"},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:        true,
			PassedReview:      false,
			EntryZonePosition: 72.5,
			RR:                1.42,
			MissingReview: []local.V7ConfirmationCheck{
				{
					Code:     "taker_buy_15m_gt_0_52",
					Passed:   false,
					Severity: local.V7ConfirmReviewWait,
				},
			},
		},
	}

	// Serializers read the cached verdict, so classify at construction like
	// hunterV7SignalsToCandidateCoins does in production.
	coin.V7ExecutionTier, coin.V7TierReason = classifyHunterV7CandidateTier(coin)

	raw := engine.formatHunterV7SignalJSON(coin)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
	if payload["execution_policy"] != "do_not_open_until_confirmed" {
		t.Fatalf("execution_policy = %v, want do_not_open_until_confirmed", payload["execution_policy"])
	}
	if payload["do_not_open_until_confirmed"] != true {
		t.Fatalf("do_not_open_until_confirmed = %v, want true", payload["do_not_open_until_confirmed"])
	}
	if payload["execution_tier"] != "WATCH" {
		t.Fatalf("execution_tier = %v, want WATCH", payload["execution_tier"])
	}
	if payload["tier_reason"] != "watch_only_confirm_required" {
		t.Fatalf("tier_reason = %v, want watch_only_confirm_required", payload["tier_reason"])
	}
	confirmSummary, ok := payload["confirmation_summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("confirmation_summary missing: %#v", payload["confirmation_summary"])
	}
	if confirmSummary["passed_review"] != false {
		t.Fatalf("passed_review = %v, want false", confirmSummary["passed_review"])
	}
	if confirmSummary["rr"] != 1.42 {
		t.Fatalf("rr = %v, want 1.42", confirmSummary["rr"])
	}
	semantics, ok := payload["tag_semantics"].([]interface{})
	if !ok || len(semantics) == 0 {
		t.Fatalf("tag_semantics missing or empty: %#v", payload["tag_semantics"])
	}
	foundWaitOnly := false
	for _, item := range semantics {
		def, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if def["tag"] == "do_not_open_until_confirmed" && def["llm_action"] == local.V7TagActionWaitOnly {
			foundWaitOnly = true
			break
		}
	}
	if !foundWaitOnly {
		t.Fatalf("tag_semantics missing wait-only definition for do_not_open_until_confirmed: %#v", semantics)
	}
}

func TestFormatHunterV7SignalJSONDefinesP2Tags(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{})
	coin := CandidateCoin{
		Symbol:             "P2USDT",
		Direction:          "LONG",
		V7SetupType:        "volatility_squeeze_breakout",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       72,
		V7SetupScore:       68,
		V7TimingScore:      66,
		V7RiskScore:        25,
		V7ReasonCodes: []string{
			"oi_invisible_accumulation_detected",
			"bb_compressed",
			"volume_burst_at_breakout",
			"taker_buy_ratio_above_0.55",
			"sector_rotation_leader",
			"whale_flow_detected",
			"stealth_accumulation_breakout",
			"accelerating_1h",
			"flow_taker_buy_neutral",
			"no_pullback_still_running",
			"chase_high_protection",
			"low_timing_watch_only",
			"leader_momentum_timing_watch_only",
			"momentum_rsi_overheated_wait",
		},
		V7RiskTags: []string{"correlation_floor_context"},
		V7Readiness: &local.V7ExecutionReadiness{
			Tier:       local.V7ReadinessReviewable,
			Reason:     "near_confirm",
			ReadyScore: 72,
		},
		V7Targets: []local.V7Target{{Price: 1.05, Reason: "tp0"}, {Price: 1.12, Reason: "tp1"}},
	}

	raw := engine.formatHunterV7SignalJSON(coin)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
	semantics, ok := payload["tag_semantics"].([]interface{})
	if !ok || len(semantics) == 0 {
		t.Fatalf("tag_semantics missing or empty: %#v", payload["tag_semantics"])
	}

	wantActions := map[string]string{
		"oi_invisible_accumulation_detected": local.V7TagActionEvidence,
		"bb_compressed":                      local.V7TagActionEvidence,
		"volume_burst_at_breakout":           local.V7TagActionOpenSupport,
		"taker_buy_ratio_above_0.55":         local.V7TagActionEvidence,
		"sector_rotation_leader":             local.V7TagActionEvidence,
		"correlation_floor_context":          local.V7TagActionContextOnly,
		"whale_flow_detected":                local.V7TagActionEvidence,
		"stealth_accumulation_breakout":      local.V7TagActionEvidence,
		"accelerating_1h":                    local.V7TagActionEvidence,
		// The prompt renders the canonical taker vocabulary (U6.3): the coin
		// carries taker_neutral_buy, the payload shows flow_taker_buy_neutral.
		"flow_taker_buy_neutral":            local.V7TagActionEvidence,
		"no_pullback_still_running":         local.V7TagActionWaitOnly,
		"chase_high_protection":             local.V7TagActionWaitOnly,
		"low_timing_watch_only":             local.V7TagActionWaitOnly,
		"leader_momentum_timing_watch_only": local.V7TagActionWaitOnly,
		"momentum_rsi_overheated_wait":      local.V7TagActionWaitOnly,
	}
	found := make(map[string]string)
	for _, item := range semantics {
		def, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := def["tag"].(string)
		action, _ := def["llm_action"].(string)
		if _, want := wantActions[tag]; want {
			found[tag] = action
		}
	}
	for tag, wantAction := range wantActions {
		if got := found[tag]; got != wantAction {
			t.Fatalf("tag %s action = %q, want %q; semantics=%#v", tag, got, wantAction, semantics)
		}
	}
	if payload["execution_readiness"] == nil {
		t.Fatalf("execution_readiness missing: %#v", payload)
	}
	if targets, ok := payload["targets"].([]interface{}); !ok || len(targets) != 2 {
		t.Fatalf("targets missing or truncated: %#v", payload["targets"])
	}
}

func TestHunterV7PromptExposesTP0PlanAndConditionalOpenChecklist(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{})
	data := &market.Data{
		Symbol:        "TP0USDT",
		CurrentPrice:  100,
		TimeframeData: map[string]*market.TimeframeSeriesData{},
	}
	coin := CandidateCoin{
		Symbol:             "TP0USDT",
		Direction:          "LONG",
		V7SetupType:        string(local.V7SetupRangeExpansion),
		V7Status:           "candidate",
		V7ExecutionQuality: "good",
		V7Confidence:       "B",
		V7TimingScore:      72,
		V7RiskScore:        28,
		V7EntryMode:        "zone_retest",
		V7EntryZone:        local.V7PriceZone{Lower: 99, Upper: 101},
		V7Invalidation:     local.V7InvalidationRule{Price: 96},
		V7Targets:          []local.V7Target{{Price: 104, Reason: "TP1"}},
		V7TP0Price:         101.5,
		V7TP0RR:            0.38,
		V7TP1Price:         104,
		V7TP2Price:         108,
		V7TPPlan: &local.V7TakeProfitPlan{
			TP0Price:               101.5,
			TP0DistancePct:         1.5,
			TP0ReducePctMin:        30,
			TP0ReducePctMax:        50,
			MoveStopToBreakeven:    true,
			TrailingBasis:          []string{"5m_ema20", "15m_vwap", "0.8-1.2atr15m"},
			TrailingDistancePctMin: 0.8,
			TrailingDistancePctMax: 1.2,
		},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.56},
		V7RequiredConfirms: []string{"15m close above ema20", "taker buy keeps bid"},
	}

	out := engine.formatHunterV7ExecutionCompact(data, &coin)
	for _, want := range []string{
		"tp0_dist=+1.50%",
		"tp0_reduce=30-50%",
		"tp0_breakeven=true",
		"trailing_stop=5m_ema20_or_15m_vwap_or_0.8-1.2atr15m_0.8-1.2%",
		"missing_execution=15m_kline,5m_kline",
		"conditional_open_if=15m_kline_resolved+5m_kline_resolved+15m close above ema20_visible+taker buy keeps bid_visible+entry_zone_valid+invalidation_and_rr_valid",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("compact output missing %q\n%s", want, out)
		}
	}

	raw := engine.formatHunterV7SignalJSON(coin)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
	if payload["tp0_price"] != 101.5 || payload["tp1_price"] != 104.0 {
		t.Fatalf("missing tp fields: %#v", payload)
	}
	if _, ok := payload["take_profit_plan"].(map[string]interface{}); !ok {
		t.Fatalf("take_profit_plan missing: %#v", payload["take_profit_plan"])
	}
}

func TestHunterV7PromptReadinessKeepsCompleteForExecutionQuality(t *testing.T) {
	data := &market.Data{
		Symbol:        "READYUSDT",
		CurrentPrice:  100,
		TimeframeData: map[string]*market.TimeframeSeriesData{},
	}
	coin := CandidateCoin{
		Symbol:           "READYUSDT",
		Direction:        "LONG",
		V7SetupType:      string(local.V7SetupLeaderMomentumLong),
		V7AIPriority:     75,
		V7SetupScore:     78,
		V7TimingScore:    76,
		V7RegimeFitScore: 70,
		V7LiquidityScore: 80,
		V7RiskScore:      30,
		V7EntryZone:      local.V7PriceZone{Lower: 99, Upper: 101},
		V7Invalidation:   local.V7InvalidationRule{Price: 96},
		V7Targets:        []local.V7Target{{Price: 104, Reason: "TP1"}},
		V7RiskTags:       []string{"regime_against_direction"},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.58},
		V7ExecutionContext: &local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes: map[string]local.V7ExecutionTimeframeSummary{
				"15m": {Timeframe: "15m", CandleCount: 30, HasEMA20: true, HasATR: true},
				"5m":  {Timeframe: "5m", CandleCount: 30, HasEMA20: true, HasATR: true},
			},
		},
		V7ConfirmSummary: &local.V7ConfirmationSummary{PassedReview: true},
	}

	readiness := hunterV7PromptExecutionReadiness(coin, data, "EXECUTABLE", "runtime_executable")
	if readiness.DataQuality != "complete_for_execution" {
		t.Fatalf("data quality = %q, want complete_for_execution", readiness.DataQuality)
	}
	tier, reason := hunterV7TierFromPromptReadiness(coin, "EXECUTABLE", "runtime_executable", readiness)
	if tier != "EXECUTABLE" || reason != "runtime_executable" {
		t.Fatalf("tier=%s reason=%s, want EXECUTABLE runtime_executable", tier, reason)
	}
}

func TestHunterV7PromptSemanticDowngradesLateRangeExpansionShort(t *testing.T) {
	coin := CandidateCoin{
		Symbol:         "DUMPUSDT",
		Direction:      "SHORT",
		V7SetupType:    string(local.V7SetupRangeExpansion),
		V7RiskTags:     []string{"event_chase_risk"},
		V7PriceContext: &local.V7PriceContext{Change24h: -18},
	}
	readiness := local.V7ExecutionReadiness{
		Tier:        local.V7ReadinessExecutable,
		Reason:      "readiness_ready",
		DataQuality: "complete_for_execution",
	}

	tier, reason := hunterV7TierFromPromptReadiness(coin, "EXECUTABLE", "runtime_executable", readiness)
	if tier != "WATCH" || reason != "range_expansion_short_exhaustion_retest_wait" {
		t.Fatalf("tier=%s reason=%s, want WATCH range_expansion_short_exhaustion_retest_wait", tier, reason)
	}
}

func TestHunterV7PromptSemanticDowngradesWhaleFlowWithoutExecutionData(t *testing.T) {
	coin := CandidateCoin{
		Symbol:      "WHALEUSDT",
		Direction:   "LONG",
		V7SetupType: string(local.V7SetupWhaleFlow),
	}
	readiness := local.V7ExecutionReadiness{
		Tier:             local.V7ReadinessExecutable,
		Reason:           "readiness_ready",
		DataQuality:      "partial",
		MissingExecution: []string{"taker_buy_15m"},
	}

	tier, reason := hunterV7TierFromPromptReadiness(coin, "EXECUTABLE", "runtime_executable", readiness)
	if tier != "WATCH" || reason != "whale_flow_execution_data_wait" {
		t.Fatalf("tier=%s reason=%s, want WATCH whale_flow_execution_data_wait", tier, reason)
	}
}

func TestHunterV7ExecutionGeometryRaisesTPCapForHigherConfiguredRR(t *testing.T) {
	geometry := HunterV7EffectiveExecutionGeometry(3.0, 2.0, 0.5, 2.0, true)
	if geometry.MaxTPMovePct != 5.25 {
		t.Fatalf("effective max TP pct = %.2f, want 5.25", geometry.MaxTPMovePct)
	}
}

func TestFormatHunterV7SignalJSONAllowsFundingFallbackReviewableWatchOnly(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{})
	coin := CandidateCoin{
		Symbol:             "FUNDUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7ExecutionTier:    "REVIEWABLE",
		V7TierReason:       "funding_short_reviewable_crowding_reversal",
		V7AIPriority:       52,
		V7RiskScore:        30,
		V7TimingScore:      72,
		V7LiquidityScore:   85,
	}

	raw := engine.formatHunterV7SignalJSON(coin)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
	if payload["execution_policy"] != "reviewable_open_allowed_only_if_live_confirmed" {
		t.Fatalf("execution_policy = %v, want reviewable_open_allowed_only_if_live_confirmed", payload["execution_policy"])
	}
	if payload["do_not_open_until_confirmed"] != nil {
		t.Fatalf("do_not_open_until_confirmed should be omitted for reviewable executable: %v", payload["do_not_open_until_confirmed"])
	}
}

func hunterV7PromptReadyLeaderCandidate(symbol string, price float64) CandidateCoin {
	return CandidateCoin{
		Symbol:             symbol,
		Sources:            []string{"hunter"},
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       73,
		V7SetupScore:       72,
		V7TimingScore:      80,
		V7RegimeFitScore:   75,
		V7RiskScore:        15,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7EntryMode:        "momentum_with_trailing_stop",
		V7Confidence:       "B",
		V7ReasonCodes:      []string{"strong_4h_momentum", "flow_taker_buy_sustained"},
		V7EntryZone:        local.V7PriceZone{Lower: price * 0.99, Upper: price * 1.01},
		V7Invalidation:     local.V7InvalidationRule{Price: price * 0.98},
		V7Targets:          []local.V7Target{{Price: price * 1.05}},
		V7PriceContext:     &local.V7PriceContext{Last: price},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.56, OIChange1h: 2.2, OIChange4h: 6.5},
	}
}

func hunterV7PromptTestMarketData(symbol string, price float64) *market.Data {
	return &market.Data{
		Symbol:       symbol,
		CurrentPrice: price,
		CurrentRSI7:  58,
		OpenInterest: &market.OIData{
			Latest:  1000000,
			Average: 950000,
		},
		FundingRate: 0.0001,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"15m": {
				Klines: []market.KlineBar{
					{Open: price * 0.98, High: price * 0.99, Low: price * 0.97, Close: price * 0.985, Volume: 100000},
					{Open: price * 0.985, High: price * 1.00, Low: price * 0.98, Close: price * 0.995, Volume: 110000},
					{Open: price * 0.995, High: price * 1.01, Low: price * 0.99, Close: price, Volume: 130000},
				},
				EMA20Values: []float64{price * 0.995},
				RSI7Values:  []float64{58},
				RSI14Values: []float64{55},
				ATR14:       price * 0.01,
			},
			"5m": {
				Klines: []market.KlineBar{
					{Open: price * 0.99, High: price * 1.00, Low: price * 0.985, Close: price * 0.995, Volume: 50000},
					{Open: price * 0.995, High: price * 1.005, Low: price * 0.99, Close: price, Volume: 60000},
				},
				RSI7Values:  []float64{57},
				RSI14Values: []float64{54},
				ATR14:       price * 0.005,
			},
		},
	}
}

func TestFormatHunterV7SignalJSONIncludesTP0AndPositionHint(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{})
	coin := CandidateCoin{
		Symbol:             "HEIUSDT",
		Direction:          "LONG",
		V7SetupType:        "mms_trend_ride_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7ExecutionTier:    "EXECUTABLE",
		V7TierReason:       "mms_long_ready_confirmed",
		V7AIPriority:       78,
		V7RiskScore:        16,
		V7TimingScore:      68,
		V7LiquidityScore:   60,
		V7EntryZone:        local.V7PriceZone{Lower: 0.1100, Upper: 0.1108},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.1084},
		V7Targets:          []local.V7Target{{Price: 0.1157, Reason: "mms_trend_ride_continuation_target"}},
		V7PriceContext:     &local.V7PriceContext{Last: 0.1109},
	}

	raw := engine.formatHunterV7SignalJSON(coin)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
	if payload["tp0_price"] == nil || payload["tp0_distance_pct"] == nil {
		t.Fatalf("tp0 fields missing: %s", raw)
	}
	if payload["move_stop_to_breakeven"] != true {
		t.Fatalf("move_stop_to_breakeven = %v, want true", payload["move_stop_to_breakeven"])
	}
	if payload["position_size_hint"] != "normal_if_backend_rr_and_confirmations_pass" {
		t.Fatalf("position_size_hint = %v", payload["position_size_hint"])
	}
}

func TestClassifyHunterV7AltLadderFreshOIAbsentBlocksExecutable(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "AKEUSDT",
		Direction:          "LONG",
		V7SetupType:        "alt_ladder_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       72,
		V7SetupScore:       70,
		V7TimingScore:      66,
		V7RiskScore:        30,
		V7LiquidityScore:   70,
		V7RiskLevel:        "MEDIUM",
		V7EntryZone:        local.V7PriceZone{Lower: 0.00310, Upper: 0.00320},
		V7PriceContext:     &local.V7PriceContext{Last: 0.00316},
		V7ReasonCodes:      []string{"alt_ladder_taker_buy", "alt_ladder_volume_expansion"},
		V7RiskTags:         []string{"fresh_oi_absent"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.57, OIChange1h: -4.37, OIChange4h: -6.02},
	}

	if hunterV7AltLadderLongExecutable(coin) {
		t.Fatal("alt-ladder long with fresh_oi_absent must not be executable")
	}
	if tier, _ := classifyHunterV7CandidateTier(coin); tier == "EXECUTABLE" {
		t.Fatalf("tier = %q, want non-EXECUTABLE for fresh_oi_absent", tier)
	}
}

func TestClassifyHunterV7MMSWeakContinuationBlocksExecutable(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "WIFUSDT",
		Direction:          "LONG",
		V7SetupType:        "mms_trend_ride_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       70,
		V7SetupScore:       68,
		V7TimingScore:      68,
		V7RiskScore:        28,
		V7LiquidityScore:   80,
		V7RiskLevel:        "MEDIUM",
		V7RiskTags:         []string{"mms_weak_continuation_review_only"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.55},
	}

	if !hunterV7MMSLongExecutableChaseBlock(coin) {
		t.Fatal("weak MMS continuation must block the executable path")
	}
	if tier, _ := classifyHunterV7CandidateTier(coin); tier == "EXECUTABLE" {
		t.Fatalf("tier = %q, want non-EXECUTABLE for mms_weak_continuation_review_only", tier)
	}
}

func TestHunterV7OpenReviewExpansionLimitGrowsWithPool(t *testing.T) {
	if got := hunterV7OpenReviewExpansionLimit(10, 0, 5); got != 8 {
		t.Fatalf("small pool limit = %d, want 8", got)
	}
	if got := hunterV7OpenReviewExpansionLimit(24, 0, 5); got != 12 {
		t.Fatalf("large pool limit = %d, want 12", got)
	}
	if got := hunterV7OpenReviewExpansionLimit(24, 2, 5); got != 10 {
		t.Fatalf("large pool with positions = %d, want 10", got)
	}
}

func TestHunterV7SelectExpandedOpenReviewKeepsRouteDiversity(t *testing.T) {
	items := []hunterV7PromptCandidate{
		{Tier: "REVIEWABLE", Coin: CandidateCoin{Symbol: "A", V7SetupType: "trend_breakout_long"}},
		{Tier: "REVIEWABLE", Coin: CandidateCoin{Symbol: "B", V7SetupType: "trend_breakout_long"}},
		{Tier: "EXECUTABLE", Coin: CandidateCoin{Symbol: "C", V7SetupType: "alt_ladder_momentum_long"}},
		{Tier: "REVIEWABLE", Coin: CandidateCoin{Symbol: "D", V7SetupType: "mms_trend_ride_long"}},
	}

	selected := hunterV7SelectExpandedOpenReview(items, 2)

	if !selected[2] {
		t.Fatal("EXECUTABLE candidate must always be expanded")
	}
	if !selected[3] {
		t.Fatalf("route-diversity pass must keep mms_trend_ride_long, selected=%v", selected)
	}
	if selected[1] {
		t.Fatalf("duplicate route should not consume a diversity slot, selected=%v", selected)
	}
}

// ---------------------------------------------------------------------------
// Hunter v7 classifier tier tables (U3.6).
//
// The tables below replace the former one-function-per-fixture classifier
// tests. Each row name is the original test function name (plus the original
// subtest name for tests that were already table-driven), each coin is the
// original fixture rebuilt verbatim with the v7Candidate builder, and the
// assertions mirror the original checks exactly (see v7TierCase).
// Fixture values are real historical market samples - preserve them verbatim.
// ---------------------------------------------------------------------------

func hunterV7PanicReversalTierCases() []v7TierCase {
	return []v7TierCase{
		{
			name: "TestClassifyHunterV7CandidateTierAllowsStrongPanicReversal",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("FLOORUSDT"),
				withQuality("near_confirm"),
				withScores(60, 58, 52, 30, 70),
				withRiskLevel("LOW"),
				withTaker(0.54),
			),
			wantTier: "EXECUTABLE",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsFundingExtremePanicReversalWhenConfirmed",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("SIRENUSDT"),
				withQuality("ready"),
				withScores(76.81, 90, 45, 45, 100),
				withRiskLevel("MEDIUM"),
				withReasons("heavy_capitulation", "oi_declining", "strong_reclaim", "flow_taker_buy_strong", "selling_decelerating", "1h_green_shoot", "rsi_recovering_from_extreme"),
				withRiskTags("funding_extreme", "regime_against_direction", "execution_stop_tightened"),
				withTaker(0.552),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "panic_reversal_ready_core_ok",
		},
		{
			name: "TestClassifyHunterV7CandidateTierKeepsWeakFundingExtremePanicReversalOnWatch",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("WEAKFUNDUSDT"),
				withQuality("ready"),
				withScores(65, 70, 45, 45, 100),
				withRiskLevel("MEDIUM"),
				withReasons("heavy_capitulation", "flow_taker_buy_strong"),
				withRiskTags("funding_extreme", "regime_against_direction"),
				withTaker(0.552),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierDemotesTrendDownKnifeCatchPanicReversal",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("LABUSDT"),
				withQuality("ready"),
				withRegime("trend_down"),
				withScores(75.01, 78, 45, 15, 100),
				withRiskLevel("LOW"),
				withReasons("heavy_capitulation", "oi_declining", "strong_reclaim", "flow_taker_buy_strong", "1h_green_shoot"),
				withRiskTags("regime_against_direction", "execution_stop_tightened"),
				withPriceCtx(&local.V7PriceContext{Change1h: 1.41, Change4h: -1.88, Change24h: -24.39}),
				withTaker(0.575),
			),
			wantTier:   "WATCH",
			wantReason: "panic_reversal_trend_down_structure_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksCounterTrendPanicFailedConfirmation",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("BUSDT"),
				withQuality("ready"),
				withRegime("trend_down"),
				withScores(73.1, 75.6, 55, 15, 65),
				withRiskLevel("LOW"),
				withReasons("moderate_capitulation", "oi_declining", "solid_reclaim", "flow_taker_buy_aggressive", "1h_green_shoot", "rsi_recovering_from_extreme"),
				withRiskTags("regime_against_direction", "execution_stop_tightened"),
				withTaker(0.606),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: false,
					MissingReview: []local.V7ConfirmationCheck{
						{
							Code:     "5m_close_above_ema20_or_entry_zone_mid",
							Passed:   false,
							Severity: local.V7ConfirmReviewWait,
						},
					},
				}),
			),
			wantTier:   "WATCH",
			wantReason: "countertrend_confirmation_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsStrongTrendDownPanicReversalException",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("FLUSHUSDT"),
				withQuality("ready"),
				withRegime("trend_down"),
				withScores(78, 82, 45, 20, 100),
				withRiskLevel("LOW"),
				withReasons("heavy_capitulation", "oi_heavy_flush", "strong_reclaim", "flow_taker_buy_aggressive", "selling_exhaustion", "1h_green_shoot"),
				withRiskTags("regime_against_direction", "execution_stop_tightened"),
				withPriceCtx(&local.V7PriceContext{Change1h: 3.4, Change4h: -0.6, Change24h: -22}),
				withTaker(0.64),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: true,
					RR:           2.1,
				}),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "panic_reversal_ready_core_ok",
		},
		{
			name: "TestClassifyHunterV7CandidateTierDemotesBackendCappedRRInfeasible",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("SKYAIUSDT"),
				withQuality("ready"),
				withScores(77.51, 90, 45, 38, 100),
				withRiskLevel("MEDIUM"),
				withPriceCtx(&local.V7PriceContext{Last: 0.20306}),
				withInvalidation(0.1970486),
				withTargets(
					local.V7Target{Price: 0.20803126111856482},
					local.V7Target{Price: 0.30},
				),
				withReasons("strong_reclaim", "flow_taker_buy_strong"),
				withRiskTags("high_volatility", "crowding_elevated", "regime_against_direction", "execution_stop_tightened"),
				withTaker(0.57),
			),
			wantTier:   "WATCH",
			wantReason: "backend_rr_infeasible",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsHighWinPanicReclaimReview",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("GUAUSDT"),
				withQuality("near_confirm"),
				withScores(57.9, 68.4, 30, 15, 100),
				withRiskLevel("LOW"),
				withConfidence("B"),
				withReasons("moderate_capitulation", "oi_declining", "strong_reclaim", "flow_taker_buy_recovering", "selling_decelerating", "1h_green_shoot"),
				withTaker(0.525),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "panic_reversal_reviewable_high_win_reclaim",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksLowTimingPanicWithoutReclaim",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("LOWPANICUSDT"),
				withQuality("near_confirm"),
				withScores(58, 70, 30, 15, 90),
				withRiskLevel("LOW"),
				withConfidence("B"),
				withReasons("moderate_capitulation", "oi_declining", "flow_taker_buy_recovering"),
				withTaker(0.525),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksLowTimingPanicCoreWithoutConfirmedTaker",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("EPICUSDT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(55.2, 82.8, 40, 30, 90),
				withRiskLevel("LOW"),
				withReasons("deep_capitulation", "oi_declining", "strong_reclaim", "flow_taker_buy_recovering", "low_timing_watch_only"),
				withRiskTags("high_volatility", "regime_against_direction", "execution_stop_tightened"),
				withTaker(0.5101538793398475),
			),
			wantTier:   "WATCH",
			wantReason: "panic_reversal_low_timing_confirmation_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksLowTimingPanicHighWinReclaim",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("EPICUSDT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(58.92, 98.4, 30, 30, 90),
				withRiskLevel("LOW"),
				withConfidence("B"),
				withReasons("deep_capitulation", "oi_declining", "strong_reclaim", "flow_taker_buy_aggressive", "selling_decelerating", "1h_green_shoot", "rsi_recovering_from_extreme", "low_timing_watch_only"),
				withRiskTags("high_volatility", "regime_against_direction", "execution_stop_tightened"),
				withTaker(0.6399579896787987),
			),
			wantTier:   "WATCH",
			wantReason: "panic_reversal_low_timing_confirmation_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsLowTimingPanicImpulseWindow",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("CLOUSDT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(51.24, 79.2, 30, 30, 90),
				withRiskLevel("LOW"),
				withConfidence("B"),
				withReasons("heavy_capitulation", "oi_declining", "strong_reclaim", "flow_taker_buy_aggressive", "rsi_recovering_from_extreme", "low_timing_watch_only"),
				withRiskTags("high_volatility", "regime_against_direction", "execution_stop_tightened"),
				withZone(0.1446415501507268, 0.15090586626472854),
				withPriceCtx(&local.V7PriceContext{Last: 0.14635, Change1h: 6.278506271379712, Change4h: -4.887242477416}),
				withDerivatives(-8.158889326203196, -7.2309014557357765, 0.8935487440522297),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "panic_reversal_reviewable_high_win_reclaim",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsRescuedPanicFloorReview",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("BLESSUSDT"),
				withStatus("wait_confirm"),
				withQuality("near_confirm"),
				withScores(51.2, 70.8, 40, 30, 80),
				withRiskLevel("LOW"),
				withConfidence("B"),
				withReasons("moderate_capitulation", "oi_declining", "flow_taker_buy_strong", "low_timing_watch_only", "reviewable_floor_rescue"),
				withRiskTags("regime_against_direction", "fallback_reviewable_needs_live_confirm"),
				withTaker(0.56),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "panic_reversal_reviewable_floor_live_confirm",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsPanicCapitulationFloorReview",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("CLOUSDT"),
				withStatus("wait_confirm"),
				withQuality("ready"),
				withScores(46.9, 31.5, 45, 15, 75),
				withRiskLevel("LOW"),
				withReasons("deep_capitulation", "strong_reclaim", "flow_taker_buy_strong", "selling_decelerating", "rsi_recovering_from_extreme"),
				withRiskTags("high_volatility"),
				withTaker(0.55),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "panic_reversal_reviewable_capitulation_floor",
		},
		{
			name: "TestClassifyHunterV7CandidateTierDemotesRecentWeakLossPatterns/low score panic reversal like ALLO",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("ALLOUSDT"),
				withQuality("ready"),
				withScores(51.8, 37.5, 45, 15, 100),
				withRiskLevel("LOW"),
				withTaker(0.546),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsStrongPullbackReview",
			coin: v7Candidate("pullback_reversal_long",
				withSymbol("MANTAUSDT"),
				withStatus("wait_confirm"),
				withQuality("near_confirm"),
				withScores(49, 75, 55, 23, 55),
				withRiskLevel("LOW"),
				withTaker(0.51),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "pullback_reviewable_strong_structure",
		},
	}
}

func TestClassifyHunterV7CandidateTierPanicReversalTable(t *testing.T) {
	runHunterV7TierCases(t, hunterV7PanicReversalTierCases())
}

func hunterV7MomentumTierCases() []v7TierCase {
	return []v7TierCase{
		{
			name: "TestClassifyHunterV7CandidateTierDemotesMissingExecutionReadiness",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("DEXEUSDT"),
				withQuality("ready"),
				withScores(85.5, 100, 83, 8, 75),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "strong_4h_momentum", "holding_1h", "oi_healthy_growth", "flow_taker_buy_sustained"),
				withZone(16.25, 16.62),
				withInvalidation(16.18),
				withTargets(local.V7Target{Price: 17.34}, local.V7Target{Price: 17.65}),
				withPriceCtx(&local.V7PriceContext{Last: 16.52}),
				withTaker(0.606),
				withReadiness(&local.V7ExecutionReadiness{
					Tier:             local.V7ReadinessReviewable,
					Reason:           "15m_kline_missing",
					ReadyScore:       78.9,
					MissingExecution: []string{"15m_kline", "5m_kline"},
					BlockedGate:      "confirmation_missing",
				}),
			),
			wantTier:   "WATCH",
			wantReason: "missing_execution_15m_kline",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksWaitOnlyReasonCodes",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("DEXEUSDT"),
				withQuality("ready"),
				withScores(85.5, 100, 83, 8, 75),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "strong_4h_momentum", "holding_1h", "oi_healthy_growth", "flow_taker_buy_sustained", "no_pullback_still_running"),
				withZone(16.25, 16.62),
				withInvalidation(16.18),
				withTargets(local.V7Target{Price: 17.34}, local.V7Target{Price: 17.65}),
				withPriceCtx(&local.V7PriceContext{Last: 16.52}),
				withTaker(0.606),
			),
			// wait-only no-pullback reason must block direct open
			notTiers: []string{"EXECUTABLE"},
		},
		{
			name: "TestClassifyHunterV7CandidateTierKeepsBackendFeasibleMomentumExecutable",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("VELVETUSDT"),
				withQuality("ready"),
				withScores(73.02, 71.2, 88, 15, 100),
				withRiskLevel("LOW"),
				withPriceCtx(&local.V7PriceContext{Last: 0.40365}),
				withInvalidation(0.39476775),
				withTargets(local.V7Target{Price: 0.45405921987980813}),
				withRiskTags("regime_against_direction", "execution_stop_tightened"),
				withTaker(0.666),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "momentum_ready_strong_flow",
		},
		{
			name: "TestClassifyHunterV7CandidateTierDemotesRecentWeakLossPatterns/weak taker momentum like UB",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("UBUSDT"),
				withQuality("ready"),
				withScores(74, 78, 69, 0, 75),
				withRiskLevel("LOW"),
				withReasons("flow_taker_buy_weak"),
				withTaker(0.437),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierKeepsGenericWatchOnlyAsWatch",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("WATCHUSDT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(52, 0, 0, 30, 85),
				withRiskLevel("LOW"),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsCleanMomentumReview",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("BSBUSDT"),
				withQuality("near_confirm"),
				withScores(75.2, 80.4, 63, 8, 100),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "strong_4h_momentum", "holding_1h", "shallow_pullback"),
				withTaker(0.54),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "momentum_reviewable_high_priority_pullback",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsRelativeStrengthMomentumReview",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("BABYUSDT"),
				withQuality("ready"),
				withScores(66.08, 57.6, 86, 15, 90),
				withRiskLevel("LOW"),
				withReasons("solid_24h_momentum", "solid_4h_momentum", "holding_1h", "oi_healthy_growth", "flow_taker_buy_neutral", "no_pullback_still_running", "strong_symbol_regime_override"),
				withRiskTags("regime_against_direction", "execution_stop_tightened"),
				withTaker(0.5004404557567651),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "momentum_reviewable_relative_strength_floor",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsConfirmedRelativeStrengthMomentumReview",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("MAGMAUSDT"),
				withQuality("ready"),
				withScores(64.8, 65.6, 78, 15, 65),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "solid_4h_momentum", "accelerating_1h", "oi_healthy_growth", "flow_taker_buy_neutral", "no_pullback_still_running", "strong_symbol_regime_override"),
				withRiskTags("regime_against_direction", "execution_stop_tightened"),
				withTaker(0.527),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: true,
					RR:           3.08,
				}),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "momentum_reviewable_confirmed_relative_strength",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksDirtyMomentumReview/weak taker",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("WEAKUSDT"),
				withQuality("near_confirm"),
				withScores(75, 82, 63, 8, 100),
				withRiskLevel("LOW"),
				withReasons("holding_1h", "shallow_pullback", "flow_taker_buy_weak"),
				withTaker(0.49),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksDirtyMomentumReview/overheated",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("HOTUSDT"),
				withQuality("near_confirm"),
				withScores(75, 82, 63, 8, 100),
				withRiskLevel("LOW"),
				withReasons("accelerating_1h", "shallow_pullback"),
				withRiskTags("momentum_overheated"),
				withTaker(0.56),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksDirtyMomentumReview/missing taker confirmation",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("MISSUSDT"),
				withQuality("near_confirm"),
				withScores(75, 82, 63, 8, 100),
				withRiskLevel("LOW"),
				withReasons("holding_1h", "shallow_pullback"),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksDirtyMomentumReview/chase risk overheated confirmation failed",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("MAGMAUSDT"),
				withStatus("wait_confirm"),
				withQuality("chase_risk"),
				withScores(52.5, 73.6, 88, 15, 65),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "strong_4h_momentum", "accelerating_1h", "oi_healthy_growth", "flow_taker_buy_sustained", "momentum_rsi_overheated_wait"),
				withRiskTags("momentum_overheated", "execution_stop_tightened"),
				withTaker(0.60),
				withZone(0.4934, 0.5055),
				withPriceCtx(&local.V7PriceContext{Last: 0.50177}),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: false,
					RR:           3.13,
				}),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksLateZoneUpperMomentumPullback",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("SAHARAUSDT"),
				withQuality("ready"),
				withScores(85.48, 94.8, 84, 0, 85),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "solid_4h_momentum", "shallow_pullback_1h", "oi_healthy_growth", "flow_taker_buy_strong", "micro_pullback"),
				withZone(0.03878450709070219, 0.03964455984098668),
				withPriceCtx(&local.V7PriceContext{Last: 0.03941, Change1h: -0.7798742138364724}),
				withTaker(0.5534996827013241),
			),
			wantTier:   "WATCH",
			wantReason: "momentum_late_pullback_zone_upper_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsZoneUpperMomentumWithStrongTaker",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("FLOWUSDT"),
				withQuality("ready"),
				withScores(86, 95, 84, 0, 90),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "solid_4h_momentum", "shallow_pullback_1h", "oi_healthy_growth", "flow_taker_buy_sustained", "micro_pullback"),
				withZone(1.0, 1.1),
				withPriceCtx(&local.V7PriceContext{Last: 1.08, Change1h: -0.4}),
				withTaker(0.61),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "momentum_ready_strong_flow",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsReadyMomentumPriorityFloor",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("DODOXUSDT"),
				withQuality("ready"),
				withScores(77.3, 100, 63, 15, 50),
				withRiskLevel("LOW"),
				withReasons("leader_momentum", "volume_expansion", "flow_taker_buy_aggressive"),
				withTaker(0.54),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "momentum_reviewable_ready_priority_floor",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksStrongMomentumChaseRiskReview",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("DODOXUSDT"),
				withStatus("wait_confirm"),
				withQuality("chase_risk"),
				withScores(48.7, 96, 45, 30, 50),
				withRiskLevel("LOW"),
				withReasons("leader_momentum", "volume_expansion", "flow_taker_buy_aggressive"),
				withTaker(0.54),
			),
			wantTier:   "WATCH",
			wantReason: "chase_risk_wait_reentry",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksFlexibleOverheatedChaseRiskReview",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("XPLUSDT"),
				withStatus("wait_confirm"),
				withQuality("chase_risk"),
				withScores(63.8, 100, 68, 8, 75),
				withRegimeFit(80),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "strong_4h_momentum", "accelerating_1h", "oi_healthy_growth", "flow_taker_buy_neutral", "no_pullback_still_running", "momentum_rsi_overheated_wait"),
				withRiskTags("crowding_elevated", "execution_stop_tightened", "momentum_overheated"),
				withZone(0.07148, 0.07285),
				withInvalidation(0.07103),
				withTargets(local.V7Target{Price: 0.07519}),
				withPriceCtx(&local.V7PriceContext{Last: 0.07248}),
				withDerivatives(8.1, 14.1, 0.512),
				withConfirmSummary(&local.V7ConfirmationSummary{PassedHard: true, PassedReview: false}),
			),
			wantTier:   "WATCH",
			wantReason: "chase_risk_wait_reentry",
		},
		// Since U3.5 the provider's five-vote upper-zone verdict (emitted as the
		// momentum_upper_zone_chase risk tag at signal time) is the single
		// authority; the kernel no longer re-votes a drifted copy at prompt time.
		{
			name: "TestClassifyHunterV7CandidateTierBlocksLeaderMomentumUpperZoneChase/provider verdict tag",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("SKYAIUSDT"),
				withQuality("ready"),
				withScores(85.56, 100, 78, 0, 80),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "solid_4h_momentum", "accelerating_1h", "oi_healthy_growth", "flow_taker_buy_strong", "no_pullback_still_running"),
				withRiskTags("momentum_upper_zone_chase"),
				withZone(0.03549129456896678, 0.036259514536637456),
				withPriceCtx(&local.V7PriceContext{Last: 0.03605, VWAP15m: 0.033929655667319744}),
				withDerivatives(-3.0062172399272358, 0, 0.5845970211937053),
			),
			wantTier:   "WATCH",
			wantReason: "momentum_upper_zone_chase_wait",
		},
		// Without the provider verdict the kernel must not demote on its own -
		// the same coin (upper zone, weak OI, stretched VWAP) now rides on the
		// provider having already voted "no chase risk" at signal time.
		{
			name: "TestClassifyHunterV7CandidateTierBlocksLeaderMomentumUpperZoneChase/no provider tags no kernel re-vote",
			coin: v7Candidate("leader_momentum_long",
				withSymbol("SKYAIUSDT"),
				withQuality("ready"),
				withScores(85.56, 100, 78, 0, 80),
				withRiskLevel("LOW"),
				withReasons("strong_24h_momentum", "solid_4h_momentum", "accelerating_1h", "oi_healthy_growth", "flow_taker_buy_strong", "no_pullback_still_running"),
				withZone(0.03549129456896678, 0.036259514536637456),
				withPriceCtx(&local.V7PriceContext{Last: 0.03605, VWAP15m: 0.033929655667319744}),
				withDerivatives(-3.0062172399272358, 0, 0.5845970211937053),
			),
			notTiers: []string{"WATCH"},
		},
	}
}

func TestClassifyHunterV7CandidateTierMomentumTable(t *testing.T) {
	runHunterV7TierCases(t, hunterV7MomentumTierCases())
}

func hunterV7BreakoutTierCases() []v7TierCase {
	return []v7TierCase{
		{
			name: "TestClassifyHunterV7CandidateTierRequiresPriorityForReady/below executable threshold",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("READYUSDT"),
				withStatus("wait_confirm"),
				withQuality("ready"),
				withScores(59, 62, 60, 15, 90),
				withRiskLevel("LOW"),
				withTaker(0.55),
			),
			wantTier: "REVIEWABLE",
		},
		{
			name: "TestClassifyHunterV7CandidateTierRequiresPriorityForReady/at ready threshold",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("READYUSDT"),
				withStatus("wait_confirm"),
				withQuality("ready"),
				withScores(59, 62, 60, 15, 90),
				withRiskLevel("LOW"),
				withTaker(0.55),
				withAIPriority(60),
			),
			wantTier: "EXECUTABLE",
		},
		{
			name: "TestClassifyHunterV7CandidateTierKeepsSqueezeFeasibleWithExtendedTargets",
			coin: v7Candidate("volatility_squeeze_breakout",
				withSymbol("REUSDT"),
				withQuality("ready"),
				withScores(72.2, 72, 81.4, 30, 100),
				withRiskLevel("LOW"),
				withReasons("volatility_squeeze_detected", "oi_building", "bb_compressed"),
				withZone(0.43779, 0.45726561552300726),
				withInvalidation(0.41831438447699276),
				withTargets(
					local.V7Target{Price: 0.4491756155230073},
					local.V7Target{Price: 0.4686512310460145},
					local.V7Target{Price: 0.507602462092029},
				),
				withPriceCtx(&local.V7PriceContext{Last: 0.4297}),
				withTaker(0.56),
			),
			// squeeze should not be blocked by global backend geometry
			notPairTier:   "WATCH",
			notPairReason: "backend_rr_infeasible",
		},
		{
			name: "TestClassifyHunterV7CandidateTierDemotesRecentWeakLossPatterns/near confirm breakout like OPEN",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("OPENUSDT"),
				withQuality("near_confirm"),
				withScores(51.1, 62, 45, 15, 65),
				withRiskLevel("LOW"),
				withRiskTags("crowding_extreme"),
				withTaker(0.573),
			),
			wantTier: "WATCH",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsConfirmedBreakoutFloorReview",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("DODOXUSDT"),
				withStatus("wait_confirm"),
				withQuality("near_confirm"),
				withScores(45.2, 56, 45, 15, 50),
				withRiskLevel("LOW"),
				withReasons("extreme_compression", "confirmed_breakout", "flow_taker_buy_aggressive", "clear_air_above"),
				withRiskTags("context_only_low_priority"),
				withTaker(0.53),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "breakout_reviewable_confirmed_low_risk_floor",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsLowRiskBreakoutPressureFloor",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("JTOUSDT"),
				withQuality("near_confirm"),
				withScores(46.5, 39.2, 55, 0, 90),
				withRiskLevel("LOW"),
				withReasons("mild_compression", "approaching_breakout", "oi_stable_breakout", "flow_taker_buy_strong", "volume_adequate", "clear_air_above"),
				withTaker(0.56),
				withZone(0.535, 0.545),
				withInvalidation(0.529),
				withTargets(local.V7Target{Price: 0.62}),
				withPriceCtx(&local.V7PriceContext{Last: 0.5398}),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "breakout_reviewable_low_risk_pressure_floor",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsLowRiskBreakoutWithOIConfirmation",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("EDENUSDT"),
				withQuality("near_confirm"),
				withScores(45.2, 47.2, 45, 0, 70),
				withRiskLevel("LOW"),
				withReasons("moderate_compression", "breakout_attempt", "oi_increasing", "flow_taker_buy_strong", "clear_air_above"),
				withTaker(0.56),
				withZone(0.0430, 0.0438),
				withInvalidation(0.0425),
				withTargets(local.V7Target{Price: 0.0475}),
				withPriceCtx(&local.V7PriceContext{Last: 0.04337}),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "breakout_reviewable_low_risk_pressure_floor",
		},
		{
			name: "TestClassifyHunterV7CandidateTierUsesReadinessReviewableFallback",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("EDGEUSDT"),
				withQuality("near_confirm"),
				withScores(46.3, 49.6, 55, 8, 55),
				withRegimeFit(53.6),
				withRiskLevel("LOW"),
				withReasons("extreme_compression", "approaching_breakout", "oi_increasing", "flow_taker_buy_strong", "clear_air_above"),
				withZone(0.40029, 0.40223),
				withInvalidation(0.39495),
				withTargets(local.V7Target{Price: 0.44525}),
				withPriceCtx(&local.V7PriceContext{Last: 0.40030}),
				withDerivatives(0.25, 1.7, 0.590),
				withReadiness(&local.V7ExecutionReadiness{
					Tier:         local.V7ReadinessReviewable,
					Reason:       "readiness_reviewable",
					ReadyScore:   67.9,
					WindowHealth: 90,
					DataQuality:  "complete",
				}),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "readiness_reviewable_readiness_reviewable",
		},
		{
			name: "TestHunterV7LiveConfirmableBreakoutPromotesReviewable",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("ZECUSDT"),
				withQuality("near_confirm"),
				withScores(64, 70, 55, 0, 100),
				withReasons("approaching_breakout", "oi_stable_breakout", "volume_expansion", "clear_air_above"),
				withConfirms("5m_or_15m_close_through_breakout_level"),
				withZone(450, 470),
				withPriceCtx(&local.V7PriceContext{Last: 465}),
				withTaker(0.54),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: false,
					MissingReview: []local.V7ConfirmationCheck{{
						Code:     "5m_or_15m_close_through_breakout_level",
						Severity: local.V7ConfirmReviewWait,
					}},
					EntryZonePosition: 60,
					RR:                3.2,
				}),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "live_reviewable_5m_or_15m_close_through_breakout_level",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsBreakoutTriggerMemoryConfirmedReview",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("SUIUSDT"),
				withQuality("near_confirm"),
				withShape("shape_trend_breakout"),
				withEntrySignal("entry_open_now"),
				withScores(52.57, 86, 25, 20, 100),
				withRiskLevel("LOW"),
				withReasons("trigger_memory_confirmed", "5m_or_15m_close_through_breakout_level"),
				withTaker(0.53),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "breakout_trigger_memory_confirmed_reviewable",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsBreakoutTriggerNearReview",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("BULLAUSDT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withShape("shape_trend_breakout"),
				withEntrySignal("entry_trigger_near"),
				withScores(54.9, 86.4, 25, 0, 100),
				withRiskLevel("LOW"),
				withReasons("shape_trend_breakout", "entry_trigger_near"),
				withTaker(0.53),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "breakout_trigger_near_reviewable",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsFlowConfirmedBreakoutTriggerNearReview",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("NEARUSDT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withShape("shape_trend_breakout"),
				withEntrySignal("entry_trigger_near"),
				withScores(48.7, 76.8, 25, 8, 85),
				withRiskLevel("LOW"),
				withReasons("approaching_breakout", "flow_taker_buy_aggressive", "clear_air_above", "entry_trigger_near"),
				withConfirmSummary(&local.V7ConfirmationSummary{RR: 20.56}),
				withDerivatives(0.3, 0, 0.64),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "breakout_trigger_near_flow_reviewable",
		},
		{
			name: "TestClassifyHunterV7TrendBreakoutStrongFlowUpgradesWatchToReviewable",
			coin: v7Candidate("trend_breakout_long",
				withSymbol("ENAUSDT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(47, 64, 25, 0, 85),
				withRiskLevel("LOW"),
				withReasons("approaching_breakout", "flow_taker_buy_aggressive", "clear_air_above", "low_timing_watch_only"),
				withDerivatives(0.4, 0, 0.58),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "breakout_watch_strong_flow_reviewable",
		},
	}
}

func TestClassifyHunterV7CandidateTierBreakoutTable(t *testing.T) {
	runHunterV7TierCases(t, hunterV7BreakoutTierCases())
}

func hunterV7ShortSideTierCases() []v7TierCase {
	return []v7TierCase{
		{
			name: "TestClassifyHunterV7CandidateTierAllowsConfirmedRangeExpansionDespiteChaseProtection",
			coin: v7Candidate("range_expansion_event",
				withSymbol("PIPPINUSDT"),
				withQuality("ready"),
				withScores(77.6, 90.2, 72, 0, 65),
				withRiskLevel("LOW"),
				withReasons("range_expansion_event", "amplitude_24h_event", "moderate_range_expansion_event", "event_continuation_long", "volume_burst_15m", "flow_taker_buy_aligned", "chase_high_protection"),
				withConfirms("15m_close_above_vwap_or_ema20_or_entry_zone_upper", "taker_buy_15m_gt_0_52", "no_new_low_after_reclaim"),
				withZone(0.0198713612, 0.0201740918),
				withInvalidation(0.0196695408),
				withTargets(local.V7Target{Price: 0.0207609969}, local.V7Target{Price: 0.0214992672}),
				withPriceCtx(&local.V7PriceContext{Last: 0.02009, Change1h: 3.56, Change4h: 3.56, Change24h: 19.928, VWAP15m: 0.0184823005}),
				withTaker(0.573),
				withReadiness(&local.V7ExecutionReadiness{
					Tier:         local.V7ReadinessExecutable,
					Reason:       "readiness_ready",
					ReadyScore:   82.8,
					WindowHealth: 100,
					EntryZonePos: 72.22,
					DataQuality:  "complete",
				}),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:        true,
					PassedReview:      true,
					EntryZonePosition: 72.22,
					StopDistancePct:   2.09,
					RewardPct:         7.01,
					RR:                3.35,
				}),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "range_expansion_ready_confirmed_continuation",
		},
		{
			name: "TestClassifyHunterV7CandidateTierRejectsExtremeVolatilityRangeExpansion",
			coin: v7Candidate("range_expansion_event",
				withSymbol("ALLOUSDT"),
				withQuality("ready"),
				withScores(77.6, 90.2, 72, 0, 65),
				withRiskLevel("LOW"),
				withReasons("range_expansion_event", "event_continuation_long", "volume_burst_15m", "flow_taker_buy_aligned", "chase_high_protection"),
				withRiskTags("extreme_volatility", "execution_stop_tightened"),
				withZone(0.3565729584, 0.3668011699),
				withInvalidation(0.3565729584),
				withTargets(local.V7Target{Price: 0.3844160104}),
				withPriceCtx(&local.V7PriceContext{Last: 0.36396, Change1h: 4.48, Change4h: 4.54, Change24h: 53.402, VWAP15m: 0.3188628103}),
				withTaker(0.5613),
				withReadiness(&local.V7ExecutionReadiness{Tier: local.V7ReadinessReviewable, Reason: "readiness_reviewable", ReadyScore: 80.1, WindowHealth: 100, EntryZonePos: 72.22, DataQuality: "complete"}),
				withConfirmSummary(&local.V7ConfirmationSummary{PassedHard: true, PassedReview: true, EntryZonePosition: 72.22, StopDistancePct: 2.03, RewardPct: 11.87, RR: 5.85}),
			),
			wantTier:   "REJECTED",
			wantReason: "extreme_volatility",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsRangeExpansionShortLiveReviewableSummaryGap",
			coin: v7Candidate("range_expansion_event",
				withSymbol("GUAUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(61.7, 83.6, 67, 15, 100),
				withRiskLevel("LOW"),
				withReasons("range_expansion_event", "amplitude_24h_extreme", "moderate_range_expansion_event", "event_directional_followthrough", "flow_taker_sell_aligned", "range_expansion_continuation", "no_new_high_after_rejection"),
				withRiskTags("range_expansion_low_volume_followthrough", "regime_against_direction", "execution_stop_tightened", "stale_data_risk"),
				withConfirms("15m_close_below_vwap_or_ema20_or_entry_zone_lower", "taker_buy_15m_lt_0_48", "no_new_high_after_rejection", "fresh_micro_confirmed"),
				withZone(0.9700, 1.0300),
				withPriceCtx(&local.V7PriceContext{Last: 0.9950}),
				withTaker(0.44),
				withReadiness(&local.V7ExecutionReadiness{
					Tier:         local.V7ReadinessReviewable,
					Reason:       "readiness_reviewable",
					ReadyScore:   84.2,
					WindowHealth: 100,
					EntryZonePos: 27.8,
					DataQuality:  "complete_for_execution",
				}),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "range_expansion_live_reviewable_short_summary",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksRangeExpansionShortExhaustionLiveReviewable",
			coin: v7Candidate("range_expansion_event",
				withSymbol("TLMUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(65.5, 96.8, 67, 30, 100),
				withReasons("range_expansion_event", "strong_range_expansion_event", "event_breakdown_short", "flow_taker_sell_aligned", "velocity_decelerating"),
				withRiskTags("range_expansion_exhaustion", "micro_reversal_against_signal", "high_volatility", "regime_against_direction", "stale_data_risk"),
				withConfirms("15m_close_below_vwap_or_ema20_or_entry_zone_lower", "taker_buy_15m_lt_0_48", "no_new_high_after_rejection", "fresh_micro_confirmed"),
				withZone(0.9700, 1.0300),
				withPriceCtx(&local.V7PriceContext{Last: 0.9950}),
				withTaker(0.44),
				withReadiness(&local.V7ExecutionReadiness{
					Tier:         local.V7ReadinessReviewable,
					ReadyScore:   83.3,
					WindowHealth: 95,
					EntryZonePos: 27.8,
				}),
			),
			wantTier:   "WATCH",
			wantReason: "confirmation_missing_summary",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsFundingShortFallbackReview",
			coin: v7Candidate("funding_reversal",
				withSymbol("FUNDUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(47.5, 0, 72, 8, 85),
				withRiskLevel("LOW"),
			),
			wantTier: "REVIEWABLE",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksFundingShortTightStop",
			coin: v7Candidate("funding_reversal",
				withSymbol("BANKUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(51.8, 0, 72, 8, 85),
				withRiskLevel("LOW"),
				withPriceCtx(&local.V7PriceContext{Last: 0.0378}),
				withInvalidation(0.03831),
				withTaker(0.434),
			),
			wantTier:   "WATCH",
			wantReason: "funding_reversal_stop_too_tight",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksFundingShortAwayFromRetestZone",
			coin: v7Candidate("funding_reversal",
				withSymbol("ZROUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("near_confirm"),
				withScores(56.6, 54, 72, 15, 65),
				withRiskLevel("LOW"),
				withReasons("elevated_funding", "extreme_long_crowding", "strong_taker_sell_reversal", "wait_zone_retest_required"),
				withRiskTags("crowding_extreme", "execution_stop_tightened", "not_near_short_retest_zone"),
				withPriceCtx(&local.V7PriceContext{Last: 1.85}),
				withInvalidation(1.91),
				withTaker(0.41),
			),
			wantTier:   "WATCH",
			wantReason: "funding_short_retest_zone_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksWeakFundingShortFallback",
			coin: v7Candidate("funding_reversal",
				withSymbol("OPENUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(45.8, 50, 67, 15, 65),
				withRiskLevel("LOW"),
				withRiskTags("crowding_extreme", "context_only_low_priority"),
				withTaker(0.449),
			),
			wantTier:   "WATCH",
			wantReason: "context_only_low_priority",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksWeakFundingShortRetestFlush",
			coin: v7Candidate("funding_reversal",
				withSymbol("DASHUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(50.05, 60, 72, 15, 55),
				withRiskLevel("LOW"),
				withConfidence("C"),
				withReasons("elevated_funding", "extreme_long_crowding", "price_turning_down", "strong_taker_sell_reversal", "wait_zone_retest_required", "funding_short_weak_4h_flush_wait"),
				withRiskTags("crowding_extreme", "execution_stop_tightened", "not_near_short_retest_zone", "weak_4h_oi_flush"),
				withPriceCtx(&local.V7PriceContext{Last: 36.85}),
				withInvalidation(37.587),
				withDerivatives(-0.6504888007843932, -0.0064086522494604115, 0.4100854659647791),
			),
			wantTier:   "WATCH",
			wantReason: "funding_short_weak_4h_flush_retest_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksHighRiskSignalTags/funding late short chase without flush",
			coin: v7Candidate("funding_reversal",
				withSymbol("LATEFUNDUSDT"),
				withDirection("SHORT"),
				withQuality("ready"),
				withScores(68, 72, 75, 20, 90),
				withRiskLevel("LOW"),
				withRiskTags("short_after_fast_drop_without_flush"),
				withTaker(0.39),
			),
			wantTier:   "WATCH",
			wantReason: "funding_reversal_late_chase_no_flush",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksHighRiskSignalTags/exhausted short squeeze long",
			coin: v7Candidate("short_squeeze_long",
				withSymbol("SQUEEZEUSDT"),
				withQuality("ready"),
				withScores(75, 78, 72, 20, 90),
				withRiskLevel("LOW"),
				withRiskTags("already_pumped_24h", "funding_expensive"),
				withTaker(0.64),
			),
			wantTier:   "WATCH",
			wantReason: "short_squeeze_crowded_or_exhausted_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksHighRiskSignalTags/accumulation with sell flow",
			coin: v7Candidate("accumulation_breakout_long",
				withSymbol("ACCUMUSDT"),
				withQuality("ready"),
				withScores(72, 76, 70, 15, 80),
				withRiskLevel("LOW"),
				withRiskTags("taker_sell_during_accumulation"),
				withTaker(0.49),
			),
			wantTier:   "WATCH",
			wantReason: "accumulation_sell_flow_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksHighRiskSignalTags/short reversion away from retest zone",
			coin: v7Candidate("distribution_short",
				withSymbol("DISTUSDT"),
				withDirection("SHORT"),
				withQuality("watch_only"),
				withScores(68, 74, 70, 20, 80),
				withRiskLevel("LOW"),
				withRiskTags("not_near_short_retest_zone"),
			),
			wantTier:   "WATCH",
			wantReason: "short_reversion_retest_zone_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksHighRiskSignalTags/panic long without reclaim",
			coin: v7Candidate("panic_reversal_long",
				withSymbol("NORECLAIMUSDT"),
				withQuality("ready"),
				withScores(65, 66, 60, 20, 80),
				withRiskLevel("LOW"),
				withRiskTags("no_reclaim_signal"),
				withTaker(0.54),
			),
			wantTier:   "WATCH",
			wantReason: "panic_reversal_no_reclaim_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksHighRiskSignalTags/pullback long above reclaim zone",
			coin: v7Candidate("pullback_reversal_long",
				withSymbol("PULLUSDT"),
				withQuality("watch_only"),
				withScores(58, 72, 58, 20, 80),
				withRiskLevel("LOW"),
				withRiskTags("not_near_long_reclaim_zone"),
				withTaker(0.52),
			),
			wantTier:   "WATCH",
			wantReason: "pullback_long_reclaim_zone_wait",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksFundingReversalOIBuilding",
			coin: v7Candidate("funding_reversal",
				withSymbol("BUILDOIUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(52, 0, 72, 8, 90),
				withRiskLevel("LOW"),
				withRiskTags("oi_building_no_flush"),
			),
			wantTier:   "WATCH",
			wantReason: "funding_reversal_oi_building",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsMixedOIFundingShortReview",
			coin: v7Candidate("funding_reversal",
				withSymbol("ROBOUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(60.2, 68, 72, 15, 65),
				withRiskLevel("LOW"),
				withReasons("elevated_funding", "heavy_long_crowding", "price_stalling_after_rally", "oi_mild_buildup", "strong_taker_sell_reversal"),
				withRiskTags("crowding_extreme", "oi_building_no_flush"),
				withDerivatives(2.53, -0.54, 0.377),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "funding_short_reviewable_crowding_reversal",
		},
		{
			name: "TestClassifyHunterV7CandidateTierBlocksPureBuildingFundingShort",
			coin: v7Candidate("funding_reversal",
				withSymbol("BUILDUSDT"),
				withDirection("SHORT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(60, 0, 72, 15, 85),
				withRiskLevel("LOW"),
				withReasons("heavy_long_crowding", "strong_taker_sell_reversal"),
				withRiskTags("oi_building_no_flush"),
				withDerivatives(1.5, 0.3, 0.38),
			),
			wantTier:   "WATCH",
			wantReason: "funding_reversal_oi_building",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsBreakdownMomentumShort",
			coin: v7Candidate("breakdown_momentum_short",
				withSymbol("SKYAIUSDT"),
				withDirection("SHORT"),
				withQuality("ready"),
				withScores(61, 78, 66, 8, 80),
				withRiskLevel("LOW"),
				withReasons("strong_1h_downside_momentum", "below_vwap_breakdown", "heavy_taker_selling", "oi_confirms_new_shorts", "sell_volume_confirmed"),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "short_or_reversion_ready_confirmed",
		},
	}
}

func TestClassifyHunterV7CandidateTierShortSideTable(t *testing.T) {
	runHunterV7TierCases(t, hunterV7ShortSideTierCases())
}

func hunterV7AltLadderTierCases() []v7TierCase {
	return []v7TierCase{
		{
			name: "TestClassifyHunterV7AltLadderLateStageNeedsFreshFlow",
			coin: v7Candidate("alt_ladder_momentum_long",
				withSymbol("NIGHTUSDT"),
				withQuality("ready"),
				withScores(77, 84, 64, 15, 100),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_momentum_long", "alt_ladder_stage_late", "alt_ladder_taker_buy", "alt_ladder_oi_inflow"),
				withRiskTags("alt_ladder_late_chase_risk", "high_volatility"),
				withZone(0.02434, 0.02460),
				withPriceCtx(&local.V7PriceContext{Last: 0.02452}),
				withDerivatives(-0.94, 9.62, 0.590),
			),
			// late-stage alt ladder without fresh flow should not be executable
			notTiers:   []string{"EXECUTABLE"},
			notReasons: []string{"alt_ladder_long_ready_confirmed"},
		},
		{
			name: "TestClassifyHunterV7AltLadderLongRequiresStrongFlowForExecutable/weak taker single oi inflow becomes reviewable",
			coin: v7Candidate("alt_ladder_momentum_long",
				withSymbol("ZBTUSDT"),
				withQuality("ready"),
				withScores(86, 98, 64, 0, 90),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_momentum_long", "alt_ladder_stage_mid", "alt_ladder_oi_inflow"),
				withZone(0.1014, 0.1024),
				withPriceCtx(&local.V7PriceContext{Last: 0.1021}),
				withDerivatives(0.5, -3.0, 0.521),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_long_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7AltLadderLongRequiresStrongFlowForExecutable/stop tightened with weak oi is downgraded",
			coin: v7Candidate("alt_ladder_momentum_long",
				withSymbol("ALLOUSDT"),
				withQuality("ready"),
				withScores(90, 98, 76, 0, 100),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_momentum_long", "alt_ladder_stage_mid", "alt_ladder_taker_buy", "alt_ladder_volume_expansion"),
				withRiskTags("execution_stop_tightened"),
				withZone(0.4498, 0.4528),
				withPriceCtx(&local.V7PriceContext{Last: 0.4518}),
				withDerivatives(-0.69, -1.26, 0.565),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_long_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7AltLadderLongRequiresStrongFlowForExecutable/strong taker and oi stays executable",
			coin: v7Candidate("alt_ladder_momentum_long",
				withSymbol("PHAUSDT"),
				withQuality("ready"),
				withScores(79, 89, 64, 8, 60),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_momentum_long", "alt_ladder_stage_early", "alt_ladder_taker_buy", "alt_ladder_oi_inflow"),
				withZone(0.02435, 0.02455),
				withPriceCtx(&local.V7PriceContext{Last: 0.02449}),
				withDerivatives(-1.98, 6.84, 0.635),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "alt_ladder_long_ready_confirmed",
		},
		{
			name: "TestClassifyHunterV7AltLadderLongRequiresStrongFlowForExecutable/late high volatility without oi inflow is reviewable",
			coin: v7Candidate("alt_ladder_momentum_long",
				withSymbol("SYNUSDT"),
				withQuality("ready"),
				withScores(87, 96, 85, 15, 100),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_momentum_long", "alt_ladder_stage_late", "alt_ladder_taker_buy", "alt_ladder_volume_expansion"),
				withRiskTags("alt_ladder_late_chase_risk", "high_volatility"),
				withZone(0.2303, 0.2329),
				withPriceCtx(&local.V7PriceContext{Last: 0.2321}),
				withDerivatives(-0.56, -7.89, 0.60),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_long_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/alt ladder long ready",
			coin: v7Candidate("alt_ladder_momentum_long",
				withSymbol("ALTAUSDT"),
				withQuality("ready"),
				withScores(66, 68, 64, 12, 75),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_momentum_long", "alt_ladder_stage_early", "alt_ladder_taker_buy", "alt_ladder_oi_inflow"),
				withTaker(0.58),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "alt_ladder_long_ready_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/alt ladder short ready",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTCUSDT"),
				withDirection("SHORT"),
				withQuality("ready"),
				withScores(64, 66, 62, 10, 75),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_early", "alt_ladder_taker_sell", "alt_ladder_new_shorts", "no_new_high_after_rejection"),
				withTaker(0.44),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_short_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/late alt ladder short without close through stays watch",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTEUSDT"),
				withDirection("SHORT"),
				withQuality("near_confirm"),
				withScores(58, 70, 60, 25, 80),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_late", "alt_ladder_new_shorts", "alt_ladder_sell_volume"),
				withTaker(0.47),
			),
			notPairTier:   "REVIEWABLE",
			notPairReason: "alt_ladder_short_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/late alt ladder short close through is reviewable",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTFUSDT"),
				withDirection("SHORT"),
				withQuality("near_confirm"),
				withScores(58, 70, 60, 25, 80),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_late", "alt_ladder_taker_sell", "alt_ladder_new_shorts", "trigger_memory_confirmed", "alt_ladder_multi_cycle_close_through", "no_new_high_after_rejection"),
				withTaker(0.45),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_short_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/early alt ladder short still reviewable with strong sell flow",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTGUSDT"),
				withDirection("SHORT"),
				withQuality("near_confirm"),
				withScores(56, 68, 60, 25, 80),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_early", "alt_ladder_taker_sell", "alt_ladder_sell_volume", "no_new_high_after_rejection"),
				withTaker(0.45),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_short_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/early alt ladder short without rebound failure stays watch",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTRUSDT"),
				withDirection("SHORT"),
				withQuality("near_confirm"),
				withScores(56, 68, 60, 25, 80),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_early", "alt_ladder_taker_sell", "alt_ladder_sell_volume"),
				withTaker(0.45),
			),
			wantTier:   "WATCH",
			wantReason: "needs_confirmation",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/early alt ladder short low taker soft release",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTSUSDT"),
				withDirection("SHORT"),
				withQuality("near_confirm"),
				withScores(56, 68, 60, 25, 80),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_early", "alt_ladder_taker_sell", "alt_ladder_sell_volume"),
				withZone(100, 101),
				withInvalidation(102.1),
				withTaker(0.37),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_short_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/mid alt ladder short new shorts OI soft release",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTOUSDT"),
				withDirection("SHORT"),
				withQuality("near_confirm"),
				withScores(58, 70, 62, 20, 80),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_mid", "alt_ladder_taker_sell", "alt_ladder_new_shorts"),
				withZone(100, 101),
				withInvalidation(102.1),
				withDerivatives(1.4, 0.3, 0.41),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_short_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/alt ladder short missing close trigger also needs rebound failure",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTMUSDT"),
				withDirection("SHORT"),
				withQuality("ready"),
				withScores(64, 66, 62, 10, 75),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_early", "alt_ladder_taker_sell", "alt_ladder_new_shorts"),
				withConfirms("5m_or_15m_close_below_trigger"),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard: true,
					MissingReview: []local.V7ConfirmationCheck{{
						Code:   "5m_or_15m_close_below_trigger",
						Passed: false,
					}},
				}),
				withTaker(0.44),
			),
			wantTier:   "WATCH",
			wantReason: "alt_ladder_short_rebound_pending",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/alt ladder short missing close trigger low taker soft live review",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTLUSDT"),
				withDirection("SHORT"),
				withQuality("ready"),
				withScores(64, 66, 62, 10, 75),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_early", "alt_ladder_taker_sell", "alt_ladder_new_shorts"),
				withConfirms("5m_or_15m_close_below_trigger"),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard: true,
					MissingReview: []local.V7ConfirmationCheck{{
						Code:   "5m_or_15m_close_below_trigger",
						Passed: false,
					}},
				}),
				withZone(100, 101),
				withInvalidation(102.1),
				withPriceCtx(&local.V7PriceContext{Last: 100.5}),
				withTaker(0.37),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "live_reviewable_5m_or_15m_close_below_trigger",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes/alt ladder short strong ready",
			coin: v7Candidate("alt_ladder_breakdown_short",
				withSymbol("ALTDUSDT"),
				withDirection("SHORT"),
				withQuality("ready"),
				withScores(68, 76, 68, 10, 75),
				withRiskLevel("LOW"),
				withReasons("alt_ladder_breakdown_short", "alt_ladder_downshift_mid", "alt_ladder_taker_sell", "alt_ladder_new_shorts", "no_new_high_after_rejection"),
				withTaker(0.42),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "alt_ladder_short_ready_strong_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierKeepsAltLadderExtremeAsWatch",
			coin: v7Candidate("alt_ladder_momentum_long",
				withSymbol("AKEUSDT"),
				withStatus("wait_confirm"),
				withQuality("watch_only"),
				withScores(43, 74, 25, 80, 100),
				withRiskLevel("EXTREME"),
				withReasons("alt_ladder_momentum_long", "alt_ladder_stage_extreme"),
				withRiskTags("alt_ladder_extreme_continuation_watch", "extreme_volatility", "funding_extreme"),
			),
			wantTier:   "WATCH",
			wantReason: "alt_ladder_extreme_continuation_watch",
		},
	}
}

func TestClassifyHunterV7CandidateTierAltLadderTable(t *testing.T) {
	runHunterV7TierCases(t, hunterV7AltLadderTierCases())
}

func hunterV7MMSDisplacementGateTierCases() []v7TierCase {
	return []v7TierCase{
		{
			name: "TestClassifyHunterV7CandidateTierRejectsCatalogRejectOnlyRiskTag",
			coin: v7Candidate("displacement_momentum_long",
				withSymbol("SLXUSDT"),
				withQuality("ready"),
				withScores(80, 85, 70, 15, 90),
				withRiskLevel("LOW"),
				withRiskTags("wash_volume_high"),
				withTaker(0.55),
			),
			wantTier:   "REJECTED",
			wantReason: "wash_volume_high",
		},
		{
			name: "TestClassifyHunterV7CandidateTierDemotesMissingRequiredConfirmation",
			coin: v7Candidate("whale_flow_reversal",
				withSymbol("TACUSDT"),
				withQuality("ready"),
				withScores(85, 90, 82, 0, 100),
				withRiskLevel("LOW"),
				withZone(0.03736, 0.03898),
				withInvalidation(0.03741),
				withTargets(local.V7Target{Price: 0.04419}),
				withPriceCtx(&local.V7PriceContext{Last: 0.038102}),
				withTaker(0.498),
				withConfirms("directional_15m_close_long", "taker_flow_confirms_long"),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: false,
					MissingReview: []local.V7ConfirmationCheck{
						{Code: "taker_flow_confirms_long", Passed: false, Severity: local.V7ConfirmReviewWait},
					},
					RR: 2.0,
				}),
			),
			wantTier:   "WATCH",
			wantReason: "confirmation_missing_taker_flow_confirms_long",
		},
		{
			name: "TestHunterV7LiveConfirmableDoesNotPromoteChaseRisk",
			coin: v7Candidate("displacement_momentum_long",
				withSymbol("TLMUSDT"),
				withQuality("near_confirm"),
				withScores(63, 82, 55, 50, 100),
				withReasons("massive_vol_displacement", "oi_confirms_new_demand", "chase_high_protection"),
				withConfirms("taker_flow_confirms_long"),
				withZone(0.0029, 0.0031),
				withPriceCtx(&local.V7PriceContext{Last: 0.00305}),
				withTaker(0.50),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: false,
					MissingReview: []local.V7ConfirmationCheck{{
						Code:     "taker_flow_confirms_long",
						Severity: local.V7ConfirmReviewWait,
					}},
					EntryZonePosition: 80,
					RR:                4,
				}),
			),
			wantTier:   "WATCH",
			wantReason: "confirmation_missing_taker_flow_confirms_long",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsMMSRoutes/bottom wake reviewable",
			coin: v7Candidate("mms_bottom_wake_long",
				withSymbol("MMSAUSDT"),
				withStatus("wait_confirm"),
				withScores(52, 64, 50, 8, 70),
				withRiskLevel("LOW"),
				withReasons("mms_bottom_wake", "mms_oi_stealth_inflow", "mms_volume_wake"),
				withTaker(0.52),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "mms_bottom_wake_reviewable_breakout_required",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsMMSRoutes/squeeze ready",
			coin: v7Candidate("mms_squeeze_engine_long",
				withSymbol("MMSCUSDT"),
				withQuality("ready"),
				withScores(72, 86, 72, 8, 80),
				withRiskLevel("LOW"),
				withReasons("mms_squeeze_engine", "mms_short_ban_active"),
				withTaker(0.57),
			),
			wantTier:   "EXECUTABLE",
			wantReason: "mms_long_ready_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsNearConfirmWhenMicroConfirmed",
			coin: v7Candidate("custom_near_confirm_setup",
				withSymbol("MICROUSDT"),
				withQuality("near_confirm"),
				withScores(58, 60, 55, 20, 80),
				withRiskLevel("LOW"),
				withTaker(0.54),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: true,
				}),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "near_confirm_reviewable_micro_confirmed",
		},
		{
			name: "TestClassifyHunterV7CandidateTierAllowsRepairedDisplacementOnlyReviewable",
			coin: v7Candidate("displacement_momentum_long",
				withSymbol("DODOXUSDT"),
				withQuality("ready"),
				withEntrySignal("entry_rr_repairable"),
				withScores(77.955, 93.5, 70, 30, 90),
				withRiskLevel("LOW"),
				withRiskTags("displacement_chase_risk_overextended", "displacement_rr_repaired_needs_review", "high_volatility", "funding_elevated", "execution_stop_tightened"),
				withPriceCtx(&local.V7PriceContext{Last: 0.027769}),
				withInvalidation(0.027074775),
				withTargets(local.V7Target{Price: 0.029071}),
				withDerivatives(11.87, 0, 0.53),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "displacement_reviewable_needs_confirm",
		},
		{
			name: "TestClassifyHunterV7CandidateTierDowngradesStopTightenedWithoutStrongFlow",
			coin: v7Candidate("mms_trend_ride_long",
				withSymbol("LABUSDT"),
				withQuality("ready"),
				withScores(79, 90, 68, 8, 90),
				withRiskLevel("LOW"),
				withRiskTags("execution_stop_tightened"),
				withTaker(0.54),
			),
			// stop-tightened weak flow should not be EXECUTABLE
			notTiers:   []string{"EXECUTABLE"},
			notReasons: []string{"mms_long_ready_confirmed"},
		},
		{
			name: "TestClassifyHunterV7CandidateTierKeepsNearConfirmWatchWhenMicroFails",
			coin: v7Candidate("custom_near_confirm_setup",
				withSymbol("MICROFAILUSDT"),
				withQuality("near_confirm"),
				withScores(58, 60, 55, 20, 80),
				withRiskLevel("LOW"),
				withTaker(0.54),
				withConfirmSummary(&local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: false,
				}),
			),
			// want WATCH-class result when micro review failed
			notTiers: []string{"REVIEWABLE", "EXECUTABLE"},
		},
		{
			name: "TestClassifyHunterV7CandidateTierReportsLowLiquidityWhenDisplacementFinalRROK",
			coin: v7Candidate("displacement_momentum_long",
				withSymbol("DATAIPUSDT"),
				withQuality("ready"),
				withScores(65.9, 85.8, 60, 45, 40),
				withRiskLevel("LOW"),
				withRiskTags("displacement_rr_insufficient", "funding_extreme", "low_liquidity"),
				withConfirmSummary(&local.V7ConfirmationSummary{RR: 1.76}),
				withTaker(0.64),
			),
			wantTier:   "REJECTED",
			wantReason: "liquidity_lt_50",
		},
		{
			name: "TestClassifyHunterV7CandidateTierReviewsStrongDisplacementRRInsufficient",
			coin: v7Candidate("displacement_momentum_long",
				withSymbol("SLXUSDT"),
				withQuality("ready"),
				withEntrySignal("entry_open_now"),
				withScores(80, 85, 70, 15, 90),
				withRiskLevel("LOW"),
				withRiskTags("displacement_rr_insufficient"),
				withConfirmSummary(&local.V7ConfirmationSummary{RR: 2.6}),
				withTaker(0.55),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "displacement_reviewable_needs_confirm",
		},
		{
			name: "TestClassifyHunterV7MMSLongDowngradesExtendedVWAPChase",
			coin: v7Candidate("mms_trend_ride_long",
				withSymbol("TRADOORUSDT"),
				withQuality("ready"),
				withScores(89, 100, 68, 0, 100),
				withRiskLevel("LOW"),
				withReasons("mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "flow_taker_buy_strong"),
				withPriceCtx(&local.V7PriceContext{Last: 0.5326, Change24h: 22.24, VWAP15m: 0.5071}),
				withDerivatives(0.58, -1.05, 0.574),
			),
			wantTier:   "REVIEWABLE",
			wantReason: "mms_long_reviewable_confirmed",
		},
		{
			name: "TestClassifyHunterV7MMSLongRejectsWeakReentryOutsideZone",
			coin: v7Candidate("mms_trend_ride_long",
				withSymbol("MYXUSDT"),
				withQuality("ready"),
				withScores(74, 78, 68, 8, 55),
				withRiskLevel("LOW"),
				withReasons("mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "mms_low_volume_retest", "shape_clean_momentum", "entry_open_now"),
				withPriceCtx(&local.V7PriceContext{Last: 0.08341, Change1h: -0.11, Change4h: -0.11, VWAP15m: 0.08075}),
				withZone(0.08279, 0.08332),
				withDerivatives(0.63, -1.35, 0.589),
			),
			// weak reentry outside zone should not be executable
			notTiers:   []string{"EXECUTABLE"},
			notReasons: []string{"mms_long_ready_confirmed"},
		},
		{
			name: "TestClassifyHunterV7WaitOnlyRiskTagBlocksExecutable",
			coin: v7Candidate("mms_trend_ride_long",
				withSymbol("HUSDT"),
				withQuality("ready"),
				withScores(88, 100, 68, 0, 55),
				withRiskLevel("LOW"),
				withRiskTags("crowding_extreme"),
				withReasons("mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "entry_open_now"),
				withZone(0.0618, 0.0624),
				withPriceCtx(&local.V7PriceContext{Last: 0.06213, Change1h: 0.13, Change4h: 0.11}),
				withDerivatives(-0.32, 0.5, 0.517),
			),
			wantTier:   "WATCH",
			wantReason: "crowding_extreme",
		},
	}
}

func TestClassifyHunterV7CandidateTierMMSDisplacementGateTable(t *testing.T) {
	runHunterV7TierCases(t, hunterV7MMSDisplacementGateTierCases())
}
