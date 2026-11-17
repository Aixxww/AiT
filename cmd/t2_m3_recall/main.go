// Command t2_m3_recall runs the pre-registered M3 recall evaluation of
// the transplanted Task-2 setup module (provider/local
// hunter_v7_t2_setups.go) over the scanner's own stored snapshots for
// the 694 distilled entry-study events.
//
// Protocol (fixed before any results were seen; see M3_REPORT.md):
//
//   - Inputs: ~/workspace/ait-t2-data/m3_snapshots/snapshots.jsonl — one
//     whale_events row per event; the payload is the scanner's stored
//     snapshot+signal dict from the scan that created or last upgraded
//     the event. ONLY the snapshot input fields (section 9.1 of the
//     spec) are fed to the detector; the payload's own signal fields
//     are used solely as the expected output.
//   - Matching rule: an event is RECALLED when the detector, run on the
//     stored snapshot, emits a signal with setup_type == the recorded
//     setup_type AND direction == the recorded direction. Score and
//     status are diagnostics, not part of the gate.
//   - Recall config: module enabled, ALL families enabled, symbol
//     validity gate OFF, no regime input (section 6.1 modulation never
//     changes setup_type/direction), no lifecycle store (each event is
//     evaluated independently on its own stored snapshot; cooldown and
//     upsert semantics are unit-tested separately).
//   - Gate (pre-registered): recall >= 90% of reconstructable events.
//     All 694 events were reconstructable (payload present for every
//     event_id), so the denominator is 694.
//   - Parity mode (-parity): rebuilds features from the stored raw
//     bars/OI rows with the Go builders and compares them against the
//     Python-computed feature dicts stored in the same payloads.
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
	"strings"

	local "github.com/Aixxww/AiT/provider/local"
)

// ---------------------------------------------------------------------------
// Stored payload shapes (spec section 9.1)
// ---------------------------------------------------------------------------

type kFeat struct {
	Price         float64  `json:"price"`
	ATR           float64  `json:"atr"`
	ATRPct        float64  `json:"atr_pct"`
	ATR14         float64  `json:"atr14"`
	RSI           float64  `json:"rsi"`
	BBWidth       float64  `json:"bb_width"`
	BBWidthMin50  float64  `json:"bb_width_min_50"`
	BBWidthRatio  float64  `json:"bb_width_ratio"`
	BBPercentile  float64  `json:"bb_percentile"`
	Range6Pct     float64  `json:"range_6_pct"`
	VolumeRatio   float64  `json:"volume_ratio"`
	Resistance    float64  `json:"resistance"`
	Support       float64  `json:"support"`
	PriorHigh     float64  `json:"prior_high"`
	PriorLow      float64  `json:"prior_low"`
	EMA20         *float64 `json:"ema20"`
	EMA50         *float64 `json:"ema50"`
	EMATrendPct   float64  `json:"ema_trend_pct"`
	Change3Pct    float64  `json:"change_3_pct"`
	Change12Pct   float64  `json:"change_12_pct"`
	Change24Pct   float64  `json:"change_24_pct"`
	LastCloseTime float64  `json:"last_close_time"`
}

func (k kFeat) toFeatures() local.V7T2KlineFeatures {
	f := local.V7T2KlineFeatures{
		Price: k.Price, ATR: k.ATR, ATRPct: k.ATRPct, ATR14: k.ATR14, RSI: k.RSI,
		BBWidth: k.BBWidth, BBWidthMin50: k.BBWidthMin50, BBWidthRatio: k.BBWidthRatio,
		BBPercentile: k.BBPercentile, Range6Pct: k.Range6Pct, VolumeRatio: k.VolumeRatio,
		Resistance: k.Resistance, Support: k.Support, PriorHigh: k.PriorHigh, PriorLow: k.PriorLow,
		EMATrendPct: k.EMATrendPct, Change3Pct: k.Change3Pct, Change12Pct: k.Change12Pct,
		Change24Pct: k.Change24Pct, LastCloseTime: int64(k.LastCloseTime),
	}
	if k.EMA20 != nil && k.EMA50 != nil {
		f.EMA20 = *k.EMA20
		f.EMA50 = *k.EMA50
		f.HasEMA = true
	}
	return f
}

type rawBar struct {
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	CloseTime float64 `json:"close_time"`
}

func (b rawBar) toBar() local.V7T2Bar {
	return local.V7T2Bar{Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume, CloseTime: int64(b.CloseTime)}
}

type oiRow struct {
	SumOIValue string `json:"sumOpenInterestValue"`
	SumOI      string `json:"sumOpenInterest"`
}

