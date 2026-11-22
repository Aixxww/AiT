package main

import (
	"time"

	"github.com/Aixxww/AiT/trader"
)

// Non-walk statuses for rows the live walk cannot evaluate.
const (
	skipNoGeometry       = "skip_no_stop_or_tp1"   // stop or TP1 <= 0: the live walk never starts (outcome stays "")
	skipHorizonOpen      = "skip_horizon_not_over" // signal time + horizon is not in the past yet
	skipKlinesError      = "skip_klines_error"     // public klines could not be read
	skipIncompleteKlines = "skip_incomplete_klines"
)

// walkResult is the live fee-net outcome of one row.
type walkResult struct {
	Status    string   `json:"status"`          // "walked" or a skip_* status
	Outcome   string   `json:"fee_net_outcome"` // store.V7FeeNet* when walked
	R         *float64 `json:"fee_net_r,omitempty"`
	Entry     float64  `json:"fee_net_entry,omitempty"`
	DriftPct  *float64 `json:"drift_at_entry_pct,omitempty"`
	Bars      int      `json:"bars"`
	GapMins   int      `json:"gap_minutes,omitempty"` // missing 1m bars inside the supplied window
	SkipError string   `json:"skip_error,omitempty"`
}

// walkConfig is the live tracker's fee-net configuration.
type walkConfig struct {
	Horizon time.Duration // TrackerConfig.TimeoutDuration (harness 2h, live default 8h)
	CostPct float64       // TrackerConfig.FeeNetCostPct
}

// walkRow runs the production SignalOutcomeTracker (trader package, OD-2
// stepFeeNet) over one row with public 1m bars, exactly as live: one
// tracker per row, so the live duplicate-thesis replacement and capacity
// limits do not apply (each row is walked on its own). Bars are fed with
// their close time, as the live candle sources do.
func walkRow(in inputRow, wc walkConfig, ks *klineStore, now time.Time) walkResult {
	if in.Stop <= 0 || in.TP1 <= 0 {
		return walkResult{Status: skipNoGeometry}
	}
	end := in.Time.Add(wc.Horizon).Add(2 * time.Minute)
	if end.After(now) {
		return walkResult{Status: skipHorizonOpen}
	}
	from := in.Time.Truncate(time.Minute)
	bars, err := ks.window(in.Symbol, from, end)
	if err != nil {
		return walkResult{Status: skipKlinesError, SkipError: err.Error()}
	}
	res := walkResult{Bars: len(bars), GapMins: gapMinutes(bars, from.Add(time.Minute), in.Time.Add(wc.Horizon))}

	cfg := trader.DefaultTrackerConfig()
	cfg.TimeoutDuration = wc.Horizon
	cfg.FeeNetCostPct = wc.CostPct
	tr := trader.NewSignalOutcomeTracker(cfg, nil)
	tr.SetCandleHistorySource(func(_ string, since time.Time) []trader.TrackedCandle {
		var out []trader.TrackedCandle
		for _, b := range bars {
			ct := time.UnixMilli(b.OpenMs + 60000 - 1) // live candles carry the close time
			if ct.After(since) {
				out = append(out, trader.TrackedCandle{T: ct, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close})
			}
		}
		return out
	})
	var last trader.TrackedOutcome
	tr.SetOutcomeCallback(func(o trader.TrackedOutcome) {
		if o.FeeNetOutcome != "" {
			last = o
		}
	})
	entry := in.Entry
	if entry <= 0 {
		entry = in.Stop // PnL path only; the fee-net walk does not read it
	}
	ok, _ := tr.RegisterWithMeta(trader.TrackedSignalMeta{OutcomeSource: in.OutcomeSource, TriggerPrice: in.Trigger},
		in.ID, in.Symbol, in.Direction, in.Setup, in.Tier, entry, in.Stop, in.TP0, in.TP1, in.TP2, in.Time)
	if !ok {
		return walkResult{Status: skipKlinesError, SkipError: "tracker refused registration"}
	}
	tr.TickNow()
	if last.FeeNetOutcome == "" {
		res.Status = skipIncompleteKlines
		return res
	}
	// A walk the live code closed only because wall-clock time passed the
	// horizon ("none" / "no_anchor") is trusted only when the bars reached
	// past the horizon; otherwise the window was cut short by missing data.
	if (last.FeeNetOutcome == "none" || last.FeeNetOutcome == "no_anchor") && !barsPastHorizon(bars, in.Time, wc.Horizon) {
		res.Status = skipIncompleteKlines
		return res
	}
	res.Status = "walked"
	res.Outcome, res.R, res.Entry, res.DriftPct = last.FeeNetOutcome, last.FeeNetR, last.FeeNetEntry, last.DriftAtEntryPct
	return res
}

// barsPastHorizon reports whether a bar opens after signal time + horizon
// (the bar on which the live walk closes an unresolved walk).
func barsPastHorizon(bars []bar, signal time.Time, horizon time.Duration) bool {
	limit := signal.Add(horizon)
	for i := len(bars) - 1; i >= 0; i-- {
		if time.UnixMilli(bars[i].OpenMs).Truncate(time.Minute).After(limit) {
			return true
		}
	}
	return false
}

// gapMinutes counts minutes in [from, to] with no bar.
func gapMinutes(bars []bar, from, to time.Time) int {
	have := make(map[int64]bool, len(bars))
	for _, b := range bars {
		have[time.UnixMilli(b.OpenMs).Truncate(time.Minute).UnixMilli()] = true
	}
	gaps := 0
	for t := from.Truncate(time.Minute); !t.After(to); t = t.Add(time.Minute) {
		if !have[t.UnixMilli()] {
			gaps++
		}
	}
	return gaps
}
