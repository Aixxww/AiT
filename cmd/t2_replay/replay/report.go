package replay

// ObserveSetups are the five setups forced to observation under the
// current (v0.7.4-observe-chronic) configuration. The CURRENT-CONFIG
// caliber excludes their events; the list is ported from
// sim_live_replay.py:29-30 (OBSERVE_SETUPS).
var ObserveSetups = map[string]bool{
	"trend_pullback_long":       true,
	"reclaim_breakout_long":     true,
	"confirmed_breakout_long":   true,
	"confirmed_breakout_short":  true,
	"compression_oi_watch_long": true,
}

// Record is the per-event replay output row.
type Record struct {
	Index       int      `json:"index"`
	EventID     string   `json:"event_id"`
	Symbol      string   `json:"symbol"`
	Setup       string   `json:"setup"`
	Direction   string   `json:"direction"`
	TriggeredAt string   `json:"triggered_at"`
	Status      string   `json:"status"` // OK / NO_DATA
	Consumed    *float64 `json:"consumed"`
	Intake      string   `json:"intake,omitempty"`
	Outcome     string   `json:"outcome,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	GrossR      *float64 `json:"gross_r,omitempty"`
	NetR        *float64 `json:"net_r,omitempty"`
	FillIdx     *int     `json:"fill_idx,omitempty"`
}

// Agg aggregates filled records the way sim_live_replay.agg does:
// wins are fills with net_r > 0; net_r sums the per-event rounded
// net_r of fills only.
type Agg struct {
	N       int     `json:"n"` // records in the cohort
	Fills   int     `json:"fills"`
	Wins    int     `json:"wins"`
	WinRate float64 `json:"winrate"` // percent, 1dp
	NetR    float64 `json:"net_r"`   // 2dp
	AvgR    float64 `json:"avg_r"`   // 3dp
}

// AggregateRecords computes Agg over a cohort of records.
func AggregateRecords(records []Record) Agg {
	a := Agg{N: len(records)}
	var sum float64
	for _, r := range records {
		if r.Intake != "FILLED" || r.NetR == nil {
			continue
		}
		a.Fills++
		if *r.NetR > 0 {
			a.Wins++
		}
		sum += *r.NetR
	}
	if a.Fills > 0 {
		a.WinRate = round1(100 * float64(a.Wins) / float64(a.Fills))
		a.NetR = round2(sum)
		a.AvgR = math3(sum / float64(a.Fills))
	}
	return a
}

// Band is one consumed band of the current-config caliber.
type Band struct {
	Name string `json:"name"`
	Lo   string `json:"lo"`
	Hi   string `json:"hi"`
	Agg
}

// bandBounds defines the consumed bands: [<lo>, <hi>) with nil for an
// open end. Mirrors the banding used in the reference analysis:
// <0, [0,0.5), [0.5,1.0), >=1.0.
var bandBounds = []struct {
	name string
	lo   *float64
	hi   *float64
}{
	{"neg", nil, f64p(0)},
	{"b0_05", f64p(0), f64p(0.5)},
	{"b05_10", f64p(0.5), f64p(1.0)},
	{"ge1", f64p(1.0), nil},
}

func f64p(v float64) *float64 { return &v }

func inBand(consumed float64, lo, hi *float64) bool {
	if lo != nil && consumed < *lo {
		return false
	}
	if hi != nil && consumed >= *hi {
		return false
	}
	return true
}

// Report is the full replay output document.
type Report struct {
	NEvents     int            `json:"n_events"`
	NOk         int            `json:"n_ok"`
	NNoData     int            `json:"n_nodata"`
	All         Agg            `json:"all_arm_a"`
	NoFill      map[string]int `json:"nofill"`
	CurrentCfgN int            `json:"current_config_n"`
	Bands       []Band         `json:"current_config_bands"`
	Records     []Record       `json:"records"`
}

// RunAll replays every event (loading its bars from dir) and builds
// the report.
func RunAll(dir string, events []Event) (*Report, error) {
	rep := &Report{NEvents: len(events), NoFill: map[string]int{}}
	rep.Records = make([]Record, 0, len(events))
	for i, ev := range events {
		bars, err := LoadBars(dir, i)
		if err != nil {
			return nil, err
		}
		res := Simulate(ev, bars)
		rec := Record{
			Index: i, EventID: ev.EventID, Symbol: ev.Symbol, Setup: ev.Setup,
			Direction: ev.Direction, TriggeredAt: ev.TriggeredAt,
			Status: res.Status, Consumed: res.Consumed, Intake: res.Intake,
			Outcome: res.Outcome, Mode: res.Mode,
		}
		if res.Intake == "FILLED" {
			g, n, fi := res.GrossR, res.NetR, res.FillIdx
			rec.GrossR, rec.NetR, rec.FillIdx = &g, &n, &fi
		}
		rep.Records = append(rep.Records, rec)
		if res.Status == "OK" {
			rep.NOk++
			if res.Intake != "" && res.Intake != "FILLED" {
				rep.NoFill[res.Intake]++
			}
		} else {
			rep.NNoData++
		}
	}
	ok := make([]Record, 0, rep.NOk)
	current := make([]Record, 0, rep.NOk)
	for _, r := range rep.Records {
		if r.Status != "OK" {
			continue
		}
		ok = append(ok, r)
		if !ObserveSetups[r.Setup] {
			current = append(current, r)
		}
	}
	rep.All = AggregateRecords(ok)
	rep.CurrentCfgN = len(current)
	for _, b := range bandBounds {
		var rows []Record
		for _, r := range current {
			if r.Consumed != nil && inBand(*r.Consumed, b.lo, b.hi) {
				rows = append(rows, r)
			}
		}
		lo, hi := "-inf", "+inf"
		if b.lo != nil {
			lo = ftoa(*b.lo)
		}
		if b.hi != nil {
			hi = ftoa(*b.hi)
		}
		rep.Bands = append(rep.Bands, Band{Name: b.name, Lo: lo, Hi: hi, Agg: AggregateRecords(rows)})
	}
	return rep, nil
}
