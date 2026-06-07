package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/Aixxww/AiT/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	dbPath := flag.String("db", "data/data.db", "Path to SQLite database")
	traderID := flag.String("trader-id", "", "Trader ID (required)")
	cycles := flag.Int("cycles", 10, "Number of recent cycles to review")
	output := flag.String("output", "", "Output file path (default: stdout)")
	format := flag.String("format", "md", "Output format: md (markdown) or json")
	flag.Parse()

	if *traderID == "" {
		fmt.Fprintf(os.Stderr, "Error: --trader-id is required\n")
		flag.Usage()
		os.Exit(1)
	}

	logger.InitWithSimpleConfig("warn")

	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}

	report, err := GenerateReport(db, *traderID, *cycles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating report: %v\n", err)
		os.Exit(1)
	}

	var outputText string
	switch *format {
	case "json":
		outputText = formatJSON(report)
	default:
		outputText = formatMarkdown(report)
	}

	if *output != "" {
		if err := os.WriteFile(*output, []byte(outputText), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report written to %s\n", *output)
	} else {
		fmt.Print(outputText)
	}
}

func formatMarkdown(r *CycleReviewReport) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# AiT Cycle Review Report\n\n"))
	b.WriteString(fmt.Sprintf("**Generated**: %s\n", r.GeneratedAt.Format("2006-01-02 15:04:05 CST")))
	b.WriteString(fmt.Sprintf("**Trader ID**: %s\n", r.TraderID))
	b.WriteString(fmt.Sprintf("**Cycle Range**: %s (%d cycles)\n\n", r.CycleRange, r.TotalCycles))

	// Overview
	b.WriteString("## Overview\n\n")
	b.WriteString(fmt.Sprintf("| Metric | Value |\n|---|---|\n"))
	b.WriteString(fmt.Sprintf("| Total Cycles | %d |\n", r.TotalCycles))
	b.WriteString(fmt.Sprintf("| Successful | %d |\n", r.SuccessCount))
	b.WriteString(fmt.Sprintf("| Failed | %d |\n", r.FailedCount))
	b.WriteString(fmt.Sprintf("| Open Trades | %d |\n", r.OpenCount))
	b.WriteString(fmt.Sprintf("| Wait | %d |\n", r.WaitCount))
	if r.TotalCycles > 0 {
		openRate := float64(r.OpenCount) / float64(r.TotalCycles) * 100
		b.WriteString(fmt.Sprintf("| Open Rate | %.1f%% |\n", openRate))
	}
	b.WriteString("\n")

	// Per-cycle summary
	b.WriteString("## Per-Cycle Summary\n\n")
	b.WriteString("| # | Time | Candidates | Action | Symbol | Tokens | Duration |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, c := range r.Cycles {
		status := "OK"
		if !c.Success {
			status = "FAIL"
		}
		cands := len(c.Candidates)
		b.WriteString(fmt.Sprintf("| %d | %s | %d | %s | %s | %d | %dms |\n",
			c.CycleNumber,
			c.Timestamp.Format("15:04:05"),
			cands,
			c.Action,
			c.Symbol,
			c.TotalTokens,
			c.DurationMs,
		))
		_ = status
	}
	b.WriteString("\n")

	// PnL
	if r.PnL.TotalClosed > 0 {
		b.WriteString("## PnL Summary (Closed Positions)\n\n")
		b.WriteString(fmt.Sprintf("| Metric | Value |\n|---|---|\n"))
		b.WriteString(fmt.Sprintf("| Total Closed | %d |\n", r.PnL.TotalClosed))
		b.WriteString(fmt.Sprintf("| Wins | %d |\n", r.PnL.WinCount))
		b.WriteString(fmt.Sprintf("| Losses | %d |\n", r.PnL.LossCount))
		b.WriteString(fmt.Sprintf("| Win Rate | %.1f%% |\n", r.PnL.WinRate))
		b.WriteString(fmt.Sprintf("| Net PnL | %.2f USDT |\n", r.PnL.TotalPnL))
		b.WriteString(fmt.Sprintf("| Max Win | %.2f USDT |\n", r.PnL.MaxWin))
		b.WriteString(fmt.Sprintf("| Max Loss | %.2f USDT |\n", r.PnL.MaxLoss))
		b.WriteString("\n")

		if len(r.PnL.Positions) > 0 {
			b.WriteString("### Recent Closed Positions\n\n")
			b.WriteString("| Symbol | Side | Entry | Exit | PnL | Reason |\n")
			b.WriteString("|---|---|---|---|---|---|\n")
			for _, p := range r.PnL.Positions {
				b.WriteString(fmt.Sprintf("| %s | %s | %.4f | %.4f | %.2f | %s |\n",
					p.Symbol, p.Side, p.EntryPrice, p.ExitPrice, p.PnL, p.CloseReason))
			}
			b.WriteString("\n")
		}
	}

	// Token usage
	b.WriteString("## Token Usage\n\n")
	b.WriteString(fmt.Sprintf("| Metric | Value |\n|---|---|\n"))
	b.WriteString(fmt.Sprintf("| Total Tokens | %d |\n", r.TokenUsage.TotalTokens))
	b.WriteString(fmt.Sprintf("| Avg per Cycle | %.0f |\n", r.TokenUsage.AvgPerCycle))
	b.WriteString(fmt.Sprintf("| Max per Cycle | %d |\n", r.TokenUsage.MaxPerCycle))
	b.WriteString("\n")

	// Guard blocks
	if len(r.GuardBlocks) > 0 {
		b.WriteString("## Execution Guard Blocks\n\n")
		b.WriteString(fmt.Sprintf("Total blocks: %d\n\n", len(r.GuardBlocks)))
		for _, gb := range r.GuardBlocks {
			b.WriteString(fmt.Sprintf("- Cycle #%d (%s): %s\n", gb.CycleNumber, gb.Symbol, gb.Reason))
		}
		b.WriteString("\n")
	}

	// Suggestions
	if len(r.Suggestions) > 0 {
		b.WriteString("## Suggestions\n\n")
		for _, s := range r.Suggestions {
			b.WriteString(fmt.Sprintf("- %s\n", s))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func formatJSON(r *CycleReviewReport) string {
	// Simple JSON output without importing encoding/json in main
	// For complex usage, import encoding/json
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "generated_at": "%s",`, r.GeneratedAt.Format(time.RFC3339)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "trader_id": "%s",`, r.TraderID))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "cycle_range": "%s",`, r.CycleRange))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "total_cycles": %d,`, r.TotalCycles))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "success_count": %d,`, r.SuccessCount))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "failed_count": %d,`, r.FailedCount))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "open_count": %d,`, r.OpenCount))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "wait_count": %d,`, r.WaitCount))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "total_tokens": %d,`, r.TokenUsage.TotalTokens))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "avg_tokens_per_cycle": %.0f,`, r.TokenUsage.AvgPerCycle))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "pnl_total": %.2f,`, r.PnL.TotalPnL))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "pnl_win_rate": %.1f,`, r.PnL.WinRate))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  "guard_blocks": %d`, len(r.GuardBlocks)))
	b.WriteString("\n")

	if len(r.Suggestions) > 0 {
		b.WriteString(`  "suggestions": [`)
		b.WriteString("\n")
		for i, s := range r.Suggestions {
			escaped := strings.ReplaceAll(s, `"`, `\"`)
			b.WriteString(fmt.Sprintf(`    "%s"`, escaped))
			if i < len(r.Suggestions)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("  ]\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// roundFloat rounds to n decimal places
func roundFloat(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
