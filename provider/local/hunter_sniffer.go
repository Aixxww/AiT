package local

import (
	"fmt"
	"log"

	"nofx/provider/nofxos"
)

// ============================================================================
// Hunter Sniffer — Institutional Ambush Pattern Detector
// ============================================================================
// Post-Hunter middleware that filters raw scored coins through 3-condition
// resonance logic to detect:
//   - LONG Ambush: smart money accumulating in low-volatility zones
//   - SHORT Distribution: smart money exiting at highs
//
// Filter gates (each direction):
//   1. Direction & score quality
//   2. Volatility squeeze (bb_squeeze_15m) — MUST
//   3. Smart money footprint (accumulation/distribution signals) — ANY ONE
//   4. Wall filter (no opposing S/R nearby) — MUST NOT
//   5. Wash trade filter — MUST NOT
//
// All tags already exist in Hunter — no new computation needed.

// AmbushType classifies the institutional pattern detected.
type AmbushType string

const (
	LongAmbush        AmbushType = "LONG_AMBUSH"
	ShortDistribution AmbushType = "SHORT_DISTRIBUTION"
)

// MinScoreSniffer is the minimum directional score for sniff mode candidates.
// Sniff mode requires higher quality than the base Hunter floor (10.0) because
// the tag-based filters already provide signal specificity.
const MinScoreSniffer = 20.0

// AmbushCandidate represents a coin that passed the sniff filter.
type AmbushCandidate struct {
	Symbol     string
	Coin       nofxos.CoinData // Original Hunter data
	Meta       *HunterCoinMeta // Bidirectional scores+tags
	AmbushType AmbushType
	Reasons    []string // Which conditions passed (for log/backtest)
}

// SnifferResult holds the filtered output from the sniff mode.
type SnifferResult struct {
	LongAmbush []AmbushCandidate
	ShortDist  []AmbushCandidate
	Stats      SnifferStats
}

// SnifferStats tracks filter statistics for logging and backtesting.
type SnifferStats struct {
	TotalScanned         int
	LongPassed           int
	ShortPassed          int
	BlockedByDirection   int // Direction mismatch
	BlockedByScore       int // Score too low
	BlockedBySqueeze     int // Missing bb_squeeze_15m
	BlockedBySignal      int // No accumulation/distribution signal
	BlockedByWall        int // near_resistance or near_support blocking
	BlockedByWash        int // Wash trade detected
}

// ============================================================================
// Main Entry Point
// ============================================================================

// FilterAmbushCandidates filters Hunter output for institutional ambush patterns.
// Input: raw coin list from GetHunterList(), coinMeta from GetHunterCoinsWithData().
// Output: SnifferResult with separated Long Ambush and Short Distribution pools.
func (c *Client) FilterAmbushCandidates(
	coins []nofxos.CoinData,
	coinMeta map[string]*HunterCoinMeta,
) *SnifferResult {
	result := &SnifferResult{
		Stats: SnifferStats{TotalScanned: len(coins)},
	}

	for _, coin := range coins {
		meta, ok := coinMeta[normalizeSymbol(coin.Pair)]
		if !ok {
			log.Printf("[Sniffer DEBUG] %s skipped: no coinMeta found", coin.Pair)
			continue
		}

		// Try LONG Ambush pipeline
		if cand := filterLongAmbush(coin, meta); cand != nil {
			result.LongAmbush = append(result.LongAmbush, *cand)
			result.Stats.LongPassed++
		}

		// Try SHORT Distribution pipeline (independent of LONG result)
		if cand := filterShortDistribution(coin, meta); cand != nil {
			result.ShortDist = append(result.ShortDist, *cand)
			result.Stats.ShortPassed++
		}
	}

	// Aggregate stats: count unique blocks across both pipelines
	// (each coin is evaluated for both, so we track the dominant block reason)
	for _, coin := range coins {
		meta, ok := coinMeta[normalizeSymbol(coin.Pair)]
		if !ok {
			continue
		}
		blockReason := classifyBlockReason(coin, meta)
		switch blockReason {
		case "direction":
			result.Stats.BlockedByDirection++
		case "score":
			result.Stats.BlockedByScore++
		case "squeeze":
			result.Stats.BlockedBySqueeze++
		case "signal":
			result.Stats.BlockedBySignal++
		case "wall":
			result.Stats.BlockedByWall++
		case "wash":
			result.Stats.BlockedByWash++
		case "passed":
			// already counted in LongPassed/ShortPassed
		}
	}

	// Log summary
	log.Printf("[Sniffer] Scanned %d coins → LONG_AMBUSH: %d passed, SHORT_DIST: %d passed",
		result.Stats.TotalScanned, result.Stats.LongPassed, result.Stats.ShortPassed)
	log.Printf("[Sniffer] Blocked: direction=%d, score=%d, squeeze=%d, signal=%d, wall=%d, wash=%d",
		result.Stats.BlockedByDirection, result.Stats.BlockedByScore,
		result.Stats.BlockedBySqueeze, result.Stats.BlockedBySignal,
		result.Stats.BlockedByWall, result.Stats.BlockedByWash)

	return result
}

