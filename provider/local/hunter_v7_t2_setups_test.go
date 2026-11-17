package local

import (
	"math"
	"testing"
	"time"
)

// baseT2Snapshot returns a benign snapshot that emits nothing; family
// tests mutate the few fields their family reads.
func baseT2Snapshot() *V7T2Snapshot {
	kf := V7T2KlineFeatures{
		Price: 100, ATR: 1.0, ATR14: 1.0, RSI: 50,
		BBWidth: 0.04, BBWidthMin50: 0.03, BBWidthRatio: 1.5,
		VolumeRatio: 1.0, Resistance: 101, Support: 99,
		PriorHigh: 110, PriorLow: 90,
		EMA20: 99, EMA50: 98, EMATrendPct: 0, HasEMA: true,
	}
	s := &V7T2Snapshot{
		Symbol: "TESTUSDT", Base: "TEST",
		QuoteVolume: 20_000_000, Change24h: 1.0,
		FundingPct: 0.01, GlobalLSR: 1.0, TopLSR: 1.0,
		K5: kf, K15: kf, K1h: kf, K4h: kf,
		OI:            V7T2OIFeatures{NotionalUSD: 10_000_000},
		Taker:         V7T2TakerFeatures{BuyShare: 0.5, BuyShare3: []float64{0.5, 0.5, 0.5}},
		Depth:         V7T2DepthFeatures{BidTotal: 100, AskTotal: 100, Ratio: 1.0, SpreadBps: 5, Available: true},
		OI5mAvailable: true,
	}
	return s
}

func TestV7T2OIValueSeriesSelection(t *testing.T) {
	// Value series filtered to positives (production-verified behavior).
	got := V7T2OIValueSeries([]float64{0, 0, 5, 6}, []float64{100, 100, 100, 100})
	if len(got) != 2 || got[0] != 5 || got[1] != 6 {
		t.Fatalf("filter-positive: got %v", got)
	}
	// All-empty value series -> contract counts.
	got = V7T2OIValueSeries([]float64{0, 0}, []float64{100, 200})
	if len(got) != 2 || got[1] != 200 {
		t.Fatalf("fallback: got %v", got)
	}
}

func TestV7T2KlineFeaturesFixture(t *testing.T) {
	// 30 flat bars then a volume spike: deterministic expectations.
	var rows []V7T2Bar
	for i := 0; i < 30; i++ {
		rows = append(rows, V7T2Bar{Open: 100, High: 101, Low: 99, Close: 100, Volume: 10, CloseTime: int64(i)})
	}
	rows[29].Volume = 100 // last bar 10x the prior mean of 10 -> ratio 10
	f := BuildV7T2KlineFeatures(rows)
	if f.Price != 100 {
		t.Fatalf("price = %v", f.Price)
	}
	if f.RSI != 50 {
		t.Fatalf("flat series RSI = %v, want 50", f.RSI)
	}
	if math.Abs(f.VolumeRatio-10) > 1e-9 {
		t.Fatalf("volume_ratio = %v, want 10", f.VolumeRatio)
	}
	if f.Resistance != 101 || f.Support != 99 {
		t.Fatalf("resistance/support = %v/%v", f.Resistance, f.Support)
	}
	if f.BBWidth != 0 {
		t.Fatalf("flat series bb_width = %v, want 0", f.BBWidth)
	}
}

func ignitionSnapshot() *V7T2Snapshot {
	s := baseT2Snapshot()
	// 1h closes: drift over closes[-9] = +5% (inside the LONG band).
	for i := 0; i < 80; i++ {
		c := 100.0
		if i >= 79 {
			c = 105
		} else if i >= 71 {
			c = 100
		}
		s.Rows1h = append(s.Rows1h, V7T2Bar{Open: c, High: c, Low: c, Close: c, Volume: 1})
	}
	s.K5.VolumeRatio = 2.0
	s.OI.Pct15m = 0
	s.Taker.BuyShare3 = []float64{0.55, 0.56, 0.57}
	s.Taker.BuyShare = 0.57
	return s
}

