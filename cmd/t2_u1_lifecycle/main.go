// Command t2_u1_lifecycle is the U1/P1 acceptance replay. For each of the
// stored Task-2 snapshots it runs detection (EvaluateV7T2Snapshot with the
// payload's stored regime), creates the event in a fresh lifecycle store at
// the snapshot time, then walks the event's cached 1m closes at the AiT scan
// cadence through UpdateOutcomes (spec section 5.3) until the event ends.
// Each event is assembled with the production candidate assembly twice:
// before U1 (the detector status, as emitted today) and after U1 (the state
// the trigger stage reaches, emitted as a candidate when TRIGGERED).
//
// Scope and limits: one event per snapshot, a fresh store per event (no
// cross-event cooldown), prices are 1m closes sampled every -cadence
// minutes from the first cached bar at or after the snapshot time.
//
// Usage:
//
//	go run ./cmd/t2_u1_lifecycle -snapshots snapshots.jsonl -data entry_study -out u1.json
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
)

type row struct {
	EventID      string   `json:"event_id"`
	Setup        string   `json:"setup"`
	Recalled     bool     `json:"recalled"`
	Created      string   `json:"created_status"`
	BeforeStatus string   `json:"before_status"`
	BeforeTier   string   `json:"before_tier"`
	BeforeReason string   `json:"before_reason"`
	FinalState   string   `json:"final_state"`
	Outcome      string   `json:"outcome"`
	RealizedR    *float64 `json:"realized_r"`
	Triggered    bool     `json:"triggered"`
	TrigMinutes  float64  `json:"trigger_minutes_after_snapshot"`
	UpstreamLag  float64  `json:"trigger_minus_upstream_minutes"`
	AfterStatus  string   `json:"after_status"`
	AfterTier    string   `json:"after_tier"`
	AfterReason  string   `json:"after_reason"`
	CoverageGap  float64  `json:"minutes_snapshot_to_first_bar"`
	// Geometry and timing against the upstream record (events.json).
	Entry           float64 `json:"entry"`
	Stop            float64 `json:"stop"`
	UpstreamEntry   float64 `json:"upstream_entry"`
	UpstreamStop    float64 `json:"upstream_stop"`
	SnapMinusUpTrig float64 `json:"snapshot_minus_upstream_trigger_minutes"`
}

func main() {
	snapPath := flag.String("snapshots", "", "snapshots.jsonl (reassembled from the p00..p07 parts)")
	dataDir := flag.String("data", "", "entry_study directory (events.json + klines/)")
	outPath := flag.String("out", "", "per-event JSON output")
	cadence := flag.Int("cadence", 3, "scan cadence in minutes (AiT scan_interval_minutes default)")
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
		st := local.NewV7T2EventStore()
		res := st.Upsert(sig, snap.Price(), snapT)
		ev := res.Event
		r.Created = ev.Status
		before := sig.ToV7SignalOutputWithLifecycle("", ev.Status)
		r.BeforeStatus = string(before.Status)
		r.BeforeTier, r.BeforeReason = assemble(before, geom)

		i, ok := idx[ex.EventID]
		if !ok {
			fatal("event %s not in events.json", ex.EventID)
		}
		r.Entry, r.Stop = sig.Entry, sig.Stop
		r.UpstreamEntry, r.UpstreamStop = events[i].Entry, events[i].Stop
		r.SnapMinusUpTrig = float64(snapT.UnixMilli()-events[i].TrigMs) / 60000
		bars, err := replay.LoadBars(*dataDir, i)
		if err != nil {
			fatal("bars %d: %v", i, err)
		}
		k := sort.Search(len(bars), func(j int) bool { return bars[j].OpenTime+60000 > snapT.UnixMilli() })
		if k < len(bars) {
			r.CoverageGap = float64(bars[k].OpenTime+60000-snapT.UnixMilli()) / 60000
		}
		var trigAt time.Time
		for j := k; j < len(bars); j += *cadence {
			now := time.UnixMilli(bars[j].OpenTime + 60000)
			for _, tr := range st.UpdateOutcomes(map[string]float64{ex.Symbol: bars[j].Close}, now) {
				if tr.To == "TRIGGERED" {
					trigAt = now
				}
			}
			if ev.Status != "WATCH" && ev.Status != "ARMED" && ev.Status != "TRIGGERED" {
				break
			}
		}
		r.FinalState, r.Outcome, r.RealizedR = ev.Status, ev.Outcome, ev.RealizedR
		after := before
		if ev.TriggeredAt != nil {
			r.Triggered = true
			r.TrigMinutes = trigAt.Sub(snapT).Minutes()
			r.UpstreamLag = float64(trigAt.UnixMilli()-events[i].TrigMs) / 60000
			after = sig.ToV7SignalOutputWithLifecycle("", "TRIGGERED")
		}
		r.AfterStatus = string(after.Status)
		r.AfterTier, r.AfterReason = assemble(after, geom)
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

func assemble(o local.V7SignalOutput, g kernel.HunterV7ExecutionGeometry) (string, string) {
	c := kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{o}, "BOTH", g)
	if len(c) != 1 {
		return "", ""
	}
	return c[0].V7ExecutionTier, c[0].V7TierReason
}

func fatal(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
