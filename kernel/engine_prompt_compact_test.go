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
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
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
}
