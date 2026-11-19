// Command t2_u13_yield is the U13 measurement: per-regime fire rates of the
// 694-event study and the regime-conditional yield targets derived from
// them (redesign plan §6.2, OD-20; docs/t2-u13-yield-target.md).
//
// Fixed method (owner decisions U13, set before measuring):
//   - Regime: the scanner's BTC RV24 tier (spec §6.1), recomputed from
//     BTCUSDT futures klines of the data.binance.vision archive: the closed
//     1h bars plus the forming 1h bar priced at the latest closed bar of
//     -fine (5m), RV24Bars bars, population std of the last RV24Returns log
//     returns x100; LOW < 0.25, MID < 0.35, HIGH >= 0.35.
//   - Check: the recomputed RV24 against the 284 snapshots that carry the
//     scanner's own regime_rv24, at the snapshot time payload.ts.
//   - Rounds: 5-minute rounds of the 48-day window 2026-09-28 to
//     2026-11-15 UTC (13,824), each labelled by its tier at the round start.
//   - Events: each event's tier at its trigger time. Stages: all T2
//     triggered events, and the structure-admitted events of t2_m2_eval at
//     each given RR floor.
//   - Rate per tier = events / rounds, with the exact two-sided 90% Poisson
//     interval; target = the interval's lower bound.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Aixxww/AiT/cmd/internal/btcregime"
	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
	"github.com/Aixxww/AiT/store"
)

type m2Record struct {
	Index    int  `json:"index"`
	Feasible bool `json:"feasible"`
}

type m2Output struct {
	RRMin   float64    `json:"rr_min"`
	Records []m2Record `json:"records"`
}

type checkStats struct {
	Fine        string  `json:"fine"`
	N           int     `json:"n"`
	Missing     int     `json:"missing"`
	TierAgree   int     `json:"tier_agree"`
	AbsDiffP50  float64 `json:"abs_diff_p50"`
	AbsDiffP90  float64 `json:"abs_diff_p90"`
	AbsDiffMax  float64 `json:"abs_diff_max"`
	Within0005  int     `json:"within_0_005"`
	Disagreeing []struct {
		Ts       string  `json:"ts"`
		Recorded float64 `json:"recorded"`
		Mine     float64 `json:"recomputed"`
	} `json:"disagreeing"`
}

type result struct {
	Window     [2]time.Time                  `json:"window"`
	Method     store.V7YieldTargetConfig     `json:"method"`
	Check      []checkStats                  `json:"rv24_check"`
	Rounds     map[string]int                `json:"rounds_per_tier"`
	RoundsAlt  map[string]int                `json:"rounds_per_tier_alt_fine"`
	EventTiers map[string]int                `json:"triggered_tiers"`
	Rows       []store.V7YieldRow            `json:"rows"`
	Targets    map[string]map[string]float64 `json:"targets"`
	Thresholds map[string]float64            `json:"g3_thresholds"`
	Observed   map[string]int                `json:"stage_events"`
	OutsideWin int                           `json:"events_outside_window"`
	// AltTierDiffers counts rounds and events whose tier differs under
	// the -fine-alt forming-bar price (sensitivity, report only).
	AltRoundTierDiffers int `json:"alt_round_tier_differs"`
	AltEventTierDiffers int `json:"alt_event_tier_differs"`
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "t2_u13_yield: "+format+"\n", a...)
	os.Exit(1)
}

func quantile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[int(math.Round(p*float64(len(s)-1)))]
}

