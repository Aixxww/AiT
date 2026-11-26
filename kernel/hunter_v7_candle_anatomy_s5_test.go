package kernel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/market"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// #111 S5: opt-in closed-bar candle anatomy in the legacy payload.

func s5Config(lang string, bars int) *store.StrategyConfig {
	cfg := u11FixtureConfig(lang)
	if bars != 0 {
		cfg.CoinSource.Hunter.V7Prompt = &store.V7PromptConfig{AnatomyBars: bars}
	}
	return cfg
}

// s5Render renders the legacy fixture at the fixture prompt time, with a
// 5m/15m execution context on every candidate (the provider builds one in
// production; the fixture candidates carry none).
func s5Render(cfg *store.StrategyConfig) (system, user string) {
	e := NewStrategyEngine(cfg)
	ctx := u11FixtureContext(e)
	ctx.PromptTime = u11PromptTime
	for i := range ctx.CandidateCoins {
		ctx.CandidateCoins[i].V7ExecutionContext = &local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes: map[string]local.V7ExecutionTimeframeSummary{
				"5m":  {Timeframe: "5m", CandleCount: 50},
				"15m": {Timeframe: "15m", CandleCount: 50},
			},
		}
	}
	sys := e.BuildSystemPrompt(1000, "")
	// The data dictionary's order is not stable between renders; the U11
	// goldens omit it the same way.
	if i := strings.Index(sys, "\n\n---\n\n"); i >= 0 {
		sys = "<data dictionary omitted>" + sys[i:]
	}
	return sys, e.BuildUserPrompt(ctx)
}

// Off (default or 0): the legacy prompt is byte-identical; the hybrid
// payload ignores the setting.
func TestS5OffIsByteIdentical(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		baseSys, baseUser := s5Render(s5Config(lang, 0))
		zero := s5Config(lang, 0)
		zero.CoinSource.Hunter.V7Prompt = &store.V7PromptConfig{AnatomyBars: 0}
		sys, user := s5Render(zero)
		if sys != baseSys || user != baseUser {
			t.Fatalf("%s: anatomy_bars=0 changed the prompt", lang)
		}
		if strings.Contains(baseUser, `"anatomy"`) || strings.Contains(baseSys, "anatomy") {
			t.Fatalf("%s: anatomy text with the setting off", lang)
		}
		h, hOn := u11HybridConfig(lang), u11HybridConfig(lang)
		hOn.CoinSource.Hunter.V7Prompt.AnatomyBars = 3
		d1, _, _, _ := u11RenderHybrid(h) // doc: the data dictionary omitted
		d2, _, _, _ := u11RenderHybrid(hOn)
		if d1 != d2 {
			t.Fatalf("%s: hybrid_v2 must ignore anatomy_bars", lang)
		}
	}
}

