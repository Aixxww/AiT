package local

import (
	"fmt"
	"log"

	"github.com/Aixxww/AiT/provider/aitos"
)

// ============================================================================
// Hunter Sniffer — Institutional Ambush Pattern Detector
// ============================================================================
// Post-Hunter middleware that filters raw scored coins through multi-condition
// resonance logic to detect:
//   - LONG Ambush: smart money accumulating (any volatility regime)
//   - SHORT Distribution: smart money exiting at highs
//
// Filter gates (each direction):
//   1. Direction & score quality
//   2. Compression signals (bb_squeeze / oi_spike / oi_accumulation /
//      range_compression) — FLEXIBLE scoring, threshold ≥ 2
//   3. Smart money footprint (accumulation/distribution signals) — ANY ONE
//      (optional when Gate 2 score ≥ 3, high-confidence mode)
//   4. Wall filter (no opposing S/R nearby) — MUST NOT
//   5. Wash trade filter — MUST NOT
//
// Gate 2 was redesigned from rigid bb_squeeze_15m to elastic compression
// scoring to capture institutional ambush patterns in ALL volatility regimes,
// not just during BB squeeze windows.

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

// CompressionScoreThreshold is the minimum compression score for Gate 2.
// Score ≥ 2 means at least one substantive signal (oi_spike, oi_accumulation,
// range_compression, or bb_squeeze) is present. This replaces the rigid
// bb_squeeze_15m requirement with a flexible multi-signal scoring system.
const CompressionScoreThreshold = 2

// HighConfidenceCompressionScore is the threshold above which Gate 3 (smart
// money signal) becomes optional. When compression_score ≥ 3, the compression
// signals themselves already imply institutional activity (e.g., bb_squeeze_15m
// alone scores 3, or oi_spike + range_compression scores 4).
const HighConfidenceCompressionScore = 3

// AmbushCandidate represents a coin that passed the sniff filter.
type AmbushCandidate struct {
	Symbol     string
	Coin       aitos.CoinData  // Original Hunter data
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
	BlockedByCompression int // Compression score < threshold (replaces BlockedBySqueeze)
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
	coins []aitos.CoinData,
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
		case "compression":
			result.Stats.BlockedByCompression++
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
	log.Printf("[Sniffer] Blocked: direction=%d, score=%d, compression=%d, signal=%d, wall=%d, wash=%d",
		result.Stats.BlockedByDirection, result.Stats.BlockedByScore,
		result.Stats.BlockedByCompression, result.Stats.BlockedBySignal,
		result.Stats.BlockedByWall, result.Stats.BlockedByWash)

	return result
}

// ============================================================================
// LONG Ambush Filter — Smart Money Accumulation Detector
// ============================================================================