// recordedRV reads the scanner's regime fields from the snapshots.
func recordedRV(path string) ([]struct {
	Ms   int64
	Ts   string
	RV   float64
	Tier string
}, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []struct {
		Ms   int64
		Ts   string
		RV   float64
		Tier string
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var row struct {
			Payload struct {
				Ts   string   `json:"ts"`
				RV   *float64 `json:"regime_rv24"`
				Tier string   `json:"regime_tier"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		if row.Payload.RV == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, row.Payload.Ts)
		if err != nil {
			return nil, fmt.Errorf("ts %q: %w", row.Payload.Ts, err)
		}
		out = append(out, struct {
			Ms   int64
			Ts   string
			RV   float64
			Tier string
		}{t.UnixMilli(), row.Payload.Ts, *row.Payload.RV, row.Payload.Tier})
	}
	return out, nil
}

func check(series *store.V7RV24Series, cfg store.V7YieldTargetConfig, fine string, rec []struct {
	Ms   int64
	Ts   string
	RV   float64
	Tier string
}) checkStats {
	cs := checkStats{Fine: fine}
	var diffs []float64
	for _, r := range rec {
		rv, ok := series.At(r.Ms)
		if !ok {
			cs.Missing++
			continue
		}
		cs.N++
		mine := math.Round(rv*1000) / 1000 // the scanner stores 3 decimals
		d := math.Abs(mine - r.RV)
		diffs = append(diffs, d)
		if d <= 0.005 {
			cs.Within0005++
		}
		if cfg.RV24Tier(rv) == r.Tier {
			cs.TierAgree++
		} else {
			cs.Disagreeing = append(cs.Disagreeing, struct {
				Ts       string  `json:"ts"`
				Recorded float64 `json:"recorded"`
				Mine     float64 `json:"recomputed"`
			}{r.Ts, r.RV, mine})
		}
	}
	cs.AbsDiffP50, cs.AbsDiffP90 = quantile(diffs, 0.5), quantile(diffs, 0.9)
	for _, d := range diffs {
		cs.AbsDiffMax = math.Max(cs.AbsDiffMax, d)
	}
	return cs
}

func main() {
	btcDir := flag.String("btc-dir", "", "directory of BTCUSDT data.binance.vision kline zips (1h + fine)")
	fine := flag.String("fine", "5m", "forming-bar price timeframe (method)")
	fineAlt := flag.String("fine-alt", "1m", "sensitivity forming-bar timeframe (report only; empty = skip)")
	dataDir := flag.String("data", "", "entry_study directory (events.json)")
	snapPath := flag.String("snapshots", "", "m3_snapshots/snapshots.jsonl (for the RV24 check)")
	m2Paths := flag.String("m2", "", "comma-separated t2_m2_eval outputs (one per RR floor)")
	outPath := flag.String("out", "", "result JSON")
	flag.Parse()
	if *btcDir == "" || *dataDir == "" || *snapPath == "" || *outPath == "" {
		fatal("usage: -btc-dir DIR -data entry_study -snapshots snapshots.jsonl -m2 a.json,b.json -out out.json")
	}
	cfg := store.EffectiveV7YieldTarget(nil)
	cfg.Targets = nil // measured here, not read
	series, err := btcregime.Series(*btcDir, *fine, cfg)
	if err != nil {
		fatal("btc series: %v", err)
	}
	res := result{Window: store.V7YieldWindowUTC, Method: cfg, Targets: map[string]map[string]float64{},
		Thresholds: map[string]float64{}, Observed: map[string]int{}}
	res.Method.Targets = nil

	rec, err := recordedRV(*snapPath)
	if err != nil {
		fatal("snapshots: %v", err)
	}
	res.Check = append(res.Check, check(series, cfg, *fine, rec))
	from, to := store.V7YieldWindowUTC[0].UnixMilli(), store.V7YieldWindowUTC[1].UnixMilli()
	res.Rounds = series.RoundsPerTier(from, to)
	var alt *store.V7RV24Series
	if *fineAlt != "" {
		alt, err = btcregime.Series(*btcDir, *fineAlt, cfg)
		if err != nil {
			fatal("alt series: %v", err)
		}
		res.Check = append(res.Check, check(alt, cfg, *fineAlt, rec))
		res.RoundsAlt = alt.RoundsPerTier(from, to)
		step := cfg.RoundMs()
		for t := (from + step - 1) / step * step; t < to; t += step {
			if series.Tier(t) != alt.Tier(t) {
				res.AltRoundTierDiffers++
			}
		}
	}

	events, err := replay.LoadEvents(*dataDir)
	if err != nil {
		fatal("events: %v", err)
	}
	tierOf := make([]string, len(events))
	res.EventTiers = map[string]int{}
	for i, ev := range events {
		tierOf[i] = series.Tier(ev.TrigMs)
		if ev.TrigMs < from || ev.TrigMs >= to {
			res.OutsideWin++
		}
		res.EventTiers[tierOf[i]]++
		if alt != nil && alt.Tier(ev.TrigMs) != tierOf[i] {
			res.AltEventTierDiffers++
		}
	}

	stages := []struct {
		name string
		in   func(i int) bool
	}{{store.V7YieldStageT2Triggered, func(int) bool { return true }}}
	if *m2Paths != "" {
		for _, p := range strings.Split(*m2Paths, ",") {
			raw, err := os.ReadFile(p)
			if err != nil {
				fatal("m2 %s: %v", p, err)
			}
			var m m2Output
			if err := json.Unmarshal(raw, &m); err != nil {
				fatal("m2 %s: %v", p, err)
			}
			admitted := map[int]bool{}
			for _, r := range m.Records {
				if r.Feasible {
					admitted[r.Index] = true
				}
			}
			stages = append(stages, struct {
				name string
				in   func(i int) bool
			}{store.V7YieldStageM2Admitted(m.RRMin), func(i int) bool { return admitted[i] }})
		}
	}
	for _, st := range stages {
		counts := map[string]int{}
		total := 0
		for i, ev := range events {
			if ev.TrigMs < from || ev.TrigMs >= to || !st.in(i) {
				continue
			}
			counts[tierOf[i]]++
			total++
		}
		res.Observed[st.name] = total
		res.Targets[st.name] = map[string]float64{}
		for _, tier := range store.V7RV24Tiers {
			row := store.BuildV7YieldRow(st.name, tier, counts[tier], res.Rounds[tier], cfg)
			res.Rows = append(res.Rows, row)
			res.Targets[st.name][tier] = row.CILo
		}
		if counts[store.V7RV24TierNone] > 0 {
			fatal("%s: %d events without a tier", st.name, counts[store.V7RV24TierNone])
		}
	}
	tcfg := cfg
	tcfg.Targets = res.Targets
	for stage := range res.Targets {
		th, ok := store.V7YieldThreshold(stage, res.Rounds, tcfg)
		if !ok {
			fatal("threshold %s", stage)
		}
		res.Thresholds[stage] = th
	}

	raw, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
		fatal("write: %v", err)
	}
	for _, c := range res.Check {
		fmt.Printf("RV24 check (fine %s): n=%d missing=%d tier agree %d/%d, |diff| p50 %.4f p90 %.4f max %.4f, within 0.005: %d\n",
			c.Fine, c.N, c.Missing, c.TierAgree, c.N, c.AbsDiffP50, c.AbsDiffP90, c.AbsDiffMax, c.Within0005)
	}
	fmt.Printf("rounds %v (alt fine: %v), events outside window %d, alt tier differs: rounds %d events %d\n",
		res.Rounds, res.RoundsAlt, res.OutsideWin, res.AltRoundTierDiffers, res.AltEventTierDiffers)
	for _, r := range res.Rows {
		fmt.Printf("%-22s %-4s rounds=%5d events=%3d rate=%.6f CI90=[%.6f, %.6f] -> target %.6f\n",
			r.Stage, r.Tier, r.Rounds, r.Events, r.Rate, r.CILo, r.CIHi, r.CILo)
	}
	names := make([]string, 0, len(res.Thresholds))
	for k := range res.Thresholds {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Printf("G3 threshold %-22s %.3f (observed %d)\n", k, res.Thresholds[k], res.Observed[k])
	}
	for stage, tiers := range res.Targets {
		for _, tier := range store.V7RV24Tiers {
			fmt.Printf("default %s %s %s\n", stage, tier, strconv.FormatFloat(tiers[tier], 'g', 17, 64))
		}
	}
}
