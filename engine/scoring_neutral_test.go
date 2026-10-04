package engine

import (
	"math"
	"testing"
)

// ============================================================================
// P1-1 Neutral-Band Downgrade Tests
// ============================================================================

// With default weights (40/40/20), setting only the Tech scores maps 1:1 to the
// normalized bull/bear totals: bull = TechBullScore, bear = TechBearScore.

func TestNeutralBand_BullDominant(t *testing.T) {
	cfg := DefaultHubConfig() // DirectionMargin = 15

	// bull=95, bear=90: diff=5 <= margin → weak LONG, score downgraded ×0.8
	set := &IndicatorSet{
		Symbol:        "BTCUSDT",
		TechBullScore: 95,
		TechBearScore: 90,
	}

	applyDirectionAndScore(set, cfg)

	if set.Direction != 1 {
		t.Errorf("Expected weak LONG direction (+1), got %d", set.Direction)
	}

	expected := 95.0 * NeutralBandScorePenalty
	if math.Abs(set.FinalScore-expected) > 1e-9 {
		t.Errorf("FinalScore = %.4f, want %.4f (95 × %.2f)", set.FinalScore, expected, NeutralBandScorePenalty)
	}
}

func TestNeutralBand_BearDominant(t *testing.T) {
	cfg := DefaultHubConfig()

	// bull=90, bear=95: diff=-5 inside band → weak SHORT, score downgraded ×0.8
	set := &IndicatorSet{
		Symbol:        "ETHUSDT",
		TechBullScore: 90,
		TechBearScore: 95,
	}

	applyDirectionAndScore(set, cfg)

	if set.Direction != -1 {
		t.Errorf("Expected weak SHORT direction (-1), got %d", set.Direction)
	}

	expected := 95.0 * NeutralBandScorePenalty
	if math.Abs(set.FinalScore-expected) > 1e-9 {
		t.Errorf("FinalScore = %.4f, want %.4f (95 × %.2f)", set.FinalScore, expected, NeutralBandScorePenalty)
	}
}

func TestNeutralBand_ExactTieStillNeutral(t *testing.T) {
	cfg := DefaultHubConfig()

	// diff exactly 0: keeps NEUTRAL (router filters it as before), no penalty.
	set := &IndicatorSet{
		Symbol:        "SOLUSDT",
		TechBullScore: 80,
		TechBearScore: 80,
	}

	applyDirectionAndScore(set, cfg)

	if set.Direction != 0 {
		t.Errorf("Expected NEUTRAL (0) on exact tie, got %d", set.Direction)
	}
	if set.FinalScore != 80 {
		t.Errorf("FinalScore = %.4f, want 80 (no penalty on tie)", set.FinalScore)
	}
}

func TestDecisiveBand_NoPenalty(t *testing.T) {
	cfg := DefaultHubConfig()

	// diff=30 > margin=15: decisive LONG, behavior unchanged, no downgrade.
	set := &IndicatorSet{
		Symbol:        "BTCUSDT",
		TechBullScore: 95,
		TechBearScore: 65,
	}

	dir, weak := determineDirection(set, cfg)
	if dir != 1 || weak {
		t.Fatalf("Expected decisive LONG (1, weak=false), got (%d, weak=%v)", dir, weak)
	}

	applyDirectionAndScore(set, cfg)

	if set.Direction != 1 {
		t.Errorf("Expected LONG (+1), got %d", set.Direction)
	}
	if set.FinalScore != 95 {
		t.Errorf("FinalScore = %.4f, want 95 (no penalty outside neutral band)", set.FinalScore)
	}
}

func TestNeutralBandScorePenaltyValue(t *testing.T) {
	if NeutralBandScorePenalty != 0.8 {
		t.Errorf("NeutralBandScorePenalty = %.2f, want 0.80", NeutralBandScorePenalty)
	}
}
