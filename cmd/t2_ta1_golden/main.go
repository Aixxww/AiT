// Command t2_ta1_golden is the TA-1 acceptance replay (Tier A plan,
// docs/tier-a-improvement-plan-20261120.md). For each stored Task-2
// snapshot it runs the U1/U4 replay exactly as cmd/t2_u1_lifecycle does
// (detection, a fresh lifecycle store, the 1m walk at the scan cadence,
// candidate assembly with the T2 minimum stop and class tier rule), and
// assembles every before/after output twice:
//
//   - base: as the Task-2 converter emits it (no TA-1 fields);
//   - ta1:  after local.ApplyV7T2ContractFields with a datafetch snapshot
//     rebuilt from the stored payload (the U0-B reconstruction).
//
// Both arms use the kernel the binary is built with. Pass rule (TA-1 full,
// on top of the TA-1 kernel unit): per signal, status, lifecycle state,
// entry mode, tier and tier_reason are all identical. Any difference is a
// violation and the command exits 2.
//
// The payload has no per-bar taker volume, so the faithful reconstruction
// gives taker_buy_ratio_15m = 0. -taker-share additionally runs the ta1
// arm with each 15m bar's taker volume set from the payload buy_share, to
// exercise the kernel's taker-reading tier rules with a realistic value.
//
// Usage:
//
//	go run ./cmd/t2_ta1_golden -snapshots snapshots.jsonl -data entry_study -out ta1_golden.json
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/Aixxww/AiT/cmd/internal/t2payload"
	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

type assembled struct {
	Status    string `json:"status"`
	Lifecycle string `json:"lifecycle_state"`
	EntryMode string `json:"entry_mode"`
	Tier      string `json:"tier"`
	Reason    string `json:"tier_reason"`
}

type compared struct {
	EventID string     `json:"event_id"`
	Setup   string     `json:"setup"`
	Phase   string     `json:"phase"` // before (detector status) | after (lifecycle state)
	Arm     string     `json:"arm"`   // ta1 | ta1_taker
	Base    assembled  `json:"base"`
	TA1     assembled  `json:"ta1"`
	Kind    string     `json:"kind"` // identical | violation
	Detail  string     `json:"detail,omitempty"`
	Ctx     *ctxValues `json:"ta1_ctx,omitempty"`
}

type ctxValues struct {
	Last       float64 `json:"last"`
	Funding    float64 `json:"funding_rate"`
	OIValue    float64 `json:"oi_value"`
	OIChange1h float64 `json:"oi_change_1h"`
	OIChange4h float64 `json:"oi_change_4h"`
	TakerBuy   float64 `json:"taker_buy_ratio_15m"`
}

type summary struct {
	Snapshots        int            `json:"snapshots"`
	Recalled         int            `json:"recalled"`
	Comparisons      int            `json:"comparisons"`
	Identical        int            `json:"identical"`
	Violations       int            `json:"violations"`
	ByArm            map[string]int `json:"comparisons_by_arm"`
	BaseTierCounts   map[string]int `json:"base_tier_counts"`
	TA1TierCounts    map[string]int `json:"ta1_tier_counts"`
	BaseMatchesU1    *bool          `json:"base_matches_u1,omitempty"`
	U1Mismatches     int            `json:"u1_mismatches,omitempty"`
	Pass             bool           `json:"pass"`
	ViolationDetails []compared     `json:"violation_details,omitempty"`
}

type u1Row struct {
	EventID      string `json:"event_id"`
	Recalled     bool   `json:"recalled"`
	BeforeTier   string `json:"before_tier"`
	BeforeReason string `json:"before_reason"`
	AfterTier    string `json:"after_tier"`
	AfterReason  string `json:"after_reason"`
}

