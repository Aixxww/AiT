package main

import (
	"encoding/json"
	"fmt"
	"math"
	"nofx/datafetch"
	"nofx/engine"
	"nofx/provider/local"
	"os"
	"sort"
	"strings"
	"time"
)

// EvalResult is the output of a single evaluation round.
type EvalResult struct {
	Timestamp     string          `json:"timestamp"`
	SnapshotMeta  SnapshotReport  `json:"snapshot"`
	NewSignals    []SignalReport  `json:"new_engine_signals"`
	OldSignals    []OldSignalReport `json:"old_engine_signals"`
	Comparison    ComparisonReport `json:"comparison"`
	Top50Scores   []ScoreEntry    `json:"top50_scores"`
}

type SnapshotReport struct {
	TotalSymbols   int    `json:"total_symbols"`
	DetailedSymbols int   `json:"detailed_symbols"`
	FetchDuration  string `json:"fetch_duration"`
	WSConnected    bool   `json:"ws_connected"`
	SocialFresh    bool   `json:"social_fresh"`
}

type SignalReport struct {
	Rank       int      `json:"rank"`
	Symbol     string   `json:"symbol"`
	Direction  string   `json:"direction"`
	FinalScore float64  `json:"final_score"`
	Grade      string   `json:"grade"`
	TechScore  float64  `json:"tech_score"`
	QuantScore float64  `json:"quant_score"`
	SocScore   float64  `json:"social_score"`
	Entry      float64  `json:"entry_price"`
	SL         float64  `json:"stop_loss"`
	TP1        float64  `json:"tp1"`
	TP2        float64  `json:"tp2"`
	BullSignals []string `json:"bull_signals"`
	BearSignals []string `json:"bear_signals"`
	Reasons     []string `json:"reasons"`
}

type OldSignalReport struct {
	Rank        int      `json:"rank"`
	Symbol      string   `json:"symbol"`
	Mode        string   `json:"mode"` // ai500 or hunter
	Score       float64  `json:"score"`
	Direction   string   `json:"direction"`
	LongScore   float64  `json:"long_score,omitempty"`
	ShortScore  float64  `json:"short_score,omitempty"`
	SignalTags  []string `json:"signal_tags,omitempty"`
	PriceChange float64  `json:"price_change_24h"`
}

type ScoreEntry struct {
	Rank       int     `json:"rank"`
	Symbol     string  `json:"symbol"`
	Direction  string  `json:"direction"`
	FinalScore float64 `json:"final_score"`
	TechBull   float64 `json:"tech_bull"`
	TechBear   float64 `json:"tech_bear"`
	QuantBull  float64 `json:"quant_bull"`
	QuantBear  float64 `json:"quant_bear"`
	SocBull    float64 `json:"soc_bull"`
	SocBear    float64 `json:"soc_bear"`
}

