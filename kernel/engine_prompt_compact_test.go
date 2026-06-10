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
		"Min Confidence: ≥70 to open position",
		"Confidence below 70 must output wait; do not open by reducing position size.",
		"`confidence`: 0-100 (opening recommended ≥ 70)",
		"Hunter v7 Execution Rules",
		"choose the best open or provide one precise blocked_reason",
		"weak upper-zone pullbacks",
		"Peak PnL reached protection near-TP1",
		"gives back >=45% from the peak",
		"do not use `hold` to claim stop tightening",
		"second protection chance",
		"pre-TP1 noise",
		"close only on planned SL/hard invalidation or when both 5m and 15m confirm structural reversal",
		"crossing from a positive peak to negative PnL is >100% giveback",
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
	if !strings.Contains(preTP1, "protection_state=pre_tp1") ||
		!strings.Contains(preTP1, "peak giveback alone is not a trailing-exit trigger") ||
		!strings.Contains(preTP1, "confirmed 5m+15m structural reversal") {
		t.Fatalf("pre-TP1 position hint missing:\n%s", preTP1)
	}

	nearTP1 := engine.formatPositionInfo(1, PositionInfo{
		Symbol:           "TESTUSDT",
		Side:             "long",
		EntryPrice:       100,
		MarkPrice:        100.3,
		Quantity:         1,
		UnrealizedPnLPct: 6.0,
		PeakPnLPct:       5.8,
		Leverage:         20,
	}, ctx)
	if !strings.Contains(nearTP1, "protection_state=near_tp1_or_better") ||
		!strings.Contains(nearTP1, "peak giveback may be a trailing-exit signal") {
		t.Fatalf("near-TP1 position hint missing:\n%s", nearTP1)
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
		"### Open-review candidates (full context, max 5)",
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
		"READYUSDT LONG tier=EXECUTABLE",
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
		V7RiskTags:         []string{"displacement_rr_insufficient"},
		V7DerivativesCtx:   &local.V7DerivativesContext{TakerBuy15m: 0.55},
	}

	tier, reason := classifyHunterV7CandidateTier(coin)

	if tier != "REJECTED" || reason != "displacement_rr_insufficient" {
		t.Fatalf("tier = %q (%s), want REJECTED displacement_rr_insufficient", tier, reason)
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

func TestClassifyHunterV7CandidateTierAllowsStrongMomentumChaseRiskReview(t *testing.T) {
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

	if tier != "REVIEWABLE" || reason != "momentum_chase_risk_reviewable_pullback_only" {
		t.Fatalf("tier = %q (%s), want REVIEWABLE momentum_chase_risk_reviewable_pullback_only", tier, reason)
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
