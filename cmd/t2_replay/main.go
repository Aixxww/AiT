// Command t2_replay is the M1 replay harness for the task-2 new
// architecture line: it replays the 694-event entry-study cache
// (events.json + 1-minute klines) through the legacy Arm A entry/exit
// semantics — plan-price limit intake with the resting guard and the
// live exit model — ported faithfully from the Python reference
// (shadow_exec.py + sim_live_replay.py).
//
// Its acceptance job is to reproduce the reference aggregates: the
// all-events Arm A totals and the consumed-band table under the
// current-config caliber (observe-only setups excluded).
//
// Usage:
//
//	go run ./cmd/t2_replay [-data DIR] [-out report.json] [-bands-only]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
)

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "ait-t2-data/entry_study"
	}
	return filepath.Join(home, "workspace", "ait-t2-data", "entry_study")
}

func main() {
	dataDir := flag.String("data", defaultDataDir(), "entry-study directory (events.json + klines/)")
	outPath := flag.String("out", "", "write the full JSON report to this path")
	bandsOnly := flag.Bool("bands-only", false, "print only the consumed-band table to stdout")
	flag.Parse()

	events, err := replay.LoadEvents(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load events: %v\n", err)
		os.Exit(1)
	}
	rep, err := replay.RunAll(*dataDir, events)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}

	if !*bandsOnly {
		fmt.Printf("events: %d  ok: %d  nodata: %d\n", rep.NEvents, rep.NOk, rep.NNoData)
		a := rep.All
		fmt.Printf("Arm A (all events): fills=%d wins=%d winrate=%.1f%% net_r=%+.2f avg_r=%+.3f\n",
			a.Fills, a.Wins, a.WinRate, a.NetR, a.AvgR)
		fmt.Printf("nofill: %v\n", rep.NoFill)
		fmt.Printf("current-config caliber: %d events (observe-only setups excluded)\n", rep.CurrentCfgN)
	}
	fmt.Println("consumed bands (current-config caliber):")
	fmt.Printf("  %-8s %6s %6s %6s %8s %8s\n", "band", "n", "fills", "wins", "winrate", "net_r")
	for _, b := range rep.Bands {
		fmt.Printf("  %-8s %6d %6d %6d %7.1f%% %+8.2f\n",
			b.Name, b.N, b.Fills, b.Wins, b.WinRate, b.NetR)
	}

	if *outPath != "" {
		raw, err := json.MarshalIndent(rep, "", " ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal report: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write report: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("report written to %s\n", *outPath)
	}
}