type ComparisonReport struct {
	NewTotalSignals   int     `json:"new_total_signals"`
	OldTotalSignals   int     `json:"old_total_signals"`
	NewUniqueSymbols  int     `json:"new_unique_symbols"`
	OldUniqueSymbols  int     `json:"old_unique_symbols"`
	OverlapSymbols    int     `json:"overlap_symbols"`
	OverlapPct        float64 `json:"overlap_pct"`
	NewAvgScore       float64 `json:"new_avg_score"`
	OldAvgScore       float64 `json:"old_avg_score"`
}

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║    AiT 新旧引擎实盘对比测试 — IndicatorHub v3.0       ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// ═══════════════════════════════════════════
	// Phase 1: Fetch real data via local.Client (Binance direct, reliable)
	// ═══════════════════════════════════════════
	fmt.Println("📡 Phase 1: 从 Binance 拉取实时数据 (local.Client 直连)...")
	fetchStart := time.Now()

	topN := 50
	evalClient := local.NewClient("")
	caches, err := evalClient.GetSymbolCachesForEval(topN)
	if err != nil {
		fmt.Printf("❌ 数据拉取失败: %v\n", err)
		os.Exit(1)
	}
	snap := local.CachesToSnapshot(caches)
	fetchDur := time.Since(fetchStart)

	fmt.Printf("✅ 数据拉取完成: %d 标的, 耗时 %v\n", len(snap.Symbols), fetchDur.Round(time.Millisecond))

	detailedCount := 0
	for _, s := range snap.Symbols {
		if s.Klines != nil && len(s.Klines["1h"]) > 0 {
			detailedCount++
		}
	}
	fmt.Printf("   详细数据标的: %d (Top %d 含 K线/OI/LSR)\n", detailedCount, topN)

	// Save snapshot to store
	store := datafetch.NewStore()
	store.Swap(snap)

	// ═══════════════════════════════════════════
	// Phase 2: New Engine — IndicatorHub scoring
	// ═══════════════════════════════════════════
	fmt.Println("\n🧠 Phase 2: 新引擎 IndicatorHub 评分...")
	hubStart := time.Now()

	cfg := engine.DefaultHubConfig()
	hub := engine.NewIndicatorHub(store, cfg)
	router := engine.NewSignalRouter(hub, cfg)

	sets := hub.ScoreAll()
	signals := router.Route(sets)
	hubDur := time.Since(hubStart)

	fmt.Printf("✅ 评分完成: %d 标的评分, %d 信号产出, 耗时 %v\n", len(sets), len(signals), hubDur.Round(time.Millisecond))

	// ═══════════════════════════════════════════
	// Phase 3: Old Engine — Hunter scoring (for comparison)
	// ═══════════════════════════════════════════
	fmt.Println("\n🔄 Phase 3: 旧引擎 Hunter 评分 (对比基准)...")
	oldStart := time.Now()

	var oldSignals []OldSignalReport
	hunterLimit := 20

	// Run Hunter
	hunterCoins, hunterErr := evalClient.GetHunterList()
	if hunterErr != nil {
		fmt.Printf("⚠️  Hunter 评分失败: %v\n", hunterErr)
	} else {
		limit := hunterLimit
		if len(hunterCoins) < limit {
			limit = len(hunterCoins)
		}
		for i := 0; i < limit; i++ {
			c := hunterCoins[i]
			oldSignals = append(oldSignals, OldSignalReport{
				Rank:        i + 1,
				Symbol:      c.Pair,
				Mode:        "hunter",
				Score:       c.Score,
				Direction:   c.Direction,
				LongScore:   c.LongScore,
				ShortScore:  c.ShortScore,
				SignalTags:  c.SignalTags,
				PriceChange: c.IncreasePercent,
			})
		}
		fmt.Printf("✅ Hunter: %d 标的\n", limit)
	}

	// Run AI500
	ai500Coins, ai500Err := evalClient.GetAI500List()
	if ai500Err != nil {
		fmt.Printf("⚠️  AI500 评分失败: %v\n", ai500Err)
	} else {
		limit := hunterLimit
		if len(ai500Coins) < limit {
			limit = len(ai500Coins)
		}
		for i := 0; i < limit; i++ {
			c := ai500Coins[i]
			oldSignals = append(oldSignals, OldSignalReport{
				Rank:        len(oldSignals) + 1,
				Symbol:      c.Pair,
				Mode:        "ai500",
				Score:       c.Score,
				PriceChange: c.IncreasePercent,
			})
		}
		fmt.Printf("✅ AI500: %d 标的\n", limit)
	}
	_ = time.Since(oldStart) // used for timing

	// ═══════════════════════════════════════════
	// Phase 4: Build comparison report
	// ═══════════════════════════════════════════
	fmt.Println("\n📊 Phase 4: 生成对比报告...")

	// New engine signal reports
	var newSignalReports []SignalReport
	for i, sig := range signals {
		newSignalReports = append(newSignalReports, SignalReport{
			Rank:        i + 1,
			Symbol:      sig.Symbol,
			Direction:   directionStr(sig.Direction),
			FinalScore:  round2(sig.FinalScore),
			Grade:       sig.Grade.String(),
			TechScore:   round2(sig.TechScore),
			QuantScore:  round2(sig.QuantScore),
			SocScore:    round2(sig.SocialScore),
			Entry:       sig.EntryPrice,
			SL:          sig.StopLoss,
			TP1:         sig.TP1,
			TP2:         sig.TP2,
			BullSignals: sig.BullSignals,
			BearSignals: sig.BearSignals,
			Reasons:     sig.Reasons,
		})
	}

	// Top 50 scores for depth analysis
	var top50 []ScoreEntry
	sort.Slice(sets, func(i, j int) bool {
		return sets[i].FinalScore > sets[j].FinalScore
	})
	for i := 0; i < 50 && i < len(sets); i++ {
		s := sets[i]
		top50 = append(top50, ScoreEntry{
			Rank:       i + 1,
			Symbol:     s.Symbol,
			Direction:  directionStr(s.Direction),
			FinalScore: round2(s.FinalScore),
			TechBull:   round2(s.TechBullScore),
			TechBear:   round2(s.TechBearScore),
			QuantBull:  round2(s.QuantBullScore),
			QuantBear:  round2(s.QuantBearScore),
			SocBull:    round2(s.SocialBullScore),
			SocBear:    round2(s.SocialBearScore),
		})
	}

	// Overlap analysis
	newSymbols := make(map[string]bool)
	for _, s := range newSignalReports {
		newSymbols[s.Symbol] = true
	}
	oldSymbols := make(map[string]bool)
	for _, s := range oldSignals {
		oldSymbols[s.Symbol] = true
	}
	overlap := 0
	for sym := range newSymbols {
		if oldSymbols[sym] {
			overlap++
		}
	}
	overlapPct := 0.0
	totalUnique := len(newSymbols) + len(oldSymbols) - overlap
	if totalUnique > 0 {
		overlapPct = float64(overlap) / float64(totalUnique) * 100
	}

	// Average scores
	newAvg := 0.0
	for _, s := range newSignalReports {
		newAvg += s.FinalScore
	}
	if len(newSignalReports) > 0 {
		newAvg /= float64(len(newSignalReports))
	}
	oldAvg := 0.0
	for _, s := range oldSignals {
		oldAvg += s.Score
	}
	if len(oldSignals) > 0 {
		oldAvg /= float64(len(oldSignals))
	}

	result := EvalResult{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		SnapshotMeta: SnapshotReport{
			TotalSymbols:    len(snap.Symbols),
			DetailedSymbols: detailedCount,
			FetchDuration:   fetchDur.Round(time.Millisecond).String(),
			WSConnected:     false,
			SocialFresh:     snap.Meta.SocialFresh,
		},
		NewSignals:  newSignalReports,
		OldSignals:  oldSignals,
		Top50Scores: top50,
		Comparison: ComparisonReport{
			NewTotalSignals:  len(newSignalReports),
			OldTotalSignals:  len(oldSignals),
			NewUniqueSymbols: len(newSymbols),
			OldUniqueSymbols: len(oldSymbols),
			OverlapSymbols:   overlap,
			OverlapPct:       round2(overlapPct),
			NewAvgScore:      round2(newAvg),
			OldAvgScore:      round2(oldAvg),
		},
	}

	// ═══════════════════════════════════════════
	// Output
	// ═══════════════════════════════════════════

	// Print formatted report
	printReport(result)

	// Save JSON
	jsonPath := "cmd/engine_eval/eval_result.json"
	jsonData, _ := json.MarshalIndent(result, "", "  ")
	os.WriteFile(jsonPath, jsonData, 0644)
	fmt.Printf("\n💾 JSON 报告已保存: %s\n", jsonPath)
}

