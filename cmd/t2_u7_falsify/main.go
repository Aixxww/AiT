// Command t2_u7_falsify runs the pre-registered U7 falsification tests
// F-1..F-3 (docs/t2-u7-preregistration.md) for the shadow E[R] estimator
// (kernel/hunter_v7_expected_r.go) on the 694 events. Every parameter comes
// from store.EffectiveV7ExpectedR defaults; the protocol has no flags that
// change it.
//
// Usage:
//
//	go run ./cmd/t2_u7_falsify -u5 u5_48h.json -data entry_study -out u7.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

type u5Rec struct {
	SetupType   string `json:"setup_type"`
	Direction   string `json:"direction"`
	SignalClass string `json:"signal_class"`
	Family      string `json:"family"`
	RRBand      string `json:"rr_band"`
}

type u5Row struct {
	EventID       string   `json:"event_id"`
	TrackedSource string   `json:"tracked_source"`
	FeeNetOutcome string   `json:"fee_net_outcome"`
	FeeNetR       *float64 `json:"fee_net_r"`
	FeeNetEntry   float64  `json:"fee_net_entry"`
	Stop          float64  `json:"stop"`
	TP1           float64  `json:"tp1"`
	States        []struct {
		Record u5Rec `json:"record"`
	} `json:"states"`
}

type ev struct {
	ID       string  `json:"event_id"`
	Setup    string  `json:"setup"`
	Fold     int     `json:"fold"`
	Score    float64 `json:"score"`
	TrigMs   int64   `json:"trig_ms"`
	Admitted bool    `json:"in_universe"`
	R        float64 `json:"realized_r"`
	ER       float64 `json:"er"`
	LB       float64 `json:"er_lower"`
	row      store.V7CalibrationOutcome
}

type foldOut struct {
	Fold      int     `json:"fold"`
	K         float64 `json:"k"`
	Train     int     `json:"train_n"`
	Universe  int     `json:"universe_n"`
	M         int     `json:"m"`
	ERMean    float64 `json:"er_set_mean_r"`
	ScoreMean float64 `json:"score_set_mean_r"`
	ERWins    bool    `json:"er_beats_score"`
}

type quint struct {
	Q        int     `json:"quintile"`
	N        int     `json:"n"`
	PredMean float64 `json:"pred_mean"`
	RealMean float64 `json:"realized_mean"`
	Lo       float64 `json:"ci_lo"`
	Hi       float64 `json:"ci_hi"`
	Inside   bool    `json:"inside"`
}

type result struct {
	Config   store.V7ExpectedRConfig `json:"config"`
	Usable   int                     `json:"usable"`
	Folds    []foldOut               `json:"folds"`
	F1Wins   int                     `json:"f1_folds_won"`
	F1Pass   bool                    `json:"f1_pass"`
	Quint    []quint                 `json:"f2_quintiles"`
	F2Inside int                     `json:"f2_quintiles_inside"`
	F2Pass   bool                    `json:"f2_pass"`
	F3Obs    float64                 `json:"f3_observed_mean_r"`
	F3Null   float64                 `json:"f3_null_mean_r"`
	F3P      float64                 `json:"f3_p"`
	F3Pass   bool                    `json:"f3_pass"`
	Switch   bool                    `json:"ranking_switch_allowed"`
	Events   []ev                    `json:"events"`
}

