// Command t2_m2_eval runs the pre-registered M2 evaluation of the
// structural geometry (provider/local/hunter_v7_structure.go) over the
// 694-event task-2 entry-study cache.
//
// Protocol (fixed before any results were seen; see M2_REPORT.md):
//
//   - Decision anchor: E = close of the 1m bar starting at
//     floor(trig_ms/60000)*60000 + 60000 (the "place_min" bar of the
//     consumed study). Decision time = that bar's close.
//   - Structure inputs: lookback 15m/1h bars whose close time is <= the
//     decision time (the builder filters internally; no lookahead).
//     All events are class FAST. The breakout/continuation anchor rule
//     is used for every event: the cache carries no invalidation
//     timestamps, so the pullback/reaction-leg rule cannot be applied
//     historically (it is implemented in the module for live use).
//   - Structure arm: stop = first admissible candidate in
//     S_A -> S_B -> S_C order (the module's chosen stop); entry is a
//     market fill at E; TP1 is the module's structural TP1.
//   - Control arm: the event's legacy stop/tp1, market entry at E, all
//     events admitted. Events whose legacy geometry is already
//     degenerate at E (E beyond the legacy stop or TP1) are counted
//     separately.
//   - Walk: the event's cached 1m bars strictly after the anchor bar
//     (<= ~12h). Within one bar, the stop is checked before TP1
//     (conservative, matching the M1 harness's WalkLive ordering).
//     After a stop, the walk continues to detect a later TP1 touch.
//   - Gates (pre-registered): structure arm P(TP1 first) > 40% over
//     admitted trades AND stopped-then-touched-TP1 < 35% over stopped
//     trades. The historical reference numbers (32.9% / 55.1%) come
//     from a limit-fill walk and are NOT directly comparable.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
	local "github.com/Aixxww/AiT/provider/local"
)

const minuteMs = int64(60000)

type lbKey struct{ sym, tf string }

var lbCache = map[lbKey][]local.V7StructureBar{}

func loadLookback(dir, sym, tf string) ([]local.V7StructureBar, error) {
	k := lbKey{sym, tf}
	if bars, ok := lbCache[k]; ok {
		return bars, nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, sym+"_"+tf+".json"))
	if err != nil {
		return nil, err
	}
	var rows [][]float64
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parse lookback %s %s: %w", sym, tf, err)
	}
	bars := make([]local.V7StructureBar, 0, len(rows))
	for _, r := range rows {
		if len(r) < 5 {
			continue
		}
		bars = append(bars, local.V7StructureBar{
			OpenTimeMs: int64(math.Round(r[0])),
			Open:       r[1], High: r[2], Low: r[3], Close: r[4],
		})
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].OpenTimeMs < bars[j].OpenTimeMs })
	lbCache[k] = bars
	return bars, nil
}

func closedCount(bars []local.V7StructureBar, intervalMs, decisionMs int64) int {
	n := 0
	for _, b := range bars {
		if b.OpenTimeMs+intervalMs <= decisionMs {
			n++
		}
	}
	return n
}

// walkResult summarises one arm's walk for one event.
type walkResult struct {
	First    string // "tp1", "stop", "none"
	StopThen bool   // stopped first, TP1 touched later in the window
}

func walk(bars []replay.Bar, placeMin int64, long bool, stop, tp1 float64) walkResult {
	res := walkResult{First: "none"}
	stopped := false
	for _, b := range bars {
		if b.OpenTime <= placeMin {
			continue
		}
		if !stopped {
			hitStop := (long && b.Low <= stop) || (!long && b.High >= stop)
			hitTP := (long && b.High >= tp1) || (!long && b.Low <= tp1)
			if hitStop {
				res.First = "stop"
				stopped = true
				// Same bar may also touch TP1 after the stop; the
				// conservative convention counts that as a later
				// touch as well (intrabar order unknown).
				if hitTP {
					res.StopThen = true
				}
				continue
			}
			if hitTP {
				res.First = "tp1"
				return res
			}
		} else {
			hitTP := (long && b.High >= tp1) || (!long && b.Low <= tp1)
			if hitTP {
				res.StopThen = true
				return res
			}
		}
	}
	return res
}

type armAgg struct {
	N           int `json:"n"`
	Admitted    int `json:"admitted"`
	TP1First    int `json:"tp1_first"`
	Stopped     int `json:"stopped"`
	StopThenTP1 int `json:"stopped_then_tp1"`
	Unresolved  int `json:"unresolved"`
}

