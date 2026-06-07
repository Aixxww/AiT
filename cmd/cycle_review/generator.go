package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Aixxww/AiT/store"
	"gorm.io/gorm"
)

// CycleReviewReport holds the generated review data.
type CycleReviewReport struct {
	GeneratedAt  time.Time
	TraderID     string
	CycleRange   string
	TotalCycles  int
	SuccessCount int
	FailedCount  int
	OpenCount    int
	WaitCount    int
	Cycles       []CycleSummary
	PnL          PnLSummary
	TokenUsage   TokenSummary
	GuardBlocks  []GuardBlock
	Suggestions  []string
}

// CycleSummary is a per-cycle overview.
type CycleSummary struct {
	CycleNumber  int
	Timestamp    time.Time
	Candidates   []string
	Action       string // "open_long", "open_short", "wait", "close", "error"
	Symbol       string
	SetupType    string
	Success      bool
	ErrorMessage string
	TotalTokens  int
	DurationMs   int64
}

// PnLSummary aggregates PnL from closed positions.
type PnLSummary struct {
	TotalClosed int
	WinCount    int
	LossCount   int
	WinRate     float64
	TotalPnL    float64
	MaxWin      float64
	MaxLoss     float64
	Positions   []PositionPnL
}

// PositionPnL is a single closed position's PnL.
type PositionPnL struct {
	Symbol      string
	Side        string
	EntryPrice  float64
	ExitPrice   float64
	PnL         float64
	CloseReason string
}

// TokenSummary aggregates token usage.
type TokenSummary struct {
	TotalTokens int
	AvgPerCycle float64
	MaxPerCycle int
}

// GuardBlock records an execution guard rejection.
type GuardBlock struct {
	CycleNumber int
	Symbol      string
	Reason      string
}

// GenerateReport creates a structured review from the last N cycles.
func GenerateReport(db *gorm.DB, traderID string, cycles int) (*CycleReviewReport, error) {
	decisionStore := store.NewDecisionStore(db)
	positionStore := store.NewPositionStore(db)

	records, err := decisionStore.GetLatestRecords(traderID, cycles)
	if err != nil {
		return nil, fmt.Errorf("failed to load decision records: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no decision records found for trader %s", traderID)
	}

	report := &CycleReviewReport{
		GeneratedAt: time.Now(),
		TraderID:    traderID,
		TotalCycles: len(records),
	}

	// Determine cycle range
	firstCycle := records[0].CycleNumber
	lastCycle := records[len(records)-1].CycleNumber
	report.CycleRange = fmt.Sprintf("#%d - #%d", firstCycle, lastCycle)

	// Process each cycle
	var totalTokens int
	var maxTokens int
	for _, rec := range records {
		cs := CycleSummary{
			CycleNumber: rec.CycleNumber,
			Timestamp:   rec.Timestamp,
			Candidates:  rec.CandidateCoins,
			Success:     rec.Success,
			TotalTokens: rec.TotalTokens,
			DurationMs:  rec.AIRequestDurationMs,
		}

		if rec.Success {
			report.SuccessCount++
		} else {
			report.FailedCount++
		}

		// Determine primary action from decisions
		cs.Action = "wait"
		for _, d := range rec.Decisions {
			if d.Action == "open_long" || d.Action == "open_short" {
				cs.Action = d.Action
				cs.Symbol = d.Symbol
				if d.Success {
					report.OpenCount++
				}
				break
			} else if d.Action == "close_long" || d.Action == "close_short" {
				cs.Action = d.Action
				cs.Symbol = d.Symbol
			}
		}
		if cs.Action == "wait" {
			report.WaitCount++
		}

		// Check for guard blocks in execution log
		for _, log := range rec.ExecutionLog {
			if strings.Contains(log, "GUARD") && strings.Contains(log, "blocked") {
				report.GuardBlocks = append(report.GuardBlocks, GuardBlock{
					CycleNumber: rec.CycleNumber,
					Symbol:      cs.Symbol,
					Reason:      log,
				})
			}
		}

		// Check for guard blocks in decision errors
		for _, d := range rec.Decisions {
			if d.Error != "" && strings.Contains(d.Error, "GUARD") {
				report.GuardBlocks = append(report.GuardBlocks, GuardBlock{
					CycleNumber: rec.CycleNumber,
					Symbol:      d.Symbol,
					Reason:      d.Error,
				})
			}
		}

		totalTokens += rec.TotalTokens
		if rec.TotalTokens > maxTokens {
			maxTokens = rec.TotalTokens
		}

		report.Cycles = append(report.Cycles, cs)
	}

	// Token summary
	report.TokenUsage = TokenSummary{
		TotalTokens: totalTokens,
		MaxPerCycle: maxTokens,
	}
	if len(records) > 0 {
		report.TokenUsage.AvgPerCycle = float64(totalTokens) / float64(len(records))
	}

	// PnL from closed positions
	closedPositions, err := positionStore.GetClosedPositions(traderID, 100)
	if err == nil {
		report.PnL = computePnL(closedPositions)
	}

	// Generate suggestions
	report.Suggestions = generateSuggestions(report)

	return report, nil
}

