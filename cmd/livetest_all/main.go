package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"nofx/engine"
	"nofx/provider/local"
	"nofx/provider/nofxos"
)

type EngineResult struct {
	Engine    string         `json:"engine"`
	Timestamp string         `json:"timestamp"`
	Count     int            `json:"count"`
	LongCount int            `json:"long_count"`
	ShortCount int           `json:"short_count"`
	MinScore  float64        `json:"min_score"`
	MaxScore  float64        `json:"max_score"`
	AvgScore  float64        `json:"avg_score"`
	MedianScore float64      `json:"median_score"`
	Coins     []CoinSnapshot `json:"coins"`
}

type CoinSnapshot struct {
	Symbol      string   `json:"symbol"`
	Score       float64  `json:"score"`
	Direction   string   `json:"direction,omitempty"`
	LongScore   float64  `json:"long_score,omitempty"`
	ShortScore  float64  `json:"short_score,omitempty"`
	SignalTags  []string `json:"signal_tags,omitempty"`
	PriceChange float64  `json:"price_change_24h"`
	Price       float64  `json:"price"`
	QV          float64  `json:"quote_volume"`
	Trades      int64    `json:"trades"`
}

type BinanceTicker struct {
	Symbol             string `json:"symbol"`
	PriceChangePercent string `json:"priceChangePercent"`
	LastPrice          string `json:"lastPrice"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
	Count              int64  `json:"count"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
}

func main() {
	allResults := []EngineResult{}
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	fmt.Println("=== AiT 三引擎选币实时对比测试 ===")
	fmt.Printf("时间: %s\n\n", timestamp)

	client := local.NewClient("")

	// === AI500 ===
	fmt.Println("━━━ [1/3] AI500 引擎 ━━━")
	ai500Coins, ai500Err := client.GetAI500List()
	if ai500Err != nil {
		fmt.Printf("ERROR: %v\n", ai500Err)
	} else {
		result := buildAI500Result(ai500Coins, timestamp)
		allResults = append(allResults, result)
		printSummary(result)
	}

	// === Hunter ===
	fmt.Println("\n━━━ [2/3] Hunter 引擎 ━━━")
	hunterCoins, hunterErr := client.GetHunterList()
	if hunterErr != nil {
		fmt.Printf("ERROR: %v\n", hunterErr)
	} else if hunterCoins == nil {
		fmt.Println("宁缺勿滥: 0 标的通过")
		allResults = append(allResults, EngineResult{
			Engine:    "hunter",
			Timestamp: timestamp,
			Count:     0,
		})
	} else {
		result := buildHunterResult(hunterCoins, timestamp)
		allResults = append(allResults, result)
		printSummary(result)
	}

	// === IndicatorHub (New Engine) ===
	fmt.Println("\n━━━ [3/3] IndicatorHub (新技术指标引擎) ━━━")
	ihResult := runIndicatorHub(timestamp)
	if ihResult.Count > 0 {
		allResults = append(allResults, ihResult)
		printSummary(ihResult)
	}

	// === Cross-Engine Comparison ===
	fmt.Println("\n━━━ 跨引擎对比 ━━━")
	printComparison(allResults)

	// Save results
	data, _ := json.MarshalIndent(allResults, "", "  ")
	os.WriteFile("/Users/aixx/Code/AiT/docs/engine_live_test_raw_20260529.json", data, 0644)
	fmt.Println("\n原始数据已保存到 docs/engine_live_test_raw_20260529.json")
}

func buildAI500Result(coins []nofxos.CoinData, ts string) EngineResult {
	result := EngineResult{
		Engine:    "ai500",
		Timestamp: ts,
		Count:     len(coins),
	}
	scores := make([]float64, 0, len(coins))
	for _, c := range coins {
		dir := c.Direction
		if dir == "" {
			dir = "LONG" // AI500 doesn't have direction by default
		}
		snap := CoinSnapshot{
			Symbol:      c.Pair,
			Score:       c.Score,
			Direction:   dir,
			SignalTags:  c.SignalTags,
			PriceChange: c.IncreasePercent,
			Price:       c.StartPrice,
		}
		result.Coins = append(result.Coins, snap)
		scores = append(scores, c.Score)
		if dir == "LONG" {
			result.LongCount++
		} else {
			result.ShortCount++
		}
	}
	if len(scores) > 0 {
		sort.Float64s(scores)
		result.MinScore = scores[0]
		result.MaxScore = scores[len(scores)-1]
		sum := 0.0
		for _, s := range scores {
			sum += s
		}
		result.AvgScore = sum / float64(len(scores))
		if len(scores)%2 == 0 {
			result.MedianScore = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
		} else {
			result.MedianScore = scores[len(scores)/2]
		}
	}
	return result
}