func (r oiRow) value() float64 {
	v, _ := strconv.ParseFloat(r.SumOIValue, 64)
	return v
}

func (r oiRow) contracts() float64 {
	v, _ := strconv.ParseFloat(r.SumOI, 64)
	return v
}

// oiSeries applies the scanner's series selection (filter the value
// series to positive entries; contract counts only if it is all-empty).
func oiSeries(rows []oiRow) []float64 {
	vals := make([]float64, len(rows))
	cts := make([]float64, len(rows))
	for i, r := range rows {
		vals[i] = r.value()
		cts[i] = r.contracts()
	}
	return local.V7T2OIValueSeries(vals, cts)
}

type payload struct {
	Symbol      string   `json:"symbol"`
	Base        string   `json:"base"`
	MarketCap   float64  `json:"market_cap"`
	QuoteVolume float64  `json:"quote_volume"`
	Change24h   float64  `json:"change_24h"`
	LastPrice   float64  `json:"last_price"`
	MarkPrice   float64  `json:"mark_price"`
	FundingPct  float64  `json:"funding_pct"`
	GlobalLSR   float64  `json:"global_lsr"`
	TopLSR      float64  `json:"top_lsr"`
	K5          kFeat    `json:"k5"`
	K15         kFeat    `json:"k15"`
	K1h         kFeat    `json:"k1h"`
	K4h         kFeat    `json:"k4h"`
	Rows5       []rawBar `json:"rows5"`
	Rows1h      []rawBar `json:"rows1h"`
	Rows4h      []rawBar `json:"rows4h"`
	OIRows      []oiRow  `json:"oi_rows"`
	OI4hRows    []oiRow  `json:"oi_4h_rows"`
	OI          struct {
		Pct5m    float64 `json:"oi_5m_pct"`
		Pct15m   float64 `json:"oi_15m_pct"`
		Pct1h    float64 `json:"oi_1h_pct"`
		Z        float64 `json:"oi_z"`
		Notional float64 `json:"oi_notional_usd"`
	} `json:"oi"`
	OIAcc struct {
		Pct4h    float64 `json:"oi_4h_pct"`
		PriceChg float64 `json:"price_change_4h_pct"`
		Ampl     float64 `json:"price_amplitude_4h_pct"`
	} `json:"oi_acc"`
	TakerSignal struct {
		BuyShare       float64   `json:"buy_share"`
		BuyShare3      []float64 `json:"buy_share_3"`
		Buy3Confirmed  bool      `json:"buy_3_confirmed"`
		Sell3Confirmed bool      `json:"sell_3_confirmed"`
		RawRatio       float64   `json:"raw_ratio"`
	} `json:"taker_signal"`
	TakerRatio    float64 `json:"taker_ratio"`
	TakerTrendPct float64 `json:"taker_trend_pct"`
	Depth         struct {
		BidTotal  float64 `json:"bid_total"`
		AskTotal  float64 `json:"ask_total"`
		Ratio     float64 `json:"ratio"`
		SpreadBps float64 `json:"spread_bps"`
	} `json:"depth"`
	// Recorded outputs (expected values; never fed to the detector).
	SetupType   string   `json:"setup_type"`
	Direction   string   `json:"direction"`
	Score       float64  `json:"score"`
	Status      string   `json:"status"`
	RegimeRV24  *float64 `json:"regime_rv24"`
	ObserveOnly *bool    `json:"observe_only"`
	Version     string   `json:"strategy_version"`
}

type extractRow struct {
	EventID     string  `json:"event_id"`
	Symbol      string  `json:"symbol"`
	Direction   string  `json:"direction"`
	SetupType   string  `json:"setup_type"`
	Score       float64 `json:"score"`
	CreatedAt   string  `json:"created_at"`
	TriggeredAt string  `json:"triggered_at"`
	Payload     payload `json:"payload"`
}

