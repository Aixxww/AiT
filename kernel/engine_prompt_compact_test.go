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
		"treat take_profit as the backend-effective TP",
		"capped effective_rr < 1.50",
		"do not cite an uncapped far TP1",
		"confirmation_summary.rr/effective_rr is already >= 1.50",
		"do not invent a stricter structural RR",
		"Min Confidence: ≥70 to open position",
		"Confidence below 70 must output wait; do not open by reducing position size.",
		"`confidence`: 0-100 (opening recommended ≥ 70)",
		"Hunter v7 Execution Rules",
		"choose the best open or provide one precise blocked_reason",
		"weak upper-zone pullbacks",
		"entry_zone_position is >45%",
		"For EXECUTABLE candidates, every required_confirmation must be explicitly passed",
		"For REVIEWABLE candidates with only live-checkable gaps",
		"context-only",
		"Do not describe required_confirmations as left to LLM/context cross-checking",
		"take_profit must be the nearest effective 5m-30m target",
		"confirmation_summary.passed_review=false",
		"Peak >=15%",
		"PnL <=-12%",
		"do not use `hold` to claim stop tightening",
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
		"Decision policy (strict tier funnel):",
		"one `take_profit` field",
		"nearest effective TP that still passes RR after backend cap",
		"far TP1 that becomes RR-insufficient after capping",
		"confirmation_summary.rr/effective_rr already passes",
		"do not override backend-validated RR",
		"entry_zone_position >45%",
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
			"taker_neutral_buy",
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
		"taker_neutral_buy":                  local.V7TagActionEvidence,
		"no_pullback_still_running":          local.V7TagActionWaitOnly,
		"chase_high_protection":              local.V7TagActionWaitOnly,
		"low_timing_watch_only":              local.V7TagActionWaitOnly,
		"leader_momentum_timing_watch_only":  local.V7TagActionWaitOnly,
		"momentum_rsi_overheated_wait":       local.V7TagActionWaitOnly,
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

