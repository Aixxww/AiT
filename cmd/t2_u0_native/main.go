// Command t2_u0_native is the U0-B harness (redesign plan U0; supersedes
// the MEAS3 measurement). For each stored Task-2 snapshot it runs the T2
// detector, lifecycle store and candidate assembly (MEAS2 quantities), then
// rebuilds a single-symbol datafetch.Snapshot from the stored payload and
// runs the native Hunter v7 stack (DefaultV7Config, T2 module off) to record
// native final/raw fires, opposite-direction co-fire, and each final
// signal's own stop/TP1 for the calibration walk.
//
// Snapshot reconstruction (fixed for U0-B): 5m/1h/4h bars verbatim, 15m
// aggregated from grid-aligned 5m bars, bar times shifted so the last 5m
// bar closes at run time; funding = funding_pct/100; LSR and taker ratio per
// -lsr / -taker (U0-B used -lsr top -taker raw); 24h high/low from the last
// 24 1h bars; OI from the last oi_rows entry; OI spike series from
// consecutive oi_4h_rows values. Limitations are listed in the U0 report.
//
// Usage:
//
//	go run ./cmd/t2_u0_native -data snapshots.jsonl -out u0b_rows.json -lsr top -taker raw
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/Aixxww/AiT/cmd/internal/t2payload"
	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
)

type oiRowT struct {
	SumOIValue string  `json:"sumOpenInterestValue"`
	SumOI      string  `json:"sumOpenInterest"`
	Timestamp  float64 `json:"timestamp"`
}

type extraPayload struct {
	Rows5    []t2payload.RawBar `json:"rows5"`
	Rows1h   []t2payload.RawBar `json:"rows1h"`
	Rows4h   []t2payload.RawBar `json:"rows4h"`
	OIRows   []oiRowT           `json:"oi_rows"`
	OI4hRows []oiRowT           `json:"oi_4h_rows"`
}

type extraRow struct {
	Payload extraPayload `json:"payload"`
}

type evOut struct {
	EventID        string   `json:"event_id"`
	Symbol         string   `json:"symbol"`
	T2Setup        string   `json:"t2_setup"`
	T2Direction    string   `json:"t2_direction"`
	T2Detected     bool     `json:"t2_detected"`
	T2FuncStatus   string   `json:"t2_func_status"`
	T2EffStatus    string   `json:"t2_effective_status"`
	T2EffNilRegime string   `json:"t2_effective_status_nil_regime"`
	T2ObserveOnly  bool     `json:"t2_observe_only"`
	T2Score        float64  `json:"t2_score"`
	CandStatus     string   `json:"candidate_status"`
	CandTier       string   `json:"candidate_tier"`
	CandReason     string   `json:"candidate_tier_reason"`
	StoreStatus    string   `json:"store_initial_status"`
	StoreExpiryH   float64  `json:"store_expiry_hours"`
	NativeIn       bool     `json:"native_universe_in"`
	NativeRegime   string   `json:"native_regime"`
	NativeRaw      int      `json:"native_raw_fires"`
	NativeFinal    int      `json:"native_final_fires"`
	CofireFinal    bool     `json:"cofire_final"`
	CofireRaw      bool     `json:"cofire_raw"`
	RawFamilies    []string `json:"raw_families"`
	FinalFamilies  []string `json:"final_families"`
	CofireFinalFam []string `json:"cofire_final_families"`
	CofireRawFam   []string `json:"cofire_raw_families"`
	SnapTs         string   `json:"snap_ts"`
	Final          []nsig   `json:"final_signals"`
}

type nsig struct {
	Setup      string  `json:"setup"`
	Direction  string  `json:"direction"`
	Status     string  `json:"status"`
	AIPriority float64 `json:"ai_priority"`
	SetupScore float64 `json:"setup_score"`
	Price      float64 `json:"price"`
	Stop       float64 `json:"stop"`
	TP1        float64 `json:"tp1"`
	Quality    string  `json:"execution_quality"`
}

