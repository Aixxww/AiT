package kernel

import (
	"flag"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/market"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// U11 prompt fixture: one cycle with a native EXECUTABLE leader, a Task-2
// TRIGGER conditional entry, a Task-2 IMPULSE and REVERSAL on TRIGGERED, and
// a native WATCH, each with 5m/15m/1h/4h bars and indicator series. The
// legacy golden was recorded on the unchanged base (38c1ce2) before any U11
// code change; the hybrid_v2 golden records the new payload.

var updateU11PromptGolden = flag.Bool("update-u11-prompt-golden", false, "re-record the U11 prompt goldens")

// u11PromptTime is the fixture's prompt build time: 7.5 minutes into a 15m
// bar, so every timeframe has one unfinished bar.
var u11PromptTime = time.Date(2026, 11, 15, 12, 7, 30, 0, time.UTC)

func u11FixtureConfig(lang string) *store.StrategyConfig {
	cfg := store.GetDefaultStrategyConfig(lang)
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.Hunter = &store.HunterConfig{
		V7T2Enabled:        true,
		V7ConditionalEntry: &store.V7ConditionalEntryConfig{Enabled: true},
	}
	cfg.CustomPrompt = ""
	return &cfg
}

// u11Bars builds n bars of one timeframe ending with the bar that contains
// at (the unfinished bar), with right-aligned indicator series shorter than
// the bars where the indicator's warm-up would make them shorter.
func u11Bars(tf string, n int, base float64, at time.Time, phase float64) *market.TimeframeSeriesData {
	iv := u11TFInterval(tf)
	lastOpen := at.UnixMilli() / iv.Milliseconds() * iv.Milliseconds()
	s := &market.TimeframeSeriesData{Timeframe: tf}
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		c := base * (1 + 0.006*math.Sin(float64(i)/3+phase) + 0.0004*float64(i-n))
		o := base * (1 + 0.006*math.Sin(float64(i-1)/3+phase) + 0.0004*float64(i-1-n))
		h := math.Max(o, c) * 1.0015
		l := math.Min(o, c) * 0.9985
		closes[i] = c
		s.Klines = append(s.Klines, market.KlineBar{
			Time: lastOpen - int64(n-1-i)*iv.Milliseconds(), Open: o, High: h, Low: l, Close: c,
			Volume: 1000 + float64((i*37)%11)*100,
		})
	}
	tail := func(k int, f func(i int) float64) []float64 {
		var out []float64
		for i := k; i < n; i++ {
			out = append(out, f(i))
		}
		return out
	}
	s.EMA20Values = tail(19, func(i int) float64 { return closes[i] * 0.999 })
	s.EMA50Values = tail(n-12, func(i int) float64 { return closes[i] * 0.997 }) // short series
	s.MACDValues = tail(25, func(i int) float64 { return closes[i] * 0.0004 })
	s.RSI7Values = tail(7, func(i int) float64 { return 50 + 10*math.Sin(float64(i)/4) })
	s.RSI14Values = tail(14, func(i int) float64 { return 50 + 6*math.Sin(float64(i)/5) })
	s.BOLLUpper = tail(19, func(i int) float64 { return closes[i] * 1.01 })
	s.BOLLMiddle = tail(19, func(i int) float64 { return closes[i] })
	s.BOLLLower = tail(19, func(i int) float64 { return closes[i] * 0.99 })
	s.ATR14 = base * 0.004
	return s
}

func u11TFInterval(tf string) time.Duration {
	switch tf {
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	}
	return time.Minute
}

func u11MarketData(symbol string, price float64, phase float64) *market.Data {
	d := &market.Data{
		Symbol: symbol, CurrentPrice: price, CurrentRSI7: 55,
		OpenInterest:  &market.OIData{Latest: 1_000_000, Average: 950_000},
		FundingRate:   0.0001,
		TimeframeData: map[string]*market.TimeframeSeriesData{},
	}
	for _, tf := range []string{"5m", "15m", "1h", "4h"} {
		d.TimeframeData[tf] = u11Bars(tf, 40, price, u11PromptTime, phase)
	}
	return d
}

func u11T2Signal(symbol, setup string, dir local.V7Direction, state string, price float64) local.V7SignalOutput {
	stop, tp1, tp2 := price*0.99, price*1.018, price*1.03
	if dir == local.V7DirShort {
		stop, tp1, tp2 = price*1.01, price*0.982, price*0.97
	}
	return local.V7SignalOutput{
		SignalID: "sig-" + strings.ToLower(symbol), Stack: local.V7StackT2, LifecycleState: state, TriggerPrice: price, ATR15mPct: 1.25,
		Symbol: symbol, Direction: dir, SetupType: local.V7SetupType(setup), Status: local.V7StatusCandidate,
		AIPriority: 70, SetupScore: 82, LiquidityScore: 80, Confidence: "MEDIUM", MarketRegime: "BULL",
		EntryZone: local.V7PriceZone{Lower: price, Upper: price}, Invalidation: local.V7InvalidationRule{Price: stop, Reason: "t2_stop"},
		Targets: []local.V7Target{{Price: tp1}, {Price: tp2}}, TP1Price: tp1, TP2Price: tp2, TP1RR: 1.8, TP2RR: 3.0, RiskLevel: local.V7RiskMedium,
		ReasonCodes:    []string{"flow_taker_buy_strong"},
		PriceCtx:       &local.V7PriceContext{Last: price, Change1h: 0.4, Change4h: 1.1, ATR1h: price * 0.006, ATR4h: price * 0.012, VWAP15m: price * 0.998},
		DerivativesCtx: &local.V7DerivativesContext{OIValue: 12_500_000, OIChange1h: 1.4, OIChange4h: 3.2, FundingRate: 0.0001, TakerBuy15m: 0.58},
		DataFreshness:  local.V7DataFreshness{SnapshotAgeMs: 4200, PriceAgeMs: 900, Kline1mAgeMs: 31000, Kline5mAgeMs: 151000, SnapshotFetchMs: 3800, StaleThresholdMs: 90000},
	}
}