func (p *payload) toSnapshot() *local.V7T2Snapshot {
	s := &local.V7T2Snapshot{
		Symbol:              p.Symbol,
		Base:                p.Base,
		MarketCap:           p.MarketCap,
		QuoteVolume:         p.QuoteVolume,
		Change24h:           p.Change24h,
		LastPrice:           p.LastPrice,
		MarkPrice:           p.MarkPrice,
		FundingPct:          p.FundingPct,
		GlobalLSR:           p.GlobalLSR,
		TopLSR:              p.TopLSR,
		K5:                  p.K5.toFeatures(),
		K15:                 p.K15.toFeatures(),
		K1h:                 p.K1h.toFeatures(),
		K4h:                 p.K4h.toFeatures(),
		TakerRatio:          p.TakerRatio,
		TakerTrendPct:       p.TakerTrendPct,
		OI5mAvailable:       true,
		OIPct1hFrom5mSeries: true,
	}
	for _, b := range p.Rows1h {
		s.Rows1h = append(s.Rows1h, b.toBar())
	}
	s.OI = local.V7T2OIFeatures{
		Pct5m: p.OI.Pct5m, Pct15m: p.OI.Pct15m, Pct1h: p.OI.Pct1h, Z: p.OI.Z, NotionalUSD: p.OI.Notional,
	}
	s.OIAcc = local.V7T2OIAccFeatures{
		Pct4h: p.OIAcc.Pct4h, PriceChange4hPct: p.OIAcc.PriceChg, PriceAmplitude4hPct: p.OIAcc.Ampl,
	}
	s.Taker = local.V7T2TakerFeatures{
		BuyShare: p.TakerSignal.BuyShare, BuyShare3: p.TakerSignal.BuyShare3,
		Buy3Confirmed: p.TakerSignal.Buy3Confirmed, Sell3Confirmed: p.TakerSignal.Sell3Confirmed,
		RawRatio: p.TakerSignal.RawRatio,
	}
	s.Depth = local.V7T2DepthFeatures{
		BidTotal: p.Depth.BidTotal, AskTotal: p.Depth.AskTotal,
		Ratio: p.Depth.Ratio, SpreadBps: p.Depth.SpreadBps, Available: true,
	}
	return s
}

// ---------------------------------------------------------------------------
// Evaluation
// ---------------------------------------------------------------------------

type eventResult struct {
	EventID        string            `json:"event_id"`
	Symbol         string            `json:"symbol"`
	ExpectedSetup  string            `json:"expected_setup"`
	ExpectedDir    string            `json:"expected_direction"`
	EmittedSetup   string            `json:"emitted_setup,omitempty"`
	EmittedDir     string            `json:"emitted_direction,omitempty"`
	Recalled       bool              `json:"recalled"`
	ExpectedScore  float64           `json:"expected_score"`
	EmittedScore   float64           `json:"emitted_score,omitempty"`
	ExpectedStatus string            `json:"expected_status"`
	EmittedStatus  string            `json:"emitted_status,omitempty"`
	ActivityFlags  int               `json:"activity_flags"`
	Layer1Pass     bool              `json:"layer1_pass"`
	Families       map[string]string `json:"families,omitempty"`
}

func layer1Pass(s *local.V7T2Snapshot) bool {
	price := s.Price()
	return price > 0 && s.Depth.Available && s.Depth.SpreadBps <= 25 &&
		s.QuoteVolume >= 10_000_000 &&
		s.OI.NotionalUSD >= 1_500_000 && s.OI.NotionalUSD <= 100_000_000 &&
		s.OIVolumeRatio() >= 0.01
}

