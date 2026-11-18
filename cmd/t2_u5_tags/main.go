// Command t2_u5_tags is the U5 acceptance replay. For each stored Task-2
// snapshot it runs detection, the U1 lifecycle walk (as cmd/t2_u1_lifecycle)
// and the production candidate assembly, then builds the persisted signal
// records with the plan §5.4 tags, once at the created state and once at
// TRIGGERED when the walk reaches it. It checks that every tag column is
// set, and registers the record with the production outcome tracker at the
// upstream trigger time, feeding the event's cached 1m bars, to get the
// tracker's OD-2 fee-net R for comparison with the U0 walk.
//
// Usage:
//
//	go run ./cmd/t2_u5_tags -snapshots snapshots.jsonl -data entry_study -out u5.json [-horizon-hours 8]
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/Aixxww/AiT/cmd/internal/t2payload"
	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
	"github.com/Aixxww/AiT/trader"
)

type stateRow struct {
	State   string                     `json:"state"`
	Tier    string                     `json:"tier"`
	Record  store.HunterV7SignalRecord `json:"record"`
	Missing []string                   `json:"missing_tags,omitempty"`
}

type row struct {
	EventID       string     `json:"event_id"`
	Setup         string     `json:"setup"`
	Recalled      bool       `json:"recalled"`
	States        []stateRow `json:"states"`
	TrackedSource string     `json:"tracked_source"`
	FeeNetOutcome string     `json:"fee_net_outcome"`
	FeeNetR       *float64   `json:"fee_net_r"`
	FeeNetEntry   float64    `json:"fee_net_entry"`
	DriftPct      *float64   `json:"drift_at_entry_pct"`
	Stop          float64    `json:"stop"`
	TP1           float64    `json:"tp1"`
}