func mean(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// topM returns the m events with the highest key; ties by earlier trigger.
func topM(es []*ev, m int, key func(*ev) float64) []*ev {
	s := append([]*ev(nil), es...)
	sort.SliceStable(s, func(i, j int) bool {
		if key(s[i]) != key(s[j]) {
			return key(s[i]) > key(s[j])
		}
		return s[i].TrigMs < s[j].TrigMs
	})
	return s[:m]
}

func meanR(es []*ev) float64 {
	xs := make([]float64, len(es))
	for i, e := range es {
		xs[i] = e.R
	}
	return mean(xs)
}

func main() {
	u5Path := flag.String("u5", "", "cmd/t2_u5_tags output at -horizon-hours 48")
	dataDir := flag.String("data", "", "entry_study directory (events.json)")
	outPath := flag.String("out", "", "result JSON")
	flag.Parse()
	cfg := store.EffectiveV7ExpectedR(nil)
	cost := store.DefaultV7FeeNetCostPct

	events, err := replay.LoadEvents(*dataDir)
	if err != nil {
		fatal("events: %v", err)
	}
	order := make([]int, len(events))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return events[order[a]].TrigMs < events[order[b]].TrigMs })
	fold := map[string]int{}
	for k, i := range order {
		fold[events[i].EventID] = 1 + k*3/len(events) // OD-2 trigger tertiles over all events
	}
	byID := map[string]int{}
	for i, e := range events {
		byID[e.EventID] = i
	}

	var rows []u5Row
	b, err := os.ReadFile(*u5Path)
	if err != nil {
		fatal("u5: %v", err)
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		fatal("u5: %v", err)
	}
	var all []*ev
	mismatch := 0
	for _, r := range rows {
		if r.FeeNetOutcome != store.V7FeeNetTP1 && r.FeeNetOutcome != store.V7FeeNetStop && r.FeeNetOutcome != store.V7FeeNetNone {
			continue
		}
		i, ok := byID[r.EventID]
		if !ok || r.FeeNetR == nil || len(r.States) == 0 {
			fatal("event %s: missing input", r.EventID)
		}
		rec := r.States[len(r.States)-1].Record
		rwin, ok := store.V7FeeNetRRAtTP1(r.FeeNetEntry, r.Stop, r.TP1, cost)
		if !ok {
			fatal("event %s: no R_win", r.EventID)
		}
		want := map[string]float64{store.V7FeeNetTP1: rwin, store.V7FeeNetStop: -1, store.V7FeeNetNone: 0}[r.FeeNetOutcome]
		if want != *r.FeeNetR {
			mismatch++
		}
		e := &ev{ID: r.EventID, Setup: store.V7CanonicalSetupType(rec.SetupType), // U11: upstream name
			Fold: fold[r.EventID], Score: events[i].Score, TrigMs: events[i].TrigMs,
			Admitted: r.TrackedSource == store.V7OutcomeSourcePromptOpenReview, R: *r.FeeNetR,
			row: store.V7CalibrationOutcome{Time: time.UnixMilli(events[i].TrigMs), SetupType: rec.SetupType, Direction: rec.Direction,
				Class: rec.SignalClass, Family: rec.Family, RRBand: rec.RRBand, Outcome: r.FeeNetOutcome, RWin: rwin}}
		all = append(all, e)
	}
	if mismatch > 0 {
		fatal("%d events: realized R differs from the OD-2 R of their outcome", mismatch)
	}

	res := result{Config: cfg, Usable: len(all)}
	var pooledUniverse [][]*ev
	var erSets []*ev
	for f := 1; f <= 3; f++ {
		var trainFolds [][]store.V7CalibrationOutcome
		var train []store.V7CalibrationOutcome
		for o := 1; o <= 3; o++ {
			if o == f {
				continue
			}
			var fr []store.V7CalibrationOutcome
			for _, e := range all {
				if e.Fold == o {
					fr = append(fr, e.row)
				}
			}
			trainFolds = append(trainFolds, fr)
			train = append(train, fr...)
		}
		k := kernel.HunterV7ERChooseK(trainFolds, cfg)
		cal := kernel.FitHunterV7ExpectedR(train, k, cfg, time.Time{})
		var uni []*ev
		for _, e := range all {
			if e.Fold != f {
				continue
			}
			in := kernel.HunterV7ERInput{Class: e.row.Class, Family: e.row.Family, Direction: e.row.Direction, RRBand: e.row.RRBand, RWin: e.row.RWin}
			e.ER, e.LB, _ = cal.Estimate(in)
			if e.Admitted {
				uni = append(uni, e)
			}
		}
		m := int(math.Ceil(cfg.AdmitFraction * float64(len(uni))))
		erSet := topM(uni, m, func(e *ev) float64 { return e.LB })
		scSet := topM(uni, m, func(e *ev) float64 { return e.Score })
		fo := foldOut{Fold: f, K: k, Train: len(train), Universe: len(uni), M: m, ERMean: meanR(erSet), ScoreMean: meanR(scSet)}
		fo.ERWins = fo.ERMean > fo.ScoreMean
		if fo.ERWins {
			res.F1Wins++
		}
		res.Folds = append(res.Folds, fo)
		pooledUniverse = append(pooledUniverse, uni)
		erSets = append(erSets, erSet...)
	}
	res.F1Pass = res.F1Wins >= cfg.F1MinFolds

	// F-2: calibration by quintile over the pooled universe.
	var pool []*ev
	for _, u := range pooledUniverse {
		pool = append(pool, u...)
	}
	sorted := append([]*ev(nil), pool...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].ER != sorted[j].ER {
			return sorted[i].ER < sorted[j].ER
		}
		return sorted[i].TrigMs < sorted[j].TrigMs
	})
	groups := make([][]*ev, cfg.Quantiles)
	for i, e := range sorted {
		q := i * cfg.Quantiles / len(sorted)
		groups[q] = append(groups[q], e)
	}
	boot := rand.New(rand.NewSource(cfg.Seed))
	tail := (1 - cfg.BootstrapCI) / 2
	for q, g := range groups {
		pred, real := make([]float64, len(g)), make([]float64, len(g))
		for i, e := range g {
			pred[i], real[i] = e.ER, e.R
		}
		means := make([]float64, cfg.BootstrapResamples)
		for s := range means {
			sum := 0.0
			for range real {
				sum += real[boot.Intn(len(real))]
			}
			means[s] = sum / float64(len(real))
		}
		sort.Float64s(means)
		n := cfg.BootstrapResamples
		qo := quint{Q: q + 1, N: len(g), PredMean: mean(pred), RealMean: mean(real),
			Lo: means[int(math.Round(float64(n)*tail))], Hi: means[int(math.Round(float64(n)*(1-tail)))-1]}
		qo.Inside = qo.PredMean >= qo.Lo && qo.PredMean <= qo.Hi
		if qo.Inside {
			res.F2Inside++
		}
		res.Quint = append(res.Quint, qo)
	}
	res.F2Pass = res.F2Inside >= cfg.F2MinQuantiles

	// F-3: E[R] sets against random sets of the same size.
	res.F3Obs = meanR(erSets)
	draws := rand.New(rand.NewSource(cfg.Seed + 1))
	ge, nullSum := 0, 0.0
	for d := 0; d < cfg.RandomDraws; d++ {
		sum, n := 0.0, 0
		for f, uni := range pooledUniverse {
			m := res.Folds[f].M
			for _, j := range draws.Perm(len(uni))[:m] {
				sum += uni[j].R
				n++
			}
		}
		dm := sum / float64(n)
		nullSum += dm
		if dm >= res.F3Obs {
			ge++
		}
	}
	res.F3Null = nullSum / float64(cfg.RandomDraws)
	res.F3P = float64(1+ge) / float64(1+cfg.RandomDraws)
	res.F3Pass = res.F3P < cfg.F3MaxP
	res.Switch = res.F1Pass && res.F3Pass
	for _, e := range all {
		res.Events = append(res.Events, *e)
	}

	out, _ := json.MarshalIndent(res, "", " ")
	if err := os.WriteFile(*outPath, out, 0o644); err != nil {
		fatal("write: %v", err)
	}
	fmt.Printf("usable %d, universe %d\n", res.Usable, len(pool))
	for _, f := range res.Folds {
		fmt.Printf("F-1 fold %d: k=%g train=%d |U|=%d m=%d  E[R] set %+.4f  score set %+.4f  %v\n", f.Fold, f.K, f.Train, f.Universe, f.M, f.ERMean, f.ScoreMean, f.ERWins)
	}
	fmt.Printf("F-1: %d of 3 folds -> pass=%v\n", res.F1Wins, res.F1Pass)
	for _, q := range res.Quint {
		fmt.Printf("F-2 Q%d n=%d pred %+.4f realized %+.4f [%+.4f, %+.4f] inside=%v\n", q.Q, q.N, q.PredMean, q.RealMean, q.Lo, q.Hi, q.Inside)
	}
	fmt.Printf("F-2: %d of %d inside -> pass=%v\n", res.F2Inside, cfg.Quantiles, res.F2Pass)
	fmt.Printf("F-3: observed %+.4f, null mean %+.4f, p=%.4f -> pass=%v\n", res.F3Obs, res.F3Null, res.F3P, res.F3Pass)
	fmt.Printf("ranking switch allowed (F-1 and F-3): %v (not switched in U7)\n", res.Switch)
}

func fatal(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
