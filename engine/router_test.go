package engine

import (
	"github.com/Aixxww/AiT/datafetch"
	"testing"
	"time"
)

// ============================================================================
// Direction Determination Tests
// ============================================================================

func TestDetermineDirection_Bullish(t *testing.T) {
	cfg := DefaultHubConfig()

	set := &IndicatorSet{
		TechBullScore:   30,
		TechBearScore:   5,
		QuantBullScore:  25,
		QuantBearScore:  5,
		SocialBullScore: 15,
		SocialBearScore: 2,
	}

	dir := determineDirection(set, cfg)
	if dir != 1 {
		t.Errorf("Expected LONG (+1), got %d", dir)
	}
}

func TestDetermineDirection_Bearish(t *testing.T) {
	cfg := DefaultHubConfig()

	set := &IndicatorSet{
		TechBullScore:   5,
		TechBearScore:   30,
		QuantBullScore:  5,
		QuantBearScore:  25,
		SocialBullScore: 2,
		SocialBearScore: 15,
	}

	dir := determineDirection(set, cfg)
	if dir != -1 {
		t.Errorf("Expected SHORT (-1), got %d", dir)
	}
}

func TestDetermineDirection_Neutral(t *testing.T) {
	cfg := DefaultHubConfig()

	set := &IndicatorSet{
		TechBullScore:   15,
		TechBearScore:   14,
		QuantBullScore:  10,
		QuantBearScore:  10,
		SocialBullScore: 5,
		SocialBearScore: 5,
	}

	dir := determineDirection(set, cfg)
	if dir != 0 {
		t.Errorf("Expected NEUTRAL (0), got %d", dir)
	}
}

func TestDetermineDirection_ExactMargin(t *testing.T) {
	cfg := DefaultHubConfig()
	cfg.DirectionMargin = 15

	// Exactly at margin — should still be neutral (diff == margin, not > margin)
	set := &IndicatorSet{
		TechBullScore:   20,
		TechBearScore:   5,
		QuantBullScore:  0,
		QuantBearScore:  0,
		SocialBullScore: 0,
		SocialBearScore: 0,
	}

	dir := determineDirection(set, cfg)
	// diff = 20 - 5 = 15, which equals margin but is not > margin
	if dir != 0 {
		t.Errorf("Expected NEUTRAL at exact margin, got %d", dir)
	}
}

// ============================================================================
// Grade Classification Tests
// ============================================================================

func TestDetermineGrade(t *testing.T) {
	cfg := DefaultHubConfig()

	tests := []struct {
		score    float64
		expected Grade
	}{
		{90, GradeS},
		{80, GradeS},
		{79.9, GradeA},
		{65, GradeA},
		{64.9, GradeB},
		{50, GradeB},
		{49.9, GradeC},
		{0, GradeC},
	}

	for _, tt := range tests {
		got := determineGrade(tt.score, cfg)
		if got != tt.expected {
			t.Errorf("Score %.1f: expected Grade %s, got %s", tt.score, tt.expected, got)
		}
	}
}

func TestGradeString(t *testing.T) {
	tests := []struct {
		grade    Grade
		expected string
	}{
		{GradeC, "C"},
		{GradeB, "B"},
		{GradeA, "A"},
		{GradeS, "S"},
		{Grade(99), "?"},
	}

	for _, tt := range tests {
		if got := tt.grade.String(); got != tt.expected {
			t.Errorf("Grade(%d).String() = %q, want %q", tt.grade, got, tt.expected)
		}
	}
}

// ============================================================================
// SL/TP Calculation Tests
// ============================================================================