// ============================================================================
// LONG Ambush Filter — Smart Money Accumulation Detector
// ============================================================================

func filterLongAmbush(coin nofxos.CoinData, meta *HunterCoinMeta) *AmbushCandidate {
	sym := coin.Pair

	// Gate 1: Direction & Score Quality
	if meta.Direction != "LONG" {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: direction=%s not LONG", sym, meta.Direction)
		return nil
	}
	if meta.LongScore < MinScoreSniffer {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: long_score %.1f < %.1f", sym, meta.LongScore, MinScoreSniffer)
		return nil
	}

	// Gate 2: Volatility Squeeze (REQUIRED)
	if !hasTag(meta.LongTags, "bb_squeeze_15m") {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: no bb_squeeze_15m (tags: %v)", sym, meta.LongTags)
		return nil
	}

	// Gate 3: Smart Money Footprint (ANY ONE required)
	allLongSignals := []string{"oi_accumulation", "taker_sustained_buying", "lsr_reversal"}
	passedSignal := ""
	for _, sig := range allLongSignals {
		if hasTag(meta.LongTags, sig) {
			passedSignal = sig
			break
		}
	}
	if passedSignal == "" {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: no accumulation signal (tags: %v)", sym, meta.LongTags)
		return nil
	}

	// Gate 4: Anti-Crash-Wall Filter (MUST NOT have resistance overhead)
	// Check BOTH LongTags and ShortTags — near_resistance tags live in ShortTags
	combinedTags := mergeTags(meta.LongTags, meta.ShortTags)
	if hasTag(combinedTags, "near_resistance_4h") {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: near_resistance_4h in tags", sym)
		return nil
	}
	if hasTag(combinedTags, "near_resistance_1d") {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: near_resistance_1d in tags", sym)
		return nil
	}

	// Gate 5: Wash Trade Killer (MUST NOT)
	if hasTag(combinedTags, "wash_micro_trades") {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: wash_micro_trades detected", sym)
		return nil
	}
	if hasTag(combinedTags, "wash_fake_volume") {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: wash_fake_volume detected", sym)
		return nil
	}

	// ALL GATES PASSED
	reasons := []string{
		"bb_squeeze_15m:volatility_ice",
		fmt.Sprintf("%s:smart_money_entry", passedSignal),
	}
	if hasTag(combinedTags, "near_support_4h") {
		reasons = append(reasons, "near_support_4h:support_zone")
	}
	if hasTag(combinedTags, "near_support_1d") {
		reasons = append(reasons, "near_support_1d:daily_support")
	}

	log.Printf("[Sniffer] %s LONG_AMBUSH PASSED (score=%.1f, reasons=%v)", sym, meta.LongScore, reasons)

	return &AmbushCandidate{
		Symbol:     sym,
		Coin:       coin,
		Meta:       meta,
		AmbushType: LongAmbush,
		Reasons:    reasons,
	}
}

// ============================================================================
// SHORT Distribution Filter — Smart Money Exit Detector
// ============================================================================

