package local

import "testing"

// Issue #74 (Q2 / AUD-A2): trend_continuation exists in the code enum, has a
// defined detection rule, and its consumers behave as they did when these
// markets were labelled compression.

func TestTrendContinuationDetection(t *testing.T) {
	// Uptrend structure on both majors, 24h pulled back, ADX not confirming.
	pauseUp := regimeAnalysis{hasData: true, adx: 18, bbWidth: 3, change24h: -1.5, ema20_4h: 101, ema60_4h: 100}
	pauseDown := regimeAnalysis{hasData: true, adx: 18, bbWidth: 3, change24h: 1.5, ema20_4h: 99, ema60_4h: 100}
	flat := regimeAnalysis{hasData: true, adx: 18, bbWidth: 3, change24h: 0.5, ema20_4h: 100, ema60_4h: 100}

	if isTrendUp(pauseUp, pauseUp) || isTrendDown(pauseUp, pauseUp) {
		t.Fatal("fixture must not already be trend_up/trend_down")
	}
	if !isTrendContinuation(pauseUp, pauseUp) || !isTrendContinuation(pauseDown, pauseDown) {
		t.Fatal("aligned EMA structure with an unconfirmed 24h move must be trend_continuation")
	}
	if isTrendContinuation(pauseUp, pauseDown) {
		t.Fatal("majors with opposite structures are not a continuation")
	}
	if isTrendContinuation(flat, flat) || !isCompression(flat, flat) {
		t.Fatal("no EMA structure stays compression")
	}
	strong := pauseUp
	strong.adx = 30
	if isTrendContinuation(strong, strong) {
		t.Fatal("trend_continuation is a subset of the compression condition (ADX < 25)")
	}
	missing := pauseUp
	missing.ema60_4h = 0
	if isTrendContinuation(missing, pauseUp) {
		t.Fatal("missing EMA data must not detect a continuation")
	}
}

func TestTrendContinuationIsInTheEnum(t *testing.T) {
	found := false
	seen := map[V7MarketRegime]bool{}
	for _, r := range V7MarketRegimes() {
		if seen[r] {
			t.Fatalf("duplicate regime %s", r)
		}
		seen[r] = true
		found = found || r == V7RegimeTrendContinuation
	}
	if !found || V7RegimeTrendContinuation != "trend_continuation" || len(seen) != 10 {
		t.Fatalf("regime enum = %v", V7MarketRegimes())
	}
}

func TestTrendContinuationConsumersMirrorCompression(t *testing.T) {
	setups := []V7SetupType{
		V7SetupPullbackLong, V7SetupShortSqueezeLong, V7SetupTrendBreakoutLong, V7SetupLeaderMomentumLong,
		V7SetupPanicReversalLong, V7SetupAccumulationLong, V7SetupDistributionShort, V7SetupLongSqueezeShort,
		V7SetupBreakdownShort, V7SetupAltLadderLong, V7SetupAltLadderShort, V7SetupMMSBottomWakeLong,
		V7SetupMMSTrendRideLong, V7SetupRelativeWeaknessShort, V7SetupMMSSqueezeLong, V7SetupRangeReversion,
		V7SetupFundingReversal, V7SetupPreBreakoutWatch, V7SetupPreSqueezeWatch, V7SetupPreDistribution,
		V7SetupAccumulationWatch, V7SetupDisplacementLong, V7SetupRangeExpansion, V7SetupIntradayScalp,
		V7SetupVolatilitySqueeze, V7SetupWhaleFlow,
	}
	for _, s := range setups {
		if a, b := regimeModuleWeight(V7RegimeTrendContinuation, s), regimeModuleWeight(V7RegimeCompression, s); a != b {
			t.Errorf("%s weight %.2f, compression %.2f", s, a, b)
		}
	}
	if regimeTPMultiplier[V7RegimeTrendContinuation] != regimeTPMultiplier[V7RegimeCompression] {
		t.Error("TP2 multiplier must match compression")
	}
	if !v7RegimeActsAsCompression(V7RegimeTrendContinuation) || v7RegimeActsAsCompression(V7RegimeRange) {
		t.Error("gating helper")
	}
	ctx := &V7SymbolContext{}
	if regimeModuleProxy(ctx, V7RegimeTrendContinuation) != regimeModuleProxy(ctx, V7RegimeCompression) {
		t.Error("pre-move radar proxy must match compression")
	}
}