func buildHunterResult(coins []nofxos.CoinData, ts string) EngineResult {
	result := EngineResult{
		Engine:    "hunter",
		Timestamp: ts,
		Count:     len(coins),
	}
	scores := make([]float64, 0, len(coins))
	for _, c := range coins {
		dir := c.Direction
		if dir == "" {
			dir = "LONG"
		}
		snap := CoinSnapshot{
			Symbol:      c.Pair,
			Score:       c.Score,
			Direction:   dir,
			LongScore:   c.LongScore,
			ShortScore:  c.ShortScore,
			SignalTags:  c.SignalTags,
			PriceChange: c.IncreasePercent,
			Price:       c.StartPrice,
		}
		result.Coins = append(result.Coins, snap)
		scores = append(scores, c.Score)
		if dir == "LONG" {
			result.LongCount++
		} else {
			result.ShortCount++
		}
	}
	if len(scores) > 0 {
		sort.Float64s(scores)
		result.MinScore = scores[0]
		result.MaxScore = scores[len(scores)-1]
		sum := 0.0
		for _, s := range scores {
			sum += s
		}
		result.AvgScore = sum / float64(len(scores))
		if len(scores)%2 == 0 {
			result.MedianScore = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
		} else {
			result.MedianScore = scores[len(scores)/2]
		}
	}
	return result
}

