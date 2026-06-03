package local

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"nofx/provider/nofxos"
)

// TestHunterSniffer_LiveData runs the sniffer against real Binance data.
// Run with: go test -C /Users/aixx/Code/AiT ./provider/local/... -v -run TestHunterSniffer_LiveData -count=1 -timeout 120s
func TestHunterSniffer_LiveData(t *testing.T) {
	if os.Getenv("LIVE_TEST") == "" {
		t.Skip("Skipping live test — set LIVE_TEST=1 to run")
	}

	// Create a real client using the standard constructor
	client := NewClient("https://fapi.binance.com")

	log.Println("=== Hunter Sniffer Live Test ===")
	log.Printf("Time: %s", time.Now().Format("2006-01-02 15:04:05"))

	// Step 1: Get raw Hunter scored coins
	log.Println("[Step 1] Fetching Hunter scored coins from Binance...")
	allCoins, err := client.GetHunterList()
	if err != nil {
		t.Fatalf("GetHunterList failed: %v", err)
	}
	log.Printf("[Step 1] Got %d scored coins", len(allCoins))

	// Step 2: Build coinMeta from allCoins (same as GetHunterCoinsWithData but without pre-fetching klines)
	log.Println("[Step 2] Building coin meta...")
	coinMeta := make(map[string]*HunterCoinMeta)
	for _, coin := range allCoins {
		sym := normalizeSymbol(coin.Pair)
		coinMeta[sym] = &HunterCoinMeta{
			Direction:  coin.Direction,
			SignalTags: coin.SignalTags,
			LongScore:  coin.LongScore,
			ShortScore: coin.ShortScore,
			LongTags:   coin.LongTags,
			ShortTags:  coin.ShortTags,
		}
	}
	log.Printf("[Step 2] Built meta for %d coins", len(coinMeta))

	// Step 3: Dump all coins before sniff for analysis
	log.Println("\n=== ALL HUNTER COINS (PRE-SNIFF) ===")
	log.Printf("%-14s %-8s %6s %6s  %-40s  %-40s",
		"SYMBOL", "DIR", "LONG", "SHORT", "LONG_TAGS", "SHORT_TAGS")
	for _, coin := range allCoins {
		meta := coinMeta[normalizeSymbol(coin.Pair)]
		longTags := formatTags(meta.LongTags)
		shortTags := formatTags(meta.ShortTags)
		if len(longTags) > 40 {
			longTags = longTags[:37] + "..."
		}
		if len(shortTags) > 40 {
			shortTags = shortTags[:37] + "..."
		}
		log.Printf("%-14s %-8s %6.1f %6.1f  %-40s  %-40s",
			coin.Pair, meta.Direction, meta.LongScore, meta.ShortScore, longTags, shortTags)
	}

	// Step 4: Apply Sniff Filter
	log.Println("\n=== APPLYING SNIFF FILTER ===")
	snifferResult := client.FilterAmbushCandidates(allCoins, coinMeta)

	// Step 5: Print results
	printSnifferResult(snifferResult)

	// Step 6: Summary for report
	t.Logf("Sniffer Result: LONG_AMBUSH=%d, SHORT_DIST=%d, TotalScanned=%d",
		len(snifferResult.LongAmbush), len(snifferResult.ShortDist), snifferResult.Stats.TotalScanned)
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	result := ""
	for i, t := range tags {
		if i > 0 {
			result += ","
		}
		result += t
	}
	return result
}

func printSnifferResult(result *SnifferResult) {
	log.Println("\n" + repeat("=", 80))
	log.Println("  HUNTER SNIFF — LIVE DATA ANALYSIS REPORT")
	log.Println(repeat("=", 80))

	// Stats overview
	log.Printf("\n📊 SCAN STATISTICS:")
	log.Printf("  Total Scanned:       %d", result.Stats.TotalScanned)
	log.Printf("  LONG_AMBUSH Passed:  %d", result.Stats.LongPassed)
	log.Printf("  SHORT_DIST Passed:   %d", result.Stats.ShortPassed)
	log.Printf("")
	log.Printf("  🔴 Blocked by Direction: %d", result.Stats.BlockedByDirection)
	log.Printf("  🔴 Blocked by Score:     %d", result.Stats.BlockedByScore)
	log.Printf("  🔴 Blocked by Squeeze:   %d", result.Stats.BlockedBySqueeze)
	log.Printf("  🔴 Blocked by Signal:    %d", result.Stats.BlockedBySignal)
	log.Printf("  🔴 Blocked by Wall:      %d", result.Stats.BlockedByWall)
	log.Printf("  🔴 Blocked by Wash:      %d", result.Stats.BlockedByWash)

	// LONG Ambush candidates
	log.Printf("\n🟢 LONG_AMBUSH CANDIDATES (%d):", len(result.LongAmbush))
	if len(result.LongAmbush) == 0 {
		log.Println("  (none — no institutional accumulation detected)")
	} else {
		log.Printf("  %-14s %6s %6s  %-40s  %s",
			"SYMBOL", "LONG", "SHORT", "REASONS", "KEY_TAGS")
		for _, amb := range result.LongAmbush {
			log.Printf("  %-14s %6.1f %6.1f  %-40s  %v",
				amb.Symbol, amb.Meta.LongScore, amb.Meta.ShortScore,
				formatReasons(amb.Reasons), getKeyTags(amb.Meta.LongTags))
		}
	}

	// SHORT Distribution candidates
	log.Printf("\n🔴 SHORT_DIST CANDIDATES (%d):", len(result.ShortDist))
	if len(result.ShortDist) == 0 {
		log.Println("  (none — no institutional distribution detected)")
	} else {
		log.Printf("  %-14s %6s %6s  %-40s  %s",
			"SYMBOL", "LONG", "SHORT", "REASONS", "KEY_TAGS")
		for _, amb := range result.ShortDist {
			log.Printf("  %-14s %6.1f %6.1f  %-40s  %v",
				amb.Symbol, amb.Meta.LongScore, amb.Meta.ShortScore,
				formatReasons(amb.Reasons), getKeyTags(amb.Meta.ShortTags))
		}
	}

	// Pass rate
	total := result.Stats.TotalScanned
	passed := result.Stats.LongPassed + result.Stats.ShortPassed
	passRate := 0.0
	if total > 0 {
		passRate = float64(passed) / float64(total) * 100
	}
	log.Printf("\n📈 PASS RATE: %d/%d (%.1f%%)", passed, total, passRate)
	log.Println(repeat("=", 80))
}

func formatReasons(reasons []string) string {
	result := ""
	for i, r := range reasons {
		if i > 0 {
			result += " + "
		}
		result += r
	}
	if len(result) > 40 {
		result = result[:37] + "..."
	}
	return result
}

func getKeyTags(tags []string) []string {
	key := []string{}
	priority := map[string]bool{
		"bb_squeeze_15m":        true,
		"oi_accumulation":       true,
		"taker_sustained_buying": true,
		"taker_sustained_selling": true,
		"oi_distribution":       true,
		"lsr_reversal":          true,
		"lsr_bearish_reversal":  true,
		"lsr_bearish_strong":    true,
		"near_support_4h":       true,
		"near_resistance_4h":    true,
		"squeeze_explosion_synergy": true,
		"oi_spike_1h":           true,
	}
	for _, t := range tags {
		if priority[t] {
			key = append(key, t)
		}
	}
	if len(key) == 0 {
		key = tags // return all if no priority match
	}
	return key
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// Ensure nofxos is used
var _ nofxos.CoinData

// Ensure fmt is used
var _ = fmt.Sprintf
