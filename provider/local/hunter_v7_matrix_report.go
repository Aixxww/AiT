package local

import (
	"fmt"
	"sort"
	"strings"
)

// ============================================================================
// Hunter v8 — Matrix Report (v8-SPEC P0-3)
// ============================================================================
// Generates per-setup, per-regime diagnostic statistics from raw signal data.
// Helps identify which regime+setup combos are producing actionable signals
// versus noise.

// MatrixCell holds aggregate statistics for a single regime×setup cell.
type MatrixCell struct {
	Regime         V7MarketRegime `json:"regime"`
	SetupType      V7SetupType    `json:"setup_type"`
	SignalCount    int            `json:"signal_count"`
	ExecCount      int            `json:"exec_count"`
	AvgPriority    float64        `json:"avg_priority"`
	AvgSetupScore  float64        `json:"avg_setup_score"`
	AvgTimingScore float64        `json:"avg_timing_score"`
}

// MatrixReport holds the full diagnostic matrix.
type MatrixReport struct {
	Cells []MatrixCell `json:"cells"`
}

// GenerateMatrixReport builds a per-setup, per-regime report from raw signal
// records.  Uses V7SignalRecord (router-local type) from the signal recorder
// callback.
func GenerateMatrixReport(signals []V7SignalRecord, regime V7MarketRegime) *MatrixReport {
	type cellAccum struct {
		count       int
		execCount   int
		sumPriority float64
		sumSetup    float64
		sumTiming   float64
	}

	cells := make(map[regimeSetupKey]*cellAccum)

	for _, rec := range signals {
		// Filter to requested regime if specified
		if regime != "" && rec.Signal.MarketRegime != regime {
			continue
		}
		key := regimeSetupKey{
			regime: rec.Signal.MarketRegime,
			setup:  rec.Signal.SetupType,
		}
		cell, ok := cells[key]
		if !ok {
			cell = &cellAccum{}
			cells[key] = cell
		}
		cell.count++
		if rec.Tier == "EXECUTABLE" || rec.Tier == "REVIEWABLE" {
			cell.execCount++
		}
		cell.sumPriority += rec.Signal.AIPriority
		cell.sumSetup += rec.Signal.SetupScore
		cell.sumTiming += rec.Signal.TimingScore
	}

	report := &MatrixReport{
		Cells: make([]MatrixCell, 0, len(cells)),
	}

	for key, acc := range cells {
		n := float64(acc.count)
		report.Cells = append(report.Cells, MatrixCell{
			Regime:         key.regime,
			SetupType:      key.setup,
			SignalCount:    acc.count,
			ExecCount:      acc.execCount,
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

// String pretty-prints the matrix report as a table.
func (mr *MatrixReport) String() string {
	if mr == nil || len(mr.Cells) == 0 {
		return "MatrixReport: no data"
	}

	var b strings.Builder
	b.WriteString("╔══════════════════════════════════════════════════════════════════════════════════════════╗\n")
	b.WriteString("║  Hunter v8 — Regime × Setup Matrix Report                                             ║\n")
	b.WriteString("╠══════════════════╦════════════════════════════╦══════╦════════╦═════════╦═════════╦═══════╣\n")
	b.WriteString("║ Regime           ║ Setup Type                 ║  Sig ║  Exec ║ AvgPri  ║ AvgSet  ║ AvgTi ║\n")
	b.WriteString("╠══════════════════╬════════════════════════════╬══════╬════════╬═════════╬═════════╬═══════╣\n")

	for _, c := range mr.Cells {
		fmt.Fprintf(&b, "║ %-16s ║ %-26s ║ %4d ║ %5d ║ %7.1f ║ %7.1f ║ %5.1f ║\n",
			c.Regime, c.SetupType,
			c.SignalCount, c.ExecCount,
			c.AvgPriority, c.AvgSetupScore, c.AvgTimingScore)
	}

	b.WriteString("╚══════════════════╩════════════════════════════╩══════╩════════╩═════════╩═════════╩═══════╝\n")
	return b.String()
}
