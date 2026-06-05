package local

import (
	"testing"

	"github.com/Aixxww/AiT/provider/aitos"
)

// ============================================================================
// Helpers
// ============================================================================

func makeCoin(pair, direction string, longScore, shortScore float64, longTags, shortTags []string) aitos.CoinData {
	return aitos.CoinData{
		Pair:       pair,
		Direction:  direction,
		LongScore:  longScore,
		ShortScore: shortScore,
		LongTags:   longTags,
		ShortTags:  shortTags,
	}
}

func makeMeta(direction string, longScore, shortScore float64, longTags, shortTags []string) *HunterCoinMeta {
	return &HunterCoinMeta{
		Direction:  direction,
		LongScore:  longScore,
		ShortScore: shortScore,
		LongTags:   longTags,
		ShortTags:  shortTags,
	}
}

// ============================================================================
// LONG Ambush Filter Tests
// ============================================================================

func TestFilterLongAmbush_Pass(t *testing.T) {
	coin := makeCoin("BTCUSDT", "LONG", 40, 10,
		[]string{"bb_squeeze_15m", "oi_accumulation", "near_support_4h"},
		nil,
	)
	meta := makeMeta("LONG", 40, 10,
		[]string{"bb_squeeze_15m", "oi_accumulation", "near_support_4h"},
		nil,
	)

	result := filterLongAmbush(coin, meta)
	if result == nil {
		t.Fatal("expected LONG_AMBUSH to pass, got nil")
	}
	if result.AmbushType != LongAmbush {
		t.Errorf("expected AmbushType=LONG_AMBUSH, got %s", result.AmbushType)
	}
	if result.Symbol != "BTCUSDT" {
		t.Errorf("expected Symbol=BTCUSDT, got %s", result.Symbol)
	}
	// Should have 3 reasons: bb_squeeze, oi_accumulation, near_support_4h
	if len(result.Reasons) != 3 {
		t.Errorf("expected 3 reasons, got %d: %v", len(result.Reasons), result.Reasons)
	}
}

func TestFilterLongAmbush_BlockedByWall(t *testing.T) {
	// LongTags are fine, but ShortTags contain near_resistance_4h → wall block
	coin := makeCoin("ETHUSDT", "LONG", 40, 10,
		[]string{"bb_squeeze_15m", "oi_accumulation"},
		[]string{"near_resistance_4h"},
	)
	meta := makeMeta("LONG", 40, 10,
		[]string{"bb_squeeze_15m", "oi_accumulation"},
		[]string{"near_resistance_4h"},
	)

	result := filterLongAmbush(coin, meta)
	if result != nil {
		t.Errorf("expected LONG_AMBUSH to be blocked by wall, got %+v", result)
	}
}

func TestFilterLongAmbush_BlockedByWash(t *testing.T) {
	coin := makeCoin("SOLUSDT", "LONG", 40, 10,
		[]string{"bb_squeeze_15m", "oi_accumulation", "wash_micro_trades"},
		nil,
	)
	meta := makeMeta("LONG", 40, 10,
		[]string{"bb_squeeze_15m", "oi_accumulation", "wash_micro_trades"},
		nil,
	)

	result := filterLongAmbush(coin, meta)
	if result != nil {
		t.Errorf("expected LONG_AMBUSH to be blocked by wash trade, got %+v", result)
	}
}

func TestFilterLongAmbush_BlockedByScore(t *testing.T) {
	// Score 15 < MinScoreSniffer (20)
	coin := makeCoin("DOGEUSDT", "LONG", 15, 10,
		[]string{"bb_squeeze_15m", "oi_accumulation"},
		nil,
	)
	meta := makeMeta("LONG", 15, 10,
		[]string{"bb_squeeze_15m", "oi_accumulation"},
		nil,
	)

	result := filterLongAmbush(coin, meta)
	if result != nil {
		t.Errorf("expected LONG_AMBUSH to be blocked by score, got %+v", result)
	}
}

func TestFilterLongAmbush_BlockedByCompression(t *testing.T) {
	// No compression signals at all (oi_accumulation alone scores 2 = passes now)
	// Use only non-compression tags to ensure Gate 2 blocks.
	coin := makeCoin("BNBUSDT", "LONG", 40, 10,
		[]string{"near_support_4h"},
		nil,
	)
	meta := makeMeta("LONG", 40, 10,
		[]string{"near_support_4h"},
		nil,
	)

	result := filterLongAmbush(coin, meta)
	if result != nil {
		t.Errorf("expected LONG_AMBUSH to be blocked by compression, got %+v", result)
	}
}

func TestFilterLongAmbush_PassWithOICompression(t *testing.T) {
	// With the new elastic scoring, oi_accumulation (score=2) alone should pass Gate 2
	coin := makeCoin("SOLUSDT", "LONG", 45, 10,
		[]string{"oi_accumulation", "near_support_4h"},
		nil,
	)
	meta := makeMeta("LONG", 45, 10,
		[]string{"oi_accumulation", "near_support_4h"},
		nil,
	)

	result := filterLongAmbush(coin, meta)
	if result == nil {
		t.Fatal("expected LONG_AMBUSH to pass with oi_accumulation compression signal")
	}
	if result.AmbushType != LongAmbush {
		t.Errorf("expected AmbushType=LONG_AMBUSH, got %s", result.AmbushType)
	}
}

// ============================================================================
// SHORT Distribution Filter Tests
// ============================================================================