func (a *armAgg) add(r walkResult) {
	a.Admitted++
	switch r.First {
	case "tp1":
		a.TP1First++
	case "stop":
		a.Stopped++
		if r.StopThen {
			a.StopThenTP1++
		}
	default:
		a.Unresolved++
	}
}

func (a armAgg) pTP1First() float64 {
	if a.Admitted == 0 {
		return math.NaN()
	}
	return float64(a.TP1First) / float64(a.Admitted)
}

func (a armAgg) pTP1FirstResolved() float64 {
	d := a.TP1First + a.Stopped
	if d == 0 {
		return math.NaN()
	}
	return float64(a.TP1First) / float64(d)
}

func (a armAgg) stopThenRate() float64 {
	if a.Stopped == 0 {
		return math.NaN()
	}
	return float64(a.StopThenTP1) / float64(a.Stopped)
}

type group struct {
	Structure armAgg `json:"structure"`
	Control   armAgg `json:"control"`
}

type rejectCounts struct {
	RRInsufficient   int `json:"rr_insufficient"`
	BackendGuardRisk int `json:"backend_guard_risk"`
	Other            int `json:"other"`
}

type eventRecord struct {
	Index     int     `json:"index"`
	EventID   string  `json:"event_id"`
	Symbol    string  `json:"symbol"`
	Direction string  `json:"direction"`
	Setup     string  `json:"setup"`
	E         float64 `json:"e"`
	Feasible  bool    `json:"feasible"`
	Reason    string  `json:"reason,omitempty"`
	StopID    string  `json:"stop_id,omitempty"`
	Stop      float64 `json:"stop,omitempty"`
	TP1       float64 `json:"tp1,omitempty"`
	TP2       float64 `json:"tp2,omitempty"`
	Struct    string  `json:"struct_first,omitempty"`
	StructST  bool    `json:"struct_stop_then_tp1,omitempty"`
	Control   string  `json:"control_first,omitempty"`
	ControlST bool    `json:"control_stop_then_tp1,omitempty"`
}

type report struct {
	Events           int                    `json:"events"`
	AnchorMissing    int                    `json:"anchor_missing"`
	LookbackMissing  int                    `json:"lookback_missing"`
	MinClosed15m     int                    `json:"min_closed_15m_bars"`
	MinClosed1h      int                    `json:"min_closed_1h_bars"`
	Rejects          rejectCounts           `json:"structure_rejects"`
	ControlDegenStop int                    `json:"control_degenerate_stop_at_e"`
	ControlDegenTP1  int                    `json:"control_degenerate_tp1_at_e"`
	Overall          group                  `json:"overall"`
	ByDirection      map[string]*group      `json:"by_direction"`
	BySetup          map[string]*group      `json:"by_setup"`
	ByFamily         map[string]*group      `json:"by_family"`
	Records          []eventRecord          `json:"records"`
	Gates            map[string]interface{} `json:"gates"`
}

func familyOf(setup string) string {
	switch {
	case strings.HasPrefix(setup, "ignition"):
		return "ignition"
	case strings.HasPrefix(setup, "momentum_continuation"):
		return "momentum_continuation"
	case strings.HasPrefix(setup, "trend_pullback"):
		return "trend_pullback"
	case strings.HasPrefix(setup, "confirmed_breakout"):
		return "confirmed_breakout"
	case strings.HasPrefix(setup, "reclaim_breakout"):
		return "reclaim_breakout"
	case strings.HasPrefix(setup, "compression_oi_watch"):
		return "compression_oi_watch"
	default:
		return "other"
	}
}

func grp(m map[string]*group, key string) *group {
	g, ok := m[key]
	if !ok {
		g = &group{}
		m[key] = g
	}
	return g
}