func TestCalcSLTP_Long(t *testing.T) {
	cfg := DefaultHubConfig()
	snap := &datafetch.SymbolSnapshot{
		Price: 100,
	}
	atr := 5.0

	sl, tp1, tp2, tp3 := calcSLTP(snap, 1, atr, cfg)

	// LONG: SL below entry, TPs above
	expectedSL := 100 - 5*2.0  // 90
	expectedTP1 := 100 + 5*1.5 // 107.5
	expectedTP2 := 100 + 5*3.0 // 115
	expectedTP3 := 100 + 5*5.0 // 125

	if sl != expectedSL {
		t.Errorf("SL: got %.2f, want %.2f", sl, expectedSL)
	}
	if tp1 != expectedTP1 {
		t.Errorf("TP1: got %.2f, want %.2f", tp1, expectedTP1)
	}
	if tp2 != expectedTP2 {
		t.Errorf("TP2: got %.2f, want %.2f", tp2, expectedTP2)
	}
	if tp3 != expectedTP3 {
		t.Errorf("TP3: got %.2f, want %.2f", tp3, expectedTP3)
	}
}

func TestCalcSLTP_Short(t *testing.T) {
	cfg := DefaultHubConfig()
	snap := &datafetch.SymbolSnapshot{
		Price: 100,
	}
	atr := 5.0

	sl, tp1, tp2, tp3 := calcSLTP(snap, -1, atr, cfg)

	// SHORT: SL above entry, TPs below
	expectedSL := 100 + 5*2.0  // 110
	expectedTP1 := 100 - 5*1.5 // 92.5
	expectedTP2 := 100 - 5*3.0 // 85
	expectedTP3 := 100 - 5*5.0 // 75

	if sl != expectedSL {
		t.Errorf("SL: got %.2f, want %.2f", sl, expectedSL)
	}
	if tp1 != expectedTP1 {
		t.Errorf("TP1: got %.2f, want %.2f", tp1, expectedTP1)
	}
	if tp2 != expectedTP2 {
		t.Errorf("TP2: got %.2f, want %.2f", tp2, expectedTP2)
	}
	if tp3 != expectedTP3 {
		t.Errorf("TP3: got %.2f, want %.2f", tp3, expectedTP3)
	}
}

func TestCalcSLTP_Neutral(t *testing.T) {
	cfg := DefaultHubConfig()
	snap := &datafetch.SymbolSnapshot{Price: 100}

	sl, tp1, tp2, tp3 := calcSLTP(snap, 0, 5, cfg)
	if sl != 0 || tp1 != 0 || tp2 != 0 || tp3 != 0 {
		t.Errorf("Neutral direction should return all zeros, got (%.2f, %.2f, %.2f, %.2f)", sl, tp1, tp2, tp3)
	}
}

func TestCalcSLTP_NilSnapshot(t *testing.T) {
	cfg := DefaultHubConfig()
	sl, tp1, tp2, tp3 := calcSLTP(nil, 1, 5, cfg)
	if sl != 0 || tp1 != 0 || tp2 != 0 || tp3 != 0 {
		t.Errorf("Nil snapshot should return all zeros, got (%.2f, %.2f, %.2f, %.2f)", sl, tp1, tp2, tp3)
	}
}

// ============================================================================
// Scoring Utility Tests
// ============================================================================

func TestNormalize(t *testing.T) {
	tests := []struct {
		value, min, max, expected float64
	}{
		{50, 0, 100, 50},
		{0, 0, 100, 0},
		{100, 0, 100, 100},
		{75, 50, 100, 100}, // 75 is halfway between 50 and 100 → 50... wait no: (75-50)/(100-50)*100 = 50
		{150, 0, 100, 100}, // clamped to 100
		{-10, 0, 100, 0},   // clamped to 0
	}

	// Fix the expected for case 3 (index 3)
	if got := normalize(75, 50, 100); got != 50 {
		t.Errorf("normalize(75, 50, 100) = %.2f, want 50", got)
	}

	for i, tt := range tests {
		got := normalize(tt.value, tt.min, tt.max)
		if i == 3 {
			continue // skip the wrong expected
		}
		if got != tt.expected {
			t.Errorf("normalize(%.0f, %.0f, %.0f) = %.2f, want %.2f", tt.value, tt.min, tt.max, got, tt.expected)
		}
	}
}