func filterLongAmbush(coin aitos.CoinData, meta *HunterCoinMeta) *AmbushCandidate {
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

	// Gate 2: Elastic Compression Scoring (replaces rigid bb_squeeze_15m)
	// Any combination of compression signals scoring ≥ 2 passes.
	// Captures institutional ambush in ALL volatility regimes, not just BB squeeze.
	compScore := computeCompressionScore(meta.LongTags)
	if compScore < CompressionScoreThreshold {
		log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: compression_score=%d < %d (tags: %v)",
			sym, compScore, CompressionScoreThreshold, meta.LongTags)
		return nil
	}

	// Gate 3: Smart Money Footprint (ANY ONE required)
	// When compression_score ≥ HighConfidenceCompressionScore (3), Gate 3 is
	// optional — the compression signals themselves imply institutional activity.
	combinedTags := mergeTags(meta.LongTags, meta.ShortTags)
	allLongSignals := []string{
		"oi_accumulation",        // OI↑ + price↓ = classic accumulation
		"taker_sustained_buying", // sustained aggressive buying
		"lsr_reversal",           // LSR turning bullish
		"oi_spike_1h",            // OI anomaly = new capital entering
		"range_compression",      // volume + tight range = stealth buying
	}
	passedSignal := ""
	for _, sig := range allLongSignals {
		if hasTag(meta.LongTags, sig) {
			passedSignal = sig
			break
		}
	}
	if passedSignal == "" {
		if compScore >= HighConfidenceCompressionScore {
			// High-confidence mode: compression signal is strong enough on its own
			passedSignal = fmt.Sprintf("high_confidence_compression(%d)", compScore)
			log.Printf("[Sniffer] %s LONG_AMBUSH Gate 3 bypassed: compression_score=%d (high confidence)",
				sym, compScore)
		} else {
			log.Printf("[Sniffer DEBUG] %s LONG_AMBUSH blocked: no accumulation signal (tags: %v)", sym, meta.LongTags)
			return nil
		}
	}

	// Gate 4: Anti-Crash-Wall Filter (MUST NOT have resistance overhead)
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

	// ALL GATES PASSED — build reason list
	reasons := []string{}
	if hasTag(meta.LongTags, "bb_squeeze_15m") {
		reasons = append(reasons, "bb_squeeze_15m:volatility_ice")
	}
	if hasTag(meta.LongTags, "bb_squeeze_5m") {
		reasons = append(reasons, "bb_squeeze_5m:short_squeeze")
	}
	if hasTag(meta.LongTags, "oi_spike_1h") {
		reasons = append(reasons, "oi_spike_1h:capital_inflow")
	}
	if hasTag(meta.LongTags, "range_compression") {
		reasons = append(reasons, "range_compression:stealth_accumulation")
	}
	if hasTag(meta.LongTags, "squeeze_explosion_synergy") {
		reasons = append(reasons, "squeeze_explosion_synergy:breakout_imminent")
	}
	reasons = append(reasons, fmt.Sprintf("%s:smart_money_entry", passedSignal))
	if hasTag(combinedTags, "near_support_4h") {
		reasons = append(reasons, "near_support_4h:support_zone")
	}
	if hasTag(combinedTags, "near_support_1d") {
		reasons = append(reasons, "near_support_1d:daily_support")
	}

	log.Printf("[Sniffer] %s LONG_AMBUSH PASSED (score=%.1f, compression=%d, reasons=%v)",
		sym, meta.LongScore, compScore, reasons)

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

func filterShortDistribution(coin aitos.CoinData, meta *HunterCoinMeta) *AmbushCandidate {
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

	// Gate 2: Elastic Compression Scoring (replaces rigid bb_squeeze_15m)
	compScore := computeCompressionScore(meta.ShortTags)
	if compScore < CompressionScoreThreshold {
		log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: compression_score=%d < %d (tags: %v)",
			sym, compScore, CompressionScoreThreshold, meta.ShortTags)
		return nil
	}

	// Gate 3: Smart Money Distribution Footprint (ANY ONE required)
	// When compression_score ≥ HighConfidenceCompressionScore, Gate 3 is optional.
	combinedTags := mergeTags(meta.LongTags, meta.ShortTags)
	allShortSignals := []string{
		"oi_distribution",         // OI↑ + price↑ = distribution
		"taker_sustained_selling", // sustained aggressive selling
		"lsr_bearish_reversal",    // LSR turning bearish
		"lsr_bearish_strong",      // strong bearish signal
		"oi_spike_1h",             // OI anomaly = new capital entering
		"range_compression",       // volume + tight range = stealth positioning
	}
	passedSignal := ""
	for _, sig := range allShortSignals {
		if hasTag(meta.ShortTags, sig) {
			passedSignal = sig
			break
		}
	}
	if passedSignal == "" {
		if compScore >= HighConfidenceCompressionScore {
			passedSignal = fmt.Sprintf("high_confidence_compression(%d)", compScore)
			log.Printf("[Sniffer] %s SHORT_DIST Gate 3 bypassed: compression_score=%d (high confidence)",
				sym, compScore)
		} else {
			log.Printf("[Sniffer DEBUG] %s SHORT_DIST blocked: no distribution signal (tags: %v)", sym, meta.ShortTags)
			return nil
		}
	}

	// Gate 4: Anti-Support-Floor Filter (MUST NOT have strong support below)
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

	// ALL GATES PASSED — build reason list
	reasons := []string{}
	if hasTag(meta.ShortTags, "bb_squeeze_15m") {
		reasons = append(reasons, "bb_squeeze_15m:high_range_stall")
	}
	if hasTag(meta.ShortTags, "bb_squeeze_5m") {
		reasons = append(reasons, "bb_squeeze_5m:short_squeeze")
	}
	if hasTag(meta.ShortTags, "oi_spike_1h") {
		reasons = append(reasons, "oi_spike_1h:capital_inflow")
	}
	if hasTag(meta.ShortTags, "range_compression") {
		reasons = append(reasons, "range_compression:stealth_positioning")
	}
	if hasTag(meta.ShortTags, "squeeze_explosion_synergy") {
		reasons = append(reasons, "squeeze_explosion_synergy:breakout_imminent")
	}
	reasons = append(reasons, fmt.Sprintf("%s:smart_money_exit", passedSignal))
	if hasTag(combinedTags, "lsr_crowded_long") || hasTag(combinedTags, "lsr_crowded_long_favor_short") {
		reasons = append(reasons, "lsr_crowded_long:crowd_trap")
	}
	if hasTag(combinedTags, "near_resistance_4h") {
		reasons = append(reasons, "near_resistance_4h:resistance_zone")
	}

	log.Printf("[Sniffer] %s SHORT_DIST PASSED (score=%.1f, compression=%d, reasons=%v)",
		sym, meta.ShortScore, compScore, reasons)

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