func main() {
	dataPath := flag.String("data", "/home/hatch/workspace/ait-t2-data/m3_snapshots/snapshots.jsonl", "extracted snapshots JSONL")
	outPath := flag.String("out", "/home/hatch/workspace/goals/goal-4/hidden_files/kiro-pkg-20261117/m3_recall_results.json", "JSON results output")
	parity := flag.Bool("parity", false, "also run the feature-builder parity check")
	flag.Parse()

	f, err := os.Open(*dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open data:", err)
		os.Exit(1)
	}
	defer f.Close()

	cfg := &local.V7T2Config{Enabled: true} // all families, symbol gate OFF
	var results []eventResult
	perSetup := map[string][2]int{} // setup -> [recalled, total]
	recalled := 0
	var scoreDeltas []float64
	statusAgree := 0
	statusComparable := 0
	funcStatusAgree := 0

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var row extractRow
		if err := json.Unmarshal(line, &row); err != nil {
			fmt.Fprintln(os.Stderr, "parse row:", err)
			os.Exit(1)
		}
		snap := row.Payload.toSnapshot()
		sig := local.EvaluateV7T2Snapshot(snap, cfg, nil)
		r := eventResult{
			EventID: row.EventID, Symbol: row.Symbol,
			ExpectedSetup: row.SetupType, ExpectedDir: row.Direction,
			// The payload is the latest scan's dict: its score/status
			// are the function-time values the detector must reproduce.
			// (The DB score column only moves on creation or on a
			// >10-point upgrade, per spec section 5.2.)
			ExpectedScore: row.Payload.Score, ExpectedStatus: row.Payload.Status,
			ActivityFlags: len(local.V7T2ActivityFlags(snap)),
			Layer1Pass:    layer1Pass(snap),
		}
		if sig != nil {
			r.EmittedSetup = sig.SetupType
			r.EmittedDir = sig.Direction
			r.EmittedScore = sig.Score
			r.EmittedStatus = sig.Status
			if sig.SetupType == row.SetupType && sig.Direction == row.Direction {
				r.Recalled = true
			}
		}
		if !r.Recalled {
			r.Families = local.DiagnoseV7T2Families(snap)
		}
		if r.Recalled {
			recalled++
			// Score deltas are measured against the payload's score
			// (rounded to 1 decimal upstream, so |delta| <= 0.05 is
			// exact reproduction). Status agreement uses the EFFECTIVE
			// status: v0.7.4 payloads store the observe-only-forced
			// WATCH in the dict itself.
			scoreDeltas = append(scoreDeltas, math.Abs(sig.Score-row.Payload.Score))
			statusComparable++
			// Version-aware status agreement: payloads written by
			// v0.7.4 carry the observe-only forcing inside the dict
			// (observe_only=true, status already WATCH); older payloads
			// predate the observe list entirely, so their stored
			// status is the raw function-time status.
			wantStatus := sig.Status
			if row.Payload.ObserveOnly != nil && *row.Payload.ObserveOnly {
				wantStatus = sig.EffectiveStatus()
			}
			if wantStatus == row.Payload.Status {
				statusAgree++
			}
			if sig.Status == row.Payload.Status {
				funcStatusAgree++
			}
		}
		agg := perSetup[row.SetupType]
		agg[1]++
		if r.Recalled {
			agg[0]++
		}
		perSetup[row.SetupType] = agg
		results = append(results, r)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "scan:", err)
		os.Exit(1)
	}

	total := len(results)
	rate := 0.0
	if total > 0 {
		rate = float64(recalled) / float64(total)
	}
	fmt.Printf("M3 RECALL: %d/%d = %.2f%% (gate >= 90%%: %v)\n", recalled, total, rate*100, rate >= 0.90)
	setups := make([]string, 0, len(perSetup))
	for k := range perSetup {
		setups = append(setups, k)
	}
	sort.Strings(setups)
	for _, k := range setups {
		v := perSetup[k]
		fmt.Printf("  %-32s %3d/%3d = %6.2f%%\n", k, v[0], v[1], float64(v[0])/float64(v[1])*100)
	}
	if len(scoreDeltas) > 0 {
		sort.Float64s(scoreDeltas)
		fmt.Printf("recalled score |delta|: p50=%.3f p90=%.3f max=%.3f\n",
			scoreDeltas[len(scoreDeltas)/2], scoreDeltas[int(float64(len(scoreDeltas))*0.9)], scoreDeltas[len(scoreDeltas)-1])
	}
	if statusComparable > 0 {
		fmt.Printf("recalled status agreement (version-aware): %d/%d = %.1f%%\n", statusAgree, statusComparable, float64(statusAgree)/float64(statusComparable)*100)
		fmt.Printf("recalled status agreement (raw function-time): %d/%d = %.1f%%\n", funcStatusAgree, statusComparable, float64(funcStatusAgree)/float64(statusComparable)*100)
	}

	out := map[string]interface{}{
		"total": total, "recalled": recalled, "recall_rate": rate,
		"gate_90pct_pass": rate >= 0.90,
		"per_setup":       perSetup,
		"events":          results,
	}
	if *parity {
		out["parity"] = runParity(*dataPath)
	}
	raw, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write out:", err)
		os.Exit(1)
	}
	fmt.Println("results written to", *outPath)
	if *parity {
		if pm, ok := out["parity"].(map[string]interface{}); ok {
			fmt.Println("parity summary:", pm["summary"])
		}
	}
}

// ---------------------------------------------------------------------------
// Parity mode: Go builders vs Python-stored feature dicts
// ---------------------------------------------------------------------------

func relDiff(a, b float64) float64 {
	if a == b {
		return 0
	}
	m := math.Max(math.Abs(a), math.Abs(b))
	if m == 0 {
		return 0
	}
	return math.Abs(a-b) / m
}