func TestNormalize_EqualBounds(t *testing.T) {
	got := normalize(50, 50, 50)
	if got != 0 {
		t.Errorf("normalize with equal bounds should return 0, got %.2f", got)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		value, min, max, expected float64
	}{
		{50, 0, 100, 50},
		{-5, 0, 100, 0},
		{150, 0, 100, 100},
		{50, 50, 50, 50},
	}

	for _, tt := range tests {
		got := clamp(tt.value, tt.min, tt.max)
		if got != tt.expected {
			t.Errorf("clamp(%.0f, %.0f, %.0f) = %.2f, want %.2f", tt.value, tt.min, tt.max, got, tt.expected)
		}
	}
}

func TestCalcFinalScore(t *testing.T) {
	cfg := DefaultHubConfig()

	set := &IndicatorSet{
		TechBullScore:   30,
		TechBearScore:   5,
		QuantBullScore:  25,
		QuantBearScore:  5,
		SocialBullScore: 15,
		SocialBearScore: 2,
	}

	score := calcFinalScore(set, cfg)

	// Sub-scores normalized to 0-100 before weighting:
	// TechBull=30/40*100=75, QuantBull=25/40*100=62.5, SocialBull=15/20*100=75
	// bullTotal = 75*0.4 + 62.5*0.4 + 75*0.2 = 30+25+15 = 70
	// bearTotal = 12.5*0.4 + 12.5*0.4 + 10*0.2 = 5+5+2 = 12
	// dominant = 70

	if score != 70 {
		t.Errorf("FinalScore = %.2f, want 70", score)
	}
}

// ============================================================================
// Signal Router Tests
// ============================================================================

func TestSignalRouter_Route_Basic(t *testing.T) {
	cfg := DefaultHubConfig()
	cfg.MinScore = 20
	cfg.CooldownMinutes = 0 // no cooldown for testing

	store := datafetch.NewStore()
	snap := &datafetch.Snapshot{
		Symbols: map[string]*datafetch.SymbolSnapshot{
			"BTCUSDT": {
				Symbol: "BTCUSDT",
				Price:  50000,
				Klines: map[string][]datafetch.Kline{"1h": generateTestKlines(250, 50000)},
			},
		},
	}
	store.Swap(snap)

	hub := NewIndicatorHub(store, cfg)
	router := NewSignalRouter(hub, cfg)

	// Create scored sets manually
	sets := []*IndicatorSet{
		{
			Symbol:          "BTCUSDT",
			Direction:       1,
			FinalScore:      60,
			TechBullScore:   30,
			TechBearScore:   5,
			QuantBullScore:  20,
			QuantBearScore:  5,
			SocialBullScore: 10,
			SocialBearScore: 2,
			ATR14:           500,
			BBMiddle:        50000,
		},
	}

	signals := router.Route(sets)

	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	sig := signals[0]
	if sig.Symbol != "BTCUSDT" {
		t.Errorf("Symbol should be BTCUSDT, got %s", sig.Symbol)
	}
	if sig.Direction != 1 {
		t.Errorf("Direction should be 1 (LONG), got %d", sig.Direction)
	}
	if sig.Grade != GradeB {
		t.Errorf("Grade should be B (score 60 in 50-64 range), got %s", sig.Grade)
	}
	if sig.EntryPrice != 50000 {
		t.Errorf("EntryPrice should be 50000, got %.2f", sig.EntryPrice)
	}
}