// computeCompressionScore calculates a weighted compression signal score from tags.
// Replaces the rigid bb_squeeze_15m check with a flexible multi-signal system.
//
// Scoring weights:
//
//	bb_squeeze_15m            → 3 (strongest: volatility ice)
//	bb_squeeze_5m             → 2 (short-term compression)
//	oi_spike_1h               → 2 (OI anomaly)
//	oi_surge_1h               → 1 (OI moderate change)
//	oi_accumulation           → 2 (OI↑ + price↓ = classic stealth)
//	oi_distribution           → 2 (OI↑ + price↑ = distribution)
//	range_compression         → 2 (volume + tight range = stealth)
//	squeeze_explosion_synergy → 1 (BB + OI dual signal)
//
// Threshold ≥ 2 means at least one substantive signal present.
func computeCompressionScore(tags []string) int {
	weights := map[string]int{
		"bb_squeeze_15m":            3,
		"bb_squeeze_5m":             2,
		"oi_spike_1h":               2,
		"oi_surge_1h":               1,
		"oi_accumulation":           2,
		"oi_distribution":           2,
		"range_compression":         2,
		"squeeze_explosion_synergy": 1,
	}
	score := 0
	for _, tag := range tags {
		if w, ok := weights[tag]; ok {
			score += w
		}
	}
	return score
}

// classifyBlockReason determines the primary reason a coin was blocked (for stats).
// Evaluated in priority order: direction > score > compression > signal > wall > wash.
func classifyBlockReason(coin aitos.CoinData, meta *HunterCoinMeta) string {
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
	compScore := computeCompressionScore(meta.LongTags)
	if compScore < CompressionScoreThreshold {
		return "compression"
	}
	longSignals := []string{
		"oi_accumulation", "taker_sustained_buying", "lsr_reversal",
		"oi_spike_1h", "range_compression",
	}
	hasSignal := false
	for _, sig := range longSignals {
		if hasTag(meta.LongTags, sig) {
			hasSignal = true
			break
		}
	}
	if !hasSignal && compScore < HighConfidenceCompressionScore {
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
	compScore := computeCompressionScore(meta.ShortTags)
	if compScore < CompressionScoreThreshold {
		return "compression"
	}
	shortSignals := []string{
		"oi_distribution", "taker_sustained_selling",
		"lsr_bearish_reversal", "lsr_bearish_strong",
		"oi_spike_1h", "range_compression",
	}
	hasSignal := false
	for _, sig := range shortSignals {
		if hasTag(meta.ShortTags, sig) {
			hasSignal = true
			break
		}
	}
	if !hasSignal && compScore < HighConfidenceCompressionScore {
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