func TestV7T2IgnitionLong(t *testing.T) {
	s := ignitionSnapshot()
	sig := EvaluateV7T2Snapshot(s, &V7T2Config{Enabled: true}, nil)
	if sig == nil {
		t.Fatal("expected ignition signal, got nil")
	}
	if sig.SetupType != "ignition_early_long" || sig.Direction != "LONG" {
		t.Fatalf("got %s/%s", sig.SetupType, sig.Direction)
	}
	if sig.Status != "WATCH" {
		t.Fatalf("ignition must be hard WATCH, got %s", sig.Status)
	}
	if sig.Score < 62 || sig.Score > 78 {
		t.Fatalf("ignition score out of band: %v", sig.Score)
	}
	// trigger_based_plan invariant: |entry-stop| == risk.
	risk := math.Max(math.Max(s.K5.ATR, s.K5.Price*0.01), s.K5.Price*0.009)
	if math.Abs(math.Abs(sig.Entry-sig.Stop)-risk) > 1e-9 {
		t.Fatalf("|entry-stop| = %v, want risk %v", math.Abs(sig.Entry-sig.Stop), risk)
	}
}

func momentumSnapshot() *V7T2Snapshot {
	s := baseT2Snapshot()
	s.Change24h = 20
	s.OIAcc = V7T2OIAccFeatures{Pct4h: 5, PriceChange4hPct: 3, PriceAmplitude4hPct: 4}
	s.K15.RSI = 60
	s.Taker.BuyShare3 = []float64{0.55, 0.56, 0.57}
	s.Taker.BuyShare = 0.57
	return s
}

func TestV7T2MomentumLongAndExtremeWatch(t *testing.T) {
	sig := EvaluateV7T2Snapshot(momentumSnapshot(), &V7T2Config{Enabled: true}, nil)
	if sig == nil || sig.SetupType != "momentum_continuation_long" {
		t.Fatalf("got %+v", sig)
	}
	if sig.Status != "ARMED" || sig.Score != 96 {
		t.Fatalf("score/status = %v/%s, want 96/ARMED", sig.Score, sig.Status)
	}
	if !sig.HasPullbackShadow || sig.EntryMode != "pullback_shadow" {
		t.Fatal("momentum must carry the pullback shadow plan")
	}
	// Extreme extension demotes to the watch variant with the 79 cap.
	s2 := momentumSnapshot()
	s2.Change24h = 65
	sig2 := EvaluateV7T2Snapshot(s2, &V7T2Config{Enabled: true}, nil)
	if sig2 == nil || sig2.SetupType != "momentum_continuation_watch_long" {
		t.Fatalf("extreme: got %+v", sig2)
	}
	if sig2.Score != 79 || sig2.Status != "WATCH" {
		t.Fatalf("extreme score/status = %v/%s, want 79/WATCH", sig2.Score, sig2.Status)
	}
}

func TestV7T2SecondaryReclaimTouch(t *testing.T) {
	// PLUMEUSDT regression (M3 miss analysis): price exactly equal to
	// resistance must still count as reclaim ("price beyond trigger"
	// includes the touch).
	s := baseT2Snapshot()
	s.Change24h = -2.9
	s.QuoteVolume = 14_400_000
	s.OI = V7T2OIFeatures{NotionalUSD: 7_560_000, Pct5m: 0.93, Pct15m: 1.21, Pct1h: 1.77}
	s.OIAcc = V7T2OIAccFeatures{Pct4h: 19.9, PriceChange4hPct: 1.44, PriceAmplitude4hPct: 3.4}
	s.K15.BBWidthRatio = 1.05
	s.K15.Resistance = 100 // price == resistance (touch)
	s.K15.VolumeRatio = 0.94
	s.K5.VolumeRatio = 0.77
	s.K5.Change12Pct = 1.76
	s.K1h.EMATrendPct = -2.7 // breakout fails its >= -1.5 trend gate
	s.K4h.PriorHigh = 120
	s.K4h.PriorLow = 92
	s.K4h.ATR14 = 5
	s.Taker.BuyShare3 = []float64{0.41, 0.59, 0.69}
	s.Depth.Ratio = 1.99
	sig := EvaluateV7T2Snapshot(s, &V7T2Config{Enabled: true}, nil)
	if sig == nil || sig.SetupType != "reclaim_breakout_long" {
		t.Fatalf("got %+v", sig)
	}
}

