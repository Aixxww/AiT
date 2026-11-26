package kernel

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// #135 (F-03): the data dictionary declares OI in USDT; every prompt surface
// prints OI as USDT notional (contracts × price) under a name that says so,
// and never the raw Binance contract count.

func oiUnit135Engine() *StrategyEngine {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.EnableOI = true
	return NewStrategyEngine(&cfg)
}

func oiUnit135Data(price float64) *market.Data {
	return &market.Data{
		Symbol:       "OIUSDT",
		CurrentPrice: price,
		OpenInterest: &market.OIData{Latest: 40_000, Average: 38_000}, // contracts
		FundingRate:  0.0001,
	}
}

func TestOIUnit135CompactLineIsLabelledNotional(t *testing.T) {
	out := oiUnit135Engine().formatCompactMarketData(oiUnit135Data(500), nil)
	if !strings.Contains(out, "oi_notional_usdt=20000000 oi_avg_notional_usdt=19000000") {
		t.Fatalf("compact line does not carry the labelled USDT notional:\n%s", out)
	}
	if strings.Contains(out, "oi_latest=") || strings.Contains(out, "oi_avg=") || strings.Contains(out, "40000") {
		t.Fatalf("compact line still prints the raw contract count:\n%s", out)
	}
}

func TestOIUnit135FullMarketDataIsLabelledNotional(t *testing.T) {
	out := oiUnit135Engine().formatMarketData(oiUnit135Data(500))
	if !strings.Contains(out, "Open Interest (oi_notional_usdt): Latest: 20000000 Average: 19000000") {
		t.Fatalf("full market block does not carry the labelled USDT notional:\n%s", out)
	}
	if strings.Contains(out, "40000.00") {
		t.Fatalf("full market block still prints the raw contract count:\n%s", out)
	}
}

// Without a usable price the notional cannot be computed: the OI figure is
// omitted rather than falling back to contracts.
func TestOIUnit135NoPriceOmitsOI(t *testing.T) {
	e := oiUnit135Engine()
	data := oiUnit135Data(0)
	for name, out := range map[string]string{
		"compact": e.formatCompactMarketData(data, nil),
		"full":    e.formatMarketData(data),
	} {
		if strings.Contains(strings.ToLower(out), "oi_") || strings.Contains(out, "Open Interest") {
			t.Fatalf("%s: OI printed without a price to convert it:\n%s", name, out)
		}
	}
}

// The dictionary injected at the top of every system prompt declares USDT and
// names the notional fields; the surfaces above use exactly those names.
func TestOIUnit135DictionaryAgrees(t *testing.T) {
	def := DataDictionary["MarketData"]["OI"]
	if def.Unit != "USDT" {
		t.Fatalf("OI dictionary unit = %q, want USDT", def.Unit)
	}
	for _, name := range []string{"oi_notional_usdt", "oi_avg_notional_usdt", "derivatives_context.oi_value", "market.oi_notional_usdt"} {
		if !strings.Contains(def.DescEN, name) || !strings.Contains(def.DescZH, name) {
			t.Errorf("OI dictionary entry does not name %s (EN/ZH)", name)
		}
	}
	for _, lang := range []Language{LangEnglish, LangChinese} {
		if p := GetSchemaPrompt(lang); !strings.Contains(p, "oi_notional_usdt") {
			t.Errorf("%s schema prompt does not carry the OI notional field names", lang)
		}
	}
}

// Every OI number in the rendered legacy prompts sits under a *_notional_usdt
// label; no legacy oi_latest / oi_avg contract field survives.
func TestOIUnit135RenderedPromptsCarryUnitLabel(t *testing.T) {
	labelled := regexp.MustCompile(`oi_notional_usdt=\d+ oi_avg_notional_usdt=\d+`)
	for _, lang := range []string{"en", "zh"} {
		out := u11RenderPrompts(u11FixtureConfig(lang))
		if strings.Contains(out, "oi_latest=") || strings.Contains(out, "oi_avg=") {
			t.Fatalf("%s legacy prompt still carries oi_latest/oi_avg contract fields", lang)
		}
		if !labelled.MatchString(out) {
			t.Fatalf("%s legacy prompt has no labelled OI notional", lang)
		}
	}
}
