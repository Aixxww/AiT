package local

import (
	"log"
	"time"

	"github.com/Aixxww/AiT/datafetch"
)

// ============================================================================
// Hunter v7 — Main Entry Point
// ============================================================================
// ScoreHunterV7 is the top-level function that runs the entire v7 pipeline:
//   1. Build multi-source candidate universe from snapshot
//   2. Detect market regime from BTC/ETH
//   3. Route all symbols through all signal modules
//   4. Return structured, sorted signals ready for AI decision

// ScoreHunterV7 runs the complete Hunter v7 pipeline on a snapshot.
// Returns a sorted slice of V7SignalOutput, highest AI priority first.
func ScoreHunterV7(snap *datafetch.Snapshot, cfg V7Config) []V7SignalOutput {
	return ScoreHunterV7Detailed(snap, cfg).Signals
}

// ScoreHunterV7Detailed runs the complete Hunter v7 pipeline and returns raw
// router output alongside the final LLM-facing signals.
func ScoreHunterV7Detailed(snap *datafetch.Snapshot, cfg V7Config) V7ScoreResult {
	start := time.Now()
	result := V7ScoreResult{}
	if snap == nil || len(snap.Symbols) == 0 {
		log.Printf("🔴 Hunter v7: empty snapshot, skipping")
		return result
	}

	// Step 1: Build universe
	universe := BuildV7Universe(snap)
	if len(universe) == 0 {
		log.Printf("⚠️ Hunter v7: 0 symbols in universe")
		return result
	}
	result.Universe = universe

	// Step 2: Detect regime
	regime := DetectV7MarketRegime(snap)
	result.Regime = regime

	// Step 3: Route through all modules
	router := NewV7Router()
	route := router.RouteDetailed(universe, regime, cfg)
	signals := route.OutputSignals
	rawSignals := route.RawSignals

	// Step 4: Cross-cycle watch state upgrade (if state manager is configured)
	if cfg.WatchStateManager != nil {
		upgraded := cfg.WatchStateManager.Process(signals, cfg.CycleNumber)
		if len(upgraded) > 0 {
			signals = mergeV7SignalUpgrades(signals, upgraded)
			rawSignals = mergeV7SignalUpgrades(rawSignals, upgraded)
			log.Printf("🔼 Hunter v7: %d watch signals upgraded by state manager", len(upgraded))
		}
	}

	elapsed := time.Since(start)
	log.Printf("🎯 Hunter v7: %d symbols → %d signals (regime=%s, elapsed=%v)",
		len(universe), len(signals), regime, elapsed)

	// Log top signals for debugging
	for i, sig := range signals {
		if i >= 5 {
			break
		}
		log.Printf("  #%d %s %s [%s] setup=%.1f risk=%.1f timing=%.1f liquidity=%.1f priority=%.1f status=%s",
			i+1, sig.Symbol, sig.Direction, sig.SetupType,
			sig.SetupScore, sig.RiskScore, sig.TimingScore,
			sig.LiquidityScore, sig.AIPriority, sig.Status)
	}

	result.RawSignals = rawSignals
	result.Signals = signals
	return result
}

func mergeV7SignalUpgrades(signals, upgraded []V7SignalOutput) []V7SignalOutput {
	if len(upgraded) == 0 {
		return signals
	}
	upgradeMap := make(map[string]V7SignalOutput, len(upgraded))
	for _, u := range upgraded {
		upgradeMap[u.Symbol+"|"+string(u.SetupType)] = u
	}
	merged := append([]V7SignalOutput{}, signals...)
	for i, sig := range merged {
		key := sig.Symbol + "|" + string(sig.SetupType)
		if u, ok := upgradeMap[key]; ok {
			merged[i] = u
			delete(upgradeMap, key)
		}
	}
	for _, u := range upgradeMap {
		merged = append(merged, u)
	}
	return merged
}

// V7FilterByDirection filters v7 signals by the requested direction.
// Returns only signals matching the direction filter (or all if direction is empty/"BOTH").
// The CandidateCoin conversion should be done in the kernel layer.
func V7FilterByDirection(signals []V7SignalOutput, direction string) []V7SignalOutput {
	if direction == "" || direction == "BOTH" {
		return signals
	}
	var filtered []V7SignalOutput
	for _, sig := range signals {
		if string(sig.Direction) == direction {
			filtered = append(filtered, sig)
		}
	}
	return filtered
}

// V7ConfidenceToCapitalLevel maps v7 confidence letter to the v6 capital level constants.
func V7ConfidenceToCapitalLevel(conf string) (int, string) {
	switch conf {
	case "A":
		return CapitalLevelStrong, "Tier-S PRIME SIGNAL"
	case "B":
		return CapitalLevelMed, "Tier-A"
	case "C":
		return CapitalLevelWeak, "Tier-B LOW CONFIDENCE"
	default:
		return CapitalLevelNone, "Untiered"
	}
}

// Note: V7SignalOutput → CandidateCoin conversion lives in kernel/engine.go
// since CandidateCoin is defined in the kernel package.