func computePnL(positions []*store.TraderPosition) PnLSummary {
	summary := PnLSummary{TotalClosed: len(positions)}
	for _, pos := range positions {
		pnl := pos.RealizedPnL - pos.Fee
		summary.TotalPnL += pnl

		pp := PositionPnL{
			Symbol:      pos.Symbol,
			Side:        pos.Side,
			EntryPrice:  pos.EntryPrice,
			ExitPrice:   pos.ExitPrice,
			PnL:         pnl,
			CloseReason: pos.CloseReason,
		}
		summary.Positions = append(summary.Positions, pp)

		if pnl > 0 {
			summary.WinCount++
			if pnl > summary.MaxWin {
				summary.MaxWin = pnl
			}
		} else {
			summary.LossCount++
			if pnl < summary.MaxLoss {
				summary.MaxLoss = pnl
			}
		}
	}
	if summary.TotalClosed > 0 {
		summary.WinRate = float64(summary.WinCount) / float64(summary.TotalClosed) * 100
	}
	return summary
}

func generateSuggestions(report *CycleReviewReport) []string {
	var suggestions []string

	// WAIT/OPEN ratio analysis
	if report.TotalCycles > 0 {
		openRate := float64(report.OpenCount) / float64(report.TotalCycles) * 100
		waitRate := float64(report.WaitCount) / float64(report.TotalCycles) * 100

		if openRate < 10 {
			suggestions = append(suggestions, fmt.Sprintf("开仓率仅 %.1f%%，考虑放宽 MinAIPriority 或降低 entry_standards", openRate))
		} else if openRate > 40 {
			suggestions = append(suggestions, fmt.Sprintf("开仓率 %.1f%% 偏高，考虑收紧 entry_standards 或增加冷静期", openRate))
		}

		if waitRate > 90 {
			suggestions = append(suggestions, fmt.Sprintf("WAIT 比例 %.1f%%，候选信号可能过于保守，检查筛选层阈值", waitRate))
		}
	}

	// PnL analysis
	if report.PnL.TotalClosed > 0 {
		if report.PnL.WinRate < 50 {
			suggestions = append(suggestions, fmt.Sprintf("胜率 %.1f%% 低于 50%%，检查入场 timing 和止损距离设置", report.PnL.WinRate))
		}
		if report.PnL.TotalPnL < 0 {
			suggestions = append(suggestions, fmt.Sprintf("净 PnL %.2f USDT 为负，考虑收紧 execution guard 或增加过滤条件", report.PnL.TotalPnL))
		}
		if report.PnL.MaxLoss < 0 && math.Abs(report.PnL.MaxLoss) > report.PnL.MaxWin*2 {
			suggestions = append(suggestions, fmt.Sprintf("最大单笔亏损 %.2f 远大于最大盈利 %.2f，需检查止损策略", report.PnL.MaxLoss, report.PnL.MaxWin))
		}
	}

	// Guard block analysis
	if len(report.GuardBlocks) > 0 {
		suggestions = append(suggestions, fmt.Sprintf("Execution guard 拦截 %d 次，可分析拦截后的价格走势验证规则是否合理", len(report.GuardBlocks)))
	}

	// Failure rate
	if report.TotalCycles > 0 && report.FailedCount > 0 {
		failRate := float64(report.FailedCount) / float64(report.TotalCycles) * 100
		if failRate > 10 {
			suggestions = append(suggestions, fmt.Sprintf("周期失败率 %.1f%%，检查 API 连接和降级机制", failRate))
		}
	}

	return suggestions
}