func filterShortDistribution(coin nofxos.CoinData, meta *HunterCoinMeta) *AmbushCandidate {
	sym := coin.Pair

	// Gate 1: Direction & Score Quality
	if meta.Direction != "SHORT" {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: direction=%s not SHORT", sym, meta.Direction)
		return nil
	}
	if meta.ShortScore < MinScoreSniffer {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: short_score %.1f < %.1f", sym, meta.ShortScore, MinScoreSniffer)
		return nil
	}

	// Gate 2: Volatility Squeeze (REQUIRED — high-level stall)
	if !hasTag(meta.ShortTags, "bb_squeeze_15m") {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: no bb_squeeze_15m (tags: %v)", sym, meta.ShortTags)
		return nil
	}

	// Gate 3: Smart Money Distribution Footprint (ANY ONE required)
	allShortSignals := []string{"oi_distribution", "taker_sustained_selling", "lsr_bearish_reversal", "lsr_bearish_strong"}
	passedSignal := ""
	for _, sig := range allShortSignals {
		if hasTag(meta.ShortTags, sig) {
			passedSignal = sig
			break
		}
	}
	if passedSignal == "" {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: no distribution signal (tags: %v)", sym, meta.ShortTags)
		return nil
	}

	// Gate 4: Anti-Support-Floor Filter (MUST NOT have strong support below)
	combinedTags := mergeTags(meta.LongTags, meta.ShortTags)
	if hasTag(combinedTags, "near_support_4h") {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: near_support_4h in tags", sym)
		return nil
	}
	if hasTag(combinedTags, "near_support_1d") {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: near_support_1d in tags", sym)
		return nil
	}

	// Gate 5: Wash Trade Killer (MUST NOT)
	if hasTag(combinedTags, "wash_micro_trades") {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: wash_micro_trades detected", sym)
		return nil
	}
	if hasTag(combinedTags, "wash_fake_volume") {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: wash_fake_volume detected", sym)
		return nil
	}

	// ALL GATES PASSED
	reasons := []string{
		"bb_squeeze_15m:high_range_stall",
		fmt.Sprintf("%s:smart_money_exit", passedSignal),
	}
	if hasTag(combinedTags, "lsr_crowded_long") || hasTag(combinedTags, "lsr_crowded_long_favor_short") {
		reasons = append(reasons, "lsr_crowded_long:crowd_trap")
	}
	if hasTag(combinedTags, "near_resistance_4h") {
		reasons = append(reasons, "near_resistance_4h:resistance_zone")
	}

	log.Printf("[Sniffer] %s SHORT_DIST PASSED (score=%.1f, reasons=%v)", sym, meta.ShortScore, reasons)

	return &AmbushCandidate{
		Symbol:     sym,
		Coin:       coin,
		Meta:       meta,
		AmbushType: ShortDistribution,
		Reasons:    reasons,
	}
}

// ============================================================================
// Helpers
// ============================================================================

// hasTag checks if a tag exists in a tag slice.
func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

// mergeTags combines two tag slices into one (deduplication not required for checking).
func mergeTags(a, b []string) []string {
	merged := make([]string, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return merged
}

// classifyBlockReason determines the primary reason a coin was blocked (for stats).
// Evaluated in priority order: direction > score > squeeze > signal > wall > wash.
func classifyBlockReason(coin nofxos.CoinData, meta *HunterCoinMeta) string {
	// Check both directions — the coin might pass one but we only track the "primary" block
	if meta.Direction == "LONG" {
		return classifyLongBlock(meta)
	}
	if meta.Direction == "SHORT" {
		return classifyShortBlock(meta)
	}
	return "direction"
}

func classifyLongBlock(meta *HunterCoinMeta) string {
	if meta.LongScore < MinScoreSniffer {
		return "score"
	}
	if !hasTag(meta.LongTags, "bb_squeeze_15m") {
		return "squeeze"
	}
	longSignals := []string{"oi_accumulation", "taker_sustained_buying", "lsr_reversal"}
	hasSignal := false
	for _, sig := range longSignals {
		if hasTag(meta.LongTags, sig) {
			hasSignal = true
			break
		}
	}
	if !hasSignal {
		return "signal"
	}
	combined := mergeTags(meta.LongTags, meta.ShortTags)
	if hasTag(combined, "near_resistance_4h") || hasTag(combined, "near_resistance_1d") {
		return "wall"
	}
	if hasTag(combined, "wash_micro_trades") || hasTag(combined, "wash_fake_volume") {
		return "wash"
	}
	return "passed"
}

func classifyShortBlock(meta *HunterCoinMeta) string {
	if meta.ShortScore < MinScoreSniffer {
		return "score"
	}
	if !hasTag(meta.ShortTags, "bb_squeeze_15m") {
		return "squeeze"
	}
	shortSignals := []string{"oi_distribution", "taker_sustained_selling", "lsr_bearish_reversal", "lsr_bearish_strong"}
	hasSignal := false
	for _, sig := range shortSignals {
		if hasTag(meta.ShortTags, sig) {
			hasSignal = true
			break
		}
	}
	if !hasSignal {
		return "signal"
	}
	combined := mergeTags(meta.LongTags, meta.ShortTags)
	if hasTag(combined, "near_support_4h") || hasTag(combined, "near_support_1d") {
		return "wall"
	}
	if hasTag(combined, "wash_micro_trades") || hasTag(combined, "wash_fake_volume") {
		return "wash"
	}
	return "passed"
}