func TestFilterShortDist_Pass(t *testing.T) {
	coin := makeCoin("AVAXUSDT", "SHORT", 10, 40,
		nil,
		[]string{"bb_squeeze_15m", "oi_distribution"},
	)
	meta := makeMeta("SHORT", 10, 40,
		nil,
		[]string{"bb_squeeze_15m", "oi_distribution"},
	)

	result := filterShortDistribution(coin, meta)
	if result == nil {
		t.Fatal("expected SHORT_DIST to pass, got nil")
	}
	if result.AmbushType != ShortDistribution {
		t.Errorf("expected AmbushType=SHORT_DISTRIBUTION, got %s", result.AmbushType)
	}
	if result.Symbol != "AVAXUSDT" {
		t.Errorf("expected Symbol=AVAXUSDT, got %s", result.Symbol)
	}
	// Should have 2 reasons: bb_squeeze, oi_distribution
	if len(result.Reasons) < 2 {
		t.Errorf("expected >=2 reasons, got %d: %v", len(result.Reasons), result.Reasons)
	}
}

func TestFilterShortDist_BlockedByWall(t *testing.T) {
	// LongTags contain near_support_4h → wall block for SHORT
	coin := makeCoin("LINKUSDT", "SHORT", 10, 40,
		[]string{"near_support_4h"},
		[]string{"bb_squeeze_15m", "oi_distribution"},
	)
	meta := makeMeta("SHORT", 10, 40,
		[]string{"near_support_4h"},
		[]string{"bb_squeeze_15m", "oi_distribution"},
	)

	result := filterShortDistribution(coin, meta)
	if result != nil {
		t.Errorf("expected SHORT_DIST to be blocked by wall, got %+v", result)
	}
}

// ============================================================================
// Integration: FilterAmbushCandidates
// ============================================================================

func TestFilterAmbushCandidates_Empty(t *testing.T) {
	// All coins filtered: one has low score, other has no squeeze
	coins := []aitos.CoinData{
		makeCoin("BTCUSDT", "LONG", 10, 5,
			[]string{"oi_accumulation"},
			nil,
		),
		makeCoin("ETHUSDT", "SHORT", 5, 10,
			nil,
			[]string{"oi_distribution"},
		),
	}
	coinMeta := map[string]*HunterCoinMeta{
		"BTCUSDT": makeMeta("LONG", 10, 5,
			[]string{"oi_accumulation"},
			nil,
		),
		"ETHUSDT": makeMeta("SHORT", 5, 10,
			nil,
			[]string{"oi_distribution"},
		),
	}

	// We need a *Client to call FilterAmbushCandidates.
	// Create a minimal client (nil fields are fine since FilterAmbushCandidates doesn't use them).
	client := &Client{}
	result := client.FilterAmbushCandidates(coins, coinMeta)

	if len(result.LongAmbush) != 0 {
		t.Errorf("expected 0 LongAmbush, got %d", len(result.LongAmbush))
	}
	if len(result.ShortDist) != 0 {
		t.Errorf("expected 0 ShortDist, got %d", len(result.ShortDist))
	}
	if result.Stats.TotalScanned != 2 {
		t.Errorf("expected TotalScanned=2, got %d", result.Stats.TotalScanned)
	}
}

func TestFilterAmbushCandidates_MixedResults(t *testing.T) {
	coins := []aitos.CoinData{
		// LONG_AMBUSH should pass
		makeCoin("BTCUSDT", "LONG", 40, 10,
			[]string{"bb_squeeze_15m", "oi_accumulation", "near_support_4h"},
			nil,
		),
		// SHORT_DIST should pass
		makeCoin("ETHUSDT", "SHORT", 10, 40,
			nil,
			[]string{"bb_squeeze_15m", "oi_distribution"},
		),
		// LONG_AMBUSH should pass under flexible compression scoring:
		// oi_accumulation contributes compression_score=2 even without BB squeeze.
		makeCoin("SOLUSDT", "LONG", 30, 10,
			[]string{"oi_accumulation"},
			nil,
		),
	}
	coinMeta := map[string]*HunterCoinMeta{
		"BTCUSDT": makeMeta("LONG", 40, 10,
			[]string{"bb_squeeze_15m", "oi_accumulation", "near_support_4h"},
			nil,
		),
		"ETHUSDT": makeMeta("SHORT", 10, 40,
			nil,
			[]string{"bb_squeeze_15m", "oi_distribution"},
		),
		"SOLUSDT": makeMeta("LONG", 30, 10,
			[]string{"oi_accumulation"},
			nil,
		),
	}

	client := &Client{}
	result := client.FilterAmbushCandidates(coins, coinMeta)

	if len(result.LongAmbush) != 2 {
		t.Errorf("expected 2 LongAmbush, got %d", len(result.LongAmbush))
	}
	if len(result.ShortDist) != 1 {
		t.Errorf("expected 1 ShortDist, got %d", len(result.ShortDist))
	}
	if result.Stats.TotalScanned != 3 {
		t.Errorf("expected TotalScanned=3, got %d", result.Stats.TotalScanned)
	}
	if result.Stats.LongPassed != 2 {
		t.Errorf("expected LongPassed=2, got %d", result.Stats.LongPassed)
	}
	if result.Stats.ShortPassed != 1 {
		t.Errorf("expected ShortPassed=1, got %d", result.Stats.ShortPassed)
	}
}