// On: each expanded candidate's 5m/15m summary carries N closed bars, the
// doctrine explains the field, and the fixture golden records it.
func TestS5OnGolden(t *testing.T) {
	sys, user := s5Render(s5Config("en", 3))
	off, offUser := s5Render(s5Config("en", 0))
	if !strings.Contains(sys, "execution_context.timeframes[tf].anatomy: the last 3 closed bars") || strings.Contains(off, "anatomy") {
		t.Fatal("doctrine line must appear only with the setting on")
	}
	n := 0
	for _, line := range strings.Split(user, "\n") {
		if !strings.HasPrefix(line, "hunter_v7_signal_json: ") {
			continue
		}
		var p struct {
			ExecutionContext struct {
				Timeframes map[string]local.V7ExecutionTimeframeSummary `json:"timeframes"`
			} `json:"execution_context"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "hunter_v7_signal_json: ")), &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		for _, tf := range []string{"5m", "15m"} {
			a := p.ExecutionContext.Timeframes[tf].Anatomy
			if len(a) != 3 || a[0].BarsAgo != 2 || a[2].BarsAgo != 0 {
				t.Fatalf("%s anatomy %+v", tf, a)
			}
		}
		n++
	}
	if n == 0 {
		t.Fatal("no expanded candidate")
	}
	added := len(user) - len(offUser)
	t.Logf("anatomy_bars=3 on 5m+15m: +%d bytes over %d expanded candidates (+%d bytes, ~%d tokens each); doctrine +%d bytes",
		added, n, added/n, added/n/4, len(sys)-len(off))
	u11CheckGolden(t, "hunter_v7_prompt_legacy_anatomy_en.golden", "===== SYSTEM (doctrine line) =====\n"+doctrineAnatomy(3, LangEnglish)+"\n===== USER =====\n"+user)
}

// The forming bar never enters the anatomy, whatever its shape; the
// sequence ends at the latest closed bar.
func TestS5ClosedBarsOnly(t *testing.T) {
	at := time.Date(2026, 11, 15, 12, 7, 30, 0, time.UTC)
	iv := int64(5 * 60 * 1000)
	lastOpen := at.UnixMilli() / iv * iv // the forming 5m bar
	var bars []market.KlineBar
	for i := 20; i >= 1; i-- {
		bars = append(bars, market.KlineBar{Time: lastOpen - int64(i)*iv, Open: 100, High: 101, Low: 99, Close: 100.5})
	}
	bars = append(bars, market.KlineBar{Time: lastOpen, Open: 100, High: 110, Low: 90, Close: 91}) // forming: huge bear
	data := &market.Data{TimeframeData: map[string]*market.TimeframeSeriesData{"5m": {Timeframe: "5m", Klines: bars}}}
	ctx := &local.V7ExecutionContext{Timeframes: map[string]local.V7ExecutionTimeframeSummary{"5m": {Timeframe: "5m", CandleCount: 21}}}
	out := hunterV7ExecutionContextWithAnatomy(ctx, data, 3, at)
	a := out.Timeframes["5m"].Anatomy
	if len(a) != 3 {
		t.Fatalf("anatomy %+v", a)
	}
	for _, b := range a {
		if b.Dir != "bull" || b.BodyPct != 25 || b.UpperWickPct != 25 || b.LowerWickPct != 50 || b.ClosePosPct != 75 || b.RangeATR != 1 {
			t.Fatalf("a closed bar was expected, got %+v (the forming bar leaked?)", b)
		}
	}
	if ctx.Timeframes["5m"].Anatomy != nil {
		t.Fatal("the candidate's own execution context must not be modified")
	}
	// One minute later the forming bar has closed and becomes bars_ago 0.
	a = hunterV7ExecutionContextWithAnatomy(ctx, data, 3, at.Add(5*time.Minute)).Timeframes["5m"].Anatomy
	if last := a[len(a)-1]; last.Dir != "bear" || last.BarsAgo != 0 || last.ClosePosPct != 5 {
		t.Fatalf("closed bar not used: %+v", last)
	}
	// Fewer closed bars than requested: what exists; no ATR without 15.
	short := hunterV7CandleAnatomy(bars[:2], 3)
	if len(short) != 2 || short[0].RangeATR != 0 {
		t.Fatalf("short series %+v", short)
	}
}

func TestS5DoctrineParityAndValidation(t *testing.T) {
	in := NewStrategyEngine(s5Config("en", 3)).hunterV7DoctrineInputs()
	if in.AnatomyBars != 3 {
		t.Fatalf("inputs %+v", in.AnatomyBars)
	}
	zh, en := doctrineTokens(hunterV7DoctrinePrompt(LangChinese, in)), doctrineTokens(hunterV7DoctrinePrompt(LangEnglish, in))
	if strings.Join(zh, " ") != strings.Join(en, " ") {
		t.Fatalf("ZH/EN doctrine tokens differ with anatomy on:\n zh %v\n en %v", zh, en)
	}
	for _, v := range []int{-1, store.MaxV7PromptAnatomyBars + 1} {
		cfg := s5Config("en", v)
		if err := store.ValidateTradingConfig(cfg); err == nil || !strings.Contains(err.Error(), "anatomy_bars") {
			t.Fatalf("anatomy_bars %d must be rejected, got %v", v, err)
		}
	}
	for _, v := range []int{0, 3, store.MaxV7PromptAnatomyBars} {
		if err := store.ValidateTradingConfig(s5Config("en", v)); err != nil && strings.Contains(err.Error(), "anatomy_bars") {
			t.Fatalf("anatomy_bars %d: %v", v, err)
		}
	}
}
