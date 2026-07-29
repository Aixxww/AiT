package api

import (
	"fmt"
	"sort"
	"strings"

	local "github.com/Aixxww/AiT/provider/local"
)

// ============================================================================
// Hunter v8 — Matrix Report (v8-SPEC P0-3)
// ============================================================================
// Generates per-setup, per-regime diagnostic statistics from raw signal data.
// Helps identify which regime+setup combos are producing actionable signals
// versus noise.

// MatrixCell holds aggregate statistics for a single regime×setup cell.
type MatrixCell struct {
	Regime         local.V7MarketRegime `json:"regime"`
	SetupType      local.V7SetupType    `json:"setup_type"`
	SignalBucket   string               `json:"signal_bucket"`
	SignalCount    int                  `json:"signal_count"`
	ExecCount      int                  `json:"exec_count"`
	ReviewCount    int                  `json:"review_count"`
	WatchCount     int                  `json:"watch_count"`
	OpenRate       float64              `json:"open_rate"`
	AvgPriority    float64              `json:"avg_priority"`
	AvgSetupScore  float64              `json:"avg_setup_score"`
	AvgTimingScore float64              `json:"avg_timing_score"`
}

// MatrixReport holds the full diagnostic matrix.
type MatrixReport struct {
	Cells              []MatrixCell `json:"cells"`
	TradeSignalCount   int          `json:"trade_signal_count"`
	TradeOpenCount     int          `json:"trade_open_count"`
	TradeOpenRate      float64      `json:"trade_open_rate"`
	ReversalWatchCount int          `json:"reversal_watch_count"`
}

// GenerateMatrixReport builds a per-setup, per-regime report from raw signal
// records.  Uses V7SignalRecord (router-local type) from the signal recorder
// callback.
func GenerateMatrixReport(signals []local.V7SignalRecord, regime local.V7MarketRegime) *MatrixReport {
	type regimeSetupKey struct {
		regime local.V7MarketRegime
		setup  local.V7SetupType
		bucket string
	}
	type cellAccum struct {
		count       int
		execCount   int
		reviewCount int
		watchCount  int
		sumPriority float64
		sumSetup    float64
		sumTiming   float64
	}

	cells := make(map[regimeSetupKey]*cellAccum)
	report := &MatrixReport{}

	for _, rec := range signals {
		// Filter to requested regime if specified
		if regime != "" && rec.Signal.MarketRegime != regime {
			continue
		}
		bucket := hunterV7SignalStatsBucket(rec)
		key := regimeSetupKey{
			regime: rec.Signal.MarketRegime,
			setup:  rec.Signal.SetupType,
			bucket: bucket,
		}
		cell, ok := cells[key]
		if !ok {
			cell = &cellAccum{}
			cells[key] = cell
		}
		cell.count++
		if rec.Tier == "EXECUTABLE" {
			cell.execCount++
		} else if rec.Tier == "REVIEWABLE" {
			cell.reviewCount++
		} else if rec.Tier == "WATCH" {
			cell.watchCount++
		}
		if bucket == "reversal_watch_pool" {
			report.ReversalWatchCount++
		} else {
			report.TradeSignalCount++
			if rec.Tier == "EXECUTABLE" || rec.Tier == "REVIEWABLE" {
				report.TradeOpenCount++
			}
		}
		cell.sumPriority += rec.Signal.AIPriority
		cell.sumSetup += rec.Signal.SetupScore
		cell.sumTiming += rec.Signal.TimingScore
	}

	report.Cells = make([]MatrixCell, 0, len(cells))
	if report.TradeSignalCount > 0 {
		report.TradeOpenRate = float64(report.TradeOpenCount) / float64(report.TradeSignalCount) * 100
	}

	for key, acc := range cells {
		n := float64(acc.count)
		report.Cells = append(report.Cells, MatrixCell{
			Regime:         key.regime,
			SetupType:      key.setup,
			SignalBucket:   key.bucket,
			SignalCount:    acc.count,
			ExecCount:      acc.execCount,
			ReviewCount:    acc.reviewCount,
			WatchCount:     acc.watchCount,
			OpenRate:       float64(acc.execCount+acc.reviewCount) / n * 100,
			AvgPriority:    acc.sumPriority / n,
			AvgSetupScore:  acc.sumSetup / n,
			AvgTimingScore: acc.sumTiming / n,
		})
	}
	sort.Slice(report.Cells, func(i, j int) bool {
		if report.Cells[i].Regime == report.Cells[j].Regime {
			return report.Cells[i].SetupType < report.Cells[j].SetupType
		}
		return report.Cells[i].Regime < report.Cells[j].Regime
	})

	return report
}

func hunterV7SignalStatsBucket(rec local.V7SignalRecord) string {
	if rec.Signal.SetupType == local.V7SetupFundingReversal &&
		rec.Signal.MarketRegime != local.V7RegimeTrendDown &&
		rec.Signal.MarketRegime != local.V7RegimePanicDump {
		return "reversal_watch_pool"
	}
	return "trade_setup_pool"
}

// String pretty-prints the matrix report as a table.
func (mr *MatrixReport) String() string {
	if mr == nil || len(mr.Cells) == 0 {
		return "MatrixReport: no data"
	}

	var b strings.Builder
	b.WriteString("╔══════════════════════════════════════════════════════════════════════════════════════════╗\n")
	b.WriteString("║  Hunter v8 — Regime × Setup Matrix Report                                             ║\n")
	b.WriteString("╠══════════════════╦════════════════════════════╦═════════════════════╦══════╦══════╦══════╦═══════╣\n")
	b.WriteString("║ Regime           ║ Setup Type                 ║ Bucket              ║  Sig ║ Exec ║ Rev  ║ Open% ║\n")
	b.WriteString("╠══════════════════╬════════════════════════════╬═════════════════════╬══════╬══════╬══════╬═══════╣\n")

	for _, c := range mr.Cells {
		fmt.Fprintf(&b, "║ %-16s ║ %-26s ║ %-19s ║ %4d ║ %4d ║ %4d ║ %5.1f ║\n",
			c.Regime, c.SetupType,
			c.SignalBucket, c.SignalCount, c.ExecCount, c.ReviewCount, c.OpenRate)
	}

	fmt.Fprintf(&b, "Trade setup open-rate: %d/%d = %.1f%%; reversal watch rows: %d\n",
		mr.TradeOpenCount, mr.TradeSignalCount, mr.TradeOpenRate, mr.ReversalWatchCount)
	b.WriteString("╚══════════════════╩════════════════════════════╩═════════════════════╩══════╩══════╩══════╩═══════╝\n")
	return b.String()
}
