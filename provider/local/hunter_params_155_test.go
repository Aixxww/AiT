package local

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/store"
)

// #155: every Hunter advanced-filter setting changes the scoring outcome when
// set, and an empty config reproduces the pre-#155 behavior.

// fx155Klines builds n bars whose closes descend linearly from start to
// end (the last close), with a ±0.5 high/low band.
func fx155Klines(n int, start, end, volume float64) []datafetch.Kline {
	out := make([]datafetch.Kline, n)
	for i := range out {
		c := start + (end-start)*float64(i)/float64(n-1)
		out[i] = datafetch.Kline{Open: c, High: c + 0.5, Low: c - 0.5, Close: c, Volume: volume, TakerBuy: volume / 2}
	}
	return out
}

// fx155Symbol is a strong LONG candidate: price at the bottom of every
// timeframe (near support everywhere), OI rising with price, bullish LSR.
func fx155Symbol(sym string) *datafetch.SymbolSnapshot {
	return &datafetch.SymbolSnapshot{
		Symbol:         sym,
		Price:          100,
		PriceChange24h: 2,
		QuoteVolume24h: 50_000_000,
		Volume24h:      500_000,
		TradeCount24h:  200_000,
		OI:             20_000_000,
		OIDelta1h:      1,
		OISpikeData:    []float64{1, 1, 1, 1, 3, 3, 3, 3},
		LongShortRatio: 1.2,
		LSRPrev:        1.0,
		Klines: map[string][]datafetch.Kline{
			"5m":  fx155Klines(50, 101, 100, 1000),
			"15m": fx155Klines(50, 103, 100, 1000),
			"1h":  fx155Klines(20, 106, 100, 10_000),
			"4h":  fx155Klines(20, 110, 100, 10_000),
			"1d":  fx155Klines(20, 130, 100, 10_000),
		},
	}
}

func fx155Snap(syms ...*datafetch.SymbolSnapshot) *datafetch.Snapshot {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{}, CreatedAt: time.Now()}
	for _, s := range syms {
		snap.Symbols[s.Symbol] = s
	}
	return snap
}

func fx155Score(t *testing.T, ss *datafetch.SymbolSnapshot, cfg *store.HunterConfig) (HunterCoinScore, bool) {
	t.Helper()
	resetBBPercentileCache155()
	p := ResolveHunterParams(cfg)
	for _, s := range ScoreHunterFromSnapshot(fx155Snap(ss), HunterSnapshotConfig{Params: &p}) {
		if s.Symbol == ss.Symbol {
			return s, true
		}
	}
	return HunterCoinScore{}, false
}

func resetBBPercentileCache155() {
	hunterBBPercentileCache.mu.Lock()
	hunterBBPercentileCache.entries = map[string]bbPercentileEntry{}
	hunterBBPercentileCache.now = time.Now
	hunterBBPercentileCache.mu.Unlock()
}