func TestV7T2ObserveOnlyAsymmetry(t *testing.T) {
	// Spec section 6.2 asymmetry: long twins flagged, short twins not
	// (except confirmed_breakout, which is flagged both ways).
	cases := map[string]bool{
		"trend_pullback_long":        true,
		"trend_pullback_short":       false,
		"reclaim_breakout_long":      true,
		"reclaim_breakout_short":     false,
		"confirmed_breakout_long":    true,
		"confirmed_breakout_short":   true,
		"compression_oi_watch_long":  true,
		"compression_oi_watch_short": false,
	}
	for setup, want := range cases {
		sig := &V7T2Signal{SetupType: setup, Status: "ARMED", Score: 90}
		v7t2ApplyObserveOnly(sig)
		if sig.ObserveOnly != want {
			t.Fatalf("%s: ObserveOnly = %v, want %v", setup, sig.ObserveOnly, want)
		}
		// Function-time status is never rewritten; forcing is effective-only.
		if sig.Status != "ARMED" {
			t.Fatalf("%s: Status rewritten to %s", setup, sig.Status)
		}
		wantEff := "ARMED"
		if want {
			wantEff = "WATCH"
		}
		if sig.EffectiveStatus() != wantEff {
			t.Fatalf("%s: EffectiveStatus = %s, want %s", setup, sig.EffectiveStatus(), wantEff)
		}
	}
}

func TestV7T2FamilyChecklistAndSymbolGate(t *testing.T) {
	// Checklist: momentum family disabled -> the momentum fixture emits
	// nothing (no re-routing into another family for this snapshot).
	cfg := &V7T2Config{Enabled: true, EnabledSetups: []string{"ignition_early_long", "ignition_early_short"}}
	if sig := EvaluateV7T2Snapshot(momentumSnapshot(), cfg, nil); sig != nil {
		t.Fatalf("checklist leak: got %s", sig.SetupType)
	}
	// Symbol gate: dirty symbol passes with the gate OFF (default),
	// blocked with it ON.
	s := ignitionSnapshot()
	s.Symbol = "龙虾USDT"
	if sig := EvaluateV7T2Snapshot(s, &V7T2Config{Enabled: true}, nil); sig == nil {
		t.Fatal("gate OFF should not block dirty symbols (scanner parity)")
	}
	if sig := EvaluateV7T2Snapshot(s, &V7T2Config{Enabled: true, SymbolValidityGate: true}, nil); sig != nil {
		t.Fatal("gate ON must block dirty symbols")
	}
	if !V7T2SymbolValid("1000PEPEUSDT") || V7T2SymbolValid("BTCUSD") || V7T2SymbolValid("ethusdt") {
		t.Fatal("symbol pattern mismatch")
	}
}

func TestV7T2ModuleDisabledByDefault(t *testing.T) {
	if out := RunV7T2Module(nil, nil, nil, nil, V7RegimeMixed, time.Now()); out != nil {
		t.Fatal("nil config must disable the module")
	}
	if out := RunV7T2Module(nil, &V7T2Config{Enabled: false}, nil, nil, V7RegimeMixed, time.Now()); out != nil {
		t.Fatal("Enabled=false must disable the module")
	}
	if dc := DefaultV7Config(); dc.T2 != nil {
		t.Fatal("DefaultV7Config must leave the Task-2 module unset (disabled)")
	}
}

