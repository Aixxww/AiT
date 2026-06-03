package main

import (
	"fmt"
	"nofx/datafetch"
	"nofx/engine"
	"nofx/provider/local"
	"os"
	"sort"
	"strings"
	"time"
)

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  AiT 数据源引擎实测对比 —", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Step 1: Fetch real-time data via SymbolCache (the shared data layer)
	fmt.Println("📡 Step 1: Fetching real-time data from Binance (top 50 USDT perps)...")
	t0 := time.Now()
	client := local.NewClient("")
	caches, err := client.GetSymbolCachesForEval(50)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to fetch symbol caches: %v\n", err)
		os.Exit(1)
	}
	fetchDur := time.Since(t0)
	fmt.Printf("   ✅ Fetched %d symbols in %v\n\n", len(caches), fetchDur)

	// Convert to snapshot for snapshot-based scorers
	snap := local.CachesToSnapshot(caches)

	// ═══════════════════════════════════════════════════════════════
	// MODE 1: AI500 (Snapshot)
	// ═══════════════════════════════════════════════════════════════
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("⚡ MODE 1: AI500 (波动率+成交量+活跃度 综合评分)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t1 := time.Now()
	ai500Coins, err := local.ScoreAI500FromSnapshot(snap, 30)
	ai500Dur := time.Since(t1)
	if err != nil {
		fmt.Printf("   ❌ AI500 error: %v\n", err)
	} else {
		fmt.Printf("   耗时: %v | 输出: %d 标的\n", ai500Dur, len(ai500Coins))
		fmt.Println()
		for i, c := range ai500Coins {
			tags := ""
			if len(c.SignalTags) > 0 {
				tags = fmt.Sprintf(" [%s]", strings.Join(c.SignalTags, ", "))
			}
			fmt.Printf("   %2d. %-15s score=%-6.1f price=%-12s chg24h=%+.2f%%%s\n",
				i+1, c.Pair, c.Score, formatPrice(c.StartPrice), c.IncreasePercent, tags)
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════
	// MODE 2: Hunter Default (Snapshot — 4-Pillar 均值回归)
	// ═══════════════════════════════════════════════════════════════
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🎯 MODE 2: Hunter Default (4-Pillar 双向评分 — 均值回归)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	hunterCfg := local.HunterSnapshotConfig{MinOIValue: 500_000, MaxSymbols: 50}
	t2 := time.Now()
	hunterScores := local.ScoreHunterFromSnapshot(snap, hunterCfg)
	hunterDur := time.Since(t2)

	// Sort by final score (higher of long/short)
	sort.Slice(hunterScores, func(i, j int) bool {
		fi := hunterScores[i].FinalScore
		if hunterScores[i].ShortFinalScore > fi {
			fi = hunterScores[i].ShortFinalScore
		}
		fj := hunterScores[j].FinalScore
		if hunterScores[j].ShortFinalScore > fj {
			fj = hunterScores[j].ShortFinalScore
		}
		return fi > fj
	})

	longCount, shortCount := 0, 0
	for _, s := range hunterScores {
		if s.Direction == "LONG" {
			longCount++
		} else if s.Direction == "SHORT" {
			shortCount++
		}
	}

	fmt.Printf("   耗时: %v | 评分: %d 标的 | LONG: %d | SHORT: %d\n", hunterDur, len(hunterScores), longCount, shortCount)
	fmt.Println()
	fmt.Println("   # | Symbol         | Dir   | Final | PosScore | OISmart | SmartMoney | Squeeze | WashMod | Tags")
	fmt.Println("   ──┼────────────────┼───────┼───────┼──────────┼─────────┼────────────┼─────────┼─────────┼────")
	for i, s := range hunterScores {
		if i >= 30 {
			break
		}
		final := s.FinalScore
		if s.ShortFinalScore > final {
			final = s.ShortFinalScore
		}
		if final < 15 {
			continue
		}
		posScore := s.PositionScore
		oiSmart := s.OISmartScore
		smartMoney := s.SmartMoneyScore
		if s.Direction == "SHORT" {
			posScore = s.ShortPositionScore
			oiSmart = s.ShortOISmartScore
			smartMoney = s.ShortSmartMoneyScore
		}
		tags := strings.Join(s.Tags, ", ")
		if len(tags) > 40 {
			tags = tags[:40] + "..."
		}
		sqTags := 0
		for _, t := range s.Tags {
			if strings.Contains(t, "squeeze") || strings.Contains(t, "oi_spike") {
				sqTags++
			}
		}
		fmt.Printf("  %2d | %-14s | %-5s | %5.1f | %8.1f | %7.1f | %10.1f | %7d | %7.2f | %s\n",
			i+1, s.Symbol, s.Direction, final, posScore, oiSmart, smartMoney,
			sqTags, s.WashMod, tags)
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════
	// MODE 3: Hunter Breakout (Snapshot — 妖币猎杀)
	// ═══════════════════════════════════════════════════════════════
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🔥 MODE 3: Hunter Breakout (BB Squeeze + OI Spike — 妖币猎杀)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// For breakout mode, we filter candidates that have bb_squeeze tags
	// In the real system, breakout mode applies BB Width coarse filter (bottom 25%)
	// Here we filter by squeeze-related tags from the default hunter scores
	var breakoutCandidates []local.HunterCoinScore
	for _, s := range hunterScores {
		hasSqueeze := false
		for _, tag := range s.Tags {
			if strings.Contains(tag, "squeeze") || strings.Contains(tag, "oi_spike") {
				hasSqueeze = true
				break
			}
		}
		if hasSqueeze {
			breakoutCandidates = append(breakoutCandidates, s)
		}
	}

	fmt.Printf("   耗时: %v (共享 Hunter 评分) | Squeeze/OI Spike 标的: %d\n", hunterDur, len(breakoutCandidates))
	fmt.Println()
	for i, s := range breakoutCandidates {
		if i >= 20 {
			break
		}
		final := s.FinalScore
		if s.ShortFinalScore > final {
			final = s.ShortFinalScore
		}
		tags := strings.Join(s.Tags, ", ")
		fmt.Printf("   %2d. %-14s dir=%-5s final=%-5.1f squeeze_tags=%-2d tags=[%s]\n",
			i+1, s.Symbol, s.Direction, final, countSqueezeTags(s.Tags), tags)
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════
	// MODE 4: Hunter Sniff (5-Gate 机构埋伏检测)
	// ═══════════════════════════════════════════════════════════════
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📡 MODE 4: Hunter Sniff (5-Gate 机构埋伏/派发检测)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var longAmbush, shortDist []local.HunterCoinScore
	blockStats := map[string]int{
		"direction": 0, "score": 0, "squeeze": 0,
		"signal": 0, "wall": 0, "wash": 0, "passed": 0,
	}

	for _, s := range hunterScores {
		// Gate 1: Direction + Score
		if s.Direction == "LONG" && s.FinalScore >= 20 {
			// Gate 2: Must have bb_squeeze_15m
			if !hasTag(s.Tags, "bb_squeeze_15m") {
				blockStats["squeeze"]++
				continue
			}
			// Gate 3: Smart money signal
			if !hasAnyTag(s.Tags, "oi_accumulation", "taker_sustained_buying", "lsr_reversal") {
				blockStats["signal"]++
				continue
			}
			// Gate 4: No resistance wall
			if hasAnyTag(s.Tags, "near_resistance_4h", "near_resistance_1d") {
				blockStats["wall"]++
				continue
			}
			// Gate 5: No wash trade
			if hasAnyTag(s.Tags, "wash_micro_trades", "wash_fake_volume") {
				blockStats["wash"]++
				continue
			}
			blockStats["passed"]++
			longAmbush = append(longAmbush, s)
		} else if s.Direction == "SHORT" && s.ShortFinalScore >= 20 {
			if !hasTag(s.Tags, "bb_squeeze_15m") {
				blockStats["squeeze"]++
				continue
			}
			if !hasAnyTag(s.Tags, "oi_distribution", "taker_sustained_selling", "lsr_bearish_reversal", "lsr_bearish_strong") {
				blockStats["signal"]++
				continue
			}
			if hasAnyTag(s.Tags, "near_support_4h", "near_support_1d") {
				blockStats["wall"]++
				continue
			}
			if hasAnyTag(s.Tags, "wash_micro_trades", "wash_fake_volume") {
				blockStats["wash"]++
				continue
			}
			blockStats["passed"]++
			shortDist = append(shortDist, s)
		} else {
			blockStats["direction"]++
		}
	}

	fmt.Printf("   LONG_AMBUSH: %d | SHORT_DISTRIBUTION: %d\n", len(longAmbush), len(shortDist))
	fmt.Printf("   过滤统计: passed=%d | blocked: direction=%d score=%d squeeze=%d signal=%d wall=%d wash=%d\n",
		blockStats["passed"], blockStats["direction"], blockStats["score"],
		blockStats["squeeze"], blockStats["signal"], blockStats["wall"], blockStats["wash"])
	fmt.Println()

	if len(longAmbush) > 0 {
		fmt.Println("   🟢 LONG_AMBUSH (机构吸筹):")
		for i, s := range longAmbush {
			tags := strings.Join(s.Tags, ", ")
			fmt.Printf("      %d. %-14s score=%-5.1f tags=[%s]\n", i+1, s.Symbol, s.FinalScore, tags)
		}
	}
	if len(shortDist) > 0 {
		fmt.Println("   🔴 SHORT_DISTRIBUTION (机构派发):")
		for i, s := range shortDist {
			tags := strings.Join(s.Tags, ", ")
			fmt.Printf("      %d. %-14s score=%-5.1f tags=[%s]\n", i+1, s.Symbol, s.ShortFinalScore, tags)
		}
	}
	if len(longAmbush) == 0 && len(shortDist) == 0 {
		fmt.Println("   ⚠️  当前无标的通过 5-Gate 共振过滤 (bb_squeeze 条件严格)")
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════
	// MODE 5: IndicatorHub (统一评分引擎)
	// ═══════════════════════════════════════════════════════════════
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🔧 MODE 5: IndicatorHub (技术40% + 量化40% + 社交20% 统一引擎)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	hubCfg := engine.HubConfig{
		TechWeight:         40,
		QuantWeight:        40,
		SocialWeight:       20,
		GradeSThreshold:    80,
		GradeAThreshold:    65,
		GradeBThreshold:    50,
		DirectionMargin:    15,
		StopLossATR:        2.0,
		TP1ATR:             1.5,
		TP2ATR:             3.0,
		TP3ATR:             5.0,
		MaxSignalsPerCycle: 10,
		MinScore:           50,
		CooldownMinutes:    0, // No cooldown for test
		TopNForScoring:     50,
	}

	// Create a fresh store with the snapshot data
	hubStore := datafetch.NewStore()
	hubStore.Swap(snap)

	t5 := time.Now()
	mainEngine := engine.NewMainEngine(hubStore, hubCfg)
	signals, err := mainEngine.RunCycle()
	hubDur := time.Since(t5)

	if err != nil {
		fmt.Printf("   ❌ IndicatorHub error: %v\n", err)
	} else {
		fmt.Printf("   耗时: %v | 信号数: %d\n", hubDur, len(signals))
		fmt.Println()
		if len(signals) > 0 {
			fmt.Println("   # | Symbol         | Dir   | Score | Grade | Tech | Quant | Social | SL       | TP1      | Reasons")
			fmt.Println("   ──┼────────────────┼───────┼───────┼───────┼──────┼───────┼────────┼──────────┼──────────┼────────")
			for i, sig := range signals {
				dirStr := "LONG"
				if sig.Direction < 0 {
					dirStr = "SHORT"
				}
				gradeStr := sig.Grade.String()
				techBull := 0.0
				quantBull := 0.0
				socialBull := 0.0
				// Extract sub-scores from the signal's reasons
				reasons := strings.Join(sig.BullSignals, "; ")
				if sig.Direction < 0 {
					reasons = strings.Join(sig.BearSignals, "; ")
				}
				if len(reasons) > 50 {
					reasons = reasons[:50] + "..."
				}
				fmt.Printf("  %2d | %-14s | %-5s | %5.1f | %-5s | %4.0f | %5.0f | %6.0f | %8.4f | %8.4f | %s\n",
					i+1, sig.Symbol, dirStr, sig.FinalScore, gradeStr,
					techBull, quantBull, socialBull,
					sig.StopLoss, sig.TP1, reasons)
			}
		} else {
			fmt.Println("   ⚠️  无信号输出 (MinScore=50 门槛)")
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════
	// SUMMARY COMPARISON
	// ═══════════════════════════════════════════════════════════════
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  📊 综合对比摘要")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  数据获取: %d 标的, %v\n", len(caches), fetchDur)
	fmt.Println()
	fmt.Printf("  %-25s | %6s | %6s | %s\n", "模式", "标的数", "耗时", "特点")
	fmt.Printf("  %-25s─┼─%6s─┼─%6s─┼────────────────────\n", "─────────────────────────", "──────", "──────")
	fmt.Printf("  %-25s | %6d | %6v | 波动率50%%+量25%%+活跃25%%\n", "AI500", len(ai500Coins), ai500Dur)
	hunterPassed := 0
	for _, s := range hunterScores {
		if s.FinalScore >= 15 || s.ShortFinalScore >= 15 {
			hunterPassed++
		}
	}
	fmt.Printf("  %-25s | %6d | %6v | 4-Pillar双向+冷却+刷量惩罚\n", "Hunter Default", hunterPassed, hunterDur)
	fmt.Printf("  %-25s | %6d | %6v | BB Squeeze+OI Spike 子集\n", "Hunter Breakout", len(breakoutCandidates), hunterDur)
	fmt.Printf("  %-25s | %6d | %6v | 5-Gate共振(需bb_squeeze)\n", "Hunter Sniff", len(longAmbush)+len(shortDist), hunterDur)
	if signals != nil {
		fmt.Printf("  %-25s | %6d | %6v | Tech+Quant+Social 三层加权\n", "IndicatorHub", len(signals), hubDur)
	} else {
		fmt.Printf("  %-25s | %6d | %6v | Tech+Quant+Social 三层加权\n", "IndicatorHub", 0, hubDur)
	}
	fmt.Println()

	// Collect all unique symbols across modes
	ai500Symbols := make(map[string]bool)
	for _, c := range ai500Coins {
		ai500Symbols[c.Pair] = true
	}
	hunterSymbols := make(map[string]bool)
	for _, s := range hunterScores {
		if s.FinalScore >= 15 || s.ShortFinalScore >= 15 {
			hunterSymbols[s.Symbol] = true
		}
	}

	// Overlap analysis
	overlap := 0
	for sym := range ai500Symbols {
		if hunterSymbols[sym] {
			overlap++
		}
	}
	fmt.Printf("  AI500 ∩ Hunter 重叠: %d 标的\n", overlap)

	// Hunter-only (not in AI500)
	hunterOnly := 0
	for sym := range hunterSymbols {
		if !ai500Symbols[sym] {
			hunterOnly++
		}
	}
	fmt.Printf("  Hunter 独有: %d 标的\n", hunterOnly)
	fmt.Println()

	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  测试完成 —", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("═══════════════════════════════════════════════════════════════")
}

func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

func hasAnyTag(tags []string, targets ...string) bool {
	for _, t := range tags {
		for _, target := range targets {
			if t == target {
				return true
			}
		}
	}
	return false
}

func countSqueezeTags(tags []string) int {
	count := 0
	for _, tag := range tags {
		if strings.Contains(tag, "squeeze") || strings.Contains(tag, "oi_spike") {
			count++
		}
	}
	return count
}

func formatPrice(p float64) string {
	if p >= 1000 {
		return fmt.Sprintf("%.2f", p)
	} else if p >= 1 {
		return fmt.Sprintf("%.4f", p)
	}
	return fmt.Sprintf("%.6f", p)
}

func formatVolume(v float64) string {
	if v >= 1e9 {
		return fmt.Sprintf("%.1fB", v/1e9)
	} else if v >= 1e6 {
		return fmt.Sprintf("%.1fM", v/1e6)
	} else if v >= 1e3 {
		return fmt.Sprintf("%.1fK", v/1e3)
	}
	return fmt.Sprintf("%.0f", v)
}