func main() {
	dataDir := flag.String("data", "/home/hatch/workspace/ait-t2-data/entry_study", "entry-study directory")
	lbDir := flag.String("lookback", "/home/hatch/workspace/ait-t2-data/lookback", "lookback directory")
	outPath := flag.String("out", "/home/hatch/workspace/goals/goal-4/hidden_files/kiro-pkg-20261117/m2_eval_results.json", "JSON output path")
	flag.Parse()

	events, err := replay.LoadEvents(*dataDir)
	if err != nil {
		fatal("load events: %v", err)
	}
	cfg := local.V7FASTStructureConfig()
	rep := &report{
		Events:      len(events),
		MinClosed15m: 1 << 30,
		MinClosed1h:  1 << 30,
		ByDirection: map[string]*group{},
		BySetup:     map[string]*group{},
		ByFamily:    map[string]*group{},
	}

	for i, ev := range events {
		bars, err := replay.LoadBars(*dataDir, i)
		if err != nil {
			fatal("load bars %d: %v", i, err)
		}
		placeMin := (ev.TrigMs/minuteMs)*minuteMs + minuteMs
		var anchor *replay.Bar
		for j := range bars {
			if bars[j].OpenTime == placeMin {
				anchor = &bars[j]
				break
			}
		}
		groups := []*group{&rep.Overall, grp(rep.ByDirection, ev.Direction), grp(rep.BySetup, ev.Setup), grp(rep.ByFamily, familyOf(ev.Setup))}
		for _, g := range groups {
			g.Structure.N++
			g.Control.N++
		}
		if anchor == nil {
			rep.AnchorMissing++
			continue
		}
		E := anchor.Close
		decisionMs := placeMin + minuteMs
		long := ev.Direction == "LONG"

		sBars, errS := loadLookback(*lbDir, ev.Symbol, "15m")
		hBars, errH := loadLookback(*lbDir, ev.Symbol, "1h")
		if errS != nil || errH != nil {
			rep.LookbackMissing++
			continue
		}
		if n := closedCount(sBars, 15*minuteMs, decisionMs); n < rep.MinClosed15m {
			rep.MinClosed15m = n
		}
		if n := closedCount(hBars, 60*minuteMs, decisionMs); n < rep.MinClosed1h {
			rep.MinClosed1h = n
		}

		rec := eventRecord{Index: i, EventID: ev.EventID, Symbol: ev.Symbol, Direction: ev.Direction, Setup: ev.Setup, E: E}

		// Structure arm. Family is recorded faithfully, but with no
		// invalidation timestamp in the cache the builder applies
		// the breakout/continuation anchor rule to every event.
		fam := local.V7FamilyBreakoutContinuation
		if strings.Contains(ev.Setup, "pullback") {
			fam = local.V7FamilyPullbackReversal
		}
		dir := local.V7DirLong
		if !long {
			dir = local.V7DirShort
		}
		plan := local.BuildV7StructurePlan(local.V7StructureInput{
			Direction: dir, EntryPrice: E,
			DecisionTimeMs: decisionMs, Family: fam,
			BarsTFS: sBars, BarsTFH: hBars, Config: cfg,
		})
		if !plan.Feasible || plan.ChosenStop == nil {
			rec.Feasible = false
			rec.Reason = plan.Reason
			switch plan.Reason {
			case "rr_insufficient":
				rep.Rejects.RRInsufficient++
			case "backend_guard_risk":
				rep.Rejects.BackendGuardRisk++
			default:
				rep.Rejects.Other++
			}
		} else {
			rec.Feasible = true
			rec.StopID = plan.ChosenStop.ID
			rec.Stop = plan.ChosenStop.Price
			rec.TP1 = plan.TP1
			rec.TP2 = plan.TP2
			r := walk(bars, placeMin, long, plan.ChosenStop.Price, plan.TP1)
			rec.Struct = r.First
			rec.StructST = r.StopThen
			for _, g := range groups {
				g.Structure.add(r)
			}
		}

		// Control arm (legacy geometry, market at E, all admitted).
		degenStop := (long && E <= ev.Stop) || (!long && E >= ev.Stop)
		degenTP := (long && E >= ev.TP1) || (!long && E <= ev.TP1)
		var cr walkResult
		switch {
		case degenStop:
			rep.ControlDegenStop++
			// Stopped immediately at E; scan the window for a
			// later TP1 touch for the stop-then rate.
			cr = walkResult{First: "stop", StopThen: scanTouched(bars, placeMin, long, ev.TP1)}
		case degenTP:
			rep.ControlDegenTP1++
			cr = walkResult{First: "tp1"}
		default:
			cr = walk(bars, placeMin, long, ev.Stop, ev.TP1)
		}
		rec.Control = cr.First
		rec.ControlST = cr.StopThen
		for _, g := range groups {
			g.Control.add(cr)
		}
		rep.Records = append(rep.Records, rec)
	}

	s := rep.Overall.Structure
	rep.Gates = map[string]interface{}{
		"p_tp1_first_structure":      s.pTP1First(),
		"gate_p_tp1_first_gt_0.40":   s.pTP1First() > 0.40,
		"stop_then_tp1_structure":    s.stopThenRate(),
		"gate_stop_then_lt_0.35":     s.stopThenRate() < 0.35,
		"p_tp1_first_control":        rep.Overall.Control.pTP1First(),
		"stop_then_tp1_control":      rep.Overall.Control.stopThenRate(),
		"historical_limit_caliber":   map[string]float64{"p_tp1_first": 0.329, "stop_then_tp1": 0.551},
		"historical_note":            "limit-fill walk caliber; not directly comparable to either arm here",
	}

	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fatal("marshal: %v", err)
	}
	if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
		fatal("write out: %v", err)
	}
	printSummary(rep)
	fmt.Printf("\nwrote %s\n", *outPath)
}