func main() {
	snapPath := flag.String("snapshots", "", "snapshots.jsonl (reassembled from the p00..p07 parts)")
	dataDir := flag.String("data", "", "entry_study directory (events.json + klines/)")
	outPath := flag.String("out", "", "per-comparison JSON output")
	cadence := flag.Int("cadence", 3, "scan cadence in minutes (as cmd/t2_u1_lifecycle)")
	t2MinStop := flag.Float64("t2-min-stop", 0, "T2 minimum stop distance in percent (0 = store default, U4)")
	takerShare := flag.Bool("taker-share", true, "also run the ta1 arm with 15m taker volume from the payload buy_share")
	u1Path := flag.String("u1", "", "optional cmd/t2_u1_lifecycle output to cross-check the base arm against")
	fields := flag.String("fields", "all", "TA-1 fields applied in the ta1 arms (attribution): all | price | deriv | confirm")
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
	geom.T2MinSLMovePct = *t2MinStop
	cfg := &local.V7T2Config{Enabled: true}

	f, err := os.Open(*snapPath)
	if err != nil {
		fatal("snapshots: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)

	sum := summary{ByArm: map[string]int{}, BaseTierCounts: map[string]int{}, TA1TierCounts: map[string]int{}}
	var rows []compared
	baseByEvent := map[string][2]assembled{} // before, after
	arms := []string{"ta1"}
	if *takerShare {
		arms = append(arms, "ta1_taker")
	}
	for sc.Scan() {
		var ex t2payload.ExtractRow
		if err := json.Unmarshal(sc.Bytes(), &ex); err != nil {
			fatal("parse: %v", err)
		}
		sum.Snapshots++
		snap := ex.Payload.ToSnapshot()
		var regime *local.V7T2RegimeInput
		if ex.Payload.RegimeRV24 != nil {
			regime = &local.V7T2RegimeInput{RV24: *ex.Payload.RegimeRV24, Valid: true}
		}
		sig := local.EvaluateV7T2Snapshot(snap, cfg, regime)
		if sig == nil || sig.SetupType != ex.SetupType || sig.Direction != ex.Direction {
			continue
		}
		sum.Recalled++
		snapT, err := time.Parse(time.RFC3339Nano, ex.Payload.Ts)
		if err != nil {
			fatal("payload ts %q: %v", ex.Payload.Ts, err)
		}
		// U1 walk, identical to cmd/t2_u1_lifecycle.
		st := local.NewV7T2EventStore()
		ev := st.Upsert(sig, snap.Price(), snapT).Event
		before := sig.ToV7SignalOutputWithLifecycle("", ev.Status)
		i, ok := idx[ex.EventID]
		if !ok {
			fatal("event %s not in events.json", ex.EventID)
		}
		bars, err := replay.LoadBars(*dataDir, i)
		if err != nil {
			fatal("bars %d: %v", i, err)
		}
		k := sort.Search(len(bars), func(j int) bool { return bars[j].OpenTime+60000 > snapT.UnixMilli() })
		for j := k; j < len(bars); j += *cadence {
			now := time.UnixMilli(bars[j].OpenTime + 60000)
			st.UpdateOutcomes(map[string]float64{ex.Symbol: bars[j].Close}, now)
			if ev.Status != "WATCH" && ev.Status != "ARMED" && ev.Status != "TRIGGERED" {
				break
			}
		}
		after := before
		if ev.TriggeredAt != nil {
			after = sig.ToV7SignalOutputWithLifecycle("", "TRIGGERED")
		}

		baseB, baseA := assemble(before, geom), assemble(after, geom)
		baseByEvent[ex.EventID] = [2]assembled{baseB, baseA}
		sum.BaseTierCounts[baseB.Tier]++
		sum.BaseTierCounts[baseA.Tier]++
		for _, arm := range arms {
			nsnap := rebuildSnapshot(&ex.Payload, snapT, arm == "ta1_taker")
			for _, ph := range []struct {
				name string
				out  local.V7SignalOutput
				base assembled
			}{{"before", before, baseB}, {"after", after, baseA}} {
				o := ph.out
				o.RequiredConfirms = append([]string(nil), o.RequiredConfirms...)
				o.DataGaps = append([]string(nil), o.DataGaps...)
				local.ApplyV7T2ContractFields(&o, nsnap)
				switch *fields {
				case "price":
					o.DerivativesCtx, o.RequiredConfirms = ph.out.DerivativesCtx, ph.out.RequiredConfirms
				case "deriv":
					o.PriceCtx, o.RequiredConfirms = ph.out.PriceCtx, ph.out.RequiredConfirms
				case "confirm":
					o.PriceCtx, o.DerivativesCtx = ph.out.PriceCtx, ph.out.DerivativesCtx
				}
				ta1 := assemble(o, geom)
				c := compared{EventID: ex.EventID, Setup: ex.SetupType, Phase: ph.name, Arm: arm, Base: ph.base, TA1: ta1}
				if o.DerivativesCtx != nil && o.PriceCtx != nil {
					c.Ctx = &ctxValues{Last: o.PriceCtx.Last, Funding: o.DerivativesCtx.FundingRate, OIValue: o.DerivativesCtx.OIValue,
						OIChange1h: o.DerivativesCtx.OIChange1h, OIChange4h: o.DerivativesCtx.OIChange4h, TakerBuy: o.DerivativesCtx.TakerBuy15m}
				}
				c.Kind, c.Detail = classify(ph.base, ta1)
				sum.Comparisons++
				sum.ByArm[arm]++
				if arm == "ta1" {
					sum.TA1TierCounts[ta1.Tier]++
				}
				switch c.Kind {
				case "identical":
					sum.Identical++
				default:
					sum.Violations++
					sum.ViolationDetails = append(sum.ViolationDetails, c)
				}
				rows = append(rows, c)
			}
		}
	}
	if err := sc.Err(); err != nil {
		fatal("scan: %v", err)
	}
	if *u1Path != "" {
		b, err := os.ReadFile(*u1Path)
		if err != nil {
			fatal("u1: %v", err)
		}
		var u1 []u1Row
		if err := json.Unmarshal(b, &u1); err != nil {
			fatal("u1 parse: %v", err)
		}
		match := true
		for _, r := range u1 {
			got, ok := baseByEvent[r.EventID]
			if !r.Recalled {
				if ok {
					match = false
					sum.U1Mismatches++
				}
				continue
			}
			if !ok || got[0].Tier != r.BeforeTier || got[0].Reason != r.BeforeReason || got[1].Tier != r.AfterTier || got[1].Reason != r.AfterReason {
				match = false
				sum.U1Mismatches++
			}
		}
		sum.BaseMatchesU1 = &match
	}
	sum.Pass = sum.Violations == 0 && (sum.BaseMatchesU1 == nil || *sum.BaseMatchesU1)
	out := struct {
		Summary summary    `json:"summary"`
		Rows    []compared `json:"rows"`
	}{sum, rows}
	b, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile(*outPath, b, 0o644); err != nil {
		fatal("write: %v", err)
	}
	s, _ := json.MarshalIndent(struct {
		summary
		ViolationDetails int `json:"violation_details,omitempty"`
	}{summary: sum}, "", " ")
	fmt.Println(string(s))
	if !sum.Pass {
		os.Exit(2)
	}
}

// classify applies the TA-1 pass rule to one base/ta1 pair.
func classify(base, ta1 assembled) (string, string) {
	if base.Status != ta1.Status || base.Lifecycle != ta1.Lifecycle || base.EntryMode != ta1.EntryMode {
		return "violation", fmt.Sprintf("signal fields changed: %+v -> %+v", base, ta1)
	}
	if base.Tier != ta1.Tier {
		return "violation", fmt.Sprintf("tier %s -> %s (%s -> %s)", base.Tier, ta1.Tier, base.Reason, ta1.Reason)
	}
	if base.Reason == ta1.Reason {
		return "identical", ""
	}
	return "violation", fmt.Sprintf("tier_reason %s -> %s on tier %s", base.Reason, ta1.Reason, base.Tier)
}

func assemble(o local.V7SignalOutput, g kernel.HunterV7ExecutionGeometry) assembled {
	a := assembled{Status: string(o.Status), Lifecycle: o.LifecycleState, EntryMode: string(o.EntryMode)}
	c := kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{o}, "BOTH", g)
	if len(c) == 1 {
		a.Tier, a.Reason = c[0].V7ExecutionTier, c[0].V7TierReason
	}
	return a
}

// rebuildSnapshot is the U0-B single-symbol reconstruction of
// cmd/t2_u0_native buildNativeSnapshot with its default mappings
// (funding pct/100, global LSR, OI value series, 24 1h bars for the 24h
// range), duplicated here so that tool's outputs are untouched. taker
// sets each 15m bar's taker volume from the payload buy_share.
func rebuildSnapshot(p *t2payload.Payload, now time.Time, taker bool) *datafetch.Snapshot {
	if len(p.Rows5) == 0 {
		return nil
	}
	last := int64(p.Rows5[len(p.Rows5)-1].CloseTime)
	shift := now.UnixMilli() - last
	ss := &datafetch.SymbolSnapshot{
		Symbol: p.Symbol, Price: p.LastPrice, Timestamp: now,
		PriceChange24h: p.Change24h, QuoteVolume24h: p.QuoteVolume,
		MarkPrice: p.MarkPrice, IndexPrice: p.MarkPrice, FundingRate: p.FundingPct / 100,
		LongShortRatio: p.GlobalLSR, TakerBuyRatio: p.TakerSignal.BuyShare,
		Klines: map[string][]datafetch.Kline{},
	}
	if p.LastPrice > 0 {
		ss.Volume24h = p.QuoteVolume / p.LastPrice
	}
	k5 := toKlines(p.Rows5, 5*60000, shift)
	ss.Klines["5m"] = k5
	k15 := agg15(k5, func(i int) int64 { return int64(p.Rows5[i].CloseTime) + 1 - 5*60000 })
	if taker {
		for i := range k15 {
			k15[i].TakerBuy = k15[i].Volume * p.TakerSignal.BuyShare
		}
	}
	ss.Klines["15m"] = k15
	ss.Klines["1h"] = toKlines(p.Rows1h, 3600000, shift)
	ss.Klines["4h"] = toKlines(p.Rows4h, 4*3600000, shift)
	h := ss.Klines["1h"]
	n := 24
	if len(h) < n {
		n = len(h)
	}
	for _, b := range h[len(h)-n:] {
		if ss.HighPrice24h == 0 || b.High > ss.HighPrice24h {
			ss.HighPrice24h = b.High
		}
		if ss.LowPrice24h == 0 || b.Low < ss.LowPrice24h {
			ss.LowPrice24h = b.Low
		}
	}
	if len(p.OIRows) > 0 {
		ss.OI = pf(p.OIRows[len(p.OIRows)-1].SumOI)
	}
	var spikes []float64
	for i := 1; i < len(p.OI4hRows); i++ {
		a, b := pf(p.OI4hRows[i-1].SumOIValue), pf(p.OI4hRows[i].SumOIValue)
		if a > 0 {
			spikes = append(spikes, (b-a)/a*100)
		}
	}
	ss.OISpikeData = spikes
	if len(spikes) >= 1 {
		ss.OIDelta1h = spikes[len(spikes)-1]
	}
	for i := len(spikes) - 4; i < len(spikes); i++ {
		if i >= 0 {
			ss.OIDelta4h += spikes[i]
		}
	}
	return &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{p.Symbol: ss}, CreatedAt: now}
}

func toKlines(rows []t2payload.RawBar, intervalMs, shift int64) []datafetch.Kline {
	out := make([]datafetch.Kline, 0, len(rows))
	for _, b := range rows {
		ct := int64(b.CloseTime) + shift
		out = append(out, datafetch.Kline{OpenTime: ct + 1 - intervalMs, CloseTime: ct, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume})
	}
	return out
}

func agg15(k5 []datafetch.Kline, origOpen func(i int) int64) []datafetch.Kline {
	var out []datafetch.Kline
	i := 0
	for i < len(k5) && origOpen(i)%(15*60000) != 0 {
		i++
	}
	for ; i+2 < len(k5); i += 3 {
		a, b, c := k5[i], k5[i+1], k5[i+2]
		out = append(out, datafetch.Kline{OpenTime: a.OpenTime, CloseTime: c.CloseTime, Open: a.Open,
			High: math.Max(a.High, math.Max(b.High, c.High)), Low: math.Min(a.Low, math.Min(b.Low, c.Low)),
			Close: c.Close, Volume: a.Volume + b.Volume + c.Volume})
	}
	return out
}

func pf(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }

func fatal(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