// u11FixtureContext returns the fixture cycle for an engine.
func u11FixtureContext(e *StrategyEngine) *Context {
	t2 := []local.V7SignalOutput{
		u11T2Signal("TRIGUSDT", "ignition_early_long", local.V7DirLong, "WATCH", 100),
		u11T2Signal("IMPUSDT", "momentum_continuation_short", local.V7DirShort, "TRIGGERED", 50),
		u11T2Signal("REVUSDT", "trend_pullback_short", local.V7DirShort, "TRIGGERED", 20),
	}
	coins := AssembleHunterV7CandidateCoins(t2, "BOTH", e.hunterV7ExecutionGeometry())
	leader := hunterV7PromptReadyLeaderCandidate("LEADUSDT", 2.0)
	leader.V7SignalID = "sig-leadusdt"
	leader.V7EntryZone = local.V7PriceZone{Lower: 1.995, Upper: 2.005}
	leader.V7Invalidation = local.V7InvalidationRule{Price: 1.95, Reason: "below_4h_support"}
	leader.V7Targets = []local.V7Target{{Price: 2.12, Reason: "atr_target"}, {Price: 2.2}}
	leader.V7TP1Price, leader.V7TP1RR, leader.V7TP2Price, leader.V7TP2RR = 2.12, 2.4, 2.2, 4.0
	leader.V7MarketRegime = "BULL"
	leader.V7MarketShape = "trend"
	leader.V7EntrySignal = "breakout"
	leader.V7DataFreshness = local.V7DataFreshness{SnapshotAgeMs: 5100, PriceAgeMs: 1200}
	watch := CandidateCoin{
		Symbol: "WATCHUSDT", Sources: []string{"hunter"}, Direction: "SHORT",
		V7SetupType: "pre_breakout_watch", V7Status: "wait_confirm", V7ExecutionQuality: "watch_only",
		V7AIPriority: 70, V7RiskScore: 20, V7LiquidityScore: 80, V7MarketShape: "range", V7EntrySignal: "none",
		V7RiskTags: []string{"do_not_open_until_confirmed"},
	}
	coins = append([]CandidateCoin{leader}, append(coins, watch)...)
	return &Context{
		CurrentTime: "2026-11-15 12:07:30", CallCount: 7, RuntimeMinutes: 35,
		Account:        AccountInfo{TotalEquity: 1000, AvailableBalance: 1000},
		CandidateCoins: coins,
		MarketDataMap: map[string]*market.Data{
			"LEADUSDT":  u11MarketData("LEADUSDT", 2.0, 0.1),
			"TRIGUSDT":  u11MarketData("TRIGUSDT", 100.2, 0.7),
			"IMPUSDT":   u11MarketData("IMPUSDT", 49.9, 1.3),
			"REVUSDT":   u11MarketData("REVUSDT", 20.05, 2.1),
			"WATCHUSDT": u11MarketData("WATCHUSDT", 3.0, 2.9),
		},
	}
}

// u11RenderPrompts renders system + user prompt as one golden document.
// The leading data dictionary (GetSchemaPrompt) is cut: it ranges over
// maps, so its line order differs run to run.
func u11RenderPrompts(cfg *store.StrategyConfig) string {
	e := NewStrategyEngine(cfg)
	ctx := u11FixtureContext(e)
	sys := e.BuildSystemPrompt(1000, "")
	if i := strings.Index(sys, "\n\n---\n\n"); i >= 0 {
		sys = "<data dictionary omitted>" + sys[i:]
	}
	return "===== SYSTEM =====\n" + sys + "\n===== USER =====\n" + e.BuildUserPrompt(ctx)
}

func u11CheckGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateU11PromptGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("recorded %s (%d bytes)", path, len(got))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update-u11-prompt-golden to record): %v", path, err)
	}
	if got == string(want) {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(gl) && i < len(wl); i++ {
		if gl[i] != wl[i] {
			t.Fatalf("%s diverged at line %d:\n want: %s\n got:  %s", name, i+1, wl[i], gl[i])
		}
	}
	t.Fatalf("%s diverged in length: %d -> %d lines", name, len(wl), len(gl))
}

// The legacy prompt (no v7_prompt block, the default) must stay
// byte-identical to the pre-U11 recording, in both languages.
func TestU11LegacyPromptGolden(t *testing.T) {
	u11CheckGolden(t, "hunter_v7_prompt_legacy_en.golden", u11RenderPrompts(u11FixtureConfig("en")))
	u11CheckGolden(t, "hunter_v7_prompt_legacy_zh.golden", u11RenderPrompts(u11FixtureConfig("zh")))
}