func runParity(dataPath string) map[string]interface{} {
	f, err := os.Open(dataPath)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	defer f.Close()
	type stat struct {
		n       int
		over01  int // fields with rel diff > 0.1%
		over1   int // fields with rel diff > 1%
		maxRel  float64
		worstID string
	}
	stats := map[string]*stat{}
	bump := func(field string, a, b float64, id string) {
		st := stats[field]
		if st == nil {
			st = &stat{}
			stats[field] = st
		}
		st.n++
		d := relDiff(a, b)
		if d > 0.001 {
			st.over01++
		}
		if d > 0.01 {
			st.over1++
		}
		if d > st.maxRel {
			st.maxRel = d
			st.worstID = id
		}
	}
	compareK := func(prefix string, got local.V7T2KlineFeatures, want kFeat, id string) {
		bump(prefix+".price", got.Price, want.Price, id)
		bump(prefix+".atr", got.ATR, want.ATR, id)
		bump(prefix+".atr14", got.ATR14, want.ATR14, id)
		bump(prefix+".rsi", got.RSI, want.RSI, id)
		bump(prefix+".bb_width", got.BBWidth, want.BBWidth, id)
		bump(prefix+".bb_width_min_50", got.BBWidthMin50, want.BBWidthMin50, id)
		bump(prefix+".bb_width_ratio", got.BBWidthRatio, want.BBWidthRatio, id)
		bump(prefix+".volume_ratio", got.VolumeRatio, want.VolumeRatio, id)
		bump(prefix+".resistance", got.Resistance, want.Resistance, id)
		bump(prefix+".support", got.Support, want.Support, id)
		bump(prefix+".prior_high", got.PriorHigh, want.PriorHigh, id)
		bump(prefix+".prior_low", got.PriorLow, want.PriorLow, id)
		if want.EMA20 != nil {
			bump(prefix+".ema20", got.EMA20, *want.EMA20, id)
		}
		if want.EMA50 != nil {
			bump(prefix+".ema50", got.EMA50, *want.EMA50, id)
		}
		bump(prefix+".ema_trend_pct", got.EMATrendPct, want.EMATrendPct, id)
		bump(prefix+".change_12_pct", got.Change12Pct, want.Change12Pct, id)
		bump(prefix+".change_24_pct", got.Change24Pct, want.Change24Pct, id)
	}
	events := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var row extractRow
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			continue
		}
		events++
		p := row.Payload
		conv := func(bs []rawBar) []local.V7T2Bar {
			out := make([]local.V7T2Bar, len(bs))
			for i, b := range bs {
				out[i] = b.toBar()
			}
			return out
		}
		compareK("k5", local.BuildV7T2KlineFeatures(conv(p.Rows5)), p.K5, row.EventID)
		compareK("k1h", local.BuildV7T2KlineFeatures(conv(p.Rows1h)), p.K1h, row.EventID)
		compareK("k4h", local.BuildV7T2KlineFeatures(conv(p.Rows4h)), p.K4h, row.EventID)
		// OI features from raw oi_rows (series selection per spec 2.4).
		gotOI := local.BuildV7T2OIFeatures(oiSeries(p.OIRows))
		bump("oi.oi_5m_pct", gotOI.Pct5m, p.OI.Pct5m, row.EventID)
		bump("oi.oi_15m_pct", gotOI.Pct15m, p.OI.Pct15m, row.EventID)
		bump("oi.oi_1h_pct", gotOI.Pct1h, p.OI.Pct1h, row.EventID)
		bump("oi.oi_notional_usd", gotOI.NotionalUSD, p.OI.Notional, row.EventID)
		vals4 := oiSeries(p.OI4hRows)
		gotAcc := local.BuildV7T2OIAccFeatures(vals4, conv(p.Rows1h))
		bump("oi_acc.oi_4h_pct", gotAcc.Pct4h, p.OIAcc.Pct4h, row.EventID)
		bump("oi_acc.price_change_4h_pct", gotAcc.PriceChange4hPct, p.OIAcc.PriceChg, row.EventID)
		bump("oi_acc.price_amplitude_4h_pct", gotAcc.PriceAmplitude4hPct, p.OIAcc.Ampl, row.EventID)
	}
	fields := map[string]interface{}{}
	worst := []string{}
	for name, st := range stats {
		fields[name] = map[string]interface{}{
			"n": st.n, "over_0.1pct": st.over01, "over_1pct": st.over1,
			"max_rel_diff": st.maxRel, "worst_event": st.worstID,
		}
		if st.over1 > 0 {
			worst = append(worst, fmt.Sprintf("%s: %d/%d fields >1%% (max %.2f%%)", name, st.over1, st.n, st.maxRel*100))
		}
	}
	sort.Strings(worst)
	summary := fmt.Sprintf("parity over %d events; fields with >1%% deviations: %s", events, strings.Join(worst, "; "))
	if len(worst) == 0 {
		summary = fmt.Sprintf("parity over %d events; no field deviates >1%% anywhere", events)
	}
	return map[string]interface{}{"events": events, "fields": fields, "summary": summary}
}