func toKlines(rows []t2payload.RawBar, intervalMs, shift int64) []datafetch.Kline {
	out := make([]datafetch.Kline, 0, len(rows))
	for _, b := range rows {
		ct := int64(b.CloseTime) + shift
		out = append(out, datafetch.Kline{OpenTime: ct + 1 - intervalMs, CloseTime: ct, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume})
	}
	return out
}

// agg15 aggregates phase-aligned 5m bars into 15m bars: leading bars are
// dropped until a bar opens on the 15m grid.
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

var (
	optFunding = "pct100" // pct100: rate = funding_pct/100; raw: rate = funding_pct
	optLSR     = "global" // global | top
	optTaker   = "share"  // share | raw | ratio1p | zero
	optOI      = "value"  // value | contracts
	optHL      = 24       // 1h bars for the 24h high/low
)

func buildNativeSnapshot(p *t2payload.Payload, x *extraPayload, now time.Time) *datafetch.Snapshot {
	if len(x.Rows5) == 0 {
		return nil
	}
	last := int64(x.Rows5[len(x.Rows5)-1].CloseTime)
	shift := now.UnixMilli() - last
	ss := &datafetch.SymbolSnapshot{
		Symbol: p.Symbol, Price: p.LastPrice, Timestamp: now,
		PriceChange24h: p.Change24h, QuoteVolume24h: p.QuoteVolume,
		MarkPrice: p.MarkPrice, IndexPrice: p.MarkPrice, FundingRate: p.FundingPct / 100,
		LongShortRatio: p.GlobalLSR, TakerBuyRatio: p.TakerSignal.BuyShare,
		Klines: map[string][]datafetch.Kline{},
	}
	if optFunding == "raw" {
		ss.FundingRate = p.FundingPct
	}
	if optLSR == "top" {
		ss.LongShortRatio = p.TopLSR
	}
	switch optTaker {
	case "raw":
		ss.TakerBuyRatio = p.TakerRatio
	case "ratio1p":
		ss.TakerBuyRatio = p.TakerRatio / (1 + p.TakerRatio)
	case "zero":
		ss.TakerBuyRatio = 0
	}
	if p.LastPrice > 0 {
		ss.Volume24h = p.QuoteVolume / p.LastPrice
	}
	k5 := toKlines(x.Rows5, 5*60000, shift)
	ss.Klines["5m"] = k5
	ss.Klines["15m"] = agg15(k5, func(i int) int64 { return int64(x.Rows5[i].CloseTime) + 1 - 5*60000 })
	ss.Klines["1h"] = toKlines(x.Rows1h, 3600000, shift)
	ss.Klines["4h"] = toKlines(x.Rows4h, 4*3600000, shift)
	h := ss.Klines["1h"]
	n := optHL
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
	if len(x.OIRows) > 0 {
		ss.OI = pf(x.OIRows[len(x.OIRows)-1].SumOI)
	}
	var spikes []float64
	for i := 1; i < len(x.OI4hRows); i++ {
		a, b := pf(x.OI4hRows[i-1].SumOIValue), pf(x.OI4hRows[i].SumOIValue)
		if optOI == "contracts" {
			a, b = pf(x.OI4hRows[i-1].SumOI), pf(x.OI4hRows[i].SumOI)
		}
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

func nonPlaceholder(s local.V7SignalOutput) bool {
	return s.SetupType != "module_no_match" && s.Status != local.V7StatusFiltered
}

func main() {
	dataPath := flag.String("data", "", "snapshots.jsonl (reconstructed from the p00..p07 parts)")
	outPath := flag.String("out", "", "per-event JSON output path")
	flag.StringVar(&optFunding, "funding", optFunding, "funding mapping: pct100 or raw")
	flag.StringVar(&optLSR, "lsr", optLSR, "LSR source: global or top")
	flag.StringVar(&optTaker, "taker", optTaker, "taker ratio source: share, raw, ratio1p or zero")
	flag.StringVar(&optOI, "oi", optOI, "OI spike source: value or contracts")
	flag.IntVar(&optHL, "hl", optHL, "1h bars used for the 24h high/low")
	flag.Parse()
	f, err := os.Open(*dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	cfg := &local.V7T2Config{Enabled: true}
	geom := kernel.HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	var outs []evOut
	for sc.Scan() {
		line := sc.Bytes()
		var row t2payload.ExtractRow
		var xr extraRow
		if err := json.Unmarshal(line, &row); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = json.Unmarshal(line, &xr)
		o := evOut{EventID: row.EventID, Symbol: row.Symbol, T2Setup: row.SetupType, T2Direction: row.Direction, SnapTs: row.Payload.Ts}
		snap := row.Payload.ToSnapshot()
		var regime *local.V7T2RegimeInput
		if row.Payload.RegimeRV24 != nil {
			regime = &local.V7T2RegimeInput{RV24: *row.Payload.RegimeRV24, Valid: true}
		}
		sig := local.EvaluateV7T2Snapshot(snap, cfg, regime)
		if nilSig := local.EvaluateV7T2Snapshot(row.Payload.ToSnapshot(), cfg, nil); nilSig != nil {
			o.T2EffNilRegime = nilSig.EffectiveStatus()
		}
		if sig != nil && sig.SetupType == row.SetupType && sig.Direction == row.Direction {
			o.T2Detected = true
		}
		if sig != nil {
			o.T2FuncStatus, o.T2EffStatus, o.T2ObserveOnly, o.T2Score = sig.Status, sig.EffectiveStatus(), sig.ObserveOnly, sig.Score
			out := sig.ToV7SignalOutput(local.V7MarketRegime(""))
			cands := kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{out}, "BOTH", geom)
			if len(cands) == 1 {
				o.CandStatus, o.CandTier, o.CandReason = cands[0].V7Status, cands[0].V7ExecutionTier, cands[0].V7TierReason
			}
			now := time.Unix(1_800_000_000, 0)
			st := local.NewV7T2EventStore()
			ur := st.Upsert(sig, snap.Price(), now)
			if ev := ur.Event; ev != nil {
				o.StoreStatus = ev.Status
				o.StoreExpiryH = ev.ExpiresAt.Sub(ev.CreatedAt).Hours()
			}
		}
		// MEAS3: native stack on a rebuilt single-symbol snapshot.
		var nativeSigs []local.V7SignalOutput // U6 golden replay input
		now := time.Now()
		if ns := buildNativeSnapshot(&row.Payload, &xr.Payload, now); ns != nil {
			res := local.ScoreHunterV7Detailed(ns, local.DefaultV7Config())
			o.NativeIn = len(res.Universe) > 0
			o.NativeRegime = string(res.Regime)
			if !o.NativeIn {
				o.NativeRegime = ""
			}
			for _, s := range res.RawSignals {
				if s.Symbol == row.Symbol && nonPlaceholder(s) {
					o.NativeRaw++
					o.RawFamilies = append(o.RawFamilies, string(s.SetupType))
					if string(s.Direction) != row.Direction {
						o.CofireRaw = true
						o.CofireRawFam = append(o.CofireRawFam, string(s.SetupType))
					}
				}
			}
			for _, s := range res.Signals {
				if s.Symbol == row.Symbol {
					nativeSigs = append(nativeSigs, s)
					tp1 := s.TP1Price
					if tp1 <= 0 && len(s.Targets) > 0 {
						tp1 = s.Targets[0].Price
					}
					o.Final = append(o.Final, nsig{Setup: string(s.SetupType), Direction: string(s.Direction), Status: string(s.Status),
						AIPriority: s.AIPriority, SetupScore: s.SetupScore, Price: row.Payload.LastPrice, Stop: s.Invalidation.Price, TP1: tp1, Quality: string(s.ExecutionQuality)})
					o.NativeFinal++
					o.FinalFamilies = append(o.FinalFamilies, string(s.SetupType))
					if string(s.Direction) != row.Direction {
						o.CofireFinal = true
						o.CofireFinalFam = append(o.CofireFinalFam, string(s.SetupType))
					}
				}
			}
		}
		goldenAdd(row.EventID, sig, nativeSigs)
		conflictAdd(row.EventID, sig, nativeSigs)
		outs = append(outs, o)
	}
	goldenWrite()
	conflictWrite()
	b, _ := json.Marshal(outs)
	_ = os.WriteFile(*outPath, b, 0o644)
	fmt.Println("events", len(outs))
}