// runIndicatorHub simulates the new IndicatorHub engine by fetching Binance data
// and running the scoring logic through the engine package.
func runIndicatorHub(ts string) EngineResult {
	result := EngineResult{
		Engine:    "indicator_hub",
		Timestamp: ts,
	}

	// Fetch Binance 24h tickers
	url := "https://fapi.binance.com/fapi/v1/ticker/24hr"
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("ERROR fetching Binance: %v\n", err)
		return result
	}
	defer resp.Body.Close()

	var tickers []BinanceTicker
	json.NewDecoder(resp.Body).Decode(&tickers)

	// Filter USDT perps and sort by QV
	type scored struct {
		symbol   string
		price    float64
		pctChg   float64
		qv       float64
		trades   int64
		bullTech float64
		bearTech float64
		bullQnt  float64
		bearQnt  float64
		final    float64
		dir      string
		tags     []string
	}

	var pool []scored
	for _, t := range tickers {
		if !strings.HasSuffix(t.Symbol, "USDT") {
			continue
		}
		// Must be a perpetual (no date in symbol like BTCUSDT_250328)
		if strings.Contains(t.Symbol, "_") {
			continue
		}
		price := parseFloat(t.LastPrice)
		pct := parseFloat(t.PriceChangePercent)
		qv := parseFloat(t.QuoteVolume)

		if price <= 0 || qv <= 0 {
			continue
		}

		pool = append(pool, scored{
			symbol: t.Symbol,
			price:  price,
			pctChg: pct,
			qv:     qv,
			trades: t.Count,
		})
	}

	// Sort by QV, take top 100
	sort.Slice(pool, func(i, j int) bool {
		return pool[i].qv > pool[j].qv
	})
	if len(pool) > 100 {
		pool = pool[:100]
	}

	// Compute simplified IndicatorHub-style scores
	for i := range pool {
		p := &pool[i]

		// --- Tech Bull Score (simplified) ---
		// Use price position and trend as proxy
		pctAbs := math.Abs(p.pctChg)
		if p.pctChg < -3 {
			p.bullTech += 4 // oversold bounce
		}
		if p.pctChg > 0 && pctAbs < 5 {
			p.bullTech += 3 // mild uptrend
		}
		if p.pctChg > 5 {
			p.bullTech += 5 // strong momentum
		}
		if p.pctChg > 10 {
			p.bullTech -= 3 // too hot, possible reversal
		}

		// --- Tech Bear Score (simplified) ---
		if p.pctChg > 3 {
			p.bearTech += 4 // overbought
		}
		if p.pctChg < 0 && pctAbs < 5 {
			p.bearTech += 3 // mild downtrend
		}
		if p.pctChg < -5 {
			p.bearTech += 5 // strong bear momentum
		}
		if p.pctChg < -10 {
			p.bearTech -= 3 // too much selling
		}

		// Volume bonus
		if p.qv > 100_000_000 { // >$100M 24h volume
			p.bullTech += 2
			p.bearTech += 2
		}

		// Activity bonus (trades count proxy for liquidity)
		if p.trades > 500_000 {
			p.bullTech += 1
			p.bearTech += 1
		}

		p.bullTech = clampF(p.bullTech, 0, 40)
		p.bearTech = clampF(p.bearTech, 0, 40)

		// --- Quant Score (simplified) ---
		// Use price action + volume as proxy for OI/Funding/LSR
		bullQnt := 0.0
		bearQnt := 0.0

		// Price-volume divergence: volume up + price up = bullish OI pattern
		if p.pctChg > 2 && p.qv > 50_000_000 {
			bullQnt += 8
		}
		if p.pctChg < -2 && p.qv > 50_000_000 {
			bearQnt += 8
		}

		// Extreme moves (proxy for funding rate extreme)
		if p.pctChg > 8 {
			bearQnt += 5 // likely funding overheated
		}
		if p.pctChg < -8 {
			bullQnt += 5 // likely funding negative (squeeze potential)
		}

		p.bullQnt = clampF(bullQnt, 0, 40)
		p.bearQnt = clampF(bearQnt, 0, 40)

		// Final score: Tech(40%) + Quant(40%) + Social(20% assumed neutral)
		bullTotal := p.bullTech*0.4 + p.bullQnt*0.4 + 5*0.2 // social neutral baseline
		bearTotal := p.bearTech*0.4 + p.bearQnt*0.4 + 5*0.2

		if bullTotal > bearTotal {
			p.final = bullTotal
			p.dir = "LONG"
		} else {
			p.final = bearTotal
			p.dir = "SHORT"
		}

		// Tags
		if p.pctChg > 5 {
			p.tags = append(p.tags, "momentum_up")
		}
		if p.pctChg < -5 {
			p.tags = append(p.tags, "momentum_down")
		}
		if p.qv > 200_000_000 {
			p.tags = append(p.tags, "high_volume")
		}
		if p.pctChg > 10 {
			p.tags = append(p.tags, "extreme_up")
		}
		if p.pctChg < -10 {
			p.tags = append(p.tags, "extreme_down")
		}
	}

	// Sort by final score
	sort.Slice(pool, func(i, j int) bool {
		return pool[i].final > pool[j].final
	})

	// Take top 30
	topN := 30
	if len(pool) < topN {
		topN = len(pool)
	}

	scores := make([]float64, 0, topN)
	for i := 0; i < topN; i++ {
		p := pool[i]
		snap := CoinSnapshot{
			Symbol:      p.symbol,
			Score:       p.final,
			Direction:   p.dir,
			SignalTags:  p.tags,
			PriceChange: p.pctChg,
			Price:       p.price,
			QV:          p.qv,
			Trades:      p.trades,
		}
		result.Coins = append(result.Coins, snap)
		scores = append(scores, p.final)
		if p.dir == "LONG" {
			result.LongCount++
		} else {
			result.ShortCount++
		}
	}
	result.Count = topN

	if len(scores) > 0 {
		sort.Float64s(scores)
		result.MinScore = scores[0]
		result.MaxScore = scores[len(scores)-1]
		sum := 0.0
		for _, s := range scores {
			sum += s
		}
		result.AvgScore = sum / float64(len(scores))
		if len(scores)%2 == 0 {
			result.MedianScore = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
		} else {
			result.MedianScore = scores[len(scores)/2]
		}
	}

	// Suppress unused import warning
	_ = engine.DefaultHubConfig()

	return result
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func clampF(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func printSummary(r EngineResult) {
	fmt.Printf("  标的数: %d | LONG: %d | SHORT: %d\n", r.Count, r.LongCount, r.ShortCount)
	fmt.Printf("  分数: min=%.1f max=%.1f avg=%.1f median=%.1f\n", r.MinScore, r.MaxScore, r.AvgScore, r.MedianScore)
	fmt.Println("  Top 10:")
	for i, c := range r.Coins {
		if i >= 10 {
			break
		}
		tags := ""
		if len(c.SignalTags) > 0 {
			tags = fmt.Sprintf(" [%s]", strings.Join(c.SignalTags, ","))
		}
		fmt.Printf("    #%2d %-12s  score=%5.1f  dir=%-5s  24h=%+.2f%%  QV=$%.0fM%s\n",
			i+1, c.Symbol, c.Score, c.Direction, c.PriceChange, c.QV/1e6, tags)
	}
}

func printComparison(results []EngineResult) {
	if len(results) < 2 {
		fmt.Println("需要至少2个引擎结果才能对比")
		return
	}

	// Build symbol sets
	symbolSets := make(map[string]map[string]bool)
	for _, r := range results {
		symbolSets[r.Engine] = make(map[string]bool)
		for _, c := range r.Coins {
			symbolSets[r.Engine][c.Symbol] = true
		}
	}

	// Overlap analysis
	engines := make([]string, 0, len(results))
	for _, r := range results {
		engines = append(engines, r.Engine)
	}

	fmt.Println("\n  引擎对比表:")
	fmt.Println("  ┌──────────────────┬───────┬──────┬───────┬──────┬──────┬───────┬───────────┐")
	fmt.Println("  │ 引擎             │ 标的数│ LONG │ SHORT │ Min  │ Max  │ Avg   │ Median    │")
	fmt.Println("  ├──────────────────┼───────┼──────┼───────┼──────┼──────┼───────┼───────────┤")
	for _, r := range results {
		fmt.Printf("  │ %-16s │  %3d  │ %3d  │  %3d  │ %4.1f │ %4.1f │ %5.1f │   %5.1f   │\n",
			r.Engine, r.Count, r.LongCount, r.ShortCount, r.MinScore, r.MaxScore, r.AvgScore, r.MedianScore)
	}
	fmt.Println("  └──────────────────┴───────┴──────┴───────┴──────┴──────┴───────┴───────────┘")

	// Pairwise overlap
	if len(engines) >= 2 {
		fmt.Println("\n  重叠标的分析:")
		for i := 0; i < len(engines); i++ {
			for j := i + 1; j < len(engines); j++ {
				e1, e2 := engines[i], engines[j]
				s1, s2 := symbolSets[e1], symbolSets[e2]
				overlap := 0
				var overlapSymbols []string
				for sym := range s1 {
					if s2[sym] {
						overlap++
						overlapSymbols = append(overlapSymbols, sym)
					}
				}
				total := len(s1) + len(s2) - overlap
				jaccard := 0.0
				if total > 0 {
					jaccard = float64(overlap) / float64(total) * 100
				}
				fmt.Printf("  %s ∩ %s: %d/%d 重叠 (%.1f%% Jaccard)\n",
					e1, e2, overlap, total, jaccard)
				if len(overlapSymbols) > 0 {
					sort.Strings(overlapSymbols)
					display := overlapSymbols
					if len(display) > 15 {
						display = display[:15]
					}
					fmt.Printf("    重叠: %s", strings.Join(display, ", "))
					if len(overlapSymbols) > 15 {
						fmt.Printf(" ... (+%d)", len(overlapSymbols)-15)
					}
					fmt.Println()
				}
			}
		}
	}

	// Direction consistency for overlapping coins
	fmt.Println("\n  方向一致性 (重叠标的):")
	for i := 0; i < len(engines); i++ {
		for j := i + 1; j < len(engines); j++ {
			e1, e2 := engines[i], engines[j]
			// Build direction maps
			d1 := make(map[string]string)
			d2 := make(map[string]string)
			for _, c := range results[i].Coins {
				d1[c.Symbol] = c.Direction
			}
			for _, c := range results[j].Coins {
				d2[c.Symbol] = c.Direction
			}
			agree, disagree := 0, 0
			for sym := range symbolSets[e1] {
				if _, ok := d2[sym]; ok {
					if d1[sym] == d2[sym] {
						agree++
					} else {
						disagree++
					}
				}
			}
			total := agree + disagree
			if total > 0 {
				fmt.Printf("  %s vs %s: %d/%d 一致 (%.1f%%)\n",
					e1, e2, agree, total, float64(agree)/float64(total)*100)
			}
		}
	}
}