func TestV7T2EventIDZoning(t *testing.T) {
	// Production check: PLUME reclaim payload (trigger == price ==
	// 0.01969) produced zone 200 on the 0.5% grid.
	sig := &V7T2Signal{Symbol: "PLUMEUSDT", SetupType: "reclaim_breakout_long", Direction: "LONG", Trigger: 0.01969}
	if id := V7T2EventID(sig, 0.01969); id != "PLUMEUSDT:reclaim_breakout_long:LONG:200" {
		t.Fatalf("event id = %s", id)
	}
	// Momentum always lands on zone 50 when trigger == price (2% grid).
	msig := &V7T2Signal{Symbol: "XUSDT", SetupType: "momentum_continuation_long", Direction: "LONG", Trigger: 100}
	if id := V7T2EventID(msig, 100); id != "XUSDT:momentum_continuation_long:LONG:50" {
		t.Fatalf("momentum event id = %s", id)
	}
}

func TestV7T2LifecycleStore(t *testing.T) {
	st := NewV7T2EventStore()
	t0 := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	mk := func(setup string, score float64, status string) *V7T2Signal {
		return &V7T2Signal{Symbol: "AAAUSDT", Direction: "LONG", SetupType: setup, Score: score, Status: status, Trigger: 100}
	}
	// Create ARMED: armed_at set, 4h expiry, notify.
	r1 := st.Upsert(mk("ignition_early_long", 70, "WATCH"), 100, t0)
	if !r1.Created || r1.Event.Status != "WATCH" || r1.Event.ArmedAt != nil {
		t.Fatalf("create WATCH: %+v", r1.Event)
	}
	if !r1.Event.ExpiresAt.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("WATCH expiry = %v", r1.Event.ExpiresAt)
	}
	// Re-hit within cooldown window: same event, watch_hits=2, expiry unchanged.
	r2 := st.Upsert(mk("ignition_early_long", 71, "WATCH"), 100, t0.Add(10*time.Minute))
	if r2.Created || r2.Event.WatchHits != 2 || !r2.Event.ExpiresAt.Equal(r1.Event.ExpiresAt) {
		t.Fatalf("re-hit: %+v", r2.Event)
	}
	// New setup, same symbol+direction, 30 min later -> cooldown drop.
	r3 := st.Upsert(mk("trend_pullback_long", 90, "ARMED"), 100, t0.Add(30*time.Minute))
	if !r3.Dropped || r3.DropReason != "cooldown" {
		t.Fatalf("cooldown: %+v", r3)
	}
	// 61 min later -> created; ARMED gets armed_at + 4h expiry + notify.
	r4 := st.Upsert(mk("trend_pullback_long", 90, "ARMED"), 100, t0.Add(61*time.Minute))
	if !r4.Created || !r4.Notify || r4.Event.ArmedAt == nil {
		t.Fatalf("create ARMED: %+v", r4)
	}
	if !r4.Event.ExpiresAt.Equal(t0.Add(61 * time.Minute).Add(4 * time.Hour)) {
		t.Fatalf("ARMED expiry = %v", r4.Event.ExpiresAt)
	}
	// Observe-only signal: event forced WATCH, no armed_at, 2h expiry.
	obs := mk("trend_pullback_long", 94, "ARMED")
	obs.Trigger = 200 // different zone -> different event_id
	v7t2ApplyObserveOnly(obs)
	r5 := st.Upsert(obs, 100, t0.Add(3*time.Hour))
	if !r5.Created || r5.Event.Status != "WATCH" || r5.Event.ArmedAt != nil || r5.Notify {
		t.Fatalf("observe-only event: %+v", r5.Event)
	}
	// Terminal events are never revived: expire r1's event, re-upsert.
	r6 := st.Upsert(mk("ignition_early_long", 75, "WATCH"), 100, t0.Add(3*time.Hour))
	if !r6.Dropped || r6.DropReason != "terminal_event" {
		t.Fatalf("terminal revival: %+v", r6)
	}
	// Activity invalidation stamps live events.
	if n := st.InvalidateActivity("AAAUSDT", t0.Add(3*time.Hour)); n == 0 {
		t.Fatal("expected at least one activity invalidation")
	}
}