// scanTouched reports whether TP1 is touched anywhere in the walk
// window (used for degenerate control events stopped at E).
func scanTouched(bars []replay.Bar, placeMin int64, long bool, tp1 float64) bool {
	for _, b := range bars {
		if b.OpenTime <= placeMin {
			continue
		}
		if (long && b.High >= tp1) || (!long && b.Low <= tp1) {
			return true
		}
	}
	return false
}

func pct(x float64) string {
	if math.IsNaN(x) {
		return "  n/a"
	}
	return fmt.Sprintf("%5.1f%%", x*100)
}

func printArm(name string, a armAgg) {
	adm := 0.0
	if a.N > 0 {
		adm = float64(a.Admitted) / float64(a.N) * 100
	}
	fmt.Printf("%-10s n=%3d admitted=%3d (%5.1f%%)  P(TP1 first)=%s  stopped=%3d  stop->TP1=%s  unresolved=%d\n",
		name, a.N, a.Admitted, adm, pct(a.pTP1First()), a.Stopped, pct(a.stopThenRate()), a.Unresolved)
}

func printSummary(rep *report) {
	fmt.Printf("events=%d anchor_missing=%d lookback_missing=%d min_closed_15m=%d min_closed_1h=%d\n",
		rep.Events, rep.AnchorMissing, rep.LookbackMissing, rep.MinClosed15m, rep.MinClosed1h)
	fmt.Printf("structure rejects: rr_insufficient=%d backend_guard_risk=%d other=%d\n",
		rep.Rejects.RRInsufficient, rep.Rejects.BackendGuardRisk, rep.Rejects.Other)
	fmt.Printf("control degenerate at E: stop=%d tp1=%d\n\n", rep.ControlDegenStop, rep.ControlDegenTP1)
	fmt.Println("== overall ==")
	printArm("structure", rep.Overall.Structure)
	printArm("control", rep.Overall.Control)
	fmt.Println("\n== by direction ==")
	for _, d := range []string{"LONG", "SHORT"} {
		if g, ok := rep.ByDirection[d]; ok {
			fmt.Printf("[%s]\n", d)
			printArm("structure", g.Structure)
			printArm("control", g.Control)
		}
	}
	fmt.Println("\n== by family ==")
	fams := make([]string, 0, len(rep.ByFamily))
	for k := range rep.ByFamily {
		fams = append(fams, k)
	}
	sort.Strings(fams)
	for _, f := range fams {
		g := rep.ByFamily[f]
		fmt.Printf("[%s]\n", f)
		printArm("structure", g.Structure)
		printArm("control", g.Control)
	}
	fmt.Println("\n== by setup ==")
	setups := make([]string, 0, len(rep.BySetup))
	for k := range rep.BySetup {
		setups = append(setups, k)
	}
	sort.Strings(setups)
	for _, sname := range setups {
		g := rep.BySetup[sname]
		fmt.Printf("[%s]\n", sname)
		printArm("structure", g.Structure)
		printArm("control", g.Control)
	}
	s := rep.Overall.Structure
	fmt.Printf("\nGATES: P(TP1 first)=%s (need >40%%) -> %v ; stop->TP1=%s (need <35%%) -> %v\n",
		pct(s.pTP1First()), s.pTP1First() > 0.40, pct(s.stopThenRate()), s.stopThenRate() < 0.35)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "t2_m2_eval: "+format+"\n", args...)
	os.Exit(1)
}