func printReport(r EvalResult) {
	fmt.Println("\n" + strings.Repeat("═", 70))
	fmt.Println("  AiT 新旧引擎实盘对比报告")
	fmt.Println(strings.Repeat("═", 70))

	fmt.Printf("  时间: %s\n", r.Timestamp)
	fmt.Printf("  数据: %d 标的 (详细: %d), 拉取耗时: %s\n",
		r.SnapshotMeta.TotalSymbols, r.SnapshotMeta.DetailedSymbols, r.SnapshotMeta.FetchDuration)

	fmt.Println("\n" + strings.Repeat("─", 70))
	fmt.Println("  📊 总体对比")
	fmt.Println(strings.Repeat("─", 70))
	fmt.Printf("  %-30s %15s %15s\n", "指标", "新引擎(IndicatorHub)", "旧引擎(Hunter+AI500)")
	fmt.Printf("  %-30s %15d %15d\n", "信号总数", r.Comparison.NewTotalSignals, r.Comparison.OldTotalSignals)
	fmt.Printf("  %-30s %15d %15d\n", "去重标的数", r.Comparison.NewUniqueSymbols, r.Comparison.OldUniqueSymbols)
	fmt.Printf("  %-30s %15d %15s\n", "重叠标的", r.Comparison.OverlapSymbols, "-")
	fmt.Printf("  %-30s %14.1f%% %14s\n", "重叠率", r.Comparison.OverlapPct, "-")
	fmt.Printf("  %-30s %14.1f %15.1f\n", "平均分", r.Comparison.NewAvgScore, r.Comparison.OldAvgScore)

	fmt.Println("\n" + strings.Repeat("─", 70))
	fmt.Println("  🚀 新引擎 Top 信号 (IndicatorHub)")
	fmt.Println(strings.Repeat("─", 70))
	if len(r.NewSignals) == 0 {
		fmt.Println("  (无信号)")
	}
	for _, s := range r.NewSignals {
		grade := fmt.Sprintf("[%s]", s.Grade)
		fmt.Printf("  #%-2d %-12s %-7s %-5s 分=%.1f 技术=%.1f 量化=%.1f 社交=%.1f\n",
			s.Rank, s.Symbol, s.Direction, grade, s.FinalScore,
			s.TechScore, s.QuantScore, s.SocScore)
		if len(s.BullSignals) > 0 {
			fmt.Printf("      看多: %s\n", strings.Join(s.BullSignals[:min(3, len(s.BullSignals))], ", "))
		}
		if len(s.BearSignals) > 0 {
			fmt.Printf("      看空: %s\n", strings.Join(s.BearSignals[:min(3, len(s.BearSignals))], ", "))
		}
		fmt.Printf("      入场=%.4f SL=%.4f TP1=%.4f TP2=%.4f\n",
			s.Entry, s.SL, s.TP1, s.TP2)
	}

	fmt.Println("\n" + strings.Repeat("─", 70))
	fmt.Println("  🔄 旧引擎 Top 信号 (Hunter + AI500)")
	fmt.Println(strings.Repeat("─", 70))
	if len(r.OldSignals) == 0 {
		fmt.Println("  (无信号)")
	}
	for _, s := range r.OldSignals {
		dir := s.Direction
		if dir == "" {
			dir = "-"
		}
		fmt.Printf("  #%-2d %-12s [%s] 分=%.1f 方向=%-5s 24h变化=%.2f%%\n",
			s.Rank, s.Symbol, s.Mode, s.Score, dir, s.PriceChange)
		if len(s.SignalTags) > 0 {
			tags := s.SignalTags
			if len(tags) > 4 {
				tags = tags[:4]
			}
			fmt.Printf("      信号: %s\n", strings.Join(tags, ", "))
		}
	}

	fmt.Println("\n" + strings.Repeat("─", 70))
	fmt.Println("  📈 新引擎 Top 50 评分明细")
	fmt.Println(strings.Repeat("─", 70))
	fmt.Printf("  %-4s %-12s %-5s %6s %6s %6s %6s %6s %6s\n",
		"#", "标的", "方向", "总分", "技多", "技空", "量多", "量空", "社多")
	for _, e := range r.Top50Scores {
		fmt.Printf("  %-4d %-12s %-5s %6.1f %6.1f %6.1f %6.1f %6.1f %6.1f\n",
			e.Rank, e.Symbol, e.Direction, e.FinalScore,
			e.TechBull, e.TechBear, e.QuantBull, e.QuantBear, e.SocBull)
	}

	fmt.Println("\n" + strings.Repeat("═", 70))
}

func directionStr(d int) string {
	switch d {
	case 1:
		return "LONG"
	case -1:
		return "SHORT"
	default:
		return "NEUTRAL"
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