func main() {
	snapPath := flag.String("snapshots", "", "snapshots.jsonl")
	dataDir := flag.String("data", "", "entry_study directory (events.json + klines/)")
	outPath := flag.String("out", "", "per-event JSON output")
	cadence := flag.Int("cadence", 3, "lifecycle scan cadence in minutes")
	horizonH := flag.Float64("horizon-hours", 0, "tracker horizon in hours (0 = tracker default)")
	flag.Parse()

	events, err := replay.LoadEvents(*dataDir)
	if err != nil {
		fatal("events: %v", err)
	}
	idx := map[string]int{}
	for i, e := range events {
		idx[e.EventID] = i
	}
	geom := kernel.HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true)
	geom.T2MinRR = store.DefaultT2MinRiskRewardRatio
	cfg := &local.V7T2Config{Enabled: true}
	opts := kernel.DefaultHunterV7RecordTagOptions()

	f, err := os.Open(*snapPath)
	if err != nil {
		fatal("snapshots: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var rows []row
	for sc.Scan() {
		var ex t2payload.ExtractRow
		if err := json.Unmarshal(sc.Bytes(), &ex); err != nil {
			fatal("parse: %v", err)
		}
		r := row{EventID: ex.EventID, Setup: ex.SetupType}
		snap := ex.Payload.ToSnapshot()
		var regime *local.V7T2RegimeInput
		if ex.Payload.RegimeRV24 != nil {
			regime = &local.V7T2RegimeInput{RV24: *ex.Payload.RegimeRV24, Valid: true}
		}
		sig := local.EvaluateV7T2Snapshot(snap, cfg, regime)
		if sig == nil || sig.SetupType != ex.SetupType || sig.Direction != ex.Direction {
			rows = append(rows, r)
			continue
		}
		r.Recalled = true
		snapT, err := time.Parse(time.RFC3339Nano, ex.Payload.Ts)
		if err != nil {
			fatal("payload ts %q: %v", ex.Payload.Ts, err)
		}
		i, ok := idx[ex.EventID]
		if !ok {
			fatal("event %s not in events.json", ex.EventID)
		}
		bars, err := replay.LoadBars(*dataDir, i)
		if err != nil {
			fatal("bars %d: %v", i, err)
		}

		// U1 lifecycle walk to find whether the event reaches TRIGGERED.
		st := local.NewV7T2EventStore()
		ev := st.Upsert(sig, snap.Price(), snapT).Event
		states := []string{ev.Status}
		k := sort.Search(len(bars), func(j int) bool { return bars[j].OpenTime+60000 > snapT.UnixMilli() })
		for j := k; j < len(bars); j += *cadence {
			st.UpdateOutcomes(map[string]float64{ex.Symbol: bars[j].Close}, time.UnixMilli(bars[j].OpenTime+60000))
			if ev.Status != "WATCH" && ev.Status != "ARMED" && ev.Status != "TRIGGERED" {
				break
			}
		}
		if ev.TriggeredAt != nil {
			states = append(states, "TRIGGERED")
		}

		var last stateRow
		var lastRec local.V7SignalRecord
		for _, state := range states {
			out := sig.ToV7SignalOutputWithLifecycle("", state)
			cands := kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{out}, "BOTH", geom)
			recs := kernel.BuildHunterV7SignalRecordsTagged([]local.V7SignalOutput{out}, cands, opts)
			db := kernel.BuildHunterV7SignalDBRecordsWithAudit(1, recs, snapT, "ACTIVE", true)
			sr := stateRow{State: state, Tier: recs[0].Tier, Record: db[0], Missing: missingTags(db[0])}
			sr.Record.RawJSON = ""
			r.States = append(r.States, sr)
			last, lastRec = sr, recs[0]
		}

		// Production tracker at the upstream trigger time.
		tcfg := trader.DefaultTrackerConfig()
		if *horizonH > 0 {
			tcfg.TimeoutDuration = time.Duration(*horizonH * float64(time.Hour))
		}
		tr := trader.NewSignalOutcomeTracker(tcfg, nil)
		tr.SetCandleHistorySource(func(_ string, since time.Time) []trader.TrackedCandle {
			var out []trader.TrackedCandle
			for _, b := range bars {
				ct := time.UnixMilli(b.OpenTime + 60000 - 1) // live candles carry the close time
				if ct.After(since) {
					out = append(out, trader.TrackedCandle{T: ct, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close})
				}
			}
			return out
		})
		var lastOut trader.TrackedOutcome
		tr.SetOutcomeCallback(func(o trader.TrackedOutcome) { lastOut = o })
		rec := lastRec.Signal
		r.TrackedSource = lastRec.Tags.OutcomeSource
		r.Stop, r.TP1 = rec.Invalidation.Price, rec.TP1Price
		tr.RegisterWithMeta(trader.TrackedSignalMeta{OutcomeSource: lastRec.Tags.OutcomeSource, TriggerPrice: lastRec.Tags.TriggerPrice},
			1, rec.Symbol, string(rec.Direction), string(rec.SetupType), last.Tier,
			kernel.HunterV7SignalEntryPrice(rec), rec.Invalidation.Price, rec.TP0Price, rec.TP1Price, rec.TP2Price,
			time.UnixMilli(events[i].TrigMs))
		tr.TickNow()
		r.FeeNetOutcome, r.FeeNetR, r.FeeNetEntry, r.DriftPct = lastOut.FeeNetOutcome, lastOut.FeeNetR, lastOut.FeeNetEntry, lastOut.DriftAtEntryPct
		rows = append(rows, r)
	}
	if err := sc.Err(); err != nil {
		fatal("scan: %v", err)
	}
	b, _ := json.Marshal(rows)
	if err := os.WriteFile(*outPath, b, 0o644); err != nil {
		fatal("write: %v", err)
	}
	fmt.Printf("events %d\n", len(rows))
}

// missingTags lists the §5.4 tag columns that are empty on a T2 record.
// fee_net_r, fee_net_outcome and drift_at_entry_pct are written later by the
// tracker and are checked from the tracker output instead.
func missingTags(r store.HunterV7SignalRecord) []string {
	var miss []string
	for name, v := range map[string]string{
		"stack": r.Stack, "signal_class": r.SignalClass, "family": r.Family, "rr_band": r.RRBand,
		"stop_atr_band": r.StopATRBand, "cell_status": r.CellStatus, "profile_version": r.ProfileVersion,
		"calibration_version": r.CalibrationVersion, "doctrine_version": r.DoctrineVersion,
		"lifecycle_state_at_decision": r.LifecycleStateAtDecision, "decision_path": r.DecisionPath,
		"outcome_source": r.OutcomeSource, "data_gaps": r.DataGaps,
	} {
		if v == "" || v == store.V7BandUnknown || v == store.V7ClassUnassigned || v == "[]" {
			miss = append(miss, name)
		}
	}
	if r.TriggerPrice == nil {
		miss = append(miss, "trigger_price")
	}
	sort.Strings(miss)
	return miss
}

func fatal(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
