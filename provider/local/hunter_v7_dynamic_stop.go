package local

import (
	"math"
	"time"
)

// ============================================================================
// Hunter v8 — Dynamic Stop Manager (P1-B)
// ============================================================================
// Replaces the static ATR-multiple stop-loss with an adaptive system that
// considers volatility regime, time decay, and trailing profit protection.

// DynamicStopConfig holds tunable parameters for dynamic stop management.
type DynamicStopConfig struct {
	// Volatility adaptive
	HighVolThreshold  float64 // ATR percentile > 80 = high vol
	LowVolThreshold   float64 // ATR percentile < 20 = low vol
	HighVolStopWiden  float64 // 1.3 — widen stop by 30% in high vol
	LowVolStopTighten float64 // 0.7 — tighten stop by 30% in low vol

	// Time decay (stop moves toward entry price over time)
	TimeDecayEnabled bool
	TimeDecayAfter   time.Duration // 30 minutes
	TimeDecayRate    float64       // 0.02 per minute (fraction of gap to close)

	// Trailing stop (profit protection)
	TrailingEnabled      bool
	TrailingActivationR  float64 // 0.5 — activate when MFE >= 0.5R
	TrailingDistanceR    float64 // 0.4 — trail at 0.4R from best price
}

// DefaultDynamicStopConfig returns sensible defaults.
func DefaultDynamicStopConfig() DynamicStopConfig {
	return DynamicStopConfig{
		HighVolThreshold:     80.0,
		LowVolThreshold:      20.0,
		HighVolStopWiden:     1.3,
		LowVolStopTighten:    0.7,
		TimeDecayEnabled:     true,
		TimeDecayAfter:       30 * time.Minute,
		TimeDecayRate:        0.02,
		TrailingEnabled:      true,
		TrailingActivationR:  0.5,
		TrailingDistanceR:    0.4,
	}
}

// DynamicStopManager computes adaptive stop-loss levels.
type DynamicStopManager struct {
	config DynamicStopConfig
}

// NewDynamicStopManager creates a manager with the given config.
func NewDynamicStopManager(cfg DynamicStopConfig) *DynamicStopManager {
	return &DynamicStopManager{config: cfg}
}

// DefaultDynamicStopManager returns a manager with default config.
func DefaultDynamicStopManager() *DynamicStopManager {
	return NewDynamicStopManager(DefaultDynamicStopConfig())
}

// CalcDynamicStop computes the adjusted stop-loss price.
//
// Parameters:
//   - entryPrice: original entry price
//   - baseStop: the initial stop-loss price (from signal)
//   - currentPrice: current market price
//   - maxFavorable: maximum favourable excursion as absolute price delta
//   - elapsed: time since signal entry
//   - atrPercentile: current ATR's percentile rank (0-100)
//   - isLong: true for LONG positions
//
// Returns the most protective adjusted stop price.
func (dsm *DynamicStopManager) CalcDynamicStop(
	entryPrice, baseStop, currentPrice, maxFavorable float64,
	elapsed time.Duration, atrPercentile float64,
	isLong bool,
) float64 {
	if dsm == nil || entryPrice <= 0 || baseStop <= 0 {
		return baseStop
	}

	risk := math.Abs(entryPrice - baseStop)
	if risk <= 0 {
		return baseStop
	}

	// 1. Volatility-adjusted stop
	volMultiplier := 1.0
	if atrPercentile > dsm.config.HighVolThreshold {
		volMultiplier = dsm.config.HighVolStopWiden
	} else if atrPercentile < dsm.config.LowVolThreshold {
		volMultiplier = dsm.config.LowVolStopTighten
	}

	var adjustedStop float64
	if isLong {
		// LONG: stop below entry, widen means further down
		adjustedStop = entryPrice - risk*volMultiplier
	} else {
		adjustedStop = entryPrice + risk*volMultiplier
	}

	// 2. Time decay: after TimeDecayAfter, stop creeps toward entry
	if dsm.config.TimeDecayEnabled && elapsed > dsm.config.TimeDecayAfter {
		decayMinutes := elapsed.Minutes() - dsm.config.TimeDecayAfter.Minutes()
		decayFactor := decayMinutes * dsm.config.TimeDecayRate
		if decayFactor > 0.5 {
			decayFactor = 0.5 // cap at 50% of the gap
		}
		if isLong {
			gap := entryPrice - adjustedStop
			adjustedStop += gap * decayFactor
		} else {
			gap := adjustedStop - entryPrice
			adjustedStop -= gap * decayFactor
		}
	}

	// 3. Trailing stop: when MFE exceeds activation threshold
	if dsm.config.TrailingEnabled && maxFavorable > 0 {
		activationDelta := risk * dsm.config.TrailingActivationR
		if maxFavorable >= activationDelta {
			trailingDist := risk * dsm.config.TrailingDistanceR
			if isLong {
				trailingStop := currentPrice - trailingDist
				if trailingStop > adjustedStop {
					adjustedStop = trailingStop
				}
			} else {
				trailingStop := currentPrice + trailingDist
				if trailingStop < adjustedStop {
					adjustedStop = trailingStop
				}
			}
		}
	}

	// For LONG, return the highest (most protective) stop
	// For SHORT, return the lowest (most protective) stop
	if isLong {
		return math.Max(adjustedStop, 0)
	}
	return math.Max(adjustedStop, 0)
}