func TestClassifyHunterV7CandidateTierAllowsStrongPanicReversal(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "FLOORUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       60,
		V7SetupScore:       58,
		V7TimingScore:      52,
		V7RiskScore:        30,
		V7LiquidityScore:   70,
		V7RiskLevel:        "LOW",
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "EXECUTABLE" {
		t.Fatalf("tier = %q (%s), want EXECUTABLE", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsFundingExtremePanicReversalWhenConfirmed(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "SIRENUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       76.81,
		V7SetupScore:       90,
		V7TimingScore:      45,
		V7RiskScore:        45,
		V7LiquidityScore:   100,
		V7RiskLevel:        "MEDIUM",
		V7ReasonCodes: []string{
			"heavy_capitulation",
			"oi_declining",
			"strong_reclaim",
			"taker_buy_strong",
			"selling_decelerating",
			"1h_green_shoot",
			"rsi_recovering_from_extreme",
		},
		V7RiskTags:       []string{"funding_extreme", "regime_against_direction", "execution_stop_tightened"},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.552},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "EXECUTABLE" || reason != "panic_reversal_ready_core_ok" {
		t.Fatalf("tier = %q (%s), want EXECUTABLE panic_reversal_ready_core_ok", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierKeepsWeakFundingExtremePanicReversalOnWatch(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "WEAKFUNDUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       65,
		V7SetupScore:       70,
		V7TimingScore:      45,
		V7RiskScore:        45,
		V7LiquidityScore:   100,
		V7RiskLevel:        "MEDIUM",
		V7ReasonCodes:      []string{"heavy_capitulation", "taker_buy_strong"},
		V7RiskTags:         []string{"funding_extreme", "regime_against_direction"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.552},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" {
		t.Fatalf("tier = %q (%s), want WATCH", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierDemotesTrendDownKnifeCatchPanicReversal(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "LABUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7MarketRegime:     "trend_down",
		V7AIPriority:       75.01,
		V7SetupScore:       78,
		V7TimingScore:      45,
		V7RiskScore:        15,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"heavy_capitulation", "oi_declining", "strong_reclaim", "taker_buy_strong", "1h_green_shoot"},
		V7RiskTags:         []string{"regime_against_direction", "execution_stop_tightened"},
		V7PriceContext:     &local.V7PriceContext{Change1h: 1.41, Change4h: -1.88, Change24h: -24.39},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.575},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "panic_reversal_trend_down_structure_wait" {
		t.Fatalf("tier = %q (%s), want WATCH panic_reversal_trend_down_structure_wait", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksCounterTrendPanicFailedConfirmation(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "BUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7MarketRegime:     "trend_down",
		V7AIPriority:       73.1,
		V7SetupScore:       75.6,
		V7TimingScore:      55,
		V7RiskScore:        15,
		V7LiquidityScore:   65,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"moderate_capitulation", "oi_declining", "solid_reclaim", "taker_buy_aggressive", "1h_green_shoot", "rsi_recovering_from_extreme"},
		V7RiskTags:         []string{"regime_against_direction", "execution_stop_tightened"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.606},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{
					Code:     "5m_close_above_ema20_or_entry_zone_mid",
					Passed:   false,
					Severity: local.V7ConfirmReviewWait,
				},
			},
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "countertrend_confirmation_wait" {
		t.Fatalf("tier = %q (%s), want WATCH countertrend_confirmation_wait", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsStrongTrendDownPanicReversalException(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "FLUSHUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7MarketRegime:     "trend_down",
		V7AIPriority:       78,
		V7SetupScore:       82,
		V7TimingScore:      45,
		V7RiskScore:        20,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes: []string{
			"heavy_capitulation",
			"oi_heavy_flush",
			"strong_reclaim",
			"taker_buy_aggressive",
			"selling_exhaustion",
			"1h_green_shoot",
		},
		V7RiskTags:       []string{"regime_against_direction", "execution_stop_tightened"},
		V7PriceContext:   &local.V7PriceContext{Change1h: 3.4, Change4h: -0.6, Change24h: -22},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.64},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: true,
			RR:           2.1,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "EXECUTABLE" || reason != "panic_reversal_ready_core_ok" {
		t.Fatalf("tier = %q (%s), want EXECUTABLE panic_reversal_ready_core_ok", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierRequiresPriorityForReady(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "READYUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "ready",
		V7AIPriority:       59,
		V7SetupScore:       62,
		V7TimingScore:      60,
		V7RiskScore:        15,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.55},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE below executable threshold", tier, reason)
	}

	coin.V7AIPriority = 60
	tier, reason = classifyHunterV7CandidateTier(coin)
	if tier != "EXECUTABLE" {
		t.Fatalf("tier = %q (%s), want EXECUTABLE at ready threshold", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierRejectsCatalogRejectOnlyRiskTag(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "SLXUSDT",
		Direction:          "LONG",
		V7SetupType:        "displacement_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       80,
		V7SetupScore:       85,
		V7TimingScore:      70,
		V7RiskScore:        15,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7RiskTags:         []string{"wash_volume_high"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.55},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REJECTED" || reason != "wash_volume_high" {
		t.Fatalf("tier = %q (%s), want REJECTED wash_volume_high", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierDemotesBackendCappedRRInfeasible(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "SKYAIUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       77.51,
		V7SetupScore:       90,
		V7TimingScore:      45,
		V7RiskScore:        38,
		V7LiquidityScore:   100,
		V7RiskLevel:        "MEDIUM",
		V7PriceContext:     &local.V7PriceContext{Last: 0.20306},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.1970486},
		V7Targets: []local.V7Target{
			{Price: 0.20803126111856482},
			{Price: 0.30},
		},
		V7ReasonCodes:    []string{"strong_reclaim", "taker_buy_strong"},
		V7RiskTags:       []string{"high_volatility", "crowding_elevated", "regime_against_direction", "execution_stop_tightened"},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.57},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "backend_rr_infeasible" {
		t.Fatalf("tier = %q (%s), want WATCH backend_rr_infeasible", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierDemotesMissingExecutionReadiness(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "DEXEUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       85.5,
		V7SetupScore:       100,
		V7TimingScore:      83,
		V7RiskScore:        8,
		V7LiquidityScore:   75,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"strong_24h_momentum", "strong_4h_momentum", "holding_1h", "oi_healthy_growth", "taker_sustained_buy"},
		V7EntryZone:        local.V7PriceZone{Lower: 16.25, Upper: 16.62},
		V7Invalidation:     local.V7InvalidationRule{Price: 16.18},
		V7Targets:          []local.V7Target{{Price: 17.34}, {Price: 17.65}},
		V7PriceContext:     &local.V7PriceContext{Last: 16.52},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.606},
		V7Readiness: &local.V7ExecutionReadiness{
			Tier:             local.V7ReadinessReviewable,
			Reason:           "15m_kline_missing",
			ReadyScore:       78.9,
			MissingExecution: []string{"15m_kline", "5m_kline"},
			BlockedGate:      "confirmation_missing",
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "missing_execution_15m_kline" {
		t.Fatalf("tier = %q (%s), want WATCH missing_execution_15m_kline", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierDemotesMissingRequiredConfirmation(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "TACUSDT",
		Direction:          "LONG",
		V7SetupType:        "whale_flow_reversal",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       85,
		V7SetupScore:       90,
		V7TimingScore:      82,
		V7RiskScore:        0,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7EntryZone:        local.V7PriceZone{Lower: 0.03736, Upper: 0.03898},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.03741},
		V7Targets:          []local.V7Target{{Price: 0.04419}},
		V7PriceContext:     &local.V7PriceContext{Last: 0.038102},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.498},
		V7RequiredConfirms: []string{"directional_15m_close_long", "taker_flow_confirms_long"},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{
				{Code: "taker_flow_confirms_long", Passed: false, Severity: local.V7ConfirmReviewWait},
			},
			RR: 2.0,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("tier = %q (%s), want WATCH confirmation_missing_taker_flow_confirms_long", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksWaitOnlyReasonCodes(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "DEXEUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       85.5,
		V7SetupScore:       100,
		V7TimingScore:      83,
		V7RiskScore:        8,
		V7LiquidityScore:   75,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"strong_24h_momentum", "strong_4h_momentum", "holding_1h", "oi_healthy_growth", "taker_sustained_buy", "no_pullback_still_running"},
		V7EntryZone:        local.V7PriceZone{Lower: 16.25, Upper: 16.62},
		V7Invalidation:     local.V7InvalidationRule{Price: 16.18},
		V7Targets:          []local.V7Target{{Price: 17.34}, {Price: 17.65}},
		V7PriceContext:     &local.V7PriceContext{Last: 16.52},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.606},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier == "EXECUTABLE" {
		t.Fatalf("tier = %q (%s), wait-only no-pullback reason must block direct open", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierKeepsSqueezeFeasibleWithExtendedTargets(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "REUSDT",
		Direction:          "LONG",
		V7SetupType:        "volatility_squeeze_breakout",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       72.2,
		V7SetupScore:       72,
		V7TimingScore:      81.4,
		V7RiskScore:        30,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"volatility_squeeze_detected", "oi_building", "bb_compressed"},
		V7EntryZone:        local.V7PriceZone{Lower: 0.43779, Upper: 0.45726561552300726},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.41831438447699276},
		V7Targets: []local.V7Target{
			{Price: 0.4491756155230073},
			{Price: 0.4686512310460145},
			{Price: 0.507602462092029},
		},
		V7PriceContext:   &local.V7PriceContext{Last: 0.4297},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.56},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier == "WATCH" && reason == "backend_rr_infeasible" {
		t.Fatalf("squeeze should not be blocked by global backend geometry: tier=%q reason=%s", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierKeepsBackendFeasibleMomentumExecutable(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "VELVETUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       73.02,
		V7SetupScore:       71.2,
		V7TimingScore:      88,
		V7RiskScore:        15,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7PriceContext:     &local.V7PriceContext{Last: 0.40365},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.39476775},
		V7Targets:          []local.V7Target{{Price: 0.45405921987980813}},
		V7RiskTags:         []string{"regime_against_direction", "execution_stop_tightened"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.666},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "EXECUTABLE" || reason != "momentum_ready_strong_flow" {
		t.Fatalf("tier = %q (%s), want EXECUTABLE momentum_ready_strong_flow", tier, reason)
	}
}

func TestHunterV7ExecutionGeometryRaisesTPCapForHigherConfiguredRR(t *testing.T) {
	geometry := HunterV7EffectiveExecutionGeometry(3.0, 2.0, 0.5, 2.0, true)
	if geometry.MaxTPMovePct != 5.25 {
		t.Fatalf("effective max TP pct = %.2f, want 5.25", geometry.MaxTPMovePct)
	}
}

func TestClassifyHunterV7CandidateTierDemotesRecentWeakLossPatterns(t *testing.T) {
	tests := []struct {
		name string
		coin CandidateCoin
	}{
		{
			name: "low score panic reversal like ALLO",
			coin: CandidateCoin{
				Symbol:             "ALLOUSDT",
				Direction:          "LONG",
				V7SetupType:        "panic_reversal_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       51.8,
				V7SetupScore:       37.5,
				V7TimingScore:      45,
				V7RiskScore:        15,
				V7LiquidityScore:   100,
				V7RiskLevel:        "LOW",
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.546},
			},
		},
		{
			name: "weak taker momentum like UB",
			coin: CandidateCoin{
				Symbol:             "UBUSDT",
				Direction:          "LONG",
				V7SetupType:        "leader_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       74,
				V7SetupScore:       78,
				V7TimingScore:      69,
				V7RiskScore:        0,
				V7LiquidityScore:   75,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"taker_weak_buy"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.437},
			},
		},
		{
			name: "near confirm breakout like OPEN",
			coin: CandidateCoin{
				Symbol:             "OPENUSDT",
				Direction:          "LONG",
				V7SetupType:        "trend_breakout_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "near_confirm",
				V7AIPriority:       51.1,
				V7SetupScore:       62,
				V7TimingScore:      45,
				V7RiskScore:        15,
				V7LiquidityScore:   65,
				V7RiskLevel:        "LOW",
				V7RiskTags:         []string{"crowding_extreme"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.573},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTier(tt.coin)
			if tier != "WATCH" {
				t.Fatalf("tier = %q (%s), want WATCH", tier, reason)
			}
		})
	}
}

func TestClassifyHunterV7CandidateTierKeepsGenericWatchOnlyAsWatch(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "WATCHUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       52,
		V7RiskScore:        30,
		V7LiquidityScore:   85,
		V7RiskLevel:        "LOW",
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" {
		t.Fatalf("tier = %q (%s), want WATCH", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsCleanMomentumReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "BSBUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       75.2,
		V7SetupScore:       80.4,
		V7TimingScore:      63,
		V7RiskScore:        8,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"strong_24h_momentum", "strong_4h_momentum", "holding_1h", "shallow_pullback"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "momentum_reviewable_high_priority_pullback" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE momentum_reviewable_high_priority_pullback", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsRelativeStrengthMomentumReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "BABYUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       66.08,
		V7SetupScore:       57.6,
		V7TimingScore:      86,
		V7RiskScore:        15,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7ReasonCodes: []string{
			"solid_24h_momentum",
			"solid_4h_momentum",
			"holding_1h",
			"oi_healthy_growth",
			"taker_neutral_buy",
			"no_pullback_still_running",
			"strong_symbol_regime_override",
		},
		V7RiskTags:       []string{"regime_against_direction", "execution_stop_tightened"},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.5004404557567651},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "momentum_reviewable_relative_strength_floor" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE momentum_reviewable_relative_strength_floor", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsConfirmedRelativeStrengthMomentumReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "MAGMAUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       64.8,
		V7SetupScore:       65.6,
		V7TimingScore:      78,
		V7RiskScore:        15,
		V7LiquidityScore:   65,
		V7RiskLevel:        "LOW",
		V7ReasonCodes: []string{
			"strong_24h_momentum",
			"solid_4h_momentum",
			"accelerating_1h",
			"oi_healthy_growth",
			"taker_neutral_buy",
			"no_pullback_still_running",
			"strong_symbol_regime_override",
		},
		V7RiskTags:       []string{"regime_against_direction", "execution_stop_tightened"},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.527},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: true,
			RR:           3.08,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "momentum_reviewable_confirmed_relative_strength" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE momentum_reviewable_confirmed_relative_strength", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksDirtyMomentumReview(t *testing.T) {
	tests := []struct {
		name string
		coin CandidateCoin
	}{
		{
			name: "weak taker",
			coin: CandidateCoin{
				Symbol:             "WEAKUSDT",
				Direction:          "LONG",
				V7SetupType:        "leader_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "near_confirm",
				V7AIPriority:       75,
				V7SetupScore:       82,
				V7TimingScore:      63,
				V7RiskScore:        8,
				V7LiquidityScore:   100,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"holding_1h", "shallow_pullback", "taker_weak_buy"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.49},
			},
		},
		{
			name: "overheated",
			coin: CandidateCoin{
				Symbol:             "HOTUSDT",
				Direction:          "LONG",
				V7SetupType:        "leader_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "near_confirm",
				V7AIPriority:       75,
				V7SetupScore:       82,
				V7TimingScore:      63,
				V7RiskScore:        8,
				V7LiquidityScore:   100,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"accelerating_1h", "shallow_pullback"},
				V7RiskTags:         []string{"momentum_overheated"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.56},
			},
		},
		{
			name: "missing taker confirmation",
			coin: CandidateCoin{
				Symbol:             "MISSUSDT",
				Direction:          "LONG",
				V7SetupType:        "leader_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "near_confirm",
				V7AIPriority:       75,
				V7SetupScore:       82,
				V7TimingScore:      63,
				V7RiskScore:        8,
				V7LiquidityScore:   100,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"holding_1h", "shallow_pullback"},
			},
		},
		{
			name: "chase risk overheated confirmation failed",
			coin: CandidateCoin{
				Symbol:             "MAGMAUSDT",
				Direction:          "LONG",
				V7SetupType:        "leader_momentum_long",
				V7Status:           "wait_confirm",
				V7ExecutionQuality: "chase_risk",
				V7AIPriority:       52.5,
				V7SetupScore:       73.6,
				V7TimingScore:      88,
				V7RiskScore:        15,
				V7LiquidityScore:   65,
				V7RiskLevel:        "LOW",
				V7ReasonCodes: []string{
					"strong_24h_momentum",
					"strong_4h_momentum",
					"accelerating_1h",
					"oi_healthy_growth",
					"taker_sustained_buy",
					"momentum_rsi_overheated_wait",
				},
				V7RiskTags:       []string{"momentum_overheated", "execution_stop_tightened"},
				V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.60},
				V7EntryZone:      local.V7PriceZone{Lower: 0.4934, Upper: 0.5055},
				V7PriceContext:   &local.V7PriceContext{Last: 0.50177},
				V7ConfirmSummary: &local.V7ConfirmationSummary{
					PassedHard:   true,
					PassedReview: false,
					RR:           3.13,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTier(tt.coin)
			if tier != "WATCH" {
				t.Fatalf("tier = %q (%s), want WATCH", tier, reason)
			}
		})
	}
}

func TestClassifyHunterV7CandidateTierBlocksLateZoneUpperMomentumPullback(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "SAHARAUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       85.48,
		V7SetupScore:       94.8,
		V7TimingScore:      84,
		V7RiskScore:        0,
		V7LiquidityScore:   85,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"strong_24h_momentum", "solid_4h_momentum", "shallow_pullback_1h", "oi_healthy_growth", "taker_strong_buy", "micro_pullback"},
		V7EntryZone:        local.V7PriceZone{Lower: 0.03878450709070219, Upper: 0.03964455984098668},
		V7PriceContext:     &local.V7PriceContext{Last: 0.03941, Change1h: -0.7798742138364724},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.5534996827013241},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "momentum_late_pullback_zone_upper_wait" {
		t.Fatalf("tier = %q (%s), want WATCH momentum_late_pullback_zone_upper_wait", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsZoneUpperMomentumWithStrongTaker(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "FLOWUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       86,
		V7SetupScore:       95,
		V7TimingScore:      84,
		V7RiskScore:        0,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"strong_24h_momentum", "solid_4h_momentum", "shallow_pullback_1h", "oi_healthy_growth", "taker_sustained_buy", "micro_pullback"},
		V7EntryZone:        local.V7PriceZone{Lower: 1.0, Upper: 1.1},
		V7PriceContext:     &local.V7PriceContext{Last: 1.08, Change1h: -0.4},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.61},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "EXECUTABLE" || reason != "momentum_ready_strong_flow" {
		t.Fatalf("tier = %q (%s), want EXECUTABLE momentum_ready_strong_flow", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsHighWinPanicReclaimReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "GUAUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       57.9,
		V7SetupScore:       68.4,
		V7TimingScore:      30,
		V7RiskScore:        15,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7Confidence:       "B",
		V7ReasonCodes: []string{
			"moderate_capitulation",
			"oi_declining",
			"strong_reclaim",
			"taker_buy_recovering",
			"selling_decelerating",
			"1h_green_shoot",
		},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.525},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "panic_reversal_reviewable_high_win_reclaim" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE panic_reversal_reviewable_high_win_reclaim", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksLowTimingPanicWithoutReclaim(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "LOWPANICUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       58,
		V7SetupScore:       70,
		V7TimingScore:      30,
		V7RiskScore:        15,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7Confidence:       "B",
		V7ReasonCodes:      []string{"moderate_capitulation", "oi_declining", "taker_buy_recovering"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.525},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" {
		t.Fatalf("tier = %q (%s), want WATCH", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksLowTimingPanicCoreWithoutConfirmedTaker(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "EPICUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       55.2,
		V7SetupScore:       82.8,
		V7TimingScore:      40,
		V7RiskScore:        30,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"deep_capitulation", "oi_declining", "strong_reclaim", "taker_buy_recovering", "low_timing_watch_only"},
		V7RiskTags:         []string{"high_volatility", "regime_against_direction", "execution_stop_tightened"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.5101538793398475},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "panic_reversal_low_timing_confirmation_wait" {
		t.Fatalf("tier = %q (%s), want WATCH panic_reversal_low_timing_confirmation_wait", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksLowTimingPanicHighWinReclaim(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "EPICUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       58.92,
		V7SetupScore:       98.4,
		V7TimingScore:      30,
		V7RiskScore:        30,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7Confidence:       "B",
		V7ReasonCodes: []string{
			"deep_capitulation",
			"oi_declining",
			"strong_reclaim",
			"taker_buy_aggressive",
			"selling_decelerating",
			"1h_green_shoot",
			"rsi_recovering_from_extreme",
			"low_timing_watch_only",
		},
		V7RiskTags:       []string{"high_volatility", "regime_against_direction", "execution_stop_tightened"},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.6399579896787987},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "panic_reversal_low_timing_confirmation_wait" {
		t.Fatalf("tier = %q (%s), want WATCH panic_reversal_low_timing_confirmation_wait", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsLowTimingPanicImpulseWindow(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "CLOUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       51.24,
		V7SetupScore:       79.2,
		V7TimingScore:      30,
		V7RiskScore:        30,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7Confidence:       "B",
		V7ReasonCodes: []string{
			"heavy_capitulation",
			"oi_declining",
			"strong_reclaim",
			"taker_buy_aggressive",
			"rsi_recovering_from_extreme",
			"low_timing_watch_only",
		},
		V7RiskTags:       []string{"high_volatility", "regime_against_direction", "execution_stop_tightened"},
		V7EntryZone:      local.V7PriceZone{Lower: 0.1446415501507268, Upper: 0.15090586626472854},
		V7PriceContext:   &local.V7PriceContext{Last: 0.14635, Change1h: 6.278506271379712, Change4h: -4.887242477416},
		V7DerivativesCtx: &local.V7DerivativesContext{OIChange1h: -8.158889326203196, OIChange4h: -7.2309014557357765, TakerBuy15m: 0.8935487440522297},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "panic_reversal_reviewable_high_win_reclaim" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE panic_reversal_reviewable_high_win_reclaim", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsRescuedPanicFloorReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "BLESSUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       51.2,
		V7SetupScore:       70.8,
		V7TimingScore:      40,
		V7RiskScore:        30,
		V7LiquidityScore:   80,
		V7RiskLevel:        "LOW",
		V7Confidence:       "B",
		V7ReasonCodes: []string{
			"moderate_capitulation",
			"oi_declining",
			"taker_buy_strong",
			"low_timing_watch_only",
			"reviewable_floor_rescue",
		},
		V7RiskTags:       []string{"regime_against_direction", "fallback_reviewable_needs_live_confirm"},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.56},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "panic_reversal_reviewable_floor_live_confirm" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE panic_reversal_reviewable_floor_live_confirm", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsStrongPullbackReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "MANTAUSDT",
		Direction:          "LONG",
		V7SetupType:        "pullback_reversal_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       49,
		V7SetupScore:       75,
		V7TimingScore:      55,
		V7RiskScore:        23,
		V7LiquidityScore:   55,
		V7RiskLevel:        "LOW",
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.51},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "pullback_reviewable_strong_structure" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE pullback_reviewable_strong_structure", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsConfirmedBreakoutFloorReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "DODOXUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       45.2,
		V7SetupScore:       56,
		V7TimingScore:      45,
		V7RiskScore:        15,
		V7LiquidityScore:   50,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"extreme_compression", "confirmed_breakout", "taker_aggressive_buy", "clear_air_above"},
		V7RiskTags:         []string{"context_only_low_priority"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.53},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "breakout_reviewable_confirmed_low_risk_floor" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE breakout_reviewable_confirmed_low_risk_floor", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsLowRiskBreakoutPressureFloor(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "JTOUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       46.5,
		V7SetupScore:       39.2,
		V7TimingScore:      55,
		V7RiskScore:        0,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"mild_compression", "approaching_breakout", "oi_stable_breakout", "taker_strong_buy", "volume_adequate", "clear_air_above"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.56},
		V7EntryZone:        local.V7PriceZone{Lower: 0.535, Upper: 0.545},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.529},
		V7Targets:          []local.V7Target{{Price: 0.62}},
		V7PriceContext:     &local.V7PriceContext{Last: 0.5398},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "breakout_reviewable_low_risk_pressure_floor" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE breakout_reviewable_low_risk_pressure_floor", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsLowRiskBreakoutWithOIConfirmation(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "EDENUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       45.2,
		V7SetupScore:       47.2,
		V7TimingScore:      45,
		V7RiskScore:        0,
		V7LiquidityScore:   70,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"moderate_compression", "breakout_attempt", "oi_increasing", "taker_strong_buy", "clear_air_above"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.56},
		V7EntryZone:        local.V7PriceZone{Lower: 0.0430, Upper: 0.0438},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.0425},
		V7Targets:          []local.V7Target{{Price: 0.0475}},
		V7PriceContext:     &local.V7PriceContext{Last: 0.04337},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "breakout_reviewable_low_risk_pressure_floor" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE breakout_reviewable_low_risk_pressure_floor", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsPanicCapitulationFloorReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "CLOUSDT",
		Direction:          "LONG",
		V7SetupType:        "panic_reversal_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "ready",
		V7AIPriority:       46.9,
		V7SetupScore:       31.5,
		V7TimingScore:      45,
		V7RiskScore:        15,
		V7LiquidityScore:   75,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"deep_capitulation", "strong_reclaim", "taker_buy_strong", "selling_decelerating", "rsi_recovering_from_extreme"},
		V7RiskTags:         []string{"high_volatility"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.55},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "panic_reversal_reviewable_capitulation_floor" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE panic_reversal_reviewable_capitulation_floor", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsReadyMomentumPriorityFloor(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "DODOXUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       77.3,
		V7SetupScore:       100,
		V7TimingScore:      63,
		V7RiskScore:        15,
		V7LiquidityScore:   50,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"leader_momentum", "volume_expansion", "taker_aggressive_buy"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "momentum_reviewable_ready_priority_floor" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE momentum_reviewable_ready_priority_floor", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksStrongMomentumChaseRiskReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "DODOXUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "chase_risk",
		V7AIPriority:       48.7,
		V7SetupScore:       96,
		V7TimingScore:      45,
		V7RiskScore:        30,
		V7LiquidityScore:   50,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"leader_momentum", "volume_expansion", "taker_aggressive_buy"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "chase_risk_wait_reentry" {
		t.Fatalf("tier = %q (%s), want WATCH chase_risk_wait_reentry", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksFlexibleOverheatedChaseRiskReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "XPLUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "chase_risk",
		V7AIPriority:       63.8,
		V7SetupScore:       100,
		V7TimingScore:      68,
		V7RegimeFitScore:   80,
		V7RiskScore:        8,
		V7LiquidityScore:   75,
		V7RiskLevel:        "LOW",
		V7ReasonCodes: []string{
			"strong_24h_momentum",
			"strong_4h_momentum",
			"accelerating_1h",
			"oi_healthy_growth",
			"taker_neutral_buy",
			"no_pullback_still_running",
			"momentum_rsi_overheated_wait",
		},
		V7RiskTags:       []string{"crowding_elevated", "execution_stop_tightened", "momentum_overheated"},
		V7EntryZone:      local.V7PriceZone{Lower: 0.07148, Upper: 0.07285},
		V7Invalidation:   local.V7InvalidationRule{Price: 0.07103},
		V7Targets:        []local.V7Target{{Price: 0.07519}},
		V7PriceContext:   &local.V7PriceContext{Last: 0.07248},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.512, OIChange1h: 8.1, OIChange4h: 14.1},
		V7ConfirmSummary: &local.V7ConfirmationSummary{PassedHard: true, PassedReview: false},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "chase_risk_wait_reentry" {
		t.Fatalf("tier = %q (%s), want WATCH chase_risk_wait_reentry", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierUsesReadinessReviewableFallback(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "EDGEUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       46.3,
		V7SetupScore:       49.6,
		V7TimingScore:      55,
		V7RegimeFitScore:   53.6,
		V7RiskScore:        8,
		V7LiquidityScore:   55,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"extreme_compression", "approaching_breakout", "oi_increasing", "taker_strong_buy", "clear_air_above"},
		V7EntryZone:        local.V7PriceZone{Lower: 0.40029, Upper: 0.40223},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.39495},
		V7Targets:          []local.V7Target{{Price: 0.44525}},
		V7PriceContext:     &local.V7PriceContext{Last: 0.40030},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.590, OIChange1h: 0.25, OIChange4h: 1.7},
		V7Readiness: &local.V7ExecutionReadiness{
			Tier:         local.V7ReadinessReviewable,
			Reason:       "readiness_reviewable",
			ReadyScore:   67.9,
			WindowHealth: 90,
			DataQuality:  "complete",
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "readiness_reviewable_readiness_reviewable" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE readiness_reviewable_readiness_reviewable", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsConfirmedRangeExpansionDespiteChaseProtection(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "PIPPINUSDT",
		Direction:          "LONG",
		V7SetupType:        "range_expansion_event",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       77.6,
		V7SetupScore:       90.2,
		V7TimingScore:      72,
		V7RiskScore:        0,
		V7LiquidityScore:   65,
		V7RiskLevel:        "LOW",
		V7ReasonCodes: []string{
			"range_expansion_event",
			"amplitude_24h_event",
			"moderate_range_expansion_event",
			"event_continuation_long",
			"volume_burst_15m",
			"taker_buy_aligned",
			"chase_high_protection",
		},
		V7RequiredConfirms: []string{
			"15m_close_above_vwap_or_ema20_or_entry_zone_upper",
			"taker_buy_15m_gt_0_52",
			"no_new_low_after_reclaim",
		},
		V7EntryZone:      local.V7PriceZone{Lower: 0.0198713612, Upper: 0.0201740918},
		V7Invalidation:   local.V7InvalidationRule{Price: 0.0196695408},
		V7Targets:        []local.V7Target{{Price: 0.0207609969}, {Price: 0.0214992672}},
		V7PriceContext:   &local.V7PriceContext{Last: 0.02009, Change1h: 3.56, Change4h: 3.56, Change24h: 19.928, VWAP15m: 0.0184823005},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.573},
		V7Readiness: &local.V7ExecutionReadiness{
			Tier:         local.V7ReadinessExecutable,
			Reason:       "readiness_ready",
			ReadyScore:   82.8,
			WindowHealth: 100,
			EntryZonePos: 72.22,
			DataQuality:  "complete",
		},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:        true,
			PassedReview:      true,
			EntryZonePosition: 72.22,
			StopDistancePct:   2.09,
			RewardPct:         7.01,
			RR:                3.35,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "EXECUTABLE" || reason != "range_expansion_ready_confirmed_continuation" {
		t.Fatalf("tier = %q (%s), want EXECUTABLE range_expansion_ready_confirmed_continuation", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierRejectsExtremeVolatilityRangeExpansion(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "ALLOUSDT",
		Direction:          "LONG",
		V7SetupType:        "range_expansion_event",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       77.6,
		V7SetupScore:       90.2,
		V7TimingScore:      72,
		V7RiskScore:        0,
		V7LiquidityScore:   65,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"range_expansion_event", "event_continuation_long", "volume_burst_15m", "taker_buy_aligned", "chase_high_protection"},
		V7RiskTags:         []string{"extreme_volatility", "execution_stop_tightened"},
		V7EntryZone:        local.V7PriceZone{Lower: 0.3565729584, Upper: 0.3668011699},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.3565729584},
		V7Targets:          []local.V7Target{{Price: 0.3844160104}},
		V7PriceContext:     &local.V7PriceContext{Last: 0.36396, Change1h: 4.48, Change4h: 4.54, Change24h: 53.402, VWAP15m: 0.3188628103},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.5613},
		V7Readiness:        &local.V7ExecutionReadiness{Tier: local.V7ReadinessReviewable, Reason: "readiness_reviewable", ReadyScore: 80.1, WindowHealth: 100, EntryZonePos: 72.22, DataQuality: "complete"},
		V7ConfirmSummary:   &local.V7ConfirmationSummary{PassedHard: true, PassedReview: true, EntryZonePosition: 72.22, StopDistancePct: 2.03, RewardPct: 11.87, RR: 5.85},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REJECTED" || reason != "extreme_volatility" {
		t.Fatalf("tier = %q (%s), want REJECTED extreme_volatility", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsRangeExpansionShortLiveReviewableSummaryGap(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "GUAUSDT",
		Direction:          "SHORT",
		V7SetupType:        "range_expansion_event",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       61.7,
		V7SetupScore:       83.6,
		V7TimingScore:      67,
		V7RiskScore:        15,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes: []string{
			"range_expansion_event",
			"amplitude_24h_extreme",
			"moderate_range_expansion_event",
			"event_directional_followthrough",
			"taker_sell_aligned",
			"range_expansion_continuation",
		},
		V7RiskTags: []string{
			"range_expansion_low_volume_followthrough",
			"regime_against_direction",
			"execution_stop_tightened",
			"stale_data_risk",
		},
		V7RequiredConfirms: []string{
			"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
			"taker_buy_15m_lt_0_48",
			"no_new_high_after_rejection",
			"fresh_micro_confirmed",
		},
		V7EntryZone:      local.V7PriceZone{Lower: 0.9700, Upper: 1.0300},
		V7PriceContext:   &local.V7PriceContext{Last: 0.9950},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.44},
		V7Readiness: &local.V7ExecutionReadiness{
			Tier:         local.V7ReadinessReviewable,
			Reason:       "readiness_reviewable",
			ReadyScore:   84.2,
			WindowHealth: 100,
			EntryZonePos: 27.8,
			DataQuality:  "complete_for_execution",
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "range_expansion_live_reviewable_short_summary" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE range_expansion_live_reviewable_short_summary", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksRangeExpansionShortExhaustionLiveReviewable(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "TLMUSDT",
		Direction:          "SHORT",
		V7SetupType:        "range_expansion_event",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       65.5,
		V7SetupScore:       96.8,
		V7TimingScore:      67,
		V7RiskScore:        30,
		V7LiquidityScore:   100,
		V7ReasonCodes: []string{
			"range_expansion_event",
			"strong_range_expansion_event",
			"event_breakdown_short",
			"taker_sell_aligned",
			"velocity_decelerating",
		},
		V7RiskTags: []string{
			"range_expansion_exhaustion",
			"micro_reversal_against_signal",
			"high_volatility",
			"regime_against_direction",
			"stale_data_risk",
		},
		V7RequiredConfirms: []string{
			"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
			"taker_buy_15m_lt_0_48",
			"no_new_high_after_rejection",
			"fresh_micro_confirmed",
		},
		V7EntryZone:      local.V7PriceZone{Lower: 0.9700, Upper: 1.0300},
		V7PriceContext:   &local.V7PriceContext{Last: 0.9950},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.44},
		V7Readiness: &local.V7ExecutionReadiness{
			Tier:         local.V7ReadinessReviewable,
			ReadyScore:   83.3,
			WindowHealth: 95,
			EntryZonePos: 27.8,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "confirmation_missing_summary" {
		t.Fatalf("tier = %q (%s), want WATCH confirmation_missing_summary", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsFundingShortFallbackReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "FUNDUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       47.5,
		V7TimingScore:      72,
		V7RiskScore:        8,
		V7LiquidityScore:   85,
		V7RiskLevel:        "LOW",
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksFundingShortTightStop(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "BANKUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       51.8,
		V7TimingScore:      72,
		V7RiskScore:        8,
		V7LiquidityScore:   85,
		V7RiskLevel:        "LOW",
		V7PriceContext:     &local.V7PriceContext{Last: 0.0378},
		V7Invalidation:     local.V7InvalidationRule{Price: 0.03831},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.434},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "funding_reversal_stop_too_tight" {
		t.Fatalf("tier = %q (%s), want WATCH funding_reversal_stop_too_tight", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksFundingShortAwayFromRetestZone(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "ZROUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       56.6,
		V7SetupScore:       54,
		V7TimingScore:      72,
		V7RiskScore:        15,
		V7LiquidityScore:   65,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"elevated_funding", "extreme_long_crowding", "strong_taker_sell_reversal", "wait_zone_retest_required"},
		V7RiskTags:         []string{"crowding_extreme", "execution_stop_tightened", "not_near_short_retest_zone"},
		V7PriceContext:     &local.V7PriceContext{Last: 1.85},
		V7Invalidation:     local.V7InvalidationRule{Price: 1.91},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.41},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "funding_short_retest_zone_wait" {
		t.Fatalf("tier = %q (%s), want WATCH funding_short_retest_zone_wait", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksWeakFundingShortFallback(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "OPENUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       45.8,
		V7SetupScore:       50,
		V7TimingScore:      67,
		V7RiskScore:        15,
		V7LiquidityScore:   65,
		V7RiskLevel:        "LOW",
		V7RiskTags:         []string{"crowding_extreme", "context_only_low_priority"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.449},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "context_only_low_priority" {
		t.Fatalf("tier = %q (%s), want WATCH context_only_low_priority", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksWeakFundingShortRetestFlush(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "DASHUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       50.05,
		V7SetupScore:       60,
		V7TimingScore:      72,
		V7RiskScore:        15,
		V7LiquidityScore:   55,
		V7RiskLevel:        "LOW",
		V7Confidence:       "C",
		V7ReasonCodes:      []string{"elevated_funding", "extreme_long_crowding", "price_turning_down", "strong_taker_sell_reversal", "wait_zone_retest_required", "funding_short_weak_4h_flush_wait"},
		V7RiskTags:         []string{"crowding_extreme", "execution_stop_tightened", "not_near_short_retest_zone", "weak_4h_oi_flush"},
		V7PriceContext:     &local.V7PriceContext{Last: 36.85},
		V7Invalidation:     local.V7InvalidationRule{Price: 37.587},
		V7DerivativesCtx: &local.V7DerivativesContext{
			OIChange1h:  -0.6504888007843932,
			OIChange4h:  -0.0064086522494604115,
			TakerBuy15m: 0.4100854659647791,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "funding_short_weak_4h_flush_retest_wait" {
		t.Fatalf("tier = %q (%s), want WATCH funding_short_weak_4h_flush_retest_wait", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksHighRiskSignalTags(t *testing.T) {
	tests := []struct {
		name       string
		coin       CandidateCoin
		wantReason string
	}{
		{
			name: "funding late short chase without flush",
			coin: CandidateCoin{
				Symbol:             "LATEFUNDUSDT",
				Direction:          "SHORT",
				V7SetupType:        "funding_reversal",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       68,
				V7SetupScore:       72,
				V7TimingScore:      75,
				V7RiskScore:        20,
				V7LiquidityScore:   90,
				V7RiskLevel:        "LOW",
				V7RiskTags:         []string{"short_after_fast_drop_without_flush"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.39},
			},
			wantReason: "funding_reversal_late_chase_no_flush",
		},
		{
			name: "exhausted short squeeze long",
			coin: CandidateCoin{
				Symbol:             "SQUEEZEUSDT",
				Direction:          "LONG",
				V7SetupType:        "short_squeeze_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       75,
				V7SetupScore:       78,
				V7TimingScore:      72,
				V7RiskScore:        20,
				V7LiquidityScore:   90,
				V7RiskLevel:        "LOW",
				V7RiskTags:         []string{"already_pumped_24h", "funding_expensive"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.64},
			},
			wantReason: "short_squeeze_crowded_or_exhausted_wait",
		},
		{
			name: "accumulation with sell flow",
			coin: CandidateCoin{
				Symbol:             "ACCUMUSDT",
				Direction:          "LONG",
				V7SetupType:        "accumulation_breakout_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       72,
				V7SetupScore:       76,
				V7TimingScore:      70,
				V7RiskScore:        15,
				V7LiquidityScore:   80,
				V7RiskLevel:        "LOW",
				V7RiskTags:         []string{"taker_sell_during_accumulation"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.49},
			},
			wantReason: "accumulation_sell_flow_wait",
		},
		{
			name: "short reversion away from retest zone",
			coin: CandidateCoin{
				Symbol:             "DISTUSDT",
				Direction:          "SHORT",
				V7SetupType:        "distribution_short",
				V7Status:           "candidate",
				V7ExecutionQuality: "watch_only",
				V7AIPriority:       68,
				V7SetupScore:       74,
				V7TimingScore:      70,
				V7RiskScore:        20,
				V7LiquidityScore:   80,
				V7RiskLevel:        "LOW",
				V7RiskTags:         []string{"not_near_short_retest_zone"},
			},
			wantReason: "short_reversion_retest_zone_wait",
		},
		{
			name: "panic long without reclaim",
			coin: CandidateCoin{
				Symbol:             "NORECLAIMUSDT",
				Direction:          "LONG",
				V7SetupType:        "panic_reversal_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       65,
				V7SetupScore:       66,
				V7TimingScore:      60,
				V7RiskScore:        20,
				V7LiquidityScore:   80,
				V7RiskLevel:        "LOW",
				V7RiskTags:         []string{"no_reclaim_signal"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
			},
			wantReason: "panic_reversal_no_reclaim_wait",
		},
		{
			name: "pullback long above reclaim zone",
			coin: CandidateCoin{
				Symbol:             "PULLUSDT",
				Direction:          "LONG",
				V7SetupType:        "pullback_reversal_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "watch_only",
				V7AIPriority:       58,
				V7SetupScore:       72,
				V7TimingScore:      58,
				V7RiskScore:        20,
				V7LiquidityScore:   80,
				V7RiskLevel:        "LOW",
				V7RiskTags:         []string{"not_near_long_reclaim_zone"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.52},
			},
			wantReason: "pullback_long_reclaim_zone_wait",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTier(tt.coin)
			if tier != "WATCH" || reason != tt.wantReason {
				t.Fatalf("tier = %q (%s), want WATCH %s", tier, reason, tt.wantReason)
			}
		})
	}
}

func TestClassifyHunterV7CandidateTierBlocksFundingReversalOIBuilding(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "BUILDOIUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       52,
		V7TimingScore:      72,
		V7RiskScore:        8,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7RiskTags:         []string{"oi_building_no_flush"},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "funding_reversal_oi_building" {
		t.Fatalf("tier = %q (%s), want WATCH funding_reversal_oi_building", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsMixedOIFundingShortReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "ROBOUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       60.2,
		V7SetupScore:       68,
		V7TimingScore:      72,
		V7RiskScore:        15,
		V7LiquidityScore:   65,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"elevated_funding", "heavy_long_crowding", "price_stalling_after_rally", "oi_mild_buildup", "strong_taker_sell_reversal"},
		V7RiskTags:         []string{"crowding_extreme", "oi_building_no_flush"},
		V7DerivativesCtx: &local.V7DerivativesContext{
			OIChange1h:  2.53,
			OIChange4h:  -0.54,
			TakerBuy15m: 0.377,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "funding_short_reviewable_crowding_reversal" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE funding_short_reviewable_crowding_reversal", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksPureBuildingFundingShort(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "BUILDUSDT",
		Direction:          "SHORT",
		V7SetupType:        "funding_reversal",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       60,
		V7TimingScore:      72,
		V7RiskScore:        15,
		V7LiquidityScore:   85,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"heavy_long_crowding", "strong_taker_sell_reversal"},
		V7RiskTags:         []string{"oi_building_no_flush"},
		V7DerivativesCtx: &local.V7DerivativesContext{
			OIChange1h:  1.5,
			OIChange4h:  0.3,
			TakerBuy15m: 0.38,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "funding_reversal_oi_building" {
		t.Fatalf("tier = %q (%s), want WATCH funding_reversal_oi_building", tier, reason)
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

func TestHunterV7LiveConfirmableBreakoutPromotesReviewable(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "ZECUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       64,
		V7SetupScore:       70,
		V7TimingScore:      55,
		V7RiskScore:        0,
		V7LiquidityScore:   100,
		V7ReasonCodes:      []string{"approaching_breakout", "oi_stable_breakout", "volume_expansion", "clear_air_above"},
		V7RequiredConfirms: []string{"5m_or_15m_close_through_breakout_level"},
		V7EntryZone:        local.V7PriceZone{Lower: 450, Upper: 470},
		V7PriceContext:     &local.V7PriceContext{Last: 465},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{{
				Code:     "5m_or_15m_close_through_breakout_level",
				Severity: local.V7ConfirmReviewWait,
			}},
			EntryZonePosition: 60,
			RR:                3.2,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)
	if tier != "REVIEWABLE" || reason != "live_reviewable_5m_or_15m_close_through_breakout_level" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE live_reviewable_5m_or_15m_close_through_breakout_level", tier, reason)
	}
}

func TestHunterV7LiveConfirmableDoesNotPromoteChaseRisk(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "TLMUSDT",
		Direction:          "LONG",
		V7SetupType:        "displacement_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       63,
		V7SetupScore:       82,
		V7TimingScore:      55,
		V7RiskScore:        50,
		V7LiquidityScore:   100,
		V7ReasonCodes:      []string{"massive_vol_displacement", "oi_confirms_new_demand", "chase_high_protection"},
		V7RequiredConfirms: []string{"taker_flow_confirms_long"},
		V7EntryZone:        local.V7PriceZone{Lower: 0.0029, Upper: 0.0031},
		V7PriceContext:     &local.V7PriceContext{Last: 0.00305},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.50},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
			MissingReview: []local.V7ConfirmationCheck{{
				Code:     "taker_flow_confirms_long",
				Severity: local.V7ConfirmReviewWait,
			}},
			EntryZonePosition: 80,
			RR:                4,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)
	if tier != "WATCH" || reason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("tier = %q (%s), want WATCH confirmation_missing_taker_flow_confirms_long", tier, reason)
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
		V7ReasonCodes:      []string{"strong_4h_momentum", "taker_sustained_buy"},
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

func TestClassifyHunterV7AltLadderLateStageNeedsFreshFlow(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "NIGHTUSDT",
		Direction:          "LONG",
		V7SetupType:        "alt_ladder_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       77,
		V7SetupScore:       84,
		V7TimingScore:      64,
		V7RiskScore:        15,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"alt_ladder_momentum_long", "alt_ladder_stage_late", "alt_ladder_taker_buy", "alt_ladder_oi_inflow"},
		V7RiskTags:         []string{"alt_ladder_late_chase_risk", "high_volatility"},
		V7EntryZone:        local.V7PriceZone{Lower: 0.02434, Upper: 0.02460},
		V7PriceContext:     &local.V7PriceContext{Last: 0.02452},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.590, OIChange1h: -0.94, OIChange4h: 9.62},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier == "EXECUTABLE" || reason == "alt_ladder_long_ready_confirmed" {
		t.Fatalf("tier = %q (%s), late-stage alt ladder without fresh flow should not be executable", tier, reason)
	}
}

func TestClassifyHunterV7AltLadderLongRequiresStrongFlowForExecutable(t *testing.T) {
	tests := []struct {
		name       string
		coin       CandidateCoin
		wantTier   string
		wantReason string
	}{
		{
			name: "weak taker single oi inflow becomes reviewable",
			coin: CandidateCoin{
				Symbol:             "ZBTUSDT",
				Direction:          "LONG",
				V7SetupType:        "alt_ladder_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       86,
				V7SetupScore:       98,
				V7TimingScore:      64,
				V7RiskScore:        0,
				V7LiquidityScore:   90,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"alt_ladder_momentum_long", "alt_ladder_stage_mid", "alt_ladder_oi_inflow"},
				V7EntryZone:        local.V7PriceZone{Lower: 0.1014, Upper: 0.1024},
				V7PriceContext:     &local.V7PriceContext{Last: 0.1021},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.521, OIChange1h: 0.5, OIChange4h: -3.0},
			},
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_long_reviewable_confirmed",
		},
		{
			name: "stop tightened with weak oi is downgraded",
			coin: CandidateCoin{
				Symbol:             "ALLOUSDT",
				Direction:          "LONG",
				V7SetupType:        "alt_ladder_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       90,
				V7SetupScore:       98,
				V7TimingScore:      76,
				V7RiskScore:        0,
				V7LiquidityScore:   100,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"alt_ladder_momentum_long", "alt_ladder_stage_mid", "alt_ladder_taker_buy", "alt_ladder_volume_expansion"},
				V7RiskTags:         []string{"execution_stop_tightened"},
				V7EntryZone:        local.V7PriceZone{Lower: 0.4498, Upper: 0.4528},
				V7PriceContext:     &local.V7PriceContext{Last: 0.4518},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.565, OIChange1h: -0.69, OIChange4h: -1.26},
			},
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_long_reviewable_confirmed",
		},
		{
			name: "strong taker and oi stays executable",
			coin: CandidateCoin{
				Symbol:             "PHAUSDT",
				Direction:          "LONG",
				V7SetupType:        "alt_ladder_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       79,
				V7SetupScore:       89,
				V7TimingScore:      64,
				V7RiskScore:        8,
				V7LiquidityScore:   60,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"alt_ladder_momentum_long", "alt_ladder_stage_early", "alt_ladder_taker_buy", "alt_ladder_oi_inflow"},
				V7EntryZone:        local.V7PriceZone{Lower: 0.02435, Upper: 0.02455},
				V7PriceContext:     &local.V7PriceContext{Last: 0.02449},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.635, OIChange1h: -1.98, OIChange4h: 6.84},
			},
			wantTier:   "EXECUTABLE",
			wantReason: "alt_ladder_long_ready_confirmed",
		},
		{
			name: "late high volatility without oi inflow is reviewable",
			coin: CandidateCoin{
				Symbol:             "SYNUSDT",
				Direction:          "LONG",
				V7SetupType:        "alt_ladder_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       87,
				V7SetupScore:       96,
				V7TimingScore:      85,
				V7RiskScore:        15,
				V7LiquidityScore:   100,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"alt_ladder_momentum_long", "alt_ladder_stage_late", "alt_ladder_taker_buy", "alt_ladder_volume_expansion"},
				V7RiskTags:         []string{"alt_ladder_late_chase_risk", "high_volatility"},
				V7EntryZone:        local.V7PriceZone{Lower: 0.2303, Upper: 0.2329},
				V7PriceContext:     &local.V7PriceContext{Last: 0.2321},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.60, OIChange1h: -0.56, OIChange4h: -7.89},
			},
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_long_reviewable_confirmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTier(tt.coin)
			if tier != tt.wantTier || reason != tt.wantReason {
				t.Fatalf("tier = %q (%s), want %s %s", tier, reason, tt.wantTier, tt.wantReason)
			}
		})
	}
}

func TestClassifyHunterV7CandidateTierAllowsAltLadderRoutes(t *testing.T) {
	tests := []struct {
		name       string
		coin       CandidateCoin
		wantTier   string
		wantReason string
	}{
		{
			name: "alt ladder long ready",
			coin: CandidateCoin{
				Symbol:             "ALTAUSDT",
				Direction:          "LONG",
				V7SetupType:        "alt_ladder_momentum_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       66,
				V7SetupScore:       68,
				V7TimingScore:      64,
				V7RiskScore:        12,
				V7LiquidityScore:   75,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"alt_ladder_momentum_long", "alt_ladder_stage_early", "alt_ladder_taker_buy", "alt_ladder_oi_inflow"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.58},
			},
			wantTier:   "EXECUTABLE",
			wantReason: "alt_ladder_long_ready_confirmed",
		},
		{
			name: "alt ladder short ready",
			coin: CandidateCoin{
				Symbol:             "ALTCUSDT",
				Direction:          "SHORT",
				V7SetupType:        "alt_ladder_breakdown_short",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       64,
				V7SetupScore:       66,
				V7TimingScore:      62,
				V7RiskScore:        10,
				V7LiquidityScore:   75,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"alt_ladder_breakdown_short", "alt_ladder_downshift_early", "alt_ladder_taker_sell", "alt_ladder_new_shorts"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.44},
			},
			wantTier:   "REVIEWABLE",
			wantReason: "alt_ladder_short_reviewable_confirmed",
		},
		{
			name: "alt ladder short strong ready",
			coin: CandidateCoin{
				Symbol:             "ALTDUSDT",
				Direction:          "SHORT",
				V7SetupType:        "alt_ladder_breakdown_short",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       68,
				V7SetupScore:       76,
				V7TimingScore:      68,
				V7RiskScore:        10,
				V7LiquidityScore:   75,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"alt_ladder_breakdown_short", "alt_ladder_downshift_mid", "alt_ladder_taker_sell", "alt_ladder_new_shorts"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.42},
			},
			wantTier:   "EXECUTABLE",
			wantReason: "alt_ladder_short_ready_strong_confirmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTier(tt.coin)
			if tier != tt.wantTier || reason != tt.wantReason {
				t.Fatalf("tier = %q (%s), want %s %s", tier, reason, tt.wantTier, tt.wantReason)
			}
		})
	}
}

func TestClassifyHunterV7CandidateTierAllowsBreakdownMomentumShort(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "SKYAIUSDT",
		Direction:          "SHORT",
		V7SetupType:        "breakdown_momentum_short",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       61,
		V7SetupScore:       78,
		V7TimingScore:      66,
		V7RiskScore:        8,
		V7LiquidityScore:   80,
		V7RiskLevel:        "LOW",
		V7ReasonCodes: []string{
			"strong_1h_downside_momentum",
			"below_vwap_breakdown",
			"heavy_taker_selling",
			"oi_confirms_new_shorts",
			"sell_volume_confirmed",
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "EXECUTABLE" || reason != "short_or_reversion_ready_confirmed" {
		t.Fatalf("tier = %q (%s), want EXECUTABLE short_or_reversion_ready_confirmed", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsBreakoutTriggerMemoryConfirmedReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "SUIUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7MarketShape:      "shape_trend_breakout",
		V7EntrySignal:      "entry_open_now",
		V7AIPriority:       52.57,
		V7SetupScore:       86,
		V7TimingScore:      25,
		V7RiskScore:        20,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"trigger_memory_confirmed", "5m_or_15m_close_through_breakout_level"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.53},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "breakout_trigger_memory_confirmed_reviewable" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE breakout_trigger_memory_confirmed_reviewable", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsBreakoutTriggerNearReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "BULLAUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7MarketShape:      "shape_trend_breakout",
		V7EntrySignal:      "entry_trigger_near",
		V7AIPriority:       54.9,
		V7SetupScore:       86.4,
		V7TimingScore:      25,
		V7RiskScore:        0,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"shape_trend_breakout", "entry_trigger_near"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.53},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "breakout_trigger_near_reviewable" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE breakout_trigger_near_reviewable", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsFlowConfirmedBreakoutTriggerNearReview(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "NEARUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7MarketShape:      "shape_trend_breakout",
		V7EntrySignal:      "entry_trigger_near",
		V7AIPriority:       48.7,
		V7SetupScore:       76.8,
		V7TimingScore:      25,
		V7RiskScore:        8,
		V7LiquidityScore:   85,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"approaching_breakout", "taker_aggressive_buy", "clear_air_above", "entry_trigger_near"},
		V7ConfirmSummary:   &local.V7ConfirmationSummary{RR: 20.56},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.64, OIChange1h: 0.3},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "breakout_trigger_near_flow_reviewable" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE breakout_trigger_near_flow_reviewable", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsMMSRoutes(t *testing.T) {
	tests := []struct {
		name       string
		coin       CandidateCoin
		wantTier   string
		wantReason string
	}{
		{
			name: "bottom wake reviewable",
			coin: CandidateCoin{
				Symbol:           "MMSAUSDT",
				Direction:        "LONG",
				V7SetupType:      "mms_bottom_wake_long",
				V7Status:         "wait_confirm",
				V7AIPriority:     52,
				V7SetupScore:     64,
				V7TimingScore:    50,
				V7RiskScore:      8,
				V7LiquidityScore: 70,
				V7RiskLevel:      "LOW",
				V7ReasonCodes:    []string{"mms_bottom_wake", "mms_oi_stealth_inflow", "mms_volume_wake"},
				V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.52},
			},
			wantTier:   "REVIEWABLE",
			wantReason: "mms_bottom_wake_reviewable_breakout_required",
		},
		{
			name: "squeeze ready",
			coin: CandidateCoin{
				Symbol:             "MMSCUSDT",
				Direction:          "LONG",
				V7SetupType:        "mms_squeeze_engine_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7AIPriority:       72,
				V7SetupScore:       86,
				V7TimingScore:      72,
				V7RiskScore:        8,
				V7LiquidityScore:   80,
				V7RiskLevel:        "LOW",
				V7ReasonCodes:      []string{"mms_squeeze_engine", "mms_short_ban_active"},
				V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.57},
			},
			wantTier:   "EXECUTABLE",
			wantReason: "mms_long_ready_confirmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, reason := classifyHunterV7CandidateTier(tt.coin)
			if tier != tt.wantTier || reason != tt.wantReason {
				t.Fatalf("tier = %q (%s), want %s %s", tier, reason, tt.wantTier, tt.wantReason)
			}
		})
	}
}

func TestClassifyHunterV7CandidateTierAllowsNearConfirmWhenMicroConfirmed(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "MICROUSDT",
		Direction:          "LONG",
		V7SetupType:        "custom_near_confirm_setup",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       58,
		V7SetupScore:       60,
		V7TimingScore:      55,
		V7RiskScore:        20,
		V7LiquidityScore:   80,
		V7RiskLevel:        "LOW",
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: true,
		},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "near_confirm_reviewable_micro_confirmed" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE near_confirm_reviewable_micro_confirmed", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierAllowsRepairedDisplacementOnlyReviewable(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "DODOXUSDT",
		Direction:          "LONG",
		V7SetupType:        "displacement_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7EntrySignal:      "entry_rr_repairable",
		V7AIPriority:       77.955,
		V7SetupScore:       93.5,
		V7TimingScore:      70,
		V7RiskScore:        30,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7RiskTags: []string{
			"displacement_chase_risk_overextended",
			"displacement_rr_repaired_needs_review",
			"high_volatility",
			"funding_elevated",
			"execution_stop_tightened",
		},
		V7PriceContext:   &local.V7PriceContext{Last: 0.027769},
		V7Invalidation:   local.V7InvalidationRule{Price: 0.027074775},
		V7Targets:        []local.V7Target{{Price: 0.029071}},
		V7DerivativesCtx: &local.V7DerivativesContext{TakerBuy15m: 0.53, OIChange1h: 11.87},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "displacement_reviewable_needs_confirm" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE displacement_reviewable_needs_confirm", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierBlocksLeaderMomentumUpperZoneChase(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "SKYAIUSDT",
		Direction:          "LONG",
		V7SetupType:        "leader_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       85.56,
		V7SetupScore:       100,
		V7TimingScore:      78,
		V7RiskScore:        0,
		V7LiquidityScore:   80,
		V7RiskLevel:        "LOW",
		V7ReasonCodes: []string{
			"strong_24h_momentum",
			"solid_4h_momentum",
			"accelerating_1h",
			"oi_healthy_growth",
			"taker_strong_buy",
			"no_pullback_still_running",
		},
		V7EntryZone:      local.V7PriceZone{Lower: 0.03549129456896678, Upper: 0.036259514536637456},
		V7PriceContext:   &local.V7PriceContext{Last: 0.03605, VWAP15m: 0.033929655667319744},
		V7DerivativesCtx: &local.V7DerivativesContext{OIChange1h: -3.0062172399272358, TakerBuy15m: 0.5845970211937053},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "momentum_upper_zone_chase_wait" {
		t.Fatalf("tier = %q (%s), want WATCH momentum_upper_zone_chase_wait", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierDowngradesStopTightenedWithoutStrongFlow(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "LABUSDT",
		Direction:          "LONG",
		V7SetupType:        "mms_trend_ride_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       79,
		V7SetupScore:       90,
		V7TimingScore:      68,
		V7RiskScore:        8,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7RiskTags:         []string{"execution_stop_tightened"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier == "EXECUTABLE" || reason == "mms_long_ready_confirmed" {
		t.Fatalf("tier = %q (%s), stop-tightened weak flow should not be EXECUTABLE", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierKeepsAltLadderExtremeAsWatch(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "AKEUSDT",
		Direction:          "LONG",
		V7SetupType:        "alt_ladder_momentum_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       43,
		V7SetupScore:       74,
		V7TimingScore:      25,
		V7RiskScore:        80,
		V7LiquidityScore:   100,
		V7RiskLevel:        "EXTREME",
		V7ReasonCodes:      []string{"alt_ladder_momentum_long", "alt_ladder_stage_extreme"},
		V7RiskTags:         []string{"alt_ladder_extreme_continuation_watch", "extreme_volatility", "funding_extreme"},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "alt_ladder_extreme_continuation_watch" {
		t.Fatalf("tier = %q (%s), want WATCH alt_ladder_extreme_continuation_watch", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierKeepsNearConfirmWatchWhenMicroFails(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "MICROFAILUSDT",
		Direction:          "LONG",
		V7SetupType:        "custom_near_confirm_setup",
		V7Status:           "candidate",
		V7ExecutionQuality: "near_confirm",
		V7AIPriority:       58,
		V7SetupScore:       60,
		V7TimingScore:      55,
		V7RiskScore:        20,
		V7LiquidityScore:   80,
		V7RiskLevel:        "LOW",
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.54},
		V7ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:   true,
			PassedReview: false,
		},
	}

	tier, _ := classifyHunterV7CandidateTier(coin)

	if tier == "REVIEWABLE" || tier == "EXECUTABLE" {
		t.Fatalf("tier = %q, want WATCH-class result when micro review failed", tier)
	}
}

func TestClassifyHunterV7CandidateTierReportsLowLiquidityWhenDisplacementFinalRROK(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "DATAIPUSDT",
		Direction:          "LONG",
		V7SetupType:        "displacement_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       65.9,
		V7SetupScore:       85.8,
		V7TimingScore:      60,
		V7RiskScore:        45,
		V7LiquidityScore:   40,
		V7RiskLevel:        "LOW",
		V7RiskTags:         []string{"displacement_rr_insufficient", "funding_extreme", "low_liquidity"},
		V7ConfirmSummary:   &local.V7ConfirmationSummary{RR: 1.76},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.64},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REJECTED" || reason != "liquidity_lt_50" {
		t.Fatalf("tier = %q (%s), want REJECTED liquidity_lt_50", tier, reason)
	}
}

func TestClassifyHunterV7CandidateTierReviewsStrongDisplacementRRInsufficient(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "SLXUSDT",
		Direction:          "LONG",
		V7SetupType:        "displacement_momentum_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7EntrySignal:      "entry_open_now",
		V7AIPriority:       80,
		V7SetupScore:       85,
		V7TimingScore:      70,
		V7RiskScore:        15,
		V7LiquidityScore:   90,
		V7RiskLevel:        "LOW",
		V7RiskTags:         []string{"displacement_rr_insufficient"},
		V7ConfirmSummary:   &local.V7ConfirmationSummary{RR: 2.6},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.55},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "displacement_reviewable_needs_confirm" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE displacement_reviewable_needs_confirm", tier, reason)
	}
}

func TestClassifyHunterV7MMSLongDowngradesExtendedVWAPChase(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "TRADOORUSDT",
		Direction:          "LONG",
		V7SetupType:        "mms_trend_ride_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       89,
		V7SetupScore:       100,
		V7TimingScore:      68,
		V7RiskScore:        0,
		V7LiquidityScore:   100,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "taker_buy_strong"},
		V7PriceContext:     &local.V7PriceContext{Last: 0.5326, Change24h: 22.24, VWAP15m: 0.5071},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.574, OIChange1h: 0.58, OIChange4h: -1.05},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "mms_long_reviewable_confirmed" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE mms_long_reviewable_confirmed", tier, reason)
	}
}

func TestClassifyHunterV7MMSLongRejectsWeakReentryOutsideZone(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "MYXUSDT",
		Direction:          "LONG",
		V7SetupType:        "mms_trend_ride_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       74,
		V7SetupScore:       78,
		V7TimingScore:      68,
		V7RiskScore:        8,
		V7LiquidityScore:   55,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "mms_low_volume_retest", "shape_clean_momentum", "entry_open_now"},
		V7PriceContext:     &local.V7PriceContext{Last: 0.08341, Change1h: -0.11, Change4h: -0.11, VWAP15m: 0.08075},
		V7EntryZone:        local.V7PriceZone{Lower: 0.08279, Upper: 0.08332},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.589, OIChange1h: 0.63, OIChange4h: -1.35},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier == "EXECUTABLE" || reason == "mms_long_ready_confirmed" {
		t.Fatalf("tier = %q (%s), weak reentry outside zone should not be executable", tier, reason)
	}
}

func TestClassifyHunterV7TrendBreakoutStrongFlowUpgradesWatchToReviewable(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "ENAUSDT",
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7Status:           "wait_confirm",
		V7ExecutionQuality: "watch_only",
		V7AIPriority:       47,
		V7SetupScore:       64,
		V7TimingScore:      25,
		V7RiskScore:        0,
		V7LiquidityScore:   85,
		V7RiskLevel:        "LOW",
		V7ReasonCodes:      []string{"approaching_breakout", "taker_aggressive_buy", "clear_air_above", "low_timing_watch_only"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.58, OIChange1h: 0.4},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REVIEWABLE" || reason != "breakout_watch_strong_flow_reviewable" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE breakout_watch_strong_flow_reviewable", tier, reason)
	}
}

func TestClassifyHunterV7WaitOnlyRiskTagBlocksExecutable(t *testing.T) {
	coin := CandidateCoin{
		Symbol:             "HUSDT",
		Direction:          "LONG",
		V7SetupType:        "mms_trend_ride_long",
		V7Status:           "candidate",
		V7ExecutionQuality: "ready",
		V7AIPriority:       88,
		V7SetupScore:       100,
		V7TimingScore:      68,
		V7RiskScore:        0,
		V7LiquidityScore:   55,
		V7RiskLevel:        "LOW",
		V7RiskTags:         []string{"crowding_extreme"},
		V7ReasonCodes:      []string{"mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "entry_open_now"},
		V7EntryZone:        local.V7PriceZone{Lower: 0.0618, Upper: 0.0624},
		V7PriceContext:     &local.V7PriceContext{Last: 0.06213, Change1h: 0.13, Change4h: 0.11},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.517, OIChange1h: -0.32, OIChange4h: 0.5},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "WATCH" || reason != "crowding_extreme" {
		t.Fatalf("tier = %q (%s), want WATCH crowding_extreme", tier, reason)
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
