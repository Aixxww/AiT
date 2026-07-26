package local

// ============================================================================
// Hunter v7 — Regime × Module Weight Matrix
// ============================================================================
// Adjusts signal module weights based on current market regime.
// This is the core intelligence: the same setup type gets different
// weights depending on whether the market is trending, ranging, panicking, etc.

// regimeModuleWeight returns a multiplier (0.0-1.5) for a given module+regime pair.
// Values > 1.0 boost the module; < 1.0 suppress it; 0.0 disables it.
func regimeModuleWeight(regime V7MarketRegime, setup V7SetupType) float64 {
	key := regimeWeightKey{regime, setup}
	if w, ok := regimeWeightMatrix[key]; ok {
		return w
	}
	return 1.0 // default: no adjustment
}

type regimeWeightKey struct {
	regime V7MarketRegime
	setup  V7SetupType
}

// regimeWeightMatrix is the 8×10 weight lookup table.
// Designed by analyzing which setups work in which market conditions.
var regimeWeightMatrix = map[regimeWeightKey]float64{
	// ===== trend_up: Strong bull market =====
	{V7RegimeTrendUp, V7SetupPullbackLong}:       1.1, // Buy dips in uptrend
	{V7RegimeTrendUp, V7SetupShortSqueezeLong}:   1.0, // Normal
	{V7RegimeTrendUp, V7SetupTrendBreakoutLong}:  1.2, // Breakouts thrive in trends
	{V7RegimeTrendUp, V7SetupLeaderMomentumLong}: 1.2, // Leaders accelerate
	{V7RegimeTrendUp, V7SetupPanicReversalLong}:  0.5, // Unlikely in uptrend
	{V7RegimeTrendUp, V7SetupAccumulationLong}:   0.8, // Less relevant
	{V7RegimeTrendUp, V7SetupDistributionShort}:  0.6, // Don't short in uptrend
	{V7RegimeTrendUp, V7SetupLongSqueezeShort}:   0.5, // Unlikely
	{V7RegimeTrendUp, V7SetupBreakdownShort}:     0.4, // Counter-trend breakdowns need stronger proof
	{V7RegimeTrendUp, V7SetupRangeReversion}:     0.4, // Ranging doesn't apply
	{V7RegimeTrendUp, V7SetupFundingReversal}:    0.7, // Lower priority
	{V7RegimeTrendUp, V7SetupDisplacementLong}:   1.1, // Displacement in uptrend = strong
	{V7RegimeTrendUp, V7SetupRangeExpansion}:     1.1, // Event continuation in uptrend
	{V7RegimeTrendUp, V7SetupAltLadderLong}:      1.2, // Alt leaders ladder higher in uptrends
	{V7RegimeTrendUp, V7SetupAltLadderShort}:     0.5, // Only confirmed alt breakdowns
	{V7RegimeTrendUp, V7SetupMMSBottomWakeLong}:  0.6,
	{V7RegimeTrendUp, V7SetupMMSTrendRideLong}:   1.2,
	{V7RegimeTrendUp, V7SetupMMSSqueezeLong}:     1.2,

	// ===== trend_down: Bear market =====
	{V7RegimeTrendDown, V7SetupPullbackLong}:       0.6, // Risky to buy dips in downtrend
	{V7RegimeTrendDown, V7SetupShortSqueezeLong}:   0.8, // Short squeeze bounces possible
	{V7RegimeTrendDown, V7SetupTrendBreakoutLong}:  0.3, // Breakouts rare in downtrend
	{V7RegimeTrendDown, V7SetupLeaderMomentumLong}: 0.4, // Few leaders in bear
	{V7RegimeTrendDown, V7SetupPanicReversalLong}:  1.2, // Panic V-reversals thrive
	{V7RegimeTrendDown, V7SetupAccumulationLong}:   1.0, // Smart money accumulates in bear
	{V7RegimeTrendDown, V7SetupDistributionShort}:  1.2, // Distribution shorts work
	{V7RegimeTrendDown, V7SetupLongSqueezeShort}:   1.1, // Long squeezes cascade
	{V7RegimeTrendDown, V7SetupBreakdownShort}:     1.3, // Downside continuation is primary
	{V7RegimeTrendDown, V7SetupRangeReversion}:     0.6, // Less reliable
	{V7RegimeTrendDown, V7SetupFundingReversal}:    1.0, // Crowding reversals work
	{V7RegimeTrendDown, V7SetupDisplacementLong}:   0.6, // Risky displacement in bear
	{V7RegimeTrendDown, V7SetupRangeExpansion}:     1.0, // Downside event continuation works
	{V7RegimeTrendDown, V7SetupAltLadderLong}:      0.5, // Counter-trend alt pumps need stronger proof
	{V7RegimeTrendDown, V7SetupAltLadderShort}:     1.3, // Alt losers staircase lower
	{V7RegimeTrendDown, V7SetupMMSBottomWakeLong}:  0.6,
	{V7RegimeTrendDown, V7SetupMMSTrendRideLong}:   0.4,
	{V7RegimeTrendDown, V7SetupMMSSqueezeLong}:     0.5,

	// ===== range: Sideways market =====
	{V7RegimeRange, V7SetupPullbackLong}:       1.0, // Support bounces work
	{V7RegimeRange, V7SetupShortSqueezeLong}:   0.8, // Squeezes less common
	{V7RegimeRange, V7SetupTrendBreakoutLong}:  0.5, // No trend to break
	{V7RegimeRange, V7SetupLeaderMomentumLong}: 0.3, // No momentum in range
	{V7RegimeRange, V7SetupPanicReversalLong}:  0.4, // No panic in range
	{V7RegimeRange, V7SetupAccumulationLong}:   1.1, // Accumulation before breakout
	{V7RegimeRange, V7SetupDistributionShort}:  0.8, // Upper range shorts OK
	{V7RegimeRange, V7SetupLongSqueezeShort}:   0.6, // Less relevant
	{V7RegimeRange, V7SetupBreakdownShort}:     0.8, // Range breaks can continue
	{V7RegimeRange, V7SetupRangeReversion}:     1.3, // KING of range markets
	{V7RegimeRange, V7SetupFundingReversal}:    1.1, // Good in range
	{V7RegimeRange, V7SetupDisplacementLong}:   0.7, // Range displacement less reliable
	{V7RegimeRange, V7SetupRangeExpansion}:     0.9, // Only strong event breaks range
	{V7RegimeRange, V7SetupAltLadderLong}:      0.9, // Early alt movers only
	{V7RegimeRange, V7SetupAltLadderShort}:     0.9, // Range breaks can trend briefly
	{V7RegimeRange, V7SetupMMSBottomWakeLong}:  1.2,
	{V7RegimeRange, V7SetupMMSTrendRideLong}:   0.7,
	{V7RegimeRange, V7SetupMMSSqueezeLong}:     0.8,

	// ===== panic_dump: Crash =====
	{V7RegimePanicDump, V7SetupPullbackLong}:       0.3, // Too early for dips
	{V7RegimePanicDump, V7SetupShortSqueezeLong}:   1.2, // Squeeze bounces after liquidation
	{V7RegimePanicDump, V7SetupTrendBreakoutLong}:  0.2, // No breakouts in panic
	{V7RegimePanicDump, V7SetupLeaderMomentumLong}: 0.1, // Everything crashing
	{V7RegimePanicDump, V7SetupPanicReversalLong}:  1.3, // PRIMARY module in panic
	{V7RegimePanicDump, V7SetupAccumulationLong}:   0.5, // Too early
	{V7RegimePanicDump, V7SetupDistributionShort}:  0.4, // Distribution already happened
	{V7RegimePanicDump, V7SetupLongSqueezeShort}:   1.1, // Cascade continues
	{V7RegimePanicDump, V7SetupBreakdownShort}:     1.2, // Capture continuation, avoid exhausted lows in module
	{V7RegimePanicDump, V7SetupRangeReversion}:     0.2, // No range in panic
	{V7RegimePanicDump, V7SetupFundingReversal}:    1.2, // Extreme funding during panic
	{V7RegimePanicDump, V7SetupDisplacementLong}:   0.3, // Displacement long in crash = trap
	{V7RegimePanicDump, V7SetupRangeExpansion}:     1.1, // Short-side event continuation
	{V7RegimePanicDump, V7SetupAltLadderLong}:      0.2, // Avoid catching knives
	{V7RegimePanicDump, V7SetupAltLadderShort}:     1.2, // Capture continuation before exhaustion
	{V7RegimePanicDump, V7SetupMMSBottomWakeLong}:  0.4,
	{V7RegimePanicDump, V7SetupMMSTrendRideLong}:   0.2,
	{V7RegimePanicDump, V7SetupMMSSqueezeLong}:     0.3,

	// ===== market_pullback: broad selloff, not a full panic cascade =====
	{V7RegimePullback, V7SetupPullbackLong}:       1.0, // Support bounces only after confirmation
	{V7RegimePullback, V7SetupShortSqueezeLong}:   1.0, // Rebound squeezes possible
	{V7RegimePullback, V7SetupTrendBreakoutLong}:  0.5, // Breakouts are lower quality
	{V7RegimePullback, V7SetupLeaderMomentumLong}: 0.4, // Avoid broad chase in risk-off
	{V7RegimePullback, V7SetupPanicReversalLong}:  1.1, // Watch reclaim after flush
	{V7RegimePullback, V7SetupAccumulationLong}:   1.1, // Institutions may absorb weakness
	{V7RegimePullback, V7SetupDistributionShort}:  0.9, // Still valid but less than euphoric tops
	{V7RegimePullback, V7SetupLongSqueezeShort}:   1.2, // Primary risk in broad pullbacks
	{V7RegimePullback, V7SetupBreakdownShort}:     1.2, // Downside continuation during pullbacks
	{V7RegimePullback, V7SetupRangeReversion}:     0.6, // Ranges break during pullbacks
	{V7RegimePullback, V7SetupFundingReversal}:    1.1, // Crowded positioning unwinds
	{V7RegimePullback, V7SetupDisplacementLong}:   0.5, // Displacement during broad pullback risky
	{V7RegimePullback, V7SetupRangeExpansion}:     1.0, // Selective expansion during pullback
	{V7RegimePullback, V7SetupAltLadderLong}:      0.6, // Only strong relative alt leaders
	{V7RegimePullback, V7SetupAltLadderShort}:     1.2, // Alt breakdowns accelerate in pullbacks
	{V7RegimePullback, V7SetupMMSBottomWakeLong}:  0.9,
	{V7RegimePullback, V7SetupMMSTrendRideLong}:   0.7,
	{V7RegimePullback, V7SetupMMSSqueezeLong}:     0.6,

	// ===== mania_pump: Euphoria =====
	{V7RegimeManiaPump, V7SetupPullbackLong}:       0.5, // Shallow dips in mania
	{V7RegimeManiaPump, V7SetupShortSqueezeLong}:   1.0, // Squeezes common
	{V7RegimeManiaPump, V7SetupTrendBreakoutLong}:  1.1, // Breakouts work
	{V7RegimeManiaPump, V7SetupLeaderMomentumLong}: 1.3, // Leaders EXPLODE
	{V7RegimeManiaPump, V7SetupPanicReversalLong}:  0.2, // No panic
	{V7RegimeManiaPump, V7SetupAccumulationLong}:   0.4, // Not accumulating, buying
	{V7RegimeManiaPump, V7SetupDistributionShort}:  1.2, // Watch for blow-off tops
	{V7RegimeManiaPump, V7SetupLongSqueezeShort}:   0.3, // Squeezes up, not down
	{V7RegimeManiaPump, V7SetupBreakdownShort}:     0.5, // Only confirmed breakdowns
	{V7RegimeManiaPump, V7SetupRangeReversion}:     0.2, // No range
	{V7RegimeManiaPump, V7SetupFundingReversal}:    1.3, // Funding EXTREME
	{V7RegimeManiaPump, V7SetupDisplacementLong}:   1.2, // Displacement thrives in mania
	{V7RegimeManiaPump, V7SetupRangeExpansion}:     1.2, // Event continuation but watch chase risk
	{V7RegimeManiaPump, V7SetupAltLadderLong}:      1.3, // Primary alt ladder regime
	{V7RegimeManiaPump, V7SetupAltLadderShort}:     0.5, // Only confirmed post-blowoff breaks
	{V7RegimeManiaPump, V7SetupMMSBottomWakeLong}:  0.4,
	{V7RegimeManiaPump, V7SetupMMSTrendRideLong}:   1.0,
	{V7RegimeManiaPump, V7SetupMMSSqueezeLong}:     1.3,

	// ===== compression: Pre-breakout =====
	{V7RegimeCompression, V7SetupPullbackLong}:       0.9, // Possible
	{V7RegimeCompression, V7SetupShortSqueezeLong}:   1.0, // Normal
	{V7RegimeCompression, V7SetupTrendBreakoutLong}:  1.3, // PRIMARY: breakouts from compression
	{V7RegimeCompression, V7SetupLeaderMomentumLong}: 0.5, // No momentum yet
	{V7RegimeCompression, V7SetupPanicReversalLong}:  0.3, // Unlikely
	{V7RegimeCompression, V7SetupAccumulationLong}:   1.3, // Accumulation is compression
	{V7RegimeCompression, V7SetupDistributionShort}:  0.6, // Possible but less likely
	{V7RegimeCompression, V7SetupLongSqueezeShort}:   0.5, // Unlikely
	{V7RegimeCompression, V7SetupBreakdownShort}:     0.9, // Compression can break down
	{V7RegimeCompression, V7SetupRangeReversion}:     1.0, // Could be ranging
	{V7RegimeCompression, V7SetupFundingReversal}:    0.8, // Less relevant
	{V7RegimeCompression, V7SetupDisplacementLong}:   1.2, // Displacement from compression = breakout
	{V7RegimeCompression, V7SetupRangeExpansion}:     1.2, // Fresh range expansion after compression
	// ===== rotation: Sector rotation =====
	{V7RegimeRotation, V7SetupPullbackLong}:         1.0, // Sector pullbacks
	{V7RegimeRotation, V7SetupShortSqueezeLong}:     1.0, // Normal
	{V7RegimeRotation, V7SetupTrendBreakoutLong}:    0.8, // Less directional
	{V7RegimeRotation, V7SetupLeaderMomentumLong}:   1.2, // New leaders emerge
	{V7RegimeRotation, V7SetupPanicReversalLong}:    0.5, // Unlikely
	{V7RegimeRotation, V7SetupAccumulationLong}:     1.0, // Rotating into new sectors
	{V7RegimeRotation, V7SetupDistributionShort}:    1.0, // Rotating out of old
	{V7RegimeRotation, V7SetupLongSqueezeShort}:     0.8, // Some squeezes
	{V7RegimeRotation, V7SetupRangeReversion}:       0.8, // Less relevant
	{V7RegimeRotation, V7SetupFundingReversal}:      1.0, // Normal
	{V7RegimeRotation, V7SetupDisplacementLong}:     1.0, // Sector displacement during rotation
	{V7RegimeRotation, V7SetupRangeExpansion}:       1.1, // Sector event movers
	{V7RegimeCompression, V7SetupAltLadderLong}:     1.0, // Fresh alt ignition
	{V7RegimeCompression, V7SetupAltLadderShort}:    0.8, // Breakdown from compression
	{V7RegimeCompression, V7SetupMMSBottomWakeLong}: 1.3,
	{V7RegimeCompression, V7SetupMMSTrendRideLong}:  0.8,
	{V7RegimeCompression, V7SetupMMSSqueezeLong}:    0.8,
	{V7RegimeRotation, V7SetupBreakdownShort}:       1.1,  // Rotation losers can trend down
	{V7RegimeRotation, V7SetupAltLadderLong}:        1.25, // Sector alt leaders
	{V7RegimeRotation, V7SetupAltLadderShort}:       1.15, // Rotation losers
	{V7RegimeRotation, V7SetupMMSBottomWakeLong}:    1.2,
	{V7RegimeRotation, V7SetupMMSTrendRideLong}:     1.1,
	{V7RegimeRotation, V7SetupMMSSqueezeLong}:       1.2,

	// ===== mixed: No clear regime =====
	{V7RegimeMixed, V7SetupPullbackLong}:       1.0, // Default weight
	{V7RegimeMixed, V7SetupShortSqueezeLong}:   1.0,
	{V7RegimeMixed, V7SetupTrendBreakoutLong}:  1.0,
	{V7RegimeMixed, V7SetupLeaderMomentumLong}: 1.0,
	{V7RegimeMixed, V7SetupPanicReversalLong}:  1.0,
	{V7RegimeMixed, V7SetupAccumulationLong}:   1.0,
	{V7RegimeMixed, V7SetupDistributionShort}:  1.0,
	{V7RegimeMixed, V7SetupLongSqueezeShort}:   1.0,
	{V7RegimeMixed, V7SetupBreakdownShort}:     1.0,
	{V7RegimeMixed, V7SetupRangeReversion}:     1.0,
	{V7RegimeMixed, V7SetupFundingReversal}:    1.0,
	{V7RegimeMixed, V7SetupDisplacementLong}:   1.0,
	{V7RegimeMixed, V7SetupRangeExpansion}:     1.0,
	// ===== v8 new modules =====
	// Intraday Scalp: thrives in range/compression with volatility
	{V7RegimeTrendUp, V7SetupIntradayScalp}:     1.0,
	{V7RegimeTrendDown, V7SetupIntradayScalp}:   0.5,
	{V7RegimeRange, V7SetupIntradayScalp}:       1.1,
	{V7RegimePanicDump, V7SetupIntradayScalp}:   0.3,
	{V7RegimePullback, V7SetupIntradayScalp}:    0.8,
	{V7RegimeManiaPump, V7SetupIntradayScalp}:   0.6,
	{V7RegimeCompression, V7SetupIntradayScalp}: 1.2,
	{V7RegimeRotation, V7SetupIntradayScalp}:    1.0,
	{V7RegimeMixed, V7SetupIntradayScalp}:       0.9,
	// Volatility Squeeze Breakout: best in compression/range
	{V7RegimeTrendUp, V7SetupVolatilitySqueeze}:     0.9,
	{V7RegimeTrendDown, V7SetupVolatilitySqueeze}:   0.6,
	{V7RegimeRange, V7SetupVolatilitySqueeze}:       1.2,
	{V7RegimePanicDump, V7SetupVolatilitySqueeze}:   0.4,
	{V7RegimePullback, V7SetupVolatilitySqueeze}:    0.8,
	{V7RegimeManiaPump, V7SetupVolatilitySqueeze}:   0.5,
	{V7RegimeCompression, V7SetupVolatilitySqueeze}: 1.3,
	{V7RegimeRotation, V7SetupVolatilitySqueeze}:    1.0,
	{V7RegimeMixed, V7SetupVolatilitySqueeze}:       0.9,
	// Whale Flow Reversal: best in rotation/range
	{V7RegimeTrendUp, V7SetupWhaleFlow}:       0.8,
	{V7RegimeTrendDown, V7SetupWhaleFlow}:     0.7,
	{V7RegimeRange, V7SetupWhaleFlow}:         1.1,
	{V7RegimePanicDump, V7SetupWhaleFlow}:     0.5,
	{V7RegimePullback, V7SetupWhaleFlow}:      0.9,
	{V7RegimeManiaPump, V7SetupWhaleFlow}:     0.6,
	{V7RegimeCompression, V7SetupWhaleFlow}:   1.0,
	{V7RegimeRotation, V7SetupWhaleFlow}:      1.2,
	{V7RegimeMixed, V7SetupWhaleFlow}:         0.9,
	{V7RegimeMixed, V7SetupAltLadderLong}:     1.0,
	{V7RegimeMixed, V7SetupAltLadderShort}:    1.0,
	{V7RegimeMixed, V7SetupMMSBottomWakeLong}: 1.0,
	{V7RegimeMixed, V7SetupMMSTrendRideLong}:  1.0,
	{V7RegimeMixed, V7SetupMMSSqueezeLong}:    1.0,
}