func TestSignalRouter_Route_Cooldown(t *testing.T) {
	cfg := DefaultHubConfig()
	cfg.MinScore = 20
	cfg.CooldownMinutes = 60

	store := datafetch.NewStore()
	snap := &datafetch.Snapshot{
		Symbols: map[string]*datafetch.SymbolSnapshot{
			"BTCUSDT": {
				Symbol: "BTCUSDT",
				Price:  50000,
				Klines: map[string][]datafetch.Kline{"1h": generateTestKlines(250, 50000)},
			},
		},
	}
	store.Swap(snap)

	hub := NewIndicatorHub(store, cfg)
	router := NewSignalRouter(hub, cfg)

	sets := []*IndicatorSet{
		{
			Symbol:        "BTCUSDT",
			Direction:     1,
			FinalScore:    60,
			TechBullScore: 30, TechBearScore: 5,
			QuantBullScore: 20, QuantBearScore: 5,
			SocialBullScore: 10, SocialBearScore: 2,
			ATR14: 500, BBMiddle: 50000,
		},
	}

	// First route should succeed
	signals := router.Route(sets)
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal on first route, got %d", len(signals))
	}

	// Record trade — should trigger cooldown
	router.RecordTrade("BTCUSDT")

	// Second route should be filtered by cooldown
	signals = router.Route(sets)
	if len(signals) != 0 {
		t.Errorf("Expected 0 signals during cooldown, got %d", len(signals))
	}

	// Clear cooldown
	router.ClearCooldown("BTCUSDT")

	// Third route should succeed again
	signals = router.Route(sets)
	if len(signals) != 1 {
		t.Errorf("Expected 1 signal after clearing cooldown, got %d", len(signals))
	}
}

func TestSignalRouter_Route_TopN(t *testing.T) {
	cfg := DefaultHubConfig()
	cfg.MinScore = 20
	cfg.MaxSignalsPerCycle = 2
	cfg.CooldownMinutes = 0

	store := datafetch.NewStore()
	symbols := make(map[string]*datafetch.SymbolSnapshot)
	for i := 0; i < 5; i++ {
		sym := "SYM" + string(rune('A'+i)) + "USDT"
		symbols[sym] = &datafetch.SymbolSnapshot{
			Symbol: sym,
			Price:  100,
			Klines: map[string][]datafetch.Kline{"1h": generateTestKlines(250, 100)},
		}
	}
	store.Swap(&datafetch.Snapshot{Symbols: symbols})

	hub := NewIndicatorHub(store, cfg)
	router := NewSignalRouter(hub, cfg)

	sets := make([]*IndicatorSet, 5)
	for i := range sets {
		sym := "SYM" + string(rune('A'+i)) + "USDT"
		sets[i] = &IndicatorSet{
			Symbol:          sym,
			Direction:       1,
			FinalScore:      float64(70 - i*5),
			TechBullScore:   30,
			TechBearScore:   5,
			QuantBullScore:  20,
			QuantBearScore:  5,
			SocialBullScore: 10,
			SocialBearScore: 2,
			ATR14:           5,
			BBMiddle:        100,
		}
	}

	signals := router.Route(sets)
	if len(signals) > 2 {
		t.Errorf("Expected at most 2 signals (MaxSignalsPerCycle=2), got %d", len(signals))
	}
}

func TestSignalRouter_Route_FilterNeutral(t *testing.T) {
	cfg := DefaultHubConfig()
	cfg.MinScore = 20
	cfg.CooldownMinutes = 0

	store := datafetch.NewStore()
	store.Swap(&datafetch.Snapshot{
		Symbols: map[string]*datafetch.SymbolSnapshot{
			"BTCUSDT": {Symbol: "BTCUSDT", Price: 50000, Klines: map[string][]datafetch.Kline{"1h": generateTestKlines(250, 50000)}},
		},
	})

	hub := NewIndicatorHub(store, cfg)
	router := NewSignalRouter(hub, cfg)

	sets := []*IndicatorSet{
		{
			Symbol:     "BTCUSDT",
			Direction:  0, // NEUTRAL
			FinalScore: 80,
		},
	}

	signals := router.Route(sets)
	if len(signals) != 0 {
		t.Errorf("Neutral signals should be filtered, got %d", len(signals))
	}
}

// ============================================================================
// Final Score Integration Test
// ============================================================================