func has155(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// The defaults are the pre-#155 literals, and an empty config scores
// exactly like no config.
func TestHunter155DefaultsPreserveBehavior(t *testing.T) {
	p := ResolveHunterParams(&store.HunterConfig{})
	if !reflect.DeepEqual(p, ResolveHunterParams(nil)) {
		t.Fatalf("empty config resolves differently from nil: %+v", p)
	}
	if p.ChasePct != 50 || p.Wash != (HunterWashThresholds{TradeCount: 1_000_000, AvgOrderUSD: 5, OIVolRatio: 0.01, SpikeMult: 10}) ||
		!p.Cooldown || p.RSIOversold != 0 || p.RSIOverbought != 0 || p.FundingSignal || p.MinTradeCount != 0 ||
		p.PositionTimeframes != nil || p.Breakout {
		t.Fatalf("defaults differ from the pre-#155 hardcoded behavior: %+v", p)
	}
	// The legacy enable_cooldown=false every config carries does not turn
	// the cooldown off (owner decision).
	if !ResolveHunterParams(&store.HunterConfig{EnableCooldown: false}).Cooldown {
		t.Fatal("legacy enable_cooldown=false must not disable the cooldown")
	}
	ss := fx155Symbol("DEFUSDT")
	resetBBPercentileCache155()
	a := ScoreHunterFromSnapshot(fx155Snap(ss), HunterSnapshotConfig{})
	b, _ := fx155Score(t, ss, &store.HunterConfig{})
	if len(a) != 1 || a[0].FinalScore != b.FinalScore || !reflect.DeepEqual(a[0].Tags, b.Tags) {
		t.Fatalf("nil Params vs empty config differ: %+v vs %+v", a, b)
	}
	if a[0].Direction != "LONG" || a[0].FinalScore < 30 {
		t.Fatalf("fixture must be a strong LONG: %+v", a[0])
	}
}

func TestHunter155RSIThresholds(t *testing.T) {
	ss := fx155Symbol("RSIUSDT") // 1h closes fall steadily: RSI near 0
	base, _ := fx155Score(t, ss, nil)
	got, _ := fx155Score(t, ss, &store.HunterConfig{RSIOversoldThreshold: 30})
	if math.Abs(got.LongFinalScore-base.LongFinalScore-5) > 1e-9 || !has155(got.LongTags, "rsi_oversold") {
		t.Fatalf("oversold: long %.2f -> %.2f, tags %v; want +5 and rsi_oversold", base.LongFinalScore, got.LongFinalScore, got.LongTags)
	}
	// Overbought only: RSI ~0 is not above 70, nothing changes.
	ob, _ := fx155Score(t, ss, &store.HunterConfig{RSIOverboughtThreshold: 70})
	if ob.LongFinalScore != base.LongFinalScore {
		t.Fatal("overbought threshold must not fire on an oversold RSI")
	}
	// Overbought fires on rising 1h closes: LONG -5.
	up := fx155Symbol("RSIUPUSDT")
	up.Klines["1h"] = fx155Klines(20, 94, 100, 10_000)
	upBase, _ := fx155Score(t, up, nil)
	upGot, _ := fx155Score(t, up, &store.HunterConfig{RSIOverboughtThreshold: 70})
	if !has155(upGot.LongTags, "rsi_overbought_long_penalty") || upGot.LongFinalScore >= upBase.LongFinalScore {
		t.Fatalf("overbought: long %.2f -> %.2f tags %v", upBase.LongFinalScore, upGot.LongFinalScore, upGot.LongTags)
	}
}

func TestHunter155FundingSignal(t *testing.T) {
	ss := fx155Symbol("FUNDUSDT")
	ss.FundingRate = -0.001
	base, _ := fx155Score(t, ss, nil)
	got, _ := fx155Score(t, ss, &store.HunterConfig{EnableFundingRateSignal: true})
	if math.Abs(got.LongFinalScore-base.LongFinalScore-5) > 1e-9 || !has155(got.LongTags, "funding_negative_long_bias") {
		t.Fatalf("funding: long %.2f -> %.2f tags %v", base.LongFinalScore, got.LongFinalScore, got.LongTags)
	}
	ss.FundingRate = 0.0001 // inside ±0.05%: no effect
	in, _ := fx155Score(t, ss, &store.HunterConfig{EnableFundingRateSignal: true})
	inBase, _ := fx155Score(t, ss, nil)
	if in.LongFinalScore != inBase.LongFinalScore || in.ShortFinalScore != inBase.ShortFinalScore {
		t.Fatal("funding inside the band must not change scores")
	}
}

func TestHunter155Max24hChange(t *testing.T) {
	ss := fx155Symbol("CHASEUSDT") // 24h change 2%
	base, _ := fx155Score(t, ss, nil)
	got, _ := fx155Score(t, ss, &store.HunterConfig{Max24hChange: 1})
	if !has155(got.LongTags, "chase_penalty") || got.LongFinalScore >= base.LongFinalScore || has155(base.LongTags, "chase_penalty") {
		t.Fatalf("max_24h_change=1: long %.2f -> %.2f tags %v", base.LongFinalScore, got.LongFinalScore, got.LongTags)
	}
}

func TestHunter155WashSensitivity(t *testing.T) {
	ss := fx155Symbol("WASHUSDT")
	// OI/volume ratio 0.015: inside medium (0.01), caught by high (0.02).
	ss.Volume24h = ss.OI / 0.015
	ss.Klines["1h"] = fx155Klines(20, 106, 100, 60_000_000) // no volume-spike hit at any level
	med, _ := fx155Score(t, ss, &store.HunterConfig{WashTradeSensitivity: "medium"})
	def, _ := fx155Score(t, ss, nil)
	high, _ := fx155Score(t, ss, &store.HunterConfig{WashTradeSensitivity: "high"})
	if med.WashMod != 1 || def.WashMod != 1 || math.Abs(high.WashMod-0.30) > 1e-9 {
		t.Fatalf("wash mods default/medium/high = %v/%v/%v, want 1/1/0.30", def.WashMod, med.WashMod, high.WashMod)
	}
	// low is looser: a 0.008 ratio is caught by medium, not by low.
	ss.Volume24h = ss.OI / 0.008
	m2, _ := fx155Score(t, ss, nil)
	l2, _ := fx155Score(t, ss, &store.HunterConfig{WashTradeSensitivity: "low"})
	if math.Abs(m2.WashMod-0.30) > 1e-9 || l2.WashMod != 1 {
		t.Fatalf("ratio 0.008: medium %v low %v, want 0.30 / 1", m2.WashMod, l2.WashMod)
	}
}

func TestHunter155DisableCooldown(t *testing.T) {
	ss := fx155Symbol("COOLUSDT")
	globalCooldown.mu.Lock()
	globalCooldown.symbols["COOLUSDT"] = &symbolCooldown{ConsecutiveWaits: 2}
	globalCooldown.mu.Unlock()
	defer func() {
		globalCooldown.mu.Lock()
		delete(globalCooldown.symbols, "COOLUSDT")
		globalCooldown.mu.Unlock()
	}()
	on, _ := fx155Score(t, ss, nil)
	off, _ := fx155Score(t, ss, &store.HunterConfig{DisableCooldown: true})
	if on.CooldownMod != 0.5 || off.CooldownMod != 1 || off.FinalScore <= on.FinalScore {
		t.Fatalf("cooldown mod on/off = %v/%v, scores %.2f/%.2f", on.CooldownMod, off.CooldownMod, on.FinalScore, off.FinalScore)
	}
}

func TestHunter155MinTradeCount(t *testing.T) {
	ss := fx155Symbol("TRADEUSDT") // 200k trades
	if _, ok := fx155Score(t, ss, &store.HunterConfig{MinTradeCount: 100_000}); !ok {
		t.Fatal("200k trades must pass a 100k floor")
	}
	if _, ok := fx155Score(t, ss, &store.HunterConfig{MinTradeCount: 300_000}); ok {
		t.Fatal("200k trades must be dropped by a 300k floor")
	}
	if _, ok := fx155Score(t, ss, nil); !ok {
		t.Fatal("no floor by default")
	}
}

func TestHunter155PositionTimeframes(t *testing.T) {
	ss := fx155Symbol("TFUSDT")
	base, _ := fx155Score(t, ss, nil)
	got, _ := fx155Score(t, ss, &store.HunterConfig{PositionTimeframes: []string{"4h"}})
	if !has155(got.LongTags, "near_support_4h") || has155(got.LongTags, "near_support_1h") || got.LongFinalScore >= base.LongFinalScore {
		t.Fatalf("timeframes [4h]: long %.2f -> %.2f tags %v", base.LongFinalScore, got.LongFinalScore, got.LongTags)
	}
	if !has155(base.LongTags, "near_support_1h") || !has155(base.LongTags, "near_support_5m") {
		t.Fatalf("default must score every timeframe: %v", base.LongTags)
	}
}

// fx155Narrowing5m oscillates with a shrinking amplitude, so the current BB
// width is the narrowest of the window (percentile 0).
func fx155Narrowing5m() []datafetch.Kline {
	k := make([]datafetch.Kline, 50)
	for i := range k {
		amp := 1.0 * (1 - float64(i)/50)
		c := 100 + amp
		if i%2 == 1 {
			c = 100 - amp
		}
		k[i] = datafetch.Kline{Open: c, High: c + 0.1, Low: c - 0.1, Close: c, Volume: 1000, TakerBuy: 500}
	}
	return k
}

// Breakout fixture: the last 5m bar spikes, so the current BB width is the
// widest of the window (percentile 100); 15m stays a squeeze.
func fx155Wide5m(sym string) *datafetch.SymbolSnapshot {
	ss := fx155Symbol(sym)
	k := fx155Klines(50, 101, 100, 1000)
	k[49].Close, k[49].High = 110, 110.5
	ss.Klines["5m"] = k
	return ss
}

func TestHunter155BreakoutModeAndCoarseFilter(t *testing.T) {
	narrow := fx155Symbol("NARROWUSDT")
	narrow.Klines["5m"] = fx155Narrowing5m() // percentile 0
	n, ok := fx155Score(t, narrow, &store.HunterConfig{StrategyMode: "breakout"})
	nb, _ := fx155Score(t, narrow, nil)
	if !ok || !has155(n.Tags, "bb_width_extreme_squeeze") || n.FinalScore <= nb.FinalScore {
		t.Fatalf("breakout narrow: ok=%v %.2f vs default %.2f tags %v", ok, n.FinalScore, nb.FinalScore, n.Tags)
	}
	wide := fx155Wide5m("WIDEUSDT")
	if _, ok := fx155Score(t, wide, nil); !ok {
		t.Fatal("default mode must not funnel")
	}
	if _, ok := fx155Score(t, wide, &store.HunterConfig{StrategyMode: "breakout"}); ok {
		t.Fatal("breakout must drop a 100th-percentile BB width")
	}
	if _, ok := fx155Score(t, wide, &store.HunterConfig{StrategyMode: "breakout", BBWidthCoarseFilter: 100}); !ok {
		t.Fatal("bb_width_coarse_filter=100 must keep it")
	}
}

func TestHunter155LoneBreakerThreshold(t *testing.T) {
	wide := fx155Wide5m("LONEUSDT")
	wide.OIDelta1h = 20
	got, ok := fx155Score(t, wide, &store.HunterConfig{StrategyMode: "breakout"})
	if !ok || !has155(got.Tags, "lone_breaker_pass") {
		t.Fatalf("OI 1h 20%% > 15%% with a 15m squeeze must pass as lone breaker: ok=%v tags=%v", ok, got.Tags)
	}
	if _, ok := fx155Score(t, wide, &store.HunterConfig{StrategyMode: "breakout", OILoneBreakerThreshold: 25}); ok {
		t.Fatal("threshold 25 must not bypass at OI 1h 20%")
	}
}

func TestHunter155BBWidthCacheTTL(t *testing.T) {
	resetBBPercentileCache155()
	now := time.Unix(1_800_000_000, 0)
	hunterBBPercentileCache.now = func() time.Time { return now }
	p := ResolveHunterParams(&store.HunterConfig{StrategyMode: "breakout", BBWidthCacheTTL: 10})
	if p.BBCacheTTL != 10*time.Second || ResolveHunterParams(nil).BBCacheTTL != 180*time.Second {
		t.Fatalf("ttl = %v (default %v)", p.BBCacheTTL, ResolveHunterParams(nil).BBCacheTTL)
	}
	narrow := closesOfSnapKlines(fx155Narrowing5m())
	wide := closesOfSnapKlines(fx155Wide5m("X").Klines["5m"])
	pct1, _, pass1, _ := p.breakoutFunnel("TTLUSDT", narrow, 0)
	now = now.Add(5 * time.Second)
	pct2, _, pass2, _ := p.breakoutFunnel("TTLUSDT", wide, 0) // within TTL: cached
	now = now.Add(6 * time.Second)
	pct3, _, pass3, _ := p.breakoutFunnel("TTLUSDT", wide, 0) // expired: recomputed
	if pct1 != pct2 || !pass1 || !pass2 || pct3 != 100 || pass3 {
		t.Fatalf("cache: %v/%v -> %v/%v -> %v/%v", pct1, pass1, pct2, pass2, pct3, pass3)
	}
	resetBBPercentileCache155()
}

// The local-provider path: the same settings reach its pure scoring helpers
// and its result-cache key.
func TestHunter155LocalProviderPath(t *testing.T) {
	c := &Client{}
	sc := &SymbolCache{Klines: map[string][]klineBar{}}
	for tf, n := range map[string]int{"5m": 20, "15m": 20, "1h": 20, "4h": 20, "1d": 20} {
		bars := make([]klineBar, n)
		for i := range bars {
			cl := 110 - 10*float64(i)/float64(n-1)
			bars[i] = klineBar{Open: cl, High: cl + 0.5, Low: cl - 0.5, Close: cl, Volume: 1}
		}
		sc.Klines[tf] = bars
	}
	tk := binanceTicker{PriceChangePercent: "2"}
	def := ResolveHunterParams(nil)
	full, fullTags := c.computePositionScore("A", tk, sc, def, false)
	only4h, tags4h := c.computePositionScore("A", tk, sc, ResolveHunterParams(&store.HunterConfig{PositionTimeframes: []string{"4h"}}), false)
	if only4h >= full || has155(tags4h, "near_support_1h") || !has155(fullTags, "near_support_1h") {
		t.Fatalf("legacy timeframes: %.1f %v vs %.1f %v", full, fullTags, only4h, tags4h)
	}
	chase, chaseTags := c.computePositionScore("A", tk, sc, ResolveHunterParams(&store.HunterConfig{Max24hChange: 1}), false)
	if chase >= full || !has155(chaseTags, "chase_penalty") || has155(fullTags, "chase_penalty") {
		t.Fatalf("legacy chase: %.1f vs %.1f", chase, full)
	}
	// Breakout exemption of the 4h opposite-side penalty (SHORT near support).
	sp, spTags := c.computeShortPositionScore("A", tk, sc, def, false)
	ex, exTags := c.computeShortPositionScore("A", tk, sc, def, true)
	if ex != sp+25 || !has155(exTags, "near_support_exempt") || !has155(spTags, "near_support_4h_penalize") {
		t.Fatalf("legacy exemption: %.1f %v -> %.1f %v", sp, spTags, ex, exTags)
	}
	// Wash sensitivity.
	wtk := binanceTicker{Count: 700_000, QuoteVolume: "4900000"} // avg order $7
	medMod, _ := c.computeWashMultiplier("A", wtk, nil, 100, 1e9, hunterWashLevels["medium"])
	highMod, _ := c.computeWashMultiplier("A", wtk, nil, 100, 1e9, hunterWashLevels["high"])
	if medMod != 1 || highMod != 0.20 {
		t.Fatalf("legacy wash medium/high = %v/%v", medMod, highMod)
	}
	// Cache key changes with any setting.
	if def.Key() == ResolveHunterParams(&store.HunterConfig{MinTradeCount: 1}).Key() ||
		def.Key() != ResolveHunterParams(&store.HunterConfig{}).Key() {
		t.Fatal("cache key must change with the settings and only with them")
	}
}
