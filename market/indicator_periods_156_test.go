package market

import (
	"math"
	"reflect"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

func fx156Klines(n int) []Kline {
	out := make([]Kline, n)
	for i := range out {
		c := 100 + 5*math.Sin(float64(i)/4) + float64(i)*0.1
		out[i] = Kline{OpenTime: int64(i) * 60_000, Open: c - 0.3, High: c + 1, Low: c - 1, Close: c, Volume: 1000 + float64(i)}
	}
	return out
}

// #156: the default configuration reproduces the pre-#156 hardcoded series
// exactly (EMA 20/50, RSI 7/14, ATR 14, BOLL 20).
func TestIndicatorPeriods156DefaultsReproduceFixedSeries(t *testing.T) {
	d := calculateTimeframeSeries(fx156Klines(120), "15m", 60)
	if !reflect.DeepEqual(d.Periods, DefaultIndicatorPeriods()) {
		t.Fatalf("default periods = %+v", d.Periods)
	}
	pairs := []struct {
		name      string
		got, want []float64
	}{
		{"EMA20", d.EMASeries[20], d.EMA20Values}, {"EMA50", d.EMASeries[50], d.EMA50Values},
		{"RSI7", d.RSISeries[7], d.RSI7Values}, {"RSI14", d.RSISeries[14], d.RSI14Values},
		{"BOLL20 upper", d.BOLLSeries[20].Upper, d.BOLLUpper}, {"BOLL20 middle", d.BOLLSeries[20].Middle, d.BOLLMiddle},
		{"BOLL20 lower", d.BOLLSeries[20].Lower, d.BOLLLower},
	}
	for _, p := range pairs {
		if len(p.want) == 0 || !reflect.DeepEqual(p.got, p.want) {
			t.Errorf("%s: configured series differs from the pre-#156 series (len %d vs %d)", p.name, len(p.got), len(p.want))
		}
	}
	if d.ATRValues[14] != d.ATR14 || d.ATR14 == 0 {
		t.Errorf("ATR14 = %v vs %v", d.ATRValues[14], d.ATR14)
	}
	if len(d.EMASeries) != 2 || len(d.RSISeries) != 2 || len(d.ATRValues) != 1 || len(d.BOLLSeries) != 1 {
		t.Errorf("default build carries extra periods: %d/%d/%d/%d", len(d.EMASeries), len(d.RSISeries), len(d.ATRValues), len(d.BOLLSeries))
	}
}

// A non-default period changes the computed series.
func TestIndicatorPeriods156NonDefaultChangesSeries(t *testing.T) {
	kl := fx156Klines(120)
	p := IndicatorPeriods{EMA: []int{9}, RSI: []int{21}, ATR: []int{7}, BOLL: []int{10}}
	d := calculateTimeframeSeries(kl, "15m", 60, p)
	if !reflect.DeepEqual(d.Periods, p) {
		t.Fatalf("periods = %+v", d.Periods)
	}
	if _, ok := d.EMASeries[20]; ok || len(d.EMASeries[9]) == 0 {
		t.Fatalf("EMA series keys: %v", SortedPeriods(d.EMASeries))
	}
	if got, want := d.EMASeries[9][len(d.EMASeries[9])-1], calculateEMA(kl, 9); got != want || got == d.EMA20Values[len(d.EMA20Values)-1] {
		t.Fatalf("EMA9 last = %v, want %v and distinct from EMA20", got, want)
	}
	if got := d.RSISeries[21][len(d.RSISeries[21])-1]; got != calculateRSI(kl, 21) || got == d.RSI14Values[len(d.RSI14Values)-1] {
		t.Fatalf("RSI21 last = %v", got)
	}
	if d.ATRValues[7] != calculateATR(kl, 7) || d.ATRValues[7] == d.ATR14 {
		t.Fatalf("ATR7 = %v (ATR14 %v)", d.ATRValues[7], d.ATR14)
	}
	if u, _, _ := calculateBOLL(kl, 10, 2.0); d.BOLLSeries[10].Upper[len(d.BOLLSeries[10].Upper)-1] != u {
		t.Fatal("BOLL10 upper mismatch")
	}
}

// #156 fix-up: the intraday series maps the configured periods onto its
// fixed slots; with the defaults it reproduces the pre-#156 fixed series
// exactly (EMA 20, RSI 7/14, ATR 14).
func TestIndicatorPeriods156IntradayDefaultsReproduceFixedSeries(t *testing.T) {
	kl := fx156Klines(120)
	def := calculateIntradaySeries(kl)
	if explicit := calculateIntradaySeries(kl, DefaultIndicatorPeriods()); !reflect.DeepEqual(def, explicit) {
		t.Fatal("default intraday build differs from the explicit-default build")
	}
	if got, want := def.EMA20Values[len(def.EMA20Values)-1], calculateEMA(kl, 20); got != want {
		t.Errorf("intraday EMA last = %v, want fixed EMA20 %v", got, want)
	}
	if got, want := def.RSI7Values[len(def.RSI7Values)-1], calculateRSI(kl, 7); got != want {
		t.Errorf("intraday RSI7 last = %v, want %v", got, want)
	}
	if got, want := def.RSI14Values[len(def.RSI14Values)-1], calculateRSI(kl, 14); got != want {
		t.Errorf("intraday RSI14 last = %v, want %v", got, want)
	}
	if def.ATR14 != calculateATR(kl, 14) || def.ATR14 == 0 {
		t.Errorf("intraday ATR14 = %v, want %v", def.ATR14, calculateATR(kl, 14))
	}
}

func TestIndicatorPeriods156IntradayConfiguredPeriods(t *testing.T) {
	kl := fx156Klines(120)
	p := IndicatorPeriods{EMA: []int{9}, RSI: []int{21}, ATR: []int{7}, BOLL: []int{10}}
	d := calculateIntradaySeries(kl, p)
	if got, want := d.EMA20Values[len(d.EMA20Values)-1], calculateEMA(kl, 9); got != want || got == calculateEMA(kl, 20) {
		t.Errorf("intraday EMA slot last = %v, want EMA9 %v", got, want)
	}
	if got, want := d.RSI7Values[len(d.RSI7Values)-1], calculateRSI(kl, 21); got != want {
		t.Errorf("intraday RSI slot last = %v, want RSI21 %v", got, want)
	}
	if len(d.RSI14Values) != 0 {
		t.Errorf("a single RSI period must leave the secondary slot empty, got %d values", len(d.RSI14Values))
	}
	if d.ATR14 != calculateATR(kl, 7) {
		t.Errorf("intraday ATR slot = %v, want ATR7 %v", d.ATR14, calculateATR(kl, 7))
	}
}

// #156 fix-up: same for the longer-term builder (EMA 20/50, RSI 14,
// ATR 14 by default; ATR3 stays fixed under any configuration).
func TestIndicatorPeriods156LongerTermDefaultsReproduceFixedData(t *testing.T) {
	kl := fx156Klines(120)
	def := calculateLongerTermData(kl)
	if explicit := calculateLongerTermData(kl, DefaultIndicatorPeriods()); !reflect.DeepEqual(def, explicit) {
		t.Fatal("default longer-term build differs from the explicit-default build")
	}
	if def.EMA20 != calculateEMA(kl, 20) || def.EMA50 != calculateEMA(kl, 50) {
		t.Errorf("longer-term EMA = %v/%v, want fixed 20/50", def.EMA20, def.EMA50)
	}
	if def.ATR14 != calculateATR(kl, 14) || def.ATR3 != calculateATR(kl, 3) {
		t.Errorf("longer-term ATR = %v/%v, want fixed 14/3", def.ATR14, def.ATR3)
	}
	if got, want := def.RSI14Values[len(def.RSI14Values)-1], calculateRSI(kl, 14); got != want {
		t.Errorf("longer-term RSI14 last = %v, want %v", got, want)
	}
}

func TestIndicatorPeriods156LongerTermConfiguredPeriods(t *testing.T) {
	kl := fx156Klines(120)
	p := IndicatorPeriods{EMA: []int{9, 21}, RSI: []int{7, 21}, ATR: []int{7}}
	d := calculateLongerTermData(kl, p)
	if d.EMA20 != calculateEMA(kl, 9) || d.EMA50 != calculateEMA(kl, 21) {
		t.Errorf("longer-term EMA slots = %v/%v, want EMA9/EMA21", d.EMA20, d.EMA50)
	}
	if d.ATR14 != calculateATR(kl, 7) {
		t.Errorf("longer-term ATR slot = %v, want ATR7 %v", d.ATR14, calculateATR(kl, 7))
	}
	// ATR3 has no configuration counterpart and stays fixed.
	if d.ATR3 != calculateATR(kl, 3) {
		t.Errorf("longer-term ATR3 = %v, want fixed ATR3 %v", d.ATR3, calculateATR(kl, 3))
	}
	if got, want := d.RSI14Values[len(d.RSI14Values)-1], calculateRSI(kl, 21); got != want {
		t.Errorf("longer-term RSI slot last = %v, want RSI21 %v", got, want)
	}
	// A single configured EMA period leaves EMA50 unset.
	if single := calculateLongerTermData(kl, IndicatorPeriods{EMA: []int{9}}); single.EMA50 != 0 || single.EMA20 != calculateEMA(kl, 9) {
		t.Errorf("single-EMA longer-term = %v/%v, want EMA9/0", single.EMA20, single.EMA50)
	}
}

// BuildDataFromKlines threads the option into both legacy builders, and
// without it reproduces the no-period builds.
func TestIndicatorPeriods156BuildDataFromKlinesThreadsOption(t *testing.T) {
	kl := fx156Klines(120)
	p := IndicatorPeriods{EMA: []int{9}, RSI: []int{21}, ATR: []int{7}}
	d, err := BuildDataFromKlines("PERUSDT", kl, kl, WithIndicatorPeriods(p))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := d.IntradaySeries.EMA20Values[len(d.IntradaySeries.EMA20Values)-1]; got != calculateEMA(kl, 9) {
		t.Errorf("intraday EMA not threaded: %v", got)
	}
	if d.LongerTermContext == nil || d.LongerTermContext.ATR14 != calculateATR(kl, 7) {
		t.Errorf("longer-term ATR not threaded: %+v", d.LongerTermContext)
	}
	def, err := BuildDataFromKlines("PERUSDT", kl, kl)
	if err != nil {
		t.Fatalf("default build: %v", err)
	}
	if !reflect.DeepEqual(def.IntradaySeries, calculateIntradaySeries(kl)) ||
		!reflect.DeepEqual(def.LongerTermContext, calculateLongerTermData(kl)) {
		t.Error("default BuildDataFromKlines differs from the no-period builder output")
	}
}

func TestIndicatorPeriods156Effective(t *testing.T) {
	got := IndicatorPeriods{EMA: []int{0, 9, 9, -1, 21}, RSI: nil}.Effective()
	want := IndicatorPeriods{EMA: []int{9, 21}, RSI: []int{7, 14}, ATR: []int{14}, BOLL: []int{20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Effective = %+v, want %+v", got, want)
	}
}

// The builders thread the option through to every timeframe and the
// current values.
func TestIndicatorPeriods156BuilderThreadsOption(t *testing.T) {
	raw := make([]datafetch.Kline, 120)
	for i, k := range fx156Klines(120) {
		raw[i] = datafetch.Kline{OpenTime: k.OpenTime, CloseTime: k.OpenTime + 59_999, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume}
	}
	ss := &datafetch.SymbolSnapshot{Symbol: "PERUSDT", Price: 100, Klines: map[string][]datafetch.Kline{"15m": raw, "1h": raw}}
	p := IndicatorPeriods{EMA: []int{9}, RSI: []int{21}}
	d, err := BuildDataFromSymbolSnapshot("PERUSDT", ss, []string{"15m", "1h"}, "15m", 60, WithIndicatorPeriods(p))
	if err != nil {
		t.Skipf("snapshot fixture rejected (%v)", err)
	}
	for tf, s := range d.TimeframeData {
		if _, ok := s.EMASeries[9]; !ok || len(s.Periods.RSI) != 1 || s.Periods.RSI[0] != 21 {
			t.Fatalf("%s: periods not threaded: %+v", tf, s.Periods)
		}
	}
	if _, ok := d.CurrentEMAs[9]; !ok || d.IndicatorPeriods.EMA[0] != 9 {
		t.Fatalf("current values not threaded: %+v %+v", d.CurrentEMAs, d.IndicatorPeriods)
	}
	def, _ := BuildDataFromSymbolSnapshot("PERUSDT", ss, []string{"15m"}, "15m", 60)
	if def.CurrentEMAs[20] != def.CurrentEMA20 || def.CurrentRSIs[7] != def.CurrentRSI7 {
		t.Fatal("default current values must equal the fixed EMA20 / RSI7")
	}
}