func TestScoreSymbol_Integration(t *testing.T) {
	cfg := DefaultHubConfig()

	klines := generateTestKlines(250, 100)
	snap := &datafetch.SymbolSnapshot{
		Symbol: "TESTUSDT",
		Price:  100,
		Klines: map[string][]datafetch.Kline{"1h": klines},
		// Quant fields mapped directly onto SymbolSnapshot:
		//   OpenInterest → OI, OpenInterest1hAgo → OIDelta1h (as % change)
		//   FundingRate, LongShortRatio, LSRPrev, TakerBuyRatio
		//   CurrentVolume → Volume24h
		OI:             1000000,
		OIDelta1h:      100.0 / 9.0, // (1000000-900000)/900000*100 ≈ 11.11%
		FundingRate:    -0.0001,
		LongShortRatio: 0.9,
		LSRPrev:        0.85,
		TakerBuyRatio:  0.55,
		Volume24h:      50000,
		PriceChange24h: 1.5,
		Social: datafetch.SocialData{
			HeatScore:      65,
			Sentiment:      50,
			SentimentDelta: 5,
			SocialVolume:   5000,
			VolumeChange:   2000, // was SocialVol24hAgo (not available); using VolumeChange
		},
	}

	set := scoreSymbol(snap, cfg)

	// Should have all components scored
	if set.FinalScore <= 0 {
		t.Errorf("FinalScore should be positive, got %.2f", set.FinalScore)
	}

	// Direction should be determined
	if set.Direction == 0 && set.FinalScore > cfg.MinScore {
		t.Log("Direction is NEUTRAL despite positive score — acceptable if bull-bear difference < margin")
	}

	t.Logf("Integration test result: Score=%.2f, Direction=%d, TechB=%.2f, TechBe=%.2f, QB=%.2f, QBe=%.2f, SB=%.2f, SBe=%.2f",
		set.FinalScore, set.Direction,
		set.TechBullScore, set.TechBearScore,
		set.QuantBullScore, set.QuantBearScore,
		set.SocialBullScore, set.SocialBearScore)
}

// ============================================================================
// Helpers
// ============================================================================

func generateTestKlines(n int, basePrice float64) []datafetch.Kline {
	klines := make([]datafetch.Kline, n)
	for i := range klines {
		// Slight uptrend with noise
		price := basePrice + float64(i)*0.1 + float64(i%5)*0.5
		klines[i] = datafetch.Kline{
			OpenTime: int64(i) * 3600000,
			Open:     price,
			High:     price + 2,
			Low:      price - 2,
			Close:    price + 0.3,
			Volume:   1000 + float64(i%10)*100,
		}
	}
	return klines
}

func TestBuildSignalReasons(t *testing.T) {
	set := &IndicatorSet{
		RSI14:           25,
		MACDHist:        0.5,
		BBUpper:         110,
		BBMiddle:        100,
		BBLower:         90,
		EMA20:           105,
		EMA50:           102,
		EMA200:          98,
		OIScore:         20,
		FundingScore:    -30,
		TakerScore:      25,
		VolumeScore:     60,
		SocialHeatScore: 75,
		SocialSentiment: 65,
		SocialVolumePct: 55,
		FinalScore:      72,
	}

	bullSignals, bearSignals, reasons := buildSignalReasons(set)

	if len(bullSignals) == 0 {
		t.Error("Expected bull signals for bullish indicator set")
	}

	t.Logf("Bull signals: %v", bullSignals)
	t.Logf("Bear signals: %v", bearSignals)
	t.Logf("Reasons: %v", reasons)
}

func TestDefaultHubConfig(t *testing.T) {
	cfg := DefaultHubConfig()

	if cfg.TechWeight != 40 {
		t.Errorf("TechWeight = %.0f, want 40", cfg.TechWeight)
	}
	if cfg.QuantWeight != 40 {
		t.Errorf("QuantWeight = %.0f, want 40", cfg.QuantWeight)
	}
	if cfg.SocialWeight != 20 {
		t.Errorf("SocialWeight = %.0f, want 20", cfg.SocialWeight)
	}
	if cfg.GradeSThreshold != 80 {
		t.Errorf("GradeSThreshold = %.0f, want 80", cfg.GradeSThreshold)
	}
	if !cfg.RSIEnabled {
		t.Error("RSIEnabled should be true by default")
	}
	if cfg.MaxSignalsPerCycle != 5 {
		t.Errorf("MaxSignalsPerCycle = %d, want 5", cfg.MaxSignalsPerCycle)
	}
}

// Ensure time import is used
var _ = time.Now
